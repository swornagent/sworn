# ADR 0002 — CLI binary name: `sworn`

Status: accepted (2026-06-15)

## Decision

The product brand is **SwornAgent**; the CLI binary is **`sworn`** (the
GitHub-CLI → `gh`, Kubernetes → `kubectl` split). Short, fast to type, reads as
verb-object: `sworn verify`, `sworn run`, `sworn top`.

- Go module stays `github.com/swornagent/swornagent`; the command package is
  `cmd/sworn`, so `go install .../cmd/sworn@latest` produces `sworn`.
- Distribution: `brew install swornagent/tap/sworn`, `go install`, container, and
  the GitHub Action — none collide. The bare npm name `sworn` is taken by a
  non-mainstream package; irrelevant (npm is not a primary channel; a scoped
  `@swornagent/*` is available if ever needed).

## Command surface (roadmap; MVP is `verify` only)

`sworn verify` (now) · `top` (Bubble Tea TUI; replaces bash `coach top`) · `run`
(E2E loop) · `plan` · `implement` · `merge` · `status` · `init` · `models`
(list / qualify / benchmark) · `cost` (FinOps) · `attest` (ledger) · `config`.

### TUI (`sworn top`) views

Runs (live PR/slice state, model, verdict, cost) · Board (release + depends_on
graph) · Cost/FinOps (live spend, cost-per-verified-PR) · Verdicts (failed gates,
rationale, proof) · Model fitness (pass/fail × task-type, escalations) · Approvals
(NEEDS_COACH / design-review queue, ack/decline) · Activity (dispatch log).
