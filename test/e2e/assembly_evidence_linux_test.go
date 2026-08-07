//go:build linux

package e2e

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/journal"
	swornruntime "github.com/swornagent/sworn/internal/runtime"
)

// A5. The assembly's identity must be recorded, and the recording must be one
// a wrong value cannot survive.
//
// Two halves, and this slice owns both of them here:
//
//  1. The inventory. Every value A5 names -- the assembly commit and tree, the
//     binary digest, the plan and slice identities, the predecessor products,
//     the runtime schema, the journal event cursor, and the final target
//     equality -- is derived from the real repository, the real journal and
//     the real binary by assemblyInventoryFrom, and then re-derived and
//     compared field by field. Nothing is transcribed.
//
//  2. The binding. The inventory is published as a sworn.evidence-bundle/v1
//     item and the bundle is bound to a verifier receipt that really exists in
//     the release's slice history, then admitted through the product's own
//     ParseEvidenceBundle and Resolve. Resolve is what makes this fail
//     closed, and the falsifiers at the end prove it does: a tampered
//     inventory byte, a wrong product tree, and a non-verifier receipt are
//     each refused.
//
// The contract's gate list (product, serial E2E, race, vet, diff,
// legacy-compatibility and the fresh semantic journey) is executed against the
// assembled release at the release step, not from inside a test that is itself
// one of those gates. What is proven here is the candidate half: that the
// exact assembly can be inventoried and bound, and that a false inventory
// cannot be.

// assemblyInventoryVersion identifies the inventory document shape.
const assemblyInventoryVersion = "sworn.assembly-inventory/v1"

// assemblyInventory is the exact identity of one merged assembly.
type assemblyInventory struct {
	SchemaVersion  string            `json:"schema_version"`
	Release        string            `json:"release"`
	AssemblyCommit string            `json:"assembly_commit"`
	AssemblyTree   string            `json:"assembly_tree"`
	BinaryDigest   string            `json:"binary_digest"`
	PlanDigest     string            `json:"plan_digest"`
	PlanSchema     string            `json:"plan_schema"`
	SliceContracts map[string]string `json:"slice_contracts"`
	Predecessors   map[string]string `json:"predecessor_product_trees"`
	RuntimeSchema  string            `json:"runtime_manifest_schema"`
	EventCursor    int64             `json:"event_cursor"`
	TargetRef      string            `json:"target_ref"`
	TargetBefore   string            `json:"target_before"`
	TargetAfter    string            `json:"target_after"`
	TargetEqual    bool              `json:"target_equals_assembly_result"`
}

