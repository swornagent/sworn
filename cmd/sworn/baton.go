package main

import (
	"context"
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
	fs := flag.NewFlagSet("baton vendor", flag.ExitOnError)
	check := fs.Bool("check", false, "print the transform diff without writing files")
	upstream := fs.Bool("upstream", false, "fetch from the public Baton repo over HTTPS")
	tagFlag := fs.String("tag", "", "semver tag to fetch (default: pinned tag from VERSION)")
	repoFlag := fs.String("repo", "", "GitHub repo as owner/name (default: github.com/sawy3r/baton)")
	_ = fs.Parse(args)

	if !*upstream {
		// Local vendor path (S48 back-compat).
		if fs.NArg() < 1 {
			fmt.Fprintf(os.Stderr, "usage: sworn baton vendor <source-dir> [--check]\n")
			fmt.Fprintf(os.Stderr, "  source-dir  path to a Baton checkout (e.g. ~/projects/baton)\n")
			fmt.Fprintf(os.Stderr, "  --check     dry-run: print the transform diff without writing\n")
			return 64
		}

		sourceDir := fs.Arg(0)
		repoRoot, err := findRepoRoot()
		if err != nil {
			fmt.Fprintf(os.Stderr, "baton vendor: %v\n", err)
			return 1
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

		printVendorResult(result, *check, sourceDir)
		return 0
	}

	// --upstream path: fetch from the public Baton repo.
	repo := *repoFlag
	if repo == "" {
		if envRepo := os.Getenv("SWORN_BATON_REPO"); envRepo != "" {
			repo = envRepo
		} else {
			repo = "sawy3r/baton"
		}
	}

	tag := *tagFlag
	if tag == "" {
		tag = baton.Version()
		if tag == "" {
			fmt.Fprintf(os.Stderr, "baton vendor: no tag specified and no baton-protocol pin in VERSION — use --tag vX.Y.Z\n")
			return 64
		}
	}

	ctx := context.Background()
	result, err := baton.FetchUpstream(ctx, repo, tag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "baton vendor: %v\n", err)
		return 1
	}
	defer result.Cleanup()

	repoRoot, err := findRepoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "baton vendor: %v\n", err)
		return 1
	}

	opts := baton.VendorOpts{
		SourceDir: result.SourceDir,
		RepoRoot:  repoRoot,
		CheckOnly: *check,
	}

	vendorResult, err := baton.Vendor(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "baton vendor: %v\n", err)
		return 1
	}

	printVendorResult(vendorResult, *check, fmt.Sprintf("%s @ %s", repo, tag))

	// Write the upstream pin only after a successful Vendor, and only when
	// not in dry-run mode (CheckOnly). This ensures a failed run doesn't
	// leave a stale pin.
	if !*check {
		if err := baton.WriteUpstreamPin(repoRoot, result.SHA, result.Digest); err != nil {
			fmt.Fprintf(os.Stderr, "baton vendor: write pin: %v\n", err)
			return 1
		}
		fmt.Printf("Upstream pin recorded: sha=%s digest=%s\n", result.SHA, result.Digest)
	}

	return 0
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
