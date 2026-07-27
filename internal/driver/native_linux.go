//go:build linux

package driver

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

const nativeOutputSchemaJSON = `{"type":"object","additionalProperties":true}`

type nativeEventState struct {
	mu         sync.Mutex
	family     ProfileFamily
	model      string
	access     WorkspaceAccess
	broker     *nativeBroker
	nativeSeen bool
	usage      Usage
	hasUsage   bool
	err        error
}

type nativeCaptureRun struct {
	provider    *nativeProviderCapture
	certificate nativeSurfaceCertificate
}

func platformInvokeNative(
	parent context.Context,
	invocation Invocation,
	config NativeAdapterConfig,
	credentialPath string,
	certificate nativeSurfaceCertificate,
) (Observation, error) {
	if validateNativeSurfaceCertificate(
		certificate,
		invocation,
		config,
	) != nil {
		return Observation{}, fail("NATIVE_NOT_CERTIFIED")
	}
	return platformRunNative(
		parent,
		invocation,
		config,
		credentialPath,
		nil,
		&certificate,
	)
}

func platformCaptureNativeSurface(
	parent context.Context,
	invocation Invocation,
	config NativeAdapterConfig,
) (nativeSurfaceCertificate, error) {
	if parent == nil || parent.Err() != nil ||
		validateInvocation(invocation) != nil ||
		invocation.Request.Workspace.Access != ReadWrite {
		return nativeSurfaceCertificate{}, fail("NATIVE_NOT_CERTIFIED")
	}
	provider, err := newNativeProviderCapture(
		config.Family,
		invocation.Selected.Model,
		invocation.Request.Workspace.Access,
	)
	if err != nil {
		return nativeSurfaceCertificate{}, err
	}
	defer provider.Close()
	credentialBody, err := nativeCaptureCredentialBody(provider)
	if err != nil {
		return nativeSurfaceCertificate{}, err
	}
	credentialDirectory, err := os.MkdirTemp("", "sworn-native-capture-")
	if err != nil {
		clearBytes(credentialBody)
		return nativeSurfaceCertificate{}, fail("NATIVE_NOT_CERTIFIED")
	}
	defer os.RemoveAll(credentialDirectory)
	credentialPath := filepath.Join(credentialDirectory, "credential")
	if err := os.WriteFile(credentialPath, credentialBody, 0o600); err != nil {
		clearBytes(credentialBody)
		return nativeSurfaceCertificate{}, fail("NATIVE_NOT_CERTIFIED")
	}
	clearBytes(credentialBody)
	run := &nativeCaptureRun{provider: provider}
	if _, err := platformRunNative(
		parent,
		invocation,
		config,
		credentialPath,
		run,
		nil,
	); err != nil {
		return nativeSurfaceCertificate{}, err
	}
	if validateNativeSurfaceCertificate(
		run.certificate,
		invocation,
		config,
	) != nil {
		return nativeSurfaceCertificate{}, fail("NATIVE_NOT_CERTIFIED")
	}
	return run.certificate, nil
}

