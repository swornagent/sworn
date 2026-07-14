# Architecture review proof

Date: 2026-07-14

Review: [#108](https://github.com/swornagent/sworn/issues/108)

Remediation epic: [#109](https://github.com/swornagent/sworn/issues/109)

## Scope

This proof was generated from live repository state after the full-repository
review. The review ran in the isolated worktree branch
`review/architecture-2026-07-14`, cut from
`b6e50df166208ce743def84a8443b0aa8835a995`. The primary `main` worktree was not
modified or rebased; it advanced independently while the review retained the
exact snapshot it was asked to assess.

The review covered all 50 Go packages and used independent agents for eight
lenses:

1. single sources of truth and duplicated policy;
2. fail-closed behavior and optimistic terminal outcomes;
3. layer boundaries and ownership;
4. historical autonomous-loop, board, and notification operations;
5. cohesion, effect ordering, and terminal-state seams;
6. dead code, split production paths, and reachability;
7. error handling, process-global state, cancellation, and child processes; and
8. public-contract, dependency-policy, and documentation drift.

Each consolidated high-severity candidate was then handed to a separate
fresh-context refutation pass at the exact base commit. Broad allegations were
narrowed or rejected rather than carried forward as findings.

## Historical operational comparison

The historical harness was operationally ahead of native Sworn in several
important ways: detached workers, liveness manifests, restart and manual-kick
semantics, append-only events and worker logs, track-local decision parking, a
responsive read-only web board, generic webhook delivery, and phone-oriented
notifications with exact next actions. It was not a sound architecture to port:
some recovery behavior could checkpoint dirty state or auto-author a decision,
delivery was not durable, and remote authorization was too weak.

Current Sworn and Baton are materially stronger in protocol semantics,
fail-closed design authority, native worktree isolation, provider-neutral role
dispatch, and typed verification records. The missing piece is the assembled
operations plane. The review therefore preserves the useful operator experience
but makes one durable command/event core the prerequisite for CLI, TUI, MCP,
web, and notification adapters.

The historical loop also stopped at verified release assembly and paged a human
for final merge. The remediation release preserves that boundary by default and
offers automatic integration merge only as a separately ratified standing-
delegation design.

## Fresh refutation results

| Candidate | Refutation result | Durable disposition |
|---|---|---|
| Shared credential writers can destroy one another's fields | **SURVIVED — high.** A real provider → login → webhook → provider → logout sequence reproduced whole-file clobbering. | One field-preserving, atomic credential authority and an end-to-end CLI regression landed in this review. This extends the already-merged #107 rather than reopening it. |
| Terminal labels can precede required durable effects | **SURVIVED — high.** Slice, scheduler, and supervisor paths could discard persistence errors, mark done before merge, or treat unknown ownership optimistically. | Safe local ordering/unknown-state fixes landed; [#115](https://github.com/swornagent/sworn/issues/115) and S01 own the typed terminal-outcome boundary. |
| The tested task engine is the production task engine | **REFUTED.** `internal/run.Run` is not selected by the CLI; the CLI duplicates task execution and accepts but discards `--base`. | Existing #27 was extended with exact production evidence; S02 owns convergence on one engine. |
| Parallel routing always uses committed-state decisions | **SURVIVED — high.** Router construction failure selected a retired static iterator and a hermetic stale-ref case returned PASS. | Production fallback now fails closed; synthetic compatibility callers opt in explicitly. [#114](https://github.com/swornagent/sworn/issues/114) and S02 own removal/convergence. |
| MCP and TUI are thin control adapters | **REFUTED — high.** They directly rewrite state, use inconsistent read refs, and construct stale CLI commands. MCP rerun could reset state, launch an obsolete command shape that exited 64, and still report a PID. | [#113](https://github.com/swornagent/sworn/issues/113) and S04–S06 define a loop-owned command/event service, revisioned state authority, and adapter parity. |
| Notification delivery has no retry | **REFUTED.** Generic webhook delivery already retries three times. The stronger concern survived: requests were synchronously coupled to phase progress, used an unbounded default client, and had no durable intent/result/replay. | A notifier-owned request and total deadline landed, and machine-local worktree paths were removed from remote payloads. [#111](https://github.com/swornagent/sworn/issues/111) and S07–S08 own the outbox and delivery projections. |
| Architecture lint enforces a project policy | **REFUTED — high.** The engine existed, but no Sworn policy was populated; malformed or empty policy could become zero-rule success. | `docs/architecture.json`, ADR-0014, strict config validation, explicit adoption `SKIP`, and a repository policy-presence test landed. Mutation evidence is below. |
| Current-format specs with no examples fail spec-quality completeness | **REFUTED — high.** All twelve new specs report 100% completeness with zero examples. | [#112](https://github.com/swornagent/sworn/issues/112) owns the false-green repair. The reported PASS is recorded as defect evidence, not trusted validation. |
| The overclaim benchmark fails closed on invalid ground truth | **REFUTED — high.** Missing/invalid labels became ordinary PASS truth and the simulated verdict was circular. | [#110](https://github.com/swornagent/sworn/issues/110) owns the benchmark contract and real mutation corpus. |
| Status lint rejects malformed or unreadable records | **REFUTED — high.** Several parse/read failures were skipped and could produce a successful sweep. | Existing #22/#52 were updated with fresh evidence; no duplicate issue was created. |
| Native `merge-release` means the integration ref moved | **REFUTED — high product/terminal-semantics gap, not a critical bypass.** The command gates, prints readiness, and exits 0 without performing the declared merge; the scheduler intentionally pauses for a human. | Existing #53 was extended. S12 requires one gated operation to own both readiness and the branch mutation, subject to a human decision on delegation. |
| Slice deadlines and cancellation own all child processes | **PARTLY REFUTED.** Driver subprocesses already use `CommandContext`; root signals, model-invoked tools, and proof/test commands do not share one process context and can outlive an attempt. | #109/S03 owns the root signal context, bounded child groups, terminal cancellation evidence, and restart reconciliation; #12/#68 remain adjacent. |
| Subscription CLI drivers receive only required environment | **REFUTED — high confidentiality risk.** Agentic children inherit the complete parent environment. | [#116](https://github.com/swornagent/sworn/issues/116) owns per-driver allowlists and canary-secret reachability tests. The architecture policy forbids new `os.Environ()` child paths. |
| The repository has no dependency decisions | **REFUTED.** SQLite, TUI, and provider dependency families have accepted ADRs. A narrower governance conflict survived between the absolute repository instruction and ADR-0007, plus two direct modules without module-specific ownership. | [#117](https://github.com/swornagent/sworn/issues/117) owns human policy alignment, YAML removal, JSON Schema ratification, and a direct-module registry guard. |
| Telemetry opt-out prevents `Fire` from transmitting | **REFUTED — high privacy defect.** `Fire` never consulted `IsEnabled`; neutral and opt-out states could still attempt a request. | `Fire` now checks consent before install-ID creation or dispatch. Neutral, file-opt-out, and environment-opt-out tests prove zero requests. [#118](https://github.com/swornagent/sworn/issues/118) owns the remaining choice between init prompt and explicit `telemetry on`. |

The planned contract-edge S11 guard-fidelity slice remains adjacent work. It does
not repair any of the guard defects reproduced above, so the review did not use
it as closure evidence.

## Delivered

- A populated 15-rule project architecture policy covering declared
  touchpoints, adapter ownership, checked terminal persistence, process-global
  mutation, context-bound subprocesses, allowlisted child environments,
  canonical credentials/provider configuration, stale operator commands,
  bounded notification transport, payload privacy, and file growth.
- ADR-0014 establishes `docs/architecture.json` as the project policy while
  retaining a legacy adoption fallback.
- One shared credential store preserves unrelated domains, rejects malformed
  existing data, writes atomically with restrictive permissions, honors an
  exact custom credential path, and lets logout remove only account fields.
- Parallel router construction, pre-cancelled execution, scheduler merge
  ordering, supervisor unknown/release semantics, notification deadlines,
  remote payload privacy, and telemetry consent received safe local fixes.
- README and CLI help now distinguish implemented pre-1.0 capabilities from
  open operational gaps.
- Root-cause and residual-decision captures separate mechanical defects from
  choices that require human judgment.
- Issues #109–#118 own new durable work; existing #22, #27, and #53 received the
  fresh evidence that extends their current scope.
- `2026-07-14-autonomous-operations` is a 12-slice, 5-track remediation release
  with 4 traced needs, 91 EARS acceptance criteria, and 15 versioned contracts.
  It sequences engine truth → control core → durable paging → mobile board →
  assembled journey.
- All twelve slices remain `planned`, use `release/v0.1.0` as their integration
  base, have `human_ratified: false`, and retain empty Type-1 `human_decision`
  fields until a person reviews them.

## Files changed

Live command before adding this proof:

```text
git diff --name-only b6e50df166208ce743def84a8443b0aa8835a995 HEAD
```

The command returned the following 64 committed files; this proof is the 65th
review file.

```text
README.md
cmd/sworn/account.go
cmd/sworn/credentials_integration_test.go
cmd/sworn/lint.go
cmd/sworn/login.go
cmd/sworn/main.go
cmd/sworn/run.go
cmd/sworn/run_test.go
cmd/sworn/telemetry.go
docs/adr/0014-project-architecture-policy.md
docs/architecture.json
docs/captures/2026-07-14-architecture-review-findings.md
docs/captures/2026-07-14-architecture-review-root-cause.md
docs/captures/2026-07-14-outstanding-work-catalogue.md
docs/release/2026-07-14-autonomous-operations/S01-terminal-outcome-commit/spec.json
docs/release/2026-07-14-autonomous-operations/S01-terminal-outcome-commit/status.json
docs/release/2026-07-14-autonomous-operations/S02-execution-authority/spec.json
docs/release/2026-07-14-autonomous-operations/S02-execution-authority/status.json
docs/release/2026-07-14-autonomous-operations/S03-cancellation-recovery/spec.json
docs/release/2026-07-14-autonomous-operations/S03-cancellation-recovery/status.json
docs/release/2026-07-14-autonomous-operations/S04-command-event-service/spec.json
docs/release/2026-07-14-autonomous-operations/S04-command-event-service/status.json
docs/release/2026-07-14-autonomous-operations/S05-revisioned-state-ownership/spec.json
docs/release/2026-07-14-autonomous-operations/S05-revisioned-state-ownership/status.json
docs/release/2026-07-14-autonomous-operations/S06-control-adapter-parity/spec.json
docs/release/2026-07-14-autonomous-operations/S06-control-adapter-parity/status.json
docs/release/2026-07-14-autonomous-operations/S07-notification-outbox/spec.json
docs/release/2026-07-14-autonomous-operations/S07-notification-outbox/status.json
docs/release/2026-07-14-autonomous-operations/S08-webhook-mobile-delivery/spec.json
docs/release/2026-07-14-autonomous-operations/S08-webhook-mobile-delivery/status.json
docs/release/2026-07-14-autonomous-operations/S09-operations-read-api/spec.json
docs/release/2026-07-14-autonomous-operations/S09-operations-read-api/status.json
docs/release/2026-07-14-autonomous-operations/S10-responsive-web-board/spec.json
docs/release/2026-07-14-autonomous-operations/S10-responsive-web-board/status.json
docs/release/2026-07-14-autonomous-operations/S11-authenticated-remote-controls/spec.json
docs/release/2026-07-14-autonomous-operations/S11-authenticated-remote-controls/status.json
docs/release/2026-07-14-autonomous-operations/S12-autonomous-operations-journey/spec.json
docs/release/2026-07-14-autonomous-operations/S12-autonomous-operations-journey/status.json
docs/release/2026-07-14-autonomous-operations/board.json
docs/release/2026-07-14-autonomous-operations/contracts.json
docs/release/2026-07-14-autonomous-operations/index.md
docs/release/2026-07-14-autonomous-operations/intake.md
docs/release/2026-07-14-autonomous-operations/screenshots/.gitkeep
internal/account/account.go
internal/account/notify.go
internal/account/notify_test.go
internal/bench/overclaim.go
internal/credentials/store.go
internal/credentials/store_test.go
internal/gate/archrules.go
internal/gate/archrules_test.go
internal/gate/design.go
internal/model/credentials.go
internal/run/blocked_report_test.go
internal/run/parallel.go
internal/run/parallel_test.go
internal/run/run_test.go
internal/run/slice.go
internal/scheduler/worker.go
internal/scheduler/worker_test.go
internal/supervisor/supervisor.go
internal/supervisor/supervisor_test.go
internal/telemetry/telemetry.go
internal/telemetry/telemetry_test.go
docs/captures/2026-07-14-architecture-review-proof.md
```

## Test results

All commands below ran in the isolated review worktree against live state.

| Command / evidence | Result |
|---|---|
| `go test -count=1 -timeout 10m ./...` | PASS across all 50 packages |
| Same full suite under `env -i` with an empty temporary HOME and isolated build cache | PASS |
| `go test -shuffle=1784006779896344205 -count=1 -v ./internal/telemetry` | PASS; reproduces and closes the prior order-dependent consent/install-ID failure |
| `go vet ./...` | PASS |
| `make build` | PASS; built `bin/sworn` |
| `bash scripts/public-safe-scan.sh` | PASS across 1,655 tracked files |
| `git diff --check` | PASS |
| strict vendored JSON Schema validation | PASS for 12 specs, 12 statuses, board, and contracts |
| spec `touchpoints` versus status `planned_files` | PASS; exact match for all 12 slices |
| `sworn lint ac 2026-07-14-autonomous-operations` | PASS; 91 well-formed criteria |
| `sworn lint trace 2026-07-14-autonomous-operations` | PASS; 4/4 needs traced through 12 slices and 91 criteria |
| `sworn board --release 2026-07-14-autonomous-operations --json` | PASS from committed Git state; 5 tracks returned |
| `sworn reqvalidate 2026-07-14-autonomous-operations` | Expected FAIL; 0/12 human-ratified |
| `sworn designfit 2026-07-14-autonomous-operations` | Expected FAIL; 28 Type-1 decisions await human judgment |
| `sworn specquality 2026-07-14-autonomous-operations` | Defect reproduced: false PASS at 100% with zero examples; #112 |

The first full-suite pass exposed two review-created test integration problems
before final validation: the stalled-peer notification test could hang its own
test-server cleanup, and the synthetic overclaim benchmark had not explicitly
opted into its non-Git static fixture path after production fallback was made
fail-closed. Both were corrected, then the complete suite and clean-environment
suite were rerun successfully.

## Reachability and architecture mutation artefact

Committed baseline:

```text
bin/sworn lint design --release 2026-07-14-autonomous-operations \
  --slice S02-execution-authority --base HEAD

Rules: 15 checked  violations: 0
PASS — no architecture rule violations
PASS — design lint clean
exit 0
```

In a disposable ignored clone at review commit `90df448`, one line was added to
the S02-declared `internal/run/parallel.go` touchpoint and committed:

```go
_ = os.Setenv("SWORN_ARCH_MUTATION", "1")
```

Running the real binary against the exact parent commit produced:

```text
bin/sworn lint design --release 2026-07-14-autonomous-operations \
  --slice S02-execution-authority --base 90df448

Rules: 15 checked  violations: 1
[error] no-new-process-global-mutation — pattern matched
in internal/run/parallel.go:131
FAIL — 1 error violation(s)
exit 1
```

The declared-touchpoint leg passed, so the non-zero result was caused by the
intended architecture rule rather than an unrelated guard. The disposable clone
was removed after capture.

## Not delivered

- The review does **not** claim that true autonomous loop operation, a durable
  control service, notification outbox, mobile board, or authenticated remote
  controls are implemented. They are planned under #109 and remain gated.
- Terminal outcome convergence, the dead/split task engine, cancellation and
  child cleanup, MCP/TUI control ownership, benchmark ground truth,
  spec-quality examples, status-lint parse failures, subscription-child secret
  inheritance, and dependency-policy alignment remain owned by the issues and
  slices named above.
- The notification deadline is containment, not durable delivery. S07 must
  persist intent/results and S08 must prove webhook/mobile replay and bounded
  adapter outcomes.
- The architecture rules prevent new instances of several patterns; they do not
  assert that every historical violation has already been removed. Their planned
  slices own the migrations.
- No Type-1 recommendation in the remediation release is recorded as a human
  decision. Requirements/design gates deliberately remain non-zero.
- The review branch was not pushed, merged, or rebased onto the independently
  moving `main` worktree.

## Divergence from plan

- The mobile-board track was made dependent on durable paging rather than
  developed in parallel. The board's notification-health surface must consume
  the real C-05 envelope/result contract; a parallel placeholder would recreate
  the seam this review is removing.
- The architecture review remained on the exact branch-cut commit while `main`
  advanced in another session. This preserves reproducibility and avoids
  interfering with concurrent work.
- Broad initial claims about notification retry, dependency ratification,
  driver cancellation, and merge safety were narrowed by fresh refutation. The
  final findings record only the surviving contracts.
