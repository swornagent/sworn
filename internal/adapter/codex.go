package adapter

import (
	"bytes"
	"context"
	"crypto/sha256"
	"debug/elf"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/swornagent/sworn/internal/effects"
	"github.com/swornagent/sworn/internal/executor"
	"github.com/swornagent/sworn/internal/protocol"
)

const (
	CodexBuilderProfileSchemaVersion = "sworn-codex-builder-profile-v1"
	CodexBuilderOutputSchemaVersion  = "codex-exec-jsonl-v1"

	pinnedCodexVersion          = "codex-cli 0.145.0-alpha.18"
	pinnedCodexDigest           = "sha256:16db86b6bf81cc426032fd42216dd97e60f97b149272f1f9963845a0675dae94"
	pinnedCodexSize             = int64(304169008)
	pinnedCodexToolSchemaDigest = "sha256:8d1a8331791d041c8f9ecae0f171b1f6c9df9bdaeb188d69020e8750436852aa"
	codexExecutableInput        = "codex"
	codexCredentialName         = "CODEX_API_KEY"
	codexProvider               = "openai"
	codexVersionTimeout         = 5 * time.Second
	codexMaximumEventLine       = 1 << 20
)

const codexBuilderPrompt = "Read /inputs/dispatch and /inputs/plan. Implement the authorized work in /workspace. Do not commit, create Git metadata, or modify /inputs. Run appropriate local validation. Finish when /workspace contains the complete candidate."

// CodexBuilderOptions contains the only deployment-selected Codex facts. The
// accepted CLI identity and behavior profile are pinned in this adapter; the
// model has deliberately no default.
type CodexBuilderOptions struct {
	BinaryPath string
	APIKey     string
	Model      string
	Timeout    time.Duration
}

type codexBuilderProfile struct {
	SchemaVersion       string                   `json:"schema_version"`
	BinaryPath          string                   `json:"binary_path"`
	BinaryVersion       string                   `json:"binary_version"`
	BinaryDigest        string                   `json:"binary_digest"`
	BinarySize          int64                    `json:"binary_size"`
	ExecutableInput     string                   `json:"executable_input"`
	Provider            string                   `json:"provider"`
	Model               string                   `json:"model"`
	ToolSchemaDigest    string                   `json:"tool_schema_digest"`
	Argv                []string                 `json:"argv"`
	EnvironmentNames    []string                 `json:"environment_names"`
	TimeoutNanoseconds  int64                    `json:"timeout_nanoseconds"`
	Network             executor.NetworkMode     `json:"network"`
	NestedSandbox       bool                     `json:"nested_sandbox"`
	WorkspaceAccess     executor.WorkspaceAccess `json:"workspace_access"`
	OutputSchemaVersion string                   `json:"output_schema_version"`
}

type codexCompletionPolicy struct{ profileDigest string }

func (policy codexCompletionPolicy) BuilderProfileDigest() string { return policy.profileDigest }

func (policy codexCompletionPolicy) ValidateBuilderCompletion(completion executor.RawCompletion) error {
	if completion.ExitCode != 0 || completion.Cancelled || completion.TimedOut || completion.OutputTruncated {
		return errors.New("Codex process did not reach an ordinary bounded completion")
	}
	return validateCodexJSONL(completion.Stdout)
}

// NewCodexBuilder validates the one accepted static Codex CLI and configures
// an otherwise process-neutral BuilderWorker. It does not discover a binary,
// choose a model, persist a credential, or introduce a provider abstraction.
func NewCodexBuilder(
	ctx context.Context,
	worker effects.BuilderWorker,
	options CodexBuilderOptions,
) (effects.BuilderWorker, error) {
	if err := validateUnconfiguredBuilder(worker); err != nil {
		return effects.BuilderWorker{}, err
	}
	if err := validateCodexOptions(options); err != nil {
		return effects.BuilderWorker{}, err
	}
	if err := validatePinnedCodexBinary(ctx, options.BinaryPath); err != nil {
		return effects.BuilderWorker{}, err
	}
	return configureCodexBuilder(worker, options)
}

