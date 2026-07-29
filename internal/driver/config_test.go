package driver

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestDriverConfigCodecDigestAndPrivacyAreStrict(t *testing.T) {
	config := completeDriverConfigFixture(t)
	body, err := EncodeDriverConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := DecodeDriverConfig(body)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ConfigurationDigest() != Digest(body) ||
		!bytes.Equal(loaded.CanonicalJSON(), body) {
		t.Fatalf(
			"digest=%s body=%s",
			loaded.ConfigurationDigest(),
			loaded.CanonicalJSON(),
		)
	}
	copyBody := loaded.CanonicalJSON()
	copyBody[0] = '!'
	if !bytes.Equal(loaded.CanonicalJSON(), body) {
		t.Fatal("canonical configuration bytes were mutable")
	}
	for _, forbidden := range []string{
		"credential-secret-canary",
		"fallback",
		"responses",
	} {
		if bytes.Contains(body, []byte(forbidden)) {
			t.Fatalf("configuration contained %q: %s", forbidden, body)
		}
	}

	noncanonical := append(append([]byte(nil), body...), '\n')
	if _, err := DecodeDriverConfig(noncanonical); !IsCode(err, "NONCANONICAL_JSON") {
		t.Fatalf("noncanonical error = %v", err)
	}
	unknown := bytes.Replace(
		body,
		[]byte(`"schema_version":"sworn.driver-config/v1"`),
		[]byte(`"schema_version":"sworn.driver-config/v1","fallback_profile":"other"`),
		1,
	)
	if _, err := DecodeDriverConfig(unknown); !IsCode(err, "UNKNOWN_FIELD") {
		t.Fatalf("unknown field error = %v", err)
	}
	duplicate := bytes.Replace(
		body,
		[]byte(`"schema_version":"sworn.driver-config/v1"`),
		[]byte(`"schema_version":"sworn.driver-config/v1","schema_version":"sworn.driver-config/v1"`),
		1,
	)
	if _, err := DecodeDriverConfig(duplicate); !IsCode(err, "DUPLICATE_NAME") {
		t.Fatalf("duplicate field error = %v", err)
	}

	reordered := config
	reordered.Profiles = append([]DriverProfile(nil), config.Profiles...)
	reordered.Profiles[0], reordered.Profiles[1] =
		reordered.Profiles[1], reordered.Profiles[0]
	if _, err := EncodeDriverConfig(reordered); !IsCode(err, "INVALID_DRIVER_CONFIG") {
		t.Fatalf("reordered profile error = %v", err)
	}
	withSecretField := bytes.Replace(
		body,
		[]byte(`"reference":"BEDROCK_MANTLE_API_KEY"`),
		[]byte(`"reference":"BEDROCK_MANTLE_API_KEY","value":"credential-secret-canary"`),
		1,
	)
	if _, err := DecodeDriverConfig(withSecretField); !IsCode(err, "UNKNOWN_FIELD") {
		t.Fatalf("secret value field error = %v", err)
	}

	pathValue := filepath.Join(t.TempDir(), "drivers.json")
	if err := os.WriteFile(pathValue, body, 0o600); err != nil {
		t.Fatal(err)
	}
	fromFile, err := LoadDriverConfig(pathValue)
	if err != nil || fromFile.ConfigurationDigest() != loaded.ConfigurationDigest() {
		t.Fatalf("loaded file = %#v, %v", fromFile, err)
	}
	if _, err := LoadDriverConfig("relative.json"); !IsCode(err, "INVALID_CONFIG_PATH") {
		t.Fatalf("relative path error = %v", err)
	}
}

