package runtime

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/journal"
)

// TestLiveSealedProposalHookSurvivesParentOperationalFailure pins A3 on the
// live writer: Service.sealedProposalHook persists exact plan bytes as the
// planner.sealed_plan child, and those bytes remain readable after the
// parent dispatch completes operational_failed.
func TestLiveSealedProposalHookSurvivesParentOperationalFailure(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 8, 23, 4, 5, 6, 0, time.UTC)
	store, err := journal.Open(ctx, filepath.Join(t.TempDir(), "sealed.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	runID := "run-sealed-1"
	if err := store.RegisterRun(ctx, journal.Run{
		ID: runID, ManifestDigest: sha256Digest([]byte("manifest")),
		Repository: "/repository", Release: "release-1",
		TargetRef: "refs/heads/main", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	owner, err := store.AcquireOwner(ctx, runID, now, time.Minute, false)
	if err != nil {
		t.Fatal(err)
	}
	parentID := "attempt/sealedparent/e1/t1"
	if err := store.RecordCommandEffect(ctx, journal.Command{
		RunID: runID, ReplayKey: parentID, Kind: "driver.dispatch",
		Payload: []byte(`{"dispatch":true}`), CreatedAt: now,
	}, journal.Effect{
		RunID: runID, ID: parentID, ReplayKey: parentID,
		Kind: "driver.dispatch", BeforeDigest: sha256Digest([]byte("before")),
		ExpectedDigest: sha256Digest([]byte("after")), UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	parentClaim, err := store.ClaimOwned(ctx, owner, parentID, now, effectLease)
	if err != nil {
		t.Fatal(err)
	}
	service := &Service{journal: store, now: func() time.Time { return now }}
	plan := []byte("```baton-plan-v2\n{\"release\":\"hook\"}\n```\n# Plan\n")
	hook := service.sealedProposalHook(owner, parentID)
	if hook == nil {
		t.Fatal("live sealed proposal hook is nil")
	}
	if err := hook(ctx, plan, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteOwned(ctx, owner, journal.Completion{
		RunID: runID, EffectID: parentID, Token: parentClaim.Token,
		State: journal.OperationalFailed, ErrorCode: "transport_error",
		EventKind: "dispatch_operational_failure", At: now.Add(time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	got, err := store.LoadSealedProposal(ctx, runID, parentID)
	if err != nil || !bytes.Equal(got, plan) {
		t.Fatalf("live hook bytes after parent fail = %q, %v", got, err)
	}
	snapshot, err := store.Snapshot(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	persisted := 0
	for _, event := range snapshot.Events {
		if event.Kind == "planner.sealed_plan_persisted" {
			persisted++
		}
	}
	if persisted != 1 {
		t.Fatalf("planner.sealed_plan_persisted events = %d, want 1", persisted)
	}
	parent, err := store.Effect(ctx, runID, parentID)
	if err != nil || parent.State != journal.OperationalFailed {
		t.Fatalf("parent after fail = %#v, %v", parent, err)
	}
}

// TestLiveSealedProposalHookPersistsAndRereadsCarriedContracts pins A1 on the
// live writer: Service.sealedProposalHook persists carried contract bytes as
// a separate planner.sealed_contracts child keyed off the same parent
// dispatch, preserving the planner.sealed_plan child byte-for-byte, and
// those contract bytes remain readable after the parent completes.
func TestLiveSealedProposalHookPersistsAndRereadsCarriedContracts(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 8, 25, 5, 6, 7, 0, time.UTC)
	store, err := journal.Open(ctx, filepath.Join(t.TempDir(), "sealed.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	runID := "run-sealed-contracts-1"
	if err := store.RegisterRun(ctx, journal.Run{
		ID: runID, ManifestDigest: sha256Digest([]byte("manifest")),
		Repository: "/repository", Release: "release-1",
		TargetRef: "refs/heads/main", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	owner, err := store.AcquireOwner(ctx, runID, now, time.Minute, false)
	if err != nil {
		t.Fatal(err)
	}
	parentID := "attempt/sealedcontracts/e1/t1"
	if err := store.RecordCommandEffect(ctx, journal.Command{
		RunID: runID, ReplayKey: parentID, Kind: "driver.dispatch",
		Payload: []byte(`{"dispatch":true}`), CreatedAt: now,
	}, journal.Effect{
		RunID: runID, ID: parentID, ReplayKey: parentID,
		Kind: "driver.dispatch", BeforeDigest: sha256Digest([]byte("before")),
		ExpectedDigest: sha256Digest([]byte("after")), UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	parentClaim, err := store.ClaimOwned(ctx, owner, parentID, now, effectLease)
	if err != nil {
		t.Fatal(err)
	}
	service := &Service{journal: store, now: func() time.Time { return now }}
	plan := []byte("```baton-plan-v2\n{\"release\":\"hook\"}\n```\n# Plan\n")
	contracts := map[string][]byte{"contracts/S1.json": []byte(`{"outcome":"Deliver S1."}`)}
	hook := service.sealedProposalHook(owner, parentID)
	if hook == nil {
		t.Fatal("live sealed proposal hook is nil")
	}
	if err := hook(ctx, plan, contracts); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteOwned(ctx, owner, journal.Completion{
		RunID: runID, EffectID: parentID, Token: parentClaim.Token,
		State: journal.OperationalFailed, ErrorCode: "transport_error",
		EventKind: "dispatch_operational_failure", At: now.Add(time.Second),
	}); err != nil {
		t.Fatal(err)
	}

	gotPlan, err := store.LoadSealedProposal(ctx, runID, parentID)
	if err != nil || !bytes.Equal(gotPlan, plan) {
		t.Fatalf("sealed plan bytes after parent fail = %q, %v", gotPlan, err)
	}
	contractsEffect, err := store.Effect(ctx, runID, parentID+"/sealed-contracts")
	if err != nil || contractsEffect.Kind != "planner.sealed_contracts" ||
		contractsEffect.State != journal.Succeeded {
		t.Fatalf("sealed contracts effect = %#v, err = %v", contractsEffect, err)
	}
	if !bytes.Equal(contractsEffect.Result, mustJSON(contracts)) {
		t.Fatalf("sealed contracts bytes = %q, want %q", contractsEffect.Result, mustJSON(contracts))
	}
	snapshot, err := store.Snapshot(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	persistedPlan, persistedContracts := 0, 0
	for _, event := range snapshot.Events {
		switch event.Kind {
		case "planner.sealed_plan_persisted":
			persistedPlan++
		case "planner.sealed_contracts_persisted":
			persistedContracts++
		}
	}
	if persistedPlan != 1 || persistedContracts != 1 {
		t.Fatalf(
			"persisted events plan=%d contracts=%d, want 1 and 1",
			persistedPlan, persistedContracts,
		)
	}
}
