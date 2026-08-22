//go:build linux

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/cockpit"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/journal"
	swornruntime "github.com/swornagent/sworn/internal/runtime"
	"github.com/swornagent/sworn/internal/skill"
)

const (
	surfaceQuestionCanary = "SURFACE-PARITY-QUESTION-canary-3b91"
	surfaceAnswerCanary   = "SURFACE-PARITY-ANSWER-canary-approved-value"
)

// surfaceRun is one real Sworn run, started with the real binary against a
// real repository and a real HTTP provider, parked on the Planner's human-only
// summary turn. Every conformance surface below drives this same run shape, so
// a behavioral difference between surfaces is a real product difference and
// not a fixture difference.
type surfaceRun struct {
	t            *testing.T
	runID        string
	release      string
	repository   string
	root         string
	binary       string
	configPath   string
	manifestPath string
	journalPath  string
	environment  map[string]string
	provider     *recoveryE2EProvider
	planBytes    []byte
	plan         baton.Plan
}

func newSurfaceRun(t *testing.T, binary, runID string) *surfaceRun {
	t.Helper()
	repository := newProductRepository(t)
	planBytes, plan := recoveryE2EPlan(t)
	provider := &recoveryE2EProvider{
		t: t, planBytes: planBytes, recover: true, human: true,
		question: surfaceQuestionCanary, answer: surfaceAnswerCanary,
		turns: make(map[string]int),
	}
	server := httptest.NewServer(http.HandlerFunc(provider.serve))
	t.Cleanup(server.Close)

	root := t.TempDir()
	configBody, loaded := recoveryE2EConfig(t, server.URL)
	configPath := filepath.Join(root, "drivers.json")
	if err := os.WriteFile(configPath, configBody, 0o600); err != nil {
		t.Fatal(err)
	}
	manifestPath := writeManifest(
		t, root, recoveryE2EManifest(t, runID, repository, loaded),
	)
	return &surfaceRun{
		t: t, runID: runID, release: "turn-recovery-release",
		repository: repository, root: root, binary: binary,
		configPath: configPath, manifestPath: manifestPath,
		journalPath: filepath.Join(root, "run.sqlite"),
		environment: map[string]string{
			"SWORN_TURN_RECOVERY_KEY": recoveryE2ESecret,
		},
		provider: provider, planBytes: planBytes, plan: plan,
	}
}

// startParked drives the real binary to the one moment every surface can be
// compared at: an open human-only Implementer turn on a run whose plan
// authority is already recorded, so the delivery board offers live controls.
func (r *surfaceRun) startParked() {
	r.t.Helper()
	stdout, stderr := runBinaryWithEnvironment(
		r.t, r.binary, 0, r.environment,
		"run",
		"--manifest", r.manifestPath,
		"--journal", r.journalPath,
		"--config", r.configPath,
	)
	if stderr != "" || !strings.Contains(stdout, "  state: parked") {
		r.t.Fatalf("surface run start stdout=%q stderr=%q", stdout, stderr)
	}
	stdout = answerRecoveryPlannerSummary(
		r.t, r.binary, r.runID, r.journalPath, r.configPath, r.environment,
	)
	if !strings.Contains(stdout, "  state: awaiting_approval") {
		r.t.Fatalf("surface summary answer stdout=%q", stdout)
	}
	authorizePlan(r.t, r.journalPath, r.runID, r.plan)
	installApprovedPlan(r.t, r.repository, r.planBytes)
	stdout, stderr = runBinaryWithEnvironmentTimeout(
		r.t, r.binary, 0, r.environment, 180*time.Second,
		"resume", "--run", r.runID, "--journal", r.journalPath,
		"--command", "surface-resume-1", "--generation", "0",
		"--config", r.configPath,
	)
	if stderr != "" {
		r.t.Fatalf("surface resume stderr=%q", stderr)
	}
	stdout, stderr = runBinaryWithEnvironmentTimeout(
		r.t, r.binary, 0, r.environment, 180*time.Second,
		"run",
		"--manifest", r.manifestPath,
		"--journal", r.journalPath,
		"--config", r.configPath,
	)
	if stderr != "" || !strings.Contains(stdout, "  state: parked") {
		r.t.Fatalf("surface human park stdout=%q stderr=%q", stdout, stderr)
	}
}

