package runtime

import (
	"encoding/json"
	"strings"

	"github.com/swornagent/sworn/internal/journal"
	"github.com/swornagent/sworn/internal/protocol"
)

const (
	// HostCheckFailureFactSchemaVersion versions the host-check failure
	// fact shape, served on EffectStatus, PinnedWork and cockpit Node.
	HostCheckFailureFactSchemaVersion = "sworn.host-check-failure-fact/v1"
	// HostCheckFailureFactMaxBytes bounds one fact's compact JSON to
	// ~8 KiB. The excerpt itself is bounded to 4 KiB by
	// protocol.HostCheckOutputManifestBytes; this cap is enforced
	// best-effort by truncating the excerpt further, never by failing
	// Status or dropping the fact.
	HostCheckFailureFactMaxBytes = 8 * 1024
)

// HostCheckFailureFact is one bounded, versioned projection of the latest
// failed host check of a work, derived purely at Status time from
// already-journaled, digest-checked check.host results. It is evidence,
// never authority: nothing in the engine reads it to decide a retry, a
// park, a verdict or a repair. Old journals (no repair, unparsable,
// missing contract) report it as absent (nil), never corrupt.
type HostCheckFailureFact struct {
	SchemaVersion string `json:"schema_version"`
	// Check is the check command as declared in the contract, verbatim.
	Check string `json:"check"`
	// Outcome and ExitCode are the first execution's outcome and exit
	// code. When the stored FailedCheck is itself the re-execution
	// result (RerunOf set), they are read from the first execution
	// through RerunOf, and RerunOutcome/RerunExitCode carry the
	// re-executed result, so the two are never confused.
	Outcome  string `json:"outcome"`
	ExitCode int    `json:"exit_code"`
	// Reran reports whether the check was re-executed once for the same
	// candidate. RerunOutcome and RerunExitCode carry that
	// re-execution's outcome; they are absent when Reran is false.
	Reran         bool   `json:"reran"`
	RerunOutcome  string `json:"rerun_outcome,omitempty"`
	RerunExitCode *int   `json:"rerun_exit_code,omitempty"`
	// NotRun is the declared checks that were not run because this one
	// failed, in the phase order the engine used
	// (phaseOrderedHostChecks): the checks after the failed check's
	// position in that order. It is known-empty (nil with
	// NotRunUnknown false) when the failed check is last. It is
	// absent with NotRunUnknown true when the resolved contract does
	// not match the stored result's contract digest, the failed check
	// is not in its list, or the contract cannot be resolved at
	// Status time. It is never guessed from a different contract.
	NotRun []string `json:"not_run,omitempty"`
	// NotRunUnknown marks NotRun as unknown (absent because the
	// contract is unavailable or does not match), distinct from
	// known-empty.
	NotRunUnknown bool `json:"not_run_unknown,omitempty"`
	// Excerpt is the bounded output excerpt taken from the same stored
	// result the implementer's repair context already holds
	// (FailedCheck), via hostOutputExcerpt. ExcerptTruncated is its
	// truthful marker.
	Excerpt          string `json:"excerpt"`
	ExcerptTruncated bool   `json:"excerpt_truncated"`
	// OutputDigest is the digest of the full bounded output of the
	// stored result (FailedCheck.OutputDigest).
	OutputDigest string `json:"output_digest"`
	// HostEffect is the first execution's check.host effect id.
	HostEffect string `json:"host_effect"`
	// RerunEffect is the re-execution's check.host effect id, present
	// only when Reran is true.
	RerunEffect string `json:"rerun_effect,omitempty"`
	// Candidate and ContractDigest bind the stored result.
	Candidate      string `json:"candidate"`
	ContractDigest string `json:"contract_digest"`
}

