package driver

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/gitx"
)

const (
	DriverContractVersion          = "sworn.driver/v1"
	RequestSchemaVersion           = "sworn.driver-request/v1"
	ResultSchemaVersion            = "sworn.driver-result/v1"
	OperationVersion               = "baton.operation/v2"
	GuestWorkspacePath             = "/workspace"
	GuestInputPath                 = "/sworn/inputs"
	MaxRequestBytes                = 1_048_576
	MaxInstructionBytes            = 262_144
	MaxProviderOutputBytes         = 1_048_576
	MaxInputs                      = 256
	MaxTimeoutMillis               = 86_400_000
	MaxSafeInteger           int64 = 9_007_199_254_740_991
	DefaultDegradationBudget int64 = 3
	MaxDegradationBudget     int64 = 100
	// DefaultMaxTurnsPerWork is the per-work turn budget a manifest gets
	// when limits.max_turns_per_work is absent. A careful implementer pass
	// over this repo needs ~30 tool turns, so the default sits far above
	// honest work and far below the MaxProviderTurns runaway guard.
	DefaultMaxTurnsPerWork int64 = 200
	// MaxTurnsPerWorkLimit caps limits.max_turns_per_work at the runaway
	// guard itself: a budget at or above MaxProviderTurns could never bind
	// anything the loop already admits.
	MaxTurnsPerWorkLimit int64 = MaxProviderTurns
	// DefaultMaxOutputTokensPerWork is the per-work output-token budget a
	// manifest gets when limits.max_output_tokens_per_work is absent.
	DefaultMaxOutputTokensPerWork int64 = 262_144
	// MaxOutputTokensPerWorkLimit caps limits.max_output_tokens_per_work.
	MaxOutputTokensPerWorkLimit int64 = 4_194_304
	// DefaultIdenticalFailureParkAfter is the consecutive-identical-failure
	// threshold a manifest gets when limits.identical_failure_park_after is
	// absent: two identical failures park before the third try burns.
	DefaultIdenticalFailureParkAfter int64 = 2
	// MaxIdenticalFailureParkAfter caps limits.identical_failure_park_after
	// at the try budget itself; an absent or zero knob means the default.
	MaxIdenticalFailureParkAfter int64 = 3
	// DefaultContinuationLifetimeMillis is the continuation retention a
	// manifest gets when limits.max_continuation_lifetime_ms is absent:
	// today's compile-time 24h, so an unset knob preserves current ageing.
	DefaultContinuationLifetimeMillis int64 = 86_400_000
	// MaxContinuationLifetimeMillisLimit caps
	// limits.max_continuation_lifetime_ms at 30 days: far beyond the
	// multi-day releases the knob serves and far from any int64
	// nanoseconds overflow of the stamped expiry.
	MaxContinuationLifetimeMillisLimit int64 = 2_592_000_000
	// DefaultMaxNativeOutputStreamBytes is the cumulative native
	// event-stream byte budget a manifest gets when
	// limits.max_native_output_stream_bytes is absent: 16MB, the operator
	// patch's precedented 16x over the per-line MaxProviderResponseBytes
	// floor.
	DefaultMaxNativeOutputStreamBytes int64 = 16_777_216
	// MaxNativeOutputStreamBytesLimit caps
	// limits.max_native_output_stream_bytes: 256MB, the same 16x step
	// above the default that the default is above the per-line floor.
	MaxNativeOutputStreamBytesLimit int64 = 268_435_456
)

var (
	identityPattern       = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,199}$`)
	driverIdentityPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	versionPattern        = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9.-]+)?$`)
	digestPattern         = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

