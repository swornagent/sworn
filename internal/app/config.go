// Package app is Sworn's production composition root. It intentionally owns
// only one bounded delivery invocation; durable lifecycle remains in Store.
package app

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	configservice "github.com/swornagent/sworn/internal/config"
	"github.com/swornagent/sworn/internal/engine"
	"github.com/swornagent/sworn/internal/executor"
	"github.com/swornagent/sworn/internal/policy"
	"github.com/swornagent/sworn/internal/protocol"
	"github.com/swornagent/sworn/internal/repo"
)

const (
	RunConfigSchemaVersion = "sworn-run-config-v1"
	maximumRunConfigBytes  = 256 << 10
)

// Config is the complete non-secret deployment input for one bounded run. The
// repository binding is persisted configuration, not live discovery: repo.Open
// must prove that the configured root still maps to these exact Git identities.
type Config struct {
	SchemaVersion   string           `json:"schema_version"`
	ControlDatabase string           `json:"control_database"`
	Repository      RepositoryConfig `json:"repository"`
	Authority       AuthorityConfig  `json:"authority"`
	Executor        ExecutorConfig   `json:"executor"`
	ContentRuntime  ContentRuntime   `json:"content_runtime"`
	Workspaces      WorkspaceConfig  `json:"workspaces"`
	Codex           CodexConfig      `json:"codex"`
	OwnerID         string           `json:"owner_id,omitempty"`
}

type RepositoryConfig struct {
	Root    string       `json:"root"`
	Binding repo.Binding `json:"binding"`
}

type AuthorityConfig struct {
	Sources []AuthoritySource `json:"sources"`
}

// AuthoritySource contains verification material only. PublicKey is canonical
// standard-base64 Ed25519 public key bytes; private keys are never accepted.
type AuthoritySource struct {
	SourceRef       string `json:"source_ref"`
	AuthorizerRef   string `json:"authorizer_ref"`
	PublicKey       string `json:"public_key"`
	BundleDirectory string `json:"bundle_directory"`
}

type ExecutorConfig struct {
	RuntimeRoot  string          `json:"runtime_root"`
	WritableRoot string          `json:"writable_root"`
	Bubblewrap   string          `json:"bubblewrap_path"`
	SystemdRun   string          `json:"systemd_run_path"`
	Systemctl    string          `json:"systemctl_path"`
	Limits       *ExecutorLimits `json:"limits,omitempty"`
}

// ExecutorLimits is all-or-nothing. Omitting limits selects executor's fixed,
// versioned safe defaults; specifying it requires every field, including an
// explicit zero swap ceiling.
type ExecutorLimits struct {
	RuntimeSeconds uint64 `json:"runtime_seconds"`
	MemoryBytes    uint64 `json:"memory_bytes"`
	SwapBytes      uint64 `json:"swap_bytes"`
	Tasks          uint64 `json:"tasks"`
	CPUPercent     uint64 `json:"cpu_percent"`
	FileBytes      uint64 `json:"file_bytes"`
	TempBytes      uint64 `json:"temp_bytes"`
	HomeBytes      uint64 `json:"home_bytes"`
	InputBytes     uint64 `json:"input_bytes"`
	WorkspaceBytes uint64 `json:"workspace_bytes"`
	StdoutBytes    uint64 `json:"stdout_bytes"`
	StderrBytes    uint64 `json:"stderr_bytes"`
}

type ContentRuntime struct {
	Source       string `json:"source"`
	Digest       string `json:"digest"`
	MaximumBytes uint64 `json:"maximum_bytes"`
}

type WorkspaceConfig struct {
	BuilderRoot string `json:"builder_root"`
	CheckRoot   string `json:"check_root"`
}

// CodexConfig selects the auth.json from one dedicated Codex CLI home. Sworn
// never accepts credential bytes, chooses a provider, or weakens the
// adapter-owned pinned tuple.
type CodexConfig struct {
	Binary          string `json:"binary"`
	ChatGPTAuthFile string `json:"chatgpt_auth_file"`
	Model           string `json:"model"`
	TimeoutSeconds  uint64 `json:"timeout_seconds"`
}

