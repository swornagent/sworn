//go:build linux

package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/engine"
)

func controlledBuildOutcomeSelector(fixture *controlledBuildStoreFixture) ControlledBuildDispatchSelector {
	return ControlledBuildDispatchSelector{
		ControllerID: fixture.ownerID, CommandID: fixture.command.ID,
		RunID: fixture.request.RunID, WorkID: fixture.request.WorkID,
		BuilderDispatchDigest: fixture.builderDigest,
	}
}

func TestControlledBuildDispatchConvergenceDistinguishesAbsenceFromExactReplay(t *testing.T) {
	fixture := newControlledBuildStoreFixture(t, nil)
	ctx := context.Background()
	selector := controlledBuildOutcomeSelector(fixture)
	before := []int{
		tableCount(t, fixture.control, "commands"),
		tableCount(t, fixture.control, "events"),
		tableCount(t, fixture.control, "effects"),
	}
	result, found, err := fixture.control.ConvergeControlledBuildDispatch(
		ctx, fixture.ownership, selector,
	)
	if err != nil || found || !reflect.DeepEqual(result, ApplyResult{}) {
		t.Fatalf("absent controlled dispatch = %+v, found=%t, error=%v", result, found, err)
	}
	if got := []int{
		tableCount(t, fixture.control, "commands"),
		tableCount(t, fixture.control, "events"),
		tableCount(t, fixture.control, "effects"),
	}; !reflect.DeepEqual(got, before) {
		t.Fatalf("absent convergence mutated Store: got %v, want %v", got, before)
	}

	want := fixture.dispatch(t)
	afterDispatch := []int{
		tableCount(t, fixture.control, "commands"),
		tableCount(t, fixture.control, "events"),
		tableCount(t, fixture.control, "effects"),
	}
	result, found, err = fixture.control.ConvergeControlledBuildDispatch(
		ctx, fixture.ownership, selector,
	)
	if err != nil || !found || !result.Replayed {
		t.Fatalf("durable controlled dispatch = %+v, found=%t, error=%v", result, found, err)
	}
	result.Replayed = false
	if !reflect.DeepEqual(result, want) {
		t.Fatalf("replayed controlled dispatch = %+v, want %+v", result, want)
	}
	if got := []int{
		tableCount(t, fixture.control, "commands"),
		tableCount(t, fixture.control, "events"),
		tableCount(t, fixture.control, "effects"),
	}; !reflect.DeepEqual(got, afterDispatch) {
		t.Fatalf("durable convergence mutated Store: got %v, want %v", got, afterDispatch)
	}
}

func TestControlledBuildDispatchConvergenceSurvivesStoreRestart(t *testing.T) {
	fixture := newControlledBuildStoreFixture(t, nil)
	want := fixture.dispatch(t)
	databasePath := fixture.control.ControlPath()
	repository := fixture.control.repository
	if err := fixture.ownership.Close(); err != nil {
		t.Fatal(err)
	}
	fixture.ownership = nil
	if err := fixture.control.Close(); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	reopened, err := OpenConfigured(ctx, databasePath, ControlConfiguration{
		BuilderDispatchDigest: fixture.builderDigest,
		Repository:            repository,
	})
	if err != nil {
		t.Fatal(err)
	}
	successor, err := reopened.AcquireControllerOwnership("controller-successor")
	if err != nil {
		_ = reopened.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = successor.Close()
		_ = reopened.Close()
	})
	if err := successor.Activate(ctx, reopened, "controller-successor"); err != nil {
		t.Fatal(err)
	}
	selector := controlledBuildOutcomeSelector(fixture)
	selector.ControllerID = "controller-successor"
	result, found, err := reopened.ConvergeControlledBuildDispatch(ctx, successor, selector)
	if err != nil || !found || !result.Replayed {
		t.Fatalf("restarted controlled dispatch = %+v, found=%t, error=%v", result, found, err)
	}
	result.Replayed = false
	if !reflect.DeepEqual(result, want) {
		t.Fatalf("restarted replay = %+v, want %+v", result, want)
	}
}

