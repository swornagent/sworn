package protocol

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/executor"
	"github.com/swornagent/sworn/internal/repo"
)

func TestResolveExactVerifierReviewClosesEverySubmissionArtifact(t *testing.T) {
	t.Parallel()
	plan, submission, artifacts, removable := exactVerifierReviewFixture(t)
	review, err := ResolveExactVerifierReview(context.Background(), artifacts, plan, submission)
	if err != nil {
		t.Fatalf("resolve exact verifier review: %v", err)
	}
	inputs := review.Inputs()
	if len(inputs) != 3 || inputs[0].Name != "review-authority" || inputs[1].Name != "review-check-01" ||
		inputs[2].Name != "review-policy" {
		t.Fatalf("exact verifier review inputs = %#v", inputs)
	}
	for _, input := range inputs {
		if input.Digest != RawDigest(input.Contents) {
			t.Fatalf("review input %q digest = %q", input.Name, input.Digest)
		}
	}
	inputs[0].Contents[0] ^= 0xff
	if review.Inputs()[0].Digest != RawDigest(review.Inputs()[0].Contents) {
		t.Fatal("review inputs were mutated through a returned view")
	}
	for label, digest := range removable {
		artifact := artifacts[digest]
		delete(artifacts, digest)
		if _, err := ResolveExactVerifierReview(context.Background(), artifacts, plan, submission); err == nil {
			t.Fatalf("missing %s artifact was accepted", label)
		}
		artifacts[digest] = artifact
	}
	if _, err := ResolveExactVerifierReview(context.Background(), nil, plan, submission); err == nil {
		t.Fatal("nil artifact reader was accepted")
	}
}

