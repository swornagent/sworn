package baton

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

const (
	// CheckResultsVersion identifies the engine-built check-evidence manifest
	// that becomes the receipt Checks bytes for slices whose contract declares
	// host_checks. It is strict JSON with per-entry provenance, so host-boundary
	// evidence can never be mistaken for role-produced evidence.
	CheckResultsVersion = "sworn.check-results/v1"
	// HostCheckOutputManifestBytes bounds how much of one host check's bounded
	// output the manifest itself carries. The full bounded output (up to the
	// runner's output cap) stays in the journaled check.host effect payload;
	// the manifest carries its digest plus this bounded excerpt, so several
	// host checks can never push the manifest past the evidence cap.
	HostCheckOutputManifestBytes = 4096
	// HostCheckTruncationPrefix is the truthful marker appended when a
	// command's output was truncated at the runner's output cap. An entry
	// whose output contains this marker is never read as the full output.
	HostCheckTruncationPrefix = "[sworn: output truncated"
	// MaxCheckCommandBytes bounds one entry's Check field. Exported so a
	// producer can truncate a long recorded command before encoding rather
	// than let it poison the whole manifest with an opaque encode failure.
	MaxCheckCommandBytes = 2_048
)

const (
	CheckProvenanceHost = "host_boundary"
	CheckProvenanceRole = "role"
)

const (
	CheckOutcomePass     = "pass"
	CheckOutcomeFail     = "fail"
	CheckOutcomeTimeout  = "timeout"
	CheckOutcomeOverflow = "overflow"
)

var checkProvenances = map[string]bool{
	CheckProvenanceHost: true,
	CheckProvenanceRole: true,
}

var checkOutcomes = map[string]bool{
	CheckOutcomePass:     true,
	CheckOutcomeFail:     true,
	CheckOutcomeTimeout:  true,
	CheckOutcomeOverflow: true,
}

// CheckResultEntry is one declared check's recorded evidence. Provenance is
// structural: host_boundary entries were executed by the engine at the host
// boundary and journaled; role entries reference exact bytes a role itself
// submitted. The two are mutually exclusive by field shape, so a manifest
// cannot launder one into the other.
type CheckResultEntry struct {
	Check string `json:"check"`
	// Provenance and Outcome are exactly what was observed, never an
	// inference: Outcome is the exit status of the recorded command as it
	// actually ran, so a pipeline (`check | head`) or `check || true` that
	// masks a failing check's real exit code still records pass here - the
	// manifest reports what the sandbox observed, never what the check
	// "really" did.
	Provenance   string `json:"provenance"`
	Outcome      string `json:"outcome"`
	ExitCode     *int   `json:"exit_code,omitempty"`
	OutputDigest string `json:"output_digest,omitempty"`
	Output       string `json:"output,omitempty"`
	Truncated    bool   `json:"truncated,omitempty"`
	Diagnostic   string `json:"diagnostic,omitempty"`
	RoleDigest   string `json:"role_digest,omitempty"`
	HostEffect   string `json:"host_effect,omitempty"`
}

// CheckResults is the admitted sworn.check-results/v1 manifest. It binds one
// exact slice, attempt, candidate and contract digest to its recorded check
// entries. ParseCheckResults proves the manifest is well-formed and
// internally consistent; matching each entry's digests against the real
// journaled effects and role-submitted bytes happens where those bytes
// actually live (the engine), never by inference here.
type CheckResults struct {
	SchemaVersion  string             `json:"schema_version"`
	Release        string             `json:"release"`
	Slice          string             `json:"slice"`
	Attempt        int64              `json:"attempt"`
	Candidate      string             `json:"candidate"`
	ContractDigest string             `json:"contract_digest"`
	Entries        []CheckResultEntry `json:"entries"`
}

