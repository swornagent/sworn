package runtime

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/journal"
)

func TestV4ProjectAuthorityIsOptionalExactAndClosed(t *testing.T) {
	manifest, body, plan := fixtureManifest(t)
	if manifest.Authority.BootstrapApprovedPlanDigest == nil ||
		*manifest.Authority.BootstrapApprovedPlanDigest != plan.Digest() {
		t.Fatal("fixture bootstrap authority is not exact")
	}

	absent := manifest
	absent.Authority.BootstrapApprovedPlanDigest = nil
	absentBody, err := canonicalManifest(absent)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admitManifest(absentBody); err != nil {
		t.Fatalf("absent optional authority = %v", err)
	}

	for name, digest := range map[string]string{
		"empty": "", "uppercase": "sha256:" + strings.Repeat("A", 64),
		"short": "sha256:abcd", "wrong algorithm": "sha512:" + strings.Repeat("a", 64),
	} {
		t.Run(name, func(t *testing.T) {
			value := manifest
			value.Authority.BootstrapApprovedPlanDigest = &digest
			raw, _ := json.Marshal(value)
			if _, err := admitManifest(append(raw, '\n')); !IsCode(err, "INVALID_AUTHORITY") {
				t.Fatalf("malformed digest = %v", err)
			}
		})
	}
	for name, authorizer := range map[string]string{
		"empty": "", "url": "https://operator", "account": "operator/user",
		"overlong": strings.Repeat("a", 129),
	} {
		t.Run("authorizer_"+name, func(t *testing.T) {
			value := manifest
			value.Authority.ExternalAuthorizer = authorizer
			raw, _ := json.Marshal(value)
			if _, err := admitManifest(append(raw, '\n')); !IsCode(err, "INVALID_AUTHORITY") {
				t.Fatalf("external authorizer = %v", err)
			}
		})
	}
	withGitHubField := strings.Replace(
		string(body), `"authority":{`,
		`"approval":{"repository":"host/project"},"authority":{`, 1)
	if _, err := admitManifest([]byte(withGitHubField)); !IsCode(err, "INVALID_MANIFEST") {
		t.Fatalf("former hosted approval field = %v", err)
	}
}

func TestEffectiveAuthorityRejectsEveryDistinctBootstrapJournalCombination(t *testing.T) {
	_, body, plan := fixtureManifest(t)
	manifest, err := admitManifest(body)
	if err != nil {
		t.Fatal(err)
	}
	command := func(digest string) journal.Command {
		return journal.Command{
			ReplayKey: "plan-authority/" + strings.TrimPrefix(digest, "sha256:"),
			Kind:      "plan_authority",
			Payload: mustJSON(planAuthorityCommand{
				Version: planAuthorityVersion, PlanDigest: digest,
			}),
		}
	}
	if got, err := effectivePlanAuthority(manifest, journal.Snapshot{
		Commands: []journal.Command{command(plan.Digest())},
	}); err != nil || got != plan.Digest() {
		t.Fatalf("identical bootstrap and journal authority = %q, %v", got, err)
	}
	other := "sha256:" + strings.Repeat("b", 64)
	if _, err := effectivePlanAuthority(manifest, journal.Snapshot{
		Commands: []journal.Command{command(other)},
	}); !IsCode(err, "AUTHORITY_CONFLICT") {
		t.Fatalf("distinct bootstrap and journal authority = %v", err)
	}
	malformed := command(plan.Digest())
	malformed.ReplayKey = "plan-authority/wrong"
	if _, err := effectivePlanAuthority(manifest, journal.Snapshot{
		Commands: []journal.Command{malformed},
	}); !IsCode(err, "CORRUPT_JOURNAL") {
		t.Fatalf("malformed journal authority = %v", err)
	}
}

func TestSavedPlanAdoptionRequiresIndependentExactAuthorityAndBatonApproval(t *testing.T) {
	_, body, plan := fixtureManifest(t)
	manifest, err := admitManifest(body)
	if err != nil {
		t.Fatal(err)
	}
	target := strings.Repeat("1", 40)
	planOID := strings.Repeat("2", 40)
	receipt := baton.Receipt{
		Version: baton.ReceiptVersion, Release: manifest.value.Release,
		Role: "planner", Result: "approved", Plan: planOID,
		Summary: "saved approval", Target: &target,
	}
	state := baton.State{
		Release:    manifest.value.Release,
		Repository: manifest.value.Authority.Project,
		Plan: baton.PlanState{
			OID: planOID, Digest: plan.Digest(), Metadata: plan.Metadata(),
			Approval: baton.ReceiptEntry{OID: strings.Repeat("3", 40), Receipt: receipt},
			History:  []baton.PlanHistory{{OID: planOID, Revision: 1, Plan: plan}},
		},
		Refs: baton.StateRefs{
			Release: baton.CapturedRef{
				Ref:  "refs/heads/release-wt/" + manifest.value.Release,
				Head: strings.Repeat("4", 40),
			},
			Target: baton.CapturedRef{Ref: manifest.value.TargetRef, Head: target},
		},
	}
	engine := &engine{manifest: manifest}
	if adopted, err := validateSavedPlanAdoption(engine, state, ""); err != nil || adopted {
		t.Fatalf("Baton approval alone adopted = %t, %v", adopted, err)
	}
	if adopted, err := validateSavedPlanAdoption(engine, state, plan.Digest()); err != nil || !adopted {
		t.Fatalf("exact conjunction = %t, %v", adopted, err)
	}
	for name, mutate := range map[string]func(*baton.State){
		"project":     func(value *baton.State) { value.Repository = "other" },
		"release":     func(value *baton.State) { value.Release = "other" },
		"target head": func(value *baton.State) { value.Refs.Target.Head = strings.Repeat("5", 40) },
		"approval target": func(value *baton.State) {
			other := strings.Repeat("6", 40)
			value.Plan.Approval.Receipt.Target = &other
		},
		"digest":        func(value *baton.State) { value.Plan.Digest = "sha256:" + strings.Repeat("7", 64) },
		"bytes":         func(value *baton.State) { value.Plan.History = nil },
		"stale lineage": func(value *baton.State) { value.Plan.TargetStale = true },
	} {
		t.Run(name, func(t *testing.T) {
			value := state
			value.Plan = state.Plan
			value.Plan.Approval = state.Plan.Approval.Clone()
			value.Plan.History = append([]baton.PlanHistory(nil), state.Plan.History...)
			mutate(&value)
			if adopted, err := validateSavedPlanAdoption(engine, value, plan.Digest()); err == nil || adopted {
				t.Fatalf("substituted saved plan adopted = %t, %v", adopted, err)
			}
		})
	}
}

