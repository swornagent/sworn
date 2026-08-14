package driver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/swornagent/sworn/internal/gitx"
)

// TestPresetProvidersAreConfigurationOnly drives three OpenAI-compatible
// providers added as configuration only (no new Go types): a plain-HTTP
// loopback chat provider with no credential, an HTTPS bearer provider reached
// through an injected loopback round tripper, and an OpenRouter preset. All
// three resolve through the one unified OpenAI adapter code path.
func TestPresetProvidersAreConfigurationOnly(t *testing.T) {
	var loopbackRequests atomic.Int64
	var loopbackAuthorization string
	loopback := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		loopbackRequests.Add(1)
		loopbackAuthorization = request.Header.Get("Authorization")
		if request.URL.Path != "/v1/chat/completions" {
			t.Errorf("loopback preset path = %s", request.URL.Path)
		}
		writeJSONResponse(t, writer, openAIToolCallResponse(
			"loopback-submit", "sworn_submit",
			presetSubmissionArguments(t, "preset-loopback"),
			3, 2,
		))
	}))
	defer loopback.Close()

	var tlsRequests atomic.Int64
	tlsServer := httptest.NewTLSServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		tlsRequests.Add(1)
		if request.Header.Get("Authorization") != "Bearer tls-secret-canary" {
			t.Errorf("TLS preset bearer header = %q", request.Header.Get("Authorization"))
		}
		writeJSONResponse(t, writer, openAIToolCallResponse(
			"tls-submit", "sworn_submit",
			presetSubmissionArguments(t, "preset-tls"),
			5, 4,
		))
	}))
	defer tlsServer.Close()

	var routerRequests atomic.Int64
	routerServer := httptest.NewTLSServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		routerRequests.Add(1)
		if request.Header.Get("Authorization") != "Bearer router-secret-canary" {
			t.Errorf("router preset bearer header = %q", request.Header.Get("Authorization"))
		}
		writeJSONResponse(t, writer, openAIToolCallResponse(
			"router-submit", "sworn_submit",
			presetSubmissionArguments(t, "preset-router"),
			7, 6,
		))
	}))
	defer routerServer.Close()

	tlsEnv := "tls-env"
	routerEnv := "router-env"
	config := DriverConfig{
		SchemaVersion: DriverConfigSchemaVersion,
		Presets: []DriverPreset{
			{
				Key: "preset-loopback", API: OpenAIChatCompletionsAPI,
				BaseURL: loopback.URL + "/v1/chat/completions", Auth: AuthModeNone,
				ResponseBytes: MaxProviderResponseBytes,
			},
			{
				Key: "preset-router", API: OpenRouterChatCompletionsAPI,
				BaseURL:          routerServer.URL + "/v1/chat/completions",
				Auth:             AuthModeBearer,
				CredentialHeader: "Authorization", CredentialPrefix: "Bearer ",
				ResponseBytes: MaxProviderResponseBytes,
			},
			{
				Key: "preset-tls", API: OpenAIChatCompletionsAPI,
				BaseURL:          tlsServer.URL + "/v1/chat/completions",
				Auth:             AuthModeBearer,
				CredentialHeader: "Authorization", CredentialPrefix: "Bearer ",
				ResponseBytes: MaxProviderResponseBytes,
			},
		},
		Credentials: []DriverCredentialSource{
			{Key: "router-env", Kind: CredentialEnvironment, Reference: "OPENROUTER_API_KEY"},
			{Key: "tls-env", Kind: CredentialEnvironment, Reference: "TLS_API_KEY"},
		},
		Adapters: []DriverAdapterConfig{
			{OpenAI: &OpenAIProfileConfig{
				HTTPProfileConfig: HTTPProfileConfig{
					Key: "a-loopback", ID: "sworn.loopback", Version: "1.0.0",
				},
				Preset: "preset-loopback",
			}},
			{OpenAI: &OpenAIProfileConfig{
				HTTPProfileConfig: HTTPProfileConfig{
					Key: "a-router", ID: "sworn.router", Version: "1.0.0",
					CredentialRefs: []string{"router-env"},
				},
				Preset: "preset-router",
			}},
			{OpenAI: &OpenAIProfileConfig{
				HTTPProfileConfig: HTTPProfileConfig{
					Key: "a-tls", ID: "sworn.tls", Version: "1.0.0",
					CredentialRefs: []string{"tls-env"},
				},
				Preset: "preset-tls",
			}},
		},
		Profiles: []DriverProfile{
			{
				Key: "loopback", Adapter: "a-loopback",
				Network: NetworkRequired, AuthMode: authModePtr(AuthModeNone),
				CertificationModels: []string{"model-loopback"},
			},
			{
				Key: "router", Adapter: "a-router",
				Network: NetworkRequired, AuthMode: authModePtr(AuthModeBearer),
				CredentialSource:    &routerEnv,
				CertificationModels: []string{"model-router"},
			},
			{
				Key: "tls", Adapter: "a-tls",
				Network: NetworkRequired, AuthMode: authModePtr(AuthModeBearer),
				CredentialSource:    &tlsEnv,
				CertificationModels: []string{"model-tls"},
			},
		},
	}
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
			"preset document digest=%s canonical=%s",
			loaded.ConfigurationDigest(),
			loaded.CanonicalJSON(),
		)
	}
	options := DriverFactoryOptions{
		EnvironmentCredentials: func(_ context.Context, ref string) ([]byte, error) {
			switch ref {
			case "TLS_API_KEY":
				return []byte("tls-secret-canary"), nil
			case "OPENROUTER_API_KEY":
				return []byte("router-secret-canary"), nil
			}
			return nil, fail("CREDENTIAL_UNAVAILABLE")
		},
		RoundTrippers: map[string]http.RoundTripper{
			"a-tls":    tlsServer.Client().Transport,
			"a-router": routerServer.Client().Transport,
		},
	}
	registry, err := loaded.BuildRegistry(
		[]string{"loopback", "router", "tls"},
		options,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		profile  string
		model    string
		requests *atomic.Int64
		tokens   []int64
	}{
		{"loopback", "model-loopback", &loopbackRequests, []int64{3, 2}},
		{"tls", "model-tls", &tlsRequests, []int64{5, 4}},
		{"router", "model-router", &routerRequests, []int64{7, 6}},
	} {
		selected, resolveErr := registry.ResolveSelection(
			ModelSelection{Profile: row.profile, Model: row.model},
		)
		if resolveErr != nil {
			t.Fatal(resolveErr)
		}
		invocation := presetInvocation(
			t,
			selected,
			"preset-"+row.profile,
			RoleImplementer,
			ImplementerImplementation,
			ReadWrite,
		)
		observation, invokeErr := (Dispatcher{}).Invoke(context.Background(), invocation)
		if invokeErr != nil || observation.Handoff == nil ||
			row.requests.Load() != 1 ||
			observation.Usage.InputTokens == nil ||
			*observation.Usage.InputTokens != row.tokens[0] ||
			observation.Usage.OutputTokens == nil ||
			*observation.Usage.OutputTokens != row.tokens[1] {
			t.Fatalf(
				"%s preset observation=%#v requests=%d error=%v",
				row.profile,
				observation,
				row.requests.Load(),
				invokeErr,
			)
		}
		report := registry.Inspect(
			context.Background(),
			row.profile,
			row.model,
		)
		expectedSurface := ProfileSurfaceOpenAIChat
		if row.profile == "router" {
			expectedSurface = ProfileSurfaceOpenRouterChat
		}
		if report.Family != ProfileOpenAIHTTP ||
			report.Surface != expectedSurface {
			t.Fatalf("%s preset report = %#v", row.profile, report)
		}
	}
	// The none-auth loopback adapter carried no placeholder secret and no
	// credential ref, and no authorization header reached the provider.
	if loopbackAuthorization != "" {
		t.Fatalf("none-auth loopback sent authorization %q", loopbackAuthorization)
	}
	loop := registryResolveAdapter(t, registry.SelectionRegistry, "loopback", "model-loopback")
	loopAdapter, ok := loop.(*loopAdapter)
	if !ok {
		t.Fatalf("preset adapter type = %T", loop)
	}
	if len(loopAdapter.transport.(*httpTransport).refs) != 0 {
		t.Fatalf("none-auth adapter retained credential refs")
	}
}

