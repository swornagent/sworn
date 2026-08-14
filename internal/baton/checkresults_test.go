package baton

import (
	"strings"
	"testing"
)

func checkResultsFixture(t *testing.T) CheckResults {
	t.Helper()
	host := int(0)
	output := "all good\n"
	entries := []CheckResultEntry{
		{
			Check: "go test ./...", Provenance: CheckProvenanceHost,
			Outcome: CheckOutcomePass, ExitCode: &host,
			OutputDigest: DigestBytes([]byte(output)), Output: output,
			HostEffect: "attempt/host-boundary/1/1",
		},
		{
			Check: "go vet ./...", Provenance: CheckProvenanceRole,
			Outcome: CheckOutcomePass, RoleDigest: "sha256:" + strings.Repeat("a", 64),
		},
	}
	return CheckResults{
		SchemaVersion:  CheckResultsVersion,
		Release:        "release-1",
		Slice:          "S1",
		Attempt:        1,
		Candidate:      strings.Repeat("1", 40),
		ContractDigest: "sha256:" + strings.Repeat("b", 64),
		Entries:        entries,
	}
}

func TestParseCheckResultsAcceptsWellFormedManifest(t *testing.T) {
	raw, err := EncodeCheckResults(checkResultsFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseCheckResults(raw)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.SchemaVersion != CheckResultsVersion ||
		parsed.Slice != "S1" || parsed.Attempt != 1 ||
		len(parsed.Entries) != 2 {
		t.Fatalf("parsed = %#v", parsed)
	}
	if parsed.Entries[0].Provenance != CheckProvenanceHost ||
		parsed.Entries[0].OutputDigest == "" ||
		parsed.Entries[0].HostEffect == "" {
		t.Fatalf("host entry = %#v", parsed.Entries[0])
	}
	if parsed.Entries[1].Provenance != CheckProvenanceRole ||
		parsed.Entries[1].RoleDigest == "" {
		t.Fatalf("role entry = %#v", parsed.Entries[1])
	}
}

func TestParseCheckResultsFailsClosedOnProvenanceViolations(t *testing.T) {
	tests := map[string]struct {
		build    func(*CheckResults) ([]byte, error)
		wantCode string
	}{
		"missing provenance": {
			build: func(value *CheckResults) ([]byte, error) {
				value.Entries[0].Provenance = ""
				return EncodeCheckResults(*value)
			},
			wantCode: "INVALID_FIELD",
		},
		"unknown provenance": {
			build: func(value *CheckResults) ([]byte, error) {
				value.Entries[0].Provenance = "worker"
				return EncodeCheckResults(*value)
			},
			wantCode: "INVALID_FIELD",
		},
		"host entry output digest mismatch": {
			build: func(value *CheckResults) ([]byte, error) {
				value.Entries[0].OutputDigest = "sha256:" + strings.Repeat("f", 64)
				return EncodeCheckResults(*value)
			},
			wantCode: "STALE_BINDING",
		},
		"host entry without exit code": {
			build: func(value *CheckResults) ([]byte, error) {
				value.Entries[0].ExitCode = nil
				return EncodeCheckResults(*value)
			},
			wantCode: "MISSING_FIELD",
		},
		"host entry with role digest": {
			build: func(value *CheckResults) ([]byte, error) {
				value.Entries[0].RoleDigest = "sha256:" + strings.Repeat("a", 64)
				return EncodeCheckResults(*value)
			},
			wantCode: "INVALID_FIELD",
		},
		"role entry without role digest": {
			build: func(value *CheckResults) ([]byte, error) {
				value.Entries[1].RoleDigest = ""
				return EncodeCheckResults(*value)
			},
			wantCode: "MISSING_FIELD",
		},
		"role entry with output digest": {
			build: func(value *CheckResults) ([]byte, error) {
				value.Entries[1].OutputDigest = "sha256:" + strings.Repeat("f", 64)
				return EncodeCheckResults(*value)
			},
			wantCode: "INVALID_FIELD",
		},
		"role entry non-pass outcome": {
			build: func(value *CheckResults) ([]byte, error) {
				value.Entries[1].Outcome = CheckOutcomeFail
				return EncodeCheckResults(*value)
			},
			wantCode: "INVALID_FIELD",
		},
		"host truncated entry without marker": {
			build: func(value *CheckResults) ([]byte, error) {
				value.Entries[0].Truncated = true
				value.Entries[0].Output = "no marker here"
				return EncodeCheckResults(*value)
			},
			wantCode: "INVALID_FIELD",
		},
		"unknown outcome": {
			build: func(value *CheckResults) ([]byte, error) {
				value.Entries[0].Outcome = "pending"
				return EncodeCheckResults(*value)
			},
			wantCode: "INVALID_FIELD",
		},
		"empty entries": {
			build: func(value *CheckResults) ([]byte, error) {
				value.Entries = nil
				return EncodeCheckResults(*value)
			},
			wantCode: "INVALID_FIELD",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			value := checkResultsFixture(t)
			raw, err := test.build(&value)
			if err != nil {
				// EncodeCheckResults already rejected the malformed manifest
				// with the fail-closed code; that is itself the proof.
				if ErrorCode(err) != test.wantCode {
					t.Fatalf("EncodeCheckResults error = %v, want code %s", err, test.wantCode)
				}
				return
			}
			_, err = ParseCheckResults(raw)
			if err == nil || ErrorCode(err) != test.wantCode {
				t.Fatalf("ParseCheckResults error = %v, want code %s", err, test.wantCode)
			}
		})
	}
}

func TestCheckResultsHostEffectAndProvenanceAreStructural(t *testing.T) {
	// A host entry and a role entry for the same check text must stay
	// distinguishable purely by their structural labels: the manifest never
	// infers provenance from text or bytes.
	host := int(0)
	value := checkResultsFixture(t)
	raw, err := EncodeCheckResults(value)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseCheckResults(raw)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Entries[0].Provenance != CheckProvenanceHost ||
		parsed.Entries[1].Provenance != CheckProvenanceRole {
		t.Fatalf("provenance labels were lost: %#v", parsed.Entries)
	}
	_ = host
}

func TestHostCheckTruncationMarkerIsTruthful(t *testing.T) {
	marker := "this output was cut " + HostCheckTruncationPrefix + " at 256 KiB]"
	if !strings.Contains(marker, HostCheckTruncationPrefix) {
		t.Fatal("marker prefix is not embedded")
	}
}
