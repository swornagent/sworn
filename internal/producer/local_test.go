package producer_test

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
	"github.com/swornagent/sworn/internal/producer"
	"github.com/swornagent/sworn/internal/protocol"
	"github.com/swornagent/sworn/internal/repo"
	"github.com/swornagent/sworn/internal/store"
	"github.com/swornagent/sworn/internal/workspace"
)

var (
	testSourceDigest      = fixedDigest("e")
	testBuilderStart      = time.Date(2026, 7, 19, 1, 0, 0, 0, time.UTC)
	testBuilderCompletion = time.Date(2026, 7, 19, 1, 1, 0, 0, time.UTC)
	testCheckStart        = time.Date(2026, 7, 19, 1, 1, 5, 0, time.UTC)
	testCheckCompletion   = time.Date(2026, 7, 19, 1, 1, 6, 0, time.UTC)
)

type fakeRunner struct {
	completion func(executor.Invocation) executor.RawCompletion
}

type contentOnlyRunner struct{ fakeRunner }

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

func submissionReceipt(
	t testing.TB,
	ctx context.Context,
	artifacts protocol.ArtifactReader,
	receiptPointer protocol.Artifact,
) protocol.LocalCheckReceipt {
	t.Helper()
	mediaType, contents, err := artifacts.Artifact(ctx, receiptPointer.Digest)
	if err != nil || mediaType != receiptPointer.MediaType || protocol.RawDigest(contents) != receiptPointer.Digest {
		t.Fatalf("resolve measured receipt = %q %x, %v", mediaType, contents, err)
	}
	receipt, err := protocol.ParseLocalCheckReceipt(contents)
	if err != nil || receipt.Outcome != "pass" {
		t.Fatalf("parse admitted receipt = %#v, %v", receipt, err)
	}
	return receipt
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

func (runner fakeRunner) RunContentBound(
	_ context.Context,
	invocation executor.Invocation,
	_ executor.RuntimeTree,
) (executor.RawCompletion, error) {
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
	definition := protocol.LocalCheckDefinition{
		SchemaVersion:    protocol.LocalCheckDefinitionSchemaVersion,
		Argv:             []string{"/usr/bin/true"},
		WorkingDirectory: ".",
		TimeoutSeconds:   10,
		Evidence: protocol.LocalEvidenceDefinition{
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
	plan := putSubmissionPlan(t, ctx, control, definitionPointer)
	runner := fakeRunner{completion: func(invocation executor.Invocation) executor.RawCompletion {
		contents, err := os.ReadFile(filepath.Join(invocation.Workspace, "value.txt"))
		if err != nil || string(contents) != "candidate\n" {
			t.Fatalf("check workspace bytes = %q, %v", contents, err)
		}
		if _, err := os.Lstat(filepath.Join(invocation.Workspace, "later.txt")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("check workspace contains moved-target bytes: %v", err)
		}
		return executor.RawCompletion{
			InvocationID: invocation.ID, RuntimeDigest: invocation.RuntimeDigest,
			WorkspaceDigest: invocation.WorkspaceDigest,
			WorkspaceAccess: executor.WorkspaceReadOnly, StartedAt: testCheckStart,
			CompletedAt: testCheckCompletion, ExitCode: 0, Stdout: []byte("checked\n"),
		}
	}}
	runtimeTree, runtimeDigest := producerRuntimeTree(t, ctx, "/usr/bin/true")
	produced, err := producer.RunLocalContentBound(ctx, contentOnlyRunner{fakeRunner: runner}, control, producer.Request{
		CheckID: "candidate", RunID: "check-run-1", Definition: definitionPointer,
		Repository: repository, Candidate: candidate, Workspace: checked,
	}, runtimeTree)
	if err != nil || produced.Receipt.Digest == "" {
		t.Fatalf("RunLocalContentBound() = %#v, %v", produced, err)
	}
	measuredReceipt := submissionReceipt(t, ctx, control, produced.Receipt)
	_, environmentBytes, err := control.Artifact(ctx, measuredReceipt.Environment.Ref)
	if err != nil {
		t.Fatal(err)
	}
	environment, err := protocol.ParseLocalEnvironment(environmentBytes)
	if err != nil || environment.SchemaVersion != protocol.ContentEnvironmentSchemaVersion ||
		environment.RuntimeManifestDigest != runtimeDigest {
		t.Fatalf("content-bound admission environment = %#v, %v", environment, err)
	}
	authorityPointer := putAuthorityApproval(t, ctx, control, plan)
	builder := protocol.BuilderRun{
		RunID: "builder-run-1", Agent: "codex", StartedAt: formatTime(testBuilderStart),
		CompletedAt: formatTime(testBuilderCompletion),
	}
	submissionInput := protocol.SubmissionInput{
		Attempt: 1, CreatedAt: testCheckCompletion.Add(time.Second), Plan: plan, WorkID: "work-1",
		AuthorityReceipt: authorityPointer, Builder: builder, Candidate: candidate,
		MeasuredChecks: []protocol.MeasuredCheck{{
			RunID: measuredReceipt.RunID, RuntimeManifestDigest: runtimeDigest, Receipt: produced.Receipt,
		}},
	}
	unknownWork := submissionInput
	unknownWork.WorkID = "missing-work"
	if _, err := protocol.BuildSubmission(ctx, repository, control, unknownWork); err == nil {
		t.Fatal("work absent from the exact plan was admitted")
	}
	_, authorityBytes, err := control.Artifact(ctx, authorityPointer.Digest)
	if err != nil {
		t.Fatal(err)
	}
	driftedApproval, err := protocol.ParseAuthorityApproval(authorityBytes)
	if err != nil {
		t.Fatal(err)
	}
	driftedApproval.Grants = append([]json.RawMessage(nil), driftedApproval.Grants...)
	driftedApproval.Grants[0], driftedApproval.Grants[1] = driftedApproval.Grants[1], driftedApproval.Grants[0]
	driftedRecord, err := protocol.EncodeAuthorityApproval(driftedApproval)
	if err != nil {
		t.Fatal(err)
	}
	driftedDigest, err := control.PutArtifact(ctx, "application/json", driftedRecord.CanonicalJSON)
	if err != nil {
		t.Fatal(err)
	}
	driftedAuthority := submissionInput
	driftedAuthority.AuthorityReceipt = protocol.Artifact{
		Ref: driftedDigest, MediaType: "application/json", Digest: driftedDigest,
	}
	if _, err := protocol.BuildSubmission(ctx, repository, control, driftedAuthority); err == nil {
		t.Fatal("approval with grants reordered from the exact plan was admitted")
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
		input.MeasuredChecks = append([]protocol.MeasuredCheck(nil), submissionInput.MeasuredChecks...)
		input.MeasuredChecks[0].Receipt = pointer
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
	if _, err := protocol.BuildSubmission(ctx, repository, control, changedDefinitionInput); err == nil {
		t.Fatal("definition outside the exact policy registry was admitted")
	}
	falseAuthorityRef := submissionInput
	falseAuthorityRef.AuthorityReceipt.Ref = "artifact:false"
	if _, err := protocol.BuildSubmission(ctx, repository, control, falseAuthorityRef); err == nil {
		t.Fatal("false non-CAS authority reference was admitted")
	}
	record, err := protocol.BuildSubmission(ctx, repository, control, submissionInput)
	if err != nil {
		t.Fatal(err)
	}
	if record.Kind != protocol.SubmissionSchemaVersion || !protocol.ValidDigest(record.Digest) ||
		protocol.CanonicalDigest(record.CanonicalJSON) != record.Digest {
		t.Fatalf("submission record = %#v", record)
	}
	var submission protocol.Submission
	if err := json.Unmarshal(record.CanonicalJSON, &submission); err != nil {
		t.Fatal(err)
	}
	contract, _ := plan.Work("work-1")
	if submission.DeliveryID != plan.DeliveryID() || submission.WorkID != submissionInput.WorkID ||
		submission.Attempt != submissionInput.Attempt || submission.PlanDigest != plan.Record().Digest ||
		submission.ContractDigest != contract.Digest() || len(submission.Checks) != 1 ||
		len(submission.Evidence) != 1 || submission.Checks[0].RunID != measuredReceipt.RunID ||
		submission.Checks[0].Receipt != produced.Receipt || submission.Evidence[0].Artifact != produced.Receipt {
		t.Fatalf("prepared submission projection = %#v", submission)
	}
	reencoded, err := protocol.EncodeSubmission(submission)
	if err != nil || reencoded.Digest != record.Digest ||
		!bytes.Equal(reencoded.CanonicalJSON, record.CanonicalJSON) {
		t.Fatalf("re-encoded submission = %#v, %v; want %#v", reencoded, err, record)
	}
	if len(record.CanonicalJSON) == 0 {
		t.Fatal("submission has empty canonical bytes")
	}
	badBinding := submissionInput
	badBinding.MeasuredChecks = append([]protocol.MeasuredCheck(nil), submissionInput.MeasuredChecks...)
	badBinding.MeasuredChecks[0].RunID = builder.RunID
	if _, err := protocol.BuildSubmission(ctx, repository, control, badBinding); err == nil {
		t.Fatal("builder-stamped journal result was admitted")
	}
	if _, err := protocol.BuildSubmission(ctx, repository, corruptArtifactReader{
		base: control, digest: produced.Receipt.Digest,
	}, submissionInput); err == nil {
		t.Fatal("changed receipt bytes were admitted")
	}
	if _, err := protocol.BuildSubmission(ctx, repository, corruptArtifactReader{
		base: control, digest: plan.Policy().Digest,
	}, submissionInput); err == nil {
		t.Fatal("changed exact policy bytes were admitted")
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
	definitionBytes, _ := protocol.EncodeCanonical(protocol.LocalCheckDefinition{
		SchemaVersion: protocol.LocalCheckDefinitionSchemaVersion, Argv: []string{"/usr/bin/false"},
		WorkingDirectory: ".", TimeoutSeconds: 10,
		Evidence: protocol.LocalEvidenceDefinition{ID: "evidence", AcceptanceIDs: []string{"AC1"}, Boundary: "component", Observed: "passed"},
	})
	digest, err := control.PutArtifact(ctx, "application/json", definitionBytes)
	if err != nil {
		t.Fatal(err)
	}
	runner := fakeRunner{completion: func(invocation executor.Invocation) executor.RawCompletion {
		return executor.RawCompletion{
			InvocationID: invocation.ID, RuntimeDigest: invocation.RuntimeDigest, WorkspaceDigest: invocation.WorkspaceDigest,
			WorkspaceAccess: executor.WorkspaceReadOnly, StartedAt: testCheckStart,
			CompletedAt: testCheckCompletion, ExitCode: 7, Stderr: []byte("failed\n"),
		}
	}}
	runtimeTree, _ := producerRuntimeTree(t, ctx, "/usr/bin/false")
	result, err := producer.RunLocalContentBound(ctx, runner, control, producer.Request{
		CheckID: "candidate", RunID: "check-run-7",
		Definition: protocol.Artifact{Ref: digest, MediaType: "application/json", Digest: digest},
		Repository: repository, Candidate: candidate, Workspace: checked,
	}, runtimeTree)
	if !errors.Is(err, producer.ErrCheckNotAdmitted) || result.Receipt.Digest == "" {
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
}

func TestContentBoundLocalCheckBindsObservedRuntime(t *testing.T) {
	ctx := context.Background()
	repository, candidate, checked := prepareProducerCandidate(t)
	control, err := store.Open(ctx, filepath.Join(t.TempDir(), "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = control.Close() })
	definition := protocol.LocalCheckDefinition{
		SchemaVersion: protocol.LocalCheckDefinitionSchemaVersion, Argv: []string{"/usr/bin/check"},
		WorkingDirectory: ".", TimeoutSeconds: 10,
		Evidence: protocol.LocalEvidenceDefinition{ID: "evidence", AcceptanceIDs: []string{"AC1"}, Boundary: "component", Observed: "passed"},
	}
	definitionBytes, err := protocol.EncodeCanonical(definition)
	if err != nil {
		t.Fatal(err)
	}
	definitionDigest, err := control.PutArtifact(ctx, "application/json", definitionBytes)
	if err != nil {
		t.Fatal(err)
	}
	runtimeTree, runtimeDigest := producerRuntimeTree(t, ctx, "/usr/bin/check")
	request := producer.Request{
		CheckID: "candidate", RunID: "content-check-run",
		Definition: protocol.Artifact{Ref: definitionDigest, MediaType: "application/json", Digest: definitionDigest},
		Repository: repository, Candidate: candidate, Workspace: checked,
	}
	runner := contentOnlyRunner{fakeRunner: fakeRunner{
		completion: func(invocation executor.Invocation) executor.RawCompletion {
			return executor.RawCompletion{
				InvocationID: invocation.ID, RuntimeDigest: invocation.RuntimeDigest,
				WorkspaceDigest: invocation.WorkspaceDigest, WorkspaceAccess: executor.WorkspaceReadOnly,
				StartedAt: testCheckStart, CompletedAt: testCheckCompletion, ExitCode: 0,
			}
		},
	}}
	result, err := producer.RunLocalContentBound(ctx, runner, control, request, runtimeTree)
	if err != nil || result.Receipt.Digest == "" {
		t.Fatalf("content-bound result = %#v, %v", result, err)
	}
	receipt := submissionReceipt(t, ctx, control, result.Receipt)
	_, environmentBytes, err := control.Artifact(ctx, receipt.Environment.Ref)
	if err != nil {
		t.Fatal(err)
	}
	environment, err := protocol.ParseLocalEnvironment(environmentBytes)
	if err != nil || environment.SchemaVersion != protocol.ContentEnvironmentSchemaVersion ||
		environment.RuntimeManifestDigest != runtimeDigest || environment.HermeticToolchain {
		t.Fatalf("content environment = %#v, %v", environment, err)
	}
	for name, mutate := range map[string]func(*protocol.LocalEnvironment){
		"missing digest": func(value *protocol.LocalEnvironment) { value.RuntimeManifestDigest = "" },
		"v1 with runtime digest": func(value *protocol.LocalEnvironment) {
			value.SchemaVersion = protocol.LocalEnvironmentSchemaVersion
		},
		"hermetic overclaim": func(value *protocol.LocalEnvironment) { value.HermeticToolchain = true },
	} {
		t.Run(name, func(t *testing.T) {
			changed := environment
			mutate(&changed)
			encoded, err := protocol.EncodeCanonical(changed)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := protocol.ParseLocalEnvironment(encoded); err == nil {
				t.Fatal("invalid content environment was accepted")
			}
		})
	}
	plan := putSubmissionPlan(t, ctx, control, request.Definition)
	record, err := protocol.BuildSubmission(ctx, repository, control, protocol.SubmissionInput{
		Attempt: 1, CreatedAt: testCheckCompletion.Add(time.Second), Plan: plan, WorkID: "work-1",
		AuthorityReceipt: putAuthorityApproval(t, ctx, control, plan),
		Builder: protocol.BuilderRun{
			RunID: "content-builder-run", Agent: "codex", StartedAt: formatTime(testBuilderStart),
			CompletedAt: formatTime(testBuilderCompletion),
		},
		Candidate: candidate, MeasuredChecks: []protocol.MeasuredCheck{{
			RunID: receipt.RunID, RuntimeManifestDigest: runtimeDigest, Receipt: result.Receipt,
		}},
	})
	if err != nil || record.Digest == "" {
		t.Fatalf("content-bound submission = %#v, %v", record, err)
	}

	badRunner := contentOnlyRunner{fakeRunner: fakeRunner{
		completion: func(invocation executor.Invocation) executor.RawCompletion {
			return executor.RawCompletion{
				InvocationID: invocation.ID, WorkspaceDigest: invocation.WorkspaceDigest,
				WorkspaceAccess: executor.WorkspaceReadOnly, StartedAt: testCheckStart,
				CompletedAt: testCheckCompletion, ExitCode: 0,
			}
		},
	}}
	request.RunID = "content-check-mismatch"
	if result, err := producer.RunLocalContentBound(ctx, badRunner, control, request, runtimeTree); err == nil ||
		result.Receipt.Digest != "" || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("runtime-mismatched result = %#v, %v", result, err)
	}
	selectorRunner := contentOnlyRunner{fakeRunner: fakeRunner{
		completion: func(invocation executor.Invocation) executor.RawCompletion {
			return executor.RawCompletion{
				InvocationID: invocation.ID, RuntimeDigest: invocation.RuntimeDigest,
				WorkspaceDigest: invocation.WorkspaceDigest, WorkspaceAccess: executor.WorkspaceReadOnly,
				ExecutableInput: "agent", StartedAt: testCheckStart,
				CompletedAt: testCheckCompletion, ExitCode: 0,
			}
		},
	}}
	request.RunID = "content-check-selector-mismatch"
	if result, err := producer.RunLocalContentBound(ctx, selectorRunner, control, request, runtimeTree); err == nil ||
		result.Receipt.Digest != "" || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("selector-mismatched result = %#v, %v", result, err)
	}
	nonPassRunner := contentOnlyRunner{fakeRunner: fakeRunner{
		completion: func(invocation executor.Invocation) executor.RawCompletion {
			return executor.RawCompletion{
				InvocationID: invocation.ID, RuntimeDigest: invocation.RuntimeDigest,
				WorkspaceDigest: invocation.WorkspaceDigest, WorkspaceAccess: executor.WorkspaceReadOnly,
				StartedAt: testCheckStart, CompletedAt: testCheckCompletion, ExitCode: 7,
			}
		},
	}}
	request.RunID = "content-check-non-pass"
	nonPass, err := producer.RunLocalContentBound(ctx, nonPassRunner, control, request, runtimeTree)
	if !errors.Is(err, producer.ErrCheckNotAdmitted) || nonPass.Receipt.Digest == "" {
		t.Fatalf("content non-pass = %#v, %v", nonPass, err)
	}
	_, receiptBytes, err := control.Artifact(ctx, nonPass.Receipt.Digest)
	if err != nil {
		t.Fatal(err)
	}
	nonPassReceipt, err := protocol.ParseLocalCheckReceipt(receiptBytes)
	if err != nil {
		t.Fatal(err)
	}
	_, nonPassEnvironmentBytes, err := control.Artifact(ctx, nonPassReceipt.Environment.Ref)
	if err != nil {
		t.Fatal(err)
	}
	nonPassEnvironment, err := protocol.ParseLocalEnvironment(nonPassEnvironmentBytes)
	if err != nil || nonPassEnvironment.SchemaVersion != protocol.ContentEnvironmentSchemaVersion ||
		nonPassEnvironment.RuntimeManifestDigest != runtimeDigest {
		t.Fatalf("content non-pass environment = %#v, %v", nonPassEnvironment, err)
	}
}

func putSubmissionPlan(
	t *testing.T,
	ctx context.Context,
	control *store.Store,
	definition protocol.Artifact,
) protocol.ExactPlan {
	t.Helper()
	policyBytes, err := protocol.EncodeCanonical(map[string]any{
		"schema_version": protocol.AssurancePolicySchemaVersion,
		"policy_id":      "standard",
		"checks": []any{map[string]any{
			"id": "candidate",
			"definition": map[string]any{
				"ref": "policy/checks/candidate.json", "media_type": "application/json", "digest": definition.Digest,
			},
		}},
		"packs": []any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	policyDigest, err := control.PutArtifact(ctx, "application/json", policyBytes)
	if err != nil {
		t.Fatal(err)
	}
	planBytes, err := protocol.EncodeCanonical(map[string]any{
		"schema_version": "delivery-plan-v1", "delivery_id": "delivery-1",
		"outcome": "Produce the exact candidate.", "created_at": "2026-07-19T00:00:00Z",
		"assurance_policy": map[string]any{"ref": "policy:standard", "digest": policyDigest},
		"target":           map[string]any{"repository": "repo-01", "ref": "refs/heads/main"},
		"authority": map[string]any{
			"ref": "authority-source",
			"grants": []any{
				map[string]any{"action": "inspect", "target": "workspace"},
				map[string]any{"action": "edit", "target": "workspace"},
				map[string]any{"action": "execute", "target": "workspace"},
				map[string]any{"action": "commit", "target": "workspace"},
			},
		},
		"work": []any{map[string]any{
			"id": "work-1", "outcome": "Produce the exact candidate.",
			"scope": map[string]any{"include": []string{"."}, "exclude": []string{}},
			"acceptance": []any{map[string]any{
				"id": "AC1", "criterion": "The registered candidate check passes.", "evidence_level": "component",
			}},
			"depends_on": []string{},
			"assurance":  map[string]any{"profile": "standard", "packs": []string{}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := protocol.ParseDeliveryPlan(planBytes)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func putAuthorityApproval(
	t *testing.T,
	ctx context.Context,
	control *store.Store,
	plan protocol.ExactPlan,
) protocol.Artifact {
	t.Helper()
	authority := plan.Authority()
	grants := make([]json.RawMessage, 0, len(authority.Grants))
	for _, grant := range authority.Grants {
		grants = append(grants, json.RawMessage(grant.CanonicalJSON()))
	}
	target := plan.Target()
	receipt, err := protocol.EncodeAuthorityApproval(protocol.AuthorityApproval{
		SchemaVersion: protocol.ControlReceiptSchemaVersion, Kind: protocol.AuthorityApprovalKind,
		ReceiptID: "authority-1", PlanDigest: plan.Record().Digest, AuthorityDigest: authority.Digest,
		SourceRef: authority.SourceRef, SourceDigest: testSourceDigest, Grants: grants,
		Repository: target.Repository, TargetRef: target.Ref, AuthorizerRef: "identity:test",
		ApprovedAt: formatTime(testBuilderStart.Add(-time.Second)),
	})
	if err != nil {
		t.Fatal(err)
	}
	digest, err := control.PutArtifact(ctx, "application/json", receipt.CanonicalJSON)
	if err != nil {
		t.Fatal(err)
	}
	return protocol.Artifact{Ref: digest, MediaType: "application/json", Digest: digest}
}

func fixedDigest(character string) string { return "sha256:" + strings.Repeat(character, 64) }

func formatTime(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }

func producerRuntimeTree(t *testing.T, ctx context.Context, executable string) (executor.RuntimeTree, string) {
	t.Helper()
	runtimeSource := t.TempDir()
	runtimeExecutable := filepath.Join(runtimeSource, strings.TrimPrefix(executable, "/usr/"))
	writeProducerFile(t, runtimeExecutable, []byte("runtime"))
	if err := os.Chmod(runtimeExecutable, 0o755); err != nil {
		t.Fatal(err)
	}
	runtimeDigest, _, err := workspace.Measure(ctx, runtimeSource, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	runtimeTree, err := executor.NewRuntimeTree(runtimeSource, runtimeDigest, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	return runtimeTree, runtimeDigest
}

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
			if _, err := protocol.ParseLocalCheckDefinition([]byte(input)); err == nil {
				t.Fatal("invalid definition was accepted")
			}
		})
	}
	arguments := make([]string, 257)
	arguments[0] = "/usr/bin/true"
	for index := 1; index < len(arguments); index++ {
		arguments[index] = "argument"
	}
	contents, err := protocol.EncodeCanonical(protocol.LocalCheckDefinition{
		SchemaVersion: protocol.LocalCheckDefinitionSchemaVersion, Argv: arguments,
		WorkingDirectory: ".", TimeoutSeconds: 1,
		Evidence: protocol.LocalEvidenceDefinition{ID: "e", AcceptanceIDs: []string{"AC1"}, Boundary: "component", Observed: "ok"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := protocol.ParseLocalCheckDefinition(contents); err == nil || !strings.Contains(err.Error(), "256") {
		t.Fatalf("oversized argv error = %v", err)
	}
}

func TestLocalCheckDefinitionJSONIsStable(t *testing.T) {
	t.Parallel()
	definition := protocol.LocalCheckDefinition{
		SchemaVersion: protocol.LocalCheckDefinitionSchemaVersion, Argv: []string{"/usr/bin/true"},
		WorkingDirectory: ".", TimeoutSeconds: 1,
		Evidence: protocol.LocalEvidenceDefinition{ID: "e", AcceptanceIDs: []string{"AC1"}, Boundary: "component", Observed: "ok"},
	}
	contents, err := json.Marshal(definition)
	if err != nil || !json.Valid(contents) {
		t.Fatalf("definition JSON = %q, %v", contents, err)
	}
}
