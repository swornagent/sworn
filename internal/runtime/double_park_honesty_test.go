package runtime

import (
	"encoding/json"
	"testing"

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/journal"
)

// TestDoubleParkHonesty_A1_ReplayDoubleParkResolvedCheckpointCleanAndReachable
// pins A1: a claimed dispatch that parked twice (first attention opened,
// answered, and resolved; second attention opened on the same work identity)
// recovers without error on restart, and the parked second attention remains
// reachable for its own answer/resume.
func TestDoubleParkHonesty_A1_ReplayDoubleParkResolvedCheckpointCleanAndReachable(t *testing.T) {
	dispatcher := &turnRecoveryFixtureDriver{
		parkS1:    true,
		yieldKind: driver.YieldHumanConfirmation,
	}
	fixture := newProductionImplementationRecoveryFixture(t, dispatcher)
	defer fixture.workspace.Close()

	if err := fixture.store.RecordCommand(
		fixture.ctx,
		journal.Command{
			RunID:     fixture.owner.RunID,
			ReplayKey: "manifest",
			Kind:      "start",
			Payload:   fixture.manifest.raw,
			CreatedAt: fixture.now,
		},
	); err != nil {
		t.Fatal(err)
	}

	if _, _, err := fixture.service.runProductionImplementationDispatch(
		fixture.ctx,
		fixture.engine,
		fixture.owner,
		fixture.workspace,
		fixture.cycle,
		fixture.coordinates,
	); !IsCode(err, "EFFECT_PARKED") {
		t.Fatalf("first human park = %v", err)
	}

	attentions, err := fixture.store.Attentions(
		fixture.ctx,
		fixture.owner.RunID,
	)
	if err != nil || len(attentions) != 1 {
		t.Fatalf("first attention = %#v, %v", attentions, err)
	}
	attention1 := attentions[0]
	if attention1.Attention.HumanTurn == nil {
		t.Fatalf("expected human turn on attention 1: %#v", attention1)
	}

	// 1. Attention 1 is answered and resolved.
	if _, err := fixture.store.AnswerAttention(
		fixture.ctx,
		journal.AnswerAttentionCommand{
			RunID:              fixture.owner.RunID,
			Attention:          attention1.Attention,
			ExpectedGeneration: attention1.Generation,
			Answer:             "Confirmed first park.",
		},
		fixture.now,
	); err != nil {
		t.Fatalf("answer attention 1 = %v", err)
	}

	if _, err := fixture.store.ResolveAttention(
		fixture.ctx,
		fixture.owner,
		journal.ResolveAttentionCommand{
			RunID:              fixture.owner.RunID,
			Attention:          attention1.Attention,
			ExpectedGeneration: 2,
		},
		fixture.now,
	); err != nil {
		t.Fatalf("resolve attention 1 = %v", err)
	}

	snapshot1, err := fixture.store.Snapshot(
		fixture.ctx,
		fixture.owner.RunID,
	)
	if err != nil {
		t.Fatalf("snapshot 1 = %v", err)
	}

	var parent1Cmd journal.Command
	for _, cmd := range snapshot1.Commands {
		if cmd.ReplayKey == fixture.cycle.DispatchEffect {
			parent1Cmd = cmd
			break
		}
	}
	var parent1Eff journal.Effect
	for _, eff := range snapshot1.Effects {
		if eff.ID == fixture.cycle.DispatchEffect {
			parent1Eff = eff
			break
		}
	}

	// 2. Prepare and record second claimed dispatch effect and command.
	parent2ID := fixture.cycle.DispatchEffect + "-second"
	if err := fixture.store.RecordCommandEffect(
		fixture.ctx,
		journal.Command{
			RunID:     fixture.owner.RunID,
			ReplayKey: parent2ID,
			Kind:      "driver.dispatch",
			Payload:   parent1Cmd.Payload,
			CreatedAt: fixture.now,
		},
		journal.Effect{
			RunID:          fixture.owner.RunID,
			ID:             parent2ID,
			ReplayKey:      parent2ID,
			Kind:           "driver.dispatch",
			BeforeDigest:   parent1Eff.BeforeDigest,
			ExpectedDigest: parent1Eff.ExpectedDigest,
			UpdatedAt:      fixture.now,
		},
	); err != nil {
		t.Fatalf("record parent 2 dispatch = %v", err)
	}

	if _, err := fixture.store.ClaimOwned(
		fixture.ctx,
		fixture.owner,
		parent2ID,
		fixture.now,
		effectLease,
	); err != nil {
		t.Fatalf("claim parent 2 dispatch = %v", err)
	}

	// 3. Open Attention 2 on the same work identity with ordinal 2.
	_, cycle := preparedTurnRecoveryFixture(t, fixture)
	humanTurn2 := *attention1.Attention.HumanTurn
	humanTurn2.Ordinal = 2
	attention2Binding := attention1.Attention
	attention2Binding.Ordinal = 2
	attention2Binding.ID = journal.AttentionID(attention2Binding.Recovery, 2)
	attention2Binding.HumanTurn = &humanTurn2

	command2 := journal.ParkRecoveryAttentionCommand{
		Step: journal.RecoveryStepCommand{
			RunID:   fixture.owner.RunID,
			ID:      journal.RecoveryStepID(cycle.binding, 2),
			Binding: cycle.binding,
			Ordinal: 2,
			Kind:    journal.RecoveryParkTrack,
		},
		Attention: journal.OpenAttentionCommand{
			RunID:              fixture.owner.RunID,
			Attention:          attention2Binding,
			ExpectedGeneration: 0,
			Question:           "Which value for second park?",
		},
	}

	// Record checkpoint 2.
	if err := fixture.service.persistHumanParkCheckpoint(
		fixture.ctx,
		fixture.owner,
		parent2ID,
		command2,
	); err != nil {
		t.Fatalf("persist checkpoint 2 = %v", err)
	}

	// Open attention 2.
	if _, err := fixture.store.OpenAttention(
		fixture.ctx,
		fixture.owner,
		command2.Attention,
		fixture.now,
	); err != nil {
		t.Fatalf("open attention 2 = %v", err)
	}

	// 4. Snapshot contains:
	// - Attention 1: opened, answered, resolved
	// - Checkpoint 1 recorded
	// - Attention 2: opened
	// - Checkpoint 2 recorded
	snapshot, err := fixture.store.Snapshot(
		fixture.ctx,
		fixture.owner.RunID,
	)
	if err != nil {
		t.Fatalf("store snapshot = %v", err)
	}

	// Replay through recoverHumanParkCheckpoint.
	recovered, err := fixture.service.recoverHumanParkCheckpoint(
		fixture.ctx,
		fixture.engine,
		fixture.owner,
		snapshot,
	)
	if err != nil || recovered {
		t.Fatalf("double park recovery = recovered:%t, err:%v", recovered, err)
	}

	// Separate assertion: attention 2's projection is still journal.AttentionOpen with its original ID.
	attentionsAfter, err := fixture.store.Attentions(
		fixture.ctx,
		fixture.owner.RunID,
	)
	if err != nil {
		t.Fatalf("attentions after recovery = %v", err)
	}
	var found2 *journal.AttentionProjection
	for i := range attentionsAfter {
		if attentionsAfter[i].Attention.ID == attention2Binding.ID {
			found2 = &attentionsAfter[i]
			break
		}
	}
	if found2 == nil {
		t.Fatalf("attention 2 not found in attentions: %#v", attentionsAfter)
	}
	if found2.State != journal.AttentionOpen {
		t.Fatalf("attention 2 state = %q, want %q", found2.State, journal.AttentionOpen)
	}
	if found2.Attention.ID != attention2Binding.ID {
		t.Fatalf("attention 2 ID = %q, want %q", found2.Attention.ID, attention2Binding.ID)
	}

	// Separate assertion: AnswerAttention against attention 2 succeeds.
	if _, err := fixture.store.AnswerAttention(
		fixture.ctx,
		journal.AnswerAttentionCommand{
			RunID:              fixture.owner.RunID,
			Attention:          found2.Attention,
			ExpectedGeneration: found2.Generation,
			Answer:             "Confirmed second park.",
		},
		fixture.now,
	); err != nil {
		t.Fatalf("answer attention 2 = %v", err)
	}
}

