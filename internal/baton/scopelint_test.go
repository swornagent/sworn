package baton

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

func TestBuildPackageGraphASTAndBuildConstraints(t *testing.T) {
	t.Parallel()
	// Build an in-memory fs.FS with multiple packages, build tags, and test files.
	mockFS := fstest.MapFS{
		"go.mod": &fstest.MapFile{
			Data: []byte("module github.com/swornagent/sworn\n\ngo 1.26\n"),
		},
		"internal/alpha/alpha.go": &fstest.MapFile{
			Data: []byte("//go:build linux\npackage alpha\n\nimport \"github.com/swornagent/sworn/internal/base\"\n"),
		},
		"internal/alpha/alpha_other.go": &fstest.MapFile{
			Data: []byte("//go:build !linux\npackage alpha\n\nimport \"github.com/swornagent/sworn/internal/base\"\n"),
		},
		"internal/alpha/alpha_test.go": &fstest.MapFile{
			Data: []byte("package alpha_test\n\nimport (\n\t\"testing\"\n\t\"github.com/swornagent/sworn/internal/helper\"\n)\nfunc TestAlpha(t *testing.T) {}\n"),
		},
		"internal/base/base.go": &fstest.MapFile{
			Data: []byte("package base\n"),
		},
		"internal/helper/helper.go": &fstest.MapFile{
			Data: []byte("package helper\n\nimport \"github.com/swornagent/sworn/internal/base\"\n"),
		},
		"cmd/cli/main.go": &fstest.MapFile{
			Data: []byte("package main\n\nimport \"github.com/swornagent/sworn/internal/alpha\"\n"),
		},
	}

	graph, err := BuildPackageGraphFS(mockFS)
	if err != nil {
		t.Fatalf("BuildPackageGraphFS failed: %v", err)
	}

	if graph.Module != "github.com/swornagent/sworn" {
		t.Fatalf("Module = %q, want github.com/swornagent/sworn", graph.Module)
	}

	// Verify internal/alpha imports base (prod) and helper (test)
	alpha, ok := graph.Packages["internal/alpha"]
	if !ok {
		t.Fatal("missing package internal/alpha")
	}
	if !slices.Contains(alpha.ProdImports, "internal/base") {
		t.Fatalf("alpha ProdImports = %v, want internal/base", alpha.ProdImports)
	}
	if !slices.Contains(alpha.AllImports, "internal/helper") {
		t.Fatalf("alpha AllImports = %v, want internal/helper", alpha.AllImports)
	}
	if !alpha.TestDeps["internal/base"] || !alpha.TestDeps["internal/helper"] {
		t.Fatalf("alpha TestDeps = %v, want base and helper", alpha.TestDeps)
	}

	// Verify cmd/cli imports alpha, and transitively pulls in base (prod closure)
	cli, ok := graph.Packages["cmd/cli"]
	if !ok {
		t.Fatal("missing package cmd/cli")
	}
	if !cli.TestDeps["internal/alpha"] || !cli.TestDeps["internal/base"] {
		t.Fatalf("cli TestDeps = %v, want alpha and base", cli.TestDeps)
	}
}