func platformRunNative(
	parent context.Context,
	invocation Invocation,
	config NativeAdapterConfig,
	credentialPath string,
	captureRun *nativeCaptureRun,
	certificate *nativeSurfaceCertificate,
) (observation Observation, resultErr error) {
	started := time.Now()
	if (captureRun == nil) == (certificate == nil) {
		return Observation{}, fail("NATIVE_NOT_CERTIFIED")
	}
	session, err := newToolSession(invocation)
	if err != nil {
		return Observation{}, err
	}
	defer func() {
		if closeErr := session.Close(); closeErr != nil {
			observation.Handoff = nil
			resultErr = joinErrors(resultErr, closeErr)
		}
	}()
	var broker *nativeBroker
	if certificate != nil {
		broker, err = newNativeBroker(session, nativeHandshakeEvidence{
			Protocol:           certificate.Protocol,
			ClientName:         certificate.ClientName,
			ClientVersion:      certificate.ClientVersion,
			InitializeDigest:   certificate.InitializeDigest,
			NotificationDigest: certificate.NotificationDigest,
			ListDigest:         certificate.ListDigest,
			ToolDigest: nativeToolSurfaceDigest(
				invocation.Request.Workspace.Access,
			),
		})
	} else {
		broker, err = newNativeBroker(session)
	}
	if err != nil {
		return Observation{}, err
	}
	defer broker.Close()
	capability := broker.capability()
	defer clearBytes(capability)
	if captureRun != nil {
		captureRun.provider.bindBrokerCapability(capability)
	}
	credential, err := acquireFileCredential(
		credentialPath,
		invocation.HostWorkspace,
		config.MaxCredentialBytes,
	)
	if err != nil {
		return Observation{}, err
	}
	credentialClosed := false
	defer func() {
		if !credentialClosed {
			resultErr = joinErrors(resultErr, credential.Close())
		}
	}()
	closure, err := openNativeClosure(config)
	if err != nil {
		return Observation{}, err
	}
	defer closeNativeFiles(closure)
	configFiles, err := nativeConfigFiles(
		config,
		invocation,
		broker.URL(),
		capability,
		captureRun,
	)
	if err != nil {
		return Observation{}, err
	}
	defer closeNativeFiles(configFiles)
	bwrap, err := trustedBubblewrap()
	if err != nil {
		return Observation{}, err
	}
	arguments, environment, extraFiles, err := nativeCommand(
		config,
		invocation,
		credential,
		closure,
		configFiles,
		captureRun,
	)
	if err != nil {
		return Observation{}, err
	}
	defer clearEnvironment(environment)
	statusReader, statusWriter, err := os.Pipe()
	if err != nil {
		return Observation{}, fail("PROCESS_START_FAILED")
	}
	defer statusReader.Close()
	defer statusWriter.Close()
	statusFD := 3 + len(extraFiles)
	arguments = append(
		[]string{"--info-fd", itoa(statusFD)},
		arguments...,
	)
	extraFiles = append(extraFiles, statusWriter)
	ctx, cancel := invocationContext(parent, invocation.Request.Limits.TimeoutMillis)
	defer cancel()
	prompt, err := modelPrompt(invocation)
	if err != nil {
		return Observation{}, err
	}
	defer clearBytes(prompt)
	command := exec.CommandContext(ctx, bwrap, arguments...)
	command.Stdin = bytes.NewReader(prompt)
	command.Env = byteEnvironmentStrings(environment)
	command.ExtraFiles = extraFiles
	command.SysProcAttr = linuxSandboxProcessAttributes()
	command.WaitDelay = processTerminationGrace
	stdout, err := command.StdoutPipe()
	if err != nil {
		return Observation{}, fail("PROCESS_START_FAILED")
	}
	stderr := newSecretGuard(capability, MaxStderrBytes)
	var captureStderr *secretGuard
	stderrWriter := io.Writer(stderr)
	var captureToken []byte
	if captureRun != nil {
		captureToken = captureRun.provider.bearer()
		defer clearBytes(captureToken)
		captureStderr = newSecretGuard(captureToken, MaxStderrBytes)
		stderrWriter = io.MultiWriter(stderr, captureStderr)
	}
	stderrFinalized := false
	defer func() {
		if !stderrFinalized {
			_ = stderr.leaked()
			if captureStderr != nil {
				_ = captureStderr.leaked()
			}
		}
	}()
	command.Stderr = stderrWriter
	state := &nativeEventState{
		family: config.Family, model: invocation.Selected.Model,
		access: invocation.Request.Workspace.Access, broker: broker,
	}
	if err := command.Start(); err != nil {
		return Observation{}, fail("PROCESS_START_FAILED")
	}
	_ = statusWriter.Close()
	childPID, processGroup, statusErr := readSandboxProcessGroup(
		statusReader,
		command.Process.Pid,
	)
	_ = statusReader.Close()
	if statusErr != nil {
		_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		_ = command.Wait()
		return Observation{}, fail("NATIVE_SURFACE_INVALID")
	}
	if runtimeErr := certifyNativeRuntime(
		childPID,
		invocation,
		config,
		credential,
		closure,
		session.projection.Root(),
		capability,
		captureRun,
	); runtimeErr != nil {
		_ = syscall.Kill(-processGroup, syscall.SIGKILL)
		_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		_ = command.Wait()
		_ = waitProcessGroup(processGroup)
		return Observation{}, fail("NATIVE_SURFACE_INVALID")
	}
	scanDone := make(chan error, 1)
	go func() {
		scanDone <- scanNativeEvents(
			stdout,
			capability,
			state,
			captureToken,
		)
	}()
	waitDone := make(chan error, 1)
	go func() { waitDone <- command.Wait() }()
	var waitErr error
	var scanErr error
	scanReceived := false
	var providerEvidence nativeProviderEvidence
	captureReceived := false
	var captureChannel <-chan nativeProviderEvidence
	if captureRun != nil {
		captureChannel = captureRun.provider.Captured()
	}
	select {
	case providerEvidence = <-captureChannel:
		captureReceived = true
		if !waitNativeCaptureGate(ctx, state, broker) {
			_ = syscall.Kill(-processGroup, syscall.SIGKILL)
		} else {
			_ = syscall.Kill(-processGroup, syscall.SIGTERM)
		}
		select {
		case waitErr = <-waitDone:
		case <-time.After(processTerminationGrace):
			_ = syscall.Kill(-processGroup, syscall.SIGKILL)
			waitErr = <-waitDone
		}
	case <-broker.Terminal():
		_ = syscall.Kill(-processGroup, syscall.SIGTERM)
		select {
		case waitErr = <-waitDone:
		case <-time.After(processTerminationGrace):
			_ = syscall.Kill(-processGroup, syscall.SIGKILL)
			waitErr = <-waitDone
		}
	case waitErr = <-waitDone:
	case scanErr = <-scanDone:
		scanReceived = true
		_ = syscall.Kill(-processGroup, syscall.SIGTERM)
		select {
		case waitErr = <-waitDone:
		case <-time.After(processTerminationGrace):
			_ = syscall.Kill(-processGroup, syscall.SIGKILL)
			waitErr = <-waitDone
		}
	case <-ctx.Done():
		broker.Cancel()
		_ = syscall.Kill(-processGroup, syscall.SIGTERM)
		select {
		case waitErr = <-waitDone:
		case <-time.After(processTerminationGrace):
			_ = syscall.Kill(-processGroup, syscall.SIGKILL)
			waitErr = <-waitDone
		}
	}
	if !scanReceived {
		scanErr = <-scanDone
	}
	if groupErr := waitProcessGroup(processGroup); groupErr != nil {
		return Observation{}, groupErr
	}
	if err := credential.Close(); err != nil {
		credentialClosed = true
		return Observation{}, err
	}
	credentialClosed = true
	if ctx.Err() != nil {
		return Observation{}, ctx.Err()
	}
	stderrLeak := stderr.leaked()
	if captureStderr != nil {
		stderrLeak = captureStderr.leaked() || stderrLeak
	}
	stderrFinalized = true
	if scanErr != nil || stderrLeak {
		return Observation{}, fail("NATIVE_SURFACE_INVALID")
	}
	state.mu.Lock()
	eventErr := state.err
	nativeSeen := state.nativeSeen
	usageValue := state.usage
	hasUsage := state.hasUsage
	state.mu.Unlock()
	if eventErr != nil || !nativeSeen || !broker.Ready() {
		return Observation{}, fail("NATIVE_SURFACE_INVALID")
	}
	if captureRun != nil {
		if !captureReceived ||
			providerEvidence.ToolDigest != nativeToolSurfaceDigest(
				invocation.Request.Workspace.Access,
			) {
			return Observation{}, fail("NATIVE_SURFACE_INVALID")
		}
		handshake, err := broker.HandshakeEvidence()
		if err != nil {
			return Observation{}, fail("NATIVE_SURFACE_INVALID")
		}
		configDigest, err := nativeConfigSurfaceDigest(configFiles)
		if err != nil {
			return Observation{}, fail("NATIVE_SURFACE_INVALID")
		}
		brokerIdentity := append([]byte(broker.URL()), 0)
		brokerIdentity = append(brokerIdentity, capability...)
		evidenceBody, err := canonicalJSON(struct {
			ProviderRequest  string
			ProviderEndpoint string
			BrokerIdentity   string
			ConfigSurface    string
		}{
			ProviderRequest:  providerEvidence.RequestDigest,
			ProviderEndpoint: captureRun.provider.endpointDigest(),
			BrokerIdentity:   Digest(brokerIdentity),
			ConfigSurface:    configDigest,
		})
		if err != nil {
			clearBytes(brokerIdentity)
			return Observation{}, fail("NATIVE_SURFACE_INVALID")
		}
		captureRun.certificate = nativeSurfaceCertificate{
			Family:                config.Family,
			ProfileDigest:         nativeProfileDigest(invocation.Selected.Profile),
			Model:                 invocation.Selected.Model,
			AdapterConfigDigest:   invocation.Selected.Adapter.ConfigurationDigest,
			ExecutableDigest:      config.CLI.Digest,
			CLIVersion:            config.CLIVersion,
			ToolDigest:            providerEvidence.ToolDigest,
			CaptureEvidenceDigest: Digest(evidenceBody),
			Protocol:              handshake.Protocol,
			ClientName:            handshake.ClientName,
			ClientVersion:         handshake.ClientVersion,
			InitializeDigest:      handshake.InitializeDigest,
			NotificationDigest:    handshake.NotificationDigest,
			ListDigest:            handshake.ListDigest,
		}
		clearBytes(evidenceBody)
		clearBytes(brokerIdentity)
		return Observation{}, nil
	}
	submitted, submitErr := session.submitted()
	if submitErr != nil {
		return Observation{}, submitErr
	}
	if !submitted {
		if waitErr != nil {
			return Observation{}, fail("PROVIDER_TRANSPORT_FAILED")
		}
		return Observation{}, fail("MISSING_SUBMISSION")
	}
	if closeErr := session.Close(); closeErr != nil {
		return Observation{}, closeErr
	}
	usage, err := NormalizeUsage(nil, nil)
	if hasUsage {
		usage, err = NormalizeUsage(&usageValue, nil)
	}
	if err != nil {
		return Observation{}, err
	}
	return completedToolObservation(started, usage, session.handoff()), nil
}

