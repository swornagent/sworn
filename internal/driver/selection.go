package driver

import (
	"bytes"
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
type ExecutableIdentity struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
}

// ProviderConfig contains public launch identity and an opaque credential key.
type ProviderConfig struct {
	Key           string             `json:"key"`
	DriverID      string             `json:"driver_id"`
	DriverVersion string             `json:"driver_version"`
	Executable    ExecutableIdentity `json:"executable"`
	Network       NetworkPolicy      `json:"network"`
	CredentialRef *string            `json:"credential_ref"`
}
type RoleSelection struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// RoleSelections is closed to the four model roles; Merge is not dispatchable.
type RoleSelections struct {
	Planner     RoleSelection `json:"planner"`
	Implementer RoleSelection `json:"implementer"`
	Captain     RoleSelection `json:"captain"`
	Verifier    RoleSelection `json:"verifier"`
}
type SelectionRegistry struct {
	providers map[string]ProviderConfig
}
type SelectedProvider struct {
	Provider ProviderConfig
	Model    string
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
		if _, err := closedObject(root[name], []string{"provider", "model"}, nil); err != nil {
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
func NewSelectionRegistry(configs []ProviderConfig) (SelectionRegistry, error) {
	providers := make(map[string]ProviderConfig, len(configs))
	for _, config := range configs {
		if err := validateProviderConfig(config); err != nil {
			return SelectionRegistry{}, err
		}
		if _, duplicate := providers[config.Key]; duplicate {
			return SelectionRegistry{}, fail("DUPLICATE_PROVIDER")
		}
		providers[config.Key] = cloneProviderConfig(config)
	}
	if len(providers) == 0 {
		return SelectionRegistry{}, fail("MISSING_PROVIDER")
	}
	return SelectionRegistry{providers: providers}, nil
}
func (registry SelectionRegistry) Resolve(selections RoleSelections, role Role) (SelectedProvider, error) {
	if err := ValidateRoleSelections(selections); err != nil {
		return SelectedProvider{}, err
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
	case RoleMerge:
		return SelectedProvider{}, fail("ROLE_NOT_DISPATCHABLE")
	default:
		return SelectedProvider{}, fail("INVALID_ROLE")
	}
	provider, ok := registry.providers[selection.Provider]
	if !ok {
		return SelectedProvider{}, fail("UNKNOWN_PROVIDER")
	}
	return SelectedProvider{
		Provider: cloneProviderConfig(provider),
		Model:    selection.Model,
	}, nil
}
func ValidateRoleSelections(selections RoleSelections) error {
	for _, selection := range []RoleSelection{
		selections.Planner,
		selections.Implementer,
		selections.Captain,
		selections.Verifier,
	} {
		if !providerKeyPattern.MatchString(selection.Provider) {
			return fail("INVALID_PROVIDER")
		}
		if err := validateText(selection.Model, 500, false); err != nil {
			return fail("INVALID_MODEL")
		}
	}
	return nil
}
func validateProviderConfig(config ProviderConfig) error {
	if !providerKeyPattern.MatchString(config.Key) ||
		!driverIdentityPattern.MatchString(config.DriverID) ||
		!versionPattern.MatchString(config.DriverVersion) {
		return fail("INVALID_PROVIDER")
	}
	if config.Executable.Path == "" || !filepath.IsAbs(config.Executable.Path) ||
		filepath.Clean(config.Executable.Path) != config.Executable.Path ||
		!digestPattern.MatchString(config.Executable.Digest) {
		return fail("INVALID_EXECUTABLE")
	}
	info, err := os.Lstat(config.Executable.Path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return fail("INVALID_EXECUTABLE")
	}
	if err := validateNetworkPolicy(config.DriverID, config.Network); err != nil {
		return err
	}
	if config.CredentialRef != nil {
		if !providerKeyPattern.MatchString(*config.CredentialRef) {
			return fail("INVALID_CREDENTIAL_REFERENCE")
		}
	}
	return nil
}
func validateNetworkPolicy(driverID string, network NetworkPolicy) error {
	if network != NetworkNone && network != NetworkRequired {
		return fail("INVALID_NETWORK_POLICY")
	}
	if driverID == FakeDriverID && network != NetworkNone {
		return fail("INVALID_NETWORK_POLICY")
	}
	return nil
}
func cloneProviderConfig(config ProviderConfig) ProviderConfig {
	config.CredentialRef = cloneString(config.CredentialRef)
	return config
}
