# Bash Coach-Loop Design Techniques: Bug Prevention & Go Port Learnings

**Prepared:** 2026-06-28  
**Scope:** Analysis of Bash coach-loop engine (166KB orchestrator + 5 runtime drivers) to extract design techniques preventing 8 specific Go port bugs, with feasibility assessment for running eval via coach-loop.

---

## Part A: Per-Bug — How Bash Avoids It (and Go Port Recommendations)

### Bug 1: Cold-start — `sworn run --parallel` cannot bootstrap a freshly-planned release

**Bash technique (coach-loop:1745-1807, `ensure_release_worktree`):**

- **Run at startup (once per release):** `parallel_coordinator()` calls `ensure_release_worktree "$release"` **before any worker spawns** (line 1949). This is the single point of idempotent bootstrap.
- **Creates in order:** (1) git worktree (`git worktree add release-$name`); (2) records paths in index.md frontmatter (`release_worktree_path:`, `release_worktree_branch:`); (3) commits & pushes to integration branch.
- **Orchestration dependency:** The release worktree MUST exist before ANY worker thread tries to compute routes or dispatch agents. Workers never race on materialization.
- **Key passage (line 1768-1770):**
  ```bash
  local wt_branch="release-wt/$release"
  local wt_path="${primary}-worktrees/release-$release"
  ```
  This naming is canonical — every worker and the coordinator use the same formula.

**Why bash avoids the bug:** The cold-start race is BLOCKED at the architectural level — a single goroutine (the coordinator) runs `ensure_release_worktree` ONCE, and only then spawns workers. No worker-1 vs worker-2 race on worktree creation.

**Go port recommendation (urgent):**
- Invoke the bootstrap equivalent **before `go run --parallel` spawns any workers**. In `internal/run/coordinator.go` (or equivalent), add:
  ```go
  // Before spawning workers, ensure release worktree + paths are materialized
  if err := ensureReleaseWorktree(ctx, release); err != nil {
    return fmt.Errorf("cold-start bootstrap: %w", err)
  }
  ```
- The bootstrap MUST:
  1. Create/verify release-wt branch from integration branch
  2. Create/verify release worktree at `${primary}-worktrees/release-${release}`
  3. Write frontmatter back to primary's index.md
  4. Commit & push (or fail loudly)
- Failures → return error, do NOT start workers.

**File:line reference:** `/home/brad/.claude/bin/coach-loop:1745-1807`

---

### Bug 2: Nil agent/verifier factory → SIGSEGV

**Bash technique (coach-loop:545-581, `run_driver_dispatch`):**

- **Driver resolution is LOUD on failure (line 549-556, `driver_path`):**
  ```bash
  driver_path() {
    local p="$BATON_DRIVERS_DIR/$1.sh"
    if [[ ! -x "$p" ]]; then
      echo "driver_path: no executable driver at $p" >&2
      return 1
    fi
    echo "$p"
  }
  ```
  Non-zero exit code from a missing driver. Caller MUST check (line 578-581):
  ```bash
  if ! drv_bin="$(driver_path "$drv")"; then
    # Leave the result file absent — preclassify_result PAGEs on it.
    return 0
  fi
  ```
- **Fallback is sensible:** If driver path fails, `run_driver_dispatch` does NOT crash — it returns cleanly (exit 0) with an ABSENT result file. The result reader (line 700-703, `preclassify_result`) treats a missing result as `PAGE:driver wrote no result line`.
- **Auto-detection is conditional (line 568-576):** Driver selection uses pattern matching on the model prefix:
  ```bash
  case "$model" in
    google/*|deepseek/*|mistral/*|groq/*|...) drv="oai-compat" ;;
    ollama/*|ollama-local/*|ollama-cloud/*) drv="ollama-native" ;;
    codex/*) drv="codex" ;;
    *) drv="${BATON_RUNTIME_DRIVER:-claude-cli}" ;;
  esac
  ```
  If the model doesn't match and `BATON_RUNTIME_DRIVER` is unset, defaults to `claude-cli` — the ONLY driver guaranteed to exist (it's first-class baton, shipped in the repo).

**Why bash avoids the bug:** Every driver path is resolved and validated BEFORE use. Missing binary → explicit PAGE, never a crash. Default driver is always available.

