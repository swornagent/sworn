package baton

import (
	"encoding/json"
	"fmt"
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
	Check        string `json:"check"`
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
// host_effect and must not carry role_digest; role entries must carry
// role_digest and must not carry any host-only field. When a host entry
// embeds its full (non-truncated) output, the output_digest must match it
// exactly; a truncated entry must carry the truthful truncation marker and is
// never read as full output.
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
	release, err := identity(object["release"], "check_results.release")
	if err != nil {
		return CheckResults{}, err
	}
	slice, err := identity(object["slice"], "check_results.slice")
	if err != nil {
		return CheckResults{}, err
	}
	attempt, err := safeInteger(object["attempt"], "check_results.attempt", 1)
	if err != nil {
		return CheckResults{}, err
	}
	candidate, err := objectID(object["candidate"], "check_results.candidate")
	if err != nil {
		return CheckResults{}, err
	}
	contractDigest, err := digestString(object["contract_digest"], "check_results.contract_digest")
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
	check, err := requiredString(object["check"], label+".check", 1, 2_048)
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
		if entry.Truncated {
			return CheckResultEntry{}, recordFail("INVALID_FIELD", label+" role entry cannot be truncated")
		}
		if outcome != CheckOutcomePass {
			return CheckResultEntry{}, recordFail("INVALID_FIELD", label+" role entry outcome must be pass")
		}
		rawDigest, ok := object["role_digest"]
		if !ok {
			return CheckResultEntry{}, recordFail("MISSING_FIELD", label+" role entry requires role_digest")
		}
		roleDigest, err := digestString(rawDigest, label+".role_digest")
		if err != nil {
			return CheckResultEntry{}, err
		}
		entry.RoleDigest = roleDigest
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
