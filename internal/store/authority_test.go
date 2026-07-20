package store

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/policy"
	"github.com/swornagent/sworn/internal/protocol"
)

const testProofDomain = "sworn/authority-proof/v1\x00"

type authorityResolver struct {
	source []byte
	proof  []byte
}

func (resolver authorityResolver) Resolve(context.Context, string, string) ([]byte, []byte, error) {
	return append([]byte(nil), resolver.source...), append([]byte(nil), resolver.proof...), nil
}

func TestAuthorityServicePersistsAtomicClosureAndRestoresAfterRestart(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "control.db")
	control := openTestStore(t, path)
	plan := exampleExactPlan(t)
	authority, root, _ := authorityFixture(t, control, plan, 1, nil, false, nil)
	historical, err := authority.Approve(ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	digest := historical.Facts().ReceiptDigest
	repeated, err := authority.Approve(ctx, plan)
	if err != nil || repeated.Facts() != historical.Facts() {
		t.Fatalf("idempotent authority approval = %#v, %v; want %#v", repeated.Facts(), err, historical.Facts())
	}
	assertAuthorityClosureCounts(t, control, 1, 1, 1, 2, 3)
	if err := control.Close(); err != nil {
		t.Fatal(err)
	}

	reopened := openTestStore(t, path)
	t.Cleanup(func() { _ = reopened.Close() })
	restored, err := reopened.AuthorityApproval(ctx, digest, root)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Facts() != historical.Facts() || restored.SourceFacts() != historical.SourceFacts() {
		t.Fatal("restart restoration lost authenticated authority facts")
	}
	seed := sha256.Sum256([]byte("wrong authority root"))
	wrongPrivate := ed25519.NewKeyFromSeed(seed[:])
	wrongRoot, err := policy.NewTrustRoot(root.SourceRef(), root.AuthorizerRef(), wrongPrivate.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.AuthorityApproval(ctx, digest, wrongRoot); err == nil {
		t.Fatal("archived approval restored under an untrusted key")
	}
}

func TestAuthoritySourceVersionIsMonotonicButHistoricalReplayRemainsIdempotent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	control := openTestStore(t, filepath.Join(t.TempDir(), "control.db"))
	t.Cleanup(func() { _ = control.Close() })
	plan := exampleExactPlan(t)
	v1, _, key := authorityFixture(t, control, plan, 1, nil, false, nil)
	if _, err := v1.Approve(ctx, plan); err != nil {
		t.Fatal(err)
	}
	v2, _, _ := authorityFixture(t, control, plan, 2, key, false, nil)
	if _, err := v2.Approve(ctx, plan); err != nil {
		t.Fatal(err)
	}
	if _, err := v1.Approve(ctx, plan); err != nil {
		t.Fatalf("exact historical replay after source advance: %v", err)
	}

	fork, _, _ := authorityFixture(t, control, plan, 2, key, false, func(source map[string]any) {
		source["valid_until"] = "9998-12-31T23:59:59Z"
	})
	if _, err := fork.Approve(ctx, plan); err == nil || !strings.Contains(err.Error(), "fork") {
		t.Fatalf("same-version source fork error = %v", err)
	}

	limitedPlan := planWithGrantActions(t, plan, "inspect")
	limitedSource := func(source map[string]any) {
		source["maximum_grants"] = grantsWithActions(t, source["maximum_grants"], "inspect")
	}
	separate := openTestStore(t, filepath.Join(t.TempDir(), "rollback.db"))
	t.Cleanup(func() { _ = separate.Close() })
	limitedV1, _, limitedKey := authorityFixture(t, separate, limitedPlan, 1, nil, false, limitedSource)
	if _, err := limitedV1.Approve(ctx, limitedPlan); err != nil {
		t.Fatal(err)
	}
	limitedV2, _, _ := authorityFixture(t, separate, limitedPlan, 2, limitedKey, false, limitedSource)
	if _, err := limitedV2.Approve(ctx, limitedPlan); err != nil {
		t.Fatal(err)
	}
	expandedPlan := planWithGrantActions(t, plan, "inspect", "edit")
	oldSourceNewProof, _, _ := authorityFixture(t, separate, expandedPlan, 1, limitedKey, false, limitedSource)
	if _, err := oldSourceNewProof.Approve(ctx, expandedPlan); err == nil || !strings.Contains(err.Error(), "rollback") {
		t.Fatalf("old source with a new proof after head advance = %v", err)
	}
	assertAuthorityClosureCounts(t, separate, 2, 2, 2, 3, 6)
	assertAuthorityClosureCounts(t, control, 2, 2, 2, 3, 6)
}

