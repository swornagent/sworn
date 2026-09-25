package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/journal"
	"github.com/swornagent/sworn/internal/protocol"
)

// TestCredentialLifetimeCrossingOwnerMapsByResponsibility pins the Lead's
// required owner-mapping correction: a nested implementer_implementation
// dispatch is owned by its enclosing git.seal work (readyLaneCandidates,
// status.go:1027-1032), while implementer_design, lead_review,
// work_verification and assembly_verification are each already, themselves,
// the lane candidate work readyLaneCandidates adds (status.go:1017-1026,
// 1034-1036, 1105-1108) - no remapping. planner_proposal and
// lead_plan_review never reach here as a live crossing (they fail
// dispatchAuthorityCurrent's own default of false), so they map to "".
func TestCredentialLifetimeCrossingOwnerMapsByResponsibility(t *testing.T) {
	before := "sha256:" + strings.Repeat("c", 64)
	workID := "sha256:" + strings.Repeat("d", 64)

	cases := []struct {
		name           string
		responsibility driver.Responsibility
		want           string
	}{
		{
			name:           "implementer_implementation maps to its enclosing git.seal work",
			responsibility: driver.ImplementerImplementation,
			want:           workIdentity(before, "git.seal"),
		},
		{
			name:           "implementer_design is already the lane candidate work",
			responsibility: driver.ImplementerDesign,
			want:           workID,
		},
		{
			name:           "lead_review is already the lane candidate work",
			responsibility: driver.LeadReview,
			want:           workID,
		},
		{
			name:           "work_verification is already the lane candidate work",
			responsibility: driver.WorkVerification,
			want:           workID,
		},
		{
			name:           "assembly_verification is already the lane candidate work",
			responsibility: driver.AssemblyVerification,
			want:           workID,
		},
		{
			name:           "planner_proposal maps to nothing",
			responsibility: driver.PlannerProposal,
			want:           "",
		},
		{
			name:           "lead_plan_review maps to nothing",
			responsibility: driver.LeadPlanReview,
			want:           "",
		},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			crossing := credentialLifetimeCrossing{
				WorkID: workID, Before: before, Responsibility: test.responsibility,
			}
			if got := credentialLifetimeCrossingOwner(crossing); got != test.want {
				t.Fatalf("credentialLifetimeCrossingOwner(%s) = %q, want %q",
					test.responsibility, got, test.want)
			}
		})
	}
}

