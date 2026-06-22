package main

import (
	"context"
	"database/sql"
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
//	           [--implement-timeout <duration>]
//
//	sworn run --parallel --release <name> [--verifier-model <provider/model>]
//	           [--implementer-model <provider/model>]
//	           [--implement-timeout <duration>]
func cmdRun(args []string) int {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	task := fs.String("task", "", "plain-language task description (required for single-slice mode)")
	implModel := fs.String("implementer-model", "", "implementer model (provider/model)")
	verifierModel := fs.String("verifier-model", "", "verifier model (provider/model)")
	base := fs.String("base", "main", "base branch to merge into on PASS")
	retryCap := fs.Int("retry-cap", -1, "max retries before escalating to human (-1 = use all escalation models)")
	escalationFlag := fs.String("escalation-models", "", "comma-separated model escalation path (provider/model,...)")
	parallel := fs.Bool("parallel", false, "run tracks concurrently from release board")
	releaseName := fs.String("release", "", "release name for --parallel mode (e.g. 2026-06-19-safe-parallelism)")
	implTimeout := fs.Duration("implement-timeout", 0, "per-attempt implement deadline (0 = use default; negative = no timeout)")

	_ = fs.Parse(args)

	// ── Basic CLI usage validation (before model resolution) ────────────
	if *parallel {
		if *releaseName == "" {
			fmt.Fprintln(os.Stderr, "sworn run: --release is required with --parallel")
			return 64
		}
	} else if *task == "" {
		fmt.Fprintln(os.Stderr, "sworn run: --task is required (or use --parallel --release)")
		return 64
	}

	// ── Load config ─────────────────────────────────────────────────────
	cfg, cfgErr := config.Load()
	if cfgErr != nil {
		fmt.Fprintf(os.Stderr, "sworn run: config warning: %v\n", cfgErr)
		// Non-fatal — env vars and flags still work.
	}

	// ── Resolve implement timeout ───────────────────────────────────────
	timeout := config.ResolveImplementTimeout(*implTimeout, os.Getenv("SWORN_IMPLEMENT_TIMEOUT"), cfg.Implementer.Timeout)

	// ── Resolve verifier model ─────────────────────────────────────────
	verifier := resolveVerifierModel(*verifierModel)
	if verifier == "" {
		fmt.Fprintln(os.Stderr, "sworn run: verifier model not configured — set --verifier-model, $SWORN_VERIFIER_MODEL, or run 'sworn init'")
		return 2
	}

	// ── Resolve implementer model ───────────────────────────────────────
	impl := *implModel
	if impl == "" {
		impl = os.Getenv("SWORN_IMPLEMENTER_MODEL")
	}

	// ── Resolve escalation models ───────────────────────────────────────
	escalationModels := resolveEscalationModels(*escalationFlag)

	// ── Parallel mode ─────────────────────────────────────────────────
	if *parallel {
		database, err := openDefaultDB()
		if err != nil {
			fmt.Fprintf(os.Stderr, "sworn run: open database: %v\n", err)
			return 1
		}
		defer database.Close()

		runSliceFn := func(ctx context.Context, worktreeRoot, specPath, statusPath string) error {
			return run.RunSlice(ctx, worktreeRoot, specPath, statusPath, run.RunSliceOptions{
				ImplementerModel: impl,
				VerifierModel:    verifier,
				EscalationModels: escalationModels,
				ImplementTimeout: timeout,
			})
		}

		err = run.RunParallel(context.Background(), run.ParallelOptions{
			ReleaseName:   *releaseName,
			WorkspaceRoot: ".",
			DB:            database,
			RunSliceFn:    runSliceFn,
			ProjectDir:    "sworn",
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "sworn run: parallel: %v\n", err)
			return 1
		}
		return 0
	}

	// ── Single-slice mode ──────────────────────────────────────────────
	err := run.Run(context.Background(), run.Options{
		Task:             *task,
		ImplementerModel: impl,
		VerifierModel:    verifier,
		Base:             *base,
		RetryCap:         *retryCap,
		EscalationModels: escalationModels,
		ImplementTimeout: timeout,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn run: %v\n", err)
		return 1
	}
	return 0
}

// resolveVerifierModel resolves the verifier model with precedence:
// flag > env > config.
func resolveVerifierModel(flagVal string) string {
	if flagVal != "" {
		return flagVal
	}
	if env := os.Getenv("SWORN_VERIFIER_MODEL"); env != "" {
		return env
	}
	return ""
}

// resolveEscalationModels resolves escalation models with precedence:
// flag > env.
func resolveEscalationModels(flagVal string) []string {
	if flagVal != "" {
		var models []string
		for _, m := range strings.Split(flagVal, ",") {
			m = strings.TrimSpace(m)
			if m != "" {
				models = append(models, m)
			}
		}
		return models
	}
	if env := os.Getenv("SWORN_ESCALATION_MODELS"); env != "" {
		var models []string
		for _, m := range strings.Split(env, ",") {
			m = strings.TrimSpace(m)
			if m != "" {
				models = append(models, m)
			}
		}
		return models
	}
	return nil
}

// openDefaultDB opens the default sworn SQLite database.
func openDefaultDB() (*sql.DB, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getwd: %w", err)
	}
	dbPath := wd + "/.sworn/sworn.db"

	driver := os.Getenv("SWORN_DB_DRIVER")
	if driver == "" {
		driver = "sqlite"
	}

	db, err := sql.Open(driver, dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db %s: %w", dbPath, err)
	}
	db.SetMaxOpenConns(1)
	return db, nil
}