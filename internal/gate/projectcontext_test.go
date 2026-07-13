package gate

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

// TestDetectProjectContext is the regression guard for the hardcoded project
// header: `sworn llm-check` used to tell the model it was evaluating "the
// SwornAgent project (a Go CLI)" no matter which repo it ran in. A TypeScript
// codebase was graded against Go priors, silently.
func TestDetectProjectContext(t *testing.T) {
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
			got := DetectProjectContext(mkRepo(t, tc.files...))
			if got != tc.want {
				t.Errorf("DetectProjectContext() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestDetectProjectContext_EnvOverride — Baton specifies {{project_context}} as
// supplied "from the repo's configuration"; detection is the default and this is
// the adopter's override.
func TestDetectProjectContext_EnvOverride(t *testing.T) {
	root := mkRepo(t, "go.mod")
	const want = "a regulated FSI document-processing service"
	t.Setenv(ProjectContextEnv, want)

	if got := DetectProjectContext(root); got != want {
		t.Errorf("DetectProjectContext() = %q, want the %s override %q", got, ProjectContextEnv, want)
	}
}

// TestUserPromptHeaderNamesTheRealProject pins the actual defect: the header must
// carry the detected context, and must not carry the old hardcoded string.
func TestUserPromptHeaderNamesTheRealProject(t *testing.T) {
	payload := buildUserPayload("a Next.js and TypeScript monorepo", "SPEC", "DIFF")

	if !contains(payload, "a Next.js and TypeScript monorepo") {
		t.Error("user payload does not tell the model what project it is reading")
	}
	if contains(payload, "SwornAgent project") || contains(payload, "a Go CLI") {
		t.Error("user payload still carries the hardcoded SwornAgent/Go-CLI header — " +
			"every check in every repo would grade against Go priors")
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}
