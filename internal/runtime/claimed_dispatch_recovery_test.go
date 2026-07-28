package runtime

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/journal"
)

func claimedDispatchState(t *testing.T, plan baton.Plan) baton.State {
	t.Helper()
	metadata := plan.Metadata()
	sliceDefinition := metadata.Tracks[0].Slices[0]
	slice := &baton.SliceState{
		Location: baton.SliceLocation{
			Track: metadata.Tracks[0],
			Slice: sliceDefinition,
		},
		InputPins: map[string]string{},
		Stage:     "design",
		Status:    "ready",
		NextRole:  "implementer",
		Attempt:   1,
		CurrentReceipt: &baton.ReceiptEntry{
			OID: "receipt-design-authority",
		},
	}
	return baton.State{
		Release: metadata.Release,
		Plan: baton.PlanState{
			OID:      "plan-v1",
			Digest:   plan.Digest(),
			Metadata: metadata,
		},
		Refs: baton.StateRefs{
			Release: baton.CapturedRef{
				Ref:  "refs/heads/release-wt/" + metadata.Release,
				Head: "release-head-v1",
			},
			Target: baton.CapturedRef{
				Ref:  metadata.TargetRef,
				Head: "target-head-v1",
			},
		},
		Tracks: []baton.TrackState{{
			ID:     metadata.Tracks[0].ID,
			Ref:    "refs/heads/track/" + metadata.Release + "/T1",
			Head:   "track-head-v1",
			Slices: []*baton.SliceState{slice},
		}},
		Slices: []*baton.SliceState{slice},
	}
}

func implementationDispatchProofFixture(
	t *testing.T,
) (*engine, journal.Snapshot, implementationCycle, sealedRecord) {
	t.Helper()
	_, manifestBody, _ := fixtureManifest(t)
	manifest, err := admitManifest(manifestBody)
	if err != nil {
		t.Fatal(err)
	}
	var script ScriptedAttempt
	found := 0
	for _, candidate := range manifest.value.Scripts {
		if candidate.Slice == "S1" &&
			candidate.Responsibility == driver.ImplementerImplementation {
			script = candidate
			found++
		}
	}
	if found != 1 {
		t.Fatalf("implementation scripts = %d, want 1", found)
	}
	submissionBody, err := base64.StdEncoding.Strict().DecodeString(
		script.Submission,
	)
	if err != nil {
		t.Fatal(err)
	}
	submission, err := driver.DecodeSubmission(submissionBody)
	if err != nil {
		t.Fatal(err)
	}
	checks, err := exactBytes(submission.Checks)
	if err != nil {
		t.Fatal(err)
	}
	cycle := implementationCycle{
		Release:     manifest.value.Release,
		Slice:       "S1",
		Binds:       strings.Repeat("1", 40),
		Before:      "sha256:" + strings.Repeat("2", 64),
		Plan:        strings.Repeat("3", 40),
		ReleaseHead: strings.Repeat("4", 40),
		TargetHead:  strings.Repeat("5", 40),
		Track:       "T1",
		TrackRef: "refs/heads/track/" +
			manifest.value.Release + "/T1",
		TrackHead: strings.Repeat("6", 40),
	}
	cycle.DispatchWork = workIdentity("dispatch-proof")
	cycle.DispatchEffect = journal.AttemptEffectID(
		cycle.DispatchWork,
		1,
		1,
	)
	cycle.PreparedWork = workIdentity("prepared-proof")
	cycle.PreparedEffect = journal.AttemptEffectID(
		cycle.PreparedWork,
		1,
		1,
	)
	record := sealedRecord{
		Slice:     cycle.Slice,
		Binds:     cycle.Binds,
		Before:    cycle.TrackHead,
		Candidate: strings.Repeat("7", 40),
		Tree:      strings.Repeat("8", 40),
		ProductTree: "sha256:" +
			strings.Repeat("9", 64),
		Receipt: baton.AppendReceiptInput{
			Release:      cycle.Release,
			Slice:        cycle.Slice,
			Role:         "implementer",
			Result:       "candidate",
			Summary:      submission.Summary,
			Detail:       []byte(submission.Detail),
			Candidate:    strings.Repeat("7", 40),
			CheckResults: checks,
		},
	}
	commandPayload := mustJSON(fakeScript{
		SchemaVersion: "sworn.fake-script/v1",
		Behavior:      script.Behavior,
		Submission:    script.Submission,
	})
	snapshot := journal.Snapshot{
		Run: journal.Run{ID: manifest.value.RunID},
		Commands: []journal.Command{{
			RunID:     manifest.value.RunID,
			ReplayKey: cycle.DispatchEffect,
			Kind:      "driver.dispatch",
			Payload:   commandPayload,
		}},
		Effects: []journal.Effect{{
			RunID:          manifest.value.RunID,
			ID:             cycle.DispatchEffect,
			ReplayKey:      cycle.DispatchEffect,
			Kind:           "driver.dispatch",
			State:          journal.Succeeded,
			BeforeDigest:   sha256Digest([]byte(cycle.Before)),
			ExpectedDigest: sha256Digest(submissionBody),
			ResultDigest:   sha256Digest(submissionBody),
			Result:         submissionBody,
		}},
	}
	return &engine{manifest: manifest}, snapshot, cycle, record
}