func TestControlledBuildDispatchConvergenceSurvivesRunningEffect(t *testing.T) {
	fixture := newControlledBuildStoreFixture(t, nil)
	want := fixture.dispatch(t)
	request, permit := fixture.executionPermit(t)
	lease, err := fixture.control.ClaimControlledBuild(
		context.Background(), fixture.ownership, fixture.authority, permit, request,
	)
	if err != nil {
		t.Fatal(err)
	}
	if lease.effect.State != EffectRunning {
		t.Fatalf("claimed effect state = %q, want running", lease.effect.State)
	}

	result, found, err := fixture.control.ConvergeControlledBuildDispatch(
		context.Background(), fixture.ownership, controlledBuildOutcomeSelector(fixture),
	)
	if err != nil || !found || !result.Replayed {
		t.Fatalf("running-effect controlled dispatch = %+v, found=%t, error=%v", result, found, err)
	}
	result.Replayed = false
	if !reflect.DeepEqual(result, want) {
		t.Fatalf("running-effect replay = %+v, want %+v", result, want)
	}
}

func TestControlledBuildDispatchConvergenceIsHistoricalNotAuthority(t *testing.T) {
	fixture := newControlledBuildStoreFixture(t, nil)
	want := fixture.dispatch(t)
	executionRequest, executionPermit := fixture.executionPermit(t)
	advanceAuthorityToRevoked(t, fixture)

	result, found, err := fixture.control.ConvergeControlledBuildDispatch(
		context.Background(), fixture.ownership, controlledBuildOutcomeSelector(fixture),
	)
	if err != nil || !found || !result.Replayed || result.CommandID != want.CommandID {
		t.Fatalf("revoked historical dispatch = %+v, found=%t, error=%v", result, found, err)
	}
	if _, err := fixture.control.ClaimControlledBuild(
		context.Background(), fixture.ownership, fixture.authority,
		executionPermit, executionRequest,
	); err == nil || !strings.Contains(err.Error(), "superseded") {
		t.Fatalf("revoked authority claimed recovered dispatch: %v", err)
	}
}

func TestControlledBuildDispatchConvergenceBindsTheCurrentIntendedAttempt(t *testing.T) {
	t.Run("same attempt later lifecycle", func(t *testing.T) {
		fixture := newControlledBuildStoreFixture(t, nil)
		want := fixture.dispatch(t)
		advanceControlledOutcomeState(t, fixture, engine.WorkChecking, engine.ActionWait)

		result, found, err := fixture.control.ConvergeControlledBuildDispatch(
			context.Background(), fixture.ownership, controlledBuildOutcomeSelector(fixture),
		)
		if err != nil || !found || !result.Replayed {
			t.Fatalf("later lifecycle convergence = %+v, found=%t, error=%v", result, found, err)
		}
		result.Replayed = false
		if !reflect.DeepEqual(result, want) {
			t.Fatalf("later lifecycle replay = %+v, want %+v", result, want)
		}
	})

	t.Run("ready next attempt", func(t *testing.T) {
		fixture := newControlledBuildStoreFixture(t, nil)
		fixture.dispatch(t)
		advanceControlledOutcomeState(t, fixture, engine.WorkReady, engine.ActionBuild)

		if _, found, err := fixture.control.ConvergeControlledBuildDispatch(
			context.Background(), fixture.ownership, controlledBuildOutcomeSelector(fixture),
		); !errors.Is(err, ErrIdempotencyConflict) || found {
			t.Fatalf("stale attempt convergence = found=%t, error=%v", found, err)
		}
	})
}

