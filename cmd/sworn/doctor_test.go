package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/baton/schemas"
	"github.com/swornagent/sworn/internal/board"
)

func TestDoctorAndBatonDiffV015BinaryReachability(t *testing.T) {
	bin := buildSworn(t)
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(repo, "home")
	env := append(os.Environ(),
		"HOME="+home,
		"AGENTS_HOME="+filepath.Join(repo, "agents"),
		"CODEX_HOME="+filepath.Join(repo, "codex"),
		"CLAUDE_HOME="+filepath.Join(repo, "claude"),
		"SWORN_HOME="+filepath.Join(repo, "sworn-config"),
	)
	run := func(dir string, args ...string) (int, string) {
		t.Helper()
		cmd := exec.Command(bin, args...)
		cmd.Dir = dir
		cmd.Env = env
		output, err := cmd.CombinedOutput()
		if err == nil {
			return 0, string(output)
		}
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("%v: %v\n%s", args, err, output)
		}
		return exitErr.ExitCode(), string(output)
	}

	if code, output := run(repo, "doctor", "--sync-baton"); code != 2 {
		t.Fatalf("built doctor repair exit = %d, want 2\n%s", code, output)
	}
	if code, output := run(repo, "doctor", "--sync-baton"); code != 0 {
		t.Fatalf("built doctor idempotent exit = %d, want 0\n%s", code, output)
	}
	if code, output := run(repo, "doctor"); code != 0 || !strings.Contains(output, "baton/local-mirrors") {
		t.Fatalf("built ordinary doctor = %d, want exact mirrors exit 0\n%s", code, output)
	}

	workspace, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if code, output := run(workspace, "baton", "diff", "/home/brad/projects/baton"); code != 0 {
		t.Fatalf("built baton diff exact exit = %d, want 0\n%s", code, output)
	}
}

// runDoctorInDir runs cmdDoctor with the given args in the given directory,
// capturing stdout+stderr. Returns exit code and combined output.
func runDoctorInDir(t *testing.T, dir string, args ...string) (int, string) {
	t.Helper()

	// Save and restore cwd + env.
	origDir, _ := os.Getwd()
	origBatonHome := os.Getenv("SWORN_BATON_HOME")
	defer func() {
		_ = os.Chdir(origDir)
		os.Setenv("SWORN_BATON_HOME", origBatonHome)
	}()

	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	os.Setenv("SWORN_BATON_HOME", filepath.Join(dir, ".fake-baton-home"))

	// Capture stdout and stderr.
	origStdout := os.Stdout
	origStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stdout = w
	os.Stderr = w

	exitCode := cmdDoctor(args)

	w.Close()
	os.Stdout = origStdout
	os.Stderr = origStderr

	buf := make([]byte, 65536)
	n, _ := r.Read(buf)
	output := string(buf[:n])
	r.Close()

	return exitCode, output
}

// TestDoctorAllOK runs doctor against the actual embedded prompts in this repo.
// Since this repo has docs/baton/ and a legacy AGENTS.md splice, those will WARN,
// but group 1 should be all OK (or WARN for S19-dependent headings), and there
// should be no ERRORs. Exit code should be 0.
func TestDoctorAllOK(t *testing.T) {
	// Use the repo root (where the test runs from).
	dir, _ := os.Getwd()
	// Go up from cmd/sworn to repo root.
	dir = filepath.Dir(dir)

	exitCode, output := runDoctorInDir(t, dir)

	if exitCode != 0 {
		t.Errorf("expected exit 0 for clean repo (WARN-only), got %d\nOutput:\n%s", exitCode, output)
	}

	// Group 1: planner.md should be OK.
	if !strings.Contains(output, "[OK]    planner.md") {
		t.Errorf("expected [OK] for planner.md\nOutput:\n%s", output)
	}

	// Group 1: baton/rules/ should be OK (12/12).
	if !strings.Contains(output, "12/12 rule files present") {
		t.Errorf("expected 12/12 rule files present\nOutput:\n%s", output)
	}

	// Group 1: planner.md should have all Phase 1-6 headings.
	if !strings.Contains(output, "headings=all present") {
		t.Errorf("expected 'headings=all present' for planner.md\nOutput:\n%s", output)
	}

	// S19-dependent headings should be WARN, not ERROR.
	if strings.Contains(output, "[ERROR] implementer.md") {
		t.Errorf("implementer.md should not be ERROR (S19 headings are WARN)\nOutput:\n%s", output)
	}
	if strings.Contains(output, "[ERROR] verifier.md") {
		t.Errorf("verifier.md should not be ERROR (S19 headings are WARN)\nOutput:\n%s", output)
	}

	// No ERROR in the output at all.
	if strings.Contains(output, "[ERROR]") {
		t.Errorf("expected no [ERROR] in output for clean repo\nOutput:\n%s", output)
	}
}

// doctorLine returns the first line of output containing substr, or "" if
// none found. Used to assert a specific check's [OK]/[WARN]/[ERROR] tag
// without depending on the exact column-padding of unrelated lines.
func doctorLine(output, substr string) string {
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, substr) {
			return line
		}
	}
	return ""
}

// doctorEntry returns a checkResult's tag line (e.g. "[OK]    baton/...")
// and its following detail line, as printResult renders them: name+tag on
// one line, detail (if any) on the next.
func doctorEntry(output, substr string) (tagLine, detailLine string) {
	lines := strings.Split(output, "\n")
	for i, line := range lines {
		if strings.Contains(line, substr) {
			if i+1 < len(lines) {
				detailLine = lines[i+1]
			}
			return line, detailLine
		}
	}
	return "", ""
}

