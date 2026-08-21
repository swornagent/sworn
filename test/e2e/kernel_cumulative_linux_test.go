//go:build linux

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/journal"
	swornruntime "github.com/swornagent/sworn/internal/runtime"
)

// A1(ii). One real run must compose the exact products of all three preceding
// slices at once:
//
//	S1  the authority the run installs and the receipts it appends are
//	    Sworn's own, with no external Baton product involved;
//	S2  the plan is a sworn.release-manifest/v1 manifest whose slice contracts
//	    are separate committed files, admitted by digest;
//	S3  the Planner reads the repository, presents a summary as a human-only
//	    turn, and emits plan bytes only on the turn resumed from the answer.
//
// and carry that composition through implementation, a fresh Verifier PASS and
// deterministic Merge. The authority is installed by the run itself, through
// `sworn approve` and the runtime's own install effect -- never by a test-side
// action call -- because that install path is the one a real delivery uses and
// the one that has to resolve the separate contract files.

const (
	cumulativeRunID   = "e2e-cumulative-t1-kernel"
	cumulativeRelease = "e2e-cumulative-t1-kernel-release"
)

// cumulativeSlicePaths is the product path each cumulative slice owns.
func cumulativeSlicePaths() map[string]string {
	return map[string]string{
		"K1": "kernel/one.txt",
		"K2": "kernel/two.txt",
	}
}

// cumulativeContracts writes both slice contracts into the repository as
// ordinary product files and commits them, exactly as a real delivery does
// before a manifest names them, and returns the committed tree plus each
// contract's admitted digest.
func cumulativeContracts(t *testing.T, repository string) (string, map[string]string) {
	t.Helper()
	digests := make(map[string]string, 2)
	var paths []string
	for _, slice := range []string{"K1", "K2"} {
		raw := manifestTouchpointContractRaw(
			t, slice, []string{cumulativeSlicePaths()[slice]},
		)
		_, digest, err := baton.ParseSliceContract(raw, slice, "T1")
		if err != nil {
			t.Fatal(err)
		}
		digests[slice] = digest
		relative := "contracts/" + slice + ".json"
		absolute := filepath.Join(repository, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absolute, raw, 0o644); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, relative)
	}
	runGit(t, repository, append([]string{"add", "--"}, paths...)...)
	runGit(t, repository, "commit", "--quiet", "-m", "commit slice contracts")
	return runGit(t, repository, "rev-parse", "main"), digests
}

// cumulativePlanBytes is the native Sworn manifest the journey Planner
// promises: one track, two dependency-ordered slices, each contract in its own
// committed file. It carries the repository-only fact in its prose, so a
// Planner that never read the repository could not have produced it.
func cumulativePlanBytes(t *testing.T, digests map[string]string) []byte {
	t.Helper()
	value := map[string]any{
		"schema_version": baton.ManifestVersion,
		"release":        cumulativeRelease,
		"revision":       int64(1),
		"previous_plan":  nil,
		"repository":     "acme-repo",
		"target_ref":     "refs/heads/main",
		"approval_ref":   "operator://" + cumulativeRelease + "/1",
		"tracks": []any{map[string]any{
			"id": "T1", "depends_on": []any{},
			"slices": []any{
				manifestTouchpointSliceEntry(
					"K1", "contracts/K1.json", digests["K1"],
					[]string{cumulativeSlicePaths()["K1"]},
				),
				manifestTouchpointSliceEntry(
					"K2", "contracts/K2.json", digests["K2"],
					[]string{cumulativeSlicePaths()["K2"]},
				),
			},
		}},
	}
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return []byte(
		"```sworn-release-manifest-v1\n" + string(body) +
			"\n```\n\nCumulative T1 kernel journey.\n" +
			"Owned surface read from the repository: " +
			journeyRepositoryCanary + ".\n",
	)
}

