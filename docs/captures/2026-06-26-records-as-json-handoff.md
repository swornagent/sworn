# Handoff — records-as-JSON + Baton pure-spec (2026-06-26)

Session handoff for the records-as-JSON migration and the Baton→pure-spec /
gate-convergence work. Written so a fresh session can finish the external-facing
Baton content and then drive the Sworn-side slicing. Spans two repos:
`sawy3r/baton` (the protocol) and the local `sworn` repo (the product).

## TL;DR — where we are

- **Schemas published + merged** (`sawy3r/baton#51`, merged to `main`): the loop
  record schemas. Baton now has a JSON-record API.
- **Role prompts migrated** (`sawy3r/baton#52`, open draft): all four prompts emit/
  consume the JSON records and reference gates abstractly via a `sworn` pointer.
- **Two ADRs ratified** (sworn `docs/adr/0009`, `0010`).
- **Immediate next (tonight's target):** make Baton **pure spec** — strip `bin/`,
  rewrite `README.md` + `install.sh`, migrate the templates. This is the
  external-facing content. Details in "Remaining work A" below.
- **Later (Phase B):** slice records-as-JSON into Sworn as a new release.

## The mission (don't lose this framing)

Two products launching: **Baton** (open protocol) and **Sworn** (the product +
hosted). The session reshaped how the loop's artefacts are stored and who runs the
gates, to make both launch-ready.

## Decisions ratified — do NOT re-litigate

1. **ADR-0009 — records as JSON, prose as Markdown.** The boundary is
   *emitted-vs-hand-authored*, three buckets:
   - **Records** (board, spec, proof, status, journeys, ledger): JSON, **emitted**
     by LLM/binary/UI, **never hand-authored**, validated against schema, **rendered**
     to Markdown for human review.
   - **Config** (arch/design/model settings): JSON, authored via hosted UI or
     LLM/script.
   - **Documents** (ADRs, guides, READMEs): hand-authored Markdown, outside the
     record graph.
   - Invariant: *no human hand-authors a record; the machine parses JSON only.*
   - **Rendering is the implementation's job, not the protocol's.**
   - Key human-factors driver: Brad does not enjoy hand-authoring JSON; the point
     is humans never touch raw records — they converse and review renders.
2. **ADR-0010 — Baton is pure spec; Sworn implements.** Converge the gate + oracle
   *implementation* on the Sworn Go binary. Baton = rules + prompts + schemas +
   templates + conformance contract, **no binaries**. Line: *Baton specifies, Sworn
   implements.* Arm's-length preserved (separate repos, **soft** dependency — the
   contract is the schemas + rule semantics; Sworn is canonical, not the only
   possible runner). Tiers: zero-binary by-hand loop (LLM emits JSON, human
   eyeballs) → + open `sworn` binary for automated gates. **Seam revision:** the
   board oracle's *implementation* moves Baton(`.mjs`)→Sworn(Go); the oracle
   *contract* (board-v1 + state-resolution) stays in Baton.
3. **Records vs data at the moat boundary.** The attestation *schema* (record
   shape) is public protocol; attestation *data/telemetry* is private moat. The
   public-repo pre-push guard was too blunt (substring-matched `attestation`); it
   now carves out `^schemas/.*\.json$`. Guard master copy:
   `~/.claude/baton/public-repo-pre-push-hook` (+ installed `baton/.git/hooks/pre-push`).
4. **`ledger` is Sworn-side product surface, not Baton protocol.** Schematize it in
   Sworn, not Baton.
5. **`covers_needs` lives in `spec.json`** (the contract). `slice-status-v1` also
   defines it → reconcile in `#50` (decide mirror vs drop).
6. **Planner "16 hats" → 6 considerations.** Reframed as *considerations, not
   roles*: a mandatory floor (security & privacy, compliance & legal,
   accessibility, performance — forget-easy, retrofit-expensive) + applied-where-
   they-bear (user experience, architecture & fit). Requirements elicitation is the
   spine of the prompt, not a hat. Em-dash-free per the prompt's own style rule.
