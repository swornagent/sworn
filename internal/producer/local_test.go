package producer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/executor"
	"github.com/swornagent/sworn/internal/protocol"
	"github.com/swornagent/sworn/internal/repo"
	"github.com/swornagent/sworn/internal/store"
)

var (
	testPlanDigest        = fixedDigest("a")
	testContractDigest    = fixedDigest("b")
	testPolicyDigest      = fixedDigest("c")
	testAuthorityDigest   = fixedDigest("d")
	testSourceDigest      = fixedDigest("e")
	testBuilderStart      = time.Date(2026, 7, 19, 1, 0, 0, 0, time.UTC)
	testBuilderCompletion = time.Date(2026, 7, 19, 1, 1, 0, 0, time.UTC)
	testCheckStart        = time.Date(2026, 7, 19, 1, 1, 5, 0, time.UTC)
	testCheckCompletion   = time.Date(2026, 7, 19, 1, 1, 6, 0, time.UTC)
)

type fakeRunner struct {
	completion func(executor.Invocation) executor.RawCompletion
}

type corruptArtifactReader struct {
	base   protocol.ArtifactReader
	digest string
}

func (reader corruptArtifactReader) Artifact(ctx context.Context, digest string) (string, []byte, error) {
	mediaType, contents, err := reader.base.Artifact(ctx, digest)
	if err == nil && digest == reader.digest {
		contents = append(append([]byte(nil), contents...), '\n')
	}
	return mediaType, contents, err
}

func (fakeRunner) Probe(context.Context) (executor.ProbeReport, error) {
	return executor.ProbeReport{
		BubblewrapVersion: "bubblewrap 0.9.0",
		SystemdVersion:    "systemd 255",
		CgroupV2:          true,
		UserManager:       "running",
		Controllers:       []string{"pids", "memory", "cpu"},
	}, nil
}

func (fakeRunner) EffectiveLimits() executor.Limits { return executor.DefaultLimits() }

func (runner fakeRunner) RunContained(_ context.Context, invocation executor.Invocation) (executor.RawCompletion, error) {
	return runner.completion(invocation), nil
}

