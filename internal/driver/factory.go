package driver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sync"
)

const (
	driverCertificationSchemaVersion = "sworn.driver-certification/v1"
	maxSystemCredentialBytes         = 65_536
)

// ProductionDriverFactory owns the process-local resources and callbacks used
// by both the runtime and the driver readiness CLI. It never discovers a
// profile, model, endpoint, credential source, or fallback: all of those facts
// come from the exact admitted DriverConfig.
type ProductionDriverFactory struct {
	options DriverFactoryOptions
	root    string

	closeOnce sync.Once
	closeErr  error
}

// NewProductionDriverFactory constructs the one host-backed factory for an
// exact canonical configuration. Credential values remain lazy and are never
// read while the factory or registry is constructed.
func NewProductionDriverFactory(
	loaded LoadedDriverConfig,
) (*ProductionDriverFactory, error) {
	body := loaded.CanonicalJSON()
	reloaded, err := DecodeDriverConfig(body)
	if err != nil || loaded.ConfigurationDigest() == "" ||
		reloaded.ConfigurationDigest() != loaded.ConfigurationDigest() {
		clearBytes(body)
		return nil, fail("INVALID_DRIVER_CONFIG")
	}
	clearBytes(body)

	temp, err := tempRoot()
	if err != nil {
		return nil, err
	}
	root, err := os.MkdirTemp(temp, "sworn-driver-certification-v1-")
	if err != nil {
		return nil, fail("FACTORY_UNAVAILABLE")
	}
	factory := &ProductionDriverFactory{root: root}
	factory.options.EnvironmentCredentials = systemEnvironmentCredential
	factory.options.FileCredentials = systemFileCredential

	awsEnvironments, err := configuredAWSEnvironments(reloaded.config)
	if err != nil {
		_ = factory.Close()
		return nil, err
	}
	factory.options.AWSCredentials = func(
		ctx context.Context,
		reference string,
	) ([][]byte, error) {
		return systemAWSEnvironment(ctx, reference, awsEnvironments)
	}
	factory.options.LiveProbes = make(map[string]ProfileLiveProbe)

	presets, err := presetMap(reloaded.config.Presets)
	if err != nil {
		_ = factory.Close()
		return nil, err
	}
	sources := credentialSourceMap(reloaded.config.Credentials)
	for _, raw := range reloaded.config.Adapters {
		resolved, resolveErr := resolvePreset(
			raw,
			presets,
			reloaded.config.Variables,
		)
		if resolveErr != nil {
			_ = factory.Close()
			return nil, resolveErr
		}
		descriptor, descriptorErr := resolved.descriptor()
		if descriptorErr != nil {
			_ = factory.Close()
			return nil, descriptorErr
		}
		switch descriptor.kind {
		case driverAdapterOpenAI, driverAdapterGemini, driverAdapterBedrock:
			config := cloneDriverAdapterConfig(resolved)
			factory.options.LiveProbes[descriptor.key] =
				factory.liveProbe(config, descriptor, sources)
		}
	}
	return factory, nil
}

// Options returns a detached callback set. The callbacks retain only this
// factory's bounded temporary root and the admitted, secret-free config facts.
func (factory *ProductionDriverFactory) Options() DriverFactoryOptions {
	if factory == nil {
		return DriverFactoryOptions{}
	}
	options := factory.options
	options.LiveProbes = maps.Clone(options.LiveProbes)
	options.RoundTrippers = maps.Clone(options.RoundTrippers)
	return options
}

// Close removes only the private temporary root created by this factory.
func (factory *ProductionDriverFactory) Close() error {
	if factory == nil {
		return nil
	}
	factory.closeOnce.Do(func() {
		if factory.root == "" || !filepath.IsAbs(factory.root) ||
			filepath.Base(factory.root) == "." ||
			filepath.Base(factory.root) == string(filepath.Separator) {
			factory.closeErr = fail("FACTORY_CLEANUP_FAILED")
			return
		}
		if err := os.RemoveAll(factory.root); err != nil {
			factory.closeErr = fail("FACTORY_CLEANUP_FAILED")
		}
		factory.root = ""
	})
	return factory.closeErr
}

