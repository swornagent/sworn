# Launch a release

This guide walks an operator through launching a release with the commands
as they stand after the launch-legibility track: `plan pin` (`--write`),
`plan lint`, `plan record` (with abbreviated ids), `sworn manifest
canonical` for runtime manifests and driver configs, the canonical manifest,
operator config (mode `0600`) and `serve`. Every refusal names its cause: one
friendly line, then `Technical code: <CODE>`, then at most one bounded
detail line. No secret, credential, absolute operator path or raw provider
body is echoed; where a message names an input it names the kind and flag,
not its contents.

## plan pin, lint, record

Pin recomputes every manifest-mirrored slice fact from contract bytes.
Lint proves every slice contract against the chosen source and runs the
recording-time scope lint. Record writes a plan revision.

```sh
sworn plan pin --manifest /abs/manifest.md --project /abs/project [--commit REV] [--write]
sworn plan lint --manifest /abs/manifest.md --project /abs/project [--commit REV]
sworn plan record --manifest /abs/manifest.md --project /abs/project --summary TEXT \
  [--detail-file /abs/detail.md] [--commit REV] [--contract-tree REV]
```

Without `--write`, `pin` prints the pinned bytes to stdout so they can be
piped, and says on stderr that it printed. With `--write`, `pin` replaces
the plan file named by `--manifest` with the pinned bytes atomically (a
sibling temporary file in the same directory, then rename) and prints the
plan digest `plan: sha256:...` to stdout. The write refuses a non-regular
or symlinked target, keeps the target's existing permission bits, and
enforces the same size limit the reader uses. Re-running `pin` on its own
output is byte-stable: the pinned bytes and the digest do not change.

`--commit` (pin, lint, record) and `--contract-tree` (record) accept any
revision Git resolves to exactly one commit: a full id, an unambiguous
abbreviation or a ref name (`HEAD`, `main`, a branch). The wider Git
revision syntax (`HEAD~1` and similar) is admitted where it resolves to
exactly one commit. Resolution happens once at the command boundary
through one exported, sanitized `internal/gitx` resolver; every downstream
record, digest and receipt receives the full id, exactly as when a full id
is typed.

The full resolved id is printed in the command output:

- `pin` prints `commit: <full-id>` on stderr so stdout stays pure for
  piping: the pinned bytes without `--write`, or `plan: sha256:...` with
  `--write`. A second stderr line confirms the action: `pinned manifest
  printed to stdout (--manifest)` without `--write`, or `wrote pinned
  manifest (--manifest)` with `--write`.
- `lint` prints `commit: <full-id>` on stdout alongside the `PASS` lines.
- `record` prints `commit: <full-id>` (when `--commit` was given) and
  `contract-tree: <full-id> (from --contract-tree|--commit|HEAD)` on stdout
  alongside the `Recorded plan revision` lines. `--contract-tree` takes
  priority, then `--commit`, then the working repository `HEAD`, each
  resolved once. `HEAD` is resolved through the same resolver; `cmd/sworn`
  runs no git command of its own.

Refusals name the flag that carried the bad value and never echo the value:

| Code | Cause | Remedy |
| --- | --- | --- |
| `AMBIGUOUS_REVISION` | The revision matches more than one object. | Use the full id or an unambiguous ref. |
| `REVISION_NOT_FOUND` | The revision does not resolve to exactly one commit (unknown ref, unknown abbreviation, empty, over-length, NUL/control characters or a leading `-`). | Check the flag value; use a full id, `HEAD` or an existing branch. |
| `NON_COMMIT_OBJECT` | The revision resolves but not to a commit (a blob or tree id). | Use a commit id or a ref that points to a commit. |
| `GIT_EXECUTION_FAILED` | The resolution transport failed (deadline, overflow or unquiesced process group). | Retry; if it persists, check Git and the repository. |
| `STALE_BINDING` | The manifest's mirrored slice facts do not match the contract bytes (digest, outcome, dependencies, consumed products, touchpoints or waivers). | Run `sworn plan pin --write --manifest ABS --project ABS` first to refresh the pinned facts. |

When `plan lint` or `plan record` refuses with `STALE_BINDING`, the message
tells the operator to run `sworn plan pin --write` first. The hint appears
only on those two verbs; no other `STALE_BINDING` (assembly, receipts)
names `pin`.

## plan pin --write

```sh
sworn plan pin --manifest /abs/manifest.md --project /abs/project --write
sworn plan pin --manifest /abs/manifest.md --project /abs/project --commit REV --write
```

