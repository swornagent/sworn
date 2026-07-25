//go:build linux

package driver

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

const processTerminationGrace = 250 * time.Millisecond

var (
	bubblewrapProbeOnce sync.Once
	bubblewrapProbeErr  error
)

func platformInvoke(parent context.Context, invocation Invocation) (Observation, error) {
	if err := validateProviderConfig(invocation.Selected.Provider); err != nil {
		return Observation{}, err
	}
	executableFile, err := openPinnedExecutable(invocation.Selected.Provider.Executable)
	if err != nil {
		return Observation{}, err
	}
	defer executableFile.Close()
	projection, err := StageInputProjection(
		invocation.Request.Workspace.Path,
		invocation.Request.Inputs,
		invocation.Inputs,
	)
	if err != nil {
		return Observation{}, err
	}
	defer projection.Close()
	workspaceFile, err := openPinnedDirectory(invocation.Request.Workspace.Path)
	if err != nil {
		return Observation{}, err
	}
	defer workspaceFile.Close()
	projectionFile, err := openPinnedDirectory(projection.Root())
	if err != nil {
		return Observation{}, err
	}
	defer projectionFile.Close()

	var before workspaceManifest
	if invocation.Request.Workspace.Access == ReadOnly {
		before, err = captureWorkspaceManifest(invocation.Request.Workspace.Path)
		if err != nil {
			return Observation{}, err
		}
	}
	projectionTarget, err := prepareProjectionTarget(invocation.Request.Workspace.Path)
	if err != nil {
		return Observation{}, err
	}
	var cleanupTargetOnce sync.Once
	cleanupTarget := func() {
		cleanupTargetOnce.Do(func() {
			_ = cleanupProjectionTarget(projectionTarget)
		})
	}
	defer cleanupTarget()
	requestBody, err := EncodeRequest(invocation.Request)
	if err != nil {
		return Observation{}, err
	}
	server, err := NewSubmissionServer(invocation.Permission)
	if err != nil {
		return Observation{}, err
	}
	parentEndpoint, childEndpoint, err := socketPair()
	if err != nil {
		return Observation{}, err
	}
	defer parentEndpoint.Close()
	defer childEndpoint.Close()

	bwrap, err := trustedBubblewrap()
	if err != nil {
		return Observation{}, fail("ISOLATION_UNAVAILABLE")
	}
	args, err := bubblewrapArguments(invocation)
	if err != nil {
		return Observation{}, err
	}
	command := exec.Command(bwrap, args...)
	command.Stdin = bytes.NewReader(requestBody)
	command.Env = []string{}
	command.ExtraFiles = []*os.File{
		childEndpoint,
		executableFile,
		workspaceFile,
		projectionFile,
	}
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	var done = make(chan struct{})
	var terminateOnce sync.Once
	terminate := func() {
		terminateOnce.Do(func() {
			_ = parentEndpoint.Close()
			if command.Process == nil {
				return
			}
			_ = syscall.Kill(-command.Process.Pid, syscall.SIGTERM)
			go func() {
				timer := time.NewTimer(processTerminationGrace)
				defer timer.Stop()
				select {
				case <-done:
				case <-timer.C:
					_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
				}
			}()
		})
	}
	stdout := &boundedBuffer{
		maximum:    MaxStdoutBytes,
		retain:     MaxStdoutBytes,
		onOverflow: terminate,
	}
	stderr := &boundedBuffer{
		retain: MaxStderrRetain,
	}
	command.Stdout = stdout
	command.Stderr = stderr

	ctx, cancel := invocationContext(parent, invocation.Request.Limits.TimeoutMillis)
	defer cancel()
	if err := ctx.Err(); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return Observation{
				Diagnostic: Diagnostic{Code: "invocation_timeout"},
			}, fail("INVOCATION_TIMEOUT")
		}
		return Observation{
			Diagnostic: Diagnostic{Code: "invocation_cancelled"},
		}, fail("INVOCATION_CANCELLED")
	}
	if err := command.Start(); err != nil {
		return Observation{}, fail("PROCESS_START_FAILED")
	}
	_ = childEndpoint.Close()
	endpointResult := make(chan error, 1)
	go func() {
		endpointResult <- serveSubmissionEndpoint(parentEndpoint, server)
	}()
	go func() {
		select {
		case <-ctx.Done():
			terminate()
		case <-done:
		}
	}()

	waitErr := command.Wait()
	close(done)
	_ = parentEndpoint.Close()
	endpointErr := <-endpointResult
	cleanupTarget()

	stdoutBody, _, stdoutOverflow := stdout.snapshot()
	_, stderrBytes, stderrTruncated := stderr.snapshot()
	diagnostic := Diagnostic{
		Code:        "none",
		StderrBytes: stderrBytes,
		Truncated:   stderrTruncated || stderrBytes > MaxStderrRetain,
	}

	if err := workspacePostcheck(invocation.Request.Workspace, projection, before); err != nil {
		diagnostic.Code = "workspace_postcheck_failed"
		return Observation{Diagnostic: diagnostic}, err
	}
	if ctx.Err() != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			diagnostic.Code = "invocation_timeout"
			return Observation{Diagnostic: diagnostic}, fail("INVOCATION_TIMEOUT")
		}
		diagnostic.Code = "invocation_cancelled"
		return Observation{Diagnostic: diagnostic}, fail("INVOCATION_CANCELLED")
	}
	if stdoutOverflow {
		diagnostic.Code = "stdout_overflow"
		return Observation{Diagnostic: diagnostic}, fail("OUTPUT_OVERFLOW")
	}
	if waitErr != nil {
		if len(stdoutBody) != 0 {
			diagnostic.Code = "nonzero_with_stdout"
			return Observation{Diagnostic: diagnostic}, fail("PROTOCOL_FAILURE")
		}
		diagnostic.Code = "process_failed"
		return Observation{Diagnostic: diagnostic}, fail("PROCESS_FAILED")
	}
	if endpointErr != nil {
		diagnostic.Code = "submission_protocol_failed"
		return Observation{Diagnostic: diagnostic}, fail("SUBMISSION_PROTOCOL_FAILED")
	}
	result, err := DecodeResult(stdoutBody, ResultBinding{
		InvocationID:  invocation.Request.InvocationID,
		DriverID:      invocation.Selected.Provider.DriverID,
		DriverVersion: invocation.Selected.Provider.DriverVersion,
		Model:         &invocation.Selected.Model,
		BindModel:     true,
	})
	if err != nil {
		diagnostic.Code = "invalid_driver_result"
		return Observation{Diagnostic: diagnostic}, err
	}
	if int64(len([]byte(result.Text))) > invocation.Request.Limits.OutputBytes {
		diagnostic.Code = "result_limit_exceeded"
		return Observation{Diagnostic: diagnostic}, fail("RESOURCE_LIMIT")
	}
	usage, err := NormalizeUsage(result.Usage, nil)
	if err != nil {
		diagnostic.Code = "invalid_usage"
		return Observation{Diagnostic: diagnostic}, err
	}
	observation := Observation{
		TransportStatus: result.TransportStatus,
		DurationMillis:  result.DurationMillis,
		TextBytes:       int64(len([]byte(result.Text))),
		TextDigest:      Digest([]byte(result.Text)),
		Usage:           usage,
		Diagnostic:      diagnostic,
	}
	submissionBody, seal, sealBytes, accepted := server.Accepted()
	if result.TransportStatus != Completed || !accepted {
		if result.TransportStatus == Completed {
			observation.Diagnostic.Code = "submission_absent"
		}
		return observation, nil
	}
	if seal.InvocationID != invocation.Request.InvocationID ||
		seal.SubmissionDigest != Digest(submissionBody) {
		observation.Diagnostic.Code = "submission_binding_failed"
		return observation, fail("SUBMISSION_BINDING_MISMATCH")
	}
	observation.Handoff = &SealedHandoff{
		SubmissionBytes:  submissionBody,
		SubmissionDigest: Digest(submissionBody),
		SealBytes:        sealBytes,
		SealDigest:       Digest(sealBytes),
	}
	return observation, nil
}