// TestHistoricalScopeRegressionCorpus reproduces the four historical under-derived scope instances (A2).
func TestHistoricalScopeRegressionCorpus(t *testing.T) {
	t.Parallel()
	// Build the package graph from the actual repository filesystem.
	repoFS := os.DirFS("../..")
	graph, err := BuildPackageGraphFS(repoFS)
	if err != nil {
		t.Fatalf("BuildPackageGraphFS on repo failed: %v", err)
	}

	tests := []struct {
		name         string
		slice        Slice
		wantMissing  []string
		wantReasonIn []string
	}{
		{
			name: "Case 1: Conformance-gate pin gap (2026-08-11-conformance-gate-boundary rev1 S1)",
			// Touchpoints: internal/runtime, internal/baton, internal/journal
			// Missing: internal/driver (whose tests import internal/baton)
			slice: Slice{
				ID: "S1",
				Scope: Scope{
					Include: []string{"internal/runtime", "internal/baton", "internal/journal"},
				},
			},
			wantMissing:  []string{"internal/driver"},
			wantReasonIn: []string{"test imports internal/baton"},
		},
		{
			name: "Case 2: Configurable-paths S1 scope gap (2026-08-12-configurable-paths rev1 S1)",
			// Touchpoints: internal/runtime, internal/gitx, internal/driver, cmd/sworn
			// Missing: internal/baton and internal/skill (which import internal/gitx)
			slice: Slice{
				ID: "S1",
				Scope: Scope{
					Include: []string{"internal/runtime", "internal/gitx", "internal/driver", "cmd/sworn"},
				},
			},
			wantMissing:  []string{"internal/baton", "internal/skill"},
			wantReasonIn: []string{"test imports internal/gitx"},
		},
		{
			name: "Case 3: Configurable-paths S2 scope gap (2026-08-12-configurable-paths rev1 S2)",
			// Touchpoints: internal/baton, internal/gitx, internal/driver, internal/runtime
			// Missing: cmd/sworn, internal/skill, tools/batongolden
			slice: Slice{
				ID: "S2",
				Scope: Scope{
					Include: []string{"internal/baton", "internal/gitx", "internal/driver", "internal/runtime"},
				},
			},
			wantMissing:  []string{"cmd/sworn", "internal/skill", "tools/batongolden"},
			wantReasonIn: []string{"test imports internal/baton"},
		},
		{
			name: "Case 4: Golden-corpus gap (2026-08-12-configurable-paths rev3 S2 omitting tools)",
			// Scope touching internal/baton, internal/gitx, internal/driver, internal/runtime, cmd/sworn, internal/skill but omitting tools
			// Missing: tools/batongolden (imports internal/baton & fixture pins it)
			slice: Slice{
				ID: "S2",
				Scope: Scope{
					Include: []string{"internal/baton", "internal/gitx", "internal/driver", "internal/runtime", "cmd/sworn", "internal/skill"},
				},
			},
			wantMissing:  []string{"tools/batongolden"},
			wantReasonIn: []string{"fixture tooling pins internal/baton", "test imports internal/baton"},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := graph.LintSlice(tc.slice)
			if err == nil {
				t.Fatalf("expected LintSlice to fail, but it passed")
			}
			recErr, ok := err.(*RecordError)
			if !ok {
				t.Fatalf("expected *RecordError, got %T (%v)", err, err)
			}
			if recErr.Code != "UNDER_DERIVED_SCOPE" {
				t.Fatalf("error code = %q, want UNDER_DERIVED_SCOPE", recErr.Code)
			}

			// Verify each expected missing package is in recErr.Paths
			for _, expectedPkg := range tc.wantMissing {
				if !slices.Contains(recErr.Paths, expectedPkg) {
					t.Errorf("missing expected package %q in paths %v", expectedPkg, recErr.Paths)
				}
			}

			// Verify expected reasons appear in error message
			for _, reason := range tc.wantReasonIn {
				if !strings.Contains(recErr.Msg, reason) {
					t.Errorf("expected reason %q in error message: %s", reason, recErr.Msg)
				}
			}
		})
	}
}

// TestScopeWaiversPermitAdmissionAndPreserveBytes tests A3: explicit waivers and digest invariance.
func TestScopeWaiversPermitAdmissionAndPreserveBytes(t *testing.T) {
	t.Parallel()
	repoFS := os.DirFS("../..")
	graph, err := BuildPackageGraphFS(repoFS)
	if err != nil {
		t.Fatalf("BuildPackageGraphFS on repo failed: %v", err)
	}

	// A slice that would otherwise be refused for missing internal/driver
	unwaivedSlice := Slice{
		ID: "S1",
		Scope: Scope{
			Include: []string{"internal/runtime", "internal/baton", "internal/journal"},
		},
	}
	if err := graph.LintSlice(unwaivedSlice); err == nil {
		t.Fatal("expected unwaived slice to fail lint")
	}

	// Waiving internal/driver and all other reverse dependents allows it to pass
	waivedSlice := Slice{
		ID: "S1",
		Scope: Scope{
			Include: []string{"internal/runtime", "internal/baton", "internal/journal"},
			Waivers: []ScopeWaiver{
				{Package: "internal/driver", Reason: "Driver changes decoupled"},
				{Package: "cmd/sworn", Reason: "CLI changes decoupled"},
				{Package: "internal/cockpit", Reason: "Cockpit decoupled"},
				{Package: "internal/observe", Reason: "Observe decoupled"},
				{Package: "internal/skill", Reason: "Skill decoupled"},
				{Package: "internal/tui", Reason: "TUI decoupled"},
				{Package: "tools/batongolden", Reason: "Golden corpus decoupled"},
			},
		},
	}
	if err := graph.LintSlice(waivedSlice); err != nil {
		t.Fatalf("expected waived slice to pass lint, got: %v", err)
	}

	// Test hierarchical waiver matching (e.g. waiving "tools" covers "tools/batongolden")
	if !isWaived("tools/batongolden", []ScopeWaiver{{Package: "tools", Reason: "All tools waived"}}) {
		t.Fatal("expected 'tools' waiver to match 'tools/batongolden'")
	}
}