7. **Schema conventions:** strict `additionalProperties: false`; version-bump
   (`-v2`) for additions; **self-contained `$defs`** (a portable schema an LLM
   points at shouldn't resolve external `$ref`s); shared `id_token` pattern
   (`^[A-Za-z0-9][A-Za-z0-9._-]*$`) makes newline-fusion fail validation by
   construction; group cohesive sibling scalars into objects (`worktree`,
   `vertical_trace`, `ratification`, `boundary`); references are id-strings, owned
   entities are objects.
8. **Baton PR merge style = merge commit** (matches repo history), not squash.

## Schemas — state

Published at `baton.sawy3r.net/schemas/` (canonical `$id`; the `/schemas/` index
404s, files resolve at their `$id`). Merged via PR #51, plus 2 fields added on the
#52 branch:

| Schema | Notes |
|---|---|
| `slice-status-v1` | pre-existing; **drift**: hosted schema has `covers_needs`/`validation`/`design_decisions`/`release_base`/`release_benefit`/`org_objective`; emitted sworn files stop at `verification`. Decide authoritative vs descriptive + reconcile. |
| `board-v1` | + `release.worktree { path, branch }` (added on #52 branch — implementer needs it) |
| `spec-v1` | + `risks []` (added on #52 branch — captain design-review treats mitigations as binding) |
| `proof-v1` | files_changed, test_results, reachability, delivered, not_delivered, divergence |
| `journeys-v1` | ratification + journeys + steps |
| `attestations-v1` | human-walkthrough record; `boundary { real_infra, mocks_off }` |
| design/arch (4) | pre-existing config schemas; **descriptions still cite the dead `release-audit-design.sh`** — de-bash them. |

**Alignment bugs to fix (Sworn-side, Phase B):** every `status.json` + `run.go:293`
+ `mcp/tools_plan.go:55` emit `$schema: https://example.com/schemas/baton/...`
(placeholder) instead of the canonical `baton.sawy3r.net` URL.

## Repos / branches / artefacts

- **Baton** (`sawy3r/baton`):
  - `main`: PR #51 merged (5 schemas) + guard carve-out + public oracle fix `b6b3bf6`.
  - branch `feat/role-prompts-records` → **PR #52 (open, draft)**: 4 prompts
    migrated + gate abstraction + `board-v1.release.worktree` + `spec-v1.risks`.
  - Role prompts live at `claude/baton/role-prompts/{planner,implementer,verifier,captain}.md`.
  - Gate scripts to remove: `bin/release-*.sh`, `bin/lib/release-board.mjs`,
    `bin/release-board-ui.mjs` (~3,800 lines).
- **Sworn** (local `release/v0.1.0`):
  - `docs/adr/0009-records-json-prose-markdown.md`, `docs/adr/0010-baton-pure-spec-sworn-implements.md`.
  - `docs/captures/2026-06-26-v0.1.0-release-test-plan.md` (the original task; Phase 0
    done, Phases 1-6 pending).
  - This handoff: `docs/captures/2026-06-26-records-as-json-handoff.md`.

## Issues

- Baton epic **#50** (records-as-JSON). Children: #45 board, #46 spec, #47 proof,
  #48 journeys (all closed by PR #51); **#49** role prompts (PR #52, in progress).
- Sworn **#20** (index.md corruption root-cause + `sworn lint --fix`), **#22**
  (replace bespoke scanners with marshaller round-trips + write-time validation).

## Remaining work A — Baton pure-spec

All in the `sawy3r/baton` repo, on `feat/role-prompts-records` (→ PR #52).

**A1 — external-facing landing + install: DONE** (commits through `c17be08`):
strip `bin/` (`72154c5`); templates → JSON (`board.json`/`spec.json`/`proof.json`);
`install.sh` + `install-codex.sh` to pure-spec; `design-fidelity.md`; top-level
`README.md` rewrite + new "Baton is pure spec" section + `sworn` as reference impl.

**A2 — residual old-model references: TODO** (15 files still carry `bin/` gates /
`spec.md`/`proof.md`/`index.md` / "16 hats"). The migration touched the four role
*prompts* but not the rest. Highest-priority first:
- **`claude/commands/*.md` (8 files)** — the user-invoked slash-command wrappers.
  These carry their own Step-0 worktree-discovery logic (read `index.md`, the
  `awk` materialise hack), `bin/` gate calls, and `spec.md`/`proof.md` reads. They
  are as load-bearing as the role prompts — migrate them the same way
  (board.json/spec.json/proof.json, abstract gate refs, delete the awk hack).
- **`claude/baton/README.md`** — the *embedded/installed* docs-package README
  (distinct from the repo landing page already done). It is vendored into Sworn and
  its `## The eleven rules` heading is what `cmd/sworn/doctor.go` checks — keep that
  heading verbatim.
- **Rule docs** `adversarial-verification.md`, `requirements-fidelity.md` (reference
  `spec.md`/`proof.md`/gates); `AGENTS-fragment.md`; `INSTALL.md` (bin/ install
  steps); `architecture.json`; `RULES-HISTORY.md`; `ROADMAP.md`.
- `release-mode-template/status.json` — check its `$schema` URL.
Find them all: `grep -rlE "release-(verify|board|trace|coverage|audit|mock|regression|llm)|bin/release|/(spec|proof|index)\.md|16 hats" claude/ ROADMAP.md`

The original numbered list below is the A1 record (now done).

### A1 detail (done)

1. **Strip `bin/`** — `git rm -r bin/` (the bash gates + `.mjs` oracle + HTML
   dashboard). All gate logic now lives in Sworn (Go).
2. **Rewrite `README.md`** to the pure-spec model. Specific touch points found:
   - L90: the "full gate suite: `release-trace.sh` …" sentence → reframe as
     "Rules 6-11 are mechanically enforceable; the reference implementation is the
     open `sworn` binary (`sworn verify`, `sworn trace`, `sworn coverage`,
     `sworn designaudit`, `sworn regression`, `sworn llm-check --check …`)."
   - L172-181: the whole **bin/ install table** → delete (Baton installs no bin/).
   - L260: workflow narrative still says "reads `spec.md`, writes `proof.md`, runs
     `release-verify.sh`" → records-as-JSON + abstract-gate treatment
     (`spec.json`/`proof.json`/the verification gate).
   - L297: "`release-verify.sh` is the proof-bundle gate …" → reframe to the
     proof-bundle verification gate (`sworn verify`).
   - KEEP the protocol value: the eleven rules, bidirectional-DRY framing, failure
     modes, "what this is NOT", adoption paths. That content IS Baton.
   - Update "## The eleven rules" heading stays (doctor checks this exact string in
     Sworn — `cmd/sworn/doctor.go`).
3. **Rewrite `install.sh`** (+ `install-codex.sh`) — its current job is largely
   copying `bin/` to `~/.claude/bin`. Becomes "vendor the spec" (rules + prompts +
   schemas + templates into the adopter's project / `~/.claude/baton/`), or shrinks
   drastically. Remove the `release-verify.sh`/board-tooling copy steps
   (install.sh L108-119, L30-33, L141-143).
4. **Update `claude/baton/design-fidelity.md`** L116 (`bin/release-audit-design.sh`
   → the design-conformance gate / `sworn designaudit`).
5. **Migrate `release-mode-template/`** → JSON record templates: `board.json` (was
   `index.md`), `spec.json` (was `spec.md`), `proof.json` (was `proof.md`);
   `status.json` already JSON; `intake.md`/`journal.md` stay Markdown (prose).
   These templates double as the future gate test fixtures.
6. **Update PR #52 body** — the "Gates (bin/)" TODO section is now wrong (ADR-0010:
   gates are *removed*, not migrated to JSON-bash). Reframe to "strip bin/, pure-spec
   README/install, templates."
7. Verify the pre-push guard still passes (no moat paths); push; the PR is then the
   full "records-as-JSON + Baton pure-spec" change.

## Remaining work B — Sworn Phase B (LATER, new release via `/plan-release`)

Slice the records-as-JSON migration into Sworn:
- Fix the `example.com` `$schema` URL → `baton.sawy3r.net` (status.json template +
  `run.go:293` + `mcp/tools_plan.go:55`).
- `board.json` adoption: oracle reads `board.json` (replaces `index.md` frontmatter
  parsing in `internal/board`); renderer emits `index.md` from it; migrate the live
  release boards.
- `spec.json` / `proof.json` / journeys adoption + renderers; Go gates read the JSON
  records.
- Scanner replacement (Sworn #22) + validate-on-write.
- Drift-guard CI: assert `committed.md == render(json)`.
- Reconcile `slice-status-v1` drift; de-bash the 4 design-schema descriptions.

## Also open from the original task (separate thread)

The session began as "test plan for building + testing Sworn before public release"
→ `docs/captures/2026-06-26-v0.1.0-release-test-plan.md`. **Phase 0 is done**
(CI green: fixed `implement_test.go` signature, the `doctor.go` "seven→eleven rules"
heading bug, an unrestored `HOME` env leak, and `index.md` newline-fusion in the
live board; gofmt'd the tree; added `gofmt`/`build` CI gates + fail-closed release
gates). **Phases 1-6 pending** (build matrix, CLI contract test, provider
integration, e2e `sworn run`, public-safety audit, release rehearsal). All Phase 0
work is committed on `release/v0.1.0` (`67e8227`, `899a82b`).

## Continuation handshake (Rule 6)

A session resuming this should first regenerate live state: `git -C
~/projects/baton log --oneline main..feat/role-prompts-records` and `git -C
~/projects/sworn log --oneline -8 release/v0.1.0`, and reconcile against the
"Decisions ratified" list above before new work.
