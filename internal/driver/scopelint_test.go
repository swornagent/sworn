package driver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDriverLivePlannerProposalScopeLintRefusalAndWaiver(t *testing.T) {
	t.Parallel()

	// Create a temporary workspace with Go files to establish a reverse dependency:
	// internal/baton is imported by internal/driver.
	workspaceDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspaceDir, "go.mod"), []byte("module github.com/swornagent/sworn\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(workspaceDir, "internal", "gitx"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspaceDir, "internal", "gitx", "gitx.go"), []byte("package gitx\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(workspaceDir, "internal", "baton"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspaceDir, "internal", "baton", "baton.go"), []byte("package baton\n\nimport \"github.com/swornagent/sworn/internal/gitx\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(workspaceDir, "internal", "driver"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspaceDir, "internal", "driver", "main.go"), []byte("package driver\n\nimport \"github.com/swornagent/sworn/internal/baton\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	request, err := NewRequest(
		"invocation-planner-scope-lint",
		RolePlanner,
		"fake-profile",
		"selected-model",
		Workspace{Path: workspaceDir, Access: ReadWrite},
		[]Input{},
		true,
		Limits{TimeoutMillis: 60_000, OutputBytes: 65_536},
	)
	if err != nil {
		t.Fatal(err)
	}
	adapter := &memoryAdapter{identity: AdapterIdentity{
		Key:                 "fake-adapter",
		ID:                  FakeDriverID,
		Version:             FakeDriverVersion,
		ConfigurationDigest: "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
	}}
	selected := SelectedProfile{
		Profile: ProfileConfig{
			Key:     request.Profile,
			Adapter: adapter.identity.Key,
			Network: NetworkNone,
		},
		Adapter: adapter.identity,
		Model:   request.Model,
		adapter: adapter,
	}
	permission, err := NewSubmissionPermission(
		request,
		selected,
		ContainmentReadWrite,
		PlannerProposal,
	)
	if err != nil {
		t.Fatal(err)
	}

	invocation := Invocation{
		Request:       request,
		HostWorkspace: workspaceDir,
		Permission:    permission,
		Selected:      selected,
		RecoveryStepHook: func(context.Context, RecoveryStepKind) error {
			return nil
		},
	}

	session, err := newToolSession(invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	// 1. Under-derived plan: touches internal/baton, omits internal/driver
	underDerivedPlanBytes := []byte("```sworn-release-manifest-v1\n" + `{
  "schema_version": "sworn.release-manifest/v1",
  "release": "under-derived-live",
  "revision": 1,
  "previous_plan": null,
  "repository": "golden/sworn",
  "target_ref": "refs/heads/main",
  "approval_ref": "golden://approval/under-derived-live/1",
  "tracks": [
    {
      "id": "T1",
      "depends_on": [],
      "slices": [
        {
          "id": "S1",
          "outcome": "Deliver S1.",
          "contract_path": "contracts/S1.json",
          "digest": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
          "depends_on": [],
          "consumes": [],
          "touchpoints": ["internal/baton"]
        }
      ]
    }
  ]
}` + "\n```\n# Manifest\n")

	underDerivedSubmission := submissionFixture(t, request.InvocationID, PlannerProposal, "")
	underDerivedSubmission.Plan, err = NewPlanBytes(underDerivedPlanBytes)
	if err != nil {
		t.Fatal(err)
	}
	rawArgs, _ := json.Marshal(map[string]any{"submission": underDerivedSubmission})

	// Must be rejected with UNDER_DERIVED_SCOPE
	res := session.execute(context.Background(), providerToolCall{
		ID:        "call-1",
		Name:      "sworn_submit",
		Arguments: rawArgs,
	})
	if !res.Failed || string(res.Content) != "error:UNDER_DERIVED_SCOPE" {
		t.Fatalf("expected error:UNDER_DERIVED_SCOPE, got: failed=%v content=%s", res.Failed, string(res.Content))
	}

	// 2. Waived plan: touches internal/baton, with waiver for internal/driver
	waivedPlanBytes := []byte("```sworn-release-manifest-v1\n" + `{
  "schema_version": "sworn.release-manifest/v1",
  "release": "waived-live",
  "revision": 1,
  "previous_plan": null,
  "repository": "golden/sworn",
  "target_ref": "refs/heads/main",
  "approval_ref": "golden://approval/waived-live/1",
  "tracks": [
    {
      "id": "T1",
      "depends_on": [],
      "slices": [
        {
          "id": "S1",
          "outcome": "Deliver S1.",
          "contract_path": "contracts/S1.json",
          "digest": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
          "depends_on": [],
          "consumes": [],
          "touchpoints": ["internal/baton"],
          "waivers": [
            {
              "package": "internal/driver",
              "reason": "Driver changes deferred"
            }
          ]
        }
      ]
    }
  ]
}` + "\n```\n# Manifest\n")

	waivedSubmission := submissionFixture(t, request.InvocationID, PlannerProposal, "")
	waivedSubmission.Plan, err = NewPlanBytes(waivedPlanBytes)
	if err != nil {
		t.Fatal(err)
	}
	waivedArgs, _ := json.Marshal(map[string]any{"submission": waivedSubmission})

	resWaived := session.execute(context.Background(), providerToolCall{
		ID:        "call-2",
		Name:      "sworn_submit",
		Arguments: waivedArgs,
	})
	if resWaived.Failed || string(resWaived.Content) != "accepted" {
		t.Fatalf("expected accepted for waived plan, got: failed=%v content=%s", resWaived.Failed, string(resWaived.Content))
	}
}
