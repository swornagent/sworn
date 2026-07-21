package engine

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/protocol"
)

const (
	testVerifierProfileDigest          = "sha256:3333333333333333333333333333333333333333333333333333333333333333"
	testVerifierDispatchDigest         = "sha256:4444444444444444444444444444444444444444444444444444444444444444"
	testVerifierAssessmentDigest       = "sha256:5555555555555555555555555555555555555555555555555555555555555555"
	testVerdictDigest                  = "sha256:6666666666666666666666666666666666666666666666666666666666666666"
	testCandidateTree                  = "7777777777777777777777777777777777777777"
	testVerifierExecutionReceiptDigest = "sha256:8888888888888888888888888888888888888888888888888888888888888888"
)

func TestVerifierDispatchRequiresFactsAndClosesExactEffect(t *testing.T) {
	t.Parallel()

	current := reviewableState(t)
	before := cloneState(current)
	intent := command(t, "cmd-verifier", current.RunID, CommandDispatchVerifier, current.Revision,
		DispatchVerifierPayload{WorkID: "work-1"})
	decision, err := Reduce(&current, intent)
	if !errors.Is(err, ErrVerifierDispatchFactsRequired) || !reflect.DeepEqual(decision, Decision{}) {
		t.Fatalf("intent-only verifier dispatch = %+v, %v", decision, err)
	}
	if !reflect.DeepEqual(current, before) {
		t.Fatal("intent-only verifier dispatch mutated its input")
	}

	facts := verifierDispatchFacts(1, "verifier-dispatch-1")
	dispatched, err := ReduceVerifierDispatch(&current, intent, facts)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := ReduceVerifierDispatch(&current, intent, facts)
	if err != nil || !reflect.DeepEqual(repeated, dispatched) {
		t.Fatalf("verifier dispatch is not deterministic: %v\nfirst: %+v\nagain: %+v", err, dispatched, repeated)
	}
	work := dispatched.State.Work[0]
	if work.State != WorkReviewable || work.NextAction != ActionVerify ||
		work.VerificationDispatchID != facts.DispatchID || work.VerificationEpoch != 1 ||
		work.VerdictBinding != (VerdictBinding{}) || work.VerdictEpoch != 0 {
		t.Fatalf("dispatched work = %+v", work)
	}
	if dispatched.State.Revision != current.Revision+1 || dispatched.Event.Kind != "verifier.dispatched" ||
		len(dispatched.Effects) != 1 || dispatched.Effects[0].Kind != EffectVerifier {
		t.Fatalf("verifier dispatch decision = %+v", dispatched)
	}
	request, err := ParseVerifierEffectRequest(dispatched.Effects[0].Request)
	if err != nil {
		t.Fatal(err)
	}
	if request.DeliveryRunID != current.RunID || request.DeliveryID != current.DeliveryID ||
		request.WorkID != work.ID || request.WorkAttempt != work.Attempt || request.PlanDigest != current.PlanDigest ||
		request.SubmissionID != work.SubmissionID || request.SubmissionDigest != work.SubmissionDigest ||
		request.Candidate != facts.Candidate || request.DispatchID != facts.DispatchID ||
		request.DispatchReceipt != facts.DispatchReceipt || request.VerifierProfileDigest != facts.VerifierProfileDigest ||
		request.Agent != facts.Agent || request.VerificationEpoch != facts.VerificationEpoch {
		t.Fatalf("verifier request = %+v", request)
	}
	if strings.Contains(string(intent.Payload), "dispatch") || strings.Contains(string(intent.Payload), "profile") ||
		strings.Contains(string(intent.Payload), "submission") {
		t.Fatalf("caller intent contains Store facts: %s", intent.Payload)
	}
}

