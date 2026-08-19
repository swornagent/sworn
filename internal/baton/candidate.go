package baton

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
)

func releaseRef(release string) string {
	return "refs/heads/release-wt/" + release
}

func trackRef(release, track string) string {
	return "refs/heads/track/" + release + "/" + track
}

func planPath(recordRoot, release string) string {
	return recordRoot + "/" + release + "/plan.md"
}

func validateObjectForFormat(format, value, label string) error {
	length := 40
	if format == "sha256" {
		length = 64
	}
	if len(value) != length {
		return recordFail("OBJECT_FORMAT_MISMATCH", fmt.Sprintf("%s must contain %d lowercase hex characters", label, length))
	}
	if !objectIDPattern.MatchString(value) {
		return recordFail("INVALID_OBJECT_ID", label+" is not one full lowercase Git object identity")
	}
	return nil
}

func exactInputs(receipt Receipt, keys []string, label string) error {
	observed := sortedInputKeys(receipt.Inputs)
	expected := append([]string(nil), keys...)
	sort.Slice(expected, func(i, j int) bool {
		return bytes.Compare([]byte(expected[i]), []byte(expected[j])) < 0
	})
	if len(observed) != len(expected) {
		return recordFail("STALE_BINDING", label+" input keys do not match the plan")
	}
	for index := range observed {
		if observed[index] != expected[index] {
			return recordFail("STALE_BINDING", label+" input keys do not match the plan")
		}
	}
	return nil
}

func inputsEqual(left, right map[string]string) bool {
	if len(left) != len(right) || (left == nil) != (right == nil) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}

func sameCandidate(left, right Receipt) bool {
	return left.Candidate != nil && right.Candidate != nil &&
		left.ProductTree != nil && right.ProductTree != nil &&
		*left.Candidate == *right.Candidate &&
		*left.ProductTree == *right.ProductTree &&
		inputsEqual(left.Inputs, right.Inputs)
}

func productTreeFor(
	repository *repository,
	candidate string,
	cache map[string]string,
) (string, error) {
	if value, ok := cache[candidate]; ok {
		return value, nil
	}
	if err := validateObjectForFormat(repository.objectFormat(), candidate, "candidate"); err != nil {
		return "", err
	}
	value, err := repository.productTree(candidate)
	if err != nil {
		return "", recordWrap(ErrorCode(err), "cannot validate candidate "+candidate, err)
	}
	cache[candidate] = value
	return value, nil
}

func verifyReleaseIntegration(repository *repository, expected, candidate, result string) error {
	for label, value := range map[string]string{
		"expected target": expected, "candidate": candidate, "result": result,
	} {
		if err := validateObjectForFormat(repository.objectFormat(), value, label); err != nil {
			return err
		}
	}
	if err := repository.verifyComposition(expected, candidate, result); err != nil {
		return err
	}
	return nil
}

func pathInScope(scope Scope, changedPath string) bool {
	included := false
	for _, include := range scope.Include {
		if changedPath == include || strings.HasPrefix(changedPath, include+"/") {
			included = true
			break
		}
	}
	if !included {
		return false
	}
	for _, exclude := range scope.Exclude {
		if changedPath == exclude || strings.HasPrefix(changedPath, exclude+"/") {
			return false
		}
	}
	return true
}

// ValidateSliceCandidateScope is a read-only engine guard for the bounded
// workspace seam. Baton receipts remain derived authority; this helper lets
// the runtime prove that a model-produced candidate changed only its plan
// scope before AppendReceipt is called.
func ValidateSliceCandidateScope(
	gitRepository GitRepository,
	resolver InertnessResolver,
	plan Plan,
	sliceID, base, candidate string,
) error {
	repository, err := newRepository(gitRepository.repository(), resolver)
	if err != nil {
		return err
	}
	_, slice, ok := plan.FindSlice(sliceID)
	if !ok {
		return recordFail("SLICE_NOT_FOUND", "plan has no current slice "+sliceID)
	}
	ancestor, err := repository.isAncestor(base, candidate)
	if err != nil {
		return err
	}
	if !ancestor {
		return recordFail("INVALID_CANDIDATE_ANCESTRY", "candidate does not descend from its bound base")
	}
	if err := repository.assertCandidateRecordRootUnchanged(base, candidate); err != nil {
		return err
	}
	paths, err := repository.changedPaths(base, candidate)
	if err != nil {
		return err
	}
	// The reserved-root check follows the configured records root and the
	// historical legacy root so a relocated root is never left unprotected
	// and the legacy fallback can never be forged.
	reservedRoot := repository.recordRoot()
	var reservedPaths []string
	var outOfScopePaths []string
	for _, changedPath := range paths {
		if changedPath == reservedRoot || strings.HasPrefix(changedPath, reservedRoot+"/") ||
			changedPath == LegacyRecordRoot || strings.HasPrefix(changedPath, LegacyRecordRoot+"/") {
			reservedPaths = append(reservedPaths, changedPath)
			continue
		}
		if !pathInScope(slice.Scope, changedPath) {
			outOfScopePaths = append(outOfScopePaths, changedPath)
		}
	}
	if len(reservedPaths) > 0 {
		sort.Strings(reservedPaths)
		deduped := make([]string, 0, len(reservedPaths))
		for i, p := range reservedPaths {
			if i == 0 || p != reservedPaths[i-1] {
				deduped = append(deduped, p)
			}
		}
		total := len(deduped)
		bounded := deduped
		if len(bounded) > 20 {
			bounded = append([]string(nil), bounded[:20]...)
		}
		return &RecordError{
			Code:       "RESERVED_RECORD_ROOT_CHANGED",
			Msg:        fmt.Sprintf("candidate changes %d reserved Baton record path(s)", total),
			Paths:      bounded,
			TotalPaths: total,
		}
	}
	if len(outOfScopePaths) > 0 {
		sort.Strings(outOfScopePaths)
		deduped := make([]string, 0, len(outOfScopePaths))
		for i, p := range outOfScopePaths {
			if i == 0 || p != outOfScopePaths[i-1] {
				deduped = append(deduped, p)
			}
		}
		total := len(deduped)
		bounded := deduped
		if len(bounded) > 20 {
			bounded = append([]string(nil), bounded[:20]...)
		}
		return &RecordError{
			Code:       "SLICE_OUTSIDE_SCOPE",
			Msg:        fmt.Sprintf("candidate changes %d out-of-scope path(s)", total),
			Paths:      bounded,
			TotalPaths: total,
		}
	}
	return nil
}
