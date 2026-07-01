package orchestrator

import "fmt"

// NOTE (ADR-0011 keystone, Step 3): the stateless prose-classifier that used to
// live here — Interpret / parseInterpretResult / firstInterpretLine and its
// interpreterSystemPrompt — was DEAD CODE (defined and unit-tested but never
// wired into any production path) AND a HasPrefix prose scrape of the kind
// ADR-0009 forbids. It was deleted with the verifier-verdict-v1 pilot: the
// agentic verifier now EMITS a schema-constrained verdict
// (internal/verify.RunAgentic → verifier-verdict-v1) instead of replying in
// prose for a downstream classifier to scrape.
//
// The INCONCLUSIVE→PAGE signalling contract below SURVIVES: it is a live
// cross-package contract consumed by the scheduler worker (internal/scheduler/
// worker.go) to detect when the loop must PAGE the Coach.

// InterpreterInconclusiveSentinel is the substring embedded in an error message
// so callers (the scheduler worker) can detect an INCONCLUSIVE outcome with
// strings.Contains without importing the orchestrator package.
const InterpreterInconclusiveSentinel = "INTERPRETER_INCONCLUSIVE"

// ErrInterpretInconclusive builds the error the triage path returns when an
// outcome classifies as INCONCLUSIVE. The worker/router detect it via the
// InterpreterInconclusiveSentinel substring.
func ErrInterpretInconclusive(sliceID string, rawPreview string) error {
	preview := rawPreview
	if len(preview) > 100 {
		preview = preview[:97] + "..."
	}
	return fmt.Errorf("%s: interpreter could not classify output for %s (raw preview: %s)",
		InterpreterInconclusiveSentinel, sliceID, preview)
}
