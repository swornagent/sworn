package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// write puts a raw record at repoRoot/.sworn/project.json.
func write(t *testing.T, repoRoot, raw string) {
	t.Helper()
	dir := filepath.Join(repoRoot, ".sworn")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "project.json"), []byte(raw), 0644); err != nil {
		t.Fatal(err)
	}
}

// repo makes a minimal Go repo so Detect has something to find.
func repo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return root
}

const ratifiedHigh = `{
  "$schema": "https://baton.sawy3r.net/schemas/project-context-v1.json",
  "context": "a Next.js and TypeScript frontend with a Go backend on Postgres",
  "stakes": {"production": true, "real_users": true, "sensitive_data": ["pii", "financial"]},
  "ratification": {"ratified": true, "by": "a human", "at": "2026-07-14T09:00:00Z"}
}`

const ratifiedLow = `{
  "context": "a Go CLI",
  "stakes": {"production": false, "real_users": false, "sensitive_data": []},
  "ratification": {"ratified": true, "by": "a human"}
}`

// TestResolve_FailsClosedOnStakes is the load-bearing guard.
//
// The stakes decide whether a `medium` security finding blocks or merely advises
// (baton v0.13.0). So the ONLY way to reach low stakes is a ratified record that
// declares them. Absent, malformed, or unratified must all resolve to HIGH — a
// model-drafted proposal may RAISE the bar, never LOWER it.
func TestResolve_FailsClosedOnStakes(t *testing.T) {
	tests := []struct {
		name       string
		record     string // "" means no record at all
		wantHigh   bool
		wantSource Source
	}{
		{
			name:       "no record at all",
			record:     "",
			wantHigh:   true,
			wantSource: SourceInferred,
		},
		{
			// The trap: a model drafts "this is just a CLI, no stakes" and nobody
			// has confirmed it. If an unratified draft could lower the bar, the
			// whole mechanism would be self-certification with extra steps.
			name:       "UNRATIFIED record claiming low stakes",
			record:     `{"context":"a Go CLI","stakes":{"production":false,"real_users":false},"ratification":{"ratified":false,"drafted_by":"some/model"}}`,
			wantHigh:   true,
			wantSource: SourceDrafted,
		},
		{
			name:       "malformed record (invalid against the schema)",
			record:     `{"context":"a Go CLI"}`, // missing required `ratification`
			wantHigh:   true,
			wantSource: SourceInferred,
		},
		{
			name:       "not even JSON",
			record:     `{{{ not json`,
			wantHigh:   true,
			wantSource: SourceInferred,
		},
		{
			// A human ratified a record that says nothing about risk. That means
			// they did not consider it, not that there is none.
			name:       "ratified but stakes omitted entirely",
			record:     `{"context":"a Go CLI","ratification":{"ratified":true,"by":"a human"}}`,
			wantHigh:   true,
			wantSource: SourceDeclared,
		},
		{
			name:       "RATIFIED high stakes",
			record:     ratifiedHigh,
			wantHigh:   true,
			wantSource: SourceDeclared,
		},
		{
			// The one and only path to a lowered bar.
			name:       "RATIFIED low stakes",
			record:     ratifiedLow,
			wantHigh:   false,
			wantSource: SourceDeclared,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := repo(t)
			if tc.record != "" {
				write(t, root, tc.record)
			}

			got := Resolve(root)
			if got.HighStakes != tc.wantHigh {
				t.Errorf("HighStakes = %v, want %v\n"+
					"only a RATIFIED record declaring low stakes may lower the security bar",
					got.HighStakes, tc.wantHigh)
			}
			if got.Source != tc.wantSource {
				t.Errorf("Source = %q, want %q", got.Source, tc.wantSource)
			}
			if got.Context == "" {
				t.Error("Context is empty — the model must always be told something")
			}
		})
	}
}

// TestResolve_DeclaredContextBeatsDetection — a declared record's context is used
// verbatim, not overridden by what the filesystem happens to look like.
func TestResolve_DeclaredContextBeatsDetection(t *testing.T) {
	root := repo(t) // has go.mod, so Detect() would say "a Go project"
	write(t, root, ratifiedHigh)

	got := Resolve(root)
	if !strings.Contains(got.Context, "Next.js") {
		t.Errorf("Context = %q — the declared record must win over detection", got.Context)
	}
}

// TestRenderStakes_NeverClaimsLowOnAGuess — the rendered block is what the model
// grades against. An inferred or drafted project must never be rendered as LOW.
func TestRenderStakes_NeverClaimsLowOnAGuess(t *testing.T) {
	for _, src := range []Source{SourceInferred, SourceDrafted} {
		r := Resolved{
			Context: "a Go project",
			Source:  src,
			// Even with a Stakes block that claims nothing is at risk...
			Stakes:     &Stakes{Production: false, RealUsers: false},
			HighStakes: true,
		}
		out := r.RenderStakes()
		if strings.Contains(out, "STAKES: LOW") {
			t.Errorf("source %q rendered LOW stakes to the model — a guess must never lower the bar\n%s", src, out)
		}
		if !strings.Contains(out, "STAKES: HIGH") {
			t.Errorf("source %q must render HIGH stakes", src)
		}
	}
}

// TestRenderStakes_TellsTheModelTheStakesAreAssumed — when the engine fails closed,
// it must say so rather than pass an assumption off as a declaration.
func TestRenderStakes_TellsTheModelTheStakesAreAssumed(t *testing.T) {
	inferred := Resolved{Context: "a Go project", Source: SourceInferred, HighStakes: true}
	if !strings.Contains(inferred.RenderStakes(), "declared no context record") {
		t.Error("an inferred project must tell the model the context was not declared")
	}

	drafted := Resolved{Context: "a Go CLI", Source: SourceDrafted, HighStakes: true, Stakes: &Stakes{}}
	if !strings.Contains(drafted.RenderStakes(), "NOT been ratified") {
		t.Error("a drafted project must tell the model the record is an unratified proposal")
	}
}

// TestSave_RefusesAnInvalidRecord — Save grades before it writes, so a malformed
// record never reaches disk to be misread later.
func TestSave_RefusesAnInvalidRecord(t *testing.T) {
	root := repo(t)
	// context is required by the schema.
	err := Save(root, &Record{Ratification: Ratification{Ratified: true}})
	if err == nil {
		t.Fatal("Save wrote a record with no context — it must grade before writing")
	}
	if _, statErr := os.Stat(filepath.Join(root, RecordPath)); !os.IsNotExist(statErr) {
		t.Error("an invalid record was written to disk anyway")
	}
}

// TestSaveLoadRoundTrip — a record written by Save is readable by Load.
func TestSaveLoadRoundTrip(t *testing.T) {
	root := repo(t)
	want := &Record{
		Context: "a Next.js and TypeScript frontend with a Go backend on Postgres",
		Stakes: &Stakes{
			Production: true, RealUsers: true,
			SensitiveData: []string{"pii", "financial"},
		},
		Ratification: Ratification{Ratified: true, By: "a human", DraftedBy: "some/model"},
	}
	if err := Save(root, want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Context != want.Context {
		t.Errorf("Context = %q, want %q", got.Context, want.Context)
	}
	if !got.Ratification.Ratified {
		t.Error("ratification did not round-trip")
	}
	if r := Resolve(root); r.Source != SourceDeclared || !r.HighStakes {
		t.Errorf("Resolve after Save = %+v, want declared + high stakes", r)
	}
}
