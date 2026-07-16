---
title: 'Normative v0.15 conformance clarifications'
description: 'Exact decision tables closing the pre-implementation ambiguity review for Baton v0.15 conformance.'
---

# Normative v0.15 conformance clarifications

This file is a normative planning input for the slices that reference it. It
does not amend Baton v0.15.1. It makes the exact tagged behavior and the
Coach-ratified Sworn adapter choices executable without relying on conversation
or implicit discovery. On conflict, the exact Baton v0.15.1 tag remains the
upstream protocol authority; this file fixes the Sworn-side spelling, record,
exit, recovery, and activation choices left to the adapter.

## 1. Vendor boundary and board path grammar

The exact upstream `board-v1` shared-touchpoint property-name expression is:

```text
^(?!/)(?!.*(?:^|/)\.\.?($|/)).+$
```

The vendored schema bytes retain that expression byte-for-byte. Because Go RE2
does not support lookahead, the compiler implements this expression with the
equivalent predicate below; it must not rewrite the schema:

1. the decoded JSON string is non-empty;
2. it does not begin with `/`;
3. after splitting on `/`, no segment is exactly `.` or `..`; and
4. all other strings accepted by the upstream JSON-schema expression are
   accepted, including dot-prefixed names such as `.github/workflows/ci.yml`,
   doubled dots inside a segment such as `a..b`, and trailing `/`.

Required negative fixtures are `/a`, `.`, `..`, `a/./b`, `a/../b`, `a/.`, and
`a/..`. Required positive boundary fixtures are `a`, `.a`, `a..b`, `a//b`,
`a/`, and `.github/workflows/ci.yml`.

`sworn baton vendor` uses these exit codes:

| Mode and outcome | Exit | Writes |
|---|---:|---|
| `--check`, valid source, destination byte-identical | 0 | none |
| `--check`, valid source, deterministic non-empty diff | 1 | none |
| `--check`, invalid/unmapped source or operational error | 2 | none |
| write mode, validation and atomic replacement succeed | 0 | atomic replacement only |
| write mode, preflight/materialisation/schema validation fails | 2 | primary worktree unchanged |
| write mode, apply fails and rollback succeeds | 2 | starting bytes, modes, and absence restored exactly |
| write mode, rollback is incomplete | 2 | class `rollback-incomplete`, byte-sorted unrestored paths, recovery material preserved |

The check-mode diff is byte-sorted by destination-relative UTF-8 path, reports
added/removed/changed without source payload bytes, and a non-empty diff is
always repository drift rather than validation success.

Write mode never reports success with partial writes. After
`rollback-incomplete`, the destination is explicitly non-authoritative and no
later write-mode invocation may report success until the preserved snapshot has
been restored exactly; diagnostics name only the unrestored destination paths
and recovery location, never source payload bytes.

For upstream write mode, the command captures one invocation instant before
materialisation. `internal/baton/version.go` constructs the complete replacement
bytes for `internal/adopt/baton/VERSION` from that instant, tag, SHA and digest
without writing them. S02 constructs and fully validates the pinned installer
archive before the first mutation. The VERSION bytes and archive bytes join the
same byte-sorted destination plan as every mapped vendor file and participate in
the same snapshot, apply, rollback, post-rollback comparison and recovery
transaction. There is no standalone pin or archive write. S01 implements and
proves the mapped-plus-VERSION machinery; S02 expands its declared candidate and
recovery set for the archive, then executes the v0.15.1 content/pin update.

Incomplete rollback is restart-authoritative only through the fixed sentinel
`<git-admin>/sworn/recovery/baton-vendor/rollback-incomplete.json`, where
`<git-admin>` is the physically resolved administrative directory for the
current worktree. The sentinel and its one transaction directory are owner-only:
directories are `0700`, sentinel/manifest/snapshot files are `0600`, and no
symlink may occur at or below the recovery root. The compact deterministic
manifest contains the physical repository root and Git-admin directory plus
destination tuples byte-sorted by canonical repository-relative UTF-8 path; it
does not contain its own transaction identity. Each tuple contains exactly the
destination path, original existence, original four-digit permission mode or
`-`, original byte SHA-256 or `-`, and a recovery-root-relative snapshot path or
`-`. Existing destinations have one regular snapshot whose bytes match the
recorded digest; originally absent destinations have neither mode, digest nor
snapshot. The transaction identity is SHA-256 over the exact compact manifest
bytes plus final LF, and the transaction directory name is the bare 64-hex
digest. The separately serialized fixed sentinel contains the lowercase
`sha256:<64 hex>` transaction identity and points to that transaction directory;
it is not an input to the manifest digest.

A later write invocation that sees the sentinel performs recovery only. Before
touching a destination it requires the record's physical repository/Git-admin
identity to equal the current invocation, every path to be canonical,
repository-confined and one of that transaction's mapped destinations, VERSION,
or the exact installer archive destination, every snapshot path to be canonical
and confined to the named
transaction directory, every recovery component to be non-symlinked with the
required owner-only mode, the manifest digest/directory identity to match, and
the complete material set to contain no missing, duplicate or foreign entry.
Traversal, foreign paths or entries, missing material, symlinks, mode drift,
tampered records/manifests/snapshots, or any bytes/mode/existence mismatch makes
recovery fail with exit 2: recovery material remains, no ordinary vendor plan is
applied, and diagnostics expose paths/classes only. Valid recovery restores each
recorded original, verifies exact bytes, mode and existence for the complete
tuple set, then removes the sentinel and transaction material; that invocation
still exits 2 with explicit re-run guidance and never combines recovery with a
new vendor write.

`sworn baton diff` is the repository-to-embedded read-only parity surface. It
exits 0 only when the complete mapped repository set, normative JSON bytes,
schema classifications, pin fields, and embedded source are exact; it exits 1
for any deterministic missing, extra, changed, misclassified, or pin-drift
finding; and it exits 2 for malformed mapping, unreadable input, or another
operational error. It never inspects or repairs the external Codex and Claude
mirrors and never prints mapped payload bytes.

## 2. Exact parity detection matrix

Every applicable surface must detect its assigned mismatch; success by a
different surface cannot mask it.

| Mismatch | Exact parity test | `sworn baton diff` | `sworn doctor` | `sworn doctor --sync-baton` | isolated Codex/Claude installer proof |
|---|---|---|---|---|---|
| embedded version/tag/SHA/digest/upstream root VERSION blob | fail | drift, exit 1 | ERROR and exit 1 | fail before install, exit 1 | fail before install |
| committed Sworn `internal/adopt/baton/VERSION` manifest bytes or parsed tag/SHA/digest differ | fail | drift, exit 1 | ERROR and exit 1 | fail before install, exit 1 | fail before install |
| mapped vendor file missing/extra/changed | fail | drift, exit 1 | ERROR and exit 1 | fail before install, exit 1 | fail before install |
| normative JSON differs byte-for-byte | fail | drift, exit 1 | ERROR and exit 1 | fail before install, exit 1 | fail before install |
| schema manifest missing/extra/misclassified | fail | drift, exit 1 | ERROR and exit 1 | fail before install, exit 1 | fail before install |
| local Codex mirror stale | fail in isolated fixture | not applicable | ERROR and exit 1 | atomically repair then verify both installations; exit 2 when changed, 0 when both already exact | Codex proof fails before repair and passes after atomic install |
| local Claude mirror stale | fail in isolated fixture | not applicable | ERROR and exit 1 | atomically repair then verify both installations; exit 2 when changed, 0 when both already exact | Claude proof fails before repair and passes after atomic install |
| either mirror cannot be replaced or verified | fail | not applicable | ERROR and exit 1 | exit 1 and restore both mirrors to their complete pre-run trees | installer proof fails |

