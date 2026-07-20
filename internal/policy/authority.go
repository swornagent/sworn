// Package policy owns authenticated authority and local policy decisions.
package policy

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/swornagent/sworn/internal/protocol"
)

const (
	AuthorityProofSchemaVersion = "sworn-authority-proof-v1"
	AuthorityReceiptKind        = protocol.ControlReceiptSchemaVersion
	BuildExecutionPurpose       = "build.execute"

	MaximumAuthoritySourceBytes = 64 << 10
	MaximumAuthorityProofBytes  = 16 << 10

	proofSignatureDomain       = "sworn/authority-proof/v1\x00"
	currentBuildPermitLifetime = 30 * time.Second
)

// Resolver returns the exact source and detached proof presented for one exact
// plan. It does not return signing keys or choose a trust root.
type Resolver interface {
	Resolve(ctx context.Context, sourceRef, planDigest string) (source, proof []byte, err error)
}

// ApprovalLedger is the sole durable boundary for authority. Persisting a
// PreparedApproval must atomically persist its contained source observation and
// approval receipt before returning.
type ApprovalLedger interface {
	PutAuthoritySource(context.Context, PreparedSource) error
	PutAuthorityApproval(context.Context, PreparedApproval) error
}

// CurrentAuthorityLedger extends durable observation with a current-head
// assertion. PutCurrentAuthoritySource must reject an authenticated historical
// replay below the source's durable high-water mark, even when that exact
// observation was previously stored successfully.
type CurrentAuthorityLedger interface {
	ApprovalLedger
	PutCurrentAuthoritySource(context.Context, PreparedSource) error
}

// Authority is the engine-owned authority service. Roots, resolver, ledger,
// and clock are fixed at startup; an approval operation accepts only an exact
// plan and cannot replace any of them.
type Authority struct {
	roots    map[string]TrustRoot
	resolver Resolver
	ledger   ApprovalLedger
	now      func() time.Time
}

// BuildPermitRequest is the complete controller-owned identity of one pending
// builder dispatch. Contract must be the exact work contract selected from the
// plan passed to AuthorizeBuild; digests alone are not accepted as substitutes.
type BuildPermitRequest struct {
	ControllerID          string
	RunID                 string
	StateRevision         int64
	WorkID                string
	WorkAttempt           int64
	Contract              protocol.ExactWorkContract
	BuilderDispatchDigest string
}

// BuildPermitFacts is the immutable, non-authorizing projection of a current
// build permit. It is safe to log or compare, but cannot be converted back into
// a permit.
type BuildPermitFacts struct {
	Purpose               string
	ControllerID          string
	RunID                 string
	StateRevision         int64
	PlanDigest            string
	WorkID                string
	WorkAttempt           int64
	WorkContractDigest    string
	BuilderDispatchDigest string
	SourceRef             string
	SourceVersion         int64
	SourceDigest          string
	AuthorizedAt          string
}

type buildPermitBinding struct {
	facts           BuildPermitFacts
	authorityDigest string
	validFrom       string
	validUntil      string
}

// CurrentBuildPermit is an opaque, short-lived capability for exactly one
// builder-dispatch request. It remains bound to the Authority instance which
// freshly resolved and authenticated it.
type CurrentBuildPermit struct {
	authority *Authority
	plan      protocol.ExactPlan
	binding   buildPermitBinding
}

// Facts returns a non-authorizing copy of the permit's exact bindings.
func (permit CurrentBuildPermit) Facts() BuildPermitFacts { return permit.binding.facts }

// RequireLedger proves that this Authority was constructed with the exact
// pointer-backed ledger supplied by its controller. Non-pointer ledger
// implementations fail closed rather than relying on potentially panicking
// interface equality.
func (authority *Authority) RequireLedger(ledger ApprovalLedger) error {
	if authority == nil || len(authority.roots) == 0 || authority.resolver == nil ||
		authority.ledger == nil || authority.now == nil {
		return errors.New("authority service is not initialized")
	}
	want := reflect.ValueOf(authority.ledger)
	got := reflect.ValueOf(ledger)
	if !want.IsValid() || !got.IsValid() || want.Kind() != reflect.Pointer || got.Kind() != reflect.Pointer ||
		want.IsNil() || got.IsNil() || want.Type() != got.Type() || want.Pointer() != got.Pointer() {
		return errors.New("authority service uses a different approval ledger")
	}
	return nil
}

// NewAuthority constructs the production authority service. Trust roots and
// their public keys are defensively copied once, and production time is always
// normalized to explicit UTC inside the service.
func NewAuthority(roots []TrustRoot, resolver Resolver, ledger ApprovalLedger) (*Authority, error) {
	return newAuthorityWithClock(roots, resolver, ledger, func() time.Time { return time.Now().UTC() })
}

func newAuthorityWithClock(
	roots []TrustRoot,
	resolver Resolver,
	ledger ApprovalLedger,
	now func() time.Time,
) (*Authority, error) {
	if len(roots) == 0 {
		return nil, errors.New("authority service requires at least one trust root")
	}
	if resolver == nil || ledger == nil || now == nil {
		return nil, errors.New("authority service requires a resolver, ledger, and clock")
	}
	copied := make(map[string]TrustRoot, len(roots))
	for _, root := range roots {
		if err := validateRoot(root); err != nil {
			return nil, err
		}
		if _, exists := copied[root.sourceRef]; exists {
			return nil, fmt.Errorf("duplicate trust root for source %q", root.sourceRef)
		}
		copied[root.sourceRef] = cloneRoot(root)
	}
	return &Authority{roots: copied, resolver: resolver, ledger: ledger, now: now}, nil
}

