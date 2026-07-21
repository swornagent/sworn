//go:build linux

package control_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/config"
	controlpkg "github.com/swornagent/sworn/internal/control"
	"github.com/swornagent/sworn/internal/effects"
	"github.com/swornagent/sworn/internal/engine"
	"github.com/swornagent/sworn/internal/executor"
	"github.com/swornagent/sworn/internal/policy"
	"github.com/swornagent/sworn/internal/protocol"
	"github.com/swornagent/sworn/internal/store"
)

const bundleIntegrationAuthorizerRef = "identity:bundle-integration-authorizer"

type bundleIntegrationRunner struct {
	*integrationBuilderRunner
	entries int
}

func (runner *bundleIntegrationRunner) RunWritable(
	ctx context.Context,
	invocation executor.Invocation,
) (executor.RawCompletion, error) {
	runner.entries++
	return runner.integrationBuilderRunner.RunWritable(ctx, invocation)
}

type integrationAuthorityBundle struct {
	SchemaVersion string `json:"schema_version"`
	Source        string `json:"source"`
	Proof         string `json:"proof"`
}

type integrationAuthorityVersion struct {
	source []byte
	proof  []byte
}

func TestBuilderControllerRefreshesProductionAuthorityBundleBeforeClaim(t *testing.T) {
	ctx := context.Background()
	repository := newIntegrationRepository(t)
	workspaceRoot := t.TempDir()
	if err := os.Chmod(workspaceRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	runner := &bundleIntegrationRunner{integrationBuilderRunner: &integrationBuilderRunner{
		configurationDigest: protocol.RawDigest([]byte("authority-bundle-integration-builder-v1")),
		limits:              executor.DefaultLimits(),
		exportRoot:          t.TempDir(),
	}}
	worker := effects.BuilderWorker{
		Runner: runner, Repository: repository,
		WorkspaceRoot: workspaceRoot, Agent: "authority-bundle-integration-builder@1",
		Argv: []string{"/usr/bin/integration-builder"}, Timeout: time.Minute,
	}
	builderDispatchDigest, err := worker.DispatchDigest()
	if err != nil {
		t.Fatal(err)
	}
	controlRoot := t.TempDir()
	if err := os.Chmod(controlRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	journal, err := store.OpenConfigured(ctx, filepath.Join(controlRoot, "control.db"), store.ControlConfiguration{
		BuilderDispatchDigest: builderDispatchDigest,
		Repository:            repository,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = journal.Close() })
	worker.Control = journal

	clock := time.Now().UTC().Add(-5 * time.Minute).Truncate(time.Second)
	plan := newIntegrationPlan(t, journal, clock)
	seed := sha256.Sum256([]byte("production authority bundle controller integration"))
	privateKey := ed25519.NewKeyFromSeed(seed[:])
	publicKey := privateKey.Public().(ed25519.PublicKey)
	v1 := newIntegrationAuthorityVersion(t, plan, privateKey, 1, "active", clock.Add(time.Minute))
	v2 := newIntegrationAuthorityVersion(t, plan, privateKey, 2, "revoked", clock.Add(2*time.Minute))
	bundleDirectory := t.TempDir()
	replaceIntegrationAuthorityBundle(t, bundleDirectory, plan.Record().Digest, v1)

	configured, err := config.OpenAuthority([]config.AuthoritySource{{
		SourceRef: plan.Authority().SourceRef, AuthorizerRef: bundleIntegrationAuthorizerRef,
		PublicKey: publicKey, BundleDirectory: bundleDirectory,
	}}, journal)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = configured.Close() })
	approval, err := configured.Service().Approve(ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	workID := plan.WorkIDs()[0]
	applyIntegrationCommand(t, journal, integrationCommand(
		t, "cmd-bundle-create", "run-bundle", engine.CommandCreate, engine.NoRevision,
		engine.CreatePayload{
			DeliveryID: plan.DeliveryID(), PlanDigest: plan.Record().Digest,
			Repository: plan.Target().Repository, TargetRef: plan.Target().Ref, Work: plan.WorkIDs(),
		},
	))
	applyIntegrationCommand(t, journal, integrationCommand(
		t, "cmd-bundle-activate", "run-bundle", engine.CommandActivate, 0,
		engine.ActivatePayload{AuthorityReceiptDigest: approval.Facts().ReceiptDigest},
	))
	builderService, err := controlpkg.NewBuilderService(journal, worker)
	if err != nil {
		t.Fatal(err)
	}
	controller, recovery, err := controlpkg.StartBuilderController(
		ctx, "controller-bundle-v1", journal, configured.Service(), builderService,
	)
	if err != nil || recovery != (controlpkg.RecoveryReport{}) {
		t.Fatalf("start v1 controller = %#v, %v", recovery, err)
	}
	malformedV1 := integrationAuthorityVersion{source: v1.source, proof: []byte("{}")}
	replaceIntegrationAuthorityBundle(t, bundleDirectory, plan.Record().Digest, malformedV1)
	if result, err := controller.DispatchBuild(ctx, "run-bundle", workID, "cmd-bundle-build"); err == nil {
		t.Fatalf("dispatch under malformed replacement = %+v, want authority error", result)
	}
	if runner.entries != 0 {
		t.Fatalf("malformed dispatch reached builder entry %d times", runner.entries)
	}
	replaceIntegrationAuthorityBundle(t, bundleDirectory, plan.Record().Digest, v1)
	if result, err := controller.DispatchBuild(ctx, "run-bundle", workID, "cmd-bundle-build"); err != nil ||
		result.Outcome != store.OutcomeApplied || len(result.EffectIDs) != 1 {
		t.Fatalf("dispatch under active v1 = %+v, %v", result, err)
	}
	if runner.entries != 0 {
		t.Fatalf("builder entries after dispatch = %d, want 0", runner.entries)
	}

	replaceIntegrationAuthorityBundle(t, bundleDirectory, plan.Record().Digest, v2)
	if err := controller.ExecutePendingBuild(ctx, "run-bundle", workID); err == nil ||
		!strings.Contains(err.Error(), "authority source is revoked") {
		t.Fatalf("pending claim under revoked v2 error = %v", err)
	}
	if runner.entries != 0 {
		t.Fatalf("revoked v2 reached builder entry %d times", runner.entries)
	}
	mediaType, persistedV2, err := journal.Artifact(ctx, protocol.RawDigest(v2.source))
	if err != nil || mediaType == "" || !bytes.Equal(persistedV2, v2.source) {
		t.Fatalf("durable revoked v2 source = media %q, equal %t, %v", mediaType, bytes.Equal(persistedV2, v2.source), err)
	}
	if err := controller.Close(); err != nil {
		t.Fatal(err)
	}
	if err := configured.Close(); err != nil {
		t.Fatal(err)
	}
	if err := configured.Close(); err != nil {
		t.Fatalf("idempotent authority close: %v", err)
	}

	replaceIntegrationAuthorityBundle(t, bundleDirectory, plan.Record().Digest, v1)
	reopened, err := config.OpenAuthority([]config.AuthoritySource{{
		SourceRef: plan.Authority().SourceRef, AuthorizerRef: bundleIntegrationAuthorizerRef,
		PublicKey: publicKey, BundleDirectory: bundleDirectory,
	}}, journal)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	successor, recovery, err := controlpkg.StartBuilderController(
		ctx, "controller-bundle-rollback", journal, reopened.Service(), builderService,
	)
	if err != nil || recovery != (controlpkg.RecoveryReport{}) {
		t.Fatalf("start rollback controller = %#v, %v", recovery, err)
	}
	t.Cleanup(func() { _ = successor.Close() })
	if err := successor.ExecutePendingBuild(ctx, "run-bundle", workID); err == nil ||
		!strings.Contains(err.Error(), "authority source version rollback") {
		t.Fatalf("restored v1 rollback error = %v", err)
	}
	if runner.entries != 0 {
		t.Fatalf("restored v1 reached builder entry %d times", runner.entries)
	}
}

func newIntegrationAuthorityVersion(
	t *testing.T,
	plan protocol.ExactPlan,
	privateKey ed25519.PrivateKey,
	version int64,
	status string,
	approvedAt time.Time,
) integrationAuthorityVersion {
	t.Helper()
	root, err := policy.NewTrustRoot(
		plan.Authority().SourceRef,
		bundleIntegrationAuthorizerRef,
		privateKey.Public().(ed25519.PublicKey),
	)
	if err != nil {
		t.Fatal(err)
	}
	grants := make([]json.RawMessage, 0, len(plan.Authority().Grants))
	for _, grant := range plan.Authority().Grants {
		grants = append(grants, json.RawMessage(grant.CanonicalJSON()))
	}
	source, err := protocol.EncodeCanonical(integrationAuthoritySource{
		Version: version, SourceID: "bundle-integration-source", Status: status,
		Repository: plan.Target().Repository, TargetRef: plan.Target().Ref,
		MaximumGrants: grants, AuthorizerRef: bundleIntegrationAuthorizerRef,
		ValidFrom:  approvedAt.Add(-time.Hour).Format(time.RFC3339Nano),
		ValidUntil: approvedAt.Add(24 * time.Hour).Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatal(err)
	}
	proof := integrationAuthorityProof{
		SchemaVersion:   policy.AuthorityProofSchemaVersion,
		SourceRef:       plan.Authority().SourceRef,
		SourceDigest:    protocol.CanonicalDigest(source),
		SourceVersion:   version,
		PlanDigest:      plan.Record().Digest,
		AuthorityDigest: plan.Authority().Digest,
		KeyID:           root.KeyID(),
		ApprovedAt:      approvedAt.Format(time.RFC3339Nano),
	}
	unsigned, err := protocol.EncodeCanonical(integrationUnsignedAuthorityProof{
		SchemaVersion: proof.SchemaVersion, SourceRef: proof.SourceRef,
		SourceDigest: proof.SourceDigest, SourceVersion: proof.SourceVersion,
		PlanDigest: proof.PlanDigest, AuthorityDigest: proof.AuthorityDigest,
		KeyID: proof.KeyID, ApprovedAt: proof.ApprovedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	message := append([]byte("sworn/authority-proof/v1\x00"), unsigned...)
	proof.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, message))
	proofBytes, err := protocol.EncodeCanonical(proof)
	if err != nil {
		t.Fatal(err)
	}
	return integrationAuthorityVersion{source: source, proof: proofBytes}
}

func replaceIntegrationAuthorityBundle(
	t *testing.T,
	directory string,
	planDigest string,
	version integrationAuthorityVersion,
) {
	t.Helper()
	hexDigest, ok := strings.CutPrefix(planDigest, "sha256:")
	if !ok || len(hexDigest) != 64 || strings.ToLower(hexDigest) != hexDigest {
		t.Fatalf("invalid plan digest for bundle filename %q", planDigest)
	}
	bundle, err := protocol.EncodeCanonical(integrationAuthorityBundle{
		SchemaVersion: config.AuthorityBundleSchemaVersion,
		Source:        base64.RawURLEncoding.EncodeToString(version.source),
		Proof:         base64.RawURLEncoding.EncodeToString(version.proof),
	})
	if err != nil {
		t.Fatal(err)
	}
	temporary, err := os.CreateTemp(directory, ".authority-bundle-")
	if err != nil {
		t.Fatal(err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath) //nolint:errcheck
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		t.Fatal(err)
	}
	if _, err := temporary.Write(bundle); err != nil {
		_ = temporary.Close()
		t.Fatal(err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		t.Fatal(err)
	}
	if err := temporary.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(temporaryPath, filepath.Join(directory, hexDigest+".json")); err != nil {
		t.Fatal(err)
	}
}