Canonical Codex output is the complete managed tree produced by the exact
pinned `install-codex.sh` in empty isolated `CODEX_HOME` and `AGENTS_HOME`,
including its command-frontmatter removal, skill-frontmatter construction,
Codex argument-resolution prelude, and documented path rewrites. Canonical
Claude output is analogously the complete managed tree produced by the exact
pinned `install-claude.sh` in an empty isolated home. Sworn-owned `VERSION`
sentinels are checked separately and excluded from the upstream-managed-tree
comparison.

Both exact installer-script oracles run under an explicitly set process umask
`0022`; inherited caller umask is not authority. Canonical managed-tree
directories are therefore `0755` and regular files are `0644`. Native generation
must reproduce those modes, and a hostile inherited umask is a required negative
fixture rather than an alternate canonical result.

Offline installer authority is the single embedded file
`internal/adopt/baton/installer-input-v0.15.1.tar`. It is produced only by the
following argv-equivalent Git archive operation against the pinned commit:

```text
git -C <validated-upstream> archive --format=tar --prefix=baton-v0.15.1/ 3fb4d275ae8a151f6287e7b9279d71628b12eea0 install-codex.sh install-claude.sh baton commands schemas
```

Its exact SHA-256 is
`27d5021cb3ec258a7fd7a5feb6eed92968be0e6cb439e2951da7c6b368e0ca15`
and its Git blob OID is `39ae650dfe0282b0fa8bda14e1a01e7084077702`.
Repository parity verifies both identities and every archived path/mode/blob
against the same upstream tree. The exact shell installers are executed only by
the isolated parity proof to generate reference trees; the shipped binary gains
no shell or external-command dependency. Doctor reads the embedded tar with Go
stdlib `archive/tar`, rejects traversal, links, devices, duplicates,
missing/extra paths or identity drift, and natively applies the exact copy,
frontmatter and path-rewrite rules to a staged tree. The mapped Sworn subset is
never treated as sufficient installer input, and the native output must
byte/mode-match independent exact-script output before installation.

The responsibility boundary is fixed. `internal/adopt/baton_archive.go` is the
only binary embed owner for the tar. `internal/baton/installer_archive.go` owns
deterministic Git-archive construction, hostile-tar validation, and native
Codex/Claude managed-tree generation. `internal/baton/diff.go` owns public
archive parity in addition to its ordinary mapped-file checks.
`internal/baton/vendor.go` and `internal/baton/vendor_transaction.go` own the
single expanded repository transaction and its restart recovery.
`internal/baton/install_transaction.go` owns staging, whole-root replacement,
rollback-incomplete persistence, and recovery-only execution for the three
logical install roots. `cmd/sworn/baton.go` and `cmd/sworn/doctor.go` remain thin
public adapters. Schema `manifest.go`, ordinary `source.go`, and mapped-content
`content.go` do not absorb archive or install-transaction responsibilities.

The exact tagged command inventory contains eight commands. Both native trees
and both independent installer-script oracles must include `design-review.md`
and derive the rest of the set from the validated archive rather than a
hard-coded command count.

`sworn doctor --sync-baton` stages the complete canonical Codex and Claude
managed trees plus their Sworn-owned sentinels, verifies them independently
from the same validated embedded source, then installs both as one
rollback-protected transaction. Success leaves both exact; failure restores
both complete pre-run installations. It exits 0 when both were already exact,
2 only after a successful repair, and 1 after a failed repair and restoration.
The command reports which installation changed, but never prints embedded
prompt, rule, schema, or credential bytes.

Before any snapshot or mutation, the command physically resolves every existing
component of `agents_home`, `codex_home`, `claude_home`, and the Sworn recovery
root without following a symlink as authority. The four resolved roots must be
pairwise disjoint: none may be equal, an ancestor or descendant of another,
inode-aliased through path resolution, or overlap the recovery directory. A
pre-existing symlink, device, socket, FIFO, invalid UTF-8 path, or other
unsupported node beneath a target fails before mutation.

The complete pre-run snapshots, manifest, transaction directory, and fixed
sentinel are owner-only and durably published and directory-synced before the
first target replacement. Sentinel presence is the sole restart authority and
always routes a later sync invocation to recovery-only restoration of all three
pre-run roots, even when process death occurred before rollback began. Normal
success verifies every installed root before atomically retiring and syncing the
sentinel authority. Fault injection kills the process before/after publication,
after each replacement and verification, and before/after retirement; no crash
state may continue a new install or infer success from partially updated roots.

If any restoration step itself fails, the command must not claim restoration.
It exits 1 with class `rollback-incomplete`, reports the byte-sorted repository-
independent target paths that could not be restored, preserves the complete
pre-run snapshots and their mode/blob manifests beneath
`<sworn-config-dir>/recovery/baton-sync/<transaction-sha256>/` with every
directory 0700. The transaction identity is lowercase SHA-256 over the
following exact manifest bytes: ASCII `sworn-baton-sync-rollback-v1` then NUL;
for every entry recursively present beneath the three logical roots
`agents_home`, `claude_home`, and `codex_home`, plus one root-level `absent`
entry when a whole target did not exist, append in unsigned-UTF-8 logical-path
order `<decimal path-byte length without leading zero>:<logical path bytes>NUL`,
then `file`, `directory`, or `absent` plus NUL, then the four ASCII octal
permission digits or `-` plus NUL, then lowercase `sha256:<64 hex>` of regular-
file bytes or `-` plus NUL. Logical paths begin with exactly one of those three
root labels and `/`; no absolute host path enters the digest. Symlinks, devices,
sockets, FIFOs, invalid UTF-8, duplicate logical paths, or other file types fail
preflight before mutation. The complete manifest is stored as `manifest.bin`;
its SHA-256 is the transaction identity.

Before the first replacement the command atomically writes
`<sworn-config-dir>/recovery/baton-sync/rollback-incomplete.json` as compact
UTF-8 JSON with HTML escaping disabled and one final LF. Its exact ordered shape
is `record_version` (constant 1), `transaction_sha256` (`sha256:<64 hex>`),
`recovery_directory` (absolute path), `targets` (array sorted by logical root,
each object ordered `logical_root`, `target_path`, `snapshot_path`), and
`unrestored_paths` (initially empty, later unique unsigned-UTF-8-byte-sorted
logical paths if rollback is incomplete). No map participates in serialization.
The sentinel names that directory, digest, exact target roots and unrestored
paths and is the sole recovery authority; updating `unrestored_paths` after a
failed restoration never changes the transaction identity or snapshot set.
Payload bytes are never printed.
`sworn doctor` treats the sentinel as ERROR. A later
`sworn doctor --sync-baton` enters recovery-only mode: it
accepts no new canonical install, revalidates the sentinel/snapshots and target
paths, retries only the recorded restorations, and removes recovery material
only after both complete pre-run trees and sentinels verify. Successful recovery
returns 2 with explicit re-run guidance; failed or tampered recovery remains
`rollback-incomplete`, exit 1. Fault injection must cover each install,
verification, rollback and recovery step.

## 3. Typed-reference failure vocabulary and precedence

Resolution follows the exact Baton v0.15 order:

1. duplicate-safe parse and validate the reviewed spec;
2. obtain the physical workspace root from the repository containing it;
3. resolve the reviewed spec physically beneath that root and require the exact
   canonical repository-relative source path;
4. validate slash-separated lexical paths (no NUL, backslash, leading or
   trailing slash, empty segment, `.` segment, `..` segment, or POSIX-clean
   change);
5. join and resolve physically beneath the workspace root;
6. test existence, regular-file type, readability, and UTF-8 in that order;
7. for generated `contract` and `slice` targets only, parse duplicate-safe JSON,
   validate the applicable embedded schema, and validate loaded release; then
8. require the exact contract-ID count or sibling slice-ID equality.

