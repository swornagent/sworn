package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/swornagent/sworn/internal/board"
	"github.com/swornagent/sworn/internal/command"
	"github.com/swornagent/sworn/internal/git"
	"github.com/swornagent/sworn/internal/router"
	"github.com/swornagent/sworn/internal/style"
)

func init() {
	command.Register(command.Command{
		Name:    "route",
		Summary: "compute the next command for a slice (deterministic, no LLM)",
		Run:     cmdRoute,
	})
}

func cmdRoute(args []string) int {
	fs := flag.NewFlagSet("route", flag.ExitOnError)
	pretty := fs.Bool("pretty", false, "pretty-print with colours")
	_ = fs.Parse(args)

	// Positional args: <slice-id> <release-name>
	pos := fs.Args()
	if len(pos) < 2 {
		fmt.Fprintln(os.Stderr, "sworn route: <slice-id> and <release-name> required")
		fmt.Fprintln(os.Stderr, "usage: sworn route <slice-id> <release-name> [--pretty]")
		return 64
	}
	sliceID := pos[0]
	releaseName := pos[1]

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn route: get cwd: %v\n", err)
		return 2
	}

	repo := git.New(cwd)

	// Resolve the release-wt ref.
	releaseRef := "refs/heads/release-wt/" + releaseName
	exists, err := repo.CatFileExists(releaseRef, "docs/release/"+releaseName+"/index.md")
	if err != nil || !exists {
		exists2, err2 := repo.CatFileExists(releaseRef, "apps/docs/content/docs/release/"+releaseName+"/index.md")
		if err2 != nil || !exists2 {
			releaseRef = "HEAD"
		}
	}

	oracle := board.NewGitOracle(repo)
	reader := gitContentReaderAdapter{repo: repo}

	// Build the OracleReader adapter.
	oracleAdapter, err := board.NewOracleReaderAdapter(oracle, reader, releaseName, releaseRef)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn route: oracle adapter: %v\n", err)
		return 2
	}

	// Resolve track info for the slice.
	ctx := context.Background()
	fullBoard, err := oracleAdapter.ReadBoard(ctx, releaseName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn route: read board: %v\n", err)
		return 2
	}

	// Find the track that owns this slice.
	var trackID, trackBranch string
	for _, t := range fullBoard.Tracks {
		for _, s := range t.Slices {
			if s.ID == sliceID {
				trackID = t.ID
				trackBranch = t.WorktreeBranch
				break
			}
		}
		if trackID != "" {
			break
		}
	}
	if trackID == "" {
		fmt.Fprintf(os.Stderr, "sworn route: slice %s not found in any track of release %s\n", sliceID, releaseName)
		return 1
	}

	if trackBranch == "" {
		trackBranch = "track/" + releaseName + "/" + trackID
	}

	// Resolve docs prefix.
	docsPrefix := resolveDocsPrefix(repo, trackBranch, releaseName, sliceID)

	// Build content reader.
	contentReader := &repoContentReader{repo: repo}

	// Route.
	input := router.RouteInput{
		Release:     releaseName,
		SliceID:     sliceID,
		TrackID:     trackID,
		TrackBranch: "refs/heads/" + trackBranch,
		ReleaseRef:  releaseRef,
		DocsPrefix:  docsPrefix,
	}

	decision, err := router.Route(ctx, oracleAdapter, contentReader, input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn route: %v\n", err)
		return 2
	}

	// Read the slice state for context.
	ss, _ := oracleAdapter.ReadSliceStatus(ctx, releaseName, sliceID)

	if *pretty {
		return printRoutePretty(sliceID, releaseName, trackID, ss, decision)
	}
	return printRouteJSON(sliceID, releaseName, trackID, ss, decision)
}

// resolveDocsPrefix probes the two possible docs prefixes against the track
// branch to determine which one holds the release artefacts.
func resolveDocsPrefix(repo *git.Repo, trackBranch, releaseName, sliceID string) string {
	for _, prefix := range []string{"docs", "apps/docs/content/docs"} {
		ref := "refs/heads/" + trackBranch
		path := prefix + "/release/" + releaseName + "/" + sliceID + "/status.json"
		exists, err := repo.CatFileExists(ref, path)
		if err == nil && exists {
			return prefix
		}
	}
	return "docs" // default
}

// gitContentReaderAdapter adapts *git.Repo to board.gitContentReader.
type gitContentReaderAdapter struct {
	repo *git.Repo
}

func (a gitContentReaderAdapter) Show(ref, path string) (string, error) {
	return a.repo.Show(ref, path)
}

func (a gitContentReaderAdapter) CatFileExists(ref, path string) (bool, error) {
	return a.repo.CatFileExists(ref, path)
}

// repoContentReader adapts *git.Repo to router.ContentReader.
type repoContentReader struct {
	repo *git.Repo
}

func (r *repoContentReader) LastCommitTime(ref, path string) (int64, error) {
	return r.repo.LastCommitTime(ref, path)
}

func (r *repoContentReader) CatFileExists(ref, path string) (bool, error) {
	return r.repo.CatFileExists(ref, path)
}

