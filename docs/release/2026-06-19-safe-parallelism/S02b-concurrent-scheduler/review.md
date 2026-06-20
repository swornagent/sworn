# Captain review — S02b-concurrent-scheduler
Date: 2026-06-20
Design commit: a08fc175c2b97fb577b1fefaf68b6621daa2383a

## Pins

1. [mechanical] §3 — `internal/board/track.go` + `track_test.go` not in planned touchpoints
   What I observed: Design §3 introduces two new files (`internal/board/track.go`,
   `internal/board/track_test.go`) as the home for `ParseTracks()` and `TrackInfo`. These
   files appear nowhere in the spec's "Planned touchpoints", `status.json.planned_files`,
   or the touchpoint matrix in index.md. Verified: no other in-release spec touches
   `internal/board/` at the code level (grep across all sibling spec.md confirmed). No
   collision risk, but the touchpoint gap means the verifier's proof-bundle check against
   `planned_files` will show the files as undeclared divergences.
   What to ask the implementer: Before writing code, add `internal/board/track.go` and
   `internal/board/track_test.go` to `status.json.planned_files`. Then declare the
   divergence in proof.md "Divergence from plan" (one line is fine: "added
   internal/board/track.go + track_test.go; not in original spec touchpoints; no matrix
   collision confirmed"). The verifier then grades against the updated planned_files.

2. [mechanical] §2 Decision 4 — sync.Map substitution must be pre-declared in proof.md
   What I observed: The spec's "In scope" section explicitly prescribes "uses
   `sync.WaitGroup` + channels for dependency signalling" in the `internal/run/parallel.go`
   entry. Design Decision 4 substitutes a `sync.Map`-backed outcome store with rationale
   ("avoids N×M channel plumbing"). All six ACs are satisfied by the sync.Map approach
   — this is a sound technical choice. But a verifier grading against spec text will
   encounter the prescriptive "channels" language and may return FAIL unless the deviation
   is pre-declared.
   What to ask the implementer: Add one sentence to proof.md "Divergence from plan":
   "Spec prescribes `sync.WaitGroup + channels` for dep signalling; implemented with
   `sync.Map` outcome store instead. All ACs pass; sync.Map avoids N×M channel fan-out
   while delivering equivalent ordering and failure-cascade semantics."

3. [memory-cited] All §2 decisions use stdlib only — [[project-dep-policy]] confirmed ✓
   What I observed: Decisions 1-5 introduce `context`, `sync`, `sync/atomic`, and
   `sync/atomic` (all stdlib). No third-party dependency is added. The existing
   `internal/board/index.go` (the package S02b extends) is also stdlib-only.
   What to ask the implementer: No action required. [[project-dep-policy]] says "minimal,
   justified deps + ADR for each"; zero new deps is squarely within policy. Ack confirms
   the citation.
   Citation: [[project-dep-policy]]

## Summary

Pins: 3 total — 2 [mechanical], 0 [memory-cited as pin], 1 [memory-cited as confirmation]
Critical pins (none): both mechanical pins are apply-inline paperwork; neither causes functional breakage if unaddressed before code runs, but pin 2 is a near-certain verifier FAIL if omitted from proof.md.

## Smaller flags (not pins, worth one-line ack)

(a) §4 worktree-cleanup attribution: "reaped by S01's stale-PID detection" is imprecise.
Confirmed via grep on `internal/supervisor/supervisor.go`: S01 has zero `worktree`
references — it manages DB rows (process registry) only. Git worktree directories left on
disk by crashed workers are not reaped by S01. The design already says "actual directory
cleanup is a future concern," so the deferral is correct. Just clarify in proof.md:
"orphan DB row reaped by supervisor.Reap(); orphan git-worktree directory left on disk
(no cleanup in this slice — future concern)."

(b) S08b future note (no action now): `get_board` in S08b will need to parse track/slice
structure from index.md. Since T4 depends_on T1, `board.ParseTracks()` (created in this
slice) will be available. Mentioning it here so the S08b implementer knows to call
`board.ParseTracks()` rather than re-parsing frontmatter independently. No design change
needed in S02b.

## §6 questions — Coach ack

Both questions in design §6 are self-answering. No human authority needed:

- Q1 (release_worktree_path source): Parse from frontmatter. Correct. The `index.md`
  frontmatter carries `release_worktree_path` as the single source of truth; reading it
  avoids the coupling of a CLI flag.
- Q2 (test fixture location): Embed fixture YAML strings in test files. Correct. Keeps
  tests self-contained; no external fixture coupling.

## Suggested ack reply
<!-- Coach-extractable section: `coach ack S02b-concurrent-scheduler` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

Design is sound. 2 mechanical pins + 1 memory-cited confirmation. §6 questions acked below.

1. **Declare `internal/board/track.go` + `track_test.go` in planned_files.** Before writing code, add both files to `status.json.planned_files`. Then add one line to proof.md "Divergence from plan": "added `internal/board/track.go` + `track_test.go`; not in original spec touchpoints; no touchpoint-matrix collision (confirmed by Captain review)."

2. **Pre-declare sync.Map divergence in proof.md.** Add to proof.md "Divergence from plan": "Spec's 'In scope' prescribes `sync.WaitGroup + channels` for dependency signalling; implemented with `sync.Map` outcome store instead. All ACs pass; sync.Map avoids N×M channel fan-out while delivering equivalent ordering and failure-cascade semantics."

Flags (not pins): (a) clarify S01's scope in proof.md worktree-cleanup note: "orphan DB row reaped by supervisor.Reap(); git-worktree directory left on disk (future concern)."

§2 decisions 1–5 acked — all stdlib, [[project-dep-policy]] confirmed.
§6 Q1 acked: parse release_worktree_path from frontmatter.
§6 Q2 acked: embed fixture YAML strings in test files.

Address pins 1–2 inline during implementation (status.json update before first commit, proof.md divergence declarations as you go), then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: Two apply-inline mechanical pins (declare undeclared touchpoints in planned_files, pre-declare sync.Map divergence in proof.md); both ACs and §6 questions are fully satisfied; no authority decisions needed.
-->