func waitNativeCaptureGate(
	ctx context.Context,
	state *nativeEventState,
	broker *nativeBroker,
) bool {
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		state.mu.Lock()
		seen := state.nativeSeen && state.err == nil
		state.mu.Unlock()
		if seen && broker.Ready() {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-timer.C:
			return false
		case <-ticker.C:
		}
	}
}

func nativeConfigSurfaceDigest(files []*os.File) (string, error) {
	if len(files) != 3 {
		return "", fail("NATIVE_SURFACE_INVALID")
	}
	var surface []byte
	defer clearBytes(surface)
	for _, file := range files {
		if _, err := file.Seek(0, 0); err != nil {
			return "", fail("NATIVE_SURFACE_INVALID")
		}
		body, err := io.ReadAll(io.LimitReader(file, 65_537))
		if err != nil || len(body) > 65_536 {
			clearBytes(body)
			return "", fail("NATIVE_SURFACE_INVALID")
		}
		surface = append(surface, body...)
		surface = append(surface, 0)
		clearBytes(body)
		if _, err := file.Seek(0, 0); err != nil {
			return "", fail("NATIVE_SURFACE_INVALID")
		}
	}
	return Digest(surface), nil
}

func nativeVersion(ctx context.Context, config NativeAdapterConfig) ([]byte, error) {
	closure, err := openNativeClosure(config)
	if err != nil {
		return nil, err
	}
	defer closeNativeFiles(closure)
	command := exec.CommandContext(ctx, "/proc/self/fd/3", "--version")
	command.ExtraFiles = []*os.File{closure[0]}
	command.Env = []string{"HOME=/tmp", "LANG=C", "LC_ALL=C", "TZ=UTC"}
	command.SysProcAttr = linuxSandboxProcessAttributes()
	var stdout, stderr boundedBuffer
	stdout.maximum, stdout.retain = 4_096, 4_096
	stderr.maximum, stderr.retain = 4_096, 4_096
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return nil, fail("NATIVE_NOT_CERTIFIED")
	}
	body, _, overflow := stdout.snapshot()
	stderrBody, _, stderrOverflow := stderr.snapshot()
	clearBytes(stderrBody)
	if overflow || stderrOverflow {
		clearBytes(body)
		return nil, fail("NATIVE_NOT_CERTIFIED")
	}
	return body, nil
}

