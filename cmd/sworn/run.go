package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/swornagent/sworn/internal/config"
	"github.com/swornagent/sworn/internal/run"
)

// cmdRun implements the `sworn run` subcommand.
//
//	sworn run --task "<description>" [--implementer-model <provider/model>]
//	           [--verifier-model <provider/model>] [--base <branch>]
//	           [--retry-cap <n>] [--escalation-models <m1,m2,...>]
//
// Model resolution (implementer + verifier):
//
//  1. --implementer-model / --verifier-model flag (explicit CLI)
//  2. $SWORN_IMPLEMENTER_MODEL / $SWORN_VERIFIER_MODEL env var
//  3. config file (implementer.model / verifier.model) — future, not yet
//
// Escalation models default to openai/gpt-4o-mini → openai/gpt-4o →
// openai/o3-mini → openai/o3 (cheapest to most capable). Override with
// --escalation-models or $SWORN_ESCALATION_MODELS (comma-separated).
func cmdRun(args []string) int {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	task := fs.String("task", "", "plain-language task description (required)")
	implModel := fs.String("implementer-model", "", "implementer model (provider/model)")
	verifierModel := fs.String("verifier-model", "", "verifier model (provider/model)")
	base := fs.String("base", "main", "base branch to merge into on PASS")
	retryCap := fs.Int("retry-cap", -1, "max retries before escalating to human (-1 = use all escalation models)")
	escalationFlag := fs.String("escalation-models", "", "comma-separated model escalation path (provider/model,...)")

	// Override usage to document model resolution.
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `usage: sworn run --task <description> [flags]

sworn run executes the full turnkey loop: implement → verify →
(on FAIL: retry/escalate up to N) → gated merge on PASS only.

flags:
`)
		fs.PrintDefaults()
		fmt.Fprint(os.Stderr, `
model resolution (implementer):
  $SWORN_IMPLEMENTER_MODEL > --implementer-model

model resolution (verifier):
  --verifier-model flag > $SWORN_VERIFIER_MODEL > config file (verifier.model)

escalation models:
  --escalation-models > $SWORN_ESCALATION_MODELS
  default: openai/gpt-4o-mini,openai/gpt-4o,openai/o3-mini,openai/o3

api keys:
  SWORN_<PROVIDER>_API_KEY (e.g. SWORN_OPENAI_API_KEY)
  SWORN_<PROVIDER>_BASE_URL (default: https://api.openai.com/v1 for openai)
`)
	}

	_ = fs.Parse(args)

	if *task == "" {
		fmt.Fprintln(os.Stderr, "sworn run: --task is required")
		fs.Usage()
		return 64
	}

	// Resolve implementer model: flag > env > (first escalation model).
	impl := *implModel
	if impl == "" {
		impl = os.Getenv("SWORN_IMPLEMENTER_MODEL")
	}

	// Resolve verifier model: flag > env > config.
	verifier := *verifierModel
	if verifier == "" {
		cfg, cfgErr := config.Load()
		if cfgErr == nil {
			resolved, err := config.ResolveVerifierModel("", cfg)
			if err == nil {
				verifier = resolved
			}
		}
		// Fallback: try env directly.
		if verifier == "" {
			verifier = os.Getenv("SWORN_VERIFIER_MODEL")
		}
	}
	if verifier == "" {
		fmt.Fprintln(os.Stderr, "sworn run: verifier model not configured — set --verifier-model, $SWORN_VERIFIER_MODEL, or run 'sworn init'")
		return 2
	}

	// Resolve escalation models: flag > env > default.
	var escalationModels []string
	if *escalationFlag != "" {
		for _, m := range strings.Split(*escalationFlag, ",") {
			m = strings.TrimSpace(m)
			if m != "" {
				escalationModels = append(escalationModels, m)
			}
		}
	}
	if len(escalationModels) == 0 {
		if env := os.Getenv("SWORN_ESCALATION_MODELS"); env != "" {
			for _, m := range strings.Split(env, ",") {
				m = strings.TrimSpace(m)
				if m != "" {
					escalationModels = append(escalationModels, m)
				}
			}
		}
	}
	// If still empty, run.DefaultEscalationModels will be used.

	err := run.Run(context.Background(), run.Options{
		Task:             *task,
		ImplementerModel: impl,
		VerifierModel:    verifier,
		Base:             *base,
		RetryCap:         *retryCap,
		EscalationModels: escalationModels,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn run: %v\n", err)
		return 1
	}
	return 0
}
