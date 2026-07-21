package protocol

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

type exactCheckArtifact struct {
	mediaType string
	contents  []byte
}

type exactCheckArtifacts map[string]exactCheckArtifact

func (artifacts exactCheckArtifacts) Artifact(
	_ context.Context,
	digest string,
) (string, []byte, error) {
	artifact, exists := artifacts[digest]
	if !exists {
		return "", nil, errors.New("artifact not found")
	}
	return artifact.mediaType, append([]byte(nil), artifact.contents...), nil
}

func (artifacts exactCheckArtifacts) putJSON(t testing.TB, value any) Artifact {
	t.Helper()
	contents, err := EncodeCanonical(value)
	if err != nil {
		t.Fatal(err)
	}
	digest := RawDigest(contents)
	artifacts[digest] = exactCheckArtifact{mediaType: "application/json", contents: contents}
	return Artifact{Ref: digest, MediaType: "application/json", Digest: digest}
}

func TestResolveArtifactEnforcesByteCeiling(t *testing.T) {
	t.Parallel()
	contents := []byte("bounded")
	digest := RawDigest(contents)
	artifacts := exactCheckArtifacts{digest: {mediaType: "application/octet-stream", contents: contents}}
	pointer := Artifact{Ref: digest, MediaType: "application/octet-stream", Digest: digest}
	if _, err := ResolveArtifact(context.Background(), artifacts, pointer, uint64(len(contents)-1)); err == nil ||
		!strings.Contains(err.Error(), "byte ceiling") {
		t.Fatalf("undersized ceiling error = %v", err)
	}
	if resolved, err := ResolveArtifact(context.Background(), artifacts, pointer, uint64(len(contents))); err != nil ||
		string(resolved) != string(contents) {
		t.Fatalf("exact ceiling artifact = %q, %v", resolved, err)
	}
}

type exactCheckSpec struct {
	checkID      string
	evidenceID   string
	boundary     string
	acceptanceID string
}

func TestResolveExactLocalChecksBindsOrderedPolicyAndReturnsDefensiveViews(t *testing.T) {
	t.Parallel()
	plan, artifacts, definitions := exactLocalChecksFixture(t, "assembled", []exactCheckSpec{
		{checkID: "zeta", evidenceID: "evidence-zeta", boundary: "assembled", acceptanceID: "AC1"},
		{checkID: "alpha", evidenceID: "evidence-alpha", boundary: "component", acceptanceID: "AC1"},
	})
	selection, err := ResolveExactLocalChecks(context.Background(), artifacts, plan, "work-1")
	if err != nil {
		t.Fatal(err)
	}
	requirements := selection.Requirements()
	if len(requirements) != 2 || requirements[0].CheckID != "zeta" ||
		requirements[1].CheckID != "alpha" || requirements[0].Definition != definitions[0] ||
		requirements[0].Definition.Ref != requirements[0].Definition.Digest {
		t.Fatalf("requirements = %#v", requirements)
	}
	contract, _ := plan.Work("work-1")
	if selection.ContractDigest() != contract.Digest() {
		t.Fatalf("contract digest = %q, want %q", selection.ContractDigest(), contract.Digest())
	}
	requirements[0].CheckID = "changed"
	requirements[0].Definition.Digest = testProtocolDigest("f")
	repeated := selection.Requirements()
	if repeated[0].CheckID != "zeta" || repeated[0].Definition != definitions[0] {
		t.Fatalf("selection was mutated through requirements: %#v", repeated)
	}
}

func TestResolveExactLocalChecksAcceptsMaximumPolicyFanout(t *testing.T) {
	t.Parallel()
	specs := make([]exactCheckSpec, MaximumExactLocalChecks)
	for index := range specs {
		specs[index] = exactCheckSpec{
			checkID: fmt.Sprintf("check-%02d", index), evidenceID: fmt.Sprintf("evidence-%02d", index),
			boundary: "component", acceptanceID: "AC1",
		}
	}
	plan, artifacts, _ := exactLocalChecksFixture(t, "component", specs)
	selection, err := ResolveExactLocalChecks(context.Background(), artifacts, plan, "work-1")
	if err != nil {
		t.Fatal(err)
	}
	requirements := selection.Requirements()
	if len(requirements) != MaximumExactLocalChecks || requirements[0].CheckID != "check-00" ||
		requirements[MaximumExactLocalChecks-1].CheckID != "check-63" {
		t.Fatalf("maximum policy requirements = %#v", requirements)
	}
}

