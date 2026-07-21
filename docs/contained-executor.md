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

The `check.local` effect worker calls only `RunContentBound`. Its
request binds the exact runtime-manifest and check-definition digests and names
a succeeded builder effect as the sole candidate source. The worker materializes
that candidate from Git; the executor then stages and remeasures the workspace
and exact runtime. The typed result keeps only the outcome and receipt reference.
The journal matches the receipt's candidate identifiers to the builder result,
validates its definition, environment, and output CAS closure, and requires the
environment runtime-manifest digest to match the request. It does not repeat Git
materialization or runtime measurement.

Each claimed check receives a deterministic attempt-bound executor invocation
identity distinct from its stable Baton receipt run ID. After interruption,
`ReconcileContentBound` proves that exact systemd unit inactive, removes its
runtime residue, and returns an opaque cleanup proof. The check worker also
removes the matching private candidate materialization before Store may
authorize an unbound retry.

The access mode is part of both the invocation and raw completion. Calling the
wrong entry point fails before dispatch. Neither mode exposes the source
workspace, repository metadata, target refs, the control database, or engine
state to the contained process.

## Admitted invocation

An invocation is rejected before dispatch unless it provides:

- the exact v2 schema, invocation identity, role, workspace access, and finite
  timeout;
- the exact runtime manifest digest when the content-bound entry point is used;
- one clean absolute workspace path and its deterministic SHA-256 manifest;
- a bounded, explicit argv whose executable is either beneath the read-only
  `/usr` runtime trust root (`/bin` is its sandbox alias) or exactly one named,
  digest-pinned executable input;
- only explicitly allowlisted environment names;
- individually named, SHA-256-pinned regular-file inputs;
- either no network or an executor-enabled host-network exception; and
- no nested user namespace unless both the invocation and executor admit it;
  and
- no credential-file access unless the invocation uses the writable entry point
  and the executor admits one exact configured source file.

Sworn never parses or constructs a shell command: argv is passed literally. An
adapter may explicitly select a shell or interpreter, but that choice remains
visible in the immutable dispatch instead of becoming an executor side effect.
Environment defaults are fixed by the executor; reserved loader, Git, locale,
home, path, and temporary-directory variables cannot be supplied by an
invocation. `CODEX_HOME` is also reserved. Networking and credential access are
absent by default.

The sole production executable-input profile is the pinned Codex builder. Its
exact static-PIE version, SHA-256 digest, byte length, argv, model, tool schema,
ChatGPT authentication mode, fixed Codex home, named permission profile,
timeout, network request, nested-sandbox request, credential-access request,
and executor configuration are bound into the builder profile. Sworn neither
finds it through `PATH` nor accepts a different Codex release under the same
profile.

Before execution, Sworn copies the workspace and every admitted input. Inputs
are staged read-only and non-executable. An invocation may select exactly one
input as its direct entrypoint; only that staged copy is executable, argv must
name `/inputs/<name>` exactly, and the raw completion records the selected name
alongside the observed input digest and size. It is never placed in the
writable or exported workspace. A content-bound invocation also copies and
remeasures its runtime under a capability ceiling narrowed by the executor's
shared input-byte ceiling. The
versioned `sworn-workspace-manifest-v1` digest binds relative paths, entry types,
ordinary rwx permission bits, symlink targets, and regular-file bytes. It excludes
timestamps, ownership, and inode alias topology. Git metadata, special files,
changed files during staging, excess logical bytes, and excess entries fail
closed. The staged manifest must exactly match the invocation digest.

The Codex authentication file is not an ordinary input. It is neither copied
into `/inputs`, hashed into a content digest, made executable, nor reported as a
`BoundInput`. It enters only through the separate credential capability
described below. The raw invocation and completion bind whether that capability
was used, while the executor configuration binds its fixed target, size ceiling,
configured source path, and admission switch.

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
network namespaces; drops all capabilities; and presents a temporary root
containing only:

- the exact staged content runtime for read-only checks, or host `/usr` for the
  distinct writable builder path, read-only at `/usr`;
- the staged workspace at `/workspace`, read-only or read-write exactly as the
  invocation declares;
- pinned files at read-only `/inputs/<name>`;
- for an explicitly credential-enabled writable invocation only, one retained
  file bound read-write at `/home/sworn/.codex/auth.json`;
- minimal `/proc` and `/dev`; and
- size-bounded temporary `/tmp` and `/home/sworn` filesystems.

Host networking requires both an invocation request and executor-level
admission. It is intentionally a broad exception, not a domain firewall.
Further user namespaces are disabled by default. A nested-sandbox invocation
requires a separate executor-level admission before that restriction is
omitted; descendants still inherit the outer mount, network, capability, and
cgroup boundary. This does not constrain deeper descendant namespace creation
or remove the kernel's user-namespace attack surface.

## Codex credential-file capability

The production Codex builder is the sole caller of the initial credential-file
capability. Executor construction must bind one clean absolute source path and
enable credential admission. The invocation must separately request
`credential_access`, must use the writable entry point, and, through the builder
profile, must also request the nested sandbox. Content-bound checks never
receive the mount.

On Linux, Sworn requires the source to be a non-empty regular file owned by the
executor user, with exact mode `0600`, exactly one hard link, no symbolic-link
remap, and a maximum size of 64 KiB. It opens the file read-write with
`O_NOFOLLOW`, acquires an exclusive nonblocking file lock, compares the retained
descriptor with the live path, and keeps both descriptor and lock for the
complete invocation. The contained service mounts that exact retained file
rather than reopening the configured pathname. Sworn revalidates identity and
file shape before releasing the lock. It explicitly unlocks only after systemd
service quiescence is proven. If quiescence is unproven, the engine closes its
descriptor without unlocking; a live shim's inherited open-file description
keeps the flock held until that process exits. A busy, replaced, relinked,
resized beyond the ceiling, or permission-drifted file fails closed.

