package cockpit

import (
	"strings"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/journal"
	runtimepkg "github.com/swornagent/sworn/internal/runtime"
)

// ReleaseSnapshot is the Baton view shown before a Sworn run exists.
type ReleaseSnapshot struct {
	Graph        Graph
	Diagnostics  []Diagnostic
	Presentation RunPresentation
}

// DiscoveredRunStatus holds the status and attention state of one discovered run.
type DiscoveredRunStatus struct {
	Binding    journal.Run
	Status     runtimepkg.RunStatus
	Attentions []journal.AttentionProjection
}

// ProjectReleaseInfo holds release metadata for catalog projection.
type ProjectReleaseInfo struct {
	Name       string
	SourceRef  string
	State      string
	Status     string
	Diagnostic string
}

// ReleaseStateAndStatus returns the lifecycle state and presentation status for a Baton release.
func ReleaseStateAndStatus(state baton.State) (string, string) {
	snapshot := ProjectRelease(state)
	if len(snapshot.Diagnostics) > 0 {
		return "needs_confirmation", snapshot.Presentation.Status
	}
	if state.Assembly.Status == "complete" && state.Assembly.Outcome == "merged" {
		return "complete", snapshot.Presentation.Status
	}
	if state.Assembly.Status == "blocked" {
		return "blocked", snapshot.Presentation.Status
	}
	handoff := projectHandoff(snapshot.Graph)
	if handoff.Ready {
		return "ready", snapshot.Presentation.Status
	}
	return "not_started", snapshot.Presentation.Status
}

// BuildProjectCatalog builds a ProjectCatalog from release info, discovered runs, and diagnostics.
func BuildProjectCatalog(
	releases []ProjectReleaseInfo,
	runs []DiscoveredRunStatus,
	diagnostics []Diagnostic,
) ProjectCatalog {
	catalogReleases := make([]CatalogRelease, 0, len(releases))
	for _, rel := range releases {
		catalogReleases = append(catalogReleases, CatalogRelease{
			Name:       rel.Name,
			SourceRef:  rel.SourceRef,
			State:      rel.State,
			Status:     rel.Status,
			Diagnostic: rel.Diagnostic,
		})
	}
	catalogRuns := make([]CatalogRun, 0, len(runs))
	for _, run := range runs {
		presentation := PresentRunState(run.Status.State)
		catalogRuns = append(catalogRuns, CatalogRun{
			ID:        run.Binding.ID,
			Release:   run.Binding.Release,
			State:     run.Status.State,
			Status:    presentation.Status,
			What:      presentation.What,
			Next:      presentation.Next,
			NeedsYou:  presentation.NeedsYou,
			CreatedAt: run.Binding.CreatedAt,
		})
	}
	needsYou := ProjectNeedsYou(runs)
	return ProjectCatalog{
		SchemaVersion: CatalogSchemaVersion,
		Releases:      catalogReleases,
		Runs:          catalogRuns,
		NeedsYou:      needsYou,
		Diagnostics:   diagnostics,
	}
}

