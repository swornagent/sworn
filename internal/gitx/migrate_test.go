package gitx

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// seedLegacyRecords writes the historical .baton/releases/<release>/plan.md
// files at the current head and commits them, then points each release ref at
// the resulting commit. It returns the commit and its tree so tests can
// assert the exact pre-migration shape.
func seedLegacyRecords(t *testing.T, repository *Repository, releases []string) (OID, OID) {
	t.Helper()
	root := repository.Root()
	headText := runTestGit(t, root, nil, "rev-parse", "HEAD")
	head, err := ParseOID(repository.ObjectFormat(), headText)
	if err != nil {
		t.Fatal(err)
	}
	for _, release := range releases {
		path := filepath.Join(root, filepath.FromSlash(LegacyRecordsRoot), release)
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(
			filepath.Join(path, "plan.md"),
			[]byte("plan for "+release+"\n"), 0o644,
		); err != nil {
			t.Fatal(err)
		}
		runTestGit(t, root, nil, "add", "--", filepath.FromSlash(LegacyRecordsRoot))
	}
	runTestGit(t, root, nil, "commit", "--quiet", "-m", "legacy records")
	headText = runTestGit(t, root, nil, "rev-parse", "HEAD")
	head, err = ParseOID(repository.ObjectFormat(), headText)
	if err != nil {
		t.Fatal(err)
	}
	treeText := runTestGit(t, root, nil, "rev-parse", "HEAD^{tree}")
	tree, err := ParseOID(repository.ObjectFormat(), treeText)
	if err != nil {
		t.Fatal(err)
	}
	for _, release := range releases {
		runTestGit(t, root, nil, "update-ref", "refs/heads/release-wt/"+release, headText)
	}
	return head, tree
}

func TestMigrateLegacyRecordsRequiresExplicitConfirmation(t *testing.T) {
	t.Parallel()
	repository, _ := newRepository(t, SHA1)
	seedLegacyRecords(t, repository, []string{"rel-a"})
	if _, err := repository.MigrateLegacyRecords(MigrateRecordsRequest{
		Confirmed: false, Identity: testIdentity,
	}); err == nil {
		t.Fatal("unconfirmed migration ran")
	} else {
		requireGitxErrorCode(t, err, "CONFIRMATION_REQUIRED")
	}
}

func TestMigrateLegacyRecordsRefusesDirtyWorktree(t *testing.T) {
	t.Parallel()
	repository, _ := newRepository(t, SHA1)
	seedLegacyRecords(t, repository, []string{"rel-a"})
	root := repository.Root()
	if err := os.WriteFile(filepath.Join(root, "dirty.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.MigrateLegacyRecords(MigrateRecordsRequest{
		Confirmed: true, Identity: testIdentity,
	}); err == nil {
		t.Fatal("dirty migration ran")
	} else {
		requireGitxErrorCode(t, err, "DIRTY_WORKTREE")
	}
}

func TestMigrateLegacyRecordsRefusesWhenNothingToMigrate(t *testing.T) {
	t.Parallel()
	repository, _ := newRepository(t, SHA1)
	if _, err := repository.MigrateLegacyRecords(MigrateRecordsRequest{
		Confirmed: true, Identity: testIdentity,
	}); err == nil {
		t.Fatal("empty migration ran")
	} else {
		requireGitxErrorCode(t, err, "NOTHING_TO_MIGRATE")
	}
}

func TestMigrateLegacyRecordsRefusesOverwrite(t *testing.T) {
	t.Parallel()
	repository, _ := newRepository(t, SHA1)
	seedLegacyRecords(t, repository, []string{"rel-a"})
	root := repository.Root()
	// A record already relocated to the configured root must block the move.
	target := filepath.Join(root, filepath.FromSlash(DefaultRecordsRoot), "rel-a")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "plan.md"), []byte("already\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, root, nil, "add", "--", filepath.FromSlash(DefaultRecordsRoot))
	runTestGit(t, root, nil, "commit", "--quiet", "-m", "already migrated")
	if _, err := repository.MigrateLegacyRecords(MigrateRecordsRequest{
		Confirmed: true, Identity: testIdentity,
	}); err == nil {
		t.Fatal("overwrite migration ran")
	} else {
		requireGitxErrorCode(t, err, "RECORD_ALREADY_MIGRATED")
	}
}

