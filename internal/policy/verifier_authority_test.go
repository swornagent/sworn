package policy

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/protocol"
)

func TestAuthorityAuthorizeVerifierExecutionPersistsFreshObservationAndBindsFacts(t *testing.T) {
	fixture := newApprovalFixture(t)
	resolver := fixture.resolver()
	ledger := &recordingLedger{}
	service, err := newAuthorityWithClock(
		[]TrustRoot{fixture.root}, resolver, ledger, func() time.Time { return fixture.now },
	)
	if err != nil {
		t.Fatal(err)
	}
	request := verifierExecutionPermitRequest(t, fixture.plan)
	permit, err := service.AuthorizeVerifierExecution(context.Background(), fixture.plan, request)
	if err != nil {
		t.Fatalf("AuthorizeVerifierExecution() error = %v", err)
	}
	if resolver.calls != 1 || resolver.resolvedSourceRef != testSourceRef ||
		resolver.resolvedPlanDigest != fixture.plan.Record().Digest || len(ledger.sources) != 1 ||
		len(ledger.approvals) != 0 || !slices.Equal(ledger.events, []string{"source"}) {
		t.Fatalf("current verifier authority lifecycle = calls %d source %q plan %q sources %d approvals %d events %v",
			resolver.calls, resolver.resolvedSourceRef, resolver.resolvedPlanDigest,
			len(ledger.sources), len(ledger.approvals), ledger.events)
	}
	sourceFacts := ledger.sources[0].Facts()
	facts := permit.Facts()
	if facts.Purpose != VerifierExecutionPurpose || facts.ControllerID != request.ControllerID ||
		facts.RunID != request.RunID || facts.StateRevision != request.StateRevision ||
		facts.PlanDigest != fixture.plan.Record().Digest || facts.WorkID != request.WorkID ||
		facts.WorkAttempt != request.WorkAttempt || facts.WorkContractDigest != request.Contract.Digest() ||
		facts.SubmissionID != request.SubmissionID || facts.SubmissionDigest != request.SubmissionDigest ||
		facts.VerifierEffectID != request.VerifierEffectID || facts.DispatchID != request.DispatchID ||
		facts.DispatchDigest != request.DispatchDigest ||
		facts.VerifierProfileDigest != request.VerifierProfileDigest || facts.SourceRef != sourceFacts.SourceRef ||
		facts.SourceVersion != sourceFacts.SourceVersion || facts.SourceDigest != sourceFacts.SourceCanonicalDigest ||
		facts.AuthorizedAt != fixture.now.Format(time.RFC3339Nano) {
		t.Fatalf("verifier execution permit facts lost an exact binding: %#v", facts)
	}
	if err := service.ValidateVerifierExecutionPermit(permit, request); err != nil {
		t.Fatalf("ValidateVerifierExecutionPermit() error = %v", err)
	}

	changed := permit.Facts()
	changed.RunID = "forged-run"
	if permit.Facts().RunID != request.RunID {
		t.Fatal("verifier execution permit facts exposed mutable internal state")
	}
}

