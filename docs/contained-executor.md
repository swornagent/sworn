# Contained executor

`internal/executor` is Sworn's sole subprocess and process-lifetime boundary.
It has two workspace modes over the same Linux containment path:

- `read_only` stages an immutable workspace for inspection or verification; and
- `writable_export` stages a fresh writable copy for a builder and may return a
  quarantined, measured workspace.

`RunContentBound` is the only exported read-only entry point. It requires an
opaque internal runtime capability, copies that tree into the private invocation
root, checks its configured manifest digest, remeasures the staged bytes, and
mounts only that copy at `/usr`. There is no flag or empty-value fallback to
host `/usr`. The separate writable builder entry point uses the host runtime but
cannot claim a content-runtime digest or produce qualifying check evidence.

The internal `check.local` effect worker calls only `RunContentBound`. Its
request binds the exact runtime-manifest and check-definition digests and names
a succeeded builder effect as the sole candidate source. The worker materializes
that candidate from Git; the executor then stages and remeasures the workspace
and exact runtime. The typed result keeps only the outcome and receipt reference.
The journal matches the receipt's candidate identifiers to the builder result,
validates its definition, environment, and output CAS closure, and requires the
environment runtime-manifest digest to match the request. It does not repeat Git
materialization or runtime measurement.

The access mode is part of both the invocation and raw completion. Calling the
wrong entry point fails before dispatch. Neither mode exposes the source
workspace, repository metadata, target refs, the control database, or engine
state to the contained process.

## Admitted invocation

An invocation is rejected before dispatch unless it provides:

- the exact v1 schema, invocation identity, role, workspace access, and finite
  timeout;
- the exact runtime manifest digest when the content-bound entry point is used;
- one clean absolute workspace path and its deterministic SHA-256 manifest;
- a bounded, explicit argv with a clean absolute executable beneath the
  read-only `/usr` runtime trust root (`/bin` is its sandbox alias);
- only explicitly allowlisted environment names;
- individually named, SHA-256-pinned regular-file inputs; and
- either no network or an executor-enabled host-network exception.

Sworn never parses or constructs a shell command: argv is passed literally. An
adapter may explicitly select a shell or interpreter, but that choice remains
visible in the immutable dispatch instead of becoming an executor side effect.
Environment defaults are fixed by the executor; reserved loader, Git, locale,
home, path, and temporary-directory variables cannot be supplied by an
invocation. Networking is absent by default.

Before execution, Sworn copies the workspace and every admitted input. A
content-bound invocation also copies and remeasures its runtime under a
capability ceiling narrowed by the executor's shared input-byte ceiling. The
versioned `sworn-workspace-manifest-v1` digest binds relative paths, entry types,
ordinary rwx permission bits, symlink targets, and regular-file bytes. It excludes
timestamps, ownership, and inode alias topology. Git metadata, special files,
changed files during staging, excess logical bytes, and excess entries fail
closed. The staged manifest must exactly match the invocation digest.

Writable execution never binds the source workspace and never copies changes
back over it. The initial tree is copied into a private host-visible directory
on a finite tmpfs, then only that copy is mounted read-write. Source hardlinks
are broken by the copy. Builder-created hardlinks inside the isolated tree are
hashed conservatively once per path and are later normalized by Git candidate
capture; the sandbox cannot link them to a separate read-only mount or an
unexposed host sibling.

## Linux boundary

The capability probe runs a real transient contained service and caches only a
successful result for that executor instance. The required floor is:

- Linux with a unified cgroup v2 hierarchy and delegated CPU, memory, and PIDs
  controllers;
- a running or degraded systemd user manager, systemd 255 or newer; and
- Bubblewrap 0.9.0 or newer with unprivileged user namespaces.

Each invocation runs as one transient user service. systemd owns the complete
cgroup and applies runtime, memory, swap, task, CPU, file-size, and descriptor
ceilings. Bubblewrap creates fresh user, PID, IPC, UTS, cgroup, and, by default,
network namespaces; drops all capabilities; disables further user namespaces;
and presents a temporary root containing only:

- the exact staged content runtime for read-only checks, or host `/usr` for the
  distinct writable builder path, read-only at `/usr`;
- the staged workspace at `/workspace`, read-only or read-write exactly as the
  invocation declares;
- pinned files at read-only `/inputs/<name>`;
- minimal `/proc` and `/dev`; and
- size-bounded temporary `/tmp` and `/home/sworn` filesystems.

Host networking requires both an invocation request and executor-level
admission. It is intentionally a broad exception, not a domain firewall.

## Writable resource claim

The writable root must be a clean, private, current-user-owned directory on a
finite tmpfs that permits execution. Writable dispatch is serialized inside one
executor instance so staging and free-capacity admission cannot race each other
in the initial serial kernel.

The live and retained bounds are deliberately separate:

