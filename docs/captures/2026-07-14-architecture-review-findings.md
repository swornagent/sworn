# Architecture review: residual decisions

Date: 2026-07-14  
Review: [#108](https://github.com/swornagent/sworn/issues/108)  
Remediation epic: [#109](https://github.com/swornagent/sworn/issues/109)

This document intentionally contains only findings that cannot be settled by a
mechanical guard. The review's code defects, false-green gates, and ownership
violations are captured as tests, architecture rules, fixes, issues, and planned
slices; repeating them here would create another prose source of truth. Their
live disposition and fresh-context evidence are in the review proof bundle.

## 1. Where autonomous authority ends

The native `merge-release` command currently runs readiness gates, prints a
success message, and exits 0 without moving the integration branch. A separate
human-routed path performs the merge but bypasses those native gates (#53).
Independent refutation confirmed this is a high product/terminal-semantics gap,
not a critical merge-safety bypass: the scheduler pauses for a human, and the
earlier slice that introduced the command deliberately excluded the final
mutation.

The mechanical part is guardable: one native operation can own both the gates
and the `release-wt → integration` mutation, and an end-to-end test can require
the target ref to move before exit 0. The unguardable part is constitutional:
whether an unattended loop is allowed to invoke that operation.

The drafted release uses this safe default:

- autonomous through verified release assembly;
- `ready_to_merge` remains distinct from merge success;
- a human invokes the native gated merge by default; and
- automatic integration merge requires a separate durable standing delegation
  scoped to release and target, with grant, expiry, and revocation.

This must be reviewed as a Type-1 decision in
`S12-autonomous-operations-journey`. Design-authority delegation is not release
authority and cannot be reused implicitly.

## 2. What “single binary, minimal dependencies” means

The repository has contradictory accepted instructions. `AGENTS.md` states an
absolute standard-library/zero-dependency rule; ADR-0007 supersedes that rule
with “standard library preferred, each direct runtime module requires an ADR.”
The module graph and accepted SQLite, TUI, and provider ADRs follow the latter.

The review narrowed the concrete drift to two direct modules:

- the YAML module is used by one TUI parser even though the parsed value is not
  used in rendering and should be removed; and
- the JSON Schema validator implements ADR-0011's central decision but is not
  named or evaluated by that ADR.

A registry/lint can enforce whichever policy is selected, but it cannot choose
the policy. The recommended ratification is:

> Sworn ships as one native binary with no required external runtime
> installation. Standard library is preferred. Every direct module must map to
> an accepted ADR record naming and justifying it.

Issue [#117](https://github.com/swornagent/sworn/issues/117) owns the decision,
instruction alignment, YAML removal, JSON Schema ratification, and the
direct-module-to-ADR guard.

## 3. Which gesture constitutes telemetry consent

The high privacy defect was mechanical and has been contained: telemetry now
checks `IsEnabled` before creating an install ID or launching a request, and
neutral, file-opt-out, and environment-opt-out mutations produce zero requests.

The residual contract decision is user experience. `ShowConsent` exists only in
tests, while `sworn telemetry on` is the real explicit opt-in gesture. Historical
S26 records say consent occurs during `sworn init`.

Either contract can be safe:

- wire the interactive consent prompt into first-run init and persist opt-out
  for non-interactive `--yes`; or
- ratify `sworn telemetry on` as the only opt-in, remove the dead prompt, and
  amend S26/docs.

The neutral default and explicit opt-out dominance are non-negotiable in either
case. Issue [#118](https://github.com/swornagent/sworn/issues/118) owns the
choice and its real-CLI reachability proof.

## 4. Which remote deployment model the mobile board supports

The requested mobile board is valuable precisely when the operator is away from
the development machine. That creates a product/security choice that cannot be
answered by responsive CSS or handler tests: which network and identity boundary
Sworn promises to support.

The drafted release deliberately supports a narrow first contract:

- the embedded server binds to loopback by default;
- read-only mobile monitoring ships before mutation;
- non-loopback bind is rejected until explicit TLS, authentication, and exact
  origin configuration validate;
- browser mutation additionally requires a bounded session, CSRF token,
  `If-Match` state revision, idempotency key, authorization, and durable audit;
  and
- notifications link to that authenticated board rather than embedding a second
  mutation protocol.

Those constraints are mechanically testable. The authentication mechanism,
credential lifecycle, and supported deployment topology remain Type-1 design
pins for `S09-operations-read-api` and
`S11-authenticated-remote-controls`. The release must not imply hosted,
multi-tenant, or internet-exposed operation unless a later decision explicitly
adds those threat models.

## Decision checkpoint

The remediation release is intentionally drafted with requirements-validation
records not yet marked human-ratified. Implementation should begin only after
these four decisions—and the affected slice designs—are reviewed. That is a
visible gate, not a silent deferral.
