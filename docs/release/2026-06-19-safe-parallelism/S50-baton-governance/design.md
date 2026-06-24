# Design TL;DR — S50-baton-governance

## §1. User-visible change

`sworn baton diff <source-dir>` compares the committed embed against the
transformed pinned source. When in sync it exits 0; when divergent it exits
non-zero and prints each divergent file path. The companion doc,
`docs/baton-governance.md`, documents the rule: protocol changes found during
sworn development must be raised as a PR against Baton, reviewed/authorised,
merged, and then re-vendored — never hand-edited into the embed in place.

## §2. Design decisions not in spec (max 5)

1. **DiffOpts struct vs reusing VendorOpts.** `Diff` gets its own `DiffOpts`
   with `SourceDir` + `RepoRoot` — narrower interface, self-documenting that
   this isn't a vendor operation. Rationale: the contract is different (returns
   `[]Divergence`, never writes), and adding `CheckOnly` to a diff opts would
   be misleading.

2. **Divergence type.** `Divergence{File string, Reason string}` — `File` is
   the embed-relative path (e.g. `internal/adopt/baton/rules/…`); `Reason` is
   a short descriptor (e.g. "content differs from transformed source").
   Rationale: the CLI prints `<file>: <reason>` per line; the spec asks for
   "file path + a short reason".

3. **Reusing the Vendor transform/mapping code paths exactly.** `Diff` calls
   the same `batonFileMappings`, `ValidateSource`, and `Transform` that
   `Vendor` uses — zero divergence in source resolution or transformation.
   The `rules.md` sentinel entry in the mapping is handled identically
   (concatenate all rule sources, transform each, compare against
   `internal/prompt/baton/rules.md`). Rationale: spec Risk #1 — any code-path
   fork makes a clean tree show false divergence.

4. **Exit code convention.** 0 = in sync, 1 = divergent, 64 = usage error
   (consistent with `sworn baton vendor`). Rationale: the sworn CLI already
   uses 64 for usage errors; 1 for divergence is unambiguous.

5. **ADR-0006 finalisation.** The ADR is already `Status: accepted`. The spec
   asks to "finalise" it — this means confirming no open question remains and
   noting in proof.md that the enforcement the ADR describes now exists. No
   edit to the ADR file itself unless an open question is found (none is).

## §3. Files I'll touch grouped by purpose

- **Core Diff logic:** `internal/baton/diff.go` (new), `internal/baton/diff_test.go` (new)
  — Implementation of `Diff(opts) ([]Divergence, error)` and its unit tests.
- **CLI surface:** `cmd/sworn/baton.go` (extend) — add `diff` subcommand to
  the existing `sworn baton` verb, routing to `cmdBatonDiff`.
- **Governance doc:** `docs/baton-governance.md` (new) — the PR-up workflow,
  linking ADR-0006 and sawy3r/baton#31.
- **ADR confirmation:** `docs/adr/0006-baton-protocol-sync.md` — confirm
  enforcement now exists if any open question remains (currently none visible).

## §4. Things I'm NOT doing

- **CI wiring.** `docs/baton-governance.md` will recommend it; no CI workflow
  file is created. → Rule 2 deferral: why = CI is a separate harness change;
  tracking = S50 proof.md "Not delivered"; ack = Coach.
- **Live-remote diff.** Diff is against the pinned local source (the same
  source directory S48 vendors from). Live-remote is a later enhancement. →
  Rule 2 deferral: why = network fetch boundary is distinct from local-source
  diff; tracking = future slice / sawy3r/baton issue; ack = Coach.
- **Actually filing upstream Baton PRs.** That's upstream work tracked at
  sawy3r/baton#31.

## §5. Reachability plan

Run `sworn baton diff <testdata/fixture>` against the existing test fixture:
- **Clean case:** after vendoring into a temp repo, `sworn baton diff` exits 0
  and prints nothing (or "in sync").
- **Divergent case:** hand-edit one embed rule file in the temp repo,
  `sworn baton diff` exits non-zero and prints the divergent file path + reason.
Both are captured in proof.md as reachability artefacts.

## §6. Open questions for the Coach

None.