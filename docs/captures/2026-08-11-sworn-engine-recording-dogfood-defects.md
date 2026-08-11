# Sworn engine-recording dogfood defects

Date: 2026-08-11
Source: releases `2026-08-09-provider-capability` (runs r1-r14) and
`2026-08-10-engine-recording` (runs r1-r5), driven with real models
(DeepSeek implementer/captain/planner, Qwen3.8-max verifier).
Full narrative and evidence: `.baton/2026-08-10-provider-capability.handoff.md`
addenda 2-14.

These are defects observed in live delivery, not review findings. Each is
stated with its root cause and a fix sketch. F8, F9 and F10 were fixed
during the run and recorded by `2026-08-10-engine-recording`; the rest are
open.

## Fixed and recorded

### F8 - worker sandbox wiped state between every tool call

Every `Bash` tool call ran in a fresh bwrap: `--dir /home/sworn` and
`--tmpfs /tmp` were per-command, so nothing survived between calls except
the workspace bind. `GOCACHE` died every command, and every `go test`
recompiled from scratch. Workers had been silently coping with single
mega-commands and in-workspace `tmp/`.

**Fix (recorded, S2-worker-harness):** one host scratch directory per
invocation, rw-bound at `/home/sworn` and `/tmp` for every command of that
invocation, destroyed at session close. Isolation holds at the invocation
and role boundary, never between consecutive commands of one worker.

### F9 - a failing command discarded its own output

A non-zero exit returned bare `error:PROCESS_FAILED` and threw away stdout,
stderr and the exit code, so a worker could not see why its own command
failed. A `timeout`-wrapped test tripping rc=124 lost everything.

**Fix (recorded, S2-worker-harness):** non-zero exit returns bounded
combined output plus the exit code as a failed tool result. Only starts,
kills and overflows remain contract errors.

### F10 - exact bytes could only leave the sandbox through the model

`sworn_submit` required plan/checks evidence as a base64 `ExactBytes`
triplet, which the model had to emit token by token. A live Qwen PASS
verdict was rejected `INVALID_EXACT_BYTES` after drifting 17 bytes while
transcribing ~7.5KB of base64; the declared digest was correct and the
payload was not.

**Fix (recorded, S2-worker-harness):** plan/checks accept
`{byte_count, digest, path}`. The engine reads the file from the workspace,
projected inputs or invocation scratch and verifies the digest itself;
symlink escapes are refused. The commitment is unchanged - the digest still
binds - but an LLM is no longer the wire for high-entropy blobs.

## Open

### F2 - intermittent CONTINUATION_INVALID at accept, still unnamed

Roughly half of long first tries die at accept with `CONTINUATION_INVALID`;
a fresh try usually completes. Three occurrences in `2026-08-10-engine-
recording` r5 alone (S3 implementation t1, assembly verification t1), all on
large-context turns. One occurrence directly followed a malformed tool-call
emission (see F13).

**Instrumentation gap blocking diagnosis:** `accept-site-1..8` labels were
added specifically so the next occurrence would name its own failure site.
They are present in the code (eight `liveStream.driverError` call sites,
confirmed by an independent verifier) but reach neither the live log nor the
journal at this failure path - the journal carries only the bare code. Until
the label surfaces, F2 cannot be localised. **This is the cheapest unblock
in the list and should be done first.**

### F12 - temp husks from processes that die before their defers run

832 empty `/tmp/sworn-driver-certification-v1-<numeric>` roots (496KB),
dated across three days of runs. `ProductionDriverFactory.Close()` removes
its root via `defer`, which never runs under SIGKILL, OOM or reboot - all of
which happened repeatedly. Inode pollution rather than a space problem.

A second, narrower gap: `internal/gitx/refs.go` cleans its
`sworn-ref-transaction-*` directory by explicit `os.RemoveAll` at each
return rather than a `defer`, so an unrecovered panic between creation and
return leaks it. Every other production temp site is defer-cleaned.

**Fix sketch:** a reaper at driver-factory construction mirroring
`reapNativeSessionRoots` (`native_linux.go`), which already sweeps stale
roots from prior runs and parks roots it cannot remove - that is the pattern
to copy. Convert `refs.go` to a `defer`. If the engine ever owns a Go cache,
create it with `GOFLAGS=-modcacherw`: module directories are 0555, so naive
`rm -rf` silently no-ops on exactly the directories worth reclaiming.

Not to be confused with the GB-scale `/tmp/sworn-w*` directories on the same
machine: those carry `mktemp`'s mixed-case suffix, contain per-worker
`GOMODCACHE` trees, and come from an external bash coach-loop harness that
lives in no repo here. The Go engine never sets `GOCACHE` or `GOMODCACHE`;
it only reads the host `GOMODCACHE` to read-only bind it.

