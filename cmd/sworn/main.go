package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/swornagent/sworn/internal/baton"
	runtimepkg "github.com/swornagent/sworn/internal/runtime"
)

const usage = `Sworn runs autonomous delivery with the Baton protocol.

Available in the v0.3 walking skeleton:
  sworn version [--json]
  sworn run --manifest PATH --journal PATH
  sworn resume --run ID --journal PATH
  sworn status --run ID --journal PATH --json
  sworn help

Pause, retry, takeover, multi-track scheduling and the board arrive later.
`

const (
	swornVersion = "0.3.0-dev"
	swornState   = "baton-rc5-admitted"
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
	case "resume":
		return runResume(args[1:], stdout, stderr)
	case "status":
		return runStatus(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "sworn: command %q is not implemented in the v0.3 walking skeleton\n", args[0])
		return 2
	}
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
	options, ok := parseOptions(args, []string{"--manifest", "--journal"}, nil)
	if !ok {
		fmt.Fprintln(stderr, "usage: sworn run --manifest PATH --journal PATH")
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
	service, err := runtimepkg.OpenService(ctx, options["--journal"])
	if err != nil {
		fmt.Fprintf(stderr, "sworn run: %v\n", err)
		return 1
	}
	defer service.Close()
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

func runResume(args []string, stdout, stderr io.Writer) int {
	options, ok := parseOptions(args, []string{"--run", "--journal"}, nil)
	if !ok {
		fmt.Fprintln(stderr, "usage: sworn resume --run ID --journal PATH")
		return 2
	}
	ctx := context.Background()
	service, err := runtimepkg.OpenService(ctx, options["--journal"])
	if err != nil {
		fmt.Fprintf(stderr, "sworn resume: %v\n", err)
		return 1
	}
	defer service.Close()
	status, err := service.Resume(ctx, options["--run"])
	if err != nil {
		fmt.Fprintf(stderr, "sworn resume: %v\n", err)
		return 1
	}
	if err := writeStatusText(stdout, status); err != nil {
		fmt.Fprintln(stderr, "sworn resume: output failed")
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

func parseOptions(
	args []string,
	values []string,
	switches []string,
) (map[string]string, bool) {
	allowedValues := make(map[string]bool, len(values))
	for _, name := range values {
		allowedValues[name] = true
	}
	allowedSwitches := make(map[string]bool, len(switches))
	for _, name := range switches {
		allowedSwitches[name] = true
	}
	result := make(map[string]string, len(values)+len(switches))
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
		if args[index] == "" {
			return nil, false
		}
		result[name] = args[index]
	}
	for _, name := range values {
		if result[name] == "" {
			return nil, false
		}
	}
	for _, name := range switches {
		if result[name] != "true" {
			return nil, false
		}
	}
	return result, true
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