func configureCodexBuilder(
	worker effects.BuilderWorker,
	options CodexBuilderOptions,
) (effects.BuilderWorker, error) {
	argv := codexBuilderArgv(options.Model)
	toolDigest, err := codexBuilderToolSchemaDigest()
	if err != nil {
		return effects.BuilderWorker{}, err
	}
	profile := codexBuilderProfile{
		SchemaVersion: CodexBuilderProfileSchemaVersion,
		BinaryPath:    options.BinaryPath, BinaryVersion: pinnedCodexVersion,
		BinaryDigest: pinnedCodexDigest, BinarySize: pinnedCodexSize,
		ExecutableInput: codexExecutableInput,
		Provider:        codexProvider, Model: options.Model, ToolSchemaDigest: toolDigest,
		Argv: slices.Clone(argv), EnvironmentNames: []string{codexCredentialName},
		TimeoutNanoseconds: options.Timeout.Nanoseconds(), Network: executor.NetworkHost,
		NestedSandbox: true, WorkspaceAccess: executor.WorkspaceWritableExport,
		OutputSchemaVersion: CodexBuilderOutputSchemaVersion,
	}
	encoded, err := protocol.EncodeCanonical(profile)
	if err != nil {
		return effects.BuilderWorker{}, fmt.Errorf("encode Codex builder profile: %w", err)
	}
	profileDigest := protocol.RawDigest(encoded)
	worker.Agent = pinnedCodexVersion
	worker.Argv = argv
	worker.Environment = map[string]string{codexCredentialName: options.APIKey}
	worker.Timeout = options.Timeout
	worker.ExecutableInput = &executor.Input{
		Name: codexExecutableInput, Path: options.BinaryPath, Digest: pinnedCodexDigest,
	}
	worker.Network = executor.NetworkHost
	worker.NestedSandbox = true
	worker.CompletionPolicy = codexCompletionPolicy{profileDigest: profileDigest}
	return worker, nil
}

func validateUnconfiguredBuilder(worker effects.BuilderWorker) error {
	if worker.Agent != "" || len(worker.Argv) != 0 || len(worker.Environment) != 0 ||
		worker.Timeout != 0 || worker.ExecutableInput != nil || worker.Network != "" ||
		worker.NestedSandbox || worker.CompletionPolicy != nil {
		return errors.New("Codex adapter requires a process-neutral builder worker")
	}
	return nil
}

func validateCodexOptions(options CodexBuilderOptions) error {
	if options.BinaryPath == "" {
		return errors.New("Codex binary path is required")
	}
	if strings.TrimSpace(options.Model) != options.Model || options.Model == "" ||
		len(options.Model) > 256 || strings.ContainsRune(options.Model, '\x00') {
		return errors.New("Codex model must be explicit and bounded")
	}
	if strings.TrimSpace(options.APIKey) != options.APIKey || options.APIKey == "" ||
		len(options.APIKey) > 8192 || strings.ContainsRune(options.APIKey, '\x00') {
		return errors.New("Codex API key must be present and bounded")
	}
	if options.Timeout <= 0 {
		return errors.New("Codex builder timeout must be positive")
	}
	return nil
}

func validatePinnedCodexBinary(parent context.Context, path string) (resultErr error) {
	if parent == nil {
		return errors.New("validate Codex binary: context is required")
	}
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return errors.New("Codex binary must use a clean absolute path")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf("resolve Codex binary: %w", err)
	}
	if resolved != path {
		return errors.New("Codex binary path must name the exact file without symlinks")
	}
	pathIdentity, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect Codex binary path: %w", err)
	}
	source, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open Codex binary: %w", err)
	}
	defer source.Close() //nolint:errcheck
	stagedPath, cleanup, err := stageCodexProbeBinary(
		source, pathIdentity, pinnedCodexSize, pinnedCodexDigest, "", io.Copy,
	)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, cleanup()) }()

	staged, err := os.Open(stagedPath)
	if err != nil {
		return fmt.Errorf("open staged Codex probe: %w", err)
	}
	defer staged.Close() //nolint:errcheck
	binary, err := elf.NewFile(staged)
	if err != nil {
		return fmt.Errorf("inspect Codex ELF: %w", err)
	}
	defer binary.Close() //nolint:errcheck
	if binary.Type != elf.ET_DYN {
		return errors.New("Codex binary must be a static PIE executable")
	}
	for _, program := range binary.Progs {
		if program.Type == elf.PT_INTERP {
			return errors.New("Codex binary must not depend on an ELF interpreter")
		}
	}

	versionContext, cancel := context.WithTimeout(parent, codexVersionTimeout)
	defer cancel()
	versionHome := filepath.Join(filepath.Dir(stagedPath), "home")
	if err := os.Mkdir(versionHome, 0o700); err != nil {
		return fmt.Errorf("create Codex version home: %w", err)
	}
	if err := os.Chmod(versionHome, 0o700); err != nil {
		return fmt.Errorf("secure Codex version home: %w", err)
	}
	command := exec.CommandContext(versionContext, stagedPath, "--version")
	command.Env = []string{
		"CODEX_HOME=" + versionHome,
		"HOME=" + versionHome,
		"LANG=C", "LC_ALL=C", "PATH=/usr/bin:/bin", "TZ=UTC",
	}
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("read pinned Codex version: %w", err)
	}
	if len(output) > 4096 {
		return errors.New("pinned Codex version output exceeds the byte ceiling")
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	version := strings.TrimSpace(lines[len(lines)-1])
	if version != pinnedCodexVersion {
		return fmt.Errorf("Codex binary version %q is not pinned profile %q", version, pinnedCodexVersion)
	}
	return nil
}

