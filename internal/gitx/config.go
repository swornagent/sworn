package gitx

// Project and host configuration for every location Sworn reads or writes.
//
// Two scopes exist, and they are kept strictly apart:
//
//   - Project-scoped locations (records root, journals root, contracts root,
//     commit prefix) determine where durable release truth lives. They are
//     read only from the committed project file at the one fixed,
//     non-configurable path ProjectConfigPath (docs/sworn/sworn.json) inside
//     the primary checkout. A user-scoped or environment override of any of
//     them is refused by name (RefuseProjectScopeOverrides) so two operators
//     of one repository can never resolve release truth to different places.
//
//   - Machine/user-scoped locations (workspace factory root, temp roots,
//     credentials directory, artefact home) resolve from environment
//     overrides with XDG-conformant defaults under a sworn subdirectory, and
//     never from a hardcoded literal (LoadHostPaths).
//
// Guest paths inside containment are deliberately NOT represented here: the
// ProjectConfig and HostPaths types expose no guest-path field, and the
// engine constructs the guest filesystem from compile-time constants in the
// driver. See internal/driver/contract.go and the guest-path immutability
// test in the driver package.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	// ProjectConfigPath is the one fixed, non-configurable path at which a
	// project declares its project-scoped locations. It is a compile-time
	// constant by design: the config file itself can never be relocated, so
	// every operator of a repository resolves the same project truth.
	ProjectConfigPath = "docs/sworn/sworn.json"

	// ProjectConfigSchemaVersion is the schema identity a project config file
	// may declare. It is optional; absent, the documented defaults apply.
	ProjectConfigSchemaVersion = "sworn.project-config/v1"

	// DefaultRecordsRoot is the unconfigured records root. It reproduces
	// today's .baton/releases location until S2 relocates project surfaces.
	DefaultRecordsRoot = ".baton/releases"
	// DefaultJournalsRoot is the unconfigured journals root.
	DefaultJournalsRoot = ".sworn"
	// DefaultContractsRoot is the unconfigured contracts root.
	DefaultContractsRoot = "contracts"
	// DefaultCommitPrefix is the unconfigured commit-message prefix used by
	// the plan/receipt actions, reproducing today's baton( subjects.
	DefaultCommitPrefix = "baton"
	// candidateCommitPrefixDefault is the unconfigured prefix used by the
	// engine's implementation-candidate commit subject, reproducing today's
	// sworn( subject exactly. S2 unifies the two defaults.
	candidateCommitPrefixDefault = "sworn"

	// Environment variable names for the machine/user-scoped host locations.
	EnvWorkspaceRoot  = "SWORN_WORKSPACE_ROOT"
	EnvTempRoot       = "SWORN_TEMP_ROOT"
	EnvCredentialsDir = "SWORN_CREDENTIALS_DIR"
	EnvArtefactHome   = "SWORN_ARTEFACT_HOME"
	// EnvNativeSessionRoot overrides the machine/user memory-backed root where
	// native continuation sessions park their crash-recovery state. It is
	// machine/user-scoped like the other host locations; its default is a
	// discovered memory-backed (tmpfs) directory rather than the general temp
	// root, because crash recovery trusts a memory-backed filesystem and the
	// general temp root usually lives on ordinary disk.
	EnvNativeSessionRoot = "SWORN_NATIVE_SESSION_ROOT"
	// Environment variable names for host tool resolution.
	EnvGitExecutable = "SWORN_GIT"
	EnvBubblewrap    = "SWORN_BWRAP"
	EnvShell         = "SWORN_SH"
	// Environment variable names that would name project-scoped locations.
	// Any non-empty value is refused by RefuseProjectScopeOverrides.
	EnvRecordsRoot   = "SWORN_RECORDS_ROOT"
	EnvJournalsRoot  = "SWORN_JOURNALS_ROOT"
	EnvContractsRoot = "SWORN_CONTRACTS_ROOT"
	EnvCommitPrefix  = "SWORN_COMMIT_PREFIX"
)

var commitPrefixPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,31}$`)

// ProjectConfig declares the project-scoped locations read from the committed
// project file. It deliberately carries no guest-path field.
type ProjectConfig struct {
	SchemaVersion string `json:"schema_version"`
	RecordsRoot   string `json:"records_root"`
	JournalsRoot  string `json:"journals_root"`
	ContractsRoot string `json:"contracts_root"`
	CommitPrefix  string `json:"commit_prefix"`
}

// DefaultProjectConfig returns the documented defaults that apply when no
// project config file exists. They reproduce today's behaviour for an
// unconfigured project apart from the relocations S2 declares.
func DefaultProjectConfig() ProjectConfig {
	return ProjectConfig{
		SchemaVersion: ProjectConfigSchemaVersion,
		RecordsRoot:   DefaultRecordsRoot,
		JournalsRoot:  DefaultJournalsRoot,
		ContractsRoot: DefaultContractsRoot,
		CommitPrefix:  DefaultCommitPrefix,
	}
}

// HostPaths carries the machine/user-scoped locations. It deliberately
// carries no guest-path field.
type HostPaths struct {
	WorkspaceRoot  string
	TempRoot       string
	CredentialsDir string
	ArtefactHome   string
}

// LoadProjectConfig reads the committed project configuration from the
// primary checkout root. A missing file resolves to the documented defaults
// with a false present flag; a present file must parse and validate so that a
// malformed project file fails closed instead of being silently ignored. The
// fixed path is never configurable.
func LoadProjectConfig(repoRoot string) (ProjectConfig, bool, error) {
	// Project-scoped locations can never be overridden from user or
	// environment scope (A5): the committed project file is the only source
	// of records, journals, contracts and commit-prefix truth. Any
	// environment value naming one of them is refused here, the single choke
	// point through which every repository open resolves project truth, so
	// no silent honouring is possible and two operators of one repository
	// always resolve the same project-scoped locations.
	if err := RefuseProjectScopeOverrides(os.Environ()); err != nil {
		return ProjectConfig{}, false, err
	}
	if repoRoot == "" || !filepath.IsAbs(repoRoot) || filepath.Clean(repoRoot) != repoRoot {
		return ProjectConfig{}, false, fail(
			"INVALID_REPOSITORY", "load project config",
			errors.New("repository root must be clean and absolute"),
		)
	}
	path := filepath.Join(repoRoot, filepath.FromSlash(ProjectConfigPath))
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return DefaultProjectConfig(), false, nil
	}
	if err != nil {
		return ProjectConfig{}, false, fail("PROJECT_CONFIG_UNREADABLE", "load project config", err)
	}
	if len(raw) == 0 || len(raw) > 64*1024 || !utf8.Valid(raw) {
		return ProjectConfig{}, false, fail("PROJECT_CONFIG_INVALID", "load project config",
			errors.New("project config must be non-empty UTF-8 under 64 KiB"))
	}
	declared, err := decodeProjectConfig(raw)
	if err != nil {
		return ProjectConfig{}, false, err
	}
	config := DefaultProjectConfig()
	if declared.SchemaVersion != nil {
		if *declared.SchemaVersion != ProjectConfigSchemaVersion {
			return ProjectConfig{}, false, fail("PROJECT_CONFIG_INVALID", "load project config",
				fmt.Errorf("schema_version must be %s", ProjectConfigSchemaVersion))
		}
		config.SchemaVersion = *declared.SchemaVersion
	}
	for name, target := range map[string]*string{
		"records_root":   declared.RecordsRoot,
		"journals_root":  declared.JournalsRoot,
		"contracts_root": declared.ContractsRoot,
	} {
		if target == nil {
			continue
		}
		value := *target
		if err := validateProjectPath(value, name); err != nil {
			return ProjectConfig{}, false, err
		}
		switch name {
		case "records_root":
			config.RecordsRoot = value
		case "journals_root":
			config.JournalsRoot = value
		case "contracts_root":
			config.ContractsRoot = value
		}
	}
	if declared.CommitPrefix != nil {
		value := *declared.CommitPrefix
		if !commitPrefixPattern.MatchString(value) {
			return ProjectConfig{}, false, fail("PROJECT_CONFIG_INVALID", "load project config",
				fmt.Errorf("commit_prefix %q is not a valid commit prefix", value))
		}
		config.CommitPrefix = value
	}
	if err := ValidateConfigPaths(config); err != nil {
		return ProjectConfig{}, false, err
	}
	return config, true, nil
}

type projectConfigFile struct {
	SchemaVersion *string `json:"schema_version"`
	RecordsRoot   *string `json:"records_root"`
	JournalsRoot  *string `json:"journals_root"`
	ContractsRoot *string `json:"contracts_root"`
	CommitPrefix  *string `json:"commit_prefix"`
}

func decodeProjectConfig(raw []byte) (projectConfigFile, error) {
	var declared projectConfigFile
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&declared); err != nil {
		if strings.HasPrefix(err.Error(), "json: unknown field ") {
			return projectConfigFile{}, fail("PROJECT_CONFIG_INVALID", "load project config",
				errors.New("project config contains an unknown field"))
		}
		return projectConfigFile{}, fail("PROJECT_CONFIG_INVALID", "load project config", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != nil && err.Error() != "EOF" {
		return projectConfigFile{}, fail("PROJECT_CONFIG_INVALID", "load project config",
			errors.New("project config has trailing content"))
	}
	return declared, nil
}

func validateProjectPath(value, name string) error {
	if err := ValidatePath(value, false); err != nil {
		return fail("PROJECT_CONFIG_INVALID", "load project config",
			fmt.Errorf("%s must be a canonical repository-relative path: %w", name, err))
	}
	return nil
}

// ValidateConfigPaths rejects project config values that would collide with
// the fixed guest filesystem inside containment or with each other.
func ValidateConfigPaths(config ProjectConfig) error {
	roots := []struct {
		name  string
		value string
	}{
		{"records_root", config.RecordsRoot},
		{"journals_root", config.JournalsRoot},
		{"contracts_root", config.ContractsRoot},
	}
	for _, root := range roots {
		if root.value == "" {
			return fail("PROJECT_CONFIG_INVALID", "validate project config",
				fmt.Errorf("%s must not be empty", root.name))
		}
	}
	if config.RecordsRoot == config.JournalsRoot || config.RecordsRoot == config.ContractsRoot ||
		config.JournalsRoot == config.ContractsRoot {
		return fail("PROJECT_CONFIG_INVALID", "validate project config",
			errors.New("records, journals and contracts roots must be distinct"))
	}
	for _, root := range roots {
		if isGuestPathValue(root.value) {
			return fail("PROJECT_CONFIG_INVALID", "validate project config",
				fmt.Errorf("%s %q names a fixed guest path inside containment", root.name, root.value))
		}
	}
	return nil
}

// guestPathConstants are the compile-time fixed guest filesystem paths inside
// containment that are bound from host surfaces or constructed as fixed
// mounts. They are engine-constructed, never operator territory. The
// authoritative constants live in internal/driver/contract.go; this mirror
// exists only so the config loader can refuse values that would collide with
// them. The driver's guest-path immutability test asserts the two stay in
// agreement against every configuration surface. /home/sworn is deliberately
// absent: it is a virtual --dir inside the guest (never bound from host
// configuration), and a host home directory legitimately shares that name on
// minimal images.
var guestPathConstants = []string{
	"/workspace", "/sworn/inputs", "/sworn",
	"/usr", "/lib", "/lib64", "/etc/ssl/certs", "/tmp", "/proc", "/dev",
}

// guestWorkspaceRoots are the model-visible guest roots beneath which no
// configuration value may ever land: the guest workspace and the guest input
// projection.
var guestWorkspaceRoots = []string{"/workspace", "/sworn/inputs"}

func isGuestPathValue(value string) bool {
	if value == "" {
		return false
	}
	// Exact equality with any fixed guest path, or any value beneath the
	// model-visible guest workspace/input roots, would hand configuration a
	// route into the guest filesystem.
	for _, guest := range guestPathConstants {
		if value == guest {
			return true
		}
	}
	for _, root := range guestWorkspaceRoots {
		if strings.HasPrefix(value, root+"/") {
			return true
		}
	}
	return false
}

// RefuseProjectScopeOverrides rejects any environment override that names a
// project-scoped location. Project-scoped locations are read only from the
// committed project file; a user-scoped or environment value for any of them
// is refused with a named error so two operators of one repository can never
// resolve records, journals, contracts or commit prefixes to different
// places. Host-path environment values are deliberately not refused: those
// locations are machine/user-scoped by design.
func RefuseProjectScopeOverrides(environ []string) error {
	for _, name := range []string{EnvRecordsRoot, EnvJournalsRoot, EnvContractsRoot, EnvCommitPrefix} {
		if value := lookupEnv(environ, name); value != "" {
			return fail(
				"PROJECT_SCOPE_OVERRIDE_REFUSED",
				"resolve project locations",
				fmt.Errorf(
					"%s is project-scoped and must be declared in the committed %s; "+
						"user and environment overrides are refused",
					name, ProjectConfigPath,
				),
			)
		}
	}
	return nil
}

func lookupEnv(environ []string, name string) string {
	prefix := name + "="
	for _, entry := range environ {
		if strings.HasPrefix(entry, prefix) {
			return entry[len(prefix):]
		}
	}
	return ""
}

// LoadHostPaths resolves the machine/user-scoped locations: each honors an
// environment override and otherwise falls back to an XDG-conformant default
// under a sworn subdirectory. The defaults are the intended new machine/user
// defaults per A2.
func LoadHostPaths() (HostPaths, error) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" || !filepath.IsAbs(home) {
		return HostPaths{}, fail("HOST_PATHS_UNAVAILABLE", "resolve host paths",
			errors.New("user home directory is unavailable"))
	}
	stateHome := os.Getenv("XDG_STATE_HOME")
	if stateHome == "" || !filepath.IsAbs(stateHome) {
		stateHome = filepath.Join(home, ".local", "state")
	}
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" || !filepath.IsAbs(configHome) {
		configHome = filepath.Join(home, ".config")
	}
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" || !filepath.IsAbs(dataHome) {
		dataHome = filepath.Join(home, ".local", "share")
	}
	paths := HostPaths{
		WorkspaceRoot:  filepath.Join(stateHome, "sworn", "workspaces"),
		TempRoot:       filepath.Join(stateHome, "sworn", "tmp"),
		CredentialsDir: filepath.Join(configHome, "sworn"),
		ArtefactHome:   filepath.Join(dataHome, "sworn"),
	}
	overrides := []struct {
		name   string
		target *string
	}{
		{EnvWorkspaceRoot, &paths.WorkspaceRoot},
		{EnvTempRoot, &paths.TempRoot},
		{EnvCredentialsDir, &paths.CredentialsDir},
		{EnvArtefactHome, &paths.ArtefactHome},
	}
	for _, override := range overrides {
		if value := os.Getenv(override.name); value != "" {
			if err := validateHostPath(value, override.name); err != nil {
				return HostPaths{}, err
			}
			*override.target = filepath.Clean(value)
		}
	}
	for _, path := range []struct {
		name  string
		value string
	}{{EnvWorkspaceRoot, paths.WorkspaceRoot}, {EnvTempRoot, paths.TempRoot},
		{EnvCredentialsDir, paths.CredentialsDir}, {EnvArtefactHome, paths.ArtefactHome}} {
		if isGuestPathValue(path.value) {
			return HostPaths{}, fail("HOST_PATHS_INVALID", "resolve host paths",
				fmt.Errorf("%s %q names a fixed guest path inside containment", path.name, path.value))
		}
	}
	return paths, nil
}

func validateHostPath(value, name string) error {
	if value == "" || !filepath.IsAbs(value) || filepath.Clean(value) != value ||
		filepath.Clean(value) == string(filepath.Separator) || strings.ContainsRune(value, 0) {
		return fail("HOST_PATHS_INVALID", "resolve host paths",
			fmt.Errorf("%s must be a clean absolute path", name))
	}
	return nil
}

// ValidateHostPathValue validates one machine/user host-path value (an
// environment override): it must be a clean absolute path that is not the
// filesystem root. It is the shared validator for every machine/user-scoped
// location, including overrides resolved outside LoadHostPaths such as the
// native-session memory root.
func ValidateHostPathValue(value, name string) error {
	return validateHostPath(value, name)
}

// RefuseGuestPathValue rejects a machine/user host-path value that names a
// fixed guest path inside containment, so no host location Sworn writes can
// be pointed at the guest filesystem.
func RefuseGuestPathValue(value, name string) error {
	if isGuestPathValue(value) {
		return fail("HOST_PATHS_INVALID", "resolve host paths",
			fmt.Errorf("%s %q names a fixed guest path inside containment", name, value))
	}
	return nil
}

// HostTempDir returns the effective process temp directory (honoring
// TMPDIR). It is the single host-location surface that may resolve from the
// system temp directory: machine/user location resolvers probe it as a
// memory-backed candidate (for example the native-session memory root) so an
// operator with TMPDIR on a tmpfs gets a sensible memory-backed default
// without a layout literal in the consumer.
func HostTempDir() string {
	return os.TempDir()
}

// ResolveTempRoot returns the configured machine/user temp root, creating it
// (0700) when it does not yet exist so os.MkdirTemp consumers find a usable
// parent on a fresh machine.
func ResolveTempRoot() (string, error) {
	paths, err := LoadHostPaths()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(paths.TempRoot, 0o700); err != nil {
		return "", fail("HOST_PATHS_UNAVAILABLE", "resolve temp root", err)
	}
	return paths.TempRoot, nil
}

// ResolveWorkspaceRoot returns the configured machine/user workspace factory
// root, creating it (0700) when it does not yet exist. The workspace owner
// admission still verifies ownership and permissions before use.
func ResolveWorkspaceRoot() (string, error) {
	paths, err := LoadHostPaths()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(paths.WorkspaceRoot, 0o700); err != nil {
		return "", fail("HOST_PATHS_UNAVAILABLE", "resolve workspace root", err)
	}
	return paths.WorkspaceRoot, nil
}

// ResolveGitExecutable resolves the Git executable from the SWORN_GIT
// environment override or discovery (exec.LookPath), returning an absolute
// canonical path. It unifies the previously duplicated LookPath logic in
// cmd/sworn and internal/runtime; gitx.Open's admitGitExecutable remains the
// authority that admits the resolved path.
func ResolveGitExecutable() (string, error) {
	value := os.Getenv(EnvGitExecutable)
	if value == "" {
		path, err := exec.LookPath("git")
		if err != nil {
			return "", fail("GIT_UNAVAILABLE", "resolve Git executable", err)
		}
		value = path
	}
	value, err := filepath.Abs(value)
	if err != nil {
		return "", fail("GIT_UNAVAILABLE", "resolve Git executable", err)
	}
	value, err = filepath.EvalSymlinks(value)
	if err != nil {
		return "", fail("GIT_UNAVAILABLE", "resolve Git executable", err)
	}
	return value, nil
}

// ResolveShellExecutable resolves the POSIX shell from the SWORN_SH
// environment override or discovery (exec.LookPath("sh")), returning an
// absolute canonical path. It never consults an absolute layout literal, so
// a nix, homebrew or minimal host with sh elsewhere on PATH works without
// patching. A configured override that cannot be admitted, or a host with no
// discoverable sh, is refused with a named error rather than silently
// falling back to a hardcoded path.
func ResolveShellExecutable() (string, error) {
	if value := os.Getenv(EnvShell); value != "" {
		resolved, err := canonicalExecutable(value)
		if err != nil {
			return "", fail("SHELL_UNAVAILABLE", "resolve shell executable",
				fmt.Errorf("SWORN_SH %q is not an executable absolute path: %w", value, err))
		}
		return resolved, nil
	}
	path, err := exec.LookPath("sh")
	if err != nil {
		return "", fail("SHELL_UNAVAILABLE", "resolve shell executable",
			errors.New("no POSIX shell found on PATH; set SWORN_SH to an absolute shell path"))
	}
	resolved, err := canonicalExecutable(path)
	if err != nil {
		return "", fail("SHELL_UNAVAILABLE", "resolve shell executable", err)
	}
	return resolved, nil
}

func canonicalExecutable(value string) (string, error) {
	if value == "" || !filepath.IsAbs(value) {
		return "", errors.New("executable must be absolute")
	}
	resolved, err := filepath.EvalSymlinks(value)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return "", errors.New("executable is not an executable regular file")
	}
	return filepath.Clean(resolved), nil
}

// ReservedNames returns the workspace-relative names the containment mask
// must always protect: .git plus the top segment of each configured
// project-scoped root (records and journals). The mask follows the
// configured roots so a relocated records or journals root is never left
// unprotected; .git is always reserved. The result is sorted and de-duplicated
// for a stable argument list.
func ReservedNames(project ProjectConfig) []string {
	set := map[string]bool{".git": true}
	for _, root := range []string{project.RecordsRoot, project.JournalsRoot} {
		if segment := firstPathSegment(root); segment != "" {
			set[segment] = true
		}
	}
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func firstPathSegment(value string) string {
	cleaned := strings.Trim(filepath.ToSlash(filepath.Clean(value)), "/")
	if cleaned == "" || cleaned == "." {
		return ""
	}
	return strings.Split(cleaned, "/")[0]
}
