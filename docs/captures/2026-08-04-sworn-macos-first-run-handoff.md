# Sworn macOS first-run on-ramp handoff

Date: 2026-08-04  
Repository: `sworn`  
Purpose: fresh-session input for investigating, specifying, and slicing a new release  
Status: evidence-backed problem statement; no implementation or release plan is approved here

## Fresh-session assignment

Investigate and plan a separate Sworn release that makes first use on macOS—and
first use generally—safe, intuitive, and honest. Use live repository and macOS
evidence as authority. Produce a Baton plan with small observable slices for
external approval; do not fold this work into the active
`2026-08-04-provider-neutral-authorization` release.

The first-run product contract should be:

> `sworn init` either leaves the project ready to use, or changes nothing and
> gives one clear, supported next action.

Do not solve the observed failure by merely dropping a Linux file check. Sworn's
native execution path is currently Linux-only, so the release must make an
explicit, evidence-backed macOS execution and containment decision.

## User observation

On an Apple Mac, from an existing clone of the Sworn repository that also has
historical Baton release material:

```text
Mac :: ~/projects/sworn ‹main› » sworn init
Project: /Users/brad.sawyer/projects/sworn
  created /Users/brad.sawyer/projects/sworn/.sworn/
  created /Users/brad.sawyer/projects/sworn/.sworn/runs/
  created /Users/brad.sawyer/projects/sworn/.sworn/.gitignore

sworn init: This machine is missing /etc/nsswitch.conf, which the sandboxed agent needs.

Baton releases: none in this project.
Sworn carries work that Baton has already planned, so a release must exist first.

Sworn cannot start a run until an AI connection file exists at
  /Users/brad.sawyer/projects/sworn/.sworn/drivers.json
```

This is a poor first-run experience because it:

- performs partial setup before discovering that the chosen driver cannot run;
- presents a Linux configuration file as a missing macOS prerequisite;
- describes the repository as having no Baton releases without distinguishing
  active local authority from historical or legacy release material;
- exposes an internal `drivers.json` file as the user's problem; and
- leaves no single, natural next action.

## Confirmed repository facts

### 1. `sworn init` assumes Linux runtime files on every platform

`cmd/sworn/init_command.go` declares one unconditional `initRuntimeTargets`
list containing:

```text
/etc/hosts
/etc/nsswitch.conf
/etc/resolv.conf
/etc/ssl/certs/ca-certificates.crt
```

`buildDriverConfig` resolves and hashes every target. The first missing path is
reported as something “the sandboxed agent needs.” There is no platform branch
before this validation.

### 2. Native agent execution is not implemented on macOS

`internal/driver/native_other.go` is selected for every non-Linux build. Its
native invocation, capture, certification, version, continuation, and recovery
functions all return `UNSUPPORTED_HOST`.

The Linux implementation uses bubblewrap and Linux process controls. Therefore
skipping `/etc/nsswitch.conf` would only move the user from a misleading init
failure to a later unsupported-host failure. It would not provide macOS support.

### 3. Init mutates the project before driver admission succeeds

`runInit` calls `prepareProjectDirectories` before
`writeProjectDriverConfig`. A driver-detection or platform failure therefore
leaves `.sworn/`, `.sworn/runs/`, and `.sworn/.gitignore` behind even though no
usable connection was created.

### 4. “Baton releases” means canonical local release refs

`reportProjectReleases` calls `baton.ListReleaseRefs`.
`internal/baton/catalog.go` enumerates only direct local refs under:

```text
refs/heads/release-wt/
```

A historical `docs/releases/` tree, a `.baton/releases/` tree reachable from
another ref, a remote-only release branch, or other legacy Baton material does
not by itself constitute active local release authority. The current output
collapses “no active canonical local release ref” into “none in this project,”
which is technically and operationally ambiguous.

## Required product outcomes

The Planner should turn these outcomes into the smallest complete plan rather
than treating this section as a predetermined slice list.

1. **Platform-aware preflight**
   - Determine the host, supported execution paths, installed agent, login
     state, credentials, and required runtime resources before changing the
     project.
   - Never prescribe Linux files on macOS.
   - A failed preflight leaves no misleading partial initialized state.

2. **A deliberate macOS execution contract**
   - Decide how Codex and/or Claude Code can be invoked safely on macOS.
   - Investigate whether a supported agent-owned sandbox can satisfy Sworn's
     containment contract, whether Sworn needs a macOS-specific containment
     adapter, or whether macOS must use an explicit remote runner.
   - Never silently fall back to an uncontained process.
   - If direct macOS execution is not delivered, fail early with an honest,
     supported alternative and one actionable next step.