func (factory *ProductionDriverFactory) liveProbe(
	config DriverAdapterConfig,
	descriptor driverAdapterDescriptor,
	sources map[string]DriverCredentialSource,
) ProfileLiveProbe {
	return func(ctx context.Context, ref string, model string) error {
		if factory == nil || factory.root == "" || ctx == nil ||
			ctx.Err() != nil {
			return fail("LIVE_PROBE_FAILED")
		}
		options := factory.Options()
		options.LiveProbes = nil
		adapter, err := buildConfiguredAdapter(
			config,
			descriptor,
			sources,
			options,
		)
		if err != nil {
			return fail("LIVE_PROBE_FAILED")
		}
		var credentialRef *string
		if descriptor.auth != AuthModeNone {
			if ref == "" {
				return fail("LIVE_PROBE_FAILED")
			}
			credentialRef = &ref
		}
		profile := ProfileConfig{
			Key:           "live-certification",
			Adapter:       adapter.Identity().Key,
			Network:       NetworkRequired,
			AuthMode:      descriptor.auth,
			CredentialRef: credentialRef,
		}
		registry, err := NewSelectionRegistry(
			[]ProfileConfig{profile},
			[]Adapter{adapter},
		)
		if err != nil {
			return fail("LIVE_PROBE_FAILED")
		}
		selections := RoleSelections{
			Planner:     RoleSelection{Profile: profile.Key, Model: model},
			Implementer: RoleSelection{Profile: profile.Key, Model: model},
			Captain:     RoleSelection{Profile: profile.Key, Model: model},
			Verifier:    RoleSelection{Profile: profile.Key, Model: model},
		}
		selected, err := registry.Resolve(selections, RoleImplementer)
		if err != nil {
			return fail("LIVE_PROBE_FAILED")
		}
		invocation, err := factory.certificationInvocation(ctx, selected)
		if err != nil {
			return fail("LIVE_PROBE_FAILED")
		}
		_, err = (Dispatcher{}).Invoke(ctx, invocation)
		return err
	}
}

func (factory *ProductionDriverFactory) certificationInvocation(
	ctx context.Context,
	selected SelectedProfile,
) (Invocation, error) {
	if factory == nil || factory.root == "" || ctx == nil ||
		ctx.Err() != nil {
		return Invocation{}, fail("LIVE_PROBE_FAILED")
	}
	workspace, err := os.MkdirTemp(factory.root, "workspace-")
	if err != nil {
		return Invocation{}, fail("LIVE_PROBE_FAILED")
	}
	instruction := []byte(
		`{"instruction":"Exercise the configured model and terminate with one valid implementer_design sworn_submit call. Do not make product changes.","schema_version":"` +
			driverCertificationSchemaVersion + `"}`,
	)
	input := Input{
		Name:   "driver-certification",
		Path:   "certification/request.json",
		Digest: Digest(instruction),
	}
	sum := sha256.Sum256([]byte(
		selected.Profile.Key + "\x00" + selected.Model + "\x00" + workspace,
	))
	invocationID := "driver-certify-" + hex.EncodeToString(sum[:8])
	request, err := NewRequest(
		invocationID,
		RoleImplementer,
		selected.Profile.Key,
		selected.Model,
		Workspace{Path: GuestWorkspacePath, Access: ReadWrite},
		[]Input{input},
		true,
		Limits{TimeoutMillis: 120_000, OutputBytes: MaxProviderOutputBytes},
	)
	if err != nil {
		return Invocation{}, err
	}
	permission, err := NewSubmissionPermission(
		request,
		selected,
		ContainmentReadWrite,
		ImplementerDesign,
	)
	if err != nil {
		return Invocation{}, err
	}
	return Invocation{
		Request:       request,
		HostWorkspace: workspace,
		Selected:      selected,
		Permission:    permission,
		Inputs: []InputContent{{
			Input: input,
			Bytes: instruction,
		}},
		// Certification grants the driver's own bounded recovery budgets
		// (one prose nudge, MaxSubmissionCorrections); with no hook a model
		// that opens in prose fails RECOVERY_STEP_REFUSED instead of being
		// nudged, which made certification flaky for reasoning models.
		RecoveryStepHook: func(context.Context, RecoveryStepKind) error {
			return nil
		},
	}, nil
}

func systemEnvironmentCredential(
	ctx context.Context,
	reference string,
) ([]byte, error) {
	if ctx == nil || ctx.Err() != nil ||
		!environmentNamePattern.MatchString(reference) {
		return nil, fail("CREDENTIAL_UNAVAILABLE")
	}
	value, ok := os.LookupEnv(reference)
	if !ok || len(value) == 0 || len(value) > maxSystemCredentialBytes {
		return nil, fail("CREDENTIAL_UNAVAILABLE")
	}
	return []byte(value), nil
}