// board reads the exact cockpit projection through the CLI surface.
func (r *surfaceRun) board() cockpit.Snapshot {
	r.t.Helper()
	stdout, stderr := runBinary(
		r.t, r.binary, 0,
		"board", "--run", r.runID, "--journal", r.journalPath, "--json",
	)
	var snapshot cockpit.Snapshot
	if stderr != "" || json.Unmarshal([]byte(stdout), &snapshot) != nil {
		r.t.Fatalf("cli board stdout=%q stderr=%q", stdout, stderr)
	}
	return snapshot
}

func (r *surfaceRun) status() swornruntime.RunStatus {
	r.t.Helper()
	stdout, stderr := runBinary(
		r.t, r.binary, 0,
		"status", "--run", r.runID, "--journal", r.journalPath, "--json",
	)
	var status swornruntime.RunStatus
	if stderr != "" || json.Unmarshal([]byte(stdout), &status) != nil {
		r.t.Fatalf("cli status stdout=%q stderr=%q", stdout, stderr)
	}
	return status
}

// openHumanTurn returns the one open human-only turn as the CLI surface sees
// it, failing when the run is not parked on exactly one.
func (r *surfaceRun) openHumanTurn() cockpit.AttentionView {
	r.t.Helper()
	snapshot := r.board()
	var open []cockpit.AttentionView
	for _, attention := range snapshot.Runtime.Attentions {
		if attention.State == "open" && attention.HumanTurn != nil {
			open = append(open, attention)
		}
	}
	if len(open) != 1 || open[0].Question != surfaceQuestionCanary ||
		open[0].HumanTurn.Responsibility !=
			string(driver.ImplementerImplementation) {
		r.t.Fatalf("open human turns = %#v", snapshot.Runtime.Attentions)
	}
	return open[0]
}

// unofferedApproval builds a syntactically complete approval command for a
// plan revision this run has not proposed. Every surface must refuse it.
func (r *surfaceRun) unofferedApproval() swornruntime.ApprovalCommand {
	r.t.Helper()
	return swornruntime.ApprovalCommand{
		SchemaVersion:     swornruntime.ApprovalCommandVersion,
		RunID:             r.runID,
		ManifestDigest:    fileDigest(r.t, r.manifestPath),
		Project:           "acme-repo",
		Release:           r.release,
		ReleaseRef:        "refs/heads/release-wt/" + r.release,
		ProposalReplayKey: "surface-parity-unoffered",
		PlanRevision:      1,
		PlanDigest:        r.plan.Digest(),
		TargetRef:         "refs/heads/main",
		TargetHead:        runGit(r.t, r.repository, "rev-parse", "main"),
		DecisionClass:     "plan_revision",
		Decision:          "approve",
		ActorClass:        "human",
		ActorAuthority:    "operator",
	}
}

// admittedAnswers counts the answers this run actually admitted. Every run in
// this file crosses exactly two human-only turns -- the Planner's summary
// boundary and the Implementer turn the surface phases act on -- so an
// admitted-once answer leaves this at two and a second admission would leave
// it at three.
func (r *surfaceRun) admittedAnswers() int {
	r.t.Helper()
	store, err := journal.OpenReadOnly(context.Background(), r.journalPath)
	if err != nil {
		r.t.Fatal(err)
	}
	defer store.Close()
	snapshot, err := store.Snapshot(context.Background(), r.runID)
	if err != nil {
		r.t.Fatal(err)
	}
	answers := 0
	for _, event := range snapshot.Events {
		if event.Kind == journal.AttentionAnsweredEvent {
			answers++
		}
	}
	return answers
}

// serveProcess is one real `sworn serve` MCP endpoint over this run.
type serveProcess struct {
	t       *testing.T
	command *exec.Cmd
	address string
	stdout  bytes.Buffer
	stderr  bytes.Buffer
	done    chan error
}

