package driver

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
)

const (
	NetworkNone     NetworkPolicy = "none"
	NetworkRequired NetworkPolicy = "required"
)

var providerKeyPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

type NetworkPolicy string

// ExecutableIdentity is deliberately private to the process adapter. Profiles
// and the common dispatcher bind only provider-neutral adapter identity.
type ExecutableIdentity struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
}

// AdapterIdentity binds one registered adapter implementation without assuming
// that it is a CLI process. ConfigurationDigest may bind a binary, an HTTP
// endpoint policy, or a cloud adapter configuration.
type AdapterIdentity struct {
	Key                 string `json:"key"`
	ID                  string `json:"id"`
	Version             string `json:"version"`
	ConfigurationDigest string `json:"configuration_digest"`
}

// Adapter is implemented by Sworn-owned CLI, HTTP, cloud, and fake adapters.
// The unexported invocation method keeps orchestration behind Dispatcher.
type Adapter interface {
	Identity() AdapterIdentity
	invoke(context.Context, Invocation) (Observation, error)
}

// continuationAdapter is deliberately opt-in. Existing adapters continue to
// satisfy Adapter without implementing or changing any continuation behavior.
// The state is opaque to the dispatcher, immutable while suspended, and owns
// no permission, workspace lease, Baton decision, or active tool session.
type continuationAdapter interface {
	invokeContinuation(
		context.Context,
		Invocation,
	) (Observation, continuationState, error)
	resumeContinuation(
		context.Context,
		Invocation,
		continuationState,
	) (Observation, error)
}

// recoverableContinuationAdapter is the same continuation substrate with
// ownership transfer when a resumed worker yields again.
type recoverableContinuationAdapter interface {
	continuationAdapter
	invokeRecoverableContinuation(
		context.Context,
		Invocation,
	) (Observation, continuationState, error)
	resumeRecoverableContinuation(
		context.Context,
		Invocation,
		continuationState,
		bool,
	) (Observation, continuationState, error)
}

// ProcessAdapter is the contained process implementation retained for CLI
// adapters and the deterministic fake. Its executable is not part of the
// provider-neutral profile schema.
type ProcessAdapter struct {
	identity   AdapterIdentity
	executable ExecutableIdentity
}

func NewProcessAdapter(
	key string,
	id string,
	version string,
	executable ExecutableIdentity,
) (*ProcessAdapter, error) {
	if !providerKeyPattern.MatchString(key) ||
		!driverIdentityPattern.MatchString(id) ||
		!versionPattern.MatchString(version) {
		return nil, fail("INVALID_ADAPTER")
	}
	if err := validateExecutableIdentity(executable); err != nil {
		return nil, err
	}
	return &ProcessAdapter{
		identity: AdapterIdentity{
			Key:                 key,
			ID:                  id,
			Version:             version,
			ConfigurationDigest: executable.Digest,
		},
		executable: executable,
	}, nil
}

func (adapter *ProcessAdapter) Identity() AdapterIdentity {
	if adapter == nil {
		return AdapterIdentity{}
	}
	return adapter.identity
}

func (adapter *ProcessAdapter) invoke(
	ctx context.Context,
	invocation Invocation,
) (Observation, error) {
	if adapter == nil {
		return Observation{}, fail("INVALID_ADAPTER")
	}
	return platformInvoke(ctx, invocation, adapter.executable)
}

// ProfileConfig chooses an adapter and public launch policy. CredentialRef is
// an opaque, explicit lookup key; adapters never perform ambient fallback.
// AuthMode carries the admitted per-profile authentication surface into
// registry-level admission so a credential-less none profile is distinguishable
// from a fail-closed omission.
type ProfileConfig struct {
	Key           string        `json:"key"`
	Adapter       string        `json:"adapter"`
	Network       NetworkPolicy `json:"network"`
	AuthMode      AuthMode      `json:"auth_mode,omitempty"`
	CredentialRef *string       `json:"credential_ref"`
}

// ModelSelection binds one explicit configured profile and model. It is
// role-neutral so the same registry resolution is available to Sworn-owned
// automation without inventing another driver or model fallback layer.
type ModelSelection struct {
	Profile string `json:"profile"`
	Model   string `json:"model"`
}

// RoleSelection remains a source-compatible name for manifest v2 callers.
type RoleSelection = ModelSelection

