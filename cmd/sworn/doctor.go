package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/swornagent/sworn/internal/adopt"
	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/board"
	"github.com/swornagent/sworn/internal/lint"
	"github.com/swornagent/sworn/internal/model"
	"github.com/swornagent/sworn/internal/project"
	"github.com/swornagent/sworn/internal/prompt"
	"github.com/swornagent/sworn/internal/style"
) // checkLevel classifies a doctor check result.
type checkLevel int

const (
	levelOK checkLevel = iota
	levelWarn
	levelError
)

func (l checkLevel) tag() string {
	switch l {
	case levelOK:
		return "[OK]"
	case levelWarn:
		return "[WARN]"
	case levelError:
		return "[ERROR]"
	default:
		return "[??]"
	}
}

// checkResult is the outcome of a single doctor check.
type checkResult struct {
	level  checkLevel
	name   string
	detail string
}

// requiredHeadings maps each embedded prompt file to the headings it must contain.
// Headings that depend on not-yet-landed slices (S19) are tracked separately
// so they emit [WARN] instead of [ERROR] when absent.
type headingSpec struct {
	required []string // headings that must be present (ERROR if missing)
	warnOnly []string // headings that emit WARN if missing (S19-dependent)
	// orderingPairs: pairs (a, b) where a must appear before b if both present.
	orderingPairs [][2]string
}

var promptHeadingSpecs = map[string]headingSpec{
	"planner.md": {
		required: []string{
			"### Phase 1", "### Phase 2", "### Phase 3",
			"### Phase 4", "### Phase 5", "### Phase 6",
			"## Re-planning a release in flight",
		},
	},
	"implementer.md": {
		warnOnly: []string{"## Dependency discipline", "## Deviation check"},
		orderingPairs: [][2]string{
			{"## Dependency discipline", "## Deviation check"},
		},
	},
	"verifier.md": {
		warnOnly: []string{
			"## Catalog conformance check",
			"independently query the package registry",
		},
	},
}

// batonRuleFiles is the list of all 12 embedded Baton rule files that must
// exist and be non-empty.
var batonRuleFiles = []string{
	"01-reachability-gate.md",
	"02-no-silent-deferrals.md",
	"03-capture-discipline.md",
	"04-commit-messages-as-capture.md", "05-session-discipline.md",
	"06-proof-bundle.md",
	"07-adversarial-verification.md",
	"08-requirements-fidelity.md",
	"09-design-fidelity.md",
	"10-customer-journey-validation.md",
	"11-process-global-mutation.md",
	"12-guard-fidelity.md",
}

// batonRulesIndexHeading is the heading the README.md must carry.
const batonRulesIndexHeading = "## The twelve rules"

// minPromptLength is the minimum byte length for an embedded prompt.
const minPromptLength = 500

// checkDepFreshness is the injectable function for sworn's own dep freshness.
// Tests can override this to simulate registry unreachability.
var checkDepFreshness = defaultCheckDepFreshness

// defaultCheckDepFreshness runs `go list -m -u ./...` in the given directory
// and returns lines for modules with newer major versions available.
// If the command fails (registry unreachable, etc.), returns (nil, error).
func defaultCheckDepFreshness(dir string) ([]string, error) {
	cmd := exec.Command("go", "list", "-m", "-u", "./...")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("go list -m -u failed: %w", err)
	}
	var upgrades []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Lines like: github.com/foo/bar v1.2.3 [v1.5.0]
		// The bracketed version is the available upgrade.
		if idx := strings.Index(line, "["); idx >= 0 {
			avail := strings.TrimSpace(line[idx:])
			avail = strings.Trim(avail, "[]")
			// Only report major upgrades.
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				installed := parts[1]
				if isMajorUpgrade(installed, avail) {
					upgradeLine := fmt.Sprintf("%s: %s installed, %s available — major version upgrade available. Review release notes before upgrading.",
						parts[0], installed, avail)
					upgrades = append(upgrades, upgradeLine)
				}
			}
		}
	}
	return upgrades, nil
}

// isMajorUpgrade reports whether avail is a newer major version than installed.
// Go modules use vMAJOR.MINOR.PATCH; major is the first number after 'v'.
func isMajorUpgrade(installed, avail string) bool {
	mi := parseMajor(installed)
	ma := parseMajor(avail)
	if mi < 0 || ma < 0 {
		return false
	}
	return ma > mi
}

func parseMajor(v string) int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, ".")
	if len(parts) == 0 {
		return -1
	}
	var n int
	_, err := fmt.Sscanf(parts[0], "%d", &n)
	if err != nil {
		return -1
	}
	return n
}

