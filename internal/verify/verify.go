// Package verify runs the SwornAgent verification protocol: a deterministic
// $0 first-pass (RunFirstPass), then an adversarial agentic verification
// (RunAgentic). It is provider-neutral and host-neutral.
//
// Goroutine-safety: stateless by construction — no package-level mutable vars
// that are written during RunFirstPass() or RunAgentic(); each call is
// independent. knownBoundaryPatterns and mockMarkerPatterns are initialised at
// program start and are read-only thereafter (concurrent reads are safe in Go).
// Verified by S03 concurrent_test.go under -race.
package verify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/swornagent/sworn/internal/agent"
	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/model"
	"github.com/swornagent/sworn/internal/prompt"
	"github.com/swornagent/sworn/internal/state"
	"github.com/swornagent/sworn/internal/verdict"
)

// verifierRolePrompt is the full Baton verifier.md role prompt (the agentic
// verifier). It instructs the model to re-run tests, read live repo state,
// and return PASS/FAIL/BLOCKED. Used by RunAgentic.
var verifierRolePrompt = prompt.Verifier()

// Input is everything a verification needs.
type Input struct {
	SpecPath  string
	DiffPath  string // "-" reads stdin
	ProofPath string // when set, gated by RunFirstPass (exists, non-empty, valid JSON for .json)
	// ProofRequired makes an EMPTY ProofPath a BLOCKED first-pass verdict
	// (Rule 6 — absence must not upgrade to PASS). The standalone CLI sets
	// this: `sworn verify` is the proof-bundle gate. Left false by callers
	// that own their own absence gate (RunSlice's proof-mandatory check) or
	// that deliberately measure spec/diff structure only (bench).
	ProofRequired bool
	Model         string
	Verifier      model.Verifier   // nil -> Unconfigured (fails closed)
	OpenDeferrals []state.Deferral // Rule-2 deferrals from status.json (S10 no-mock-boundary)
}

// RunFirstPass is a structural pre-flight gate ($0 cost) that catches
// blocker-level issues before the expensive agentic verifier is dispatched.
// It is purely deterministic — no model call, no token spend. It checks:
//
//	(a) spec is present and non-empty
//	(b) diff is present and non-empty
//	(c) the proof bundle, when supplied (or required — Input.ProofRequired),
//	    exists, is non-empty, and parses as JSON for .json bundles (Rule 6)
//	(d) no undeclared boundary mocks (S10 Rule 7/Rule 2 enforcement)
//
// RunFirstPass MUST NOT be used to drive state transitions to verified.
// A PASS from RunFirstPass only means "no structural blockers found";
// only the agentic verifier (RunAgentic) can drive state transitions.
//
// The function signature accepts Input for caller compatibility; Verifier,
// Model, and OpenDeferrals fields are consumed deterministically; no model
// dispatch occurs.
func RunFirstPass(ctx context.Context, in Input) verdict.Result {
	// --- Deterministic first-pass ($0 gate) ---
	_, err := readNonEmpty(in.SpecPath)
	if err != nil {
		return blocked("first_pass:spec", err.Error())
	}
	diff, err := readNonEmpty(in.DiffPath)
	if err != nil {
		return blocked("first_pass:diff", err.Error())
	}
	// --- Proof-bundle gate (Rule 6) ---
	// A supplied proof must exist, be non-empty, and (for .json bundles)
	// parse as JSON — a missing/empty/unparseable proof must never upgrade
	// to PASS. An empty ProofPath blocks only when the caller marked proof
	// required (see Input.ProofRequired).
	if in.ProofPath == "" {
		if in.ProofRequired {
			return blocked("first_pass:proof", "no proof bundle provided — fail closed (Rule 6)")
		}
	} else {
		proofContent, err := readNonEmpty(in.ProofPath)
		if err != nil {
			return blocked("first_pass:proof", err.Error())
		}
		if strings.HasSuffix(in.ProofPath, ".json") && !json.Valid([]byte(proofContent)) {
			return blocked("first_pass:proof", display(in.ProofPath)+" is not valid JSON")
		}
	}

	// --- Boundary-mock check (S10 first-pass gate) ---
	report := CheckBoundaryMocks(diff, in.OpenDeferrals)
	if len(report.UndeclaredMocks) > 0 {
		var b strings.Builder
		b.WriteString("Undeclared boundary mock(s) — fail closed per Rule 7/Rule 2:\n")
		for _, m := range report.UndeclaredMocks {
			b.WriteString(fmt.Sprintf("  - %s (boundary: %s) at %s:%d\n", m.MockType, m.Boundary, m.File, m.Line))
		}
		return verdict.Result{
			Verdict:    verdict.Fail,
			FailedGate: "boundary_mock",
			Rationale:  b.String(),
		}
	}
	var rationale string
	if len(report.DeclaredMocks) > 0 {
		var b strings.Builder
		b.WriteString("First-pass PASS with declared boundary mock(s) — allowed with known deferral:\n")
		for _, m := range report.DeclaredMocks {
			b.WriteString(fmt.Sprintf("  - %s (boundary: %s) at %s:%d\n", m.MockType, m.Boundary, m.File, m.Line))
		}
		rationale = b.String()
	}

	return verdict.Result{
		Verdict:   verdict.Pass,
		Rationale: rationale,
	}
} // verifierEmitSchema is the model-authored JUDGEMENT subset of
// verifier-verdict-v1 handed to ChatStructured (ADR-0011 authoring path). It
// deliberately stays inside OpenAI's strict-mode keyword subset — no minLength /
// pattern / format (those would break a strict response_format target; see the
// internal/model/structured.go strict-projection constraint). The canonical
// verifier-verdict-v1.json schema (which DOES carry minLength/format) is what
// baton.ValidateSchema validates the stamped emission against; the two agree on
// the judgement core and any drift fails closed (validation → INCONCLUSIVE).
// The "title" sets the OpenAI json_schema name (^[a-zA-Z0-9_-]+$).
var verifierEmitSchema = []byte(`{
  "title": "verifier-verdict-v1",
  "type": "object",
  "additionalProperties": false,
  "required": ["verdict", "rationale"],
  "properties": {
    "verdict": { "type": "string", "enum": ["PASS", "FAIL", "BLOCKED", "INCONCLUSIVE"] },
    "rationale": { "type": "string" },
    "failed_gate": { "type": "string" },
    "routing": { "type": "string", "enum": ["needs_planner", "needs_human", "needs_implementer"] },
    "violations": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["gate", "description"],
        "properties": {
          "gate": { "type": "string" },
          "description": { "type": "string" },
          "evidence": { "type": "string" },
          "proposed_amendment": { "type": "string" }
        }
      }
    }
  }
}`)

