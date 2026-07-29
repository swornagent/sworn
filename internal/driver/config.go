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
)

const (
	DriverConfigSchemaVersion = "sworn.driver-config/v1"
	MaxDriverConfigBytes      = 1_048_576
)

var environmentNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)

// DriverConfig is the canonical, secret-free configured-driver document.
// Credential values are resolved lazily and never enter this document.
type DriverConfig struct {
	SchemaVersion string                   `json:"schema_version"`
	Credentials   []DriverCredentialSource `json:"credentials"`
	Adapters      []DriverAdapterConfig    `json:"adapters"`
	Profiles      []DriverProfile          `json:"profiles"`
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

// DriverAdapterConfig is a closed union over the existing adapter
// constructors. Exactly one field is present. A Mantle entry's Endpoint is
// the exact POST URL ending /v1/chat/completions, not a base URL.
type DriverAdapterConfig struct {
	Process  *DriverProcessAdapterConfig `json:"process,omitempty"`
	Native   *NativeAdapterConfig        `json:"native,omitempty"`
	OpenAI   *OpenAIProfileConfig        `json:"openai,omitempty"`
	DeepSeek *HTTPProfileConfig          `json:"deepseek,omitempty"`
	Gemini   *HTTPProfileConfig          `json:"gemini,omitempty"`
	Bedrock  *BedrockProfileConfig       `json:"bedrock,omitempty"`
	Mantle   *BedrockMantleProfileConfig `json:"mantle,omitempty"`
}

type DriverProfile struct {
	Key                 string        `json:"key"`
	Adapter             string        `json:"adapter"`
	Network             NetworkPolicy `json:"network"`
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
	NativeSmokeBuilders    map[string]NativeSmokeBuilder
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
	driverAdapterDeepSeek
	driverAdapterGemini
	driverAdapterBedrock
	driverAdapterMantle
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
		nil,
		&config,
	); err != nil {
		return LoadedDriverConfig{}, err
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

// BuildAllRegistry constructs all profiles and requires every existing family
// plus both Bedrock surfaces.
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
	adapters, err := describeAdapters(loaded.config.Adapters)
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
	for _, config := range loaded.config.Adapters {
		descriptor, err := config.descriptor()
		if err != nil {
			return ConfiguredDriverRegistry{}, err
		}
		if _, needed := neededAdapters[descriptor.key]; !needed {
			continue
		}
		adapter, err := buildConfiguredAdapter(config, descriptor, credentials, options)
		if err != nil {
			return ConfiguredDriverRegistry{}, err
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
			options.NativeSmokeBuilders[descriptor.key],
		)
	case driverAdapterOpenAI:
		value := cloneOpenAIProfileConfig(*config.OpenAI)
		return NewOpenAIAdapter(
			value,
			headerSourceResolver(sources, options),
			probe,
			roundTripper,
		)
	case driverAdapterDeepSeek, driverAdapterGemini:
		var source HTTPProfileConfig
		switch descriptor.kind {
		case driverAdapterDeepSeek:
			source = *config.DeepSeek
		case driverAdapterGemini:
			source = *config.Gemini
		}
		value := cloneHTTPProfileConfig(source)
		resolver := headerSourceResolver(sources, options)
		switch descriptor.kind {
		case driverAdapterDeepSeek:
			return NewDeepSeekAdapter(value, resolver, probe, roundTripper)
		default:
			return NewGeminiAdapter(value, resolver, probe, roundTripper)
		}
	case driverAdapterBedrock:
		return NewBedrockAdapter(
			cloneBedrockProfileConfig(*config.Bedrock),
			awsSourceResolver(sources, options),
			probe,
			roundTripper,
		)
	case driverAdapterMantle:
		value := cloneMantleProfileConfig(*config.Mantle)
		if value.AuthMode == BedrockMantleAPIKey {
			return NewBedrockMantleAdapter(
				value,
				headerSourceResolver(sources, options),
				nil,
				probe,
				roundTripper,
			)
		}
		return NewBedrockMantleAdapter(
			value,
			nil,
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
		len(config.Credentials) == 0 ||
		len(config.Adapters) == 0 ||
		len(config.Profiles) == 0 {
		return fail("INVALID_DRIVER_CONFIG")
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
	adapters, err := describeAdapters(config.Adapters)
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

func describeAdapters(
	configs []DriverAdapterConfig,
) (map[string]driverAdapterDescriptor, error) {
	result := make(map[string]driverAdapterDescriptor, len(configs))
	previous := ""
	for _, config := range configs {
		descriptor, err := config.descriptor()
		if err != nil || (previous != "" && descriptor.key <= previous) {
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
		if config.OpenAI.API == OpenAIResponsesAPI {
			surface = ProfileSurfaceOpenAIResponses
		}
		descriptors = append(descriptors, driverAdapterDescriptor{
			kind: driverAdapterOpenAI, key: config.OpenAI.Key,
			id: config.OpenAI.ID, version: config.OpenAI.Version,
			family:  ProfileOpenAIHTTP,
			surface: surface,
			refs:    config.OpenAI.CredentialRefs,
			sources: []CredentialSourceKind{
				CredentialEnvironment,
				CredentialFile,
			},
		})
	}
	for _, candidate := range []struct {
		config *HTTPProfileConfig
		kind   driverAdapterKind
		family ProfileFamily
	}{
		{config.DeepSeek, driverAdapterDeepSeek, ProfileDeepSeek},
		{config.Gemini, driverAdapterGemini, ProfileGemini},
	} {
		if candidate.config != nil {
			descriptors = append(descriptors, driverAdapterDescriptor{
				kind: candidate.kind, key: candidate.config.Key,
				id: candidate.config.ID, version: candidate.config.Version,
				family: candidate.family, refs: candidate.config.CredentialRefs,
				sources: []CredentialSourceKind{
					CredentialEnvironment,
					CredentialFile,
				},
			})
		}
	}
	if config.Bedrock != nil {
		descriptors = append(descriptors, driverAdapterDescriptor{
			kind: driverAdapterBedrock, key: config.Bedrock.Key,
			id: config.Bedrock.ID, version: config.Bedrock.Version,
			family:  ProfileBedrock,
			surface: ProfileSurfaceBedrockRuntimeConverse,
			refs:    config.Bedrock.CredentialRefs,
			sources: []CredentialSourceKind{CredentialAWS},
		})
	}
	if config.Mantle != nil {
		sources := []CredentialSourceKind{
			CredentialEnvironment,
			CredentialFile,
		}
		if config.Mantle.AuthMode == BedrockMantleAWS {
			sources = []CredentialSourceKind{CredentialAWS}
		}
		descriptors = append(descriptors, driverAdapterDescriptor{
			kind: driverAdapterMantle, key: config.Mantle.Key,
			id: config.Mantle.ID, version: config.Mantle.Version,
			family:  ProfileBedrock,
			surface: ProfileSurfaceBedrockMantleChat,
			refs:    config.Mantle.CredentialRefs, sources: sources,
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
			!validCredentialRefs(descriptor.refs)) ||
		(descriptor.kind == driverAdapterProcess &&
			len(descriptor.refs) != 0) ||
		(descriptor.kind == driverAdapterOpenAI &&
			!config.OpenAI.valid()) ||
		(descriptor.kind == driverAdapterMantle &&
			(!config.Mantle.AuthMode.valid() ||
				(config.Mantle.AuthMode == BedrockMantleAPIKey &&
					config.Mantle.Chain != nil) ||
				(config.Mantle.AuthMode == BedrockMantleAWS &&
					config.Mantle.Chain == nil))) ||
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
	case driverAdapterDeepSeek:
		endpoint = config.DeepSeek.Endpoint
	case driverAdapterGemini:
		endpoint = config.Gemini.Endpoint
	case driverAdapterBedrock:
		endpoint = config.Bedrock.Endpoint
	case driverAdapterMantle:
		return validateMantleEndpoint(config.Mantle.Endpoint)
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
	if profile.Network != NetworkRequired || profile.CredentialSource == nil ||
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

func cloneMantleProfileConfig(
	config BedrockMantleProfileConfig,
) BedrockMantleProfileConfig {
	config.CredentialRefs = append([]string(nil), config.CredentialRefs...)
	if config.Chain != nil {
		chain := cloneAWSChainSpec(*config.Chain)
		config.Chain = &chain
	}
	return config
}