func TestVerdictAdmissionRoutesEveryOutcome(t *testing.T) {
	t.Parallel()

	tests := []struct {
		outcome   VerdictOutcome
		state     WorkState
		action    NextAction
		attention bool
	}{
		{VerdictPass, WorkVerified, ActionReplan, false},
		{VerdictFail, WorkRepair, ActionRepair, false},
		{VerdictSpecBlock, WorkBlocked, ActionReplan, true},
		{VerdictInconclusive, WorkRetry, ActionRetryVerification, false},
	}
	for _, test := range tests {
		test := test
		t.Run(string(test.outcome), func(t *testing.T) {
			t.Parallel()
			current := dispatchedReviewableState(t, 1, "verifier-dispatch-1")
			current.Attention = []string{"PASS admission awaits refreshed current authority"}
			if err := current.Validate(); err != nil {
				t.Fatalf("state with pending delivery attention: %v", err)
			}
			before := cloneState(current)
			intent := command(t, "cmd-verdict-"+strings.ToLower(string(test.outcome)), current.RunID,
				CommandAdmitVerdict, current.Revision, AdmitVerdictPayload{WorkID: "work-1"})
			decision, err := Reduce(&current, intent)
			if !errors.Is(err, ErrVerdictAdmissionFactsRequired) || !reflect.DeepEqual(decision, Decision{}) {
				t.Fatalf("intent-only verdict admission = %+v, %v", decision, err)
			}
			facts := verdictAdmissionFacts(1, "verifier-dispatch-1", test.outcome)
			admitted, err := ReduceVerdictAdmission(&current, intent, facts)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(current, before) {
				t.Fatal("verdict admission mutated its input")
			}
			work := admitted.State.Work[0]
			if work.State != test.state || work.NextAction != test.action || work.VerdictBinding != facts.VerdictBinding ||
				work.VerdictEpoch != 1 || work.VerificationEpoch != 1 || work.SubmissionBinding != before.Work[0].SubmissionBinding {
				t.Fatalf("admitted work = %+v", work)
			}
			if (work.Attention != "") != test.attention ||
				(test.attention && !protocol.ValidNonEmpty(work.Attention)) {
				t.Fatalf("attention = %q, want present %t", work.Attention, test.attention)
			}
			if len(admitted.State.Attention) != 0 {
				t.Fatalf("admitted verdict retained delivery attention: %v", admitted.State.Attention)
			}
			if admitted.State.Revision != current.Revision+1 || admitted.Event.Kind != "verdict.admitted" ||
				len(admitted.Effects) != 0 {
				t.Fatalf("verdict decision = %+v", admitted)
			}
		})
	}
}

func TestRepeatedVerificationRetainsCurrentVerdictUntilFreshAdmission(t *testing.T) {
	t.Parallel()

	first := dispatchedReviewableState(t, 1, "verifier-dispatch-1")
	inconclusiveIntent := command(t, "cmd-verdict-1", first.RunID, CommandAdmitVerdict, first.Revision,
		AdmitVerdictPayload{WorkID: "work-1"})
	inconclusive, err := ReduceVerdictAdmission(
		&first, inconclusiveIntent, verdictAdmissionFacts(1, "verifier-dispatch-1", VerdictInconclusive),
	)
	if err != nil {
		t.Fatal(err)
	}
	prior := inconclusive.State.Work[0].VerdictBinding

	// The current verdict cannot be replaced without a strictly newer dispatch.
	secondVerdictIntent := command(t, "cmd-verdict-without-dispatch", first.RunID, CommandAdmitVerdict,
		inconclusive.State.Revision, AdmitVerdictPayload{WorkID: "work-1"})
	if _, err := ReduceVerdictAdmission(
		&inconclusive.State, secondVerdictIntent,
		verdictAdmissionFacts(1, "verifier-dispatch-1", VerdictPass),
	); err == nil {
		t.Fatal("a second verdict was admitted without a fresh verifier dispatch")
	}

	dispatchIntent := command(t, "cmd-verifier-2", first.RunID, CommandDispatchVerifier,
		inconclusive.State.Revision, DispatchVerifierPayload{WorkID: "work-1"})
	dispatched, err := ReduceVerifierDispatch(
		&inconclusive.State, dispatchIntent, verifierDispatchFacts(2, "verifier-dispatch-2"),
	)
	if err != nil {
		t.Fatal(err)
	}
	work := dispatched.State.Work[0]
	if work.State != WorkRetry || work.NextAction != ActionRetryVerification || work.VerdictBinding != prior ||
		work.VerdictEpoch != 1 || work.VerificationEpoch != 2 || work.VerificationDispatchID != "verifier-dispatch-2" {
		t.Fatalf("repeat dispatch erased or misrepresented current verdict: %+v", work)
	}

	passIntent := command(t, "cmd-verdict-2", first.RunID, CommandAdmitVerdict,
		dispatched.State.Revision, AdmitVerdictPayload{WorkID: "work-1"})
	passed, err := ReduceVerdictAdmission(
		&dispatched.State, passIntent, verdictAdmissionFacts(2, "verifier-dispatch-2", VerdictPass),
	)
	if err != nil || passed.State.Work[0].State != WorkVerified || passed.State.Work[0].VerdictEpoch != 2 {
		t.Fatalf("fresh verdict admission = %+v, %v", passed, err)
	}
}

