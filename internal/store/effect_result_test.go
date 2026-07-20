package store

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/engine"
	"github.com/swornagent/sworn/internal/protocol"
)

const localCheckReceiptMediaType = "application/vnd.sworn.local-check-receipt+json"

func TestLocalCheckResultClosesEveryKnownOutcome(t *testing.T) {
	t.Parallel()

	for _, outcome := range []string{
		engine.LocalCheckOutcomePass,
		engine.LocalCheckOutcomeNotAdmitted,
		engine.LocalCheckOutcomeControlled,
	} {
		outcome := outcome
		t.Run(outcome, func(t *testing.T) {
			t.Parallel()

			fixture := newLocalCheckEffectFixture(t, outcome)
			lease, result := fixture.insertAndClaim(t)
			if err := fixture.control.BindEffectResult(context.Background(), lease, result); err != nil {
				t.Fatalf("bind %s result: %v", outcome, err)
			}
			if err := fixture.control.CompleteEffect(context.Background(), lease); err != nil {
				t.Fatalf("complete %s result: %v", outcome, err)
			}

			journal, err := fixture.control.SucceededEffect(context.Background(), fixture.effectID)
			if err != nil {
				t.Fatalf("read succeeded %s journal fact: %v", outcome, err)
			}
			parsed, err := engine.ParseLocalCheckEffectResult(journal.Result)
			if err != nil || parsed.Outcome != outcome || parsed.Receipt != fixture.result.Receipt {
				t.Fatalf("succeeded %s result = %+v, %v", outcome, parsed, err)
			}
			rows, err := listEffects(context.Background(), fixture.control, EffectSucceeded)
			if err != nil || len(rows) != 2 || rows[1].ID != fixture.effectID || !bytes.Equal(rows[1].Result, result) {
				t.Fatalf("succeeded effects after %s = %+v, %v", outcome, rows, err)
			}
		})
	}
}

func TestLocalCheckResultRejectsBrokenArtifactClosure(t *testing.T) {
	t.Parallel()

	tests := map[string]func(*testing.T, *localCheckEffectFixture){
		"builder candidate": func(t *testing.T, fixture *localCheckEffectFixture) {
			fixture.receipt.Candidate.Commit = strings.Repeat("e", 40)
			fixture.persistReceipt(t)
		},
		"definition digest": func(t *testing.T, fixture *localCheckEffectFixture) {
			changed := validLocalCheckDefinition([]string{"/usr/bin/false"})
			pointer := fixture.putJSONArtifact(t, "application/json", changed)
			fixture.receipt.Definition = pointer
			fixture.receipt.Argv = append([]string(nil), changed.Argv...)
			fixture.persistReceipt(t)
		},
		"definition argv": func(t *testing.T, fixture *localCheckEffectFixture) {
			fixture.receipt.Argv = []string{"/usr/bin/false"}
			fixture.persistReceipt(t)
		},
		"runtime environment": func(t *testing.T, fixture *localCheckEffectFixture) {
			driftedRuntime := testLocalCheckDigest("f")
			pointer := fixture.putJSONArtifact(
				t, protocol.LocalEnvironmentMediaType, validContentEnvironment(driftedRuntime),
			)
			fixture.receipt.Environment.Ref = pointer.Digest
			fixture.persistReceipt(t)
		},
		"environment timeout": func(t *testing.T, fixture *localCheckEffectFixture) {
			environment := validContentEnvironment(fixture.request.RuntimeManifestDigest)
			environment.Limits.RuntimeNanoseconds = 1_000_000_000
			pointer := fixture.putJSONArtifact(t, protocol.LocalEnvironmentMediaType, environment)
			fixture.receipt.Environment.Ref = pointer.Digest
			fixture.persistReceipt(t)
		},
		"stdout limit": func(t *testing.T, fixture *localCheckEffectFixture) {
			environment := validContentEnvironment(fixture.request.RuntimeManifestDigest)
			environment.Limits.StdoutBytes = 1
			pointer := fixture.putJSONArtifact(t, protocol.LocalEnvironmentMediaType, environment)
			fixture.receipt.Environment.Ref = pointer.Digest
			fixture.persistReceipt(t)
		},
		"stderr limit": func(t *testing.T, fixture *localCheckEffectFixture) {
			environment := validContentEnvironment(fixture.request.RuntimeManifestDigest)
			environment.Limits.StderrBytes = 1
			pointer := fixture.putJSONArtifact(t, protocol.LocalEnvironmentMediaType, environment)
			fixture.receipt.Environment.Ref = pointer.Digest
			fixture.persistReceipt(t)
		},
		"semantic outcome": func(_ *testing.T, fixture *localCheckEffectFixture) {
			fixture.result.Outcome = engine.LocalCheckOutcomePass
		},
		"receipt pointer": func(_ *testing.T, fixture *localCheckEffectFixture) {
			fixture.result.Receipt.Ref = testLocalCheckDigest("9")
		},
		"missing receipt CAS": func(_ *testing.T, fixture *localCheckEffectFixture) {
			missing := testLocalCheckDigest("8")
			fixture.result.Receipt.Ref = missing
			fixture.result.Receipt.Digest = missing
		},
		"capture pointer": func(t *testing.T, fixture *localCheckEffectFixture) {
			fixture.receipt.Stdout.Ref = fixture.receipt.Stderr.Digest
			fixture.persistReceipt(t)
		},
	}

	for name, mutate := range tests {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fixture := newLocalCheckEffectFixture(t, engine.LocalCheckOutcomeNotAdmitted)
			mutate(t, fixture)
			lease, result := fixture.insertAndClaim(t)
			if err := fixture.control.BindEffectResult(context.Background(), lease, result); err == nil {
				t.Fatal("effect result with a broken closure was bound")
			}
			running, err := listEffects(context.Background(), fixture.control, EffectRunning)
			if err != nil || len(running) != 1 || running[0].ID != fixture.effectID || len(running[0].Result) != 0 {
				t.Fatalf("rejected result changed journal state: %+v, %v", running, err)
			}
		})
	}
}

