package driver

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/swornagent/sworn/internal/baton"
)

// S5-A3: check evidence carries provenance. Every Bash call a verifier's
// session runs is recorded into a bounded accumulator; at submit time, for a
// WorkVerification responsibility, the accumulator - never whatever the
// model itself wrote into its own checks field - becomes the sworn.check-
// results/v1 manifest bound into the receipt. A pass claim that a declared
// check has no covering recorded pass is refused in-turn, naming the check
// and the matching rule, so a worker can act on the refusal inside the same
// turn instead of losing it silently to a post-turn backstop.

const (
	// checkEvidenceEntryLimit matches baton.MaxListItems, the manifest's own
	// entries-array bound.
	checkEvidenceEntryLimit = baton.MaxListItems
	// checkEvidenceByteBudget is a running total ceiling well under
	// baton.MaxCheckBytes/MaxEvidenceBytes (1,048,576), leaving headroom for
	// the JSON envelope and the outer submission's own share of the same
	// cap once base64-encoded.
	checkEvidenceByteBudget      = 512 * 1024
	checkCommandTruncationMarker = "...[truncated]"
)

// recordCheckEvidence appends one baton.CheckResultEntry for a completed
// Bash call. It never holds session.mu while redacting: redactionSecrets
// itself locks session.mu, so redacting first and locking only to append
// avoids a self-deadlock on the session's own (non-reentrant) mutex.
func (session *toolSession) recordCheckEvidence(
	script string, output []byte, code int, runErr error,
) {
	outcome, diagnostic := checkResultOutcome(code, runErr)
	redacted, _ := redactToolResultSpan(output, session.redactionSecrets())
	roleDigest := baton.DigestBytes(redacted)
	excerpt, truncated := boundedCheckExcerpt(redacted)
	check := script
	if len(check) > baton.MaxCheckCommandBytes {
		check = truncateCheckCommand(check)
	}
	entry := baton.CheckResultEntry{
		Check:      check,
		Provenance: baton.CheckProvenanceRole,
		Outcome:    outcome,
		Diagnostic: diagnostic,
		RoleDigest: roleDigest,
		Output:     excerpt,
		Truncated:  truncated,
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.closed {
		return
	}
	session.appendCheckEvidenceLocked(entry)
}

// appendCheckEvidenceLocked evicts the oldest recorded entry first whenever
// the new one would cross either bound, so the declared checks a verifier
// runs last are never the ones dropped.
func (session *toolSession) appendCheckEvidenceLocked(entry baton.CheckResultEntry) {
	size := checkEvidenceEntrySize(entry)
	if size > checkEvidenceByteBudget {
		return
	}
	for len(session.checkEvidence) > 0 &&
		(len(session.checkEvidence) >= checkEvidenceEntryLimit ||
			session.checkEvidenceBytes+size > checkEvidenceByteBudget) {
		removed := session.checkEvidence[0]
		session.checkEvidence = session.checkEvidence[1:]
		session.checkEvidenceBytes -= checkEvidenceEntrySize(removed)
	}
	session.checkEvidence = append(session.checkEvidence, entry)
	session.checkEvidenceBytes += size
}

func checkEvidenceEntrySize(entry baton.CheckResultEntry) int {
	encoded, err := json.Marshal(entry)
	if err != nil {
		return 0
	}
	return len(encoded)
}

func (session *toolSession) snapshotCheckEvidence() []baton.CheckResultEntry {
	session.mu.Lock()
	defer session.mu.Unlock()
	return append([]baton.CheckResultEntry(nil), session.checkEvidence...)
}

// checkResultOutcome classifies one Bash call's result exactly as observed:
// Outcome is the exit status of the recorded command as it actually ran
// under sh -eu (tools_linux.go), never an inference about what the check
// "really" did. A nonzero exit with a nil error means the sandboxed command
// ran to completion and said no - a fact, not a harness fault - and records
// fail. Any harness-side failure (a killed group, a sandbox-start failure)
// also records fail, since from a check-evidence standpoint the command did
// not complete either way and a role entry may never claim pass for a check
// that did not run to a real exit.
func checkResultOutcome(code int, runErr error) (outcome, diagnostic string) {
	if runErr == nil {
		if code == 0 {
			return baton.CheckOutcomePass, ""
		}
		return baton.CheckOutcomeFail, fmt.Sprintf("exited %d", code)
	}
	if isContextError(runErr) {
		return baton.CheckOutcomeTimeout, runErr.Error()
	}
	if IsCode(runErr, "OUTPUT_OVERFLOW") {
		return baton.CheckOutcomeOverflow, "OUTPUT_OVERFLOW"
	}
	if name := contractErrorCode(runErr); name != "" {
		return baton.CheckOutcomeFail, name
	}
	return baton.CheckOutcomeFail, "harness error"
}

func contractErrorCode(err error) string {
	var contractErr *ContractError
	if errors.As(err, &contractErr) {
		return contractErr.Code
	}
	return ""
}

// boundedCheckExcerpt bounds a redacted output to HostCheckOutputManifestBytes,
// the manifest's own existing display-excerpt convention, carrying the
// truthful truncation marker when it cuts. Invalid UTF-8 (Bash output is
// arbitrary bytes; the manifest's output field must be valid UTF-8) is
// replaced, never silently dropped mid-rune.
func boundedCheckExcerpt(redacted []byte) (excerpt string, truncated bool) {
	if len(redacted) == 0 {
		return "", false
	}
	if len(redacted) <= baton.HostCheckOutputManifestBytes {
		return strings.ToValidUTF8(string(redacted), "�"), false
	}
	cut := redacted[:baton.HostCheckOutputManifestBytes]
	marked := baton.HostCheckTruncationPrefix + " at " +
		strconv.Itoa(baton.HostCheckOutputManifestBytes) + " bytes]\n" + string(cut)
	return strings.ToValidUTF8(marked, "�"), true
}

// truncateCheckCommand bounds a recorded command to baton.MaxCheckCommandBytes,
// keeping the head: CheckCommandCovers matches a declared check as a prefix
// of the recorded command, so preserving the head keeps coverage possible
// even for a script far longer than the manifest's own per-entry bound.
func truncateCheckCommand(script string) string {
	limit := baton.MaxCheckCommandBytes - len(checkCommandTruncationMarker)
	if limit < 0 {
		limit = 0
	}
	cut := script
	if len(cut) > limit {
		cut = cut[:limit]
		for len(cut) > 0 && !utf8.ValidString(cut) {
			cut = cut[:len(cut)-1]
		}
	}
	return cut + checkCommandTruncationMarker
}

// verifierContractBinding is what the driver can resolve, from data it
// already receives, about the slice a WorkVerification submission concerns.
// checksResolved distinguishes "resolution succeeded" from a zero-value
// checks list: only when true does the in-turn completeness gate act: a
// structural resolution hiccup here never blocks a worker, because the
// durable appendReceipt backstop remains the source of truth regardless.
type verifierContractBinding struct {
	release        string
	slice          string
	attempt        int64
	contractDigest string
	checks         []string
	hostChecks     bool
	checksResolved bool
}

// resolveVerifierContractBinding resolves the declared checks and binding
// identity for this invocation's slice entirely from data internal/driver
// already receives: work-context.json and plan.md are already
// Invocation.Inputs for every baton-dispatched role (production_dispatch.go),
// and the contract bytes are read directly from the checked-out workspace
// tree at the path the plan declares. No new Invocation field, no
// internal/runtime change, no repository handle - baton.ParsePlan and
// Plan.ResolveSliceContract are pure over already-admitted bytes.
func resolveVerifierContractBinding(invocation Invocation) verifierContractBinding {
	var context struct {
		Release string `json:"release"`
		Slice   string `json:"slice"`
		Attempt int64  `json:"attempt"`
	}
	var planBytes []byte
	for _, input := range invocation.Inputs {
		switch input.Input.Name {
		case "work-context":
			_ = json.Unmarshal(input.Bytes, &context)
		case "plan":
			planBytes = input.Bytes
		}
	}
	binding := verifierContractBinding{
		release: context.Release, slice: context.Slice, attempt: context.Attempt,
	}
	if context.Slice == "" || planBytes == nil {
		return binding
	}
	plan, err := baton.ParsePlan(planBytes)
	if err != nil {
		return binding
	}
	_, declared, found := plan.FindSlice(context.Slice)
	if !found {
		return binding
	}
	if digest, ok := plan.Contract(context.Slice); ok {
		binding.contractDigest = digest
	}
	resolvedSlice := declared
	if declared.ContractPath != "" {
		raw, err := readPinnedWorkspaceFile(invocation.HostWorkspace, declared.ContractPath)
		if err != nil {
			return binding
		}
		resolvedSlice, err = plan.ResolveSliceContract(context.Slice, raw)
		if err != nil {
			return binding
		}
	}
	binding.checks = resolvedSlice.Checks
	binding.hostChecks = len(resolvedSlice.HostChecks) != 0
	binding.checksResolved = true
	return binding
}

// readPinnedWorkspaceFile reads a workspace-relative path's exact bytes,
// refusing anything but a plain regular file that stays under root - the
// same discipline native.go's openPinnedRuntimeFile applies for a different
// purpose - and bounding the read at baton.MaxPlanBytes.
func readPinnedWorkspaceFile(root, relative string) ([]byte, error) {
	if relative == "" || filepath.IsAbs(relative) {
		return nil, fail("INVALID_CONTRACT_PATH")
	}
	cleaned := filepath.Clean(relative)
	if cleaned != relative || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return nil, fail("INVALID_CONTRACT_PATH")
	}
	rootClean := filepath.Clean(root)
	full := filepath.Join(rootClean, cleaned)
	if full != rootClean && !strings.HasPrefix(full, rootClean+string(filepath.Separator)) {
		return nil, fail("INVALID_CONTRACT_PATH")
	}
	info, err := os.Lstat(full)
	if err != nil || !info.Mode().IsRegular() {
		return nil, fail("INVALID_CONTRACT_PATH")
	}
	file, err := os.Open(full)
	if err != nil {
		return nil, fail("INVALID_CONTRACT_PATH")
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, baton.MaxPlanBytes+1))
	if err != nil || int64(len(body)) > baton.MaxPlanBytes {
		return nil, fail("INVALID_CONTRACT_PATH")
	}
	return body, nil
}

