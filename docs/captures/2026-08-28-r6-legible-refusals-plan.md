# R6 legible-refusals — proposed slate (2026-08-28)

Authored from native-lane-honesty's run evidence, for ratification.

## The theme

R5 made the *driver boundary* honest: refusals there carry a typed
Kind, the provider's own words survive sanitization, and a park names
its cause. Every expensive failure in R5's own six-run endgame came
from a boundary that did **not** inherit that treatment.

Three filed issues are one mechanism. A worker — or an operator, or a
CI run — meets a bare failure code with no named field, and the only
way forward is to bisect:

- #245 a rejected submission names nothing, so the worker probes the
  validator with a minimal payload, and the correction loop seals the
  probe as the authoritative design, implementation, or decision. Six
  placeholder seals across four runs, including a captain PROCEED
  whose entire content was `probe: minimal submission to isolate field
  validation`.
- #250 a recovery decision is refused AUTOMATION_BINDING_MISMATCH on a
  binding the worker reports copying verbatim, with nothing naming
  which field disagreed.
- #251 five distinct Bash-sandbox failure modes emit one bare
  PROCESS_START_FAILED, so each of four CI occurrences cost a round
  just to guess the mechanism.

Beside those sit the two defects that cost the most autonomy: work
that cannot be re-sealed (#246), and a guard that silently skips
(#249). Both are about the loop being able to finish honestly without
an operator.

## Proposed slices

**S1 sandbox refusal detail (#251)** — the five bare
PROCESS_START_FAILED sites (`newToolSession` scratch create and
surfaces; `runToolBash` mask open, status pipe, exec start) each carry
a named detail in the established idiom, and the underlying errno
survives in a bounded, secret-free form. R5-S6 is what makes that
safe: sanitization no longer scrubs it. Additive only, no behaviour
change. **Lands before fired dogfood** — dogfood will hammer this
sandbox on an unfamiliar host, where the class is far harder to
diagnose than in CI. Leading hypothesis to confirm or kill once named:
concurrent user-namespace creation on a two-core runner, since `go
test` runs packages in parallel and several spawn sandboxes. Ruled out
already: mask-fd leak, and prepare.go's RemoveAll.

**S2 submission refusals name the refused field (#245)** — the
validator tells the worker which field or bound refused, so there is
nothing to bisect. A content floor stays as defence in depth (the R5
operator patch is the seed), and the loop must never seal a payload
whose own summary declares it a probe. The models label these
honestly; today the engine ignores its own record.

**S3 re-seal without a code change (#246)** — a remediation whose
prior failure was receipt-scoped can re-seal an unchanged candidate,
or receipt-quality failures route to a re-seal rather than a
re-implementation. Today the implementer has nothing to edit, every
try dies EMPTY_CANDIDATE, and the run parks on work that is already
correct. This is the only defect in the set that can permanently
strand verified-good work; in R5 it cost a track reset and 734 lines.

**S4 automation binding legibility (#250)** — log the request-side
binding beside the validator's expectation, name the disagreeing
field, and fix whatever the diagnosis shows. The recovery lane's
escalation path is currently unusable.

**S5 verification integrity (#249)** — a test that guards a named
acceptance criterion must not skip silently; skips become loud where
they matter, and the verifier shows provenance for its check evidence
rather than only results. This is the meta-defect: the certify
regression (#248) shipped unguarded because the only nearby test keys
off a vendor CLI present on one operator host. Fourth instance of the
class across R3 and R5, and the one with compounding value — it
protects every future release.

**S6 attested operator answers (#247)** — an operator answer reaches a
worker as bare recovery text it cannot distinguish from injected data,
so a well-designed worker correctly refuses security-relevant values
supplied that way (it did, twice, in R5). Give answers provenance the
worker can check, or an operator channel that supplies a file with a
verifiable digest. Carries the authoring rule that any value a worker
must transcribe belongs in contract bytes in full.

Dependencies: none between S1-S6 except that S2 and S4 both extend the
S4-taxonomy vocabulary R5 shipped, and neither blocks the other. S1 is
sequenced first for the dogfood reason, not a technical one.

## What this deliberately excludes

Learning-spec phases 3-4 (the capability/outcome graph and soft-prior
routing). Phases 1-2 shipped in R5; phases 3-4 are the actual
in-engine learning and the spec itself says they want the literature
to inform the memory and retrieval structure before contracts are
authored. Not hardening — new conceptual risk. They should not ride a
release whose theme is finishing R5's honesty work.

## Backlog state after R5 (20 open)

Ratified but unstarted: fired dogfood (checklist ready, gated on S1
here); #219 verifier session continuity, slotted behind R5; #108
guard-first architecture review, commissioned for a fresh session;
#202 LSP-mux navigation seam.

Measured and waiting: #236 read economics; #238 codex sandbox has no
Go toolchain; #235 operator-surface narrowed tails; #208 test
economics; #195 tool results reach no observer; #223 CI provisioning
hang (adjacent to S1 here, distinct mechanism).

Long tail: #176 provider-neutral live output; #151 release hygiene;
#8 prompt caching; #1 Windows build matrix.

Closed with evidence on R5's delivery: #220 #237 #239 #241 #242 #243,
plus #248 fixed as a rider.
