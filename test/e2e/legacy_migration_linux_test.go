//go:build linux

package e2e

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/driver"
	swornruntime "github.com/swornagent/sworn/internal/runtime"
)

// A3. A repository whose delivery history was written in the Baton era must
// keep working under Sworn's own owners: no Baton product installed, no
// history rewritten, and a new release whose authority and provenance are
// Sworn's.
//
// The legacy history here is real, not a fixture of convenience: a completed
// baton.plan/v2 release with its inline slice bodies and its four
// Baton-Receipt commits, written through the same production action API that
// produced such history originally. The new run then happens over that exact
// repository with a home directory that contains nothing and a PATH that
// contains only Git, so no Baton executable can participate even if one
// existed on this machine.

const (
	legacyRelease     = "legacy-baton-era-release"
	legacySwornRunID  = "e2e-legacy-resume"
	legacySwornRelase = "e2e-legacy-resume-release"
)

func legacySlicePaths() map[string]string {
	return map[string]string{"L1": "legacy/after.txt"}
}

// seedLegacyBatonEraRelease installs a completed baton.plan/v2 release and
// returns its plan digest plus the OID of every object reachable when it was
// finished.
func seedLegacyBatonEraRelease(t *testing.T, repository string) (baton.Plan, []string) {
	t.Helper()
	planBytes, plan := e2ePlan(t, legacyRelease, repository)
	if plan.Metadata().SchemaVersion != baton.PlanVersion {
		t.Fatalf("legacy plan schema = %q, want the Baton-era fence %q",
			plan.Metadata().SchemaVersion, baton.PlanVersion)
	}
	// installApprovedPlan + installAndPassComponent write exactly the record
	// shape the Baton era produced: an inline baton.plan/v2 plan revision and
	// Baton-Receipt commits for design, proceed, candidate and pass.
	installAndPassComponent(t, repository, legacyRelease, planBytes)
	return plan, reachableCommits(t, repository)
}

// reachableCommits lists every commit reachable from any ref, sorted.
func reachableCommits(t *testing.T, repository string) []string {
	t.Helper()
	raw := runGit(t, repository, "rev-list", "--all")
	var commits []string
	for _, line := range strings.Split(raw, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			commits = append(commits, line)
		}
	}
	sort.Strings(commits)
	return commits
}

// legacyResumePlanBytes is the new release's authority: a Sworn-native
// manifest with its own approval reference, over the same repository.
func legacyResumePlanBytes(t *testing.T, digest string) []byte {
	t.Helper()
	value := map[string]any{
		"schema_version": baton.ManifestVersion,
		"release":        legacySwornRelase,
		"revision":       int64(1),
		"previous_plan":  nil,
		"repository":     "acme-repo",
		"target_ref":     "refs/heads/main",
		"approval_ref":   "operator://" + legacySwornRelase + "/1",
		"tracks": []any{map[string]any{
			"id": "T1", "depends_on": []any{},
			"slices": []any{manifestTouchpointSliceEntry(
				"L1", "contracts/L1.json", digest,
				[]string{legacySlicePaths()["L1"]},
			)},
		}},
	}
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return []byte(
		"```sworn-release-manifest-v1\n" + string(body) +
			"\n```\n\nSworn-native continuation over Baton-era history.\n" +
			"Owned surface read from the repository: " +
			journeyRepositoryCanary + ".\n",
	)
}

