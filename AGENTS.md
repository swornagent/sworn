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

## How work is organised (we dogfood Baton)

Work is **sliced**. Each slice lives at `docs/release/<release>/<slice-id>/` with
a `spec.md` (the contract) and `status.json` (machine state). State machine:

`planned → in_progress → implemented → verified | failed_verification`

- The **implementer** implements against the spec and writes a proof bundle **from
  live repo state** (git diff + test output), then stops at `implemented` — it
  **never certifies its own work**.
- A **fresh-context verifier** (no implementer context, artefact-only) returns
  PASS / FAIL / BLOCKED, fail-closed. Only `verified` slices may merge.

Read the active release board at `docs/release/2026-06-15-e2e-turnkey-loop/`
(`index.md` + the relevant slice's `spec.md`) before implementing anything.

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
