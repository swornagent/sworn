# R4 contract-identity — release plan (2026-08-24)

Successor to the R4 section of `2026-08-20-issue-audit-and-portfolio.md`,
augmented per Brad's 2026-08-24 ratification of #234 and a fresh sweep of
the open backlog for bundle-ins. Contracts to be authored post-R3-merge
(S3 builds on R3-S7's proposal persistence; all digests bind the post-R3
head).

Root cause the release owns: contract, plan, and receipt identity is
path- and whole-bytes-shaped, and the authoring loop that manages that
identity has no product surface. Every revision so far paid the toll:
path churn (`rev2/`…`rev5/`), hand-mirrored manifests, STALE_BINDING
round-trips, receipts voided by orthogonal edits, scratch tooling
(`tmp/plantool` + per-session `sync_plan.py`) doing the most
safety-critical operator work in the product.

## Core slices (ratified scope)

- **S1 digest-addressed contracts** (#200; `internal/baton` plan/record +
  `internal/runtime` install): a recorded revision carries the contract
  digest as identity, path as provenance. Resolution at dispatch fetches
  bytes by digest from the record tree. `rev*/` path churn disappears;
  canonicalised-digest semantics get documented.
- **S2 plan-authoring surface** (#234; `internal/baton` + `cmd/sworn`):
  derive-don't-mirror — the manifest references contracts by
  path+digest and the engine computes the mirrored facts (waivers,
  depends_on, outcome, touchpoints) at parse/record time. New verbs:
  `sworn plan pin` (recompute + rewrite manifest from contracts),
  `sworn plan lint` (recording-time scope lint, runnable pre-commit),
  `sworn plan record` (RecordPlanRevision with ContractTree=HEAD).
  Retires `tmp/plantool` and `sync_plan.py`. Depends on S1 — the verbs
  speak digest identity natively. Self-retiring scaffold: R4 itself is
  the last release authored with plantool.
- **S3 proposal contract persistence** (#210; `internal/runtime`
  proposal): proposals carry new-contract bytes durably (journal
  command payload beside plan_bytes); install sources contract files
  from the proposal instead of failing CONTRACT_NOT_FOUND against the
  bound target tree. Author against whatever R3-S7 lands — S7 makes
  plan-proposal bytes durable; this extends the same persistence to
  contract files a proposal introduces.
- **S4 receipt identity split** (#218 mechanism 2; `internal/baton`
  receipts): acceptance/scope identity vs checks/evidence identity, so
  a checks-list edit stops voiding design receipts. Ledger note: #218's
  mechanisms 1/4/5 are delivered (#211 ancestry adoption, #216, #215);
  mechanism 3 is S3 + R3-S7. After S4 the umbrella closes with
  evidence.
- **S5 engine-bindings role-asset addendum** (assets + digest chain):
  the three sentences that cost review turns in both instrumented
  releases — canonical digests; before/product_tree are
  invocation-state digests; seal epoch lockstep.

## Bundle-ins (backlog sweep 2026-08-24)

Chosen for bounded size + per-run operator value; grouped so each slice
stays one mechanism.

- **S6 init environment honesty** (#226 + #228; `cmd/sworn` init +
  tests): one mechanism — init must not read host state it shouldn't.
  Product half: darwin skips the Linux sandbox runtime-file preflight
  and states that native dispatch requires Linux (#226, hit on first
  day-job macOS use). Test half: init tests pin the full agent
  environment instead of inheriting host PATH (#228) — kills the
  standing 9-test local env class; stretch: same pinning for the 4
  codex-certification tests so the whole known-env-red class dies.
- **S7 worker-surface truthfulness** (#188 + #189; `internal/driver`
  toolset + containment): one mechanism — the worker-facing surface
  stops lying. Bash accepts `command` as alias for `script` (every
  model reaches for it; paid turns on every run). The `.git` worktree
  mask becomes an empty regular file plus an explicit prompt note
  (today's /dev/null bind shows a character device and burns worker
  turns on theories). Both from the 2026-08-11 dogfood capture
  (F13/F14). Adjacent to R3-S8's correction machinery: S8 corrects
  malformed calls; this removes two classes before correction is
  needed.
  AMENDMENT 2026-08-25 (operator-proposed from the R3 overnight, for
  ratification at contract authoring): S7 should also gain (a) `Read`
  `offset`/`limit` parameters and a batched `{"paths": [...]}` form —
  #236 measured whole-file reads at 93% of a blown 1M window, #188
  documents models sending limit/offset unprompted, and ox-alpha's
  dup-key batching (0/6 tries) is a batching intent with no legal
  syntax; and (b) an environment-facts block in the worker context
  naming the toolchain path (/usr/local/go is off-PATH — #238 rider),
  the .git mask, and the read budget, so workers stop re-deriving
  environment facts by habit. Rationale + evidence:
  docs/captures/2026-08-25-in-engine-learning-spec.md ("reduce the
  failure surface at source").
- **S8 temp-root reaper** (#194; `internal/driver` factory +
  `internal/gitx`): reaper at factory construction copying the
  `reapNativeSessionRoots` pattern; `refs.go` cleanup converted to
  defer. Tiny — drop first if the release needs to shrink.
- **S9 scope-refusal retry floor** (#224; `internal/runtime` cycle
  admission): RULED by Brad 2026-08-24 (this capture's ratification
  session) — option (b)-with-floor: after a seal/scope refusal with a
  succeeded dispatch, the cycle re-dispatches the worker in-cycle with
  the refusal context (named paths) instead of burning t2/t3 as
  WORK_ALREADY_SUCCEEDED admission refusals; on exhaustion the park
  names the scope code. Disjoint seam from S1-S8.

RATIFICATION 2026-08-24: Brad approved the full 9-slice slate (S6, S7,
S8 all ride; #224 ruled in as S9).

## Riders pending rulings

- **#227** degradation-budget calibration for continuation-less
  adapters: asks 1+2 delivered by R2-S5; verify post-R3 (R3-S4 economy
  guards + S6 dialect tolerance may moot the remainder). If the
  miscount stands, small calibration slice.

## Deliberately out (stays on the ladder)

#219 (R5 continuation-economics), #195/#176 (R6 worker-observability),
#202 (R7 navigation), #208/#151 (R8 test-economics), #220/#223
(watching).

## Hygiene beside the release (operator tasks, no slices)

1. **Close-with-evidence sweep** now that R1+R2 are on main
   (b6c82116): R1 scope #169 #172 #173 #177 #196; R2 scope #209 #225
   #229.
2. **Legacy backlog triage**: ~55 pre-greenfield issues (#39–#99 era)
   target the retired board.json/oracle/Rule-gate engine and were
   outside the 2026-08-20 audit's 50. Most are stale or superseded by
   the runtime reset; a dedicated audit-and-close sweep (same
   evidence-comment discipline) clears the noise floor. Overlaps the
   commissioned architecture review's outstanding-work catalogue.
3. **Post-R3 issue batch** (from the R3 handoff queue): file
   exact-bytes transport limit, DSML leak, e2e timing doctrine;
   several may be moot once R3-S6/S8 merge.
