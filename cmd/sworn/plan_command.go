package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/gitx"
)

// planEngineIdentity is the explicit engine identity the plan verbs commit
// with. It is attribution only; the approval_ref discipline is untouched.
var planEngineIdentity = gitx.Identity{
	Name:  "Sworn Plan Engine",
	Email: "plan@sworn.dev",
}

func runPlan(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: sworn plan pin|lint|record ...")
		return 2
	}
	verb := args[0]
	rest := args[1:]
	switch verb {
	case "pin":
		return runPlanPin(rest, stdout, stderr)
	case "lint":
		return runPlanLint(rest, stdout, stderr)
	case "record":
		return runPlanRecord(rest, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "sworn plan: unknown verb %q\n", verb)
		return 2
	}
}

func runPlanPin(args []string, stdout, stderr io.Writer) int {
	options, ok := parsePlanManifestOptions(args, []string{"--manifest", "--project"}, []string{"--commit"})
	if !ok {
		fmt.Fprintln(stderr, "usage: sworn plan pin --manifest ABS --project ABS [--commit OID]")
		return 2
	}
	manifestBytes, err := readManifest(options["--manifest"])
	if err != nil {
		writeKnownFailure(stderr, "plan pin", "Could not read the manifest. Check that --manifest points to an absolute regular file.", "")
		return 1
	}
	repo, err := openPlanRepository(options["--project"])
	if err != nil {
		writeKnownFailure(stderr, "plan pin", "Could not open the Git project.", commandErrorCode(err))
		return 1
	}
	gitRepo := baton.UseGitRepository(repo)
	pinned, err := baton.PinManifest(baton.PinManifestInput{
		ManifestBytes: manifestBytes,
		Repository:    gitRepo,
		Commit:        options["--commit"],
	})
	if err != nil {
		writeCommandFailure(stderr, "plan pin", "Could not pin the manifest.", err)
		return 1
	}
	if _, err := stdout.Write(pinned); err != nil {
		fmt.Fprintln(stderr, "sworn plan pin: output failed")
		return 1
	}
	return 0
}

func runPlanLint(args []string, stdout, stderr io.Writer) int {
	options, ok := parsePlanManifestOptions(args, []string{"--manifest", "--project"}, []string{"--commit"})
	if !ok {
		fmt.Fprintln(stderr, "usage: sworn plan lint --manifest ABS --project ABS [--commit OID]")
		return 2
	}
	manifestBytes, err := readManifest(options["--manifest"])
	if err != nil {
		writeKnownFailure(stderr, "plan lint", "Could not read the manifest. Check that --manifest points to an absolute regular file.", "")
		return 1
	}
	repo, err := openPlanRepository(options["--project"])
	if err != nil {
		writeKnownFailure(stderr, "plan lint", "Could not open the Git project.", commandErrorCode(err))
		return 1
	}
	gitRepo := baton.UseGitRepository(repo)
	results, err := baton.RunPlanScopeLint(baton.RunPlanScopeLintInput{
		ManifestBytes: manifestBytes,
		Repository:    gitRepo,
		Commit:        options["--commit"],
	})
	if err != nil {
		writeCommandFailure(stderr, "plan lint", "Scope lint failed.", err)
		for _, r := range results {
			if r.Status == "FAIL" && len(r.Paths) > 0 {
				fmt.Fprintf(stderr, "  missing: %s\n", strings.Join(r.Paths, ", "))
			}
		}
		return 1
	}
	for _, r := range results {
		fmt.Fprintf(stdout, "%s: %s\n", r.Slice, r.Status)
	}
	return 0
}

