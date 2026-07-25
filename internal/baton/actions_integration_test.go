package baton

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/gitx"
)

const literalTestGit = "/usr/bin/git"

type actionGoldenEvent struct {
	Name  string         `json:"name"`
	First map[string]any `json:"first"`
	Retry map[string]any `json:"retry"`
}

type actionGoldenFormat struct {
	ObjectFormat     string              `json:"object_format"`
	OIDHexLength     int                 `json:"oid_hex_length"`
	BaseCommit       string              `json:"base_commit"`
	PlanDigest       string              `json:"plan_digest"`
	ApprovalDigest   string              `json:"approval_digest"`
	ProductCandidate string              `json:"product_candidate"`
	ProductTree      string              `json:"product_tree"`
	Events           []actionGoldenEvent `json:"events"`
	FinalRefs        []struct {
		Ref  string  `json:"ref"`
		Head *string `json:"head"`
	} `json:"final_refs"`
}

type actionGoldenCorpus struct {
	Schema   string                `json:"schema"`
	Profile  string                `json:"profile"`
	Formats  []actionGoldenFormat  `json:"formats"`
	Rebounds []actionGoldenRebound `json:"rebounds"`
}

type actionGoldenRebound struct {
	ObjectFormat       string         `json:"object_format"`
	PreviousPlanDigest string         `json:"previous_plan_digest"`
	RevisedPlanDigest  string         `json:"revised_plan_digest"`
	First              map[string]any `json:"first"`
	Retry              map[string]any `json:"retry"`
}

