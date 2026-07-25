package baton

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type recordGoldenCase struct {
	Name     string `json:"name"`
	InputHex string `json:"input_hex"`
	Outcome  string `json:"outcome"`
}

type recordGoldenCorpus struct {
	Schema string `json:"schema"`
	Plan   struct {
		InputHex string `json:"input_hex"`
		Digest   string `json:"digest"`
		Release  string `json:"release"`
	} `json:"plan"`
	StrictJSON []recordGoldenCase `json:"strict_json"`
}

type lifecycleGoldenCase struct {
	Name    string           `json:"name"`
	Result  TransitionResult `json:"result"`
	Source  string           `json:"source"`
	Target  string           `json:"target"`
	Outcome string           `json:"outcome"`
}

func loadLifecycleGolden(t *testing.T) []lifecycleGoldenCase {
	t.Helper()
	file, err := os.Open(filepath.Join("..", "..", "tools", "batongolden", "testdata", "corpus", "lifecycle.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var corpus struct {
		Schema string                `json:"schema"`
		Cases  []lifecycleGoldenCase `json:"cases"`
	}
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&corpus); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		t.Fatalf("lifecycle golden has trailing JSON: %v", err)
	}
	if corpus.Schema != "sworn.baton-golden-lifecycle/v1" {
		t.Fatalf("foreign lifecycle golden identity: %#v", corpus)
	}
	return corpus.Cases
}

func loadRecordGolden(t *testing.T) recordGoldenCorpus {
	t.Helper()
	file, err := os.Open(filepath.Join("..", "..", "tools", "batongolden", "testdata", "corpus", "records.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var corpus recordGoldenCorpus
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&corpus); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		t.Fatalf("record golden has trailing JSON: %v", err)
	}
	if corpus.Schema != "sworn.baton-golden-records/v1" {
		t.Fatalf("foreign record golden identity: %#v", corpus)
	}
	return corpus
}

func TestStrictJSONStableErrorCodes(t *testing.T) {
	t.Parallel()
	for _, test := range loadRecordGolden(t).StrictJSON {
		test := test
		t.Run(test.Name, func(t *testing.T) {
			t.Parallel()
			raw, err := hex.DecodeString(test.InputHex)
			if err != nil {
				t.Fatal(err)
			}
			_, parseErr := strictParseJSON(raw, "fixture", MaxPlanBytes)
			if test.Outcome == "pass" {
				if parseErr != nil {
					t.Fatalf("strict JSON rejected oracle positive: %v", parseErr)
				}
			} else if ErrorCode(parseErr) != test.Outcome {
				t.Fatalf("error = %v, code = %q, want %q", parseErr, ErrorCode(parseErr), test.Outcome)
			}
		})
	}
	deep := bytes.Repeat([]byte{'['}, MaxJSONDepth+2)
	deep = append(deep, '0')
	deep = append(deep, bytes.Repeat([]byte{']'}, MaxJSONDepth+2)...)
	if _, err := strictParseJSON(deep, "deep", MaxPlanBytes); ErrorCode(err) != "RESOURCE_LIMIT" {
		t.Fatalf("deep error = %v", err)
	}
}