func systemFileCredential(
	ctx context.Context,
	reference string,
) ([]byte, error) {
	if ctx == nil || ctx.Err() != nil || reference == "" ||
		!filepath.IsAbs(reference) || filepath.Clean(reference) != reference {
		return nil, fail("CREDENTIAL_UNAVAILABLE")
	}
	info, err := os.Lstat(reference)
	if err != nil || !info.Mode().IsRegular() ||
		info.Mode()&os.ModeSymlink != 0 ||
		info.Mode().Perm()&0o077 != 0 ||
		info.Size() < 1 || info.Size() > maxSystemCredentialBytes {
		return nil, fail("CREDENTIAL_UNAVAILABLE")
	}
	file, err := os.Open(reference)
	if err != nil {
		return nil, fail("CREDENTIAL_UNAVAILABLE")
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, fail("CREDENTIAL_UNAVAILABLE")
	}
	body, err := io.ReadAll(io.LimitReader(file, maxSystemCredentialBytes+1))
	if err != nil || len(body) == 0 || len(body) > maxSystemCredentialBytes {
		clearBytes(body)
		return nil, fail("CREDENTIAL_UNAVAILABLE")
	}
	return body, nil
}

func configuredAWSEnvironments(
	config DriverConfig,
) (map[string][]string, error) {
	presets, err := presetMap(config.Presets)
	if err != nil {
		return nil, err
	}
	adapters := make(map[string]DriverAdapterConfig, len(config.Adapters))
	for _, raw := range config.Adapters {
		resolved, resolveErr := resolvePreset(raw, presets, config.Variables)
		if resolveErr != nil {
			return nil, resolveErr
		}
		descriptor, descriptorErr := resolved.descriptor()
		if descriptorErr != nil {
			return nil, descriptorErr
		}
		adapters[descriptor.key] = resolved
	}
	sources := credentialSourceMap(config.Credentials)
	result := make(map[string][]string)
	for _, profile := range config.Profiles {
		if profile.CredentialSource == nil {
			continue
		}
		source, ok := sources[*profile.CredentialSource]
		if !ok || source.Kind != CredentialAWS {
			continue
		}
		adapter := adapters[profile.Adapter]
		keys, ok := adapterAWSEnvironmentKeys(adapter)
		if !ok {
			return nil, fail("INVALID_DRIVER_CONFIG")
		}
		keys = normalizedAWSEnvironmentKeys(keys)
		if prior, exists := result[source.Reference]; exists &&
			!slices.Equal(prior, keys) {
			return nil, fail("INVALID_DRIVER_CONFIG")
		}
		result[source.Reference] = keys
	}
	return result, nil
}

func adapterAWSEnvironmentKeys(
	config DriverAdapterConfig,
) ([]string, bool) {
	switch {
	case config.Bedrock != nil:
		return append([]string(nil), config.Bedrock.Chain.EnvironmentKeys...), true
	case config.OpenAI != nil &&
		config.OpenAI.effectiveAuth() == AuthModeAWSSigV4 &&
		config.OpenAI.Chain != nil:
		return append([]string(nil), config.OpenAI.Chain.EnvironmentKeys...), true
	default:
		return nil, false
	}
}

func systemAWSEnvironment(
	ctx context.Context,
	reference string,
	configured map[string][]string,
) ([][]byte, error) {
	if ctx == nil || ctx.Err() != nil ||
		!providerKeyPattern.MatchString(reference) {
		return nil, fail("CREDENTIAL_UNAVAILABLE")
	}
	keys, ok := configured[reference]
	if !ok || len(keys) == 0 {
		return nil, fail("CREDENTIAL_UNAVAILABLE")
	}
	environment := make([][]byte, 0, len(keys))
	for _, key := range keys {
		value, present := os.LookupEnv(key)
		if !present || value == "" ||
			len(key)+1+len(value) > maxSystemCredentialBytes {
			clearEnvironment(environment)
			return nil, fail("CREDENTIAL_UNAVAILABLE")
		}
		environment = append(environment, []byte(key+"="+value))
	}
	return environment, nil
}

func cloneDriverAdapterConfig(
	config DriverAdapterConfig,
) DriverAdapterConfig {
	switch {
	case config.Process != nil:
		value := *config.Process
		config.Process = &value
	case config.Native != nil:
		value := cloneNativeAdapterConfig(*config.Native)
		config.Native = &value
	case config.OpenAI != nil:
		value := cloneOpenAIProfileConfig(*config.OpenAI)
		config.OpenAI = &value
	case config.Gemini != nil:
		value := cloneHTTPProfileConfig(*config.Gemini)
		config.Gemini = &value
	case config.Bedrock != nil:
		value := cloneBedrockProfileConfig(*config.Bedrock)
		config.Bedrock = &value
	}
	return config
}
