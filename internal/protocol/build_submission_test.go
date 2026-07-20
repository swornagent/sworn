package protocol

import (
	"reflect"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/repo"
)

func TestProjectMeasuredReceiptDerivesPolicyCheckAndEvidence(t *testing.T) {
	t.Parallel()

	measured, receipt, requirement, candidate := measuredReceiptFixture()
	check, evidence, err := projectMeasuredReceipt(measured, receipt, requirement, candidate)
	if err != nil {
		t.Fatal(err)
	}
	exitCode := 0
	wantCheck := Check{
		ID: requirement.CheckID, Outcome: "pass", RunID: measured.RunID,
		CandidateTree: candidate.Tree, Environment: receipt.Environment,
		StartedAt: receipt.StartedAt, CompletedAt: receipt.CompletedAt,
		ExitCode: &exitCode, Receipt: measured.Receipt,
	}
	wantEvidence := Evidence{
		ID: "evidence-1", AcceptanceIDs: []string{"acceptance-1"}, Kind: "test",
		Boundary: "assembled", Environment: receipt.Environment, UsesMocks: false,
		ProducerRunID: measured.RunID, CandidateTree: candidate.Tree,
		CapturedAt: receipt.CompletedAt, Artifact: measured.Receipt,
		Observed: "the exact policy check passed",
	}
	if !reflect.DeepEqual(check, wantCheck) || !reflect.DeepEqual(evidence, wantEvidence) {
		t.Fatalf("projection = %#v, %#v; want %#v, %#v", check, evidence, wantCheck, wantEvidence)
	}

	receipt.Argv[0] = "changed-after-projection"
	requirement.definition.Evidence.AcceptanceIDs[0] = "changed-after-projection"
	if check.ID != "check-1" || check.ExitCode == nil || *check.ExitCode != 0 ||
		evidence.AcceptanceIDs[0] != "acceptance-1" {
		t.Fatal("derived projection retained mutable caller input")
	}
}

func TestProjectMeasuredReceiptRejectsFalseJournalAndPolicyBindings(t *testing.T) {
	t.Parallel()

	tests := map[string]func(*MeasuredCheck, *LocalCheckReceipt, *LocalCheckRequirement, *repo.Candidate){
		"non-pass": func(_ *MeasuredCheck, receipt *LocalCheckReceipt, _ *LocalCheckRequirement, _ *repo.Candidate) {
			receipt.Outcome = "not_admitted"
		},
		"wrong policy order": func(_ *MeasuredCheck, receipt *LocalCheckReceipt, _ *LocalCheckRequirement, _ *repo.Candidate) {
			receipt.CheckID = "check-2"
		},
		"wrong journal run": func(measured *MeasuredCheck, _ *LocalCheckReceipt, _ *LocalCheckRequirement, _ *repo.Candidate) {
			measured.RunID = "effect-2"
		},
		"wrong definition": func(_ *MeasuredCheck, receipt *LocalCheckReceipt, _ *LocalCheckRequirement, _ *repo.Candidate) {
			receipt.Definition = jsonCAS(testProtocolDigest("f"))
		},
		"wrong repository": func(_ *MeasuredCheck, receipt *LocalCheckReceipt, _ *LocalCheckRequirement, _ *repo.Candidate) {
			receipt.Candidate.Repository = "different"
		},
		"wrong commit": func(_ *MeasuredCheck, receipt *LocalCheckReceipt, _ *LocalCheckRequirement, _ *repo.Candidate) {
			receipt.Candidate.Commit = testSubmissionOID("d")
		},
		"wrong tree": func(_ *MeasuredCheck, receipt *LocalCheckReceipt, _ *LocalCheckRequirement, _ *repo.Candidate) {
			receipt.Candidate.Tree = testSubmissionOID("e")
		},
		"wrong directory": func(_ *MeasuredCheck, receipt *LocalCheckReceipt, _ *LocalCheckRequirement, _ *repo.Candidate) {
			receipt.WorkingDirectory = "subdir"
		},
		"wrong argv": func(_ *MeasuredCheck, receipt *LocalCheckReceipt, _ *LocalCheckRequirement, _ *repo.Candidate) {
			receipt.Argv = []string{"go", "test", "./different"}
		},
		"wrong timeout": func(_ *MeasuredCheck, receipt *LocalCheckReceipt, _ *LocalCheckRequirement, _ *repo.Candidate) {
			receipt.TimeoutSeconds++
		},
		"wrong exit": func(_ *MeasuredCheck, receipt *LocalCheckReceipt, _ *LocalCheckRequirement, _ *repo.Candidate) {
			receipt.ExitCode = 1
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			measured, receipt, requirement, candidate := measuredReceiptFixture()
			mutate(&measured, &receipt, &requirement, &candidate)
			if _, _, err := projectMeasuredReceipt(measured, receipt, requirement, candidate); err == nil {
				t.Fatal("false measured binding was accepted")
			}
		})
	}
}

func measuredReceiptFixture() (MeasuredCheck, LocalCheckReceipt, LocalCheckRequirement, repo.Candidate) {
	candidate := repo.Candidate{
		RepositoryID: "repo-1", Commit: testSubmissionOID("a"), Tree: testSubmissionOID("b"),
	}
	receiptPointer := Artifact{
		Ref: testProtocolDigest("c"), MediaType: "application/vnd.sworn.local-check-receipt+json",
		Digest: testProtocolDigest("c"),
	}
	definition := LocalCheckDefinition{
		SchemaVersion: LocalCheckDefinitionSchemaVersion, Argv: []string{"go", "test", "./..."},
		WorkingDirectory: ".", TimeoutSeconds: 60,
		Evidence: LocalEvidenceDefinition{
			ID: "evidence-1", AcceptanceIDs: []string{"acceptance-1"}, Boundary: "assembled",
			Observed: "the exact policy check passed",
		},
	}
	requirement := LocalCheckRequirement{
		CheckID: "check-1", Definition: jsonCAS(testProtocolDigest("d")), definition: definition,
	}
	receipt := LocalCheckReceipt{
		CheckID: requirement.CheckID, RunID: "effect-1", Definition: requirement.Definition,
		Candidate: CandidatePoint{
			Repository: candidate.RepositoryID, Commit: candidate.Commit, Tree: candidate.Tree,
		},
		Environment:      Environment{Kind: "local", Ref: testProtocolDigest("e")},
		WorkingDirectory: definition.WorkingDirectory, Argv: append([]string(nil), definition.Argv...),
		TimeoutSeconds: definition.TimeoutSeconds, StartedAt: "2026-07-20T00:01:00Z",
		CompletedAt: "2026-07-20T00:02:00Z", Outcome: "pass",
	}
	measured := MeasuredCheck{
		RunID: receipt.RunID, RuntimeManifestDigest: testProtocolDigest("f"), Receipt: receiptPointer,
	}
	return measured, receipt, requirement, candidate
}

func testSubmissionOID(character string) string { return strings.Repeat(character, 40) }
