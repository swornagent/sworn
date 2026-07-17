# Proof Bundle: `S02-v015-parity-and-installs`

Generated from live repository state on 2026-07-16. The machine-readable twin
is `proof.json` (`proof-v1`).

## Scope

Pin SwornAgent's binary, vendored protocol bytes, offline installer authority,
and supported Codex and Claude managed trees to exact Baton v0.15.1 with
fail-closed transactional repair and exact-schema consumer compatibility.

## Files changed

The immutable slice base is
`e61cb190736ee7483fb4ed1a993442b26ce3574c`.

`git diff --name-only e61cb190736ee7483fb4ed1a993442b26ce3574c..HEAD`
contains 75 paths: 45 semantic implementation paths; the ratified release and
planning records needed to repair the bootstrap provenance; the immutable
preflight, orphan, authoritative closure, claim and receipt evidence; and this
proof bundle. The exact path array is recorded in `proof.json` and reproduces
the live Git set byte-for-byte.

## Test results

```text
$ make build                                                        # exit 0
$ go vet ./...                                                       # exit 0
$ go test ./internal/baton -run '^TestBatonV015CodexAndClaudeMirrorParity$' -count=1
                                                                    # exit 0
$ go test ./internal/baton -count=1                                 # exit 0; 322.281s
$ go test ./internal/gate ./internal/run ./internal/state -count=1 # exit 0
$ go test ./... -count=1                                            # exit 0; all packages
$ bin/sworn baton diff /home/brad/projects/baton                    # exit 0
$ bin/sworn baton vendor /home/brad/projects/baton --check          # exit 0
$ isolated disposable-home doctor --sync-baton run 1                # expected exit 2; repaired
$ isolated disposable-home doctor --sync-baton run 2                # exit 0; exact/idempotent
$ bin/sworn lint ac 2026-07-15-baton-v0.16-conformance              # exit 0; 109/109
$ bin/sworn lint trace 2026-07-15-baton-v0.16-conformance           # exit 0; 11/11 needs
$ bin/sworn lint coverage --slice S02-v015-parity-and-installs \
    --release 2026-07-15-baton-v0.16-conformance \
    --base e61cb190736ee7483fb4ed1a993442b26ce3574c                 # exit 0; 5/5 ACs
$ bin/sworn reqvalidate 2026-07-15-baton-v0.16-conformance          # exit 0; 18/18
$ bin/sworn designfit 2026-07-15-baton-v0.16-conformance            # exit 0
$ bin/sworn specquality 2026-07-15-baton-v0.16-conformance          # exit 0
$ bin/sworn designaudit .                                           # exit 0; non-UI exempt
$ git diff --check                                                  # exit 0
```

The replacement claim, full report, receipt and lifecycle status also passed
duplicate-key rejection, Draft 2020-12 schemas, exact Baton plus Sworn report
overlays, token-hash reproduction, committed report-blob equality, invocation
equality, all four sealed digest comparisons, exact scope-union comparison, and
independent `baton-maintainability-v1` fingerprint reproduction.

## Reachability artefact

- **Type:** CLI runs through the built public binary.
- **Read-only parity:** `bin/sworn baton diff /home/brad/projects/baton` and
  `bin/sworn baton vendor /home/brad/projects/baton --check` both exited 0 and
  reported exact embedded/vendor parity.
- **Repair path:** from a clean temporary Git repository with disposable,
  physically disjoint `AGENTS_HOME`, `CODEX_HOME`, `CLAUDE_HOME`, `SWORN_HOME`
  and `HOME`, `bin/sworn doctor --sync-baton` repaired all three managed trees
  and Sworn VERSION sentinels with expected exit 2; the immediate second run
  exited 0 and reported the Codex and Claude mirrors exact.
- **Integration guard:**
  `cmd/sworn/doctor_test.go:TestDoctorAndBatonDiffV015BinaryReachability`
  drives these public entry points across exact, repair, rollback and
  recovery-only outcomes. No real user home was read or written.

## Maintainability Implementer closure

- **Operation:** exact Baton v0.15.1 `maintainability-review`, cycle 0,
  Implementer `closure`, temperature 0, fresh no-history judgement.
- **Reviewed semantic boundary:** immutable base `e61cb190…` through exact head
  `2a17443d67d39cf681dba117a57673714a916d7f`; 45 included and 20 excluded
  paths.
- **Canonical fingerprint:**
  `sha256:c72341fa8bab5c4a9b7a548b7ffb3ba1d57955f5e322d527c5284a1eed54f8d2`.
- **Result:** `PASS`; no blocking findings. One low-severity advisory preserves
  the reviewer's observation about dormant owner-only copy helpers.
- **Crash-durable authority:** claim commit `9896756…` consumed the sole
  replacement permit before dispatch. Report and receipt were co-committed at
  `434a455…`; the report blob is
  `7aadead414b8f04674f2bce5b0966c657922e370` and the receipt proves continuity
  from the pre-dispatch token hash.
- **Lifecycle:** the ledger retains the original preflight `FAIL`, appends the
  permitted closure `PASS`, sets maintainability to `passed`, and pins
  `implementation_head` to `2a17443…`. A distinct fresh Verifier remains
  mandatory before `verified`.

## Delivered

- **AC-01:** exact v0.15.1 tag, source digest, upstream/adopting VERSION
  identities, mapped bytes, schema classifications, fixtures, and complete
  78-entry archive are proven by `TestBatonV015ExactParity` and
  `TestV015SchemaManifestComplete`.
- **AC-02:** public vendor check and Baton diff are mutation-free, exit 0 at
  exact parity, and keep `spec-ambiguity-report-v1` independently embedded and
  classified.
- **AC-03:** both exact tagged installer scripts under fixed umask 0022 and the
  native stdlib generator produce byte/mode-identical complete managed trees;
  both independently named parity tests pass.
- **AC-04:** unsafe root topology fails before mutation; all three managed homes
  are one whole-root crash-durable transaction; fault, rollback, recovery,
  restore and retire paths are mutation-tested; the isolated built CLI repair
  reached expected exits 2 then 0.
- **AC-05:** canned consumers carry canonical exact-schema identities and
  `state.Status` preserves an explicitly supplied maintainability object
  opaquely and losslessly without synthesising lifecycle state.
- The one-shot replacement closure is bound to its claim and receipt, passed
  all provenance checks, and advanced only the authoritative lifecycle ledger.

## Not delivered

- **Complete typed maintainability/null semantics, protocol selection and
  active-record migration.** Why: S02 carries only the bounded opaque object
  needed for exact v0.15.1 adoption. Tracking:
  `S03-lossless-record-carriers`, `S05-protocol-provenance-archive`, and
  `S17-engine-replan-migration`. Acknowledged by the Coach in the ratified
  replan.
- **Requested-check matching, typed ambiguity separation, and generic
  maintainability retirement.** Why: these require the typed report/reference
  boundary. Tracking: `S04-typed-reference-ambiguity`. Acknowledged by the
  Coach in the ratified replan.

## Divergence from plan

- Planned generic owners `internal/baton/diff_test.go` and
  `internal/baton/vendor_test.go` remain byte-unchanged because their existing
  tests already own generic mapped-drift, operational-source, transformed-
  vendor, idempotence and check-only behavior. Exact v0.15.1 composition and
  mutation coverage instead landed in `records_conformance_test.go`.
- The initial install transaction bag failed the sparse maintainability
  preflight. Within the ratified one-file boundary it was replaced by explicit
  preflight, captured, staged and published private states before the
  authoritative closure PASS.
