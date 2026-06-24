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