func TestPreparedImplementationRequiresExactSucceededDispatchProof(
	t *testing.T,
) {
	exactEngine, snapshot, cycle, record :=
		implementationDispatchProofFixture(t)
	if err := validateImplementationDispatchProof(
		exactEngine,
		snapshot,
		cycle,
		record,
	); err != nil {
		t.Fatalf("exact dispatch proof rejected: %v", err)
	}

	tests := map[string]func(
		*engine,
		*journal.Snapshot,
		*sealedRecord,
	){
		"missing effect": func(
			_ *engine,
			snapshot *journal.Snapshot,
			_ *sealedRecord,
		) {
			snapshot.Effects = nil
		},
		"duplicate effect": func(
			_ *engine,
			snapshot *journal.Snapshot,
			_ *sealedRecord,
		) {
			snapshot.Effects = append(
				snapshot.Effects,
				snapshot.Effects[0],
			)
		},
		"failed effect": func(
			_ *engine,
			snapshot *journal.Snapshot,
			_ *sealedRecord,
		) {
			snapshot.Effects[0].State = journal.OperationalFailed
			snapshot.Effects[0].Result = nil
			snapshot.Effects[0].ResultDigest = ""
		},
		"coherent unadmitted submission": func(
			_ *engine,
			snapshot *journal.Snapshot,
			_ *sealedRecord,
		) {
			submission, err := driver.DecodeSubmission(
				snapshot.Effects[0].Result,
			)
			if err != nil {
				t.Fatal(err)
			}
			submission.Summary = "Substituted summary."
			body, err := driver.EncodeSubmission(submission)
			if err != nil {
				t.Fatal(err)
			}
			encoded := base64.StdEncoding.EncodeToString(body)
			snapshot.Commands[0].Payload = mustJSON(fakeScript{
				SchemaVersion: "sworn.fake-script/v1",
				Behavior:      "submit",
				Submission:    encoded,
			})
			snapshot.Effects[0].ExpectedDigest = sha256Digest(body)
			snapshot.Effects[0].ResultDigest = sha256Digest(body)
			snapshot.Effects[0].Result = body
		},
		"receipt detail": func(
			_ *engine,
			_ *journal.Snapshot,
			record *sealedRecord,
		) {
			record.Receipt.Detail = []byte("substituted detail")
		},
		"receipt checks": func(
			_ *engine,
			_ *journal.Snapshot,
			record *sealedRecord,
		) {
			record.Receipt.CheckResults = []byte("substituted checks")
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			engine, snapshot, cycle, record :=
				implementationDispatchProofFixture(t)
			mutate(engine, &snapshot, &record)
			if err := validateImplementationDispatchProof(
				engine,
				snapshot,
				cycle,
				record,
			); !IsCode(err, "CORRUPT_JOURNAL") {
				t.Fatalf("substituted proof error = %v", err)
			}
		})
	}
}

func TestImplementationCycleAuthorityDerivesSliceTrackFromPlan(
	t *testing.T,
) {
	_, _, plan := fixtureManifest(t)
	state := claimedDispatchState(t, plan)
	slice, _ := state.Slice("S1")
	slice.Stage = "implement"
	slice.NextRole = "implementer"
	state.Plan.Metadata.Tracks = append(
		state.Plan.Metadata.Tracks,
		baton.Track{ID: "T2"},
	)
	state.Tracks = append(state.Tracks, baton.TrackState{
		ID:   "T2",
		Ref:  "refs/heads/track/" + state.Release + "/T2",
		Head: "track-head-v2",
	})
	cycle := implementationCycle{
		Release:     state.Release,
		Slice:       "S1",
		Binds:       slice.CurrentReceipt.OID,
		Before:      sliceFingerprint(state, "S1"),
		Plan:        state.Plan.OID,
		ReleaseHead: state.Refs.Release.Head,
		TargetHead:  state.Refs.Target.Head,
		Track:       "T1",
		TrackRef:    "refs/heads/track/" + state.Release + "/T1",
		TrackHead:   state.Tracks[0].Head,
	}
	if err := validateImplementationCyclePlanAuthority(
		state,
		cycle,
	); err != nil {
		t.Fatalf("exact plan topology rejected: %v", err)
	}
	staleTarget := state
	staleTarget.Plan.TargetStale = true
	staleTarget.Refs.Target.Head = strings.Repeat("8", 40)
	if err := validateImplementationCyclePlanAuthority(
		staleTarget,
		cycle,
	); err != nil {
		t.Fatalf("historical target-bound cycle rejected after target drift: %v", err)
	}
	wrong := cycle
	wrong.Track = "T2"
	wrong.TrackRef = "refs/heads/track/" + state.Release + "/T2"
	wrong.TrackHead = state.Tracks[1].Head
	if err := validateImplementationCyclePlanAuthority(
		state,
		wrong,
	); !IsCode(err, "CORRUPT_JOURNAL") {
		t.Fatalf("wrong-track authority error = %v", err)
	}
}

