# Architecture review: decision ratification

Date: 2026-07-14

Review: [#108](https://github.com/swornagent/sworn/issues/108)

Remediation epic: [#109](https://github.com/swornagent/sworn/issues/109)

The repository owner ratified the four release-level Type-1 decisions below in
an interactive review. This record is deliberately narrower than blanket
approval of the twelve remediation slice designs. Slice-specific choices that
were not discussed retain an empty `human_decision` and must still pass their
own design review.

## D1 — Human default with scoped standing delegation

The autonomous loop ends at a durable `ready_to_merge` result by default.
Assembly-to-integration merge may run unattended only when an authorized person
has recorded a standing delegation that names the repository/release, exact
target branch, permitted operation, required gates, grantor, creation time,
expiry, and revocation state.

The normal gates run again immediately before the mutation. Missing, ambiguous,
expired, revoked, or target-mismatched delegation fails closed to
`ready_to_merge`. Promotion from the release integration branch to production
`main` remains a separate human-authorized operation for the first version.

## D2 — Native distribution with governed direct modules

SwornAgent ships as one native binary with no required external runtime
installation. The Go standard library is preferred. Every direct third-party
module requires an accepted ADR that names, justifies, and owns it.

The owning record must explain why the standard library is insufficient,
security and maintenance implications, and replacement or removal conditions.
CI must reject an unregistered direct module. Transitive modules remain visible
and are attributed to their owning direct dependency. Official SDK status alone
is not approval.

Issue [#117](https://github.com/swornagent/sworn/issues/117) owns instruction
alignment, removal of the unused YAML module, explicit JSON Schema validator
ratification, and the module-to-ADR enforcement registry.

## D3 — Explicit telemetry opt-in with a value-led invitation

Telemetry remains disabled until the user runs `sworn telemetry on`. `sworn
init` may disclose that telemetry is off but must not enable it or present a
default-yes consent prompt. No install identifier is created before opt-in, and
the persistent opt-out plus `SWORN_NO_TELEMETRY=1` continue to dominate stored
opt-in.

After the first meaningful success, an interactive terminal may show one
non-blocking invitation explaining how anonymous reliability data improves
command stability and platform compatibility. The invitation appears once,
never enters JSON/piped/CI/MCP output, creates no remote identifier, and does not
gate functionality. `sworn telemetry preview` shows the exact representative
payload before consent. Release communication should periodically identify
improvements made from the collected signal so the value proposition remains
truthful.

Issue [#118](https://github.com/swornagent/sworn/issues/118) owns the reachable
CLI implementation, removal of the dead consent prompt, preview/invitation
semantics, and documentation reconciliation.

## D4 — Self-hosted foundation plus optional hosted control plane

Sworn supports secure self-hosted operation and an optional hosted control
plane. The local Sworn agent remains the final authority for repository and
worktree access, verification, runtime state transitions, command authorization,
and merge-policy enforcement.

Hosted remote commands travel over an authenticated outbound agent connection.
They are typed, signed, revision-checked, idempotent, and re-authorized against
local policy before any effect. The hosted service never directly edits Git or
Baton records. Public-safe operation events carry the result back to monitoring,
notification, and audit projections.

The hosted service is the intended primary remote/mobile, team, fleet,
notification, and audit experience. Direct secure self-hosting and offline
operation remain supported. The current #109 release delivers the local
command/event authority, secure self-hosted surface, and transport-independent
contracts; hosted tenancy, identity, relay, deployment, and model-access details
require a separate architecture review and are not silently added to these
slices.

## Remaining design gates

This ratification settles authority boundaries and product direction. It does
not settle storage schemas, REST/SSE selection, session construction, TLS
certificate lifecycle, browser implementation, process-group mechanics,
canonicalization bytes, or other detailed choices recorded in individual
`status.json` files. Requirements validation and design-fit remain fail-closed
until those slice-level records are reviewed at their normal Baton gates.
