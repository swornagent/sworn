package runtime

import (
	"encoding/base64"
	"encoding/json"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/journal"
)

type activeCycleTestFixture struct {
	outerCommand    journal.Command
	dispatchCommand journal.Command
	outer           journal.Effect
	dispatch        journal.Effect
	cycle           implementationCycle
	attention       journal.AttentionProjection
}

type directAttentionTestFixture struct {
	production *productionImplementationRecoveryFixture
	prepared   preparedDriverDispatch
	command    journal.Command
	effectID   string
	work       string
	claim      journal.Claim
	recovery   turnRecoveryCycle
}

func newDirectAttentionTestFixture(
	t *testing.T,
) directAttentionTestFixture {
	t.Helper()

	fixture := newProductionImplementationRecoveryFixture(t, nil)
	prepared, err := fixture.service.prepareDriverDispatch(
		fixture.ctx,
		fixture.engine,
		fixture.workspace,
		driver.RoleImplementer,
		fixture.coordinates,
		fixture.cycle.Before,
	)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.productionContext == nil {
		t.Fatal("production context is absent")
	}
	persisted := prepared.productionContext
	work := driverWorkIdentity(
		fixture.manifest.digest,
		persisted.Slice,
		persisted.Responsibility,
		persisted.Attempt,
		persisted.Before,
	)
	effectID := journal.AttemptEffectID(
		work,
		persisted.Epoch,
		persisted.Try,
	)
	command := journal.Command{
		RunID:     fixture.owner.RunID,
		ReplayKey: effectID,
		Kind:      "driver.dispatch",
		Payload:   prepared.commandPayload,
		CreatedAt: fixture.now,
	}
	if err := fixture.store.EnsureAttempt(
		fixture.ctx,
		command,
		journal.Effect{
			RunID:          fixture.owner.RunID,
			ID:             effectID,
			ReplayKey:      effectID,
			Kind:           "driver.dispatch",
			BeforeDigest:   sha256Digest([]byte(persisted.Before)),
			ExpectedDigest: productionOutputExpectation,
			UpdatedAt:      fixture.now,
		},
		journal.EffectAttempt{
			WorkID: work,
			Epoch:  persisted.Epoch,
			Try:    persisted.Try,
		},
	); err != nil {
		t.Fatal(err)
	}
	claim, err := fixture.store.ClaimOwned(
		fixture.ctx,
		fixture.owner,
		effectID,
		fixture.now,
		effectLease,
	)
	if err != nil {
		t.Fatal(err)
	}
	recovery, err := turnRecoveryCycleForDispatch(
		fixture.manifest,
		prepared,
		fixture.coordinates,
		work,
		persisted.Before,
	)
	if err != nil {
		t.Fatal(err)
	}
	return directAttentionTestFixture{
		production: fixture,
		prepared:   prepared,
		command:    command,
		effectID:   effectID,
		work:       work,
		claim:      claim,
		recovery:   recovery,
	}
}

