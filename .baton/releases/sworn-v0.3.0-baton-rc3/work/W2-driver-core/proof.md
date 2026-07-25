# W2 driver core implementation proof

This proof is Implementer evidence only; it does not record or claim a
Verifier verdict.

## Candidate binding

- Repository: `swornagent/sworn`
- Release/work/track: `sworn-v0.3.0-baton-rc3` /
  `W2-driver-core` / `T2-driver`
- Owner branch: `track/sworn-v0.3.0-baton-rc3/T2-driver`
- Authoritative clean implementation start:
  `babb19d9c5c995d095915bc6eb79cd4540130d64`
- Materialization base:
  `c846e8d8b9c1e054657e4b94dd586c4b8e7afac7`
- Frozen T0 dependency:
  `7d925851dc91a4ee324d9fe29c33d631f44d1a56`
- Read-only W1 product input:
  `42c305a4f747520c5d8c54d60da7f3c63c0f8dfe`, tree
  `3b64024769da7235deb4b0fc34181b4f5b312914`, product identity
  `sha256:6630f1fb5117d2de34c9116717106a0e4508f2b10e39ec82c56d0d9401873e50`
- Durable W1 record-only publication observed before the W2 commit:
  `c92e0f05f6ada5b7b1ee524e2b98f10e38dca3c5`
- Candidate:
  `6687f928915d1c0da43ce0957d40e99096617c94`
- Candidate parent:
  `babb19d9c5c995d095915bc6eb79cd4540130d64`
- Candidate tree:
  `e8e1c420baf64f8e90135dcb02c885010afb5150`
- Candidate product-tree identity:
  `sha256:0fe650a3425c34a787ac2172f6c348e63cf770ac8698a3738dbdd4f0ab96012d`
- Candidate binary patch: 195,661 bytes,
  `sha256:4bd3da199d9172db1254a83c77fc26a0b214cc2b72cd8b528508a288893cf688`
- Plan:
  `sha256:66dd2c09b538b4eb41783128b3c4d110d10d04124a15f46b2d354858b2409d74`
- Approval:
  `github://swornagent/sworn/issues/157#baton-plan-approval-sworn-v0.3.0-baton-rc3-v1`,
  `sha256:35415561708d3421c1272fbebada8e56748166077313ba1b540ec96736bc7602`
- Design:
  `sha256:3457802379f6a66ebd5d70b0ad23ce7fc51251313a053b8618aa9701e0bc7144`
- Design producer:
  `codex:/root/w2-rc3-design-implementer/sworn-v0.3.0-baton-rc3/T2-driver/W2-driver-core/design/3`
- Captain decision: `PROCEED`,
  `codex:/root/w2-rc3-design-captain/sworn-v0.3.0-baton-rc3/T2-driver/W2-driver-core/review/3`
- Implementer invocation:
  `codex:/root/w2-rc3-product-implementer/sworn-v0.3.0-baton-rc3/T2-driver/W2-driver-core/implement/1`

The admitted package identity is version `1.0.0-rc.3`, annotated tag object
`34324784694696a38d951061c2313363b405c1e4`, peeled commit
`affaf16cc37f845b5dc43b22988d8b680ff1f212`, and manifest
`sha256:b58b3c2f844c0c0b07195c3ef5af8c1819871f11b51d697e06207ad0d4b2ec9c`.

## RC2 feasibility rebind

RC2 product bytes were feasibility input only. The three full-index patch
digests were recomputed as:

- `3b30295e6f8d04963266f9b98880df434f9c4f6c`:
  `sha256:aa00493645b22d5e90ec0199ee5e93bc54b8107dc6744286a3cdf4350a43a751`
- `287f83d05dff3540831996553c5610002c92f3da`:
  `sha256:37a2ee63a67e5554d8070cfc110486b3871015cbcf4f31761a08cfac4523bf93`
- `0136a96c4355e60c815b5cab043b54e860d00062`:
  `sha256:ade0486ab9784047026475e80d127ae82382c0f23d2c8254c7204afb4807bd44`

The W0-to-final `internal/driver` patch is
`sha256:9bcaa63667fc4e0ac159439307c82a5c9192de538a7b76203db0898f600b3d52`;
the reconstructed RC2 driver subtree is
`85eed1c35838c241f7edf6a3331cb771c59c66ce`. The raw reconstruction log is
`sha256:b9ebdfd57082b1cf2b1d1be498e5a0b11a2e2c1c4bc656e671a034c93e575ce6`.

No RC2 record, design, proof, `PASS`, status, journal, or history byte was
reused as RC3 acceptance evidence.

## Scope and physical-line bindings