func TestResolveExactLocalChecksRejectsPolicyDefinitionAndCoverageDrift(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		acceptanceLevel string
		specs           []exactCheckSpec
		want            string
	}{
		"unknown acceptance": {
			acceptanceLevel: "component",
			specs: []exactCheckSpec{{
				checkID: "test", evidenceID: "evidence", boundary: "component", acceptanceID: "AC2",
			}},
			want: "unknown exact-plan acceptance",
		},
		"duplicate evidence": {
			acceptanceLevel: "component",
			specs: []exactCheckSpec{
				{checkID: "first", evidenceID: "shared", boundary: "component", acceptanceID: "AC1"},
				{checkID: "second", evidenceID: "shared", boundary: "component", acceptanceID: "AC1"},
			},
			want: "reuse evidence id",
		},
		"insufficient boundary": {
			acceptanceLevel: "assembled",
			specs: []exactCheckSpec{{
				checkID: "test", evidenceID: "evidence", boundary: "component", acceptanceID: "AC1",
			}},
			want: "lacks sufficient evidence coverage",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			plan, artifacts, _ := exactLocalChecksFixture(t, test.acceptanceLevel, test.specs)
			_, err := ResolveExactLocalChecks(context.Background(), artifacts, plan, "work-1")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestResolveExactLocalChecksCapsPolicyBeforeResolvingDefinitions(t *testing.T) {
	t.Parallel()
	artifacts := exactCheckArtifacts{}
	definition := artifacts.putJSON(t, localCheckDefinition("evidence", "component", "AC1"))
	checks := make([]any, MaximumExactLocalChecks+1)
	for index := range checks {
		checks[index] = map[string]any{
			"id": fmt.Sprintf("check-%02d", index),
			"definition": map[string]any{
				"ref":        fmt.Sprintf("policy/checks/%02d.json", index),
				"media_type": "application/json", "digest": definition.Digest,
			},
		}
	}
	policy := artifacts.putJSON(t, map[string]any{
		"schema_version": AssurancePolicySchemaVersion, "policy_id": "standard",
		"checks": checks, "packs": []any{},
	})
	plan := exactLocalChecksPlan(t, policy.Digest, "component")
	_, err := ResolveExactLocalChecks(context.Background(), artifacts, plan, "work-1")
	if err == nil || !strings.Contains(err.Error(), "requires 1-64 local checks") {
		t.Fatalf("error = %v", err)
	}
}

func TestResolveExactLocalChecksRejectsUnavailableOrInvalidDefinition(t *testing.T) {
	t.Parallel()
	plan, artifacts, definitions := exactLocalChecksFixture(t, "component", []exactCheckSpec{{
		checkID: "test", evidenceID: "evidence", boundary: "component", acceptanceID: "AC1",
	}})
	delete(artifacts, definitions[0].Digest)
	if _, err := ResolveExactLocalChecks(context.Background(), artifacts, plan, "work-1"); err == nil ||
		!strings.Contains(err.Error(), "resolve check \"test\" definition") {
		t.Fatalf("missing definition error = %v", err)
	}
	invalidArtifacts := exactCheckArtifacts{}
	invalidDefinition := invalidArtifacts.putJSON(t, map[string]any{"schema_version": "wrong"})
	invalidPolicy := invalidArtifacts.putJSON(t, map[string]any{
		"schema_version": AssurancePolicySchemaVersion, "policy_id": "standard",
		"checks": []any{map[string]any{
			"id": "test", "definition": map[string]any{
				"ref": "policy/checks/test.json", "media_type": "application/json",
				"digest": invalidDefinition.Digest,
			},
		}},
		"packs": []any{},
	})
	invalidPlan := exactLocalChecksPlan(t, invalidPolicy.Digest, "component")
	if _, err := ResolveExactLocalChecks(context.Background(), invalidArtifacts, invalidPlan, "work-1"); err == nil ||
		!strings.Contains(err.Error(), "parse check \"test\" definition") {
		t.Fatalf("invalid definition error = %v", err)
	}
}

func exactLocalChecksFixture(
	t testing.TB,
	acceptanceLevel string,
	specs []exactCheckSpec,
) (ExactPlan, exactCheckArtifacts, []Artifact) {
	t.Helper()
	artifacts := exactCheckArtifacts{}
	definitions := make([]Artifact, len(specs))
	checks := make([]any, len(specs))
	for index, spec := range specs {
		definitions[index] = artifacts.putJSON(
			t, localCheckDefinition(spec.evidenceID, spec.boundary, spec.acceptanceID),
		)
		checks[index] = map[string]any{
			"id": spec.checkID,
			"definition": map[string]any{
				"ref":        "policy/checks/" + spec.checkID + ".json",
				"media_type": "application/json", "digest": definitions[index].Digest,
			},
		}
	}
	policy := artifacts.putJSON(t, map[string]any{
		"schema_version": AssurancePolicySchemaVersion, "policy_id": "standard",
		"checks": checks, "packs": []any{},
	})
	return exactLocalChecksPlan(t, policy.Digest, acceptanceLevel), artifacts, definitions
}

func exactLocalChecksPlan(t testing.TB, policyDigest, acceptanceLevel string) ExactPlan {
	t.Helper()
	contents, err := EncodeCanonical(map[string]any{
		"schema_version": DeliveryPlanSchemaVersion,
		"delivery_id":    "delivery-1", "outcome": "Produce the exact candidate.",
		"created_at":       "2026-07-20T00:00:00Z",
		"assurance_policy": map[string]any{"ref": "policy:standard", "digest": policyDigest},
		"target":           map[string]any{"repository": "repo-1", "ref": "refs/heads/main"},
		"authority": map[string]any{
			"ref":    "authority-source",
			"grants": []any{map[string]any{"action": "execute", "target": "workspace"}},
		},
		"work": []any{map[string]any{
			"id": "work-1", "outcome": "Produce the exact candidate.",
			"scope": map[string]any{"include": []string{"."}, "exclude": []string{}},
			"acceptance": []any{map[string]any{
				"id": "AC1", "criterion": "The exact candidate is proven.", "evidence_level": acceptanceLevel,
			}},
			"depends_on": []string{},
			"assurance":  map[string]any{"profile": "standard", "packs": []string{}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := ParseDeliveryPlan(contents)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func localCheckDefinition(evidenceID, boundary, acceptanceID string) LocalCheckDefinition {
	return LocalCheckDefinition{
		SchemaVersion: LocalCheckDefinitionSchemaVersion,
		Argv:          []string{"/usr/bin/true"}, WorkingDirectory: ".", TimeoutSeconds: 30,
		Evidence: LocalEvidenceDefinition{
			ID: evidenceID, AcceptanceIDs: []string{acceptanceID}, Boundary: boundary,
			Observed: "The exact candidate behavior.",
		},
	}
}
