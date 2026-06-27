# Design TL;DR — S06-implementer

## §1. User-visible change

A new `internal/implement` package drives the agentic tool loop (S03) with the
embedded Baton implementer prompt (S04) to implement a spec in the workspace.
After the model finishes, the package generates `proof.md` from **live repo
state** — `git diff`, actual test output, reachability artefacts — rather than
trusting the model's narration. It then updates `status.json` to `implemented`
and stops. It never self-certifies to `verified`.

## §2. Design decisions not in spec (max 5)

1. **Proof generation is post-hoc, not model-authored.** The model implements
   code, but the proof bundle is regenerated from live git state *after* the loop
   terminates. The model never writes proof.md directly. Rationale: spec AC2
   requires proof "files changed" = `git diff --name-only`, and a model can
   hallucinate file lists.

2. **Single `Run` function, no streaming interface.** The caller blocks until the
   agent loop completes or errors. The run-loop (S07) owns retry and
   orchestration — the implementer is a single "implement this spec" call.
   Rationale: matches the verify.Run pattern; S07 adds timeout/retry.

3. **Workspace root + slice path as arguments, not a Status struct.**
   `Run(ctx, workspaceRoot, specPath string, agent agent.Agent) error`. The
   package reads spec.md and status.json itself from the slice directory.
   Rationale: keeps the interface narrow; the caller doesn't need to pre-parse.

4. **User prompt is the spec verbatim plus workspace instructions.** The system
   prompt is `prompt.Implementer()`. The user prompt is a simple
   `spec.md` contents + "Implement this spec in workspace <root>. After
   implementation, stop." No additional templating layer. Rationale: the
   implementer role prompt (from Baton protocol) already contains the full
   discipline — we just need to feed it the spec.

5. **Proof structure follows Baton proof template.** The generated proof.md uses
   the same sections as the Baton release-mode proof template: Scope, Files
   changed, Test results, Reachability artefact, Delivered, Not delivered,
   Divergence. Rationale: the verifier (S01/S02) is trained on this format.

## §3. Files I'll touch grouped by purpose

- **`internal/implement/implement.go`** — single `Run` function: read spec, build
  prompts, drive agent loop, generate proof.md from git state, update status.json.
- **`internal/implement/implement_test.go`** — fake agent that implements a
  trivial spec in a temp git repo; assertions that proof.md matches git diff and
  status.json reaches `implemented`.

## §4. Things I'm NOT doing

- I am NOT writing the run-loop orchestration (S07) — that consumes this package.
- I am NOT touching the verifier (S01/S02) — this package never certifies.
- I am NOT generating a reachability artefact beyond the test output cited in
  proof.md; S07 provides the full run-loop reachability.
- I am NOT adding CLI subcommands; `sworn run` (S07) calls this package.

## §5. Reachability plan

The integration test in `implement_test.go` directly exercises `Run()` end-to-end:
a temp git repo, a spec file, a fake agent that makes a file edit + runs a bash
command, and then assertion that proof.md is generated with correct `git diff
--name-only` output. This is a Go test — run with `go test ./internal/implement/`.

## §6. Open questions for the Coach

- The spec says "implement against a spec" but the S06 spec itself is being
  implemented by this package — the test needs a separate trivial spec fixture.
  I'll inline a minimal spec as a test constant. Is that acceptable, or should
  there be a testdata/specs/ directory?