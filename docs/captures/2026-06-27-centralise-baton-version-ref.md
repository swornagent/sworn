# Centralise the Baton VERSION ref — Type-1 decision + proof bundle (2026-06-27)

Sworn-side fix on branch `fix/centralise-baton-version` (off `release/v0.1.0`).
Surfaced while tracing how `sworn run` sources its role directions during the
Baton records-as-JSON work.

## The finding

Sworn embedded **three** Baton version files claiming three different versions,
with no single source of truth and no drift guard:

| File | Claimed | Provenance | Read by |
|---|---|---|---|
| `internal/adopt/baton/VERSION` | **v0.5.0** | `upstream-sha` + sha256 digest + date | `baton.Version()` → everything authoritative |
| `internal/prompt/VERSION.txt` | **v0.4.2** | bare semver | **nothing** (orphaned) |
| `internal/prompt/baton/VERSION.txt` | **v1.0.0** | bare semver | `BatonAll()` / MCP listing only |

The authoritative surfaces (`sworn version` via `main.go`, the `sworn://baton/version`
MCP resource, and the `doctor` version checks) **already** read
`internal/adopt/baton/VERSION` via `baton.Version()`. The other two were
orphaned rot — not in the vendor pipeline (`internal/baton/source.go` has no
VERSION entries), so they never updated and drifted to v0.4.2 / v1.0.0 while the
real pin moved to v0.5.0. `doctor` even *validated their semver*, lending them
false legitimacy (Rule 2: dark, contradictory metadata).

## Type-1 design decision (Baton Rule 9)

This is architecturally significant — it defines how Sworn pins its upstream
protocol — so the decision was human-owned, not model-self-authorised.

**Decision (human-selected):** make `internal/adopt/baton/VERSION` the **single
source of truth**, delete the two zombie files, and add a **fail-closed drift
guard** so a recurrence is a hard build failure.

**Options considered:**
- **A — single canonical file + drift gate (CHOSEN).** Richest provenance,
  reuses existing `internal/baton/version.go` machinery, everything already
  reads it. Adds a guard so it can't recur.
- **B — centralise only, defer the gate.** Smaller diff; leaves the recurrence
  door open. Rejected: the gate is the point.
- **C — new top-level `baton.pin` file.** Relocates machinery that already
  exists on the canonical file for no gain. Rejected.

**Decision-maker:** Brad (via the AskUserQuestion design fork, 2026-06-27).
Model proposed + classified the options; did not self-authorise.

## Scope

One source of truth for the Baton protocol version in the Sworn binary, with a
fail-closed guard against the three-contradictory-files recurrence.

## Files changed

`git diff --name-only release/v0.1.0` (this branch):

```
cmd/sworn/doctor.go
internal/baton/version_test.go
internal/prompt/VERSION.txt           (deleted)
internal/prompt/baton/VERSION.txt     (deleted)
internal/prompt/prompt.go
internal/prompt/prompt_test.go
```

## Test results

Slice-relevant commands (full suite is the merge gate's job; note pre-existing
breakage below):

- `go build ./...` → **exit 0** (clean).
- `gofmt -l` on the four edited Go files → empty (all formatted).
- `go test ./internal/prompt/... ./internal/baton/...` → **ok** (both packages),
  including the new `TestNoEmbeddedVersionFile` and `TestUpstreamPinComplete`.
- `go test ./cmd/sworn/ -run 'Doctor|Version'` → **ok** (consolidated version
  check; `doctor_test.go` still finds the `baton/VERSION (baton-protocol)` label).

**Pre-existing, out-of-scope breakage (NOT caused by this change):** several
test files in untouched packages fail to compile against a `state` package
refactor — `cmd/sworn/{slice,query}_test.go`, `internal/.../ledger_test.go`,
`internal/parallel`, etc. (`state.Dispatch`, `state.Verification.Model`,
`adopt.AgentsFragment` undefined). Confirmed independent: `git diff --name-only`
touches none of them, and `internal/prompt` + `internal/baton` + the `cmd/sworn`
version path all build and test green. Surfaced here as a Rule 2 deferral — it
predates this branch and belongs to whoever owns the `state` refactor; flagged,
not silently absorbed.

## Reachability artefact

The guard is reachable two ways and fails closed:
- **Build/CI:** `internal/prompt/prompt_test.go::TestNoEmbeddedVersionFile`
  fails if any `VERSION`/`VERSION.txt` re-enters the prompt embed;
  `internal/baton/version_test.go::TestUpstreamPinComplete` fails if the single
  pin loses its `upstream-sha` or `upstream-digest`.
- **Runtime:** `sworn doctor` Group-3 check `baton/VERSION (baton-protocol)`
  now ERRORs (fail closed) on a missing/non-semver/incomplete pin, OK with
  "single source of truth; sha+digest pinned".
- **Smoke:** re-add `internal/prompt/VERSION.txt` → `go test ./internal/prompt/...`
  fails; blank the `upstream-digest` line → `go test ./internal/baton/...` and
  `sworn doctor` both fail.

## Delivered

- Deleted `internal/prompt/VERSION.txt` (v0.4.2) and
  `internal/prompt/baton/VERSION.txt` (v1.0.0); dropped `VERSION.txt` from the
  `//go:embed` directive and the `BatonAll()` doc.
- `internal/adopt/baton/VERSION` is the sole Baton version source; everything
  authoritative already read it via `baton.Version()` (unchanged).
- `doctor.go`: merged the two redundant/mislabeled version checks into one
  accurate `baton/VERSION (baton-protocol)` check + a **completeness gate**
  (fail closed unless `upstream-sha` AND `upstream-digest` are set).
- Guard tests: `TestNoEmbeddedVersionFile`, `TestUpstreamPinComplete`.
- Fixed the misleading origin references the human flagged — `prompt.go`'s
  package doc said the prompts were vendored from `~/.claude/baton/role-prompts/`
  (a local install); repointed to `github.com/sawy3r/baton` at the pinned SHA,
  matching `internal/adopt/baton/VERSION`'s own provenance lines.

## Not delivered

- **Re-vendoring the actual stale prompt/rule content** — the embedded role
  prompts + rules are still pre-records-as-JSON because the pin (`9ae08fb`)
  predates PR #52. That is the step-3 `/plan-release` Phase-B re-vendor, gated
  on #52 merging. Out of scope here; this slice fixes the version *ref*, not the
  vendored *content*.
- **The `state`-refactor test breakage** above — not this slice's; tracked as a
  Rule 2 deferral for the `state` owner.

## Divergence from plan

- The drift gate's *form* changed from the original AskUserQuestion preview
  ("embedded bytes == upstream-digest"). That was based on a wrong model of the
  digest: `upstream-digest` is the SHA-256 of the **upstream tarball** (used by
  `internal/baton/fetch.go`'s sync verifier), not a hash of the embedded subset,
  so an "embedded bytes" comparison is not computable offline. The delivered
  gate instead enforces **single-source + pin completeness** (one version file;
  `sha`+`digest` present) — deterministic, no network, and it directly prevents
  the three-files recurrence. Full content-vs-upstream verification already
  exists in `fetch.go` at sync time (re-fetch the pinned SHA, sha256, compare).