func TestAuthorityAuthorizeVerifierExecutionRequiresOnlyInspectAndExecute(t *testing.T) {
	fixture := newApprovalFixture(t)
	dropSourceGrant(t, &fixture, "edit")
	dropSourceGrant(t, &fixture, "commit")
	fixture.rebindSource(t)
	service, err := newAuthorityWithClock(
		[]TrustRoot{fixture.root}, fixture.resolver(), &recordingLedger{}, func() time.Time { return fixture.now },
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AuthorizeVerifierExecution(
		context.Background(), fixture.plan, verifierExecutionPermitRequest(t, fixture.plan),
	); err != nil {
		t.Fatalf("non-verifier ceiling reduction denied verifier execution: %v", err)
	}

	for _, action := range []string{"inspect", "execute"} {
		t.Run("current source missing "+action, func(t *testing.T) {
			denied := newApprovalFixture(t)
			dropSourceGrant(t, &denied, action)
			denied.rebindSource(t)
			ledger := &recordingLedger{}
			service, err := newAuthorityWithClock(
				[]TrustRoot{denied.root}, denied.resolver(), ledger, func() time.Time { return denied.now },
			)
			if err != nil {
				t.Fatal(err)
			}
			permit, err := service.AuthorizeVerifierExecution(
				context.Background(), denied.plan, verifierExecutionPermitRequest(t, denied.plan),
			)
			if err == nil || !strings.Contains(err.Error(), "source lacks "+action+" workspace grant") ||
				permit.Facts() != (VerifierExecutionPermitFacts{}) || len(ledger.sources) != 1 {
				t.Fatalf("missing %s grant = permit %#v, sources %d, error %v",
					action, permit.Facts(), len(ledger.sources), err)
			}
		})

		t.Run("exact plan missing "+action, func(t *testing.T) {
			denied := newApprovalFixture(t)
			denied.plan = planWithoutAuthorityGrant(t, denied.plan, action)
			denied.rebindSource(t)
			resolver := denied.resolver()
			service, err := newAuthorityWithClock(
				[]TrustRoot{denied.root}, resolver, &recordingLedger{}, func() time.Time { return denied.now },
			)
			if err != nil {
				t.Fatal(err)
			}
			permit, err := service.AuthorizeVerifierExecution(
				context.Background(), denied.plan, verifierExecutionPermitRequest(t, denied.plan),
			)
			if err == nil || !strings.Contains(err.Error(), "requires "+action+" workspace grant") ||
				permit.Facts() != (VerifierExecutionPermitFacts{}) || resolver.calls != 1 {
				t.Fatalf("plan missing %s = permit %#v, calls %d, error %v",
					action, permit.Facts(), resolver.calls, err)
			}
		})
	}
}

func TestCurrentVerifierExecutionPermitRejectsEveryChangedBinding(t *testing.T) {
	fixture := newApprovalFixture(t)
	now := fixture.now
	ledger := &recordingLedger{}
	service, err := newAuthorityWithClock(
		[]TrustRoot{fixture.root}, fixture.resolver(), ledger, func() time.Time { return now },
	)
	if err != nil {
		t.Fatal(err)
	}
	request := verifierExecutionPermitRequest(t, fixture.plan)
	permit, err := service.AuthorizeVerifierExecution(context.Background(), fixture.plan, request)
	if err != nil {
		t.Fatal(err)
	}
	otherPlan := planWithCreatedAt(t, fixture.plan, "2026-07-19T00:00:01Z")
	otherContract, exists := otherPlan.Work(request.WorkID)
	if !exists || otherContract.Digest() != request.Contract.Digest() || otherContract == request.Contract {
		t.Fatal("cross-plan contract fixture did not preserve only the work digest")
	}
	tests := []struct {
		name   string
		mutate func(*VerifierExecutionPermitRequest)
	}{
		{name: "controller", mutate: func(value *VerifierExecutionPermitRequest) { value.ControllerID = "controller-2" }},
		{name: "run", mutate: func(value *VerifierExecutionPermitRequest) { value.RunID = "verifier-run-2" }},
		{name: "revision", mutate: func(value *VerifierExecutionPermitRequest) { value.StateRevision++ }},
		{name: "work", mutate: func(value *VerifierExecutionPermitRequest) { value.WorkID = "work-2" }},
		{name: "attempt", mutate: func(value *VerifierExecutionPermitRequest) { value.WorkAttempt++ }},
		{name: "work contract", mutate: func(value *VerifierExecutionPermitRequest) { value.Contract = otherContract }},
		{name: "submission", mutate: func(value *VerifierExecutionPermitRequest) { value.SubmissionID = "submission-2" }},
		{name: "submission digest", mutate: func(value *VerifierExecutionPermitRequest) { value.SubmissionDigest = fixedDigest("d") }},
		{name: "effect", mutate: func(value *VerifierExecutionPermitRequest) { value.VerifierEffectID = "verifier-effect-2" }},
		{name: "dispatch", mutate: func(value *VerifierExecutionPermitRequest) { value.DispatchID = "verifier-dispatch-2" }},
		{name: "dispatch digest", mutate: func(value *VerifierExecutionPermitRequest) { value.DispatchDigest = fixedDigest("e") }},
		{name: "profile digest", mutate: func(value *VerifierExecutionPermitRequest) { value.VerifierProfileDigest = fixedDigest("f") }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := request
			test.mutate(&changed)
			if err := service.ValidateVerifierExecutionPermit(permit, changed); err == nil {
				t.Fatal("changed verifier execution binding was accepted")
			}
		})
	}

	t.Run("purpose", func(t *testing.T) {
		wrongPurpose := permit
		wrongPurpose.binding.facts.Purpose = PASSAdmissionPurpose
		if err := service.ValidateVerifierExecutionPermit(wrongPurpose, request); err == nil ||
			!strings.Contains(err.Error(), "wrong purpose") {
			t.Fatalf("wrong-purpose verifier permit error = %v", err)
		}
	})

	t.Run("foreign authority", func(t *testing.T) {
		foreign, err := newAuthorityWithClock(
			[]TrustRoot{fixture.root}, fixture.resolver(), ledger, func() time.Time { return now },
		)
		if err != nil {
			t.Fatal(err)
		}
		if err := foreign.ValidateVerifierExecutionPermit(permit, request); err == nil ||
			!strings.Contains(err.Error(), "another authority") {
			t.Fatalf("foreign verifier permit error = %v", err)
		}
	})

	t.Run("stale", func(t *testing.T) {
		now = fixture.now.Add(currentEffectPermitLifetime)
		if err := service.ValidateVerifierExecutionPermit(permit, request); err == nil ||
			!strings.Contains(err.Error(), "stale") {
			t.Fatalf("stale verifier permit error = %v", err)
		}
		now = fixture.now
	})
}