// TestCanonicalDigestInvarianceWithoutWaivers ensures existing contracts keep byte-identical digests.
func TestCanonicalDigestInvarianceWithoutWaivers(t *testing.T) {
	t.Parallel()
	contractValue := map[string]any{
		"outcome":     "Deliver S1.",
		"scope":       map[string]any{"include": []any{"internal/baton"}, "exclude": []any{}},
		"acceptance":  []any{map[string]any{"id": "A1", "text": "Text"}},
		"checks":      []any{"check 1"},
		"constraints": []any{"constraint 1"},
		"depends_on":  []any{},
		"consumes":    []any{},
	}
	raw, err := json.Marshal(contractValue)
	if err != nil {
		t.Fatal(err)
	}

	slice, digestNoWaivers, err := ParseSliceContract(raw, "S1", "T1")
	if err != nil {
		t.Fatal(err)
	}

	if len(slice.Scope.Waivers) != 0 {
		t.Fatalf("expected empty waivers, got %v", slice.Scope.Waivers)
	}

	// Empty waivers array in JSON must yield identical digest to omitted waivers
	contractValueWithEmptyWaivers := map[string]any{
		"outcome":     "Deliver S1.",
		"scope":       map[string]any{"include": []any{"internal/baton"}, "exclude": []any{}, "waivers": []any{}},
		"acceptance":  []any{map[string]any{"id": "A1", "text": "Text"}},
		"checks":      []any{"check 1"},
		"constraints": []any{"constraint 1"},
		"depends_on":  []any{},
		"consumes":    []any{},
	}
	rawEmpty, err := json.Marshal(contractValueWithEmptyWaivers)
	if err != nil {
		t.Fatal(err)
	}
	_, digestEmptyWaivers, err := ParseSliceContract(rawEmpty, "S1", "T1")
	if err != nil {
		t.Fatal(err)
	}
	if digestEmptyWaivers != digestNoWaivers {
		t.Fatalf("empty waivers digest %s != omitted waivers digest %s", digestEmptyWaivers, digestNoWaivers)
	}

	// Non-empty waivers change the digest and preserve waiver contents
	contractValueWithWaiver := map[string]any{
		"outcome": "Deliver S1.",
		"scope": map[string]any{
			"include": []any{"internal/baton"},
			"exclude": []any{},
			"waivers": []any{
				map[string]any{
					"package": "tools/batongolden",
					"reason":  "Golden tools waived for standalone baton test",
				},
			},
		},
		"acceptance":  []any{map[string]any{"id": "A1", "text": "Text"}},
		"checks":      []any{"check 1"},
		"constraints": []any{"constraint 1"},
		"depends_on":  []any{},
		"consumes":    []any{},
	}
	rawWaiver, err := json.Marshal(contractValueWithWaiver)
	if err != nil {
		t.Fatal(err)
	}
	waivedSlice, digestWithWaiver, err := ParseSliceContract(rawWaiver, "S1", "T1")
	if err != nil {
		t.Fatal(err)
	}
	if digestWithWaiver == digestNoWaivers {
		t.Fatal("digest with waiver should differ from digest without waiver")
	}
	if len(waivedSlice.Scope.Waivers) != 1 || waivedSlice.Scope.Waivers[0].Package != "tools/batongolden" ||
		waivedSlice.Scope.Waivers[0].Reason != "Golden tools waived for standalone baton test" {
		t.Fatalf("waivers not parsed correctly: %#v", waivedSlice.Scope.Waivers)
	}
}