// cmdDoctor implements the "sworn doctor" subcommand.
func cmdDoctor(args []string) int {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	fix := fs.Bool("fix", false, "apply safe auto-repairs (removes docs/baton/, migrates legacy AGENTS.md)")
	syncBaton := fs.Bool("sync-baton", false, "transactionally repair the Codex and Claude Baton mirrors")
	_ = fs.Parse(args)

	repoRoot, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn doctor: cannot determine working directory: %v\n", err)
		return 1
	}

	// Verify we're in a git repo root.
	if !isGitRepo(repoRoot) {
		fmt.Fprintf(os.Stderr, "[ERROR] not a git repository: %s\n", repoRoot)
		return 1
	}

	var results []checkResult
	hasError := false

	// --- Group 1: Embedded prompt integrity ---
	fmt.Println(style.Heading("Group 1: Embedded prompt integrity"))
	results = append(results, checkEmbeddedPrompts()...)
	for _, r := range results {
		if r.level == levelError {
			hasError = true
		}
		printResult(r)
	}

	// --- Group 1b: Baton schema manifest ---
	fmt.Println()
	fmt.Println(style.Heading("Group 1b: Baton schema manifest"))
	g1b := checkSchemaManifest()
	for _, r := range g1b {
		if r.level == levelError {
			hasError = true
		}
		printResult(r)
	}

	// --- Group 1bb: Credentials ---
	fmt.Println()
	fmt.Println(style.Heading("Group 1bb: Provider credentials"))
	for _, r := range checkCredentials() {
		printResult(r)
	}

	// --- Group 1c: Project context ---
	fmt.Println()
	fmt.Println(style.Heading("Group 1c: Project context"))
	for _, r := range checkProjectContext(repoRoot) {
		printResult(r)
	}

	// --- Group 2: Repo artifact audit ---
	fmt.Println()
	fmt.Println(style.Heading("Group 2: Repo artifact audit"))
	g2 := checkRepoArtifacts(repoRoot)
	for _, r := range g2 {
		printResult(r)
	}
	g2drift := checkRenderDrift(repoRoot)
	for _, r := range g2drift {
		if r.level == levelError {
			hasError = true
		}
		printResult(r)
	}

	// --- Group 2b: Release status timestamp sanity ---
	fmt.Println()
	fmt.Println(style.Heading("Group 2b: Release status timestamp sanity"))
	g2b := checkStatusTimestamps(repoRoot)
	for _, r := range g2b {
		if r.level == levelError {
			hasError = true
		}
		printResult(r)
	}

	// --- Group 3: Complete local Baton mirrors (optional when no supported
	// home exists; mandatory for --sync-baton and explicit root overrides). ---
	installOpts, installApplicable, installOptsErr := doctorBatonInstallOpts(*syncBaton)
	if installApplicable || installOptsErr != nil {
		fmt.Println()
		fmt.Println(style.Heading("Group 3: Local Baton mirrors"))
		if installOptsErr != nil {
			printResult(checkResult{level: levelError, name: "baton/local-mirrors", detail: installOptsErr.Error()})
			if !*syncBaton {
				hasError = true
			}
		} else {
			drift, checkErr := baton.CheckBatonInstall(installOpts)
			if checkErr != nil {
				printResult(checkResult{level: levelError, name: "baton/local-mirrors", detail: checkErr.Error()})
				if !*syncBaton {
					hasError = true
				}
			} else if len(drift) != 0 {
				printResult(checkResult{level: levelError, name: "baton/local-mirrors", detail: "drift: " + strings.Join(drift, ", ")})
				if !*syncBaton {
					hasError = true
				}
			} else {
				printResult(checkResult{level: levelOK, name: "baton/local-mirrors", detail: "Codex and Claude mirrors match the embedded Baton authority"})
			}
		}
	}

	// --- Group 4: Dependency version freshness ---
	fmt.Println()
	fmt.Println(style.Heading("Group 4: Dependency version freshness"))
	g4 := checkDependencyFreshness(repoRoot)
	for _, r := range g4 {
		printResult(r)
	}

	// --- --fix: apply safe auto-repairs ---
	if *fix {
		fixed := applyFixes(repoRoot)
		if fixed > 0 {
			// Exit 2 when fixes were applied.
			if hasError {
				return 1
			}
			return 2
		}
	}

	// --- --sync-baton: one three-root transaction over the embedded authority. ---
	if *syncBaton {
		if installOptsErr != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] --sync-baton: %v\n", installOptsErr)
			return 1
		}
		result, err := baton.SyncBatonInstall(installOpts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] --sync-baton: %v\n", err)
			return 1
		}
		switch result.State {
		case baton.InstallRepaired:
			fmt.Printf("[OK]    repaired Baton mirrors: %s\n", strings.Join(result.Changed, ", "))
			if hasError {
				return 1
			}
			return 2
		case baton.InstallRecovered:
			fmt.Println("[OK]    restored all pre-run Baton homes from durable recovery authority; rerun --sync-baton to install")
			if hasError {
				return 1
			}
			return 2
		case baton.InstallAlreadyExact:
			fmt.Println("[OK]    Baton mirrors already match the embedded authority")
		default:
			fmt.Fprintf(os.Stderr, "[ERROR] --sync-baton: unknown result %q\n", result.State)
			return 1
		}
	}

	if hasError {
		return 1
	}
	return 0
}

func doctorBatonInstallOpts(force bool) (baton.InstallOpts, bool, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return baton.InstallOpts{}, force, fmt.Errorf("resolve home directory: %w", err)
	}
	agentsHome := os.Getenv("AGENTS_HOME")
	if agentsHome == "" {
		agentsHome = filepath.Join(home, ".agents")
	}
	codexHome := os.Getenv("CODEX_HOME")
	if codexHome == "" {
		codexHome = filepath.Join(home, ".codex")
	}
	claudeHome := os.Getenv("CLAUDE_HOME")
	if claudeHome == "" {
		claudeHome = filepath.Join(home, ".claude")
	}
	swornHome := os.Getenv("SWORN_HOME")
	if swornHome == "" {
		configDir, configErr := os.UserConfigDir()
		if configErr != nil {
			return baton.InstallOpts{}, force, fmt.Errorf("resolve Sworn config directory: %w", configErr)
		}
		swornHome = filepath.Join(configDir, "sworn")
	}
	recoveryRoot := filepath.Join(swornHome, "recovery", "baton-sync")
	explicit := os.Getenv("AGENTS_HOME") != "" || os.Getenv("CODEX_HOME") != "" || os.Getenv("CLAUDE_HOME") != "" || os.Getenv("SWORN_HOME") != ""
	// Preserve doctor's historical project-audit behavior until this binary has
	// installed its own sentinels. Explicit overrides and --sync-baton always
	// opt into the complete mirror boundary; ordinary doctor auto-discovers it
	// only from Sworn-owned sentinels or pending recovery authority, not merely
	// because unrelated Codex/Claude config homes happen to exist.
	applicable := force || explicit || anyPathExists(
		recoveryRoot,
		filepath.Join(agentsHome, filepath.FromSlash(".sworn-baton/VERSION")),
		filepath.Join(codexHome, filepath.FromSlash(".sworn-baton/VERSION")),
		filepath.Join(claudeHome, filepath.FromSlash(".sworn-baton/VERSION")),
	)
	if !applicable {
		return baton.InstallOpts{}, false, nil
	}
	archiveBytes := adopt.BatonInstallerArchive()
	trees, err := baton.GenerateInstallerManagedTrees(archiveBytes)
	if err != nil {
		return baton.InstallOpts{}, true, fmt.Errorf("validate embedded Baton installer input: %w", err)
	}
	version, err := adopt.BatonDocsFS().ReadFile("baton/VERSION")
	if err != nil {
		return baton.InstallOpts{}, true, fmt.Errorf("read embedded Baton VERSION: %w", err)
	}
	return baton.InstallOpts{
		Roots: baton.InstallRoots{
			AgentsHome: agentsHome, CodexHome: codexHome,
			ClaudeHome: claudeHome, RecoveryRoot: recoveryRoot,
		},
		Trees: trees, Version: version,
	}, true, nil
}

