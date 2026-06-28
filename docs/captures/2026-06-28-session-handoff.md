# Session handoff — 2026-06-28 (context compaction point)

Master index: `docs/captures/2026-06-28-synthesis-and-forward-plan.md`. All day's captures are
`docs/captures/2026-06-28-*.md` + the S27 slice + `sworn-internal/docs/strategy/2026-06-28-loop-delivery-benchmark-commercial.md`.

## Done this session (committed)
- **3-model dogfood** of `sworn run --parallel` (codex/deepseek/glm): DOA cold-start; 12+ bugs; harness-not-model is the cause. Fixes folded as slice **S27-parallel-dispatch-fix**.
- **Architecture recommendation** (driver-contract keystone; §3.5 verification gap; §3.6 greenfield Planner-Slice-0 harness; §3.7 pre-merge UAT; §3.8 AC-completeness scrutiny). **`sworn run`→`sworn loop`** rename landed.
- **Benchmarks**: SWE-bench under-measures sworn (hides ACs); SWE-AGI fits. SWE-AGI ini: claude 83% / deepseek 76% private; loop-lift retry **net −1** (AC-completeness ceiling).
- **Driver-contract release sliced**: `docs/release/2026-06-28-driver-contract/` (3 tracks/9 slices).
- **coach-loop fixes → canonical Baton, released v0.6.2** (pushed + GH release): merge-track honours `BATON_AUTO_CONFIRM` (1de3baf→re-authored); implement-slice **restores the Design TL;DR gate** (45aa433→re-authored). Both **re-installed to ~/.claude** via `baton/install-claude.sh`. Design-review gate now fires.
- **Canonical replan (target 1) DONE**: `docs/release/2026-06-27-conformance-foundation/index.md` on `release/v0.1.0` — `internal/model/oai.go`+drivers declared DOCUMENTED SHARED across T2/T3/T7; **T3 & T7 `depends_on: T2-model-layer`** (clean, no inline comment).

## REMAINING — pick up here
1. **Target 2 — deepseek build replan** (`~/sworn-eval-coach-deepseek`, the running eval clone; origin removed, isolated). It is PAUSED with **S12-first-pass-demote BLOCKED** (touchpoint conflict: T3 & T2 both touch `internal/model/oai.go`). Apply the same fix in its `index.md` (oai.go shared T2/T3/T7; T3/T7 depend_on T2 — NO inline comments), **clear S12's `verification.result` → pending** in the T3 worktree status.json, **forward-merge `release-wt` into the T3 worktree resolving oai.go (combine both sides)**, commit, then RESUME:
   ```bash
   cd ~/sworn-eval-coach-deepseek
   COACH_ENV_FILE="$PWD/.coach-env" coach loop          # add --bg to daemonize
   ```
   T2/T6 already merged; T7/S25 already resolved earlier. Build state visible via `coach top` / the oracle in that clone.
2. **Stale branch cleanup — PARTIALLY DONE.** Deleted `track/2026-06-27-conformance-foundation/*` (5 branches; the Go-eval pollution that made the main-repo oracle read stale state — now it reads the fixed `release/v0.1.0` plan). **~35 older `release-wt/*` + `track/*`** from historical eval releases (06-15/06-16/06-19 etc.) REMAIN — not mass-deleted under low context. Review `git branch --list 'release-wt/*' 'track/*'` and delete the eval leftovers (keep nothing that the real release history needs; the canonical plan lives on `release/v0.1.0`, not these branches). Lesson: future eval clones must remove `origin` (like the coach clones did) so they can't push branches back.

## Key gotchas (don't re-trip)
- **YAML inline comments break the frontmatter parser** — `depends_on: X  # note` parses the comment into the value. Keep frontmatter values bare.
- **`coach loop` re-sources `~/.config/coach/env`** — to force a model use `COACH_ENV_FILE=<override>`; durable overrides at `~/sworn-eval-coach-<M>/.coach-env`. The `l` hotkey in `coach top` also reverts to native env.
- **`/replan-release` (v0.6.2) needs `board.json`** — index.md-era releases (like 2026-06-27) STOP at Step 0; do the equivalent revision by hand (back-compat gap; relates to S14).
- Re-test the **committed** commit, not the live worktree (the running loop leaves files dirty mid-dispatch).

## Open design items captured (not yet implemented)
- Three-tier interpreter→Captain→human escalation + page TYPES (informational vs blocking) + session-held-open-on-page (checkpoint/resume). `bash-coachloop-learnings.md`.
- Driver-contract re-architecture for Go (release sliced, not built).
- Recurring touchpoint conflicts = planning under-declaration + missing invariant-2; design-review gate is the early catch.