Failures in steps 1 through 5 fail the check before model dispatch with this
exact precedence: `reviewed-spec-schema-invalid` →
`workspace-root-unavailable` → `reviewed-spec-source-path-mismatch` →
`reference-path-invalid` → `reference-path-escape`. Safe failures in steps 6
through 8 are supplied to the ambiguity model, never silently omitted, as:

```text
UNRESOLVED <reference-key>: <missing|non-regular|unreadable|invalid-utf8|invalid-json|schema-invalid|record-release-mismatch|contract-id-missing|contract-id-duplicate|slice-id-mismatch>
```

That list is also the exact per-reference precedence. Resolved artifacts render
as `--- ARTIFACT <repo-relative-path> ---`, LF, verbatim UTF-8 bytes, LF, sorted
bytewise by resolved path and deduplicated by path. Unresolved entries follow,
sorted bytewise by the fixed key `contract:<id>`, `slice:<id>`, or `file:<path>`.

A kind `slice` reference is same-release. A kind `file` reference is an explicit
raw UTF-8 workspace file; JSON-looking file content is not recursively treated
as a Baton record and receives no implicit release comparison. A kind `file`
may therefore intentionally pin a cross-release fixture. Only generated
contract and sibling-slice targets receive the record-release check. Resolution
is one level deep and performs no network fetch.

## 4. Command policy and protocol activation

The exact `release-protocol-authority-v1` object has required top-level members
`$schema`, `record_version`, `release`, `protocol_pin`, `origin`, and
`authority`, with no unknown or duplicate key at any depth. `protocol_pin` has
exactly `name`, `version`, `upstream_sha`, `upstream_digest`, and
`upstream_version_blob_oid`. That OID names the upstream tag's root `VERSION`
blob—exactly `v0.15.1` plus LF at C-01—and never the adopting repository's
multi-line manifest. `origin: native` forbids
`migration_receipt_path`. `origin: migrated` requires
`migration_receipt_path` to equal the canonical repository-relative path
`docs/release/<release>/protocol-migration-receipt.json`; that path must resolve
as a regular committed file beneath the physical release root. The exact
`$schema` value is `https://swornagent.dev/schemas/release-protocol-v1.json`,
`record_version` is 1, and `authority` is exactly `planning` or `current`.

For live authority, each participating ref separately resolves committed
`internal/adopt/baton/VERSION`. Those adopting-manifest blobs must be identical
across participants and their parsed `baton-protocol`, `upstream-sha`, and
`upstream-digest` fields must equal `protocol_pin` and the running binary. The
marker's upstream root VERSION OID and the participating manifest blob are two
different identities; neither may substitute for the other.

The policy tables below govern native Sworn handlers, not the temporary human
bootstrap roles used to build this self-hosting release. While the marker is
`planning`, T1 and T2 are integrated by fresh Track Integrator sessions
following the pinned Baton v0.15.1 `/merge-track` role verbatim, never by the
current-only native `sworn merge-track` handler. For each dependency-ready
track, the role reads the board through the read-only oracle, revalidates the
complete lifecycle/rollback/integration-ready predicate, performs Baton's
canonical forward-sync and regression gates, creates the exact two-parent
track integration commit on `release-wt`, and re-renders only `index.md`; the
pure-plan board stays byte-identical and no worktree or branch is cleaned. The
session puts the freshly verified track/release-wt binary first on `PATH` for
the oracle and non-writing regression command, never the pre-v0.15 installed
binary. The Track Integrator itself does not push; after it returns the exact
merge/projection commits, the orchestrator performs one non-force push of that
local `release-wt` head only when `origin/release-wt` still equals the pinned
pre-operation head. Before an integration commit exists, an interrupted merge
is aborted and the clean pre-operation release/track heads are revalidated.
Once the exact canonical integration commit exists, a missing deterministic
index projection is the only permitted local continuation; once both exact
commits exist, a missing remote update permits only the compare-and-swap push.
Any other parent/tree/provenance/projection/ref shape blocks and the integration
is rebuilt from the retained clean refs. S13's cutover validator
owns recognition of these two bootstrap integrations and refuses activation
unless all are exact. After C-13 activation, every T5/T6/T7 integration uses
the current-authority native Sworn handler.

Every registered CLI spelling and alias carries exactly one policy enum. An
unregistered spelling exits 64 before a handler. A newly registered spelling
without a policy fails registry tests and cannot dispatch.

| Policy | Exact CLI operations |
|---|---|
| `protocol_independent` | no-argument TUI; `help`, `-h`, `--help`; `version`, `-v`, `--version`; `capabilities`; `init`; `verify`; `bench`; MCP transport startup before a tool is selected; `doctor`; `baton vendor`; `baton diff`; `account` including `buy`, `set-webhook`, `notifications`, and `default`; all `telemetry`; all `memory`; all `induction`; `ledger report`; `ledger recommend`; `login`; `logout`; `models`; default journey elicitation |
| `read_only_inspection` | `board`; `route`; `top`; all deterministic `lint`; `reqvalidate`; `designfit`; `journeys --check`; `journeys --impact`; `specquality`; `designaudit`; `render`; `ledger sync`; future `baton protocol check`; future `baton protocol inspect` |
| `planning_validation` | `reqverify`; generic `llm-check` except the retired maintainability spelling; non-writing `replan validate`; non-writing `regress` |
| `activation` | native `maintainability cutover <release>`; pristine `replan migrate --delta <path>`; migrated `replan activate --delta <path>` |
| `current` | `loop` and alias `run`; `ship`; `merge-track`; `merge-release`; `journeys --regen`; `maintainability review`; `maintainability adjudicate`; `mark-shipped`; ordinary `replan apply` |
| `retired` | generic `llm-check --type maintainability-review`; exit 64 with the dedicated-command guidance, zero dispatch and zero mutation |

`render` may write only the byte-exact deterministic `index.md` projection.
`ledger sync` may write only its deduplicated, non-authoritative ledger
projection outside release roots. Neither operation may mutate a canonical
release record or ref.

Every MCP tool also has one operation-level policy before its handler:

| Policy | Exact MCP tools |
|---|---|
| `protocol_independent` | `get_induction_status`; `get_considerations`; `search_decisions`; `record_decision`; `check_design_system`; `update_design_system`; `record_architecture_pattern`; `get_credits` |
| `read_only_inspection` | `get_board`; `get_blocked`; `get_slice_context`; `list_releases`; `sworn.lint`; `sworn.lint_trace`; `sworn.lint_coverage`; `sworn.lint_design`; `sworn.lint_mock` |
| `planning_validation` | `sworn.llm_check` except `type=maintainability-review` |
| `activation` | `migrate_plan_delta`; `activate_migrated_plan` |
| `current` | `rerun_slice`; `patch_slice`; `approve_merge`; `defer_slice`; `apply_plan_delta` |
| `retired` | `sworn.llm_check` with `type=maintainability-review`; `plan_release`; `create_slice`; `set_track`; `update_intake` |

The four retired direct planning writers return dedicated Planner/delta-tool
guidance with zero mutation. Future C-12 MCP tools call the same engine
operations as their CLI counterparts and may not fork transaction semantics.

Subcommands and MCP tools override only the transport's top-level policy. A
release-aware read-only inspection validates the committed marker if present
and may inspect a planning or historical marker, but cannot return current
authority. Planning validation requires the exact matching pin and
`authority: planning` or `current`. Current policy requires `authority: current`
before its handler and ordinary `replan apply` rejects
`operation: protocol_migration`. Activation is the sole policy exception: a
native planning marker may invoke only C-13 `maintainability cutover`; an absent
pre-v0.15 marker plus a C-12 pristine source may invoke only `replan migrate`;
and a migrated planning marker plus its exact committed C-12 receipt may invoke
only `replan activate`. The spelling fixes the edge before any mutation; flags,
aliases, or delta content cannot reclassify another command.

