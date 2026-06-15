---
title: S10-benchmark-dogfood
description: model × hosting-jurisdiction × cost × pass-rate benchmark + a real-repo E2E dogfood run (the launch proof).
---

# Slice: `S10-benchmark-dogfood`

## User outcome

A `model × hosting-jurisdiction × cost × pass-rate` benchmark report exists (the
launch-proof number), the **safe-hosted default model is chosen from data**, and
`sworn run` is proven on a real repo end-to-end.

## Entry point

CLI: `sworn bench` (`cmd/sworn/bench.go`); the dogfood is a real `sworn run`.

## In scope

- A benchmark harness (`internal/bench/`) over a public task set, running the loop
  across candidate models — **Sonnet + Opus baseline** (per-role: saving =
  implementer leg only) and safe-hosted commodity options — recording pass-rate,
  cost, and hosting jurisdiction.
- Pick the safe-hosted default from the data.
- Dogfood: run `sworn run` on a real repo and land a verified, merged change.

## Out of scope

- Publishing / marketing.

## Planned touchpoints

- `internal/bench/`, `cmd/sworn/bench.go`, `docs/benchmark/`

## Acceptance checks

- [ ] A report table (`model × jurisdiction × cost × pass-rate`) is produced.
- [ ] The safe-hosted default is selected from benchmark data (no non-trusted-
      hosted model blessed as default).
- [ ] A real `sworn run` lands a verified, merged change (the turnkey demo).

## Required tests

- **Unit**: benchmark aggregation (pass-rate, cost roll-ups).
- **Reachability artefact**: the dogfood `sworn run` transcript + the merged commit.
- **playwright-screenshot** N/A — not a Playwright slice; reachability artefact is CLI transcript, not visual screenshot.
## Risks

- Task-set bias — use a public set; document selection.
- Cost blowout — per-run cost cap.

## Deferrals allowed?

No.