func TestAllOldBatonAppendRequiresExactCurrentAuthorityBeforeCallback(
	t *testing.T,
) {
	_, _, plan := fixtureManifest(t)
	exact := func() (baton.State, batonActionCommand) {
		state := claimedDispatchState(t, plan)
		slice, _ := state.Slice("S1")
		slice.Stage = "implement"
		slice.NextRole = "implementer"
		state.Tracks[0].Head = strings.Repeat("9", 40)
		command := batonActionCommand{
			Authority: batonActionAuthority{
				Before:  sliceFingerprint(state, "S1"),
				Binds:   slice.CurrentReceipt.OID,
				Attempt: slice.Attempt,
			},
			Input: mustJSON(baton.AppendReceiptInput{
				Release:   state.Release,
				Slice:     "S1",
				Role:      "implementer",
				Result:    "candidate",
				Summary:   "Exact candidate.",
				Detail:    []byte("Exact detail."),
				Candidate: state.Tracks[0].Head,
				CheckResults: []byte(
					"exact checks\n",
				),
			}),
		}
		return state, command
	}
	state, command := exact()
	if err := validateBatonAllOldStateAuthority(
		state,
		"baton.append_receipt",
		command,
	); err != nil {
		t.Fatalf("exact append authority rejected: %v", err)
	}
	tests := map[string]func(*baton.State, *batonActionCommand){
		"binds": func(
			_ *baton.State,
			command *batonActionCommand,
		) {
			command.Authority.Binds = strings.Repeat("a", 40)
		},
		"attempt": func(
			_ *baton.State,
			command *batonActionCommand,
		) {
			command.Authority.Attempt++
		},
		"stage": func(
			state *baton.State,
			_ *batonActionCommand,
		) {
			slice, _ := state.Slice("S1")
			slice.Stage = "verify"
		},
		"candidate": func(
			_ *baton.State,
			command *batonActionCommand,
		) {
			var input baton.AppendReceiptInput
			if err := json.Unmarshal(command.Input, &input); err != nil {
				t.Fatal(err)
			}
			input.Candidate = strings.Repeat("b", 40)
			command.Input = mustJSON(input)
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			state, command := exact()
			mutate(&state, &command)
			if err := validateBatonAllOldStateAuthority(
				state,
				"baton.append_receipt",
				command,
			); !IsCode(err, "CORRUPT_JOURNAL") {
				t.Fatalf("stale authority error = %v", err)
			}
		})
	}
}

func TestImplementationReceiptAppliedUsesRetiredFullEvidence(
	t *testing.T,
) {
	_, _, plan := fixtureManifest(t)
	metadata := plan.Metadata()
	release := metadata.Release
	sliceID := "S1"
	planOID := strings.Repeat("1", 40)
	binds := strings.Repeat("2", 40)
	candidateOID := strings.Repeat("3", 40)
	tree := strings.Repeat("4", 40)
	productTree := "sha256:" + strings.Repeat("6", 64)
	attempt := int64(2)
	contract := metadata.Contracts[sliceID]
	checks := []byte("exact checks\n")
	detail := []byte("exact detail")
	record := sealedRecord{
		Slice:       sliceID,
		Binds:       binds,
		Before:      strings.Repeat("5", 40),
		Candidate:   candidateOID,
		Tree:        tree,
		ProductTree: productTree,
		Receipt: baton.AppendReceiptInput{
			Release: release, Slice: sliceID,
			Role: "implementer", Result: "candidate",
			Summary: "Exact retired candidate.",
			Detail:  detail, Candidate: candidateOID,
			CheckResults: checks,
		},
	}
	cycle := implementationCycle{
		Release: release, Slice: sliceID,
		Binds: binds, Plan: planOID,
	}
	bound := baton.ReceiptEntry{
		OID: binds,
		Receipt: baton.Receipt{
			Version: baton.ReceiptVersion,
			Release: release, Slice: &sliceID,
			Role: "captain", Result: "proceed",
			Attempt: &attempt, Plan: planOID,
			Contract: &contract,
		},
	}
	candidate := baton.ReceiptEntry{
		OID:    strings.Repeat("6", 40),
		Detail: append([]byte(nil), detail...),
		Receipt: baton.Receipt{
			Version: baton.ReceiptVersion,
			Release: release, Slice: &sliceID,
			Role: "implementer", Result: "candidate",
			Attempt: &attempt, Plan: planOID,
			Contract: &contract, Binds: binds,
			Detail:    baton.DigestBytes(detail),
			Summary:   record.Receipt.Summary,
			Candidate: &candidateOID, ProductTree: &productTree,
			Inputs: map[string]string{},
			Checks: func() *string {
				value := baton.DigestBytes(checks)
				return &value
			}(),
		},
	}
	exact := func() baton.State {
		return baton.State{
			Release: release,
			Plan: baton.PlanState{
				OID: planOID, Metadata: metadata,
			},
			SliceHistories: []baton.SliceHistoryState{{
				Slice: sliceID,
				Track: "T1",
				Ref: "refs/heads/track/" +
					release + "/T1",
				History: baton.SliceHistory{
					Entries: []baton.ReceiptEntry{
						bound.Clone(),
						candidate.Clone(),
					},
				},
			}},
		}
	}
	applied, err := implementationReceiptApplied(
		exact(),
		cycle,
		record,
	)
	if err != nil || !applied {
		t.Fatalf("retired exact receipt applied=%v err=%v", applied, err)
	}
	for name, mutate := range map[string]func(*baton.State){
		"detail": func(state *baton.State) {
			state.SliceHistories[0].History.Entries[1].Detail =
				[]byte("other detail")
		},
		"checks": func(state *baton.State) {
			value := baton.DigestBytes([]byte("other checks"))
			state.SliceHistories[0].History.Entries[1].
				Receipt.Checks = &value
		},
		"attempt": func(state *baton.State) {
			value := attempt + 1
			state.SliceHistories[0].History.Entries[1].
				Receipt.Attempt = &value
		},
		"product tree": func(state *baton.State) {
			value := "sha256:" + strings.Repeat("7", 64)
			state.SliceHistories[0].History.Entries[1].
				Receipt.ProductTree = &value
		},
	} {
		t.Run(name, func(t *testing.T) {
			state := exact()
			mutate(&state)
			applied, err := implementationReceiptApplied(
				state,
				cycle,
				record,
			)
			if err != nil || applied {
				t.Fatalf("substitution applied=%v err=%v", applied, err)
			}
		})
	}
	t.Run("ambiguous duplicate", func(t *testing.T) {
		state := exact()
		duplicate := candidate.Clone()
		duplicate.OID = strings.Repeat("7", 40)
		state.SliceHistories[0].History.Entries =
			append(
				state.SliceHistories[0].History.Entries,
				duplicate,
			)
		applied, err := implementationReceiptApplied(
			state,
			cycle,
			record,
		)
		if applied || !IsCode(err, "AMBIGUOUS_ACTION_HISTORY") {
			t.Fatalf("ambiguous applied=%v err=%v", applied, err)
		}
	})
}