Historical archive inspection resolves the requested record blob at the
authority commit, then walks first-parent history newest-to-oldest while that
path resolves to the exact same blob OID. The evidence commit is the oldest
commit in that uninterrupted equal-blob suffix: its first parent is absent,
lacks the path, or contains a different blob. Deletion and later reintroduction
therefore selects the most recent introduction, never an older occurrence of
the same path/blob pair. The adopting-repository version evidence path is
exactly `internal/adopt/baton/VERSION`. Missing or shallow history, an
unprovable boundary, or a missing VERSION blob fails validation.

`maintainability cutover` is owned by S13 and is the only native
planning-to-current transition for this release:

1. complete and freshly verify S01 through S13 under the ratified manual v0.15
   bootstrap, integrating T1 then T2 with the exact
   pinned Track Integrator transaction above while the marker remains
   `planning`;
2. before creating T5 or T6, run in the clean primary release worktree on the
   exact local and remote `release-wt/<release>` head; no downstream track ref
   may already exist;
3. require the exact committed planning marker and embedded pin, canonical T1
   and T2 bootstrap integration ancestry, S01 through S13 verified evidence, the S01 through S13
   engine cutover reproduction, and current schema, trace, requirements,
   design-fit, spec-quality, and ambiguity PASS results;
4. write only `protocol.json.authority` from `planning` to `current`; every
   other marker byte remains unchanged;
5. commit once on `release-wt` with before/after marker blob OIDs in the commit
   body, push that exact ref, and report success only after the remote contains
   it; and
6. on rerun, fail on any byte mismatch; if the exact commit exists locally but
   not remotely, perform only the missing push; if it is already remote,
   revalidate every prerequisite before idempotent success.

No caller selects the ref, target state, participant set, force, waiver, or
downgrade. Completed T1 and T2 track refs are historical evidence, not
current participants. C-12's separately named migrated-release activation does
not accept a native marker and cannot target this conformance release. No other
handler auto-promotes or accepts planning authority. T5 and T6 are materialized
only from the activated release-wt head, so later current operations compare the
active owner and assembly refs against the same current marker.

### Planning-authority bootstrap adapter for per-slice Gate 8

`authority: planning` is not a maintainability waiver. Before S13 makes the
generalized stateful command authoritative, the human-ratified bootstrap roles
must execute the exact operation manually for every S01 through S13 slice. The
adapter is the role session plus deterministic Git and record tooling; its CLI
spelling is deliberately non-normative. It may not use the installed legacy
generic `llm-check --type maintainability-review` result, because that binary's
pre-cutover prompt and report schema do not carry the exact v0.15.1 semantic
identity.

For each Implementer preflight and fresh Verifier authoritative run, the adapter
MUST apply the exact Baton v0.15.1 `baton/llm-checks/README.md` bounded
maintainability lifecycle and `maintainability-review.md` prompt at pinned
upstream commit `3fb4d275ae8a151f6287e7b9279d71628b12eea0`:

1. require a clean track worktree and use the immutable `start_commit`; the
   Implementer head is the stable committed implementation/proof checkpoint and
   the Verifier head is only the resulting pinned `implementation_head`;
2. derive first-parent non-merge candidate paths, validate every synchronization
   merge and slice/merge overlap, exclude only the exact release-record,
   generated and lockfile classes, and construct the exact mode/object manifest,
   `sha256:` fingerprint and canonical UTF-8 binary-capable prompt diff;
3. invoke a fresh role-isolated model at temperature 0 with the untouched tagged
   prompt, reject prose or duplicate-key output, derive PASS/FAIL from blocking
   findings, and validate the completed identity fields against the exact tagged
   `llm-check-report-v1` schema;
4. write each immutable full report beneath
   `<slice>/reports/maintainability/<role>-cycle-<cycle>-<invocation-id>.json`,
   commit it, resolve its committed Git blob id, then append the matching unique
   ledger entry to `status.json` and commit/push the role transition; and
5. reject report reuse unless every fingerprint, review-scope, role, phase,
   cycle, invocation, path and committed-blob identity matches. Adapter failure
   is a failed gate, never a reason to synthesize a report or retain
   `maintainability.state: pending` at an implemented/verified boundary.

The adapter grants no public command authority, lifecycle automation, general
merge composition, adjudication, rollback, or cutover behavior. S06 through S13
still deliver those product operations. S13 must reproduce the scope and ledger
validation for every S01 through S13 bootstrap report before it may change only
`protocol.json.authority` from `planning` to `current`.

## 5. Canonical semantic scope

Scope construction requires `git status --porcelain=v1 -z --untracked-files=all`
to emit zero bytes for the worktree/index being reviewed. Dirty or untracked
bytes never enter semantic identity; their presence fails before a partial
scope or model dispatch. Identical committed histories in separately clean
checkouts remain identical despite locale, Git config, attributes, external
drivers, or textconv configuration.

The physical release-record root is the workspace-confined, symlink-resolved
grandparent directory of the reviewed slice's physical `status.json`. A path is
a release record when it is strictly beneath that repository-relative root.
The complete release root is excluded, including release-level board, contract,
protocol, intake, rendered index, planning, screenshots, and every sibling slice
record/evidence path. A similarly named path outside that exact prefix remains
semantic input.

The exact excluded lockfile basenames are:

```text
package-lock.json
pnpm-lock.yaml
yarn.lock
bun.lock
bun.lockb
go.sum
Cargo.lock
poetry.lock
Pipfile.lock
Gemfile.lock
composer.lock
```

The semantic manifest is byte-exact Baton v0.15. It begins with the exact ASCII
bytes `baton-maintainability-v1` followed by NUL. For each included path in
unsigned UTF-8 byte order it then appends, without whitespace:

```text
<base-10 path byte length without leading zero>:<path bytes>NUL
<base mode or ->NUL
<full base object id or ->NUL
<head mode or ->NUL
<full head object id or ->NUL
```

There is no final record count or additional terminator beyond the last field's
NUL. Empty scope is exactly the header followed by NUL and has no path records.
The fingerprint is `sha256:` plus lowercase SHA-256 of those exact bytes.

C-04 separates immutable semantic identity from a moving freshness frontier.
The semantic identity contains the exact release, slice, canonical physical
status path, immutable start/base, requested review head, exact manifest, and
fingerprint. The freshness frontier contains the track/ref names, the validated
track head and release-wt head, and every recognized synchronization
merge/parents/base tuple through which freshness was proven. Missing optional
Git identity is represented as null in that typed frontier, never inserted into
the manifest.

Report equality and reuse compare only the immutable semantic and lifecycle
identity. Moving heads are never equality members. On recovery, the current
track head must descend from the stored frontier, and every intervening commit
must be release-record-only or an exact recognized synchronization with no
candidate overlap. A changed current head therefore triggers deterministic
freshness revalidation, not automatic redispatch or identity mismatch. Any
accepted path tuple change changes the exact upstream manifest and fingerprint;
an invalid or unrecognized frontier blocks authority.

The prompt diff runs from a temporary bare Git directory whose object store
points read-only at the reviewed repository and whose `info/attributes` is
absent. Process environment suppresses system, global, and inherited local diff
presentation, attributes, external drivers, and text conversion without adding
or changing Git arguments. With each included path appended as one distinct
literal pathspec argv entry in unsigned UTF-8 byte order, the exact effective
Baton v0.15 invocation is:

```text
LC_ALL=C git diff --no-ext-diff --no-textconv --no-color --no-relative --binary --full-index --no-renames --submodule=short --ignore-submodules=none --diff-algorithm=myers --no-indent-heuristic --unified=3 --inter-hunk-context=0 --no-function-context --src-prefix=a/ --dst-prefix=b/ --line-prefix= <base>..<head> -- <each byte-sorted path as :(literal)<path>>
```

