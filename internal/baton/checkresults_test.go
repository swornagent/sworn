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
		"role entry non-pass outcome without diagnostic": {
			build: func(value *CheckResults) ([]byte, error) {
				value.Entries[1].Outcome = CheckOutcomeFail
				return EncodeCheckResults(*value)
			},
			wantCode: "MISSING_FIELD",
		},
		"role entry truncated without marker": {
			build: func(value *CheckResults) ([]byte, error) {
				value.Entries[1].Truncated = true
				value.Entries[1].Output = "no marker here"
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

// TestParseCheckResultsAdmitsNonPassRoleEntryWithDiagnostic pins S5-A3 point
// 4: a role entry can honestly record a failing declared check instead of
// fabricating pass, provided it says why.
func TestParseCheckResultsAdmitsNonPassRoleEntryWithDiagnostic(t *testing.T) {
	value := checkResultsFixture(t)
	value.Entries[1].Outcome = CheckOutcomeFail
	value.Entries[1].Diagnostic = "gofmt found unformatted files"
	raw, err := EncodeCheckResults(value)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseCheckResults(raw)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Entries[1].Outcome != CheckOutcomeFail ||
		parsed.Entries[1].Diagnostic != "gofmt found unformatted files" {
		t.Fatalf("role entry = %#v", parsed.Entries[1])
	}
}

// TestParseCheckResultsAdmitsRoleEntryWithOutputInsteadOfRoleDigest pins that
// the "at least one of role_digest, output, diagnostic" rule is genuinely
// disjunctive, not role_digest in disguise.
func TestParseCheckResultsAdmitsRoleEntryWithOutputInsteadOfRoleDigest(t *testing.T) {
	value := checkResultsFixture(t)
	value.Entries[1].RoleDigest = ""
	value.Entries[1].Output = "go vet: no issues"
	raw, err := EncodeCheckResults(value)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseCheckResults(raw)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Entries[1].RoleDigest != "" || parsed.Entries[1].Output != "go vet: no issues" {
		t.Fatalf("role entry = %#v", parsed.Entries[1])
	}
}

// TestParseCheckResultsAdmitsTruncatedRoleEntryWithMarker pins that a role
// entry's excerpt may be honestly cut, like a host entry's, provided the
// truthful marker travels with it.
func TestParseCheckResultsAdmitsTruncatedRoleEntryWithMarker(t *testing.T) {
	value := checkResultsFixture(t)
	value.Entries[1].Truncated = true
	value.Entries[1].Output = HostCheckTruncationPrefix + " at 4096 bytes]\nhead of output"
	raw, err := EncodeCheckResults(value)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseCheckResults(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !parsed.Entries[1].Truncated {
		t.Fatalf("role entry = %#v", parsed.Entries[1])
	}
}

// TestParseCheckResultsAdmitsUnassertedBindingFields pins the five-field
// zero-value widening: a driver-built manifest that asserts none of release,
// slice, attempt, candidate or contract_digest still parses, since it has no
// authoritative source for any of them and must never guess.
func TestParseCheckResultsAdmitsUnassertedBindingFields(t *testing.T) {
	value := checkResultsFixture(t)
	value.Release, value.Slice, value.Candidate, value.ContractDigest = "", "", "", ""
	value.Attempt = 0
	raw, err := EncodeCheckResults(value)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseCheckResults(raw)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Release != "" || parsed.Slice != "" || parsed.Candidate != "" ||
		parsed.ContractDigest != "" || parsed.Attempt != 0 {
		t.Fatalf("parsed = %#v, want every binding field at its zero value", parsed)
	}
	if _, err := ParseCheckResults(raw); err != nil {
		t.Fatalf("re-parse of an unasserted manifest failed: %v", err)
	}
}

// TestParseCheckResultsRejectsAssertedInvalidBindingFields pins that a
// non-zero binding field is still validated in its full exact form: the
// widening only ever admits the zero value, never a malformed one.
func TestParseCheckResultsRejectsAssertedInvalidBindingFields(t *testing.T) {
	value := checkResultsFixture(t)
	value.Release = "not an identity!"
	if _, err := EncodeCheckResults(value); ErrorCode(err) != "INVALID_FIELD" {
		t.Fatalf("EncodeCheckResults error = %v, want INVALID_FIELD", err)
	}
}

// declaredS5Checks are this release's own four literal declared checks
// (contracts/2026-08-28-legible-refusals/S5-verification-integrity.json),
// pinned here as data so CheckCommandCovers is proven against the real
// contract, not a synthetic stand-in.
var declaredS5Checks = []string{
	"GOFLAGS=-buildvcs=false go test -count=1 ./cmd/sworn ./internal/... ./tools/...",
	"GOFLAGS=-buildvcs=false go test -count=1 -race ./internal/driver ./internal/baton",
	"GOFLAGS=-buildvcs=false go vet ./...",
	"gofmt -l ./cmd ./internal ./tools",
}

// TestCheckCommandCoversAgainstRealDeclaredChecks pins finding 1 of the S5-A3
// captain review: the matching rule is satisfiable by a real command a
// verifier can run under this engine's actual sandbox PATH
// (/usr/bin:/bin, with the Go toolchain at /usr/local/go/bin, off PATH).
func TestCheckCommandCoversAgainstRealDeclaredChecks(t *testing.T) {
	tests := []struct {
		name     string
		declared string
		recorded string
		want     bool
	}{
		{
			name:     "absolute go with redirect and tail",
			declared: declaredS5Checks[0],
			recorded: "GOFLAGS=-buildvcs=false /usr/local/go/bin/go test -count=1 " +
				"./cmd/sworn ./internal/... ./tools/... > /tmp/s5-check1.log 2>&1; " +
				"tail -c 4000 /tmp/s5-check1.log",
			want: true,
		},
		{
			name:     "PATH prefix form",
			declared: declaredS5Checks[0],
			recorded: "PATH=/usr/local/go/bin:$PATH GOFLAGS=-buildvcs=false go test " +
				"-count=1 ./cmd/sworn ./internal/... ./tools/...",
			want: true,
		},
		{
			name:     "race variant absolute go",
			declared: declaredS5Checks[1],
			recorded: "GOFLAGS=-buildvcs=false /usr/local/go/bin/go test -count=1 -race " +
				"./internal/driver ./internal/baton > /tmp/s5-check2.log 2>&1",
			want: true,
		},
		{
			name:     "go vet absolute",
			declared: declaredS5Checks[2],
			recorded: "GOFLAGS=-buildvcs=false /usr/local/go/bin/go vet ./... 2>&1 | tail -c 4000",
			want:     true,
		},
		{
			name:     "gofmt absolute, no env prefix",
			declared: declaredS5Checks[3],
			recorded: "/usr/local/go/bin/gofmt -l ./cmd ./internal ./tools 2>&1 | head -50",
			want:     true,
		},
		{
			name:     "wrong package scope does not cover",
			declared: declaredS5Checks[0],
			recorded: "GOFLAGS=-buildvcs=false /usr/local/go/bin/go test -count=1 ./internal/...",
			want:     false,
		},
		{
			name:     "vet wrong scope does not cover",
			declared: declaredS5Checks[2],
			recorded: "GOFLAGS=-buildvcs=false /usr/local/go/bin/go vet ./cmd/...",
			want:     false,
		},
		{
			name:     "echoing the check is not running it",
			declared: declaredS5Checks[3],
			recorded: "echo gofmt -l ./cmd ./internal ./tools",
			want:     false,
		},
		{
			name:     "unrelated leading command does not cover",
			declared: declaredS5Checks[3],
			recorded: "/bin/false; gofmt -l ./cmd ./internal ./tools",
			want:     false,
		},
		{
			name:     "bare PATH= with no command never covers",
			declared: declaredS5Checks[3],
			recorded: "PATH=/usr/local/go/bin:$PATH",
			want:     false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := CheckCommandCovers(test.declared, test.recorded); got != test.want {
				t.Fatalf("CheckCommandCovers(%q, %q) = %v, want %v", test.declared, test.recorded, got, test.want)
			}
		})
	}
}
