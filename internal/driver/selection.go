package driver

import (
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

// ProviderConfig contains only public launch identity. CredentialRef is an
// opaque lookup key for a provider adapter; this package never resolves or
// serializes the referenced bytes.
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

// RoleSelections is deliberately a closed four-entry value. Merge remains a
// portable codec role but has no production model dispatch selection.
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
	value, err := decodeStrict(body, 65_536)
	if err != nil {
		return RoleSelections{}, err
	}
	root, err := closedObject(value,
		[]string{"planner", "implementer", "captain", "verifier"}, nil)
	if err != nil {
		return RoleSelections{}, err
	}
	read := func(name string) (RoleSelection, error) {
		object, err := closedObject(root[name], []string{"provider", "model"}, nil)
		if err != nil {
			return RoleSelection{}, err
		}
		provider, err := requiredString(object, "provider")
		if err != nil {
			return RoleSelection{}, err
		}
		model, err := requiredString(object, "model")
		if err != nil {
			return RoleSelection{}, err
		}
		return RoleSelection{Provider: provider, Model: model}, nil
	}
	var selections RoleSelections
	if selections.Planner, err = read("planner"); err != nil {
		return RoleSelections{}, err
	}
	if selections.Implementer, err = read("implementer"); err != nil {
		return RoleSelections{}, err
	}
	if selections.Captain, err = read("captain"); err != nil {
		return RoleSelections{}, err
	}
	if selections.Verifier, err = read("verifier"); err != nil {
		return RoleSelections{}, err
	}
	if err := ValidateRoleSelections(selections); err != nil {
		return RoleSelections{}, err
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

func ValidateSelectedResult(selected SelectedProvider, result Result) error {
	if result.DriverID != selected.Provider.DriverID ||
		result.DriverVersion != selected.Provider.DriverVersion ||
		result.ObservedModel == nil || *result.ObservedModel != selected.Model {
		return fail("RESULT_BINDING_MISMATCH")
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
	if config.Network != NetworkNone && config.Network != NetworkRequired {
		return fail("INVALID_NETWORK_POLICY")
	}
	if config.CredentialRef != nil {
		if !providerKeyPattern.MatchString(*config.CredentialRef) {
			return fail("INVALID_CREDENTIAL_REFERENCE")
		}
	}
	return nil
}

func cloneProviderConfig(config ProviderConfig) ProviderConfig {
	config.CredentialRef = cloneString(config.CredentialRef)
	return config
}