The command is executed as an argv vector, never a shell string; the empty value
for `--line-prefix=` is one argument, and `<base>..<head>` is one range
argument. An empty included set emits zero diff bytes without invoking Git and
the review operation persists the deterministic schema-valid PASS report for
that exact empty C-04 identity with zero model dispatch. It does not synthesize
a prompt call merely to confirm that there are no semantic bytes to review.
Non-zero Git exit, invalid UTF-8 output, an unexpected object type, or any path
not equal to the previously accepted literal set fails scope construction
without dispatch.

## 6. Recognized synchronization and release-record composition

For a candidate synchronization merge `M`, parent 1 is the current track head
`P1`, parent 2 is the exact `release-wt/<release>` head `P2`, and there are
exactly two parents. The second parent must equal or appear on that release-wt
ref's first-parent chain. `git merge-base --all P1 P2` must return exactly one
base `B`.

For every non-record path in the byte-sorted union changed by `B..P1`, `B..P2`,
or `P1..M`, derive expected mode/object tuple `E`:

1. if P1 and P2 tuples are equal, use that tuple;
2. else if P1 equals B, use P2;
3. else if P2 equals B, use P1;
4. else require the exact path in validated `board.shared_touchpoints`, require
   both actual contributing tracks declared with distinct non-empty regions,
   require three same-mode regular blobs, and use successful
   `git -c diff.algorithm=myers merge-file --object-id P1 B P2`; or
5. fail as undeclared/custom composition.

`M` must equal `E` for every non-record path, including absence. The structural
release-wt segment newly introduced by P2 may contain only planner non-merge
commits under the release root and canonical two-parent `/merge-track`
integrations whose second parent is the retained declared track ref and whose
outside-record tree equals that second parent exactly.

Release-record paths use the same pathwise three-way rules 1 through 3. If both
parents changed a release-record path to different blobs, synchronization is
unrecognized unless both already carry an identical C-12 propagated blob; no
manual JSON/Markdown merge is licensed. The generated `index.md` is the sole
exception: discard both parent versions and require the exact deterministic
render of the composed board/spec/status records. This rule naturally preserves
the current track's owner status when release-wt is unchanged, takes newer
sibling/planner records when the track is unchanged, and requires the protocol
marker to remain byte-identical through T2 integration. The separate C-13
post-T2 cutover is the only operation permitted to change that marker from
planning to current.

#### Sealed pre-C12 historical reconciliation evidence

Before C-12 existed, manual planning authority created four synchronization
merges that are already inside the immutable S01 and S02 maintainability
intervals but do not satisfy the release-record paragraph above. Rewriting them
would invalidate S01's committed report heads/blob identities and S02's
immutable `start_commit`; silently accepting their shape would create a generic
manual-merge waiver. Neither is permitted.

For this release only, while `protocol.json.authority` is exactly `planning`,
the schema-valid sealed manifest
`planning/bootstrap-sync-reconciliations.json` at exact SHA-256
`f9e0de63c0a5ecf15cdb6058a52166ff0a609fa0d0cf2ecdf81d7955030b1943`
is historical recognition evidence. Its only entries are exact merges
`36d1bd56cdc12ddc824533e24d385ef9e8cc550a`,
`d062d055cdbe90e8290f0bf47574be660cd9a675`,
`b8df1857ab8c7d2acf4b91c4c37df3cf07f80832`, and
`7696c9bf9c235fffb937d3ed7e4be5a8a2bbda2a`.

Each entry also carries one exact authorization envelope: owning consumer
slice, immutable `start_commit`, exact `review_head`, interval semantics, and
permitted purpose. `first-parent-start-exclusive-head-inclusive` means the
merge must appear in the first-parent ancestry path `start_commit..review_head`:
the start is excluded and the exact review head is included. The consumer must
present that same slice, status start, requested review head, and one of the
entry's exact purposes (`semantic_scope`, `freshness_revalidation`, or
`bootstrap_cutover_revalidation`). A descendant or ancestor review head, the
same merge requested by another slice, or another purpose is not authorized.
Cutover uses the owning S01/S02 envelope to reproduce that slice's evidence; it
does not substitute S13 as the consumer.

Recognition requires all of the following, fail-closed:

1. the manifest validates against
   `planning/bootstrap-sync-reconciliations-v1.schema.json`, has no duplicate
   JSON keys, and its committed bytes reproduce the pinned digest;
2. release, planning authority, consumer slice, status `start_commit`, exact
   requested `review_head`, first-parent interval membership, permitted purpose,
   merge OID, ordered parents, unique merge base, every exceptional path, and
   every base/parent-1/parent-2/result mode-and-blob tuple equal the manifest
   exactly;
3. the observed exceptional path set equals the listed set exactly, while
   topology, release-wt structural ancestry, every non-record path, every
   unlisted release-record path, and deterministic `index.md` rendering still
   satisfy the ordinary rules in this section; and
4. the consumer uses the entry only for the exact authorized slice interval and
   purpose. It cannot create or recompute a merge, accept another result, widen
   either endpoint, select a caller-supplied exception, authorize another slice,
   or authorize any future reconciliation.

The separately audited pre-start merges `9f1d499c595b098e14b329e377f901bd55dfecaf`,
`5e4a7b3410e975ce7507a13291d3411749bd680e`, and
`1ee6c319505b7dd466485bff406f11828cf0507f` remain unrecognized because they
are below the relevant immutable review starts. If a later operation proves it
must traverse one, that operation blocks and returns to the Planner; the sealed
manifest is not extended in place. Any S01/S02 review-head change likewise
blocks until a new human-ratified planning amendment and digest replace the
authorization; consumers never widen it dynamically. C-13 revalidates every
consumed entry, authorization envelope, interval, and the manifest digest before
cutover, after which the exception is unavailable.

## 7. Lifecycle transition table

Before lifecycle evaluation, C-05 treats every full-report finding `id` as its
decoded UTF-8 byte string with no Unicode normalization or case folding. Any
duplicate finding ID anywhere in one full report makes that report
non-authoritative, even when the duplicate findings have different blocking
values. The compact `blocking_finding_ids` array is exactly the IDs of findings
whose `blocking` member is true, sorted by unsigned UTF-8 bytes; it is never
deduplicated, rewritten, or severity-derived. PASS requires that array to be
empty and FAIL requires it to be non-empty, matching the report's derived
verdict.

Every raw model response validates against exact Baton's
`llm-check-report-v1` plus the finding-disposition constraint in
`planning/sworn-maintainability-raw-response-v1.schema.json`. That committed
raw schema expressly forbids the engine-owned `sworn` member. The vendored
prompt and schema bytes remain untouched; prose is never scraped for
disposition. After the engine adds its top-level provenance, the complete
persisted report validates against exact Baton and
`planning/sworn-maintainability-report-overlay-v1.schema.json`. Both Sworn
schemas restrict every Git object ID to exactly 40 or exactly 64 lowercase hex
characters; intermediate lengths are invalid.

Every blocking finding carries:

```json
"sworn_disposition": {
  "action": "remediate_in_scope",
  "required_touchpoints": ["path/a.go"]
}
```

or the exact action `re_slice`. The action is required only for a blocker.
`required_touchpoints` is non-empty, unsigned-UTF-8-byte-sorted,
duplicate-free, and each path passes C-02 lexical and physical confinement.
`remediate_in_scope` requires every path to equal a `spec.touchpoints` member.
`re_slice` is authoritative even if every named path is already a spec
touchpoint, because an ownership or architecture boundary can change without a
new path. A non-blocking finding must omit `sworn_disposition`. Missing,
malformed, unsorted, duplicate, or unconfined structured disposition makes the
report non-authoritative before lifecycle evaluation.

The engine then adds one top-level `sworn` object before persistence. It carries
`record_version: 1`, immutable `scope_provenance` (release, slice, status path,
start commit, review base, and review head), and the C-04 `freshness_frontier`.
The persisted report validates against both schemas. No provenance member is
inserted into Baton's closed `review_scope` or compact status entry; the compact
entry's committed `report_path` and `report_blob_oid` pin the complete report.

