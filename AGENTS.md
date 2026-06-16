# AGENTS.md

Canonical guidance for any agent or human working in this repository. Tool-
agnostic by design (Claude Code, other models, CI bots all read this).

## What this is

**SwornAgent** is the verification layer that makes coding agents accountable:
independent, **fresh-context adversarial verification** of a code change against
its spec, **fail-closed**, so unverified work cannot merge. The CLI binary is
**`sworn`**. Built on the open **Baton** protocol (embedded into the binary).

The MVP is the **end-to-end loop** — `sworn run`: plan → implement → verify →
(retry/escalate) → gated merge — turnkey and self-serve, not a standalone gate.

## Non-negotiables (do not violate)

- **Native Go, single binary, ZERO runtime dependencies.** Stdlib only. Adding a
  third-party dependency requires an ADR in `docs/adr/` justifying it. The point
  is a binary that runs in any CI or a scratch container with nothing else.
  Concretely: the model client (S02) speaks OpenAI-compatible `/chat/completions`
  via stdlib **`net/http` + `encoding/json`** — **NOT** `github.com/openai/go-openai`
  or any provider SDK. The same rule holds for every provider/runtime: normalise
  to one stdlib client, never pull a vendor SDK.
- **Fail closed.** Exit `0` ONLY on PASS. FAIL, BLOCKED, unconfigured, error, or
  an unparseable verdict are all non-zero and do not merge. The gate's default
  state is "no"; only positive evidence flips it to "yes." Never weaken
  `internal/verdict`'s contract or exit-code mapping.
- **Customer owns the model + key; SwornAgent owns the protocol.** Models are
  configurable and BYO-key — never hardcode a provider. The adversarial property
  (fresh context, artefact-only inputs) is enforced by the harness, not the model.
- **Safe-hosted default.** Any default model must be hosted in a trusted
  jurisdiction; never bless a non-trusted-hosted model as the default.
- **Security.** Never log API keys, request bodies, or model payloads.

## Docs discipline (this repo is public)

Only **public-safe, technical** docs belong here: `README.md`, `docs/adr/`,
`docs/release/` (technical slice specs), CONTRIBUTING. **Do NOT** add business,
pricing, competitive, moat, financial, or customer-strategy content, and do not
reference any private/internal repository or unrelated company in committed
files. Strategy is maintained privately, elsewhere.

## Layout

- `cmd/sworn/` — the CLI. **One file per subcommand** (`verify.go`, later
  `run.go`, `init.go`, `bench.go`); `main.go` only dispatches.
- `internal/verdict/` — the verdict contract (PASS/FAIL/BLOCKED + exit codes).
  The core invariant; touch with care.
- `internal/model/` — model client(s) behind a single interface; provider-neutral,
  BYO-key.
- `internal/verify/` — the verification protocol (deterministic first-pass →
  dispatch → conservative verdict parse).
- `internal/...` — engine/state/git/implement/run packages as slices land.
- `docs/adr/` — architecture decision records. `docs/release/<release>/` — the
  slice board.

## Build / test

```sh
make build     # -> bin/sworn
go test ./...
go vet ./...
gofmt -l -w .
```

Keep `go vet` clean and code `gofmt`'d. Every new package ships with tests; prefer
table-driven tests against fakes (e.g. `httptest`) so they run with no network and
no token spend.

## Engineering Process — Baton (we dogfood the protocol)

This project follows the **Baton** rule-set — the same protocol SwornAgent
productises. Full rule docs with provenance live in [`docs/baton/`](docs/baton/);
the seven rules below are the canonical fragment, adapted for this Go CLI. They
are **listed in priority order** (higher rule wins on conflict).

### 1. Reachability Gate (CRITICAL)

For any feature with a user-facing affordance (a `sworn` subcommand/flag, an exit
code, an API call, a config key), the first failing test must exercise the feature
**through the integration point that owns it** — the `cmd/sworn` dispatch or an
end-to-end `sworn` invocation — NOT a leaf function in isolation.

- If the entry point can't reach the feature yet, THAT failure is the correct TDD
  red. Build the wiring first; the leaf falls out.
- A function imported only by its own test file is a red flag. Investigate before
  claiming a task done.

Before marking any phase complete, produce a **reachability artefact**: an actual
`sworn <subcommand>` run with its output/exit code, or an end-to-end test that
drives the binary. A green `go test` on leaf packages is not a reachability
artefact.