func TestAuthorityAuthorizePASSAdmissionPersistsFreshObservationAndBindsFacts(t *testing.T) {
	fixture := newApprovalFixture(t)
	resolver := fixture.resolver()
	ledger := &recordingLedger{}
	service, err := newAuthorityWithClock(
		[]TrustRoot{fixture.root}, resolver, ledger, func() time.Time { return fixture.now },
	)
	if err != nil {
		t.Fatal(err)
	}
	request := passAdmissionPermitRequest(t, fixture.plan)
	permit, err := service.AuthorizePASSAdmission(context.Background(), fixture.plan, request)
	if err != nil {
		t.Fatalf("AuthorizePASSAdmission() error = %v", err)
	}
	if resolver.calls != 1 || len(ledger.sources) != 1 || len(ledger.approvals) != 0 ||
		!slices.Equal(ledger.events, []string{"source"}) {
		t.Fatalf("current PASS authority lifecycle = calls %d sources %d approvals %d events %v",
			resolver.calls, len(ledger.sources), len(ledger.approvals), ledger.events)
	}
	sourceFacts := ledger.sources[0].Facts()
	facts := permit.Facts()
	if facts.Purpose != PASSAdmissionPurpose || facts.ControllerID != request.ControllerID ||
		facts.RunID != request.RunID || facts.StateRevision != request.StateRevision ||
		facts.PlanDigest != fixture.plan.Record().Digest || facts.WorkID != request.WorkID ||
		facts.WorkAttempt != request.WorkAttempt || facts.WorkContractDigest != request.Contract.Digest() ||
		facts.SubmissionID != request.SubmissionID || facts.SubmissionDigest != request.SubmissionDigest ||
		facts.VerifierEffectID != request.VerifierEffectID || facts.DispatchID != request.DispatchID ||
		facts.DispatchDigest != request.DispatchDigest || facts.AssessmentDigest != request.AssessmentDigest ||
		facts.Outcome != "PASS" || facts.SourceRef != sourceFacts.SourceRef ||
		facts.SourceVersion != sourceFacts.SourceVersion || facts.SourceDigest != sourceFacts.SourceCanonicalDigest ||
		facts.AuthorizedAt != fixture.now.Format(time.RFC3339Nano) {
		t.Fatalf("PASS admission permit facts lost an exact binding: %#v", facts)
	}
	if err := service.ValidatePASSAdmissionPermit(permit, request); err != nil {
		t.Fatalf("ValidatePASSAdmissionPermit() error = %v", err)
	}
}

