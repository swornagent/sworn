// Command batongolden verifies or explicitly reproduces the checked-in,
// development-only Baton RC2 oracle corpus. Ordinary verification never
// invokes Node, Git, the network, or a user installation.
package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/swornagent/sworn/internal/baton"
)

const (
	goldenSchema = "sworn.baton-golden-admission/v1"
	corpusSchema = "sworn.baton-golden-corpus/v1"
	generator    = "sworn.batongolden/v1"
	corpusRoot   = "testdata/corpus"
)

//go:embed testdata/corpus/*.json
var embeddedCorpus embed.FS

//go:embed oracle.mjs
var oracleScript []byte

type verification struct {
	Schema         string         `json:"schema"`
	Identity       baton.Identity `json:"identity"`
	CorpusManifest string         `json:"corpus_manifest"`
	VectorFiles    int            `json:"vector_files"`
	VectorBytes    int64          `json:"vector_bytes"`
}

type corpusManifest struct {
	Schema           string `json:"schema"`
	GeneratorVersion string `json:"generator_version"`
	OracleSHA256     string `json:"oracle_sha256"`
	Baton            struct {
		Tag           string `json:"tag"`
		Commit        string `json:"commit"`
		Tree          string `json:"tree"`
		SupportDigest string `json:"support_digest"`
	} `json:"baton"`
	ReferenceSources []manifestSource `json:"reference_sources"`
	Git              struct {
		RealPath             string `json:"real_path"`
		Version              string `json:"version"`
		SanitizedEnvironment string `json:"sanitized_environment"`
	} `json:"git"`
	ObjectFormats []manifestFormat `json:"object_formats"`
	Files         []manifestFile   `json:"files"`
}

type manifestSource struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type manifestFormat struct {
	Name         string `json:"name"`
	OIDHexLength int    `json:"oid_hex_length"`
}

type manifestFile struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && args[0] == "verify" {
		pkg, err := baton.Load()
		if err != nil {
			fmt.Fprintf(stderr, "batongolden: %v\n", err)
			return 1
		}
		identity, err := pkg.Identity()
		if err != nil {
			fmt.Fprintf(stderr, "batongolden: %v\n", err)
			return 1
		}
		manifestDigest, files, size, err := verifyCorpus(embeddedCorpus)
		if err != nil {
			fmt.Fprintf(stderr, "batongolden: %v\n", err)
			return 1
		}
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(verification{
			Schema: goldenSchema, Identity: identity, CorpusManifest: manifestDigest,
			VectorFiles: files, VectorBytes: size,
		}); err != nil {
			fmt.Fprintf(stderr, "batongolden: write verification: %v\n", err)
			return 1
		}
		return 0
	}
	if len(args) == 9 && args[0] == "generate" && args[1] == "--reference-root" &&
		args[3] == "--node" && args[5] == "--git" && args[7] == "--output" {
		if err := generateCorpus(args[2], args[4], args[6], args[8]); err != nil {
			fmt.Fprintf(stderr, "batongolden: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, "batongolden: generated pinned RC2 corpus from JavaScript oracle")
		return 0
	}
	fmt.Fprintln(stderr, "usage: batongolden verify")
	fmt.Fprintln(stderr, "       batongolden generate --reference-root ABS --node ABS --git ABS --output ABS")
	return 2
}

func verifyCorpus(source fs.FS) (string, int, int64, error) {
	manifestBytes, err := fs.ReadFile(source, corpusRoot+"/manifest.json")
	if err != nil {
		return "", 0, 0, fmt.Errorf("read corpus manifest: %w", err)
	}
	var manifest corpusManifest
	if err := decodeClosed(manifestBytes, &manifest); err != nil {
		return "", 0, 0, fmt.Errorf("decode corpus manifest: %w", err)
	}
	if err := validateManifest(manifest); err != nil {
		return "", 0, 0, err
	}
	var inventory []string
	err = fs.WalkDir(source, corpusRoot, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative := strings.TrimPrefix(name, corpusRoot+"/")
		if relative == name || filepath.ToSlash(filepath.Clean(relative)) != relative {
			return fmt.Errorf("noncanonical corpus path %q", name)
		}
		inventory = append(inventory, relative)
		return nil
	})
	if err != nil {
		return "", 0, 0, fmt.Errorf("walk corpus: %w", err)
	}
	sort.Strings(inventory)
	expected := []string{"manifest.json"}
	for _, file := range manifest.Files {
		expected = append(expected, file.Path)
	}
	sort.Strings(expected)
	if !reflect.DeepEqual(inventory, expected) {
		return "", 0, 0, fmt.Errorf("corpus inventory is %v, expected %v", inventory, expected)
	}
	var total int64
	for _, file := range manifest.Files {
		body, err := fs.ReadFile(source, corpusRoot+"/"+file.Path)
		if err != nil {
			return "", 0, 0, fmt.Errorf("read vector %s: %w", file.Path, err)
		}
		if int64(len(body)) != file.Size || baton.DigestBytes(body) != file.SHA256 {
			return "", 0, 0, fmt.Errorf("vector %s does not match its size/digest", file.Path)
		}
		var valid any
		if err := json.Unmarshal(body, &valid); err != nil {
			return "", 0, 0, fmt.Errorf("vector %s is not JSON: %w", file.Path, err)
		}
		total += int64(len(body))
	}
	return baton.DigestBytes(manifestBytes), len(manifest.Files), total, nil
}

