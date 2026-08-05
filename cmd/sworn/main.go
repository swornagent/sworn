package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/cockpit"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/journal"
	runtimepkg "github.com/swornagent/sworn/internal/runtime"
)

const usage = `Sworn carries software work through Baton's recorded handoffs.

Commands:
  init      Set this project up to work with Sworn.
  tui       Browse this project's releases and runs in an interactive terminal.
  run       Start or continue a run.
  board     Show what Sworn is doing and what happens next.
  serve     Open the run board as a local browser service.
  pause     Stop starting new work at a safe boundary.
  resume    Continue a paused run.
  cancel    Stop a run at a safe boundary.
  takeover  Continue after the previous Sworn process stopped.
  retry     Retry one stopped work item.
  answer    Answer a saved question that needs human judgment.
  approve   Admit one exact plan approval (low-level recovery/scripting).
  status    Return the stable machine-readable run record.
  driver    Check configured AI connections.
  version   Show the Sworn and Baton versions.
  help      Show this help.

Exact syntax:
  sworn version [--json]
  sworn init [--project ABS] [--force]
  sworn tui [--project ABS] [--journal ABS] [--config ABS] [--manifest-dir ABS]
  sworn run --manifest ABS --journal ABS [--config ABS]
  sworn pause|cancel --run ID --journal ABS --command ID --generation N
  sworn resume|takeover --run ID --journal ABS --command ID --generation N [--config ABS]
  sworn retry --run ID --journal ABS --command ID --generation N --work SHA256 --epoch N [--config ABS]
  sworn answer --run ID --journal ABS --attention SHA256 --generation 1 --answer TEXT [--config ABS]
  sworn approve --journal ABS [--config ABS] --run ID --manifest-digest SHA256 --project PROJECT --release RELEASE --release-ref REF --release-head OID|absent --proposal-replay-key KEY --plan-revision N --prior-plan OID|absent --plan-digest SHA256 --target-ref REF --target-head OID --decision-class CLASS --decision approve --actor-class external_authorizer --actor-authority AUTHORITY
  sworn status --run ID --journal ABS --json
  sworn board --run ID --journal ABS [--json]
  sworn serve --run ID --journal ABS [--manifest ABS] [--config ABS] [--operator-config ABS]
  sworn driver inspect|doctor|certify --config ABS (--profile PROFILE --model MODEL | --all) --json
`

const (
	swornVersion = "1.0.0-rc.2-dev"
	swornState   = "baton-rc14-admitted"
)

type versionInfo struct {
	Version string         `json:"version"`
	State   string         `json:"state"`
	Baton   baton.Identity `json:"baton"`
}

