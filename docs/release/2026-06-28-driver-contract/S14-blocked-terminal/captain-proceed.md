# Coach acknowledgement — S14-blocked-terminal

Date: 2026-07-11
Decided by: Brad (Coach) — Captain verdict PROCEED, zero escalate pins;
all pins mechanical/memory-cited, applied per the pre-authorised batch
protocol.
Review: review.md (Captain, verdict PROCEED)
Verdict: PROCEED — dispositions below

## Pin dispositions

1. **(CRITICAL) supervisor state coercion — ACCEPTED.** Do NOT call
   releaseTrack("blocked"): supervisor.Release coerces unknown states
   to "done" (supervisor.go:205-207), which would record a blocked
   track as complete. Use supervisor.StateFailed as the persisted
   supervisor state; the blocked-vs-failed distinction travels via the
   RecordBlocked side-channel so the exit report still separates
   BLOCKED lanes from FAIL lanes.

2. **design_decisions — ACCEPTED.** Record D1-D6 in
   status.json.design_decisions at the in_progress transition, matching
   every landed sibling's record shape (Rule 9 gate fails closed on an
   empty record).

3. **Terminal driver errors join the BLOCKED report section —
   ACCEPTED AS DESIGNED.** An auth/credits terminal halt is
   blocked-class by nature (re-dispatch cannot help); no reason-string
   sniffing to carve them out. The report's verbatim-blocker line
   carries the terminal error text.

4. Any remaining pin in review.md not restated above is accepted as
   written in the Captain's suggested acknowledgement reply.

Proceed to implementation.