func TestVerifierDispatchHasHardFreshEpochCeiling(t *testing.T) {
	t.Parallel()

	current := dispatchedReviewableState(t, 1, "verifier-dispatch-1")
	for epoch := int64(1); epoch <= MaximumVerificationEpoch; epoch++ {
		verdictIntent := command(t, "cmd-verdict-ceiling-"+string(rune('0'+epoch)), current.RunID,
			CommandAdmitVerdict, current.Revision, AdmitVerdictPayload{WorkID: "work-1"})
		admitted, err := ReduceVerdictAdmission(
			&current, verdictIntent,
			verdictAdmissionFacts(epoch, current.Work[0].VerificationDispatchID, VerdictInconclusive),
		)
		if err != nil {
			t.Fatalf("admit epoch %d: %v", epoch, err)
		}
		current = admitted.State
		if epoch == MaximumVerificationEpoch {
			break
		}
		nextEpoch := epoch + 1
		dispatchIntent := command(t, "cmd-dispatch-ceiling-"+string(rune('0'+nextEpoch)), current.RunID,
			CommandDispatchVerifier, current.Revision, DispatchVerifierPayload{WorkID: "work-1"})
		dispatched, err := ReduceVerifierDispatch(
			&current, dispatchIntent,
			verifierDispatchFacts(nextEpoch, "verifier-dispatch-"+string(rune('0'+nextEpoch))),
		)
		if err != nil {
			t.Fatalf("dispatch epoch %d: %v", nextEpoch, err)
		}
		current = dispatched.State
	}
	over := command(t, "cmd-dispatch-over-ceiling", current.RunID, CommandDispatchVerifier, current.Revision,
		DispatchVerifierPayload{WorkID: "work-1"})
	exhausted := current.Work[0]
	if exhausted.State != WorkAttention || exhausted.Verdict != VerdictInconclusive ||
		exhausted.NextAction != ActionReplan || exhausted.Attention != verificationExhaustedAttention {
		t.Fatalf("exhausted verification route = %+v", exhausted)
	}
	invalidRetry := cloneState(current)
	invalidRetry.Work[0].State = WorkRetry
	invalidRetry.Work[0].NextAction = ActionRetryVerification
	invalidRetry.Work[0].Attention = ""
	if err := invalidRetry.Validate(); err == nil {
		t.Fatal("maximum-epoch INCONCLUSIVE advertised an impossible retry")
	}
	_, err := Reduce(&current, over)
	assertRejection(t, err, "invalid_transition")
}

