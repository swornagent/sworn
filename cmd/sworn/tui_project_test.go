package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/journal"
	runtimepkg "github.com/swornagent/sworn/internal/runtime"
)

func TestDiscoverProjectResolvesNestedRootAndSortedReleasesWithoutCreatingState(
	t *testing.T,
) {
	t.Parallel()

	root, head := projectRepositoryFixture(t)
	projectCreateReleaseRef(t, root, "zeta", head)
	projectCreateReleaseRef(t, root, "alpha", head)
	nested := filepath.Join(root, "one", "two")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}

	catalog, err := discoverProject(
		context.Background(), nested, "", "", "",
	)
	if err != nil {
		t.Fatal(err)
	}
	if catalog.paths.root != root || catalog.repository.Root() != root {
		t.Fatalf(
			"project roots = paths %q, repository %q, want %q",
			catalog.paths.root,
			catalog.repository.Root(),
			root,
		)
	}
	if got := projectReleaseNames(catalog.releases); !reflect.DeepEqual(
		got,
		[]string{"alpha", "zeta"},
	) {
		t.Fatalf("release order = %#v", got)
	}
	for index, release := range catalog.releases {
		if release.sourceRef != "refs/heads/release-wt/"+release.name ||
			len(release.runs) != 0 {
			t.Fatalf("release[%d] = %#v", index, release)
		}
	}
	if len(catalog.diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", catalog.diagnostics)
	}
	if _, err := os.Lstat(filepath.Join(root, ".sworn")); !os.IsNotExist(err) {
		t.Fatalf("read-only discovery created .sworn: %v", err)
	}
}

