package runtime

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/protocol"
)

func equalAnchorTokens(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestCriterionAnchorTokensParsesDeclaredClauseShapes pins A2's fixed
// extraction predicate against every clause shape this release's own
// contracts use: a single anchor, two anchors joined by "and", and a
// criterion with no anchor clause at all.
func TestCriterionAnchorTokensParsesDeclaredClauseShapes(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []string
	}{
		{
			"single anchor with trailing period",
			"Some acceptance text. Anchor: internal/runtime/host_checks_test.go.",
			[]string{"internal/runtime/host_checks_test.go"},
		},
		{
			"two anchors joined by and",
			"Some text. Anchor: internal/runtime/host_repair_test.go and internal/runtime/refusal_paths_test.go.",
			[]string{"internal/runtime/host_repair_test.go", "internal/runtime/refusal_paths_test.go"},
		},
		{
			"criterion with no anchor clause",
			"Some acceptance text with no anchor clause at all.",
			nil,
		},
		{
			"docs anchor still path-like",
			"Some text. Anchor: docs/run.md.",
			[]string{"docs/run.md"},
		},
		{
			"root-level anchor with no directory component",
			"Some text. Anchor: base.txt.",
			[]string{"base.txt"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := criterionAnchorTokens(tc.text)
			if !equalAnchorTokens(got, tc.want) {
				t.Fatalf("criterionAnchorTokens(%q) = %v, want %v", tc.text, got, tc.want)
			}
		})
	}
}

// TestResolveAnchorRequirementsFiltersToBaseTreeMembershipAndSkipsClauseless
// pins resolveAnchorRequirements' own two rules: a criterion with no anchor
// clause is never gated, and a criterion whose anchor clause names only
// paths absent from the base tree has nothing to check presence against and
// is not gated either - only the base-tree-present tokens ride the
// requirement.
func TestResolveAnchorRequirementsFiltersToBaseTreeMembershipAndSkipsClauseless(t *testing.T) {
	criteria := []protocol.Criterion{
		{ID: "A1", Text: "No anchor clause at all."},
		{ID: "A2", Text: "Some text. Anchor: tracked.go and untracked.go."},
		{ID: "A3", Text: "Some text. Anchor: only-untracked.go."},
	}
	baseTree := map[string]struct{}{"tracked.go": {}}
	requirements := resolveAnchorRequirements(criteria, baseTree)
	if len(requirements) != 1 || requirements[0].id != "A2" ||
		!equalAnchorTokens(requirements[0].paths, []string{"tracked.go"}) {
		t.Fatalf("resolveAnchorRequirements(...) = %#v", requirements)
	}
}