// TestWaiverValidationFailClosed verifies invalid waiver structures fail closed.
func TestWaiverValidationFailClosed(t *testing.T) {
	t.Parallel()
	invalidCases := []struct {
		name string
		raw  string
		code string
	}{
		{
			name: "waiver_missing_reason",
			raw: `{
				"outcome": "Deliver S1.",
				"scope": {
					"include": ["internal/baton"],
					"exclude": [],
					"waivers": [{"package": "tools/batongolden"}]
				},
				"acceptance": [{"id": "A1", "text": "T"}],
				"checks": ["c"], "constraints": ["c"], "depends_on": [], "consumes": []
			}`,
			code: "MISSING_FIELD",
		},
		{
			name: "waiver_empty_reason",
			raw: `{
				"outcome": "Deliver S1.",
				"scope": {
					"include": ["internal/baton"],
					"exclude": [],
					"waivers": [{"package": "tools/batongolden", "reason": ""}]
				},
				"acceptance": [{"id": "A1", "text": "T"}],
				"checks": ["c"], "constraints": ["c"], "depends_on": [], "consumes": []
			}`,
			code: "INVALID_FIELD",
		},
		{
			name: "waiver_reserved_record_path",
			raw: `{
				"outcome": "Deliver S1.",
				"scope": {
					"include": ["internal/baton"],
					"exclude": [],
					"waivers": [{"package": ".sworn/records", "reason": "invalid"}]
				},
				"acceptance": [{"id": "A1", "text": "T"}],
				"checks": ["c"], "constraints": ["c"], "depends_on": [], "consumes": []
			}`,
			code: "RESERVED_RECORD_ROOT",
		},
		{
			name: "waiver_duplicate_package",
			raw: `{
				"outcome": "Deliver S1.",
				"scope": {
					"include": ["internal/baton"],
					"exclude": [],
					"waivers": [
						{"package": "tools/batongolden", "reason": "reason 1"},
						{"package": "tools/batongolden", "reason": "reason 2"}
					]
				},
				"acceptance": [{"id": "A1", "text": "T"}],
				"checks": ["c"], "constraints": ["c"], "depends_on": [], "consumes": []
			}`,
			code: "DUPLICATE_IDENTITY",
		},
	}

	for _, tc := range invalidCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := ParseSliceContract([]byte(tc.raw), "S1", "T1")
			if err == nil {
				t.Fatalf("expected ParseSliceContract to fail, but it passed")
			}
			if code := ErrorCode(err); code != tc.code {
				t.Fatalf("error code = %q (%v), want %q", code, err, tc.code)
			}
		})
	}
}

// TestRecordPlanRevisionScopeLintRefusesUnderDerivedPlan verifies A4 recording gate.
func TestRecordPlanRevisionScopeLintRefusesUnderDerivedPlan(t *testing.T) {
	t.Parallel()
	repoPath, _, actions := createActionHarness(t)

	// Add Go module structure to the action repo
	goMod := []byte("module github.com/swornagent/sworn\n\ngo 1.26\n")
	gitxFile := []byte("package gitx\n")
	batonFile := []byte("package baton\n\nimport \"github.com/swornagent/sworn/internal/gitx\"\n")
	driverFile := []byte("package driver\n\nimport \"github.com/swornagent/sworn/internal/baton\"\n")

	base := actionGit(t, repoPath, nil, nil, "rev-parse", "refs/heads/main")
	withModule := prepareActionContractTree(t, repoPath, base, map[string][]byte{
		"go.mod":                  goMod,
		"internal/gitx/gitx.go":   gitxFile,
		"internal/baton/baton.go": batonFile,
		"internal/driver/main.go": driverFile,
	})
	actionGit(t, repoPath, nil, nil, "update-ref", "refs/heads/main", withModule, base)

	// Under-derived plan: touching internal/baton, omitting internal/driver
	contractPath := "contracts/S1.json"
	contractBody := map[string]any{
		"outcome":     "Deliver S1.",
		"scope":       map[string]any{"include": []any{"internal/baton"}, "exclude": []any{}},
		"acceptance":  []any{map[string]any{"id": "A1", "text": "Text"}},
		"checks":      []any{"check"},
		"constraints": []any{"constraint"},
		"depends_on":  []any{}, "consumes": []any{},
	}
	contractRaw := manifestContractRaw(t, contractBody)
	_, digest, err := ParseSliceContract(contractRaw, "S1", "T1")
	if err != nil {
		t.Fatal(err)
	}

	withContract := prepareActionContractTree(t, repoPath, withModule, map[string][]byte{contractPath: contractRaw})
	actionGit(t, repoPath, nil, nil, "update-ref", "refs/heads/main", withContract, withModule)

	release := "under-derived-recording"
	planBytes := manifestActionPlanBytes(t, release, contractPath, "internal/baton", digest, []any{})

	// Should be refused with UNDER_DERIVED_SCOPE
	_, err = actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes:    planBytes,
		ContractTree: withContract,
		Summary:      "Approve under-derived plan.",
		Detail:       []byte("approval"),
	})
	if err == nil {
		t.Fatal("expected RecordPlanRevision to fail with UNDER_DERIVED_SCOPE, but it passed")
	}
	if code := ErrorCode(err); code != "UNDER_DERIVED_SCOPE" {
		t.Fatalf("error code = %q (%v), want UNDER_DERIVED_SCOPE", code, err)
	}
	recErr := err.(*RecordError)
	if !slices.Contains(recErr.Paths, "internal/driver") {
		t.Fatalf("expected internal/driver in paths %v", recErr.Paths)
	}
}