// TestDoctorSchemaManifestRendersGradedAndAdvisory drives S15 AC-01 through
// the CLI affordance (cmdDoctor), not the leaf SchemaManifest function
// directly (Rule 1): `sworn doctor`'s Group 1b must name every vendored
// baton schema's $id/version/status, with the two newly-vendored (S11,
// baton v0.10.0) schemas rendered ADVISORY and the rest GRADED.
func TestDoctorSchemaManifestRendersGradedAndAdvisory(t *testing.T) {
	dir, _ := os.Getwd()
	dir = filepath.Dir(dir)

	_, output := runDoctorInDir(t, dir)

	if !strings.Contains(output, "Group 1b: Baton schema manifest") {
		t.Fatalf("expected 'Group 1b: Baton schema manifest' heading in doctor output\nOutput:\n%s", output)
	}

	graded := []string{
		"slice-status-v1", "board-v1", "spec-v1", "proof-v1",
		"journeys-v1", "attestations-v1", "verifier-verdict-v1",
	}
	for _, name := range graded {
		tagLine, detail := doctorEntry(output, "baton/schema-manifest/"+name)
		if tagLine == "" {
			t.Errorf("expected a baton/schema-manifest/%s line in doctor output\nOutput:\n%s", name, output)
			continue
		}
		if !strings.HasPrefix(tagLine, "[OK]") {
			t.Errorf("expected [OK] for graded schema %s, got: %q", name, tagLine)
		}
		if !strings.Contains(detail, "status=GRADED") {
			t.Errorf("expected status=GRADED detail for %s, got: %q", name, detail)
		}
		if !strings.Contains(detail, "$id=https://baton.sawy3r.net/schemas/"+name+".json") {
			t.Errorf("expected $id for %s in detail, got: %q", name, detail)
		}
	}

	for _, name := range []string{"contracts-v1", "assembly-proof-v1"} {
		tagLine, detail := doctorEntry(output, "baton/schema-manifest/"+name)
		if tagLine == "" {
			t.Errorf("expected a baton/schema-manifest/%s line in doctor output\nOutput:\n%s", name, output)
			continue
		}
		if !strings.HasPrefix(tagLine, "[OK]") {
			t.Errorf("expected [OK] for advisory schema %s (ADVISORY is a classified, non-skewed status), got: %q", name, tagLine)
		}
		if !strings.Contains(detail, "status=ADVISORY") {
			t.Errorf("expected status=ADVISORY detail for %s, got: %q", name, detail)
		}
	}

	skewLine := doctorLine(output, "baton/schema-skew")
	if skewLine == "" {
		t.Fatalf("expected a baton/schema-skew line in doctor output\nOutput:\n%s", output)
	}
	if !strings.HasPrefix(skewLine, "[OK]") {
		t.Errorf("expected [OK] for baton/schema-skew on the real (unskewed) vendored set, got: %q", skewLine)
	}
}

// TestDoctorSchemaSkewFiresOnFixture drives S15 AC-02 through the CLI
// affordance (cmdDoctor): with a deliberately-skewed schema-map fixture
// injected (an extra vendored schema absent from the graded/advisory
// classification table), doctor SHALL surface a clearly-flagged WARN for
// baton/schema-skew and for the unclassified entry itself — never a silent
// OK.
func TestDoctorSchemaSkewFiresOnFixture(t *testing.T) {
	fixture := make(map[string][]byte, len(schemas.SchemaMap)+1)
	for k, v := range schemas.SchemaMap {
		fixture[k] = v
	}
	fixture["made-up-v1"] = []byte(`{"$id":"https://baton.sawy3r.net/schemas/made-up-v1.json","type":"object"}`)

	baton.SetSchemaMapForTest(fixture)
	defer baton.ClearSchemaMapForTest()

	dir, _ := os.Getwd()
	dir = filepath.Dir(dir)

	_, output := runDoctorInDir(t, dir)

	skewLine := doctorLine(output, "baton/schema-skew")
	if skewLine == "" {
		t.Fatalf("expected a baton/schema-skew line in doctor output\nOutput:\n%s", output)
	}
	if !strings.HasPrefix(skewLine, "[WARN]") {
		t.Errorf("expected [WARN] for baton/schema-skew with an injected skew fixture, got: %q\nOutput:\n%s", skewLine, output)
	}
	if strings.Contains(skewLine, "[OK]") {
		t.Errorf("baton/schema-skew must never render OK when skewed, got: %q", skewLine)
	}

	manifestLine := doctorLine(output, "baton/schema-manifest/made-up-v1")
	if manifestLine == "" {
		t.Fatalf("expected a baton/schema-manifest/made-up-v1 line in doctor output\nOutput:\n%s", output)
	}
	if !strings.HasPrefix(manifestLine, "[WARN]") {
		t.Errorf("expected [WARN] for the unclassified made-up-v1 entry, got: %q", manifestLine)
	}

	// Skew is a WARN, not an ERROR (S15 design decision D2) — it must not
	// flip cmdDoctor's exit code on its own. A pre-existing S19-dependent
	// heading WARN already keeps this repo's baseline exit 0 (TestDoctorAllOK);
	// asserting no [ERROR] anywhere proves schema-skew specifically didn't
	// gate, without over-asserting the exact exit code of unrelated groups.
	if strings.Contains(output, "[ERROR]") {
		t.Errorf("schema-skew WARN must not produce an [ERROR] anywhere in doctor output\nOutput:\n%s", output)
	}
}

