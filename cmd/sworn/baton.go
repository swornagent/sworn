package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/command"
)

func init() {
	// T14-baton-integration owns the baton verb.
	// Forward handoff to S50: S50-baton-governance adds `sworn baton diff` to
	// this same file (cmd/sworn/baton.go). S50 depends_on S48; sequencing is safe.
	command.Register(command.Command{
		Name:    "baton",
		Summary: "vendor and manage the embedded Baton protocol",
		Run:     cmdBaton,
	})
}

// cmdBaton dispatches the "sworn baton" command tree.
func cmdBaton(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, `sworn baton — manage the embedded Baton protocol

usage:
  sworn baton vendor <source-dir> [--check]   vendor the embedded Baton protocol from a checkout
  sworn baton vendor --upstream [--tag vX.Y.Z] [--repo owner/name] [--check]
                                              vendor from the public Baton repo over HTTPS
  sworn baton diff <source-dir>               compare the committed embed against the pinned source

See 'sworn baton vendor --help' or 'sworn baton diff --help' for details.
`)
		return 64
	}

	switch args[0] {
	case "vendor":
		return cmdBatonVendor(args[1:])
	case "diff":
		return cmdBatonDiff(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown baton subcommand %q\n\n", args[0])
		fmt.Fprint(os.Stderr, "usage: sworn baton vendor <source-dir> [--check]\n")
		fmt.Fprint(os.Stderr, "usage: sworn baton vendor --upstream [--tag vX.Y.Z] [--repo owner/name] [--check]\n")
		fmt.Fprint(os.Stderr, "usage: sworn baton diff <source-dir>\n")
		return 64
	}
}

// cmdBatonVendor implements `sworn baton vendor [<source-dir>] [--upstream] [--tag ...] [--repo ...] [--check]`.
//
// With --upstream, the binary fetches the pinned Baton release tarball from the
// public GitHub repo over HTTPS, verifies the resolved commit SHA and content
// digest against the VERSION pin, extracts the tarball into a temp directory,
// feeds it through the existing transform pipeline, and writes the updated pin
// back to VERSION. Without --upstream, behaviour is unchanged from S48 (local
// directory vendor).
//
// With --check, it prints the transform diff without writing any files (dry-run
// mode). In upstream + dry-run mode, no pin is written even on success.
func cmdBatonVendor(args []string) int {
	parsed, err := parseBatonVendorArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "baton vendor: %v\n", err)
		printBatonVendorUsage(os.Stderr)
		return 2
	}
	if parsed.help {
		printBatonVendorUsage(os.Stderr)
		return 2
	}

	if !parsed.upstream {
		// Local vendor path (S48 back-compat).
		if len(parsed.positionals) != 1 {
			fmt.Fprintln(os.Stderr, "baton vendor: local mode requires exactly one source-dir")
			printBatonVendorUsage(os.Stderr)
			return 2
		}

		sourceDir := parsed.positionals[0]
		repoRoot, err := findRepoRoot()
		if err != nil {
			fmt.Fprintf(os.Stderr, "baton vendor: %v\n", err)
			return 2
		}

		opts := baton.VendorOpts{
			SourceDir: sourceDir,
			RepoRoot:  repoRoot,
			CheckOnly: parsed.check,
		}
		if inputs, pinned, err := baton.PinnedInstallerVendorInputs(context.Background(), sourceDir, time.Now().UTC()); err != nil {
			fmt.Fprintf(os.Stderr, "baton vendor: %v\n", err)
			return 2
		} else if pinned {
			opts.VersionCandidate = &inputs.Version
			opts.InstallerArchiveCandidate = inputs.Archive
		}

		result, err := baton.Vendor(opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "baton vendor: %v\n", err)
			return 2
		}

		printVendorResult(result, parsed.check, sourceDir)
		if parsed.check && result.Diff != "" {
			return 1
		}
		return 0
	}

	if len(parsed.positionals) != 0 {
		fmt.Fprintln(os.Stderr, "baton vendor: upstream mode does not accept a source-dir")
		printBatonVendorUsage(os.Stderr)
		return 2
	}

	// --upstream path: recover an interrupted local transaction before doing
	// any network work, then fetch from the public Baton repo.
	repoRoot, err := findRepoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "baton vendor: %v\n", err)
		return 2
	}
	if !parsed.check {
		if err := baton.RecoverVendorIfPending(repoRoot); err != nil {
			fmt.Fprintf(os.Stderr, "baton vendor: %v\n", err)
			return 2
		}
	}

	repo := parsed.repo
	if repo == "" {
		if envRepo := os.Getenv("SWORN_BATON_REPO"); envRepo != "" {
			repo = envRepo
		} else {
			repo = "sawy3r/baton"
		}
	}

	tag := parsed.tag
	if tag == "" {
		tag = baton.Version()
		if tag == "" {
			fmt.Fprintf(os.Stderr, "baton vendor: no tag specified and no baton-protocol pin in VERSION — use --tag vX.Y.Z\n")
			return 2
		}
	}

	ctx := context.Background()
	result, err := baton.FetchUpstream(ctx, repo, tag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "baton vendor: %v\n", err)
		return 2
	}
	defer result.Cleanup()
	invocationInstant := time.Now().UTC()

	opts := baton.VendorOpts{
		SourceDir: result.SourceDir,
		RepoRoot:  repoRoot,
		CheckOnly: parsed.check,
		VersionCandidate: &baton.UpstreamVersionCandidate{
			Tag:        tag,
			SHA:        result.SHA,
			Digest:     result.Digest,
			CapturedAt: invocationInstant,
		},
	}

	vendorResult, err := baton.Vendor(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "baton vendor: %v\n", err)
		return 2
	}

	printVendorResult(vendorResult, parsed.check, fmt.Sprintf("%s @ %s", repo, tag))
	if parsed.check && vendorResult.Diff != "" {
		return 1
	}
	return 0
}

