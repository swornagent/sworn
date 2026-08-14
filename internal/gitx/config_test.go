package gitx

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// defaultConfigFixture writes a project config file whose declared fields
// equal the documented defaults, so callers can prove "absent the file, the
// engine behaves exactly as if the defaults had been written" (A1).
func defaultConfigFixture(t *testing.T, root string) {
	t.Helper()
	body := `{
  "schema_version": "` + ProjectConfigSchemaVersion + `",
  "records_root": ".baton/releases",
  "journals_root": ".sworn",
  "contracts_root": "contracts",
  "commit_prefix": "baton"
}
`
	if err := os.MkdirAll(filepath.Join(root, "docs", "sworn"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(ProjectConfigPath)), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadProjectConfigAbsentFileResolvesExactlyDefaults(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	config, found, err := LoadProjectConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("absent project file reported as found")
	}
	if !reflect.DeepEqual(config, DefaultProjectConfig()) {
		t.Fatalf("absent-file config = %#v, want exactly the defaults %#v", config, DefaultProjectConfig())
	}
	if config.RecordsRoot != DefaultRecordsRoot ||
		config.JournalsRoot != DefaultJournalsRoot ||
		config.ContractsRoot != DefaultContractsRoot ||
		config.CommitPrefix != DefaultCommitPrefix {
		t.Fatalf("defaults not reproduced: %#v", config)
	}
}

func TestLoadProjectConfigDefaultsFileBehavesExactlyLikeAbsent(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	defaultConfigFixture(t, root)
	config, found, err := LoadProjectConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("present project file reported as absent")
	}
	if !reflect.DeepEqual(config, DefaultProjectConfig()) {
		t.Fatalf("defaults-file config = %#v, want exactly the defaults %#v", config, DefaultProjectConfig())
	}
}

func TestLoadProjectConfigHonorsDeclaredValues(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	body := `{
  "records_root": ".sworn/records",
  "journals_root": ".sworn/state",
  "contracts_root": "docs/specs",
  "commit_prefix": "sworn"
}
`
	if err := os.MkdirAll(filepath.Join(root, "docs", "sworn"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(ProjectConfigPath)), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	config, found, err := LoadProjectConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("present project file reported as absent")
	}
	if config.RecordsRoot != ".sworn/records" ||
		config.JournalsRoot != ".sworn/state" ||
		config.ContractsRoot != "docs/specs" ||
		config.CommitPrefix != "sworn" {
		t.Fatalf("declared values not honored: %#v", config)
	}
}

func TestLoadProjectConfigRejectsMalformedAndUnsafeFiles(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		body string
		code string
	}{
		"unknown field": {
			body: `{"records_root": ".baton/releases", "containment_binary": "/usr/bin/bwrap"}`,
			code: "PROJECT_CONFIG_INVALID",
		},
		"invalid json": {
			body: `{"records_root": `,
			code: "PROJECT_CONFIG_INVALID",
		},
		"bad records path": {
			body: `{"records_root": "../escape"}`,
			code: "PROJECT_CONFIG_INVALID",
		},
		"absolute records path": {
			body: `{"records_root": "/absolute"}`,
			code: "PROJECT_CONFIG_INVALID",
		},
		"git records path": {
			body: `{"records_root": ".git/records"}`,
			code: "PROJECT_CONFIG_INVALID",
		},
		"bad commit prefix": {
			body: `{"commit_prefix": "not a prefix"}`,
			code: "PROJECT_CONFIG_INVALID",
		},
		"empty commit prefix": {
			body: `{"commit_prefix": ""}`,
			code: "PROJECT_CONFIG_INVALID",
		},
		"wrong schema": {
			body: `{"schema_version": "sworn.project-config/v9"}`,
			code: "PROJECT_CONFIG_INVALID",
		},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.MkdirAll(filepath.Join(root, "docs", "sworn"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(ProjectConfigPath)), []byte(test.body), 0o644); err != nil {
				t.Fatal(err)
			}
			_, _, err := LoadProjectConfig(root)
			if test.code == "" {
				if err != nil {
					t.Fatalf("expected valid config, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error code %s, got nil", test.code)
			}
			var typed *Error
			if !errors.As(err, &typed) || typed.Code != test.code {
				t.Fatalf("error = %v, want code %s", err, test.code)
			}
		})
	}
}

func TestLoadProjectConfigRejectsGuestPathValues(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"/workspace", "/sworn/inputs", "/sworn/inputs/x"} {
		if !isGuestPathValue(value) {
			t.Fatalf("isGuestPathValue(%q) = false, want true", value)
		}
	}
	// A host home directory under /home/sworn is legitimate and not a guest
	// path (the host and guest namespaces are distinct).
	if isGuestPathValue("/home/sworn/.local/state/sworn/workspaces") {
		t.Fatal("host path under /home/sworn misclassified as a guest path")
	}
	if isGuestPathValue("/home/sworn") {
		t.Fatal("host home misclassified as a guest path")
	}
}

func TestLoadHostPathsResolvesXDGDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, name := range []string{
		"XDG_STATE_HOME", "XDG_CACHE_HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME",
		EnvWorkspaceRoot, EnvTempRoot, EnvCredentialsDir, EnvArtefactHome,
	} {
		t.Setenv(name, "")
	}
	paths, err := LoadHostPaths()
	if err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(home, ".local", "state")
	config := filepath.Join(home, ".config")
	data := filepath.Join(home, ".local", "share")
	if paths.WorkspaceRoot != filepath.Join(state, "sworn", "workspaces") {
		t.Fatalf("workspace root = %q", paths.WorkspaceRoot)
	}
	if paths.TempRoot != filepath.Join(state, "sworn", "tmp") {
		t.Fatalf("temp root = %q, want XDG_STATE_HOME/sworn/tmp", paths.TempRoot)
	}
	if paths.CredentialsDir != filepath.Join(config, "sworn") {
		t.Fatalf("credentials dir = %q", paths.CredentialsDir)
	}
	if paths.ArtefactHome != filepath.Join(data, "sworn") {
		t.Fatalf("artefact home = %q", paths.ArtefactHome)
	}
}

func TestLoadHostPathsHonorsEnvironmentOverrides(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv(EnvWorkspaceRoot, filepath.Join(home, "custom", "workspaces"))
	t.Setenv(EnvTempRoot, filepath.Join(home, "custom", "tmp"))
	t.Setenv(EnvCredentialsDir, filepath.Join(home, "custom", "creds"))
	t.Setenv(EnvArtefactHome, filepath.Join(home, "custom", "artefacts"))
	paths, err := LoadHostPaths()
	if err != nil {
		t.Fatal(err)
	}
	if paths.WorkspaceRoot != filepath.Join(home, "custom", "workspaces") ||
		paths.TempRoot != filepath.Join(home, "custom", "tmp") ||
		paths.CredentialsDir != filepath.Join(home, "custom", "creds") ||
		paths.ArtefactHome != filepath.Join(home, "custom", "artefacts") {
		t.Fatalf("overrides not honored: %#v", paths)
	}
}

func TestLoadHostPathsRefusesRelativeAndGuestOverrides(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cases := map[string]struct {
		name  string
		value string
	}{
		"relative workspace": {EnvWorkspaceRoot, "relative"},
		"relative temp":      {EnvTempRoot, "tmp"},
		"guest workspace":    {EnvWorkspaceRoot, "/workspace"},
		"guest inputs":       {EnvTempRoot, "/sworn/inputs"},
		"guest sworn":        {EnvCredentialsDir, "/sworn"},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			for _, n := range []string{EnvWorkspaceRoot, EnvTempRoot, EnvCredentialsDir, EnvArtefactHome} {
				t.Setenv(n, "")
			}
			t.Setenv(test.name, test.value)
			if _, err := LoadHostPaths(); err == nil {
				t.Fatalf("override %s=%q admitted", test.name, test.value)
			} else {
				var typed *Error
				if !errors.As(err, &typed) || typed.Code != "HOST_PATHS_INVALID" {
					t.Fatalf("error = %v, want HOST_PATHS_INVALID", err)
				}
			}
		})
	}
}

func TestRefuseProjectScopeOverridesNamesEachField(t *testing.T) {
	t.Parallel()
	for _, name := range []string{EnvRecordsRoot, EnvJournalsRoot, EnvContractsRoot, EnvCommitPrefix} {
		err := RefuseProjectScopeOverrides([]string{"PATH=/usr/bin", name + "=relocated"})
		if err == nil {
			t.Fatalf("%s override admitted", name)
		}
		var typed *Error
		if !errors.As(err, &typed) || typed.Code != "PROJECT_SCOPE_OVERRIDE_REFUSED" {
			t.Fatalf("error = %v, want PROJECT_SCOPE_OVERRIDE_REFUSED", err)
		}
		if !strings.Contains(err.Error(), name) {
			t.Fatalf("error %q does not name the refused field %s", err, name)
		}
	}
	if err := RefuseProjectScopeOverrides([]string{"PATH=/usr/bin", EnvWorkspaceRoot + "=/tmp/x"}); err != nil {
		t.Fatalf("host-path override wrongly refused: %v", err)
	}
}

func TestLoadProjectConfigRefusesEnvironmentOverride(t *testing.T) {
	root := t.TempDir()
	t.Setenv(EnvRecordsRoot, ".sworn/records")
	t.Cleanup(func() { t.Setenv(EnvRecordsRoot, "") })
	if _, _, err := LoadProjectConfig(root); err == nil {
		t.Fatal("SWORN_RECORDS_ROOT override admitted")
	} else {
		var typed *Error
		if !errors.As(err, &typed) || typed.Code != "PROJECT_SCOPE_OVERRIDE_REFUSED" {
			t.Fatalf("error = %v, want PROJECT_SCOPE_OVERRIDE_REFUSED", err)
		}
	}
}

