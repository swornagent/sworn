//go:build linux

package store

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/engine"
	"github.com/swornagent/sworn/internal/executor"
	"github.com/swornagent/sworn/internal/policy"
	"github.com/swornagent/sworn/internal/protocol"
)

const (
	verifierLifecycleAgent = "codex-cli/verifier-test"
)

type verifierLifecycleFixture struct {
	base       *atomicAdmissionFixture
	control    *Store
	plan       protocol.ExactPlan
	authority  *policy.Authority
	privateKey ed25519.PrivateKey
	ownership  *ControllerOwnership
	ownerID    string
	runID      string
	workID     string
	profile    protocol.VerifierProfile
}

func newVerifierLifecycleFixture(t *testing.T) *verifierLifecycleFixture {
	t.Helper()
	ctx := context.Background()
	base := newAtomicAdmissionFixture(t, atomicAdmissionOptions{})
	if result, err := base.control.Apply(ctx, base.command); err != nil || result.Outcome != OutcomeApplied {
		t.Fatalf("admit verifier fixture submission = %+v, %v", result, err)
	}
	state, err := base.control.State(ctx, base.command.RunID)
	if err != nil {
		t.Fatal(err)
	}
	profile, profileRecord := verifierLifecycleProfile(t, base.repository.Binding().RepositoryID)
	profileDigest, err := base.control.PutArtifact(
		ctx, protocol.VerifierProfileMediaType, profileRecord.CanonicalJSON,
	)
	if err != nil || profileDigest != profileRecord.Digest {
		t.Fatalf("put verifier lifecycle profile = %q, %v", profileDigest, err)
	}
	base.control.verifierProfileDigest = profileRecord.Digest
	base.control.verifierAgent = profile.Agent
	plan, err := base.control.Plan(ctx, state.PlanDigest)
	if err != nil {
		t.Fatal(err)
	}
	authority, _, privateKey := authorityFixture(
		t, base.control, plan, 2, nil, false, controlledSourceMutation(plan, nil),
	)
	if err := os.Chmod(filepath.Dir(base.control.ControlPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	fixture := &verifierLifecycleFixture{
		base: base, control: base.control, plan: plan, authority: authority, privateKey: privateKey,
		ownerID: "verifier-controller-1", runID: state.RunID, workID: state.Work[0].ID, profile: profile,
	}
	fixture.ownership, err = fixture.control.AcquireControllerOwnership(fixture.ownerID)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.ownership.Activate(ctx, fixture.control, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if fixture.ownership != nil {
			_ = fixture.ownership.Close()
		}
	})
	return fixture
}

func verifierLifecycleProfile(
	t *testing.T,
	repositoryID string,
) (protocol.VerifierProfile, protocol.EncodedRecord) {
	t.Helper()
	schemaDigest, err := protocol.VerifierAssessmentOutputSchemaDigest()
	if err != nil {
		t.Fatal(err)
	}
	profile := protocol.VerifierProfile{
		SchemaVersion:               protocol.VerifierProfileSchemaVersion,
		Agent:                       verifierLifecycleAgent,
		BinaryPath:                  "/opt/sworn/codex",
		BinaryVersion:               verifierLifecycleAgent,
		BinaryDigest:                "sha256:1111111111111111111111111111111111111111111111111111111111111111",
		BinarySize:                  1,
		ExecutableInput:             "codex",
		Provider:                    "openai",
		Authentication:              "codex-cli-chatgpt-file-v1",
		CredentialHome:              executor.CredentialHome,
		PermissionProfile:           "sworn_verifier",
		Model:                       "gpt-test",
		ToolSchemaDigest:            "sha256:2222222222222222222222222222222222222222222222222222222222222222",
		Argv:                        protocol.CanonicalCodexVerifierArgv("gpt-test"),
		EnvironmentNames:            []string{},
		PromptDigest:                protocol.RawDigest([]byte(protocol.NativeCodexVerifierPrompt)),
		OutputSchemaDigest:          schemaDigest,
		TimeoutNanoseconds:          int64(time.Minute),
		Network:                     string(executor.NetworkHost),
		WorkspaceAccess:             string(executor.WorkspaceReadOnly),
		NestedSandbox:               true,
		CredentialAccess:            true,
		ModelToolNetwork:            false,
		ModelToolCredentialAccess:   false,
		ExecutorConfigurationDigest: "sha256:3333333333333333333333333333333333333333333333333333333333333333",
		RepositoryID:                repositoryID,
		WorkspaceRoot:               "/var/lib/sworn/verifier-test",
		MaterializeBytes:            1 << 20,
		MaterializeEntries:          1_000,
	}
	record, err := protocol.EncodeVerifierProfile(profile)
	if err != nil {
		t.Fatal(err)
	}
	return profile, record
}

func (fixture *verifierLifecycleFixture) dispatch(t *testing.T, commandID string) ApplyResult {
	t.Helper()
	ctx := context.Background()
	state, plan, request, prepared, err := fixture.control.PrepareControlledVerifierDispatch(
		ctx, fixture.ownership, fixture.ownerID, fixture.runID, fixture.workID, commandID,
	)
	if err != nil {
		t.Fatal(err)
	}
	permit, err := fixture.authority.AuthorizeVerifierExecution(ctx, plan, request)
	if err != nil {
		t.Fatal(err)
	}
	command := testCommand(
		t, commandID, engine.CommandDispatchVerifier, state.Revision,
		engine.DispatchVerifierPayload{WorkID: fixture.workID},
	)
	result, err := fixture.control.ApplyControlledVerifierDispatch(
		ctx, fixture.ownership, fixture.authority, permit, request, prepared, command,
	)
	if err != nil || result.Outcome != OutcomeApplied || len(result.EffectIDs) != 1 {
		t.Fatalf("controlled verifier dispatch = %+v, %v", result, err)
	}
	if result.EffectIDs[0] != request.VerifierEffectID || request.DispatchID != request.VerifierEffectID {
		t.Fatalf("dispatch identities = result %v request %+v", result.EffectIDs, request)
	}
	return result
}

func (fixture *verifierLifecycleFixture) claimAndPrepare(
	t *testing.T,
) (policy.VerifierExecutionPermitRequest, AuthorizedVerifierLease, PreparedAuthorizedVerifierLease) {
	t.Helper()
	ctx := context.Background()
	_, plan, request, err := fixture.control.PendingVerifierExecutionPermitRequest(
		ctx, fixture.ownership, fixture.ownerID, fixture.runID, fixture.workID,
	)
	if err != nil {
		t.Fatal(err)
	}
	permit, err := fixture.authority.AuthorizeVerifierExecution(ctx, plan, request)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := fixture.control.ClaimControlledVerifier(
		ctx, fixture.ownership, fixture.authority, permit, request,
	)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := fixture.control.PrepareAuthorizedVerifierExecution(ctx, lease)
	if err != nil {
		t.Fatal(err)
	}
	return request, lease, prepared
}

func (fixture *verifierLifecycleFixture) resultFor(
	t *testing.T,
	effectID string,
	outcome engine.VerdictOutcome,
) json.RawMessage {
	t.Helper()
	ctx := context.Background()
	effect, err := loadEffect(ctx, fixture.control.db, effectID)
	if err != nil {
		t.Fatal(err)
	}
	request, err := engine.ParseVerifierEffectRequest(effect.Request)
	if err != nil {
		t.Fatal(err)
	}
	assessment := fixture.assessment(t, outcome)
	assessmentBytes, err := protocol.EncodeCanonical(assessment)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := protocol.ParseVerifierAssessment(assessmentBytes); err != nil {
		t.Fatalf("invalid verifier fixture assessment: %v", err)
	}
	assessmentDigest, err := fixture.control.PutArtifact(ctx, "application/json", assessmentBytes)
	if err != nil {
		t.Fatal(err)
	}
	startedAt := atomicAdmissionTime.Format(time.RFC3339Nano)
	receipt := fixture.putVerifierExecutionReceipt(
		t, effect, request, assessmentBytes, startedAt, startedAt,
	)
	result, err := engine.EncodeVerifierEffectResult(engine.VerifierEffectResult{
		SchemaVersion: engine.VerifierEffectResultSchemaVersion,
		Outcome:       engine.VerifierOutcomeAssessmentReady,
		DispatchID:    effectID, VerificationEpoch: request.VerificationEpoch,
		Assessment: protocol.Artifact{
			Ref: assessmentDigest, MediaType: "application/json", Digest: assessmentDigest,
		},
		ExecutionReceipt: receipt,
		StartedAt:        startedAt,
		CompletedAt:      startedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func (fixture *verifierLifecycleFixture) rewriteVerifierExecutionReceipt(
	t *testing.T,
	encoded json.RawMessage,
	mutate func(*protocol.VerifierExecutionReceipt),
) json.RawMessage {
	t.Helper()
	ctx := context.Background()
	result, err := engine.ParseVerifierEffectResult(encoded)
	if err != nil {
		t.Fatal(err)
	}
	receiptBytes, err := protocol.ResolveArtifact(
		ctx, fixture.control, result.ExecutionReceipt, protocol.MaximumVerifierExecutionReceiptBytes,
	)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := protocol.ParseVerifierExecutionReceipt(receiptBytes)
	if err != nil {
		t.Fatal(err)
	}
	mutate(&receipt)
	receiptRecord, err := protocol.EncodeVerifierExecutionReceipt(receipt)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := fixture.control.PutArtifact(
		ctx, protocol.VerifierExecutionReceiptMediaType, receiptRecord.CanonicalJSON,
	)
	if err != nil || digest != receiptRecord.Digest {
		t.Fatalf("put rewritten verifier execution receipt = %q, %v", digest, err)
	}
	result.ExecutionReceipt = protocol.Artifact{
		Ref: digest, MediaType: protocol.VerifierExecutionReceiptMediaType, Digest: digest,
	}
	rewritten, err := engine.EncodeVerifierEffectResult(result)
	if err != nil {
		t.Fatal(err)
	}
	return rewritten
}

func (fixture *verifierLifecycleFixture) putVerifierExecutionReceipt(
	t *testing.T,
	effect Effect,
	request engine.VerifierEffectRequest,
	assessmentBytes []byte,
	startedAt, completedAt string,
) protocol.Artifact {
	t.Helper()
	ctx := context.Background()
	assessmentDigest := protocol.RawDigest(assessmentBytes)
	schemaBytes, err := protocol.VerifierAssessmentOutputSchema()
	if err != nil {
		t.Fatal(err)
	}
	schemaDigest, err := fixture.control.PutArtifact(
		ctx, protocol.VerifierAssessmentSchemaMediaType, schemaBytes,
	)
	if err != nil || schemaDigest != fixture.profile.OutputSchemaDigest {
		t.Fatalf("put verifier assessment schema = %q, %v", schemaDigest, err)
	}
	planRecord := fixture.plan.Record()
	_, submissionBytes, err := fixture.control.Record(ctx, request.SubmissionDigest)
	if err != nil {
		t.Fatal(err)
	}
	submission, err := protocol.ParseSubmission(submissionBytes)
	if err != nil {
		t.Fatal(err)
	}
	dispatchBytes, err := protocol.ResolveArtifact(
		ctx, fixture.control, request.DispatchReceipt, protocol.MaximumControlReceiptBytes,
	)
	if err != nil {
		t.Fatal(err)
	}
	review, err := protocol.ResolveExactVerifierReview(ctx, fixture.control, fixture.plan, submission)
	if err != nil {
		t.Fatal(err)
	}
	inputs := []protocol.VerifierExecutionInput{
		{Name: "assessment-schema", Digest: schemaDigest, Size: uint64(len(schemaBytes))},
		{Name: fixture.profile.ExecutableInput, Digest: fixture.profile.BinaryDigest, Size: uint64(fixture.profile.BinarySize)},
		{Name: "dispatch", Digest: request.DispatchReceipt.Digest, Size: uint64(len(dispatchBytes))},
		{Name: "plan", Digest: planRecord.Digest, Size: uint64(len(planRecord.CanonicalJSON))},
		{Name: "submission", Digest: request.SubmissionDigest, Size: uint64(len(submissionBytes))},
	}
	for _, input := range review.Inputs() {
		inputs = append(inputs, protocol.VerifierExecutionInput{
			Name: input.Name, Digest: input.Digest, Size: uint64(len(input.Contents)),
		})
	}
	slices.SortFunc(inputs, func(left, right protocol.VerifierExecutionInput) int {
		return strings.Compare(left.Name, right.Name)
	})
	stdout := fixture.putVerifierCapture(
		t, verifierLifecycleJSONL(t, assessmentBytes, "thread-test-1"),
	)
	stderr := fixture.putVerifierCapture(t, nil)
	identity, err := engine.VerifierAttemptIdentityFor(
		effect.ID, effect.Attempt, request.DispatchID, request.DispatchReceipt.Digest,
		request.VerifierProfileDigest, request.Agent, request.VerificationEpoch,
	)
	if err != nil {
		t.Fatal(err)
	}
	receiptRecord, err := protocol.EncodeVerifierExecutionReceipt(protocol.VerifierExecutionReceipt{
		SchemaVersion:               protocol.VerifierExecutionReceiptSchemaVersion,
		EffectID:                    effect.ID,
		EffectAttempt:               effect.Attempt,
		InvocationID:                identity.InvocationID,
		DeliveryRunID:               request.DeliveryRunID,
		DeliveryID:                  request.DeliveryID,
		WorkID:                      request.WorkID,
		WorkAttempt:                 request.WorkAttempt,
		PlanDigest:                  request.PlanDigest,
		SubmissionID:                request.SubmissionID,
		SubmissionDigest:            request.SubmissionDigest,
		Candidate:                   request.Candidate,
		DispatchID:                  request.DispatchID,
		DispatchDigest:              request.DispatchReceipt.Digest,
		VerifierProfileDigest:       request.VerifierProfileDigest,
		Agent:                       request.Agent,
		VerificationEpoch:           request.VerificationEpoch,
		ExecutorConfigurationDigest: fixture.profile.ExecutorConfigurationDigest,
		ExecutableInput:             fixture.profile.ExecutableInput,
		ExecutableDigest:            fixture.profile.BinaryDigest,
		Unit:                        "sworn-verifier-test.service",
		WorkspaceDigest:             "sha256:4444444444444444444444444444444444444444444444444444444444444444",
		WorkspaceAccess:             fixture.profile.WorkspaceAccess,
		Inputs:                      inputs,
		Network:                     fixture.profile.Network,
		NestedSandbox:               fixture.profile.NestedSandbox,
		CredentialAccess:            fixture.profile.CredentialAccess,
		ModelToolNetwork:            fixture.profile.ModelToolNetwork,
		ModelToolCredentialAccess:   fixture.profile.ModelToolCredentialAccess,
		AssessmentDigest:            assessmentDigest,
		Stdout:                      stdout,
		Stderr:                      stderr,
		ThreadID:                    "thread-test-1",
		StartedAt:                   startedAt,
		CompletedAt:                 completedAt,
		TargetStarted:               true,
		ServiceQuiescent:            true,
		ExitCode:                    0,
	})
	if err != nil {
		t.Fatal(err)
	}
	digest, err := fixture.control.PutArtifact(
		ctx, protocol.VerifierExecutionReceiptMediaType, receiptRecord.CanonicalJSON,
	)
	if err != nil || digest != receiptRecord.Digest {
		t.Fatalf("put verifier execution receipt = %q, %v", digest, err)
	}
	return protocol.Artifact{
		Ref: digest, MediaType: protocol.VerifierExecutionReceiptMediaType, Digest: digest,
	}
}

func (fixture *verifierLifecycleFixture) putVerifierCapture(
	t *testing.T,
	contents []byte,
) protocol.CapturedArtifact {
	t.Helper()
	digest, err := fixture.control.PutArtifact(context.Background(), "application/octet-stream", contents)
	if err != nil {
		t.Fatal(err)
	}
	return protocol.CapturedArtifact{
		Ref: digest, MediaType: "application/octet-stream", Digest: digest, Size: int64(len(contents)),
	}
}

func verifierLifecycleJSONL(t testing.TB, assessment []byte, threadID string) []byte {
	t.Helper()
	thread, err := json.Marshal(map[string]any{
		"type": "thread.started", "thread_id": threadID,
	})
	if err != nil {
		t.Fatal(err)
	}
	agent, err := json.Marshal(map[string]any{
		"type": "item.completed",
		"item": map[string]any{
			"id": "item-test-1", "type": "agent_message", "text": string(assessment),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	contents := []byte(strings.Join([]string{
		string(thread),
		`{"type":"turn.started"}`,
		string(agent),
		`{"type":"turn.completed","usage":{"input_tokens":1,"cached_input_tokens":0,"cache_write_input_tokens":0,"output_tokens":1,"reasoning_output_tokens":0}}`,
		"",
	}, "\n"))
	turn, err := protocol.ParseNativeCodexVerifierJSONL(contents)
	if err != nil || string(turn.Assessment) != string(assessment) || turn.ThreadID != threadID {
		t.Fatalf("construct verifier lifecycle JSONL = %#v, %v", turn, err)
	}
	return contents
}

func (fixture *verifierLifecycleFixture) assessment(
	t *testing.T,
	outcome engine.VerdictOutcome,
) protocol.VerifierAssessment {
	t.Helper()
	ctx := context.Background()
	state, err := fixture.control.State(ctx, fixture.runID)
	if err != nil {
		t.Fatal(err)
	}
	kind, submissionBytes, err := fixture.control.Record(ctx, state.Work[0].SubmissionDigest)
	if err != nil || kind != protocol.SubmissionSchemaVersion {
		t.Fatalf("load verifier fixture submission = %q, %v", kind, err)
	}
	submission, err := protocol.ParseSubmission(submissionBytes)
	if err != nil {
		t.Fatal(err)
	}
	contract, exists := fixture.plan.Work(fixture.workID)
	if !exists {
		t.Fatal("verifier fixture work contract is absent")
	}
	view, submissionView := contract.View(), submission.View()
	acceptance := make([]protocol.AcceptanceResult, len(view.Acceptance))
	for index, item := range view.Acceptance {
		var evidenceIDs []string
		for _, evidence := range submissionView.Evidence {
			for _, acceptanceID := range evidence.AcceptanceIDs {
				if acceptanceID == item.ID {
					evidenceIDs = append(evidenceIDs, evidence.ID)
					break
				}
			}
		}
		if len(evidenceIDs) == 0 {
			t.Fatalf("acceptance %q lacks verifier evidence", item.ID)
		}
		acceptance[index] = protocol.AcceptanceResult{
			AcceptanceID: item.ID, Outcome: "pass", EvidenceIDs: evidenceIDs,
			Summary: "The exact retained evidence was assessed.",
		}
	}
	assurance := make([]protocol.AssuranceResult, len(view.Assurance.Packs))
	for index, pack := range view.Assurance.Packs {
		var evidenceIDs []string
		for _, evidence := range submissionView.Evidence {
			for _, packID := range evidence.PackIDs {
				if packID == pack {
					evidenceIDs = append(evidenceIDs, evidence.ID)
					break
				}
			}
		}
		assurance[index] = protocol.AssuranceResult{
			Pack: pack, Outcome: "pass", EvidenceIDs: evidenceIDs,
			Summary: "The selected assurance pack was assessed.",
		}
	}
	findings := []protocol.Finding{}
	switch outcome {
	case engine.VerdictFail:
		findings = append(findings, verifierLifecycleFinding("implementation"))
	case engine.VerdictSpecBlock:
		findings = append(findings, verifierLifecycleFinding("contract"))
	case engine.VerdictInconclusive:
		findings = append(findings, verifierLifecycleFinding("environment"))
	}
	return protocol.VerifierAssessment{
		SchemaVersion: protocol.VerifierAssessmentSchemaVersion,
		Outcome:       string(outcome), Summary: "Independent verification completed.",
		AcceptanceResults: acceptance, AssuranceResults: assurance, Findings: findings,
	}
}

func verifierLifecycleFinding(kind string) protocol.Finding {
	return protocol.Finding{
		ID: "finding-" + kind, Kind: kind, Principle: "B3", Severity: "blocking",
		Summary:       "The verifier found a blocking " + kind + " condition.",
		AcceptanceIDs: []string{}, EvidenceIDs: []string{},
	}
}

func (fixture *verifierLifecycleFixture) execute(
	t *testing.T,
	outcome engine.VerdictOutcome,
	checkOneShot bool,
) string {
	t.Helper()
	ctx := context.Background()
	request, lease, prepared := fixture.claimAndPrepare(t)
	result := fixture.resultFor(t, request.VerifierEffectID, outcome)
	if _, err := fixture.control.PrepareAuthorizedVerifierExecution(ctx, lease); err == nil {
		t.Fatal("authorized verifier lease was prepared twice")
	}
	copyOfPrepared := prepared
	runResult, err := prepared.RunVerifier(func(effect engine.JournalEffect) (json.RawMessage, error) {
		if effect.ID != request.VerifierEffectID || effect.Kind != engine.EffectVerifier || effect.Attempt != 1 {
			t.Fatalf("verifier journal invocation = %+v", effect)
		}
		return result, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checkOneShot {
		if _, err := copyOfPrepared.RunVerifier(func(engine.JournalEffect) (json.RawMessage, error) {
			return result, nil
		}); err == nil || !strings.Contains(err.Error(), "already consumed") {
			t.Fatalf("copied verifier capability reuse error = %v", err)
		}
		generic := lease.effectLease()
		if err := fixture.control.BindEffectResult(ctx, generic, runResult); err == nil {
			t.Fatal("generic lease bound a controlled verifier result")
		}
		if err := fixture.control.CompleteEffect(ctx, generic); err == nil {
			t.Fatal("generic lease completed a controlled verifier")
		}
		if err := fixture.control.FailEffect(ctx, generic, "generic failure"); err == nil {
			t.Fatal("generic lease failed a controlled verifier")
		}
		if err := fixture.control.BindAuthorizedVerifierResult(
			ctx, PreparedAuthorizedVerifierLease{}, runResult,
		); err == nil {
			t.Fatal("zero prepared verifier capability bound a result")
		}
	}
	if err := fixture.control.BindAuthorizedVerifierResult(ctx, prepared, runResult); err != nil {
		t.Fatal(err)
	}
	if err := fixture.control.CompleteAuthorizedVerifier(ctx, prepared); err != nil {
		t.Fatal(err)
	}
	if err := fixture.control.CompleteAuthorizedVerifier(ctx, prepared); err == nil {
		t.Fatal("completed verifier capability was accepted twice")
	}
	return request.VerifierEffectID
}

func (fixture *verifierLifecycleFixture) admit(
	t *testing.T,
	outcome engine.VerdictOutcome,
	commandID string,
) ApplyResult {
	t.Helper()
	ctx := context.Background()
	state, plan, passRequest, prepared, err := fixture.control.PrepareControlledVerdictAdmission(
		ctx, fixture.ownership, fixture.ownerID, fixture.runID, fixture.workID, commandID,
	)
	if err != nil {
		t.Fatal(err)
	}
	command := testCommand(
		t, commandID, engine.CommandAdmitVerdict, state.Revision,
		engine.AdmitVerdictPayload{WorkID: fixture.workID},
	)
	var result ApplyResult
	if outcome == engine.VerdictPass {
		if passRequest.Outcome != string(engine.VerdictPass) {
			t.Fatalf("PASS preparation request = %+v", passRequest)
		}
		permit, err := fixture.authority.AuthorizePASSAdmission(ctx, plan, passRequest)
		if err != nil {
			t.Fatal(err)
		}
		result, err = fixture.control.ApplyControlledPASSAdmission(
			ctx, fixture.ownership, fixture.authority, permit, passRequest, prepared, command,
		)
		if err != nil {
			t.Fatal(err)
		}
	} else {
		if passRequest != (policy.PASSAdmissionPermitRequest{}) {
			t.Fatalf("non-PASS preparation requested PASS authority: %+v", passRequest)
		}
		result, err = fixture.control.ApplyControlledVerdictAdmission(
			ctx, fixture.ownership, fixture.ownerID, prepared, command,
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	if result.Outcome != OutcomeApplied || result.Revision != state.Revision+1 || len(result.EffectIDs) != 0 {
		t.Fatalf("controlled verdict admission = %+v", result)
	}
	return result
}

func TestStoreVerifierLifecycleRoutesDurableVerdicts(t *testing.T) {
	tests := []struct {
		outcome engine.VerdictOutcome
		state   engine.WorkState
		action  engine.NextAction
	}{
		{engine.VerdictPass, engine.WorkVerified, engine.ActionReplan},
		{engine.VerdictFail, engine.WorkRepair, engine.ActionRepair},
		{engine.VerdictSpecBlock, engine.WorkBlocked, engine.ActionReplan},
		{engine.VerdictInconclusive, engine.WorkRetry, engine.ActionRetryVerification},
	}
	for _, test := range tests {
		test := test
		t.Run(string(test.outcome), func(t *testing.T) {
			fixture := newVerifierLifecycleFixture(t)
			dispatch := fixture.dispatch(t, "cmd-verifier-dispatch-1")
			effectID := fixture.execute(t, test.outcome, test.outcome == engine.VerdictPass)
			if effectID != dispatch.EffectIDs[0] {
				t.Fatalf("executed verifier %q, dispatched %q", effectID, dispatch.EffectIDs[0])
			}
			fixture.admit(t, test.outcome, "cmd-verdict-admit-1")
			state, err := fixture.control.State(context.Background(), fixture.runID)
			if err != nil {
				t.Fatal(err)
			}
			work := state.Work[0]
			if work.State != test.state || work.NextAction != test.action || work.Verdict != test.outcome ||
				!engine.ValidID(work.VerdictID) || !engine.ValidDigest(work.VerdictDigest) ||
				work.VerificationEpoch != 1 || work.VerdictEpoch != 1 {
				t.Fatalf("admitted %s work = %+v", test.outcome, work)
			}
			if (test.outcome == engine.VerdictSpecBlock) != (work.Attention != "") {
				t.Fatalf("%s work attention = %q", test.outcome, work.Attention)
			}
			var dispatchID, verdictID, digest, outcome string
			var reviewEpoch, eventRevision int64
			if err := fixture.control.db.QueryRow(`
				SELECT dispatch_id, verdict_id, digest, outcome, review_epoch, event_revision
				FROM verdict_records WHERE verifier_effect_id = ?`, effectID,
			).Scan(&dispatchID, &verdictID, &digest, &outcome, &reviewEpoch, &eventRevision); err != nil {
				t.Fatal(err)
			}
			if dispatchID != effectID || verdictID != work.VerdictID || digest != work.VerdictDigest ||
				outcome != string(test.outcome) || reviewEpoch != 1 || eventRevision != state.Revision {
				t.Fatalf("durable verdict row = %q %q %q %q %d %d", dispatchID, verdictID, digest, outcome, reviewEpoch, eventRevision)
			}
			kind, contents, err := fixture.control.Record(context.Background(), digest)
			if err != nil || kind != protocol.DeliveryVerdictSchemaVersion {
				t.Fatalf("durable verdict record = %q, %d bytes, %v", kind, len(contents), err)
			}
			verdict, err := protocol.ParseDeliveryVerdict(contents)
			if err != nil || verdict.VerdictID != verdictID || verdict.Outcome != outcome ||
				verdict.Review.RunID != effectID || verdict.SubmissionDigest != work.SubmissionDigest {
				t.Fatalf("parsed durable verdict = %+v, %v", verdict, err)
			}
		})
	}
}

func TestStoreVerifierPASSAuthorityFailureRaisesAttentionWithoutVerdict(t *testing.T) {
	fixture := newVerifierLifecycleFixture(t)
	ctx := context.Background()
	fixture.dispatch(t, "cmd-pass-stop-dispatch")
	fixture.execute(t, engine.VerdictPass, false)
	state, plan, passRequest, prepared, err := fixture.control.PrepareControlledVerdictAdmission(
		ctx, fixture.ownership, fixture.ownerID, fixture.runID, fixture.workID, "cmd-pass-stop-verdict",
	)
	if err != nil || passRequest.Outcome != string(engine.VerdictPass) {
		t.Fatalf("prepare stopped PASS = %+v, %v", passRequest, err)
	}
	stalePermit, err := fixture.authority.AuthorizePASSAdmission(ctx, plan, passRequest)
	if err != nil {
		t.Fatal(err)
	}
	revoked, _, _ := authorityFixture(
		t, fixture.control, fixture.plan, 3, fixture.privateKey, false,
		controlledSourceMutation(fixture.plan, func(source map[string]any) {
			source["status"] = "revoked"
			source["maximum_grants"] = []any{}
		}),
	)
	if _, err := revoked.Approve(ctx, fixture.plan); err == nil || !strings.Contains(err.Error(), "revoked") {
		t.Fatalf("persist revoked PASS authority head = %v", err)
	}
	if permit, err := fixture.authority.AuthorizePASSAdmission(ctx, plan, passRequest); err == nil ||
		permit.Facts() != (policy.PASSAdmissionPermitFacts{}) {
		t.Fatalf("fresh PASS authorization after revocation = %+v, %v", permit.Facts(), err)
	}
	command := testCommand(
		t, "cmd-pass-stop-verdict", engine.CommandAdmitVerdict, state.Revision,
		engine.AdmitVerdictPayload{WorkID: fixture.workID},
	)
	if result, err := fixture.control.ApplyControlledPASSAdmission(
		ctx, fixture.ownership, fixture.authority, stalePermit, passRequest, prepared, command,
	); err == nil {
		t.Fatalf("stale PASS permit admitted verdict: %+v", result)
	}
	unchanged, err := fixture.control.State(ctx, fixture.runID)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.Revision != state.Revision || unchanged.Work[0].State != engine.WorkReviewable ||
		unchanged.Work[0].VerdictBinding != (engine.VerdictBinding{}) || len(unchanged.Attention) != 0 {
		t.Fatalf("failed PASS admission mutated delivery truth: %+v", unchanged)
	}
	assertCount(t, fixture.control, "verdict_records", 0)
	attention, err := PASSAttentionCommand(
		"cmd-pass-stop-attention", fixture.runID, fixture.workID, state.Revision,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := fixture.control.Apply(ctx, attention); err == nil {
		t.Fatalf("raw Store applied controlled PASS attention: %+v", result)
	}
	result, err := fixture.control.ApplyControlledPASSAttention(
		ctx, fixture.ownership, fixture.ownerID, prepared, attention,
	)
	if err != nil || result.Outcome != OutcomeApplied || result.Revision != state.Revision+1 {
		t.Fatalf("controlled PASS attention = %+v, %v", result, err)
	}
	stopped, err := fixture.control.State(ctx, fixture.runID)
	if err != nil {
		t.Fatal(err)
	}
	if stopped.Work[0].State != engine.WorkReviewable ||
		stopped.Work[0].VerdictBinding != (engine.VerdictBinding{}) || len(stopped.Attention) != 1 ||
		!protocol.ValidNonEmpty(stopped.Attention[0]) {
		t.Fatalf("PASS authority stop projection = %+v", stopped)
	}
	assertCount(t, fixture.control, "verdict_records", 0)
}

func TestStoreVerifierRetryRetainsVerdictAndAdvancesEpoch(t *testing.T) {
	fixture := newVerifierLifecycleFixture(t)
	ctx := context.Background()
	fixture.dispatch(t, "cmd-retry-dispatch-1")
	fixture.execute(t, engine.VerdictInconclusive, false)
	fixture.admit(t, engine.VerdictInconclusive, "cmd-retry-verdict-1")
	first, err := fixture.control.State(ctx, fixture.runID)
	if err != nil {
		t.Fatal(err)
	}
	priorVerdict := first.Work[0].VerdictBinding
	if first.Work[0].State != engine.WorkRetry || first.Work[0].VerificationEpoch != 1 ||
		first.Work[0].VerdictEpoch != 1 {
		t.Fatalf("first retry state = %+v", first.Work[0])
	}
	fixture.dispatch(t, "cmd-retry-dispatch-2")
	dispatched, err := fixture.control.State(ctx, fixture.runID)
	if err != nil {
		t.Fatal(err)
	}
	if dispatched.Work[0].State != engine.WorkRetry || dispatched.Work[0].VerdictBinding != priorVerdict ||
		dispatched.Work[0].VerificationEpoch != 2 || dispatched.Work[0].VerdictEpoch != 1 {
		t.Fatalf("fresh retry dispatch lost current verdict: %+v", dispatched.Work[0])
	}
	fixture.execute(t, engine.VerdictPass, false)
	fixture.admit(t, engine.VerdictPass, "cmd-retry-verdict-2")
	verified, err := fixture.control.State(ctx, fixture.runID)
	if err != nil {
		t.Fatal(err)
	}
	if verified.Work[0].State != engine.WorkVerified || verified.Work[0].Verdict != engine.VerdictPass ||
		verified.Work[0].VerdictBinding == priorVerdict || verified.Work[0].VerificationEpoch != 2 ||
		verified.Work[0].VerdictEpoch != 2 {
		t.Fatalf("fresh retry PASS = %+v", verified.Work[0])
	}
	assertCount(t, fixture.control, "verifier_dispatch_records", 2)
	assertCount(t, fixture.control, "verdict_records", 2)
}

func TestStoreVerifierFreshEpochCeilingIsDurable(t *testing.T) {
	fixture := newVerifierLifecycleFixture(t)
	ctx := context.Background()
	for epoch := int64(1); epoch <= engine.MaximumVerificationEpoch; epoch++ {
		fixture.dispatch(t, "cmd-ceiling-dispatch-"+string(rune('0'+epoch)))
		fixture.execute(t, engine.VerdictInconclusive, false)
		fixture.admit(t, engine.VerdictInconclusive, "cmd-ceiling-verdict-"+string(rune('0'+epoch)))
	}
	before, err := fixture.control.State(ctx, fixture.runID)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, prepared, err := fixture.control.PrepareControlledVerifierDispatch(
		ctx, fixture.ownership, fixture.ownerID, fixture.runID, fixture.workID, "cmd-ceiling-dispatch-over",
	)
	if err == nil || prepared.issuer != nil ||
		prepared.commandID != "" || prepared.runID != "" || prepared.workID != "" {
		t.Fatalf("dispatch beyond epoch ceiling = %+v, %v", prepared, err)
	}
	after, err := fixture.control.State(ctx, fixture.runID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != before.Revision || after.Work[0].VerificationEpoch != engine.MaximumVerificationEpoch ||
		after.Work[0].VerdictEpoch != engine.MaximumVerificationEpoch || after.Work[0].State != engine.WorkAttention ||
		after.Work[0].NextAction != engine.ActionReplan || after.Work[0].Attention == "" {
		t.Fatalf("epoch ceiling changed state: before %+v after %+v", before.Work[0], after.Work[0])
	}
	assertCount(t, fixture.control, "verifier_dispatch_records", int(engine.MaximumVerificationEpoch))
	assertCount(t, fixture.control, "verdict_records", int(engine.MaximumVerificationEpoch))
}

func TestStoreVerifierBoundResultRecoversWithoutAnotherTurn(t *testing.T) {
	fixture := newVerifierLifecycleFixture(t)
	ctx := context.Background()
	fixture.dispatch(t, "cmd-recovery-dispatch")
	request, _, prepared := fixture.claimAndPrepare(t)
	result := fixture.resultFor(t, request.VerifierEffectID, engine.VerdictFail)
	turns := 0
	runResult, err := prepared.RunVerifier(func(effect engine.JournalEffect) (json.RawMessage, error) {
		turns++
		if effect.ID != request.VerifierEffectID {
			t.Fatalf("recovery verifier invocation = %+v", effect)
		}
		return result, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.control.BindAuthorizedVerifierResult(ctx, prepared, runResult); err != nil {
		t.Fatal(err)
	}
	if err := fixture.ownership.Close(); err != nil {
		t.Fatal(err)
	}
	fixture.ownership = nil
	recoveryOwner := "verifier-recovery-controller"
	recovery, err := fixture.control.AcquireControllerOwnership(recoveryOwner)
	if err != nil {
		t.Fatal(err)
	}
	fixture.ownership, fixture.ownerID = recovery, recoveryOwner
	if recovered, err := fixture.control.RecoverControlledInterruptedEffects(
		ctx, recovery, recoveryOwner, "verifier controller stopped after binding",
	); err != nil || recovered != 1 {
		t.Fatalf("interrupt bound verifier = %d, %v", recovered, err)
	}
	unknown, err := loadEffect(ctx, fixture.control.db, request.VerifierEffectID)
	if err != nil || unknown.State != EffectUnknown || len(unknown.Result) == 0 {
		t.Fatalf("interrupted bound verifier = %+v, %v", unknown, err)
	}
	if err := fixture.control.RecoverBoundEffect(
		ctx, request.VerifierEffectID, 1, recoveryOwner,
	); err == nil {
		t.Fatal("raw recovery closed a controlled verifier")
	}
	if err := fixture.control.RecoverControlledBoundEffect(
		ctx, recovery, recoveryOwner, request.VerifierEffectID, 1,
	); err != nil {
		t.Fatal(err)
	}
	if err := recovery.Activate(ctx, fixture.control, recoveryOwner); err != nil {
		t.Fatal(err)
	}
	if turns != 1 {
		t.Fatalf("bound recovery repeated verifier turn %d times", turns)
	}
	succeeded, err := loadEffect(ctx, fixture.control.db, request.VerifierEffectID)
	if err != nil || succeeded.State != EffectSucceeded || len(succeeded.Result) == 0 {
		t.Fatalf("recovered verifier = %+v, %v", succeeded, err)
	}
	fixture.admit(t, engine.VerdictFail, "cmd-recovery-verdict")
	state, err := fixture.control.State(ctx, fixture.runID)
	if err != nil || state.Work[0].State != engine.WorkRepair || state.Work[0].Verdict != engine.VerdictFail {
		t.Fatalf("recovered verifier admission = %+v, %v", state, err)
	}
}
