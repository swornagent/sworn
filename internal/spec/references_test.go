package spec

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/baton"
)

type referenceFixture struct {
	root    string
	release string
	slice   string
}

func newReferenceFixture(t *testing.T, references string) *referenceFixture {
	t.Helper()
	f := &referenceFixture{root: t.TempDir(), release: "2026-07-17-references", slice: "S01-reviewed"}
	cmd := exec.Command("git", "init", "-q", f.root)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, output)
	}
	f.write(t, f.specPath(), fixtureSpec(f.release, f.slice, references))
	raw, err := os.ReadFile(f.specPath())
	if err != nil {
		t.Fatal(err)
	}
	if err := baton.ValidateSchema("spec-v1", raw); err != nil {
		t.Fatalf("fixture spec must validate: %v\n%s", err, raw)
	}
	return f
}

func (f *referenceFixture) specPath() string {
	return filepath.Join(f.root, "docs", "release", f.release, f.slice, "spec.json")
}

func (f *referenceFixture) path(parts ...string) string {
	return filepath.Join(append([]string{f.root}, parts...)...)
}

func (f *referenceFixture) write(t *testing.T, filename, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func fixtureSpec(release, slice, references string) string {
	return fmt.Sprintf(`{
  "$schema": "https://baton.sawy3r.net/schemas/spec-v1.json",
  "slice_id": %q,
  "release": %q,
  "user_outcome": "A checked reference is available.",
  "covers_needs": ["N-01"],
  "acceptance_criteria": [{"id":"AC-01","text":"THE SYSTEM SHALL preserve the typed reference.","ears_pattern":"ubiquitous"}],
  "in_scope": [],
  "out_of_scope": [],
  "references": %s
}`+"\n", slice, release, references)
}

func fixtureContracts(release string, ids ...string) string {
	entries := make([]string, 0, len(ids))
	for _, id := range ids {
		entries = append(entries, fmt.Sprintf(`{"id":%q,"kind":"schema-version","surface":"surface","shape":"shape","owner":"S01-owner"}`, id))
	}
	return fmt.Sprintf(`{"$schema":"https://baton.sawy3r.net/schemas/contracts-v1.json","release":%q,"contracts":[%s]}`+"\n", release, strings.Join(entries, ","))
}

func TestResolveReferences_TypedArtifactsAndSafeUnresolved(t *testing.T) {
	f := newReferenceFixture(t, `[
  {"kind":"file","path":"docs/reference.txt"},
  {"kind":"contract","contract_id":"C-01"},
  {"kind":"slice","slice_id":"S02-sibling"},
  {"kind":"file","path":"docs/missing.txt"}
]`)
	f.write(t, f.path("docs", "reference.txt"), "file reference\n")
	f.write(t, f.path("docs", "release", f.release, "contracts.json"), fixtureContracts(f.release, "C-01"))
	f.write(t, f.path("docs", "release", f.release, "S02-sibling", "spec.json"), fixtureSpec(f.release, "S02-sibling", "[]"))
	// This is deliberately named in a non-normative field and must never reach
	// the model payload through implicit discovery.
	f.write(t, f.path("private-canary.txt"), "MUST-NOT-LEAK")

	resolution, err := ResolveReferences(f.specPath())
	if err != nil {
		t.Fatalf("ResolveReferences: %v", err)
	}
	gotPaths := make([]string, 0, len(resolution.Artifacts))
	for _, artifact := range resolution.Artifacts {
		gotPaths = append(gotPaths, artifact.Path)
	}
	wantPaths := []string{
		"docs/reference.txt",
		"docs/release/2026-07-17-references/S02-sibling/spec.json",
		"docs/release/2026-07-17-references/contracts.json",
	}
	if strings.Join(gotPaths, "\n") != strings.Join(wantPaths, "\n") {
		t.Fatalf("artifact paths = %v, want %v", gotPaths, wantPaths)
	}
	if len(resolution.Unresolved) != 1 || resolution.Unresolved[0] != (UnresolvedReference{Key: "file:docs/missing.txt", Reason: "missing"}) {
		t.Fatalf("unresolved = %+v", resolution.Unresolved)
	}
	rendered := resolution.Render()
	for _, want := range []string{
		"--- ARTIFACT docs/reference.txt ---\nfile reference\n",
		"--- ARTIFACT docs/release/2026-07-17-references/S02-sibling/spec.json ---",
		"--- ARTIFACT docs/release/2026-07-17-references/contracts.json ---",
		"UNRESOLVED file:docs/missing.txt: missing\n",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("rendered payload missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "MUST-NOT-LEAK") || strings.Contains(rendered, "private-canary") {
		t.Fatalf("implicit artefact discovery leaked an unreferenced canary: %s", rendered)
	}
}

func TestResolveReferences_UsesBytewiseArtifactAndUnresolvedOrder(t *testing.T) {
	f := newReferenceFixture(t, `[
  {"kind":"file","path":"z/missing.txt"},
  {"kind":"slice","slice_id":"S99-sibling"},
  {"kind":"contract","contract_id":"C-02"},
  {"kind":"file","path":"a/missing.txt"},
  {"kind":"contract","contract_id":"C-01"}
]`)
	resolution, err := ResolveReferences(f.specPath())
	if err != nil {
		t.Fatalf("ResolveReferences: %v", err)
	}
	var keys []string
	for _, unresolved := range resolution.Unresolved {
		keys = append(keys, unresolved.Key)
	}
	want := []string{
		"contract:C-01",
		"contract:C-02",
		"file:a/missing.txt",
		"file:z/missing.txt",
		"slice:S99-sibling",
	}
	if strings.Join(keys, "\n") != strings.Join(want, "\n") {
		t.Fatalf("unresolved order = %v, want %v", keys, want)
	}
}

func TestReferenceResolutionFailureMatrixBeforeDispatch(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T) (string, string)
		want  string
	}{
		{
			name: "duplicate reviewed spec key",
			setup: func(t *testing.T) (string, string) {
				f := newReferenceFixture(t, "[]")
				f.write(t, f.specPath(), `{"slice_id":"S01-reviewed","slice_id":"S99-shadow"}`)
				return f.specPath(), ""
			},
			want: FailureReviewedSpecSchemaInvalid,
		},
		{
			name: "workspace root unavailable",
			setup: func(t *testing.T) (string, string) {
				root := t.TempDir()
				path := filepath.Join(root, "docs", "release", "2026-07-17-references", "S01-reviewed", "spec.json")
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(fixtureSpec("2026-07-17-references", "S01-reviewed", "[]")), 0o644); err != nil {
					t.Fatal(err)
				}
				return path, ""
			},
			want: FailureWorkspaceRootUnavailable,
		},
		{
			name: "reviewed source path mismatch",
			setup: func(t *testing.T) (string, string) {
				f := newReferenceFixture(t, "[]")
				wrong := f.path("elsewhere", "spec.json")
				f.write(t, wrong, fixtureSpec(f.release, f.slice, "[]"))
				return wrong, ""
			},
			want: FailureReviewedSpecSourcePath,
		},
		{
			name: "lexically invalid reference path",
			setup: func(t *testing.T) (string, string) {
				f := newReferenceFixture(t, `[{"kind":"file","path":"../outside.txt"}]`)
				return f.specPath(), ""
			},
			want: FailureReferencePathInvalid,
		},
		{
			name: "physical escape through symlink",
			setup: func(t *testing.T) (string, string) {
				f := newReferenceFixture(t, `[{"kind":"file","path":"docs/escape.txt"}]`)
				outside := filepath.Join(t.TempDir(), "outside.txt")
				if err := os.WriteFile(outside, []byte("MUST-NOT-LEAK"), 0o644); err != nil {
					t.Fatal(err)
				}
				link := f.path("docs", "escape.txt")
				if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, link); err != nil {
					t.Fatal(err)
				}
				return f.specPath(), ""
			},
			want: FailureReferencePathEscape,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, _ := tt.setup(t)
			_, err := ResolveReferences(path)
			var failure *ReferenceResolutionError
			if !errors.As(err, &failure) {
				t.Fatalf("ResolveReferences error = %v, want %s", err, tt.want)
			}
			if failure.Class != tt.want {
				t.Fatalf("failure class = %q, want %q", failure.Class, tt.want)
			}
		})
	}
}