func TestPlanAdmissionIsRawDigestBoundAndCopySafe(t *testing.T) {
	t.Parallel()
	plan := fixturePlan(t)
	golden := loadRecordGolden(t)
	originalDigest := plan.Digest()
	if originalDigest != golden.Plan.Digest || hex.EncodeToString(plan.Bytes()) != golden.Plan.InputHex ||
		plan.Metadata().Release != golden.Plan.Release {
		t.Fatalf("plan admission differs from oracle: %s/%s", originalDigest, golden.Plan.Digest)
	}
	first := plan.Bytes()
	first[0] = 'x'
	if plan.Digest() != originalDigest || !bytes.HasPrefix(plan.Bytes(), []byte(planOpen)) {
		t.Fatal("mutating returned plan bytes changed admission")
	}
	metadata := plan.Metadata()
	metadata.Tracks[0].Work[0].Scope.Include[0] = "elsewhere"
	if plan.Metadata().Tracks[0].Work[0].Scope.Include[0] != "product.txt" {
		t.Fatal("mutating metadata copy changed admission")
	}
	raw := plan.Bytes()
	raw = bytes.Replace(raw, []byte(`"touch_surfaces": ["product.txt"]`), []byte(`"touch_surfaces": ["other"]`), 1)
	if _, err := ParsePlan(raw); ErrorCode(err) != "WORK_OUTSIDE_TRACK_SCOPE" {
		t.Fatalf("out-of-scope plan error = %v", err)
	}
	if _, err := ParsePlan(append([]byte("\n"), plan.Bytes()...)); ErrorCode(err) != "INVALID_PLAN_FENCE" {
		t.Fatalf("leading-byte plan error = %v", err)
	}
}

