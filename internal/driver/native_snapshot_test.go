package driver

import (
	"errors"
	"strings"
	"testing"
)

// nativeSnapshotDeclarationFixture is an otherwise-valid Claude adapter that
// opts in to a run snapshot. Its host path deliberately does not exist:
// admission checks only its shape, and each run resolves it at start.
func nativeSnapshotDeclarationFixture(t *testing.T) NativeAdapterConfig {
	t.Helper()
	config := nativePinModeTestConfig(t, "", "", "", ClaudeCredentialTarget, "")
	config.CLI = ExecutableIdentity{Path: "/opt/host/bin/claude"}
	config.CLIResolution = NativeCLIResolutionRunSnapshot
	return config
}

func requireNativeRefusal(t *testing.T, err error, code, detail string) {
	t.Helper()
	var contractErr *ContractError
	if !errors.As(err, &contractErr) || contractErr.Code != code ||
		contractErr.Detail != detail {
		t.Fatalf("refusal = %v, want %s/%s", err, code, detail)
	}
}

func TestValidateNativeConfigAdmitsARunSnapshotDeclaration(t *testing.T) {
	if err := validateNativeConfig(nativeSnapshotDeclarationFixture(t)); err != nil {
		t.Fatalf("run-snapshot declaration refused: %v", err)
	}
}

func TestValidateNativeConfigRefusesPinnedFieldsBesideARunSnapshot(t *testing.T) {
	for name, mutate := range map[string]func(*NativeAdapterConfig){
		"digest": func(config *NativeAdapterConfig) {
			config.CLI.Digest = ClaudeCLIDigest
		},
		"cli_version": func(config *NativeAdapterConfig) {
			config.CLIVersion = "2.1.280"
		},
		"version_output": func(config *NativeAdapterConfig) {
			config.VersionOutput = "2.1.280 (Claude Code)"
		},
	} {
		t.Run(name, func(t *testing.T) {
			config := nativeSnapshotDeclarationFixture(t)
			mutate(&config)
			requireNativeRefusal(
				t, validateNativeConfig(config),
				"NATIVE_NOT_CERTIFIED", "cli_resolution_pinned",
			)
		})
	}
}

// A bare command name would resolve through the PATH of whichever process
// read the file, so doctor and serve could bind different binaries.
func TestValidateNativeConfigRefusesARelativeRunSnapshotPath(t *testing.T) {
	for _, pathValue := range []string{"", "claude", "bin/claude", "/opt/../opt/claude"} {
		config := nativeSnapshotDeclarationFixture(t)
		config.CLI.Path = pathValue
		requireNativeRefusal(
			t, validateNativeConfig(config), "NATIVE_NOT_CERTIFIED", "cli_identity",
		)
	}
}

func TestValidateNativeConfigRefusesAnUnknownCLIResolution(t *testing.T) {
	config := nativeSnapshotDeclarationFixture(t)
	config.CLIResolution = "snapshot"
	requireNativeRefusal(
		t, validateNativeConfig(config), "NATIVE_NOT_CERTIFIED", "cli_resolution",
	)
}

// Without the explicit opt-in, a path with no digest is still refused: it
// never falls back to a snapshot.
func TestValidateNativeConfigStillRefusesAMissingDigestWithoutTheOptIn(t *testing.T) {
	pinned := nativePinModeTestConfig(
		t, "", ClaudeCLIVersion, "", ClaudeCredentialTarget,
		ClaudeCLIVersion+" (Claude Code)",
	)
	requireNativeRefusal(
		t, validateNativeConfig(pinned), "NATIVE_NOT_CERTIFIED", "cli_identity",
	)
	undeclared := nativeSnapshotDeclarationFixture(t)
	undeclared.CLIResolution = ""
	requireNativeRefusal(
		t, validateNativeConfig(undeclared), "NATIVE_NOT_CERTIFIED", "cli_identity",
	)
}

