// Package router deterministically routes a slice's current state to the next
// command. It ports ~/.claude/bin/captain-route.sh into pure Go: the same
// decision tree, the same JSON .next output, no LLM. See spec S58-slice-router.
//
// Route is a pure function: all I/O flows through the injected interfaces,
// making the router table-testable with fakes.
package router

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/swornagent/sworn/internal/board"
)

// NextType enumerates the kinds of next command the router can recommend.
type NextType string

const (
	NextImplement     NextType = "implement"
	NextReview        NextType = "review"
	NextVerify        NextType = "verify"
	NextMergeTrack    NextType = "merge-track"
	NextMergeRelease  NextType = "merge-release"
	NextReplanRelease NextType = "replan-release"
	NextRedesign      NextType = "redesign"
	NextCoachDecision NextType = "coach_decision"
	NextNone          NextType = "none"
)

// Decision is the router's output — a deterministic recommendation for what
// command to run next for a slice.
type Decision struct {
	NextType    NextType `json:"type"`
	NextCommand string   `json:"command"`
	NextReason  string   `json:"reason"`
	TargetSlice string   `json:"target_slice,omitempty"`
	TargetTrack string   `json:"target_track,omitempty"`
}

// OracleReader is the state-read interface the router consumes. It hides
// git-ref resolution and track-map construction behind simple signatures.
type OracleReader interface {
	ReadSliceStatus(ctx context.Context, release, sliceID string) (board.SliceState, error)
	ReadBoard(ctx context.Context, release string) (*board.BoardState, error)
}

// ContentReader is the git-object-read interface for commit-time queries,
// file existence checks, and ancestry tests. Separate from OracleReader
// per interface segregation (Captain pin 3).
type ContentReader interface {
	LastCommitTime(ref, path string) (int64, error)
	CatFileExists(ref, path string) (bool, error)
	IsAncestor(ancestor, branch string) (bool, error)
}

// RouteInput carries the resolved parameters Route needs from the CLI layer.
type RouteInput struct {
	Release     string
	SliceID     string
	TrackID     string
	TrackBranch string // e.g. "refs/heads/track/<release>/<track>"
	ReleaseRef  string // e.g. "refs/heads/release-wt/<release>"
	DocsPrefix  string // e.g. "docs" or "apps/docs/content/docs"
}

// Route deterministically computes the next command for a slice given its
// committed state. All I/O flows through the injected readers; the function
// itself is pure and table-testable.
func Route(
	ctx context.Context,
	oracle OracleReader,
	content ContentReader,
	input RouteInput,
) (Decision, error) {
	ss, err := oracle.ReadSliceStatus(ctx, input.Release, input.SliceID)
	if err != nil {
		return Decision{}, fmt.Errorf("read slice status: %w", err)
	}

	s := string(ss.State)

	// ---------- BLOCKED precedes state ----------
	if ss.Blocked {
		reason := buildVerificationReason(ss)
		return Decision{
			NextType:    NextReplanRelease,
			NextCommand: fmt.Sprintf("/replan-release %s", input.Release),
			NextReason: fmt.Sprintf(
				"Verifier BLOCKED with reason: %s. Only the Planner can clear a BLOCKED verdict — re-running /verify-slice or /implement-slice will re-enter the same loop.",
				reason,
			),
		}, nil
	}

	switch s {
	// ---------- design_review ----------
	case "design_review":
		return routeDesignReview(ctx, content, input)

	// ---------- failed_verification ----------
	case "failed_verification":
		return routeFailedVerification(ss, input)

	// ---------- implemented ----------
	case "implemented":
		return routeImplemented(ss, input)

	// ---------- in_progress ----------
	case "in_progress":
		return Decision{
			NextType:    NextImplement,
			NextCommand: fmt.Sprintf("/implement-slice %s %s", input.SliceID, input.Release),
			NextReason:  "Slice is in_progress but no implementer is live (the loop is synchronous — it only routes between dispatches). A prior /implement-slice died mid-flight and stranded the slice. Resume /implement-slice — it picks up the in_progress slice + partial work and drives to implemented. stuck_check backstops genuine non-progress.",
		}, nil

	// ---------- planned ----------
	case "planned":
		return Decision{
			NextType:    NextImplement,
			NextCommand: fmt.Sprintf("/implement-slice %s %s", input.SliceID, input.Release),
			NextReason:  "Slice in planned. Dispatch /implement-slice — the Design TL;DR gate (Step 4) will halt for Coach review before any code lands.",
		}, nil

	// ---------- shipped ----------
	case "shipped":
		return Decision{
			NextType:   NextNone,
			NextReason: "Slice is shipped (terminal). No routing needed.",
		}, nil

	// ---------- deferred (top-level fall-through) ----------
	case "deferred":
		return Decision{
			NextType:   NextNone,
			NextReason: "Slice is deferred (terminal). No routing needed — resume via /implement-slice when un-deferred.",
		}, nil

	// ---------- verified ----------
	case "verified":
		return routeVerified(ctx, oracle, content, input)

	// ---------- unrecognised ----------
	default:
		return Decision{
			NextType:   NextNone,
			NextReason: fmt.Sprintf("Slice in unrecognised state '%s'. Inspect status.json manually.", s),
		}, nil
	}
}

