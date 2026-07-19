//go:build linux

package executor

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	minimumBubblewrapMinor = 9
	minimumSystemdVersion  = 255
	shutdownGrace          = 3 * time.Second
)

type LinuxExecutor struct {
	options    Options
	probeMutex sync.Mutex
	probe      *ProbeReport
}

func NewLinux(options Options) (*LinuxExecutor, error) {
	if options.Limits == (Limits{}) {
		options.Limits = DefaultLimits()
	}
	if err := options.Limits.Validate(); err != nil {
		return nil, err
	}
	var err error
	if options.BubblewrapPath, err = executablePath(options.BubblewrapPath, "bwrap"); err != nil {
		return nil, err
	}
	if options.SystemdRunPath, err = executablePath(options.SystemdRunPath, "systemd-run"); err != nil {
		return nil, err
	}
	if options.SystemctlPath, err = executablePath(options.SystemctlPath, "systemctl"); err != nil {
		return nil, err
	}
	if len(options.ShimArgv) == 0 {
		self, err := os.Executable()
		if err != nil {
			return nil, fmt.Errorf("resolve executor shim: %w", err)
		}
		self, err = filepath.EvalSymlinks(self)
		if err != nil {
			return nil, fmt.Errorf("resolve executor shim symlinks: %w", err)
		}
		options.ShimArgv = []string{self, "__executor-shim"}
	}
	if err := validateShimArgv(options.ShimArgv); err != nil {
		return nil, err
	}
	if err := ensurePrivateRuntimeRoot(options.RuntimeRoot); err != nil {
		return nil, err
	}
	return &LinuxExecutor{options: options}, nil
}