// LoadConfig opens one exact private regular file and rejects aliases, size
// drift, trailing values, duplicate object names, and unknown schema members.
func LoadConfig(path string) (Config, error) {
	if err := validateCleanAbsolutePath(path, "run config"); err != nil {
		return Config{}, err
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return Config{}, fmt.Errorf("resolve run config: %w", err)
	}
	if resolved != path {
		return Config{}, errors.New("run config path contains a symbolic-link remap")
	}
	before, err := os.Lstat(path)
	if err != nil {
		return Config{}, fmt.Errorf("inspect run config: %w", err)
	}
	if err := validateConfigFile(before); err != nil {
		return Config{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("open run config: %w", err)
	}
	defer file.Close() //nolint:errcheck
	opened, err := file.Stat()
	if err != nil {
		return Config{}, fmt.Errorf("inspect opened run config: %w", err)
	}
	if err := validateConfigFile(opened); err != nil {
		return Config{}, err
	}
	if !os.SameFile(before, opened) || before.Size() != opened.Size() {
		return Config{}, errors.New("run config changed while being opened")
	}
	contents, err := io.ReadAll(io.LimitReader(file, maximumRunConfigBytes+1))
	if err != nil {
		return Config{}, fmt.Errorf("read run config: %w", err)
	}
	if int64(len(contents)) != opened.Size() {
		return Config{}, errors.New("run config changed while being read")
	}
	return ParseConfig(contents)
}

// ParseConfig applies the same strict JSON and semantic schema checks without
// opening deployment paths. It is exposed inside internal/app for tooling and
// tests; production callers should use LoadConfig.
func ParseConfig(contents []byte) (Config, error) {
	if len(contents) == 0 || len(contents) > maximumRunConfigBytes {
		return Config{}, errors.New("run config is empty or exceeds its byte ceiling")
	}
	if _, err := protocol.CanonicalizeJSON(contents); err != nil {
		return Config{}, fmt.Errorf("validate strict run config JSON: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	var configuration Config
	if err := decoder.Decode(&configuration); err != nil {
		return Config{}, fmt.Errorf("decode run config: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return Config{}, errors.New("run config contains a trailing JSON value")
		}
		return Config{}, fmt.Errorf("decode trailing run config: %w", err)
	}
	if err := requireCompleteLimitsObject(contents, configuration.Executor.Limits != nil); err != nil {
		return Config{}, err
	}
	if err := configuration.validate(); err != nil {
		return Config{}, err
	}
	return configuration, nil
}

func (configuration Config) validate() error {
	if configuration.SchemaVersion != RunConfigSchemaVersion {
		return fmt.Errorf("unknown run config schema %q", configuration.SchemaVersion)
	}
	paths := []struct{ label, path string }{
		{"control database", configuration.ControlDatabase},
		{"repository root", configuration.Repository.Root},
		{"executor runtime root", configuration.Executor.RuntimeRoot},
		{"executor writable root", configuration.Executor.WritableRoot},
		{"Bubblewrap executable", configuration.Executor.Bubblewrap},
		{"systemd-run executable", configuration.Executor.SystemdRun},
		{"systemctl executable", configuration.Executor.Systemctl},
		{"content runtime source", configuration.ContentRuntime.Source},
		{"builder workspace root", configuration.Workspaces.BuilderRoot},
		{"check workspace root", configuration.Workspaces.CheckRoot},
		{"Codex binary", configuration.Codex.Binary},
		{"Codex ChatGPT auth file", configuration.Codex.ChatGPTAuthFile},
	}
	for _, selected := range paths {
		if err := validateCleanAbsolutePath(selected.path, selected.label); err != nil {
			return err
		}
	}
	if err := configuration.Repository.Binding.Validate(); err != nil {
		return fmt.Errorf("validate repository binding: %w", err)
	}
	if len(configuration.Authority.Sources) == 0 {
		return errors.New("run config requires at least one authority source")
	}
	if _, err := configuration.authoritySources(); err != nil {
		return err
	}
	limits, err := configuration.executorLimits()
	if err != nil {
		return err
	}
	if !engine.ValidDigest(configuration.ContentRuntime.Digest) ||
		configuration.ContentRuntime.MaximumBytes == 0 {
		return errors.New("content runtime requires an exact digest and byte ceiling")
	}
	if configuration.Workspaces.BuilderRoot == configuration.Workspaces.CheckRoot {
		return errors.New("builder and check workspace roots must be distinct")
	}
	if strings.TrimSpace(configuration.Codex.Model) != configuration.Codex.Model ||
		configuration.Codex.Model == "" || len(configuration.Codex.Model) > 256 ||
		strings.ContainsRune(configuration.Codex.Model, '\x00') {
		return errors.New("Codex model must be explicit and bounded")
	}
	authFile := configuration.Codex.ChatGPTAuthFile
	for _, selected := range []struct{ label, path string }{
		{"control database", configuration.ControlDatabase},
		{"Codex binary", configuration.Codex.Binary},
	} {
		if selected.path == authFile {
			return fmt.Errorf("Codex ChatGPT auth file must be distinct from %s", selected.label)
		}
	}
	controlledTrees := []struct{ label, path string }{
		{"repository root", configuration.Repository.Root},
		{"repository common directory", configuration.Repository.Binding.CommonDir},
		{"repository object directory", configuration.Repository.Binding.ObjectDir},
		{"executor runtime root", configuration.Executor.RuntimeRoot},
		{"executor writable root", configuration.Executor.WritableRoot},
		{"content runtime source", configuration.ContentRuntime.Source},
		{"builder workspace root", configuration.Workspaces.BuilderRoot},
		{"check workspace root", configuration.Workspaces.CheckRoot},
	}
	for _, source := range configuration.Authority.Sources {
		controlledTrees = append(controlledTrees, struct{ label, path string }{
			label: "authority bundle directory", path: source.BundleDirectory,
		})
	}
	for _, selected := range controlledTrees {
		if pathWithin(authFile, selected.path) {
			return fmt.Errorf("Codex ChatGPT auth file must be outside %s", selected.label)
		}
	}
	timeout, err := secondsDuration(configuration.Codex.TimeoutSeconds, "Codex timeout")
	if err != nil {
		return err
	}
	if timeout > limits.Runtime {
		return errors.New("Codex timeout exceeds the executor runtime ceiling")
	}
	if configuration.OwnerID != "" && !engine.ValidID(configuration.OwnerID) {
		return errors.New("owner_id must be a valid Sworn id when present")
	}
	return nil
}

func (configuration Config) executorLimits() (executor.Limits, error) {
	if configuration.Executor.Limits == nil {
		return executor.DefaultLimits(), nil
	}
	selected := configuration.Executor.Limits
	runtime, err := secondsDuration(selected.RuntimeSeconds, "executor runtime")
	if err != nil {
		return executor.Limits{}, err
	}
	if selected.StdoutBytes > uint64(math.MaxInt) || selected.StderrBytes > uint64(math.MaxInt) {
		return executor.Limits{}, errors.New("executor output ceiling exceeds the platform integer range")
	}
	limits := executor.Limits{
		Runtime: runtime, MemoryBytes: selected.MemoryBytes, SwapBytes: selected.SwapBytes,
		Tasks: selected.Tasks, CPUPercent: selected.CPUPercent, FileBytes: selected.FileBytes,
		TempBytes: selected.TempBytes, HomeBytes: selected.HomeBytes,
		InputBytes: selected.InputBytes, WorkspaceBytes: selected.WorkspaceBytes,
		StdoutBytes: int(selected.StdoutBytes), StderrBytes: int(selected.StderrBytes),
	}
	if err := limits.Validate(); err != nil {
		return executor.Limits{}, fmt.Errorf("validate executor limits: %w", err)
	}
	return limits, nil
}

func (configuration Config) authoritySources() ([]configservice.AuthoritySource, error) {
	sources := make([]configservice.AuthoritySource, 0, len(configuration.Authority.Sources))
	seen := make(map[string]struct{}, len(configuration.Authority.Sources))
	for index, source := range configuration.Authority.Sources {
		if err := validateCleanAbsolutePath(source.BundleDirectory, "authority bundle directory"); err != nil {
			return nil, fmt.Errorf("authority source %d: %w", index, err)
		}
		decoded, err := base64.StdEncoding.Strict().DecodeString(source.PublicKey)
		if err != nil || len(decoded) != ed25519.PublicKeySize ||
			base64.StdEncoding.EncodeToString(decoded) != source.PublicKey {
			return nil, fmt.Errorf("authority source %d has an invalid canonical Ed25519 public key", index)
		}
		publicKey := ed25519.PublicKey(slices.Clone(decoded))
		if _, err := policy.NewTrustRoot(source.SourceRef, source.AuthorizerRef, publicKey); err != nil {
			return nil, fmt.Errorf("authority source %d trust root: %w", index, err)
		}
		if _, duplicate := seen[source.SourceRef]; duplicate {
			return nil, fmt.Errorf("authority source %d duplicates source reference %q", index, source.SourceRef)
		}
		seen[source.SourceRef] = struct{}{}
		sources = append(sources, configservice.AuthoritySource{
			SourceRef: source.SourceRef, AuthorizerRef: source.AuthorizerRef,
			PublicKey: publicKey, BundleDirectory: source.BundleDirectory,
		})
	}
	return sources, nil
}

func validateConfigFile(info os.FileInfo) error {
	if info == nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("run config must be a non-symlink regular file")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return errors.New("run config must be private to its owner")
	}
	if info.Size() <= 0 || info.Size() > maximumRunConfigBytes {
		return errors.New("run config is empty or exceeds its byte ceiling")
	}
	return nil
}

func validateCleanAbsolutePath(path, label string) error {
	if path == "" || strings.ContainsRune(path, '\x00') || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return fmt.Errorf("%s must be a clean absolute path", label)
	}
	return nil
}

func secondsDuration(seconds uint64, label string) (time.Duration, error) {
	if seconds == 0 || seconds > uint64(math.MaxInt64/int64(time.Second)) {
		return 0, fmt.Errorf("%s must be a positive representable duration", label)
	}
	return time.Duration(seconds) * time.Second, nil
}

func pathWithin(path, root string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

func requireCompleteLimitsObject(contents []byte, decodedPresent bool) error {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(contents, &top); err != nil {
		return err
	}
	var configuredExecutor map[string]json.RawMessage
	if err := json.Unmarshal(top["executor"], &configuredExecutor); err != nil {
		return errors.New("executor must be a JSON object")
	}
	raw, present := configuredExecutor["limits"]
	if !present {
		return nil
	}
	if !decodedPresent || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return errors.New("executor limits must be omitted or a complete object")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return errors.New("executor limits must be omitted or a complete object")
	}
	required := []string{
		"runtime_seconds", "memory_bytes", "swap_bytes", "tasks", "cpu_percent",
		"file_bytes", "temp_bytes", "home_bytes", "input_bytes", "workspace_bytes",
		"stdout_bytes", "stderr_bytes",
	}
	for _, name := range required {
		if _, ok := fields[name]; !ok {
			return fmt.Errorf("executor limits missing required field %q", name)
		}
	}
	return nil
}