func activeImplementationCycleFixture(
	t *testing.T,
	manifest *admittedManifest,
	epoch, try int64,
	legacy bool,
) activeCycleTestFixture {
	t.Helper()

	before := "sha256:" + strings.Repeat("1", 64)
	outerWork := workIdentity(before, "git.seal")
	outerID := journal.AttemptEffectID(outerWork, epoch, try)
	cycle := implementationCycle{
		Release:     manifest.value.Release,
		Slice:       "S1",
		Binds:       strings.Repeat("2", 40),
		Before:      before,
		Plan:        strings.Repeat("3", 40),
		ReleaseHead: strings.Repeat("4", 40),
		TargetHead:  strings.Repeat("5", 40),
		Track:       "T1",
		TrackRef: "refs/heads/track/" +
			manifest.value.Release + "/T1",
		TrackHead: strings.Repeat("6", 40),
	}
	childEpoch, childTry := epoch, try
	if legacy {
		cycle.DispatchWork = workIdentity(
			outerID,
			"driver.dispatch",
		)
		cycle.PreparedWork = workIdentity(
			outerID,
			"git.seal.prepared",
		)
		childEpoch, childTry = 1, 1
	} else {
		cycle.DispatchWork = workIdentity(
			outerWork,
			"driver.dispatch",
		)
		cycle.PreparedWork = workIdentity(
			outerWork,
			"git.seal.prepared",
		)
	}
	cycle.DispatchEffect = journal.AttemptEffectID(
		cycle.DispatchWork,
		childEpoch,
		childTry,
	)
	cycle.PreparedEffect = journal.AttemptEffectID(
		cycle.PreparedWork,
		childEpoch,
		childTry,
	)
	payload := mustJSON(cycle)
	command := journal.Command{
		RunID:     manifest.value.RunID,
		ReplayKey: outerID,
		Kind:      "git.seal",
		Payload:   payload,
	}
	outer := journal.Effect{
		RunID:          manifest.value.RunID,
		ID:             outerID,
		ReplayKey:      outerID,
		Kind:           "git.seal",
		State:          journal.Claimed,
		BeforeDigest:   outerWork,
		ExpectedDigest: sha256Digest(payload),
		CurrentClaim:   strings.Repeat("a", 64),
	}
	dispatch := journal.Effect{
		RunID:        manifest.value.RunID,
		ID:           cycle.DispatchEffect,
		ReplayKey:    cycle.DispatchEffect,
		Kind:         "driver.dispatch",
		State:        journal.Claimed,
		CurrentClaim: strings.Repeat("b", 64),
	}
	var script ScriptedAttempt
	found := false
	for _, candidate := range manifest.value.Scripts {
		if candidate.Slice == cycle.Slice &&
			candidate.Responsibility ==
				driver.ImplementerImplementation &&
			candidate.BatonAttempt == 1 &&
			candidate.Epoch == childEpoch &&
			candidate.Try == childTry {
			script = candidate
			found = true
			break
		}
	}
	if !found {
		script = ScriptedAttempt{
			Slice:          cycle.Slice,
			Responsibility: driver.ImplementerImplementation,
			BatonAttempt:   1,
			Epoch:          childEpoch,
			Try:            childTry,
			Behavior:       "submit",
		}
		submission := driver.Submission{
			SchemaVersion: driver.SubmissionSchemaVersion,
			InvocationID: invocationID(
				manifest.value.RunID,
				script,
			),
			Responsibility: driver.ImplementerImplementation,
			Summary:        "Exact implementation.",
			Detail:         "Bounded fixture detail.",
		}
		submission.Checks, _ = driver.NewCheckBytes(
			[]byte("implementation checks\n"),
		)
		script.Submission = encodeSubmission(t, submission)
		manifest.value.Scripts = append(
			manifest.value.Scripts,
			script,
		)
	}
	submissionBody, err := base64.StdEncoding.Strict().DecodeString(
		script.Submission,
	)
	if err != nil {
		t.Fatal(err)
	}
	dispatch.ExpectedDigest = sha256Digest(submissionBody)
	dispatch.BeforeDigest = sha256Digest([]byte(cycle.Before))
	dispatchCommand := journal.Command{
		RunID:     manifest.value.RunID,
		ReplayKey: dispatch.ID,
		Kind:      "driver.dispatch",
		Payload: mustJSON(fakeScript{
			SchemaVersion: "sworn.fake-script/v1",
			Behavior:      script.Behavior,
			Submission:    script.Submission,
		}),
	}
	recovery, err := turnRecoveryCycleForDispatch(
		*manifest,
		preparedDriverDispatch{fake: true},
		dispatchCoordinates{
			Slice:          cycle.Slice,
			Responsibility: driver.ImplementerImplementation,
			BatonAttempt:   script.BatonAttempt,
			Epoch:          childEpoch,
			Try:            childTry,
		},
		cycle.DispatchWork,
		cycle.Before,
	)
	if err != nil {
		t.Fatal(err)
	}
	binding := journal.AttentionBinding{
		Ordinal:  1,
		Recovery: recovery.binding,
	}
	binding.ID = journal.AttentionID(binding.Recovery, binding.Ordinal)
	return activeCycleTestFixture{
		outerCommand:    command,
		dispatchCommand: dispatchCommand,
		outer:           outer,
		dispatch:        dispatch,
		cycle:           cycle,
		attention: journal.AttentionProjection{
			Attention:  binding,
			State:      journal.AttentionOpen,
			Generation: 1,
		},
	}
}

func TestActiveImplementationCycleSelectionIgnoresOnlyFailedRetryHistory(
	t *testing.T,
) {
	t.Parallel()

	_, manifestBody, _ := fixtureManifest(t)
	manifest, err := admitManifest(manifestBody)
	if err != nil {
		t.Fatal(err)
	}
	failed := activeImplementationCycleFixture(
		t, &manifest, 1, 1, false)
	active := activeImplementationCycleFixture(
		t, &manifest, 1, 2, false)
	failed.outer.State = journal.OperationalFailed
	failed.outer.CurrentClaim = ""
	failed.dispatch.State = journal.OperationalFailed
	failed.dispatch.CurrentClaim = ""
	activeWork := map[string]journal.AttentionProjection{
		active.cycle.DispatchWork: active.attention,
	}
	effects := map[string]journal.Effect{
		failed.outer.ID:    failed.outer,
		failed.dispatch.ID: failed.dispatch,
		active.outer.ID:    active.outer,
		active.dispatch.ID: active.dispatch,
	}
	commands := []journal.Command{
		failed.outerCommand,
		failed.dispatchCommand,
		active.outerCommand,
		active.dispatchCommand,
	}
	for _, reverse := range []bool{false, true} {
		ordered := append([]journal.Command(nil), commands...)
		if reverse {
			slices.Reverse(ordered)
		}
		selected, err := selectActiveImplementationCycles(
			manifest,
			ordered,
			effects,
			activeWork,
		)
		if err != nil {
			t.Fatalf("reverse=%t: %v", reverse, err)
		}
		if len(selected) != 1 ||
			selected[active.outer.ID].cycle.DispatchEffect !=
				active.dispatch.ID {
			t.Fatalf("reverse=%t selection = %#v", reverse, selected)
		}
		projected, err := projectedRecoveryClaims(
			manifest,
			journal.Snapshot{
				Commands: ordered,
				Effects: []journal.Effect{
					failed.outer,
					failed.dispatch,
					active.outer,
					active.dispatch,
				},
			},
			activeWork,
		)
		if err != nil {
			t.Fatalf("reverse=%t projected: %v", reverse, err)
		}
		if len(projected) != 2 ||
			projected[active.outer.ID] != journal.AttentionOpen ||
			projected[active.dispatch.ID] != journal.AttentionOpen {
			t.Fatalf("reverse=%t projected = %#v", reverse, projected)
		}
	}
}

