package board

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/engine"
)

func TestProjectionMatchesBatonGolden(t *testing.T) {
	t.Parallel()

	state := engine.State{
		SchemaVersion: engine.StateSchemaVersion,
		RunID:         "run-1",
		DeliveryID:    "delivery-1",
		PlanDigest:    "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Repository:    "repo-1",
		TargetRef:     "refs/heads/main",
		Revision:      0,
		Phase:         engine.PhasePlanned,
		Work: []engine.Work{{
			ID: "work-1", State: engine.WorkWaiting, NextAction: engine.ActionWait,
		}},
	}
	projection, err := FromState(state)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := WriteJSON(&output, projection); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/planned.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(output.Bytes(), want) {
		t.Fatalf("projection:\n%s\nwant:\n%s", output.Bytes(), want)
	}
}

func TestProjectionMapsCheckingToPublicActive(t *testing.T) {
	t.Parallel()

	state := activeState(engine.WorkChecking, engine.ActionWait)
	projection, err := FromState(state)
	if err != nil {
		t.Fatal(err)
	}
	work := projection.Work[0]
	if work.State != string(engine.WorkActive) || work.NextAction != string(engine.ActionWait) ||
		work.SubmissionID != "" || work.SubmissionDigest != "" || work.CandidateCommit != "" {
		t.Fatalf("checking projection = %+v", work)
	}
}

func TestProjectionIncludesReviewableSubmissionBinding(t *testing.T) {
	t.Parallel()

	state := activeState(engine.WorkReviewable, engine.ActionVerify)
	state.Work[0].SubmissionID = "submission-1"
	state.Work[0].SubmissionDigest = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	state.Work[0].CandidateCommit = "dddddddddddddddddddddddddddddddddddddddd"
	projection, err := FromState(state)
	if err != nil {
		t.Fatal(err)
	}
	work := projection.Work[0]
	if work.State != string(engine.WorkReviewable) || work.NextAction != string(engine.ActionVerify) ||
		work.SubmissionID != state.Work[0].SubmissionID ||
		work.SubmissionDigest != state.Work[0].SubmissionDigest ||
		work.CandidateCommit != state.Work[0].CandidateCommit {
		t.Fatalf("reviewable projection = %+v", work)
	}
}

func TestProjectionRoutesVerifierOutcomesToBatonLifecycle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		outcome       engine.VerdictOutcome
		workState     engine.WorkState
		nextAction    engine.NextAction
		workAttention string
		deliveryState string
		epoch         int64
	}{
		{
			name: "PASS", outcome: engine.VerdictPass, workState: engine.WorkVerified,
			nextAction: engine.ActionReplan, deliveryState: "verified",
		},
		{
			name: "FAIL", outcome: engine.VerdictFail, workState: engine.WorkRepair,
			nextAction: engine.ActionRepair, deliveryState: "active",
		},
		{
			name: "SPEC_BLOCK", outcome: engine.VerdictSpecBlock, workState: engine.WorkBlocked,
			nextAction: engine.ActionReplan, workAttention: "verifier reported a contract or authority block",
			deliveryState: "attention",
		},
		{
			name: "INCONCLUSIVE", outcome: engine.VerdictInconclusive, workState: engine.WorkRetry,
			nextAction: engine.ActionRetryVerification, deliveryState: "active",
		},
		{
			name: "INCONCLUSIVE exhausted", outcome: engine.VerdictInconclusive, workState: engine.WorkAttention,
			nextAction: engine.ActionReplan, workAttention: "verification retry ceiling reached",
			deliveryState: "attention", epoch: engine.MaximumVerificationEpoch,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			epoch := test.epoch
			if epoch == 0 {
				epoch = 1
			}
			state := verdictState(test.workState, test.nextAction, test.outcome, test.workAttention, epoch)
			projection, err := FromState(state)
			if err != nil {
				t.Fatal(err)
			}
			if projection.State != test.deliveryState {
				t.Fatalf("delivery state = %q, want %q", projection.State, test.deliveryState)
			}
			work := projection.Work[0]
			if work.State != string(test.workState) || work.NextAction != string(test.nextAction) ||
				work.SubmissionBinding != state.Work[0].SubmissionBinding ||
				work.VerdictBinding != state.Work[0].VerdictBinding || work.Attention != test.workAttention {
				t.Fatalf("verdict projection = %+v", work)
			}

			encoded, err := json.Marshal(projection)
			if err != nil {
				t.Fatal(err)
			}
			for _, internal := range []string{"verification_dispatch_id", "verification_epoch", "verdict_epoch"} {
				if strings.Contains(string(encoded), internal) {
					t.Fatalf("projection leaked internal field %q: %s", internal, encoded)
				}
			}
			assertBatonWorkShape(t, encoded, test.workAttention != "")
		})
	}
}