func TestOrphanLocalCheckReceiptCannotReconcileSuccess(t *testing.T) {
	t.Parallel()

	fixture := newLocalCheckEffectFixture(t, engine.LocalCheckOutcomePass)
	lease, _ := fixture.insertAndClaim(t)
	if mediaType, contents, err := fixture.control.Artifact(
		context.Background(), fixture.result.Receipt.Digest,
	); err != nil || mediaType != localCheckReceiptMediaType || len(contents) == 0 {
		t.Fatalf("orphan receipt fixture = %q, %d bytes, %v", mediaType, len(contents), err)
	}
	if recovered, err := fixture.control.RecoverInterruptedEffects(
		context.Background(), "worker exited after writing an unbound receipt artifact",
	); err != nil || recovered != 1 {
		t.Fatalf("recover unbound check attempt = %d, %v", recovered, err)
	}
	if err := fixture.control.ReconcileUnknownEffect(
		context.Background(), fixture.effectID, lease.Invocation().Attempt, "reconciler-1", ReconcileSucceeded, "",
	); err == nil {
		t.Fatal("orphan receipt artifact reconciled an unbound check effect as succeeded")
	}
	unknown, err := listEffects(context.Background(), fixture.control, EffectUnknown)
	if err != nil || len(unknown) != 1 || unknown[0].ID != fixture.effectID || len(unknown[0].Result) != 0 {
		t.Fatalf("unknown check changed after rejected reconciliation = %+v, %v", unknown, err)
	}
}

type localCheckEffectFixture struct {
	control  *Store
	effectID string
	request  engine.LocalCheckEffectRequest
	result   engine.LocalCheckEffectResult
	receipt  protocol.LocalCheckReceipt
}

