package gate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/baton/schemas"
	"github.com/swornagent/sworn/internal/model"
	"github.com/swornagent/sworn/internal/project"
	"github.com/swornagent/sworn/internal/prompt"
	"github.com/swornagent/sworn/internal/spec"
)

// ProofReceiptSchema is the identity of the deliberately separate, strict
// receipt record. A proof receipt is never an LLMCheckReport.
const ProofReceiptSchema = "https://swornagent.dev/schemas/llm-check-proof-receipt-v1.json"

// ProofReceiptAttemptClass is the complete receipt classification vocabulary.
// Values are metadata only; no class may contain an error message or payload.
type ProofReceiptAttemptClass string

const (
	ProofReceiptFinalVerdict     ProofReceiptAttemptClass = "final_verdict"
	ProofReceiptRateLimit        ProofReceiptAttemptClass = "rate_limit"
	ProofReceiptUpstream         ProofReceiptAttemptClass = "upstream"
	ProofReceiptTransient        ProofReceiptAttemptClass = "transient"
	ProofReceiptNetwork          ProofReceiptAttemptClass = "network"
	ProofReceiptDeadline         ProofReceiptAttemptClass = "deadline"
	ProofReceiptRunnerFailure    ProofReceiptAttemptClass = "runner_failure"
	ProofReceiptReceiptFailure   ProofReceiptAttemptClass = "receipt_failure"
	ProofReceiptHTTPClientError  ProofReceiptAttemptClass = "http_client_error"
	ProofReceiptParseFailure     ProofReceiptAttemptClass = "parse_failure"
	ProofReceiptSchemaFailure    ProofReceiptAttemptClass = "schema_failure"
	ProofReceiptIdentityMismatch ProofReceiptAttemptClass = "identity_mismatch"
	ProofReceiptMalformedTool    ProofReceiptAttemptClass = "malformed_tool"
	ProofReceiptOpaque           ProofReceiptAttemptClass = "opaque"
	ProofReceiptUntrustedBinding ProofReceiptAttemptClass = "untrusted_binding"
	ProofReceiptUnknown          ProofReceiptAttemptClass = "unknown"
)

// ProofReceiptResult is deliberately limited to the terminal public result
// vocabulary. Non-final outcomes use UNPARSEABLE rather than inventing a model
// verdict.
type ProofReceiptResult string

const (
	ProofReceiptPass        ProofReceiptResult = "PASS"
	ProofReceiptFail        ProofReceiptResult = "FAIL"
	ProofReceiptBlocked     ProofReceiptResult = "BLOCKED"
	ProofReceiptUnparseable ProofReceiptResult = "UNPARSEABLE"
)

// ProofReceiptExit is encoded exactly as the receipt schema permits: a process
// exit code or the explicit unavailable sentinel for a durable reservation
// whose finalization could not be persisted.
type ProofReceiptExit struct {
	code        int
	unavailable bool
}

// ProofReceiptExitCode constructs an available process-exit value.
func ProofReceiptExitCode(code int) ProofReceiptExit { return ProofReceiptExit{code: code} }

// ProofReceiptExitUnavailable constructs the explicit unavailable sentinel.
func ProofReceiptExitUnavailable() ProofReceiptExit { return ProofReceiptExit{unavailable: true} }

// Code reports the numeric process exit and whether it was durably available.
func (e ProofReceiptExit) Code() (int, bool) { return e.code, !e.unavailable }

func (e ProofReceiptExit) String() string {
	if e.unavailable {
		return "unavailable"
	}
	return fmt.Sprintf("%d", e.code)
}

func (e ProofReceiptExit) MarshalJSON() ([]byte, error) {
	if e.unavailable {
		return []byte(`"unavailable"`), nil
	}
	if e.code < 0 || e.code > 255 {
		return nil, errors.New("invalid proof receipt process exit")
	}
	return []byte(fmt.Sprintf("%d", e.code)), nil
}

