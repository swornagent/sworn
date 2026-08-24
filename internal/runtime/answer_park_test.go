package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/journal"
)

const answerParkFixtureQuestion = "Which exact approved value should I use?"

func recordManifestCommand(
	t *testing.T,
	fixture *productionImplementationRecoveryFixture,
) {
	t.Helper()
	if err := fixture.store.RecordCommand(
		fixture.ctx,
		journal.Command{
			RunID:     fixture.owner.RunID,
			ReplayKey: "manifest",
			Kind:      "start",
			Payload:   fixture.manifest.raw,
			CreatedAt: fixture.now,
		},
	); err != nil {
		t.Fatal(err)
	}
}

// answerParkCheckpointChildCount pins the single-checkpoint invariant: the
// dispatch effect ever carries exactly one runtime.answer_park child under
// the fixed ID.
func answerParkCheckpointChildCount(
	t *testing.T,
	fixture *productionImplementationRecoveryFixture,
	parentEffect string,
) int {
	t.Helper()
	snapshot, err := fixture.store.Snapshot(
		fixture.ctx,
		fixture.owner.RunID,
	)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, command := range snapshot.Commands {
		if command.Kind != "runtime.answer_park" {
			continue
		}
		count++
		if command.ReplayKey != answerParkCheckpointID(parentEffect) {
			t.Fatalf(
				"answer park checkpoint replay key=%q want %q",
				command.ReplayKey,
				answerParkCheckpointID(parentEffect),
			)
		}
	}
	return count
}

