package gate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

const (
	receiptTestRelease = "2026-07-15-baton-v0.16-conformance"
	receiptTestSlice   = "S22-openrouter-tool-structured-output"
	receiptTestModel   = "openrouter/z-ai/glm-5.2"
	receiptTestStart   = "a09b0e46df465862d00469d4aef2a997442b3d5b"
)

func testProofReceiptBinding() ProofReceiptBinding {
	return ProofReceiptBinding{
		Release:              receiptTestRelease,
		SliceID:              receiptTestSlice,
		CheckType:            CheckSpecAmbiguity,
		ModelID:              receiptTestModel,
		ImmutableStartCommit: receiptTestStart,
	}
}

func testProofReceiptOutcome(class ProofReceiptAttemptClass, result ProofReceiptResult, exit int) ProofReceiptOutcome {
	return ProofReceiptOutcome{AttemptClass: class, Result: result, ProcessExitCode: ProofReceiptExitCode(exit)}
}

func writeTestReceipt(t *testing.T, dir string, receipt ProofReceipt) {
	t.Helper()
	if err := atomicWriteProofReceipt(proofReceiptPath(dir, receipt.Attempt), receipt); err != nil {
		t.Fatal(err)
	}
}

func writeTestReceiptJSON(t *testing.T, dir string, receipt ProofReceipt) {
	t.Helper()
	data, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(proofReceiptPath(dir, receipt.Attempt), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func readTestReceipt(t *testing.T, dir string, attempt int) ProofReceipt {
	t.Helper()
	receipt, exists, err := readProofReceipt(dir, attempt)
	if err != nil || !exists {
		t.Fatalf("receipt %d was not readable", attempt)
	}
	return receipt
}

func TestProofReceiptAtomicReservationAndFinalization(t *testing.T) {
	dir := t.TempDir()
	binding := testProofReceiptBinding()
	var calls atomic.Int32

	final, err := RunProofReceipt(context.Background(), binding, dir, func(context.Context) ProofReceiptOutcome {
		calls.Add(1)
		reserved := readTestReceipt(t, dir, 1)
		if reserved.AttemptClass != ProofReceiptReceiptFailure || reserved.Result != ProofReceiptUnparseable {
			t.Fatal("provider runner observed a non-conservative reservation")
		}
		if _, available := reserved.ProcessExitCode.Code(); available {
			t.Fatal("reservation claimed an unavailable process result was final")
		}
		return testProofReceiptOutcome(ProofReceiptFinalVerdict, ProofReceiptPass, 0)
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("runner calls = %d, want 1", calls.Load())
	}
	if final.Attempt != 1 || final.AttemptClass != ProofReceiptFinalVerdict || final.Result != ProofReceiptPass {
		t.Fatalf("final receipt = %#v", final)
	}

	data, err := os.ReadFile(proofReceiptPath(dir, 1))
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 11 {
		t.Fatalf("receipt field count = %d, want strict 11", len(fields))
	}
	for _, forbidden := range []string{"raw_response", "endpoint", "headers", "request", "response", "findings", "prompt", "diff", "key", "credential"} {
		if _, found := fields[forbidden]; found {
			t.Fatalf("receipt contains forbidden field %q", forbidden)
		}
	}
	if mode := mustFileMode(t, proofReceiptPath(dir, 1)); mode&0o077 != 0 {
		t.Fatalf("receipt permissions = %04o, want owner-only", mode)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".proof-receipt-") {
			t.Fatal("atomic temporary file remained after finalization")
		}
	}
}

func TestProofReceiptPreflightWriteFailureHasZeroDispatch(t *testing.T) {
	dir := t.TempDir()
	var calls atomic.Int32
	receipt, err := runProofReceipt(context.Background(), testProofReceiptBinding(), dir, func(context.Context) ProofReceiptOutcome {
		calls.Add(1)
		return testProofReceiptOutcome(ProofReceiptFinalVerdict, ProofReceiptPass, 0)
	}, func(string, ProofReceipt) error {
		return errors.New("test write fault")
	})
	if !errors.Is(err, ErrProofReceiptPreflight) {
		t.Fatalf("error = %v, want preflight rejection", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("runner calls = %d, want 0", calls.Load())
	}
	if receipt.Attempt != 1 || receipt.AttemptClass != ProofReceiptReceiptFailure || receipt.Result != ProofReceiptUnparseable {
		t.Fatalf("safe preflight result = %#v", receipt)
	}
	if _, err := os.Stat(proofReceiptPath(dir, 1)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed preflight left a non-atomic receipt")
	}
}

func TestProofReceiptFinalizationFailureRetainsReservation(t *testing.T) {
	dir := t.TempDir()
	var writes atomic.Int32
	var calls atomic.Int32
	receipt, err := runProofReceipt(context.Background(), testProofReceiptBinding(), dir, func(context.Context) ProofReceiptOutcome {
		calls.Add(1)
		return testProofReceiptOutcome(ProofReceiptFinalVerdict, ProofReceiptPass, 0)
	}, func(path string, value ProofReceipt) error {
		if writes.Add(1) == 2 {
			return errors.New("test finalization fault")
		}
		return atomicWriteProofReceipt(path, value)
	})
	if !errors.Is(err, ErrProofReceiptFinalization) {
		t.Fatalf("error = %v, want finalization failure", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("runner calls = %d, want 1", calls.Load())
	}
	if receipt.AttemptClass != ProofReceiptReceiptFailure || receipt.Result != ProofReceiptUnparseable {
		t.Fatalf("returned receipt invented a model verdict: %#v", receipt)
	}
	persisted := readTestReceipt(t, dir, 1)
	if persisted.AttemptClass != ProofReceiptReceiptFailure || persisted.Result != ProofReceiptUnparseable {
		t.Fatalf("persisted receipt invented a model verdict: %#v", persisted)
	}
}

func TestProofReceiptPostRenameSyncFailureRestoresReservation(t *testing.T) {
	dir := t.TempDir()
	var writes atomic.Int32
	var calls atomic.Int32
	receipt, err := runProofReceipt(context.Background(), testProofReceiptBinding(), dir, func(context.Context) ProofReceiptOutcome {
		calls.Add(1)
		return testProofReceiptOutcome(ProofReceiptFinalVerdict, ProofReceiptPass, 0)
	}, func(path string, value ProofReceipt) error {
		if writes.Add(1) == 2 {
			// This performs the real rename of a final PASS then simulates the
			// mandatory parent-directory sync fault after it.
			return atomicWriteProofReceiptWithSync(path, value, func(string) error {
				return errors.New("test post-rename sync fault")
			})
		}
		return atomicWriteProofReceipt(path, value)
	})
	if !errors.Is(err, ErrProofReceiptFinalization) || calls.Load() != 1 {
		t.Fatal("post-rename finalization fault did not fail closed")
	}
	if receipt.AttemptClass != ProofReceiptReceiptFailure || receipt.Result != ProofReceiptUnparseable {
		t.Fatal("returned post-rename result was trusted as a model verdict")
	}
	persisted := readTestReceipt(t, dir, 1)
	if persisted.AttemptClass != ProofReceiptReceiptFailure || persisted.Result != ProofReceiptUnparseable {
		t.Fatal("post-rename recovery left a trusted final receipt on disk")
	}
}

func TestProofReceiptPostRenameSyncFailureNeverTrustsFinalVerdict(t *testing.T) {
	dir := t.TempDir()
	var writes atomic.Int32
	var calls atomic.Int32
	receipt, err := runProofReceipt(context.Background(), testProofReceiptBinding(), dir, func(context.Context) ProofReceiptOutcome {
		calls.Add(1)
		return testProofReceiptOutcome(ProofReceiptFinalVerdict, ProofReceiptPass, 0)
	}, func(path string, value ProofReceipt) error {
		switch writes.Add(1) {
		case 2:
			// Rename a final PASS into place, then fail its required directory sync.
			return atomicWriteProofReceiptWithSync(path, value, func(string) error {
				return errors.New("test post-rename sync fault")
			})
		case 3:
			// Simulate the independent reservation-restoration fault.
			return errors.New("test reservation restoration fault")
		default:
			return atomicWriteProofReceipt(path, value)
		}
	})
	if !errors.Is(err, ErrProofReceiptFinalization) || calls.Load() != 1 {
		t.Fatal("double fault did not fail closed after exactly one dispatch")
	}
	if receipt.AttemptClass != ProofReceiptReceiptFailure || receipt.Result != ProofReceiptUnparseable {
		t.Fatalf("double fault surfaced a model verdict: %#v", receipt)
	}
	if code, available := receipt.ProcessExitCode.Code(); available || code != 0 {
		t.Fatalf("double fault exit semantics = (%d, %t), want unavailable", code, available)
	}
	if _, err := os.Stat(proofReceiptTrustGuardPath(dir, 1)); err != nil {
		t.Fatalf("durable trust guard missing after double fault: %v", err)
	}
	if _, _, err := readProofReceipt(dir, 1); !errors.Is(err, ErrProofReceiptPreflight) {
		t.Fatalf("later reader trusted unacknowledged renamed verdict: %v", err)
	}
	public := PrintProofReceipt(receipt) + JSONProofReceipt(receipt)
	if strings.Contains(public, "PASS") || !strings.Contains(public, "receipt_failure") || !strings.Contains(public, "UNPARSEABLE") || !strings.Contains(public, "unavailable") {
		t.Fatalf("double-fault output was not the sanitized receipt failure: %s", public)
	}
}

func TestProofReceiptConcurrentReservationHasOneWinner(t *testing.T) {
	dir := t.TempDir()
	binding := testProofReceiptBinding()
	started := make(chan struct{})
	allowFinish := make(chan struct{})
	firstDone := make(chan error, 1)
	var firstCalls atomic.Int32
	var secondCalls atomic.Int32

	go func() {
		_, err := RunProofReceipt(context.Background(), binding, dir, func(context.Context) ProofReceiptOutcome {
			firstCalls.Add(1)
			close(started)
			<-allowFinish
			return testProofReceiptOutcome(ProofReceiptFinalVerdict, ProofReceiptPass, 0)
		})
		firstDone <- err
	}()
	<-started
	_, err := RunProofReceipt(context.Background(), binding, dir, func(context.Context) ProofReceiptOutcome {
		secondCalls.Add(1)
		return testProofReceiptOutcome(ProofReceiptFinalVerdict, ProofReceiptPass, 0)
	})
	if !errors.Is(err, ErrProofReceiptPreflight) {
		t.Fatal("concurrent caller was not rejected before dispatch")
	}
	if secondCalls.Load() != 0 {
		t.Fatalf("losing concurrent caller dispatched %d requests", secondCalls.Load())
	}
	close(allowFinish)
	if err := <-firstDone; err != nil || firstCalls.Load() != 1 {
		t.Fatal("winning concurrent caller did not finalize exactly one receipt")
	}
	if _, err := os.Stat(filepath.Join(dir, ".proof-receipt.lock")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("receipt lock remained after winner finalized")
	}
}

func TestProofReceiptRejectsMalformedHistoryAndInvalidExitBeforeDispatch(t *testing.T) {
	binding := testProofReceiptBinding()
	tests := []struct {
		name string
		data func(t *testing.T) []byte
	}{
		{name: "malformed JSON", data: func(t *testing.T) []byte { return []byte(`{`) }},
		{name: "null process exit", data: func(t *testing.T) []byte {
			data, err := json.Marshal(proofReceiptReservation(binding, 1))
			if err != nil {
				t.Fatal(err)
			}
			return bytes.Replace(data, []byte(`"unavailable"`), []byte("null"), 1)
		}},
		{name: "absent process exit", data: func(t *testing.T) []byte {
			fields := map[string]any{}
			data, err := json.Marshal(proofReceiptReservation(binding, 1))
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(data, &fields); err != nil {
				t.Fatal(err)
			}
			delete(fields, "process_exit_code")
			data, err = json.Marshal(fields)
			if err != nil {
				t.Fatal(err)
			}
			return data
		}},
		{name: "contradictory final PASS exit", data: func(t *testing.T) []byte {
			receipt := proofReceiptReservation(binding, 1)
			receipt.AttemptClass = ProofReceiptFinalVerdict
			receipt.Result = ProofReceiptPass
			receipt.ProcessExitCode = ProofReceiptExitCode(1)
			data, err := json.Marshal(receipt)
			if err != nil {
				t.Fatal(err)
			}
			return data
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(proofReceiptPath(dir, 1), tt.data(t), 0o600); err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int32
			_, err := RunProofReceipt(context.Background(), binding, dir, func(context.Context) ProofReceiptOutcome {
				calls.Add(1)
				return testProofReceiptOutcome(ProofReceiptFinalVerdict, ProofReceiptPass, 0)
			})
			if !errors.Is(err, ErrProofReceiptPreflight) || calls.Load() != 0 {
				t.Fatal("invalid historical receipt reached a provider runner")
			}
		})
	}
}

func TestProofReceiptRejectsMismatchedBindingWithoutBudgetConsumption(t *testing.T) {
	binding := testProofReceiptBinding()
	tests := []struct {
		name  string
		apply func(*ProofReceipt)
	}{
		{name: "check", apply: func(r *ProofReceipt) { r.CheckType = CheckACSatisfaction }},
		{name: "model", apply: func(r *ProofReceipt) { r.ModelID = "openrouter/other" }},
		{name: "slice", apply: func(r *ProofReceipt) { r.SliceID = "S99-other" }},
		{name: "release", apply: func(r *ProofReceipt) { r.Release = "other-release" }},
		{name: "start", apply: func(r *ProofReceipt) { r.ImmutableStartCommit = strings.Repeat("b", 40) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			receipt := proofReceiptReservation(binding, 1)
			tt.apply(&receipt)
			writeTestReceipt(t, dir, receipt)
			before := receiptDirectorySnapshot(t, dir)
			var calls atomic.Int32
			_, err := RunProofReceipt(context.Background(), binding, dir, func(context.Context) ProofReceiptOutcome {
				calls.Add(1)
				return testProofReceiptOutcome(ProofReceiptFinalVerdict, ProofReceiptPass, 0)
			})
			if !errors.Is(err, ErrProofReceiptPreflight) {
				t.Fatal("mismatched receipt was accepted")
			}
			if calls.Load() != 0 {
				t.Fatalf("mismatched binding dispatched %d requests", calls.Load())
			}
			if after := receiptDirectorySnapshot(t, dir); after != before {
				t.Fatal("mismatched binding consumed or rewrote receipt budget")
			}
		})
	}
}

func TestProofReceiptFinalVerdictsNeverRetry(t *testing.T) {
	binding := testProofReceiptBinding()
	tests := []struct {
		name   string
		class  ProofReceiptAttemptClass
		result ProofReceiptResult
	}{
		{name: "PASS", class: ProofReceiptFinalVerdict, result: ProofReceiptPass},
		{name: "FAIL", class: ProofReceiptFinalVerdict, result: ProofReceiptFail},
		{name: "BLOCKED", class: ProofReceiptFinalVerdict, result: ProofReceiptBlocked},
		{name: "HTTP 400", class: ProofReceiptHTTPClientError, result: ProofReceiptUnparseable},
		{name: "parse", class: ProofReceiptParseFailure, result: ProofReceiptUnparseable},
		{name: "schema", class: ProofReceiptSchemaFailure, result: ProofReceiptUnparseable},
		{name: "identity", class: ProofReceiptIdentityMismatch, result: ProofReceiptUnparseable},
		{name: "malformed tool", class: ProofReceiptMalformedTool, result: ProofReceiptUnparseable},
		{name: "opaque", class: ProofReceiptOpaque, result: ProofReceiptUnparseable},
		{name: "untrusted", class: ProofReceiptUntrustedBinding, result: ProofReceiptUnparseable},
		{name: "unknown", class: ProofReceiptUnknown, result: ProofReceiptUnparseable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			receipt := proofReceiptReservation(binding, 1)
			receipt.AttemptClass = tt.class
			receipt.Result = tt.result
			if tt.class == ProofReceiptFinalVerdict {
				receipt.ProcessExitCode = ProofReceiptExitCode(1)
			}
			writeTestReceipt(t, dir, receipt)
			var calls atomic.Int32
			_, err := RunProofReceipt(context.Background(), binding, dir, func(context.Context) ProofReceiptOutcome {
				calls.Add(1)
				return testProofReceiptOutcome(ProofReceiptFinalVerdict, ProofReceiptPass, 0)
			})
			if !errors.Is(err, ErrProofReceiptPreflight) {
				t.Fatal("terminal receipt permitted a retry")
			}
			if calls.Load() != 0 {
				t.Fatalf("terminal receipt dispatched %d requests", calls.Load())
			}
		})
	}
}

func TestProofReceiptOpaqueAndContractFailuresDoNotRetry(t *testing.T) {
	for _, class := range []ProofReceiptAttemptClass{
		ProofReceiptHTTPClientError,
		ProofReceiptParseFailure,
		ProofReceiptSchemaFailure,
		ProofReceiptIdentityMismatch,
		ProofReceiptMalformedTool,
		ProofReceiptOpaque,
		ProofReceiptUntrustedBinding,
		ProofReceiptUnknown,
	} {
		t.Run(string(class), func(t *testing.T) {
			dir := t.TempDir()
			first := proofReceiptReservation(testProofReceiptBinding(), 1)
			first.AttemptClass = class
			first.ProcessExitCode = ProofReceiptExitCode(2)
			writeTestReceipt(t, dir, first)

			var calls atomic.Int32
			_, err := RunProofReceipt(context.Background(), testProofReceiptBinding(), dir, func(context.Context) ProofReceiptOutcome {
				calls.Add(1)
				return testProofReceiptOutcome(ProofReceiptFinalVerdict, ProofReceiptPass, 0)
			})
			if !errors.Is(err, ErrProofReceiptPreflight) {
				t.Fatalf("class %q error = %v, want preflight rejection", class, err)
			}
			if calls.Load() != 0 {
				t.Fatalf("class %q dispatched %d retries", class, calls.Load())
			}
		})
	}
}

func TestProofReceiptRetryableAttemptPermitsOnlyAttemptTwo(t *testing.T) {
	binding := testProofReceiptBinding()
	for _, class := range []ProofReceiptAttemptClass{
		ProofReceiptRateLimit, ProofReceiptUpstream, ProofReceiptTransient, ProofReceiptNetwork,
		ProofReceiptDeadline, ProofReceiptRunnerFailure, ProofReceiptReceiptFailure,
	} {
		t.Run(string(class), func(t *testing.T) {
			dir := t.TempDir()
			first := proofReceiptReservation(binding, 1)
			first.AttemptClass = class
			first.ProcessExitCode = ProofReceiptExitCode(2)
			writeTestReceipt(t, dir, first)

			var calls atomic.Int32
			second, err := RunProofReceipt(context.Background(), binding, dir, func(context.Context) ProofReceiptOutcome {
				calls.Add(1)
				return testProofReceiptOutcome(ProofReceiptFinalVerdict, ProofReceiptPass, 0)
			})
			if err != nil {
				t.Fatal(err)
			}
			if second.Attempt != 2 || second.Result != ProofReceiptPass || calls.Load() != 1 {
				t.Fatal("retryable first receipt did not produce exactly attempt two")
			}
			_, err = RunProofReceipt(context.Background(), binding, dir, func(context.Context) ProofReceiptOutcome {
				calls.Add(1)
				return testProofReceiptOutcome(ProofReceiptFinalVerdict, ProofReceiptPass, 0)
			})
			if !errors.Is(err, ErrProofReceiptPreflight) || calls.Load() != 1 {
				t.Fatal("attempt two permitted a third dispatch")
			}
		})
	}
}

func TestProofReceiptConfiguredRecoveryWritesVersionTwoReceipt(t *testing.T) {
	dir := t.TempDir()
	binding := testProofReceiptBinding()
	recoveryBinding := binding
	recoveryBinding.ModelID = "openrouter/configured-recovery-model"
	first := proofReceiptReservation(binding, 1)
	first.AttemptClass = ProofReceiptReceiptFailure
	first.ProcessExitCode = ProofReceiptExitUnavailable()
	writeTestReceipt(t, dir, first)

	second := proofReceiptReservation(binding, 2)
	second.AttemptClass = ProofReceiptOpaque
	second.ProcessExitCode = ProofReceiptExitCode(2)
	writeTestReceipt(t, dir, second)

	var calls atomic.Int32
	receipt, err := RunConfiguredRecoveryProofReceipt(context.Background(), binding, recoveryBinding, dir, func(context.Context) ProofReceiptOutcome {
		calls.Add(1)
		return testProofReceiptOutcome(ProofReceiptFinalVerdict, ProofReceiptPass, 0)
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("configured recovery dispatches = %d, want 1", calls.Load())
	}
	if receipt.Attempt != 3 || receipt.RecordVersion != 2 || receipt.Schema != ProofReceiptSchemaV2 {
		t.Fatalf("configured recovery receipt = %#v", receipt)
	}
	if receipt.ModelID != recoveryBinding.ModelID {
		t.Fatalf("configured recovery model = %q, want %q", receipt.ModelID, recoveryBinding.ModelID)
	}
	if persisted, _, err := readProofReceipt(dir, 3); err != nil || persisted.RecordVersion != 2 || persisted.Schema != ProofReceiptSchemaV2 || persisted.Attempt != 3 {
		t.Fatalf("persisted attempt-3 receipt = %#v, err %v", persisted, err)
	}
}

func TestProofReceiptConfiguredRecoveryRequiresExactAttemptOneAndTwo(t *testing.T) {
	binding := testProofReceiptBinding()
	recoveryBinding := binding
	recoveryBinding.ModelID = "openrouter/configured-recovery-model"
	attemptOne := proofReceiptReservation(binding, 1)
	attemptOne.AttemptClass = ProofReceiptReceiptFailure
	attemptTwo := proofReceiptReservation(binding, 2)
	attemptTwo.AttemptClass = ProofReceiptOpaque
	attemptTwo.ProcessExitCode = ProofReceiptExitCode(2)

	for _, tt := range []struct {
		name  string
		setup func(t *testing.T, dir string, attemptOne, attemptTwo ProofReceipt)
	}{
		{
			name:  "missing attempt one",
			setup: func(*testing.T, string, ProofReceipt, ProofReceipt) {},
		},
		{
			name: "attempt one not configured",
			setup: func(t *testing.T, dir string, attemptOne, _ ProofReceipt) {
				attemptOne.AttemptClass = ProofReceiptFinalVerdict
				attemptOne.ProcessExitCode = ProofReceiptExitCode(0)
				writeTestReceiptJSON(t, dir, attemptOne)
			},
		},
		{
			name: "attempt one exit became available",
			setup: func(t *testing.T, dir string, attemptOne, _ ProofReceipt) {
				attemptOne.AttemptClass = ProofReceiptReceiptFailure
				attemptOne.ProcessExitCode = ProofReceiptExitCode(2)
				writeTestReceiptJSON(t, dir, attemptOne)
			},
		},
		{
			name: "missing attempt two",
			setup: func(t *testing.T, dir string, attemptOne, _ ProofReceipt) {
				writeTestReceiptJSON(t, dir, attemptOne)
			},
		},
		{
			name: "attempt two terminal",
			setup: func(t *testing.T, dir string, attemptOne, attemptTwo ProofReceipt) {
				writeTestReceiptJSON(t, dir, attemptOne)
				attemptTwo.AttemptClass = ProofReceiptFinalVerdict
				attemptTwo.Result = ProofReceiptPass
				attemptTwo.ProcessExitCode = ProofReceiptExitCode(0)
				writeTestReceiptJSON(t, dir, attemptTwo)
			},
		},
		{
			name: "attempt three already present",
			setup: func(t *testing.T, dir string, attemptOne, attemptTwo ProofReceipt) {
				writeTestReceiptJSON(t, dir, attemptOne)
				writeTestReceiptJSON(t, dir, attemptTwo)
				receipt3 := proofReceiptReservation(recoveryBinding, 3)
				receipt3.AttemptClass = ProofReceiptFinalVerdict
				receipt3.Result = ProofReceiptPass
				receipt3.ProcessExitCode = ProofReceiptExitCode(0)
				writeTestReceiptJSON(t, dir, receipt3)
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tt.setup(t, dir, attemptOne, attemptTwo)
			before := receiptDirectorySnapshot(t, dir)
			var calls atomic.Int32
			_, err := RunConfiguredRecoveryProofReceipt(context.Background(), binding, recoveryBinding, dir, func(context.Context) ProofReceiptOutcome {
				calls.Add(1)
				return testProofReceiptOutcome(ProofReceiptFinalVerdict, ProofReceiptPass, 0)
			})
			if !errors.Is(err, ErrProofReceiptPreflight) {
				t.Fatalf("configured recovery accepted invalid history")
			}
			if calls.Load() != 0 {
				t.Fatalf("configured recovery used provider dispatch for invalid history")
			}
			if after := receiptDirectorySnapshot(t, dir); before != after {
				t.Fatal("configured-recovery preflight consumed or rewrote receipt budget")
			}
		})
	}
}

func TestProofReceiptConfiguredRecoveryRejectsPerFieldHistoryMutation(t *testing.T) {
	historical := testProofReceiptBinding()
	recovery := historical
	recovery.ModelID = "openrouter/example/recovery"
	first := proofReceiptReservation(historical, 1)
	first.AttemptClass = ProofReceiptReceiptFailure
	first.ProcessExitCode = ProofReceiptExitUnavailable()
	second := proofReceiptReservation(historical, 2)
	second.AttemptClass = ProofReceiptOpaque
	second.ProcessExitCode = ProofReceiptExitCode(2)
	tests := []struct {
		name   string
		mutate func(*ProofReceipt, *ProofReceipt)
	}{
		{name: "release", mutate: func(a, _ *ProofReceipt) { a.Release = "other" }},
		{name: "slice", mutate: func(a, _ *ProofReceipt) { a.SliceID = "S99-other" }},
		{name: "check", mutate: func(a, _ *ProofReceipt) { a.CheckType = CheckSecurityReview }},
		{name: "model", mutate: func(a, _ *ProofReceipt) { a.ModelID = "openrouter/other" }},
		{name: "start", mutate: func(a, _ *ProofReceipt) { a.ImmutableStartCommit = strings.Repeat("b", 40) }},
		{name: "attempt one ordinal", mutate: func(a, _ *ProofReceipt) { a.Attempt = 2 }},
		{name: "attempt one class", mutate: func(a, _ *ProofReceipt) { a.AttemptClass = ProofReceiptOpaque }},
		{name: "attempt one result", mutate: func(a, _ *ProofReceipt) { a.Result = ProofReceiptPass }},
		{name: "attempt one exit", mutate: func(a, _ *ProofReceipt) { a.ProcessExitCode = ProofReceiptExitCode(2) }},
		{name: "attempt two release", mutate: func(_, b *ProofReceipt) { b.Release = "other" }},
		{name: "attempt two model", mutate: func(_, b *ProofReceipt) { b.ModelID = "openrouter/other" }},
		{name: "attempt two ordinal", mutate: func(_, b *ProofReceipt) { b.Attempt = 1 }},
		{name: "attempt two class", mutate: func(_, b *ProofReceipt) { b.AttemptClass = ProofReceiptUpstream }},
		{name: "attempt two result", mutate: func(_, b *ProofReceipt) { b.Result = ProofReceiptPass }},
		{name: "attempt two exit", mutate: func(_, b *ProofReceipt) { b.ProcessExitCode = ProofReceiptExitCode(1) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			a, b := first, second
			tt.mutate(&a, &b)
			for ordinal, receipt := range map[int]ProofReceipt{1: a, 2: b} {
				data, err := json.Marshal(receipt)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(proofReceiptPath(dir, ordinal), data, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			var calls atomic.Int32
			_, err := RunConfiguredRecoveryProofReceipt(context.Background(), historical, recovery, dir, func(context.Context) ProofReceiptOutcome {
				calls.Add(1)
				return testProofReceiptOutcome(ProofReceiptFinalVerdict, ProofReceiptPass, 0)
			})
			if !errors.Is(err, ErrProofReceiptPreflight) || calls.Load() != 0 {
				t.Fatalf("mutated history err=%v calls=%d", err, calls.Load())
			}
		})
	}
}

func TestProofReceiptVersionTwoMatchesPlannerSchema(t *testing.T) {
	path := filepath.Join("..", "..", "docs", "release", receiptTestRelease, receiptTestSlice, "llm-check-proof-receipt-v2.schema.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		AdditionalProperties bool                       `json:"additionalProperties"`
		Required             []string                   `json:"required"`
		Properties           map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	wantFields := []string{"$schema", "record_version", "release", "slice_id", "check_type", "model_id", "immutable_start_commit", "attempt", "attempt_class", "result", "process_exit_code"}
	if schema.AdditionalProperties || len(schema.Required) != len(wantFields) || len(schema.Properties) != len(wantFields) {
		t.Fatalf("v2 schema shape drifted: required=%d properties=%d additional=%v", len(schema.Required), len(schema.Properties), schema.AdditionalProperties)
	}
	for _, field := range wantFields {
		if _, ok := schema.Properties[field]; !ok || !containsString(schema.Required, field) {
			t.Fatalf("v2 schema missing required field %q", field)
		}
	}
	binding := testProofReceiptBinding()
	binding.ModelID = "openrouter/example/recovery"
	receipt := proofReceiptReservation(binding, 3)
	rendered := []byte(JSONProofReceipt(receipt))
	decoded, err := decodeProofReceipt(rendered)
	if err != nil || decoded != receipt {
		t.Fatalf("rendered v2 receipt failed strict decode: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(rendered, &fields); err != nil || len(fields) != len(wantFields) {
		t.Fatalf("rendered v2 fields drifted: %v", err)
	}
	var schemaConst string
	var versionConst, attemptConst int
	if json.Unmarshal(schema.Properties["$schema"], &struct {
		Const *string `json:"const"`
	}{Const: &schemaConst}) != nil ||
		json.Unmarshal(schema.Properties["record_version"], &struct {
			Const *int `json:"const"`
		}{Const: &versionConst}) != nil ||
		json.Unmarshal(schema.Properties["attempt"], &struct {
			Const *int `json:"const"`
		}{Const: &attemptConst}) != nil ||
		schemaConst != receipt.Schema || versionConst != receipt.RecordVersion || attemptConst != receipt.Attempt {
		t.Fatal("v2 schema constants do not match rendered receipt")
	}
	var classRule, resultRule struct {
		Enum []string `json:"enum"`
	}
	if json.Unmarshal(schema.Properties["attempt_class"], &classRule) != nil ||
		json.Unmarshal(schema.Properties["result"], &resultRule) != nil {
		t.Fatal("v2 schema enums are unreadable")
	}
	wantClasses := []ProofReceiptAttemptClass{
		ProofReceiptFinalVerdict, ProofReceiptRateLimit, ProofReceiptUpstream, ProofReceiptTransient,
		ProofReceiptNetwork, ProofReceiptDeadline, ProofReceiptRunnerFailure, ProofReceiptReceiptFailure,
		ProofReceiptHTTPClientError, ProofReceiptParseFailure, ProofReceiptSchemaFailure,
		ProofReceiptIdentityMismatch, ProofReceiptMalformedTool, ProofReceiptOpaque,
		ProofReceiptUntrustedBinding, ProofReceiptUnknown,
	}
	if len(classRule.Enum) != len(wantClasses) {
		t.Fatalf("v2 attempt_class enum count = %d, want %d", len(classRule.Enum), len(wantClasses))
	}
	for _, class := range wantClasses {
		if !containsString(classRule.Enum, string(class)) {
			t.Fatalf("v2 schema missing attempt class %q", class)
		}
	}
	for _, result := range []ProofReceiptResult{ProofReceiptPass, ProofReceiptFail, ProofReceiptBlocked, ProofReceiptUnparseable} {
		if !containsString(resultRule.Enum, string(result)) {
			t.Fatalf("v2 schema missing result %q", result)
		}
	}
	withExtra := bytes.TrimSuffix(rendered, []byte("\n"))
	withExtra = append(bytes.TrimSuffix(withExtra, []byte("}")), []byte(`,"extra":true}`)...)
	if _, err := decodeProofReceipt(withExtra); !errors.Is(err, ErrProofReceiptPreflight) {
		t.Fatal("v2 decoder accepted a field prohibited by the Planner schema")
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestProofReceiptDecodeEnforcesAttemptSchemaPairs(t *testing.T) {
	binding := testProofReceiptBinding()
	validV1 := proofReceiptReservation(binding, 1)
	validV2 := proofReceiptReservation(binding, 3)

	t.Run("valid v1 attempt", func(t *testing.T) {
		dir := t.TempDir()
		writeTestReceiptJSON(t, dir, validV1)
		if _, exists, err := readProofReceipt(dir, 1); err != nil || !exists {
			t.Fatalf("v1 attempt 1 was not readable: exists=%v err=%v", exists, err)
		}
	})
	t.Run("valid v2 attempt", func(t *testing.T) {
		dir := t.TempDir()
		writeTestReceiptJSON(t, dir, validV2)
		if _, exists, err := readProofReceipt(dir, 3); err != nil || !exists {
			t.Fatalf("v2 attempt 3 was not readable: exists=%v err=%v", exists, err)
		}
	})
	t.Run("schema-version mismatch", func(t *testing.T) {
		cases := []ProofReceipt{
			func() ProofReceipt {
				r := proofReceiptReservation(binding, 1)
				r.Schema = ProofReceiptSchemaV2
				return r
			}(),
			func() ProofReceipt {
				r := proofReceiptReservation(binding, 3)
				r.RecordVersion = 1
				return r
			}(),
		}
		for _, receipt := range cases {
			dir := t.TempDir()
			writeTestReceiptJSON(t, dir, receipt)
			if _, _, err := readProofReceipt(dir, receipt.Attempt); !errors.Is(err, ErrProofReceiptPreflight) {
				t.Fatalf("mismatched schema/version pair was accepted: %+v", receipt)
			}
		}
	})
}

func TestProofReceiptSpecAmbiguityOutcomesAreFailClosed(t *testing.T) {
	valid := `{"$schema":"https://baton.sawy3r.net/schemas/spec-ambiguity-report-v1.json","schema_version":1,"check":"spec-ambiguity","slice_id":"S22-openrouter-tool-structured-output","release":"2026-07-15-baton-v0.16-conformance","verdict":"PASS","blocking_findings":{},"advisory_findings":{}}`
	tests := []struct {
		name      string
		raw       string
		wantClass ProofReceiptAttemptClass
		want      ProofReceiptResult
	}{
		{name: "valid PASS", raw: valid, wantClass: ProofReceiptFinalVerdict, want: ProofReceiptPass},
		{name: "parse", raw: "not-json", wantClass: ProofReceiptParseFailure, want: ProofReceiptUnparseable},
		{name: "schema", raw: `{"check":"spec-ambiguity"}`, wantClass: ProofReceiptSchemaFailure, want: ProofReceiptUnparseable},
		{name: "identity", raw: strings.Replace(valid, receiptTestSlice, "S99-other", 1), wantClass: ProofReceiptIdentityMismatch, want: ProofReceiptUnparseable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := proofReceiptSpecAmbiguityOutcome(tt.raw, receiptTestSlice, receiptTestRelease)
			if got.AttemptClass != tt.wantClass || got.Result != tt.want {
				t.Fatalf("outcome = %#v", got)
			}
		})
	}
}

func TestProofReceiptLeakCanaries(t *testing.T) {
	dir := t.TempDir()
	binding := testProofReceiptBinding()
	receipt, err := RunProofReceipt(context.Background(), binding, dir, func(context.Context) ProofReceiptOutcome {
		return testProofReceiptOutcome(ProofReceiptFinalVerdict, ProofReceiptPass, 0)
	})
	if err != nil {
		t.Fatal(err)
	}
	public := PrintProofReceipt(receipt) + JSONProofReceipt(receipt)
	persisted, err := os.ReadFile(proofReceiptPath(dir, 1))
	if err != nil {
		t.Fatal(err)
	}
	for _, canary := range []string{"S22-RAW-RESPONSE-CANARY", "S22-REQUEST-CANARY", "S22-KEY-CANARY", "https://endpoint.invalid"} {
		if strings.Contains(public, canary) || strings.Contains(string(persisted), canary) {
			t.Fatal("receipt renderer leaked a protected canary")
		}
	}
}

func receiptDirectorySnapshot(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot strings.Builder
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		snapshot.WriteString(entry.Name())
		snapshot.WriteByte(0)
		snapshot.Write(data)
		snapshot.WriteByte('\n')
	}
	return snapshot.String()
}

func mustFileMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}
