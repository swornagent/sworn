package project

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/swornagent/sworn/internal/model"
)

// elicitSystemPrompt asks the model to draft a project-context-v1 record.
//
// It drafts; it does not decide. The stakes it returns are a PROPOSAL — the record
// is written unratified, and until a human ratifies it the engine treats its stakes
// as HIGH regardless of what the model claimed (see Resolve). A model can read the
// auth code and the payment integration; it cannot know whether real people depend
// on this system today.
const elicitSystemPrompt = `You are helping set up an engineering-quality tool in a code repository.

Read the evidence about the repository below and describe the project, so that later
automated code reviews grade it against the right expectations.

Respond with a JSON object and nothing else:
{
  "context": "one line completing the sentence 'You are evaluating a slice in a release of ___'",
  "stakes": {
    "production": true | false,
    "real_users": true | false,
    "sensitive_data": ["pii" | "financial" | "health" | "credentials" | "government-id" | "location" | "biometric"],
    "regulated": ["any regulatory regime you have concrete evidence for"],
    "notes": "anything a reviewer should know before judging how severe a defect is"
  },
  "uncertain": ["each stakes field you are GUESSING at rather than reading evidence for"]
}

Rules for "context":
- Name the languages, the frameworks, and the data layer. A reviewer should be able to
  tell from this line what kind of code they are about to read.
- Cover the WHOLE repo. A monorepo with a TypeScript frontend and a Go backend is both,
  not whichever is larger.
- Good: "a Next.js and TypeScript frontend with a Go backend on Postgres"
- Bad: "a web app" (says nothing), "a TypeScript monorepo" (drops the backend)

Rules for "stakes":
- These decide whether a medium-severity security finding BLOCKS a release or merely
  advises. Getting them wrong in the lenient direction ships vulnerabilities.
- "sensitive_data": include a category only if you can see evidence — an auth flow, a
  payments integration, a schema with customer records, a health or identity field.
- "production" and "real_users": you usually CANNOT know these from code alone. Make your
  best inference from deployment configs, billing code, or a public-facing README — and
  then LIST THEM IN "uncertain". Do not quietly guess.
- When you are unsure, lean towards the higher-stakes answer and say so in "uncertain".
  A human will review this. It is far cheaper for them to relax a claim you overstated
  than to notice one you understated.

Be concrete. Cite what you saw in "notes" if it drove a stakes decision.
Temperature 0 — be deterministic.`

// Draft is a model-proposed record plus the fields it says it guessed at.
type Draft struct {
	Record    *Record
	Uncertain []string
}

// Elicit asks the model to draft a project-context record from repository evidence.
//
// THE CALL IS THE ADOPTER'S, NOT THE PROTOCOL'S. It runs through the caller's own
// configured model and credentials, against their own provider. Baton specifies no
// hosted service and no phone-home for this: drafting the record means sending
// repository content to a model, and where that content goes is a data-residency
// decision the adopter owns. The engine must never reach out to a third party of its
// own choosing to fill this gap.
//
// The returned record is always UNRATIFIED. A human must review it — that is the
// whole point, and Resolve treats an unratified record's stakes as HIGH until they do.
func Elicit(ctx context.Context, repoRoot string, modelID string, verifier model.Verifier) (*Draft, error) {
	evidence, err := gatherEvidence(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("project: gather evidence: %w", err)
	}

	raw, _, _, _, err := verifier.Verify(ctx, elicitSystemPrompt, evidence)
	if err != nil {
		return nil, fmt.Errorf("project: model call failed: %w", err)
	}

	var resp struct {
		Context   string   `json:"context"`
		Stakes    *Stakes  `json:"stakes"`
		Uncertain []string `json:"uncertain"`
	}
	if err := json.Unmarshal([]byte(extractJSON(raw)), &resp); err != nil {
		return nil, fmt.Errorf("project: model response was not the expected JSON: %w (raw: %.200s)", err, raw)
	}
	if strings.TrimSpace(resp.Context) == "" {
		return nil, fmt.Errorf("project: model returned an empty context line")
	}

	return &Draft{
		Record: &Record{
			Context: strings.TrimSpace(resp.Context),
			Stakes:  resp.Stakes,
			// Never ratified by the model. Only a human ratifies.
			Ratification: Ratification{Ratified: false, DraftedBy: modelID},
		},
		Uncertain: resp.Uncertain,
	}, nil
}

// EvidenceFiles are the manifests worth showing the model. Bounded on purpose: this
// content leaves the machine for the adopter's model, so send what identifies the
// stack and nothing more. No source files, no .env, no secrets.
var EvidenceFiles = []string{
	"README.md", "go.mod", "package.json", "tsconfig.json", "tsconfig.base.json",
	"pyproject.toml", "Cargo.toml", "Gemfile", "composer.json", "pom.xml",
	"docker-compose.yml", "turbo.json", "pnpm-workspace.yaml",
}

const (
	maxFileBytes = 4000 // per evidence file
	maxTreeLines = 120
)

// gatherEvidence builds the user payload: a bounded view of the repo's shape.
func gatherEvidence(repoRoot string) (string, error) {
	var b strings.Builder

	b.WriteString("--- DETECTED STACK (from marker files) ---\n\n")
	b.WriteString(Detect(repoRoot) + "\n\n")

	b.WriteString("--- REPOSITORY LAYOUT (top two levels) ---\n\n")
	tree, err := shallowTree(repoRoot)
	if err != nil {
		return "", err
	}
	b.WriteString(tree)

	b.WriteString("\n--- MANIFESTS ---\n")
	for _, name := range EvidenceFiles {
		raw, err := os.ReadFile(filepath.Join(repoRoot, name))
		if err != nil {
			continue
		}
		if len(raw) > maxFileBytes {
			raw = append(raw[:maxFileBytes], []byte("\n... (truncated)")...)
		}
		fmt.Fprintf(&b, "\n### %s\n\n%s\n", name, raw)
	}

	return b.String(), nil
}

// shallowTree lists the repo's directories two levels deep, plus any manifest files
// found in them. Enough to see "apps/web is Next.js, go/ is the backend" without
// walking a node_modules tree.
func shallowTree(repoRoot string) (string, error) {
	var lines []string

	top, err := os.ReadDir(repoRoot)
	if err != nil {
		return "", err
	}
	for _, e := range top {
		name := e.Name()
		if strings.HasPrefix(name, ".") || skipDirs[name] {
			continue
		}
		if !e.IsDir() {
			continue
		}
		lines = append(lines, name+"/")

		children, err := os.ReadDir(filepath.Join(repoRoot, name))
		if err != nil {
			continue
		}
		for _, c := range children {
			cn := c.Name()
			if strings.HasPrefix(cn, ".") || skipDirs[cn] {
				continue
			}
			suffix := ""
			if c.IsDir() {
				suffix = "/"
			}
			lines = append(lines, "  "+name+"/"+cn+suffix)
		}
	}
	sort.Strings(lines)

	if len(lines) > maxTreeLines {
		lines = append(lines[:maxTreeLines], fmt.Sprintf("... (%d more entries)", len(lines)-maxTreeLines))
	}
	return strings.Join(lines, "\n") + "\n", nil
}

// extractJSON pulls a JSON object out of a response that may wrap it in markdown
// fences or prose.
func extractJSON(raw string) string {
	s := strings.TrimSpace(raw)
	if i := strings.Index(s, "```"); i >= 0 {
		s = s[i+3:]
		s = strings.TrimPrefix(s, "json")
		if j := strings.Index(s, "```"); j >= 0 {
			s = s[:j]
		}
	}
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return strings.TrimSpace(s)
}
