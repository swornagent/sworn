# Design TL;DR — S01-vendor-boundary-readiness

**Slice:** `S01-vendor-boundary-readiness` · **Track:** `T1-foundation` · **Release:** `2026-07-15-baton-v0.16-conformance`
**State:** `design_review` — revised after Captain `NEEDS_COACH`; no production code or tests written
**Covers:** `N-01`, `N-02`, `N-10` · **Effort/complexity:** low / low (`quick`)

## User outcome

`sworn baton vendor` and `sworn baton vendor --check` will share one exact,
fail-closed materialisation path. Prose such as `board.json.shared_touchpoints`
will no longer be mistaken for a script; real executable references will still
block; the untouched v0.15 `board-v1` bytes will compile with their ECMA-262 path
semantics; and VERSION plus every mapped destination will be either fully applied
or explicitly non-authoritative under restart-safe recovery.

## Revised approach

1. **Capture once; materialise everything before mutation.** The public upstream
   write invocation captures one instant before materialisation. A pure helper in
   `internal/baton/version.go` constructs the complete VERSION replacement bytes
   from that instant, tag, SHA, and digest without writing. `Vendor` transforms
   every mapped source, compiles every mapped JSON schema from untouched bytes,
   adds VERSION as an ordinary candidate, byte-sorts the complete destination
   plan, snapshots every original, and finishes the deterministic diff before the
   first primary-worktree mutation.
2. **Use one plan for check and write modes.** Check mode exercises the identical
   transform, schema, and materialisation path but never constructs a writable
   transaction: it creates no destination directory, repository temporary, pin
   write, sentinel, manifest, snapshot, or other worktree/Git-admin state. Write
   mode submits that same immutable plan to `vendor_transaction.go`; there is no
   later standalone `WriteUpstreamPin` step.
3. **Expose the exact public exit map.** `cmd/sworn/baton.go` maps only a valid
   byte-identical check or fully successful write to exit 0; only a valid,
   deterministic non-empty check drift to exit 1; and invalid source,
   operational, preflight, apply, rollback, and recovery outcomes to exit 2. A
   successful recovery-only invocation also exits 2 with re-run guidance and
   never combines restoration with a new vendor write.
4. **Apply atomically and restore exact originals.** Each changed destination is
   installed with a same-directory temporary plus rename after full preflight. A
   failed apply rolls back in reverse order to the captured bytes, four-digit
   permission mode, or absence, then verifies the entire tuple set rather than
   trusting operation return values. Exact restoration returns the original
   operational exit 2; incomplete restoration returns class
   `rollback-incomplete`, lists unrestored paths in byte order, and publishes the
   sole durable recovery authority.
5. **Make recovery non-self-referential and restart-authoritative.** Recovery is
   confined beneath the physically resolved current-worktree Git administrative
   directory at `sworn/recovery/baton-vendor`. Directories are mode `0700` and the
   sentinel, manifest, and snapshots are mode `0600`; symlinks are forbidden at
   and below the recovery root. The compact manifest records physical repository
   and Git-admin identity plus byte-sorted destination tuples containing only
   canonical repository-relative path, original existence, original mode or `-`,
   original SHA-256 or `-`, and transaction-relative snapshot path or `-`.
   Existing originals have one regular digest-matching snapshot; absent originals
   have no mode, digest, or snapshot.
6. **Separate manifest identity from discovery.** The manifest never contains its
   own transaction identity. The identity is SHA-256 over the exact compact
   manifest bytes plus final LF; its bare 64-hex value names the transaction
   directory. The separately serialized fixed
   `rollback-incomplete.json` sentinel contains the lowercase
   `sha256:<64-hex>` identity and points to that directory, but is not input to
   the manifest digest.
7. **Fail closed before recovery writes.** A fresh write invocation that sees the
   sentinel is recovery-only and validates the complete material set before
   touching any destination: current physical repository/Git-admin identity;
   canonical, repository-confined, transaction-authorized mapped/VERSION paths;
   canonical snapshot confinement to the named transaction directory; exact
   owner-only modes and no symlinks; manifest digest/directory identity; and no
   missing, duplicate, or foreign entry. Traversal, foreign material, missing
   material, symlinks, mode drift, tampered sentinel/manifest/snapshot, or any
   integrity mismatch exits 2, retains recovery authority, and performs no
   ordinary vendor write. Valid recovery restores and verifies every tuple,
   removes the sentinel and transaction material, then exits 2 with re-run
   guidance.
8. **Detect executable references lexically.** Replace substring scanning with a
   small token scanner that recognizes shell, Python, and module-script tokens at
   lexical boundaries, including bare and path-prefixed forms. Punctuation,
   Markdown code/link targets, and real `scripts/example.sh`, `example.py`, and
   `example.mjs` tokens remain blocking; embedded prose fragments such as
   `board.json.shared_touchpoints` do not become tokens.