func claimedDispatchJournal(
	t *testing.T,
	manifest admittedManifest,
) (*Service, *journal.Store, journal.OwnerLease, time.Time) {
	t.Helper()
	ctx := context.Background()
	now := time.Date(2026, 7, 27, 1, 2, 3, 0, time.UTC)
	store, err := journal.Open(ctx, filepath.Join(t.TempDir(), "run.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RegisterRun(ctx, journal.Run{
		ID:             manifest.value.RunID,
		ManifestDigest: manifest.digest,
		Repository:     manifest.value.Repository,
		Release:        manifest.value.Release,
		TargetRef:      manifest.value.TargetRef,
		CreatedAt:      now,
	}); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	owner, err := store.AcquireOwner(
		ctx,
		manifest.value.RunID,
		now,
		time.Minute,
		false,
	)
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	service := &Service{
		journal: store,
		now:     func() time.Time { return now },
	}
	return service, store, owner, now
}

func addClaimedDispatch(
	t *testing.T,
	store *journal.Store,
	owner journal.OwnerLease,
	now time.Time,
	work string,
	before string,
) journal.Effect {
	t.Helper()
	ctx := context.Background()
	id := journal.AttemptEffectID(work, 1, 1)
	command := journal.Command{
		RunID:     owner.RunID,
		ReplayKey: id,
		Kind:      "driver.dispatch",
		Payload: mustJSON(fakeScript{
			SchemaVersion: "sworn.fake-script/v1",
			Behavior:      "submit",
		}),
		CreatedAt: now,
	}
	effect := journal.Effect{
		RunID:          owner.RunID,
		ID:             id,
		ReplayKey:      id,
		Kind:           "driver.dispatch",
		BeforeDigest:   sha256Digest([]byte(before)),
		ExpectedDigest: sha256Digest(nil),
		UpdatedAt:      now,
	}
	if err := store.EnsureAttempt(
		ctx,
		command,
		effect,
		journal.EffectAttempt{WorkID: work, Epoch: 1, Try: 1},
	); err != nil {
		t.Fatal(err)
	}
	claim, err := store.ClaimOwned(ctx, owner, id, now, effectLease)
	if err != nil {
		t.Fatal(err)
	}
	effect.State = journal.Claimed
	effect.CurrentClaim = claim.Token
	return effect
}

