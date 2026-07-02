# Starter prompt — re-plan `2026-06-28-driver-contract` (fresh session)

Copy from the line below into a fresh session in `~/projects/sworn`.

---

Run **`/plan-release 2026-06-28-driver-contract`** — a fresh planning pass, NOT
`/replan-release`. Verified 2026-07-02: this release has **no `board.json`**
(it predates the cutover), zero branches/worktrees, and all 9 slices sit at
`planned` in `spec.md`-era records — there is no in-flight state to preserve,
so re-cut the plan in canonical form (board.json + spec-v1 + EARS ACs +
touchpoint matrix), treating the existing `docs/release/2026-06-28-driver-contract/`
artefacts (index.md frontmatter plan + 9 spec.md slices) as raw material.

## Why the re-cut (what moved since 2026-06-28)

- The **verifier-verdict-v1 keystone** landed inside the loop (schema-constrained
  verifier output; prose scraping deleted), changing the dispatch seam T2
  (S05-runslice-via-driver, S06-scheduler-driver-dispatch) was scoped against.
- **PR #78** (11 conformance fixes) and the **render-drift release** (board.json
  oracle for loop/MCP/TUI/CLI, drift guard) merged to `release/v0.1.0`.
- **Baton v0.7.0** shipped; the sworn re-vendor is tracked as sworn#48.

## Priorities to raise in the planning conversation

1. **Promote S02-subprocess-agent-driver to the front** (likely track 1 with its
   registry/resolution dependencies S01/S04). It is the fix for **sworn#35**
   (claude-cli/anthropic advertise Chat but ignore tools; `cliDriver.Chat` also
   sets no `cmd.Dir`) — the confirmed blocker for running `sworn loop` on a
   Claude subscription (implementer sonnet / verifier opus via claude-cli was
   attempted 2026-07-02 and stopped pre-spend on exactly this).
   Design question for the human: should the subprocess driver serve the
   **verifier** role too (CLI re-runs tests itself in the worktree)? That would
   also close **sworn#55** (engine verifier has no tool loop).
2. **Re-ground S08-differential-validation.** Its reference implementation
   (coach-loop) is retired and schema-incompatible with baton v0.7.0; an archive
   lives at `~/projects/fired/baton-backup` and can serve as reference
   architecture if desired. Decide: differential-validate against the archive
   (pinning old schemas — probably not worth it) vs. dropping S08 in favour of a
   beefed-up S09-driver-conformance-suite. Human call.
3. **Consume the open backlog as planning input** (don't rediscover): sworn#35
   (toolless Chat), #15 (self-registering factory — S04 territory), #31
   (provider prefix rename), #19 (codex driver, currently a Rule-2 deferral in
   cli.go), #55 (verifier tool loop), #70 (cost telemetry nominal — S07
   normalized-result territory; claude-cli reports cost 0 by design, decide how
   the normalized result records subscription-dispatch cost honestly).

## Constraints / traps

- Driver interface shape is **architecturally significant ⇒ Type-1** — options +
  rationale recorded, human decision required (Rule 9); the model must not
  self-ratify it.
- The loop's verifier requires `ChatStructured` (only oai/openai-responses
  implement it today) — any driver intended for the verifier role must either
  implement it or the seam must be redesigned deliberately.
- No paid model dispatch during planning; live probes with stripped env only
  (logged-in `~/.sworn` creds route through the proxy — sworn#69).
- Planning artefacts commit to the release branch flow per track-mode; validate
  board.json against board-v1 and render index.md from it (the drift guard is
  fail-closed now).

## Follow-on once this release lands

Run the queued autonomous-loop dogfood: `sworn loop --release
2026-07-01-loop-cli-ux` with implementer sonnet / verifier opus via the new
subprocess driver — the original goal this release now unblocks. (loop-cli-ux is
canonical-format, 3 chore slices, 1 serial track — verified ready 2026-07-02.)
