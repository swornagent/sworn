package baton

import (
	"bytes"
	"strings"
	"testing"
)

// planTemplateBytes reads the exact embedded Planner template asset through
// the same admitted-package API a real Planner run uses.
func planTemplateBytes(t *testing.T) []byte {
	t.Helper()
	pkg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := pkg.ReadAsset("templates/plan.md")
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// extractFencedBlock returns the bytes strictly between the first occurrence
// of open and the next occurrence of close after it.
func extractFencedBlock(t *testing.T, raw []byte, open, close string) []byte {
	t.Helper()
	start := bytes.Index(raw, []byte(open))
	if start < 0 {
		t.Fatalf("template does not contain fence %q", open)
	}
	body := raw[start+len(open):]
	end := bytes.Index(body, []byte(close))
	if end < 0 {
		t.Fatalf("template fence %q is never closed", open)
	}
	return body[:end]
}

// TestPlannerTemplateNamesOnlySwornSchemaAndPreparedTreeContracts proves the
// embedded Planner template proposes only the Sworn-native
// sworn.release-manifest/v1 identity, never the legacy baton.plan/v2 fence
// nor an unapproved baton.plan/v3, and documents the supported prepared-tree
// contract delivery path rather than inventing a new wire envelope.
func TestPlannerTemplateNamesOnlySwornSchemaAndPreparedTreeContracts(t *testing.T) {
	t.Parallel()
	raw := planTemplateBytes(t)
	text := string(raw)

	if !bytes.HasPrefix(raw, []byte("```sworn-release-manifest-v1\n")) {
		t.Fatalf("template must open with the sworn-release-manifest-v1 fence, got:\n%s", text[:min(80, len(text))])
	}
	if !strings.Contains(text, `"schema_version": "`+ManifestVersion+`"`) {
		t.Fatalf("template does not declare schema_version %q", ManifestVersion)
	}
	for _, forbidden := range []string{"baton.plan/v2", "baton-plan-v2", "baton.plan/v3", "baton-plan-v3"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("template must not name %q", forbidden)
		}
	}
	if !strings.Contains(text, "contract_path") || !strings.Contains(text, "digest") {
		t.Fatal("template must show a manifest slice entry naming a separate contract_path and digest")
	}
	if !strings.Contains(text, "committed at `target_ref` before this") {
		t.Fatal("template must document committing contracts through the supported prepared-tree path before the revision names them")
	}
}

// TestPlannerTemplateWorkedExampleIsSelfConsistentAndAdmissible proves the
// template is not merely descriptive prose: its embedded manifest and
// contract example are exactly what the real canonical parser and digest
// algorithm admit, with the manifest's declared digest and touchpoints
// matching the contract's real computed digest and scope.
func TestPlannerTemplateWorkedExampleIsSelfConsistentAndAdmissible(t *testing.T) {
	t.Parallel()
	raw := planTemplateBytes(t)

	manifestJSON := extractFencedBlock(t, raw, "```sworn-release-manifest-v1\n", "\n```\n")
	manifestBytes := append(append([]byte("```sworn-release-manifest-v1\n"), manifestJSON...), []byte("\n```\n")...)
	plan, err := ParsePlan(manifestBytes)
	if err != nil {
		t.Fatalf("template manifest example does not parse: %v", err)
	}
	metadata := plan.Metadata()
	if len(metadata.Tracks) != 1 || len(metadata.Tracks[0].Slices) != 1 {
		t.Fatalf("unexpected template shape: %#v", metadata.Tracks)
	}
	slice := metadata.Tracks[0].Slices[0]
	declaredDigest := metadata.Contracts[slice.ID]

	contractJSON := extractFencedBlock(t, raw, "```json\n", "\n```\n")
	resolved, err := plan.ResolveSliceContract(slice.ID, contractJSON)
	if err != nil {
		t.Fatalf("template contract example does not resolve against the template manifest: %v", err)
	}
	_, realDigest, err := ParseSliceContract(contractJSON, slice.ID, metadata.Tracks[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if realDigest != declaredDigest {
		t.Fatalf("template manifest digest %q does not match the real contract digest %q", declaredDigest, realDigest)
	}
	if resolved.ContractPath != slice.ContractPath {
		t.Fatalf("resolved contract path = %q, want %q", resolved.ContractPath, slice.ContractPath)
	}
}