func TestActiveImplementationCycleSelectionRejectsSucceededSibling(
	t *testing.T,
) {
	t.Parallel()

	_, manifestBody, _ := fixtureManifest(t)
	manifest, err := admitManifest(manifestBody)
	if err != nil {
		t.Fatal(err)
	}
	first := activeImplementationCycleFixture(
		t, &manifest, 1, 1, false)
	second := activeImplementationCycleFixture(
		t, &manifest, 1, 2, false)
	first.outer.State = journal.Succeeded
	first.outer.CurrentClaim = ""
	effects := map[string]journal.Effect{
		first.outer.ID:     first.outer,
		second.outer.ID:    second.outer,
		second.dispatch.ID: second.dispatch,
	}
	activeWork := map[string]journal.AttentionProjection{
		first.cycle.DispatchWork: second.attention,
	}
	for _, commands := range [][]journal.Command{
		{
			first.outerCommand,
			first.dispatchCommand,
			second.outerCommand,
			second.dispatchCommand,
		},
		{
			second.dispatchCommand,
			second.outerCommand,
			first.dispatchCommand,
			first.outerCommand,
		},
	} {
		if _, err := selectActiveImplementationCycles(
			manifest,
			commands,
			effects,
			activeWork,
		); !IsCode(err, "CORRUPT_JOURNAL") {
			t.Fatalf("succeeded sibling = %v", err)
		}
	}
}

func TestActiveImplementationCycleSelectionPreservesLegacyChildrenAndCachedSuccess(
	t *testing.T,
) {
	t.Parallel()

	_, manifestBody, _ := fixtureManifest(t)
	manifest, err := admitManifest(manifestBody)
	if err != nil {
		t.Fatal(err)
	}
	fixture := activeImplementationCycleFixture(
		t, &manifest, 3, 2, true)
	fixture.dispatch.State = journal.Succeeded
	fixture.dispatch.CurrentClaim = ""
	var script fakeScript
	if err := json.Unmarshal(
		fixture.dispatchCommand.Payload,
		&script,
	); err != nil {
		t.Fatal(err)
	}
	result, err := base64.StdEncoding.Strict().DecodeString(
		script.Submission,
	)
	if err != nil {
		t.Fatal(err)
	}
	fixture.dispatch.Result = result
	fixture.dispatch.ResultDigest = sha256Digest(result)
	effects := map[string]journal.Effect{
		fixture.outer.ID:    fixture.outer,
		fixture.dispatch.ID: fixture.dispatch,
	}
	answeredAttention := fixture.attention
	answeredAttention.State = journal.AttentionAnswered
	answeredAttention.Generation = 2
	answered := map[string]journal.AttentionProjection{
		fixture.cycle.DispatchWork: answeredAttention,
	}
	selected, err := selectActiveImplementationCycles(
		manifest,
		[]journal.Command{
			fixture.outerCommand,
			fixture.dispatchCommand,
		},
		effects,
		answered,
	)
	if err != nil {
		t.Fatal(err)
	}
	got := selected[fixture.outer.ID]
	work, epoch, try, err := attemptCoordinates(got.cycle.DispatchEffect)
	if err != nil ||
		work != fixture.cycle.DispatchWork ||
		epoch != 1 ||
		try != 1 {
		t.Fatalf(
			"legacy dispatch coordinates = %s/%d/%d, %v",
			work,
			epoch,
			try,
			err,
		)
	}
	open := map[string]journal.AttentionProjection{
		fixture.cycle.DispatchWork: fixture.attention,
	}
	if _, err := selectActiveImplementationCycles(
		manifest,
		[]journal.Command{
			fixture.outerCommand,
			fixture.dispatchCommand,
		},
		effects,
		open,
	); !IsCode(err, "CORRUPT_JOURNAL") {
		t.Fatalf("open cached success = %v", err)
	}
}

func TestProjectedRecoveryClaimsRejectsOrphanAttention(t *testing.T) {
	t.Parallel()

	_, manifestBody, _ := fixtureManifest(t)
	manifest, err := admitManifest(manifestBody)
	if err != nil {
		t.Fatal(err)
	}
	orphan := workIdentity("orphan-dispatch")
	if _, err := projectedRecoveryClaims(
		manifest,
		journal.Snapshot{},
		map[string]journal.AttentionProjection{
			orphan: {State: journal.AttentionOpen},
		},
	); !IsCode(err, "CORRUPT_JOURNAL") {
		t.Fatalf("orphan attention = %v", err)
	}
}

