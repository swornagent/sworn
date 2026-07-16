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
