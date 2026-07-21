package protocol

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestParseAssurancePolicyRegistryRequiresCanonicalExactSchema(t *testing.T) {
	t.Parallel()
	canonical := assurancePolicyFixture(t, nil)
	registry, err := parseAssurancePolicyRegistry(canonical)
	if err != nil {
		t.Fatal(err)
	}
	if len(registry.checks) != 1 || registry.checks[0].id != "test" ||
		registry.checks[0].definition.Ref != "policy/checks/test.json" ||
		registry.checks[0].definition.Digest != "sha256:"+strings.Repeat("a", 64) {
		t.Fatalf("registry = %#v", registry)
	}

	var pretty bytes.Buffer
	if err := json.Indent(&pretty, canonical, "", "  "); err != nil {
		t.Fatal(err)
	}
	if _, err := parseAssurancePolicyRegistry([]byte(pretty.String())); err == nil ||
		!strings.Contains(err.Error(), "stored as canonical JSON") {
		t.Fatalf("noncanonical policy error = %v", err)
	}
}

func TestValidateInitialContractRetainsNarrowStandardCapability(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		workCount int
		work      PlanWorkView
	}{
		"multiple work contracts": {
			workCount: 2,
			work:      PlanWorkView{Assurance: PlanAssurance{Profile: "standard", Packs: []string{}}},
		},
		"dependency": {
			workCount: 1,
			work: PlanWorkView{
				DependsOn: []string{"earlier-work"},
				Assurance: PlanAssurance{Profile: "standard", Packs: []string{}},
			},
		},
		"assured": {
			workCount: 1,
			work:      PlanWorkView{Assurance: PlanAssurance{Profile: "assured", Packs: []string{"security@1"}}},
		},
		"standard with pack": {
			workCount: 1,
			work:      PlanWorkView{Assurance: PlanAssurance{Profile: "standard", Packs: []string{"security@1"}}},
		},
		"live acceptance": {
			workCount: 1,
			work: PlanWorkView{
				Assurance:  PlanAssurance{Profile: "standard", Packs: []string{}},
				Acceptance: []PlanAcceptance{{ID: "AC1", EvidenceLevel: "live"}},
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if err := validateInitialContract(test.workCount, test.work); err == nil {
				t.Fatal("unsupported initial contract was accepted")
			}
		})
	}
}

func TestParseAssurancePolicyRegistryRejectsSchemaAndIdentityDrift(t *testing.T) {
	t.Parallel()
	tests := map[string]func(map[string]any){
		"unknown field": func(policy map[string]any) { policy["extra"] = true },
		"missing packs": func(policy map[string]any) { delete(policy, "packs") },
		"empty checks":  func(policy map[string]any) { policy["checks"] = []any{} },
		"duplicate check id": func(policy map[string]any) {
			checks := policy["checks"].([]any)
			checks = append(checks, map[string]any{
				"id": "test", "definition": map[string]any{
					"ref": "other.json", "media_type": "application/json",
					"digest": "sha256:" + strings.Repeat("b", 64),
				},
			})
			policy["checks"] = checks
		},
		"invalid pack id": func(policy map[string]any) {
			policy["packs"].([]any)[0].(map[string]any)["id"] = "security"
		},
		"unknown definition field": func(policy map[string]any) {
			policy["checks"].([]any)[0].(map[string]any)["definition"].(map[string]any)["extra"] = true
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := parseAssurancePolicyRegistry(assurancePolicyFixture(t, mutate)); err == nil {
				t.Fatal("invalid assurance policy was accepted")
			}
		})
	}
}

func assurancePolicyFixture(t testing.TB, mutate func(map[string]any)) []byte {
	t.Helper()
	definition := func(ref, digest string) map[string]any {
		return map[string]any{"ref": ref, "media_type": "application/json", "digest": digest}
	}
	policy := map[string]any{
		"schema_version": AssurancePolicySchemaVersion,
		"policy_id":      "standard",
		"checks": []any{map[string]any{
			"id": "test", "definition": definition("policy/checks/test.json", "sha256:"+strings.Repeat("a", 64)),
		}},
		"packs": []any{map[string]any{
			"id": "security@1", "definition": definition("policy/packs/security.json", "sha256:"+strings.Repeat("c", 64)),
		}},
	}
	if mutate != nil {
		mutate(policy)
	}
	contents, err := EncodeCanonical(policy)
	if err != nil {
		t.Fatal(err)
	}
	return contents
}