func registryResolveAdapter(
	t *testing.T,
	registry SelectionRegistry,
	profile string,
	model string,
) Adapter {
	t.Helper()
	selected, err := registry.ResolveSelection(
		ModelSelection{Profile: profile, Model: model},
	)
	if err != nil {
		t.Fatal(err)
	}
	return selected.adapter
}

func presetInvocation(
	t *testing.T,
	selected SelectedProfile,
	invocationID string,
	role Role,
	responsibility Responsibility,
	access WorkspaceAccess,
) Invocation {
	t.Helper()
	request, err := NewRequest(
		invocationID,
		role,
		selected.Profile.Key,
		selected.Model,
		Workspace{Path: GuestWorkspacePath, Access: access},
		nil,
		true,
		Limits{TimeoutMillis: 5_000, OutputBytes: 65_536},
	)
	if err != nil {
		t.Fatal(err)
	}
	containment := ContainmentReadWrite
	if access == ReadOnly {
		containment = ContainmentReadOnly
	}
	permission, err := NewSubmissionPermission(
		request,
		selected,
		containment,
		responsibility,
	)
	if err != nil {
		t.Fatal(err)
	}
	return Invocation{
		Request: request, HostWorkspace: t.TempDir(),
		Selected: selected, Permission: permission,
	}
}