func certifyNativeRuntime(
	pid int,
	invocation Invocation,
	config NativeAdapterConfig,
	credential nativeCredentialLease,
	closure []*os.File,
	projectionRoot string,
	capability []byte,
	captureRun *nativeCaptureRun,
) error {
	if pid <= 0 || credential == nil || credential.File() == nil ||
		len(closure) != len(config.RuntimeFiles)+1 ||
		len(capability) == 0 {
		return fail("NATIVE_SURFACE_INVALID")
	}
	var captureToken []byte
	if captureRun != nil {
		captureToken = captureRun.provider.bearer()
		defer clearBytes(captureToken)
	}
	procRoot := "/proc/" + itoa(pid)
	hostNetwork, hostErr := os.Readlink("/proc/self/ns/net")
	childNetwork, childErr := os.Readlink(procRoot + "/ns/net")
	hostMount, hostMountErr := os.Readlink("/proc/self/ns/mnt")
	childMount, childMountErr := os.Readlink(procRoot + "/ns/mnt")
	if hostErr != nil || childErr != nil || hostNetwork != childNetwork ||
		hostMountErr != nil || childMountErr != nil || hostMount == childMount {
		return fail("NATIVE_SURFACE_INVALID")
	}
	cmdline, err := readBoundedProcFile(procRoot+"/cmdline", 262_144)
	if err != nil || bytes.Contains(cmdline, capability) ||
		containsCapability(cmdline, captureToken) ||
		bytes.Contains(cmdline, []byte(invocation.HostWorkspace)) ||
		bytes.Contains(cmdline, []byte(projectionRoot)) {
		clearBytes(cmdline)
		return fail("NATIVE_SURFACE_INVALID")
	}
	clearBytes(cmdline)
	environment, err := readBoundedProcFile(procRoot+"/environ", 262_144)
	if err != nil || validateNativeProcessEnvironment(
		environment,
		config.Family,
		capability,
		captureRun,
	) != nil ||
		containsCapability(environment, captureToken) ||
		bytes.Contains(environment, []byte(invocation.HostWorkspace)) ||
		bytes.Contains(environment, []byte(projectionRoot)) {
		clearBytes(environment)
		return fail("NATIVE_SURFACE_INVALID")
	}
	clearBytes(environment)
	mountInfo, err := readBoundedProcFile(procRoot+"/mountinfo", 1_048_576)
	if err != nil ||
		bytes.Contains(mountInfo, []byte(invocation.HostWorkspace)) ||
		bytes.Contains(mountInfo, []byte(projectionRoot)) {
		clearBytes(mountInfo)
		return fail("NATIVE_SURFACE_INVALID")
	}
	clearBytes(mountInfo)
	root := procRoot + "/root"
	workspaceEntries, err := os.ReadDir(root + GuestWorkspacePath)
	if err != nil || len(workspaceEntries) != 0 {
		return fail("NATIVE_SURFACE_INVALID")
	}
	if _, err := os.Stat(root + GuestInputPath); !os.IsNotExist(err) {
		return fail("NATIVE_SURFACE_INVALID")
	}
	if _, err := os.Stat(root + "/usr/bin/sh"); !os.IsNotExist(err) {
		return fail("NATIVE_SURFACE_INVALID")
	}
	var configBodies [][]byte
	defer func() {
		for _, body := range configBodies {
			clearBytes(body)
		}
	}()
	for _, configPath := range []string{
		nativeMCPConfigTarget(config.Family),
		"/sworn/config/output-schema.json",
		"/sworn/config/models.json",
	} {
		info, statErr := os.Stat(root + configPath)
		body, readErr := readBoundedProcFile(root+configPath, 65_536)
		if statErr != nil || readErr != nil || !info.Mode().IsRegular() ||
			info.Mode().Perm() != 0o600 {
			clearBytes(body)
			return fail("NATIVE_SURFACE_INVALID")
		}
		configBodies = append(configBodies, body)
	}
	configSurface := bytes.Join(configBodies, []byte{0})
	defer clearBytes(configSurface)
	switch config.Family {
	case ProfileCodex:
		if bytes.Count(configSurface, capability) != 1 ||
			(captureRun != nil &&
				bytes.Count(configSurface, captureToken) != 1) {
			return fail("NATIVE_SURFACE_INVALID")
		}
	case ProfileClaude:
		if bytes.Count(configSurface, capability) != 1 ||
			containsCapability(configSurface, captureToken) {
			return fail("NATIVE_SURFACE_INVALID")
		}
	default:
		return fail("NATIVE_SURFACE_INVALID")
	}
	if !sameOpenFile(root+"/sworn/bin/agent", closure[0]) ||
		!sameOpenFile(root+config.CredentialTarget, credential.File()) {
		return fail("NATIVE_SURFACE_INVALID")
	}
	for index, runtimeFile := range config.RuntimeFiles {
		if !sameOpenFile(root+runtimeFile.Target, closure[index+1]) {
			return fail("NATIVE_SURFACE_INVALID")
		}
	}
	descriptors, err := os.ReadDir(procRoot + "/fd")
	if err != nil || len(descriptors) > 256 {
		return fail("NATIVE_SURFACE_INVALID")
	}
	for _, descriptor := range descriptors {
		target, linkErr := os.Readlink(procRoot + "/fd/" + descriptor.Name())
		if linkErr != nil {
			continue
		}
		if strings.Contains(target, invocation.HostWorkspace) ||
			strings.Contains(target, projectionRoot) ||
			bytes.Contains([]byte(target), capability) {
			return fail("NATIVE_SURFACE_INVALID")
		}
	}
	return nil
}

