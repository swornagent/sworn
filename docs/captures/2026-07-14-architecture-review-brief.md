# Architectural review — brief for a fresh session

**Read this whole file before doing anything.** It is a self-contained commission. It carries
a *learned prior* from the 2026-07-13/14 session that is the single most valuable input you
have, and a catalogue of already-known work you must not duplicate.

**Companion (mandatory):** `docs/captures/2026-07-14-outstanding-work-catalogue.md`.

---

## 0. The commission

A full architectural review of `sworn` (49 packages, ~49k non-test Go LOC, ~58k test LOC),
producing — **in descending order of durability**:

1. **Guards** — populated `architecture.json` rules, lint rules, tests, so each finding
   *cannot recur*.
2. **Fixes** — code, for what is cheap and safe to remediate in place.
3. **Issues** — GitHub issues for what needs planning.
4. **A remediation release plan** — slices, tracks, ordering.
5. **A findings document** — last, and only for what cannot be guarded.
6. **A root-cause and protocol-change essay** (§6 below) — the Coach has asked for this
   explicitly and it is a first-class deliverable, not an appendix.

> **A finding without a guard is a decoration.** This is Baton Rule 12 applied to
> architecture. A review that ends in a document decays — and we have proof: `S08-init-config`
> *wrote down* "API keys in the XDG config file, 0600," and the driver slices *wrote down*
> canonical env names (`ANTHROPIC_API_KEY`, not `SWORN_ANTHROPIC_API_KEY`). **Both were
> recorded design decisions. Both drifted. Nothing caught it**, because nothing was
> machine-checkable. Do not produce another document that drifts.

---

## 1. The hook you are handed

`internal/gate/archrules.go` **exists** — sworn has an architecture-rules gate.

There is **no project-level `architecture.json`**. The gate enforces *nothing* on sworn
itself.

The tool that gates other people's architecture does not gate its own. **Populating that file
with rules derived from your findings is the highest-leverage single output of this review.**

---

## 2. The learned prior — start here, do not start from a generic checklist

On 2026-07-13/14, five defects were found independently. They are **the same defect**:

| Where | The shape |
|---|---|
| Board oracle release-ref probe | **3 duplicated probes**, one of them on the loop's router path |
| `sworn llm-check` model resolution | **4 bespoke resolution paths**; llm-check read a *different env var* from its siblings |
| Role → model assignment | Planner ended in a hardcoded `"openai/gpt-4o"`; **Captain took `escalationModels[0]`** — the cheapest rung of a *retry* ladder — so the Rule 9 design-authority role silently ran on the weakest model |
| Provider credentials | **3 code paths that disagreed.** Two providers canonical, two canonical on *one* path and `SWORN_`-only on the other, the rest `SWORN_`-only |
| Escalation ladder defaults | **2 hardcoded copies** of the same stale model list — and only one was the live path |

**The pattern, stated once:**

> **N places that should be one → they drift → the divergence is silent.**

Its two siblings, both confirmed repeatedly:

> **Silent fallback.** The code substitutes a default/alternative *and then reports the failure
> against the substitute*, not the thing you asked for. (`sworn board` retargeted to `HEAD` and
> reported "not found in HEAD" for a board sitting on `release-wt`.) A hardcoded default is a
> *lie with a shelf life*: `DefaultConfig` pre-filled `openai/gpt-4o-mini`, which meant **no
> "not configured" error could ever fire** — the default quietly answered for the user.

> **Bootstrapping in the caller.** `model.LoadDotEnv()` — which loaded the key file `sworn init`
> *writes* — was called by **exactly one command**. So the loop worked and every sibling failed,
> each resolving a model correctly and then failing for want of a key sitting on disk. A
> bootstrap step owned by a caller is a step every *other* caller can forget, and the one that
> forgets is never the one you test.

**Build your primary lens from this.** A generic "look for duplication" pass would find some of
these. A lens built from a five-times-confirmed pattern will find the rest.

### 2b. Guard fidelity in the tests themselves