The bind target and outer `CODEX_HOME` are fixed at
`/home/sworn/.codex/auth.json` and `/home/sworn/.codex`. Only `auth.json` is
mounted from the dedicated host Codex home; other user configuration, sessions,
logs, rules, and state do not enter the container. The mount is deliberately
read-write because the trusted pinned CLI owns ChatGPT token refresh. Its
contents are never supplied in an environment variable or stored in executor
completion metadata.

The outer Codex control process can read and refresh that file and has the
separately admitted host-network exception. Every model-directed tool runs
under the adapter's named Codex permission profile, which extends
`:workspace`, disables nested network access, and denies read access to the
entire `/home/sworn/.codex` tree. Its shell environment inherits nothing and
contains only fixed non-secret values. This nested mask, rather than the host
file's `0600` mode, is what keeps the authentication material unavailable to a
tool running under the same outer UID. There is no Platform API-key environment
or fallback path. See [ADR
0009](adr/0009-codex-cli-managed-chatgpt-authentication.md).

## Writable resource claim

The writable root must be a clean, private, current-user-owned directory on a
finite tmpfs that permits execution. Writable dispatch and reconciliation are
serialized by both an in-process mutex and a process-shared lock on that root.
Staging, cleanup, and free-capacity admission therefore cannot race across
executor instances in the initial serial kernel. The kernel releases the lock
if an owner process dies.

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
logical export ceiling. The shared writable-root lock deliberately serializes
cross-process admission; it is not a parallel capacity-reservation system.

## Lifetime, start proof, and quiescence

The Sworn engine holds a private pipe open to a tiny hidden shim for the whole
invocation. EOF means the engine died. The shim terminates Bubblewrap, after
which systemd removes every process in the service cgroup. The same cgroup
cleanup runs on explicit cancellation, timeout, and stdout or stderr overflow.
Unit names are deterministic opaque hashes of the executor's private runtime
root and invocation ID. Writable runtime and workspace paths are deterministic
from the same one-shot invocation identity. Content-bound runtime paths use the
check attempt's own deterministic invocation identity. Residue is therefore
discoverable after restart and a duplicate cannot be mistaken for a fresh run,
while independent executor roots sharing one user systemd manager do not
collide.

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
cancellation/timeout/truncation flags, declared workspace and credential access,
and the input bindings it actually staged. Content-bound completion also
returns the exact runtime manifest digest observed while staging; the producer
requires it to match the invocation before storing an environment or receipt.
It does not interpret semantic success, create evidence, manufacture a
submission, or advance engine state.

A writable run yields no export after cancellation, timeout, output overflow,
control-start failure, an unsafe tree, or an excessive tree. An ordinary
non-zero target exit may still yield a measured workspace for diagnosis or a
later explicit engine decision. Its presence never means candidate-ready or
successful.

`WorkspaceExport` is a versioned, quarantined handle binding the invocation,
deterministic one-shot generation, source digest, exact host path, final
manifest digest, and logical bytes. `ValidateExport` confirms the unit is
quiescent and remeasures the tree immediately before handoff. `DiscardExport`
checks only executor ownership and quiescence, then removes the tree without
requiring its contents to match; a rejected or externally changed export
therefore remains cleanable. Invocation IDs are never reused across attempts.

Failure-path cleanup uses a fresh bounded context and removes a writable tree
only after service quiescence is proven. If systemd state cannot be established,
Sworn leaves the attempt-bound residue instead of racing a possible writer.
`ReconcileWritable` later acquires the process-shared ownership lock, proves the
exact deterministic unit inactive, removes both runtime and workspace paths,
rechecks quiescence and absence, and only then returns an opaque cleanup proof.
`ReconcileContentBound` applies the same no-racing-writer rule to the exact
read-only check unit and runtime residue. Neither proof changes journal state by
itself; a Store-issued recovery capability must seal it to the matching attempt.

The exact-candidate boundary independently scans and stages Git-visible bytes.
The native builder clones the original repository `Workspace` binding,
replaces only its path with the validated export path, and calls
`repo.PrepareCandidate`. That operation writes candidate objects but publishes
no ref; Store publishes only after the typed result is durable. The executor
digest is structural evidence, never Git candidate identity or a quality
verdict.

## Trust boundary and deliberate non-claims

The private roots exclude other host users. A malicious process already running
as the same host UID is inside the engine's trust boundary; it could race or
alter any same-UID filesystem object. Sworn does not claim to defend itself from
its own host account or administrator.

This package is connected to the production Codex builder and `check.local`
workers. `sworn run` reaches both through the sole controller; it does not expose
either worker as a standalone command. A content-bound runtime proves which
bytes executed; it does not retain those bytes or claim hermetic reproduction.
The kernel, CPU, systemd user manager, Bubblewrap, host `/usr`, and containment
implementation remain trusted host facts.

The outer Codex control process receives read-write access to the single
CLI-managed ChatGPT authentication file and a broad host-network exception. Its
model-directed tool process runs in Codex's nested sandbox with neither access
to the Codex home nor network. This is not an egress firewall: the trusted outer
process may reach arbitrary host-network destinations, and same-UID host
observers remain inside the trust boundary. The executor does not infer quality
from exit status. Effect completion validates runtime and artifact closure;
atomic admission closes the embedded protocol snapshot before exposing
reviewable. See [ADR
0008](adr/0008-builder-to-reviewable-production-vertical.md) and [ADR
0009](adr/0009-codex-cli-managed-chatgpt-authentication.md).
