package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/swornagent/sworn/internal/executor"
)

const (
	LocalCheckDefinitionSchemaVersion = "sworn-local-check-v1"
	LocalEnvironmentSchemaVersion     = "sworn-local-environment-v1"
	ContentEnvironmentSchemaVersion   = "sworn-local-environment-v2"
	LocalEnvironmentMediaType         = "application/vnd.sworn.local-environment+json"
	MaximumLocalCheckDefinitionBytes  = 256 << 10
	MaximumLocalEnvironmentBytes      = 64 << 10
)

type LocalEvidenceDefinition struct {
	ID            string   `json:"id"`
	AcceptanceIDs []string `json:"acceptance_ids"`
	Boundary      string   `json:"boundary"`
	UsesMocks     bool     `json:"uses_mocks"`
	Observed      string   `json:"observed"`
}

type LocalCheckDefinition struct {
	SchemaVersion    string                  `json:"schema_version"`
	Argv             []string                `json:"argv"`
	WorkingDirectory string                  `json:"working_directory"`
	TimeoutSeconds   int64                   `json:"timeout_seconds"`
	Evidence         LocalEvidenceDefinition `json:"evidence"`
}

// ParseLocalCheckDefinition validates the exact policy artifact semantics used
// both before execution and again when a submission is constructed.
func ParseLocalCheckDefinition(contents []byte) (LocalCheckDefinition, error) {
	var definition LocalCheckDefinition
	if err := decodeLocalJSON(
		contents, MaximumLocalCheckDefinitionBytes, "local check definition", false, &definition,
	); err != nil {
		return LocalCheckDefinition{}, err
	}
	if definition.SchemaVersion != LocalCheckDefinitionSchemaVersion || definition.WorkingDirectory != "." ||
		definition.TimeoutSeconds <= 0 || definition.TimeoutSeconds > int64((24*time.Hour)/time.Second) ||
		len(definition.Argv) == 0 {
		return LocalCheckDefinition{}, errors.New("local check definition exceeds the initial execution capability")
	}
	for _, argument := range definition.Argv {
		if argument == "" || !utf8.ValidString(argument) || strings.ContainsRune(argument, '\x00') {
			return LocalCheckDefinition{}, errors.New("local check definition contains invalid argv")
		}
	}
	if err := executor.ValidateArgv(definition.Argv); err != nil {
		return LocalCheckDefinition{}, fmt.Errorf("local check definition argv is unsupported: %w", err)
	}
	evidence := definition.Evidence
	if !ValidID(evidence.ID) || len(evidence.AcceptanceIDs) == 0 ||
		!slices.IsSorted(evidence.AcceptanceIDs) || duplicateStrings(evidence.AcceptanceIDs) ||
		(evidence.Boundary != "component" && evidence.Boundary != "assembled") ||
		(evidence.UsesMocks && evidence.Boundary != "component") ||
		strings.TrimSpace(evidence.Observed) == "" || !utf8.ValidString(evidence.Observed) {
		return LocalCheckDefinition{}, errors.New("local check definition has invalid evidence semantics")
	}
	for _, acceptanceID := range evidence.AcceptanceIDs {
		if !ValidID(acceptanceID) {
			return LocalCheckDefinition{}, errors.New("local check definition has an invalid acceptance id")
		}
	}
	return definition, nil
}

type LocalExecutorProbe struct {
	BubblewrapVersion string   `json:"bubblewrap_version"`
	SystemdVersion    string   `json:"systemd_version"`
	CgroupV2          bool     `json:"cgroup_v2"`
	UserManager       string   `json:"user_manager"`
	Controllers       []string `json:"controllers"`
}

type LocalExecutionLimits struct {
	RuntimeNanoseconds int64  `json:"runtime_nanoseconds"`
	MemoryBytes        uint64 `json:"memory_bytes"`
	SwapBytes          uint64 `json:"swap_bytes"`
	Tasks              uint64 `json:"tasks"`
	CPUPercent         uint64 `json:"cpu_percent"`
	FileBytes          uint64 `json:"file_bytes"`
	TempBytes          uint64 `json:"temp_bytes"`
	HomeBytes          uint64 `json:"home_bytes"`
	InputBytes         uint64 `json:"input_bytes"`
	WorkspaceBytes     uint64 `json:"workspace_bytes"`
	StdoutBytes        int64  `json:"stdout_bytes"`
	StderrBytes        int64  `json:"stderr_bytes"`
}

