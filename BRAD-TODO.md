# Brad TODO — Re-vendor Baton v0.4.3 into Sworn

Baton `v0.4.3` is published.

- Release: <https://github.com/sawy3r/baton/releases/tag/v0.4.3>
- Tag target commit SHA: `bf835ba7bd244660eee7afd8b03e1cb40a7d6703`
- Annotated tag object SHA: `81e095166501ddc38d2c612a90b7e332d59863fe`

## Context

The Sworn R3/T14 Baton integration slices built the intended re-vendor mechanism:

- `S48-baton-vendor`: `sworn baton vendor`
- `S49-baton-version`: semver tag pinning
- `S50-baton-governance`: `sworn baton diff`
- `S62-baton-upstream-source`: `sworn baton vendor --upstream`

Those files may live in the release/track worktree rather than the primary checkout. The oracle reported the relevant worktree as:

`/home/brad/projects/sworn-worktrees/release-2026-06-19-safe-parallelism-T14-baton-integration`

## Update Path

1. Work from a checkout that contains the R3/T14 `sworn baton` implementation.

2. Update `internal/adopt/baton/VERSION`:

```text
baton-protocol: v0.4.3
upstream: github.com/sawy3r/baton
upstream-sha: bf835ba7bd244660eee7afd8b03e1cb40a7d6703
vendored: 2026-06-25
```

Preserve any relevant existing provenance lines such as `rules-added:`.

3. Update `internal/prompt/VERSION.txt` to report:

```text
v0.4.3
```

4. If `internal/adopt/baton/VERSION` has an old `upstream-digest:` line, remove it before the first `v0.4.3` fetch. The upstream vendor command computes and writes the new digest after a successful run.

5. Build Sworn:

```sh
make build
```

6. Dry-run the re-vendor:

```sh
./bin/sworn baton vendor --upstream --tag v0.4.3 --check
```

7. Apply the re-vendor:

```sh
./bin/sworn baton vendor --upstream --tag v0.4.3
```

8. Verify drift against a local Baton checkout at `v0.4.3`:

```sh
git -C ~/projects/baton worktree add /tmp/opencode/baton-v0.4.3 v0.4.3
./bin/sworn baton diff /tmp/opencode/baton-v0.4.3
```

## Important Nuance

`sworn baton vendor --upstream` verifies the resolved tag SHA against `internal/adopt/baton/VERSION`, so the tag/SHA pin must be bumped before fetching `v0.4.3`.
