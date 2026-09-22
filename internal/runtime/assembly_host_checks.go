package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/swornagent/sworn/internal/journal"
	"github.com/swornagent/sworn/internal/protocol"
)

// assemblyHostCheckSet is the union of the declared host checks of every
// slice in an assembly (sworn#343): the deduplicated check commands in the
// phase order host_checks.go defines (quick checks first, long suites after),
// the union contract digest that binds them, and, per check, the slices whose
// approved contract declared it, in track and slice order. It is derived only
// from the admitted plan and Protocol state, so a retry derives the same set.
type assemblyHostCheckSet struct {
	Checks         []string
	ContractDigest string
	Declaring      map[string][]assemblySliceCheck
}

// assemblySliceCheck names one slice that declares a check, with the slice's
// current candidate and that candidate's product tree identity, so the reuse
// rule can find a recorded result for the identical product. Candidate and
// ProductTree are empty when the slice has no candidate.
type assemblySliceCheck struct {
	Slice          string
	Candidate      string
	ProductTree    string
	ContractDigest string
}

// resolvedAssemblyHostCheck is one declared check's recorded result for the
// assembled tree: the assembly's own check.host effect, or, by the reuse
// rule, a slice candidate's recorded pass for the identical tree, in which
// case ReusedFrom names that slice.
type resolvedAssemblyHostCheck struct {
	Result     hostCheckResult
	ReusedFrom string
}

// assemblyDeclaredHostChecks resolves the approved contract of every slice
// in the assembly at the captured heads and unions their declared host
// checks. Any contract that fails to resolve fails the whole set closed, so
// an assembly can never be checked against fewer checks than its slices
// declared.
func assemblyDeclaredHostChecks(
	engine *engine,
	plan protocol.Plan,
	state protocol.State,
) (assemblyHostCheckSet, error) {
	set := assemblyHostCheckSet{Declaring: make(map[string][]assemblySliceCheck)}
	var raw, digests []string
	for _, track := range state.Tracks {
		for _, slice := range track.Slices {
			sliceID := slice.Location.Slice.ID
			hostChecks, contractDigest, err := resolveSliceHostChecks(
				engine, plan, sliceID, state.Refs.Target.Head, state.Refs.Release.Head)
			if err != nil {
				return assemblyHostCheckSet{}, err
			}
			if len(hostChecks) == 0 {
				continue
			}
			digests = append(digests, contractDigest)
			declaring := assemblySliceCheck{Slice: sliceID, ContractDigest: contractDigest}
			if slice.Candidate != nil && slice.Candidate.Receipt.Candidate != nil &&
				slice.Candidate.Receipt.ProductTree != nil {
				declaring.Candidate = *slice.Candidate.Receipt.Candidate
				declaring.ProductTree = *slice.Candidate.Receipt.ProductTree
			}
			for _, check := range hostChecks {
				if _, seen := set.Declaring[check]; !seen {
					raw = append(raw, check)
				}
				set.Declaring[check] = append(set.Declaring[check], declaring)
			}
		}
	}
	if len(raw) == 0 {
		return set, nil
	}
	set.Checks = phaseOrderedHostChecks(raw)
	set.ContractDigest = assemblyContractDigest(digests)
	return set, nil
}

// resolveAssemblyHostCheck reads the recorded result that speaks for one
// declared check of the assembled tree, never executing anything. The
// assembly's own check.host effect wins whenever it has succeeded; otherwise
// the reuse rule applies: the first declaring slice, in track and slice
// order, whose candidate has exactly the assembled product tree and whose
// journaled result for the same check command is a pass. Identity is the
// product tree, the same identity every candidate receipt carries: an
// assembly is composed from the release head, whose reserved record root
// holds the installed plan and contract records a track candidate never
// carries, so the Git trees of an identical product legitimately differ
// there and only there. The runner and the assembly roll-up both resolve
// through here, so the result the preparation consumed is the result the
// verifier is shown. Not found is honest absence, not an error.
func resolveAssemblyHostCheck(
	ctx context.Context,
	engine *engine,
	candidate, productTree string,
	set assemblyHostCheckSet,
	check string,
) (resolvedAssemblyHostCheck, bool, error) {
	work := assemblyHostCheckWork(candidate, set.ContractDigest, check)
	effect, effectID, err := latestJournaledHostCheck(ctx, engine, work)
	switch {
	case err == nil && effect.Kind == "check.host" && effect.State == journal.Succeeded:
		result, parseErr := parseHostCheckResult(
			"", candidate, set.ContractDigest, check, effectID, effect.Result)
		if parseErr != nil {
			return resolvedAssemblyHostCheck{}, false, parseErr
		}
		return resolvedAssemblyHostCheck{Result: result}, true, nil
	case err != nil && !journal.IsCode(err, "EFFECT_NOT_FOUND"):
		return resolvedAssemblyHostCheck{}, false, err
	}
	for _, declaring := range set.Declaring[check] {
		if declaring.Candidate == "" || declaring.ProductTree == "" ||
			declaring.ProductTree != productTree {
			continue
		}
		sliceWork := hostCheckWork(
			declaring.Slice, declaring.Candidate, declaring.ContractDigest, check)
		effect, effectID, err := latestJournaledHostCheck(ctx, engine, sliceWork)
		if err != nil {
			if journal.IsCode(err, "EFFECT_NOT_FOUND") {
				continue
			}
			return resolvedAssemblyHostCheck{}, false, err
		}
		if effect.Kind != "check.host" || effect.State != journal.Succeeded {
			continue
		}
		result, parseErr := parseHostCheckResult(
			declaring.Slice, declaring.Candidate, declaring.ContractDigest,
			check, effectID, effect.Result)
		if parseErr != nil {
			return resolvedAssemblyHostCheck{}, false, parseErr
		}
		if result.Outcome != protocol.CheckOutcomePass {
			continue
		}
		return resolvedAssemblyHostCheck{Result: result, ReusedFrom: declaring.Slice}, true, nil
	}
	return resolvedAssemblyHostCheck{}, false, nil
}

