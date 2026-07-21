package board

import (
	"bytes"
	"os"
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
