# Coach-loop parity archaeology baseline

Date: 2026-07-24  
Branch: `prep/v0.3.0-coach-parity`  
Head: `6ab7dc251ff4cac23cdbffa9cd1a828961efe61f`

## 1) Exact commits/paths inspected

- Local Sworn repo baseline:
  - `docs/captures/2026-07-24-sworn-v0.3-greenfield-scope.md` @ `6ab7dc25`, `aa95b3da`, `ccde1ebe`, `969157ad`, `6fab4f43`
  - `docs/captures/2026-07-24-sworn-coach-parity-archaeology.md` (this file)
  - `docs/roadmap.md` @ `6fab4f43`, `bcc31d9c`, `221ee4c8`, `3601bb82`, `74d4bf3c`, `af1c9696`
- `docs/adr/0001-greenfield-v1-kernel.md` @ `6fab4f43`, `65f00c35`, `eeda9731`, `03522666`
- `docs/adr/0006-current-authority-controller.md` @ `8f01b2ab`, `3f3e2ecb`, `a7b1f75e`, `af1c9696`, `74d4bf3c`
- `docs/adr/0007-native-agent-boundary.md` @ `bcc31d9c`, `3601bb82`
- `docs/releases/v0.2.0.md` @ `6ab7dc25`
- `AGENTS.md` @ `6fab4f43`, `03522666`
- Historical loop sources from Fired installation mirror:
  - Container git for this inspection: `/home/brad/projects/fired` @ `592c9f91ddb5003da6c108bcfed6c2087a8d2751`
  - `.../baton-install-backup/opencode/commands/captain-dispatch.sh` @ not git-anchored in this workspace (file evidence from `/home/brad/projects/fired` snapshot)
  - `.../baton-install-backup/opencode/commands/captain-route.sh` @ not git-anchored in this workspace (file evidence from `/home/brad/projects/fired` snapshot)
  - `.../baton-install-backup/opencode/commands/captain-prepare.sh` @ not git-anchored in this workspace (file evidence from `/home/brad/projects/fired` snapshot)
  - `.../baton-install-backup/opencode/commands/coach` @ not git-anchored in this workspace (file evidence from `/home/brad/projects/fired` snapshot)
  - `.../baton-install-backup/opencode/commands/coach-loop` @ not git-anchored in this workspace (file evidence from `/home/brad/projects/fired` snapshot)
  - `.../baton-install-backup/opencode/commands/coach-ntfy-bridge.sh` @ not git-anchored in this workspace (file evidence from `/home/brad/projects/fired` snapshot)
  - `.../baton-install-backup/opencode/commands/release-board-status.sh` @ not git-anchored in this workspace (file evidence from `/home/brad/projects/fired` snapshot)
  - `.../baton-install-backup/opencode/commands/release-board-ui.mjs` @ not git-anchored in this workspace (file evidence from `/home/brad/projects/fired` snapshot)
  - `.../baton-install-backup/opencode/commands/lib/release-board.mjs` @ not git-anchored in this workspace (file evidence from `/home/brad/projects/fired` snapshot)
  - `.../baton-install-backup/opencode/commands/release-verify.sh` @ not git-anchored in this workspace (file evidence from `/home/brad/projects/fired` snapshot)
  - Drivers from the mirror:
    - `.../baton-install-backup/opencode/commands/drivers/codex.sh` @ not git-anchored in this workspace (snapshot)
    - `.../baton-install-backup/opencode/commands/drivers/claude-cli.sh` @ not git-anchored in this workspace (snapshot)
    - `.../baton-install-backup/opencode/commands/drivers/oai-compat.sh` @ not git-anchored in this workspace (snapshot)
    - `.../baton-install-backup/opencode/commands/drivers/ollama-native.sh` @ not git-anchored in this workspace (snapshot)
    - `.../baton-install-backup/opencode/commands/drivers/completion.sh` @ not git-anchored in this workspace (snapshot)
- Internal strategy memory (for rebuild direction):
  - `.../sworn-internal/docs/captures/2026-06-24-baton-v0.4.0-opencore-session-handoff.md` @ `ba8aac4`, `b568e99`, `449e110`
  - `.../sworn-internal/docs/captures/2026-06-28-synthesis-and-forward-plan.md` @ `a1d7b07`, `449e110`
  - `.../sworn-internal/docs/captures/2026-06-28-session-handoff.md` @ `a1d7b07`