3. **Natural first-run UX**
   - Prefer a guided TUI path for interactive users.
   - Detect and configure supported installed agents without asking users to
     understand `drivers.json`.
   - Keep configuration files, digests, refs, and Baton internals available for
     diagnostics but out of the ordinary happy path.
   - Finish with one obvious action, ideally opening or running `sworn`.

4. **Truthful release discovery and migration guidance**
   - Distinguish at least active canonical release authority, remote-only or
     otherwise recoverable authority, historical completed records, and legacy
     unsupported release material where those states can be proven safely.
   - Do not silently grant authority from historical files or ambiguous refs.
   - Offer the appropriate next action: resume, restore/fetch, migrate, create
     delivery, or inspect diagnostics.

5. **Recovery and repeatability**
   - Re-running init after interruption is idempotent.
   - Existing valid configuration is preserved unless replacement is explicit.
   - A partially created setup from older Sworn versions is detected and
     reconciled or explained without destructive cleanup.

## Acceptance boundaries to preserve

- Provider-neutral authorization remains exact and fail-closed.
- Git identity, provider membership, credentials, and authorship never become
  approval authority.
- Historical or legacy Baton material cannot silently become an active release.
- `.sworn/` remains local and ignored; `.baton/releases` remains reserved Baton
  metadata and never a product/runtime input.
- Do not weaken Linux containment or existing driver authentication as an
  incidental way to make macOS pass.
- Baton itself changes only if investigation proves a protocol defect. This is
  expected to be Sworn-scoped platform and product-onboarding work.
- Detached worker survival and reconnect remain separate unless the Planner can
  demonstrate they are a true prerequisite for the first successful macOS run.

## Investigation questions

The fresh session should answer these from current code and real Mac probes:

1. Which Sworn build produced the observation (`sworn version --json`), and is
   the installed binary current for the intended release?
2. Which Mac architectures and supported minimum macOS versions are required?
3. What containment guarantees do current Codex and Claude Code CLIs provide on
   macOS, and can Sworn validate and bind those guarantees without trusting
   mutable ambient configuration?
4. Can Sworn's existing HTTP/provider adapters supply a supported immediate Mac
   path, or are they unsuitable for the intended local-agent workflow?
5. What historical Baton material exists in the observed repository, and which
   refs are local, remote-only, merged, legacy, or absent?
6. Should init create a new delivery, launch a planning flow, or only explain
   that no active delivery exists? Keep plan approval and authority explicit.
7. What should `sworn init`, bare `sworn`, TUI setup, and diagnostics each own so
   that the same state is not described inconsistently?

Useful read-only probes on the Mac include:

```sh
sworn version --json
uname -a
uname -m
git for-each-ref --format='%(refname) %(objectname)' \
  refs/heads/release-wt/ refs/remotes/
find .baton/releases docs/releases -maxdepth 2 -type f 2>/dev/null | sort
command -v codex
codex --version
command -v claude
claude --version
```

Do not include credentials, connection-file contents, or agent authentication
tokens in the handoff or plan evidence.

## Verification expectations

Plan for evidence on real supported hosts, not cross-compilation alone:

- a clean Apple Silicon/macOS first-run journey;
- failed/unsupported macOS preflight with zero unintended project mutation;
- repeated and interrupted init recovery;
- existing valid and partial `.sworn` state;
- active, remote-only, historical, legacy, malformed, and absent release facts;
- Linux first-run and bubblewrap containment regression coverage;
- clear TUI and CLI copy reviewed as user-facing behavior; and
- exact driver/authority bindings after successful initialization.

Cross-compiled unit tests may support these checks but cannot prove the native
macOS runtime, containment, filesystem, credential, or process behavior.

## Release relationship and current state

Keep this as a new release identity. The active
`2026-08-04-provider-neutral-authorization` release has an approved revision 2
bound to:

```text
plan object: 416a6bc0af12e912ecf5e7a15cbc6c77dbc3a307
plan digest: sha256:a0b66478d3a34ee4930e78214be6926fb19efe29bf9887c79b9c643cc57854a8
approved target: 0588058f34130ff96836ede9e2969a3556dd314f
approval receipt: 2f6c9918e1a2a54ffafbac78ce4fe1ec199a7f3d
```

That release owns provider-neutral authority, shared TUI/MCP/CLI approval, Baton
RC14 embedding, and delegated Captain decisions. It does not own macOS native
execution or general first-run platform support. Do not amend it merely to
record this discovery, and do not move its approved target while it is in
flight.

## Expected fresh-session output

Return:

1. a live-state and Mac-evidence assessment;
2. the explicit macOS execution/containment decision and alternatives rejected;
3. a small Baton plan with observable outcomes, acceptance, constraints,
   dependencies, and mandatory real-Mac verification;
4. an exact raw-plan digest for external approval; and
5. any genuinely human-only product or security decision that remains.

Do not implement from this handoff before the exact plan is approved.