func TestControlledBuildDispatchConvergenceRejectsUnsupportedDurableOutcome(t *testing.T) {
	fixture := newControlledBuildStoreFixture(t, nil)
	command := fixture.command
	command.ID = "cmd-rejected-dispatch"
	requestJSON, err := json.Marshal(command)
	if err != nil {
		t.Fatal(err)
	}
	rejected := ApplyResult{
		CommandID: command.ID, RunID: command.RunID, Outcome: OutcomeRejected,
		Revision: command.ExpectedRevision, ErrorCode: "controlled_test",
		ErrorMessage: "unsupported controlled rejection",
	}
	transaction, err := fixture.control.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := insertCommand(
		context.Background(), transaction, command, requestJSON,
		commandDigest(command), rejected, fixture.control.now().UTC().UnixMicro(),
	); err != nil {
		_ = transaction.Rollback()
		t.Fatal(err)
	}
	if err := transaction.Commit(); err != nil {
		t.Fatal(err)
	}
	selector := controlledBuildOutcomeSelector(fixture)
	selector.CommandID = command.ID
	if _, found, err := fixture.control.ConvergeControlledBuildDispatch(
		context.Background(), fixture.ownership, selector,
	); err == nil || found {
		t.Fatalf("unsupported controlled outcome = found=%t, error=%v", found, err)
	}
}

func advanceControlledOutcomeState(
	t *testing.T,
	fixture *controlledBuildStoreFixture,
	workState engine.WorkState,
	nextAction engine.NextAction,
) {
	t.Helper()
	state, err := fixture.control.State(context.Background(), fixture.request.RunID)
	if err != nil {
		t.Fatal(err)
	}
	state.Revision++
	state.Work[0].State = workState
	state.Work[0].NextAction = nextAction
	if err := state.Validate(); err != nil {
		t.Fatalf("validate advanced controlled outcome state: %v", err)
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	execControlledOutcomeSQL(t, fixture.control, `
		UPDATE runs SET revision = ?, state_json = ?, updated_at_us = updated_at_us + 1
		WHERE run_id = ?`, state.Revision, encoded, state.RunID,
	)
}

func TestControlledBuildDispatchConvergenceRejectsOwnershipAndSelectorDrift(t *testing.T) {
	ctx := context.Background()

	t.Run("nil ownership", func(t *testing.T) {
		fixture := newControlledBuildStoreFixture(t, nil)
		fixture.dispatch(t)
		if _, found, err := fixture.control.ConvergeControlledBuildDispatch(
			ctx, nil, controlledBuildOutcomeSelector(fixture),
		); !errors.Is(err, ErrInvalidControllerOwnership) || found {
			t.Fatalf("nil ownership convergence = found=%t, error=%v", found, err)
		}
	})

	t.Run("released ownership", func(t *testing.T) {
		fixture := newControlledBuildStoreFixture(t, nil)
		fixture.dispatch(t)
		if err := fixture.ownership.Close(); err != nil {
			t.Fatal(err)
		}
		if _, found, err := fixture.control.ConvergeControlledBuildDispatch(
			ctx, fixture.ownership, controlledBuildOutcomeSelector(fixture),
		); !errors.Is(err, ErrInvalidControllerOwnership) || found {
			t.Fatalf("released ownership convergence = found=%t, error=%v", found, err)
		}
	})

	t.Run("recovery ownership", func(t *testing.T) {
		fixture := newControlledBuildStoreFixture(t, nil)
		fixture.dispatch(t)
		if err := fixture.ownership.Close(); err != nil {
			t.Fatal(err)
		}
		recovery, err := fixture.control.AcquireControllerOwnership("controller-recovery")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = recovery.Close() })
		selector := controlledBuildOutcomeSelector(fixture)
		selector.ControllerID = "controller-recovery"
		if _, found, err := fixture.control.ConvergeControlledBuildDispatch(
			ctx, recovery, selector,
		); !errors.Is(err, ErrInvalidControllerOwnership) || found {
			t.Fatalf("recovery ownership convergence = found=%t, error=%v", found, err)
		}
	})

	t.Run("foreign ownership", func(t *testing.T) {
		fixture := newControlledBuildStoreFixture(t, nil)
		fixture.dispatch(t)
		foreign := openTestStore(t, filepath.Join(t.TempDir(), "foreign.db"))
		t.Cleanup(func() { _ = foreign.Close() })
		if err := os.Chmod(filepath.Dir(foreign.ControlPath()), 0o700); err != nil {
			t.Fatal(err)
		}
		foreignOwnership, err := foreign.AcquireControllerOwnership("controller-foreign")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = foreignOwnership.Close() })
		if err := foreignOwnership.Activate(ctx, foreign, "controller-foreign"); err != nil {
			t.Fatal(err)
		}
		selector := controlledBuildOutcomeSelector(fixture)
		selector.ControllerID = "controller-foreign"
		if _, found, err := fixture.control.ConvergeControlledBuildDispatch(
			ctx, foreignOwnership, selector,
		); !errors.Is(err, ErrInvalidControllerOwnership) || found {
			t.Fatalf("foreign ownership convergence = found=%t, error=%v", found, err)
		}
	})

	t.Run("occupied command collision", func(t *testing.T) {
		fixture := newControlledBuildStoreFixture(t, nil)
		selector := controlledBuildOutcomeSelector(fixture)
		selector.CommandID = "cmd-create"
		if _, found, err := fixture.control.ConvergeControlledBuildDispatch(
			ctx, fixture.ownership, selector,
		); !errors.Is(err, ErrIdempotencyConflict) || found {
			t.Fatalf("occupied command convergence = found=%t, error=%v", found, err)
		}
	})

	fixture := newControlledBuildStoreFixture(t, nil)
	fixture.dispatch(t)
	for name, mutate := range map[string]func(*ControlledBuildDispatchSelector){
		"run":  func(value *ControlledBuildDispatchSelector) { value.RunID = "run-other" },
		"work": func(value *ControlledBuildDispatchSelector) { value.WorkID = "work-other" },
		"builder": func(value *ControlledBuildDispatchSelector) {
			value.BuilderDispatchDigest = "sha256:" + strings.Repeat("e", 64)
		},
	} {
		t.Run(name, func(t *testing.T) {
			selector := controlledBuildOutcomeSelector(fixture)
			mutate(&selector)
			if _, found, err := fixture.control.ConvergeControlledBuildDispatch(
				ctx, fixture.ownership, selector,
			); err == nil || found {
				t.Fatalf("drifted selector convergence = found=%t, error=%v", found, err)
			}
		})
	}
}