func anyPathExists(paths ...string) bool {
	for _, path := range paths {
		if _, err := os.Lstat(path); err == nil {
			return true
		}
	}
	return false
}

// printResult prints a single check result line.
func printResult(r checkResult) {
	fmt.Printf("%-7s %s\n", r.level.tag(), r.name)
	if r.detail != "" {
		fmt.Printf("        %s\n", r.detail)
	}
}

// checkEmbeddedPrompts verifies the integrity of all embedded prompts and
// Baton protocol docs.
func checkEmbeddedPrompts() []checkResult {
	var results []checkResult

	// Check each prompt file: length + headings.
	promptFiles := []string{"planner.md", "implementer.md", "verifier.md", "captain.md", "verify-stateless.md"}
	promptReaders := map[string]func() string{
		"planner.md":          prompt.Planner,
		"implementer.md":      prompt.Implementer,
		"verifier.md":         prompt.Verifier,
		"captain.md":          prompt.Captain,
		"verify-stateless.md": prompt.VerifyStateless,
	}

	for _, name := range promptFiles {
		content := promptReaders[name]()
		var res checkResult
		res.name = name

		// Length check.
		if len(content) < minPromptLength {
			res.level = levelError
			res.detail = fmt.Sprintf("length=%d BELOW MINIMUM (expected >%d) — embed may be corrupted", len(content), minPromptLength)
			results = append(results, res)
			continue // skip heading checks if length is wrong
		}

		// Heading checks.
		spec, hasSpec := promptHeadingSpecs[name]
		missingRequired := []string{}
		missingWarn := []string{}

		if hasSpec {
			for _, h := range spec.required {
				if !strings.Contains(content, h) {
					missingRequired = append(missingRequired, h)
				}
			}
			for _, h := range spec.warnOnly {
				if !strings.Contains(content, h) {
					missingWarn = append(missingWarn, h)
				}
			}
			// Ordering check: if both members of a pair are present, verify order.
			orderingErrors := []string{}
			for _, pair := range spec.orderingPairs {
				aIdx := strings.Index(content, pair[0])
				bIdx := strings.Index(content, pair[1])
				if aIdx >= 0 && bIdx >= 0 && aIdx > bIdx {
					orderingErrors = append(orderingErrors, fmt.Sprintf("%q must appear before %q", pair[0], pair[1]))
				}
			}
			if len(missingRequired) > 0 {
				res.level = levelError
				res.detail = fmt.Sprintf("missing required headings: %s", strings.Join(missingRequired, ", "))
			} else if len(missingWarn) > 0 || len(orderingErrors) > 0 {
				parts := []string{}
				if len(missingWarn) > 0 {
					parts = append(parts, fmt.Sprintf("missing headings (WARN — not yet landed): %s", strings.Join(missingWarn, ", ")))
				}
				parts = append(parts, orderingErrors...)
				res.level = levelWarn
				res.detail = strings.Join(parts, "; ")
			} else {
				res.level = levelOK
				res.detail = fmt.Sprintf("length=%d   headings=all present", len(content))
			}
		} else {
			// No heading spec for this file — just length OK.
			res.level = levelOK
			res.detail = fmt.Sprintf("length=%d", len(content))
		}

		results = append(results, res)
	}

	// Check Baton rules: 10 rule files + README heading.
	rulesOK := true
	missingRules := []string{}
	for _, rf := range batonRuleFiles {
		data, err := adopt.BatonDocsFS().ReadFile("baton/rules/" + rf)
		if err != nil || len(data) == 0 {
			missingRules = append(missingRules, rf)
			rulesOK = false
		}
	}
	readmeData, readmeErr := adopt.BatonDocsFS().ReadFile("baton/README.md")
	readmeOK := readmeErr == nil && strings.Contains(string(readmeData), batonRulesIndexHeading)

	if rulesOK && readmeOK {
		results = append(results, checkResult{
			level:  levelOK,
			name:   "baton/rules/",
			detail: fmt.Sprintf("%d/%d rule files present, README heading OK", len(batonRuleFiles), len(batonRuleFiles)),
		})
	} else {
		detailParts := []string{}
		if len(missingRules) > 0 {
			detailParts = append(detailParts, fmt.Sprintf("missing rule files: %s", strings.Join(missingRules, ", ")))
		}
		if !readmeOK {
			detailParts = append(detailParts, "README.md missing rules-index heading")
		}
		results = append(results, checkResult{
			level:  levelError,
			name:   "baton/rules/",
			detail: strings.Join(detailParts, "; "),
		})
	}

	// Check track-mode.md.
	tm := prompt.TrackMode()
	if strings.Contains(tm, "## The safety invariants") {
		results = append(results, checkResult{
			level:  levelOK,
			name:   "baton/track-mode.md",
			detail: fmt.Sprintf("length=%d   heading present", len(tm)),
		})
	} else {
		results = append(results, checkResult{
			level:  levelError,
			name:   "baton/track-mode.md",
			detail: "missing heading '## The safety invariants'",
		})
	}

	// Check VERSION.txt — the embedded prompt version (tightened: ERROR on non-semver).
	v := baton.Version()
	if v == "" {
		results = append(results, checkResult{
			level:  levelError,
			name:   "baton/VERSION.txt",
			detail: "version file empty or unparseable",
		})
	} else if !baton.IsSemverTag(v) {
		results = append(results, checkResult{
			level:  levelError,
			name:   "baton/VERSION.txt",
			detail: fmt.Sprintf("version=%s — not a valid semver tag (must be vMAJOR.MINOR.PATCH)", v),
		})
	} else {
		results = append(results, checkResult{
			level:  levelOK,
			name:   "baton/VERSION.txt",
			detail: fmt.Sprintf("version=%s", v),
		})
	}

	// Check baton-protocol pin is a semver tag, not a SHA (fail-closed).
	pin := baton.Version()
	if !baton.IsSemverTag(pin) {
		results = append(results, checkResult{
			level:  levelError,
			name:   "baton/VERSION (baton-protocol)",
			detail: fmt.Sprintf("pin=%s — [ERROR] baton-protocol must be a semver tag (vMAJOR.MINOR.PATCH), not a SHA", pin),
		})
	} else {
		results = append(results, checkResult{
			level:  levelOK,
			name:   "baton/VERSION (baton-protocol)",
			detail: fmt.Sprintf("on Baton %s", pin),
		})
	}

	// --- Pin-currency check: SHA-vs-HEAD drift detection.
	results = append(results, checkPinCurrency())

	// --- Prompt-currency check: pre-records-as-JSON marker detection.
	results = append(results, checkPromptCurrency())

	return results
}

