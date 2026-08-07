package baton

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func evidenceBundleValue(
	release, slice string, attempt int64,
	candidate, productTree, checks, result, receipt string,
	items []any,
) map[string]any {
	return map[string]any{
		"schema_version": EvidenceBundleVersion,
		"release":        release, "slice": slice, "attempt": attempt,
		"candidate": candidate, "product_tree": productTree, "checks": checks,
		"result": result, "receipt": receipt, "items": items,
	}
}

func evidenceItemValue(path, kind, digest string, byteCount int) map[string]any {
	return map[string]any{"path": path, "kind": kind, "digest": digest, "bytes": int64(byteCount)}
}

func evidenceRaw(t *testing.T, value map[string]any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// evidencePassFixture drives one real slice through designed -> proceed ->
// candidate -> verifier pass using the same golden action harness the rest
// of this package's tests use, then returns the resulting State, the real
// verifier PASS ReceiptEntry for slice S1, and the repository path.
func evidencePassFixture(t *testing.T) (State, ReceiptEntry, string) {
	t.Helper()
	repoPath, repository, actions := createActionHarness(t)
	release := "evidence-release"
	result, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: actionPlanBytes(release), Summary: "Approve.", Detail: []byte("approval"),
	})
	if err != nil || !result.Changed {
		t.Fatalf("record plan revision: result = %#v, err = %v", result, err)
	}
	advanceActionSlice(t, actions, repoPath, release, "T1", "S1", "one.txt", 1_000_000_100, "pass")
	state := readActionState(t, repository, release)
	history, ok := state.HistoryForSlice("S1")
	if !ok {
		t.Fatal("missing S1 history")
	}
	var pass *ReceiptEntry
	for index := range history.History.Entries {
		entry := history.History.Entries[index]
		if entry.Receipt.Role == "verifier" && entry.Receipt.Result == "pass" {
			pass = &history.History.Entries[index]
		}
	}
	if pass == nil {
		t.Fatal("missing real verifier pass receipt")
	}
	return state, *pass, repoPath
}

func validEvidenceBundleValue(state State, pass ReceiptEntry, itemPath string, itemBody []byte) map[string]any {
	return evidenceBundleValue(
		state.Release, "S1", *pass.Receipt.Attempt, *pass.Receipt.Candidate,
		*pass.Receipt.ProductTree, *pass.Receipt.Checks, pass.Receipt.Result, pass.OID,
		[]any{evidenceItemValue(itemPath, "screenshot", DigestBytes(itemBody), len(itemBody))},
	)
}

func TestEvidenceBundleResolveBindsToRealVerifierPass(t *testing.T) {
	t.Parallel()
	state, pass, _ := evidencePassFixture(t)
	itemBody := []byte("screenshot-bytes")
	bundle, err := ParseEvidenceBundle(evidenceRaw(t, validEvidenceBundleValue(state, pass, "evidence/S1/one.png", itemBody)))
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Items()) != 1 || bundle.Items()[0].Path != "evidence/S1/one.png" {
		t.Fatalf("items = %#v", bundle.Items())
	}
	resolved, err := bundle.Resolve(state, map[string][]byte{"evidence/S1/one.png": itemBody})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved.OID != pass.OID {
		t.Fatalf("resolved OID = %q, want %q", resolved.OID, pass.OID)
	}
}