The candidate has one product commit over the exact start. Its 19 changed
paths are all under `internal/driver`; the ordered path manifest is 603 bytes
and has
`sha256:c6e280555e27889b257e708fa26a1dddade52aa5371126aae4ffd3039b8568e2`.
There are no `.baton/releases`, `docs/`, module, configuration, generated,
runtime, journal, Git, CLI, or provider-adapter changes.

| Binding | Exact result | Manifest |
| --- | --- | --- |
| W0 driver stub | 1 file / 3 physical lines | `sha256:24039de96e40042617fd368e61802df7d04af1e5d6911048db6359f02475af51` |
| Final W2 production | 11 files / 3,277 physical lines | `sha256:32a52185a849165094ac0c9484c78f28c2172991d9cdf3fc6e4d5ccd3de6a677` |
| W2 production delta | 11 touched files, 3,276 additions, 2 deletions, net +3,274; 10 new files | `sha256:723ffaf9e20f7b8208dbceab31d415a6417d2379fc6011fd4a631214767a109e` |
| Dependency-complete post-W1 input | 19 production files / 7,190 lines | `sha256:d0cbc444a815a6981cf1af59d24b79f7c42a004d42b477dbd4c72e0e1f3480f0` |
| Post-W1 plus W2 aggregate | 29 production files / 10,464 lines | `sha256:fafbd7b8477560a7610953d99136764a3b7aeee3afa04d72c25c216df2c598af` |

Both dependency compositions were made from record-free archives. The
aggregate-budget raw output has
`sha256:bebb09d1d00fa5393ec4121cbebf9673aa661c11a26346ed8ac056297540f6b6`.

## Contract and authority evidence

- Strict, bounded, canonical request, result, driver-info, selection,
  submission, frame, seal, and permission codecs bind the exact RC3 package,
  invocation, ordered input bytes, operation, explicit model, provider,
  executable, network, workspace, containment, freshness, and responsibility.
- Planner, Implementer, Captain, and Verifier each require one explicit
  configured provider and model. Production Merge dispatch and any default,
  fallback, retry, rotation, or provider orchestration are refused.
- Verifier admission requires both read-only workspace access and fresh
  context before staging or process start.
- A deterministic proof-binding fixture compiled against the exact candidate
  produced request digest
  `sha256:41e4a3032ce33b29972182bdcb64b712c6695f3b7a3bb65a649d37308baf2694`,
  ordered-input digest
  `sha256:278fcc503077b51e68e371efc95d12a267c40d64c8a85296878904ce4a921911`,
  and canonical permission-descriptor digest
  `sha256:2ab6892fa08e9e60e5e89499ac0dde071a1b688e2ce9586c64ddb909258ab798`.
  The overlay fixture did not alter the candidate or worktree. Its raw output
  has
  `sha256:66d9798d0917cb4237b4fa73073b0f6b15257a9885d4c907fc51812b9b648013`.

## Containment, terminal arbiter, and usage evidence

- Every invocation receives a new Bubblewrap process tree with an empty
  inherited environment, fixed guest paths, descriptor-pinned executable,
  workspace and outside-worktree input roots, dropped capabilities, private
  namespaces, reserved-path masks, and shared network only for an explicitly
  admitted non-fake provider policy.
- `/proc` is an empty read-only namespace-owned mount. Host paths are absent
  from request, argv, environment, diagnostics, and guest mount observation;
  `.git`, `.baton`, `.sworn`, and escaping/absolute aliases are unavailable.
- `--info-fd` and `--new-session` provide a trusted contained process identity
  and a separate sandbox process group. Engine stop targets that group, while
  Bubblewrap remains alive to preserve the child's real exit status.
- One serialized arbiter owns stdout, stderr, submission, endpoint faults,
  cancellation, deadline, process exit, post-check, cleanup, event sequence,
  and publication. A first submit closes admission; an accepted seal remains
  pending until exactly one complete bound `completed` result exists.
- Acknowledgement precedes engine stop but publication remains impossible
  until stop classification, complete-tree quiescence, source post-check,
  input cleanup, producer join, and a final synchronous context check.
- Stdout, stderr, result, input, submission, and frame counters are hard
  limits. Overflow terminates and cannot become successful truncation.
- Usage preserves reported zero independently from unavailable tokens and
  cost; partial, negative, unsafe, inferred, estimated, or pricing-derived
  values are refused.

The observed accepted sequence was:

`result_completed → submit_accepted_pending → submit_acknowledged →
engine_stop_after_submit → process_waited → process_group_quiescent →
workspace_postcheck → input_projection_removed → producers_joined →
published`.