9. **Adapt only the exact unsupported schema expression.** Configure the existing
   schema compiler with a stdlib-only regexp adapter. Ordinary expressions use Go
   `regexp`; the exact upstream
   `^(?!/)(?!.*(?:^|/)\.\.?($|/)).+$` expression uses the equivalent decoded-string
   predicate from the normative clarification. The schema document is never
   rewritten, and tests assert compiler-input byte identity before the complete
   positive/negative path matrix.

## Design choices

- **Type-1 bootstrap authority:** preserve the Coach-ratified staged manual
  v0.15 bootstrap and require later engine revalidation.
- **Type-2 — immutable transaction plan:** one ordered plan drives diff, check,
  snapshot, apply, rollback, verification, and recovery, preventing VERSION or
  check/write semantic drift while keeping work O(total mapped bytes + files).
- **Type-2 — deterministic recovery identity:** digest the manifest rather than a
  self-referential record, and make the fixed sentinel the only discovery point.
- **Type-2 — injected filesystem seam:** fault-inject narrow file operations at
  every apply/rollback index without process-global mutation.
- **Type-2 — path-safe diagnostics:** expose phase, error class, recovery
  location, and canonical destination paths only; never emit mapped payload
  bytes, request bodies, or secrets.
- **Boundary — S02 owns content and installs:** S01 adds pure VERSION-byte
  construction and transaction machinery but does not execute the v0.15.1
  content/pin replacement, alter mapped content, or update Codex/Claude mirrors.

## Planned files and AC trace

| File | Planned change | AC |
|---|---|---|
| `cmd/sworn/baton.go` | Capture the upstream invocation instant, pass VERSION construction inputs into materialisation, remove the standalone pin write, and enforce exits 0/1/2. | AC-03, AC-04 |
| `cmd/sworn/baton_test.go` | Drive public command dispatch through clean check, drift, invalid/preflight, apply/rollback, recovery, and zero-mutation check paths. | AC-03, AC-04 |
| `internal/baton/version.go` | Purely construct complete VERSION bytes from the captured instant and pin fields; perform no write. | AC-03 |
| `internal/baton/version_test.go` | Prove one captured instant determines exact candidate bytes and that no standalone VERSION write occurs. | AC-03 |
| `internal/baton/vendor.go` | Build the complete immutable mapped-plus-VERSION plan and deterministic drift result before mutation. | AC-03, AC-04 |
| `internal/baton/vendor_test.go` | Prove full preflight precedes mutation and check mode shares validation without repo or Git-admin writes. | AC-03, AC-04 |
| `internal/baton/vendor_transaction.go` | Own snapshots, atomic apply, exact reverse rollback, manifest/sentinel publication, fresh-invocation recovery-only validation, and typed failure classes. | AC-03, AC-04 |
| `internal/baton/vendor_transaction_test.go` | Fault-inject every apply/rollback boundary and reject fresh-invocation tamper, traversal, missing, symlinked, mode-drifted, duplicate, and foreign recovery material. | AC-03 |
| `internal/baton/transform.go` | Replace substring matching with lexical executable-reference extraction. | AC-01 |
| `internal/baton/transform_test.go` | Cover prose, bare/relative/path-prefixed tokens, punctuation, Markdown targets, and all executable suffixes. | AC-01 |
| `internal/baton/validate_schema.go` | Install the stdlib-only exact-pattern predicate while preserving raw schema bytes. | AC-02 |
| `internal/baton/validate_schema_test.go` | Assert untouched v0.15 input bytes and every specified accepted/rejected path. | AC-02 |

## Reachability plan

The first red is
`cmd/sworn/baton_test.go:TestBatonVendorAtomicPreflightReachability`, driving the
registered `sworn baton vendor` command. It pairs every public result with its
exit: clean check/write success 0, check drift 1, and invalid/preflight/apply/
rollback/recovery 2. Filesystem snapshots prove check/preflight zero mutation,
full mapped-plus-VERSION rollback, and fresh-invocation recovery-only behavior.
Package tests isolate the transaction fault matrix, recovery trust boundary,
lexical grammar, and exact schema expression.

## Review pins for the fresh Captain

1. **[MECHANICAL] VERSION is an ordinary transaction member:** one invocation
   instant, pure byte construction before mutation, byte-sorted snapshot/apply/
   rollback/recovery participation, and no post-vendor pin write.
2. **[MECHANICAL] Public exits are exhaustive:** 0 for clean/success, 1 only for
   valid check drift, and 2 for every invalid, operational, atomicity, or recovery
   outcome including completed recovery-only guidance.
3. **[SECURITY] Recovery authority is fixed and confined:** non-self-referential
   manifest identity, separate sentinel, physical Git-admin confinement,
   owner-only modes, no symlinks, complete-set validation, and no destination
   touch before trust checks pass.
4. **[MECHANICAL] Normative schema bytes remain exact:** only the decoded exact
   ECMA-262 expression receives the explicit predicate; raw upstream bytes are
   asserted unchanged.
5. **[BOUNDARY] S02 still owns v0.15.1 content/pin/install execution:** S01 lands
   machinery and proof only.
