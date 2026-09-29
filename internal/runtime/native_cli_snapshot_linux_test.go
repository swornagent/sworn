//go:build linux

package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/journal"
)

type nativeCLISnapshotRunFixture struct {
	ctx      context.Context
	config   driver.LoadedDriverConfig
	manifest admittedManifest
	store    *journal.Store
	host     string
	versions string
	snapshot string
	now      time.Time
}

// newNativeCLISnapshotRunFixture starts one production run whose every role
// uses a run-snapshot Claude adapter. The host CLI is a launcher symlink into
// a versions directory, the shape a self-updating CLI installs, and the
// snapshot store lives under a per-test artefact home.
func newNativeCLISnapshotRunFixture(t *testing.T) *nativeCLISnapshotRunFixture {
	t.Helper()
	root := t.TempDir()
	artefactHome := filepath.Join(root, "artefacts")
	t.Setenv(gitx.EnvArtefactHome, artefactHome)
	fixture := &nativeCLISnapshotRunFixture{
		ctx:      context.Background(),
		host:     filepath.Join(root, "bin", "claude"),
		versions: filepath.Join(root, "versions"),
		snapshot: filepath.Join(artefactHome, "native-cli-snapshots"),
		now:      time.Date(2026, 9, 26, 1, 2, 3, 0, time.UTC),
	}
	for _, directory := range []string{filepath.Dir(fixture.host), fixture.versions} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	fixture.install(t, "2.1.280")
	var runtimeFiles []driver.PinnedRuntimeFile
	var required []string
	for _, target := range []string{
		"/etc/hosts",
		"/etc/nsswitch.conf",
		"/etc/resolv.conf",
		"/etc/ssl/certs/ca-certificates.crt",
	} {
		body := []byte("runtime " + target)
		pathValue := filepath.Join(
			root, "runtime-"+strings.ReplaceAll(target[1:], "/", "-"),
		)
		if err := os.WriteFile(pathValue, body, 0o600); err != nil {
			t.Fatal(err)
		}
		runtimeFiles = append(runtimeFiles, driver.PinnedRuntimeFile{
			Path: pathValue, Target: target, Digest: sha256Digest(body),
		})
		required = append(required, target)
	}
	credential := "claude-file"
	body, err := driver.EncodeDriverConfig(driver.DriverConfig{
		SchemaVersion: driver.DriverConfigSchemaVersion,
		Credentials: []driver.DriverCredentialSource{{
			Key: credential, Kind: driver.CredentialFile,
			Reference: filepath.Join(root, "claude.json"),
		}},
		Adapters: []driver.DriverAdapterConfig{{Native: &driver.NativeAdapterConfig{
			Key: "claude", ID: "sworn.claude", Version: "1.0.0",
			Family:                 driver.ProfileClaude,
			CLI:                    driver.ExecutableIdentity{Path: fixture.host},
			RuntimeFiles:           runtimeFiles,
			RequiredRuntimeTargets: required,
			CredentialTarget:       driver.ClaudeCredentialTarget,
			CredentialRefs:         []string{credential},
			MaxCredentialBytes:     1_048_576,
			CLIResolution:          driver.NativeCLIResolutionRunSnapshot,
		}}},
		Profiles: []driver.DriverProfile{{
			Key: "planner", Adapter: "claude", Network: driver.NetworkRequired,
			CredentialSource: &credential,
			CertificationModels: []string{
				"implementer-model", "lead-model", "planner-model", "verifier-model",
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.config, err = driver.DecodeDriverConfig(body)
	if err != nil {
		t.Fatal(err)
	}
	fixture.manifest = productionManifest(t, productionRepository(t), fixture.config)
	fixture.store, err = journal.Open(
		fixture.ctx, filepath.Join(t.TempDir(), "journal.sqlite"),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fixture.store.Close() })
	value := fixture.manifest.value
	if err := fixture.store.RegisterRun(fixture.ctx, journal.Run{
		ID: value.RunID, ManifestDigest: fixture.manifest.digest,
		Repository: value.Repository, Release: value.Release,
		TargetRef: value.TargetRef, CreatedAt: fixture.now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := fixture.store.RecordCommand(fixture.ctx, journal.Command{
		RunID: value.RunID, ReplayKey: "manifest", Kind: "start",
		Payload: fixture.manifest.raw, CreatedAt: fixture.now,
	}); err != nil {
		t.Fatal(err)
	}
	return fixture
}

// install writes one CLI version and repoints the launcher at it atomically,
// the way a self-updating CLI does.
func (fixture *nativeCLISnapshotRunFixture) install(t *testing.T, version string) {
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
}

// service opens a fresh Service over the fixture's journal, as a serve
// restart does: a new driver runtime with no in-memory registry.
func (fixture *nativeCLISnapshotRunFixture) service(t *testing.T) *Service {
	t.Helper()
	reloaded, err := driver.DecodeDriverConfig(fixture.config.CanonicalJSON())
	if err != nil {
		t.Fatal(err)
	}
	production, err := newProductionDriverRuntime(
		reloaded, driver.DriverFactoryOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		t.Fatal(err)
	}
	return &Service{
		journal: fixture.store, production: production,
		gitExecutable: gitExecutable,
		now:           func() time.Time { return fixture.now },
	}
}

// plannerIdentity opens the engine and returns the adapter identity the
// planner dispatches through; it binds the snapshot's path and digest.
func (fixture *nativeCLISnapshotRunFixture) plannerIdentity(
	t *testing.T,
	service *Service,
) driver.AdapterIdentity {
	t.Helper()
	engine, err := service.openEngine(fixture.manifest)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	selected, err := engine.registry.Resolve(
		fixture.manifest.value.Roles, driver.RolePlanner,
	)
	if err != nil {
		t.Fatal(err)
	}
	return selected.Adapter
}

// recordedFacts returns the run's snapshot commands and events.
func (fixture *nativeCLISnapshotRunFixture) recordedFacts(
	t *testing.T,
) ([]journal.Command, []journal.Event) {
	t.Helper()
	snapshot, err := fixture.store.Snapshot(fixture.ctx, fixture.manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}
	var commands []journal.Command
	for _, command := range snapshot.Commands {
		if command.Kind == nativeCLISnapshotCommandKind {
			commands = append(commands, command)
		}
	}
	var events []journal.Event
	for _, event := range snapshot.Events {
		if event.Kind == nativeCLISnapshotEventKind {
			events = append(events, event)
		}
	}
	return commands, events
}

func (fixture *nativeCLISnapshotRunFixture) storeEntries(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(fixture.snapshot)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(entries))
	for index, entry := range entries {
		names[index] = entry.Name()
	}
	return names
}

func requireRuntimeRefusal(t *testing.T, err error, code, detail string) {
	t.Helper()
	var runtimeErr *Error
	if !errors.As(err, &runtimeErr) || runtimeErr.Code != code ||
		runtimeErr.Detail != detail {
		t.Fatalf("refusal = %v, want %s/%s", err, code, detail)
	}
}

// A drive cycle records the run's snapshot fact before it opens the engine,
// and so before any dispatch can happen. A paused run stops right after the
// engine opens, which isolates exactly that step.
func TestDriveCycleRecordsTheNativeCLISnapshotBeforeTheEngineOpens(t *testing.T) {
	fixture := newNativeCLISnapshotRunFixture(t)
	runID := fixture.manifest.value.RunID
	if _, err := fixture.store.ApplyControl(fixture.ctx, journal.ControlCommand{
		RunID: runID, ID: "pause-1", Kind: journal.Pause, ExpectedGeneration: 0,
	}, fixture.now); err != nil {
		t.Fatal(err)
	}
	owner, err := fixture.store.AcquireOwner(
		fixture.ctx, runID, fixture.now, time.Minute, false,
	)
	if err != nil {
		t.Fatal(err)
	}
	service := fixture.service(t)
	if _, err := service.driveOwnedCycle(fixture.ctx, runID, owner); err != nil {
		t.Fatalf("paused drive cycle = %v", err)
	}
	commands, events := fixture.recordedFacts(t)
	if len(commands) != 1 || len(events) != 1 ||
		commands[0].ReplayKey != "native-cli-snapshot/claude" ||
		string(events[0].Body) != string(commands[0].Payload) {
		t.Fatalf("snapshot facts = %+v / %+v", commands, events)
	}
	fact, err := driver.DecodeNativeCLISnapshot(commands[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(filepath.Join(fixture.versions, "2.1.280"))
	if err != nil {
		t.Fatal(err)
	}
	if fact.RunID != runID || fact.Adapter != "claude" ||
		fact.SourcePath != fixture.host || fact.Digest != sha256Digest(source) ||
		fact.CLIVersion != "2.1.280" ||
		fact.SnapshotPath != filepath.Join(
			fixture.snapshot, strings.TrimPrefix(fact.Digest, "sha256:"),
		) {
		t.Fatalf("recorded fact = %+v", fact)
	}
}

// A restart or retry in the same run reuses the recorded snapshot. The host
// CLI updating itself, and then disappearing altogether, changes nothing the
// run executes, and no second fact or copy is ever made.
func TestRestartReusesTheRecordedNativeCLISnapshotWithoutTheHost(t *testing.T) {
	fixture := newNativeCLISnapshotRunFixture(t)
	first := fixture.service(t)
	if err := first.recordNativeCLISnapshots(fixture.ctx, fixture.manifest); err != nil {
		t.Fatal(err)
	}
	identity := fixture.plannerIdentity(t, first)
	entries := fixture.storeEntries(t)
	if len(entries) != 1 ||
		!strings.Contains(identity.ConfigurationDigest, "sha256:") {
		t.Fatalf("store = %v, identity = %+v", entries, identity)
	}

	fixture.install(t, "2.1.281")
	updated := fixture.service(t)
	if err := updated.recordNativeCLISnapshots(fixture.ctx, fixture.manifest); err != nil {
		t.Fatal(err)
	}
	if got := fixture.plannerIdentity(t, updated); got != identity {
		t.Fatalf("host update changed the run's CLI identity: %+v != %+v", got, identity)
	}

	if err := os.RemoveAll(fixture.versions); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(fixture.host); err != nil {
		t.Fatal(err)
	}
	restarted := fixture.service(t)
	if err := restarted.recordNativeCLISnapshots(fixture.ctx, fixture.manifest); err != nil {
		t.Fatalf("restart consulted the removed host CLI: %v", err)
	}
	if got := fixture.plannerIdentity(t, restarted); got != identity {
		t.Fatalf("restart changed the run's CLI identity: %+v != %+v", got, identity)
	}
	commands, events := fixture.recordedFacts(t)
	if len(commands) != 1 || len(events) != 1 {
		t.Fatalf("snapshot facts after restarts = %d commands, %d events",
			len(commands), len(events))
	}
	if got := fixture.storeEntries(t); len(got) != 1 || got[0] != entries[0] {
		t.Fatalf("store after restarts = %v, want %v", got, entries)
	}
}

// A run's recorded snapshot is never taken again: a missing or changed
// snapshot file fails the engine closed with a typed refusal, and the next
// drive cycle does not repair it from the host.
func TestOpenEngineFailsClosedOnAMissingOrChangedNativeCLISnapshot(t *testing.T) {
	fixture := newNativeCLISnapshotRunFixture(t)
	service := fixture.service(t)
	_, err := service.openEngine(fixture.manifest)
	requireRuntimeRefusal(t, err, "NATIVE_CLI_SNAPSHOT_INVALID", "snapshot_unrecorded")

	if err := service.recordNativeCLISnapshots(fixture.ctx, fixture.manifest); err != nil {
		t.Fatal(err)
	}
	commands, _ := fixture.recordedFacts(t)
	fact, err := driver.DecodeNativeCLISnapshot(commands[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(fact.SnapshotPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		fact.SnapshotPath,
		[]byte("#!/usr/bin/sh\necho '9.9.9 (Claude Code)'\n"),
		0o755,
	); err != nil {
		t.Fatal(err)
	}
	restarted := fixture.service(t)
	if err := restarted.recordNativeCLISnapshots(fixture.ctx, fixture.manifest); err != nil {
		t.Fatal(err)
	}
	_, err = restarted.openEngine(fixture.manifest)
	requireRuntimeRefusal(t, err, "NATIVE_CLI_SNAPSHOT_INVALID", "snapshot_changed")

	if err := os.Remove(fact.SnapshotPath); err != nil {
		t.Fatal(err)
	}
	restarted = fixture.service(t)
	if err := restarted.recordNativeCLISnapshots(fixture.ctx, fixture.manifest); err != nil {
		t.Fatal(err)
	}
	_, err = restarted.openEngine(fixture.manifest)
	requireRuntimeRefusal(t, err, "NATIVE_CLI_SNAPSHOT_INVALID", "snapshot_missing")
	if _, err := os.Lstat(fact.SnapshotPath); !os.IsNotExist(err) {
		t.Fatalf("a missing snapshot was taken again: %v", err)
	}
	if commands, events := fixture.recordedFacts(t); len(commands) != 1 || len(events) != 1 {
		t.Fatalf("snapshot facts = %d commands, %d events", len(commands), len(events))
	}
}