// routeDesignReview handles the design_review sub-state routing by commit-time-
// newest artefact (the S06 overnight-spin guard).
func routeDesignReview(ctx context.Context, content ContentReader, input RouteInput) (Decision, error) {
	trackRef := input.TrackBranch
	if !strings.HasPrefix(trackRef, "refs/heads/") {
		trackRef = "refs/heads/" + trackRef
	}

	// approved-ack.md presence via committed-ref (CatFileExists).
	ackPath := fmt.Sprintf("%s/release/%s/%s/approved-ack.md", input.DocsPrefix, input.Release, input.SliceID)
	ackExists, err := content.CatFileExists(trackRef, ackPath)
	if err != nil {
		return Decision{}, fmt.Errorf("check approved-ack.md: %w", err)
	}

	if ackExists {
		return Decision{
			NextType:    NextImplement,
			NextCommand: fmt.Sprintf("/implement-slice %s %s", input.SliceID, input.Release),
			NextReason:  "Coach approved the ack (approved-ack.md present). Resume /implement-slice — it'll read the ack, transition to in_progress, and write code.",
		}, nil
	}

	// Commit times for design/review/decline.
	designPath := fmt.Sprintf("%s/release/%s/%s/design.md", input.DocsPrefix, input.Release, input.SliceID)
	reviewPath := fmt.Sprintf("%s/release/%s/%s/review.md", input.DocsPrefix, input.Release, input.SliceID)
	declinePath := fmt.Sprintf("%s/release/%s/%s/decline.md", input.DocsPrefix, input.Release, input.SliceID)

	ctDesign, _ := content.LastCommitTime(trackRef, designPath)
	ctReview, _ := content.LastCommitTime(trackRef, reviewPath)
	ctDecline, _ := content.LastCommitTime(trackRef, declinePath)

	// Which artefact is newest? Ties resolve design < decline < review.
	newest := "design"
	max := ctDesign
	if ctDecline > max {
		newest = "decline"
		max = ctDecline
	}
	if ctReview > max {
		newest = "review"
		max = ctReview
	}

	if newest == "review" && max > 0 {
		return Decision{
			NextType:    NextCoachDecision,
			NextCommand: fmt.Sprintf("coach ack %s  OR  coach decline %s \"<reason>\"", input.SliceID, input.SliceID),
			NextReason:  "Captain has reviewed the current design (review.md is the newest artefact). Coach must approve or decline before implementation proceeds. Read review.md, then run: coach ack <slice> (accept suggested ack), coach ack <slice> --edit (edit before accepting), or coach decline <slice> \"<reason>\" (push back to implementer).",
		}, nil
	}

	if newest == "decline" && max > 0 {
		return Decision{
			NextType:    NextImplement,
			NextCommand: fmt.Sprintf("/implement-slice %s %s", input.SliceID, input.Release),
			NextReason:  "Coach declined and the design has not yet been revised (decline.md is newer than design.md). Resume /implement-slice — it'll read the push-back, address the pins, and re-commit a revised design.md.",
		}, nil
	}

	return Decision{
		NextType:    NextReview,
		NextCommand: fmt.Sprintf("/design-review %s %s", input.SliceID, input.Release),
		NextReason:  "design.md is the newest artefact (fresh, or revised after a Coach decline) and awaits Captain review. Run /design-review in a fresh terminal — Captain produces pins + a new review.md. (A decline that has been addressed routes here, not back to implement — guards the S06 overnight-spin regression.)",
	}, nil
}

