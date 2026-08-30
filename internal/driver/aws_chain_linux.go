//go:build linux

package driver

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

func execAWSCommand(
	ctx context.Context,
	spec AWSChainSpec,
	environment [][]byte,
	arguments ...string,
) ([]byte, error) {
	if ctx == nil || validateAWSChainSpec(spec) != nil ||
		!validAWSCommandArguments(spec.Profile, arguments) {
		return nil, fail("AWS_NOT_CERTIFIED")
	}
	closure, err := openAWSClosure(spec)
	if err != nil {
		return nil, err
	}
	defer closeNativeFiles(closure)
	bwrap, err := trustedBubblewrap()
	if err != nil {
		return nil, fail("AWS_NOT_CERTIFIED")
	}
	statusReader, statusWriter, err := os.Pipe()
	if err != nil {
		return nil, fail("AWS_RESOLUTION_FAILED")
	}
	defer statusReader.Close()
	defer statusWriter.Close()
	statusFD := 3 + len(closure)
	bwrapArguments, err := awsBubblewrapArguments(spec, statusFD)
	if err != nil {
		return nil, err
	}
	bwrapArguments = append(bwrapArguments, arguments...)
	command := exec.CommandContext(ctx, bwrap, bwrapArguments...)
	command.ExtraFiles = append(append([]*os.File(nil), closure...), statusWriter)
	command.Env = []string{
		"LC_ALL=C", "LANG=C", "TZ=UTC", "AWS_PAGER=", "AWS_CLI_AUTO_PROMPT=off",
	}
	for _, entry := range environment {
		command.Env = append(command.Env, string(entry))
	}
	command.SysProcAttr = linuxSandboxProcessAttributes()
	command.WaitDelay = processTerminationGrace
	stdout := &boundedBuffer{maximum: MaxAWSExportBytes, retain: MaxAWSExportBytes}
	stderr := &boundedBuffer{maximum: MaxAWSListBytes, retain: MaxAWSListBytes}
	command.Stdout, command.Stderr = stdout, stderr
	if err := command.Start(); err != nil {
		return nil, fail("AWS_RESOLUTION_FAILED")
	}
	_ = statusWriter.Close()
	_, group, statusErr := readSandboxProcessGroup(
		statusReader,
		command.Process.Pid,
	)
	_ = statusReader.Close()
	if statusErr != nil {
		_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		_ = command.Wait()
		return nil, fail("AWS_RESOLUTION_FAILED")
	}
	runErr := command.Wait()
	_ = syscall.Kill(-group, syscall.SIGTERM)
	groupErr := waitProcessGroup(group)
	stderrBody, _, stderrOverflow := stderr.snapshot()
	clearBytes(stderrBody)
	if isContextError(ctx.Err()) {
		return nil, ctx.Err()
	}
	if runErr != nil || groupErr != nil {
		return nil, fail("AWS_RESOLUTION_FAILED")
	}
	body, _, overflow := stdout.snapshot()
	if overflow || stderrOverflow {
		clearBytes(body)
		return nil, fail("OUTPUT_OVERFLOW")
	}
	return body, nil
}

func openAWSClosure(spec AWSChainSpec) ([]*os.File, error) {
	binary, err := openPinnedExecutable(spec.CLI)
	if err != nil {
		return nil, fail("AWS_NOT_CERTIFIED")
	}
	files := []*os.File{binary}
	for _, runtimeFile := range spec.RuntimeFiles {
		file, openErr := openPinnedRuntimeFile(runtimeFile)
		if openErr != nil {
			closeNativeFiles(files)
			return nil, fail("AWS_NOT_CERTIFIED")
		}
		files = append(files, file)
	}
	return files, nil
}