func (e *ProofReceiptExit) UnmarshalJSON(data []byte) error {
	var code int
	if err := json.Unmarshal(data, &code); err == nil {
		if code < 0 || code > 255 {
			return errors.New("invalid proof receipt process exit")
		}
		*e = ProofReceiptExitCode(code)
		return nil
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil || value != "unavailable" {
		return errors.New("invalid proof receipt process exit")
	}
	*e = ProofReceiptExitUnavailable()
	return nil
}

// ProofReceipt is the strict metadata-only durable evidence record.
type ProofReceipt struct {
	Schema               string                   `json:"$schema"`
	RecordVersion        int                      `json:"record_version"`
	Release              string                   `json:"release"`
	SliceID              string                   `json:"slice_id"`
	CheckType            CheckType                `json:"check_type"`
	ModelID              string                   `json:"model_id"`
	ImmutableStartCommit string                   `json:"immutable_start_commit"`
	Attempt              int                      `json:"attempt"`
	AttemptClass         ProofReceiptAttemptClass `json:"attempt_class"`
	Result               ProofReceiptResult       `json:"result"`
	ProcessExitCode      ProofReceiptExit         `json:"process_exit_code"`
}

// ProofReceiptBinding is the immutable identity checked before a reservation
// can be written or a provider request can be issued.
type ProofReceiptBinding struct {
	Release              string
	SliceID              string
	CheckType            CheckType
	ModelID              string
	ImmutableStartCommit string
}

// ProofReceiptOutcome contains only the sanitised classification returned by
// the one structured provider call. It intentionally has no error or payload.
type ProofReceiptOutcome struct {
	AttemptClass    ProofReceiptAttemptClass
	Result          ProofReceiptResult
	ProcessExitCode ProofReceiptExit
}

// ProofReceiptRunner performs exactly one already-prepared structured call.
// Preflight and atomic reservation happen before it is invoked.
type ProofReceiptRunner func(context.Context) ProofReceiptOutcome

var (
	// ErrProofReceiptPreflight is intentionally source-free so a bad path,
	// malformed record, or binding mismatch cannot leak through CLI stderr.
	ErrProofReceiptPreflight = errors.New("proof receipt preflight rejected")
	// ErrProofReceiptFinalization means the durable reservation remains as the
	// conservative receipt_failure record; no model verdict is inferred.
	ErrProofReceiptFinalization = errors.New("proof receipt finalization failed")
)

// RunProofReceipt validates existing metadata, reserves an ordinal atomically,
// invokes the runner once, then atomically finalizes the same receipt. It never
// accepts a raw response, request body, endpoint, credential, or error text.
func RunProofReceipt(ctx context.Context, binding ProofReceiptBinding, receiptDir string, run ProofReceiptRunner) (ProofReceipt, error) {
	return runProofReceipt(ctx, binding, receiptDir, run, atomicWriteProofReceipt)
}

type proofReceiptWriter func(string, ProofReceipt) error

func runProofReceipt(ctx context.Context, binding ProofReceiptBinding, receiptDir string, run ProofReceiptRunner, write proofReceiptWriter) (ProofReceipt, error) {
	attempt, err := preflightProofReceipt(binding, receiptDir)
	if err != nil {
		return ProofReceipt{}, ErrProofReceiptPreflight
	}

	path := proofReceiptPath(receiptDir, attempt)
	reservation := proofReceiptReservation(binding, attempt)
	if err := write(path, reservation); err != nil {
		return reservation, ErrProofReceiptPreflight
	}

	outcome := normaliseProofReceiptOutcome(run(ctx))
	final := reservation
	final.AttemptClass = outcome.AttemptClass
	final.Result = outcome.Result
	final.ProcessExitCode = outcome.ProcessExitCode
	if err := write(path, final); err != nil {
		// The first atomic write is already a valid receipt_failure reservation.
		// Returning it is the only safe representation of a failed finalization.
		return reservation, ErrProofReceiptFinalization
	}
	return final, nil
}

func preflightProofReceipt(binding ProofReceiptBinding, receiptDir string) (int, error) {
	if !validProofReceiptBinding(binding) {
		return 0, ErrProofReceiptPreflight
	}
	info, err := os.Stat(receiptDir)
	if err != nil || !info.IsDir() {
		return 0, ErrProofReceiptPreflight
	}

	first, firstExists, err := readProofReceipt(receiptDir, 1)
	if err != nil {
		return 0, ErrProofReceiptPreflight
	}
	second, secondExists, err := readProofReceipt(receiptDir, 2)
	if err != nil {
		return 0, ErrProofReceiptPreflight
	}
	if !firstExists && secondExists {
		return 0, ErrProofReceiptPreflight
	}
	if firstExists && !receiptMatchesBinding(first, binding, 1) {
		return 0, ErrProofReceiptPreflight
	}
	if secondExists && !receiptMatchesBinding(second, binding, 2) {
		return 0, ErrProofReceiptPreflight
	}
	if secondExists {
		return 0, ErrProofReceiptPreflight
	}
	if !firstExists {
		return 1, nil
	}
	if !proofReceiptRetryable(first.AttemptClass) {
		return 0, ErrProofReceiptPreflight
	}
	return 2, nil
}

func validProofReceiptBinding(binding ProofReceiptBinding) bool {
	return strings.TrimSpace(binding.Release) != "" &&
		strings.TrimSpace(binding.SliceID) != "" &&
		ValidCheckTypes[binding.CheckType] && !IsRetiredLLMCheck(binding.CheckType) &&
		strings.TrimSpace(binding.ModelID) != "" &&
		validProofReceiptCommit(binding.ImmutableStartCommit)
}

func validProofReceiptCommit(commit string) bool {
	if len(commit) != 40 && len(commit) != 64 {
		return false
	}
	for _, c := range commit {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func proofReceiptPath(dir string, attempt int) string {
	return filepath.Join(dir, fmt.Sprintf("attempt-%d.json", attempt))
}

func readProofReceipt(dir string, attempt int) (ProofReceipt, bool, error) {
	data, err := os.ReadFile(proofReceiptPath(dir, attempt))
	if errors.Is(err, os.ErrNotExist) {
		return ProofReceipt{}, false, nil
	}
	if err != nil {
		return ProofReceipt{}, false, ErrProofReceiptPreflight
	}
	receipt, err := decodeProofReceipt(data)
	if err != nil {
		return ProofReceipt{}, false, ErrProofReceiptPreflight
	}
	return receipt, true, nil
}

func decodeProofReceipt(data []byte) (ProofReceipt, error) {
	var receipt ProofReceipt
	if err := spec.DecodeJSONNoDuplicate(data, &receipt); err != nil {
		return ProofReceipt{}, ErrProofReceiptPreflight
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil || len(fields) != 11 {
		return ProofReceipt{}, ErrProofReceiptPreflight
	}
	for _, key := range []string{"$schema", "record_version", "release", "slice_id", "check_type", "model_id", "immutable_start_commit", "attempt", "attempt_class", "result", "process_exit_code"} {
		if _, ok := fields[key]; !ok {
			return ProofReceipt{}, ErrProofReceiptPreflight
		}
	}
	if receipt.Schema != ProofReceiptSchema || receipt.RecordVersion != 1 ||
		strings.TrimSpace(receipt.Release) == "" || strings.TrimSpace(receipt.SliceID) == "" ||
		!ValidCheckTypes[receipt.CheckType] || IsRetiredLLMCheck(receipt.CheckType) ||
		strings.TrimSpace(receipt.ModelID) == "" || !validProofReceiptCommit(receipt.ImmutableStartCommit) ||
		(receipt.Attempt != 1 && receipt.Attempt != 2) || !validProofReceiptAttemptClass(receipt.AttemptClass) ||
		!validProofReceiptResult(receipt.Result) {
		return ProofReceipt{}, ErrProofReceiptPreflight
	}
	if receipt.AttemptClass == ProofReceiptFinalVerdict {
		if receipt.Result != ProofReceiptPass && receipt.Result != ProofReceiptFail && receipt.Result != ProofReceiptBlocked {
			return ProofReceipt{}, ErrProofReceiptPreflight
		}
	} else if receipt.Result != ProofReceiptUnparseable {
		return ProofReceipt{}, ErrProofReceiptPreflight
	}
	if code, available := receipt.ProcessExitCode.Code(); available && (code < 0 || code > 255) {
		return ProofReceipt{}, ErrProofReceiptPreflight
	}
	return receipt, nil
}

func receiptMatchesBinding(receipt ProofReceipt, binding ProofReceiptBinding, attempt int) bool {
	return receipt.Attempt == attempt && receipt.Release == binding.Release && receipt.SliceID == binding.SliceID &&
		receipt.CheckType == binding.CheckType && receipt.ModelID == binding.ModelID &&
		receipt.ImmutableStartCommit == binding.ImmutableStartCommit
}

func proofReceiptReservation(binding ProofReceiptBinding, attempt int) ProofReceipt {
	return ProofReceipt{
		Schema:               ProofReceiptSchema,
		RecordVersion:        1,
		Release:              binding.Release,
		SliceID:              binding.SliceID,
		CheckType:            binding.CheckType,
		ModelID:              binding.ModelID,
		ImmutableStartCommit: binding.ImmutableStartCommit,
		Attempt:              attempt,
		AttemptClass:         ProofReceiptReceiptFailure,
		Result:               ProofReceiptUnparseable,
		ProcessExitCode:      ProofReceiptExitUnavailable(),
	}
}

func normaliseProofReceiptOutcome(outcome ProofReceiptOutcome) ProofReceiptOutcome {
	if !validProofReceiptAttemptClass(outcome.AttemptClass) {
		return ProofReceiptOutcome{AttemptClass: ProofReceiptUnknown, Result: ProofReceiptUnparseable, ProcessExitCode: ProofReceiptExitCode(2)}
	}
	if outcome.AttemptClass == ProofReceiptFinalVerdict {
		if outcome.Result == ProofReceiptPass || outcome.Result == ProofReceiptFail || outcome.Result == ProofReceiptBlocked {
			return outcome
		}
		return ProofReceiptOutcome{AttemptClass: ProofReceiptSchemaFailure, Result: ProofReceiptUnparseable, ProcessExitCode: ProofReceiptExitCode(1)}
	}
	return ProofReceiptOutcome{AttemptClass: outcome.AttemptClass, Result: ProofReceiptUnparseable, ProcessExitCode: outcome.ProcessExitCode}
}

func validProofReceiptAttemptClass(class ProofReceiptAttemptClass) bool {
	switch class {
	case ProofReceiptFinalVerdict, ProofReceiptRateLimit, ProofReceiptUpstream, ProofReceiptTransient,
		ProofReceiptNetwork, ProofReceiptDeadline, ProofReceiptRunnerFailure, ProofReceiptReceiptFailure,
		ProofReceiptHTTPClientError, ProofReceiptParseFailure, ProofReceiptSchemaFailure,
		ProofReceiptIdentityMismatch, ProofReceiptMalformedTool, ProofReceiptOpaque,
		ProofReceiptUntrustedBinding, ProofReceiptUnknown:
		return true
	default:
		return false
	}
}

func validProofReceiptResult(result ProofReceiptResult) bool {
	return result == ProofReceiptPass || result == ProofReceiptFail || result == ProofReceiptBlocked || result == ProofReceiptUnparseable
}

func proofReceiptRetryable(class ProofReceiptAttemptClass) bool {
	switch class {
	case ProofReceiptRateLimit, ProofReceiptUpstream, ProofReceiptTransient, ProofReceiptNetwork,
		ProofReceiptDeadline, ProofReceiptRunnerFailure, ProofReceiptReceiptFailure:
		return true
	default:
		return false
	}
}

func atomicWriteProofReceipt(path string, receipt ProofReceipt) error {
	data, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return ErrProofReceiptPreflight
	}
	data = append(data, '\n')
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".proof-receipt-")
	if err != nil {
		return ErrProofReceiptPreflight
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return ErrProofReceiptPreflight
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return ErrProofReceiptPreflight
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return ErrProofReceiptPreflight
	}
	if err := tmp.Close(); err != nil {
		return ErrProofReceiptPreflight
	}
	if err := os.Rename(tmpName, path); err != nil {
		return ErrProofReceiptPreflight
	}
	parent, err := os.Open(dir)
	if err != nil {
		return ErrProofReceiptPreflight
	}
	defer parent.Close()
	if err := parent.Sync(); err != nil {
		return ErrProofReceiptPreflight
	}
	return nil
}

// PrintProofReceipt renders only the schema allowlist, never model findings or
// an underlying error. The output remains safe to add to a proof bundle.
func PrintProofReceipt(receipt ProofReceipt) string {
	return fmt.Sprintf("proof receipt\nrelease: %s\nslice: %s\ncheck: %s\nmodel: %s\nstart: %s\nattempt: %d\nclass: %s\nresult: %s\nprocess exit: %s\n",
		receipt.Release, receipt.SliceID, receipt.CheckType, receipt.ModelID,
		receipt.ImmutableStartCommit, receipt.Attempt, receipt.AttemptClass,
		receipt.Result, receipt.ProcessExitCode.String())
}

// JSONProofReceipt serializes the strict record rather than a generic report.
func JSONProofReceipt(receipt ProofReceipt) string {
	data, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return `{"error":"proof receipt render failed"}`
	}
	return string(data)
}

// NewProofReceiptRunner prepares the selected check before receipt reservation
// and returns a one-call runner. S22 deliberately confines native receipts to
// the direct spec-ambiguity proof; it does not create a generic retry surface.
func NewProofReceiptRunner(checkType CheckType, sliceDir string, verifier model.Verifier) (ProofReceiptRunner, error) {
	if checkType != CheckSpecAmbiguity {
		return nil, ErrProofReceiptPreflight
	}
	resolution, err := spec.ResolveReferences(filepath.Join(sliceDir, "spec.json"))
	if err != nil {
		return nil, ErrProofReceiptPreflight
	}
	systemPrompt, err := prompt.LLMCheck(string(CheckSpecAmbiguity))
	if err != nil {
		return nil, ErrProofReceiptPreflight
	}
	payload := buildUserPayload(project.Resolve(resolution.WorkspaceRoot), spec.RenderMarkdown(resolution.Record), "")
	payload += "\n\n--- REFERENCED ARTIFACTS ---\n\n" + resolution.Render()

	return func(ctx context.Context) ProofReceiptOutcome {
		raw, err := model.ChatStructuredJSON(ctx, verifier, systemPrompt, payload, schemas.SpecAmbiguityReportV1)
		if err != nil {
			return proofReceiptModelErrorOutcome(err)
		}
		return proofReceiptSpecAmbiguityOutcome(raw, resolution.Record.SliceID, resolution.Record.Release)
	}, nil
}

func proofReceiptModelErrorOutcome(err error) ProofReceiptOutcome {
	class := ProofReceiptUnknown
	switch model.ClassifyProofReceiptError(err) {
	case model.ProofReceiptErrorRateLimit:
		class = ProofReceiptRateLimit
	case model.ProofReceiptErrorUpstream:
		class = ProofReceiptUpstream
	case model.ProofReceiptErrorTransient:
		class = ProofReceiptTransient
	case model.ProofReceiptErrorNetwork:
		class = ProofReceiptNetwork
	case model.ProofReceiptErrorDeadline:
		class = ProofReceiptDeadline
	case model.ProofReceiptErrorHTTPClient:
		class = ProofReceiptHTTPClientError
	case model.ProofReceiptErrorMalformedTool:
		class = ProofReceiptMalformedTool
	case model.ProofReceiptErrorOpaque:
		class = ProofReceiptOpaque
	}
	return ProofReceiptOutcome{AttemptClass: class, Result: ProofReceiptUnparseable, ProcessExitCode: ProofReceiptExitCode(2)}
}

func proofReceiptSpecAmbiguityOutcome(raw, expectedSlice, expectedRelease string) ProofReceiptOutcome {
	var report SpecAmbiguityReport
	if err := spec.DecodeJSONNoDuplicate([]byte(raw), &report); err != nil {
		return ProofReceiptOutcome{AttemptClass: ProofReceiptParseFailure, Result: ProofReceiptUnparseable, ProcessExitCode: ProofReceiptExitCode(1)}
	}
	if err := baton.ValidateSchema("spec-ambiguity-report-v1", []byte(raw)); err != nil {
		return ProofReceiptOutcome{AttemptClass: ProofReceiptSchemaFailure, Result: ProofReceiptUnparseable, ProcessExitCode: ProofReceiptExitCode(1)}
	}
	if report.SliceID != expectedSlice || report.Release != expectedRelease || report.Check != CheckSpecAmbiguity {
		return ProofReceiptOutcome{AttemptClass: ProofReceiptIdentityMismatch, Result: ProofReceiptUnparseable, ProcessExitCode: ProofReceiptExitCode(1)}
	}
	for fingerprint := range report.BlockingFindings {
		if _, overlap := report.AdvisoryFindings[fingerprint]; overlap {
			return ProofReceiptOutcome{AttemptClass: ProofReceiptSchemaFailure, Result: ProofReceiptUnparseable, ProcessExitCode: ProofReceiptExitCode(1)}
		}
	}
	derived := ProofReceiptPass
	if len(report.BlockingFindings) != 0 {
		derived = ProofReceiptFail
	}
	if report.Verdict != string(derived) {
		return ProofReceiptOutcome{AttemptClass: ProofReceiptSchemaFailure, Result: ProofReceiptUnparseable, ProcessExitCode: ProofReceiptExitCode(1)}
	}
	return ProofReceiptOutcome{AttemptClass: ProofReceiptFinalVerdict, Result: derived, ProcessExitCode: ProofReceiptExitCode(exitForProofReceiptResult(derived))}
}

func exitForProofReceiptResult(result ProofReceiptResult) int {
	if result == ProofReceiptPass {
		return 0
	}
	return 1
}
