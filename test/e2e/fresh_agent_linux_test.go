//go:build linux

package e2e

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/cockpit"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/journal"
	swornruntime "github.com/swornagent/sworn/internal/runtime"
)

// A2. A fresh agent, in a repository Sworn has never touched, must be able to
// enter through the installed Sworn skill and the local MCP service, take a
// human goal, discover repository facts for itself, ask a person only for the
// meaning it cannot discover, and drive one whole delivery -- separate slice
// artifacts, exact authorization, implementation, a fresh read-only Verifier
// PASS, and a deterministic merge of exactly the covered candidate.
//
// Two things make this a proof rather than a demonstration.
//
// First, the entry is the installed skill's own literal text, evaluated
// against the repository exactly as it stands at that moment. This journey
// never runs `sworn init` on its own initiative: it runs it only because the
// installed step 1 it just read told it to. If the skill went back to saying
// "this does not apply" for an uninitialized Git worktree, the journey stops
// there and fails, which is precisely the entry defect this slice repaired.
//
// Second, every state change after entry goes through an MCP tool the agent
// discovered from tools/list. The journey records every real-binary
// invocation it makes and asserts that none of them is a mutating CLI verb, so
// "it was done over MCP" is checked, not asserted.

const (
	freshAgentRunID   = "e2e-fresh-agent"
	freshAgentRelease = "e2e-fresh-agent-release"
	// freshAgentGoal is the human goal this run exists to serve. It is
	// recorded in the manifest the agent starts, so the run's own identity is
	// bound to it.
	freshAgentGoal = "Give the kernel repository a durable two-slice greeting surface."
)

func freshAgentSlicePaths() map[string]string {
	return map[string]string{
		"F1": "greeting/one.txt",
		"F2": "greeting/two.txt",
	}
}

// freshAgent is the coding agent: one real Sworn binary, one recorded
// invocation log, and nothing else.
type freshAgent struct {
	t           *testing.T
	binary      string
	invocations [][]string
}

// cli runs the real binary and records exactly what was asked of it.
func (a *freshAgent) cli(
	wantExit int, environment map[string]string, args ...string,
) (string, string) {
	a.t.Helper()
	a.invocations = append(a.invocations, append([]string(nil), args...))
	return runBinaryWithEnvironmentTimeout(
		a.t, a.binary, wantExit, environment, 600*time.Second, args...,
	)
}

// assertOnlyEntryVerbsWereUsed proves the delivery itself never went through
// the command line. Only three verbs may appear: installing the skill,
// initializing the project because the skill said to, and starting the local
// MCP service the skill routes to.
func (a *freshAgent) assertOnlyEntryVerbsWereUsed() {
	a.t.Helper()
	allowed := map[string]bool{"skill": true, "init": true, "serve": true}
	for _, invocation := range a.invocations {
		if len(invocation) == 0 || !allowed[invocation[0]] {
			a.t.Fatalf(
				"the delivery used a command-line verb: %v (all invocations: %v)",
				invocation, a.invocations,
			)
		}
	}
}

// freshAgentEntry is what the agent learns from the installed skill, and the
// only reason it is allowed to do anything next.
type freshAgentEntry struct {
	installedPath string
	applies       bool
	initialize    bool
	routesToMCP   bool
}

