# Sworn project TUI

## Why this exists

Running `sworn` in a project should open the product, not print a help page.
The current release candidate has a terminal report and a browser board, but no
interactive terminal interface and no project-wide release discovery. That
misses the original multi-run cockpit scope.

The TUI is a frontend, not another engine. The journal, Baton projection,
runtime and typed cockpit commands remain the only sources of state and action
authority.

## Required experience

- Bare `sworn` in a project TTY opens the TUI. `sworn tui` is the explicit
  equivalent. Non-interactive bare use remains predictable and prints help.
- The first screen lists the project's local Baton releases. Saved runs or
  manifests whose Baton release is missing remain visible with a warning.
- A release can be opened before Sworn has started a run; its Baton graph still
  explains the planned work and current handoff state.
- When a saved Sworn run exists, its live state, questions, diagnostics and safe
  actions overlay that release.
- The view refreshes without losing the selected release.
- Start, pause, resume, cancel, takeover, retry, answer and notification
  redelivery use the same typed services and exact generation, work, epoch,
  attention and notification bindings as the command-line and browser views.
- Driver inspection and certification, JSON output, and starting the browser
  service remain explicit command-line operations. They are administrative
  commands, not controls on one delivery board.
- Small terminals remain usable, actions are named in plain language, and the
  help view plus contextual footer make every control discoverable.

## Project discovery

The Git root is the project boundary. Baton releases come from the project's
local `refs/heads/release-wt/*` branches, not only from the branch currently
checked out. Saved runs and run manifests still appear with a warning when
their Baton release branch is missing.

The default project runtime locations are:

```text
.sworn/sworn.db       saved runs
.sworn/drivers.json   AI connection configuration
.sworn/runs/*.json    admitted run manifests
```

Explicit `sworn tui` path flags may override these defaults. Discovery is
bounded, read-only and never creates a journal merely to render a screen.

## One command surface

The TUI consumes the Baton release graph before a run and `sworn.cockpit/v2`
snapshots after one exists. It does not infer whether a live-run action is safe.
It renders the snapshot's admitted actions, captures any human input, and
dispatches the same typed service used by the CLI and browser board. A start
action is available only for an admitted project manifest. Every command is
rechecked against the current board before dispatch, so a stale screen cannot
silently repeat an old action.

The historical Bubble Tea interface is useful evidence for terminal lifecycle,
navigation and responsive layout. Its old release state model is archaeology
and is not copied into this release line.

## Done when

From a project with more than one Baton release, a person can run `sworn`, move
between releases, inspect each graph, operate an existing run, answer a parked
question and start an admitted run without reconstructing command-line flags.
The CLI, TUI and browser board continue to show the same saved facts and invoke
the same engine.
