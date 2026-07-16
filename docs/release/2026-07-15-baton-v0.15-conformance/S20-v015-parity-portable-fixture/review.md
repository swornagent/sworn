# Captain review — S20-v015-parity-portable-fixture

Date: 2026-07-17T04:22:55+10:00
Design commit: b95201f6608b9cf73f589266f88e39bfed94dc5b

## Pins

1. [mechanical] §Proposed implementation.8 — Preserve the public `baton diff` 0/1/2 contract through the built binary.
   What I observed: the live `cmdBatonDiff` converts every `baton.Diff` error into exit 1, while AC-02 requires deterministic drift to be 1 and malformed mapping, unreadable input, or other operational failures to be 2. The design promises a thin 0/1/2 adapter but does not name the error classification or an invalid-source binary reachability case.
   What to ask the implementer: add one bounded error classification at the adapter boundary and exercise the built binary against an isolated repository fixture: exact verified clone is 0, deterministic mapped-byte drift is 1, and malformed or missing source is 2 with no repository mutation.

2. [mechanical] §Proposed implementation.6 and .8 — Keep the production `doctor --sync-baton` path free of external-command dependence.
   What I observed: live `cmdDoctor` always reaches Group 4; this checkout has `docs/considerations.md`, so its `defaultCheckDepFreshness` runs `exec.Command("go", "list", "-m", "-u", "./...")` before the current sync. The normative boundary permits exact installer scripts and Git only in isolated proof, while the shipped installation path is stdlib-only and has no shell or external-command dependency. The design says “no runtime shell dependency” but does not fence this existing command from the sync path.
   What to ask the implementer: make the `--sync-baton` installation/recovery path bypass or replace external dependency freshness work, and prove the built binary completes its isolated sync with Go and a shell unavailable on `PATH` after test-only bundle/oracle setup.

3. [mechanical] §Proposed implementation.5, .6, and .8 — Make temporary-home isolation and restoration an asserted test guard.
   What I observed: the current doctor tests mutate process-wide CWD and `SWORN_BATON_HOME` directly and stage only one local mirror. S20 instead drives three logical roots, a recovery root, hostile umask, scripts, and a built binary; “temporary HOME” alone is not a restore or target assertion under Rule 11.
   What to ask the implementer: run each oracle and built-binary invocation as a child process with explicit `Cmd.Dir` and complete temporary `HOME`, `AGENTS_HOME`, `CODEX_HOME`, `CLAUDE_HOME`, and Sworn-config environment; use test cleanup for any unavoidable process-global mutation; assert every resolved target and recovery root is contained in the test root and pairwise disjoint before invoking sync; and make the reachability test fail if a real-home path can be selected.

4. [memory-cited] §Proposed implementation.2 — Retain byte-exact schema authority through the new ambiguity schema path.
   What I observed: the design adds the explicit mapping, embed, fixture, and grade-manifest entry for `spec-ambiguity-report-v1`, and retains byte-identical normative JSON. The prior Sworn parity repair found that a version/prompt bump can look complete while a schema remains stale or receives prose-style transformation.
   What to ask the implementer: acknowledge that the source map, `SchemaMap`, fixture, and manifest are checked as one complete set and that the new schema is copied and compared byte-for-byte, never passed through Markdown transformations.
   Citation: [[Sworn Baton v0.13.1 prerequisite upgrade and parity verification]]

## Summary

Pins: 4 total — 3 [mechanical], 1 [memory-cited], 0 [escalate]
Critical pins: 1, 2, 3

## Smaller flags (not pins, worth one-line acknowledgement)

- The 78-entry archive plan already binds its literal identity, prefix, path/mode/blob inventory, hostile extraction checks, and participation in the existing vendor transaction; keep that inventory comparison direct against the verified temporary clone rather than an aggregate count alone.
- Every live sibling is verified, deferred, or planned; none is `in_progress` or `implemented`, and `board.json` declares no shared touchpoints. S20 may proceed within its declared T1 surface while stopping on any newly discovered cross-track file.
- The design-review LLM check passed with the current CLI spelling: `sworn llm-check --type design-review --release 2026-07-15-baton-v0.15-conformance --slice S20-v015-parity-portable-fixture`.

## Suggested acknowledgement reply

TL;DR The portable bundle, exact parity, and whole-root recovery design is sound. 4 pins + 3 flags:

1. **Preserve public diff exits.** Add a bounded adapter classification and built-binary reachability coverage proving verified clone = 0, deterministic drift = 1, and malformed or missing source = 2 without mutation.
2. **Keep sync stdlib-only.** Ensure `doctor --sync-baton` does not run dependency-freshness or another external command; after test-only bundle/oracle setup, prove the built binary syncs with Go and a shell unavailable on `PATH`.
3. **Make proof homes hermetic.** Launch oracle and binary processes with explicit temporary HOME, AGENTS_HOME, CODEX_HOME, CLAUDE_HOME, and Sworn-config roots; guard containment/disjointness before sync and use cleanup for every process-global mutation.
4. **Keep schema authority exact.** Treat the ambiguity schema mapping, embed, fixture, and manifest as one complete byte-exact set, with no prose transform.

Flags (not pins): (a) compare the full archive inventory directly to the verified clone, not just its count; (b) no live sibling collision exists today; (c) the design-review LLM check passed.

§2 decisions, including the recorded test-only bundle and opaque maintainability boundary, are acknowledged. §6 has no open questions.

Address pins 1–4 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: The remaining gaps are bounded, mechanically verifiable reachability and isolation guards; no scope, product, or architectural choice needs a Coach decision.
-->
