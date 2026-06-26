// Package tui provides the Bubble Tea TUI for the SwornAgent CLI.
//
// gate.go implements per-slice gate result display in the board view (S72).
// It reads gate results from the sworn lint commands (S65-S70) — trace,
// coverage, design, mock, and LLM check — computing them on-the-fly
// from the gate package.
package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/swornagent/sworn/internal/gate"
)

// GateResult holds per-slice gate check results for compact TUI display.
// Zero values mean "not checked" / "not applicable".
type GateResult struct {
	TraceVerdict string // "PASS" or "FAIL" (from trace report), "" if not checked
	CoveragePct  string // "8/10" (covered/total), "" if not checked
	DesignCount  int    // violation count, 0 = clean, -1 = not checked (default -1)
	MockStatus   string // "clean" or "flagged", "" if not checked
	LLMResult    string // "PASS" or "FAIL", "" if not checked / not run
}

// IsClean returns true when no gates are flagged (all PASS/clean/0 violations).
func (g *GateResult) IsClean() bool {
	if g.TraceVerdict == "" && g.CoveragePct == "" && g.DesignCount < 0 && g.MockStatus == "" && g.LLMResult == "" {
		return false // no data at all — not "clean", just unchecked
	}
	return g.TraceVerdict != "FAIL" &&
		(g.CoveragePct == "" || !isPartialCoverage(g.CoveragePct)) &&
		(g.DesignCount < 0 || g.DesignCount == 0) &&
		g.MockStatus != "flagged" &&
		g.LLMResult != "FAIL"
}

// HasFailures returns true when any gate has a hard failure.
func (g *GateResult) HasFailures() bool {
	return g.TraceVerdict == "FAIL" ||
		g.MockStatus == "flagged" ||
		g.LLMResult == "FAIL" ||
		g.DesignCount > 0
}

// isPartialCoverage returns true when "N/M" has N < M.
func isPartialCoverage(s string) bool {
	parts := strings.SplitN(s, "/", 2)
	return len(parts) == 2 && parts[0] != "" && parts[1] != "" && parts[0] != parts[1]
}

// RenderInline returns a compact one-line gate status string for the board view.
// Format: "[T:✓ C:8/10 D:0 M:✓]" with ANSI colouring.
func (g *GateResult) RenderInline() string {
	parts := []string{
		g.renderTrace(),
		g.renderCoverage(),
		g.renderDesign(),
		g.renderMock(),
		g.renderLLM(),
	}
	var nonEmpty []string
	for _, p := range parts {
		if p != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}
	if len(nonEmpty) == 0 {
		return GateNeutralStyle.Render("[no gates]")
	}
	return GateBracketStyle.Render("[") +
		strings.Join(nonEmpty, GateSepStyle.Render(" ")) +
		GateBracketStyle.Render("]")
}

func (g *GateResult) renderTrace() string {
	if g.TraceVerdict == "" {
		return ""
	}
	if g.TraceVerdict == "PASS" {
		return "T:" + GatePassStyle.Render("✓")
	}
	return "T:" + GateFailStyle.Render("✗")
}

func (g *GateResult) renderCoverage() string {
	if g.CoveragePct == "" {
		return ""
	}
	if isPartialCoverage(g.CoveragePct) {
		return "C:" + GateWarnStyle.Render(g.CoveragePct)
	}
	return "C:" + GatePassStyle.Render(g.CoveragePct)
}

func (g *GateResult) renderDesign() string {
	if g.DesignCount < 0 {
		return ""
	}
	if g.DesignCount == 0 {
		return "D:" + GatePassStyle.Render("0")
	}
	return fmt.Sprintf("D:%s", GateFailStyle.Render(fmt.Sprintf("%d", g.DesignCount)))
}

func (g *GateResult) renderMock() string {
	if g.MockStatus == "" {
		return ""
	}
	if g.MockStatus == "clean" {
		return "M:" + GatePassStyle.Render("✓")
	}
	return "M:" + GateFailStyle.Render("✗")
}

