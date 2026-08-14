# Sworn host and project configuration

Sworn separates **what a person reads** from **what the engine runs on**, and
makes every host location it reads or writes configurable with a sensible
default. This document is the operator reference: the schema, the defaults,
the environment overrides, and the refusal rules that keep two operators of
one repository honest.

Two scopes exist, and they are strictly apart:

- **Project-scoped locations** determine where durable release truth lives.
  They are read only from the one committed project file, never from a user
  or environment scope, so every operator of a repository resolves the same
  records, journals, contracts and commit prefixes.
- **Machine/user-scoped locations** are where the engine keeps its ephemeral
  and user-level state. They resolve from `SWORN_*` environment overrides with
  XDG-conformant defaults, and never from a hardcoded literal.

The guest filesystem inside containment is **not** operator territory. Paths
such as the guest workspace root, the guest input root, and the bind and mask
targets are constructed by the engine to make containment work; they are
compile-time constants, they are not configurable, and no configuration input
can reach them. The containment mask does follow the configured project
roots, so a relocated records or journals root is never left unprotected.

## The project configuration file

The project configuration file lives at the one fixed, non-configurable path
`docs/sworn/sworn.json`. The path itself is a compile-time constant and can
never be relocated, so every operator of a repository resolves the same
project truth. It is a reviewed, committed specification (the read surface);
the machine-written authority and run state it points at live under `.sworn/`.

Absent the file, the documented defaults apply and the engine behaves exactly
as if the defaults had been written.

```json
{
  "schema_version": "sworn.project-config/v1",
  "records_root":   ".baton/releases",
  "journals_root":  ".sworn",
  "contracts_root": "contracts",
  "commit_prefix":  "baton"
}
```

| Field           | Default          | Meaning                                                             |
|-----------------|------------------|---------------------------------------------------------------------|
| `schema_version`| `sworn.project-config/v1` | Schema identity; any other value is refused.             |
| `records_root`  | `.baton/releases`| Where release receipts, plans and control records are written.     |
| `journals_root` | `.sworn`         | Where the run journal, driver config and run manifests live.       |
| `contracts_root`| `contracts`      | Where declared slice contract files must live (enforced).          |
| `commit_prefix` | `baton`          | The prefix of engine commit-message subjects.                      |

Each field is optional: an absent field keeps the documented default. The
file must be a single closed JSON object of exactly these fields — an unknown
field (for example a `containment_binary`) is refused at parse time. Every
root must be a canonical repository-relative path (no leading `/`, no `..`,
no `.git` first segment). The three roots must be distinct.

### What the configured values change

- **Records root**: plan files and receipts are written under it, and every
  reserved-record admission, product-tree exclusion and candidate-scope check
  reads it. The containment mask follows its top segment, so a configured
  records root is masked inside containment exactly like the default.
- **Journals root**: the TUI, init and runtime derive `sworn.db`,
  `drivers.json` and the `runs/` manifest directory from it.
- **Contracts root**: declared `contract_path` values in a
  `sworn.release-manifest/v1` plan are enforced to live beneath it at both
  write-time and read-time contract resolution.
- **Commit prefix**: plan, approval, retirement, receipt and candidate commit
  subjects use it. An unconfigured repository keeps today's subjects exactly:
  plan and receipt actions write `baton(...)`, and the engine's
  implementation-candidate commit keeps its historical `sworn(...)` subject.
  Historical `baton(` and `sworn(` subjects are always recognised as
  engine-owned regardless of configuration.

## Machine/user-scoped locations

These resolve from environment overrides with XDG-conformant defaults under a
`sworn` subdirectory. They are the intended new machine/user defaults (A2),
so an unconfigured host places ephemeral state in the XDG locations rather
than hardcoded literals.

