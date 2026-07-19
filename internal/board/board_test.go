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
