package effects

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/engine"
	"github.com/swornagent/sworn/internal/executor"
	"github.com/swornagent/sworn/internal/protocol"
	"github.com/swornagent/sworn/internal/repo"
	"github.com/swornagent/sworn/internal/workspace"
)

type memoryControl struct {
	artifacts map[string]memoryArtifact
	builder   engine.JournalEffect
}

type memoryArtifact struct {
	mediaType string
	contents  []byte
}

func (control *memoryControl) PutArtifact(_ context.Context, mediaType string, contents []byte) (string, error) {
	digest := protocol.RawDigest(contents)
	control.artifacts[digest] = memoryArtifact{mediaType: mediaType, contents: append([]byte(nil), contents...)}
	return digest, nil
}

func (control *memoryControl) Artifact(_ context.Context, digest string) (string, []byte, error) {
	artifact, exists := control.artifacts[digest]
	if !exists {
		return "", nil, errors.New("artifact not found")
	}
	return artifact.mediaType, append([]byte(nil), artifact.contents...), nil
}

func (control *memoryControl) SucceededEffect(_ context.Context, effectID string) (engine.JournalEffect, error) {
	if effectID != control.builder.ID {
		return engine.JournalEffect{}, errors.New("effect not found")
	}
	return control.builder, nil
}

type recordingRunner struct {
	completion executor.RawCompletion
	hostCalls  int
	boundCalls int
}

func (*recordingRunner) Probe(context.Context) (executor.ProbeReport, error) {
	return executor.ProbeReport{
		BubblewrapVersion: "bubblewrap 0.9.0", SystemdVersion: "systemd 255", CgroupV2: true,
		UserManager: "running", Controllers: []string{"cpu", "memory", "pids"},
	}, nil
}

func (*recordingRunner) EffectiveLimits() executor.Limits { return executor.DefaultLimits() }

func (runner *recordingRunner) RunContained(context.Context, executor.Invocation) (executor.RawCompletion, error) {
	runner.hostCalls++
	return executor.RawCompletion{}, errors.New("host runtime was called")
}

func (runner *recordingRunner) RunContentBound(
	_ context.Context,
	invocation executor.Invocation,
	runtime executor.RuntimeTree,
) (executor.RawCompletion, error) {
	runner.boundCalls++
	completion := runner.completion
	completion.InvocationID = invocation.ID
	completion.RuntimeDigest = runtime.Digest()
	completion.WorkspaceDigest = invocation.WorkspaceDigest
	completion.WorkspaceAccess = executor.WorkspaceReadOnly
	return completion, nil
}