// EncodeCheckResults builds canonical manifest bytes for an exact binding and
// proves they re-admit before returning them, so the engine can never bind a
// malformed manifest into a receipt.
func EncodeCheckResults(value CheckResults) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, recordWrap("INVALID_JSON", "encode check results", err)
	}
	if _, err := ParseCheckResults(raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// ParseCheckResults admits one sworn.check-results/v1 manifest, failing closed
// on any entry whose provenance is absent, unknown, or inconsistent with its
// other fields. Host entries must carry exit_code, output_digest and
// host_effect and must not carry role_digest; role entries must carry at
// least one of role_digest, output or diagnostic, and must not carry any
// host-only field. When a host entry embeds its full (non-truncated) output,
// the output_digest must match it exactly; a truncated entry must carry the
// truthful truncation marker and is never read as full output.
//
// Release, slice, attempt, candidate and contract_digest stay structurally
// required keys (the shape stays self-describing), but each admits its own
// zero value (empty string, or 0 for attempt) as "not asserted by the
// producer": a manifest a driver mints in-turn has no authoritative source
// for these and must never guess one. Each is still validated in its full
// exact form whenever it is non-zero, so a manifest that does assert every
// field (buildHostCheckResultsManifest's own host-boundary manifests always
// do) parses exactly as it always has.
func ParseCheckResults(raw []byte) (CheckResults, error) {
	if len(raw) > MaxEvidenceBytes {
		return CheckResults{}, recordFail("RESOURCE_LIMIT", fmt.Sprintf("check results exceed %d bytes", MaxEvidenceBytes))
	}
	value, err := strictParseJSON(raw, "check results", MaxEvidenceBytes)
	if err != nil {
		return CheckResults{}, err
	}
	object, err := asObject(value, "check results")
	if err != nil {
		return CheckResults{}, err
	}
	required := []string{
		"schema_version", "release", "slice", "attempt", "candidate",
		"contract_digest", "entries",
	}
	if err := exactKeys(object, required, nil, "check results"); err != nil {
		return CheckResults{}, err
	}
	schema, err := requiredString(object["schema_version"], "check_results.schema_version", 1, 100)
	if err != nil {
		return CheckResults{}, err
	}
	if schema != CheckResultsVersion {
		return CheckResults{}, recordFail("INVALID_FIELD", "check_results.schema_version must be "+CheckResultsVersion)
	}
	release, err := identityOrZero(object["release"], "check_results.release")
	if err != nil {
		return CheckResults{}, err
	}
	slice, err := identityOrZero(object["slice"], "check_results.slice")
	if err != nil {
		return CheckResults{}, err
	}
	attempt, err := safeInteger(object["attempt"], "check_results.attempt", 0)
	if err != nil {
		return CheckResults{}, err
	}
	candidate, err := objectIDOrZero(object["candidate"], "check_results.candidate")
	if err != nil {
		return CheckResults{}, err
	}
	contractDigest, err := digestStringOrZero(object["contract_digest"], "check_results.contract_digest")
	if err != nil {
		return CheckResults{}, err
	}
	rawEntries, err := asArray(object["entries"], "check_results.entries", true, MaxListItems)
	if err != nil {
		return CheckResults{}, err
	}
	entries := make([]CheckResultEntry, 0, len(rawEntries))
	for index, rawEntry := range rawEntries {
		label := fmt.Sprintf("check_results.entries[%d]", index)
		entry, err := parseCheckResultEntry(rawEntry, label)
		if err != nil {
			return CheckResults{}, err
		}
		entries = append(entries, entry)
	}
	return CheckResults{
		SchemaVersion: schema, Release: release, Slice: slice,
		Attempt: attempt, Candidate: candidate, ContractDigest: contractDigest,
		Entries: entries,
	}, nil
}

func parseCheckResultEntry(value any, label string) (CheckResultEntry, error) {
	object, err := asObject(value, label)
	if err != nil {
		return CheckResultEntry{}, err
	}
	optional := []string{
		"exit_code", "output_digest", "output", "truncated",
		"diagnostic", "role_digest", "host_effect",
	}
	if err := exactKeys(object, []string{"check", "provenance", "outcome"}, optional, label); err != nil {
		return CheckResultEntry{}, err
	}
	check, err := requiredString(object["check"], label+".check", 1, MaxCheckCommandBytes)
	if err != nil {
		return CheckResultEntry{}, err
	}
	provenance, err := requiredString(object["provenance"], label+".provenance", 1, 64)
	if err != nil {
		return CheckResultEntry{}, err
	}
	if !checkProvenances[provenance] {
		return CheckResultEntry{}, recordFail("INVALID_FIELD", label+".provenance must be host_boundary or role")
	}
	outcome, err := requiredString(object["outcome"], label+".outcome", 1, 64)
	if err != nil {
		return CheckResultEntry{}, err
	}
	if !checkOutcomes[outcome] {
		return CheckResultEntry{}, recordFail("INVALID_FIELD", label+".outcome must be one of pass, fail, timeout, overflow")
	}
	entry := CheckResultEntry{Check: check, Provenance: provenance, Outcome: outcome}
	if raw, ok := object["truncated"]; ok {
		flag, ok := raw.(bool)
		if !ok {
			return CheckResultEntry{}, recordFail("INVALID_FIELD", label+".truncated must be a boolean")
		}
		entry.Truncated = flag
	}
	if raw, ok := object["output"]; ok {
		output, err := asString(raw, label+".output", 0, MaxEvidenceBytes)
		if err != nil {
			return CheckResultEntry{}, err
		}
		entry.Output = output
	}
	if raw, ok := object["diagnostic"]; ok {
		diagnostic, err := asString(raw, label+".diagnostic", 1, 4_096)
		if err != nil {
			return CheckResultEntry{}, err
		}
		entry.Diagnostic = diagnostic
	}
	switch provenance {
	case CheckProvenanceHost:
		if entry.Truncated && outcome == CheckOutcomePass {
			return CheckResultEntry{}, recordFail("INVALID_FIELD", label+" truncated output cannot be a pass")
		}
		rawExit, ok := object["exit_code"]
		if !ok {
			return CheckResultEntry{}, recordFail("MISSING_FIELD", label+" host entry requires exit_code")
		}
		exitCode, err := safeInteger(rawExit, label+".exit_code", -256)
		if err != nil {
			return CheckResultEntry{}, err
		}
		exit := int(exitCode)
		entry.ExitCode = &exit
		rawDigest, ok := object["output_digest"]
		if !ok {
			return CheckResultEntry{}, recordFail("MISSING_FIELD", label+" host entry requires output_digest")
		}
		outputDigest, err := digestString(rawDigest, label+".output_digest")
		if err != nil {
			return CheckResultEntry{}, err
		}
		entry.OutputDigest = outputDigest
		rawEffect, ok := object["host_effect"]
		if !ok {
			return CheckResultEntry{}, recordFail("MISSING_FIELD", label+" host entry requires host_effect")
		}
		hostEffect, err := requiredString(rawEffect, label+".host_effect", 1, 512)
		if err != nil {
			return CheckResultEntry{}, err
		}
		entry.HostEffect = hostEffect
		if _, ok := object["role_digest"]; ok {
			return CheckResultEntry{}, recordFail("INVALID_FIELD", label+" host entry cannot carry role_digest")
		}
		if entry.Output != "" {
			if entry.Truncated {
				if !strings.Contains(entry.Output, HostCheckTruncationPrefix) {
					return CheckResultEntry{}, recordFail(
						"INVALID_FIELD",
						label+" truncated output must carry the truthful truncation marker",
					)
				}
			} else if DigestBytes([]byte(entry.Output)) != entry.OutputDigest {
				return CheckResultEntry{}, recordFail(
					"STALE_BINDING",
					label+" output_digest does not match its embedded output",
				)
			}
		}
	case CheckProvenanceRole:
		// A role entry's outcome need not be pass - a verifier that ran a
		// declared check and watched it fail must be able to record that
		// honestly rather than fabricate a pass, provided it says why.
		if outcome != CheckOutcomePass && entry.Diagnostic == "" {
			return CheckResultEntry{}, recordFail(
				"MISSING_FIELD",
				label+" role entry with a non-pass outcome requires diagnostic",
			)
		}
		// role_digest covers the full recorded (redacted) output, never
		// only the bounded Output excerpt; a cut excerpt is a truncation of
		// display content, not of what role_digest attests to.
		if entry.Truncated {
			if entry.Output == "" || !strings.Contains(entry.Output, HostCheckTruncationPrefix) {
				return CheckResultEntry{}, recordFail(
					"INVALID_FIELD",
					label+" truncated output must carry the truthful truncation marker",
				)
			}
		}
		if raw, ok := object["role_digest"]; ok {
			roleDigest, err := digestString(raw, label+".role_digest")
			if err != nil {
				return CheckResultEntry{}, err
			}
			entry.RoleDigest = roleDigest
		}
		if entry.RoleDigest == "" && entry.Output == "" && entry.Diagnostic == "" {
			return CheckResultEntry{}, recordFail(
				"MISSING_FIELD",
				label+" role entry requires role_digest, output, or diagnostic",
			)
		}
		for _, forbidden := range []string{
			"exit_code", "output_digest", "host_effect",
		} {
			if _, present := object[forbidden]; present {
				return CheckResultEntry{}, recordFail(
					"INVALID_FIELD",
					label+" role entry cannot carry "+forbidden,
				)
			}
		}
	default:
		return CheckResultEntry{}, recordFail("INVALID_FIELD", label+".provenance is invalid")
	}
	return entry, nil
}

// identityOrZero, objectIDOrZero and digestStringOrZero admit the empty
// string as "not asserted" beside their normal identity/objectID/digestString
// exact form, for the five check-results binding fields a driver-built
// manifest may leave unasserted (ParseCheckResults's own doc comment).
func identityOrZero(value any, label string) (string, error) {
	if text, ok := value.(string); ok && text == "" {
		return "", nil
	}
	return identity(value, label)
}

func objectIDOrZero(value any, label string) (string, error) {
	if text, ok := value.(string); ok && text == "" {
		return "", nil
	}
	return objectID(value, label)
}

func digestStringOrZero(value any, label string) (string, error) {
	if text, ok := value.(string); ok && text == "" {
		return "", nil
	}
	return digestString(value, label)
}

// CheckCommandCovers reports whether recorded is an actual sandboxed run of
// the declared check string: PATH=... is sandbox plumbing and is dropped, a
// leading run of KEY=VALUE assignments is kept verbatim (they are part of a
// declared check's own identity, e.g. GOFLAGS=), the first remaining token -
// the binary - is compared by base name so an absolute path invocation (the
// only way to reach an off-PATH toolchain under a sandbox that does not put
// it on PATH) still matches a bare-name declared check, and every following
// declared token must appear verbatim at the same position. Trailing tokens
// after the declared check's own length are never compared, so an ordinary
// redirect-to-a-file-and-tail wrapper does not defeat coverage.
func CheckCommandCovers(declared, recorded string) bool {
	declaredTokens := strings.Fields(declared)
	recordedTokens := strings.Fields(recorded)
	if len(declaredTokens) == 0 || len(recordedTokens) < len(declaredTokens) {
		return false
	}
	if recordedTokens[0] == "" {
		return false
	}
	if strings.HasPrefix(recordedTokens[0], "PATH=") {
		recordedTokens = recordedTokens[1:]
		if len(recordedTokens) < len(declaredTokens) {
			return false
		}
	}
	index := 0
	for index < len(recordedTokens) && index < len(declaredTokens) &&
		isEnvAssignment(declaredTokens[index]) &&
		recordedTokens[index] == declaredTokens[index] {
		index++
	}
	if index >= len(recordedTokens) || index >= len(declaredTokens) {
		return false
	}
	if filepath.Base(recordedTokens[index]) != declaredTokens[index] {
		return false
	}
	index++
	for index < len(declaredTokens) {
		if index >= len(recordedTokens) || recordedTokens[index] != declaredTokens[index] {
			return false
		}
		index++
	}
	return true
}

// isEnvAssignment reports whether token has the shape of a leading
// environment-variable assignment (KEY=..., KEY drawn from
// [A-Za-z_][A-Za-z0-9_]*), applied only to leading declared tokens so a
// check with no env prefix (e.g. gofmt -l ...) is unaffected.
func isEnvAssignment(token string) bool {
	equals := strings.IndexByte(token, '=')
	if equals <= 0 {
		return false
	}
	for index := 0; index < equals; index++ {
		char := token[index]
		letter := (char >= 'A' && char <= 'Z') || (char >= 'a' && char <= 'z') || char == '_'
		digit := char >= '0' && char <= '9'
		if !letter && !(digit && index > 0) {
			return false
		}
	}
	return true
}