| Location        | Override               | Default (XDG)                                  |
|-----------------|------------------------|------------------------------------------------|
| Workspace root  | `SWORN_WORKSPACE_ROOT` | `$XDG_STATE_HOME/sworn/workspaces`             |
| Temp root       | `SWORN_TEMP_ROOT`      | `$XDG_STATE_HOME/sworn/tmp`                    |
| Credentials dir | `SWORN_CREDENTIALS_DIR`| `$XDG_CONFIG_HOME/sworn`                       |
| Artefact home   | `SWORN_ARTEFACT_HOME`  | `$XDG_DATA_HOME/sworn`                         |

Where an XDG variable is unset, the conventional fallback applies
(`~/.local/state`, `~/.cache`, `~/.config`, `~/.local/share`). Overrides must
be clean absolute paths; a relative, empty or root path is refused.

- **Workspace root**: the engine's workspace factory root (worktrees, leases).
- **Temp root**: all ephemeral scratch — certification roots, input
  projections, invocation scratch, Git homes/indexes/contexts, native
  captures. The native session memory root is the same configured temp root;
  because it must be memory-backed for crash recovery, a temp root that is
  not a tmpfs fails loudly in session validation rather than silently
  degrading.
- **Credentials dir**: where Sworn looks for agent credential files
  (`$XDG_CONFIG_HOME/sworn/.codex/auth.json` and
  `$XDG_CONFIG_HOME/sworn/.claude/.credentials.json` by default). The XDG
  default is always effective — it is never bypassed in favour of the user
  home. An operator who keeps the agent-owned files at their standard
  locations sets `SWORN_CREDENTIALS_DIR` to the parent directory that holds
  `.codex`/`.claude` (typically the user home).
- **Artefact home**: where Sworn's user-scoped artefacts live. `sworn skill
  install` (without `--home`) additionally places the skill there; the
  agent-discovery roots under the user home stay intact so agents keep
  finding the skill.

## Host tools

Host tool locations resolve from configuration or discovery, never absolute
literals that assume a Debian layout, so a nix, homebrew or minimal host
works without patching source.

| Tool             | Override      | Resolution                                  |
|------------------|---------------|---------------------------------------------|
| Git              | `SWORN_GIT`   | override, else `exec.LookPath("git")`       |
| Containment binary | `SWORN_BWRAP` | override, else `exec.LookPath("bwrap")`   |
| POSIX shell      | `SWORN_SH`    | override, else `/bin/sh`, else `LookPath("sh")` |

The containment binary's trust requirements are unchanged: it must be an
absolute, regular, executable file owned by `uid 0` with no group or world
write bits, and it must pass the same capability probe. Its path override is
**refused from project-scoped configuration** — the project config schema has
no containment-binary field, so a `docs/sworn/sworn.json` naming one is
refused at parse time.

## Refusal rules (A5)

Project-scoped locations cannot be overridden per user. The engine resolves
project truth only from the committed `docs/sworn/sworn.json`, and any
environment value naming a project-scoped location is refused with a named
error (`PROJECT_SCOPE_OVERRIDE_REFUSED`) rather than silently honoured:

- `SWORN_RECORDS_ROOT`
- `SWORN_JOURNALS_ROOT`
- `SWORN_CONTRACTS_ROOT`
- `SWORN_COMMIT_PREFIX`

Two operators of one repository therefore always resolve the same records,
journals, contracts and commit prefixes. Host-path overrides
(`SWORN_WORKSPACE_ROOT`, `SWORN_TEMP_ROOT`, `SWORN_CREDENTIALS_DIR`,
`SWORN_ARTEFACT_HOME`) are machine/user-scoped by design and are not refused.

## Guest paths stay fixed

The guest workspace root (`/workspace`), the guest input root (`/sworn/inputs`)
and every bind and mask target inside containment are compile-time constants.
The configuration types deliberately expose no guest-path field, and the
guest-path immutability test proves no configuration surface can alter any
bind or mask target. The only configurable influence on containment is the
mask: its reserved segments follow the configured records and journals roots
plus `.git`, so a configured root is never left unprotected.
