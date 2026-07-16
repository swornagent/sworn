package baton

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha1" //nolint:gosec // Git's pinned repository object format is SHA-1.
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	PinnedBatonTag                   = "v0.15.1"
	PinnedBatonCommit                = "3fb4d275ae8a151f6287e7b9279d71628b12eea0"
	PinnedBatonSourceDigest          = "sha256:8f0839ea897374eb10d6db2a789939714727739621babef1117d74cbf4488d2f"
	PinnedBatonVersionBlobOID        = "5f1dd0af59642311ee04e018a0023562d4dde008"
	PinnedInstallerArchiveSHA256     = "27d5021cb3ec258a7fd7a5feb6eed92968be0e6cb439e2951da7c6b368e0ca15"
	PinnedInstallerArchiveBlobOID    = "39ae650dfe0282b0fa8bda14e1a01e7084077702"
	installerArchivePath             = "internal/adopt/baton/installer-input-v0.15.1.tar"
	installerArchivePrefix           = "baton-v0.15.1/"
	pinnedInstallerArchiveEntryCount = 78
)

var installerArchivePaths = []string{
	"install-codex.sh",
	"install-claude.sh",
	"baton",
	"commands",
	"schemas",
}

var pinnedCommandNames = []string{
	"design-review.md",
	"implement-slice.md",
	"mark-shipped.md",
	"merge-release.md",
	"merge-track.md",
	"plan-release.md",
	"replan-release.md",
	"verify-slice.md",
}

type InstallerArchiveEntry struct {
	Path    string
	Mode    os.FileMode
	IsDir   bool
	Bytes   []byte
	BlobOID string
}

type InstallerArchive struct {
	Bytes   []byte
	Entries []InstallerArchiveEntry
	Files   map[string][]byte
}

type InstallerVendorInputs struct {
	Archive []byte
	Version UpstreamVersionCandidate
}

// ManagedTreeEntry is one canonical installer output beneath a logical home.
// Paths are slash-separated, relative, and byte-sorted in ManagedTree.
type ManagedTreeEntry struct {
	Path  string
	Mode  os.FileMode
	IsDir bool
	Bytes []byte
}

// ManagedTree is the complete Baton-owned output beneath one logical home.
type ManagedTree struct {
	Entries []ManagedTreeEntry
}

// InstallerManagedTrees contains the three independently installed roots.
type InstallerManagedTrees struct {
	AgentsHome ManagedTree
	CodexHome  ManagedTree
	ClaudeHome ManagedTree
}

// PinnedInstallerVendorInputs returns the exact archive and adopting-manifest
// candidate only when sourceDir is the clean pinned v0.15.1 Git checkout.
// Other local checkout versions retain the existing local-vendor behavior.
func PinnedInstallerVendorInputs(ctx context.Context, sourceDir string, capturedAt time.Time) (*InstallerVendorInputs, bool, error) {
	head, err := runPinnedGit(ctx, sourceDir, "rev-parse", "HEAD")
	if err != nil {
		return nil, false, nil
	}
	if strings.TrimSpace(string(head)) != PinnedBatonCommit {
		return nil, false, nil
	}
	status, err := runPinnedGit(ctx, sourceDir, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return nil, true, fmt.Errorf("inspect pinned Baton checkout: %w", err)
	}
	if len(status) != 0 {
		return nil, true, fmt.Errorf("pinned Baton checkout is dirty")
	}
	tagCommit, err := runPinnedGit(ctx, sourceDir, "rev-parse", PinnedBatonTag+"^{commit}")
	if err != nil || strings.TrimSpace(string(tagCommit)) != PinnedBatonCommit {
		return nil, true, fmt.Errorf("pinned Baton tag does not resolve to %s", PinnedBatonCommit)
	}
	versionOID, err := runPinnedGit(ctx, sourceDir, "rev-parse", PinnedBatonCommit+":VERSION")
	if err != nil || strings.TrimSpace(string(versionOID)) != PinnedBatonVersionBlobOID {
		return nil, true, fmt.Errorf("upstream VERSION blob identity differs from C-01")
	}
	versionBytes, err := runPinnedGit(ctx, sourceDir, "cat-file", "blob", PinnedBatonCommit+":VERSION")
	if err != nil || !bytes.Equal(versionBytes, []byte(PinnedBatonTag+"\n")) {
		return nil, true, fmt.Errorf("upstream VERSION bytes differ from C-01")
	}

	args := []string{"archive", "--format=tar", "--prefix=" + installerArchivePrefix, PinnedBatonCommit}
	args = append(args, installerArchivePaths...)
	archiveBytes, err := runPinnedGit(ctx, sourceDir, args...)
	if err != nil {
		return nil, true, fmt.Errorf("construct pinned installer archive: %w", err)
	}
	if _, err := ValidateInstallerArchive(archiveBytes); err != nil {
		return nil, true, err
	}
	return &InstallerVendorInputs{
		Archive: archiveBytes,
		Version: UpstreamVersionCandidate{
			Tag:        PinnedBatonTag,
			SHA:        PinnedBatonCommit,
			Digest:     PinnedBatonSourceDigest,
			CapturedAt: capturedAt,
		},
	}, true, nil
}

