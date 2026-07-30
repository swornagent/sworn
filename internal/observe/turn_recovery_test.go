package observe

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTurnRecoveryAggregationIsClosedAndCanonical(t *testing.T) {
	t.Parallel()

	var aggregate turnRecoveryAggregate
	for _, kind := range []string{
		"ordinary_event",
		"turn_recovery.action.resume_worker",
		"turn_recovery.action.ask_captain",
		"turn_recovery.action.resume_worker",
		"turn_recovery.outcome.recovered",
		"turn_recovery.action.pause_track_for_human",
	} {
		if err := aggregate.add(kind); err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
	}
	summary := aggregate.summary()
	if summary.Recovered != 1 || summary.HumanEscalations != 1 ||
		summary.FalseAcceptances != 0 ||
		len(summary.Actions) != 3 ||
		summary.Actions[0] != (TurnRecoveryCount{
			Action: turnRecoveryAskCaptain,
			Count:  1,
		}) ||
		summary.Actions[1] != (TurnRecoveryCount{
			Action: turnRecoveryPauseForHuman,
			Count:  1,
		}) ||
		summary.Actions[2] != (TurnRecoveryCount{
			Action: turnRecoveryResumeWorker,
			Count:  2,
		}) ||
		!validTurnRecoverySummary(summary, 5) {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestTurnRecoveryRejectsOpenOrContentBearingFacts(t *testing.T) {
	t.Parallel()

	for _, kind := range []string{
		"turn_recovery.action",
		"turn_recovery.action.private_action",
		"turn_recovery.action.resume_worker.private",
		"turn_recovery.outcome.human_escalation",
		"turn_recovery.outcome.private_outcome",
		"turn_recovery.outcome.recovered.private",
		"turn_recovery.private.resume_worker",
	} {
		var aggregate turnRecoveryAggregate
		if err := aggregate.add(kind); !IsCode(
			err,
			"INVALID_EVALUATION_FACT",
		) {
			t.Errorf("%q = %v", kind, err)
		}
	}
}

func TestRecoveredOutcomeMayCarryOneClosedContinuationFact(t *testing.T) {
	t.Parallel()

	var aggregate turnRecoveryAggregate
	if err := aggregate.add(
		"turn_recovery.outcome.recovered." +
			"continuation.provider_cursor.reuse",
	); err != nil {
		t.Fatal(err)
	}
	summary := aggregate.summary()
	if summary.Recovered != 1 ||
		!validTurnRecoverySummary(summary, 1) {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestTurnRecoverySummaryJSONContainsOnlyClosedFacts(t *testing.T) {
	t.Parallel()

	const sentinel = "PRIVATE_QUESTION_ANSWER_REASONING"
	var aggregate turnRecoveryAggregate
	if err := aggregate.add(
		"turn_recovery.action.retry_operationally",
	); err != nil {
		t.Fatal(err)
	}
	if err := aggregate.add(
		"turn_recovery.outcome.false_acceptance",
	); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(aggregate.summary())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), sentinel) ||
		string(body) !=
			`{"recovered":0,"human_escalations":0,`+
				`"false_acceptances":1,"actions":[{`+
				`"action":"retry_operationally","count":1}]}` {
		t.Fatalf("summary JSON = %s", body)
	}
}
