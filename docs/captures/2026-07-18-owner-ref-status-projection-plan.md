# Owner-ref status projection plan

## Problem

The ref-aware catalog introduced for swornagent/sworn#123 discovers release
topology across refs, but its status election scans every status copy plus the
launch working tree. That conflicts with the established swornagent/sworn#81
ownership rule: a slice's authoritative committed state is the copy on its
owning `track/<release>/<track>` ref, with the selected release ref as fallback.

The election also constructs only `docs/release/...` status paths. In projects
where logical `docs` is a symlink and Git stores release records at
`apps/docs/content/docs/release/...`, topology is discovered but owner status is
not projected.

## Plan

1. Add CLI integration coverage using a committed `docs` symlink, canonical
   release records, divergent owner and ghost status copies, and dirty launch
   working-tree content.
2. Resolve each topology-owned slice from its exact local owner-track ref, then
   from the selected release ref when the owner ref has no valid status.
3. Probe both canonical release-doc prefixes in Git objects. Do not admit launch
   working-tree files into Git-backed catalog state.
4. Preserve per-slice `stateSource` and `stateDurability`, and expose the selected
   topology `sourceRef` in both aggregate and named JSON output.
5. Run focused CLI tests, the full Go suite, vet, and an actual built-binary
   command from a fixture repository.

## Boundaries

This change does not vendor Baton, change protocol schemas, bump a version,
publish, tag, or merge a release.