### 2. No Silent Deferrals

"Deferred" is not a decision unless all three are present: **why** (concrete
reason), **tracking** (linked issue / slice / punch-list item), **acknowledgement**
(decision-maker told in plain text). Without all three, a `// TODO` / `// later`
on a contract surface is rationalisation, not a decision — surface it first.

### 3. Capture Discipline

Conversation context is the most ephemeral persistence layer. Decisions and
subagent findings must land in durable storage before a session ends. Durability
hierarchy, most to least permanent: **git history → code → `docs/` → GitHub
issues → memory → conversation.** Bias every capture toward the more permanent.

### 4. Commit Messages as Capture Layer

Commits that land a decision restate it in the **message body** (3–5 lines), not
just "see slice X" — `git log` is permanent, plans move. AI commits add a
`Co-Authored-By:` trailer after the rationale. Single-line messages are fine for
trivial mechanical changes only.

### 5. Session Discipline

Non-trivial work is anchored to a GitHub issue. Capture decisions and trade-offs
at natural breakpoints and at session end. Use issues for epics/specs/session
captures; use `docs/` for ADRs and stable reference material.

### 6. Proof Bundle (CRITICAL)

Before claiming any slice complete, produce a proof bundle at
`docs/release/<release>/<slice-id>/proof.md`, generated from **live repo state**
— not recalled from context. Required sections: **Scope**, **Files changed**
(`git diff --name-only <base>`), **Test results** (`go test ./...` + `go vet`),
**Reachability artefact** (Rule 1), **Delivered** (each with evidence),
**Not delivered** (each a Rule 2 deferral), **Divergence from plan** (empty is
valid; the section must be present). Claiming completion without a proof bundle is
a silent deferral of verification (Rule 2).

### 7. Adversarial Verification (CRITICAL)

No slice reaches `verified` without a PASS from a **fresh-context** session loaded
only with the slice artefacts (`spec.md`, `proof.md`, `status.json`) and live repo
state. **The implementer never certifies its own work.** Verifier returns exactly
one of `PASS` / `FAIL: <numbered violations>` / `BLOCKED: <reason>`, and **fails
closed** — absence of evidence is FAIL, not optimistic PASS. State machine:

`planned → in_progress → implemented → [fresh verifier] → verified | failed_verification`

The `implemented` checkpoint exists so no agent can shortcut straight to
`verified`. This rule is exactly what the `sworn` binary automates — we run it by
hand on ourselves until the binary can.

### Operating the board

Work is **sliced**: each slice lives at `docs/release/<release>/<slice-id>/` with
a `spec.md` (the contract) and `status.json` (machine state). Read the active
release board at `docs/release/2026-06-15-e2e-turnkey-loop/` (`index.md` + the
relevant slice's `spec.md`) before implementing anything. Only `verified` slices
merge.

## Branching

- **`main` is prod.** Never commit release work directly to it; `main` only
  advances via `/merge-release` when a release ships.
- **`release/vX.Y.Z` is the integration base** for a release (current:
  `release/v0.1.0`, cut from `main`). It is declared in the active release's
  `index.md` ("Release summary → Target version / integration branch") and as
  `release_base` in each slice's `status.json`, and merges to `main` at ship.
- Baton layers beneath it: `release/vX.Y.Z` ← `release-wt/<release>` (per-release
  worktree integration) ← `track/<release>/<track>` ← slice work. Each branch
  strictly contains its base, so the chain stays fast-forwardable and drift gates
  stay green.

## Conventions

- **Naming:** brand is **SwornAgent**; the binary is **`sworn`**.
- **Subcommands** each in their own file under `cmd/sworn/`; `main.go` only routes
  (it is the one documented shared file across tracks).
- **Workspace confinement:** file/command tools operate only within the workspace
  root.
- **Tests pair verdict + exit code.** Every test of a verification path asserts
  both the verdict AND the exit code — they must stay consistent (fail-closed).
- **Commits:** Conventional Commits (`feat|fix|docs|refactor|chore(scope): …`);
  restate any decision in the body, not just "update X"; AI commits add a
  `Co-Authored-By:` trailer.
- **No new runtime deps** without an ADR.

See `docs/adr/0001` (one binary, embedded protocol, distribution) and
`docs/adr/0002` (CLI name `sworn` + command/TUI surface).