// TestAnchorPathInScopeMirrorsIncludeExcludeRule pins anchorPathInScope
// against the same shape of include/exclude rule the whole candidate is
// judged by: a path must sit under an included prefix and under none of the
// excluded ones.
func TestAnchorPathInScopeMirrorsIncludeExcludeRule(t *testing.T) {
	scope := protocol.Scope{
		Include: []string{"internal/runtime", "README.md"},
		Exclude: []string{"internal/runtime/generated"},
	}
	tests := []struct {
		path string
		want bool
	}{
		{"internal/runtime/host_checks.go", true},
		{"internal/runtime/generated/thing.go", false},
		{"README.md", true},
		{"internal/other/file.go", false},
		{"internal/runtimefoo/file.go", false},
	}
	for _, tc := range tests {
		if got := anchorPathInScope(scope, tc.path); got != tc.want {
			t.Fatalf("anchorPathInScope(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

// newAnchorGateEngineFixture opens a bare production-shaped engine over a
// throwaway repository, with no journal or workspace lease attached: enough
// for anchorPresenceGate's own git-only reads (ListTree, ChangedPaths), not
// a full dispatch cycle. Returns the engine and its base commit's OID
// string.
func newAnchorGateEngineFixture(t *testing.T) (*engine, string) {
	t.Helper()
	repository := productionRepository(t)
	config := productionConfig(t)
	manifest := productionManifest(t, repository, config)
	production, err := newProductionDriverRuntime(config, driver.DriverFactoryOptions{})
	if err != nil {
		t.Fatal(err)
	}
	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		t.Fatal(err)
	}
	service := &Service{production: production, gitExecutable: gitExecutable}
	eng, err := service.openEngine(manifest)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = eng.Close() })
	base := runRuntimeGit(t, repository, "rev-parse", "HEAD")
	return eng, base
}

// writeAndCommitAnchorFixture writes name/content into repository and
// commits it on top of the current HEAD, returning the new commit's OID
// string.
func writeAndCommitAnchorFixture(t *testing.T, repository, name, content string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repository, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	runRuntimeGit(t, repository, "add", "--", name)
	runRuntimeGit(
		t, repository,
		"-c", "user.name=Production Fixture",
		"-c", "user.email=production@example.invalid",
		"commit", "--quiet", "-m", "anchor gate fixture: "+name,
	)
	return runRuntimeGit(t, repository, "rev-parse", "HEAD")
}

// TestAnchorPresenceGateRefusesUntouchedAnchorAndAdmitsDirectTouch pins A2's
// core promise directly against the gate function: a candidate that touches
// none of a criterion's declared anchor files refuses ANCHOR_NOT_TOUCHED
// naming the criterion and the file, and a candidate that does touch the
// anchor clears the gate with no substitute declared.
func TestAnchorPresenceGateRefusesUntouchedAnchorAndAdmitsDirectTouch(t *testing.T) {
	engine, base := newAnchorGateEngineFixture(t)
	repository := engine.repository.Root()
	contract := protocol.Slice{
		ID: "S1",
		Scope: protocol.Scope{
			Include: []string{"one.txt", "README.md"}, Exclude: []string{},
		},
		Acceptance: []protocol.Criterion{{
			ID: "A2", Text: "Anchor presence test. Anchor: README.md.",
		}},
	}

	unrelated := writeAndCommitAnchorFixture(t, repository, "one.txt", "unrelated change\n")
	_, err := anchorPresenceGate(engine, contract, base, unrelated, nil)
	var recordErr *protocol.RecordError
	if !errors.As(err, &recordErr) || recordErr.Code != "ANCHOR_NOT_TOUCHED" {
		t.Fatalf("expected ANCHOR_NOT_TOUCHED, got %v", err)
	}
	if len(recordErr.Paths) != 1 || recordErr.Paths[0] != "README.md" ||
		!strings.Contains(recordErr.Msg, "A2") || !strings.Contains(recordErr.Msg, "anchor base") {
		t.Fatalf("unexpected refusal shape: %#v", recordErr)
	}

	touching := writeAndCommitAnchorFixture(t, repository, "README.md", "covers the anchor\n")
	if honored, err := anchorPresenceGate(engine, contract, base, touching, nil); err != nil || len(honored) != 0 {
		t.Fatalf("expected the anchor-touching candidate to clear the gate, honored=%#v err=%v", honored, err)
	}
}

// TestAnchorPresenceGateHonoursValidSubstituteAndRejectsInvalidOnes pins A3:
// a declared substitute the candidate touches, and that sits inside the
// slice's approved scope, satisfies its criterion and is returned as
// honored; an untouched substitute, and one outside scope, are both named as
// distinct failures in the refusal rather than silently ignored.
func TestAnchorPresenceGateHonoursValidSubstituteAndRejectsInvalidOnes(t *testing.T) {
	engine, base := newAnchorGateEngineFixture(t)
	repository := engine.repository.Root()
	contract := protocol.Slice{
		ID: "S1",
		Scope: protocol.Scope{
			Include: []string{"one.txt", "substitute.txt"}, Exclude: []string{},
		},
		Acceptance: []protocol.Criterion{{
			ID: "A2", Text: "Anchor presence test. Anchor: README.md.",
		}},
	}
	unrelated := writeAndCommitAnchorFixture(t, repository, "one.txt", "unrelated change\n")

	if _, err := anchorPresenceGate(
		engine, contract, base, unrelated, map[string]string{"A2": "substitute.txt"},
	); err == nil || !strings.Contains(err.Error(), "untouched") {
		t.Fatalf("expected an untouched-substitute failure, got %v", err)
	}

	if _, err := anchorPresenceGate(
		engine, contract, base, unrelated, map[string]string{"A2": "outside-scope.txt"},
	); err == nil {
		t.Fatal("expected the out-of-scope declared substitute to fail")
	}

	withSubstitute := writeAndCommitAnchorFixture(t, repository, "substitute.txt", "covers via substitution\n")
	honored, err := anchorPresenceGate(
		engine, contract, base, withSubstitute, map[string]string{"A2": "substitute.txt"},
	)
	if err != nil || honored["A2"] != "substitute.txt" {
		t.Fatalf("expected the declared substitute to be honored, got honored=%#v err=%v", honored, err)
	}
}

// TestAnchorPresenceGateSkipsCriterionWithNoBaseTreeAnchor pins the "not
// gated" half of resolveAnchorRequirements: a criterion whose declared
// anchor names only a file absent from the base tree (a wholly new file
// this same candidate is expected to add) is never gated.
func TestAnchorPresenceGateSkipsCriterionWithNoBaseTreeAnchor(t *testing.T) {
	engine, base := newAnchorGateEngineFixture(t)
	repository := engine.repository.Root()
	contract := protocol.Slice{
		ID:    "S1",
		Scope: protocol.Scope{Include: []string{"one.txt"}, Exclude: []string{}},
		Acceptance: []protocol.Criterion{{
			ID: "A9", Text: "Anchor: brand-new-file-not-in-base.go.",
		}},
	}
	candidate := writeAndCommitAnchorFixture(t, repository, "one.txt", "content\n")
	if _, err := anchorPresenceGate(engine, contract, base, candidate, nil); err != nil {
		t.Fatalf("a criterion with no base-tree anchor must never gate: %v", err)
	}
}

// TestAnchorPresenceGateFailsClosedOnUnreadableOID pins A5's fail-closed
// promise for this gate: an OID the repository cannot resolve refuses
// ANCHOR_GATE_UNREADABLE rather than silently passing, for either the base
// or the candidate.
func TestAnchorPresenceGateFailsClosedOnUnreadableOID(t *testing.T) {
	engine, base := newAnchorGateEngineFixture(t)
	contract := protocol.Slice{
		Acceptance: []protocol.Criterion{{ID: "A2", Text: "Anchor: README.md."}},
	}
	if _, err := anchorPresenceGate(engine, contract, "not-an-oid", base, nil); !IsCode(err, "ANCHOR_GATE_UNREADABLE") {
		t.Fatalf("unreadable base error = %v, want ANCHOR_GATE_UNREADABLE", err)
	}
	if _, err := anchorPresenceGate(engine, contract, base, "not-an-oid", nil); !IsCode(err, "ANCHOR_GATE_UNREADABLE") {
		t.Fatalf("unreadable candidate error = %v, want ANCHOR_GATE_UNREADABLE", err)
	}
}
