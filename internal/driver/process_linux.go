//go:build linux

package driver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
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
		invocation.HostWorkspace,
		invocation.Request.Inputs,
		invocation.Inputs,
	)
	if err != nil {
		return Observation{}, err
	}
	defer projection.Close()
	workspaceFile, err := openPinnedDirectory(invocation.HostWorkspace)
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
		before, err = captureWorkspaceManifest(invocation.HostWorkspace)
		if err != nil {
			return Observation{}, err
		}
	}
	requestBody, err := EncodeRequest(invocation.Request)
	if err != nil {
		return Observation{}, err
	}
	server, err := newSubmissionServer(invocation.Permission)
	if err != nil {
		return Observation{}, err
	}
	parentEndpoint, childEndpoint, err := socketPair()
	if err != nil {
		return Observation{}, err
	}
	defer parentEndpoint.Close()
	defer childEndpoint.Close()
	statusReader, statusWriter, err := os.Pipe()
	if err != nil {
		return Observation{}, fail("PROCESS_START_FAILED")
	}
	defer statusReader.Close()
	defer statusWriter.Close()
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
		statusWriter,
	}
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var done = make(chan struct{})
	terminationDone := make(chan struct{})
	var terminateOnce sync.Once
	var sandboxProcessGroup atomic.Int64
	engineSignalled := false
	terminate := func() bool {
		terminateOnce.Do(func() {
			_ = parentEndpoint.Close()
			if command.Process == nil {
				close(terminationDone)
				return
			}
			target := sandboxProcessGroup.Load()
			if target == 0 {
				target = int64(command.Process.Pid)
			}
			engineSignalled = syscall.Kill(-int(target), syscall.SIGTERM) == nil
			go func() {
				defer close(terminationDone)
				timer := time.NewTimer(processTerminationGrace)
				defer timer.Stop()
				select {
				case <-done:
				case <-timer.C:
					_ = syscall.Kill(-int(target), syscall.SIGKILL)
				}
			}()
		})
		return engineSignalled
	}
	arbiter := newTerminalArbiter(invocation, server)
	arbiter.stop = terminate
	stderr := &boundedBuffer{
		maximum: MaxStderrBytes,
		retain:  MaxStderrRetain,
		onOverflow: func() {
			arbiter.fail("stderr_overflow", fail("OUTPUT_OVERFLOW"), fatalOverflow)
		},
	}
	command.Stdout = arbiter
	command.Stderr = stderr
	ctx, cancel := invocationContext(parent, invocation.Request.Limits.TimeoutMillis)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return contextFailure(err)
	}
	if err := command.Start(); err != nil {
		return Observation{}, fail("PROCESS_START_FAILED")
	}
	_ = childEndpoint.Close()
	_ = statusWriter.Close()
	watcherStop := make(chan struct{})
	watcherDone := make(chan struct{})
	go func() {
		defer close(watcherDone)
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				arbiter.cancel("invocation_timeout", fail("INVOCATION_TIMEOUT"), fatalTimeout)
			} else {
				arbiter.cancel("invocation_cancelled", fail("INVOCATION_CANCELLED"), fatalCancellation)
			}
		case <-watcherStop:
		}
	}()
	var status struct {
		ChildPID int `json:"child-pid"`
	}
	decodeErr := json.NewDecoder(statusReader).Decode(&status)
	group, groupErr := syscall.Getpgid(status.ChildPID)
	deadline := time.Now().Add(processTerminationGrace)
	for groupErr == nil && group == command.Process.Pid && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
		group, groupErr = syscall.Getpgid(status.ChildPID)
	}
	if decodeErr != nil || groupErr != nil || status.ChildPID <= 0 ||
		group <= 0 || group == command.Process.Pid {
		arbiter.fail("process_status_failed", fail("PROCESS_START_FAILED"), fatalTransport)
	} else {
		sandboxProcessGroup.Store(int64(group))
	}
	_ = statusReader.Close()
	endpointResult := make(chan error, 1)
	go func() {
		endpointResult <- serveSubmissionEndpoint(parentEndpoint, arbiter)
	}()
	waitErr := command.Wait()
	waitStatus := command.ProcessState.Sys().(syscall.WaitStatus)
	exitCode := waitStatus.ExitStatus()
	engineExit := waitStatus.Exited() &&
		(exitCode == 128+int(syscall.SIGTERM) || exitCode == 128+int(syscall.SIGKILL))
	close(done)
	arbiter.processDone(waitErr, engineExit)
	terminate()
	<-terminationDone
	_ = parentEndpoint.Close()
	<-endpointResult
	if err := waitProcessGroup(command.Process.Pid); err != nil {
		arbiter.fail("process_not_quiescent", err, fatalTransport)
	} else {
		arbiter.mark("process_group_quiescent")
	}
	if err := workspacePostcheck(
		invocation.HostWorkspace,
		invocation.Request.Workspace.Access,
		workspaceFile,
		projection,
		before,
	); err != nil {
		arbiter.fail("workspace_postcheck_failed", err, fatalPostcheck)
	} else {
		arbiter.mark("workspace_postcheck")
	}
	if err := projection.Close(); err != nil {
		arbiter.fail("input_cleanup_failed", err, fatalPostcheck)
	} else {
		arbiter.mark("input_projection_removed")
	}
	arbiter.mark("producers_joined")
	arbiter.publish(ctx.Err())
	close(watcherStop)
	<-watcherDone
	observation, resultErr := arbiter.observation()
	_, stderrBytes, stderrOverflow := stderr.snapshot()
	observation.Diagnostic.StderrBytes = stderrBytes
	observation.Diagnostic.Truncated = stderrOverflow || stderrBytes > MaxStderrRetain
	return observation, resultErr
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
			"--info-fd",
			"--new-session",
			"--remount-ro",
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
	workspace := GuestWorkspacePath
	arguments := []string{
		"--die-with-parent",
		"--new-session",
		"--info-fd", "7",
		"--unshare-all",
		"--unshare-user",
		"--disable-userns",
		"--cap-drop", "ALL",
		"--clearenv",
		"--tmpfs", "/proc", "--remount-ro", "/proc",
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
	for _, reserved := range []string{".git", ".baton", ".sworn"} {
		info, err := os.Lstat(filepath.Join(invocation.HostWorkspace, reserved))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return nil, fail("UNSAFE_WORKSPACE_SURFACE")
		}
		target := filepath.Join(workspace, reserved)
		if info.IsDir() {
			arguments = append(arguments, "--tmpfs", target, "--remount-ro", target)
		} else if info.Mode().IsRegular() {
			arguments = append(arguments, "--ro-bind", "/dev/null", target)
		} else {
			return nil, fail("UNSAFE_WORKSPACE_SURFACE")
		}
	}
	arguments = append(arguments,
		"--ro-bind-fd", "6", GuestInputPath,
		"--ro-bind-fd", "4", "/sworn/driver",
		"--chdir", workspace,
		"--setenv", "HOME", "/home/sworn",
		"--setenv", "TMPDIR", "/tmp",
		"--setenv", "LANG", "C.UTF-8",
		"--setenv", "LC_ALL", "C.UTF-8",
		"--setenv", "TZ", "UTC",
		"--setenv", "PATH", "/usr/bin:/bin",
		"--setenv", "PWD", workspace,
		"--setenv", SubmissionProtocolEnvironment, SubmissionControlVersion,
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
func socketPair() (*os.File, *os.File, error) {
	files, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		return nil, nil, fail("ENDPOINT_UNAVAILABLE")
	}
	syscall.CloseOnExec(files[0])
	syscall.CloseOnExec(files[1])
	parent := os.NewFile(uintptr(files[0]), "sworn-submission-parent")
	child := os.NewFile(uintptr(files[1]), "sworn-submission-child")
	return parent, child, nil
}
func waitProcessGroup(pid int) error {
	deadline := time.Now().Add(2 * processTerminationGrace)
	for {
		err := syscall.Kill(-pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		if err != nil && !errors.Is(err, syscall.EPERM) {
			return fail("PROCESS_TREE_NOT_QUIESCENT")
		}
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		if time.Now().After(deadline) {
			return fail("PROCESS_TREE_NOT_QUIESCENT")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
func workspacePostcheck(
	hostWorkspace string,
	access WorkspaceAccess,
	pinned *os.File,
	projection *InputProjection,
	before workspaceManifest,
) error {
	pathInfo, err := os.Lstat(hostWorkspace)
	if err != nil {
		return fail("WORKSPACE_INSPECTION_FAILED")
	}
	pinnedInfo, err := pinned.Stat()
	if err != nil || !os.SameFile(pathInfo, pinnedInfo) ||
		validateWorkspaceBoundary(hostWorkspace) != nil {
		return fail("WORKSPACE_IDENTITY_CHANGED")
	}
	if err = projection.validate(); err != nil {
		return err
	}
	if access == ReadOnly {
		after, err := captureWorkspaceManifest(hostWorkspace)
		if err != nil {
			return err
		}
		if !equalManifest(before, after) {
			return fail("WORKSPACE_MUTATED")
		}
	}
	return nil
}
