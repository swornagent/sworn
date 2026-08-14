// Package skill installs and migrates the one thin Sworn agent skill.
//
// The skill is a discovery and transport adapter only: it recognizes
// Sworn-governed work and routes an agent to the local Sworn CLI or MCP
// service. It contains no reducer, receipt writer, role instruction,
// verification verdict, or merge procedure; Sworn's command service remains
// the sole lifecycle authority.
package skill

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/gitx"
)

// Name is the one supported Sworn skill directory name.
const Name = "sworn"

// LegacyNames are the standalone Baton role skills this installer recognizes
// for safe migration. Sworn never reads them.
var LegacyNames = []string{
	"baton-plan",
	"baton-implement",
	"baton-design-review",
	"baton-verify",
	"baton-merge",
}

const (
	legacyMarkerBegin  = "<!-- baton-skill\n"
	legacyGeneratorKey = "generator-version: baton.skill-generator/v1"
	migratedMarkerLine = "<!-- sworn-migrated-skill\n"
	markerEnd          = "-->\n"
)

// SupportedRoots returns the supported skill install roots under homeDir, in
// the precedence order an agent's skill discovery is expected to search.
func SupportedRoots(homeDir string) []string {
	return []string{
		filepath.Join(homeDir, ".claude", "skills"),
		filepath.Join(homeDir, ".agents", "skills"),
	}
}

// CollisionError reports a legacy skill copy that installation cannot safely
// migrate because it no longer matches its recognized generated shape.
type CollisionError struct {
	Path string
}

func (e *CollisionError) Error() string {
	return fmt.Sprintf(
		"an existing skill at %s does not match a recognized generated Baton or Sworn skill; "+
			"resolve it manually before installing the sworn skill",
		e.Path,
	)
}

// InstallArtefact places the sworn skill artefact in the configured
// machine/user artefact home (SWORN_ARTEFACT_HOME or the XDG-conformant
// default). It is additive to Install: the agent-discovery install under the
// user's home stays intact so agents keep finding the skill, while Sworn's
// own artefact home also carries a copy as its user-scoped artefact. It
// returns the installed path.
func InstallArtefact() (string, error) {
	paths, err := gitx.LoadHostPaths()
	if err != nil {
		return "", fmt.Errorf("resolve artefact home: %w", err)
	}
	path := filepath.Join(paths.ArtefactHome, Name, "SKILL.md")
	if err := writeAtomic(path, swornSkillContent()); err != nil {
		return "", fmt.Errorf("install sworn skill artefact at %s: %w", path, err)
	}
	return path, nil
}

// Report describes what one Install call changed.
type Report struct {
	// MigratedStubs lists legacy skill paths replaced with a bounded,
	// non-actionable migration stub, in a stable sorted order.
	MigratedStubs []string
	// InstalledPaths lists the sworn skill files written or confirmed, in a
	// stable sorted order.
	InstalledPaths []string
}

type legacyFinding struct {
	root string
	name string
	path string
}

// Install performs one idempotent, atomic install or upgrade of the sworn
// skill under homeDir. It never mutates a legacy skill copy that does not
// match a recognized generated shape; it returns a *CollisionError naming
// the exact earlier-precedence path instead.
func Install(homeDir string) (Report, error) {
	recognized, collision, err := scanLegacy(homeDir)
	if err != nil {
		return Report{}, err
	}
	if collision != nil {
		return Report{}, &CollisionError{Path: collision.path}
	}

	var report Report
	rootsToInstall := make(map[string]bool)
	for _, finding := range recognized {
		if err := writeAtomic(finding.path, migrationStub(finding.name)); err != nil {
			return Report{}, fmt.Errorf("migrate %s: %w", finding.path, err)
		}
		report.MigratedStubs = append(report.MigratedStubs, finding.path)
		rootsToInstall[finding.root] = true
	}
	if len(rootsToInstall) == 0 {
		root, ok := defaultInstallRoot(homeDir)
		if !ok {
			return Report{}, errors.New(
				"no supported skill root exists under this home; " +
					"install a supported agent tool first",
			)
		}
		rootsToInstall[root] = true
	}

	var roots []string
	for root := range rootsToInstall {
		roots = append(roots, root)
	}
	sort.Strings(roots)
	for _, root := range roots {
		path := filepath.Join(root, Name, "SKILL.md")
		if err := writeAtomic(path, swornSkillContent()); err != nil {
			return Report{}, fmt.Errorf("install sworn skill at %s: %w", path, err)
		}
		report.InstalledPaths = append(report.InstalledPaths, path)
	}
	sort.Strings(report.MigratedStubs)
	sort.Strings(report.InstalledPaths)
	return report, nil
}