// enterCleanRepository installs the Sworn skill with the real binary, reads
// the installed text back, and evaluates its literal first step against this
// repository as it stands right now.
func (a *freshAgent) enterCleanRepository(
	home, repository string,
) freshAgentEntry {
	a.t.Helper()
	// The agent host this journey models is Claude Code, so its skill root
	// exists before Sworn is asked to install into it.
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		a.t.Fatal(err)
	}
	stdout, stderr := a.cli(0, nil, "skill", "install", "--home", home)
	if stderr != "" || !strings.Contains(stdout, "installed ") {
		a.t.Fatalf("skill install stdout=%q stderr=%q", stdout, stderr)
	}
	var installed string
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		_, path, found := strings.Cut(line, "sworn skill install: installed ")
		if found {
			installed = strings.TrimSpace(path)
		}
	}
	if installed == "" || !strings.HasPrefix(installed, home) {
		a.t.Fatalf("installed skill path = %q", installed)
	}
	body, err := os.ReadFile(installed)
	if err != nil {
		a.t.Fatal(err)
	}
	text := string(body)
	step := skillStepOne(a.t, text)

	entry := freshAgentEntry{installedPath: installed}
	present := func(marker string) bool {
		if !strings.Contains(step, "`"+marker+"`") {
			return false
		}
		_, statErr := os.Stat(filepath.Join(repository, filepath.FromSlash(marker)))
		return statErr == nil
	}
	if !strings.Contains(step, "`.git`") {
		a.t.Fatalf("installed step 1 does not recognize a Git worktree:\n%s", step)
	}
	if !present(".git") {
		return entry
	}
	entry.applies = true
	entry.initialize = !present(".baton/releases") && !present(".sworn")
	if entry.initialize && !strings.Contains(step, "`sworn init`") {
		a.t.Fatalf(
			"installed step 1 leaves an uninitialized Git worktree with no way in:\n%s",
			step,
		)
	}
	for _, sentence := range strings.Split(step, ". ") {
		if strings.Contains(sentence, "does not apply") &&
			!strings.Contains(sentence, "outside a Git worktree") {
			a.t.Fatalf("installed step 1 refuses a Git worktree:\n%s", step)
		}
	}
	entry.routesToMCP = strings.Contains(text, "MCP") &&
		strings.Contains(text, "`sworn serve`")
	return entry
}

// freshAgentPlanBytes is the native manifest the Planner promises for this
// goal: separate committed contract files, one per slice.
func freshAgentPlanBytes(t *testing.T, digests map[string]string) []byte {
	t.Helper()
	value := map[string]any{
		"schema_version": baton.ManifestVersion,
		"release":        freshAgentRelease,
		"revision":       int64(1),
		"previous_plan":  nil,
		"repository":     "acme-repo",
		"target_ref":     "refs/heads/main",
		"approval_ref":   "operator://" + freshAgentRelease + "/1",
		"tracks": []any{map[string]any{
			"id": "T1", "depends_on": []any{},
			"slices": []any{
				manifestTouchpointSliceEntry("F1", "contracts/F1.json",
					digests["F1"], []string{freshAgentSlicePaths()["F1"]}),
				manifestTouchpointSliceEntry("F2", "contracts/F2.json",
					digests["F2"], []string{freshAgentSlicePaths()["F2"]}),
			},
		}},
	}
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return []byte(
		"```sworn-release-manifest-v1\n" + string(body) +
			"\n```\n\nGoal: " + freshAgentGoal +
			"\nOwned surface read from the repository: " +
			journeyRepositoryCanary + ".\n",
	)
}

