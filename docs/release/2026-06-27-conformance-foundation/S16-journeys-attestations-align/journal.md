# Journal — S16-journeys-attestations-align

## Session 2026-07-25 — Implementation

### State transition: planned → in_progress → implemented

**Decisions:**

1. **Nested Ratification struct**: Replaced flat `RatifiedAt`/`RatifiedBy`/`IsRatified` on `JourneyArtefact` with a nested `Ratification` sub-struct (`{By, At, IsRatified}`). This matches the canonical journeys-v1 schema shape. Same pattern applied to `AttestationArtefact`.

2. **Boundary struct**: Added `Boundary` sub-struct (`{Name, MockBanned, EntitlementBoundary}`) to `AttestationArtefact` matching the canonical attestations-v1 shape.

3. **Canonical schemas sourced from spec**: The canonical schemas at `~/.claude/baton/schemas/` do not exist. Schemas were designed from the spec's description of required nested shapes and the existing embed/validator patterns from S13-S15.

4. **Unratified artefact validation**: The validator allows empty `by`/`at` fields when `is_ratified` is false. This is necessary because `NewArtefact()`, `AddJourney()`, and `DraftTemplate()` all produce unratified artefacts that must be saveable. When `is_ratified` is true, `by` and `at` must be non-empty.

5. **$schema auto-population**: Both `SaveArtefact` and `SaveAttestations` auto-set `$schema` if empty before marshalling, ensuring all written artefacts carry the canonical URI.

6. **No migration concern**: Spec risk noted existing `.sworn/journeys.json` files might fail to parse. Since none exist in this repo, no migration code was written.

**Trade-offs:**

- The `Attestation` struct's `WalkedBy` and `WalkedAt` fields lost their `omitempty` tags because the attestations-v1 schema requires them. This is safe — attestations are always written with these fields populated.
- The validator uses structural required-fields checks rather than full JSON Schema validation (consistent with S13's approach). Full validation is deferred to ADR-0007.

**Subagent dispatches:** None.