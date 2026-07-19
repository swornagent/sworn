# Contained executor

`internal/executor` is Sworn's sole subprocess and process-lifetime boundary.
This slice implements its read-only Linux foundation. It is a real containment
backend, but it is not yet the writable builder handoff.

## Admitted invocation

An invocation is rejected before dispatch unless it provides:

- the exact v1 schema, invocation identity, role, and finite timeout;
- one clean absolute workspace path and its deterministic SHA-256 manifest;
- a bounded, explicit argv with a clean absolute `/usr/bin`, `/usr/local/bin`,
  or `/bin` executable;
- only explicitly allowlisted environment names;
- individually named, SHA-256-pinned regular-file inputs; and
- either no network or an executor-enabled host-network exception.

Sworn never parses or constructs a shell command: argv is passed literally. An
adapter may explicitly select a shell or interpreter, but that choice remains
visible in the immutable dispatch instead of being an executor side effect.
Environment defaults are fixed by the executor, reserved loader, Git, locale,
home, path, and temporary-directory variables cannot be supplied by an
invocation, and networking is absent by default.

Before execution, Sworn copies the workspace and every admitted input into a
private runtime directory. The versioned `sworn-workspace-manifest-v1` digest
binds relative paths, entry types, Unix permission bits, symlink targets, and
regular-file bytes. It excludes timestamps and ownership. Git metadata, special
files, changed files during staging, excess bytes, and excess entries fail
closed. The staged manifest must exactly match the invocation digest.

## Linux boundary

The capability probe runs a real transient contained service and caches only a
successful result for that executor instance. The required floor is:

- Linux with a unified cgroup v2 hierarchy and delegated CPU, memory, and PIDs
  controllers;
- a running or degraded systemd user manager, systemd 255 or newer; and
- Bubblewrap 0.9.0 or newer with unprivileged user namespaces.

Each invocation runs as one transient user service. systemd owns the complete
cgroup and applies runtime, memory, swap, task, CPU, file-size, and descriptor
ceilings. Bubblewrap creates fresh user, PID, IPC, UTS, cgroup, and,
by default, network namespaces; drops all capabilities; disables further user
namespaces; and presents a temporary root containing only:

- read-only host `/usr` as the configured runtime trust root;
- the staged workspace at read-only `/workspace`;
- pinned files at read-only `/inputs/<name>`;
- minimal `/proc` and `/dev`; and
- size-bounded temporary `/tmp` and `/home/sworn` filesystems.

Host paths, repository metadata, the control database, credentials, the source
workspace, and engine state are not mounted. Host networking requires both an
invocation request and executor-level admission; it is intentionally a broad
exception, not a domain firewall.

## Lifetime and result

The Sworn engine holds a private pipe open to a tiny hidden shim for the whole
invocation. EOF means the engine died. The shim terminates Bubblewrap and exits,
after which systemd removes every process in the service cgroup. The same cgroup
cleanup runs on explicit cancellation, timeout, and stdout or stderr overflow.
Unit names are deterministic opaque hashes of invocation IDs, so a still-live
duplicate cannot be mistaken for a fresh run.

The executor returns only raw, bounded stdout and stderr, exit status, timing,
cancellation/timeout/truncation flags, and the workspace and input bindings it
actually staged. It does not interpret success, create evidence, manufacture a
submission, or advance engine state.

## Deliberate non-claims

This package is not connected to an engine effect or public command yet. It
does not provide a writable workspace, export edits, run a native agent adapter,
pin the bytes of the host `/usr` runtime, filter an admitted host network, or
make a semantic quality claim from a zero exit status. The next executor slice
must add a size-bounded writable builder layer and measured export without
adding a second subprocess path or weakening this read-only boundary.