func (r *surfaceRun) startServe() *serveProcess {
	r.t.Helper()
	address := telemetryParityAddress(r.t)
	operatorConfig := telemetryParityOperatorConfig(r.t, r.root, address, "")
	process := &serveProcess{t: r.t, address: address, done: make(chan error, 1)}
	process.command = exec.Command(
		r.binary, "serve",
		"--run", r.runID,
		"--journal", r.journalPath,
		"--manifest", r.manifestPath,
		"--operator-config", operatorConfig,
		"--config", r.configPath,
	)
	process.command.Env = cleanEnvironment(r.environment)
	process.command.Stdout, process.command.Stderr = &process.stdout, &process.stderr
	if err := process.command.Start(); err != nil {
		r.t.Fatal(err)
	}
	go func() { process.done <- process.command.Wait() }()
	r.t.Cleanup(func() { process.stop(false) })
	telemetryParityWaitHealth(
		r.t, address, func(cockpit.TelemetryHealth) bool { return true },
	)
	return process
}

func (p *serveProcess) stop(strict bool) {
	if p.command.Process == nil {
		return
	}
	_ = p.command.Process.Signal(syscall.SIGTERM)
	var runErr error
	select {
	case runErr = <-p.done:
	case <-time.After(15 * time.Second):
		_ = p.command.Process.Kill()
		<-p.done
		p.t.Fatal("sworn serve did not stop")
	}
	p.command.Process = nil
	if !strict {
		return
	}
	exit := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		if !errors.As(runErr, &exitErr) {
			p.t.Fatalf("wait sworn serve: %v", runErr)
		}
		exit = exitErr.ExitCode()
	}
	if exit != 0 || p.stdout.String() != "sworn serve: ready\n" ||
		p.stderr.Len() != 0 {
		p.t.Fatalf(
			"serve exit=%d stdout=%q stderr=%q",
			exit, p.stdout.String(), p.stderr.String(),
		)
	}
}

