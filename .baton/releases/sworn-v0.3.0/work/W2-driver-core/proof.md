# Candidate binding

- Repository: `swornagent/sworn`
- Base commit: `b2bb28778a4f73f5841bb936a993860d0b10d0ce`
- Candidate commit: `3b30295e6f8d04963266f9b98880df434f9c4f6c`
- Candidate tree: `b895c8ac991dfdc0184c8d58c770e76aebbf188b`
- Product-tree digest: `sha256:117297e4ba69c9a9e7defcbb465630459419db9722f0c7971b7d743cf2d8b8b7`
- Plan digest: `sha256:cf6e9103219c76a12834fbaf1eb9da8576b765dfe0602ebe416e756ed8ca10f8`
- Approval digest: `sha256:1cf79386fa391d93c19e03abe322e0425455967bec5a03785a046356b8aa2a0c`
- Design digest: `sha256:b191c42e070914fdd32e88c36cd0177f408eb5c4ad30999b5ed5c9d35a374b03`
- Captain invocation: `codex:/root/w2_driver_captain/sworn-v0.3.0/T2-driver/W2-driver-core/review/2`
- Producer invocation: `codex:/root/w2_driver_build/sworn-v0.3.0/T2-driver/W2-driver-core/implement/1`

# Acceptance evidence

| Acceptance | Result | Evidence reference |
| --- | --- | --- |
| A-W2-contract | pass | Candidate `3b30295e6f8d04963266f9b98880df434f9c4f6c` implements one role-neutral `Driver`/`Invoker` and strict closed codecs for the exact admitted `baton.driver/v1` info, request and result values. Canonical operations are loaded from the W0-admitted Baton RC2 package and bind exact instruction bytes and digests. Portable coverage exercises all five role/operation pairs including deliberate `merge` with `model:null`; production selection requires exactly Planner, Implementer, Captain and Verifier provider/model pairs and rejects Merge, missing providers, defaults, fallback and mismatched results. The invocation-bound permission and sealed-submission protocol admits only the six exact responsibility rows, artifact order and decision set; it exposes no Baton mutation or Git action. The shared deterministic fake and adapter conformance harness pass exact empty-text, malformed-envelope, binding, replay, cross-invocation and secret-retention cases. |
| A-W2-isolation-usage | pass | Real `/usr/bin/bwrap` `0.9.0` tests execute separately built driver fixtures through an FD-pinned root-owned Bubblewrap boundary with an empty inherited environment, isolated mount/PID/user namespaces, no network for the fake, a disposable workspace, and only digest-bound `.sworn-inputs/v1` files mounted read-only. Read-only workspace manifests, reserved-path and mount conflicts, cancellation, blocked descendants, malformed/partial control frames, missing/multiple/oversized/non-zero results, bounded stderr and parallel cross-talk tests fail closed with no eligible handoff or surviving process. Normalized receipts preserve reported zero versus unavailable token values and accept cost only as a complete typed `provider_reported` observation; no estimate or pricing path exists. |

# Checks

| Command or check | Exit status | Raw evidence reference |
| --- | --- | --- |
| `GOFLAGS=-buildvcs=false go test -count=1 -timeout 180s ./...` | 0 | Independent committed-candidate run; combined output `sha256:9737b69ca6ba7322d4b5dc5a02731da39d60e33871afebe2c29ef2447bb7082d`. |
| `GOFLAGS=-buildvcs=false go test -race -count=1 -timeout 180s ./internal/driver/...` | 0 | Independent committed-candidate run; combined output `sha256:31b4b5623c9c1141a44b6e7f27184f1dce27282a853cb9b6c2e4c0c01ba6ffe6`. |
| `GOFLAGS=-buildvcs=false go test -race -shuffle=on -count=3 -timeout 180s ./internal/driver/...` | 0 | Independent adversarial-order repetition; package passed in 11.255 seconds. |
| Five repetitions of cancellation/descendant cleanup, malformed submission channel, parallel cross-talk and accepted-then-partial-frame cases | 0 | Independent focused run; combined output `sha256:eaa54f436e6979e432f7c2377f13df8381870eaac1c9307bedeeba3101f0e22e`. |
| `GOFLAGS=-buildvcs=false go vet ./internal/driver/...` | 0 | No output; `sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`. |
| Product-only formatting, `git diff --check`, clean worktree and changed-path audit | 0 | Final product commit changes exactly 19 paths, all under `internal/driver`; final-commit name-status digest `sha256:a32e10f45ecefc83ed5dd1fdeb1449d0080650892ce0c3f13b729e99a31b15b2`. No Baton record, runtime, journal, Git, CLI, configuration, dependency, adapter or provider path changed. |
| RC2 product identity through the pinned JavaScript development oracle | 0 | Candidate tree `b895c8ac991dfdc0184c8d58c770e76aebbf188b`; 78 ordered product entries; product-tree digest `sha256:117297e4ba69c9a9e7defcbb465630459419db9722f0c7971b7d743cf2d8b8b7`. |

# Deviations

None.

# Not delivered

Production provider adapters, credential injection, scheduling, retry,
authority mutation, Git provenance, journal recovery, evaluation and OTel are
deliberately owned by later approved work and are not W2 claims.
