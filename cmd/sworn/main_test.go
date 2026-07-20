package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/app"
	"github.com/swornagent/sworn/internal/board"
	"github.com/swornagent/sworn/internal/buildinfo"
	"github.com/swornagent/sworn/internal/engine"
	"github.com/swornagent/sworn/internal/store"
)

func TestVersionJSON(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	if code := run([]string{"version", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run() code = %d, stderr = %q", code, stderr.String())
	}
	var info buildinfo.Info
	if err := json.Unmarshal(stdout.Bytes(), &info); err != nil {
		t.Fatalf("decode stdout: %v", err)
	}
	if info.Version != "1.0.0-dev" {
		t.Fatalf("version = %q, want 1.0.0-dev", info.Version)
	}
}

func TestBoardReadsCommittedProjection(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "control.db")
	control, err := store.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(engine.CreatePayload{
		DeliveryID: "delivery-1",
		PlanDigest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Repository: "repo-1",
		TargetRef:  "refs/heads/main",
		Work:       []string{"work-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = control.Apply(context.Background(), engine.Command{
		ID: "cmd-create", RunID: "run-1", Kind: engine.CommandCreate,
		ExpectedRevision: engine.NoRevision, Payload: payload,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := control.Close(); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := run([]string{"board", "run-1", "--store", path, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run() code = %d, stderr = %q", code, stderr.String())
	}
	var projection board.Projection
	if err := json.Unmarshal(stdout.Bytes(), &projection); err != nil {
		t.Fatal(err)
	}
	if projection.SchemaVersion != board.SchemaVersion || projection.DeliveryID != "delivery-1" || projection.SourceRevision != 0 {
		t.Fatalf("projection = %+v", projection)
	}
}

func TestUnknownCommandFailsExplicitly(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	if code := run([]string{"deliver"}, &stdout, &stderr); code != 2 {
		t.Fatalf("run() code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "not implemented") {
		t.Fatalf("stderr = %q, want explicit not implemented error", stderr.String())
	}
}

func TestRunCommandInvokesOneBoundedApplication(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "run.json")
	var received app.Request
	invocations := 0
	application := func(_ context.Context, request app.Request) (app.Result, error) {
		invocations++
		received = request
		return validRunResult(), nil
	}
	var stdout, stderr bytes.Buffer
	code := runWithApplication(
		context.Background(),
		[]string{"run", "run-1", "work-1", "--config", configPath, "--json"},
		&stdout, &stderr, application,
	)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("run command = %d, stderr = %q", code, stderr.String())
	}
	if invocations != 1 || received != (app.Request{
		ConfigPath: configPath, RunID: "run-1", WorkID: "work-1",
	}) {
		t.Fatalf("application invocations = %d, request = %#v", invocations, received)
	}
	var result app.Result
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.SchemaVersion != app.RunResultSchemaVersion || result.State != engine.WorkReviewable ||
		result.Revision != 7 {
		t.Fatalf("run result = %#v", result)
	}
}

func TestRunCommandRejectsInvalidApplicationResult(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*app.Result)
	}{
		{"partial executed shape", func(result *app.Result) { result.Admission = nil }},
		{"duplicate effect", func(result *app.Result) { result.CheckEffectIDs[0] = result.BuildEffectID }},
		{"duplicate command", func(result *app.Result) { result.Checks.CommandID = result.Build.CommandID }},
		{"negative recovery", func(result *app.Result) { result.Recovery.Bound = -1 }},
		{"non-contiguous revisions", func(result *app.Result) { result.Checks.Revision-- }},
		{"already reviewable with effect", func(result *app.Result) {
			result.Build = nil
			result.Checks = nil
			result.Admission = nil
			result.BuildEffectID = ""
		}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			result := validRunResult()
			test.mutate(&result)
			var stdout, stderr bytes.Buffer
			code := runWithApplication(
				context.Background(),
				[]string{"run", "run-1", "--config", filepath.Join(t.TempDir(), "run.json"), "--json"},
				&stdout, &stderr,
				func(context.Context, app.Request) (app.Result, error) { return result, nil },
			)
			if code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "invalid bounded run result") {
				t.Fatalf("invalid result = %d, stdout = %q, stderr = %q", code, stdout.String(), stderr.String())
			}
		})
	}
}

func TestRunCommandRejectsInvalidSurfaceBeforeComposition(t *testing.T) {
	t.Parallel()

	invoked := false
	application := func(context.Context, app.Request) (app.Result, error) {
		invoked = true
		return app.Result{}, errors.New("must not run")
	}
	for _, args := range [][]string{
		{"run", "run-1"},
		{"run", "run-1", "--config", "relative.json"},
		{"run", "run-1", "--config", "/tmp/run.json", "--config", "/tmp/run.json"},
		{"run", "run-1", "--config", "/tmp/run.json", "--json", "--json"},
		{"run", "run-1", "work-1", "work-2", "--config", "/tmp/run.json"},
	} {
		var stdout, stderr bytes.Buffer
		if code := runWithApplication(context.Background(), args, &stdout, &stderr, application); code != 2 {
			t.Fatalf("runWithApplication(%v) = %d, stderr = %q", args, code, stderr.String())
		}
	}
	if invoked {
		t.Fatal("invalid command reached application composition")
	}
}

func TestRunCommandReportsApplicationFailureWithoutOutput(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "run.json")
	var stdout, stderr bytes.Buffer
	code := runWithApplication(
		context.Background(), []string{"run", "run-1", "--config", configPath},
		&stdout, &stderr,
		func(context.Context, app.Request) (app.Result, error) {
			return app.Result{}, errors.New("bounded convergence failed")
		},
	)
	if code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "bounded convergence failed") {
		t.Fatalf("run failure = %d, stdout = %q, stderr = %q", code, stdout.String(), stderr.String())
	}
}

func validRunResult() app.Result {
	return app.Result{
		SchemaVersion: app.RunResultSchemaVersion,
		RunID:         "run-1", WorkID: "work-1", State: engine.WorkReviewable, Revision: 7,
		BuildEffectID: "effect-build", CheckEffectIDs: []string{"effect-check"},
		Build:     &app.CommandResult{CommandID: "cmd-build", Revision: 5},
		Checks:    &app.CommandResult{CommandID: "cmd-checks", Revision: 6},
		Admission: &app.CommandResult{CommandID: "cmd-admit", Revision: 7},
	}
}