func addClaimedSealEffect(
	t *testing.T,
	store *journal.Store,
	owner journal.OwnerLease,
	now time.Time,
	kind string,
	label string,
) journal.Effect {
	t.Helper()
	ctx := context.Background()
	work := workIdentity(label)
	id := journal.AttemptEffectID(work, 1, 1)
	payload := mustJSON(map[string]string{"fixture": label})
	effect := journal.Effect{
		RunID: owner.RunID, ID: id, ReplayKey: id, Kind: kind,
		BeforeDigest: work, ExpectedDigest: sha256Digest(payload),
		UpdatedAt: now,
	}
	if err := store.EnsureAttempt(
		ctx,
		journal.Command{
			RunID: owner.RunID, ReplayKey: id, Kind: kind,
			Payload: payload, CreatedAt: now,
		},
		effect,
		journal.EffectAttempt{WorkID: work, Epoch: 1, Try: 1},
	); err != nil {
		t.Fatal(err)
	}
	claim, err := store.ClaimOwned(ctx, owner, id, now, effectLease)
	if err != nil {
		t.Fatal(err)
	}
	effect.State = journal.Claimed
	effect.CurrentClaim = claim.Token
	return effect
}

func TestPendingDriverRecoveryFencesOnlyUnresolvedDispatches(t *testing.T) {
	for _, state := range []journal.EffectState{
		journal.Claimed,
		journal.Uncertain,
	} {
		t.Run(string(state), func(t *testing.T) {
			snapshot := journal.Snapshot{Effects: []journal.Effect{
				{Kind: "driver.dispatch", State: journal.Succeeded},
				{Kind: "baton.append_receipt", State: state},
				{Kind: "driver.dispatch", State: state},
			}}
			if !snapshotHasPendingDriverRecovery(snapshot) {
				t.Fatal("unresolved driver handoff did not fence scheduling")
			}
		})
	}
	for _, state := range []journal.EffectState{
		journal.Pending,
		journal.Succeeded,
		journal.OperationalFailed,
	} {
		t.Run(string(state), func(t *testing.T) {
			snapshot := journal.Snapshot{Effects: []journal.Effect{{
				Kind:  "driver.dispatch",
				State: state,
			}}}
			if snapshotHasPendingDriverRecovery(snapshot) {
				t.Fatalf("%s driver effect fenced scheduling", state)
			}
		})
	}
}