func TestStatusStrictShapeSemanticsAndCopies(t *testing.T) {
	t.Parallel()
	plan := fixturePlan(t)
	status, err := baselineStatus(plan, plan.Metadata().Tracks[0], plan.Metadata().Tracks[0].Work[0], strings.Repeat("sha256:", 0)+"sha256:"+strings.Repeat("0", 64))
	if err != nil {
		t.Fatal(err)
	}
	first := status.Bytes()
	first[0] = 'x'
	if status.View().Stage != "design" || status.Bytes()[0] != '{' {
		t.Fatal("status accessors alias admission")
	}
	duplicate := bytes.Replace(status.Bytes(), []byte(`"kind":"work"`), []byte(`"kind":"work","kind":"work"`), 1)
	if _, err := ParseStatus(duplicate); ErrorCode(err) != "DUPLICATE_NAME" {
		t.Fatalf("duplicate status error = %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(status.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	document["unknown"] = true
	raw, _ := json.Marshal(document)
	if _, err := ParseStatus(raw); ErrorCode(err) != "UNKNOWN_FIELD" {
		t.Fatalf("unknown status error = %v", err)
	}
	document = map[string]any{}
	if err := json.Unmarshal(status.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	document["authority_ref"] = "refs/heads/foreign"
	raw, _ = json.Marshal(document)
	if _, err := ParseStatus(raw); ErrorCode(err) != "INVALID_OWNER" {
		t.Fatalf("foreign authority error = %v", err)
	}
}

func TestLifecycleRequiresFreshIndependentGates(t *testing.T) {
	t.Parallel()
	plan := fixturePlan(t)
	metadata := plan.Metadata()
	approval := "sha256:" + strings.Repeat("0", 64)
	initial, err := baselineStatus(plan, metadata.Tracks[0], metadata.Tracks[0].Work[0], approval)
	if err != nil {
		t.Fatal(err)
	}
	designView := initial.View()
	designView.Stage, designView.NextRole = "design", "captain"
	designView.Design = &DesignBinding{
		Digest: "sha256:" + strings.Repeat("1", 64), ProducerInvocation: "test:/implementer/design/1",
	}
	design, err := encodeStatus(designView)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateTransition(initial, design, DesignWritten); err != nil {
		t.Fatal(err)
	}
	proceedView := design.View()
	proceedView.Stage, proceedView.NextRole, proceedView.Outcome = "implement", "implementer", "proceed"
	proceedView.Captain = &CaptainBinding{
		Outcome: "proceed", Invocation: "test:/captain/review/1",
		PlanDigest: plan.Digest(), DesignDigest: designView.Design.Digest,
	}
	proceed, err := encodeStatus(proceedView)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateTransition(design, proceed, Proceed); err != nil {
		t.Fatal(err)
	}
	if err := ValidateTransition(design, proceed, Revise); ErrorCode(err) != "INVALID_TRANSITION" {
		t.Fatalf("wrong-result error = %v", err)
	}
	selfReview := design.View()
	selfReview.Stage, selfReview.NextRole, selfReview.Outcome = "implement", "implementer", "proceed"
	selfReview.Captain = &CaptainBinding{
		Outcome: "proceed", Invocation: selfReview.Design.ProducerInvocation,
		PlanDigest: plan.Digest(), DesignDigest: selfReview.Design.Digest,
	}
	if _, err := encodeStatus(selfReview); ErrorCode(err) != "SELF_REVIEW" {
		t.Fatalf("self review error = %v", err)
	}
	mutated := proceed.View()
	mutated.Plan.Approval.Digest = "sha256:" + strings.Repeat("2", 64)
	mutatedStatus, err := encodeStatus(mutated)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateTransition(proceed, mutatedStatus, NoVerdict); ErrorCode(err) != "IMMUTABLE_BINDING_CHANGED" {
		t.Fatalf("NO_VERDICT mutation error = %v", err)
	}
}

func TestCompleteLifecycleMatchesJavaScriptOracle(t *testing.T) {
	t.Parallel()
	plan := fixturePlan(t)
	metadata := plan.Metadata()
	repeated := func(value byte, count int) string { return strings.Repeat(string([]byte{value}), count) }
	digest := func(value byte) string { return "sha256:" + repeated(value, 64) }
	oid := func(value byte) string { return repeated(value, 40) }
	mustStatus := func(view StatusView) Status {
		t.Helper()
		status, err := encodeStatus(view)
		if err != nil {
			t.Fatal(err)
		}
		return status
	}

	initial, err := baselineStatus(plan, metadata.Tracks[0], metadata.Tracks[0].Work[0], digest('0'))
	if err != nil {
		t.Fatal(err)
	}
	materializedView := initial.View()
	materializedView.AuthorityRef = materializedView.OwnerRef
	materializedView.Materialization = &Materialization{BaseCommit: oid('a'), Dependencies: []MaterializationDependency{}}
	materialized := mustStatus(materializedView)

	designView := materialized.View()
	designView.NextRole = "captain"
	designView.Design = &DesignBinding{Digest: digest('1'), ProducerInvocation: "test:/implementer/design/1"}
	design := mustStatus(designView)

	proceedView := design.View()
	proceedView.Stage, proceedView.NextRole, proceedView.Outcome = "implement", "implementer", "proceed"
	proceedView.Captain = &CaptainBinding{
		Outcome: "proceed", Invocation: "test:/captain/review/1",
		PlanDigest: plan.Digest(), DesignDigest: designView.Design.Digest,
	}
	proceed := mustStatus(proceedView)

	implementedView := proceed.View()
	implementedView.Stage, implementedView.NextRole, implementedView.Outcome = "verify", "verifier", "none"
	implementedView.Proof = &ProofBinding{
		Digest: digest('2'), ProducerInvocation: "test:/implementer/implement/1",
		Repository: metadata.Repository, BaseCommit: implementedView.Materialization.BaseCommit,
		CandidateCommit: oid('b'), CandidateTree: oid('c'), ProductTree: digest('3'),
		PlanDigest: plan.Digest(), ApprovalDigest: digest('0'),
		DesignDigest: &implementedView.Design.Digest, CaptainInvocation: &implementedView.Captain.Invocation,
		Components: []ProofComponent{},
	}
	implemented := mustStatus(implementedView)

	verification := func(source StatusView, outcome, invocation string, marker byte) StatusView {
		next := copyStatusView(source)
		next.Verification = &VerificationBinding{
			Outcome: outcome, Invocation: invocation, AttestationRef: "test://dispatch/" + outcome + "/1",
			AttestationDigest: digest(marker), PlanDigest: next.Plan.Digest,
			ProofDigest: next.Proof.Digest, CandidateCommit: next.Proof.CandidateCommit,
			ProductTree: next.Proof.ProductTree,
		}
		return next
	}

	passView := verification(implemented.View(), "pass", "test:/verifier/work/pass/1", '4')
	passView.Stage, passView.Status, passView.NextRole, passView.Outcome = "merge", "ready", "merge", "pass"
	passed := mustStatus(passView)

	reviseView := design.View()
	reviseView.NextRole, reviseView.Outcome = "implementer", "revise"
	reviseView.Captain = &CaptainBinding{
		Outcome: "revise", Invocation: "test:/captain/review/2",
		PlanDigest: plan.Digest(), DesignDigest: reviseView.Design.Digest,
	}
	revise := mustStatus(reviseView)

	escalateView := design.View()
	escalateView.Status, escalateView.NextRole, escalateView.Outcome = "blocked", "planner", "escalate"
	escalateView.Captain = &CaptainBinding{
		Outcome: "escalate", Invocation: "test:/captain/review/3",
		PlanDigest: plan.Digest(), DesignDigest: escalateView.Design.Digest,
	}
	escalateView.Blocker = &Blocker{Code: "captain_blocked", Summary: "needs an external decision"}
	escalate := mustStatus(escalateView)

	failView := verification(implemented.View(), "fail", "test:/verifier/work/fail/1", '5')
	failView.Stage, failView.NextRole, failView.Outcome = "implement", "implementer", "fail"
	failed := mustStatus(failView)

	blockedView := verification(implemented.View(), "blocked", "test:/verifier/work/blocked/1", '6')
	blockedView.Status, blockedView.NextRole, blockedView.Outcome = "blocked", "planner", "blocked"
	blockedView.Blocker = &Blocker{Code: "verification_blocked", Summary: "cannot finish verification"}
	blocked := mustStatus(blockedView)

	assemblyView := StatusView{
		Schema: StatusSchema, SchemaVersion: StatusVersion, Kind: "assembly", Release: metadata.Release,
		OwnerRef: metadata.ReleaseRef, AuthorityRef: metadata.ReleaseRef, TargetRef: metadata.TargetRef,
		Plan:  PlanBinding{Digest: plan.Digest(), Approval: ApprovalBinding{Ref: metadata.ApprovalRef, Digest: digest('0')}},
		Stage: "verify", Status: "ready", NextRole: "verifier", Outcome: "none",
		Proof: &ProofBinding{
			Digest: digest('7'), ProducerInvocation: "test:/merge/assembly/1",
			Repository: metadata.Repository, BaseCommit: oid('d'), CandidateCommit: oid('d'),
			CandidateTree: oid('e'), ProductTree: digest('8'), PlanDigest: plan.Digest(),
			ApprovalDigest: digest('0'), Components: []ProofComponent{{TrackID: "T1", Head: oid('f')}},
		},
	}
	assembly := mustStatus(assemblyView)

	assemblyPassView := verification(assembly.View(), "pass", "test:/verifier/assembly/pass/1", '9')
	assemblyPassView.Stage, assemblyPassView.NextRole, assemblyPassView.Outcome = "merge", "merge", "pass"
	assemblyPass := mustStatus(assemblyPassView)

	assemblyFailView := verification(assembly.View(), "fail", "test:/verifier/assembly/fail/1", 'a')
	assemblyFailView.NextRole, assemblyFailView.Outcome = "planner", "fail"
	assemblyFail := mustStatus(assemblyFailView)

	assemblyBlockedView := verification(assembly.View(), "blocked", "test:/verifier/assembly/blocked/1", 'b')
	assemblyBlockedView.Status, assemblyBlockedView.NextRole, assemblyBlockedView.Outcome = "blocked", "planner", "blocked"
	assemblyBlockedView.Blocker = &Blocker{Code: "assembly_blocked", Summary: "cannot finish assembly verification"}
	assemblyBlocked := mustStatus(assemblyBlockedView)

	mergedView := assemblyPass.View()
	mergedView.Status, mergedView.NextRole, mergedView.Outcome = "complete", "none", "merged"
	mergedView.Merge = &MergeBinding{
		Scope: "release", PassedCandidate: mergedView.Proof.CandidateCommit,
		ExpectedTarget: oid('1'), Outcome: "merged", ObservedTarget: oid('1'), ResultCommit: oid('2'),
		PlanDigest: plan.Digest(), VerificationAttestationDigest: mergedView.Verification.AttestationDigest,
	}
	merged := mustStatus(mergedView)

	reboundView := initial.View()
	reboundView.Plan.Approval.Digest = digest('c')
	rebound := mustStatus(reboundView)

	cases := map[string]struct {
		previous Status
		next     Status
		result   TransitionResult
	}{
		"design-written":   {materialized, design, DesignWritten},
		"captain-proceed":  {design, proceed, Proceed},
		"captain-revise":   {design, revise, Revise},
		"captain-escalate": {design, escalate, Escalate},
		"implemented":      {proceed, implemented, Implemented},
		"work-pass":        {implemented, passed, Pass},
		"work-fail":        {implemented, failed, Fail},
		"work-blocked":     {implemented, blocked, Blocked},
		"assembly-pass":    {assembly, assemblyPass, Pass},
		"assembly-fail":    {assembly, assemblyFail, Fail},
		"assembly-blocked": {assembly, assemblyBlocked, Blocked},
		"merged":           {assemblyPass, merged, Merged},
		"materialize":      {initial, materialized, Materialize},
		"rebound":          {initial, rebound, Rebound},
		"no-verdict":       {implemented, implemented, NoVerdict},
	}
	golden := loadLifecycleGolden(t)
	if len(golden) != len(cases) {
		t.Fatalf("oracle lifecycle cases = %d, Go cases = %d", len(golden), len(cases))
	}
	for _, expected := range golden {
		test, ok := cases[expected.Name]
		if !ok {
			t.Fatalf("Go lifecycle lacks oracle case %s", expected.Name)
		}
		if expected.Outcome != "pass" || test.result != expected.Result ||
			test.previous.Projection() != expected.Source || test.next.Projection() != expected.Target {
			t.Fatalf("lifecycle %s differs from oracle: %#v", expected.Name, expected)
		}
		if err := ValidateTransition(test.previous, test.next, test.result); err != nil {
			t.Fatalf("lifecycle %s: %v", expected.Name, err)
		}
	}
}

func TestEvidenceAdmissionFailsClosed(t *testing.T) {
	t.Parallel()
	plan := fixturePlan(t)
	approvalBytes := []byte("approved\n")
	status, err := baselineStatus(plan, plan.Metadata().Tracks[0], plan.Metadata().Tracks[0].Work[0], DigestBytes(approvalBytes))
	if err != nil {
		t.Fatal(err)
	}
	good := func(request EvidenceRequest) (Evidence, error) {
		return Evidence{
			Bytes: approvalBytes,
			Provenance: EvidenceProvenance{
				Kind: "approval", Ref: request.Ref, Protected: true, Decision: "approved",
				PlanDigest: request.PlanDigest, AuthorizerIsolated: true, DeliveryWritable: false,
			},
		}, nil
	}
	admission, err := resolveStatusEvidence(status, Autonomous, good)
	if err != nil {
		t.Fatal(err)
	}
	if err := requireEvidenceAdmission(status, admission, Autonomous); err != nil {
		t.Fatal(err)
	}
	badDigest := func(request EvidenceRequest) (Evidence, error) {
		result, _ := good(request)
		result.Bytes = []byte("other\n")
		return result, nil
	}
	if _, err := resolveStatusEvidence(status, Autonomous, badDigest); ErrorCode(err) != "EVIDENCE_BINDING_MISMATCH" {
		t.Fatalf("bad evidence error = %v", err)
	}
	badAuthority := func(request EvidenceRequest) (Evidence, error) {
		result, _ := good(request)
		result.Provenance.DeliveryWritable = true
		return result, nil
	}
	if _, err := resolveStatusEvidence(status, Autonomous, badAuthority); ErrorCode(err) != "UNTRUSTED_EVIDENCE_PROVENANCE" {
		t.Fatalf("bad provenance error = %v", err)
	}
}
