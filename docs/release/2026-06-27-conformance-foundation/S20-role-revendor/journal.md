# Journal: S20-role-revendor

## Session 2026-06-28 — Initial implementation

### Decisions

- **Re-vendored from canonical source**: Copied `planner.md`, `implementer.md`, `captain.md` from `$HOME/.claude/baton/role-prompts/` (post-records-as-JSON canonical). 
- **Public-safety scrub**: `implementer.md` contained one `[[feedback_materialise_newline_eats_next_track_entry]]` reference banned by `TestEmbeddedPromptsPublicSafe`. Replaced with "a known issue with freehand multi-line replacement". This is a single-line deviation from canonical required by public-repo safety. `planner.md` and `captain.md` required no scrubbing.
- **VERSION.txt**: Created with S22 pin SHA `42eb48b`. Note: S23 removed `internal/prompt/VERSION.txt` as dead (version centralized to `internal/adopt/baton/VERSION`), but the spec explicitly requires it. Created anyway per spec contract. This file is not embedded by `prompt.go` (the go:embed directive was removed in S23).
- **No changes to `design-reviewer.md`**: Out of scope per spec; handled by S19.

### Trade-offs

- The `implementer.md` diff AC (byte-for-byte identical to canonical) cannot pass simultaneously with the public-safety test (`TestEmbeddedPromptsPublicSafe`). The canonical source contains internal wiki references that must be scrubbed for public consumption. The single-line deviation is the minimum scrub needed.
- VERSION.txt is a dead file (not embedded, removed by S23) but created per spec. If this is a spec defect, the verifier will flag it.

### Deferrals

None. All spec scope satisfied.