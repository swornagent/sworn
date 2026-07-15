---
title: 'Release intake: local-first account safety'
description: 'Planning record for credential ownership, account isolation, removal of managed inference and credits, and explicit outbound consent.'
---

# Release Intake: `2026-07-15-local-first-account-safety`

## Release goal

Restore Sworn's local-first account boundary so a user can configure direct
provider credentials, log in and out of a Sworn account, and continue resolving
the same direct or subscription-backed model path without credential loss,
route changes, hidden spending, or unapproved outbound traffic. Remove the
dormant Sworn-managed inference proxy and credit-purchase/cache surfaces instead
of hardening them. Shipped means the real CLI proves, from a temporary home,
that account identity is control-plane-only, provider credentials survive every
account operation, ordinary delivery commands make zero Sworn-hosted requests
when outbound features are disabled, and existing direct/BYO and subscription
drivers remain usable.

## Source of truth

- **Human stakeholder**: repository owner / Coach
- **Tracking issue / epic**: [sworn#121](https://github.com/swornagent/sworn/issues/121)
- **Related captures**:
  - `docs/captures/2026-07-14-architecture-review-root-cause.md`
  - `docs/captures/2026-07-14-architecture-review-findings.md`
  - `docs/captures/2026-07-14-architecture-review-ratification.md`
  - `docs/release/2026-07-14-autonomous-operations/`
  - `docs/release/2026-07-14-local-cloud-providers/`
- **Related memory entries consulted**:
  - `project_architecture_review_commissioned`
  - `project_baton_engine_prereq_llm_checks`
  - `project_autonomous_loop_not_operational`
  - `project_opencore_decision`

## Users and their gestures

- **Local BYO operator**: configures a provider key, runs a model-backed command,
  logs in for account/control-plane identity, runs again through the same direct
  provider path, logs out, and finds the original provider key and route intact.
- **Subscription-driver operator**: uses a supported CLI subscription driver
  before, during and after account login without acquiring a provider key or
  being redirected through a Sworn-managed model service.
- **Account user**: runs `sworn login`, `sworn account` and `sworn logout` for
  identity/account status only; no account gesture selects, purchases or funds
  model inference.
- **Privacy-conscious operator**: leaves telemetry and every notification
  channel disabled and observes no DNS, HTTP, retry or background traffic from
  ordinary delivery commands to Sworn-hosted endpoints.
- **Webhook operator**: explicitly configures a generic webhook destination and
  receives only the minimal safe event projection; account presence alone sends
  no notification.
- **Maintainer/verifier**: runs one real temporary-home CLI journey proving the
  combined credential, routing, consent, payload and recovery contracts rather
  than relying on package fixtures alone.

## What's currently broken or missing

- Account and provider secrets historically had incompatible writers for one
  `credentials.json`. The current integration branch contains a field-preserving
  shared envelope that prevents simple sequential clobbering, but it has no
  explicit schema version, complete migration/rollback contract or old/new
  binary recovery proof.
- Account presence still selects the Sworn proxy for eligible in-process models
  unless `SWORN_DIRECT=1` is set. Authentication is therefore still an implicit
  inference-routing choice.
- `sworn account buy`, integer credit caches, TUI credit rendering and the proxy
  API contract expose a Sworn-managed inference/credit product that is not an
  approved product direction and has no live authoritative backend.
- A logged-in account still enables hosted email notification implicitly. The
  outbound event can contain free-form violation or error details without a
  separate notification/data-sharing choice.
- `SWORN_PROXY_URL` can redirect bearer-authenticated proxy, credit and hosted
  notification requests to an arbitrary host.
- The telemetry sender now enforces opt-in before identifier creation or network
  dispatch on the integration branch, but the obsolete consent prompt and the
  ratified preview/value-led invitation contract are not reconciled.
- Account login still hard-codes tier/expiry values rather than preserving one
  authoritative server contract. Credit consumers disagree on field names, but
  the chosen direction is removal of credit commerce rather than schema repair.

## What the human wants

- N-01: **Credential ownership and recovery.** Account login/logout and provider
  configuration have distinct, versioned lifecycles; corrupt or interrupted
  migration preserves recoverable source bytes and never loses a provider key.
- N-02: **Identity cannot route inference.** Login state has no effect on model
  resolution. Direct provider prefixes and subscription-driver prefixes remain
  the explicit route authority.
- N-03: **Remove Sworn-managed inference and credits.** The CLI, TUI, account
  state, model resolver and public API documentation expose no Sworn proxy,
  credit purchase, credit balance or automatic managed route.
- N-04: **Preserve customer-selected aggregators.** A provider such as OpenRouter
  remains usable through its ordinary direct/BYO provider prefix and the
  customer's own credential; removing Sworn-managed inference must not remove
  provider-neutral direct integrations.
- N-05: **Separate outbound consent.** Telemetry and each notification channel
  remain independently disabled until an affirmative, inspectable user action.
  Account presence is never consent.
- N-06: **Minimise notification payloads.** Generic webhook delivery uses a
  stable safe event projection without paths, source/spec/proof content,
  secrets, free-form errors or violation details; local diagnostics retain the
  useful detail.
- N-07: **Prove zero unintended network traffic.** A real CLI journey with
  telemetry and notifications disabled observes zero Sworn-hosted network
  attempts across success, error and early-exit paths.
- N-08: **Truthful account surface.** Account commands preserve authoritative
  identity state and describe only capabilities that exist; expired, absent,
  corrupt and partial state fail safely without disabling local operation.
- N-09: **Public-safe recovery guidance.** CLI diagnostics and public technical
  documentation describe credential locations, migration recovery, outbound
  defaults and the removal of obsolete proxy/credit behaviour without private
  commercial rationale.

## Constraints and non-negotiables

- Planning and implementation target `release/v0.2.0`; `main` remains production.
- This planning session writes only this release's artefacts. It does not modify
  either adjacent July 14 release or any production code.
- Sworn remains one native Go binary with no required external runtime. Direct
  third-party modules remain governed by accepted owning ADRs.
- Fail closed: ambiguous credentials, routes, consent, account state or migration
  cannot cause a merge, credential deletion, provider switch or outbound send.
- Account identity is for present/future account and control-plane capability
  only. It never authorises inference routing, telemetry or notification.
- Supported model execution after this release is direct/BYO or an explicit
  subscription CLI driver. Customer-owned aggregator keys are direct/BYO.
- No API key, account token, request body, model payload, path, proof/spec text or
  free-form violation/error detail may enter logs or outbound safe projections.
- Provider configuration remains compatible with the canonical driver registry;
  this release must not build a second routing engine.
- Generic webhook failure never changes verdict/gate semantics or blocks the
  local delivery loop.
- Public artefacts contain only public-safe technical requirements. No private
  strategy, pricing, provider-negotiation or customer material enters this repo.
- Every user-facing removal/change requires a real `sworn` command reachability
  test; package-only tests are insufficient.

## Adjacent / out of scope

- **Autonomous engine truth, durable paging and the mobile WebUI**: remain owned
  by `docs/release/2026-07-14-autonomous-operations/` and sworn#109. **Why
  deferred**: this release establishes the safe account/consent contracts those
  surfaces consume and must not duplicate their command/event/outbox engine.
  **Tracking**: sworn#109 and its release board. **Acknowledged**: Coach,
  2026-07-15.
- **Local/cloud endpoint and dialect expansion**: remains owned by
  `docs/release/2026-07-14-local-cloud-providers/`. **Why deferred**: provider
  endpoint capability is independent of removing account-driven routing and its
  planned files already overlap the model/driver surface. **Tracking**: that
  release board. **Acknowledged**: Coach, 2026-07-15.
- **Hosted control-plane backend, tenancy, relay and deployment**: not built in
  this release. **Why deferred**: the release restores local safety before any
  hosted expansion; hosted architecture has separate identity, tenancy and
  operations gates. **Tracking**: sworn#109 owns the transport-independent local
  foundation; a separate hosted release must be planned before deployment.
  **Acknowledged**: Coach, 2026-07-15.
- **Partner-managed inference or reseller integration**: not implemented or
  preserved as a dormant production route. **Why deferred**: no partner contract
  or approved technical/data boundary exists, and Sworn-managed inference was
  explicitly rejected for this release. **Tracking**: requires a new ratified
  decision and dedicated future release before any implementation. **Acknowledged**:
  Coach, 2026-07-15.
- **Native mobile applications and privileged hosted commands**: excluded.
  **Why deferred**: the ratified sequence is responsive read-only monitoring and
  alerts before remote mutation, and both belong to the operations release.
  **Tracking**: sworn#109 S10/S11. **Acknowledged**: Coach, 2026-07-15.

## Decisions made during planning

### 2026-07-15 — Ratify the release goal

- **Proposed goal**: make SwornAgent accounts identity-only so signing in or out
  cannot alter provider credentials or model routing, expose managed inference
  or credit-purchase surfaces, or cause unapproved outbound traffic; preserve
  direct BYO-key and subscription-driver execution.
- **Decision**: ratified as written by the Coach.
- **Why**: it gives the release one user-visible safety promise and makes the
  account, routing, commerce-removal and outbound-consent work testable as one
  end-to-end contract.

### 2026-07-15 — Split credential storage by ownership

- **Context**: the current field-preserving `credentials.json` envelope limits
  sequential clobbering but still couples provider keys, account sessions and
  notification configuration to one corruption and lifecycle boundary.
- **Options considered**: keep one versioned composite owned by one package;
  preserve `credentials.json` for provider keys and introduce a separately
  versioned `account.json`; or introduce a general encrypted state database.
- **Decision**: preserve `credentials.json` as the provider-key record and move
  the login session into a separately versioned `account.json`. Notification
  configuration belongs to neither record and will be decided separately.
- **Migration contract**: copy account fields from the legacy composite, verify
  the new record, then clean the legacy fields while retaining recoverable source
  bytes. An interrupted or corrupt migration cannot delete provider credentials.
- **Rollback contract**: an older binary continues to find provider keys in
  `credentials.json` but sees no account session after migration, which is the
  safe failure mode.
- **Why**: the identity-only account promise becomes a structural ownership
  boundary rather than a convention every future writer must remember.

### 2026-07-15 — Keep account status authoritative and identity-only

- **Context**: the device-token response already carries tier and expiry data,
  but the CLI discards it, manufactures a `free` tier and 24-hour expiry, then
  mixes credit commerce and notification configuration into account status.
- **Options considered**: show only identity and expiry; show identity plus
  optional server-authored plan/tier and expiry; or remove `sworn account` and
  retain only login/logout.
- **Decision**: retain `sworn account` as an identity/session status command. It
  shows login state and email, plus plan/tier and expiry only when supplied by
  the auth server. It never manufactures account values or claims model access,
  inference funding, notification consent or delivery state.
- **State behaviour**: absent and expired sessions are successful status queries
  with clear login guidance; corrupt or unsupported records return non-zero with
  secret-safe recovery guidance. Credits, purchasing, webhook configuration and
  notification status leave the account command.
- **Why**: this preserves a useful hosted control-plane seam without coupling
  account identity to model execution or presenting invented product state.

### 2026-07-15 — Remove `SWORN_DIRECT` immediately

- **Context**: `SWORN_DIRECT=1` exists only to bypass the Sworn-managed proxy.
  Once that route is removed, direct/BYO and subscription-driver resolution is
  unconditional and the variable has no remaining behaviour to select.
- **Options considered**: remove all active recognition in this release; retain
  a one-release no-op with a `sworn doctor` warning; or repurpose the name for a
  future routing concern.
- **Decision**: remove the runtime reads, routing branches, active help text and
  current documentation now. Do not add a recognized no-op or replacement flag.
  Historical release artefacts remain unchanged.
- **Compatibility**: users who already set the variable continue to use the same
  direct route because direct execution is now invariant. Current release notes
  explain that stale shell configuration may be deleted.
- **Why**: preserving dead configuration would imply that account-driven or
  managed routing still exists and would weaken the explicit-prefix authority.

### 2026-07-15 — Migrate account state automatically and recoverably

- **Context**: splitting the shared envelope can be interrupted after creating
  `account.json` but before removing account fields from `credentials.json`, or
  can encounter conflicting, corrupt or unsupported records.
- **Options considered**: automatic idempotent migration when an account-aware
  command opens the store; require `sworn doctor --fix`; or read both layouts
  indefinitely.
- **Decision**: account-aware commands use one automatic idempotent migration
  routine. Provider-only commands never open or migrate account state. The
  routine validates the legacy record, writes and syncs versioned `account.json`,
  re-reads and verifies it, then atomically removes only account fields from the
  provider record.
- **Recovery contract**: temporary recovery bytes exist only while migration is
  incomplete and must not become a permanent token-bearing backup. Every
  interrupted stage converges safely when retried. Conflicting old/new records,
  unsupported versions or incomplete recovery leave both records untouched and
  return non-zero with secret-safe `sworn doctor` guidance; the doctor fix path
  calls the same migration routine.
- **Why**: this is turnkey for ordinary upgrades without allowing convenience to
  outrank durable verification or secret hygiene.

### 2026-07-15 — Complete the ratified telemetry consent experience

- **Context**: outbound dispatch and install-ID creation now gate on opt-in, but
  the CLI still has a pre-value startup disclosure and dead default-yes consent
  function while the ratified exact preview and success-triggered invitation are
  absent.
- **Options considered**: complete the full ratified contract and resolve #118;
  limit this release to transport safety; or remove outbound telemetry entirely.
- **Decision**: complete the full contract in this release. Telemetry remains off
  until `sworn telemetry on`; `sworn init` may state that it is disabled but does
  not ask for consent; the obsolete prompt and pre-value disclosure are removed;
  and `sworn telemetry preview` shows the exact representative event without an
  install ID or network request.
- **Invitation contract**: after the first successful PASS or verified delivery
  gesture, an interactive TTY may show one non-blocking value-led invitation.
  Dismissal leaves telemetry off and suppresses future invitations. It never
  enters JSON, piped, CI, MCP or other non-interactive output. Persistent opt-out
  and `SWORN_NO_TELEMETRY=1` continue to dominate stored opt-in.
- **Tracking**: this release absorbs the reachable implementation and
  reconciliation scope of [sworn#118](https://github.com/swornagent/sworn/issues/118).
- **Why**: consent must be coherent from invitation through preview, explicit
  enablement and transport; another partial repair would retain a known trust
  contradiction.

### 2026-07-15 — Give notifications an independent configuration record

- **Context**: the planned operations release preserves notification secrets in
  the shared credential envelope and retains `sworn account set-webhook`, which
  conflicts with the ratified identity-only account boundary and independent
  outbound consent.
- **Options considered**: a separate versioned `notifications.json`; a section in
  general `config.json`; or environment variables only.
- **Decision**: create a `0600`, versioned `notifications.json` owned only by
  notification configuration. `sworn notifications webhook set <url>` is the
  affirmative enable gesture; `off` retains configuration while disabling it,
  `remove` deletes it, `status` redacts secret-bearing destination components,
  and `preview` renders the exact safe payload without sending.
- **Migration and delivery contract**: an existing `webhook_url` migrates enabled
  only after validation because it originated in an explicit set gesture; an
  invalid value leaves the source untouched. Account presence never enables
  email or another channel. The operations database stores only an opaque
  destination reference, and any reachable pre-outbox sender must consume the
  same allowlisted projection and bounded HTTP policy rather than the legacy
  payload.
- **Tracking**: durable enqueue, retry, replay and mobile delivery remain with
  sworn#109. Its planned S08 slice must be replanned to consume this record and
  command surface rather than the shared credential envelope.
- **Why**: notification destinations are neither identity nor provider
  credentials and require an independently inspectable consent lifecycle.

### 2026-07-15 — Retain only a scoped login-issuer override

- **Context**: `SWORN_PROXY_URL` redirects already-issued bearer credentials
  across proxy, credit and hosted-notification paths. `SWORN_AUTH_URL` instead
  selects the issuer for a new device-code login and is also the built-binary
  loopback test seam.
- **Options considered**: retain a tightly scoped issuer selector; allow only the
  compiled official issuer; or introduce a general configurable API base URL.
- **Decision**: retain `SWORN_AUTH_URL` only for `sworn login`. It selects the
  issuer for that new flow and never receives an existing bearer token. Store the
  canonical issuer with the resulting token in `account.json`; any future
  authenticated control-plane request may return the token only to that bound
  issuer. Remove `SWORN_PROXY_URL` entirely and add no general API-host override.
- **URL and test contract**: accept absolute HTTPS issuers, plus HTTP only on
  loopback for test or self-host use; reject userinfo, query, fragment, malformed
  host and unsafe redirects. State a non-default issuer before starting login.
  Package tests inject HTTP directly while real-binary reachability may use the
  loopback issuer selector.
- **Why**: issuer selection preserves self-hosting and reachability without
  recreating an environment-controlled bearer-token exfiltration path.

### 2026-07-15 — Name the release for its user promise

- **Context**: the private handoff used “trust-contract safety”, which is accurate
  internal language but does not describe the shipped outcome clearly.
- **Options considered**: `2026-07-15-local-first-account-safety`,
  `2026-07-15-identity-routing-and-consent`, and
  `2026-07-15-local-first-boundary-repair`.
- **Decision**: use `2026-07-15-local-first-account-safety`.
- **Why**: it names the user promise and stays accurate when unsafe dormant
  product surfaces are removed rather than hardened.

### 2026-07-15 — Remove managed inference and credit surfaces

- **Context**: the existing client contains dormant proxy routing, account credit
  purchase/cache/UI and a stub proxy contract even though operating inference is
  outside Sworn's chosen product boundary.
- **Options considered**: harden a Sworn-managed route, preserve it disabled for
  later, or remove it and require a separately ratified partner integration if
  the direction changes.
- **Decision**: remove the Sworn-managed proxy and credit surfaces. Account
  identity becomes control-plane-only; supported execution remains direct/BYO
  and subscription drivers.
- **Why**: proxying, upstream rights, financial metering and inference support are
  not Sworn's core competency. Dormant unsafe code is still a trust liability.

## Schema-vs-spec audit notes

- The current credentials file is an unversioned composite JSON envelope with
  provider keys under `providers` and account/notification fields at the top
  level. The simple shared writer is containment, not a complete ownership or
  migration contract.
- Account login currently constructs tier and expiry locally despite the token
  response carrying server fields; exact response/cache ownership must be
  reconciled before specs are emitted.
- Credit cache consumers disagree (`credits` response/cache versus TUI
  `balance`). Because credits are removal scope, no new canonical credit schema
  will be invented.
- The model registry's provider prefixes are explicit routing authority. A direct
  `openrouter/` prefix with the customer's key is not the removed Sworn proxy.
- The autonomous-operations release owns future command/event/outbox records.
  This release may establish consent/config contracts but must not create a
  competing durable operations schema.

## Proposed slice decomposition (draft)

Decomposition is intentionally pending structured discovery. The private
handoff's A1/A2/B1/B2/C1/C2/D1 proposal must be re-cut because managed routing,
credits and hosted email are now removal scope.

## Ambiguity register

| # | Ambiguity | Affects | Resolution |
|---|---|---|---|
| A-01 | Whether account and provider credentials use separate versioned files or one versioned composite owned by one package | N-01, N-08 | **Resolved**: provider-owned `credentials.json` plus session-owned `account.json`; copy-verify-clean migration with recoverable source bytes |
| A-02 | Exact retained `sworn account` fields and behaviour after credit removal | N-03, N-08 | **Resolved**: authoritative identity/session status; optional server-authored plan and expiry; no commerce, models or notification state |
| A-03 | Whether obsolete `SWORN_DIRECT` is removed immediately or retained as a one-release no-op deprecation | N-02, N-03, N-09 | **Resolved**: remove active recognition now; direct routing becomes invariant and no replacement flag is introduced |
| A-04 | Migration and rollback behaviour when an older Sworn binary encounters the new credential layout | N-01, N-09 | **Resolved**: automatic idempotent account-command migration; verify before cleanup; temporary recovery only; conflicts fail closed; old binaries retain providers and appear logged out |
| A-05 | Whether this release includes the complete ratified telemetry preview/invitation UX or only safety-boundary reconciliation | N-05, N-07 | **Resolved**: full ratified preview, explicit opt-in, value-led one-time invitation and output suppression; absorb #118 |
| A-06 | Exact generic webhook consent/config storage before the autonomous release supplies the durable outbox | N-05, N-06 | **Resolved**: versioned `notifications.json`; explicit top-level webhook gestures; redacted status and no-send preview; #109 outbox consumes opaque references |
| A-07 | Which account/auth endpoint override seam remains for tests after all ordinary bearer-token redirection is removed | N-07, N-08 | **Resolved**: retain scoped login-only issuer selection and bind tokens to canonical issuer; remove all general bearer-host overrides |

## Screenshots / references

- No screenshots supplied. This release is driven by executable CLI/network
  journeys and persisted-state evidence.
