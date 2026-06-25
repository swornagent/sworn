# Baton protocol governance — the PR-up workflow

The SwornAgent binary embeds the Baton protocol via a vendor-down pipeline
(`sworn baton vendor`, see [ADR-0006](adr/0006-baton-protocol-sync.md)). This
document describes the **reverse flow**: what to do when a protocol change is
discovered during sworn development.

## The rule

**Never edit the embedded protocol directly.** The embed under `internal/adopt/baton/`
and `internal/prompt/` is a build product of (pinned tag + transform). Hand-editing
an embedded rule or prompt is a **silent fork** of an open protocol. Any protocol
change discovered during sworn development must flow back upstream.

## The PR-up workflow

1. **Open a PR** against [github.com/sawy3r/baton](https://github.com/sawy3r/baton)
   with the proposed protocol change (new rule, wording fix, restructuring). The
   Baton repo is the canonical home of the protocol.

2. **Review, authorise, merge.** The PR is reviewed by the Baton maintainer(s),
   discussed if needed, and merged. A new semver tag is created on the merge
   commit (e.g. `v0.4.3`).

3. **Re-vendor.** In the SwornAgent repo, run `sworn baton vendor <source-dir>`
   with the updated Baton checkout. This re-applies the transform and writes the
   revised protocol into the embed. Commit the result.

4. **Verify.** Run `sworn baton diff <source-dir>` — it must exit 0 (clean). This
   command is the fail-closed governance gate: it compares the committed embed
   against the transformed pinned source and exits non-zero if the embed has been
   edited out-of-band. Wire it into CI so no silent fork can merge.

## Tracking

- **Open protocol changes:** [sawy3r/baton#31](https://github.com/sawy3r/baton/issues/31)
  tracks the fidelity-layer rules (08/09/10) born in sworn that need upstream PRs
  and the VERSION/tag-discipline ask.
- **CI wiring:** a future slice (S27-public-readiness-scrub, or a dedicated
  CI-config slice) will add `sworn baton diff` to the CI workflow. Until then,
  run it manually before merging protocol changes.

## Related documents

- [ADR-0006 — Baton↔SwornAgent protocol sync](adr/0006-baton-protocol-sync.md) —
  the architectural decision record that establishes the vendor-down + PR-up
  governance model.
- `internal/baton/` — the vendor/transform/diff/version pipeline.
- `sworn baton diff --help` — the divergence detector's usage.
- `sworn baton vendor --help` — the vendor-down pipeline's usage.