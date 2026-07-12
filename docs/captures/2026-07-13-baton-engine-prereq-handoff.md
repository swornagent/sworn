# Handoff — Baton engine prerequisite + LLM-check prompts (Phase C, sworn side)

Date: 2026-07-13
Baton PR: [sawy3r/baton#68](https://github.com/sawy3r/baton/pull/68) (`feat/publish-llm-checks-and-engine-prereq`, v0.12.0)
Sworn issue: [swornagent/sworn#100](https://github.com/swornagent/sworn/issues/100) (fixed, `ca6fcc3`)

## Origin

Dogfooding a fresh Baton install into a downstream project (`fired`, via Codex) surfaced
two independent problems that had been conflated:

1. `sworn board` failed with "path does not exist in HEAD" on a board.json-native
   release. **This was a sworn bug, not a missing engine.** Fixed — see below.
2. Baton's README/INSTALL claimed a zero-binary by-hand loop that **does not exist**. The
   seven slash commands all BLOCK without an engine, and the six LLM checks were named in
   Baton but their prompt bodies shipped only inside sworn — so there was no by-hand
   fallback even in principle, and no second engine could be conformant.

## Phase A — DONE (sworn, `ca6fcc3`)

Board oracle release-ref probe. It validated the `release-wt` ref by probing for
`index.md`; per ADR-0009 the authoritative record is `board.json` and `index.md` is a
rendered view a records-as-JSON release need not carry. Two stacked defects: a **wrong
probe**, and a **silent fallback** that retargeted to HEAD and then reported the failure
against HEAD.

Duplicated at three sites (`cmd/sworn/board.go`, `cmd/sworn/route.go`,
`internal/tui/board.go`); all three now call `board.ReleaseRefFor`. `route.go` is the
loop's router, so this broke `sworn run` on any board.json-native release.

Proof: `docs/captures/2026-07-13-board-release-ref-probe-proof.md`.

## Phase B — DONE, awaiting Coach merge (baton PR #68)

Published `baton/llm-checks/` (six prompt bodies as spec) +
`schemas/llm-check-report-v1.json`. Re-cut README/INSTALL/ROADMAP/AGENTS-fragment around
two honest tiers. Installer preflight for an engine. Split the BLOCKED taxonomy in all six
commands.

**Hold point: Brad reviews, merges, and cuts the `v0.12.0` tag.** Publishing the check
prompts changes Baton's public conformance surface — a Type-1, Coach-owned call under
Rule 9.

## Phase C — BLOCKED on the v0.12.0 tag (sworn side)

Sworn pins Baton by semver tag (`internal/adopt/baton/VERSION`, currently
`baton-protocol: v0.11.0`, sha `9f50909`). The vendored copy is a pinned tag, not a
tracking submodule, and `sworn doctor` inspects the pin's digest — so the vendored copy
**cannot** be edited directly. Phase C is strictly downstream of the tag.

Three tasks once `v0.12.0` exists:

1. **Re-vendor to v0.12.0.** Bump BOTH embed roots — `internal/adopt/baton/` *and*
   `internal/prompt/baton/` (they have drifted apart before; see the FT-6 note in
   project memory). Update `VERSION`'s `upstream-sha` / `upstream-digest`.

2. **Load the check prompts from the vendored Baton root.** `internal/gate/llmcheck.go`
   currently holds all six as inline Go string constants in a `systemPrompts` map
   (~140 lines, lines 84–222). Replace with a load from the embedded
   `baton/llm-checks/<name>.md`, stripping YAML frontmatter (the body IS the prompt,
   verbatim — that is the published contract, so do not reformat it). Precedent:
   `internal/prompt/` already embeds role prompts as whole `.md` files.

3. **De-hardcode `userPromptHeader`** (`internal/gate/llmcheck.go:225`). It currently
   reads:

   ```go
   const userPromptHeader = `You are evaluating a slice in a release of the SwornAgent project (a Go CLI).
   ```

   **This is a live correctness bug, not just a packaging wart.** Running
   `sworn llm-check` in any non-Go project tells the model it is grading a Go CLI. Baton
   v0.12.0 specifies `{{project_context}}` as a **required** substitution supplied by the
   engine from repo config. Wire it to the project's `sworn init` configuration.

   Also consider emitting `llm-check-report-v1` (validated) rather than the current ad-hoc
   report struct.

## Known wart, tracked not hidden (Rule 2)

The six checks use two severity vocabularies: five grade `FAIL/WARN/INFO`,
`security-review` grades `critical/high/medium/low`. `llm-check-report-v1` accepts both
rather than silently redefining a prompt's contract. Reconciling them is a behaviour
change that deserves its own decision. *Tracking:* baton PR #68 body. *Acknowledgement:*
raised with the Coach in the session wrap-up.

## Not done, deliberately

**baton-web site copy** (`~/projects/baton-web`) still carries the pre-v0.12.0 framing and
will restate the "zero-binary by-hand loop" claim. *Why:* out of scope for the two repos
in flight; the site should follow the merged protocol wording, not lead it. *Tracking:*
this capture + the session wrap-up. *Acknowledgement:* raised with the Coach.