func readBoundedProcFile(name string, maximum int64) ([]byte, error) {
	file, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil || int64(len(body)) > maximum {
		clearBytes(body)
		return nil, fail("NATIVE_SURFACE_INVALID")
	}
	return body, nil
}

func validateNativeProcessEnvironment(
	body []byte,
	family ProfileFamily,
	capability []byte,
	captureRun *nativeCaptureRun,
) error {
	expected := map[string][]byte{
		"HOME":   []byte("/home/sworn"),
		"TMPDIR": []byte("/tmp"),
		"LANG":   []byte("C.UTF-8"),
		"LC_ALL": []byte("C.UTF-8"),
		"TZ":     []byte("UTC"),
	}
	switch family {
	case ProfileClaude:
		expected["CLAUDE_CODE_DISABLE_AUTO_MEMORY"] = []byte("1")
		expected["CLAUDE_CODE_DISABLE_FEEDBACK_SURVEY"] = []byte("1")
		expected["CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC"] = []byte("1")
		expected["DISABLE_AUTOUPDATER"] = []byte("1")
		expected["DISABLE_TELEMETRY"] = []byte("1")
		expected["DISABLE_ERROR_REPORTING"] = []byte("1")
		expected["DISABLE_FEEDBACK_COMMAND"] = []byte("1")
		if captureRun != nil {
			expected["ANTHROPIC_BASE_URL"] = []byte(
				captureRun.provider.BaseURL(),
			)
		}
		if bytes.Contains(body, capability) {
			return fail("NATIVE_SURFACE_INVALID")
		}
	case ProfileCodex:
		expected["CODEX_HOME"] = []byte("/home/sworn/.codex")
		expected["CODEX_EXEC_SERVER_URL"] = []byte("none")
		if bytes.Contains(body, capability) {
			return fail("NATIVE_SURFACE_INVALID")
		}
	default:
		return fail("NATIVE_SURFACE_INVALID")
	}
	entries := bytes.Split(bytes.TrimSuffix(body, []byte{0}), []byte{0})
	if len(entries) != len(expected) {
		return fail("NATIVE_SURFACE_INVALID")
	}
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		separator := bytes.IndexByte(entry, '=')
		if separator < 1 {
			return fail("NATIVE_SURFACE_INVALID")
		}
		key := string(entry[:separator])
		value, present := expected[key]
		if !present || !bytes.Equal(entry[separator+1:], value) {
			return fail("NATIVE_SURFACE_INVALID")
		}
		if _, duplicate := seen[key]; duplicate {
			return fail("NATIVE_SURFACE_INVALID")
		}
		seen[key] = struct{}{}
	}
	return nil
}

func sameOpenFile(name string, file *os.File) bool {
	if file == nil {
		return false
	}
	pathInfo, pathErr := os.Stat(name)
	fileInfo, fileErr := file.Stat()
	return pathErr == nil && fileErr == nil && os.SameFile(pathInfo, fileInfo)
}

func nativeConfigFiles(
	config NativeAdapterConfig,
	invocation Invocation,
	brokerURL string,
	capability []byte,
	captureRun *nativeCaptureRun,
) ([]*os.File, error) {
	mcpConfiguration := map[string]any{"mcpServers": map[string]any{}}
	if config.Family == ProfileClaude {
		mcpConfiguration = map[string]any{"mcpServers": map[string]any{
			"sworn": map[string]any{
				"type": "http", "url": brokerURL,
				"headers": map[string]string{
					"Authorization": "Bearer " + string(capability),
				},
			},
		}}
	}
	var mcpBody []byte
	var err error
	switch config.Family {
	case ProfileClaude:
		mcpBody, err = json.Marshal(mcpConfiguration)
	case ProfileCodex:
		prefix :=
			`model_catalog_json = "/sworn/config/models.json"` + "\n" +
				`web_search = "disabled"` + "\n" +
				`include_permissions_instructions = false` + "\n" +
				`include_apps_instructions = false` + "\n" +
				`include_collaboration_mode_instructions = false` + "\n" +
				`include_environment_context = false` + "\n"
		if captureRun != nil {
			token := captureRun.provider.bearer()
			prefix +=
				`model_provider = "sworn_capture"` + "\n" +
					`[model_providers.sworn_capture]` + "\n" +
					`name = "Sworn capture"` + "\n" +
					`base_url = "` + captureRun.provider.BaseURL() + `"` + "\n" +
					`wire_api = "responses"` + "\n" +
					`experimental_bearer_token = "` + string(token) + `"` + "\n"
			clearBytes(token)
		}
		mcpBody = []byte(
			prefix +
				`[analytics]` + "\n" +
				`enabled = false` + "\n" +
				`[agents]` + "\n" +
				`enabled = false` + "\n" +
				`[orchestrator.skills]` + "\n" +
				`enabled = false` + "\n" +
				`[orchestrator.mcp]` + "\n" +
				`enabled = false` + "\n" +
				`[tools.experimental_request_user_input]` + "\n" +
				`enabled = false` + "\n" +
				`[mcp_servers.sworn]` + "\n" +
				`url = "` + brokerURL + `"` + "\n" +
				`http_headers = { Authorization = "Bearer ` +
				string(capability) + `" }` + "\n" +
				`required = true` + "\n" +
				`startup_timeout_sec = 5` + "\n" +
				`tool_timeout_sec = 300` + "\n" +
				`[skills]` + "\n" +
				`include_instructions = false` + "\n" +
				`[skills.bundled]` + "\n" +
				`enabled = false` + "\n",
		)
	default:
		err = fail("NATIVE_NOT_CERTIFIED")
	}
	if err != nil {
		return nil, fail("INVALID_BROKER")
	}
	mcp, err := unlinkedConfigFile(mcpBody)
	clearBytes(mcpBody)
	if err != nil {
		return nil, err
	}
	schemaBody := []byte(nativeOutputSchemaJSON)
	schema, err := unlinkedConfigFile(schemaBody)
	clearBytes(schemaBody)
	if err != nil {
		_ = mcp.Close()
		return nil, err
	}
	catalogBody, err := codexModelCatalog(invocation.Selected.Model)
	if err != nil {
		_ = mcp.Close()
		_ = schema.Close()
		return nil, err
	}
	catalog, err := unlinkedConfigFile(catalogBody)
	clearBytes(catalogBody)
	if err != nil {
		_ = mcp.Close()
		_ = schema.Close()
		return nil, err
	}
	return []*os.File{mcp, schema, catalog}, nil
}

