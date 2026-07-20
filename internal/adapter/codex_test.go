package adapter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/effects"
	"github.com/swornagent/sworn/internal/executor"
)

func TestConfigureCodexBuilderPinsOneExplicitProfile(t *testing.T) {
	t.Parallel()
	options := CodexBuilderOptions{
		BinaryPath: "/opt/sworn/codex", APIKey: "secret-one",
		Model: "gpt-explicit", Timeout: 4 * time.Minute,
	}
	worker, err := configureCodexBuilder(effects.BuilderWorker{}, options)
	if err != nil {
		t.Fatal(err)
	}
	if worker.Agent != pinnedCodexVersion || worker.Timeout != options.Timeout ||
		worker.Network != executor.NetworkHost || !worker.NestedSandbox {
		t.Fatalf("Codex worker capability profile = %#v", worker)
	}
	if worker.ExecutableInput == nil || worker.ExecutableInput.Name != codexExecutableInput ||
		worker.ExecutableInput.Path != options.BinaryPath || worker.ExecutableInput.Digest != pinnedCodexDigest {
		t.Fatalf("Codex executable input = %#v", worker.ExecutableInput)
	}
	if !reflect.DeepEqual(worker.Environment, map[string]string{codexCredentialName: options.APIKey}) {
		t.Fatalf("Codex environment = %#v", worker.Environment)
	}
	if len(worker.Argv) == 0 || worker.Argv[0] != "/inputs/codex" ||
		!containsCodexArguments(worker.Argv, []string{"-m", options.Model}) ||
		!containsCodexArguments(worker.Argv, []string{"-c", `model_provider="openai"`}) ||
		!containsCodexArguments(worker.Argv, []string{"exec", "--strict-config", "--ephemeral", "--ignore-user-config", "--ignore-rules"}) ||
		!containsCodexArguments(worker.Argv, []string{"--json", "--add-dir", "/workspace", codexBuilderPrompt}) {
		t.Fatalf("Codex argv = %#v", worker.Argv)
	}
	joined := strings.Join(worker.Argv, "\x00")
	if strings.Contains(joined, "base_url") || strings.Contains(joined, "model_providers.") ||
		strings.Contains(joined, options.APIKey) {
		t.Fatalf("Codex argv introduced provider transport or credential: %q", joined)
	}
	firstDigest := worker.CompletionPolicy.BuilderProfileDigest()
	rotated := options
	rotated.APIKey = "secret-two"
	rotatedWorker, err := configureCodexBuilder(effects.BuilderWorker{}, rotated)
	if err != nil {
		t.Fatal(err)
	}
	if rotatedWorker.CompletionPolicy.BuilderProfileDigest() != firstDigest {
		t.Fatal("credential rotation changed the non-secret Codex profile digest")
	}
	changedModel := options
	changedModel.Model = "gpt-other"
	changedWorker, err := configureCodexBuilder(effects.BuilderWorker{}, changedModel)
	if err != nil {
		t.Fatal(err)
	}
	if changedWorker.CompletionPolicy.BuilderProfileDigest() == firstDigest {
		t.Fatal("model change did not change the Codex profile digest")
	}
}