The path boundary set is the sorted unique union of all blockers'
`required_touchpoints`. A FAIL is mechanically in-scope only when every action
is `remediate_in_scope` and every boundary member equals a spec touchpoint. Any
`re_slice` action or outside member is boundary-expanding. No path prefix,
directory ownership, Unicode normalization, case folding, or prose inference
widens the ratified boundary.

Each cycle starts with one Implementer preflight. `implementation_head` is set
only by the newest Implementer PASS and is retained through a Verifier PASS. It
is set to null on any transition to `needs_coach` or `re_slice_required`; the
historical reviewed heads remain immutable in the full reports and compact
ledger.

| Cycle | New report/result | Required next state | Next authority |
|---:|---|---|---|
| 0 | preflight PASS | `passed` | one fresh Verifier authoritative review |
| 0 | in-scope preflight FAIL | `pending` | one bounded remediation, then one closure review |
| 0 | boundary-expanding preflight FAIL | `needs_coach` | Coach: `re_slice` only |
| 0 | closure PASS | `passed` | one fresh Verifier authoritative review |
| 0 | closure FAIL | `needs_coach` | Coach: `resume_in_scope` or `re_slice` |
| 0 | Verifier PASS | `passed` and overall slice may become `verified` | none |
| 0 | Verifier FAIL | `needs_coach` | Coach: `resume_in_scope` or `re_slice` |
| 1 | preflight PASS | `passed` | one fresh Verifier authoritative review |
| 1 | in-scope preflight FAIL | `resume_approved` (cycle and adjudication remain 1) | one bounded remediation, then one closure review |
| 1 | boundary-expanding preflight FAIL | `re_slice_required` (retain immutable cycle-1 adjudication) | re-plan only; no second Coach |
| 1 | closure PASS | `passed` | one fresh Verifier authoritative review |
| 1 | closure FAIL | `re_slice_required` | re-plan only |
| 1 | Verifier PASS | `passed` and overall slice may become `verified` | none |
| 1 | Verifier FAIL | `re_slice_required` | re-plan only |

"Any cycle-1 blocking failure" means a terminal cycle result: boundary-expanding
preflight FAIL, closure FAIL, or Verifier FAIL. An in-scope cycle-1 preflight
FAIL is not terminal until its sole closure budget is consumed.

Coach authority is explicit. The caller supplies `decision`, non-empty
`rationale`, and non-empty `approved_by`. For `resume_in_scope`, the caller also
supplies non-empty unique `permitted_touchpoints`, each exactly equal to a
`spec.touchpoints` member; equality with the whole set is allowed. A `re_slice`
request forbids `permitted_touchpoints`. The command derives the eligible
report invocation IDs, report fingerprints, blocking finding IDs, and citation
set only from committed terminal evidence. It captures `approved_at` once from
its injected clock during the atomic transition and reuses that unchanged value
on recovery; callers cannot supply or override it. Missing, inferred,
decision-incompatible, or invalid authority fields fail before mutation. The
public and internal adapters share one typed request carrying exactly these
Coach-supplied fields.

`adjudication.blocking_finding_ids` is the unsigned-UTF-8-byte-sorted unique
union of `blocking_finding_ids` from every cited report, without normalization
or case folding. Report invocation IDs and fingerprints retain chronological
citation order. Any omission, addition, duplicate, or order mismatch makes the
adjudication non-authoritative. The eligible citation set is the terminal
cycle-0 FAIL report and the immediately preceding report that authorized or led
to it:

| Terminal failure | Exact two cited reports |
|---|---|
| closure FAIL after preflight FAIL | that preflight FAIL and that closure FAIL |
| Verifier FAIL after preflight PASS | that preflight PASS and that Verifier FAIL |
| Verifier FAIL after closure PASS | that closure PASS and that Verifier FAIL |

A boundary-expanding cycle-0 preflight enters `needs_coach` with that one report
as the cited authority, but can never authorize `resume_in_scope`; only an
accepted Coach `re_slice` decision transitions it to `re_slice_required` with a
cycle-0 `re_slice` adjudication. A boundary-expanding cycle-1 preflight is
terminal immediately: it transitions to `re_slice_required` while retaining the
existing immutable `resume_in_scope` adjudication and invokes no second Coach
decision. Coach decisions consume zero model dispatches. No report may be
appended after a terminal row.

The cycle-1 remediation boundary is Git-derived, not caller-asserted. Set
`resume_base_head` to the review head of the last report cited by the immutable
cycle-0 adjudication. Before cycle-1 preflight and closure, enumerate
first-parent non-merge authored paths in
`resume_base_head..requested_review_head` using C-04's NUL-safe, no-renames
algorithm. Exclude only release-record paths and recognized merge-only
contributions; authored/merge overlap fails closed. Every remaining path must
equal an immutable `adjudication.permitted_touchpoints` member. An outside path
transitions immediately to `re_slice_required` before dispatch. A cycle-1
preflight whose structured disposition is in the permitted set receives one
bounded remediation and exactly one closure; a `re_slice` disposition or path
outside the permitted set performs zero remediation, closure, Verifier, or
second-Coach dispatches. The model still reviews the exact full
`start_commit..review_head` semantic diff; this resumed-delta guard is additional
authorization, not a reduced review scope.

## 8. Evidence intervals and rollback

Recognized synchronization uses section 6 exactly. A release-record change is
permitted only when its expected result is produced by the release-record rule
there; every other release-record result makes the sync unrecognized.

For ordinary rollback, the envelope is every authored path in first-parent,
non-merge semantic history from the failed original slice's immutable
`start_commit` through the verified rollback slice's pinned
`maintainability.implementation_head`. It includes the original interval,
unowned gaps, rollback-slice-authored paths, generated files, and lockfiles.
Exclude only the physical release-record root and recognized merge-only
contributions; authored/merge overlap fails closed. Do not subtract a later
active authority. Unexpected later ordinary history is a provenance failure
requiring track reconstruction. At rollback head, every envelope path's mode,
object ID, or absence must equal the original `start_commit` tree exactly.

Post-sync rollback is distinct. Its candidate interval ends at the recorded
`invalidated_review_head`, its rollback baseline is exact synchronization parent
2, and every later authoritative candidate interval must be disjoint. Overlap
blocks automatic rollback.

After the verified frontier, a recognized two-parent synchronization whose
contribution intersects the invalidated candidate set and is disjoint from all
later authoritative candidates produces one deterministic invalidation plan:
preserve the report ledger and adjudication; clear
`maintainability.implementation_head`; set maintainability state to
`re_slice_required`; set overall state to `failed_verification`; set
verification result to `fail`; and persist in status and journal the exact
`invalidated_review_head`, `invalidating_sync_merge`, and parent-2
`rollback_baseline_commit`. The Track Integrator commits that transition locally
on the track branch, performs no push or release-wt merge, returns non-success,
and routes to `/replan-release`.

If any later-authority overlap exists, the synchronization is unrecognized or
custom, or shipped authority would be affected, the classifier blocks with no
lifecycle mutation or ref advance. S11 only returns this mutation-free
classification and transition plan; S15 owns the permitted local commit.

## 9. Deployment evidence

`sworn mark-shipped <release> [--deployed-commit <full-oid>]
[--deploy-ref <note>]` is the native public adapter for exact Baton v0.15.1
`/mark-shipped`. Before any ordinary or nothing-to-do result it requires the
primary clean worktree on the declared integration branch,
fetches `origin`, and requires `origin/<integration>` to be an ancestor of or
equal to local integration history; local-behind or divergent history blocks.
It then performs Baton's complete schema, committed maintainability, provenance,
rollback and terminal-state gates across every slice. If no slice remains
`verified`, it returns Baton's exact successful nothing-to-do result without
requiring or resolving deployment evidence, a release-merge identity, or a
timestamp.