// structuredVerdict is the typed view of the model's emitted judgement. It is
// parsed from the validated emission, never scraped from prose.
type structuredVerdict struct {
	Verdict    string `json:"verdict"`
	Rationale  string `json:"rationale"`
	FailedGate string `json:"failed_gate"`
	Routing    string `json:"routing"`
	Violations []struct {
		Gate        string `json:"gate"`
		Description string `json:"description"`
		Evidence    string `json:"evidence"`
	} `json:"violations"`
}

// RunAgentic executes the agentic verification protocol: it dispatches the full
// verifier.md role prompt and the SPEC+DIFF+PROOF payload, and the verifier
// EMITS its verdict as a schema-constrained structured-output object
// (verifier-verdict-v1), which is validated before acceptance (ADR-0011
// authoring path). This replaces the prior prose reply scraped by HasPrefix —
// the one live ADR-0009 invariant breach in the hot path.
//
// Fail-closed at every boundary: a verifier driver that cannot emit structured
// output, a dispatch error, a malformed emission, or an emission that fails
// schema validation (e.g. a FAIL verdict that cites no violations) all resolve
// to INCONCLUSIVE — never an optimistic or scraped verdict.
//
// The caller (RunSlice) is responsible for the proof-mandatory check and
// no-mock wiring before calling RunAgentic, and stamps the identity triple
// (slice_id, release) into status.json post-emission — the model payload is
// judgement-only (ADR-0011 §3.3 g).
func RunAgentic(ctx context.Context, spec, diff, proof string, verifierAgent agent.Agent) (verdict.Result, error) {
	userPayload := buildPayload(spec, diff, proof)

	messages := []model.ChatMessage{
		{Role: "system", Content: verifierRolePrompt},
		{Role: "user", Content: userPayload},
	}

	// The verifier must emit a schema-constrained object. A driver that does not
	// advertise CapStructuredOutput cannot be trusted to a prose verdict any
	// more (ADR-0009) — fail closed to INCONCLUSIVE.
	so, ok := verifierAgent.(model.StructuredOutput)
	if !ok {
		return inconclusive("verifier_structured_unsupported",
			"verifier driver does not support structured output (ADR-0011) — cannot emit verifier-verdict-v1"), nil
	}

	resp, err := so.ChatStructured(ctx, messages, verifierEmitSchema)
	if err != nil {
		// Terminal provider errors (KindAuth/KindCredits — revoked key,
		// exhausted credits) can never succeed on retry or model escalation:
		// surface BLOCKED so triage Halts (Blocked → Halt) instead of walking
		// the retry/escalation ladder at real implementer spend — mirroring
		// the implementer path's terminal-error halt (S09 AC1).
		if model.IsTerminal(err) {
			return blockedTerminal(err), nil
		}
		return inconclusive("verifier_structured_dispatch", err.Error()), nil
	}
	if len(resp.Choices) == 0 {
		return inconclusive("verifier_structured_dispatch", "empty response choices"), nil
	}

	return acceptStructuredVerdict(resp.Choices[0].Message.Content, resp.Usage), nil
}

