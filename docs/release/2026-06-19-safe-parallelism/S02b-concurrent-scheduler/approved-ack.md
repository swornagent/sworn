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