**Go port recommendation (critical):**
- Implement `driverFactory(model string) (Driver, error)` with explicit error return and panic-free fallback:
  ```go
  func driverFactory(model string) (Driver, error) {
    var driveName string
    switch {
    case strings.HasPrefix(model, "google/"):
      driveName = "oai-compat"
    case strings.HasPrefix(model, "deepseek/"):
      driveName = "oai-compat"
    // ... other providers ...
    default:
      driveName = "claude-cli"
    }
    
    drv, err := loadDriver(driveName) // reads from disk, checks executable bit
    if err != nil {
      return nil, fmt.Errorf("driver %q not available: %w", driveName, err)
    }
    return drv, nil
  }
  ```
- **Callers MUST check the error** and escalate to Page instead of proceeding.
- **No nil pointers:** A missing driver cannot produce a nil agent that later dereferenced. Return error or a panic in `init()` when vendored drivers are missing.

**File:line reference:** `/home/brad/.claude/bin/coach-loop:545-581`

---

### Bug 3: Universal agent-loop serialization bug: `content,omitempty` drops required `content` field

**Bash technique (oai-compat.sh:559-577):**

The oai-compat driver ALWAYS emits `content` in assistant messages, even when null or empty:

```bash
jq -nc \
  --arg model "$model" \
  --arg content "$content_acc" \
  --arg finish_reason "${finish_reason:-stop}" \
  --argjson tool_calls "$tool_calls_json" \
  --argjson usage "$usage_json" \
  '{model: $model,
    choices: [{
      finish_reason: $finish_reason,
      message: ({
        role: "assistant",
        content: (if $content == "" then null else $content end)  # ALWAYS emitted
      }
      + (if ($tool_calls | length) == 0 then {} else {tool_calls: $tool_calls} end))
    }],
    usage: $usage}' > "$response_file"
```

**Key design (line 570):** `content: (if $content == "" then null else $content end)` is ALWAYS in the object. The field is PRESENT — it is just null when empty. This is different from the `tool_calls` field (line 575), which is OMITTED when empty.

**Why bash avoids the bug:** `content` is always in the JSON (even if null), so providers that check for field presence don't choke.

**Go port recommendation (high priority):**
- When marshaling assistant messages, do NOT use `omitempty` on the `Content` field:
  ```go
  type AssistantMessage struct {
    Role      string       `json:"role"`
    Content   *string      `json:"content"`  // NO omitempty
    ToolCalls []ToolCall   `json:"tool_calls,omitempty"`
  }
  ```
- Test both: message with empty content + message with content.

**File:line reference:** `/home/brad/.claude/bin/drivers/oai-compat.sh:559-577`

---

### Bug 4: 25-turn cap hit with "no text response" — model never emits a terminal text turn

**Bash technique (oai-compat.sh:851-872, force-summary):**

The driver detects a "success but no final message" case and injects a **force-summary turn**:

```bash
# Force-summary pass: reasoning-heavy models often stop after their last tool call
# with no text content — result_text stays empty. One cheap follow-up prompt gets
# the summary text the interpreter needs.
if [[ "$status" == "ok" && -z "$result_text" ]]; then
  # ... spawn another turn with a summary prompt ...
  result_text="$(jq -r '.choices[0].message.content // empty' < "$_sum_resp_tmp" 2>/dev/null || true)"
  turn=$((turn + 1))
fi
```

**Loop exit condition (oai-compat.sh:778-783):**
```bash
# Break on stop only if no tool calls accumulated
local tc_len; tc_len="$(printf '%s\n' "$tool_calls" | jq 'length' 2>/dev/null || echo 0)"
if [[ "$tc_len" == "0" ]]; then
  break  # EXIT: no tools pending
fi
```

The exit condition is **NOT "received stop reason"** — it is **"no tool calls to execute"**. A model that emits `finish_reason:stop` but also a tool call will loop again.

**Why bash avoids the bug:** The driver owns the loop exit logic. Reasoning models may be verbose. Rather than failing on a cap, it detects empty text after successful completion and injects ONE synthetic follow-up. The loop exits on "no more tools", not "turn budget exhausted".