// acceptStructuredVerdict validates the emitted judgement against the canonical
// verifier-verdict-v1 schema and maps it to a verdict.Result. Any failure along
// the way is INCONCLUSIVE (fail-closed) — the verdict is taken from the typed,
// validated object, never inferred from prose.
func acceptStructuredVerdict(emitted string, usage *model.UsageBlock) verdict.Result {
	cost := computeAgenticCost(usage)

	// Stamp the binary-owned fields the model does not author, then validate the
	// completed record against the canonical schema (this is where the
	// FAIL/BLOCKED ⇒ violations≥1 invariant is enforced).
	var obj map[string]any
	if err := json.Unmarshal([]byte(emitted), &obj); err != nil {
		return inconclusiveCost("verifier_structured_malformed",
			fmt.Sprintf("emitted verdict is not a JSON object: %v", err), cost)
	}
	obj["schema_version"] = 1
	obj["$schema"] = "https://baton.sawy3r.net/schemas/verifier-verdict-v1.json"
	stamped, err := json.Marshal(obj)
	if err != nil {
		return inconclusiveCost("verifier_structured_malformed", err.Error(), cost)
	}
	if err := baton.ValidateSchema("verifier-verdict-v1", stamped); err != nil {
		return inconclusiveCost("verifier_verdict_invalid",
			fmt.Sprintf("emitted verdict failed verifier-verdict-v1 validation: %v", err), cost)
	}

	var sv structuredVerdict
	if err := json.Unmarshal([]byte(emitted), &sv); err != nil {
		return inconclusiveCost("verifier_structured_malformed", err.Error(), cost)
	}

	res := verdict.Result{
		Verdict:    verdict.Verdict(sv.Verdict), // schema-validated to the 4-value enum
		Rationale:  sv.Rationale,
		FailedGate: sv.FailedGate,
		Routing:    sv.Routing,
		CostUSD:    cost,
	}
	if usage != nil {
		res.InputTokens = int64(usage.PromptTokens)
		res.OutputTokens = int64(usage.CompletionTokens)
	}
	for _, v := range sv.Violations {
		if v.Gate != "" {
			res.Violations = append(res.Violations, v.Gate+": "+v.Description)
		} else {
			res.Violations = append(res.Violations, v.Description)
		}
	}
	return res
}

// computeAgenticCost computes a nominal cost from a UsageBlock.
// Uses the same ~$2/1M tokens estimate as agent.computeCost for consistency.
func computeAgenticCost(usage *model.UsageBlock) float64 {
	if usage == nil {
		return 0
	}
	return float64(usage.TotalTokens) * 0.000002 // ~$2/1M tokens
}

func buildPayload(spec, diff, proof string) string {
	var b strings.Builder
	b.WriteString("## SPEC\n")
	b.WriteString(spec)
	b.WriteString("\n\n## DIFF\n")
	b.WriteString(diff)
	if proof != "" {
		b.WriteString("\n\n## PROOF\n")
		b.WriteString(proof)
	}
	return b.String()
}

// NOTE (ADR-0011): parseVerdict / firstVerdictLine / stripMarkdown — the prose
// HasPrefix verdict scrape — were deleted with the keystone Step-3 pilot. The
// verifier now EMITS a schema-constrained verifier-verdict-v1 object
// (acceptStructuredVerdict above); there is no prose verdict to parse.

func blocked(gate, why string) verdict.Result {
	return verdict.Result{Verdict: verdict.Blocked, FailedGate: gate, Rationale: why}
}