// TestAnswerableYieldParksFirstOccurrenceVerbatimWithoutConsumingTry pins
// A1 and A2: a first question/blocked yield opens one attention carrying the
// worker's words, no automation runs, no attempt row exists, the outer and
// dispatch effects stay claimed, and the fold is carried into the park
// step's accounting.
func TestAnswerableYieldParksFirstOccurrenceVerbatimWithoutConsumingTry(
	t *testing.T,
) {
	for _, kind := range []driver.YieldKind{
		driver.YieldQuestion,
		driver.YieldBlocked,
	} {
		t.Run(string(kind), func(t *testing.T) {
			fixtureDriver := &turnRecoveryFixtureDriver{
				parkS1:    true,
				yieldKind: kind,
				measured:  true,
			}
			fixture := newProductionImplementationRecoveryFixture(
				t,
				fixtureDriver,
			)
			defer fixture.workspace.Close()
			recordManifestCommand(t, fixture)
			if _, _, err := fixture.service.
				runProductionImplementationDispatch(
					fixture.ctx,
					fixture.engine,
					fixture.owner,
					fixture.workspace,
					fixture.cycle,
					fixture.coordinates,
				); !IsCode(err, "EFFECT_PARKED") {
				t.Fatalf("answerable park = %v", err)
			}

			attentions, err := fixture.store.Attentions(
				fixture.ctx,
				fixture.owner.RunID,
			)
			if err != nil || len(attentions) != 1 ||
				attentions[0].State != journal.AttentionOpen ||
				attentions[0].Generation != 1 ||
				attentions[0].Attention.HumanTurn != nil ||
				attentions[0].Question != answerParkFixtureQuestion {
				t.Fatalf("answerable attention = %#v, %v", attentions, err)
			}

			fixtureDriver.mu.Lock()
			automationCalls := fixtureDriver.automationCalls
			answerCalls := fixtureDriver.answerCalls
			fixtureDriver.mu.Unlock()
			if automationCalls != 0 || answerCalls != 0 {
				t.Fatalf(
					"first ask automation=%d worker resumes=%d",
					automationCalls,
					answerCalls,
				)
			}
			for _, effectID := range []string{
				fixture.outer.ID,
				fixture.cycle.DispatchEffect,
			} {
				effect, effectErr := fixture.store.Effect(
					fixture.ctx,
					fixture.owner.RunID,
					effectID,
				)
				if effectErr != nil ||
					effect.State != journal.Claimed {
					t.Fatalf(
						"parked effect %s = %#v, %v",
						effectID,
						effect,
						effectErr,
					)
				}
			}
			status, err := fixture.service.Status(
				fixture.ctx,
				fixture.owner.RunID,
			)
			if err != nil || status.State != "parked" {
				t.Fatalf("parked status = %#v, %v", status, err)
			}
			observation, err := fixture.store.ReadObservation(
				fixture.ctx,
				fixture.owner.RunID,
				journal.MaxObservationAttempts,
				journal.MaxObservationEvents,
			)
			if err != nil {
				t.Fatal(err)
			}
			for _, attempt := range observation.Attempts {
				if attempt.EffectID ==
					fixture.cycle.DispatchEffect {
					t.Fatalf(
						"park wrote an attempt row: %#v",
						attempt,
					)
				}
			}

			snapshot, err := fixture.store.Snapshot(
				fixture.ctx,
				fixture.owner.RunID,
			)
			if err != nil {
				t.Fatal(err)
			}
			checkpoint, found, err := answerParkCheckpointForEffect(
				fixture.manifest,
				snapshot,
				fixture.cycle.DispatchEffect,
			)
			if err != nil || !found {
				t.Fatalf(
					"answer park checkpoint found=%t error=%v",
					found,
					err,
				)
			}
			if checkpoint.Command.Attention.Attention.HumanTurn != nil ||
				checkpoint.Command.Resolve != nil ||
				checkpoint.Command.Attention.Question !=
					answerParkFixtureQuestion {
				t.Fatalf("answer park checkpoint = %#v", checkpoint)
			}
			if answerParkCheckpointChildCount(
				t,
				fixture,
				fixture.cycle.DispatchEffect,
			) != 1 {
				t.Fatal("answer park checkpoint child count != 1")
			}

			// The carried fold must land in the park step's accounting so
			// the yield turn's spend survives the answer.
			_, cycle := preparedTurnRecoveryFixture(t, fixture)
			budget, err := fixture.store.RecoveryBudget(
				fixture.ctx,
				fixture.owner.RunID,
				cycle.binding,
			)
			if err != nil {
				t.Fatal(err)
			}
			if budget.Accounting == nil ||
				budget.Accounting.InputTokens == nil ||
				*budget.Accounting.InputTokens != 7 ||
				budget.Accounting.OutputTokens == nil ||
				*budget.Accounting.OutputTokens != 11 {
				t.Fatalf(
					"park step accounting = %#v",
					budget.Accounting,
				)
			}
		})
	}
}

