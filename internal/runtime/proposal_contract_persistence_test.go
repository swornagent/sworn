package runtime

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/journal"
)

// proposalContractFixture stages a minimal real git repository and a
// sworn.release-manifest/v1 plan whose one slice declares contract_path
// "contracts/S1.json" bound to a real contract's canonical-content digest.
// The target tree never carries that contract file: a proposal minting it
// resolves proposal-time only through submission.Contracts, exactly as S3
// requires. It returns the ready-to-run Service, the exact contract bytes,
// and the run ID.
func proposalContractFixture(t *testing.T) (*Service, string, []byte, string) {
	t.Helper()
	ctx := context.Background()

	repoDir := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	gitExec, err := resolveGitExecutable()
	if err != nil {
		t.Fatal(err)
	}
	runGit := func(args ...string) {
		cmd := exec.Command(gitExec, append([]string{"-C", repoDir}, args...)...)
		cmd.Env = []string{
			"HOME=" + t.TempDir(),
			"LANG=C", "LC_ALL=C",
			"GIT_CONFIG_NOSYSTEM=1",
			"GIT_CONFIG_GLOBAL=/dev/null",
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com",
		}
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	runGit("init", "--quiet")
	runGit("branch", "-M", "main")
	if err := os.MkdirAll(filepath.Join(repoDir, "internal", "baton"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(repoDir, "internal", "baton", "baton.go"),
		[]byte("package baton\n"), 0o644,
	); err != nil {
		t.Fatal(err)
	}
	runGit("add", "--all")
	runGit("commit", "--quiet", "-m", "initial")

	contractRaw := []byte(`{
  "outcome": "Deliver S1.",
  "scope": {"include": ["internal/baton"], "exclude": []},
  "acceptance": [{"id": "A-S1", "text": "S1 is exact."}],
  "checks": ["check S1"],
  "constraints": ["deterministic"],
  "depends_on": [],
  "consumes": []
}
`)
	_, contractDigest, err := baton.ParseSliceContract(contractRaw, "S1", "T1")
	if err != nil {
		t.Fatal(err)
	}
	contractPath := "contracts/S1.json"

	release := "contract-persistence"
	planBytes := []byte("```sworn-release-manifest-v1\n" + `{
  "schema_version": "sworn.release-manifest/v1",
  "release": "` + release + `",
  "revision": 1,
  "previous_plan": null,
  "repository": "acme-repo",
  "target_ref": "refs/heads/main",
  "approval_ref": "operator://` + release + `/1",
  "tracks": [
    {
      "id": "T1",
      "depends_on": [],
      "slices": [
        {
          "id": "S1",
          "outcome": "Deliver S1.",
          "contract_path": "` + contractPath + `",
          "digest": "` + contractDigest + `",
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
	manifest.Release = release
	manifest.Authority.Project = "acme-repo"
	manifest.Authority.ExternalAuthorizer = "operator"
	manifest.Authority.BootstrapApprovedPlanDigest = nil

	planVal, err := driver.NewPlanBytes(planBytes)
	if err != nil {
		t.Fatal(err)
	}
	contractExact := &driver.ExactBytes{
		ByteCount: int64(len(contractRaw)),
		Digest:    driver.Digest(contractRaw),
		Bytes:     base64.StdEncoding.EncodeToString(contractRaw),
	}
	manifest.Scripts = []ScriptedAttempt{
		{
			Responsibility: driver.PlannerProposal,
			BatonAttempt:   1, Epoch: 1, Try: 1,
			Behavior: "submit",
		},
	}
	plannerSubmission := driver.Submission{
		SchemaVersion:  driver.SubmissionSchemaVersion,
		InvocationID:   invocationID(manifest.RunID, manifest.Scripts[0]),
		Responsibility: driver.PlannerProposal,
		Summary:        "Mint the new S1 contract.",
		Detail:         "Carries the new contract beside the plan.",
		Plan:           planVal,
		Contracts:      map[string]*driver.ExactBytes{contractPath: contractExact},
	}
	manifest.Scripts[0].Submission = encodeSubmission(t, plannerSubmission)

	body, err := canonicalManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}

	store, err := journal.Open(ctx, filepath.Join(t.TempDir(), "run.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })

	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	dispatcher := fixtureDriver(func(_ context.Context, invocation driver.Invocation) (driver.Observation, error) {
		encoded, err := base64.StdEncoding.DecodeString(manifest.Scripts[0].Submission)
		if err != nil {
			t.Fatal(err)
		}
		return driver.Observation{
			TransportStatus: driver.Completed,
			Usage: driver.UsageReceipt{
				TokenStatus: driver.UsageUnavailable, CostStatus: driver.UsageUnavailable,
			},
			Diagnostic: driver.Diagnostic{Code: "none"},
			Handoff: &driver.SealedHandoff{
				SubmissionBytes: encoded, SubmissionDigest: driver.Digest(encoded),
			},
		}, nil
	})
	service := &Service{
		journal: store, dispatcher: dispatcher,
		gitExecutable: gitExec, now: func() time.Time { return now },
	}

	status, err := service.Start(ctx, body)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if status.State != "awaiting_approval" {
		t.Fatalf("state = %q, want awaiting_approval", status.State)
	}
	return service, manifest.RunID, contractRaw, contractPath
}

// TestProposalContractsSealAndReadBackDurably pins S3 A1: a proposal minting
// a new contract file records that contract's exact bytes durably beside the
// plan bytes, and re-admitting it from the journal (as every Status call
// does) re-proves those bytes against the sealed submission without error.
func TestProposalContractsSealAndReadBackDurably(t *testing.T) {
	t.Parallel()
	service, runID, contractRaw, contractPath := proposalContractFixture(t)
	ctx := context.Background()

	snapshot, err := service.journal.Snapshot(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	var proposalCommand journal.Command
	found := false
	for _, command := range snapshot.Commands {
		if command.Kind == "planner_proposal" {
			proposalCommand, found = command, true
		}
	}
	if !found {
		t.Fatal("no planner_proposal command recorded")
	}
	var wire planProposalCommand
	if err := json.Unmarshal(proposalCommand.Payload, &wire); err != nil {
		t.Fatal(err)
	}
	if got := wire.ContractBytes[contractPath]; string(got) != string(contractRaw) {
		t.Fatalf("recorded contract bytes = %q, want %q", got, contractRaw)
	}

	// Every Status call re-admits every planner_proposal command through
	// admitPlanProposal, which re-proves carried contract bytes against the
	// sealed submission (submission-proven) and against the plan's own
	// declared digest (digest-proven). A second call proves this is durable
	// rereading, not one-time acceptance.
	for i := 0; i < 2; i++ {
		if _, err := service.Status(ctx, runID); err != nil {
			t.Fatalf("Status[%d]: %v", i, err)
		}
	}
}

// TestProposalContractTamperFailsClosedOnReplay pins S3 A3: a carried
// contract byte tampered in the journal-recorded planner_proposal command
// disagrees with the sealed submission, and admitPlanProposal fails closed
// with CORRUPT_JOURNAL on replay instead of trusting the journal alone.
func TestProposalContractTamperFailsClosedOnReplay(t *testing.T) {
	t.Parallel()
	service, runID, _, contractPath := proposalContractFixture(t)
	ctx := context.Background()

	snapshot, err := service.journal.Snapshot(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	manifest, _, err := loadRunSnapshot(snapshot, runID)
	if err != nil {
		t.Fatal(err)
	}
	commands := make(map[string]journal.Command, len(snapshot.Commands))
	for _, command := range snapshot.Commands {
		commands[command.ReplayKey] = command
	}
	effects := make(map[string]journal.Effect, len(snapshot.Effects))
	for _, effect := range snapshot.Effects {
		effects[effect.ID] = effect
	}
	var proposalCommand journal.Command
	found := false
	for _, command := range snapshot.Commands {
		if command.Kind == "planner_proposal" {
			proposalCommand, found = command, true
		}
	}
	if !found {
		t.Fatal("no planner_proposal command recorded")
	}

	// The untampered command re-admits cleanly: this is the control case
	// that proves the tamper below is what triggers the failure.
	if _, err := admitPlanProposal(manifest, proposalCommand, commands, effects); err != nil {
		t.Fatalf("admitPlanProposal on the exact recorded command: %v", err)
	}

	var wire planProposalCommand
	if err := json.Unmarshal(proposalCommand.Payload, &wire); err != nil {
		t.Fatal(err)
	}
	tampered := append([]byte(nil), wire.ContractBytes[contractPath]...)
	tampered[0] ^= 0xff
	wire.ContractBytes[contractPath] = tampered
	tamperedCommand := proposalCommand
	tamperedCommand.Payload = mustJSON(wire)

	if _, err := admitPlanProposal(manifest, tamperedCommand, commands, effects); !IsCode(err, "CORRUPT_JOURNAL") {
		t.Fatalf("admitPlanProposal on tampered contract bytes = %v, want CORRUPT_JOURNAL", err)
	}
}
