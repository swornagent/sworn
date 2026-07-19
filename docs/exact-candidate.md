# Exact local candidate

`internal/repo` is the sole owner of measured Git repository, target, workspace,
and candidate facts. It does not trust a remote alias, current checkout, agent
report, index, or branch shorthand as identity.

The boundary is deliberately small:

1. `Discover` measures physical Git common and object directories plus the
   object format while accepting an opaque repository ID from configuration. It
   never derives or equates repository identity from a path or URL.
2. `Open` compares those live facts with the immutable binding. A different
   common directory or object format fails as binding drift.
3. `BindTarget` accepts only a Git-validated full `refs/heads/...` ref and
   resolves its exact commit and tree.
4. `Materialize` copies that tree through a private index into a new plain
   workspace outside the source worktree and Git common directory. The workspace
   contains no `.git` path and cannot directly mutate repository refs or the
   user's index.
5. `Capture` rechecks the target, stages workspace bytes in another private
   index, writes the tree, and derives actual changed paths from the base-tree to
   candidate-tree diff. Baton literal-prefix scope is then enforced over every
   changed path; exclusions win.
6. For a changed tree, Sworn creates a single-parent commit whose exact parent is
   the bound base. For an unchanged tree, the candidate remains the base commit;
   no artificial commit is created.
7. The verified commit is retained under
   `refs/sworn/v1/candidates/<commit-oid>` with an expected-absent update and
   exact readback. `EnsureCandidate` can restore that ref after interruption only
   while the base, commit, trees, parent, and changed paths all still match.

Git runs with system and global configuration, replacement objects, prompts,
hooks, credential helpers, external diffs, filesystem monitors, and configured
clean/smudge/process filters disabled. Candidate capture uses bounded command
output and never invokes a shell. Repositories using object alternates, grafts,
or Gitlinks fail as unsupported instead of weakening the claim.

## Current boundary

This package exposes primitives to the later command/effect path; no mutating
CLI command calls them yet. The runtime config must persist the discovered
binding before activation. A separate [read-only Linux containment
foundation](contained-executor.md) now exists, but it is not wired to this
package and cannot yet return bounded builder changes. This slice does not run a
builder, create a Baton submission, store a candidate pointer in SQLite, execute
checks, or integrate a target.

The target is rechecked immediately before candidate retention. Any movement
afterward is still external reality and must be handled by the later integration
compare-and-swap and reconciliation path.