func TestLegacyStructuralAuthorityIdentityCannotPreemptAuthenticatedApproval(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	plan := exampleExactPlan(t)

	scratch := openTestStore(t, filepath.Join(t.TempDir(), "scratch.db"))
	authority, _, _ := authorityFixture(t, scratch, plan, 1, nil, false, nil)
	historical, err := authority.Approve(ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	if err := scratch.Close(); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "control.db")
	database := rawDatabase(t, path)
	for _, name := range migrationNames[:2] {
		contents, err := migrationFiles.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := database.ExecContext(ctx, string(contents)); err != nil {
			t.Fatalf("apply legacy migration %s: %v", name, err)
		}
	}
	if _, err := database.ExecContext(ctx, "PRAGMA application_id = "+strconv.Itoa(applicationID)); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, "PRAGMA user_version = 2"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO protocol_identities (identity_kind, identity_id, binding_digest)
		VALUES ('authority_approval', ?, ?)`,
		historical.Facts().ReceiptID,
		"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}

	control := openTestStore(t, path)
	t.Cleanup(func() { _ = control.Close() })
	authority, _, _ = authorityFixture(t, control, plan, 1, nil, false, nil)
	approved, err := authority.Approve(ctx, plan)
	if err != nil {
		t.Fatalf("legacy structural identity preempted authenticated authority: %v", err)
	}
	if approved.Facts() != historical.Facts() {
		t.Fatal("authenticated approval identity drifted across equivalent ledgers")
	}
	assertCount(t, control, "authority_approvals", 1)
}

func TestSignedRevocationAdvancesHeadAndNewerSignedSourceMayReactivate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	control := openTestStore(t, filepath.Join(t.TempDir(), "control.db"))
	t.Cleanup(func() { _ = control.Close() })
	plan := exampleExactPlan(t)
	v1, _, key := authorityFixture(t, control, plan, 1, nil, false, nil)
	if _, err := v1.Approve(ctx, plan); err != nil {
		t.Fatal(err)
	}
	revoked, _, _ := authorityFixture(t, control, plan, 2, key, false, func(source map[string]any) {
		source["status"] = "revoked"
		source["maximum_grants"] = []any{}
	})
	if _, err := revoked.Approve(ctx, plan); err == nil || !strings.Contains(err.Error(), "revoked") {
		t.Fatalf("revoked source result = %v", err)
	}
	assertAuthorityClosureCounts(t, control, 2, 2, 1, 3, 5)

	reactivated, _, _ := authorityFixture(t, control, plan, 3, key, false, nil)
	if _, err := reactivated.Approve(ctx, plan); err != nil {
		t.Fatalf("newer signed active source after revocation: %v", err)
	}
	revised := reviseExamplePlan(t, plan, "A new plan cannot roll authority back.")
	rollback, _, _ := authorityFixture(t, control, revised, 1, key, false, nil)
	if _, err := rollback.Approve(ctx, revised); err == nil || !strings.Contains(err.Error(), "rollback") {
		t.Fatalf("old source new-plan result = %v", err)
	}
	assertAuthorityClosureCounts(t, control, 3, 3, 2, 4, 8)
}

func TestCanonicalSourceHeadAcceptsDistinctRawFormattingPerApproval(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	control := openTestStore(t, filepath.Join(t.TempDir(), "control.db"))
	t.Cleanup(func() { _ = control.Close() })
	plan := exampleExactPlan(t)
	first, _, key := authorityFixture(t, control, plan, 1, nil, false, nil)
	if _, err := first.Approve(ctx, plan); err != nil {
		t.Fatal(err)
	}
	revised := reviseExamplePlan(t, plan, "The same signed source may be formatted differently.")
	second, _, _ := authorityFixture(t, control, revised, 1, key, true, nil)
	if _, err := second.Approve(ctx, revised); err != nil {
		t.Fatalf("canonical-equivalent source formatting: %v", err)
	}
	assertAuthorityClosureCounts(t, control, 1, 2, 2, 3, 6)
}

func TestCanonicalSourceHeadAcceptsDistinctRawFormattingForSameApproval(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	control := openTestStore(t, filepath.Join(t.TempDir(), "control.db"))
	t.Cleanup(func() { _ = control.Close() })
	plan := exampleExactPlan(t)
	first, _, key := authorityFixture(t, control, plan, 1, nil, false, nil)
	original, err := first.Approve(ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	formatted, _, _ := authorityFixture(t, control, plan, 1, key, true, nil)
	repeated, err := formatted.Approve(ctx, plan)
	if err != nil {
		t.Fatalf("canonical-equivalent same-plan retry: %v", err)
	}
	if repeated.Facts() != original.Facts() {
		t.Fatal("format-only retry changed the semantic approval identity")
	}
	assertAuthorityClosureCounts(t, control, 1, 2, 1, 2, 4)
}

func TestFirstObservedRevocationSetsHighWaterAndRollsBackOlderSource(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	control := openTestStore(t, filepath.Join(t.TempDir(), "control.db"))
	t.Cleanup(func() { _ = control.Close() })
	plan := exampleExactPlan(t)
	revoked, _, key := authorityFixture(t, control, plan, 2, nil, false, func(source map[string]any) {
		source["status"] = "revoked"
		source["maximum_grants"] = []any{}
	})
	if _, err := revoked.Approve(ctx, plan); err == nil || !strings.Contains(err.Error(), "revoked") {
		t.Fatalf("first observed signed revocation = %v", err)
	}
	stale, _, _ := authorityFixture(t, control, plan, 1, key, false, nil)
	if _, err := stale.Approve(ctx, plan); err == nil || !strings.Contains(err.Error(), "rollback") {
		t.Fatalf("stale source after initial revocation = %v", err)
	}
	assertAuthorityClosureCounts(t, control, 1, 1, 0, 2, 2)
}

func TestAuthorityLedgerRejectsZeroCapabilities(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	control := openTestStore(t, filepath.Join(t.TempDir(), "control.db"))
	t.Cleanup(func() { _ = control.Close() })
	if err := control.PutAuthoritySource(ctx, policy.PreparedSource{}); err == nil {
		t.Fatal("zero prepared source was stored")
	}
	if err := control.PutAuthorityApproval(ctx, policy.PreparedApproval{}); err == nil {
		t.Fatal("zero prepared approval was stored")
	}
	assertAuthorityClosureCounts(t, control, 0, 0, 0, 0, 0)
}

func TestAuthorityLedgerTablesAreImmutable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	control := openTestStore(t, filepath.Join(t.TempDir(), "control.db"))
	t.Cleanup(func() { _ = control.Close() })
	plan := exampleExactPlan(t)
	authority, _, _ := authorityFixture(t, control, plan, 1, nil, false, nil)
	if _, err := authority.Approve(ctx, plan); err != nil {
		t.Fatal(err)
	}
	if _, err := control.db.ExecContext(ctx, "UPDATE authority_approvals SET authorizer_ref = 'forged'"); err == nil || !strings.Contains(err.Error(), "authority approvals are immutable") {
		t.Fatalf("authority approval immutability error = %v", err)
	}
	if _, err := control.db.ExecContext(ctx, "DELETE FROM authority_source_snapshots"); err == nil || !strings.Contains(err.Error(), "authority source snapshots are immutable") {
		t.Fatalf("authority source snapshot immutability error = %v", err)
	}
	if _, err := control.db.ExecContext(ctx, "DELETE FROM authority_source_authentications"); err == nil || !strings.Contains(err.Error(), "authority source authentications are immutable") {
		t.Fatalf("authority source authentication immutability error = %v", err)
	}
}

func authorityFixture(
	t *testing.T,
	ledger policy.ApprovalLedger,
	plan protocol.ExactPlan,
	version int64,
	privateKey ed25519.PrivateKey,
	prettySource bool,
	mutate func(map[string]any),
) (*policy.Authority, policy.TrustRoot, ed25519.PrivateKey) {
	t.Helper()
	if privateKey == nil {
		seed := sha256.Sum256([]byte("sworn store authority fixture"))
		privateKey = ed25519.NewKeyFromSeed(seed[:])
	}
	root, err := policy.NewTrustRoot(
		plan.Authority().SourceRef,
		"identity:example-authorizer",
		privateKey.Public().(ed25519.PublicKey),
	)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := protocol.SnapshotFS()
	if err != nil {
		t.Fatal(err)
	}
	sourceBytes, err := fs.ReadFile(snapshot, "examples/authority-source.json")
	if err != nil {
		t.Fatal(err)
	}
	var source map[string]any
	if err := json.Unmarshal(sourceBytes, &source); err != nil {
		t.Fatal(err)
	}
	source["version"] = version
	source["valid_from"] = "2026-07-19T00:00:00Z"
	source["valid_until"] = "9999-12-31T23:59:59Z"
	if mutate != nil {
		mutate(source)
	}
	if prettySource {
		sourceBytes, err = json.MarshalIndent(source, "", "  ")
		if err == nil {
			sourceBytes = append(sourceBytes, '\n')
		}
	} else {
		sourceBytes, err = protocol.EncodeCanonical(source)
	}
	if err != nil {
		t.Fatal(err)
	}
	sourceCanonical, err := protocol.CanonicalizeJSON(sourceBytes)
	if err != nil {
		t.Fatal(err)
	}
	proof := map[string]any{
		"schema_version":   policy.AuthorityProofSchemaVersion,
		"source_ref":       plan.Authority().SourceRef,
		"source_digest":    protocol.CanonicalDigest(sourceCanonical),
		"source_version":   version,
		"plan_digest":      plan.Record().Digest,
		"authority_digest": plan.Authority().Digest,
		"key_id":           root.KeyID(),
		"approved_at":      "2026-07-19T00:00:30Z",
	}
	unsigned, err := protocol.EncodeCanonical(proof)
	if err != nil {
		t.Fatal(err)
	}
	message := append([]byte(testProofDomain), unsigned...)
	proof["signature"] = base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, message))
	proofBytes, err := protocol.EncodeCanonical(proof)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := policy.NewAuthority(
		[]policy.TrustRoot{root},
		authorityResolver{source: sourceBytes, proof: proofBytes},
		ledger,
	)
	if err != nil {
		t.Fatal(err)
	}
	return authority, root, privateKey
}

func reviseExamplePlan(t *testing.T, plan protocol.ExactPlan, outcome string) protocol.ExactPlan {
	t.Helper()
	current := `"outcome":"Expose a health endpoint that reports the assembled service as ready."`
	revisedBytes := strings.Replace(
		string(plan.Record().CanonicalJSON), current, `"outcome":`+string(mustJSONString(t, outcome)), 1,
	)
	if revisedBytes == string(plan.Record().CanonicalJSON) {
		t.Fatal("example plan outcome was not revised")
	}
	revised, err := protocol.ParseDeliveryPlan([]byte(revisedBytes))
	if err != nil {
		t.Fatal(err)
	}
	return revised
}

func planWithGrantActions(t *testing.T, plan protocol.ExactPlan, actions ...string) protocol.ExactPlan {
	t.Helper()
	var document map[string]any
	if err := json.Unmarshal(plan.Record().CanonicalJSON, &document); err != nil {
		t.Fatal(err)
	}
	authority, ok := document["authority"].(map[string]any)
	if !ok {
		t.Fatal("plan authority fixture is not an object")
	}
	authority["grants"] = grantsWithActions(t, authority["grants"], actions...)
	canonical, err := protocol.EncodeCanonical(document)
	if err != nil {
		t.Fatal(err)
	}
	revised, err := protocol.ParseDeliveryPlan(canonical)
	if err != nil {
		t.Fatal(err)
	}
	return revised
}

func grantsWithActions(t *testing.T, value any, actions ...string) []any {
	t.Helper()
	allowed := make(map[string]struct{}, len(actions))
	for _, action := range actions {
		allowed[action] = struct{}{}
	}
	items, ok := value.([]any)
	if !ok {
		t.Fatal("grant fixture is not an array")
	}
	selected := make([]any, 0, len(actions))
	for _, item := range items {
		grant, ok := item.(map[string]any)
		if !ok {
			t.Fatal("grant fixture item is not an object")
		}
		action, _ := grant["action"].(string)
		if _, include := allowed[action]; include {
			selected = append(selected, item)
			delete(allowed, action)
		}
	}
	if len(allowed) != 0 {
		t.Fatalf("grant fixture lacks requested actions: %v", allowed)
	}
	return selected
}

func mustJSONString(t *testing.T, value string) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func assertAuthorityClosureCounts(
	t *testing.T,
	control *Store,
	snapshots, authentications, approvals, records, artifacts int,
) {
	t.Helper()
	assertCount(t, control, "authority_source_snapshots", snapshots)
	assertCount(t, control, "authority_source_authentications", authentications)
	assertCount(t, control, "authority_approvals", approvals)
	assertCount(t, control, "records", records)
	assertCount(t, control, "artifacts", artifacts)
}
