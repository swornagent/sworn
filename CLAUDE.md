# CLAUDE.md

Claude Code — and any other agent or model — should use **[AGENTS.md](AGENTS.md)**
as the canonical guidance for this repository. One source of truth; do not
duplicate guidance here (a second detailed copy drifts).

The rules most worth repeating as a safety net:

1. **Fail closed.** Exit `0` only on PASS. Single Go binary, **minimal, justified deps**
   — stdlib preferred; the model client uses `net/http` + `encoding/json`,   **not** a provider SDK like `github.com/openai/go-openai`. Each new dep requires an ADR (see ADR-0007).
2. **This repo is public-safe.** No business / pricing / competitive / strategy
   content, and no references to private/internal repositories. Strategy lives
   privately, elsewhere.

Everything else — layout, build/test, the slice workflow, conventions — is in
[AGENTS.md](AGENTS.md).

## Engineering Process — Baton

This project follows the **Baton** rule-set (see `docs/baton/` for
full rule docs and provenance). Seven rules, listed in priority order:

### 1. Reachability Gate (CRITICAL)

For any feature with a user-facing affordance (UI control, route, form field,
API endpoint), the first failing test must render through the integration point
that owns the affordance — NOT the leaf component in isolation.

- If the integration point can't render the feature yet, THAT failure is the
  correct TDD red. Build the integration glue first; the leaf falls out.
- Leaf-level unit tests are fine in addition. They cannot be the sole proof of
  life.
- A component imported only by its own test file is a red flag. Investigate
  before claiming task done.

Before marking any phase complete, produce a **reachability artefact**:
screenshot, end-to-end test run, or explicit "open browser, do X, observe Y"
smoke step. A green typecheck plus green unit suite is not a reachability
artefact.

### 2. No Silent Deferrals

"Deferred" as an inline code comment is not a decision unless all three are
present: **why** (concrete reason), **tracking** (linked issue, plan task, or
punch-list item), **acknowledgement** (decision-maker told in plain text).
Without all three, the inline comment is rationalisation, not decision.

### 3. Capture Discipline

Conversation context is the most ephemeral persistence layer. Subagent findings
and session decisions must land in durable storage before session ends.

**Durability hierarchy (most to least permanent):** git history → code →
`docs/` → GitHub Issues → project memory → conversation context.

Bias every capture decision toward higher-permanence layers. Conversation
context is a working surface, not a storage surface.

### 4. Commit Messages as Capture Layer

Commits that land a documented decision MUST restate the decision in the
message body, not just "see plan X." Plans get edited and moved; `git log`
is permanent. Use 3–5 line bodies for any commit landing a decision.

### 5. Session Discipline

Implementation sessions of non-trivial scope are anchored to GitHub Issues.
Session start: confirm the issue. During session: capture decisions at natural
breakpoints. Session end: record decisions, completed work, deferred items,
next steps.

### 6. Proof Bundle (CRITICAL)

Before marking any task, phase, or session complete, produce a **proof bundle**
at the appropriate path, generated from **live repo state** — not recalled from
context. Required sections: Scope, Files changed, Test results, Reachability
artefact, Delivered, Not delivered, Divergence from plan.

Claiming completion without a proof bundle is a silent deferral of verification
(Rule 2).

### 7. Adversarial Verification (CRITICAL)

No slice may transition to verified state without a PASS verdict from a
**fresh-context session** loaded only with the slice artefacts and live repo
state. The session that implemented the work is never allowed to certify it.

Verifier return contract: exactly one of PASS, FAIL (with numbered violations),
or BLOCKED (with reason). Fail closed — absence of evidence is FAIL, not
optimistic PASS. The verifier does not propose redesigns, does not edit
production code, and does not consult the implementer for clarification.

State machine: planned → in_progress → implemented → [fresh verifier] →
verified | failed_verification. The implemented checkpoint exists so no agent
can shortcut straight to verified.

Full rule docs: `docs/baton/`.