// firstUncoveredCheck returns the first declared check with no recorded
// entry that both matches it (baton.CheckCommandCovers) and observed pass.
func firstUncoveredCheck(
	declaredChecks []string, entries []baton.CheckResultEntry,
) (check string, incomplete bool) {
	for _, declared := range declaredChecks {
		covered := false
		for _, entry := range entries {
			if entry.Outcome == baton.CheckOutcomePass &&
				baton.CheckCommandCovers(declared, entry.Check) {
				covered = true
				break
			}
		}
		if !covered {
			return declared, true
		}
	}
	return "", false
}

// applyCheckEvidence is session.submit's S5-A3 seam for a WorkVerification
// submission: if the pass claim's declared checks are resolvable and none
// declares host_checks, an uncovered declared check refuses the submission
// in-turn (session.submit keeps the session alive via rejectSubmission), and
// otherwise the accumulator - never the model's own submitted checks bytes -
// becomes the manifest bound into submission.Checks. An empty accumulator
// (no Bash call ran this turn) leaves submission.Checks untouched: the
// driver has no evidence to report and must never guess one, and a pass
// claim with no evidence is already refused by the completeness gate above.
func (session *toolSession) applyCheckEvidence(submission *Submission) error {
	entries := session.snapshotCheckEvidence()
	binding := resolveVerifierContractBinding(session.invocation)
	if binding.checksResolved && !binding.hostChecks &&
		submission.Decision != nil && submission.Decision.Outcome == DecisionPass {
		if check, incomplete := firstUncoveredCheck(binding.checks, entries); incomplete {
			return submitCheckEvidenceIncompleteError(check)
		}
	}
	if len(entries) == 0 {
		return nil
	}
	results := baton.CheckResults{
		SchemaVersion:  baton.CheckResultsVersion,
		Release:        binding.release,
		Slice:          binding.slice,
		Attempt:        binding.attempt,
		ContractDigest: binding.contractDigest,
		Entries:        entries,
	}
	encoded, err := baton.EncodeCheckResults(results)
	if err != nil {
		return submitCheckEvidenceEncodeError(err)
	}
	checkBytes, err := NewCheckBytes(encoded)
	if err != nil {
		return submitCheckEvidenceEncodeError(err)
	}
	submission.Checks = checkBytes
	return nil
}