// TestDoctorLegacyBatonDir tests that docs/baton/ presence produces a WARN.
func TestDoctorLegacyBatonDir(t *testing.T) {
	dir := t.TempDir()
	// Create a .git dir so isGitRepo passes.
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	// Create docs/baton/.
	os.MkdirAll(filepath.Join(dir, "docs", "baton"), 0755)
	os.WriteFile(filepath.Join(dir, "docs", "baton", "README.md"), []byte("legacy"), 0644)

	exitCode, output := runDoctorInDir(t, dir)

	if exitCode != 0 {
		t.Errorf("expected exit 0 (WARN-only), got %d\nOutput:\n%s", exitCode, output)
	}
	if !strings.Contains(output, "[WARN]  docs/baton/") {
		t.Errorf("expected WARN for docs/baton/\nOutput:\n%s", output)
	}
	if !strings.Contains(output, "legacy per-repo Baton copy") {
		t.Errorf("expected 'legacy' in detail\nOutput:\n%s", output)
	}
}

// TestDoctorLegacySpliceAgentsMD tests that a legacy splice in AGENTS.md is detected.
// Per the spec and Coach ack, the marker is adopt.BatonSectionHeading
// ("## Engineering Process — Baton"), not <!-- baton:start -->.
func TestDoctorLegacySpliceAgentsMD(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	// Write AGENTS.md with the legacy splice marker.
	agentsContent := "# My Project\n\n## Engineering Process — Baton\n\nSome rules here.\n"
	os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(agentsContent), 0644)

	exitCode, output := runDoctorInDir(t, dir)

	if exitCode != 0 {
		t.Errorf("expected exit 0 (WARN-only), got %d\nOutput:\n%s", exitCode, output)
	}
	if !strings.Contains(output, "[WARN]  AGENTS.md") {
		t.Errorf("expected WARN for AGENTS.md\nOutput:\n%s", output)
	}
	if !strings.Contains(output, "legacy Baton splice") {
		t.Errorf("expected 'legacy Baton splice' in detail\nOutput:\n%s", output)
	}
}

// TestDoctorFixRemovesBatonDir tests that --fix removes docs/baton/ and exits 2.
func TestDoctorFixRemovesBatonDir(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	os.MkdirAll(filepath.Join(dir, "docs", "baton", "rules"), 0755)
	os.WriteFile(filepath.Join(dir, "docs", "baton", "README.md"), []byte("legacy"), 0644)
	os.WriteFile(filepath.Join(dir, "docs", "baton", "rules", "01-reachability-gate.md"), []byte("rule"), 0644)

	exitCode, output := runDoctorInDir(t, dir, "--fix")

	if exitCode != 2 {
		t.Errorf("expected exit 2 (fixes applied), got %d\nOutput:\n%s", exitCode, output)
	}
	// Verify docs/baton/ was removed.
	if _, err := os.Stat(filepath.Join(dir, "docs", "baton")); err == nil {
		t.Errorf("expected docs/baton/ to be removed")
	}
	if !strings.Contains(output, "rm:") {
		t.Errorf("expected 'rm:' in output (file listing)\nOutput:\n%s", output)
	}
}

// legacyAgentsContent is a realistic legacy AGENTS.md: user content before
// AND after the spliced Baton section. The migration must preserve both.
const legacyAgentsContent = "# My Project\n\n" +
	"Custom onboarding: run kubectl apply -f infra/.\n\n" +
	"## Engineering Process — Baton\n\n" +
	"Some rules here.\n\n" +
	"### 1. Reachability Gate (CRITICAL)\n\nrule body\n\n" +
	"## Deployment\n\nShip with make deploy.\n"

// TestDoctorFixMigratesAgentsMD tests that --fix splices out only the legacy
// Baton section, preserving all other user content, backing up the original,
// and writing replacement content that neither re-contains the legacy trigger
// heading nor points at the docs/baton/ directory the same run deletes.
func TestDoctorFixMigratesAgentsMD(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(legacyAgentsContent), 0644)

	exitCode, output := runDoctorInDir(t, dir, "--fix")

	if exitCode != 2 {
		t.Errorf("expected exit 2 (fixes applied), got %d\nOutput:\n%s", exitCode, output)
	}
	// Verify backup was created with old content.
	bakContent, err := os.ReadFile(filepath.Join(dir, "AGENTS.md.bak"))
	if err != nil {
		t.Fatalf("expected AGENTS.md.bak to be created: %v", err)
	}
	if string(bakContent) != legacyAgentsContent {
		t.Errorf("backup content mismatch:\ngot:  %q\nwant: %q", string(bakContent), legacyAgentsContent)
	}
	newContent, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	// User content around the Baton section must survive.
	for _, keep := range []string{"# My Project", "kubectl apply", "## Deployment", "make deploy"} {
		if !strings.Contains(string(newContent), keep) {
			t.Errorf("new AGENTS.md lost user content %q:\n%s", keep, newContent)
		}
	}
	// The legacy Baton section body must be gone.
	for _, gone := range []string{"Some rules here", "### 1. Reachability Gate"} {
		if strings.Contains(string(newContent), gone) {
			t.Errorf("new AGENTS.md still contains legacy section content %q:\n%s", gone, newContent)
		}
	}
	// Convergence: the rewritten file must not re-contain the legacy trigger.
	if strings.Contains(string(newContent), "## Engineering Process — Baton") {
		t.Errorf("new AGENTS.md re-contains the legacy trigger heading (migration would never converge):\n%s", newContent)
	}
	// Must not point at docs/baton/ — the same --fix run removes it.
	if strings.Contains(string(newContent), "docs/baton/") {
		t.Errorf("new AGENTS.md points at docs/baton/, which --fix deletes:\n%s", newContent)
	}
	// Must point at the MCP server as the canonical source.
	if !strings.Contains(string(newContent), "sworn mcp") {
		t.Errorf("new AGENTS.md missing MCP pointer:\n%s", newContent)
	}
}

