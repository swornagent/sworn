package baton

import (
	"testing"
)

func TestIsSemverTag(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		// Valid semver tags
		{"v0.3.0", true},
		{"v1.0.0", true},
		{"v0.0.0", true},
		{"v10.20.30", true},

		// SHA-style (should reject)
		{"cf158423f65c20860a3d4ec0310acb6cc7fb5aa0", false},

		// Missing v prefix
		{"0.3.0", false},
		{"1.0.0", false},

		// Empty or short
		{"", false},
		{"v", false},
		{"v1", false},
		{"v1.0", false},

		// Pre-release / build suffixes
		{"v1.0.0-alpha", false},
		{"v1.0.0+sha.abc123", false},

		// Leading zeros in components
		{"v01.0.0", false},
		{"v1.01.0", false},
		{"v1.0.01", false},

		// Non-numeric components
		{"vX.Y.Z", false},
		{"v1.0.zero", false},
	}
	for _, tt := range tests {
		got := IsSemverTag(tt.input)
		if got != tt.want {
			t.Errorf("IsSemverTag(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

// TestUpstreamPinComplete enforces that the single Baton version source of truth
// (internal/adopt/baton/VERSION) carries a COMPLETE pin: a semver protocol tag,
// an upstream-sha, and an upstream-digest. A shipped binary must trace its
// vendored bytes to an exact upstream commit; an incomplete pin is the drift
// that let three contradictory version files (v0.4.2 / v0.5.0 / v1.0.0) ship
// unnoticed before this was the single source of truth.
func TestUpstreamPinComplete(t *testing.T) {
	pin, err := ReadUpstreamPin()
	if err != nil {
		t.Fatalf("ReadUpstreamPin() error: %v", err)
	}
	if pin.SHA == "" {
		t.Error("upstream-sha is empty in internal/adopt/baton/VERSION — pin incomplete")
	}
	if pin.Digest == "" {
		t.Error("upstream-digest is empty in internal/adopt/baton/VERSION — pin incomplete")
	}
	if v := Version(); !IsSemverTag(v) {
		t.Errorf("baton-protocol = %q — not a semver tag", v)
	}
}

func TestVersionIsSemverNotSha(t *testing.T) {
	// Version() reads from the adopt embed which should contain a semver tag
	// after S49 reconciliation. The test verifies the live embed.
	v := Version()
	if v == "" {
		t.Fatal("Version() returned empty string — embed may be missing baton/VERSION")
	}
	if !IsSemverTag(v) {
		t.Errorf("Version() = %q — not a semver tag; it may still be a SHA", v)
	}
}
