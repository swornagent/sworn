package cockpit

// RunPresentation explains a stable run state without replacing the raw value
// used by JSON, journals, or other machine-facing contracts.
type RunPresentation struct {
	Status   string
	What     string
	Next     string
	NeedsYou string
	Checked  string
}

// PresentRunState returns the human explanation shared by command-line and
// terminal views.
func PresentRunState(state string) RunPresentation {
	presentation := RunPresentation{
		Checked: "The latest saved run record.",
	}
	switch state {
	case "new":
		presentation.Status = "Ready to start"
		presentation.What = "Sworn has recorded the run and has not started delivery yet."
		presentation.Next = "Start the run when its plan and AI connections are ready."
		presentation.NeedsYou = "Only if you want to start the run."
	case "running":
		presentation.Status = "Sworn is working"
		presentation.What = "Sworn is carrying the next recorded Baton handoff."
		presentation.Next = "Sworn will continue with the next ready handoff."
		presentation.NeedsYou = "No, unless Sworn asks a question."
	case "awaiting_approval":
		presentation.Status = "Waiting for approval"
		presentation.What = "The proposed plan is ready, but delivery has not been approved."
		presentation.Next = "Approve the plan through the configured approval source."
		presentation.NeedsYou = "Yes — approve or decline the plan."
	case "pausing":
		presentation.Status = "Pausing safely"
		presentation.What = "Sworn is waiting for current work to reach a safe stopping point."
		presentation.Next = "The run will become paused when in-flight work is settled."
		presentation.NeedsYou = "No."
	case "paused":
		presentation.Status = "Paused"
		presentation.What = "Sworn has stopped starting new work."
		presentation.Next = "Resume the run when you want delivery to continue."
		presentation.NeedsYou = "Only when you are ready to resume."
	case "cancelling":
		presentation.Status = "Cancelling safely"
		presentation.What = "Sworn is stopping the run at a safe boundary."
		presentation.Next = "The run will become cancelled when in-flight work is settled."
		presentation.NeedsYou = "No."
	case "cancelled":
		presentation.Status = "Cancelled"
		presentation.What = "This run will not start any more delivery work."
		presentation.Next = "No delivery work remains for this run."
		presentation.NeedsYou = "No."
	case "parked":
		presentation.Status = "Stopped and needs your attention"
		presentation.What = "One work item needs an answer or has failed repeatedly."
		presentation.Next = "Open the latest board to answer the question or review the retry action."
		presentation.NeedsYou = "Yes — review the stopped work."
	case "takeover_required":
		presentation.Status = "Resume required"
		presentation.What = "The previous Sworn process stopped and no process currently owns the run."
		presentation.Next = "Take over the run so Sworn can recheck it and continue."
		presentation.NeedsYou = "Yes — take over the run."
	case "uncertain":
		presentation.Status = "Needs confirmation"
		presentation.What = "Sworn cannot confirm whether the last external action finished."
		presentation.Next = "Recover the run before repeating that action."
		presentation.NeedsYou = "Yes — confirm or recover the last action."
	case "complete":
		presentation.Status = "Complete"
		presentation.What = "Baton records show that the checked release was merged."
		presentation.Next = "No delivery work remains."
		presentation.NeedsYou = "No."
	default:
		presentation.Status = "Status unavailable"
		presentation.What = "Sworn does not have a plain-language explanation for this recorded state."
		presentation.Next = "Review the technical state before continuing."
		presentation.NeedsYou = "Yes — review the technical details."
	}
	return presentation
}

// PresentSnapshot adds the human-attention and diagnostic facts available to
// the board without changing the underlying snapshot.
func PresentSnapshot(snapshot Snapshot) RunPresentation {
	presentation := PresentRunState(snapshot.Run.State)
	for _, diagnostic := range snapshot.Diagnostics {
		if diagnostic.Code != "BATON_UNAVAILABLE" {
			continue
		}
		presentation.Status = "Needs confirmation"
		presentation.What = "Sworn could not confirm the current Baton handoff records."
		presentation.Next = "Restore access to the repository and refresh before continuing."
		presentation.NeedsYou = "Yes — the delivery controls are disabled until the facts can be confirmed."
		presentation.Checked = "The saved run record was available, but Baton records could not be confirmed."
		return presentation
	}
	for _, attention := range snapshot.Runtime.Attentions {
		if attention.State != "open" {
			continue
		}
		presentation.Status = "Waiting for your answer"
		presentation.What = "One part of the work needs a judgment Sworn cannot make safely."
		presentation.Next = "Answer the saved question; unrelated work may continue."
		presentation.NeedsYou = "Yes — answer the question shown in Human attention."
		return presentation
	}
	if snapshot.Run.State == "parked" {
		for _, action := range snapshot.Actions {
			if action.Kind != "retry" {
				continue
			}
			presentation.Status = "Stopped after repeated failures"
			presentation.What = "Sworn stopped this work instead of retrying without a safe reason."
			presentation.Next = "Review the failed item, then retry it using the latest action."
			presentation.NeedsYou = "Yes — review the failure before retrying."
			return presentation
		}
	}
	return presentation
}
