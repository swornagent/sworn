# Running the bounded builder-to-reviewable vertical

`sworn run` is Sworn v0.2.0's sole mutating command:

```text
sworn run <run> [<work>] --config <clean-absolute-path> [--json]
```

It acquires exclusive ownership of one existing control Store, completes the
Store-wide recovery barrier, and converges exactly one current work item through
the pinned Codex builder, its complete ordered local-check batch, and atomic
admission to `reviewable`. It exits after that bounded operation. It does not
create or activate a delivery, poll for work, advance another item, obtain a
verifier verdict, accept `PASS`, or update the target ref.

The run and work arguments are Sworn IDs. An active delivery has exactly one
non-waiting work item; omitting `<work>` selects it. An explicit waiting or
foreign work ID fails. `--config` and `--json` may each appear only once.

## Deployment prerequisites

This is a pre-alpha production vertical, not a bootstrap flow. Before invoking
it, deployment tooling must have:

- created a private control database containing the exact planned and activated
  delivery, authenticated historical approval, and current work item;
- measured and persisted the repository binding and the content-runtime
  manifest digest;
- published the plan-selected signed authority bundle into a configured trusted
  directory;
- provisioned private executor, writable, builder, and check roots, with the
  writable root on a finite executable tmpfs;
- installed exact Bubblewrap, `systemd-run`, and `systemctl` executables; and
- supplied the exact accepted Codex static binary; and
- created a dedicated private Codex home and authenticated it through the Codex
  CLI's file-backed ChatGPT login.

There is no public `init`, plan activation, repository-discovery, runtime-digest,
config-generation, or authorizer command yet. These are deliberate adoption
gaps, not operations which `sworn run` performs implicitly.

The execution host must satisfy the Linux capability floor in [Contained
executor](contained-executor.md): delegated cgroup v2 controllers, a systemd 255
or newer user manager, Bubblewrap 0.9.0 or newer, unprivileged user namespaces,
and finite resource backing. The host account, same-UID processes, kernel,
systemd, Bubblewrap, host `/usr`, and outer Codex process remain in the trusted
computing base.

## Strict run configuration

The configuration is a complete non-secret deployment binding. It must be a
non-empty regular file no larger than 256 KiB, mode `0600` or otherwise without
group or world permissions, reached by a clean absolute path with no symlink
remap. JSON is strict: duplicate or unknown members, trailing values, invalid
I-JSON, and an unknown schema version fail closed.

The `v1` suffixes in this configuration and the command's JSON result identify
their schema generations. They are intentionally independent of the v0.2.0
package version and do not imply a Sworn 1.0 release.

This example shows the complete shape. Every path, digest, public key, identity,
and limit must be replaced with the deployment's measured value:

```json
{
  "schema_version": "sworn-run-config-v1",
  "control_database": "/srv/sworn/control.db",
  "repository": {
    "root": "/srv/project",
    "binding": {
      "schema_version": "sworn-repository-binding-v1",
      "repository_id": "project",
      "common_dir": "/srv/project/.git",
      "object_dir": "/srv/project/.git/objects",
      "object_format": "sha1"
    }
  },
  "authority": {
    "sources": [
      {
        "source_ref": "release-authority",
        "authorizer_ref": "release-captain",
        "public_key": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
        "bundle_directory": "/srv/sworn/authority"
      }
    ]
  },
  "executor": {
    "runtime_root": "/srv/sworn/executor",
    "writable_root": "/run/user/1000/sworn-writable",
    "bubblewrap_path": "/usr/bin/bwrap",
    "systemd_run_path": "/usr/bin/systemd-run",
    "systemctl_path": "/usr/bin/systemctl"
  },
  "content_runtime": {
    "source": "/srv/sworn/check-runtime",
    "digest": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    "maximum_bytes": 1073741824
  },
  "workspaces": {
    "builder_root": "/srv/sworn/builder",
    "check_root": "/srv/sworn/checks"
  },
  "codex": {
    "binary": "/srv/sworn/bin/codex",
    "chatgpt_auth_file": "/srv/sworn/codex-home/auth.json",
    "model": "gpt-5.4",
    "timeout_seconds": 300
  }
}
```

The repository binding includes the complete Git common directory, object
directory, and object format; Sworn remeasures them on every open. A path or
remote URL alone is not repository identity. Authority sources contain only an
authorizer identity, canonical base64 Ed25519 public key, and trusted bundle
directory. Private signing material is never accepted.

All configured paths are clean and absolute. The control database and private
roots must already exist. Repository and content-runtime sources may not be
symlink remaps. The builder and check roots must be distinct. Executor program
paths must name exact non-symlink executable files. See [Exact plan and
authenticated authority](authenticated-authority.md) for bundle publication and
[Exact local candidate](exact-candidate.md) for repository binding semantics.

`executor.limits` may be omitted to select Sworn's versioned finite defaults. If
present, it must be one complete object with all of these fields:

```json
{
  "runtime_seconds": 300,
  "memory_bytes": 2147483648,
  "swap_bytes": 0,
  "tasks": 256,
  "cpu_percent": 100,
  "file_bytes": 67108864,
  "temp_bytes": 536870912,
  "home_bytes": 134217728,
  "input_bytes": 1073741824,
  "workspace_bytes": 1073741824,
  "stdout_bytes": 4194304,
  "stderr_bytes": 4194304
}
```

The Codex model and timeout are explicit; there is no provider or model default
in the config. `codex.chatgpt_auth_file` names only the `auth.json` created in a
dedicated Codex CLI home. Sworn does not accept the credential bytes in the
configuration or environment, and it does not hash or copy them into the Store,
profile digest, command output, or candidate workspace. The configured path is
part of the executor configuration binding; the secret file contents are not.

