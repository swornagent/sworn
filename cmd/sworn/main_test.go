package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

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
	if code := run([]string{"run"}, &stdout, &stderr); code != 2 {
		t.Fatalf("run() code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "not implemented") {
		t.Fatalf("stderr = %q, want explicit not implemented error", stderr.String())
	}
}
