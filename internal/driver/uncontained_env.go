package driver

// These environment names are read by platform-neutral code (fake.go's
// guest-path resolution) and set by the linux dispatch path, so they live in
// a platform-neutral file: declaring them next to the linux-only dispatch
// broke every non-linux build (fake.go compiles everywhere).
const (
	// testUncontainedDispatchEnv is the sole environment signal for the
	// test-only uncontained dispatch mode. It is a request, never a route: a
	// binary that did not link the gate refuses it in platformInvoke before
	// any driver or sandbox interaction, so the environment value alone can
	// never enable the uncontained branch.
	testUncontainedDispatchEnv = "SWORN_TEST_UNCONTAINED_DISPATCH"
	// testUncontainedGuestWorkspaceEnv and testUncontainedGuestInputsEnv are
	// the engine-set guest-path overrides that let a fake driver resolve the
	// guest paths (/workspace and /sworn/inputs) it cannot otherwise see in an
	// uncontained dispatch. They exist only in the controlled environment the
	// engine builds for the gate-linked uncontained branch; the contained
	// branch's --clearenv plus fixed --setenv list never carries them.
	testUncontainedGuestWorkspaceEnv = "SWORN_TEST_GUEST_WORKSPACE"
	testUncontainedGuestInputsEnv    = "SWORN_TEST_GUEST_INPUTS"
)
