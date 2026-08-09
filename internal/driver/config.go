package driver

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
)

const (
	DriverConfigSchemaVersion = "sworn.driver-config/v1"
	MaxDriverConfigBytes      = 1_048_576
)

var environmentNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)

// DriverConfig is the canonical, secret-free configured-driver document.
// Credential values are resolved lazily and never enter this document.
// Presets express OpenAI-compatible providers as configuration only; Variables
// bound the named endpoint-template placeholders used by preset base URLs and
// adapter endpoints.
type DriverConfig struct {
	SchemaVersion string                   `json:"schema_version"`
	Credentials   []DriverCredentialSource `json:"credentials"`
	Adapters      []DriverAdapterConfig    `json:"adapters"`
	Profiles      []DriverProfile          `json:"profiles"`
	Presets       []DriverPreset           `json:"presets,omitempty"`
	Variables     map[string]string        `json:"variables,omitempty"`
}

// DriverPreset is one configuration-only OpenAI-compatible provider. A vendor
// name may appear in a preset's identity but never in the type system or in
// control flow; the adapter union stays keyed by wire protocol.
type DriverPreset struct {
	Key              string    `json:"key"`
	API              OpenAIAPI `json:"api"`
	BaseURL          string    `json:"base_url"`
	Auth             AuthMode  `json:"auth"`
	CredentialHeader string    `json:"credential_header"`
	CredentialPrefix string    `json:"credential_prefix"`
	ResponseBytes    int       `json:"response_bytes"`
	ReasoningEfforts []string  `json:"reasoning_efforts,omitempty"`
	OpaqueReasoning  bool      `json:"opaque_reasoning,omitempty"`
}

type CredentialSourceKind string

const (
	CredentialEnvironment CredentialSourceKind = "environment"
	CredentialFile        CredentialSourceKind = "file"
	CredentialAWS         CredentialSourceKind = "aws"
)

type DriverCredentialSource struct {
	Key       string               `json:"key"`
	Kind      CredentialSourceKind `json:"kind"`
	Reference string               `json:"reference"`
}

type DriverProcessAdapterConfig struct {
	Key        string             `json:"key"`
	ID         string             `json:"id"`
	Version    string             `json:"version"`
	Executable ExecutableIdentity `json:"executable"`
}

// DriverAdapterConfig is a closed union keyed by wire protocol over the
// existing adapter constructors. Exactly one field is present. Endpoints are
// exact POST URLs, never base URLs.
type DriverAdapterConfig struct {
	Process *DriverProcessAdapterConfig `json:"process,omitempty"`
	Native  *NativeAdapterConfig        `json:"native,omitempty"`
	OpenAI  *OpenAIProfileConfig        `json:"openai,omitempty"`
	Gemini  *HTTPProfileConfig          `json:"gemini,omitempty"`
	Bedrock *BedrockProfileConfig       `json:"bedrock,omitempty"`
}

type DriverProfile struct {
	Key                 string        `json:"key"`
	Adapter             string        `json:"adapter"`
	Network             NetworkPolicy `json:"network"`
	AuthMode            *AuthMode     `json:"auth_mode,omitempty"`
	CredentialSource    *string       `json:"credential_source"`
	CertificationModels []string      `json:"certification_models"`
}

type ProfileCertification struct {
	Profile string         `json:"profile"`
	Model   string         `json:"model"`
	Family  ProfileFamily  `json:"family"`
	Surface ProfileSurface `json:"surface,omitempty"`
}

type LoadedDriverConfig struct {
	config    DriverConfig
	canonical []byte
	digest    string
}

// DriverFactoryOptions supplies process-local credential access and live-test
// seams. Nil access fails closed only when that exact source is used. Registry
// construction never calls a credential resolver.
type DriverFactoryOptions struct {
	EnvironmentCredentials HeaderCredentialResolver
	FileCredentials        HeaderCredentialResolver
	AWSCredentials         AWSRuntimeResolver
	LiveProbes             map[string]ProfileLiveProbe
	RoundTrippers          map[string]http.RoundTripper
}

type ConfiguredDriverRegistry struct {
	SelectionRegistry
	configurationDigest string
	certifications      []ProfileCertification
}

type driverAdapterKind uint8