func TestAuthorityAuthorizePASSAdmissionUsesExactPlanGrantCeilingOnly(t *testing.T) {
	t.Run("no extra workspace grant", func(t *testing.T) {
		fixture := newApprovalFixture(t)
		for _, action := range []string{"inspect", "edit", "execute", "commit"} {
			fixture.plan = planWithoutAuthorityGrant(t, fixture.plan, action)
		}
		fixture.rebindSource(t)
		service, err := newAuthorityWithClock(
			[]TrustRoot{fixture.root}, fixture.resolver(), &recordingLedger{}, func() time.Time { return fixture.now },
		)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.AuthorizePASSAdmission(
			context.Background(), fixture.plan, passAdmissionPermitRequest(t, fixture.plan),
		); err != nil {
			t.Fatalf("PASS admission invented a workspace grant requirement: %v", err)
		}
	})

	t.Run("current source must cover every exact plan grant", func(t *testing.T) {
		fixture := newApprovalFixture(t)
		dropSourceGrant(t, &fixture, "commit")
		fixture.rebindSource(t)
		ledger := &recordingLedger{}
		service, err := newAuthorityWithClock(
			[]TrustRoot{fixture.root}, fixture.resolver(), ledger, func() time.Time { return fixture.now },
		)
		if err != nil {
			t.Fatal(err)
		}
		permit, err := service.AuthorizePASSAdmission(
			context.Background(), fixture.plan, passAdmissionPermitRequest(t, fixture.plan),
		)
		if err == nil || !strings.Contains(err.Error(), "exceeds the source ceiling") ||
			permit.Facts() != (PASSAdmissionPermitFacts{}) || len(ledger.sources) != 1 {
			t.Fatalf("reduced PASS ceiling = permit %#v, sources %d, error %v",
				permit.Facts(), len(ledger.sources), err)
		}
	})

	t.Run("non-PASS is not an admission authority request", func(t *testing.T) {
		fixture := newApprovalFixture(t)
		resolver := fixture.resolver()
		service, err := newAuthorityWithClock(
			[]TrustRoot{fixture.root}, resolver, &recordingLedger{}, func() time.Time { return fixture.now },
		)
		if err != nil {
			t.Fatal(err)
		}
		request := passAdmissionPermitRequest(t, fixture.plan)
		request.Outcome = "FAIL"
		permit, err := service.AuthorizePASSAdmission(context.Background(), fixture.plan, request)
		if err == nil || !strings.Contains(err.Error(), "requires outcome PASS") ||
			permit.Facts() != (PASSAdmissionPermitFacts{}) || resolver.calls != 0 {
			t.Fatalf("non-PASS request = permit %#v, calls %d, error %v", permit.Facts(), resolver.calls, err)
		}
	})
}

