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

const usage = `Sworn runs autonomous delivery with the Baton protocol.

Available in the v0.3 walking skeleton:
  sworn version [--json]
  sworn run --manifest PATH --journal PATH [--config ABS]
  sworn pause|resume|cancel|takeover --run ID --journal PATH --command ID --generation N
  sworn retry --run ID --journal PATH --command ID --generation N --work SHA256 --epoch N
  sworn answer --run ID --journal PATH --attention SHA256 --generation 1 --answer TEXT [--config ABS]
  sworn driver inspect|doctor|certify --config ABS (--profile PROFILE --model MODEL | --all) --json
  sworn status --run ID --journal PATH --json
  sworn board --run ID --journal PATH [--json]
  sworn serve --run ID --journal ABS [--manifest ABS] [--config ABS] [--operator-config ABS]
  sworn help
`

const (
	swornVersion = "1.0.0-rc.1"
	swornState   = "baton-rc11-admitted"
)

type versionInfo struct {
	Version string         `json:"version"`
	State   string         `json:"state"`
	Baton   baton.Identity `json:"baton"`
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		_, _ = io.WriteString(stdout, usage)
		return 0
	}
	switch args[0] {
	case "version":
		return runVersion(args[1:], stdout, stderr)
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
	case "status":
		return runStatus(args[1:], stdout, stderr)
	case "board":
		return runBoard(args[1:], stdout, stderr)
	case "serve":
		return runServe(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "sworn: command %q is not implemented in the v0.3 walking skeleton\n", args[0])
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
		fmt.Fprintln(stderr, "sworn answer: invalid generation")
		return 2
	}
	ctx := context.Background()
	service, factory, err := openRuntimeService(
		ctx,
		options["--journal"],
		options["--config"],
	)
	if err != nil {
		fmt.Fprintf(stderr, "sworn answer: %v\n", err)
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
		fmt.Fprintf(stderr, "sworn answer: %v\n", err)
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
		fmt.Fprintf(stderr, "sworn version: %v\n", err)
		return 1
	}
	batonIdentity, err := pkg.Identity()
	if err != nil {
		fmt.Fprintf(stderr, "sworn version: %v\n", err)
		return 1
	}
	if err := writeVersion(stdout, asJSON, batonIdentity); err != nil {
		fmt.Fprintf(stderr, "sworn version: %v\n", err)
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
		fmt.Fprintln(stderr, "sworn run: manifest is unavailable")
		return 1
	}
	if _, err := runtimepkg.ParseManifest(body); err != nil {
		fmt.Fprintf(stderr, "sworn run: %v\n", err)
		return 1
	}
	ctx := context.Background()
	service, factory, err := openRuntimeService(
		ctx,
		options["--journal"],
		options["--config"],
	)
	if err != nil {
		fmt.Fprintf(stderr, "sworn run: %v\n", err)
		return 1
	}
	defer service.Close()
	defer factory.Close()
	status, err := service.Start(ctx, body)
	if err != nil {
		fmt.Fprintf(stderr, "sworn run: %v\n", err)
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
		fmt.Fprintf(stderr, "sworn %s: invalid generation\n", kind)
		return 2
	}
	epoch := int64(0)
	if kind == journal.Retry {
		epoch, err = strconv.ParseInt(options["--epoch"], 10, 64)
		if err != nil || epoch < 1 {
			fmt.Fprintln(stderr, "sworn retry: invalid epoch")
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
		fmt.Fprintf(stderr, "sworn %s: %v\n", kind, err)
		return 1
	}
	defer service.Close()
	defer factory.Close()
	status, err := service.Control(ctx, runtimepkg.ControlCommand{
		RunID: options["--run"], ID: options["--command"], Kind: kind,
		ExpectedGeneration: generation, WorkID: options["--work"], ExpectedEpoch: epoch,
	})
	if err != nil {
		fmt.Fprintf(stderr, "sworn %s: %v\n", kind, err)
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
		fmt.Fprintf(stderr, "sworn status: %v\n", err)
		return 1
	}
	defer service.Close()
	status, err := service.Status(ctx, options["--run"])
	if err != nil {
		fmt.Fprintf(stderr, "sworn status: %v\n", err)
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
	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		fmt.Fprintln(stderr, "sworn board: git is unavailable")
		return 1
	}
	ctx := context.Background()
	journalReader, err := journal.OpenReadOnly(ctx, options["--journal"])
	if err != nil {
		fmt.Fprintln(stderr, "sworn board: journal is unavailable")
		return 1
	}
	defer journalReader.Close()
	statusReader, err := runtimepkg.OpenStatusService(ctx, options["--journal"])
	if err != nil {
		if runtimepkg.IsCode(err, "GIT_UNAVAILABLE") {
			fmt.Fprintln(stderr, "sworn board: git is unavailable")
		} else {
			fmt.Fprintln(stderr, "sworn board: journal is unavailable")
		}
		return 1
	}
	defer statusReader.Close()
	stateReader, err := cockpit.NewGitStateReader(gitExecutable)
	if err != nil {
		fmt.Fprintln(stderr, "sworn board: git is unavailable")
		return 1
	}
	projector, err := cockpit.NewProjector(
		journalReader,
		statusReader,
		stateReader,
	)
	if err != nil {
		fmt.Fprintln(stderr, "sworn board: snapshot is unavailable")
		return 1
	}
	snapshot, err := projector.Snapshot(ctx, options["--run"])
	if err != nil {
		fmt.Fprintln(stderr, "sworn board: snapshot is unavailable")
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
	_, err := fmt.Fprintf(
		out,
		"run %s\nstate %s\nplan %s\n",
		status.RunID,
		status.State,
		status.PlanDigest,
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
		"sworn %s\nstate %s\nbaton %s (%s)\n",
		info.Version,
		info.State,
		info.Baton.PackageVersion,
		info.Baton.Commit,
	)
	return err
}
