# Live validation output

All commands were run in the assigned T1 track worktree. None invokes a local
Baton installation sync or writes an install target.

## Build

```text
$ make build
go build -ldflags "-s -w -X main.version=0.1.0" -o bin/sworn ./cmd/sworn
exit: 0
```

## Uncached repository suite

```text
$ go test ./... -count=1
ok  github.com/swornagent/sworn/cmd/sworn 107.779s
ok  github.com/swornagent/sworn/internal/account 10.265s
ok  github.com/swornagent/sworn/internal/adopt 0.014s
ok  github.com/swornagent/sworn/internal/agent 0.035s
ok  github.com/swornagent/sworn/internal/baton 49.639s
ok  github.com/swornagent/sworn/internal/baton/schemas 0.004s
ok  github.com/swornagent/sworn/internal/bench 1.211s
ok  github.com/swornagent/sworn/internal/board 0.165s
ok  github.com/swornagent/sworn/internal/captain 0.028s
ok  github.com/swornagent/sworn/internal/command 0.005s
ok  github.com/swornagent/sworn/internal/config 0.022s
ok  github.com/swornagent/sworn/internal/credentials 0.013s
ok  github.com/swornagent/sworn/internal/db 1.412s
ok  github.com/swornagent/sworn/internal/design 0.014s
ok  github.com/swornagent/sworn/internal/designaudit 0.021s
ok  github.com/swornagent/sworn/internal/designfit 0.021s
ok  github.com/swornagent/sworn/internal/driver 2.042s
ok  github.com/swornagent/sworn/internal/driver/drivertest 0.053s
ok  github.com/swornagent/sworn/internal/driver/inprocess 0.221s
ok  github.com/swornagent/sworn/internal/driver/registry 0.029s
ok  github.com/swornagent/sworn/internal/ears 0.010s
ok  github.com/swornagent/sworn/internal/gate 0.094s
ok  github.com/swornagent/sworn/internal/git 0.344s
ok  github.com/swornagent/sworn/internal/implement 0.687s
ok  github.com/swornagent/sworn/internal/journey 0.034s
ok  github.com/swornagent/sworn/internal/ledger 0.015s
ok  github.com/swornagent/sworn/internal/lint 0.131s
ok  github.com/swornagent/sworn/internal/mcp 0.133s
ok  github.com/swornagent/sworn/internal/memory 1.390s
ok  github.com/swornagent/sworn/internal/model 2.241s
ok  github.com/swornagent/sworn/internal/orchestrator 0.002s
ok  github.com/swornagent/sworn/internal/project 0.025s
ok  github.com/swornagent/sworn/internal/prompt 0.013s
ok  github.com/swornagent/sworn/internal/reqvalidate 0.019s
ok  github.com/swornagent/sworn/internal/reqverify 0.013s
ok  github.com/swornagent/sworn/internal/router 0.072s
ok  github.com/swornagent/sworn/internal/rtm 0.015s
ok  github.com/swornagent/sworn/internal/run 6.341s
ok  github.com/swornagent/sworn/internal/scheduler 0.188s
ok  github.com/swornagent/sworn/internal/spec 0.005s
ok  github.com/swornagent/sworn/internal/specquality 0.015s
ok  github.com/swornagent/sworn/internal/state 0.025s
ok  github.com/swornagent/sworn/internal/style 0.004s
ok  github.com/swornagent/sworn/internal/supervisor 1.155s
ok  github.com/swornagent/sworn/internal/telemetry 0.469s
ok  github.com/swornagent/sworn/internal/templates 0.003s
ok  github.com/swornagent/sworn/internal/tracklog 0.004s
ok  github.com/swornagent/sworn/internal/tui 2.182s
?   github.com/swornagent/sworn/internal/verdict [no test files]
ok  github.com/swornagent/sworn/internal/verify 0.025s
exit: 0
```

## Vet and whole-tree equality backstop

```text
$ go vet ./...
exit: 0

$ git diff --exit-code e61cb190736ee7483fb4ed1a993442b26ce3574c HEAD -- . ':(exclude)docs/release/2026-07-15-baton-v0.15-conformance/**'
exit: 0
```

## Built binary reachability (sanitized)

The capability listing proves that the built public binary reaches its command
dispatch successfully. Provider-availability lines are retained only at the
level needed for this proof so the record does not expose local configuration
state.

```text
$ ./bin/sworn capabilities
registered drivers (resolution: explicit prefix -> driver, no fallback):

claude-subprocess
  prefixes:  claude-cli/
  roles:     implementer,verifier
  available: yes — binary "claude" found on PATH; login not probed

codex-subprocess
  prefixes:  codex/
  roles:     implementer,verifier
  available: yes — binary "codex" found on PATH; login not probed

oai-inprocess
  provider availability: omitted to avoid recording local configuration state

oai-responses-inprocess
  provider availability: omitted to avoid recording local configuration state
exit: 0
```

## Deterministic repair revalidation

All repair commands ran from the assigned T1 worktree after checker checkpoint
`c6842d8db3404843fdc8441a8cfefa41c03bd917`. No local Baton installation,
production code, test harness, S20 record, or release ref was written.