func TestEvidenceBundleResolveRejectsBindingMismatches(t *testing.T) {
	t.Parallel()
	state, pass, _ := evidencePassFixture(t)
	itemBody := []byte("screenshot-bytes")
	itemPath := "evidence/S1/one.png"

	// A real, but wrong-role, receipt from the same slice's history proves
	// Resolve rejects a syntactically valid receipt identity that does not
	// name a verifier receipt.
	history, ok := state.HistoryForSlice("S1")
	if !ok {
		t.Fatal("missing S1 history")
	}
	var candidateReceipt *ReceiptEntry
	for index := range history.History.Entries {
		entry := history.History.Entries[index]
		if entry.Receipt.Role == "implementer" && entry.Receipt.Result == "candidate" {
			candidateReceipt = &history.History.Entries[index]
		}
	}
	if candidateReceipt == nil {
		t.Fatal("missing real implementer candidate receipt")
	}
	unknownReceipt := flipFirstHexChar(pass.OID)
	placeholderCandidate := strings.Repeat("a", len(*pass.Receipt.Candidate))
	if placeholderCandidate == *pass.Receipt.Candidate {
		placeholderCandidate = strings.Repeat("b", len(*pass.Receipt.Candidate))
	}
	placeholderDigest := "sha256:" + strings.Repeat("c", 64)

	cases := []struct {
		name   string
		mutate func(map[string]any)
		code   string
	}{
		{"release_mismatch", func(v map[string]any) { v["release"] = "other-release" }, "STALE_BINDING"},
		{"slice_unknown", func(v map[string]any) { v["slice"] = "GHOST" }, "SLICE_NOT_FOUND"},
		{"slice_wrong_but_real", func(v map[string]any) { v["slice"] = "S2" }, "RECEIPT_NOT_FOUND"},
		{"receipt_unknown", func(v map[string]any) { v["receipt"] = unknownReceipt }, "RECEIPT_NOT_FOUND"},
		{"receipt_wrong_role", func(v map[string]any) { v["receipt"] = candidateReceipt.OID }, "STALE_BINDING"},
		{"attempt_stale", func(v map[string]any) { v["attempt"] = *pass.Receipt.Attempt + 1 }, "STALE_BINDING"},
		{"candidate_mismatch", func(v map[string]any) { v["candidate"] = placeholderCandidate }, "STALE_BINDING"},
		{"product_tree_mismatch", func(v map[string]any) { v["product_tree"] = placeholderDigest }, "STALE_BINDING"},
		{"checks_mismatch", func(v map[string]any) { v["checks"] = placeholderDigest }, "STALE_BINDING"},
		{"verdict_mismatch", func(v map[string]any) { v["result"] = "fail" }, "STALE_BINDING"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			value := validEvidenceBundleValue(state, pass, itemPath, itemBody)
			test.mutate(value)
			bundle, err := ParseEvidenceBundle(evidenceRaw(t, value))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			_, err = bundle.Resolve(state, map[string][]byte{itemPath: itemBody})
			if code := ErrorCode(err); code != test.code {
				t.Fatalf("code = %q (%v), want %q", code, err, test.code)
			}
		})
	}
}

func flipFirstHexChar(value string) string {
	if len(value) == 0 {
		return value
	}
	replacement := byte('a')
	if value[0] == 'a' {
		replacement = 'b'
	}
	return string(replacement) + value[1:]
}

func TestEvidenceBundleResolveRejectsItemMismatches(t *testing.T) {
	t.Parallel()
	state, pass, _ := evidencePassFixture(t)
	itemPath := "evidence/S1/one.png"
	itemBody := []byte("screenshot-bytes")
	value := validEvidenceBundleValue(state, pass, itemPath, itemBody)
	bundle, err := ParseEvidenceBundle(evidenceRaw(t, value))
	if err != nil {
		t.Fatal(err)
	}

	t.Run("digest_mismatch", func(t *testing.T) {
		_, err := bundle.Resolve(state, map[string][]byte{itemPath: []byte("tampered-bytes")})
		if code := ErrorCode(err); code != "STALE_BINDING" {
			t.Fatalf("code = %q (%v), want STALE_BINDING", code, err)
		}
	})
	t.Run("item_missing", func(t *testing.T) {
		_, err := bundle.Resolve(state, map[string][]byte{})
		if code := ErrorCode(err); code != "EVIDENCE_ITEM_MISSING" {
			t.Fatalf("code = %q (%v), want EVIDENCE_ITEM_MISSING", code, err)
		}
	})
}