func TestProjectionDeliveryAttentionPreservesFactualReviewableRow(t *testing.T) {
	t.Parallel()

	state := activeState(engine.WorkReviewable, engine.ActionVerify)
	state.Work[0].SubmissionBinding = engine.SubmissionBinding{
		SubmissionID:     "submission-1",
		SubmissionDigest: "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		CandidateCommit:  "dddddddddddddddddddddddddddddddddddddddd",
	}
	state.Attention = []string{"authority changed after submission"}

	projection, err := FromState(state)
	if err != nil {
		t.Fatal(err)
	}
	if projection.State != "attention" || len(projection.Attention) != 1 ||
		projection.Attention[0] != state.Attention[0] {
		t.Fatalf("delivery attention projection = %+v", projection)
	}
	work := projection.Work[0]
	if work.State != string(engine.WorkReviewable) || work.NextAction != string(engine.ActionVerify) ||
		work.SubmissionBinding != state.Work[0].SubmissionBinding || work.VerdictBinding != (engine.VerdictBinding{}) ||
		work.Attention != "" {
		t.Fatalf("attention changed factual work row = %+v", work)
	}

	state.Attention[0] = "mutated"
	if projection.Attention[0] != "authority changed after submission" {
		t.Fatal("projection aliases mutable engine attention")
	}
}

func assertBatonWorkShape(t *testing.T, encoded []byte, hasAttention bool) {
	t.Helper()

	var document struct {
		Work []map[string]json.RawMessage `json:"work"`
	}
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatal(err)
	}
	if len(document.Work) != 1 {
		t.Fatalf("projected work count = %d, want 1", len(document.Work))
	}
	want := map[string]bool{
		"id": true, "state": true, "attempt": true,
		"submission_id": true, "submission_digest": true, "candidate_commit": true,
		"verdict_id": true, "verdict_digest": true, "verdict": true, "next_action": true,
	}
	if hasAttention {
		want["attention"] = true
	}
	if len(document.Work[0]) != len(want) {
		t.Fatalf("projected work fields = %v, want %v", document.Work[0], want)
	}
	for field := range want {
		if _, ok := document.Work[0][field]; !ok {
			t.Errorf("projected work lacks Baton field %q", field)
		}
	}
}

func verdictState(
	workState engine.WorkState,
	action engine.NextAction,
	outcome engine.VerdictOutcome,
	attention string,
	epoch int64,
) engine.State {
	state := activeState(workState, action)
	state.Work[0].SubmissionBinding = engine.SubmissionBinding{
		SubmissionID:     "submission-1",
		SubmissionDigest: "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		CandidateCommit:  "dddddddddddddddddddddddddddddddddddddddd",
	}
	state.Work[0].VerdictBinding = engine.VerdictBinding{
		VerdictID:     "verdict-1",
		VerdictDigest: "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
		Verdict:       outcome,
	}
	state.Work[0].VerificationDispatchID = "dispatch-1"
	state.Work[0].VerificationEpoch = epoch
	state.Work[0].VerdictEpoch = epoch
	state.Work[0].Attention = attention
	return state
}

func activeState(workState engine.WorkState, action engine.NextAction) engine.State {
	return engine.State{
		SchemaVersion:          engine.StateSchemaVersion,
		RunID:                  "run-1",
		DeliveryID:             "delivery-1",
		PlanDigest:             "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Repository:             "repo-1",
		TargetRef:              "refs/heads/main",
		Revision:               3,
		Phase:                  engine.PhaseActive,
		AuthorityReceiptDigest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Work: []engine.Work{{
			ID: "work-1", State: workState, Attempt: 1, NextAction: action,
		}},
	}
}
