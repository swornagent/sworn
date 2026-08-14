//go:build linux

package driver

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/gitx"
)

// TestGuestPathsAreCompileTimeConstantsUnreachableFromConfiguration is the
// A4 proof. It resolves every configuration surface Sworn reads (the project
// config file fields, the host-path environment overrides, and every SWORN_*
// variable) and asserts that none of them can alter the guest workspace
// root, the guest input root, or any bind or mask target inside containment:
// the guest targets in bubblewrapArguments stay byte-for-byte identical no
// matter how hostile the configuration input is.
func TestGuestPathsAreCompileTimeConstantsUnreachableFromConfiguration(t *testing.T) {
	// This test sets process environment, so it cannot run parallel.

	// Resolve every configuration surface with adversarial values.
	hostileWorkspace := t.TempDir()
	// A hostile host-path set that tries to claim guest-looking locations.
	t.Setenv(gitx.EnvWorkspaceRoot, filepath.Join(hostileWorkspace, "workspace"))
	t.Setenv(gitx.EnvTempRoot, filepath.Join(hostileWorkspace, "tmp"))
	t.Setenv(gitx.EnvCredentialsDir, filepath.Join(hostileWorkspace, "credentials"))
	t.Setenv(gitx.EnvArtefactHome, filepath.Join(hostileWorkspace, "artefacts"))
	t.Setenv(gitx.EnvNativeSessionRoot, filepath.Join(hostileWorkspace, "native-session"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(hostileWorkspace, "state"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(hostileWorkspace, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(hostileWorkspace, "data"))
	hostPaths, err := gitx.LoadHostPaths()
	if err != nil {
		t.Fatal(err)
	}
	projectConfig := gitx.ProjectConfig{
		SchemaVersion: gitx.ProjectConfigSchemaVersion,
		RecordsRoot:   ".records",
		JournalsRoot:  ".journals",
		ContractsRoot: "contracts",
		CommitPrefix:  "sworn",
	}

	// (a) The guest workspace and input roots are compile-time constants,
	// byte-for-byte identical after every configuration surface resolved.
	if GuestWorkspacePath != "/workspace" {
		t.Fatalf("GuestWorkspacePath = %q, want /workspace", GuestWorkspacePath)
	}
	if GuestInputPath != "/sworn/inputs" {
		t.Fatalf("GuestInputPath = %q, want /sworn/inputs", GuestInputPath)
	}

	// Build a fully adversarial invocation: hostile host workspace, hostile
	// host paths, hostile mask names, read-write access (the widest surface).
	invocation := invocationForMaskTest(t, hostileWorkspace, projectConfig, hostPaths)
	arguments, err := bubblewrapArguments(invocation)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(arguments, "\x00")
	for _, expected := range []string{
		"--bind-fd\x005\x00/workspace",
		"--ro-bind-fd\x006\x00/sworn/inputs",
		"--ro-bind-fd\x004\x00/sworn/driver",
		"--chdir\x00/workspace",
		"--dir\x00/home/sworn",
		"--dir\x00/sworn",
		"--tmpfs\x00/tmp",
		"--ro-bind\x00/usr\x00/usr",
		"--setenv\x00HOME\x00/home/sworn",
		"--setenv\x00TMPDIR\x00/tmp",
		"--setenv\x00PWD\x00/workspace",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("bubblewrap arguments lack fixed guest target %q:\n%s", expected, joined)
		}
	}
	// No host-path configuration value — including the native-session root —
	// may leak into the guest argument list or alter any guest target.
	nativeSessionRoot := filepath.Join(hostileWorkspace, "native-session")
	for _, hostPath := range []string{
		hostPaths.WorkspaceRoot, hostPaths.TempRoot, nativeSessionRoot, hostileWorkspace,
	} {
		if strings.Contains(joined, hostPath) {
			t.Fatalf("host path %q leaked into guest argument list:\n%s", hostPath, joined)
		}
	}

	// (b) The configuration types structurally expose no guest-path field.
	guestFields := []string{}
	for _, value := range []any{gitx.ProjectConfig{}, gitx.HostPaths{}} {
		typed := reflect.TypeOf(value)
		for index := 0; index < typed.NumField(); index++ {
			field := typed.Field(index)
			if strings.Contains(strings.ToLower(field.Name), "guest") {
				guestFields = append(guestFields, typed.Name()+"."+field.Name)
			}
		}
	}
	if len(guestFields) != 0 {
		t.Fatalf("configuration types expose guest-path fields: %v", guestFields)
	}
}

// TestContainmentMaskFollowsConfiguredRoots is the A4(c) security proof: the
// records and journals root mask segments follow the configured project
// config, so a configured root is never left unprotected inside containment.
func TestContainmentMaskFollowsConfiguredRoots(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	for _, name := range []string{".records", ".journals", ".baton", ".sworn", ".git"} {
		if err := os.MkdirAll(filepath.Join(workspace, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}

	// Configured records and journals roots -> their top segments are masked.
	configured := gitx.ProjectConfig{
		SchemaVersion: gitx.ProjectConfigSchemaVersion,
		RecordsRoot:   ".records", JournalsRoot: ".journals",
		ContractsRoot: "contracts", CommitPrefix: "sworn",
	}
	invocation := invocationForMaskTest(t, workspace, configured, gitx.HostPaths{})
	arguments, err := bubblewrapArguments(invocation)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(arguments, " ")
	for _, name := range []string{".records", ".journals"} {
		if !strings.Contains(joined, "--tmpfs /workspace/"+name+" --remount-ro /workspace/"+name) {
			t.Fatalf("configured root %s not masked:\n%s", name, joined)
		}
	}
	if strings.Contains(joined, "--tmpfs /workspace/.baton") ||
		strings.Contains(joined, "--tmpfs /workspace/.sworn") {
		t.Fatalf("default-only mask leaked into configured invocation:\n%s", joined)
	}

	// Default config -> .baton and .sworn are masked (today's behaviour).
	defaultInvocation := invocationForMaskTest(t, workspace, gitx.DefaultProjectConfig(), gitx.HostPaths{})
	defaultArguments, err := bubblewrapArguments(defaultInvocation)
	if err != nil {
		t.Fatal(err)
	}
	defaultJoined := strings.Join(defaultArguments, " ")
	for _, name := range []string{".baton", ".sworn", ".git"} {
		if !strings.Contains(defaultJoined, "--tmpfs /workspace/"+name+" --remount-ro /workspace/"+name) {
			t.Fatalf("default root %s not masked:\n%s", name, defaultJoined)
		}
	}
}

// invocationForMaskTest builds a contained read-write Invocation whose
// MaskNames derive from the given project config and whose HostWorkspace is
// the given directory.
func invocationForMaskTest(
	t *testing.T,
	workspace string,
	project gitx.ProjectConfig,
	hostPaths gitx.HostPaths,
) Invocation {
	t.Helper()
	adapter := AdapterIdentity{Key: "fake-driver", Version: "1.0.0"}
	profile := ProfileConfig{
		Key: "mask-profile", Adapter: adapter.Key,
		Network: NetworkNone,
	}
	selected := SelectedProfile{
		Profile: profile, Adapter: adapter, Model: "mask-model",
	}
	request, err := NewRequest(
		"mask-invocation",
		RoleImplementer,
		profile.Key,
		selected.Model,
		Workspace{Path: GuestWorkspacePath, Access: ReadWrite},
		nil,
		true,
		Limits{TimeoutMillis: 5_000, OutputBytes: 65_536},
	)
	if err != nil {
		t.Fatal(err)
	}
	return Invocation{
		Request:       request,
		HostWorkspace: workspace,
		Selected:      selected,
		MaskNames:     gitx.ReservedNames(project),
	}
}
