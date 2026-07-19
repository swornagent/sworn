// Package executor owns Sworn's sole contained subprocess boundary.
package executor

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	InvocationSchemaVersion      = "sworn-executor-invocation-v1"
	WorkspaceExportSchemaVersion = "sworn-workspace-export-v1"
	ContainmentPolicyVersion     = "sworn-linux-containment-v1"
)

const (
	maximumInvocationInputs        = 256
	maximumInvocationArgumentBytes = 512 << 10
	maximumInvocationEnvironment   = 64 << 10
	maximumWorkspaceEntries        = 100_000
)

type NetworkMode string

const (
	NetworkNone NetworkMode = "none"
	NetworkHost NetworkMode = "host"
)

type WorkspaceAccess string

const (
	WorkspaceReadOnly       WorkspaceAccess = "read_only"
	WorkspaceWritableExport WorkspaceAccess = "writable_export"
)

type Input struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Digest string `json:"digest"`
}

type Invocation struct {
	SchemaVersion   string            `json:"schema_version"`
	ID              string            `json:"id"`
	Role            string            `json:"role"`
	RuntimeDigest   string            `json:"runtime_digest,omitempty"`
	Workspace       string            `json:"workspace"`
	WorkspaceDigest string            `json:"workspace_digest"`
	WorkspaceAccess WorkspaceAccess   `json:"workspace_access"`
	Inputs          []Input           `json:"inputs,omitempty"`
	Argv            []string          `json:"argv"`
	Environment     map[string]string `json:"environment,omitempty"`
	Network         NetworkMode       `json:"network"`
	Timeout         time.Duration     `json:"timeout"`
}

type Limits struct {
	Runtime     time.Duration
	MemoryBytes uint64
	SwapBytes   uint64
	Tasks       uint64
	CPUPercent  uint64
	FileBytes   uint64
	TempBytes   uint64
	HomeBytes   uint64
	InputBytes  uint64
	// WorkspaceBytes is the admitted post-run logical manifest ceiling. Live
	// allocation is bounded separately by the service cgroup and host tmpfs.
	WorkspaceBytes uint64
	StdoutBytes    int
	StderrBytes    int
}

func DefaultLimits() Limits {
	return Limits{
		Runtime:        5 * time.Minute,
		MemoryBytes:    2 << 30,
		SwapBytes:      0,
		Tasks:          256,
		CPUPercent:     100,
		FileBytes:      64 << 20,
		TempBytes:      512 << 20,
		HomeBytes:      128 << 20,
		InputBytes:     1 << 30,
		WorkspaceBytes: 1 << 30,
		StdoutBytes:    4 << 20,
		StderrBytes:    4 << 20,
	}
}

func (limits Limits) Validate() error {
	if limits.Runtime <= 0 || limits.MemoryBytes == 0 || limits.Tasks == 0 ||
		limits.CPUPercent == 0 || limits.FileBytes == 0 || limits.TempBytes == 0 ||
		limits.HomeBytes == 0 || limits.InputBytes == 0 || limits.WorkspaceBytes == 0 ||
		limits.StdoutBytes <= 0 || limits.StderrBytes <= 0 {
		return errors.New("executor limits must be finite and non-zero")
	}
	if limits.CPUPercent > 1000 {
		return errors.New("executor CPU limit exceeds 1000 percent")
	}
	return nil
}

type Options struct {
	RuntimeRoot        string
	WritableRoot       string
	ShimArgv           []string
	BubblewrapPath     string
	SystemdRunPath     string
	SystemctlPath      string
	Limits             Limits
	AllowedEnvironment []string
	AllowHostNetwork   bool
}

type BoundInput struct {
	Name   string `json:"name"`
	Digest string `json:"digest"`
	Size   uint64 `json:"size"`
}

// WorkspaceExport is a quarantined measured filesystem handle. It does not
// imply target success, scope admission, candidate identity, or quality.
type WorkspaceExport struct {
	SchemaVersion string `json:"schema_version"`
	InvocationID  string `json:"invocation_id"`
	Generation    string `json:"generation"`
	BaseDigest    string `json:"base_digest"`
	Path          string `json:"path"`
	Digest        string `json:"digest"`
	Bytes         uint64 `json:"bytes"`
}

type RawCompletion struct {
	InvocationID    string           `json:"invocation_id"`
	Unit            string           `json:"unit"`
	RuntimeDigest   string           `json:"runtime_digest,omitempty"`
	WorkspaceDigest string           `json:"workspace_digest"`
	WorkspaceAccess WorkspaceAccess  `json:"workspace_access"`
	Inputs          []BoundInput     `json:"inputs,omitempty"`
	StartedAt       time.Time        `json:"started_at"`
	CompletedAt     time.Time        `json:"completed_at"`
	ExitCode        int              `json:"exit_code"`
	Stdout          []byte           `json:"stdout,omitempty"`
	Stderr          []byte           `json:"stderr,omitempty"`
	Cancelled       bool             `json:"cancelled,omitempty"`
	TimedOut        bool             `json:"timed_out,omitempty"`
	OutputTruncated bool             `json:"output_truncated,omitempty"`
	Export          *WorkspaceExport `json:"export,omitempty"`
}

type ProbeReport struct {
	BubblewrapVersion string   `json:"bubblewrap_version"`
	SystemdVersion    string   `json:"systemd_version"`
	CgroupV2          bool     `json:"cgroup_v2"`
	UserManager       string   `json:"user_manager"`
	Controllers       []string `json:"controllers"`
}

var (
	idPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	envPattern = regexp.MustCompile(`^[A-Z_][A-Z0-9_]{0,63}$`)
)