**Go port recommendation:**
- Detect end-of-dispatch via **"no tool calls remain"**, not "turn budget exhausted".
- On successful completion but empty result text, send ONE follow-up prompt:
  ```go
  if len(toolCalls) == 0 && resultText == "" {
    // Inject a summary turn
    resultText, err = askForSummary(ctx, messages)
    numTurns++
  }
  ```
- Set `MAX_TURNS` as a circuit-breaker for pathological loops, not the normal exit.

**File:line reference:** `/home/brad/.claude/bin/drivers/oai-compat.sh:851-872`

---

### Bug 5: Retries RESET the worktree and discard the best attempt

**Bash technique (coach-loop:765-789, `commit_worktree_wip`):**

Before EVERY dispatch, the loop **auto-checkpoints** uncommitted work:

```bash
commit_worktree_wip() {
  local wt="$1" label="${2:-dispatch}"
  # Skip if mid-merge/rebase, or if tree is already clean
  # ...
  log "auto-checkpoint uncommitted work before ${label}"
  git -C "$wt" add -A 2>/dev/null || return 0
  git -C "$wt" commit -q -m "wip(coach-loop): auto-checkpoint uncommitted work before ${label}" 2>/dev/null || true
}
```

**Called before every dispatch (coach-loop:1385):**
```bash
commit_worktree_wip "$W_WT" "$NEXT_TYPE $SLICE"
```

Every dispatch starts from a clean tree with a commit history. Retries don't reset — they continue from the best state so far.

**Why bash avoids the bug:** Partial progress is preserved and counts as "forward motion". The verifier (fresh session, Rule 7) sees the full history and the best attempt's output.

**Go port recommendation (critical):**
- After EVERY agent dispatch, call `git add -A && git commit` before the next dispatch:
  ```go
  if err := autoWIPCommit(ctx, worktree, sliceID, role); err != nil {
    log.Warn("auto-commit failed", "slice", sliceID, "err", err)
  }
  ```
- DO NOT reset or clean the worktree on retry. Carry the history forward.

**File:line reference:** `/home/brad/.claude/bin/coach-loop:765-789`

---

### Bug 6: Hardcoded openai/* escalation models cascade-fail a non-OpenAI run

**Bash technique (coach-loop:102-104):**

Models are configured via **environment variables**, not hardcoded:

```bash
IMPL_MODEL="${COACH_IMPL_MODEL:-${COACH_MODEL:-fable[1M]}}"
CAPTAIN_MODEL="${COACH_CAPTAIN_MODEL:-${COACH_MODEL:-fable[1M]}}"
VERIFIER_MODEL="${COACH_VERIFIER_MODEL:-${COACH_MODEL:-fable[1M]}}"
```

**Per-role model rotation (coach-loop:1475-1490):** If `COACH_IMPL_MODELS` is set (comma-separated), the loop rotates through models on failures.

**Runtime configuration source (coach-loop:44-50):** The env is sourced from `~/.config/coach/env`, which can have ANY models.

**Why bash avoids the bug:** No hardcoded model list. The loop reads from config. If a model fails or cascades, the operator changes the env and restarts — no code change needed.

**Go port recommendation:**
- **NO hardcoded model strings.** Read all models from a config file or environment.
- Model rotation must be **configurable**, not hardcoded per-role.
- Provider inference is **in the driver**, not the orchestrator.

**File:line reference:** `/home/brad/.claude/bin/coach-loop:102-104`

---

### Bug 7: Phase fail-fast cancels all sibling tracks on one failure

**Bash technique (coach-loop:1956-2024):**

The coordinator runs one worker **per track**, in parallel, and a failure in one track does NOT cancel others. Each worker runs in its own subshell. The ONLY global stop signal is the pause marker.

**Why bash avoids the bug:** Tracks are independent. A PAGE in track T2 does not kill T1 or T3 — it only sets the global pause marker. Other tracks finish their iteration or see the pause and exit cleanly.

**Go port recommendation:**
- Each track runs in its own goroutine with its own `runTrackWorker()` loop.
- A PAGE from one track does NOT cancel other goroutines.
- Implement a shared pause signal (context or file) that all workers check, but workers do NOT abort mid-dispatch.

**File:line reference:** `/home/brad/.claude/bin/coach-loop:1956-2024`

---

### Bug 8: Tool-exec pollutes the worktree with go module/build caches

**Bash technique (coach-loop:607-643):**

