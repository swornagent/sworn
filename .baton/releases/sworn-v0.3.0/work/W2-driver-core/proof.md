# Candidate binding

- Repository: `swornagent/sworn`
- Base commit: `b2bb28778a4f73f5841bb936a993860d0b10d0ce`
- Candidate commit: `0136a96c4355e60c815b5cab043b54e860d00062`
- Candidate tree: `252f1a7c326cbc604600e7beb2393bb7f4f22fd1`
- Product-tree digest: `sha256:a9148b61fa37c11c6abc293160e6b8c3d4b28830d04a1ac2cc913506b6d016bf`
- Plan digest: `sha256:cf6e9103219c76a12834fbaf1eb9da8576b765dfe0602ebe416e756ed8ca10f8`
- Approval digest: `sha256:1cf79386fa391d93c19e03abe322e0425455967bec5a03785a046356b8aa2a0c`
- Design digest: `sha256:b191c42e070914fdd32e88c36cd0177f408eb5c4ad30999b5ed5c9d35a374b03`
- Captain invocation: `codex:/root/w2_driver_captain/sworn-v0.3.0/T2-driver/W2-driver-core/review/2`
- Producer invocation: `codex:/root/w2_driver_build/sworn-v0.3.0/T2-driver/W2-driver-core/implement/2`

# Acceptance evidence

| Acceptance | Result | Evidence reference |
| --- | --- | --- |
| A-W2-contract | pass | The candidate implements one role-neutral `Driver`/`Invoker` and strict closed codecs for the exact admitted `baton.driver/v1` info, request and result values. Canonical operations are loaded from the W0-admitted Baton RC2 package and bind exact instruction bytes and digests. Portable coverage exercises all five role/operation pairs, including deliberate `merge` with `model:null`; production selection requires exactly Planner, Implementer, Captain and Verifier provider/model pairs and rejects Merge, missing providers, defaults, fallback and mismatched results. The invocation-bound permission and sealed-submission protocol admits only the six exact responsibility rows, artifact order and decision set; it exposes no Baton mutation or Git action. The shared deterministic fake and adapter conformance harness pass exact empty-text, malformed-envelope, binding, replay, cross-invocation and secret-retention cases. |
| A-W2-isolation-usage | pass | Real `/usr/bin/bwrap` `0.9.0` tests execute separately built driver fixtures through an FD-pinned root-owned Bubblewrap boundary with an empty inherited environment, isolated mount/PID/user namespaces, a disposable workspace, and only digest-bound `.sworn-inputs/v1` files mounted read-only. The fake driver is now bound to `NetworkNone` at configuration, permission and launch argument construction, so untrusted input cannot select `--share-net`. Read-only workspace manifests, reserved-path and mount conflicts, cancellation, blocked descendants, malformed/partial control frames, missing/multiple/oversized/non-zero results, bounded stderr and parallel cross-talk tests fail closed with no eligible handoff or surviving process. Normalized receipts preserve reported zero versus unavailable token values and accept cost only as a complete typed `provider_reported` observation; no estimate or pricing path exists. |

# Fresh-verifier repair closure

The first fresh verifier rejected candidate
`3b30295e6f8d04963266f9b98880df434f9c4f6c` and the Baton record captured
that `FAIL` before repair. This superseding candidate closes each reported
finding:

1. Workspace and repository-path admission now matches the pinned RC2 codec:
   UTF-8 and C0/C1 control rejection, byte-counted 4096/1000 limits,
   canonical relative repository paths, and rejection only when the first
   repository segment is `.git`. Focused boundaries prove `.git/config`
   rejects while `nested/.git/file` and `nested/.gitignore` remain valid.
2. The fake driver cannot request network access through configuration,
   submission permission or direct Bubblewrap argument construction. A
   networked vendor driver still receives its declared network boundary.
3. The control endpoint preserves the accepted seal and the typed
   `SUBMISSION_CONFLICT` result across the transport. Replay is idempotent and
   a conflicting later submission cannot replace the accepted bytes.

# Checks

| Command or check | Exit status | Raw evidence reference |
| --- | --- | --- |
| `GOFLAGS=-buildvcs=false go test -count=1 -timeout=180s ./...` | 0 | Independent root run against the exact final candidate; combined output `sha256:b0e2cb69a9f31eabcf24c451da1238877d6ab34700164f30ded1364415695ef8`. |
| `GOFLAGS=-buildvcs=false go test -race -count=1 -timeout=180s ./internal/driver/...` | 0 | Independent root run against the exact final candidate; combined output `sha256:d142f5bbb2fcd479eaaa8f5054a0eee73991d21865fd9787029ae0c96e16d5fc`. |
| `go test -race -shuffle=on -count=3 -timeout=180s ./internal/driver` | 0 | Independent adversarial-order repetition; combined output `sha256:fe407eb4338d14b9a03822f4661aa1f0d543f0946942e1c965706d9ffd67ad46`. |
| Twenty repetitions of RC2 path parity, fake network isolation and typed submission-conflict preservation | 0 | Independent focused run; combined output `sha256:3562a68128de9ee566b568623839d588764b2fe1c27c5077f2e9f29944b5b6dd`. |
| `GOFLAGS=-buildvcs=false go vet ./internal/driver/...` | 0 | No output; `sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`. |
| Product formatting, `git diff --check`, clean worktree and changed-path audit | 0 | Product history changes exactly 19 paths, all under `internal/driver`; name-status digest `sha256:a32e10f45ecefc83ed5dd1fdeb1449d0080650892ce0c3f13b729e99a31b15b2`. The repair after recorded `FAIL` changes exactly 10 of those paths; repair name-status digest `sha256:73af225b800c8d598cbedee68861f6066f68dd2bdd7e202a4ce19a42ab92dab0`. No runtime, journal, Git, CLI, configuration, dependency, adapter or provider implementation path changed. |
| RC2 product identity through the pinned JavaScript development oracle | 0 | Candidate tree `252f1a7c326cbc604600e7beb2393bb7f4f22fd1`; 78 ordered product entries; product-tree digest `sha256:a9148b61fa37c11c6abc293160e6b8c3d4b28830d04a1ac2cc913506b6d016bf`. |

# Deviations

None.

# Not delivered

Production provider adapters, credential injection, scheduling, retry,
authority mutation, Git provenance, journal recovery, evaluation and OTel are
deliberately owned by later approved work and are not W2 claims.
