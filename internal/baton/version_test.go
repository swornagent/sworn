package baton

import (
	"strings"
	"testing"
	"time"
)

func TestIsSemverTag(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		// Valid semver tags
		{"v0.3.0", true},
		{"v2.0.0", true},
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

func TestUpstreamPinReplacementUsesCapturedInstant(t *testing.T) {
	existing := []byte("baton-protocol: v0.13.1\nvendored: 2026-07-14\nupstream-sha: old\nupstream-digest: sha256:old\n")
	captured := time.Date(2026, 7, 16, 23, 59, 59, 0, time.FixedZone("AEST", 10*60*60))
	candidate := UpstreamVersionCandidate{
		Tag:        "v0.15.1",
		SHA:        "3fb4d275ae8a151f6287e7b9279d71628b12eea0",
		Digest:     "sha256:8f0839ea897374eb10d6db2a789939714727739621babef1117d74cbf4488d2f",
		CapturedAt: captured,
	}

	got, err := UpstreamPinReplacement(existing, candidate)
	if err != nil {
		t.Fatal(err)
	}
	wantLines := []string{
		"baton-protocol: v0.15.1",
		"vendored: 2026-07-16",
		"upstream-sha: 3fb4d275ae8a151f6287e7b9279d71628b12eea0",
		"upstream-digest: sha256:8f0839ea897374eb10d6db2a789939714727739621babef1117d74cbf4488d2f",
	}
	for _, line := range wantLines {
		if !strings.Contains(string(got), line) {
			t.Errorf("VERSION candidate missing %q:\n%s", line, got)
		}
	}
	if string(existing) != "baton-protocol: v0.13.1\nvendored: 2026-07-14\nupstream-sha: old\nupstream-digest: sha256:old\n" {
		t.Fatal("pure VERSION construction mutated its input")
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