// Approve authenticates and durably commits authority for one exact plan.
// A denied authenticated source is durably observed when it does not conflict
// with the source high-water mark; rollback and fork attempts fail atomically.
func (authority *Authority) Approve(
	ctx context.Context,
	plan protocol.ExactPlan,
) (HistoricalApproval, error) {
	preparedSource, now, err := authority.resolveAuthenticatedSource(ctx, plan)
	if err != nil {
		return HistoricalApproval{}, err
	}
	preparedApproval, err := mintCurrentApproval(preparedSource, now)
	if err != nil {
		if persistErr := authority.ledger.PutAuthoritySource(ctx, preparedSource); persistErr != nil {
			return HistoricalApproval{}, fmt.Errorf("persist denied authority source: %w", persistErr)
		}
		return HistoricalApproval{}, err
	}
	if err := authority.ledger.PutAuthorityApproval(ctx, preparedApproval); err != nil {
		return HistoricalApproval{}, fmt.Errorf("persist authority approval: %w", err)
	}
	return historicalFromPrepared(preparedApproval), nil
}

// AuthorizeBuild freshly resolves and authenticates current authority for one
// exact builder dispatch. Every authenticated non-rollback, non-fork source is
// durably recorded before current status, time, or grant policy can return a
// permit or denial. A rollback or fork fails atomically with no permit.
// HistoricalApproval is deliberately not accepted by this boundary.
func (authority *Authority) AuthorizeBuild(
	ctx context.Context,
	plan protocol.ExactPlan,
	request BuildPermitRequest,
) (CurrentBuildPermit, error) {
	if authority == nil || authority.resolver == nil || authority.ledger == nil || authority.now == nil {
		return CurrentBuildPermit{}, errors.New("authority service is not initialized")
	}
	if err := ctx.Err(); err != nil {
		return CurrentBuildPermit{}, err
	}
	if err := validateBuildPermitRequest(plan, request); err != nil {
		return CurrentBuildPermit{}, err
	}
	currentLedger, ok := authority.ledger.(CurrentAuthorityLedger)
	if !ok {
		return CurrentBuildPermit{}, errors.New("authority ledger cannot assert the current source head")
	}
	preparedSource, authenticatedAt, err := authority.resolveAuthenticatedSource(ctx, plan)
	if err != nil {
		return CurrentBuildPermit{}, err
	}
	if err := currentLedger.PutCurrentAuthoritySource(ctx, preparedSource); err != nil {
		return CurrentBuildPermit{}, fmt.Errorf("persist current authority source: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return CurrentBuildPermit{}, err
	}
	// Sample again after durable persistence so source expiry during ledger I/O
	// cannot produce a permit from the earlier authentication instant.
	now := authority.now()
	if now.IsZero() || now.Location() != time.UTC {
		return CurrentBuildPermit{}, errors.New("authority clock must return explicit UTC")
	}
	if now.Before(authenticatedAt) {
		return CurrentBuildPermit{}, errors.New("authority clock moved backward during current authorization")
	}
	if err := validateCurrentSourceWindow(preparedSource, now); err != nil {
		return CurrentBuildPermit{}, err
	}
	if err := validateRequiredBuildGrants(plan, &preparedSource); err != nil {
		return CurrentBuildPermit{}, err
	}
	facts := BuildPermitFacts{
		Purpose: BuildExecutionPurpose, ControllerID: request.ControllerID,
		RunID: request.RunID, StateRevision: request.StateRevision,
		PlanDigest: plan.Record().Digest, WorkID: request.WorkID,
		WorkAttempt: request.WorkAttempt, WorkContractDigest: request.Contract.Digest(),
		BuilderDispatchDigest: request.BuilderDispatchDigest,
		SourceRef:             preparedSource.facts.SourceRef, SourceVersion: preparedSource.facts.SourceVersion,
		SourceDigest: preparedSource.facts.SourceCanonicalDigest,
		AuthorizedAt: now.Format(time.RFC3339Nano),
	}
	return CurrentBuildPermit{
		authority: authority,
		plan:      plan,
		binding: buildPermitBinding{
			facts: facts, authorityDigest: preparedSource.facts.AuthorityDigest,
			validFrom: preparedSource.facts.ValidFrom, validUntil: preparedSource.facts.ValidUntil,
		},
	}, nil
}

// ValidateBuildPermit accepts only an unexpired permit minted by this exact
// Authority instance for the same complete controller request. It performs no
// I/O: callers must therefore authorize immediately before dispatch rather
// than retain permits as standing authority.
func (authority *Authority) ValidateBuildPermit(
	permit CurrentBuildPermit,
	request BuildPermitRequest,
) error {
	if authority == nil || authority.resolver == nil || authority.ledger == nil || authority.now == nil {
		return errors.New("authority service is not initialized")
	}
	if permit.authority != authority {
		return errors.New("build permit belongs to another authority service")
	}
	if permit.binding.facts.Purpose != BuildExecutionPurpose {
		return errors.New("build permit has the wrong purpose")
	}
	if err := validateBuildPermitRequest(permit.plan, request); err != nil {
		return err
	}
	planRecord := permit.plan.Record()
	planAuthority := permit.plan.Authority()
	if permit.binding.facts.PlanDigest != planRecord.Digest ||
		permit.binding.authorityDigest != planAuthority.Digest ||
		permit.binding.facts.SourceRef != planAuthority.SourceRef ||
		!protocol.ValidPositiveSafeInteger(permit.binding.facts.SourceVersion) ||
		!protocol.ValidDigest(permit.binding.facts.SourceDigest) {
		return errors.New("build permit no longer matches its exact authority")
	}
	want := permit.binding.facts
	want.Purpose = BuildExecutionPurpose
	want.ControllerID = request.ControllerID
	want.RunID = request.RunID
	want.StateRevision = request.StateRevision
	want.PlanDigest = planRecord.Digest
	want.WorkID = request.WorkID
	want.WorkAttempt = request.WorkAttempt
	want.WorkContractDigest = request.Contract.Digest()
	want.BuilderDispatchDigest = request.BuilderDispatchDigest
	if permit.binding.facts != want {
		return errors.New("build permit does not match the exact dispatch request")
	}
	if err := validateRequiredBuildGrants(permit.plan, nil); err != nil {
		return err
	}
	now := authority.now()
	if now.IsZero() || now.Location() != time.UTC {
		return errors.New("authority clock must return explicit UTC")
	}
	authorizedAt, err := time.Parse(time.RFC3339Nano, permit.binding.facts.AuthorizedAt)
	if err != nil || authorizedAt.Location() != time.UTC || now.Before(authorizedAt) ||
		now.Sub(authorizedAt) >= currentBuildPermitLifetime {
		return errors.New("build permit is stale")
	}
	nowValue := now.Format(time.RFC3339Nano)
	fromToNow, fromErr := protocol.CompareDateTimes(permit.binding.validFrom, nowValue)
	nowToUntil, untilErr := protocol.CompareDateTimes(nowValue, permit.binding.validUntil)
	if fromErr != nil || untilErr != nil || fromToNow > 0 || nowToUntil >= 0 {
		return errors.New("build permit authority is no longer current")
	}
	return nil
}

func (authority *Authority) resolveAuthenticatedSource(
	ctx context.Context,
	plan protocol.ExactPlan,
) (PreparedSource, time.Time, error) {
	if authority == nil || authority.resolver == nil || authority.ledger == nil || authority.now == nil {
		return PreparedSource{}, time.Time{}, errors.New("authority service is not initialized")
	}
	if err := ctx.Err(); err != nil {
		return PreparedSource{}, time.Time{}, err
	}
	planRecord := plan.Record()
	planAuthority := plan.Authority()
	if planRecord.Kind != protocol.DeliveryPlanSchemaVersion || planRecord.Digest == "" ||
		planAuthority.SourceRef == "" || planAuthority.Digest == "" {
		return PreparedSource{}, time.Time{}, errors.New("authority requires an exact delivery plan")
	}
	root, exists := authority.roots[planAuthority.SourceRef]
	if !exists {
		return PreparedSource{}, time.Time{}, fmt.Errorf("no configured trust root for source %q", planAuthority.SourceRef)
	}
	sourceRaw, proofRaw, err := authority.resolver.Resolve(ctx, root.sourceRef, planRecord.Digest)
	if err != nil {
		return PreparedSource{}, time.Time{}, fmt.Errorf("resolve authority: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return PreparedSource{}, time.Time{}, err
	}
	sourceRaw = bytes.Clone(sourceRaw)
	proofRaw = bytes.Clone(proofRaw)
	// Sample current time after resolver I/O so a source that expires while it
	// is being resolved cannot be minted from a stale pre-resolution instant.
	now := authority.now()
	if now.IsZero() || now.Location() != time.UTC {
		return PreparedSource{}, time.Time{}, errors.New("authority clock must return explicit UTC")
	}
	preparedSource, err := authenticateSource(plan, root, sourceRaw, proofRaw, now, true)
	if err != nil {
		return PreparedSource{}, time.Time{}, err
	}
	return preparedSource, now, nil
}

// TrustRoot is an immutable configured verification capability. The private
// signing key never crosses this boundary.
type TrustRoot struct {
	sourceRef     string
	authorizerRef string
	keyID         string
	publicKey     ed25519.PublicKey
}

func cloneRoot(root TrustRoot) TrustRoot {
	root.publicKey = bytes.Clone(root.publicKey)
	return root
}

// NewTrustRoot pins one source and authorizer identity to an Ed25519 public
// key. The key identifier is derived rather than accepted from configuration.
func NewTrustRoot(sourceRef, authorizerRef string, publicKey ed25519.PublicKey) (TrustRoot, error) {
	if !protocol.ValidNonEmpty(sourceRef) || len(sourceRef) > 512 {
		return TrustRoot{}, errors.New("trust root requires a bounded source reference")
	}
	if !protocol.ValidNonEmpty(authorizerRef) || len(authorizerRef) > 512 {
		return TrustRoot{}, errors.New("trust root requires a bounded authorizer reference")
	}
	if len(publicKey) != ed25519.PublicKeySize {
		return TrustRoot{}, errors.New("trust root requires an Ed25519 public key")
	}
	key := append(ed25519.PublicKey(nil), publicKey...)
	return TrustRoot{
		sourceRef:     sourceRef,
		authorizerRef: authorizerRef,
		keyID:         KeyID(key),
		publicKey:     key,
	}, nil
}

// KeyID returns the deterministic identifier pinned by this root. It exposes
// no signing capability.
func (root TrustRoot) KeyID() string { return root.keyID }

// SourceRef returns the sole logical source this root can authenticate.
func (root TrustRoot) SourceRef() string { return root.sourceRef }

// AuthorizerRef returns the authorizer identity this root authenticates.
func (root TrustRoot) AuthorizerRef() string { return root.authorizerRef }

// KeyID derives the only accepted key identifier representation.
func KeyID(publicKey ed25519.PublicKey) string {
	digest := sha256.Sum256(publicKey)
	return "ed25519-sha256:" + hex.EncodeToString(digest[:])
}

// SourceClosure contains the exact and canonical authenticated source/proof
// bytes. Access through PreparedSource returns defensive copies.
type SourceClosure struct {
	SourceRaw       []byte
	SourceCanonical []byte
	ProofRaw        []byte
	ProofCanonical  []byte
}

// SourceFacts is the immutable public projection of an authenticated source.
// Raw and canonical artifact identities are distinct so storage cannot
// silently normalize caller-presented bytes.
type SourceFacts struct {
	PlanDigest            string
	AuthorityDigest       string
	SourceRef             string
	SourceID              string
	SourceVersion         int64
	SourceStatus          string
	SourceCanonicalDigest string
	SourceRawDigest       string
	Repository            string
	TargetRef             string
	AuthorizerRef         string
	ValidFrom             string
	ValidUntil            string
	ProofRawDigest        string
	ProofCanonicalDigest  string
	RootKeyID             string
	ApprovedAt            string
}

// PreparedSource is an opaque authenticated source observation. It may be
// persisted even when revoked or no longer current, but cannot itself authorize
// delivery.
type PreparedSource struct {
	plan    protocol.ExactPlan
	facts   SourceFacts
	closure SourceClosure
	grants  map[string]struct{}
}

func (source PreparedSource) Plan() protocol.ExactPlan { return source.plan }

func (source PreparedSource) Facts() SourceFacts { return source.facts }

func (source PreparedSource) Closure() SourceClosure { return cloneSourceClosure(source.closure) }

// ApprovalFacts is the minimal immutable projection unique to a minted Baton
// receipt; every causal input remains available through SourceFacts.
type ApprovalFacts struct {
	ReceiptID     string
	ReceiptDigest string
}

// PreparedApproval is an opaque current approval write capability. It contains
// its authenticated source so the ledger can persist both atomically.
type PreparedApproval struct {
	source  PreparedSource
	facts   ApprovalFacts
	receipt protocol.EncodedRecord
}

func (approval PreparedApproval) Source() PreparedSource { return approval.source }

func (approval PreparedApproval) Plan() protocol.ExactPlan { return approval.source.plan }

func (approval PreparedApproval) SourceFacts() SourceFacts { return approval.source.facts }

// Facts returns the immutable store-facing approval projection.
func (approval PreparedApproval) Facts() ApprovalFacts { return approval.facts }

// Receipt returns the canonical Baton authority_approval receipt.
func (approval PreparedApproval) Receipt() protocol.EncodedRecord {
	record := approval.receipt
	record.CanonicalJSON = bytes.Clone(record.CanonicalJSON)
	return record
}

// HistoricalApproval is the immutable read capability returned after the
// ledger has committed an approval or reconstructed one from storage. It is
// intentionally distinct from both persistence write capabilities.
type HistoricalApproval struct {
	plan        protocol.ExactPlan
	sourceFacts SourceFacts
	facts       ApprovalFacts
	receipt     protocol.EncodedRecord
}

func (approval HistoricalApproval) Plan() protocol.ExactPlan { return approval.plan }

func (approval HistoricalApproval) SourceFacts() SourceFacts { return approval.sourceFacts }

func (approval HistoricalApproval) Facts() ApprovalFacts { return approval.facts }

func (approval HistoricalApproval) Receipt() protocol.EncodedRecord {
	record := approval.receipt
	record.CanonicalJSON = bytes.Clone(record.CanonicalJSON)
	return record
}

type authoritySource struct {
	Version       int64             `json:"version"`
	SourceID      string            `json:"source_id"`
	Status        string            `json:"status"`
	Repository    string            `json:"repository"`
	TargetRef     string            `json:"target_ref"`
	MaximumGrants []json.RawMessage `json:"maximum_grants"`
	AuthorizerRef string            `json:"authorizer_ref"`
	ValidFrom     string            `json:"valid_from"`
	ValidUntil    string            `json:"valid_until"`
}

type authorityProof struct {
	SchemaVersion   string `json:"schema_version"`
	SourceRef       string `json:"source_ref"`
	SourceDigest    string `json:"source_digest"`
	SourceVersion   int64  `json:"source_version"`
	PlanDigest      string `json:"plan_digest"`
	AuthorityDigest string `json:"authority_digest"`
	KeyID           string `json:"key_id"`
	ApprovedAt      string `json:"approved_at"`
	Signature       string `json:"signature"`
}

type unsignedAuthorityProof struct {
	SchemaVersion   string `json:"schema_version"`
	SourceRef       string `json:"source_ref"`
	SourceDigest    string `json:"source_digest"`
	SourceVersion   int64  `json:"source_version"`
	PlanDigest      string `json:"plan_digest"`
	AuthorityDigest string `json:"authority_digest"`
	KeyID           string `json:"key_id"`
	ApprovedAt      string `json:"approved_at"`
}

type parsedSource struct {
	value     authoritySource
	raw       []byte
	canonical []byte
	digest    string
	grantSet  map[string]struct{}
}

type parsedProof struct {
	value     authorityProof
	raw       []byte
	canonical []byte
	digest    string
}

func authenticateSource(
	plan protocol.ExactPlan,
	root TrustRoot,
	sourceRaw, proofRaw []byte,
	now time.Time,
	checkFuture bool,
) (PreparedSource, error) {
	if err := validateRoot(root); err != nil {
		return PreparedSource{}, err
	}
	planRecord := plan.Record()
	authority := plan.Authority()
	target := plan.Target()
	if planRecord.Kind != protocol.DeliveryPlanSchemaVersion || planRecord.Digest == "" ||
		authority.SourceRef == "" || authority.Digest == "" || target.Repository == "" || target.Ref == "" {
		return PreparedSource{}, errors.New("authority requires an exact delivery plan")
	}
	if authority.SourceRef != root.sourceRef {
		return PreparedSource{}, errors.New("plan authority source does not match the configured trust root")
	}
	if checkFuture && (now.IsZero() || now.Location() != time.UTC) {
		return PreparedSource{}, errors.New("authority authentication time must be explicit UTC")
	}
	source, err := parseSource(sourceRaw)
	if err != nil {
		return PreparedSource{}, err
	}
	proof, err := parseProof(proofRaw)
	if err != nil {
		return PreparedSource{}, err
	}
	if source.value.AuthorizerRef != root.authorizerRef {
		return PreparedSource{}, errors.New("authority source authorizer does not match the configured trust root")
	}
	if source.value.Repository != target.Repository || source.value.TargetRef != target.Ref {
		return PreparedSource{}, errors.New("authority source target does not match the exact plan")
	}
	if proof.value.SchemaVersion != AuthorityProofSchemaVersion {
		return PreparedSource{}, fmt.Errorf("unknown authority proof schema %q", proof.value.SchemaVersion)
	}
	if proof.value.SourceRef != root.sourceRef || proof.value.SourceDigest != source.digest ||
		proof.value.SourceVersion != source.value.Version || proof.value.PlanDigest != planRecord.Digest ||
		proof.value.AuthorityDigest != authority.Digest || proof.value.KeyID != root.keyID {
		return PreparedSource{}, errors.New("authority proof does not match its source, plan, authority, or trust root")
	}
	message, err := proofMessage(proof.value)
	if err != nil {
		return PreparedSource{}, err
	}
	signature, err := decodeSignature(proof.value.Signature)
	if err != nil {
		return PreparedSource{}, err
	}
	if !ed25519.Verify(root.publicKey, message, signature) {
		return PreparedSource{}, errors.New("authority proof signature is invalid")
	}
	createdToApproved, err := protocol.CompareDateTimes(plan.CreatedAt(), proof.value.ApprovedAt)
	if err != nil {
		return PreparedSource{}, fmt.Errorf("compare plan and approval times: %w", err)
	}
	if createdToApproved > 0 {
		return PreparedSource{}, errors.New("authority approval predates the exact plan")
	}
	fromToApproved, err := protocol.CompareDateTimes(source.value.ValidFrom, proof.value.ApprovedAt)
	if err != nil {
		return PreparedSource{}, fmt.Errorf("compare authority validity and approval: %w", err)
	}
	approvedToUntil, err := protocol.CompareDateTimes(proof.value.ApprovedAt, source.value.ValidUntil)
	if err != nil {
		return PreparedSource{}, fmt.Errorf("compare authority approval and expiry: %w", err)
	}
	if fromToApproved > 0 || approvedToUntil >= 0 {
		return PreparedSource{}, errors.New("authority approval is outside the source validity period")
	}
	if checkFuture {
		nowValue := now.Format(time.RFC3339Nano)
		approvedToNow, err := protocol.CompareDateTimes(proof.value.ApprovedAt, nowValue)
		if err != nil {
			return PreparedSource{}, fmt.Errorf("compare authority approval and current time: %w", err)
		}
		if approvedToNow > 0 {
			return PreparedSource{}, errors.New("authority approval is in the future")
		}
	}
	facts := SourceFacts{
		PlanDigest: planRecord.Digest, AuthorityDigest: authority.Digest,
		SourceRef: root.sourceRef, SourceID: source.value.SourceID,
		SourceVersion: source.value.Version, SourceStatus: source.value.Status,
		SourceCanonicalDigest: source.digest, SourceRawDigest: protocol.RawDigest(source.raw),
		Repository: target.Repository, TargetRef: target.Ref, AuthorizerRef: root.authorizerRef,
		ValidFrom: source.value.ValidFrom, ValidUntil: source.value.ValidUntil,
		ProofRawDigest: protocol.RawDigest(proof.raw), ProofCanonicalDigest: proof.digest,
		RootKeyID: root.keyID, ApprovedAt: proof.value.ApprovedAt,
	}
	closure := SourceClosure{
		SourceRaw:       bytes.Clone(source.raw),
		SourceCanonical: bytes.Clone(source.canonical),
		ProofRaw:        bytes.Clone(proof.raw),
		ProofCanonical:  bytes.Clone(proof.canonical),
	}
	return PreparedSource{plan: plan, facts: facts, closure: closure, grants: cloneGrantSet(source.grantSet)}, nil
}

func mintCurrentApproval(source PreparedSource, now time.Time) (PreparedApproval, error) {
	if err := validateCurrentSource(source, now); err != nil {
		return PreparedApproval{}, err
	}
	return mintArchivedApproval(source)
}

func validateCurrentSource(source PreparedSource, now time.Time) error {
	if now.IsZero() || now.Location() != time.UTC {
		return errors.New("authority minting time must be explicit UTC")
	}
	if source.facts.SourceStatus != "active" {
		return errors.New("authority source is revoked")
	}
	if err := validateGrantCeiling(source); err != nil {
		return err
	}
	return validateCurrentSourceTime(source, now)
}

func validateCurrentSourceWindow(source PreparedSource, now time.Time) error {
	if now.IsZero() || now.Location() != time.UTC {
		return errors.New("authority minting time must be explicit UTC")
	}
	if source.facts.SourceStatus != "active" {
		return errors.New("authority source is revoked")
	}
	return validateCurrentSourceTime(source, now)
}

func validateCurrentSourceTime(source PreparedSource, now time.Time) error {
	nowValue := now.Format(time.RFC3339Nano)
	fromToNow, err := protocol.CompareDateTimes(source.facts.ValidFrom, nowValue)
	if err != nil {
		return fmt.Errorf("compare authority activation and current time: %w", err)
	}
	if fromToNow > 0 {
		return errors.New("authority source is not yet valid")
	}
	nowToUntil, err := protocol.CompareDateTimes(nowValue, source.facts.ValidUntil)
	if err != nil {
		return fmt.Errorf("compare current time and authority expiry: %w", err)
	}
	if nowToUntil >= 0 {
		return errors.New("authority source has expired")
	}
	return nil
}

func mintArchivedApproval(source PreparedSource) (PreparedApproval, error) {
	if source.facts.SourceStatus != "active" {
		return PreparedApproval{}, errors.New("historical approval source was not active")
	}
	if err := validateGrantCeiling(source); err != nil {
		return PreparedApproval{}, err
	}
	planRecord := source.plan.Record()
	authority := source.plan.Authority()
	target := source.plan.Target()
	if planRecord.Digest != source.facts.PlanDigest || authority.Digest != source.facts.AuthorityDigest ||
		authority.SourceRef != source.facts.SourceRef || target.Repository != source.facts.Repository ||
		target.Ref != source.facts.TargetRef {
		return PreparedApproval{}, errors.New("prepared source no longer matches its exact plan")
	}
	orderedGrants := make([]json.RawMessage, 0, len(authority.Grants))
	for _, grant := range authority.Grants {
		orderedGrants = append(orderedGrants, json.RawMessage(grant.CanonicalJSON()))
	}
	receiptID := "authority-" + strings.TrimPrefix(source.facts.ProofCanonicalDigest, "sha256:")
	receipt := protocol.AuthorityApproval{
		SchemaVersion: protocol.ControlReceiptSchemaVersion, Kind: protocol.AuthorityApprovalKind,
		ReceiptID: receiptID, PlanDigest: source.facts.PlanDigest,
		AuthorityDigest: source.facts.AuthorityDigest, SourceRef: source.facts.SourceRef,
		SourceDigest: source.facts.SourceCanonicalDigest, Grants: orderedGrants,
		Repository: source.facts.Repository, TargetRef: source.facts.TargetRef,
		AuthorizerRef: source.facts.AuthorizerRef, ApprovedAt: source.facts.ApprovedAt,
	}
	receiptRecord, err := protocol.EncodeAuthorityApproval(receipt)
	if err != nil {
		return PreparedApproval{}, fmt.Errorf("encode authority receipt: %w", err)
	}
	return PreparedApproval{
		source: source,
		facts:  ApprovalFacts{ReceiptID: receiptID, ReceiptDigest: receiptRecord.Digest},
		receipt: protocol.EncodedRecord{
			Kind: receiptRecord.Kind, Digest: receiptRecord.Digest,
			CanonicalJSON: bytes.Clone(receiptRecord.CanonicalJSON),
		},
	}, nil
}

func validateGrantCeiling(source PreparedSource) error {
	for _, grant := range source.plan.Authority().Grants {
		canonical := grant.CanonicalJSON()
		if len(canonical) == 0 {
			return errors.New("exact plan contains an unbound authority grant")
		}
		if _, allowed := source.grants[string(canonical)]; !allowed {
			return errors.New("plan authority grant exceeds the source ceiling")
		}
	}
	return nil
}

func validateRequiredBuildGrants(plan protocol.ExactPlan, source *PreparedSource) error {
	required := []string{"inspect", "edit", "execute", "commit"}
	for _, grant := range plan.Authority().Grants {
		for index, action := range required {
			if grant.Action() == action {
				if source != nil {
					if _, allowed := source.grants[string(grant.CanonicalJSON())]; !allowed {
						return fmt.Errorf("current authority source lacks %s workspace grant", action)
					}
				}
				required = append(required[:index], required[index+1:]...)
				break
			}
		}
	}
	if len(required) != 0 {
		return fmt.Errorf("build authority requires %s workspace grant", required[0])
	}
	return nil
}

func validateBuildPermitRequest(plan protocol.ExactPlan, request BuildPermitRequest) error {
	planRecord := plan.Record()
	if planRecord.Kind != protocol.DeliveryPlanSchemaVersion || !protocol.ValidDigest(planRecord.Digest) {
		return errors.New("build permit requires an exact delivery plan")
	}
	if !protocol.ValidID(request.ControllerID) || !protocol.ValidID(request.RunID) ||
		!protocol.ValidPositiveSafeInteger(request.StateRevision) || !protocol.ValidID(request.WorkID) ||
		!protocol.ValidPositiveSafeInteger(request.WorkAttempt) ||
		!protocol.ValidDigest(request.BuilderDispatchDigest) {
		return errors.New("build permit request has invalid controller or dispatch identity")
	}
	contractDigest := request.Contract.Digest()
	contractView := request.Contract.View()
	exactContract, exists := plan.Work(request.WorkID)
	if !exists || contractView.ID != request.WorkID || !protocol.ValidDigest(contractDigest) ||
		request.Contract != exactContract || exactContract.Digest() != contractDigest {
		return errors.New("build permit request does not match the exact work contract")
	}
	return nil
}

func historicalFromPrepared(prepared PreparedApproval) HistoricalApproval {
	return HistoricalApproval{
		plan: prepared.source.plan, sourceFacts: prepared.source.facts, facts: prepared.facts,
		receipt: prepared.Receipt(),
	}
}

func parseSource(contents []byte) (parsedSource, error) {
	if len(contents) == 0 || len(contents) > MaximumAuthoritySourceBytes {
		return parsedSource{}, errors.New("authority source is empty or exceeds its byte ceiling")
	}
	canonical, err := protocol.CanonicalizeJSON(contents)
	if err != nil {
		return parsedSource{}, fmt.Errorf("authority source is not strict I-JSON: %w", err)
	}
	var source authoritySource
	sourceObject, err := decodeExactObject(canonical, &source, "authority source", []string{
		"version", "source_id", "status", "repository", "target_ref", "maximum_grants",
		"authorizer_ref", "valid_from", "valid_until",
	})
	if err != nil {
		return parsedSource{}, err
	}
	if raw := sourceObject["maximum_grants"]; len(raw) == 0 || raw[0] != '[' {
		return parsedSource{}, errors.New("authority source maximum_grants must be an array")
	}
	if !protocol.ValidPositiveSafeInteger(source.Version) {
		return parsedSource{}, errors.New("authority source version is outside the interoperable range")
	}
	if !protocol.ValidID(source.SourceID) {
		return parsedSource{}, errors.New("authority source has an invalid source_id")
	}
	if source.Status != "active" && source.Status != "revoked" {
		return parsedSource{}, fmt.Errorf("authority source has invalid status %q", source.Status)
	}
	if !protocol.ValidNonEmpty(source.Repository) || len(source.Repository) > 512 || !protocol.ValidBranchRef(source.TargetRef) {
		return parsedSource{}, errors.New("authority source has an invalid repository or target ref")
	}
	if !protocol.ValidNonEmpty(source.AuthorizerRef) || len(source.AuthorizerRef) > 512 {
		return parsedSource{}, errors.New("authority source has an invalid authorizer_ref")
	}
	grantSet := make(map[string]struct{}, len(source.MaximumGrants))
	for index, raw := range source.MaximumGrants {
		grant, err := protocol.ParseAuthorityGrant(raw)
		if err != nil {
			return parsedSource{}, fmt.Errorf("authority source maximum grant %d: %w", index, err)
		}
		canonicalGrant := grant.CanonicalJSON()
		if target, integration := grant.Integration(); integration &&
			(target.Repository != source.Repository || target.Ref != source.TargetRef) {
			return parsedSource{}, fmt.Errorf("authority source maximum grant %d: integration grant target differs from the authority source target", index)
		}
		if _, exists := grantSet[string(canonicalGrant)]; exists {
			return parsedSource{}, errors.New("authority source contains duplicate maximum grants")
		}
		grantSet[string(canonicalGrant)] = struct{}{}
	}
	if !protocol.ValidDateTime(source.ValidFrom) || !protocol.ValidDateTime(source.ValidUntil) {
		return parsedSource{}, errors.New("authority source has an invalid validity time")
	}
	validityOrder, err := protocol.CompareDateTimes(source.ValidFrom, source.ValidUntil)
	if err != nil || validityOrder >= 0 {
		return parsedSource{}, errors.New("authority source validity period is empty or reversed")
	}
	return parsedSource{
		value: source, raw: bytes.Clone(contents), canonical: canonical,
		digest: protocol.CanonicalDigest(canonical), grantSet: grantSet,
	}, nil
}

func parseProof(contents []byte) (parsedProof, error) {
	if len(contents) == 0 || len(contents) > MaximumAuthorityProofBytes {
		return parsedProof{}, errors.New("authority proof is empty or exceeds its byte ceiling")
	}
	canonical, err := protocol.CanonicalizeJSON(contents)
	if err != nil {
		return parsedProof{}, fmt.Errorf("authority proof is not strict I-JSON: %w", err)
	}
	var proof authorityProof
	if _, err := decodeExactObject(canonical, &proof, "authority proof", []string{
		"schema_version", "source_ref", "source_digest", "source_version", "plan_digest",
		"authority_digest", "key_id", "approved_at", "signature",
	}); err != nil {
		return parsedProof{}, err
	}
	if !protocol.ValidPositiveSafeInteger(proof.SourceVersion) {
		return parsedProof{}, errors.New("authority proof source version is outside the interoperable range")
	}
	if !protocol.ValidNonEmpty(proof.SourceRef) || len(proof.SourceRef) > 512 ||
		!protocol.ValidDigest(proof.SourceDigest) || !protocol.ValidDigest(proof.PlanDigest) ||
		!protocol.ValidDigest(proof.AuthorityDigest) || !validKeyID(proof.KeyID) {
		return parsedProof{}, errors.New("authority proof has invalid binding fields")
	}
	if !protocol.ValidDateTime(proof.ApprovedAt) {
		return parsedProof{}, errors.New("authority proof has an invalid approved_at")
	}
	return parsedProof{
		value: proof, raw: bytes.Clone(contents), canonical: canonical,
		digest: protocol.CanonicalDigest(canonical),
	}, nil
}

func proofMessage(proof authorityProof) ([]byte, error) {
	unsigned := unsignedAuthorityProof{
		SchemaVersion: proof.SchemaVersion, SourceRef: proof.SourceRef,
		SourceDigest: proof.SourceDigest, SourceVersion: proof.SourceVersion,
		PlanDigest: proof.PlanDigest, AuthorityDigest: proof.AuthorityDigest,
		KeyID: proof.KeyID, ApprovedAt: proof.ApprovedAt,
	}
	canonical, err := protocol.EncodeCanonical(unsigned)
	if err != nil {
		return nil, fmt.Errorf("canonicalize authority proof message: %w", err)
	}
	message := make([]byte, 0, len(proofSignatureDomain)+len(canonical))
	message = append(message, proofSignatureDomain...)
	message = append(message, canonical...)
	return message, nil
}

func decodeExactObject(
	contents []byte,
	destination any,
	label string,
	required []string,
) (map[string]json.RawMessage, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(contents, &object); err != nil {
		return nil, fmt.Errorf("decode %s object: %w", label, err)
	}
	if len(object) != len(required) {
		return nil, fmt.Errorf("%s has unknown or missing fields", label)
	}
	for _, name := range required {
		if _, exists := object[name]; !exists {
			return nil, fmt.Errorf("%s is missing exact field %q", label, name)
		}
	}
	if err := json.Unmarshal(contents, destination); err != nil {
		return nil, fmt.Errorf("decode %s: %w", label, err)
	}
	return object, nil
}

func decodeSignature(encoded string) ([]byte, error) {
	if len(encoded) != 86 {
		return nil, errors.New("authority proof signature has an invalid length")
	}
	signature, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
	if err != nil || len(signature) != ed25519.SignatureSize ||
		base64.RawURLEncoding.EncodeToString(signature) != encoded {
		return nil, errors.New("authority proof signature is not canonical unpadded base64url")
	}
	return signature, nil
}

func validateRoot(root TrustRoot) error {
	if !protocol.ValidNonEmpty(root.sourceRef) || !protocol.ValidNonEmpty(root.authorizerRef) ||
		len(root.publicKey) != ed25519.PublicKeySize || root.keyID != KeyID(root.publicKey) {
		return errors.New("authority requires a valid configured trust root")
	}
	return nil
}

func cloneSourceClosure(closure SourceClosure) SourceClosure {
	return SourceClosure{
		SourceRaw: bytes.Clone(closure.SourceRaw), SourceCanonical: bytes.Clone(closure.SourceCanonical),
		ProofRaw: bytes.Clone(closure.ProofRaw), ProofCanonical: bytes.Clone(closure.ProofCanonical),
	}
}

func cloneGrantSet(source map[string]struct{}) map[string]struct{} {
	cloned := make(map[string]struct{}, len(source))
	for grant := range source {
		cloned[grant] = struct{}{}
	}
	return cloned
}

func validKeyID(value string) bool {
	const prefix = "ed25519-sha256:"
	if len(value) != len(prefix)+64 || !strings.HasPrefix(value, prefix) {
		return false
	}
	for _, character := range value[len(prefix):] {
		if character < '0' || character > '9' && character < 'a' || character > 'f' {
			return false
		}
	}
	return true
}
