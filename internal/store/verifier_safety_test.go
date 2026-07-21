//go:build linux

package store

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/engine"
	"github.com/swornagent/sworn/internal/policy"
	"github.com/swornagent/sworn/internal/protocol"
)

const (
	rotatedVerifierProfileDigest = "sha256:9292929292929292929292929292929292929292929292929292929292929292"
	rotatedVerifierAgent         = "codex-cli/verifier-rotated-test"
)

type verifierSafetySnapshot struct {
	state   engine.State
	effects []Effect
	counts  [9]int
}

type verifierDBEntryContext struct {
	context.Context
	once    sync.Once
	reached chan struct{}
}

func (ctx *verifierDBEntryContext) Done() <-chan struct{} {
	ctx.once.Do(func() { close(ctx.reached) })
	return ctx.Context.Done()
}

func takeVerifierSafetySnapshot(t *testing.T, fixture *verifierLifecycleFixture) verifierSafetySnapshot {
	t.Helper()
	ctx := context.Background()
	state, err := fixture.control.State(ctx, fixture.runID)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := fixture.control.db.QueryContext(ctx, `
		SELECT effect_id, run_id, command_id, ordinal, kind, request_json, state,
		       attempt, owner_id, receipt_json, last_error, created_at_us,
		       started_at_us, completed_at_us
		FROM effects ORDER BY effect_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close() //nolint:errcheck
	var effects []Effect
	for rows.Next() {
		effect, err := scanEffect(rows)
		if err != nil {
			t.Fatal(err)
		}
		effects = append(effects, effect)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	var counts [9]int
	if err := fixture.control.db.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM commands),
			(SELECT COUNT(*) FROM events),
			(SELECT COUNT(*) FROM effects),
			(SELECT COUNT(*) FROM effect_observations),
			(SELECT COUNT(*) FROM records),
			(SELECT COUNT(*) FROM artifacts),
			(SELECT COUNT(*) FROM authority_source_snapshots),
			(SELECT COUNT(*) FROM verifier_dispatch_records),
			(SELECT COUNT(*) FROM verdict_records)`).Scan(
		&counts[0], &counts[1], &counts[2], &counts[3], &counts[4],
		&counts[5], &counts[6], &counts[7], &counts[8],
	); err != nil {
		t.Fatal(err)
	}
	return verifierSafetySnapshot{state: state, effects: effects, counts: counts}
}

func requireVerifierSafetyUnchanged(
	t *testing.T,
	fixture *verifierLifecycleFixture,
	want verifierSafetySnapshot,
) {
	t.Helper()
	got := takeVerifierSafetySnapshot(t, fixture)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("rejected verifier operation mutated Store truth:\n got: %+v\nwant: %+v", got, want)
	}
}

func advanceVerifierAuthorityHead(
	t *testing.T,
	fixture *verifierLifecycleFixture,
	revoked bool,
) *policy.Authority {
	t.Helper()
	replacement, _, _ := authorityFixture(
		t, fixture.control, fixture.plan, 3, fixture.privateKey, false,
		controlledSourceMutation(fixture.plan, func(source map[string]any) {
			if revoked {
				source["status"] = "revoked"
				source["maximum_grants"] = []any{}
			}
		}),
	)
	_, err := replacement.Approve(context.Background(), fixture.plan)
	if revoked {
		if err == nil || !strings.Contains(err.Error(), "revoked") {
			t.Fatalf("persist verifier authority revocation = %v", err)
		}
		return replacement
	}
	if err != nil {
		t.Fatalf("persist superseding verifier authority = %v", err)
	}
	return replacement
}

func rotateVerifierConfiguration(fixture *verifierLifecycleFixture) {
	fixture.control.verifierProfileDigest = rotatedVerifierProfileDigest
	fixture.control.verifierAgent = rotatedVerifierAgent
}

func verifierSafetyCommand(
	t *testing.T,
	fixture *verifierLifecycleFixture,
	id string,
	kind engine.CommandKind,
	revision int64,
	payload any,
) engine.Command {
	t.Helper()
	command := testCommand(t, id, kind, revision, payload)
	command.RunID = fixture.runID
	return command
}

func TestStoreVerifierRawApplyCannotDispatchOrAdmit(t *testing.T) {
	t.Run("dispatch", func(t *testing.T) {
		fixture := newVerifierLifecycleFixture(t)
		ctx := context.Background()
		state, err := fixture.control.State(ctx, fixture.runID)
		if err != nil {
			t.Fatal(err)
		}
		command := verifierSafetyCommand(
			t, fixture, "cmd-raw-verifier-dispatch", engine.CommandDispatchVerifier, state.Revision,
			engine.DispatchVerifierPayload{WorkID: fixture.workID},
		)
		before := takeVerifierSafetySnapshot(t, fixture)
		if result, err := fixture.control.Apply(ctx, command); err == nil ||
			!strings.Contains(err.Error(), "current controller authority") {
			t.Fatalf("raw verifier dispatch = %+v, %v", result, err)
		}
		requireVerifierSafetyUnchanged(t, fixture, before)
	})

	t.Run("verdict", func(t *testing.T) {
		fixture := newVerifierLifecycleFixture(t)
		ctx := context.Background()
		fixture.dispatch(t, "cmd-raw-verdict-dispatch")
		fixture.execute(t, engine.VerdictFail, false)
		state, err := fixture.control.State(ctx, fixture.runID)
		if err != nil {
			t.Fatal(err)
		}
		command := verifierSafetyCommand(
			t, fixture, "cmd-raw-verdict-admit", engine.CommandAdmitVerdict, state.Revision,
			engine.AdmitVerdictPayload{WorkID: fixture.workID},
		)
		before := takeVerifierSafetySnapshot(t, fixture)
		if result, err := fixture.control.Apply(ctx, command); err == nil ||
			!strings.Contains(err.Error(), "Store-owned verdict admission") {
			t.Fatalf("raw verifier verdict admission = %+v, %v", result, err)
		}
		requireVerifierSafetyUnchanged(t, fixture, before)
	})
}

