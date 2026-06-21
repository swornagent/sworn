# Captain review — S04a-tui-foundation
Date: 2026-06-21
Design commit: 2618edf249507792496d3e14e45ff18ca9bf3796

## Pins

**1. [mechanical] §3 — go.mod/go.sum absent from touchpoint plan**

What I observed: Design §3 lists 7 planned files (`cmd/sworn/main.go`, `cmd/sworn/top.go`,
`internal/tui/{model,releases,board,styles}.go`, `internal/tui/tui_test.go`). Adding bubbletea
and lipgloss (confirmed absent from current `go.mod`) will produce commits that touch `go.mod`
and `go.sum`. Neither appears in §3 or in `planned_files` in `status.json`.

S02b verification history: files appearing in `git diff --name-only <start_commit>` but absent
from `planned_files` or `proof.md` → Gate 2 FAIL (Round 5: "parallel_test.go and sworn binary
in committed diff but absent from proof.md").

What to ask the implementer: Add `go.mod` and `go.sum` to §3 (under "Entry point wiring") and
to `planned_files` in `status.json` before writing any code.

---

**2. [memory-cited] §2 — bubbletea + lipgloss need ADR documentation before go.mod changes**

What I observed: Design adds two new runtime dependencies (bubbletea and lipgloss, both absent
from current `go.mod`). The dep policy (revised 2026-06-20) requires "an ADR commit before the
dep appears in go.mod." Design §2 and §3 make no mention of an ADR for these deps. ADR-0004
(added by S10-provider-foundation) covers provider SDKs; it may not cover TUI deps.

What to ask the implementer: Confirm whether bubbletea + lipgloss are covered by ADR-0004 (if
that ADR is general-purpose) or write a brief `docs/adr/000X-tui-deps.md` (one-paragraph "why
bubbletea" + "why lipgloss") as the first commit of implementation — before any `go get`.

Citation: [[project-dep-policy]]

---

**3. [mechanical] §3 — internal/tui/tui.go not declared; T6 handoff anchor undetermined**

What I observed: Release touchpoint matrix (index.md) marks `internal/tui/tui.go` as a T2
deliverable with "(T2 dep)" for T6's `internal/tui/settings.go`. Design §3 lists model.go,
releases.go, board.go, styles.go, tui_test.go — but not tui.go. `tui.Run()` is called from
`cmd/sworn/main.go` and must live somewhere. If it goes in `model.go`, the matrix annotation
pointing T6 at `tui.go` is wrong; if it gets its own file, the design omits it from §3.

What to ask the implementer: Decide and declare: (a) add `internal/tui/tui.go` to §3 and
`planned_files` as the `tui.Run()` home, or (b) confirm `tui.Run()` lives in `model.go`, record
this as Divergence from plan, and correct the touchpoint matrix annotation. Either is valid — the
choice must be committed so T6's implementer has a reliable anchor.

---

**4. [mechanical] §2b — design_decisions field absent from status.json**

What I observed: `status.json` has no `design_decisions` field. `sworn designfit <release>` reads
this field to gate Type-1 choices before code is written (Step 2b). Design §2 has 5 decisions.
Without the field, designfit either passes trivially (no decisions to check) or errors.

All 5 §2 decisions appear Type-2 (reversible, small scope):
- D1: no-args routes to tui.Run() instead of usage()+exit(64)
- D2: sworn top (no release arg) delegates to tui.Run()
- D3: releases list reads filesystem via filepath.Glob
- D4: root model uses standard tea.Model pattern
- D5: unit tests are pure model-state, no TTY

What to ask the implementer: Transcribe all 5 decisions into a `design_decisions` array in
`status.json` classified as Type-2, then run `sworn designfit 2026-06-19-safe-parallelism` to
confirm the gate passes before writing production code.

---

**5. [mechanical] §6.Q1 — answer already in spec; no Coach decision needed**

What I observed: §6 Q1 asks whether `sworn top <release>` should open the TUI pre-navigated or
preserve existing evidence-surface behaviour. The implementer proposes keeping existing behaviour
for the release-arg case.

Spec §In-scope states: "if R2's S15 created this file, it is updated to delegate to `tui.Run()`
so `sworn top` and `sworn` (no args) behave identically." This applies to the no-arg case. The
same section lists `sworn top <release>` as continuing its existing role. The implementer's
resolution is correct and matches the spec.

What to ask the implementer: No action needed. Q1 is answered by spec. Acknowledge and proceed.

---

## Summary

Pins: 5 total — 4 [mechanical], 1 [memory-cited], 0 [escalate]
Critical pins: 1 (go.mod/go.sum missing → Verifier Gate 2 FAIL if unaddressed), 4 (designfit gate cannot run without design_decisions field)

## Smaller flags (not pins, worth one-line ack)

(a) **§6 Q2 (repo root detection) is self-decided.** Implementer proposes `git rev-parse
--show-toplevel` + `os.Getwd()` fallback — reasonable, no Coach input needed. Note in Divergence
from plan if spec's "relative to repo root" phrasing causes ambiguity at verify time.

(b) **S04b/S04c will both extend model.go and tui_test.go** (sequential within T2 worktree —
no conflict risk). Implementer should design model.go with clear extension points so S04b's
`concurrent.go` can hook in cleanly.

(c) **go.mod/go.sum touchpoint matrix gap.** Index.md marks go.mod under T1 and T5 only; T2
is not listed. This is a matrix documentation gap (Pin 1 addresses the planned_files fix; the
matrix itself does not need amendment since T5 picks up T2's go.mod changes via release-wt at
merge time).

## Suggested ack reply
<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

Clean design, 5 pins all apply-inline:

1. **go.mod/go.sum missing from §3.** Add both to §3 and `planned_files` in `status.json` before writing any code. bubbletea and lipgloss will appear in `git diff --name-only` and must be explained.
2. **ADR required for bubbletea + lipgloss.** Per dep policy, each new dep needs an ADR commit before `go.mod` changes. Write a brief `docs/adr/000X-tui-deps.md` (or confirm ADR-0004 covers TUI deps if general-purpose) as the first commit of implementation — before any `go get`.
3. **tui.Run() location must be declared.** Either add `internal/tui/tui.go` to §3 + `planned_files` (preferred — matches touchpoint matrix), or put Run() in `model.go` and note Divergence from plan + correct the matrix annotation.
4. **design_decisions in status.json.** Transcribe all 5 §2 decisions (all Type-2) into the `design_decisions` field and run `sworn designfit 2026-06-19-safe-parallelism` before writing production code.
5. **§6 Q1 is answered by spec.** `sworn top` (no args) → TUI; `sworn top <release>` → existing evidence surface. Your proposed resolution is correct. No Coach decision needed.

Flags: (a) §6 Q2 self-decided — no action; (b) model.go should expose clean extension points for S04b/S04c.

§2 decisions D1–D5 all Type-2 — ack. §6 Q1 spec-answered, Q2 self-decided — ack.

Address pins 1–4 inline at implementation start (pre-code steps), then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: All 5 pins are apply-inline mechanical/memory corrections — no design redesign or Coach judgment required; Verifier (Rule 7) backstops.
-->