const (
	driverAdapterProcess driverAdapterKind = iota + 1
	driverAdapterNative
	driverAdapterOpenAI
	driverAdapterGemini
	driverAdapterBedrock
)

type driverAdapterDescriptor struct {
	kind    driverAdapterKind
	key     string
	id      string
	version string
	family  ProfileFamily
	surface ProfileSurface
	refs    []string
	sources []CredentialSourceKind
	auth    AuthMode
}

func EncodeDriverConfig(config DriverConfig) ([]byte, error) {
	if err := validateDriverConfig(config); err != nil {
		return nil, err
	}
	return canonicalJSON(config)
}

func DecodeDriverConfig(body []byte) (LoadedDriverConfig, error) {
	var config DriverConfig
	if _, err := decodeTyped(
		body,
		MaxDriverConfigBytes,
		[]string{"schema_version", "credentials", "adapters", "profiles"},
		[]string{"presets", "variables"},
		&config,
	); err != nil {
		// The new closed union rejects vendor-named adapter kinds before the
		// production decode can run. When the document instead parses through
		// the legacy-aware shadow shape, migrate it deterministically; the
		// migrated canonical is the new identity. Byte-preserving semantics
		// are untouched for every document without legacy entries.
		legacy, legacyErr := decodeLegacyDriverConfig(body)
		if legacyErr != nil {
			return LoadedDriverConfig{}, err
		}
		migrated, migrationErr := migrateDriverConfig(legacy)
		if migrationErr != nil {
			return LoadedDriverConfig{}, migrationErr
		}
		canonical, encodeErr := EncodeDriverConfig(migrated)
		if encodeErr != nil {
			return LoadedDriverConfig{}, encodeErr
		}
		return LoadedDriverConfig{
			config:    migrated,
			canonical: append([]byte(nil), canonical...),
			digest:    Digest(canonical),
		}, nil
	}
	canonical, err := EncodeDriverConfig(config)
	if err != nil {
		return LoadedDriverConfig{}, err
	}
	if !bytes.Equal(canonical, body) {
		return LoadedDriverConfig{}, fail("NONCANONICAL_JSON")
	}
	return LoadedDriverConfig{
		config:    config,
		canonical: append([]byte(nil), canonical...),
		digest:    Digest(canonical),
	}, nil
}

func LoadDriverConfig(pathValue string) (LoadedDriverConfig, error) {
	if pathValue == "" || !filepath.IsAbs(pathValue) ||
		filepath.Clean(pathValue) != pathValue {
		return LoadedDriverConfig{}, fail("INVALID_CONFIG_PATH")
	}
	file, err := os.Open(pathValue)
	if err != nil {
		return LoadedDriverConfig{}, fail("CONFIG_UNAVAILABLE")
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, MaxDriverConfigBytes+1))
	if err != nil {
		clearBytes(body)
		return LoadedDriverConfig{}, fail("CONFIG_UNAVAILABLE")
	}
	defer clearBytes(body)
	if len(body) > MaxDriverConfigBytes {
		return LoadedDriverConfig{}, fail("RESOURCE_LIMIT")
	}
	return DecodeDriverConfig(body)
}

func (loaded LoadedDriverConfig) ConfigurationDigest() string {
	return loaded.digest
}

func (loaded LoadedDriverConfig) CanonicalJSON() []byte {
	return append([]byte(nil), loaded.canonical...)
}

func (registry ConfiguredDriverRegistry) ConfigurationDigest() string {
	return registry.configurationDigest
}

func (registry ConfiguredDriverRegistry) Certifications() []ProfileCertification {
	return append([]ProfileCertification(nil), registry.certifications...)
}

// BuildRegistry constructs only an explicit non-empty profile subset.
func (loaded LoadedDriverConfig) BuildRegistry(
	profiles []string,
	options DriverFactoryOptions,
) (ConfiguredDriverRegistry, error) {
	if len(profiles) == 0 {
		return ConfiguredDriverRegistry{}, fail("MISSING_PROFILE")
	}
	selected := append([]string(nil), profiles...)
	sort.Strings(selected)
	for index, profile := range selected {
		if !providerKeyPattern.MatchString(profile) {
			return ConfiguredDriverRegistry{}, fail("INVALID_PROFILE")
		}
		if index > 0 && selected[index-1] == profile {
			return ConfiguredDriverRegistry{}, fail("DUPLICATE_PROFILE")
		}
	}
	return loaded.build(selected, options, false)
}

