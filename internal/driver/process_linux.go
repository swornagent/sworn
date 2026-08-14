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

	"github.com/swornagent/sworn/internal/gitx"
)

const processTerminationGrace = 250 * time.Millisecond

const (
	// testUncontainedDispatchEnv is the sole environment signal for the
	// test-only uncontained dispatch mode. It is a request, never a route: a
	// binary that did not link the gate refuses it in platformInvoke before
	// any driver or sandbox interaction, so the environment value alone can
	// never enable the uncontained branch.
	testUncontainedDispatchEnv = "SWORN_TEST_UNCONTAINED_DISPATCH"
	// testUncontainedGuestWorkspaceEnv and testUncontainedGuestInputsEnv are
	// the engine-set guest-path overrides that let a fake driver resolve the
	// guest paths (/workspace and /sworn/inputs) it cannot otherwise see in an
	// uncontained dispatch. They exist only in the controlled environment the
	// engine builds for the gate-linked uncontained branch; the contained
	// branch's --clearenv plus fixed --setenv list never carries them.
	testUncontainedGuestWorkspaceEnv = "SWORN_TEST_GUEST_WORKSPACE"
	testUncontainedGuestInputsEnv    = "SWORN_TEST_GUEST_INPUTS"
)

// testUncontainedDispatch is the link-time test-only gate for the uncontained
// dispatch branch, mirroring the established runtime.testHooksFromEnv pattern.
// A production build links the zero value (""), which has no manifest, config,
// environment, or argument route: the uncontained branch is reached only when
// this gate is linked, the process asks for it, and the selected adapter is
// the fake driver.
var testUncontainedDispatch string

// uncontainedDispatchEnabled reports whether this binary is allowed to take
// the test-only uncontained dispatch branch. Both the linked gate and the
// environment request are required; the environment alone is refused.
func uncontainedDispatchEnabled() bool {
	return testUncontainedDispatch == "1" && os.Getenv(testUncontainedDispatchEnv) == "1"
}

// uncontainedDispatchRequested reports whether the process asked for the
// test-only uncontained dispatch mode. A binary without the gate refuses the
// request before any dispatch.
func uncontainedDispatchRequested() bool {
	return os.Getenv(testUncontainedDispatchEnv) == "1"
}

var (
	bubblewrapProbeOnce sync.Once
	bubblewrapProbeErr  error
)

func platformInvoke(
	parent context.Context,
	invocation Invocation,
	executable ExecutableIdentity,
) (Observation, error) {
	// A request for the test-only uncontained dispatch mode is refused by any
	// binary that did not link the gate. The environment signal is a request,
	// never a route: it can never enable dispatch on its own, and the refusal
	// happens before any driver or sandbox interaction.
	if uncontainedDispatchRequested() && !uncontainedDispatchEnabled() {
		return Observation{}, fail("UNCONTAINED_DISPATCH_REFUSED")
	}
	if err := validateExecutableIdentity(executable); err != nil {
		return Observation{}, err
	}
	executableFile, err := openPinnedExecutable(executable)
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
	// The uncontained branch is a test-only dispatch mode, reachable only
	// through the linked gate plus the environment request plus the fake
	// driver adapter. Every other invocation keeps real containment.
	uncontained := uncontainedDispatchEnabled() &&
		invocation.Selected.Adapter.ID == FakeDriverID
	var command *exec.Cmd
	if uncontained {
		command, err = uncontainedCommand(
			invocation,
			executable,
			projection,
			requestBody,
			childEndpoint,
		)
		if err != nil {
			return Observation{}, err
		}
	} else {
		bwrap, err := trustedBubblewrap()
		if err != nil {
			return Observation{}, fail("ISOLATION_UNAVAILABLE")
		}
		args, err := bubblewrapArguments(invocation)
		if err != nil {
			return Observation{}, err
		}
		command = exec.Command(bwrap, args...)
		command.Stdin = bytes.NewReader(requestBody)
		command.Env = []string{}
		command.ExtraFiles = []*os.File{
			childEndpoint,
			executableFile,
			workspaceFile,
			projectionFile,
			statusWriter,
		}
		command.SysProcAttr = linuxSandboxProcessAttributes()
	}
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
	if uncontained {
		// The uncontained driver is its own process-group leader (Setpgid),
		// so there is no sandbox indirection and no status-pipe handshake:
		// the engine's process group is the child pid itself.
		sandboxProcessGroup.Store(int64(command.Process.Pid))
	} else {
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
	}
	_ = statusReader.Close()
	endpointResult := make(chan error, 1)
	go func() {
		endpointResult <- serveSubmissionEndpoint(parentEndpoint, arbiter)
	}()
	waitErr := command.Wait()
	waitStatus := command.ProcessState.Sys().(syscall.WaitStatus)
	exitCode := waitStatus.ExitStatus()
	// An engine stop surfaces as either form depending on how the process was
	// launched: under bubblewrap the sandboxed child is wrapped, so the
	// observed process exits with 128+signal; on the uncontained direct-exec
	// path the driver itself is signalled, so the observed status is
	// Signaled(SIGTERM|SIGKILL). Both are engine stops, never spontaneous
	// exits.
	engineExit := (waitStatus.Exited() &&
		(exitCode == 128+int(syscall.SIGTERM) || exitCode == 128+int(syscall.SIGKILL))) ||
		(waitStatus.Signaled() &&
			(waitStatus.Signal() == syscall.SIGTERM || waitStatus.Signal() == syscall.SIGKILL))
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
		reservedMaskNames(invocation),
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
	// The containment binary resolves from the machine/user SWORN_BWRAP
	// override or the default literal. It is deliberately absent from the
	// project config schema, so a project-scoped configuration can never name
	// the containment binary (A3/A5): a project file carrying it is refused
	// at parse time. The trust requirements are unchanged: absolute, regular,
	// executable, no group/world write bits, owned by uid 0, and the
	// capability probe.
	executable := os.Getenv(gitx.EnvBubblewrap)
	if executable == "" {
		executable = "/usr/bin/bwrap"
	}
	if !filepath.IsAbs(executable) {
		return "", fail("ISOLATION_UNAVAILABLE")
	}
	resolved, err := filepath.EvalSymlinks(executable)
	if err != nil {
		return "", fail("ISOLATION_UNAVAILABLE")
	}
	info, err := os.Lstat(resolved)
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
		command := exec.CommandContext(ctx, resolved, "--help")
		command.Env = []string{}
		command.SysProcAttr = linuxSandboxProcessAttributes()
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
			"--ro-bind-data",
			"--perms",
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
	return resolved, nil
}

