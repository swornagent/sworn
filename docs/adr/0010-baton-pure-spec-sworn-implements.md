# ADR 0010: Baton is pure specification; Sworn is the implementation

## Status

Accepted (2026-06-26) — ratified by the Coach (Brad). **Type-1 seam revision.** It
revises the 2026-06-24 open-core seam in one spot (the board oracle) and is the
direct sequel to ADR-0009 (records as JSON). Sequenced before the work it
authorises (capture-before-change, Rule 2/3).

## Context

ADR-0009 moved the loop's machine-read artefacts to JSON records (`board.json`,
`spec.json`, `proof.json`). That raised the question it deferred: **who runs the
mechanical gates that read those records?**

The gates already existed *twice*:

- ~3,800 lines of Baton `bin/` tooling — bash with embedded Python plus Node
  `.mjs` (the board oracle `lib/release-board.mjs`, `release-trace.sh`,
  `release-verify.sh`, `release-coverage.sh`, `release-audit-design.sh`, …).
- The Sworn Go binary — `sworn verify`, `lint` (ac/trace), `reqverify`,
  `reqvalidate`, `designfit`, `designaudit`, `specquality`, `regress`,
  `llm-check`, `journeys`, `ship`. Essentially the same gate suite.

Both still parsed Markdown. Migrating *both* to JSON would be duplicated effort,
and maintaining two implementations of `trace`/`verify`/`coverage` in lockstep is
the DRY-on-knowledge violation Baton exists to prevent. Bash is also a poor JSON
target — `jq` gymnastics bolted onto an already-polyglot bash+python+node base.
Sworn already vendors Baton at a semver pin and is already transforming bash→Go.

So the records-as-JSON move is the moment to collapse the gates to one
implementation rather than carry two.

## Decision

**Converge the gate and oracle *implementation* on the Sworn Go binary. Baton
becomes pure specification.** The line is: **Baton specifies, Sworn implements.**

- **Baton (the specification, public).** Rules (Markdown), role prompts
  (Markdown), record schemas (JSON Schema), record templates (JSON), and the
  conformance contract — what each gate must check, that it fails closed, and the
  oracle's state-resolution semantics. **No binaries, no bash, no `.mjs`.** Baton
  is self-describing: anyone could implement it. That, not "ships a binary," is
  what makes Baton standalone.
- **Sworn (the reference implementation, and the product — open Go binary).**
  Implements every gate plus the oracle, then adds orchestration (`sworn run`) and
  the hosted-layer hooks on top. Continues to vendor Baton at a semver pin.

**Arm's-length is preserved, and sharpened.** Baton and Sworn stay separate repos.
The dependency is *soft*: the contract is the schemas + rule semantics, so Sworn is
the canonical runner but not the only possible one. Convergence clarifies the seam
(spec vs implementation) rather than collapsing it. The thing explicitly *not*
done: moving Baton's rules/prompts/schemas into the Sworn repo — that would collapse
them; Baton stays the upstream spec that Sworn vendors.

**Baton-standalone tiers:**

- *Zero binaries* — drive the loop by hand: paste the prompts, the LLM emits the
  JSON records, the human eyeballs them. Pure Markdown, no dependency.
- *+ the open `sworn` binary* — automated gates. One zero-dependency static binary
  is a better minimal-deps story than the old bash+python+node+jq.

Baton never *requires* Sworn; it requires it only to automate the gates, and even
then the binary is open and the contract is implementable.

**Role prompts reference gates abstractly.** Gate invocations name the gate by its
protocol role ("run the trace gate", "the proof-bundle verification gate") with a
"reference implementation: `sworn <cmd>`" pointer, rather than hardcoding bash
(`bin/release-*.sh`) or welding the spec to the product binary name.

## Seam revision (explicit)

The 2026-06-24 open-core seam kept the **board oracle in Baton** (as
`lib/release-board.mjs`). This ADR moves the oracle *implementation* to Sworn (Go);
the oracle *contract* (the `board-v1` schema + state-resolution rules) stays in
Baton. This is the one point where ADR-0010 revises the ratified seam, recorded
here rather than dropped silently.

## Options considered

- **Rewrite the bash gates to read JSON.** Rejected: duplicates the Go gates,
  invests in the layer we are retiring, and bash is a poor JSON target.
- **Keep both bash and Go gate suites.** Rejected: ongoing bash↔Go drift, two test
  surfaces, the DRY violation.
- **Converge on Go; Baton pure-spec (chosen).** One implementation, JSON-native,
  cleaner open-core split, less total work.
- **Keep one minimal bash first-pass script as a zero-dependency floor.** Considered
  and declined in favour of pure-spec — the zero-binary tier is served by the
  by-hand loop, so even a minimal bash floor is unnecessary surface in Baton.

## Consequences

- **Baton repo:** remove `bin/` (gates + oracle). README/ROADMAP updated to "plain
  Markdown + schemas," dropping the "+ one optional shell script" framing. Baton's
  surface becomes rules + prompts + schemas + templates + conformance contract.
- **Sworn repo:** owns all gate + oracle implementations and migrates them to read
  `board.json` / `spec.json` / `proof.json` (Phase B of the records-as-JSON work).
  Continues vendoring Baton.
- **Role prompts:** gate references abstracted + `sworn` pointer (lands in the
  records-as-JSON prompts PR, `sawy3r/baton#52`).
- **Adopters:** a Baton-standalone user who wants automated gates installs the open
  `sworn` binary; the by-hand loop needs no binary.
- **One implementation of each gate** — no bash↔Go drift to police.

## References

- ADR-0009 (records as JSON, prose as Markdown) — the parent decision.
- The 2026-06-24 open-core seam ratification (Baton = protocol incl. oracle;
  Sworn = product) — revised here on the oracle's implementation home.
- `sawy3r/baton#52` (records-as-JSON: role prompts + gates) — where the prompt
  abstraction and `bin/` removal land.