- `InputBytes` bounds the logical bytes copied before the service starts.
- cgroup `MemoryMax` and `MemorySwapMax` collectively bound new service-charged
  anonymous memory, tmpfs pages, and accounted kernel memory while the process
  runs. `/workspace`, `/tmp`, `/home/sworn`, and the process itself compete
  inside that service ceiling. Linux documents tmpfs/shared-memory and kernel
  accounting in [cgroup v2](https://docs.kernel.org/admin-guide/cgroup-v2.html).
- `FileBytes` is a per-file `RLIMIT_FSIZE` ceiling.
- after the complete service is quiescent, `WorkspaceBytes` and the 100,000
  entry ceiling bound the logical tree Sworn is willing to retain and expose.
- the finite host tmpfs remains the global physical backstop.

This is not a dedicated per-workspace tmpfs quota. Initial copied pages are
charged to the engine rather than the service, sparse logical files need little
physical memory, and the workspace competes with process memory while live.
The exact claim is a hard live resource ceiling plus a separately measured
logical export ceiling. The initial v1 kernel also assumes one Sworn executor
process owns the configured root; cross-process capacity reservations are not
implemented.

## Lifetime, start proof, and quiescence

The Sworn engine holds a private pipe open to a tiny hidden shim for the whole
invocation. EOF means the engine died. The shim terminates Bubblewrap, after
which systemd removes every process in the service cgroup. The same cgroup
cleanup runs on explicit cancellation, timeout, and stdout or stderr overflow.
Unit names are deterministic opaque hashes of the executor's private runtime
root and invocation ID. A still-live duplicate within one engine cannot be
mistaken for a fresh run, while independent executor roots sharing one user
systemd manager do not collide.

Bubblewrap reports its child start over a private JSON status descriptor. The
shim writes a private host marker only after that event, and the executor checks
the marker before accepting a completion. Missing systemd units, a failed shim,
Bubblewrap setup errors, and target exec failures therefore cannot masquerade as
ordinary target exit codes or produce an export.

For writable runs, measurement begins only after `systemd-run --wait` returns
and the deterministic unit is confirmed inactive or absent. Tests cover a
session-detached child that attempts a delayed write: all service writers are
gone and the measured tree remains unchanged.

## Raw completion and measured export

The executor returns raw, bounded stdout and stderr, exit status, timing,
cancellation/timeout/truncation flags, declared workspace access, and the input
bindings it actually staged. Content-bound completion also returns the exact
runtime manifest digest observed while staging; the producer requires it to
match the invocation before storing an environment or receipt. It does not
interpret semantic success, create
evidence, manufacture a submission, or advance engine state.

A writable run yields no export after cancellation, timeout, output overflow,
control-start failure, an unsafe tree, or an excessive tree. An ordinary
non-zero target exit may still yield a measured workspace for diagnosis or a
later explicit engine decision. Its presence never means candidate-ready or
successful.

`WorkspaceExport` is a versioned, quarantined handle binding the invocation,
fresh random generation, source digest, exact host path, final manifest digest,
and logical bytes. `ValidateExport` confirms the unit is quiescent and
remeasures the tree immediately before handoff. `DiscardExport` checks only
executor ownership and quiescence, then removes the tree without requiring its
contents to match; a rejected or externally changed export therefore remains
cleanable. Generation-specific paths make a stale handle harmless if an
invocation ID is ever reused.

Failure-path cleanup uses a fresh bounded context and removes a writable tree
only after service quiescence is proven. If systemd state cannot be established,
Sworn leaves the generation-bound residue for reconciliation instead of racing
a possible writer.

The exact-candidate boundary independently scans and stages Git-visible bytes.
The tested handoff clones the original repository `Workspace` binding, replaces
only its path with the validated export path, and calls `repo.Capture`. The
executor digest is structural evidence, never Git candidate identity or a
quality verdict.

## Trust boundary and deliberate non-claims

The private roots exclude other host users. A malicious process already running
as the same host UID is inside the engine's trust boundary; it could race or
alter any same-UID filesystem object. Sworn does not claim to defend itself from
its own host account or administrator.

This package is connected to an internal `check.local` worker, and
`checks.dispatch` derives the complete ordered batch from the exact plan. The
worker and reducer edge remain unreachable from the public command surface and
autonomous engine flow. A content-bound runtime proves which bytes executed; it
does not retain those bytes or claim hermetic reproduction. The kernel, CPU,
and containment implementation remain host facts. Effect completion validates
the runtime request and artifact closure; the later atomic admission transaction
also closes the embedded protocol-snapshot binding before exposing reviewable.
The executor also does not run a native agent adapter, filter an admitted host
network, or infer quality from an exit status. If the engine dies, the shim and
cgroup still stop all writers, but reclaiming a generation-bound writable
workspace is part of the later interrupted-effect reconciliation slice.
