# Journal — S48-baton-vendor

## 2026-07-07 — Session 1: Design TL;DR

- Materialised track worktree for T14-baton-integration (new track, depends_on T3-commercial + T15-cli-registry, both merged).
- Produced Design TL;DR — see `design.md`.
- **Planned_files discrepancy**: `status.json` `planned_files` includes `cmd/sworn/main.go` but the spec explicitly says "Does NOT edit cmd/sworn/main.go — that file is owned solely by T15-cli-registry." The S51/T15 command registry means `baton` self-registers from `cmd/sworn/baton.go`'s own `init()`, not by editing `main.go`. This is a planner artefact and will be routed through design review.
- **Network fetch deferred**: S48 MVP reads from a local filesystem path. Network fetch of a Baton tag is deferred to S49 (pin reconciliation) or a future enhancement — will surface a hook in `source.go`.
- State transition: `planned` → `design_review`.
## 2026-07-07 — Session 2: Pre-implementation pins (Coach-approved)

- **Pin 1**: Removed `cmd/sworn/main.go` from `status.json` `planned_files` — spec + design both state main.go is NOT touched; S30 touchpoint linter (Gate 2) would fail.
- **Pin 2**: Filed GitHub issue #11 — "sworn baton vendor: network fetch support for Baton semver tag" — the correct Rule 2 tracking home for the network-fetch deferral. Updated design.md §4 tracking reference from `S49-baton-version` to `GitHub issue #11`.
- Pin 3 (memory-cited): Ack confirmed — design aligns with [[project_baton_sworn_architecture]]; no action.
- Coach flags noted for later: (a) populate `design_decisions` in status.json before transitioning to implemented; (b) forward-handoff comment in baton.go for S50's `sworn baton diff`.