func runPinnedGit(ctx context.Context, sourceDir string, args ...string) ([]byte, error) {
	argv := append([]string{"-C", sourceDir}, args...)
	cmd := exec.CommandContext(ctx, "git", argv...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// ValidateInstallerArchive rejects any archive that is not byte-identical to
// C-01 or contains unsafe/non-canonical entries. The exact archive digest pins
// every path, mode, byte, and ordering decision made by Git archive.
func ValidateInstallerArchive(archiveBytes []byte) (*InstallerArchive, error) {
	digest := sha256.Sum256(archiveBytes)
	if hex.EncodeToString(digest[:]) != PinnedInstallerArchiveSHA256 {
		return nil, fmt.Errorf("installer archive SHA-256 differs from C-01")
	}
	if gitBlobOID(archiveBytes) != PinnedInstallerArchiveBlobOID {
		return nil, fmt.Errorf("installer archive Git blob differs from C-01")
	}

	result := &InstallerArchive{
		Bytes: append([]byte(nil), archiveBytes...),
		Files: make(map[string][]byte),
	}
	seen := make(map[string]struct{})
	commands := make(map[string]struct{})
	sawGlobalPAX := false
	reader := tar.NewReader(bytes.NewReader(archiveBytes))
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read installer archive: %w", err)
		}
		// Git emits exactly one global PAX commit comment before the filesystem
		// tree. It is authenticated metadata, not an installable node.
		if header.Typeflag == tar.TypeXGlobalHeader {
			// archive/tar consumes the raw PAX payload while parsing Next; the
			// whole-archive SHA/blob gates above authenticate those exact bytes.
			if sawGlobalPAX || len(result.Entries) != 0 ||
				header.Name != "pax_global_header" ||
				len(header.PAXRecords) != 1 || header.PAXRecords["comment"] != PinnedBatonCommit {
				return nil, fmt.Errorf("installer archive global metadata differs from C-01")
			}
			sawGlobalPAX = true
			continue
		}
		if !sawGlobalPAX {
			return nil, fmt.Errorf("installer archive omits leading C-01 metadata")
		}
		if !utf8.ValidString(header.Name) || !strings.HasPrefix(header.Name, installerArchivePrefix) {
			return nil, fmt.Errorf("installer archive path is invalid: %q", header.Name)
		}
		rel := strings.TrimPrefix(header.Name, installerArchivePrefix)
		if header.Typeflag == tar.TypeDir {
			rel = strings.TrimSuffix(rel, "/")
		}
		if rel != "" {
			if path.IsAbs(rel) || path.Clean(rel) != rel || rel == "." || strings.Contains(rel, "\\") {
				return nil, fmt.Errorf("installer archive path is non-canonical")
			}
			for _, segment := range strings.Split(rel, "/") {
				if segment == "" || segment == "." || segment == ".." {
					return nil, fmt.Errorf("installer archive path has forbidden segment")
				}
			}
		}
		key := header.Name
		if _, duplicate := seen[key]; duplicate {
			return nil, fmt.Errorf("installer archive contains duplicate path")
		}
		seen[key] = struct{}{}

		entry := InstallerArchiveEntry{Path: rel, Mode: os.FileMode(header.Mode).Perm()}
		switch header.Typeflag {
		case tar.TypeDir:
			if entry.Mode != 0o775 {
				return nil, fmt.Errorf("installer archive directory mode differs from Git tree")
			}
			entry.IsDir = true
		case tar.TypeReg, tar.TypeRegA:
			if entry.Mode != 0o664 && entry.Mode != 0o775 {
				return nil, fmt.Errorf("installer archive file mode differs from Git tree")
			}
			contents, err := io.ReadAll(reader)
			if err != nil {
				return nil, fmt.Errorf("read installer archive file: %w", err)
			}
			entry.Bytes = contents
			entry.BlobOID = gitBlobOID(contents)
			result.Files[rel] = append([]byte(nil), contents...)
			if strings.HasPrefix(rel, "commands/") && strings.Count(rel, "/") == 1 {
				commands[strings.TrimPrefix(rel, "commands/")] = struct{}{}
			}
		default:
			return nil, fmt.Errorf("installer archive contains unsupported node type")
		}
		result.Entries = append(result.Entries, entry)
	}
	if !sawGlobalPAX {
		return nil, fmt.Errorf("installer archive omits C-01 metadata")
	}
	if len(result.Entries) != pinnedInstallerArchiveEntryCount {
		return nil, fmt.Errorf("installer archive entry count = %d, want %d", len(result.Entries), pinnedInstallerArchiveEntryCount)
	}
	if len(commands) != len(pinnedCommandNames) {
		return nil, fmt.Errorf("installer archive command inventory is incomplete")
	}
	for _, name := range pinnedCommandNames {
		if _, ok := commands[name]; !ok {
			return nil, fmt.Errorf("installer archive command inventory omits %s", name)
		}
	}
	return result, nil
}