// TestRecordPlanRevisionWithWaiversSucceeds verifies A3 plan recording with waivers.
func TestRecordPlanRevisionWithWaiversSucceeds(t *testing.T) {
	t.Parallel()
	repoPath, _, actions := createActionHarness(t)

	// Add Go module structure to the action repo
	goMod := []byte("module github.com/swornagent/sworn\n\ngo 1.26\n")
	gitxFile := []byte("package gitx\n")
	batonFile := []byte("package baton\n\nimport \"github.com/swornagent/sworn/internal/gitx\"\n")
	driverFile := []byte("package driver\n\nimport \"github.com/swornagent/sworn/internal/baton\"\n")

	base := actionGit(t, repoPath, nil, nil, "rev-parse", "refs/heads/main")
	withModule := prepareActionContractTree(t, repoPath, base, map[string][]byte{
		"go.mod":                  goMod,
		"internal/gitx/gitx.go":   gitxFile,
		"internal/baton/baton.go": batonFile,
		"internal/driver/main.go": driverFile,
	})
	actionGit(t, repoPath, nil, nil, "update-ref", "refs/heads/main", withModule, base)

	// Contract with waiver for internal/driver
	contractPath := "contracts/S1.json"
	contractBody := map[string]any{
		"outcome": "Deliver S1.",
		"scope": map[string]any{
			"include": []any{"internal/baton"},
			"exclude": []any{},
			"waivers": []any{
				map[string]any{
					"package": "internal/driver",
					"reason":  "Driver changes deferred to subsequent slice",
				},
			},
		},
		"acceptance":  []any{map[string]any{"id": "A1", "text": "Text"}},
		"checks":      []any{"check"},
		"constraints": []any{"constraint"},
		"depends_on":  []any{}, "consumes": []any{},
	}
	contractRaw := manifestContractRaw(t, contractBody)
	_, digest, err := ParseSliceContract(contractRaw, "S1", "T1")
	if err != nil {
		t.Fatal(err)
	}

	withContract := prepareActionContractTree(t, repoPath, withModule, map[string][]byte{contractPath: contractRaw})
	actionGit(t, repoPath, nil, nil, "update-ref", "refs/heads/main", withContract, withModule)

	release := "waived-recording"
	entry := manifestSliceEntry("S1", contractPath, "internal/baton", digest)
	entry["waivers"] = []any{
		map[string]any{
			"package": "internal/driver",
			"reason":  "Driver changes deferred to subsequent slice",
		},
	}
	value := map[string]any{
		"schema_version": ManifestVersion, "release": release, "revision": int64(1),
		"previous_plan": nil, "repository": "golden/sworn", "target_ref": "refs/heads/main",
		"approval_ref": "golden://approval/" + release + "/1",
		"tracks": []any{
			map[string]any{
				"id": "T1", "depends_on": []any{},
				"slices": []any{entry},
			},
		},
	}
	planBytes := manifestRaw(t, value)

	result, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes:    planBytes,
		ContractTree: withContract,
		Summary:      "Approve waived plan.",
		Detail:       []byte("approval"),
	})
	if err != nil || !result.Changed {
		t.Fatalf("result = %#v, err = %v", result, err)
	}

	// Verify reading back the recorded plan and contract preserves waivers
	planFile, err := actions.repository.file(result.Head, planPath(RecordRoot, release))
	if err != nil || !planFile.Present {
		t.Fatalf("plan file = %#v, err = %v", planFile, err)
	}
	reread, err := ParsePlan(planFile.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	_, slice, ok := reread.FindSlice("S1")
	if !ok || len(slice.Scope.Waivers) != 1 {
		t.Fatalf("reread slice waivers = %v", slice.Scope.Waivers)
	}
	if slice.Scope.Waivers[0].Package != "internal/driver" ||
		slice.Scope.Waivers[0].Reason != "Driver changes deferred to subsequent slice" {
		t.Fatalf("reread waiver text mismatch: %#v", slice.Scope.Waivers[0])
	}
}