Optional top-level `owner_id` may be a valid Sworn ID. If omitted, Sworn derives
a deterministic identity bound to the Store path, repository identity, and run.
It identifies process ownership and audit attribution; it is not authority.

The production adapter currently accepts only the 304,169,008-byte static-PIE
`codex-cli 0.145.0-alpha.18` with SHA-256
`16db86b6bf81cc426032fd42216dd97e60f97b149272f1f9963845a0675dae94`.
Sworn does not yet download or install that alpha binary; a current or otherwise
different Codex build is rejected.

## Codex ChatGPT authentication

Create a Codex home used only by Sworn, then run the accepted Codex binary's
normal ChatGPT login with file storage forced explicitly:

```sh
install -d -m 0700 /srv/sworn/codex-home
CODEX_HOME=/srv/sworn/codex-home \
  /srv/sworn/bin/codex \
  -c 'forced_login_method="chatgpt"' \
  -c 'cli_auth_credentials_store="file"' \
  login
chmod 0600 /srv/sworn/codex-home/auth.json
```

The directory and `auth.json` must be owned by the account which runs Sworn and
must remain private. The file must be a non-empty, non-symlink regular file with
exact mode `0600`, exactly one hard link, and no more than 64 KiB. Point
`codex.chatgpt_auth_file` at that exact file. Do not point Sworn at the Codex
home used for interactive work, copy a transient access token into the file, or
place the file under a repository or its Git metadata, an authority bundle,
executor runtime, writable root, content runtime, builder root, or check root.
It must also not be the control database or Codex binary.

The Codex CLI owns the authentication format and refresh lifecycle. Sworn opens
and locks the configured file, retains its exact identity, and binds only that
single file read-write at `/home/sworn/.codex/auth.json` for the trusted outer
Codex process. Read-write access is required because the CLI may refresh and
rotate its ChatGPT tokens during a run. The rest of the configured Codex home is
not mounted. The outer process receives broad host networking; its named nested
permission profile disables network and denies the whole
`/home/sworn/.codex` tree to model-directed tools. `CODEX_HOME` is fixed by the
executor and cannot be supplied by an invocation.

Sworn does not read `OPENAI_API_KEY`, `CODEX_API_KEY`, or another Platform API
key, and the accepted profile has no API-key fallback. A missing, unsafe, busy,
replaced, or Platform API-key-mode credential fails the run instead of
selecting another authentication method. Sworn does not parse the CLI-owned
file to reclassify other modes which the pinned CLI considers part of its
ChatGPT login family; the dedicated-home provisioning procedure is therefore
part of the operational boundary.

Stop every Sworn process which uses this file before logging out, changing
accounts, or reauthenticating. Then rerun the same pinned-CLI login command
against the same dedicated `CODEX_HOME`, restore exact mode `0600`, and confirm
the result before restarting Sworn:

```sh
CODEX_HOME=/srv/sworn/codex-home \
  /srv/sworn/bin/codex \
  -c 'forced_login_method="chatgpt"' \
  -c 'cli_auth_credentials_store="file"' \
  login status
```

See [Codex authentication](https://developers.openai.com/codex/auth) for the
upstream login flow and [ADR
0009](adr/0009-codex-cli-managed-chatgpt-authentication.md) for Sworn's narrower
credential boundary.

## Convergence and output

On startup the controller marks interrupted effects unknown, converges exact
bound results, and requeues only unbound builder or check attempts with complete
machine-proved cleanup. It activates only when no running or unknown effect
remains. For the selected work attempt, stable domain-separated IDs identify
build dispatch, check dispatch, and admission. Re-running the same command after
an interruption therefore observes or completes the same durable work instead
of creating a second workflow.

Fresh current authority is resolved before builder scheduling, before pending
builder execution, and before each pending local-check claim. Check dispatch and
submission admission are deterministic historical transactions over exact
Store truth and do not add redundant permits.

Text output reports only the committed terminal projection:

```text
run run-1 work work-1: reviewable (revision 4)
```

`--json` emits one `sworn-run-result-v1` object containing run, work, final
state, revision, builder and check effect IDs, applied or replayed command IDs,
and startup recovery counts. Command entries are absent when the work was
already reviewable. Errors produce no success object. After interruption or
failure, `sworn board <run> --store <database> --json` remains the read-only view
of committed truth.

## Deliberate limits

`reviewable` means the exact local candidate and required local evidence were
atomically admitted. It is not an independent verdict or `PASS`. v0.2.0 has
no verifier adapter, verdict routing, bounded repair policy, scheduler,
integration edge, or external authorizer transport.

The outer Codex process has broad host-network access and read-write access to
the single managed ChatGPT authentication file; its nested tool process has
neither network nor read access to the Codex home. Production `sworn run` uses
the built-in OpenAI provider through that ChatGPT login and consumes the
account's Codex usage. It never uses a Platform API key. The automated
real-Codex boundary proof mounts synthetic file-backed ChatGPT state while using
a scripted local Responses endpoint, so it consumes no provider tokens. No
live delivery runs in the ordinary suite. On 2026-07-21, the separate opt-in
release smoke test passed at the built-process boundary using `gpt-5.4` and the
exact pinned CLI: one live turn created the exact candidate, passed its local
check, reached `reviewable` at revision 4, and left the target ref unchanged; a
second built-process invocation converged with no commands, effects, or model
turn. The scripted provider authenticates with a separate test-only bearer, so
that proof does not claim that its model request used the mounted ChatGPT state.

See the [v0.2.0 release notes](releases/v0.2.0.md) for the packaged boundary and
the [roadmap](roadmap.md) for the v0.3.0 verifier direction.