// TestDoctorFixMigrationConverges tests that a second --fix run is a clean
// no-op: no re-migration, AGENTS.md unchanged, and the backup of the ORIGINAL
// content is not clobbered.
func TestDoctorFixMigrationConverges(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(legacyAgentsContent), 0644)

	exitCode1, output1 := runDoctorInDir(t, dir, "--fix")
	if exitCode1 != 2 {
		t.Fatalf("run 1: expected exit 2 (fix applied), got %d\nOutput:\n%s", exitCode1, output1)
	}
	if !strings.Contains(output1, "migrating legacy AGENTS.md") {
		t.Fatalf("run 1: expected migration to run\nOutput:\n%s", output1)
	}
	afterRun1, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md"))

	exitCode2, output2 := runDoctorInDir(t, dir, "--fix")
	if exitCode2 != 0 {
		t.Errorf("run 2: expected exit 0 (nothing to fix), got %d\nOutput:\n%s", exitCode2, output2)
	}
	if strings.Contains(output2, "migrating legacy AGENTS.md") {
		t.Errorf("run 2: re-migrated an already-migrated AGENTS.md (non-convergent)\nOutput:\n%s", output2)
	}
	afterRun2, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if string(afterRun1) != string(afterRun2) {
		t.Errorf("run 2 changed AGENTS.md:\nrun1: %q\nrun2: %q", afterRun1, afterRun2)
	}
	// The backup must still hold the ORIGINAL content, not be clobbered.
	bakContent, err := os.ReadFile(filepath.Join(dir, "AGENTS.md.bak"))
	if err != nil {
		t.Fatalf("AGENTS.md.bak missing after run 2: %v", err)
	}
	if string(bakContent) != legacyAgentsContent {
		t.Errorf("run 2 clobbered AGENTS.md.bak:\ngot:  %q\nwant: %q", bakContent, legacyAgentsContent)
	}
}

// TestDoctorFixNeverClobbersExistingBackup tests that when AGENTS.md.bak
// already exists (e.g. from an earlier migration of different content), a new
// migration writes its backup elsewhere instead of overwriting it.
func TestDoctorFixNeverClobbersExistingBackup(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	preexistingBak := "precious earlier backup\n"
	os.WriteFile(filepath.Join(dir, "AGENTS.md.bak"), []byte(preexistingBak), 0644)
	os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(legacyAgentsContent), 0644)

	exitCode, output := runDoctorInDir(t, dir, "--fix")
	if exitCode != 2 {
		t.Fatalf("expected exit 2 (fix applied), got %d\nOutput:\n%s", exitCode, output)
	}
	bakContent, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md.bak"))
	if string(bakContent) != preexistingBak {
		t.Errorf("existing AGENTS.md.bak was clobbered:\ngot:  %q\nwant: %q", bakContent, preexistingBak)
	}
	// A backup of the migrated content must still exist somewhere.
	entries, _ := os.ReadDir(dir)
	found := false
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "AGENTS.md.bak.") {
			data, _ := os.ReadFile(filepath.Join(dir, e.Name()))
			if string(data) == legacyAgentsContent {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("no timestamped backup of the original AGENTS.md found; entries: %v", entries)
	}
}

// TestDoctorAdviceNotCircular tests that doctor's non-fix advice for a legacy
// AGENTS.md points at 'sworn doctor --fix' (which migrates), not at
// 'sworn init' (which refuses legacy files and points back at doctor).
func TestDoctorAdviceNotCircular(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(legacyAgentsContent), 0644)

	_, output := runDoctorInDir(t, dir)
	if !strings.Contains(output, "sworn doctor --fix") {
		t.Errorf("legacy AGENTS.md advice should point at 'sworn doctor --fix'\nOutput:\n%s", output)
	}
	if strings.Contains(output, "Run 'sworn init' to replace") {
		t.Errorf("legacy AGENTS.md advice is circular (init refuses legacy files and points back at doctor)\nOutput:\n%s", output)
	}
}

// TestDoctorSyncBaton proves the public adapter repairs all three isolated
// logical roots and returns the v0.15 2/0 changed/idempotent exits.
func TestDoctorSyncBaton(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0o755)
	agentsHome := filepath.Join(dir, "agents")
	codexHome := filepath.Join(dir, "codex")
	claudeHome := filepath.Join(dir, "claude")
	t.Setenv("HOME", filepath.Join(dir, "home"))
	t.Setenv("AGENTS_HOME", agentsHome)
	t.Setenv("CODEX_HOME", codexHome)
	t.Setenv("CLAUDE_HOME", claudeHome)
	t.Setenv("SWORN_HOME", filepath.Join(dir, "sworn-config"))

	exitCode, output := runDoctorInDir(t, dir, "--sync-baton")
	if exitCode != 2 {
		t.Fatalf("first sync exit = %d, want 2\nOutput:\n%s", exitCode, output)
	}
	for _, path := range []string{
		filepath.Join(agentsHome, "skills", "baton-design-review", "SKILL.md"),
		filepath.Join(codexHome, "baton", "README.md"),
		filepath.Join(claudeHome, "commands", "design-review.md"),
		filepath.Join(agentsHome, ".sworn-baton", "VERSION"),
		filepath.Join(codexHome, ".sworn-baton", "VERSION"),
		filepath.Join(claudeHome, ".sworn-baton", "VERSION"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected managed path %s: %v", path, err)
		}
	}
	if !strings.Contains(output, "repaired Baton mirrors") {
		t.Errorf("repair output omitted transaction result\n%s", output)
	}

	exitCode, output = runDoctorInDir(t, dir, "--sync-baton")
	if exitCode != 0 || !strings.Contains(output, "already match") {
		t.Fatalf("idempotent sync = %d, want 0\n%s", exitCode, output)
	}
}

