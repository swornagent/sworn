//go:build linux

package driver

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/gitx"
)

type nativeSnapshotFixture struct {
	loaded   LoadedDriverConfig
	host     string
	versions string
	store    string
}

// newNativeSnapshotFixture configures one run-snapshot Claude adapter whose
// host path is a launcher symlink into a versions directory, the shape an
// auto-updating CLI installs. The snapshot store lives under a per-test
// artefact home.
func newNativeSnapshotFixture(t *testing.T, version string) nativeSnapshotFixture {
	t.Helper()
	root := t.TempDir()
	artefactHome := filepath.Join(root, "artefacts")
	t.Setenv(gitx.EnvArtefactHome, artefactHome)
	fixture := nativeSnapshotFixture{
		host:     filepath.Join(root, "bin", "claude"),
		versions: filepath.Join(root, "versions"),
		store:    filepath.Join(artefactHome, nativeCLISnapshotStore),
	}
	for _, directory := range []string{filepath.Dir(fixture.host), fixture.versions} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	fixture.install(t, version)
	runtimeFiles := driverRuntimeFilesFixture(t, root)
	required := make([]string, len(runtimeFiles))
	for index := range runtimeFiles {
		required[index] = runtimeFiles[index].Target
	}
	credential := "claude-file"
	body, err := EncodeDriverConfig(DriverConfig{
		SchemaVersion: DriverConfigSchemaVersion,
		Credentials: []DriverCredentialSource{{
			Key: credential, Kind: CredentialFile,
			Reference: filepath.Join(root, "claude.json"),
		}},
		Adapters: []DriverAdapterConfig{{Native: &NativeAdapterConfig{
			Key: "a-claude", ID: "sworn.claude", Version: "1.0.0",
			Family:                 ProfileClaude,
			CLI:                    ExecutableIdentity{Path: fixture.host},
			RuntimeFiles:           runtimeFiles,
			RequiredRuntimeTargets: required,
			CredentialTarget:       ClaudeCredentialTarget,
			CredentialRefs:         []string{credential},
			MaxCredentialBytes:     1_048_576,
			CLIResolution:          NativeCLIResolutionRunSnapshot,
		}}},
		Profiles: []DriverProfile{{
			Key: "claude", Adapter: "a-claude", Network: NetworkRequired,
			CredentialSource:    &credential,
			CertificationModels: []string{"model-claude"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.loaded, err = DecodeDriverConfig(body)
	if err != nil {
		t.Fatal(err)
	}
	return fixture
}

// install writes one CLI version and repoints the launcher symlink at it
// atomically, the way a self-updating CLI does.
func (fixture nativeSnapshotFixture) install(t *testing.T, version string) string {
	t.Helper()
	executable := filepath.Join(fixture.versions, version)
	script := "#!/usr/bin/sh\necho '" + version + " (Claude Code)'\n"
	if err := os.WriteFile(executable, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	link := fixture.host + ".next"
	if err := os.Symlink(executable, link); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(link, fixture.host); err != nil {
		t.Fatal(err)
	}
	return executable
}

func (fixture nativeSnapshotFixture) nativeAdapter(
	t *testing.T,
	snapshots map[string]NativeCLISnapshot,
) (ConfiguredDriverRegistry, *nativeAdapter) {
	t.Helper()
	registry, err := fixture.loaded.BuildRegistry(
		[]string{"claude"},
		DriverFactoryOptions{NativeCLISnapshots: snapshots},
	)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := registry.ResolveSelection(
		ModelSelection{Profile: "claude", Model: "model-claude"},
	)
	if err != nil {
		t.Fatal(err)
	}
	adapter, ok := selected.adapter.(*nativeAdapter)
	if !ok {
		t.Fatalf("selected adapter = %T", selected.adapter)
	}
	return registry, adapter
}

func TestSnapshotNativeCLICopiesTheHostBinaryOnceIntoAContentAddressedStore(t *testing.T) {
	fixture := newNativeSnapshotFixture(t, "2.1.280")
	source := filepath.Join(fixture.versions, "2.1.280")
	sourceBytes, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := fixture.loaded.SnapshotNativeCLI(
		context.Background(), "a-claude", "run-1",
	)
	if err != nil {
		t.Fatal(err)
	}
	want := Digest(sourceBytes)
	if snapshot.Digest != want ||
		snapshot.SnapshotPath != filepath.Join(fixture.store, strings.TrimPrefix(want, "sha256:")) ||
		snapshot.SourcePath != fixture.host || snapshot.RunID != "run-1" ||
		snapshot.Adapter != "a-claude" || snapshot.Family != ProfileClaude ||
		snapshot.CLIVersion != "2.1.280" ||
		snapshot.VersionOutput != "2.1.280 (Claude Code)" {
		t.Fatalf("snapshot fact = %+v", snapshot)
	}
	info, err := os.Lstat(snapshot.SnapshotPath)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o555 {
		t.Fatalf("snapshot file = %v, %v; want a read-only regular file", info, err)
	}
	copied, err := os.ReadFile(snapshot.SnapshotPath)
	if err != nil || !bytes.Equal(copied, sourceBytes) {
		t.Fatalf("snapshot bytes differ from the host binary: %v", err)
	}
	body, err := EncodeNativeCLISnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if decoded, err := DecodeNativeCLISnapshot(body); err != nil || decoded != snapshot {
		t.Fatalf("fact round trip = %+v, %v", decoded, err)
	}

	// Content addressing: the same bytes resolve to the same entry, and the
	// store holds exactly one file, with no partial copy left behind.
	again, err := fixture.loaded.SnapshotNativeCLI(
		context.Background(), "a-claude", "run-2",
	)
	if err != nil {
		t.Fatal(err)
	}
	if again.SnapshotPath != snapshot.SnapshotPath || again.Digest != snapshot.Digest {
		t.Fatalf("second snapshot = %+v, want the same entry", again)
	}
	entries, err := os.ReadDir(fixture.store)
	if err != nil || len(entries) != 1 {
		t.Fatalf("store entries = %v, %v; want exactly one", entries, err)
	}
}

// Every dispatch in a run executes the snapshot: after the host CLI updates
// itself, the run's adapter still opens, hashes and runs the bytes it
// recorded, while a new preview reports the new host version.
func TestRunSnapshotAdapterKeepsExecutingItsSnapshotAfterTheHostUpdates(t *testing.T) {
	fixture := newNativeSnapshotFixture(t, "2.1.280")
	snapshot, err := fixture.loaded.SnapshotNativeCLI(
		context.Background(), "a-claude", "run-1",
	)
	if err != nil {
		t.Fatal(err)
	}
	snapshots := map[string]NativeCLISnapshot{"a-claude": snapshot}
	registry, adapter := fixture.nativeAdapter(t, snapshots)
	if adapter.config.CLI != (ExecutableIdentity{
		Path: snapshot.SnapshotPath, Digest: snapshot.Digest,
	}) || adapter.config.CLIVersion != "2.1.280" ||
		adapter.config.VersionOutput != "2.1.280 (Claude Code)" ||
		adapter.config.CLIResolution != "" {
		t.Fatalf("run adapter is not bound to its snapshot: %+v", adapter.config)
	}

	fixture.install(t, "2.1.281")

	closure, err := openNativeClosure(adapter.config)
	if err != nil {
		t.Fatalf("snapshot closure refused after a host update: %v", err)
	}
	closeNativeFiles(closure)
	body, err := nativeVersion(context.Background(), adapter.config)
	if err != nil || string(body) != "2.1.280 (Claude Code)\n" {
		t.Fatalf("snapshot version after a host update = %q, %v", body, err)
	}
	report := registry.Doctor(context.Background(), "claude", "model-claude")
	if report.State != ReadinessPass || report.CLICompatibility == nil ||
		report.CLICompatibility.CLIVersion != "2.1.280" {
		t.Fatalf("run doctor after a host update = %+v", report)
	}
	_, restarted := fixture.nativeAdapter(t, snapshots)
	if restarted.identity != adapter.identity {
		t.Fatalf("rebinding the recorded snapshot changed identity: %+v != %+v",
			restarted.identity, adapter.identity)
	}

	previews, err := fixture.loaded.PreviewNativeCLISnapshots(
		context.Background(), []string{"claude"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if previews["a-claude"].CLIVersion != "2.1.281" {
		t.Fatalf("preview after a host update = %+v", previews["a-claude"])
	}
}

func TestVerifyNativeCLISnapshotFailsClosedOnAMissingOrChangedSnapshot(t *testing.T) {
	fixture := newNativeSnapshotFixture(t, "2.1.280")
	snapshot, err := fixture.loaded.SnapshotNativeCLI(
		context.Background(), "a-claude", "run-1",
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyNativeCLISnapshot(snapshot); err != nil {
		t.Fatalf("intact snapshot refused: %v", err)
	}

	if err := os.Chmod(snapshot.SnapshotPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		snapshot.SnapshotPath,
		[]byte("#!/usr/bin/sh\necho '9.9.9 (Claude Code)'\n"),
		0o755,
	); err != nil {
		t.Fatal(err)
	}
	requireNativeRefusal(
		t, VerifyNativeCLISnapshot(snapshot),
		"NATIVE_CLI_SNAPSHOT_INVALID", "snapshot_changed",
	)
	// The store never overwrites a corrupted entry with fresh bytes.
	_, err = fixture.loaded.SnapshotNativeCLI(
		context.Background(), "a-claude", "run-2",
	)
	requireNativeRefusal(t, err, "NATIVE_CLI_SNAPSHOT_INVALID", "snapshot_changed")

	if err := os.Remove(snapshot.SnapshotPath); err != nil {
		t.Fatal(err)
	}
	requireNativeRefusal(
		t, VerifyNativeCLISnapshot(snapshot),
		"NATIVE_CLI_SNAPSHOT_INVALID", "snapshot_missing",
	)
}

func TestBuildRegistryRefusesARunSnapshotAdapterWithoutItsSnapshot(t *testing.T) {
	fixture := newNativeSnapshotFixture(t, "2.1.280")
	_, err := fixture.loaded.BuildRegistry(
		[]string{"claude"}, DriverFactoryOptions{},
	)
	requireNativeRefusal(t, err, "NATIVE_CLI_SNAPSHOT_INVALID", "snapshot_unbound")

	snapshot, err := fixture.loaded.SnapshotNativeCLI(
		context.Background(), "a-claude", "run-1",
	)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.SourcePath = filepath.Join(fixture.versions, "2.1.280")
	_, err = fixture.loaded.BuildRegistry(
		[]string{"claude"},
		DriverFactoryOptions{
			NativeCLISnapshots: map[string]NativeCLISnapshot{"a-claude": snapshot},
		},
	)
	requireNativeRefusal(t, err, "NATIVE_CLI_SNAPSHOT_INVALID", "binding_mismatch")
}

// A declaration that carries a pinned field beside the opt-in is refused
// when the registry binds it, not overwritten by the snapshot.
func TestBuildRegistryRefusesAPinnedFieldBesideARunSnapshot(t *testing.T) {
	fixture := newNativeSnapshotFixture(t, "2.1.280")
	snapshot, err := fixture.loaded.SnapshotNativeCLI(
		context.Background(), "a-claude", "run-1",
	)
	if err != nil {
		t.Fatal(err)
	}
	config := fixture.loaded.config
	native := cloneNativeAdapterConfig(*config.Adapters[0].Native)
	native.CLI.Digest = snapshot.Digest
	config.Adapters = []DriverAdapterConfig{{Native: &native}}
	body, err := EncodeDriverConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := DecodeDriverConfig(body)
	if err != nil {
		t.Fatal(err)
	}
	_, err = loaded.BuildRegistry(
		[]string{"claude"},
		DriverFactoryOptions{
			NativeCLISnapshots: map[string]NativeCLISnapshot{"a-claude": snapshot},
		},
	)
	requireNativeRefusal(t, err, "INVALID_ADAPTER", "cli_resolution_pinned")
}

// The readiness commands resolve the host CLI the way a run would, without
// copying it, and report its version through the usual compatibility field.
func TestPreviewNativeCLISnapshotsReportsTheHostCLIWithoutCopyingIt(t *testing.T) {
	fixture := newNativeSnapshotFixture(t, "2.1.280")
	previews, err := fixture.loaded.PreviewNativeCLISnapshots(
		context.Background(), []string{"claude"},
	)
	if err != nil {
		t.Fatal(err)
	}
	preview := previews["a-claude"]
	if preview.SnapshotPath != filepath.Join(fixture.versions, "2.1.280") ||
		preview.RunID != "" || preview.CLIVersion != "2.1.280" {
		t.Fatalf("preview = %+v", preview)
	}
	if _, err := os.Lstat(fixture.store); !os.IsNotExist(err) {
		t.Fatalf("a preview wrote the snapshot store: %v", err)
	}
	registry, _ := fixture.nativeAdapter(t, previews)
	report := registry.Doctor(context.Background(), "claude", "model-claude")
	if report.State != ReadinessPass || report.CLICompatibility == nil ||
		report.CLICompatibility.CLIVersion != "2.1.280" ||
		report.CLICompatibility.Status != NativeCLIUntested {
		t.Fatalf("doctor over a preview = %+v", report)
	}
	if none, err := fixture.loaded.PreviewNativeCLISnapshots(
		context.Background(), []string{"unknown"},
	); err != nil || none != nil {
		t.Fatalf("preview of no run-snapshot adapter = %v, %v", none, err)
	}

	if err := os.Remove(fixture.host); err != nil {
		t.Fatal(err)
	}
	_, err = fixture.loaded.PreviewNativeCLISnapshots(
		context.Background(), []string{"claude"},
	)
	requireNativeRefusal(
		t, err, "NATIVE_CLI_SNAPSHOT_UNAVAILABLE", "source_unavailable",
	)
	_, err = fixture.loaded.SnapshotNativeCLI(
		context.Background(), "a-claude", "run-1",
	)
	requireNativeRefusal(
		t, err, "NATIVE_CLI_SNAPSHOT_UNAVAILABLE", "source_unavailable",
	)
}

func TestSnapshotNativeCLIRefusesAnUnreadableVersion(t *testing.T) {
	fixture := newNativeSnapshotFixture(t, "2.1.280")
	executable := filepath.Join(fixture.versions, "broken")
	if err := os.WriteFile(
		executable, []byte("#!/usr/bin/sh\necho 'not a version'\n"), 0o755,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(fixture.host); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(executable, fixture.host); err != nil {
		t.Fatal(err)
	}
	_, err := fixture.loaded.SnapshotNativeCLI(
		context.Background(), "a-claude", "run-1",
	)
	requireNativeRefusal(t, err, "NATIVE_CLI_SNAPSHOT_UNAVAILABLE", "version_output")
}
