//go:build linux

package driver

import "testing"

// TestNativeDispatchStampsExecutedDigestOnSuccessAndPostClosureFailure pins
// A2: the executed binary's digest reaches the durable Observation.Usage on
// both completion classes of a real native dispatch - a successful turn and
// a post-closure failure that sanitizeFailedObservation otherwise flattens.
func TestNativeDispatchStampsExecutedDigestOnSuccessAndPostClosureFailure(t *testing.T) {
	nativeBinary := w8BuildNativeFixture(t)
	expectedDigest, err := executableDigest(nativeBinary)
	if err != nil {
		t.Fatal(err)
	}
	target := w8NewNativeTarget(t, "claude", ProfileClaude, nativeBinary)

	_, observation, err := target.invoke(
		t, "a2-success", RolePlanner, PlannerProposal, "",
		ReadWrite, "a2-success",
	)
	if err != nil {
		t.Fatalf("expected a successful dispatch, got %v", err)
	}
	if observation.Usage.ExecutedDigest == nil ||
		*observation.Usage.ExecutedDigest != expectedDigest {
		t.Fatalf(
			"success executed digest = %#v, want %s",
			observation.Usage.ExecutedDigest, expectedDigest,
		)
	}

	_, failed, err := target.invoke(
		t, "p06-malformed", RolePlanner, PlannerProposal, "",
		ReadWrite, "p06-malformed",
	)
	if err == nil {
		t.Fatalf("expected a post-closure failure, observation=%#v", failed)
	}
	if failed.Usage.ExecutedDigest == nil ||
		*failed.Usage.ExecutedDigest != expectedDigest {
		t.Fatalf(
			"post-closure failure executed digest = %#v, want %s",
			failed.Usage.ExecutedDigest, expectedDigest,
		)
	}
	if failed.Usage.TokenStatus != UsageUnavailable {
		t.Fatalf("sanitized failure usage unexpectedly changed: %#v", failed.Usage)
	}
}
