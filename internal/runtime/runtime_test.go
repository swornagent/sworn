package runtime

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/journal"
	"github.com/swornagent/sworn/internal/protocol"
)

var runtimeTestGitIdentity = gitx.Identity{Name: "Runtime Test Engine", Email: "engine@example.test"}

func runtimePlan(t *testing.T, release, repository, target, marker string) ([]byte, protocol.Plan) {
	t.Helper()
	slice := func(id, path string) protocol.Slice {
		return protocol.Slice{
			ID: id, Outcome: "Deliver " + id + ".",
			Scope:      protocol.Scope{Include: []string{path}, Exclude: []string{}},
			Acceptance: []protocol.Criterion{{ID: "A-" + id, Text: id + " is exact."}},
			Checks:     []string{"check " + id}, Constraints: []string{"deterministic"},
			DependsOn: []string{}, Consumes: []string{},
		}
	}
	metadata := protocol.Metadata{
		SchemaVersion: protocol.PlanVersion,
		Release:       release,
		Revision:      1,
		PreviousPlan:  nil,
		Repository:    repository,
		TargetRef:     target,
		ApprovalRef:   "operator://" + release + "/1",
		Tracks: []protocol.Track{
			{ID: "T1", DependsOn: []string{}, Slices: []protocol.Slice{slice("S1", "one.txt")}},
			{ID: "T2", DependsOn: []string{}, Slices: []protocol.Slice{slice("S2", "two.txt")}},
		},
	}
	metadataBody, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(
		"```protocol-plan-v2\n" + string(metadataBody) +
			"\n```\n\nFixture plan.\n",
	)
	plan, err := protocol.ParsePlan(body)
	if err != nil {
		t.Fatal(err)
	}
	return body, plan
}

