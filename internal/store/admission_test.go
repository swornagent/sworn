package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/engine"
	"github.com/swornagent/sworn/internal/protocol"
	"github.com/swornagent/sworn/internal/repo"
)

var atomicAdmissionTime = time.Date(2026, 7, 20, 2, 0, 0, 0, time.UTC)

func TestAtomicAdmissionCommitsReviewableSubmissionAndReplays(t *testing.T) {
	t.Parallel()
	fixture := newAtomicAdmissionFixture(t, atomicAdmissionOptions{})
	ctx := context.Background()
	effectsBefore := tableCount(t, fixture.control, "effects")

	result, err := fixture.control.Apply(ctx, fixture.command)
	if err != nil || result.Outcome != OutcomeApplied || result.Revision != 4 ||
		len(result.EffectIDs) != 0 || result.Replayed {
		t.Fatalf("submission.admit = %+v, %v", result, err)
	}
	state, err := fixture.control.State(ctx, fixture.command.RunID)
	if err != nil {
		t.Fatal(err)
	}
	work := state.Work[0]
	expectedID, err := protocol.SubmissionID(state.DeliveryID, work.ID, work.Attempt)
	if err != nil {
		t.Fatal(err)
	}
	if state.Phase != engine.PhaseActive || work.State != engine.WorkReviewable ||
		work.NextAction != engine.ActionVerify || work.SubmissionID != expectedID ||
		work.SubmissionDigest == "" || work.CandidateCommit != fixture.candidate.Commit {
		t.Fatalf("reviewable state = %+v", state)
	}

	var deliveryID, workID, recordDigest, runID, commandID string
	var attempt int64
	if err := fixture.control.db.QueryRowContext(ctx, `
		SELECT delivery_id, work_id, attempt, digest, run_id, command_id
		FROM submission_records WHERE submission_id = ?`, expectedID,
	).Scan(&deliveryID, &workID, &attempt, &recordDigest, &runID, &commandID); err != nil {
		t.Fatal(err)
	}
	if deliveryID != state.DeliveryID || workID != work.ID || attempt != work.Attempt ||
		recordDigest != work.SubmissionDigest || runID != state.RunID || commandID != fixture.command.ID {
		t.Fatalf("submission provenance = %q %q %d %q %q %q", deliveryID, workID, attempt, recordDigest, runID, commandID)
	}
	kind, canonical, err := fixture.control.Record(ctx, recordDigest)
	if err != nil || kind != protocol.SubmissionSchemaVersion {
		t.Fatalf("canonical submission record = %q, %d bytes, %v", kind, len(canonical), err)
	}
	var submission protocol.Submission
	if err := json.Unmarshal(canonical, &submission); err != nil {
		t.Fatal(err)
	}
	reencoded, err := protocol.EncodeSubmission(submission)
	if err != nil || reencoded.Digest != recordDigest || !bytes.Equal(reencoded.CanonicalJSON, canonical) ||
		submission.SubmissionID != expectedID || submission.Candidate.Commit != fixture.candidate.Commit ||
		len(submission.Checks) != len(fixture.checkEffectIDs) {
		t.Fatalf("persisted submission = %+v, record=%+v, err=%v", submission, reencoded, err)
	}
	for index, check := range submission.Checks {
		if check.ID != fixture.requirements[index].CheckID || check.RunID != fixture.checkEffectIDs[index] {
			t.Fatalf("persisted check %d = %+v", index, check)
		}
	}
	var eventKind string
	var eventData []byte
	if err := fixture.control.db.QueryRowContext(ctx,
		"SELECT kind, data_json FROM events WHERE command_id = ?", fixture.command.ID,
	).Scan(&eventKind, &eventData); err != nil {
		t.Fatal(err)
	}
	var event struct {
		WorkID           string `json:"work_id"`
		SubmissionID     string `json:"submission_id"`
		SubmissionDigest string `json:"submission_digest"`
		CandidateCommit  string `json:"candidate_commit"`
	}
	if err := json.Unmarshal(eventData, &event); err != nil {
		t.Fatal(err)
	}
	if eventKind != "submission.admitted" || event.WorkID != work.ID || event.SubmissionID != expectedID ||
		event.SubmissionDigest != recordDigest || event.CandidateCommit != fixture.candidate.Commit {
		t.Fatalf("admission event = %q %+v", eventKind, event)
	}
	if got := tableCount(t, fixture.control, "effects"); got != effectsBefore {
		t.Fatalf("admission emitted effects: before=%d after=%d", effectsBefore, got)
	}

	replayed, err := fixture.control.Apply(ctx, fixture.command)
	if err != nil || !replayed.Replayed {
		t.Fatalf("submission.admit replay = %+v, %v", replayed, err)
	}
	replayed.Replayed = false
	replayed.EffectIDs = nil
	result.EffectIDs = nil
	if !reflect.DeepEqual(replayed, result) {
		t.Fatalf("replayed result = %+v, want %+v", replayed, result)
	}
	conflict := fixture.command
	conflict.Payload = json.RawMessage(`{"work_id":"different-work"}`)
	if _, err := fixture.control.Apply(ctx, conflict); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("changed admission replay = %v, want idempotency conflict", err)
	}
	assertCount(t, fixture.control, "submission_records", 1)
	assertCount(t, fixture.control, "commands", 5)
	assertCount(t, fixture.control, "events", 5)
}