func validateManifest(value corpusManifest) error {
	if value.Schema != corpusSchema || value.GeneratorVersion != generator ||
		value.OracleSHA256 != baton.DigestBytes(oracleScript) ||
		value.Baton.Tag != baton.TagName || value.Baton.Commit != baton.Commit ||
		value.Baton.Tree != baton.Tree || value.Baton.SupportDigest != baton.SupportPackageSHA256 {
		return errors.New("corpus manifest has a foreign Baton identity")
	}
	if !reflect.DeepEqual(value.ReferenceSources, pinnedReferenceSources()) {
		return errors.New("corpus source bindings are not exact")
	}
	if value.Git.RealPath != "/usr/bin/git" || value.Git.Version != "git version 2.43.0" ||
		value.Git.SanitizedEnvironment != "sworn.git-literal/v1" {
		return errors.New("corpus Git provenance is not exact")
	}
	if !reflect.DeepEqual(value.ObjectFormats, []manifestFormat{{"sha1", 40}, {"sha256", 64}}) {
		return errors.New("corpus object-format matrix is not exact")
	}
	if len(value.Files) != 4 {
		return fmt.Errorf("corpus has %d vector files", len(value.Files))
	}
	seen := make(map[string]bool)
	for _, file := range value.Files {
		if file.Path == "" || filepath.Base(file.Path) != file.Path || seen[file.Path] ||
			file.Size <= 0 || !strings.HasPrefix(file.SHA256, "sha256:") {
			return fmt.Errorf("invalid corpus file entry %q", file.Path)
		}
		seen[file.Path] = true
	}
	return nil
}

func pinnedReferenceSources() []manifestSource {
	return []manifestSource{
		{"actions.mjs", "sha256:dd450cbf7073dd7979c7ea74b806c5555169c0defb4dd941923933dd35dc8f78"},
		{"git.mjs", "sha256:6e41ea115f06580d1a0415e65a4ff4d5dd4568842a168fd89815c06ab76b56b2"},
		{"records.mjs", "sha256:447e0277e3f088578427cc00828c5da0f83f828238b72b639d4e9aca42772c84"},
		{"transition.mjs", "sha256:87e89368749516cfefe9b5e0735a5d29089c28ed0edc117fb340d48d51241fa3"},
	}
}

func literalExecutable(value, label string) (string, error) {
	if !filepath.IsAbs(value) {
		return "", fmt.Errorf("%s must be absolute", label)
	}
	resolved, err := filepath.EvalSymlinks(value)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", label, err)
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("%s must resolve to a regular executable", label)
	}
	return resolved, nil
}