func main() {
	args := os.Args[1:]
	if len(args) == 0 && terminalIsInteractive(os.Stdin, os.Stdout) {
		args = []string{"tui"}
	}
	os.Exit(run(args, os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		_, _ = io.WriteString(stdout, usage)
		return 0
	}
	switch args[0] {
	case "tui":
		return runTUI(args[1:], stdout, stderr)
	case "version":
		return runVersion(args[1:], stdout, stderr)
	case "init":
		return runInit(args[1:], stdout, stderr)
	case "run":
		return runStart(args[1:], stdout, stderr)
	case "driver":
		return runDriver(args[1:], stdout, stderr)
	case "resume":
		return runControl(journal.Resume, args[1:], stdout, stderr)
	case "pause":
		return runControl(journal.Pause, args[1:], stdout, stderr)
	case "cancel":
		return runControl(journal.Cancel, args[1:], stdout, stderr)
	case "retry":
		return runControl(journal.Retry, args[1:], stdout, stderr)
	case "takeover":
		return runControl(journal.Takeover, args[1:], stdout, stderr)
	case "answer":
		return runAnswer(args[1:], stdout, stderr)
	case "approve":
		return runApprove(args[1:], stdout, stderr)
	case "status":
		return runStatus(args[1:], stdout, stderr)
	case "board":
		return runBoard(args[1:], stdout, stderr)
	case "serve":
		return runServe(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(
			stderr,
			"sworn: unknown command %q\nRun \"sworn help\" to see available commands.\n",
			args[0],
		)
		return 2
	}
}

func runAnswer(args []string, stdout, stderr io.Writer) int {
	options, ok := parseOptionsWithOptionalValues(
		args,
		[]string{
			"--run",
			"--journal",
			"--attention",
			"--generation",
			"--answer",
		},
		[]string{"--config"},
		nil,
		nil,
	)
	if !ok {
		fmt.Fprintln(
			stderr,
			"usage: sworn answer --run ID --journal PATH "+
				"--attention SHA256 --generation 1 --answer TEXT [--config ABS]",
		)
		return 2
	}
	generation, err := strconv.ParseInt(options["--generation"], 10, 64)
	if err != nil || generation != 1 {
		fmt.Fprintln(
			stderr,
			"sworn answer: generation must be 1 for an unanswered question",
		)
		return 2
	}
	ctx := context.Background()
	service, factory, err := openRuntimeService(
		ctx,
		options["--journal"],
		options["--config"],
	)
	if err != nil {
		writeCommandFailure(
			stderr,
			"answer",
			"Could not open the saved run or its AI connections.",
			err,
		)
		return 1
	}
	defer service.Close()
	defer factory.Close()
	status, err := service.AnswerAttention(
		ctx,
		runtimepkg.AnswerAttentionCommand{
			RunID:              options["--run"],
			AttentionID:        options["--attention"],
			ExpectedGeneration: generation,
			Answer:             options["--answer"],
		},
	)
	if err != nil {
		writeCommandFailure(
			stderr,
			"answer",
			"Could not record that answer.",
			err,
		)
		return 1
	}
	if err := writeStatusText(stdout, status); err != nil {
		fmt.Fprintln(stderr, "sworn answer: output failed")
		return 1
	}
	return 0
}

func runVersion(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	if len(args) == 1 && args[0] == "--json" {
		asJSON = true
	} else if len(args) != 0 {
		fmt.Fprintln(stderr, "usage: sworn version [--json]")
		return 2
	}
	pkg, err := baton.Load()
	if err != nil {
		writeCommandFailure(
			stderr,
			"version",
			"Could not read the included Baton package.",
			err,
		)
		return 1
	}
	batonIdentity, err := pkg.Identity()
	if err != nil {
		writeCommandFailure(
			stderr,
			"version",
			"Could not confirm the included Baton identity.",
			err,
		)
		return 1
	}
	if err := writeVersion(stdout, asJSON, batonIdentity); err != nil {
		fmt.Fprintln(stderr, "sworn version: Could not write the version result.")
		return 1
	}
	return 0
}

func runStart(args []string, stdout, stderr io.Writer) int {
	options, ok := parseOptionsWithOptionalValues(
		args,
		[]string{"--manifest", "--journal"},
		[]string{"--config"},
		nil,
		nil,
	)
	if !ok {
		fmt.Fprintln(
			stderr,
			"usage: sworn run --manifest PATH --journal PATH [--config ABS]",
		)
		return 2
	}
	body, err := readManifest(options["--manifest"])
	if err != nil {
		writeKnownFailure(
			stderr,
			"run",
			"Could not read the run definition. Check that --manifest points to an absolute regular file.",
			"",
		)
		return 1
	}
	if _, err := runtimepkg.ParseManifest(body); err != nil {
		writeCommandFailure(
			stderr,
			"run",
			"The run definition is invalid. Check that it is current canonical JSON with a final newline.",
			err,
		)
		return 1
	}
	ctx := context.Background()
	service, factory, err := openRuntimeService(
		ctx,
		options["--journal"],
		options["--config"],
	)
	if err != nil {
		writeCommandFailure(
			stderr,
			"run",
			"Could not open the saved run or its AI connections.",
			err,
		)
		return 1
	}
	defer service.Close()
	defer factory.Close()
	status, err := service.Start(ctx, body)
	if err != nil {
		writeCommandFailure(
			stderr,
			"run",
			"Could not start or continue this run.",
			err,
		)
		return 1
	}
	if err := writeStatusText(stdout, status); err != nil {
		fmt.Fprintln(stderr, "sworn run: output failed")
		return 1
	}
	return 0
}

func runControl(kind journal.ControlKind, args []string, stdout, stderr io.Writer) int {
	values := []string{"--run", "--journal", "--command", "--generation"}
	if kind == journal.Retry {
		values = append(values, "--work", "--epoch")
	}
	var optionalValues []string
	if kind == journal.Resume || kind == journal.Retry ||
		kind == journal.Takeover {
		optionalValues = []string{"--config"}
	}
	options, ok := parseOptionsWithOptionalValues(
		args,
		values,
		optionalValues,
		nil,
		nil,
	)
	if !ok {
		fmt.Fprintf(stderr, "usage: sworn %s --run ID --journal PATH --command ID --generation N", kind)
		if kind == journal.Retry {
			fmt.Fprint(stderr, " --work SHA256 --epoch N")
		}
		if len(optionalValues) != 0 {
			fmt.Fprint(stderr, " [--config ABS]")
		}
		fmt.Fprintln(stderr)
		return 2
	}
	generation, err := strconv.ParseInt(options["--generation"], 10, 64)
	if err != nil || generation < 0 {
		fmt.Fprintf(
			stderr,
			"sworn %s: generation must be the non-negative whole number from the latest board\n",
			kind,
		)
		return 2
	}
	epoch := int64(0)
	if kind == journal.Retry {
		epoch, err = strconv.ParseInt(options["--epoch"], 10, 64)
		if err != nil || epoch < 1 {
			fmt.Fprintln(
				stderr,
				"sworn retry: epoch must be the positive whole number from the latest retry action",
			)
			return 2
		}
	}
	ctx := context.Background()
	service, factory, err := openRuntimeService(
		ctx,
		options["--journal"],
		options["--config"],
	)
	if err != nil {
		writeCommandFailure(
			stderr,
			string(kind),
			"Could not open the saved run or its AI connections.",
			err,
		)
		return 1
	}
	defer service.Close()
	defer factory.Close()
	status, err := service.Control(ctx, runtimepkg.ControlCommand{
		RunID: options["--run"], ID: options["--command"], Kind: kind,
		ExpectedGeneration: generation, WorkID: options["--work"], ExpectedEpoch: epoch,
	})
	if err != nil {
		writeCommandFailure(
			stderr,
			string(kind),
			"Could not apply that control to the current run.",
			err,
		)
		return 1
	}
	if err := writeStatusText(stdout, status); err != nil {
		fmt.Fprintf(stderr, "sworn %s: output failed\n", kind)
		return 1
	}
	return 0
}

func runStatus(args []string, stdout, stderr io.Writer) int {
	options, ok := parseOptions(args, []string{"--run", "--journal"}, []string{"--json"})
	if !ok {
		fmt.Fprintln(stderr, "usage: sworn status --run ID --journal PATH --json")
		return 2
	}
	ctx := context.Background()
	service, err := runtimepkg.OpenStatusService(ctx, options["--journal"])
	if err != nil {
		writeCommandFailure(
			stderr,
			"status",
			"Could not open the saved run record.",
			err,
		)
		return 1
	}
	defer service.Close()
	status, err := service.Status(ctx, options["--run"])
	if err != nil {
		writeCommandFailure(
			stderr,
			"status",
			"Could not find that run in the saved record.",
			err,
		)
		return 1
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(status); err != nil {
		fmt.Fprintln(stderr, "sworn status: output failed")
		return 1
	}
	return 0
}

func runBoard(args []string, stdout, stderr io.Writer) int {
	options, ok := parseOptionsWithOptionalSwitches(
		args,
		[]string{"--run", "--journal"},
		[]string{"--json"},
	)
	if !ok {
		fmt.Fprintln(stderr, "usage: sworn board --run ID --journal PATH [--json]")
		return 2
	}
	ctx := context.Background()
	snapshot, err := readRunBoard(
		ctx,
		options["--run"],
		options["--journal"],
	)
	if err != nil {
		if errors.Is(err, errRunBoardGit) {
			writeKnownFailure(
				stderr,
				"board",
				"Could not find Git. Install Git or make it available on PATH.",
				"GIT_UNAVAILABLE",
			)
			return 1
		}
		if errors.Is(err, errRunBoardJournal) {
			writeKnownFailure(
				stderr,
				"board",
				"Could not open the saved run record. Check the journal path and file permissions.",
				"JOURNAL_UNAVAILABLE",
			)
			return 1
		}
		writeCommandFailure(
			stderr,
			"board",
			"Could not build the delivery board from the saved run and Git state.",
			err,
		)
		return 1
	}
	if options["--json"] == "true" {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		err = encoder.Encode(snapshot)
	} else {
		_, err = io.WriteString(stdout, cockpit.RenderTerminal(snapshot))
	}
	if err != nil {
		fmt.Fprintln(stderr, "sworn board: output failed")
		return 1
	}
	return 0
}

func parseOptions(
	args []string,
	values []string,
	switches []string,
) (map[string]string, bool) {
	return parseOptionSets(args, values, switches, nil)
}

func parseOptionsWithOptionalValues(
	args []string,
	requiredValues []string,
	optionalValues []string,
	requiredSwitches []string,
	optionalSwitches []string,
) (map[string]string, bool) {
	return parseOptionSetsWithOptionalValues(
		args,
		requiredValues,
		optionalValues,
		requiredSwitches,
		optionalSwitches,
	)
}

func parseOptionsWithOptionalSwitches(
	args []string,
	values []string,
	switches []string,
) (map[string]string, bool) {
	return parseOptionSets(args, values, nil, switches)
}

func parseOptionSets(
	args []string,
	values []string,
	requiredSwitches []string,
	optionalSwitches []string,
) (map[string]string, bool) {
	return parseOptionSetsWithOptionalValues(
		args,
		values,
		nil,
		requiredSwitches,
		optionalSwitches,
	)
}

func parseOptionSetsWithOptionalValues(
	args []string,
	requiredValues []string,
	optionalValues []string,
	requiredSwitches []string,
	optionalSwitches []string,
) (map[string]string, bool) {
	allowedValues := make(
		map[string]bool,
		len(requiredValues)+len(optionalValues),
	)
	for _, name := range requiredValues {
		allowedValues[name] = true
	}
	for _, name := range optionalValues {
		allowedValues[name] = true
	}
	allowedSwitches := make(
		map[string]bool,
		len(requiredSwitches)+len(optionalSwitches),
	)
	for _, name := range requiredSwitches {
		allowedSwitches[name] = true
	}
	for _, name := range optionalSwitches {
		allowedSwitches[name] = true
	}
	result := make(
		map[string]string,
		len(requiredValues)+len(optionalValues)+
			len(requiredSwitches)+len(optionalSwitches),
	)
	for index := 0; index < len(args); index++ {
		name := args[index]
		if allowedSwitches[name] {
			if _, duplicate := result[name]; duplicate {
				return nil, false
			}
			result[name] = "true"
			continue
		}
		if !allowedValues[name] || index+1 >= len(args) {
			return nil, false
		}
		if _, duplicate := result[name]; duplicate {
			return nil, false
		}
		index++
		if args[index] == "" || strings.HasPrefix(args[index], "--") {
			return nil, false
		}
		result[name] = args[index]
	}
	for _, name := range requiredValues {
		if result[name] == "" {
			return nil, false
		}
	}
	for _, name := range requiredSwitches {
		if result[name] != "true" {
			return nil, false
		}
	}
	return result, true
}

func openRuntimeService(
	ctx context.Context,
	journalPath string,
	configPath string,
) (*runtimepkg.Service, *driver.ProductionDriverFactory, error) {
	if configPath == "" {
		service, err := runtimepkg.OpenService(ctx, journalPath)
		return service, nil, err
	}
	loaded, err := driver.LoadDriverConfig(configPath)
	if err != nil {
		return nil, nil, err
	}
	factory, err := driver.NewProductionDriverFactory(loaded)
	if err != nil {
		return nil, nil, err
	}
	service, err := runtimepkg.OpenServiceWithDriverConfig(
		ctx,
		journalPath,
		loaded,
		factory.Options(),
	)
	if err != nil {
		_ = factory.Close()
		return nil, nil, err
	}
	return service, factory, nil
}

func resolveGitExecutable() (string, error) {
	executable, err := exec.LookPath("git")
	if err != nil {
		return "", err
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(executable)
}

func readManifest(path string) ([]byte, error) {
	if path == "" || !filepath.IsAbs(path) {
		return nil, errors.New("manifest path must be absolute")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() ||
		info.Mode()&os.ModeSymlink != 0 ||
		info.Size() < 1 || info.Size() > runtimepkg.MaxManifestBytes {
		return nil, errors.New("manifest is not an admitted regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil || !os.SameFile(info, openedInfo) {
		return nil, errors.New("manifest changed while it was admitted")
	}
	body, err := io.ReadAll(io.LimitReader(file, runtimepkg.MaxManifestBytes+1))
	if err != nil || len(body) > runtimepkg.MaxManifestBytes {
		return nil, errors.New("manifest exceeds the limit")
	}
	return body, nil
}

func writeStatusText(out io.Writer, status runtimepkg.RunStatus) error {
	presentation := cockpit.PresentRunState(status.State)
	plan := status.PlanDigest
	if plan == "" {
		plan = "not recorded"
	}
	outcome := status.Outcome
	if outcome == "" {
		outcome = "not recorded"
	}
	_, err := fmt.Fprintf(
		out,
		"Sworn run %s\n"+
			"Status: %s\n"+
			"What's happening: %s\n"+
			"Next: %s\n"+
			"Needs you: %s\n"+
			"Checked: %s\n\n"+
			"Technical details:\n"+
			"  state: %s\n"+
			"  desired_state: %s\n"+
			"  control_generation: %d\n"+
			"  plan: %s\n"+
			"  outcome: %s\n"+
			"  authority_state: %s\n"+
			"  project: %s\n"+
			"  external_authorizer: %s\n"+
			"  authority_digest: %s\n",
		status.RunID,
		presentation.Status,
		presentation.What,
		presentation.Next,
		presentation.NeedsYou,
		presentation.Checked,
		status.State,
		status.DesiredState,
		status.ControlGeneration,
		plan,
		outcome,
		status.AuthorityState,
		status.Project,
		status.ExternalAuthorizer,
		status.AuthorityDigest,
	)
	return err
}

func writeVersion(out io.Writer, asJSON bool, batonIdentity baton.Identity) error {
	info := versionInfo{
		Version: swornVersion,
		State:   swornState,
		Baton:   batonIdentity,
	}
	if asJSON {
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(info)
	}
	_, err := fmt.Fprintf(
		out,
		"Sworn %s\n"+
			"Baton %s\n\n"+
			"Technical details:\n"+
			"  state: %s\n"+
			"  baton commit: %s\n",
		info.Version,
		info.Baton.PackageVersion,
		info.State,
		info.Baton.Commit,
	)
	return err
}

func writeKnownFailure(
	out io.Writer,
	command string,
	message string,
	code string,
) {
	fmt.Fprintf(
		out,
		"sworn %s: %s\n",
		command,
		message,
	)
	if code != "" {
		fmt.Fprintf(out, "Technical code: %s\n", code)
	}
}

func writeCommandFailure(
	out io.Writer,
	command string,
	fallback string,
	err error,
) {
	code := commandErrorCode(err)
	message := fallback
	switch code {
	case "APPROVAL_PENDING":
		message = "The plan is waiting for approval."
	case "RECOVERY_UNCERTAIN":
		message = "Cannot confirm whether the last external action finished. Recover the run before retrying it."
	case "EFFECT_PARKED":
		message = "The work stopped after repeated failures. Review the latest board before retrying."
	case "RUN_NOT_FOUND", "INVALID_RUN":
		message = "Could not find that run in the saved record."
	case "BATON_UNAVAILABLE":
		message = "Could not confirm the current Baton handoff records."
	case "GIT_UNAVAILABLE":
		message = "Could not find or use Git."
	}
	fmt.Fprintf(out, "sworn %s: %s\n", command, message)
	if code != "" {
		fmt.Fprintf(out, "Technical code: %s\n", code)
	}
}

func commandErrorCode(err error) string {
	var runtimeErr *runtimepkg.Error
	if errors.As(err, &runtimeErr) {
		return runtimeErr.Code
	}
	var journalErr *journal.Error
	if errors.As(err, &journalErr) {
		return journalErr.Code
	}
	var driverErr *driver.ContractError
	if errors.As(err, &driverErr) {
		return driverErr.Code
	}
	var cockpitErr *cockpit.Error
	if errors.As(err, &cockpitErr) {
		return cockpitErr.Code
	}
	return ""
}