func awsBubblewrapArguments(spec AWSChainSpec, statusFD int) ([]string, error) {
	if validateAWSChainSpec(spec) != nil || statusFD < 4 {
		return nil, fail("AWS_NOT_CERTIFIED")
	}
	arguments := []string{
		"--die-with-parent", "--new-session",
		"--info-fd", itoa(statusFD),
		"--unshare-all", "--share-net", "--unshare-user", "--disable-userns",
		"--cap-drop", "ALL",
		"--proc", "/proc", "--dev", "/dev", "--tmpfs", "/tmp",
	}
	directories := map[string]struct{}{
		"/home": {}, "/home/sworn": {},
	}
	for parent := filepath.Dir(spec.CLI.Path); parent != "/" && parent != "."; parent = filepath.Dir(parent) {
		directories[parent] = struct{}{}
	}
	for _, runtimeFile := range spec.RuntimeFiles {
		for parent := filepath.Dir(runtimeFile.Target); parent != "/" && parent != "."; parent = filepath.Dir(parent) {
			directories[parent] = struct{}{}
		}
	}
	dirList := make([]string, 0, len(directories))
	for directory := range directories {
		dirList = append(dirList, directory)
	}
	sort.Slice(dirList, func(left, right int) bool {
		leftDepth := strings.Count(dirList[left], "/")
		rightDepth := strings.Count(dirList[right], "/")
		if leftDepth == rightDepth {
			return dirList[left] < dirList[right]
		}
		return leftDepth < rightDepth
	})
	for _, directory := range dirList {
		arguments = append(arguments, "--dir", directory)
	}
	arguments = append(arguments, "--ro-bind-fd", "3", spec.CLI.Path)
	for index, runtimeFile := range spec.RuntimeFiles {
		arguments = append(
			arguments,
			"--ro-bind-fd", itoa(4+index), runtimeFile.Target,
		)
	}
	arguments = append(arguments, "--chdir", "/tmp", spec.CLI.Path)
	return arguments, nil
}

func validAWSCommandArguments(profile string, arguments []string) bool {
	expected := [][]string{
		{"--version"},
		{"configure", "list"},
		{"configure", "export-credentials", "--format", "process"},
	}
	for _, base := range expected {
		candidate := append([]string(nil), base...)
		if profile != "" && len(base) > 1 {
			candidate = append(candidate, "--profile", profile)
		}
		if equalStrings(candidate, arguments) {
			return true
		}
	}
	return false
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

// The handshake's three failure modes are materially distinct and the raise
// sites keep them distinguishable for the caller (sworn#251): a report that
// never arrived means the launcher died during setup, before forking its
// child - the only mode a caller may safely retry, because no child ever ran;
// a group probe error means the reported child died immediately; and an
// unchanged group after the start grace means a live child was never
// scheduled onto its own group, which no healthy host produces even under
// heavy load at processStartHandshakeGrace.
var (
	errSandboxGroupReportMissing = errors.New(
		"sandbox launcher exited before reporting its child",
	)
	errSandboxChildUnprobeable = errors.New(
		"sandbox child process group unprobeable",
	)
	errSandboxGroupUnchanged = errors.New(
		"sandbox child never left the parent process group",
	)
)

func readSandboxProcessGroup(
	reader *os.File,
	parentPID int,
) (int, int, error) {
	if reader == nil || parentPID <= 0 {
		return 0, 0, fail("PROCESS_START_FAILED")
	}
	var status struct {
		ChildPID int `json:"child-pid"`
	}
	if err := json.NewDecoder(reader).Decode(&status); err != nil ||
		status.ChildPID <= 0 {
		return 0, 0, errSandboxGroupReportMissing
	}
	group, err := syscall.Getpgid(status.ChildPID)
	deadline := time.Now().Add(processStartHandshakeGrace)
	for err == nil && group == parentPID && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
		group, err = syscall.Getpgid(status.ChildPID)
	}
	if err != nil || group <= 0 {
		return 0, 0, errSandboxChildUnprobeable
	}
	if group == parentPID {
		return 0, 0, errSandboxGroupUnchanged
	}
	return status.ChildPID, group, nil
}
