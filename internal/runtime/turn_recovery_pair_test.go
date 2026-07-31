package runtime

import (
	"testing"

	"github.com/swornagent/sworn/internal/driver"
)

func TestTurnRecoveryTotalsExposePairedSignedDeltas(t *testing.T) {
	aggregate := func(durations ...int64) driver.Observation {
		t.Helper()
		var totals turnRecoveryTotals
		for _, duration := range durations {
			observation := measuredTurnRecoveryObservation(duration, 7, 5)
			if err := totals.add(
				observation.DurationMillis,
				observation.Usage,
			); err != nil {
				t.Fatal(err)
			}
		}
		var result driver.Observation
		totals.apply(&result)
		return result
	}

	direct := aggregate(5, 11)
	recovery := aggregate(5, 11, 19, 31)
	if direct.Usage.InputTokens == nil ||
		direct.Usage.OutputTokens == nil ||
		recovery.Usage.InputTokens == nil ||
		recovery.Usage.OutputTokens == nil {
		t.Fatalf("paired totals lack reported usage direct=%#v recovery=%#v",
			direct, recovery)
	}
	durationDelta := recovery.DurationMillis - direct.DurationMillis
	inputDelta := *recovery.Usage.InputTokens - *direct.Usage.InputTokens
	outputDelta := *recovery.Usage.OutputTokens - *direct.Usage.OutputTokens
	if direct.DurationMillis != 16 ||
		recovery.DurationMillis != 66 ||
		durationDelta != 50 ||
		*direct.Usage.InputTokens != 14 ||
		*direct.Usage.OutputTokens != 10 ||
		*recovery.Usage.InputTokens != 28 ||
		*recovery.Usage.OutputTokens != 20 ||
		inputDelta != 14 ||
		outputDelta != 10 {
		t.Fatalf(
			"paired totals direct=%#v recovery=%#v duration_delta=%+d input_delta=%+d output_delta=%+d",
			direct,
			recovery,
			durationDelta,
			inputDelta,
			outputDelta,
		)
	}
	t.Logf(
		"paired totals duration_ms direct=%d recovery=%d signed_delta=%+d; tokens direct=%d/%d recovery=%d/%d signed_delta=%+d/%+d",
		direct.DurationMillis,
		recovery.DurationMillis,
		durationDelta,
		*direct.Usage.InputTokens,
		*direct.Usage.OutputTokens,
		*recovery.Usage.InputTokens,
		*recovery.Usage.OutputTokens,
		inputDelta,
		outputDelta,
	)
}