// TestRecordCredentialLifetimeParkFactContentAddressedReplayKey pins the
// write side's own idempotence: an identical refusal recorded twice is a
// true no-op (one event), while a legitimately different refusal for the
// identical (workID, epoch) - a different Detail - gets its own row instead
// of AppendEventOnce's content-equality check reporting REPLAY_CONFLICT.
func TestRecordCredentialLifetimeParkFactContentAddressedReplayKey(t *testing.T) {
	ctx := context.Background()
	repository := productionRepository(t)
	config := productionConfig(t)
	manifest := productionManifest(t, repository, config)
	now := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	store, err := journal.Open(ctx, filepath.Join(t.TempDir(), "journal.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.RegisterRun(ctx, journal.Run{
		ID: manifest.value.RunID, ManifestDigest: manifest.digest,
		Repository: manifest.value.Repository,
		Release:    manifest.value.Release, TargetRef: manifest.value.TargetRef,
		CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	production, err := newProductionDriverRuntime(config, driver.DriverFactoryOptions{})
	if err != nil {
		t.Fatal(err)
	}
	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		t.Fatal(err)
	}
	service := &Service{
		journal: store, dispatcher: driver.Dispatcher{}, production: production,
		gitExecutable: gitExecutable, now: func() time.Time { return now },
	}
	engine, err := service.openEngine(manifest)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()

	coordinates := dispatchCoordinates{
		Slice: "S1", Responsibility: driver.ImplementerDesign,
		ProtocolAttempt: 1, Epoch: 1, Try: 1,
	}
	before := "sha256:" + strings.Repeat("e", 64)

	if err := service.recordCredentialLifetimeParkFact(
		ctx, engine, coordinates, before, "CREDENTIAL_STALE", "",
	); err != nil {
		t.Fatalf("first record: %v", err)
	}
	if err := service.recordCredentialLifetimeParkFact(
		ctx, engine, coordinates, before, "CREDENTIAL_STALE", "",
	); err != nil {
		t.Fatalf("identical repeat conflicted instead of no-op: %v", err)
	}
	if err := service.recordCredentialLifetimeParkFact(
		ctx, engine, coordinates, before,
		"CREDENTIAL_EXPIRES_DURING_DISPATCH", "remaining 1m0s, required 5m0s",
	); err != nil {
		t.Fatalf(
			"a distinct refusal for the identical (workID, epoch) conflicted instead of appending: %v",
			err,
		)
	}

	snapshot, err := store.Snapshot(ctx, manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, event := range snapshot.Events {
		if event.Kind == "credential_lifetime_park" {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("credential_lifetime_park events = %d, want 2 (one per distinct body)", count)
	}
}

// credentialLifetimeReadyDesignWork mirrors readyLaneCandidates' own
// implementer-design branch (status.go:1017-1023) to compute the exact
// dispatch coordinates and work identity the fixture's single ready slice
// (T1/S1, design stage) would be dispatched under.
func credentialLifetimeReadyDesignWork(
	t *testing.T,
	f *economyGuardFixture,
) (workID, sliceID, before string, attempt int64) {
	t.Helper()
	state, err := protocol.ReadState(f.engine.git, f.manifest.value.Release, f.engine.inertness)
	if err != nil {
		t.Fatal(err)
	}
	for _, track := range state.Tracks {
		for _, slice := range track.Slices {
			if slice.Status == "ready" && slice.NextRole == "implementer" &&
				slice.Stage == "design" {
				sliceID = slice.Location.Slice.ID
				attempt = slice.Attempt
			}
		}
	}
	if sliceID == "" {
		t.Fatal("no ready implementer-design slice in fixture")
	}
	before = sliceFingerprint(state, sliceID)
	if before == "" {
		t.Fatal("empty slice fingerprint")
	}
	workID = driverWorkIdentity(
		f.manifest.digest, sliceID, driver.ImplementerDesign, attempt, before,
	)
	return workID, sliceID, before, attempt
}

// credentialLifetimeGateAdapterFixture installs a native adapter under
// profile key "planner" (which productionManifest binds every role,
// including Implementer, to - configured.validateSelected's own map-
// membership check admits it regardless of which role resolves it) with a
// live scripted CLI and a resolver bound to credentialPath, mirroring
// TestPrepareDriverDispatchPreflightRegistryRefusesStaleCredentialWithZeroBurn's
// own fixture construction.
func credentialLifetimeGateAdapterFixture(
	t *testing.T,
	engine *engine,
	credentialPath string,
) {
	t.Helper()
	executable := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(
		executable, []byte("#!/usr/bin/sh\necho '2.1.999 (Claude Code)'\nexit 0\n"), 0o700,
	); err != nil {
		t.Fatal(err)
	}
	executableBytes, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	targets := []string{
		"/etc/ssl/certs/ca-certificates.crt", "/etc/resolv.conf",
		"/etc/hosts", "/etc/nsswitch.conf",
	}
	files := make([]driver.PinnedRuntimeFile, len(targets))
	required := make([]string, len(targets))
	for index, target := range targets {
		sourcePath := filepath.Join(t.TempDir(), filepath.Base(target))
		if err := os.WriteFile(sourcePath, []byte(target+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		sourceBytes, err := os.ReadFile(sourcePath)
		if err != nil {
			t.Fatal(err)
		}
		files[index] = driver.PinnedRuntimeFile{
			Path: sourcePath, Target: target, Digest: driver.Digest(sourceBytes),
		}
		required[index] = target
	}
	nativeConfig := driver.NativeAdapterConfig{
		Key: "planner-native", ID: "sworn.claude", Version: "1.0.0",
		Family: driver.ProfileClaude,
		CLI: driver.ExecutableIdentity{
			Path: executable, Digest: driver.Digest(executableBytes),
		},
		CLIVersion: "2.1.999", VersionOutput: "2.1.999 (Claude Code)",
		RuntimeFiles: files, RequiredRuntimeTargets: required,
		CredentialTarget:   driver.ClaudeCredentialTarget,
		CredentialRefs:     []string{"planner-credential"},
		MaxCredentialBytes: 1_048_576,
		PinMode:            driver.NativePinModeMinor,
	}
	nativeAdapter, err := driver.NewNativeAdapter(
		nativeConfig,
		func(context.Context, string) (string, error) { return credentialPath, nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	ref := "planner-credential"
	nativeProfile := driver.ProfileConfig{
		Key: "planner", Adapter: nativeConfig.Key,
		Network: driver.NetworkRequired, CredentialRef: &ref,
	}
	registry, err := driver.NewSelectionRegistry(
		[]driver.ProfileConfig{nativeProfile}, []driver.Adapter{nativeAdapter},
	)
	if err != nil {
		t.Fatal(err)
	}
	engine.registry = registry
}

// TestCredentialLifetimeParkFactZeroBurnVisibleAndSelfHeals is the Lead's
// required recovery-path test: refuse a direct implementer_design dispatch
// (the lane candidate work is the dispatch WorkID itself, no owner
// remapping) at admission with an expired credential, prove it is zero-
// burn and shown on the status board, prove the lane is never excluded
// from scheduling (pinCrossingLanes), refresh the credential and re-drive
// under the identical coordinates, and prove the entry is gone once a
// dispatch attempt for that work is journaled.
func TestCredentialLifetimeParkFactZeroBurnVisibleAndSelfHeals(t *testing.T) {
	f := newEconomyGuardFixture(t, driver.Limits{
		TimeoutMillis: 120_000, OutputBytes: 65_536,
	})
	runID := f.manifest.value.RunID
	workID, sliceID, before, attempt := credentialLifetimeReadyDesignWork(t, f)

	credential := filepath.Join(t.TempDir(), "credential")
	expired := f.now.UnixMilli() - 60_000
	if err := os.WriteFile(
		credential,
		[]byte(`{"claudeAiOauth":{"accessToken":"a","expiresAt":`+
			strconv.FormatInt(expired, 10)+`}}`),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	credentialLifetimeGateAdapterFixture(t, f.engine, credential)

	target, _ := plannerProductionAuthority(t, f.engine)
	workspace, err := f.engine.workspaces.OpenSnapshot(target.Head)
	if err != nil {
		t.Fatal(err)
	}
	defer workspace.Close()

	coordinates := dispatchCoordinates{
		Slice: sliceID, Responsibility: driver.ImplementerDesign,
		ProtocolAttempt: attempt, Epoch: 1, Try: 1,
	}

	if _, err := f.service.prepareDriverDispatch(
		f.ctx, f.engine, workspace, driver.RoleImplementer, coordinates, before,
	); !IsCode(err, "CREDENTIAL_STALE") {
		t.Fatalf("prepareDriverDispatch error = %v, want CREDENTIAL_STALE", err)
	}

	// Zero burn: no attempt effect was ever written.
	effectID := journal.AttemptEffectID(workID, 1, 1)
	if _, effectErr := f.store.Effect(f.ctx, runID, effectID); !journal.IsCode(effectErr, "EFFECT_NOT_FOUND") {
		t.Fatalf("effect after admission refusal = %v, want EFFECT_NOT_FOUND", effectErr)
	}

	state, err := protocol.ReadState(f.engine.git, f.manifest.value.Release, f.engine.inertness)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := f.store.Snapshot(f.ctx, runID)
	if err != nil {
		t.Fatal(err)
	}

	crossings := credentialLifetimeParkCrossings(state, snapshot)
	if len(crossings) != 1 || crossings[0].WorkID != workID ||
		crossings[0].Code != "CREDENTIAL_STALE" {
		t.Fatalf("credentialLifetimeParkCrossings = %#v, want one CREDENTIAL_STALE crossing for %s",
			crossings, workID)
	}
	if owner := credentialLifetimeCrossingOwner(crossings[0]); owner != workID {
		t.Fatalf("crossing owner = %q, want the dispatch work itself %q", owner, workID)
	}

	// pinCrossingLanes must never exclude the lane: the probe has to keep
	// re-running on every drive round for this to ever self-heal.
	control, err := f.store.ControlProjection(f.ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	lanes := readyLaneCandidates(f.manifest, nil, true, state, snapshot)
	pinned, err := f.service.pinCrossingLanes(f.ctx, f.engine, runID, snapshot, control, lanes, state)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := pinned["T1"]; ok {
		t.Fatalf("pinCrossingLanes pinned lane T1 for a credential-lifetime crossing, want it excluded from pinning entirely")
	}

	// The board shows it: release the owner so State reflects the park
	// (an active owner reads "running" regardless, matching every other
	// cause).
	if err := f.store.ReleaseOwner(f.ctx, f.owner, f.now); err != nil {
		t.Fatal(err)
	}
	status, err := f.service.Status(f.ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	var found *PinnedWork
	for index := range status.PinnedWork {
		if status.PinnedWork[index].WorkID == workID {
			found = &status.PinnedWork[index]
		}
	}
	if found == nil || found.Cause != ParkCauseCredentialLifetime ||
		found.Code != "CREDENTIAL_STALE" {
		t.Fatalf("status.PinnedWork = %#v, want a credential_lifetime entry for %s",
			status.PinnedWork, workID)
	}
	if status.State != "parked" {
		t.Fatalf("status.State = %q, want parked", status.State)
	}

	// Refresh the credential and re-acquire the owner (an operator's next
	// drive), then redrive the identical coordinates: no Retry, no epoch
	// bump. This also proves the generic content-addressed replay-key
	// correction - without it, the probe-audit event's second, different-
	// outcome write would conflict and this call would fail
	// JOURNAL_WRITE_FAILED instead of succeeding.
	future := int64(8_000_000_000_000_000)
	if err := os.WriteFile(
		credential,
		[]byte(`{"claudeAiOauth":{"accessToken":"a","expiresAt":`+
			strconv.FormatInt(future, 10)+`}}`),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.AcquireOwner(f.ctx, runID, f.now, time.Minute, false); err != nil {
		t.Fatal(err)
	}
	// The credential probes must now pass cleanly under the identical
	// coordinates: neither a credential refusal (the self-heal claim) nor
	// JOURNAL_WRITE_FAILED (the replay-key hazard the redrive would hit
	// without the content-addressed suffix fix) may occur. Whatever
	// happens further into production work-context capture is outside
	// this test's scope.
	_, redriveErr := f.service.prepareDriverDispatch(
		f.ctx, f.engine, workspace, driver.RoleImplementer, coordinates, before,
	)
	if IsCode(redriveErr, "CREDENTIAL_STALE") ||
		IsCode(redriveErr, "CREDENTIAL_EXPIRES_DURING_DISPATCH") ||
		IsCode(redriveErr, "JOURNAL_WRITE_FAILED") {
		t.Fatalf(
			"redrive after refresh = %v, want the credential probe to pass (self-heal)",
			redriveErr,
		)
	}

	// Simulate the dispatch attempt that a passing admission now proceeds
	// to journal (runDriverEffectWithPreparation's own write, out of
	// prepareDriverDispatch's own scope): the clearing condition is that a
	// driver.dispatch attempt now exists at this work's (epoch, try).
	// effects.replay_key carries a foreign key into commands, so the
	// command and its effect are recorded together, exactly as every real
	// dispatch attempt does.
	attemptReplayKey := "credential-lifetime-test-attempt"
	if err := f.store.RecordCommandEffect(
		f.ctx,
		journal.Command{
			RunID: runID, ReplayKey: attemptReplayKey, Kind: "driver.dispatch",
			Payload: []byte("{}"), CreatedAt: f.now,
		},
		journal.Effect{
			RunID: runID, ID: effectID, ReplayKey: attemptReplayKey,
			Kind: "driver.dispatch", BeforeDigest: testWork(),
			ExpectedDigest: "sha256:" + strings.Repeat("f", 64),
			UpdatedAt:      f.now,
		},
	); err != nil {
		t.Fatal(err)
	}

	snapshot, err = f.store.Snapshot(f.ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if crossings := credentialLifetimeParkCrossings(state, snapshot); len(crossings) != 0 {
		t.Fatalf("crossings after refresh and attempt = %#v, want none (cleared)", crossings)
	}

	status, err = f.service.Status(f.ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	for _, work := range status.PinnedWork {
		if work.WorkID == workID && work.Cause == ParkCauseCredentialLifetime {
			t.Fatalf("status.PinnedWork still carries the cleared entry: %#v", work)
		}
	}
}

// TestCredentialLifetimeParkFactSurvivesEarlierTryAttempt pins the Lead's
// required correction-3 case: a try-1 attempt effect already exists for a
// work, try 2 is then refused at admission, and the resulting fact must
// still be shown on the status board. Correction 2's clearing condition
// only clears a fact whose own (Epoch, Try) is at or before a journaled
// attempt; an attempt strictly earlier than the fact's own try must never
// clear it, or every retried work's second-try refusal would vanish from
// the board the instant it was written.
func TestCredentialLifetimeParkFactSurvivesEarlierTryAttempt(t *testing.T) {
	f := newEconomyGuardFixture(t, driver.Limits{
		TimeoutMillis: 120_000, OutputBytes: 65_536,
	})
	runID := f.manifest.value.RunID
	workID, sliceID, before, attempt := credentialLifetimeReadyDesignWork(t, f)

	try1EffectID := journal.AttemptEffectID(workID, 1, 1)
	try1ReplayKey := "credential-lifetime-test-try1-attempt"
	if err := f.store.RecordCommandEffect(
		f.ctx,
		journal.Command{
			RunID: runID, ReplayKey: try1ReplayKey, Kind: "driver.dispatch",
			Payload: []byte("{}"), CreatedAt: f.now,
		},
		journal.Effect{
			RunID: runID, ID: try1EffectID, ReplayKey: try1ReplayKey,
			Kind: "driver.dispatch", BeforeDigest: testWork(),
			ExpectedDigest: "sha256:" + strings.Repeat("f", 64),
			UpdatedAt:      f.now,
		},
	); err != nil {
		t.Fatal(err)
	}

	coordinates := dispatchCoordinates{
		Slice: sliceID, Responsibility: driver.ImplementerDesign,
		ProtocolAttempt: attempt, Epoch: 1, Try: 2,
	}
	if err := f.service.recordCredentialLifetimeParkFact(
		f.ctx, f.engine, coordinates, before, "CREDENTIAL_STALE", "",
	); err != nil {
		t.Fatal(err)
	}

	state, err := protocol.ReadState(f.engine.git, f.manifest.value.Release, f.engine.inertness)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := f.store.Snapshot(f.ctx, runID)
	if err != nil {
		t.Fatal(err)
	}

	crossings := credentialLifetimeParkCrossings(state, snapshot)
	if len(crossings) != 1 || crossings[0].WorkID != workID || crossings[0].Try != 2 {
		t.Fatalf(
			"credentialLifetimeParkCrossings with only an earlier try-1 attempt = %#v, want the try-2 crossing to survive",
			crossings,
		)
	}

	if err := f.store.ReleaseOwner(f.ctx, f.owner, f.now); err != nil {
		t.Fatal(err)
	}
	status, err := f.service.Status(f.ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	var found *PinnedWork
	for index := range status.PinnedWork {
		if status.PinnedWork[index].WorkID == workID {
			found = &status.PinnedWork[index]
		}
	}
	if found == nil || found.Cause != ParkCauseCredentialLifetime ||
		found.Code != "CREDENTIAL_STALE" {
		t.Fatalf(
			"status.PinnedWork = %#v, want the try-2 credential_lifetime entry for %s despite the earlier try-1 attempt",
			status.PinnedWork, workID,
		)
	}
}

// TestCredentialLifetimeParkFactNestedGitSealOwnerEndToEnd pins the Lead's
// other required correction-3 case: a credential-lifetime refusal recorded
// for a nested implementer_implementation dispatch must read back, all the
// way through Status()'s PinnedWork and pinCrossingLanes' exclusion
// boundary, under its enclosing git.seal work - not the dispatch's own work
// identity - exactly mirroring the direct-dispatch path
// TestCredentialLifetimeParkFactZeroBurnVisibleAndSelfHeals already pins for
// implementer_design. TestCredentialLifetimeCrossingOwnerMapsByResponsibility
// pins credentialLifetimeCrossingOwner's mapping in isolation; this test
// exercises the same mapping through the full read path over a real
// git.seal-wrapped implementation cycle (newProductionImplementationRecoveryFixture).
func TestCredentialLifetimeParkFactNestedGitSealOwnerEndToEnd(t *testing.T) {
	fixture := newProductionImplementationRecoveryFixture(t, fixtureDriver(
		func(context.Context, driver.Invocation) (driver.Observation, error) {
			t.Fatal("dispatcher must not be invoked")
			return driver.Observation{}, nil
		},
	))
	runID := fixture.manifest.value.RunID
	outerWork := workIdentity(fixture.cycle.Before, "git.seal")

	// The production fixture does not journal the manifest command (its
	// own tests never project Status); record it so pinCrossingLanes and
	// Status can evaluate the park over this journal, exactly as
	// TestIdenticalFailureLiveDispatchAdmissionParksBeforeThirdTry does for
	// the identical-failure nested case.
	if err := fixture.store.RecordCommand(fixture.ctx, journal.Command{
		RunID: runID, ReplayKey: "manifest", Kind: "start",
		Payload: fixture.manifest.raw, CreatedAt: fixture.now,
	}); err != nil {
		t.Fatal(err)
	}

	if err := fixture.service.recordCredentialLifetimeParkFact(
		fixture.ctx, fixture.engine, fixture.coordinates, fixture.cycle.Before,
		"CREDENTIAL_STALE", "",
	); err != nil {
		t.Fatal(err)
	}

	state, err := protocol.ReadState(
		fixture.engine.git, fixture.manifest.value.Release, fixture.engine.inertness,
	)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := fixture.store.Snapshot(fixture.ctx, runID)
	if err != nil {
		t.Fatal(err)
	}

	crossings := credentialLifetimeParkCrossings(state, snapshot)
	if len(crossings) != 1 ||
		crossings[0].Responsibility != driver.ImplementerImplementation {
		t.Fatalf(
			"credentialLifetimeParkCrossings = %#v, want one implementer_implementation crossing",
			crossings,
		)
	}
	if owner := credentialLifetimeCrossingOwner(crossings[0]); owner != outerWork {
		t.Fatalf("crossing owner = %q, want the enclosing git.seal work %q", owner, outerWork)
	}

	// pinCrossingLanes must never exclude the lane for this cause, exactly
	// as the direct-dispatch case requires: the probe has to keep
	// re-running on every drive round for this to ever self-heal.
	control, err := fixture.store.ControlProjection(fixture.ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	lanes := readyLaneCandidates(fixture.manifest, nil, true, state, snapshot)
	pinned, err := fixture.service.pinCrossingLanes(
		fixture.ctx, fixture.engine, runID, snapshot, control, lanes, state,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := pinned[fixture.track.ID]; ok {
		t.Fatalf(
			"pinCrossingLanes pinned lane %s for a credential-lifetime crossing, want it excluded from pinning entirely",
			fixture.track.ID,
		)
	}

	status, err := fixture.service.Status(fixture.ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	var found *PinnedWork
	for index := range status.PinnedWork {
		if status.PinnedWork[index].WorkID == outerWork {
			found = &status.PinnedWork[index]
		}
	}
	if found == nil || found.Cause != ParkCauseCredentialLifetime ||
		found.Code != "CREDENTIAL_STALE" {
		t.Fatalf(
			"status.PinnedWork = %#v, want a credential_lifetime entry for the enclosing git.seal work %s",
			status.PinnedWork, outerWork,
		)
	}
}