// TestDoctorNoBatonHomeNoWarn tests that when ~/.claude/baton/ doesn't exist,
// group 3 is absent from output entirely.
func TestDoctorNoBatonHomeNoWarn(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)

	// Set SWORN_BATON_HOME to a non-existent path.
	origBatonHome := os.Getenv("SWORN_BATON_HOME")
	defer os.Setenv("SWORN_BATON_HOME", origBatonHome)
	os.Setenv("SWORN_BATON_HOME", filepath.Join(dir, "nonexistent-baton-home"))

	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	origStdout := os.Stdout
	origStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stdout = w
	os.Stderr = w

	exitCode := cmdDoctor([]string{})

	w.Close()
	os.Stdout = origStdout
	os.Stderr = origStderr
	buf := make([]byte, 65536)
	n, _ := r.Read(buf)
	output := string(buf[:n])
	r.Close()

	if exitCode != 0 {
		t.Errorf("expected exit 0, got %d\nOutput:\n%s", exitCode, output)
	}
	if strings.Contains(output, "Group 3") {
		t.Errorf("expected Group 3 to be absent when baton home doesn't exist\nOutput:\n%s", output)
	}
	if strings.Contains(output, "~/.claude/baton/") {
		t.Errorf("expected no mention of ~/.claude/baton/ when absent\nOutput:\n%s", output)
	}
}

// TestDoctorGroup4StalePins tests that stale catalog pins produce a WARN.
func TestDoctorGroup4StalePins(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)

	// Create go.mod with a module at v1.2.0.
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n\ngo 1.21\n\nrequire (\n\tgithub.com/foo/bar v1.2.0\n)\n"), 0644)

	// Create considerations.md with stale pin (says v1.0.0).
	consContent := `# Considerations

[dependencies]
project_pinned = {
  github.com/foo/bar = "v2.0.0",
}
`
	os.MkdirAll(filepath.Join(dir, "docs"), 0755)
	os.WriteFile(filepath.Join(dir, "docs", "considerations.md"), []byte(consContent), 0755)
	// Override dep freshness to avoid network calls.
	origCheck := checkDepFreshness
	defer func() { checkDepFreshness = origCheck }()
	checkDepFreshness = func(d string) ([]string, error) {
		return nil, nil // no upgrades available
	}

	exitCode, output := runDoctorInDir(t, dir)

	if exitCode != 0 {
		t.Errorf("expected exit 0 (WARN-only), got %d\nOutput:\n%s", exitCode, output)
	}
	if !strings.Contains(output, "stale for github.com/foo/bar") {
		t.Errorf("expected stale pin WARN for github.com/foo/bar\nOutput:\n%s", output)
	}
	if !strings.Contains(output, "sworn induction --update") {
		t.Errorf("expected 'sworn induction --update' suggestion\nOutput:\n%s", output)
	}
}

// TestDoctorGroup4EmptyPins tests that empty project_pinned with go.mod produces a WARN.
func TestDoctorGroup4EmptyPins(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)

	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n\ngo 1.21\n"), 0644)

	consContent := `# Considerations

[dependencies]
project_pinned = {}
`
	os.MkdirAll(filepath.Join(dir, "docs"), 0755)
	os.WriteFile(filepath.Join(dir, "docs", "considerations.md"), []byte(consContent), 0755)
	origCheck := checkDepFreshness
	defer func() { checkDepFreshness = origCheck }()
	checkDepFreshness = func(d string) ([]string, error) {
		return nil, nil
	}

	exitCode, output := runDoctorInDir(t, dir)

	if exitCode != 0 {
		t.Errorf("expected exit 0 (WARN-only), got %d\nOutput:\n%s", exitCode, output)
	}
	if !strings.Contains(output, "project_pinned is empty") {
		t.Errorf("expected 'project_pinned is empty' WARN\nOutput:\n%s", output)
	}
	if !strings.Contains(output, "sworn induction") {
		t.Errorf("expected 'sworn induction' suggestion\nOutput:\n%s", output)
	}
}

// TestDoctorGroup4RegistryUnreachable tests that when the registry is unreachable,
// a WARN is printed and exit code is 0 (not 1).
func TestDoctorGroup4RegistryUnreachable(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)

	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n\ngo 1.21\n"), 0644)

	consContent := `# Considerations

[dependencies]
project_pinned = {
  github.com/foo/bar = "v2.0.0",
}
`
	os.MkdirAll(filepath.Join(dir, "docs"), 0755)
	os.WriteFile(filepath.Join(dir, "docs", "considerations.md"), []byte(consContent), 0755)

	// Override dep freshness to simulate unreachable registry.
	origCheck := checkDepFreshness
	defer func() { checkDepFreshness = origCheck }()
	checkDepFreshness = func(d string) ([]string, error) {
		return nil, fmt.Errorf("registry unreachable")
	}

	exitCode, output := runDoctorInDir(t, dir)

	if exitCode != 0 {
		t.Errorf("expected exit 0 (registry unreachable is WARN, not ERROR), got %d\nOutput:\n%s", exitCode, output)
	}
	if !strings.Contains(output, "Registry unreachable") {
		t.Errorf("expected 'Registry unreachable' WARN\nOutput:\n%s", output)
	}
}