type batonVendorArgs struct {
	check       bool
	upstream    bool
	tag         string
	repo        string
	help        bool
	positionals []string
}

// parseBatonVendorArgs accepts vendor flags before or after the local source
// operand. The standard flag package stops at the first positional argument,
// which made `vendor SOURCE --check` silently perform a write.
func parseBatonVendorArgs(args []string) (batonVendorArgs, error) {
	var parsed batonVendorArgs
	flagsEnabled := true
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !flagsEnabled || arg == "" || arg[0] != '-' || arg == "-" {
			parsed.positionals = append(parsed.positionals, arg)
			continue
		}
		if arg == "--" {
			flagsEnabled = false
			continue
		}
		if strings.HasPrefix(arg, "---") {
			return batonVendorArgs{}, fmt.Errorf("unknown flag %q", arg)
		}

		name, value, hasValue := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		switch name {
		case "h", "help":
			if hasValue {
				return batonVendorArgs{}, fmt.Errorf("flag --%s does not accept a value", name)
			}
			parsed.help = true
		case "check":
			if !hasValue {
				parsed.check = true
				continue
			}
			boolValue, err := strconv.ParseBool(value)
			if err != nil {
				return batonVendorArgs{}, fmt.Errorf("invalid value %q for --check", value)
			}
			parsed.check = boolValue
		case "upstream":
			if !hasValue {
				parsed.upstream = true
				continue
			}
			boolValue, err := strconv.ParseBool(value)
			if err != nil {
				return batonVendorArgs{}, fmt.Errorf("invalid value %q for --upstream", value)
			}
			parsed.upstream = boolValue
		case "tag", "repo":
			if !hasValue {
				i++
				if i >= len(args) || strings.HasPrefix(args[i], "-") {
					return batonVendorArgs{}, fmt.Errorf("flag --%s requires a value", name)
				}
				value = args[i]
			}
			if value == "" {
				return batonVendorArgs{}, fmt.Errorf("flag --%s requires a non-empty value", name)
			}
			if name == "tag" {
				parsed.tag = value
			} else {
				parsed.repo = value
			}
		default:
			return batonVendorArgs{}, fmt.Errorf("unknown flag %q", arg)
		}
	}
	return parsed, nil
}

