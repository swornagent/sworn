package baton

import (
	"testing"
)

// TestAcceptanceIdentityIgnoresChecksAndHostChecks proves A1: acceptanceIdentity
// is unchanged by an edit to Checks or HostChecks alone, while the fused
// contract digest validateSliceBody computes changes, and every other field
// changes acceptanceIdentity.
func TestAcceptanceIdentityIgnoresChecksAndHostChecks(t *testing.T) {
	t.Parallel()
	base := func() Slice {
		return Slice{
			ID: "S1", Outcome: "Deliver S1.",
			Scope:       Scope{Include: []string{"one.txt"}, Exclude: []string{"two.txt"}},
			Acceptance:  []Criterion{{ID: "A-S1", Text: "S1 is exact."}},
			Checks:      []string{"check one"},
			Constraints: []string{"deterministic"},
			DependsOn:   []string{"S0"},
			Consumes:    []string{"S0"},
		}
	}
	fusedDigestOf := func(t *testing.T, slice Slice) string {
		t.Helper()
		raw := actionSlicePlanContractJSON(t, slice)
		_, digest, err := ParseSliceContract(raw, slice.ID, "T1")
		if err != nil {
			t.Fatal(err)
		}
		return digest
	}
	original := base()
	originalID, err := acceptanceIdentity("T1", original)
	if err != nil {
		t.Fatal(err)
	}
	originalFused := fusedDigestOf(t, original)

	checksOnly := base()
	checksOnly.Checks = []string{"a totally different check"}
	checksOnlyID, err := acceptanceIdentity("T1", checksOnly)
	if err != nil {
		t.Fatal(err)
	}
	if checksOnlyID != originalID {
		t.Fatalf("acceptanceIdentity changed on a checks-only edit: %q vs %q", checksOnlyID, originalID)
	}
	if fusedDigestOf(t, checksOnly) == originalFused {
		t.Fatal("fused digest did not change on a checks-only edit (test fixture is not exercising the split)")
	}

	hostChecksOnly := base()
	hostChecksOnly.HostChecks = []string{"check one"}
	hostChecksOnlyID, err := acceptanceIdentity("T1", hostChecksOnly)
	if err != nil {
		t.Fatal(err)
	}
	if hostChecksOnlyID != originalID {
		t.Fatalf("acceptanceIdentity changed on a host_checks-only edit: %q vs %q", hostChecksOnlyID, originalID)
	}

	mutators := map[string]func(*Slice){
		"outcome":       func(s *Slice) { s.Outcome = "Deliver something else." },
		"scope.include": func(s *Slice) { s.Scope.Include = []string{"other.txt"} },
		"scope.exclude": func(s *Slice) { s.Scope.Exclude = []string{"other.txt"} },
		"scope.waivers": func(s *Slice) { s.Scope.Waivers = []ScopeWaiver{{Package: "pkg", Reason: "reason"}} },
		"acceptance":    func(s *Slice) { s.Acceptance = []Criterion{{ID: "A-S1", Text: "S1 is precisely exact."}} },
		"constraints":   func(s *Slice) { s.Constraints = []string{"different constraint"} },
		"depends_on":    func(s *Slice) { s.DependsOn = []string{"S9"} },
		"consumes":      func(s *Slice) { s.Consumes = []string{"S9"} },
	}
	for name, mutate := range mutators {
		t.Run(name, func(t *testing.T) {
			mutated := base()
			mutate(&mutated)
			mutatedID, err := acceptanceIdentity("T1", mutated)
			if err != nil {
				t.Fatal(err)
			}
			if mutatedID == originalID {
				t.Fatalf("acceptanceIdentity did not change on a %s edit", name)
			}
		})
	}
}

// actionSlicePlanContractJSON renders slice as the standalone contract JSON
// body ParseSliceContract admits, so tests can recompute the fused digest
// for a fixture Slice value without going through a full plan.
func actionSlicePlanContractJSON(t *testing.T, slice Slice) []byte {
	t.Helper()
	body := map[string]any{
		"outcome":     slice.Outcome,
		"scope":       map[string]any{"include": slice.Scope.Include, "exclude": slice.Scope.Exclude},
		"acceptance":  criteriaAny(slice.Acceptance),
		"checks":      slice.Checks,
		"constraints": slice.Constraints,
		"depends_on":  slice.DependsOn,
		"consumes":    slice.Consumes,
	}
	if len(slice.Scope.Waivers) > 0 {
		body["scope"].(map[string]any)["waivers"] = waiversAny(slice.Scope.Waivers)
	}
	if len(slice.HostChecks) > 0 {
		body["host_checks"] = slice.HostChecks
	}
	return manifestContractRaw(t, body)
}