func TestMigrateLegacyRecordsExactTransitionMarkerAndIdempotency(t *testing.T) {
	t.Parallel()
	repository, _ := newRepository(t, SHA1)
	before, _ := seedLegacyRecords(t, repository, []string{"rel-a", "rel-b"})

	migration, err := repository.MigrateLegacyRecords(MigrateRecordsRequest{
		Confirmed: true, Identity: testIdentity,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(migration.Releases, []string{"rel-a", "rel-b"}) {
		t.Fatalf("migrated releases = %v", migration.Releases)
	}
	root := repository.Root()

	// The current branch head is the migration commit with the marker subject.
	headText := runTestGit(t, root, nil, "rev-parse", "HEAD")
	if headText != migration.Commit.String() {
		t.Fatalf("HEAD = %s, want migration commit %s", headText, migration.Commit)
	}
	subject := runTestGit(t, root, nil, "log", "-1", "--format=%s")
	if subject != MigrationMarkerSubject {
		t.Fatalf("marker subject = %q", subject)
	}
	if parent := runTestGit(t, root, nil, "rev-parse", "HEAD^"); parent != before.String() {
		t.Fatalf("migration parent = %s, want %s", parent, before)
	}

	// Every relocated plan exists under .sworn/records.
	for _, release := range []string{"rel-a", "rel-b"} {
		body := runTestGit(t, root, nil, "show", "HEAD:"+DefaultRecordsRoot+"/"+release+"/plan.md")
		if body != "plan for "+release {
			t.Fatalf("%s record = %q", release, body)
		}
	}
	entries, err := repository.ListTree(migration.Commit)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Path, LegacyRecordsRoot) {
			t.Fatalf("legacy path %s remains after migration", entry.Path)
		}
	}

	// Each release ref head carries its own relocated plan and no .baton tree.
	for _, release := range []string{"rel-a", "rel-b"} {
		ref := "refs/heads/release-wt/" + release
		refHead := runTestGit(t, root, nil, "rev-parse", ref)
		body := runTestGit(t, root, nil, "show", refHead+":"+DefaultRecordsRoot+"/"+release+"/plan.md")
		if body != "plan for "+release {
			t.Fatalf("%s ref record = %q", ref, body)
		}
		refTree := runTestGit(t, root, nil, "rev-parse", refHead+"^{tree}")
		raw := runTestGit(t, root, nil, "ls-tree", "-r", "--name-only", refTree)
		if strings.Contains(raw, LegacyRecordsRoot) {
			t.Fatalf("%s tree still contains %s", ref, LegacyRecordsRoot)
		}
	}

	// Idempotency: a second confirmed run refuses.
	if _, err := repository.MigrateLegacyRecords(MigrateRecordsRequest{
		Confirmed: true, Identity: testIdentity,
	}); err == nil {
		t.Fatal("second migration ran")
	} else {
		requireGitxErrorCode(t, err, "NOTHING_TO_MIGRATE")
	}
}

func TestMigrateLegacyRecordsToConfiguredNonDefaultRoot(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	git, err := ResolveGitExecutable()
	if err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(git, "init", "--quiet", "--initial-branch=main", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	configBody := `{
  "schema_version": "` + ProjectConfigSchemaVersion + `",
  "records_root": ".custom/records",
  "journals_root": ".sworn",
  "contracts_root": "contracts",
  "commit_prefix": "sworn",
  "documents_root": "docs/sworn"
}
`
	if err := os.MkdirAll(filepath.Join(root, "docs", "sworn"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(ProjectConfigPath)), []byte(configBody), 0o644); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, root, nil, "add", "--", filepath.FromSlash(ProjectConfigPath))
	runTestGit(t, root, nil, "commit", "--quiet", "-m", "add config")

	repository, err := Open(root, git)
	if err != nil {
		t.Fatal(err)
	}
	if repository.RecordRoot() != ".custom/records" {
		t.Fatalf("repository RecordRoot() = %q, want .custom/records", repository.RecordRoot())
	}

	before, _ := seedLegacyRecords(t, repository, []string{"rel-custom"})
	migration, err := repository.MigrateLegacyRecords(MigrateRecordsRequest{
		Confirmed: true, Identity: testIdentity,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(migration.Releases, []string{"rel-custom"}) {
		t.Fatalf("migrated releases = %v", migration.Releases)
	}

	// The current branch head is the migration commit with the marker subject.
	headText := runTestGit(t, root, nil, "rev-parse", "HEAD")
	if headText != migration.Commit.String() {
		t.Fatalf("HEAD = %s, want migration commit %s", headText, migration.Commit)
	}
	if parent := runTestGit(t, root, nil, "rev-parse", "HEAD^"); parent != before.String() {
		t.Fatalf("migration parent = %s, want %s", parent, before)
	}

	// Relocated plan exists under .custom/records.
	body := runTestGit(t, root, nil, "show", "HEAD:.custom/records/rel-custom/plan.md")
	if body != "plan for rel-custom" {
		t.Fatalf("custom record = %q", body)
	}

	// No plan under .sworn/records or .baton/releases.
	raw := runTestGit(t, root, nil, "ls-tree", "-r", "--name-only", "HEAD")
	if strings.Contains(raw, DefaultRecordsRoot) {
		t.Fatalf("default records root %s was created in custom migration", DefaultRecordsRoot)
	}
	if strings.Contains(raw, LegacyRecordsRoot) {
		t.Fatalf("legacy records root %s remains after custom migration", LegacyRecordsRoot)
	}

	// Release ref carries custom record.
	ref := "refs/heads/release-wt/rel-custom"
	refHead := runTestGit(t, root, nil, "rev-parse", ref)
	refBody := runTestGit(t, root, nil, "show", refHead+":.custom/records/rel-custom/plan.md")
	if refBody != "plan for rel-custom" {
		t.Fatalf("custom ref record = %q", refBody)
	}

	// Idempotency.
	if _, err := repository.MigrateLegacyRecords(MigrateRecordsRequest{
		Confirmed: true, Identity: testIdentity,
	}); err == nil {
		t.Fatal("second migration ran")
	} else {
		requireGitxErrorCode(t, err, "NOTHING_TO_MIGRATE")
	}
}