func trustedBubblewrap() (string, error) {
	const executable = "/usr/bin/bwrap"
	info, err := os.Lstat(executable)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 ||
		info.Mode().Perm()&0o022 != 0 {
		return "", fail("ISOLATION_UNAVAILABLE")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != 0 {
		return "", fail("ISOLATION_UNAVAILABLE")
	}
	bubblewrapProbeOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, executable, "--help")
		command.Env = []string{}
		output, err := command.Output()
		if err != nil {
			bubblewrapProbeErr = fail("ISOLATION_UNAVAILABLE")
			return
		}
		for _, required := range []string{
			"--unshare-all",
			"--unshare-user",
			"--disable-userns",
			"--cap-drop",
			"--clearenv",
			"--ro-bind-fd",
			"--bind-fd",
			"--die-with-parent",
		} {
			if !bytes.Contains(output, []byte(required)) {
				bubblewrapProbeErr = fail("ISOLATION_UNAVAILABLE")
				return
			}
		}
	})
	if bubblewrapProbeErr != nil {
		return "", bubblewrapProbeErr
	}
	return executable, nil
}

func bubblewrapArguments(invocation Invocation) ([]string, error) {
	if err := validateNetworkPolicy(
		invocation.Selected.Provider.DriverID,
		invocation.Selected.Provider.Network,
	); err != nil {
		return nil, err
	}
	workspace := invocation.Request.Workspace.Path
	targetProjection := filepath.Join(workspace, ".sworn-inputs", "v1")
	arguments := []string{
		"--die-with-parent",
		"--unshare-all",
		"--unshare-user",
		"--disable-userns",
		"--cap-drop", "ALL",
		"--clearenv",
		"--proc", "/proc",
		"--dev", "/dev",
		"--tmpfs", "/tmp",
		"--dir", "/home",
		"--dir", "/home/sworn",
		"--dir", "/sworn",
		"--ro-bind", "/usr", "/usr",
	}
	for _, systemPath := range []string{"/lib", "/lib64", "/etc/ssl/certs"} {
		if _, err := os.Stat(systemPath); err == nil {
			arguments = append(arguments, "--ro-bind", systemPath, systemPath)
		}
	}
	if invocation.Selected.Provider.Network == NetworkRequired {
		arguments = append(arguments, "--share-net")
	}
	if invocation.Request.Workspace.Access == ReadOnly {
		arguments = append(arguments, "--ro-bind-fd", "5", workspace)
	} else {
		arguments = append(arguments, "--bind-fd", "5", workspace)
	}
	arguments = append(arguments,
		"--ro-bind-fd", "6", targetProjection,
		"--ro-bind-fd", "4", "/sworn/driver",
		"--chdir", workspace,
		"--setenv", "HOME", "/home/sworn",
		"--setenv", "TMPDIR", "/tmp",
		"--setenv", "LANG", "C.UTF-8",
		"--setenv", "LC_ALL", "C.UTF-8",
		"--setenv", "TZ", "UTC",
		"--setenv", "PATH", "/usr/bin:/bin",
		"--setenv", "PWD", workspace,
		"--setenv", SubmissionProtocolEnvironment, SubmissionProtocolID,
		"--setenv", SubmissionFDEnvironment, "3",
	)
	if invocation.Selected.Provider.DriverID == FakeDriverID {
		arguments = append(arguments,
			"--setenv", "BATON_FAKE_PROFILE", string(invocation.FakeProfile),
		)
	}
	arguments = append(arguments,
		"/sworn/driver", "run",
	)
	return arguments, nil
}

