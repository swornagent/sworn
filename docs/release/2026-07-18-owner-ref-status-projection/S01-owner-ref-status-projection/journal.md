# S01 owner-ref status projection journal

## 2026-07-18 - implementation and protocol recovery

- Anchored to swornagent/sworn#124 and related authority contracts #123 and #81.
- Recorded the failing public CLI fixture in commit `fd3cf540`. Before the fix,
  the projected winner was `shipped`, sourced from `working-tree`, and marked
  `uncommitted` instead of the committed blocked owner verdict.
- Landed the owner-ref, canonical-prefix, and named-source projection in commit
  `60ff1e59`.
- The release artefacts were reconstructed after those implementation commits at
  the orchestrator's request. The status `start_commit` deliberately remains the
  integration base so verification covers both checkpoints and does not erase
  their history.
- Targeted CLI and board tests, the full Go suite, and vet passed. A built feature
  binary was also run from a separate live consumer project checkout. It reported
  the selected release source and the blocked slice verdict from the exact owner
  track with committed durability. Consumer identifiers and content are omitted
  because this repository is public.
- No Baton vendoring, version bump, tag, publication, or merge was performed.
- The implementer will leave the slice at `implemented` with verification
  pending. A fresh verifier must certify or reject it.
