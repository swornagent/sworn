# ADR 0009: Use Codex CLI-managed ChatGPT authentication

- Date: 2026-07-21
- Status: accepted
- Supersedes in part: ADR 0007 and ADR 0008 credential transport only

## Context

ADRs 0007 and 0008 established the important process split: one exact trusted
Codex control process may reach the model while every model-directed tool is
confined to the measured workspace without network or credential access. Their
first proof transported a Platform API key in an environment variable. That was
a useful canary for the boundary, but it is not the production authentication
contract.

Sworn should use the operator's Codex-enabled ChatGPT account through the Codex
CLI's own login and refresh lifecycle. It must not ask for, read, rename, or
silently fall back to a Platform API key. Authentication also must not become a
generic secret framework or make the whole interactive Codex home visible to a
builder.

The CLI's file store places authentication in `CODEX_HOME/auth.json`. Unlike an
ordinary Sworn input, that file is mutable: the trusted CLI may refresh and
rotate tokens while a run is active. Mounting it read-only would eventually
break refresh. Copying it into a disposable per-attempt input and discarding the
updated copy could retain a consumed refresh token. Mounting the complete Codex
home would admit unrelated user configuration, rules, sessions, logs, and
state. A host keyring is not available through Sworn's private systemd and
Bubblewrap boundary, while a standalone access token would move refresh
ownership back out of the CLI.

## Decision

### Provision one dedicated Codex home

Deployment creates a private Codex home used only by Sworn. While Sworn is
stopped, the operator runs the exact accepted Codex binary with that
`CODEX_HOME`, `forced_login_method="chatgpt"`, and
`cli_auth_credentials_store="file"`, then completes the normal ChatGPT login.
The resulting `auth.json` is the only credential source accepted by the bounded
vertical.

The strict `sworn-run-config-v1` object names that file through
`codex.chatgpt_auth_file`. The path is non-secret configuration; the file
contents remain secret state owned by Codex. The old
`codex.credential_environment` field is removed before the first public v1 run
configuration ships. There is no compatibility reader, environment fallback,
or dual authentication mode.

The dedicated directory is current-user-owned and mode `0700`. Its `auth.json`
must be current-user-owned, non-empty, no larger than 64 KiB, a non-symlink
regular file with exact mode `0600` and exactly one hard link. It must be
outside the repository and Git metadata trees, authority bundle directories,
content runtime, executor runtime and writable roots, and builder and check
roots. It must also be distinct from the control database and Codex binary. The
deployment must not point Sworn at the Codex home used for interactive work.

### Add one narrow mutable credential capability

Credential access is a separate fact in the v2 executor invocation, raw
completion, builder dispatch, and profile. Executor configuration must admit
one exact source path, and the invocation must separately request access through
the writable entry point. The production Codex builder is the sole caller;
content-bound checks receive no credential mount.

On Linux, the executor opens the configured file read-write with `O_NOFOLLOW`,
acquires a nonblocking exclusive file lock, verifies its retained identity and
shape, and keeps that descriptor and lock for the whole invocation. The
contained service mounts that exact retained file read-write at the fixed target
`/home/sworn/.codex/auth.json` without reopening the configured pathname, and
sets the trusted outer process's `CODEX_HOME` to `/home/sworn/.codex`. The source
pathname is revalidated before the lock is released. Explicit unlock requires
proven service quiescence. On quiescence uncertainty, the engine closes without
unlocking so a live shim's inherited open-file description retains the flock
until that process exits. Failure to acquire, retain, mount, or revalidate the
exact file fails the operation.

Only that file is bound from the host. The rest of `/home/sworn` and the outer
Codex home remain invocation-local temporary filesystems. The credential is not
an ordinary input: Sworn does not copy or hash its bytes, place it under
`/inputs`, include it in a profile or content digest, return it as a bound input,
or persist it in SQLite, output, or the candidate workspace. The executor
configuration digest binds the configured path, fixed target, byte ceiling, and
admission switch instead.

The lock coordinates Sworn processes which share this dedicated file. The Codex
CLI does not participate in that Sworn lock, so operations must stop Sworn
before logout, account changes, or reauthentication. The same pinned CLI and
dedicated `CODEX_HOME` are then used to refresh the login before Sworn restarts.

### Keep model-directed tools blind