// assemblyInventoryFrom derives the inventory from real state. It is the only
// place any of these values is produced, so recording and re-checking cannot
// drift apart: the test below calls it twice and compares.
func assemblyInventoryFrom(
	t *testing.T,
	repository, release, journalPath, runID, binary, targetBefore string,
) assemblyInventory {
	t.Helper()
	state := readBatonState(t, repository, release)
	if state.Assembly.Outcome != "merged" || state.Assembly.ResultCommit == "" {
		t.Fatalf("inventory needs a merged assembly, got %#v", state.Assembly)
	}
	commit := state.Assembly.ResultCommit
	contracts := map[string]string{}
	if len(state.Plan.History) == 0 {
		t.Fatal("installed plan has no history")
	}
	plan := state.Plan.History[len(state.Plan.History)-1].Plan
	for _, track := range state.Plan.Metadata.Tracks {
		for _, slice := range track.Slices {
			digest, present := plan.Contract(slice.ID)
			if !present {
				t.Fatalf("slice %s has no contract identity", slice.ID)
			}
			contracts[slice.ID] = digest
		}
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
	var cursor int64
	for _, event := range snapshot.Events {
		if event.Offset > cursor {
			cursor = event.Offset
		}
	}
	if cursor == 0 {
		t.Fatal("the run recorded no events")
	}
	target := runGit(t, repository, "rev-parse", "main")
	predecessors := map[string]string{}
	for _, receipt := range kernelReceiptsFromHistory(t, kernelReleaseName) {
		if receipt.Role == "verifier" && receipt.Result == "pass" &&
			receipt.ProductTree != "" {
			predecessors[receipt.Slice] = receipt.ProductTree
		}
	}
	return assemblyInventory{
		SchemaVersion:  assemblyInventoryVersion,
		Release:        release,
		AssemblyCommit: commit,
		AssemblyTree:   runGit(t, repository, "rev-parse", commit+"^{tree}"),
		BinaryDigest:   fileDigest(t, binary),
		PlanDigest:     state.Plan.Digest,
		PlanSchema:     state.Plan.Metadata.SchemaVersion,
		SliceContracts: contracts,
		Predecessors:   predecessors,
		RuntimeSchema:  swornruntime.ManifestVersion,
		EventCursor:    cursor,
		TargetRef:      state.Plan.Metadata.TargetRef,
		TargetBefore:   targetBefore,
		TargetAfter:    target,
		TargetEqual:    target == commit,
	}
}

// TestRealBinaryAssemblyEvidenceRecordResolvesAndFailsClosed is A5.
func TestRealBinaryAssemblyEvidenceRecordResolvesAndFailsClosed(t *testing.T) {
	t.Parallel()
	buildRoot := t.TempDir()
	fakeBinary := filepath.Join(buildRoot, "e2e-fake")
	buildBinary(t, fakeBinary, "./test/e2e/testdata/fake", "")
	fakeDigest := fileDigest(t, fakeBinary)
	swornBinary := filepath.Join(buildRoot, "sworn")
	buildBinary(t, swornBinary, "./cmd/sworn", "")

	repository := newProductRepository(t)
	runRoot := t.TempDir()
	journalPath := filepath.Join(runRoot, "run.sqlite")
	const (
		runID   = "e2e-assembly-evidence"
		release = "e2e-assembly-evidence-release"
	)
	manifestBody, planBytes, plan := e2eManifest(
		t, runID, repository, release, fakeBinary, fakeDigest, "verifier-model",
	)
	manifestPath := writeManifest(t, runRoot, manifestBody)
	targetBefore := runGit(t, repository, "rev-parse", "main")

	runBinary(
		t, swornBinary, 0,
		"run", "--manifest", manifestPath, "--journal", journalPath,
	)
	authorizePlan(t, journalPath, runID, plan)
	installAndPassComponent(t, repository, release, planBytes)
	stdout, stderr := runBinary(
		t, swornBinary, 0,
		"resume", "--run", runID, "--journal", journalPath,
		"--command", "evidence-resume-1", "--generation", "0",
	)
	if stderr != "" || !strings.Contains(stdout, "  state: complete") {
		t.Fatalf("assembly evidence run stdout=%q stderr=%q", stdout, stderr)
	}

	inventory := assemblyInventoryFrom(
		t, repository, release, journalPath, runID, swornBinary, targetBefore,
	)
	if !inventory.TargetEqual || inventory.AssemblyCommit == targetBefore ||
		inventory.RuntimeSchema != swornruntime.ManifestVersion ||
		len(inventory.SliceContracts) == 0 {
		t.Fatalf("inventory = %#v", inventory)
	}

	// The recording is executable: re-derive every value from the same real
	// state and require an exact match. A transcribed value that drifted from
	// the product would fail here.
	recomputed := assemblyInventoryFrom(
		t, repository, release, journalPath, runID, swornBinary, targetBefore,
	)
	first, err := json.Marshal(inventory)
	if err != nil {
		t.Fatal(err)
	}
	second, err := json.Marshal(recomputed)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("inventory is not reproducible:\n%s\n%s", first, second)
	}
	// Every value the contract names is present and derived, not blank.
	for name, value := range map[string]string{
		"assembly_commit":         inventory.AssemblyCommit,
		"assembly_tree":           inventory.AssemblyTree,
		"binary_digest":           inventory.BinaryDigest,
		"plan_digest":             inventory.PlanDigest,
		"plan_schema":             inventory.PlanSchema,
		"runtime_manifest_schema": inventory.RuntimeSchema,
		"target_ref":              inventory.TargetRef,
		"target_before":           inventory.TargetBefore,
		"target_after":            inventory.TargetAfter,
	} {
		if strings.TrimSpace(value) == "" {
			t.Fatalf("inventory field %s is empty", name)
		}
	}
	// The binary digest names the exact binary that produced this assembly.
	if inventory.BinaryDigest != fileDigest(t, swornBinary) {
		t.Fatal("inventory binary digest does not name the binary that ran")
	}

	// Publish the inventory as a real evidence item and bind the bundle to a
	// real verifier receipt in this release's slice history.
	state := readBatonState(t, repository, release)
	slice, ok := state.Slice("S1")
	if !ok || slice.Pass == nil {
		t.Fatalf("slice S1 has no PASS to bind to: %#v", slice)
	}
	pass := slice.Pass
	inventoryBytes, err := json.MarshalIndent(inventory, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	inventoryBytes = append(inventoryBytes, '\n')
	const itemPath = "evidence/assembly-inventory.json"
	itemFile := filepath.Join(runRoot, "assembly-inventory.json")
	if err := os.WriteFile(itemFile, inventoryBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	onDisk, err := os.ReadFile(itemFile)
	if err != nil {
		t.Fatal(err)
	}

	bundleBytes := assemblyEvidenceBundleBytes(
		t, release, "S1", pass, itemPath, onDisk, nil,
	)
	bundle, err := baton.ParseEvidenceBundle(bundleBytes)
	if err != nil {
		t.Fatalf("evidence bundle did not admit: %v", err)
	}
	resolved, err := bundle.Resolve(state, map[string][]byte{itemPath: onDisk})
	if err != nil {
		t.Fatalf("evidence bundle did not resolve: %v", err)
	}
	if resolved.OID != pass.OID || resolved.Receipt.Role != "verifier" ||
		resolved.Receipt.Result != "pass" {
		t.Fatalf("resolved receipt = %#v", resolved.Receipt)
	}
	if bundle.Digest() != baton.DigestBytes(bundleBytes) {
		t.Fatal("bundle digest does not name its own bytes")
	}
	items := bundle.Items()
	if len(items) != 1 || items[0].Path != itemPath ||
		items[0].Bytes != int64(len(onDisk)) {
		t.Fatalf("bundle items = %#v", items)
	}

	// Falsifiers. Each of these is a way the record could lie; each must be
	// refused, or the record proves nothing.
	t.Run("tampered_inventory_is_refused", func(t *testing.T) {
		tampered := append([]byte(nil), onDisk...)
		tampered = []byte(strings.Replace(
			string(tampered), inventory.AssemblyCommit,
			strings.Repeat("0", len(inventory.AssemblyCommit)), 1,
		))
		if string(tampered) == string(onDisk) {
			t.Fatal("the tamper did not change the inventory")
		}
		if _, err := bundle.Resolve(
			state, map[string][]byte{itemPath: tampered},
		); baton.ErrorCode(err) != "STALE_BINDING" {
			t.Fatalf("tampered inventory resolved: %v", err)
		}
	})
	t.Run("wrong_product_tree_is_refused", func(t *testing.T) {
		wrong := assemblyEvidenceBundleBytes(
			t, release, "S1", pass, itemPath, onDisk,
			map[string]any{
				"product_tree": "sha256:" + strings.Repeat("0", 64),
			},
		)
		parsed, err := baton.ParseEvidenceBundle(wrong)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := parsed.Resolve(
			state, map[string][]byte{itemPath: onDisk},
		); baton.ErrorCode(err) != "STALE_BINDING" {
			t.Fatalf("wrong product tree resolved: %v", err)
		}
	})
	t.Run("non_verifier_receipt_is_refused", func(t *testing.T) {
		var candidate *baton.ReceiptEntry
		for index := range slice.History.Entries {
			entry := &slice.History.Entries[index]
			if entry.Receipt.Role == "implementer" &&
				entry.Receipt.Result == "candidate" {
				candidate = entry
			}
		}
		if candidate == nil {
			t.Fatal("slice S1 has no candidate receipt to mis-bind to")
		}
		wrong := assemblyEvidenceBundleBytes(
			t, release, "S1", pass, itemPath, onDisk,
			map[string]any{"receipt": candidate.OID},
		)
		parsed, err := baton.ParseEvidenceBundle(wrong)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := parsed.Resolve(
			state, map[string][]byte{itemPath: onDisk},
		); baton.ErrorCode(err) != "STALE_BINDING" {
			t.Fatalf("non-verifier receipt resolved: %v", err)
		}
	})
	t.Run("missing_item_is_refused", func(t *testing.T) {
		if _, err := bundle.Resolve(
			state, map[string][]byte{},
		); baton.ErrorCode(err) != "EVIDENCE_ITEM_MISSING" {
			t.Fatalf("missing item resolved: %v", err)
		}
	})
}

// assemblyEvidenceBundleBytes builds one sworn.evidence-bundle/v1 document
// from a real verifier receipt, optionally overriding named fields so a
// falsifier can state exactly one wrong thing.
func assemblyEvidenceBundleBytes(
	t *testing.T,
	release, slice string,
	pass *baton.ReceiptEntry,
	itemPath string,
	itemBytes []byte,
	overrides map[string]any,
) []byte {
	t.Helper()
	if pass.Receipt.Attempt == nil || pass.Receipt.Candidate == nil ||
		pass.Receipt.ProductTree == nil || pass.Receipt.Checks == nil {
		t.Fatalf("verifier receipt is not fully bound: %#v", pass.Receipt)
	}
	value := map[string]any{
		"schema_version": baton.EvidenceBundleVersion,
		"release":        release,
		"slice":          slice,
		"attempt":        *pass.Receipt.Attempt,
		"candidate":      *pass.Receipt.Candidate,
		"product_tree":   *pass.Receipt.ProductTree,
		"checks":         *pass.Receipt.Checks,
		"result":         pass.Receipt.Result,
		"receipt":        pass.OID,
		"items": []any{map[string]any{
			"path":   itemPath,
			"kind":   "command-output",
			"digest": baton.DigestBytes(itemBytes),
			"bytes":  int64(len(itemBytes)),
		}},
	}
	keys := make([]string, 0, len(overrides))
	for key := range overrides {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if _, known := value[key]; !known {
			t.Fatalf("override names an unknown bundle field %q", key)
		}
		value[key] = overrides[key]
	}
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(body, '\n')
}