- Internal strategy docs:
  - `.../sworn-internal/docs/strategy/2026-06-12-baton-extraction-scope.md` @ `449e110`
  - `.../sworn-internal/docs/strategy/2026-06-13-baton-mvp-launch-spec.md` @ `449e110`
  - `.../sworn-internal/docs/strategy/2026-06-14-swornagent-reference-implementation-no-orchestration-segment.md` @ `449e110`
  - `.../sworn-internal/docs/strategy/2026-06-15-swornagent-mvp-pivot-e2e-native-turnkey.md` @ `449e110`

Missing path requested but not present: `/home/brad/projects/fired/docs/releases` (directory does not exist in current filesystem).

## 2) Recovered topology and user experience (verified vs inferred)

- Verified: Topology is worktree/branch based:
  - Track-level worktrees are explicit in board metadata (`worktreePath`, `worktreeBranch`).
  - `release-wt/<release>` branch carries release-wide composition and completion metadata.
  - Track ownership is tracked by branch (`track/<release>/<track-id>`) and index track field.
  - Slice state is read from branch-scoped `status.json`, with fallback to release-wt and working tree only when needed.
- Verified: Single oracle drives all scheduling decisions:
  - `release-board.mjs` is the branch-aware oracle for terminal and web views.
  - Both `release-board-status.sh` and `release-board-ui.mjs` render from that oracle (same truth).
  - `captain-dispatch.sh` filters actionable slices by track-order, state, dependencies, and touchpoint collisions.
  - `captain-route.sh` drives transitions for each state (`planned`, `design_review`, `implemented`, `failed_verification`, `verified`, merge gates).
- Verified: Recovered control flow (reference journey):
  - Planner creates/replans release (`/plan-release`, `/replan-release`) and sets tracks.
  - Implementer runs `/implement-slice` -> writes progress, can reach `design_review` and `implemented`.
  - Captain reviews via `/review-tldr`, emits `review.md` and enforces ACK/DECLINE contract.
  - Verifier runs `/verify-slice` in fresh context and writes verdict (pass/fail/blocked, violations, pending).
  - Merge path performs `/merge-track` and then `/merge-release` after all tracks are merged into `release-wt`.
  - Coach loop pauses for human intervention when required (`PAGE`, blocked, ack decisions, ship).
- Verified: Recovery and orchestration behavior in loop:
  - One scheduler per track (parallel mode), one worker per active track; coordinator and worker manifests/logging.
  - pause / resume / retry / take/attach semantics exposed by `coach` commands and `coach-loop` state files.
  - In-flight / orphan protections for `in_progress`, stale `verification.result=pending`, and merge gating via git ancestry.
- Verified: User experience surfaces:
  - Terminal: `coach` command center (`status`, `next`, `dispatch`, `board`, `workers`, `log`, `pause`, `resume`, `loop`).
  - Web: `release-board-ui.mjs` (live-ish board, next-command hints, per-slice drill-down, worker panel).
  - Phone: `coach-ntfy-bridge.sh` with command grammar `status|review|ack|decline|note|note!|pause|resume|loop`.
  - Gate/instrumentation: deterministic board view, event/log trails, worker metrics (cost/duration), per-slice evidence links.
- Inferred (not directly confirmed in this run): exact behavior of some planner/operator command implementations (`plan-release`, `implement-slice`, etc.) that live as runtime role prompts/agents outside the inspected script set; inferred from orchestrator contracts.

## 3) Feature-by-feature mapping to v0.3 stages and Gate 9

| Stage | Feature | Baseline recovery | Sworn v0.3 parity intent |
|---|---|---|---|
| S0 | Branch reset / evidence seam | Verified (scope doc + v0.2 release notes): pinned baseline and documented release cutover sequencing | Preserve pinned Baton snapshot + protocol evidence gating; no mixed claims about RC2+ parity before evidence |
| S1 | Single-track correctness | Inferred/supplemented by `coach-loop` sequential defaults and state machine | Keep as first runnable track in isolated S0 track; then fan out |
| S2 | Native CLI roles | Verified: Codex/Claude/OAI/ollama/OAI-comp compat drivers, role-specific invocation | Keep role-independent driver contract; per-role model selection remains configurable |
| S3 | Coach topology + recovery | Verified: `coach-loop`, `captain-route`, `captain-dispatch`, release-wt/track model | Implement one track + one worker per track, deterministic routing, orphan/CRASH recoveries, pause/resume/takeover, merge gating |
| S4 | HTTP/cloud variants | Inferred: presence of HTTP-compatible driver in scope and driver files | Retain adapter surface but defer hardening to real integration tests |
| S5 | Observability | Verified: worker manifests/events logs, board UI metrics, `top`/`workers` command | Preserve operator-readable evidence + measurable metrics (cost, duration, retries) with optional telemetry |
| S6 | Cockpit (terminal/web) | Verified: status/board/workers commands + dashboard UI + ntfy bridge | Keep terminal + embedded web overlay for live control, replay-safe over restart |
| S7 | Parity proof | Verified: acceptance gate 9 defined in scope; explicit S06/M07 lessons in internal docs | Publish scenario-level parity matrix + measured evidence bundle before claiming `ready` |
| S8 | Public cutover | Verified: explicit post-S7 release-readiness prerequisite in scope | Only update public product surface after gate-9 completion |