// blockedTerminal maps a terminal verifier-dispatch error (model.IsTerminal:
// KindAuth / KindCredits) to a BLOCKED verdict. BLOCKED — not INCONCLUSIVE —
// because the triage policy retries/escalates Inconclusive but Halts on
// Blocked, and dead verifier credentials fail identically on every attempt.
// Mirrors the kind-label + UserMessage format of the implementer path's
// terminal halt (internal/run/slice.go, S09 AC1).
func blockedTerminal(err error) verdict.Result {
	reason := err.Error()
	var me *model.Error
	if model.AsError(err, &me) {
		k := me.Kind.String()
		reason = fmt.Sprintf("Kind%s%s: %s", strings.ToUpper(k[:1]), k[1:], me.UserMessage())
	}
	return blocked("verifier_terminal_error",
		reason+" — halting; check verifier provider credentials")
}

// inconclusive builds a fail-closed INCONCLUSIVE result for the structured
// authoring path: a verifier that could not emit, or emitted an unparseable or
// schema-invalid object, is treated as not-yet-determinate (re-verify), never
// as a scraped or optimistic verdict (ADR-0011).
func inconclusive(gate, why string) verdict.Result {
	return verdict.Result{Verdict: verdict.Inconclusive, FailedGate: gate, Rationale: why}
}

// inconclusiveCost is inconclusive with the dispatch cost attached (the call was
// made and billed even though the emission was not acceptable).
func inconclusiveCost(gate, why string, cost float64) verdict.Result {
	return verdict.Result{Verdict: verdict.Inconclusive, FailedGate: gate, Rationale: why, CostUSD: cost}
}

func readNonEmpty(path string) (string, error) {
	s, err := readFile(path)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(s) == "" {
		return "", fmt.Errorf("%s is empty", display(path))
	}
	return s, nil
}

func readFile(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("no path provided")
	}
	if path == "-" {
		b, err := io.ReadAll(os.Stdin)
		return string(b), err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func display(path string) string {
	if path == "-" {
		return "stdin"
	}
	return path
}

// --- S10: Boundary-mock detection ---

// BoundaryMock records one detected mock at a validated boundary.
type BoundaryMock struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Boundary string `json:"boundary"` // "db", "auth", "entitlement"
	MockType string `json:"mock_type"`
	Declared bool   `json:"declared"`
	Deferral string `json:"deferral,omitempty"`
}

// BoundaryMockReport groups detected mocks by declaration status.
type BoundaryMockReport struct {
	UndeclaredMocks []BoundaryMock `json:"undeclared_mocks"`
	DeclaredMocks   []BoundaryMock `json:"declared_mocks"`
}

// boundaryPattern associates a keyword string with a validated boundary.
type boundaryPattern struct {
	Keyword  string // keyword to match in a line
	Boundary string // canonical boundary name
}

// knownBoundaryPatterns list patterns that, when combined with a mock/stub/fake
// construct, indicate a mock at a validated boundary.
var knownBoundaryPatterns = []boundaryPattern{
	{Keyword: "sql.", Boundary: "db"},
	{Keyword: "database/sql", Boundary: "db"},
	{Keyword: "*sql.DB", Boundary: "db"},
	{Keyword: "*sql.Tx", Boundary: "db"},
	{Keyword: "*sql.Conn", Boundary: "db"},
	{Keyword: "sql.DB", Boundary: "db"},
	{Keyword: "sql.Tx", Boundary: "db"},
	{Keyword: "sql.Conn", Boundary: "db"},
	{Keyword: "DB", Boundary: "db"},
	{Keyword: "auth", Boundary: "auth"},
	{Keyword: "Auth", Boundary: "auth"},
	{Keyword: "Authenticate", Boundary: "auth"},
	{Keyword: "Authorize", Boundary: "auth"},
	{Keyword: "entitle", Boundary: "entitlement"},
	{Keyword: "Entitle", Boundary: "entitlement"},
	{Keyword: "premium", Boundary: "entitlement"},
	{Keyword: "Premium", Boundary: "entitlement"},
	{Keyword: "subscription", Boundary: "entitlement"},
	{Keyword: "Subscription", Boundary: "entitlement"},
	{Keyword: "credits", Boundary: "entitlement"},
	{Keyword: "Credits", Boundary: "entitlement"},
	{Keyword: "keyless", Boundary: "entitlement"},
	{Keyword: "Keyless", Boundary: "entitlement"},
	{Keyword: "claude -p", Boundary: "entitlement"},
}