func TestAtomicAdmissionPreconditionsWriteNothing(t *testing.T) {
	t.Parallel()
	tests := map[string]atomicAdmissionOptions{
		"pending check":                 {pendingCheck: true},
		"non-passing succeeded check":   {nonPassCheck: true},
		"missing configured repository": {withoutRepository: true},
		"missing candidate retention":   {removeCandidateRetention: true},
		"protocol snapshot drift":       {wrongSnapshot: true},
		"candidate scope drift":         {outOfScopeCandidate: true},
	}
	for name, options := range tests {
		name, options := name, options
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fixture := newAtomicAdmissionFixture(t, options)
			fixture.assertAdmissionFailureWritesNothing(t)
		})
	}
}

func TestAtomicAdmissionDetectsFinalCompositionDrift(t *testing.T) {
	t.Parallel()
	tests := map[string]func(*testing.T, *atomicAdmissionFixture){
		"dispatch event mismatch": func(t *testing.T, fixture *atomicAdmissionFixture) {
			execAdmissionSQL(t, fixture.control, "DROP TRIGGER events_no_update")
			execAdmissionSQL(t, fixture.control, `UPDATE events SET data_json = CAST('{"work_id":"other"}' AS BLOB) WHERE kind = 'checks.dispatched'`)
		},
		"dispatch command digest mismatch": func(t *testing.T, fixture *atomicAdmissionFixture) {
			execAdmissionSQL(t, fixture.control, "DROP TRIGGER commands_no_update")
			if _, err := fixture.control.db.Exec(`
				UPDATE commands SET request_digest = ?
				WHERE command_id = (SELECT command_id FROM effects WHERE effect_id = ?)`,
				testLocalCheckDigest("f"), fixture.checkEffectIDs[0],
			); err != nil {
				t.Fatal(err)
			}
		},
		"extra dispatch effect": func(t *testing.T, fixture *atomicAdmissionFixture) {
			effect, err := loadEffect(context.Background(), fixture.control.db, fixture.checkEffectIDs[0])
			if err != nil {
				t.Fatal(err)
			}
			if _, err := fixture.control.db.Exec(`
				INSERT INTO effects (
					effect_id, run_id, command_id, ordinal, kind, request_json,
					state, attempt, created_at_us
				) VALUES ('effect-extra', ?, ?, 2, ?, ?, 'pending', 0, ?)`,
				effect.DeliveryRunID, effect.CommandID, effect.Kind, []byte(effect.Request), effect.CreatedAtUS,
			); err != nil {
				t.Fatal(err)
			}
		},
		"check effect request mismatch": func(t *testing.T, fixture *atomicAdmissionFixture) {
			execAdmissionSQL(t, fixture.control, "DROP TRIGGER effects_restrict_update")
			if _, err := fixture.control.db.Exec(
				"UPDATE effects SET request_json = CAST('{}' AS BLOB) WHERE effect_id = ?", fixture.checkEffectIDs[0],
			); err != nil {
				t.Fatal(err)
			}
		},
		"historical authority missing": func(t *testing.T, fixture *atomicAdmissionFixture) {
			execAdmissionSQL(t, fixture.control, "DROP TRIGGER authority_approvals_no_delete")
			execAdmissionSQL(t, fixture.control, "DELETE FROM authority_approvals")
		},
		"environment CAS corruption": func(t *testing.T, fixture *atomicAdmissionFixture) {
			effect, err := loadEffect(context.Background(), fixture.control.db, fixture.checkEffectIDs[0])
			if err != nil {
				t.Fatal(err)
			}
			result, err := engine.ParseLocalCheckEffectResult(effect.Result)
			if err != nil {
				t.Fatal(err)
			}
			_, receiptBytes, err := fixture.control.Artifact(context.Background(), result.Receipt.Digest)
			if err != nil {
				t.Fatal(err)
			}
			receipt, err := protocol.ParseLocalCheckReceipt(receiptBytes)
			if err != nil {
				t.Fatal(err)
			}
			_, environmentBytes, err := fixture.control.Artifact(context.Background(), receipt.Environment.Ref)
			if err != nil {
				t.Fatal(err)
			}
			execAdmissionSQL(t, fixture.control, "DROP TRIGGER artifacts_no_update")
			if _, err := fixture.control.db.Exec(
				"UPDATE artifacts SET content = ? WHERE digest = ?", append(environmentBytes, ' '), receipt.Environment.Ref,
			); err != nil {
				t.Fatal(err)
			}
		},
		"builder journal chronology drift": func(t *testing.T, fixture *atomicAdmissionFixture) {
			execAdmissionSQL(t, fixture.control, "DROP TRIGGER effects_restrict_update")
			if _, err := fixture.control.db.Exec(
				"UPDATE effects SET started_at_us = started_at_us + 1 WHERE effect_id = ?", fixture.builderEffectID,
			); err != nil {
				t.Fatal(err)
			}
		},
		"builder completion after admission clock": func(t *testing.T, fixture *atomicAdmissionFixture) {
			execAdmissionSQL(t, fixture.control, "DROP TRIGGER effects_restrict_update")
			if _, err := fixture.control.db.Exec(
				"UPDATE effects SET completed_at_us = completed_at_us + 1 WHERE effect_id = ?", fixture.builderEffectID,
			); err != nil {
				t.Fatal(err)
			}
		},
		"check journal chronology drift": func(t *testing.T, fixture *atomicAdmissionFixture) {
			execAdmissionSQL(t, fixture.control, "DROP TRIGGER effects_restrict_update")
			if _, err := fixture.control.db.Exec(
				"UPDATE effects SET completed_at_us = completed_at_us - 1 WHERE effect_id = ?", fixture.checkEffectIDs[0],
			); err != nil {
				t.Fatal(err)
			}
		},
		"check completion after admission clock": func(t *testing.T, fixture *atomicAdmissionFixture) {
			execAdmissionSQL(t, fixture.control, "DROP TRIGGER effects_restrict_update")
			if _, err := fixture.control.db.Exec(
				"UPDATE effects SET completed_at_us = completed_at_us + 1 WHERE effect_id = ?", fixture.checkEffectIDs[0],
			); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, mutate := range tests {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fixture := newAtomicAdmissionFixture(t, atomicAdmissionOptions{})
			mutate(t, fixture)
			fixture.assertAdmissionFailureWritesNothing(t)
		})
	}
}