The spontaneous-exit canary observed:

`result_completed → submit_accepted_pending → submit_acknowledged →
engine_stop_after_submit → process_waited → fatal:process_failed →
process_group_quiescent → workspace_postcheck → input_projection_removed →
producers_joined`.

It released no handoff. A malformed control frame against a blocking child
terminated in about 38 milliseconds and observed
`fatal:submission_protocol_failed` before wait/quiescence/post-check/cleanup;
it also released no handoff.

The race matrix covers submit-first and result-first acknowledgement,
cancellation-first and publication-first arbitration, timeout before result,
overflow racing submit, rejected/late/conflicting submission, missing,
partial, malformed, duplicate, extra, mismatched, non-completed and post-result
stdout, deliberate exit 17 after acknowledgement, and descendants on normal,
intentional-stop, cancellation, timeout, protocol and overflow paths. The
verbose evidence has
`sha256:858f4689dbad26e65083c75677a327f5b6c6d5bd7adad92f79b57474de48e231`.

## Check evidence

All commands below ran against the exact candidate tree before the single
content-preserving commit; the post-commit tree, product identity, patch,
format and clean-worktree checks re-bound those bytes to the candidate.

| Command/check | Result | Raw output SHA-256 |
| --- | --- | --- |
| `GOFLAGS=-buildvcs=false go test ./internal/driver/...` | exit 0 | `f3659cabbb35fd3bcd00d8f1cf5426de7bcac40447b6bafa7f7982905d8beeb3` |
| `GOFLAGS=-buildvcs=false go test -race ./internal/driver/...` | exit 0 | `f0edd833f0e19fe9a4e63e47956e7bb3ae37319ad92427dcd6a120ac8738f677` |
| `GOFLAGS=-buildvcs=false go test -shuffle=on -count=20 ./internal/driver/...` | exit 0, 20 repetitions | `d13c10d00c937e6beb604f4365d684ed7775838cf58abc33b0f1f7f092d65ff0` |
| `GOFLAGS=-buildvcs=false go test -race -shuffle=on -count=10 ./internal/driver/...` | exit 0, 10 race repetitions | `1c5c73ff6baac209f58c2e31ecba872dbbfce7b4ea6e746c961e6a2c34270f31` |
| `GOFLAGS=-buildvcs=false go test ./...` | exit 0 | `3c29b838aa3b16af3319082b131825fc0b227991057fcf7419d340b7200aa497` |
| `GOFLAGS=-buildvcs=false go test -race ./...` | exit 0 | `afe086975c3925222c22a812419f95883bb5786b7ffe261c5eca34c456d68236` |
| `GOFLAGS=-buildvcs=false go vet ./...` | exit 0 | `ba47fb157f8925f3a0e7e9b8223a20e712d47d063cc6332055f91f6d7da5da0d` |
| `CGO_ENABLED=0 GOFLAGS=-buildvcs=false go build -mod=readonly -buildvcs=false -trimpath -o /tmp/sworn-w2 ./cmd/sworn` | exit 0; binary `sha256:813e48e49cec3d2cb9b7813c3f6c6dfa671bedf48725c36908d54d911cb59c12` | `1eb22c736d282baf1d53ecab938aedf4ed843deed5bcb2f5a708e43b0e6c0956` |
| Two independent record-free sources with fresh build and module caches | byte-identical binaries | `baa103e8704eab4741adad07b64fff4417db4b5a950204c698a9833a9f3df342` |
| Initial authority/history, formatting, `git diff --check`, scope and independent W2 budget | exact | `5c6837170827330a7388250995d6f137326b3565cc2d7655a8dc207aa1d1fa4e` |
| Candidate commit/parent/tree/product/scope/history/clean-worktree rebind | exact | `aea52b3d0c5cd6c547c6b1912a480c6aacf5b52dc4a784884c5d06a3c6accbe6` |
| Post-W1 and aggregate physical-line composition | exact | `bebb09d1d00fa5393ec4121cbebf9673aa661c11a26346ed8ac056297540f6b6` |

A final fresh adversarial implementation audit also ran 20 shuffled targeted
repetitions, 300 fast normal-exit repetitions, and manual session-escaping
descendant and FD/path-leak probes. It found no concrete implementation
blocker; this audit is not a Verifier verdict.

## Not delivered

W5 owns production provider executable-closure discovery, credentials,
capability inspection and certification. W2 does not claim any production
provider/profile runtime closure, scheduler, retry policy, authority mutation,
Git provenance, journal recovery, evaluation, observability, or deployment.
Only the T2 candidate ref was pushed after immutable proof preparation; no
release, dependency, or other-track ref changed.
