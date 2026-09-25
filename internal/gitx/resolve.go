package gitx

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"
)

// Revision resolution codes for the sanitized commit resolver.
//
//   - AMBIGUOUS_REVISION: the revision matches more than one object.
//   - REVISION_NOT_FOUND: the revision does not resolve to exactly one commit.
//   - NON_COMMIT_OBJECT: the revision resolves but not to a commit (existing code, reused).
//   - GIT_EXECUTION_FAILED: the resolution transport failed (existing code, reused).
//
// All resolver failures use the fixed operation "resolve revision" and a
// fixed, value-free reason. The caller never sees the git arguments or the
// raw git stderr that distinguished ambiguous from not-found.
const (
	maxRevisionLength = 256
	resolveRevisionOp = "resolve revision"
)

// ResolveCommitRevision resolves any revision Git resolves to exactly one
// commit (a full id, an unambiguous abbreviation or a ref name, including
// the wider Git revision syntax such as HEAD~1 where it resolves to exactly
// one commit) to its full commit OID.
//
// Sanitization rejects empty input, over-length input, NUL or control
// characters, invalid UTF-8 and a leading '-' before Git is invoked, failing
// closed with REVISION_NOT_FOUND. Exactly one argument, rev+"^{commit}", is
// passed after --verify, so option injection is impossible and only commits
// resolve: blob, tree and other non-commit objects fail with
// NON_COMMIT_OBJECT where they can be told apart, otherwise with
// REVISION_NOT_FOUND. Transport failures (deadline, overflow, unquiesced
// process group, cleanup) fail with GIT_EXECUTION_FAILED. No error echoes
// the revision value, a path, or raw git output.
func (r *Repository) ResolveCommitRevision(rev string) (OID, error) {
	unresolvable := func() (OID, error) {
		return OID{}, fail(
			"REVISION_NOT_FOUND",
			resolveRevisionOp,
			errors.New("revision does not resolve to exactly one commit"),
		)
	}
	if r == nil {
		return unresolvable()
	}
	if rev == "" || len(rev) > maxRevisionLength || len(strings.TrimSpace(rev)) == 0 {
		return unresolvable()
	}
	if strings.HasPrefix(rev, "-") {
		return unresolvable()
	}
	if !utf8.ValidString(rev) {
		return unresolvable()
	}
	for _, c := range rev {
		if c <= 0x1f || (c >= 0x7f && c <= 0x9f) {
			return unresolvable()
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), CommandTimeout)
	defer cancel()
	outcome := r.runOutcome(
		ctx,
		"",
		0,
		nil,
		nil,
		"/dev/null",
		MaxDiagnostic,
		"rev-parse",
		"--verify",
		rev+"^{commit}",
	)
	if outcome.successful() && outcome.cleanupErr == nil {
		oid, err := r.parseOID(string(outcome.stdout))
		if err != nil {
			return OID{}, fail(
				"GIT_EXECUTION_FAILED",
				resolveRevisionOp,
				errors.New("revision resolution failed"),
			)
		}
		return oid, nil
	}
	if outcome.timedOut || outcome.overflow || !outcome.groupQuiet || !outcome.reaped || outcome.cleanupErr != nil {
		return OID{}, fail(
			"GIT_EXECUTION_FAILED",
			resolveRevisionOp,
			errors.New("revision resolution failed"),
		)
	}
	lower := strings.ToLower(string(outcome.stderr))
	if strings.Contains(lower, "ambiguous") {
		return OID{}, fail(
			"AMBIGUOUS_REVISION",
			resolveRevisionOp,
			errors.New("revision is ambiguous"),
		)
	}
	if strings.Contains(lower, "expected commit type") {
		return OID{}, fail(
			"NON_COMMIT_OBJECT",
			resolveRevisionOp,
			errors.New("revision does not resolve to a commit"),
		)
	}
	return unresolvable()
}