// routeFailedVerification classifies violations by gate to determine re-entry.
func routeFailedVerification(ss board.SliceState, input RouteInput) (Decision, error) {
	// Check for Gate 1/2/6 violations (design-level, need re-design).
	designLevel := false
	for _, v := range ss.Violations {
		vl := strings.ToLower(v)
		if strings.Contains(vl, "gate 1") || strings.Contains(vl, "gate 2") || strings.Contains(vl, "gate 6") {
			designLevel = true
			break
		}
	}

	if designLevel {
		return Decision{
			NextType:    NextRedesign,
			NextCommand: fmt.Sprintf("/implement-slice %s %s", input.SliceID, input.Release),
			NextReason:  "Verifier FAIL includes Gate 1/2/6 (design-level) violations. The loop will remove approved-ack.md so the Design TL;DR gate fires again — implementer rewrites design.md addressing the violations, then Captain re-reviews before code is written.",
		}, nil
	}

	return Decision{
		NextType:    NextImplement,
		NextCommand: fmt.Sprintf("/implement-slice %s %s", input.SliceID, input.Release),
		NextReason:  "Verifier FAIL on Gate 3/4/5 (mechanical). Implementer addresses violations directly in a fresh session without needing design re-review.",
	}, nil
}

// routeImplemented always routes to verify.
func routeImplemented(ss board.SliceState, input RouteInput) (Decision, error) {
	var reason string
	switch ss.VerificationResult {
	case "":
		reason = "Slice is implemented; no verifier verdict recorded. Run /verify-slice in a FRESH terminal (Rule 7)."
	case "pending":
		reason = "Slice is implemented with verification.result=pending. A prior verifier session likely crashed or was killed mid-run (stale pending). Dispatch a fresh /verify-slice — the new verdict will overwrite. If a verifier is genuinely mid-flight elsewhere, the later verdict simply wins."
	default:
		reason = fmt.Sprintf("Slice is implemented again after a prior verifier verdict (stale result=%s). Implementer addressed the violations and re-transitioned to implemented; the stale verdict no longer applies. Run /verify-slice — verifier will overwrite the stale result.", ss.VerificationResult)
	}

	return Decision{
		NextType:    NextVerify,
		NextCommand: fmt.Sprintf("/verify-slice %s %s", input.SliceID, input.Release),
		NextReason:  reason,
	}, nil
}

// routeVerified walks the track for the next non-terminal slice, or decides
// merge-track vs merge-release if the track is fully terminal.
func routeVerified(ctx context.Context, oracle OracleReader, content ContentReader, input RouteInput) (Decision, error) {
	boardState, err := oracle.ReadBoard(ctx, input.Release)
	if err != nil {
		return Decision{}, fmt.Errorf("read board: %w", err)
	}

	// Find this track.
	var thisTrack *board.TrackState
	for i := range boardState.Tracks {
		if boardState.Tracks[i].ID == input.TrackID {
			thisTrack = &boardState.Tracks[i]
			break
		}
	}
	if thisTrack == nil {
		return Decision{}, fmt.Errorf("track %s not found in board", input.TrackID)
	}

	// Walk track slices forward from the current slice.
	passedSelf := false
	for _, s := range thisTrack.Slices {
		if !passedSelf {
			if s.ID == input.SliceID {
				passedSelf = true
			}
			continue
		}

		// Ghost-slice filter: only consider slices owned by this track.
		if s.Track != input.TrackID {
			continue
		}

		// Terminal states: skip (verified, shipped, deferred).
		terminal := false
		switch string(s.State) {
		case "verified", "shipped", "deferred":
			terminal = true
		}

		if !terminal {
			return routeNextSlice(s, input)
		}
	}

	// No more non-terminal slices in this track. Decide merge-track vs merge-release.
	return routeMergeDecision(boardState, content, input)
}

// routeNextSlice returns the right command for the next non-terminal sibling.
func routeNextSlice(s board.SliceState, input RouteInput) (Decision, error) {
	ss := string(s.State)
	switch ss {
	case "planned":
		return Decision{
			NextType:    NextImplement,
			NextCommand: fmt.Sprintf("/implement-slice %s %s", s.ID, input.Release),
			NextReason:  fmt.Sprintf("%s is verified. Next planned slice in track (%s) is %s.", input.SliceID, input.TrackID, s.ID),
			TargetSlice: s.ID,
			TargetTrack: input.TrackID,
		}, nil

	case "design_review":
		return Decision{
			NextType:    NextReview,
			NextCommand: fmt.Sprintf("/design-review %s %s", s.ID, input.Release),
			NextReason:  fmt.Sprintf("%s is verified. Next slice in track (%s) is %s — it already has a design.md; review it before code is written.", input.SliceID, input.TrackID, s.ID),
			TargetSlice: s.ID,
			TargetTrack: input.TrackID,
		}, nil

	case "in_progress", "implemented", "failed_verification":
		return Decision{
			NextType:    NextImplement,
			NextCommand: fmt.Sprintf("/implement-slice %s %s", s.ID, input.Release),
			NextReason:  fmt.Sprintf("%s is verified. Next slice in track (%s) is %s (currently %s) — continue where it left off.", input.SliceID, input.TrackID, s.ID, ss),
			TargetSlice: s.ID,
			TargetTrack: input.TrackID,
		}, nil

	default:
		return Decision{
			NextType:    NextImplement,
			NextCommand: fmt.Sprintf("/implement-slice %s %s", s.ID, input.Release),
			NextReason:  fmt.Sprintf("%s is verified. Next slice in track (%s) is %s (state=%s).", input.SliceID, input.TrackID, s.ID, ss),
			TargetSlice: s.ID,
			TargetTrack: input.TrackID,
		}, nil
	}
}

