package baton

import (
	"bytes"
	"strings"
	"testing"
)

// TestCanonicalDigestDocumentation states the three facts an operator
// authoring contracts must know: the canonical payload shape, why
// sha256sum over the contract file never matches the declared digest, and
// the stability promise. The test-pinned surface is the Go constant
// CanonicalDigestDocumentation(), not a docs/ path or a vendored asset.
func TestCanonicalDigestDocumentation(t *testing.T) {
	t.Parallel()
	doc := CanonicalDigestDocumentation()
	if doc == "" {
		t.Fatal("documentation is empty")
	}
	// (a) The canonical payload shape is stated.
	if !strings.Contains(doc, "track") || !strings.Contains(doc, "id") ||
		!strings.Contains(doc, "outcome") || !strings.Contains(doc, "scope") ||
		!strings.Contains(doc, "acceptance") || !strings.Contains(doc, "checks") ||
		!strings.Contains(doc, "constraints") || !strings.Contains(doc, "depends_on") ||
		!strings.Contains(doc, "consumes") {
		t.Fatal("documentation does not state the canonical payload shape")
	}
	// (b) The reason sha256sum over the contract file never matches is
	// stated: track and id are injected from the manifest, not present in
	// the file.
	if !strings.Contains(doc, "sha256sum") {
		t.Fatal("documentation does not mention sha256sum")
	}
	if !strings.Contains(doc, "injected") {
		t.Fatal("documentation does not explain the injection of track and id")
	}
	// (c) The stability promise is stated: the canonical payload shape is
	// frozen, every existing contract digest stays byte-for-byte stable.
	if !strings.Contains(doc, "frozen") || !strings.Contains(doc, "stable") {
		t.Fatal("documentation does not state the stability promise")
	}
}

// TestDigestAddressedInPlaceRevision proves A2: a plan revision that revises
// one contract's bytes in place — same contract_path, new digest in the
// manifest — records cleanly, reads cleanly via readState, and the old
// revision's contract is still resolvable from its old digest-addressed
// store path at the historical release head. The rev2/-rev5/ directory
// pattern disappears because a revised contract no longer needs a new path;
// the path is provenance, the digest is identity.
func TestDigestAddressedInPlaceRevision(t *testing.T) {
	t.Parallel()
	repoPath, repository, actions := createActionHarness(t)
	release := "in-place-revision"
	contractPath := "contracts/S1.json"

	// Revision 1: contract with outcome "Deliver S1."
	contractRawV1 := manifestContractRaw(t, manifestContractBody("S1", "one.txt"))
	_, digestV1, err := ParseSliceContract(contractRawV1, "S1", "T1")
	if err != nil {
		t.Fatal(err)
	}

	base := actionGit(t, repoPath, nil, nil, "rev-parse", "refs/heads/main")
	withContractV1 := prepareActionContractTree(t, repoPath, base, map[string][]byte{contractPath: contractRawV1})
	actionGit(t, repoPath, nil, nil, "update-ref", "refs/heads/main", withContractV1, base)

	planV1Bytes := manifestActionPlanBytes(t, release, contractPath, "one.txt", digestV1, []any{})
	resultV1, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: planV1Bytes, ContractTree: withContractV1,
		Summary: "Approve revision 1.", Detail: []byte("approval"),
	})
	if err != nil || !resultV1.Changed {
		t.Fatalf("record revision 1: result = %#v, err = %v", resultV1, err)
	}

	// Read state at revision 1 — resolves from the digest store.
	stateV1 := readActionState(t, repository, release)
	if stateV1.Plan.Metadata.Contracts["S1"] != digestV1 {
		t.Fatalf("revision 1 contract digest = %q, want %q", stateV1.Plan.Metadata.Contracts["S1"], digestV1)
	}

	// Revision 2: same contract_path, revised contract bytes (new outcome).
	contractBodyV2 := manifestContractBody("S1", "one.txt")
	contractBodyV2["outcome"] = "Deliver revised S1."
	contractRawV2 := manifestContractRaw(t, contractBodyV2)
	_, digestV2, err := ParseSliceContract(contractRawV2, "S1", "T1")
	if err != nil {
		t.Fatal(err)
	}
	if digestV1 == digestV2 {
		t.Fatal("revised contract must have a different digest")
	}

	withContractV2 := prepareActionContractTree(t, repoPath, withContractV1, map[string][]byte{contractPath: contractRawV2})
	actionGit(t, repoPath, nil, nil, "update-ref", "refs/heads/main", withContractV2, withContractV1)

	// Build revision 2 manifest: same contract_path, new digest, revision 2.
	// The manifest entry's outcome preview must match the revised contract.
	rev2Entry := manifestSliceEntry("S1", contractPath, "one.txt", digestV2)
	rev2Entry["outcome"] = "Deliver revised S1."
	planV2Value := map[string]any{
		"schema_version": ManifestVersion, "release": release, "revision": int64(2),
		"previous_plan": resultV1.Plan, "repository": "golden/sworn",
		"target_ref":    "refs/heads/main",
		"approval_ref":  "golden://approval/" + release + "/2",
		"tracks": []any{
			map[string]any{
				"id": "T1", "depends_on": []any{},
				"slices": []any{rev2Entry},
			},
		},
	}
	planV2Bytes := manifestRaw(t, planV2Value)
	resultV2, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: planV2Bytes, ContractTree: withContractV2,
		Summary: "Approve revision 2.", Detail: []byte("approval 2"),
	})
	if err != nil || !resultV2.Changed {
		t.Fatalf("record revision 2: result = %#v, err = %v", resultV2, err)
	}

	// Read state at revision 2 — resolves from the new digest store.
	stateV2 := readActionState(t, repository, release)
	if stateV2.Plan.Metadata.Revision != 2 {
		t.Fatalf("revision = %d, want 2", stateV2.Plan.Metadata.Revision)
	}
	if stateV2.Plan.Metadata.Contracts["S1"] != digestV2 {
		t.Fatalf("revision 2 contract digest = %q, want %q", stateV2.Plan.Metadata.Contracts["S1"], digestV2)
	}

	// The new digest store path exists at the revision 2 release head.
	storeV2 := contractStorePath(RecordRoot, release, digestV2)
	storeFileV2, err := actions.repository.file(resultV2.Head, storeV2)
	if err != nil || !storeFileV2.Present || !bytes.Equal(storeFileV2.Bytes, contractRawV2) {
		t.Fatalf("digest store v2 = %#v, err = %v", storeFileV2, err)
	}

	// The old digest store path still exists at the revision 1 release head
	// (historical commits are ancestors of the current release head).
	storeV1 := contractStorePath(RecordRoot, release, digestV1)
	storeFileV1, err := actions.repository.file(resultV1.Head, storeV1)
	if err != nil || !storeFileV1.Present || !bytes.Equal(storeFileV1.Bytes, contractRawV1) {
		t.Fatalf("digest store v1 at historical head = %#v, err = %v", storeFileV1, err)
	}
}