func TestImplementationAttentionRejectsWrongCycleOrLaneInStatus(
	t *testing.T,
) {
	t.Parallel()

	_, manifestBody, _ := fixtureManifest(t)
	manifest, err := admitManifest(manifestBody)
	if err != nil {
		t.Fatal(err)
	}
	fixture := activeImplementationCycleFixture(
		t,
		&manifest,
		1,
		1,
		false,
	)
	commands := []journal.Command{
		fixture.outerCommand,
		fixture.dispatchCommand,
	}
	effects := map[string]journal.Effect{
		fixture.outer.ID:    fixture.outer,
		fixture.dispatch.ID: fixture.dispatch,
	}
	snapshotEffects := []journal.Effect{
		fixture.outer,
		fixture.dispatch,
	}
	for _, test := range []struct {
		name   string
		mutate func(*journal.AttentionProjection)
	}{
		{
			name: "wrong cycle",
			mutate: func(attention *journal.AttentionProjection) {
				attention.Attention.Recovery.CycleID =
					driver.Digest([]byte("wrong-cycle"))
			},
		},
		{
			name: "wrong lane",
			mutate: func(attention *journal.AttentionProjection) {
				attention.Attention.Recovery.LaneID = "T9"
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			wrong := fixture.attention
			test.mutate(&wrong)
			wrong.Attention.ID = journal.AttentionID(
				wrong.Attention.Recovery,
				wrong.Attention.Ordinal,
			)
			activeWork := map[string]journal.AttentionProjection{
				fixture.cycle.DispatchWork: wrong,
			}
			if _, err := selectActiveImplementationCycles(
				manifest,
				commands,
				effects,
				activeWork,
			); !IsCode(err, "CORRUPT_JOURNAL") {
				t.Fatalf("selector binding = %v", err)
			}
			if _, err := projectedRecoveryClaims(
				manifest,
				journal.Snapshot{
					Commands: commands,
					Effects:  snapshotEffects,
				},
				activeWork,
			); !IsCode(err, "CORRUPT_JOURNAL") {
				t.Fatalf("status binding = %v", err)
			}
		})
	}
}

func TestImplementationRecoveryRejectsWrongAttentionCycleOrLane(
	t *testing.T,
) {
	for _, test := range []struct {
		name   string
		mutate func(*journal.RecoveryBinding)
	}{
		{
			name: "wrong cycle",
			mutate: func(binding *journal.RecoveryBinding) {
				binding.CycleID = driver.Digest([]byte("wrong-cycle"))
			},
		},
		{
			name: "wrong lane",
			mutate: func(binding *journal.RecoveryBinding) {
				binding.LaneID = "T9"
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newProductionImplementationRecoveryFixture(t, nil)
			prepared, err := fixture.service.prepareDriverDispatch(
				fixture.ctx,
				fixture.engine,
				fixture.workspace,
				driver.RoleImplementer,
				fixture.coordinates,
				fixture.cycle.Before,
			)
			if err != nil {
				t.Fatal(err)
			}
			if err := fixture.store.EnsureAttempt(
				fixture.ctx,
				journal.Command{
					RunID:     fixture.owner.RunID,
					ReplayKey: fixture.cycle.DispatchEffect,
					Kind:      "driver.dispatch",
					Payload:   prepared.commandPayload,
					CreatedAt: fixture.now,
				},
				journal.Effect{
					RunID:          fixture.owner.RunID,
					ID:             fixture.cycle.DispatchEffect,
					ReplayKey:      fixture.cycle.DispatchEffect,
					Kind:           "driver.dispatch",
					BeforeDigest:   sha256Digest([]byte(fixture.cycle.Before)),
					ExpectedDigest: productionOutputExpectation,
					UpdatedAt:      fixture.now,
				},
				journal.EffectAttempt{
					WorkID: fixture.cycle.DispatchWork,
					Epoch:  fixture.coordinates.Epoch,
					Try:    fixture.coordinates.Try,
				},
			); err != nil {
				t.Fatal(err)
			}
			if _, err := fixture.store.ClaimOwned(
				fixture.ctx,
				fixture.owner,
				fixture.cycle.DispatchEffect,
				fixture.now,
				effectLease,
			); err != nil {
				t.Fatal(err)
			}
			recovery, err := turnRecoveryCycleForDispatch(
				fixture.manifest,
				prepared,
				fixture.coordinates,
				fixture.cycle.DispatchWork,
				fixture.cycle.Before,
			)
			if err != nil {
				t.Fatal(err)
			}
			test.mutate(&recovery.binding)
			attention := journal.AttentionBinding{
				Ordinal:  1,
				Recovery: recovery.binding,
			}
			attention.ID = journal.AttentionID(
				attention.Recovery,
				attention.Ordinal,
			)
			if _, err := fixture.store.OpenAttention(
				fixture.ctx,
				fixture.owner,
				journal.OpenAttentionCommand{
					RunID:              fixture.owner.RunID,
					Attention:          attention,
					ExpectedGeneration: 0,
					Question:           "Use the wrong recovery binding?",
				},
				fixture.now,
			); err != nil {
				t.Fatal(err)
			}
			if err := fixture.workspace.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := fixture.service.recoverImplementationClaims(
				fixture.ctx,
				fixture.engine,
				fixture.owner,
			); !IsCode(err, "CORRUPT_JOURNAL") {
				t.Fatalf("recovery binding = %v", err)
			}
			currentAttention, err := fixture.store.Attention(
				fixture.ctx,
				fixture.owner.RunID,
				attention.ID,
			)
			if err != nil ||
				currentAttention.State != journal.AttentionOpen {
				t.Fatalf("wrong attention was consumed = %#v, %v",
					currentAttention, err)
			}
			for _, id := range []string{
				fixture.outer.ID,
				fixture.cycle.DispatchEffect,
			} {
				effect, err := fixture.store.Effect(
					fixture.ctx,
					fixture.owner.RunID,
					id,
				)
				if err != nil || effect.State != journal.Claimed {
					t.Fatalf("effect %s changed = %#v, %v",
						id, effect, err)
				}
			}
		})
	}
}

