package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

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
		fmt.Fprint(os.Stderr, "usage: sworn baton diff <source-dir>\n")
		return 64
	}
}

// cmdBatonVendor implements `sworn baton vendor <source-dir> [--check]`.
//
// It reads a Baton checkout from source-dir, applies the script→sworn command
// transform, and writes the result into the binary's go:embed trees
// (internal/adopt/baton/ and internal/prompt/).
//
// With --check, it prints the transform diff without writing any files.
func cmdBatonVendor(args []string) int {
	fs := flag.NewFlagSet("baton vendor", flag.ExitOnError)
	check := fs.Bool("check", false, "print the transform diff without writing files")
	_ = fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "usage: sworn baton vendor <source-dir> [--check]\n")
		fmt.Fprintf(os.Stderr, "  source-dir  path to a Baton checkout (e.g. ~/projects/baton)\n")
		fmt.Fprintf(os.Stderr, "  --check     dry-run: print the transform diff without writing\n")
		return 64
	}

	sourceDir := fs.Arg(0)

	// Resolve RepoRoot: find the git repo root from the current directory.
	// Fall back to CWD if we can't determine it.
	repoRoot, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "baton vendor: cannot determine current directory: %v\n", err)
		return 1
	}
	// Walk up to find the .git directory. We're in cmd/sworn/baton.go so
	// the repo root is three levels up from the binary's CWD.
	for {
		if _, err := os.Stat(filepath.Join(repoRoot, ".git")); err == nil {
			break
		}
		parent := filepath.Dir(repoRoot)
		if parent == repoRoot {
			// Hit filesystem root without finding .git — use CWD.
			break
		}
		repoRoot = parent
	}

	opts := baton.VendorOpts{
		SourceDir: sourceDir,
		RepoRoot:  repoRoot,
		CheckOnly: *check,
	}

	result, err := baton.Vendor(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "baton vendor: %v\n", err)
		return 1
	}

	if *check {
		if result.Diff == "" {
			fmt.Println("No changes — the embed matches the vendored source.")
		} else {
			fmt.Print(result.Diff)
		}
	} else {
		if result.FilesWritten == 0 {
			fmt.Println("No changes — the embed already matches the vendored source.")
		} else {
			fmt.Printf("Vendored %d files from %s\n", result.FilesWritten, sourceDir)
			if result.Diff != "" {
				fmt.Print(result.Diff)
			}
		}
	}

	return 0
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

	// RepoRoot discovery.
	repoRoot, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "baton diff: cannot determine current directory: %v\n", err)
		return 1
	}
	for {
		if _, err := os.Stat(filepath.Join(repoRoot, ".git")); err == nil {
			break
		}
		parent := filepath.Dir(repoRoot)
		if parent == repoRoot {
			break
		}
		repoRoot = parent
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