func TestStoreVerifierNonPASSAdmissionRechecksOwnershipInsideTransaction(t *testing.T) {
	fixture := newVerifierLifecycleFixture(t)
	ctx := context.Background()
	fixture.dispatch(t, "cmd-non-pass-ownership-dispatch")
	fixture.execute(t, engine.VerdictFail, false)
	state, _, passRequest, prepared, err := fixture.control.PrepareControlledVerdictAdmission(
		ctx, fixture.ownership, fixture.ownerID, fixture.runID, fixture.workID,
		"cmd-non-pass-ownership-verdict",
	)
	if err != nil || passRequest != (policy.PASSAdmissionPermitRequest{}) {
		t.Fatalf("prepare non-PASS ownership race = %+v, %v", passRequest, err)
	}
	command := verifierSafetyCommand(
		t, fixture, "cmd-non-pass-ownership-verdict", engine.CommandAdmitVerdict,
		state.Revision, engine.AdmitVerdictPayload{WorkID: fixture.workID},
	)
	before := takeVerifierSafetySnapshot(t, fixture)
	heldConnection, err := fixture.control.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	entryContext := &verifierDBEntryContext{Context: ctx, reached: make(chan struct{})}
	type admissionResult struct {
		result ApplyResult
		err    error
	}
	finished := make(chan admissionResult, 1)
	ownership, ownerID := fixture.ownership, fixture.ownerID
	go func() {
		result, err := fixture.control.ApplyControlledVerdictAdmission(
			entryContext, ownership, ownerID, prepared, command,
		)
		finished <- admissionResult{result: result, err: err}
	}()
	select {
	case <-entryContext.reached:
	case <-time.After(2 * time.Second):
		_ = heldConnection.Close()
		t.Fatal("non-PASS admission did not reach its Store transaction")
	}
	if err := ownership.Close(); err != nil {
		_ = heldConnection.Close()
		t.Fatal(err)
	}
	fixture.ownership = nil
	if err := heldConnection.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case outcome := <-finished:
		if outcome.err == nil || !strings.Contains(outcome.err.Error(), "ownership") {
			t.Fatalf("non-PASS admission crossed ownership loss = %+v, %v", outcome.result, outcome.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("non-PASS admission did not stop after ownership loss")
	}
	requireVerifierSafetyUnchanged(t, fixture, before)
}

func TestStoreVerifierStaleAuthorityCannotDispatchOrClaim(t *testing.T) {
	changes := []struct {
		name    string
		revoked bool
	}{
		{name: "superseded"},
		{name: "revoked", revoked: true},
	}
	for _, change := range changes {
		change := change
		t.Run(change.name+"_before_dispatch", func(t *testing.T) {
			fixture := newVerifierLifecycleFixture(t)
			ctx := context.Background()
			state, plan, request, prepared, err := fixture.control.PrepareControlledVerifierDispatch(
				ctx, fixture.ownership, fixture.ownerID, fixture.runID, fixture.workID,
				"cmd-stale-authority-dispatch",
			)
			if err != nil {
				t.Fatal(err)
			}
			permit, err := fixture.authority.AuthorizeVerifierExecution(ctx, plan, request)
			if err != nil {
				t.Fatal(err)
			}
			command := verifierSafetyCommand(
				t, fixture, "cmd-stale-authority-dispatch", engine.CommandDispatchVerifier,
				state.Revision, engine.DispatchVerifierPayload{WorkID: fixture.workID},
			)
			advanceVerifierAuthorityHead(t, fixture, change.revoked)
			before := takeVerifierSafetySnapshot(t, fixture)
			if result, err := fixture.control.ApplyControlledVerifierDispatch(
				ctx, fixture.ownership, fixture.authority, permit, request, prepared, command,
			); err == nil || !strings.Contains(err.Error(), "superseded") {
				t.Fatalf("stale-authority verifier dispatch = %+v, %v", result, err)
			}
			requireVerifierSafetyUnchanged(t, fixture, before)
		})

		t.Run(change.name+"_before_claim", func(t *testing.T) {
			fixture := newVerifierLifecycleFixture(t)
			ctx := context.Background()
			dispatch := fixture.dispatch(t, "cmd-stale-authority-claim-dispatch")
			_, plan, request, err := fixture.control.PendingVerifierExecutionPermitRequest(
				ctx, fixture.ownership, fixture.ownerID, fixture.runID, fixture.workID,
			)
			if err != nil {
				t.Fatal(err)
			}
			permit, err := fixture.authority.AuthorizeVerifierExecution(ctx, plan, request)
			if err != nil {
				t.Fatal(err)
			}
			advanceVerifierAuthorityHead(t, fixture, change.revoked)
			before := takeVerifierSafetySnapshot(t, fixture)
			if lease, err := fixture.control.ClaimControlledVerifier(
				ctx, fixture.ownership, fixture.authority, permit, request,
			); err == nil || !strings.Contains(err.Error(), "superseded") {
				t.Fatalf("stale-authority verifier claim = %+v, %v", lease.effect, err)
			}
			requireVerifierSafetyUnchanged(t, fixture, before)
			effect, err := loadEffect(ctx, fixture.control.db, dispatch.EffectIDs[0])
			if err != nil || effect.State != EffectPending || effect.Attempt != 0 || effect.OwnerID != "" {
				t.Fatalf("rejected claim effect = %+v, %v", effect, err)
			}
		})
	}
}

func TestStoreVerifierAuthoritySupersededAfterClaimStopsPreparation(t *testing.T) {
	fixture := newVerifierLifecycleFixture(t)
	ctx := context.Background()
	fixture.dispatch(t, "cmd-post-claim-authority-dispatch")
	_, plan, request, err := fixture.control.PendingVerifierExecutionPermitRequest(
		ctx, fixture.ownership, fixture.ownerID, fixture.runID, fixture.workID,
	)
	if err != nil {
		t.Fatal(err)
	}
	permit, err := fixture.authority.AuthorizeVerifierExecution(ctx, plan, request)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := fixture.control.ClaimControlledVerifier(
		ctx, fixture.ownership, fixture.authority, permit, request,
	)
	if err != nil {
		t.Fatal(err)
	}
	replacement := advanceVerifierAuthorityHead(t, fixture, false)
	before := takeVerifierSafetySnapshot(t, fixture)
	if prepared, err := fixture.control.PrepareAuthorizedVerifierExecution(ctx, lease); err == nil ||
		!strings.Contains(err.Error(), "superseded") {
		t.Fatalf("post-claim stale authority prepared verifier = %+v, %v", prepared.effect, err)
	}
	requireVerifierSafetyUnchanged(t, fixture, before)
	copyOfLease := lease
	if err := fixture.control.AbortAuthorizedVerifier(
		ctx, lease, "current verifier authority was superseded before worker entry",
	); err != nil {
		t.Fatal(err)
	}
	if err := fixture.control.AbortAuthorizedVerifier(ctx, copyOfLease, "copied retry"); err == nil {
		t.Fatal("copied authorized verifier lease aborted twice")
	}
	if _, err := fixture.control.PrepareAuthorizedVerifierExecution(ctx, copyOfLease); err == nil {
		t.Fatal("aborted authorized verifier lease entered worker preparation")
	}
	effect, err := loadEffect(ctx, fixture.control.db, request.VerifierEffectID)
	if err != nil || effect.State != EffectPending || effect.Attempt != 1 || len(effect.Result) != 0 ||
		effect.OwnerID != "" || effect.LastError != "" {
		t.Fatalf("aborted post-claim verifier = %+v, %v", effect, err)
	}
	var unresolved int
	if err := fixture.control.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM effects WHERE state IN ('running', 'unknown')`,
	).Scan(&unresolved); err != nil || unresolved != 0 {
		t.Fatalf("post-abort unresolved verifier effects = %d, %v", unresolved, err)
	}
	fixture.authority = replacement
	_, retryPlan, retryRequest, err := fixture.control.PendingVerifierExecutionPermitRequest(
		ctx, fixture.ownership, fixture.ownerID, fixture.runID, fixture.workID,
	)
	if err != nil {
		t.Fatal(err)
	}
	retryPermit, err := fixture.authority.AuthorizeVerifierExecution(ctx, retryPlan, retryRequest)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := fixture.control.ClaimControlledVerifier(
		ctx, fixture.ownership, fixture.authority, retryPermit, retryRequest,
	)
	if err != nil || retry.effect.ID != request.VerifierEffectID || retry.effect.Attempt != 2 {
		t.Fatalf("safe verifier re-claim after abort = %+v, %v", retry.effect, err)
	}
	state, err := fixture.control.State(ctx, fixture.runID)
	if err != nil || state.Work[0].VerificationEpoch != 1 ||
		state.Work[0].VerificationDispatchID != request.VerifierEffectID {
		t.Fatalf("safe verifier re-claim state = %+v, %v", state, err)
	}
	if err := fixture.control.AbortAuthorizedVerifier(ctx, retry, "test cleanup before retry entry"); err != nil {
		t.Fatal(err)
	}
}

func TestStoreVerifierPreparedAbortClosesAdapterSetupFailure(t *testing.T) {
	fixture := newVerifierLifecycleFixture(t)
	ctx := context.Background()
	fixture.dispatch(t, "cmd-prepared-abort-dispatch")
	request, claimed, prepared := fixture.claimAndPrepare(t)
	copyOfPrepared := prepared
	advanceVerifierAuthorityHead(t, fixture, true)
	if err := fixture.control.AbortAuthorizedVerifier(ctx, claimed, "wrong abort phase"); err == nil {
		t.Fatal("claimed-lease abort accepted a verifier that was already prepared")
	}
	if err := fixture.control.AbortPreparedAuthorizedVerifier(
		ctx, prepared, "adapter setup failed before verifier worker entry",
	); err != nil {
		t.Fatal(err)
	}
	turns := 0
	if _, err := copyOfPrepared.RunVerifier(func(engine.JournalEffect) (json.RawMessage, error) {
		turns++
		return nil, nil
	}); err == nil || !strings.Contains(err.Error(), "already consumed") {
		t.Fatalf("copied prepared verifier entered after abort: turns=%d error=%v", turns, err)
	}
	if turns != 0 {
		t.Fatalf("prepared abort allowed %d verifier turns", turns)
	}
	if err := fixture.control.AbortPreparedAuthorizedVerifier(
		ctx, copyOfPrepared, "copied prepared retry",
	); err == nil {
		t.Fatal("copied prepared verifier abort was reusable")
	}
	effect, err := loadEffect(ctx, fixture.control.db, request.VerifierEffectID)
	if err != nil || effect.State != EffectPending || effect.Attempt != 1 || len(effect.Result) != 0 ||
		effect.OwnerID != "" || effect.LastError != "" {
		t.Fatalf("prepared verifier abort effect = %+v, %v", effect, err)
	}
	var notAppliedObservations int
	if err := fixture.control.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM effect_observations
		WHERE effect_id = ? AND attempt = 1 AND kind = 'not_applied'`, request.VerifierEffectID,
	).Scan(&notAppliedObservations); err != nil || notAppliedObservations != 1 {
		t.Fatalf("prepared verifier abort observations = %d, %v", notAppliedObservations, err)
	}
}