// BuildAllRegistry constructs all profiles and requires every production
// family plus the Bedrock Converse surface.
func (loaded LoadedDriverConfig) BuildAllRegistry(
	options DriverFactoryOptions,
) (ConfiguredDriverRegistry, error) {
	profiles := make([]string, len(loaded.config.Profiles))
	for index := range loaded.config.Profiles {
		profiles[index] = loaded.config.Profiles[index].Key
	}
	return loaded.build(profiles, options, true)
}

func (loaded LoadedDriverConfig) build(
	selectedKeys []string,
	options DriverFactoryOptions,
	requireAll bool,
) (ConfiguredDriverRegistry, error) {
	if !digestPattern.MatchString(loaded.digest) ||
		validateDriverConfig(loaded.config) != nil {
		return ConfiguredDriverRegistry{}, fail("INVALID_DRIVER_CONFIG")
	}
	credentials := credentialSourceMap(loaded.config.Credentials)
	presets, err := presetMap(loaded.config.Presets)
	if err != nil {
		return ConfiguredDriverRegistry{}, err
	}
	adapters, err := describeAdapters(loaded.config)
	if err != nil {
		return ConfiguredDriverRegistry{}, err
	}
	profiles := profileConfigMap(loaded.config.Profiles)
	neededAdapters := make(map[string]struct{})
	profileConfigs := make([]ProfileConfig, 0, len(selectedKeys))
	var certifications []ProfileCertification
	for _, key := range selectedKeys {
		profile, ok := profiles[key]
		if !ok {
			return ConfiguredDriverRegistry{}, fail("UNKNOWN_PROFILE")
		}
		descriptor := adapters[profile.Adapter]
		neededAdapters[profile.Adapter] = struct{}{}
		profileConfigs = append(profileConfigs, ProfileConfig{
			Key:           profile.Key,
			Adapter:       profile.Adapter,
			Network:       profile.Network,
			AuthMode:      descriptor.auth,
			CredentialRef: cloneString(profile.CredentialSource),
		})
		for _, model := range profile.CertificationModels {
			certifications = append(certifications, ProfileCertification{
				Profile: profile.Key,
				Model:   model,
				Family:  descriptor.family,
				Surface: descriptor.surface,
			})
		}
	}

	built := make([]Adapter, 0, len(neededAdapters))
	for _, raw := range loaded.config.Adapters {
		resolved, resolveErr := resolvePreset(
			raw,
			presets,
			loaded.config.Variables,
		)
		if resolveErr != nil {
			return ConfiguredDriverRegistry{}, resolveErr
		}
		descriptor, descriptorErr := resolved.descriptor()
		if descriptorErr != nil {
			return ConfiguredDriverRegistry{}, descriptorErr
		}
		if _, needed := neededAdapters[descriptor.key]; !needed {
			continue
		}
		adapter, buildErr := buildConfiguredAdapter(
			resolved,
			descriptor,
			credentials,
			options,
		)
		if buildErr != nil {
			return ConfiguredDriverRegistry{}, buildErr
		}
		built = append(built, adapter)
	}
	var registry SelectionRegistry
	if requireAll {
		registry, err = NewProductionRegistry(profileConfigs, built)
	} else {
		registry, err = NewSelectionRegistry(profileConfigs, built)
	}
	if err != nil {
		return ConfiguredDriverRegistry{}, err
	}
	return ConfiguredDriverRegistry{
		SelectionRegistry:   registry,
		configurationDigest: loaded.digest,
		certifications:      certifications,
	}, nil
}