Three tests were found **passing for the wrong reason** in one session:

- a test asserting "no model configured → exit 2" that actually read the **developer's real
  `~/.config/sworn/config.json`**, found a model, and exited 2 for a different reason;
- a test asserting "unset keys are empty" that read the developer's **real exported
  `MISTRAL_API_KEY`**;
- a helper named `clearModelsProviderEnv` that cleared **6 of 13** providers and no credentials
  file — a helper whose scope was narrower than its name.

A green test that protects nothing is worse than no test: it converts an unknown into a false
assurance. **Sweep the whole test suite for this class.** Suggested probes: tests that read
`$HOME`, real config paths, or ambient env without `t.Setenv` isolation; assertions broad
enough to pass for the wrong reason (e.g. asserting only an exit code when two different
failures share it); `_test.go` helpers whose coverage is narrower than their name.

---

## 3. The lenses

Run all eight. **Lens 1 is the proven one — weight it accordingly.**

1. **Single-source drift** — N implementations of one concept. Duplicated probes, resolvers,
   config readers, path builders, validators. *Where does the same question get answered in
   more than one place, and do the answers agree?*
2. **Silent fallback / fail-open** — hardcoded defaults that answer for the user; substitutions
   reported against the substitute; validation *after* the write (see #66); guards that return
   green over a domain they never searched.
3. **Layering violations** — bootstrapping in the caller; wire types leaking across boundaries;
   UI doing blocking I/O (see #82); a package reaching around its own abstraction.
4. **Guard fidelity** (§2b) — tests passing for the wrong reason; scope-narrow helpers; guards
   that have never failed.
5. **Cohesion / god objects** — `cmd/sworn/doctor.go` is **1,316 lines**; `internal/run/slice.go`
   is 1,077. Are these cohesive or accreted?
6. **Dead / unreachable code** — already known: #68 (`PauseEngine`), #70 (pricing registry),
   #67 (`example.com` literals). Find the rest. *A component imported only by its own test is a
   red flag* (Baton Rule 1).
7. **Error handling + process-global state (Rule 11)** — swallowed errors; ambient-cwd `git`
   operations; unrestored env/cwd mutation. Known residuals: #63, #64.
8. **Contract drift — code vs its own records.** `docs/release/*/design.md` and `docs/adr/`
   state *intended* architecture. **Today proved they drift.** Treat every design record as a
   *claim to verify against the code*, not a description of it. Note: **ADR-0011 is cited 30+
   times and was never committed** (#79) — do not chase it.

---

## 4. Method

**Fan out.** 49 packages × 8 lenses does not fit one context, and a *sampled* review gives
false assurance — a check narrower than its claim is worse than no check (Rule 12 scope
parity). Use a multi-agent workflow: parallel readers, each carrying **one lens over a
package cluster**.

**Adversarially verify every finding.** Architectural findings are exactly where
plausible-but-wrong lives. A fresh agent, per finding, prompted to **refute** it — default to
"refuted" if uncertain. Do not report a finding that has not survived this.

**Cross-reference before reporting.** Every surviving finding must be checked against
`2026-07-14-outstanding-work-catalogue.md` and against `gh issue list --repo swornagent/sworn
--state open --limit 100` (**81 open issues**). Report it as *new*, *duplicates #N*, or
*extends #N*. **This is not optional**: on 2026-07-14 the `--upstream --tag` SHA-guard bug was
diagnosed and fixed from scratch, and only afterwards found to be **sworn#26, already filed**.

**Rank by blast radius**, not by count. "How many callers can this silently mislead?" beats
"how many occurrences are there?"

---

## 5. Constraints

- **The repo is public.** `scripts/public-safe-scan.sh` runs in CI. Never write a local path
  (`/home/<user>`, `~/projects/...`), the product name, or the consumer repo's name into a
  tracked file. See §6 of the catalogue.
- **Do not duplicate the 12 specced slices** of `2026-07-11-contract-edge-gates` (catalogue §2).
- **Two PRs are open and unmerged** (#105, #106). Read them before reviewing the code they
  touch — `internal/config`, `internal/model`, `internal/project`, `internal/gate/llmcheck.go`
  and the credential layer were substantially rewritten on 2026-07-14 and the fixes are *not on
  `main`*.
- **`main` is the trunk.** `release/v0.1.0` is merged and deleted.
- **Fail closed.** Exit 0 only on PASS. Minimal, justified deps — each new dep needs an ADR.

---

## 6. The essay (a required deliverable, not an appendix)

The Coach has asked for this explicitly. Write it as
`docs/captures/<date>-architecture-review-root-cause.md`.

### A) How did these happen?

Not "what is wrong" — **why did a competent process produce it?** The interesting cases are
the ones where the process *worked* and the defect landed anyway:

- The `SWORN_` prefix was **never designed**. It leaked as an implementation detail, and then
  `S06-loop-dispatch-rewire` **D7** *preserved* it — a decision that was **flagged for Captain
  review and correctly approved**, because removing it would have broken "the documented worker
  setup". A justified compatibility shim, reviewed and acknowledged — **with no tracked
  removal**. Six weeks later it is not a shim, it is the contract. *Rule 2 governs silent
  deferrals; who governs an acknowledged one that nobody scheduled to die?*
- The Captain's model came from `escalationModels[0]`. **Nobody chose that.** It was in scope,
  it worked, it shipped. A design decision encoded as an implementation detail and never
  surfaced for review. And a **green test was actively guarding it** — asserting the
  captain-failure message names the *implementer's* escalation head.
- `S08` said "keys in the XDG config file, 0600". The drivers said canonical env names. **Both
  written down. Both drifted.** A recorded decision with no machine check is a wish.
- **81 open issues, and one was re-discovered from scratch in a day.** A backlog nobody reads is
  a second, slower way of finding the same bug.

### B) What changes to Baton and to Sworn would prevent the pattern?

Be concrete and specific about **which layer owns each fix** — the protocol (Baton) or the
engine (Sworn). Candidate directions, to be argued for or against, not accepted:

- **Baton:** does a rule exist for *"a decision recorded in a design record must be
  machine-checkable, or explicitly marked unenforceable"*? Rule 12 governs guards on *defects*.
  Nothing governs guards on *decisions*.
- **Baton:** Rule 2 covers silent deferrals. An **acknowledged** deferral with no scheduled
  removal (the D7 shim) passes Rule 2 cleanly and still rots. Is that a gap in Rule 2, or a new
  rule?
- **Baton/Sworn:** a *single-source-of-truth* gate. "This concept is answered in N places" is
  mechanically detectable (duplicate probe/resolver shapes, repeated literal model IDs,
  parallel env reads). It is the defect class that recurred **five times in one session** and
  nothing in the twelve rules names it.
- **Sworn:** populate `architecture.json` and turn `archrules.go` on **against sworn itself**.
- **Sworn:** a lint for hardcoded model IDs and hardcoded provider defaults outside a declared
  registry.
- **Process:** the backlog is not being read. What mechanism makes 81 open issues *reachable* at
  the moment a bug is being diagnosed, rather than after it is fixed?

---

## 7. Definition of done

- [ ] Every finding **adversarially verified** by a fresh context, or dropped.
- [ ] Every finding **cross-referenced** against the 81 open issues and the 12 specced slices,
      and labelled *new* / *duplicates #N* / *extends #N*.
- [ ] `architecture.json` populated; `sworn lint` (arch rules) runs green on `main` **and is
      proven to fail** on a deliberate violation (Rule 12 mutation proof — a guard that has
      never failed is a decoration).
- [ ] Guards (lint/tests) for every finding that admits one; each with a recorded mutation
      proof.
- [ ] Issues filed for what needs planning; a remediation release plan drafted.
- [ ] The §6 essay written.
- [ ] `bash scripts/public-safe-scan.sh` passes.
- [ ] A proof bundle at `docs/captures/<date>-architecture-review-proof.md`, generated from
      **live repo state**.