// checkSchemaManifest renders the graded-schema-version manifest: one
// checkResult per vendored Baton schema (name/$id/version/GRADED-or-ADVISORY,
// baton.SchemaManifest), plus a final skew check that WARNs when the
// declared graded/advisory classification table disagrees with the live
// vendored schema set. Never a silent OK on skew (S15 AC-01/AC-02) — the
// scar this manages is baton#54/#55/#58 (vendored schemas at one version
// while the binary graded stale shapes, silently).
func checkSchemaManifest() []checkResult {
	var results []checkResult

	entries, err := baton.SchemaManifest()
	if err != nil {
		return []checkResult{{
			level:  levelError,
			name:   "baton/schema-manifest",
			detail: fmt.Sprintf("failed to build schema manifest: %v", err),
		}}
	}

	for _, e := range entries {
		if e.Status == "" {
			results = append(results, checkResult{
				level:  levelWarn,
				name:   fmt.Sprintf("baton/schema-manifest/%s", e.Name),
				detail: fmt.Sprintf("$id=%s version=%s status=UNCLASSIFIED — no graded/advisory entry in schemaGradeStatus", e.ID, e.Version),
			})
			continue
		}
		results = append(results, checkResult{
			level:  levelOK,
			name:   fmt.Sprintf("baton/schema-manifest/%s", e.Name),
			detail: fmt.Sprintf("$id=%s version=%s status=%s", e.ID, e.Version, e.Status),
		})
	}

	// Skew is WARN, never ERROR — it does not gate cmdDoctor's exit code
	// (S15 design decision D2). A structural failure above (malformed
	// embedded schema JSON, which //go:embed makes a compile-time
	// impossibility in practice) is the only path that returns levelError.
	if skew := baton.SchemaSkew(); len(skew) == 0 {
		results = append(results, checkResult{
			level:  levelOK,
			name:   "baton/schema-skew",
			detail: "declared graded/advisory set matches the vendored schema set",
		})
	} else {
		results = append(results, checkResult{
			level:  levelWarn,
			name:   "baton/schema-skew",
			detail: strings.Join(skew, "; "),
		})
	}

	return results
}

// checkPinCurrency verifies the vendored pin is from a post-baton/ layout
// commit. Pre-layout commits (before the baton/ directory restructure) lack
// the baton/rules/ directory in the embed. The check tries to read
// baton/rules/01-reachability-gate.md from the adopt embed; if absent, the
// pin predates the baton/ layout and is stale.

// readBatonDoc is the injectable function for reading Baton doc files.
// Tests override this to simulate pre-layout pins.
var readBatonDoc = func(path string) ([]byte, error) {
	return adopt.BatonDocsFS().ReadFile(path)
}

func checkPinCurrency() checkResult {
	// Try to read a known post-layout file from the embed.
	_, err := readBatonDoc("baton/rules/01-reachability-gate.md")
	if err != nil {
		// Pre-baton/ layout — pin is stale.
		pin, _ := baton.ReadUpstreamPin()
		return checkResult{
			level:  levelError,
			name:   "baton/pin-currency",
			detail: fmt.Sprintf("PIN-STALE: upstream-sha %s predates baton/ layout — re-vendor required", pin.SHA),
		}
	}
	return checkResult{
		level:  levelOK,
		name:   "baton/pin-currency",
		detail: "vendored pin is from a post-baton/ layout commit",
	}
}

// checkPromptCurrency scans embedded prompts for pre-records-as-JSON
// markers that indicate stale vendored prompts. The markers are:
//   - the pre-consolidation version string (pre-consolidation version string)
//   - "proof.md-primary" (old proof bundle naming)
//   - "PROOF-optional" (old proof bundle marker)
//   - "scripts/release-verify.sh" (old first-pass script path)

// promptReadersForCheck is the injectable map of prompt file readers.
// Tests override this to inject mock prompts containing stale markers.
var promptReadersForCheck = map[string]func() string{
	"verifier.md":         prompt.Verifier,
	"implementer.md":      prompt.Implementer,
	"planner.md":          prompt.Planner,
	"captain.md":          prompt.Captain,
	"verify-stateless.md": prompt.VerifyStateless,
}

func checkPromptCurrency() checkResult {
	markers := []string{"v0.4" + ".2", "proof.md-primary", "PROOF-optional", "scripts/release-verify.sh"}
	var findings []string
	for name, reader := range promptReadersForCheck {
		content := reader()
		for _, marker := range markers {
			if strings.Contains(content, marker) {
				findings = append(findings, fmt.Sprintf("%s contains %q", name, marker))
			}
		}
	}
	if len(findings) > 0 {
		return checkResult{
			level:  levelError,
			name:   "baton/prompt-currency",
			detail: "PROMPT-STALE: " + strings.Join(findings, "; ") + " — re-vendor prompts",
		}
	}
	return checkResult{
		level:  levelOK,
		name:   "baton/prompt-currency",
		detail: "no pre-JSON markers found in embedded prompts",
	}
}

