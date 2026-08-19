package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/journal"
)

func installProjectPlan(t *testing.T, root, release string) {
	t.Helper()
	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		t.Fatal(err)
	}
	repoView, err := gitx.Open(root, gitExecutable)
	if err != nil {
		t.Fatal(err)
	}
	inertness := func(request gitx.RecordRootRequest) (gitx.RecordRootDecision, error) {
		return gitx.RecordRootDecision{Kind: request.Kind, Repository: request.Repository,
			RecordRoot: request.RecordRoot, Commit: request.Commit, Decision: "inert"}, nil
	}
	identity := gitx.Identity{Name: "Test Engine", Email: "engine@example.test"}
	actions, err := baton.NewActions(baton.UseGitRepository(repoView), inertness, identity)
	if err != nil {
		t.Fatal(err)
	}
	planBytes := []byte(fmt.Sprintf("```baton-plan-v2\n{\n  \"schema_version\": \"baton.plan/v2\",\n  \"release\": %q,\n  \"revision\": 1,\n  \"previous_plan\": null,\n  \"repository\": \"acme-repo\",\n  \"target_ref\": \"refs/heads/main\",\n  \"approval_ref\": \"operator://%s/1\",\n  \"tracks\": [\n    {\n      \"id\": \"T1\",\n      \"depends_on\": [],\n      \"slices\": [\n        {\n          \"id\": \"S1\",\n          \"outcome\": \"Deliver S1.\",\n          \"scope\": {\n            \"include\": [\"README.md\"],\n            \"exclude\": []\n          },\n          \"acceptance\": [\n            {\n              \"id\": \"A1\",\n              \"text\": \"S1 is complete.\"\n            }\n          ],\n          \"checks\": [\"true\"],\n          \"constraints\": [\"deterministic\"],\n          \"depends_on\": [],\n          \"consumes\": []\n        }\n      ]\n    }\n  ]\n}\n```\n\nFixture plan.\n", release, release))
	if _, err := actions.RecordPlanRevision(baton.RecordPlanRevisionInput{
		PlanBytes: planBytes,
		Summary:   "Install fixture plan",
		Detail:    []byte("Fixture detail"),
	}); err != nil {
		t.Fatal(err)
	}
}

