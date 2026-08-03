package runtime

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/journal"
)

func runtimePlan(t *testing.T, release, repository, target, marker string) ([]byte, baton.Plan) {
	t.Helper()
	slice := func(id, path string) baton.Slice {
		return baton.Slice{
			ID: id, Outcome: "Deliver " + id + ".",
			Scope:      baton.Scope{Include: []string{path}, Exclude: []string{}},
			Acceptance: []baton.Criterion{{ID: "A-" + id, Text: id + " is exact."}},
			Checks:     []string{"check " + id}, Constraints: []string{"deterministic"},
			DependsOn: []string{}, Consumes: []string{},
		}
	}
	metadata := baton.Metadata{
		SchemaVersion: baton.PlanVersion,
		Release:       release,
		Revision:      1,
		PreviousPlan:  nil,
		Repository:    repository,
		TargetRef:     target,
		ApprovalRef:   "operator://" + release + "/1",
		Tracks: []baton.Track{
			{ID: "T1", DependsOn: []string{}, Slices: []baton.Slice{slice("S1", "one.txt")}},
			{ID: "T2", DependsOn: []string{}, Slices: []baton.Slice{slice("S2", "two.txt")}},
		},
	}
	metadataBody, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(
		"```baton-plan-v2\n" + string(metadataBody) +
			"\n```\n\nFixture plan.\n",
	)
	plan, err := baton.ParsePlan(body)
	if err != nil {
		t.Fatal(err)
	}
	return body, plan
}

func encodeSubmission(t *testing.T, submission driver.Submission) string {
	t.Helper()
	body, err := driver.EncodeSubmission(submission)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(body)
}

func fixtureManifest(t *testing.T) (Manifest, []byte, baton.Plan) {
	t.Helper()
	const (
		runID      = "run-1"
		release    = "release-1"
		repository = "acme-repo"
		target     = "refs/heads/main"
		marker     = "approval-release-1-v1"
	)
	planBytes, plan := runtimePlan(t, release, repository, target, marker)
	submission := func(
		slice string,
		responsibility driver.Responsibility,
		batonAttempt int64,
	) driver.Submission {
		script := ScriptedAttempt{Slice: slice, Responsibility: responsibility,
			BatonAttempt: batonAttempt, Epoch: 1, Try: 1}
		return driver.Submission{
			SchemaVersion:  driver.SubmissionSchemaVersion,
			InvocationID:   invocationID(runID, script),
			Responsibility: responsibility,
			Summary:        "Exact " + string(responsibility) + ".",
			Detail:         "Bounded fixture detail.",
		}
	}
	planner := submission("", driver.PlannerProposal, 1)
	planner.Plan, _ = driver.NewPlanBytes(planBytes)
	design := submission("S1", driver.ImplementerDesign, 1)
	captain := submission("S1", driver.CaptainReview, 1)
	captain.Decision, _ = driver.NewDecision(driver.DecisionProceed)
	implementation := submission("S1", driver.ImplementerImplementation, 1)
	implementation.Checks, _ = driver.NewCheckBytes([]byte("implementation checks\n"))
	work := submission("S1", driver.WorkVerification, 1)
	work.Checks, _ = driver.NewCheckBytes([]byte("work checks\n"))
	work.Decision, _ = driver.NewDecision(driver.DecisionPass)
	assembly := submission("", driver.AssemblyVerification, 1)
	assembly.Checks, _ = driver.NewCheckBytes([]byte("assembly checks\n"))
	assembly.Decision, _ = driver.NewDecision(driver.DecisionPass)
	manifest := Manifest{
		SchemaVersion: ManifestVersion,
		RunID:         runID, Repository: "/repository", Release: release,
		TargetRef: target, Intent: "Deliver the exact fixture.",
		MaxParallelTracks: 2,
		Authority: ProjectAuthority{
			Project: repository, ExternalAuthorizer: "operator",
			BootstrapApprovedPlanDigest: func() *string {
				digest := plan.Digest()
				return &digest
			}(),
		},
		Driver: &FakeDriverConfig{
			Executable: "/bin/true",
			Digest:     "sha256:" + strings.Repeat("a", 64),
			AdapterKey: "fixture", Profile: "fixture",
		},
		Roles: driver.RoleSelections{
			Planner:     driver.RoleSelection{Profile: "fixture", Model: "planner-model"},
			Implementer: driver.RoleSelection{Profile: "fixture", Model: "implementer-model"},
			Captain:     driver.RoleSelection{Profile: "fixture", Model: "captain-model"},
			Verifier:    driver.RoleSelection{Profile: "fixture", Model: "verifier-model"},
		},
		Automation: &AutomationSelections{
			Recovery: driver.RoleSelection{
				Profile: "fixture",
				Model:   "recovery-model",
			},
		},
		Limits: driver.Limits{TimeoutMillis: 30_000, OutputBytes: 65_536},
		Scripts: []ScriptedAttempt{
			{Responsibility: driver.AssemblyVerification, BatonAttempt: 1, Epoch: 1, Try: 1,
				Behavior: "submit", Submission: encodeSubmission(t, assembly)},
			{Slice: "S1", Responsibility: driver.CaptainReview, BatonAttempt: 1, Epoch: 1, Try: 1,
				Behavior: "submit", Submission: encodeSubmission(t, captain)},
			{Slice: "S1", Responsibility: driver.ImplementerDesign, BatonAttempt: 1, Epoch: 1, Try: 1,
				Behavior: "submit", Submission: encodeSubmission(t, design)},
			{Slice: "S1", Responsibility: driver.ImplementerImplementation, BatonAttempt: 1, Epoch: 1, Try: 1,
				Behavior: "submit", Submission: encodeSubmission(t, implementation)},
			{Responsibility: driver.PlannerProposal, BatonAttempt: 1, Epoch: 1, Try: 1,
				Behavior: "submit", Submission: encodeSubmission(t, planner)},
			{Slice: "S1", Responsibility: driver.WorkVerification, BatonAttempt: 1, Epoch: 1, Try: 1,
				Behavior: "submit", Submission: encodeSubmission(t, work)},
		},
	}
	body, err := canonicalManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	return manifest, body, plan
}