// ProjectNeedsYou extracts human-action-required items across runs with precedence:
// 1. Open attentions ("answer_attention")
// 2. Parked runs ("retry")
// 3. Takeover required ("takeover")
// 4. Awaiting approval ("approve")
func ProjectNeedsYou(runs []DiscoveredRunStatus) []NeedsYouItem {
	items := make([]NeedsYouItem, 0)
	for _, run := range runs {
		// Precedence 1: Open attention turns
		var openAttention *journal.AttentionProjection
		for index := range run.Attentions {
			if run.Attentions[index].State == journal.AttentionOpen {
				openAttention = &run.Attentions[index]
				break
			}
		}
		if openAttention != nil {
			items = append(items, NeedsYouItem{
				RunID:       run.Binding.ID,
				Release:     run.Binding.Release,
				State:       run.Status.State,
				Action:      "answer_attention",
				Reason:      openAttention.Question,
				AttentionID: openAttention.Attention.ID,
			})
			continue
		}

		// Precedence 2: Parked run (needs retry)
		if run.Status.State == "parked" {
			workID := ""
			for _, effect := range run.Status.Effects {
				if work, _, ok := exhaustedAttempt(effect); ok {
					workID = work
					break
				}
			}
			items = append(items, NeedsYouItem{
				RunID:   run.Binding.ID,
				Release: run.Binding.Release,
				State:   "parked",
				Action:  "retry",
				Reason:  "Review the failed item, then retry it using the latest action.",
				WorkID:  workID,
			})
			continue
		}

		// Precedence 3: Takeover required
		if run.Status.State == "takeover_required" {
			items = append(items, NeedsYouItem{
				RunID:   run.Binding.ID,
				Release: run.Binding.Release,
				State:   "takeover_required",
				Action:  "takeover",
				Reason:  "Take over the run so Sworn can recheck it and continue.",
			})
			continue
		}

		// Precedence 4: Awaiting approval
		if run.Status.State == "awaiting_approval" {
			items = append(items, NeedsYouItem{
				RunID:   run.Binding.ID,
				Release: run.Binding.Release,
				State:   "awaiting_approval",
				Action:  "approve",
				Reason:  "Authorize the exact plan through the configured external authorizer.",
			})
			continue
		}
	}
	return items
}

// ProjectRelease builds the same Baton graph used by a live run without
// inventing a journal, run ID, runtime state, or eligible control.
func ProjectRelease(state baton.State) ReleaseSnapshot {
	graph := projectGraph(state, "not_started", nil)
	handoff := projectHandoff(graph)
	diagnostics := make([]Diagnostic, 0, len(state.Diagnostics))
	for _, diagnostic := range state.Diagnostics {
		diagnostics = append(diagnostics, Diagnostic{
			Code:  diagnostic.Code,
			Track: diagnostic.Track,
			Work:  diagnostic.Work,
		})
	}
	return ReleaseSnapshot{
		Graph:        graph,
		Diagnostics:  diagnostics,
		Presentation: presentRelease(state, handoff, diagnostics),
	}
}

func presentRelease(
	state baton.State,
	handoff Handoff,
	diagnostics []Diagnostic,
) RunPresentation {
	presentation := RunPresentation{
		Status:   "Ready for Sworn",
		What:     "Sworn has recorded this release and its next step.",
		Next:     "Start Sworn when the run setup and AI connections are ready.",
		NeedsYou: "Only if you want to start or change the release.",
		Checked:  "The latest saved Sworn release.",
	}
	if len(diagnostics) > 0 {
		presentation.Status = "Needs confirmation"
		presentation.What = "Sworn found a problem with this release."
		presentation.Next = "Review the saved release before starting more work."
		presentation.NeedsYou = "Yes — review the release."
		return presentation
	}
	if state.Assembly.Status == "complete" &&
		state.Assembly.Outcome == "merged" {
		presentation.Status = "Complete"
		presentation.What = "Sworn's records show that this release was merged."
		presentation.Next = "No delivery work remains for this release."
		presentation.NeedsYou = "No."
		return presentation
	}
	if state.Assembly.Status == "blocked" {
		presentation.Status = "Stopped and needs your attention"
		presentation.What = "Part of this release cannot continue."
		presentation.Next = "Review the blocked work before continuing."
		presentation.NeedsYou = "Yes — review the blocked work."
		return presentation
	}
	if handoff.Ready {
		roles := make([]string, 0, len(handoff.Responsibilities))
		for _, role := range handoff.Responsibilities {
			roles = append(roles, humanRole(role))
		}
		presentation.Status = "Ready for " + strings.Join(roles, " and ")
		presentation.What = "Sworn has work ready to be handed over."
		presentation.Next = "Start Sworn to carry the work forward."
		presentation.NeedsYou = "Only if the next step asks for approval or judgment."
		return presentation
	}
	presentation.Status = "Waiting"
	presentation.What = "No work is ready to hand over yet."
	presentation.Next = "Finish the step this release is waiting for."
	presentation.NeedsYou = "Only if the release is waiting on you."
	return presentation
}

func humanRole(role string) string {
	if role == "" {
		return ""
	}
	return strings.ToUpper(role[:1]) + role[1:]
}
