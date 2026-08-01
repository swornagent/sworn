package cockpit

import (
	"strings"

	"github.com/swornagent/sworn/internal/baton"
)

// ReleaseSnapshot is the Baton view shown before a Sworn run exists.
type ReleaseSnapshot struct {
	Graph        Graph
	Diagnostics  []Diagnostic
	Presentation RunPresentation
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
		What:     "Baton has recorded this release and its next step.",
		Next:     "Start Sworn when the run setup and AI connections are ready.",
		NeedsYou: "Only if you want to start or change the release.",
		Checked:  "The latest saved Baton release.",
	}
	if len(diagnostics) > 0 {
		presentation.Status = "Needs confirmation"
		presentation.What = "Baton found a problem with this release."
		presentation.Next = "Review the saved release before starting more work."
		presentation.NeedsYou = "Yes — review the release."
		return presentation
	}
	if state.Assembly.Status == "complete" &&
		state.Assembly.Outcome == "merged" {
		presentation.Status = "Complete"
		presentation.What = "Baton records show that this release was merged."
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
		presentation.What = "Baton has work ready to be handed over."
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
