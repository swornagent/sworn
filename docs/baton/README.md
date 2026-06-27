# Baton — the open protocol SwornAgent embeds

This directory is a **vendored copy** of the open [Baton](https://github.com/sawy3r/baton)
engineering protocol. SwornAgent is the installable product; Baton is the
methodology it enforces. The relationship mirrors OCI ↔ Docker: two projects,
one install — the protocol ships inside the `sworn` binary (via `go:embed`), so
the loop runs with no external rule files.

The vendored upstream commit is pinned in [`VERSION`](./VERSION). Do **not** edit
the rule docs here to fork the protocol; update the pin and re-vendor from
upstream so the methodology stays a single source of truth.

## The seven rules

Baton is seven rules, **listed in priority order** (higher rule wins on conflict).
They are the normative core of the protocol and govern how every agent — and
human — works in this repo. Full text in [`rules/`](./rules/):

1. **[Reachability Gate](./rules/01-reachability-gate.md)** — a feature's first
   failing test must render through the integration point that owns its
   user-facing affordance, not the leaf component in isolation. Before any phase
   is "done", produce a reachability artefact (screenshot, e2e run, smoke step).
2. **[No Silent Deferrals](./rules/02-no-silent-deferrals.md)** — "deferred" is a
   decision only with all three of *why* (concrete reason), *tracking* (issue /
   task / punch-list), and *acknowledgement* (decision-maker told in plain text).
   Otherwise the inline comment is rationalisation, not a decision.
3. **[Capture Discipline](./rules/03-capture-discipline.md)** — conversation
   context is the most ephemeral persistence layer; findings and decisions must
   land in durable storage (git → code → docs → issues → memory) before a session
   ends.
4. **[Commit Messages as Capture Layer](./rules/04-commit-messages-as-capture.md)**
   — commits that land a decision restate the decision in the message body, not
   just "see plan X". `git log` is permanent; plans move.
5. **[Session Discipline](./rules/05-session-discipline.md)** — non-trivial work
   is anchored to an issue; capture decisions and trade-offs at natural
   breakpoints and at session end.
6. **[Proof Bundle](./rules/06-proof-bundle.md)** — before claiming any task,
   phase, or session complete, produce a proof bundle generated from **live repo
   state** (files changed, test results, reachability artefact, delivered /
   not-delivered). Claiming completion without one is a silent deferral of
   verification.
7. **[Adversarial Verification](./rules/07-adversarial-verification.md)** — no
   slice reaches `verified` without a PASS from a **fresh-context** session loaded
   only with the slice artefacts and live repo state. The implementer never
   certifies its own work; the verifier fails closed (absence of evidence is
   FAIL, not optimistic PASS).
8. **[Requirements Fidelity](./rules/08-requirements-fidelity.md)** — the spec
   is not an axiom. Requirements are verified (quality), validated
   (sense-check), and traced (need -> AC -> test -> proof) so a need cannot
   drop silently between intake and spec. The 2-D requirements traceability
   matrix (RTM) enforces this fail-closed.

This last rule is the one SwornAgent productises: **independent, fresh-context,
fail-closed verification of a change against its spec.**
## How the product consumes this

- The `sworn` binary embeds these rules and the derived role prompts
  (`internal/prompt/`, slice S04) so verification runs self-contained.
- The repo also **dogfoods** the protocol: work here is sliced under
  `docs/release/<release>/`, implemented and then certified by a fresh-context
  verifier, exactly as the seven rules require. See [`AGENTS.md`](../../AGENTS.md),
  which carries the canonical seven-rule fragment.
