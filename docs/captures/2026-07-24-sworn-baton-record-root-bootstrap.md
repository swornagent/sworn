# Sworn Baton record-root bootstrap bridge

Date: 2026-07-24
Status: authorized maintenance bridge (pending publication and merge)
Base: `c32d6846a98aef59a33d0a4bca89a4fde434a1d1`
Authority: exact user approval `approved: maintenance bridge`, recorded
2026-07-24T22:53:09+10:00

This capture records the first maintenance bridge that removes the CLI
composition for `sworn run` and keeps `sworn board` plus `__executor-shim`
unavailable before any delivery command, process, Git, workspace, or
control-database effect.

## Rehearsal status

The disposable rehearsal produced the reviewed implementation transferred into
this isolated authoritative worktree. The external approval above authorizes
only this maintenance bridge. It does not approve the superseded or replacement
Sworn v0.3 Baton plan.

## Rehearsal evidence

The disposable Implementer was one headless Codex CLI invocation:

- model: `gpt-5.3-codex-spark`
- invocation: `codex:019f9403-49ab-7a11-86b5-eaf8987a0053`
- mode: `--yolo exec --ephemeral --ignore-user-config --ignore-rules`
- reported usage: 151,014 tokens

Its first candidate passed its own reported checks but retained an in-process
`app.Run` seam, let the twin-build proof copy the record root, kept a
record-reading legacy board, and left the README's default VCS-stamped build
instructions in place. Captain and clean-context review rejected those claims.

The revised candidate removes the delivery and board compositions, constructs
proof inputs from an explicit product pathspec, uses separate source roots and
fresh Go caches, compares normalized archive entries, and makes the maintenance
state explicit in the README and CLI help. The raw model transcript is
disposable; this bounded capture and the reviewed Git diff are the durable
evidence.

## First authoritative candidate evidence

The reviewed implementation was transferred onto
`bootstrap/v0.3.0-record-inertness` at the exact base above. Excluding this
authority capture, both the rehearsal and authoritative staged diffs have
SHA-256
`ba143944fe3032c9ec10f79c1e773e9ad806ee2a372d0c5ce41c03205c99dd04`.

The authoritative worktree passed:

- product-only `gofmt` discovery;
- `GOFLAGS=-buildvcs=false go test ./...`;
- `GOFLAGS=-buildvcs=false go test -race ./...`;
- `GOFLAGS=-buildvcs=false go vet ./...`;
- the VCS-free, CGO-free, trimmed official build; and
- process smokes proving `run`, `board`, and `__executor-shim` all refuse
  before argument paths can be consumed.

`go version -m` reported no VCS settings for the official binary. These were
the Implementer's candidate-1 claims; the Verifier later rejected the
sufficiency of three focused proofs as captured below.

## First authoritative Captain decision

Decision: `PROCEED`

Captain invocation: `/root/bridge_captain_exact`

The independent read-only Captain confirmed:

1. `run`, `board`, and `__executor-shim` refuse before parsing paths or
   reaching the legacy composition;
2. product copies, formatting, twin builds, and archives exclude the Baton
   record root;
3. official and documented builds disable VCS stamping;
4. the obsolete release workflow is absent; and
5. the README, agent rules, CLI help, and this capture state the narrow
   maintenance boundary truthfully.

## First authoritative Verifier result

Candidate: `51c09b53e1acc01034e9517ca5eb45b2228a135a`

Tree: `7ab58f65a565cb44a967c6a17ba062db111bd0ef`

Verifier invocation: `/root/bridge_verifier_exact`

Verdict: `FAIL`

The clean-context Verifier found four false-green boundaries:

1. the `version` command imported the monolithic legacy protocol package,
   retaining executor and repository initializers in the shipped binary;
2. process tests proved refusal text but did not prove that the shim marker or
   blocking run/board input paths remained untouched;
3. twin builds used Git-free copies, so they could not detect reintroduced VCS
   stamping across a record-only commit; and
4. the CI format command could hide a failing discovery or formatter pipeline.

The Implementer repair removes the legacy import graph from the shipped
maintenance binary, adds exact linked-package and build-metadata assertions,
uses blocking path canaries plus an untouched shim marker, commits the
record-only twin-build mutation, and makes CI formatting fail under pipeline
errors. Candidate 1 remains immutable failure evidence.