// runAssemblyHostChecks executes the union of the declared host checks of
// every slice in the assembly against the exact assembled candidate, once per
// candidate, and returns the engine-built manifest that becomes the assembly
// candidate receipt's checks bytes (sworn#343). Each check is journaled as a
// check.host effect keyed by the assembly candidate (assemblyHostCheckWork),
// exactly-once through the same admission the slice seal uses, unless the
// reuse rule already holds a slice candidate's recorded pass for the
// identical product tree, in which case that record is cited instead of
// re-running.
// A nil manifest means no slice declares a host check. A failed, timed-out
// or overflowed check returns the typed HOST_CHECK_FAILED failure so the
// preparation refuses; it is never a pass and never absent.
func (s *Service) runAssemblyHostChecks(
	ctx context.Context,
	engine *engine,
	owner journal.OwnerLease,
	state protocol.State,
	plan protocol.Plan,
	candidate string,
) ([]byte, error) {
	set, err := assemblyDeclaredHostChecks(engine, plan, state)
	if err != nil {
		return nil, err
	}
	if len(set.Checks) == 0 {
		return nil, nil
	}
	productTree, err := commitProductTree(engine, candidate)
	if err != nil {
		return nil, runtimeFail("INVALID_CANDIDATE", err)
	}
	results := make([]hostCheckResult, 0, len(set.Checks))
	for _, check := range set.Checks {
		resolved, found, err := resolveAssemblyHostCheck(ctx, engine, candidate, productTree, set, check)
		if err != nil {
			return nil, err
		}
		result := resolved.Result
		if !found || resolved.ReusedFrom == "" {
			// The assembly's own record, when one exists, is replayed by
			// executeHostCheck with the same bounded re-execution rule a
			// slice candidate gets (#296).
			result, err = s.executeHostCheck(
				ctx, engine, owner, "", candidate, set.ContractDigest, check)
			if err != nil {
				return nil, err
			}
		}
		if result.Outcome != protocol.CheckOutcomePass {
			return nil, &hostCheckFailure{result: result, err: runtimeFail(
				"HOST_CHECK_FAILED",
				fmt.Errorf("%s recorded %s: %s", check, result.Outcome, result.Diagnostic),
			)}
		}
		results = append(results, result)
	}
	return buildAssemblyHostCheckResultsManifest(
		state.Release, state.Plan.Metadata.Revision, candidate,
		set.ContractDigest, results)
}

// assemblyCheckResultsFor is the PrepareAssemblyInput.CheckResultsFor hook
// the persisted prepare_assembly action installs: it re-reads Protocol state,
// proves it is still the authority the action was journaled under, and runs
// the assembly host checks for the exact composed candidate.
func (s *Service) assemblyCheckResultsFor(
	ctx context.Context,
	engine *engine,
	owner journal.OwnerLease,
	authority protocolActionAuthority,
	candidate string,
) ([]byte, error) {
	state, err := protocol.ReadState(engine.git, authority.Release, engine.inertness)
	if err != nil {
		return nil, runtimeFail("PROTOCOL_UNAVAILABLE", err)
	}
	if state.Plan.OID != authority.Plan ||
		state.Refs.Release.Head != authority.ReleaseHead ||
		state.Refs.Target.Head != authority.TargetHead {
		return nil, runtimeFail("STALE_DISPATCH", nil)
	}
	plan, err := planFromState(state)
	if err != nil {
		return nil, err
	}
	return s.runAssemblyHostChecks(ctx, engine, owner, state, plan, candidate)
}

// hostCheckFailureResult renders the durable result a protocol action effect
// records when it failed on a typed host-check failure: a refusal binding
// whose detail names the check, its outcome and the candidate, bounded and
// control-free so the exhaustion park can carry it verbatim. Nil for every
// other failure, which records no result exactly as before.
func hostCheckFailureResult(err error) []byte {
	var failed *hostCheckFailure
	if !errors.As(err, &failed) {
		return nil
	}
	return mustJSON(productionRefusalBinding{
		Code:   "HOST_CHECK_FAILED",
		Detail: hostCheckFailureDetail(failed.result),
	})
}

func hostCheckFailureDetail(result hostCheckResult) string {
	detail := fmt.Sprintf(
		"Host check recorded %s (exit %d) for assembly candidate %s: %s",
		result.Outcome, result.ExitCode, result.Candidate, result.Check)
	if result.Diagnostic != "" {
		detail += " (" + result.Diagnostic + ")"
	}
	cleaned := strings.Map(func(character rune) rune {
		if character < 0x20 || character == 0x7f {
			return ' '
		}
		return character
	}, detail)
	if len(cleaned) > 2_048 {
		cleaned = truncateUTF8(cleaned, 2_048)
	}
	return cleaned
}

// hostCheckExhaustionDetail reads the refusal detail off an exhausted
// protocol action effect that failed HOST_CHECK_FAILED. Only the refusal
// binding hostCheckFailureResult writes is read; empty is honest absence.
func hostCheckExhaustionDetail(result []byte) string {
	var refusal productionRefusalBinding
	if len(result) == 0 || json.Unmarshal(result, &refusal) != nil ||
		refusal.Code != "HOST_CHECK_FAILED" || !validParkDetail(refusal.Detail) {
		return ""
	}
	return refusal.Detail
}