```text
$ make build
exit: 0

$ go test ./... -count=1
exit: 0

$ go vet ./...
exit: 0

$ git diff --exit-code e61cb190736ee7483fb4ed1a993442b26ce3574c HEAD -- . ':(exclude)docs/release/2026-07-15-baton-v0.15-conformance/**'
exit: 0

$ proof/check-rollback.sh --head 4b38887e666f7e4ab664bac4780535b080ad54eb --require-maintainability --require-proof-bundle
CONTRACT_AMENDMENT PASS schema=b62d48f698059fc0151ea0a3b9da18dfe1e507f5 record=9e298676129ee628714ffa80caa8c02bcea244f7
S19_SPEC_HISTORY PASS first=c0d7d672fe14090655fea7db3f5bf0e22dfd29f9 second=2c25021305b62d4b1e1f75bf1c7e0e6db504651b
RENDERED_INDEX PASS head=c6842d8db3404843fdc8441a8cfefa41c03bd917
ROLLBACK_CHECK PASS
ENVELOPE_PATHS 45 baseline-present=37 baseline-absent=8
exit: 0

$ sworn lint coverage --slice S19-s02-v015-rollback --release 2026-07-15-baton-v0.15-conformance --base 640396fa8cc319229d6f96dedfdbef65dbe317fe --json
sworn lint coverage: coverage: scan internal/baton/install_transaction_test.go: open internal/baton/install_transaction_test.go: no such file or directory
exit: 2
```

The coverage scanner non-pass is deliberate and bounded: it assumes a
persistent non-release test path that this rollback must leave absent. The
planner-ratified `proof/contract-amendment.json` instead names the committed
checker as the required executable integration proof and prohibits creating or
restoring a persistent non-release harness. The result is recorded rather than
treated as coverage success.

## First-parent S02 record-history repair

All commands in this section ran in the assigned T1 track worktree after repair
commit `6558bbe522e7e1da8039cd0da966da4c2b560a16`. The only persistent changes
are the checker and S19 lifecycle/proof records. The two adversarial histories
were disposable detached worktrees and were removed after each run.

```text
$ bash -n docs/release/2026-07-15-baton-v0.15-conformance/S19-s02-v015-rollback/proof/check-rollback.sh
exit: 0

$ proof/check-rollback.sh --head 4b38887e666f7e4ab664bac4780535b080ad54eb --require-maintainability --require-proof-bundle
S02_RECORD_HISTORY PASS first-parent-commits=19
CONTRACT_AMENDMENT PASS schema=b62d48f698059fc0151ea0a3b9da18dfe1e507f5 record=9e298676129ee628714ffa80caa8c02bcea244f7
S19_SPEC_HISTORY PASS first=c0d7d672fe14090655fea7db3f5bf0e22dfd29f9 second=2c25021305b62d4b1e1f75bf1c7e0e6db504651b
RENDERED_INDEX PASS head=6558bbe522e7e1da8039cd0da966da4c2b560a16
ROLLBACK_CHECK PASS
ENVELOPE_PATHS 45 baseline-present=37 baseline-absent=8
RELEASE_RECORD_CHANGES_AFTER_S19_START 14
MAINTAINABILITY_BINDING PASS head=4b38887e666f7e4ab664bac4780535b080ad54eb report=docs/release/2026-07-15-baton-v0.15-conformance/S19-s02-v015-rollback/reports/maintainability/implementer-cycle-0-73909151-0ead-4f0b-8cca-c1b4e78e6fdf.json
PROOF_BUNDLE_BINDING PASS path=docs/release/2026-07-15-baton-v0.15-conformance/S19-s02-v015-rollback/proof.json
exit: 0

$ make build
go build -ldflags "-s -w -X main.version=0.1.0" -o bin/sworn ./cmd/sworn
exit: 0

$ go test ./... -count=1
ok  github.com/swornagent/sworn/cmd/sworn 169.978s
ok  github.com/swornagent/sworn/internal/account 10.257s
ok  github.com/swornagent/sworn/internal/adopt 0.031s
ok  github.com/swornagent/sworn/internal/agent 0.039s
ok  github.com/swornagent/sworn/internal/baton 43.058s
ok  github.com/swornagent/sworn/internal/baton/schemas 0.004s
ok  github.com/swornagent/sworn/internal/bench 1.237s
ok  github.com/swornagent/sworn/internal/board 0.172s
ok  github.com/swornagent/sworn/internal/captain 0.030s
ok  github.com/swornagent/sworn/internal/command 0.006s
ok  github.com/swornagent/sworn/internal/config 0.026s
ok  github.com/swornagent/sworn/internal/credentials 0.028s
ok  github.com/swornagent/sworn/internal/db 1.228s
ok  github.com/swornagent/sworn/internal/design 0.015s
ok  github.com/swornagent/sworn/internal/designaudit 0.023s
ok  github.com/swornagent/sworn/internal/designfit 0.021s
ok  github.com/swornagent/sworn/internal/driver 1.999s
ok  github.com/swornagent/sworn/internal/driver/drivertest 0.049s
ok  github.com/swornagent/sworn/internal/driver/inprocess 0.225s
ok  github.com/swornagent/sworn/internal/driver/registry 0.029s
ok  github.com/swornagent/sworn/internal/ears 0.010s
ok  github.com/swornagent/sworn/internal/gate 0.095s
ok  github.com/swornagent/sworn/internal/git 0.360s
ok  github.com/swornagent/sworn/internal/implement 0.695s
ok  github.com/swornagent/sworn/internal/journey 0.039s
ok  github.com/swornagent/sworn/internal/ledger 0.018s
ok  github.com/swornagent/sworn/internal/lint 0.135s
ok  github.com/swornagent/sworn/internal/mcp 0.135s
ok  github.com/swornagent/sworn/internal/memory 1.234s
ok  github.com/swornagent/sworn/internal/model 2.131s
ok  github.com/swornagent/sworn/internal/orchestrator 0.004s
ok  github.com/swornagent/sworn/internal/project 0.033s
ok  github.com/swornagent/sworn/internal/prompt 0.014s
ok  github.com/swornagent/sworn/internal/reqvalidate 0.020s
ok  github.com/swornagent/sworn/internal/reqverify 0.013s
ok  github.com/swornagent/sworn/internal/router 0.064s
ok  github.com/swornagent/sworn/internal/rtm 0.013s
ok  github.com/swornagent/sworn/internal/run 6.254s
ok  github.com/swornagent/sworn/internal/scheduler 0.186s
ok  github.com/swornagent/sworn/internal/spec 0.004s
ok  github.com/swornagent/sworn/internal/specquality 0.018s
ok  github.com/swornagent/sworn/internal/state 0.024s
ok  github.com/swornagent/sworn/internal/style 0.003s
ok  github.com/swornagent/sworn/internal/supervisor 1.042s
ok  github.com/swornagent/sworn/internal/telemetry 0.469s
ok  github.com/swornagent/sworn/internal/templates 0.002s
ok  github.com/swornagent/sworn/internal/tracklog 0.005s
ok  github.com/swornagent/sworn/internal/tui 2.052s
?   github.com/swornagent/sworn/internal/verdict [no test files]
ok  github.com/swornagent/sworn/internal/verify 0.021s
exit: 0

$ go vet ./...
exit: 0

$ git diff --exit-code e61cb190736ee7483fb4ed1a993442b26ce3574c HEAD -- . ':(exclude)docs/release/2026-07-15-baton-v0.15-conformance/**'
exit: 0
```

