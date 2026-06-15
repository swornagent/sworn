# Design TL;DR — S04-embed-baton-prompts

## §1. User-visible change

The `sworn verify` command now uses the real Baton verifier prompt (embedded at
build time) instead of the 4-line placeholder. Running `sworn version` also
prints the vendored Baton protocol version, so any user can see exactly which
protocol revision the binary enforces — no external prompt files or runtime FS
reads needed. The planner, implementer, and captain prompts are also embedded
(available for S06/S07 consumers), but the only user-visible change in this
slice is the verifier prompt going live and the version line appearing.

## §2. Design decisions not in spec (max 5)

1. **Vendor all four role prompts (not just verifier).** The spec says "vendor the
   Baton role-prompt content"; S06 (implementer) and S07 (run loop) need the
   planner/implementer/captain prompts. Vendoring all four now (one `go:embed`
   directory) avoids a second embedding pass later. The verifier prompt is the only
   one wired in this slice; the others are inert embedded assets until consumed.
   *Rationale: one embed operation, one vendoring commit; spec says "role-prompt
   content" (plural).*

2. **Baton protocol version via `VERSION.txt` in the embedded directory.** The
   source prompts live at `~/.claude/baton/role-prompts/` (not a git repo). I'm
   recording the initial vendored version as `v1.0.0` with a note that future
   prompt updates must bump this. The `go:embed` directive picks it up alongside
   the `.md` files. *Rationale: co-located with the prompts it versions; build-time
   accessible via `prompt.Version()`.*

3. **Registry as a single `prompt` package with four accessor functions.**
   `prompt.Verifier()`, `prompt.Planner()`, `prompt.Implementer()`,
   `prompt.Captain()` each return `string`. The `go:embed` directive is at package
   level, embedding `*.md` and `VERSION.txt` into an `embed.FS`. Accessors read
   from the FS at init time (not per-call). *Rationale: simple, testable, no
   interfaces or registries — just four functions.*

4. **Replace `const systemPrompt` with a package-level `var` initialised from
   `prompt.Verifier()`.** The `systemPrompt` currently lives as a `const` in
   `verify.go`. Changing it to a `var` initialised via `prompt.Verifier()` is the
   minimal diff. The `Run` function signature does not change. *Rationale: smallest
   change surface; existing tests pass unchanged because they use `fakeVerifier`
   which ignores the system prompt entirely.*

5. **Extend `sworn version` to print both binary version + baton-protocol version.**
   Currently: `sworn 0.0.0-dev`. After: `sworn 0.0.0-dev` (line 1) and `baton-
   protocol v1.0.0` (line 2). No new flag; the version subcommand gains one extra
   line. *Rationale: spec says "surfaced (sworn version or a build var)" — doing
   both (build var + version output) is the strongest evidence.*

## §3. Files I'll touch grouped by purpose

- **`internal/prompt/` (new package)** — embedded `.md` files (`planner.md`,
  `implementer.md`, `verifier.md`, `captain.md`) vendored from
  `~/.claude/baton/role-prompts/` + `VERSION.txt` + `prompt.go` with `go:embed`,
  accessor functions, and a unit test. *Why: the spec's core deliverable — baton
  prompts as compile-time assets.*

- **`internal/verify/verify.go`** — replace `const systemPrompt` with a `var`
  initialised from `prompt.Verifier()`. *Why: AC1 — verifier uses the embedded
  prompt, not the placeholder.*

- **`cmd/sworn/main.go`** — extend the `version` subcommand to also print
  `baton-protocol v1.0.0`. *Why: AC3 — vendored version surfaced.*

## §4. Things I'm NOT doing

- **Not authoring or editing prompt content.** The `.md` files are copied verbatim
  from `~/.claude/baton/role-prompts/`. Any prompt changes happen upstream in the
  Baton protocol and are re-vendored in a future slice/release. This is explicitly
  out of scope per the spec.
- **Not wiring planner/implementer/captain prompts to consumers.** The embedded
  assets exist and are accessible via `prompt.Planner()` etc., but nothing calls
  them yet. That wiring belongs to S06/S07.
- **Not adding runtime file-read fallback.** If `go:embed` fails at build time, the
  build fails — there is no fallback path to read prompts from disk at runtime.

## §5. Reachability plan

- **Unit test**: `internal/prompt/prompt_test.go` — asserts `prompt.Verifier()` is
  non-empty and contains the PASS/FAIL/BLOCKED verdict-contract instruction
  (`go test ./internal/prompt/`).
- **Binary smoke test**: `go build -o /tmp/sworn ./cmd/sworn && /tmp/sworn version`
  — asserts output contains both `sworn` and `baton-protocol`.
- **Integration**: existing `internal/verify/verify_test.go` tests pass unchanged
  after the `systemPrompt` replacement (they use `fakeVerifier` and don't inspect
  the prompt string).

## §6. Open questions for the Coach

- None at this stage.