func runPlanRecord(args []string, stdout, stderr io.Writer) int {
	options, ok := parsePlanManifestOptions(
		args,
		[]string{"--manifest", "--project", "--summary"},
		[]string{"--detail-file", "--commit", "--contract-tree"},
	)
	if !ok {
		fmt.Fprintln(stderr, "usage: sworn plan record --manifest ABS --project ABS --summary TEXT [--detail-file ABS] [--commit OID] [--contract-tree OID]")
		return 2
	}
	manifestBytes, err := readManifest(options["--manifest"])
	if err != nil {
		writeKnownFailure(stderr, "plan record", "Could not read the manifest. Check that --manifest points to an absolute regular file.", "")
		return 1
	}
	repo, err := openPlanRepository(options["--project"])
	if err != nil {
		writeKnownFailure(stderr, "plan record", "Could not open the Git project.", commandErrorCode(err))
		return 1
	}

	// Resolve the contract tree: --contract-tree takes priority, then
	// --commit, then the working repository's HEAD. A3 pins the default as
	// "the working repository's head as ContractTree", so the default must
	// resolve without operator input.
	contractTree := options["--contract-tree"]
	if contractTree == "" {
		contractTree = options["--commit"]
	}
	if contractTree == "" {
		head, headErr := resolveRepositoryHEAD(repo)
		if headErr != nil {
			writeKnownFailure(stderr, "plan record", "Could not resolve the working repository HEAD.", commandErrorCode(headErr))
			return 1
		}
		contractTree = head
	}

	var detail []byte
	if detailFile := options["--detail-file"]; detailFile != "" {
		detail, err = os.ReadFile(detailFile)
		if err != nil {
			writeKnownFailure(stderr, "plan record", "Could not read the detail file.", "")
			return 1
		}
	}

	inertness := func(request gitx.RecordRootRequest) (gitx.RecordRootDecision, error) {
		return gitx.RecordRootDecision{
			Kind: request.Kind, Repository: request.Repository,
			RecordRoot: request.RecordRoot, Commit: request.Commit,
			Decision: "inert",
		}, nil
	}
	actions, err := baton.NewActions(baton.UseGitRepository(repo), inertness, planEngineIdentity)
	if err != nil {
		writeCommandFailure(stderr, "plan record", "Could not open the recording engine.", err)
		return 1
	}
	result, err := actions.RecordPlanRevision(baton.RecordPlanRevisionInput{
		PlanBytes:    manifestBytes,
		Summary:      options["--summary"],
		Detail:       detail,
		ContractTree: contractTree,
	})
	if err != nil {
		writeCommandFailure(stderr, "plan record", "Could not record the plan revision.", err)
		return 1
	}

	// Surface the resulting state (approval result, diagnostics) the way
	// the scratch tool does today (Correction 4). ReadState yields the
	// approval and diagnostics A3 names.
	gitRepo := baton.UseGitRepository(repo)
	state, stateErr := baton.ReadState(gitRepo, result.Release, inertness)
	fmt.Fprintf(stdout, "Recorded plan revision %d for release %s.\n", result.Revision, result.Release)
	fmt.Fprintf(stdout, "  plan: %s\n", result.Plan)
	fmt.Fprintf(stdout, "  ref: %s\n", result.Ref)
	fmt.Fprintf(stdout, "  head: %s\n", result.Head)
	fmt.Fprintf(stdout, "  target: %s\n", result.Target)
	if stateErr == nil {
		if state.Plan.Approval.Receipt.Result != "" {
			fmt.Fprintf(stdout, "  approval: %s\n", state.Plan.Approval.Receipt.Result)
		}
		for _, diag := range state.Diagnostics {
			fmt.Fprintf(stdout, "  diagnostic: %s: %s\n", diag.Code, diag.Message)
		}
	}
	return 0
}

// parsePlanManifestOptions parses the shared --manifest/--project flags with
// optional value flags. It follows the same absolute-path validation as the
// existing cmd/sworn option parsers.
func parsePlanManifestOptions(
	args []string,
	requiredValues []string,
	optionalValues []string,
) (map[string]string, bool) {
	options, ok := parseOptionsWithOptionalValues(
		args, requiredValues, optionalValues, nil, nil,
	)
	if !ok {
		return nil, false
	}
	for _, key := range []string{"--manifest", "--project", "--detail-file"} {
		if val := options[key]; val != "" {
			if !filepath.IsAbs(val) || filepath.Clean(val) != val || strings.ContainsRune(val, 0) {
				return nil, false
			}
		}
	}
	return options, true
}

// openPlanRepository opens the Git project at the given absolute path.
func openPlanRepository(project string) (*gitx.Repository, error) {
	if project == "" || !filepath.IsAbs(project) || filepath.Clean(project) != project {
		return nil, fmt.Errorf("project must be an absolute path to the Git project")
	}
	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		return nil, fmt.Errorf("could not find Git")
	}
	return gitx.Open(project, gitExecutable)
}

// resolveRepositoryHEAD resolves the working repository's HEAD commit OID by
// running the resolved Git executable directly. gitx.Repository.run is
// unexported and CaptureHeadRefs rejects HEAD via ValidateHeadRef's
// refs/heads/ prefix requirement (Correction 1), so the head is resolved inside
// cmd/sworn using the exported gitx surface (Root, GitExecutable,
// ObjectFormat) and the same sanitized git execution pattern as
// cmd/sworn/init_command.go. The pattern follows the exact precedent of
// MigrateLegacyRecords (migrate.go:83: rev-parse --verify HEAD^{commit}).
func resolveRepositoryHEAD(repo *gitx.Repository) (string, error) {
	if repo == nil {
		return "", fmt.Errorf("one admitted Git repository is required")
	}
	command := exec.Command(repo.GitExecutable(), "-C", repo.Root(),
		"rev-parse", "--verify", "HEAD^{commit}")
	command.Env = []string{
		"HOME=/tmp", "LANG=C", "LC_ALL=C", "TZ=UTC",
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_NO_REPLACE_OBJECTS=1",
		"GIT_LITERAL_PATHSPECS=1", "GIT_TERMINAL_PROMPT=0",
	}
	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("could not resolve HEAD: %w", err)
	}
	head := strings.TrimSpace(string(output))
	if head == "" {
		return "", fmt.Errorf("HEAD resolved to an empty identity")
	}
	// Validate the OID is well-formed for the repository's object format.
	if _, err := gitx.ParseOID(repo.ObjectFormat(), head); err != nil {
		return "", fmt.Errorf("HEAD resolved to an invalid OID: %w", err)
	}
	return head, nil
}