func TestAtomicAdmissionIdentityFailureRollsBackAndRetries(t *testing.T) {
	t.Parallel()
	fixture := newAtomicAdmissionFixture(t, atomicAdmissionOptions{})
	ctx := context.Background()
	before := fixture.snapshot(t)
	if _, err := fixture.control.db.Exec(`
		CREATE TEMP TRIGGER fail_atomic_submission_identity
		BEFORE INSERT ON submission_records BEGIN
			SELECT RAISE(ABORT, 'injected atomic submission identity failure');
		END`); err != nil {
		t.Fatal(err)
	}
	if result, err := fixture.control.Apply(ctx, fixture.command); err == nil {
		t.Fatalf("submission.admit survived injected identity failure: %+v", result)
	}
	fixture.assertSnapshot(t, before)
	if _, err := fixture.control.db.Exec("DROP TRIGGER fail_atomic_submission_identity"); err != nil {
		t.Fatal(err)
	}
	result, err := fixture.control.Apply(ctx, fixture.command)
	if err != nil || result.Outcome != OutcomeApplied || result.Revision != 4 {
		t.Fatalf("submission.admit retry = %+v, %v", result, err)
	}
	assertCount(t, fixture.control, "submission_records", 1)
}

type atomicAdmissionOptions struct {
	pendingCheck             bool
	nonPassCheck             bool
	withoutRepository        bool
	removeCandidateRetention bool
	wrongSnapshot            bool
	outOfScopeCandidate      bool
}

