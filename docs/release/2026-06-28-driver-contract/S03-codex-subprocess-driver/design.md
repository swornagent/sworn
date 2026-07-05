# Design TL;DR — S03-codex-subprocess-driver

## User outcome (from spec.json)

An operator with the Codex CLI can dispatch loop roles through a codex exec
subprocess driver exactly as they can with claude-cli — the N=2 proof that
the Driver contract (S01) is not claude-shaped — closing the open sworn#19
deferral (S63-deferral-1).

## Approach

Add `internal/driver/codex.go` (`CodexDriver`) implementing `driver.Driver`
over S02's shared `subprocess.go` plumbing, in the same track worktree/branch
as S02 (sequential, same track). One dispatch = one `codex exec` subprocess
call, same as claude — codex's own agentic loop runs inside the CLI process;
this driver only spawns it, parses its output, and normalizes into `Result`.

The substance of this slice, per spec rationale, is the **envelope
normalisation delta**: codex's non-interactive JSON output does not share
claude's single-JSON-envelope shape, and this needs a documented assumption
(R-01) rather than verified ground truth, since no live codex binary is
exercised — only a fake-binary fixture, same convention as `claude_test.go`.

## Key design choices + rationale

1. **Invocation: `codex exec --json -C <WorktreeRoot> <prompt>`, with
   `cmd.Dir` also set to `WorktreeRoot`.** AC-01 literally calls out both
   `cmd.Dir` *and* the `-C` flag — belt-and-braces so a codex CLI version
   that doesn't honour `-C` (or a future one that changes its meaning still
   gets a correctly-rooted child via `cmd.Dir`). `--json` is the assumed
   flag for machine-readable output (the AC's "machine-readable output
   enabled"). No `--no-session-persistence`-equivalent flag is added for the
   verifier role — neither AC-01 nor AC-02 names one for codex, unlike
   claude's AC-03, so I'm not inventing one.

2. **Codex envelope: JSONL event stream, not a single JSON object — this is
   the R-01 assumption, documented as a version-pinned comment in
   `codex.go`.** I'm assuming `codex exec --json` emits one JSON object per
   stdout line: `{"type":"item.completed","item":{"type":"agent_message","text":"..."}}`
   for the agent's final message (last one wins if the CLI streams
   intermediate messages), and a terminal `{"type":"turn.completed","usage":
   {"input_tokens":N,"output_tokens":N},"model":"...","duration_ms":N}`
   carrying usage/model/duration. This is a genuine unknown (the R-01 risk
   this slice exists to absorb) — the fake-binary fixtures encode exactly
   this shape, `codex.go`'s doc comment states the assumption explicitly
   (not implied), and S10's conformance suite runs the same behavioural
   clauses against this fake, not a real binary. A real-binary drift is a
   Rule-2 deferral for S10/SIT to surface, not something this slice can
   close on its own.

3. **Parsing is defensive per-line, same posture as claude's envelope
   parsing**: an unparseable line is a hard `ErrKind=protocol` failure (the
   outer-envelope-protocol-error principle from S02 decision 6, extended to
   "per line" instead of "the one envelope"); a well-formed stream with no
   `item.completed`/`agent_message` event leaves `ResultText` empty rather
   than erroring (nothing to fabricate); missing `usage` degrades to
   `InputTokens=0, OutputTokens=0, CostSource="unknown"`, exactly matching
   claude's convention (AC-04). `ModelID` falls back to the requested model
   when the stream omits it; `DurationMS` falls back to the measured
   wall-clock time when the stream omits it — same fallback rules as
   `claudeEnvelope.modelID`/`durationMS`.

4. **Verifier role**: identical pattern to claude — `VerdictSchema` is
   appended to the prompt as the required output contract (AC-02), and the
   final agent message is required to parse as a JSON object or the
   dispatch fails closed with `ErrKind=protocol` (same `isJSONObject` helper,
   reused unchanged from `claude.go` — it's a pure string→bool function with
   no claude-specific behaviour, so codex.go calls it directly rather than
   duplicating it).

5. **`subprocess.go` is generalised, not duplicated, to let codex's
   non-zero-exit classification differ from claude's — without touching
   `claude.go` (not a touchpoint of this slice).** Today `classifySpawnError`
   hardcodes non-zero-exit → `ErrKindAuth`. I'm splitting `spawn()` into a
   thin wrapper and a new `spawnClassified(ctx, binary, args, dir, timeout,
   nonZeroExitKind string) spawnResult`, with `classifySpawnError` taking
   the same extra parameter. `spawn()` keeps its existing signature and
   calls `spawnClassified(..., ErrKindAuth)` — so claude.go's call site and
   every existing claude/subprocess test is byte-for-byte unaffected.
   `codex.go` calls `spawnClassified` directly with its own chosen Kind (see
   decision 6). Timeout→`transient` and missing-binary→`config` are
   unaffected by this split — those failure modes mean the same thing
   regardless of which CLI is being spawned, so only the non-zero-exit arm
   takes the parameter.