When at least one slice is `verified`, `--deployed-commit` is required by the
non-interactive Sworn CLI. The command resolves that commit and searches the
declared integration history for Baton's conventional release-merge commit. If
the release-merge identity exists, the deployed commit must contain it. If it
does not exist (the exact upstream legacy-release case), containment validation
is skipped and the Step 5 commit body records the skip. The command then
captures one UTC timestamp. Each transitioned status
sets `last_updated_by` to `mark-shipped`, sets `last_updated_at` to the one
captured timestamp, and gains the exact immutable Baton `ship` block:

```json
{
  "shipped_at": "<ISO-8601 UTC timestamp>",
  "deployed_commit": "<deployed-commit full SHA>",
  "deploy_ref": "<human-readable note, or null>",
  "shipped_by": "/mark-shipped"
}
```

The command changes only currently `verified` slices to `shipped`; it leaves
already-shipped and C-09-valid legal deferred slices untouched (including an
unstarted planned-intent deferral whose status remains `planned`), preserves every
verification record, validates and preserves `board.json` byte-identically,
and re-renders `index.md` from that pure plan plus the authoritative statuses.
The rendered slice/aggregate views derive from lifecycle state and Recent
activity derives the ship event from the new `ship` blocks; neither becomes
new board authority. It stages only the rendered index and the explicit
affected status paths and creates exactly one local
integration-branch bookkeeping commit whose subject is
`docs(release/<release>): mark <N> slices shipped — deployed <short SHA>` and
whose body records the release, deployed commit/ref and byte-sorted shipped
slice list. It never pushes, merges, builds, deploys, deletes branches or cleans
worktrees. The handoff names the commit, the transitioned count, the exact
integration push command, and exact derived worktree-removal/branch-deletion
suggestions which it does not execute.

The nothing-to-do result writes nothing, does not rewrite existing `ship`
blocks and does not recreate the prior handoff. There is no push-only recovery
phase: a rerun after the local
bookkeeping commit is the same gated already-shipped no-op whether or not a
human has pushed it, provided fetched origin is not ahead or divergent.

## 10. Crash and restart outcomes

| Durable point observed on restart | Required outcome | Model/Planner redispatch |
|---|---|---:|
| no report or transaction write | validate current budget and start a normal authorized attempt | one only if budget permits |
| standalone `maintainability review`/`adjudicate` sees report-only or any coherent-looking but uncommitted dirty report/status/journal set | fail non-zero, preserve bytes, name partial-local-evidence; the stateful command does not infer that it owns an interrupted loop | 0 |
| resumed `sworn loop` sees any uncommitted tracked or untracked debris | assert the expected owner-track path and branch, select the validated committed local owner-track head, run `git reset --hard` to that head and `git clean` for untracked debris, prove the worktree clean, then reconstruct the next legal committed phase | 0 for completed phases; one only for a genuinely missing legal phase when budget permits |
| exact coherent report/lifecycle transition commit whose C-10 operation authorizes push exists locally, remote lacks it | validate report/blob/history/commit identity and push only that commit | 0 |
| exact pushed report/lifecycle transition commit already on remote | revalidate and return its recorded PASS/FAIL outcome idempotently | 0 |
| exact C-08 local invalidation commit exists on the track | revalidate the tuple, preserve the unchanged remote and release-wt refs, return non-success with re-plan routing | 0 |
| exact planning-authority bootstrap Track Integrator merge exists locally but its deterministic `index.md` projection commit is missing | validate the pinned track/release heads and canonical merge parents/tree/provenance, render only the board/status/ref-derived index, create only the projection commit, then perform the compare-and-swap release-wt push | 0 |
| exact planning-authority bootstrap merge plus projection commits exist locally while remote release-wt remains at the pinned pre-operation head | revalidate both commits and every gate, then push only that exact local release-wt head without force | 0 |
| bootstrap release-wt history, projection bytes, local/remote head, or retained track ref differs from either exact state above | fail non-zero without mutation or native merge-track fallback; rebuild from retained clean refs under the pinned Track Integrator role | 0 |
| exact C-11 local mark-shipped status/index bookkeeping commit exists | fetch origin, revalidate the complete read-only upstream gate, byte-identical pure-plan board and exact existing terminal records, then return Baton's successful nothing-to-do result without mutation, handoff or push | 0 |
| exact C-12 release-wt transaction commit exists with only some local or remote target refs advanced | validate the committed receipt and advance each missing ref only from its separately pinned local or remote expected head; preserve an exact C-08/S15 invalidation as propagation parent 1 | 0 |
| exact C-12 migrated activation commit exists with only some declared refs advanced | validate the unchanged receipt, gates, transaction and activation topology and advance only missing exact activation commits | 0 |
| any rewritten, extra, divergent, or unexpected local/remote bytes | fail non-zero without mutation | 0 |

The public conformance suite kills processes at each row. A standalone stateful
command treats "after report write" as partial local evidence and fails without
mutation. A resumed loop instead owns the exact Baton process-global restore:
it must target-assert before destructive Git operations, preserve all committed
progress, discard every uncommitted leftover, and redispatch only after the
worktree is clean. Zero-dispatch reuse begins once the exact coherent transition
commit exists.

## 11. Re-plan transaction topology

The submitted delta's canonical path is
`docs/release/<release>/plan-deltas/<delta_id>.json` on the derived Planner ref
`plan/<release>/<delta_id>`. That ref is one canonical non-merge commit whose
sole parent is `source.commit_oid` and whose tree equals the source tree except
for creation of that one delta path. The containing commit uses author and
committer `SwornAgent <noreply@swornagent.dev>`, the ratification timestamp for
both Git dates, and subject `docs(replan): ratify <delta_id>`. The delta's
`source.ref` names the release-wt ref to mutate and `source.commit_oid` names the
pre-transaction base; neither field claims the containing Planner commit, so no
self-referential OID exists. The engine derives and validates the Planner ref,
commit, path, and delta blob before side effects. `source.ref` must equal
`release-wt/<release>`. Every target ref must equal one distinct live
`track/<release>/<track-id>` derived from that source board and may not equal the
source, Planner, integration, tag, or arbitrary caller ref. `--delta` must equal
the canonical workspace-relative path.

Every C-12 commit is an unsigned canonical Git commit object in the repository's
declared object format. Its header bytes are exactly, in this order: `tree`, each
`parent` in the order stated below, `author`, then `committer`; there is no
`encoding`, `gpgsig`, mergetag, or other header. Author and committer are both
the exact UTF-8 bytes `SwornAgent <noreply@swornagent.dev>`, followed by the
ratification instant as base-10 Unix seconds and the numeric UTC offset preserved
from the ratification literal as `+HHMM` or `-HHMM`; a `Z`-suffixed
`ratified_at` is schema-invalid because it does not carry those bytes. A blank LF separates
headers from the message. The complete message is exactly the stated ASCII
subject followed by one LF: no body, blank trailing paragraph, comment,
signature, or trailer. Replacement paths retain their source tree mode; every
C-12 record path must be a regular `100644` blob and every created delta,
receipt, or protocol marker is `100644`. Trees use canonical Git byte ordering.
Implementations construct and hash these bytes directly; user Git identity,
timezone, signing, hooks, templates and configuration never participate.

