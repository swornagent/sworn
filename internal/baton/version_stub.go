package baton

// SetVersionForTest sets the version returned by Version() for testing.
// This is the only way to inject a version string for testing the
// doctor check failure modes.
func SetVersionForTest(v string) {
	versionForTest = v
}

// SetUpstreamPinForTest overrides the upstream pin for testing.
func SetUpstreamPinForTest(pin *UpstreamPin) { upstreamPinForTest = pin }

// ClearUpstreamPinForTest restores the default (embedded) upstream pin.
func ClearUpstreamPinForTest() { upstreamPinForTest = nil }