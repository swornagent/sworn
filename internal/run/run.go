// Package run orchestrates the sworn run turnkey loop: implement → verify →
// (on FAIL: retry/escalate up to N) → gated merge on PASS only.
//
// It is the single-slice v0.1 engine: a task string becomes an auto-generated
// slice, the implementer model builds it, the verifier model checks it, and the
// binary merges only when the verifier returns PASS.
//
// Stdlib only — zero runtime dependencies beyond the internal packages.
package run

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/swornagent/sworn/internal/account"
	"github.com/swornagent/sworn/internal/db"
	"github.com/swornagent/sworn/internal/driver/registry"
	"github.com/swornagent/sworn/internal/git"
	"github.com/swornagent/sworn/internal/implement"
	"github.com/swornagent/sworn/internal/spec"
	"github.com/swornagent/sworn/internal/state"
	"github.com/swornagent/sworn/internal/supervisor"
)

// DefaultEscalationModels is the default model escalation path when none is
// provided. Each entry is a "provider/model" ID resolvable by the driver
// registry. The list runs from cheapest to most capable; on retry the next
// model is used.
var DefaultEscalationModels = []string{
	"openai/gpt-4o-mini",
	"openai/gpt-4o",
	"openai/o3-mini",
	"openai/o3",
}

// Options configures the run loop.
type Options struct {
	// Task is the plain-language task description (required).
	Task string

	// ImplementerModel is the initial implementer model ID (provider/model).
	// If empty, the first entry in EscalationModels is used.
	ImplementerModel string

	// VerifierModel is the verifier model ID (provider/model). Required — the
	// run loop fails closed without a verifier.
	VerifierModel string

	// Base is the branch to merge into on PASS. Default "main".
	Base string

	// RetryCap is the maximum number of retries before escalating to the human.
	// 0 means no retries (single attempt). Use -1 to accept the default (3).
	RetryCap int

	// EscalationModels is the ordered list of model IDs to try on retry.
	// If empty, DefaultEscalationModels is used. The implementer model starts
	// at ImplementerModel (or the first entry) and advances one position on
	// each retry. The verifier model stays fixed.
	EscalationModels []string

	// WorkspaceRoot is the repo root directory. Default ".".
	WorkspaceRoot string

	// Registry is the driver-resolution authority for every role leg (S06
	// rewire, replacing the deleted NewAgent/NewVerifier factories). When
	// nil, RunSlice defaults it to
	// registry.Default(model.ProviderConfigFromEnv()) (production path).
	// Tests inject fake drivers through a test registry.
	Registry *registry.Registry

	// DBPath is the path to the SQLite database. If empty, the default
	// (.sworn/sworn.db under WorkspaceRoot) is used.
	DBPath string

	// DB is an already-opened database handle. When set, DBPath is ignored.
	// When nil, the run loop opens (or creates) the database at DBPath.
	DB *sql.DB

	// Supervisor is the process supervisor for track ownership. When nil,
	// the run loop creates one from the database. When set, DB must also
	// be set (or the supervisor must use its own connection).
	Supervisor *supervisor.Supervisor

	// Notifier is the notification dispatcher for FAIL/BLOCKED verdicts.
	// When nil, notifications are skipped (test path / zero-config).
	Notifier *account.Notifier

	// ImplementTimeout is the per-attempt deadline for the implement step.
	// 0 means use DefaultImplementTimeout. A negative value means no timeout.
	ImplementTimeout time.Duration
}