// checkRepoArtifacts checks for legacy Baton artifacts in the repo.
func checkRepoArtifacts(repoRoot string) []checkResult {
	var results []checkResult

	// docs/baton/ existence.
	batonDir := filepath.Join(repoRoot, "docs", "baton")
	if _, err := os.Stat(batonDir); err == nil {
		results = append(results, checkResult{
			level:  levelWarn,
			name:   "docs/baton/",
			detail: "legacy per-repo Baton copy. The binary is now the canonical source. Safe to remove: rm -rf docs/baton/",
		})
	} else {
		results = append(results, checkResult{
			level:  levelOK,
			name:   "docs/baton/",
			detail: "not present (canonical source is the binary embed)",
		})
	}

	// AGENTS.md checks.
	agentsPath := filepath.Join(repoRoot, "AGENTS.md")
	agentsContent, err := os.ReadFile(agentsPath)
	if err != nil {
		results = append(results, checkResult{
			level:  levelWarn,
			name:   "AGENTS.md",
			detail: "not found. Run 'sworn init' to create it.",
		})
		return results
	}

	content := string(agentsContent)

	// Legacy splice detection.
	if strings.Contains(content, adopt.BatonSectionHeading) {
		results = append(results, checkResult{
			level:  levelWarn,
			name:   "AGENTS.md",
			detail: "contains legacy Baton splice content. Run 'sworn doctor --fix' to migrate: replaces only the Baton section with an MCP pointer, preserves the rest of the file, backs up the original to AGENTS.md.bak",
		})
	} else {
		results = append(results, checkResult{
			level:  levelOK,
			name:   "AGENTS.md",
			detail: "no legacy Baton splice detected",
		})
	}

	return results
}

// checkRenderDrift scans docs/release/ for board.json-backed releases and
// verifies each one's committed index.md matches board.Render's in-memory
// output byte for byte (ADR-0009: index.md is a rendered VIEW of board.json,
// never a hand-edited source of truth — a mismatch means the committed file
// has drifted from the record it's supposed to represent). Releases with no
// board.json are skipped (AC-03) — there is no JSON source to render from.
// A release whose board.json exists but cannot be rendered (malformed,
// legacy string-form release, a referenced slice missing its spec/status)
// also reports ERROR: a release that can't render can't be proven
// non-drifted, so this fails closed rather than being silently skipped
// (mirrors board.Render's own fail-closed contract). This replaces the
// former internal/board.driftGuard, which was advisory-only and re-parsed
// raw index.md frontmatter instead of comparing against a real render.
func checkRenderDrift(repoRoot string) []checkResult {
	releaseRoot := filepath.Join(repoRoot, "docs", "release")
	entries, err := os.ReadDir(releaseRoot)
	if err != nil {
		return []checkResult{{
			level:  levelOK,
			name:   "render drift",
			detail: "no docs/release/ directory — nothing to check",
		}}
	}

	var drifted []checkResult
	checked := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		release := entry.Name()
		releaseDir := filepath.Join(releaseRoot, release)
		if _, err := os.Stat(filepath.Join(releaseDir, "board.json")); err != nil {
			continue // AC-03: no board.json, nothing to render from
		}
		checked++

		rendered, err := board.Render(repoRoot, release)
		if err != nil {
			drifted = append(drifted, checkResult{
				level:  levelError,
				name:   fmt.Sprintf("render drift (%s)", release),
				detail: fmt.Sprintf("cannot render: %v", err),
			})
			continue
		}

		committed, err := os.ReadFile(filepath.Join(releaseDir, "index.md"))
		if err != nil {
			drifted = append(drifted, checkResult{
				level:  levelError,
				name:   fmt.Sprintf("render drift (%s)", release),
				detail: fmt.Sprintf("cannot read committed index.md: %v", err),
			})
			continue
		}

		if rendered != string(committed) {
			drifted = append(drifted, checkResult{
				level:  levelError,
				name:   fmt.Sprintf("render drift (%s)", release),
				detail: fmt.Sprintf("committed index.md does not match render(board.json) — re-render via 'sworn render %s'", release),
			})
		}
	}

	if len(drifted) == 0 {
		return []checkResult{{
			level:  levelOK,
			name:   "render drift",
			detail: fmt.Sprintf("%d board.json-backed release(s) match their rendered index.md", checked),
		}}
	}

	summary := checkResult{
		level:  levelError,
		name:   "render drift",
		detail: fmt.Sprintf("%d of %d board.json-backed release(s) drifted or failed to render", len(drifted), checked),
	}
	return append([]checkResult{summary}, drifted...)
}

// checkStatusTimestamps scans docs/release/ for status.json files and validates
// that last_updated_at and verification.verifier_verdict_at timestamps are not
// in the future beyond a 5-minute clock-skew allowance.
func checkStatusTimestamps(repoRoot string) []checkResult {
	releaseRoot := filepath.Join(repoRoot, "docs", "release")
	if _, err := os.Stat(releaseRoot); err != nil {
		return []checkResult{{
			level:  levelOK,
			name:   "status timestamps",
			detail: "no docs/release/ directory — nothing to check",
		}}
	}

	var allResults []checkResult
	entries, _ := os.ReadDir(releaseRoot)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		releaseDir := filepath.Join(releaseRoot, entry.Name())
		violations := lint.CheckStatusTimestamps(releaseDir, lint.DefaultClock)
		if len(violations) > 0 {
			for _, v := range violations {
				allResults = append(allResults, checkResult{
					level:  levelError,
					name:   fmt.Sprintf("status timestamp (%s/%s)", v.Release, v.SliceID),
					detail: fmt.Sprintf("%s: %q exceeds allowed maximum %s", v.Field, v.Value, v.AllowedAt),
				})
			}
		}
	}

	if len(allResults) == 0 {
		return []checkResult{{
			level:  levelOK,
			name:   "status timestamps",
			detail: fmt.Sprintf("all timestamps within allowed window across %d release(s)", len(entries)),
		}}
	}

	// Prepend a summary result.
	summary := checkResult{
		level:  levelError,
		name:   "status timestamps",
		detail: fmt.Sprintf("%d violation(s) across scanned releases", len(allResults)),
	}
	return append([]checkResult{summary}, allResults...)
}