type LocalEnvironment struct {
	SchemaVersion          string               `json:"schema_version"`
	ProtocolSnapshotDigest string               `json:"protocol_snapshot_digest"`
	EngineRuntime          string               `json:"engine_runtime"`
	OS                     string               `json:"os"`
	Architecture           string               `json:"architecture"`
	Executor               LocalExecutorProbe   `json:"executor"`
	ExecutorPolicyVersion  string               `json:"executor_policy_version"`
	Limits                 LocalExecutionLimits `json:"limits"`
	RuntimeTrustRoot       string               `json:"runtime_trust_root"`
	RuntimeManifestDigest  string               `json:"runtime_manifest_digest,omitempty"`
	HermeticToolchain      bool                 `json:"hermetic_toolchain"`
	WorkspaceAccess        string               `json:"workspace_access"`
	Network                string               `json:"network"`
}

// ParseLocalEnvironment proves that a concrete environment pointer resolves
// to the engine-owned schema rather than merely to arbitrary canonical JSON.
func ParseLocalEnvironment(contents []byte) (LocalEnvironment, error) {
	var environment LocalEnvironment
	if err := decodeLocalJSON(
		contents, MaximumLocalEnvironmentBytes, "local environment", true, &environment,
	); err != nil {
		return LocalEnvironment{}, err
	}
	probe := environment.Executor
	limits := environment.Limits
	contentRuntime := environment.SchemaVersion == ContentEnvironmentSchemaVersion
	validRuntime := environment.RuntimeTrustRoot == "/usr" && !environment.HermeticToolchain &&
		((contentRuntime && digestPattern.MatchString(environment.RuntimeManifestDigest)) ||
			(environment.SchemaVersion == LocalEnvironmentSchemaVersion && environment.RuntimeManifestDigest == ""))
	if !validRuntime ||
		!digestPattern.MatchString(environment.ProtocolSnapshotDigest) ||
		!nonEmpty(environment.EngineRuntime) || !nonEmpty(environment.OS) || !nonEmpty(environment.Architecture) ||
		environment.WorkspaceAccess != "read_only" || environment.Network != "none" ||
		!nonEmpty(probe.BubblewrapVersion) || !nonEmpty(probe.SystemdVersion) || !probe.CgroupV2 ||
		(probe.UserManager != "running" && probe.UserManager != "degraded") || len(probe.Controllers) == 0 ||
		!slices.IsSorted(probe.Controllers) || duplicateStrings(probe.Controllers) ||
		environment.ExecutorPolicyVersion != executor.ContainmentPolicyVersion ||
		limits.RuntimeNanoseconds <= 0 || limits.MemoryBytes == 0 || limits.Tasks == 0 ||
		limits.CPUPercent == 0 || limits.CPUPercent > 1000 || limits.FileBytes == 0 ||
		limits.TempBytes == 0 || limits.HomeBytes == 0 || limits.InputBytes == 0 ||
		limits.WorkspaceBytes == 0 || limits.StdoutBytes <= 0 || limits.StderrBytes <= 0 {
		return LocalEnvironment{}, errors.New("local environment exceeds the initial contained capability")
	}
	for _, required := range []string{"cpu", "memory", "pids"} {
		if !slices.Contains(probe.Controllers, required) {
			return LocalEnvironment{}, fmt.Errorf("local environment lacks required %s controller", required)
		}
	}
	for _, controller := range probe.Controllers {
		if !ValidID(controller) {
			return LocalEnvironment{}, errors.New("local environment has an invalid executor controller")
		}
	}
	reencoded, err := EncodeCanonical(environment)
	if err != nil || !bytes.Equal(contents, reencoded) {
		return LocalEnvironment{}, errors.New("local environment does not use its schema's exact field shape")
	}
	return environment, nil
}

func decodeLocalJSON(
	contents []byte,
	maximumBytes int,
	label string,
	requireCanonical bool,
	destination any,
) error {
	if len(contents) > maximumBytes {
		return fmt.Errorf("%s exceeds byte ceiling", label)
	}
	canonical, err := CanonicalizeJSON(contents)
	if err != nil {
		return fmt.Errorf("%s is not strict I-JSON: %w", label, err)
	}
	if requireCanonical && !bytes.Equal(contents, canonical) {
		return fmt.Errorf("%s is not canonical JSON", label)
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decode %s: %w", label, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%s has trailing input", label)
	}
	return nil
}
