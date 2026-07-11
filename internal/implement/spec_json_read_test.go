package implement

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/state"
)

// specJSONMarker is a distinctive user_outcome only present in the spec.json
// fixture, so a test can prove the prompt was built from spec.json rather than
// the spec.md body.
const specJSONMarker = "SPECJSON-OUTCOME-MARKER: the engine reads spec.json"

// specJSONFixture returns a valid spec-v1 spec.json for the S06-test-slice
// fixture. withTestRefs adds an AC test_refs array (used by the AC-03
// byte-equality regression).
func specJSONFixture(withTestRefs bool) string {
	acExtra := ""
	if withTestRefs {
		acExtra = `, "test_refs": ["internal/implement/spec_json_read_test.go:TestRun_PlannerSpecJSON_ByteUnchanged"]`
	}
	return `{
  "$schema": "https://baton.sawy3r.net/schemas/spec-v1.json",
  "slice_id": "S06-test-slice",
  "release": "2026-06-15-test",
  "user_outcome": "` + specJSONMarker + `.",
  "covers_needs": ["N-01"],
  "in_scope": ["Read the machine contract from spec.json"],
  "out_of_scope": ["Deleting spec.md support"],
  "acceptance_criteria": [
    {"id": "AC-01", "text": "WHEN spec.json exists, THE engine SHALL read it (N-01).", "ears_pattern": "event-driven"` + acExtra + `}
  ]
}
`
}

// AC-01: the engine implement leg runs on a spec.json-only slice (no spec.md)
// and builds the implementer prompt from spec.json without the sworn#97
// spec.md-missing error, then transitions to implemented.
func TestRun_SpecJSONOnly_ReadsSpecJSON(t *testing.T) {
	workspaceRoot, specPath, _ := setupTempRepo(t)
	sliceDir := filepath.Dir(specPath)

	// Convert to a spec.json-ONLY slice: remove spec.md, author spec.json.
	if err := os.Remove(specPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sliceDir, "spec.json"), []byte(specJSONFixture(false)), 0o644); err != nil {
		t.Fatal(err)
	}

	fa := &fakeImplDriver{t: t, effect: writeFile("hello.txt", "hello world")}
	// specPath still names spec.md (now absent) — Run resolves spec.json from
	// the slice directory via spec.LoadSpec.
	if _, err := Run(context.Background(), workspaceRoot, specPath, "", fa, "fake/model", 0); err != nil {
		if strings.Contains(err.Error(), "spec.md") {
			t.Fatalf("sworn#97 regression — implement leg hard-failed on missing spec.md: %v", err)
		}
		t.Fatalf("Run() on spec.json-only slice: %v", err)
	}

	// Prompt was built from spec.json (carries the spec.json-only marker).
	if !strings.Contains(fa.lastUserPrompt, specJSONMarker) {
		t.Fatalf("implementer prompt not built from spec.json:\n%s", fa.lastUserPrompt)
	}

	// State advanced to implemented.
	st, err := state.Read(filepath.Join(sliceDir, "status.json"))
	if err != nil {
		t.Fatal(err)
	}
	if st.State != state.Implemented {
		t.Fatalf("expected implemented, got %q", st.State)
	}
}

// AC-02 (json-authoritative): with BOTH spec.json and spec.md present and
// differing, spec.json wins — the prompt carries the spec.json outcome, not
// the spec.md body.
func TestRun_SpecJSONAuthoritative_OverSpecMD(t *testing.T) {
	workspaceRoot, specPath, _ := setupTempRepo(t) // spec.md says "Write a hello world file..."
	sliceDir := filepath.Dir(specPath)

	if err := os.WriteFile(filepath.Join(sliceDir, "spec.json"), []byte(specJSONFixture(false)), 0o644); err != nil {
		t.Fatal(err)
	}

	fa := &fakeImplDriver{t: t, effect: writeFile("hello.txt", "hello world")}
	if _, err := Run(context.Background(), workspaceRoot, specPath, "", fa, "fake/model", 0); err != nil {
		t.Fatalf("Run(): %v", err)
	}

	if !strings.Contains(fa.lastUserPrompt, specJSONMarker) {
		t.Fatalf("prompt should carry the spec.json outcome (authoritative):\n%s", fa.lastUserPrompt)
	}
	if strings.Contains(fa.lastUserPrompt, "Write a hello world file and verify it exists") {
		t.Fatalf("prompt used spec.md body despite spec.json being present (not authoritative):\n%s", fa.lastUserPrompt)
	}
}

// AC-02 (md-legacy-fallback): a legacy slice with spec.md and no spec.json
// still reads via the spec.md fallback — the prompt carries the spec.md body.
func TestRun_SpecMDLegacyFallback(t *testing.T) {
	workspaceRoot, specPath, _ := setupTempRepo(t) // spec.md only, no spec.json
	sliceDir := filepath.Dir(specPath)
	if _, err := os.Stat(filepath.Join(sliceDir, "spec.json")); err == nil {
		t.Fatal("fixture should not have a spec.json")
	}

	fa := &fakeImplDriver{t: t, effect: writeFile("hello.txt", "hello world")}
	if _, err := Run(context.Background(), workspaceRoot, specPath, "", fa, "fake/model", 0); err != nil {
		t.Fatalf("Run() on legacy spec.md slice: %v", err)
	}

	if !strings.Contains(fa.lastUserPrompt, "Write a hello world file and verify it exists") {
		t.Fatalf("prompt should carry the spec.md body on a legacy slice:\n%s", fa.lastUserPrompt)
	}
}

// AC-03: a planner-authored spec.json survives an implement run byte-for-byte —
// WriteSpecRecord validates rather than regenerating from spec.md, so
// ears_pattern/test_refs are never lost (R-02 regression).
func TestRun_PlannerSpecJSON_ByteUnchanged(t *testing.T) {
	workspaceRoot, specPath, _ := setupTempRepo(t)
	sliceDir := filepath.Dir(specPath)

	// A planner-authored spec.json carrying ears_pattern + test_refs, no spec.md.
	if err := os.Remove(specPath); err != nil {
		t.Fatal(err)
	}
	jsonPath := filepath.Join(sliceDir, "spec.json")
	authored := []byte(specJSONFixture(true))
	if err := os.WriteFile(jsonPath, authored, 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatal(err)
	}

	fa := &fakeImplDriver{t: t, effect: writeFile("hello.txt", "hello world")}
	if _, err := Run(context.Background(), workspaceRoot, specPath, "", fa, "fake/model", 0); err != nil {
		t.Fatalf("Run(): %v", err)
	}

	after, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("spec.json was rewritten by the implement run (R-02 regression)\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if !bytes.Contains(after, []byte("test_refs")) {
		t.Fatal("planner spec.json lost its test_refs — regenerated from spec.md")
	}
}