// cumulativeRunManifest is the production run manifest: a real configured HTTP
// driver, no scripts, so every role decision crosses the real dispatcher and
// the Planner's human-only summary rule applies.
func cumulativeRunManifest(
	t *testing.T, repository string, config driver.LoadedDriverConfig,
) []byte {
	t.Helper()
	manifest := swornruntime.Manifest{
		GitIdentity: gitx.Identity{
			Name: "E2E Engine", Email: "engine@example.test",
		},
		SchemaVersion:     swornruntime.ManifestVersion,
		RunID:             cumulativeRunID,
		Repository:        repository,
		Release:           cumulativeRelease,
		TargetRef:         "refs/heads/main",
		Intent:            "Compose the exact S1, S2 and S3 products in one run.",
		MaxParallelTracks: 1,
		Authority: swornruntime.ProjectAuthority{
			Project: "acme-repo", ExternalAuthorizer: "operator",
		},
		DriverConfigDigest: config.ConfigurationDigest(),
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
	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	body = append(body, '\n')
	if _, err := swornruntime.ParseManifest(body); err != nil {
		t.Fatal(err)
	}
	return body
}

// approveThroughRealBinary takes the approval the run itself is offering and
// admits it through the real `sworn approve` verb. Nothing here invents an
// approval: every field comes from the offer the running product published.
func approveThroughRealBinary(
	t *testing.T, binary, runID, journalPath, configPath string,
	environment map[string]string,
) swornruntime.ApprovalCommand {
	t.Helper()
	statusBody, stderr := runBinary(
		t, binary, 0, "status", "--run", runID, "--journal", journalPath, "--json",
	)
	var status swornruntime.RunStatus
	if stderr != "" || json.Unmarshal([]byte(statusBody), &status) != nil ||
		status.ApprovalOffer == nil {
		t.Fatalf("approval offer body=%q stderr=%q", statusBody, stderr)
	}
	offer := status.ApprovalOffer.Command
	absent := func(value string) string {
		if value == "" {
			return "absent"
		}
		return value
	}
	arguments := []string{
		"approve", "--journal", journalPath,
		"--run", offer.RunID,
		"--manifest-digest", offer.ManifestDigest,
		"--project", offer.Project,
		"--release", offer.Release,
		"--release-ref", offer.ReleaseRef,
		"--release-head", absent(offer.ReleaseHead),
		"--proposal-replay-key", offer.ProposalReplayKey,
		"--plan-revision", fmt.Sprintf("%d", offer.PlanRevision),
		"--prior-plan", absent(offer.PriorPlan),
		"--plan-digest", offer.PlanDigest,
		"--target-ref", offer.TargetRef,
		"--target-head", offer.TargetHead,
		"--decision-class", offer.DecisionClass,
		"--decision", offer.Decision,
		"--actor-class", offer.ActorClass,
		"--actor-authority", offer.ActorAuthority,
		"--config", configPath,
	}
	stdout, stderr := runBinaryWithEnvironmentTimeout(
		t, binary, 0, environment, 600*time.Second, arguments...,
	)
	if stderr != "" || stdout == "" {
		t.Fatalf("sworn approve stdout=%q stderr=%q", stdout, stderr)
	}
	return offer
}

// kernelEffectCounts counts succeeded effects and recorded events by kind.
func kernelEffectCounts(
	t *testing.T, journalPath, runID string,
) (map[string]int, journal.Snapshot) {
	t.Helper()
	store, err := journal.OpenReadOnly(context.Background(), journalPath)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(context.Background(), runID)
	_ = store.Close()
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, command := range snapshot.Commands {
		counts["command:"+command.Kind]++
	}
	for _, effect := range snapshot.Effects {
		if effect.State == journal.Succeeded {
			counts["effect:"+effect.Kind]++
		}
	}
	for _, event := range snapshot.Events {
		counts["event:"+event.Kind]++
	}
	return counts, snapshot
}

// TestRealBinaryCumulativeT1KernelJourney is A1(ii).
func TestRealBinaryCumulativeT1KernelJourney(t *testing.T) {
	t.Parallel()
	repository := newProductRepository(t)
	contractTree, digests := cumulativeContracts(t, repository)
	planBytes := cumulativePlanBytes(t, digests)
	plan, err := baton.ParsePlan(planBytes)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Metadata().SchemaVersion != baton.ManifestVersion {
		t.Fatalf("cumulative plan schema = %q", plan.Metadata().SchemaVersion)
	}

	provider := &journeyProvider{
		t: t, planBytes: planBytes,
		slicePaths: cumulativeSlicePaths(),
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
	manifestPath := writeManifest(t, root, cumulativeRunManifest(t, repository, loaded))
	journalPath := filepath.Join(root, "run.sqlite")
	binary := filepath.Join(root, "sworn")
	buildBinary(t, binary, "./cmd/sworn", "")
	environment := map[string]string{
		"SWORN_JOURNEY_OPENAI_KEY": journeyOpenAISecret,
		"SWORN_JOURNEY_GEMINI_KEY": journeyGeminiSecret,
	}

	targetBefore := runGit(t, repository, "rev-parse", "main")
	if targetBefore != contractTree {
		t.Fatalf("contracts are not at the target head: %s vs %s", contractTree, targetBefore)
	}

	// This journey can only succeed because the runtime installer now hands
	// admission the authorized target head to read the separate contract files
	// from. Prove that gate is load-bearing rather than free: the identical
	// plan bytes with no contract source are refused, and the refusal leaves no
	// Git trace, so the completion below is evidence of the repair and not of a
	// permissive admission.
	openedRepository, err := gitx.Open(repository, e2eGit)
	if err != nil {
		t.Fatal(err)
	}
	actions, err := baton.NewActions(
		baton.UseGitRepository(openedRepository), inertResolver,
		gitx.Identity{Name: "E2E Engine", Email: "engine@example.test"},
	)
	if err != nil {
		t.Fatal(err)
	}
	_, refused := actions.RecordPlanRevision(baton.RecordPlanRevisionInput{
		PlanBytes: planBytes,
		Summary:   "Attempt a native manifest with no contract source.",
		Detail:    []byte("Falsifier for the runtime contract-source repair."),
	})
	if baton.ErrorCode(refused) != "CONTRACT_SOURCE_REQUIRED" {
		t.Fatalf(
			"install without a contract source = %q (%v); "+
				"this journey would not prove the repair",
			baton.ErrorCode(refused), refused,
		)
	}
	if runGit(t, repository, "rev-parse", "main") != targetBefore {
		t.Fatal("refused admission moved the target ref")
	}

	// S3's product: the run parks on the Planner's human-only summary turn and
	// no plan exists yet.
	stdout, stderr := runBinaryWithEnvironment(
		t, binary, 0, environment,
		"run", "--manifest", manifestPath, "--journal", journalPath,
		"--config", configPath,
	)
	if stderr != "" || !strings.Contains(stdout, "  state: parked") ||
		runGit(t, repository, "rev-parse", "main") != targetBefore {
		t.Fatalf("cumulative start stdout=%q stderr=%q", stdout, stderr)
	}
	attention := openPlannerSummaryAttention(t, binary, cumulativeRunID, journalPath)
	stdout, stderr = runBinaryWithEnvironmentTimeout(
		t, binary, 0, environment, 600*time.Second,
		"answer", "--run", cumulativeRunID, "--journal", journalPath,
		"--attention", attention.ID, "--generation", "1",
		"--answer", journeySummaryAnswer, "--config", configPath,
	)
	if stderr != "" {
		t.Fatalf("cumulative summary answer stderr=%q", stderr)
	}
	stdout, stderr = runBinaryWithEnvironmentTimeout(
		t, binary, 0, environment, 600*time.Second,
		"run", "--manifest", manifestPath, "--journal", journalPath,
		"--config", configPath,
	)
	if stderr != "" || !strings.Contains(stdout, "  state: awaiting_approval") ||
		runGit(t, repository, "rev-parse", "main") != targetBefore {
		t.Fatalf("cumulative summary answer stdout=%q stderr=%q", stdout, stderr)
	}
	provider.mu.Lock()
	factReads := provider.plannerFactReads
	provider.mu.Unlock()
	if factReads != 1 || strings.Contains(journeySummaryAnswer, journeyRepositoryCanary) {
		t.Fatalf("planner repository reads = %d", factReads)
	}

	// S1 and S2's products: the run installs its own authority, and that
	// authority is a native manifest whose contracts are separate committed
	// files. This is the load-bearing step -- the installer has to resolve
	// contracts/K1.json and contracts/K2.json out of the authorized target
	// head, and admission fails closed if it cannot.
	offer := approveThroughRealBinary(
		t, binary, cumulativeRunID, journalPath, configPath, environment,
	)
	if offer.PlanDigest != plan.Digest() || offer.TargetHead != targetBefore {
		t.Fatalf("approval offer = %#v", offer)
	}

	stdout, stderr = runBinaryWithEnvironmentTimeout(
		t, binary, 0, environment, 600*time.Second,
		"resume", "--run", cumulativeRunID, "--journal", journalPath,
		"--command", "cumulative-resume-1", "--generation", "0",
		"--config", configPath,
	)
	if stderr != "" {
		t.Fatalf("cumulative resume stderr=%q", stderr)
	}
	stdout, stderr = runBinaryWithEnvironmentTimeout(
		t, binary, 0, environment, 600*time.Second,
		"run", "--manifest", manifestPath, "--journal", journalPath,
		"--config", configPath,
	)
	if stderr != "" || !strings.Contains(stdout, "  state: complete") {
		counts, _ := kernelEffectCounts(t, journalPath, cumulativeRunID)
		t.Fatalf(
			"cumulative resume stdout=%q stderr=%q counts=%#v", stdout, stderr, counts,
		)
	}

	counts, snapshot := kernelEffectCounts(t, journalPath, cumulativeRunID)
	if counts["effect:approval.admit"] != 1 || counts["effect:baton.install"] != 1 ||
		counts["effect:baton.merge"] != 1 {
		t.Fatalf("cumulative authority effects = %#v", counts)
	}
	for _, effect := range snapshot.Effects {
		if effect.State == journal.Uncertain ||
			effect.State == journal.OperationalFailed {
			t.Fatalf("cumulative effect %s = %s/%s", effect.Kind, effect.State, effect.ErrorCode)
		}
	}

	// The authority the run installed is the exact native manifest, and every
	// slice contract it admitted is the separate committed file, by digest.
	state := readBatonState(t, repository, cumulativeRelease)
	if state.Plan.Digest != plan.Digest() ||
		state.Plan.Metadata.SchemaVersion != baton.ManifestVersion ||
		state.Plan.Approval.Receipt.Role != "planner" ||
		state.Plan.Approval.Receipt.Result != "approved" {
		t.Fatalf("installed cumulative authority = %#v", state.Plan)
	}
	installed := state.Plan.Metadata
	if len(installed.Tracks) != 1 || len(installed.Tracks[0].Slices) != 2 {
		t.Fatalf("installed topology = %#v", installed.Tracks)
	}
	if len(state.Plan.History) == 0 {
		t.Fatal("installed plan has no history")
	}
	installedPlan := state.Plan.History[len(state.Plan.History)-1].Plan
	for _, slice := range installed.Tracks[0].Slices {
		want, present := digests[slice.ID]
		if !present {
			t.Fatalf("installed unknown slice %q", slice.ID)
		}
		if slice.ContractPath != "contracts/"+slice.ID+".json" {
			t.Fatalf("slice %s was installed without its separate contract file: %#v",
				slice.ID, slice)
		}
		digest, found := installedPlan.Contract(slice.ID)
		if !found || digest != want {
			t.Fatalf("slice %s installed contract digest = %q/%v, want %q",
				slice.ID, digest, found, want)
		}
		// The digest names the real committed bytes, not a restated fixture.
		committed := runGit(
			t, repository, "show", contractTree+":"+slice.ContractPath,
		)
		_, recomputed, err := baton.ParseSliceContract(
			[]byte(committed+"\n"), slice.ID, "T1",
		)
		if err != nil || recomputed != want {
			t.Fatalf("committed contract for %s = %q (err=%v)", slice.ID, recomputed, err)
		}
	}

	// Deterministic merge of exactly the passed composition.
	if state.Assembly.Outcome != "merged" || state.Assembly.Candidate == nil ||
		state.Assembly.Pass == nil || state.Assembly.ResultCommit == "" {
		t.Fatalf("cumulative assembly = %#v", state.Assembly)
	}
	target := runGit(t, repository, "rev-parse", "main")
	assemblyCandidate := *state.Assembly.Candidate.Receipt.Candidate
	if target != state.Assembly.ResultCommit ||
		runGit(t, repository, "rev-parse", target+"^{tree}") !=
			runGit(t, repository, "rev-parse", assemblyCandidate+"^{tree}") {
		t.Fatalf("cumulative target=%s result=%s candidate=%s",
			target, state.Assembly.ResultCommit, assemblyCandidate)
	}
	for slice, pathValue := range cumulativeSlicePaths() {
		want := slice + " production journey"
		if got := runGit(t, repository, "show", "main:"+pathValue); got != want {
			t.Fatalf("%s product = %q, want %q", slice, got, want)
		}
	}
	// Every slice reached PASS through a fresh Verifier, and the contracts the
	// run started from are still exactly where the manifest said they were.
	for _, slice := range []string{"K1", "K2"} {
		record, ok := state.Slice(slice)
		if !ok || record.Outcome != "pass" || record.Pass == nil ||
			record.Pass.Receipt.Role != "verifier" ||
			record.Pass.Receipt.Candidate == nil ||
			record.Candidate == nil ||
			*record.Pass.Receipt.Candidate != *record.Candidate.Receipt.Candidate {
			t.Fatalf("slice %s outcome=%q pass=%#v", slice, record.Outcome, record.Pass)
		}
	}
	for _, slice := range []string{"K1", "K2"} {
		if runGit(t, repository, "show", "main:contracts/"+slice+".json") !=
			runGit(t, repository, "show", contractTree+":contracts/"+slice+".json") {
			t.Fatalf("merge rewrote the %s contract file", slice)
		}
	}
}

// e2eEngineIdentity is the Git identity every e2e run records with.
func e2eEngineIdentity() gitx.Identity {
	return gitx.Identity{Name: "E2E Engine", Email: "engine@example.test"}
}
