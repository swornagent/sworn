package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	manifestSchema = "sworn.baton-assets/v1"
	maxAssetBytes  = int64(8 << 20)
	maxTotalBytes  = int64(32 << 20)
)

type limits struct{ asset, total int64 }

type options struct {
	repo, commit, out string
	paths             []string
}

type asset struct {
	path string
	data []byte
	mode os.FileMode
}

type manifest struct {
	Schema string          `json:"schema"`
	Commit string          `json:"commit"`
	Assets []manifestEntry `json:"assets"`
}

type manifestEntry struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "batonassets:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	opts, err := parseArgs(args)
	if err != nil {
		return err
	}
	return snapshot(opts, limits{asset: maxAssetBytes, total: maxTotalBytes})
}

func parseArgs(args []string) (options, error) {
	if len(args) == 0 || args[0] != "snapshot" {
		return options{}, errors.New("expected snapshot subcommand")
	}
	values := make(map[string]string, 4)
	for i := 1; i < len(args); i += 2 {
		if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
			return options{}, fmt.Errorf("missing value for %q", args[i])
		}
		name := args[i]
		switch name {
		case "--repo", "--commit", "--paths", "--out":
		default:
			return options{}, fmt.Errorf("unknown flag %q", name)
		}
		if _, exists := values[name]; exists {
			return options{}, fmt.Errorf("duplicate flag %q", name)
		}
		values[name] = args[i+1]
	}
	for _, name := range []string{"--repo", "--commit", "--paths", "--out"} {
		if _, exists := values[name]; !exists {
			return options{}, fmt.Errorf("missing required flag %q", name)
		}
	}
	paths, err := parsePaths(values["--paths"])
	if err != nil {
		return options{}, err
	}
	return options{
		repo:   values["--repo"],
		commit: values["--commit"],
		paths:  paths,
		out:    values["--out"],
	}, nil
}

func parsePaths(raw string) ([]string, error) {
	if !utf8.ValidString(raw) {
		return nil, errors.New("paths JSON is not valid UTF-8")
	}
	var paths []string
	if err := json.Unmarshal([]byte(raw), &paths); err != nil || len(paths) == 0 {
		return nil, errors.New("paths must be one non-empty JSON array of strings")
	}
	for i, value := range paths {
		if err := validatePath(value); err != nil {
			return nil, fmt.Errorf("invalid path at index %d: %w", i, err)
		}
		if i > 0 && paths[i-1] >= value {
			return nil, errors.New("paths must be strictly sorted and unique")
		}
	}
	return paths, nil
}