// ContractError carries a stable code and never request, model, stderr, or
// secret bytes. Per the S5-provider-limit-evidence ruling, Detail carries
// the provider's own words for exactly the closed provider status family:
// the status envelope's error.message, normalized to single-line,
// control-free valid UTF-8 and bounded to maxProviderErrorDetailBytes at
// extraction, then re-validated at the dispatcher boundary. NATIVE_SURFACE_INVALID
// and PROCESS_START_FAILED carry their own distinct closed-vocabulary
// structured envelopes instead (native_surface_check.go, sandbox_start_detail.go).
// Request, credential, header, and sibling-envelope bytes structurally
// cannot enter any of these; every other code keeps Detail empty.
type ContractError struct {
	Code string
	// Detail is populated beside the provider status codes (PROVIDER_LIMITED,
	// PROVIDER_AUTHORIZATION_FAILED, PROVIDER_REQUEST_REJECTED,
	// PROVIDER_UNAVAILABLE, PROVIDER_ERROR, PROVIDER_TRANSPORT_FAILED) as
	// bounded provider/native-stderr text, and beside NATIVE_SURFACE_INVALID
	// and PROCESS_START_FAILED as their own structured envelopes, in every
	// case only after normalizeAdapterError re-validates it at the
	// dispatcher boundary. A Detail that never reaches normalizeAdapterError
	// - CONTINUATION_INVALID, the submission-refusal family, and
	// INVALID_RECOVERY_DECISION/INVALID_ADVISORY_RESULT's automation Detail
	// among them - is same-process loop-correction or refusal text instead:
	// constructed and consumed within this package, so it needs no funnel
	// re-validation.
	Detail string
	// RetryAfter is the provider-advised pacing for a retryable rejection
	// (429 RetryInfo body or Retry-After header). Zero when no usable delay
	// could be read. It is advisory transport metadata, never provider
	// content.
	RetryAfter time.Duration
	// HardLimit marks a PROVIDER_LIMITED error classified as a hard wall: a
	// 429 that names no retry window (no RetryInfo retryDelay, no Retry-After
	// header). The dispatch fails immediately instead of pacing into it; a
	// windowed 429 keeps today's paced path. The flag rides Detail through
	// the dispatcher boundary for exactly the provider status codes.
	HardLimit bool
	// Kind classifies why the refusal happened, distinct from Code (what
	// refused). It is computed once, centrally, by classifyKind at the
	// normalizeAdapterError funnel (invoke.go) and never set at an
	// individual raise site, so every admitted code carries it without
	// per-site churn. A code this slice does not place classifies to the
	// empty RefusalKind ("").
	Kind RefusalKind
}

// RefusalKind distinguishes the cause of a driver-boundary refusal from its
// stable Code. It is evidence for pacing and future routing decisions, not
// itself a routing decision: this slice only classifies and records.
type RefusalKind string

const (
	// KindAuthorization covers credential-identity refusals: stale, missing,
	// uncertified, or rotated-away-from credentials, and provider-reported
	// authorization failures.
	KindAuthorization RefusalKind = "authorization"
	// KindHardExhaustion marks a provider refusal that must never be paced:
	// a 429 naming no retry window, or one whose body matches the closed
	// hard-cap exhaustion vocabulary even under a named window.
	KindHardExhaustion RefusalKind = "hard_exhaustion"
	// KindSoftRateLimit marks a provider 429 that names a retry window and
	// carries no hard-cap phrase: today's paced-retry path, unchanged.
	KindSoftRateLimit RefusalKind = "soft_rate_limit"
	// KindTransport covers process, network, and endpoint failures below the
	// provider's own status envelope.
	KindTransport RefusalKind = "transport"
	// KindSurfaceIntegrity covers a containment or protocol invariant the
	// native surface itself enforces (NATIVE_SURFACE_INVALID).
	KindSurfaceIntegrity RefusalKind = "surface_integrity"
	// KindEconomy covers a manifest-governed budget crossing: turn or
	// output-token economy, and the native output-stream byte economy.
	KindEconomy RefusalKind = "economy"
)

func (e *ContractError) Error() string {
	if e.Detail != "" {
		return "driver contract: " + e.Code + ": " + e.Detail
	}
	return "driver contract: " + e.Code
}

func fail(code string) error { return &ContractError{Code: code} }

func failContinuation(site string) error {
	return &ContractError{Code: "CONTINUATION_INVALID", Detail: site}
}
func IsCode(err error, code string) bool {
	var contractErr *ContractError
	return errors.As(err, &contractErr) && contractErr.Code == code
}

type Role string

const (
	RolePlanner     Role = "planner"
	RoleImplementer Role = "implementer"
	RoleCaptain     Role = "captain"
	RoleVerifier    Role = "verifier"
)

func (r Role) valid() bool {
	return r == RolePlanner || r == RoleImplementer || r == RoleCaptain ||
		r == RoleVerifier
}

var operationForRole = map[Role]string{
	RolePlanner:     "baton-plan",
	RoleImplementer: "baton-implement",
	RoleCaptain:     "baton-design-review",
	RoleVerifier:    "baton-verify",
}

type Operation struct {
	ID           string `json:"id"`
	Version      string `json:"version"`
	Digest       string `json:"digest"`
	Instructions string `json:"instructions"`
}
type PackageIdentity struct {
	Version        string `json:"version"`
	ManifestSHA256 string `json:"manifest_sha256"`
}
type WorkspaceAccess string

const (
	ReadOnly  WorkspaceAccess = "read_only"
	ReadWrite WorkspaceAccess = "read_write"
)