## Replacement candidate evidence

The repair is the exact child of failed candidate 1. Excluding this capture,
its diff has SHA-256
`60fc61bacdde73139a3cef04fecf772ab042e6864ef5d52f3f1733fb99269678`.

The replacement passed product-only formatting, full tests, race tests, vet,
diff checking, and a CGO-free VCS-free trimmed build. The built binary has
SHA-256
`85f59f5013fe602b517192b89a42c98df23f8b03c540d70bf4891f83458da9c7`,
reports `0.3.0-dev` and `maintenance-bootstrap`, has no `vcs.*` build settings,
and contains no `github.com/swornagent/sworn/internal/` symbol.

## Replacement Captain decision

Decision: `PROCEED`

Captain invocation: `/root/bridge_captain_exact`

The Captain bound the repair to parent
`51c09b53e1acc01034e9517ca5eb45b2228a135a`, full staged repair digest
`49f80ac41af2b1b53baa20664409329bb38c08cbb65872475b8f571b695e0b03`,
and the non-capture digest above. The review confirmed all four Verifier
findings are addressed without scope escape. The replacement now awaits a
fresh Verifier verdict.

## Discovered consumers

Affected binaries and paths are:
- `sworn run`
- `sworn __executor-shim`
- `cmd/sworn/main.go`
- `cmd/sworn/main_test.go`
- `cmd/sworn/board.go`
- `cmd/sworn/run.go`
- `cmd/sworn/live_delivery_linux_test.go`
- `cmd/sworn/binary_integration_test.go`
- `internal/executor/linux_integration_test.go` shim usage path
- `sworn board --store`
- `README.md` and `AGENTS.md` build instructions
- `.github/workflows/ci.yml` command/build matrix
- `.github/workflows/release.yml`
- `.gitattributes` export list

## Invariant rationale

A permissive `record_root_inert` assertion is forbidden in this bridge because
it would encode a false completeness claim about record-root behavior before R0
replaces this kernel.
The bridge must instead gate the shipped delivery entrypoints before any
configuration parsing or side-effecting path is entered, and prove the
supported build/test/package surfaces ignore the record root.

## Bridge boundary

Read-only or repository-independent commands remain available:
- `sworn version`
- `sworn help`

The public delivery composition and its credential-gated live test are removed.
The shipped CLI cannot dispatch delivery, executor process control, or the
legacy board: all three entrypoints return an explicit refusal before parsing
their arguments or opening configuration.

The legacy SQLite board is not the v0.3 Baton oracle. It is unavailable during
the bridge; R0 replaces it with the Baton release-and-track projection.

Legacy internal packages still execute against isolated fixtures in regression
tests. They are archaeology, not shipped delivery entrypoints. Their ordinary
test discovery and formatting commands are VCS-free and exclude the repository
record root.

The old release workflow is deleted. `.gitattributes` excludes the record root
from `git archive`, and the test compares normalized product entries across a
record-only commit. Git/source-archive bytes and checksums may still encode
commit metadata, so they are repository provenance and must not be used as
product identity. No v0.3 product package or release is authorized until R0
installs a normalized product-only release path.

## Required plan revisions

R0 must formalize the official VCS-free build proof with:
- official build configured with `GOFLAGS=-buildvcs=false`, `-buildvcs=false`,
  and `-trimpath`
- no ordinary commit identity stamping into the supported binary
- a deterministic twin-build proof over separate product copies and fresh Go
  build caches
- `.baton/releases/.../status.json` and descendants ignored for payload identity

R3 must move to product-only worktrees and record-preserving capture without
reusing the legacy candidate/workspace pipeline.

R4 must use bounded control inputs without record-root mounts.

S7 must replace ordinary Git commit stamping with product-tree identity controls.

## Control and product access

No retained command opens repository, database, or `.baton/releases` paths.
The product-copy, format, build, and archive surfaces exclude the record root
explicitly. R0 introduces conforming Baton control-plane reads. Product
delivery, model, check, candidate, and release-package consumption remain
unreachable through the shipped CLI until R0 lands the replacement engine.