func TestVerifierReducersRejectChangedStoreBindings(t *testing.T) {
	t.Parallel()

	current := reviewableState(t)
	intent := command(t, "cmd-verifier-invalid", current.RunID, CommandDispatchVerifier, current.Revision,
		DispatchVerifierPayload{WorkID: "work-1"})
	mutations := map[string]func(*VerifierDispatchFacts){
		"plan":              func(value *VerifierDispatchFacts) { value.PlanDigest = testAuthorityDigest },
		"submission id":     func(value *VerifierDispatchFacts) { value.SubmissionID = "submission-2" },
		"submission digest": func(value *VerifierDispatchFacts) { value.SubmissionDigest = testAuthorityDigest },
		"repository":        func(value *VerifierDispatchFacts) { value.Candidate.Repository = "repo-2" },
		"commit":            func(value *VerifierDispatchFacts) { value.Candidate.Commit = strings.Repeat("8", 40) },
		"tree":              func(value *VerifierDispatchFacts) { value.Candidate.Tree = "bad" },
		"dispatch":          func(value *VerifierDispatchFacts) { value.DispatchID = "bad id" },
		"receipt":           func(value *VerifierDispatchFacts) { value.DispatchReceipt.Digest = "sha256:no" },
		"profile":           func(value *VerifierDispatchFacts) { value.VerifierProfileDigest = "sha256:no" },
		"agent":             func(value *VerifierDispatchFacts) { value.Agent = " \t" },
		"epoch":             func(value *VerifierDispatchFacts) { value.VerificationEpoch = 2 },
	}
	for name, mutate := range mutations {
		facts := verifierDispatchFacts(1, "verifier-dispatch-1")
		mutate(&facts)
		if _, err := ReduceVerifierDispatch(&current, intent, facts); err == nil {
			t.Fatalf("changed %s binding accepted", name)
		}
	}

	dispatched := dispatchedReviewableState(t, 1, "verifier-dispatch-1")
	verdictIntent := command(t, "cmd-verdict-invalid", dispatched.RunID, CommandAdmitVerdict, dispatched.Revision,
		AdmitVerdictPayload{WorkID: "work-1"})
	verdictMutations := map[string]func(*VerdictAdmissionFacts){
		"dispatch":   func(value *VerdictAdmissionFacts) { value.DispatchID = "verifier-dispatch-2" },
		"epoch":      func(value *VerdictAdmissionFacts) { value.VerificationEpoch = 2 },
		"verdict id": func(value *VerdictAdmissionFacts) { value.VerdictID = "bad id" },
		"digest":     func(value *VerdictAdmissionFacts) { value.VerdictDigest = "sha256:no" },
		"outcome":    func(value *VerdictAdmissionFacts) { value.Verdict = "MAYBE" },
	}
	for name, mutate := range verdictMutations {
		facts := verdictAdmissionFacts(1, "verifier-dispatch-1", VerdictPass)
		mutate(&facts)
		if _, err := ReduceVerdictAdmission(&dispatched, verdictIntent, facts); err == nil {
			t.Fatalf("changed verdict %s binding accepted", name)
		}
	}
}

func TestVerifierAndAttentionIntentsRejectCallerSelectedFacts(t *testing.T) {
	t.Parallel()

	current := reviewableState(t)
	commands := []Command{
		{
			ID: "cmd-caller-verifier", RunID: current.RunID, Kind: CommandDispatchVerifier,
			ExpectedRevision: current.Revision,
			Payload:          json.RawMessage(`{"work_id":"work-1","submission_digest":"` + testSubmissionDigest + `"}`),
		},
		{
			ID: "cmd-caller-verdict", RunID: current.RunID, Kind: CommandAdmitVerdict,
			ExpectedRevision: current.Revision,
			Payload:          json.RawMessage(`{"work_id":"work-1","verdict":"PASS"}`),
		},
		{
			ID: "cmd-caller-attention", RunID: current.RunID, Kind: CommandRaiseDeliveryAttention,
			ExpectedRevision: current.Revision,
			Payload:          json.RawMessage(`{"work_id":"work-1","message":"invented model outcome"}`),
		},
	}
	for _, command := range commands {
		decision, err := Reduce(&current, command)
		if !reflect.DeepEqual(decision, Decision{}) {
			t.Fatalf("caller facts produced a decision for %s: %+v", command.Kind, decision)
		}
		assertRejection(t, err, "invalid_payload")
	}
}