func TestManifestIsClosedCanonicalAndBindsEverySubmission(t *testing.T) {
	t.Parallel()

	manifest, body, _ := fixtureManifest(t)
	admitted, err := admitManifest(body)
	if err != nil {
		t.Fatal(err)
	}
	if admitted.value.RunID != manifest.RunID ||
		admitted.digest != sha256Digest(body) {
		t.Fatalf("admission = %#v", admitted)
	}
	unknown := append([]byte(nil), body...)
	unknown = []byte(strings.Replace(
		string(unknown),
		`"schema_version":"sworn.runtime-manifest/v4"`,
		`"schema_version":"sworn.runtime-manifest/v4","unknown":true`,
		1,
	))
	if _, err := admitManifest(unknown); !IsCode(err, "INVALID_MANIFEST") {
		t.Fatalf("unknown manifest field = %v", err)
	}
	duplicate := []byte(strings.Replace(
		string(body),
		`"run_id":"run-1"`,
		`"run_id":"run-1","run_id":"run-1"`,
		1,
	))
	if _, err := admitManifest(duplicate); !IsCode(err, "INVALID_MANIFEST") {
		t.Fatalf("duplicate manifest field = %v", err)
	}
	pretty := append([]byte("{\n"), body[1:]...)
	if _, err := admitManifest(pretty); !IsCode(err, "NONCANONICAL_MANIFEST") {
		t.Fatalf("noncanonical manifest = %v", err)
	}
	mutated := manifest
	mutated.Scripts[1].Submission = mutated.Scripts[5].Submission
	mutatedBody, err := json.Marshal(mutated)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admitManifest(append(mutatedBody, '\n')); !IsCode(err, "INVALID_SCRIPTED_SUBMISSION") {
		t.Fatalf("responsibility substitution = %v", err)
	}
	withoutAutomation := manifest
	withoutAutomation.Automation = nil
	withoutAutomationBody, err := json.Marshal(withoutAutomation)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admitManifest(
		append(withoutAutomationBody, '\n'),
	); !IsCode(err, "INVALID_AUTOMATION") {
		t.Fatalf("missing v3 automation = %v", err)
	}
	legacyWithAutomation := manifest
	legacyWithAutomation.SchemaVersion = ManifestVersionV2
	legacyWithAutomationBody, err := json.Marshal(legacyWithAutomation)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admitManifest(
		append(legacyWithAutomationBody, '\n'),
	); !IsCode(err, "MIGRATION_REQUIRED") {
		t.Fatalf("v2 migration = %v", err)
	}
}