func TestLocalCheckWorkerUsesJournalBuilderAndContentRuntimeForKnownOutcomes(t *testing.T) {
	ctx := context.Background()
	repository, candidate := effectCandidate(t)
	runtimeSource := t.TempDir()
	writeEffectFile(t, filepath.Join(runtimeSource, "bin", "check"), []byte("runtime\n"), 0o755)
	runtimeDigest, _, err := workspace.Measure(ctx, runtimeSource, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	runtimeTree, err := executor.NewRuntimeTree(runtimeSource, runtimeDigest, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	buildRequest, _ := json.Marshal(engine.BuildEffectRequest{
		SchemaVersion: engine.BuildEffectRequestSchemaVersion, DeliveryRunID: "delivery-run",
		DeliveryID: "delivery-1", WorkID: "work-1", WorkAttempt: 1,
		DispatchDigest: testEffectDigest("a"),
	})
	buildResult, err := engine.EncodeBuildEffectResult(engine.BuildEffectResult{
		SchemaVersion: engine.BuildEffectResultSchemaVersion, Outcome: engine.BuildOutcomeCandidateReady,
		Builder: protocol.BuilderRun{
			RunID: "builder-effect", Agent: "codex",
			StartedAt: "2026-07-20T00:00:00Z", CompletedAt: "2026-07-20T00:01:00Z",
		},
		Candidate: candidate,
	})
	if err != nil {
		t.Fatal(err)
	}
	for name, completion := range map[string]executor.RawCompletion{
		engine.LocalCheckOutcomePass: {
			StartedAt:   time.Date(2026, 7, 20, 0, 2, 0, 0, time.UTC),
			CompletedAt: time.Date(2026, 7, 20, 0, 2, 1, 0, time.UTC), ExitCode: 0,
		},
		engine.LocalCheckOutcomeNotAdmitted: {
			StartedAt:   time.Date(2026, 7, 20, 0, 2, 0, 0, time.UTC),
			CompletedAt: time.Date(2026, 7, 20, 0, 2, 1, 0, time.UTC), ExitCode: 7,
		},
		engine.LocalCheckOutcomeControlled: {
			StartedAt:   time.Date(2026, 7, 20, 0, 2, 0, 0, time.UTC),
			CompletedAt: time.Date(2026, 7, 20, 0, 2, 1, 0, time.UTC), ExitCode: -1, TimedOut: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			control := &memoryControl{artifacts: make(map[string]memoryArtifact), builder: engine.JournalEffect{
				ID: "builder-effect", DeliveryRunID: "delivery-run", Kind: engine.EffectBuild,
				Attempt: 1, Request: buildRequest, Result: buildResult,
			}}
			definition, err := protocol.EncodeCanonical(protocol.LocalCheckDefinition{
				SchemaVersion: protocol.LocalCheckDefinitionSchemaVersion,
				Argv:          []string{"/usr/bin/check"}, WorkingDirectory: ".", TimeoutSeconds: 10,
				Evidence: protocol.LocalEvidenceDefinition{
					ID: "evidence-1", AcceptanceIDs: []string{"AC1"}, Boundary: "component", Observed: "passed",
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			definitionDigest, _ := control.PutArtifact(ctx, "application/json", definition)
			request, err := engine.EncodeLocalCheckEffectRequest(engine.LocalCheckEffectRequest{
				SchemaVersion: engine.LocalCheckEffectRequestSchemaVersion,
				DeliveryRunID: "delivery-run", DeliveryID: "delivery-1", WorkID: "work-1", WorkAttempt: 1,
				BuilderEffectID: "builder-effect", CheckID: "check-1",
				DefinitionDigest: definitionDigest, RuntimeManifestDigest: runtimeDigest,
			})
			if err != nil {
				t.Fatal(err)
			}
			runner := &recordingRunner{completion: completion}
			root := t.TempDir()
			if err := os.Chmod(root, 0o700); err != nil {
				t.Fatal(err)
			}
			worker := LocalCheckWorker{
				Control: control, Runner: runner, Repository: repository, Runtime: runtimeTree,
				WorkspaceRoot: root, MaterializeLimits: repo.MaterializeLimits{Bytes: 1 << 20, Entries: 100},
			}
			encoded, err := worker.Run(ctx, engine.JournalEffect{
				ID: "check-effect-" + strings.ReplaceAll(name, "_", "-"), DeliveryRunID: "delivery-run",
				Kind: engine.EffectLocalCheck, Attempt: 1, Request: request,
			})
			if err != nil {
				t.Fatal(err)
			}
			result, err := engine.ParseLocalCheckEffectResult(encoded)
			if err != nil || result.Outcome != name || result.Receipt.Digest == "" {
				t.Fatalf("result = %#v, %v", result, err)
			}
			if runner.boundCalls != 1 || runner.hostCalls != 0 {
				t.Fatalf("runner calls = bound:%d host:%d", runner.boundCalls, runner.hostCalls)
			}
			entries, err := os.ReadDir(root)
			if err != nil || len(entries) != 0 {
				t.Fatalf("workspace cleanup = %v, %v", entries, err)
			}
		})
	}
}

func TestLocalCheckWorkerRejectsRuntimeDriftBeforeExecution(t *testing.T) {
	request, err := engine.EncodeLocalCheckEffectRequest(engine.LocalCheckEffectRequest{
		SchemaVersion: engine.LocalCheckEffectRequestSchemaVersion,
		DeliveryRunID: "delivery-run", DeliveryID: "delivery-1", WorkID: "work-1", WorkAttempt: 1,
		BuilderEffectID: "builder-effect", CheckID: "check-1",
		DefinitionDigest: testEffectDigest("a"), RuntimeManifestDigest: testEffectDigest("b"),
	})
	if err != nil {
		t.Fatal(err)
	}
	runtimeSource := t.TempDir()
	writeEffectFile(t, filepath.Join(runtimeSource, "bin", "check"), []byte("runtime\n"), 0o755)
	digest, _, _ := workspace.Measure(context.Background(), runtimeSource, 1<<20)
	runtimeTree, _ := executor.NewRuntimeTree(runtimeSource, digest, 1<<20)
	runner := &recordingRunner{}
	worker := LocalCheckWorker{
		Control: &memoryControl{artifacts: make(map[string]memoryArtifact)}, Runner: runner,
		Repository: &repo.Repository{}, Runtime: runtimeTree, WorkspaceRoot: t.TempDir(),
	}
	if _, err := worker.Run(context.Background(), engine.JournalEffect{
		ID: "check-effect", DeliveryRunID: "delivery-run", Kind: engine.EffectLocalCheck, Attempt: 1, Request: request,
	}); err == nil || !strings.Contains(err.Error(), "configured runtime") {
		t.Fatalf("runtime drift error = %v", err)
	}
	if runner.boundCalls != 0 || runner.hostCalls != 0 {
		t.Fatal("runtime drift reached the runner")
	}
}

func TestLocalCheckWorkerRejectsSymlinkedWorkspaceRootBeforeResolution(t *testing.T) {
	runtimeSource := t.TempDir()
	writeEffectFile(t, filepath.Join(runtimeSource, "bin", "check"), []byte("runtime\n"), 0o755)
	digest, _, err := workspace.Measure(context.Background(), runtimeSource, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	runtimeTree, err := executor.NewRuntimeTree(runtimeSource, digest, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	request, err := engine.EncodeLocalCheckEffectRequest(engine.LocalCheckEffectRequest{
		SchemaVersion: engine.LocalCheckEffectRequestSchemaVersion,
		DeliveryRunID: "delivery-run", DeliveryID: "delivery-1", WorkID: "work-1", WorkAttempt: 1,
		BuilderEffectID: "builder-effect", CheckID: "check-1",
		DefinitionDigest: testEffectDigest("a"), RuntimeManifestDigest: digest,
	})
	if err != nil {
		t.Fatal(err)
	}
	workspaceLink := filepath.Join(t.TempDir(), "workspace-root")
	if err := os.Symlink(t.TempDir(), workspaceLink); err != nil {
		t.Skipf("create workspace-root symlink: %v", err)
	}
	runner := &recordingRunner{}
	worker := LocalCheckWorker{
		Control: &memoryControl{artifacts: make(map[string]memoryArtifact)}, Runner: runner,
		Repository: &repo.Repository{}, Runtime: runtimeTree, WorkspaceRoot: workspaceLink,
	}
	if _, err := worker.Run(context.Background(), engine.JournalEffect{
		ID: "check-effect", DeliveryRunID: "delivery-run", Kind: engine.EffectLocalCheck, Attempt: 1, Request: request,
	}); err == nil || !strings.Contains(err.Error(), "symbolic-link remap") {
		t.Fatalf("symlinked workspace-root error = %v", err)
	}
	if runner.boundCalls != 0 || runner.hostCalls != 0 {
		t.Fatal("symlinked workspace root reached the runner")
	}
}

func TestRemoveWorkspaceRootRefusesReplacedDirectory(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "invocation")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	identity, err := os.Lstat(root)
	if err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(parent, "moved")
	if err := os.Rename(root, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := removeWorkspaceRoot(root, identity); err == nil || !strings.Contains(err.Error(), "identity changed") {
		t.Fatalf("replaced workspace cleanup error = %v", err)
	}
	for _, path := range []string{root, moved} {
		if info, err := os.Lstat(path); err != nil || !info.IsDir() {
			t.Fatalf("cleanup removed unrelated directory %q: %v", path, err)
		}
	}
}

func effectCandidate(t *testing.T) (*repo.Repository, repo.Candidate) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "repository")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	runEffectGit(t, root, "init", "-b", "main")
	runEffectGit(t, root, "config", "user.name", "Test Author")
	runEffectGit(t, root, "config", "user.email", "test@example.invalid")
	writeEffectFile(t, filepath.Join(root, "value.txt"), []byte("base\n"), 0o644)
	runEffectGit(t, root, "add", "--all")
	runEffectGit(t, root, "commit", "-m", "base")
	binding, err := repo.Discover(context.Background(), root, "repo-1")
	if err != nil {
		t.Fatal(err)
	}
	repository, err := repo.Open(context.Background(), root, binding)
	if err != nil {
		t.Fatal(err)
	}
	target, err := repository.BindTarget(context.Background(), "refs/heads/main")
	if err != nil {
		t.Fatal(err)
	}
	builder, err := repository.Materialize(context.Background(), target, filepath.Join(t.TempDir(), "builder"))
	if err != nil {
		t.Fatal(err)
	}
	writeEffectFile(t, filepath.Join(builder.Path, "value.txt"), []byte("candidate\n"), 0o644)
	candidate, err := repository.Capture(context.Background(), builder, repo.CaptureOptions{
		Scope: repo.Scope{Include: []string{"."}}, Timestamp: time.Date(2026, 7, 20, 0, 1, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	return repository, candidate
}

func runEffectGit(t *testing.T, root string, arguments ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, arguments...)...)
	command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(arguments, " "), err, output)
	}
}

func writeEffectFile(t *testing.T, path string, contents []byte, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, contents, mode); err != nil {
		t.Fatal(err)
	}
}

func testEffectDigest(character string) string {
	return "sha256:" + strings.Repeat(character, 64)
}
