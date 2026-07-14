package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/swornagent/sworn/internal/config"
	"github.com/swornagent/sworn/internal/model"
	"github.com/swornagent/sworn/internal/project"
	"github.com/swornagent/sworn/internal/style"
)

// setupProjectContext drafts and ratifies the project's Baton project-context-v1
// record (.sworn/project.json).
//
// Elicited → ratified → durable, the same contract Rule 10 applies to journeys:
//
//  1. The model DRAFTS the record by reading the repo. It runs on the adopter's own
//     configured model and credentials — sworn never phones home. Drafting means
//     sending repository content to a model, and where that content goes is the
//     adopter's decision, so it is made explicitly and it is skippable.
//  2. A HUMAN ratifies. The model can read the auth code; it cannot know whether real
//     people depend on this system today, and that is the fact that decides whether a
//     medium security finding blocks. Ratification is never defaulted to yes.
//  3. The record is committed and every later session reads the same context.
//
// Skipping is safe, not silent: an unratified or absent record makes every check run
// at fail-closed HIGH stakes with an inferred description (project.Resolve).
func setupProjectContext(repoRoot string, in *bufio.Reader, assumeYes bool) error {
	if existing, err := project.Load(repoRoot); err == nil && existing.Ratification.Ratified {
		fmt.Printf("  %s  project context — already declared and ratified\n", style.Dim("ok"))
		return nil
	}

	fmt.Println()
	fmt.Println(style.Heading("Project context"))
	fmt.Println()
	fmt.Println("  The code reviews need to know what this project is, and what is at risk")
	fmt.Println("  if a defect ships. Without it, they fall back to guessing from your file")
	fmt.Println("  layout — which can read your languages, but can never know whether real")
	fmt.Println("  customers depend on this. That is what decides whether a medium-severity")
	fmt.Println("  security finding blocks a release or merely advises.")
	fmt.Println()
	fmt.Printf("  To draft it, sworn will send %s to %s:\n",
		style.Bold("your repo's layout and manifests"), style.Bold("your own configured model"))
	fmt.Println("    - the top two levels of directory names")
	fmt.Println("    - README.md, go.mod, package.json, tsconfig.json and similar")
	fmt.Println()
	fmt.Printf("  %s\n", style.Dim("No source files. No .env. Your key, your provider — sworn never phones home."))
	fmt.Println()

	if !assumeYes {
		fmt.Print(style.Bold("Draft the project context with your model? (y/n) [y]: "))
		resp, _ := in.ReadString('\n')
		if r := strings.TrimSpace(strings.ToLower(resp)); r == "n" || r == "no" {
			fmt.Printf("  %s  project context — checks will run at HIGH stakes with an inferred description\n",
				style.Dim("skipped"))
			fmt.Printf("  %s\n", style.Dim("      write .sworn/project.json by hand, or re-run 'sworn init', to declare it"))
			return nil
		}
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("no model configured (%w) — declare .sworn/project.json by hand", err)
	}
	verifier, modelID, err := verifierFromConfig(cfg)
	if err != nil {
		return fmt.Errorf("%w — declare .sworn/project.json by hand", err)
	}

	fmt.Printf("\n  Reading the repo with %s...\n", style.Bold(modelID))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	draft, err := project.Elicit(ctx, repoRoot, modelID, verifier)
	if err != nil {
		return err
	}

	// --- Review. This is the point of the whole exercise. ---
	fmt.Println()
	fmt.Println(style.Heading("  Draft — review it, it is a proposal, not a decision"))
	fmt.Println()
	fmt.Printf("  %s\n    %s\n\n", style.Bold("Project:"), draft.Record.Context)
	fmt.Printf("  %s\n", style.Bold("At risk if a defect ships:"))
	printStakes(draft.Record.Stakes)

	if len(draft.Uncertain) > 0 {
		fmt.Println()
		fmt.Printf("  %s the model says it GUESSED at: %s\n",
			style.Warn("Check these —"), strings.Join(draft.Uncertain, ", "))
	}

	fmt.Println()
	fmt.Println(style.Dim("  A model can read your auth code and your payment integration. It cannot know"))
	fmt.Println(style.Dim("  whether real people depend on this today. Only you can confirm that — and until"))
	fmt.Println(style.Dim("  you do, every check runs at HIGH stakes regardless of what the draft claims."))
	fmt.Println()

	// Ratification is NEVER defaulted to yes. An unratified record is the safe state;
	// a wrongly-ratified one silently lowers the security bar.
	fmt.Print(style.Bold("Is this correct? Ratify it? (y/n) [n]: "))
	resp, _ := in.ReadString('\n')
	ratify := strings.TrimSpace(strings.ToLower(resp))

	if ratify == "y" || ratify == "yes" {
		draft.Record.Ratification.Ratified = true
		draft.Record.Ratification.At = time.Now().UTC().Format(time.RFC3339)
		draft.Record.Ratification.By = currentUser()
	} else {
		fmt.Printf("\n  %s  saved UNRATIFIED — checks run at HIGH stakes until you ratify it\n", style.Dim("ok"))
		fmt.Printf("  %s\n", style.Dim("      edit .sworn/project.json, set ratification.ratified = true"))
	}

	if err := project.Save(repoRoot, draft.Record); err != nil {
		return err
	}

	fmt.Printf("\n  %s  %s\n", style.Success("wrote"), project.RecordPath)
	fmt.Printf("  %s\n", style.Dim("      commit it — every session, teammate and CI run reads the same context"))
	return nil
}

func printStakes(s *project.Stakes) {
	if s == nil {
		fmt.Println("    (none proposed)")
		return
	}
	any := false
	if s.Production {
		fmt.Println("    - deployed and live in production")
		any = true
	}
	if s.RealUsers {
		fmt.Println("    - real people outside the team depend on it")
		any = true
	}
	if len(s.SensitiveData) > 0 {
		fmt.Printf("    - holds sensitive data: %s\n", strings.Join(s.SensitiveData, ", "))
		any = true
	}
	if len(s.Regulated) > 0 {
		fmt.Printf("    - subject to: %s\n", strings.Join(s.Regulated, ", "))
	}
	if s.Notes != "" {
		fmt.Printf("    - %s\n", s.Notes)
	}
	if !any {
		fmt.Println("    - not in production, no real users, no sensitive data")
	}
}

func currentUser() string {
	for _, k := range []string{"GIT_AUTHOR_NAME", "USER", "USERNAME"} {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return "unknown"
}

// verifierFromConfig builds a model client from the adopter's configured verifier
// model. The elicitation runs on THEIR model and THEIR credentials — that is the
// whole reason it lives in `sworn init`, which already holds both.
//
// It resolves through config.ResolveVerifierModel — the same precedence every other
// model-using command uses ($SWORN_VERIFIER_MODEL > config.json). Reading
// cfg.Verifier.Model directly would have been a fourth, subtly different resolution
// path, which is precisely the drift that made `sworn llm-check` unrunnable on a
// fully-configured setup.
func verifierFromConfig(cfg config.Config) (model.Verifier, string, error) {
	id, err := config.ResolveVerifierModel("", cfg)
	if err != nil {
		return nil, "", err
	}
	v, err := model.FromEnv(id)
	if err != nil {
		return nil, "", fmt.Errorf("model %q: %w", id, err)
	}
	return v, id, nil
}