// TestChecksOnlyRevisionRetainsPassAcceptanceChangeVoids proves A2 for the
// legacy baton.plan/v2 schema: a plan revision that edits only S1's checks
// list leaves S1's PASS retained (the receipt still matches the split
// acceptance identity), while a later revision editing S1's acceptance
// criteria (leaving checks untouched) voids it, exactly as a fused-identity
// change already does today.
func TestChecksOnlyRevisionRetainsPassAcceptanceChangeVoids(t *testing.T) {
	repoPath, repository, actions := createActionHarness(t)
	release := "checks-only-legacy"
	s1 := actionSlice("S1", "one.txt")
	tracks := []Track{{ID: "T1", DependsOn: []string{}, Slices: []Slice{s1}}}

	recorded, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: actionPlanRevisionBytes(release, 1, nil, tracks),
		Summary:   "Approve revision one.", Detail: []byte("approval one"),
	})
	if err != nil {
		t.Fatal(err)
	}
	advanceActionSlice(t, actions, repoPath, release, "T1", "S1", "one.txt", 1000000500, "pass")

	checksOnlyS1 := s1
	checksOnlyS1.Checks = []string{"a completely different check"}
	previous := recorded.Plan
	revisionTwo, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: actionPlanRevisionBytes(release, 2, &previous, []Track{
			{ID: "T1", DependsOn: []string{}, Slices: []Slice{checksOnlyS1}},
		}),
		Summary: "Approve revision two (checks only).", Detail: []byte("approval two"),
	})
	if err != nil {
		t.Fatal(err)
	}
	state := readActionState(t, repository, release)
	s1State, _ := state.Slice("S1")
	if s1State.Pass == nil || !s1State.Retained {
		t.Fatalf("checks-only revision voided S1's PASS: %#v", s1State)
	}

	acceptanceChangedS1 := checksOnlyS1
	acceptanceChangedS1.Acceptance = []Criterion{{ID: "A-S1", Text: "S1 is precisely exact, not merely exact."}}
	previous = revisionTwo.Plan
	if _, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: actionPlanRevisionBytes(release, 3, &previous, []Track{
			{ID: "T1", DependsOn: []string{}, Slices: []Slice{acceptanceChangedS1}},
		}),
		Summary: "Approve revision three (acceptance changed).", Detail: []byte("approval three"),
	}); err != nil {
		t.Fatal(err)
	}
	state = readActionState(t, repository, release)
	s1State, _ = state.Slice("S1")
	if s1State.Pass != nil || s1State.Attempt != 2 || s1State.Stage != "design" {
		t.Fatalf("acceptance-changed revision did not void S1's PASS: %#v", s1State)
	}
}

