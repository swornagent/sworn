package journal

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAppendEventOnceLinearizesConcurrentCaptainRefusal(t *testing.T) {
	store, run, _, _ := journalFixture(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_075, 0).UTC()
	body := []byte(`{"schema_version":"sworn.captain-plan-refusal-event/v1"}`)
	identity := Command{RunID: run.ID, ReplayKey: "captain-refusal/" + strings.Repeat("a", 64), Kind: "captain_refusal", Payload: body, CreatedAt: now}
	const callers = 24
	errors := make(chan error, callers)
	var wait sync.WaitGroup
	for index := 0; index < callers; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			errors <- store.AppendEventOnce(ctx, identity, "captain_plan_refused", body, now)
		}()
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := store.Snapshot(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	commands, events := 0, 0
	for _, command := range snapshot.Commands {
		if command.Kind == "captain_refusal" {
			commands++
		}
	}
	for _, event := range snapshot.Events {
		if event.Kind == "captain_plan_refused" {
			events++
		}
	}
	if commands != 1 || events != 1 {
		t.Fatalf("refusal identity commands=%d events=%d", commands, events)
	}
}

func TestDelegatedBootstrapIsAtomicBeforeAuthorityAdmission(t *testing.T) {
	store, err := Open(context.Background(), t.TempDir()+"/journal.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	now := time.Unix(1_700_000_050, 0).UTC()
	run := Run{ID: "run-bootstrap", ManifestDigest: "sha256:" + strings.Repeat("1", 64), Repository: "/tmp/repository", Release: "release-1", TargetRef: "refs/heads/main", CreatedAt: now}
	manifest := Command{RunID: run.ID, ReplayKey: "same", Kind: "start", Payload: []byte("manifest"), CreatedAt: now}
	authority := Command{RunID: run.ID, ReplayKey: "same", Kind: "captain_delegation", Payload: []byte("authority"), CreatedAt: now}
	effect := Effect{RunID: run.ID, ID: "authority", ReplayKey: "same", Kind: "captain.delegation", BeforeDigest: digest(authority.Payload), ExpectedDigest: digest([]byte("result")), UpdatedAt: now}
	if err := store.RegisterRunCommandsEffect(ctx, run, manifest, authority, effect); !IsCode(err, "REPLAY_CONFLICT") {
		t.Fatalf("bootstrap conflict = %v", err)
	}
	if _, err := store.RunBinding(ctx, run.ID); !IsCode(err, "RUN_NOT_FOUND") {
		t.Fatalf("partial run survived rollback = %v", err)
	}
	manifest.ReplayKey = "manifest"
	if err := store.RegisterRunCommandsEffect(ctx, run, manifest, authority, effect); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(ctx, run.ID)
	if err != nil || len(snapshot.Commands) != 2 || len(snapshot.Effects) != 1 || snapshot.Effects[0].State != Pending {
		t.Fatalf("atomic bootstrap = %#v, %v", snapshot, err)
	}
}

func TestCompleteWithChildIsAtomicAndEventOffsetFencesAuthority(t *testing.T) {
	store, run, _, effect := journalFixture(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_100, 0).UTC()
	claim, err := store.Claim(ctx, run.ID, effect.ID, now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	stale := int64(0)
	if err := store.AppendEvent(ctx, run.ID, "authority_changed", []byte("revoked"), now); err != nil {
		t.Fatal(err)
	}
	childCommand := Command{RunID: run.ID, ReplayKey: "child-1", Kind: "approval", Payload: []byte("child"), CreatedAt: now}
	childEffect := Effect{RunID: run.ID, ID: "child-1", ReplayKey: "child-1", Kind: "approval.admit", BeforeDigest: digest([]byte("child")), ExpectedDigest: digest([]byte("result")), UpdatedAt: now}
	completion := Completion{RunID: run.ID, EffectID: effect.ID, Token: claim.Token, State: Succeeded, Result: []byte("decision"), EventKind: "captain_plan_decided", EventBody: []byte("safe"), At: now, ExpectedEventOffset: &stale}
	if err := store.CompleteWithChild(ctx, completion, &childCommand, &childEffect); !IsCode(err, "STALE_COMPLETION") {
		t.Fatalf("stale completion = %v", err)
	}
	if _, err := store.Effect(ctx, run.ID, "child-1"); !IsCode(err, "EFFECT_NOT_FOUND") {
		t.Fatalf("stale child exists = %v", err)
	}
	current := int64(1)
	completion.ExpectedEventOffset = &current
	if err := store.CompleteWithChild(ctx, completion, &childCommand, &childEffect); err != nil {
		t.Fatal(err)
	}
	child, err := store.Effect(ctx, run.ID, "child-1")
	if err != nil || child.State != Pending {
		t.Fatalf("child = %#v, %v", child, err)
	}
}

func TestCaptainCompletionLinearizesAgainstRevocationAndReplacement(t *testing.T) {
	for _, mutation := range []string{"revocation", "replacement"} {
		for _, winner := range []string{"captain", mutation} {
			t.Run(mutation+"_"+winner+"_wins", func(t *testing.T) {
				store, run, _, captainEffect := journalFixture(t)
				ctx := context.Background()
				now := time.Unix(1_700_000_200, 0).UTC()
				mutationCommand := Command{RunID: run.ID, ReplayKey: mutation, Kind: "captain_delegation", Payload: []byte(mutation), CreatedAt: now}
				mutationEffect := Effect{RunID: run.ID, ID: mutation, ReplayKey: mutation, Kind: "captain.delegation", BeforeDigest: digest([]byte(mutation)), ExpectedDigest: digest([]byte("changed")), UpdatedAt: now}
				if err := store.RecordCommandEffect(ctx, mutationCommand, mutationEffect); err != nil {
					t.Fatal(err)
				}
				captainClaim, err := store.Claim(ctx, run.ID, captainEffect.ID, now, time.Minute)
				if err != nil {
					t.Fatal(err)
				}
				mutationClaim, err := store.Claim(ctx, run.ID, mutationEffect.ID, now, time.Minute)
				if err != nil {
					t.Fatal(err)
				}
				offset := int64(0)
				childCommand := Command{RunID: run.ID, ReplayKey: "delegated-approval", Kind: "approval", Payload: []byte("approval"), CreatedAt: now}
				childEffect := Effect{RunID: run.ID, ID: "delegated-approval", ReplayKey: "delegated-approval", Kind: "approval.admit", BeforeDigest: digest([]byte("approval")), ExpectedDigest: digest([]byte("admitted")), UpdatedAt: now}
				captainCompletion := Completion{RunID: run.ID, EffectID: captainEffect.ID, Token: captainClaim.Token, State: Succeeded, Result: []byte("proceed"), EventKind: "captain_plan_decided", EventBody: []byte("safe"), At: now, ExpectedEventOffset: &offset}
				mutationCompletion := Completion{RunID: run.ID, EffectID: mutationEffect.ID, Token: mutationClaim.Token, State: Succeeded, Result: []byte("changed"), EventKind: "captain_delegation_" + mutation, EventBody: []byte(mutation), At: now, ExpectedEventOffset: &offset}
				if winner == "captain" {
					if err := store.CompleteWithChild(ctx, captainCompletion, &childCommand, &childEffect); err != nil {
						t.Fatal(err)
					}
					if err := store.Complete(ctx, mutationCompletion); !IsCode(err, "STALE_COMPLETION") {
						t.Fatalf("mutation loser = %v", err)
					}
					child, err := store.Effect(ctx, run.ID, childEffect.ID)
					if err != nil || child.State != Pending {
						t.Fatalf("approval child = %#v, %v", child, err)
					}
				} else {
					if err := store.Complete(ctx, mutationCompletion); err != nil {
						t.Fatal(err)
					}
					if err := store.CompleteWithChild(ctx, captainCompletion, &childCommand, &childEffect); !IsCode(err, "STALE_COMPLETION") {
						t.Fatalf("captain loser = %v", err)
					}
					if _, err := store.Effect(ctx, run.ID, childEffect.ID); !IsCode(err, "EFFECT_NOT_FOUND") {
						t.Fatalf("losing decision created approval = %v", err)
					}
				}
			})
		}
	}
}

func TestReviseSupersessionEventCrashCutReplaysOneContinuation(t *testing.T) {
	store, run, _, decision := journalFixture(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_300, 0).UTC()
	claim, err := store.Claim(ctx, run.ID, decision.ID, now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	childCommand := Command{RunID: run.ID, ReplayKey: "planner-continuation", Kind: "planner_continuation", Payload: []byte("bounded continuation"), CreatedAt: now}
	childEffect := Effect{RunID: run.ID, ID: "planner-continuation", ReplayKey: "planner-continuation", Kind: "planner.continue", BeforeDigest: digest(childCommand.Payload), ExpectedDigest: digest([]byte("scheduled")), UpdatedAt: now}
	offset := int64(0)
	completion := Completion{RunID: run.ID, EffectID: decision.ID, Token: claim.Token, State: Succeeded, Result: []byte("revise"), EventKind: "captain_plan_decided", EventBody: []byte("safe revise"), At: now, ExpectedEventOffset: &offset}
	if err := store.CompleteWithChild(ctx, completion, &childCommand, &childEffect); err != nil {
		t.Fatal(err)
	}
	for restart := 0; restart < 3; restart++ {
		snapshot, err := store.Snapshot(ctx, run.ID)
		if err != nil {
			t.Fatal(err)
		}
		continuations, events := 0, 0
		for _, effect := range snapshot.Effects {
			if effect.Kind == "planner.continue" && effect.State == Pending {
				continuations++
			}
		}
		for _, event := range snapshot.Events {
			if event.Kind == "captain_plan_decided" {
				events++
			}
		}
		if continuations != 1 || events != 1 {
			t.Fatalf("restart %d continuation=%d event=%d", restart, continuations, events)
		}
	}
}

func TestDelegatedApprovalAdmissionLinearizesAgainstRevocationAndReplacement(t *testing.T) {
	for _, mutation := range []string{"revocation", "replacement"} {
		for _, winner := range []string{"approval", mutation} {
			t.Run(mutation+"_"+winner+"_wins", func(t *testing.T) {
				store, run, _, approval := journalFixture(t)
				ctx := context.Background()
				now := time.Unix(1_700_000_400, 0).UTC()
				mutationCommand := Command{RunID: run.ID, ReplayKey: mutation, Kind: "captain_delegation", Payload: []byte(mutation), CreatedAt: now}
				mutationEffect := Effect{RunID: run.ID, ID: mutation, ReplayKey: mutation, Kind: "captain.delegation", BeforeDigest: digest([]byte(mutation)), ExpectedDigest: digest([]byte("changed")), UpdatedAt: now}
				if err := store.RecordCommandEffect(ctx, mutationCommand, mutationEffect); err != nil {
					t.Fatal(err)
				}
				approvalClaim, _ := store.Claim(ctx, run.ID, approval.ID, now, time.Minute)
				mutationClaim, _ := store.Claim(ctx, run.ID, mutationEffect.ID, now, time.Minute)
				offset := int64(0)
				approvalCompletion := Completion{RunID: run.ID, EffectID: approval.ID, Token: approvalClaim.Token, State: Succeeded, Result: []byte("admitted"), EventKind: "approval_admitted", EventBody: []byte("safe"), At: now, ExpectedEventOffset: &offset}
				mutationCompletion := Completion{RunID: run.ID, EffectID: mutationEffect.ID, Token: mutationClaim.Token, State: Succeeded, Result: []byte("changed"), EventKind: "captain_delegation_" + mutation, EventBody: []byte(mutation), At: now, ExpectedEventOffset: &offset}
				if winner == "approval" {
					if err := store.Complete(ctx, approvalCompletion); err != nil {
						t.Fatal(err)
					}
					if err := store.Complete(ctx, mutationCompletion); !IsCode(err, "STALE_COMPLETION") {
						t.Fatalf("mutation loser = %v", err)
					}
				} else {
					if err := store.Complete(ctx, mutationCompletion); err != nil {
						t.Fatal(err)
					}
					if err := store.Complete(ctx, approvalCompletion); !IsCode(err, "STALE_COMPLETION") {
						t.Fatalf("approval loser = %v", err)
					}
				}
				current, err := store.Effect(ctx, run.ID, approval.ID)
				if err != nil || (current.State == Succeeded) != (winner == "approval") {
					t.Fatalf("approval winner state = %#v, %v", current, err)
				}
			})
		}
	}
}