// TestAnswerParkAnswerResumesInPlaceAndCompletes pins A3's in-process half:
// the answered attention resumes the retained conversation with the answer
// in place, the attention resolves, one recovered event is durable, the
// dispatch terminalizes, and the single answer-park checkpoint child is the
// only checkpoint ever created for the dispatch effect.
func TestAnswerParkAnswerResumesInPlaceAndCompletes(t *testing.T) {
	fixtureDriver := &turnRecoveryFixtureDriver{
		parkS1:         true,
		yieldKind:      driver.YieldQuestion,
		measured:       true,
		expectedAnswer: "Use the exact approved fixture value.",
	}
	fixture := newProductionImplementationRecoveryFixture(
		t,
		fixtureDriver,
	)
	defer fixture.workspace.Close()
	recordManifestCommand(t, fixture)
	if _, _, err := fixture.service.
		runProductionImplementationDispatch(
			fixture.ctx,
			fixture.engine,
			fixture.owner,
			fixture.workspace,
			fixture.cycle,
			fixture.coordinates,
		); !IsCode(err, "EFFECT_PARKED") {
		t.Fatalf("answerable park = %v", err)
	}
	attentions, err := fixture.store.Attentions(
		fixture.ctx,
		fixture.owner.RunID,
	)
	if err != nil || len(attentions) != 1 ||
		attentions[0].State != journal.AttentionOpen ||
		attentions[0].Question != answerParkFixtureQuestion {
		t.Fatalf("answerable attention = %#v, %v", attentions, err)
	}
	attention := attentions[0]
	if _, err := fixture.service.AnswerAttention(
		fixture.ctx,
		AnswerAttentionCommand{
			RunID:              fixture.owner.RunID,
			AttentionID:        attention.Attention.ID,
			ExpectedGeneration: attention.Generation,
			Answer:             "Use the exact approved fixture value.",
		},
	); err != nil {
		t.Fatalf("answerable answer = %v", err)
	}
	if _, _, err := fixture.service.
		runProductionImplementationDispatch(
			fixture.ctx,
			fixture.engine,
			fixture.owner,
			fixture.workspace,
			fixture.cycle,
			fixture.coordinates,
		); err != nil {
		t.Fatalf("answer resume = %v", err)
	}
	fixtureDriver.mu.Lock()
	automationCalls := fixtureDriver.automationCalls
	answerCalls := fixtureDriver.answerCalls
	fixtureDriver.mu.Unlock()
	if automationCalls != 0 || answerCalls != 1 {
		t.Fatalf(
			"answerable resume automation=%d worker resumes=%d",
			automationCalls,
			answerCalls,
		)
	}
	resolved, err := fixture.store.Attention(
		fixture.ctx,
		fixture.owner.RunID,
		attention.Attention.ID,
	)
	if err != nil || resolved.State != journal.AttentionResolved {
		t.Fatalf("resolved attention = %#v, %v", resolved, err)
	}
	effect, err := fixture.store.Effect(
		fixture.ctx,
		fixture.owner.RunID,
		fixture.cycle.DispatchEffect,
	)
	if err != nil || effect.State != journal.Succeeded {
		t.Fatalf("terminal dispatch = %#v, %v", effect, err)
	}
	if answerParkCheckpointChildCount(
		t,
		fixture,
		fixture.cycle.DispatchEffect,
	) != 1 {
		t.Fatal("answer park checkpoint child count != 1")
	}
	snapshot, err := fixture.store.Snapshot(
		fixture.ctx,
		fixture.owner.RunID,
	)
	if err != nil {
		t.Fatal(err)
	}
	recoveredEvents := 0
	for _, event := range snapshot.Events {
		if event.Kind == turnRecoveryRecoveredEvent {
			recoveredEvents++
		}
	}
	if recoveredEvents != 1 {
		t.Fatalf("durable recovered events = %d, want 1", recoveredEvents)
	}
}