type managedTreeBuilder struct {
	entries map[string]ManagedTreeEntry
}

func newManagedTreeBuilder() *managedTreeBuilder {
	return &managedTreeBuilder{entries: make(map[string]ManagedTreeEntry)}
}

func (b *managedTreeBuilder) addDir(name string) error {
	if err := validateManagedRelativePath(name); err != nil {
		return err
	}
	for current := name; current != "." && current != ""; current = path.Dir(current) {
		if existing, ok := b.entries[current]; ok && !existing.IsDir {
			return fmt.Errorf("managed installer path collides with file: %s", current)
		}
		b.entries[current] = ManagedTreeEntry{Path: current, Mode: 0o755, IsDir: true}
	}
	return nil
}

func (b *managedTreeBuilder) addFile(name string, contents []byte) error {
	if err := validateManagedRelativePath(name); err != nil {
		return err
	}
	if existing, ok := b.entries[name]; ok && existing.IsDir {
		return fmt.Errorf("managed installer path collides with directory: %s", name)
	}
	if parent := path.Dir(name); parent != "." {
		if err := b.addDir(parent); err != nil {
			return err
		}
	}
	b.entries[name] = ManagedTreeEntry{
		Path:  name,
		Mode:  0o644,
		Bytes: append([]byte(nil), contents...),
	}
	return nil
}

func (b *managedTreeBuilder) tree() ManagedTree {
	entries := make([]ManagedTreeEntry, 0, len(b.entries))
	for _, entry := range b.entries {
		entry.Bytes = append([]byte(nil), entry.Bytes...)
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return ManagedTree{Entries: entries}
}

func validateManagedRelativePath(name string) error {
	if name == "" || !utf8.ValidString(name) || path.IsAbs(name) || path.Clean(name) != name || strings.Contains(name, "\\") {
		return fmt.Errorf("managed installer path is non-canonical: %q", name)
	}
	for _, segment := range strings.Split(name, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return fmt.Errorf("managed installer path has forbidden segment: %q", name)
		}
	}
	return nil
}