func buildConfiguredAdapter(
	config DriverAdapterConfig,
	descriptor driverAdapterDescriptor,
	sources map[string]DriverCredentialSource,
	options DriverFactoryOptions,
) (Adapter, error) {
	probe := options.LiveProbes[descriptor.key]
	roundTripper := options.RoundTrippers[descriptor.key]
	switch descriptor.kind {
	case driverAdapterProcess:
		value := config.Process
		return NewProcessAdapter(
			value.Key, value.ID, value.Version, value.Executable,
		)
	case driverAdapterNative:
		value := cloneNativeAdapterConfig(*config.Native)
		return NewNativeAdapter(
			value,
			filePathResolver(sources),
		)
	case driverAdapterOpenAI:
		value := cloneOpenAIProfileConfig(*config.OpenAI)
		return NewOpenAIAdapter(
			value,
			headerSourceResolver(sources, options),
			awsSourceResolver(sources, options),
			probe,
			roundTripper,
		)
	case driverAdapterGemini:
		value := cloneHTTPProfileConfig(*config.Gemini)
		return NewGeminiAdapter(
			value,
			headerSourceResolver(sources, options),
			probe,
			roundTripper,
		)
	case driverAdapterBedrock:
		return NewBedrockAdapter(
			cloneBedrockProfileConfig(*config.Bedrock),
			awsSourceResolver(sources, options),
			probe,
			roundTripper,
		)
	default:
		return nil, fail("INVALID_DRIVER_CONFIG")
	}
}

func headerSourceResolver(
	sources map[string]DriverCredentialSource,
	options DriverFactoryOptions,
) HeaderCredentialResolver {
	return func(ctx context.Context, ref string) ([]byte, error) {
		source, ok := sources[ref]
		if !ok {
			return nil, fail("CREDENTIAL_UNAVAILABLE")
		}
		switch source.Kind {
		case CredentialEnvironment:
			if options.EnvironmentCredentials != nil {
				return options.EnvironmentCredentials(ctx, source.Reference)
			}
		case CredentialFile:
			if options.FileCredentials != nil {
				return options.FileCredentials(ctx, source.Reference)
			}
		}
		return nil, fail("CREDENTIAL_UNAVAILABLE")
	}
}

func filePathResolver(
	sources map[string]DriverCredentialSource,
) FileCredentialResolver {
	return func(_ context.Context, ref string) (string, error) {
		source, ok := sources[ref]
		if !ok || source.Kind != CredentialFile {
			return "", fail("CREDENTIAL_UNAVAILABLE")
		}
		return source.Reference, nil
	}
}

func awsSourceResolver(
	sources map[string]DriverCredentialSource,
	options DriverFactoryOptions,
) AWSRuntimeResolver {
	return func(ctx context.Context, ref string) ([][]byte, error) {
		source, ok := sources[ref]
		if !ok || source.Kind != CredentialAWS ||
			options.AWSCredentials == nil {
			return nil, fail("CREDENTIAL_UNAVAILABLE")
		}
		return options.AWSCredentials(ctx, source.Reference)
	}
}

func validateDriverConfig(config DriverConfig) error {
	if config.SchemaVersion != DriverConfigSchemaVersion ||
		len(config.Adapters) == 0 ||
		len(config.Profiles) == 0 {
		return fail("INVALID_DRIVER_CONFIG")
	}
	if err := validateConfigVariables(config.Variables); err != nil {
		return err
	}
	if err := validatePresets(config.Presets, config.Variables); err != nil {
		return err
	}
	credentials := make(map[string]DriverCredentialSource, len(config.Credentials))
	previous := ""
	for _, source := range config.Credentials {
		if !providerKeyPattern.MatchString(source.Key) ||
			(previous != "" && source.Key <= previous) ||
			validateCredentialSource(source) != nil {
			return fail("INVALID_DRIVER_CONFIG")
		}
		previous = source.Key
		credentials[source.Key] = source
	}
	adapters, err := describeAdapters(config)
	if err != nil {
		return err
	}
	usedAdapters := make(map[string]struct{})
	usedCredentials := make(map[string]struct{})
	usedRefs := make(map[string]map[string]struct{})
	previous = ""
	for _, profile := range config.Profiles {
		descriptor, ok := adapters[profile.Adapter]
		if !ok || !providerKeyPattern.MatchString(profile.Key) ||
			(previous != "" && profile.Key <= previous) ||
			validateDriverProfile(profile, descriptor, credentials) != nil {
			return fail("INVALID_DRIVER_CONFIG")
		}
		previous = profile.Key
		usedAdapters[profile.Adapter] = struct{}{}
		if profile.CredentialSource != nil {
			usedCredentials[*profile.CredentialSource] = struct{}{}
			if usedRefs[profile.Adapter] == nil {
				usedRefs[profile.Adapter] = make(map[string]struct{})
			}
			usedRefs[profile.Adapter][*profile.CredentialSource] = struct{}{}
		}
	}
	if len(usedAdapters) != len(adapters) ||
		len(usedCredentials) != len(credentials) {
		return fail("INVALID_DRIVER_CONFIG")
	}
	for key, descriptor := range adapters {
		if len(descriptor.refs) != len(usedRefs[key]) {
			return fail("INVALID_DRIVER_CONFIG")
		}
		for _, ref := range descriptor.refs {
			if _, used := usedRefs[key][ref]; !used {
				return fail("INVALID_DRIVER_CONFIG")
			}
		}
	}
	return nil
}

