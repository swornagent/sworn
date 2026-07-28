// Command batongolden verifies or explicitly regenerates the checked-in RC8
// reference corpus. Ordinary verification executes no subprocess.
package main

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/swornagent/sworn/internal/baton"
)

const (
	verificationSchema = "sworn.batongolden-verification/v2"
	corpusSchema       = "sworn.batongolden/v2"
	corpusRoot         = "testdata/corpus"
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

type manifestEntry struct {
	File   string `json:"file"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

type corpusManifest struct {
	Schema       string          `json:"schema"`
	Baton        string          `json:"baton"`
	Generator    string          `json:"generator"`
	OracleSHA256 string          `json:"oracle_sha256"`
	References   []manifestEntry `json:"references"`
	Files        []manifestEntry `json:"files"`
}

var pinnedReferences = []manifestEntry{
	{File: "actions.mjs", SHA256: "25c40728241580b3300ffac8d341541cc05c58f11e1d173d9e84e03b9e773ce8", Bytes: 41798},
	{File: "git.mjs", SHA256: "441bb55f91b521f68edfb40f711d2ed115421f32bd9b048c069df45c55304908", Bytes: 86433},
	{File: "receipts.mjs", SHA256: "205489602318f100ddd8bb536c8d25640bdff1df81c2e4c54d31ca1ca0bc2943", Bytes: 30352},
	{File: "state.mjs", SHA256: "7493e146a090b800fb686a4496843a5aa2e2dcb38c0f9177bf82d6035e7e89bf", Bytes: 70666},
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	switch {
	case len(args) == 1 && args[0] == "verify":
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
		manifestDigest, files, size, err := verifyCorpus(embeddedCorpus, corpusRoot, pkg)
		if err != nil {
			fmt.Fprintf(stderr, "batongolden: %v\n", err)
			return 1
		}
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(verification{
			Schema: verificationSchema, Identity: identity,
			CorpusManifest: manifestDigest, VectorFiles: files, VectorBytes: size,
		}); err != nil {
			fmt.Fprintf(stderr, "batongolden: encode verification: %v\n", err)
			return 1
		}
		return 0
	case len(args) == 7 && args[0] == "generate" && args[1] == "--root" &&
		args[3] == "--node" && args[5] == "--output":
		if err := generateCorpus(args[2], args[4], args[6]); err != nil {
			fmt.Fprintf(stderr, "batongolden: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, "batongolden: generated exact RC8 reference corpus")
		return 0
	default:
		fmt.Fprintln(stderr, "usage: batongolden verify")
		fmt.Fprintln(stderr, "       batongolden generate --root ABS --node ABS --output ABS")
		return 2
	}
}

func verifyCorpus(source fs.FS, root string, pkg baton.Package) (string, int, int64, error) {
	manifestBytes, err := fs.ReadFile(source, path.Join(root, "manifest.json"))
	if err != nil {
		return "", 0, 0, fmt.Errorf("read corpus manifest: %w", err)
	}
	var manifest corpusManifest
	if err := decodeClosed(manifestBytes, &manifest); err != nil {
		return "", 0, 0, fmt.Errorf("decode corpus manifest: %w", err)
	}
	if manifest.Schema != corpusSchema || manifest.Baton != baton.PackageVersion ||
		manifest.Generator != "exact embedded RC8 JavaScript reference" ||
		manifest.OracleSHA256 != rawSHA256(oracleScript) {
		return "", 0, 0, errors.New("corpus manifest has a foreign generator or Baton identity")
	}
	if !reflect.DeepEqual(manifest.References, pinnedReferences) {
		return "", 0, 0, errors.New("corpus reference bindings are not exact")
	}
	for _, expected := range pinnedReferences {
		body, err := pkg.ReadAsset("reference/records/" + expected.File)
		if err != nil {
			return "", 0, 0, err
		}
		if int64(len(body)) != expected.Bytes || rawSHA256(body) != expected.SHA256 {
			return "", 0, 0, fmt.Errorf("embedded reference %s drifted", expected.File)
		}
	}
	expectedNames := []string{
		"actions.json", "git.json", "manifest.json", "receipts.json", "state.json",
	}
	var inventory []string
	if err := fs.WalkDir(source, root, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			inventory = append(inventory, strings.TrimPrefix(name, strings.TrimSuffix(root, "/")+"/"))
		}
		return nil
	}); err != nil {
		return "", 0, 0, err
	}
	sort.Strings(inventory)
	if !reflect.DeepEqual(inventory, expectedNames) {
		return "", 0, 0, fmt.Errorf("corpus inventory is %v, expected %v", inventory, expectedNames)
	}
	if len(manifest.Files) != 4 {
		return "", 0, 0, fmt.Errorf("corpus has %d vector files, want 4", len(manifest.Files))
	}
	seen := make(map[string]bool)
	var total int64
	for _, file := range manifest.Files {
		if file.File == "" || filepath.Base(file.File) != file.File ||
			file.File == "manifest.json" || seen[file.File] || file.Bytes <= 0 ||
			len(file.SHA256) != 64 {
			return "", 0, 0, fmt.Errorf("invalid corpus file entry %q", file.File)
		}
		seen[file.File] = true
		body, err := fs.ReadFile(source, path.Join(root, file.File))
		if err != nil {
			return "", 0, 0, err
		}
		if int64(len(body)) != file.Bytes || rawSHA256(body) != file.SHA256 {
			return "", 0, 0, fmt.Errorf("vector %s does not match its size/digest", file.File)
		}
		var decoded any
		if err := json.Unmarshal(body, &decoded); err != nil {
			return "", 0, 0, fmt.Errorf("vector %s is not JSON: %w", file.File, err)
		}
		total += int64(len(body))
	}
	return baton.DigestBytes(manifestBytes), len(manifest.Files), total, nil
}

func generateCorpus(root, node, output string) error {
	for label, value := range map[string]string{"root": root, "node": node, "output": output} {
		if !filepath.IsAbs(value) {
			return fmt.Errorf("%s must be absolute", label)
		}
	}
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve root: %w", err)
	}
	node, err = literalExecutable(node)
	if err != nil {
		return fmt.Errorf("admit Node: %w", err)
	}
	oracle := filepath.Join(root, "tools", "batongolden", "oracle.mjs")
	if body, err := os.ReadFile(oracle); err != nil || !bytes.Equal(body, oracleScript) {
		return errors.New("root does not contain this exact oracle")
	}
	for _, expected := range pinnedReferences {
		body, err := os.ReadFile(filepath.Join(
			root, "internal", "baton", "snapshot", "assets",
			"reference", "records", expected.File,
		))
		if err != nil || rawSHA256(body) != expected.SHA256 || int64(len(body)) != expected.Bytes {
			return fmt.Errorf("root has a foreign %s reference", expected.File)
		}
	}
	if entries, err := os.ReadDir(output); err == nil && len(entries) != 0 {
		return errors.New("output must be absent or empty")
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	command := exec.Command(node, oracle, output)
	command.Dir = root
	command.Env = []string{
		"HOME=/dev/null", "XDG_CONFIG_HOME=/dev/null", "LANG=C", "LC_ALL=C",
		"PATH=/usr/bin", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null",
	}
	if body, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("run exact oracle: %w: %s", err, strings.TrimSpace(string(body)))
	}
	pkg, err := baton.Load()
	if err != nil {
		return err
	}
	if _, _, _, err := verifyCorpus(os.DirFS(output), ".", pkg); err != nil {
		return fmt.Errorf("verify generated corpus: %w", err)
	}
	for _, name := range []string{"actions.json", "git.json", "manifest.json", "receipts.json", "state.json"} {
		info, err := os.Stat(filepath.Join(output, name))
		if err != nil || info.Mode().Perm() != 0o644 {
			return fmt.Errorf("generated %s does not have mode 0644", name)
		}
	}
	return nil
}

func literalExecutable(value string) (string, error) {
	resolved, err := filepath.EvalSymlinks(value)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return "", errors.New("path must resolve to an executable regular file")
	}
	return resolved, nil
}

func rawSHA256(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
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