// GenerateInstallerManagedTrees reproduces the exact v0.15.1 installer copy,
// rewrite, and Codex wrapper rules using stdlib only. The exact scripts remain
// independent test oracles; production never invokes them.
func GenerateInstallerManagedTrees(archiveBytes []byte) (InstallerManagedTrees, error) {
	archive, err := ValidateInstallerArchive(archiveBytes)
	if err != nil {
		return InstallerManagedTrees{}, err
	}

	agents := newManagedTreeBuilder()
	codex := newManagedTreeBuilder()
	claude := newManagedTreeBuilder()
	if err := agents.addDir("skills"); err != nil {
		return InstallerManagedTrees{}, err
	}
	if err := codex.addDir("baton"); err != nil {
		return InstallerManagedTrees{}, err
	}
	if err := claude.addDir("commands"); err != nil {
		return InstallerManagedTrees{}, err
	}
	if err := claude.addDir("baton"); err != nil {
		return InstallerManagedTrees{}, err
	}

	for _, entry := range archive.Entries {
		if entry.Path == "baton" || !strings.HasPrefix(entry.Path, "baton/") {
			continue
		}
		rel := strings.TrimPrefix(entry.Path, "baton/")
		dest := path.Join("baton", rel)
		if entry.IsDir {
			if err := codex.addDir(dest); err != nil {
				return InstallerManagedTrees{}, err
			}
			if err := claude.addDir(dest); err != nil {
				return InstallerManagedTrees{}, err
			}
			continue
		}
		codexBytes := entry.Bytes
		if strings.HasSuffix(rel, ".md") || strings.HasSuffix(rel, ".json") {
			codexBytes = rewriteClaudeBatonPaths(codexBytes)
		}
		if err := codex.addFile(dest, codexBytes); err != nil {
			return InstallerManagedTrees{}, err
		}
		if err := claude.addFile(dest, entry.Bytes); err != nil {
			return InstallerManagedTrees{}, err
		}
	}

	for _, commandFile := range pinnedCommandNames {
		source, ok := archive.Files[path.Join("commands", commandFile)]
		if !ok {
			return InstallerManagedTrees{}, fmt.Errorf("installer archive omits command %s", commandFile)
		}
		if err := claude.addFile(path.Join("commands", commandFile), source); err != nil {
			return InstallerManagedTrees{}, err
		}
		commandName := strings.TrimSuffix(commandFile, ".md")
		skillName := "baton-" + commandName
		wrapped, err := codexSkillBytes(skillName, commandName, source)
		if err != nil {
			return InstallerManagedTrees{}, err
		}
		if err := agents.addFile(path.Join("skills", skillName, "SKILL.md"), wrapped); err != nil {
			return InstallerManagedTrees{}, err
		}
	}

	schemaNames := make([]string, 0)
	for name := range archive.Files {
		if strings.HasPrefix(name, "schemas/") && strings.Count(name, "/") == 1 && strings.HasSuffix(name, ".json") {
			schemaNames = append(schemaNames, name)
		}
	}
	sort.Strings(schemaNames)
	if len(schemaNames) == 0 {
		return InstallerManagedTrees{}, fmt.Errorf("installer archive contains no record schemas")
	}
	for _, sourcePath := range schemaNames {
		dest := path.Join("baton", "schemas", path.Base(sourcePath))
		if err := codex.addFile(dest, archive.Files[sourcePath]); err != nil {
			return InstallerManagedTrees{}, err
		}
		if err := claude.addFile(dest, archive.Files[sourcePath]); err != nil {
			return InstallerManagedTrees{}, err
		}
	}

	return InstallerManagedTrees{
		AgentsHome: agents.tree(),
		CodexHome:  codex.tree(),
		ClaudeHome: claude.tree(),
	}, nil
}

