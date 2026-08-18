package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/journal"
)

func TestRefusalPathsSealingWithReservedRootRecordedInJournalAndCarriedOnRetry(t *testing.T) {
	dispatcher := fixtureDriver(func(
		_ context.Context,
		invocation driver.Invocation,
	) (driver.Observation, error) {
		return productionImplementationObservation(t, invocation), nil
	})
	fixture := newProductionImplementationRecoveryFixture(t, dispatcher)

	// Write reserved-root file in workspace
	reservedFile := filepath.Join(
		fixture.workspace.Path(),
		filepath.FromSlash(gitx.DefaultRecordsRoot),
		fixture.manifest.value.Release,
		"forged_plan.md",
	)
	if err := os.MkdirAll(filepath.Dir(reservedFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(reservedFile, []byte("forged\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Run production implementation dispatch
	_, _, dispatchErr := fixture.service.runProductionImplementationDispatch(
		fixture.ctx,
		fixture.engine,
		fixture.owner,
		fixture.workspace,
		fixture.cycle,
		fixture.coordinates,
	)
	if dispatchErr == nil {
		t.Fatal("expected dispatch to fail on reserved root change, got nil")
	}

	// Complete outer failure as scheduler does
	if err := fixture.service.completeImplementationFailure(
		fixture.ctx,
		fixture.owner,
		fixture.outer.ID,
		fixture.outer.CurrentClaim,
		stableErrorCode(dispatchErr),
		extractRefusalResult(dispatchErr),
	); err != nil {
		t.Fatal(err)
	}

	// 1. Verify driver.dispatch effect in journal
	dispatchEffect, err := fixture.store.Effect(
		fixture.ctx,
		fixture.manifest.value.RunID,
		fixture.cycle.DispatchEffect,
	)
	if err != nil {
		t.Fatal(err)
	}
	if dispatchEffect.State != journal.OperationalFailed {
		t.Fatalf("dispatch effect state = %s, want operational_failed", dispatchEffect.State)
	}
	if dispatchEffect.ErrorCode != "AUTHORITY_PATH_CHANGED" {
		t.Fatalf("dispatch error code = %s, want AUTHORITY_PATH_CHANGED", dispatchEffect.ErrorCode)
	}
	var dispatchRefusal productionRefusalBinding
	if err := json.Unmarshal(dispatchEffect.Result, &dispatchRefusal); err != nil {
		t.Fatalf("cannot decode dispatch effect refusal: %v (raw: %s)", err, string(dispatchEffect.Result))
	}
	expectedPath := filepath.ToSlash(filepath.Join(gitx.DefaultRecordsRoot, fixture.manifest.value.Release, "forged_plan.md"))
	if dispatchRefusal.Code != "AUTHORITY_PATH_CHANGED" {
		t.Fatalf("refusal code = %s, want AUTHORITY_PATH_CHANGED", dispatchRefusal.Code)
	}
	if !reflect.DeepEqual(dispatchRefusal.Paths, []string{expectedPath}) {
		t.Fatalf("refusal paths = %v, want [%s]", dispatchRefusal.Paths, expectedPath)
	}
	if dispatchRefusal.TotalPaths != 1 {
		t.Fatalf("refusal total paths = %d, want 1", dispatchRefusal.TotalPaths)
	}

	// 2. Verify git.seal effect in journal
	sealEffect, err := fixture.store.Effect(
		fixture.ctx,
		fixture.manifest.value.RunID,
		fixture.outer.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if sealEffect.State != journal.OperationalFailed {
		t.Fatalf("seal effect state = %s, want operational_failed", sealEffect.State)
	}
	if sealEffect.ErrorCode != "AUTHORITY_PATH_CHANGED" {
		t.Fatalf("seal error code = %s, want AUTHORITY_PATH_CHANGED", sealEffect.ErrorCode)
	}
	var sealRefusal productionRefusalBinding
	if err := json.Unmarshal(sealEffect.Result, &sealRefusal); err != nil {
		t.Fatalf("cannot decode seal effect refusal: %v", err)
	}
	if sealRefusal.Code != "AUTHORITY_PATH_CHANGED" || sealRefusal.TotalPaths != 1 {
		t.Fatalf("unexpected seal refusal: %#v", sealRefusal)
	}

	// 3. Verify Try 2 retry context carries the prior refusal
	retryCoords := fixture.coordinates
	retryCoords.Try = 2

	retryWorkContext, contextBytes, err := captureProductionWorkContext(
		fixture.ctx,
		fixture.engine,
		retryCoords,
		fixture.cycle.Before,
		driver.ReadWrite,
	)
	if err != nil {
		t.Fatalf("captureProductionWorkContext failed: %v", err)
	}
	if retryWorkContext.Refusal == nil {
		t.Fatal("expected retry work context to have non-nil Refusal")
	}
	if retryWorkContext.Refusal.Code != "AUTHORITY_PATH_CHANGED" {
		t.Fatalf("refusal code = %s, want AUTHORITY_PATH_CHANGED", retryWorkContext.Refusal.Code)
	}
	if !reflect.DeepEqual(retryWorkContext.Refusal.Paths, []string{expectedPath}) {
		t.Fatalf("refusal paths = %v, want [%s]", retryWorkContext.Refusal.Paths, expectedPath)
	}
	if retryWorkContext.Refusal.TotalPaths != 1 {
		t.Fatalf("refusal total paths = %d, want 1", retryWorkContext.Refusal.TotalPaths)
	}

	var jsonMap map[string]any
	if err := json.Unmarshal(contextBytes, &jsonMap); err != nil {
		t.Fatal(err)
	}
	refusalMap, ok := jsonMap["refusal"].(map[string]any)
	if !ok {
		t.Fatalf("work-context.json missing refusal object: %s", string(contextBytes))
	}
	if refusalMap["code"] != "AUTHORITY_PATH_CHANGED" {
		t.Fatalf("work-context.json refusal code = %v", refusalMap["code"])
	}
}

func TestRefusalPathsScopeViolationRecordedInJournalAndCarriedOnRetry(t *testing.T) {
	dispatcher := fixtureDriver(func(
		_ context.Context,
		invocation driver.Invocation,
	) (driver.Observation, error) {
		return productionImplementationObservation(t, invocation), nil
	})
	fixture := newProductionImplementationRecoveryFixture(t, dispatcher)

	// S1 touchpoints in test plan are pkg/...
	// Write 3 files outside scope
	outPaths := []string{
		"outside_scope_a.txt",
		"outside_scope_b.txt",
		"outside_scope_c.txt",
	}
	for _, p := range outPaths {
		full := filepath.Join(fixture.workspace.Path(), p)
		if err := os.WriteFile(full, []byte("outside\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Run production implementation dispatch
	_, _, dispatchErr := fixture.service.runProductionImplementationDispatch(
		fixture.ctx,
		fixture.engine,
		fixture.owner,
		fixture.workspace,
		fixture.cycle,
		fixture.coordinates,
	)
	if dispatchErr == nil {
		t.Fatal("expected dispatch to fail on scope violation, got nil")
	}

	// Complete outer failure as scheduler does
	if err := fixture.service.completeImplementationFailure(
		fixture.ctx,
		fixture.owner,
		fixture.outer.ID,
		fixture.outer.CurrentClaim,
		stableErrorCode(dispatchErr),
		extractRefusalResult(dispatchErr),
	); err != nil {
		t.Fatal(err)
	}

	// 1. Verify driver.dispatch effect in journal
	dispatchEffect, err := fixture.store.Effect(
		fixture.ctx,
		fixture.manifest.value.RunID,
		fixture.cycle.DispatchEffect,
	)
	if err != nil {
		t.Fatal(err)
	}
	if dispatchEffect.State != journal.OperationalFailed {
		t.Fatalf("dispatch effect state = %s, want operational_failed", dispatchEffect.State)
	}
	if dispatchEffect.ErrorCode != "CANDIDATE_SCOPE_FAILED" {
		t.Fatalf("dispatch error code = %s, want CANDIDATE_SCOPE_FAILED", dispatchEffect.ErrorCode)
	}
	var dispatchRefusal productionRefusalBinding
	if err := json.Unmarshal(dispatchEffect.Result, &dispatchRefusal); err != nil {
		t.Fatalf("cannot decode dispatch effect refusal: %v (raw: %s)", err, string(dispatchEffect.Result))
	}
	if dispatchRefusal.Code != "SLICE_OUTSIDE_SCOPE" {
		t.Fatalf("refusal code = %s, want SLICE_OUTSIDE_SCOPE", dispatchRefusal.Code)
	}
	if !reflect.DeepEqual(dispatchRefusal.Paths, outPaths) {
		t.Fatalf("refusal paths = %v, want %v", dispatchRefusal.Paths, outPaths)
	}
	if dispatchRefusal.TotalPaths != 3 {
		t.Fatalf("refusal total paths = %d, want 3", dispatchRefusal.TotalPaths)
	}

	// 2. Verify git.seal effect in journal
	sealEffect, err := fixture.store.Effect(
		fixture.ctx,
		fixture.manifest.value.RunID,
		fixture.outer.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if sealEffect.State != journal.OperationalFailed {
		t.Fatalf("seal effect state = %s, want operational_failed", sealEffect.State)
	}
	if sealEffect.ErrorCode != "CANDIDATE_SCOPE_FAILED" {
		t.Fatalf("seal error code = %s, want CANDIDATE_SCOPE_FAILED", sealEffect.ErrorCode)
	}
	var sealRefusal productionRefusalBinding
	if err := json.Unmarshal(sealEffect.Result, &sealRefusal); err != nil {
		t.Fatalf("cannot decode seal effect refusal: %v", err)
	}
	if sealRefusal.Code != "SLICE_OUTSIDE_SCOPE" || sealRefusal.TotalPaths != 3 {
		t.Fatalf("unexpected seal refusal: %#v", sealRefusal)
	}
	if !reflect.DeepEqual(sealRefusal.Paths, outPaths) {
		t.Fatalf("seal refusal paths = %v, want %v", sealRefusal.Paths, outPaths)
	}

	// 3. Verify Try 2 retry context carries the 3 out-of-scope paths
	retryCoords := fixture.coordinates
	retryCoords.Try = 2

	retryWorkContext, contextBytes, err := captureProductionWorkContext(
		fixture.ctx,
		fixture.engine,
		retryCoords,
		fixture.cycle.Before,
		driver.ReadWrite,
	)
	if err != nil {
		t.Fatalf("captureProductionWorkContext failed: %v", err)
	}
	if retryWorkContext.Refusal == nil {
		t.Fatal("expected retry work context to have non-nil Refusal")
	}
	if retryWorkContext.Refusal.Code != "SLICE_OUTSIDE_SCOPE" {
		t.Fatalf("refusal code = %s, want SLICE_OUTSIDE_SCOPE", retryWorkContext.Refusal.Code)
	}
	if !reflect.DeepEqual(retryWorkContext.Refusal.Paths, outPaths) {
		t.Fatalf("refusal paths = %v, want %v", retryWorkContext.Refusal.Paths, outPaths)
	}
	if retryWorkContext.Refusal.TotalPaths != 3 {
		t.Fatalf("refusal total paths = %d, want 3", retryWorkContext.Refusal.TotalPaths)
	}

	var jsonMap map[string]any
	if err := json.Unmarshal(contextBytes, &jsonMap); err != nil {
		t.Fatal(err)
	}
	refusalMap, ok := jsonMap["refusal"].(map[string]any)
	if !ok {
		t.Fatalf("work-context.json missing refusal object: %s", string(contextBytes))
	}
	if refusalMap["code"] != "SLICE_OUTSIDE_SCOPE" {
		t.Fatalf("work-context.json refusal code = %v", refusalMap["code"])
	}
}

func TestValidateProductionWorkContextRefusalValidation(t *testing.T) {
	dispatcher := fixtureDriver(func(
		_ context.Context,
		invocation driver.Invocation,
	) (driver.Observation, error) {
		return productionImplementationObservation(t, invocation), nil
	})
	fixture := newProductionImplementationRecoveryFixture(t, dispatcher)

	validContext, _, err := captureProductionWorkContext(
		fixture.ctx,
		fixture.engine,
		fixture.coordinates,
		fixture.cycle.Before,
		driver.ReadWrite,
	)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Try 1 with Refusal is rejected
	try1WithRefusal := validContext
	try1WithRefusal.Refusal = &productionRefusalBinding{
		Code:       "SLICE_OUTSIDE_SCOPE",
		Paths:      []string{"a.txt"},
		TotalPaths: 1,
	}
	if err := validateProductionWorkContext(fixture.manifest, try1WithRefusal); err == nil {
		t.Fatal("expected validation failure for Try 1 with Refusal")
	}

	// 2. Try 2 with valid Refusal is accepted
	try2WithRefusal := validContext
	try2WithRefusal.Try = 2
	try2WithRefusal.InvocationID = dispatchInvocationID(
		fixture.manifest.value.RunID,
		dispatchCoordinates{
			Slice:          "S1",
			Responsibility: driver.ImplementerImplementation,
			BatonAttempt:   1,
			Epoch:          1,
			Try:            2,
		},
	)
	try2WithRefusal.Refusal = &productionRefusalBinding{
		Code:       "SLICE_OUTSIDE_SCOPE",
		Paths:      []string{"a.txt", "b.txt"},
		TotalPaths: 2,
	}
	if err := validateProductionWorkContext(fixture.manifest, try2WithRefusal); err != nil {
		t.Fatalf("expected valid Try 2 refusal to pass validation, got: %v", err)
	}

	// 3. Try 2 with invalid Code is rejected
	invalidCode := try2WithRefusal
	invalidCode.Refusal = &productionRefusalBinding{
		Code:       "INVALID CODE WITH SPACES",
		Paths:      []string{"a.txt"},
		TotalPaths: 1,
	}
	if err := validateProductionWorkContext(fixture.manifest, invalidCode); err == nil {
		t.Fatal("expected validation failure for invalid refusal code")
	}

	// 4. Try 2 with 0 paths is rejected
	zeroPaths := try2WithRefusal
	zeroPaths.Refusal = &productionRefusalBinding{
		Code:       "SLICE_OUTSIDE_SCOPE",
		Paths:      []string{},
		TotalPaths: 0,
	}
	if err := validateProductionWorkContext(fixture.manifest, zeroPaths); err == nil {
		t.Fatal("expected validation failure for 0 paths")
	}

	// 5. Try 2 with >20 paths is rejected
	tooManyPaths := try2WithRefusal
	twentyOne := make([]string, 21)
	for i := 0; i < 21; i++ {
		twentyOne[i] = string(rune('a' + i))
	}
	tooManyPaths.Refusal = &productionRefusalBinding{
		Code:       "SLICE_OUTSIDE_SCOPE",
		Paths:      twentyOne,
		TotalPaths: 21,
	}
	if err := validateProductionWorkContext(fixture.manifest, tooManyPaths); err == nil {
		t.Fatal("expected validation failure for >20 paths")
	}

	// 6. Try 2 with TotalPaths < len(Paths) is rejected
	badTotal := try2WithRefusal
	badTotal.Refusal = &productionRefusalBinding{
		Code:       "SLICE_OUTSIDE_SCOPE",
		Paths:      []string{"a.txt", "b.txt"},
		TotalPaths: 1,
	}
	if err := validateProductionWorkContext(fixture.manifest, badTotal); err == nil {
		t.Fatal("expected validation failure for TotalPaths < len(Paths)")
	}

	// 7. Try 2 with unsorted paths is rejected
	unsorted := try2WithRefusal
	unsorted.Refusal = &productionRefusalBinding{
		Code:       "SLICE_OUTSIDE_SCOPE",
		Paths:      []string{"b.txt", "a.txt"},
		TotalPaths: 2,
	}
	if err := validateProductionWorkContext(fixture.manifest, unsorted); err == nil {
		t.Fatal("expected validation failure for unsorted paths")
	}

	// 8. productionWorkContextV1 clears Refusal and rejects V1 with Refusal
	v1, err := productionWorkContextV1(fixture.manifest, try2WithRefusal)
	if err != nil {
		t.Fatalf("productionWorkContextV1 failed: %v", err)
	}
	if v1.Refusal != nil {
		t.Fatalf("expected V1 Refusal to be nil, got: %#v", v1.Refusal)
	}
	v1.Refusal = try2WithRefusal.Refusal
	if err := validateProductionWorkContext(fixture.manifest, v1); err == nil {
		t.Fatal("expected validation failure for V1 schema with Refusal")
	}
}

func TestPreChangeJournalFixtureRemainsReadable(t *testing.T) {
	ctx := context.Background()
	store, err := journal.Open(ctx, filepath.Join(t.TempDir(), "prechange.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	now := time.Date(2026, 7, 29, 5, 6, 7, 0, time.UTC)
	runID := "run-prechange"
	manifestDigest := "sha256:0000000000000000000000000000000000000000000000000000000000000001"

	if err := store.RegisterRun(ctx, journal.Run{
		ID:             runID,
		ManifestDigest: manifestDigest,
		Repository:     "/workspace",
		Release:        "rel-prechange",
		TargetRef:      "refs/heads/main",
		CreatedAt:      now,
	}); err != nil {
		t.Fatal(err)
	}

	owner, err := store.AcquireOwner(ctx, runID, now, time.Minute, false)
	if err != nil {
		t.Fatal(err)
	}

	cmdPayload := []byte(`{"command":"test"}`)
	sum := sha256.Sum256([]byte("work-1"))
	workID := "sha256:" + hex.EncodeToString(sum[:])
	effectID := journal.AttemptEffectID(workID, 1, 1)

	command := journal.Command{
		RunID:     runID,
		ReplayKey: effectID,
		Kind:      "driver.dispatch",
		Payload:   cmdPayload,
		CreatedAt: now,
	}
	payloadSum := sha256.Sum256(cmdPayload)
	effect := journal.Effect{
		RunID:          runID,
		ID:             effectID,
		ReplayKey:      effectID,
		Kind:           "driver.dispatch",
		BeforeDigest:   manifestDigest,
		ExpectedDigest: "sha256:" + hex.EncodeToString(payloadSum[:]),
		UpdatedAt:      now,
	}
	if err := store.EnsureAttempt(ctx, command, effect, journal.EffectAttempt{WorkID: workID, Epoch: 1, Try: 1}); err != nil {
		t.Fatal(err)
	}

	claim, err := store.ClaimOwned(ctx, owner, effectID, now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	// Complete with operational failure without result (pre-change style)
	if err := store.CompleteOwned(ctx, owner, journal.Completion{
		RunID:     runID,
		EffectID:  effectID,
		Token:     claim.Token,
		State:     journal.OperationalFailed,
		ErrorCode: "CANDIDATE_SCOPE_FAILED",
		Result:    nil,
		EventKind: "dispatch_operational_failure",
		EventBody: []byte("failure"),
		At:        now,
	}); err != nil {
		t.Fatal(err)
	}

	// Verify reading effect
	readEffect, err := store.Effect(ctx, runID, effectID)
	if err != nil {
		t.Fatalf("failed to read pre-change effect: %v", err)
	}
	if readEffect.State != journal.OperationalFailed {
		t.Fatalf("effect state = %s, want operational_failed", readEffect.State)
	}
	if readEffect.ErrorCode != "CANDIDATE_SCOPE_FAILED" {
		t.Fatalf("error code = %s, want CANDIDATE_SCOPE_FAILED", readEffect.ErrorCode)
	}
	if len(readEffect.Result) != 0 {
		t.Fatalf("expected empty result, got: %v", readEffect.Result)
	}

	// Verify snapshot
	snapshot, err := store.Snapshot(ctx, runID)
	if err != nil {
		t.Fatalf("failed to snapshot pre-change journal: %v", err)
	}
	found := false
	for _, eff := range snapshot.Effects {
		if eff.ID == effectID {
			found = true
			if eff.State != journal.OperationalFailed || eff.ErrorCode != "CANDIDATE_SCOPE_FAILED" || len(eff.Result) != 0 {
				t.Fatalf("unexpected snapshot effect: %#v", eff)
			}
		}
	}
	if !found {
		t.Fatalf("snapshot missing effect %s: %#v", effectID, snapshot.Effects)
	}

	// Verify window read
	window, err := store.ReadWindow(ctx, runID, 0, 100)
	if err != nil {
		t.Fatalf("failed to read window from pre-change journal: %v", err)
	}
	foundWindow := false
	for _, eff := range window.Snapshot.Effects {
		if eff.ID == effectID {
			foundWindow = true
		}
	}
	if !foundWindow {
		t.Fatalf("window missing effect %s: %#v", effectID, window.Snapshot.Effects)
	}
}
