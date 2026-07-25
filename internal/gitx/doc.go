// Package gitx provides Baton-agnostic, literal Git mechanics.
//
// The package deliberately has no knowledge of plans, approvals, work,
// lifecycle transitions, evidence, or authority. Callers provide closed typed
// requests; gitx executes an explicitly admitted Git binary with a sanitized
// environment and returns captured object facts or immutable prepared objects.
package gitx