// Run executes the sworn run turnkey loop. It returns nil only when the
// implementation passed verification and was merged.
func Run(ctx context.Context, opts Options) error {
	if opts.Task == "" {
		return fmt.Errorf("run: --task is required")
	}
	if opts.Base == "" {
		opts.Base = "main"
	}
	if opts.WorkspaceRoot == "" {
		opts.WorkspaceRoot = "."
	}

	workspaceRoot, err := filepath.Abs(opts.WorkspaceRoot)
	if err != nil {
		return fmt.Errorf("run: resolve workspace: %w", err)
	}

	// ── Non-blocking credit fetch (S06b) ──────────────────────────────
	// FetchCredits runs in a goroutine with a 3s timeout. It updates the
	// cache file if successful; the run proceeds regardless of outcome.
	go func() {
		credsDir := filepath.Dir(account.CredentialsPath())
		creds, credErr := account.Load(credsDir)
		if credErr != nil || creds == nil || !account.IsLoggedIn(creds) {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if _, err := account.FetchCredits(ctx, creds); err != nil {
			fmt.Fprintf(os.Stderr, "sworn run: credit fetch warning: %v\n", err)
		}
	}()

	repo := git.New(workspaceRoot)
	// ── Open database and initialise supervisor ───────────────────────
	var database *sql.DB
	if opts.DB != nil {
		database = opts.DB
	} else {
		dbPath := opts.DBPath
		if dbPath == "" {
			dbPath = db.DefaultPath(workspaceRoot)
		}
		database, err = db.Open(dbPath)
		if err != nil {
			return fmt.Errorf("run: open database: %w", err)
		}
		defer database.Close()
	}

	// ── Create auto-generated release + slice ─────────────────────────
	releaseDir, sliceDir, err := setupSlice(workspaceRoot, opts.Task)
	if err != nil {
		return fmt.Errorf("run: setup slice: %w", err)
	}
	absSliceDir := filepath.Join(workspaceRoot, sliceDir)
	// spec.json-preferred, spec.md legacy fallback: setupSlice now writes an
	// authoritative spec.json, so resolve the truthful spec path.
	specPath := spec.SpecFilePath(absSliceDir)
	statusPath := filepath.Join(absSliceDir, "status.json")

	// Extract the release name from the generated release dir.
	releaseName := filepath.Base(releaseDir)

	// ── Supervisor acquire/release ────────────────────────────────────
	var sup *supervisor.Supervisor
	if opts.Supervisor != nil {
		sup = opts.Supervisor
	} else {
		sup = supervisor.New(database, releaseName)
	}

	// Reap any stale rows from previous crashed sessions.
	if reaped, reapErr := sup.Reap(); reapErr != nil {
		fmt.Fprintf(os.Stderr, "sworn run: reap warning: %v\n", reapErr)
	} else if reaped > 0 {
		fmt.Fprintf(os.Stderr, "sworn run: reaped %d stale track(s)\n", reaped)
	}

	// Acquire ownership for this track. The task-based single-slice mode
	// uses a synthetic single-track ID "S01-task".
	if err := sup.Acquire("S01-task"); err != nil {
		return fmt.Errorf("run: acquire track: %w", err)
	}
	defer sup.MustRelease("S01-task", supervisor.StateDone)
	// ── Branch off base ──────────────────────────────────────────────
	featureBranch := sanitiseBranch(opts.Task)

	if err := repo.Checkout(opts.Base); err != nil {
		return fmt.Errorf("run: checkout base %q: %w", opts.Base, err)
	}
	if err := repo.Branch(featureBranch); err != nil {
		return fmt.Errorf("run: create branch %q: %w", featureBranch, err)
	}

	// Stage and commit the auto-generated slice files.
	if err := repo.Stage(releaseDir); err != nil {
		return fmt.Errorf("run: stage slice: %w", err)
	}
	if err := repo.Commit("chore(run): auto-generated slice " + releaseDir); err != nil {
		return fmt.Errorf("run: commit slice: %w", err)
	}

	// Update start_commit so the verifier has an exact diff base.
	startCommit, err := repo.RevParse("HEAD")
	if err != nil {
		return fmt.Errorf("run: rev-parse HEAD: %w", err)
	}
	st, err := state.Read(statusPath)
	if err != nil {
		return fmt.Errorf("run: read status: %w", err)
	}
	st.StartCommit = startCommit
	if err := state.Write(statusPath, st); err != nil {
		return fmt.Errorf("run: write start_commit: %w", err)
	}

	// ── Run the implement→verify retry loop ────────────────────────
	err = RunSlice(ctx, workspaceRoot, specPath, statusPath, RunSliceOptions{
		ImplementerModel: opts.ImplementerModel,
		VerifierModel:    opts.VerifierModel,
		EscalationModels: opts.EscalationModels,
		RetryCap:         opts.RetryCap,
		ImplementTimeout: opts.ImplementTimeout,
		Registry:         opts.Registry,
		Notifier:         opts.Notifier,
		DB:               database,
	})
	if err != nil {
		// Re-wrap Blocked errors to preserve the run: prefix for
		// existing tests that check "verification blocked".
		if IsBlocked(err) {
			return fmt.Errorf("run: %s", err)
		}
		return fmt.Errorf("run: %w", err)
	}

	// ── Gated merge on PASS only ──────────────────────────────────
	if err := repo.Stage("."); err != nil {
		return fmt.Errorf("run: stage for merge: %w", err)
	}
	if err := repo.Commit("chore(run): verified — merge to " + opts.Base); err != nil {
		return fmt.Errorf("run: commit verified state: %w", err)
	}
	if err := repo.Checkout(opts.Base); err != nil {
		return fmt.Errorf("run: checkout base for merge: %w", err)
	}
	if err := repo.Merge(featureBranch); err != nil {
		return fmt.Errorf("run: merge into %s: %w", opts.Base, err)
	}
	fmt.Fprintf(os.Stderr, "sworn run: merged %s into %s (PASS)\n", featureBranch, opts.Base)
	return nil
}

// resolveSpecPath returns the truthful machine-contract path for a slice
// directory: spec.json when present, else the spec.md legacy path (ADR-0009).
func resolveSpecPath(sliceDir string) string {
	return spec.SpecFilePath(sliceDir)
}

// loadSpecText resolves a slice's machine contract to a readable spec body,
// spec.json preferred (rendered to markdown) with spec.md legacy fallback
// (ADR-0009). It gives the design/captain/verify legs of RunSlice one uniform
// text surface, so a spec.json-only slice (no spec.md) drives the whole loop
// without a spec.md-missing hard failure
// (S01-spec-json-read-conformance AC-02/AC-05). Fails closed like spec.LoadSpec.
func loadSpecText(sliceDir string) (string, error) {
	rec, md, err := spec.LoadSpec(sliceDir)
	if err != nil {
		return "", err
	}
	if rec != nil {
		return spec.RenderMarkdown(rec), nil
	}
	return md, nil
}

// setupSlice creates a release directory and a single-slice directory with
// auto-generated spec.md and status.json (Pin 3). Returns the release dir and
// slice dir (both relative to workspaceRoot).
func setupSlice(workspaceRoot, task string) (releaseDir, sliceDir string, err error) {
	ts := time.Now().UTC().Format("20060102-150405")
	releaseName := "run-" + ts
	releaseDir = filepath.Join("docs", "release", releaseName)
	sliceDir = filepath.Join(releaseDir, "S01-task")

	absSlice := filepath.Join(workspaceRoot, sliceDir)

	if err := os.MkdirAll(absSlice, 0o755); err != nil {
		return "", "", err
	}

	specContent := fmt.Sprintf(`# Task

%s

## User outcome

%s

## Acceptance checks

- [ ] The implementation satisfies the task description

## Required tests

- **Unit/Integration**: go test ./...

## Out of scope

- N/A
`, task, task)
	if err := os.WriteFile(filepath.Join(absSlice, "spec.md"), []byte(specContent), 0o644); err != nil {
		return "", "", err
	}

	st := &state.Status{
		Schema:        "https://example.com/schemas/baton/slice-status-v1.json",
		SliceID:       "S01-task",
		Release:       releaseName,
		Track:         "",
		CoversNeeds:   []string{"N/A-task-mode"},
		State:         state.InProgress,
		Owner:         "sworn-run",
		LastUpdatedBy: "run-loop",
		LastUpdatedAt: time.Now().UTC().Format(time.RFC3339),
		SpecPath:      filepath.Join(sliceDir, "spec.json"),
		ProofPath:     filepath.Join(sliceDir, "proof.md"),
		JournalPath:   filepath.Join(sliceDir, "journal.md"),
		PlannedFiles:  []string{},
		TestCommands:  []string{"go test ./..."},
		Verification:  state.Verification{},
		ReleaseBase:   "main",
	}
	statusPath := filepath.Join(absSlice, "status.json")
	if err := state.Write(statusPath, st); err != nil {
		return "", "", err
	}

	// CHOICE-B: keep the engine on one machine-contract path — synthesise an
	// authoritative spec.json from the human spec.md so every read site
	// (implement/verify/gates) reads spec.json, not the legacy spec.md
	// (S01-spec-json-read-conformance). spec.md is retained as the human artefact.
	if err := implement.WriteSpecRecord(filepath.Join(absSlice, "spec.md"), statusPath, absSlice); err != nil {
		return "", "", fmt.Errorf("setupSlice: write spec.json: %w", err)
	}

	return releaseDir, sliceDir, nil
}

// sanitiseBranch converts a task string into a safe branch name.
func sanitiseBranch(task string) string {
	var b strings.Builder
	b.WriteString("sworn/")
	for _, r := range task {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + 32)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '_' || r == '-':
			b.WriteRune('-')
		}
	}
	name := b.String()
	if len(name) > 50 {
		name = name[:50]
	}
	name = strings.Trim(name, "-")
	if name == "sworn" || name == "sworn/" {
		name = "sworn/task"
	}
	return name
}

// NOTE (S06): newAgentFromModel / newVerifierFromModel — the factory
// defaults behind the deleted Options.NewAgent/NewVerifier fields — were
// removed with the registry rewire. Their CapChat capability gate is
// subsumed by the registry's role check ("capability IS the role set",
// S05): an incapable driver is rejected by name at Resolve time, before any
// dispatch.
