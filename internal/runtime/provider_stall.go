package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/journal"
)

const (
	// providerStallEventVersion is the schema version of the provider-stall
	// wait and probe event bodies. These are lighter, work/epoch/try-scoped
	// facts in the continuationFallbackEvent shape, not the shared, strictly
	// canonical park-event family degradation.go governs.
	providerStallEventVersion = "sworn.provider-stall/v1"

	providerStallWaitEventKind  = "provider_stall_wait"
	providerStallProbeEventKind = "provider_stall_probe"

	providerStallReasonReset    = "provider_reset"
	providerStallReasonSchedule = "backoff_schedule"

	// providerStallProbePassedCode mirrors driver's own private
	// laneProbeCodeLivePassed: the one Code value LaneProbeResult ever
	// reports as Ready. It is duplicated here, not imported, because the
	// driver constant is deliberately unexported; ProbeLane's own Ready
	// bool is the authority at the call site, this string is only for
	// reading a passed probe back out of a journaled event.
	providerStallProbePassedCode = "live_probe_passed"
)

// providerStallWaitEvent is one journaled backoff wait: the engine chose to
// wait for WaitDurationMillis starting at ScheduledAt (the Index-th wait for
// this try) before probing the lane again. Reason names why that duration
// was chosen (a provider-named reset time, or the fixed backoff schedule),
// for a human or seat reading the journal.
type providerStallWaitEvent struct {
	SchemaVersion      string    `json:"schema_version"`
	Work               string    `json:"work"`
	Epoch              int64     `json:"epoch"`
	Try                int64     `json:"try"`
	Index              int64     `json:"index"`
	ScheduledAt        time.Time `json:"scheduled_at"`
	WaitDurationMillis int64     `json:"wait_duration_ms"`
	Reason             string    `json:"reason"`
}

// providerStallProbeEvent is one journaled live-probe outcome following the
// wait of the same Index: the closed, secret-free readiness Code and the
// provider's own request id, never Message.
type providerStallProbeEvent struct {
	SchemaVersion string    `json:"schema_version"`
	Work          string    `json:"work"`
	Epoch         int64     `json:"epoch"`
	Try           int64     `json:"try"`
	Index         int64     `json:"index"`
	ProbedAt      time.Time `json:"probed_at"`
	Profile       string    `json:"profile"`
	Model         string    `json:"model"`
	Code          string    `json:"code"`
	RequestID     string    `json:"request_id,omitempty"`
}

// providerStallSchedule is the fixed 60/120/240/240-second production
// backoff, overridable only by a test binary linked with
// testHooksFromEnv=1 and SWORN_TEST_PROVIDER_STALL_STEP_MILLIS set: a
// production binary carries no runtime knob because nothing ever sets that
// variable.
func providerStallSchedule() []time.Duration {
	if testProviderStallStepMillis != "" {
		if step, err := strconv.ParseInt(testProviderStallStepMillis, 10, 64); err == nil && step >= 10 {
			d := time.Duration(step) * time.Millisecond
			return []time.Duration{d, 2 * d, 4 * d, 4 * d}
		}
	}
	return []time.Duration{60 * time.Second, 120 * time.Second, 240 * time.Second, 240 * time.Second}
}

// providerStallTotalBound is the declared total wait-and-probe bound (A3):
// 30 minutes in production, overridable the same way as the schedule.
func providerStallTotalBound() time.Duration {
	if testProviderStallBoundMillis != "" {
		if bound, err := strconv.ParseInt(testProviderStallBoundMillis, 10, 64); err == nil && bound >= 100 {
			return time.Duration(bound) * time.Millisecond
		}
	}
	return 30 * time.Minute
}

// providerStallGate is the between-tries admission shape shared by every
// call site (S5 C2): it never runs for the loop's last try (the existing
// exhaustion park already owns that case, and running the gate there would
// wait and probe for nothing before it), and never runs when try+1's own
// attempt effect already exists (a replay after a passed probe, or a
// journal written before this release), so a restart never retroactively
// waits in front of a try that has already started. work is the outer work
// identity the try chain is driving; dispatchEffect is the
// driver.dispatch-shaped effect (raw provider ErrorCode and Result) the
// failed try produced.
func (s *Service) providerStallGate(
	ctx context.Context,
	engine *engine,
	work string,
	epoch, try, maxTry int64,
	role driver.Role,
	dispatchEffect journal.Effect,
) (bool, error) {
	if try >= maxTry {
		return false, nil
	}
	nextEffectID := journal.AttemptEffectID(work, epoch, try+1)
	_, nextErr := s.journal.Effect(ctx, engine.manifest.value.RunID, nextEffectID)
	if nextErr == nil {
		return false, nil
	}
	if !journal.IsCode(nextErr, "EFFECT_NOT_FOUND") {
		return false, runtimeFail("JOURNAL_READ_FAILED", nextErr)
	}
	return s.providerStallGuardsParked(
		ctx, engine, engine.manifest.value.RunID, work, epoch, try, role, dispatchEffect,
	)
}