// The literal document below is the canonical form every pinned native
// configuration had before cli_resolution existed. Decoding it must still
// succeed byte-for-byte, so its configuration digest is unchanged.
func TestPinnedNativeDriverConfigKeepsItsCanonicalBytes(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	document := `{"schema_version":"sworn.driver-config/v1",` +
		`"credentials":[{"key":"claude-file","kind":"file","reference":"/credentials/claude.json"}],` +
		`"adapters":[{"native":{"key":"a-claude","id":"sworn.claude","version":"1.0.0","family":"claude_code_cli",` +
		`"cli":{"path":"/opt/claude/bin/claude","digest":"` + digest + `"},` +
		`"cli_version":"2.1.280","version_output":"2.1.280 (Claude Code)",` +
		`"runtime_files":[` +
		`{"path":"/etc/hosts","target":"/etc/hosts","digest":"` + digest + `"},` +
		`{"path":"/etc/nsswitch.conf","target":"/etc/nsswitch.conf","digest":"` + digest + `"},` +
		`{"path":"/etc/resolv.conf","target":"/etc/resolv.conf","digest":"` + digest + `"},` +
		`{"path":"/etc/ssl/certs/ca-certificates.crt","target":"/etc/ssl/certs/ca-certificates.crt","digest":"` + digest + `"}],` +
		`"required_runtime_targets":["/etc/hosts","/etc/nsswitch.conf","/etc/resolv.conf","/etc/ssl/certs/ca-certificates.crt"],` +
		`"credential_target":"/home/sworn/.claude/.credentials.json","credential_refs":["claude-file"],` +
		`"max_credential_bytes":65536}}],` +
		`"profiles":[{"key":"claude","adapter":"a-claude","network":"required","credential_source":"claude-file",` +
		`"certification_models":["model-claude"]}]}`
	loaded, err := DecodeDriverConfig([]byte(document))
	if err != nil {
		t.Fatalf("pinned native document no longer canonical: %v", err)
	}
	if loaded.ConfigurationDigest() != Digest([]byte(document)) ||
		string(loaded.CanonicalJSON()) != document {
		t.Fatalf("pinned native document changed identity:\n%s", loaded.CanonicalJSON())
	}
	native := *loaded.config.Adapters[0].Native
	body, err := canonicalJSON(native)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(document, string(body)) ||
		strings.Contains(string(body), "cli_resolution") {
		t.Fatalf("pinned adapter canonical bytes changed: %s", body)
	}
}

// A run-snapshot declaration leaves the pinned fields absent rather than
// empty, and names the opt-in explicitly.
func TestRunSnapshotDeclarationCanonicalBytesOmitThePinnedFields(t *testing.T) {
	body, err := canonicalJSON(nativeSnapshotDeclarationFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, absent := range []string{`"digest":""`, `"cli_version"`, `"version_output"`} {
		if strings.Contains(string(body), absent) {
			t.Fatalf("run-snapshot declaration carries %s: %s", absent, body)
		}
	}
	if !strings.Contains(string(body), `"cli":{"path":"/opt/host/bin/claude"}`) ||
		!strings.Contains(string(body), `"cli_resolution":"run_snapshot"`) {
		t.Fatalf("run-snapshot declaration shape: %s", body)
	}
}

func TestNativeCLIVersionReadsEachFamilyFormat(t *testing.T) {
	for _, testCase := range []struct {
		family ProfileFamily
		output string
		want   string
		ok     bool
	}{
		{ProfileCodex, "codex-cli 0.146.0", "0.146.0", true},
		{ProfileClaude, "2.1.280 (Claude Code)", "2.1.280", true},
		{ProfileClaude, " (Claude Code)", "", false},
		{ProfileCodex, "2.1.280 (Claude Code)", "", false},
		{ProfileOpenAIHTTP, "codex-cli 0.146.0", "", false},
	} {
		got, ok := NativeCLIVersion(testCase.family, testCase.output)
		if got != testCase.want || ok != testCase.ok {
			t.Fatalf("%s %q = (%q, %v), want (%q, %v)", testCase.family,
				testCase.output, got, ok, testCase.want, testCase.ok)
		}
	}
}

func TestDecodeNativeCLISnapshotAdmitsOnlyCanonicalFacts(t *testing.T) {
	digest := "sha256:" + strings.Repeat("b", 64)
	fact := NativeCLISnapshot{
		SchemaVersion: NativeCLISnapshotEventVersion,
		RunID:         "run-1",
		Adapter:       "a-claude",
		Family:        ProfileClaude,
		SourcePath:    "/home/operator/.local/bin/claude",
		SnapshotPath:  "/store/native-cli-snapshots/" + strings.Repeat("b", 64),
		Digest:        digest,
		CLIVersion:    "2.1.280",
		VersionOutput: "2.1.280 (Claude Code)",
	}
	body, err := EncodeNativeCLISnapshot(fact)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeNativeCLISnapshot(body)
	if err != nil || decoded != fact {
		t.Fatalf("round trip = %+v, %v", decoded, err)
	}
	for name, mutate := range map[string]func(*NativeCLISnapshot){
		"run id":         func(fact *NativeCLISnapshot) { fact.RunID = "" },
		"not addressed":  func(fact *NativeCLISnapshot) { fact.SnapshotPath = "/store/claude" },
		"version output": func(fact *NativeCLISnapshot) { fact.VersionOutput = "2.1.281 (Claude Code)" },
		"relative":       func(fact *NativeCLISnapshot) { fact.SourcePath = "claude" },
		"family":         func(fact *NativeCLISnapshot) { fact.Family = ProfileOpenAIHTTP },
	} {
		t.Run(name, func(t *testing.T) {
			mutated := fact
			mutate(&mutated)
			_, err := EncodeNativeCLISnapshot(mutated)
			requireNativeRefusal(t, err, "NATIVE_CLI_SNAPSHOT_INVALID", "fact_invalid")
		})
	}
	requireNativeRefusal(
		t, func() error {
			_, err := DecodeNativeCLISnapshot(append(body, '\n'))
			return err
		}(),
		"NATIVE_CLI_SNAPSHOT_INVALID", "fact_invalid",
	)
}