// TestManifestChecksOnlyRevisionRetainsPassAcceptanceChangeVoids proves A2
// for the sworn.release-manifest/v1 schema this release introduces: the
// manifest's own slice declaration is a stub carrying no acceptance,
// constraints, or scope.exclude (validateManifestSlice), so acceptanceIdentity
// must be sourced from the fully resolved contract body read through the
// digest-addressed store (S1) at the release head, not from that stub. A
// checks-only contract revision leaves S1's PASS retained; a later revision
// changing only the contract's acceptance text (checks unchanged) voids it.
func TestManifestChecksOnlyRevisionRetainsPassAcceptanceChangeVoids(t *testing.T) {
	repoPath, repository, actions := createActionHarness(t)
	release := "checks-only-manifest"
	contractPath := "contracts/S1.json"

	contractBodyV1 := manifestContractBody("S1", "one.txt")
	contractRawV1 := manifestContractRaw(t, contractBodyV1)
	_, digestV1, err := ParseSliceContract(contractRawV1, "S1", "T1")
	if err != nil {
		t.Fatal(err)
	}
	base := actionGit(t, repoPath, nil, nil, "rev-parse", "refs/heads/main")
	treeV1 := prepareActionContractTree(t, repoPath, base, map[string][]byte{contractPath: contractRawV1})
	actionGit(t, repoPath, nil, nil, "update-ref", "refs/heads/main", treeV1, base)

	planV1 := manifestActionPlanBytes(t, release, contractPath, "one.txt", digestV1, []any{})
	resultV1, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: planV1, ContractTree: treeV1,
		Summary: "Approve revision one.", Detail: []byte("approval one"),
	})
	if err != nil || !resultV1.Changed {
		t.Fatalf("record revision 1: result = %#v, err = %v", resultV1, err)
	}
	advanceActionSlice(t, actions, repoPath, release, "T1", "S1", "one.txt", 1000000500, "pass")

	// Revision 2: checks-only edit. Acceptance, outcome, scope, constraints
	// are byte-identical to revision 1.
	contractBodyV2 := manifestContractBody("S1", "one.txt")
	contractBodyV2["checks"] = []any{"a completely different check"}
	contractRawV2 := manifestContractRaw(t, contractBodyV2)
	_, digestV2, err := ParseSliceContract(contractRawV2, "S1", "T1")
	if err != nil {
		t.Fatal(err)
	}
	if digestV2 == digestV1 {
		t.Fatal("checks-only revision must still change the fused digest")
	}
	treeV2 := prepareActionContractTree(t, repoPath, treeV1, map[string][]byte{contractPath: contractRawV2})
	actionGit(t, repoPath, nil, nil, "update-ref", "refs/heads/main", treeV2, treeV1)

	entryV2 := manifestSliceEntry("S1", contractPath, "one.txt", digestV2)
	planV2Value := map[string]any{
		"schema_version": ManifestVersion, "release": release, "revision": int64(2),
		"previous_plan": resultV1.Plan, "repository": "golden/sworn",
		"target_ref": "refs/heads/main", "approval_ref": "golden://approval/" + release + "/2",
		"tracks": []any{map[string]any{"id": "T1", "depends_on": []any{}, "slices": []any{entryV2}}},
	}
	resultV2, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: manifestRaw(t, planV2Value), ContractTree: treeV2,
		Summary: "Approve revision two (checks only).", Detail: []byte("approval two"),
	})
	if err != nil || !resultV2.Changed {
		t.Fatalf("record revision 2: result = %#v, err = %v", resultV2, err)
	}
	state := readActionState(t, repository, release)
	s1State, _ := state.Slice("S1")
	if s1State.Pass == nil || !s1State.Retained {
		t.Fatalf("checks-only manifest revision voided S1's PASS: %#v", s1State)
	}

	// Revision 3: acceptance-only edit (checks stay at revision 2's value).
	contractBodyV3 := manifestContractBody("S1", "one.txt")
	contractBodyV3["checks"] = []any{"a completely different check"}
	contractBodyV3["acceptance"] = []any{map[string]any{"id": "A-S1", "text": "S1 is precisely exact, not merely exact."}}
	contractRawV3 := manifestContractRaw(t, contractBodyV3)
	_, digestV3, err := ParseSliceContract(contractRawV3, "S1", "T1")
	if err != nil {
		t.Fatal(err)
	}
	treeV3 := prepareActionContractTree(t, repoPath, treeV2, map[string][]byte{contractPath: contractRawV3})
	actionGit(t, repoPath, nil, nil, "update-ref", "refs/heads/main", treeV3, treeV2)

	entryV3 := manifestSliceEntry("S1", contractPath, "one.txt", digestV3)
	planV3Value := map[string]any{
		"schema_version": ManifestVersion, "release": release, "revision": int64(3),
		"previous_plan": resultV2.Plan, "repository": "golden/sworn",
		"target_ref": "refs/heads/main", "approval_ref": "golden://approval/" + release + "/3",
		"tracks": []any{map[string]any{"id": "T1", "depends_on": []any{}, "slices": []any{entryV3}}},
	}
	if _, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: manifestRaw(t, planV3Value), ContractTree: treeV3,
		Summary: "Approve revision three (acceptance changed).", Detail: []byte("approval three"),
	}); err != nil {
		t.Fatal(err)
	}
	state = readActionState(t, repository, release)
	s1State, _ = state.Slice("S1")
	if s1State.Pass != nil || s1State.Attempt != 2 || s1State.Stage != "design" {
		t.Fatalf("acceptance-changed manifest revision did not void S1's PASS: %#v", s1State)
	}
}