Paths, owner seeds, rollback links, and targets are unique and sorted by
unsigned UTF-8 bytes. In addition to whole-object uniqueness, every
`rollback_links[].original_slice_id` and every `rollback_slice_id` is unique;
one original slice can therefore acquire only one immutable rollback slice.
Every mutation path is strictly beneath the named release root, passes C-02
lexical confinement, never enters the release's `plan-deltas/` subtree, and
pins exact before and after Git blob OIDs. Decoded `after_utf8_base64` must be
valid UTF-8 and hash to the declared after blob. The receipt path is
release-confined, absent at the source commit, differs from every mutation path
and from the canonical Planner delta path. These cross-item/path predicates are
validated before object creation or ref mutation. `replan apply` accepts only
`operation: replan` under current authority;
`replan migrate` accepts only `operation: protocol_migration` plus the pristine
eligibility predicate and no existing target protocol marker. A protocol
migration requires `source.protocol_blob_oid: null`, empty owner seeds and
rollback links, one create mutation for the canonical migrated planning marker,
one replace mutation for every selected pristine status, and an empty target
list because no track ref may predate activation. Any started record or
pre-existing owner/target/protocol authority requires a new v0.15 release rather
than in-place migration.

The receipt is deterministic. `receipt_id` equals `delta_id`; `delta_path` and
`delta_blob_oid` are the derived Planner path/blob; `planner_authority` is the
derived Planner ref/commit; `source`, `ratification_digest`, byte-sorted
before/after mutation identities, and byte-sorted target local/remote expected heads are
copied exactly; and `created_at` is `ratification.ratified_at` normalized to UTC
with Go `time.RFC3339Nano`. It is encoded by one fixed ordered Go struct using
`encoding/json` with HTML escaping disabled, no indentation, and one final LF.
Top-level key order is exactly `record_version`, `receipt_id`, `release`,
`delta_path`, `delta_blob_oid`, `planner_authority`, `source`,
`ratification_digest`, `mutations`, `targets`, `created_at`.
`planner_authority` order is `ref`, `commit_oid`; `source` order is `ref`,
`commit_oid`, `board_blob_oid`, `contracts_blob_oid`, `protocol_blob_oid`;
mutation order is `path`, `before_blob_oid`, `after_blob_oid`; target order is
`ref`, `expected_local_head_oid`, `expected_remote_head_oid`. No map participates
in receipt serialization.

`protocol-migration-receipt-v1` is the fixed v1 wire filename and is used for
both delta operation values; for `operation: replan` it is a plan-transaction
receipt with no protocol-marker mutation, while for `protocol_migration` it is
also the marker migration receipt referenced by C-03. The filename never grants
activation authority by itself.

The engine constructs the release-wt transaction from the declared source head
with exactly the mutation after-blobs plus that receipt. The canonical delta
path exists only on the Planner ref and is referenced by the receipt; it is
never a transaction mutation and is never copied into the release-wt
transaction. It uses the same fixed identity and
ratification Git dates and subject `docs(replan): apply <delta_id>`.
It compare-and-swaps the local and remote `source.ref` from the declared source
head to that transaction; no other source head is accepted.

For each byte-sorted target, validate its local and remote pre-state separately.
Normally `expected_local_head_oid` and `expected_remote_head_oid` must be equal.
The sole split-head form is the exact C-08/S15 local-only invalidation: the local
head equals an owner seed's validated one-parent invalidation commit, the remote
head equals that commit's sole parent, the owner seed pins the invalidated status
blob at the local commit, and release-wt remains at the declared source. No other
local/remote split is valid.

Construct one canonical two-parent propagation commit whose parent 1 is the
exact `expected_local_head_oid`, parent 2 is the release-wt transaction commit,
and tree is parent 1 with only the delta's release-record after-blobs and receipt
installed. It uses the same fixed identity/timestamp and subject
`docs(replan): propagate <delta_id> to <target-ref>`. Compare-and-swap the local
target from `expected_local_head_oid` and its remote from
`expected_remote_head_oid` to that same canonical commit; this preserves the
local invalidation in ancestry while publishing only the re-planned terminal
result. A local or remote already at the exact canonical commit is complete; a
ref at its respective expected head is missing and may advance; every other head
is divergent and blocks without force. Restart may complete either missing ref
independently after revalidating the receipt, both declared pre-states, and the
canonical commit.

For `protocol_migration`, fully propagated planning state is not current
authority. `replan activate --delta <canonical-path>` first reruns every schema,
trace, requirements, design-fit, spec-quality, typed-reference, ambiguity,
pristine-state, receipt, source, and target gate. It then derives one source
activation commit whose parent is the transaction commit and whose tree differs
only by changing the migrated marker's `authority` from `planning` to `current`.
It uses the same fixed identity/timestamp and subject
`docs(replan): activate <delta_id>`. Each non-empty target receives an analogous
two-parent activation propagation commit: parent 1 is its exact transaction
propagation commit, parent 2 is the source activation commit, and only the same
marker byte change is applied. Source and targets advance by compare-and-swap;
partial propagation recovers only missing exact commits. Native markers, absent
receipts, ordinary deltas, gate failure, alternate refs, downgrade, force, or
divergence fail without mutation.

The receipt contains no transaction, propagation, or activation commit and no
asserted recovery-state member. `not_started`, `locally_committed`,
`fully_propagated`, and migration-only `fully_activated` are derived by
reconstructing the exact commits above. A valid committed FAIL/gate result
remains non-success; recovery never edits the delta or receipt. The committed
`planning/local-first-migration-manifest.json` is a complete schema-valid C-12
delta with 17 byte-exact mutations, including the downstream contracts registry
consumer repair. In the isolated proof its bytes are placed
at the canonical downstream delta path and committed on the canonical Planner
ref. Its seven status replacements set `validation.human_ratified` to true,
`ratified_by` to `Coach`, and `ratified_at` to the delta timestamp while keeping
every existing scenario and benefit hypothesis byte-identical; this is planning
ratification, never execution evidence. Its ratification decision is the exact
UTF-8 sentence `Brad ratified the existing positive and negative validation
scenarios and benefit hypotheses for all seven local-first slices, and ratified
their pristine-only migration through the v0.15 engine with no synthetic
execution evidence.` whose digest is
`sha256:47d8eb4439e8db4c99997e48d30948bfc1e8a24ca8257aa24305227032a4d01b`.

For `planning/local-first-migration-manifest.json`, the canonical delta path is
`docs/release/2026-07-15-local-first-account-safety/plan-deltas/local-first-account-safety-v015.json`.
Its commit date bytes are `1784124556 +1000`; its receipt `created_at` is
`2026-07-15T14:09:16Z`. Independent construction in a temporary SHA-1 object
store must reproduce these golden identities exactly:

| Object | Golden OID |
|---|---|
| delta blob | `8f362f86d5516148e437c98b64735c102872c145` |
| Planner tree | `2c602ac89fbd7bbd6a4d9739cd761cfab5434aa3` |
| Planner commit | `748cc928173a2c5c71453b425d4526c2f0b4eaab` |
| receipt blob | `89e70a6830f3a029eb9579a675ec4d9ea55c2d82` |
| transaction tree | `d1bdd62932df5e4b33276eb2eec5f3bf9ca070b8` |
| transaction commit | `ceb14cc439d646f2e40632ace446e1567e2dde7d` |
| activated protocol blob | `3a12dd96c44aec2b69f59957aed1df8c22204967` |
| activation tree | `9fc113f062f0833e2c139dbcfb7b0651f07e65cf` |
| activation commit | `1ea27ba2e86f544ed1a1aa661a82eec4f20e1d4b` |

This fixture has no target refs, so it intentionally has no propagation commit
golden. Generic target propagation and activation still use the exact parent
orders, trees, identities, timestamps and one-line subjects specified above.

## Ratification

These clarifications apply exact Baton v0.15.1 behavior and the release intake's
ratified architecture. The remaining Sworn adapter choices above, including the
expanded repository transaction, single embed, whole-root install rollback,
bounded helper ownership, complete eight-command inventory, distinct upstream
and adopting-manifest identities, physically disjoint install/recovery roots,
and pre-replacement durable recovery authority, were selected under the Coach's
2026-07-16 instruction to proceed with the orchestrator's recommendation. They
are Type-1 where copied into a slice's `design_decisions`; fixed umask `0022`
and path-only diagnostics are the recorded Type-2 defaults.
