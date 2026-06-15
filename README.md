# SwornAgent

**The verification layer that makes coding agents accountable.** SwornAgent runs
an independent, fresh-context adversarial verifier against a change's
`spec → diff (→ proof)` and returns a fail-closed verdict — so unverified work
cannot reach merged state. Built on the open [Baton](https://github.com/sawy3r/baton)
protocol.

> Status: early scaffold (S1 — provider-neutral verifier core). The model
> dispatch leg is stubbed (fails closed) until the OpenAI-compatible client lands.

## What it is

- **Provider-neutral core** — a single Go binary, zero runtime dependencies,
  runs in any CI. You choose the model and bring your own key; SwornAgent owns
  the protocol (fresh context, artefact-only inputs, fail-closed verdict).
- **PASS / FAIL / BLOCKED**, exit `0` only on PASS — so a CI required-check
  blocks the merge by default.

## Quick start

```sh
make build
./bin/swornagent verify --spec spec.md --diff change.diff
```

`verify` emits a JSON verdict and sets the exit code from it
(`0`=PASS, `1`=FAIL, `2`=BLOCKED).

## Roadmap (MVP)

- [x] Verdict contract + deterministic first-pass + fail-closed model stub (S1)
- [ ] OpenAI-compatible verifier client (BYO-key, customer-chosen model)
- [ ] Spec/diff/proof resolution at the PR boundary (fail-closed)
- [ ] GitHub Action + required status check (the merge gate)
- [ ] `model × hosting-jurisdiction × cost × pass-rate` benchmark
- [ ] Distribution: Action Marketplace + container (GHCR) + Homebrew

## Licence

MIT. SwornAgent is the product; Baton is the open protocol it depends on.