// TestAnswerParkSingleCheckpointAndReplayGuard pins correction 3 end to
// end: the full first-park / answer / successor-question / automation-park /
// answer / completion sequence keeps exactly one runtime.answer_park
// checkpoint child for the dispatch effect, the successor park never opens
// a second answerable park, and the fixed-ID replay guard refuses a second
// differing checkpoint body under the same parent.
func TestAnswerParkSingleCheckpointAndReplayGuard(t *testing.T) {
	fixtureDriver := &turnRecoveryFixtureDriver{
		parkS1: true,
	}
	fixture := newProductionImplementationRecoveryFixture(
		t,
		fixtureDriver,
	)
	defer fixture.workspace.Close()
	if _, _, err := fixture.service.
		runProductionImplementationDispatch(
			fixture.ctx,
			fixture.engine,
			fixture.owner,
			fixture.workspace,
			fixture.cycle,
			fixture.coordinates,
		); !IsCode(err, "EFFECT_PARKED") {
		t.Fatalf("answerable park = %v", err)
	}
	if answerParkCheckpointChildCount(
		t,
		fixture,
		fixture.cycle.DispatchEffect,
	) != 1 {
		t.Fatal("first park checkpoint child count != 1")
	}
	snapshot, err := fixture.store.Snapshot(
		fixture.ctx,
		fixture.owner.RunID,
	)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, found, err := answerParkCheckpointForEffect(
		fixture.manifest,
		snapshot,
		fixture.cycle.DispatchEffect,
	)
	if err != nil || !found {
		t.Fatalf("answer park checkpoint found=%t error=%v", found, err)
	}

	// Exact replay of the same checkpoint body is idempotent.
	if err := fixture.service.persistAnswerParkCheckpoint(
		fixture.ctx,
		fixture.owner,
		fixture.cycle.DispatchEffect,
		checkpoint.Command,
	); err != nil {
		t.Fatalf("checkpoint replay = %v", err)
	}
	// A second differing park under the same fixed ID is corruption.
	differing := checkpoint.Command
	differing.Attention.Question = "A different question body."
	if err := fixture.service.persistAnswerParkCheckpoint(
		fixture.ctx,
		fixture.owner,
		fixture.cycle.DispatchEffect,
		differing,
	); !IsCode(err, "CORRUPT_JOURNAL") {
		t.Fatalf("differing replay = %v", err)
	}

	attentions, err := fixture.store.Attentions(
		fixture.ctx,
		fixture.owner.RunID,
	)
	if err != nil || len(attentions) != 1 {
		t.Fatalf("first attention = %#v, %v", attentions, err)
	}
	first := attentions[0]
	if _, err := fixture.store.AnswerAttention(
		fixture.ctx,
		journal.AnswerAttentionCommand{
			RunID:              fixture.owner.RunID,
			Attention:          first.Attention,
			ExpectedGeneration: first.Generation,
			Answer:             "Use the exact approved fixture value.",
		},
		fixture.now,
	); err != nil {
		t.Fatal(err)
	}
	fixtureDriver.mu.Lock()
	fixtureDriver.successor =
		"Which approved follow-up value should I use?"
	fixtureDriver.mu.Unlock()
	if _, _, err := fixture.service.
		runProductionImplementationDispatch(
			fixture.ctx,
			fixture.engine,
			fixture.owner,
			fixture.workspace,
			fixture.cycle,
			fixture.coordinates,
		); !IsCode(err, "EFFECT_PARKED") {
		t.Fatalf("successor park = %v", err)
	}
	fixtureDriver.mu.Lock()
	automationCalls := fixtureDriver.automationCalls
	fixtureDriver.mu.Unlock()
	if automationCalls != 1 {
		t.Fatalf(
			"successor automation calls = %d, want 1",
			automationCalls,
		)
	}
	if answerParkCheckpointChildCount(
		t,
		fixture,
		fixture.cycle.DispatchEffect,
	) != 1 {
		t.Fatal("successor park checkpoint child count != 1")
	}
	attentions, err = fixture.store.Attentions(
		fixture.ctx,
		fixture.owner.RunID,
	)
	if err != nil || len(attentions) != 2 {
		t.Fatalf("successor attentions = %#v, %v", attentions, err)
	}
	var successor journal.AttentionProjection
	for _, attention := range attentions {
		if attention.Attention.ID == first.Attention.ID {
			if attention.State != journal.AttentionResolved {
				t.Fatalf(
					"first attention not resolved: %#v",
					attention,
				)
			}
			continue
		}
		if attention.State != journal.AttentionOpen {
			t.Fatalf("successor attention = %#v", attention)
		}
		successor = attention
	}
	snapshot, err = fixture.store.Snapshot(
		fixture.ctx,
		fixture.owner.RunID,
	)
	if err != nil {
		t.Fatal(err)
	}
	// The successor park is a plain park: no answer-park checkpoint claims
	// its binding, so the ask bound stays one answerable park per cycle.
	if _, found, err := answerParkCheckpointForAttention(
		fixture.manifest,
		snapshot,
		successor.Attention,
	); err != nil || found {
		t.Fatalf(
			"successor checkpoint claim found=%t error=%v",
			found,
			err,
		)
	}
	if _, err := fixture.store.AnswerAttention(
		fixture.ctx,
		journal.AnswerAttentionCommand{
			RunID:              fixture.owner.RunID,
			Attention:          successor.Attention,
			ExpectedGeneration: successor.Generation,
			Answer:             "Use the exact approved fixture value.",
		},
		fixture.now,
	); err != nil {
		t.Fatal(err)
	}
	if _, _, err := fixture.service.
		runProductionImplementationDispatch(
			fixture.ctx,
			fixture.engine,
			fixture.owner,
			fixture.workspace,
			fixture.cycle,
			fixture.coordinates,
		); err != nil {
		t.Fatalf("successor completion = %v", err)
	}
	effect, err := fixture.store.Effect(
		fixture.ctx,
		fixture.owner.RunID,
		fixture.cycle.DispatchEffect,
	)
	if err != nil || effect.State != journal.Succeeded {
		t.Fatalf("terminal dispatch = %#v, %v", effect, err)
	}
	if answerParkCheckpointChildCount(
		t,
		fixture,
		fixture.cycle.DispatchEffect,
	) != 1 {
		t.Fatal("final checkpoint child count != 1")
	}
}