type Workspace struct {
	Path   string          `json:"path"`
	Access WorkspaceAccess `json:"access"`
}
type Input struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Digest string `json:"digest"`
}
type Limits struct {
	TimeoutMillis     int64 `json:"timeout_ms"`
	OutputBytes       int64 `json:"output_bytes"`
	DegradationBudget int64 `json:"degradation_budget,omitempty"`
	// MaxTurnsPerWork and MaxOutputTokensPerWork are the per-work economy
	// budgets (A1): a dispatch crossing either parks the work at the next
	// safe conversation-loop boundary. Absent/zero means the documented
	// default. IdenticalFailureParkAfter is the consecutive identical
	// operational-failure threshold that parks a work early (A2); the
	// degradation_budget pattern applies exactly — omitempty, absent/zero
	// means the default of DefaultIdenticalFailureParkAfter.
	MaxTurnsPerWork           int64 `json:"max_turns_per_work,omitempty"`
	MaxOutputTokensPerWork    int64 `json:"max_output_tokens_per_work,omitempty"`
	IdenticalFailureParkAfter int64 `json:"identical_failure_park_after,omitempty"`
	// MaxContinuationLifetimeMillis is the continuation retention window
	// (A1): a suspend stamps expiresNano from this governed value. Absent
	// or zero means DefaultContinuationLifetimeMillis (24h).
	MaxContinuationLifetimeMillis int64 `json:"max_continuation_lifetime_ms,omitempty"`
	// MaxNativeOutputStreamBytes is the cumulative native event-stream byte
	// budget (S3-output-stream-economy A1): a native dispatch whose
	// cumulative decoded event bytes cross it fails
	// ECONOMY_OUTPUT_BUDGET_EXCEEDED instead of NATIVE_SURFACE_INVALID.
	// Absent or zero means DefaultMaxNativeOutputStreamBytes; a declared
	// value must be at least MaxProviderResponseBytes, the per-line floor.
	MaxNativeOutputStreamBytes int64 `json:"max_native_output_stream_bytes,omitempty"`
}

func (l Limits) EffectiveDegradationBudget() int64 {
	if l.DegradationBudget > 0 {
		return l.DegradationBudget
	}
	return DefaultDegradationBudget
}

func (l Limits) EffectiveMaxTurnsPerWork() int64 {
	if l.MaxTurnsPerWork > 0 {
		return l.MaxTurnsPerWork
	}
	return DefaultMaxTurnsPerWork
}

func (l Limits) EffectiveMaxOutputTokensPerWork() int64 {
	if l.MaxOutputTokensPerWork > 0 {
		return l.MaxOutputTokensPerWork
	}
	return DefaultMaxOutputTokensPerWork
}

func (l Limits) EffectiveIdenticalFailureParkAfter() int64 {
	if l.IdenticalFailureParkAfter > 0 {
		return l.IdenticalFailureParkAfter
	}
	return DefaultIdenticalFailureParkAfter
}

func (l Limits) EffectiveContinuationLifetime() time.Duration {
	millis := l.MaxContinuationLifetimeMillis
	if millis <= 0 {
		millis = DefaultContinuationLifetimeMillis
	}
	return time.Duration(millis) * time.Millisecond
}

func (l Limits) EffectiveMaxNativeOutputStreamBytes() int64 {
	if l.MaxNativeOutputStreamBytes > 0 {
		return l.MaxNativeOutputStreamBytes
	}
	return DefaultMaxNativeOutputStreamBytes
}