`--write` replaces the file named by `--manifest` with the exact pinned
bytes and prints the plan digest (`protocol.DigestBytes` of those bytes) as
`plan: sha256:...` on stdout. The digest is the same identity the run
record uses for the plan, not a Git object id. The write is atomic and
preserves the target's mode; a `0600` plan file stays `0600`. Pinning an
already-pinned file returns identical bytes, so the second run changes
nothing and prints the same digest.

## Canonical manifest

The runtime manifest is compact JSON with a final newline: exactly
`json.Marshal(manifest)` plus `\n`. `sworn run` and `sworn serve` admit
only `sworn.runtime-manifest/v5`; legacy `v2`/`v3`/`v4` refuse with
`MIGRATION_REQUIRED`.

Serve obtains the manifest code by running `runtime.ParseManifest` on the
same bytes before cockpit admission, so the refusal is the runtime's own
code rather than the collapsed `INVALID_MANIFEST` that cockpit's manifest
admission returns.

| Code | Cause | Remedy |
| --- | --- | --- |
| `NONCANONICAL_MANIFEST` | Valid JSON but not the exact canonical bytes (whitespace, key order or newline differs). | Run `sworn manifest canonical --manifest ABS` to rewrite it in canonical form. |
| `MIGRATION_REQUIRED` | Legacy `v2`/`v3`/`v4` manifest. | Migrate the manifest to `v5`. |
| `INVALID_MANIFEST_VERSION` | Unknown `schema_version`. | Use `sworn.runtime-manifest/v5`. |
| `INVALID_MANIFEST` | The `--manifest` file cannot be read as an admitted regular file, or its content is invalid. | Check `--manifest` points to an absolute regular file and the content is a current canonical manifest. |
| `MANIFEST_RUN_MISMATCH` | The manifest's `run_id` differs from `--run`. Neither id is echoed. | Use a matching `--run` and `--manifest`. |
| `NONCANONICAL_JSON` | The driver config (`--config`) is valid JSON but not the exact canonical bytes. | Run `sworn manifest canonical --driver-config ABS` to rewrite it in canonical form. |

## sworn manifest canonical

Every canonical launch input is produced by command instead of
reverse-engineering the encoder:

```sh
sworn manifest canonical --manifest /abs/manifest.json
sworn manifest canonical --manifest /abs/manifest.json --write
sworn manifest canonical --driver-config /abs/drivers.json
sworn manifest canonical --driver-config /abs/drivers.json --write
```

Exactly one of `--manifest` or `--driver-config` is required, plus an
optional `--write`. Without `--write`, the command prints the exact bytes
admission accepts to stdout (pure for piping) and says on stderr that it
printed. With `--write`, it replaces the named file atomically (a sibling
temporary file in the same directory, then rename), leaves stdout empty,
and says on stderr that it wrote. Writes refuse a non-regular or symlinked
target, keep the target's existing permission bits, and enforce the same
size limit the reader uses for that input.

The two canonical rules stay as they are and the command applies the right
one per input so the operator never needs to know the difference: the
runtime manifest ends with one newline (`json.Marshal` plus `\n`), the
driver config has none (`canonicalJSON` without a newline). Admission is
the only judge of canonicality: the manifest path accepts its result only
if `runtime.ParseManifest` admits it, the driver path only if
`driver.DecodeDriverConfig` admits it, so the command cannot drift from
the engine. The digest of any manifest or driver config that admission
accepts today is unchanged, and the command produces those same bytes from
it. A validation refusal carries admission's own code (for example
`INVALID_ROLES` or `INVALID_DRIVER_CONFIG`), never a generic line.

## Operator config

The operator config is opt-in. Absent, `serve` listens only on
`127.0.0.1:7337` with no public listener, webhooks or telemetry. Present,
it must be a regular, non-symlink file with mode `0600`, reached by a
clean absolute path whose parent is not reached through a symlink:

```sh
chmod 0600 /absolute/path/operator.json
sworn serve --run RUN_ID --journal /abs/run.sqlite \
  --config /abs/drivers.json --operator-config /abs/operator.json
```

Public listening, webhook destinations and OpenTelemetry export are
opt-in siblings (`public`, `webhooks`, `otel`, `share`) validated for
exact fields, canonical listen authorities, origins, tokens, URLs and
secrets.

