package runtime

import (
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
		manifest.Roles.Lead.Profile,
		manifest.Roles.Verifier.Profile,
	}
	if recovery, enabled := manifest.recoverySelection(); enabled {
		profiles = append(profiles, recovery.Profile)
	}
	sort.Strings(profiles)
	return slices.Compact(profiles)
}

// checkManifestBinding refuses a manifest bound to a different driver config.
func (runtime *productionDriverRuntime) checkManifestBinding(
	manifest admittedManifest,
) error {
	if runtime == nil {
		return runtimeFail("DRIVER_CONFIG_UNAVAILABLE", nil)
	}
	if manifest.value.DriverConfigDigest != runtime.digest ||
		sha256Digest(runtime.body) != runtime.digest {
		return runtimeFail("DRIVER_CONFIG_DRIFT", nil)
	}
	return nil
}

// nativeCLISnapshotAdapters names the run-snapshot native adapters the
// manifest's profiles use.
func (runtime *productionDriverRuntime) nativeCLISnapshotAdapters(
	manifest admittedManifest,
) ([]string, error) {
	if err := runtime.checkManifestBinding(manifest); err != nil {
		return nil, err
	}
	return runtime.config.NativeCLISnapshotAdapters(
		manifestProfiles(manifest.value),
	), nil
}

// registryFor builds, once per profile set and bound snapshot set, the
// registry a run dispatches through. snapshots holds the run's verified
// native CLI snapshot facts by adapter key.
func (runtime *productionDriverRuntime) registryFor(
	manifest admittedManifest,
	snapshots map[string]driver.NativeCLISnapshot,
) (*configuredRuntimeRegistry, error) {
	if err := runtime.checkManifestBinding(manifest); err != nil {
		return nil, err
	}
	profiles := manifestProfiles(manifest.value)
	key := strings.Join(profiles, "\x00")
	for _, adapter := range slices.Sorted(maps.Keys(snapshots)) {
		snapshot := snapshots[adapter]
		key += "\x00" + adapter + "\x00" + snapshot.SnapshotPath +
			"\x00" + snapshot.Digest + "\x00" + snapshot.VersionOutput
	}
	options := runtime.options
	options.NativeCLISnapshots = snapshots

	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if existing := runtime.registries[key]; existing != nil {
		return existing, nil
	}
	registry, err := runtime.config.BuildRegistry(profiles, options)
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
		registry: registry,
		families: families,
	}
	runtime.registries[key] = configured
	return configured, nil
}

func (configured *configuredRuntimeRegistry) validateSelected(
	selected driver.SelectedProfile,
) error {
	if configured == nil {
		return nil
	}
	if _, ok := configured.families[selected.Profile.Key]; !ok {
		return runtimeFail("DRIVER_SELECTION_FAILED", nil)
	}
	return nil
}