func presetSubmissionArguments(t *testing.T, invocationID string) string {
	t.Helper()
	submission := submissionFixture(
		t,
		invocationID,
		ImplementerImplementation,
		"",
	)
	return submissionToolArguments(t, submission)
}

func authModePtr(mode AuthMode) *AuthMode {
	return &mode
}

// TestPresetAuthAdmissionFailsClosed covers the A2 no-placeholder-secret
// promise: explicit none admits a credential-less loopback, while omission
// never yields none.
func TestPresetAuthAdmissionFailsClosed(t *testing.T) {
	loopback := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		writeJSONResponse(t, writer, openAIToolCallResponse(
			"loopback-submit", "sworn_submit",
			presetSubmissionArguments(t, "preset-fail-closed"),
			1, 1,
		))
	}))
	defer loopback.Close()

	noneEnv := "none-env"
	base := func() DriverConfig {
		return DriverConfig{
			SchemaVersion: DriverConfigSchemaVersion,
			Presets: []DriverPreset{{
				Key: "preset-loopback", API: OpenAIChatCompletionsAPI,
				BaseURL: loopback.URL + "/v1/chat/completions", Auth: AuthModeNone,
				ResponseBytes: MaxProviderResponseBytes,
			}},
			Adapters: []DriverAdapterConfig{{
				OpenAI: &OpenAIProfileConfig{
					HTTPProfileConfig: HTTPProfileConfig{
						Key: "a-loopback", ID: "sworn.loopback", Version: "1.0.0",
					},
					Preset: "preset-loopback",
				},
			}},
			Profiles: []DriverProfile{{
				Key: "loopback", Adapter: "a-loopback",
				Network:             NetworkRequired,
				AuthMode:            authModePtr(AuthModeNone),
				CertificationModels: []string{"model-loopback"},
			}},
		}
	}

	// A zero-credential configuration whose only adapter is auth_mode=none
	// admits without inventing a placeholder secret.
	if _, err := EncodeDriverConfig(base()); err != nil {
		t.Fatalf("zero-credential none config error = %v", err)
	}

	// Explicit none with a credential reference is contradictory.
	withCredential := base()
	withCredential.Credentials = []DriverCredentialSource{{
		Key: "none-env", Kind: CredentialEnvironment,
		Reference: "SWORN_TEST_TOKEN",
	}}
	withCredential.Profiles[0].CredentialSource = &noneEnv
	if _, err := EncodeDriverConfig(withCredential); !IsCode(err, "INVALID_DRIVER_CONFIG") {
		t.Fatalf("none profile with credential error = %v", err)
	}

	// Omitted auth mode with no credential source fails closed: none is never
	// obtained by omission.
	omitted := base()
	omitted.Profiles[0].AuthMode = nil
	if _, err := EncodeDriverConfig(omitted); !IsCode(err, "INVALID_DRIVER_CONFIG") {
		t.Fatalf("omitted auth mode error = %v", err)
	}

	// Explicit none on a bearer-only adapter fails closed.
	bearer := base()
	bearer.Presets[0].Auth = AuthModeBearer
	bearer.Presets[0].CredentialHeader = "Authorization"
	bearer.Presets[0].CredentialPrefix = "Bearer "
	if _, err := EncodeDriverConfig(bearer); !IsCode(err, "INVALID_DRIVER_CONFIG") {
		t.Fatalf("none profile against bearer adapter error = %v", err)
	}
}