type atomicAdmissionFixture struct {
	control         *Store
	repository      *repo.Repository
	candidate       repo.Candidate
	builderEffectID string
	requirements    []protocol.LocalCheckRequirement
	checkEffectIDs  []string
	wrongSnapshot   bool
	command         engine.Command
}

func newAtomicAdmissionFixture(t *testing.T, options atomicAdmissionOptions) *atomicAdmissionFixture {
	t.Helper()
	ctx := context.Background()
	repository, candidate := atomicAdmissionCandidate(t, options.outOfScopeCandidate)
	configuration := ControlConfiguration{LocalCheckRuntimeManifestDigest: dispatchRuntimeDigest}
	if !options.withoutRepository {
		configuration.Repository = repository
	}
	control, err := OpenConfigured(ctx, filepath.Join(t.TempDir(), "control.db"), configuration)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = control.Close() })
	control.now = func() time.Time { return atomicAdmissionTime }
	plan := multiCheckExactPlan(t, control)
	authority, _, _ := authorityFixture(t, control, plan, 1, nil, false, func(source map[string]any) {
		source["repository"] = "repo-01"
		for _, raw := range source["maximum_grants"].([]any) {
			grant := raw.(map[string]any)
			if grant["action"] == "integrate" {
				grant["target"].(map[string]any)["repository"] = "repo-01"
			}
		}
	})
	approval, err := authority.Approve(ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	target, workID := plan.Target(), plan.WorkIDs()[0]
	create := testCommand(t, "cmd-create", engine.CommandCreate, engine.NoRevision, engine.CreatePayload{
		DeliveryID: plan.DeliveryID(), PlanDigest: plan.Record().Digest,
		Repository: target.Repository, TargetRef: target.Ref, Work: plan.WorkIDs(),
	})
	if result, err := control.Apply(ctx, create); err != nil || result.Outcome != OutcomeApplied {
		t.Fatalf("create admission delivery = %+v, %v", result, err)
	}
	activate := testCommand(t, "cmd-activate", engine.CommandActivate, 0, engine.ActivatePayload{
		AuthorityReceiptDigest: approval.Facts().ReceiptDigest,
	})
	if result, err := control.Apply(ctx, activate); err != nil || result.Outcome != OutcomeApplied {
		t.Fatalf("activate admission delivery = %+v, %v", result, err)
	}
	contract, _ := plan.Work(workID)
	build := testCommand(t, "cmd-build", engine.CommandDispatchBuild, 1, engine.DispatchBuildPayload{
		WorkID: workID, DispatchDigest: contract.Digest(),
	})
	buildDispatch, err := control.Apply(ctx, build)
	if err != nil || len(buildDispatch.EffectIDs) != 1 {
		t.Fatalf("dispatch admission builder = %+v, %v", buildDispatch, err)
	}
	builderLease, err := control.ClaimNextEffect(ctx, "builder-worker")
	if err != nil || builderLease.Invocation().ID != buildDispatch.EffectIDs[0] {
		t.Fatalf("claim admission builder = %+v, %v", builderLease.Invocation(), err)
	}
	builderResult, err := engine.EncodeBuildEffectResult(engine.BuildEffectResult{
		SchemaVersion: engine.BuildEffectResultSchemaVersion,
		Outcome:       engine.BuildOutcomeCandidateReady,
		Builder: protocol.BuilderRun{
			RunID: builderLease.Invocation().ID, Agent: "sworn-builder/1",
			StartedAt:   atomicAdmissionTime.Format(time.RFC3339Nano),
			CompletedAt: atomicAdmissionTime.Format(time.RFC3339Nano),
		},
		Candidate: candidate,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := control.BindEffectResult(ctx, builderLease, builderResult); err != nil {
		t.Fatal(err)
	}
	if err := control.CompleteEffect(ctx, builderLease); err != nil {
		t.Fatal(err)
	}
	selection, err := protocol.ResolveExactLocalChecks(ctx, control, plan, workID)
	if err != nil {
		t.Fatal(err)
	}
	requirements := selection.Requirements()
	checks := make([]engine.CheckSelection, len(requirements))
	for index, requirement := range requirements {
		checks[index] = engine.CheckSelection{
			CheckID: requirement.CheckID, DefinitionDigest: requirement.Definition.Digest,
		}
	}
	checkCommand := testCommand(t, "cmd-checks", engine.CommandDispatchChecks, 2, engine.DispatchChecksPayload{
		WorkID: workID, BuilderEffectID: builderLease.Invocation().ID,
		RuntimeManifestDigest: dispatchRuntimeDigest, Checks: checks,
	})
	checkDispatch, err := control.Apply(ctx, checkCommand)
	if err != nil || len(checkDispatch.EffectIDs) != len(requirements) {
		t.Fatalf("dispatch admission checks = %+v, %v", checkDispatch, err)
	}
	fixture := &atomicAdmissionFixture{
		control: control, repository: repository, candidate: candidate,
		builderEffectID: builderLease.Invocation().ID, wrongSnapshot: options.wrongSnapshot,
		requirements: requirements, checkEffectIDs: checkDispatch.EffectIDs,
		command: testCommand(t, "cmd-admit", engine.CommandAdmitSubmission, 3,
			engine.AdmitSubmissionPayload{WorkID: workID}),
	}
	for index := range requirements {
		if options.pendingCheck {
			break
		}
		outcome := engine.LocalCheckOutcomePass
		if options.nonPassCheck && index == 0 {
			outcome = engine.LocalCheckOutcomeNotAdmitted
		}
		fixture.completeCheck(t, index, outcome)
	}
	if options.removeCandidateRetention {
		runAtomicAdmissionGit(t, repository.Root(), "update-ref", "-d", candidate.Ref)
	}
	return fixture
}

func (fixture *atomicAdmissionFixture) completeCheck(t *testing.T, index int, outcome string) {
	t.Helper()
	ctx := context.Background()
	lease, err := fixture.control.ClaimNextEffect(ctx, "check-worker-"+fixture.requirements[index].CheckID)
	if err != nil || lease.Invocation().ID != fixture.checkEffectIDs[index] {
		t.Fatalf("claim admission check %d = %+v, %v", index, lease.Invocation(), err)
	}
	request, err := engine.ParseLocalCheckEffectRequest(lease.Invocation().Request)
	if err != nil {
		t.Fatal(err)
	}
	definitionType, definitionBytes, err := fixture.control.Artifact(ctx, request.DefinitionDigest)
	if err != nil || definitionType != "application/json" {
		t.Fatalf("load admission check definition = %q, %v", definitionType, err)
	}
	definition, err := protocol.ParseLocalCheckDefinition(definitionBytes)
	if err != nil {
		t.Fatal(err)
	}
	snapshotDigest, err := protocol.SnapshotDigest()
	if err != nil {
		t.Fatal(err)
	}
	environment := validContentEnvironment(request.RuntimeManifestDigest)
	environment.ProtocolSnapshotDigest = "sha256:" + snapshotDigest
	if fixture.wrongSnapshot {
		environment.ProtocolSnapshotDigest = testLocalCheckDigest("f")
	}
	environmentPointer := fixture.putJSONArtifact(t, protocol.LocalEnvironmentMediaType, environment)
	started := atomicAdmissionTime
	receipt := protocol.LocalCheckReceipt{
		SchemaVersion: protocol.LocalCheckReceiptSchemaVersion,
		CheckID:       request.CheckID, RunID: lease.Invocation().ID,
		Definition: protocol.Artifact{
			Ref: request.DefinitionDigest, MediaType: "application/json", Digest: request.DefinitionDigest,
		},
		Candidate: protocol.CandidatePoint{
			Repository: fixture.candidate.RepositoryID,
			Commit:     fixture.candidate.Commit, Tree: fixture.candidate.Tree,
		},
		WorkspaceDigest:  testLocalCheckDigest(string(rune('1' + index))),
		Environment:      protocol.Environment{Kind: "local", Ref: environmentPointer.Digest},
		WorkspaceAccess:  "read_only",
		WorkingDirectory: definition.WorkingDirectory,
		Argv:             append([]string(nil), definition.Argv...),
		TimeoutSeconds:   definition.TimeoutSeconds,
		Network:          "none",
		StartedAt:        started.Format(time.RFC3339Nano),
		CompletedAt:      started.Format(time.RFC3339Nano),
		Stdout:           fixture.putCapturedArtifact(t, []byte("ok\n")),
		Stderr:           fixture.putCapturedArtifact(t, []byte{}),
	}
	if outcome == engine.LocalCheckOutcomePass {
		receipt.Outcome = "pass"
	} else {
		receipt.Outcome, receipt.ExitCode = "not_admitted", 7
	}
	encodedReceipt, err := protocol.EncodeLocalCheckReceipt(receipt)
	if err != nil {
		t.Fatal(err)
	}
	receiptDigest, err := fixture.control.PutArtifact(ctx, localCheckReceiptMediaType, encodedReceipt.CanonicalJSON)
	if err != nil {
		t.Fatal(err)
	}
	result, err := engine.EncodeLocalCheckEffectResult(engine.LocalCheckEffectResult{
		SchemaVersion: engine.LocalCheckEffectResultSchemaVersion,
		Outcome:       outcome,
		Receipt: protocol.Artifact{
			Ref: receiptDigest, MediaType: localCheckReceiptMediaType, Digest: receiptDigest,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.control.BindEffectResult(ctx, lease, result); err != nil {
		t.Fatal(err)
	}
	if err := fixture.control.CompleteEffect(ctx, lease); err != nil {
		t.Fatal(err)
	}
}

func (fixture *atomicAdmissionFixture) putJSONArtifact(t *testing.T, mediaType string, value any) protocol.Artifact {
	t.Helper()
	contents, err := protocol.EncodeCanonical(value)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := fixture.control.PutArtifact(context.Background(), mediaType, contents)
	if err != nil {
		t.Fatal(err)
	}
	return protocol.Artifact{Ref: digest, MediaType: mediaType, Digest: digest}
}

func (fixture *atomicAdmissionFixture) putCapturedArtifact(t *testing.T, contents []byte) protocol.CapturedArtifact {
	t.Helper()
	digest, err := fixture.control.PutArtifact(context.Background(), "application/octet-stream", contents)
	if err != nil {
		t.Fatal(err)
	}
	return protocol.CapturedArtifact{
		Ref: digest, MediaType: "application/octet-stream", Digest: digest, Size: int64(len(contents)),
	}
}

type atomicAdmissionSnapshot struct {
	state  engine.State
	counts map[string]int
}

func (fixture *atomicAdmissionFixture) snapshot(t *testing.T) atomicAdmissionSnapshot {
	t.Helper()
	state, err := fixture.control.State(context.Background(), fixture.command.RunID)
	if err != nil {
		t.Fatal(err)
	}
	counts := make(map[string]int)
	for _, table := range []string{"commands", "events", "effects", "records", "submission_records"} {
		counts[table] = tableCount(t, fixture.control, table)
	}
	return atomicAdmissionSnapshot{state: state, counts: counts}
}

func (fixture *atomicAdmissionFixture) assertSnapshot(t *testing.T, want atomicAdmissionSnapshot) {
	t.Helper()
	got := fixture.snapshot(t)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("atomic admission changed durable truth after failure:\n got: %+v\nwant: %+v", got, want)
	}
}

func (fixture *atomicAdmissionFixture) assertAdmissionFailureWritesNothing(t *testing.T) {
	t.Helper()
	want := fixture.snapshot(t)
	if result, err := fixture.control.Apply(context.Background(), fixture.command); err == nil {
		t.Fatalf("invalid submission admission applied: %+v", result)
	}
	fixture.assertSnapshot(t, want)
}

func tableCount(t *testing.T, control *Store, table string) int {
	t.Helper()
	var count int
	if err := control.db.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func atomicAdmissionCandidate(t *testing.T, outOfScope bool) (*repo.Repository, repo.Candidate) {
	t.Helper()
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "repository")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	runAtomicAdmissionGit(t, root, "init", "-b", "main")
	runAtomicAdmissionGit(t, root, "config", "user.name", "Test Author")
	runAtomicAdmissionGit(t, root, "config", "user.email", "test@example.invalid")
	writeAtomicAdmissionFile(t, filepath.Join(root, "src", "main.go"), []byte("package main\n"))
	runAtomicAdmissionGit(t, root, "add", "--all")
	runAtomicAdmissionGit(t, root, "commit", "-m", "base")
	binding, err := repo.Discover(ctx, root, "repo-01")
	if err != nil {
		t.Fatal(err)
	}
	repository, err := repo.Open(ctx, root, binding)
	if err != nil {
		t.Fatal(err)
	}
	target, err := repository.BindTarget(ctx, "refs/heads/main")
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := repository.Materialize(ctx, target, filepath.Join(t.TempDir(), "builder"))
	if err != nil {
		t.Fatal(err)
	}
	writeAtomicAdmissionFile(t, filepath.Join(workspace.Path, "src", "main.go"), []byte("package main\n\nfunc ready() bool { return true }\n"))
	scope := repo.Scope{Include: []string{"src", "tests"}, Exclude: []string{"vendor"}}
	if outOfScope {
		writeAtomicAdmissionFile(t, filepath.Join(workspace.Path, "outside.txt"), []byte("outside plan scope\n"))
		scope = repo.Scope{Include: []string{"."}}
	}
	candidate, err := repository.Capture(ctx, workspace, repo.CaptureOptions{
		Scope:     scope,
		Timestamp: time.Date(2026, 7, 20, 0, 1, 1, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	return repository, candidate
}

func execAdmissionSQL(t *testing.T, control *Store, statement string) {
	t.Helper()
	if _, err := control.db.Exec(statement); err != nil {
		t.Fatal(err)
	}
}

func runAtomicAdmissionGit(t *testing.T, root string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, arguments...)...)
	command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(arguments, " "), err, output)
	}
	return string(output)
}

func writeAtomicAdmissionFile(t *testing.T, path string, contents []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		t.Fatal(err)
	}
}