func rewriteClaudeBatonPaths(contents []byte) []byte {
	rewritten := bytes.ReplaceAll(contents, []byte("$HOME/.claude/baton/"), []byte("$HOME/.codex/baton/"))
	return bytes.ReplaceAll(rewritten, []byte("~/.claude/baton/"), []byte("~/.codex/baton/"))
}

func codexSkillBytes(skillName, commandName string, source []byte) ([]byte, error) {
	text := string(source)
	description := ""
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "description:") {
			description = strings.TrimLeft(strings.TrimPrefix(line, "description:"), " ")
			break
		}
	}
	if description == "" {
		description = "baton " + commandName + " command"
	}

	body := text
	lines := strings.Split(text, "\n")
	if len(lines) > 0 && lines[0] == "---" {
		closing := -1
		for i := 1; i < len(lines); i++ {
			if lines[i] == "---" {
				closing = i
				break
			}
		}
		if closing < 0 {
			return nil, fmt.Errorf("command %s has unterminated frontmatter", commandName)
		}
		body = strings.Join(lines[closing+1:], "\n")
	}
	body = strings.TrimRight(body, "\n")
	body = string(rewriteClaudeBatonPaths([]byte(body)))

	prelude := fmt.Sprintf("> **Codex argument resolution.** This skill was generated by baton's install-codex.sh from the Claude Code slash-command body, which uses positional substitution (`$1`, `$2`). Codex skills receive arguments as free-form prompt text instead, so before reading the body below, **resolve `$1` and `$2` yourself** from the user's invocation message — they are the first and second whitespace-separated tokens after `$%s`. By shape: a token matching `^S[0-9]+-` is a slice-id; a token matching `^[0-9]{4}-[0-9]{2}-[0-9]{2}-` is a release-name. If the tokens are swapped, trust the shape and reassign. Wherever the body below shows `$1` / `$2`, substitute your resolved values.", skillName)

	return []byte(fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n\n%s\n\n%s\n", skillName, description, prelude, body)), nil
}

// WriteManagedTree materialises one native tree with explicit fixed modes,
// independent of the process umask. It rejects symlinks and special nodes at
// every existing managed path.
func WriteManagedTree(root string, tree ManagedTree) error {
	if info, err := os.Lstat(root); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("managed root is not a real directory: %s", root)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect managed root %s: %w", root, err)
	} else if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("create managed root %s: %w", root, err)
	}
	if err := os.Chmod(root, 0o755); err != nil {
		return fmt.Errorf("set managed root mode %s: %w", root, err)
	}

	for _, entry := range tree.Entries {
		if !entry.IsDir {
			continue
		}
		dest := filepath.Join(root, filepath.FromSlash(entry.Path))
		if info, err := os.Lstat(dest); err == nil {
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("managed directory path is unsafe: %s", entry.Path)
			}
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect managed directory %s: %w", entry.Path, err)
		} else if err := os.MkdirAll(dest, 0o755); err != nil {
			return fmt.Errorf("create managed directory %s: %w", entry.Path, err)
		}
		if err := os.Chmod(dest, 0o755); err != nil {
			return fmt.Errorf("set managed directory mode %s: %w", entry.Path, err)
		}
	}
	for _, entry := range tree.Entries {
		if entry.IsDir {
			continue
		}
		dest := filepath.Join(root, filepath.FromSlash(entry.Path))
		if info, err := os.Lstat(dest); err == nil && (info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular()) {
			return fmt.Errorf("managed file path is unsafe: %s", entry.Path)
		} else if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("inspect managed file %s: %w", entry.Path, err)
		}
		if err := os.WriteFile(dest, entry.Bytes, 0o644); err != nil {
			return fmt.Errorf("write managed file %s: %w", entry.Path, err)
		}
		if err := os.Chmod(dest, 0o644); err != nil {
			return fmt.Errorf("set managed file mode %s: %w", entry.Path, err)
		}
	}
	return nil
}

func gitBlobOID(data []byte) string {
	h := sha1.New() //nolint:gosec // Required for byte-exact Git SHA-1 object identity.
	fmt.Fprintf(h, "blob %d\x00", len(data))
	_, _ = h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}
