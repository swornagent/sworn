# ADR 0009: Records as JSON, Prose as Markdown — A Structured Source of Truth

## Status

Proposed (2026-06-26). **Type-1 decision (Baton Rule 9):** structural and hard to
reverse — the board/spec/proof formats are Baton protocol surface and existing
live boards must migrate. This ADR records options and rationale for Coach
ratification; it is not self-accepted. The near-term scanner-replacement work
(see Consequences) stands on its own and does not depend on ratifying the larger
format change.

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

The question this ADR answers: **where is the boundary between machine-owned
structured data and human-authored prose, and what format owns each side?**

## Decision

### The boundary: records vs prose

The discriminator is **how the content is authored**, not whether it is
human-readable (almost everything is):

- **Records** — content with *fields*: state, the track/slice/dependency graph,
  acceptance-criterion entries (id + EARS clause + trace links + test refs), proof
  *facts* (files-changed list, test command + exit code, reachability path,
  delivered/not-delivered items), journey definitions, ledger/cost rows. Humans
  *read* these as tables; they do not *compose* them as sentences.
  → **JSON is the source of truth. Markdown, where wanted, is a render.**

- **Prose** — content authored by writing sentences: intake narrative, design
  rationale / ADR bodies, the explanatory part of a spec, session/handoff notes.
  Forcing these into JSON string fields loses headings, lists, links, and clean
  prose diffs, and fights Rules 8/9 (deliberately human-owned).
  → **Markdown is the source of truth. The machine never parses it.**

### The invariant

> **The machine reads JSON only. Humans compose prose only in markdown the machine
> never parses. Any artifact that is *both* is split along the records/prose seam —
> structured half in JSON, prose half in markdown.**

The split is **per artifact, not per file**: `index.md` / `spec.md` / `proof.md`
are each both today, which is exactly why they are fragile.

### Per-artifact mapping

Per slice:

| Artifact | Kind | Source format |
|---|---|---|
| `status.json` | state machine | JSON *(already; this is the proof of the pattern)* |
| `spec.json` | id, scope, ACs (EARS + trace + test refs), touchpoints | JSON |
| `spec.md` | rationale / human prose | Markdown (prose section; machine ignores) |
| `proof.json` | files changed, test results, reachability ref, delivered / not-delivered | JSON |
| `proof.md` | human-readable bundle | Rendered from `proof.json` |

Per release:

| Artifact | Kind | Source format |
|---|---|---|
| `board.json` | tracks / slices / deps / state / worktree paths | JSON (the graph the oracle reads) |
| `index.md` | board tables + dependency diagram + session-log narrative | Rendered (tables from `board.json`) + hand-authored prose section the machine never parses |

### Two mechanisms (use both, per artifact)

1. **Render** — JSON is source, markdown is generated (board tables, proof
   bundle). Use where humans want to *see the structured data* in readable form.
2. **Sidecar / opaque body** — a JSON file (or strict JSON frontmatter) carries
   the contract; a separate markdown body is pure prose the machine treats as
   opaque. Use where the prose is genuinely independent (spec rationale, intake).
   No renderer needed.

Both guarantee the corruption immunity: the machine only ever touches JSON, whose
integrity survives reflow/newline damage; prose corruption cannot break the state
machine because nothing parses the prose.

### Authoring path for generated records

Records the planner/implementer/verifier *produce* should be emitted via
schema-constrained structured output (validated against a JSON Schema such as the
existing `slice-status-v1.json`), not written as markdown and scraped back. A
validated tool-call round-trip is far more durable than free-text-plus-scanner.

## Options considered

- **A — Full JSON (everything is JSON, all markdown rendered).** Purest
  single-source story. Rejected: it pushes intake / ADR / rationale into JSON
  string fields, which is hostile to author and diff and fights Rules 8/9. The
  purity is not worth making human-owned prose miserable.
- **B — Split by authorship (chosen).** JSON is canonical for records; markdown is
  canonical for prose; hybrids are split. Gets Option A's robustness guarantee —
  *machine reads JSON only* — without the prose cost.
- **C — Status quo, harden the parsers in place.** Keep markdown/YAML sources;
  replace the bespoke scanners with a real parser + validate-on-write. Cheapest,
  but YAML's whitespace-significance keeps the corruption *possible* rather than
  *impossible*, and the dual-authored drift remains. Adopted only as the near-term
  step *toward* B (it is a strict win regardless), not as the end state.

## Consequences

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
- **Near-term independent win (does not require ratifying B).** Replace the bespoke
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
