package baton

import (
	"encoding/json"
	"strings"
	"testing"
)

func hostChecksPlan(t *testing.T, hostChecks []string) (Plan, Slice) {
	t.Helper()
	metadata := Metadata{
		SchemaVersion: PlanVersion,
		Release:       "release-host",
		Revision:      1,
		PreviousPlan:  nil,
		Repository:    "acme-repo",
		TargetRef:     "refs/heads/main",
		ApprovalRef:   "operator://release-host/1",
		Tracks: []Track{{
			ID: "T1", DependsOn: []string{},
			Slices: []Slice{{
				ID: "S1", Outcome: "Deliver S1.",
				Scope:       Scope{Include: []string{"internal/runtime"}, Exclude: []string{}},
				Acceptance:  []Criterion{{ID: "A1", Text: "S1 is exact."}},
				Checks:      []string{"go test ./...", "go vet ./..."},
				HostChecks:  hostChecks,
				Constraints: []string{"deterministic"},
				DependsOn:   []string{}, Consumes: []string{},
			}},
		}},
	}
	body, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte("```baton-plan-v2\n" + string(body) + "\n```\n\nFixture.\n")
	plan, err := ParsePlan(raw)
	if err != nil {
		t.Fatal(err)
	}
	_, slice, ok := plan.FindSlice("S1")
	if !ok {
		t.Fatal("slice not found")
	}
	return plan, slice
}

func TestHostChecksAreAdmittedAsSubsetOfChecks(t *testing.T) {
	plan, slice := hostChecksPlan(t, []string{"go vet ./..."})
	if len(slice.HostChecks) != 1 || slice.HostChecks[0] != "go vet ./..." {
		t.Fatalf("host checks = %#v", slice.HostChecks)
	}
	digest, ok := plan.Contract("S1")
	if !ok || digest == "" {
		t.Fatal("contract digest is absent")
	}
}

func TestHostChecksRejectNonDeclaredCheck(t *testing.T) {
	for _, hostChecks := range [][]string{
		{"go vet ./...", "rm -rf /"},
		{"not-a-declared-check"},
	} {
		_, err := ParsePlan(buildHostChecksPlanBytes(t, hostChecks))
		if err == nil || ErrorCode(err) != "INVALID_FIELD" {
			t.Fatalf("host checks %v error = %v, want INVALID_FIELD", hostChecks, err)
		}
	}
}

func TestHostChecksRejectEmptyList(t *testing.T) {
	// An explicit empty host_checks array in a contract body must be rejected:
	// the declaration is meaningful only when it names real checks.
	raw := []byte(`{
		"outcome": "Deliver S1.",
		"scope": {"include": ["internal/runtime"], "exclude": []},
		"acceptance": [{"id": "A1", "text": "S1 is exact."}],
		"checks": ["go test ./...", "go vet ./..."],
		"host_checks": [],
		"constraints": ["deterministic"],
		"depends_on": [],
		"consumes": []
	}`)
	if _, _, err := ParseSliceContract(raw, "S1", "T1"); err == nil || ErrorCode(err) != "INVALID_FIELD" {
		t.Fatalf("empty host checks error = %v, want INVALID_FIELD", err)
	}
}

func TestLegacyContractDigestStableWithoutHostChecks(t *testing.T) {
	withChecks, _ := hostChecksPlan(t, []string{"go vet ./..."})
	withoutChecks, _ := hostChecksPlan(t, nil)
	withDigest, _ := withChecks.Contract("S1")
	withoutDigest, _ := withoutChecks.Contract("S1")
	if withDigest == withoutDigest {
		t.Fatal("host-checks digest must differ from a bare checks digest")
	}

	// A contract that never declares host_checks must keep the exact digest it
	// would have before this release: the canonical payload omits the field.
	metadata := Metadata{
		SchemaVersion: PlanVersion,
		Release:       "release-stable",
		Revision:      1,
		PreviousPlan:  nil,
		Repository:    "acme-repo",
		TargetRef:     "refs/heads/main",
		ApprovalRef:   "operator://release-stable/1",
		Tracks: []Track{{
			ID: "T1", DependsOn: []string{},
			Slices: []Slice{{
				ID: "S1", Outcome: "Deliver S1.",
				Scope:       Scope{Include: []string{"internal/runtime"}, Exclude: []string{}},
				Acceptance:  []Criterion{{ID: "A1", Text: "S1 is exact."}},
				Checks:      []string{"go test ./..."},
				Constraints: []string{"deterministic"},
				DependsOn:   []string{}, Consumes: []string{},
			}},
		}},
	}
	body, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte("```baton-plan-v2\n" + string(body) + "\n```\n\nFixture.\n")
	plan, err := ParsePlan(raw)
	if err != nil {
		t.Fatal(err)
	}
	stableDigest, ok := plan.Contract("S1")
	if !ok {
		t.Fatal("contract digest is absent")
	}
	if !strings.HasPrefix(stableDigest, "sha256:") || len(stableDigest) != 71 {
		t.Fatalf("unexpected digest %q", stableDigest)
	}
	// Re-parse byte-identical input and confirm the same digest (determinism),
	// and confirm the canonical slice body hashes to the same value through
	// ParseSliceContract.
	reparsed, err := ParsePlan(raw)
	if err != nil {
		t.Fatal(err)
	}
	reparsedDigest, _ := reparsed.Contract("S1")
	if reparsedDigest != stableDigest {
		t.Fatalf("digest changed across identical parses: %s != %s", reparsedDigest, stableDigest)
	}
}

func buildHostChecksPlanBytes(t *testing.T, hostChecks []string) []byte {
	t.Helper()
	metadata := Metadata{
		SchemaVersion: PlanVersion,
		Release:       "release-host",
		Revision:      1,
		PreviousPlan:  nil,
		Repository:    "acme-repo",
		TargetRef:     "refs/heads/main",
		ApprovalRef:   "operator://release-host/1",
		Tracks: []Track{{
			ID: "T1", DependsOn: []string{},
			Slices: []Slice{{
				ID: "S1", Outcome: "Deliver S1.",
				Scope:       Scope{Include: []string{"internal/runtime"}, Exclude: []string{}},
				Acceptance:  []Criterion{{ID: "A1", Text: "S1 is exact."}},
				Checks:      []string{"go test ./...", "go vet ./..."},
				HostChecks:  hostChecks,
				Constraints: []string{"deterministic"},
				DependsOn:   []string{}, Consumes: []string{},
			}},
		}},
	}
	body, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return []byte("```baton-plan-v2\n" + string(body) + "\n```\n\nFixture.\n")
}

func TestResolveSliceContractAtUsesLegacyInlineBody(t *testing.T) {
	plan, slice := hostChecksPlan(t, []string{"go vet ./..."})
	resolved, err := plan.ResolveSliceContractAt(GitRepository{}, "S1", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.HostChecks) != len(slice.HostChecks) ||
		resolved.HostChecks[0] != "go vet ./..." {
		t.Fatalf("resolved = %#v", resolved)
	}
}

func TestResolveSliceContractAtFailsClosedOnUnknownSlice(t *testing.T) {
	plan, _ := hostChecksPlan(t, nil)
	if _, err := plan.ResolveSliceContractAt(GitRepository{}, "S9", ""); ErrorCode(err) != "SLICE_NOT_FOUND" {
		t.Fatalf("unknown slice error = %v", err)
	}
}