func TestResolveReferences_SafeFailureVocabulary(t *testing.T) {
	tests := []struct {
		name       string
		references string
		prepare    func(t *testing.T, f *referenceFixture)
		want       string
	}{
		{
			name:       "missing",
			references: `[{"kind":"file","path":"docs/missing.txt"}]`,
			want:       "missing",
		},
		{
			name:       "non regular",
			references: `[{"kind":"file","path":"docs/directory"}]`,
			prepare: func(t *testing.T, f *referenceFixture) {
				if err := os.MkdirAll(f.path("docs", "directory"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
			want: "non-regular",
		},
		{
			name:       "unreadable",
			references: `[{"kind":"file","path":"docs/unreadable.txt"}]`,
			prepare: func(t *testing.T, f *referenceFixture) {
				filename := f.path("docs", "unreadable.txt")
				f.write(t, filename, "blocked")
				if err := os.Chmod(filename, 0o000); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(filename, 0o644) })
			},
			want: "unreadable",
		},
		{
			name:       "invalid utf8",
			references: `[{"kind":"file","path":"docs/invalid.bin"}]`,
			prepare: func(t *testing.T, f *referenceFixture) {
				filename := f.path("docs", "invalid.bin")
				if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filename, []byte{0xff}, 0o644); err != nil {
					t.Fatal(err)
				}
			},
			want: "invalid-utf8",
		},
		{
			name:       "contract invalid json",
			references: `[{"kind":"contract","contract_id":"C-01"}]`,
			prepare: func(t *testing.T, f *referenceFixture) {
				f.write(t, f.path("docs", "release", f.release, "contracts.json"), "{")
			},
			want: "invalid-json",
		},
		{
			name:       "contract schema invalid",
			references: `[{"kind":"contract","contract_id":"C-01"}]`,
			prepare: func(t *testing.T, f *referenceFixture) {
				f.write(t, f.path("docs", "release", f.release, "contracts.json"), `{"release":"`+f.release+`","contracts":[],"unexpected":true}`)
			},
			want: "schema-invalid",
		},
		{
			name:       "contract record release mismatch",
			references: `[{"kind":"contract","contract_id":"C-01"}]`,
			prepare: func(t *testing.T, f *referenceFixture) {
				f.write(t, f.path("docs", "release", f.release, "contracts.json"), fixtureContracts("another-release", "C-01"))
			},
			want: "record-release-mismatch",
		},
		{
			name:       "contract id missing",
			references: `[{"kind":"contract","contract_id":"C-01"}]`,
			prepare: func(t *testing.T, f *referenceFixture) {
				f.write(t, f.path("docs", "release", f.release, "contracts.json"), fixtureContracts(f.release, "C-02"))
			},
			want: "contract-id-missing",
		},
		{
			name:       "contract id duplicate",
			references: `[{"kind":"contract","contract_id":"C-01"}]`,
			prepare: func(t *testing.T, f *referenceFixture) {
				f.write(t, f.path("docs", "release", f.release, "contracts.json"), fixtureContracts(f.release, "C-01", "C-01"))
			},
			want: "contract-id-duplicate",
		},
		{
			name:       "slice record release mismatch",
			references: `[{"kind":"slice","slice_id":"S02-sibling"}]`,
			prepare: func(t *testing.T, f *referenceFixture) {
				f.write(t, f.path("docs", "release", f.release, "S02-sibling", "spec.json"), fixtureSpec("another-release", "S02-sibling", "[]"))
			},
			want: "record-release-mismatch",
		},
		{
			name:       "slice id mismatch",
			references: `[{"kind":"slice","slice_id":"S02-sibling"}]`,
			prepare: func(t *testing.T, f *referenceFixture) {
				f.write(t, f.path("docs", "release", f.release, "S02-sibling", "spec.json"), fixtureSpec(f.release, "S03-different", "[]"))
			},
			want: "slice-id-mismatch",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newReferenceFixture(t, tt.references)
			if tt.prepare != nil {
				tt.prepare(t, f)
			}
			resolution, err := ResolveReferences(f.specPath())
			if err != nil {
				t.Fatalf("ResolveReferences: %v", err)
			}
			if len(resolution.Unresolved) != 1 {
				t.Fatalf("unresolved = %+v, want one %q", resolution.Unresolved, tt.want)
			}
			if got := resolution.Unresolved[0].Reason; got != tt.want {
				t.Fatalf("unresolved reason = %q, want %q", got, tt.want)
			}
		})
	}
}
