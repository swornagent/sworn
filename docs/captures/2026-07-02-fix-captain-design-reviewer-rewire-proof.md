# Proof bundle — fix captain-design-reviewer-rewire (2026-07-02)

Finding: verified slice S19-captain-split regressed to dark code (finding id
`captain-split-regressed-dark-design-reviewer`, CONFIRMED). The engine's
design-review dispatch sent the conflated `captain.md` ("You are the
**release-level orchestrator**"); `design-reviewer.md` was not in the go:embed
set and had zero Go consumers; `orchestrator-notes.md` falsely claimed
captain.md carries a split-notice header.

## Scope

Restore the delivered S19-captain-split decision: the design-review dispatch
runs under the design-reviewer identity. Embed `design-reviewer.md`, expose
`prompt.DesignReviewer()`, point `internal/captain.Review` at it, and correct
the false claim in `orchestrator-notes.md`.

## Files changed

`git diff --name-only 632d4f3` (this commit's files; the first three are the
prior fix in this chain):

```
cmd/sworn/doctor.go                                          (prior commit)
cmd/sworn/doctor_test.go                                     (prior commit)
docs/captures/2026-07-02-fix-doctor-fix-agents-splice-proof.md (prior commit)
internal/adopt/adopt.go                                      (prior commit)
internal/captain/review.go
internal/captain/review_test.go
internal/prompt/orchestrator-notes.md
internal/prompt/prompt.go
internal/prompt/prompt_test.go
```

## Test results

RED first (new test through `captain.Review`, the engine dispatch that owns
the design-review affordance, against pre-fix code):

```
--- FAIL: TestReviewDispatchesDesignReviewerPrompt
    review_test.go:289: system prompt missing design-reviewer identity; starts with: # Captain role
        You are the **Captain**. You are the **release-level orchestrator** — ...
    review_test.go:292: system prompt still carries the conflated release-orchestrator identity (S19 regression)
```

GREEN after fix:

```
$ go test -timeout 120s ./internal/prompt/ ./internal/captain/ ./internal/run/...
ok  	github.com/swornagent/sworn/internal/prompt	0.011s
ok  	github.com/swornagent/sworn/internal/captain	0.016s
ok  	github.com/swornagent/sworn/internal/run	4.668s

--- PASS: TestReviewDispatchesDesignReviewerPrompt (0.00s)
--- PASS: TestDesignReviewer_Identity (0.00s)
```

`go vet ./internal/prompt/ ./internal/captain/` clean; touched files `gofmt`'d
(prompt.go had pre-existing fused-newline formatting; gofmt'd as part of the
touch — whitespace only).

## Reachability artefact

The prompt is now IN the shipped binary and has a production consumer:

```
$ go build -buildvcs=false -o bin/sworn ./cmd/sworn
$ grep -ac 'You are the \*\*Design Reviewer\*\*' bin/sworn
1                                    # pre-fix: 0 (not embedded — dark code)

$ grep -rn "DesignReviewer()" --include='*.go' . | grep -v _test
internal/captain/review.go:62:	systemPrompt := prompt.DesignReviewer()
internal/prompt/prompt.go:81:func DesignReviewer() string { return designReviewer }
```

Dispatch path: `internal/run/slice.go` design-review branch → `captain.Review`
→ `prompt.DesignReviewer()`; `TestReviewDispatchesDesignReviewerPrompt`
captures the dispatched messages and asserts the system prompt carries the
design-reviewer identity and NOT "release-level orchestrator".

Re-run: `go test -run TestReviewDispatchesDesignReviewerPrompt ./internal/captain/ -v`
and `grep -ac 'You are the \*\*Design Reviewer\*\*' bin/sworn` (expect 1).

## Delivered

- `design-reviewer.md` added to the `go:embed` set; `prompt.DesignReviewer()`
  exposed (test: `TestDesignReviewer_Identity`; also added to
  `TestEmbeddedPromptsPublicSafe`).
- Engine design-review dispatch rewired: `internal/captain/review.go` uses
  `prompt.DesignReviewer()` instead of the conflated `prompt.Captain()`
  (test: `TestReviewDispatchesDesignReviewerPrompt`).
- `orchestrator-notes.md` false claim corrected: it now states captain.md is
  vendored verbatim upstream (no split header; parity re-vendors clobber local
  headers) and that the engine dispatches `prompt.DesignReviewer()`.
- Re-vendor-proof: the fix survives future captain.md parity re-vendors
  (e.g. the pending v0.7.0 re-vendor, sworn#48) because the dispatch no longer
  depends on captain.md content.

## Not delivered

- `orchestrator-notes.md` is NOT added to the embed — it is a reference doc
  for implementers, not a dispatched prompt; S19's spec called it "not a
  prompt". No consumer needed.
- No captain.md split-notice header restored. Why: captain.md is re-vendored
  byte-identical from upstream Baton (which has no split), so a local header
  is structurally clobbered on every parity re-vendor — the durable fix is the
  dispatch rewire landed here. Tracking: upstreaming the split to baton
  role-prompts belongs to the vendor path / audit punch list
  (Refs swornagent/sworn#51, relates to sworn#48). Acknowledged in plain text
  per Rule 2.
- `prompt.Captain()` retained: still the vendored captain artefact (served for
  MCP/backward compatibility per S19 "callers must not break").

## Divergence from plan

- The fix guidance offered embedding orchestrator-notes.md "if the finding
  shows it was meant to be embedded" — the finding and S19 spec show it is a
  reference doc, so it stays unembedded; its false sentence was corrected
  instead.
