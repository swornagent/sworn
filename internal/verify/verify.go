// Package verify runs the SwornAgent verification protocol: a deterministic
// $0 first-pass, then an adversarial fresh-context model verification. It is
// provider-neutral and host-neutral — it operates only on the spec -> diff
// (-> proof) triple and a Verifier, never on a git host or a specific model.
package verify

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/swornagent/sworn/internal/model"
	"github.com/swornagent/sworn/internal/prompt"
	"github.com/swornagent/sworn/internal/verdict"
)

// systemPrompt is the sworn-authored stateless judge prompt, vendored at
// build time via go:embed (internal/prompt). It instructs the model to judge
// from SPEC+DIFF+PROOF only with a verdict-leading reply — no tools, no repo.
var systemPrompt = prompt.VerifyStateless()// Input is everything a verification needs.
type Input struct {
	SpecPath  string
	DiffPath  string // "-" reads stdin
	ProofPath string // optional in S1
	Model     string
	Verifier  model.Verifier // nil -> Unconfigured (fails closed)
}

// Run executes the protocol and returns a fail-closed Result.
func Run(ctx context.Context, in Input) verdict.Result {
	// --- Deterministic first-pass ($0 gate) ---
	spec, err := readNonEmpty(in.SpecPath)
	if err != nil {
		return blocked("first_pass:spec", err.Error())
	}
	diff, err := readNonEmpty(in.DiffPath)
	if err != nil {
		return blocked("first_pass:diff", err.Error())
	}
	proof := ""
	if in.ProofPath != "" {
		proof, _ = readFile(in.ProofPath)
	}

	// --- Adversarial model verification ---
	v := in.Verifier
	if v == nil {
		v = model.Unconfigured{}
	}
	text, cost, err := v.Verify(ctx, systemPrompt, buildPayload(spec, diff, proof))
	if err != nil {
		return blocked("verifier_dispatch", err.Error())
	}
	return parseVerdict(text, cost)
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

// parseVerdict maps the model's leading token to a Result. Conservative: only a
// clear PASS passes; an unrecognised reply BLOCKS.
func parseVerdict(text string, cost float64) verdict.Result {
	t := strings.TrimSpace(text)
	upper := strings.ToUpper(t)
	switch {
	case strings.HasPrefix(upper, "PASS"):
		return verdict.Result{Verdict: verdict.Pass, Rationale: t, CostUSD: cost}
	case strings.HasPrefix(upper, "FAIL"):
		return verdict.Result{Verdict: verdict.Fail, FailedGate: "adversarial", Rationale: t, CostUSD: cost}
	case strings.HasPrefix(upper, "BLOCKED"):
		return verdict.Result{Verdict: verdict.Blocked, FailedGate: "adversarial", Rationale: t, CostUSD: cost}
	case strings.HasPrefix(upper, "INCONCLUSIVE"):
		return verdict.Result{Verdict: verdict.Inconclusive, FailedGate: "adversarial", Rationale: t, CostUSD: cost}
	default:
		return verdict.Result{Verdict: verdict.Blocked, FailedGate: "unparseable_verdict",
			Rationale: "verifier reply did not start with PASS/FAIL/BLOCKED/INCONCLUSIVE", CostUSD: cost}
	}
}
func blocked(gate, why string) verdict.Result {
	return verdict.Result{Verdict: verdict.Blocked, FailedGate: gate, Rationale: why}
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
