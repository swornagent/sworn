package repo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	maxGitStdout = 16 << 20
	maxGitStderr = 256 << 10
)

type boundedBuffer struct {
	buffer    bytes.Buffer
	limit     int
	truncated bool
}

func (buffer *boundedBuffer) Write(contents []byte) (int, error) {
	written := len(contents)
	remaining := buffer.limit - buffer.buffer.Len()
	if remaining > 0 {
		if remaining > len(contents) {
			remaining = len(contents)
		}
		_, _ = buffer.buffer.Write(contents[:remaining])
	}
	if remaining < len(contents) {
		buffer.truncated = true
	}
	return written, nil
}

type gitResult struct {
	stdout   []byte
	stderr   []byte
	exitCode int
}

type gitError struct {
	args     []string
	exitCode int
	detail   string
}

func (err *gitError) Error() string {
	if err.detail == "" {
		return fmt.Sprintf("git %s exited with status %d", strings.Join(err.args, " "), err.exitCode)
	}
	return fmt.Sprintf("git %s exited with status %d: %s", strings.Join(err.args, " "), err.exitCode, err.detail)
}

func executeGit(
	ctx context.Context,
	gitPath string,
	environment []string,
	stdin io.Reader,
	args ...string,
) (gitResult, error) {
	command := exec.CommandContext(ctx, gitPath, args...)
	command.Env = environment
	command.Stdin = stdin
	stdout := &boundedBuffer{limit: maxGitStdout}
	stderr := &boundedBuffer{limit: maxGitStderr}
	command.Stdout = stdout
	command.Stderr = stderr
	err := command.Run()
	result := gitResult{stdout: stdout.buffer.Bytes(), stderr: stderr.buffer.Bytes()}
	if stdout.truncated || stderr.truncated {
		return result, errors.New("Git output exceeded the retained-output limit")
	}
	if err == nil {
		return result, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.exitCode = exitErr.ExitCode()
		return result, &gitError{
			args:     append([]string(nil), args...),
			exitCode: result.exitCode,
			detail:   strings.TrimSpace(string(result.stderr)),
		}
	}
	return result, fmt.Errorf("start git: %w", err)
}

func baseGitEnvironment(indexPath string) []string {
	environment := []string{
		"HOME=/nonexistent",
		"XDG_CONFIG_HOME=/nonexistent",
		"LANG=C",
		"LC_ALL=C",
		"TZ=UTC",
		"GIT_ATTR_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_DISCOVERY_ACROSS_FILESYSTEM=0",
		"GIT_EXTERNAL_DIFF=",
		"GIT_NO_LAZY_FETCH=1",
		"GIT_NO_REPLACE_OBJECTS=1",
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_PAGER=",
		"GIT_TERMINAL_PROMPT=0",
		"GCM_INTERACTIVE=never",
	}
	if indexPath != "" {
		environment = append(environment, "GIT_INDEX_FILE="+indexPath)
	}
	return environment
}

func baselineConfigArgs() []string {
	values := [][2]string{
		{"advice.detachedHead", "false"},
		{"commit.gpgSign", "false"},
		{"core.attributesFile", "/dev/null"},
		{"core.autocrlf", "false"},
		{"core.excludesFile", "/dev/null"},
		{"core.fileMode", "true"},
		{"core.fsmonitor", "false"},
		{"core.fsync", "committed"},
		{"core.hooksPath", "/dev/null"},
		{"core.ignoreCase", "false"},
		{"core.symlinks", "true"},
		{"credential.helper", ""},
		{"diff.external", ""},
		{"protocol.file.allow", "never"},
		{"tag.gpgSign", "false"},
	}
	args := make([]string, 0, len(values)*2)
	for _, value := range values {
		args = append(args, "-c", value[0]+"="+value[1])
	}
	return args
}

func (repository *Repository) git(
	ctx context.Context,
	stdin io.Reader,
	args ...string,
) (gitResult, error) {
	commandArgs := []string{"--git-dir=" + repository.commonDir}
	commandArgs = append(commandArgs, baselineConfigArgs()...)
	commandArgs = append(commandArgs, args...)
	return executeGit(ctx, repository.gitPath, baseGitEnvironment(""), stdin, commandArgs...)
}

type gitOperation struct {
	repository *Repository
	root       string
	gitDir     string
	indexPath  string
}

// newGitOperation creates isolated repository metadata for any command that
// observes workspace bytes. The operation reads and writes the bound object
// database explicitly, but it cannot see local config, info/exclude, hooks,
// refs, remotes, or the user's index.
func (repository *Repository) newGitOperation(ctx context.Context) (*gitOperation, func(), error) {
	root, err := repository.privateTemporaryDirectory("operation-")
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() { _ = os.RemoveAll(root) }
	template := filepath.Join(root, "template")
	if err := os.Mkdir(template, 0o700); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("create empty Git template: %w", err)
	}
	gitDir := filepath.Join(root, "git")
	args := baselineConfigArgs()
	args = append(args,
		"init", "--bare", "--quiet", "--template="+template,
		"--object-format="+repository.binding.ObjectFormat, gitDir,
	)
	if _, err := executeGit(ctx, repository.gitPath, baseGitEnvironment(""), nil, args...); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("initialize isolated Git metadata: %w", err)
	}
	return &gitOperation{
		repository: repository,
		root:       root,
		gitDir:     gitDir,
		indexPath:  filepath.Join(root, "index"),
	}, cleanup, nil
}

func (operation *gitOperation) git(
	ctx context.Context,
	worktree string,
	environment []string,
	stdin io.Reader,
	args ...string,
) (gitResult, error) {
	commandArgs := []string{"--git-dir=" + operation.gitDir}
	if worktree != "" {
		commandArgs = append(commandArgs, "--work-tree="+worktree)
	}
	commandArgs = append(commandArgs, baselineConfigArgs()...)
	commandArgs = append(commandArgs, args...)
	commandEnvironment := baseGitEnvironment(operation.indexPath)
	commandEnvironment = append(commandEnvironment,
		"GIT_OBJECT_DIRECTORY="+operation.repository.objectDir,
		"GIT_ALTERNATE_OBJECT_DIRECTORIES=",
	)
	commandEnvironment = append(commandEnvironment, environment...)
	return executeGit(ctx, operation.repository.gitPath, commandEnvironment, stdin, commandArgs...)
}

func (repository *Repository) privateTemporaryDirectory(pattern string) (string, error) {
	root := filepath.Join(repository.commonDir, "sworn", "v1", "tmp")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", fmt.Errorf("create control temporary directory: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve control temporary directory: %w", err)
	}
	if filepath.Clean(resolved) != filepath.Clean(root) {
		return "", errors.New("control temporary directory contains a symbolic-link remap")
	}
	info, err := os.Stat(root)
	if err != nil {
		return "", fmt.Errorf("inspect control temporary directory: %w", err)
	}
	if !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return "", errors.New("control temporary directory must be private")
	}
	temporary, err := os.MkdirTemp(root, pattern)
	if err != nil {
		return "", fmt.Errorf("create private operation directory: %w", err)
	}
	return temporary, nil
}

func canonicalDirectory(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve absolute path: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve path symlinks: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("inspect path: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("path %q is not a directory", resolved)
	}
	return filepath.Clean(resolved), nil
}

func pathWithin(path, parent string) bool {
	relative, err := filepath.Rel(parent, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
