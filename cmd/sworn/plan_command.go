package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/protocol"
)

// engineIdentityDomain is the one domain every machine identity the engine
// commits with lives on. It must be a domain the project controls: a
// records commit's author email is an identity other systems act on
// (Vercel refuses deploys from non-member authors, forges link it to
// accounts), and sworn.dev, the original value, is not ours.
const engineIdentityDomain = "sworn.sh"

// planEngineIdentity is the explicit engine identity the plan verbs commit
// with. It is attribution only; the approval_ref discipline is untouched.
var planEngineIdentity = gitx.Identity{
	Name:  "Sworn Plan Engine",
	Email: "plan@" + engineIdentityDomain,
}

const planStaleBindingHint = "Run `sworn plan pin --write --manifest ABS --project ABS` first to refresh the pinned facts."

// parsePlanPinOptions parses pin's required and optional value flags plus the
// optional --write switch. It mirrors parsePlanManifestOptions validation and
// refuses a duplicated or misplaced switch with the same closed usage path.
func parsePlanPinOptions(args []string) (map[string]string, bool, bool) {
	options, ok := parseOptionsWithOptionalValues(
		args, []string{"--manifest", "--project"}, []string{"--commit"}, nil, []string{"--write"},
	)
	if !ok {
		return nil, false, false
	}
	for _, key := range []string{"--manifest", "--project"} {
		if val := options[key]; val != "" {
			if !filepath.IsAbs(val) || filepath.Clean(val) != val || strings.ContainsRune(val, 0) {
				return nil, false, false
			}
		}
	}
	return options, options["--write"] == "true", true
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
	options, write, ok := parsePlanPinOptions(args)
	if !ok {
		fmt.Fprintln(stderr, "usage: sworn plan pin --manifest ABS --project ABS [--commit OID] [--write]")
		return 2
	}
	manifestBytes, err := readManifest(options["--manifest"])
	if err != nil {
		writeKnownFailure(stderr, "plan pin", "Could not read the manifest. Check that --manifest points to an absolute regular file.", "")
		return 1
	}
	repo, err := openPlanRepository(options["--project"])
	if err != nil {
		writeKnownFailure(stderr, "plan pin", "Could not open the Git project.", commandErrorCode(err), commandErrorDetail(err))
		return 1
	}
	commit := options["--commit"]
	var resolved string
	if commit != "" {
		resolved, err = resolvePlanRevisionFlag(repo, "--commit", commit)
		if err != nil {
			writeCommandFailure(stderr, "plan pin", "Could not resolve --commit to exactly one commit.", err)
			return 1
		}
		commit = resolved
	}
	gitRepo := protocol.UseGitRepository(repo)
	pinned, err := protocol.PinManifest(protocol.PinManifestInput{
		ManifestBytes: manifestBytes,
		Repository:    gitRepo,
		Commit:        commit,
	})
	if err != nil {
		writeCommandFailure(stderr, "plan pin", "Could not pin the manifest.", err)
		return 1
	}
	if write {
		if err := writeFileAtomic(options["--manifest"], pinned, protocol.MaxPlanBytes); err != nil {
			writeKnownFailure(stderr, "plan pin", "Could not write the pinned manifest. Check that --manifest points to an absolute regular file.", "")
			return 1
		}
		if _, err := fmt.Fprintf(stdout, "plan: %s\n", protocol.DigestBytes(pinned)); err != nil {
			fmt.Fprintln(stderr, "sworn plan pin: output failed")
			return 1
		}
		if resolved != "" {
			fmt.Fprintf(stderr, "commit: %s\n", resolved)
		}
		fmt.Fprintln(stderr, "wrote pinned manifest (--manifest)")
		return 0
	}
	if _, err := stdout.Write(pinned); err != nil {
		fmt.Fprintln(stderr, "sworn plan pin: output failed")
		return 1
	}
	if resolved != "" {
		fmt.Fprintf(stderr, "commit: %s\n", resolved)
	}
	fmt.Fprintln(stderr, "pinned manifest printed to stdout (--manifest)")
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
		writeKnownFailure(stderr, "plan lint", "Could not open the Git project.", commandErrorCode(err), commandErrorDetail(err))
		return 1
	}
	commit := options["--commit"]
	var resolved string
	if commit != "" {
		resolved, err = resolvePlanRevisionFlag(repo, "--commit", commit)
		if err != nil {
			writeCommandFailure(stderr, "plan lint", "Could not resolve --commit to exactly one commit.", err)
			return 1
		}
		commit = resolved
	}
	gitRepo := protocol.UseGitRepository(repo)
	results, err := protocol.RunPlanScopeLint(protocol.RunPlanScopeLintInput{
		ManifestBytes: manifestBytes,
		Repository:    gitRepo,
		Commit:        commit,
	})
	if err != nil {
		writeCommandFailure(stderr, "plan lint", "Scope lint failed.", err)
		if protocol.ErrorCode(err) == "STALE_BINDING" {
			fmt.Fprintln(stderr, planStaleBindingHint)
		}
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
	if resolved != "" {
		fmt.Fprintf(stdout, "commit: %s\n", resolved)
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
		writeKnownFailure(stderr, "plan record", "Could not open the Git project.", commandErrorCode(err), commandErrorDetail(err))
		return 1
	}

	// Resolve the contract tree: --contract-tree takes priority, then
	// --commit, then the working repository's HEAD. Every revision is
	// resolved once at the boundary through the sanitized gitx resolver;
	// only full ids reach the protocol.
	var commitResolved string
	if options["--commit"] != "" {
		var err error
		commitResolved, err = resolvePlanRevisionFlag(repo, "--commit", options["--commit"])
		if err != nil {
			writeCommandFailure(stderr, "plan record", "Could not resolve --commit to exactly one commit.", err)
			return 1
		}
	}
	var contractTree, contractTreeSource string
	if options["--contract-tree"] != "" {
		resolved, err := resolvePlanRevisionFlag(repo, "--contract-tree", options["--contract-tree"])
		if err != nil {
			writeCommandFailure(stderr, "plan record", "Could not resolve --contract-tree to exactly one commit.", err)
			return 1
		}
		contractTree = resolved
		contractTreeSource = "--contract-tree"
	} else if commitResolved != "" {
		contractTree = commitResolved
		contractTreeSource = "--commit"
	} else {
		headOID, headErr := repo.ResolveCommitRevision("HEAD")
		if headErr != nil {
			writeCommandFailure(stderr, "plan record", "Could not resolve the working repository HEAD.", headErr)
			return 1
		}
		contractTree = headOID.String()
		contractTreeSource = "HEAD"
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
	actions, err := protocol.NewActions(protocol.UseGitRepository(repo), inertness, planEngineIdentity)
	if err != nil {
		writeCommandFailure(stderr, "plan record", "Could not open the recording engine.", err)
		return 1
	}
	result, err := actions.RecordPlanRevision(protocol.RecordPlanRevisionInput{
		PlanBytes:    manifestBytes,
		Summary:      options["--summary"],
		Detail:       detail,
		ContractTree: contractTree,
	})
	if err != nil {
		writeCommandFailure(stderr, "plan record", "Could not record the plan revision.", err)
		if protocol.ErrorCode(err) == "STALE_BINDING" {
			fmt.Fprintln(stderr, planStaleBindingHint)
		}
		return 1
	}

	// Surface the resulting state (approval result, diagnostics) the way
	// the scratch tool does today (Correction 4). ReadState yields the
	// approval and diagnostics A3 names.
	gitRepo := protocol.UseGitRepository(repo)
	state, stateErr := protocol.ReadState(gitRepo, result.Release, inertness)
	fmt.Fprintf(stdout, "Recorded plan revision %d for release %s.\n", result.Revision, result.Release)
	if commitResolved != "" {
		fmt.Fprintf(stdout, "  commit: %s\n", commitResolved)
	}
	fmt.Fprintf(stdout, "  contract-tree: %s (from %s)\n", contractTree, contractTreeSource)
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

// resolvePlanRevisionFlag resolves one --commit or --contract-tree value
// through the sanitized gitx resolver. An empty value resolves to an empty
// id (working-tree mode for pin/lint); otherwise the full resolved id is
// returned. Failures are typed gitx errors with fixed, value-free detail.
func resolvePlanRevisionFlag(repo *gitx.Repository, flag, value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if repo == nil {
		return "", fmt.Errorf("one admitted Git repository is required for %s", flag)
	}
	oid, err := repo.ResolveCommitRevision(value)
	if err != nil {
		return "", err
	}
	return oid.String(), nil
}