func printBatonVendorUsage(w *os.File) {
	fmt.Fprintln(w, "usage: sworn baton vendor <source-dir> [--check]")
	fmt.Fprintln(w, "       sworn baton vendor --upstream [--tag vX.Y.Z] [--repo owner/name] [--check]")
	fmt.Fprintln(w, "  source-dir  path to a Baton checkout (e.g. ~/projects/baton)")
	fmt.Fprintln(w, "  --check     dry-run: print the transform diff without writing")
}

// printVendorResult prints the VendorResult in the standard format.
func printVendorResult(result *baton.VendorResult, check bool, source string) {
	if check {
		if result.Diff == "" {
			fmt.Println("No changes — the embed matches the vendored source.")
		} else {
			fmt.Print(result.Diff)
		}
	} else {
		if result.FilesWritten == 0 {
			fmt.Println("No changes — the embed already matches the vendored source.")
		} else {
			fmt.Printf("Vendored %d files from %s\n", result.FilesWritten, source)
			if result.Diff != "" {
				fmt.Print(result.Diff)
			}
		}
	}
}

// findRepoRoot walks up from the current directory to find the .git directory.
func findRepoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("cannot determine current directory: %w", err)
	}
	for dir := wd; ; {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// Hit filesystem root without finding .git.
			return wd, nil
		}
		dir = parent
	}
}

// cmdBatonDiff implements `sworn baton diff <source-dir>`.
//
// It compares the committed embed against the transformed pinned source and
// exits 0 when in sync, non-zero when divergent. This is the governance /
// fail-closed surface: it detects silent forks of the embedded protocol.
//
// Unlike `sworn baton vendor --check` (a developer dry-run that shows the
// transform diff without committing), `diff` is the gate that answers "has
// the embed been edited out-of-band?" and is designed for CI and pre-commit
// hooks.
func cmdBatonDiff(args []string) int {
	fs := flag.NewFlagSet("baton diff", flag.ExitOnError)
	_ = fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "usage: sworn baton diff <source-dir>\n")
		fmt.Fprintf(os.Stderr, "  source-dir  path to a Baton checkout (e.g. ~/projects/baton)\n")
		fmt.Fprintf(os.Stderr, "\n")
		fmt.Fprintf(os.Stderr, "Compares the committed embed against the transformed pinned source.\n")
		fmt.Fprintf(os.Stderr, "Exits 0 when in sync; non-zero and prints each divergent file when\n")
		fmt.Fprintf(os.Stderr, "the embed has been edited out-of-band (a silent fork).\n")
		fmt.Fprintf(os.Stderr, "\n")
		fmt.Fprintf(os.Stderr, "Tip: use 'sworn baton vendor --check' for a developer dry-run that\n")
		fmt.Fprintf(os.Stderr, "shows the full transform diff. 'diff' is the fail-closed governance\n")
		fmt.Fprintf(os.Stderr, "gate — does the embed match the pinned source?\n")
		return 64
	}

	sourceDir := fs.Arg(0)

	repoRoot, err := findRepoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "baton diff: %v\n", err)
		return 1
	}

	divs, err := baton.Diff(baton.DiffOpts{
		SourceDir: sourceDir,
		RepoRoot:  repoRoot,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "baton diff: %v\n", err)
		return 1
	}

	if len(divs) == 0 {
		fmt.Println("In sync — embedded protocol matches pinned source.")
		return 0
	}

	for _, d := range divs {
		fmt.Printf("%s: %s\n", d.File, d.Reason)
	}
	return 1
}