// validatePresets admits a closed, canonical preset list. Static fields are
// checked here; BaseURL templates are resolved and admitted here too, so an
// unreferenced preset still cannot smuggle an invalid endpoint into a
// document. Presets are configuration only: no vendor name enters control flow.
func validatePresets(presets []DriverPreset, variables map[string]string) error {
	previous := ""
	for _, preset := range presets {
		if !providerKeyPattern.MatchString(preset.Key) ||
			(previous != "" && preset.Key <= previous) {
			return fail("INVALID_DRIVER_CONFIG")
		}
		previous = preset.Key
		if !preset.API.valid() ||
			!preset.Auth.valid() ||
			!validReasoningEfforts(preset.ReasoningEfforts) ||
			preset.ResponseBytes < 1 ||
			preset.ResponseBytes > MaxProviderResponseBytes ||
			(preset.Auth == AuthModeBearer &&
				(!httpToken(preset.CredentialHeader) ||
					len(preset.CredentialPrefix) > 64)) ||
			(preset.Auth != AuthModeBearer &&
				(preset.CredentialHeader != "" ||
					preset.CredentialPrefix != "")) {
			return fail("INVALID_DRIVER_CONFIG")
		}
		resolved, err := resolveEndpointTemplate(preset.BaseURL, variables)
		if err != nil || !validResolvedEndpoint(resolved) {
			return fail("INVALID_ENDPOINT")
		}
	}
	return nil
}

// validResolvedEndpoint admits a fully resolved exact POST URL: absolute,
// loopback-or-https, and free of query strings and fragments.
func validResolvedEndpoint(value string) bool {
	parsed, err := url.Parse(value)
	if validateEndpoint(value) != nil || err != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	return true
}

func presetMap(presets []DriverPreset) (map[string]DriverPreset, error) {
	result := make(map[string]DriverPreset, len(presets))
	for _, preset := range presets {
		if _, duplicate := result[preset.Key]; duplicate {
			return nil, fail("INVALID_DRIVER_CONFIG")
		}
		result[preset.Key] = preset
	}
	return result, nil
}

// resolvePreset turns an adapter document into its effective configuration
// exactly once at admission. An OpenAI adapter that names a preset inherits
// the preset's wire defaults; per-adapter fields override. Both the inherited
// preset base URL and an adapter's own endpoint are resolved from the declared
// document variables into exact absolute URLs before validation.
func resolvePreset(
	config DriverAdapterConfig,
	presets map[string]DriverPreset,
	variables map[string]string,
) (DriverAdapterConfig, error) {
	if config.OpenAI == nil {
		return config, nil
	}
	clone := cloneOpenAIProfileConfig(*config.OpenAI)
	if clone.Preset != "" {
		preset, ok := presets[clone.Preset]
		if !ok {
			return DriverAdapterConfig{}, fail("UNKNOWN_PRESET")
		}
		baseURL, err := resolveEndpointTemplate(preset.BaseURL, variables)
		if err != nil || !validResolvedEndpoint(baseURL) {
			return DriverAdapterConfig{}, fail("INVALID_ENDPOINT")
		}
		if clone.Endpoint == "" {
			clone.Endpoint = baseURL
		}
		if clone.API == "" {
			clone.API = preset.API
		}
		if clone.AuthMode == "" {
			clone.AuthMode = preset.Auth
		}
		if clone.CredentialHeader == "" {
			clone.CredentialHeader = preset.CredentialHeader
		}
		if clone.CredentialPrefix == "" {
			clone.CredentialPrefix = preset.CredentialPrefix
		}
		if clone.ResponseBytes == 0 {
			clone.ResponseBytes = preset.ResponseBytes
		}
		if len(clone.ReasoningEfforts) == 0 {
			clone.ReasoningEfforts = append(
				[]string(nil),
				preset.ReasoningEfforts...,
			)
		}
		if clone.OpaqueReasoning == nil {
			value := preset.OpaqueReasoning
			clone.OpaqueReasoning = &value
		}
	}
	if strings.Contains(clone.Endpoint, "{") {
		resolved, err := resolveEndpointTemplate(clone.Endpoint, variables)
		if err != nil {
			return DriverAdapterConfig{}, err
		}
		clone.Endpoint = resolved
	}
	config.OpenAI = &clone
	return config, nil
}