func newLocalCheckEffectFixture(t *testing.T, outcome string) *localCheckEffectFixture {
	t.Helper()
	ctx := context.Background()
	control := openTestStore(t, filepath.Join(t.TempDir(), "control.db"))
	t.Cleanup(func() { _ = control.Close() })

	builderEffectID := createActivateAndDispatch(t, control)
	builderLease, err := control.ClaimNextEffect(ctx, "builder-worker")
	if err != nil {
		t.Fatalf("claim builder: %v", err)
	}
	builderResult := validBuildResult(t, builderEffectID, "sworn-builder/1")
	if err := control.BindEffectResult(ctx, builderLease, builderResult); err != nil {
		t.Fatalf("bind builder: %v", err)
	}
	if err := control.CompleteEffect(ctx, builderLease); err != nil {
		t.Fatalf("complete builder: %v", err)
	}
	parsedBuilder, err := engine.ParseBuildEffectResult(builderResult)
	if err != nil {
		t.Fatalf("parse builder fixture: %v", err)
	}

	fixture := &localCheckEffectFixture{
		control:  control,
		effectID: "effect-check-1",
		request: engine.LocalCheckEffectRequest{
			SchemaVersion:         engine.LocalCheckEffectRequestSchemaVersion,
			DeliveryRunID:         "run-1",
			DeliveryID:            "delivery-1",
			WorkID:                "work-1",
			WorkAttempt:           1,
			BuilderEffectID:       builderEffectID,
			CheckID:               "check-1",
			RuntimeManifestDigest: testLocalCheckDigest("e"),
		},
		result: engine.LocalCheckEffectResult{
			SchemaVersion: engine.LocalCheckEffectResultSchemaVersion,
			Outcome:       outcome,
		},
	}
	definition := validLocalCheckDefinition([]string{"/usr/bin/true"})
	definitionPointer := fixture.putJSONArtifact(t, "application/json", definition)
	fixture.request.DefinitionDigest = definitionPointer.Digest
	environmentPointer := fixture.putJSONArtifact(
		t, protocol.LocalEnvironmentMediaType, validContentEnvironment(fixture.request.RuntimeManifestDigest),
	)
	stdout := fixture.putCapturedArtifact(t, []byte("ok\n"))
	stderr := fixture.putCapturedArtifact(t, []byte("diagnostic\n"))
	fixture.receipt = protocol.LocalCheckReceipt{
		SchemaVersion: protocol.LocalCheckReceiptSchemaVersion,
		CheckID:       fixture.request.CheckID,
		RunID:         fixture.effectID,
		Definition:    definitionPointer,
		Candidate: protocol.CandidatePoint{
			Repository: parsedBuilder.Candidate.RepositoryID,
			Commit:     parsedBuilder.Candidate.Commit,
			Tree:       parsedBuilder.Candidate.Tree,
		},
		WorkspaceDigest:  testLocalCheckDigest("d"),
		Environment:      protocol.Environment{Kind: "local", Ref: environmentPointer.Digest},
		WorkspaceAccess:  "read_only",
		WorkingDirectory: definition.WorkingDirectory,
		Argv:             append([]string(nil), definition.Argv...),
		TimeoutSeconds:   definition.TimeoutSeconds,
		Network:          "none",
		StartedAt:        "2026-07-20T00:00:02Z",
		CompletedAt:      "2026-07-20T00:00:03Z",
		Stdout:           stdout,
		Stderr:           stderr,
	}
	switch outcome {
	case engine.LocalCheckOutcomePass:
		fixture.receipt.Outcome = "pass"
		fixture.receipt.ExitCode = 0
	case engine.LocalCheckOutcomeNotAdmitted:
		fixture.receipt.Outcome = "not_admitted"
		fixture.receipt.ExitCode = 7
	case engine.LocalCheckOutcomeControlled:
		fixture.receipt.Outcome = "not_admitted"
		fixture.receipt.ExitCode = -1
		fixture.receipt.TimedOut = true
	default:
		t.Fatalf("unsupported fixture outcome %q", outcome)
	}
	fixture.persistReceipt(t)
	return fixture
}

func (fixture *localCheckEffectFixture) insertAndClaim(t *testing.T) (EffectLease, json.RawMessage) {
	t.Helper()
	request, err := engine.EncodeLocalCheckEffectRequest(fixture.request)
	if err != nil {
		t.Fatalf("encode local check request: %v", err)
	}
	result, err := engine.EncodeLocalCheckEffectResult(fixture.result)
	if err != nil {
		t.Fatalf("encode local check result: %v", err)
	}
	now := fixture.control.now().UTC().UnixMicro()
	if _, err := fixture.control.db.ExecContext(context.Background(), `
		INSERT INTO effects (
			effect_id, run_id, command_id, ordinal, kind, request_json, state,
			attempt, owner_id, receipt_json, last_error, created_at_us,
			started_at_us, completed_at_us
		) VALUES (?, 'run-1', 'cmd-dispatch', 1, ?, ?, 'pending', 0, NULL, NULL, NULL, ?, NULL, NULL)`,
		fixture.effectID, engine.EffectLocalCheck, []byte(request), now,
	); err != nil {
		t.Fatalf("insert local check effect: %v", err)
	}
	lease, err := fixture.control.ClaimNextEffect(context.Background(), "check-worker")
	if err != nil || lease.Invocation().ID != fixture.effectID {
		t.Fatalf("claim local check effect = %+v, %v", lease, err)
	}
	return lease, result
}