func openPinnedDirectory(name string) (*os.File, error) {
	pathInfo, err := os.Lstat(name)
	if err != nil || !pathInfo.IsDir() {
		return nil, fail("INVALID_DIRECTORY")
	}
	file, err := os.Open(name)
	if err != nil {
		return nil, fail("INVALID_DIRECTORY")
	}
	fileInfo, err := file.Stat()
	if err != nil || !fileInfo.IsDir() || !os.SameFile(pathInfo, fileInfo) {
		_ = file.Close()
		return nil, fail("INVALID_DIRECTORY")
	}
	return file, nil
}

func openPinnedExecutable(identity ExecutableIdentity) (*os.File, error) {
	pathInfo, err := os.Lstat(identity.Path)
	if err != nil || !pathInfo.Mode().IsRegular() ||
		pathInfo.Mode().Perm()&0o111 == 0 {
		return nil, fail("INVALID_EXECUTABLE")
	}
	file, err := os.Open(identity.Path)
	if err != nil {
		return nil, fail("INVALID_EXECUTABLE")
	}
	ok := false
	defer func() {
		if !ok {
			_ = file.Close()
		}
	}()
	fileInfo, err := file.Stat()
	if err != nil || !fileInfo.Mode().IsRegular() ||
		fileInfo.Mode().Perm()&0o111 == 0 ||
		!os.SameFile(pathInfo, fileInfo) {
		return nil, fail("INVALID_EXECUTABLE")
	}
	digest, err := streamDigest(file)
	if err != nil || digest != identity.Digest {
		return nil, fail("EXECUTABLE_IDENTITY_MISMATCH")
	}
	if _, err := file.Seek(0, 0); err != nil {
		return nil, fail("INVALID_EXECUTABLE")
	}
	ok = true
	return file, nil
}