type codexCopyFunc func(io.Writer, io.Reader) (int64, error)

// stageCodexProbeBinary reads only the retained descriptor and hashes the
// exact bytes written to its private executable before returning that path.
func stageCodexProbeBinary(
	source *os.File,
	pathIdentity os.FileInfo,
	expectedSize int64,
	expectedDigest string,
	temporaryParent string,
	copyBytes codexCopyFunc,
) (_ string, _ func() error, resultErr error) {
	if source == nil || pathIdentity == nil || expectedSize <= 0 ||
		!protocol.ValidDigest(expectedDigest) || copyBytes == nil {
		return "", nil, errors.New("stage Codex probe requires an exact source and identity")
	}
	sourceIdentity, err := source.Stat()
	if err != nil || !os.SameFile(pathIdentity, sourceIdentity) ||
		!sourceIdentity.Mode().IsRegular() || sourceIdentity.Mode().Perm()&0o111 == 0 {
		return "", nil, errors.New("Codex source descriptor does not match its path identity")
	}
	if sourceIdentity.Size() != expectedSize {
		return "", nil, fmt.Errorf("Codex binary size %d is not pinned profile %d", sourceIdentity.Size(), expectedSize)
	}
	root, err := os.MkdirTemp(temporaryParent, "sworn-codex-probe-")
	if err != nil {
		return "", nil, fmt.Errorf("create private Codex probe root: %w", err)
	}
	cleanup := func() error { return os.RemoveAll(root) }
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, cleanup())
		}
	}()
	if err := os.Chmod(root, 0o700); err != nil {
		return "", nil, fmt.Errorf("secure private Codex probe root: %w", err)
	}
	path := filepath.Join(root, "codex")
	destination, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o500)
	if err != nil {
		return "", nil, fmt.Errorf("create private Codex probe: %w", err)
	}
	closed := false
	defer func() {
		if !closed {
			resultErr = errors.Join(resultErr, destination.Close())
		}
	}()
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return "", nil, fmt.Errorf("rewind Codex source descriptor: %w", err)
	}
	hasher := sha256.New()
	written, copyErr := copyBytes(io.MultiWriter(destination, hasher), source)
	if copyErr != nil {
		return "", nil, fmt.Errorf("copy private Codex probe: %w", copyErr)
	}
	if written != expectedSize {
		return "", nil, fmt.Errorf("copy private Codex probe: short write %d of %d", written, expectedSize)
	}
	if err := destination.Sync(); err != nil {
		return "", nil, fmt.Errorf("sync private Codex probe: %w", err)
	}
	if err := destination.Close(); err != nil {
		return "", nil, fmt.Errorf("close private Codex probe: %w", err)
	}
	closed = true
	if err := os.Chmod(path, 0o500); err != nil {
		return "", nil, fmt.Errorf("secure private Codex probe: %w", err)
	}
	digest := "sha256:" + hex.EncodeToString(hasher.Sum(nil))
	if digest != expectedDigest {
		return "", nil, fmt.Errorf("Codex binary digest %s is not the pinned %s profile", digest, expectedDigest)
	}
	stagedIdentity, err := os.Lstat(path)
	if err != nil || !stagedIdentity.Mode().IsRegular() || stagedIdentity.Mode().Perm() != 0o500 ||
		stagedIdentity.Size() != expectedSize {
		return "", nil, errors.New("private Codex probe identity is invalid")
	}
	return path, cleanup, nil
}

func codexBuilderToolNames() []string {
	return []string{
		"custom:apply_patch",
		"function:exec_command",
		"function:request_user_input",
		"function:update_plan",
		"function:view_image",
		"function:write_stdin",
	}
}