func (executor *LinuxExecutor) Probe(ctx context.Context) (ProbeReport, error) {
	executor.probeMutex.Lock()
	if executor.probe != nil {
		report := cloneProbeReport(*executor.probe)
		executor.probeMutex.Unlock()
		return report, nil
	}
	executor.probeMutex.Unlock()

	bubblewrapOutput, err := runProbe(ctx, executor.options.BubblewrapPath, "--version")
	if err != nil {
		return ProbeReport{}, fmt.Errorf("probe Bubblewrap: %w", err)
	}
	bubblewrapVersion := strings.TrimSpace(string(bubblewrapOutput))
	if err := requireBubblewrapVersion(bubblewrapVersion); err != nil {
		return ProbeReport{}, err
	}
	systemdOutput, err := runProbe(ctx, executor.options.SystemdRunPath, "--version")
	if err != nil {
		return ProbeReport{}, fmt.Errorf("probe systemd: %w", err)
	}
	firstLine := strings.SplitN(strings.TrimSpace(string(systemdOutput)), "\n", 2)[0]
	if err := requireSystemdVersion(firstLine); err != nil {
		return ProbeReport{}, err
	}
	managerOutput, managerErr := runProbe(ctx, executor.options.SystemctlPath, "--user", "is-system-running")
	manager := strings.TrimSpace(string(managerOutput))
	if manager != "running" && manager != "degraded" {
		if managerErr != nil {
			return ProbeReport{}, fmt.Errorf("user systemd manager is unavailable: %w", managerErr)
		}
		return ProbeReport{}, fmt.Errorf("user systemd manager is %q", manager)
	}
	controllersBytes, err := os.ReadFile("/sys/fs/cgroup/cgroup.controllers")
	if err != nil {
		return ProbeReport{}, errors.New("cgroup v2 unified hierarchy is required")
	}
	controllers := strings.Fields(string(controllersBytes))
	for _, required := range []string{"cpu", "memory", "pids"} {
		if !containsString(controllers, required) {
			return ProbeReport{}, fmt.Errorf("cgroup v2 controller %q is unavailable", required)
		}
	}
	probeContext, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	probeArgs := executor.bubblewrapBaseArgs(NetworkNone, 1<<20, 1<<20)
	probeArgs = append(probeArgs, "--", "/usr/bin/true")
	unit := UnitName(fmt.Sprintf("probe-%d-%d", os.Getpid(), time.Now().UnixNano()))
	serviceArgs := []string{
		"--user",
		"--wait",
		"--collect",
		"--quiet",
		"--service-type=exec",
		"--expand-environment=no",
		"--unit=" + unit,
	}
	for _, property := range executor.serviceProperties(5 * time.Second) {
		serviceArgs = append(serviceArgs, "--property="+property)
	}
	serviceArgs = append(serviceArgs, "--", executor.options.BubblewrapPath)
	serviceArgs = append(serviceArgs, probeArgs...)
	command := exec.CommandContext(probeContext, executor.options.SystemdRunPath, serviceArgs...)
	command.Env = controlEnvironment()
	if output, err := command.CombinedOutput(); err != nil {
		kill := exec.Command(executor.options.SystemctlPath, "--user", "kill", "--kill-whom=all", "--signal=SIGKILL", unit)
		kill.Env = controlEnvironment()
		_ = kill.Run()
		return ProbeReport{}, fmt.Errorf("contained service probe failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	sort.Strings(controllers)
	report := ProbeReport{
		BubblewrapVersion: bubblewrapVersion,
		SystemdVersion:    firstLine,
		CgroupV2:          true,
		UserManager:       manager,
		Controllers:       controllers,
	}
	executor.probeMutex.Lock()
	if executor.probe == nil {
		cached := cloneProbeReport(report)
		executor.probe = &cached
	}
	executor.probeMutex.Unlock()
	return report, nil
}

func (executor *LinuxExecutor) RunContained(ctx context.Context, invocation Invocation) (completion RawCompletion, resultErr error) {
	if err := invocation.validate(executor.options); err != nil {
		return RawCompletion{}, err
	}
	if _, err := executor.Probe(ctx); err != nil {
		return RawCompletion{}, err
	}
	runRoot, err := os.MkdirTemp(executor.options.RuntimeRoot, "invocation-")
	if err != nil {
		return RawCompletion{}, fmt.Errorf("create executor runtime: %w", err)
	}
	defer func() {
		if err := removePrivateTree(runRoot); err != nil {
			cleanupErr := fmt.Errorf("remove executor runtime: %w", err)
			if resultErr == nil {
				resultErr = cleanupErr
			} else {
				resultErr = errors.Join(resultErr, cleanupErr)
			}
		}
	}()
	workspacePath := filepath.Join(runRoot, "workspace")
	workspaceDigest, workspaceBytes, err := stageWorkspace(
		ctx, invocation.Workspace, workspacePath, executor.options.Limits.InputBytes,
	)
	if err != nil {
		return RawCompletion{}, err
	}
	if workspaceDigest != invocation.WorkspaceDigest {
		return RawCompletion{}, fmt.Errorf(
			"workspace digest mismatch: observed %s, want %s",
			workspaceDigest, invocation.WorkspaceDigest,
		)
	}
	inputsPath := filepath.Join(runRoot, "inputs")
	if err := os.Mkdir(inputsPath, 0o700); err != nil {
		return RawCompletion{}, fmt.Errorf("create staged inputs: %w", err)
	}
	remaining := executor.options.Limits.InputBytes - workspaceBytes
	inputs := append([]Input(nil), invocation.Inputs...)
	sort.Slice(inputs, func(left, right int) bool { return inputs[left].Name < inputs[right].Name })
	boundInputs := make([]BoundInput, 0, len(inputs))
	for _, input := range inputs {
		bound, err := stageInput(ctx, input, filepath.Join(inputsPath, input.Name), remaining)
		if err != nil {
			return RawCompletion{}, err
		}
		remaining -= bound.Size
		boundInputs = append(boundInputs, bound)
	}
	bubblewrapArgv := append(
		[]string{executor.options.BubblewrapPath},
		executor.bubblewrapArgs(invocation, workspacePath, inputsPath)...,
	)
	unit := UnitName(invocation.ID)
	completion, resultErr = executor.runService(ctx, invocation, unit, bubblewrapArgv)
	completion.WorkspaceDigest = workspaceDigest
	completion.Inputs = boundInputs
	return completion, resultErr
}

func UnitName(invocationID string) string {
	digest := sha256.Sum256([]byte(invocationID))
	return "sworn-v1-" + hex.EncodeToString(digest[:12]) + ".service"
}

func (executor *LinuxExecutor) bubblewrapArgs(
	invocation Invocation,
	workspacePath, inputsPath string,
) []string {
	args := executor.bubblewrapBaseArgs(
		invocation.Network,
		executor.options.Limits.TempBytes,
		executor.options.Limits.HomeBytes,
	)
	args = append(args, "--ro-bind", workspacePath, "/workspace", "--dir", "/inputs")
	for _, input := range invocation.Inputs {
		args = append(args, "--ro-bind", filepath.Join(inputsPath, input.Name), "/inputs/"+input.Name)
	}
	for _, value := range sortedEnvironment(invocation.Environment) {
		args = append(args, "--setenv", value[0], value[1])
	}
	args = append(args, "--", invocation.Argv[0])
	args = append(args, invocation.Argv[1:]...)
	return args
}

func (executor *LinuxExecutor) bubblewrapBaseArgs(network NetworkMode, tempBytes, homeBytes uint64) []string {
	args := []string{
		"--die-with-parent",
		"--new-session",
		"--unshare-user",
		"--unshare-pid",
		"--unshare-ipc",
		"--unshare-uts",
		"--unshare-cgroup",
	}
	if network == NetworkNone {
		args = append(args, "--unshare-net")
	}
	args = append(args,
		"--disable-userns",
		"--cap-drop", "ALL",
		"--clearenv",
		"--tmpfs", "/",
		"--proc", "/proc",
		"--dev", "/dev",
		"--ro-bind", "/usr", "/usr",
		"--symlink", "usr/bin", "/bin",
		"--symlink", "usr/lib", "/lib",
		"--symlink", "usr/lib64", "/lib64",
		"--dir", "/workspace",
		"--size", strconv.FormatUint(tempBytes, 10), "--tmpfs", "/tmp",
		"--dir", "/home",
		"--size", strconv.FormatUint(homeBytes, 10), "--tmpfs", "/home/sworn",
		"--setenv", "PATH", "/usr/bin:/bin",
		"--setenv", "HOME", "/home/sworn",
		"--setenv", "TMPDIR", "/tmp",
		"--setenv", "LANG", "C",
		"--setenv", "LC_ALL", "C",
		"--setenv", "TZ", "UTC",
		"--chdir", "/workspace",
	)
	if network == NetworkHost {
		args = append(args,
			"--dir", "/etc",
			"--dir", "/etc/ssl",
			"--ro-bind-try", "/etc/resolv.conf", "/etc/resolv.conf",
			"--ro-bind-try", "/etc/hosts", "/etc/hosts",
			"--ro-bind-try", "/etc/nsswitch.conf", "/etc/nsswitch.conf",
			"--ro-bind-try", "/etc/ssl/certs", "/etc/ssl/certs",
		)
	}
	return args
}

func (executor *LinuxExecutor) runService(
	ctx context.Context,
	invocation Invocation,
	unit string,
	bubblewrapArgv []string,
) (RawCompletion, error) {
	serviceArgv := executor.systemdRunArgs(invocation, unit, bubblewrapArgv)
	command := exec.Command(executor.options.SystemdRunPath, serviceArgv...)
	command.Env = controlEnvironment()
	watchReader, watchWriter, err := os.Pipe()
	if err != nil {
		return RawCompletion{}, fmt.Errorf("create executor lifetime pipe: %w", err)
	}
	defer watchReader.Close() //nolint:errcheck
	defer watchWriter.Close() //nolint:errcheck
	overflow := make(chan struct{})
	var overflowOnce sync.Once
	onOverflow := func() { overflowOnce.Do(func() { close(overflow) }) }
	stdout := &boundedCapture{limit: executor.options.Limits.StdoutBytes, overflow: onOverflow}
	stderr := &boundedCapture{limit: executor.options.Limits.StderrBytes, overflow: onOverflow}
	command.Stdin = watchReader
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Start(); err != nil {
		return RawCompletion{}, fmt.Errorf("start transient executor service: %w", err)
	}
	_ = watchReader.Close()
	completion := RawCompletion{
		InvocationID: invocation.ID,
		Unit:         unit,
		StartedAt:    time.Now().UTC(),
		ExitCode:     -1,
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	executionContext, cancel := context.WithTimeout(ctx, invocation.Timeout)
	defer cancel()
	var runErr error
	select {
	case runErr = <-done:
	case <-executionContext.Done():
		completion.TimedOut = errors.Is(executionContext.Err(), context.DeadlineExceeded) && ctx.Err() == nil
		completion.Cancelled = !completion.TimedOut
		_ = watchWriter.Close()
		runErr = executor.waitOrKill(unit, command, done)
	case <-overflow:
		completion.OutputTruncated = true
		_ = watchWriter.Close()
		runErr = executor.waitOrKill(unit, command, done)
	}
	_ = watchWriter.Close()
	completion.CompletedAt = time.Now().UTC()
	completion.Stdout = stdout.Bytes()
	completion.Stderr = stderr.Bytes()
	completion.OutputTruncated = completion.OutputTruncated || stdout.Truncated() || stderr.Truncated()
	completion.ExitCode = processExitCode(runErr)
	if runErr != nil && completion.ExitCode == -1 {
		return completion, fmt.Errorf("wait for transient executor service: %w", runErr)
	}
	return completion, nil
}

func (executor *LinuxExecutor) waitOrKill(
	unit string,
	command *exec.Cmd,
	done <-chan error,
) error {
	timer := time.NewTimer(shutdownGrace)
	defer timer.Stop()
	select {
	case err := <-done:
		return err
	case <-timer.C:
		kill := exec.Command(
			executor.options.SystemctlPath,
			"--user", "kill", "--kill-whom=all", "--signal=SIGKILL", unit,
		)
		kill.Env = controlEnvironment()
		_ = kill.Run()
		if command.Process != nil {
			_ = command.Process.Kill()
		}
		return <-done
	}
}

func (executor *LinuxExecutor) systemdRunArgs(
	invocation Invocation,
	unit string,
	bubblewrapArgv []string,
) []string {
	runtimeLimit := invocation.Timeout + shutdownGrace
	properties := executor.serviceProperties(runtimeLimit)
	args := []string{
		"--user",
		"--pipe",
		"--wait",
		"--collect",
		"--quiet",
		"--service-type=exec",
		"--expand-environment=no",
		"--unit=" + unit,
	}
	for _, property := range properties {
		args = append(args, "--property="+property)
	}
	args = append(args, "--")
	args = append(args, executor.options.ShimArgv...)
	args = append(args, bubblewrapArgv...)
	return args
}

func (executor *LinuxExecutor) serviceProperties(runtimeLimit time.Duration) []string {
	return []string{
		"KillMode=control-group",
		"RuntimeMaxSec=" + durationValue(runtimeLimit),
		"TimeoutStopSec=" + durationValue(shutdownGrace),
		"MemoryMax=" + strconv.FormatUint(executor.options.Limits.MemoryBytes, 10),
		"MemorySwapMax=" + strconv.FormatUint(executor.options.Limits.SwapBytes, 10),
		"TasksMax=" + strconv.FormatUint(executor.options.Limits.Tasks, 10),
		"CPUQuota=" + strconv.FormatUint(executor.options.Limits.CPUPercent, 10) + "%",
		"LimitFSIZE=" + strconv.FormatUint(executor.options.Limits.FileBytes, 10),
		"LimitNOFILE=1024",
		"UMask=0077",
		"NoNewPrivileges=yes",
		"RestrictSUIDSGID=yes",
		"LockPersonality=yes",
		"KeyringMode=private",
		"OOMPolicy=kill",
		"Restart=no",
	}
}

type boundedCapture struct {
	mutex     sync.Mutex
	buffer    bytes.Buffer
	limit     int
	truncated bool
	overflow  func()
}

func (capture *boundedCapture) Write(contents []byte) (int, error) {
	capture.mutex.Lock()
	written := len(contents)
	remaining := capture.limit - capture.buffer.Len()
	if remaining > 0 {
		if remaining > len(contents) {
			remaining = len(contents)
		}
		_, _ = capture.buffer.Write(contents[:remaining])
	}
	if remaining < len(contents) {
		capture.truncated = true
	}
	truncated := capture.truncated
	capture.mutex.Unlock()
	if truncated && capture.overflow != nil {
		capture.overflow()
	}
	return written, nil
}

func (capture *boundedCapture) Bytes() []byte {
	capture.mutex.Lock()
	defer capture.mutex.Unlock()
	return append([]byte(nil), capture.buffer.Bytes()...)
}

func (capture *boundedCapture) Truncated() bool {
	capture.mutex.Lock()
	defer capture.mutex.Unlock()
	return capture.truncated
}

func executablePath(configured, fallback string) (string, error) {
	path := configured
	var err error
	if path == "" {
		path, err = exec.LookPath(fallback)
		if err != nil {
			return "", fmt.Errorf("%s executable is required", fallback)
		}
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve %s executable: %w", fallback, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("%s executable %q is unavailable", fallback, path)
	}
	return path, nil
}

func validateShimArgv(argv []string) error {
	if len(argv) == 0 || !filepath.IsAbs(argv[0]) {
		return errors.New("executor shim argv requires an absolute executable")
	}
	info, err := os.Stat(argv[0])
	if err != nil || info.IsDir() || info.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("executor shim executable %q is unavailable", argv[0])
	}
	for _, argument := range argv {
		if strings.ContainsRune(argument, '\x00') || len(argument) > 1<<20 {
			return errors.New("executor shim argv contains an invalid argument")
		}
	}
	return nil
}

func ensurePrivateRuntimeRoot(root string) error {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return errors.New("executor runtime root must be a clean absolute path")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return fmt.Errorf("create executor runtime root: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve executor runtime root: %w", err)
	}
	if filepath.Clean(resolved) != root {
		return errors.New("executor runtime root contains a symbolic-link remap")
	}
	info, err := os.Stat(root)
	if err != nil {
		return fmt.Errorf("inspect executor runtime root: %w", err)
	}
	if !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return errors.New("executor runtime root must be private")
	}
	statistics, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(statistics.Uid) != os.Geteuid() {
		return errors.New("executor runtime root must be owned by the current user")
	}
	return nil
}

