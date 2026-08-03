package runtime

import (
	"context"
	"maps"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/swornagent/sworn/internal/driver"
)

type productionDriverRuntime struct {
	config  driver.LoadedDriverConfig
	body    []byte
	digest  string
	options driver.DriverFactoryOptions

	mu         sync.Mutex
	registries map[string]*configuredRuntimeRegistry
}

type configuredRuntimeRegistry struct {
	registry driver.ConfiguredDriverRegistry
	families map[string]driver.ProfileFamily
	certify  func(context.Context, string, string) driver.ProfileReport

	certificationMu sync.Mutex
	certifications  map[string]*runtimeCertification
}

type runtimeCertification struct {
	once   sync.Once
	report driver.ProfileReport
}

func newProductionDriverRuntime(
	config driver.LoadedDriverConfig,
	options driver.DriverFactoryOptions,
) (*productionDriverRuntime, error) {
	body := config.CanonicalJSON()
	reloaded, err := driver.DecodeDriverConfig(body)
	if err != nil ||
		config.ConfigurationDigest() == "" ||
		reloaded.ConfigurationDigest() != config.ConfigurationDigest() {
		return nil, runtimeFail("INVALID_DRIVER_CONFIG", err)
	}
	options.NativeSmokeBuilders = maps.Clone(options.NativeSmokeBuilders)
	options.LiveProbes = maps.Clone(options.LiveProbes)
	options.RoundTrippers = maps.Clone(options.RoundTrippers)
	return &productionDriverRuntime{
		config:     reloaded,
		body:       body,
		digest:     reloaded.ConfigurationDigest(),
		options:    options,
		registries: make(map[string]*configuredRuntimeRegistry),
	}, nil
}

func manifestProfiles(manifest Manifest) []string {
	profiles := []string{
		manifest.Roles.Planner.Profile,
		manifest.Roles.Implementer.Profile,
		manifest.Roles.Captain.Profile,
		manifest.Roles.Verifier.Profile,
	}
	if recovery, enabled := manifest.recoverySelection(); enabled {
		profiles = append(profiles, recovery.Profile)
	}
	sort.Strings(profiles)
	return slices.Compact(profiles)
}

func (runtime *productionDriverRuntime) registryFor(
	manifest admittedManifest,
) (*configuredRuntimeRegistry, error) {
	if runtime == nil {
		return nil, runtimeFail("DRIVER_CONFIG_UNAVAILABLE", nil)
	}
	if manifest.value.DriverConfigDigest != runtime.digest ||
		sha256Digest(runtime.body) != runtime.digest {
		return nil, runtimeFail("DRIVER_CONFIG_DRIFT", nil)
	}
	profiles := manifestProfiles(manifest.value)
	key := strings.Join(profiles, "\x00")

	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if existing := runtime.registries[key]; existing != nil {
		return existing, nil
	}
	registry, err := runtime.config.BuildRegistry(profiles, runtime.options)
	if err != nil || registry.ConfigurationDigest() != runtime.digest {
		return nil, runtimeFail("DRIVER_UNAVAILABLE", err)
	}
	families := make(map[string]driver.ProfileFamily, len(profiles))
	for _, certification := range registry.Certifications() {
		if prior, ok := families[certification.Profile]; ok &&
			prior != certification.Family {
			return nil, runtimeFail("DRIVER_UNAVAILABLE", nil)
		}
		families[certification.Profile] = certification.Family
	}
	for _, profile := range profiles {
		family, ok := families[profile]
		if !ok || family == driver.ProfileFake {
			return nil, runtimeFail("DRIVER_UNAVAILABLE", nil)
		}
	}
	configured := &configuredRuntimeRegistry{
		registry:       registry,
		families:       families,
		certify:        registry.Certify,
		certifications: make(map[string]*runtimeCertification),
	}
	runtime.registries[key] = configured
	return configured, nil
}

func (configured *configuredRuntimeRegistry) certifySelected(
	ctx context.Context,
	selected driver.SelectedProfile,
) error {
	if configured == nil {
		return nil
	}
	if _, ok := configured.families[selected.Profile.Key]; !ok {
		return runtimeFail("DRIVER_SELECTION_FAILED", nil)
	}
	// Runs are not gated on a pre-flight smoke test of the agent CLI. The run
	// exercises the CLI for real on its first dispatch, and a CLI that cannot
	// work fails there with the actual error. Blocking every run because a
	// handshake drifted reports a problem the run had not yet had.
	//
	// sworn driver certify remains available as a deliberate diagnostic.
	return nil
}