func describeAdapters(
	config DriverConfig,
) (map[string]driverAdapterDescriptor, error) {
	presets, err := presetMap(config.Presets)
	if err != nil {
		return nil, err
	}
	result := make(map[string]driverAdapterDescriptor, len(config.Adapters))
	previous := ""
	for _, raw := range config.Adapters {
		resolved, resolveErr := resolvePreset(raw, presets, config.Variables)
		if resolveErr != nil {
			return nil, resolveErr
		}
		descriptor, descriptorErr := resolved.descriptor()
		if descriptorErr != nil ||
			(previous != "" && descriptor.key <= previous) {
			return nil, fail("INVALID_DRIVER_CONFIG")
		}
		if _, duplicate := result[descriptor.key]; duplicate {
			return nil, fail("INVALID_DRIVER_CONFIG")
		}
		previous = descriptor.key
		result[descriptor.key] = descriptor
	}
	return result, nil
}

func (config DriverAdapterConfig) descriptor() (driverAdapterDescriptor, error) {
	var descriptors []driverAdapterDescriptor
	if config.Process != nil {
		descriptors = append(descriptors, driverAdapterDescriptor{
			kind: driverAdapterProcess, key: config.Process.Key,
			id: config.Process.ID, version: config.Process.Version,
			family: ProfileFake,
		})
	}
	if config.Native != nil {
		descriptors = append(descriptors, driverAdapterDescriptor{
			kind: driverAdapterNative, key: config.Native.Key,
			id: config.Native.ID, version: config.Native.Version,
			family: config.Native.Family, refs: config.Native.CredentialRefs,
			sources: []CredentialSourceKind{CredentialFile},
		})
	}
	if config.OpenAI != nil {
		surface := ProfileSurfaceOpenAIChat
		switch config.OpenAI.API {
		case OpenAIResponsesAPI:
			surface = ProfileSurfaceOpenAIResponses
		case OpenRouterChatCompletionsAPI:
			surface = ProfileSurfaceOpenRouterChat
		}
		auth := config.OpenAI.effectiveAuth()
		var sources []CredentialSourceKind
		switch auth {
		case AuthModeBearer:
			sources = []CredentialSourceKind{
				CredentialEnvironment,
				CredentialFile,
			}
		case AuthModeAWSSigV4:
			sources = []CredentialSourceKind{CredentialAWS}
		}
		descriptors = append(descriptors, driverAdapterDescriptor{
			kind: driverAdapterOpenAI, key: config.OpenAI.Key,
			id: config.OpenAI.ID, version: config.OpenAI.Version,
			family:  ProfileOpenAIHTTP,
			surface: surface,
			refs:    config.OpenAI.CredentialRefs,
			sources: sources,
			auth:    auth,
		})
	}
	if config.Gemini != nil {
		descriptors = append(descriptors, driverAdapterDescriptor{
			kind: driverAdapterGemini, key: config.Gemini.Key,
			id: config.Gemini.ID, version: config.Gemini.Version,
			family: ProfileGemini, refs: config.Gemini.CredentialRefs,
			sources: []CredentialSourceKind{
				CredentialEnvironment,
				CredentialFile,
			},
			auth: AuthModeBearer,
		})
	}
	if config.Bedrock != nil {
		descriptors = append(descriptors, driverAdapterDescriptor{
			kind: driverAdapterBedrock, key: config.Bedrock.Key,
			id: config.Bedrock.ID, version: config.Bedrock.Version,
			family:  ProfileBedrock,
			surface: ProfileSurfaceBedrockRuntimeConverse,
			refs:    config.Bedrock.CredentialRefs,
			sources: []CredentialSourceKind{CredentialAWS},
			auth:    AuthModeAWSSigV4,
		})
	}
	if len(descriptors) != 1 {
		return driverAdapterDescriptor{}, fail("INVALID_ADAPTER")
	}
	descriptor := descriptors[0]
	if !providerKeyPattern.MatchString(descriptor.key) ||
		!driverIdentityPattern.MatchString(descriptor.id) ||
		!versionPattern.MatchString(descriptor.version) ||
		!descriptor.family.valid() ||
		!descriptor.surface.validFor(descriptor.family) ||
		(descriptor.kind == driverAdapterProcess &&
			descriptor.id != FakeDriverID) ||
		(descriptor.kind == driverAdapterNative &&
			descriptor.family != ProfileCodex &&
			descriptor.family != ProfileClaude) ||
		(descriptor.kind != driverAdapterProcess &&
			descriptor.auth != AuthModeNone &&
			!validCredentialRefs(descriptor.refs)) ||
		(descriptor.kind == driverAdapterProcess &&
			len(descriptor.refs) != 0) ||
		(descriptor.kind == driverAdapterOpenAI &&
			(!config.OpenAI.valid() || !validOpenAIAuth(config.OpenAI))) ||
		validateAdapterDocumentEndpoint(config, descriptor.kind) != nil {
		return driverAdapterDescriptor{}, fail("INVALID_ADAPTER")
	}
	return descriptor, nil
}