func TestMeasuredSubmissionWalkingSkeleton(t *testing.T) {
	ctx := context.Background()
	source := newProducerTestRepository(t)
	binding, err := repo.Discover(ctx, source, "repo-01")
	if err != nil {
		t.Fatal(err)
	}
	repository, err := repo.Open(ctx, source, binding)
	if err != nil {
		t.Fatal(err)
	}
	target, err := repository.BindTarget(ctx, "refs/heads/main")
	if err != nil {
		t.Fatal(err)
	}
	builderWorkspace, err := repository.Materialize(ctx, target, filepath.Join(t.TempDir(), "builder"))
	if err != nil {
		t.Fatal(err)
	}
	writeProducerFile(t, filepath.Join(builderWorkspace.Path, "value.txt"), []byte("candidate\n"))
	candidate, err := repository.Capture(ctx, builderWorkspace, repo.CaptureOptions{
		Scope: repo.Scope{Include: []string{"."}}, Timestamp: testBuilderCompletion,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Mutable target movement does not rewrite the retained candidate or its
	// evidence. Integration will later enforce compare-and-swap.
	writeProducerFile(t, filepath.Join(source, "later.txt"), []byte("later target\n"))
	runProducerGit(t, source, "add", "--all")
	runProducerGit(t, source, "commit", "-m", "move target")
	checked, err := repository.MaterializeCandidate(ctx, candidate, filepath.Join(t.TempDir(), "checked"), repo.MaterializeLimits{
		Bytes: 1 << 20, Entries: 100,
	})
	if err != nil {
		t.Fatal(err)
	}

	control, err := store.Open(ctx, filepath.Join(t.TempDir(), "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = control.Close() })
	definition := LocalCheckDefinition{
		SchemaVersion:    LocalCheckDefinitionSchemaVersion,
		Argv:             []string{"/usr/bin/true"},
		WorkingDirectory: ".",
		TimeoutSeconds:   10,
		Evidence: EvidenceDefinition{
			ID: "candidate-check", AcceptanceIDs: []string{"AC1"}, Boundary: "component",
			UsesMocks: false, Observed: "The registered candidate check exited successfully.",
		},
	}
	definitionBytes, err := protocol.EncodeCanonical(definition)
	if err != nil {
		t.Fatal(err)
	}
	definitionDigest, err := control.PutArtifact(ctx, "application/json", definitionBytes)
	if err != nil {
		t.Fatal(err)
	}
	definitionPointer := protocol.Artifact{Ref: definitionDigest, MediaType: "application/json", Digest: definitionDigest}
	runner := fakeRunner{completion: func(invocation executor.Invocation) executor.RawCompletion {
		contents, err := os.ReadFile(filepath.Join(invocation.Workspace, "value.txt"))
		if err != nil || string(contents) != "candidate\n" {
			t.Fatalf("check workspace bytes = %q, %v", contents, err)
		}
		if _, err := os.Lstat(filepath.Join(invocation.Workspace, "later.txt")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("check workspace contains moved-target bytes: %v", err)
		}
		return executor.RawCompletion{
			InvocationID: invocation.ID, WorkspaceDigest: invocation.WorkspaceDigest,
			WorkspaceAccess: executor.WorkspaceReadOnly, StartedAt: testCheckStart,
			CompletedAt: testCheckCompletion, ExitCode: 0, Stdout: []byte("checked\n"),
		}
	}}
	produced, err := RunLocal(ctx, runner, control, Request{
		CheckID: "candidate", RunID: "check-run-1", Definition: definitionPointer,
		Repository: repository, Candidate: candidate, Workspace: checked,
	})
	if err != nil || produced.Check == nil || produced.Evidence == nil {
		t.Fatalf("RunLocal() = %#v, %v", produced, err)
	}
	authorityPointer := putAuthorityApproval(t, ctx, control)
	work := protocol.StructuralWork{
		DeliveryID: "delivery-1", WorkID: "work-1", PlanDigest: testPlanDigest,
		ContractDigest: testContractDigest, Repository: "repo-01", TargetRef: "refs/heads/main",
		Scope: repo.Scope{Include: []string{"."}}, PolicyRef: "policy:standard", PolicyDigest: testPolicyDigest,
		AuthorityDigest: testAuthorityDigest, AuthoritySourceRef: "authority-source",
		AuthoritySourceDigest: testSourceDigest,
		Acceptance:            []protocol.AcceptanceRequirement{{ID: "AC1", Boundary: "component"}},
		BaselineChecks: []protocol.BaselineCheck{{
			ID: "candidate", Definition: definitionPointer,
			Evidence: protocol.EvidenceRequirement{
				ID: "candidate-check", AcceptanceIDs: []string{"AC1"}, Boundary: "component",
				Observed: "The registered candidate check exited successfully.",
			},
		}},
	}
	builder := protocol.BuilderRun{
		RunID: "builder-run-1", Agent: "codex", StartedAt: formatTime(testBuilderStart),
		CompletedAt: formatTime(testBuilderCompletion),
	}
	submissionInput := protocol.SubmissionInput{
		Attempt: 1, CreatedAt: testCheckCompletion.Add(time.Second), Work: work,
		AuthorityReceipt: authorityPointer, Builder: builder, Candidate: candidate,
		Checks: []protocol.Check{*produced.Check}, Evidence: []protocol.Evidence{*produced.Evidence},
	}
	_, receiptBytes, err := control.Artifact(ctx, produced.Receipt.Digest)
	if err != nil {
		t.Fatal(err)
	}
	measuredReceipt, err := protocol.ParseLocalCheckReceipt(receiptBytes)
	if err != nil {
		t.Fatal(err)
	}
	withReceipt := func(test testing.TB, receipt protocol.LocalCheckReceipt) protocol.SubmissionInput {
		test.Helper()
		encoded, err := protocol.EncodeLocalCheckReceipt(receipt)
		if err != nil {
			test.Fatal(err)
		}
		digest, err := control.PutArtifact(ctx, "application/vnd.sworn.local-check-receipt+json", encoded.CanonicalJSON)
		if err != nil {
			test.Fatal(err)
		}
		pointer := protocol.Artifact{
			Ref: digest, MediaType: "application/vnd.sworn.local-check-receipt+json", Digest: digest,
		}
		input := submissionInput
		input.Checks = append([]protocol.Check(nil), submissionInput.Checks...)
		input.Evidence = append([]protocol.Evidence(nil), submissionInput.Evidence...)
		input.Checks[0].Receipt = pointer
		input.Checks[0].Environment = receipt.Environment
		input.Evidence[0].Artifact = pointer
		input.Evidence[0].Environment = receipt.Environment
		return input
	}
	for name, mutate := range map[string]func(*protocol.LocalCheckReceipt){
		"argv":    func(receipt *protocol.LocalCheckReceipt) { receipt.Argv = []string{"/usr/bin/false"} },
		"timeout": func(receipt *protocol.LocalCheckReceipt) { receipt.TimeoutSeconds++ },
	} {
		t.Run("reject receipt definition drift "+name, func(t *testing.T) {
			receipt := measuredReceipt
			mutate(&receipt)
			if _, err := protocol.BuildSubmission(ctx, repository, control, withReceipt(t, receipt)); err == nil {
				t.Fatal("definition-drifted receipt was admitted")
			}
		})
	}
	invalidEnvironmentDigest, err := control.PutArtifact(ctx, protocol.LocalEnvironmentMediaType, []byte("{}"))
	if err != nil {
		t.Fatal(err)
	}
	invalidEnvironmentReceipt := measuredReceipt
	invalidEnvironmentReceipt.Environment = protocol.Environment{Kind: "local", Ref: invalidEnvironmentDigest}
	if _, err := protocol.BuildSubmission(ctx, repository, control, withReceipt(t, invalidEnvironmentReceipt)); err == nil {
		t.Fatal("schema-less local environment was admitted")
	}
	changedDefinition := definition
	changedDefinition.Evidence.Observed = "Different policy semantics."
	changedDefinitionBytes, err := protocol.EncodeCanonical(changedDefinition)
	if err != nil {
		t.Fatal(err)
	}
	changedDefinitionDigest, err := control.PutArtifact(ctx, "application/json", changedDefinitionBytes)
	if err != nil {
		t.Fatal(err)
	}
	changedDefinitionPointer := protocol.Artifact{
		Ref: changedDefinitionDigest, MediaType: "application/json", Digest: changedDefinitionDigest,
	}
	changedDefinitionReceipt := measuredReceipt
	changedDefinitionReceipt.Definition = changedDefinitionPointer
	changedDefinitionInput := withReceipt(t, changedDefinitionReceipt)
	changedDefinitionInput.Work.BaselineChecks = append([]protocol.BaselineCheck(nil), work.BaselineChecks...)
	changedDefinitionInput.Work.BaselineChecks[0].Definition = changedDefinitionPointer
	if _, err := protocol.BuildSubmission(ctx, repository, control, changedDefinitionInput); err == nil {
		t.Fatal("definition with different admitted evidence semantics was admitted")
	}
	falseAuthorityRef := submissionInput
	falseAuthorityRef.AuthorityReceipt.Ref = "artifact:false"
	if _, err := protocol.BuildSubmission(ctx, repository, control, falseAuthorityRef); err == nil {
		t.Fatal("false non-CAS authority reference was admitted")
	}
	built, err := protocol.BuildSubmission(ctx, repository, control, submissionInput)
	if err != nil {
		t.Fatal(err)
	}
	badBinding := submissionInput
	badBinding.Evidence = append([]protocol.Evidence(nil), submissionInput.Evidence...)
	badBinding.Evidence[0].ProducerRunID = builder.RunID
	if _, err := protocol.BuildSubmission(ctx, repository, control, badBinding); err == nil {
		t.Fatal("builder-stamped evidence was admitted")
	}
	if _, err := protocol.BuildSubmission(ctx, repository, corruptArtifactReader{
		base: control, digest: produced.Receipt.Digest,
	}, submissionInput); err == nil {
		t.Fatal("changed receipt bytes were admitted")
	}
	incomplete, err := store.Open(ctx, filepath.Join(t.TempDir(), "incomplete.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = incomplete.Close() })
	for _, pointer := range []protocol.Artifact{authorityPointer, produced.Receipt} {
		mediaType, contents, err := control.Artifact(ctx, pointer.Digest)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := incomplete.PutArtifact(ctx, mediaType, contents); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := incomplete.PutSubmission(ctx, built); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("incomplete destination artifact closure error = %v", err)
	}
	digest, err := control.PutSubmission(ctx, built)
	if err != nil {
		t.Fatal(err)
	}
	record := built.Record()
	submission := built.Submission()
	if digest != record.Digest {
		t.Fatalf("stored digest = %q, built digest %q", digest, record.Digest)
	}
	storedDigest, canonical, err := control.SubmissionRecord(ctx, submission.SubmissionID)
	if err != nil || storedDigest != digest || !bytes.Equal(canonical, record.CanonicalJSON) {
		t.Fatalf("stored record = %q %q, %v", storedDigest, canonical, err)
	}
	for _, pointer := range built.Dependencies() {
		mediaType, contents, err := control.Artifact(ctx, pointer.Digest)
		if err != nil || mediaType != pointer.MediaType || protocol.RawDigest(contents) != pointer.Digest {
			t.Fatalf("artifact %s = %q %x, %v", pointer.Digest, mediaType, contents, err)
		}
	}
}

func TestLocalCheckNonPassIsRetainedButCannotBecomeEvidence(t *testing.T) {
	ctx := context.Background()
	repository, candidate, checked := prepareProducerCandidate(t)
	control, err := store.Open(ctx, filepath.Join(t.TempDir(), "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = control.Close() })
	definitionBytes, _ := protocol.EncodeCanonical(LocalCheckDefinition{
		SchemaVersion: LocalCheckDefinitionSchemaVersion, Argv: []string{"/usr/bin/false"},
		WorkingDirectory: ".", TimeoutSeconds: 10,
		Evidence: EvidenceDefinition{ID: "evidence", AcceptanceIDs: []string{"AC1"}, Boundary: "component", Observed: "passed"},
	})
	digest, err := control.PutArtifact(ctx, "application/json", definitionBytes)
	if err != nil {
		t.Fatal(err)
	}
	runner := fakeRunner{completion: func(invocation executor.Invocation) executor.RawCompletion {
		return executor.RawCompletion{
			InvocationID: invocation.ID, WorkspaceDigest: invocation.WorkspaceDigest,
			WorkspaceAccess: executor.WorkspaceReadOnly, StartedAt: testCheckStart,
			CompletedAt: testCheckCompletion, ExitCode: 7, Stderr: []byte("failed\n"),
		}
	}}
	result, err := RunLocal(ctx, runner, control, Request{
		CheckID: "candidate", RunID: "check-run-7",
		Definition: protocol.Artifact{Ref: digest, MediaType: "application/json", Digest: digest},
		Repository: repository, Candidate: candidate, Workspace: checked,
	})
	if !errors.Is(err, ErrCheckNotAdmitted) || result.Check != nil || result.Evidence != nil || result.Receipt.Digest == "" {
		t.Fatalf("non-pass result = %#v, %v", result, err)
	}
	_, raw, err := control.Artifact(ctx, result.Receipt.Digest)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := protocol.ParseLocalCheckReceipt(raw)
	if err != nil || receipt.Outcome != "not_admitted" || receipt.ExitCode != 7 {
		t.Fatalf("non-pass receipt = %#v, %v", receipt, err)
	}
	if _, _, err := control.SubmissionRecord(ctx, "anything"); err == nil {
		t.Fatal("non-pass execution created a submission")
	}
}

func putAuthorityApproval(t *testing.T, ctx context.Context, control *store.Store) protocol.Artifact {
	t.Helper()
	receipt := map[string]any{
		"schema_version": "control-receipt-v1", "kind": "authority_approval",
		"receipt_id": "authority-1", "plan_digest": testPlanDigest,
		"authority_digest": testAuthorityDigest, "source_ref": "authority-source",
		"source_digest": testSourceDigest,
		"grants": []any{
			map[string]any{"action": "inspect", "target": "workspace"},
			map[string]any{"action": "edit", "target": "workspace"},
			map[string]any{"action": "execute", "target": "workspace"},
			map[string]any{"action": "commit", "target": "workspace"},
		},
		"repository": "repo-01", "target_ref": "refs/heads/main",
		"authorizer_ref": "identity:test", "approved_at": formatTime(testBuilderStart.Add(-time.Second)),
	}
	contents, err := protocol.EncodeCanonical(receipt)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := control.PutArtifact(ctx, "application/json", contents)
	if err != nil {
		t.Fatal(err)
	}
	return protocol.Artifact{Ref: digest, MediaType: "application/json", Digest: digest}
}

func fixedDigest(character string) string { return "sha256:" + strings.Repeat(character, 64) }

func prepareProducerCandidate(t *testing.T) (*repo.Repository, repo.Candidate, repo.CandidateWorkspace) {
	t.Helper()
	ctx := context.Background()
	source := newProducerTestRepository(t)
	binding, err := repo.Discover(ctx, source, "repo-01")
	if err != nil {
		t.Fatal(err)
	}
	repository, err := repo.Open(ctx, source, binding)
	if err != nil {
		t.Fatal(err)
	}
	target, err := repository.BindTarget(ctx, "refs/heads/main")
	if err != nil {
		t.Fatal(err)
	}
	builder, err := repository.Materialize(ctx, target, filepath.Join(t.TempDir(), "builder"))
	if err != nil {
		t.Fatal(err)
	}
	writeProducerFile(t, filepath.Join(builder.Path, "value.txt"), []byte("candidate\n"))
	candidate, err := repository.Capture(ctx, builder, repo.CaptureOptions{
		Scope: repo.Scope{Include: []string{"."}}, Timestamp: testBuilderCompletion,
	})
	if err != nil {
		t.Fatal(err)
	}
	checked, err := repository.MaterializeCandidate(
		ctx,
		candidate,
		filepath.Join(t.TempDir(), "checked"),
		repo.MaterializeLimits{Bytes: 1 << 20, Entries: 100},
	)
	if err != nil {
		t.Fatal(err)
	}
	return repository, candidate, checked
}

func newProducerTestRepository(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "repository")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	runProducerGit(t, root, "init", "-b", "main")
	runProducerGit(t, root, "config", "user.name", "Test Author")
	runProducerGit(t, root, "config", "user.email", "test@example.invalid")
	writeProducerFile(t, filepath.Join(root, "value.txt"), []byte("base\n"))
	runProducerGit(t, root, "add", "--all")
	runProducerGit(t, root, "commit", "-m", "base")
	return root
}

func runProducerGit(t *testing.T, root string, arguments ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, arguments...)...)
	command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(arguments, " "), err, output)
	}
}

func writeProducerFile(t *testing.T, path string, contents []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDefinitionRejectsUnknownOrAmbiguousEvidence(t *testing.T) {
	t.Parallel()
	for name, input := range map[string]string{
		"unknown":               `{"schema_version":"sworn-local-check-v1","argv":["/usr/bin/true"],"working_directory":".","timeout_seconds":1,"evidence":{"id":"e","acceptance_ids":["AC1"],"boundary":"component","uses_mocks":false,"observed":"ok"},"extra":true}`,
		"duplicate acceptance":  `{"schema_version":"sworn-local-check-v1","argv":["/usr/bin/true"],"working_directory":".","timeout_seconds":1,"evidence":{"id":"e","acceptance_ids":["AC1","AC1"],"boundary":"component","uses_mocks":false,"observed":"ok"}}`,
		"mocked assembled":      `{"schema_version":"sworn-local-check-v1","argv":["/usr/bin/true"],"working_directory":".","timeout_seconds":1,"evidence":{"id":"e","acceptance_ids":["AC1"],"boundary":"assembled","uses_mocks":true,"observed":"ok"}}`,
		"invalid evidence id":   `{"schema_version":"sworn-local-check-v1","argv":["/usr/bin/true"],"working_directory":".","timeout_seconds":1,"evidence":{"id":"bad/id","acceptance_ids":["AC1"],"boundary":"component","uses_mocks":false,"observed":"ok"}}`,
		"invalid acceptance id": `{"schema_version":"sworn-local-check-v1","argv":["/usr/bin/true"],"working_directory":".","timeout_seconds":1,"evidence":{"id":"e","acceptance_ids":["bad/id"],"boundary":"component","uses_mocks":false,"observed":"ok"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseDefinition([]byte(input)); err == nil {
				t.Fatal("invalid definition was accepted")
			}
		})
	}
	arguments := make([]string, 257)
	arguments[0] = "/usr/bin/true"
	for index := 1; index < len(arguments); index++ {
		arguments[index] = "argument"
	}
	contents, err := protocol.EncodeCanonical(LocalCheckDefinition{
		SchemaVersion: LocalCheckDefinitionSchemaVersion, Argv: arguments,
		WorkingDirectory: ".", TimeoutSeconds: 1,
		Evidence: EvidenceDefinition{ID: "e", AcceptanceIDs: []string{"AC1"}, Boundary: "component", Observed: "ok"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parseDefinition(contents); err == nil || !strings.Contains(err.Error(), "256") {
		t.Fatalf("oversized argv error = %v", err)
	}
}

func TestLocalCheckDefinitionJSONIsStable(t *testing.T) {
	t.Parallel()
	definition := LocalCheckDefinition{
		SchemaVersion: LocalCheckDefinitionSchemaVersion, Argv: []string{"/usr/bin/true"},
		WorkingDirectory: ".", TimeoutSeconds: 1,
		Evidence: EvidenceDefinition{ID: "e", AcceptanceIDs: []string{"AC1"}, Boundary: "component", Observed: "ok"},
	}
	contents, err := json.Marshal(definition)
	if err != nil || !json.Valid(contents) {
		t.Fatalf("definition JSON = %q, %v", contents, err)
	}
}
