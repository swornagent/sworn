package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"errors"

	"github.com/swornagent/sworn/internal/account"
	"github.com/swornagent/sworn/internal/config"
	"github.com/swornagent/sworn/internal/db"
	"github.com/swornagent/sworn/internal/model"
	"github.com/swornagent/sworn/internal/run"
	"github.com/swornagent/sworn/internal/supervisor"
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

	// ── Load .env files ────────────────────────────────────────────────
	if err := model.LoadDotEnv(); err != nil {
		fmt.Fprintf(os.Stderr, "sworn run: load .env: %v\n", err)
		return 1
	}
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

	// ── Load config ────────────────────────────────────────────────────
	cfg, cfgErr := config.Load()
	if cfgErr != nil {
		fmt.Fprintf(os.Stderr, "sworn run: load config: %v\n", cfgErr)
		return 1
	}

	// ── Resolve implement timeout ─────────────────────────────────────
	// Precedence: flag > env > default. No config-file tier — touching
	// internal/config/config.go for S42 was the source of the BLOCKED verdict.
	implementTimeout := resolveImplementTimeout(*implTimeout, os.Getenv("SWORN_IMPLEMENT_TIMEOUT"))

	// ── Resolve verifier model ─────────────────────────────────────────
	verifier, err := config.ResolveVerifierModel(*verifierModel, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn run: %v\n", err)
		return 2
	}

	// ── Resolve implementer model ──────────────────────────────────────
	impl, err := config.ResolveImplementerModel(*implModel, cfg, "", "", "quality", 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn run: %v\n", err)
		return 2
	}

	// ── Resolve escalation models ──────────────────────────────────────
	escalationModels := config.ResolveEscalationModels(parseEscalationFlag(*escalationFlag), cfg)

	// ── Resolve max attempts ───────────────────────────────────────────
	maxAttempts := config.ResolveMaxAttempts(*retryCap, cfg)

	// Load credentials for the notifier (shared by both modes).
	credsDir := filepath.Dir(account.CredentialsPath())
	creds, _ := account.Load(credsDir)

	var webhookURL string
	if creds != nil {
		webhookURL = creds.WebhookURL
	}
	notifier := account.NewNotifier(webhookURL, creds)

	// ── Parallel mode ─────────────────────────────────────────────────
	if *parallel {
		database, dbErr := openDefaultDB()
		if dbErr != nil {
			fmt.Fprintf(os.Stderr, "sworn run: open database: %v\n", dbErr)
			return 1
		}
		defer database.Close()

		// Open the release-specific event store so events survive process exit.
		eventDB, evErr := supervisor.Open(*releaseName, ".")
		if evErr != nil {
			fmt.Fprintf(os.Stderr, "sworn run: open event store: %v\n", evErr)
			database.Close()
			return 1
		}
		defer eventDB.Close()
		runSliceFn := func(ctx context.Context, worktreeRoot, specPath, statusPath string) error {
			return run.RunSlice(ctx, worktreeRoot, specPath, statusPath, run.RunSliceOptions{
				ImplementerModel: impl,
				VerifierModel:    verifier,
				EscalationModels: escalationModels,
				RetryCap:         maxAttempts,
				ImplementTimeout: implementTimeout,
				Notifier:         notifier,
			})
		}
		err = run.RunParallel(context.Background(), run.ParallelOptions{
			ReleaseName:   *releaseName,
			WorkspaceRoot: ".",
			DB:            database,
			RunSliceFn:    runSliceFn,
			ProjectDir:    "sworn",
			Notifier:      notifier,
		})
		if err != nil {
			printModelError(err)
			fmt.Fprintf(os.Stderr, "sworn run: parallel: %v\n", err)
			return 1
		}
		return 0
	}
	// ── Single-slice mode ──────────────────────────────────────────────
	err = run.Run(context.Background(), run.Options{
		Task:             *task,
		ImplementerModel: impl,
		VerifierModel:    verifier,
		Base:             *base,
		RetryCap:         maxAttempts,
		EscalationModels: escalationModels,
		ImplementTimeout: implementTimeout,
		Notifier:         notifier,
	})
	if err != nil {
		printModelError(err)
		fmt.Fprintf(os.Stderr, "sworn run: %v\n", err)
		return 1
	}
	return 0
}

// resolveImplementTimeout returns the per-attempt implement timeout from the
// first available source, in precedence order:
//
//  1. --implement-timeout flag (non-zero)
//  2. $SWORN_IMPLEMENT_TIMEOUT env var (parsed as duration string)
//  3. run.DefaultImplementTimeout constant (15m)
//
// A negative flag or env value means "no timeout" (opt-out). Zero means "use
// default". There is intentionally no config-file tier — that was the source
// of the S42 BLOCKED verdict (cross-track collision with config.go ownership).
func resolveImplementTimeout(flagVal time.Duration, envVal string) time.Duration {
	if flagVal != 0 {
		if flagVal < 0 {
			return 0 // opt-out
		}
		return flagVal
	}
	if envVal != "" {
		if d, err := time.ParseDuration(envVal); err == nil {
			if d < 0 {
				return 0 // opt-out
			}
			return d
		}
	}
	return run.DefaultImplementTimeout
} // parseEscalationFlag splits a comma-separated escalation models string into
// a []string. Returns nil when the flag is empty.
func parseEscalationFlag(raw string) []string {
	if raw == "" {
		return nil
	}
	var models []string
	for _, m := range strings.Split(raw, ",") {
		m = strings.TrimSpace(m)
		if m != "" {
			models = append(models, m)
		}
	}
	return models
}

// openDefaultDB opens the default sworn SQLite database with schema initialization.
func openDefaultDB() (*sql.DB, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getwd: %w", err)
	}
	return db.Open(filepath.Join(wd, db.DefaultDir, db.DefaultName))
}
// printModelError unwraps a *model.Error from err (via errors.As) and
// prints its UserMessage to stderr. This gives the user actionable
// guidance (e.g. "check the API key", "out of credits") instead of
// raw provider JSON. If err is not a *model.Error, nothing is printed.
func printModelError(err error) {
	var me *model.Error
	if errors.As(err, &me) {
		fmt.Fprintf(os.Stderr, "sworn run: %s\n", me.UserMessage())
	}
}