The historical verifier's first alleged bypass used a pinned implementation
head. AC-03 instead requires an adversarial descendant head. The contract-correct
case was exercised in a detached disposable worktree:

```text
$ proof/check-rollback.sh --head bdef578b3fce9e7327dad448704531c870724c91 --require-maintainability --require-proof-bundle
ROLLBACK_CHECK FAIL: unexpected later ordinary authority 4b38887e666f7e4ab664bac4780535b080ad54eb; only the pinned implementation head may restore semantics
exit: 1

$ cleanup /tmp/s19-ac03.dPRkZf
AC03_DISPOSABLE_CLEANUP PASS refs_unchanged=true worktree_removed=true
```

The accepted AC-04 defect is a byte-restoring S02-record history. A detached
commit `6d9dce04c8737579ab48530adc298dda3c9c54e8` added a marker to the S02
journal and descendant `524f1b443209403c1dd463439b8e70afd28ed266` restored its
final bytes exactly. The final-tree backstop therefore passed, but the new
first-parent transition guard rejected the history:

```text
$ git diff --exit-code 6558bbe522e7e1da8039cd0da966da4c2b560a16 524f1b443209403c1dd463439b8e70afd28ed266 -- docs/release/2026-07-15-baton-v0.15-conformance/S02-v015-parity-and-installs/
exit: 0

$ proof/check-rollback.sh --head 4b38887e666f7e4ab664bac4780535b080ad54eb --require-maintainability --require-proof-bundle
ROLLBACK_CHECK FAIL: S02 release record transition on T1 first-parent history at 6d9dce04c8737579ab48530adc298dda3c9c54e8: M docs/release/2026-07-15-baton-v0.15-conformance/S02-v015-parity-and-installs/journal.md;
exit: 1

$ cleanup /tmp/s19-s02-history.0dcTEt
S02_HISTORY_DISPOSABLE_CLEANUP PASS refs_unchanged=true worktree_removed=true
```

The existing propagation merge `c7d56c10f62c5583b5aeb27fda5aa9c8de50b81d`
illustrates why the guard compares only parent one: its S02 records match parent
one exactly, while parent two differs by propagated S02 release records. The
live 19-transition baseline above proves this legal first-parent history passes.

```text
$ git diff --no-ext-diff 640396fa8cc319229d6f96dedfdbef65dbe317fe | sworn verify --spec docs/release/2026-07-15-baton-v0.15-conformance/S19-s02-v015-rollback/spec.json --diff - --proof docs/release/2026-07-15-baton-v0.15-conformance/S19-s02-v015-rollback/proof.json
{
  "verdict": "PASS",
  "rationale": "",
  "cost_usd": 0
}
exit: 0
```