func TestCodexBuilderRequiresExplicitUnconfiguredInputs(t *testing.T) {
	t.Parallel()
	valid := CodexBuilderOptions{
		BinaryPath: "/nonexistent/codex", APIKey: "key", Model: "gpt-explicit", Timeout: time.Minute,
	}
	for _, test := range []struct {
		name    string
		worker  effects.BuilderWorker
		options CodexBuilderOptions
		want    string
	}{
		{name: "model", options: CodexBuilderOptions{BinaryPath: valid.BinaryPath, APIKey: "key", Timeout: time.Minute}, want: "model"},
		{name: "credential", options: CodexBuilderOptions{BinaryPath: valid.BinaryPath, Model: "gpt-explicit", Timeout: time.Minute}, want: "API key"},
		{name: "timeout", options: CodexBuilderOptions{BinaryPath: valid.BinaryPath, APIKey: "key", Model: "gpt-explicit"}, want: "timeout"},
		{name: "preconfigured", worker: effects.BuilderWorker{Agent: "other"}, options: valid, want: "process-neutral"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewCodexBuilder(context.Background(), test.worker, test.options)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("NewCodexBuilder error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestPinnedCodexBinaryRejectsMutableNamesBeforeExecution(t *testing.T) {
	t.Parallel()
	target := filepath.Join(t.TempDir(), "codex")
	if err := os.WriteFile(target, []byte("not the pinned binary\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "codex-link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := validatePinnedCodexBinary(context.Background(), link); err == nil ||
		!strings.Contains(err.Error(), "without symlinks") {
		t.Fatalf("symlink error = %v", err)
	}
	if err := validatePinnedCodexBinary(context.Background(), target); err == nil ||
		!strings.Contains(err.Error(), "size") {
		t.Fatalf("unpinned binary error = %v", err)
	}
}

func TestStagedCodexProbeExecutesRetainedVerifiedBytesAfterPathSwap(t *testing.T) {
	t.Parallel()
	sourceRoot := t.TempDir()
	sourcePath := filepath.Join(sourceRoot, "codex")
	verified := []byte("#!/bin/sh\nprintf 'verified-copy\\n'\n")
	if err := os.WriteFile(sourcePath, verified, 0o700); err != nil {
		t.Fatal(err)
	}
	pathIdentity, err := os.Lstat(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close() //nolint:errcheck
	stagingRoot := t.TempDir()
	stagedPath, cleanup, err := stageCodexProbeBinary(
		source, pathIdentity, int64(len(verified)), codexTestDigest(verified), stagingRoot, io.Copy,
	)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(stagedPath)
	if err != nil || info.Mode().Perm() != 0o500 || info.Size() != int64(len(verified)) {
		t.Fatalf("staged probe identity = %#v, %v", info, err)
	}
	if err := os.Rename(sourcePath, sourcePath+".verified"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcePath, []byte("#!/bin/sh\nprintf 'unverified-path\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(stagedPath).Output()
	if err != nil || string(output) != "verified-copy\n" {
		t.Fatalf("staged probe output = %q, %v", output, err)
	}
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	assertCodexStagingRootEmpty(t, stagingRoot)
}

func TestStagedCodexProbeRejectsIdentityAndShortCopyWithoutResidue(t *testing.T) {
	t.Parallel()
	contents := []byte("#!/bin/sh\nexit 0\n")
	newSource := func(t *testing.T) (string, *os.File, os.FileInfo) {
		t.Helper()
		path := filepath.Join(t.TempDir(), "codex")
		if err := os.WriteFile(path, contents, 0o700); err != nil {
			t.Fatal(err)
		}
		identity, err := os.Lstat(path)
		if err != nil {
			t.Fatal(err)
		}
		file, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = file.Close() })
		return path, file, identity
	}
	t.Run("foreign path identity", func(t *testing.T) {
		_, source, _ := newSource(t)
		_, _, foreign := newSource(t)
		stagingRoot := t.TempDir()
		if _, _, err := stageCodexProbeBinary(
			source, foreign, int64(len(contents)), codexTestDigest(contents), stagingRoot, io.Copy,
		); err == nil || !strings.Contains(err.Error(), "identity") {
			t.Fatalf("foreign identity error = %v", err)
		}
		assertCodexStagingRootEmpty(t, stagingRoot)
	})
	t.Run("short copy", func(t *testing.T) {
		_, source, identity := newSource(t)
		stagingRoot := t.TempDir()
		shortCopy := func(destination io.Writer, input io.Reader) (int64, error) {
			return io.CopyN(destination, input, int64(len(contents)-1))
		}
		if _, _, err := stageCodexProbeBinary(
			source, identity, int64(len(contents)), codexTestDigest(contents), stagingRoot, shortCopy,
		); err == nil || !strings.Contains(err.Error(), "short write") {
			t.Fatalf("short-copy error = %v", err)
		}
		assertCodexStagingRootEmpty(t, stagingRoot)
	})
}

func codexTestDigest(contents []byte) string {
	digest := sha256.Sum256(contents)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func assertCodexStagingRootEmpty(t *testing.T, root string) {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("Codex staging residue = %#v", entries)
	}
}

func TestCodexCompletionRequiresOneSuccessfulTerminalTurn(t *testing.T) {
	t.Parallel()
	valid := strings.Join([]string{
		`{"type":"thread.started","thread_id":"thread-1"}`,
		`{"type":"turn.started"}`,
		`{"type":"item.started","item":{"type":"command_execution"}}`,
		`{"type":"item.completed","item":{"type":"agent_message"}}`,
		`{"type":"turn.completed","usage":{"input_tokens":10,"output_tokens":2}}`,
		"",
	}, "\n")
	if err := validateCodexJSONL([]byte(valid)); err != nil {
		t.Fatalf("validate Codex JSONL: %v", err)
	}
	for _, test := range []struct {
		name   string
		output string
	}{
		{name: "empty", output: ""},
		{name: "malformed", output: "not-json\n"},
		{name: "missing thread", output: `{"type":"turn.started"}` + "\n"},
		{name: "failed", output: strings.Replace(valid, `{"type":"turn.completed","usage":{"input_tokens":10,"output_tokens":2}}`, `{"type":"turn.failed"}`, 1)},
		{name: "missing usage", output: strings.Replace(valid, `,"usage":{"input_tokens":10,"output_tokens":2}`, "", 1)},
		{name: "after terminal", output: valid + `{"type":"item.completed"}` + "\n"},
		{name: "unknown", output: strings.Replace(valid, `{"type":"item.started","item":{"type":"command_execution"}}`, `{"type":"future.event"}`, 1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := validateCodexJSONL([]byte(test.output)); err == nil {
				t.Fatal("invalid Codex JSONL was accepted")
			}
		})
	}
	policy := codexCompletionPolicy{profileDigest: pinnedCodexDigest}
	if err := policy.ValidateBuilderCompletion(executor.RawCompletion{ExitCode: 0, Stdout: []byte(valid)}); err != nil {
		t.Fatalf("completion policy: %v", err)
	}
	if err := policy.ValidateBuilderCompletion(executor.RawCompletion{ExitCode: 1, Stdout: []byte(valid)}); err == nil {
		t.Fatal("nonzero Codex completion was accepted")
	}
}

func TestCodexToolSchemaIsTheProvenFixedSet(t *testing.T) {
	t.Parallel()
	want := []string{
		"custom:apply_patch",
		"function:exec_command",
		"function:request_user_input",
		"function:update_plan",
		"function:view_image",
		"function:write_stdin",
	}
	if got := codexBuilderToolNames(); !slices.Equal(got, want) {
		t.Fatalf("Codex tool schema = %#v", got)
	}
	digest, err := codexBuilderToolSchemaDigest()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(digest, "sha256:") || len(digest) != 71 {
		t.Fatalf("Codex tool schema digest = %q", digest)
	}
}

func containsCodexArguments(arguments, sequence []string) bool {
	for start := 0; start+len(sequence) <= len(arguments); start++ {
		if slices.Equal(arguments[start:start+len(sequence)], sequence) {
			return true
		}
	}
	return false
}