// providerStallGuardsParked is the provider-stall wait-and-probe state
// machine (A1). It runs synchronously inside the caller's own try loop
// (driveOwnedCycle's owner-lease-renewing goroutine keeps the lease alive
// independently of this call, exactly as it does during any other long
// dispatch): it blocks until either a probe of the failed try's own profile
// and model passes (returns false, admitting the next try) or the declared
// total bound is exceeded (writes the typed provider-stall park and returns
// true). A restart resumes: every wait and probe it journals is replay-key
// idempotent, so re-entering this function after a crash re-reads exactly
// where it left off instead of starting a fresh wait or skipping the step.
func (s *Service) providerStallGuardsParked(
	ctx context.Context,
	engine *engine,
	runID, work string,
	epoch, try int64,
	role driver.Role,
	failedEffect journal.Effect,
) (bool, error) {
	if failedEffect.ErrorCode != "PROVIDER_UNAVAILABLE" &&
		failedEffect.ErrorCode != "PROVIDER_LIMITED" {
		return false, nil
	}
	if engine == nil || engine.configured == nil {
		// A non-production (fake) manifest has no live lane to probe.
		return false, nil
	}
	var resetAfter time.Duration
	if failedEffect.ErrorCode == "PROVIDER_LIMITED" && len(failedEffect.Result) != 0 {
		var refusal productionRefusalBinding
		if json.Unmarshal(failedEffect.Result, &refusal) == nil && refusal.ResetAfterMillis > 0 {
			resetAfter = time.Duration(refusal.ResetAfterMillis) * time.Millisecond
		}
	}
	selection, ok := selectionForRole(engine.manifest.value.Roles, role)
	if !ok {
		return false, nil
	}
	profile, model := selection.Profile, selection.Model
	bound := providerStallTotalBound()
	schedule := providerStallSchedule()
	for {
		snapshot, err := s.journal.Snapshot(ctx, runID)
		if err != nil {
			return false, runtimeFail("JOURNAL_READ_FAILED", err)
		}
		waits, probes := providerStallEventsFor(snapshot, work, epoch, try)
		anchor := s.now().UTC()
		if len(waits) != 0 {
			anchor = waits[0].ScheduledAt
		}
		var wait providerStallWaitEvent
		if len(waits) != 0 {
			latest := waits[len(waits)-1]
			if probe, found := providerStallProbeForIndex(probes, latest.Index); found {
				if probe.Code == providerStallProbePassedCode {
					return false, nil
				}
				// The latest wait's own probe already failed: fall through
				// to schedule the next index below.
			} else {
				// The latest wait is still pending its probe (a restart
				// mid-wait): resume it rather than scheduling a new one.
				wait = latest
			}
		}
		if wait.SchemaVersion == "" {
			index := int64(len(waits)) + 1
			elapsed := s.now().UTC().Sub(anchor)
			if elapsed >= bound {
				return true, s.appendProviderStallParkEvent(
					ctx, runID, work, failedEffect, elapsed, providerStallLastProbeCode(probes),
				)
			}
			var duration time.Duration
			reason := providerStallReasonSchedule
			if index == 1 && resetAfter > 0 {
				duration, reason = resetAfter, providerStallReasonReset
			} else {
				stepIdx := index - 1
				if stepIdx >= int64(len(schedule)) {
					stepIdx = int64(len(schedule) - 1)
				}
				duration = schedule[stepIdx]
			}
			if remaining := bound - elapsed; duration > remaining {
				duration = remaining
			}
			if duration < 0 {
				duration = 0
			}
			now := s.now().UTC()
			wait = providerStallWaitEvent{
				SchemaVersion: providerStallEventVersion, Work: work, Epoch: epoch, Try: try,
				Index: index, ScheduledAt: now, WaitDurationMillis: duration.Milliseconds(),
				Reason: reason,
			}
			body := mustJSON(wait)
			if err := s.journal.AppendEventOnce(ctx, journal.Command{
				RunID: runID, ReplayKey: providerStallReplayKey("wait", work, epoch, try, index),
				Kind: "provider-stall-wait", Payload: body, CreatedAt: now,
			}, providerStallWaitEventKind, body, now); err != nil {
				return false, runtimeFail("JOURNAL_WRITE_FAILED", err)
			}
		}
		deadline := wait.ScheduledAt.Add(time.Duration(wait.WaitDurationMillis) * time.Millisecond)
		if sleepErr := s.sleepFor(ctx, deadline.Sub(s.now().UTC())); sleepErr != nil {
			return false, sleepErr
		}
		result, probeErr := driver.ProbeLane(ctx, engine.configured.registry, profile, model)
		if probeErr != nil {
			return false, runtimeFail("PROVIDER_STALL_PROBE_UNAVAILABLE", probeErr)
		}
		probedAt := s.now().UTC()
		probeBody := mustJSON(providerStallProbeEvent{
			SchemaVersion: providerStallEventVersion, Work: work, Epoch: epoch, Try: try,
			Index: wait.Index, ProbedAt: probedAt, Profile: profile, Model: model,
			Code: result.Code, RequestID: result.RequestID,
		})
		if err := s.journal.AppendEventOnce(ctx, journal.Command{
			RunID: runID, ReplayKey: providerStallReplayKey("probe", work, epoch, try, wait.Index),
			Kind: "provider-stall-probe", Payload: probeBody, CreatedAt: probedAt,
		}, providerStallProbeEventKind, probeBody, probedAt); err != nil {
			return false, runtimeFail("JOURNAL_WRITE_FAILED", err)
		}
		if result.Ready {
			return false, nil
		}
		// Loop: re-read the journal (now carrying this failed probe) and
		// schedule the next index.
	}
}