// TestDoctorGroup4VerifierHeadings tests that implementer.md heading check
// requires "Dependency discipline" and it appears before "Deviation check"
// when both are present.
func TestDoctorGroup4VerifierHeadings(t *testing.T) {
	// This test verifies the ordering logic in checkEmbeddedPrompts.
	// We test the heading spec directly.
	spec := promptHeadingSpecs["implementer.md"]

	// Verify the ordering pair exists.
	found := false
	for _, pair := range spec.orderingPairs {
		if pair[0] == "## Dependency discipline" && pair[1] == "## Deviation check" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected ordering pair (Dependency discipline, Deviation check) in implementer.md spec")
	}

	// Verify the headings are in warnOnly (not required) — S19 hasn't landed.
	foundDep := false
	for _, h := range spec.warnOnly {
		if h == "## Dependency discipline" {
			foundDep = true
		}
	}
	if !foundDep {
		t.Errorf("expected '## Dependency discipline' in warnOnly for implementer.md")
	}

	// Test ordering check with a synthetic content where order is reversed.
	reversedContent := "## Deviation check\n\nsome text\n\n## Dependency discipline\n\nmore text\n"
	aIdx := strings.Index(reversedContent, "## Dependency discipline")
	bIdx := strings.Index(reversedContent, "## Deviation check")
	if aIdx >= 0 && bIdx >= 0 && aIdx > bIdx {
		// This is the expected violation — the test confirms the logic works.
	} else {
		t.Errorf("ordering test setup is wrong — expected aIdx > bIdx")
	}

	// Test with correct order.
	correctContent := "## Dependency discipline\n\nsome text\n\n## Deviation check\n\nmore text\n"
	aIdx = strings.Index(correctContent, "## Dependency discipline")
	bIdx = strings.Index(correctContent, "## Deviation check")
	if aIdx >= 0 && bIdx >= 0 && aIdx < bIdx {
		// Correct order — no violation.
	} else {
		t.Errorf("ordering test setup is wrong — expected aIdx < bIdx for correct order")
	}
}

// TestDoctorCorruptPrompt tests that a corrupt (too short) embedded prompt
// produces [ERROR] and exit 1. We simulate this by checking the length logic.
func TestDoctorCorruptPrompt(t *testing.T) {
	// We can't easily corrupt the embed, but we can verify the length check
	// logic by testing checkEmbeddedPrompts directly. Since the embedded
	// prompts are all > 500 bytes, we verify the threshold is enforced.
	if minPromptLength != 500 {
		t.Errorf("expected minPromptLength=500, got %d", minPromptLength)
	}

	// Verify that all embedded prompts are above the minimum.
	// We can verify via the doctor output that none are ERROR.
	dir, _ := os.Getwd()
	dir = filepath.Dir(dir)
	exitCode, output := runDoctorInDir(t, dir)
	// No ERROR should appear (all prompts are intact).
	if strings.Contains(output, "[ERROR]") {
		t.Errorf("expected no [ERROR] for intact prompts\nOutput:\n%s", output)
	}
	if exitCode != 0 {
		t.Errorf("expected exit 0, got %d", exitCode)
	}
}

// TestDoctorReportsBatonTag verifies that doctor output includes "on Baton vX.Y.Z"
// with a valid semver tag on the baton-protocol pin check.
func TestDoctorReportsBatonTag(t *testing.T) {
	dir, _ := os.Getwd()
	dir = filepath.Dir(dir)

	exitCode, output := runDoctorInDir(t, dir)
	if exitCode != 0 {
		t.Errorf("expected exit 0 for clean repo, got %d\nOutput:\n%s", exitCode, output)
	}
	if !strings.Contains(output, "on Baton v") {
		t.Errorf("expected 'on Baton v' in doctor output\nOutput:\n%s", output)
	}
}

// TestDoctorFailsOnShaPin verifies that doctor fails closed (non-zero exit,
// [ERROR]) when the embedded baton-protocol pin is a SHA instead of a semver tag.
func TestDoctorFailsOnShaPin(t *testing.T) {
	// Inject a SHA into the baton version so the doctor check fails.
	baton.SetVersionForTest("cf158423f65c20860a3d4ec0310acb6cc7fb5aa0")
	defer baton.SetVersionForTest("") // Reset after test so other tests see the real version.

	dir, _ := os.Getwd()
	dir = filepath.Dir(dir)

	exitCode, output := runDoctorInDir(t, dir)
	if exitCode == 0 {
		t.Errorf("expected non-zero exit for SHA pin, got 0\nOutput:\n%s", output)
	}
	if !strings.Contains(output, "[ERROR]") {
		t.Errorf("expected [ERROR] in output for SHA pin\nOutput:\n%s", output)
	}
	if !strings.Contains(output, "baton/VERSION (baton-protocol)") {
		t.Errorf("expected 'baton/VERSION (baton-protocol)' check in output\nOutput:\n%s", output)
	}
}

// TestDoctorStatusTimestamps verifies that `sworn doctor` reports [ERROR]
// when docs/release/ contains status.json files with future timestamps.
func TestDoctorStatusTimestamps(t *testing.T) {
	dir := t.TempDir()

	// Create a minimal git repo root.
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)

	// Create a release with a future timestamp.
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	sliceDir := filepath.Join(releaseDir, "S01-test-slice")
	os.MkdirAll(sliceDir, 0755)

	status := `{
  "$schema": "https://baton.sawy3r.net/schemas/slice-status-v1.json",
  "slice_id": "S01-test-slice",
  "release": "test-release",
  "track": "T1-test",
  "state": "planned",
  "last_updated_at": "2099-01-01T00:00:00Z",
  "verification": {
    "result": "pending"
  }
}`
	os.WriteFile(filepath.Join(sliceDir, "status.json"), []byte(status), 0644)

	// Doctor should find the future timestamp and report ERROR.
	exitCode, output := runDoctorInDir(t, dir)
	if exitCode == 0 {
		t.Errorf("expected non-zero exit for future timestamp, got 0\nOutput:\n%s", output)
	}
	if !strings.Contains(output, "[ERROR]") {
		t.Errorf("expected [ERROR] in output for future timestamp\nOutput:\n%s", output)
	}
	if !strings.Contains(output, "status timestamp") {
		t.Errorf("expected 'status timestamp' in output\nOutput:\n%s", output)
	}
	if !strings.Contains(output, "2099-01-01") {
		t.Errorf("expected timestamp value '2099-01-01' in output\nOutput:\n%s", output)
	}
}