The loop **does NOT clean caches** — instead, it **loads the repo's own env file** into the dispatch:

```bash
# Load go/.env (DB creds + encryption keys) into the dispatch environment
_goenv="${_primary:-.}/go/.env"
if [[ -f "$_goenv" ]]; then
  # Parse key=value and add to env_wrap
  # ...
fi
```

The dispatcher inherits the repo's settings. If the repo wants to isolate caches, it edits `.env`.

**Why bash avoids the bug:** The loop does NOT try to sandbox caches — that is the repo's responsibility. The repo can set `GOCACHE=.cache/go` in `.env`, and every dispatch inherits that.

**Go port recommendation:**
- **Allow the repo to control cache locations via `.env`**, not the orchestrator.
- Load `go/.env` into the dispatch environment:
  ```go
  func loadGoEnv(repoPath string) (map[string]string, error) {
    env := make(map[string]string)
    // Parse repo/go/.env
    // ...
    return env, nil
  }
  cmd.Env = append(os.Environ(), mapToEnv(goEnv)...)
  ```

**File:line reference:** `/home/brad/.claude/bin/coach-loop:607-643`

---

## Part A-bis — LIVE bug observed 2026-06-28: merge-track confirmation-stall loop

During the DeepSeek sworn-build coach run, T2 (fully built + verified: S08/S09/S10, 17 commits) got
stuck in an infinite `/merge-track` retry (9+ dispatches, ~every 50s). Same pair this report keeps naming:
1. **Interactive role behavior in an autonomous context.** The `/merge-track` agent emits a
   "**Merge confirmation — Track T2… No blockers**" summary and STOPS to await a human go-ahead instead of
   executing the merge. Correct for a human at a slash command; a stall for the unattended loop.
2. **Interpreter gap (FT-1).** The loop can't classify an "awaiting-confirmation" output as a verdict, so
   it re-routes `/merge-track`, the agent re-asks, and it loops forever (never PAGEs, never proceeds).

**Corrected mechanism (from the log):** the haiku interpreter DOES run on the merge output — the log
shows `interpretation RETRY`. So it is not deterministic-only. The interpreter is a **classifier**
(DONE/RETRY/BLOCKED), not a responder: it sees the merge did not COMPLETE and returns RETRY → re-dispatch
→ re-ask → loop.

**ROOT CAUSE (found 2026-06-28) — a dangling auto-confirm flag.** The coach-loop ALREADY passes
`BATON_AUTO_CONFIRM=1` as the dispatch env_prefix for `/merge-track`
(`coach-loop` ~line where `dispatch_and_interpret "$MERGE_TRACK_MODEL" "/merge-track …" … "BATON_AUTO_CONFIRM=1"`).
But NO command honoured it — `grep -rl BATON_AUTO_CONFIRM ~/.claude/commands` returned nothing. So
`merge-track.md` Step 3 unconditionally called `AskUserQuestion`; with no human, the agent emits the
confirmation summary and ends the turn → the merge never executes → RETRY loop. The wiring was half-done:
loop sets the flag, command ignores it.

**FIX APPLIED (one file).** `~/.claude/commands/merge-track.md` Step 3 now honours `BATON_AUTO_CONFIRM`:
when set (autonomous loop) AND the Step 1.4 gate `<ready_to_merge>` is true, proceed without
`AskUserQuestion` (the deterministic gate is the authorization) and log an `auto-confirm` line; when unset
(human driving), keep the interactive confirm. Minimal, uses the existing flag, human UX unchanged.
`merge-release` does NOT need it — the loop `page_coach`s a human to run it (human-triggered by design),
so its confirm is correct. This is the interim fix; the three-tier Captain-in-session design below is the
fuller long-term form.

**Design fix (Brad, 2026-06-28) — interpreter as a bounded in-session responder.** Keep the merge
dispatch session OPEN; when the agent asks "proceed?", the interpreter replies "proceed" in-session and
the same session executes the merge. Turns classify-and-retry into respond-and-continue using the chat
interface's existing multi-turn nature — no human, no new interactivity model. CRITICAL GUARDRAIL: the
interpreter's "proceed" must be authorized by the DETERMINISTIC merge gate (all slices verified via the
oracle + no blockers), not the interpreter's own opinion — else it is a cheap model rubber-stamping a
merge (homework-marking). The gate computes the truth; the interpreter only relays it into the open
session. Non-interactive in spirit (gated, mechanical decision), chat-interactive in mechanism.
This upgrades the FT-1 interpreter spec from one-shot output-classifier to bounded in-session responder:
it may answer an agent's clarifying question ONLY when a deterministic gate supplies the authoritative
answer; otherwise PAGE once (never silent re-loop). Maps to sworn S01-llm-interpreter + S05-merge-gate.

