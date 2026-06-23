# Build & run

## Canonical build

```sh
make build
```

This produces `./bin/sworn` at the repo root. The binary is already gitignored
(`/bin/` in `.gitignore`).

## Run from repo root

Always run `./bin/sworn` **from the repo root**, not from a subdirectory:

```sh
./bin/sworn <subcommand> [flags]
```

### Why

SwornAgent writes its run-state **relative to the current working directory**:

| Artifact               | Path (relative to cwd)       |
|------------------------|------------------------------|
| Process registry DB    | `.sworn/sworn.db`            |
| Memory index           | `.sworn/memory.json`         |
| Journeys manifest      | `.sworn/journeys.json`       |
| Attestations           | `.sworn/attestations.json`   |
| Run scratch dirs       | `docs/release/run-*`         |

Running from the repo root ensures all of these land at the repo root, where
they are gitignored (`.sworn/` and `docs/release/run-*` in `.gitignore`).

Running from a subdirectory (e.g. `cmd/sworn/`) writes them there instead,
littering the tree with tracked-but-ignored artefacts.

## Build flags

`make build` uses the following `-ldflags`:

- `-s -w` — strip debug information (smaller binary)
- `-X main.version=$(VERSION)` — inject version; `VERSION` defaults to
  `0.0.0-dev` and is overridden by `release.yml` for tagged releases

## Other targets

| Target | Command        | Purpose                        |
|--------|----------------|--------------------------------|
| test   | `go test ./...` | Run the full test suite        |
| vet    | `go vet ./...`  | Run the Go static analyser     |
| fmt    | `gofmt -l -w .` | Format all Go source           |
| clean  | `rm -rf bin dist` | Remove build artefacts       |