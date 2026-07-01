# ADR 0009: Records as JSON, Prose as Markdown — A Structured Source of Truth

## Status

Accepted (2026-06-26) — ratified by the Coach (Brad), who directed the
records-as-JSON work to proceed and Baton to be perfected ahead of slicing it into
the reference implementation as a new release. **Type-1 decision (Baton Rule 9):**
structural and hard to reverse — the board/spec/proof formats are Baton protocol
surface and existing live boards must migrate. The near-term scanner-replacement
work (see Consequences) stands on its own and does not depend on this format change.

Execution is tracked on the Baton repo: epic `sawy3r/baton#50`, with record-schema
issues #45 (board), #46 (spec), #47 (proof), #48 (journeys) and role-prompt update
#49. `ledger` is treated as reference-implementation (Sworn) surface, not protocol.

## Context

A recurring corruption class has bitten the release boards: a "newline-eating"
edit fuses two structured lines into one — e.g. `id: T14   slices: [...]` on a
single line, or a `state: merged---` frontmatter fence glued to a scalar. The
most recent instance hid the slice lists of tracks T14 and T17 from the board
parser (`TestLiveReleaseBoardsAreValid` caught it on 2026-06-26).

Examining where it does and does not happen is instructive:

- **`status.json` has never corrupted.** It is written with `json.MarshalIndent`
  (`internal/state/state.go`) and read back as JSON. A fused line is *still valid
  JSON* — whitespace between tokens is insignificant, so reflow/newline damage is
  survivable, and the marshaller cannot emit a malformed structure by
  construction.
- **`index.md` corrupted repeatedly.** It is the one board artifact the binary
  only *reads*, never writes; it is edited as free text (by automation, humans, or
  models) and parsed by a hand-rolled line scanner (`internal/board` splits on
  `\n`). Its frontmatter is YAML-shaped, and **in YAML, indentation and line
  breaks *are* the syntax** — a fused line does not error; `yaml.Unmarshal` would
  read the tail as a scalar and silently drop the following key. YAML is uniquely
  defenceless against exactly this corruption, on top of its other footguns
  (boolean/number coercion, multiple spec versions; the tree already carries two
  YAML libraries).

Two deeper smells compound it:

1. **Bespoke scanners over structured docs.** `internal/board`, `internal/lint`,
   and `internal/lint/status_time.go` parse structured content with
   `strings.Split` / `HasPrefix` / `regexp` rather than a marshaller — one site
   even comments that it deliberately *avoids* `json.Unmarshal` and walks the path
   by hand. This reimplements a parser badly and is the actual brittleness.
2. **Dual-authored documents.** `index.md` (and `spec.md`, `proof.md`) are
   *simultaneously* a machine contract and a human document, edited by both. That
   is why they drift: there is no single source of truth.

### Two consumption models and one human

The boundary must serve both ways Baton is consumed:

- **A — Baton as a standalone protocol.** Installed by an LLM or via the install
  script; a human drives the slash commands manually in their LLM interface,
  and/or scripts their own automation, and/or has an agentic orchestrator (Hermes)
  run the loop end to end. Here the **LLM (or automation) emits the records** from
  the human's conversation.
- **B — Sworn, the reference implementation.** Baton in an autonomous loop with the
  verified-releases capability. Here the **binary and worker LLMs emit the
  records**; the human reviews at gates.

In *every* model the human's authoring surface is **conversation**, and their
control surface is **review of a rendered view, then approve/reject**. A human
never sits down and hand-types a record's JSON — config aside, hand-authoring JSON
is a developer taste most humans (the project owner included) do not share. This
is decisive for the boundary below: JSON-for-records costs the human nothing,
because no human ever edits the raw record.

The question this ADR answers: **what does each artifact's source format need to
be, given that records are emitted (never hand-authored) and reviewed as renders,
config is structured, and only documents are composed by hand?**

## Decision

### The discriminator: emitted vs hand-authored

The line is not "records vs prose" — almost everything is human-readable, and most
of what looks like prose is in fact emitted, then reviewed. The line is **who
produces the artifact and how**. That sorts every artifact into three buckets:

1. **Records** — board, spec, proof, status, journeys, ledger. Content with
   *fields*: state, the track/slice/dependency graph, acceptance criteria (id +
   EARS clause + trace + test refs), proof facts (files changed, test results,
   reachability, delivered/not-delivered), journey definitions + walk status,
   ledger/cost rows. **JSON, always emitted** by the LLM, the binary, or a UI form
   — **never hand-authored** — and **rendered to markdown for human review.**
   Human judgement that belongs in a record (a Rule 9 design decision, a Rule 8
   validation sense-check, a journey ratification, a ship attestation) is a
   **structured field with a prose rationale**: the human says a sentence, the tool
   wraps it. Still never raw JSON.