// routeMergeDecision decides between merge-track and merge-release when a
// track has no more non-terminal slices.
func routeMergeDecision(
	boardState *board.BoardState,
	content ContentReader,
	input RouteInput,
) (Decision, error) {
	// Count undone (non-terminal) slices across the whole release.
	undone := 0
	for _, t := range boardState.Tracks {
		for _, s := range t.Slices {
			switch string(s.State) {
			case "verified", "shipped", "deferred":
				// terminal
			default:
				undone++
			}
		}
	}

	if undone > 0 {
		return Decision{
			NextType:    NextMergeTrack,
			NextCommand: fmt.Sprintf("/merge-track %s %s", input.TrackID, input.Release),
			NextReason:  fmt.Sprintf("%s is verified — track %s has no more non-terminal slices. Merge this track now; %d slice(s) remain across other tracks.", input.SliceID, input.TrackID, undone),
			TargetTrack: input.TrackID,
		}, nil
	}

	// All slices terminal. Check if any track hasn't been merged into release-wt.
	releaseRef := input.ReleaseRef
	if !strings.HasPrefix(releaseRef, "refs/heads/") {
		releaseRef = "refs/heads/" + releaseRef
	}

	unmergedTrack := findFirstUnmergedTrack(content, boardState, releaseRef)
	if unmergedTrack != "" {
		return Decision{
			NextType:    NextMergeTrack,
			NextCommand: fmt.Sprintf("/merge-track %s %s", unmergedTrack, input.Release),
			NextReason:  fmt.Sprintf("All owned slices verified across %s, but track %s has not yet been merged into release-wt (%s). Merge it before /merge-release — otherwise the release branch is missing that track's commits.", input.Release, unmergedTrack, releaseRef),
			TargetTrack: unmergedTrack,
		}, nil
	}

	return Decision{
		NextType:    NextMergeRelease,
		NextCommand: fmt.Sprintf("/merge-release %s", input.Release),
		NextReason:  fmt.Sprintf("%s is verified — every slice across %s is in a terminal state and every track has been merged into release-wt (%s). Time to merge the release.", input.SliceID, input.Release, releaseRef),
	}, nil
}

// findFirstUnmergedTrack returns the ID of the first track whose branch is not
// an ancestor of releaseRef, or "" if all tracks are merged.
func findFirstUnmergedTrack(content ContentReader, boardState *board.BoardState, releaseRef string) string {
	// Sort tracks by ID for deterministic output.
	ids := make([]string, 0, len(boardState.Tracks))
	for _, t := range boardState.Tracks {
		ids = append(ids, t.ID)
	}
	sort.Strings(ids)

	for _, tid := range ids {
		var wb string
		for _, t := range boardState.Tracks {
			if t.ID == tid {
				wb = t.WorktreeBranch
				break
			}
		}
		if wb == "" {
			continue
		}
		// Try local ref, then origin/ fallback.
		ancestor, err := content.IsAncestor(wb, releaseRef)
		if err != nil {
			originRef := "origin/" + wb
			ancestor, err = content.IsAncestor(originRef, releaseRef)
			if err != nil {
				continue
			}
		}
		if !ancestor {
			return tid
		}
	}
	return ""
}

// buildVerificationReason builds the reason string for a BLOCKED verdict.
// Uses violations joined by "; " (the .reason field was a bash-only jq
// fallback that was never populated in practice — no status.json has it).
func buildVerificationReason(ss board.SliceState) string {
	if len(ss.Violations) == 0 {
		return "no violations recorded"
	}
	return strings.Join(ss.Violations, "; ")
}