// hostCheckFailureFactsForSnapshot derives the host-check failure fact for
// every owner work whose latest terminal driver.dispatch failed
// HOST_CHECK_FAILED. Latest means the highest (epoch,try) terminal
// (operationally failed or succeeded) dispatch for the owner work; an
// in-flight (claimed, pending, uncertain) dispatch never clears the fact,
// while a later terminal dispatch (e.g. a passing candidate) does.
//
// contractHostChecks resolves the declared host_checks for a slice at
// Status time. It returns the declared list in declared order, the plan's
// contract digest for the slice, and whether resolution succeeded. Any
// resolution failure reports NotRun as unknown, never as corrupt and never
// by failing Status.
func hostCheckFailureFactsForSnapshot(
	snapshot journal.Snapshot,
	contractHostChecks func(sliceID string) ([]string, string, bool),
) map[string]*HostCheckFailureFact {
	result := make(map[string]*HostCheckFailureFact)
	effectsByID := make(map[string]journal.Effect, len(snapshot.Effects))
	for _, effect := range snapshot.Effects {
		effectsByID[effect.ID] = effect
	}
	type latestTerminal struct {
		effect journal.Effect
		epoch  int64
		try    int64
	}
	latest := make(map[string]latestTerminal)
	for _, effect := range snapshot.Effects {
		if effect.Kind != "driver.dispatch" {
			continue
		}
		if effect.State != journal.OperationalFailed && effect.State != journal.Succeeded {
			continue
		}
		work, epoch, try, err := attemptCoordinates(effect.ID)
		if err != nil {
			continue
		}
		owner := ownerWorkForDispatch(snapshot, work)
		existing, ok := latest[owner]
		if !ok || epoch > existing.epoch || (epoch == existing.epoch && try > existing.try) {
			latest[owner] = latestTerminal{effect: effect, epoch: epoch, try: try}
		}
	}
	for _, terminal := range latest {
		effect := terminal.effect
		if effect.State != journal.OperationalFailed || effect.ErrorCode != "HOST_CHECK_FAILED" {
			continue
		}
		if len(effect.Result) == 0 {
			continue
		}
		fact := buildHostCheckFailureFact(snapshot, effectsByID, effect, contractHostChecks)
		if fact == nil {
			continue
		}
		result[effect.ID] = fact
	}
	return result
}

// buildHostCheckFailureFact derives one fact from a HOST_CHECK_FAILED
// dispatch's repair result. Nil means absent (unparsable, shape mismatch,
// missing or mismatched journaled evidence), never corrupt.
func buildHostCheckFailureFact(
	snapshot journal.Snapshot,
	effectsByID map[string]journal.Effect,
	dispatchEffect journal.Effect,
	contractHostChecks func(sliceID string) ([]string, string, bool),
) (fact *HostCheckFailureFact) {
	defer func() {
		_ = recover()
	}()
	if dispatchEffect.Kind != "driver.dispatch" ||
		dispatchEffect.State != journal.OperationalFailed ||
		dispatchEffect.ErrorCode != "HOST_CHECK_FAILED" ||
		len(dispatchEffect.Result) == 0 {
		return nil
	}
	var repair productionHostRepair
	if json.Unmarshal(dispatchEffect.Result, &repair) != nil {
		return nil
	}
	if !bytesEqualCanonicalJSON(dispatchEffect.Result, repair) {
		return nil
	}
	if validateHostRepair(repair, repair.Submission.InvocationID, repair.FailedCheck.Slice) != nil {
		return nil
	}
	check := repair.FailedCheck
	stored, ok := effectsByID[check.EffectID]
	if !ok || stored.Kind != "check.host" || stored.State != journal.Succeeded {
		return nil
	}
	if _, err := parseHostCheckResult(
		check.Slice, check.Candidate, check.ContractDigest, check.Check,
		check.EffectID, stored.Result,
	); err != nil {
		return nil
	}
	if !bytesEqualCanonicalJSON(stored.Result, check) {
		return nil
	}
	work := hostCheckWork(check.Slice, check.Candidate, check.ContractDigest, check.Check)
	var firstResult hostCheckResult
	var rerunResult *hostCheckResult
	var firstEffectID, rerunEffectID string
	reran := false
	if check.RerunOf != "" {
		if check.EffectID != hostCheckRerunEffectID(work) {
			return nil
		}
		if check.RerunOf != hostCheckEffectID(work) {
			return nil
		}
		firstStored, found := effectsByID[check.RerunOf]
		if !found || firstStored.Kind != "check.host" || firstStored.State != journal.Succeeded {
			firstResult = check
			firstEffectID = check.EffectID
			rerunCopy := check
			rerunResult = &rerunCopy
			rerunEffectID = check.EffectID
			reran = true
		} else {
			firstParsed, err := parseHostCheckResult(
				check.Slice, check.Candidate, check.ContractDigest, check.Check,
				check.RerunOf, firstStored.Result,
			)
			if err != nil {
				firstResult = check
				firstEffectID = check.EffectID
				rerunCopy := check
				rerunResult = &rerunCopy
				rerunEffectID = check.EffectID
				reran = true
			} else {
				firstResult = firstParsed
				firstEffectID = check.RerunOf
				rerunCopy := check
				rerunResult = &rerunCopy
				rerunEffectID = check.EffectID
				reran = true
			}
		}
	} else {
		if check.EffectID != hostCheckEffectID(work) {
			return nil
		}
		firstResult = check
		firstEffectID = check.EffectID
		rerunID := hostCheckRerunEffectID(work)
		if rerunStored, found := effectsByID[rerunID]; found &&
			rerunStored.Kind == "check.host" && rerunStored.State == journal.Succeeded {
			if rerunParsed, err := parseHostCheckResult(
				check.Slice, check.Candidate, check.ContractDigest, check.Check,
				rerunID, rerunStored.Result,
			); err == nil && rerunParsed.RerunOf == check.EffectID {
				rerunCopy := rerunParsed
				rerunResult = &rerunCopy
				rerunEffectID = rerunID
				reran = true
			}
		}
	}
	var notRun []string
	notRunUnknown := true
	if contractHostChecks != nil {
		if hostChecks, contractDigest, resolved := contractHostChecks(check.Slice); resolved &&
			contractDigest == check.ContractDigest {
			ordered := phaseOrderedHostChecks(hostChecks)
			index := -1
			for position, declared := range ordered {
				if declared == check.Check {
					index = position
					break
				}
			}
			if index >= 0 {
				notRunUnknown = false
				if index+1 < len(ordered) {
					notRun = append([]string(nil), ordered[index+1:]...)
				} else {
					notRun = nil
				}
			}
		}
	}
	excerpt, excerptTruncated := hostOutputExcerpt(check.Output, check.Truncated)
	built := &HostCheckFailureFact{
		SchemaVersion:    HostCheckFailureFactSchemaVersion,
		Check:            check.Check,
		Outcome:          firstResult.Outcome,
		ExitCode:         firstResult.ExitCode,
		Reran:            reran,
		NotRun:           notRun,
		NotRunUnknown:    notRunUnknown,
		Excerpt:          excerpt,
		ExcerptTruncated: excerptTruncated,
		OutputDigest:     check.OutputDigest,
		HostEffect:       firstEffectID,
		Candidate:        check.Candidate,
		ContractDigest:   check.ContractDigest,
	}
	if reran && rerunResult != nil {
		built.RerunOutcome = rerunResult.Outcome
		exitCode := rerunResult.ExitCode
		built.RerunExitCode = &exitCode
		built.RerunEffect = rerunEffectID
	}
	enforceHostCheckFailureFactBound(built)
	return built
}