**Design evolution (Brad, 2026-06-28) — three-tier confirmation/escalation with session continuity:**
- **Tier 1 — interpreter (haiku): classify.** Clean verdicts (DONE/RETRY/BLOCKED) route deterministically.
- **Tier 2 — Captain (sonnet/opus): adjudicate.** When the worker emits a confirmation/clarification the
  interpreter can't resolve, the Captain (the Rule-9 judgment + escalation authority) reads it, HOLDS THE
  SESSION OPEN, and either ANSWERS so the worker proceeds in-session, or — only if it's unclear or touches
  a point that must go to a human (Type-1 / design / policy) — escalates. Right role for a judgment call;
  tiering keeps cost down (Captain only on the ambiguous moments, not every dispatch).
- **Tier 3 — human PAGE, with the worker session HELD OPEN.** On escalation the worker session is NOT
  killed; it is suspended so the human's answer RESUMES the same session with full context — no full
  rework (the difference between losing 24 turns and losing nothing).

Implementation notes: (a) "hold the session open" = CHECKPOINT-AND-RESUME, not a live held process — a
page can last hours; persist + resume (`claude --resume <session-id>` for claude-cli; saved conversation
transcript replayed for oai-compat) so it survives long pauses and process restarts. (b) Gate stays the
authority for mechanical answers (Captain relays the oracle's verdict on a merge; it does not rubber-stamp).
(c) Log the Captain's answer / escalation reason to the orchestrator decision-log (S02) for the audit
trail (who confirmed the merge, why) — load-bearing for the regulated-delivery story.

---

## Part B: Running the Eval via Coach-Loop — Feasibility & Commands

### Release Selection & Configuration

Active release from `~/.config/coach/active-release.<repo-key>`. Models/keys from `~/.config/coach/env`.

### Model Configuration & Driver Mapping

- `google/*` → `oai-compat` (GOOGLE_API_KEY)
- `deepseek/*` → `oai-compat` (DEEPSEEK_API_KEY)
- `groq/*` → `oai-compat` (GROQ_API_KEY)
- Bare name → `claude-cli` (CLAUDE binary)
- `ollama/*` → `ollama-native` (Ollama endpoint)

### Exact Command to Run Eval

```bash
# In the sworn repo
cd /home/brad/projects/sworn

# Set active release
coach use 2026-06-27-conformance-foundation

# Start the loop
coach loop

# Or with model overrides
COACH_IMPL_MODEL=google/gemini-2.5-pro-preview \
COACH_VERIFIER_MODEL=google/gemini-2.5-pro \
coach loop

# Monitor
coach log
coach top
```

### Feasibility Assessment

**YES.** A release in the sworn board can run end-to-end via coach-loop in a fresh clone with:
- API keys in `~/.config/coach/env`
- `go/.env` with `TEST_DATABASE_URL` (if tests require it)
- Integration branch checked out

**Expected time:** 45 min–3 hours depending on parallelism and model speed.

---

## Summary: Top 5 Architectural Learnings

1. **Cold-start materialization is ONE POINT OF SERIALIZATION:** The bash loop calls `ensure_release_worktree()` **before any worker spawns**. Go must do the same.

2. **Driver contract is PROCESS-BOUNDARY SAFE:** Every driver outputs one normalized result JSON. No in-process factory, no nil dereference.

3. **Content field is ALWAYS EMITTED:** Never use `omitempty` on content. Go marshaling must always emit the field, even if null.

4. **Exit condition is "NO TOOL CALLS", NOT "TURN BUDGET":** The agent loop exits when there are no pending tool calls, not when it hits the turn cap.

5. **Retries PRESERVE WORKTREE STATE:** Every dispatch auto-commits work. Retries continue from the best state, not a reset tree.