// TestRealBinaryLegacyBatonHistoryResumesUnderSwornAuthority is A3.
func TestRealBinaryLegacyBatonHistoryResumesUnderSwornAuthority(t *testing.T) {
	t.Parallel()
	repository := newProductRepository(t)
	legacyPlan, before := seedLegacyBatonEraRelease(t, repository)

	// The legacy release is real and complete before Sworn's new run begins.
	legacyBefore := readBatonState(t, repository, legacyRelease)
	if legacyBefore.Plan.Digest != legacyPlan.Digest() ||
		legacyBefore.Plan.Metadata.SchemaVersion != baton.PlanVersion {
		t.Fatalf("seeded legacy authority = %#v", legacyBefore.Plan)
	}
	legacySlice, ok := legacyBefore.Slice("S2")
	if !ok || legacySlice.Outcome != "pass" || legacySlice.Pass == nil {
		t.Fatalf("seeded legacy slice = %#v", legacySlice)
	}
	legacyPassOID := legacySlice.Pass.OID
	legacyReceiptCount := len(legacySlice.History.Entries)

	// The new release's contract lives in its own committed file.
	contractRaw := manifestTouchpointContractRaw(
		t, "L1", []string{legacySlicePaths()["L1"]},
	)
	_, contractDigest, err := baton.ParseSliceContract(contractRaw, "L1", "T1")
	if err != nil {
		t.Fatal(err)
	}
	contractPath := filepath.Join(repository, "contracts")
	if err := os.MkdirAll(contractPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(contractPath, "L1.json"), contractRaw, 0o644,
	); err != nil {
		t.Fatal(err)
	}
	runGit(t, repository, "add", "--", "contracts/L1.json")
	runGit(t, repository, "commit", "--quiet", "-m", "commit continuation contract")

	planBytes := legacyResumePlanBytes(t, contractDigest)
	plan, err := baton.ParsePlan(planBytes)
	if err != nil {
		t.Fatal(err)
	}

	provider := &journeyProvider{
		t: t, planBytes: planBytes,
		slicePaths: legacySlicePaths(),
		turns:      make(map[string]int),
		families:   make(map[string]driver.ProfileFamily),
		models:     make(map[string]string),
		access:     make(map[string]driver.WorkspaceAccess),
	}
	providerHTTP := httptest.NewServer(http.HandlerFunc(provider.serve))
	defer providerHTTP.Close()
	configBody, loaded := productionJourneyConfig(t, providerHTTP.URL)
	root := t.TempDir()
	configPath := filepath.Join(root, "drivers.json")
	if err := os.WriteFile(configPath, configBody, 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := swornruntime.Manifest{
		GitIdentity:   e2eEngineIdentity(),
		SchemaVersion: swornruntime.ManifestVersion,
		RunID:         legacySwornRunID,
		Repository:    repository,
		Release:       legacySwornRelase,
		TargetRef:     "refs/heads/main",
		Intent:        "Continue this project under Sworn without touching its Baton-era history.",

		MaxParallelTracks: 1,
		Authority: swornruntime.ProjectAuthority{
			Project: "acme-repo", ExternalAuthorizer: "operator",
		},
		DriverConfigDigest: loaded.ConfigurationDigest(),
		Roles: driver.RoleSelections{
			Planner:     driver.RoleSelection{Profile: "openai", Model: "journey-planner"},
			Implementer: driver.RoleSelection{Profile: "gemini", Model: "journey-implementer"},
			Captain:     driver.RoleSelection{Profile: "openai", Model: "journey-captain"},
			Verifier:    driver.RoleSelection{Profile: "gemini", Model: "journey-verifier"},
		},
		Automation: &swornruntime.AutomationSelections{
			Recovery: driver.RoleSelection{Profile: "openai", Model: "journey-planner"},
		},
		Limits: driver.Limits{TimeoutMillis: 30_000, OutputBytes: 65_536},
	}
	manifestBody, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestBody = append(manifestBody, '\n')
	manifestPath := writeManifest(t, root, manifestBody)
	journalPath := filepath.Join(root, "run.sqlite")
	binary := filepath.Join(root, "sworn")
	buildBinary(t, binary, "./cmd/sworn", "")

	// Offline and Baton-free: an empty home, and a PATH holding only Git.
	pathDir := filepath.Join(root, "path")
	if err := os.MkdirAll(pathDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(e2eGit, filepath.Join(pathDir, "git")); err != nil {
		t.Fatal(err)
	}
	emptyHome := filepath.Join(root, "home")
	if err := os.MkdirAll(emptyHome, 0o755); err != nil {
		t.Fatal(err)
	}
	environment := map[string]string{
		"HOME":                     emptyHome,
		"PATH":                     pathDir,
		"SWORN_JOURNEY_OPENAI_KEY": journeyOpenAISecret,
		"SWORN_JOURNEY_GEMINI_KEY": journeyGeminiSecret,
	}
	entries, err := os.ReadDir(pathDir)
	if err != nil || len(entries) != 1 || entries[0].Name() != "git" {
		t.Fatalf("the run's PATH is not Baton-free: %v (%v)", entries, err)
	}

	targetBefore := runGit(t, repository, "rev-parse", "main")
	stdout, stderr := runBinaryWithEnvironmentTimeout(
		t, binary, 0, environment, 600*time.Second,
		"run", "--manifest", manifestPath, "--journal", journalPath,
		"--config", configPath,
	)
	if stderr != "" || !strings.Contains(stdout, "  state: parked") {
		t.Fatalf("legacy resume start stdout=%q stderr=%q", stdout, stderr)
	}
	attention := openPlannerSummaryAttention(t, binary, legacySwornRunID, journalPath)
	stdout, stderr = runBinaryWithEnvironmentTimeout(
		t, binary, 0, environment, 600*time.Second,
		"answer", "--run", legacySwornRunID, "--journal", journalPath,
		"--attention", attention.ID, "--generation", "1",
		"--answer", journeySummaryAnswer, "--config", configPath,
	)
	if stderr != "" || !strings.Contains(stdout, "  state: awaiting_approval") {
		t.Fatalf("legacy resume answer stdout=%q stderr=%q", stdout, stderr)
	}
	approveThroughRealBinary(
		t, binary, legacySwornRunID, journalPath, configPath, environment,
	)
	stdout, stderr = runBinaryWithEnvironmentTimeout(
		t, binary, 0, environment, 600*time.Second,
		"resume", "--run", legacySwornRunID, "--journal", journalPath,
		"--command", "legacy-resume-1", "--generation", "0",
		"--config", configPath,
	)
	if stderr != "" || !strings.Contains(stdout, "  state: complete") {
		counts, _ := kernelEffectCounts(t, journalPath, legacySwornRunID)
		t.Fatalf("legacy resume stdout=%q stderr=%q counts=%#v", stdout, stderr, counts)
	}

	// 1. History was not rewritten: every commit that existed before the run
	//    is still reachable, unchanged.
	after := reachableCommits(t, repository)
	present := make(map[string]bool, len(after))
	for _, commit := range after {
		present[commit] = true
	}
	for _, commit := range before {
		if !present[commit] {
			t.Fatalf("the Sworn run removed pre-existing commit %s", commit)
		}
	}
	if len(after) <= len(before) {
		t.Fatalf("the run added no history: before=%d after=%d", len(before), len(after))
	}

	// 2. The Baton-era records still read, with their original identities,
	//    through the same reader -- no migration, no rewrite.
	legacyAfter := readBatonState(t, repository, legacyRelease)
	if legacyAfter.Plan.Digest != legacyPlan.Digest() ||
		legacyAfter.Plan.Metadata.SchemaVersion != baton.PlanVersion ||
		legacyAfter.Plan.Metadata.Release != legacyRelease {
		t.Fatalf("legacy authority changed: %#v", legacyAfter.Plan)
	}
	legacySliceAfter, ok := legacyAfter.Slice("S2")
	if !ok || legacySliceAfter.Outcome != "pass" ||
		legacySliceAfter.Pass == nil ||
		legacySliceAfter.Pass.OID != legacyPassOID ||
		len(legacySliceAfter.History.Entries) != legacyReceiptCount {
		t.Fatalf("legacy slice record changed: %#v", legacySliceAfter)
	}

	// 3. The new authority is Sworn's own, native, and separately contracted.
	state := readBatonState(t, repository, legacySwornRelase)
	if state.Plan.Digest != plan.Digest() ||
		state.Plan.Metadata.SchemaVersion != baton.ManifestVersion ||
		state.Plan.Metadata.ApprovalRef != "operator://"+legacySwornRelase+"/1" ||
		state.Plan.Approval.Receipt.Role != "planner" ||
		state.Plan.Approval.Receipt.Result != "approved" {
		t.Fatalf("new Sworn authority = %#v", state.Plan)
	}
	if state.Assembly.Outcome != "merged" || state.Assembly.ResultCommit == "" {
		t.Fatalf("new release assembly = %#v", state.Assembly)
	}
	if runGit(t, repository, "rev-parse", "main") == targetBefore {
		t.Fatal("the new release merged nothing")
	}
	if got := runGit(t, repository, "show", "main:"+legacySlicePaths()["L1"]); got !=
		"L1 production journey" {
		t.Fatalf("continuation product = %q", got)
	}

	// 4. Truthful provenance: both releases coexist on the same target, each
	//    reporting its own era, and neither claims the other's identity.
	if legacyAfter.Plan.Metadata.Release == state.Plan.Metadata.Release {
		t.Fatal("the new release overwrote the legacy release identity")
	}
	// Both eras remain readable side by side through the product's own reader,
	// each still declaring the schema it was written in.
	if legacyAfter.Plan.Metadata.SchemaVersion ==
		state.Plan.Metadata.SchemaVersion {
		t.Fatalf(
			"legacy and Sworn-native authorities report the same schema %q",
			state.Plan.Metadata.SchemaVersion,
		)
	}

	// 5. No operator surface tells the person to install or restore Baton.
	board, boardErr := runBinaryWithEnvironment(
		t, binary, 0, environment,
		"board", "--run", legacySwornRunID, "--journal", journalPath,
	)
	status, statusErr := runBinaryWithEnvironment(
		t, binary, 0, environment,
		"status", "--run", legacySwornRunID, "--journal", journalPath, "--json",
	)
	if boardErr != "" || statusErr != "" {
		t.Fatalf("operator surfaces stderr board=%q status=%q", boardErr, statusErr)
	}
	for _, surface := range []string{board, status} {
		lowered := strings.ToLower(surface)
		for _, forbidden := range []string{
			"install baton", "restore baton", "baton install",
			"reinstall baton", "npm i -g baton",
		} {
			if strings.Contains(lowered, forbidden) {
				t.Fatalf("an operator surface asks for Baton: %q in %q", forbidden, surface)
			}
		}
	}
}
