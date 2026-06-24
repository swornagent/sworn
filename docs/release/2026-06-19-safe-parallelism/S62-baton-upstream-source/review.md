# Captain review — S62-baton-upstream-source
Date: 2026-07-09
Design commit: fbb69bf

## Pins

1. **[mechanical] §2b — design_decisions missing from status.json (7th recurrence).**
   What I observed: `status.json` has no `design_decisions` field. The design has 5 decisions in §2, all Type-2 (local, reversible, narrow). The designfit gate fails closed on a missing `design_decisions` array.
   What to ask the implementer: Add a `design_decisions` array to `status.json` with all 5 §2 decisions, each classified `Type-2` with a one-line summary. This is the 7th time this has surfaced in the trial log — fix before code.

2. **[mechanical] §3 — planned_files mismatch with design file list.**
   What I observed: `status.json` `planned_files` lists `internal/baton/source.go` but design §3 does not touch it (Decision 5: "No SourceProvider interface" — `FetchUpstream` lives in `fetch.go`). Conversely, design §3 lists `internal/baton/version.go` (add `WriteUpstreamPin`) but `planned_files` omits it.
   What to ask the implementer: Remove `internal/baton/source.go` from `planned_files` (design chooses not to modify it), add `internal/baton/version.go` (design adds `WriteUpstreamPin` there). Reconcile before code so Gate 2 (touchpoint audit) passes.

3. **[mechanical] §2.3 — commit SHA resolution mechanism is ambiguous and likely incorrect.**
   What I observed: Design §2 Decision 3 says the resolved commit SHA comes "from the GitHub API tag resolution or `X-GitHub-Commit` header if available." But `codeload.github.com` (the tarball endpoint) does not return a commit SHA in any response header. The commit SHA requires a separate API call to `api.github.com/repos/{owner}/{repo}/git/refs/tags/{tag}`. For annotated tags (v0.4.2 is annotated — tag object SHA 986ce82 points to commit 729f188), the ref response returns the tag object SHA, not the commit SHA; the implementer must dereference via the tag object endpoint or use `api.github.com/repos/{owner}/{repo}/commits/{tag}` which returns the commit directly. The design describes a single GET (codeload) but the SHA resolution requires a second HTTP call the design doesn't mention.
   What to ask the implementer: Clarify the SHA resolution path: (a) make a separate `net/http` call to `api.github.com` to resolve tag → commit SHA; (b) handle annotated tags by dereferencing the tag object to the commit; (c) update `FetchUpstream` to make two HTTP calls (API resolve + codeload fetch), or split into a resolve step + fetch step. The test fixtures must mock both endpoints. This is the core security feature — if SHA resolution doesn't work, the version lock is non-functional.

4. **[mechanical] §2.3 — bootstrap case: first fetch has no `upstream-digest` in VERSION.**
   What I observed: Current VERSION has `upstream-sha: 729f188...` but no `upstream-digest:` line. Design §3 says VERSION "receives `upstream-digest:` line after first upstream fetch (write-back, not hand-edited)." Design §1 says "verifies the resolved commit SHA and content digest against the VERSION pin" — but on first fetch, there is no digest pin to verify against. The design doesn't address this bootstrap case.
   What to ask the implementer: State the first-fetch behaviour explicitly: when `upstream-digest` is absent from VERSION, skip digest verification (SHA verification still catches force-moved tags), compute the digest from the fetched tarball, and write it to VERSION after successful Vendor. On subsequent fetches, verify both SHA and digest. Add a test case for the first-fetch (no digest pin) path.

5. **[memory-cited] §2 — design aligns with [[project_baton_sworn_architecture]].**
   What I observed: Memory codifies "stdlib HTTPS tarball (codeload tar.gz → net/http+gzip+tar; no git, no dep, no ADR), version-locked by tag + commit-SHA/content-digest, fail-closed." Design §2 and §3 implement exactly this: stdlib only, tarball over HTTPS, SHA+digest lock, fail-closed.
   Citation: [[project_baton_sworn_architecture]]

6. **[memory-cited] §2 — design aligns with [[project_dep_policy]].**
   What I observed: Memory codifies "minimal, justified deps — each new dependency requires an ADR entry." Design uses stdlib only (`net/http`, `compress/gzip`, `archive/tar`, `crypto/sha256`), no new deps, no ADR needed. Consistent with the policy.
   Citation: [[project_dep_policy]]

## Summary

Pins: 6 total — 4 [mechanical], 2 [memory-cited], 0 [escalate]
Critical pins: #3 (SHA resolution mechanism — if incorrect, version lock is non-functional)

## Smaller flags (not pins, worth one-line ack)

- Design §1 and §2 Decision 4 mention `--repo` override but not config-based override. The spec AC says "`--repo`/config override is honoured." The implementer should check whether `sworn init` config has a repo field and wire it as a fallback for `--repo`. Minor — likely a one-line config read.
- Design §2 Decision 5 function signature `FetchUpstream(ctx context.Context, repo, tag string)` — `repo` is presumably `owner/name` format (e.g. `sawy3r/baton`). Confirm the codeload URL is constructed as `codeload.github.com/{repo}/tar.gz/refs/tags/{tag}` and the API URL as `api.github.com/repos/{repo}/...`.

## Suggested ack reply

TL;DR sound design, stdlib-only network fetch with version lock — 4 pins + 2 flags, all mechanical:

1. **Add design_decisions to status.json.** Add a `design_decisions` array with all 5 §2 decisions, each `Type-2` with a one-line summary. 7th recurrence — fix before code.
2. **Reconcile planned_files.** Remove `internal/baton/source.go` from `planned_files` (design doesn't modify it — Decision 5 puts `FetchUpstream` in `fetch.go`). Add `internal/baton/version.go` (design adds `WriteUpstreamPin` there).
3. **Clarify commit SHA resolution.** Codeload doesn't return SHA in headers. Make a separate `net/http` call to `api.github.com/repos/{owner}/{repo}/commits/{tag}` (returns commit directly, handles annotated tags). Update `FetchUpstream` to make two HTTP calls (API resolve + codeload fetch). Test fixtures must mock both endpoints. This is the core security feature — get it right.
4. **Handle first-fetch bootstrap.** When `upstream-digest` is absent from VERSION (first fetch), skip digest verification (SHA still catches force-moved tags), compute digest, write it after successful Vendor. Add a test case for the no-digest-pin path.

Flags (not pins): (a) wire config-based repo override as fallback for `--repo` if `sworn init` config has a repo field; (b) confirm `repo` param is `owner/name` format for URL construction.

§2 decisions 1–5 ack. §6 empty — ack. Pins 5–6 memory-cited: [[project_baton_sworn_architecture]] and [[project_dep_policy]] both align.

Address pins 1–4 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: All 4 mechanical pins are apply-inline corrections (status.json fix, planned_files reconciliation, SHA resolution mechanism clarification, bootstrap case handling) — no design re-review needed; Verifier backstops.
-->