func (fixture *localCheckEffectFixture) persistReceipt(t *testing.T) {
	t.Helper()
	encoded, err := protocol.EncodeLocalCheckReceipt(fixture.receipt)
	if err != nil {
		t.Fatalf("encode local check receipt: %v", err)
	}
	digest, err := fixture.control.PutArtifact(context.Background(), localCheckReceiptMediaType, encoded.CanonicalJSON)
	if err != nil {
		t.Fatalf("store local check receipt: %v", err)
	}
	fixture.result.Receipt = protocol.Artifact{Ref: digest, MediaType: localCheckReceiptMediaType, Digest: digest}
}

func (fixture *localCheckEffectFixture) putCapturedArtifact(t *testing.T, contents []byte) protocol.CapturedArtifact {
	t.Helper()
	digest, err := fixture.control.PutArtifact(context.Background(), "application/octet-stream", contents)
	if err != nil {
		t.Fatalf("store captured artifact: %v", err)
	}
	return protocol.CapturedArtifact{
		Ref: digest, MediaType: "application/octet-stream", Digest: digest, Size: int64(len(contents)),
	}
}

func (fixture *localCheckEffectFixture) putJSONArtifact(
	t *testing.T,
	mediaType string,
	value any,
) protocol.Artifact {
	t.Helper()
	contents, err := protocol.EncodeCanonical(value)
	if err != nil {
		t.Fatalf("encode %s artifact: %v", mediaType, err)
	}
	digest, err := fixture.control.PutArtifact(context.Background(), mediaType, contents)
	if err != nil {
		t.Fatalf("store %s artifact: %v", mediaType, err)
	}
	return protocol.Artifact{Ref: digest, MediaType: mediaType, Digest: digest}
}

func validLocalCheckDefinition(argv []string) protocol.LocalCheckDefinition {
	return protocol.LocalCheckDefinition{
		SchemaVersion:    protocol.LocalCheckDefinitionSchemaVersion,
		Argv:             argv,
		WorkingDirectory: ".",
		TimeoutSeconds:   10,
		Evidence: protocol.LocalEvidenceDefinition{
			ID: "evidence-1", AcceptanceIDs: []string{"AC1"},
			Boundary: "component", Observed: "the candidate check completed",
		},
	}
}

func validContentEnvironment(runtimeDigest string) protocol.LocalEnvironment {
	return protocol.LocalEnvironment{
		SchemaVersion:          protocol.ContentEnvironmentSchemaVersion,
		ProtocolSnapshotDigest: testLocalCheckDigest("a"),
		EngineRuntime:          "go1.25.0",
		OS:                     "linux",
		Architecture:           "amd64",
		Executor: protocol.LocalExecutorProbe{
			BubblewrapVersion: "bubblewrap 0.11.0",
			SystemdVersion:    "systemd 257",
			CgroupV2:          true,
			UserManager:       "running",
			Controllers:       []string{"cpu", "memory", "pids"},
		},
		ExecutorPolicyVersion: "sworn-linux-containment-v1",
		Limits: protocol.LocalExecutionLimits{
			RuntimeNanoseconds: 10_000_000_000,
			MemoryBytes:        64 << 20,
			Tasks:              16,
			CPUPercent:         100,
			FileBytes:          1 << 20,
			TempBytes:          1 << 20,
			HomeBytes:          1 << 20,
			InputBytes:         1 << 20,
			WorkspaceBytes:     1 << 20,
			StdoutBytes:        1 << 20,
			StderrBytes:        1 << 20,
		},
		RuntimeTrustRoot:      "/usr",
		RuntimeManifestDigest: runtimeDigest,
		WorkspaceAccess:       "read_only",
		Network:               "none",
	}
}

func testLocalCheckDigest(hex string) string {
	return "sha256:" + strings.Repeat(hex, 64)
}