func (invocation Invocation) validate(options Options) error {
	if invocation.SchemaVersion != InvocationSchemaVersion {
		return fmt.Errorf("unknown invocation schema %q", invocation.SchemaVersion)
	}
	if !idPattern.MatchString(invocation.ID) || !idPattern.MatchString(invocation.Role) {
		return errors.New("invocation requires valid id and role")
	}
	if invocation.RuntimeDigest != "" && !validDigest(invocation.RuntimeDigest) {
		return errors.New("invocation has an invalid content runtime digest")
	}
	if invocation.WorkspaceAccess != WorkspaceReadOnly && invocation.WorkspaceAccess != WorkspaceWritableExport {
		return fmt.Errorf("unsupported workspace access %q", invocation.WorkspaceAccess)
	}
	if invocation.Timeout <= 0 || invocation.Timeout > options.Limits.Runtime {
		return errors.New("invocation timeout is absent or exceeds the executor ceiling")
	}
	if invocation.Network != NetworkNone && invocation.Network != NetworkHost {
		return fmt.Errorf("unsupported network mode %q", invocation.Network)
	}
	if invocation.Network == NetworkHost && !options.AllowHostNetwork {
		return errors.New("host network is not admitted by this executor")
	}
	if err := validateAbsoluteDirectory(invocation.Workspace, "workspace"); err != nil {
		return err
	}
	if !validDigest(invocation.WorkspaceDigest) {
		return errors.New("workspace requires an exact sha256 digest")
	}
	if err := validateArgv(invocation.Argv); err != nil {
		return err
	}
	allowed := make(map[string]struct{}, len(options.AllowedEnvironment))
	for _, name := range options.AllowedEnvironment {
		if !envPattern.MatchString(name) || reservedEnvironment(name) {
			return fmt.Errorf("invalid allowed environment name %q", name)
		}
		allowed[name] = struct{}{}
	}
	var environmentBytes int
	for name, value := range invocation.Environment {
		if !envPattern.MatchString(name) || reservedEnvironment(name) {
			return fmt.Errorf("invalid invocation environment name %q", name)
		}
		if _, ok := allowed[name]; !ok {
			return fmt.Errorf("environment %q is not allowlisted", name)
		}
		if strings.ContainsRune(value, '\x00') || len(value) > 8192 {
			return fmt.Errorf("invalid invocation environment value for %q", name)
		}
		environmentBytes += len(name) + len(value) + 2
	}
	if len(invocation.Environment) > 64 {
		return errors.New("invocation environment exceeds 64 entries")
	}
	if environmentBytes > maximumInvocationEnvironment {
		return errors.New("invocation environment exceeds byte ceiling")
	}
	if len(invocation.Inputs) > maximumInvocationInputs {
		return fmt.Errorf("invocation inputs exceed %d entries", maximumInvocationInputs)
	}
	seenInputs := make(map[string]struct{}, len(invocation.Inputs))
	for _, input := range invocation.Inputs {
		if !idPattern.MatchString(input.Name) {
			return fmt.Errorf("invalid input name %q", input.Name)
		}
		if _, exists := seenInputs[input.Name]; exists {
			return fmt.Errorf("duplicate input name %q", input.Name)
		}
		seenInputs[input.Name] = struct{}{}
		if !filepath.IsAbs(input.Path) || filepath.Clean(input.Path) != input.Path {
			return fmt.Errorf("input %q path must be a clean absolute path", input.Name)
		}
		if !validDigest(input.Digest) {
			return fmt.Errorf("input %q requires an exact sha256 digest", input.Name)
		}
	}
	return nil
}

func validateArgv(argv []string) error {
	if len(argv) == 0 || len(argv) > 256 {
		return errors.New("invocation argv must contain 1 to 256 entries")
	}
	if !filepath.IsAbs(argv[0]) || filepath.Clean(argv[0]) != argv[0] ||
		(!strings.HasPrefix(argv[0], "/usr/") && !strings.HasPrefix(argv[0], "/bin/")) {
		return errors.New("invocation executable must be a clean absolute path beneath the mounted /usr trust root")
	}
	var total int
	for _, argument := range argv {
		if strings.ContainsRune(argument, '\x00') || len(argument) >= 128<<10 {
			return errors.New("invocation argv contains an invalid argument")
		}
		total += len(argument) + 1
	}
	if total > maximumInvocationArgumentBytes {
		return errors.New("invocation argv exceeds byte ceiling")
	}
	return nil
}

// ValidateArgv lets policy and receipt admission reject execution shapes that
// the contained boundary cannot run, before any producer effect is attempted.
func ValidateArgv(argv []string) error { return validateArgv(argv) }

func validateAbsoluteDirectory(path, label string) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return fmt.Errorf("%s must be a clean absolute path", label)
	}
	return nil
}

func reservedEnvironment(name string) bool {
	switch name {
	case "HOME", "PATH", "TMPDIR", "LANG", "LC_ALL", "TZ":
		return true
	}
	return strings.HasPrefix(name, "GIT_") || strings.HasPrefix(name, "LD_")
}

func validDigest(value string) bool {
	if len(value) != len("sha256:")+64 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	for _, char := range value[len("sha256:"):] {
		if !(char >= '0' && char <= '9') && !(char >= 'a' && char <= 'f') {
			return false
		}
	}
	return true
}

func sortedEnvironment(environment map[string]string) [][2]string {
	names := make([]string, 0, len(environment))
	for name := range environment {
		names = append(names, name)
	}
	sort.Strings(names)
	values := make([][2]string, len(names))
	for index, name := range names {
		values[index] = [2]string{name, environment[name]}
	}
	return values
}