| Code | Cause | Remedy |
| --- | --- | --- |
| `OPERATOR_CONFIG_UNAVAILABLE` | The file cannot be admitted (bad path, missing parent, parent symlink, symlink file, replacement between inspection and open, read failure or out-of-range size). | Check the `--operator-config` path is absolute and clean, the parent exists without symlinks, and the file is a regular non-symlink file. |
| `OPERATOR_CONFIG_INSECURE_MODE` | A regular, non-symlink file with an in-range size whose mode is not `0600`. The refusal names the required mode. | Run `chmod 0600` on the file. |
| `OPERATOR_CONFIG_INVALID` | The bytes are not an admitted operator config (ambiguous JSON, non-exact fields, schema, `local.listen`, `public`, webhook, `otel` or `share` validation). | Fix the JSON to the `sworn.operator-config/v1` schema with canonical listen authorities and valid opt-in blocks. |

## serve

Project mode (no `--run`/`--journal`) serves every discoverable run and
release from the current checkout. Run mode (`--run` with `--journal`,
optional `--manifest`, `--config`, `--operator-config`) hosts one run and
drives it in-process, creating the run only at `Start`, never at admission.

On failure `serve` keeps its friendly first line, names which input
failed, and prints the typed code like every other command:

```text
sworn serve: Could not open the local delivery board (journal). Check the run, journal, and operator settings.
Technical code: JOURNAL_UNAVAILABLE
journal (--journal)
```

The seven inputs are `manifest`, `journal`, `run authority`, `operator
config`, `driver config`, `Git project` and `listener`. The detail line is
the input kind (and flag) only, for example `manifest (--manifest)`,
`journal (--journal)`, `run authority (--run)`, `operator config
(--operator-config)`, `driver config (--config)`, `Git project` or
`listener`. For `OPERATOR_CONFIG_INSECURE_MODE` the friendly line also
names the required mode `0600`, since the detail stays the input kind.

| Input | Representative codes | Remedy |
| --- | --- | --- |
| `manifest` (`--manifest`) | `NONCANONICAL_MANIFEST`, `MIGRATION_REQUIRED`, `INVALID_MANIFEST_VERSION`, `INVALID_MANIFEST`, `MANIFEST_RUN_MISMATCH` (above). | Run `sworn manifest canonical --manifest ABS` for `NONCANONICAL_MANIFEST`; otherwise fix the manifest bytes or match `--run` to the manifest. |
| `journal` (`--journal`) | `JOURNAL_UNAVAILABLE` (wrapping `INVALID_PATH`, `INSECURE_PERMISSIONS`, `OPEN_FAILED`, `IDENTITY_MISMATCH` and similar), `RUN_NOT_FOUND`, `INVALID_RUN`. | Check `--journal` is absolute and clean, its parent exists without symlinks, an existing file is a regular `0600` file, and the run exists in it. |
| `run authority` (`--run`) | `OPERATOR_UNAVAILABLE` for a mismatched or unavailable binding; the `Reconcile*` runtime codes where a reconcile fails. | Ensure the journal binding's manifest digest matches the served manifest, or serve with `--manifest` for a run not yet started. A conflicting binding never activates background work. |
| `operator config` (`--operator-config`) | `OPERATOR_CONFIG_UNAVAILABLE`, `OPERATOR_CONFIG_INSECURE_MODE`, `OPERATOR_CONFIG_INVALID` (above); webhook-destination cockpit codes for invalid webhook endpoints. | Fix the file admission, mode or JSON as above. |
| `driver config` (`--config`) | `NONCANONICAL_JSON`, `INVALID_CONFIG_PATH`, `CONFIG_UNAVAILABLE`, `RESOURCE_LIMIT`, `INVALID_DRIVER_CONFIG`, `FACTORY_UNAVAILABLE` and the driver contract codes from admission. | Run `sworn manifest canonical --driver-config ABS` for `NONCANONICAL_JSON`; otherwise check `--config` is absolute and clean and the file is a canonical secret-free driver config. |
| `Git project` | `GIT_UNAVAILABLE`, `INVALID_REPOSITORY`, `INVALID_GIT_EXECUTABLE`, `INVALID_PROJECT_CONFIG` and the `gitx` codes from opening the project. | Install Git or make it available on `PATH`, run inside an admitted Git project, and check the committed project config. |
| `listener` | `OPERATOR_UNAVAILABLE` for a bind, serve, readiness-write or shutdown failure; `INVALID_HTTP_CONFIG` for handler config; `INVALID_TELEMETRY`, `OTEL_EXPORTER_START_FAILED`, `INVALID_EVALUATOR` for telemetry startup. | Check the listen address is free and canonical, the public TLS/origin/token block is valid, and telemetry exporters can start. A later listener failure leaves no phantom run. |

A failed startup never creates a run before `Start` and never leaves a
phantom run: the readiness line `sworn serve: ready` is printed only after
every input is admitted and both listeners are bound.
