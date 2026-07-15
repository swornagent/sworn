# Design TL;DR — S01-vendor-boundary-readiness

**Slice:** `S01-vendor-boundary-readiness` · **Track:** `T1-foundation` · **Release:** `2026-07-15-baton-v0.15-conformance`  
**State:** `design_review` — no production code or tests written  
**Covers:** `N-01`, `N-02`, `N-10` · **Effort/complexity:** low / low (`quick`), confirmed

## User outcome

`sworn baton vendor` and `sworn baton vendor --check` will share one exact,
fail-closed preflight. Prose such as `board.json.shared_touchpoints` will no
longer be mistaken for a script; real executable references will still block;
the exact v0.15 `board-v1` schema bytes will compile with their ECMA-262 path
semantics; and no failed write will be reported as authoritative success.

## Approach

1. **Materialise before mutation.** `Vendor` will validate the source, materialise
   every mapping in destination-byte order, compile every mapped JSON schema from
   the untouched source bytes, snapshot each destination's bytes/mode/existence,
   and build the deterministic diff entirely before changing the worktree.
2. **Use one plan for check and write modes.** Check mode returns only the
   validated in-memory plan/diff and creates no destination directories, repo
   temporaries, pin writes, or other worktree state. Write mode applies that same
   plan through a transaction in `vendor_transaction.go`.
3. **Apply atomically and roll back exactly.** Each changed destination is
   installed with a same-directory temporary plus rename after the full preflight.
   A failed apply rolls back in reverse order to the captured bytes, permission
   mode, or absence. Fault-injected filesystem operations will exercise every
   apply and rollback index. An incomplete rollback returns a typed
   `rollback-incomplete` error with byte-sorted unrestored destinations and keeps
   mode-0700 recovery material; a later write invocation enters restoration-only
   handling and cannot report success until the snapshot is exact.
4. **Detect executable references lexically.** Replace substring scanning with a
   small token scanner that recognises shell/Python/module-script tokens at
   lexical boundaries, including bare and path-prefixed forms. Punctuation,
   Markdown code/link targets, and real `scripts/example.sh`, `example.py`, and
   `example.mjs` tokens remain blocking; embedded prose fragments such as
   `board.json.shared_touchpoints` do not become tokens.
5. **Adapt only the exact unsupported schema expression.** Configure the existing
   `jsonschema/v6` compiler with a stdlib-only regexp engine. Ordinary expressions
   continue through Go `regexp`; the exact upstream
   `^(?!/)(?!.*(?:^|/)\.\.?($|/)).+$` expression is represented by the equivalent
   decoded-string predicate from the normative clarification. The schema document
   itself is never rewritten.

## Design choices

- **Type-1 bootstrap authority:** already Coach-ratified in `status.json`: use the
  staged manual v0.15 bootstrap and require later engine revalidation.
- **Type-2 — immutable in-memory plan:** one ordered plan drives diff, check, apply,
  and rollback, preventing check/write semantic drift while keeping work linear in
  mapped bytes plus file count.
- **Type-2 — injected filesystem seam:** transaction tests inject failures through
  narrow file-operation hooks rather than process-global mutation; production uses
  direct stdlib operations.
- **Type-2 — exact-pattern regexp adapter:** implement the one normative ECMA-262
  expression explicitly and delegate every supported expression to Go `regexp`.
  This avoids a new runtime dependency and avoids laundering a schema-byte rewrite.
- **Type-2 — path-safe diagnostics:** errors expose phase, destination-relative
  path, error class, and recovery location only; mapped payload bytes, request
  bodies, and secrets never enter diagnostics.

## Planned files and AC trace

| File | Planned change | Acceptance criteria |
|---|---|---|
| `internal/baton/vendor.go` | Build the full ordered plan, share it between check/write modes, and report deterministic drift/results. | AC-03, AC-04 |
| `internal/baton/vendor_transaction.go` | Add snapshot, atomic apply, reverse rollback, recovery-only guard, typed failure classes, and injected file operations. | AC-03, AC-04 |
| `internal/baton/vendor_transaction_test.go` | Fault-inject every preflight/apply/rollback boundary; assert exact bytes, modes, absence, sorted paths, and recovery blocking. | AC-03 |
| `internal/baton/vendor_test.go` | Prove full preflight precedes mutation and check mode shares validation without filesystem writes. | AC-03, AC-04 |
| `internal/baton/transform.go` | Replace substring matching with lexical executable-reference extraction. | AC-01 |
| `internal/baton/transform_test.go` | Cover prose, bare/relative/path-prefixed tokens, punctuation, Markdown code/link targets, and all three executable suffixes. | AC-01 |
| `internal/baton/validate_schema.go` | Install the stdlib-only exact-pattern regexp adapter for schema compilation. | AC-02 |
| `internal/baton/validate_schema_test.go` | Compile untouched v0.15 `board-v1` bytes and mutation-test every required positive/negative path. | AC-02 |
| `cmd/sworn/baton_test.go` | Drive `sworn baton vendor [--check]` through command dispatch and assert exits 0/1/2 plus zero-mutation check/preflight behavior. | AC-03, AC-04 |

## Reachability plan

The first red is `cmd/sworn/baton_test.go:TestBatonVendorAtomicPreflightReachability`,
driving the registered `sworn baton vendor` command rather than a leaf helper. It
will cover byte-identical check (0), deterministic drift (1), invalid/preflight,
recoverable apply failure, and rollback-incomplete (2), paired with filesystem
snapshots proving the promised mutation boundary. Package-level transaction,
transform, and schema tests then isolate the fault matrix and exact grammar.

## Risks and Captain pins

1. **[ESCALATE / mechanical scope defect] CLI exit mapping is outside the declared
   touchpoints.** `cmd/sworn/baton.go` currently returns `0` for check-mode drift and
   `1` for vendor errors, while AC-03/AC-04 and the normative table require drift
   `1` and invalid/operational/rollback failures `2`. That file is not in this
   slice's touchpoints, and the Implementer may not edit it. Before implementation,
   the Captain must require a planner correction adding `cmd/sworn/baton.go` (and
   explicitly confirming upstream pin-write transaction ordering) or decline this
   design; tests alone cannot change the command contract.
2. **[MECHANICAL] Rollback completeness:** reverse-order restoration must compare
   every destination to the captured snapshot after attempted rollback, not infer
   success from operation return values. Only mismatched paths enter the sorted
   `rollback-incomplete` list.
3. **[MECHANICAL] Exact-byte schema proof:** tests must compare the compiler input
   with the pinned upstream file before exercising the predicate, so a semantic
   copy or pattern rewrite cannot satisfy AC-02.
4. **[SECURITY] Recovery material:** originals may contain private adopting-project
   content. Recovery directories and files must be owner-only, diagnostics must
   name paths rather than bytes, and successful restoration must remove the
   material.
5. **[BOUNDARY] S02 owns the pin/content/install update.** This slice repairs the
   vendor boundary only; it must not advance `VERSION`, change mappings, or update
   embedded Baton bytes.