func TestCurrentPASSAdmissionPermitRejectsEveryChangedBinding(t *testing.T) {
	fixture := newApprovalFixture(t)
	now := fixture.now
	ledger := &recordingLedger{}
	service, err := newAuthorityWithClock(
		[]TrustRoot{fixture.root}, fixture.resolver(), ledger, func() time.Time { return now },
	)
	if err != nil {
		t.Fatal(err)
	}
	request := passAdmissionPermitRequest(t, fixture.plan)
	permit, err := service.AuthorizePASSAdmission(context.Background(), fixture.plan, request)
	if err != nil {
		t.Fatal(err)
	}
	otherPlan := planWithCreatedAt(t, fixture.plan, "2026-07-19T00:00:01Z")
	otherContract, exists := otherPlan.Work(request.WorkID)
	if !exists || otherContract.Digest() != request.Contract.Digest() || otherContract == request.Contract {
		t.Fatal("cross-plan contract fixture did not preserve only the work digest")
	}
	tests := []struct {
		name   string
		mutate func(*PASSAdmissionPermitRequest)
	}{
		{name: "controller", mutate: func(value *PASSAdmissionPermitRequest) { value.ControllerID = "controller-2" }},
		{name: "run", mutate: func(value *PASSAdmissionPermitRequest) { value.RunID = "verifier-run-2" }},
		{name: "revision", mutate: func(value *PASSAdmissionPermitRequest) { value.StateRevision++ }},
		{name: "work", mutate: func(value *PASSAdmissionPermitRequest) { value.WorkID = "work-2" }},
		{name: "attempt", mutate: func(value *PASSAdmissionPermitRequest) { value.WorkAttempt++ }},
		{name: "work contract", mutate: func(value *PASSAdmissionPermitRequest) { value.Contract = otherContract }},
		{name: "submission", mutate: func(value *PASSAdmissionPermitRequest) { value.SubmissionID = "submission-2" }},
		{name: "submission digest", mutate: func(value *PASSAdmissionPermitRequest) { value.SubmissionDigest = fixedDigest("d") }},
		{name: "effect", mutate: func(value *PASSAdmissionPermitRequest) { value.VerifierEffectID = "verifier-effect-2" }},
		{name: "dispatch", mutate: func(value *PASSAdmissionPermitRequest) { value.DispatchID = "verifier-dispatch-2" }},
		{name: "dispatch digest", mutate: func(value *PASSAdmissionPermitRequest) { value.DispatchDigest = fixedDigest("e") }},
		{name: "assessment", mutate: func(value *PASSAdmissionPermitRequest) { value.AssessmentDigest = fixedDigest("f") }},
		{name: "outcome", mutate: func(value *PASSAdmissionPermitRequest) { value.Outcome = "FAIL" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := request
			test.mutate(&changed)
			if err := service.ValidatePASSAdmissionPermit(permit, changed); err == nil {
				t.Fatal("changed PASS admission binding was accepted")
			}
		})
	}

	t.Run("purpose", func(t *testing.T) {
		wrongPurpose := permit
		wrongPurpose.binding.facts.Purpose = VerifierExecutionPurpose
		if err := service.ValidatePASSAdmissionPermit(wrongPurpose, request); err == nil ||
			!strings.Contains(err.Error(), "wrong purpose") {
			t.Fatalf("wrong-purpose PASS permit error = %v", err)
		}
	})

	t.Run("foreign authority", func(t *testing.T) {
		foreign, err := newAuthorityWithClock(
			[]TrustRoot{fixture.root}, fixture.resolver(), ledger, func() time.Time { return now },
		)
		if err != nil {
			t.Fatal(err)
		}
		if err := foreign.ValidatePASSAdmissionPermit(permit, request); err == nil ||
			!strings.Contains(err.Error(), "another authority") {
			t.Fatalf("foreign PASS permit error = %v", err)
		}
	})

	t.Run("stale", func(t *testing.T) {
		now = fixture.now.Add(currentEffectPermitLifetime)
		if err := service.ValidatePASSAdmissionPermit(permit, request); err == nil ||
			!strings.Contains(err.Error(), "stale") {
			t.Fatalf("stale PASS permit error = %v", err)
		}
		now = fixture.now
	})
}