func TestEveryInstallEffectStateOutranksFreshAdoption(t *testing.T) {
	for _, state := range []journal.EffectState{
		journal.Pending, journal.Claimed, journal.Uncertain,
	} {
		validate, err := installEffectPrecedence(state)
		if validate || !IsCode(err, "INSTALL_RECOVERY_PENDING") {
			t.Fatalf("%s precedence = validate:%t err:%v", state, validate, err)
		}
	}
	validate, err := installEffectPrecedence(journal.OperationalFailed)
	if validate || !IsCode(err, "INSTALL_FAILED") {
		t.Fatalf("failed install precedence = validate:%t err:%v", validate, err)
	}
	validate, err = installEffectPrecedence(journal.Succeeded)
	if !validate || err != nil {
		t.Fatalf("succeeded install precedence = validate:%t err:%v", validate, err)
	}
}

func TestLegacyV2V3AreReadOnlyBeforeEveryMutation(t *testing.T) {
	for _, version := range []string{ManifestVersionV2, ManifestVersionV3} {
		t.Run(version, func(t *testing.T) {
			ctx := context.Background()
			legacy := []byte(`{"schema_version":"` + version + `","run_id":"legacy"}` + "\n")
			if got, err := ClassifyManifestVersion(legacy); err != nil || got != version {
				t.Fatalf("classification = %q, %v", got, err)
			}
			path := filepath.Join(t.TempDir(), "legacy.sqlite")
			store, err := journal.Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			now := time.Date(2026, 8, 4, 0, 0, 0, 0, time.UTC)
			service := &Service{journal: store, now: func() time.Time { return now }}
			if _, err := service.Start(ctx, legacy); !IsCode(err, "MIGRATION_REQUIRED") {
				t.Fatalf("legacy start = %v", err)
			}
			bindings, err := store.RunBindings(ctx)
			if err != nil || len(bindings) != 0 {
				t.Fatalf("legacy start registered a run: %#v, %v", bindings, err)
			}
			run := journal.Run{ID: "legacy", ManifestDigest: sha256Digest(legacy),
				Repository: "/legacy", Release: "legacy-release",
				TargetRef: "refs/heads/main", CreatedAt: now}
			if err := store.RegisterRun(ctx, run); err != nil {
				t.Fatal(err)
			}
			if err := store.RecordCommand(ctx, journal.Command{RunID: run.ID,
				ReplayKey: "manifest", Kind: "start", Payload: legacy, CreatedAt: now}); err != nil {
				t.Fatal(err)
			}
			status, err := service.Status(ctx, run.ID)
			if err != nil || status.State != "migration_required" {
				t.Fatalf("legacy status = %#v, %v", status, err)
			}
			before, _ := store.ControlProjection(ctx, run.ID)
			for _, kind := range []journal.ControlKind{
				journal.Pause, journal.Resume, journal.Cancel,
				journal.Retry, journal.Takeover,
			} {
				if _, err := service.Control(ctx, journal.ControlCommand{
					RunID: run.ID, ID: "legacy-" + string(kind), Kind: kind,
					ExpectedGeneration: before.Generation,
					WorkID:             "sha256:" + strings.Repeat("a", 64), ExpectedEpoch: 1,
				}); !IsCode(err, "MIGRATION_REQUIRED") {
					t.Fatalf("legacy %s control = %v", kind, err)
				}
			}
			after, _ := store.ControlProjection(ctx, run.ID)
			if !reflect.DeepEqual(after, before) {
				t.Fatalf("legacy control wrote projection: before=%#v after=%#v", before, after)
			}
			if _, err := service.AnswerAttention(ctx, AnswerAttentionCommand{
				RunID: run.ID, AttentionID: "anything", ExpectedGeneration: 1,
				Answer: "continue",
			}); !IsCode(err, "MIGRATION_REQUIRED") {
				t.Fatalf("legacy attention mutation = %v", err)
			}
			_ = store.Close()
		})
	}
}
