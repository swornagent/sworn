package project

import (
	"os"
	"path/filepath"
	"testing"
)

// mkRepo builds a fake repo tree from a set of relative file paths.
func mkRepo(t *testing.T, files ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, f := range files {
		p := filepath.Join(root, f)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("{}"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// TestDetect is the regression guard for the hardcoded project
// header: `sworn llm-check` used to tell the model it was evaluating "the
// SwornAgent project (a Go CLI)" no matter which repo it ran in. A TypeScript
// codebase was graded against Go priors, silently.
func TestDetect(t *testing.T) {
	tests := []struct {
		name  string
		files []string
		want  string
	}{
		{
			name:  "go project",
			files: []string{"go.mod", "main.go"},
			want:  "a Go project",
		},
		{
			// The shape that motivated the fix: a Turborepo monorepo whose real
			// markers live under apps/, with only tsconfig.base.json at the root.
			// A root-only, exact-name scan reports this as "a JavaScript project".
			name: "typescript nextjs monorepo",
			files: []string{
				"package.json", "turbo.json", "pnpm-workspace.yaml", "tsconfig.base.json",
				"apps/web/next.config.mjs", "apps/web/tsconfig.json",
				"packages/auth/tsconfig.json",
			},
			want: "a Next.js and TypeScript monorepo",
		},
		{
			// The real shape that bit: a polyglot monorepo whose Go backend sits
			// in a TOP-LEVEL go/ directory, beside apps/ and packages/. Scanning
			// only the JS-ecosystem workspace names (apps, packages, services,
			// libs) reports this as "a Next.js and TypeScript monorepo" while the
			// backend sits in plain sight at go/go.mod — so a security check on a
			// diff in go/ would be told it is reading a TypeScript frontend.
			name: "polyglot monorepo with a top-level go backend",
			files: []string{
				"package.json", "turbo.json", "pnpm-workspace.yaml", "tsconfig.base.json",
				"apps/web/next.config.mjs", "apps/web/tsconfig.json",
				"packages/auth/tsconfig.json",
				"go/go.mod",
				"node_modules/react/package.json", // must not be scanned
			},
			want: "a Go, Next.js and TypeScript monorepo",
		},
		{
			name:  "polyglot",
			files: []string{"go.mod", "tsconfig.json", "pyproject.toml"},
			want:  "a Go, Python and TypeScript project",
		},
		{
			// Naming the framework already implies its ecosystem.
			name:  "nextjs does not also say javascript",
			files: []string{"package.json", "next.config.js"},
			want:  "a Next.js project",
		},
		{
			// Vague but true, where the old header was specific and false.
			name:  "unrecognised repo",
			files: []string{"README.md"},
			want:  "a software project",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Detect(mkRepo(t, tc.files...))
			if got != tc.want {
				t.Errorf("Detect() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestDetect_EnvOverride — Baton specifies {{project_context}} as
// supplied "from the repo's configuration"; detection is the default and this is
// the adopter's override.
func TestDetect_EnvOverride(t *testing.T) {
	root := mkRepo(t, "go.mod")
	const want = "a regulated FSI document-processing service"
	t.Setenv(ContextEnv, want)

	if got := Detect(root); got != want {
		t.Errorf("Detect() = %q, want the %s override %q", got, ContextEnv, want)
	}
}
