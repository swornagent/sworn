# Proof bundle — fix: `sworn verify` fails closed on empty spec and missing/empty/malformed proof

Fix ID: `cli-verify-fail-closed` (audit findings
`rule-7-verify--cli-verify-vacuous-pass` and `rule-6-proof--verify-cli-proof-fail-open`,
both adversarially verified, severity ADJUSTED to medium). Refs swornagent/sworn#51.

## Scope

`sworn verify` fail-opened on two surfaces:

1. Default (first-pass) path returned `{"verdict":"PASS"}` / exit 0 with an
   absent, empty, malformed, or nonexistent `--proof`, and `RunFirstPass`
   explicitly ignored `ProofPath` (`_ = in.ProofPath`).
2. Agentic path silently discarded the proof read error
   (`proofContent, _ := readFileContent(*proof)`) and dispatched the model with
   an empty/absent proof (and empty spec/diff), spending on a payload that
   should have been gated.

This fix makes the standalone CLI the proof-bundle gate (Rule 6, fail-closed):

- `verify.Input` gains `ProofRequired`; `RunFirstPass` now gates the proof —
  a supplied proof must exist, be non-empty, and (for `.json` bundles) parse as
  JSON, and an absent proof BLOCKs when `ProofRequired` is set.
- `cmd/sworn/verify.go` sets `ProofRequired: true` on the default path, and the
  agentic path validates non-empty spec, non-empty diff, and present/non-empty/
  parseable proof BEFORE creating or dispatching the verifier.

Empty spec was already gated (`first_pass:spec`) on the default path and is now
gated on the agentic path too.

## Files changed

`git diff --name-only 632d4f3` (whole branch, includes the prior fix1 commit):

```
cmd/sworn/verify.go
docs/captures/2026-07-02-fix-verifier-terminal-inconclusive-proof.md
internal/verify/verify.go
internal/verify/verify_agentic_test.go
internal/verify/verify_test.go
```

This fix's files (`git diff --name-only e5e1eff` + new untracked test):

```
cmd/sworn/verify.go
internal/verify/verify.go
internal/verify/verify_test.go
cmd/sworn/verify_test.go   (new)
```

## Test results

`go test -timeout 120s ./internal/verify/... ./internal/run/... ./internal/bench/... ./cmd/sworn/...`:

```
ok  	github.com/swornagent/sworn/internal/verify	(cached)
ok  	github.com/swornagent/sworn/internal/run	4.812s
ok  	github.com/swornagent/sworn/internal/bench	1.375s
ok  	github.com/swornagent/sworn/cmd/sworn	42.341s
```

New tests: `internal/verify/verify_test.go::TestFirstPass_ProofGate` (7 cases:
missing/empty/malformed/required-absent → BLOCKED `first_pass:proof` exit 2;
valid md/json + not-required-absent → PASS); `cmd/sworn/verify_test.go`
(`TestCmdVerify_FailClosed` drives cmdVerify through the CLI dispatch for
no-proof, missing/empty/malformed proof, empty spec → all non-zero;
`TestCmdVerify_WellFormedPasses` → exit 0). `go vet` clean; touched files
gofmt-clean.

Regression check: `internal/run` (RunSlice passes `ProofPath` without
`ProofRequired`; its own `checkProofAbsent` gate runs first, proof.md is
markdown so the JSON check does not apply) and `internal/bench` (RunFirstPass
with empty ProofPath, ProofRequired false → unchanged) both green.

## Reachability artefact

Live through the `sworn verify` CLI (binary built
`go build -buildvcs=false -o bin/sworn ./cmd/sworn`), bogus key, scratch files.

Default path — was PASS/exit 0 at 632d4f3, now BLOCKED/exit 2:

```
A no --proof         → BLOCKED first_pass:proof "no proof bundle provided — fail closed (Rule 6)"  EXIT=2
B empty proof        → BLOCKED first_pass:proof "empty-proof.md is empty"                           EXIT=2
C malformed json     → BLOCKED first_pass:proof "malformed-proof.json is not valid JSON"            EXIT=2
D nonexistent proof  → BLOCKED first_pass:proof "open /nonexistent/proof.json: no such file..."     EXIT=2
E empty spec         → BLOCKED first_pass:spec  "empty-spec.txt is empty"                           EXIT=2
F good spec+diff+proof → PASS                                                                       EXIT=0
```

Agentic path — was silent dispatch (spend) at 632d4f3, now gated before dispatch:

```
E agentic nonexistent proof → "read proof: open /nonexistent/proof.json..."             EXIT=2
F agentic no --spec         → "spec is required and must be non-empty (--spec)..."       EXIT=2
G agentic empty spec        → "spec is required and must be non-empty (--spec)..."       EXIT=2
H agentic no --proof        → "proof bundle is required (--proof) — fail closed (Rule 6)" EXIT=2
I agentic malformed json    → "proof bundle ... is not valid JSON — fail closed (Rule 6)" EXIT=2
```

(The exact live repro commands from both finding files — cases A-E default and
E agentic — now exit non-zero as required.)

## Delivered

- Default `sworn verify` fails closed on absent/empty/malformed/nonexistent
  proof — evidence: `RunFirstPass` proof gate + `ProofRequired`,
  `TestFirstPass_ProofGate`, `TestCmdVerify_FailClosed`, live A-D.
- Default `sworn verify` still fails closed on empty spec, and the agentic path
  now gates empty spec too — evidence: live E (default), F/G (agentic),
  `TestCmdVerify_FailClosed/empty_spec`.
- Agentic `sworn verify` validates spec/diff/proof BEFORE model creation and
  dispatch (no silent proof-read-error swallow, no spend on an empty payload) —
  evidence: `cmd/sworn/verify.go` agentic block, live E/F/G/H/I.
- Well-formed invocation still PASSes (fail-closed, not fail-shut) — evidence:
  live F (default), `TestCmdVerify_WellFormedPasses`.

## Not delivered

- Distinguishing a first-pass PASS from an agentic PASS at the JSON surface
  (e.g. a `"gate":"first_pass"` marker) — the second, separable footgun the
  `cli-verify-vacuous-pass` finding raised. Why: out of scope for the
  fail-closed fix; the verdict is now honestly non-PASS whenever proof/spec is
  missing, which is the security-relevant half. Tracking: audit umbrella
  swornagent/sworn#51. Acknowledgement: recorded here and in the commit body.
- Doc drift the finding also noted (`internal/prompt/implementer.md:94,144`
  missing the "(first-pass)" qualifier; `cmd/sworn/doctor.go` "PROOF-optional"
  stale-marker). Why: unrelated to the fail-closed behavior change; folding
  prose edits into this commit would blur scope (Rule 2 — surfaced, not
  silently deferred). Tracking: swornagent/sworn#51. Acknowledgement: here +
  commit body. (The two stale strings ON the verify flags — `--proof` help and
  `--agentic` help — WERE updated, since they live in the file this fix owns.)

## Divergence from plan

- The fix adds an `Input.ProofRequired` flag rather than making proof
  unconditionally required in `RunFirstPass`. Reason: `RunSlice` and `bench`
  are existing `RunFirstPass` callers that own their own proof handling (or
  deliberately measure spec/diff structure only); an unconditional requirement
  would regress them. The standalone CLI — the fail-open surface — opts in.
