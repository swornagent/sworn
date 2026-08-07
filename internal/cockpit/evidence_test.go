package cockpit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/baton"
)

// evidenceBoundStateFixture hand-builds one baton.State whose slice history
// contains exactly one real, well-formed verifier PASS receipt, without any
// Git backing: HistoryForSlice and Resolve only ever read these in-memory
// fields, so this is sufficient to prove binding behavior.
func evidenceBoundStateFixture() (baton.State, baton.ReceiptEntry) {
	attempt := int64(2)
	candidate := strings.Repeat("c", 40)
	productTree := "sha256:" + strings.Repeat("d", 64)
	checks := "sha256:" + strings.Repeat("e", 64)
	receiptOID := strings.Repeat("f", 40)
	pass := baton.ReceiptEntry{
		OID: receiptOID,
		Receipt: baton.Receipt{
			Role: "verifier", Result: "pass", Release: "release-1",
			Attempt: &attempt, Candidate: &candidate,
			ProductTree: &productTree, Checks: &checks,
		},
	}
	state := baton.State{
		Release: "release-1",
		SliceHistories: []baton.SliceHistoryState{
			{
				Slice: "S1", Track: "T1",
				History: baton.SliceHistory{Entries: []baton.ReceiptEntry{pass}},
			},
		},
	}
	return state, pass
}

func writeEvidenceBundleFixture(
	t *testing.T, root, release, sliceID, bundleName string,
	pass baton.ReceiptEntry, itemPath string, itemBody []byte, mutate func(map[string]any),
) {
	t.Helper()
	dir := filepath.Join(root, evidenceRoot, release, sliceID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	value := map[string]any{
		"schema_version": baton.EvidenceBundleVersion,
		"release":        release, "slice": sliceID,
		"attempt": *pass.Receipt.Attempt, "candidate": *pass.Receipt.Candidate,
		"product_tree": *pass.Receipt.ProductTree, "checks": *pass.Receipt.Checks,
		"result": pass.Receipt.Result, "receipt": pass.OID,
		"items": []any{
			map[string]any{
				"path": itemPath, "kind": "screenshot",
				"digest": baton.DigestBytes(itemBody), "bytes": int64(len(itemBody)),
			},
		},
	}
	if mutate != nil {
		mutate(value)
	}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, bundleName), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, itemPath), itemBody, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestDiscoverBoundEvidenceAbsentDirectoryIsNormal proves a slice with no
// evidence directory at all is a normal, error-free outcome: evidence is
// optional and its absence carries no diagnostic.
func TestDiscoverBoundEvidenceAbsentDirectoryIsNormal(t *testing.T) {
	t.Parallel()
	state, _ := evidenceBoundStateFixture()
	items, err := DiscoverBoundEvidence(t.TempDir(), state, "S1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("items = %#v, want none", items)
	}
}

// TestDiscoverBoundEvidenceListsValidBundleMetadataOnly proves a real,
// correctly bound bundle's items are discovered and that only metadata --
// path, kind, digest, byte count -- is returned, never raw content.
func TestDiscoverBoundEvidenceListsValidBundleMetadataOnly(t *testing.T) {
	t.Parallel()
	state, pass := evidenceBoundStateFixture()
	root := t.TempDir()
	itemBody := []byte("real screenshot bytes")
	writeEvidenceBundleFixture(t, root, state.Release, "S1", "attempt-2.json", pass, "shot.png", itemBody, nil)

	items, err := DiscoverBoundEvidence(root, state, "S1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %#v, want exactly one", items)
	}
	item := items[0]
	if item.Bundle != "attempt-2.json" || item.Path != "shot.png" ||
		item.Kind != "screenshot" || item.Digest != baton.DigestBytes(itemBody) ||
		item.Bytes != int64(len(itemBody)) {
		t.Fatalf("item = %#v", item)
	}
}

