package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/journal"
	runtimepkg "github.com/swornagent/sworn/internal/runtime"
)

const (
	maxProjectManifests = 128
)

type projectPaths struct {
	root           string
	journal        string
	config         string
	manifestDir    string
	operatorConfig string
}

type projectRun struct {
	binding     journal.Run
	journalPath string
	configPath  string
}

type projectRelease struct {
	name           string
	sourceRef      string
	runs           []projectRun
	manifest       string
	manifestDigest string
	diagnostic     string
}

type projectCatalog struct {
	paths          projectPaths
	repository     *gitx.Repository
	releases       []projectRelease
	diagnostics    []string
	operatorConfig string
}

func discoverProject(
	ctx context.Context,
	startPath, journalPath, configPath, manifestDir string,
) (projectCatalog, error) {
	if ctx == nil {
		return projectCatalog{}, errors.New("project context is unavailable")
	}
	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		return projectCatalog{}, errors.New("Git is unavailable")
	}
	if startPath == "" {
		startPath, err = os.Getwd()
		if err != nil {
			return projectCatalog{}, errors.New("project directory is unavailable")
		}
	}
	startPath, err = filepath.Abs(startPath)
	if err != nil {
		return projectCatalog{}, errors.New("project directory is unavailable")
	}
	repository, err := gitx.Open(filepath.Clean(startPath), gitExecutable)
	if err != nil {
		return projectCatalog{}, errors.New("current directory is not inside an admitted Git project")
	}
	paths, err := resolveProjectPaths(
		repository.Root(), journalPath, configPath, manifestDir,
	)
	if err != nil {
		return projectCatalog{}, err
	}

	releaseRefs, err := baton.ListReleaseRefs(
		baton.UseGitRepository(repository),
	)
	if err != nil {
		return projectCatalog{}, errors.New("Sworn release catalog is unavailable")
	}
	byRelease := make(map[string]*projectRelease, len(releaseRefs))
	for _, releaseRef := range releaseRefs {
		release := &projectRelease{
			name:      releaseRef.Release,
			sourceRef: releaseRef.Ref,
		}
		byRelease[release.name] = release
	}

	manifests := discoverProjectManifests(paths)
	for release, manifest := range manifests {
		entry := byRelease[release]
		if entry == nil {
			entry = &projectRelease{
				name:       release,
				diagnostic: "BATON_UNAVAILABLE",
			}
			byRelease[release] = entry
		}
		entry.manifest = manifest.path
		entry.manifestDigest = manifest.digest
		if manifest.diagnostic != "" {
			entry.diagnostic = manifest.diagnostic
		}
	}

	var diagnostics []string
	operatorConfigPath, operatorDiagnostic := discoverOperatorConfig(paths)
	if operatorDiagnostic != "" {
		diagnostics = append(diagnostics, operatorDiagnostic)
	}

	runs, journalDiagnostic := discoverProjectRuns(ctx, paths)
	if journalDiagnostic != "" {
		diagnostics = append(diagnostics, journalDiagnostic)
	}
	for _, run := range runs {
		if run.Repository != paths.root {
			continue
		}
		entry := byRelease[run.Release]
		if entry == nil {
			entry = &projectRelease{
				name:       run.Release,
				diagnostic: "BATON_UNAVAILABLE",
			}
			byRelease[run.Release] = entry
		}
		entry.runs = append(entry.runs, projectRun{
			binding:     run.Run,
			journalPath: run.journalPath,
			configPath:  existingRegularFile(paths.config),
		})
	}

	releases := make([]projectRelease, 0, len(byRelease))
	for _, release := range byRelease {
		releases = append(releases, *release)
	}
	sort.Slice(releases, func(left, right int) bool {
		return releases[left].name < releases[right].name
	})
	return projectCatalog{
		paths: paths, repository: repository,
		releases: releases, diagnostics: diagnostics,
		operatorConfig: operatorConfigPath,
	}, nil
}

func resolveProjectPaths(
	root, journalPath, configPath, manifestDir string,
) (projectPaths, error) {
	// The journal, driver-config and manifest files live under the
	// configured journals root (default .sworn) so an operator can relocate
	// the project's run state with the committed project file.
	project, _, err := gitx.LoadProjectConfig(root)
	if err != nil {
		return projectPaths{}, err
	}
	journalsRoot := filepath.FromSlash(project.JournalsRoot)
	defaults := projectPaths{
		root:           root,
		journal:        filepath.Join(root, journalsRoot, "sworn.db"),
		config:         filepath.Join(root, journalsRoot, "drivers.json"),
		manifestDir:    filepath.Join(root, journalsRoot, "runs"),
		operatorConfig: filepath.Join(root, journalsRoot, "operator.json"),
	}
	for _, override := range []struct {
		value       string
		destination *string
	}{
		{journalPath, &defaults.journal},
		{configPath, &defaults.config},
		{manifestDir, &defaults.manifestDir},
	} {
		value, destination := override.value, override.destination
		if value == "" {
			continue
		}
		if !filepath.IsAbs(value) || filepath.Clean(value) != value {
			return projectPaths{}, errors.New("TUI paths must be clean and absolute")
		}
		*destination = value
	}
	return defaults, nil
}