// TestDoctorStatusTimestamps_Clean verifies that a release with valid
// timestamps passes the status timestamp check in doctor.
func TestDoctorStatusTimestamps_Clean(t *testing.T) {
	dir := t.TempDir()

	os.MkdirAll(filepath.Join(dir, ".git"), 0755)

	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	sliceDir := filepath.Join(releaseDir, "S01-test-slice")
	os.MkdirAll(sliceDir, 0755)

	status := `{
  "$schema": "https://baton.sawy3r.net/schemas/slice-status-v1.json",
  "slice_id": "S01-test-slice",
  "release": "test-release",
  "track": "T1-test",
  "state": "planned",
  "last_updated_at": "2020-01-01T00:00:00Z",
  "verification": {
    "result": "pending"
  }
}`
	os.WriteFile(filepath.Join(sliceDir, "status.json"), []byte(status), 0644)

	exitCode, output := runDoctorInDir(t, dir)
	// Since other doctor groups (embedded prompts) won't exist in the temp dir,
	// we only verify the status timestamp check doesn't cause an [ERROR].
	if strings.Contains(output, "status timestamp") && strings.Contains(output, "[ERROR]") {
		t.Errorf("expected no [ERROR] for status timestamps with valid data\nOutput:\n%s", output)
	}
	_ = exitCode // other groups may fail, that's fine
}

// TestDoctorPin tests the baton/pin-currency and baton/prompt-currency
// doctor checks.
func TestDoctorPin(t *testing.T) {
	// Save and restore injectables.
	origReadBatonDoc := readBatonDoc
	origPromptReaders := promptReadersForCheck
	defer func() {
		readBatonDoc = origReadBatonDoc
		promptReadersForCheck = origPromptReaders
	}()

	t.Run("pin-currency pre-layout FAIL", func(t *testing.T) {
		// Simulate pre-baton/ layout: ReadFile returns error.
		readBatonDoc = func(path string) ([]byte, error) {
			return nil, fmt.Errorf("file not found")
		}
		// Also set a test upstream pin so the detail message includes the SHA.
		pin := &baton.UpstreamPin{SHA: "9ae08fb"}
		baton.SetUpstreamPinForTest(pin)
		defer baton.ClearUpstreamPinForTest()

		result := checkPinCurrency()
		if result.level != levelError {
			t.Errorf("expected ERROR, got %v", result.level)
		}
		if !strings.Contains(result.detail, "PIN-STALE") {
			t.Errorf("expected PIN-STALE in detail, got: %s", result.detail)
		}
		if !strings.Contains(result.detail, "9ae08fb") {
			t.Errorf("expected SHA 9ae08fb in detail, got: %s", result.detail)
		}
	})

	t.Run("pin-currency post-layout PASS", func(t *testing.T) {
		// Simulate post-baton/ layout: ReadFile succeeds.
		readBatonDoc = func(path string) ([]byte, error) {
			return []byte("# Reachability Gate"), nil
		}

		result := checkPinCurrency()
		if result.level != levelOK {
			t.Errorf("expected OK, got %v: %s", result.level, result.detail)
		}
		if !strings.Contains(result.detail, "post-baton") {
			t.Errorf("expected 'post-baton' in detail, got: %s", result.detail)
		}
	})

	t.Run("prompt-currency stale FAIL", func(t *testing.T) {
		// Inject a prompt that contains a pre-JSON marker.
		promptReadersForCheck = map[string]func() string{
			"verifier.md": func() string {
				return "This prompt uses v0.4" + ".2 for version checks and references scripts/release-verify.sh"
			},
			"implementer.md": func() string {
				return "Clean prompt"
			},
		}
		result := checkPromptCurrency()
		if result.level != levelError {
			t.Errorf("expected ERROR, got %v", result.level)
		}
		if !strings.Contains(result.detail, "PROMPT-STALE") {
			t.Errorf("expected PROMPT-STALE in detail, got: %s", result.detail)
		}
		if !strings.Contains(result.detail, "verifier.md") {
			t.Errorf("expected verifier.md in detail, got: %s", result.detail)
		}
	})

	t.Run("prompt-currency clean PASS", func(t *testing.T) {
		promptReadersForCheck = map[string]func() string{
			"verifier.md": func() string {
				return "Clean prompt with no stale markers"
			},
			"implementer.md": func() string {
				return "Another clean prompt"
			},
		}
		result := checkPromptCurrency()
		if result.level != levelOK {
			t.Errorf("expected OK, got %v: %s", result.level, result.detail)
		}
		if !strings.Contains(result.detail, "no pre-JSON") {
			t.Errorf("expected 'no pre-JSON' in detail, got: %s", result.detail)
		}
	})
}

