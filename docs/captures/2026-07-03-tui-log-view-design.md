# Design TL;DR — Durable worker logs + TUI live log view

- **Date:** 2026-07-03
- **Branch:** `tui/2026-07-03-board-live-logs`
- **Rule 9 artefact:** design-only (no production code in this commit). Implementation follows as a tracked slice.
- **Tracking:** GitHub issue #12 (TUI oracle/gates + live log view feature).
- **Human need (Brad, verbatim):** "have a live log view in the TUI so through the one UI you can manage the loop and dive in to see specific workers logs or the overall consolidated one."

## Problem

Today the loop is observable two ways, and neither lets you read what a worker actually did:

1. **Live board** (`internal/tui/concurrent.go` `LiveView`) — polls SQLite `.sworn/sworn.db` every tick and shows one row per track (slice, state, elapsed). This is *state*, not *narration*.
2. **Process stderr** — every worker/coordinator narration line (`fmt.Fprintf(os.Stderr, "[T1] ...")`) is scattered across `internal/scheduler/worker.go` (~40 call sites) and `internal/run/parallel.go` (~14 call sites). It scrolls past in the terminal that launched `sworn run`, interleaved across all tracks, and is **not captured anywhere**. There are **no log files in the repo today** — confirmed by grep.

So a user watching the TUI cannot answer "what is T3 doing / why did T3 pause" without hunting the launching terminal's scrollback, and once the loop exits that narration is gone. The consolidated multi-track stream is only ever seen live and interleaved, never per-track.