6. **OPEN QUESTION FOR THE CAPTAIN — codex's non-zero-exit `ErrKind` is
   internally inconsistent in spec.json AC-03 and needs a reviewer call
   before I lock it in.** AC-03's literal text: *"the same ErrKind mapping
   as the claude driver (timeout -> transient; binary-not-found -> config;
   non-zero exit -> provider with stderr excerpt)"*. But the claude driver's
   actual, ratified, verified mapping (S02 decision 2 / pin 2 resolution,
   live today in `subprocess.go`'s `classifySpawnError`) is non-zero-exit
   → **`ErrKindAuth`**, not `provider`. The parenthetical's stated value
   contradicts the clause introducing it ("the same... as the claude
   driver"). This isn't cosmetic: S02's pin 2 was escalated and ratified
   specifically to preserve `internal/run/slice.go:487`'s
   terminal-halt-on-auth fail-fast, which keys off `ErrKind==auth` after
   translation. If codex's own non-zero exit — which can just as easily mean
   an expired/invalid codex login as claude's can — is classified `provider`
   instead of `auth`, a codex-CLI auth failure silently will **not** trigger
   that fail-fast, reintroducing for codex exactly the regression S02's pin
   2 was raised to prevent for claude. My working theory: `provider` is a
   stale carry-over from S02's *pre-ratification* draft wording (before the
   pin-2 fork flipped claude's own mapping from `provider` to `auth`), not a
   deliberate codex-specific choice — `ErrKindProvider`'s own doc comment in
   `subprocess.go` ("reserved for a future driver's genuinely-distinct
   provider-side failure") is ambiguous about whether "future driver" means
   codex specifically or some other driver entirely.
   **My proposed default, pending Captain confirmation: codex's non-zero
   exit also maps to `ErrKindAuth`**, matching claude's ratified mapping and
   preserving fail-fast parity across both subprocess drivers, treating the
   AC's "same ErrKind mapping as the claude driver" clause as controlling
   over the parenthetical's stale value. The `spawnClassified` split in
   decision 5 makes either resolution a one-constant change — I'm not
   betting the architecture on my guess, just the specific Kind value. This
   is flagged as `design_decisions[0]` in `status.json` (Type-1, stake:
   whether a codex-CLI auth failure silently loses the engine's terminal
   fail-fast) with `human_decision` left for Captain design review — I have
   not resolved it myself.

7. **Fake codex CLI harness extends the existing shared `TestMain` in
   `subprocess_test.go` rather than adding a second one.** Go permits exactly
   one `TestMain` per package, and `driver_test.go`'s existing one already
   dispatches on `GO_TEST_FAKE_CLAUDE`. I'm adding a parallel
   `GO_TEST_FAKE_CODEX` env var with its own switch arm in the same
   `TestMain`, plus `fakeCodex*` functions alongside the existing
   `fakeClaude*` ones (same file, same re-exec convention) — this is why
   `subprocess_test.go` is a touchpoint of this slice rather than
   `codex_test.go` growing its own harness.

## Files touched

| File | Change |
|---|---|
| `internal/driver/codex.go` | NEW — `CodexDriver` (`Name`, `Roles`, `Dispatch`), argv construction, JSONL envelope parsing |
| `internal/driver/codex_test.go` | NEW — AC-01..AC-05 tests mirroring `claude_test.go`'s coverage shape: `TestCodexDispatchImplementer`, `TestCodexDispatchVerifier` (+ `_ProtocolError`), `TestCodexWorktreeGate`, `TestCodexErrorMapping` (table-driven), `TestCodexEnvHygiene`, `TestCodexEnvelopeDefaults`, `TestCodexDriver_Name_Roles` |
| `internal/driver/subprocess.go` | Split `spawn`/`classifySpawnError` to parameterize the non-zero-exit `ErrKind`; `spawn()` keeps its signature, delegating `ErrKindAuth` (claude.go unaffected) |
| `internal/driver/subprocess_test.go` | Add `GO_TEST_FAKE_CODEX` arm to the shared `TestMain` + `fakeCodex*` fixture functions; existing `TestSpawn_*`/`fakeClaude*` unchanged |

`claude.go` / `claude_test.go` are **not** touched — out of this slice's
touchpoints per spec.json, and decision 5's split is designed specifically
to keep them that way.

## AC traceability

| AC | Covered by |
|---|---|
| AC-01 | `codex.go` Dispatch (implementer path, argv + envelope parse) + `TestCodexDispatchImplementer` |
| AC-02 | `codex.go` Dispatch (verifier path: schema-in-prompt, `StructuredJSON` / `ErrKind=protocol`) + `TestCodexDispatchVerifier` (+ `_ProtocolError`) |
| AC-03 | `subprocess.go` `spawnClassified` + `codex.go`'s chosen non-zero-exit Kind (see decision 6) + `TestCodexErrorMapping` (table-driven: timeout/missing-binary/non-zero-exit) |
| AC-04 | `subprocess.go` shared `hygieneEnv` (unchanged) + `TestCodexEnvHygiene`; defensive parsing defaults + `TestCodexEnvelopeDefaults` |
| AC-05 | `go test ./internal/driver/...` — full package run, including S02's untouched `claude_test.go` |

## Risks / open items for the Captain

- **Decision 6 (escalate-class): codex non-zero-exit `ErrKind` value.** The
  spec's own AC-03 text is internally inconsistent (see decision 6 above in
  full). Recorded as `design_decisions[0]` in `status.json`, Type-1,
  `human_decision` pending. I need a reviewer call before I lock in
  `ErrKindAuth` vs `ErrKindProvider` for codex's non-zero exit — implementing
  either is a one-line change given the `spawnClassified` split, so this
  does not block starting the rest of the slice's build, but it does block
  writing `TestCodexErrorMapping`'s non-zero-exit case and I will not guess
  past design review on a fail-fast-affecting choice this consequential.
- **R-01 (spec's own risk, restated here for the reviewer):** the JSONL
  envelope shape in decision 2 is a documented assumption, not verified
  against a live codex binary. If it's wrong, S10's conformance suite (which
  exercises the same fake, not live) will not catch the drift — only a real
  SIT/cutover run against an actual `codex exec` binary would. That's
  explicitly out of this slice's scope (and out of S10's, per its own spec)
  — flagging so it isn't lost before whichever slice first runs against a
  real codex binary.