2. **Config** — architecture rules, design-system declaration, model/provider
   settings, per-release/per-slice overrides. **JSON**, authored via the **hosted
   UI** (the preferred path) or, in standalone Baton, by the LLM or install script.
   The existing design/architecture schemas already live here.

3. **Documents** — ADRs, guides, READMEs, longform captures. **Markdown,
   hand- or LLM-authored**, living *outside* the loop record graph. This is the
   only place a human composes markdown by hand.

What this reclassifies: intake narrative and "spec rationale" — filed as
hand-authored prose in this ADR's first draft — are really **LLM-drafted,
human-reviewed records with prose fields**, not things a human types into either a
JSON or a markdown editor. True hand-authored markdown shrinks to bucket 3.

### The invariant

> **No human ever hand-authors a record. Records are emitted as JSON (LLM / binary
> / UI form), validated against a schema, and rendered to markdown for review.
> Config is JSON authored through a UI or script. Markdown is hand-authored only
> for documents outside the record graph. The machine parses JSON only — never
> prose.**

If any flow asks a human to hand-edit a record's JSON, that is the bug.

### Rendering is the implementation's job, not the protocol's

Baton (the protocol) defines the **JSON record + its schema**. *How* a record is
shown to a human is an affordance each implementation layers on top:

- Standalone Baton (model A): the LLM renders a view on request ("show me the
  board"); the human reviews in chat.
- Sworn (model B): renders markdown views and provides review surfaces
  (`sworn top`, the TUI, gate prompts).
- Hosted: renders records in the web UI; config and decisions captured via forms.

This keeps the layering clean — **protocol = schema'd records; everything
human-facing = renderer** — and means the protocol does not mandate a rendering
engine. Where a rendered markdown file is committed (e.g. `index.md`), it is a
build artifact of its JSON source (see the drift guard in Consequences).

### Per-artifact mapping

Per slice:

| Artifact | Bucket | Source | Human sees |
|---|---|---|---|
| `status.json` | record | JSON *(already — the proof of the pattern)* | rendered state in TUI/board |
| `spec.json` | record | JSON (id, scope, ACs as EARS + trace + test refs, touchpoints) | rendered `spec` view |
| `proof.json` | record | JSON (files changed, test results, reachability, delivered/not-delivered) | rendered `proof` bundle |

Per release:

| Artifact | Bucket | Source | Human sees |
|---|---|---|---|
| `board.json` | record | JSON (tracks / slices / deps / state / worktree paths) | rendered `index.md` (tables + dependency diagram) |
| arch / design configs | config | JSON | hosted UI form (or LLM/script in standalone) |
| ADRs, guides | document | Markdown (hand/LLM-authored) | the markdown itself |

### Authoring path for emitted records

Records the planner/implementer/verifier *produce* are emitted via
schema-constrained structured output (validated against the published JSON Schema),
not written as markdown and scraped back. In standalone Baton this is the slash
command instructing the LLM to emit against the schema; in Sworn it is the binary's
structured-output call; in hosted it is a form write. All three validate on the way
in. A validated round-trip is far more durable than free-text-plus-scanner — and it
is the same mechanism in every consumption model, which is what makes the protocol
portable.

## Options considered

(Named to avoid collision with the A/B *consumption models* above.)

- **Opt-1 — Everything JSON, including hand-authored prose.** Purest single-source
  story. Rejected: it pushes documents (ADRs, guides) and rationale into JSON
  string fields — hostile to author and diff, and it fights Rules 8/9. The purity
  is not worth making the document bucket miserable, and it buys nothing, because
  records are emitted anyway.
- **Opt-2 — Three buckets by emitted-vs-hand-authored (chosen).** Records are
  emitted JSON, validated and rendered; config is JSON via UI/script; documents are
  hand-authored markdown outside the record graph. Gets the robustness guarantee —
  *machine parses JSON only, no human hand-authors a record* — while leaving the one
  bucket humans actually compose (documents) as markdown.
- **Opt-3 — Status quo, harden the parsers in place.** Keep markdown/YAML sources;
  replace the bespoke scanners with a real parser + validate-on-write. Cheapest,
  but YAML's whitespace-significance keeps the corruption *possible* rather than
  *impossible*, and the dual-authored drift remains. Adopted only as the near-term
  step *toward* Opt-2 (it is a strict win regardless), not as the end state.

## Consequences

- **The record schemas are Baton's API surface (model A).** For an arbitrary LLM
  running the slash commands — or someone's automation, or Hermes — to emit a
  board / spec / proof / journey that is valid and interoperable, each record type
  needs a **published schema it can be pointed at** (structured-output target +
  validator). Baton today publishes schemas for *config* and the *status leaf* but
  not the records that flow through the loop; closing that gap is what makes "a
  protocol any LLM can drive" true rather than aspirational. See Appendix A.
- **Migration.** Existing live boards and slice folders migrate to the split
  format; the planner / replan flows that write them, and the Baton oracle that
  reads them, update accordingly. The board format is Baton protocol surface
  (see ADR-0006, ADR-0008), so the change is coordinated across Baton and sworn.
- **The oracle simplifies.** Reading `board.json` replaces hand-parsing markdown
  frontmatter — fewer bespoke scanners, no whitespace-sensitivity.
- **A drift guard becomes mandatory.** Rendering creates a second copy that can
  diverge (someone hand-edits a generated `index.md`). Fail closed: treat rendered
  `.md` as build artifacts and add a CI check asserting `committed.md ==
  render(json)`, failing on divergence. This keeps single-source-of-truth honest.
- **Near-term independent win (does not require ratifying Opt-2).** Replace the bespoke
  `strings.Split` / `HasPrefix` scanners in `internal/board` and `internal/lint`
  with marshaller round-trips and run the validator as a write-time post-condition.
  Tracked separately (relates to `sworn#20`, `sworn lint --fix`).
- **Validation everywhere the binary writes.** Each command that writes a
  record re-reads and validates its own output before returning success, and
  refuses to leave a corrupt file — pinning failures to the producing command.

## References

- ADR-0003 (sqlite orchestration state), ADR-0006 / ADR-0008 (Baton protocol sync /
  canonical Baton) — the board format is shared protocol surface.
- Baton Rule 8 (Requirements Fidelity) and Rule 9 (Design Fidelity) — why prose
  surfaces stay human-owned and why this is a Type-1 decision.
- `docs/captures/2026-06-26-v0.1.0-release-test-plan.md` — the corruption instance
  that motivated this ADR.

## Appendix A — Schema inventory and gaps (as of 2026-06-26)

Five schemas are published at `baton.sawy3r.net/schemas/` (canonical `$id`; the
`/schemas/` index returns 404 — no directory listing, but each file resolves at its
`$id`). They cover **config** and the **status leaf** — the edges of the loop — but
not the records that flow through it.

### Published (overlap)

| Schema | Bucket | What it covers |
|---|---|---|
| `slice-status-v1` | record (leaf) | one slice's runtime state |
| `architecture-rules-v1` | config | project architectural rules |
| `architecture-overrides-v1` | config | per-release rule overrides |
| `design-fidelity-v1` | config | design-system declaration (Rule 9) |
| `design-allowlist-v1` | config | per-slice design escape hatch |

### Gaps (records used, no schema)

| Needed schema | Bucket | Today in sworn | Rule | Cost |
|---|---|---|---|---|
| `board-v1` | record | `index.md` YAML frontmatter, hand-parsed | — | **High value** — corruption surface; oracle input; `board.json` |
| `spec-v1` | record | `spec.md` (markdown) | 8 | Defines ACs (EARS) + scope + touchpoints + trace; not JSON yet |
| `proof-v1` | record | `proof.md` (markdown) | 6 | Proof facts: files changed, tests, reachability, delivered/not-delivered |
| `journeys-v1` | record | `.sworn/journeys.json` — **already JSON** (`internal/journey` structs) | 10 | **Cheap** — struct exists; publish a schema |
| `ledger-v1` | record | ledger — **already JSON** (`internal/ledger`, `"v":1`) | — | **Cheap** — sworn-side; publish a schema |

### Alignment problems on top of the gaps

1. **sworn points at a placeholder URL.** Emitted records carry
   `"$schema": "https://example.com/schemas/baton/slice-status-v1.json"`, not the
   canonical `baton.sawy3r.net` `$id`. Hardcoded in `internal/run/run.go:293`,
   `internal/mcp/tools_plan.go:55`, and every committed `status.json`.
2. **`slice-status-v1` is ahead of what sworn emits.** The hosted schema includes
   `covers_needs`, `validation`, `design_decisions`, `release_base`,
   `release_benefit`, `org_objective`; sampled emitted files stop at `verification`.
   Decide whether the schema is **authoritative** (sworn must emit those) or
   **descriptive**, and reconcile.
3. **The four design schemas describe the dead bash harness.** Their `description`
   fields cite `release-audit-design.sh`, not `sworn designaudit`. Confirm the field
   contracts still match what the Go binary reads before adopters see them.

### Sequencing (leverage-first)

1. `board-v1` — unblocks the anti-corruption centerpiece and the oracle simplification.
2. `spec-v1` — the Rule 8 contract everything traces to.
3. `proof-v1` — Rule 6.
4. `journeys-v1` + `ledger-v1` — nearly free; completes the set.
5. Reconcile `slice-status-v1` drift and fix the `example.com` URL in one pass.
6. De-bash the four design-schema descriptions.

Items 1–4 are the **Baton-protocol push** (publish the record API). The URL fix,
validate-on-write, and renderers are the **Sworn implementation** work.
