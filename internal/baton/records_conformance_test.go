package baton_test

// Records-conformance sweep for the S12 record migration (sworn#48 data half).
//
// AC-03 / AC-06: every migrated spec.json and board.json across the five
// spec-v1-era releases validates against the vendored v0.10.0 spec-v1 / board-v1
// schema under full draft-2020-12 evaluation (baton.ValidateSchema, not the
// lenient hand-rolled baton.Validate). This doubles as durable CI regression —
// a future un-migrated record fails here — and Rule 1 reachability: the real
// committed records flow through the real strict validator, no fixture.
//
// AC-07 (sworn#95): after the type->ears_pattern migration + the ears.go reader
// repoint, the EARS classifier reads the migrated records and does NOT collapse
// every AC to Ubiquitous — the pre-migration all-Ubiquitous degradation is the
// regression this guards.
//
// One test file beyond S12's declared touchpoints (Captain acknowledged, review
// pin 3): no CLI surface runs ValidateSchema over on-disk records, so this Go
// test is the AC-03/AC-06 sweep mechanism.

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/ears"
)

// specV1EraReleases are the five releases that carry spec.json records and are
// in scope for the S12 v0.10.0 migration. Pre-spec-v1 legacy releases
// (markdown-era, 0 spec.json) are excluded (Coach decision 2026-07-10).
var specV1EraReleases = []string{
	"2026-06-28-driver-contract",
	"2026-06-30-sworn-operational-readiness",
	"2026-07-01-loop-cli-ux",
	"2026-07-01-release-hygiene",
	"2026-07-01-render-drift-reconciliation",
}

// repoRoot walks up from this test file to the directory that contains
// docs/release — the repo (worktree) root — so the sweep reads the real
// committed records rather than a temp fixture.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(file)
	for i := 0; i < 12; i++ {
		if fi, err := os.Stat(filepath.Join(dir, "docs", "release")); err == nil && fi.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("could not locate repo root (docs/release) from %s", file)
	return ""
}

// TestRecordsConformance_SpecV1Era proves AC-03 / AC-06: every migrated spec.json
// and board.json across the five spec-v1-era releases conforms to the strict
// vendored v0.10.0 schema.
func TestRecordsConformance_SpecV1Era(t *testing.T) {
	root := repoRoot(t)
	specCount, boardCount := 0, 0
	for _, rel := range specV1EraReleases {
		relDir := filepath.Join(root, "docs", "release", rel)
		if fi, err := os.Stat(relDir); err != nil || !fi.IsDir() {
			t.Fatalf("spec-v1-era release dir missing: %s", relDir)
		}

		specs, err := filepath.Glob(filepath.Join(relDir, "S*", "spec.json"))
		if err != nil {
			t.Fatal(err)
		}
		if len(specs) == 0 {
			t.Errorf("release %s: no spec.json found (glob broken or release un-migrated)", rel)
		}
		for _, p := range specs {
			data, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			if err := baton.ValidateSchema("spec-v1", data); err != nil {
				t.Errorf("spec-v1 conformance FAIL %s:\n  %v", p, err)
			}
			specCount++
		}

		boardPath := filepath.Join(relDir, "board.json")
		data, err := os.ReadFile(boardPath)
		if err != nil {
			t.Fatalf("read board.json %s: %v", boardPath, err)
		}
		if err := baton.ValidateSchema("board-v1", data); err != nil {
			t.Errorf("board-v1 conformance FAIL %s:\n  %v", boardPath, err)
		}
		boardCount++
	}

	// Fail closed: the sweep must actually have validated records. The five
	// releases carry 15/6/3/2/7 = 33 spec.json; a broken glob that validates
	// nothing must not read as PASS.
	if specCount < 33 {
		t.Fatalf("expected >=33 spec.json across the five spec-v1-era releases, validated %d — glob likely broken", specCount)
	}
	if boardCount != len(specV1EraReleases) {
		t.Fatalf("expected %d board.json, validated %d", len(specV1EraReleases), boardCount)
	}
	t.Logf("records-conformance PASS: %d spec.json + %d board.json validate against v0.10.0 spec-v1/board-v1", specCount, boardCount)
}

// TestRecordsConformance_EARSClassificationPreserved proves AC-07 on real
// migrated data (sworn#95): running the EARS classifier over the migrated
// driver-contract release does NOT collapse every AC to Ubiquitous — the
// event-driven and unwanted-behaviour ACs are still classified as such. The
// pre-fix stale reader (reading the now-absent ears_keyword) produced an
// all-Ubiquitous distribution; this test fails closed on that regression.
func TestRecordsConformance_EARSClassificationPreserved(t *testing.T) {
	root := repoRoot(t)
	relDir := filepath.Join(root, "docs", "release", "2026-06-28-driver-contract")

	report, err := ears.Validate(relDir)
	if err != nil {
		t.Fatalf("ears.Validate(%s): %v", relDir, err)
	}
	if report.HasViolations() {
		t.Fatalf("unexpected EARS violations in migrated release: %d", len(report.Violations))
	}
	if report.Dist[ears.PatternEventDriven] == 0 {
		t.Error("event-driven ACs classified as 0 — EARS classification degraded (sworn#95 regression)")
	}
	if report.Dist[ears.PatternUnwanted] == 0 {
		t.Error("unwanted-behaviour ACs classified as 0 — EARS classification degraded (sworn#95 regression)")
	}
	if report.TotalACs > 0 && report.Dist[ears.PatternUbiquitous] == report.TotalACs {
		t.Errorf("ALL %d ACs classified Ubiquitous — the sworn#95 all-Ubiquitous degradation", report.TotalACs)
	}
	t.Logf("EARS classification preserved on migrated data: ubiquitous=%d event-driven=%d state-driven=%d unwanted-behaviour=%d total=%d",
		report.Dist[ears.PatternUbiquitous], report.Dist[ears.PatternEventDriven],
		report.Dist[ears.PatternStateDriven], report.Dist[ears.PatternUnwanted], report.TotalACs)
}