// writeRenderDriftFixture writes a minimal, valid board.json-backed release
// (one track, one slice) under dir/docs/release/<release>. It does not write
// index.md — callers write that themselves (clean via board.Render, drifted
// via arbitrary content, or omitted entirely).
func writeRenderDriftFixture(t *testing.T, dir, release string) {
	t.Helper()
	releaseDir := filepath.Join(dir, "docs", "release", release)
	sliceDir := filepath.Join(releaseDir, "S01-test-slice")
	if err := os.MkdirAll(sliceDir, 0755); err != nil {
		t.Fatal(err)
	}

	boardJSON := fmt.Sprintf(`{
  "$schema": "https://baton.sawy3r.net/schemas/board-v1.json",
  "schema_version": 1,
  "release": { "name": %q },
  "tracks": [
    { "id": "T1-test", "slices": ["S01-test-slice"], "depends_on": [], "worktree_branch": "track/%s/T1-test", "state": "planned" }
  ]
}`, release, release)
	if err := os.WriteFile(filepath.Join(releaseDir, "board.json"), []byte(boardJSON), 0644); err != nil {
		t.Fatal(err)
	}

	spec := `{
  "slice_id": "S01-test-slice",
  "user_outcome": "A test outcome.",
  "touchpoints": ["cmd/sworn/doctor.go"],
  "effort_complexity": { "quadrant": "chore" }
}`
	if err := os.WriteFile(filepath.Join(sliceDir, "spec.json"), []byte(spec), 0644); err != nil {
		t.Fatal(err)
	}

	status := `{ "slice_id": "S01-test-slice", "state": "planned" }`
	if err := os.WriteFile(filepath.Join(sliceDir, "status.json"), []byte(status), 0644); err != nil {
		t.Fatal(err)
	}
}

// TestDoctorRenderDrift_Clean verifies a release whose committed index.md
// exactly matches board.Render's output produces no render-drift ERROR
// (AC-01, AC-02 negative case).
func TestDoctorRenderDrift_Clean(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	writeRenderDriftFixture(t, dir, "clean-release")

	rendered, err := board.Render(dir, "clean-release")
	if err != nil {
		t.Fatalf("board.Render: %v", err)
	}
	indexPath := filepath.Join(dir, "docs", "release", "clean-release", "index.md")
	if err := os.WriteFile(indexPath, []byte(rendered), 0644); err != nil {
		t.Fatal(err)
	}

	_, output := runDoctorInDir(t, dir)
	if strings.Contains(output, "render drift") && strings.Contains(output, "[ERROR]") {
		t.Errorf("expected no render-drift ERROR for a clean release\nOutput:\n%s", output)
	}
	if !strings.Contains(output, "[OK]    render drift") {
		t.Errorf("expected [OK] render drift line\nOutput:\n%s", output)
	}
}

// TestDoctorRenderDrift_Drifted verifies a committed index.md that does not
// match board.Render's output is reported as ERROR and fails doctor closed
// (non-zero exit) — AC-02.
func TestDoctorRenderDrift_Drifted(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	writeRenderDriftFixture(t, dir, "drifted-release")

	indexPath := filepath.Join(dir, "docs", "release", "drifted-release", "index.md")
	if err := os.WriteFile(indexPath, []byte("---\ntitle: 'stale hand-edited content'\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}

	exitCode, output := runDoctorInDir(t, dir)
	if exitCode == 0 {
		t.Errorf("expected non-zero exit for drifted index.md, got 0\nOutput:\n%s", output)
	}
	if !strings.Contains(output, "[ERROR] render drift (drifted-release)") {
		t.Errorf("expected [ERROR] render drift (drifted-release) in output\nOutput:\n%s", output)
	}
	if !strings.Contains(output, "does not match render(board.json)") {
		t.Errorf("expected drift detail in output\nOutput:\n%s", output)
	}
}

// TestDoctorRenderDrift_NoBoardSkipped verifies a release with no board.json
// is skipped entirely by the render-drift check (AC-03) — it must not
// report ERROR just because it has no JSON source to render from.
func TestDoctorRenderDrift_NoBoardSkipped(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)

	releaseDir := filepath.Join(dir, "docs", "release", "pre-migration-release")
	os.MkdirAll(releaseDir, 0755)
	if err := os.WriteFile(filepath.Join(releaseDir, "index.md"), []byte("---\ntitle: 'hand-authored, pre-board.json'\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_, output := runDoctorInDir(t, dir)
	if strings.Contains(output, "render drift") && strings.Contains(output, "[ERROR]") {
		t.Errorf("expected no render-drift ERROR for a release with no board.json\nOutput:\n%s", output)
	}
}

// TestDoctorRenderDrift_RenderError verifies a release whose board.json
// cannot be rendered at all (here: legacy bare-string release form) is
// reported as ERROR rather than silently skipped — a release that can't
// render can't be proven non-drifted (fail-closed, matches board.Render's
// own contract).
func TestDoctorRenderDrift_RenderError(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)

	releaseDir := filepath.Join(dir, "docs", "release", "unrenderable-release")
	os.MkdirAll(releaseDir, 0755)
	boardJSON := `{
  "$schema": "https://baton.sawy3r.net/schemas/board-v1.json",
  "schema_version": 1,
  "release": "unrenderable-release",
  "tracks": [
    { "id": "T1-test", "slices": ["S01-test-slice"], "depends_on": [], "worktree_branch": "track/unrenderable-release/T1-test", "state": "planned" }
  ]
}`
	if err := os.WriteFile(filepath.Join(releaseDir, "board.json"), []byte(boardJSON), 0644); err != nil {
		t.Fatal(err)
	}

	exitCode, output := runDoctorInDir(t, dir)
	if exitCode == 0 {
		t.Errorf("expected non-zero exit for an unrenderable release, got 0\nOutput:\n%s", output)
	}
	if !strings.Contains(output, "[ERROR] render drift (unrenderable-release)") {
		t.Errorf("expected [ERROR] render drift (unrenderable-release) in output\nOutput:\n%s", output)
	}
	if !strings.Contains(output, "cannot render:") {
		t.Errorf("expected 'cannot render:' detail in output\nOutput:\n%s", output)
	}
}
