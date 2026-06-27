# ADR 0006 — Baton↔SwornAgent protocol sync (vendor-down + PR-up governance)

Status: accepted (2026-06-22)

## Context

**Baton** is the open engineering-process protocol — ten rules, role prompts,
slash commands, track-mode, proof bundles — published at `github.com/sawy3r/baton`
and tagged `v0.1.0`…`v0.3.0`. It is designed to be cloned and used in Claude,
Codex, or any agent **without** SwornAgent, and ships a lightweight bash/node
reference toolchain (`release-board-status.sh`, `release-verify.sh`,
`release-board-ui.mjs`) so the standalone protocol is runnable on its own.

**SwornAgent (`sworn`)** is the all-Go product built on Baton. It implements the
protocol natively in a single binary with no bundled bash/python, and surfaces
"SwornAgent vA.B.C **on Baton vX.Y.Z**". sworn embeds the protocol
(`internal/adopt/baton/` rules + `internal/prompt/baton/` prompts/track-mode) via
`go:embed`.

Three problems made the relationship implicit and drift-prone:

1. **The embed pins a raw SHA, not a semver tag.** `internal/adopt/baton/VERSION`
   reads `baton-protocol: cf158423f65c20860a3d4ec0310acb6cc7fb5aa0`. A SHA can't
   answer "which released protocol version is this binary on?" and silently drifts.
2. **No defined transform from Baton → sworn embed.** Baton's docs reference its
   bash/node scripts (`release-verify.sh`, `release-board-status.sh`,
   `design-audit.sh`, `port-deriver.sh`,
   `captain-memory-search.py`). Those are correct **for standalone Baton** but are   superseded inside sworn by the Go binary's own capabilities. Copying the
   protocol verbatim leaks script references the product can't honour.
3. **No governance for protocol changes discovered during sworn development.** The
   embed `VERSION` records `rules-added: 08/09/10 during the fidelity-layer
   release` — three rules born downstream in sworn with no upstream PR. That is a
   silent fork of an open protocol.

## Decision

### 1. Three layers, explicitly separated

- **Baton** (`~/projects/baton`) — the open protocol + its standalone bash/node
  reference impl. Clonable and usable without sworn. The bash/node stays; it is
  the reference, not a sworn dependency.
- **SwornAgent** — the all-Go product. No bundled bash/python. Implements the
  protocol natively; surfaces the Baton version it is built on.
- **Local bootstrap harness** (`~/.claude/baton/`, `~/.claude/bin/`,
  `captain-prepare.sh`, `captain-memory-search.py`) — the scaffolding that *builds*
  sworn. Not the product; may remain bash/python; absorbed into sworn over time
  (memory → S25, captain-prep/review → S46) and deleted as it is.

### 2. Vendor-down flow (`sworn baton vendor`, slice S48)

sworn pins Baton by **semver tag** (not SHA), reads the pinned upstream protocol,
and **transforms** it into the embed. The transform applies to **rules AND
role-prompts**, and its core job is to **strip Baton's bash/node script references
and replace them with sworn-native commands**:

| Baton reference            | sworn-native replacement |
|----------------------------|--------------------------|
| `release-verify.sh`        | `sworn verify`           |
| `release-board-status.sh`  | `sworn board`            |
| `design-audit.sh`          | `sworn designaudit`      |
| `port-deriver.sh`          | native (no script)       || `captain-memory-search.py` | `sworn memory search`    |

The embed is **sworn-native** (not a dual bash/Go copy). Re-running the vendor
reproduces the sworn-native embed deterministically, so it **subsumes the one-time
"public-readiness scrub"** of script refs — drift can't re-accrue because the
embed is generated, not hand-maintained.

### 3. Version surfacing + tag-pin discipline (slice S49)

The pin becomes a semver tag (`v0.3.0` at adoption), unified across
`internal/adopt/baton/VERSION` and `internal/prompt/VERSION.txt`. `sworn version`
reports "on Baton vX.Y.Z"; `sworn doctor` fails closed if the pin is a 40-char SHA
rather than a tag.

### 4. PR-up governance (slice S50, `sworn baton diff`)

sworn **never silently forks** the protocol. Protocol changes discovered during
sworn development are raised as a **PR against Baton → reviewed/authorised →
merged → sworn re-vendors** at the new tag. `sworn baton diff` compares the
embedded protocol against the upstream pin and surfaces any local divergence (the
"you've forked, you owe a PR" detector). `docs/baton-governance.md` documents the
workflow. The three fidelity-layer rules (08/09/10) and the VERSION/tag-discipline
ask are tracked upstream at **sawy3r/baton#31**.

## Consequences

- The binary can truthfully state which released protocol version it implements.
- The embed is a build product of a pinned tag + a transform, not a hand-curated
  copy — eliminating the verbatim-copy script leak and the scrub-then-redrift loop.
- Protocol and product reconverge: downstream-born rules flow back upstream; the
  pin only ever moves to a reviewed tag.
- New surface area: `internal/baton/` (vendor/transform/diff/version),
  `cmd/sworn/baton.go`, `docs/baton-governance.md`. Delivered by track
  **T14-baton-integration** (`depends_on T3-commercial` — S21 creates the embed
  this track vendors into).
- `S27-public-readiness-scrub` (T10) keeps its dogfood provenance-comment
  scrub; the *script-reference* portion is now produced by S48's transform, and
  T10 `depends_on T14` so the generated embed is in place before the final gate.
