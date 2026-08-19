package runtime

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/journal"
)

type customWrappedErr struct {
	code string
	err  error
}

func (e *customWrappedErr) Error() string {
	if e.err != nil {
		return e.code + ": " + e.err.Error()
	}
	return e.code
}

func (e *customWrappedErr) Unwrap() error {
	return e.err
}

func TestStableErrorCodeUnwrapsJournalAndBatonErrorsWithPrecedence(t *testing.T) {
	t.Parallel()

	// A1: Journal error unwraps STALE_RETRY_EPOCH
	journalErr := &journal.Error{Code: "STALE_RETRY_EPOCH", Err: errors.New("stale epoch")}
	if got := stableErrorCode(journalErr); got != "STALE_RETRY_EPOCH" {
		t.Fatalf("stableErrorCode(journalErr) = %q, want STALE_RETRY_EPOCH", got)
	}

	// A2: Baton error unwraps TARGET_DIVERGED
	batonErr := &baton.RecordError{Code: "TARGET_DIVERGED", Msg: "target has diverged"}
	if got := stableErrorCode(batonErr); got != "TARGET_DIVERGED" {
		t.Fatalf("stableErrorCode(batonErr) = %q, want TARGET_DIVERGED", got)
	}

	// Precedence tests: Runtime > Gitx > Driver Contract > Journal > Baton > operational_failure
	runtimeErr := &Error{Code: "RUNTIME_CODE", Err: &gitx.Error{Code: "GITX_CODE"}}
	if got := stableErrorCode(runtimeErr); got != "RUNTIME_CODE" {
		t.Fatalf("runtime precedence = %q, want RUNTIME_CODE", got)
	}

	gitErr := &gitx.Error{Code: "GITX_CODE", Err: &journal.Error{Code: "JOURNAL_CODE"}}
	if got := stableErrorCode(gitErr); got != "GITX_CODE" {
		t.Fatalf("gitx precedence = %q, want GITX_CODE", got)
	}

	contractErr := &driver.ContractError{Code: "CONTRACT_CODE"}
	if got := stableErrorCode(contractErr); got != "CONTRACT_CODE" {
		t.Fatalf("driver contract precedence = %q, want CONTRACT_CODE", got)
	}

	wrappedContractErr := &customWrappedErr{
		code: "WRAPPED_CONTRACT",
		err: &customWrappedErr{
			code: "WRAP",
			err:  &driver.ContractError{Code: "CONTRACT_CODE"},
		},
	}
	if got := stableErrorCode(wrappedContractErr); got != "CONTRACT_CODE" {
		t.Fatalf("wrapped contract precedence = %q, want CONTRACT_CODE", got)
	}

	journalPrecedenceErr := &journal.Error{Code: "JOURNAL_CODE", Err: &baton.RecordError{Code: "BATON_CODE"}}
	if got := stableErrorCode(journalPrecedenceErr); got != "JOURNAL_CODE" {
		t.Fatalf("journal precedence = %q, want JOURNAL_CODE", got)
	}

	// Uncoded and malformed codes fallback to operational_failure
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"plain error", errors.New("generic error")},
		{"empty runtime code", &Error{Code: ""}},
		{"malformed journal code with spaces", &journal.Error{Code: "INVALID CODE"}},
		{"malformed baton code with symbols", &baton.RecordError{Code: "@INVALID!"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := stableErrorCode(tc.err); got != "operational_failure" {
				t.Fatalf("stableErrorCode(%s) = %q, want operational_failure", tc.name, got)
			}
		})
	}
}

