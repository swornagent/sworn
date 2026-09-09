package gitx

import "os"

// Recovery cut points reachable from inside this package (before a caller in
// internal/runtime regains control to check its own gate) mirror
// internal/runtime's testHooksFromEnv/testCrashAfterEffect pattern exactly,
// rather than importing internal/runtime: a production binary links neither
// var, so it carries no runtime crash knob, and both packages read the
// identical SWORN_TEST_CRASH_AFTER_EFFECT contract independently once a test
// binary links testHooksFromEnv=1 in both places.
var (
	testHooksFromEnv     string
	testCrashAfterEffect string
)

func init() {
	if testHooksFromEnv != "1" {
		return
	}
	if value := os.Getenv("SWORN_TEST_CRASH_AFTER_EFFECT"); value != "" {
		testCrashAfterEffect = value
	}
}
