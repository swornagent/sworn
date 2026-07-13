package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/swornagent/sworn/internal/config"
	"github.com/swornagent/sworn/internal/ears"
	"github.com/swornagent/sworn/internal/gate"
	"github.com/swornagent/sworn/internal/lint"
	"github.com/swornagent/sworn/internal/model"
)

// RegisterLintTools registers the gate-engine lint tools on the MCP server.
//
// Six tools, matching the CLI counterparts from S65-S70:
//
//	sworn.lint         — composite: runs all mechanical lint checks for a release (or release+slice)
//	sworn.lint_trace    — RTM + EARS traceability check (release-level)
//	sworn.lint_coverage — AC → test coverage mapping (slice-level)
//	sworn.lint_design   — design conformance + architecture rules (slice-level)
//	sworn.lint_mock     — mock boundary enforcement (slice-level)
//	sworn.llm_check     — LLM quality check (slice-level, requires model)
func RegisterLintTools(s *Server, repoRoot string) {
	// ---- 1. sworn.lint (composite) ----
	s.RegisterTool("sworn.lint", json.RawMessage(`{
		"type": "object",
		"properties": {
			"release":  {"type": "string", "description": "Release name (e.g. 2026-06-19-safe-parallelism)"},
			"slice_id": {"type": "string", "description": "Optional slice ID for per-slice checks (coverage, design, mock)"},
			"base":     {"type": "string", "description": "Optional base ref for git diff"}
		},
		"required": ["release"]
	}`), func(ctx context.Context, params json.RawMessage) (*ToolResult, error) {
		var p struct {
			Release string `json:"release"`
			SliceID string `json:"slice_id"`
			Base    string `json:"base"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}

		releaseDir := resolveMCPReleaseDir(repoRoot, p.Release)
		if releaseDir == "" {
			return nil, fmt.Errorf("release %q not found", p.Release)
		}

		results := make(map[string]any)
		hasFailures := false

		// Release-level checks.
		addResult := func(name string, passed bool, detail any) {
			results[name] = detail
			if !passed {
				hasFailures = true
			}
		}

		// 1a. lint ac (EARS)
		acReport, err := ears.Validate(releaseDir)
		if err != nil {
			addResult("ac", false, map[string]any{"error": err.Error()})
		} else {
			addResult("ac", !acReport.HasViolations(), acReport)
		}

		// 1b. lint trace (RTM)
		traceReport, err := gate.RunTrace(releaseDir)
		if err != nil {
			addResult("trace", false, map[string]any{"error": err.Error()})
		} else {
			addResult("trace", !traceReport.HasViolations(), traceReport)
		}

		// 1c. lint status (timestamps)
		statusViolations := lint.CheckStatusTimestamps(releaseDir, lint.DefaultClock)
		addResult("status", len(statusViolations) == 0, map[string]any{
			"violations": statusViolations,
			"count":      len(statusViolations),
		})

		// Per-slice checks — only when slice_id is provided.
		if p.SliceID != "" {
			sliceDir := filepath.Join(releaseDir, p.SliceID)
			if _, err := os.Stat(sliceDir); err != nil {
				addResult("slice", false, map[string]any{"error": fmt.Sprintf("slice %q not found", p.SliceID)})
			} else {
				ref := p.Base
				if ref == "" {
					var err error
					ref, err = gate.BaseRefForSlice(sliceDir, p.Release)
					if err != nil {
						ref = "HEAD"
					}
				}

				// deps
				if err := lint.CheckDeps(sliceDir, ref); err != nil {
					addResult("deps", false, map[string]any{"error": err.Error()})
				} else {
					addResult("deps", true, map[string]any{"status": "all dependency files declared"})
				}

				// touchpoints
				if err := lint.CheckTouchpoints(sliceDir, releaseDir); err != nil {
					addResult("touchpoints", false, map[string]any{"error": err.Error()})
				} else {
					addResult("touchpoints", true, map[string]any{"status": "all references declared"})
				}

				// symbols
				if err := lint.CheckSymbols(sliceDir, repoRoot); err != nil {
					addResult("symbols", false, map[string]any{"error": err.Error()})
				} else {
					addResult("symbols", true, map[string]any{"status": "all identifiers resolved"})
				}

				// coverage
				covReport, err := gate.RunCoverage(releaseDir, p.SliceID, ref)
				if err != nil {
					addResult("coverage", false, map[string]any{"error": err.Error()})
				} else {
					addResult("coverage", !covReport.HasViolations(), covReport)
				}

				// design
				designReport, err := gate.RunDesign(releaseDir, p.SliceID, ref)
				if err != nil {
					addResult("design", false, map[string]any{"error": err.Error()})
				} else {
					addResult("design", !designReport.HasViolations(), designReport)
				}

				// mock
				mockReport, err := gate.RunMock(releaseDir, p.SliceID, ref)
				if err != nil {
					addResult("mock", false, map[string]any{"error": err.Error()})
				} else {
					addResult("mock", !mockReport.HasViolations(), mockReport)
				}
			}
		}

		result := map[string]any{
			"release":      p.Release,
			"slice_id":     p.SliceID,
			"verdict":      "PASS",
			"checks":       results,
			"total_checks": len(results),
		}
		if hasFailures {
			result["verdict"] = "FAIL"
		}

		b, _ := json.Marshal(result)
		return &ToolResult{Content: []ContentItem{{Type: "text", Text: string(b)}}}, nil
	})

	// ---- 2. sworn.lint_trace ----
	s.RegisterTool("sworn.lint_trace", json.RawMessage(`{
		"type": "object",
		"properties": {
			"release": {"type": "string", "description": "Release name"}
		},
		"required": ["release"]
	}`), func(ctx context.Context, params json.RawMessage) (*ToolResult, error) {
		var p struct {
			Release string `json:"release"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}

		releaseDir := resolveMCPReleaseDir(repoRoot, p.Release)
		if releaseDir == "" {
			return nil, fmt.Errorf("release %q not found", p.Release)
		}

		report, err := gate.RunTrace(releaseDir)
		if err != nil {
			return nil, fmt.Errorf("lint_trace: %w", err)
		}

		b, _ := json.Marshal(report)
		return &ToolResult{Content: []ContentItem{{Type: "text", Text: string(b)}}}, nil
	})

	// ---- 3. sworn.lint_coverage ----
	s.RegisterTool("sworn.lint_coverage", json.RawMessage(`{
		"type": "object",
		"properties": {
			"release":  {"type": "string", "description": "Release name"},
			"slice_id": {"type": "string", "description": "Slice ID"},
			"base":     {"type": "string", "description": "Optional base ref for git diff"}
		},
		"required": ["release", "slice_id"]
	}`), func(ctx context.Context, params json.RawMessage) (*ToolResult, error) {
		var p struct {
			Release string `json:"release"`
			SliceID string `json:"slice_id"`
			Base    string `json:"base"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}

		releaseDir := resolveMCPReleaseDir(repoRoot, p.Release)
		if releaseDir == "" {
			return nil, fmt.Errorf("release %q not found", p.Release)
		}

		sliceDir := filepath.Join(releaseDir, p.SliceID)
		if _, err := os.Stat(sliceDir); err != nil {
			return nil, fmt.Errorf("slice %q not found in release %q", p.SliceID, p.Release)
		}

		ref := p.Base
		if ref == "" {
			var err error
			ref, err = gate.BaseRefForSlice(sliceDir, p.Release)
			if err != nil {
				ref = "HEAD"
			}
		}

		report, err := gate.RunCoverage(releaseDir, p.SliceID, ref)
		if err != nil {
			return nil, fmt.Errorf("lint_coverage: %w", err)
		}

		b, _ := json.Marshal(report)
		return &ToolResult{Content: []ContentItem{{Type: "text", Text: string(b)}}}, nil
	})

	// ---- 4. sworn.lint_design ----
	s.RegisterTool("sworn.lint_design", json.RawMessage(`{
		"type": "object",
		"properties": {
			"release":  {"type": "string", "description": "Release name"},
			"slice_id": {"type": "string", "description": "Slice ID"},
			"base":     {"type": "string", "description": "Optional base ref for git diff"}
		},
		"required": ["release", "slice_id"]
	}`), func(ctx context.Context, params json.RawMessage) (*ToolResult, error) {
		var p struct {
			Release string `json:"release"`
			SliceID string `json:"slice_id"`
			Base    string `json:"base"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}

		releaseDir := resolveMCPReleaseDir(repoRoot, p.Release)
		if releaseDir == "" {
			return nil, fmt.Errorf("release %q not found", p.Release)
		}

		sliceDir := filepath.Join(releaseDir, p.SliceID)
		if _, err := os.Stat(sliceDir); err != nil {
			return nil, fmt.Errorf("slice %q not found in release %q", p.SliceID, p.Release)
		}

		ref := p.Base
		if ref == "" {
			var err error
			ref, err = gate.BaseRefForSlice(sliceDir, p.Release)
			if err != nil {
				ref = "HEAD"
			}
		}

		report, err := gate.RunDesign(releaseDir, p.SliceID, ref)
		if err != nil {
			return nil, fmt.Errorf("lint_design: %w", err)
		}

		b, _ := json.Marshal(report)
		return &ToolResult{Content: []ContentItem{{Type: "text", Text: string(b)}}}, nil
	})

	// ---- 5. sworn.lint_mock ----
	s.RegisterTool("sworn.lint_mock", json.RawMessage(`{
		"type": "object",
		"properties": {
			"release":  {"type": "string", "description": "Release name"},
			"slice_id": {"type": "string", "description": "Slice ID"},
			"base":     {"type": "string", "description": "Optional base ref for git diff"}
		},
		"required": ["release", "slice_id"]
	}`), func(ctx context.Context, params json.RawMessage) (*ToolResult, error) {
		var p struct {
			Release string `json:"release"`
			SliceID string `json:"slice_id"`
			Base    string `json:"base"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}

		releaseDir := resolveMCPReleaseDir(repoRoot, p.Release)
		if releaseDir == "" {
			return nil, fmt.Errorf("release %q not found", p.Release)
		}

		sliceDir := filepath.Join(releaseDir, p.SliceID)
		if _, err := os.Stat(sliceDir); err != nil {
			return nil, fmt.Errorf("slice %q not found in release %q", p.SliceID, p.Release)
		}

		ref := p.Base
		if ref == "" {
			var err error
			ref, err = gate.BaseRefForSlice(sliceDir, p.Release)
			if err != nil {
				ref = "HEAD"
			}
		}

		report, err := gate.RunMock(releaseDir, p.SliceID, ref)
		if err != nil {
			return nil, fmt.Errorf("lint_mock: %w", err)
		}

		b, _ := json.Marshal(report)
		return &ToolResult{Content: []ContentItem{{Type: "text", Text: string(b)}}}, nil
	})

	// ---- 6. sworn.llm_check ----
	s.RegisterTool("sworn.llm_check", json.RawMessage(`{
		"type": "object",
		"properties": {
			"release":  {"type": "string", "description": "Release name"},
			"slice_id": {"type": "string", "description": "Slice ID"},
			"type":     {"type": "string", "description": "Check type: ac-satisfaction, spec-ambiguity, design-review, security-review, semantic-coverage, maintainability-review"},
			"model":    {"type": "string", "description": "Optional model ID (provider/model); default: $SWORN_VERIFIER_MODEL or config.json verifier model"},
			"base":     {"type": "string", "description": "Optional base ref for git diff"}
		},
		"required": ["release", "slice_id", "type"]
	}`), func(ctx context.Context, params json.RawMessage) (*ToolResult, error) {
		var p struct {
			Release string `json:"release"`
			SliceID string `json:"slice_id"`
			Type    string `json:"type"`
			Model   string `json:"model"`
			Base    string `json:"base"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}

		ct := gate.CheckType(p.Type)
		if !gate.ValidCheckTypes[ct] {
			return nil, fmt.Errorf("unknown check type %q (valid: ac-satisfaction, spec-ambiguity, design-review, security-review, semantic-coverage, maintainability-review)", p.Type)
		}

		releaseDir := resolveMCPReleaseDir(repoRoot, p.Release)
		if releaseDir == "" {
			return nil, fmt.Errorf("release %q not found", p.Release)
		}

		sliceDir := filepath.Join(releaseDir, p.SliceID)
		if _, err := os.Stat(sliceDir); err != nil {
			return nil, fmt.Errorf("slice %q not found in release %q", p.SliceID, p.Release)
		}

		// Resolve model (param > $SWORN_VERIFIER_MODEL > config.json).
		//
		// This is the SAME llm-check gate the CLI runs, so it must resolve the model
		// the same way. It previously read env-only ($SWORN_MODEL) — a different env
		// var from every sibling — so an agent driving llm-check over MCP got
		// "no model configured" on a fully-configured setup, exactly as the CLI did.
		cfg, cfgErr := config.Load()
		if cfgErr != nil {
			return nil, fmt.Errorf("loading config: %w", cfgErr)
		}
		mid, err := config.ResolveVerifierModel(p.Model, cfg)
		if err != nil {
			return nil, err
		}

		verifier, err := model.FromEnv(mid)
		if err != nil {
			return nil, fmt.Errorf("model setup: %w", err)
		}

		// Resolve base ref for diff.
		ref := p.Base
		if ref == "" {
			var err error
			ref, err = gate.BaseRefForSlice(sliceDir, p.Release)
			if err != nil {
				ref = "HEAD"
			}
		}

		diffContent, err := runGitDiff(repoRoot, ref)
		if err != nil {
			return nil, fmt.Errorf("git diff: %w", err)
		}

		report, err := gate.RunLLMCheck(ctx, ct, sliceDir, diffContent, verifier)
		if err != nil {
			return nil, fmt.Errorf("llm_check: %w", err)
		}

		b, _ := json.Marshal(report)
		return &ToolResult{Content: []ContentItem{{Type: "text", Text: string(b)}}}, nil
	})
}

// resolveMCPReleaseDir resolves a release directory relative to repoRoot.
// Returns "" if the directory does not exist.
func resolveMCPReleaseDir(repoRoot, name string) string {
	dir := filepath.Join(repoRoot, "docs", "release", name)
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return ""
	}
	return dir
}

// runGitDiff runs git diff and returns the output, or empty string if the ref is HEAD.
func runGitDiff(repoRoot, ref string) (string, error) {
	if ref == "HEAD" {
		return "", nil
	}
	cmd := exec.Command("git", "-C", repoRoot, "diff", ref+"..HEAD", "--", ".")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git diff: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}
