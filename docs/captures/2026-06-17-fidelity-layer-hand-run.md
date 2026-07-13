# Hand-run: fidelity-layer release, slice S01-rtm-spine (2026-06-17)

> **Rename notice (2026-06-18, S16-lint-rename):** `sworn rtm` was renamed to
> `sworn lint trace` and `sworn ears` to `sworn lint ac` in commit `6518f3b`.
> References to the original names below are historically accurate for the date
> of this capture and are preserved as-is.

Exploration of the live `sworn` implementation against the planned
`2026-06-16-fidelity-layer` release. Goal: exercise a single slice end-to-end
and record what currently works, what is missing, and what blocks
verification.

## Context

- Workspace used: `/home/user/projects/sworn-worktrees/release-2026-06-16-fidelity-layer`
  (branch `release-wt/2026-06-16-fidelity-layer`, materialised from
  `release/v0.1.0`).
- Binary version: `sworn 0.0.0-dev`, `baton-protocol v1.0.0`.
- Slice selected: **S01-rtm-spine** (keystone of T1-fidelity-core).
- S01 user outcome: `sworn rtm <release>` reports a 2-D requirements
  traceability matrix and fails closed on broken traces.

## What was run

### 1. Native `sworn rtm` command (the slice's own entry point)

```text
$ ./bin/sworn rtm 2026-06-16-fidelity-layer
unknown command "rtm"
exit=64
```

**Result:** the S01 command surface is not implemented. The binary only knows
`bench`, `init`, `run`, `verify`, `version`, `help`. Every fidelity-layer slice
verb (`rtm`, `ears`, `reqverify`, `reqvalidate`, `designfit`, `journeys`, etc.)
is still spec-only on this branch.

### 2. Stateless `sworn verify` against the S01 spec

Created a trivial synthetic diff:

```diff
--- a/docs/release/2026-06-16-fidelity-layer/S01-rtm-spine/spec.md
+++ b/docs/release/2026-06-16-fidelity-layer/S01-rtm-spine/spec.md
@@ -10,3 +10,5 @@
 some existing line
+// trivial no-op addition for verify smoke test
```

#### 2a. Unconfigured verifier (fail-closed baseline)

```text
$ ./bin/sworn verify --spec .../S01-rtm-spine/spec.md --diff /tmp/s01-rtm.diff
sworn verify: model: SWORN_OPENAI_API_KEY not set
exit=2
```

**Result:** correct fail-closed behaviour. No key → `BLOCKED`, exit 2.

#### 2b. Model-backed verifier

Keys remapped from `~/.config/coach/env` to `SWORN_<PROVIDER>_API_KEY` /
`SWORN_<PROVIDER>_BASE_URL` for `openai`, `groq`, and `deepseek`.

| provider | model | result | exit | note |
|---|---|---|---|---|
| openai | `gpt-4.1-mini` | `BLOCKED / verifier_dispatch` HTTP 429 | 2 | account quota exhausted; correct fail-closed |
| groq | `llama-3.3-70b-versatile` | `INCONCLUSIVE` / `FAIL` / `INCONCLUSIVE` across 3 runs | 3 or 1 | sees the diff is a no-op, verdict varies |
| deepseek | `deepseek-chat` | `PASS` on run 3 / `FAIL` on runs 1-2 | 0 or 1 | **erroneous PASS for a no-op diff** |

**Key finding:** the stateless verify gate is functional and returns parseable
verdicts, but model judgement on a trivial no-op diff is inconsistent. DeepSeek
returned `PASS` for a diff that touches none of the acceptance checks — this is
a false positive. Groq consistently found the diff insufficient (FAIL or
INCONCLUSIVE). This is consistent with the earlier capture
`private-notes/captures/2026-06-16-swornagent-verify-prompt-runtime-mismatch.md`
which showed the same path could emit `unparseable_verdict` before the
stateless-prompt fix; the current build has the fix (`prompt.VerifyStateless()`
+ tolerant parser), but model reliability on thin/no-op diffs remains weak.

### 3. Baton first-pass script (`release-verify.sh`)

```text
$ bash /home/user/.claude/bin/release-verify.sh S01-rtm-spine 2026-06-16-fidelity-layer
exit=1 (FIRST-PASS FAIL)
```

Failures:

1. `status.json` state is `planned` — slice not yet ready for verifier.
2. Dark-code marker hit in `internal/adopt/adopt.go` (pre-existing in the
   `release/v0.1.0` base, not this slice's fault, but the script scans the
   whole diff vs `main`).
3. `proof.md` still contains `<paste output here>` template placeholders.

**Result:** S01 is correctly not verifiable yet — it has not been implemented.

### 4. Release board status

```text
$ bash /home/user/.claude/bin/release-board-status.sh --verbose
fidelity-layer: 0 / 15 verified — BLOCKED (15 remaining)
T1-fidelity-core: in_progress, 0/6
T2/T3/T4: planned, blocked on T1
```

## Interpretation

- The **E2E turnkey commands** from `release/v0.1.0` (`sworn verify`, `sworn run`,
  `sworn bench`, `sworn init`) are present and build cleanly.
- The **fidelity-layer slice commands** (`rtm`, `ears`, etc.) are **not yet
  implemented**. The release is still at the planning/spec stage on this
  worktree.
- `sworn run` is the only turnkey path that could implement a slice, but it
  auto-generates a brand-new slice from a `--task` string; it does not accept an
  existing fidelity-layer spec as input. Running it would create a new
  `docs/release/run-YYYYMMDD-HHMMSS/S01-task` release rather than advancing
  S01-rtm-spine.
- The current stateless verify gate parses verdicts correctly, but model
  reliability on a no-op diff is poor (false PASS from DeepSeek). This is
  pre-code verification of a spec-only slice, so the result is expected to be
  negative; the concern is the *variance*, not the negative result.

## Blockers to running a single fidelity-layer slice end-to-end

1. **No command entry point exists** for S01 (`sworn rtm`). It must be built
   first.
2. **Slice status is `planned`**, not `in_progress` / `implemented`. The Baton
   first-pass script rejects it.
3. **`sworn run` cannot target an existing slice spec** — only a free-text
   task. A new orchestration path (or manual implement → test → verify) is
   needed to run a pre-planned slice.
4. **False-positive risk** in the model verifier on trivial diffs suggests the
   stateless judge prompt may still be too permissive; worth tightening before
   relying on it for fidelity-layer gates.

## Next-step options

A. **Implement S01-rtm-spine manually** in the T1-fidelity-core track
   worktree, then run its own `sworn rtm` command and the full proof bundle.
B. **Extend `sworn run`** so it can accept an existing slice spec path
   (`--spec` or `--slice`) instead of only auto-generating a task.
C. **Run the entire T1-fidelity-core track** serially: S01 → S02 → S04 → S05 →
   S07 → S11, using the existing implement/verify/merge flow, but anchored to
   the pre-written specs.

## Evidence artefacts

- Binary build: `/home/user/projects/sworn-worktrees/release-2026-06-16-fidelity-layer/bin/sworn`
- Synthetic diff: `/tmp/s01-rtm.diff`
- Verify outputs: `/tmp/groq-{1,2,3}.json`, `/tmp/deepseek-{1,2,3}.json`
- This capture: `docs/captures/2026-06-17-fidelity-layer-hand-run.md`

## Provenance

Session 2026-06-17. Ran against live repo state; keys sourced from
`~/.config/coach/env` and remapped to `SWORN_*` env vars for the test.