func (g *GateResult) renderLLM() string {
	if g.LLMResult == "" {
		return ""
	}
	if g.LLMResult == "PASS" {
		return "L:" + GatePassStyle.Render("✓")
	}
	return "L:" + GateFailStyle.Render("✗")
}

// LoadGateResults computes gate results for all slices in a release.
// repoRoot is the absolute path to the repository root.
// releaseName is the release folder name (e.g. "2026-06-19-safe-parallelism").
func LoadGateResults(repoRoot, releaseName string) map[string]GateResult {
	releaseDir := filepath.Join(repoRoot, "docs", "release", releaseName)
	results := make(map[string]GateResult)

	// Discover slice directories.
	sliceIDs := discoverSliceDirs(releaseDir)
	for _, sid := range sliceIDs {
		// Default: not checked.
		results[sid] = GateResult{DesignCount: -1}
	}

	// 1. Trace — release-level check; runs once.
	traceReport, err := gate.RunTrace(releaseDir)
	if err == nil && traceReport != nil {
		sliceTraceFails := map[string]bool{}
		for _, v := range traceReport.Violations {
			if v.Slice != "" && v.Severity == "FAIL" {
				sliceTraceFails[v.Slice] = true
			}
		}
		for _, sid := range sliceIDs {
			gr := results[sid]
			if sliceTraceFails[sid] {
				gr.TraceVerdict = "FAIL"
			} else {
				gr.TraceVerdict = "PASS"
			}
			results[sid] = gr
		}
	}

	// 2. Per-slice gates — only for slices with a start_commit (implemented+).
	for _, sid := range sliceIDs {
		sliceDir := filepath.Join(releaseDir, sid)
		baseRef, err := gate.BaseRefForSlice(sliceDir, releaseName)
		if err != nil || baseRef == "" {
			continue
		}

		gr := results[sid]

		// Coverage.
		if cov, err := gate.RunCoverage(releaseDir, sid, baseRef); err == nil && cov != nil {
			gr.CoveragePct = fmt.Sprintf("%d/%d", cov.Covered, cov.TotalACs)
		}

		// Design.
		if des, err := gate.RunDesign(releaseDir, sid, baseRef); err == nil && des != nil {
			gr.DesignCount = des.TotalViolations
		}

		// Mock.
		if mock, err := gate.RunMock(releaseDir, sid, baseRef); err == nil && mock != nil {
			if mock.TotalViolations == 0 {
				gr.MockStatus = "clean"
			} else {
				gr.MockStatus = "flagged"
			}
		}

		// LLM — read cached result if available.
		llmPath := filepath.Join(sliceDir, "llm-check.json")
		if llmResult := readLLMCached(llmPath); llmResult != "" {
			gr.LLMResult = llmResult
		}

		results[sid] = gr
	}

	return results
}

// discoverSliceDirs returns the slice IDs for all S<NN>-* directories in releaseDir.
func discoverSliceDirs(releaseDir string) []string {
	entries, err := os.ReadDir(releaseDir)
	if err != nil {
		return nil
	}
	var ids []string
	for _, e := range entries {
		if !e.IsDir() || len(e.Name()) < 3 {
			continue
		}
		name := e.Name()
		if name[0] == 'S' && name[1] >= '0' && name[1] <= '9' && name[2] >= '0' && name[2] <= '9' {
			ids = append(ids, name)
		}
	}
	return ids
}

// readLLMCached reads a cached LLM check result from a JSON file.
// Returns "PASS", "FAIL", or "" if no cached result or unreadable.
func readLLMCached(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var wrapper struct {
		Verdict string `json:"verdict"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return ""
	}
	if wrapper.Verdict == "PASS" || wrapper.Verdict == "FAIL" {
		return wrapper.Verdict
	}
	return ""
}