The outer Codex process remains exact trusted control-plane code. It receives
the mutable authentication file and the separately admitted broad host-network
exception. Its fixed argv selects the built-in OpenAI provider and:

- forces ChatGPT login and file-backed credential storage;
- selects the named `sworn_builder` permission profile;
- extends the built-in `:workspace` filesystem permissions;
- denies the entire `/home/sworn/.codex` tree to nested processes;
- disables nested network access; and
- gives nested shells no inherited environment and only fixed non-secret
  values.

The nested filesystem deny rule, not Unix mode alone, separates the credential
from a model-directed tool running under the same outer UID. `CODEX_HOME` is a
reserved executor environment name and cannot be supplied by an invocation.
`OPENAI_API_KEY`, `CODEX_API_KEY`, and other Platform API-key values are neither
read nor allowlisted. A missing login or a Platform API-key-mode login fails
closed.

Sworn does not parse the CLI-owned file to duplicate Codex's authentication
schema. The pinned CLI classifies some externally supplied ChatGPT-family modes
alongside its normal browser login; the private dedicated-home provisioning
procedure is what selects the latter. The machine-enforced boundary is that
Platform API-key modes and fallback are unavailable, while the exact trusted
CLI owns classification inside its forced ChatGPT family.

The exact authentication mode, fixed Codex home, permission-profile name, argv,
credential-access fact, network and nested-sandbox requests, binary facts, model,
tool schema, timeout, output schema, and executor configuration are bound by
`sworn-codex-builder-profile-v2`. The executor configuration, invocation, and
containment policy also advance to v2 so an older dispatch cannot be mistaken
for this credential boundary.

## Required release evidence

The v0.2.0 release candidate must prove all of the following before merge:

1. strict run configuration accepts only `codex.chatgpt_auth_file` and rejects
   the removed credential-environment field, aliases, overlap, unsafe shape,
   replacement, contention, and permission or identity drift;
2. only a credential-enabled writable invocation receives the fixed read-write
   file mount, and completion binds that fact without exposing file bytes;
3. the real pinned Codex binary mounts a synthetic ChatGPT-shaped `auth.json`
   while executing against a local scripted Responses endpoint, without a
   provider model call or Platform API key, and the evidence states explicitly
   that the scripted provider uses a separate test-only bearer;
4. its real nested tool can edit the measured workspace but cannot read or
   enumerate `/home/sworn/.codex`, recover the token through its environment or
   visible `/proc`, or reach the outer endpoint; and
5. success, failure, timeout, cancellation, output overflow, and recovery leave
   the credential lock and executor-owned runtime in a truthful reusable state.
6. an opt-in built-process smoke test uses the built-in provider through the
   dedicated ChatGPT login to create the exact candidate, pass its real local
   check, reach `reviewable`, preserve the target ref, and then converge on a
   second process invocation without another model turn.

The scripted proof is token-free and proves the mount plus nested denial; it
does not prove that its test-provider request authenticated from that file. A
live `sworn run` is a separate operational smoke test and consumes the dedicated
ChatGPT account's Codex usage; it is not part of the ordinary automated suite.
That smoke test passed on 2026-07-21 with `gpt-5.4` and the exact pinned CLI.

## Consequences and deliberate limits

Sworn delegates login format, token refresh, and ChatGPT account semantics to
the pinned Codex CLI while retaining a small, explicit secret boundary. The
engine does not become an OAuth client, keyring bridge, provider abstraction, or
general credential store. Token refresh persists across runs without admitting
the complete Codex home or exposing secret bytes to model-directed tools.

The trusted outer Codex process can read, replace the contents of, or truncate
the bound file and can reach arbitrary host-network destinations. The host
account and same-UID processes remain trusted, as in ADR 0007. A dedicated home
prevents ordinary interactive Codex use from racing the Sworn file, but it does
not defend against a malicious same-UID process or administrator.

Initial login and later reauthentication remain explicit deployment operations;
Sworn has no login command or browser flow. File-backed ChatGPT authentication
through the dedicated-home procedure is the only supported production setup.
Keyring storage, Platform API keys, direct token provisioning by Sworn, provider
selection, and authentication fallback are out of scope. A materially different
Codex binary must re-prove both its credential mutation behavior and its nested
deny-read enforcement before its profile can be admitted.