// mcpSnapshot reads the projection through the MCP surface. A detached drive
// writing the journal makes the projection transiently unstable, so the
// honest SNAPSHOT_UNSTABLE refusal is retried rather than fatal.
func mcpSnapshot(t *testing.T, address, runID string) cockpit.Snapshot {
	t.Helper()
	var body []byte
	deadline := time.Now().Add(30 * time.Second)
	for {
		body = journeyMCPCall(t, address, "sworn_status", map[string]any{
			"run_id": runID,
		})
		var envelope struct {
			Result struct {
				IsError           bool             `json:"isError"`
				StructuredContent cockpit.Snapshot `json:"structuredContent"`
			} `json:"result"`
		}
		if json.Unmarshal(body, &envelope) == nil && !envelope.Result.IsError {
			return envelope.Result.StructuredContent
		}
		if (!bytes.Contains(body, []byte("SNAPSHOT_UNSTABLE")) &&
			!bytes.Contains(body, []byte("RUNTIME_UNAVAILABLE"))) ||
			time.Now().After(deadline) {
			t.Fatalf("sworn_status = %s", body)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// mcpAttentions reads the open human-only turns through the MCP surface,
// retrying the transient SNAPSHOT_UNSTABLE refusal a detached drive causes.
func mcpAttentions(t *testing.T, address string) []cockpit.AttentionView {
	t.Helper()
	var body []byte
	var envelope struct {
		Result struct {
			IsError           bool `json:"isError"`
			StructuredContent struct {
				Attentions []cockpit.AttentionView `json:"attentions"`
			} `json:"structuredContent"`
		} `json:"result"`
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		body = journeyMCPCall(t, address, "sworn_attentions", map[string]any{})
		envelope.Result.IsError = false
		envelope.Result.StructuredContent.Attentions = nil
		if json.Unmarshal(body, &envelope) == nil && !envelope.Result.IsError {
			break
		}
		if (!bytes.Contains(body, []byte("SNAPSHOT_UNSTABLE")) &&
			!bytes.Contains(body, []byte("RUNTIME_UNAVAILABLE"))) ||
			time.Now().After(deadline) {
			t.Fatalf("sworn_attentions = %s", body)
		}
		time.Sleep(100 * time.Millisecond)
	}
	var open []cockpit.AttentionView
	for _, attention := range envelope.Result.StructuredContent.Attentions {
		if attention.State == "open" && attention.HumanTurn != nil {
			open = append(open, attention)
		}
	}
	return open
}

// skillEntry is what an agent learns by reading the installed Sworn skill:
// nothing more. Every installed_skill_mcp anchor below routes through this,
// so if the installed skill stopped naming the local MCP service, or stopped
// recognizing the repository, the anchor cannot be produced at all.
type skillEntry struct {
	installedPath string
	applies       bool
	initialize    bool
	transport     string
}

func enterThroughInstalledSkill(
	t *testing.T, homeDir, repository string,
) skillEntry {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(homeDir, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	report, err := skill.Install(homeDir)
	if err != nil {
		t.Fatalf("install sworn skill: %v", err)
	}
	if len(report.InstalledPaths) != 1 {
		t.Fatalf("installed skill paths = %v", report.InstalledPaths)
	}
	installed := report.InstalledPaths[0]
	body, err := os.ReadFile(installed)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	step := skillStepOne(t, text)

	// Evaluate the literal recognition rule the installed text states, against
	// this repository exactly as it is right now.
	entry := skillEntry{installedPath: installed}
	markers := map[string]bool{}
	for _, marker := range []string{".git", ".sworn/records", ".sworn"} {
		if !strings.Contains(step, "`"+marker+"`") {
			continue
		}
		_, statErr := os.Stat(filepath.Join(repository, filepath.FromSlash(marker)))
		markers[marker] = statErr == nil
	}
	if !strings.Contains(step, "`.git`") {
		t.Fatalf("installed skill step 1 does not recognize a Git worktree:\n%s", step)
	}
	if !markers[".git"] {
		return entry
	}
	entry.applies = true
	entry.initialize = !markers[".sworn/records"] && !markers[".sworn"]
	if entry.initialize && !strings.Contains(step, "`sworn init`") {
		t.Fatalf(
			"installed skill step 1 gives an uninitialized Git worktree no way in:\n%s",
			step,
		)
	}
	// A "does not apply" clause may only be scoped to a non-Git directory.
	for _, sentence := range strings.Split(step, ". ") {
		if strings.Contains(sentence, "does not apply") &&
			!strings.Contains(sentence, "outside a Git worktree") {
			t.Fatalf("installed skill step 1 refuses a Git worktree:\n%s", step)
		}
	}
	if !strings.Contains(text, "MCP") || !strings.Contains(text, "`sworn serve`") {
		t.Fatalf("installed skill does not route to the local MCP service:\n%s", text)
	}
	entry.transport = "mcp"
	return entry
}

// skillStepOne extracts the installed skill's literal first numbered step.
func skillStepOne(t *testing.T, text string) string {
	t.Helper()
	begin := strings.Index(text, "\n1. ")
	if begin < 0 {
		t.Fatalf("installed skill has no step 1:\n%s", text)
	}
	rest := text[begin+1:]
	end := strings.Index(rest, "\n2. ")
	if end < 0 {
		t.Fatalf("installed skill has no step 2:\n%s", text)
	}
	return rest[:end]
}

// answerActionIndex is the position of the open turn's answer control in the
// exact list the board publishes -- the same list the TUI renders.
func answerActionIndex(t *testing.T, snapshot cockpit.Snapshot) (int, cockpit.Action) {
	t.Helper()
	actions := append([]cockpit.Action(nil), snapshot.Actions...)
	if snapshot.ApprovalOffer != nil {
		command := snapshot.ApprovalOffer.Command
		actions = append(
			[]cockpit.Action{{Kind: "approve", Approval: &command}}, actions...,
		)
	}
	for index, action := range actions {
		if action.Kind == "answer_attention" {
			return index, action
		}
	}
	t.Fatalf("board publishes no answer control: %#v", actions)
	return 0, cockpit.Action{}
}

func mcpIsError(body []byte) bool {
	return bytes.Contains(body, []byte(`"isError":true`))
}

func mcpAnswer(
	t *testing.T, address, runID, attentionID string,
) []byte {
	t.Helper()
	return journeyMCPCall(t, address, "sworn_answer_attention", map[string]any{
		"run_id": runID, "attention_id": attentionID,
		"expected_generation": 1, "answer": surfaceAnswerCanary,
	})
}

func approvalArguments(t *testing.T, command swornruntime.ApprovalCommand) map[string]any {
	t.Helper()
	body, err := json.Marshal(command)
	if err != nil {
		t.Fatal(err)
	}
	var arguments map[string]any
	if err := json.Unmarshal(body, &arguments); err != nil {
		t.Fatal(err)
	}
	delete(arguments, "release_head")
	delete(arguments, "prior_plan")
	return arguments
}

func approveCLIArguments(
	command swornruntime.ApprovalCommand, journalPath string,
) []string {
	return []string{
		"approve",
		"--journal", journalPath,
		"--run", command.RunID,
		"--manifest-digest", command.ManifestDigest,
		"--project", command.Project,
		"--release", command.Release,
		"--release-ref", command.ReleaseRef,
		"--release-head", "absent",
		"--proposal-replay-key", command.ProposalReplayKey,
		"--plan-revision", fmt.Sprint(command.PlanRevision),
		"--prior-plan", "absent",
		"--plan-digest", command.PlanDigest,
		"--target-ref", command.TargetRef,
		"--target-head", command.TargetHead,
		"--decision-class", command.DecisionClass,
		"--decision", command.Decision,
		"--actor-class", command.ActorClass,
		"--actor-authority", command.ActorAuthority,
	}
}

// TestSwornConformanceObservableSurfaceParity is the executable half of the
// Sworn conformance profile. Each phase drives a real Sworn binary through one
// declared behavior on one declared surface and, only after observing it,
// registers the anchor that certifies that (case, surface) pair.
func TestSwornConformanceObservableSurfaceParity(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "sworn")
	buildBinary(t, binary, "./cmd/sworn", "")

	surfaceReadParity(t, binary)
	surfaceCLIAnswerAndRefusals(t, binary)
	surfaceMCPAnswerAndRefusals(t, binary, false)
	surfaceMCPAnswerAndRefusals(t, binary, true)
	surfaceTUIAnswer(t, binary)
	surfaceTUIStaleRefusal(t, binary)
}

// surfaceReadParity proves one parked run reads identically on all five
// surfaces at the same moment.
func surfaceReadParity(t *testing.T, binary string) {
	run := newSurfaceRun(t, binary, "surface-read-parity")
	run.startParked()
	serve := run.startServe()

	cliStatus := run.status()
	cliBoard := run.board()
	cliTurn := run.openHumanTurn()

	direct := mcpSnapshot(t, serve.address, run.runID)
	directTurns := mcpAttentions(t, serve.address)

	home := t.TempDir()
	entry := enterThroughInstalledSkill(t, home, run.repository)
	if !entry.applies || entry.transport != "mcp" {
		t.Fatalf("installed skill entry = %#v", entry)
	}
	skillRouted := mcpSnapshot(t, serve.address, run.runID)
	skillTurns := mcpAttentions(t, serve.address)

	// The configured driver is the fifth surface. What it observes is the
	// dispatch it was actually given; its invocation identity must name this
	// same run and the same responsibility every other surface reports as the
	// one waiting on a person.
	run.provider.mu.Lock()
	invocations := make([]string, 0, len(run.provider.turns))
	for invocation := range run.provider.turns {
		invocations = append(invocations, invocation)
	}
	run.provider.mu.Unlock()
	implementerInvocations := 0
	for _, invocation := range invocations {
		if !strings.HasPrefix(invocation, run.runID+"/") {
			t.Fatalf("configured driver saw a foreign run: %q", invocation)
		}
		if strings.Contains(
			invocation, string(driver.ImplementerImplementation),
		) {
			implementerInvocations++
		}
	}
	if implementerInvocations != 1 ||
		cliTurn.HumanTurn.Responsibility !=
			string(driver.ImplementerImplementation) {
		t.Fatalf("configured driver invocations = %v", invocations)
	}

	// Read parity: same run identity, same lifecycle state, everywhere.
	if cliStatus.RunID != run.runID || cliStatus.State != "parked" {
		t.Fatalf("cli status = %#v", cliStatus)
	}
	for label, snapshot := range map[string]cockpit.Snapshot{
		"cli": cliBoard, "direct_mcp": direct, "installed_skill_mcp": skillRouted,
	} {
		if snapshot.Run.ID != run.runID ||
			snapshot.Run.State != string(cliStatus.State) ||
			snapshot.Run.Release != run.release {
			t.Fatalf("%s run view = %#v", label, snapshot.Run)
		}
	}

	// Turn visibility parity: the same one turn, with the same question.
	for label, turns := range map[string][]cockpit.AttentionView{
		"direct_mcp": directTurns, "installed_skill_mcp": skillTurns,
	} {
		if len(turns) != 1 || turns[0].ID != cliTurn.ID ||
			turns[0].Question != cliTurn.Question ||
			turns[0].Generation != cliTurn.Generation {
			t.Fatalf("%s open turns = %#v", label, turns)
		}
	}

	// The TUI is a real terminal surface; it must show the same run, the same
	// state, and the same one waiting question.
	session := startPTYSession(
		t, binary, run.environment,
		"tui",
		"--project", run.repository,
		"--journal", run.journalPath,
		"--config", run.configPath,
	)
	session.waitFor("catalog", 20*time.Second, "RELEASES", run.release)
	session.send("\r")
	session.waitFor(
		"board", 20*time.Second,
		"Status", "Needs you", "Yes — answer the question shown in Human attention.",
	)
	session.openControls()
	session.waitFor(
		"controls", 20*time.Second, "Answer: "+surfaceQuestionCanary,
	)
	session.send("\x1b")
	session.quit()

	serve.stop(true)

	for _, surface := range []string{
		surfaceCLI, surfaceDirectMCP, surfaceInstalledSkillMCP, surfaceTUI,
		surfaceConfiguredDriver,
	} {
		recordSwornConformance(
			t, caseReadParity, surface, "surface-parity/read/"+surface,
		)
	}
	for _, surface := range []string{
		surfaceCLI, surfaceDirectMCP, surfaceInstalledSkillMCP, surfaceTUI,
	} {
		recordSwornConformance(
			t, caseTurnVisibility, surface, "surface-parity/turn-visible/"+surface,
		)
	}
}

// surfaceCLIAnswerAndRefusals proves the CLI admits exactly one answer,
// refuses the replay, and refuses a transition the run does not offer.
func surfaceCLIAnswerAndRefusals(t *testing.T, binary string) {
	run := newSurfaceRun(t, binary, "surface-cli-answer")
	run.startParked()
	turn := run.openHumanTurn()
	targetBefore := runGit(t, run.repository, "rev-parse", "main")

	// Unavailable transition: a complete approval for a plan revision this run
	// has never proposed.
	stdout, stderr := runBinaryWithEnvironment(
		t, binary, 1, run.environment,
		approveCLIArguments(run.unofferedApproval(), run.journalPath)...,
	)
	if stdout != "" || !strings.Contains(stderr, "sworn approve") {
		t.Fatalf("cli unoffered approval stdout=%q stderr=%q", stdout, stderr)
	}
	if run.status().State != "parked" ||
		runGit(t, run.repository, "rev-parse", "main") != targetBefore {
		t.Fatal("refused approval changed run state")
	}
	recordSwornConformance(
		t, caseUnavailableRefused, surfaceCLI, "surface-parity/unoffered-approval/cli",
	)

	stdout, stderr = runBinaryWithEnvironmentTimeout(
		t, binary, 0, run.environment, 180*time.Second,
		"answer", "--run", run.runID, "--journal", run.journalPath,
		"--attention", turn.ID, "--generation", "1",
		"--answer", surfaceAnswerCanary, "--config", run.configPath,
	)
	if stderr != "" {
		t.Fatalf("cli answer stderr=%q", stderr)
	}
	stdout, stderr = runBinaryWithEnvironmentTimeout(
		t, binary, 0, run.environment, 180*time.Second,
		"run", "--manifest", run.manifestPath, "--journal", run.journalPath,
		"--config", run.configPath,
	)
	if stderr != "" || !strings.Contains(stdout, "  state: complete") {
		t.Fatalf("cli answer stdout=%q stderr=%q", stdout, stderr)
	}
	answeredState := run.status().State
	recordSwornConformance(
		t, caseAnswerAdmittedOnce, surfaceCLI, "surface-parity/answer/cli",
	)

	stdout, stderr = runBinaryWithEnvironment(
		t, binary, 1, run.environment,
		"answer", "--run", run.runID, "--journal", run.journalPath,
		"--attention", turn.ID, "--generation", "1",
		"--answer", surfaceAnswerCanary, "--config", run.configPath,
	)
	if stdout != "" || !strings.Contains(stderr, "sworn answer") {
		t.Fatalf("cli replayed answer stdout=%q stderr=%q", stdout, stderr)
	}
	if run.status().State != answeredState {
		t.Fatalf("replayed answer changed state to %q", run.status().State)
	}
	if answers := run.admittedAnswers(); answers != 2 {
		t.Fatalf("replayed answer produced %d admitted answers", answers)
	}
	recordSwornConformance(
		t, caseStaleAnswerRefused, surfaceCLI, "surface-parity/replayed-answer/cli",
	)
}

// surfaceMCPAnswerAndRefusals proves the same three behaviors over MCP, once
// for a client speaking MCP directly and once for a client that reached MCP
// only by reading the installed Sworn skill.
func surfaceMCPAnswerAndRefusals(t *testing.T, binary string, throughSkill bool) {
	surface := surfaceDirectMCP
	label := "direct"
	if throughSkill {
		surface = surfaceInstalledSkillMCP
		label = "skill"
	}
	run := newSurfaceRun(t, binary, "surface-"+label+"-mcp-answer")
	run.startParked()
	if throughSkill {
		entry := enterThroughInstalledSkill(t, t.TempDir(), run.repository)
		if !entry.applies || entry.transport != "mcp" {
			t.Fatalf("installed skill entry = %#v", entry)
		}
	}
	serve := run.startServe()

	refused := journeyMCPCall(
		t, serve.address, "sworn_approve",
		approvalArguments(t, run.unofferedApproval()),
	)
	if !mcpIsError(refused) {
		t.Fatalf("%s mcp admitted an unoffered approval: %s", label, refused)
	}
	if run.status().State != "parked" {
		t.Fatal("refused MCP approval changed run state")
	}
	if !throughSkill {
		recordSwornConformance(
			t, caseUnavailableRefused, surface,
			"surface-parity/unoffered-approval/"+surface,
		)
	}

	turns := mcpAttentions(t, serve.address)
	if len(turns) != 1 {
		t.Fatalf("%s mcp open turns = %#v", label, turns)
	}
	answered := mcpAnswer(t, serve.address, run.runID, turns[0].ID)
	if mcpIsError(answered) {
		t.Fatalf("%s mcp answer = %s", label, answered)
	}
	deadline := time.Now().Add(60 * time.Second)
	for {
		snap := mcpSnapshot(t, serve.address, run.runID)
		if snap.Run.State == "complete" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s resident driver did not complete; last state = %q", label, snap.Run.State)
		}
		time.Sleep(100 * time.Millisecond)
	}
	recordSwornConformance(
		t, caseAnswerAdmittedOnce, surface, "surface-parity/answer/"+surface,
	)

	replayed := mcpAnswer(t, serve.address, run.runID, turns[0].ID)
	if !mcpIsError(replayed) {
		t.Fatalf("%s mcp admitted a replayed answer: %s", label, replayed)
	}
	if len(mcpAttentions(t, serve.address)) != 0 {
		t.Fatalf("%s mcp still reports an open turn after answering", label)
	}
	if answers := run.admittedAnswers(); answers != 2 {
		t.Fatalf("%s mcp produced %d admitted answers", label, answers)
	}
	recordSwornConformance(
		t, caseStaleAnswerRefused, surface, "surface-parity/replayed-answer/"+surface,
	)
	serve.stop(true)
}

// surfaceTUIAnswer proves a person at the real terminal board admits the same
// one answer, through the same command service, with the same result.
func surfaceTUIAnswer(t *testing.T, binary string) {
	run := newSurfaceRun(t, binary, "surface-tui-answer")
	run.startParked()
	index, action := answerActionIndex(t, run.board())
	if action.ExpectedGeneration != 1 {
		t.Fatalf("tui answer action = %#v", action)
	}

	session := startPTYSession(
		t, binary, run.environment,
		"tui",
		"--project", run.repository,
		"--journal", run.journalPath,
		"--config", run.configPath,
	)
	session.waitFor("catalog", 20*time.Second, "RELEASES", run.release)
	session.send("\r")
	session.waitFor("board", 20*time.Second, "Needs you")
	session.openControls()
	for range index {
		session.send("j")
	}
	session.send("\r")
	session.waitFor("answer overlay", 20*time.Second, "ctrl+s send")
	session.send(surfaceAnswerCanary)
	session.clear()
	session.send("\x13")
	session.waitFor(
		"accepted answer", 240*time.Second,
		"Answer: "+surfaceQuestionCanary+" accepted.",
	)
	// The answer returns once durable while the TUI process's own service
	// carries the drive, so completion is awaited before the TUI - and with
	// it the drive it hosts - is quit.
	completionDeadline := time.Now().Add(120 * time.Second)
	for run.status().State != "complete" {
		if time.Now().After(completionDeadline) {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	session.quit()

	if state := run.status().State; state != "complete" {
		t.Fatalf("tui answer left state %q", state)
	}
	if len(mcpOpenTurnCount(t, run)) != 0 {
		t.Fatal("tui answer left the turn open")
	}
	recordSwornConformance(
		t, caseAnswerAdmittedOnce, surfaceTUI, "surface-parity/answer/tui",
	)
}

// surfaceTUIStaleRefusal proves the terminal board refuses an action whose
// authority was superseded while the person was still typing.
func surfaceTUIStaleRefusal(t *testing.T, binary string) {
	run := newSurfaceRun(t, binary, "surface-tui-stale")
	run.startParked()
	index, _ := answerActionIndex(t, run.board())
	turn := run.openHumanTurn()

	session := startPTYSession(
		t, binary, run.environment,
		"tui",
		"--project", run.repository,
		"--journal", run.journalPath,
		"--config", run.configPath,
	)
	session.waitFor("catalog", 20*time.Second, "RELEASES", run.release)
	session.send("\r")
	session.waitFor("board", 20*time.Second, "Needs you")
	session.openControls()
	for range index {
		session.send("j")
	}
	session.send("\r")
	session.waitFor("answer overlay", 20*time.Second, "ctrl+s send")
	session.send(surfaceAnswerCanary)

	// The same turn is answered elsewhere while this overlay is still open, so
	// the control the terminal is holding is now superseded.
	stdout, stderr := runBinaryWithEnvironmentTimeout(
		t, binary, 0, run.environment, 240*time.Second,
		"answer", "--run", run.runID, "--journal", run.journalPath,
		"--attention", turn.ID, "--generation", "1",
		"--answer", surfaceAnswerCanary, "--config", run.configPath,
	)
	if stderr != "" {
		t.Fatalf("out-of-band answer stderr=%q", stderr)
	}
	stdout, stderr = runBinaryWithEnvironmentTimeout(
		t, binary, 0, run.environment, 240*time.Second,
		"run", "--manifest", run.manifestPath, "--journal", run.journalPath,
		"--config", run.configPath,
	)
	if stderr != "" || !strings.Contains(stdout, "  state: complete") {
		t.Fatalf("out-of-band answer stdout=%q stderr=%q", stdout, stderr)
	}
	stateAfterAnswer := run.status().State

	session.clear()
	session.send("\x13")
	session.waitFor(
		"stale refusal", 120*time.Second,
		"That action was not accepted.",
	)
	session.quit()

	if run.status().State != stateAfterAnswer {
		t.Fatalf("stale tui action changed state to %q", run.status().State)
	}
	recordSwornConformance(
		t, caseStaleAnswerRefused, surfaceTUI, "surface-parity/stale-action/tui",
	)
}

// mcpOpenTurnCount reads the open human-only turns through the CLI board, so
// the TUI phases do not need their own server.
func mcpOpenTurnCount(t *testing.T, run *surfaceRun) []cockpit.AttentionView {
	t.Helper()
	var open []cockpit.AttentionView
	for _, attention := range run.board().Runtime.Attentions {
		if attention.State == "open" && attention.HumanTurn != nil {
			open = append(open, attention)
		}
	}
	return open
}