func TestDiscoverProjectAssociatesExactRepositoryRunsAndSelectsLatest(
	t *testing.T,
) {
	t.Parallel()

	root, head := projectRepositoryFixture(t)
	projectCreateReleaseRef(t, root, "delivery", head)
	stateDir := filepath.Join(root, ".sworn")
	if err := os.Mkdir(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	journalPath := filepath.Join(stateDir, "sworn.db")
	store, err := journal.Open(context.Background(), journalPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	createdAt := time.Unix(1_700_000_000, 123).UTC()
	runs := []journal.Run{
		projectJournalRun("run-a", root, "delivery", createdAt),
		projectJournalRun("run-z", root, "delivery", createdAt),
		projectJournalRun(
			"run-foreign",
			filepath.Join(t.TempDir(), "other-repository"),
			"delivery",
			createdAt.Add(time.Hour),
		),
	}
	for _, run := range runs {
		if err := store.RegisterRun(context.Background(), run); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	catalog, err := discoverProject(
		context.Background(), root, "", "", "",
	)
	if err != nil {
		t.Fatal(err)
	}
	release, found := projectFindRelease(catalog.releases, "delivery")
	if !found {
		t.Fatalf("delivery release missing from %#v", catalog.releases)
	}
	if len(release.runs) != 2 ||
		release.runs[0].binding.ID != "run-a" ||
		release.runs[1].binding.ID != "run-z" {
		t.Fatalf("associated runs = %#v", release.runs)
	}
	for _, run := range release.runs {
		if run.binding.Repository != root || run.journalPath != journalPath {
			t.Fatalf("run association = %#v", run)
		}
	}
	latest, found := latestProjectRun(release)
	if !found || latest.binding.ID != "run-z" {
		t.Fatalf("latest run = %#v, found = %t", latest, found)
	}
}

func TestDiscoverProjectKeepsBatonReleasesWhenJournalIsUnavailable(
	t *testing.T,
) {
	t.Parallel()

	root, head := projectRepositoryFixture(t)
	projectCreateReleaseRef(t, root, "delivery", head)
	stateDir := filepath.Join(root, ".sworn")
	if err := os.Mkdir(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	journalPath := filepath.Join(stateDir, "sworn.db")
	if err := os.WriteFile(journalPath, []byte("not a Sworn journal\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	catalog, err := discoverProject(
		context.Background(), root, "", "", "",
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := projectReleaseNames(catalog.releases); !reflect.DeepEqual(
		got,
		[]string{"delivery"},
	) {
		t.Fatalf("releases = %#v", got)
	}
	if !reflect.DeepEqual(catalog.diagnostics, []string{"SWORN_UNAVAILABLE"}) {
		t.Fatalf("diagnostics = %#v", catalog.diagnostics)
	}
	if len(catalog.releases[0].runs) != 0 {
		t.Fatalf("corrupt journal produced runs: %#v", catalog.releases[0].runs)
	}
}

func TestDiscoverProjectRunsDistinguishesMissingFromUnsafeJournal(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	journalPath := filepath.Join(stateDir, "sworn.db")
	paths := projectPaths{journal: journalPath}
	if runs, diagnostic := discoverProjectRuns(context.Background(), paths); len(runs) != 0 || diagnostic != "" {
		t.Fatalf("missing journal = runs %#v, diagnostic %q", runs, diagnostic)
	}

	if err := os.Mkdir(journalPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if runs, diagnostic := discoverProjectRuns(context.Background(), paths); runs != nil || diagnostic != "SWORN_UNAVAILABLE" {
		t.Fatalf("directory journal = runs %#v, diagnostic %q", runs, diagnostic)
	}
	if err := os.Remove(journalPath); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(stateDir, "target.db")
	if err := os.WriteFile(target, []byte("not a journal\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, journalPath); err != nil {
		t.Fatal(err)
	}
	if runs, diagnostic := discoverProjectRuns(context.Background(), paths); runs != nil || diagnostic != "SWORN_UNAVAILABLE" {
		t.Fatalf("symlink journal = runs %#v, diagnostic %q", runs, diagnostic)
	}
}

func TestResolveProjectPathsRequiresCleanAbsoluteOverrides(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	accepted := []string{
		filepath.Join(root, "journal.db"),
		filepath.Join(root, "drivers.json"),
		filepath.Join(root, "runs"),
	}
	paths, err := resolveProjectPaths(
		root, accepted[0], accepted[1], accepted[2],
	)
	if err != nil {
		t.Fatal(err)
	}
	if paths.root != root || paths.journal != accepted[0] ||
		paths.config != accepted[1] || paths.manifestDir != accepted[2] {
		t.Fatalf("resolved paths = %#v", paths)
	}

	separator := string(os.PathSeparator)
	unclean := root + separator + "nested" + separator + ".." +
		separator + "state"
	for _, test := range []struct {
		name        string
		journal     string
		config      string
		manifestDir string
	}{
		{name: "relative journal", journal: "journal.db"},
		{name: "unclean journal", journal: unclean},
		{name: "relative config", config: "drivers.json"},
		{name: "unclean config", config: unclean},
		{name: "relative manifests", manifestDir: "runs"},
		{name: "unclean manifests", manifestDir: unclean},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := resolveProjectPaths(
				root,
				test.journal,
				test.config,
				test.manifestDir,
			); err == nil || err.Error() != "TUI paths must be clean and absolute" {
				t.Fatalf("resolveProjectPaths error = %v", err)
			}
		})
	}
}

func TestDiscoverProjectManifestsAdmitsOnlyExactSafeUniqueRepositoryFiles(
	t *testing.T,
) {
	t.Parallel()

	root, _ := projectRepositoryFixture(t)
	manifestDir := filepath.Join(root, ".sworn", "runs")
	if err := os.MkdirAll(manifestDir, 0o700); err != nil {
		t.Fatal(err)
	}
	projectWriteManifest(t, manifestDir, "exact.json", root, "exact", "run-exact")
	projectWriteManifest(t, manifestDir, "dupe-a.json", root, "dupe", "run-dupe-a")
	projectWriteManifest(t, manifestDir, "dupe-b.json", root, "dupe", "run-dupe-b")
	projectWriteManifest(
		t,
		manifestDir,
		"foreign.json",
		filepath.Join(t.TempDir(), "foreign"),
		"foreign",
		"run-foreign",
	)
	if err := os.Symlink(
		filepath.Join(manifestDir, "exact.json"),
		filepath.Join(manifestDir, "unsafe-link.json"),
	); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(manifestDir, "unsafe-dir.json"), 0o700); err != nil {
		t.Fatal(err)
	}

	paths, err := resolveProjectPaths(root, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	manifests := discoverProjectManifests(paths)
	want := map[string]projectManifest{
		"exact": {
			path: filepath.Join(manifestDir, "exact.json"),
			digest: func() string {
				body, err := readManifest(filepath.Join(manifestDir, "exact.json"))
				if err != nil {
					t.Fatal(err)
				}
				return sha256Digest(body)
			}(),
		},
	}
	if !reflect.DeepEqual(manifests, want) {
		t.Fatalf("manifests = %#v, want %#v", manifests, want)
	}
}

func projectRepositoryFixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	projectGit(t, root, "init", "--quiet", "--initial-branch=main")
	if err := os.WriteFile(
		filepath.Join(root, "README.md"),
		[]byte("fixture\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	projectGit(t, root, "add", "--", "README.md")
	projectGit(
		t,
		root,
		"-c", "user.name=Sworn Project Test",
		"-c", "user.email=sworn-project@example.invalid",
		"commit", "--quiet", "-m", "fixture",
	)
	return root, projectGitOutput(t, root, "rev-parse", "HEAD")
}

func projectCreateReleaseRef(t *testing.T, root, release, head string) {
	t.Helper()
	projectGit(
		t,
		root,
		"update-ref",
		"refs/heads/release-wt/"+release,
		head,
	)
}

func projectGit(t *testing.T, root string, args ...string) {
	t.Helper()
	_ = projectGitOutput(t, root, args...)
}

func projectGitOutput(t *testing.T, root string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func projectJournalRun(
	id, repository, release string,
	createdAt time.Time,
) journal.Run {
	digest := sha256.Sum256([]byte("manifest-" + id))
	return journal.Run{
		ID:             id,
		ManifestDigest: fmt.Sprintf("sha256:%x", digest),
		Repository:     repository,
		Release:        release,
		TargetRef:      "refs/heads/main",
		CreatedAt:      createdAt,
	}
}

func projectWriteManifest(
	t *testing.T,
	directory, name, repository, release, runID string,
) {
	t.Helper()
	profile := driver.RoleSelection{
		Profile: "fixture",
		Model:   "fixture-model",
	}
	manifest := runtimepkg.Manifest{
		SchemaVersion:     runtimepkg.ManifestVersion,
		RunID:             runID,
		Repository:        repository,
		Release:           release,
		TargetRef:         "refs/heads/main",
		Intent:            "Exercise project manifest discovery.",
		MaxParallelTracks: 1,
		Approval: runtimepkg.ApprovalPolicy{
			Repository:          "acme/repo",
			Issue:               1,
			AllowedAuthorIDs:    []int64{1},
			AllowedAssociations: []string{"OWNER"},
		},
		Driver: &runtimepkg.FakeDriverConfig{
			Executable: "/bin/true",
			Digest:     "sha256:" + strings.Repeat("a", 64),
			AdapterKey: "fixture",
			Profile:    "fixture",
		},
		Roles: driver.RoleSelections{
			Planner:     profile,
			Implementer: profile,
			Captain:     profile,
			Verifier:    profile,
		},
		Automation: &runtimepkg.AutomationSelections{
			Recovery: profile,
		},
		Limits: driver.Limits{
			TimeoutMillis: 1,
			OutputBytes:   1,
		},
		Scripts: []runtimepkg.ScriptedAttempt{{
			Responsibility: driver.PlannerProposal,
			BatonAttempt:   1,
			Epoch:          1,
			Try:            1,
			Behavior:       "none",
		}},
	}
	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	body = append(body, '\n')
	if _, err := runtimepkg.ParseManifest(body); err != nil {
		t.Fatalf("fixture manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(directory, name), body, 0o600); err != nil {
		t.Fatal(err)
	}
}

func projectReleaseNames(releases []projectRelease) []string {
	result := make([]string, len(releases))
	for index, release := range releases {
		result[index] = release.name
	}
	return result
}

func projectFindRelease(
	releases []projectRelease,
	name string,
) (projectRelease, bool) {
	for _, release := range releases {
		if release.name == name {
			return release, true
		}
	}
	return projectRelease{}, false
}