func codexBuilderToolSchemaDigest() (string, error) {
	if !protocol.ValidDigest(pinnedCodexToolSchemaDigest) {
		return "", errors.New("pinned Codex tool schema digest is invalid")
	}
	return pinnedCodexToolSchemaDigest, nil
}

func codexBuilderArgv(model string) []string {
	return codexBuilderArgvWithPrompt(model, codexBuilderPrompt)
}

func codexBuilderArgvWithPrompt(model, prompt string) []string {
	return []string{
		"/inputs/codex",
		"-a", "never",
		"-s", "workspace-write",
		"-m", model,
		"-c", `model_provider="openai"`,
		"-c", `web_search="disabled"`,
		"-c", `sandbox_workspace_write.network_access=false`,
		"-c", `shell_environment_policy.inherit="none"`,
		"-c", `shell_environment_policy.set={PATH="/usr/bin:/bin",HOME="/home/sworn",TMPDIR="/tmp",LANG="C",LC_ALL="C",TZ="UTC"}`,
		"-c", `allow_login_shell=false`,
		"-c", `history.persistence="none"`,
		"-c", `check_for_update_on_startup=false`,
		"-c", `features.enable_request_compression=false`,
		"-c", `features.apps=false`,
		"-c", `features.goals=false`,
		"-c", `features.hooks=false`,
		"-c", `features.memories=false`,
		"-c", `features.multi_agent=false`,
		"-c", `features.remote_plugin=false`,
		"-c", `features.shell_snapshot=false`,
		"-c", `features.skill_mcp_dependency_install=false`,
		"-C", "/tmp",
		"exec",
		"--strict-config",
		"--ephemeral",
		"--ignore-user-config",
		"--ignore-rules",
		"--skip-git-repo-check",
		"--json",
		"--add-dir", "/workspace",
		prompt,
	}
}

func validateCodexJSONL(contents []byte) error {
	if len(contents) == 0 {
		return errors.New("Codex JSONL output is empty")
	}
	lines := bytes.Split(contents, []byte{'\n'})
	threadStarted, turnStarted, turnCompleted := false, false, false
	seenEvents := 0
	for index, line := range lines {
		line = bytes.TrimSuffix(line, []byte{'\r'})
		if len(line) == 0 {
			if index == len(lines)-1 {
				continue
			}
			return fmt.Errorf("Codex JSONL event %d is empty", index+1)
		}
		if len(line) > codexMaximumEventLine {
			return fmt.Errorf("Codex JSONL event %d exceeds the byte ceiling", index+1)
		}
		if turnCompleted {
			return errors.New("Codex emitted an event after turn completion")
		}
		if _, err := protocol.CanonicalizeJSON(line); err != nil {
			return fmt.Errorf("decode Codex JSONL event %d: %w", index+1, err)
		}
		var event struct {
			Type     string          `json:"type"`
			ThreadID string          `json:"thread_id"`
			Usage    json.RawMessage `json:"usage"`
		}
		if err := json.Unmarshal(line, &event); err != nil {
			return fmt.Errorf("decode Codex JSONL event %d: %w", index+1, err)
		}
		seenEvents++
		switch event.Type {
		case "thread.started":
			if threadStarted || turnStarted || event.ThreadID == "" || len(event.ThreadID) > 256 {
				return errors.New("Codex JSONL has an invalid thread start")
			}
			threadStarted = true
		case "turn.started":
			if !threadStarted || turnStarted {
				return errors.New("Codex JSONL has an invalid turn start")
			}
			turnStarted = true
		case "item.started", "item.updated", "item.completed":
			if !turnStarted {
				return errors.New("Codex JSONL item is outside the active turn")
			}
		case "turn.completed":
			if !turnStarted || len(event.Usage) == 0 || bytes.Equal(event.Usage, []byte("null")) {
				return errors.New("Codex JSONL has an invalid terminal completion")
			}
			var usage map[string]json.RawMessage
			if err := json.Unmarshal(event.Usage, &usage); err != nil || len(usage) == 0 {
				return errors.New("Codex JSONL terminal completion lacks usage")
			}
			turnCompleted = true
		case "turn.failed", "error":
			return fmt.Errorf("Codex JSONL reported terminal failure %q", event.Type)
		default:
			return fmt.Errorf("Codex JSONL contains unsupported event %q", event.Type)
		}
	}
	if seenEvents == 0 || !threadStarted || !turnStarted || !turnCompleted {
		return errors.New("Codex JSONL lacks one complete thread and turn")
	}
	return nil
}