Two halves are needed: **(A) durable capture in the engine** (write the narration to per-track files without changing today's stderr behaviour), and **(B) a TUI log view** (read those files, per-track and consolidated, tail-following while the loop runs).

---

## A. Log capture (engine)

### A.1 Where logs live

```
.sworn/
  sworn.db
  logs/
    <release>/
      <track-id>.log     # one file per track  (e.g. T1.log, T3.log)
      merge__T3.log      # merge:<track> actors — ':' sanitised to '__'
      loop.log           # RunParallel coordinator-level lines (not track-prefixed)
```

**Justification for `.sworn/logs/<release>/<track-id>.log`:**

- `.sworn/` is the already-established sworn state dir (`internal/db.DefaultDir = ".sworn"`, houses `sworn.db`). Reusing it means **zero new gitignore work**: `internal/db.writeSelfIgnore` already stamps `.sworn/.gitignore` containing `*` on first `db.Open`, and git ignores everything beneath it — including `logs/`. In every real run `db.Open` fires before any worker writes, so the self-ignore is present by the time logs are created. (Defensive: the log-dir creator calls the same ensure-self-ignore path so logs are never accidentally tracked even if opened first.)
- `<release>/` subdir keys logs to the release the same way `tracks.release` / `events.release` key the DB, and matches how the TUI already scopes everything by `ReleaseName`. It also means a new run of a *different* release never clobbers a prior release's logs.
- Per-`<track-id>` file = the natural unit for "dive into a specific worker": the TUI's `TrackRow.ID` is the filename stem. No parsing/splitting a shared file to isolate one track.
- **Track IDs are sanitised for the filesystem**: `merge:T3` → `merge__T3` (colon is illegal on some FS and awkward in shells). Sanitisation is a pure `strings.Map` over `[^A-Za-z0-9._-]→'_'`. The raw track ID is still the in-file line prefix, so display is unaffected.

### A.2 What gets captured

- **Minimum (this slice):** exactly the existing `[<track>] …` stderr narration lines from `worker.go` (start / worktree materialise / router decision / slice running / pause reasons / result / auto-merge / done) and the `[<track>] result: …` lines from `parallel.go`. Coordinator lines in `parallel.go` that are *not* track-scoped (`RunParallel: …`, `INVARIANT-2: …`) go to `loop.log`.
- **Content-safety argument (load-bearing):** teeing changes the *destination*, not the *content*. Every line written to a `.log` file is a line **already written to stderr today**. These lines carry track IDs, slice IDs, router decision types, states, and Go error strings from control flow — the same data an operator already sees on their terminal. So the tee inherits the existing security posture; it introduces **no new exposure surface**, and the sink lives under the `*`-gitignored `.sworn/`.
- **Agent turn summaries — DEFERRED (Rule 2 deferral):**
  - *Why:* `internal/agent/agent.go` carries a package-level Security contract — "No logging of message history, file contents, or tool outputs … The message history may contain sensitive workspace data." The agent's `[]Message` history (system/user/assistant/tool content, tool arguments) is exactly what may hold secrets. Capturing even tool *names* per turn crosses into that package's forbidden territory and needs its own security review, and the seam lives in a different package that is **not** on this feature's critical path.
  - *Tracking:* issue #12, punch-list item "agent turn-summary capture (security-reviewed)".
  - *Acknowledgement:* stated here in plain text for the Captain/Coach — agent transcript capture is **out of scope** for this slice; only the pre-existing, already-on-stderr `[track]` narration is captured. Do not silently add it later without the security review.

### A.3 The tee seam (SURGICAL — engine constraint)

`internal/scheduler` and `internal/run` are planned touchpoints of the in-flight `2026-06-28-driver-contract` release (S06-loop-dispatch-rewire, S07-scheduler-failfast, both still `planned`). **This design must not refactor worker control flow** or it will collide on merge. The seam is therefore the narrowest possible:

- **You cannot tee `os.Stderr` per-track.** `os.Stderr` is process-global and every track goroutine shares it; redirecting it globally would (a) interleave all tracks into one sink and (b) be a Rule 11 process-global mutation. The seam must be an **explicit per-track `io.Writer`**, not a redirect.

- Add **one field** to `WorkerOptions`:

  ```go
  // LogDir, when non-empty, is the directory into which the worker tees its
  // stderr narration (append-only, one <track>.log per track). Empty = today's
  // behaviour exactly (stderr only). Set by RunParallel to .sworn/logs/<release>.
  LogDir string
  ```

- At the top of `RunTrack`, construct the tee once and thread it as a plain `io.Writer`:

  ```go
  w, closeLog := trackWriter(opts.LogDir, trackID) // io.Writer + func() error
  defer closeLog()
  ```

  `trackWriter` returns `os.Stderr` unchanged when `LogDir == ""` (so tests and legacy callers are byte-for-byte unaffected), otherwise a `*teeLogger` (below).

- The change to the ~40 call sites is a **mechanical token replacement** `os.Stderr → w`, **not** a logic change — no branch, loop, or control-flow edit. The three helpers that also emit (`finishTrack`, `stripCaptainProceed`, `RunTrackLegacy`) take the `io.Writer` as an added parameter (mechanical signature widen, default `os.Stderr` from callers). This keeps the diff reviewable line-by-line and rebase-friendly against S06/S07.
  - *Merge-ordering note (Rule 2, tracked):* if S06/S07 land first, re-target the token replacement against their rewritten worker; if this lands first, S06/S07 inherit `w` as the narration sink. Flagged to the Coach so the ordering is a decision, not a surprise.

- `RunParallel` (`parallel.go`) computes `logDir = filepath.Join(repoRoot, ".sworn", "logs", release)`, `os.MkdirAll`s it (0o755) once before spawning, ensures the self-ignore, sets `opts.LogDir` per worker, and opens `loop.log` for its own coordinator lines via the same `trackWriter("loop", …)` helper.

### A.4 The `teeLogger` (format + crash-safety)

```go
type teeLogger struct {
    stderr io.Writer // os.Stderr — verbatim, preserves today's output exactly
    file   io.Writer // *os.File opened O_APPEND|O_CREATE|O_WRONLY, 0o644
}
// Write writes p verbatim to stderr, and a timestamp-prefixed copy to file.
```

- **stderr side is byte-for-byte identical to today** — the tee never alters what the launching terminal shows. This is the "preserve current stderr behaviour" requirement, enforced by test (A.6 below).
- **file side prepends a short timestamp per line**: `15:04:05.000 [T1] router: …`. The timestamp is the **enabler for the consolidated view** — per-track files are independently chronological, and the TUI k-way-merges them on this prefix (see B.3). stderr stays bare so live watching is unchanged.
- **Crash-safety:** `O_APPEND` = every `write(2)` is positioned at end-of-file atomically by the kernel, so concurrent goroutines (and the coordinator) never corrupt each other's files, and a crash truncates at most the final partial line — prior content is never rewritten. Writes are **line-oriented** (the narration always ends each message with `\n`), so the append boundary is a line boundary. **No `fsync`** — the OS page cache is sufficient for an operator log; durability-to-power-loss is not a requirement here, and fsync-per-line would throttle the loop.

### A.5 Rotation / size policy (simplest correct)

- **Soft cap per file: 8 MiB.** Checked cheaply on open and every ~256 KiB written. On exceed: single `os.Rename(<track>.log → <track>.log.1)` (replacing any existing `.1`) and reopen — **one prior generation kept, two files max per track.** Bounded disk (≤16 MiB/track), no unbounded growth, no external rotor dependency (stdlib-only per AGENTS.md).
- Loop runs are bounded in wall-time, so in practice a single run rarely hits the cap; the cap exists to stop a pathological retry-storm from filling the disk. The TUI reads `.log` (and, when tailing, ignores `.1` — scrollback into `.1` is a tracked follow-up, not this slice).

---

## B. Log view (TUI)

### B.1 Entry points ("manage the loop AND dive in — one UI")

- **From the live view** (`viewLive`): the existing track table (`LiveView`) gains a **cursor** (`j`/`k`) and **`enter` opens the selected track's log** (`<track-id>.log`). This is the "dive into a specific worker" journey: select row → enter. Reuses the exact `j`/`k`/`enter` idiom already in `handleBoardKey`.
- **Consolidated view:** a new key **`L` (shift-L)** from the live view opens the **interleaved all-tracks stream** (`loop.log` + every `<track>.log`, merge-sorted). "The overall consolidated one."
- **From the board view** (`viewBoard`): `L` also opens the consolidated log for the current release, so logs are reachable without first entering the live table. (Rule 1: the affordance is owned by the root `Model` key dispatch, which is where the reachability test drives it.)
- New root state `viewLog`; new component `LogView` in a new file `internal/tui/logview.go` (no edits to `concurrent.go`'s poll logic beyond adding the row cursor + `enter` handler). `esc` returns to the originating view (live or board); `b` back to board — consistent with `handleLiveKey`.

### B.2 `LogView` component (mirrors `LiveView` construction)

```go
type LogView struct {
    ReleaseName string
    LogDir      string   // .sworn/logs/<release>
    Track       string   // "" => consolidated; else single <track-id>
    Lines       []string // rendered, already interleaved if consolidated
    offset      int      // scrollback position (0 = follow tail)
    follow      bool     // true while pinned to tail
    Height      int      // viewport rows, from Model.Height
}
```

- `StartLogView(repoRoot, release, track)` opens/stats the file(s) and does the **first read synchronously** so the initial `View()` is populated — same pattern as `StartLiveView`.

### B.3 Per-track vs consolidated

- **Per-track** (`Track != ""`): read `<track>.log`, strip nothing, show lines as-is (they already carry the `[track]` prefix and file timestamp).
- **Consolidated** (`Track == ""`): read every `*.log` in `LogDir` (excluding `.1`), **k-way merge-sort by the leading `HH:MM:SS.mmm` timestamp** each line carries (A.4). Because every file is already chronological, this is a linear merge of already-sorted streams — no full sort needed. Lines that fail to parse a timestamp (should not happen for teed lines) sort stably after the last parsed time of their file. This is the "by mtime/line-timestamp" interleave the request asked for, done at **read time** — so there is **no second consolidated file to write, dedupe, or keep crash-consistent** at the engine layer.

### B.4 Tail-follow, scrollback, viewport

- **Tail-follow reuses the existing tick idiom.** `LogView.Init()` returns the same `tea.Tick(1s)` `tickMsg` cmd `LiveView` uses; `Model.Update`'s `tickMsg` case forwards to `LogView` when `state == viewLog` (exactly mirroring the `viewLive` forward at model.go:91-100). On each tick `LogView` re-reads new bytes from the current end offset (`os.File.Seek` + read the tail; cheap incremental read, not a full re-read) and appends. While `follow == true` the viewport stays pinned to the bottom — **live log while the loop runs**, no new polling machinery invented.
- **Scrollback:** `k`/`↑` and `j`/`↓` move `offset`; `g`/`G` jump top/bottom; any upward scroll sets `follow = false` (freeze), `G` re-pins `follow = true`. The viewport renders `Lines[offset : offset+Height]` sized from `Model.Height` (already stored at model.go:89).
- When the loop is **not** running the tick still fires but reads no new bytes — the view is a static pager. When it **is** running, new narration streams in. Same component, both cases.

---

## C. Stakes classification (Rule 9)

| # | Choice | Type | Rationale |
|---|--------|------|-----------|
| C1 | **Log file location + on-disk format** (`.sworn/logs/<release>/<track>.log`, `HH:MM:SS.mmm [track] msg` lines) | **Type-1** | Hard to reverse and wide blast radius: the moment logs exist on disk, other tools (a hosted portal ingest, `sworn` subcommands, an operator's `tail -f`, CI log collection) may read the path and line format. Changing either later breaks those readers. This is a **contract surface** → full human decision with the location/format options + rationale recorded in `status.json`. The model proposes; the human ratifies. |
| C2 | **The tee seam shape** (add `WorkerOptions.LogDir` + `io.Writer` threading vs. any control-flow refactor) | **Type-1** | Architecturally significant: it touches two packages that are live touchpoints of an in-flight release; the choice of a minimal-diff writer seam vs. a refactor determines merge-safety of another release. Recorded with the "why surgical" rationale + the S06/S07 merge-ordering note. |
| C3 | Rotation policy (8 MiB soft cap, keep-1) | **Type-2** | Easy to reverse, local to `teeLogger`; a noted default. Revisit if real runs show it's wrong. |
| C4 | Consolidated view = **read-time interleave** (no second file) | **Type-2** | Local to the TUI reader; reversible (could add a write-time consolidated file later without changing the per-track contract). Noted default. |
| C5 | Keybindings (`enter` = per-track, `L` = consolidated, `esc`/`b` back) | **Type-2** | Trivially reversible UI detail; follows existing idioms. |
| C6 | Agent turn-summary capture | **Deferred** (Rule 2, §A.2) — not a design choice this slice makes; explicitly out of scope pending security review. |

C1 and C2 are the two Type-1 choices; both require a recorded human decision in `status.json` before implementation starts (`planned → in_progress`). The model has proposed defaults for both but **may not self-ratify** them.

---

## D. Test plan

### D.1 Reachability (Rule 1) — model-level TUI tests

All through the real integration point (`internal/tui` `Model.Update`/`View` and `LogView.Update`), following the existing patterns in `tui_test.go` (`TestLiveBoardToggle`, `TestModelTickForwarding`, `TestConcurrentStatusPoll`):

1. **`TestLiveEnterOpensTrackLog`** — seed `.sworn/logs/<release>/T1.log`; drive `m.Update(KeyMsg enter)` from `viewLive` with the row cursor on T1; assert `m.state == viewLog`, `m.Log.Track == "T1"`, and `m.View()` contains a seeded T1 line. (Dive-into-a-worker journey.)
2. **`TestConsolidatedLogInterleaves`** — seed `T1.log` and `T3.log` with timestamp-ordered lines that interleave; open consolidated (`L`); assert `View()` shows lines in global timestamp order across both files.
3. **`TestLogTailFollowOnTick`** — open a log view, append a new line to the file on disk, send `tickMsg` **through `Model.Update`** (not directly on `LogView`), assert the new line now renders and `follow` kept the viewport pinned to the tail. (Proves the tick/poll idiom is wired through the root model, the S04b lesson from `TestModelTickForwarding`.)
4. **`TestLogScrollbackFreezesFollow`** — `k` scrolls up and sets `follow=false`; a subsequent `tickMsg` appends to `Lines` but does **not** move the viewport; `G` re-pins.
5. **`TestBoardLOpensConsolidated`** — `L` from `viewBoard` reaches `viewLog` consolidated. (Second entry point.)
6. **`TestLogViewMissingDirGraceful`** — no `logs/` dir: opening shows an empty "no logs yet" state, never panics (mirrors `HasInProgressTracks` treating missing as empty).

### D.2 Engine — tee writes file AND stderr unchanged

In `internal/scheduler` (or a small `teelog` helper package if cleaner):

7. **`TestTrackWriterTeesToFileAndStderr`** — construct `trackWriter(tmpLogDir, "T1")`, write a known line; assert (a) the captured stderr side equals the input **byte-for-byte** (no timestamp, no mutation — the "stderr unchanged" guarantee), and (b) `T1.log` contains the line **with** a parseable `HH:MM:SS.mmm` prefix and the original text.
8. **`TestTrackWriterEmptyDirIsStderrOnly`** — `LogDir == ""` returns a writer whose only effect is stderr; **no file is created** under any tmp dir (legacy/back-compat invariant).
9. **`TestTrackWriterAppendCrashSafe`** — write, "reopen" (new writer, same path), write again; assert the file contains **both** runs' lines in order (append, not truncate).
10. **`TestTrackWriterRotates`** — with an artificially low cap, exceed it; assert `<track>.log.1` appears and the live `<track>.log` continues.
11. **`TestTrackIDSanitisedForFilename`** — `merge:T3` produces `merge__T3.log` and never a path with a raw `:`.

### D.3 Reachability artefact (produced at implementation, not now)

An actual `sworn run` (or a scripted fake-agent run) that (a) emits narration to stderr as today AND (b) leaves populated `.sworn/logs/<release>/*.log`, followed by opening the TUI, pressing `enter` on a track and `L` for consolidated, and observing the lines — captured as the slice's proof-bundle reachability step. A green `go test` alone is **not** the artefact (AGENTS.md Rule 1).

---

## E. Scope boundary (what this design does NOT do)

- No agent transcript/turn capture (§A.2 / C6 — Rule 2 deferral, tracked #12, security-review-gated).
- No change to `os.Stderr` behaviour — stderr output is byte-identical to today (tested, D.1 #7).
- No refactor of worker control flow — additive `io.Writer` seam only (§A.3).
- No scrollback into rotated `.log.1` from the TUI (tracked follow-up).
- No hosted/portal log shipping — local files only; the on-disk contract (C1) is the seam a future shipper would read.