func runProbe(ctx context.Context, path string, args ...string) ([]byte, error) {
	probeContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	command := exec.CommandContext(probeContext, path, args...)
	command.Env = controlEnvironment()
	combined, err := command.CombinedOutput()
	if len(combined) > 64<<10 {
		return nil, errors.New("probe output exceeded limit")
	}
	return combined, err
}

func requireBubblewrapVersion(value string) error {
	fields := strings.Fields(value)
	if len(fields) != 2 || fields[0] != "bubblewrap" {
		return fmt.Errorf("unrecognized Bubblewrap version %q", value)
	}
	parts := strings.Split(fields[1], ".")
	if len(parts) < 2 {
		return fmt.Errorf("unrecognized Bubblewrap version %q", value)
	}
	major, majorErr := strconv.Atoi(parts[0])
	minor, minorErr := strconv.Atoi(parts[1])
	if majorErr != nil || minorErr != nil || major < 0 || (major == 0 && minor < minimumBubblewrapMinor) {
		return fmt.Errorf("Bubblewrap 0.%d or newer is required, got %q", minimumBubblewrapMinor, fields[1])
	}
	return nil
}

func requireSystemdVersion(value string) error {
	fields := strings.Fields(value)
	if len(fields) < 2 || fields[0] != "systemd" {
		return fmt.Errorf("unrecognized systemd version %q", value)
	}
	version, err := strconv.Atoi(fields[1])
	if err != nil || version < minimumSystemdVersion {
		return fmt.Errorf("systemd %d or newer is required, got %q", minimumSystemdVersion, fields[1])
	}
	return nil
}

func processExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func durationValue(duration time.Duration) string {
	return strconv.FormatInt(duration.Milliseconds(), 10) + "ms"
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func controlEnvironment() []string {
	environment := []string{"LANG=C", "LC_ALL=C", "PATH=/usr/bin:/bin"}
	for _, name := range []string{"XDG_RUNTIME_DIR", "DBUS_SESSION_BUS_ADDRESS"} {
		if value := os.Getenv(name); value != "" && !strings.ContainsRune(value, '\x00') {
			environment = append(environment, name+"="+value)
		}
	}
	return environment
}

func cloneProbeReport(report ProbeReport) ProbeReport {
	report.Controllers = append([]string(nil), report.Controllers...)
	return report
}