// freshAgentContracts commits one contract file per slice, as ordinary product
// content, before any plan names them.
func freshAgentContracts(t *testing.T, repository string) (string, map[string]string) {
	t.Helper()
	digests := make(map[string]string, 2)
	var paths []string
	for _, slice := range []string{"F1", "F2"} {
		raw := manifestTouchpointContractRaw(
			t, slice, []string{freshAgentSlicePaths()[slice]},
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

// mcpResult decodes one MCP tool result envelope.
type mcpResult struct {
	IsError    bool
	Structured json.RawMessage
	Raw        []byte
}

func freshAgentCall(
	t *testing.T, address, tool string, arguments map[string]any,
) mcpResult {
	t.Helper()
	body := journeyMCPCall(t, address, tool, arguments)
	var envelope struct {
		Result struct {
			IsError           bool            `json:"isError"`
			StructuredContent json.RawMessage `json:"structuredContent"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("%s response = %s", tool, body)
	}
	return mcpResult{
		IsError:    envelope.Result.IsError,
		Structured: envelope.Result.StructuredContent,
		Raw:        body,
	}
}

func freshAgentSnapshot(t *testing.T, address string) cockpit.Snapshot {
	t.Helper()
	result := freshAgentCall(t, address, "sworn_status", map[string]any{
		"run_id": freshAgentRunID,
	})
	if result.IsError {
		t.Fatalf("sworn_status = %s", result.Raw)
	}
	var snapshot cockpit.Snapshot
	if err := json.Unmarshal(result.Structured, &snapshot); err != nil {
		t.Fatalf("sworn_status content = %s", result.Structured)
	}
	return snapshot
}

// TestRealBinaryFreshAgentSkillToMCPDelivery is A2.
func TestRealBinaryFreshAgentSkillToMCPDelivery(t *testing.T) {
	t.Parallel()
	repository := newProductRepository(t)
	// The repository is genuinely untouched by Sworn.
	for _, marker := range []string{".baton", ".sworn"} {
		if _, err := os.Stat(filepath.Join(repository, marker)); err == nil {
			t.Fatalf("the fresh repository already contains %s", marker)
		}
	}
	contractTree, digests := freshAgentContracts(t, repository)
	planBytes := freshAgentPlanBytes(t, digests)
	plan, err := baton.ParsePlan(planBytes)
	if err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	agent := &freshAgent{t: t, binary: filepath.Join(root, "sworn")}
	buildBinary(t, agent.binary, "./cmd/sworn", "")

	// 1. Entry: the installed skill, read literally, against this repository.
	home := t.TempDir()
	entry := agent.enterCleanRepository(home, repository)
	if !entry.applies || !entry.routesToMCP {
		t.Fatalf("installed skill entry for a clean repository = %#v", entry)
	}
	if !entry.initialize {
		t.Fatal("a repository with no Sworn markers was not recognized as uninitialized")
	}

	// 2. The agent runs `sworn init` only because the installed step 1 said
	//    to. PATH is pinned to Git alone so no agent CLI can be auto-detected
	//    here; the connection file this journey uses is its own, exactly as an
	//    agent that already knows its provider would supply. What init must
	//    deliver is the initialized project, and that is what is asserted.
	pathDir := filepath.Join(root, "path")
	if err := os.MkdirAll(pathDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(e2eGit, filepath.Join(pathDir, "git")); err != nil {
		t.Fatal(err)
	}
	initEnvironment := map[string]string{"PATH": pathDir}
	stdout, _ := agent.cli(1, initEnvironment, "init", "--project", repository)
	if !strings.Contains(stdout, "created ") {
		t.Fatalf("sworn init stdout=%q", stdout)
	}
	projectDir := filepath.Join(repository, ".sworn")
	if info, err := os.Stat(projectDir); err != nil || !info.IsDir() {
		t.Fatalf("sworn init did not initialize the project: %v", err)
	}
	// Re-reading the same installed step now recognizes the project, so the
	// entry rule and the initialization actually agree.
	if after := agent.enterCleanRepository(home, repository); !after.applies ||
		after.initialize {
		t.Fatalf("installed skill entry after init = %#v", after)
	}

	// 3. The agent brings its own configured provider and records the human
	//    goal in the manifest the run will be bound to.
	provider := &journeyProvider{
		t: t, planBytes: planBytes,
		slicePaths: freshAgentSlicePaths(),
		turns:      make(map[string]int),
		families:   make(map[string]driver.ProfileFamily),
		models:     make(map[string]string),
		access:     make(map[string]driver.WorkspaceAccess),
	}
	providerHTTP := httptest.NewServer(http.HandlerFunc(provider.serve))
	defer providerHTTP.Close()
	configBody, loaded := productionJourneyConfig(t, providerHTTP.URL)
	configPath := filepath.Join(projectDir, "drivers.json")
	if err := os.WriteFile(configPath, configBody, 0o600); err != nil {
		t.Fatal(err)
	}
	manifestBody := freshAgentRunManifest(t, repository, loaded)
	if !bytes.Contains(manifestBody, []byte(freshAgentGoal)) {
		t.Fatal("the run manifest does not carry the human goal")
	}
	manifestDigestBytes := sha256.Sum256(manifestBody)
	manifestDigest := "sha256:" + hex.EncodeToString(manifestDigestBytes[:])
	manifestPath := filepath.Join(projectDir, "manifest.json")
	if err := os.WriteFile(manifestPath, manifestBody, 0o600); err != nil {
		t.Fatal(err)
	}
	journalPath := filepath.Join(projectDir, "run.sqlite")
	address := telemetryParityAddress(t)
	operatorConfigPath := telemetryParityOperatorConfig(t, root, address, "")

	// 4. The skill routes to the local MCP service, so the agent starts it.
	environment := map[string]string{
		"SWORN_JOURNEY_OPENAI_KEY": journeyOpenAISecret,
		"SWORN_JOURNEY_GEMINI_KEY": journeyGeminiSecret,
	}
	agent.invocations = append(agent.invocations, []string{"serve"})
	serve := exec.Command(
		agent.binary, "serve", "--run", freshAgentRunID,
		"--journal", journalPath, "--manifest", manifestPath,
		"--operator-config", operatorConfigPath, "--config", configPath,
	)
	serve.Env = cleanEnvironment(environment)
	var serveOut, serveErr bytes.Buffer
	serve.Stdout, serve.Stderr = &serveOut, &serveErr
	if err := serve.Start(); err != nil {
		t.Fatal(err)
	}
	serveDone := make(chan error, 1)
	go func() { serveDone <- serve.Wait() }()
	stopped := false
	defer func() {
		if serve.Process != nil && !stopped {
			_ = serve.Process.Signal(syscall.SIGTERM)
			select {
			case <-serveDone:
			case <-time.After(15 * time.Second):
				_ = serve.Process.Kill()
			}
		}
	}()
	telemetryParityWaitHealth(
		t, address, func(cockpit.TelemetryHealth) bool { return true },
	)

	// 5. The agent knows only what the service advertises.
	advertised := journeyMCPTools(t, address)
	for _, required := range []string{
		"sworn_start", "sworn_status", "sworn_attentions",
		"sworn_answer_attention", "sworn_approve", "sworn_control",
	} {
		if !advertised[required] {
			t.Fatalf("advertised MCP tools = %#v", advertised)
		}
	}

	targetBefore := runGit(t, repository, "rev-parse", "main")
	started := freshAgentCall(t, address, "sworn_start", map[string]any{
		"manifest_digest": manifestDigest,
	})
	if started.IsError || !bytes.Contains(started.Raw, []byte(`"state":"parked"`)) {
		t.Fatalf("sworn_start = %s", started.Raw)
	}

	// 6. The only thing asked of a person is the meaning the repository cannot
	//    supply: the Planner's summary confirmation.
	attentionID, found := journeyMCPOpenHumanTurn(t, address)
	if !found {
		t.Fatal("the run asked a person for nothing")
	}
	snapshot := freshAgentSnapshot(t, address)
	var question string
	for _, attention := range snapshot.Runtime.Attentions {
		if attention.ID == attentionID {
			question = attention.Question
		}
	}
	if question == "" || strings.Contains(question, journeyRepositoryCanary) {
		t.Fatalf("the run asked a person for a repository-discoverable fact: %q", question)
	}
	answered := freshAgentCall(t, address, "sworn_answer_attention", map[string]any{
		"run_id": freshAgentRunID, "attention_id": attentionID,
		"expected_generation": 1, "answer": journeySummaryAnswer,
	})
	if answered.IsError {
		t.Fatalf("sworn_answer_attention = %s", answered.Raw)
	}
	if _, stillOpen := journeyMCPOpenHumanTurn(t, address); stillOpen {
		t.Fatal("a second human turn was opened for the same delivery")
	}
	provider.mu.Lock()
	factReads := provider.plannerFactReads
	provider.mu.Unlock()
	if factReads != 1 {
		t.Fatalf("planner repository reads = %d", factReads)
	}

	// 7. Exact approval: every field comes from the offer the run published,
	//    and nothing has moved until it is admitted.
	snapshot = freshAgentSnapshot(t, address)
	if snapshot.Run.ManifestDigest != manifestDigest {
		t.Fatalf("run manifest identity = %q, want the goal manifest %q",
			snapshot.Run.ManifestDigest, manifestDigest)
	}
	if snapshot.ApprovalOffer == nil {
		t.Fatalf("no approval offer: %#v", snapshot.Run)
	}
	offer := snapshot.ApprovalOffer.Command
	if offer.PlanDigest != plan.Digest() || offer.TargetHead != targetBefore ||
		runGit(t, repository, "rev-parse", "main") != targetBefore {
		t.Fatalf("approval offer = %#v", offer)
	}
	approved := freshAgentCall(
		t, address, "sworn_approve", approvalArguments(t, offer),
	)
	if approved.IsError {
		t.Fatalf("sworn_approve = %s", approved.Raw)
	}

	// 8. Carry the run to its end over MCP.
	deadline := time.Now().Add(10 * time.Minute)
	resumes := 0
	for {
		snapshot = freshAgentSnapshot(t, address)
		if snapshot.Run.State == "complete" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("run did not complete; last state = %q", snapshot.Run.State)
		}
		if snapshot.Run.State == "running" {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		resumes++
		if resumes > 4 {
			t.Fatalf("run stalled in %q after %d resumes",
				snapshot.Run.State, resumes)
		}
		control := freshAgentCall(t, address, "sworn_control", map[string]any{
			"run_id": freshAgentRunID,
			"command_id": "fresh-agent-resume-" +
				strings.TrimPrefix(snapshot.Run.State, "awaiting_"),
			"kind":                string(journal.Resume),
			"expected_generation": snapshot.Run.ControlGeneration,
		})
		if control.IsError {
			t.Fatalf("sworn_control resume = %s", control.Raw)
		}
	}

	stopped = true
	_ = serve.Process.Signal(syscall.SIGTERM)
	select {
	case <-serveDone:
	case <-time.After(20 * time.Second):
		_ = serve.Process.Kill()
		t.Fatal("sworn serve did not stop")
	}
	if serveOut.String() != "sworn serve: ready\n" || serveErr.Len() != 0 {
		t.Fatalf("serve stdout=%q stderr=%q", serveOut.String(), serveErr.String())
	}

	// 9. The delivery never touched a mutating command-line verb.
	agent.assertOnlyEntryVerbsWereUsed()

	// 10. Separate slice artifacts, Sworn-owned authority, one fresh
	//     read-only Verifier PASS per slice, and a merge of exactly that.
	state := readBatonState(t, repository, freshAgentRelease)
	if state.Plan.Digest != plan.Digest() ||
		state.Plan.Metadata.SchemaVersion != baton.ManifestVersion ||
		state.Plan.Approval.Receipt.Role != "planner" ||
		state.Plan.Approval.Receipt.Result != "approved" {
		t.Fatalf("installed authority = %#v", state.Plan)
	}
	installedPlan := state.Plan.History[len(state.Plan.History)-1].Plan
	for slice, want := range digests {
		_, declared, ok := installedPlan.FindSlice(slice)
		if !ok || declared.ContractPath != "contracts/"+slice+".json" {
			t.Fatalf("slice %s has no separate contract artifact: %#v", slice, declared)
		}
		digest, present := installedPlan.Contract(slice)
		if !present || digest != want {
			t.Fatalf("slice %s contract digest = %q, want %q", slice, digest, want)
		}
		if runGit(t, repository, "show", contractTree+":"+declared.ContractPath) == "" {
			t.Fatalf("slice %s contract file is not committed content", slice)
		}
	}

	verifierInvocations := 0
	provider.mu.Lock()
	for invocation, access := range provider.access {
		if !strings.Contains(invocation, "/work_verification/") {
			continue
		}
		verifierInvocations++
		if access != driver.ReadOnly {
			provider.mu.Unlock()
			t.Fatalf("verifier %s ran with access %s", invocation, access)
		}
	}
	provider.mu.Unlock()
	if verifierInvocations != len(digests) {
		t.Fatalf("work verifications = %d, want %d", verifierInvocations, len(digests))
	}

	if state.Assembly.Outcome != "merged" || state.Assembly.Candidate == nil ||
		state.Assembly.Pass == nil || state.Assembly.ResultCommit == "" {
		t.Fatalf("assembly = %#v", state.Assembly)
	}
	target := runGit(t, repository, "rev-parse", "main")
	assemblyCandidate := *state.Assembly.Candidate.Receipt.Candidate
	if target != state.Assembly.ResultCommit ||
		runGit(t, repository, "rev-parse", target+"^{tree}") !=
			runGit(t, repository, "rev-parse", assemblyCandidate+"^{tree}") {
		t.Fatalf("merged target=%s result=%s candidate=%s",
			target, state.Assembly.ResultCommit, assemblyCandidate)
	}
	// Only the covered candidate reached the target: the *product* difference
	// between the pre-run target and the merged one is exactly the paths the
	// approved plan's slices declared. The release's own record root is not
	// product and is excluded, exactly as every product identity excludes it.
	var changed []string
	for _, line := range strings.Split(
		runGit(t, repository, "diff", "--name-only", targetBefore, "main"), "\n",
	) {
		line = strings.TrimSpace(line)
		if line == "" || line == baton.RecordRoot ||
			strings.HasPrefix(line, baton.RecordRoot+"/") {
			continue
		}
		changed = append(changed, line)
	}
	sort.Strings(changed)
	var wanted []string
	for _, pathValue := range freshAgentSlicePaths() {
		wanted = append(wanted, pathValue)
	}
	sort.Strings(wanted)
	if strings.Join(changed, ",") != strings.Join(wanted, ",") {
		t.Fatalf("merge changed %v, want exactly %v", changed, wanted)
	}
	for slice, pathValue := range freshAgentSlicePaths() {
		want := slice + " production journey"
		if got := runGit(t, repository, "show", "main:"+pathValue); got != want {
			t.Fatalf("%s product = %q, want %q", slice, got, want)
		}
		record, ok := state.Slice(slice)
		if !ok || record.Outcome != "pass" || record.Pass == nil ||
			record.Pass.Receipt.Role != "verifier" {
			t.Fatalf("slice %s did not reach a fresh Verifier PASS: %#v", slice, record)
		}
	}

	// The delivery is Sworn's own: nothing instructed the operator to install
	// or restore an external Baton product, and no Baton package was needed.
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills")); err != nil {
		t.Fatalf("installed skill home = %v", err)
	}
	body, err := os.ReadFile(entry.installedPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"install baton", "npm i -g baton", "baton install"} {
		if strings.Contains(strings.ToLower(string(body)), forbidden) {
			t.Fatalf("the installed skill still routes through an external Baton product: %q", forbidden)
		}
	}
}

// freshAgentRunManifest records the human goal as the run's intent and pins
// the agent's own configured provider.
func freshAgentRunManifest(
	t *testing.T, repository string, config driver.LoadedDriverConfig,
) []byte {
	t.Helper()
	manifest := swornruntime.Manifest{
		GitIdentity:   e2eEngineIdentity(),
		SchemaVersion: swornruntime.ManifestVersion,
		RunID:         freshAgentRunID,
		Repository:    repository,
		Release:       freshAgentRelease,
		TargetRef:     "refs/heads/main",
		Intent:        freshAgentGoal,

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