func TestClaimedDirectDispatchIsPreservedOnlyForExactCurrentAuthority(t *testing.T) {
	_, manifestBody, plan := fixtureManifest(t)
	manifest, err := admitManifest(manifestBody)
	if err != nil {
		t.Fatal(err)
	}
	state := claimedDispatchState(t, plan)
	before := sliceFingerprint(state, "S1")
	work := driverWorkIdentity(
		manifest.digest,
		"S1",
		driver.ImplementerDesign,
		1,
		before,
	)
	service, store, owner, now := claimedDispatchJournal(t, manifest)
	defer store.Close()
	effect := addClaimedDispatch(t, store, owner, now, work, before)
	snapshot, err := store.Snapshot(context.Background(), owner.RunID)
	if err != nil {
		t.Fatal(err)
	}
	engine := &engine{manifest: manifest}
	recovered, err := service.recoverStaleClaimedDispatchesFromSnapshot(
		context.Background(),
		engine,
		owner,
		snapshot,
		state,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !recovered {
		t.Fatal("exact current driver claim was not classified")
	}
	current, err := store.Effect(context.Background(), owner.RunID, effect.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.State != journal.Uncertain ||
		current.CurrentClaim != "" {
		t.Fatalf("current ambiguity = %#v", current)
	}

	// The identical historical claim becomes obsolete when its Baton plan
	// authority advances. Recovery must clear it atomically instead of allowing
	// status to remain poisoned by an expired claim forever.
	state.Plan.OID = "plan-v2"
	snapshot, err = store.Snapshot(context.Background(), owner.RunID)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err = service.recoverStaleClaimedDispatchesFromSnapshot(
		context.Background(),
		engine,
		owner,
		snapshot,
		state,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !recovered {
		t.Fatal("obsolete driver claim was not recovered")
	}
	terminal, err := store.Effect(context.Background(), owner.RunID, effect.ID)
	if err != nil {
		t.Fatal(err)
	}
	if terminal.State != journal.OperationalFailed ||
		terminal.ErrorCode != "stale_authority" ||
		terminal.CurrentClaim != "" {
		t.Fatalf("obsolete claim = %#v", terminal)
	}
}

func TestUncertainPlannerDispatchRemainsCurrentWithReadyTrack(t *testing.T) {
	_, manifestBody, plan := fixtureManifest(t)
	manifest, err := admitManifest(manifestBody)
	if err != nil {
		t.Fatal(err)
	}
	state := claimedDispatchState(t, plan)
	metadata := plan.Metadata()
	plannerSlice := &baton.SliceState{
		Location: baton.SliceLocation{
			Track: metadata.Tracks[1],
			Slice: metadata.Tracks[1].Slices[0],
		},
		InputPins: map[string]string{},
		Stage:     "verify",
		Status:    "blocked",
		NextRole:  "planner",
		Outcome:   "blocked",
		Attempt:   1,
		CurrentReceipt: &baton.ReceiptEntry{
			OID: "receipt-planner-authority",
		},
	}
	state.Tracks = append(state.Tracks, baton.TrackState{
		ID:     metadata.Tracks[1].ID,
		Ref:    "refs/heads/track/" + metadata.Release + "/T2",
		Head:   "track-head-v1",
		Slices: []*baton.SliceState{plannerSlice},
	})
	state.Slices = append(state.Slices, plannerSlice)
	authority := planProposalAuthority{
		Release:     manifest.value.Release,
		PriorPlan:   state.Plan.OID,
		ReleaseRef:  state.Refs.Release.Ref,
		ReleaseHead: state.Refs.Release.Head,
		TargetRef:   state.Refs.Target.Ref,
		TargetHead:  state.Refs.Target.Head,
	}
	authority.Before = plannerAuthorityBefore(authority)
	work := driverWorkIdentity(
		manifest.digest,
		"",
		driver.PlannerProposal,
		state.Plan.Metadata.Revision+1,
		authority.Before,
	)
	service, store, owner, now := claimedDispatchJournal(t, manifest)
	defer store.Close()
	effect := addClaimedDispatch(
		t,
		store,
		owner,
		now,
		work,
		authority.Before,
	)
	if err := store.ReconcileOwned(
		context.Background(),
		owner,
		journal.Completion{
			RunID: owner.RunID, EffectID: effect.ID,
			Token: effect.CurrentClaim, EventKind: "fixture_uncertain",
			At: now.Add(time.Second),
		},
		journal.RecoveryAmbiguous,
	); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(context.Background(), owner.RunID)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := service.recoverStaleClaimedDispatchesFromSnapshot(
		context.Background(),
		&engine{manifest: manifest},
		owner,
		snapshot,
		state,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if recovered {
		t.Fatal("current planner ambiguity was resolved while another track was ready")
	}
	current, err := store.Effect(
		context.Background(),
		owner.RunID,
		effect.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if current.State != journal.Uncertain || current.ErrorCode != "" {
		t.Fatalf("planner ambiguity = %#v", current)
	}
}

func TestPreparedSealReclaimUsesCurrentFenceForUncertainty(t *testing.T) {
	_, manifestBody, _ := fixtureManifest(t)
	manifest, err := admitManifest(manifestBody)
	if err != nil {
		t.Fatal(err)
	}
	service, store, owner, now := claimedDispatchJournal(t, manifest)
	defer store.Close()
	outer := addClaimedSealEffect(
		t,
		store,
		owner,
		now,
		"git.seal",
		"prepared-reclaim-outer",
	)
	prepared := addClaimedSealEffect(
		t,
		store,
		owner,
		now,
		"git.seal.prepared",
		"prepared-reclaim-child",
	)
	oldToken := prepared.CurrentClaim
	cycle := implementationCycle{
		Slice:          "S1",
		PreparedEffect: prepared.ID,
	}
	prepared, err = service.reclaimAllOldPreparedSeal(
		context.Background(),
		owner,
		cycle,
		prepared,
	)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.State != journal.Claimed ||
		prepared.CurrentClaim == "" ||
		prepared.CurrentClaim == oldToken {
		t.Fatalf("reclaimed prepared effect = %#v", prepared)
	}
	err = service.markSealCycleUncertain(
		context.Background(),
		owner,
		cycle,
		outer,
		prepared,
		errors.New("fixture third state"),
	)
	if !IsCode(err, "RECOVERY_UNCERTAIN") {
		t.Fatalf("third-state error = %v", err)
	}
	for _, id := range []string{prepared.ID, outer.ID} {
		effect, effectErr := store.Effect(
			context.Background(),
			owner.RunID,
			id,
		)
		if effectErr != nil {
			t.Fatal(effectErr)
		}
		if effect.State != journal.Uncertain ||
			effect.CurrentClaim != "" {
			t.Fatalf("uncertain effect %q = %#v", id, effect)
		}
	}
}

func TestUncertainDirectDispatchRemainsCurrentThenResolvesWhenSuperseded(
	t *testing.T,
) {
	_, manifestBody, plan := fixtureManifest(t)
	manifest, err := admitManifest(manifestBody)
	if err != nil {
		t.Fatal(err)
	}
	state := claimedDispatchState(t, plan)
	before := sliceFingerprint(state, "S1")
	work := driverWorkIdentity(
		manifest.digest,
		"S1",
		driver.ImplementerDesign,
		1,
		before,
	)
	service, store, owner, now := claimedDispatchJournal(t, manifest)
	defer store.Close()
	effect := addClaimedDispatch(t, store, owner, now, work, before)
	if err := store.ReconcileOwned(
		context.Background(),
		owner,
		journal.Completion{
			RunID: owner.RunID, EffectID: effect.ID,
			Token: effect.CurrentClaim, EventKind: "fixture_uncertain",
			At: now.Add(time.Second),
		},
		journal.RecoveryAmbiguous,
	); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(context.Background(), owner.RunID)
	if err != nil {
		t.Fatal(err)
	}
	engine := &engine{manifest: manifest}
	recovered, err := service.recoverStaleClaimedDispatchesFromSnapshot(
		context.Background(), engine, owner, snapshot, state, nil)
	if err != nil {
		t.Fatal(err)
	}
	if recovered {
		t.Fatal("exact current uncertain dispatch was resolved")
	}
	current, err := store.Effect(
		context.Background(), owner.RunID, effect.ID)
	if err != nil || current.State != journal.Uncertain {
		t.Fatalf("current uncertain effect = %#v, err=%v", current, err)
	}

	state.Plan.OID = "plan-v2"
	recovered, err = service.recoverStaleClaimedDispatchesFromSnapshot(
		context.Background(), engine, owner, snapshot, state, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !recovered {
		t.Fatal("superseded uncertain dispatch was not resolved")
	}
	terminal, err := store.Effect(
		context.Background(), owner.RunID, effect.ID)
	if err != nil ||
		terminal.State != journal.OperationalFailed ||
		terminal.ErrorCode != "stale_authority" {
		t.Fatalf("resolved uncertain effect = %#v, err=%v", terminal, err)
	}
}

func TestImplementationCycleEnvelopeBindsExactTopologyAndWork(t *testing.T) {
	_, manifestBody, _ := fixtureManifest(t)
	manifest, err := admitManifest(manifestBody)
	if err != nil {
		t.Fatal(err)
	}
	cycle := implementationCycle{
		Release:     manifest.value.Release,
		Slice:       "S1",
		Binds:       "1111111111111111111111111111111111111111",
		Before:      "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Plan:        "2222222222222222222222222222222222222222",
		ReleaseHead: "3333333333333333333333333333333333333333",
		TargetHead:  "4444444444444444444444444444444444444444",
		Track:       "T1",
		TrackRef: "refs/heads/track/" +
			manifest.value.Release + "/T1",
		TrackHead: "5555555555555555555555555555555555555555",
	}
	outerWork := workIdentity(cycle.Before, "git.seal")
	outerID := journal.AttemptEffectID(outerWork, 1, 1)
	cycle.DispatchWork = workIdentity(outerID, "driver.dispatch")
	cycle.DispatchEffect = journal.AttemptEffectID(
		cycle.DispatchWork, 1, 1)
	cycle.PreparedWork = workIdentity(outerID, "git.seal.prepared")
	cycle.PreparedEffect = journal.AttemptEffectID(
		cycle.PreparedWork, 1, 1)
	exact := func(value implementationCycle) (
		journal.Command,
		journal.Effect,
	) {
		payload := mustJSON(value)
		return journal.Command{
				RunID: "run-1", ReplayKey: outerID,
				Kind: "git.seal", Payload: payload,
			}, journal.Effect{
				RunID: "run-1", ID: outerID, ReplayKey: outerID,
				Kind: "git.seal", BeforeDigest: outerWork,
				ExpectedDigest: sha256Digest(payload),
			}
	}
	command, effect := exact(cycle)
	if got, err := validateImplementationCycleEnvelope(
		manifest,
		command,
		effect,
	); err != nil || got != cycle {
		t.Fatalf("exact cycle = %#v, err=%v", got, err)
	}
	tests := map[string]func(*implementationCycle, *journal.Command, *journal.Effect){
		"release": func(value *implementationCycle, _ *journal.Command, _ *journal.Effect) {
			value.Release = "other-release"
		},
		"track_ref": func(value *implementationCycle, _ *journal.Command, _ *journal.Effect) {
			value.TrackRef = "refs/heads/track/" +
				manifest.value.Release + "/T2"
		},
		"dispatch_work": func(value *implementationCycle, _ *journal.Command, _ *journal.Effect) {
			value.DispatchWork = workIdentity("substituted")
		},
		"prepared_effect": func(value *implementationCycle, _ *journal.Command, _ *journal.Effect) {
			value.PreparedEffect = journal.AttemptEffectID(
				workIdentity("substituted"), 1, 1)
		},
		"outer_work": func(_ *implementationCycle, _ *journal.Command, effect *journal.Effect) {
			effect.BeforeDigest = workIdentity("substituted")
		},
		"attempt": func(_ *implementationCycle, command *journal.Command, effect *journal.Effect) {
			other := journal.AttemptEffectID(
				workIdentity("substituted"), 1, 1)
			command.ReplayKey = other
			effect.ID, effect.ReplayKey = other, other
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			value := cycle
			command, effect := exact(value)
			mutate(&value, &command, &effect)
			if value != cycle {
				command.Payload = mustJSON(value)
				effect.ExpectedDigest = sha256Digest(command.Payload)
			}
			if _, err := validateImplementationCycleEnvelope(
				manifest,
				command,
				effect,
			); !IsCode(err, "CORRUPT_JOURNAL") {
				t.Fatalf("substituted cycle error = %v", err)
			}
		})
	}
	command, effect = exact(cycle)
	command.Payload = append([]byte(" "), command.Payload...)
	effect.ExpectedDigest = sha256Digest(command.Payload)
	if _, err := validateImplementationCycleEnvelope(
		manifest,
		command,
		effect,
	); !IsCode(err, "CORRUPT_JOURNAL") {
		t.Fatalf("noncanonical cycle error = %v", err)
	}
}

func TestClaimedImplementationDispatchRequiresItsExactLiveSealCycle(t *testing.T) {
	_, manifestBody, plan := fixtureManifest(t)
	manifest, err := admitManifest(manifestBody)
	if err != nil {
		t.Fatal(err)
	}
	state := claimedDispatchState(t, plan)
	slice, _ := state.Slice("S1")
	slice.Stage = "implement"
	slice.NextRole = "implementer"
	before := sliceFingerprint(state, "S1")
	outerWork := workIdentity(before, "git.seal")
	outerID := journal.AttemptEffectID(outerWork, 1, 1)
	cycle := implementationCycle{
		Release:     state.Release,
		Slice:       "S1",
		Binds:       slice.CurrentReceipt.OID,
		Before:      before,
		Plan:        state.Plan.OID,
		ReleaseHead: state.Refs.Release.Head,
		TargetHead:  state.Refs.Target.Head,
		Track:       state.Tracks[0].ID,
		TrackRef:    state.Tracks[0].Ref,
		TrackHead:   state.Tracks[0].Head,
		DispatchWork: workIdentity(
			outerID,
			"driver.dispatch",
		),
		PreparedWork: workIdentity(
			outerID,
			"git.seal.prepared",
		),
	}
	cycle.DispatchEffect = journal.AttemptEffectID(cycle.DispatchWork, 1, 1)
	cycle.PreparedEffect = journal.AttemptEffectID(cycle.PreparedWork, 1, 1)

	service, store, owner, now := claimedDispatchJournal(t, manifest)
	defer store.Close()
	outerPayload := mustJSON(cycle)
	if err := store.EnsureAttempt(
		context.Background(),
		journal.Command{
			RunID: owner.RunID, ReplayKey: outerID,
			Kind: "git.seal", Payload: outerPayload, CreatedAt: now,
		},
		journal.Effect{
			RunID: owner.RunID, ID: outerID, ReplayKey: outerID,
			Kind: "git.seal", BeforeDigest: outerWork,
			ExpectedDigest: sha256Digest(outerPayload), UpdatedAt: now,
		},
		journal.EffectAttempt{WorkID: outerWork, Epoch: 1, Try: 1},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ClaimOwned(
		context.Background(),
		owner,
		outerID,
		now,
		effectLease,
	); err != nil {
		t.Fatal(err)
	}
	child := addClaimedDispatch(
		t,
		store,
		owner,
		now,
		cycle.DispatchWork,
		cycle.Before,
	)
	snapshot, err := store.Snapshot(context.Background(), owner.RunID)
	if err != nil {
		t.Fatal(err)
	}
	engine := &engine{manifest: manifest}
	recovered, err := service.recoverStaleClaimedDispatchesFromSnapshot(
		context.Background(),
		engine,
		owner,
		snapshot,
		state,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if recovered {
		t.Fatal("current seal-cycle dispatch was terminalized")
	}

	state.Tracks[0].Head = "track-head-v2"
	recovered, err = service.recoverStaleClaimedDispatchesFromSnapshot(
		context.Background(),
		engine,
		owner,
		snapshot,
		state,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !recovered {
		t.Fatal("stale seal-cycle dispatch was not recovered")
	}
	terminal, err := store.Effect(context.Background(), owner.RunID, child.ID)
	if err != nil {
		t.Fatal(err)
	}
	if terminal.State != journal.OperationalFailed ||
		terminal.ErrorCode != "stale_authority" {
		t.Fatalf("stale implementation dispatch = %#v", terminal)
	}
}

func TestClaimedDispatchRecoveryRejectsSubstitutedCommandBinding(t *testing.T) {
	_, manifestBody, plan := fixtureManifest(t)
	manifest, err := admitManifest(manifestBody)
	if err != nil {
		t.Fatal(err)
	}
	state := claimedDispatchState(t, plan)
	before := sliceFingerprint(state, "S1")
	work := driverWorkIdentity(
		manifest.digest,
		"S1",
		driver.ImplementerDesign,
		1,
		before,
	)
	service, store, owner, now := claimedDispatchJournal(t, manifest)
	defer store.Close()
	effect := addClaimedDispatch(t, store, owner, now, work, before)
	snapshot, err := store.Snapshot(context.Background(), owner.RunID)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Commands[0].Kind = "baton.merge"
	_, err = service.recoverStaleClaimedDispatchesFromSnapshot(
		context.Background(),
		&engine{manifest: manifest},
		owner,
		snapshot,
		state,
		nil,
	)
	if !IsCode(err, "CORRUPT_JOURNAL") {
		t.Fatalf("substituted binding error = %v", err)
	}
	current, readErr := store.Effect(
		context.Background(),
		owner.RunID,
		effect.ID,
	)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if current.State != journal.Claimed {
		t.Fatalf("corrupt claim was mutated: %#v", current)
	}
}
