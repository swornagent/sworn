package runtime

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/journal"
)

func approvalFixture(t *testing.T) (admittedManifest, admittedPlanProposal, ApprovalCommand) {
	t.Helper()
	manifestValue, body, plan := fixtureManifest(t)
	manifestValue.Authority.BootstrapApprovedPlanDigest = nil
	body, err := canonicalManifest(manifestValue)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := admitManifest(body)
	if err != nil {
		t.Fatal(err)
	}
	proposal := admittedPlanProposal{
		plan:      plan,
		replayKey: "plan-proposal/00000000000000000001/" + strings.Repeat("1", 64),
		authority: planProposalAuthority{
			Release:    manifest.value.Release,
			ReleaseRef: "refs/heads/release-wt/" + manifest.value.Release,
			TargetRef:  manifest.value.TargetRef,
			TargetHead: strings.Repeat("2", 40),
		},
	}
	command, err := approvalCommandForProposal(manifest, proposal)
	if err != nil {
		t.Fatal(err)
	}
	return manifest, proposal, command
}

func TestReplanDecisionClassComesOnlyFromExactLineage(t *testing.T) {
	manifest, initial, _ := approvalFixture(t)
	prior := strings.Repeat("9", 40)
	metadata := initial.plan.Metadata()
	metadata.Revision = 2
	metadata.PreviousPlan = &prior
	metadata.ApprovalRef = "operator://" + manifest.value.Release + "/2"
	body, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := baton.ParsePlan([]byte(
		"```baton-plan-v2\n" + string(body) + "\n```\n\nReplan.\n",
	))
	if err != nil {
		t.Fatal(err)
	}
	proposal := initial
	proposal.plan = plan
	proposal.authority.PriorPlan = prior
	proposal.authority.ReleaseHead = strings.Repeat("8", 40)
	command, err := approvalCommandForProposal(manifest, proposal)
	if err != nil || command.DecisionClass != PlannerReplanClass {
		t.Fatalf("derived replan class = %q, %v", command.DecisionClass, err)
	}
	command.DecisionClass = PlannerProposalClass
	if err := validateApprovalAuthority(manifest, proposal, command); err == nil {
		t.Fatal("caller-selected proposal class overrode replan lineage")
	}
	proposal.authority.PriorPlan = strings.Repeat("7", 40)
	if _, err := approvalDecisionClass(proposal); !IsCode(err, "APPROVAL_AMBIGUOUS") {
		t.Fatalf("broken lineage = %v", err)
	}
}

func TestApprovalCommandIsCanonicalAndEveryBindingFailsClosed(t *testing.T) {
	manifest, proposal, command := approvalFixture(t)
	canonical, err := CanonicalApprovalCommand(command)
	if err != nil || string(append(canonical, '\n')) != string(mustJSON(command)) {
		t.Fatalf("canonical command = %q, %v", canonical, err)
	}
	mutations := map[string]func(*ApprovalCommand){
		"schema":          func(v *ApprovalCommand) { v.SchemaVersion = "wrong" },
		"run":             func(v *ApprovalCommand) { v.RunID = "other" },
		"manifest":        func(v *ApprovalCommand) { v.ManifestDigest = "sha256:" + strings.Repeat("3", 64) },
		"project":         func(v *ApprovalCommand) { v.Project = "other" },
		"release":         func(v *ApprovalCommand) { v.Release = "other" },
		"release_ref":     func(v *ApprovalCommand) { v.ReleaseRef += "-other" },
		"release_head":    func(v *ApprovalCommand) { v.ReleaseHead = strings.Repeat("4", 40) },
		"proposal":        func(v *ApprovalCommand) { v.ProposalReplayKey += "-other" },
		"revision":        func(v *ApprovalCommand) { v.PlanRevision++ },
		"prior":           func(v *ApprovalCommand) { v.PriorPlan = strings.Repeat("5", 40) },
		"digest":          func(v *ApprovalCommand) { v.PlanDigest = "sha256:" + strings.Repeat("6", 64) },
		"target_ref":      func(v *ApprovalCommand) { v.TargetRef = "refs/heads/other" },
		"target_head":     func(v *ApprovalCommand) { v.TargetHead = strings.Repeat("7", 40) },
		"class":           func(v *ApprovalCommand) { v.DecisionClass = PlannerReplanClass },
		"decision":        func(v *ApprovalCommand) { v.Decision = "reject" },
		"actor_class":     func(v *ApprovalCommand) { v.ActorClass = "captain" },
		"actor_authority": func(v *ApprovalCommand) { v.ActorAuthority = "other" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			changed := command
			mutate(&changed)
			if reflect.DeepEqual(changed, command) {
				t.Fatal("mutation did not change command")
			}
			if err := validateApprovalAuthority(manifest, proposal, changed); err == nil {
				t.Fatal("mutated binding was admitted")
			}
		})
	}
}

func TestSucceededExactAdmissionIsAuthorityAndHistoricalAdmissionIsInert(t *testing.T) {
	manifest, proposal, command := approvalFixture(t)
	replayKey, effectID, payload, err := approvalIdentity(command)
	if err != nil {
		t.Fatal(err)
	}
	_, resultBody, err := canonicalApprovalResult(command)
	if err != nil {
		t.Fatal(err)
	}
	exactCommand := journal.Command{
		RunID: command.RunID, ReplayKey: replayKey, Kind: "approval", Payload: payload,
	}
	exactEffect := journal.Effect{
		RunID: command.RunID, ID: effectID, ReplayKey: replayKey,
		Kind: approvalEffectKind, State: journal.Succeeded,
		ExpectedDigest: sha256Digest(resultBody), Result: resultBody,
	}
	if got, err := effectivePlanAuthority(manifest, journal.Snapshot{
		Commands: []journal.Command{exactCommand}, Effects: []journal.Effect{exactEffect},
	}, &proposal); err != nil || got != command.PlanDigest {
		t.Fatalf("exact admission authority = %q, %v", got, err)
	}

	historicalProposal := proposal
	historicalProposal.authority.TargetHead = strings.Repeat("8", 40)
	historical, err := approvalCommandForProposal(manifest, historicalProposal)
	if err != nil {
		t.Fatal(err)
	}
	historicalKey, historicalID, historicalPayload, _ := approvalIdentity(historical)
	_, historicalResult, _ := canonicalApprovalResult(historical)
	if got, err := effectivePlanAuthority(manifest, journal.Snapshot{
		Commands: []journal.Command{{
			RunID: historical.RunID, ReplayKey: historicalKey,
			Kind: "approval", Payload: historicalPayload,
		}},
		Effects: []journal.Effect{{
			RunID: historical.RunID, ID: historicalID, ReplayKey: historicalKey,
			Kind: approvalEffectKind, State: journal.Succeeded,
			ExpectedDigest: sha256Digest(historicalResult), Result: historicalResult,
		}},
	}, &proposal); err != nil || got != "" {
		t.Fatalf("historical admission became authority = %q, %v", got, err)
	}
}

func TestApprovalResultIsStableAcrossExactReplay(t *testing.T) {
	_, _, command := approvalFixture(t)
	first, firstBody, err := canonicalApprovalResult(command)
	if err != nil {
		t.Fatal(err)
	}
	second, secondBody, err := canonicalApprovalResult(command)
	if err != nil || !reflect.DeepEqual(first, second) || string(firstBody) != string(secondBody) {
		t.Fatalf("replayed result drifted: %#v %#v %v", first, second, err)
	}
}