type Request struct {
	SchemaVersion string    `json:"schema_version"`
	InvocationID  string    `json:"invocation_id"`
	Role          Role      `json:"role"`
	Operation     Operation `json:"operation"`
	Profile       string    `json:"profile"`
	Model         string    `json:"model"`
	Workspace     Workspace `json:"workspace"`
	Inputs        []Input   `json:"inputs"`
	// FreshContext describes a well-formed request. Execution routes decide
	// whether a nonfresh request has the continuation authority it requires.
	FreshContext bool   `json:"fresh_context"`
	Limits       Limits `json:"limits"`
}
type Usage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
	// CacheReadTokens and CacheWriteTokens are the normalized cache-accounting
	// pair surfaced from provider responses. Each side is nil (omitted) when
	// the provider vocabulary reports only one side (Gemini and the Responses
	// API report reads only); a nil side is honest absence, never a measured
	// zero.
	CacheReadTokens  *int64 `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens *int64 `json:"cache_write_tokens,omitempty"`
	// ReasoningTokens is the one additive, optional, backward-compatible
	// receipt field this release authorizes. It rides the same path
	// cache_read_tokens takes (Gemini's thoughtsTokenCount into the driver
	// result, through NormalizeUsage into the journal receipt, observe
	// aggregation and telemetry, and the production journey's receipt
	// assertion); nil is honest absence and stays omitted, so every receipt
	// written before the field existed re-encodes byte-identically.
	ReasoningTokens *int64 `json:"reasoning_tokens,omitempty"`
}
type TransportStatus string

const (
	Completed      TransportStatus = "completed"
	TransportError TransportStatus = "transport_error"
	TimedOut       TransportStatus = "timeout"
	Cancelled      TransportStatus = "cancelled"
	RunnerError    TransportStatus = "runner_error"
)

func (s TransportStatus) valid() bool {
	return s == Completed || s == TransportError || s == TimedOut ||
		s == Cancelled || s == RunnerError
}

type Result struct {
	SchemaVersion   string           `json:"schema_version"`
	InvocationID    string           `json:"invocation_id"`
	AdapterID       string           `json:"adapter_id"`
	AdapterVersion  string           `json:"adapter_version"`
	ObservedModel   string           `json:"observed_model"`
	DurationMillis  int64            `json:"duration_ms"`
	TransportStatus TransportStatus  `json:"transport_status"`
	Usage           *Usage           `json:"usage,omitempty"`
	Cost            *CostObservation `json:"cost,omitempty"`
}
type ResultBinding struct {
	InvocationID, AdapterID, AdapterVersion string
	Model                                   string
	BindModel                               bool
}
type DriverInfo struct {
	ContractVersion string `json:"contract_version"`
	AdapterID       string `json:"adapter_id"`
	AdapterVersion  string `json:"adapter_version"`
}
type DriverInfoBinding struct {
	AdapterID, AdapterVersion string
}

func ValidateDriverInfo(info DriverInfo, expected DriverInfoBinding) error {
	if info.ContractVersion != DriverContractVersion {
		return fail("INVALID_VERSION")
	}
	if !driverIdentityPattern.MatchString(info.AdapterID) ||
		!versionPattern.MatchString(info.AdapterVersion) {
		return fail("INVALID_DRIVER")
	}
	if expected.AdapterID != "" && info.AdapterID != expected.AdapterID {
		return fail("DRIVER_BINDING_MISMATCH")
	}
	if expected.AdapterVersion != "" && info.AdapterVersion != expected.AdapterVersion {
		return fail("DRIVER_BINDING_MISMATCH")
	}
	return nil
}
func EncodeDriverInfo(info DriverInfo) ([]byte, error) {
	return encodeValidated(ValidateDriverInfo(info, DriverInfoBinding{}), info)
}
func DecodeDriverInfo(body []byte, expected DriverInfoBinding) (DriverInfo, error) {
	var info DriverInfo
	if _, err := decodeTyped(
		body,
		4_096,
		[]string{"contract_version", "adapter_id", "adapter_version"},
		nil,
		&info,
	); err != nil {
		return DriverInfo{}, err
	}
	if err := ValidateDriverInfo(info, expected); err != nil {
		return DriverInfo{}, err
	}
	canonical, err := EncodeDriverInfo(info)
	if err != nil {
		return DriverInfo{}, err
	}
	if !bytes.Equal(canonical, body) {
		return DriverInfo{}, fail("NONCANONICAL_JSON")
	}
	return info, nil
}
func CanonicalOperation(role Role) (Operation, error) {
	id, ok := operationForRole[role]
	if !ok {
		return Operation{}, fail("INVALID_ROLE")
	}
	pkg, _, err := admittedPackage()
	if err != nil {
		return Operation{}, fail("INVALID_PACKAGE")
	}
	body, err := pkg.ReadAsset("operations/" + id + ".md")
	if err != nil {
		return Operation{}, fail("INVALID_PACKAGE")
	}
	if len(body) == 0 || len(body) > MaxInstructionBytes || !utf8.Valid(body) ||
		body[len(body)-1] != '\n' || bytes.ContainsRune(body, '\r') {
		return Operation{}, fail("INVALID_PACKAGE")
	}
	return Operation{
		ID:           id,
		Version:      OperationVersion,
		Digest:       Digest(body),
		Instructions: string(body),
	}, nil
}

// RoleAssetAddendumVersion is sworn's own version for the role-asset
// addendum, distinct from OperationVersion: the addendum is sworn-authored
// guidance riding beside the vendored Baton operation, not a Baton asset,
// so a version string that reads like Baton's would blur that provenance
// line.
const RoleAssetAddendumVersion = "sworn.role-addendum/v1"

// roleAssetAddendumText states, for the roles that dispatch work against a
// candidate, the three facts that recur as review turns when a role
// re-derives or mis-adjudicates them instead: canonical-content digest
// semantics, before/product_tree as invocation-state digests, and
// seal-epoch lockstep with the try ledger. LF-only, no CR, trailing
// newline, matching CanonicalOperation's own byte discipline.
const roleAssetAddendumText = "Contract and receipt digests are digests of canonical content: equivalent content hashes identically regardless of key order or formatting.\nbefore and product_tree are digests of invocation state, the tree identities a verifier checks bindings against rather than reconstructing.\nThe seal epoch moves in lockstep with the try ledger: a retry never crosses epochs, and an epoch never re-admits succeeded work.\n"

// Addendum carries sworn-owned role guidance delivered beside the vendored
// Baton Operation. Its Digest is independent of Operation.Digest, computed
// by the same Digest helper over the addendum's own bytes, so the addendum
// gets its own accounting rather than mutating a pinned vendored digest.
type Addendum struct {
	Version string `json:"version"`
	Digest  string `json:"digest"`
	Text    string `json:"text"`
}

// RoleAssetAddendum returns the sworn-owned addendum for roles that
// dispatch work against a candidate: implementer, captain, and verifier.
// The plan template already states canonical-content digest semantics to
// the planner (internal/baton/snapshot/assets/templates/plan.md), so
// RolePlanner and any other role return nil.
func RoleAssetAddendum(role Role) *Addendum {
	if !role.valid() || role == RolePlanner {
		return nil
	}
	return &Addendum{
		Version: RoleAssetAddendumVersion,
		Digest:  Digest([]byte(roleAssetAddendumText)),
		Text:    roleAssetAddendumText,
	}
}

// admittedPackage loads Sworn's own embedded role-asset bundle. Admission is
// self-consistency only: the compiled bundle must match its own recorded
// digests. It never requires a separately installed, tagged, checked-out, or
// certified external Baton release.
func admittedPackage() (baton.Package, PackageIdentity, error) {
	pkg, err := baton.Load()
	if err != nil {
		return baton.Package{}, PackageIdentity{}, err
	}
	identity, err := pkg.Identity()
	if err != nil ||
		identity.RoleAssetsVersion != baton.RoleAssetsVersion ||
		identity.ManifestSHA256 != baton.ManifestSHA256 {
		return baton.Package{}, PackageIdentity{}, fail("INVALID_PACKAGE")
	}
	return pkg, PackageIdentity{
		Version:        identity.RoleAssetsVersion,
		ManifestSHA256: identity.ManifestSHA256,
	}, nil
}
func NewRequest(
	invocationID string,
	role Role,
	profile string,
	model string,
	workspace Workspace,
	inputs []Input,
	freshContext bool,
	limits Limits,
) (Request, error) {
	operation, err := CanonicalOperation(role)
	if err != nil {
		return Request{}, err
	}
	request := Request{
		SchemaVersion: RequestSchemaVersion,
		InvocationID:  invocationID,
		Role:          role,
		Operation:     operation,
		Profile:       profile,
		Model:         model,
		Workspace:     workspace,
		Inputs:        make([]Input, len(inputs)),
		FreshContext:  freshContext,
		Limits:        limits,
	}
	copy(request.Inputs, inputs)
	if err := ValidateRequest(request); err != nil {
		return Request{}, err
	}
	return request, nil
}

// ValidateRequest validates a request against the fixed reserved-name
// defaults. The engine-computed reserved set (which follows configured
// project roots) is threaded separately by validateRequest so invocation
// admission rejects a relocated records or journals root.
func ValidateRequest(request Request) error {
	return validateRequest(request, nil)
}

func validateRequest(request Request, reserved []string) error {
	if request.SchemaVersion != RequestSchemaVersion {
		return fail("INVALID_VERSION")
	}
	if err := validateIdentity(request.InvocationID); err != nil {
		return err
	}
	if !request.Role.valid() {
		return fail("INVALID_ROLE")
	}
	canonical, err := CanonicalOperation(request.Role)
	if err != nil {
		return err
	}
	if request.Operation != canonical {
		if request.Operation.ID != canonical.ID {
			return fail("OPERATION_ROLE_MISMATCH")
		}
		return fail("STALE_OPERATION")
	}
	if !providerKeyPattern.MatchString(request.Profile) {
		return fail("INVALID_PROFILE")
	}
	if validateText(request.Model, 500, false) != nil {
		return fail("INVALID_MODEL")
	}
	if err := validateWorkspace(request.Workspace); err != nil {
		return err
	}
	if request.Role == RoleVerifier &&
		request.Workspace.Access != ReadOnly {
		return fail("INVALID_VERIFIER")
	}
	if request.Inputs == nil {
		return fail("INVALID_FIELD")
	}
	if len(request.Inputs) > MaxInputs {
		return fail("RESOURCE_LIMIT")
	}
	names := make(map[string]struct{}, len(request.Inputs))
	paths := make(map[string]struct{}, len(request.Inputs))
	for _, input := range request.Inputs {
		if !driverIdentityPattern.MatchString(input.Name) {
			return fail("INVALID_INPUT")
		}
		if err := validateRepositoryPath(input.Path, reserved); err != nil {
			return err
		}
		if !digestPattern.MatchString(input.Digest) {
			return fail("INVALID_DIGEST")
		}
		if _, exists := names[input.Name]; exists {
			return fail("DUPLICATE_INPUT")
		}
		if _, exists := paths[input.Path]; exists {
			return fail("DUPLICATE_INPUT")
		}
		names[input.Name] = struct{}{}
		paths[input.Path] = struct{}{}
	}
	if request.Limits.TimeoutMillis < 1 || request.Limits.TimeoutMillis > MaxTimeoutMillis {
		return fail("INVALID_LIMIT")
	}
	if request.Limits.OutputBytes < 1 || request.Limits.OutputBytes > MaxProviderOutputBytes {
		return fail("INVALID_LIMIT")
	}
	if request.Limits.DegradationBudget < 0 || request.Limits.DegradationBudget > MaxDegradationBudget {
		return fail("INVALID_LIMIT")
	}
	if request.Limits.MaxTurnsPerWork < 0 ||
		request.Limits.MaxTurnsPerWork > MaxTurnsPerWorkLimit {
		return fail("INVALID_LIMIT")
	}
	if request.Limits.MaxOutputTokensPerWork < 0 ||
		request.Limits.MaxOutputTokensPerWork > MaxOutputTokensPerWorkLimit {
		return fail("INVALID_LIMIT")
	}
	if request.Limits.IdenticalFailureParkAfter < 0 ||
		request.Limits.IdenticalFailureParkAfter > MaxIdenticalFailureParkAfter {
		return fail("INVALID_LIMIT")
	}
	if request.Limits.MaxContinuationLifetimeMillis < 0 ||
		request.Limits.MaxContinuationLifetimeMillis > MaxContinuationLifetimeMillisLimit {
		return fail("INVALID_LIMIT")
	}
	if request.Limits.MaxNativeOutputStreamBytes < 0 ||
		(request.Limits.MaxNativeOutputStreamBytes > 0 &&
			request.Limits.MaxNativeOutputStreamBytes < MaxProviderResponseBytes) ||
		request.Limits.MaxNativeOutputStreamBytes > MaxNativeOutputStreamBytesLimit {
		return fail("INVALID_LIMIT")
	}
	body, err := json.Marshal(request)
	if err != nil || len(body)+1 > MaxRequestBytes {
		return fail("RESOURCE_LIMIT")
	}
	return nil
}
func EncodeRequest(request Request) ([]byte, error) {
	return encodeValidated(ValidateRequest(request), request)
}
func DecodeRequest(body []byte) (Request, error) {
	var request Request
	if _, err := decodeTyped(
		body,
		MaxRequestBytes,
		[]string{
			"schema_version", "invocation_id", "role", "operation", "profile", "model",
			"workspace", "inputs", "fresh_context", "limits",
		},
		nil,
		&request,
	); err != nil {
		return Request{}, err
	}
	if err := ValidateRequest(request); err != nil {
		return Request{}, err
	}
	canonical, err := EncodeRequest(request)
	if err != nil {
		return Request{}, err
	}
	if !bytes.Equal(canonical, body) {
		return Request{}, fail("NONCANONICAL_JSON")
	}
	return request, nil
}
func ValidateResult(result Result, expected ResultBinding) error {
	if result.SchemaVersion != ResultSchemaVersion {
		return fail("INVALID_VERSION")
	}
	if err := validateIdentity(result.InvocationID); err != nil {
		return err
	}
	if !driverIdentityPattern.MatchString(result.AdapterID) ||
		!versionPattern.MatchString(result.AdapterVersion) {
		return fail("INVALID_DRIVER")
	}
	if err := validateText(result.ObservedModel, 500, false); err != nil {
		return fail("INVALID_MODEL")
	}
	if result.DurationMillis < 0 || result.DurationMillis > MaxSafeInteger {
		return fail("INVALID_FIELD")
	}
	if !result.TransportStatus.valid() {
		return fail("INVALID_TRANSPORT_STATUS")
	}
	if result.Usage != nil {
		if result.Usage.InputTokens < 0 || result.Usage.InputTokens > MaxSafeInteger ||
			result.Usage.OutputTokens < 0 || result.Usage.OutputTokens > MaxSafeInteger {
			return fail("INVALID_USAGE")
		}
		if result.Usage.CacheReadTokens != nil &&
			(*result.Usage.CacheReadTokens < 0 ||
				*result.Usage.CacheReadTokens > MaxSafeInteger) {
			return fail("INVALID_USAGE")
		}
		if result.Usage.CacheWriteTokens != nil &&
			(*result.Usage.CacheWriteTokens < 0 ||
				*result.Usage.CacheWriteTokens > MaxSafeInteger) {
			return fail("INVALID_USAGE")
		}
		if result.Usage.ReasoningTokens != nil &&
			(*result.Usage.ReasoningTokens < 0 ||
				*result.Usage.ReasoningTokens > MaxSafeInteger) {
			return fail("INVALID_USAGE")
		}
	}
	if result.Cost != nil {
		if err := validateCostObservation(*result.Cost); err != nil {
			return err
		}
	}
	if expected.InvocationID != "" && result.InvocationID != expected.InvocationID {
		return fail("RESULT_BINDING_MISMATCH")
	}
	if expected.AdapterID != "" && result.AdapterID != expected.AdapterID {
		return fail("RESULT_BINDING_MISMATCH")
	}
	if expected.AdapterVersion != "" && result.AdapterVersion != expected.AdapterVersion {
		return fail("RESULT_BINDING_MISMATCH")
	}
	if expected.BindModel {
		if result.ObservedModel != expected.Model {
			return fail("RESULT_BINDING_MISMATCH")
		}
	}
	return nil
}
func EncodeResult(result Result) ([]byte, error) {
	return encodeValidated(ValidateResult(result, ResultBinding{}), result)
}
func DecodeResult(body []byte, expected ResultBinding) (Result, error) {
	if len(body) < 2 || body[len(body)-1] != '\n' {
		return Result{}, fail("INVALID_RESULT")
	}
	var result Result
	root, err := decodeTyped(
		body,
		8*1024*1024,
		[]string{
			"schema_version", "invocation_id", "adapter_id", "adapter_version",
			"observed_model", "duration_ms", "transport_status",
		},
		[]string{"usage", "cost"},
		&result,
	)
	if err != nil {
		return Result{}, err
	}
	if usage, present := root["usage"]; present {
		if _, err := closedObject(
			usage,
			[]string{"input_tokens", "output_tokens"},
			[]string{
				"cache_read_tokens", "cache_write_tokens",
				"reasoning_tokens",
			},
		); err != nil {
			return Result{}, err
		}
	}
	if cost, present := root["cost"]; present {
		if _, err := closedObject(
			cost,
			[]string{"micro_units", "currency", "source"},
			nil,
		); err != nil {
			return Result{}, err
		}
	}
	if err := ValidateResult(result, expected); err != nil {
		return Result{}, err
	}
	canonical, err := EncodeResult(result)
	if err != nil {
		return Result{}, err
	}
	if !bytes.Equal(canonical, body) {
		return Result{}, fail("NONCANONICAL_JSON")
	}
	return result, nil
}
func Digest(body []byte) string {
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}
func encodeValidated(validation error, value any) ([]byte, error) {
	if validation != nil {
		return nil, validation
	}
	body, err := json.Marshal(value)
	if err != nil {
		return nil, fail("INVALID_JSON")
	}
	return append(body, '\n'), nil
}
func validateIdentity(value string) error {
	if !identityPattern.MatchString(value) {
		return fail("INVALID_IDENTITY")
	}
	return nil
}
func validateText(value string, maximum int, allowEmpty bool) error {
	if (!allowEmpty && value == "") || len([]byte(value)) > maximum || !utf8.ValidString(value) {
		return fail("INVALID_FIELD")
	}
	if containsControlCharacter(value) {
		return fail("INVALID_FIELD")
	}
	return nil
}
func validateWorkspace(workspace Workspace) error {
	if workspace.Path == "" || len([]byte(workspace.Path)) > 4096 ||
		!utf8.ValidString(workspace.Path) || containsControlCharacter(workspace.Path) ||
		!filepath.IsAbs(workspace.Path) || filepath.Clean(workspace.Path) != workspace.Path ||
		strings.ContainsRune(workspace.Path, '\x00') {
		return fail("INVALID_WORKSPACE")
	}
	if workspace.Access != ReadOnly && workspace.Access != ReadWrite {
		return fail("INVALID_ACCESS")
	}
	return nil
}

// validateRepositoryPath admits guest-relative input and evidence paths. It
// rejects any path whose first segment is a reserved workspace name as a
// first-line guard. The reserved set is the engine-computed one derived from
// the configured project roots plus .git (reserved), threaded through
// invocation admission; an empty set means the fixed default names. Inputs
// are staged under /sworn/inputs and never touch the workspace, so this is
// an admission check; the same reserved set also drives the guest mask sites
// (bubblewrapArguments, runToolBash), the workspace-boundary symlink guard
// and the input projection, all of which read the engine-computed MaskNames
// so a relocated records or journals root is never admitted as an input
// path.
func validateRepositoryPath(value string, reserved []string) error {
	if value == "" || len([]byte(value)) > 1000 || !utf8.ValidString(value) ||
		containsControlCharacter(value) || strings.Contains(value, `\`) ||
		path.IsAbs(value) || path.Clean(value) != value {
		return fail("INVALID_PATH")
	}
	segments := strings.Split(value, "/")
	for _, name := range repositoryReservedNames(reserved) {
		if segments[0] == name {
			return fail("INVALID_PATH")
		}
	}
	for _, segment := range segments {
		if segment == "" || segment == "." || segment == ".." {
			return fail("INVALID_PATH")
		}
	}
	return nil
}

// repositoryReservedNames returns the reserved first-segment names for path
// admission: the caller's engine-computed set when present, else the fixed
// defaults derived once from the default project config.
func repositoryReservedNames(reserved []string) []string {
	if len(reserved) != 0 {
		return reserved
	}
	return gitx.ReservedNames(gitx.DefaultProjectConfig())
}
func containsControlCharacter(value string) bool {
	for _, r := range value {
		if r <= 0x1f || (r >= 0x7f && r <= 0x9f) {
			return true
		}
	}
	return false
}
func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
func decodeStrict(body []byte, maximum int) (any, error) {
	if len(body) == 0 {
		return nil, fail("MISSING_JSON")
	}
	if len(body) > maximum {
		return nil, fail("RESOURCE_LIMIT")
	}
	if !utf8.Valid(body) {
		return nil, fail("INVALID_UTF8")
	}
	if !validJSONUnicodeEscapes(body) {
		return nil, fail("INVALID_UNICODE")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	value, err := decodeValue(decoder)
	if err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, fail("TRAILING_JSON")
	}
	return value, nil
}
func decodeTyped(
	body []byte,
	maximum int,
	required, optional []string,
	target any,
) (map[string]any, error) {
	value, err := decodeStrict(body, maximum)
	if err != nil {
		return nil, err
	}
	root, err := closedObject(value, required, optional)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		if strings.HasPrefix(err.Error(), "json: unknown field ") {
			return nil, fail("UNKNOWN_FIELD")
		}
		return nil, fail("INVALID_FIELD")
	}
	return root, nil
}
func decodeValue(decoder *json.Decoder) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, fail("INVALID_JSON")
	}
	switch value := token.(type) {
	case json.Delim:
		switch value {
		case '{':
			object := make(map[string]any)
			for decoder.More() {
				nameToken, err := decoder.Token()
				if err != nil {
					return nil, fail("INVALID_JSON")
				}
				name, ok := nameToken.(string)
				if !ok {
					return nil, fail("INVALID_JSON")
				}
				if _, duplicate := object[name]; duplicate {
					return nil, fail("DUPLICATE_NAME")
				}
				child, err := decodeValue(decoder)
				if err != nil {
					return nil, err
				}
				object[name] = child
			}
			end, err := decoder.Token()
			if err != nil || end != json.Delim('}') {
				return nil, fail("INVALID_JSON")
			}
			return object, nil
		case '[':
			var array []any
			for decoder.More() {
				child, err := decodeValue(decoder)
				if err != nil {
					return nil, err
				}
				array = append(array, child)
			}
			end, err := decoder.Token()
			if err != nil || end != json.Delim(']') {
				return nil, fail("INVALID_JSON")
			}
			return array, nil
		default:
			return nil, fail("INVALID_JSON")
		}
	case string:
		return value, nil
	case json.Number, bool, nil:
		return value, nil
	default:
		return nil, fail("INVALID_JSON")
	}
}
func validJSONUnicodeEscapes(body []byte) bool {
	inString := false
	for index := 0; index < len(body); index++ {
		switch body[index] {
		case '"':
			inString = !inString
		case '\\':
			if !inString || index+1 >= len(body) {
				continue
			}
			index++
			if body[index] != 'u' {
				continue
			}
			if index+4 >= len(body) {
				return false
			}
			value, ok := parseHexCodeUnit(body[index+1 : index+5])
			if !ok {
				return true // The JSON decoder reports the syntax failure.
			}
			index += 4
			switch {
			case value >= 0xd800 && value <= 0xdbff:
				if index+6 >= len(body) || body[index+1] != '\\' || body[index+2] != 'u' {
					return false
				}
				low, ok := parseHexCodeUnit(body[index+3 : index+7])
				if !ok || low < 0xdc00 || low > 0xdfff {
					return false
				}
				index += 6
			case value >= 0xdc00 && value <= 0xdfff:
				return false
			}
		}
	}
	return true
}
func parseHexCodeUnit(body []byte) (uint16, bool) {
	if len(body) != 4 {
		return 0, false
	}
	var value uint16
	for _, character := range body {
		value <<= 4
		switch {
		case character >= '0' && character <= '9':
			value |= uint16(character - '0')
		case character >= 'a' && character <= 'f':
			value |= uint16(character-'a') + 10
		case character >= 'A' && character <= 'F':
			value |= uint16(character-'A') + 10
		default:
			return 0, false
		}
	}
	return value, true
}
func closedObject(value any, required, optional []string) (map[string]any, error) {
	object, ok := value.(map[string]any)
	if !ok {
		return nil, fail("INVALID_FIELD")
	}
	allowed := make(map[string]struct{}, len(required)+len(optional))
	for _, key := range required {
		allowed[key] = struct{}{}
		if _, present := object[key]; !present {
			return nil, fail("MISSING_FIELD")
		}
	}
	for _, key := range optional {
		allowed[key] = struct{}{}
	}
	for key := range object {
		if _, present := allowed[key]; !present {
			return nil, fail("UNKNOWN_FIELD")
		}
	}
	return object, nil
}
func canonicalJSON(value any) ([]byte, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, fail("INVALID_JSON")
	}
	return body, nil
}
