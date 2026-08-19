package runtime

import (
	"context"
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/journal"
)

func TestRuntimeProposePlanAttemptScopeLintRefusalAndWaiver(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// 1. Create a real git repository with internal/baton and internal/driver
	repoDir := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}

	gitExec, err := resolveGitExecutable()
	if err != nil {
		t.Fatal(err)
	}

	runGit := func(args ...string) string {
		cmd := exec.Command(gitExec, append([]string{"-C", repoDir}, args...)...)
		cmd.Env = []string{
			"HOME=" + t.TempDir(),
			"LANG=C", "LC_ALL=C",
			"GIT_CONFIG_NOSYSTEM=1",
			"GIT_CONFIG_GLOBAL=/dev/null",
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com",
		}
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return string(out)
	}

	runGit("init", "--quiet")
	runGit("branch", "-M", "main")

	if err := os.WriteFile(filepath.Join(repoDir, "go.mod"), []byte("module github.com/swornagent/sworn\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repoDir, "internal", "gitx"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "internal", "gitx", "gitx.go"), []byte("package gitx\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repoDir, "internal", "baton"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "internal", "baton", "baton.go"), []byte("package baton\n\nimport \"github.com/swornagent/sworn/internal/gitx\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repoDir, "internal", "driver"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "internal", "driver", "main.go"), []byte("package driver\n\nimport \"github.com/swornagent/sworn/internal/baton\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	runGit("add", "--all")
	runGit("commit", "--quiet", "-m", "initial")

	// 2. Set up under-derived plan (touching internal/baton, omitting internal/driver)
	underDerivedPlanBytes := []byte("```sworn-release-manifest-v1\n" + `{
  "schema_version": "sworn.release-manifest/v1",
  "release": "runtime-scope-lint",
  "revision": 1,
  "previous_plan": null,
  "repository": "acme-repo",
  "target_ref": "refs/heads/main",
  "approval_ref": "operator://runtime-scope-lint/1",
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

	manifest, _, _ := fixtureManifest(t)
	manifest.Repository = repoDir
	manifest.Release = "runtime-scope-lint"
	manifest.Authority.Project = "acme-repo"
	manifest.Authority.ExternalAuthorizer = "operator"
	manifest.Authority.BootstrapApprovedPlanDigest = nil
	manifest.Scripts = []ScriptedAttempt{
		{
			Responsibility: driver.PlannerProposal,
			BatonAttempt:   1,
			Epoch:          1,
			Try:            1,
			Behavior:       "submit",
		},
	}

	planVal, err := driver.NewPlanBytes(underDerivedPlanBytes)
	if err != nil {
		t.Fatal(err)
	}
	underDerivedSub := driver.Submission{
		SchemaVersion:  driver.SubmissionSchemaVersion,
		InvocationID:   invocationID(manifest.RunID, manifest.Scripts[0]),
		Responsibility: driver.PlannerProposal,
		Summary:        "Under-derived planner proposal.",
		Plan:           planVal,
	}
	manifest.Scripts[0].Submission = encodeSubmission(t, underDerivedSub)

	body, err := canonicalManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}

	store, err := journal.Open(ctx, filepath.Join(t.TempDir(), "runtime-scopelint.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	now := time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)
	dispatcher := fixtureDriver(func(_ context.Context, invocation driver.Invocation) (driver.Observation, error) {
		encoded, _ := base64.StdEncoding.DecodeString(manifest.Scripts[0].Submission)
		return driver.Observation{
			TransportStatus: driver.Completed,
			Usage:           driver.UsageReceipt{TokenStatus: driver.UsageUnavailable, CostStatus: driver.UsageUnavailable},
			Diagnostic:      driver.Diagnostic{Code: "none"},
			Handoff: &driver.SealedHandoff{
				SubmissionBytes:  encoded,
				SubmissionDigest: driver.Digest(encoded),
			},
		}, nil
	})

	service := &Service{
		journal:       store,
		dispatcher:    dispatcher,
		gitExecutable: gitExec,
		now:           func() time.Time { return now },
	}

	// Starting run with under-derived plan should fail with UNDER_DERIVED_SCOPE at proposePlanAttempt
	status, startErr := service.Start(ctx, body)
	if startErr == nil {
		t.Fatalf("expected Start to fail with UNDER_DERIVED_SCOPE, got status: %#v", status)
	}
	if !IsCode(startErr, "UNDER_DERIVED_SCOPE") {
		t.Fatalf("expected error code UNDER_DERIVED_SCOPE, got: %v", startErr)
	}

	// 3. Set up waived plan (carrying waiver for internal/driver)
	waivedPlanBytes := []byte("```sworn-release-manifest-v1\n" + `{
  "schema_version": "sworn.release-manifest/v1",
  "release": "runtime-scope-lint-waived",
  "revision": 1,
  "previous_plan": null,
  "repository": "acme-repo",
  "target_ref": "refs/heads/main",
  "approval_ref": "operator://runtime-scope-lint-waived/1",
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
              "reason": "Driver decoupled"
            }
          ]
        }
      ]
    }
  ]
}` + "\n```\n# Manifest\n")

	manifestWaived := manifest
	manifestWaived.RunID = "runtime-scope-lint-waived-run"
	manifestWaived.Release = "runtime-scope-lint-waived"

	planWaivedVal, err := driver.NewPlanBytes(waivedPlanBytes)
	if err != nil {
		t.Fatal(err)
	}
	waivedSub := driver.Submission{
		SchemaVersion:  driver.SubmissionSchemaVersion,
		InvocationID:   invocationID(manifestWaived.RunID, manifestWaived.Scripts[0]),
		Responsibility: driver.PlannerProposal,
		Summary:        "Waived planner proposal.",
		Plan:           planWaivedVal,
	}
	manifestWaived.Scripts[0].Submission = encodeSubmission(t, waivedSub)

	bodyWaived, err := canonicalManifest(manifestWaived)
	if err != nil {
		t.Fatal(err)
	}

	storeWaived, err := journal.Open(ctx, filepath.Join(t.TempDir(), "runtime-scopelint-waived.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer storeWaived.Close()

	dispatcherWaived := fixtureDriver(func(_ context.Context, invocation driver.Invocation) (driver.Observation, error) {
		encoded, _ := base64.StdEncoding.DecodeString(manifestWaived.Scripts[0].Submission)
		return driver.Observation{
			TransportStatus: driver.Completed,
			Usage:           driver.UsageReceipt{TokenStatus: driver.UsageUnavailable, CostStatus: driver.UsageUnavailable},
			Diagnostic:      driver.Diagnostic{Code: "none"},
			Handoff: &driver.SealedHandoff{
				SubmissionBytes:  encoded,
				SubmissionDigest: driver.Digest(encoded),
			},
		}, nil
	})

	serviceWaived := &Service{
		journal:       storeWaived,
		dispatcher:    dispatcherWaived,
		gitExecutable: gitExec,
		now:           func() time.Time { return now },
	}

	// Starting run with waived plan must succeed in proposePlanAttempt and transition to awaiting_approval
	statusWaived, startWaivedErr := serviceWaived.Start(ctx, bodyWaived)
	if startWaivedErr != nil {
		t.Fatalf("expected Start to succeed for waived plan, got error: %v", startWaivedErr)
	}
	if statusWaived.State != "awaiting_approval" {
		t.Fatalf("expected state awaiting_approval, got: %s", statusWaived.State)
	}
}