func (r *repoContentReader) IsAncestor(ancestor, branch string) (bool, error) {
	return r.repo.IsAncestor(ancestor, branch)
}

// ---------- JSON output ----------

type routeOutput struct {
	Version     string        `json:"version"`
	GeneratedAt string        `json:"generated_at"`
	Slice       routeSlice    `json:"slice"`
	Next        routeNext     `json:"next"`
}

type routeSlice struct {
	ID           string             `json:"id"`
	Release      string             `json:"release"`
	TrackID      string             `json:"track_id"`
	State        string             `json:"state"`
	Verification routeVerification  `json:"verification"`
}

type routeVerification struct {
	Result                *string  `json:"result"`
	Reason                *string  `json:"reason"`
	Violations            []string `json:"violations"`
	VerifierWasFreshContext bool    `json:"verifier_was_fresh_context"`
}

type routeNext struct {
	Type        string  `json:"type"`
	Command     *string `json:"command"`
	Reason      string  `json:"reason"`
	TargetSlice *string `json:"target_slice"`
	TargetTrack *string `json:"target_track"`
}

func printRouteJSON(sliceID, release, trackID string, ss board.SliceState, decision router.Decision) int {
	generatedAt := time.Now().UTC().Format(time.RFC3339)

	var verifResult *string
	if ss.VerificationResult != "" {
		v := ss.VerificationResult
		verifResult = &v
	}

	var reason *string
	reasonStr := buildVerificationReason(ss.Violations)
	if reasonStr != "" {
		reason = &reasonStr
	}

	var nextCmd *string
	if decision.NextCommand != "" {
		nextCmd = &decision.NextCommand
	}

	var targetSlice *string
	if decision.TargetSlice != "" {
		targetSlice = &decision.TargetSlice
	}

	var targetTrack *string
	if decision.TargetTrack != "" {
		targetTrack = &decision.TargetTrack
	}

	out := routeOutput{
		Version:     "0.1",
		GeneratedAt: generatedAt,
		Slice: routeSlice{
			ID:      sliceID,
			Release: release,
			TrackID: trackID,
			State:   string(ss.State),
			Verification: routeVerification{
				Result:                  verifResult,
				Reason:                  reason,
				Violations:              ss.Violations,
				VerifierWasFreshContext: false,
			},
		},
		Next: routeNext{
			Type:        string(decision.NextType),
			Command:     nextCmd,
			Reason:      decision.NextReason,
			TargetSlice: targetSlice,
			TargetTrack: targetTrack,
		},
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(out); err != nil {
		fmt.Fprintf(os.Stderr, "sworn route: json: %v\n", err)
		return 2
	}
	return 0
}

func buildVerificationReason(violations []string) string {
	if len(violations) == 0 {
		return ""
	}
	return joinViolations(violations)
}

func joinViolations(violations []string) string {
	result := ""
	for i, v := range violations {
		if i > 0 {
			result += "; "
		}
		result += v
	}
	return result
}

// ---------- pretty output ----------

func printRoutePretty(sliceID, release, trackID string, ss board.SliceState, decision router.Decision) int {
	fmt.Println()
	fmt.Println(style.Bold(sliceID))
	fmt.Printf("  %s\n", style.Dim("in "+trackID+"  ·  release "+release))
	fmt.Println()

	// State line.
	stateColoured := string(ss.State)
	switch string(ss.State) {
	case "verified":
		stateColoured = style.Accent(stateColoured)
	case "failed_verification":
		stateColoured = style.Danger(stateColoured)
	case "shipped":
		stateColoured = style.Accent(stateColoured)
	case "implemented":
		stateColoured = style.Dim(stateColoured)
	case "in_progress":
		stateColoured = style.Warn(stateColoured)
	case "planned":
		stateColoured = style.Dim(stateColoured)	}
	fmt.Printf("State:      %s\n", stateColoured)

	if ss.VerificationResult != "" {
		verdictColoured := ss.VerificationResult
		switch ss.VerificationResult {
		case "pass":
			verdictColoured = style.Accent("PASS")
		case "fail":
			verdictColoured = style.Danger("FAIL")
		case "blocked":
			verdictColoured = style.Warn("BLOCKED")		}
		fmt.Printf("Last verdict: %s\n", verdictColoured)

		reason := buildVerificationReason(ss.Violations)
		if reason != "" {
			fmt.Printf("  %s\n", style.Dim(reason))		}
		if len(ss.Violations) > 0 {
			fmt.Println("  Violations:")
			for _, v := range ss.Violations {
				fmt.Printf("    - %s\n", v)
			}
		}
	}
	fmt.Println()

	if decision.NextType == "none" || decision.NextCommand == "" {
		fmt.Printf("%s %s\n", style.Dim("Next:"), style.Bold(style.Dim("no command — see reason below")))	} else {
		fmt.Printf("%s %s\n", style.Bold("Next:"), style.Accent(decision.NextCommand))
	}
	fmt.Println()
	fmt.Printf("  %s\n", decision.NextReason)
	fmt.Println()

	return 0
}