// A1: Bare sworn run in a project with exactly one resumable run resumes it.
func TestBareRunResumesSingleResumableRun(t *testing.T) {
	root, _ := projectRepositoryFixture(t)
	installProjectPlan(t, root, "delivery")

	stateDir := filepath.Join(root, ".sworn")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	manifestDir := filepath.Join(stateDir, "runs")
	if err := os.MkdirAll(manifestDir, 0o700); err != nil {
		t.Fatal(err)
	}
	projectWriteManifest(t, manifestDir, "delivery.json", root, "delivery", "run-1")

	manifestBody, err := os.ReadFile(filepath.Join(manifestDir, "delivery.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifestDigest := fmt.Sprintf("sha256:%x", sha256.Sum256(manifestBody))

	journalPath := filepath.Join(stateDir, "sworn.db")
	store, err := journal.Open(context.Background(), journalPath)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 123).UTC()
	if err := store.RegisterRun(context.Background(), journal.Run{
		ID:             "run-1",
		ManifestDigest: manifestDigest,
		Repository:     root,
		Release:        "delivery",
		TargetRef:      "refs/heads/main",
		CreatedAt:      now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordCommand(context.Background(), journal.Command{
		RunID:     "run-1",
		ReplayKey: "manifest",
		Kind:      "start",
		Payload:   manifestBody,
		CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(wd) }()

	var stdout, stderr bytes.Buffer
	code := run([]string{"run"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run bare = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Sworn run run-1") {
		t.Fatalf("stdout did not resume run-1: %s", stdout.String())
	}
}

// A1: Bare sworn run with exactly one recorded-and-approved release and no journal starts it.
func TestBareRunStartsSingleRecordedAndApprovedRelease(t *testing.T) {
	root, _ := projectRepositoryFixture(t)
	installProjectPlan(t, root, "delivery")

	stateDir := filepath.Join(root, ".sworn")
	manifestDir := filepath.Join(stateDir, "runs")
	if err := os.MkdirAll(manifestDir, 0o700); err != nil {
		t.Fatal(err)
	}
	projectWriteManifest(t, manifestDir, "delivery.json", root, "delivery", "run-fresh")

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(wd) }()

	var stdout, stderr bytes.Buffer
	code := run([]string{"run"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run bare = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Sworn run run-fresh") {
		t.Fatalf("stdout did not start fresh run: %s", stdout.String())
	}
}

// A1: Bare sworn run ambiguity refuses with candidate run IDs named.
func TestBareRunRefusesAmbiguousRuns(t *testing.T) {
	root, _ := projectRepositoryFixture(t)
	installProjectPlan(t, root, "rel-a")
	installProjectPlan(t, root, "rel-b")

	stateDir := filepath.Join(root, ".sworn")
	manifestDir := filepath.Join(stateDir, "runs")
	if err := os.MkdirAll(manifestDir, 0o700); err != nil {
		t.Fatal(err)
	}
	projectWriteManifest(t, manifestDir, "rel-a.json", root, "rel-a", "run-a")
	projectWriteManifest(t, manifestDir, "rel-b.json", root, "rel-b", "run-b")

	journalPath := filepath.Join(stateDir, "sworn.db")
	store, err := journal.Open(context.Background(), journalPath)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 123).UTC()
	if err := store.RegisterRun(context.Background(), projectJournalRun(
		"run-a", root, "rel-a", now,
	)); err != nil {
		t.Fatal(err)
	}
	if err := store.RegisterRun(context.Background(), projectJournalRun(
		"run-b", root, "rel-b", now,
	)); err != nil {
		t.Fatal(err)
	}
	_ = store.Close()

	wd, _ := os.Getwd()
	_ = os.Chdir(root)
	defer func() { _ = os.Chdir(wd) }()

	var stdout, stderr bytes.Buffer
	code := run([]string{"run"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("run bare ambiguous = %d, want 1", code)
	}
	errStr := stderr.String()
	if !strings.Contains(errStr, "multiple resumable runs found") ||
		!strings.Contains(errStr, "run-a") ||
		!strings.Contains(errStr, "run-b") {
		t.Fatalf("ambiguity refusal missing candidate runs: %s", errStr)
	}
}

// A1: Bare sworn run ambiguity refuses with candidate releases named.
func TestBareRunRefusesAmbiguousReleases(t *testing.T) {
	root, _ := projectRepositoryFixture(t)
	installProjectPlan(t, root, "alpha")
	installProjectPlan(t, root, "beta")

	stateDir := filepath.Join(root, ".sworn")
	manifestDir := filepath.Join(stateDir, "runs")
	if err := os.MkdirAll(manifestDir, 0o700); err != nil {
		t.Fatal(err)
	}
	projectWriteManifest(t, manifestDir, "alpha.json", root, "alpha", "run-alpha")
	projectWriteManifest(t, manifestDir, "beta.json", root, "beta", "run-beta")

	wd, _ := os.Getwd()
	_ = os.Chdir(root)
	defer func() { _ = os.Chdir(wd) }()

	var stdout, stderr bytes.Buffer
	code := run([]string{"run"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("run bare ambiguous releases = %d, want 1", code)
	}
	errStr := stderr.String()
	if !strings.Contains(errStr, "multiple approved releases found") ||
		!strings.Contains(errStr, "alpha") ||
		!strings.Contains(errStr, "beta") {
		t.Fatalf("ambiguity refusal missing candidate releases: %s", errStr)
	}
}

// A1 + Captain Correction 2: Resume refusal when single run has no manifest.
func TestBareRunRefusesResumeWhenManifestMissing(t *testing.T) {
	root, _ := projectRepositoryFixture(t)
	installProjectPlan(t, root, "delivery")

	stateDir := filepath.Join(root, ".sworn")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	journalPath := filepath.Join(stateDir, "sworn.db")
	store, err := journal.Open(context.Background(), journalPath)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 123).UTC()
	if err := store.RegisterRun(context.Background(), projectJournalRun(
		"run-orphan", root, "delivery", now,
	)); err != nil {
		t.Fatal(err)
	}
	_ = store.Close()

	wd, _ := os.Getwd()
	_ = os.Chdir(root)
	defer func() { _ = os.Chdir(wd) }()

	var stdout, stderr bytes.Buffer
	code := run([]string{"run"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("run bare missing manifest = %d, want 1", code)
	}
	errStr := stderr.String()
	if !strings.Contains(errStr, "cannot resume run run-orphan") ||
		!strings.Contains(errStr, "delivery") ||
		!strings.Contains(errStr, filepath.Join(root, ".sworn", "runs")) {
		t.Fatalf("resume refusal missing required details: %s", errStr)
	}
}

// A1 + Captain Correction 1: Unapproved manifest-only release (BATON_UNAVAILABLE) is refused, never started.
func TestBareRunRefusesUnapprovedManifestOnlyRelease(t *testing.T) {
	root, _ := projectRepositoryFixture(t)
	// Do NOT install plan or create release-wt ref.
	stateDir := filepath.Join(root, ".sworn")
	manifestDir := filepath.Join(stateDir, "runs")
	if err := os.MkdirAll(manifestDir, 0o700); err != nil {
		t.Fatal(err)
	}
	projectWriteManifest(t, manifestDir, "unapproved.json", root, "unapproved", "run-unapproved")

	wd, _ := os.Getwd()
	_ = os.Chdir(root)
	defer func() { _ = os.Chdir(wd) }()

	var stdout, stderr bytes.Buffer
	code := run([]string{"run"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("run bare unapproved = %d, want 1", code)
	}
	errStr := stderr.String()
	if !strings.Contains(errStr, "BATON_UNAVAILABLE") ||
		!strings.Contains(errStr, "unapproved") {
		t.Fatalf("unapproved release refusal missing BATON_UNAVAILABLE: %s", errStr)
	}
}

// A1: Empty project refuses with searched paths named.
func TestBareRunRefusesEmptyProjectNamingSearchedPaths(t *testing.T) {
	root, _ := projectRepositoryFixture(t)

	wd, _ := os.Getwd()
	_ = os.Chdir(root)
	defer func() { _ = os.Chdir(wd) }()

	var stdout, stderr bytes.Buffer
	code := run([]string{"run"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("run bare empty = %d, want 1", code)
	}
	errStr := stderr.String()
	if !strings.Contains(errStr, "no runs or releases found") ||
		!strings.Contains(errStr, root) ||
		!strings.Contains(errStr, "refs/heads/release-wt/*") {
		t.Fatalf("empty project refusal missing searched paths: %s", errStr)
	}
}

// A2: Precedence table fixture for sworn run <release>.
func TestRunReleasePrecedenceTable(t *testing.T) {
	root, _ := projectRepositoryFixture(t)
	installProjectPlan(t, root, "featured")

	stateDir := filepath.Join(root, ".sworn")
	manifestDir := filepath.Join(stateDir, "runs")
	if err := os.MkdirAll(manifestDir, 0o700); err != nil {
		t.Fatal(err)
	}
	projectWriteManifest(t, manifestDir, "featured.json", root, "featured", "run-project-manifest")

	// Custom external files for precedence testing
	externalDir := t.TempDir()
	customManifest := filepath.Join(externalDir, "custom-manifest.json")
	projectWriteManifest(t, externalDir, "custom-manifest.json", root, "featured", "run-custom-manifest")
	customJournal := filepath.Join(externalDir, "custom.db")
	customDrivers := filepath.Join(externalDir, "custom-drivers.json")
	if err := os.WriteFile(customDrivers, []byte("{\"schema_version\":\"sworn.driver-config/v1\",\"profiles\":{}}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	wd, _ := os.Getwd()
	_ = os.Chdir(root)
	defer func() { _ = os.Chdir(wd) }()

	// Test 1: Release name only -> resolves project manifest and project journal
	var stdout1, stderr1 bytes.Buffer
	if code := run([]string{"run", "featured"}, &stdout1, &stderr1); code != 0 {
		t.Fatalf("run featured = %d, stderr = %q", code, stderr1.String())
	}
	if !strings.Contains(stdout1.String(), "run-project-manifest") {
		t.Fatalf("expected project manifest run ID: %s", stdout1.String())
	}

	// Test 2: Explicit --manifest overrides project manifest
	var stdout2, stderr2 bytes.Buffer
	if code := run([]string{"run", "featured", "--manifest", customManifest}, &stdout2, &stderr2); code != 0 {
		t.Fatalf("run with explicit --manifest = %d, stderr = %q", code, stderr2.String())
	}
	if !strings.Contains(stdout2.String(), "run-custom-manifest") {
		t.Fatalf("expected custom manifest run ID: %s", stdout2.String())
	}

	// Test 3: Explicit --journal overrides project journal
	var stdout3, stderr3 bytes.Buffer
	if code := run([]string{"run", "featured", "--journal", customJournal}, &stdout3, &stderr3); code != 0 {
		t.Fatalf("run with explicit --journal = %d, stderr = %q", code, stderr3.String())
	}
	if !strings.Contains(stdout3.String(), "run-project-manifest") {
		t.Fatalf("expected run started in custom journal: %s", stdout3.String())
	}

	// Test 4: Unknown release refuses with named search paths and discovered releases
	var stdout4, stderr4 bytes.Buffer
	if code := run([]string{"run", "nonexistent"}, &stdout4, &stderr4); code != 1 {
		t.Fatalf("run nonexistent = %d, want 1", code)
	}
	if !strings.Contains(stderr4.String(), `release "nonexistent" not found`) ||
		!strings.Contains(stderr4.String(), "refs/heads/release-wt/*") ||
		!strings.Contains(stderr4.String(), "featured") {
		t.Fatalf("unknown release refusal missing names: %s", stderr4.String())
	}

	// Test 5: Release without manifest refuses naming searched directory
	installProjectPlan(t, root, "no-manifest-rel")
	var stdout5, stderr5 bytes.Buffer
	if code := run([]string{"run", "no-manifest-rel"}, &stdout5, &stderr5); code != 1 {
		t.Fatalf("run no-manifest-rel = %d, want 1", code)
	}
	if !strings.Contains(stderr5.String(), `no manifest found for release "no-manifest-rel"`) ||
		!strings.Contains(stderr5.String(), manifestDir) {
		t.Fatalf("missing manifest refusal missing names: %s", stderr5.String())
	}

	// Test 6: Release with BATON_UNAVAILABLE refuses
	projectWriteManifest(t, manifestDir, "unapproved-rel.json", root, "unapproved-rel", "run-unapproved")
	var stdout6, stderr6 bytes.Buffer
	if code := run([]string{"run", "unapproved-rel"}, &stdout6, &stderr6); code != 1 {
		t.Fatalf("run unapproved-rel = %d, want 1", code)
	}
	if !strings.Contains(stderr6.String(), "BATON_UNAVAILABLE") ||
		!strings.Contains(stderr6.String(), `release "unapproved-rel" is not recorded and approved`) {
		t.Fatalf("unapproved release refusal missing details: %s", stderr6.String())
	}
}

// A4: Resolver locates .sworn/operator.json by convention (present, absent, unreadable cases).
func TestDiscoverOperatorConfigConventionsAndRefusals(t *testing.T) {
	root, _ := projectRepositoryFixture(t)
	stateDir := filepath.Join(root, ".sworn")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	operatorPath := filepath.Join(stateDir, "operator.json")

	// Case 1: Absent operator.json -> surfaced as empty string without diagnostics
	catalog1, err := discoverProject(context.Background(), root, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if catalog1.operatorConfig != "" {
		t.Fatalf("absent operatorConfig = %q, want empty", catalog1.operatorConfig)
	}
	if len(catalog1.diagnostics) != 0 {
		t.Fatalf("absent operator.json produced diagnostics: %#v", catalog1.diagnostics)
	}

	// Case 2: Present and valid 0600 operator.json -> surfaced as clean path without diagnostics
	validConfig := `{
  "schema_version": "sworn.operator-config/v1",
  "local": {
    "listen": "127.0.0.1:7337"
  }
}`
	if err := os.WriteFile(operatorPath, []byte(validConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	catalog2, err := discoverProject(context.Background(), root, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if catalog2.operatorConfig != operatorPath {
		t.Fatalf("valid operatorConfig = %q, want %q", catalog2.operatorConfig, operatorPath)
	}
	if len(catalog2.diagnostics) != 0 {
		t.Fatalf("valid operator.json produced diagnostics: %#v", catalog2.diagnostics)
	}

	// Case 3: Present but unreadable permissions (0644) -> named diagnostic OPERATOR_CONFIG_UNAVAILABLE
	if err := os.Chmod(operatorPath, 0o644); err != nil {
		t.Fatal(err)
	}
	catalog3, err := discoverProject(context.Background(), root, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if catalog3.operatorConfig != "" {
		t.Fatalf("unreadable operatorConfig = %q, want empty", catalog3.operatorConfig)
	}
	if !reflect.DeepEqual(catalog3.diagnostics, []string{"OPERATOR_CONFIG_UNAVAILABLE"}) {
		t.Fatalf("0644 operator.json diagnostics = %#v, want OPERATOR_CONFIG_UNAVAILABLE", catalog3.diagnostics)
	}

	// Case 4: Present but invalid JSON with 0600 permissions -> named diagnostic OPERATOR_CONFIG_UNAVAILABLE
	if err := os.WriteFile(operatorPath, []byte("not valid json"), 0o600); err != nil {
		t.Fatal(err)
	}
	catalog4, err := discoverProject(context.Background(), root, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if catalog4.operatorConfig != "" {
		t.Fatalf("invalid json operatorConfig = %q, want empty", catalog4.operatorConfig)
	}
	if !reflect.DeepEqual(catalog4.diagnostics, []string{"OPERATOR_CONFIG_UNAVAILABLE"}) {
		t.Fatalf("invalid json operator.json diagnostics = %#v, want OPERATOR_CONFIG_UNAVAILABLE", catalog4.diagnostics)
	}

	// Case 5: Directory named operator.json -> named diagnostic OPERATOR_CONFIG_UNAVAILABLE
	_ = os.Remove(operatorPath)
	if err := os.Mkdir(operatorPath, 0o700); err != nil {
		t.Fatal(err)
	}
	catalog5, err := discoverProject(context.Background(), root, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if catalog5.operatorConfig != "" {
		t.Fatalf("directory operatorConfig = %q, want empty", catalog5.operatorConfig)
	}
	if !reflect.DeepEqual(catalog5.diagnostics, []string{"OPERATOR_CONFIG_UNAVAILABLE"}) {
		t.Fatalf("directory operator.json diagnostics = %#v, want OPERATOR_CONFIG_UNAVAILABLE", catalog5.diagnostics)
	}
}