// TestLegacyMantleAndDeepSeekConfigsMigrateExactly pins the recorded mapping
// byte-for-byte: deepseek and both mantle auth kinds rewrite into the unified
// openai adapter naming each field.
func TestLegacyMantleAndDeepSeekConfigsMigrateExactly(t *testing.T) {
	root := t.TempDir()
	awsPath := filepath.Join(root, "aws")
	if err := os.WriteFile(awsPath, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	runtimeFiles := driverRuntimeFilesFixture(t, root)
	required := make([]string, len(runtimeFiles))
	for index := range runtimeFiles {
		required[index] = runtimeFiles[index].Target
	}
	chain := AWSChainSpec{
		CLI:              ExecutableIdentity{Path: awsPath, Digest: AWSCLIDigest},
		CLIVersion:       AWSCLIVersion,
		Region:           "ap-southeast-2",
		RegionSource:     AWSSourceEnvironment,
		CredentialSource: AWSSourceEnvironment,
		EnvironmentKeys: []string{
			"AWS_ACCESS_KEY_ID", "AWS_DEFAULT_REGION",
			"AWS_REGION", "AWS_SECRET_ACCESS_KEY",
		},
		RuntimeFiles:           runtimeFiles,
		RequiredRuntimeTargets: required,
	}
	legacy := legacyDriverConfig{
		SchemaVersion: DriverConfigSchemaVersion,
		Credentials: []DriverCredentialSource{
			{Key: "aws", Kind: CredentialAWS, Reference: "AWS_RUNTIME_SOURCE"},
			{Key: "deepseek-env", Kind: CredentialEnvironment, Reference: "DEEPSEEK_API_KEY"},
			{Key: "mantle-env", Kind: CredentialEnvironment, Reference: "BEDROCK_MANTLE_API_KEY"},
		},
		Adapters: []legacyDriverAdapterConfig{
			{DeepSeek: &HTTPProfileConfig{
				Key: "a-deepseek", ID: "sworn.deepseek", Version: "1.0.0",
				Endpoint:         "https://api.example.invalid/chat/completions",
				CredentialHeader: "Authorization", CredentialPrefix: "Bearer ",
				CredentialRefs: []string{"deepseek-env"},
				ResponseBytes:  MaxProviderResponseBytes,
			}},
			{Mantle: &legacyMantleProfileConfig{
				Key: "a-mantle-api", ID: "sworn.mantle.api", Version: "1.0.0",
				Endpoint:       "https://api.example.invalid/v1/chat/completions",
				CredentialRefs: []string{"mantle-env"},
				ResponseBytes:  MaxProviderResponseBytes,
				AuthMode:       legacyMantleAPIKey,
			}},
			{Mantle: &legacyMantleProfileConfig{
				Key: "a-mantle-aws", ID: "sworn.mantle.aws", Version: "1.0.0",
				Endpoint:       "https://api.example.invalid/v1/chat/completions",
				CredentialRefs: []string{"aws"},
				ResponseBytes:  MaxProviderResponseBytes,
				AuthMode:       legacyMantleAWS,
				Chain:          &chain,
			}},
		},
		Profiles: []DriverProfile{
			{
				Key: "deepseek", Adapter: "a-deepseek",
				Network:             NetworkRequired,
				CredentialSource:    stringPtr("deepseek-env"),
				CertificationModels: []string{"model-deepseek"},
			},
			{
				Key: "mantle-api", Adapter: "a-mantle-api",
				Network:             NetworkRequired,
				CredentialSource:    stringPtr("mantle-env"),
				CertificationModels: []string{"model-mantle-api"},
			},
			{
				Key: "mantle-aws", Adapter: "a-mantle-aws",
				Network:             NetworkRequired,
				CredentialSource:    stringPtr("aws"),
				CertificationModels: []string{"model-mantle-aws"},
			},
		},
	}
	legacyBody, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := DecodeDriverConfig(legacyBody)
	if err != nil {
		t.Fatal(err)
	}

	opaque := true
	expected := DriverConfig{
		SchemaVersion: DriverConfigSchemaVersion,
		Credentials: []DriverCredentialSource{
			{Key: "aws", Kind: CredentialAWS, Reference: "AWS_RUNTIME_SOURCE"},
			{Key: "deepseek-env", Kind: CredentialEnvironment, Reference: "DEEPSEEK_API_KEY"},
			{Key: "mantle-env", Kind: CredentialEnvironment, Reference: "BEDROCK_MANTLE_API_KEY"},
		},
		Adapters: []DriverAdapterConfig{
			{OpenAI: &OpenAIProfileConfig{
				HTTPProfileConfig: HTTPProfileConfig{
					Key: "a-deepseek", ID: "sworn.deepseek", Version: "1.0.0",
					Endpoint:         "https://api.example.invalid/chat/completions",
					CredentialHeader: "Authorization", CredentialPrefix: "Bearer ",
					CredentialRefs: []string{"deepseek-env"},
					ResponseBytes:  MaxProviderResponseBytes,
				},
				API:             OpenAIChatCompletionsAPI,
				AuthMode:        AuthModeBearer,
				OpaqueReasoning: &opaque,
			}},
			{OpenAI: &OpenAIProfileConfig{
				HTTPProfileConfig: HTTPProfileConfig{
					Key: "a-mantle-api", ID: "sworn.mantle.api", Version: "1.0.0",
					Endpoint:         "https://api.example.invalid/v1/chat/completions",
					CredentialHeader: "Authorization", CredentialPrefix: "Bearer ",
					CredentialRefs: []string{"mantle-env"},
					ResponseBytes:  MaxProviderResponseBytes,
				},
				API:      OpenAIChatCompletionsAPI,
				AuthMode: AuthModeBearer,
			}},
			{OpenAI: &OpenAIProfileConfig{
				HTTPProfileConfig: HTTPProfileConfig{
					Key: "a-mantle-aws", ID: "sworn.mantle.aws", Version: "1.0.0",
					Endpoint:       "https://api.example.invalid/v1/chat/completions",
					CredentialRefs: []string{"aws"},
					ResponseBytes:  MaxProviderResponseBytes,
				},
				API:      OpenAIChatCompletionsAPI,
				AuthMode: AuthModeAWSSigV4,
				Chain:    &chain,
			}},
		},
		Profiles: []DriverProfile{
			{
				Key: "deepseek", Adapter: "a-deepseek",
				Network:             NetworkRequired,
				CredentialSource:    stringPtr("deepseek-env"),
				CertificationModels: []string{"model-deepseek"},
			},
			{
				Key: "mantle-api", Adapter: "a-mantle-api",
				Network:             NetworkRequired,
				CredentialSource:    stringPtr("mantle-env"),
				CertificationModels: []string{"model-mantle-api"},
			},
			{
				Key: "mantle-aws", Adapter: "a-mantle-aws",
				Network:             NetworkRequired,
				CredentialSource:    stringPtr("aws"),
				CertificationModels: []string{"model-mantle-aws"},
			},
		},
	}
	expectedBody, err := EncodeDriverConfig(expected)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(loaded.CanonicalJSON(), expectedBody) {
		t.Fatalf(
			"migrated canonical:\n%s\nwant:\n%s",
			loaded.CanonicalJSON(),
			expectedBody,
		)
	}
	canonical := loaded.CanonicalJSON()
	for _, marker := range []string{
		`"api":"chat_completions"`,
		`"auth_mode":"bearer"`,
		`"auth_mode":"aws_sigv4"`,
		`"opaque_reasoning":true`,
		`"credential_header":"Authorization"`,
		`"credential_prefix":"Bearer "`,
		`"chain":{`,
	} {
		if !bytes.Contains(canonical, []byte(marker)) {
			t.Fatalf("migrated canonical missing %s: %s", marker, canonical)
		}
	}
	for _, forbidden := range []string{
		`"deepseek":`, `"mantle":`, `"api_key_bearer"`, `"aws_chain_sigv4"`,
	} {
		if bytes.Contains(canonical, []byte(forbidden)) {
			t.Fatalf("migrated canonical retained legacy field %s: %s", forbidden, canonical)
		}
	}
}

// TestLegacyMigrationBuildsAndInspects proves a migrated configuration builds
// into a registry and reports through the production adapter surfaces.
func TestLegacyMigrationBuildsAndInspects(t *testing.T) {
	config := completeDriverConfigFixture(t)
	// The current fixture is already in the new shape; re-encode a legacy
	// variant by round-tripping through the shadow shape.
	legacy := legacyDriverConfig{
		SchemaVersion: config.SchemaVersion,
		Credentials:   config.Credentials,
		Profiles:      config.Profiles,
		Presets:       config.Presets,
	}
	for _, adapter := range config.Adapters {
		legacy.Adapters = append(legacy.Adapters, legacyDriverAdapterConfig{
			Process: adapter.Process,
			Native:  adapter.Native,
			OpenAI:  adapter.OpenAI,
			Gemini:  adapter.Gemini,
			Bedrock: adapter.Bedrock,
		})
	}
	legacyBody, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := DecodeDriverConfig(legacyBody)
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
	all, err := loaded.BuildAllRegistry(options)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Load() != 0 ||
		len(all.Profiles()) != len(config.Profiles) {
		t.Fatalf(
			"migrated all profiles=%v resolved=%d",
			all.Profiles(),
			resolved.Load(),
		)
	}
	chat := all.Inspect(context.Background(), "openai-chat", "model-openai-chat")
	opaque := all.Inspect(context.Background(), "openai-opaque", "model-openai-opaque")
	aws := all.Inspect(context.Background(), "openai-aws", "model-openai-aws")
	if chat.Family != ProfileOpenAIHTTP || chat.Surface != ProfileSurfaceOpenAIChat ||
		opaque.Family != ProfileOpenAIHTTP || opaque.Surface != ProfileSurfaceOpenAIChat ||
		aws.Family != ProfileOpenAIHTTP || aws.Surface != ProfileSurfaceOpenAIChat {
		t.Fatalf(
			"migrated reports chat=%#v opaque=%#v aws=%#v",
			chat,
			opaque,
			aws,
		)
	}
}

// TestProductionDriverFactoryRefusesUnavailableTempRoot is the A2 consumer
// proof for the host factory: an uncreatable configured temp root fails
// factory construction instead of silently creating the certification root in
// the process/system temp directory.
func TestProductionDriverFactoryRefusesUnavailableTempRoot(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(gitx.EnvTempRoot, filepath.Join(blocker, "tmp"))
	config := completeDriverConfigFixture(t)
	body, err := EncodeDriverConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := DecodeDriverConfig(body)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewProductionDriverFactory(loaded); err == nil {
		t.Fatal("uncreatable temp root silently escaped for the driver factory")
	}
}

// TestProductionDriverFactoryAdmitsMigratedAndPresetRegistries proves the
// host factory recognizes the unified aws_sigv4 chain (so migrated mantle-aws
// configurations construct) and the none-auth preset path (which carries no
// credential reference).
func TestProductionDriverFactoryAdmitsMigratedAndPresetRegistries(t *testing.T) {
	config := completeDriverConfigFixture(t)
	body, err := EncodeDriverConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := DecodeDriverConfig(body)
	if err != nil {
		t.Fatal(err)
	}
	factory, err := NewProductionDriverFactory(loaded)
	if err != nil {
		t.Fatal(err)
	}
	defer factory.Close()
	options := factory.Options()
	registry, err := loaded.BuildRegistry(
		[]string{"openai-aws", "openai-chat"},
		options,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(registry.Profiles()) != 2 {
		t.Fatalf("factory registry profiles = %v", registry.Profiles())
	}

	// The preset configuration with a none-auth loopback constructs too.
	loopback := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		writeJSONResponse(t, writer, openAIToolCallResponse(
			"factory-submit", "sworn_submit",
			presetSubmissionArguments(t, "preset-factory"),
			1, 1,
		))
	}))
	defer loopback.Close()
	presetConfig := DriverConfig{
		SchemaVersion: DriverConfigSchemaVersion,
		Presets: []DriverPreset{{
			Key: "preset-loopback", API: OpenAIChatCompletionsAPI,
			BaseURL: loopback.URL + "/v1/chat/completions", Auth: AuthModeNone,
			ResponseBytes: MaxProviderResponseBytes,
		}},
		Adapters: []DriverAdapterConfig{{
			OpenAI: &OpenAIProfileConfig{
				HTTPProfileConfig: HTTPProfileConfig{
					Key: "a-loopback", ID: "sworn.loopback", Version: "1.0.0",
				},
				Preset: "preset-loopback",
			},
		}},
		Profiles: []DriverProfile{{
			Key: "loopback", Adapter: "a-loopback",
			Network:             NetworkRequired,
			AuthMode:            authModePtr(AuthModeNone),
			CertificationModels: []string{"model-loopback"},
		}},
	}
	presetBody, err := EncodeDriverConfig(presetConfig)
	if err != nil {
		t.Fatal(err)
	}
	presetLoaded, err := DecodeDriverConfig(presetBody)
	if err != nil {
		t.Fatal(err)
	}
	presetFactory, err := NewProductionDriverFactory(presetLoaded)
	if err != nil {
		t.Fatal(err)
	}
	defer presetFactory.Close()
	presetRegistry, err := presetLoaded.BuildRegistry(
		[]string{"loopback"},
		presetFactory.Options(),
	)
	if err != nil {
		t.Fatal(err)
	}
	report := presetRegistry.Inspect(
		context.Background(),
		"loopback",
		"model-loopback",
	)
	if report.Family != ProfileOpenAIHTTP ||
		report.Surface != ProfileSurfaceOpenAIChat ||
		report.State != ReadinessPass {
		t.Fatalf("preset factory report = %#v", report)
	}
}

