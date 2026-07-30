package observe

import (
	"sort"
	"strings"
)

const turnRecoveryEventPrefix = "turn_recovery."

const (
	turnRecoveryResumeWorker       = "resume_worker"
	turnRecoveryAskCaptain         = "ask_captain"
	turnRecoveryRetryOperationally = "retry_operationally"
	turnRecoveryPauseForHuman      = "pause_track_for_human"
)

type TurnRecoveryCount struct {
	Action string `json:"action"`
	Count  int64  `json:"count"`
}

// TurnRecoverySummary contains only closed, content-free recovery outcomes.
// Token use and elapsed time remain in the enclosing evaluation record.
type TurnRecoverySummary struct {
	Recovered        int64               `json:"recovered"`
	HumanEscalations int64               `json:"human_escalations"`
	FalseAcceptances int64               `json:"false_acceptances"`
	Actions          []TurnRecoveryCount `json:"actions"`
}

type turnRecoveryAggregate struct {
	recovered        int64
	humanEscalations int64
	falseAcceptances int64
	actions          map[string]int64
}

func (a *turnRecoveryAggregate) add(kind string) error {
	if !strings.HasPrefix(kind, turnRecoveryEventPrefix) {
		return nil
	}
	parts := strings.Split(kind, ".")
	if len(parts) < 3 || parts[0] != "turn_recovery" {
		return fail("INVALID_EVALUATION_FACT")
	}
	switch parts[1] {
	case "action":
		if len(parts) != 3 {
			return fail("INVALID_EVALUATION_FACT")
		}
		if !validTurnRecoveryAction(parts[2]) {
			return fail("INVALID_EVALUATION_FACT")
		}
		if a.actions == nil {
			a.actions = make(map[string]int64)
		}
		value, err := safeAdd(a.actions[parts[2]], 1)
		if err != nil {
			return err
		}
		a.actions[parts[2]] = value
		if parts[2] == turnRecoveryPauseForHuman {
			a.humanEscalations, err = safeAdd(
				a.humanEscalations,
				1,
			)
			if err != nil {
				return err
			}
		}
	case "outcome":
		if len(parts) != 3 &&
			(len(parts) != 6 ||
				parts[2] != "recovered" ||
				parts[3] != continuationEventSegment ||
				!validContinuationMode(parts[4]) ||
				!validContinuationOutcome(parts[5])) {
			return fail("INVALID_EVALUATION_FACT")
		}
		var target *int64
		switch parts[2] {
		case "recovered":
			target = &a.recovered
		case "false_acceptance":
			target = &a.falseAcceptances
		default:
			return fail("INVALID_EVALUATION_FACT")
		}
		value, err := safeAdd(*target, 1)
		if err != nil {
			return err
		}
		*target = value
	default:
		return fail("INVALID_EVALUATION_FACT")
	}
	return nil
}

func (a turnRecoveryAggregate) summary() TurnRecoverySummary {
	result := TurnRecoverySummary{
		Recovered:        a.recovered,
		HumanEscalations: a.humanEscalations,
		FalseAcceptances: a.falseAcceptances,
	}
	for action, count := range a.actions {
		result.Actions = append(result.Actions, TurnRecoveryCount{
			Action: action,
			Count:  count,
		})
	}
	sort.Slice(result.Actions, func(left, right int) bool {
		return result.Actions[left].Action < result.Actions[right].Action
	})
	return result
}

func validTurnRecoveryAction(value string) bool {
	switch value {
	case turnRecoveryResumeWorker, turnRecoveryAskCaptain,
		turnRecoveryRetryOperationally, turnRecoveryPauseForHuman:
		return true
	default:
		return false
	}
}

func validTurnRecoverySummary(
	value TurnRecoverySummary,
	events int64,
) bool {
	if value.Recovered < 0 || value.HumanEscalations < 0 ||
		value.FalseAcceptances < 0 || len(value.Actions) > 4 {
		return false
	}
	var actions int64
	var pauses int64
	previous := ""
	for index, count := range value.Actions {
		if !validTurnRecoveryAction(count.Action) || count.Count < 1 ||
			(index != 0 && count.Action <= previous) {
			return false
		}
		var err error
		actions, err = telemetryAdd(actions, count.Count)
		if err != nil {
			return false
		}
		if count.Action == turnRecoveryPauseForHuman {
			pauses = count.Count
		}
		previous = count.Action
	}
	total, err := telemetryAdd(actions, value.Recovered)
	if err == nil {
		total, err = telemetryAdd(total, value.FalseAcceptances)
	}
	return err == nil &&
		value.HumanEscalations == pauses &&
		total <= events
}