### F13 - the Bash tool's parameter name fights universal convention

Sworn's `Bash` tool takes `{"script": ...}` with a closed schema. Every
model tested reaches for `{"command": ...}` first, because that is the
near-universal spelling. Observed twice as well-formed JSON with the wrong
key, and once as a garbled emission immediately preceding an F2 accept
failure. Same class as an earlier finding where a model sent Claude-style
`limit`/`offset` to `Read`.

**Fix sketch:** accept `command` as an alias for `script`. Pure friction
removal; costs every worker turns on every run.

### F14 - a masked `.git` appears as a character device

In a worktree `.git` is a pointer *file*, so the containment mask takes the
regular-file branch and binds `/dev/null` over it. The worker sees a
character device and reasons at length that "git operations are routed
through some special mechanism ... a git daemon style device" before
concluding git cannot work.

**Fix sketch:** mask with an empty regular file, or state the masking
explicitly in the model prompt.

### F15 - the e2e conformance gate cannot run inside any sandboxed role

Inside a worker sandbox `/usr/bin/bwrap` appears owned by uid 65534
(nobody); on the host it is uid 0. `--unshare-user` with no uid map projects
root to nobody, and `trustedBubblewrap()` requires uid 0, so every nested
driver dispatch returns `ISOLATION_UNAVAILABLE`. Nested containment is
structurally impossible, not merely unavailable. One run logged 155
occurrences across roles.

**Consequence:** the check
`go test -count=1 -parallel=1 -timeout=60m ./test/e2e`, present in every
slice of every release, is not executable by any worker or verifier. It has
only ever "passed" because verifiers correctly classify the failures as
environmental. Any acceptance criterion requiring empirical e2e observation
is unsatisfiable by construction - `2026-08-10-engine-recording` S3/A4 was
the first to demand it and the implementer correctly refused to fabricate a
result, escalating instead.

**Fix sketch (design decision needed):** either give sandboxed roles a
trusted containment path, or make the e2e gate an operator/host-boundary
check that slices declare rather than execute. Until decided, contracts must
not require in-worker e2e observation.

### F16 - a `blocked` yield burns retry budget before the lane parks

An honest `blocked` yield does not park cleanly. Turn recovery raises
`INVALID_CONTINUATION` (`internal/runtime/turn_recovery.go:1147,1196,1219,
1236`) and the try dies `operational_failed`. In r5 the first two tries were
consumed this way; only the third surfaced an answerable attention item. The
work is rebuilt from scratch on each try and nothing is sealed until the
end, so correct work is repeatedly discarded - the S3 guard was written
correctly three times in one night and survived nowhere.

**Fix sketch:** a `blocked` or `question` yield should open the attention on
the first occurrence, without consuming a try.

### F17 - control commands drive the run in the caller's foreground

`sworn answer` and `sworn takeover` do not record a control and return; they
block for the entire duration of the resumed work, which can be hours. Any
caller that pipes the output, wraps it in `timeout`, or runs it from a
session with a deadline orphans the run mid-dispatch. Both failure modes
were hit in one night (SIGPIPE from `| head`, then a `timeout` wrapper),
each costing a takeover cycle. Same defect class as the TUI's `start`.

**Fix sketch:** separate "record the control" from "drive the run".
`answer` contributes data and should return once the answer is durable.
`takeover` legitimately becomes the driver - the owner lease *is* the right
to execute - but the driving belongs in a resident engine rather than in
whichever shell typed the command.

### F18 - takeover is silently gated on owner-lease expiry

A takeover fired while the dead owner's lease has not yet expired returns
`s.Status(...)` - exit 0, a status dump, no explanation
(`internal/runtime/service.go`, the `OWNER_ACTIVE` branch). `resume` gets an
explicit `OWNER_TRANSITION_PENDING` in the same situation. The asymmetry
cost two failed attempts and a wrong hypothesis about control generations.

**Fix sketch:** give takeover the same explicit signal, naming the wait.

## Cross-cutting: the observability gap that produced this list

Every defect above was found by hand-reading raw worker transcripts. The
engine renders provider *output* deltas only - turn boundaries, reasoning,
tool calls, per-turn usage. **Tool results are never rendered**: they are
engine-side inputs to the next request. An observer sees intent and action
but not evidence, and watches the model react to output it cannot see.

This is the precondition for intervention by a human, a Captain, or a future
orchestrator, and it does not scale as hand-reading. See
`.baton/2026-08-10-provider-capability.handoff.md` addenda 13-14 for the
turn-feed and global-surface design notes, including the two synergies worth
keeping: emit at the F9 formatting seam so model and observer receive
byte-identical content, and use F8's per-invocation scratch as the
full-fidelity store behind a bounded event payload.