func TestDirectClaimedRecoveryRejectsWrongAttentionCycleOrLane(
	t *testing.T,
) {
	for _, test := range []struct {
		name   string
		mutate func(*journal.RecoveryBinding)
	}{
		{
			name: "wrong cycle",
			mutate: func(binding *journal.RecoveryBinding) {
				binding.CycleID =
					driver.Digest([]byte("wrong-direct-cycle"))
			},
		},
		{
			name: "wrong lane",
			mutate: func(binding *journal.RecoveryBinding) {
				binding.LaneID = "T9"
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newDirectAttentionTestFixture(t)
			binding := fixture.recovery.binding
			test.mutate(&binding)
			attention := journal.AttentionBinding{
				Ordinal:  1,
				Recovery: binding,
			}
			attention.ID = journal.AttentionID(
				attention.Recovery,
				attention.Ordinal,
			)
			if _, err := fixture.production.store.OpenAttention(
				fixture.production.ctx,
				fixture.production.owner,
				journal.OpenAttentionCommand{
					RunID:              fixture.production.owner.RunID,
					Attention:          attention,
					ExpectedGeneration: 0,
					Question:           "Use the wrong direct recovery binding?",
				},
				fixture.production.now,
			); err != nil {
				t.Fatal(err)
			}
			if err := fixture.production.workspace.Close(); err != nil {
				t.Fatal(err)
			}
			state, err := baton.ReadState(
				fixture.production.engine.git,
				fixture.production.manifest.value.Release,
				fixture.production.engine.inertness,
			)
			if err != nil {
				t.Fatal(err)
			}
			effect, err := fixture.production.store.Effect(
				fixture.production.ctx,
				fixture.production.owner.RunID,
				fixture.effectID,
			)
			if err != nil {
				t.Fatal(err)
			}
			recovered, err := fixture.production.service.
				recoverStaleClaimedDispatchesFromSnapshot(
					fixture.production.ctx,
					fixture.production.engine,
					fixture.production.owner,
					journal.Snapshot{
						Commands: []journal.Command{
							fixture.command,
						},
						Effects: []journal.Effect{effect},
					},
					state,
					nil,
				)
			if !recovered || !IsCode(err, "CORRUPT_JOURNAL") {
				t.Fatalf("direct recovery binding = %t, %v",
					recovered, err)
			}
			currentAttention, err := fixture.production.store.Attention(
				fixture.production.ctx,
				fixture.production.owner.RunID,
				attention.ID,
			)
			if err != nil ||
				currentAttention.State != journal.AttentionOpen {
				t.Fatalf("wrong direct attention was consumed = %#v, %v",
					currentAttention, err)
			}
			effect, err = fixture.production.store.Effect(
				fixture.production.ctx,
				fixture.production.owner.RunID,
				fixture.effectID,
			)
			if err != nil ||
				effect.State != journal.Claimed ||
				effect.CurrentClaim != fixture.claim.Token {
				t.Fatalf("direct claim changed = %#v, %v", effect, err)
			}
		})
	}
}

