package gate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mockRunner is a testRunner that returns pre-canned results keyed by
// "<dir>/<name> <args...>".
type mockRunner struct {
	results map[string]mockResult
}

type mockResult struct {
	stdout   string
	exitCode int
	err      error
}

func (m mockRunner) Run(dir, name string, args ...string) (string, int, error) {
	key := dir + "/" + name + " " + strings.Join(args, " ")
	if r, ok := m.results[key]; ok {
		return r.stdout, r.exitCode, r.err
	}
	return "", -1, nil
}

// --- unit: all-pass ---

func TestRunRegress_AllPass(t *testing.T) {
	worktree := t.TempDir()
	// Root go.mod so the Go suite resolves the repo-root module path.
	os.WriteFile(filepath.Join(worktree, "go.mod"), []byte("module fixture\n"), 0644)
	// Create package.json so the TS suite check passes.
	os.WriteFile(filepath.Join(worktree, "package.json"), []byte("{}"), 0644)

	runner := mockRunner{results: mockResults(worktree, map[string]mockResult{
		"go test ./...":                          {stdout: "ok\t./...\t0.123s\n", exitCode: 0},
		"pnpm --version":                         {stdout: "8.15.0\n", exitCode: 0},
		"pnpm test":                              {stdout: "> test passed\n", exitCode: 0},
		"git diff --exit-code -- **/testdata/**": {stdout: "", exitCode: 0},
	})}

	report, err := runRegress(worktree, "test-release", runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report.Failed != 0 {
		t.Errorf("expected 0 failed, got %d", report.Failed)
	}
	if report.Skipped != 0 {
		t.Errorf("expected 0 skipped, got %d", report.Skipped)
	}
	if report.Passed != 3 {
		t.Errorf("expected 3 passed (go, ts, golden), got %d", report.Passed)
	}
	if !report.AllPassed {
		t.Error("expected AllPassed=true")
	}
	if report.HasViolations() {
		t.Error("expected no violations")
	}
	if report.Release != "test-release" {
		t.Errorf("expected Release=test-release, got %s", report.Release)
	}
}

// --- unit: all-fail ---

func TestRunRegress_AllFail(t *testing.T) {
	worktree := t.TempDir()
	os.WriteFile(filepath.Join(worktree, "go.mod"), []byte("module fixture\n"), 0644)
	os.WriteFile(filepath.Join(worktree, "package.json"), []byte("{}"), 0644)

	runner := mockRunner{results: mockResults(worktree, map[string]mockResult{
		"go test ./...":                          {stdout: "FAIL\t./...\n", exitCode: 1},
		"pnpm --version":                         {stdout: "8.15.0\n", exitCode: 0},
		"pnpm test":                              {stdout: "FAIL\n", exitCode: 1},
		"git diff --exit-code -- **/testdata/**": {stdout: "diff --git ...\n", exitCode: 1},
	})}

	report, err := runRegress(worktree, "test-release", runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report.Failed != 3 {
		t.Errorf("expected 3 failed, got %d", report.Failed)
	}
	if report.Skipped != 0 {
		t.Errorf("expected 0 skipped, got %d", report.Skipped)
	}
	if report.Passed != 0 {
		t.Errorf("expected 0 passed, got %d", report.Passed)
	}
	if report.AllPassed {
		t.Error("expected AllPassed=false")
	}
	if !report.HasViolations() {
		t.Error("expected violations")
	}
}

// --- unit: mixed pass/fail/skip ---

func TestRunRegress_Mixed(t *testing.T) {
	// Go passes, pnpm missing (skip), golden fails.
	// No package.json needed — pnpm check fails before we reach os.Stat.
	worktree := t.TempDir()
	os.WriteFile(filepath.Join(worktree, "go.mod"), []byte("module fixture\n"), 0644)

	runner := mockRunner{results: mockResults(worktree, map[string]mockResult{
		"go test ./...":                          {stdout: "ok\t./...\n", exitCode: 0},
		"pnpm --version":                         {stdout: "", exitCode: -1, err: errMockNotFound},
		"git diff --exit-code -- **/testdata/**": {stdout: "diff --git a/foo b/foo\n", exitCode: 1},
	})}

	report, err := runRegress(worktree, "test-release", runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report.Passed != 1 {
		t.Errorf("expected 1 passed (go), got %d", report.Passed)
	}
	if report.Failed != 1 {
		t.Errorf("expected 1 failed (golden), got %d", report.Failed)
	}
	if report.Skipped != 1 {
		t.Errorf("expected 1 skipped (ts), got %d", report.Skipped)
	}
	if report.AllPassed {
		t.Error("expected AllPassed=false")
	}
	if !report.HasViolations() {
		t.Error("expected violations (golden failed)")
	}

	// Verify the TS suite was skipped with the right reason.
	var tsSuite *SuiteResult
	for i := range report.Suites {
		if report.Suites[i].Name == "TypeScript tests" {
			tsSuite = &report.Suites[i]
			break
		}
	}
	if tsSuite == nil {
		t.Fatal("expected TypeScript tests suite in report")
	}
	if !tsSuite.Skipped {
		t.Error("expected TS suite to be skipped")
	}
	if tsSuite.SkippedReason != "pnpm not available" {
		t.Errorf("expected skip reason 'pnpm not available', got %q", tsSuite.SkippedReason)
	}
}

// --- unit: pnpm available but no package.json → skip ---

func TestRunRegress_NoPackageJSON(t *testing.T) {
	// pnpm is available but there's no package.json in the worktree.
	worktree := t.TempDir() // no package.json
	os.WriteFile(filepath.Join(worktree, "go.mod"), []byte("module fixture\n"), 0644)

	runner := mockRunner{results: mockResults(worktree, map[string]mockResult{
		"go test ./...":                          {stdout: "ok\n", exitCode: 0},
		"pnpm --version":                         {stdout: "8.15.0\n", exitCode: 0},
		"git diff --exit-code -- **/testdata/**": {stdout: "", exitCode: 0},
	})}

	report, err := runRegress(worktree, "test-release", runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report.Skipped != 1 {
		t.Errorf("expected 1 skipped (ts, no package.json), got %d", report.Skipped)
	}
	// Verify the TS suite was skipped with the right reason.
	var tsSuite *SuiteResult
	for i := range report.Suites {
		if report.Suites[i].Name == "TypeScript tests" {
			tsSuite = &report.Suites[i]
			break
		}
	}
	if tsSuite == nil {
		t.Fatal("expected TypeScript tests suite in report")
	}
	if !tsSuite.Skipped {
		t.Error("expected TS suite to be skipped")
	}
	if tsSuite.SkippedReason != "no package.json in worktree" {
		t.Errorf("expected skip reason 'no package.json in worktree', got %q", tsSuite.SkippedReason)
	}
}

// --- unit: Go module in a first-level subdirectory (consumer-repo shape) ---

func TestRunRegress_GoModuleInSubdir(t *testing.T) {
	worktree := t.TempDir()
	os.MkdirAll(filepath.Join(worktree, "go"), 0755)
	os.WriteFile(filepath.Join(worktree, "go", "go.mod"), []byte("module fixture\n"), 0644)
	os.WriteFile(filepath.Join(worktree, "package.json"), []byte("{}"), 0644)

	moduleDir := filepath.Join(worktree, "go")
	results := map[string]mockResult{
		"pnpm --version":                         {stdout: "8.15.0\n", exitCode: 0},
		"pnpm test":                              {stdout: "> test passed\n", exitCode: 0},
		"git diff --exit-code -- **/testdata/**": {stdout: "", exitCode: 0},
	}
	runner := mockRunner{results: mockResults(worktree, results)}
	// go test must run with cmd.Dir = the module dir, not the worktree root.
	runner.results[moduleDir+"/go test ./..."] = mockResult{stdout: "ok\t./...\t0.1s\n", exitCode: 0}

	report, err := runRegress(worktree, "test-release", runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var goSuite *SuiteResult
	for i := range report.Suites {
		if report.Suites[i].Name == "Go tests" {
			goSuite = &report.Suites[i]
			break
		}
	}
	if goSuite == nil {
		t.Fatal("expected Go tests suite in report")
	}
	if goSuite.Skipped {
		t.Fatalf("expected Go suite not skipped, got skip reason %q", goSuite.SkippedReason)
	}
	if !goSuite.Passed {
		t.Errorf("expected Go suite Passed=true (ran from module dir %s), got Passed=false, ExitCode=%d", moduleDir, goSuite.ExitCode)
	}
}

// --- unit: no go.mod anywhere under the worktree -> skipped, not FAIL ---

func TestRunRegress_NoGoMod(t *testing.T) {
	worktree := t.TempDir() // no go.mod anywhere

	runner := mockRunner{results: mockResults(worktree, map[string]mockResult{
		"pnpm --version":                         {stdout: "", exitCode: -1, err: errMockNotFound},
		"git diff --exit-code -- **/testdata/**": {stdout: "", exitCode: 0},
	})}

	report, err := runRegress(worktree, "test-release", runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var goSuite *SuiteResult
	for i := range report.Suites {
		if report.Suites[i].Name == "Go tests" {
			goSuite = &report.Suites[i]
			break
		}
	}
	if goSuite == nil {
		t.Fatal("expected Go tests suite in report")
	}
	if !goSuite.Skipped {
		t.Error("expected Go suite to be skipped when no go.mod exists")
	}
	if goSuite.SkippedReason == "" {
		t.Error("expected a non-empty skip reason")
	}
	if goSuite.Passed {
		t.Error("a skipped suite must not also report Passed=true")
	}
}

// --- unit: multiple first-level Go modules -> skipped with distinct reason (D1) ---

func TestRunRegress_MultipleGoModules(t *testing.T) {
	worktree := t.TempDir()
	os.MkdirAll(filepath.Join(worktree, "a"), 0755)
	os.MkdirAll(filepath.Join(worktree, "b"), 0755)
	os.WriteFile(filepath.Join(worktree, "a", "go.mod"), []byte("module a\n"), 0644)
	os.WriteFile(filepath.Join(worktree, "b", "go.mod"), []byte("module b\n"), 0644)

	runner := mockRunner{results: mockResults(worktree, map[string]mockResult{
		"pnpm --version":                         {stdout: "", exitCode: -1, err: errMockNotFound},
		"git diff --exit-code -- **/testdata/**": {stdout: "", exitCode: 0},
	})}

	report, err := runRegress(worktree, "test-release", runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var goSuite *SuiteResult
	for i := range report.Suites {
		if report.Suites[i].Name == "Go tests" {
			goSuite = &report.Suites[i]
			break
		}
	}
	if goSuite == nil {
		t.Fatal("expected Go tests suite in report")
	}
	if !goSuite.Skipped {
		t.Error("expected Go suite to be skipped when multiple modules are found")
	}
	if !strings.Contains(goSuite.SkippedReason, "multiple") {
		t.Errorf("expected a multi-module skip reason, got %q", goSuite.SkippedReason)
	}
}

// --- unit: vendor/hidden dirs are ignored during discovery (R-01) ---

func TestFindGoModuleDir_IgnoresVendorAndHidden(t *testing.T) {
	worktree := t.TempDir()
	os.MkdirAll(filepath.Join(worktree, "vendor"), 0755)
	os.MkdirAll(filepath.Join(worktree, ".git"), 0755)
	os.MkdirAll(filepath.Join(worktree, "go"), 0755)
	os.WriteFile(filepath.Join(worktree, "vendor", "go.mod"), []byte("module vendored\n"), 0644)
	os.WriteFile(filepath.Join(worktree, ".git", "go.mod"), []byte("module hidden\n"), 0644)
	os.WriteFile(filepath.Join(worktree, "go", "go.mod"), []byte("module fixture\n"), 0644)

	dir, found := findGoModuleDir(worktree)
	if found != 1 {
		t.Fatalf("expected exactly 1 module found, got %d", found)
	}
	want := filepath.Join(worktree, "go")
	if dir != want {
		t.Errorf("expected discovery to pick %s, got %s", want, dir)
	}
}

// --- unit: PrintRegress output ---

func TestPrintRegress_Output(t *testing.T) {
	report := &RegressReport{
		Release:  "test-release",
		Worktree: "/worktree",
		Suites: []SuiteResult{
			{Name: "Go tests", Passed: true, ExitCode: 0},
			{Name: "TypeScript tests", Skipped: true, SkippedReason: "pnpm not available"},
			{Name: "Golden fixtures", Passed: true, ExitCode: 0},
		},
		Passed:    2,
		Failed:    0,
		Skipped:   1,
		AllPassed: true,
	}

	out := PrintRegress(report)
	if !strings.Contains(out, "test-release") {
		t.Error("expected release name in output")
	}
	if !strings.Contains(out, "/worktree") {
		t.Error("expected worktree path in output")
	}
	if !strings.Contains(out, "PASS") {
		t.Error("expected PASS in output")
	}
	if !strings.Contains(out, "SKIP") {
		t.Error("expected SKIP in output")
	}
	if !strings.Contains(out, "All") {
		t.Error("expected summary line in output")
	}

	// JSON round-trip
	jsonOut := JSONRegress(report)
	if !strings.Contains(jsonOut, `"all_passed": true`) {
		t.Error("expected all_passed in JSON")
	}
	if !strings.Contains(jsonOut, `"release": "test-release"`) {
		t.Error("expected release in JSON")
	}
}

// --- helpers ---

// errMockNotFound is used when a mock should simulate a missing command.
var errMockNotFound = &mockExecError{msg: "exec: \"pnpm\": executable file not found in $PATH"}

type mockExecError struct{ msg string }

func (e *mockExecError) Error() string { return e.msg }

// mockResults prefixes keys with the worktree path for convenience.
func mockResults(worktree string, m map[string]mockResult) map[string]mockResult {
	out := make(map[string]mockResult, len(m))
	for k, v := range m {
		out[worktree+"/"+k] = v
	}
	return out
}