func validatePath(value string) error {
	if value == "" || !utf8.ValidString(value) {
		return errors.New("path must be non-empty valid UTF-8")
	}
	if path.IsAbs(value) || path.Clean(value) != value || value == "." {
		return errors.New("path must be canonical and relative")
	}
	if strings.Contains(value, `\`) {
		return errors.New("path must use POSIX separators")
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." {
			return errors.New("path contains an invalid component")
		}
		for _, char := range part {
			if char == 0 || char < 0x20 || char == 0x7f {
				return errors.New("path contains a control character")
			}
		}
	}
	return nil
}

func snapshot(opts options, bounds limits) error {
	if bounds.asset <= 0 || bounds.total <= 0 || bounds.asset > bounds.total {
		return errors.New("invalid byte limits")
	}
	if err := validateAbsoluteDirectory(opts.repo, "repo"); err != nil {
		return err
	}
	info, err := os.Stat(opts.repo)
	if err != nil || !info.IsDir() {
		return errors.New("repo must be an existing directory")
	}
	if err := validateCommit(opts.commit); err != nil {
		return err
	}
	if err := validateOutput(opts.out); err != nil {
		return err
	}
	if err := verifyCommit(opts.repo, opts.commit); err != nil {
		return err
	}

	assets := make([]asset, 0, len(opts.paths))
	var total int64
	for _, name := range opts.paths {
		item, size, err := readAsset(opts.repo, opts.commit, name, bounds.asset)
		if err != nil {
			return err
		}
		if size > bounds.total-total {
			return fmt.Errorf("assets exceed total byte limit %d", bounds.total)
		}
		total += size
		assets = append(assets, item)
	}
	return publish(opts.out, opts.commit, assets, os.Rename)
}

func validateAbsoluteDirectory(value, name string) error {
	if value == "" || !filepath.IsAbs(value) || filepath.Clean(value) != value {
		return fmt.Errorf("%s must be a canonical absolute path", name)
	}
	return nil
}

func validateCommit(commit string) error {
	if len(commit) != 40 || strings.Trim(commit, "0123456789abcdef") != "" {
		return errors.New("commit must be exactly 40 lowercase hexadecimal characters")
	}
	return nil
}

func validateOutput(out string) error {
	if err := validateAbsoluteDirectory(out, "out"); err != nil {
		return err
	}
	if out == string(filepath.Separator) {
		return errors.New("out cannot be the filesystem root")
	}
	parent, err := os.Stat(filepath.Dir(out))
	if err != nil || !parent.IsDir() {
		return errors.New("out parent must be an existing directory")
	}
	if _, err := os.Lstat(out); err == nil {
		return errors.New("out already exists")
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect out: %w", err)
	}
	return nil
}

func verifyCommit(repo, commit string) error {
	resolved, err := gitOutput(repo, "rev-parse", "--verify", commit+"^{commit}")
	if err != nil || string(resolved) != commit+"\n" {
		return errors.New("commit does not resolve identically to a commit")
	}
	return nil
}

func readAsset(repo, commit, name string, maximum int64) (asset, int64, error) {
	listing, err := gitOutput(repo, "ls-tree", "-z", "--full-tree", commit, "--", ":(literal)"+name)
	if err != nil {
		return asset{}, 0, fmt.Errorf("inspect %q: %w", name, err)
	}
	parts := bytes.Split(listing, []byte{0})
	if len(parts) != 2 || len(parts[0]) == 0 || len(parts[1]) != 0 {
		return asset{}, 0, fmt.Errorf("path %q is missing or ambiguous", name)
	}
	header, gotName, found := bytes.Cut(parts[0], []byte{'\t'})
	fields := bytes.Fields(header)
	if !found || string(gotName) != name || len(fields) != 3 {
		return asset{}, 0, fmt.Errorf("path %q has malformed Git metadata", name)
	}
	mode := string(fields[0])
	if (mode != "100644" && mode != "100755") || string(fields[1]) != "blob" {
		return asset{}, 0, fmt.Errorf("path %q is not an ordinary blob", name)
	}
	oid := string(fields[2])
	if err := validateCommit(oid); err != nil {
		return asset{}, 0, fmt.Errorf("path %q has an invalid blob object", name)
	}
	rawSize, err := gitOutput(repo, "cat-file", "-s", oid)
	if err != nil {
		return asset{}, 0, fmt.Errorf("size %q: %w", name, err)
	}
	size, err := strconv.ParseInt(strings.TrimSuffix(string(rawSize), "\n"), 10, 64)
	if err != nil || size < 0 || size > maximum {
		return asset{}, 0, fmt.Errorf("path %q exceeds individual byte limit %d", name, maximum)
	}
	data, err := readBlob(repo, oid, size)
	if err != nil {
		return asset{}, 0, fmt.Errorf("read %q: %w", name, err)
	}
	permissions := os.FileMode(0o644)
	if mode == "100755" {
		permissions = 0o755
	}
	return asset{path: name, data: data, mode: permissions}, size, nil
}

func readBlob(repo, oid string, size int64) ([]byte, error) {
	command := gitCommand(repo, "cat-file", "blob", oid)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(stdout, size+1))
	if readErr != nil || int64(len(data)) > size {
		_ = command.Process.Kill()
		_ = command.Wait()
		return nil, errors.New("blob exceeded advertised size")
	}
	if err := command.Wait(); err != nil {
		return nil, fmt.Errorf("git cat-file failed: %s", strings.TrimSpace(stderr.String()))
	}
	if int64(len(data)) != size {
		return nil, errors.New("blob did not match advertised size")
	}
	return data, nil
}

func gitOutput(repo string, args ...string) ([]byte, error) {
	output, err := gitCommand(repo, args...).Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return nil, fmt.Errorf("git failed: %s", strings.TrimSpace(string(exit.Stderr)))
		}
		return nil, err
	}
	return output, nil
}

func gitCommand(repo string, args ...string) *exec.Cmd {
	base := []string{"--no-replace-objects", "-c", "core.hooksPath=/dev/null", "-c", "core.fsmonitor=false", "-C", repo}
	command := exec.Command("git", append(base, args...)...)
	command.Env = cleanGitEnvironment()
	return command
}

func cleanGitEnvironment() []string {
	environment := make([]string, 0, len(os.Environ())+4)
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if !strings.HasPrefix(key, "GIT_") && key != "LC_ALL" {
			environment = append(environment, entry)
		}
	}
	return append(environment,
		"GIT_NO_LAZY_FETCH=1",
		"GIT_NO_REPLACE_OBJECTS=1",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_TERMINAL_PROMPT=0",
		"LC_ALL=C",
	)
}

func publish(out, commit string, assets []asset, rename func(string, string) error) (err error) {
	stage, err := os.MkdirTemp(filepath.Dir(out), "."+filepath.Base(out)+".tmp-")
	if err != nil {
		return fmt.Errorf("create private staging directory: %w", err)
	}
	defer func() {
		if stage != "" {
			_ = os.RemoveAll(stage)
		}
	}()

	entries := make([]manifestEntry, 0, len(assets))
	for _, item := range assets {
		target := filepath.Join(stage, "assets", filepath.FromSlash(item.path))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return fmt.Errorf("create asset directory: %w", err)
		}
		if err := os.WriteFile(target, item.data, 0o600); err != nil {
			return fmt.Errorf("write asset %q: %w", item.path, err)
		}
		if err := os.Chmod(target, item.mode); err != nil {
			return fmt.Errorf("set asset mode %q: %w", item.path, err)
		}
		digest := sha256.Sum256(item.data)
		entries = append(entries, manifestEntry{
			Path: item.path, Size: int64(len(item.data)), SHA256: "sha256:" + fmt.Sprintf("%x", digest),
		})
	}
	body, err := json.Marshal(manifest{Schema: manifestSchema, Commit: commit, Assets: entries})
	if err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}
	body = append(body, '\n')
	if err := os.WriteFile(filepath.Join(stage, "manifest.json"), body, 0o600); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	if err := os.Chmod(filepath.Join(stage, "manifest.json"), 0o644); err != nil {
		return fmt.Errorf("set manifest mode: %w", err)
	}
	if _, err := os.Lstat(out); err == nil {
		return errors.New("out already exists")
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect out before publish: %w", err)
	}
	if err := rename(stage, out); err != nil {
		return fmt.Errorf("publish snapshot: %w", err)
	}
	stage = ""
	return nil
}