func validateAdapterDocumentEndpoint(
	config DriverAdapterConfig,
	kind driverAdapterKind,
) error {
	endpoint := ""
	switch kind {
	case driverAdapterOpenAI:
		endpoint = config.OpenAI.Endpoint
	case driverAdapterGemini:
		endpoint = config.Gemini.Endpoint
	case driverAdapterBedrock:
		endpoint = config.Bedrock.Endpoint
	default:
		return nil
	}
	parsed, err := url.Parse(endpoint)
	if validateEndpoint(endpoint) != nil || err != nil || parsed.RawQuery != "" {
		return fail("INVALID_ENDPOINT")
	}
	return nil
}

func validCredentialRefs(refs []string) bool {
	if len(refs) == 0 {
		return false
	}
	for index, ref := range refs {
		if !providerKeyPattern.MatchString(ref) ||
			(index > 0 && refs[index-1] >= ref) {
			return false
		}
	}
	return true
}

func validateCredentialSource(source DriverCredentialSource) error {
	switch source.Kind {
	case CredentialEnvironment:
		if !environmentNamePattern.MatchString(source.Reference) {
			return fail("INVALID_CREDENTIAL_SOURCE")
		}
	case CredentialFile:
		if source.Reference == "" ||
			!filepath.IsAbs(source.Reference) ||
			filepath.Clean(source.Reference) != source.Reference {
			return fail("INVALID_CREDENTIAL_SOURCE")
		}
	case CredentialAWS:
		if !providerKeyPattern.MatchString(source.Reference) {
			return fail("INVALID_CREDENTIAL_SOURCE")
		}
	default:
		return fail("INVALID_CREDENTIAL_SOURCE")
	}
	return nil
}