func generateCorpus(referenceRoot, nodeExecutable, gitExecutable, output string) error {
	for label, value := range map[string]string{
		"reference root": referenceRoot, "Node executable": nodeExecutable,
		"Git executable": gitExecutable, "output": output,
	} {
		if !filepath.IsAbs(value) {
			return fmt.Errorf("%s must be absolute", label)
		}
	}
	referenceRoot, err := filepath.EvalSymlinks(referenceRoot)
	if err != nil {
		return fmt.Errorf("resolve reference root: %w", err)
	}
	info, err := os.Stat(referenceRoot)
	if err != nil || !info.IsDir() {
		return errors.New("reference root must resolve to a directory")
	}
	nodeExecutable, err = literalExecutable(nodeExecutable, "Node executable")
	if err != nil {
		return err
	}
	gitExecutable, err = literalExecutable(gitExecutable, "Git executable")
	if err != nil {
		return err
	}
	if gitExecutable != "/usr/bin/git" {
		return fmt.Errorf("Git resolves to %s, corpus pins /usr/bin/git", gitExecutable)
	}
	command := exec.Command(gitExecutable, "--version")
	command.Env = []string{"HOME=/dev/null", "LANG=C", "LC_ALL=C", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null"}
	version, err := command.Output()
	if err != nil || strings.TrimSpace(string(version)) != "git version 2.43.0" {
		return fmt.Errorf("Git version does not match corpus provenance")
	}
	for _, source := range pinnedReferenceSources() {
		body, err := os.ReadFile(filepath.Join(referenceRoot, source.Path))
		if err != nil {
			return fmt.Errorf("read reference %s: %w", source.Path, err)
		}
		if baton.DigestBytes(body) != source.SHA256 {
			return fmt.Errorf("reference %s has a foreign digest", source.Path)
		}
	}
	if entries, err := os.ReadDir(output); err == nil && len(entries) != 0 {
		return errors.New("output directory must be absent or empty")
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("inspect output: %w", err)
	}
	temporary, err := os.MkdirTemp("", "batongolden-oracle-*")
	if err != nil {
		return fmt.Errorf("create oracle workspace: %w", err)
	}
	defer os.RemoveAll(temporary)
	script := filepath.Join(temporary, "oracle.mjs")
	if err := os.WriteFile(script, oracleScript, 0o600); err != nil {
		return fmt.Errorf("write oracle script: %w", err)
	}
	vectors := filepath.Join(temporary, "vectors")
	command = exec.Command(nodeExecutable, script, referenceRoot, gitExecutable, vectors)
	command.Env = []string{"HOME=/dev/null", "LANG=C", "LC_ALL=C", "PATH=/nonexistent"}
	if outputBytes, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("run JavaScript oracle: %w: %s", err, strings.TrimSpace(string(outputBytes)))
	}
	names := []string{"actions.json", "git.json", "lifecycle.json", "records.json"}
	inventory, err := os.ReadDir(vectors)
	if err != nil {
		return fmt.Errorf("read oracle output: %w", err)
	}
	var observed []string
	for _, entry := range inventory {
		if entry.IsDir() {
			return fmt.Errorf("oracle emitted unexpected directory %s", entry.Name())
		}
		observed = append(observed, entry.Name())
	}
	sort.Strings(observed)
	if !reflect.DeepEqual(observed, names) {
		return fmt.Errorf("oracle emitted %v, expected %v", observed, names)
	}
	manifest := corpusManifest{
		Schema: corpusSchema, GeneratorVersion: generator,
		OracleSHA256:     baton.DigestBytes(oracleScript),
		ReferenceSources: pinnedReferenceSources(),
		ObjectFormats:    []manifestFormat{{"sha1", 40}, {"sha256", 64}},
	}
	manifest.Baton.Tag = baton.TagName
	manifest.Baton.Commit = baton.Commit
	manifest.Baton.Tree = baton.Tree
	manifest.Baton.SupportDigest = baton.SupportPackageSHA256
	manifest.Git.RealPath = gitExecutable
	manifest.Git.Version = strings.TrimSpace(string(version))
	manifest.Git.SanitizedEnvironment = "sworn.git-literal/v1"
	for _, name := range names {
		body, err := os.ReadFile(filepath.Join(vectors, name))
		if err != nil {
			return fmt.Errorf("read oracle vector %s: %w", name, err)
		}
		var decoded any
		if err := json.Unmarshal(body, &decoded); err != nil {
			return fmt.Errorf("oracle vector %s is not JSON: %w", name, err)
		}
		manifest.Files = append(manifest.Files, manifestFile{
			Path: name, Size: int64(len(body)), SHA256: baton.DigestBytes(body),
		})
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode corpus manifest: %w", err)
	}
	manifestBytes = append(manifestBytes, '\n')
	if err := os.MkdirAll(output, 0o755); err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	for _, name := range names {
		body, err := os.ReadFile(filepath.Join(vectors, name))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(output, name), body, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}
	if err := os.WriteFile(filepath.Join(output, "manifest.json"), manifestBytes, 0o644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	generated := os.DirFS(filepath.Dir(output))
	if _, _, _, err := verifyCorpus(prefixedFS{FS: generated, Prefix: filepath.Base(output)}); err != nil {
		return fmt.Errorf("verify generated corpus: %w", err)
	}
	return nil
}

type prefixedFS struct {
	fs.FS
	Prefix string
}

func (p prefixedFS) Open(name string) (fs.File, error) {
	if name == "." {
		return p.FS.Open(p.Prefix)
	}
	if strings.HasPrefix(name, corpusRoot) {
		name = strings.TrimPrefix(name, corpusRoot)
		name = strings.TrimPrefix(name, "/")
		return p.FS.Open(filepath.ToSlash(filepath.Join(p.Prefix, name)))
	}
	return nil, fs.ErrNotExist
}

func decodeClosed(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return fmt.Errorf("trailing JSON: %v", err)
	}
	return nil
}