// providerStallEventsFor scans snapshot for this work/epoch/try's wait and
// probe events, sorted ascending by Index.
func providerStallEventsFor(
	snapshot journal.Snapshot, work string, epoch, try int64,
) ([]providerStallWaitEvent, []providerStallProbeEvent) {
	var waits []providerStallWaitEvent
	var probes []providerStallProbeEvent
	for _, event := range snapshot.Events {
		switch event.Kind {
		case providerStallWaitEventKind:
			var wait providerStallWaitEvent
			if json.Unmarshal(event.Body, &wait) == nil &&
				wait.SchemaVersion == providerStallEventVersion &&
				wait.Work == work && wait.Epoch == epoch && wait.Try == try {
				waits = append(waits, wait)
			}
		case providerStallProbeEventKind:
			var probe providerStallProbeEvent
			if json.Unmarshal(event.Body, &probe) == nil &&
				probe.SchemaVersion == providerStallEventVersion &&
				probe.Work == work && probe.Epoch == epoch && probe.Try == try {
				probes = append(probes, probe)
			}
		}
	}
	sort.Slice(waits, func(i, j int) bool { return waits[i].Index < waits[j].Index })
	sort.Slice(probes, func(i, j int) bool { return probes[i].Index < probes[j].Index })
	return waits, probes
}

func providerStallProbeForIndex(
	probes []providerStallProbeEvent, index int64,
) (providerStallProbeEvent, bool) {
	for _, probe := range probes {
		if probe.Index == index {
			return probe, true
		}
	}
	return providerStallProbeEvent{}, false
}

func providerStallLastProbeCode(probes []providerStallProbeEvent) string {
	if len(probes) == 0 {
		return ""
	}
	return probes[len(probes)-1].Code
}

func providerStallReplayKey(kind, work string, epoch, try, index int64) string {
	return "provider-stall/" + kind + "/" + work + "/" +
		strconv.FormatInt(epoch, 10) + "/" + strconv.FormatInt(try, 10) + "/" +
		strconv.FormatInt(index, 10)
}

// appendProviderStallParkEvent writes the terminal typed provider-stall
// park (A3), work-scoped exactly like the economy and identical-failure
// causes, once per work via the shared cause-scoped replay key.
func (s *Service) appendProviderStallParkEvent(
	ctx context.Context,
	runID, work string,
	failedEffect journal.Effect,
	elapsed time.Duration,
	lastProbeCode string,
) error {
	body, err := canonicalDegradationParkEvent(DegradationParkEvent{
		SchemaVersion: ParkEventVersion,
		RunID:         runID,
		Cause:         ParkCauseProviderStall,
		FailureCode:   failedEffect.ErrorCode,
		FailureDetail: providerStallParkDetail(failedEffect.ErrorCode, elapsed, lastProbeCode),
		Work:          work,
	})
	if err != nil {
		return err
	}
	return s.appendParkEventOnce(ctx, runID, ParkCauseProviderStall, body)
}

