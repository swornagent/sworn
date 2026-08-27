package driver

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// fabricatedRuntimeFiles builds format-valid PinnedRuntimeFile entries.
// validateNativeConfig's validatePinnedRuntimeFiles never stats these paths
// (only openNativeClosure does, at dispatch time), so these fixtures are
// deliberately host-independent.
func fabricatedRuntimeFiles() ([]PinnedRuntimeFile, []string) {
	fakeDigest := "sha256:" + string(bytes.Repeat([]byte("a"), 64))
	targets := []string{
		"/etc/ssl/certs/ca-certificates.crt",
		"/etc/resolv.conf",
		"/etc/hosts",
		"/etc/nsswitch.conf",
	}
	files := make([]PinnedRuntimeFile, len(targets))
	for index, target := range targets {
		files[index] = PinnedRuntimeFile{
			Path: "/fixture" + target, Target: target, Digest: fakeDigest,
		}
	}
	return files, append([]string(nil), targets...)
}

// nativePinModeTestConfig builds an otherwise-valid Claude NativeAdapterConfig
// varying only the pin-admission-relevant fields. The CLI path is a real,
// executable file (validateExecutableIdentity stats it), but its digest is
// never required to match the file's actual bytes at this validation layer -
// that binding is enforced later, at dispatch, by openPinnedExecutable.
func nativePinModeTestConfig(
	t *testing.T,
	pinMode, cliVersion, cliDigest, credentialTarget, versionOutput string,
) NativeAdapterConfig {
	t.Helper()
	executable := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	files, required := fabricatedRuntimeFiles()
	return NativeAdapterConfig{
		Key: "a-claude", ID: "sworn.claude", Version: "1.0.0",
		Family:                 ProfileClaude,
		CLI:                    ExecutableIdentity{Path: executable, Digest: cliDigest},
		CLIVersion:             cliVersion,
		VersionOutput:          versionOutput,
		RuntimeFiles:           files,
		RequiredRuntimeTargets: required,
		CredentialTarget:       credentialTarget,
		CredentialRefs:         []string{"claude-file"},
		MaxCredentialBytes:     1_048_576,
		PinMode:                pinMode,
	}
}

func TestValidateNativeConfigPinModeAbsentAndExactPreserveTodaysFourChecks(t *testing.T) {
	for _, mode := range []string{"", NativePinModeExact} {
		config := nativePinModeTestConfig(
			t, mode, ClaudeCLIVersion, ClaudeCLIDigest,
			ClaudeCredentialTarget, ClaudeCLIVersion+" (Claude Code)",
		)
		if err := validateNativeConfig(config); err != nil {
			t.Fatalf("pin_mode %q: exact match rejected: %v", mode, err)
		}
		mismatched := config
		mismatched.CLIVersion = "9.9.9"
		if err := validateNativeConfig(mismatched); !IsCode(err, "NATIVE_NOT_CERTIFIED") {
			t.Fatalf("pin_mode %q: version mismatch admitted: %v", mode, err)
		}
	}
}

func TestValidateNativeConfigPinModeMinorAdmitsInRangeVersion(t *testing.T) {
	config := nativePinModeTestConfig(
		t, NativePinModeMinor, "2.1.999",
		"sha256:"+string(bytes.Repeat([]byte("b"), 64)),
		ClaudeCredentialTarget, "2.1.999 (Claude Code)",
	)
	if err := validateNativeConfig(config); err != nil {
		t.Fatalf("in-range minor-mode version rejected: %v", err)
	}
}

func TestValidateNativeConfigPinModeMinorRejectsOutOfRangeVersion(t *testing.T) {
	for _, version := range []string{"2.2.0", "3.1.5"} {
		config := nativePinModeTestConfig(
			t, NativePinModeMinor, version,
			"sha256:"+string(bytes.Repeat([]byte("b"), 64)),
			ClaudeCredentialTarget, version+" (Claude Code)",
		)
		if err := validateNativeConfig(config); !IsCode(err, "NATIVE_NOT_CERTIFIED") {
			t.Fatalf("out-of-range version %q admitted: %v", version, err)
		}
	}
}

func TestValidateNativeConfigPinModeMinorKeepsCredentialTargetExact(t *testing.T) {
	config := nativePinModeTestConfig(
		t, NativePinModeMinor, "2.1.999",
		"sha256:"+string(bytes.Repeat([]byte("b"), 64)),
		"/home/sworn/.claude/wrong-target.json", "2.1.999 (Claude Code)",
	)
	if err := validateNativeConfig(config); !IsCode(err, "NATIVE_NOT_CERTIFIED") {
		t.Fatalf("minor mode admitted a wrong credential target: %v", err)
	}
}

func TestValidateNativeConfigPinModeMinorRequiresSelfConsistentVersionOutput(t *testing.T) {
	config := nativePinModeTestConfig(
		t, NativePinModeMinor, "2.1.999",
		"sha256:"+string(bytes.Repeat([]byte("b"), 64)),
		ClaudeCredentialTarget, ClaudeCLIVersion+" (Claude Code)",
	)
	if err := validateNativeConfig(config); !IsCode(err, "NATIVE_NOT_CERTIFIED") {
		t.Fatalf("minor mode admitted a version_output that names the pinned constant, not the declared version: %v", err)
	}
}

func TestValidateNativeConfigRejectsUnknownPinMode(t *testing.T) {
	config := nativePinModeTestConfig(
		t, "bogus", ClaudeCLIVersion, ClaudeCLIDigest,
		ClaudeCredentialTarget, ClaudeCLIVersion+" (Claude Code)",
	)
	if err := validateNativeConfig(config); !IsCode(err, "NATIVE_NOT_CERTIFIED") {
		t.Fatalf("unknown pin_mode admitted: %v", err)
	}
}

func TestNativeAdapterConfigPinModeOmitemptyKeepsCanonicalBytesUnchanged(t *testing.T) {
	config := nativePinModeTestConfig(
		t, "", ClaudeCLIVersion, ClaudeCLIDigest,
		ClaudeCredentialTarget, ClaudeCLIVersion+" (Claude Code)",
	)
	body, err := canonicalJSON(config)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(body, []byte(`"pin_mode"`)) {
		t.Fatalf("omitempty PinMode leaked into canonical bytes: %s", body)
	}
}

func TestNativeVersionSatisfiesMinorRangeIsPatchOnly(t *testing.T) {
	cases := []struct {
		declared string
		want     bool
	}{
		{"2.1.241", true},
		{"2.1.0", true},
		{"2.1.999", true},
		{"2.2.0", false},
		{"2.0.999", false},
		{"3.1.241", false},
	}
	for _, testCase := range cases {
		if got := nativeVersionSatisfiesMinor(testCase.declared, ClaudeCLIVersion); got != testCase.want {
			t.Fatalf(
				"nativeVersionSatisfiesMinor(%q, %q) = %v, want %v",
				testCase.declared, ClaudeCLIVersion, got, testCase.want,
			)
		}
	}
}