func TestVerifierExecutionAndPASSAdmissionResolveAuthoritySeparately(t *testing.T) {
	fixture := newApprovalFixture(t)
	resolver := fixture.resolver()
	ledger := &recordingLedger{}
	service, err := newAuthorityWithClock(
		[]TrustRoot{fixture.root}, resolver, ledger, func() time.Time { return fixture.now },
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AuthorizeVerifierExecution(
		context.Background(), fixture.plan, verifierExecutionPermitRequest(t, fixture.plan),
	); err != nil {
		t.Fatal(err)
	}

	fixture.source.Version++
	fixture.source.Status = "revoked"
	fixture.rebindSource(t)
	resolver.source = slices.Clone(fixture.sourceRaw)
	resolver.proof = slices.Clone(fixture.proofRaw)
	permit, err := service.AuthorizePASSAdmission(
		context.Background(), fixture.plan, passAdmissionPermitRequest(t, fixture.plan),
	)
	if err == nil || !strings.Contains(err.Error(), "revoked") ||
		permit.Facts() != (PASSAdmissionPermitFacts{}) || resolver.calls != 2 || len(ledger.sources) != 2 {
		t.Fatalf("fresh PASS gate = permit %#v, calls %d, sources %d, error %v",
			permit.Facts(), resolver.calls, len(ledger.sources), err)
	}
}

func TestVerifierAuthorityGatesRequireDurableCurrentHead(t *testing.T) {
	fixture := newApprovalFixture(t)
	ledger := &recordingLedger{currentErr: context.DeadlineExceeded}
	service, err := newAuthorityWithClock(
		[]TrustRoot{fixture.root}, fixture.resolver(), ledger, func() time.Time { return fixture.now },
	)
	if err != nil {
		t.Fatal(err)
	}
	verifier, verifierErr := service.AuthorizeVerifierExecution(
		context.Background(), fixture.plan, verifierExecutionPermitRequest(t, fixture.plan),
	)
	if verifierErr == nil || !strings.Contains(verifierErr.Error(), "persist current authority source") ||
		verifier.Facts() != (VerifierExecutionPermitFacts{}) {
		t.Fatalf("unpersisted verifier authority = permit %#v, error %v", verifier.Facts(), verifierErr)
	}
	pass, passErr := service.AuthorizePASSAdmission(
		context.Background(), fixture.plan, passAdmissionPermitRequest(t, fixture.plan),
	)
	if passErr == nil || !strings.Contains(passErr.Error(), "persist current authority source") ||
		pass.Facts() != (PASSAdmissionPermitFacts{}) || len(ledger.sources) != 2 {
		t.Fatalf("unpersisted PASS authority = permit %#v, sources %d, error %v",
			pass.Facts(), len(ledger.sources), passErr)
	}
}

func TestVerifierAuthorityPermitsExpireWithTheirCurrentSource(t *testing.T) {
	fixture := newApprovalFixture(t)
	fixture.source.ValidUntil = "2026-07-19T00:01:20Z"
	fixture.rebindSource(t)
	now := fixture.now
	service, err := newAuthorityWithClock(
		[]TrustRoot{fixture.root}, fixture.resolver(), &recordingLedger{}, func() time.Time { return now },
	)
	if err != nil {
		t.Fatal(err)
	}
	verifierRequest := verifierExecutionPermitRequest(t, fixture.plan)
	verifierPermit, err := service.AuthorizeVerifierExecution(
		context.Background(), fixture.plan, verifierRequest,
	)
	if err != nil {
		t.Fatal(err)
	}
	passRequest := passAdmissionPermitRequest(t, fixture.plan)
	passPermit, err := service.AuthorizePASSAdmission(context.Background(), fixture.plan, passRequest)
	if err != nil {
		t.Fatal(err)
	}
	now = mustTime(t, fixture.source.ValidUntil)
	if err := service.ValidateVerifierExecutionPermit(verifierPermit, verifierRequest); err == nil ||
		!strings.Contains(err.Error(), "no longer current") {
		t.Fatalf("expired verifier source error = %v", err)
	}
	if err := service.ValidatePASSAdmissionPermit(passPermit, passRequest); err == nil ||
		!strings.Contains(err.Error(), "no longer current") {
		t.Fatalf("expired PASS source error = %v", err)
	}
}

func verifierExecutionPermitRequest(t *testing.T, plan protocol.ExactPlan) VerifierExecutionPermitRequest {
	t.Helper()
	workIDs := plan.WorkIDs()
	if len(workIDs) == 0 {
		t.Fatal("verifier permit fixture plan has no work")
	}
	contract, exists := plan.Work(workIDs[0])
	if !exists {
		t.Fatalf("verifier permit fixture work %q is absent", workIDs[0])
	}
	return VerifierExecutionPermitRequest{
		ControllerID: "controller-1", RunID: "verifier-run-1", StateRevision: 7,
		WorkID: workIDs[0], WorkAttempt: 1, Contract: contract,
		SubmissionID: "submission-1", SubmissionDigest: fixedDigest("7"),
		VerifierEffectID: "verifier-effect-1", DispatchID: "verifier-dispatch-1",
		DispatchDigest: fixedDigest("8"), VerifierProfileDigest: fixedDigest("9"),
	}
}

func passAdmissionPermitRequest(t *testing.T, plan protocol.ExactPlan) PASSAdmissionPermitRequest {
	t.Helper()
	execution := verifierExecutionPermitRequest(t, plan)
	return PASSAdmissionPermitRequest{
		ControllerID: execution.ControllerID, RunID: execution.RunID,
		StateRevision: execution.StateRevision, WorkID: execution.WorkID,
		WorkAttempt: execution.WorkAttempt, Contract: execution.Contract,
		SubmissionID: execution.SubmissionID, SubmissionDigest: execution.SubmissionDigest,
		VerifierEffectID: execution.VerifierEffectID, DispatchID: execution.DispatchID,
		DispatchDigest: execution.DispatchDigest, AssessmentDigest: fixedDigest("a"), Outcome: "PASS",
	}
}