func discoverOperatorConfig(paths projectPaths) (string, string) {
	if paths.operatorConfig == "" {
		return "", ""
	}
	info, err := os.Lstat(paths.operatorConfig)
	if errors.Is(err, os.ErrNotExist) {
		return "", ""
	}
	if err != nil || !validOperatorFileInfo(info) {
		return "", "OPERATOR_CONFIG_UNAVAILABLE"
	}
	if _, err := loadOperatorSettings(paths.operatorConfig); err != nil {
		return "", "OPERATOR_CONFIG_UNAVAILABLE"
	}
	return paths.operatorConfig, ""
}

type discoveredRun struct {
	journal.Run
	journalPath string
}

func discoverProjectRuns(
	ctx context.Context,
	paths projectPaths,
) ([]discoveredRun, string) {
	info, err := os.Lstat(paths.journal)
	if err == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
		return nil, "SWORN_UNAVAILABLE"
	}
	journalDir := filepath.Dir(paths.journal)
	entries, err := os.ReadDir(journalDir)
	if errors.Is(err, os.ErrNotExist) {
		if os.IsNotExist(err) {
			return []discoveredRun{}, ""
		}
		return nil, "SWORN_UNAVAILABLE"
	}
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []discoveredRun{}, ""
		}
		return nil, "SWORN_UNAVAILABLE"
	}

	seenPaths := make(map[string]bool)
	var candidatePaths []string
	if info != nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
		candidatePaths = append(candidatePaths, paths.journal)
		seenPaths[paths.journal] = true
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == "sworn.db" || strings.HasSuffix(name, ".db") || strings.HasSuffix(name, ".sqlite") {
			fullPath := filepath.Join(journalDir, name)
			if !seenPaths[fullPath] {
				fileInfo, statErr := os.Lstat(fullPath)
				if statErr == nil && fileInfo.Mode().IsRegular() && fileInfo.Mode()&os.ModeSymlink == 0 {
					candidatePaths = append(candidatePaths, fullPath)
					seenPaths[fullPath] = true
				}
			}
		}
	}
	sort.Strings(candidatePaths)

	var allRuns []discoveredRun
	seenRunIDs := make(map[string]bool)
	for _, candPath := range candidatePaths {
		store, err := journal.OpenReadOnly(ctx, candPath)
		if err != nil {
			if len(candidatePaths) == 1 && candPath == paths.journal {
				return nil, "SWORN_UNAVAILABLE"
			}
			continue
		}
		runs, err := store.RunBindings(ctx)
		_ = store.Close()
		if err != nil {
			if len(candidatePaths) == 1 && candPath == paths.journal {
				return nil, "SWORN_UNAVAILABLE"
			}
			continue
		}
		for _, r := range runs {
			if !seenRunIDs[r.ID] {
				seenRunIDs[r.ID] = true
				allRuns = append(allRuns, discoveredRun{
					Run:         r,
					journalPath: candPath,
				})
			}
		}
	}
	return allRuns, ""
}

type projectManifest struct {
	path       string
	digest     string
	diagnostic string
}

func discoverProjectManifests(paths projectPaths) map[string]projectManifest {
	result := make(map[string]projectManifest)
	duplicates := make(map[string]bool)
	entries, err := os.ReadDir(paths.manifestDir)
	if errors.Is(err, os.ErrNotExist) {
		return result
	}
	if err != nil || len(entries) > maxProjectManifests {
		return result
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(paths.manifestDir, entry.Name())
		body, readErr := readManifest(path)
		if readErr != nil {
			continue
		}
		identity, parseErr := runtimepkg.InspectManifestIdentity(body)
		if parseErr != nil || identity.Repository != paths.root {
			continue
		}
		if duplicates[identity.Release] {
			continue
		}
		if _, duplicate := result[identity.Release]; duplicate {
			delete(result, identity.Release)
			duplicates[identity.Release] = true
			continue
		}
		diagnostic := ""
		if identity.SchemaVersion == runtimepkg.ManifestVersionV2 ||
			identity.SchemaVersion == runtimepkg.ManifestVersionV3 ||
			identity.SchemaVersion == runtimepkg.ManifestVersionV4 {
			diagnostic = "MIGRATION_REQUIRED"
		}
		result[identity.Release] = projectManifest{
			path: path, digest: sha256Digest(body), diagnostic: diagnostic,
		}
	}
	return result
}

func existingRegularFile(path string) string {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() ||
		info.Mode()&os.ModeSymlink != 0 {
		return ""
	}
	return path
}

func latestProjectRun(release projectRelease) (projectRun, bool) {
	if len(release.runs) == 0 {
		return projectRun{}, false
	}
	latest := release.runs[0]
	for _, candidate := range release.runs[1:] {
		if candidate.binding.CreatedAt.After(latest.binding.CreatedAt) ||
			(candidate.binding.CreatedAt.Equal(latest.binding.CreatedAt) &&
				candidate.binding.ID > latest.binding.ID) {
			latest = candidate
		}
	}
	return latest, true
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