// checkBatonSync compares the embedded Baton docs against the local ~/.claude/baton/.
func checkBatonSync(batonHome string) []checkResult {
	var results []checkResult

	// Compare rules files.
	mismatches := 0
	for _, rf := range batonRuleFiles {
		embedded, _ := adopt.BatonDocsFS().ReadFile("baton/rules/" + rf)
		localPath := filepath.Join(batonHome, "rules", rf)
		local, locErr := os.ReadFile(localPath)
		if locErr != nil || string(embedded) != string(local) {
			mismatches++
		}
	}

	// Compare README.
	embREADME, _ := adopt.BatonDocsFS().ReadFile("baton/README.md")
	localREADME, locErr := os.ReadFile(filepath.Join(batonHome, "README.md"))
	if locErr != nil || string(embREADME) != string(localREADME) {
		mismatches++
	}

	if mismatches > 0 {
		results = append(results, checkResult{
			level:  levelWarn,
			name:   "~/.claude/baton/",
			detail: fmt.Sprintf("differs from the binary's embedded Baton (%d files differ). Slash commands use the local copy; run 'sworn doctor --sync-baton' to update. (Only affects interactive slash commands, not autonomous sworn run.)", mismatches),
		})
	} else {
		results = append(results, checkResult{
			level:  levelOK,
			name:   "~/.claude/baton/",
			detail: "in sync with embedded Baton",
		})
	}

	return results
}

// checkDependencyFreshness checks project pins vs catalog and sworn's own deps.
func checkDependencyFreshness(repoRoot string) []checkResult {
	var results []checkResult

	// Check if docs/considerations.md exists.
	considerationsPath := filepath.Join(repoRoot, "docs", "considerations.md")
	consContent, consErr := os.ReadFile(considerationsPath)

	hasGoMod := fileExists(filepath.Join(repoRoot, "go.mod"))
	hasPackageJSON := fileExists(filepath.Join(repoRoot, "package.json"))
	hasRequirements := fileExists(filepath.Join(repoRoot, "requirements.txt"))

	hasDepFile := hasGoMod || hasPackageJSON || hasRequirements

	if consErr != nil {
		// No considerations.md — skip group 4 entirely.
		if !hasDepFile {
			results = append(results, checkResult{
				level:  levelOK,
				name:   "dependency freshness",
				detail: "no dependency file or catalog found — nothing to check",
			})
			return results
		}
		// Has dep file but no catalog — warn about empty pins.
		results = append(results, checkResult{
			level:  levelWarn,
			name:   "dependency catalog",
			detail: fmt.Sprintf("dependency file found but %s not found. Run 'sworn induction' to populate it — implementers need this to know which versions are already pinned.", "docs/considerations.md"),
		})
		return results
	}

	// Parse [dependencies] section from considerations.md.
	consStr := string(consContent)
	projectPinned := parseProjectPinned(consStr)

	// Check: empty catalog pins with a dependency file present.
	if hasDepFile && len(projectPinned) == 0 {
		depFileName := "go.mod"
		if hasPackageJSON {
			depFileName = "package.json"
		} else if hasRequirements {
			depFileName = "requirements.txt"
		}
		results = append(results, checkResult{
			level:  levelWarn,
			name:   "dependency catalog",
			detail: fmt.Sprintf("%s found but [dependencies].project_pinned is empty in docs/considerations.md. Run 'sworn induction' to populate it — implementers need this to know which versions are already pinned.", depFileName),
		})
	}

	// Check: project pins vs catalog (go.mod only for now).
	if hasGoMod && len(projectPinned) > 0 {
		stalePins := checkStalePins(repoRoot, projectPinned)
		for _, sp := range stalePins {
			results = append(results, checkResult{
				level:  levelWarn,
				name:   "project_pinned",
				detail: sp,
			})
		}
	}

	// Check: sworn's own deps (Go only).
	if hasGoMod {
		upgrades, err := checkDepFreshness(repoRoot)
		if err != nil {
			results = append(results, checkResult{
				level:  levelWarn,
				name:   "sworn deps",
				detail: "Registry unreachable — sworn dep freshness check skipped.",
			})
		} else {
			if len(upgrades) == 0 {
				results = append(results, checkResult{
					level:  levelOK,
					name:   "sworn deps",
					detail: "no major version upgrades available",
				})
			} else {
				for _, u := range upgrades {
					results = append(results, checkResult{
						level:  levelWarn,
						name:   "sworn deps",
						detail: u,
					})
				}
			}
		}
	} else {
		results = append(results, checkResult{
			level:  levelOK,
			name:   "sworn deps",
			detail: "no go.mod — skipped Go dep freshness check",
		})
	}

	return results
}

// parseProjectPinned extracts the [dependencies].project_pinned entries from
// considerations.md content. Returns a map of module → version.
func parseProjectPinned(content string) map[string]string {
	pinned := map[string]string{}

	// Find [dependencies] section.
	lines := strings.Split(content, "\n")
	inDeps := false
	inPinned := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[dependencies]") {
			inDeps = true
			continue
		}
		if inDeps && strings.HasPrefix(trimmed, "[") && trimmed != "[dependencies]" {
			// New section — stop.
			inDeps = false
			inPinned = false
			continue
		}
		if inDeps && strings.HasPrefix(trimmed, "project_pinned") {
			// Could be "project_pinned = ..." on one line or a multi-line list.
			inPinned = true
			// Try to parse inline: project_pinned = { module = "v1.0", ... }
			rest := strings.TrimPrefix(trimmed, "project_pinned")
			rest = strings.TrimSpace(rest)
			if strings.HasPrefix(rest, "=") {
				rest = strings.TrimSpace(rest[1:])
			}
			parsePinnedEntries(rest, pinned)
			continue
		}
		if inPinned {
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}
			parsePinnedEntries(trimmed, pinned)
		}
	}
	return pinned
}