func validateDriverProfile(
	profile DriverProfile,
	adapter driverAdapterDescriptor,
	credentials map[string]DriverCredentialSource,
) error {
	if !providerKeyPattern.MatchString(profile.Adapter) ||
		len(profile.CertificationModels) == 0 {
		return fail("INVALID_PROFILE")
	}
	for index, model := range profile.CertificationModels {
		if validateText(model, 500, false) != nil ||
			(index > 0 && profile.CertificationModels[index-1] >= model) {
			return fail("INVALID_MODEL")
		}
	}
	if adapter.family == ProfileFake {
		if profile.Network != NetworkNone || profile.CredentialSource != nil {
			return fail("INVALID_PROFILE")
		}
		return nil
	}
	if profile.Network != NetworkRequired {
		return fail("INVALID_PROFILE")
	}
	// Native and process adapters carry file or no credentials and are exempt
	// from the HTTP auth-mode cross-check, so fixtures that omit auth_mode and
	// use file credentials admit unchanged.
	if adapter.kind == driverAdapterNative {
		if profile.CredentialSource == nil ||
			!slices.Contains(adapter.refs, *profile.CredentialSource) {
			return fail("INVALID_PROFILE")
		}
		source, ok := credentials[*profile.CredentialSource]
		if !ok {
			return fail("UNKNOWN_CREDENTIAL_SOURCE")
		}
		if !slices.Contains(adapter.sources, source.Kind) {
			return fail("INVALID_CREDENTIAL_SOURCE")
		}
		return nil
	}
	// Network adapters admit an explicit per-profile auth mode. Omission keeps
	// the legacy deterministic default derived from the credential kind, and a
	// credential-less profile without an explicit none mode fails closed.
	effective := AuthMode("")
	if profile.AuthMode != nil {
		effective = *profile.AuthMode
		if !effective.valid() {
			return fail("INVALID_PROFILE")
		}
	} else if profile.CredentialSource != nil {
		source, ok := credentials[*profile.CredentialSource]
		if !ok {
			return fail("UNKNOWN_CREDENTIAL_SOURCE")
		}
		if source.Kind == CredentialAWS {
			effective = AuthModeAWSSigV4
		} else {
			effective = AuthModeBearer
		}
	}
	if effective == "" || effective != adapter.auth {
		return fail("INVALID_PROFILE")
	}
	if effective == AuthModeNone {
		if profile.CredentialSource != nil {
			return fail("INVALID_PROFILE")
		}
		return nil
	}
	if profile.CredentialSource == nil ||
		!slices.Contains(adapter.refs, *profile.CredentialSource) {
		return fail("INVALID_PROFILE")
	}
	source, ok := credentials[*profile.CredentialSource]
	if !ok {
		return fail("UNKNOWN_CREDENTIAL_SOURCE")
	}
	if !slices.Contains(adapter.sources, source.Kind) {
		return fail("INVALID_CREDENTIAL_SOURCE")
	}
	return nil
}

func credentialSourceMap(
	sources []DriverCredentialSource,
) map[string]DriverCredentialSource {
	result := make(map[string]DriverCredentialSource, len(sources))
	for _, source := range sources {
		result[source.Key] = source
	}
	return result
}

func profileConfigMap(profiles []DriverProfile) map[string]DriverProfile {
	result := make(map[string]DriverProfile, len(profiles))
	for _, profile := range profiles {
		result[profile.Key] = profile
	}
	return result
}

func cloneNativeAdapterConfig(config NativeAdapterConfig) NativeAdapterConfig {
	config.RuntimeFiles = append([]PinnedRuntimeFile(nil), config.RuntimeFiles...)
	config.RequiredRuntimeTargets = append([]string(nil), config.RequiredRuntimeTargets...)
	config.CredentialRefs = append([]string(nil), config.CredentialRefs...)
	return config
}

func cloneHTTPProfileConfig(config HTTPProfileConfig) HTTPProfileConfig {
	config.CredentialRefs = append([]string(nil), config.CredentialRefs...)
	return config
}

func cloneOpenAIProfileConfig(config OpenAIProfileConfig) OpenAIProfileConfig {
	config.HTTPProfileConfig = cloneHTTPProfileConfig(config.HTTPProfileConfig)
	config.ReasoningEfforts = append([]string(nil), config.ReasoningEfforts...)
	if config.Chain != nil {
		chain := cloneAWSChainSpec(*config.Chain)
		config.Chain = &chain
	}
	if config.OpaqueReasoning != nil {
		value := *config.OpaqueReasoning
		config.OpaqueReasoning = &value
	}
	return config
}

func cloneAWSChainSpec(spec AWSChainSpec) AWSChainSpec {
	spec.EnvironmentKeys = append([]string(nil), spec.EnvironmentKeys...)
	spec.RuntimeFiles = append([]PinnedRuntimeFile(nil), spec.RuntimeFiles...)
	spec.RequiredRuntimeTargets = append([]string(nil), spec.RequiredRuntimeTargets...)
	return spec
}

func cloneBedrockProfileConfig(config BedrockProfileConfig) BedrockProfileConfig {
	config.CredentialRefs = append([]string(nil), config.CredentialRefs...)
	config.Chain = cloneAWSChainSpec(config.Chain)
	return config
}
