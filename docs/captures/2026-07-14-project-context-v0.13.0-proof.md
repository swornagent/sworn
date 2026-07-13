# Proof bundle — Baton v0.13.0: declared project context + stakes

Date: 2026-07-14
Handoff: `docs/captures/2026-07-14-baton-v0.13.0-project-context-handoff.md` (`0d4b9cc`)
Protocol: [Baton v0.13.0](https://github.com/sawy3r/baton/releases/tag/v0.13.0) (`f2ccdff`)
Base: stacked on `feat/baton-v0.12.0-llm-checks` (sworn#105)

## Scope

Consume Baton v0.13.0: make the project context a **declared, human-ratified,
version-controlled record** (`.sworn/project.json`), key the security-review blocking
threshold to its stakes, and fail closed to HIGH stakes whenever the record is absent,
malformed, or unratified.

## Reconciliation against the handoff (Rule 6 continuation handshake)

Each acceptance item, checked against live repo state:

| Handoff item | State | Evidence |
|---|---|---|
| Re-vendor to v0.13.0, both embed roots | **done** | `internal/adopt/baton/VERSION` = `v0.13.0` / `f2ccdff`; `internal/prompt/baton/llm-checks/` carries the stakes-keyed `security-review.md` |
| 1. `.sworn/project.json` validated against `project-context-v1` | **done** | `internal/project` `Load`/`Save`; schema `GRADED` in `manifest.go` |
| 1a. `sworn init` elicits; model drafts; cannot self-ratify | **done** | `internal/project/elicit.go`, `cmd/sworn/init_project.go`; `Ratified: false` is hardcoded on the drafted record |
| 1b. Validate on write | **done** | `Save` grades before writing — `TestSave_RefusesAnInvalidRecord` |
| 2. `llm-check` consumes `{{project_context}}` + `{{project_stakes}}` | **done** | `userPromptHeaderFor(project.Resolved)`; `TestUserPayloadCarriesStakes` |
| 3. Fail-closed stakes resolution | **done** | `project.Resolve`; `TestResolve_FailsClosedOnStakes` (7 cases) |
| 4. `sworn doctor` flags inferred/unratified as undeclared | **done** | `checkProjectContext`; four live states below |
| 5a. Detection stays, **labelled** | **done** | `SourceInferred` + `RenderStakes` says so to the model |
| 5b. Detection sees a top-level `go/` module | **done** | `757da54` (landed on the base branch); `TestDetect/polyglot_monorepo_with_a_top-level_go_backend` |

## Files changed

New package `internal/project`:

```text
internal/project/project.go       Record, Load, Save, Resolve, RenderStakes  (the fail-closed core)
internal/project/elicit.go        Elicit — the model DRAFTS from repo evidence
internal/project/detect.go        (moved from internal/gate) the labelled fallback
internal/project/project_test.go  the fail-closed contract
cmd/sworn/init_project.go         elicit -> review -> ratify
cmd/sworn/doctor.go               Group 1c: project context
internal/gate/llmcheck.go         payload carries {{project_stakes}}
internal/gate/userpayload_test.go payload guards
internal/baton/schemas/           + project-context-v1.json (vendored byte-identical)
internal/baton/manifest.go        project-context-v1 = GRADED
internal/adopt/baton/VERSION      v0.12.0 -> v0.13.0
```

## The fail-closed rule (the load-bearing part)

Stakes decide whether a `medium` security finding **blocks** or merely **advises**. So the
only path to a lowered bar is a **ratified** record that declares low stakes:

| record state | stakes | source |
|---|---|---|
| absent | **HIGH** | inferred |
| malformed / not JSON | **HIGH** | inferred |
| **unratified, claiming low stakes** | **HIGH** | drafted |
| ratified, stakes omitted | **HIGH** | declared |
| ratified, declares low stakes | low | declared |
| ratified, declares high stakes | **HIGH** | declared |

`TestResolve_FailsClosedOnStakes` covers all seven. The third row is the trap: a model
drafts *"this is just a CLI, nothing at risk"* and nobody confirms it. If an unratified
draft could lower the bar, the whole mechanism would be self-certification with extra
steps — the same hole as sworn#103, one layer up.

**A proposal may raise the bar; it may never lower it.** Same asymmetry as
`LLMFinding.IsBlocking()`, for the same reason.

## Reachability artefact

`sworn doctor` (Group 1c), live binary, all four states:

```text
1. UNDECLARED (no record)
[WARN]  project/context
        UNDECLARED — no .sworn/project.json. The context "a Go project" was INFERRED from
        your file layout, which can read your languages but cannot know whether real
        customers depend on this. Every check runs at fail-closed HIGH stakes.
        Run 'sworn init' to draft and ratify it.

2. DRAFTED, unratified — the record CLAIMS low stakes
[WARN]  project/context
        DRAFTED but NOT RATIFIED — "a Go CLI". A model proposed this; no human has
        confirmed it, so every check runs at fail-closed HIGH stakes regardless of the
        stakes it claims. Review .sworn/project.json and set ratification.ratified = true.

3. DECLARED + RATIFIED
[OK]    project/context
        declared + ratified — "a Next.js and TypeScript frontend with a Go backend on
        Postgres" (HIGH stakes)

4. MALFORMED
[ERROR] project/context
        .sworn/project.json is present but INVALID — checks run at fail-closed HIGH stakes
```

This is the visibility mechanism the whole design turns on: a detection guess can no
longer masquerade as a declaration.

## The elicitation is the adopter's call, not the protocol's

`sworn init` drafts the record with the **adopter's own configured model and credentials**,
against their own provider. sworn never phones home. Drafting means sending repository
content to a model, so the step:

- **states what it sends** before sending it (top two levels of directory names; README,
  go.mod, package.json, tsconfig — **no source files, no `.env`**),
- **is skippable** (skipping is safe, not silent: checks then run at HIGH stakes with an
  inferred description),
- **never defaults ratification to yes.** The unratified record is the safe state; a
  wrongly-ratified one silently lowers the security bar. The prompt is `(y/n) [n]`.

The model is also asked to return an `uncertain` list — the stakes fields it *guessed* at
rather than read evidence for — which `init` surfaces to the human explicitly.

## Test results

`go test ./...`: **48 packages ok, 0 failures, exit 0.** `gofmt -l` clean, `go vet` clean.

```text
TestResolve_FailsClosedOnStakes/no_record_at_all                        PASS
TestResolve_FailsClosedOnStakes/UNRATIFIED_record_claiming_low_stakes   PASS
TestResolve_FailsClosedOnStakes/malformed_record                        PASS
TestResolve_FailsClosedOnStakes/not_even_JSON                           PASS
TestResolve_FailsClosedOnStakes/ratified_but_stakes_omitted_entirely    PASS
TestResolve_FailsClosedOnStakes/RATIFIED_high_stakes                    PASS
TestResolve_FailsClosedOnStakes/RATIFIED_low_stakes                     PASS
TestResolve_DeclaredContextBeatsDetection                               PASS
TestRenderStakes_NeverClaimsLowOnAGuess                                 PASS
TestRenderStakes_TellsTheModelTheStakesAreAssumed                       PASS
TestSave_RefusesAnInvalidRecord                                         PASS
TestSaveLoadRoundTrip                                                   PASS
TestUserPayloadCarriesStakes                                            PASS
TestUserPayloadNeverClaimsLowStakesOnAGuess                             PASS
TestUserPromptHeaderNamesTheRealProject                                 PASS
```

## Not delivered

- **The end-to-end "a medium finding blocks at high stakes" is not proved against a live
  model.** The behaviour is a *composition*: sworn renders the stakes truthfully (tested),
  the model marks the finding `blocking: true` per the stakes-keyed prompt (a model
  behaviour, driven by the Baton prompt), and sworn honours `blocking` (tested —
  `TestHasViolations_BlockingEscalatesButCannotDeEscalate`). **Both halves sworn owns are
  tested; the middle needs a real model call.** *Why:* a live run needs an API key and a
  real slice with a genuine medium-severity finding. *Tracking:* this bundle.
  *Acknowledgement:* raised with the Coach — this is the one thing worth doing by hand
  before relying on the stakes gate.
- **`sworn init`'s elicitation path is not covered by an automated test.** It is an
  interactive flow that makes a model call. The pieces beneath it (`Elicit` parsing,
  `Save` validation, `Resolve` fail-closed) are tested; the prompt-and-confirm shell is
  not. *Tracking:* this bundle. *Acknowledgement:* session wrap-up.

## Divergence from plan

None. Every handoff item landed as specified. Two additions beyond it, both defensive:

- **The `uncertain` field.** The elicitation prompt asks the model to name the stakes
  fields it *guessed* at, and `init` shows them to the human. The handoff asked for a
  draft; a draft that cannot say *"I am not sure whether you have real users"* invites the
  human to rubber-stamp exactly the field that matters most.
- **A malformed record is an ERROR, not a WARN.** A record that is present but unreadable
  is worse than an absent one: it looks declared and reads as nothing.