// RoleSelections is closed to exactly the four model-facing roles.
type RoleSelections struct {
	Planner     RoleSelection `json:"planner"`
	Implementer RoleSelection `json:"implementer"`
	Captain     RoleSelection `json:"captain"`
	Verifier    RoleSelection `json:"verifier"`
}

type registeredProfile struct {
	config  ProfileConfig
	adapter Adapter
}

type SelectionRegistry struct {
	profiles map[string]registeredProfile
}

type SelectedProfile struct {
	Profile ProfileConfig
	Adapter AdapterIdentity
	Model   string
	adapter Adapter
}

func EncodeRoleSelections(selections RoleSelections) ([]byte, error) {
	if err := ValidateRoleSelections(selections); err != nil {
		return nil, err
	}
	body, err := json.Marshal(selections)
	if err != nil {
		return nil, fail("INVALID_JSON")
	}
	return body, nil
}

func DecodeRoleSelections(body []byte) (RoleSelections, error) {
	var selections RoleSelections
	root, err := decodeTyped(
		body,
		65_536,
		[]string{"planner", "implementer", "captain", "verifier"},
		nil,
		&selections,
	)
	if err != nil {
		return RoleSelections{}, err
	}
	for _, name := range []string{"planner", "implementer", "captain", "verifier"} {
		if _, err := closedObject(root[name], []string{"profile", "model"}, nil); err != nil {
			return RoleSelections{}, err
		}
	}
	if err := ValidateRoleSelections(selections); err != nil {
		return RoleSelections{}, err
	}
	canonical, err := EncodeRoleSelections(selections)
	if err != nil {
		return RoleSelections{}, err
	}
	if !bytes.Equal(canonical, body) {
		return RoleSelections{}, fail("NONCANONICAL_JSON")
	}
	return selections, nil
}

func NewSelectionRegistry(
	configs []ProfileConfig,
	adapters []Adapter,
) (SelectionRegistry, error) {
	registeredAdapters := make(map[string]Adapter, len(adapters))
	for _, adapter := range adapters {
		if adapter == nil {
			return SelectionRegistry{}, fail("INVALID_ADAPTER")
		}
		identity := adapter.Identity()
		if err := validateAdapterIdentity(identity); err != nil {
			return SelectionRegistry{}, err
		}
		if _, duplicate := registeredAdapters[identity.Key]; duplicate {
			return SelectionRegistry{}, fail("DUPLICATE_ADAPTER")
		}
		registeredAdapters[identity.Key] = adapter
	}
	if len(registeredAdapters) == 0 {
		return SelectionRegistry{}, fail("MISSING_ADAPTER")
	}

	profiles := make(map[string]registeredProfile, len(configs))
	for _, config := range configs {
		if err := validateProfileConfig(config); err != nil {
			return SelectionRegistry{}, err
		}
		adapter, ok := registeredAdapters[config.Adapter]
		if !ok {
			return SelectionRegistry{}, fail("UNKNOWN_ADAPTER")
		}
		if err := validateNetworkPolicy(adapter.Identity().ID, config.Network); err != nil {
			return SelectionRegistry{}, err
		}
		if _, duplicate := profiles[config.Key]; duplicate {
			return SelectionRegistry{}, fail("DUPLICATE_PROFILE")
		}
		profiles[config.Key] = registeredProfile{
			config:  cloneProfileConfig(config),
			adapter: adapter,
		}
	}
	if len(profiles) == 0 {
		return SelectionRegistry{}, fail("MISSING_PROFILE")
	}
	return SelectionRegistry{profiles: profiles}, nil
}

func (registry SelectionRegistry) Resolve(
	selections RoleSelections,
	role Role,
) (SelectedProfile, error) {
	if err := ValidateRoleSelections(selections); err != nil {
		return SelectedProfile{}, err
	}
	var selection RoleSelection
	switch role {
	case RolePlanner:
		selection = selections.Planner
	case RoleImplementer:
		selection = selections.Implementer
	case RoleCaptain:
		selection = selections.Captain
	case RoleVerifier:
		selection = selections.Verifier
	default:
		if role == Role("merge") {
			return SelectedProfile{}, fail("ROLE_NOT_DISPATCHABLE")
		}
		return SelectedProfile{}, fail("INVALID_ROLE")
	}
	return registry.ResolveSelection(selection)
}