// providerStallParkFacts carries one work's terminal provider-stall park
// facts (code, bounded detail) for resolveLanePins/parkStatusFor, in the
// same shape identicalFailureFacts and exhaustionParkFacts already use.
type providerStallParkFacts struct {
	code, detail string
}

// providerStallParkedWorks scans the journal's already-written
// provider_stall park events directly, keyed by the work each one names.
// Unlike identical-failure and exhaustion, a provider-stall park is never
// predicted ahead of its own write: providerStallGuardsParked writes the
// typed park event synchronously, before the failing try loop it gates
// ever returns, so by the time Status reads the journal a genuine
// terminal park is already a written fact.
func providerStallParkedWorks(snapshot journal.Snapshot) map[string]providerStallParkFacts {
	result := make(map[string]providerStallParkFacts)
	for _, event := range snapshot.Events {
		if event.Kind != ParkEventKind {
			continue
		}
		parsed, err := ParseDegradationParkEvent(event.Body)
		if err != nil || parsed.Cause != ParkCauseProviderStall {
			continue
		}
		result[parsed.Work] = providerStallParkFacts{
			code: parsed.FailureCode, detail: parsed.FailureDetail,
		}
	}
	return result
}

// firstProviderStallPark returns one deterministic (lowest work identity)
// entry from parked, for the coarse run-scoped park summary a Status call
// falls back to before Protocol state is confirmed readable.
func firstProviderStallPark(
	parked map[string]providerStallParkFacts,
) (string, *providerStallParkFacts) {
	if len(parked) == 0 {
		return "", nil
	}
	works := make([]string, 0, len(parked))
	for work := range parked {
		works = append(works, work)
	}
	sort.Strings(works)
	facts := parked[works[0]]
	return works[0], &facts
}

// providerStallWaitingStatuses projects every work currently mid-wait (A2):
// a wait/probe group whose owning work already carries a terminal park, or
// whose next try's attempt effect already exists, has resolved and is
// omitted - only a genuinely still-waiting work is reported.
func providerStallWaitingStatuses(
	snapshot journal.Snapshot,
	parked map[string]providerStallParkFacts,
) []ProviderStallStatus {
	type groupKey struct {
		work       string
		epoch, try int64
	}
	seen := make(map[groupKey]struct{})
	var order []groupKey
	for _, event := range snapshot.Events {
		if event.Kind != providerStallWaitEventKind {
			continue
		}
		var wait providerStallWaitEvent
		if json.Unmarshal(event.Body, &wait) != nil ||
			wait.SchemaVersion != providerStallEventVersion {
			continue
		}
		key := groupKey{wait.Work, wait.Epoch, wait.Try}
		if _, exists := seen[key]; !exists {
			seen[key] = struct{}{}
			order = append(order, key)
		}
	}
	effectsByID := make(map[string]journal.Effect, len(snapshot.Effects))
	for _, effect := range snapshot.Effects {
		effectsByID[effect.ID] = effect
	}
	bound := providerStallTotalBound().Milliseconds()
	var result []ProviderStallStatus
	for _, key := range order {
		if _, done := parked[key.work]; done {
			continue
		}
		nextEffectID := journal.AttemptEffectID(key.work, key.epoch, key.try+1)
		if _, exists := effectsByID[nextEffectID]; exists {
			continue
		}
		waits, probes := providerStallEventsFor(snapshot, key.work, key.epoch, key.try)
		if len(waits) == 0 {
			continue
		}
		latest := waits[len(waits)-1]
		status := ProviderStallStatus{
			WorkID: key.work,
			Index:  latest.Index,
			NextProbeAt: latest.ScheduledAt.Add(
				time.Duration(latest.WaitDurationMillis) * time.Millisecond),
			ElapsedMillis: latest.ScheduledAt.Sub(waits[0].ScheduledAt).Milliseconds(),
			BoundMillis:   bound,
		}
		if probe, found := providerStallProbeForIndex(probes, latest.Index); found {
			status.Profile, status.Model = probe.Profile, probe.Model
			status.LastProbeCode = probe.Code
			status.LastProbeRequestID = probe.RequestID
		}
		result = append(result, status)
	}
	return result
}

// providerStallParkDetail renders the bounded, honest park detail: the
// failure code, the wait so far, and the last probe's closed code. Empty
// (never invalid) when the rendering would not pass validParkDetail.
func providerStallParkDetail(code string, elapsed time.Duration, lastProbeCode string) string {
	if lastProbeCode == "" {
		lastProbeCode = "none"
	}
	detail := fmt.Sprintf(
		"%s: waited %s, last probe %s", code, elapsed.Round(time.Second), lastProbeCode,
	)
	if !validParkDetail(detail) {
		return ""
	}
	return detail
}