func prepareProjectionTarget(workspace string) (string, error) {
	root := filepath.Join(workspace, ".sworn-inputs")
	target := filepath.Join(root, "v1")
	if err := os.Mkdir(root, 0o700); err != nil {
		return "", fail("INPUT_STAGE_FAILED")
	}
	if err := os.Mkdir(target, 0o500); err != nil {
		_ = os.Remove(root)
		return "", fail("INPUT_STAGE_FAILED")
	}
	if err := os.Chmod(root, 0o500); err != nil {
		_ = os.Remove(target)
		_ = os.Remove(root)
		return "", fail("INPUT_STAGE_FAILED")
	}
	return root, nil
}

func cleanupProjectionTarget(root string) error {
	if root == "" {
		return nil
	}
	if err := os.Chmod(root, 0o700); err != nil {
		return fail("INPUT_CLEANUP_FAILED")
	}
	if err := os.Remove(filepath.Join(root, "v1")); err != nil {
		return fail("INPUT_CLEANUP_FAILED")
	}
	if err := os.Remove(root); err != nil {
		return fail("INPUT_CLEANUP_FAILED")
	}
	return nil
}

func socketPair() (*os.File, *os.File, error) {
	files, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		return nil, nil, fail("ENDPOINT_UNAVAILABLE")
	}
	syscall.CloseOnExec(files[0])
	syscall.CloseOnExec(files[1])
	parent := os.NewFile(uintptr(files[0]), "sworn-submission-parent")
	child := os.NewFile(uintptr(files[1]), "sworn-submission-child")
	if parent == nil || child == nil {
		if parent != nil {
			_ = parent.Close()
		}
		if child != nil {
			_ = child.Close()
		}
		return nil, nil, fail("ENDPOINT_UNAVAILABLE")
	}
	return parent, child, nil
}

func workspacePostcheck(
	workspace Workspace,
	projection *InputProjection,
	before workspaceManifest,
) error {
	if _, err := os.Lstat(filepath.Join(workspace.Path, ".sworn-inputs")); err == nil || !os.IsNotExist(err) {
		return fail("WORKSPACE_MUTATED")
	}
	if err := projection.validate(); err != nil {
		return err
	}
	if workspace.Access == ReadOnly {
		after, err := captureWorkspaceManifest(workspace.Path)
		if err != nil {
			return err
		}
		if !equalManifest(before, after) {
			return fail("WORKSPACE_MUTATED")
		}
	}
	return nil
}
