# ADR 0010: Execute the conformance gate at the host boundary, not inside a worker

- Date: 2026-08-11
- Status: accepted

## Context

Every slice contract written to date carries the check

    GOFLAGS=-buildvcs=false go test -count=1 -parallel=1 -timeout=60m ./test/e2e

and no worker or verifier has ever been able to run it. Two independent
mechanisms prevent it, and neither is accidental:

1. `trustedBubblewrap` requires `/usr/bin/bwrap` to be owned by uid 0. Inside
   a worker sandbox that binary reads as uid 65534, because `--unshare-user`
   with no uid map projects host root onto nobody. The check is correct about
   its own question and simply cannot be satisfied from inside.
2. Workers run with `--disable-userns`, so the kernel refuses to create a
   nested user namespace at all. A probe from inside a live worker returned
   `unshare -Ur -> No space left on device`.

So nested containment is prevented twice, the second time on purpose. Every
nested driver dispatch returns `ISOLATION_UNAVAILABLE`; one run logged 155
occurrences across roles.

The gate has appeared to pass only because verifiers correctly classify the
failures as environmental. That is honest reporting of a check that did not
run. The situation became load-bearing when a contract first required
*empirical* e2e observation (revising four conformance pins): the implementer
could not obtain the evidence, correctly refused to fabricate revisions that
would have corrupted the conformance suite, and escalated. An operator ran the
suite on the host and answered. The release completed, but only because a
human was available to execute a check the loop had declared for itself.

## Decision

**The conformance gate executes at the host boundary. Contracts declare it;
they do not require a sandboxed role to run it.**

Three parts:

1. **Host-boundary execution.** Declared checks that require containment are
   executed by the engine on the host at a defined boundary, with output and
   exit code journaled and bound as evidence. A verifier reads that recorded
   evidence rather than re-running the check. Evidence carries explicit
   provenance so host-executed results are never mistaken for in-worker ones.
   Contracts are human-approved artifacts, so engine execution of their
   declared checks sits at the same trust level as the approved plan.

2. **Test-only uncontained dispatch.** Most end-to-end tests dispatch the fake
   driver in `test/e2e/testdata/fake` and are exercising scheduling, journal
   semantics, exactly-once and crash recovery — not isolation. A test-only
   uncontained dispatch mode lets those run inside a worker sandbox. It is
   gated exactly as the crash hooks are: linked in through ldflags, absent
   from production binaries, unreachable without a deliberate test build.

3. **The suite splits along that line.** Tests that genuinely prove isolation
   keep real containment and run only at the host boundary. The split is a
   consequence of (1) and (2) rather than a taxonomy maintained by hand.

Effective immediately, ahead of the engineering: **no contract may require
in-worker execution or empirical observation of the e2e gate.** A contract
that does is unsatisfiable, and a correct worker will refuse it.

## Rejected alternatives

**Weaken worker containment to permit nesting** — drop `--disable-userns` and
make the trust check namespace-aware. This would restore the full suite to
workers at the cost of granting every model-directed worker the ability to
create user namespaces. The containment boundary is the product; it is not
spent to buy test-execution convenience.

**Formalise operator execution** — declare the gate human-run and supply
results through the attention and answer path. This works, and is what
unblocked the release that produced this ADR, but it does not scale and
reintroduces the human step that recorded delivery exists to remove. It
remains the interim until (1) lands.

## Consequences

- Contract authoring changes now: containment-requiring checks are declared,
  not executed by the role.
- The engine acquires a host-side check runner with bounded output, timeouts
  and explicit provenance. That is new privileged surface and must be scoped
  to checks declared in an approved contract, never to arbitrary input.
- Evidence for a slice may now legitimately come from two places. Both must be
  labelled, and a verifier must be able to tell them apart.
- Workers regain real end-to-end signal on the orchestration majority of the
  suite, which is the part that catches scheduling and recovery regressions.
- Isolation guarantees are unchanged. No production binary gains an uncontained
  dispatch path.
