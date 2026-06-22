---
title: 'S50-baton-governance — `sworn baton diff` divergence check + PR-up process doc; sworn never silently forks the protocol'
description: 'sworn vendors Baton (S48) and pins it by tag (S49), but nothing detects when the embedded protocol has diverged from its upstream pin, and nothing documents that protocol changes discovered during sworn development must go back upstream. S50 adds `sworn baton diff` (embed vs upstream pin, fail-closed on divergence), docs/baton-governance.md (the PR-up workflow), and finalises ADR-0006. depends_on S48. See ADR-0006 and sawy3r/baton#31.'
---

# Slice: `S50-baton-governance`

## User outcome

A maintainer runs `sworn baton diff` and the binary reports whether its embedded Baton
protocol still matches the upstream pin (modulo the known, declared sworn-native
transform from S48). If someone hand-edited an embedded rule or prompt — a silent fork
— the command **fails closed** and names the divergent file. `docs/baton-governance.md`
documents the rule: protocol changes found during sworn development are raised as a
**PR against Baton**, reviewed/authorised, merged, and then re-vendored — never forked
in place. ADR-0006 is the architectural record; the divergence detector is its
enforcement.

## Entry point

`sworn baton diff` — a new subcommand on `cmd/sworn/baton.go` (the file S48 creates),
dispatched via `cmd/sworn/main.go` → `internal/baton`. Reachable from the CLI on any
checkout.

## Background

ADR-0006 establishes the governance model: the embed is a build product of (pinned tag
+ transform), and sworn never silently forks. S48 gives the transform and the vendor;
S49 gives the semver pin and surfacing. The missing piece is **detection + process**:

- **Detection** — `sworn baton diff` re-applies the S48 transform to the pinned source
  and compares against the committed embed. A non-empty diff means the embed was edited
  out-of-band (a fork) or the source/transform changed without a re-vendor. This is the
  consumer↔creator drift guard for the protocol itself.
- **Process** — a written workflow so a contributor who finds a protocol gap (a new
  rule, a wording fix) knows to PR Baton, not edit the embed. The three fidelity-layer
  rules (08/09/10) born in sworn, and the VERSION/tag-discipline ask, are the live
  examples — tracked upstream at sawy3r/baton#31.

This slice `depends_on S48-baton-vendor` (it reuses `internal/baton`'s transform +
source resolution) and runs after S49 in track order (the pin it diffs against is S49's
semver tag).

## In scope

- `internal/baton/diff.go` (new) — `Diff(opts) ([]Divergence, error)`: re-applies
  `Transform` to the pinned source and compares file-by-file against the committed
  embed (`internal/adopt/baton/**`, `internal/prompt/baton/**`); returns the list of
  divergent files (path + a short reason). Empty list ⇒ in sync.
- `cmd/sworn/baton.go` (extend) — add the `diff` subcommand: prints divergences and
  **exits non-zero** when the list is non-empty (fail-closed); exits 0 when in sync.
- `docs/baton-governance.md` (new) — the PR-up workflow: (1) protocol change discovered
  → open a PR against `github.com/sawy3r/baton`; (2) review/authorise/merge + tag bump;
  (3) `sworn baton vendor` re-pins to the new tag; (4) `sworn baton diff` must be clean
  in CI. Documents that the embed is generated, not hand-edited, and links ADR-0006 and
  sawy3r/baton#31 (incl. the 08/09/10 reconvergence debt).
- Finalise `docs/adr/0006-baton-protocol-sync.md` status if any open question remains
  (it is already written as `accepted` this replan; S50 confirms the enforcement it
  describes now exists).

## Out of scope

- The vendor/transform itself (**S48**) and the version pin/surfacing (**S49**).
- Wiring `sworn baton diff` into CI — recommended in the doc, but adding a CI workflow
  file is a separate harness change; if a hook is left, surface it as a Rule-2 deferral.
- Networked fetch of the upstream tag to diff against a live remote — the diff is
  against the pinned local source (same source S48 vendors from); a live-remote diff is
  a later enhancement.
- Actually filing/merging the upstream Baton PRs — that is upstream work tracked at
  sawy3r/baton#31, not a sworn slice deliverable.

## Planned touchpoints

- `internal/baton/diff.go` (new)
- `internal/baton/diff_test.go` (new)
- `cmd/sworn/baton.go` (extend with `diff` subcommand — created by S48, same track)
- `docs/baton-governance.md` (new)
- `docs/adr/0006-baton-protocol-sync.md` (confirm/finalise — written this replan)

## Acceptance checks

- [ ] `Diff` returns an empty list against a freshly-vendored (in-sync) embed
- [ ] `Diff` returns a non-empty list naming the file when an embedded rule/prompt is
  hand-edited away from the transformed-pinned source (test by mutating a fixture embed)
- [ ] `sworn baton diff` exits 0 when in sync and **non-zero** when divergent (assert
  exit code via the command, not just `Diff` — Rule 1 integration point)
- [ ] `sworn baton diff` output names each divergent file path
- [ ] `docs/baton-governance.md` exists and documents the four-step PR-up workflow and
  links ADR-0006 + sawy3r/baton#31 (assert the file contains the issue ref and the
  "never edit the embed directly" rule)
- [ ] `go test -race ./internal/baton/... ./cmd/sworn/...` passes; `go build ./...` clean

## Required tests

- **Unit**: `internal/baton/diff_test.go` — `TestDiffCleanWhenInSync`,
  `TestDiffDetectsHandEditedEmbed`, `TestBatonDiffExitsNonZeroOnDivergence` (drives the
  `cmd/sworn` entry point).
- **Reachability artefact**: paste `sworn baton diff` output for both the in-sync (exit
  0) and divergent (exit non-zero) cases in `proof.md`.

## Risks

- `Diff` must use the **same** transform + source resolution as S48's `Vendor`, or a
  clean tree will show false divergence. Share the code path; assert "vendor then diff
  is clean" in a test.
- The diff must tolerate the declared sworn-native transform (script refs are
  *expected* to differ from raw Baton) — it compares against the **transformed** pinned
  source, not raw upstream. Getting this wrong makes diff always-dirty.
- Don't let `docs/baton-governance.md` duplicate ADR-0006 prose and drift — the ADR is
  the decision record; the governance doc is the operational how-to that links it.

## Deferrals allowed?

CI wiring and live-remote diff may be deferred as Rule-2 deferrals (why + tracking +
acknowledgement in `proof.md`). The `diff` command, fail-closed exit, and the
governance doc are core and may not be deferred.