func TestDriverConfigFactoryBuildsSubsetAndEveryFamilyWithoutResolvingSecrets(t *testing.T) {
	config := completeDriverConfigFixture(t)
	body, err := EncodeDriverConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := DecodeDriverConfig(body)
	if err != nil {
		t.Fatal(err)
	}
	var resolved atomic.Int64
	options := DriverFactoryOptions{
		EnvironmentCredentials: func(context.Context, string) ([]byte, error) {
			resolved.Add(1)
			return nil, fail("CREDENTIAL_UNAVAILABLE")
		},
		FileCredentials: func(context.Context, string) ([]byte, error) {
			resolved.Add(1)
			return nil, fail("CREDENTIAL_UNAVAILABLE")
		},
		AWSCredentials: func(context.Context, string) ([][]byte, error) {
			resolved.Add(1)
			return nil, fail("CREDENTIAL_UNAVAILABLE")
		},
	}
	subset, err := loaded.BuildRegistry([]string{"openai"}, options)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Load() != 0 ||
		subset.ConfigurationDigest() != loaded.ConfigurationDigest() ||
		len(subset.Profiles()) != 1 ||
		len(subset.Certifications()) != 1 {
		t.Fatalf(
			"subset profiles=%v certifications=%v resolved=%d digest=%s",
			subset.Profiles(),
			subset.Certifications(),
			resolved.Load(),
			subset.ConfigurationDigest(),
		)
	}
	if _, err := subset.Resolve(explicitSelections("missing"), RolePlanner); !IsCode(err, "UNKNOWN_PROFILE") {
		t.Fatalf("unknown selection error = %v", err)
	}
	if _, err := loaded.BuildRegistry(nil, options); !IsCode(err, "MISSING_PROFILE") {
		t.Fatalf("implicit subset error = %v", err)
	}
	if _, err := loaded.BuildRegistry(
		[]string{"openai", "missing"},
		options,
	); !IsCode(err, "UNKNOWN_PROFILE") {
		t.Fatalf("unknown configured profile error = %v", err)
	}

	all, err := loaded.BuildAllRegistry(options)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Load() != 0 ||
		len(all.Profiles()) != len(config.Profiles) ||
		len(all.Certifications()) != len(config.Profiles) {
		t.Fatalf(
			"all profiles=%v certifications=%v resolved=%d",
			all.Profiles(),
			all.Certifications(),
			resolved.Load(),
		)
	}
	classic := all.Inspect(context.Background(), "bedrock", "model-bedrock")
	mantleAPI := all.Inspect(context.Background(), "mantle-api", "model-mantle-api")
	mantleAWS := all.Inspect(context.Background(), "mantle-aws", "model-mantle-aws")
	if classic.Family != ProfileBedrock ||
		classic.Surface != ProfileSurfaceBedrockRuntimeConverse ||
		mantleAPI.Family != ProfileBedrock ||
		mantleAPI.Surface != ProfileSurfaceBedrockMantleChat ||
		mantleAWS.Family != ProfileBedrock ||
		mantleAWS.Surface != ProfileSurfaceBedrockMantleChat {
		t.Fatalf(
			"Bedrock reports classic=%#v api=%#v aws=%#v",
			classic,
			mantleAPI,
			mantleAWS,
		)
	}
	reportBody, err := canonicalJSON([]ProfileReport{classic, mantleAPI, mantleAWS})
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{
		"AWS_RUNTIME_SOURCE",
		"BEDROCK_MANTLE_API_KEY",
		"/credentials/",
	} {
		if bytes.Contains(reportBody, []byte(private)) {
			t.Fatalf("profile reports leaked %q: %s", private, reportBody)
		}
	}
}

func explicitSelections(profile string) RoleSelections {
	return RoleSelections{
		Planner:     RoleSelection{Profile: profile, Model: "planner"},
		Implementer: RoleSelection{Profile: profile, Model: "implementer"},
		Captain:     RoleSelection{Profile: profile, Model: "captain"},
		Verifier:    RoleSelection{Profile: profile, Model: "verifier"},
	}
}