### Gate 9 mapping (to parity evidence)

- Gate 1: “real autonomous-engine cases” → must run old-logic scenarios against rebuilt scheduler.
- Gate 2: real multi-track completion → run multi-track unattended to `merge-release`.
- Gate 3: all drivers pass shared deterministic/live smoke → codex/claude + one cloud driver minimum.
- Gate 4: verifier freshness + containment → fresh-context verifier semantics with immutable artifacts.
- Gate 5: recovery at all external-effect edges → kill/restart loop+workers paths.
- Gate 6: terminal/web truth during restart/failure → board/worker overlays remain consistent.
- Gate 7: telemetry fault-tolerant → optional projection only.
- Gate 8: no unratified gap in parity matrix → every row below must be either “implemented” or explicitly deferred.
- Gate 9: measured delta (time/token/retry/quality) vs preserved baseline runbook.

## 4) What should not be carried forward into Sworn

- The private Bash loop orchestration itself (`coach-loop`, `coach`) as production implementation; recreate as kernel-native behavior instead of embedding private scripts.
- Opaque/private operational details that depend on fired-specific machine state (e.g., `~/.claude` project corpus assumptions, local credential hosts).
- The legacy “memory-search” semantic retrieval that depends on a fixed personal Ollama host corpus.
- Scrub-sensitive shell-topic/host leakage patterns that were only incidentally present in legacy scripts.
- The exact bash interpreter heuristics that were tied to old host/environment behavior (replace with explicit typed state transitions in Go).
- Any claim of Baton RC2 or later marketing claims without corresponding parity evidence in v0.3 bundle.

## 5) Missing evidence (explicit gaps)

- Exact current versions of `plan-release`, `implement-slice`, `merge-track`, `merge-release`, and `review-tldr` command implementations are not fully recovered from this snapshot (only caller/oracle behavior is captured).
- Legacy exact driver matrix for all cloud adapters is partially implied (not fully enumerated in inspected files).
- `release-wt` and `track/*` garbage-collection lifecycle timing across very long-running sessions was only partially evidenced.
- Concrete historical timing/latency baselines for gate-9 metrics require a runnable baseline run dataset.

## 6) Executable S7 parity scenarios (measurable)

These are acceptance tests to execute against the rebuilt Sworn v0.3 loop against a fixture repo.

1. Multi-track dependency+parallel dispatch
   - Run one release with at least 2 tracks and one `depends_on` track edge.
   - Expected: only non-blocked tracks dispatch initially; dependent track remains idle until prerequisite track merged into `release-wt`.
   - Measure: no invalid `/implement-slice` dispatched against dependency-gated slices; wall time to dependent dispatch.

2. One-worker-per-track isolation
   - Add two independent slices in separate tracks and run orchestrator with parallel enabled.
   - Expected: max one active worker per track, no inter-track working-tree conflicts.
   - Measure: worker manifest count equals track count; no shared-track double occupancy.

3. Crash-orphan recovery
   - Kill process mid `/implement-slice`; restart loop.
   - Expected: orphaned `in_progress` slice is resumed; progress continues to `implemented` then `verified` where appropriate.
   - Measure: recovery time-to-resume and no duplicate conflicting dispatch to same slice.

4. Verifier freshness + stale pending
   - Force a verifier crash to leave `implemented` with `verification.result=pending`.
   - Expected: next route re-dispatches verifier and stale verdict is overwritten by new run.
   - Measure: count of pending verdicts returns to 0 after successful retry.

5. Merge sequence strictness
   - Complete all slices in Track A but leave Track B unmerged.
   - Expected: no `/merge-release` until every tracked branch is ancestry-merged into `release-wt`; route goes through pending merge-track.
   - Measure: command sequence `merge-track*` occurrences precede first `merge-release`.

Unresolved archaeology question (still open): whether historical runbooks include a canonical numeric budget for acceptable stale/retry thresholds by scenario (e.g., max acceptable retries and max stale-branch tolerance) that should be carried as hard gate criteria.