func TestDeliveryAttentionRequiresFactsAndPreservesWorkTruth(t *testing.T) {
	t.Parallel()

	current := dispatchedReviewableState(t, 1, "verifier-dispatch-1")
	before := cloneState(current)
	intent := command(t, "cmd-attention", current.RunID, CommandRaiseDeliveryAttention, current.Revision,
		RaiseDeliveryAttentionPayload{WorkID: "work-1", Code: "pass-authority-lost"})
	decision, err := Reduce(&current, intent)
	if !errors.Is(err, ErrDeliveryAttentionFactsRequired) || !reflect.DeepEqual(decision, Decision{}) {
		t.Fatalf("intent-only attention = %+v, %v", decision, err)
	}
	raised, err := ReduceDeliveryAttention(
		&current, intent, DeliveryAttentionFacts{Message: "current authority no longer permits PASS admission"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(current, before) || !reflect.DeepEqual(raised.State.Work, before.Work) {
		t.Fatal("delivery attention mutated existing work truth")
	}
	if raised.State.Revision != current.Revision+1 ||
		!reflect.DeepEqual(raised.State.Attention, []string{"current authority no longer permits PASS admission"}) ||
		raised.Event.Kind != "delivery.attention_raised" || len(raised.Effects) != 0 {
		t.Fatalf("attention decision = %+v", raised)
	}
	if _, err := ReduceDeliveryAttention(&current, intent, DeliveryAttentionFacts{Message: " \t"}); err == nil {
		t.Fatal("blank Store-derived attention accepted")
	}
}

func TestVerifierStateRejectsCrossOutcomeAndEpochDrift(t *testing.T) {
	t.Parallel()

	valid := dispatchedReviewableState(t, 1, "verifier-dispatch-1")
	intent := command(t, "cmd-state-verdict", valid.RunID, CommandAdmitVerdict, valid.Revision,
		AdmitVerdictPayload{WorkID: "work-1"})
	decision, err := ReduceVerdictAdmission(
		&valid, intent, verdictAdmissionFacts(1, "verifier-dispatch-1", VerdictFail),
	)
	if err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(*State){
		"wrong outcome":          func(state *State) { state.Work[0].Verdict = VerdictPass },
		"missing verdict":        func(state *State) { state.Work[0].VerdictBinding = VerdictBinding{} },
		"missing dispatch":       func(state *State) { state.Work[0].VerificationDispatchID = "" },
		"future verdict":         func(state *State) { state.Work[0].VerdictEpoch = 2 },
		"unexpected attention":   func(state *State) { state.Work[0].Attention = "wrong" },
		"bad delivery attention": func(state *State) { state.Attention = []string{" \t"} },
	}
	for name, mutate := range mutations {
		state := cloneState(decision.State)
		mutate(&state)
		if err := state.Validate(); err == nil {
			t.Fatalf("state with %s was accepted", name)
		}
	}
}

func TestVerifierEffectPayloadsAndAttemptIdentityAreStrictlyBound(t *testing.T) {
	t.Parallel()

	request := verifierEffectRequest(1, "verifier-dispatch-1")
	encodedRequest, err := EncodeVerifierEffectRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	parsedRequest, err := ParseVerifierEffectRequest(encodedRequest)
	if err != nil || parsedRequest != request {
		t.Fatalf("parsed verifier request = %+v, %v", parsedRequest, err)
	}
	result := verifierEffectResult(1, "verifier-dispatch-1")
	encodedResult, err := EncodeVerifierEffectResult(result)
	if err != nil {
		t.Fatal(err)
	}
	parsedResult, err := ParseVerifierEffectResult(encodedResult)
	if err != nil || parsedResult != result {
		t.Fatalf("parsed verifier result = %+v, %v", parsedResult, err)
	}
	if err := ValidateEffectResult(EffectVerifier, request.DispatchID, encodedRequest, encodedResult); err != nil {
		t.Fatal(err)
	}
	if err := ValidateEffectResult(EffectVerifier, "verifier-dispatch-other", encodedRequest, encodedResult); err == nil {
		t.Fatal("verifier request from another effect was accepted")
	}
	changedResult := result
	changedResult.DispatchID = "verifier-dispatch-2"
	changedResultBytes, err := EncodeVerifierEffectResult(changedResult)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateEffectResult(EffectVerifier, request.DispatchID, encodedRequest, changedResultBytes); err == nil {
		t.Fatal("result from another verifier dispatch was accepted")
	}
	if _, err := ParseVerifierEffectRequest(append(json.RawMessage{' '}, encodedRequest...)); err == nil {
		t.Fatal("noncanonical verifier request accepted")
	}
	if _, err := ParseVerifierEffectResult(append(json.RawMessage{'\n'}, encodedResult...)); err == nil {
		t.Fatal("noncanonical verifier result accepted")
	}
	requestMutations := map[string]func(*VerifierEffectRequest){
		"schema":     func(value *VerifierEffectRequest) { value.SchemaVersion = "sworn-verifier-effect-request-v2" },
		"run":        func(value *VerifierEffectRequest) { value.DeliveryRunID = "bad id" },
		"plan":       func(value *VerifierEffectRequest) { value.PlanDigest = "sha256:no" },
		"submission": func(value *VerifierEffectRequest) { value.SubmissionID = "bad id" },
		"candidate":  func(value *VerifierEffectRequest) { value.Candidate.Tree = "bad" },
		"dispatch":   func(value *VerifierEffectRequest) { value.DispatchID = "bad id" },
		"receipt":    func(value *VerifierEffectRequest) { value.DispatchReceipt.MediaType = "text/plain" },
		"profile":    func(value *VerifierEffectRequest) { value.VerifierProfileDigest = "sha256:no" },
		"agent":      func(value *VerifierEffectRequest) { value.Agent = " \t" },
		"epoch":      func(value *VerifierEffectRequest) { value.VerificationEpoch = MaximumVerificationEpoch + 1 },
	}
	for name, mutate := range requestMutations {
		invalid := request
		mutate(&invalid)
		if _, err := EncodeVerifierEffectRequest(invalid); err == nil {
			t.Fatalf("invalid verifier request %s encoded", name)
		}
	}
	resultMutations := map[string]func(*VerifierEffectResult){
		"schema":            func(value *VerifierEffectResult) { value.SchemaVersion = "sworn-verifier-effect-result-v2" },
		"outcome":           func(value *VerifierEffectResult) { value.Outcome = "PASS" },
		"dispatch":          func(value *VerifierEffectResult) { value.DispatchID = "bad id" },
		"epoch":             func(value *VerifierEffectResult) { value.VerificationEpoch = 0 },
		"assessment":        func(value *VerifierEffectResult) { value.Assessment.Digest = "sha256:no" },
		"execution receipt": func(value *VerifierEffectResult) { value.ExecutionReceipt.MediaType = "application/json" },
		"start":             func(value *VerifierEffectResult) { value.StartedAt = "not-a-time" },
		"order":             func(value *VerifierEffectResult) { value.StartedAt = "2026-07-21T00:00:02Z" },
	}
	for name, mutate := range resultMutations {
		invalid := result
		mutate(&invalid)
		if _, err := EncodeVerifierEffectResult(invalid); err == nil {
			t.Fatalf("invalid verifier result %s encoded", name)
		}
	}

	first, err := VerifierAttemptIdentityFor(
		"effect-verifier-1", 1, request.DispatchID, request.DispatchReceipt.Digest,
		request.VerifierProfileDigest, request.Agent, request.VerificationEpoch,
	)
	if err != nil {
		t.Fatal(err)
	}
	second, err := VerifierAttemptIdentityFor(
		"effect-verifier-1", 2, request.DispatchID, request.DispatchReceipt.Digest,
		request.VerifierProfileDigest, request.Agent, request.VerificationEpoch,
	)
	if err != nil || first.InvocationID == second.InvocationID || !ValidID(first.InvocationID) {
		t.Fatalf("attempt identities = %+v, %+v, %v", first, second, err)
	}
	encodedIdentity, err := EncodeVerifierAttemptIdentity(first)
	if err != nil {
		t.Fatal(err)
	}
	parsedIdentity, err := ParseVerifierAttemptIdentity(encodedIdentity)
	if err != nil || parsedIdentity != first {
		t.Fatalf("parsed verifier attempt = %+v, %v", parsedIdentity, err)
	}
	forged := first
	forged.VerificationEpoch++
	if _, err := EncodeVerifierAttemptIdentity(forged); err == nil {
		t.Fatal("forged verifier attempt identity accepted")
	}
}

func reviewableState(t *testing.T) State {
	t.Helper()
	state := State{
		SchemaVersion: StateSchemaVersion,
		RunID:         "run-1", DeliveryID: "delivery-1", PlanDigest: testPlanDigest,
		Repository: "repo-1", TargetRef: "refs/heads/main", Revision: 4,
		Phase: PhaseActive, AuthorityReceiptDigest: testAuthorityDigest,
		Work: []Work{{
			ID: "work-1", State: WorkReviewable, Attempt: 1,
			SubmissionBinding: SubmissionBinding{
				SubmissionID: "submission-1", SubmissionDigest: testSubmissionDigest,
				CandidateCommit: testCandidateCommit,
			},
			NextAction: ActionVerify,
		}},
	}
	if err := state.Validate(); err != nil {
		t.Fatal(err)
	}
	return state
}

func dispatchedReviewableState(t *testing.T, epoch int64, dispatchID string) State {
	t.Helper()
	current := reviewableState(t)
	intent := command(t, "cmd-dispatch-fixture-"+dispatchID, current.RunID, CommandDispatchVerifier,
		current.Revision, DispatchVerifierPayload{WorkID: "work-1"})
	decision, err := ReduceVerifierDispatch(&current, intent, verifierDispatchFacts(epoch, dispatchID))
	if err != nil {
		t.Fatal(err)
	}
	return decision.State
}

func verifierDispatchFacts(epoch int64, dispatchID string) VerifierDispatchFacts {
	return VerifierDispatchFacts{
		PlanDigest: testPlanDigest, SubmissionID: "submission-1", SubmissionDigest: testSubmissionDigest,
		Candidate: protocol.CandidatePoint{
			Repository: "repo-1", Commit: testCandidateCommit, Tree: testCandidateTree,
		},
		DispatchID: dispatchID,
		DispatchReceipt: protocol.Artifact{
			Ref: testVerifierDispatchDigest, MediaType: verifierArtifactMediaType, Digest: testVerifierDispatchDigest,
		},
		VerifierProfileDigest: testVerifierProfileDigest, Agent: "codex-cli", VerificationEpoch: epoch,
	}
}

func verdictAdmissionFacts(epoch int64, dispatchID string, outcome VerdictOutcome) VerdictAdmissionFacts {
	return VerdictAdmissionFacts{
		DispatchID: dispatchID, VerificationEpoch: epoch,
		VerdictBinding: VerdictBinding{
			VerdictID: "verdict-" + dispatchID, VerdictDigest: testVerdictDigest, Verdict: outcome,
		},
	}
}

func verifierEffectRequest(epoch int64, dispatchID string) VerifierEffectRequest {
	facts := verifierDispatchFacts(epoch, dispatchID)
	return VerifierEffectRequest{
		SchemaVersion: VerifierEffectRequestSchemaVersion,
		DeliveryRunID: "run-1", DeliveryID: "delivery-1", WorkID: "work-1", WorkAttempt: 1,
		PlanDigest: facts.PlanDigest, SubmissionID: facts.SubmissionID, SubmissionDigest: facts.SubmissionDigest,
		Candidate: facts.Candidate, DispatchID: facts.DispatchID, DispatchReceipt: facts.DispatchReceipt,
		VerifierProfileDigest: facts.VerifierProfileDigest, Agent: facts.Agent, VerificationEpoch: epoch,
	}
}

func verifierEffectResult(epoch int64, dispatchID string) VerifierEffectResult {
	return VerifierEffectResult{
		SchemaVersion: VerifierEffectResultSchemaVersion, Outcome: VerifierOutcomeAssessmentReady,
		DispatchID: dispatchID, VerificationEpoch: epoch,
		Assessment: protocol.Artifact{
			Ref: testVerifierAssessmentDigest, MediaType: verifierArtifactMediaType, Digest: testVerifierAssessmentDigest,
		},
		ExecutionReceipt: protocol.Artifact{
			Ref: testVerifierExecutionReceiptDigest, MediaType: verifierExecutionReceiptMediaType,
			Digest: testVerifierExecutionReceiptDigest,
		},
		StartedAt: "2026-07-21T00:00:00Z", CompletedAt: "2026-07-21T00:00:01Z",
	}
}