// scanLegacy classifies every present legacy skill copy under homeDir. It
// returns the first modified (unrecognized) collision it finds, in root
// precedence order, without mutating anything.
func scanLegacy(homeDir string) (recognized []legacyFinding, collision *legacyFinding, err error) {
	for _, root := range SupportedRoots(homeDir) {
		for _, name := range LegacyNames {
			path := filepath.Join(root, name, "SKILL.md")
			body, readErr := os.ReadFile(path)
			if errors.Is(readErr, os.ErrNotExist) {
				continue
			}
			if readErr != nil {
				return nil, nil, fmt.Errorf("read %s: %w", path, readErr)
			}
			finding := legacyFinding{root: root, name: name, path: path}
			if !isRecognizedSkillBody(name, body) {
				return recognized, &finding, nil
			}
			recognized = append(recognized, finding)
		}
	}
	return recognized, nil, nil
}

// isRecognizedSkillBody reports whether body is either an exact generated
// Baton role skill for name (any prior generator release) or this
// installer's own migration stub for name. Both are safe to overwrite;
// anything else is a hand-modified collision.
func isRecognizedSkillBody(name string, body []byte) bool {
	frontmatterPrefix := []byte("---\nname: " + name + "\n")
	if !bytes.HasPrefix(body, frontmatterPrefix) {
		return false
	}
	if block, ok := commentBlock(body, legacyMarkerBegin); ok {
		return bytes.Contains(block, []byte(legacyGeneratorKey))
	}
	if block, ok := commentBlock(body, migratedMarkerLine); ok {
		return bytes.Contains(block, []byte("from: "+name+"\n"))
	}
	return false
}

func commentBlock(body []byte, beginToken string) ([]byte, bool) {
	begin := bytes.Index(body, []byte(beginToken))
	if begin < 0 {
		return nil, false
	}
	end := bytes.Index(body[begin:], []byte(markerEnd))
	if end < 0 {
		return nil, false
	}
	return body[begin : begin+end+len(markerEnd)], true
}

func defaultInstallRoot(homeDir string) (string, bool) {
	for _, root := range SupportedRoots(homeDir) {
		if info, err := os.Stat(filepath.Dir(root)); err == nil && info.IsDir() {
			return root, true
		}
	}
	return "", false
}

func writeAtomic(path string, body []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, ".sworn-skill-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	if _, err := temp.Write(body); err != nil {
		temp.Close()
		os.Remove(tempPath)
		return err
	}
	if err := temp.Close(); err != nil {
		os.Remove(tempPath)
		return err
	}
	if err := os.Chmod(tempPath, 0o644); err != nil {
		os.Remove(tempPath)
		return err
	}
	return os.Rename(tempPath, path)
}

func migrationStub(name string) []byte {
	return []byte(fmt.Sprintf(
		"---\nname: %s\ndescription: \"Migrated: use the sworn skill instead.\"\n---\n\n"+
			"<!-- sworn-migrated-skill\nfrom: %s\nrole-assets-version: %s\n-->\n\n"+
			"This skill has moved. It is no longer an actionable workflow and Sworn\n"+
			"never reads it. Use the `sworn` skill together with the local Sworn CLI\n"+
			"or MCP service instead.\n",
		name, name, baton.RoleAssetsVersion,
	))
}

func swornSkillContent() []byte {
	return []byte(fmt.Sprintf(
		"---\nname: sworn\ndescription: \"Recognize Sworn-governed work and route it to the local Sworn CLI or MCP service.\"\n---\n\n"+
			"<!-- sworn-skill\nversion: %s\n-->\n\n"+
			"Use this skill only to recognize Sworn-governed work and connect the caller\n"+
			"to Sworn's own binary and command service. It carries no reducer, receipt\n"+
			"writer, role instruction, verification verdict, or merge procedure of its\n"+
			"own; Sworn's CLI, TUI, and MCP tools remain the sole authority for those\n"+
			"decisions.\n\n"+
			"1. Look for a Sworn-governed repository. The unit of work is a Git\n"+
			"   worktree, recognized by `.git`. Inside one, check for these markers:\n"+
			"   `.baton/releases` control records, an existing Sworn journal file, or\n"+
			"   an initialized Sworn project directory `.sworn`. If the worktree\n"+
			"   carries none of them, Sworn is simply not initialized here yet: run\n"+
			"   `sworn init` and continue with the rest of this skill. Only outside a\n"+
			"   Git worktree does this skill not apply; say so and stop.\n"+
			"2. Prefer the local Sworn MCP service for headless operation. If it is not\n"+
			"   already reachable, start it (for example with `sworn serve`, or the\n"+
			"   project's documented equivalent) and connect to it before taking any\n"+
			"   other action.\n"+
			"3. If MCP is unavailable, use the `sworn` CLI directly: `sworn status\n"+
			"   --json` to read the current run, and `sworn board` to see what is next\n"+
			"   and whether a person is needed.\n"+
			"4. Present the operator with the exact choices Sworn's own board or MCP\n"+
			"   status reports (for example: answer a question, approve a plan, or\n"+
			"   resume a run). Do not invent a Planner, Implementer, Captain, or\n"+
			"   Verifier decision, a receipt, or a merge outcome; only Sworn's command\n"+
			"   service may produce those.\n"+
			"5. If Sworn reports that no action is currently possible (paused, blocked,\n"+
			"   or waiting on another operator), report that state plainly and stop.\n",
		baton.RoleAssetsVersion,
	))
}
