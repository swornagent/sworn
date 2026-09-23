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

func TestValidateNativeConfigAdmitsTestedVersionInEveryPinMode(t *testing.T) {
	for _, mode := range []string{"", NativePinModeExact, NativePinModeMinor} {
		config := nativePinModeTestConfig(
			t, mode, ClaudeCLIVersion, ClaudeCLIDigest,
			ClaudeCredentialTarget, ClaudeCLIVersion+" (Claude Code)",
		)
		if err := validateNativeConfig(config); err != nil {
			t.Fatalf("pin_mode %q: tested version rejected: %v", mode, err)
		}
	}
}

// A CLI newer (or older) than the tested version is admitted in every pin
// mode: the configured digest and version output bind the binary, and the
// tested version is a compatibility statement, not a gate. This is what lets
// an operator follow a CLI that releases daily without a new Sworn build.
func TestValidateNativeConfigAdmitsUntestedVersionsWithTheirOwnDigest(t *testing.T) {
	for _, mode := range []string{"", NativePinModeExact, NativePinModeMinor} {
		for _, version := range []string{"2.1.280", "2.2.0", "3.0.1", "2.0.9"} {
			config := nativePinModeTestConfig(
				t, mode, version,
				"sha256:"+string(bytes.Repeat([]byte("b"), 64)),
				ClaudeCredentialTarget, version+" (Claude Code)",
			)
			if err := validateNativeConfig(config); err != nil {
				t.Fatalf("pin_mode %q: version %q rejected: %v", mode, version, err)
			}
		}
	}
}

func TestValidateNativeConfigRejectsMalformedVersion(t *testing.T) {
	for _, version := range []string{"", "latest", "2.1", "v2.1.280"} {
		config := nativePinModeTestConfig(
			t, "", version, ClaudeCLIDigest,
			ClaudeCredentialTarget, version+" (Claude Code)",
		)
		if err := validateNativeConfig(config); !IsCode(err, "NATIVE_NOT_CERTIFIED") {
			t.Fatalf("malformed version %q admitted: %v", version, err)
		}
	}
}

func TestValidateNativeConfigKeepsCredentialTargetExact(t *testing.T) {
	config := nativePinModeTestConfig(
		t, "", "2.1.280",
		"sha256:"+string(bytes.Repeat([]byte("b"), 64)),
		"/home/sworn/.claude/wrong-target.json", "2.1.280 (Claude Code)",
	)
	if err := validateNativeConfig(config); !IsCode(err, "NATIVE_NOT_CERTIFIED") {
		t.Fatalf("a wrong credential target was admitted: %v", err)
	}
}

func TestValidateNativeConfigRequiresSelfConsistentVersionOutput(t *testing.T) {
	config := nativePinModeTestConfig(
		t, "", "2.1.280",
		"sha256:"+string(bytes.Repeat([]byte("b"), 64)),
		ClaudeCredentialTarget, ClaudeCLIVersion+" (Claude Code)",
	)
	if err := validateNativeConfig(config); !IsCode(err, "NATIVE_NOT_CERTIFIED") {
		t.Fatalf("a version_output naming a different version was admitted: %v", err)
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

func TestNativeCLICompatibilityReportsTestedAndUntested(t *testing.T) {
	tested := CLICompatibilityOf(NativeAdapterConfig{
		Family: ProfileClaude, CLIVersion: ClaudeCLIVersion,
	})
	if tested == nil || tested.Status != NativeCLITested ||
		tested.TestedVersion != ClaudeCLIVersion || tested.Note != "" {
		t.Fatalf("tested claude CLI misreported: %+v", tested)
	}
	untested := CLICompatibilityOf(NativeAdapterConfig{
		Family: ProfileClaude, CLIVersion: "2.1.280",
	})
	if untested == nil || untested.Status != NativeCLIUntested ||
		untested.CLIVersion != "2.1.280" ||
		!bytes.Contains([]byte(untested.Note), []byte("not guaranteed")) {
		t.Fatalf("untested claude CLI misreported: %+v", untested)
	}
	codex := CLICompatibilityOf(NativeAdapterConfig{
		Family: ProfileCodex, CLIVersion: "0.200.0",
	})
	if codex == nil || codex.Status != NativeCLIUntested ||
		codex.TestedVersion != CodexCLIVersion {
		t.Fatalf("untested codex CLI misreported: %+v", codex)
	}
	if CLICompatibilityOf(NativeAdapterConfig{Family: ProfileOpenAIHTTP}) != nil {
		t.Fatal("a non-native family reported CLI compatibility")
	}
}