func TestProductionManifestIsClosedCanonicalAndExclusiveWithFakeMode(t *testing.T) {
	t.Parallel()

	fake, fakeBody, _ := fixtureManifest(t)
	if !bytes.Contains(fakeBody, []byte(`"driver":{`)) ||
		!bytes.Contains(fakeBody, []byte(`"scripted_attempts":[`)) ||
		bytes.Contains(fakeBody, []byte(`"driver_config_digest"`)) {
		t.Fatalf("fake v3 shape changed: %s", fakeBody)
	}

	legacy := fake
	legacy.SchemaVersion = ManifestVersionV2
	legacy.Automation = nil
	legacyBody, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admitManifest(append(legacyBody, '\n')); !IsCode(err, "MIGRATION_REQUIRED") {
		t.Fatalf("legacy v2 admission = %v", err)
	}

	production := fake
	production.Driver = nil
	production.DriverConfigDigest = "sha256:" + strings.Repeat("b", 64)
	production.Scripts = nil
	production.Roles = driver.RoleSelections{
		Planner: driver.RoleSelection{
			Profile: "planner-profile", Model: "planner-model",
		},
		Implementer: driver.RoleSelection{
			Profile: "implementer-profile", Model: "implementer-model",
		},
		Captain: driver.RoleSelection{
			Profile: "captain-profile", Model: "captain-model",
		},
		Verifier: driver.RoleSelection{
			Profile: "verifier-profile", Model: "verifier-model",
		},
	}
	production.Automation = &AutomationSelections{
		Recovery: driver.RoleSelection{
			Profile: "recovery-profile",
			Model:   "recovery-model",
		},
	}
	body, err := canonicalManifest(production)
	if err != nil {
		t.Fatal(err)
	}
	admitted, err := admitManifest(body)
	if err != nil {
		t.Fatal(err)
	}
	if !admitted.value.production() ||
		admitted.value.DriverConfigDigest != production.DriverConfigDigest ||
		bytes.Contains(body, []byte(`"driver":`)) ||
		bytes.Contains(body, []byte(`"scripted_attempts":`)) {
		t.Fatalf("production admission = %#v\n%s", admitted.value, body)
	}

	for name, mutate := range map[string]func(*Manifest){
		"neither driver source": func(value *Manifest) {
			value.DriverConfigDigest = ""
		},
		"fake and production source": func(value *Manifest) {
			value.Driver = fake.Driver
		},
		"production scripts": func(value *Manifest) {
			value.Scripts = []ScriptedAttempt{{
				Responsibility: driver.PlannerProposal,
				BatonAttempt:   1,
				Epoch:          1,
				Try:            1,
				Behavior:       "none",
			}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			value := production
			mutate(&value)
			raw, marshalErr := json.Marshal(value)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			if _, admissionErr := admitManifest(append(raw, '\n')); !IsCode(admissionErr, "INVALID_MANIFEST_VARIANT") {
				t.Fatalf("admission = %v", admissionErr)
			}
		})
	}

	invalidDigest := production
	invalidDigest.DriverConfigDigest = "sha256:not-a-digest"
	raw, err := json.Marshal(invalidDigest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admitManifest(append(raw, '\n')); !IsCode(err, "INVALID_DRIVER_CONFIG_DIGEST") {
		t.Fatalf("invalid config digest = %v", err)
	}
}

func TestRecoveryCommandBindingRejectsKindKeyAndPayloadSubstitution(t *testing.T) {
	payload := []byte("{\"value\":1}\n")
	command := journal.Command{
		RunID: "run-1", ReplayKey: "effect-1",
		Kind: "baton.install", Payload: payload,
	}
	effect := journal.Effect{
		RunID: "run-1", ID: "effect-1", ReplayKey: "effect-1",
		Kind: "baton.install", ExpectedDigest: sha256Digest(payload),
	}
	if err := validateRecoveryCommand(command, effect, true); err != nil {
		t.Fatalf("exact binding rejected: %v", err)
	}
	for name, mutate := range map[string]func(*journal.Command, *journal.Effect){
		"run": func(command *journal.Command, _ *journal.Effect) {
			command.RunID = "run-2"
		},
		"replay_key": func(command *journal.Command, _ *journal.Effect) {
			command.ReplayKey = "effect-2"
		},
		"kind": func(command *journal.Command, _ *journal.Effect) {
			command.Kind = "baton.merge"
		},
		"payload": func(command *journal.Command, _ *journal.Effect) {
			command.Payload = []byte("{\"value\":2}\n")
		},
	} {
		t.Run(name, func(t *testing.T) {
			mutatedCommand, mutatedEffect := command, effect
			mutate(&mutatedCommand, &mutatedEffect)
			if err := validateRecoveryCommand(
				mutatedCommand, mutatedEffect, true,
			); !IsCode(err, "CORRUPT_JOURNAL") {
				t.Fatalf("substitution error = %v, want CORRUPT_JOURNAL", err)
			}
		})
	}
}

func TestProposalAuthorityRequiresFreshSameRevisionAfterRefDrift(t *testing.T) {
	_, _, initialPlan := fixtureManifest(t)
	targetOld, _ := gitx.ParseOID(gitx.SHA1, strings.Repeat("1", 40))
	targetNew, _ := gitx.ParseOID(gitx.SHA1, strings.Repeat("2", 40))
	releaseOld, _ := gitx.ParseOID(gitx.SHA1, strings.Repeat("3", 40))
	releaseNew, _ := gitx.ParseOID(gitx.SHA1, strings.Repeat("4", 40))
	releaseRef := "refs/heads/release-wt/release-1"
	targetRef := "refs/heads/main"

	initial := admittedPlanProposal{
		plan: initialPlan,
		authority: planProposalAuthority{
			Release: "release-1", ReleaseRef: releaseRef,
			TargetRef: targetRef, TargetHead: targetOld.String(),
		},
	}
	missing := &baton.RecordError{Code: "REF_NOT_FOUND"}
	if proposalMatchesPendingAuthority(
		initial,
		gitx.RefHead{Ref: releaseRef, State: gitx.RefAbsent},
		gitx.RefHead{Ref: targetRef, State: gitx.RefDirect, Head: targetNew},
		baton.State{},
		missing,
	) {
		t.Fatal("initial proposal survived target drift")
	}
	freshInitial := initial
	freshInitial.authority.TargetHead = targetNew.String()
	if !proposalMatchesPendingAuthority(
		freshInitial,
		gitx.RefHead{Ref: releaseRef, State: gitx.RefAbsent},
		gitx.RefHead{Ref: targetRef, State: gitx.RefDirect, Head: targetNew},
		baton.State{},
		missing,
	) {
		t.Fatal("fresh initial proposal for the same revision was rejected")
	}

	priorPlan := strings.Repeat("a", 40)
	metadata := initialPlan.Metadata()
	metadata.Revision = 2
	metadata.PreviousPlan = &priorPlan
	metadata.ApprovalRef = "operator://release-1/2"
	metadataBody, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	revisionPlan, err := baton.ParsePlan([]byte(
		"```baton-plan-v2\n" + string(metadataBody) +
			"\n```\n\nFixture revision.\n",
	))
	if err != nil {
		t.Fatal(err)
	}
	state := baton.State{
		Plan: baton.PlanState{
			OID: priorPlan,
			Metadata: baton.Metadata{
				Revision: 1,
			},
		},
		Refs: baton.StateRefs{
			Release: baton.CapturedRef{
				Ref: releaseRef, Head: releaseNew.String(),
			},
			Target: baton.CapturedRef{
				Ref: targetRef, Head: targetNew.String(),
			},
		},
	}
	revision := admittedPlanProposal{
		plan: revisionPlan,
		authority: planProposalAuthority{
			Release: "release-1", PriorPlan: priorPlan,
			ReleaseRef: releaseRef, ReleaseHead: releaseOld.String(),
			TargetRef: targetRef, TargetHead: targetOld.String(),
		},
	}
	if proposalMatchesPendingAuthority(
		revision,
		gitx.RefHead{
			Ref: releaseRef, State: gitx.RefDirect, Head: releaseNew,
		},
		gitx.RefHead{
			Ref: targetRef, State: gitx.RefDirect, Head: targetNew,
		},
		state,
		nil,
	) {
		t.Fatal("revision proposal survived release/target drift")
	}
	freshRevision := revision
	freshRevision.authority.ReleaseHead = releaseNew.String()
	freshRevision.authority.TargetHead = targetNew.String()
	if !proposalMatchesPendingAuthority(
		freshRevision,
		gitx.RefHead{
			Ref: releaseRef, State: gitx.RefDirect, Head: releaseNew,
		},
		gitx.RefHead{
			Ref: targetRef, State: gitx.RefDirect, Head: targetNew,
		},
		state,
		nil,
	) {
		t.Fatal("fresh proposal for the same revision was rejected")
	}
}

func TestTargetStaleRejectsEveryModelDispatchAuthority(t *testing.T) {
	state := baton.State{
		Plan: baton.PlanState{TargetStale: true},
	}
	for _, test := range []struct {
		responsibility driver.Responsibility
		slice          string
	}{
		{driver.ImplementerDesign, "S1"},
		{driver.CaptainReview, "S1"},
		{driver.ImplementerImplementation, "S1"},
		{driver.WorkVerification, "S1"},
		{driver.AssemblyVerification, ""},
		{driver.PlannerProposal, ""},
	} {
		t.Run(string(test.responsibility), func(t *testing.T) {
			if dispatchAuthorityCurrent(
				state,
				test.slice,
				test.responsibility,
				"sha256:"+strings.Repeat("0", 64),
			) {
				t.Fatal("target-stale authority admitted a model dispatch")
			}
		})
	}
}

func TestAllNewBatonActionResultsReconstructFromDurableProjection(t *testing.T) {
	const release = "release-1"
	candidate := strings.Repeat("c", 40)
	targetHead := strings.Repeat("d", 40)
	planOID := strings.Repeat("e", 40)
	binds := strings.Repeat("f", 40)
	detail := []byte("exact detail")
	summary := "Exact durable action."
	target := targetHead
	candidateValue := candidate
	receipt := func(role, result string) baton.Receipt {
		return baton.Receipt{
			Version: baton.ReceiptVersion,
			Release: release,
			Role:    role, Result: result,
			Plan: planOID, Binds: binds,
			Summary: summary, Target: &target,
			Candidate: &candidateValue,
		}
	}
	entry := func(oid string, value baton.Receipt) *baton.ReceiptEntry {
		return &baton.ReceiptEntry{
			OID: oid, Detail: append([]byte(nil), detail...), Receipt: value,
		}
	}
	planBytes, installedPlan := runtimePlan(
		t, release, "acme-repo", "refs/heads/main",
		"approval-release-1-v1",
	)
	planMetadata := installedPlan.Metadata()
	installAdmission := approvalAdmission{
		planBytes: planBytes, planDigest: installedPlan.Digest(),
		reference: planMetadata.ApprovalRef,
	}
	installReceipt := receipt("planner", "approved")
	installReceipt.Plan = planOID
	installReceipt.Summary = "Install the exact locally authorized plan."
	installReceipt.Candidate = nil
	installApproval := baton.ReceiptEntry{
		OID:     "approval-receipt",
		Detail:  installDetail(installAdmission),
		Receipt: installReceipt,
	}
	retiredSlice := "S-retired"
	retirementReceipt := baton.Receipt{
		Version: baton.ReceiptVersion,
		Release: release,
		Slice:   &retiredSlice,
		Role:    "planner", Result: "retired",
		Plan: planOID, Binds: installApproval.OID,
		Summary: "Retired exact historical slice.",
	}
	previousPlan := planOID
	nextMetadata := planMetadata
	nextMetadata.Revision = 2
	nextMetadata.PreviousPlan = &previousPlan
	nextMetadata.ApprovalRef = "operator://release-1/2"
	nextMetadataBytes, err := json.MarshalIndent(nextMetadata, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	nextPlan, err := baton.ParsePlan([]byte(
		"```baton-plan-v2\n" + string(nextMetadataBytes) +
			"\n```\n\nLater fixture plan.\n",
	))
	if err != nil {
		t.Fatal(err)
	}
	nextPlanOID := strings.Repeat("2", 40)
	nextApproval := baton.ReceiptEntry{
		OID:    "later-approval-receipt",
		Detail: []byte("later authority detail"),
		Receipt: baton.Receipt{
			Version: baton.ReceiptVersion,
			Release: release,
			Role:    "planner", Result: "approved",
			Plan:    nextPlanOID,
			Summary: "Install the exact locally authorized plan.",
			Target:  &target,
		},
	}
	installState := baton.State{
		Release: release,
		Plan: baton.PlanState{
			OID: nextPlanOID, Digest: nextPlan.Digest(),
			Metadata: nextPlan.Metadata(),
			Approval: nextApproval,
			History: []baton.PlanHistory{
				{
					OID: planOID, Revision: 1,
					Approval: installApproval, Plan: installedPlan,
					InstallHead: "retirement-receipt",
					Retirements: []baton.RetirementResult{{
						Slice:         retiredSlice,
						ReceiptCommit: "retirement-receipt",
						Receipt:       retirementReceipt,
					}},
				},
				{
					OID: nextPlanOID, Revision: 2,
					Approval: nextApproval, Plan: nextPlan,
					InstallHead: "later-approval-receipt",
				},
			},
		},
		Refs: baton.StateRefs{
			Release: baton.CapturedRef{
				Ref:  "refs/heads/release-wt/" + release,
				Head: "later-release-head",
			},
			Target: baton.CapturedRef{
				Ref: "refs/heads/main", Head: "later-target-head",
			},
		},
	}
	sliceID := "S1"
	sliceReceipt := receipt("captain", "proceed")
	sliceReceipt.Slice = &sliceID
	sliceEntry := entry("slice-receipt", sliceReceipt)
	sliceState := &baton.SliceState{
		Location: baton.SliceLocation{
			Track: baton.Track{ID: "T1"},
			Slice: baton.Slice{ID: sliceID},
		},
		History: baton.SliceHistory{
			Entries: []baton.ReceiptEntry{sliceEntry.Clone()},
		},
		CurrentReceipt: entry(
			"later-slice-receipt",
			receipt("implementer", "candidate"),
		),
	}
	appendState := baton.State{
		Release: release,
		Slices:  []*baton.SliceState{sliceState},
		Tracks: []baton.TrackState{{
			ID: "T1", Ref: "refs/heads/track/release-1/T1",
		}},
	}
	assemblyReceipt := receipt("verifier", "pass")
	assemblyEntry := entry("assembly-verdict", assemblyReceipt)
	assemblyState := baton.State{
		Release: release,
		Refs: baton.StateRefs{
			Release: baton.CapturedRef{
				Ref: "refs/heads/release-wt/" + release,
			},
			Target: baton.CapturedRef{Ref: "refs/heads/main"},
		},
		Assembly: baton.AssemblyState{
			History: []baton.ReceiptEntry{assemblyEntry.Clone()},
			CurrentReceipt: entry(
				"later-assembly-verdict",
				receipt("verifier", "fail"),
			),
		},
	}
	preparedReceipt := receipt("implementer", "candidate")
	preparedReceipt.Inputs = map[string]string{"T1": candidate}
	preparedEntry := entry("assembly-candidate", preparedReceipt)
	laterPreparedReceipt := receipt("implementer", "candidate")
	laterPreparedReceipt.Summary = "Later assembly candidate."
	preparedState := baton.State{
		Release: release,
		Assembly: baton.AssemblyState{
			History: []baton.ReceiptEntry{preparedEntry.Clone()},
			Candidate: entry(
				"later-assembly-candidate",
				laterPreparedReceipt,
			),
		},
	}
	mergedReceipt := receipt("merge", "merged")
	mergedResult := strings.Repeat("9", 40)
	mergedReceipt.ResultCommit = &mergedResult
	mergedEntry := entry("merge-receipt", mergedReceipt)
	laterMergedReceipt := receipt("merge", "merged")
	laterMergedReceipt.Summary = "Later merge."
	mergedState := baton.State{
		Release: release,
		Refs: baton.StateRefs{
			Target: baton.CapturedRef{Ref: "refs/heads/main"},
		},
		Assembly: baton.AssemblyState{
			History: []baton.ReceiptEntry{mergedEntry.Clone()},
			CurrentReceipt: entry(
				"later-merge-receipt",
				laterMergedReceipt,
			),
			ResultCommit: strings.Repeat("8", 40),
		},
	}
	tests := []struct {
		name       string
		state      baton.State
		kind       string
		command    batonActionCommand
		wantAction string
		wantCommit string
		wantHead   string
		wantTarget string
		wantResult string
	}{
		{
			name: "install", state: installState, kind: "baton.install",
			command: batonActionCommand{
				Authority: batonActionAuthority{
					Release: release, TargetHead: targetHead,
				},
				Input: mustJSON(installActionInput{
					PlanBytes:  planBytes,
					PlanDigest: installedPlan.Digest(),
					Reference:  planMetadata.ApprovalRef,
				}),
			},
			wantAction: "recordPlanRevision", wantCommit: "approval-receipt",
			wantHead: "retirement-receipt", wantTarget: targetHead,
		},
		{
			name: "append_receipt", state: appendState,
			kind: "baton.append_receipt",
			command: batonActionCommand{
				Authority: batonActionAuthority{
					Release: release, Plan: planOID, Binds: binds,
				},
				Input: mustJSON(baton.AppendReceiptInput{
					Release: release, Slice: sliceID,
					Role: "captain", Result: "proceed",
					Summary: summary, Detail: detail,
				}),
			},
			wantAction: "appendReceipt", wantCommit: "slice-receipt",
		},
		{
			name: "assembly_verdict", state: assemblyState,
			kind: "baton.assembly_verdict",
			command: batonActionCommand{
				Authority: batonActionAuthority{
					Release: release, Plan: planOID, Binds: binds,
				},
				Input: mustJSON(baton.AppendReceiptInput{
					Release: release, Role: "verifier", Result: "pass",
					Summary: summary, Detail: detail,
				}),
			},
			wantAction: "appendReceipt", wantCommit: "assembly-verdict",
		},
		{
			name: "prepare_assembly", state: preparedState,
			kind: "baton.prepare_assembly",
			command: batonActionCommand{
				Authority: batonActionAuthority{
					Release: release, Plan: planOID, Binds: binds,
					TargetHead: targetHead,
				},
				Input: mustJSON(baton.PrepareAssemblyInput{
					Release: release, Summary: summary, Detail: detail,
				}),
			},
			wantAction: "prepareAssembly", wantCommit: "assembly-candidate",
		},
		{
			name: "merge", state: mergedState, kind: "baton.merge",
			command: batonActionCommand{
				Authority: batonActionAuthority{
					Release: release, Plan: planOID, Binds: binds,
					Candidate: candidate, TargetHead: targetHead,
				},
				Input: mustJSON(baton.MergePassedCandidateInput{
					Release: release, Summary: summary, Detail: detail,
				}),
			},
			wantAction: "mergePassedCandidate", wantCommit: "merge-receipt",
			wantTarget: "refs/heads/main", wantResult: mergedResult,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := reconstructAllNewBatonAction(
				test.state, test.kind, test.command)
			if err != nil {
				t.Fatal(err)
			}
			if result.Kind != "baton.action-result/v2" ||
				result.Action != test.wantAction ||
				result.Changed ||
				result.ReceiptCommit != test.wantCommit ||
				result.Receipt == nil {
				t.Fatalf("reconstructed result = %#v", result)
			}
			if test.wantHead != "" && result.Head != test.wantHead {
				t.Fatalf("reconstructed head = %q, want %q",
					result.Head, test.wantHead)
			}
			if test.wantTarget != "" && result.Target != test.wantTarget {
				t.Fatalf("reconstructed target = %q, want %q",
					result.Target, test.wantTarget)
			}
			if test.wantResult != "" &&
				result.ResultCommit != test.wantResult {
				t.Fatalf("reconstructed result commit = %q, want %q",
					result.ResultCommit, test.wantResult)
			}
			if test.name == "install" &&
				(len(result.Retirements) != 1 ||
					result.Retirements[0].Slice != retiredSlice ||
					result.Retirements[0].ReceiptCommit !=
						"retirement-receipt") {
				t.Fatalf("reconstructed retirements = %#v",
					result.Retirements)
			}
		})
	}

	t.Run("retired slice receipt remains reconstructable", func(t *testing.T) {
		retired := appendState
		retired.Slices = nil
		retired.Tracks = nil
		retired.SliceHistories = []baton.SliceHistoryState{{
			Slice: sliceID, Track: "T1",
			Ref: "refs/heads/track/" + release + "/T1",
			History: baton.SliceHistory{
				Entries: []baton.ReceiptEntry{
					sliceEntry.Clone(),
				},
			},
		}}
		command := batonActionCommand{
			Authority: batonActionAuthority{
				Release: release, Plan: planOID, Binds: binds,
			},
			Input: mustJSON(baton.AppendReceiptInput{
				Release: release, Slice: sliceID,
				Role: "captain", Result: "proceed",
				Summary: summary, Detail: detail,
			}),
		}
		result, err := reconstructAllNewBatonAction(
			retired,
			"baton.append_receipt",
			command,
		)
		if err != nil {
			t.Fatal(err)
		}
		if result.ReceiptCommit != sliceEntry.OID ||
			result.Ref != retired.SliceHistories[0].Ref ||
			result.Receipt == nil {
			t.Fatalf("retired reconstruction = %#v", result)
		}
	})

	t.Run("install authority detail substitution is not applied", func(t *testing.T) {
		substituted := installState
		substituted.Plan.History = append(
			[]baton.PlanHistory(nil), installState.Plan.History...)
		substituted.Plan.History[0].Approval =
			substituted.Plan.History[0].Approval.Clone()
		substituted.Plan.History[0].Approval.Detail = []byte("other authority\n")

		command := batonActionCommand{
			Authority: batonActionAuthority{
				Release: release, TargetHead: targetHead,
			},
			Input: mustJSON(installActionInput{
				PlanBytes:  planBytes,
				PlanDigest: installedPlan.Digest(),
				Reference:  planMetadata.ApprovalRef,
			}),
		}
		applied, applyErr := actionAlreadyApplied(
			substituted, "baton.install", command)
		if applyErr != nil {
			t.Fatal(applyErr)
		}
		if applied {
			t.Fatal("different authority detail laundered the install")
		}
	})

	t.Run("external exact plan permits only a fresh idempotent call", func(t *testing.T) {
		externalApproval := installApproval.Clone()
		externalApproval.Detail = []byte("external authority\n")
		releaseRef := "refs/heads/release-wt/" + release
		targetRef := "refs/heads/main"
		external := baton.State{
			Release: release,
			Plan: baton.PlanState{
				OID: planOID, Digest: installedPlan.Digest(),
				Metadata: planMetadata, Approval: externalApproval,
			},
			Refs: baton.StateRefs{
				Release: baton.CapturedRef{
					Ref: releaseRef, Head: "external-release-head",
				},
				Target: baton.CapturedRef{
					Ref: targetRef, Head: targetHead,
				},
			},
		}
		command := batonActionCommand{
			Authority: batonActionAuthority{
				Release:   release,
				TargetRef: targetRef, TargetHead: targetHead,
				OwnerRef: releaseRef,
			},
			Input: mustJSON(installActionInput{
				PlanBytes:  planBytes,
				PlanDigest: installedPlan.Digest(),
				Reference:  planMetadata.ApprovalRef,
			}),
		}
		applied, applyErr := actionAlreadyApplied(
			external, "baton.install", command)
		if applyErr != nil {
			t.Fatal(applyErr)
		}
		if applied {
			t.Fatal("external Baton approval inferred a Sworn effect")
		}
		if !installActionIdempotentlyCallable(external, command) {
			t.Fatal("exact external plan rejected a fresh idempotent call")
		}
		external.Refs.Target.Head = "moved-target-head"
		if installActionIdempotentlyCallable(external, command) {
			t.Fatal("moved target admitted an idempotent install call")
		}
	})
}

func TestActionResultAttestationAllowsOnlyChangedBitVariance(t *testing.T) {
	target := strings.Repeat("1", 40)
	candidate := strings.Repeat("2", 40)
	receiptCommit := strings.Repeat("3", 40)
	retired := "S-retired"
	expected := baton.ActionResult{
		Kind: "baton.action-result/v2", Action: "recordPlanRevision",
		Release: "release-1", Revision: 2,
		Plan: strings.Repeat("4", 40),
		Ref:  "refs/heads/release-wt/release-1",
		Head: strings.Repeat("5", 40), Target: target,
		ReceiptCommit: receiptCommit,
		Receipt: &baton.Receipt{
			Version: baton.ReceiptVersion,
			Release: "release-1", Role: "planner", Result: "approved",
			Plan: strings.Repeat("4", 40), Target: &target,
		},
		Retirements: []baton.RetirementResult{{
			Slice: retired, ReceiptCommit: strings.Repeat("6", 40),
			Receipt: baton.Receipt{
				Version: baton.ReceiptVersion,
				Release: "release-1", Slice: &retired,
				Role: "planner", Result: "retired",
				Candidate: &candidate,
			},
		}},
	}
	actual := expected
	actual.Changed = true
	if !actionResultMatchesDurableTruth(actual, expected) {
		t.Fatal("live changed result did not match reconstructed truth")
	}
	for name, mutate := range map[string]func(*baton.ActionResult){
		"plan": func(value *baton.ActionResult) {
			value.Plan = strings.Repeat("7", 40)
		},
		"head": func(value *baton.ActionResult) {
			value.Head = strings.Repeat("7", 40)
		},
		"receipt": func(value *baton.ActionResult) {
			value.ReceiptCommit = strings.Repeat("7", 40)
		},
		"retirement": func(value *baton.ActionResult) {
			value.Retirements = cloneRuntimeRetirements(
				value.Retirements)
			value.Retirements[0].ReceiptCommit =
				strings.Repeat("7", 40)
		},
	} {
		t.Run(name, func(t *testing.T) {
			substituted := actual
			mutate(&substituted)
			if actionResultMatchesDurableTruth(
				substituted,
				expected,
			) {
				t.Fatal("substituted action result matched durable truth")
			}
		})
	}
}

func TestHistoricalExhaustionOnlyParksCurrentlyApplicableWork(t *testing.T) {
	_, body, plan := fixtureManifest(t)
	manifest, err := admitManifest(body)
	if err != nil {
		t.Fatal(err)
	}
	metadata := plan.Metadata()
	sliceDefinition := metadata.Tracks[0].Slices[0]
	receipt := &baton.ReceiptEntry{OID: "receipt-current"}
	currentSlice := &baton.SliceState{
		Location: baton.SliceLocation{
			Track: metadata.Tracks[0],
			Slice: sliceDefinition,
		},
		Stage: "design", Status: "ready", NextRole: "implementer",
		Attempt: 1, CurrentReceipt: receipt, InputPins: map[string]string{},
	}
	state := baton.State{
		Release: metadata.Release,
		Plan: baton.PlanState{
			OID: "plan-v2", Digest: plan.Digest(), Metadata: metadata,
		},
		Refs: baton.StateRefs{
			Release: baton.CapturedRef{Head: "release-v2"},
			Target:  baton.CapturedRef{Head: "target-v1"},
		},
		Tracks: []baton.TrackState{{
			ID: metadata.Tracks[0].ID, Ref: "refs/heads/track/release-1/T1",
			Head: "track-v1", Slices: []*baton.SliceState{currentSlice},
		}},
		Slices: []*baton.SliceState{currentSlice},
	}
	old := state
	old.Plan.OID = "plan-v1"
	oldBefore := sliceFingerprint(old, sliceDefinition.ID)
	oldWork := driverWorkIdentity(manifest.digest, sliceDefinition.ID,
		driver.ImplementerDesign, 1, oldBefore)
	if exhaustedWorkApplies(manifest, nil, true, state, journal.Snapshot{},
		map[string]struct{}{oldWork: {}}) {
		t.Fatal("changed-plan exhaustion still parks current work")
	}
	currentBefore := sliceFingerprint(state, sliceDefinition.ID)
	currentWork := driverWorkIdentity(manifest.digest, sliceDefinition.ID,
		driver.ImplementerDesign, 1, currentBefore)
	if !exhaustedWorkApplies(manifest, nil, true, state, journal.Snapshot{},
		map[string]struct{}{currentWork: {}}) {
		t.Fatal("exact current exhaustion did not park")
	}
	currentTrackBaseWork := workIdentity(
		trackBaseBefore(state, currentSlice),
		"git.prepare_track_base",
	)
	if !exhaustedWorkApplies(
		manifest,
		nil,
		true,
		state,
		journal.Snapshot{},
		map[string]struct{}{currentTrackBaseWork: {}},
	) {
		t.Fatal("exact current track-base exhaustion did not park")
	}
	state.Slices = nil
	state.Tracks[0].Slices = nil
	if exhaustedWorkApplies(manifest, nil, true, state, journal.Snapshot{},
		map[string]struct{}{oldWork: {}}) {
		t.Fatal("retired-slice exhaustion still parks current work")
	}
}

func TestInvocationIdentityIsStableAcrossResume(t *testing.T) {
	t.Parallel()

	for _, responsibility := range []driver.Responsibility{
		driver.PlannerProposal,
		driver.ImplementerDesign,
		driver.CaptainReview,
		driver.ImplementerImplementation,
		driver.WorkVerification,
		driver.AssemblyVerification,
	} {
		script := ScriptedAttempt{Slice: "S1", Responsibility: responsibility,
			BatonAttempt: 2, Epoch: 3, Try: 1}
		got := invocationID("run-1", script)
		want := "run-1/S1/" + string(responsibility) + "/2/3/1"
		if got != want {
			t.Fatalf("invocation ID = %q, want %q", got, want)
		}
	}
	if _, err := time.Parse(time.RFC3339, "2026-07-26T01:02:03Z"); err != nil {
		t.Fatal(err)
	}
}