func TestParseEvidenceBundleRejectsUnsafeAndAmbiguousStructure(t *testing.T) {
	t.Parallel()
	placeholderDigest := "sha256:" + strings.Repeat("a", 64)
	placeholderOID := strings.Repeat("a", 40)
	base := func() map[string]any {
		return evidenceBundleValue(
			"release-1", "S1", int64(1), placeholderOID, placeholderDigest, placeholderDigest,
			"pass", placeholderOID,
			[]any{evidenceItemValue("evidence/one.png", "screenshot", placeholderDigest, 1)},
		)
	}

	cases := []struct {
		name   string
		mutate func(map[string]any)
		code   string
	}{
		{"path_escape", func(v map[string]any) {
			v["items"] = []any{evidenceItemValue("../escape.png", "screenshot", placeholderDigest, 1)}
		}, "INVALID_PATH"},
		{"path_absolute", func(v map[string]any) {
			v["items"] = []any{evidenceItemValue("/escape.png", "screenshot", placeholderDigest, 1)}
		}, "INVALID_PATH"},
		{"path_reserved_root", func(v map[string]any) {
			v["items"] = []any{evidenceItemValue(RecordRoot+"/evidence.png", "screenshot", placeholderDigest, 1)}
		}, "RESERVED_RECORD_ROOT"},
		{"duplicate_path", func(v map[string]any) {
			v["items"] = []any{
				evidenceItemValue("evidence/one.png", "screenshot", placeholderDigest, 1),
				evidenceItemValue("evidence/one.png", "trace", placeholderDigest, 1),
			}
		}, "DUPLICATE_IDENTITY"},
		{"unknown_kind", func(v map[string]any) {
			v["items"] = []any{evidenceItemValue("evidence/one.mp4", "video", placeholderDigest, 1)}
		}, "INVALID_FIELD"},
		{"unknown_schema", func(v map[string]any) { v["schema_version"] = "sworn.evidence-bundle/v2" }, "INVALID_FIELD"},
		{"invalid_result", func(v map[string]any) { v["result"] = "approved" }, "INVALID_FIELD"},
		{"empty_items", func(v map[string]any) { v["items"] = []any{} }, "INVALID_FIELD"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			value := base()
			test.mutate(value)
			_, err := ParseEvidenceBundle(evidenceRaw(t, value))
			if code := ErrorCode(err); code != test.code {
				t.Fatalf("code = %q (%v), want %q", code, err, test.code)
			}
		})
	}
}

// TestEvidenceBundleResolveDoesNotMutateRepository proves both a successful
// and a rejected Resolve leave every ref in the repository byte-identical:
// evidence reading has no lifecycle authority and no side effects.
func TestEvidenceBundleResolveDoesNotMutateRepository(t *testing.T) {
	t.Parallel()
	state, pass, repoPath := evidencePassFixture(t)
	before := actionGit(t, repoPath, nil, nil, "for-each-ref")

	itemPath := "evidence/S1/one.png"
	itemBody := []byte("screenshot-bytes")
	goodBundle, err := ParseEvidenceBundle(evidenceRaw(t, validEvidenceBundleValue(state, pass, itemPath, itemBody)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := goodBundle.Resolve(state, map[string][]byte{itemPath: itemBody}); err != nil {
		t.Fatalf("resolve: %v", err)
	}

	badValue := validEvidenceBundleValue(state, pass, itemPath, itemBody)
	badValue["result"] = "fail"
	badBundle, err := ParseEvidenceBundle(evidenceRaw(t, badValue))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := badBundle.Resolve(state, map[string][]byte{itemPath: itemBody}); err == nil {
		t.Fatal("expected a rejected resolve")
	}

	after := actionGit(t, repoPath, nil, nil, "for-each-ref")
	if before != after {
		t.Fatalf("repository refs changed after evidence resolve:\nbefore=%q\nafter=%q", before, after)
	}
}

// TestEvidenceBundleNeverReferencedByAdmissionOrDispatch is a structural
// assertion, not a behavioral one: it proves plan admission, candidate
// scope validation, state derivation, and the runtime dispatch/scheduler
// owners contain no source reference to the evidence bundle API, so its
// absence can never affect plan parsing, approval, dispatch, implementation,
// verification, Merge, or status projection.
func TestEvidenceBundleNeverReferencedByAdmissionOrDispatch(t *testing.T) {
	t.Parallel()
	forbidden := []string{"EvidenceBundle", "ParseEvidenceBundle"}
	guarded := []string{
		"plan.go", "actions.go", "candidate.go", "state.go", "catalog.go",
		filepath.Join("..", "runtime", "scheduler.go"),
		filepath.Join("..", "runtime", "dispatch.go"),
		filepath.Join("..", "runtime", "captain_delegation.go"),
		filepath.Join("..", "driver", "submission.go"),
	}
	for _, path := range guarded {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, symbol := range forbidden {
			if strings.Contains(string(body), symbol) {
				t.Fatalf("%s references %s; evidence bundle reads must never gate admission or dispatch", path, symbol)
			}
		}
	}
}
