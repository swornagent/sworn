# Design TL;DR — S62-baton-upstream-source

## §1. User-visible change

`sworn baton vendor` gains `--upstream`, `--tag`, and `--repo` flags. With
`--upstream`, the binary fetches the pinned Baton release tarball from the public
GitHub repo (`github.com/sawy3r/baton`) over HTTPS, verifies the resolved commit
SHA and content digest against the VERSION pin, extracts the tarball into a temp
directory, and feeds it through the existing transform pipeline — identical
output, just network-sourced. Without `--upstream`, behaviour is exactly
unchanged (local directory, as S48 shipped).

## §2. Design decisions not in spec (max 5)

1. **Temp directory lifecycle.** The network provider downloads to `os.MkdirTemp`,
   extracts, feeds to `Vendor()`, and cleans up on function return. The temp dir
   is transient; the durable artefact is the committed embed files. No persistent
   cache.

2. **Digest algorithm: SHA-256.** `crypto/sha256` (stdlib), hex-encoded, stored
   as `upstream-digest: sha256:<hex>` in VERSION. Verifies tarball integrity and
   offers content-addressable tamper detection — a force-moved tag that changes
   content produces a different digest even if the commit SHA is the same (a
   second-order supply-chain vector that SHA-alone doesn't cover).

3. **VERSION record is written after successful Vendor.** The resolved commit SHA
   (from the GitHub API tag resolution or `X-GitHub-Commit` header if available)
   and the calculated digest are written to `internal/adopt/baton/VERSION` only
   after `Vendor()` returns nil — so a fetch that fails late (e.g. transform
   error) doesn't leave a stale pin from a half-finished run.

4. **Flag semantics — local vs. upstream.** When `--upstream` is set, the
   positional `source-dir` argument becomes optional (the provider resolves it);
   `--tag` and `--repo` only apply with `--upstream`. Omitting `--tag` uses the
   pinned semver tag from VERSION (`baton.Version()`). This keeps the CLI
   backward-compatible and the common case single-flag.

5. **No `SourceProvider` interface.** A single function `FetchUpstream(ctx
   context.Context, repo, tag string) (*FetchResult, error)` returns a source
   directory, resolved SHA, and content digest. It's called from
   `cmdBatonVendor`; the returned `sourceDir` drops into `VendorOpts.SourceDir`
   with no other pipeline changes. Simple, testable, no abstraction tax.

## §3. Files I'll touch grouped by purpose

- **Network fetch + verify:** `internal/baton/fetch.go` (new) — `FetchUpstream()`
  does GET → sha256 → gzip.NewReader → tar.NewReader, strips the top-level
  `<repo>-<ref>/` prefix, and verifies SHA/digest against the pin. Stdlib only.
- **Network fetch tests:** `internal/baton/fetch_test.go` (new) — `httptest.Server`
  fixtures covering success, SHA mismatch, digest mismatch, 404/5xx, bad gzip,
  prefix-strip correctness.
- **CLI wiring:** `cmd/sworn/baton.go` — add `--upstream`/`--tag`/`--repo` flags
  to `cmdBatonVendor`; when `--upstream`, call `FetchUpstream` and pass the
  result's sourceDir to `Vendor()`, then write the pin update to VERSION.
- **Pin record write-back:** `internal/baton/version.go` — add `WriteUpstreamPin`
  that reads VERSION, updates/adds `upstream-sha` and `upstream-digest`, and
  writes back atomically.
- **VERSION file:** `internal/adopt/baton/VERSION` — receives `upstream-digest:`
  line after first upstream fetch (write-back, not hand-edited).

## §4. Things I'm NOT doing

- **Private-repo auth (PAT).** Explicitly out of scope; tracked in issue #11.
- **git clone or go-git transport.** Tarball-only, per spec.
- **Any FileMapping change.** The mapping S48 defined is unchanged; network is
  just another way to get the source bytes.
- **Transform pipeline changes.** The existing `Transform()` + `Vendor()`
  pipeline is unchanged.
- **Live-remote `baton diff`.** Out of scope; S50's `diff` remains local.
- **VERSION format refactoring.** Extend (add `upstream-digest`), don't
  restructure what S49 owns.

## §5. Reachability plan

- **Integration test** (`go test ./cmd/sworn/...`): drive `sworn baton vendor
  --upstream` against an `httptest.Server` that serves a fixture tarball, then
  assert the embed files were written and match expectations. This is the Rule 1
  artefact — through the command, not just the leaf.
- **Unit tests** (`go test ./internal/baton/...`): `fetch_test.go` — success,
  mismatch, error paths.
- **Proof transcript:** `sworn baton vendor --upstream` against a local
  httptest fixture showing fetch → verify → transform → write, plus a tampered
  digest run failing closed with non-zero exit and no file change.

## §6. Open questions for the Coach

_None._