// TestLegacyFreeDocumentsKeepStrictCanonicalSemantics proves the migration
// path is byte-preserving for documents without legacy entries.
func TestLegacyFreeDocumentsKeepStrictCanonicalSemantics(t *testing.T) {
	config := completeDriverConfigFixture(t)
	body, err := EncodeDriverConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	noncanonical := append(append([]byte(nil), body...), '\n')
	if _, err := DecodeDriverConfig(noncanonical); !IsCode(err, "NONCANONICAL_JSON") {
		t.Fatalf("noncanonical error = %v", err)
	}
}

// TestProfileAuthFlavourPairingFailsClosedAtAdmission proves a profile cannot
// borrow an auth surface or protocol flavour its adapter does not offer.
func TestProfileAuthFlavourPairingFailsClosedAtAdmission(t *testing.T) {
	base := func() DriverConfig {
		return DriverConfig{
			SchemaVersion: DriverConfigSchemaVersion,
			Credentials: []DriverCredentialSource{
				{Key: "aws", Kind: CredentialAWS, Reference: "AWS_RUNTIME_SOURCE"},
				{Key: "env", Kind: CredentialEnvironment, Reference: "SWORN_TEST_TOKEN"},
			},
			Adapters: []DriverAdapterConfig{
				{OpenAI: &OpenAIProfileConfig{
					HTTPProfileConfig: HTTPProfileConfig{
						Key: "a-openai", ID: "sworn.openai", Version: "1.0.0",
						Endpoint:         "http://localhost:4110/v1/chat/completions",
						CredentialHeader: "Authorization", CredentialPrefix: "Bearer ",
						CredentialRefs: []string{"env"},
						ResponseBytes:  MaxProviderResponseBytes,
					},
					API: OpenAIChatCompletionsAPI,
				}},
				{Gemini: &HTTPProfileConfig{
					Key: "a-gemini", ID: "sworn.gemini", Version: "1.0.0",
					Endpoint:         "http://localhost:4111",
					CredentialHeader: "x-goog-api-key", CredentialPrefix: "",
					CredentialRefs: []string{"env"},
					ResponseBytes:  MaxProviderResponseBytes,
				}},
			},
			Profiles: []DriverProfile{{
				Key: "openai", Adapter: "a-openai",
				Network: NetworkRequired, CredentialSource: stringPtr("env"),
				CertificationModels: []string{"model-openai"},
			}},
		}
	}

	// A SigV4 profile against the bearer-only OpenAI adapter fails.
	awsSigV4 := base()
	awsSigV4.Profiles[0].AuthMode = authModePtr(AuthModeAWSSigV4)
	awsSigV4.Profiles[0].CredentialSource = stringPtr("aws")
	if _, err := EncodeDriverConfig(awsSigV4); !IsCode(err, "INVALID_DRIVER_CONFIG") {
		t.Fatalf("SigV4 profile against bearer adapter error = %v", err)
	}

	// An AWS credential against the Gemini bearer-only adapter fails.
	geminiAws := base()
	geminiAws.Profiles[0].Adapter = "a-gemini"
	geminiAws.Profiles[0].CredentialSource = stringPtr("aws")
	if _, err := EncodeDriverConfig(geminiAws); !IsCode(err, "INVALID_DRIVER_CONFIG") {
		t.Fatalf("AWS credential against Gemini adapter error = %v", err)
	}

	// An environment credential cannot feed a SigV4 adapter.
	sigV4Adapter := base()
	sigV4Adapter.Adapters[0].OpenAI.AuthMode = AuthModeAWSSigV4
	sigV4Adapter.Adapters[0].OpenAI.CredentialHeader = ""
	sigV4Adapter.Adapters[0].OpenAI.CredentialPrefix = ""
	sigV4Adapter.Adapters[0].OpenAI.Chain = &AWSChainSpec{}
	if _, err := EncodeDriverConfig(sigV4Adapter); !IsCode(err, "INVALID_DRIVER_CONFIG") {
		t.Fatalf("env credential against SigV4 adapter error = %v", err)
	}
}

