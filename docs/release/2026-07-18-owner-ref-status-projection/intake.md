---
title: 'Release intake: owner-ref status projection'
description: 'Technical release record for issue #124 owner-track board authority.'
---

# Release intake: owner-ref status projection

## Release goal

Restore the established owner-track authority rule inside ref-aware board
discovery. An operator must see the status committed on a slice's owning track,
not a farther ghost copy or launch-working-tree file, including when the logical
`docs` path is a symlink and release records live at the canonical Git tree path.

## Needs

- N-01: Board discovery resolves every topology-owned slice from its exact
  owner-track ref, with the selected release ref as the committed fallback.
- N-02: Canonical release records behind a logical documentation symlink are
  read through Git tree paths rather than filesystem traversal.
- N-03: Aggregate and named board commands expose selected topology source,
  slice state source, and committed durability consistently.
- N-04: Ghost refs and launch working-tree bytes cannot change Git-backed board
  state.

## Source of truth

- Tracking issue: [swornagent/sworn#124](https://github.com/swornagent/sworn/issues/124)
- Related discovery issue: [swornagent/sworn#123](https://github.com/swornagent/sworn/issues/123)
- Established owner-state issue: [swornagent/sworn#81](https://github.com/swornagent/sworn/issues/81)

## Constraints

- Native Go and standard library only.
- Discovery stays read-only.
- Existing topology-source ranking and canonical-skew behaviour stay intact.
- No Baton vendoring, version bump, tag, publication, or release merge.

## Approved decomposition

One slice owns the bounded board-package and CLI correction because the source
selection, provenance projection, and integration fixture form one rollback and
verification unit.
