package gate

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ProjectContextEnv lets an adopter state their project's context explicitly,
// overriding detection. Baton v0.12.0 specifies {{project_context}} as supplied
// by the engine "from the repo's configuration"; detection is the default, this
// is the override.
const ProjectContextEnv = "SWORN_PROJECT_CONTEXT"

// techMarker maps a marker file to the technology it implies. Matched by exact
// name or, where a project may name the file several ways, by prefix.
type techMarker struct {
	prefix string // match any file whose name starts with this
	name   string // match this exact filename (when prefix is empty)
	tech   string
}

var techMarkers = []techMarker{
	{name: "next.config.js", tech: "Next.js"},
	{name: "next.config.ts", tech: "Next.js"},
	{name: "next.config.mjs", tech: "Next.js"},
	{name: "go.mod", tech: "Go"},
	// tsconfig.json, tsconfig.base.json, tsconfig.build.json — a monorepo root
	// commonly carries only the .base variant, so an exact match misses it.
	{prefix: "tsconfig", tech: "TypeScript"},
	{name: "Cargo.toml", tech: "Rust"},
	{name: "pyproject.toml", tech: "Python"},
	{name: "setup.py", tech: "Python"},
	{name: "requirements.txt", tech: "Python"},
	{name: "Gemfile", tech: "Ruby"},
	{name: "composer.json", tech: "PHP"},
	{name: "pom.xml", tech: "Java"},
	{name: "build.gradle", tech: "Java"},
	{name: "build.gradle.kts", tech: "Kotlin"},
	{name: "package.json", tech: "JavaScript"},
}

// monorepoMarkers signal a workspace root whose real technology markers live
// below it rather than at the root.
var monorepoMarkers = []string{"turbo.json", "pnpm-workspace.yaml", "lerna.json", "nx.json", "rush.json"}

// workspaceDirs are the conventional places a monorepo keeps its packages. These
// get scanned one level deeper (apps/web/next.config.mjs), because the directory
// itself holds no markers.
var workspaceDirs = map[string]bool{
	"apps": true, "packages": true, "services": true, "libs": true, "modules": true,
}

// skipDirs are never scanned: dependency trees, build output, and agent scratch.
// Without these, a repo's node_modules would dominate detection and cost a
// pointless walk.
var skipDirs = map[string]bool{
	"node_modules": true, ".git": true, "dist": true, "build": true, "out": true,
	"vendor": true, "target": true, ".next": true, ".turbo": true, ".claude": true,
	"coverage": true, "tmp": true, ".venv": true, "venv": true, "__pycache__": true,
}

// DetectProjectContext returns a one-line description of the project rooted at
// repoRoot, for substitution into the LLM checks' user payload
// ({{project_context}}, Baton v0.12.0). For example "a Go project", or
// "a Next.js and TypeScript monorepo".
//
// This replaces a header hardcoded to "the SwornAgent project (a Go CLI)", which
// was sent to the model on EVERY check in EVERY repo. Running the checks against
// a TypeScript codebase told the model it was reading a Go CLI, so it graded
// against the wrong priors — silently, and in the direction of leniency. Baton
// makes this substitution REQUIRED rather than defaulted for exactly that reason.
//
// SWORN_PROJECT_CONTEXT overrides detection. Detection never fails: an
// unrecognised repo is "a software project" — vague but true, where the old
// header was specific and false.
func DetectProjectContext(repoRoot string) string {
	if v := strings.TrimSpace(os.Getenv(ProjectContextEnv)); v != "" {
		return v
	}

	seen := map[string]bool{}
	scanDir(repoRoot, seen)

	isMonorepo := false
	for _, m := range monorepoMarkers {
		if _, err := os.Stat(filepath.Join(repoRoot, m)); err == nil {
			isMonorepo = true
			break
		}
	}

	// A monorepo root carries only a workspace config; the technologies that
	// actually describe the project live below it. Scan EVERY child directory,
	// not just the JS-ecosystem workspace names — a polyglot repo keeps its
	// backend in a top-level dir named after its language (go/, server/, api/).
	// Scanning only apps/ and packages/ reported one such repo as "a Next.js and
	// TypeScript monorepo" while its Go backend sat in plain sight at go/go.mod.
	if isMonorepo {
		children, err := os.ReadDir(repoRoot)
		if err == nil {
			for _, c := range children {
				if !c.IsDir() || skipDirs[c.Name()] || strings.HasPrefix(c.Name(), ".") {
					continue
				}
				child := filepath.Join(repoRoot, c.Name())
				scanDir(child, seen)

				// Workspace dirs hold no markers themselves — go one level deeper.
				if workspaceDirs[c.Name()] {
					pkgs, err := os.ReadDir(child)
					if err != nil {
						continue
					}
					for _, p := range pkgs {
						if p.IsDir() && !skipDirs[p.Name()] {
							scanDir(filepath.Join(child, p.Name()), seen)
						}
					}
				}
			}
		}
	}

	// Naming a framework already implies its ecosystem: say "Next.js and
	// TypeScript", not "Next.js, TypeScript and JavaScript".
	if seen["Next.js"] || seen["TypeScript"] {
		delete(seen, "JavaScript")
	}

	var techs []string
	for t := range seen {
		techs = append(techs, t)
	}
	if len(techs) == 0 {
		return "a software project"
	}
	sort.Strings(techs)

	kind := "project"
	if isMonorepo {
		kind = "monorepo"
	}
	return "a " + joinAnd(techs) + " " + kind
}

// scanDir records every technology whose marker file is present directly in dir.
func scanDir(dir string, seen map[string]bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	names := make(map[string]bool, len(entries))
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		names[e.Name()] = true
		files = append(files, e.Name())
	}

	for _, m := range techMarkers {
		if m.prefix != "" {
			for _, f := range files {
				if strings.HasPrefix(f, m.prefix) {
					seen[m.tech] = true
					break
				}
			}
			continue
		}
		if names[m.name] {
			seen[m.tech] = true
		}
	}
}

// joinAnd renders a list as "A", "A and B", or "A, B and C".
func joinAnd(items []string) string {
	switch len(items) {
	case 1:
		return items[0]
	case 2:
		return items[0] + " and " + items[1]
	default:
		return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
	}
}

// repoRootFrom walks up from dir looking for a .git entry, returning the repo
// root. Falls back to dir when no .git is found — a non-git checkout still gets a
// best-effort context rather than a hardcoded lie.
func repoRootFrom(dir string) string {
	d, err := filepath.Abs(dir)
	if err != nil {
		return dir
	}
	for {
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			return dir
		}
		d = parent
	}
}