func exactVerifierReviewFixture(
	t testing.TB,
) (ExactPlan, ExactSubmission, exactCheckArtifacts, map[string]string) {
	t.Helper()
	artifacts := exactCheckArtifacts{}
	definition := artifacts.putJSON(t, localCheckDefinition("evidence-1", "assembled", "AC1"))
	policy := artifacts.putJSON(t, map[string]any{
		"schema_version": AssurancePolicySchemaVersion,
		"policy_id":      "standard",
		"checks": []any{map[string]any{
			"id": "test",
			"definition": map[string]any{
				"ref": "policy/checks/test.json", "media_type": "application/json", "digest": definition.Digest,
			},
		}},
		"packs": []any{},
	})
	grantValues := []map[string]any{
		{"action": "inspect", "target": "workspace"},
		{"action": "edit", "target": "workspace"},
		{"action": "execute", "target": "workspace"},
		{"action": "commit", "target": "workspace"},
	}
	planBytes, err := EncodeCanonical(map[string]any{
		"schema_version": DeliveryPlanSchemaVersion,
		"delivery_id":    "delivery-1",
		"outcome":        "Produce the exact candidate.",
		"created_at":     "2026-07-20T00:00:00Z",
		"assurance_policy": map[string]any{
			"ref": "policy:standard", "digest": policy.Digest,
		},
		"target": map[string]any{"repository": "repo-1", "ref": "refs/heads/main"},
		"authority": map[string]any{
			"ref": "authority-source", "grants": grantValues,
		},
		"work": []any{map[string]any{
			"id": "work-1", "outcome": "Produce the exact candidate.",
			"scope": map[string]any{"include": []string{"."}, "exclude": []string{}},
			"acceptance": []any{map[string]any{
				"id": "AC1", "criterion": "The exact candidate is proven.", "evidence_level": "assembled",
			}},
			"depends_on": []string{},
			"assurance":  map[string]any{"profile": "standard", "packs": []string{}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := ParseDeliveryPlan(planBytes)
	if err != nil {
		t.Fatal(err)
	}
	grants := make([]json.RawMessage, len(plan.Authority().Grants))
	for index, grant := range plan.Authority().Grants {
		grants[index] = grant.CanonicalJSON()
	}
	authority, err := EncodeAuthorityApproval(AuthorityApproval{
		SchemaVersion:   ControlReceiptSchemaVersion,
		Kind:            AuthorityApprovalKind,
		ReceiptID:       "authority-1",
		PlanDigest:      plan.Record().Digest,
		AuthorityDigest: plan.Authority().Digest,
		SourceRef:       plan.Authority().SourceRef,
		SourceDigest:    testProtocolDigest("8"),
		Grants:          grants,
		Repository:      plan.Target().Repository,
		TargetRef:       plan.Target().Ref,
		AuthorizerRef:   "identity:test",
		ApprovedAt:      "2026-07-20T00:00:01Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	authorityPointer := putVerifierReviewArtifact(
		artifacts, "application/json", authority.CanonicalJSON,
	)

	snapshotDigest, err := SnapshotDigest()
	if err != nil {
		t.Fatal(err)
	}
	environmentBytes, err := EncodeCanonical(LocalEnvironment{
		SchemaVersion:          ContentEnvironmentSchemaVersion,
		ProtocolSnapshotDigest: "sha256:" + snapshotDigest,
		EngineRuntime:          "go-test",
		OS:                     "linux",
		Architecture:           "amd64",
		Executor: LocalExecutorProbe{
			BubblewrapVersion: "1.0", SystemdVersion: "257", CgroupV2: true,
			UserManager: "running", Controllers: []string{"cpu", "memory", "pids"},
		},
		ExecutorPolicyVersion: executor.ContainmentPolicyVersion,
		Limits: LocalExecutionLimits{
			RuntimeNanoseconds: 1, MemoryBytes: 1, Tasks: 1, CPUPercent: 1,
			FileBytes: 1, TempBytes: 1, HomeBytes: 1, InputBytes: 1,
			WorkspaceBytes: 1, StdoutBytes: 1, StderrBytes: 1,
		},
		RuntimeTrustRoot: "/usr", RuntimeManifestDigest: testProtocolDigest("9"),
		HermeticToolchain: false, WorkspaceAccess: "read_only", Network: "none",
	})
	if err != nil {
		t.Fatal(err)
	}
	environment := putVerifierReviewArtifact(artifacts, LocalEnvironmentMediaType, environmentBytes)
	stdout := putVerifierReviewArtifact(artifacts, "application/octet-stream", []byte("ok\n"))
	stderr := putVerifierReviewArtifact(artifacts, "application/octet-stream", nil)
	candidate := repo.Candidate{
		RepositoryID: "repo-1",
		TargetRef:    "refs/heads/main",
		BaseCommit:   strings.Repeat("a", 40),
		Commit:       strings.Repeat("b", 40),
		Tree:         strings.Repeat("c", 40),
	}
	receiptRecord, err := EncodeLocalCheckReceipt(LocalCheckReceipt{
		SchemaVersion: LocalCheckReceiptSchemaVersion,
		CheckID:       "test",
		RunID:         "check-1",
		Definition:    definition,
		Candidate: CandidatePoint{
			Repository: candidate.RepositoryID, Commit: candidate.Commit, Tree: candidate.Tree,
		},
		WorkspaceDigest:  testProtocolDigest("a"),
		Environment:      Environment{Kind: "local", Ref: environment.Digest},
		WorkspaceAccess:  "read_only",
		WorkingDirectory: ".",
		Argv:             []string{"/usr/bin/true"},
		TimeoutSeconds:   30,
		Network:          "none",
		StartedAt:        "2026-07-20T00:02:00Z",
		CompletedAt:      "2026-07-20T00:03:00Z",
		ExitCode:         0,
		Outcome:          "pass",
		Stdout: CapturedArtifact{
			Ref: stdout.Ref, MediaType: stdout.MediaType, Digest: stdout.Digest, Size: 3,
		},
		Stderr: CapturedArtifact{
			Ref: stderr.Ref, MediaType: stderr.MediaType, Digest: stderr.Digest, Size: 0,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	receipt := putVerifierReviewArtifact(
		artifacts, "application/vnd.sworn.local-check-receipt+json", receiptRecord.CanonicalJSON,
	)
	selection, err := ResolveExactLocalChecks(context.Background(), artifacts, plan, "work-1")
	if err != nil {
		t.Fatal(err)
	}
	requirement := selection.requirements[0]
	parsedReceipt, err := ParseLocalCheckReceipt(receiptRecord.CanonicalJSON)
	if err != nil {
		t.Fatal(err)
	}
	check, evidence, err := projectMeasuredReceipt(
		MeasuredCheck{RunID: "check-1", Receipt: receipt}, parsedReceipt, requirement, candidate,
	)
	if err != nil {
		t.Fatal(err)
	}
	contract, _ := plan.Work("work-1")
	submissionID, err := SubmissionID("delivery-1", "work-1", 1)
	if err != nil {
		t.Fatal(err)
	}
	submissionRecord, err := EncodeSubmission(Submission{
		SchemaVersion:    SubmissionSchemaVersion,
		SubmissionID:     submissionID,
		DeliveryID:       "delivery-1",
		WorkID:           "work-1",
		Attempt:          1,
		CreatedAt:        "2026-07-20T00:04:00Z",
		PlanDigest:       plan.Record().Digest,
		ContractDigest:   contract.Digest(),
		AuthorityReceipt: authorityPointer,
		Builder: BuilderRun{
			RunID: "builder-1", Agent: "test", StartedAt: "2026-07-20T00:00:02Z",
			CompletedAt: "2026-07-20T00:01:00Z",
		},
		Base: GitPoint{
			Repository: candidate.RepositoryID, Ref: candidate.TargetRef, Commit: candidate.BaseCommit,
		},
		Candidate: CandidatePoint{
			Repository: candidate.RepositoryID, Commit: candidate.Commit, Tree: candidate.Tree,
		},
		Assurance: Assurance{
			Profile: "standard", Packs: []string{}, PolicyRef: plan.Policy().Ref, PolicyDigest: plan.Policy().Digest,
		},
		ChangedPaths: []string{"file.txt"},
		Checks:       []Check{check},
		Evidence:     []Evidence{evidence},
	})
	if err != nil {
		t.Fatal(err)
	}
	submission, err := ParseSubmission(submissionRecord.CanonicalJSON)
	if err != nil {
		t.Fatal(err)
	}
	return plan, submission, artifacts, map[string]string{
		"policy": policy.Digest, "definition": definition.Digest, "authority": authorityPointer.Digest,
		"receipt": receipt.Digest, "environment": environment.Digest,
		"stdout": stdout.Digest, "stderr": stderr.Digest,
	}
}

func putVerifierReviewArtifact(
	artifacts exactCheckArtifacts,
	mediaType string,
	contents []byte,
) Artifact {
	digest := RawDigest(contents)
	artifacts[digest] = exactCheckArtifact{mediaType: mediaType, contents: append([]byte(nil), contents...)}
	return Artifact{Ref: digest, MediaType: mediaType, Digest: digest}
}
