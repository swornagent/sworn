//go:build linux

package e2e

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/journal"
	swornruntime "github.com/swornagent/sworn/internal/runtime"
)

func e2eDigest(body []byte) string {
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func e2eWorkIdentity(values ...any) string {
	body, err := json.Marshal(values)
	if err != nil {
		panic(err)
	}
	return e2eDigest(append(body, '\n'))
}

type e2ePlanProposalAuthority struct {
	Release      string `json:"release"`
	PriorPlan    string `json:"prior_plan,omitempty"`
	ReleaseRef   string `json:"release_ref"`
	ReleaseHead  string `json:"release_head,omitempty"`
	TargetRef    string `json:"target_ref"`
	TargetHead   string `json:"target_head"`
	Before       string `json:"before"`
	SourceWork   string `json:"source_work"`
	SourceEffect string `json:"source_effect"`
}

type e2ePlanProposalCommand struct {
	Version    string                   `json:"version"`
	Authority  e2ePlanProposalAuthority `json:"authority"`
	PlanBytes  []byte                   `json:"plan_bytes"`
	PlanDigest string                   `json:"plan_digest"`
}

func recordPlannerProposalFixture(
	t *testing.T,
	journalPath string,
	runID string,
	planBytes []byte,
	state baton.State,
) {
	t.Helper()
	plan, err := baton.ParsePlan(planBytes)
	if err != nil {
		t.Fatal(err)
	}
	store, err := journal.Open(context.Background(), journalPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	snapshot, err := store.Snapshot(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	authority := e2ePlanProposalAuthority{
		Release: state.Release, PriorPlan: state.Plan.OID,
		ReleaseRef:  state.Refs.Release.Ref,
		ReleaseHead: state.Refs.Release.Head,
		TargetRef:   state.Refs.Target.Ref,
		TargetHead:  state.Refs.Target.Head,
	}
	authority.Before = e2eWorkIdentity(
		authority.Release,
		authority.PriorPlan,
		authority.ReleaseRef,
		authority.ReleaseHead,
		authority.TargetRef,
		authority.TargetHead,
	)
	revision := plan.Metadata().Revision
	authority.SourceWork = e2eWorkIdentity(
		snapshot.Run.ManifestDigest,
		"",
		driver.PlannerProposal,
		revision,
		authority.Before,
	)
	authority.SourceEffect = journal.AttemptEffectID(
		authority.SourceWork, 1, 1)
	submission := driver.Submission{
		SchemaVersion: driver.SubmissionSchemaVersion,
		InvocationID: fmt.Sprintf(
			"%s/release/%s/%d/1/1",
			runID, driver.PlannerProposal, revision,
		),
		Responsibility: driver.PlannerProposal,
		Summary:        "Exact revised plan.",
		Detail:         "Durable fixture proposal.",
	}
	submission.Plan, err = driver.NewPlanBytes(planBytes)
	if err != nil {
		t.Fatal(err)
	}
	submissionBody, err := driver.EncodeSubmission(submission)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := store.EnsureAttempt(
		context.Background(),
		journal.Command{
			RunID: runID, ReplayKey: authority.SourceEffect,
			Kind: "driver.dispatch", Payload: []byte("{}\n"), CreatedAt: now,
		},
		journal.Effect{
			RunID: runID, ID: authority.SourceEffect,
			ReplayKey: authority.SourceEffect, Kind: "driver.dispatch",
			BeforeDigest:   e2eDigest([]byte(authority.Before)),
			ExpectedDigest: e2eDigest(submissionBody), UpdatedAt: now,
		},
		journal.EffectAttempt{
			WorkID: authority.SourceWork, Epoch: 1, Try: 1,
		},
	); err != nil {
		t.Fatal(err)
	}
	claim, err := store.Claim(
		context.Background(), runID, authority.SourceEffect, now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Complete(context.Background(), journal.Completion{
		RunID: runID, EffectID: authority.SourceEffect, Token: claim.Token,
		State: journal.Succeeded, Result: submissionBody,
		EventKind: "dispatch_completed",
		EventBody: []byte(driver.PlannerProposal), At: now,
	}); err != nil {
		t.Fatal(err)
	}
	wire := e2ePlanProposalCommand{
		Version: "sworn.plan-proposal/v1", Authority: authority,
		PlanBytes: planBytes, PlanDigest: plan.Digest(),
	}
	payload, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	replayKey := fmt.Sprintf(
		"plan-proposal/%020d/%s",
		revision,
		strings.TrimPrefix(authority.SourceWork, "sha256:"),
	)
	if err := store.RecordCommand(context.Background(), journal.Command{
		RunID: runID, ReplayKey: replayKey, Kind: "planner_proposal",
		Payload: append(payload, '\n'), CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
}

func scriptedSubmission(t *testing.T, runID, slice string, responsibility driver.Responsibility,
	batonAttempt, epoch, try int64) string {
	t.Helper()
	work := slice
	if work == "" {
		work = "release"
	}
	submission := driver.Submission{SchemaVersion: driver.SubmissionSchemaVersion,
		InvocationID: fmt.Sprintf("%s/%s/%s/%d/%d/%d",
			runID, work, responsibility, batonAttempt, epoch, try),
		Responsibility: responsibility, Summary: "Exact " + string(responsibility) + ".",
		Detail: "Deterministic topology evidence."}
	switch responsibility {
	case driver.CaptainReview:
		submission.Decision, _ = driver.NewDecision(driver.DecisionProceed)
	case driver.ImplementerImplementation:
		submission.Checks, _ = driver.NewCheckBytes([]byte("implementation checks\n"))
	case driver.WorkVerification, driver.AssemblyVerification:
		submission.Checks, _ = driver.NewCheckBytes([]byte("work checks\n"))
		submission.Decision, _ = driver.NewDecision(driver.DecisionPass)
	}
	return encodedSubmission(t, submission)
}

func exactPlannerSubmission(t *testing.T, runID string, attempt, try int64, plan []byte) string {
	t.Helper()
	submission := driver.Submission{SchemaVersion: driver.SubmissionSchemaVersion,
		InvocationID: fmt.Sprintf("%s/release/%s/%d/1/%d",
			runID, driver.PlannerProposal, attempt, try),
		Responsibility: driver.PlannerProposal, Summary: "Exact revised plan.",
		Detail: "Deterministic revision evidence."}
	submission.Plan, _ = driver.NewPlanBytes(plan)
	return encodedSubmission(t, submission)
}

func exactBlockedSubmission(t *testing.T, runID string, try int64) string {
	t.Helper()
	submission := driver.Submission{SchemaVersion: driver.SubmissionSchemaVersion,
		InvocationID:   fmt.Sprintf("%s/S1/%s/1/1/%d", runID, driver.WorkVerification, try),
		Responsibility: driver.WorkVerification, Summary: "Block exact revision input.",
		Detail: "Fresh verification requires a revised contract."}
	submission.Checks, _ = driver.NewCheckBytes([]byte("blocked work checks\n"))
	submission.Decision, _ = driver.NewDecision(driver.DecisionBlocked)
	return encodedSubmission(t, submission)
}

func revisedPlan(t *testing.T, repository string, initialBytes []byte, initial baton.Plan,
) ([]byte, baton.Plan) {
	t.Helper()
	command := exec.Command(e2eGit, "-C", repository, "hash-object", "--stdin")
	command.Stdin = bytes.NewReader(initialBytes)
	rawOID, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	previous := strings.TrimSpace(string(rawOID))
	metadata := initial.Metadata()
	metadata.Revision, metadata.PreviousPlan = 2, &previous
	metadata.ApprovalRef = fmt.Sprintf(
		"operator://%s/%d", metadata.Release, metadata.Revision)
	metadata.Tracks[0].Slices[0].Outcome = "Deliver revised S1."
	metadataBody, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	body := []byte("```baton-plan-v2\n" + string(metadataBody) +
		"\n```\n\nReal-binary revision-2 topology plan.\n")
	plan, err := baton.ParsePlan(body)
	if err != nil {
		t.Fatal(err)
	}
	return body, plan
}

func singleTrackPlan(t *testing.T, initial baton.Plan) ([]byte, baton.Plan) {
	t.Helper()
	metadata := initial.Metadata()
	metadata.Tracks = append([]baton.Track(nil), metadata.Tracks[:1]...)
	metadataBody, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	body := []byte("```baton-plan-v2\n" + string(metadataBody) +
		"\n```\n\nSingle-track stale-authority fixture plan.\n")
	plan, err := baton.ParsePlan(body)
	if err != nil {
		t.Fatal(err)
	}
	return body, plan
}

func bindInitialPlannerScripts(
	t *testing.T,
	manifest *swornruntime.Manifest,
	runID string,
	planBytes []byte,
) {
	t.Helper()
	for index := range manifest.Scripts {
		script := &manifest.Scripts[index]
		if script.Responsibility == driver.PlannerProposal &&
			script.BatonAttempt == 1 {
			script.Submission = exactPlannerSubmission(
				t, runID, 1, script.Try, planBytes)
		}
	}
}

func addRevisionTwoScripts(
	t *testing.T,
	manifest *swornruntime.Manifest,
	runID string,
	revisionBytes []byte,
) {
	t.Helper()
	for try := int64(1); try <= 3; try++ {
		manifest.Scripts = append(manifest.Scripts, swornruntime.ScriptedAttempt{
			Responsibility: driver.PlannerProposal, BatonAttempt: 2,
			Epoch: 1, Try: try, Behavior: "submit",
			Submission: exactPlannerSubmission(t, runID, 2, try, revisionBytes)})
		for _, responsibility := range []driver.Responsibility{
			driver.ImplementerDesign, driver.CaptainReview,
			driver.ImplementerImplementation, driver.WorkVerification,
		} {
			manifest.Scripts = append(manifest.Scripts, swornruntime.ScriptedAttempt{
				Slice: "S1", Responsibility: responsibility, BatonAttempt: 2,
				Epoch: 1, Try: try, Behavior: "submit",
				Submission: scriptedSubmission(
					t, runID, "S1", responsibility, 2, 1, try)})
		}
		manifest.Scripts = append(manifest.Scripts, swornruntime.ScriptedAttempt{
			Responsibility: driver.AssemblyVerification, BatonAttempt: 2,
			Epoch: 1, Try: try, Behavior: "submit",
			Submission: scriptedSubmission(
				t, runID, "", driver.AssemblyVerification, 2, 1, try)})
	}
	sort.Slice(manifest.Scripts, func(i, j int) bool {
		left := fmt.Sprintf("%s/%s/%020d/%020d/%d",
			manifest.Scripts[i].Responsibility, manifest.Scripts[i].Slice,
			manifest.Scripts[i].BatonAttempt,
			manifest.Scripts[i].Epoch, manifest.Scripts[i].Try)
		right := fmt.Sprintf("%s/%s/%020d/%020d/%d",
			manifest.Scripts[j].Responsibility, manifest.Scripts[j].Slice,
			manifest.Scripts[j].BatonAttempt,
			manifest.Scripts[j].Epoch, manifest.Scripts[j].Try)
		return left < right
	})
}

func topologyManifest(
	t *testing.T,
	runID, repository, release string,
	fakeBinary, fakeDigest, s1DesignBehavior, s1VerifyBehavior string,
) ([]byte, []byte, baton.Plan) {
	t.Helper()
	body, planBytes, plan := e2eManifest(t, runID, repository, release,
		fakeBinary, fakeDigest, "verifier-model")
	var manifest swornruntime.Manifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		t.Fatal(err)
	}
	// The topology scenarios script identical failures to reach genuine
	// try-exhaustion; the identical-failure guard would park at two, so
	// the fixture raises it to the cap.
	manifest.Limits.IdenticalFailureParkAfter = driver.MaxIdenticalFailureParkAfter
	for index := range manifest.Scripts {
		script := &manifest.Scripts[index]
		if script.Slice == "S1" && script.Responsibility == driver.ImplementerDesign &&
			s1DesignBehavior != "submit" {
			script.Behavior, script.Submission = s1DesignBehavior, ""
		}
		if script.Slice == "S1" && script.Responsibility == driver.WorkVerification &&
			s1VerifyBehavior != "submit" {
			script.Behavior, script.Submission = s1VerifyBehavior, ""
		}
	}
	for _, responsibility := range []driver.Responsibility{
		driver.ImplementerDesign, driver.CaptainReview,
		driver.ImplementerImplementation, driver.WorkVerification,
	} {
		for try := int64(1); try <= 3; try++ {
			manifest.Scripts = append(manifest.Scripts, swornruntime.ScriptedAttempt{
				Slice: "S2", Responsibility: responsibility, BatonAttempt: 1,
				Epoch: 1, Try: try, Behavior: "submit",
				Submission: scriptedSubmission(t, runID, "S2", responsibility, 1, 1, try),
			})
		}
	}
	if s1VerifyBehavior != "submit" {
		for try := int64(1); try <= 3; try++ {
			manifest.Scripts = append(manifest.Scripts, swornruntime.ScriptedAttempt{
				Slice: "S1", Responsibility: driver.WorkVerification, BatonAttempt: 1,
				Epoch: 2, Try: try, Behavior: "submit",
				Submission: scriptedSubmission(t, runID, "S1", driver.WorkVerification, 1, 2, try),
			})
		}
	}
	sort.Slice(manifest.Scripts, func(i, j int) bool {
		left := fmt.Sprintf("%s/%s/%020d/%020d/%d", manifest.Scripts[i].Responsibility,
			manifest.Scripts[i].Slice, manifest.Scripts[i].BatonAttempt,
			manifest.Scripts[i].Epoch, manifest.Scripts[i].Try)
		right := fmt.Sprintf("%s/%s/%020d/%020d/%d", manifest.Scripts[j].Responsibility,
			manifest.Scripts[j].Slice, manifest.Scripts[j].BatonAttempt,
			manifest.Scripts[j].Epoch, manifest.Scripts[j].Try)
		return left < right
	})
	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	body = append(body, '\n')
	if _, err := swornruntime.ParseManifest(body); err != nil {
		t.Fatal(err)
	}
	return body, planBytes, plan
}

func parkedWork(t *testing.T, journalPath, runID string) string {
	t.Helper()
	store, err := journal.OpenReadOnly(context.Background(), journalPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	snapshot, err := store.Snapshot(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	derived := make(map[string]struct{})
	for _, command := range snapshot.Commands {
		if command.Kind == "git.seal" {
			var probe struct {
				DispatchWork string `json:"dispatch_work"`
				PreparedWork string `json:"prepared_work"`
			}
			if json.Unmarshal(command.Payload, &probe) == nil {
				if probe.DispatchWork != "" {
					derived[probe.DispatchWork] = struct{}{}
				}
				if probe.PreparedWork != "" {
					derived[probe.PreparedWork] = struct{}{}
				}
			}
		}
	}
	for _, effect := range snapshot.Effects {
		if effect.State == journal.OperationalFailed &&
			strings.HasSuffix(effect.ID, "/t3") {
			parts := strings.Split(effect.ID, "/")
			if len(parts) == 4 {
				work := "sha256:" + parts[1]
				if _, isDerived := derived[work]; !isDerived {
					return work
				}
			}
		}
	}
	t.Fatal("no exhausted work")
	return ""
}

func effectsByID(effects []journal.Effect) map[string]journal.Effect {
	result := make(map[string]journal.Effect, len(effects))
	for _, effect := range effects {
		result[effect.ID] = effect
	}
	return result
}

type claimedDriverScript struct {
	SchemaVersion string `json:"schema_version"`
	Behavior      string `json:"behavior"`
	Submission    string `json:"submission,omitempty"`
}

func seedExactClaimedDesignDispatch(
	t *testing.T,
	journalPath string,
	manifest swornruntime.Manifest,
	state baton.State,
	sliceID string,
) string {
	t.Helper()
	slice, ok := state.Slice(sliceID)
	if !ok || slice.CurrentReceipt == nil ||
		slice.Stage != "design" ||
		slice.NextRole != "implementer" {
		t.Fatalf("slice %s is not ready for design: %#v", sliceID, slice)
	}
	track, ok := state.Track(slice.Location.Track.ID)
	if !ok {
		t.Fatalf("track %s is absent", slice.Location.Track.ID)
	}
	before := e2eWorkIdentity(
		state.Plan.OID,
		state.Refs.Target.Head,
		track.Head,
		sliceID,
		slice.Stage,
		slice.NextRole,
		slice.Attempt,
		slice.CurrentReceipt.OID,
		slice.InputPins,
	)

	var script swornruntime.ScriptedAttempt
	found := 0
	for _, candidate := range manifest.Scripts {
		if candidate.Slice == sliceID &&
			candidate.Responsibility == driver.ImplementerDesign &&
			candidate.BatonAttempt == slice.Attempt &&
			candidate.Epoch == 1 &&
			candidate.Try == 1 {
			script = candidate
			found++
		}
	}
	if found != 1 {
		t.Fatalf("design scripts for %s = %d, want 1", sliceID, found)
	}
	submission, err := base64.StdEncoding.Strict().DecodeString(
		script.Submission,
	)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(claimedDriverScript{
		SchemaVersion: "sworn.fake-script/v1",
		Behavior:      script.Behavior,
		Submission:    script.Submission,
	})
	if err != nil {
		t.Fatal(err)
	}
	payload = append(payload, '\n')

	ctx := context.Background()
	store, err := journal.Open(ctx, journalPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	snapshot, err := store.Snapshot(ctx, manifest.RunID)
	if err != nil {
		t.Fatal(err)
	}
	work := e2eWorkIdentity(
		snapshot.Run.ManifestDigest,
		sliceID,
		driver.ImplementerDesign,
		slice.Attempt,
		before,
	)
	effectID := journal.AttemptEffectID(work, 1, 1)
	now := time.Now().UTC()
	owner, err := store.AcquireOwner(
		ctx,
		manifest.RunID,
		now,
		100*time.Millisecond,
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureAttempt(
		ctx,
		journal.Command{
			RunID: manifest.RunID, ReplayKey: effectID,
			Kind: "driver.dispatch", Payload: payload, CreatedAt: now,
		},
		journal.Effect{
			RunID: manifest.RunID, ID: effectID, ReplayKey: effectID,
			Kind:           "driver.dispatch",
			BeforeDigest:   e2eDigest([]byte(before)),
			ExpectedDigest: e2eDigest(submission),
			UpdatedAt:      now,
		},
		journal.EffectAttempt{WorkID: work, Epoch: 1, Try: 1},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ClaimOwned(
		ctx,
		owner,
		effectID,
		now,
		time.Minute,
	); err != nil {
		t.Fatal(err)
	}
	// Leave the owner and driver effect claimed to model a process that died
	// after invoking the driver and before recording its outcome.
	return effectID
}

func nonPlannerDriverEffects(
	t *testing.T,
	journalPath, runID string,
) map[string]struct{} {
	t.Helper()
	store, err := journal.OpenReadOnly(context.Background(), journalPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	snapshot, err := store.Snapshot(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	result := make(map[string]struct{})
	for _, effect := range snapshot.Effects {
		if effect.Kind != "driver.dispatch" {
			continue
		}
		if effect.State != journal.Succeeded {
			// A new nonterminal or failed dispatch is still new model work. The
			// target-stale fixture's planner proposal succeeds, so only its
			// decoded planner result may be excluded below.
			result[effect.ID] = struct{}{}
			continue
		}
		submission, err := driver.DecodeSubmission(effect.Result)
		if err != nil {
			t.Fatalf("decode driver effect %s: %v", effect.ID, err)
		}
		if submission.Responsibility != driver.PlannerProposal {
			result[effect.ID] = struct{}{}
		}
	}
	return result
}

type claimedActionFixture struct {
	Effect    journal.Effect
	Input     baton.AppendReceiptInput
	Plan      string
	OwnerRef  string
	OwnerHead string
}

func claimedAppendAction(
	t *testing.T,
	journalPath, runID string,
) claimedActionFixture {
	t.Helper()
	store, err := journal.OpenReadOnly(context.Background(), journalPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	snapshot, err := store.Snapshot(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	commands := make(map[string]journal.Command, len(snapshot.Commands))
	for _, command := range snapshot.Commands {
		commands[command.ReplayKey] = command
	}
	for _, effect := range snapshot.Effects {
		if effect.Kind != "baton.append_receipt" || effect.State != journal.Claimed {
			continue
		}
		command, ok := commands[effect.ReplayKey]
		if !ok {
			t.Fatal("claimed append has no command")
		}
		var persisted struct {
			Authority struct {
				Plan      string `json:"plan"`
				OwnerRef  string `json:"owner_ref"`
				OwnerHead string `json:"owner_head"`
			} `json:"authority"`
			Input json.RawMessage `json:"input"`
		}
		var input baton.AppendReceiptInput
		if json.Unmarshal(command.Payload, &persisted) != nil ||
			json.Unmarshal(persisted.Input, &input) != nil ||
			input.Slice == "" {
			t.Fatal("claimed append command is corrupt")
		}
		return claimedActionFixture{
			Effect: effect, Input: input, Plan: persisted.Authority.Plan,
			OwnerRef:  persisted.Authority.OwnerRef,
			OwnerHead: persisted.Authority.OwnerHead,
		}
	}
	t.Fatal("no claimed append action")
	return claimedActionFixture{}
}

type claimedSealFixture struct {
	Outer          journal.Effect
	Prepared       journal.Effect
	Record         []byte
	TrackRef       string
	TrackHead      string
	Candidate      string
	Plan           string
	PreparedEffect string
}

func claimedSeal(
	t *testing.T,
	journalPath, runID string,
) claimedSealFixture {
	t.Helper()
	store, err := journal.OpenReadOnly(context.Background(), journalPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	snapshot, err := store.Snapshot(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	commands := make(map[string]journal.Command, len(snapshot.Commands))
	effects := make(map[string]journal.Effect, len(snapshot.Effects))
	for _, command := range snapshot.Commands {
		commands[command.ReplayKey] = command
	}
	for _, effect := range snapshot.Effects {
		effects[effect.ID] = effect
	}
	for _, outer := range snapshot.Effects {
		if outer.Kind != "git.seal" || outer.State != journal.Claimed {
			continue
		}
		command, ok := commands[outer.ReplayKey]
		if !ok {
			t.Fatal("claimed seal has no command")
		}
		var cycle struct {
			TrackRef       string `json:"track_ref"`
			TrackHead      string `json:"track_head"`
			Plan           string `json:"plan"`
			PreparedEffect string `json:"prepared_effect"`
		}
		if json.Unmarshal(command.Payload, &cycle) != nil ||
			cycle.TrackRef == "" || cycle.TrackHead == "" ||
			cycle.Plan == "" || cycle.PreparedEffect == "" {
			t.Fatal("claimed seal command is corrupt")
		}
		prepared, ok := effects[cycle.PreparedEffect]
		if !ok || prepared.Kind != "git.seal.prepared" ||
			prepared.State != journal.Claimed {
			t.Fatalf("claimed seal prepared effect = %#v", prepared)
		}
		preparedCommand, ok := commands[cycle.PreparedEffect]
		if !ok {
			t.Fatal("claimed prepared seal has no command")
		}
		var record struct {
			Candidate string `json:"candidate"`
		}
		if json.Unmarshal(preparedCommand.Payload, &record) != nil ||
			record.Candidate == "" {
			t.Fatal("claimed prepared seal record is corrupt")
		}
		return claimedSealFixture{
			Outer: outer, Prepared: prepared,
			Record:   append([]byte(nil), preparedCommand.Payload...),
			TrackRef: cycle.TrackRef, TrackHead: cycle.TrackHead,
			Candidate: record.Candidate, Plan: cycle.Plan,
			PreparedEffect: cycle.PreparedEffect,
		}
	}
	t.Fatal("no claimed seal cycle")
	return claimedSealFixture{}
}

func assertClaimedActionTerminalizedStale(
	t *testing.T,
	journalPath, runID string,
	claimed claimedActionFixture,
) {
	t.Helper()
	store, err := journal.OpenReadOnly(context.Background(), journalPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	effect, err := store.Effect(context.Background(), runID, claimed.Effect.ID)
	if err != nil || effect.State != journal.OperationalFailed ||
		effect.ErrorCode != "stale_authority" {
		t.Fatalf("stale claimed action = %#v, err=%v", effect, err)
	}
}

func assertClaimedSealTerminalizedStale(
	t *testing.T,
	journalPath, runID string,
	claimed claimedSealFixture,
) {
	t.Helper()
	store, err := journal.OpenReadOnly(context.Background(), journalPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, id := range []string{claimed.Prepared.ID, claimed.Outer.ID} {
		effect, err := store.Effect(context.Background(), runID, id)
		if err != nil || effect.State != journal.OperationalFailed ||
			effect.ErrorCode != "stale_authority" {
			t.Fatalf("stale claimed seal %s = %#v, err=%v", id, effect, err)
		}
	}
	snapshot, err := store.Snapshot(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	for _, effect := range snapshot.Effects {
		if (effect.Kind == "git.seal" || effect.Kind == "git.seal.prepared") &&
			(effect.State == journal.Claimed || effect.State == journal.Uncertain) {
			t.Fatalf("stale seal remained nonterminal: %#v", effect)
		}
	}
}

func observeBatonState(repositoryPath, release string) (baton.State, error) {
	repository, err := gitx.Open(repositoryPath, e2eGit)
	if err != nil {
		return baton.State{}, err
	}
	return baton.ReadState(baton.UseGitRepository(repository), release, inertResolver)
}

func runRealBinaryParallelTracksParkingRetryAndPause(t *testing.T) {
	buildRoot := t.TempDir()
	fakeBinary := filepath.Join(buildRoot, "fake")
	buildBinary(t, fakeBinary, "./test/e2e/testdata/fake", "")
	fakeDigest := fileDigest(t, fakeBinary)
	swornBinary := filepath.Join(buildRoot, "sworn")
	buildBinary(t, swornBinary, "./cmd/sworn", "")
	preActionCrashBinary := filepath.Join(buildRoot, "sworn-before-action")
	buildBinary(t, preActionCrashBinary, "./cmd/sworn", hookGateLDFlags)
	preActionCrashEnvironment := map[string]string{
		"SWORN_TEST_CRASH_BEFORE_EFFECT": "baton.append_receipt",
		"SWORN_TEST_OWNER_LEASE_MILLIS":  testLeaseMillis,
	}

	t.Run("parked_lane_does_not_stop_independent_track_and_exact_retry_recovers", func(t *testing.T) {
		repository, root := newProductRepository(t), t.TempDir()
		journalPath := filepath.Join(root, "run.sqlite")
		const runID, release = "topology-park", "topology-park-release"
		body, _, plan := topologyManifest(t, runID, repository, release,
			fakeBinary, fakeDigest, "submit", "none")

		manifestPath := writeManifest(t, root, body)
		runBinary(t, swornBinary, 0, "run", "--manifest", manifestPath, "--journal", journalPath)
		authorizePlan(t, journalPath, runID, plan)
		runBinary(t, swornBinary, 0, "resume", "--run", runID,
			"--journal", journalPath, "--command", "resume-1", "--generation", "0")
		stdout, _ := runBinary(t, swornBinary, 0, "run", "--manifest", manifestPath, "--journal", journalPath)
		if !strings.Contains(stdout, "  state: parked") {
			t.Fatalf("parked status = %q", stdout)
		}
		state := readBatonState(t, repository, release)
		s1, _ := state.Slice("S1")
		s2, _ := state.Slice("S2")
		if s1.Pass != nil || s2.Pass == nil || state.Assembly.Candidate != nil {
			t.Fatalf("parking isolation: S1=%#v S2=%#v assembly=%#v", s1, s2, state.Assembly)
		}
		work := parkedWork(t, journalPath, runID)
		runBinary(t, swornBinary, 0, "retry", "--run", runID, "--journal", journalPath,
			"--command", "retry-1", "--generation", "1", "--work", work, "--epoch", "1")
		runBinary(t, swornBinary, 0, "resume", "--run", runID,
			"--journal", journalPath, "--command", "resume-2", "--generation", "2")
		stdout, _ = runBinary(t, swornBinary, 0, "run", "--manifest", manifestPath, "--journal", journalPath)
		if !strings.Contains(stdout, "  state: complete") {
			t.Fatalf("retry completion = %q", stdout)
		}
	})

	t.Run("barrier_proves_overlap_and_pause_reaches_quiescence", func(t *testing.T) {
		repository, root := newProductRepository(t), t.TempDir()
		journalPath := filepath.Join(root, "run.sqlite")
		const runID, release = "topology-pause", "topology-pause-release"
		body, _, plan := topologyManifest(t, runID, repository, release,
			fakeBinary, fakeDigest, "block", "submit")

		manifestPath := writeManifest(t, root, body)
		runBinary(t, swornBinary, 0, "run", "--manifest", manifestPath, "--journal", journalPath)
		authorizePlan(t, journalPath, runID, plan)
		// The resume hosts the drive, so the resume process itself is the
		// backgrounded driver whose barrier overlap and pause quiescence
		// the scenario observes.
		command := exec.Command(swornBinary, "resume", "--run", runID,
			"--journal", journalPath, "--command", "resume-1", "--generation", "0")
		command.Env = cleanEnvironment(nil)
		var output bytes.Buffer
		command.Stdout, command.Stderr = &output, &output
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(15 * time.Second)
		overlap := false
		for time.Now().Before(deadline) {
			store, err := journal.OpenReadOnly(context.Background(), journalPath)
			if err == nil {
				snapshot, _ := store.Snapshot(context.Background(), runID)
				_ = store.Close()
				blockClaimed := false
				for _, effect := range snapshot.Effects {
					if effect.Kind == "driver.dispatch" && effect.State == journal.Claimed {
						for _, control := range snapshot.Commands {
							blockClaimed = blockClaimed || (control.ReplayKey == effect.ReplayKey &&
								strings.Contains(string(control.Payload), `"behavior":"block"`))
						}
					}
				}
				if blockClaimed {
					state, err := observeBatonState(repository, release)
					if err != nil {
						time.Sleep(25 * time.Millisecond)
						continue
					}
					s2, _ := state.Slice("S2")
					// Any S2 receipt while the S1 barrier remains claimed
					// proves the tracks overlap; pinning the designed-only
					// moment races the scripted fake's pace on slow runners.
					if s2.CurrentReceipt != nil {
						overlap = true
						break
					}
				}
			}
			time.Sleep(25 * time.Millisecond)
		}
		if !overlap {
			_ = command.Process.Kill()
			t.Fatal("did not observe S2 designed while S1 barrier remained claimed")
		}
		runBinary(t, swornBinary, 0, "pause", "--run", runID, "--journal", journalPath,
			"--command", "pause-1", "--generation", "1")
		if err := command.Wait(); err != nil {
			t.Fatalf("resume after pause: %v\n%s", err, output.String())
		}
		status, _ := runBinary(t, swornBinary, 0, "status", "--run", runID,
			"--journal", journalPath, "--json")
		if !strings.Contains(status, `"state": "paused"`) {
			t.Fatalf("pause did not quiesce: %s", status)
		}
	})

	t.Run("scope_failure_is_bounded_without_candidate", func(t *testing.T) {
		repository, root := newProductRepository(t), t.TempDir()
		journalPath := filepath.Join(root, "run.sqlite")
		const runID, release = "topology-scope", "topology-scope-release"
		body, _, topologyPlan := topologyManifest(t, runID, repository, release,
			fakeBinary, fakeDigest, "submit", "submit")

		planBytes, plan := singleTrackPlan(t, topologyPlan)
		var manifest swornruntime.Manifest
		if err := json.Unmarshal(body, &manifest); err != nil {
			t.Fatal(err)
		}
		manifest.MaxParallelTracks = 1
		manifest.Roles.Implementer.Model = "scope-escape-model"
		bindInitialPlannerScripts(t, &manifest, runID, planBytes)
		body, _ = json.Marshal(manifest)
		manifestPath := writeManifest(t, root, append(body, '\n'))
		runBinary(t, swornBinary, 0, "run", "--manifest", manifestPath, "--journal", journalPath)
		authorizePlan(t, journalPath, runID, plan)
		runBinary(t, swornBinary, 0, "resume", "--run", runID,
			"--journal", journalPath, "--command", "resume-1", "--generation", "0")
		stdout, _ := runBinary(t, swornBinary, 0, "run", "--manifest", manifestPath, "--journal", journalPath)
		if !strings.Contains(stdout, "  state: parked") {
			t.Fatalf("scope exhaustion status = %q", stdout)
		}
		store, err := journal.OpenReadOnly(context.Background(), journalPath)
		if err != nil {
			t.Fatal(err)
		}
		snapshot, err := store.Snapshot(context.Background(), runID)
		_ = store.Close()
		commands := make(map[string]journal.Command, len(snapshot.Commands))
		effects := make(map[string]journal.Effect, len(snapshot.Effects))
		for _, command := range snapshot.Commands {
			commands[command.ReplayKey] = command
		}
		for _, effect := range snapshot.Effects {
			effects[effect.ID] = effect
		}
		outerByWork := make(map[string]map[string]journal.Effect)
		dispatchIDs := make(map[string]struct{})
		scopeFailures := 0
		admissionRefusals := 0
		prepared := false
		for _, effect := range snapshot.Effects {
			switch effect.Kind {
			case "git.seal":
				parts := strings.Split(effect.ID, "/")
				if len(parts) != 4 || parts[0] != "attempt" ||
					parts[2] != "e1" || !strings.HasPrefix(parts[3], "t") {
					t.Fatalf("outer implementation effect id = %q", effect.ID)
				}
				workID := "sha256:" + parts[1]
				if effect.BeforeDigest != workID {
					t.Fatalf(
						"outer implementation work binding = %s, want %s",
						effect.BeforeDigest, workID)
				}
				if outerByWork[workID] == nil {
					outerByWork[workID] = make(map[string]journal.Effect)
				}
				if _, exists := outerByWork[workID][parts[3]]; exists {
					t.Fatalf("duplicate outer try %s for %s", parts[3], workID)
				}
				outerByWork[workID][parts[3]] = effect
				if effect.State != journal.OperationalFailed {
					t.Fatalf("outer implementation failure = %#v", effect)
				}
				switch effect.ErrorCode {
				case "CANDIDATE_SCOPE_FAILED":
					scopeFailures++
				case "WORK_ALREADY_SUCCEEDED":
					// The first try's inner dispatch succeeded before the
					// seal refused its scope; later tries refuse at
					// admission because a succeeded work is never re-paid
					// within its epoch. No dispatch exists for these tries.
					admissionRefusals++
					continue
				default:
					t.Fatalf("outer implementation failure = %#v", effect)
				}
				command, ok := commands[effect.ReplayKey]
				if !ok {
					t.Fatalf("outer implementation command missing for %s", effect.ID)
				}
				var cycle struct {
					DispatchWork   string `json:"dispatch_work"`
					DispatchEffect string `json:"dispatch_effect"`
				}
				if json.Unmarshal(command.Payload, &cycle) != nil ||
					cycle.DispatchWork == "" ||
					cycle.DispatchEffect != journal.AttemptEffectID(
						cycle.DispatchWork, 1, 1) {
					t.Fatalf("outer implementation cycle = %s", command.Payload)
				}
				if _, duplicate := dispatchIDs[cycle.DispatchEffect]; duplicate {
					t.Fatalf("inner dispatch reused by outer cycles: %s", cycle.DispatchEffect)
				}
				dispatchIDs[cycle.DispatchEffect] = struct{}{}
				child, ok := effects[cycle.DispatchEffect]
				if !ok || child.Kind != "driver.dispatch" ||
					child.State != journal.Succeeded {
					t.Fatalf("inner implementation dispatch = %#v", child)
				}
				prefix := "attempt/" +
					strings.TrimPrefix(cycle.DispatchWork, "sha256:") + "/"
				children := 0
				for _, candidate := range snapshot.Effects {
					if strings.HasPrefix(candidate.ID, prefix) {
						children++
					}
				}
				if children != 1 {
					t.Fatalf(
						"inner dispatch attempts for %s = %d, want 1",
						effect.ID, children)
				}
			case "git.seal.prepared":
				prepared = true
			}
		}
		state := readBatonState(t, repository, release)
		for _, slice := range state.Slices {
			if slice.Candidate != nil {
				t.Fatalf("scope failure produced candidate for %s", slice.Location.Slice.ID)
			}
		}
		if len(outerByWork) != 1 {
			t.Fatalf("scope failure work identities = %d, want 1", len(outerByWork))
		}
		for workID, attempts := range outerByWork {
			if len(attempts) != 3 ||
				attempts["t1"].ID == "" ||
				attempts["t2"].ID == "" ||
				attempts["t3"].ID == "" ||
				attempts["t4"].ID != "" {
				t.Fatalf("scope failure attempts for %s = %#v", workID, attempts)
			}
		}
		if err != nil || len(dispatchIDs) != 1 || scopeFailures != 1 ||
			admissionRefusals != 2 || prepared {
			t.Fatalf(
				"scope failure evidence: works=%d dispatches=%d scope=%d refused=%d prepared=%t err=%v",
				len(outerByWork), len(dispatchIDs), scopeFailures,
				admissionRefusals, prepared, err)
		}
	})

	t.Run("revision_two_requires_new_exact_authority_without_mutation", func(t *testing.T) {
		repository, root := newProductRepository(t), t.TempDir()
		journalPath := filepath.Join(root, "run.sqlite")
		const runID, release = "topology-revision", "topology-revision-release"
		body, initialBytes, initialPlan := topologyManifest(t, runID, repository, release,
			fakeBinary, fakeDigest, "submit", "submit")

		revisionBytes, _ := revisedPlan(
			t, repository, initialBytes, initialPlan)

		var manifest swornruntime.Manifest
		if err := json.Unmarshal(body, &manifest); err != nil {
			t.Fatal(err)
		}
		for index := range manifest.Scripts {
			script := &manifest.Scripts[index]
			if script.Slice == "S1" && script.Responsibility == driver.WorkVerification {
				script.Submission = exactBlockedSubmission(t, runID, script.Try)
			}
		}
		addRevisionTwoScripts(t, &manifest, runID, revisionBytes)
		body, _ = json.Marshal(manifest)
		manifestPath := writeManifest(t, root, append(body, '\n'))
		runBinary(t, swornBinary, 0, "run", "--manifest", manifestPath, "--journal", journalPath)
		authorizePlan(t, journalPath, runID, initialPlan)
		runBinary(t, swornBinary, 0, "resume", "--run", runID,
			"--journal", journalPath, "--command", "resume-1", "--generation", "0")
		// The hosted resume carries the drive through the revision trigger
		// and the first recorded, unapproved proposal; the driven state is
		// read from status rather than performed by a redundant drive.
		stdout, _ := runBinary(t, swornBinary, 0, "status", "--run", runID,
			"--journal", journalPath, "--json")
		if !strings.Contains(stdout, "awaiting_approval") {
			t.Fatalf("revision proposal = %q", stdout)
		}
		before := readBatonState(t, repository, release)
		s1Before, _ := before.Slice("S1")
		s2Before, _ := before.Slice("S2")
		if s1Before.Outcome != "blocked" || s2Before.Pass == nil {
			t.Fatalf("revision trigger state: S1=%#v S2=%#v", s1Before, s2Before)
		}
		// A further drive without new exact authority meets the second
		// scripted proposal and is refused; a hosted resume replay would
		// swallow that refusal into run state, so the explicit run is the
		// drive that surfaces it.
		_, stderr := runBinary(t, swornBinary, 1,
			"run", "--manifest", manifestPath, "--journal", journalPath)
		if !strings.Contains(stderr, "PLAN_AUTHORITY_CONFLICT") {
			t.Fatalf("revision authority stderr = %q", stderr)
		}
		after := readBatonState(t, repository, release)
		s1After, _ := after.Slice("S1")
		s2After, _ := after.Slice("S2")
		if after.Plan.Metadata.Revision != 1 || len(after.Plan.History) != 1 ||
			s1After.Outcome != "blocked" || s2After.Pass == nil ||
			s2After.Pass.OID != s2Before.Pass.OID {
			t.Fatalf("unauthorized revision mutated state: plan=%#v S1=%#v S2=%#v",
				after.Plan.Metadata, s1After, s2After)
		}
	})

	t.Run("claimed_action_target_supersession_never_replays_old_handoff", func(t *testing.T) {
		repository, root := newProductRepository(t), t.TempDir()
		journalPath := filepath.Join(root, "run.sqlite")
		const runID, release = "topology-target-stale", "topology-target-stale-release"
		body, _, topologyPlan := topologyManifest(
			t, runID, repository, release,
			fakeBinary, fakeDigest, "submit", "submit")

		initialBytes, initialPlan := singleTrackPlan(t, topologyPlan)
		revisionBytes, _ := revisedPlan(
			t, repository, initialBytes, initialPlan)

		var manifest swornruntime.Manifest
		if err := json.Unmarshal(body, &manifest); err != nil {
			t.Fatal(err)
		}
		manifest.MaxParallelTracks = 1
		bindInitialPlannerScripts(t, &manifest, runID, initialBytes)
		addRevisionTwoScripts(t, &manifest, runID, revisionBytes)
		body, _ = json.Marshal(manifest)
		manifestPath := writeManifest(t, root, append(body, '\n'))
		runBinary(t, swornBinary, 0,
			"run", "--manifest", manifestPath, "--journal", journalPath)
		authorizePlan(t, journalPath, runID, initialPlan)
		runBinaryWithEnvironment(t, preActionCrashBinary, 86,
			preActionCrashEnvironment,
			"resume", "--run", runID, "--journal", journalPath,
			"--command", "resume-1", "--generation", "0")
		claimed := claimedAppendAction(t, journalPath, runID)
		if err := os.WriteFile(
			filepath.Join(repository, "target-moved.txt"),
			[]byte("external target move\n"),
			0o644,
		); err != nil {
			t.Fatal(err)
		}
		runGit(t, repository, "add", "--", "target-moved.txt")
		runGit(t, repository, "commit", "--quiet", "-m", "external target move")
		leaseExpiryWait()
		runBinary(t, swornBinary, 0,
			"takeover", "--run", runID, "--journal", journalPath,
			"--command", "takeover-1", "--generation", "1")
		stdout, _ := runBinary(t, swornBinary, 0,
			"run", "--manifest", manifestPath, "--journal", journalPath)
		if !strings.Contains(stdout, "  state: complete") {
			t.Fatalf("forward-target recovery = %q", stdout)
		}
		assertClaimedActionTerminalizedStale(
			t, journalPath, runID, claimed)
		state := readBatonState(t, repository, release)
		slice, ok := state.Slice(claimed.Input.Slice)
		if !ok || state.Plan.TargetStale || state.Plan.Metadata.Revision != 1 {
			t.Fatalf("target supersession state: plan=%#v slice=%#v",
				state.Plan, slice)
		}
	})

	t.Run("paused_exact_resume_replay_never_recovers_claimed_handoff", func(t *testing.T) {
		repository, root := newProductRepository(t), t.TempDir()
		journalPath := filepath.Join(root, "run.sqlite")
		const runID = "topology-paused-replay"
		const release = "topology-paused-replay-release"
		body, _, topologyPlan := topologyManifest(
			t, runID, repository, release,
			fakeBinary, fakeDigest, "submit", "submit")

		initialBytes, initialPlan := singleTrackPlan(t, topologyPlan)
		var manifest swornruntime.Manifest
		if err := json.Unmarshal(body, &manifest); err != nil {
			t.Fatal(err)
		}
		manifest.MaxParallelTracks = 1
		bindInitialPlannerScripts(t, &manifest, runID, initialBytes)
		body, _ = json.Marshal(manifest)
		manifestPath := writeManifest(t, root, append(body, '\n'))
		runBinary(t, swornBinary, 0,
			"run", "--manifest", manifestPath, "--journal", journalPath)
		authorizePlan(t, journalPath, runID, initialPlan)
		runBinaryWithEnvironment(t, preActionCrashBinary, 86,
			preActionCrashEnvironment,
			"resume", "--run", runID, "--journal", journalPath,
			"--command", "resume-1", "--generation", "0")
		claimed := claimedAppendAction(t, journalPath, runID)
		releaseBefore := runGit(
			t, repository, "rev-parse", "refs/heads/release-wt/"+release)
		runBinary(t, swornBinary, 0,
			"pause", "--run", runID, "--journal", journalPath,
			"--command", "pause-1", "--generation", "1")
		leaseExpiryWait()
		runBinary(t, swornBinary, 0,
			"resume", "--run", runID, "--journal", journalPath,
			"--command", "resume-1", "--generation", "0")
		status, _ := runBinary(
			t,
			swornBinary,
			0,
			"status",
			"--run",
			runID,
			"--journal",
			journalPath,
			"--json",
		)
		if !strings.Contains(status, `"desired_state": "paused"`) {
			t.Fatalf("old resume replay changed desired state: %s", status)
		}
		if got := runGit(
			t, repository, "rev-parse", "refs/heads/release-wt/"+release,
		); got != releaseBefore {
			t.Fatalf("paused replay moved release to %s, want %s", got, releaseBefore)
		}
		store, err := journal.OpenReadOnly(context.Background(), journalPath)
		if err != nil {
			t.Fatal(err)
		}
		snapshot, err := store.Snapshot(context.Background(), runID)
		_ = store.Close()
		if err != nil {
			t.Fatal(err)
		}
		effect := effectsByID(snapshot.Effects)[claimed.Effect.ID]
		if effect.State != journal.Claimed {
			t.Fatalf("paused replay reconciled claimed action: %#v", effect)
		}
	})

	t.Run("claimed_action_plan_supersession_never_mutates_new_plan", func(t *testing.T) {
		repository, root := newProductRepository(t), t.TempDir()
		journalPath := filepath.Join(root, "run.sqlite")
		const runID, release = "topology-plan-stale", "topology-plan-stale-release"
		body, _, topologyPlan := topologyManifest(
			t, runID, repository, release,
			fakeBinary, fakeDigest, "submit", "submit")

		initialBytes, initialPlan := singleTrackPlan(t, topologyPlan)
		revisionBytes, _ := revisedPlan(
			t, repository, initialBytes, initialPlan)

		var manifest swornruntime.Manifest
		if err := json.Unmarshal(body, &manifest); err != nil {
			t.Fatal(err)
		}
		manifest.MaxParallelTracks = 1
		bindInitialPlannerScripts(t, &manifest, runID, initialBytes)
		addRevisionTwoScripts(t, &manifest, runID, revisionBytes)
		body, _ = json.Marshal(manifest)
		manifestPath := writeManifest(t, root, append(body, '\n'))
		runBinary(t, swornBinary, 0,
			"run", "--manifest", manifestPath, "--journal", journalPath)
		authorizePlan(t, journalPath, runID, initialPlan)
		runBinaryWithEnvironment(t, preActionCrashBinary, 86,
			preActionCrashEnvironment,
			"resume", "--run", runID, "--journal", journalPath,
			"--command", "resume-1", "--generation", "0")
		claimed := claimedAppendAction(t, journalPath, runID)

		recordPlannerProposalFixture(
			t,
			journalPath,
			runID,
			revisionBytes,
			readBatonState(t, repository, release),
		)
		gitRepository, err := gitx.Open(repository, e2eGit)
		if err != nil {
			t.Fatal(err)
		}
		actions, err := baton.NewActions(
			baton.UseGitRepository(gitRepository), inertResolver, gitx.Identity{Name: "E2E Engine", Email: "engine@example.test"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := actions.RecordPlanRevision(baton.RecordPlanRevisionInput{
			PlanBytes: revisionBytes,
			Summary:   "Install externally approved superseding fixture plan.",
			Detail:    []byte("Fixture authority supersession."),
		}); err != nil {
			t.Fatal(err)
		}
		leaseExpiryWait()
		runBinary(t, swornBinary, 0,
			"takeover", "--run", runID, "--journal", journalPath,
			"--command", "takeover-1", "--generation", "1")
		stdout, _ := runBinary(t, swornBinary, 0,
			"run", "--manifest", manifestPath, "--journal", journalPath)
		if !strings.Contains(stdout, "  state: awaiting_approval") ||
			!strings.Contains(stdout, "  authority_state: authority_conflict") {
			store, _ := journal.OpenReadOnly(context.Background(), journalPath)
			snapshot, _ := store.Snapshot(context.Background(), runID)
			_ = store.Close()
			commandByReplay := make(map[string]journal.Command, len(snapshot.Commands))
			for _, command := range snapshot.Commands {
				commandByReplay[command.ReplayKey] = command
			}
			var effects []string
			for _, effect := range snapshot.Effects {
				if effect.State == journal.Claimed ||
					effect.State == journal.Uncertain ||
					effect.State == journal.OperationalFailed {
					command := commandByReplay[effect.ReplayKey]
					effects = append(effects, fmt.Sprintf(
						"%s:%s:%s:%s:%s",
						effect.Kind, effect.ID, effect.State, effect.ErrorCode,
						strings.TrimSpace(string(command.Payload))))
				}
			}
			state, _ := observeBatonState(repository, release)
			t.Fatalf("plan-supersession recovery = %q effects=%v assembly=%#v",
				stdout, effects, state.Assembly)
		}
		assertClaimedActionTerminalizedStale(
			t, journalPath, runID, claimed)
		state := readBatonState(t, repository, release)
		slice, ok := state.Slice(claimed.Input.Slice)
		if !ok || state.Plan.Metadata.Revision != 2 {
			t.Fatalf("superseding plan state: plan=%#v slice=%#v",
				state.Plan.Metadata, slice)
		}
		for _, entry := range slice.History.Entries {
			if entry.Receipt.Plan == claimed.Plan &&
				entry.Receipt.Role == claimed.Input.Role &&
				entry.Receipt.Result == claimed.Input.Result &&
				entry.Receipt.Summary == claimed.Input.Summary {
				t.Fatalf("old-plan claimed handoff replayed as %s", entry.OID)
			}
		}
	})

	t.Run("claimed_all_new_install_completes_without_replay_after_target_move", func(t *testing.T) {
		crashBinary := filepath.Join(buildRoot, "sworn-install-all-new-target-move")
		buildBinary(t, crashBinary, "./cmd/sworn", hookGateLDFlags)
		crashEnvironment := map[string]string{
			"SWORN_TEST_CRASH_AFTER_EFFECT": "baton.install",
			"SWORN_TEST_OWNER_LEASE_MILLIS": testLeaseMillis,
		}
		repository, root := newProductRepository(t), t.TempDir()
		journalPath := filepath.Join(root, "run.sqlite")
		const runID = "install-all-new-target-move"
		const release = "install-all-new-target-move-release"
		body, initialBytes, initialPlan := topologyManifest(
			t, runID, repository, release,
			fakeBinary, fakeDigest, "submit", "submit")

		revisionBytes, _ := revisedPlan(
			t, repository, initialBytes, initialPlan)

		var manifest swornruntime.Manifest
		if err := json.Unmarshal(body, &manifest); err != nil {
			t.Fatal(err)
		}
		addRevisionTwoScripts(t, &manifest, runID, revisionBytes)
		body, _ = json.Marshal(manifest)
		manifestPath := writeManifest(t, root, append(body, '\n'))
		runBinary(t, swornBinary, 0,
			"run", "--manifest", manifestPath, "--journal", journalPath)
		authorizePlan(t, journalPath, runID, initialPlan)
		runBinaryWithEnvironment(t, crashBinary, 86, crashEnvironment,
			"resume", "--run", runID, "--journal", journalPath,
			"--command", "resume-1", "--generation", "0")
		store, err := journal.OpenReadOnly(
			context.Background(), journalPath)
		if err != nil {
			t.Fatal(err)
		}
		snapshot, err := store.Snapshot(context.Background(), runID)
		_ = store.Close()
		if err != nil {
			t.Fatal(err)
		}
		var install journal.Effect
		for _, effect := range snapshot.Effects {
			if effect.Kind == "baton.install" &&
				effect.State == journal.Claimed {
				install = effect
			}
		}
		if install.ID == "" {
			t.Fatal("install crash left no claimed install effect")
		}
		if err := os.WriteFile(
			filepath.Join(repository, "install-target-moved.txt"),
			[]byte("external target move\n"),
			0o644,
		); err != nil {
			t.Fatal(err)
		}
		runGit(t, repository, "add", "--", "install-target-moved.txt")
		runGit(t, repository, "commit", "--quiet", "-m", "move target after install")
		leaseExpiryWait()
		runBinary(
			t, swornBinary, 0,
			"takeover", "--run", runID, "--journal", journalPath,
			"--command", "takeover-1", "--generation", "1")
		stdout, _ := runBinary(
			t, swornBinary, 0,
			"run", "--manifest", manifestPath, "--journal", journalPath)
		if !strings.Contains(stdout, "  state: complete") {
			t.Fatalf("all-new install recovery = %q", stdout)
		}
		store, err = journal.OpenReadOnly(context.Background(), journalPath)
		if err != nil {
			t.Fatal(err)
		}
		recovered, err := store.Effect(
			context.Background(), runID, install.ID)
		snapshot, snapshotErr := store.Snapshot(
			context.Background(), runID)
		_ = store.Close()
		if err != nil || snapshotErr != nil ||
			recovered.State != journal.Succeeded {
			t.Fatalf(
				"recovered install = %#v, effectErr=%v snapshotErr=%v",
				recovered, err, snapshotErr)
		}
		for _, effect := range snapshot.Effects {
			if effect.Kind == "baton.install" &&
				effect.ID != install.ID {
				t.Fatalf("all-new recovery replayed install as %#v", effect)
			}
			if effect.State == journal.Uncertain {
				t.Fatalf("all-new recovery became uncertain: %#v", effect)
			}
		}
		state := readBatonState(t, repository, release)
		if state.Plan.TargetStale ||
			state.Plan.Metadata.Revision != 1 {
			t.Fatalf("post-recovery plan = %#v", state.Plan)
		}
	})

	sealCrashCuts := []struct {
		name        string
		binary      string
		environment map[string]string
	}{
		{name: "git.seal.prepared"},
		{name: "git.seal"},
	}
	for index := range sealCrashCuts {
		cut := &sealCrashCuts[index]
		cut.binary = filepath.Join(
			buildRoot, "sworn-stale-"+strings.ReplaceAll(cut.name, ".", "-"))
		buildBinary(t, cut.binary, "./cmd/sworn", hookGateLDFlags)
		cut.environment = map[string]string{
			"SWORN_TEST_CRASH_AFTER_EFFECT": cut.name,
			"SWORN_TEST_OWNER_LEASE_MILLIS": testLeaseMillis,
		}
	}
	for _, authorityKind := range []string{"target", "plan"} {
		for cutIndex, crash := range sealCrashCuts {
			authorityKind, crash := authorityKind, crash
			if authorityKind == "plan" && crash.name == "git.seal" {
				// Baton refuses a plan revision while an unreceipted candidate
				// owns the track, so this all-new plan supersession cannot be
				// produced through an admitted authority action. The target
				// case below covers all-new rollback; the prepared case covers
				// the reachable all-old plan supersession.
				continue
			}
			t.Run(
				"claimed_seal_"+strings.ReplaceAll(crash.name, ".", "_")+
					"_"+authorityKind+"_authority_change_recovers_safely",
				func(t *testing.T) {
					repository, root := newProductRepository(t), t.TempDir()
					journalPath := filepath.Join(root, "run.sqlite")
					runID := fmt.Sprintf(
						"seal-stale-%s-%d", authorityKind, cutIndex)
					release := runID + "-release"
					body, _, topologyPlan := topologyManifest(
						t, runID, repository, release,
						fakeBinary, fakeDigest, "submit", "submit")

					initialBytes, initialPlan := singleTrackPlan(t, topologyPlan)
					revisionBytes, _ := revisedPlan(
						t, repository, initialBytes, initialPlan)

					var manifest swornruntime.Manifest
					if err := json.Unmarshal(body, &manifest); err != nil {
						t.Fatal(err)
					}
					manifest.MaxParallelTracks = 1
					bindInitialPlannerScripts(t, &manifest, runID, initialBytes)
					addRevisionTwoScripts(t, &manifest, runID, revisionBytes)
					body, _ = json.Marshal(manifest)
					manifestPath := writeManifest(t, root, append(body, '\n'))
					runBinary(t, swornBinary, 0,
						"run", "--manifest", manifestPath, "--journal", journalPath)
					authorizePlan(t, journalPath, runID, initialPlan)
					runBinaryWithEnvironment(t, crash.binary, 86,
						crash.environment,
						"resume", "--run", runID, "--journal", journalPath,
						"--command", "resume-1", "--generation", "0")
					claimed := claimedSeal(t, journalPath, runID)
					trackAtCrash := runGit(
						t, repository, "rev-parse", claimed.TrackRef)
					wantAtCrash := claimed.TrackHead
					if crash.name == "git.seal" {
						wantAtCrash = claimed.Candidate
					}
					if trackAtCrash != wantAtCrash {
						t.Fatalf(
							"%s crash track = %s, want %s",
							crash.name, trackAtCrash, wantAtCrash)
					}

					switch authorityKind {
					case "target":
						if err := os.WriteFile(
							filepath.Join(repository, "target-moved.txt"),
							[]byte("external target move\n"),
							0o644,
						); err != nil {
							t.Fatal(err)
						}
						runGit(t, repository, "add", "--", "target-moved.txt")
						runGit(t, repository, "commit", "--quiet",
							"-m", "external target move")
					case "plan":
						recordPlannerProposalFixture(
							t,
							journalPath,
							runID,
							revisionBytes,
							readBatonState(t, repository, release),
						)
						gitRepository, err := gitx.Open(repository, e2eGit)
						if err != nil {
							t.Fatal(err)
						}
						actions, err := baton.NewActions(
							baton.UseGitRepository(gitRepository), inertResolver, gitx.Identity{Name: "E2E Engine", Email: "engine@example.test"})
						if err != nil {
							t.Fatal(err)
						}
						if _, err := actions.RecordPlanRevision(
							baton.RecordPlanRevisionInput{
								PlanBytes: revisionBytes,
								Summary:   "Install externally approved superseding fixture plan.",
								Detail:    []byte("Fixture seal authority supersession."),
							},
						); err != nil {
							t.Fatal(err)
						}
					default:
						t.Fatal("unknown authority fixture")
					}

					leaseExpiryWait()
					runBinary(
						t, swornBinary, 0,
						"takeover", "--run", runID, "--journal", journalPath,
						"--command", "takeover-1", "--generation", "1")
					stdout, _ := runBinary(
						t, swornBinary, 0,
						"run", "--manifest", manifestPath, "--journal", journalPath)
					if authorityKind == "target" &&
						!strings.Contains(stdout, "  state: complete") {
						t.Fatalf("forward-target seal recovery = %q", stdout)
					}
					if authorityKind == "plan" &&
						(!strings.Contains(stdout, "  state: awaiting_approval") ||
							!strings.Contains(stdout, "  authority_state: authority_conflict")) {
						t.Fatalf("plan-stale seal recovery = %q", stdout)
					}
					assertClaimedSealTerminalizedStale(
						t, journalPath, runID, claimed)
					state := readBatonState(t, repository, release)
					if authorityKind == "target" && state.Plan.TargetStale {
						t.Fatal("forward target move staled the installed plan")
					}
					if authorityKind == "plan" &&
						state.Plan.Metadata.Revision != 2 {
						t.Fatalf(
							"plan supersession revision = %d, want 2",
							state.Plan.Metadata.Revision)
					}
					slice, ok := state.Slice("S1")
					if !ok {
						t.Fatal("S1 missing after seal recovery")
					}
					entries := append(
						[]baton.ReceiptEntry(nil), slice.History.Entries...)
					if slice.Candidate != nil {
						entries = append(entries, *slice.Candidate)
					}
					for _, entry := range entries {
						if authorityKind != "plan" {
							continue
						}
						if entry.Receipt.Plan == claimed.Plan &&
							entry.Receipt.Role == "implementer" &&
							entry.Receipt.Result == "candidate" &&
							entry.Receipt.Candidate != nil &&
							*entry.Receipt.Candidate == claimed.Candidate {
							t.Fatalf(
								"stale candidate receipt survived as %s",
								entry.OID)
						}
					}
				},
			)
		}
	}

	t.Run("claimed_all_new_seal_third_track_state_is_uncertain_without_mutation", func(t *testing.T) {
		repository, root := newProductRepository(t), t.TempDir()
		journalPath := filepath.Join(root, "run.sqlite")
		const runID = "seal-third-state"
		const release = "seal-third-state-release"
		body, _, topologyPlan := topologyManifest(
			t, runID, repository, release,
			fakeBinary, fakeDigest, "submit", "submit")

		initialBytes, initialPlan := singleTrackPlan(t, topologyPlan)
		var manifest swornruntime.Manifest
		if err := json.Unmarshal(body, &manifest); err != nil {
			t.Fatal(err)
		}
		manifest.MaxParallelTracks = 1
		bindInitialPlannerScripts(t, &manifest, runID, initialBytes)
		body, _ = json.Marshal(manifest)
		manifestPath := writeManifest(t, root, append(body, '\n'))
		runBinary(t, swornBinary, 0,
			"run", "--manifest", manifestPath, "--journal", journalPath)
		authorizePlan(t, journalPath, runID, initialPlan)
		runBinaryWithEnvironment(t, sealCrashCuts[1].binary, 86,
			sealCrashCuts[1].environment,
			"resume", "--run", runID, "--journal", journalPath,
			"--command", "resume-1", "--generation", "0")
		claimed := claimedSeal(t, journalPath, runID)
		if got := runGit(
			t, repository, "rev-parse", claimed.TrackRef,
		); got != claimed.Candidate {
			t.Fatalf("all-new crash track = %s, want %s", got, claimed.Candidate)
		}
		beforeStore, err := journal.OpenReadOnly(
			context.Background(), journalPath)
		if err != nil {
			t.Fatal(err)
		}
		beforeSnapshot, err := beforeStore.Snapshot(
			context.Background(), runID)
		_ = beforeStore.Close()
		if err != nil {
			t.Fatal(err)
		}
		dispatchesBefore := 0
		for _, effect := range beforeSnapshot.Effects {
			if effect.Kind == "driver.dispatch" {
				dispatchesBefore++
			}
		}
		targetBefore := runGit(t, repository, "rev-parse", "main")
		thirdWorktree := filepath.Join(root, "third-track-worktree")
		runGit(
			t,
			repository,
			"worktree",
			"add",
			"--quiet",
			"--detach",
			thirdWorktree,
			claimed.Candidate,
		)
		t.Cleanup(func() {
			runGit(
				t,
				repository,
				"worktree",
				"remove",
				"--force",
				thirdWorktree,
			)
		})
		if err := os.WriteFile(
			filepath.Join(thirdWorktree, "third-state.txt"),
			[]byte("external third state\n"),
			0o644,
		); err != nil {
			t.Fatal(err)
		}
		runGit(t, thirdWorktree, "add", "--", "third-state.txt")
		runGit(
			t,
			thirdWorktree,
			"commit",
			"--quiet",
			"-m",
			"external third track state",
		)
		third := runGit(t, thirdWorktree, "rev-parse", "HEAD")
		if targetAfter := runGit(
			t, repository, "rev-parse", "main",
		); targetAfter != targetBefore {
			t.Fatalf(
				"detached third track commit moved target to %s, want %s",
				targetAfter,
				targetBefore,
			)
		}
		runGit(
			t,
			repository,
			"update-ref",
			claimed.TrackRef,
			third,
			claimed.Candidate,
		)
		leaseExpiryWait()
		runBinary(
			t, swornBinary, 0,
			"takeover", "--run", runID, "--journal", journalPath,
			"--command", "takeover-1", "--generation", "1")
		// The takeover returns once the ownership transition is durable; the
		// drive that meets the uncertain third state is the explicit run.
		_, stderr := runBinary(
			t, swornBinary, 1,
			"run", "--manifest", manifestPath, "--journal", journalPath)
		if !strings.Contains(stderr, "RECOVERY_UNCERTAIN") {
			t.Fatalf("third-state recovery stderr = %q", stderr)
		}
		if got := runGit(
			t, repository, "rev-parse", claimed.TrackRef,
		); got != third {
			t.Fatalf("third-state recovery mutated track to %s, want %s", got, third)
		}
		if got := runGit(
			t, repository, "rev-parse", "main",
		); got != targetBefore {
			t.Fatalf(
				"third-state recovery mutated target to %s, want %s",
				got,
				targetBefore,
			)
		}
		store, err := journal.OpenReadOnly(context.Background(), journalPath)
		if err != nil {
			t.Fatal(err)
		}
		snapshot, err := store.Snapshot(context.Background(), runID)
		_ = store.Close()
		if err != nil {
			t.Fatal(err)
		}
		effects := effectsByID(snapshot.Effects)
		if effects[claimed.Prepared.ID].State != journal.Uncertain ||
			effects[claimed.Outer.ID].State != journal.Uncertain {
			t.Fatalf(
				"third-state effects: prepared=%#v outer=%#v",
				effects[claimed.Prepared.ID], effects[claimed.Outer.ID])
		}
		dispatchesAfter := 0
		for _, effect := range snapshot.Effects {
			if effect.Kind == "driver.dispatch" {
				dispatchesAfter++
			}
		}
		if dispatchesAfter != dispatchesBefore {
			t.Fatalf(
				"third-state recovery dispatched work: before=%d after=%d",
				dispatchesBefore, dispatchesAfter)
		}
		runGit(
			t,
			repository,
			"update-ref",
			claimed.TrackRef,
			claimed.Candidate,
			third,
		)
		runBinary(
			t, swornBinary, 0,
			"takeover", "--run", runID, "--journal", journalPath,
			"--command", "takeover-1", "--generation", "1")
		stdout, _ := runBinary(
			t, swornBinary, 0,
			"run", "--manifest", manifestPath, "--journal", journalPath)
		if !strings.Contains(stdout, "  state: complete") {
			t.Fatalf("restored all-new recovery status = %q", stdout)
		}
		store, err = journal.OpenReadOnly(
			context.Background(), journalPath)
		if err != nil {
			t.Fatal(err)
		}
		snapshot, err = store.Snapshot(context.Background(), runID)
		_ = store.Close()
		if err != nil {
			t.Fatal(err)
		}
		effects = effectsByID(snapshot.Effects)
		for _, id := range []string{
			claimed.Prepared.ID,
			claimed.Outer.ID,
		} {
			if effects[id].State != journal.Succeeded {
				t.Fatalf(
					"restored all-new effect %s = %#v",
					id,
					effects[id],
				)
			}
		}
		state, err := observeBatonState(repository, release)
		if err != nil {
			t.Fatal(err)
		}
		if state.Assembly.Outcome != "merged" {
			t.Fatalf(
				"restored all-new assembly outcome = %s",
				state.Assembly.Outcome,
			)
		}
	})

	for _, preparedCase := range []struct {
		name       string
		crash      string
		parentMode string
	}{
		{name: "all_old_pending_parent", crash: "git.seal.prepared", parentMode: "pending"},
		{name: "all_new_pending_parent", crash: "git.seal", parentMode: "pending"},
		{name: "all_old_failed_parent", crash: "git.seal.prepared", parentMode: "failed"},
		{name: "all_new_failed_parent", crash: "git.seal", parentMode: "failed"},
		{name: "all_old_succeeded_parent", crash: "git.seal.prepared", parentMode: "succeeded"},
		{name: "all_new_succeeded_parent", crash: "git.seal", parentMode: "succeeded"},
	} {
		preparedCase := preparedCase
		t.Run("claimed_prepared_child_"+preparedCase.name, func(t *testing.T) {
			repository, root := newProductRepository(t), t.TempDir()
			journalPath := filepath.Join(root, "run.sqlite")
			runID := "prepared-" + strings.ReplaceAll(preparedCase.name, "_", "-")
			release := runID + "-release"
			body, _, topologyPlan := topologyManifest(
				t, runID, repository, release,
				fakeBinary, fakeDigest, "submit", "submit")

			initialBytes, initialPlan := singleTrackPlan(t, topologyPlan)
			var manifest swornruntime.Manifest
			if err := json.Unmarshal(body, &manifest); err != nil {
				t.Fatal(err)
			}
			manifest.MaxParallelTracks = 1
			bindInitialPlannerScripts(t, &manifest, runID, initialBytes)
			body, _ = json.Marshal(manifest)
			manifestPath := writeManifest(t, root, append(body, '\n'))
			runBinary(t, swornBinary, 0,
				"run", "--manifest", manifestPath, "--journal", journalPath)
			authorizePlan(t, journalPath, runID, initialPlan)
			crashCut := &sealCrashCuts[0]
			if preparedCase.crash == "git.seal" {
				crashCut = &sealCrashCuts[1]
			}
			runBinaryWithEnvironment(t, crashCut.binary, 86,
				crashCut.environment,
				"resume", "--run", runID, "--journal", journalPath,
				"--command", "resume-1", "--generation", "0")
			claimed := claimedSeal(t, journalPath, runID)
			trackBefore := runGit(
				t, repository, "rev-parse", claimed.TrackRef)
			targetBefore := runGit(t, repository, "rev-parse", "main")
			store, err := journal.Open(context.Background(), journalPath)
			if err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC()
			if preparedCase.parentMode == "pending" {
				err = store.Reconcile(
					context.Background(),
					journal.Completion{
						RunID: runID, EffectID: claimed.Outer.ID,
						Token:     claimed.Outer.CurrentClaim,
						EventKind: "fixture_parent_pending", At: now,
					},
					journal.RecoveryAllOld,
				)
			} else if preparedCase.parentMode == "failed" {
				err = store.Complete(context.Background(), journal.Completion{
					RunID: runID, EffectID: claimed.Outer.ID,
					Token:     claimed.Outer.CurrentClaim,
					State:     journal.OperationalFailed,
					ErrorCode: "fixture_parent_failed",
					EventKind: "fixture_parent_failed", At: now,
				})
			} else {
				err = store.Complete(context.Background(), journal.Completion{
					RunID: runID, EffectID: claimed.Outer.ID,
					Token: claimed.Outer.CurrentClaim,
					State: journal.Succeeded, Result: claimed.Record,
					EventKind: "fixture_parent_succeeded", At: now,
				})
			}
			_ = store.Close()
			if err != nil {
				t.Fatal(err)
			}
			leaseExpiryWait()
			runBinary(
				t, swornBinary, 0,
				"takeover", "--run", runID, "--journal", journalPath,
				"--command", "takeover-1", "--generation", "1")
			if preparedCase.parentMode == "succeeded" {
				// The takeover returns once durable; the drive that
				// meets the impossible succeeded parent is the
				// explicit run of the same journal.
				_, stderr := runBinary(
					t, swornBinary, 1,
					"run", "--manifest", manifestPath, "--journal", journalPath)
				if !strings.Contains(stderr, "CORRUPT_JOURNAL") {
					t.Fatalf(
						"impossible succeeded parent stderr = %q",
						stderr)
				}
				if got := runGit(
					t, repository, "rev-parse", claimed.TrackRef,
				); got != trackBefore {
					t.Fatalf(
						"impossible succeeded parent moved track to %s, want %s",
						got, trackBefore)
				}
				if got := runGit(
					t, repository, "rev-parse", "main",
				); got != targetBefore {
					t.Fatalf(
						"impossible succeeded parent moved target to %s, want %s",
						got, targetBefore)
				}
				return
			}
			if preparedCase.parentMode == "failed" {
				stdout, _ := runBinary(
					t, swornBinary, 0,
					"run", "--manifest", manifestPath, "--journal", journalPath)
				if !strings.Contains(stdout, "  state: parked") {
					t.Fatalf("failed parent recovery = %q", stdout)
				}
				if got := runGit(t, repository, "rev-parse", claimed.TrackRef); got != claimed.TrackHead && got != claimed.Candidate {
					t.Fatalf("failed parent left invalid track %s", got)
				}
				if got := runGit(t, repository, "rev-parse", "main"); got != targetBefore {
					t.Fatalf("failed parent moved target to %s, want %s", got, targetBefore)
				}
				return
			}
			stdout, _ := runBinary(
				t, swornBinary, 0,
				"run", "--manifest", manifestPath, "--journal", journalPath)
			if !strings.Contains(stdout, "  state: complete") {
				t.Fatalf("prepared child recovery = %q", stdout)
			}
			store, err = journal.OpenReadOnly(
				context.Background(), journalPath)
			if err != nil {
				t.Fatal(err)
			}
			snapshot, err := store.Snapshot(
				context.Background(), runID)
			_ = store.Close()
			if err != nil {
				t.Fatal(err)
			}
			effects := effectsByID(snapshot.Effects)
			outer := effects[claimed.Outer.ID]
			child := effects[claimed.Prepared.ID]
			parts := strings.Split(claimed.Outer.ID, "/")
			if len(parts) != 4 {
				t.Fatalf("outer effect ID = %q", claimed.Outer.ID)
			}
			work := "sha256:" + parts[1]
			t2, hasT2 := effects[journal.AttemptEffectID(work, 1, 2)]
			if preparedCase.parentMode == "pending" {
				if outer.State != journal.Succeeded ||
					child.State != journal.Succeeded || hasT2 {
					t.Fatalf(
						"pending parent recovery: outer=%#v child=%#v t2=%#v/%t",
						outer, child, t2, hasT2)
				}
			} else if outer.State != journal.OperationalFailed ||
				outer.ErrorCode != "fixture_parent_failed" ||
				child.State != journal.OperationalFailed ||
				child.ErrorCode != "orphaned_parent" ||
				!hasT2 || t2.State != journal.Succeeded {
				t.Fatalf(
					"failed parent recovery: outer=%#v child=%#v t2=%#v/%t",
					outer, child, t2, hasT2)
			}
		})
	}

	for index, crash := range []struct {
		cut     string
		claimed string
		before  bool
	}{
		{cut: "baton.install", claimed: "baton.install"},
		{cut: "baton.append_receipt", claimed: "baton.append_receipt"},
		{cut: "git.seal", claimed: "git.seal.prepared"},
		{cut: "git.seal.prepared", claimed: "git.seal.prepared"},
		{cut: "implementation.handoff", claimed: "git.seal"},
		{cut: "baton.prepare_assembly", claimed: "baton.prepare_assembly"},
		{cut: "baton.assembly_verdict", claimed: "baton.assembly_verdict"},
		{cut: "baton.merge", claimed: "baton.merge"},
		{
			cut: "baton.prepare_assembly", claimed: "baton.prepare_assembly",
			before: true,
		},
		{cut: "baton.merge", claimed: "baton.merge", before: true},
	} {
		cut := crash.cut
		name := "crash_cut_"
		hook := "testCrashAfterEffect"
		if crash.before {
			name = "crash_before_"
			hook = "testCrashBeforeEffect"
		}
		t.Run(name+strings.ReplaceAll(cut, ".", "_"), func(t *testing.T) {
			crashBinary := filepath.Join(
				buildRoot,
				"sworn-"+strings.ReplaceAll(name+cut, ".", "-"),
			)
			buildBinary(t, crashBinary, "./cmd/sworn", hookGateLDFlags)
			crashEnvironment := map[string]string{
				crashHookEnvironmentName(t, hook): cut,
				"SWORN_TEST_OWNER_LEASE_MILLIS":   testLeaseMillis,
			}
			repository, root := newProductRepository(t), t.TempDir()
			journalPath := filepath.Join(root, "run.sqlite")
			runID := fmt.Sprintf("topology-crash-%d", index)
			release := runID + "-release"
			body, _, plan := topologyManifest(t, runID, repository, release,
				fakeBinary, fakeDigest, "submit", "submit")

			var manifest swornruntime.Manifest
			if err := json.Unmarshal(body, &manifest); err != nil {
				t.Fatal(err)
			}
			manifest.MaxParallelTracks = 1
			body, _ = json.Marshal(manifest)
			manifestPath := writeManifest(t, root, append(body, '\n'))
			runBinary(t, swornBinary, 0, "run", "--manifest", manifestPath, "--journal", journalPath)
			authorizePlan(t, journalPath, runID, plan)
			runBinaryWithEnvironment(t, crashBinary, 86, crashEnvironment,
				"resume", "--run", runID, "--journal", journalPath,
				"--command", "resume-1", "--generation", "0")
			store, err := journal.OpenReadOnly(context.Background(), journalPath)
			if err != nil {
				t.Fatal(err)
			}
			snapshot, err := store.Snapshot(context.Background(), runID)
			_ = store.Close()
			var claimedIDs []string
			for _, effect := range snapshot.Effects {
				if effect.Kind == crash.claimed && effect.State == journal.Claimed {
					claimedIDs = append(claimedIDs, effect.ID)
				}
			}
			if err != nil || len(claimedIDs) != 1 {
				t.Fatalf(
					"%s claimed crash cuts = %v, want exactly one: %v",
					cut, claimedIDs, err)
			}
			leaseExpiryWait()
			runBinary(t, swornBinary, 0, "takeover", "--run", runID,
				"--journal", journalPath, "--command", "takeover-1", "--generation", "1")
			stdout, _ := runBinary(t, swornBinary, 0, "run", "--manifest", manifestPath, "--journal", journalPath)
			if !strings.Contains(stdout, "  state: complete") {
				store, _ := journal.OpenReadOnly(context.Background(), journalPath)
				snapshot, _ := store.Snapshot(context.Background(), runID)
				_ = store.Close()
				var nonterminal []string
				for _, effect := range snapshot.Effects {
					if effect.State == journal.Claimed || effect.State == journal.Uncertain {
						nonterminal = append(nonterminal,
							fmt.Sprintf("%s:%s:%s", effect.Kind, effect.ID, effect.State))
					}
				}
				observed, _ := observeBatonState(repository, release)
				if cut == "implementation.handoff" &&
					strings.Contains(stdout, "  state: parked") &&
					len(nonterminal) == 0 {
					return
				}
				t.Fatalf("%s recovery = %q; outcome=%s nonterminal=%v",
					cut, stdout, observed.Assembly.Outcome, nonterminal)
			}
			state := readBatonState(t, repository, release)
			if state.Assembly.Outcome != "merged" ||
				runGit(t, repository, "rev-parse", "main") != state.Assembly.ResultCommit {
				t.Fatalf("%s recovery did not preserve exact target identity", cut)
			}
			store, err = journal.OpenReadOnly(context.Background(), journalPath)
			if err != nil {
				t.Fatal(err)
			}
			snapshot, err = store.Snapshot(context.Background(), runID)
			_ = store.Close()
			if err != nil {
				t.Fatal(err)
			}
			for _, effect := range snapshot.Effects {
				if effect.State == journal.Claimed || effect.State == journal.Uncertain {
					t.Fatalf("%s left nonterminal effect: %#v", cut, effect)
				}
			}
			if cut == "implementation.handoff" {
				parts := strings.Split(claimedIDs[0], "/")
				if len(parts) != 4 || parts[0] != "attempt" ||
					parts[2] != "e1" || parts[3] != "t1" {
					t.Fatalf("crashed implementation effect = %q", claimedIDs[0])
				}
				workID := "sha256:" + parts[1]
				t1 := effectsByID(snapshot.Effects)[journal.AttemptEffectID(workID, 1, 1)]
				t2 := effectsByID(snapshot.Effects)[journal.AttemptEffectID(workID, 1, 2)]
				t3, hasT3 := effectsByID(snapshot.Effects)[journal.AttemptEffectID(workID, 1, 3)]
				t4, hasT4 := effectsByID(snapshot.Effects)[journal.AttemptEffectID(workID, 1, 4)]
				if t1.State != journal.OperationalFailed ||
					t1.ErrorCode != "implementation_interrupted" ||
					t2.State != journal.Succeeded || hasT3 || hasT4 {
					t.Fatalf(
						"implementation retry sequence: t1=%#v t2=%#v t3=%#v/%t t4=%#v/%t",
						t1, t2, t3, hasT3, t4, hasT4)
				}
				commands := make(map[string]journal.Command, len(snapshot.Commands))
				for _, command := range snapshot.Commands {
					commands[command.ReplayKey] = command
				}
				inner := make(map[string]struct{})
				for _, outer := range []journal.Effect{t1, t2} {
					var cycle struct {
						DispatchWork   string `json:"dispatch_work"`
						DispatchEffect string `json:"dispatch_effect"`
					}
					if json.Unmarshal(
						commands[outer.ReplayKey].Payload, &cycle,
					) != nil ||
						cycle.DispatchEffect != journal.AttemptEffectID(
							cycle.DispatchWork, 1, 1) {
						t.Fatalf("implementation cycle command = %#v", commands[outer.ReplayKey])
					}
					if _, duplicate := inner[cycle.DispatchEffect]; duplicate {
						t.Fatalf("implementation cycles reused %s", cycle.DispatchEffect)
					}
					inner[cycle.DispatchEffect] = struct{}{}
					child := effectsByID(snapshot.Effects)[cycle.DispatchEffect]
					if child.Kind != "driver.dispatch" ||
						child.State != journal.Succeeded {
						t.Fatalf("implementation child = %#v", child)
					}
				}
				if len(inner) != 2 {
					t.Fatalf("implementation inner dispatches = %d, want 2", len(inner))
				}
			}
		})
	}

	t.Run("takeover_fences_parallel_tracks_behind_exact_driver_ambiguity", func(t *testing.T) {
		repository, root := newProductRepository(t), t.TempDir()
		journalPath := filepath.Join(root, "run.sqlite")
		const runID = "topology-driver-fence"
		const release = "topology-driver-fence-release"
		body, planBytes, plan := topologyManifest(
			t,
			runID,
			repository,
			release,

			fakeBinary,
			fakeDigest,
			"submit",
			"submit")

		var manifest swornruntime.Manifest
		if err := json.Unmarshal(body, &manifest); err != nil {
			t.Fatal(err)
		}
		if manifest.MaxParallelTracks < 2 {
			t.Fatalf(
				"parallel fixture capacity = %d, want at least 2",
				manifest.MaxParallelTracks,
			)
		}
		manifestPath := writeManifest(t, root, body)
		runBinary(
			t,
			swornBinary,
			0,
			"run",
			"--manifest",
			manifestPath,
			"--journal",
			journalPath,
		)
		authorizePlan(t, journalPath, runID, plan)

		product, err := gitx.Open(repository, e2eGit)
		if err != nil {
			t.Fatal(err)
		}
		actions, err := baton.NewActions(
			baton.UseGitRepository(product),
			inertResolver,
			gitx.Identity{Name: "E2E Engine", Email: "engine@example.test"},
		)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := actions.RecordPlanRevision(
			baton.RecordPlanRevisionInput{
				PlanBytes: planBytes,
				Summary:   "Install exact two-track recovery fixture.",
				Detail: []byte(
					"Both independent tracks are ready before takeover.",
				),
			},
		); err != nil {
			t.Fatal(err)
		}
		state := readBatonState(t, repository, release)
		for _, sliceID := range []string{"S1", "S2"} {
			slice, ok := state.Slice(sliceID)
			if !ok ||
				slice.Status != "ready" ||
				slice.Stage != "design" ||
				slice.NextRole != "implementer" {
				t.Fatalf("%s initial state = %#v", sliceID, slice)
			}
		}
		claimedID := seedExactClaimedDesignDispatch(
			t,
			journalPath,
			manifest,
			state,
			"S1",
		)

		store, err := journal.OpenReadOnly(
			context.Background(),
			journalPath,
		)
		if err != nil {
			t.Fatal(err)
		}
		beforeTakeover, err := store.Snapshot(
			context.Background(),
			runID,
		)
		_ = store.Close()
		if err != nil {
			t.Fatal(err)
		}
		driverEffects := make(map[string]struct{})
		for _, effect := range beforeTakeover.Effects {
			if effect.Kind == "driver.dispatch" {
				driverEffects[effect.ID] = struct{}{}
			}
		}

		time.Sleep(150 * time.Millisecond)
		runBinary(
			t,
			swornBinary,
			0,
			"takeover",
			"--run",
			runID,
			"--journal",
			journalPath,
			"--command",
			"takeover-1",
			"--generation",
			"0",
		)

		store, err = journal.OpenReadOnly(
			context.Background(),
			journalPath,
		)
		if err != nil {
			t.Fatal(err)
		}
		afterTakeover, err := store.Snapshot(
			context.Background(),
			runID,
		)
		_ = store.Close()
		if err != nil {
			t.Fatal(err)
		}
		claimed := effectsByID(afterTakeover.Effects)[claimedID]
		if claimed.State != journal.Claimed &&
			claimed.State != journal.Uncertain {
			t.Fatalf("preserved driver ambiguity = %#v", claimed)
		}
		for _, effect := range afterTakeover.Effects {
			if effect.Kind != "driver.dispatch" {
				continue
			}
			if _, existed := driverEffects[effect.ID]; !existed {
				t.Fatalf(
					"takeover scheduled new parallel driver effect: %#v",
					effect,
				)
			}
		}
		afterState := readBatonState(t, repository, release)
		s2, _ := afterState.Slice("S2")
		if s2 == nil ||
			s2.Stage != "design" ||
			s2.NextRole != "implementer" {
			t.Fatalf(
				"parallel S2 advanced beside unresolved S1: %#v",
				s2,
			)
		}
	})

	t.Run("driver_crash_is_quiescent_uncertain_and_never_retried", func(t *testing.T) {
		crashBinary := filepath.Join(buildRoot, "sworn-driver-crash")
		buildBinary(t, crashBinary, "./cmd/sworn", hookGateLDFlags)
		crashEnvironment := map[string]string{
			"SWORN_TEST_CRASH_AFTER_EFFECT": "driver.dispatch",
			"SWORN_TEST_OWNER_LEASE_MILLIS": testLeaseMillis,
		}
		repository, root := newProductRepository(t), t.TempDir()
		journalPath := filepath.Join(root, "run.sqlite")
		const runID, release = "topology-driver-crash", "topology-driver-crash-release"
		body, _, plan := topologyManifest(t, runID, repository, release,
			fakeBinary, fakeDigest, "submit", "submit")

		var manifest swornruntime.Manifest
		if err := json.Unmarshal(body, &manifest); err != nil {
			t.Fatal(err)
		}
		manifest.MaxParallelTracks = 1
		body, _ = json.Marshal(manifest)
		manifestPath := writeManifest(t, root, append(body, '\n'))
		runBinary(t, swornBinary, 0, "run", "--manifest", manifestPath, "--journal", journalPath)
		authorizePlan(t, journalPath, runID, plan)
		targetBefore := runGit(t, repository, "rev-parse", "main")
		runBinaryWithEnvironment(t, crashBinary, 86, crashEnvironment,
			"resume", "--run", runID, "--journal", journalPath,
			"--command", "resume-1", "--generation", "0")
		store, err := journal.OpenReadOnly(context.Background(), journalPath)
		if err != nil {
			t.Fatal(err)
		}
		snapshot, err := store.Snapshot(context.Background(), runID)
		_ = store.Close()
		if err != nil {
			t.Fatal(err)
		}
		var crashed []string
		for _, effect := range snapshot.Effects {
			if effect.Kind == "driver.dispatch" && effect.State == journal.Claimed {
				crashed = append(crashed, effect.ID)
			}
		}
		if len(crashed) != 1 {
			t.Fatalf("claimed driver crash effects = %v, want one", crashed)
		}
		parts := strings.Split(crashed[0], "/")
		if len(parts) != 4 || parts[0] != "attempt" ||
			parts[2] != "e1" || parts[3] != "t1" {
			t.Fatalf("crashed driver effect = %q", crashed[0])
		}
		workID := "sha256:" + parts[1]
		leaseExpiryWait()
		runBinary(t, swornBinary, 0, "takeover", "--run", runID,
			"--journal", journalPath, "--command", "takeover-1", "--generation", "1")
		status, _ := runBinary(t, swornBinary, 0, "status", "--run", runID,
			"--journal", journalPath, "--json")
		// The crashed dispatch's effect claim (production effectLease, five
		// minutes) is still unexpired at takeover, so the reconciled sweep
		// preserves the claim instead of writing dispatch_uncertain: the
		// run stays quiescent-uncertain and the target never moves.
		if !strings.Contains(status, `"state": "uncertain"`) ||
			runGit(t, repository, "rev-parse", "main") != targetBefore {
			t.Fatalf("driver crash was retried or moved target: %s", status)
		}
		store, err = journal.OpenReadOnly(context.Background(), journalPath)
		if err != nil {
			t.Fatal(err)
		}
		snapshot, err = store.Snapshot(context.Background(), runID)
		_ = store.Close()
		if err != nil {
			t.Fatal(err)
		}
		for _, event := range snapshot.Events {
			if event.Kind == "dispatch_uncertain" {
				t.Fatalf(
					"unexpired crashed claim was written uncertain: %#v",
					event,
				)
			}
		}
		byID := effectsByID(snapshot.Effects)
		t1 := byID[journal.AttemptEffectID(workID, 1, 1)]
		_, hasT2 := byID[journal.AttemptEffectID(workID, 1, 2)]
		_, hasT3 := byID[journal.AttemptEffectID(workID, 1, 3)]
		_, hasT4 := byID[journal.AttemptEffectID(workID, 1, 4)]
		if t1.State != journal.Claimed || t1.CurrentClaim == "" ||
			hasT2 || hasT3 || hasT4 {
			t.Fatalf(
				"driver crash retry sequence: t1=%#v t2=%t t3=%t t4=%t",
				t1, hasT2, hasT3, hasT4)
		}
	})

	t.Run("forward_target_claimed_driver_is_terminalized_before_fresh_dispatch", func(t *testing.T) {
		crashBinary := filepath.Join(buildRoot, "sworn-driver-stale-crash")
		buildBinary(t, crashBinary, "./cmd/sworn", hookGateLDFlags)
		crashEnvironment := map[string]string{
			"SWORN_TEST_CRASH_AFTER_EFFECT": "driver.dispatch",
			"SWORN_TEST_OWNER_LEASE_MILLIS": testLeaseMillis,
		}
		repository, root := newProductRepository(t), t.TempDir()
		journalPath := filepath.Join(root, "run.sqlite")
		const runID = "topology-driver-stale"
		const release = "topology-driver-stale-release"
		body, _, topologyPlan := topologyManifest(
			t, runID, repository, release,
			fakeBinary, fakeDigest, "submit", "submit")

		initialBytes, initialPlan := singleTrackPlan(t, topologyPlan)
		var manifest swornruntime.Manifest
		if err := json.Unmarshal(body, &manifest); err != nil {
			t.Fatal(err)
		}
		manifest.MaxParallelTracks = 1
		bindInitialPlannerScripts(t, &manifest, runID, initialBytes)
		body, _ = json.Marshal(manifest)
		manifestPath := writeManifest(t, root, append(body, '\n'))
		runBinary(t, swornBinary, 0,
			"run", "--manifest", manifestPath, "--journal", journalPath)
		authorizePlan(t, journalPath, runID, initialPlan)
		runBinaryWithEnvironment(t, crashBinary, 86, crashEnvironment,
			"resume", "--run", runID, "--journal", journalPath,
			"--command", "resume-1", "--generation", "0")

		store, err := journal.OpenReadOnly(context.Background(), journalPath)
		if err != nil {
			t.Fatal(err)
		}
		snapshot, err := store.Snapshot(context.Background(), runID)
		_ = store.Close()
		if err != nil {
			t.Fatal(err)
		}
		claimedID := ""
		for _, effect := range snapshot.Effects {
			if effect.Kind == "driver.dispatch" &&
				effect.State == journal.Claimed {
				if claimedID != "" {
					t.Fatal("multiple claimed driver effects at crash")
				}
				claimedID = effect.ID
			}
		}
		if claimedID == "" {
			t.Fatal("driver crash did not leave a claimed effect")
		}
		if err := os.WriteFile(
			filepath.Join(repository, "target-moved.txt"),
			[]byte("external target move\n"),
			0o644,
		); err != nil {
			t.Fatal(err)
		}
		runGit(t, repository, "add", "--", "target-moved.txt")
		runGit(t, repository, "commit", "--quiet", "-m", "external target move")

		leaseExpiryWait()
		runBinary(
			t, swornBinary, 0,
			"takeover", "--run", runID, "--journal", journalPath,
			"--command", "takeover-1", "--generation", "1")
		stdout, _ := runBinary(
			t, swornBinary, 0,
			"run", "--manifest", manifestPath, "--journal", journalPath)
		if !strings.Contains(stdout, "  state: complete") {
			t.Fatalf("forward-target driver takeover = %q", stdout)
		}
		state := readBatonState(t, repository, release)
		if state.Plan.TargetStale || state.Plan.Metadata.Revision != 1 {
			t.Fatalf("forward-target driver plan = %#v", state.Plan)
		}
		store, err = journal.OpenReadOnly(context.Background(), journalPath)
		if err != nil {
			t.Fatal(err)
		}
		snapshot, err = store.Snapshot(context.Background(), runID)
		_ = store.Close()
		if err != nil {
			t.Fatal(err)
		}
		terminal := effectsByID(snapshot.Effects)[claimedID]
		if terminal.State != journal.OperationalFailed ||
			terminal.ErrorCode != "stale_authority" {
			t.Fatalf("superseded driver effect = %#v", terminal)
		}
		for _, effect := range snapshot.Effects {
			if effect.State == journal.Uncertain ||
				effect.State == journal.Claimed {
				t.Fatalf("superseded authority poisoned status: %#v", effect)
			}
		}
	})
}