// reservedMaskNames returns the workspace-relative names the containment
// mask protects: the engine-computed MaskNames when present (which follow the
// configured project roots), else the fixed defaults.
func reservedMaskNames(invocation Invocation) []string {
	if len(invocation.MaskNames) != 0 {
		return append([]string(nil), invocation.MaskNames...)
	}
	return gitx.ReservedNames(gitx.DefaultProjectConfig())
}

// withoutGit returns the reserved set with ".git" removed, for read-only
// verifier workspaces that expose read-only git instead of masking it.
func withoutGit(names []string) []string {
	result := make([]string, 0, len(names))
	for _, name := range names {
		if name != ".git" {
			result = append(result, name)
		}
	}
	return result
}

func linuxSandboxProcessAttributes() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setpgid:   true,
		Pdeathsig: syscall.SIGKILL,
	}
}

func bubblewrapArguments(invocation Invocation) ([]string, error) {
	if err := validateNetworkPolicy(
		invocation.Selected.Adapter.ID,
		invocation.Selected.Profile.Network,
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
	if invocation.Selected.Profile.Network == NetworkRequired {
		arguments = append(arguments, "--share-net")
	}
	if invocation.Request.Workspace.Access == ReadOnly {
		arguments = append(arguments, "--ro-bind-fd", "5", workspace)
	} else {
		arguments = append(arguments, "--bind-fd", "5", workspace)
	}
	for _, reserved := range reservedMaskNames(invocation) {
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
	if invocation.Selected.Adapter.ID == FakeDriverID {
		arguments = append(arguments,
			"--setenv", "BATON_FAKE_PROFILE", string(invocation.FakeProfile),
		)
	}
	arguments = append(arguments,
		"/sworn/driver", "run",
	)
	return arguments, nil
}

// uncontainedCommand builds the direct-exec command for the test-only
// uncontained dispatch mode. The driver runs outside any sandbox as its own
// process-group leader, with a fully controlled environment that carries the
// submission protocol, the fake profile, the uncontained marker, and the
// engine-set guest-path overrides that let the fake driver resolve the guest
// paths it could not otherwise see. The contained branch's --clearenv plus
// fixed --setenv list never acquires these variables, so the invariant that
// the parent environment never reaches the child is preserved.
func uncontainedCommand(
	invocation Invocation,
	executable ExecutableIdentity,
	projection *InputProjection,
	requestBody []byte,
	childEndpoint *os.File,
) (*exec.Cmd, error) {
	if invocation.Selected.Adapter.ID != FakeDriverID {
		return nil, fail("UNCONTAINED_DISPATCH_REFUSED")
	}
	command := exec.Command(executable.Path, "run")
	command.Dir = invocation.HostWorkspace
	command.Stdin = bytes.NewReader(requestBody)
	command.Env = []string{
		"HOME=/home/sworn",
		"TMPDIR=/tmp",
		"LANG=C.UTF-8",
		"LC_ALL=C.UTF-8",
		"TZ=UTC",
		"PATH=/usr/bin:/bin",
		"PWD=" + invocation.HostWorkspace,
		SubmissionProtocolEnvironment + "=" + SubmissionControlVersion,
		SubmissionFDEnvironment + "=3",
		"BATON_FAKE_PROFILE=" + string(invocation.FakeProfile),
		testUncontainedDispatchEnv + "=1",
		testUncontainedGuestWorkspaceEnv + "=" + invocation.HostWorkspace,
		testUncontainedGuestInputsEnv + "=" + projection.Root(),
	}
	command.ExtraFiles = []*os.File{childEndpoint}
	command.SysProcAttr = linuxSandboxProcessAttributes()
	return command, nil
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
	reserved []string,
) error {
	pathInfo, err := os.Lstat(hostWorkspace)
	if err != nil {
		return fail("WORKSPACE_INSPECTION_FAILED")
	}
	pinnedInfo, err := pinned.Stat()
	if err != nil || !os.SameFile(pathInfo, pinnedInfo) ||
		validateWorkspaceBoundary(hostWorkspace, reserved) != nil {
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