// enforceHostCheckFailureFactBound truncates the excerpt further when the
// whole fact exceeds HostCheckFailureFactMaxBytes. It never fails and
// never drops the fact; the excerpt stays prefix-preserving with a
// truthful truncation marker.
func enforceHostCheckFailureFactBound(fact *HostCheckFailureFact) {
	if fact == nil {
		return
	}
	defer func() {
		_ = recover()
	}()
	for attempt := 0; attempt < 4; attempt++ {
		body, err := json.Marshal(fact)
		if err != nil || len(body) <= HostCheckFailureFactMaxBytes {
			return
		}
		over := len(body) - HostCheckFailureFactMaxBytes
		target := len(fact.Excerpt) - over - 256
		if target < 0 {
			target = 0
		}
		fact.Excerpt = truncateUTF8(fact.Excerpt, target)
		if !strings.Contains(fact.Excerpt, protocol.HostCheckTruncationPrefix) {
			marker := "\n[sworn: output truncated at " + fact.OutputDigest + "]\n"
			if len(fact.Excerpt)+len(marker) <= protocol.HostCheckOutputManifestBytes+256 {
				fact.Excerpt += marker
			}
		}
		fact.ExcerptTruncated = true
		if len(fact.Excerpt) == 0 {
			return
		}
	}
}

// hostCheckFailureForPinnedWork finds the pinned work's latest failed
// dispatch fact, reusing the already-derived Effect fact (same pointer, no
// re-decode) so Effects, PinnedWork and Node stay in parity. Nil means the
// pin has no failed dispatch to show. Matching follows
// failureContextForPinnedWork: an economy pin names its own dispatch-work
// identity directly, otherwise the dispatch whose owner is the pinned work.
func hostCheckFailureForPinnedWork(
	snapshot journal.Snapshot,
	effects []EffectStatus,
	pinned PinnedWork,
) *HostCheckFailureFact {
	byID := make(map[string]*HostCheckFailureFact, len(effects))
	for index := range effects {
		if effects[index].HostCheckFailure != nil {
			byID[effects[index].ID] = effects[index].HostCheckFailure
		}
	}
	if len(byID) == 0 {
		return nil
	}
	var bestID string
	var bestEpoch, bestTry int64 = -1, -1
	for _, effect := range effects {
		if effect.HostCheckFailure == nil {
			continue
		}
		work, epoch, try, err := attemptCoordinates(effect.ID)
		if err != nil {
			continue
		}
		matched := false
		if pinned.DispatchWorkID != "" {
			matched = work == pinned.DispatchWorkID
		} else {
			matched = ownerWorkForDispatch(snapshot, work) == pinned.WorkID
		}
		if !matched {
			continue
		}
		if epoch > bestEpoch || (epoch == bestEpoch && try > bestTry) {
			bestEpoch, bestTry, bestID = epoch, try, effect.ID
		}
	}
	if bestID == "" {
		return nil
	}
	return byID[bestID]
}

