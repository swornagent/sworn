// Package orchestrator holds intra-run orchestration logic for the sworn run
// loop. The triage policy replaces the fixed verdict-switch branching with an
// explainable, deterministic escalation budget — decide whether to retry the
// same model (resolve_in_place), advance the model (escalate_model), or commit
// a terminal state (halt) — based on the verifier's verdict, attempt history,
// and escalation configuration.
//
// Lifecycle routing (BLOCKED→replan-release, failed_verification→redesign|implement)
// is owned by the ported router (internal/router, S58) — the triage only decides
// the intra-run action; it does not re-classify spec-defect vs genuine here.
package orchestrator

import (
	"fmt"

	"github.com/swornagent/sworn/internal/verdict"
)

// Action is the intra-run action chosen by the triage policy.
type Action string

const (
	// ResolveInPlace retries the same model with the S44 feedback (the verifier's
	// prior rationale). The run loop stays on the current escalation slot and
	// increments the per-model resolve counter.
	ResolveInPlace Action = "resolve_in_place"

	// EscalateModel advances to the next model in the escalation list. The
	// per-model resolve counter resets to zero.
	EscalateModel Action = "escalate_model"

	// Halt commits the terminal state (blocked or failed_verification) and
	// returns control to the caller/loop. The router (S58) decides what runs
	// next.
	Halt Action = "halt"
)

// Input carries the information the triage policy needs to make a decision.
type Input struct {
	// Verdict is the verifier's verdict for the just-completed attempt.
	Verdict verdict.Verdict

	// AttemptOnModel is the number of resolve_in_place attempts already made on
	// the current model (0-indexed: 0 means the first attempt on this model
	// just finished).
	AttemptOnModel int

	// ModelIdx is the current model's index in the escalation list.
	ModelIdx int

	// EscalationLen is the total number of models in the escalation list.
	EscalationLen int

	// MaxResolves is the maximum number of resolve_in_place attempts allowed per
	// model before escalating. Default 1 (the spec's K parameter).
	MaxResolves int
}

// Output carries the triage decision.
type Output struct {
	Action Action
	Reason string // explainable rationale for auditability (AC4)
}

// Decide applies the deterministic triage policy.
//
// Policy (first cut, S47 spec):
//   - BLOCKED → halt immediately (no resolve_in_place, no escalation).
//   - FAIL/Inconclusive → resolve_in_place for the first MaxResolves attempts on
//     the same model, then escalate_model, then halt when the escalation list is
//     exhausted.
//
// This is deliberately not an LLM call — it's an explainable, deterministic
// policy that the run loop can log for auditability (AC4).
func Decide(in Input) Output {
	// Ensure MaxResolves is at least 0.
	if in.MaxResolves < 0 {
		in.MaxResolves = 0
	}

	switch in.Verdict {
	case verdict.Blocked:
		return Output{
			Action: Halt,
			Reason: "BLOCKED: halting immediately — violations will be routed to replan-release by the router (S58)",
		}
	case verdict.Pass:
		// PASS should not reach triage — the caller handles it directly.
		// But if it does, halt (no further action needed).
		return Output{
			Action: Halt,
			Reason: "PASS: triage not applicable (caller should handle PASS before triage)",
		}
	default: // FAIL, Inconclusive, or any unknown verdict
		// resolve_in_place: retry same model while under MaxResolves budget.
		if in.AttemptOnModel < in.MaxResolves {
			return Output{
				Action: ResolveInPlace,
				Reason: fmt.Sprintf(
					"FAIL/Inconclusive: resolve_in_place attempt %d/%d on model %d — retrying same model with S44 feedback",
					in.AttemptOnModel+1, in.MaxResolves, in.ModelIdx,
				),
			}
		}

		// escalate_model: advance to next model if available.
		if in.ModelIdx+1 < in.EscalationLen {
			return Output{
				Action: EscalateModel,
				Reason: fmt.Sprintf(
					"FAIL/Inconclusive: resolve budget (%d) exhausted for model %d — escalating to model %d",
					in.MaxResolves, in.ModelIdx, in.ModelIdx+1,
				),
			}
		}

		// halt: escalation list exhausted.
		return Output{
			Action: Halt,
			Reason: fmt.Sprintf(
				"FAIL/Inconclusive: escalation list exhausted (model %d of %d) — halting",
				in.ModelIdx, in.EscalationLen,
			),
		}
	}
}