func TestRecoverImplementationClaimsConsumesAnsweredCachedChildSuccess(
	t *testing.T,
) {
	fixture := newProductionImplementationRecoveryFixture(t, nil)
	record, _, dispatchClaim := prepareClaimedProductionImplementation(
		t,
		fixture,
		"cached recovery candidate\n",
	)
	snapshot, err := fixture.store.Snapshot(
		fixture.ctx,
		fixture.owner.RunID,
	)
	if err != nil {
		t.Fatal(err)
	}
	var dispatchCommand journal.Command
	found := 0
	for _, command := range snapshot.Commands {
		if command.ReplayKey == fixture.cycle.DispatchEffect {
			dispatchCommand = command
			found++
		}
	}
	if found != 1 {
		t.Fatalf("dispatch commands = %d", found)
	}
	persisted, err := parseProductionDispatchCommand(
		fixture.manifest,
		dispatchCommand.Payload,
	)
	if err != nil {
		t.Fatal(err)
	}
	recovery, err := turnRecoveryCycleForDispatch(
		fixture.manifest,
		preparedDriverDispatch{
			productionContext: &persisted.Context,
		},
		fixture.coordinates,
		fixture.cycle.DispatchWork,
		fixture.cycle.Before,
	)
	if err != nil {
		t.Fatal(err)
	}
	attention := journal.AttentionBinding{
		Ordinal:  1,
		Recovery: recovery.binding,
	}
	attention.ID = journal.AttentionID(
		attention.Recovery,
		attention.Ordinal,
	)
	if _, err := fixture.store.OpenAttention(
		fixture.ctx,
		fixture.owner,
		journal.OpenAttentionCommand{
			RunID:              fixture.owner.RunID,
			Attention:          attention,
			ExpectedGeneration: 0,
			Question:           "Use the exact cached recovery value?",
		},
		fixture.now,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.AnswerAttention(
		fixture.ctx,
		journal.AnswerAttentionCommand{
			RunID:              fixture.owner.RunID,
			Attention:          attention,
			ExpectedGeneration: 1,
			Answer:             "Use the exact cached recovery value.",
		},
		fixture.now,
	); err != nil {
		t.Fatal(err)
	}
	submission, submissionBody := productionImplementationSubmission(
		t,
		persisted.Context.InvocationID,
	)
	if record.Receipt.Summary != submission.Summary {
		t.Fatalf("prepared/cached submission mismatch: %#v", record)
	}
	if err := fixture.store.CompleteOwned(
		fixture.ctx,
		fixture.owner,
		journal.Completion{
			RunID:     fixture.owner.RunID,
			EffectID:  fixture.cycle.DispatchEffect,
			Token:     dispatchClaim.Token,
			State:     journal.Succeeded,
			Result:    submissionBody,
			EventKind: "driver_completed",
			EventBody: []byte("driver.dispatch"),
			At:        fixture.now,
		},
	); err != nil {
		t.Fatal(err)
	}
	answeredProjection, err := fixture.store.Attention(
		fixture.ctx,
		fixture.owner.RunID,
		attention.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err = fixture.store.Snapshot(
		fixture.ctx,
		fixture.owner.RunID,
	)
	if err != nil {
		t.Fatal(err)
	}
	projected, err := projectedRecoveryClaims(
		fixture.manifest,
		snapshot,
		map[string]journal.AttentionProjection{
			fixture.cycle.DispatchWork: answeredProjection,
		},
	)
	if err != nil ||
		projected[fixture.outer.ID] != journal.AttentionAnswered {
		t.Fatalf("cached-success status projection = %#v, %v",
			projected, err)
	}
	if err := fixture.workspace.Close(); err != nil {
		t.Fatal(err)
	}
	recovered, err := fixture.service.recoverImplementationClaims(
		fixture.ctx,
		fixture.engine,
		fixture.owner,
	)
	if err != nil || !recovered {
		t.Fatalf("cached-success recovery = %t, %v", recovered, err)
	}
	fresh, err := baton.ReadState(
		fixture.engine.git,
		fixture.manifest.value.Release,
		fixture.engine.inertness,
	)
	if err != nil {
		t.Fatal(err)
	}
	slice, present := fresh.Slice(fixture.cycle.Slice)
	if !present ||
		slice.Stage != "verify" ||
		slice.NextRole != "verifier" {
		t.Fatalf("cached-success slice = %#v", slice)
	}
	currentAttention, err := fixture.store.Attention(
		fixture.ctx,
		fixture.owner.RunID,
		attention.ID,
	)
	if err != nil ||
		currentAttention.State != journal.AttentionResolved {
		t.Fatalf("cached-success attention = %#v, %v", currentAttention, err)
	}
	for _, expected := range []struct {
		id    string
		state journal.EffectState
	}{
		{fixture.outer.ID, journal.Succeeded},
		{fixture.cycle.DispatchEffect, journal.Succeeded},
		{fixture.cycle.PreparedEffect, journal.Succeeded},
	} {
		effect, err := fixture.store.Effect(
			fixture.ctx,
			fixture.owner.RunID,
			expected.id,
		)
		if err != nil || effect.State != expected.state {
			t.Fatalf("effect %s = %#v, %v", expected.id, effect, err)
		}
	}
}

func TestStaleSweepRetiresAnsweredTerminalDirectDispatch(t *testing.T) {
	fixture := newProductionImplementationRecoveryFixture(t, nil)
	prepared, err := fixture.service.prepareDriverDispatch(
		fixture.ctx,
		fixture.engine,
		fixture.workspace,
		driver.RoleImplementer,
		fixture.coordinates,
		fixture.cycle.Before,
	)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.productionContext == nil {
		t.Fatal("production context is absent")
	}
	persistedContext := *prepared.productionContext
	dispatchWork := driverWorkIdentity(
		fixture.manifest.digest,
		persistedContext.Slice,
		persistedContext.Responsibility,
		persistedContext.Attempt,
		persistedContext.Before,
	)
	dispatchEffectID := journal.AttemptEffectID(
		dispatchWork,
		persistedContext.Epoch,
		persistedContext.Try,
	)
	dispatchCommand := journal.Command{
		RunID:     fixture.owner.RunID,
		ReplayKey: dispatchEffectID,
		Kind:      "driver.dispatch",
		Payload:   prepared.commandPayload,
		CreatedAt: fixture.now,
	}
	if err := fixture.store.EnsureAttempt(
		fixture.ctx,
		dispatchCommand,
		journal.Effect{
			RunID:          fixture.owner.RunID,
			ID:             dispatchEffectID,
			ReplayKey:      dispatchEffectID,
			Kind:           "driver.dispatch",
			BeforeDigest:   sha256Digest([]byte(fixture.cycle.Before)),
			ExpectedDigest: productionOutputExpectation,
			UpdatedAt:      fixture.now,
		},
		journal.EffectAttempt{
			WorkID: dispatchWork,
			Epoch:  persistedContext.Epoch,
			Try:    persistedContext.Try,
		},
	); err != nil {
		t.Fatal(err)
	}
	dispatchClaim, err := fixture.store.ClaimOwned(
		fixture.ctx,
		fixture.owner,
		dispatchEffectID,
		fixture.now,
		effectLease,
	)
	if err != nil {
		t.Fatal(err)
	}
	recovery, err := turnRecoveryCycleForDispatch(
		fixture.manifest,
		prepared,
		fixture.coordinates,
		dispatchWork,
		fixture.cycle.Before,
	)
	if err != nil {
		t.Fatal(err)
	}
	attention := journal.AttentionBinding{
		Ordinal:  1,
		Recovery: recovery.binding,
	}
	attention.ID = journal.AttentionID(
		attention.Recovery,
		attention.Ordinal,
	)
	if _, err := fixture.store.OpenAttention(
		fixture.ctx,
		fixture.owner,
		journal.OpenAttentionCommand{
			RunID:              fixture.owner.RunID,
			Attention:          attention,
			ExpectedGeneration: 0,
			Question:           "Use the exact stale terminal value?",
		},
		fixture.now,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.AnswerAttention(
		fixture.ctx,
		journal.AnswerAttentionCommand{
			RunID:              fixture.owner.RunID,
			Attention:          attention,
			ExpectedGeneration: 1,
			Answer:             "Use the exact stale terminal value.",
		},
		fixture.now,
	); err != nil {
		t.Fatal(err)
	}
	claimedProjection, err := fixture.store.Attention(
		fixture.ctx,
		fixture.owner.RunID,
		attention.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	claimedEffect, err := fixture.store.Effect(
		fixture.ctx,
		fixture.owner.RunID,
		dispatchEffectID,
	)
	if err != nil {
		t.Fatal(err)
	}
	claimedSnapshot := journal.Snapshot{
		Commands: []journal.Command{dispatchCommand},
		Effects:  []journal.Effect{claimedEffect},
	}
	for _, mutate := range []func(*journal.AttentionProjection){
		func(value *journal.AttentionProjection) {
			value.Attention.Recovery.CycleID =
				driver.Digest([]byte("wrong-direct-cycle"))
		},
		func(value *journal.AttentionProjection) {
			value.Attention.Recovery.LaneID = "T9"
		},
	} {
		wrong := claimedProjection
		mutate(&wrong)
		wrong.Attention.ID = journal.AttentionID(
			wrong.Attention.Recovery,
			wrong.Attention.Ordinal,
		)
		if _, err := projectedRecoveryClaims(
			fixture.manifest,
			claimedSnapshot,
			map[string]journal.AttentionProjection{
				dispatchWork: wrong,
			},
		); !IsCode(err, "CORRUPT_JOURNAL") {
			t.Fatalf("wrong direct claimed binding = %v", err)
		}
	}
	_, submissionBody := productionImplementationSubmission(
		t,
		persistedContext.InvocationID,
	)
	if err := fixture.store.CompleteOwned(
		fixture.ctx,
		fixture.owner,
		journal.Completion{
			RunID:     fixture.owner.RunID,
			EffectID:  dispatchEffectID,
			Token:     dispatchClaim.Token,
			State:     journal.Succeeded,
			Result:    submissionBody,
			EventKind: "driver_completed",
			EventBody: []byte("driver.dispatch"),
			At:        fixture.now,
		},
	); err != nil {
		t.Fatal(err)
	}
	if err := fixture.workspace.Close(); err != nil {
		t.Fatal(err)
	}
	staleState, err := baton.ReadState(
		fixture.engine.git,
		fixture.manifest.value.Release,
		fixture.engine.inertness,
	)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := fixture.store.Snapshot(
		fixture.ctx,
		fixture.owner.RunID,
	)
	if err != nil {
		t.Fatal(err)
	}
	answeredProjection, err := fixture.store.Attention(
		fixture.ctx,
		fixture.owner.RunID,
		attention.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	directSnapshot := journal.Snapshot{
		Run:      snapshot.Run,
		Commands: []journal.Command{dispatchCommand},
	}
	dispatchEffect, err := fixture.store.Effect(
		fixture.ctx,
		fixture.owner.RunID,
		dispatchEffectID,
	)
	if err != nil {
		t.Fatal(err)
	}
	directSnapshot.Effects = []journal.Effect{dispatchEffect}
	projected, err := projectedRecoveryClaims(
		fixture.manifest,
		directSnapshot,
		map[string]journal.AttentionProjection{
			dispatchWork: answeredProjection,
		},
	)
	if err != nil ||
		projected[dispatchEffectID] != journal.AttentionAnswered {
		t.Fatalf("terminal direct status projection = %#v, %v",
			projected, err)
	}
	staleSlice, present := staleState.Slice(fixture.cycle.Slice)
	if !present {
		t.Fatal("fixture slice is absent")
	}
	staleSlice.Status = "complete"
	staleSlice.NextRole = "none"
	recovered, err := fixture.service.
		recoverStaleClaimedDispatchesFromSnapshot(
			fixture.ctx,
			fixture.engine,
			fixture.owner,
			directSnapshot,
			staleState,
			nil,
		)
	if err != nil || !recovered {
		t.Fatalf("stale terminal sweep = %t, %v", recovered, err)
	}
	currentAttention, err := fixture.store.Attention(
		fixture.ctx,
		fixture.owner.RunID,
		attention.ID,
	)
	if err != nil ||
		currentAttention.State != journal.AttentionCancelled {
		t.Fatalf("retired terminal attention = %#v, %v", currentAttention, err)
	}
	dispatchEffect, err = fixture.store.Effect(
		fixture.ctx,
		fixture.owner.RunID,
		dispatchEffectID,
	)
	if err != nil ||
		dispatchEffect.State != journal.Succeeded ||
		dispatchEffect.ErrorCode != "" {
		t.Fatalf("asserted terminal dispatch = %#v, %v", dispatchEffect, err)
	}
}

func TestStaleImplementationAttentionRetiresExactCycleForFreshScheduling(
	t *testing.T,
) {
	fixtureDriver := &turnRecoveryFixtureDriver{parkS1: true}
	fixture := newProductionImplementationRecoveryFixture(
		t,
		fixtureDriver,
	)
	if err := fixture.workspace.Close(); err != nil {
		t.Fatal(err)
	}
	if err := fixture.store.ReconcileOwned(
		fixture.ctx,
		fixture.owner,
		journal.Completion{
			RunID:     fixture.owner.RunID,
			EffectID:  fixture.outer.ID,
			Token:     fixture.outer.CurrentClaim,
			EventKind: "test_outer_not_started",
			At:        fixture.now,
		},
		journal.RecoveryAllOld,
	); err != nil {
		t.Fatal(err)
	}
	if err := fixture.service.advanceSlice(
		fixture.ctx,
		fixture.engine,
		fixture.owner,
		fixture.cycle.Slice,
	); !IsCode(err, "EFFECT_PARKED") {
		t.Fatalf("initial park = %v", err)
	}
	attentions, err := fixture.store.Attentions(
		fixture.ctx,
		fixture.owner.RunID,
	)
	if err != nil || len(attentions) != 1 ||
		attentions[0].State != journal.AttentionOpen {
		t.Fatalf("parked attention = %#v, %v", attentions, err)
	}

	oldTarget := fixture.state.Refs.Target.Head
	treeCommand := exec.Command(
		fixture.service.gitExecutable,
		"-C",
		fixture.repository,
		"rev-parse",
		oldTarget+"^{tree}",
	)
	treeBody, err := treeCommand.Output()
	if err != nil {
		t.Fatal(err)
	}
	tree := strings.TrimSpace(string(treeBody))
	commitCommand := exec.Command(
		fixture.service.gitExecutable,
		"-C",
		fixture.repository,
		"-c",
		"user.name=Sworn Test",
		"-c",
		"user.email=sworn@example.invalid",
		"commit-tree",
		tree,
		"-p",
		oldTarget,
	)
	commitCommand.Stdin = strings.NewReader("advance target authority\n")
	commitBody, err := commitCommand.Output()
	if err != nil {
		t.Fatal(err)
	}
	newTarget := strings.TrimSpace(string(commitBody))
	if output, err := exec.Command(
		fixture.service.gitExecutable,
		"-C",
		fixture.repository,
		"update-ref",
		fixture.manifest.value.TargetRef,
		newTarget,
		oldTarget,
	).CombinedOutput(); err != nil {
		t.Fatalf("advance target: %v: %s", err, output)
	}

	recovered, err := fixture.service.recoverImplementationClaims(
		fixture.ctx,
		fixture.engine,
		fixture.owner,
	)
	if err != nil || !recovered {
		t.Fatalf("stale implementation retirement = %t, %v", recovered, err)
	}
	currentAttention, err := fixture.store.Attention(
		fixture.ctx,
		fixture.owner.RunID,
		attentions[0].Attention.ID,
	)
	if err != nil ||
		currentAttention.State != journal.AttentionCancelled {
		t.Fatalf("retired implementation attention = %#v, %v",
			currentAttention, err)
	}
	for _, id := range []string{
		fixture.outer.ID,
		fixture.cycle.DispatchEffect,
	} {
		effect, err := fixture.store.Effect(
			fixture.ctx,
			fixture.owner.RunID,
			id,
		)
		if err != nil ||
			effect.State != journal.OperationalFailed ||
			effect.ErrorCode != "stale_authority" {
			t.Fatalf("retired implementation effect %s = %#v, %v",
				id, effect, err)
		}
	}
	recovered, err = fixture.service.recoverImplementationClaims(
		fixture.ctx,
		fixture.engine,
		fixture.owner,
	)
	if err != nil || recovered {
		t.Fatalf("fresh scheduling gate = %t, %v", recovered, err)
	}
}