// checkOutcomeForEffect reports the journaled check's outcome for one
// check.host effect, bound through the effect's journaled hostCheckCommand
// payload (slice, candidate, contract_digest, check, effect id). Assembly
// check.host effects (slice "") are bound the same way. Any parse or
// binding mismatch leaves it absent (""), never corrupt.
func checkOutcomeForEffect(
	effect journal.Effect,
	commands map[string]journal.Command,
) string {
	defer func() {
		_ = recover()
	}()
	if effect.Kind != "check.host" || effect.State != journal.Succeeded {
		return ""
	}
	command, ok := commands[effect.ReplayKey]
	if !ok || command.Kind != "check.host" {
		return ""
	}
	var hostCommand hostCheckCommand
	if json.Unmarshal(command.Payload, &hostCommand) != nil {
		return ""
	}
	if !bytesEqualCanonicalJSON(command.Payload, hostCommand) {
		return ""
	}
	if hostCommand.SchemaVersion != hostCheckSchemaVersion {
		return ""
	}
	work := hostCheckWork(
		hostCommand.Slice, hostCommand.Candidate,
		hostCommand.ContractDigest, hostCommand.Check,
	)
	var expectedWork, expectedID string
	if hostCommand.RerunOf == "" {
		expectedWork, expectedID = work, hostCheckEffectID(work)
	} else {
		if hostCommand.RerunOf != hostCheckEffectID(work) {
			return ""
		}
		expectedWork, expectedID = hostCheckRerunWork(work), hostCheckRerunEffectID(work)
	}
	if effect.ID != expectedID || effect.BeforeDigest != expectedWork {
		return ""
	}
	result, err := parseHostCheckResult(
		hostCommand.Slice, hostCommand.Candidate, hostCommand.ContractDigest,
		hostCommand.Check, effect.ID, effect.Result,
	)
	if err != nil {
		return ""
	}
	switch result.Outcome {
	case protocol.CheckOutcomePass,
		protocol.CheckOutcomeFail,
		protocol.CheckOutcomeTimeout,
		protocol.CheckOutcomeOverflow:
		return result.Outcome
	default:
		return ""
	}
}

// statusContractHostChecksResolver builds the Status-time contract
// resolver for hostCheckFailureFactsForSnapshot. It resolves the declared
// host_checks from the Status-selected proposal plan at the current heads
// (or the proposal's own authority heads when Protocol state is
// unavailable). Any resolution error reports NotRun as unknown, never as
// corrupt and never by failing Status.
func statusContractHostChecksResolver(
	proposal admittedPlanProposal,
	proposalFound bool,
	statusEngine *engine,
	state protocol.State,
	stateErr error,
) func(string) ([]string, string, bool) {
	if !proposalFound || statusEngine == nil {
		return func(string) ([]string, string, bool) {
			return nil, "", false
		}
	}
	var targetHead, releaseHead string
	if stateErr == nil {
		targetHead, releaseHead = state.Refs.Target.Head, state.Refs.Release.Head
	} else {
		targetHead, releaseHead = proposal.authority.TargetHead, proposal.authority.ReleaseHead
	}
	return func(sliceID string) (hostChecks []string, contractDigest string, resolved bool) {
		defer func() {
			if recovered := recover(); recovered != nil {
				hostChecks, contractDigest, resolved = nil, "", false
			}
		}()
		if sliceID == "" || statusEngine == nil {
			return nil, "", false
		}
		hostChecks, contractDigest, err := resolveSliceHostChecks(
			statusEngine, proposal.plan, sliceID, targetHead, releaseHead,
		)
		if err != nil {
			return nil, "", false
		}
		return hostChecks, contractDigest, true
	}
}

// attachHostCheckFailureFacts derives the facts for snapshot with resolver
// and attaches them to result's EffectStatus entries by effect id,
// clearing any prior attachment for driver.dispatch effects. It never
// fails; unparsable or missing evidence reports absent.
func attachHostCheckFailureFacts(
	result *RunStatus,
	snapshot journal.Snapshot,
	resolver func(string) ([]string, string, bool),
) {
	if result == nil {
		return
	}
	defer func() {
		_ = recover()
	}()
	facts := hostCheckFailureFactsForSnapshot(snapshot, resolver)
	for index := range result.Effects {
		if result.Effects[index].Kind != "driver.dispatch" {
			continue
		}
		if fact, ok := facts[result.Effects[index].ID]; ok {
			result.Effects[index].HostCheckFailure = fact
		} else {
			result.Effects[index].HostCheckFailure = nil
		}
	}
}
