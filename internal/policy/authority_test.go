package policy

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/protocol"
)

const (
	testSourceRef     = "examples/authority-source.json"
	testAuthorizerRef = "identity:test-owner"
	testApprovedAt    = "2026-07-19T00:00:30Z"
)

type staticResolver struct {
	source             []byte
	proof              []byte
	err                error
	resolvedSourceRef  string
	resolvedPlanDigest string
	calls              int
}

type resolverFunc func(context.Context, string, string) ([]byte, []byte, error)

func (resolve resolverFunc) Resolve(
	ctx context.Context,
	sourceRef, planDigest string,
) ([]byte, []byte, error) {
	return resolve(ctx, sourceRef, planDigest)
}

type recordingLedger struct {
	sources     []PreparedSource
	approvals   []PreparedApproval
	events      []string
	sourceErr   error
	approvalErr error
}

func (ledger *recordingLedger) PutAuthoritySource(_ context.Context, source PreparedSource) error {
	ledger.events = append(ledger.events, "source")
	ledger.sources = append(ledger.sources, source)
	return ledger.sourceErr
}

func (ledger *recordingLedger) PutAuthorityApproval(_ context.Context, approval PreparedApproval) error {
	ledger.events = append(ledger.events, "approval")
	ledger.approvals = append(ledger.approvals, approval)
	return ledger.approvalErr
}

func authenticateAndMintForTest(
	ctx context.Context,
	plan protocol.ExactPlan,
	root TrustRoot,
	resolver Resolver,
	now time.Time,
) (PreparedApproval, error) {
	if resolver == nil {
		return PreparedApproval{}, errors.New("authority resolver is required")
	}
	if now.IsZero() || now.Location() != time.UTC {
		return PreparedApproval{}, errors.New("authority evaluation time must be explicit UTC")
	}
	if err := ctx.Err(); err != nil {
		return PreparedApproval{}, err
	}
	record := plan.Record()
	source, proof, err := resolver.Resolve(ctx, root.SourceRef(), record.Digest)
	if err != nil {
		return PreparedApproval{}, fmt.Errorf("resolve authority: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return PreparedApproval{}, err
	}
	prepared, err := authenticateSource(plan, root, source, proof, now, true)
	if err != nil {
		return PreparedApproval{}, err
	}
	return mintCurrentApproval(prepared, now)
}

func (resolver *staticResolver) Resolve(
	_ context.Context,
	sourceRef, planDigest string,
) ([]byte, []byte, error) {
	resolver.calls++
	resolver.resolvedSourceRef = sourceRef
	resolver.resolvedPlanDigest = planDigest
	return bytes.Clone(resolver.source), bytes.Clone(resolver.proof), resolver.err
}

type approvalFixture struct {
	plan       protocol.ExactPlan
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
	root       TrustRoot
	source     authoritySource
	proof      authorityProof
	sourceRaw  []byte
	proofRaw   []byte
	now        time.Time
}

func TestAuthorityServiceOwnsTrustTimeAndPersistence(t *testing.T) {
	fixture := newApprovalFixture(t)
	resolver := fixture.resolver()
	ledger := &recordingLedger{}
	roots := []TrustRoot{fixture.root}
	service, err := newAuthorityWithClock(roots, resolver, ledger, func() time.Time { return fixture.now })
	if err != nil {
		t.Fatal(err)
	}

	// Startup configuration is copied. Per-operation mutation of the caller's
	// original root cannot replace the service's verification capability.
	roots[0].publicKey[0] ^= 0xff
	historical, err := service.Approve(context.Background(), fixture.plan)
	if err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	if resolver.calls != 1 || resolver.resolvedSourceRef != testSourceRef ||
		resolver.resolvedPlanDigest != fixture.plan.Record().Digest {
		t.Fatalf("resolver call = (%d, %q, %q)", resolver.calls, resolver.resolvedSourceRef, resolver.resolvedPlanDigest)
	}
	if len(ledger.sources) != 0 || len(ledger.approvals) != 1 ||
		!slices.Equal(ledger.events, []string{"approval"}) {
		t.Fatalf("ledger lifecycle = sources %d approvals %d events %v",
			len(ledger.sources), len(ledger.approvals), ledger.events)
	}
	prepared := ledger.approvals[0]
	if prepared.SourceFacts().SourceCanonicalDigest == "" ||
		historical.Facts() != prepared.Facts() || historical.SourceFacts() != prepared.SourceFacts() ||
		historical.Plan().Record().Digest != fixture.plan.Record().Digest ||
		historical.Receipt().Digest != prepared.Receipt().Digest {
		t.Fatal("committed historical authority lost its exact bindings")
	}
}

func TestAuthorityServicePersistsAuthenticatedDenialsWithoutApproval(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *approvalFixture)
		want   string
	}{
		{
			name: "revoked source",
			mutate: func(t *testing.T, fixture *approvalFixture) {
				fixture.source.Status = "revoked"
				fixture.source.MaximumGrants = []json.RawMessage{}
				fixture.rebindSource(t)
			},
			want: "revoked",
		},
		{
			name: "expired source",
			mutate: func(t *testing.T, fixture *approvalFixture) {
				fixture.now = mustTime(t, "2026-07-21T00:00:00Z")
			},
			want: "expired",
		},
		{
			name: "reduced grant ceiling",
			mutate: func(t *testing.T, fixture *approvalFixture) {
				fixture.source.MaximumGrants = slices.Clone(fixture.source.MaximumGrants[1:])
				fixture.rebindSource(t)
			},
			want: "exceeds the source ceiling",
		},
		{
			name: "empty grant ceiling",
			mutate: func(t *testing.T, fixture *approvalFixture) {
				fixture.source.MaximumGrants = []json.RawMessage{}
				fixture.rebindSource(t)
			},
			want: "exceeds the source ceiling",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newApprovalFixture(t)
			test.mutate(t, &fixture)
			resolver := fixture.resolver()
			ledger := &recordingLedger{}
			service, err := newAuthorityWithClock(
				[]TrustRoot{fixture.root}, resolver, ledger, func() time.Time { return fixture.now },
			)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := service.Approve(context.Background(), fixture.plan); err == nil ||
				!strings.Contains(err.Error(), test.want) {
				t.Fatalf("Approve() error = %v, want containing %q", err, test.want)
			}
			if len(ledger.sources) != 1 || len(ledger.approvals) != 0 ||
				!slices.Equal(ledger.events, []string{"source"}) {
				t.Fatalf("denied ledger lifecycle = sources %d approvals %d events %v",
					len(ledger.sources), len(ledger.approvals), ledger.events)
			}
			if ledger.sources[0].Facts().SourceStatus != fixture.source.Status ||
				ledger.sources[0].Facts().SourceCanonicalDigest != fixture.proof.SourceDigest {
				t.Fatal("denied source was not durably bound to its authenticated proof")
			}
		})
	}
}