// TestDiscoverBoundEvidenceExcludesTamperedBundleSilently proves discovery
// (list) treats a bundle whose item bytes do not match its declared digest
// as simply absent -- no error, no items -- rather than failing the whole
// listing or fabricating a partial result.
func TestDiscoverBoundEvidenceExcludesTamperedBundleSilently(t *testing.T) {
	t.Parallel()
	state, pass := evidenceBoundStateFixture()
	root := t.TempDir()
	writeEvidenceBundleFixture(
		t, root, state.Release, "S1", "tampered.json", pass, "shot.png",
		[]byte("original bytes"), nil,
	)
	// Overwrite the item after the bundle declared its digest, simulating
	// tampering discovered on a later read.
	if err := os.WriteFile(
		filepath.Join(root, evidenceRoot, state.Release, "S1", "shot.png"),
		[]byte("tampered bytes"), 0o644,
	); err != nil {
		t.Fatal(err)
	}

	items, err := DiscoverBoundEvidence(root, state, "S1")
	if err != nil {
		t.Fatalf("discovery must not error on a tampered bundle: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("items = %#v, want none for a tampered bundle", items)
	}
}

// TestReadBoundEvidenceRejectsTamperedBundleOnExplicitRead proves that,
// unlike silent discovery exclusion, an explicit read of one named bundle
// fails closed with the real rejection reason and never mutates state.
func TestReadBoundEvidenceRejectsTamperedBundleOnExplicitRead(t *testing.T) {
	t.Parallel()
	state, pass := evidenceBoundStateFixture()
	root := t.TempDir()
	writeEvidenceBundleFixture(
		t, root, state.Release, "S1", "tampered.json", pass, "shot.png",
		[]byte("original bytes"), nil,
	)
	if err := os.WriteFile(
		filepath.Join(root, evidenceRoot, state.Release, "S1", "shot.png"),
		[]byte("tampered bytes"), 0o644,
	); err != nil {
		t.Fatal(err)
	}

	before := state
	_, err := ReadBoundEvidence(root, state, "S1", "tampered.json")
	if err == nil {
		t.Fatal("expected explicit read of a tampered bundle to fail closed")
	}
	if baton.ErrorCode(err) != "STALE_BINDING" {
		t.Fatalf("code = %q, want STALE_BINDING", baton.ErrorCode(err))
	}
	if before.Release != state.Release || len(before.SliceHistories) != len(state.SliceHistories) {
		t.Fatal("explicit read mutated state")
	}
}

// TestReadBoundEvidenceRejectsStaleBinding proves an explicit read fails
// closed when the bundle's own declared binding (not the item bytes) has
// gone stale against the receipt it names, e.g. a bundle written for one
// attempt but the recorded receipt has since moved to another.
func TestReadBoundEvidenceRejectsStaleBinding(t *testing.T) {
	t.Parallel()
	state, pass := evidenceBoundStateFixture()
	root := t.TempDir()
	itemBody := []byte("real screenshot bytes")
	writeEvidenceBundleFixture(
		t, root, state.Release, "S1", "stale.json", pass, "shot.png", itemBody,
		func(value map[string]any) { value["result"] = "fail" },
	)

	_, err := ReadBoundEvidence(root, state, "S1", "stale.json")
	if baton.ErrorCode(err) != "STALE_BINDING" {
		t.Fatalf("code = %q (%v), want STALE_BINDING", baton.ErrorCode(err), err)
	}
}

// TestEvidenceAbsenceDoesNotAffectGraphControlsOrStatus proves that
// projecting a slice with no evidence directory present leaves its node
// state, stage, and the overall snapshot's Actions exactly as they would be
// without evidence ever being consulted.
func TestEvidenceAbsenceDoesNotAffectGraphControlsOrStatus(t *testing.T) {
	t.Parallel()
	state := baton.State{
		Release: "release-1",
		Tracks: []baton.TrackState{
			{
				ID: "T1",
				Slices: []*baton.SliceState{
					{
						Location: baton.SliceLocation{
							Track: baton.Track{ID: "T1"},
							Slice: baton.Slice{ID: "S1"},
						},
						Stage: "implement", Status: "ready", NextRole: "implementer",
						Outcome: "none",
					},
				},
			},
		},
		Assembly: baton.AssemblyState{Stage: "verify", Status: "waiting", NextRole: "none", Outcome: "none"},
	}
	graph := projectGraph(state, "running", nil)
	var slice *Node
	for index := range graph.Nodes {
		if graph.Nodes[index].ID == "slice:S1" {
			slice = &graph.Nodes[index]
		}
	}
	if slice == nil {
		t.Fatal("missing slice:S1 node")
	}
	if len(slice.BoundEvidence) != 0 {
		t.Fatalf("bound_evidence = %#v, want none (no evidence directory exists)", slice.BoundEvidence)
	}
	if slice.State != "ready" || !slice.HasBaton || slice.NextResponsibility != "implementer" {
		t.Fatalf("evidence absence altered slice projection: %#v", slice)
	}
}