func nativeCommand(
	config NativeAdapterConfig,
	invocation Invocation,
	credential nativeCredentialLease,
	closure []*os.File,
	configFiles []*os.File,
	captureRun *nativeCaptureRun,
) ([]string, [][]byte, []*os.File, error) {
	if len(closure) != len(config.RuntimeFiles)+1 || len(configFiles) != 3 ||
		credential.File() == nil {
		return nil, nil, nil, fail("NATIVE_NOT_CERTIFIED")
	}
	extra := []*os.File{closure[0], credential.File()}
	extra = append(extra, configFiles...)
	extra = append(extra, closure[1:]...)
	arguments := []string{
		"--die-with-parent", "--new-session",
		"--unshare-all", "--share-net", "--unshare-user", "--disable-userns",
		"--cap-drop", "ALL",
		"--proc", "/proc", "--dev", "/dev", "--tmpfs", "/tmp",
	}
	directories := map[string]struct{}{
		"/home": {}, "/home/sworn": {}, "/workspace": {},
		"/sworn": {}, "/sworn/bin": {}, "/sworn/config": {},
	}
	addParents := func(target string) {
		for parent := filepath.Dir(target); parent != "/" && parent != "."; parent = filepath.Dir(parent) {
			directories[parent] = struct{}{}
		}
	}
	addParents(config.CredentialTarget)
	addParents(nativeMCPConfigTarget(config.Family))
	for _, runtimeFile := range config.RuntimeFiles {
		addParents(runtimeFile.Target)
	}
	dirList := make([]string, 0, len(directories))
	for directory := range directories {
		dirList = append(dirList, directory)
	}
	sort.Slice(dirList, func(left, right int) bool {
		leftDepth, rightDepth := strings.Count(dirList[left], "/"), strings.Count(dirList[right], "/")
		if leftDepth == rightDepth {
			return dirList[left] < dirList[right]
		}
		return leftDepth < rightDepth
	})
	for _, directory := range dirList {
		arguments = append(arguments, "--dir", directory)
	}
	arguments = append(arguments,
		"--ro-bind-fd", "3", "/sworn/bin/agent",
		"--bind-fd", "4", config.CredentialTarget,
		"--perms", "0600", "--ro-bind-data", "5", nativeMCPConfigTarget(config.Family),
		"--perms", "0600", "--ro-bind-data", "6", "/sworn/config/output-schema.json",
		"--perms", "0600", "--ro-bind-data", "7", "/sworn/config/models.json",
	)
	for index, runtimeFile := range config.RuntimeFiles {
		arguments = append(
			arguments,
			"--ro-bind-fd", itoa(8+index), runtimeFile.Target,
		)
	}
	arguments = append(arguments, "--chdir", GuestWorkspacePath)
	environment := [][]byte{
		[]byte("HOME=/home/sworn"), []byte("TMPDIR=/tmp"),
		[]byte("LANG=C.UTF-8"), []byte("LC_ALL=C.UTF-8"), []byte("TZ=UTC"),
	}
	var cliArguments []string
	switch config.Family {
	case ProfileClaude:
		environment = append(environment,
			[]byte("CLAUDE_CODE_DISABLE_AUTO_MEMORY=1"),
			[]byte("CLAUDE_CODE_DISABLE_FEEDBACK_SURVEY=1"),
			[]byte("CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1"),
			[]byte("DISABLE_AUTOUPDATER=1"),
			[]byte("DISABLE_TELEMETRY=1"),
			[]byte("DISABLE_ERROR_REPORTING=1"),
			[]byte("DISABLE_FEEDBACK_COMMAND=1"),
		)
		if captureRun != nil {
			environment = append(
				environment,
				[]byte("ANTHROPIC_BASE_URL="+captureRun.provider.BaseURL()),
			)
		}
		cliArguments = claudeArguments(
			invocation.Selected.Model,
			invocation.Request.Workspace.Access,
		)
	case ProfileCodex:
		environment = append(environment,
			[]byte("CODEX_HOME=/home/sworn/.codex"),
			[]byte("CODEX_EXEC_SERVER_URL=none"),
		)
		cliArguments = codexArguments(
			invocation.Selected.Model,
			invocation.Request.FreshContext,
		)
	default:
		return nil, nil, nil, fail("NATIVE_NOT_CERTIFIED")
	}
	arguments = append(arguments, "/sworn/bin/agent")
	arguments = append(arguments, cliArguments...)
	return arguments, environment, extra, nil
}