// runtimeSingleTrackPlan is runtimePlan's one-track sibling: T1/S1 only, no
// independent T2/S2 lane to keep dispatching alongside a fixture that only
// sets up guard-mechanism plumbing for the one lane under test.
func runtimeSingleTrackPlan(t *testing.T, release, repository, target, marker string) ([]byte, protocol.Plan) {
	t.Helper()
	metadata := protocol.Metadata{
		SchemaVersion: protocol.PlanVersion,
		Release:       release,
		Revision:      1,
		PreviousPlan:  nil,
		Repository:    repository,
		TargetRef:     target,
		ApprovalRef:   "operator://" + release + "/1",
		Tracks: []protocol.Track{
			{ID: "T1", DependsOn: []string{}, Slices: []protocol.Slice{{
				ID: "S1", Outcome: "Deliver S1.",
				Scope:      protocol.Scope{Include: []string{"one.txt"}, Exclude: []string{}},
				Acceptance: []protocol.Criterion{{ID: "A-S1", Text: "S1 is exact."}},
				Checks:     []string{"check S1"}, Constraints: []string{"deterministic"},
				DependsOn: []string{}, Consumes: []string{},
			}}},
		},
	}
	metadataBody, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(
		"```protocol-plan-v2\n" + string(metadataBody) +
			"\n```\n\nFixture plan.\n",
	)
	plan, err := protocol.ParsePlan(body)
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

func fixtureManifest(t *testing.T) (Manifest, []byte, protocol.Plan) {
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
		protocolAttempt int64,
	) driver.Submission {
		script := ScriptedAttempt{Slice: slice, Responsibility: responsibility,
			ProtocolAttempt: protocolAttempt, Epoch: 1, Try: 1}
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
	lead := submission("S1", driver.LeadReview, 1)
	lead.Decision, _ = driver.NewDecision(driver.DecisionProceed)
	implementation := submission("S1", driver.ImplementerImplementation, 1)
	implementation.Checks, _ = driver.NewCheckBytes([]byte("implementation checks\n"))
	work := submission("S1", driver.WorkVerification, 1)
	work.Checks, _ = driver.NewCheckBytes([]byte("work checks\n"))
	work.Decision, _ = driver.NewDecision(driver.DecisionPass)
	assembly := submission("", driver.AssemblyVerification, 1)
	assembly.Checks, _ = driver.NewCheckBytes([]byte("assembly checks\n"))
	assembly.Decision, _ = driver.NewDecision(driver.DecisionPass)
	manifest := Manifest{
		GitIdentity:   runtimeTestGitIdentity,
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
			Executable: "/usr/bin/true",
			Digest:     "sha256:" + strings.Repeat("a", 64),
			AdapterKey: "fixture", Profile: "fixture",
		},
		Roles: driver.RoleSelections{
			Planner:     driver.RoleSelection{Profile: "fixture", Model: "planner-model"},
			Implementer: driver.RoleSelection{Profile: "fixture", Model: "implementer-model"},
			Lead:        driver.RoleSelection{Profile: "fixture", Model: "lead-model"},
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
			{Responsibility: driver.AssemblyVerification, ProtocolAttempt: 1, Epoch: 1, Try: 1,
				Behavior: "submit", Submission: encodeSubmission(t, assembly)},
			{Slice: "S1", Responsibility: driver.LeadReview, ProtocolAttempt: 1, Epoch: 1, Try: 1,
				Behavior: "submit", Submission: encodeSubmission(t, lead)},
			{Slice: "S1", Responsibility: driver.ImplementerDesign, ProtocolAttempt: 1, Epoch: 1, Try: 1,
				Behavior: "submit", Submission: encodeSubmission(t, design)},
			{Slice: "S1", Responsibility: driver.ImplementerImplementation, ProtocolAttempt: 1, Epoch: 1, Try: 1,
				Behavior: "submit", Submission: encodeSubmission(t, implementation)},
			{Responsibility: driver.PlannerProposal, ProtocolAttempt: 1, Epoch: 1, Try: 1,
				Behavior: "submit", Submission: encodeSubmission(t, planner)},
			{Slice: "S1", Responsibility: driver.WorkVerification, ProtocolAttempt: 1, Epoch: 1, Try: 1,
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
		`"schema_version":"sworn.runtime-manifest/v5"`,
		`"schema_version":"sworn.runtime-manifest/v5","unknown":true`,
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
	missingIdentity := manifest
	missingIdentity.GitIdentity = gitx.Identity{}
	missingIdentityBody, err := json.Marshal(missingIdentity)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admitManifest(append(missingIdentityBody, '\n')); !IsCode(err, "INVALID_GIT_IDENTITY") {
		t.Fatalf("missing v5 Git identity = %v", err)
	}
	v4 := manifest
	v4.SchemaVersion = ManifestVersionV4
	v4Body, err := json.Marshal(v4)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admitManifest(append(v4Body, '\n')); !IsCode(err, "MIGRATION_REQUIRED") {
		t.Fatalf("v4 migration = %v", err)
	}
}

func TestProtocolCommandPersistsIdentityWithoutChangingReplayIdentity(t *testing.T) {
	authority := protocolActionAuthority{
		Release: "release-1", Before: "sha256:" + strings.Repeat("a", 64),
		OwnerRef: "refs/heads/track/release-1/T1", OwnerHead: strings.Repeat("1", 40),
		ReleaseHead: strings.Repeat("2", 40), TargetRef: "refs/heads/main",
		TargetHead: strings.Repeat("3", 40),
	}
	first := runtimeTestGitIdentity
	second := gitx.Identity{Name: "Replacement Engine", Email: "replacement@example.test"}
	firstPayload := marshalActionCommand(first, authority, installActionInput{Reference: "plan.md"})
	secondPayload := marshalActionCommand(second, authority, installActionInput{Reference: "plan.md"})
	if bytes.Equal(firstPayload, secondPayload) || sha256Digest(firstPayload) == sha256Digest(secondPayload) {
		t.Fatal("changed persisted identity did not change the canonical command digest")
	}
	parsed, err := parseActionCommand(firstPayload)
	if err != nil || parsed.GitIdentity != first {
		t.Fatalf("persisted identity = %#v, %v", parsed.GitIdentity, err)
	}
	effect := journal.Effect{
		RunID: "run-1", ID: "effect-1", ReplayKey: "effect-1",
		Kind: "protocol.install", ExpectedDigest: sha256Digest(firstPayload),
	}
	changed := journal.Command{
		RunID: "run-1", ReplayKey: "effect-1", Kind: "protocol.install", Payload: secondPayload,
	}
	if err := validateRecoveryCommand(changed, effect, true); !IsCode(err, "CORRUPT_JOURNAL") {
		t.Fatalf("changed identity for existing work = %v", err)
	}
	legacy := append([]byte(nil), firstPayload...)
	legacy = bytes.Replace(legacy, []byte(protocolActionCommandVersion), []byte("sworn.protocol-action/v1"), 1)
	if _, err := parseActionCommand(legacy); !IsCode(err, "CORRUPT_JOURNAL") {
		t.Fatalf("legacy actionable command = %v", err)
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
		Lead: driver.RoleSelection{
			Profile: "lead-profile", Model: "lead-model",
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
				Responsibility:  driver.PlannerProposal,
				ProtocolAttempt: 1,
				Epoch:           1,
				Try:             1,
				Behavior:        "none",
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
		Kind: "protocol.install", Payload: payload,
	}
	effect := journal.Effect{
		RunID: "run-1", ID: "effect-1", ReplayKey: "effect-1",
		Kind: "protocol.install", ExpectedDigest: sha256Digest(payload),
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
			command.Kind = "protocol.merge"
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
	missing := &protocol.RecordError{Code: "REF_NOT_FOUND"}
	if proposalMatchesPendingAuthority(
		initial,
		gitx.RefHead{Ref: releaseRef, State: gitx.RefAbsent},
		gitx.RefHead{Ref: targetRef, State: gitx.RefDirect, Head: targetNew},
		protocol.State{},
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
		protocol.State{},
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
	revisionPlan, err := protocol.ParsePlan([]byte(
		"```protocol-plan-v2\n" + string(metadataBody) +
			"\n```\n\nFixture revision.\n",
	))
	if err != nil {
		t.Fatal(err)
	}
	state := protocol.State{
		Plan: protocol.PlanState{
			OID: priorPlan,
			Metadata: protocol.Metadata{
				Revision: 1,
			},
		},
		Refs: protocol.StateRefs{
			Release: protocol.CapturedRef{
				Ref: releaseRef, Head: releaseNew.String(),
			},
			Target: protocol.CapturedRef{
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
	state := protocol.State{
		Plan: protocol.PlanState{TargetStale: true},
	}
	for _, test := range []struct {
		responsibility driver.Responsibility
		slice          string
	}{
		{driver.ImplementerDesign, "S1"},
		{driver.LeadReview, "S1"},
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

func TestAllNewProtocolActionResultsReconstructFromDurableProjection(t *testing.T) {
	const release = "release-1"
	candidate := strings.Repeat("c", 40)
	targetHead := strings.Repeat("d", 40)
	planOID := strings.Repeat("e", 40)
	binds := strings.Repeat("f", 40)
	detail := []byte("exact detail")
	summary := "Exact durable action."
	target := targetHead
	candidateValue := candidate
	receipt := func(role, result string) protocol.Receipt {
		return protocol.Receipt{
			Version: protocol.ReceiptVersion,
			Release: release,
			Role:    role, Result: result,
			Plan: planOID, Binds: binds,
			Summary: summary, Target: &target,
			Candidate: &candidateValue,
		}
	}
	entry := func(oid string, value protocol.Receipt) *protocol.ReceiptEntry {
		return &protocol.ReceiptEntry{
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
	installApproval := protocol.ReceiptEntry{
		OID:     "approval-receipt",
		Detail:  installDetail(installAdmission),
		Receipt: installReceipt,
	}
	retiredSlice := "S-retired"
	retirementReceipt := protocol.Receipt{
		Version: protocol.ReceiptVersion,
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
	nextPlan, err := protocol.ParsePlan([]byte(
		"```protocol-plan-v2\n" + string(nextMetadataBytes) +
			"\n```\n\nLater fixture plan.\n",
	))
	if err != nil {
		t.Fatal(err)
	}
	nextPlanOID := strings.Repeat("2", 40)
	nextApproval := protocol.ReceiptEntry{
		OID:    "later-approval-receipt",
		Detail: []byte("later authority detail"),
		Receipt: protocol.Receipt{
			Version: protocol.ReceiptVersion,
			Release: release,
			Role:    "planner", Result: "approved",
			Plan:    nextPlanOID,
			Summary: "Install the exact locally authorized plan.",
			Target:  &target,
		},
	}
	installState := protocol.State{
		Release: release,
		Plan: protocol.PlanState{
			OID: nextPlanOID, Digest: nextPlan.Digest(),
			Metadata: nextPlan.Metadata(),
			Approval: nextApproval,
			History: []protocol.PlanHistory{
				{
					OID: planOID, Revision: 1,
					Approval: installApproval, Plan: installedPlan,
					InstallHead: "retirement-receipt",
					Retirements: []protocol.RetirementResult{{
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
		Refs: protocol.StateRefs{
			Release: protocol.CapturedRef{
				Ref:  "refs/heads/release-wt/" + release,
				Head: "later-release-head",
			},
			Target: protocol.CapturedRef{
				Ref: "refs/heads/main", Head: "later-target-head",
			},
		},
	}
	sliceID := "S1"
	sliceReceipt := receipt("lead", "proceed")
	sliceReceipt.Slice = &sliceID
	sliceEntry := entry("slice-receipt", sliceReceipt)
	sliceState := &protocol.SliceState{
		Location: protocol.SliceLocation{
			Track: protocol.Track{ID: "T1"},
			Slice: protocol.Slice{ID: sliceID},
		},
		History: protocol.SliceHistory{
			Entries: []protocol.ReceiptEntry{sliceEntry.Clone()},
		},
		CurrentReceipt: entry(
			"later-slice-receipt",
			receipt("implementer", "candidate"),
		),
	}
	appendState := protocol.State{
		Release: release,
		Slices:  []*protocol.SliceState{sliceState},
		Tracks: []protocol.TrackState{{
			ID: "T1", Ref: "refs/heads/track/release-1/T1",
		}},
	}
	assemblyReceipt := receipt("verifier", "pass")
	assemblyEntry := entry("assembly-verdict", assemblyReceipt)
	assemblyState := protocol.State{
		Release: release,
		Refs: protocol.StateRefs{
			Release: protocol.CapturedRef{
				Ref: "refs/heads/release-wt/" + release,
			},
			Target: protocol.CapturedRef{Ref: "refs/heads/main"},
		},
		Assembly: protocol.AssemblyState{
			History: []protocol.ReceiptEntry{assemblyEntry.Clone()},
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
	preparedState := protocol.State{
		Release: release,
		Assembly: protocol.AssemblyState{
			History: []protocol.ReceiptEntry{preparedEntry.Clone()},
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
	mergedState := protocol.State{
		Release: release,
		Refs: protocol.StateRefs{
			Target: protocol.CapturedRef{Ref: "refs/heads/main"},
		},
		Assembly: protocol.AssemblyState{
			History: []protocol.ReceiptEntry{mergedEntry.Clone()},
			CurrentReceipt: entry(
				"later-merge-receipt",
				laterMergedReceipt,
			),
			ResultCommit: strings.Repeat("8", 40),
		},
	}
	tests := []struct {
		name       string
		state      protocol.State
		kind       string
		command    protocolActionCommand
		wantAction string
		wantCommit string
		wantHead   string
		wantTarget string
		wantResult string
	}{
		{
			name: "install", state: installState, kind: "protocol.install",
			command: protocolActionCommand{
				Authority: protocolActionAuthority{
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
			kind: "protocol.append_receipt",
			command: protocolActionCommand{
				Authority: protocolActionAuthority{
					Release: release, Plan: planOID, Binds: binds,
				},
				Input: mustJSON(protocol.AppendReceiptInput{
					Release: release, Slice: sliceID,
					Role: "lead", Result: "proceed",
					Summary: summary, Detail: detail,
				}),
			},
			wantAction: "appendReceipt", wantCommit: "slice-receipt",
		},
		{
			name: "assembly_verdict", state: assemblyState,
			kind: "protocol.assembly_verdict",
			command: protocolActionCommand{
				Authority: protocolActionAuthority{
					Release: release, Plan: planOID, Binds: binds,
				},
				Input: mustJSON(protocol.AppendReceiptInput{
					Release: release, Role: "verifier", Result: "pass",
					Summary: summary, Detail: detail,
				}),
			},
			wantAction: "appendReceipt", wantCommit: "assembly-verdict",
		},
		{
			name: "prepare_assembly", state: preparedState,
			kind: "protocol.prepare_assembly",
			command: protocolActionCommand{
				Authority: protocolActionAuthority{
					Release: release, Plan: planOID, Binds: binds,
					TargetHead: targetHead,
				},
				Input: mustJSON(protocol.PrepareAssemblyInput{
					Release: release, Summary: summary, Detail: detail,
				}),
			},
			wantAction: "prepareAssembly", wantCommit: "assembly-candidate",
		},
		{
			name: "merge", state: mergedState, kind: "protocol.merge",
			command: protocolActionCommand{
				Authority: protocolActionAuthority{
					Release: release, Plan: planOID, Binds: binds,
					Candidate: candidate, TargetHead: targetHead,
				},
				Input: mustJSON(protocol.MergePassedCandidateInput{
					Release: release, Summary: summary, Detail: detail,
				}),
			},
			wantAction: "mergePassedCandidate", wantCommit: "merge-receipt",
			wantTarget: "refs/heads/main", wantResult: mergedResult,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := reconstructAllNewProtocolAction(
				test.state, test.kind, test.command)
			if err != nil {
				t.Fatal(err)
			}
			if result.Kind != "protocol.action-result/v2" ||
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
		retired.SliceHistories = []protocol.SliceHistoryState{{
			Slice: sliceID, Track: "T1",
			Ref: "refs/heads/track/" + release + "/T1",
			History: protocol.SliceHistory{
				Entries: []protocol.ReceiptEntry{
					sliceEntry.Clone(),
				},
			},
		}}
		command := protocolActionCommand{
			Authority: protocolActionAuthority{
				Release: release, Plan: planOID, Binds: binds,
			},
			Input: mustJSON(protocol.AppendReceiptInput{
				Release: release, Slice: sliceID,
				Role: "lead", Result: "proceed",
				Summary: summary, Detail: detail,
			}),
		}
		result, err := reconstructAllNewProtocolAction(
			retired,
			"protocol.append_receipt",
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
			[]protocol.PlanHistory(nil), installState.Plan.History...)
		substituted.Plan.History[0].Approval =
			substituted.Plan.History[0].Approval.Clone()
		substituted.Plan.History[0].Approval.Detail = []byte("other authority\n")

		command := protocolActionCommand{
			Authority: protocolActionAuthority{
				Release: release, TargetHead: targetHead,
			},
			Input: mustJSON(installActionInput{
				PlanBytes:  planBytes,
				PlanDigest: installedPlan.Digest(),
				Reference:  planMetadata.ApprovalRef,
			}),
		}
		applied, applyErr := actionAlreadyApplied(
			substituted, "protocol.install", command)
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
		external := protocol.State{
			Release: release,
			Plan: protocol.PlanState{
				OID: planOID, Digest: installedPlan.Digest(),
				Metadata: planMetadata, Approval: externalApproval,
			},
			Refs: protocol.StateRefs{
				Release: protocol.CapturedRef{
					Ref: releaseRef, Head: "external-release-head",
				},
				Target: protocol.CapturedRef{
					Ref: targetRef, Head: targetHead,
				},
			},
		}
		command := protocolActionCommand{
			Authority: protocolActionAuthority{
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
			external, "protocol.install", command)
		if applyErr != nil {
			t.Fatal(applyErr)
		}
		if applied {
			t.Fatal("external Protocol approval inferred a Sworn effect")
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
	expected := protocol.ActionResult{
		Kind: "protocol.action-result/v2", Action: "recordPlanRevision",
		Release: "release-1", Revision: 2,
		Plan: strings.Repeat("4", 40),
		Ref:  "refs/heads/release-wt/release-1",
		Head: strings.Repeat("5", 40), Target: target,
		ReceiptCommit: receiptCommit,
		Receipt: &protocol.Receipt{
			Version: protocol.ReceiptVersion,
			Release: "release-1", Role: "planner", Result: "approved",
			Plan: strings.Repeat("4", 40), Target: &target,
		},
		Retirements: []protocol.RetirementResult{{
			Slice: retired, ReceiptCommit: strings.Repeat("6", 40),
			Receipt: protocol.Receipt{
				Version: protocol.ReceiptVersion,
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
	for name, mutate := range map[string]func(*protocol.ActionResult){
		"plan": func(value *protocol.ActionResult) {
			value.Plan = strings.Repeat("7", 40)
		},
		"head": func(value *protocol.ActionResult) {
			value.Head = strings.Repeat("7", 40)
		},
		"receipt": func(value *protocol.ActionResult) {
			value.ReceiptCommit = strings.Repeat("7", 40)
		},
		"retirement": func(value *protocol.ActionResult) {
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
	receipt := &protocol.ReceiptEntry{OID: "receipt-current"}
	currentSlice := &protocol.SliceState{
		Location: protocol.SliceLocation{
			Track: metadata.Tracks[0],
			Slice: sliceDefinition,
		},
		Stage: "design", Status: "ready", NextRole: "implementer",
		Attempt: 1, CurrentReceipt: receipt, InputPins: map[string]string{},
	}
	state := protocol.State{
		Release: metadata.Release,
		Plan: protocol.PlanState{
			OID: "plan-v2", Digest: plan.Digest(), Metadata: metadata,
		},
		Refs: protocol.StateRefs{
			Release: protocol.CapturedRef{Head: "release-v2"},
			Target:  protocol.CapturedRef{Head: "target-v1"},
		},
		Tracks: []protocol.TrackState{{
			ID: metadata.Tracks[0].ID, Ref: "refs/heads/track/release-1/T1",
			Head: "track-v1", Slices: []*protocol.SliceState{currentSlice},
		}},
		Slices: []*protocol.SliceState{currentSlice},
	}
	old := state
	old.Plan.OID = "plan-v1"
	oldBefore := sliceFingerprint(old, sliceDefinition.ID)
	oldWork := driverWorkIdentity(manifest.digest, sliceDefinition.ID,
		driver.ImplementerDesign, 1, oldBefore)
	if lanes := readyLaneCandidates(manifest, nil, true, state, journal.Snapshot{}); lanesContainWork(lanes, oldWork) {
		t.Fatal("changed-plan exhaustion still parks current work")
	}
	currentBefore := sliceFingerprint(state, sliceDefinition.ID)
	currentWork := driverWorkIdentity(manifest.digest, sliceDefinition.ID,
		driver.ImplementerDesign, 1, currentBefore)
	if lanes := readyLaneCandidates(manifest, nil, true, state, journal.Snapshot{}); !lanesContainWork(lanes, currentWork) {
		t.Fatal("exact current exhaustion did not park")
	}
	currentTrackBaseWork := workIdentity(
		trackBaseBefore(state, currentSlice),
		"git.prepare_track_base",
	)
	if lanes := readyLaneCandidates(
		manifest, nil, true, state, journal.Snapshot{},
	); !lanesContainWork(lanes, currentTrackBaseWork) {
		t.Fatal("exact current track-base exhaustion did not park")
	}
	state.Slices = nil
	state.Tracks[0].Slices = nil
	if lanes := readyLaneCandidates(manifest, nil, true, state, journal.Snapshot{}); lanesContainWork(lanes, oldWork) {
		t.Fatal("retired-slice exhaustion still parks current work")
	}
}

func lanesContainWork(lanes []laneCandidates, work string) bool {
	for _, lane := range lanes {
		if _, ok := lane.works[work]; ok {
			return true
		}
	}
	return false
}

func TestInvocationIdentityIsStableAcrossResume(t *testing.T) {
	t.Parallel()

	for _, responsibility := range []driver.Responsibility{
		driver.PlannerProposal,
		driver.ImplementerDesign,
		driver.LeadReview,
		driver.ImplementerImplementation,
		driver.WorkVerification,
		driver.AssemblyVerification,
	} {
		script := ScriptedAttempt{Slice: "S1", Responsibility: responsibility,
			ProtocolAttempt: 2, Epoch: 3, Try: 1}
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

func TestStatusReadPathExcludesDerivedWorksFromExhaustionAndMarksParked(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 19, 1, 0, 0, 0, time.UTC)
	repoPath := productionRepository(t)
	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		t.Fatal(err)
	}

	manifestValue, _, plan := fixtureManifest(t)
	manifestValue.Repository = repoPath
	body, err := canonicalManifest(manifestValue)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := admitManifest(body)
	if err != nil {
		t.Fatal(err)
	}

	repoView, err := gitx.Open(repoPath, gitExecutable)
	if err != nil {
		t.Fatal(err)
	}

	inertness := func(request gitx.RecordRootRequest) (gitx.RecordRootDecision, error) {
		return gitx.RecordRootDecision{Kind: request.Kind, Repository: request.Repository,
			RecordRoot: request.RecordRoot, Commit: request.Commit, Decision: "inert"}, nil
	}
	actions, err := protocol.NewActions(protocol.UseGitRepository(repoView), inertness, manifest.value.GitIdentity)
	if err != nil {
		t.Fatal(err)
	}
	installer := newAuthorityInstaller(actions)
	targetP := runRuntimeGit(t, repoPath, "rev-parse", "refs/heads/main")
	// Single-track plan: no independent T2 lane stays admissible while T1
	// exhausts, so the run-level parked assertion below stays exactly the
	// "only remaining work" shape lane-scoped parking (A1) leaves unchanged.
	singleTrackMetadata := plan.Metadata()
	singleTrackMetadata.Tracks = singleTrackMetadata.Tracks[:1]
	singleTrackBody, err := json.MarshalIndent(singleTrackMetadata, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	singleTrackPlanBytes := []byte(
		"```protocol-plan-v2\n" + string(singleTrackBody) + "\n```\n\nFixture plan.\n",
	)
	singleTrackPlan, err := protocol.ParsePlan(singleTrackPlanBytes)
	if err != nil {
		t.Fatal(err)
	}
	admission := approvalAdmission{
		planBytes:  singleTrackPlan.Bytes(),
		planDigest: singleTrackPlan.Digest(),
		reference:  singleTrackPlan.Metadata().ApprovalRef,
	}
	if _, err := installer.install(admission, targetP); err != nil {
		t.Fatal(err)
	}

	sliceDefinition := plan.Metadata().Tracks[0].Slices[0]
	if _, err := actions.AppendReceipt(protocol.AppendReceiptInput{
		Release: manifest.value.Release, Slice: sliceDefinition.ID,
		Role: "implementer", Result: "designed",
		Summary: "Designed S1", Detail: []byte("Design detail"),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := actions.AppendReceipt(protocol.AppendReceiptInput{
		Release: manifest.value.Release, Slice: sliceDefinition.ID,
		Role: "lead", Result: "proceed",
		Summary: "Proceed S1", Detail: []byte("Proceed detail"),
	}); err != nil {
		t.Fatal(err)
	}

	store, err := journal.Open(ctx, filepath.Join(t.TempDir(), "status-parked.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	run := journal.Run{
		ID:             manifest.value.RunID,
		ManifestDigest: manifest.digest,
		Repository:     manifest.value.Repository,
		Release:        manifest.value.Release,
		TargetRef:      manifest.value.TargetRef,
		CreatedAt:      now,
	}
	if err := store.RegisterRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordCommand(ctx, journal.Command{
		RunID: run.ID, ReplayKey: "manifest", Kind: "start",
		Payload: manifest.raw, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	state, err := protocol.ReadState(protocol.UseGitRepository(repoView), manifest.value.Release, inertness)
	if err != nil {
		t.Fatal(err)
	}
	slice, ok := state.Slice(sliceDefinition.ID)
	if !ok || slice.CurrentReceipt == nil {
		t.Fatal("slice or current receipt not found")
	}
	track, ok := state.Track(slice.Location.Track.ID)
	if !ok {
		t.Fatal("track not found")
	}
	before := sliceFingerprint(state, sliceDefinition.ID)
	cycleWork := workIdentity(before, "git.seal")
	dispatchWork := workIdentity(cycleWork, "driver.dispatch")
	preparedWork := workIdentity(cycleWork, "git.seal.prepared")

	cyclePayload := mustJSON(implementationCycle{
		Release: manifest.value.Release, GitIdentity: manifest.value.GitIdentity,
		Slice: sliceDefinition.ID, Binds: slice.CurrentReceipt.OID, Before: before,
		Plan: state.Plan.OID, ReleaseHead: state.Refs.Release.Head, TargetHead: state.Refs.Target.Head,
		Track: track.ID, TrackRef: track.Ref,
		TrackHead: track.Head, DispatchWork: dispatchWork,
		DispatchEffect: journal.AttemptEffectID(dispatchWork, 1, 1),
		PreparedWork:   preparedWork,
		PreparedEffect: journal.AttemptEffectID(preparedWork, 1, 1),
	})

	// Record the git.seal command and effects for git.seal, driver.dispatch, git.seal.prepared (3 tries failed)
	for try := int64(1); try <= 3; try++ {
		// Cycle work
		cycleEffectID := journal.AttemptEffectID(cycleWork, 1, try)
		if err := store.EnsureAttempt(ctx,
			journal.Command{RunID: run.ID, ReplayKey: cycleEffectID, Kind: "git.seal",
				Payload: cyclePayload, CreatedAt: now},
			journal.Effect{RunID: run.ID, ID: cycleEffectID, ReplayKey: cycleEffectID, Kind: "git.seal",
				BeforeDigest: cycleWork, ExpectedDigest: sha256Digest(cyclePayload), UpdatedAt: now},
			journal.EffectAttempt{WorkID: cycleWork, Epoch: 1, Try: try}); err != nil {
			t.Fatal(err)
		}
		claim, err := store.Claim(ctx, run.ID, cycleEffectID, now, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Complete(ctx, journal.Completion{RunID: run.ID, EffectID: cycleEffectID,
			Token: claim.Token, State: journal.OperationalFailed, ErrorCode: "cycle_failure",
			EventKind: "failed", At: now}); err != nil {
			t.Fatal(err)
		}

		// Derived dispatch
		dispatchEffectID := journal.AttemptEffectID(dispatchWork, 1, try)
		if err := store.EnsureAttempt(ctx,
			journal.Command{RunID: run.ID, ReplayKey: dispatchEffectID, Kind: "driver.dispatch",
				Payload: []byte("dispatch"), CreatedAt: now},
			journal.Effect{RunID: run.ID, ID: dispatchEffectID, ReplayKey: dispatchEffectID, Kind: "driver.dispatch",
				BeforeDigest: cycleWork, ExpectedDigest: sha256Digest([]byte("dispatch")), UpdatedAt: now},
			journal.EffectAttempt{WorkID: dispatchWork, Epoch: 1, Try: try}); err != nil {
			t.Fatal(err)
		}
		claim, err = store.Claim(ctx, run.ID, dispatchEffectID, now, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Complete(ctx, journal.Completion{RunID: run.ID, EffectID: dispatchEffectID,
			Token: claim.Token, State: journal.OperationalFailed, ErrorCode: "dispatch_failure",
			EventKind: "failed", At: now}); err != nil {
			t.Fatal(err)
		}
	}

	service := &Service{
		journal:       store,
		gitExecutable: gitExecutable,
		now:           func() time.Time { return now },
	}
	status, err := service.Status(ctx, run.ID)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}

	if status.State != "parked" {
		t.Fatalf("status.State = %q, want 'parked'", status.State)
	}

	// Verify Derived flags on EffectStatus
	effects := make(map[string]EffectStatus)
	for _, effect := range status.Effects {
		effects[effect.ID] = effect
	}

	cycleT3 := effects[journal.AttemptEffectID(cycleWork, 1, 3)]
	if cycleT3.Derived {
		t.Fatalf("git.seal effect marked as Derived: %#v", cycleT3)
	}

	dispatchT3 := effects[journal.AttemptEffectID(dispatchWork, 1, 3)]
	if !dispatchT3.Derived {
		t.Fatalf("driver.dispatch effect not marked as Derived: %#v", dispatchT3)
	}
}

func TestRuntimeJournalEventsCarryStructuredAssociation(t *testing.T) {
	ctx := context.Background()
	repository := productionRepository(t)
	manifestValue, _, plan := fixtureManifest(t)
	manifestValue.Repository = repository

	metadata := plan.Metadata()
	metadata.Tracks = metadata.Tracks[:1]
	metadataBody, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	planBytes := []byte("```protocol-plan-v2\n" + string(metadataBody) + "\n```\n\nFixture plan.\n")
	plan, err = protocol.ParsePlan(planBytes)
	if err != nil {
		t.Fatal(err)
	}
	manifestValue.Authority.BootstrapApprovedPlanDigest = nil

	for index := range manifestValue.Scripts {
		script := &manifestValue.Scripts[index]
		if script.Responsibility != driver.PlannerProposal {
			continue
		}
		encoded, err := base64.StdEncoding.DecodeString(script.Submission)
		if err != nil {
			t.Fatal(err)
		}
		submission, err := driver.DecodeSubmission(encoded)
		if err != nil {
			t.Fatal(err)
		}
		submission.Plan, err = driver.NewPlanBytes(plan.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		script.Submission = encodeSubmission(t, submission)
	}

	body, err := canonicalManifest(manifestValue)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := admitManifest(body)
	if err != nil {
		t.Fatal(err)
	}

	submissions := make(map[string][]byte, len(manifest.value.Scripts))
	for _, script := range manifest.value.Scripts {
		encoded, err := base64.StdEncoding.DecodeString(script.Submission)
		if err != nil {
			t.Fatal(err)
		}
		submissions[invocationID(manifest.value.RunID, script)] = encoded
	}

	dispatcher := fixtureDriver(func(
		_ context.Context,
		invocation driver.Invocation,
	) (driver.Observation, error) {
		submission := submissions[invocation.Request.InvocationID]
		if len(submission) == 0 {
			t.Fatalf("unexpected invocation %s", invocation.Request.InvocationID)
		}
		if strings.Contains(invocation.Request.InvocationID, "implementer_implementation") {
			if err := os.WriteFile(
				filepath.Join(invocation.HostWorkspace, "one.txt"),
				[]byte("impl content\n"),
				0o600,
			); err != nil {
				t.Fatal(err)
			}
		}
		return driver.Observation{
			TransportStatus: driver.Completed,
			Usage: driver.UsageReceipt{
				TokenStatus: driver.UsageUnavailable,
				CostStatus:  driver.UsageUnavailable,
			},
			Diagnostic: driver.Diagnostic{Code: "none"},
			Handoff: &driver.SealedHandoff{
				SubmissionBytes:  submission,
				SubmissionDigest: driver.Digest(submission),
			},
		}, nil
	})

	store, err := journal.Open(ctx, filepath.Join(t.TempDir(), "association-test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 20, 1, 0, 0, 0, time.UTC)
	service := &Service{
		journal:       store,
		dispatcher:    dispatcher,
		gitExecutable: gitExecutable,
		now:           func() time.Time { return now },
	}

	status, err := service.Start(ctx, body)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if status.State != "awaiting_approval" {
		t.Fatalf("status = %q, want 'awaiting_approval'", status.State)
	}

	offer, err := service.ApprovalOffer(ctx, manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Approve(ctx, offer.Command); err != nil {
		t.Fatalf("Approve failed: %v", err)
	}

	status, err = service.Wait(ctx, manifest.value.RunID)
	if err != nil {
		t.Fatalf("Wait failed: %v", err)
	}
	if status.State != "complete" {
		t.Fatalf("status = %q, want 'complete'; effects=%#v", status.State, status.Effects)
	}

	snapshot, err := store.Snapshot(ctx, manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Events) == 0 {
		t.Fatal("no events journaled")
	}

	observedKinds := make(map[string]int)
	for _, event := range snapshot.Events {
		observedKinds[event.Kind]++
		if len(event.Body) == 0 {
			continue
		}
		var assoc EventAssociation
		if err := json.Unmarshal(event.Body, &assoc); err != nil {
			continue
		}
		if assoc.EffectID == "" || assoc.WorkID == "" {
			t.Fatalf("event %s (offset %d) has empty EffectID or WorkID: %#v", event.Kind, event.Offset, assoc)
		}
		switch event.Kind {
		case "candidate_sealed", "candidate_prepared":
			if assoc.Slice != "S1" {
				t.Fatalf("event %s has unexpected slice %q", event.Kind, assoc.Slice)
			}
			if assoc.Track != "T1" {
				t.Fatalf("event %s has unexpected track %q", event.Kind, assoc.Track)
			}
		case "dispatch_completed":
			if assoc.Slice != "" && assoc.Slice != "S1" {
				t.Fatalf("dispatch_completed has unexpected slice %q", assoc.Slice)
			}
			if !knownDispatchResponsibility(assoc.Responsibility) {
				t.Fatalf("dispatch_completed has unexpected responsibility %q", assoc.Responsibility)
			}
			assertEventBodyKeysWithin(
				t,
				event.Body,
				[]string{"effect_id", "work_id", "responsibility"},
				[]string{"effect_id", "work_id", "track", "slice", "responsibility"},
			)
		}
	}

	for _, requiredKind := range []string{
		"candidate_sealed", "candidate_prepared", "dispatch_completed", "protocol_action_completed",
	} {
		if observedKinds[requiredKind] == 0 {
			t.Fatalf("required event kind %s was not observed (observed: %v)", requiredKind, observedKinds)
		}
	}
}

// A2: the runtime's attempt-write fallback is loud. An observation whose
// receipt cannot encode (here: a partial v1-shaped receipt with no cost
// status) still lands a receipt naming the surface and the capture-failed
// reason instead of the silent all-unavailable literal.
func TestRuntimeUsageFallbackNamesSurfaceAndReason(t *testing.T) {
	ctx := context.Background()
	repository := productionRepository(t)
	manifestValue, _, plan := fixtureManifest(t)
	manifestValue.Repository = repository

	metadata := plan.Metadata()
	metadata.Tracks = metadata.Tracks[:1]
	metadataBody, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	planBytes := []byte("```protocol-plan-v2\n" + string(metadataBody) + "\n```\n\nFixture plan.\n")
	plan, err = protocol.ParsePlan(planBytes)
	if err != nil {
		t.Fatal(err)
	}
	manifestValue.Authority.BootstrapApprovedPlanDigest = nil

	for index := range manifestValue.Scripts {
		script := &manifestValue.Scripts[index]
		if script.Responsibility != driver.PlannerProposal {
			continue
		}
		encoded, err := base64.StdEncoding.DecodeString(script.Submission)
		if err != nil {
			t.Fatal(err)
		}
		submission, err := driver.DecodeSubmission(encoded)
		if err != nil {
			t.Fatal(err)
		}
		submission.Plan, err = driver.NewPlanBytes(plan.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		script.Submission = encodeSubmission(t, submission)
	}

	body, err := canonicalManifest(manifestValue)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := admitManifest(body)
	if err != nil {
		t.Fatal(err)
	}
	submissions := make(map[string][]byte, len(manifest.value.Scripts))
	for _, script := range manifest.value.Scripts {
		encoded, err := base64.StdEncoding.DecodeString(script.Submission)
		if err != nil {
			t.Fatal(err)
		}
		submissions[invocationID(manifest.value.RunID, script)] = encoded
	}
	dispatcher := fixtureDriver(func(
		_ context.Context,
		invocation driver.Invocation,
	) (driver.Observation, error) {
		submission := submissions[invocation.Request.InvocationID]
		if len(submission) == 0 {
			t.Fatalf("unexpected invocation %s", invocation.Request.InvocationID)
		}
		if strings.Contains(
			invocation.Request.InvocationID,
			"implementer_implementation",
		) {
			if err := os.WriteFile(
				filepath.Join(invocation.HostWorkspace, "one.txt"),
				[]byte("impl content\n"),
				0o600,
			); err != nil {
				t.Fatal(err)
			}
		}
		return driver.Observation{
			TransportStatus: driver.Completed,
			// A partial receipt with no cost status cannot encode; the seam
			// must substitute the loud fallback.
			Usage: driver.UsageReceipt{
				TokenStatus: driver.UsageUnavailable,
			},
			Diagnostic: driver.Diagnostic{Code: "none"},
			Handoff: &driver.SealedHandoff{
				SubmissionBytes:  submission,
				SubmissionDigest: driver.Digest(submission),
			},
		}, nil
	})
	store, err := journal.Open(ctx, filepath.Join(t.TempDir(), "loud-fallback.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 21, 1, 0, 0, 0, time.UTC)
	service := &Service{
		journal:       store,
		dispatcher:    dispatcher,
		gitExecutable: gitExecutable,
		now:           func() time.Time { return now },
	}
	status, err := service.Start(ctx, body)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if status.State != "awaiting_approval" {
		t.Fatalf("status = %q, want 'awaiting_approval'", status.State)
	}
	offer, err := service.ApprovalOffer(ctx, manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Approve(ctx, offer.Command); err != nil {
		t.Fatalf("Approve failed: %v", err)
	}
	status, err = service.Wait(ctx, manifest.value.RunID)
	if err != nil || status.State != "complete" {
		t.Fatalf("Wait failed: state=%q error=%v", status.State, err)
	}
	observation, err := store.ReadObservation(
		ctx,
		manifest.value.RunID,
		journal.MaxObservationAttempts,
		journal.MaxObservationEvents,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(observation.Attempts) == 0 {
		t.Fatal("no attempts journaled")
	}
	for _, attempt := range observation.Attempts {
		var usage driver.UsageReceipt
		if err := json.Unmarshal(attempt.Usage, &usage); err != nil ||
			usage.SchemaVersion != driver.UsageSchemaV2 ||
			usage.Surface != driver.FakeDriverID ||
			usage.UnavailableReason != driver.UsageReasonCaptureFailed ||
			usage.TokenStatus != driver.UsageUnavailable {
			t.Fatalf("fallback usage = %s error=%v", attempt.Usage, err)
		}
	}
}

// Correction 2: the fresh-rehydrate zero-observation predicate keeps its
// exact semantics through the field-wise receipt emptiness check.
func TestZeroDriverObservationPredicatePreservesFreshRehydrateSemantics(
	t *testing.T,
) {
	if !zeroDriverObservation(driver.Observation{}) {
		t.Fatal("zero observation was not detected")
	}
	variants := []driver.Observation{
		{TransportStatus: driver.Completed},
		{DurationMillis: 1},
		{Usage: driver.UsageReceipt{TokenStatus: driver.UsageUnavailable}},
		{Usage: driver.UsageReceipt{SchemaVersion: driver.UsageSchemaV2}},
		{Diagnostic: driver.Diagnostic{Code: "none"}},
		{Handoff: &driver.SealedHandoff{}},
		{Events: []driver.TerminalEvent{{Sequence: 1, Kind: "published"}}},
	}
	for _, observation := range variants {
		if zeroDriverObservation(observation) {
			t.Fatalf("observation %#v reported zero", observation)
		}
	}
}