func TestReservedNamesFollowConfiguredRoots(t *testing.T) {
	t.Parallel()
	defaults := ReservedNames(DefaultProjectConfig())
	if !reflect.DeepEqual(defaults, []string{".baton", ".git", ".sworn"}) {
		t.Fatalf("default reserved names = %v, want [.baton .git .sworn]", defaults)
	}
	configured := ReservedNames(ProjectConfig{
		RecordsRoot: ".sworn/records", JournalsRoot: ".sworn/state",
		ContractsRoot: "docs/specs", CommitPrefix: "sworn",
	})
	if !reflect.DeepEqual(configured, []string{".git", ".sworn"}) {
		t.Fatalf("configured reserved names = %v, want [.git .sworn]", configured)
	}
	distinct := ReservedNames(ProjectConfig{
		RecordsRoot: ".records", JournalsRoot: ".journals", ContractsRoot: "contracts",
	})
	if !reflect.DeepEqual(distinct, []string{".git", ".journals", ".records"}) {
		t.Fatalf("distinct reserved names = %v", distinct)
	}
}

func TestCommitPrefixDefaultsAndConfiguredValues(t *testing.T) {
	t.Parallel()
	// Test the accessor defaults on a zero repository plus the configured
	// value on a loaded one.
	var zero *Repository
	if zero.CommitPrefix() != DefaultCommitPrefix {
		t.Fatalf("zero commit prefix = %q", zero.CommitPrefix())
	}
	if zero.CandidateCommitPrefix() != candidateCommitPrefixDefault {
		t.Fatalf("zero candidate prefix = %q", zero.CandidateCommitPrefix())
	}
	config := ProjectConfig{
		SchemaVersion: ProjectConfigSchemaVersion,
		RecordsRoot:   ".baton/releases", JournalsRoot: ".sworn",
		ContractsRoot: "contracts", CommitPrefix: "sworn",
	}
	repo := &Repository{config: config, configured: true, recordRoot: config.RecordsRoot}
	if repo.CommitPrefix() != "sworn" || repo.CandidateCommitPrefix() != "sworn" {
		t.Fatalf("configured prefixes = %q / %q", repo.CommitPrefix(), repo.CandidateCommitPrefix())
	}
}

// TestOpenResolvesConfiguredRecordRootThroughAdmission proves the A1 records
// root flows from a committed project file into the admission surface: a
// configured records root is honored by ResolveRecordPathAdmission and the
// plan path, and differs from the default.
func TestOpenResolvesConfiguredRecordRootThroughAdmission(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs", "sworn"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(ProjectConfigPath)),
		[]byte(`{"records_root": ".sworn/records"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	git, err := ResolveGitExecutable()
	if err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(git, "init", "--quiet", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	repository, err := Open(root, git)
	if err != nil {
		t.Fatal(err)
	}
	if repository.recordRoot != ".sworn/records" {
		t.Fatalf("repository record root = %q, want configured .sworn/records", repository.recordRoot)
	}
	admission, err := repository.ResolveRecordPathAdmission()
	if err != nil {
		t.Fatal(err)
	}
	if admission.Root() != ".sworn/records" {
		t.Fatalf("admission root = %q, want configured root", admission.Root())
	}
	if !reflect.DeepEqual(repository.ReservedNames(), []string{".git", ".sworn"}) {
		t.Fatalf("reserved names = %v", repository.ReservedNames())
	}
}

// TestNoProductionTempLiteralsOutsideConfig asserts the A2 whole-tree
// property: production (non-test) source files outside config.go never
// resolve a temp or workspace location from os.TempDir() or os.MkdirTemp("")
// literals.
func TestNoProductionTempLiteralsOutsideConfig(t *testing.T) {
	t.Parallel()
	root := moduleRootDir(t)
	var offenders []string
	patterns := []string{"os.MkdirTemp(\"\"", "os.TempDir()"}
	for _, packageDir := range []string{
		filepath.Join("internal", "gitx"),
		filepath.Join("internal", "driver"),
		filepath.Join("internal", "runtime"),
		filepath.Join("internal", "baton"),
		filepath.Join("internal", "skill"),
		filepath.Join("cmd", "sworn"),
	} {
		dir := filepath.Join(root, packageDir)
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") ||
				strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			if packageDir == filepath.Join("internal", "gitx") && entry.Name() == "config.go" {
				continue
			}
			body, err := os.ReadFile(filepath.Join(dir, entry.Name()))
			if err != nil {
				t.Fatal(err)
			}
			for _, pattern := range patterns {
				if strings.Contains(string(body), pattern) {
					offenders = append(offenders, filepath.Join(packageDir, entry.Name())+" ("+pattern+")")
				}
			}
		}
	}
	if len(offenders) != 0 {
		t.Fatalf("production files resolve temp/workspace locations from literals: %v", offenders)
	}
}

func moduleRootDir(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}