// parsePinnedEntries parses entries like `module = "version"` or
// `module@version` from a line and adds them to the map.
func parsePinnedEntries(line string, pinned map[string]string) {
	line = strings.TrimSpace(line)
	// Remove surrounding braces/quotes.
	line = strings.Trim(line, "{}")
	// Split by comma or newline.
	for _, entry := range strings.Split(line, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		// module = "version" or module@version
		if idx := strings.Index(entry, "="); idx >= 0 {
			mod := strings.TrimSpace(entry[:idx])
			ver := strings.TrimSpace(entry[idx+1:])
			ver = strings.Trim(ver, "\"'")
			if mod != "" && ver != "" {
				pinned[mod] = ver
			}
		} else if idx := strings.IndexByte(entry, '@'); idx >= 0 {
			mod := strings.TrimSpace(entry[:idx])
			ver := strings.TrimSpace(entry[idx+1:])
			if mod != "" && ver != "" {
				pinned[mod] = ver
			}
		}
	}
}

// checkStalePins compares catalog pins against actual go.mod entries.
func checkStalePins(repoRoot string, pinned map[string]string) []string {
	var stale []string
	goModPath := filepath.Join(repoRoot, "go.mod")
	goModContent, err := os.ReadFile(goModPath)
	if err != nil {
		return stale
	}
	for _, line := range strings.Split(string(goModContent), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "require") || strings.HasPrefix(line, "//") || line == "" {
			continue
		}
		// Parse: module-path version
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			mod := parts[0]
			ver := parts[1]
			if catalogVer, ok := pinned[mod]; ok {
				if catalogVer != ver {
					stale = append(stale, fmt.Sprintf("docs/considerations.md [dependencies].project_pinned is stale for %s: catalog says %s but go.mod has %s. Run 'sworn induction --update' to sync.", mod, catalogVer, ver))
				}
			}
		}
	}
	return stale
}

// applyFixes applies safe auto-repairs and returns the number of fixes applied.
func applyFixes(repoRoot string) int {
	fixed := 0

	// Remove docs/baton/ if present.
	batonDir := filepath.Join(repoRoot, "docs", "baton")
	if _, err := os.Stat(batonDir); err == nil {
		fmt.Println("== --fix: removing legacy docs/baton/ ==")
		filepath.Walk(batonDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if !info.IsDir() {
				rel, _ := filepath.Rel(batonDir, path)
				fmt.Printf("  rm: %s\n", rel)
			}
			return nil
		})
		if err := os.RemoveAll(batonDir); err != nil {
			fmt.Fprintf(os.Stderr, "  [ERROR] failed to remove: %v\n", err)
		} else {
			fmt.Println("  removed docs/baton/")
			fixed++
		}
	}

	// Migrate legacy AGENTS.md if splice detected.
	agentsPath := filepath.Join(repoRoot, "AGENTS.md")
	content, err := os.ReadFile(agentsPath)
	if err == nil && strings.Contains(string(content), adopt.BatonSectionHeading) {
		fmt.Println("== --fix: migrating legacy AGENTS.md ==")
		// Back up old content. Never clobber an existing backup — it may hold
		// the only copy of a previous migration's original; fall back to a
		// timestamped name instead.
		bakPath := filepath.Join(repoRoot, "AGENTS.md.bak")
		if _, statErr := os.Stat(bakPath); statErr == nil {
			bakPath = filepath.Join(repoRoot, "AGENTS.md.bak."+time.Now().UTC().Format("20060102T150405Z"))
		}
		if err := os.WriteFile(bakPath, content, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "  [ERROR] failed to write backup: %v\n", err)
		} else {
			fmt.Printf("  backed up old AGENTS.md to %s\n", filepath.Base(bakPath))
		}
		// Splice: replace only the legacy Baton section(s), preserving all
		// other user content.
		if err := os.WriteFile(agentsPath, []byte(migrateLegacyAgents(string(content))), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "  [ERROR] failed to write new AGENTS.md: %v\n", err)
		} else {
			fmt.Println("  replaced legacy Baton section with MCP pointer (rest of file preserved)")
			fixed++
		}
	}

	return fixed
}

// agentsMCPPointerSection is what replaces a legacy spliced Baton section
// during doctor --fix. It intentionally contains neither
// adopt.BatonSectionHeading (so a second doctor run detects no legacy splice
// and the migration converges) nor a docs/baton/ pointer (the same --fix run
// removes that directory).
const agentsMCPPointerSection = `## Engineering Process

This project follows the [Baton](https://swornagent.com) protocol via sworn.
The canonical rules and role prompts are served by the sworn MCP server
(run ` + "`sworn mcp`" + `; full protocol at resource ` + "`sworn://baton/rules`" + `) —
always fetch the current protocol from the binary, not from per-repo copies.
`

// migrateLegacyAgents replaces every legacy spliced Baton section in content
// (from adopt.BatonSectionHeading to the next same-level "## " heading or EOF)
// with agentsMCPPointerSection, preserving all surrounding user content. The
// loop guarantees the result no longer contains the legacy trigger heading.
func migrateLegacyAgents(content string) string {
	for {
		headingIdx := strings.Index(content, adopt.BatonSectionHeading)
		if headingIdx < 0 {
			return content
		}
		// Skip past the heading line itself before searching for the next
		// section, mirroring adopt's splice bounds logic.
		bodyStart := headingIdx + len(adopt.BatonSectionHeading)
		if nl := strings.IndexByte(content[bodyStart:], '\n'); nl >= 0 {
			bodyStart += nl + 1
		}
		sectionEnd := len(content)
		if next := strings.Index(content[bodyStart:], "\n## "); next >= 0 {
			sectionEnd = bodyStart + next + 1
		}
		rest := content[sectionEnd:]
		if rest != "" {
			rest = "\n" + rest
		}
		content = content[:headingIdx] + agentsMCPPointerSection + rest
	}
}

