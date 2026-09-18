package runtime

import (
	"sort"
	"strings"

	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/protocol"
)

// anchorClauseMarker is the fixed prose marker S1-seal-time-gates' A2 gate
// keys off: an acceptance criterion that names files for the Verifier ends
// its text with "Anchor: path[, path...][ and path]." A criterion whose text
// carries no such clause is never gated.
const anchorClauseMarker = "Anchor:"

// criterionAnchorTokens extracts the candidate path tokens named in one
// criterion's trailing "Anchor:" clause, in the order they were written.
// This is a fixed, documented predicate over contract bytes the engine
// already holds: no model call, no judgement of sufficiency. It does not
// itself decide which tokens are real paths - a bare word never coincides
// with a tracked repository path, so resolveAnchorRequirements' own
// base-tree membership filter is the sole and sufficient authority for
// telling a genuine anchor file (nested, like
// internal/runtime/host_repair_test.go, or root-level, like base.txt) apart
// from stray prose.
func criterionAnchorTokens(text string) []string {
	idx := strings.LastIndex(text, anchorClauseMarker)
	if idx < 0 {
		return nil
	}
	clause := strings.TrimSpace(text[idx+len(anchorClauseMarker):])
	clause = strings.TrimSuffix(clause, ".")
	clause = strings.ReplaceAll(clause, " and ", ", ")
	fields := strings.Split(clause, ",")
	tokens := make([]string, 0, len(fields))
	for _, field := range fields {
		token := strings.Trim(strings.TrimSpace(field), ".")
		if token == "" {
			continue
		}
		tokens = append(tokens, token)
	}
	return tokens
}

// anchorCriterionRequirement names one acceptance criterion's declared
// anchor paths, filtered to the tokens A2 requires: those that exist in the
// exact base tree the candidate is diffed against.
type anchorCriterionRequirement struct {
	id    string
	paths []string
}

// resolveAnchorRequirements extracts every criterion's anchor clause and
// keeps only the named paths present in baseTree, A2's own "path-like
// tokens that exist in the exact base tree". A criterion whose anchor
// clause names only files absent from the base (for example a wholly new
// test file this same candidate is expected to add) has nothing to check
// presence against and is not gated; its diff is still visible to the
// Verifier, whose sufficiency judgement this gate never substitutes for.
func resolveAnchorRequirements(
	criteria []protocol.Criterion,
	baseTree map[string]struct{},
) []anchorCriterionRequirement {
	requirements := make([]anchorCriterionRequirement, 0, len(criteria))
	for _, criterion := range criteria {
		tokens := criterionAnchorTokens(criterion.Text)
		if len(tokens) == 0 {
			continue
		}
		present := make([]string, 0, len(tokens))
		for _, token := range tokens {
			if _, ok := baseTree[token]; ok {
				present = append(present, token)
			}
		}
		if len(present) == 0 {
			continue
		}
		requirements = append(requirements, anchorCriterionRequirement{
			id: criterion.ID, paths: present,
		})
	}
	return requirements
}

// anchorPathInScope mirrors protocol's own unexported scope predicate
// (pathInScope in candidate.go) so a declared A3 substitute is judged by
// the identical include/exclude rule ValidateSliceCandidateScope already
// enforces on the whole candidate, without reaching into protocol's
// internals.
func anchorPathInScope(scope protocol.Scope, path string) bool {
	included := false
	for _, include := range scope.Include {
		if path == include || strings.HasPrefix(path, include+"/") {
			included = true
			break
		}
	}
	if !included {
		return false
	}
	for _, exclude := range scope.Exclude {
		if path == exclude || strings.HasPrefix(path, exclude+"/") {
			return false
		}
	}
	return true
}