func completeDriverConfigFixture(t *testing.T) DriverConfig {
	t.Helper()
	root := t.TempDir()
	executable := func(name string, digest string) ExecutableIdentity {
		t.Helper()
		pathValue := filepath.Join(root, name)
		if err := os.WriteFile(pathValue, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		return ExecutableIdentity{Path: pathValue, Digest: digest}
	}
	runtimeFiles := driverRuntimeFilesFixture(t, root)
	required := make([]string, len(runtimeFiles))
	for index := range runtimeFiles {
		required[index] = runtimeFiles[index].Target
	}
	awsChain := AWSChainSpec{
		CLI:              executable("aws", AWSCLIDigest),
		CLIVersion:       AWSCLIVersion,
		Profile:          "",
		Region:           "ap-southeast-2",
		RegionSource:     AWSSourceEnvironment,
		CredentialSource: AWSSourceEnvironment,
		EnvironmentKeys: []string{
			"AWS_ACCESS_KEY_ID",
			"AWS_DEFAULT_REGION",
			"AWS_REGION",
			"AWS_SECRET_ACCESS_KEY",
		},
		RuntimeFiles:           runtimeFiles,
		RequiredRuntimeTargets: required,
	}
	credentials := []DriverCredentialSource{
		{Key: "aws", Kind: CredentialAWS, Reference: "AWS_RUNTIME_SOURCE"},
		{Key: "claude-file", Kind: CredentialFile, Reference: filepath.Join(root, "credentials", "claude.json")},
		{Key: "codex-file", Kind: CredentialFile, Reference: filepath.Join(root, "credentials", "codex.json")},
		{Key: "deepseek-env", Kind: CredentialEnvironment, Reference: "DEEPSEEK_API_KEY"},
		{Key: "gemini-file", Kind: CredentialFile, Reference: filepath.Join(root, "credentials", "gemini.key")},
		{Key: "mantle-aws", Kind: CredentialAWS, Reference: "AWS_MANTLE_SOURCE"},
		{Key: "mantle-env", Kind: CredentialEnvironment, Reference: "BEDROCK_MANTLE_API_KEY"},
		{Key: "openai-env", Kind: CredentialEnvironment, Reference: "OPENAI_API_KEY"},
	}
	native := func(
		key, id string,
		family ProfileFamily,
		cli ExecutableIdentity,
		version string,
		output string,
		target string,
	) *NativeAdapterConfig {
		return &NativeAdapterConfig{
			Key:                    key,
			ID:                     id,
			Version:                "1.0.0",
			Family:                 family,
			CLI:                    cli,
			CLIVersion:             version,
			VersionOutput:          output,
			RuntimeFiles:           runtimeFiles,
			RequiredRuntimeTargets: required,
			CredentialTarget:       target,
			CredentialRefs:         []string{strings.TrimPrefix(key, "a-") + "-file"},
			MaxCredentialBytes:     1_048_576,
		}
	}
	adapters := []DriverAdapterConfig{
		{Bedrock: &BedrockProfileConfig{
			Key: "a-bedrock", ID: "sworn.bedrock", Version: "1.0.0",
			Endpoint: "http://localhost:4101", CredentialRefs: []string{"aws"},
			ResponseBytes: MaxProviderResponseBytes, Chain: awsChain,
			AllowCachePoint: false, AllowGuardContent: false,
		}},
		{
			Native: native(
				"a-claude",
				"sworn.claude",
				ProfileClaude,
				executable("claude", ClaudeCLIDigest),
				ClaudeCLIVersion,
				ClaudeCLIVersion+" (Claude Code)",
				ClaudeCredentialTarget,
			),
		},
		{
			Native: native(
				"a-codex",
				"sworn.codex",
				ProfileCodex,
				executable("codex", CodexCLIDigest),
				CodexCLIVersion,
				"codex-cli "+CodexCLIVersion,
				CodexCredentialTarget,
			),
		},
		{DeepSeek: &HTTPProfileConfig{
			Key: "a-deepseek", ID: "sworn.deepseek", Version: "1.0.0",
			Endpoint:         "http://localhost:4102/chat/completions",
			CredentialHeader: "Authorization", CredentialPrefix: "Bearer ",
			CredentialRefs: []string{"deepseek-env"},
			ResponseBytes:  MaxProviderResponseBytes,
		}},
		{Process: &DriverProcessAdapterConfig{
			Key: "a-fake", ID: FakeDriverID, Version: FakeDriverVersion,
			Executable: executable("fake", Digest([]byte("fake"))),
		}},
		{Gemini: &HTTPProfileConfig{
			Key: "a-gemini", ID: "sworn.gemini", Version: "1.0.0",
			Endpoint:         "http://localhost:4103",
			CredentialHeader: "x-goog-api-key", CredentialPrefix: "",
			CredentialRefs: []string{"gemini-file"},
			ResponseBytes:  MaxProviderResponseBytes,
		}},
		{Mantle: &BedrockMantleProfileConfig{
			Key: "a-mantle-api", ID: "sworn.bedrock.mantle.api", Version: "1.0.0",
			Endpoint:       "http://localhost:4104/v1/chat/completions",
			CredentialRefs: []string{"mantle-env"},
			ResponseBytes:  MaxProviderResponseBytes, AuthMode: BedrockMantleAPIKey,
		}},
		{Mantle: &BedrockMantleProfileConfig{
			Key: "a-mantle-aws", ID: "sworn.bedrock.mantle.aws", Version: "1.0.0",
			Endpoint:       "http://localhost:4105/v1/chat/completions",
			CredentialRefs: []string{"mantle-aws"},
			ResponseBytes:  MaxProviderResponseBytes, AuthMode: BedrockMantleAWS,
			Chain: &awsChain,
		}},
		{OpenAI: &HTTPProfileConfig{
			Key: "a-openai", ID: "sworn.openai", Version: "1.0.0",
			Endpoint:         "http://localhost:4106/chat/completions",
			CredentialHeader: "Authorization", CredentialPrefix: "Bearer ",
			CredentialRefs: []string{"openai-env"},
			ResponseBytes:  MaxProviderResponseBytes,
		}},
	}
	profile := func(
		key, adapter, credential, model string,
	) DriverProfile {
		credentialCopy := credential
		return DriverProfile{
			Key: key, Adapter: adapter, Network: NetworkRequired,
			CredentialSource:    &credentialCopy,
			CertificationModels: []string{model},
		}
	}
	profiles := []DriverProfile{
		profile("bedrock", "a-bedrock", "aws", "model-bedrock"),
		profile("claude", "a-claude", "claude-file", "model-claude"),
		profile("codex", "a-codex", "codex-file", "model-codex"),
		profile("deepseek", "a-deepseek", "deepseek-env", "model-deepseek"),
		{
			Key: "fake", Adapter: "a-fake", Network: NetworkNone,
			CredentialSource: nil, CertificationModels: []string{"model-fake"},
		},
		profile("gemini", "a-gemini", "gemini-file", "model-gemini"),
		profile("mantle-api", "a-mantle-api", "mantle-env", "model-mantle-api"),
		profile("mantle-aws", "a-mantle-aws", "mantle-aws", "model-mantle-aws"),
		profile("openai", "a-openai", "openai-env", "model-openai"),
	}
	return DriverConfig{
		SchemaVersion: DriverConfigSchemaVersion,
		Credentials:   credentials,
		Adapters:      adapters,
		Profiles:      profiles,
	}
}

func driverRuntimeFilesFixture(
	t *testing.T,
	root string,
) []PinnedRuntimeFile {
	t.Helper()
	targets := []string{
		"/etc/hosts",
		"/etc/nsswitch.conf",
		"/etc/resolv.conf",
		"/etc/ssl/certs/ca-certificates.crt",
	}
	files := make([]PinnedRuntimeFile, len(targets))
	for index, target := range targets {
		name := strings.ReplaceAll(strings.TrimPrefix(target, "/"), "/", "-")
		pathValue := filepath.Join(root, "runtime-"+name)
		body := []byte("runtime " + target)
		if err := os.WriteFile(pathValue, body, 0o600); err != nil {
			t.Fatal(err)
		}
		files[index] = PinnedRuntimeFile{
			Path: pathValue, Target: target, Digest: Digest(body),
		}
	}
	return files
}