// syncBatonHome copies all embedded Baton docs to the given directory.
func syncBatonHome(dir string) error {
	if err := os.MkdirAll(filepath.Join(dir, "rules"), 0755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	// Write README.md.
	readme, err := adopt.BatonDocsFS().ReadFile("baton/README.md")
	if err != nil {
		return fmt.Errorf("read README.md: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), readme, 0644); err != nil {
		return fmt.Errorf("write README.md: %w", err)
	}
	fmt.Printf("  wrote %s/README.md\n", dir)

	// Write VERSION.
	versionData, err := adopt.BatonDocsFS().ReadFile("baton/VERSION")
	if err != nil {
		return fmt.Errorf("read VERSION: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "VERSION"), versionData, 0644); err != nil {
		return fmt.Errorf("write VERSION: %w", err)
	}
	fmt.Printf("  wrote %s/VERSION\n", dir)

	// Write all rule files.
	for _, rf := range batonRuleFiles {
		data, err := adopt.BatonDocsFS().ReadFile("baton/rules/" + rf)
		if err != nil {
			return fmt.Errorf("read rules/%s: %w", rf, err)
		}
		if err := os.WriteFile(filepath.Join(dir, "rules", rf), data, 0644); err != nil {
			return fmt.Errorf("write rules/%s: %w", rf, err)
		}
		fmt.Printf("  wrote %s/rules/%s\n", dir, rf)
	}

	return nil
}

// isGitRepo reports whether the given directory is inside a git repository.
func isGitRepo(dir string) bool {
	gitDir := filepath.Join(dir, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		return true
	}
	// Also accept being inside a worktree (gitdir file).
	if data, err := os.ReadFile(gitDir); err == nil {
		_ = data
		return true
	}
	// Walk up to find .git.
	d := dir
	for {
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
			return true
		}
		parent := filepath.Dir(d)
		if parent == d {
			return false
		}
		d = parent
	}
}

// fileExists reports whether the file exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// checkProjectContext reports whether the project's Baton project-context-v1
// record (.sworn/project.json) is DECLARED and ratified, merely DRAFTED, or
// entirely absent (so the context is INFERRED from the file layout).
//
// This is the visibility mechanism the whole declared-context design turns on: a
// detection guess must never be able to masquerade as a declaration. The engine
// already fails closed on stakes when the record is absent or unratified — this
// makes that fact visible instead of silent, and names the remedy.
func checkProjectContext(repoRoot string) []checkResult {
	// A malformed record is worse than a missing one: it looks declared and reads
	// as nothing. Surface it as an error, with what the schema objected to.
	if _, err := project.Load(repoRoot); err != nil && !errors.Is(err, project.ErrNoRecord) {
		return []checkResult{{
			level: levelError,
			name:  "project/context",
			detail: fmt.Sprintf("%s is present but INVALID — checks run at fail-closed HIGH stakes: %v",
				project.RecordPath, err),
		}}
	}

	r := project.Resolve(repoRoot)

	switch r.Source {
	case project.SourceDeclared:
		stakes := "low stakes"
		if r.HighStakes {
			stakes = "HIGH stakes"
		}
		return []checkResult{{
			level:  levelOK,
			name:   "project/context",
			detail: fmt.Sprintf("declared + ratified — %q (%s)", r.Context, stakes),
		}}

	case project.SourceDrafted:
		return []checkResult{{
			level: levelWarn,
			name:  "project/context",
			detail: fmt.Sprintf("DRAFTED but NOT RATIFIED — %q. A model proposed this; no human has confirmed it, "+
				"so every check runs at fail-closed HIGH stakes regardless of the stakes it claims. "+
				"Review %s and set ratification.ratified = true.", r.Context, project.RecordPath),
		}}

	default: // inferred
		return []checkResult{{
			level: levelWarn,
			name:  "project/context",
			detail: fmt.Sprintf("UNDECLARED — no %s. The context %q was INFERRED from your file layout, "+
				"which can read your languages but cannot know whether real customers depend on this. "+
				"Every check runs at fail-closed HIGH stakes. Run 'sworn init' to draft and ratify it.",
				project.RecordPath, r.Context),
		}}
	}
}

// checkCredentials reports where sworn is finding provider API keys — and warns
// when keys are sitting in a place sworn no longer looks.
//
// Keys used to live in ~/.sworn/.env (a dotenv file, outside XDG) and in
// SWORN_-prefixed env vars. Worse, that file was loaded into the environment by ONE
// command (`sworn run`), so a key written by `sworn init` was visible to the loop
// and invisible to llm-check, verify, reqverify and MCP — each resolved a model
// correctly and then failed for want of a key that was on disk the whole time.
//
// Keys now live in credentials.json (XDG) or the canonical env vars, and the model
// layer resolves them itself. This check makes the remaining legacy keys visible
// instead of silently ignored.
func checkCredentials() []checkResult {
	var results []checkResult

	configured := model.ConfiguredProviders()
	if len(configured) == 0 {
		results = append(results, checkResult{
			level: levelWarn,
			name:  "credentials",
			detail: fmt.Sprintf("no provider API keys found — set a canonical env var (OPENAI_API_KEY, "+
				"ANTHROPIC_API_KEY, …) or add one to %s (run 'sworn init')", model.CredentialsPath()),
		})
	} else {
		sort.Strings(configured)
		results = append(results, checkResult{
			level:  levelOK,
			name:   "credentials",
			detail: fmt.Sprintf("keys found for: %s (%s)", strings.Join(configured, ", "), model.CredentialsPath()),
		})
	}

	// Legacy keys that sworn NO LONGER READS.
	if legacy := model.FindLegacyCredentials(); len(legacy) > 0 {
		var stranded []string
		for provider := range legacy {
			if model.ProviderKey(provider) == "" {
				stranded = append(stranded, provider)
			}
		}
		if len(stranded) > 0 {
			sort.Strings(stranded)
			results = append(results, checkResult{
				level: levelWarn,
				name:  "credentials/legacy",
				detail: fmt.Sprintf("keys for %s are in a legacy location sworn NO LONGER READS "+
					"(SWORN_-prefixed env vars, or %s). Run 'sworn init' to migrate them to %s.",
					strings.Join(stranded, ", "), model.LegacyEnvPath(), model.CredentialsPath()),
			})
		}
	}

	return results
}