func loadActionGoldenCorpus(t *testing.T) actionGoldenCorpus {
	t.Helper()
	file, err := os.Open(filepath.Join("..", "..", "tools", "batongolden", "testdata", "corpus", "actions.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.UseNumber()
	var corpus actionGoldenCorpus
	if err := decoder.Decode(&corpus); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		t.Fatalf("action golden has trailing JSON: %v", err)
	}
	if corpus.Schema != "sworn.baton-golden-actions/v1" || corpus.Profile != "autonomous" {
		t.Fatalf("foreign action golden identity: %#v", corpus)
	}
	return corpus
}

func loadActionGolden(t *testing.T, format gitx.ObjectFormat) actionGoldenFormat {
	t.Helper()
	corpus := loadActionGoldenCorpus(t)
	for _, candidate := range corpus.Formats {
		if candidate.ObjectFormat == string(format) {
			return candidate
		}
	}
	t.Fatalf("action golden lacks %s", format)
	return actionGoldenFormat{}
}

func loadReboundGolden(t *testing.T, format gitx.ObjectFormat) actionGoldenRebound {
	t.Helper()
	corpus := loadActionGoldenCorpus(t)
	for _, candidate := range corpus.Rebounds {
		if candidate.ObjectFormat == string(format) {
			return candidate
		}
	}
	t.Fatalf("rebound golden lacks %s", format)
	return actionGoldenRebound{}
}

func fixturePlan(t *testing.T) Plan {
	t.Helper()
	raw := []byte("```baton-plan-v1\n" + `{
  "schema_version": "baton.plan/v1",
  "release": "demo-v1",
  "repository": "example/sworn",
  "target_ref": "refs/heads/release/v0.3.0",
  "release_ref": "refs/heads/release-wt/demo-v1",
  "record_root": ".baton/releases",
  "approval_ref": "test://approval/demo-v1",
  "tracks": [
    {
      "id": "T1",
      "ref": "refs/heads/track/demo-v1/T1",
      "depends_on": [],
      "touch_surfaces": ["product.txt"],
      "work": [
        {
          "id": "W1",
          "outcome": "deliver product",
          "scope": {"include": ["product.txt"], "exclude": []},
          "acceptance": [{"id": "A1", "text": "product is delivered"}],
          "checks": ["go test ./..."],
          "constraints": ["deterministic"],
          "depends_on": []
        }
      ]
    }
  ]
}
` + "```\n\n# Demo\n")
	plan, err := ParsePlan(raw)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func runFixtureGit(t *testing.T, repository string, stdin []byte, args ...string) string {
	t.Helper()
	command := exec.Command(literalTestGit, append([]string{"-C", repository}, args...)...)
	command.Env = append(os.Environ(),
		"LANG=C", "LC_ALL=C",
		"GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid",
		"GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid",
		"GIT_AUTHOR_DATE=@1000000000 +0000", "GIT_COMMITTER_DATE=@1000000000 +0000",
	)
	command.Stdin = bytes.NewReader(stdin)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func fixtureRepository(t *testing.T, format gitx.ObjectFormat) (*gitx.Repository, string) {
	t.Helper()
	directory := t.TempDir()
	command := exec.Command(literalTestGit, "init", "--quiet", "--initial-branch=main", "--object-format="+string(format), directory)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, output)
	}
	if err := os.WriteFile(filepath.Join(directory, "product.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runFixtureGit(t, directory, nil, "add", "--", "product.txt")
	runFixtureGit(t, directory, nil, "commit", "--quiet", "-m", "base")
	head := runFixtureGit(t, directory, nil, "rev-parse", "HEAD")
	runFixtureGit(t, directory, nil, "update-ref", "refs/heads/release/v0.3.0", head)
	repository, err := gitx.Open(directory, literalTestGit)
	if err != nil {
		t.Fatal(err)
	}
	return repository, head
}

func mustSnapshot(t *testing.T, plan Plan, repository Repository) Snapshot {
	t.Helper()
	reader, err := NewReader(plan, repository)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := reader.Capture()
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func TestCompleteSevenActionLoopAndExactRetries(t *testing.T) {
	for _, format := range []gitx.ObjectFormat{gitx.SHA1, gitx.SHA256} {
		format := format
		t.Run(string(format), func(t *testing.T) {
			t.Parallel()
			plan := fixturePlan(t)
			mechanical, targetHead := fixtureRepository(t, format)
			golden := loadActionGolden(t, format)
			if golden.OIDHexLength != len(targetHead) || golden.BaseCommit != targetHead ||
				golden.PlanDigest != plan.Digest() {
				t.Fatalf("fixture does not match oracle: golden=%#v target=%s plan=%s", golden, targetHead, plan.Digest())
			}
			eventIndex := 0
			assertGoldenEvent := func(name string, first, retry Receipt) {
				t.Helper()
				if eventIndex >= len(golden.Events) {
					t.Fatalf("oracle lacks event %s", name)
				}
				expected := golden.Events[eventIndex]
				eventIndex++
				if expected.Name != name || !reflect.DeepEqual(first.Data(), expected.First) ||
					!reflect.DeepEqual(retry.Data(), expected.Retry) {
					t.Fatalf(
						"event %s differs from oracle\nfirst=%#v\nwant=%#v\nretry=%#v\nwant=%#v",
						name, first.Data(), expected.First, retry.Data(), expected.Retry,
					)
				}
			}
			repository := UseGitRepository(mechanical)
			approvalBytes := []byte("approved demo-v1\n")
			approvalDigest := DigestBytes(approvalBytes)
			if golden.ApprovalDigest != approvalDigest {
				t.Fatalf("approval digest = %s, oracle = %s", approvalDigest, golden.ApprovalDigest)
			}
			dispatches := make(map[string][]byte)
			resolver := func(request EvidenceRequest) (Evidence, error) {
				switch request.Kind {
				case "approval":
					return Evidence{
						Bytes: append([]byte(nil), approvalBytes...),
						Provenance: EvidenceProvenance{
							Kind: "approval", Ref: request.Ref, Protected: true, Decision: "approved",
							PlanDigest: request.PlanDigest, AuthorizerIsolated: true, DeliveryWritable: false,
						},
					}, nil
				case "verifier_dispatch":
					body := dispatches[request.Ref]
					if body == nil {
						return Evidence{}, fmt.Errorf("unknown dispatch %s", request.Ref)
					}
					return Evidence{
						Bytes: append([]byte(nil), body...),
						Provenance: EvidenceProvenance{
							Kind: "verifier_dispatch", Ref: request.Ref, Protected: true,
							Role: "verifier", FreshContext: true, ReadOnly: true, EngineControlled: true,
							Invocation: request.Invocation, PlanDigest: request.PlanDigest,
							ProofDigest: request.ProofDigest, CandidateCommit: request.CandidateCommit,
							ProductTree: request.ProductTree,
						},
					}, nil
				default:
					return Evidence{}, fmt.Errorf("unknown evidence kind %s", request.Kind)
				}
			}
			inertness := func(request InertnessRequest) (InertnessDecision, error) {
				return InertnessDecision{
					Repository: request.Repository, RecordRoot: request.RecordRoot,
					Commit: request.Commit, Decision: "inert", Consumed: false,
				}, nil
			}
			actions, err := NewActions(plan, Autonomous, repository, resolver, inertness)
			if err != nil {
				t.Fatal(err)
			}

			receipt, err := actions.InstallApprovedPlan(approvalDigest)
			if err != nil {
				t.Fatal(err)
			}
			if receipt.Data()["changed"] != true {
				t.Fatalf("install receipt = %#v", receipt.Data())
			}
			retry, err := actions.InstallApprovedPlan(approvalDigest)
			if err != nil || retry.Data()["changed"] != false {
				t.Fatalf("install retry = %#v, %v", retry.Data(), err)
			}
			assertGoldenEvent("installApprovedPlan", receipt, retry)

			receipt, err = actions.MaterializeTrack("T1")
			if err != nil {
				t.Fatal(err)
			}
			retry, err = actions.MaterializeTrack("T1")
			if err != nil || retry.Data()["changed"] != false {
				t.Fatalf("materialize retry = %#v, %v", retry.Data(), err)
			}
			assertGoldenEvent("materializeTrack", receipt, retry)
			snapshot := mustSnapshot(t, plan, repository)
			materialized, err := snapshot.SelectWork("W1")
			if err != nil {
				t.Fatal(err)
			}
			if materialized.Source != "owner" {
				t.Fatalf("materialized source = %s", materialized.Source)
			}

			designBytes := []byte("# Design\n\nExact.\n")
			designView := materialized.Status.View()
			designView.Stage, designView.Status, designView.NextRole, designView.Outcome = "design", "ready", "captain", "none"
			designView.Design = &DesignBinding{
				Digest: DigestBytes(designBytes), ProducerInvocation: "test:/implementer/design/1",
			}
			designStatus, err := encodeStatus(designView)
			if err != nil {
				t.Fatal(err)
			}
			designInput := RecordTransitionInput{
				Scope: "work", WorkID: "W1", Result: DesignWritten, Next: designStatus,
				Handoffs: Handoffs{Design: designBytes},
			}
			receipt, err = actions.RecordTransition(designInput)
			if err != nil {
				t.Fatal(err)
			}
			retry, err = actions.RecordTransition(designInput)
			if err != nil || retry.Data()["changed"] != false {
				t.Fatalf("design retry = %#v, %v", retry.Data(), err)
			}
			assertGoldenEvent("recordTransition:DESIGN_WRITTEN", receipt, retry)
			staleSelection, err := snapshot.SelectWork("W1")
			if err != nil {
				t.Fatal(err)
			}
			if staleSelection.Status.Projection() != "design/ready/implementer" {
				t.Fatalf("captured snapshot changed after ref movement: %s", staleSelection.Status.Projection())
			}
			freshSelection, err := mustSnapshot(t, plan, repository).SelectWork("W1")
			if err != nil {
				t.Fatal(err)
			}
			if freshSelection.Status.Projection() != "design/ready/captain" {
				t.Fatalf("fresh snapshot did not observe ref movement: %s", freshSelection.Status.Projection())
			}

			proceedView := designStatus.View()
			proceedView.Stage, proceedView.Status, proceedView.NextRole, proceedView.Outcome = "implement", "ready", "implementer", "proceed"
			proceedView.Captain = &CaptainBinding{
				Outcome: "proceed", Invocation: "test:/captain/review/1",
				PlanDigest: plan.Digest(), DesignDigest: proceedView.Design.Digest,
			}
			proceedStatus, err := encodeStatus(proceedView)
			if err != nil {
				t.Fatal(err)
			}
			proceedInput := RecordTransitionInput{Scope: "work", WorkID: "W1", Result: Proceed, Next: proceedStatus}
			receipt, err = actions.RecordTransition(proceedInput)
			if err != nil {
				t.Fatal(err)
			}
			retry, err = actions.RecordTransition(proceedInput)
			if err != nil || retry.Data()["changed"] != false {
				t.Fatalf("proceed retry = %#v, %v", retry.Data(), err)
			}
			assertGoldenEvent("recordTransition:PROCEED", receipt, retry)

			ownerRefs, err := mechanical.CaptureHeadRefs([]string{"refs/heads/track/demo-v1/T1"})
			if err != nil || len(ownerRefs) != 1 || ownerRefs[0].Head == nil {
				t.Fatalf("capture owner: %#v, %v", ownerRefs, err)
			}
			owner := ownerRefs[0].Head
			timestamp, err := mechanical.CommitTimestamp(*owner)
			if err != nil {
				t.Fatal(err)
			}
			candidate, err := mechanical.PrepareRecord(gitx.RecordRequest{
				Parent: *owner, Changes: []gitx.BlobChange{{Path: "product.txt", Bytes: []byte("delivered\n")}},
				Message: "Implement W1\n", Identity: gitx.Identity{Name: "Implementer", Email: "implementer@example.invalid"},
				Timestamp: timestamp + 1,
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := mechanical.AtomicUpdateRefs([]gitx.RefOperation{{
				Kind: gitx.UpdateRef, Ref: "refs/heads/track/demo-v1/T1", NewHead: &candidate.Commit, Expected: owner,
			}}); err != nil {
				t.Fatal(err)
			}
			productTree, err := mechanical.ProductTreeIdentity(candidate.Commit, ".baton/releases")
			if err != nil {
				t.Fatal(err)
			}
			if candidate.Commit.String() != golden.ProductCandidate || productTree != golden.ProductTree {
				t.Fatalf(
					"product candidate differs from oracle: %s/%s want %s/%s",
					candidate.Commit, productTree, golden.ProductCandidate, golden.ProductTree,
				)
			}
			proofBytes := []byte("# Proof\n\nChecks pass.\n")
			implementedView := proceedStatus.View()
			implementedView.Stage, implementedView.Status, implementedView.NextRole, implementedView.Outcome = "verify", "ready", "verifier", "none"
			designDigest := implementedView.Design.Digest
			captainInvocation := implementedView.Captain.Invocation
			implementedView.Proof = &ProofBinding{
				Digest: DigestBytes(proofBytes), ProducerInvocation: "test:/implementer/implement/1",
				Repository: "example/sworn", BaseCommit: implementedView.Materialization.BaseCommit,
				CandidateCommit: candidate.Commit.String(), CandidateTree: candidate.Tree.String(),
				ProductTree: productTree, PlanDigest: plan.Digest(), ApprovalDigest: approvalDigest,
				DesignDigest: &designDigest, CaptainInvocation: &captainInvocation, Components: []ProofComponent{},
			}
			implementedStatus, err := encodeStatus(implementedView)
			if err != nil {
				t.Fatal(err)
			}
			prepareAdversarial := func(parent gitx.OID, changes []gitx.BlobChange, message string) gitx.PreparedCommit {
				t.Helper()
				parentTimestamp, err := mechanical.CommitTimestamp(parent)
				if err != nil {
					t.Fatal(err)
				}
				prepared, err := mechanical.PrepareRecord(gitx.RecordRequest{
					Parent: parent, Changes: changes, Message: message + "\n",
					Identity:  gitx.Identity{Name: "Fixture", Email: "fixture@example.invalid"},
					Timestamp: parentTimestamp + 1,
				})
				if err != nil {
					t.Fatal(err)
				}
				return prepared
			}
			statusForCandidate := func(prepared gitx.PreparedCommit) Status {
				t.Helper()
				product, err := mechanical.ProductTreeIdentity(prepared.Commit, ".baton/releases")
				if err != nil {
					t.Fatal(err)
				}
				view := implementedStatus.View()
				view.Proof.CandidateCommit = prepared.Commit.String()
				view.Proof.CandidateTree = prepared.Tree.String()
				view.Proof.ProductTree = product
				status, err := encodeStatus(view)
				if err != nil {
					t.Fatal(err)
				}
				return status
			}
			assertCandidateError := func(name string, prepared gitx.PreparedCommit, code string) {
				t.Helper()
				status := statusForCandidate(prepared)
				if _, err := ValidateWorkCandidate(
					plan, repository, "W1", status, prepared.Commit.String(), inertness,
				); ErrorCode(err) != code {
					t.Fatalf("%s candidate error = %v, want %s", name, err, code)
				}
			}

			markerOID, err := gitx.ParseOID(format, materialized.Head)
			if err != nil {
				t.Fatal(err)
			}
			earlyProduct := prepareAdversarial(markerOID, []gitx.BlobChange{{
				Path: "product.txt", Bytes: []byte("too early\n"),
			}}, "early product")
			assertCandidateError("product before PROCEED", earlyProduct, "PRODUCT_BEFORE_PROCEED")

			outside := prepareAdversarial(*owner, []gitx.BlobChange{{
				Path: "outside.txt", Bytes: []byte("outside\n"),
			}}, "outside scope")
			assertCandidateError("outside scope", outside, "WORK_OUTSIDE_SCOPE")

			mixed := prepareAdversarial(*owner, []gitx.BlobChange{
				{Path: "product.txt", Bytes: []byte("mixed\n")},
				{Path: WorkProofPath(plan, "W1"), Bytes: []byte("mixed record\n")},
			}, "mixed product and record")
			assertCandidateError("mixed product and record", mixed, "MIXED_PRODUCT_RECORD_COMMIT")

			recordFinal := prepareAdversarial(candidate.Commit, []gitx.BlobChange{{
				Path: WorkProofPath(plan, "W1"), Bytes: []byte("record final\n"),
			}}, "record final")
			assertCandidateError("record final", recordFinal, "NON_PRODUCT_FINAL_CANDIDATE")

			implementedInput := RecordTransitionInput{
				Scope: "work", WorkID: "W1", Result: Implemented, Next: implementedStatus,
				Handoffs: Handoffs{Proof: proofBytes},
			}
			receipt, err = actions.RecordTransition(implementedInput)
			if err != nil {
				t.Fatal(err)
			}
			retry, err = actions.RecordTransition(implementedInput)
			if err != nil || retry.Data()["changed"] != false {
				t.Fatalf("implemented retry = %#v, %v", retry.Data(), err)
			}
			assertGoldenEvent("recordTransition:IMPLEMENTED", receipt, retry)
			implementedHeads, err := mechanical.CaptureHeadRefs([]string{"refs/heads/track/demo-v1/T1"})
			if err != nil || len(implementedHeads) != 1 || implementedHeads[0].Head == nil {
				t.Fatalf("capture implemented head: %#v, %v", implementedHeads, err)
			}
			hiddenProduct := prepareAdversarial(*implementedHeads[0].Head, []gitx.BlobChange{{
				Path: "product.txt", Bytes: []byte("hidden tail product\n"),
			}}, "hidden product tail")
			if _, err := ValidateWorkCandidate(
				plan, repository, "W1", implementedStatus, hiddenProduct.Commit.String(), inertness,
			); ErrorCode(err) != "PRODUCT_AFTER_CANDIDATE" {
				t.Fatalf("hidden product tail error = %v", err)
			}

			workDispatchRef := "test://dispatch/work/1"
			workDispatch := []byte("fresh work verifier\n")
			dispatches[workDispatchRef] = workDispatch
			passView := implementedStatus.View()
			passView.Stage, passView.Status, passView.NextRole, passView.Outcome = "merge", "ready", "merge", "pass"
			passView.Verification = &VerificationBinding{
				Outcome: "pass", Invocation: "test:/verifier/work/1",
				AttestationRef: workDispatchRef, AttestationDigest: DigestBytes(workDispatch),
				PlanDigest: plan.Digest(), ProofDigest: passView.Proof.Digest,
				CandidateCommit: passView.Proof.CandidateCommit, ProductTree: passView.Proof.ProductTree,
			}
			passStatus, err := encodeStatus(passView)
			if err != nil {
				t.Fatal(err)
			}
			passInput := RecordTransitionInput{Scope: "work", WorkID: "W1", Result: Pass, Next: passStatus}
			receipt, err = actions.RecordTransition(passInput)
			if err != nil {
				t.Fatal(err)
			}
			retry, err = actions.RecordTransition(passInput)
			if err != nil || retry.Data()["changed"] != false {
				t.Fatalf("pass retry = %#v, %v", retry.Data(), err)
			}
			assertGoldenEvent("recordTransition:PASS", receipt, retry)

			receipt, err = actions.ComposeTrack("T1")
			if err != nil {
				t.Fatal(err)
			}
			retry, err = actions.ComposeTrack("T1")
			if err != nil || retry.Data()["changed"] != false {
				t.Fatalf("compose retry = %#v, %v", retry.Data(), err)
			}
			assertGoldenEvent("composeTrack", receipt, retry)

			assemblyProof := []byte("# Assembly proof\n\nExact composition.\n")
			prepareInput := PrepareAssemblyInput{ProofBytes: assemblyProof, ProducerInvocation: "test:/merge/assembly/1"}
			receipt, err = actions.PrepareAssembly(prepareInput)
			if err != nil {
				t.Fatal(err)
			}
			retry, err = actions.PrepareAssembly(prepareInput)
			if err != nil || retry.Data()["changed"] != false {
				t.Fatalf("assembly retry = %#v, %v", retry.Data(), err)
			}
			assertGoldenEvent("prepareAssembly", receipt, retry)

			snapshot = mustSnapshot(t, plan, repository)
			assembly, exists, err := snapshot.SelectAssembly()
			if err != nil || !exists {
				t.Fatal(err)
			}
			assemblyDispatchRef := "test://dispatch/assembly/1"
			assemblyDispatch := []byte("fresh assembly verifier\n")
			dispatches[assemblyDispatchRef] = assemblyDispatch
			assemblyPassView := assembly.Status.View()
			assemblyPassView.Stage, assemblyPassView.Status, assemblyPassView.NextRole, assemblyPassView.Outcome = "merge", "ready", "merge", "pass"
			assemblyPassView.Verification = &VerificationBinding{
				Outcome: "pass", Invocation: "test:/verifier/assembly/1",
				AttestationRef: assemblyDispatchRef, AttestationDigest: DigestBytes(assemblyDispatch),
				PlanDigest: plan.Digest(), ProofDigest: assemblyPassView.Proof.Digest,
				CandidateCommit: assemblyPassView.Proof.CandidateCommit,
				ProductTree:     assemblyPassView.Proof.ProductTree,
			}
			assemblyPass, err := encodeStatus(assemblyPassView)
			if err != nil {
				t.Fatal(err)
			}
			assemblyPassInput := RecordTransitionInput{Scope: "assembly", Result: Pass, Next: assemblyPass}
			receipt, err = actions.RecordTransition(assemblyPassInput)
			if err != nil {
				t.Fatal(err)
			}
			retry, err = actions.RecordTransition(assemblyPassInput)
			if err != nil || retry.Data()["changed"] != false {
				t.Fatalf("assembly PASS retry = %#v, %v", retry.Data(), err)
			}
			assertGoldenEvent("recordTransition:ASSEMBLY_PASS", receipt, retry)

			integrated, err := actions.IntegrateRelease()
			if err != nil {
				t.Fatal(err)
			}
			if integrated.Data()["changed"] != true {
				t.Fatalf("integration = %#v", integrated.Data())
			}
			retry, err = actions.IntegrateRelease()
			if err != nil || retry.Data()["changed"] != false {
				t.Fatalf("integrate retry = %#v, %v", retry.Data(), err)
			}
			assertGoldenEvent("integrateRelease", integrated, retry)
			if eventIndex != len(golden.Events) {
				t.Fatalf("consumed %d oracle events, have %d", eventIndex, len(golden.Events))
			}
			finalHeads, err := repository.CaptureHeadRefs([]string{
				"refs/heads/release/v0.3.0", "refs/heads/release-wt/demo-v1",
			})
			if err != nil {
				t.Fatal(err)
			}
			if finalHeads[0].Head == targetHead || finalHeads[0].Head == finalHeads[1].Head {
				t.Fatalf("target/release terminal identities are not distinct: %#v", finalHeads)
			}
			for index := range finalHeads {
				if index >= len(golden.FinalRefs) || finalHeads[index].Ref != golden.FinalRefs[index].Ref ||
					golden.FinalRefs[index].Head == nil || finalHeads[index].Head != *golden.FinalRefs[index].Head {
					t.Fatalf("final ref %d differs from oracle: %#v want %#v", index, finalHeads[index], golden.FinalRefs)
				}
			}
			finalSnapshot := mustSnapshot(t, plan, repository)
			finalAssembly, exists, err := finalSnapshot.SelectAssembly()
			if err != nil || !exists || finalAssembly.Status.Projection() != "merge/complete/none" {
				t.Fatalf("terminal assembly = %#v, %v", finalAssembly, err)
			}
		})
	}
}

func TestPristineReboundAndExactRetry(t *testing.T) {
	t.Parallel()
	previous := fixturePlan(t)
	revisedBytes := bytes.Replace(previous.Bytes(), []byte("# Demo\n"), []byte("# Demo revised\n"), 1)
	revised, err := ParsePlan(revisedBytes)
	if err != nil {
		t.Fatal(err)
	}
	if revised.Digest() == previous.Digest() {
		t.Fatal("revised plan digest did not change")
	}
	golden := loadReboundGolden(t, gitx.SHA1)
	if golden.PreviousPlanDigest != previous.Digest() || golden.RevisedPlanDigest != revised.Digest() {
		t.Fatalf("rebound plans differ from oracle: %#v", golden)
	}
	mechanical, _ := fixtureRepository(t, gitx.SHA1)
	repository := UseGitRepository(mechanical)
	oldApproval := []byte("old approval\n")
	newApproval := []byte("new approval\n")
	evidence := map[string][]byte{
		DigestBytes(oldApproval): oldApproval,
		DigestBytes(newApproval): newApproval,
	}
	resolver := func(request EvidenceRequest) (Evidence, error) {
		body := evidence[request.Digest]
		if body == nil {
			return Evidence{}, fmt.Errorf("unknown approval %s", request.Digest)
		}
		return Evidence{
			Bytes: body,
			Provenance: EvidenceProvenance{
				Kind: "approval", Ref: request.Ref, Protected: true, Decision: "approved",
				PlanDigest: request.PlanDigest, AuthorizerIsolated: true, DeliveryWritable: false,
			},
		}, nil
	}
	inertness := func(request InertnessRequest) (InertnessDecision, error) {
		return InertnessDecision{
			Repository: request.Repository, RecordRoot: request.RecordRoot,
			Commit: request.Commit, Decision: "inert",
		}, nil
	}
	oldActions, err := NewActions(previous, Guided, repository, resolver, inertness)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := oldActions.InstallApprovedPlan(DigestBytes(oldApproval)); err != nil {
		t.Fatal(err)
	}
	newActions, err := NewActions(revised, Guided, repository, resolver, inertness)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := newActions.ReboundPristinePlan(previous, DigestBytes(newApproval))
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Data()["changed"] != true {
		t.Fatalf("rebound receipt = %#v", receipt.Data())
	}
	retry, err := newActions.ReboundPristinePlan(previous, DigestBytes(newApproval))
	if err != nil {
		t.Fatal(err)
	}
	if retry.Data()["changed"] != false {
		t.Fatalf("rebound retry = %#v", retry.Data())
	}
	if !reflect.DeepEqual(receipt.Data(), golden.First) || !reflect.DeepEqual(retry.Data(), golden.Retry) {
		t.Fatalf(
			"rebound differs from oracle\nfirst=%#v\nwant=%#v\nretry=%#v\nwant=%#v",
			receipt.Data(), golden.First, retry.Data(), golden.Retry,
		)
	}
	snapshot := mustSnapshot(t, revised, repository)
	selected, err := snapshot.SelectWork("W1")
	if err != nil {
		t.Fatal(err)
	}
	if selected.Source != "baseline" || selected.Status.View().Plan.Digest != revised.Digest() ||
		selected.Status.View().Plan.Approval.Digest != DigestBytes(newApproval) {
		t.Fatalf("rebound selection = %#v", selected)
	}
}