func TestAuthorityServiceDoesNotPersistUnauthenticatedOrFutureProof(t *testing.T) {
	fixture := newApprovalFixture(t)
	fixture.proof.ApprovedAt = "2026-07-19T00:01:01Z"
	fixture.resign(t)
	ledger := &recordingLedger{}
	service, err := newAuthorityWithClock(
		[]TrustRoot{fixture.root}, fixture.resolver(), ledger, func() time.Time { return fixture.now },
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Approve(context.Background(), fixture.plan); err == nil ||
		!strings.Contains(err.Error(), "in the future") {
		t.Fatalf("future proof error = %v", err)
	}
	if len(ledger.events) != 0 {
		t.Fatalf("unauthenticated proof reached ledger: %v", ledger.events)
	}
}

func TestAuthorityServiceSamplesCurrentTimeAfterResolution(t *testing.T) {
	fixture := newApprovalFixture(t)
	fixture.source.ValidUntil = "2026-07-19T00:01:00Z"
	fixture.rebindSource(t)
	resolved := false
	baseResolver := fixture.resolver()
	resolver := resolverFunc(func(
		ctx context.Context,
		sourceRef, planDigest string,
	) ([]byte, []byte, error) {
		source, proof, err := baseResolver.Resolve(ctx, sourceRef, planDigest)
		resolved = true
		return source, proof, err
	})
	clock := func() time.Time {
		if resolved {
			return mustTime(t, "2026-07-19T00:01:00Z")
		}
		return mustTime(t, "2026-07-19T00:00:59Z")
	}
	ledger := &recordingLedger{}
	service, err := newAuthorityWithClock([]TrustRoot{fixture.root}, resolver, ledger, clock)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Approve(context.Background(), fixture.plan); err == nil ||
		!strings.Contains(err.Error(), "expired") {
		t.Fatalf("approval that expired during resolution = %v", err)
	}
	if !resolved || len(ledger.sources) != 1 || len(ledger.approvals) != 0 {
		t.Fatalf("post-resolution expiry lifecycle = resolved %t, sources %d, approvals %d",
			resolved, len(ledger.sources), len(ledger.approvals))
	}
}

func TestAuthorityServiceConstructionAndLedgerFailuresFailClosed(t *testing.T) {
	fixture := newApprovalFixture(t)
	if _, err := NewAuthority(nil, fixture.resolver(), &recordingLedger{}); err == nil {
		t.Fatal("authority service accepted no roots")
	}
	if _, err := NewAuthority([]TrustRoot{fixture.root, fixture.root}, fixture.resolver(), &recordingLedger{}); err == nil {
		t.Fatal("authority service accepted duplicate source roots")
	}
	if _, err := NewAuthority([]TrustRoot{fixture.root}, nil, &recordingLedger{}); err == nil {
		t.Fatal("authority service accepted no resolver")
	}

	t.Run("atomic approval persistence", func(t *testing.T) {
		ledger := &recordingLedger{approvalErr: errors.New("commit failed")}
		service, err := newAuthorityWithClock(
			[]TrustRoot{fixture.root}, fixture.resolver(), ledger, func() time.Time { return fixture.now },
		)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.Approve(context.Background(), fixture.plan); err == nil ||
			!strings.Contains(err.Error(), "commit failed") {
			t.Fatalf("approval persistence error = %v", err)
		}
	})

	t.Run("denied source persistence", func(t *testing.T) {
		revoked := newApprovalFixture(t)
		revoked.source.Status = "revoked"
		revoked.rebindSource(t)
		ledger := &recordingLedger{sourceErr: errors.New("source commit failed")}
		service, err := newAuthorityWithClock(
			[]TrustRoot{revoked.root}, revoked.resolver(), ledger, func() time.Time { return revoked.now },
		)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.Approve(context.Background(), revoked.plan); err == nil ||
			!strings.Contains(err.Error(), "source commit failed") {
			t.Fatalf("denied source persistence error = %v", err)
		}
	})
}

func TestAuthorityAuthenticationEmitsDeterministicReceipt(t *testing.T) {
	fixture := newApprovalFixture(t)
	resolver := fixture.resolver()

	approval, err := authenticateAndMintForTest(context.Background(), fixture.plan, fixture.root, resolver, fixture.now)
	if err != nil {
		t.Fatalf("authenticateAndMintForTest() error = %v", err)
	}
	if resolver.calls != 1 || resolver.resolvedSourceRef != testSourceRef ||
		resolver.resolvedPlanDigest != fixture.plan.Record().Digest {
		t.Fatalf("resolver call = (%d, %q, %q)", resolver.calls, resolver.resolvedSourceRef, resolver.resolvedPlanDigest)
	}

	sourceClosure := approval.Source().Closure()
	receiptRecord := approval.Receipt()
	wantSourceCanonical, err := protocol.CanonicalizeJSON(fixture.sourceRaw)
	if err != nil {
		t.Fatal(err)
	}
	wantProofCanonical, err := protocol.CanonicalizeJSON(fixture.proofRaw)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(sourceClosure.SourceRaw, fixture.sourceRaw) ||
		!bytes.Equal(sourceClosure.SourceCanonical, wantSourceCanonical) ||
		!bytes.Equal(sourceClosure.ProofRaw, fixture.proofRaw) ||
		!bytes.Equal(sourceClosure.ProofCanonical, wantProofCanonical) {
		t.Fatal("approval did not retain exact raw and canonical authority closure")
	}
	if bytes.Equal(sourceClosure.SourceRaw, sourceClosure.SourceCanonical) ||
		bytes.Equal(sourceClosure.ProofRaw, sourceClosure.ProofCanonical) {
		t.Fatal("noncanonical strict input unexpectedly equals its canonical representation")
	}

	receipt, err := protocol.ParseAuthorityApproval(receiptRecord.CanonicalJSON)
	if err != nil {
		t.Fatalf("decode receipt: %v", err)
	}
	authority := fixture.plan.Authority()
	target := fixture.plan.Target()
	if receipt.SchemaVersion != "control-receipt-v1" || receipt.Kind != "authority_approval" ||
		receipt.PlanDigest != fixture.plan.Record().Digest || receipt.AuthorityDigest != authority.Digest ||
		receipt.SourceRef != testSourceRef || receipt.SourceDigest != protocol.CanonicalDigest(wantSourceCanonical) ||
		receipt.Repository != target.Repository || receipt.TargetRef != target.Ref ||
		receipt.AuthorizerRef != testAuthorizerRef || receipt.ApprovedAt != testApprovedAt {
		t.Fatalf("receipt does not carry exact authenticated bindings: %#v", receipt)
	}
	if len(receipt.ReceiptID) != len("authority-")+sha256.Size*2 || !protocol.ValidID(receipt.ReceiptID) {
		t.Fatalf("receipt id = %q", receipt.ReceiptID)
	}
	wantProofDigest := protocol.CanonicalDigest(wantProofCanonical)
	if receipt.ReceiptID != "authority-"+strings.TrimPrefix(wantProofDigest, "sha256:") {
		t.Fatalf("receipt id = %q, want proof-bound identity", receipt.ReceiptID)
	}
	const wantReceiptID = "authority-f93d1ddc59238a186083adf4628adff8b0466ae1962430a54fd31993b65f7f2c"
	const wantReceiptDigest = "sha256:d0ffc66995eab46ee7842dcd51b0564f480bfa02262fbdd36df311a187574159"
	const wantReceiptJSON = `{"approved_at":"2026-07-19T00:00:30Z","authority_digest":"sha256:20d9d443a98f0a43d64e4eaffdb29bf111c1a00f7c42847094a5a57e81d8da4b","authorizer_ref":"identity:test-owner","grants":[{"action":"inspect","target":"workspace"},{"action":"edit","target":"workspace"},{"action":"execute","target":"workspace"},{"action":"commit","target":"workspace"},{"action":"integrate","target":{"ref":"refs/heads/main","repository":"local:example"}}],"kind":"authority_approval","plan_digest":"sha256:5f44521823b466b350b572813c7aa8677a5e487e4eadfc8f35fde23580f5422f","receipt_id":"authority-f93d1ddc59238a186083adf4628adff8b0466ae1962430a54fd31993b65f7f2c","repository":"local:example","schema_version":"control-receipt-v1","source_digest":"sha256:1d884d087a97dd6a9acdf8d8396796482eae9233109ec38aff55021f2449ffe9","source_ref":"examples/authority-source.json","target_ref":"refs/heads/main"}`
	if receipt.ReceiptID != wantReceiptID || receiptRecord.Digest != wantReceiptDigest ||
		string(receiptRecord.CanonicalJSON) != wantReceiptJSON {
		t.Fatalf("deterministic authority receipt drifted: id %q digest %q json %s",
			receipt.ReceiptID, receiptRecord.Digest, receiptRecord.CanonicalJSON)
	}
	if len(receipt.Grants) != len(authority.Grants) {
		t.Fatalf("receipt grants = %d, want %d", len(receipt.Grants), len(authority.Grants))
	}
	for index, grant := range authority.Grants {
		canonical, err := protocol.CanonicalizeJSON(receipt.Grants[index])
		if err != nil || !bytes.Equal(canonical, grant.CanonicalJSON()) {
			t.Fatalf("receipt grant %d does not preserve exact plan order", index)
		}
	}
	if receiptRecord.Kind != AuthorityReceiptKind ||
		receiptRecord.Digest != protocol.CanonicalDigest(receiptRecord.CanonicalJSON) {
		t.Fatalf("receipt record = %#v", receiptRecord)
	}
	approvalFacts := approval.Facts()
	sourceFacts := approval.SourceFacts()
	if approvalFacts.ReceiptID != receipt.ReceiptID || approvalFacts.ReceiptDigest != receiptRecord.Digest ||
		sourceFacts.PlanDigest != fixture.plan.Record().Digest || sourceFacts.AuthorityDigest != authority.Digest ||
		sourceFacts.SourceRef != testSourceRef || sourceFacts.SourceID != fixture.source.SourceID ||
		sourceFacts.SourceVersion != fixture.source.Version || sourceFacts.SourceStatus != "active" ||
		sourceFacts.SourceCanonicalDigest != receipt.SourceDigest ||
		sourceFacts.SourceRawDigest != protocol.RawDigest(fixture.sourceRaw) ||
		sourceFacts.Repository != target.Repository || sourceFacts.TargetRef != target.Ref ||
		sourceFacts.AuthorizerRef != testAuthorizerRef || sourceFacts.ValidFrom != fixture.source.ValidFrom ||
		sourceFacts.ValidUntil != fixture.source.ValidUntil || sourceFacts.ProofRawDigest != protocol.RawDigest(fixture.proofRaw) ||
		sourceFacts.ProofCanonicalDigest != protocol.CanonicalDigest(wantProofCanonical) ||
		sourceFacts.RootKeyID != fixture.root.KeyID() || sourceFacts.ApprovedAt != testApprovedAt {
		t.Fatalf("approval facts lost an immutable binding: %#v %#v", sourceFacts, approvalFacts)
	}
	if approval.Plan().Record().Digest != fixture.plan.Record().Digest {
		t.Fatal("prepared approval did not retain the exact plan")
	}

	repeated, err := authenticateAndMintForTest(context.Background(), fixture.plan, fixture.root, fixture.resolver(), fixture.now)
	if err != nil {
		t.Fatalf("repeat authenticateAndMintForTest() error = %v", err)
	}
	if repeated.Receipt().Digest != approval.Receipt().Digest ||
		!bytes.Equal(repeated.Receipt().CanonicalJSON, approval.Receipt().CanonicalJSON) {
		t.Fatal("exact approval retry changed its deterministic receipt")
	}

	// The source intentionally presents its ceiling in reverse order. Authority
	// is a set ceiling, while the receipt must preserve plan grant order.
	if bytes.Equal(fixture.source.MaximumGrants[0], authority.Grants[0].CanonicalJSON()) {
		t.Fatal("fixture did not exercise set-vs-order behavior")
	}
}

func TestTrustRootAndPreparedApprovalAreDefensiveCapabilities(t *testing.T) {
	fixture := newApprovalFixture(t)
	public := bytes.Clone(fixture.publicKey)
	root, err := NewTrustRoot(testSourceRef, testAuthorizerRef, public)
	if err != nil {
		t.Fatal(err)
	}
	public[0] ^= 0xff
	approval, err := authenticateAndMintForTest(context.Background(), fixture.plan, root, fixture.resolver(), fixture.now)
	if err != nil {
		t.Fatalf("root retained caller-owned public key memory: %v", err)
	}

	firstSource := approval.Source().Closure()
	firstApproval := approval.Receipt()
	firstSource.SourceRaw[0] ^= 0xff
	firstSource.SourceCanonical[0] ^= 0xff
	firstSource.ProofRaw[0] ^= 0xff
	firstSource.ProofCanonical[0] ^= 0xff
	firstApproval.CanonicalJSON[0] ^= 0xff
	secondSource := approval.Source().Closure()
	secondApproval := approval.Receipt()
	if secondSource.SourceRaw[0] != fixture.sourceRaw[0] || secondSource.ProofRaw[0] != fixture.proofRaw[0] ||
		secondSource.SourceCanonical[0] != '{' || secondSource.ProofCanonical[0] != '{' ||
		secondApproval.CanonicalJSON[0] != '{' {
		t.Fatal("prepared approval exposed mutable internal bytes")
	}

	if _, err := NewTrustRoot("", testAuthorizerRef, fixture.publicKey); err == nil {
		t.Fatal("empty source reference was accepted")
	}
	if _, err := NewTrustRoot(testSourceRef, "", fixture.publicKey); err == nil {
		t.Fatal("empty authorizer reference was accepted")
	}
	if _, err := NewTrustRoot(testSourceRef, testAuthorizerRef, fixture.publicKey[:31]); err == nil {
		t.Fatal("short public key was accepted")
	}
}

func TestAuthorityAuthenticationRejectsForgeryMutationAndBindingSwaps(t *testing.T) {
	base := newApprovalFixture(t)
	otherPublic, otherPrivate, err := ed25519.GenerateKey(strings.NewReader(strings.Repeat("x", 64)))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		mutate func(*testing.T, *approvalFixture)
		want   string
	}{
		{
			name: "forged signing key",
			mutate: func(t *testing.T, fixture *approvalFixture) {
				fixture.proof.Signature = signProof(t, fixture.proof, otherPrivate)
				fixture.proofRaw = indentedJSON(t, fixture.proof)
			},
			want: "signature is invalid",
		},
		{
			name: "source content mutation",
			mutate: func(t *testing.T, fixture *approvalFixture) {
				fixture.source.SourceID = "mutated-source"
				fixture.sourceRaw = indentedJSON(t, fixture.source)
			},
			want: "proof does not match",
		},
		{
			name: "source ref swap",
			mutate: func(t *testing.T, fixture *approvalFixture) {
				fixture.proof.SourceRef = "authority:other"
				fixture.resign(t)
			},
			want: "proof does not match",
		},
		{
			name: "source digest swap",
			mutate: func(t *testing.T, fixture *approvalFixture) {
				fixture.proof.SourceDigest = fixedDigest("0")
				fixture.resign(t)
			},
			want: "proof does not match",
		},
		{
			name: "source version swap",
			mutate: func(t *testing.T, fixture *approvalFixture) {
				fixture.proof.SourceVersion++
				fixture.resign(t)
			},
			want: "proof does not match",
		},
		{
			name: "plan digest swap",
			mutate: func(t *testing.T, fixture *approvalFixture) {
				fixture.proof.PlanDigest = fixedDigest("1")
				fixture.resign(t)
			},
			want: "proof does not match",
		},
		{
			name: "authority digest swap",
			mutate: func(t *testing.T, fixture *approvalFixture) {
				fixture.proof.AuthorityDigest = fixedDigest("2")
				fixture.resign(t)
			},
			want: "proof does not match",
		},
		{
			name: "key id swap",
			mutate: func(t *testing.T, fixture *approvalFixture) {
				fixture.proof.KeyID = KeyID(otherPublic)
				fixture.resign(t)
			},
			want: "proof does not match",
		},
		{
			name: "authorizer swap",
			mutate: func(t *testing.T, fixture *approvalFixture) {
				fixture.source.AuthorizerRef = "identity:attacker"
				fixture.rebindSource(t)
			},
			want: "authorizer does not match",
		},
		{
			name: "repository swap",
			mutate: func(t *testing.T, fixture *approvalFixture) {
				fixture.source.Repository = "local:other"
				fixture.source.MaximumGrants = workspaceGrants(t)
				fixture.rebindSource(t)
			},
			want: "target does not match",
		},
		{
			name: "target ref swap",
			mutate: func(t *testing.T, fixture *approvalFixture) {
				fixture.source.TargetRef = "refs/heads/other"
				fixture.source.MaximumGrants = workspaceGrants(t)
				fixture.rebindSource(t)
			},
			want: "target does not match",
		},
		{
			name: "root pins another source",
			mutate: func(t *testing.T, fixture *approvalFixture) {
				fixture.root, _ = NewTrustRoot("authority:other", testAuthorizerRef, fixture.publicKey)
			},
			want: "plan authority source does not match",
		},
		{
			name: "padded signature",
			mutate: func(t *testing.T, fixture *approvalFixture) {
				decoded, err := base64.RawURLEncoding.DecodeString(fixture.proof.Signature)
				if err != nil {
					t.Fatal(err)
				}
				fixture.proof.Signature = base64.URLEncoding.EncodeToString(decoded)
				fixture.proofRaw = indentedJSON(t, fixture.proof)
			},
			want: "invalid length",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := base.clone()
			test.mutate(t, &fixture)
			_, err := authenticateAndMintForTest(context.Background(), fixture.plan, fixture.root, fixture.resolver(), fixture.now)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("authenticateAndMintForTest() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestAuthorityAuthenticationRejectsStrictShapeAndGrantViolations(t *testing.T) {
	t.Run("strict source and proof", func(t *testing.T) {
		fixtures := []struct {
			name   string
			mutate func(*approvalFixture)
			want   string
		}{
			{
				name: "source unknown field",
				mutate: func(fixture *approvalFixture) {
					fixture.sourceRaw = append(fixture.sourceRaw[:len(fixture.sourceRaw)-2], []byte(",\n  \"surprise\": true\n}")...)
				},
				want: "unknown or missing fields",
			},
			{
				name: "source duplicate field",
				mutate: func(fixture *approvalFixture) {
					fixture.sourceRaw = []byte(`{"version":1,"version":2}`)
				},
				want: "duplicate object name",
			},
			{
				name: "source case folded field",
				mutate: func(fixture *approvalFixture) {
					fixture.sourceRaw = bytes.Replace(fixture.sourceRaw, []byte(`"source_id"`), []byte(`"Source_ID"`), 1)
				},
				want: "missing exact field",
			},
			{
				name: "proof unknown field",
				mutate: func(fixture *approvalFixture) {
					fixture.proofRaw = append(fixture.proofRaw[:len(fixture.proofRaw)-2], []byte(",\n  \"surprise\": true\n}")...)
				},
				want: "unknown or missing fields",
			},
			{
				name: "proof duplicate field",
				mutate: func(fixture *approvalFixture) {
					fixture.proofRaw = []byte(`{"schema_version":"sworn-authority-proof-v1","schema_version":"other"}`)
				},
				want: "duplicate object name",
			},
			{
				name: "proof case folded field",
				mutate: func(fixture *approvalFixture) {
					fixture.proofRaw = bytes.Replace(fixture.proofRaw, []byte(`"plan_digest"`), []byte(`"Plan_Digest"`), 1)
				},
				want: "missing exact field",
			},
		}
		for _, test := range fixtures {
			t.Run(test.name, func(t *testing.T) {
				fixture := newApprovalFixture(t)
				test.mutate(&fixture)
				_, err := authenticateAndMintForTest(context.Background(), fixture.plan, fixture.root, fixture.resolver(), fixture.now)
				if err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf("authenticateAndMintForTest() error = %v, want containing %q", err, test.want)
				}
			})
		}
	})

	tests := []struct {
		name   string
		mutate func(*testing.T, *approvalFixture)
		want   string
	}{
		{
			name: "revoked source",
			mutate: func(t *testing.T, fixture *approvalFixture) {
				fixture.source.Status = "revoked"
				fixture.rebindSource(t)
			},
			want: "revoked",
		},
		{
			name: "zero version",
			mutate: func(t *testing.T, fixture *approvalFixture) {
				fixture.source.Version = 0
				fixture.rebindSource(t)
			},
			want: "version is outside",
		},
		{
			name: "missing plan grant",
			mutate: func(t *testing.T, fixture *approvalFixture) {
				fixture.source.MaximumGrants = slices.Clone(fixture.source.MaximumGrants[1:])
				fixture.rebindSource(t)
			},
			want: "exceeds the source ceiling",
		},
		{
			name: "null maximum grants",
			mutate: func(t *testing.T, fixture *approvalFixture) {
				fixture.source.MaximumGrants = nil
				fixture.rebindSource(t)
			},
			want: "must be an array",
		},
		{
			name: "duplicate maximum grant",
			mutate: func(t *testing.T, fixture *approvalFixture) {
				fixture.source.MaximumGrants = append(fixture.source.MaximumGrants, bytes.Clone(fixture.source.MaximumGrants[0]))
				fixture.rebindSource(t)
			},
			want: "duplicate maximum grants",
		},
		{
			name: "unknown maximum grant",
			mutate: func(t *testing.T, fixture *approvalFixture) {
				fixture.source.MaximumGrants[0] = json.RawMessage(`{"action":"publish","target":"workspace"}`)
				fixture.rebindSource(t)
			},
			want: "unknown action",
		},
		{
			name: "case folded maximum grant field",
			mutate: func(t *testing.T, fixture *approvalFixture) {
				fixture.source.MaximumGrants[0] = json.RawMessage(`{"Action":"inspect","target":"workspace"}`)
				fixture.rebindSource(t)
			},
			want: "missing required property",
		},
		{
			name: "case folded integration target field",
			mutate: func(t *testing.T, fixture *approvalFixture) {
				fixture.source.MaximumGrants[0] = json.RawMessage(`{"action":"integrate","target":{"Repository":"local:example","ref":"refs/heads/main"}}`)
				fixture.rebindSource(t)
			},
			want: "missing required property",
		},
		{
			name: "integration grant escapes source target",
			mutate: func(t *testing.T, fixture *approvalFixture) {
				fixture.source.MaximumGrants[0] = json.RawMessage(`{"action":"integrate","target":{"repository":"local:example","ref":"refs/heads/other"}}`)
				fixture.rebindSource(t)
			},
			want: "differs from the authority source target",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newApprovalFixture(t)
			test.mutate(t, &fixture)
			_, err := authenticateAndMintForTest(context.Background(), fixture.plan, fixture.root, fixture.resolver(), fixture.now)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("authenticateAndMintForTest() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestAuthorityAuthenticationEnforcesExactTimeBoundaries(t *testing.T) {
	tests := []struct {
		name       string
		createdAt  string
		validFrom  string
		validUntil string
		approvedAt string
		now        string
		want       string
	}{
		{
			name: "approval equals plan and valid from", createdAt: testApprovedAt,
			validFrom: testApprovedAt, validUntil: "2026-07-19T00:01:00Z",
			approvedAt: testApprovedAt, now: testApprovedAt,
		},
		{
			name: "now immediately before expiry", createdAt: "2026-07-19T00:00:00Z",
			validFrom: "2026-07-19T00:00:00Z", validUntil: "2026-07-19T00:01:00.0000000001Z",
			approvedAt: testApprovedAt, now: "2026-07-19T00:01:00Z",
		},
		{
			name: "approval predates plan", createdAt: "2026-07-19T00:00:31Z",
			validFrom: "2026-07-19T00:00:00Z", validUntil: "2026-07-19T00:02:00Z",
			approvedAt: testApprovedAt, now: "2026-07-19T00:01:00Z", want: "predates",
		},
		{
			name: "approval before valid from", createdAt: "2026-07-19T00:00:00Z",
			validFrom: "2026-07-19T00:00:31Z", validUntil: "2026-07-19T00:02:00Z",
			approvedAt: testApprovedAt, now: "2026-07-19T00:01:00Z", want: "outside the source validity",
		},
		{
			name: "approval equals valid until", createdAt: "2026-07-19T00:00:00Z",
			validFrom: "2026-07-19T00:00:00Z", validUntil: testApprovedAt,
			approvedAt: testApprovedAt, now: testApprovedAt, want: "outside the source validity",
		},
		{
			name: "approval after now", createdAt: "2026-07-19T00:00:00Z",
			validFrom: "2026-07-19T00:00:00Z", validUntil: "2026-07-19T00:02:00Z",
			approvedAt: testApprovedAt, now: "2026-07-19T00:00:29.999999999Z", want: "in the future",
		},
		{
			name: "now equals valid until", createdAt: "2026-07-19T00:00:00Z",
			validFrom: "2026-07-19T00:00:00Z", validUntil: "2026-07-19T00:01:00Z",
			approvedAt: testApprovedAt, now: "2026-07-19T00:01:00Z", want: "expired",
		},
		{
			name: "empty validity period", createdAt: "2026-07-19T00:00:00Z",
			validFrom: "2026-07-19T00:01:00Z", validUntil: "2026-07-19T00:01:00Z",
			approvedAt: testApprovedAt, now: "2026-07-19T00:00:30Z", want: "empty or reversed",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newApprovalFixture(t)
			fixture.plan = planWithCreatedAt(t, fixture.plan, test.createdAt)
			fixture.source.ValidFrom = test.validFrom
			fixture.source.ValidUntil = test.validUntil
			fixture.proof.ApprovedAt = test.approvedAt
			fixture.rebindSource(t)
			fixture.now = mustTime(t, test.now)
			_, err := authenticateAndMintForTest(context.Background(), fixture.plan, fixture.root, fixture.resolver(), fixture.now)
			if test.want == "" && err != nil {
				t.Fatalf("authenticateAndMintForTest() error = %v", err)
			}
			if test.want != "" && (err == nil || !strings.Contains(err.Error(), test.want)) {
				t.Fatalf("authenticateAndMintForTest() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestRestoreHistoricalApprovalAuthenticatesHistoryWithoutCurrentAuthority(t *testing.T) {
	fixture := newApprovalFixture(t)
	approval, err := authenticateAndMintForTest(context.Background(), fixture.plan, fixture.root, fixture.resolver(), fixture.now)
	if err != nil {
		t.Fatal(err)
	}
	sourceClosure := approval.Source().Closure()
	receiptRecord := approval.Receipt()

	// Current admission has expired, but the archived approval was made while
	// active and remains verifiable historical truth without re-resolution.
	fixture.now = mustTime(t, "2026-07-22T00:00:00Z")
	if _, err := authenticateAndMintForTest(context.Background(), fixture.plan, fixture.root, fixture.resolver(), fixture.now); err == nil ||
		!strings.Contains(err.Error(), "expired") {
		t.Fatalf("current expired authenticateAndMintForTest() error = %v", err)
	}
	restored, err := RestoreHistoricalApproval(fixture.plan, fixture.root, sourceClosure, receiptRecord)
	if err != nil {
		t.Fatalf("RestoreHistoricalApproval() error = %v", err)
	}
	if restored.Receipt().Digest != approval.Receipt().Digest {
		t.Fatal("restored historical receipt changed identity")
	}

	tests := []struct {
		name   string
		mutate func(*SourceClosure, *protocol.EncodedRecord)
	}{
		{"source raw", func(source *SourceClosure, _ *protocol.EncodedRecord) { source.SourceRaw[10] ^= 1 }},
		{"source canonical", func(source *SourceClosure, _ *protocol.EncodedRecord) { source.SourceCanonical[0] ^= 1 }},
		{"proof raw", func(source *SourceClosure, _ *protocol.EncodedRecord) { source.ProofRaw[10] ^= 1 }},
		{"proof canonical", func(source *SourceClosure, _ *protocol.EncodedRecord) { source.ProofCanonical[0] ^= 1 }},
		{"receipt bytes", func(_ *SourceClosure, receipt *protocol.EncodedRecord) { receipt.CanonicalJSON[0] ^= 1 }},
		{"receipt digest", func(_ *SourceClosure, receipt *protocol.EncodedRecord) { receipt.Digest = fixedDigest("f") }},
		{"receipt kind", func(_ *SourceClosure, receipt *protocol.EncodedRecord) { receipt.Kind = "other" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tamperedSource := approval.Source().Closure()
			tamperedReceipt := approval.Receipt()
			test.mutate(&tamperedSource, &tamperedReceipt)
			if _, err := RestoreHistoricalApproval(fixture.plan, fixture.root, tamperedSource, tamperedReceipt); err == nil {
				t.Fatal("tampered historical closure was restored")
			}
		})
	}
}

func TestAuthorityAuthenticationPropagatesResolverAndContextFailure(t *testing.T) {
	fixture := newApprovalFixture(t)
	resolver := fixture.resolver()
	resolver.err = errors.New("source unavailable")
	if _, err := authenticateAndMintForTest(context.Background(), fixture.plan, fixture.root, resolver, fixture.now); err == nil ||
		!strings.Contains(err.Error(), "source unavailable") {
		t.Fatalf("resolver error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	resolver = fixture.resolver()
	if _, err := authenticateAndMintForTest(ctx, fixture.plan, fixture.root, resolver, fixture.now); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled authenticateAndMintForTest() error = %v", err)
	}
	if resolver.calls != 0 {
		t.Fatal("cancelled authority request reached resolver")
	}
}

func TestAuthorityAuthenticationRequiresExplicitUTCNow(t *testing.T) {
	fixture := newApprovalFixture(t)
	fixture.now = fixture.now.In(time.FixedZone("zero-offset-but-not-UTC", 0))
	if _, err := authenticateAndMintForTest(context.Background(), fixture.plan, fixture.root, fixture.resolver(), fixture.now); err == nil ||
		!strings.Contains(err.Error(), "explicit UTC") {
		t.Fatalf("non-explicit UTC evaluation error = %v", err)
	}
}

func newApprovalFixture(t *testing.T) approvalFixture {
	t.Helper()
	planBytes, err := os.ReadFile("../protocol/snapshot/examples/standard-plan.json")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := protocol.ParseDeliveryPlan(planBytes)
	if err != nil {
		t.Fatal(err)
	}
	seed := sha256.Sum256([]byte("sworn authority test root"))
	privateKey := ed25519.NewKeyFromSeed(seed[:])
	publicKey := bytes.Clone(privateKey.Public().(ed25519.PublicKey))
	root, err := NewTrustRoot(testSourceRef, testAuthorizerRef, publicKey)
	if err != nil {
		t.Fatal(err)
	}

	authority := plan.Authority()
	maximumGrants := make([]json.RawMessage, 0, len(authority.Grants))
	for index := len(authority.Grants) - 1; index >= 0; index-- {
		maximumGrants = append(maximumGrants, json.RawMessage(authority.Grants[index].CanonicalJSON()))
	}
	source := authoritySource{
		Version: 1, SourceID: "local-main", Status: "active",
		Repository: plan.Target().Repository, TargetRef: plan.Target().Ref,
		MaximumGrants: maximumGrants, AuthorizerRef: testAuthorizerRef,
		ValidFrom: "2026-07-18T00:00:00Z", ValidUntil: "2026-07-21T00:00:00Z",
	}
	fixture := approvalFixture{
		plan: plan, privateKey: privateKey, publicKey: publicKey, root: root,
		source: source, now: mustTime(t, "2026-07-19T00:01:00Z"),
	}
	fixture.rebindSource(t)
	return fixture
}

func (fixture *approvalFixture) rebindSource(t *testing.T) {
	t.Helper()
	approvedAt := fixture.proof.ApprovedAt
	if approvedAt == "" {
		approvedAt = testApprovedAt
	}
	fixture.sourceRaw = indentedJSON(t, fixture.source)
	canonical, err := protocol.CanonicalizeJSON(fixture.sourceRaw)
	if err != nil {
		t.Fatal(err)
	}
	fixture.proof = authorityProof{
		SchemaVersion: AuthorityProofSchemaVersion,
		SourceRef:     testSourceRef, SourceDigest: protocol.CanonicalDigest(canonical),
		SourceVersion: fixture.source.Version, PlanDigest: fixture.plan.Record().Digest,
		AuthorityDigest: fixture.plan.Authority().Digest, KeyID: fixture.root.KeyID(),
		ApprovedAt: approvedAt,
	}
	fixture.resign(t)
}

func (fixture *approvalFixture) resign(t *testing.T) {
	t.Helper()
	fixture.proof.Signature = signProof(t, fixture.proof, fixture.privateKey)
	fixture.proofRaw = indentedJSON(t, fixture.proof)
}

func (fixture approvalFixture) resolver() *staticResolver {
	return &staticResolver{source: bytes.Clone(fixture.sourceRaw), proof: bytes.Clone(fixture.proofRaw)}
}

func (fixture approvalFixture) clone() approvalFixture {
	fixture.privateKey = bytes.Clone(fixture.privateKey)
	fixture.publicKey = bytes.Clone(fixture.publicKey)
	fixture.source.MaximumGrants = cloneRawMessages(fixture.source.MaximumGrants)
	fixture.sourceRaw = bytes.Clone(fixture.sourceRaw)
	fixture.proofRaw = bytes.Clone(fixture.proofRaw)
	return fixture
}

func signProof(t *testing.T, proof authorityProof, privateKey ed25519.PrivateKey) string {
	t.Helper()
	message, err := proofMessage(proof)
	if err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, message))
}

func indentedJSON(t *testing.T, value any) []byte {
	t.Helper()
	contents, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(contents, '\n')
}

func cloneRawMessages(values []json.RawMessage) []json.RawMessage {
	cloned := make([]json.RawMessage, len(values))
	for index := range values {
		cloned[index] = bytes.Clone(values[index])
	}
	return cloned
}

func workspaceGrants(t *testing.T) []json.RawMessage {
	t.Helper()
	return []json.RawMessage{
		json.RawMessage(`{"action":"inspect","target":"workspace"}`),
		json.RawMessage(`{"action":"edit","target":"workspace"}`),
		json.RawMessage(`{"action":"execute","target":"workspace"}`),
		json.RawMessage(`{"action":"commit","target":"workspace"}`),
	}
}

func fixedDigest(character string) string {
	return "sha256:" + strings.Repeat(character, 64)
}

func mustTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func planWithCreatedAt(t *testing.T, plan protocol.ExactPlan, createdAt string) protocol.ExactPlan {
	t.Helper()
	var object map[string]any
	if err := json.Unmarshal(plan.Record().CanonicalJSON, &object); err != nil {
		t.Fatal(err)
	}
	object["created_at"] = createdAt
	contents, err := json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := protocol.ParseDeliveryPlan(contents)
	if err != nil {
		t.Fatalf("parse plan with created_at %q: %v", createdAt, err)
	}
	return updated
}