func TestStatusReadBackSurfacesJournalCodedErrors(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 8, 19, 10, 0, 0, 0, time.UTC)
	manifestValue, manifestBody, _ := fixtureManifest(t)
	journalPath := filepath.Join(t.TempDir(), "status-coded-error.sqlite")
	store, err := journal.Open(ctx, journalPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	runID := manifestValue.RunID
	manifestDigest := sha256Digest(manifestBody)
	if err := store.RegisterRun(ctx, journal.Run{
		ID:             runID,
		ManifestDigest: manifestDigest,
		Repository:     manifestValue.Repository,
		Release:        manifestValue.Release,
		TargetRef:      manifestValue.TargetRef,
		CreatedAt:      now,
	}); err != nil {
		t.Fatal(err)
	}

	if err := store.RecordCommand(ctx, journal.Command{
		RunID:     runID,
		ReplayKey: "manifest",
		Kind:      "start",
		Payload:   manifestBody,
		CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	workID := "sha256:" + strings.Repeat("c", 64)
	effectID := journal.AttemptEffectID(workID, 1, 1)
	cmdPayload := []byte("{\"effect\":\"attempt\"}\n")
	if err := store.EnsureAttempt(
		ctx,
		journal.Command{
			RunID: runID, ReplayKey: effectID,
			Kind: "driver.dispatch", Payload: cmdPayload, CreatedAt: now,
		},
		journal.Effect{
			RunID: runID, ID: effectID, ReplayKey: effectID,
			Kind:           "driver.dispatch",
			BeforeDigest:   workID,
			ExpectedDigest: sha256Digest(cmdPayload),
			UpdatedAt:      now,
		},
		journal.EffectAttempt{WorkID: workID, Epoch: 1, Try: 1},
	); err != nil {
		t.Fatal(err)
	}

	claim, err := store.Claim(ctx, runID, effectID, now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	journalErr := &journal.Error{Code: "STALE_RETRY_EPOCH", Err: errors.New("retry epoch mismatch")}
	errorCode := stableErrorCode(journalErr)

	if err := store.Complete(ctx, journal.Completion{
		RunID:     runID,
		EffectID:  effectID,
		Token:     claim.Token,
		State:     journal.OperationalFailed,
		ErrorCode: errorCode,
		EventKind: "effect_operational_failure",
		At:        now,
	}); err != nil {
		t.Fatal(err)
	}

	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		t.Fatal(err)
	}

	service := &Service{
		journal:       store,
		gitExecutable: gitExecutable,
		now:           func() time.Time { return now },
	}

	status, err := service.Status(ctx, runID)
	if err != nil {
		t.Fatalf("service.Status() error = %v", err)
	}

	found := false
	for _, eff := range status.Effects {
		if eff.ID == effectID {
			found = true
			if eff.ErrorCode != "STALE_RETRY_EPOCH" {
				t.Fatalf("EffectStatus.ErrorCode = %q, want STALE_RETRY_EPOCH", eff.ErrorCode)
			}
			if eff.State != string(journal.OperationalFailed) {
				t.Fatalf("EffectStatus.State = %q, want OperationalFailed", eff.State)
			}
		}
	}
	if !found {
		t.Fatalf("effect %s not found in status.Effects: %#v", effectID, status.Effects)
	}
}

func TestFailedBatonActionJournalsTargetDivergedCode(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 8, 19, 11, 0, 0, 0, time.UTC)
	repoPath := productionRepository(t)
	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		t.Fatal(err)
	}

	manifestValue, _, plan := fixtureManifest(t)
	manifestValue.Repository = repoPath
	manifestBody, err := canonicalManifest(manifestValue)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := admitManifest(manifestBody)
	if err != nil {
		t.Fatal(err)
	}

	journalPath := filepath.Join(t.TempDir(), "baton-action-fail.sqlite")
	store, err := journal.Open(ctx, journalPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	run := journal.Run{
		ID:             manifest.value.RunID,
		ManifestDigest: manifest.digest,
		Repository:     manifest.value.Repository,
		Release:        manifest.value.Release,
		TargetRef:      manifest.value.TargetRef,
		CreatedAt:      now,
	}
	if err := store.RegisterRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordCommand(ctx, journal.Command{
		RunID: run.ID, ReplayKey: "manifest", Kind: "start",
		Payload: manifest.raw, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	owner, err := store.AcquireOwner(ctx, run.ID, now, time.Minute, false)
	if err != nil {
		t.Fatal(err)
	}

	service := &Service{
		journal:       store,
		gitExecutable: gitExecutable,
		now:           func() time.Time { return now },
	}
	engine, err := service.openEngine(manifest)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()

	// Install initial plan
	targetHead := runRuntimeGit(t, repoPath, "rev-parse", "refs/heads/main")
	admission := approvalAdmission{
		planBytes:  plan.Bytes(),
		planDigest: plan.Digest(),
		reference:  plan.Metadata().ApprovalRef,
	}
	if _, err := engine.installer.install(admission, targetHead); err != nil {
		t.Fatal(err)
	}

	sliceID := "S1"
	state, err := baton.ReadState(engine.git, manifest.value.Release, engine.inertness)
	if err != nil {
		t.Fatal(err)
	}
	slice, ok := state.Slice(sliceID)
	if !ok {
		t.Fatalf("slice %s absent", sliceID)
	}
	track, ok := state.Track(slice.Location.Track.ID)
	if !ok {
		t.Fatalf("track absent for slice %s", sliceID)
	}

	// Prepare append_receipt command
	input := baton.AppendReceiptInput{
		Release: manifest.value.Release,
		Slice:   sliceID,
		Role:    "implementer",
		Result:  "designed",
		Summary: "Design S1",
		Detail:  []byte("Design detail"),
	}
	before := sliceFingerprint(state, sliceID)
	workID := workIdentity(before, "append", input.Role, input.Result, input.Candidate)
	authority := stateActionAuthority(
		state, track.Ref, track.Head, before, slice.CurrentReceipt.OID,
		input.Candidate, slice.Attempt,
	)
	payload := marshalActionCommand(engine.manifest.value.GitIdentity, authority, input)

	// Failing baton action returning TARGET_DIVERGED
	divergedErr := &baton.RecordError{Code: "TARGET_DIVERGED", Msg: "target ref has diverged from record ancestry"}
	failingAction := func() (baton.ActionResult, error) {
		return baton.ActionResult{}, divergedErr
	}

	// Direct test of reconcileClaimedBatonAction
	effectID := journal.AttemptEffectID(workID, 1, 1)
	if err := store.EnsureAttempt(ctx,
		journal.Command{RunID: run.ID, ReplayKey: effectID, Kind: "baton.append_receipt",
			Payload: payload, CreatedAt: now},
		journal.Effect{RunID: run.ID, ID: effectID, ReplayKey: effectID, Kind: "baton.append_receipt",
			BeforeDigest: workID, ExpectedDigest: sha256Digest(payload), UpdatedAt: now},
		journal.EffectAttempt{WorkID: workID, Epoch: 1, Try: 1}); err != nil {
		t.Fatal(err)
	}
	claim, err := store.ClaimOwned(ctx, owner, effectID, now, effectLease)
	if err != nil {
		t.Fatal(err)
	}
	effect, err := store.Effect(ctx, run.ID, effectID)
	if err != nil {
		t.Fatal(err)
	}
	effect.CurrentClaim = claim.Token
	persisted, err := parseActionCommand(payload)
	if err != nil {
		t.Fatal(err)
	}

	truth, _, actionErr := service.reconcileClaimedBatonAction(
		ctx, engine, owner, effect, persisted, failingAction, true, false,
	)
	if truth != actionAllOld {
		t.Fatalf("truth = %q, want %q", truth, actionAllOld)
	}
	if !errors.Is(actionErr, divergedErr) {
		t.Fatalf("actionErr = %v, want divergedErr", actionErr)
	}

	storedEffect, err := store.Effect(ctx, run.ID, effectID)
	if err != nil {
		t.Fatalf("store.Effect() error = %v", err)
	}
	if storedEffect.State != journal.OperationalFailed {
		t.Fatalf("storedEffect.State = %q, want OperationalFailed", storedEffect.State)
	}
	if storedEffect.ErrorCode != "TARGET_DIVERGED" {
		t.Fatalf("storedEffect.ErrorCode = %q, want TARGET_DIVERGED (was baton_action_failed)", storedEffect.ErrorCode)
	}

	// Verify status projects this error code
	status, err := service.Status(ctx, run.ID)
	if err != nil {
		t.Fatalf("service.Status() error = %v", err)
	}
	found := false
	for _, eff := range status.Effects {
		if eff.ID == effectID {
			found = true
			if eff.ErrorCode != "TARGET_DIVERGED" {
				t.Fatalf("status effect error code = %q, want TARGET_DIVERGED", eff.ErrorCode)
			}
		}
	}
	if !found {
		t.Fatalf("effect %s not found in status.Effects", effectID)
	}
}