// ResolveSelection resolves one exact profile/model pair through the same
// provider-neutral registry used by role dispatch. There are no defaults or
// fallback profiles.
func (registry SelectionRegistry) ResolveSelection(
	selection ModelSelection,
) (SelectedProfile, error) {
	if err := ValidateModelSelection(selection); err != nil {
		return SelectedProfile{}, err
	}
	registered, ok := registry.profiles[selection.Profile]
	if !ok {
		return SelectedProfile{}, fail("UNKNOWN_PROFILE")
	}
	identity := registered.adapter.Identity()
	if err := validateAdapterIdentity(identity); err != nil {
		return SelectedProfile{}, err
	}
	if identity.Key != registered.config.Adapter {
		return SelectedProfile{}, fail("ADAPTER_IDENTITY_CHANGED")
	}
	return SelectedProfile{
		Profile: cloneProfileConfig(registered.config),
		Adapter: identity,
		Model:   selection.Model,
		adapter: registered.adapter,
	}, nil
}

func ValidateRoleSelections(selections RoleSelections) error {
	for _, selection := range []ModelSelection{
		selections.Planner,
		selections.Implementer,
		selections.Captain,
		selections.Verifier,
	} {
		if err := ValidateModelSelection(selection); err != nil {
			return err
		}
	}
	return nil
}

func ValidateModelSelection(selection ModelSelection) error {
	if !providerKeyPattern.MatchString(selection.Profile) {
		return fail("INVALID_PROFILE")
	}
	if err := validateText(selection.Model, 500, false); err != nil {
		return fail("INVALID_MODEL")
	}
	return nil
}

func validateAdapterIdentity(identity AdapterIdentity) error {
	if !providerKeyPattern.MatchString(identity.Key) ||
		!driverIdentityPattern.MatchString(identity.ID) ||
		!versionPattern.MatchString(identity.Version) ||
		!digestPattern.MatchString(identity.ConfigurationDigest) {
		return fail("INVALID_ADAPTER")
	}
	return nil
}

func validateProfileConfig(config ProfileConfig) error {
	if !providerKeyPattern.MatchString(config.Key) ||
		!providerKeyPattern.MatchString(config.Adapter) {
		return fail("INVALID_PROFILE")
	}
	if config.Network != NetworkNone && config.Network != NetworkRequired {
		return fail("INVALID_NETWORK_POLICY")
	}
	if config.AuthMode != "" && !config.AuthMode.valid() {
		return fail("INVALID_PROFILE")
	}
	if config.CredentialRef != nil &&
		!providerKeyPattern.MatchString(*config.CredentialRef) {
		return fail("INVALID_CREDENTIAL_REFERENCE")
	}
	return nil
}

func validateSelectedProfile(selected SelectedProfile) error {
	if err := validateProfileConfig(selected.Profile); err != nil {
		return err
	}
	if err := validateAdapterIdentity(selected.Adapter); err != nil {
		return err
	}
	if selected.adapter == nil ||
		selected.Profile.Adapter != selected.Adapter.Key ||
		selected.adapter.Identity() != selected.Adapter ||
		validateText(selected.Model, 500, false) != nil {
		return fail("INVALID_SELECTION")
	}
	return validateNetworkPolicy(selected.Adapter.ID, selected.Profile.Network)
}

func validateExecutableIdentity(executable ExecutableIdentity) error {
	if executable.Path == "" || !filepath.IsAbs(executable.Path) ||
		filepath.Clean(executable.Path) != executable.Path ||
		!digestPattern.MatchString(executable.Digest) {
		return fail("INVALID_EXECUTABLE")
	}
	info, err := os.Lstat(executable.Path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return fail("INVALID_EXECUTABLE")
	}
	return nil
}

func validateNetworkPolicy(adapterID string, network NetworkPolicy) error {
	if network != NetworkNone && network != NetworkRequired {
		return fail("INVALID_NETWORK_POLICY")
	}
	if adapterID == FakeDriverID && network != NetworkNone {
		return fail("INVALID_NETWORK_POLICY")
	}
	return nil
}

func cloneProfileConfig(config ProfileConfig) ProfileConfig {
	config.CredentialRef = cloneString(config.CredentialRef)
	return config
}