// TestAnswerParkCheckpointRecoversAcrossHostDeath pins A3's durability half:
// the checkpoint commits before the attention commit, a fresh Service over
// the same store re-opens the exact attention through
// recoverImplementationClaims, and the answer completes the work.
func TestAnswerParkCheckpointRecoversAcrossHostDeath(t *testing.T) {
	testAnswerParkCrashCut = "before_answer_park_commit"
	defer func() { testAnswerParkCrashCut = "" }()
	fixtureDriver := &turnRecoveryFixtureDriver{
		parkS1:   true,
		measured: true,
	}
	fixture := newProductionImplementationRecoveryFixture(
		t,
		fixtureDriver,
	)
	recordManifestCommand(t, fixture)
	if _, _, err := fixture.service.
		runProductionImplementationDispatch(
			fixture.ctx,
			fixture.engine,
			fixture.owner,
			fixture.workspace,
			fixture.cycle,
			fixture.coordinates,
		); !IsCode(err, "EFFECT_PARKED") {
		t.Fatalf("crash cut = %v", err)
	}
	attentions, err := fixture.store.Attentions(
		fixture.ctx,
		fixture.owner.RunID,
	)
	if err != nil || len(attentions) != 0 {
		t.Fatalf("pre-commit attentions = %#v, %v", attentions, err)
	}
	if answerParkCheckpointChildCount(
		t,
		fixture,
		fixture.cycle.DispatchEffect,
	) != 1 {
		t.Fatal("crash cut checkpoint child count != 1")
	}
	dispatch, err := fixture.store.Effect(
		fixture.ctx,
		fixture.owner.RunID,
		fixture.cycle.DispatchEffect,
	)
	if err != nil || dispatch.State != journal.Claimed {
		t.Fatalf("crash cut dispatch = %#v, %v", dispatch, err)
	}

	if err := fixture.workspace.Close(); err != nil {
		t.Fatal(err)
	}
	if err := fixture.engine.Close(); err != nil {
		t.Fatal(err)
	}
	if err := fixture.store.ReleaseOwner(
		fixture.ctx,
		fixture.owner,
		fixture.now,
	); err != nil {
		t.Fatal(err)
	}
	driverAfterRestart := &turnRecoveryFixtureDriver{measured: true}
	restartedProduction, err := newProductionDriverRuntime(
		fixture.config,
		driver.DriverFactoryOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	restarted := &Service{
		journal:       fixture.store,
		dispatcher:    driverAfterRestart,
		production:    restartedProduction,
		gitExecutable: fixture.service.gitExecutable,
		now:           fixture.service.now,
	}
	restartedOwner, err := fixture.store.AcquireOwner(
		fixture.ctx,
		fixture.owner.RunID,
		fixture.now,
		time.Minute,
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.store.ReleaseOwner(
		context.Background(),
		restartedOwner,
		fixture.now,
	)
	restartedManifest, _, err := restarted.loadRun(
		fixture.ctx,
		fixture.owner.RunID,
	)
	if err != nil {
		t.Fatal(err)
	}
	restartedEngine, err := restarted.openEngine(restartedManifest)
	if err != nil {
		t.Fatal(err)
	}
	defer restartedEngine.Close()
	recovered, err := restarted.recoverImplementationClaims(
		fixture.ctx,
		restartedEngine,
		restartedOwner,
	)
	if err != nil || !recovered {
		t.Fatalf("answer park recovery = %t, %v", recovered, err)
	}
	attentions, err = fixture.store.Attentions(
		fixture.ctx,
		fixture.owner.RunID,
	)
	if err != nil || len(attentions) != 1 ||
		attentions[0].State != journal.AttentionOpen ||
		attentions[0].Generation != 1 ||
		attentions[0].Question != answerParkFixtureQuestion ||
		attentions[0].Attention.HumanTurn != nil {
		t.Fatalf("re-opened attention = %#v, %v", attentions, err)
	}
	attention := attentions[0]
	if _, err := fixture.store.AnswerAttention(
		fixture.ctx,
		journal.AnswerAttentionCommand{
			RunID:              fixture.owner.RunID,
			Attention:          attention.Attention,
			ExpectedGeneration: attention.Generation,
			Answer:             "Use the exact approved fixture value.",
		},
		fixture.now,
	); err != nil {
		t.Fatal(err)
	}
	recovered, err = restarted.recoverImplementationClaims(
		fixture.ctx,
		restartedEngine,
		restartedOwner,
	)
	if err != nil || !recovered {
		t.Fatalf("answer park completion = %t, %v", recovered, err)
	}
	recoveredState, err := baton.ReadState(
		restartedEngine.git,
		fixture.manifest.value.Release,
		restartedEngine.inertness,
	)
	if err != nil {
		t.Fatal(err)
	}
	recoveredS1, found := recoveredState.Slice("S1")
	if !found || recoveredS1.Stage != "verify" {
		t.Fatalf("restarted answer park recovery = %#v", recoveredS1)
	}
	effect, err := fixture.store.Effect(
		fixture.ctx,
		fixture.owner.RunID,
		fixture.cycle.DispatchEffect,
	)
	if err != nil || effect.State != journal.Succeeded {
		t.Fatalf("terminal dispatch = %#v, %v", effect, err)
	}
	if answerParkCheckpointChildCount(
		t,
		fixture,
		fixture.cycle.DispatchEffect,
	) != 1 {
		t.Fatal("final checkpoint child count != 1")
	}
}

// TestAnswerParkSupersessionSkipAndFailClosed pins correction 2: a
// checkpoint whose own attention was resolved while a successor attention
// owns the work is skipped as legitimate advancement, while a checkpoint
// whose attention is absent entirely with a different attention active for
// the same work fails closed.
func TestAnswerParkSupersessionSkipAndFailClosed(t *testing.T) {
	t.Run("resolved predecessor skips", func(t *testing.T) {
		fixtureDriver := &turnRecoveryFixtureDriver{parkS1: true}
		fixture := newProductionImplementationRecoveryFixture(
			t,
			fixtureDriver,
		)
		defer fixture.workspace.Close()
		if _, _, err := fixture.service.
			runProductionImplementationDispatch(
				fixture.ctx,
				fixture.engine,
				fixture.owner,
				fixture.workspace,
				fixture.cycle,
				fixture.coordinates,
			); !IsCode(err, "EFFECT_PARKED") {
			t.Fatalf("answerable park = %v", err)
		}
		attentions, err := fixture.store.Attentions(
			fixture.ctx,
			fixture.owner.RunID,
		)
		if err != nil || len(attentions) != 1 {
			t.Fatalf("first attention = %#v, %v", attentions, err)
		}
		if _, err := fixture.store.AnswerAttention(
			fixture.ctx,
			journal.AnswerAttentionCommand{
				RunID:              fixture.owner.RunID,
				Attention:          attentions[0].Attention,
				ExpectedGeneration: attentions[0].Generation,
				Answer:             "Use the exact approved fixture value.",
			},
			fixture.now,
		); err != nil {
			t.Fatal(err)
		}
		fixtureDriver.mu.Lock()
		fixtureDriver.successor =
			"Which approved follow-up value should I use?"
		fixtureDriver.mu.Unlock()
		if _, _, err := fixture.service.
			runProductionImplementationDispatch(
				fixture.ctx,
				fixture.engine,
				fixture.owner,
				fixture.workspace,
				fixture.cycle,
				fixture.coordinates,
			); !IsCode(err, "EFFECT_PARKED") {
			t.Fatalf("successor park = %v", err)
		}
		snapshot, err := fixture.store.Snapshot(
			fixture.ctx,
			fixture.owner.RunID,
		)
		if err != nil {
			t.Fatal(err)
		}
		recovered, err := fixture.service.recoverAnswerParkCheckpoint(
			fixture.ctx,
			fixture.engine,
			fixture.owner,
			snapshot,
		)
		if err != nil || recovered {
			t.Fatalf(
				"superseded checkpoint skip = %t, %v",
				recovered,
				err,
			)
		}
	})

	t.Run("absent attention with different active fails closed", func(t *testing.T) {
		testAnswerParkCrashCut = "before_answer_park_commit"
		defer func() { testAnswerParkCrashCut = "" }()
		fixtureDriver := &turnRecoveryFixtureDriver{parkS1: true}
		fixture := newProductionImplementationRecoveryFixture(
			t,
			fixtureDriver,
		)
		defer fixture.workspace.Close()
		if _, _, err := fixture.service.
			runProductionImplementationDispatch(
				fixture.ctx,
				fixture.engine,
				fixture.owner,
				fixture.workspace,
				fixture.cycle,
				fixture.coordinates,
			); !IsCode(err, "EFFECT_PARKED") {
			t.Fatalf("crash cut = %v", err)
		}
		_, cycle := preparedTurnRecoveryFixture(t, fixture)
		prior := journal.AttentionBinding{
			Ordinal:  99,
			Recovery: cycle.binding,
		}
		prior.ID = journal.AttentionID(
			prior.Recovery,
			prior.Ordinal,
		)
		if _, err := fixture.store.OpenAttention(
			fixture.ctx,
			fixture.owner,
			journal.OpenAttentionCommand{
				RunID:              fixture.owner.RunID,
				Attention:          prior,
				ExpectedGeneration: 0,
				Question:           "Unrelated recovery question.",
			},
			fixture.now,
		); err != nil {
			t.Fatal(err)
		}
		snapshot, err := fixture.store.Snapshot(
			fixture.ctx,
			fixture.owner.RunID,
		)
		if err != nil {
			t.Fatal(err)
		}
		_, err = fixture.service.recoverAnswerParkCheckpoint(
			fixture.ctx,
			fixture.engine,
			fixture.owner,
			snapshot,
		)
		if !IsCode(err, "CORRUPT_JOURNAL") {
			t.Fatalf("absent attention recovery = %v", err)
		}
	})
}

// TestAnswerParkAdmissionBindsToExactCheckpointAndLeavesLegacyPlainParks
// pins the admission seams: an answer-park checkpoint claiming a binding
// fails closed on any mismatch, while bindings no checkpoint claims keep
// today's legacy plain-park semantics.
func TestAnswerParkAdmissionBindsToExactCheckpointAndLeavesLegacyPlainParks(
	t *testing.T,
) {
	fixtureDriver := &turnRecoveryFixtureDriver{parkS1: true}
	fixture := newProductionImplementationRecoveryFixture(
		t,
		fixtureDriver,
	)
	defer fixture.workspace.Close()
	if _, _, err := fixture.service.
		runProductionImplementationDispatch(
			fixture.ctx,
			fixture.engine,
			fixture.owner,
			fixture.workspace,
			fixture.cycle,
			fixture.coordinates,
		); !IsCode(err, "EFFECT_PARKED") {
		t.Fatalf("answerable park = %v", err)
	}
	attentions, err := fixture.store.Attentions(
		fixture.ctx,
		fixture.owner.RunID,
	)
	if err != nil || len(attentions) != 1 {
		t.Fatalf("first attention = %#v, %v", attentions, err)
	}
	attention := attentions[0]
	snapshot, err := fixture.store.Snapshot(
		fixture.ctx,
		fixture.owner.RunID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if validateAnswerParkAnswerAdmission(
		snapshot,
		fixture.manifest,
		attention,
		"Use the exact approved fixture value.",
	) != nil {
		t.Fatal("exact answerable admission refused")
	}
	mutated := attention
	mutated.Attention.Recovery.ProgressID =
		driver.Digest([]byte("wrong-work"))
	if validateAnswerParkAnswerAdmission(
		snapshot,
		fixture.manifest,
		mutated,
		"Use the exact approved fixture value.",
	) == nil {
		t.Fatal("mutated binding admitted")
	}
	answered := attention
	answered.State = journal.AttentionAnswered
	answered.Generation = 2
	answered.Answer = "Use the exact approved fixture value."
	if err := fixture.service.validateAnswerParkResume(
		fixture.ctx,
		fixture.manifest,
		fixture.cycle.DispatchEffect,
		answered,
	); err != nil {
		t.Fatalf("exact answerable resume refused: %v", err)
	}
	if err := fixture.service.validateAnswerParkResume(
		fixture.ctx,
		fixture.manifest,
		"wrong-effect-id",
		answered,
	); !IsCode(err, "INVALID_ANSWER_PARK") {
		t.Fatalf("wrong parent resume = %v", err)
	}
	open := answered
	open.State = journal.AttentionOpen
	open.Generation = 1
	if err := fixture.service.validateAnswerParkResume(
		fixture.ctx,
		fixture.manifest,
		fixture.cycle.DispatchEffect,
		open,
	); !IsCode(err, "INVALID_ANSWER_PARK") {
		t.Fatalf("unanswered resume = %v", err)
	}

	// A plain attention with no checkpoint keeps legacy semantics.
	_, cycle := preparedTurnRecoveryFixture(t, fixture)
	plain := journal.AttentionBinding{
		Ordinal:  99,
		Recovery: cycle.binding,
	}
	plain.ID = journal.AttentionID(plain.Recovery, plain.Ordinal)
	opened, err := fixture.store.OpenAttention(
		fixture.ctx,
		fixture.owner,
		journal.OpenAttentionCommand{
			RunID:              fixture.owner.RunID,
			Attention:          plain,
			ExpectedGeneration: 0,
			Question:           "Legacy recovery question.",
		},
		fixture.now,
	)
	if err != nil {
		t.Fatal(err)
	}
	plainOpen := journal.AttentionProjection{
		Attention:  plain,
		Generation: opened.Generation,
		State:      journal.AttentionOpen,
		Question:   "Legacy recovery question.",
	}
	snapshot, err = fixture.store.Snapshot(
		fixture.ctx,
		fixture.owner.RunID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if validateAnswerParkAnswerAdmission(
		snapshot,
		fixture.manifest,
		plainOpen,
		"Any answer.",
	) != nil {
		t.Fatal("legacy plain admission refused")
	}
	plainAnswered := plainOpen
	plainAnswered.State = journal.AttentionAnswered
	plainAnswered.Generation = opened.Generation + 1
	plainAnswered.Answer = "Any answer."
	if err := fixture.service.validateAnswerParkResume(
		fixture.ctx,
		fixture.manifest,
		fixture.cycle.DispatchEffect,
		plainAnswered,
	); err != nil {
		t.Fatalf("legacy plain resume refused: %v", err)
	}
}