// anchorPresenceGate is S1-seal-time-gates' A2/A3 deterministic, pre-host
// seal-time gate. It calls no model: it diffs base..candidate by name only,
// checks every acceptance criterion's declared anchor files (or a valid
// declared A3 substitute) against that diff, and refuses ANCHOR_NOT_TOUCHED
// naming every criterion and file still untouched, and separately naming any
// declared substitute that failed and why, when any criterion's anchors are
// absent from the diff. Presence only: a touched anchor satisfies this gate
// regardless of whether it actually proves anything, which stays the
// Verifier's judgement. An unreadable base tree or an ambiguous diff fails
// closed with ANCHOR_GATE_UNREADABLE rather than silently passing. On
// success it returns the criterion-to-path map of every declared substitute
// that was honoured, so the caller can record it on the seal itself.
func anchorPresenceGate(
	engine *engine,
	contract protocol.Slice,
	base, candidate string,
	substitutes map[string]string,
) (map[string]string, error) {
	if engine == nil || engine.repository == nil {
		return nil, runtimeFail("INVALID_ENGINE", nil)
	}
	format := engine.repository.ObjectFormat()
	baseOID, err := gitx.ParseOID(format, base)
	if err != nil {
		return nil, runtimeFail("ANCHOR_GATE_UNREADABLE", err)
	}
	candidateOID, err := gitx.ParseOID(format, candidate)
	if err != nil {
		return nil, runtimeFail("ANCHOR_GATE_UNREADABLE", err)
	}
	entries, err := engine.repository.ListTree(baseOID)
	if err != nil {
		return nil, runtimeFail("ANCHOR_GATE_UNREADABLE", err)
	}
	baseTree := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		baseTree[entry.Path] = struct{}{}
	}
	requirements := resolveAnchorRequirements(contract.Acceptance, baseTree)
	if len(requirements) == 0 {
		return nil, nil
	}
	changedPaths, err := engine.repository.ChangedPaths(baseOID, candidateOID)
	if err != nil {
		return nil, runtimeFail("ANCHOR_GATE_UNREADABLE", err)
	}
	changed := make(map[string]struct{}, len(changedPaths))
	for _, path := range changedPaths {
		changed[path] = struct{}{}
	}
	var missingCriteria []string
	missingPaths := make(map[string]struct{})
	var substituteFailures []string
	honored := make(map[string]string)
	for _, requirement := range requirements {
		satisfied := false
		for _, path := range requirement.paths {
			if _, touched := changed[path]; touched {
				satisfied = true
				break
			}
		}
		if !satisfied {
			if substitute, declared := substitutes[requirement.id]; declared && substitute != "" {
				if _, touched := changed[substitute]; !touched {
					substituteFailures = append(substituteFailures,
						requirement.id+": declared substitute "+substitute+" is untouched")
				} else if !anchorPathInScope(contract.Scope, substitute) {
					substituteFailures = append(substituteFailures,
						requirement.id+": declared substitute "+substitute+" is outside the slice's approved scope")
				} else {
					satisfied = true
					honored[requirement.id] = substitute
				}
			}
		}
		if !satisfied {
			missingCriteria = append(missingCriteria, requirement.id)
			for _, path := range requirement.paths {
				missingPaths[path] = struct{}{}
			}
		}
	}
	if len(missingCriteria) == 0 {
		return honored, nil
	}
	paths := make([]string, 0, len(missingPaths))
	for path := range missingPaths {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	sort.Strings(missingCriteria)
	sort.Strings(substituteFailures)
	total := len(paths)
	bounded := paths
	if len(bounded) > 20 {
		bounded = append([]string(nil), bounded[:20]...)
	}
	msg := "criteria " + strings.Join(missingCriteria, ", ") +
		" touch none of their declared anchor files (anchor base " + base + ")"
	if len(substituteFailures) > 0 {
		msg += "; " + strings.Join(substituteFailures, "; ")
	}
	return nil, &protocol.RecordError{
		Code:       "ANCHOR_NOT_TOUCHED",
		Msg:        msg,
		Paths:      bounded,
		TotalPaths: total,
	}
}