func TestStoreVerifierAbortRejectsConsumedAndForeignCapabilities(t *testing.T) {
	t.Run("consumed", func(t *testing.T) {
		fixture := newVerifierLifecycleFixture(t)
		ctx := context.Background()
		fixture.dispatch(t, "cmd-consumed-abort-dispatch")
		request, claimed, prepared := fixture.claimAndPrepare(t)
		result := fixture.resultFor(t, request.VerifierEffectID, engine.VerdictFail)
		runResult, err := prepared.RunVerifier(func(engine.JournalEffect) (json.RawMessage, error) {
			return result, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		before := takeVerifierSafetySnapshot(t, fixture)
		if err := fixture.control.AbortPreparedAuthorizedVerifier(
			ctx, prepared, "worker already entered",
		); err == nil {
			t.Fatal("consumed prepared verifier capability was abortable")
		}
		if err := fixture.control.AbortAuthorizedVerifier(
			ctx, claimed, "worker already entered",
		); err == nil {
			t.Fatal("consumed claimed verifier capability was abortable")
		}
		requireVerifierSafetyUnchanged(t, fixture, before)
		if err := fixture.control.BindAuthorizedVerifierResult(ctx, prepared, runResult); err != nil {
			t.Fatal(err)
		}
		if err := fixture.control.CompleteAuthorizedVerifier(ctx, prepared); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("foreign_store", func(t *testing.T) {
		first := newVerifierLifecycleFixture(t)
		second := newVerifierLifecycleFixture(t)
		ctx := context.Background()
		first.dispatch(t, "cmd-foreign-abort-dispatch")
		_, plan, request, err := first.control.PendingVerifierExecutionPermitRequest(
			ctx, first.ownership, first.ownerID, first.runID, first.workID,
		)
		if err != nil {
			t.Fatal(err)
		}
		permit, err := first.authority.AuthorizeVerifierExecution(ctx, plan, request)
		if err != nil {
			t.Fatal(err)
		}
		lease, err := first.control.ClaimControlledVerifier(
			ctx, first.ownership, first.authority, permit, request,
		)
		if err != nil {
			t.Fatal(err)
		}
		firstBefore := takeVerifierSafetySnapshot(t, first)
		secondBefore := takeVerifierSafetySnapshot(t, second)
		if err := second.control.AbortAuthorizedVerifier(
			ctx, lease, "foreign Store abort",
		); err == nil {
			t.Fatal("foreign Store aborted an authorized verifier lease")
		}
		requireVerifierSafetyUnchanged(t, first, firstBefore)
		requireVerifierSafetyUnchanged(t, second, secondBefore)
		if err := first.control.AbortAuthorizedVerifier(ctx, lease, "test cleanup before entry"); err != nil {
			t.Fatal(err)
		}
	})
}

func TestStoreVerifierRejectsMalformedAndMismatchedTypedResults(t *testing.T) {
	tests := []struct {
		name      string
		result    func(*testing.T, *verifierLifecycleFixture, string) json.RawMessage
		wantError string
	}{
		{
			name: "malformed",
			result: func(_ *testing.T, _ *verifierLifecycleFixture, _ string) json.RawMessage {
				return json.RawMessage(`{"schema_version":`)
			},
			wantError: "decode verifier effect result",
		},
		{
			name: "mismatched_dispatch",
			result: func(t *testing.T, fixture *verifierLifecycleFixture, effectID string) json.RawMessage {
				valid := fixture.resultFor(t, effectID, engine.VerdictFail)
				parsed, err := engine.ParseVerifierEffectResult(valid)
				if err != nil {
					t.Fatal(err)
				}
				parsed.DispatchID = "different-verifier-dispatch"
				mismatched, err := engine.EncodeVerifierEffectResult(parsed)
				if err != nil {
					t.Fatal(err)
				}
				return mismatched
			},
			wantError: "does not match",
		},
		{
			name: "mismatched_profile",
			result: func(t *testing.T, fixture *verifierLifecycleFixture, effectID string) json.RawMessage {
				return fixture.rewriteVerifierExecutionReceipt(
					t, fixture.resultFor(t, effectID, engine.VerdictFail),
					func(receipt *protocol.VerifierExecutionReceipt) {
						receipt.VerifierProfileDigest = rotatedVerifierProfileDigest
					},
				)
			},
			wantError: "does not match its journal request and result",
		},
		{
			name: "mismatched_executor_configuration",
			result: func(t *testing.T, fixture *verifierLifecycleFixture, effectID string) json.RawMessage {
				return fixture.rewriteVerifierExecutionReceipt(
					t, fixture.resultFor(t, effectID, engine.VerdictFail),
					func(receipt *protocol.VerifierExecutionReceipt) {
						receipt.ExecutorConfigurationDigest = rotatedVerifierProfileDigest
					},
				)
			},
			wantError: "does not match its exact profile",
		},
		{
			name: "mismatched_review_input",
			result: func(t *testing.T, fixture *verifierLifecycleFixture, effectID string) json.RawMessage {
				return fixture.rewriteVerifierExecutionReceipt(
					t, fixture.resultFor(t, effectID, engine.VerdictFail),
					func(receipt *protocol.VerifierExecutionReceipt) {
						for index := range receipt.Inputs {
							if receipt.Inputs[index].Name == "review-policy" {
								receipt.Inputs[index].Size++
								return
							}
						}
						t.Fatal("verifier receipt lacks review-policy input")
					},
				)
			},
			wantError: "does not bind its exact review input closure",
		},
		{
			name: "mismatched_stdout_capture",
			result: func(t *testing.T, fixture *verifierLifecycleFixture, effectID string) json.RawMessage {
				return fixture.rewriteVerifierExecutionReceipt(
					t, fixture.resultFor(t, effectID, engine.VerdictFail),
					func(receipt *protocol.VerifierExecutionReceipt) { receipt.Stdout.Size++ },
				)
			},
			wantError: "resolve verifier execution stdout capture: invalid exact capture",
		},
		{
			name: "malformed_stdout_stream",
			result: func(t *testing.T, fixture *verifierLifecycleFixture, effectID string) json.RawMessage {
				return fixture.rewriteVerifierExecutionReceipt(
					t, fixture.resultFor(t, effectID, engine.VerdictFail),
					func(receipt *protocol.VerifierExecutionReceipt) {
						receipt.Stdout = fixture.putVerifierCapture(t, []byte("not JSONL\n"))
					},
				)
			},
			wantError: "parse verifier execution stdout capture",
		},
		{
			name: "mismatched_stdout_assessment",
			result: func(t *testing.T, fixture *verifierLifecycleFixture, effectID string) json.RawMessage {
				alternate, err := protocol.EncodeCanonical(fixture.assessment(t, engine.VerdictPass))
				if err != nil {
					t.Fatal(err)
				}
				return fixture.rewriteVerifierExecutionReceipt(
					t, fixture.resultFor(t, effectID, engine.VerdictFail),
					func(receipt *protocol.VerifierExecutionReceipt) {
						receipt.Stdout = fixture.putVerifierCapture(
							t, verifierLifecycleJSONL(t, alternate, receipt.ThreadID),
						)
					},
				)
			},
			wantError: "does not reproduce its exact assessment and thread",
		},
		{
			name: "mismatched_stdout_thread",
			result: func(t *testing.T, fixture *verifierLifecycleFixture, effectID string) json.RawMessage {
				return fixture.rewriteVerifierExecutionReceipt(
					t, fixture.resultFor(t, effectID, engine.VerdictFail),
					func(receipt *protocol.VerifierExecutionReceipt) {
						receipt.ThreadID = "thread-other"
					},
				)
			},
			wantError: "does not reproduce its exact assessment and thread",
		},
		{
			name: "mismatched_completion_time",
			result: func(t *testing.T, fixture *verifierLifecycleFixture, effectID string) json.RawMessage {
				return fixture.rewriteVerifierExecutionReceipt(
					t, fixture.resultFor(t, effectID, engine.VerdictFail),
					func(receipt *protocol.VerifierExecutionReceipt) {
						receipt.CompletedAt = atomicAdmissionTime.Add(time.Second).Format(time.RFC3339Nano)
					},
				)
			},
			wantError: "does not match its journal request and result",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			fixture := newVerifierLifecycleFixture(t)
			ctx := context.Background()
			fixture.dispatch(t, "cmd-bad-result-dispatch")
			request, _, prepared := fixture.claimAndPrepare(t)
			workerResult := test.result(t, fixture, request.VerifierEffectID)
			runResult, err := prepared.RunVerifier(func(effect engine.JournalEffect) (json.RawMessage, error) {
				if effect.ID != request.VerifierEffectID {
					t.Fatalf("bad-result verifier invocation = %+v", effect)
				}
				return workerResult, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			before := takeVerifierSafetySnapshot(t, fixture)
			if err := fixture.control.BindAuthorizedVerifierResult(ctx, prepared, runResult); err == nil ||
				!strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("bind %s verifier result error = %v", test.name, err)
			}
			requireVerifierSafetyUnchanged(t, fixture, before)
			effect, err := loadEffect(ctx, fixture.control.db, request.VerifierEffectID)
			if err != nil || effect.State != EffectRunning || len(effect.Result) != 0 {
				t.Fatalf("rejected %s result effect = %+v, %v", test.name, effect, err)
			}
		})
	}
}

func TestStoreVerifierCompletionRejectsFutureReviewIntervalBeforeTerminalState(t *testing.T) {
	fixture := newVerifierLifecycleFixture(t)
	ctx := context.Background()
	fixture.dispatch(t, "cmd-future-review-dispatch")
	request, _, prepared := fixture.claimAndPrepare(t)
	future := atomicAdmissionTime.Add(time.Hour)
	encoded := fixture.rewriteVerifierExecutionReceipt(
		t, fixture.resultFor(t, request.VerifierEffectID, engine.VerdictFail),
		func(receipt *protocol.VerifierExecutionReceipt) {
			receipt.StartedAt = future.Format(time.RFC3339Nano)
			receipt.CompletedAt = future.Format(time.RFC3339Nano)
		},
	)
	result, err := engine.ParseVerifierEffectResult(encoded)
	if err != nil {
		t.Fatal(err)
	}
	result.StartedAt = future.Format(time.RFC3339Nano)
	result.CompletedAt = future.Format(time.RFC3339Nano)
	encoded, err = engine.EncodeVerifierEffectResult(result)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := prepared.RunVerifier(func(engine.JournalEffect) (json.RawMessage, error) {
		return encoded, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.control.BindAuthorizedVerifierResult(ctx, prepared, observed); err != nil {
		t.Fatalf("bind future verifier interval: %v", err)
	}
	if err := fixture.control.CompleteAuthorizedVerifier(ctx, prepared); err == nil ||
		!strings.Contains(err.Error(), "completed journal lease") {
		t.Fatalf("future verifier interval became terminal: %v", err)
	}
	effect, err := loadEffect(ctx, fixture.control.db, request.VerifierEffectID)
	if err != nil || effect.State != EffectRunning || len(effect.Result) == 0 || effect.CompletedAtUS != 0 {
		t.Fatalf("rejected future interval effect = %+v, %v", effect, err)
	}

	// Once the durable clock actually contains the adapter-recorded interval,
	// the same bound turn completes; no second verifier invocation is needed.
	fixture.control.now = func() time.Time { return future.Add(time.Second) }
	if err := fixture.control.CompleteAuthorizedVerifier(ctx, prepared); err != nil {
		t.Fatalf("complete contained verifier interval: %v", err)
	}
	fixture.admit(t, engine.VerdictFail, "cmd-future-review-verdict")
	state, err := fixture.control.State(ctx, fixture.runID)
	if err != nil || state.Work[0].State != engine.WorkRepair || state.Work[0].Verdict != engine.VerdictFail {
		t.Fatalf("contained future verifier verdict = %+v, %v", state, err)
	}
}

func TestStoreVerifierHistoricalVerdictSurvivesConfigurationRotation(t *testing.T) {
	tests := []struct {
		outcome engine.VerdictOutcome
		state   engine.WorkState
	}{
		{outcome: engine.VerdictFail, state: engine.WorkRepair},
		{outcome: engine.VerdictPass, state: engine.WorkVerified},
	}
	for _, test := range tests {
		test := test
		t.Run(string(test.outcome), func(t *testing.T) {
			fixture := newVerifierLifecycleFixture(t)
			ctx := context.Background()
			fixture.dispatch(t, "cmd-config-rotation-dispatch")
			fixture.execute(t, test.outcome, false)
			rotateVerifierConfiguration(fixture)
			fixture.admit(t, test.outcome, "cmd-config-rotation-verdict")
			state, err := fixture.control.State(ctx, fixture.runID)
			if err != nil || state.Work[0].State != test.state || state.Work[0].Verdict != test.outcome {
				t.Fatalf("rotated-config historical %s verdict = %+v, %v", test.outcome, state, err)
			}
		})
	}
}

func TestStoreVerifierPendingOldProfileCannotRunAfterConfigurationRotation(t *testing.T) {
	fixture := newVerifierLifecycleFixture(t)
	ctx := context.Background()
	dispatch := fixture.dispatch(t, "cmd-pending-old-profile-dispatch")
	rotateVerifierConfiguration(fixture)
	before := takeVerifierSafetySnapshot(t, fixture)
	_, plan, request, gateErr := fixture.control.PendingVerifierExecutionPermitRequest(
		ctx, fixture.ownership, fixture.ownerID, fixture.runID, fixture.workID,
	)
	if gateErr == nil {
		permit, err := fixture.authority.AuthorizeVerifierExecution(ctx, plan, request)
		gateErr = err
		if gateErr == nil {
			_, gateErr = fixture.control.ClaimControlledVerifier(
				ctx, fixture.ownership, fixture.authority, permit, request,
			)
		}
	}
	if gateErr == nil {
		t.Fatal("pending verifier under an old profile was claimable after configuration rotation")
	}
	requireVerifierSafetyUnchanged(t, fixture, before)
	effect, err := loadEffect(ctx, fixture.control.db, dispatch.EffectIDs[0])
	if err != nil || effect.State != EffectPending || effect.Attempt != 0 || effect.OwnerID != "" {
		t.Fatalf("old-profile pending verifier = %+v, %v; gate error = %v", effect, err, gateErr)
	}
}

func TestOpenConfiguredRejectsPendingVerifierConfigurationRotation(t *testing.T) {
	fixture := newVerifierLifecycleFixture(t)
	ctx := context.Background()
	fixture.dispatch(t, "cmd-reopen-pending-profile-dispatch")
	path := fixture.control.ControlPath()
	repository := fixture.control.repository
	original := ControlConfiguration{
		LocalCheckRuntimeManifestDigest: fixture.control.localCheckRuntimeManifestDigest,
		BuilderDispatchDigest:           fixture.control.builderDispatchDigest,
		VerifierProfileDigest:           fixture.control.verifierProfileDigest,
		VerifierAgent:                   fixture.control.verifierAgent,
		Repository:                      repository,
	}
	rotated := original
	rotated.VerifierProfileDigest = rotatedVerifierProfileDigest
	rotated.VerifierAgent = rotatedVerifierAgent
	if err := fixture.ownership.Close(); err != nil {
		t.Fatal(err)
	}
	fixture.ownership = nil
	if err := fixture.control.Close(); err != nil {
		t.Fatal(err)
	}
	if reopened, err := OpenConfigured(ctx, path, rotated); err == nil ||
		!strings.Contains(err.Error(), "pending verifier effect") ||
		!strings.Contains(err.Error(), "configured profile and agent differ") {
		if reopened != nil {
			_ = reopened.Close()
		}
		t.Fatalf("rotated configured reopen across pending verifier = %v", err)
	}
	reopened, err := OpenConfigured(ctx, path, original)
	if err != nil {
		t.Fatalf("matching configured reopen after rejection: %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestControllerActivationRechecksPendingVerifierConfiguration(t *testing.T) {
	fixture := newVerifierLifecycleFixture(t)
	ctx := context.Background()
	rotated := ControlConfiguration{
		LocalCheckRuntimeManifestDigest: fixture.control.localCheckRuntimeManifestDigest,
		BuilderDispatchDigest:           fixture.control.builderDispatchDigest,
		VerifierProfileDigest:           rotatedVerifierProfileDigest,
		VerifierAgent:                   rotatedVerifierAgent,
		Repository:                      fixture.control.repository,
	}

	// This process opens while the old controller has no pending verifier, so
	// the early diagnostic passes. The old owner then dispatches before
	// releasing its retained lock; activation must repeat the check inside the
	// owned SQLite snapshot.
	successorStore, err := OpenConfigured(ctx, fixture.control.ControlPath(), rotated)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = successorStore.Close() })
	dispatch := fixture.dispatch(t, "cmd-activation-profile-race-dispatch")
	if err := fixture.ownership.Close(); err != nil {
		t.Fatal(err)
	}
	fixture.ownership = nil

	const successorID = "activation-profile-race-successor"
	successor, err := successorStore.AcquireControllerOwnership(successorID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = successor.Close() })
	if err := successor.Activate(ctx, successorStore, successorID); err == nil ||
		!strings.Contains(err.Error(), "prove pending verifier configuration") ||
		!strings.Contains(err.Error(), "configured profile and agent differ") {
		t.Fatalf("activation crossed pending verifier profile race: %v", err)
	}
	if err := successor.ValidateRecovery(successorStore, successorID); err != nil {
		t.Fatalf("rejected activation did not remain recovery-only: %v", err)
	}
	effect, err := loadEffect(ctx, successorStore.db, dispatch.EffectIDs[0])
	if err != nil || effect.State != EffectPending || effect.Attempt != 0 || effect.OwnerID != "" {
		t.Fatalf("rejected activation changed pending verifier = %+v, %v", effect, err)
	}
}

func TestOpenConfiguredAllowsSucceededVerifierConfigurationRotation(t *testing.T) {
	fixture := newVerifierLifecycleFixture(t)
	ctx := context.Background()
	fixture.dispatch(t, "cmd-reopen-succeeded-profile-dispatch")
	fixture.execute(t, engine.VerdictFail, false)
	path := fixture.control.ControlPath()
	configuration := ControlConfiguration{
		LocalCheckRuntimeManifestDigest: fixture.control.localCheckRuntimeManifestDigest,
		BuilderDispatchDigest:           fixture.control.builderDispatchDigest,
		VerifierProfileDigest:           rotatedVerifierProfileDigest,
		VerifierAgent:                   rotatedVerifierAgent,
		Repository:                      fixture.control.repository,
	}
	if err := fixture.ownership.Close(); err != nil {
		t.Fatal(err)
	}
	fixture.ownership = nil
	if err := fixture.control.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenConfigured(ctx, path, configuration)
	if err != nil {
		t.Fatalf("rotated configured reopen rejected succeeded verifier: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	const successorID = "succeeded-profile-rotation-controller"
	successor, err := reopened.AcquireControllerOwnership(successorID)
	if err != nil {
		t.Fatal(err)
	}
	fixture.control, fixture.ownership, fixture.ownerID = reopened, successor, successorID
	if err := successor.Activate(ctx, reopened, successorID); err != nil {
		t.Fatal(err)
	}
	fixture.admit(t, engine.VerdictFail, "cmd-reopen-succeeded-profile-verdict")
	state, err := reopened.State(ctx, fixture.runID)
	if err != nil || state.Work[0].State != engine.WorkRepair ||
		state.Work[0].Verdict != engine.VerdictFail {
		t.Fatalf("rotated reopen historical verdict = %+v, %v", state, err)
	}
}

func TestStoreVerifierPASSAttentionCannotReuseVerdictCommandID(t *testing.T) {
	fixture := newVerifierLifecycleFixture(t)
	ctx := context.Background()
	fixture.dispatch(t, "cmd-pass-attention-identity-dispatch")
	fixture.execute(t, engine.VerdictPass, false)
	const verdictCommandID = "cmd-pass-attention-identity-verdict"
	state, _, request, prepared, err := fixture.control.PrepareControlledVerdictAdmission(
		ctx, fixture.ownership, fixture.ownerID, fixture.runID, fixture.workID, verdictCommandID,
	)
	if err != nil || request.Outcome != string(engine.VerdictPass) {
		t.Fatalf("prepare PASS attention identity = %+v, %v", request, err)
	}
	attention, err := PASSAttentionCommand(
		verdictCommandID, fixture.runID, fixture.workID, state.Revision,
	)
	if err != nil {
		t.Fatal(err)
	}
	before := takeVerifierSafetySnapshot(t, fixture)
	if result, err := fixture.control.ApplyControlledPASSAttention(
		ctx, fixture.ownership, fixture.ownerID, prepared, attention,
	); err == nil || !strings.Contains(err.Error(), "prepared state") {
		t.Fatalf("PASS attention reused verdict command identity = %+v, %v", result, err)
	}
	requireVerifierSafetyUnchanged(t, fixture, before)
}

func TestStoreVerifierUnboundInterruptionCannotRetryOrStartAnotherTurn(t *testing.T) {
	fixture := newVerifierLifecycleFixture(t)
	ctx := context.Background()
	fixture.dispatch(t, "cmd-unbound-interruption-dispatch")
	_, plan, request, err := fixture.control.PendingVerifierExecutionPermitRequest(
		ctx, fixture.ownership, fixture.ownerID, fixture.runID, fixture.workID,
	)
	if err != nil {
		t.Fatal(err)
	}
	permit, err := fixture.authority.AuthorizeVerifierExecution(ctx, plan, request)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := fixture.control.ClaimControlledVerifier(
		ctx, fixture.ownership, fixture.authority, permit, request,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.ownership.Close(); err != nil {
		t.Fatal(err)
	}
	fixture.ownership = nil
	const recoveryOwner = "verifier-unbound-recovery-controller"
	recovery, err := fixture.control.AcquireControllerOwnership(recoveryOwner)
	if err != nil {
		t.Fatal(err)
	}
	fixture.ownership, fixture.ownerID = recovery, recoveryOwner
	if recovered, err := fixture.control.RecoverControlledInterruptedEffects(
		ctx, recovery, recoveryOwner, "verifier stopped before binding a result",
	); err != nil || recovered != 1 {
		t.Fatalf("recover unbound interrupted verifier = %d, %v", recovered, err)
	}

	unknown, err := loadEffect(ctx, fixture.control.db, request.VerifierEffectID)
	if err != nil || unknown.State != EffectUnknown || unknown.Attempt != 1 || len(unknown.Result) != 0 {
		t.Fatalf("unbound verifier after interruption = %+v, %v", unknown, err)
	}
	unknownEffects, err := fixture.control.UnknownEffects(ctx)
	if err != nil || len(unknownEffects) != 1 || unknownEffects[0].ID != request.VerifierEffectID {
		t.Fatalf("unknown verifier projection = %+v, %v", unknownEffects, err)
	}
	before := takeVerifierSafetySnapshot(t, fixture)
	if _, err := fixture.control.ClaimNextEffect(ctx, "generic-verifier-worker"); !errors.Is(err, ErrNoPendingEffect) {
		t.Fatalf("unknown verifier was generically claimable: %v", err)
	}
	if _, err := fixture.control.PrepareAuthorizedVerifierExecution(ctx, lease); err == nil {
		t.Fatal("stale pre-interruption lease prepared another verifier turn")
	}
	if err := fixture.control.RecoverControlledBoundEffect(
		ctx, recovery, recoveryOwner, request.VerifierEffectID, 1,
	); err == nil || !strings.Contains(err.Error(), "no result bound") {
		t.Fatalf("unbound verifier recovered as bound result: %v", err)
	}
	if _, err := fixture.control.db.ExecContext(ctx, `
		UPDATE effects
		SET state = 'pending', owner_id = NULL, started_at_us = NULL,
		    completed_at_us = NULL, last_error = NULL
		WHERE effect_id = ?`, request.VerifierEffectID,
	); err == nil || !strings.Contains(err.Error(), "invalid effect transition") {
		t.Fatalf("unbound verifier was manually requeued: %v", err)
	}
	if err := recovery.Activate(ctx, fixture.control, recoveryOwner); err == nil ||
		!strings.Contains(err.Error(), "1 unresolved effects") {
		t.Fatalf("controller activated across unknown verifier: %v", err)
	}
	requireVerifierSafetyUnchanged(t, fixture, before)
	state, err := fixture.control.State(ctx, fixture.runID)
	if err != nil || state.Work[0].State != engine.WorkReviewable ||
		state.Work[0].VerdictBinding != (engine.VerdictBinding{}) {
		t.Fatalf("unknown verifier invented an outcome = %+v, %v", state, err)
	}
	assertCount(t, fixture.control, "verdict_records", 0)
}