// mockMarkerPatterns are tokens on a line that suggest a mock/stub/fake/test
// double is being created or assigned.  At least one boundary pattern must also
// match for the line to be flagged.
var mockMarkerPatterns = []string{
	"mock", "Mock", "MOCK",
	"fake", "Fake", "FAKE",
	"stub", "Stub", "STUB",
	"testdouble", "TestDouble",
	"newMock", "NewMock",
	"newTest", "NewTest",
}

// CheckBoundaryMocks scans diff content for mocks/stubs at validated boundaries
// and cross-references against open deferrals.  Returns a report of undeclared
// (violations) and declared (known deferrals) boundary mocks.
//
// Detection is heuristic: a line must contain at least one boundary pattern AND
// at least one mock-marker pattern to be flagged.  If the mock description
// (boundary + mock type) matches any open deferral, it is treated as declared.
func CheckBoundaryMocks(diffContent string, openDeferrals []state.Deferral) BoundaryMockReport {
	var report BoundaryMockReport
	lines := strings.Split(diffContent, "\n")
	for i, raw := range lines {
		line := i + 1 // 1-indexed
		t := strings.TrimSpace(raw)

		// Skip non-added lines (---) and context lines.
		if !strings.HasPrefix(t, "+") && !strings.HasPrefix(t, "-") {
			continue
		}
		content := strings.TrimPrefix(strings.TrimPrefix(t, "+"), "-")

		// Check for mock markers.
		hasMock := false
		for _, marker := range mockMarkerPatterns {
			if strings.Contains(content, marker) {
				hasMock = true
				break
			}
		}
		if !hasMock {
			continue
		}

		// Check for boundary patterns.
		matched := ""
		for _, bp := range knownBoundaryPatterns {
			if strings.Contains(content, bp.Keyword) {
				matched = bp.Boundary
				break
			}
		}
		if matched == "" {
			continue
		}

		// Extract a compact mock-type description.
		mockType := extractMockType(content)

		// Check against open deferrals.
		bm := BoundaryMock{
			File:     "diff",
			Line:     line,
			Boundary: matched,
			MockType: mockType,
		}
		if isDeclared(mockType, matched, openDeferrals) {
			bm.Declared = true
			report.DeclaredMocks = append(report.DeclaredMocks, bm)
		} else {
			report.UndeclaredMocks = append(report.UndeclaredMocks, bm)
		}
	}
	return report
}

// extractMockType extracts a compact description of the mock from a line.
// It returns the mock-marker token and surrounding context, trimmed to 80 chars.
func extractMockType(line string) string {
	lower := strings.ToLower(line)
	for _, marker := range mockMarkerPatterns {
		idx := strings.Index(line, marker)
		if idx >= 0 {
			start := idx - 15
			if start < 0 {
				start = 0
			}
			end := idx + len(marker) + 15
			if end > len(line) {
				end = len(line)
			}
			snippet := strings.TrimSpace(line[start:end])
			if len(snippet) > 80 {
				snippet = snippet[:77] + "..."
			}
			// Single occurrence per line is sufficient.
			if strings.Contains(lower, "mock") {
				return "mock: " + snippet
			}
			if strings.Contains(lower, "fake") {
				return "fake: " + snippet
			}
			if strings.Contains(lower, "stub") {
				return "stub: " + snippet
			}
			return "testdouble: " + snippet
		}
	}
	// Fallback — take first 60 chars.
	s := line
	if len(s) > 60 {
		s = s[:57] + "..."
	}
	return s
}

// isDeclared checks whether a mock at a given boundary matches any open deferral.
// Matching is case-insensitive substring over the deferral's description-bearing
// fields (Item + Why) only — not Tracking/Acknowledgement, which are IDs/URLs
// that could spuriously contain a boundary keyword and over-declare (AC-05 / D3).
// Each deferral is checked for the boundary name AND a mock/fake/stub keyword. A
// deferral whose item/why reads "db mock for integration tests" matches a
// db-boundary mock; enforcement stays at least as strict as the old []string match.
func isDeclared(mockType, boundary string, openDeferrals []state.Deferral) bool {
	for _, d := range openDeferrals {
		dl := strings.ToLower(d.Item + " " + d.Why)
		if strings.Contains(dl, strings.ToLower(boundary)) &&
			(strings.Contains(dl, "mock") || strings.Contains(dl, "fake") ||
				strings.Contains(dl, "stub") || strings.Contains(dl, "testdouble")) {
			return true
		}
	}
	return false
}