func claudeArguments(model string, access WorkspaceAccess) []string {
	tools := make([]string, 0, len(toolDefinitions(access)))
	for _, definition := range toolDefinitions(access) {
		tools = append(tools, "mcp__sworn__"+definition.Name)
	}
	toolList := strings.Join(tools, ",")
	return []string{
		"-p",
		"--setting-sources", "",
		"--strict-mcp-config",
		"--mcp-config", "/sworn/config/mcp.json",
		"--tools", toolList,
		"--allowedTools", toolList,
		"--permission-mode", "dontAsk",
		"--disable-slash-commands",
		"--no-chrome",
		"--no-session-persistence",
		"--max-turns", itoa(MaxProviderTurns),
		"--model", model,
		"--json-schema", nativeOutputSchemaJSON,
		"--output-format", "stream-json",
		"--verbose",
	}
}

func codexArguments(model string, fresh bool) []string {
	arguments := []string{
		"exec",
		"--ephemeral",
		"--yolo",
		"--ignore-user-config",
		"--strict-config",
		"--json",
		"--skip-git-repo-check",
		"-C", GuestWorkspacePath,
		"-m", model,
		"--output-schema", "/sworn/config/output-schema.json",
		"-o", "/tmp/final",
	}
	for _, feature := range codexDisabledFeatures {
		arguments = append(arguments, "--disable", feature)
	}
	if fresh {
		arguments = append(arguments, "--ignore-rules")
	}
	return append(arguments, "-")
}

func nativeMCPConfigTarget(family ProfileFamily) string {
	if family == ProfileCodex {
		return "/etc/codex/config.toml"
	}
	return "/sworn/config/mcp.json"
}

var codexDisabledFeatures = []string{
	"apps", "artifact", "auth_elicitation", "browser_use",
	"browser_use_external", "browser_use_full_cdp_access", "chronicle",
	"code_mode", "code_mode_buffered_exec", "code_mode_host", "code_mode_only",
	"computer_use", "external_agent_memory_import", "goals", "hooks",
	"image_generation", "in_app_browser", "memories", "multi_agent",
	"multi_agent_v2", "network_proxy", "plugins", "plugin_sharing",
	"remote_plugin", "request_permissions_tool", "shell_snapshot", "shell_tool",
	"skill_mcp_dependency_install", "skill_search", "standalone_web_search",
	"tool_call_mcp_elicitation", "tool_suggest", "unified_exec",
	"workspace_dependencies",
}

func codexModelCatalog(model string) ([]byte, error) {
	value := map[string]any{"models": []map[string]any{{
		"slug": model, "display_name": model, "description": nil,
		"supported_reasoning_levels": []any{}, "shell_type": "disabled",
		"visibility": "list", "supported_in_api": true, "priority": 1,
		"availability_nux": nil, "upgrade": nil, "base_instructions": "",
		"model_messages": nil, "default_reasoning_summary": "auto",
		"support_verbosity": false, "default_verbosity": nil,
		"apply_patch_tool_type":          nil,
		"truncation_policy":              map[string]any{"mode": "bytes", "limit": 10000},
		"supports_parallel_tool_calls":   false,
		"supports_image_detail_original": false,
		"context_window":                 nil, "auto_compact_token_limit": nil,
		"effective_context_window_percent": 95,
		"experimental_supported_tools":     []any{},
		"input_modalities":                 []string{"text"},
		"supports_search_tool":             false,
	}}}
	body, err := json.Marshal(value)
	if err != nil || len(body) > 65_536 {
		return nil, fail("NATIVE_NOT_CERTIFIED")
	}
	return body, nil
}

func scanNativeEvents(
	reader io.Reader,
	capability []byte,
	state *nativeEventState,
	additionalSecrets ...[]byte,
) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), MaxProviderResponseBytes)
	total := 0
	for scanner.Scan() {
		line := append([]byte(nil), scanner.Bytes()...)
		total += len(line)
		secretFound := containsCapability(line, capability)
		for _, secret := range additionalSecrets {
			secretFound = secretFound || containsCapability(line, secret)
		}
		if total > MaxProviderResponseBytes || secretFound {
			clearBytes(line)
			return fail("NATIVE_SURFACE_INVALID")
		}
		err := state.accept(line)
		clearBytes(line)
		if err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil &&
		!errors.Is(err, os.ErrClosed) && !errors.Is(err, context.Canceled) {
		return fail("PROVIDER_TRANSPORT_FAILED")
	}
	return nil
}