// TestOpenAIConversationNeverEmitsAnotherProtocolShape proves the unified
// adapter owns only the OpenAI-compatible wire shapes.
func TestOpenAIConversationNeverEmitsAnotherProtocolShape(t *testing.T) {
	conversation, err := newOpenAIConversation(
		"https://api.example.invalid/v1/chat/completions",
		"exact-model",
		toolDefinitions(ReadOnly),
		[]byte(`{"prompt":"bounded"}`),
		providerDialectOpenAIChat,
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer conversation.close()
	request, err := conversation.request()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(request.Body, []byte(`"messages"`)) ||
		bytes.Contains(request.Body, []byte(`"contents"`)) ||
		bytes.Contains(request.Body, []byte(`"functionDeclarations"`)) ||
		bytes.Contains(request.Body, []byte(`"input"`)) {
		t.Fatalf("unified adapter emitted a foreign request shape: %s", request.Body)
	}
}

// TestEndpointTemplateVariablesResolveOnceAtAdmission proves a profile whose
// base URL carries workspace or region identifiers is expressible without
// call-time string surgery, and that unresolved, duplicate, or oversized
// variables fail closed at admission.
func TestEndpointTemplateVariablesResolveOnceAtAdmission(t *testing.T) {
	var requests atomic.Int64
	var receivedURL string
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		requests.Add(1)
		receivedURL = request.URL.String()
		writeJSONResponse(t, writer, openAIToolCallResponse(
			"template-submit", "sworn_submit",
			presetSubmissionArguments(t, "preset-template"),
			2, 2,
		))
	}))
	defer server.Close()
	host, port, _ := strings.Cut(strings.TrimPrefix(server.URL, "http://"), ":")

	config := DriverConfig{
		SchemaVersion: DriverConfigSchemaVersion,
		Variables: map[string]string{
			"host":      host,
			"port":      port,
			"workspace": "v1",
		},
		Presets: []DriverPreset{{
			Key: "preset-template", API: OpenAIChatCompletionsAPI,
			BaseURL:       "http://{host}:{port}/{workspace}/chat/completions",
			Auth:          AuthModeNone,
			ResponseBytes: MaxProviderResponseBytes,
		}},
		Adapters: []DriverAdapterConfig{{
			OpenAI: &OpenAIProfileConfig{
				HTTPProfileConfig: HTTPProfileConfig{
					Key: "a-template", ID: "sworn.template", Version: "1.0.0",
				},
				Preset: "preset-template",
			},
		}},
		Profiles: []DriverProfile{{
			Key: "template", Adapter: "a-template",
			Network: NetworkRequired, AuthMode: authModePtr(AuthModeNone),
			CertificationModels: []string{"model-template"},
		}},
	}
	body, err := EncodeDriverConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := DecodeDriverConfig(body)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := loaded.BuildRegistry(
		[]string{"template"},
		DriverFactoryOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := registry.ResolveSelection(
		ModelSelection{Profile: "template", Model: "model-template"},
	)
	if err != nil {
		t.Fatal(err)
	}
	invocation := presetInvocation(
		t,
		selected,
		"preset-template",
		RoleImplementer,
		ImplementerImplementation,
		ReadWrite,
	)
	if _, err := (Dispatcher{}).Invoke(context.Background(), invocation); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 1 {
		t.Fatalf("template provider requests = %d", requests.Load())
	}
	expected := "/v1/chat/completions"
	if receivedURL != expected {
		t.Fatalf("resolved request URL = %q, want %q", receivedURL, expected)
	}

	// Unresolved placeholder fails closed.
	unresolved := config
	unresolved.Variables = map[string]string{"host": host, "port": port}
	if _, err := EncodeDriverConfig(unresolved); !IsCode(err, "INVALID_ENDPOINT") {
		t.Fatalf("unresolved variable error = %v", err)
	}

	// Duplicate placeholder in one template fails closed.
	duplicate := config
	duplicate.Presets[0].BaseURL = "http://{host}:{port}/{workspace}/{workspace}/chat/completions"
	if _, err := EncodeDriverConfig(duplicate); !IsCode(err, "INVALID_ENDPOINT") {
		t.Fatalf("duplicate placeholder error = %v", err)
	}

	// Oversized declared variable fails closed.
	oversized := config
	oversized.Variables = map[string]string{
		"host": host, "port": port, "workspace": strings.Repeat("x", 129),
	}
	if _, err := EncodeDriverConfig(oversized); !IsCode(err, "INVALID_DRIVER_CONFIG") {
		t.Fatalf("oversized variable error = %v", err)
	}

	// A template resolving to a non-absolute value fails closed.
	relative := config
	relative.Presets[0].BaseURL = "{host}"
	if _, err := EncodeDriverConfig(relative); !IsCode(err, "INVALID_ENDPOINT") {
		t.Fatalf("relative resolved endpoint error = %v", err)
	}

	// A resolved query-carrying endpoint fails closed.
	query := config
	query.Presets[0].BaseURL = "http://{host}:{port}/{workspace}/chat/completions?api=1"
	if _, err := EncodeDriverConfig(query); !IsCode(err, "INVALID_ENDPOINT") {
		t.Fatalf("query-carrying resolved endpoint error = %v", err)
	}
}

func stringPtr(value string) *string {
	return &value
}