// TestDoubleParkHonesty_A2_ActiveAttentionBindingMismatchFails pins A2:
// a checkpoint whose own attention is still open or answered must match the
// active attention for its work identity exactly, or recovery fails closed
// with CORRUPT_JOURNAL. Per bounded correction 1, the altered field on the
// checkpoint side must not be WorkIdentity.
func TestDoubleParkHonesty_A2_ActiveAttentionBindingMismatchFails(t *testing.T) {
	dispatcher := &turnRecoveryFixtureDriver{
		parkS1:    true,
		yieldKind: driver.YieldHumanConfirmation,
	}
	fixture := newProductionImplementationRecoveryFixture(t, dispatcher)
	defer fixture.workspace.Close()

	if err := fixture.store.RecordCommand(
		fixture.ctx,
		journal.Command{
			RunID:     fixture.owner.RunID,
			ReplayKey: "manifest",
			Kind:      "start",
			Payload:   fixture.manifest.raw,
			CreatedAt: fixture.now,
		},
	); err != nil {
		t.Fatal(err)
	}

	if _, _, err := fixture.service.runProductionImplementationDispatch(
		fixture.ctx,
		fixture.engine,
		fixture.owner,
		fixture.workspace,
		fixture.cycle,
		fixture.coordinates,
	); !IsCode(err, "EFFECT_PARKED") {
		t.Fatalf("human park = %v", err)
	}

	snapshot, err := fixture.store.Snapshot(
		fixture.ctx,
		fixture.owner.RunID,
	)
	if err != nil {
		t.Fatal(err)
	}

	parentID := fixture.cycle.DispatchEffect
	checkpointID := humanParkCheckpointID(parentID)

	// Mutate the checkpoint's HumanTurn binding field (Role, not WorkIdentity).
	var mutatedCheckpoint humanParkCheckpoint
	foundIndex := -1
	for i, cmd := range snapshot.Commands {
		if cmd.ReplayKey == checkpointID {
			foundIndex = i
			if err := json.Unmarshal(cmd.Payload, &mutatedCheckpoint); err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	if foundIndex == -1 {
		t.Fatalf("checkpoint command not found in snapshot: %s", checkpointID)
	}

	// Change Role on checkpoint side to driver.RolePlanner (differing from active attention's Role).
	mutatedCheckpoint.Command.Attention.Attention.HumanTurn.Role = string(driver.RolePlanner)
	mutatedPayload := mustJSON(mutatedCheckpoint)

	// Update command and effect in snapshot with matching digests so validateHumanParkCheckpoint passes.
	for i := range snapshot.Commands {
		if snapshot.Commands[i].ReplayKey == checkpointID {
			snapshot.Commands[i].Payload = mutatedPayload
		}
	}
	for i := range snapshot.Effects {
		if snapshot.Effects[i].ID == checkpointID {
			snapshot.Effects[i].ExpectedDigest = sha256Digest(mutatedPayload)
			snapshot.Effects[i].ResultDigest = sha256Digest(mutatedPayload)
			snapshot.Effects[i].Result = mutatedPayload
		}
	}

	_, err = fixture.service.recoverHumanParkCheckpoint(
		fixture.ctx,
		fixture.engine,
		fixture.owner,
		snapshot,
	)
	if !IsCode(err, "CORRUPT_JOURNAL") {
		t.Fatalf("mismatched HumanTurn binding recovery = %v, want CORRUPT_JOURNAL", err)
	}
}

// TestDoubleParkHonesty_A3_DuplicateActiveAttentionsFail pins A3:
// two OPEN attentions sharing one work identity refuse CORRUPT_JOURNAL
// via activeAttentionWork's duplicate-ProgressID check, reported and asserted
// independently of A4.
func TestDoubleParkHonesty_A3_DuplicateActiveAttentionsFail(t *testing.T) {
	dispatcher := &turnRecoveryFixtureDriver{
		parkS1:    true,
		yieldKind: driver.YieldHumanConfirmation,
	}
	fixture := newProductionImplementationRecoveryFixture(t, dispatcher)
	defer fixture.workspace.Close()

	if err := fixture.store.RecordCommand(
		fixture.ctx,
		journal.Command{
			RunID:     fixture.owner.RunID,
			ReplayKey: "manifest",
			Kind:      "start",
			Payload:   fixture.manifest.raw,
			CreatedAt: fixture.now,
		},
	); err != nil {
		t.Fatal(err)
	}

	if _, _, err := fixture.service.runProductionImplementationDispatch(
		fixture.ctx,
		fixture.engine,
		fixture.owner,
		fixture.workspace,
		fixture.cycle,
		fixture.coordinates,
	); !IsCode(err, "EFFECT_PARKED") {
		t.Fatalf("human park = %v", err)
	}

	attentions, err := fixture.store.Attentions(
		fixture.ctx,
		fixture.owner.RunID,
	)
	if err != nil || len(attentions) != 1 {
		t.Fatalf("first attention = %#v, %v", attentions, err)
	}
	first := attentions[0]

	// Open a second concurrent AttentionOpen sharing the same Recovery.ProgressID.
	secondBinding := first.Attention
	secondBinding.Ordinal = 99
	secondBinding.ID = journal.AttentionID(secondBinding.Recovery, 99)
	secondHuman := *first.Attention.HumanTurn
	secondHuman.Ordinal = 99
	secondBinding.HumanTurn = &secondHuman

	if _, err := fixture.store.OpenAttention(
		fixture.ctx,
		fixture.owner,
		journal.OpenAttentionCommand{
			RunID:              fixture.owner.RunID,
			Attention:          secondBinding,
			ExpectedGeneration: 0,
			Question:           "Conflicting concurrent question.",
		},
		fixture.now,
	); err != nil {
		t.Fatalf("open second attention = %v", err)
	}

	snapshot, err := fixture.store.Snapshot(
		fixture.ctx,
		fixture.owner.RunID,
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = fixture.service.recoverHumanParkCheckpoint(
		fixture.ctx,
		fixture.engine,
		fixture.owner,
		snapshot,
	)
	if !IsCode(err, "CORRUPT_JOURNAL") {
		t.Fatalf("duplicate active attentions recovery = %v, want CORRUPT_JOURNAL", err)
	}
}

// TestDoubleParkHonesty_A4_AbsentCheckpointAttentionWithActiveWorkFails pins A4:
// a human_park checkpoint whose own attention ID has no corresponding attention
// record in the journal still fails CORRUPT_JOURNAL under the narrower resolved-only
// carve-out, provided a second, different Open attention shares its work identity.
// Reported and asserted independently of A3.
func TestDoubleParkHonesty_A4_AbsentCheckpointAttentionWithActiveWorkFails(t *testing.T) {
	dispatcher := &turnRecoveryFixtureDriver{
		parkS1:    true,
		yieldKind: driver.YieldHumanConfirmation,
	}
	fixture := newProductionImplementationRecoveryFixture(t, dispatcher)
	defer fixture.workspace.Close()

	if err := fixture.store.RecordCommand(
		fixture.ctx,
		journal.Command{
			RunID:     fixture.owner.RunID,
			ReplayKey: "manifest",
			Kind:      "start",
			Payload:   fixture.manifest.raw,
			CreatedAt: fixture.now,
		},
	); err != nil {
		t.Fatal(err)
	}

	if _, _, err := fixture.service.runProductionImplementationDispatch(
		fixture.ctx,
		fixture.engine,
		fixture.owner,
		fixture.workspace,
		fixture.cycle,
		fixture.coordinates,
	); !IsCode(err, "EFFECT_PARKED") {
		t.Fatalf("human park = %v", err)
	}

	snapshot, err := fixture.store.Snapshot(
		fixture.ctx,
		fixture.owner.RunID,
	)
	if err != nil {
		t.Fatal(err)
	}

	parentID := fixture.cycle.DispatchEffect
	checkpointID := humanParkCheckpointID(parentID)

	// Checkpoint has attention ID Y (absent everywhere from attentions),
	// while the active attention in the journal has ID X.
	// Both share work identity W.
	var absentCheckpoint humanParkCheckpoint
	foundIndex := -1
	for i, cmd := range snapshot.Commands {
		if cmd.ReplayKey == checkpointID {
			foundIndex = i
			if err := json.Unmarshal(cmd.Payload, &absentCheckpoint); err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	if foundIndex == -1 {
		t.Fatalf("checkpoint command not found in snapshot: %s", checkpointID)
	}

	// Change the checkpoint's attention ID to an absent ID Y.
	absentCheckpoint.Command.Attention.Attention.ID = "absent-attention-id-y"
	mutatedPayload := mustJSON(absentCheckpoint)

	for i := range snapshot.Commands {
		if snapshot.Commands[i].ReplayKey == checkpointID {
			snapshot.Commands[i].Payload = mutatedPayload
		}
	}
	for i := range snapshot.Effects {
		if snapshot.Effects[i].ID == checkpointID {
			snapshot.Effects[i].ExpectedDigest = sha256Digest(mutatedPayload)
			snapshot.Effects[i].ResultDigest = sha256Digest(mutatedPayload)
			snapshot.Effects[i].Result = mutatedPayload
		}
	}

	_, err = fixture.service.recoverHumanParkCheckpoint(
		fixture.ctx,
		fixture.engine,
		fixture.owner,
		snapshot,
	)
	if !IsCode(err, "CORRUPT_JOURNAL") {
		t.Fatalf("absent checkpoint attention recovery = %v, want CORRUPT_JOURNAL", err)
	}
}