func (state *nativeEventState) accept(body []byte) error {
	value, err := decodeStrict(body, MaxProviderResponseBytes)
	if err != nil {
		return fail("NATIVE_SURFACE_INVALID")
	}
	root, ok := value.(map[string]any)
	if !ok {
		return fail("NATIVE_SURFACE_INVALID")
	}
	eventType, _ := root["type"].(string)
	state.mu.Lock()
	defer state.mu.Unlock()
	switch state.family {
	case ProfileClaude:
		if eventType == "system" && root["subtype"] == "init" {
			if _, err := closedObject(
				root,
				[]string{
					"type", "subtype", "model", "permissionMode",
					"slash_commands", "skills", "plugins", "tools", "mcp_servers",
				},
				[]string{
					"session_id", "cwd", "apiKeySource", "claude_code_version",
					"output_style", "agents", "fast_mode_state", "uuid",
					"capabilities", "analytics_disabled",
					"product_feedback_disabled",
				},
			); err != nil {
				return fail("NATIVE_SURFACE_INVALID")
			}
			if state.nativeSeen || root["model"] != state.model ||
				root["permissionMode"] != "dontAsk" ||
				!emptyJSONArray(root["slash_commands"]) ||
				!emptyJSONArray(root["skills"]) ||
				!emptyJSONArray(root["plugins"]) ||
				!exactClaudeTools(root["tools"], state.access) ||
				!exactClaudeMCP(root["mcp_servers"]) ||
				!exactClaudeCapabilities(root["capabilities"]) ||
				root["analytics_disabled"] != true ||
				root["product_feedback_disabled"] != true {
				return fail("NATIVE_SURFACE_INVALID")
			}
			if err := state.broker.Arm(); err != nil {
				return err
			}
			state.nativeSeen = true
		}
		if eventType == "result" {
			state.captureUsage(root["usage"])
		}
	case ProfileCodex:
		if eventType == "thread.started" {
			thread, threadErr := closedObject(
				root,
				[]string{"type", "thread_id"},
				nil,
			)
			threadID, threadIDOK := thread["thread_id"].(string)
			if threadErr != nil || !threadIDOK ||
				validateText(threadID, 256, false) != nil ||
				state.nativeSeen {
				return fail("NATIVE_SURFACE_INVALID")
			}
			if err := state.broker.Arm(); err != nil {
				return err
			}
			state.nativeSeen = true
		}
		if eventType == "item.started" || eventType == "item.completed" {
			if _, err := closedObject(
				root,
				[]string{"type", "item"},
				nil,
			); err != nil {
				return fail("NATIVE_SURFACE_INVALID")
			}
			if item, itemOK := root["item"].(map[string]any); itemOK {
				switch item["type"] {
				case "command_execution", "file_change", "web_search",
					"image_generation", "collab_agent_tool_call":
					return fail("NATIVE_SURFACE_INVALID")
				}
			}
		}
		if eventType == "turn.completed" {
			if _, err := closedObject(
				root,
				[]string{"type", "usage"},
				nil,
			); err != nil {
				return fail("NATIVE_SURFACE_INVALID")
			}
			state.captureUsage(root["usage"])
		}
	default:
		return fail("NATIVE_SURFACE_INVALID")
	}
	return nil
}

func (state *nativeEventState) captureUsage(value any) {
	usage, ok := value.(map[string]any)
	if !ok {
		return
	}
	input, inputOK := safeJSONInt(usage["input_tokens"])
	output, outputOK := safeJSONInt(usage["output_tokens"])
	if !inputOK || !outputOK {
		state.err = fail("INVALID_USAGE")
		return
	}
	state.usage = Usage{InputTokens: input, OutputTokens: output}
	state.hasUsage = true
}

func exactClaudeCapabilities(value any) bool {
	array, ok := value.([]any)
	if !ok || len(array) != 2 {
		return false
	}
	expected := map[string]struct{}{
		"interrupt_receipt_v1": {},
		"msg_lifecycle_v1":     {},
	}
	for _, value := range array {
		name, ok := value.(string)
		if !ok {
			return false
		}
		if _, present := expected[name]; !present {
			return false
		}
		delete(expected, name)
	}
	return len(expected) == 0
}

func exactClaudeTools(value any, access WorkspaceAccess) bool {
	array, ok := value.([]any)
	if !ok || len(array) != len(toolDefinitions(access))+1 {
		return false
	}
	expected := make(map[string]struct{})
	for _, definition := range toolDefinitions(access) {
		expected["mcp__sworn__"+definition.Name] = struct{}{}
	}
	expected["StructuredOutput"] = struct{}{}
	if len(array) != len(expected) {
		return false
	}
	for _, raw := range array {
		name, nameOK := raw.(string)
		if !nameOK {
			if object, objectOK := raw.(map[string]any); objectOK {
				name, nameOK = object["name"].(string)
			}
		}
		if !nameOK {
			return false
		}
		if _, present := expected[name]; !present {
			return false
		}
		delete(expected, name)
	}
	return len(expected) == 0
}

func exactClaudeMCP(value any) bool {
	array, ok := value.([]any)
	if !ok || len(array) != 1 {
		return false
	}
	server, ok := array[0].(map[string]any)
	if !ok || server["name"] != "sworn" {
		return false
	}
	status, _ := server["status"].(string)
	return status == "connected"
}

func emptyJSONArray(value any) bool {
	array, ok := value.([]any)
	return ok && len(array) == 0
}

type secretGuard struct {
	mu       sync.Mutex
	token    []byte
	tail     []byte
	total    int64
	maximum  int64
	found    bool
	overflow bool
}

func newSecretGuard(token []byte, maximum int64) *secretGuard {
	return &secretGuard{
		token: append([]byte(nil), token...), maximum: maximum,
	}
}

func (guard *secretGuard) Write(body []byte) (int, error) {
	guard.mu.Lock()
	defer guard.mu.Unlock()
	guard.total += int64(len(body))
	if guard.total > guard.maximum {
		guard.overflow = true
	}
	combined := append(append([]byte(nil), guard.tail...), body...)
	if bytes.Contains(combined, guard.token) {
		guard.found = true
	}
	keep := len(guard.token) - 1
	if keep < 0 {
		keep = 0
	}
	if keep > len(combined) {
		keep = len(combined)
	}
	clearBytes(guard.tail)
	guard.tail = append(guard.tail[:0], combined[len(combined)-keep:]...)
	clearBytes(combined)
	return len(body), nil
}

func (guard *secretGuard) leaked() bool {
	guard.mu.Lock()
	defer guard.mu.Unlock()
	leaked := guard.found || guard.overflow
	clearBytes(guard.token)
	clearBytes(guard.tail)
	guard.token, guard.tail = nil, nil
	return leaked
}

func byteEnvironmentStrings(environment [][]byte) []string {
	values := make([]string, len(environment))
	for index, entry := range environment {
		values[index] = string(entry)
	}
	return values
}
