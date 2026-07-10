package orchestrator

// BlockedLaneSentinel is the substring embedded in every blocked-terminal
// error run.RunSlice returns — verifier BLOCKED verdicts, implementer
// StatusBlocked results, proof-absent/first-pass blocks, and terminal
// driver errors (auth/credits). The scheduler worker classifies a lane as
// BLOCKED (terminal for the run, replan required) by matching this
// substring, mirroring how InterpreterInconclusiveSentinel is consumed.
//
// It lives here — not in internal/run, whose private errVerdictBlockedPrefix
// aliases it with an identical value — because scheduler cannot import run
// (run imports scheduler) and both already import orchestrator (S14 D2).
const BlockedLaneSentinel = "RunSlice: verification blocked:"

// BlockedLaneRouteSuffix is the route directive RunSlice appends to the
// implementer-blocked terminal error so the AC-01 error is self-contained.
// The scheduler worker trims it from the reason it records via
// WorkerOptions.RecordBlocked, so the exit report does not render the
// directive twice (S14, Captain review flag (a)).
const BlockedLaneRouteSuffix = " — route: /replan-release (BLOCKED is terminal for this lane)"