func TestControlledBuildDispatchConvergenceRejectsCorruptClosure(t *testing.T) {
	mutations := map[string]func(*testing.T, *controlledBuildStoreFixture, ApplyResult){
		"command digest": func(t *testing.T, fixture *controlledBuildStoreFixture, _ ApplyResult) {
			execControlledOutcomeSQL(t, fixture.control, "DROP TRIGGER commands_no_update")
			execControlledOutcomeSQL(t, fixture.control,
				"UPDATE commands SET request_digest = 'sha256:"+strings.Repeat("0", 64)+"' WHERE command_id = ?",
				fixture.command.ID,
			)
		},
		"result": func(t *testing.T, fixture *controlledBuildStoreFixture, _ ApplyResult) {
			execControlledOutcomeSQL(t, fixture.control, "DROP TRIGGER commands_no_update")
			execControlledOutcomeSQL(t, fixture.control,
				"UPDATE commands SET result_json = CAST('{}' AS BLOB) WHERE command_id = ?", fixture.command.ID,
			)
		},
		"missing event": func(t *testing.T, fixture *controlledBuildStoreFixture, _ ApplyResult) {
			execControlledOutcomeSQL(t, fixture.control, "DROP TRIGGER events_no_delete")
			execControlledOutcomeSQL(t, fixture.control, "DELETE FROM events WHERE command_id = ?", fixture.command.ID)
		},
		"event payload": func(t *testing.T, fixture *controlledBuildStoreFixture, _ ApplyResult) {
			execControlledOutcomeSQL(t, fixture.control, "DROP TRIGGER events_no_update")
			execControlledOutcomeSQL(t, fixture.control,
				"UPDATE events SET data_json = CAST('{}' AS BLOB) WHERE command_id = ?", fixture.command.ID,
			)
		},
		"extra event": func(t *testing.T, fixture *controlledBuildStoreFixture, result ApplyResult) {
			execControlledOutcomeSQL(t, fixture.control, `
				INSERT INTO events (
					event_id, run_id, command_id, revision, ordinal, kind, data_json, recorded_at_us
				) SELECT 'event-extra', run_id, command_id, ?, 1, kind, data_json, recorded_at_us
				  FROM events WHERE event_id = ?`, result.Revision+100, result.EventID,
			)
		},
		"missing effect": func(t *testing.T, fixture *controlledBuildStoreFixture, result ApplyResult) {
			execControlledOutcomeSQL(t, fixture.control, "DROP TRIGGER effects_no_delete")
			execControlledOutcomeSQL(t, fixture.control, "DELETE FROM effects WHERE effect_id = ?", result.EffectIDs[0])
		},
		"effect request": func(t *testing.T, fixture *controlledBuildStoreFixture, result ApplyResult) {
			execControlledOutcomeSQL(t, fixture.control, "DROP TRIGGER effects_restrict_update")
			execControlledOutcomeSQL(t, fixture.control,
				"UPDATE effects SET request_json = CAST('{}' AS BLOB) WHERE effect_id = ?", result.EffectIDs[0],
			)
		},
		"extra effect": func(t *testing.T, fixture *controlledBuildStoreFixture, result ApplyResult) {
			execControlledOutcomeSQL(t, fixture.control, `
				INSERT INTO effects (
					effect_id, run_id, command_id, ordinal, kind, request_json,
					state, attempt, created_at_us
				) SELECT 'effect-extra', run_id, command_id, 1, kind, request_json,
				         'pending', 0, created_at_us
				  FROM effects WHERE effect_id = ?`, result.EffectIDs[0],
			)
		},
		"regressed run": func(t *testing.T, fixture *controlledBuildStoreFixture, result ApplyResult) {
			state, err := fixture.control.State(context.Background(), fixture.request.RunID)
			if err != nil {
				t.Fatal(err)
			}
			state.Revision = result.Revision - 1
			encoded, err := json.Marshal(state)
			if err != nil {
				t.Fatal(err)
			}
			execControlledOutcomeSQL(t, fixture.control, "DROP TRIGGER runs_restrict_update")
			execControlledOutcomeSQL(t, fixture.control,
				"UPDATE runs SET revision = ?, state_json = ? WHERE run_id = ?",
				state.Revision, encoded, state.RunID,
			)
		},
		"plan mismatch": func(t *testing.T, fixture *controlledBuildStoreFixture, _ ApplyResult) {
			state, err := fixture.control.State(context.Background(), fixture.request.RunID)
			if err != nil {
				t.Fatal(err)
			}
			state.PlanDigest = "sha256:" + strings.Repeat("f", 64)
			encoded, err := json.Marshal(state)
			if err != nil {
				t.Fatal(err)
			}
			execControlledOutcomeSQL(t, fixture.control, "DROP TRIGGER runs_restrict_update")
			execControlledOutcomeSQL(t, fixture.control,
				"UPDATE runs SET plan_digest = ?, state_json = ? WHERE run_id = ?",
				state.PlanDigest, encoded, state.RunID,
			)
		},
	}

	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			fixture := newControlledBuildStoreFixture(t, nil)
			result := fixture.dispatch(t)
			mutate(t, fixture, result)
			if _, found, err := fixture.control.ConvergeControlledBuildDispatch(
				context.Background(), fixture.ownership, controlledBuildOutcomeSelector(fixture),
			); err == nil || found {
				t.Fatalf("corrupt controlled dispatch convergence = found=%t, error=%v", found, err)
			}
		})
	}
}

func execControlledOutcomeSQL(t *testing.T, control *Store, query string, arguments ...any) {
	t.Helper()
	if _, err := control.db.Exec(query, arguments...); err != nil {
		t.Fatalf("execute controlled outcome SQL: %v", err)
	}
}
