package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/swornagent/sworn/internal/adopt"
	"github.com/swornagent/sworn/internal/prompt"
	"github.com/swornagent/sworn/internal/style"
)

// checkLevel classifies a doctor check result.
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

// batonRuleFiles is the list of all 11 embedded Baton rule files that must
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
}
// batonRulesIndexHeading is the heading the README.md must carry.
const batonRulesIndexHeading = "## The seven rules"

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
	syncBaton := fs.Bool("sync-baton", false, "copy embedded Baton docs to ~/.claude/baton/ (or SWORN_BATON_HOME)")
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

	// --- Group 2: Repo artifact audit ---
	fmt.Println()
	fmt.Println(style.Heading("Group 2: Repo artifact audit"))
	g2 := checkRepoArtifacts(repoRoot)
	for _, r := range g2 {
		printResult(r)
	}

	// --- Group 3: Local Baton sync (optional) ---
	batonHome := os.Getenv("SWORN_BATON_HOME")
	if batonHome == "" {
		home, _ := os.UserHomeDir()
		batonHome = filepath.Join(home, ".claude", "baton")
	}
	if _, err := os.Stat(batonHome); err == nil {
		fmt.Println()
		fmt.Println(style.Heading("Group 3: Local Baton sync"))
		g3 := checkBatonSync(batonHome)
		for _, r := range g3 {
			printResult(r)
		}
	}
	// If batonHome doesn't exist, group 3 is skipped entirely (no output).

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

	// --- --sync-baton: copy embedded Baton to local ---
	if *syncBaton {
		if err := syncBatonHome(batonHome); err != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] --sync-baton: %v\n", err)
			return 1
		}
		fmt.Printf("[OK]    synced embedded Baton docs to %s\n", batonHome)
	}

	if hasError {
		return 1
	}
	return 0
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

	// Check VERSION.txt.
	v := prompt.BatonVersion()
	if v == "" {
		results = append(results, checkResult{
			level:  levelError,
			name:   "baton/VERSION.txt",
			detail: "version file empty or unparseable",
		})
	} else if v == "0.0.0" || strings.HasSuffix(v, "-dev") {
		results = append(results, checkResult{
			level:  levelWarn,
			name:   "baton/VERSION.txt",
			detail: fmt.Sprintf("version=%s — not yet set; run 'sworn doctor --set-version <v>'", v),
		})
	} else {
		results = append(results, checkResult{
			level:  levelOK,
			name:   "baton/VERSION.txt",
			detail: fmt.Sprintf("version=%s", v),
		})
	}

	return results
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
			detail: "contains legacy Baton splice content. Run 'sworn init' to replace with the current minimal MCP-pointer template (backs up old AGENTS.md to AGENTS.md.bak)",
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
		// Back up old content.
		bakPath := filepath.Join(repoRoot, "AGENTS.md.bak")
		if err := os.WriteFile(bakPath, content, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "  [ERROR] failed to write backup: %v\n", err)
		} else {
			fmt.Println("  backed up old AGENTS.md to AGENTS.md.bak")
		}
		// Write minimal template (just the fragment, which is what sworn init
		// would create for a fresh repo).
		if err := os.WriteFile(agentsPath, []byte(adopt.AgentsFragment()+"\n"), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "  [ERROR] failed to write new AGENTS.md: %v\n", err)
		} else {
			fmt.Println("  wrote minimal AGENTS.md template")
			fixed++
		}
	}

	return fixed
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
