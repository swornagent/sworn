package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/observe"
)

// Runtime files the sandboxed agent CLI needs for name resolution and TLS.
// They are pinned by digest, so a host that changes them changes the recorded
// configuration.
var initRuntimeTargets = []string{
	"/etc/hosts",
	"/etc/nsswitch.conf",
	"/etc/resolv.conf",
	"/etc/ssl/certs/ca-certificates.crt",
}

const canonicalGitignore = "*\n!records/\n!records/**\n"

// initAgent is one agent CLI Sworn can drive natively.
type initAgent struct {
	name    string
	family  driver.ProfileFamily
	command string
	// resolve turns the executable found on PATH into the binary Sworn must
	// actually run. Package managers commonly install a launcher script whose
	// job is to exec a platform build elsewhere.
	resolve func(string) (string, error)
	target  string
}

func initAgents() []initAgent {
	return []initAgent{
		{
			name: "Codex", family: driver.ProfileCodex, command: "codex",
			resolve: resolveCodexBinary, target: driver.CodexCredentialTarget,
		},
		{
			name: "Claude Code", family: driver.ProfileClaude, command: "claude",
			resolve: resolveDirectBinary, target: driver.ClaudeCredentialTarget,
		},
	}
}

type initSession struct {
	in             *bufio.Reader
	out            io.Writer
	errOut         io.Writer
	nonInteractive bool
	force          bool
}

func newInitSession(in io.Reader, out, errOut io.Writer, nonInteractive, force bool) *initSession {
	var reader *bufio.Reader
	if in != nil {
		reader = bufio.NewReader(in)
	}
	return &initSession{
		in:             reader,
		out:            out,
		errOut:         errOut,
		nonInteractive: nonInteractive,
		force:          force,
	}
}

func (s *initSession) confirm(prompt string, defaultYes bool) bool {
	if s.force && !defaultYes {
		return true
	}
	if s.nonInteractive {
		return defaultYes
	}
	defaultHint := "[Y/n]"
	if !defaultYes {
		defaultHint = "[y/N]"
	}
	fmt.Fprintf(s.out, "%s %s ", prompt, defaultHint)
	if s.in == nil {
		fmt.Fprintln(s.out)
		return defaultYes
	}
	line, err := s.in.ReadString('\n')
	if err != nil && line == "" {
		fmt.Fprintln(s.out)
		return defaultYes
	}
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return defaultYes
	}
	if strings.EqualFold(trimmed, "y") || strings.EqualFold(trimmed, "yes") {
		return true
	}
	if strings.EqualFold(trimmed, "n") || strings.EqualFold(trimmed, "no") {
		return false
	}
	return defaultYes
}

func runInit(args []string, stdout, stderr io.Writer) int {
	return runInitWithIO(args, os.Stdin, stdout, stderr)
}

func runInitWithIO(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	options, ok := parseOptionsWithOptionalValues(
		args,
		nil,
		[]string{"--project"},
		nil,
		[]string{"--force", "--yes", "-y"},
	)
	if !ok {
		fmt.Fprintln(stderr, "usage: sworn init [--project ABS] [--force] [--yes]")
		return 2
	}

	root, err := initProjectRoot(options["--project"])
	if err != nil {
		writeKnownFailure(
			stderr,
			"init",
			"Sworn could not find a Git project here. Run this inside a Git project, or pass --project with an absolute path.",
			"",
		)
		return 1
	}
	paths, err := resolveProjectPaths(root, "", "", "")
	if err != nil {
		writeKnownFailure(stderr, "init", "Project paths could not be resolved.", "")
		return 1
	}

	forced := options["--force"] == "true"
	nonInteractive := options["--yes"] == "true" || options["-y"] == "true"
	session := newInitSession(stdin, stdout, stderr, nonInteractive, forced)

	fmt.Fprintf(stdout, "Project: %s\n", root)

	if err := setupProjectSurface(session, paths); err != nil {
		writeKnownFailure(stderr, "init", "Sworn could not prepare the project directory .sworn.", "")
		return 1
	}

	if err := setupDriverConfig(session, paths); err != nil {
		writeKnownFailure(stderr, "init", err.Error(), "")
		return 1
	}

	if err := setupOperatorConfig(session, paths); err != nil {
		writeKnownFailure(stderr, "init", err.Error(), "")
		return 1
	}

	reportProjectReleases(stdout, root)
	reportNextStep(stdout, paths)

	if _, statErr := os.Stat(paths.config); os.IsNotExist(statErr) {
		fmt.Fprintln(
			stdout,
			"\nSworn cannot start a run until an AI connection file exists at",
		)
		fmt.Fprintf(stdout, "  %s\n", paths.config)
		return 1
	}

	return 0
}

func initProjectRoot(override string) (string, error) {
	start := override
	if start == "" {
		working, err := os.Getwd()
		if err != nil {
			return "", err
		}
		start = working
	}
	if !filepath.IsAbs(start) || filepath.Clean(start) != start {
		return "", fmt.Errorf("project path must be absolute and clean")
	}
	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		return "", err
	}
	repository, err := gitx.Open(start, gitExecutable)
	if err != nil {
		return "", err
	}
	return repository.Root(), nil
}

func setupProjectSurface(s *initSession, paths projectPaths) error {
	home := filepath.Dir(paths.config)
	manifestDir := paths.manifestDir
	ignorePath := filepath.Join(home, ".gitignore")

	homeInfo, homeErr := os.Stat(home)
	homeExists := homeErr == nil && homeInfo.IsDir()

	manInfo, manErr := os.Stat(manifestDir)
	manExists := manErr == nil && manInfo.IsDir()

	ignInfo, ignErr := os.Lstat(ignorePath)
	ignExists := ignErr == nil && ignInfo.Mode().IsRegular()

	var ignContent []byte
	ignMatches := false
	if ignExists {
		ignContent, _ = os.ReadFile(ignorePath)
		ignMatches = bytes.Equal(ignContent, []byte(canonicalGitignore))
	}

	if homeExists && manExists && ignExists && ignMatches {
		fmt.Fprintf(s.out, "  Project surface already current: %s/\n", home)
		return nil
	}

	if !homeExists || !manExists || !ignExists {
		prompt := fmt.Sprintf("Create project surface (%s/, %s/, %s)?", home, manifestDir, ignorePath)
		if s.confirm(prompt, true) {
			if !homeExists {
				if err := os.MkdirAll(home, 0o755); err != nil {
					return err
				}
				fmt.Fprintf(s.out, "  created %s/\n", home)
			}
			if !manExists {
				if err := os.MkdirAll(manifestDir, 0o755); err != nil {
					return err
				}
				fmt.Fprintf(s.out, "  created %s/\n", manifestDir)
			}
			if !ignExists {
				if err := os.WriteFile(ignorePath, []byte(canonicalGitignore), 0o644); err != nil {
					return err
				}
				fmt.Fprintf(s.out, "  created %s\n", ignorePath)
			}
		} else {
			fmt.Fprintln(s.out, "  skipped project surface")
		}
	}

	if ignExists && !ignMatches {
		fmt.Fprintf(s.out, "  .gitignore at %s differs from canonical allowlist:\n%s", ignorePath, renderDiff(ignContent, []byte(canonicalGitignore), ignInfo.Mode(), 0o644))
		if s.confirm(fmt.Sprintf("Replace %s?", ignorePath), false) {
			if err := os.WriteFile(ignorePath, []byte(canonicalGitignore), 0o644); err != nil {
				return err
			}
			if err := os.Chmod(ignorePath, 0o644); err != nil {
				return err
			}
			fmt.Fprintf(s.out, "  replaced %s\n", ignorePath)
		} else {
			fmt.Fprintf(s.out, "  kept existing %s\n", ignorePath)
		}
	}

	return nil
}

func setupDriverConfig(s *initSession, paths projectPaths) error {
	agent, agentErr := detectInitAgent()
	if agentErr != nil {
		if _, statErr := os.Stat(paths.config); statErr == nil {
			fmt.Fprintf(s.out, "  AI connection file present: %s\n", paths.config)
		} else {
			fmt.Fprintf(s.out, "  AI driver configuration: missing (%s)\n", strings.ReplaceAll(strings.TrimSpace(agentErr.Error()), "\n", " "))
		}
		return nil
	}

	body, err := buildDriverConfig(agent)
	if err != nil {
		fmt.Fprintf(s.out, "  AI driver configuration: unavailable (%s)\n", err.Error())
		return nil
	}

	info, statErr := os.Lstat(paths.config)
	if os.IsNotExist(statErr) {
		prompt := fmt.Sprintf("Write AI driver configuration for %s %s (%s)?", agent.name, agent.version, paths.config)
		if s.confirm(prompt, true) {
			if err := os.MkdirAll(filepath.Dir(paths.config), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(paths.config, body, 0o600); err != nil {
				return fmt.Errorf("The AI connection file at %s could not be written.", paths.config)
			}
			fmt.Fprintf(s.out, "  wrote %s (%s %s)\n", paths.config, agent.name, agent.version)
		} else {
			fmt.Fprintf(s.out, "  skipped %s\n", paths.config)
		}
		return nil
	}

	if statErr == nil {
		existing, readErr := os.ReadFile(paths.config)
		isMode600 := info.Mode().Perm() == 0o600 && info.Mode().IsRegular()
		if readErr == nil && bytes.Equal(existing, body) && isMode600 {
			fmt.Fprintf(s.out, "  AI connection file already current: %s\n", paths.config)
			return nil
		}

		fmt.Fprintf(s.out, "  AI connection file at %s differs from proposed configuration:\n%s", paths.config, renderDiff(existing, body, info.Mode(), 0o600))
		prompt := fmt.Sprintf("Replace AI driver configuration (%s)?", paths.config)
		if s.confirm(prompt, false) {
			if err := os.WriteFile(paths.config, body, 0o600); err != nil {
				return fmt.Errorf("The AI connection file at %s could not be written.", paths.config)
			}
			if err := os.Chmod(paths.config, 0o600); err != nil {
				return fmt.Errorf("The AI connection file at %s could not be written.", paths.config)
			}
			fmt.Fprintf(s.out, "  replaced %s (%s %s)\n", paths.config, agent.name, agent.version)
		} else {
			fmt.Fprintf(s.out, "  kept existing %s\n", paths.config)
		}
	}

	return nil
}

func buildDefaultOperatorConfig() []byte {
	return []byte("{\n  \"schema_version\": \"sworn.operator-config/v1\",\n  \"local\": {\n    \"listen\": \"127.0.0.1:7337\"\n  }\n}\n")
}

// setupOperatorConfig scaffolds the operator config with the guided
// telemetry step (A5): interactive runs ask for the private OTLP/HTTP
// endpoint and the share opt-in, and seed the proposed body from any blocks
// an existing config already carries, so re-running reports "already
// current" instead of offering to replace the operator's telemetry config
// with the bare default. --yes and --force keep the non-interactive path
// byte-identical to today: no prompt is ever asked and no share/otel block
// is ever written (C4 - --force's confirm() semantics answer a default-no
// question with yes, so the share opt-in must never route through confirm).
func setupOperatorConfig(s *initSession, paths projectPaths) error {
	info, statErr := os.Lstat(paths.operatorConfig)
	if os.IsNotExist(statErr) {
		prompt := fmt.Sprintf("Write operator configuration with local listen default 127.0.0.1:7337 (%s)?", paths.operatorConfig)
		if s.confirm(prompt, true) {
			if err := os.MkdirAll(filepath.Dir(paths.operatorConfig), 0o755); err != nil {
				return err
			}
			body := buildDefaultOperatorConfig()
			if !s.nonInteractive && !s.force {
				guided, err := guidedOperatorBody(s)
				if err != nil {
					return err
				}
				body = guided
			}
			if err := os.WriteFile(paths.operatorConfig, body, 0o600); err != nil {
				return fmt.Errorf("The operator configuration at %s could not be written.", paths.operatorConfig)
			}
			fmt.Fprintf(s.out, "  wrote %s (local listen 127.0.0.1:7337)\n", paths.operatorConfig)
		} else {
			fmt.Fprintf(s.out, "  skipped %s\n", paths.operatorConfig)
		}
		return nil
	}

	if statErr == nil {
		existing, readErr := os.ReadFile(paths.operatorConfig)
		isMode600 := info.Mode().Perm() == 0o600 && info.Mode().IsRegular()
		if s.nonInteractive || s.force {
			// The non-interactive flows are exactly today's: bare default
			// body, unchanged-file and diff+replace prompts, never a
			// telemetry question (C4).
			body := buildDefaultOperatorConfig()
			if readErr == nil && bytes.Equal(existing, body) && isMode600 {
				fmt.Fprintf(s.out, "  Operator configuration already current: %s\n", paths.operatorConfig)
				return nil
			}
			fmt.Fprintf(s.out, "  Operator configuration at %s differs from proposed configuration:\n%s", paths.operatorConfig, renderDiff(existing, body, info.Mode(), 0o600))
			prompt := fmt.Sprintf("Replace operator configuration (%s)?", paths.operatorConfig)
			if s.confirm(prompt, false) {
				if err := os.WriteFile(paths.operatorConfig, body, 0o600); err != nil {
					return fmt.Errorf("The operator configuration at %s could not be written.", paths.operatorConfig)
				}
				if err := os.Chmod(paths.operatorConfig, 0o600); err != nil {
					return fmt.Errorf("The operator configuration at %s could not be written.", paths.operatorConfig)
				}
				fmt.Fprintf(s.out, "  replaced %s (local listen 127.0.0.1:7337)\n", paths.operatorConfig)
			} else {
				fmt.Fprintf(s.out, "  kept existing %s\n", paths.operatorConfig)
			}
			return nil
		}

		// Interactive guided flow: seed from the existing file's telemetry
		// blocks, ask only for what is missing, and leave an unchanged
		// config byte-identical.
		proposed, guidedErr := seedGuidedOperatorBody(s, existing)
		if guidedErr != nil {
			// The existing file cannot be read as an operator config; keep
			// it and offer the bare-default replacement exactly as before.
			body := buildDefaultOperatorConfig()
			if readErr == nil && bytes.Equal(existing, body) && isMode600 {
				fmt.Fprintf(s.out, "  Operator configuration already current: %s\n", paths.operatorConfig)
				return nil
			}
			fmt.Fprintf(s.out, "  Operator configuration at %s differs from proposed configuration:\n%s", paths.operatorConfig, renderDiff(existing, body, info.Mode(), 0o600))
			prompt := fmt.Sprintf("Replace operator configuration (%s)?", paths.operatorConfig)
			if s.confirm(prompt, false) {
				if err := os.WriteFile(paths.operatorConfig, body, 0o600); err != nil {
					return fmt.Errorf("The operator configuration at %s could not be written.", paths.operatorConfig)
				}
				if err := os.Chmod(paths.operatorConfig, 0o600); err != nil {
					return fmt.Errorf("The operator configuration at %s could not be written.", paths.operatorConfig)
				}
				fmt.Fprintf(s.out, "  replaced %s (local listen 127.0.0.1:7337)\n", paths.operatorConfig)
			} else {
				fmt.Fprintf(s.out, "  kept existing %s\n", paths.operatorConfig)
			}
			return nil
		}
		if bytes.Equal(proposed, existing) && isMode600 {
			fmt.Fprintf(s.out, "  Operator configuration already current: %s\n", paths.operatorConfig)
			return nil
		}
		fmt.Fprintf(s.out, "  Operator configuration at %s differs from proposed configuration:\n%s", paths.operatorConfig, renderDiff(existing, proposed, info.Mode(), 0o600))
		prompt := fmt.Sprintf("Replace operator configuration (%s)?", paths.operatorConfig)
		if s.confirm(prompt, false) {
			if err := os.WriteFile(paths.operatorConfig, proposed, 0o600); err != nil {
				return fmt.Errorf("The operator configuration at %s could not be written.", paths.operatorConfig)
			}
			if err := os.Chmod(paths.operatorConfig, 0o600); err != nil {
				return fmt.Errorf("The operator configuration at %s could not be written.", paths.operatorConfig)
			}
			fmt.Fprintf(s.out, "  replaced %s (local listen 127.0.0.1:7337)\n", paths.operatorConfig)
		} else {
			fmt.Fprintf(s.out, "  kept existing %s\n", paths.operatorConfig)
		}
	}

	return nil
}

// guidedOperatorBody renders a fresh operator config body after the guided
// telemetry questions (the brand-new scaffold flow).
func guidedOperatorBody(s *initSession) ([]byte, error) {
	config, err := parseOperatorBody(buildDefaultOperatorConfig())
	if err != nil {
		return nil, err
	}
	otelBlock, err := guidedOTelBlock(s)
	if err != nil {
		return nil, err
	}
	shareBlock, err := guidedShareBlock(s)
	if err != nil {
		return nil, err
	}
	if otelBlock == nil && shareBlock == nil {
		return buildDefaultOperatorConfig(), nil
	}
	config.OTel = otelBlock
	config.Share = shareBlock
	return renderOperatorConfigBody(config), nil
}

// seedGuidedOperatorBody parses an existing operator config, asks only the
// telemetry questions whose blocks are absent, and returns the proposed
// body. When nothing was answered in, it returns the existing bytes exactly,
// so an operator-edited telemetry config is never offered replacement with
// the bare default (captain F6).
func seedGuidedOperatorBody(
	s *initSession,
	existing []byte,
) ([]byte, error) {
	config, err := parseOperatorBody(existing)
	if err != nil {
		return nil, err
	}
	var otelBlock *observe.Config
	var shareBlock *observe.ShareConfig
	if config.OTel != nil {
		otelBlock = config.OTel
	} else {
		otelBlock, err = guidedOTelBlock(s)
		if err != nil {
			return nil, err
		}
	}
	if config.Share != nil {
		shareBlock = config.Share
	} else {
		shareBlock, err = guidedShareBlock(s)
		if err != nil {
			return nil, err
		}
	}
	if (config.OTel != nil || otelBlock == nil) &&
		(config.Share != nil || shareBlock == nil) {
		return existing, nil
	}
	config.OTel = otelBlock
	config.Share = shareBlock
	return renderOperatorConfigBody(config), nil
}

// guidedOTelBlock asks for the private OTLP/HTTP endpoint in free text. An
// empty answer means no private channel; a non-empty answer must parse
// through the same strict endpoint rules the operator config uses, or it is
// reported and skipped. This is interactive-only by construction.
func guidedOTelBlock(s *initSession) (*observe.Config, error) {
	if s.nonInteractive || s.force {
		return nil, nil
	}
	endpoint, err := s.promptText("Private telemetry OTLP endpoint (empty to skip)?")
	if err != nil {
		return nil, err
	}
	if endpoint == "" {
		return nil, nil
	}
	candidate := observe.Config{
		SchemaVersion: observe.OTelConfigSchemaVersion,
		Endpoint:      endpoint,
		Headers:       map[string]string{},
	}
	body, err := json.Marshal(candidate)
	if err != nil {
		return nil, err
	}
	parsed, err := observe.ParseConfig(body)
	if err != nil {
		fmt.Fprintf(s.out, "  skipped private telemetry (endpoint %q is not a valid OTLP endpoint)\n", endpoint)
		return nil, nil
	}
	return &parsed, nil
}

// guidedShareBlock asks the share opt-in question (default no). It is
// interactive-only and must never be answered by a flag: under --force,
// confirm() turns a default-no question into yes, which would opt an
// operator into exporting without them ever answering (C4).
func guidedShareBlock(s *initSession) (*observe.ShareConfig, error) {
	if s.nonInteractive || s.force {
		return nil, nil
	}
	prompt := fmt.Sprintf("Share fleet telemetry with the Sworn project gateway (%s)?", observe.ShareDefaultEndpoint)
	if !s.confirm(prompt, false) {
		return nil, nil
	}
	return &observe.ShareConfig{
		SchemaVersion: observe.ShareConfigSchemaVersion,
		Enabled:       true,
		Endpoint:      observe.ShareDefaultEndpoint,
		Headers:       map[string]string{},
	}, nil
}

// promptText reads one free-text line interactively. It is never used on a
// non-interactive session; a missing or exhausted input yields the empty
// answer, which declines the question.
func (s *initSession) promptText(prompt string) (string, error) {
	if s.nonInteractive || s.force {
		return "", nil
	}
	fmt.Fprintf(s.out, "%s ", prompt)
	if s.in == nil {
		fmt.Fprintln(s.out)
		return "", nil
	}
	line, err := s.in.ReadString('\n')
	if err != nil && line == "" {
		fmt.Fprintln(s.out)
		return "", nil
	}
	return strings.TrimSpace(line), nil
}

// parseOperatorBody applies the same closed admission rules as
// parseOperatorConfig but returns the typed body so the guided flow can
// re-render it with added telemetry blocks.
func parseOperatorBody(body []byte) (operatorConfig, error) {
	if len(body) < 2 || len(body) > maxOperatorConfigBytes ||
		rejectAmbiguousOperatorJSON(body) != nil ||
		validateExactOperatorFields(body) != nil {
		return operatorConfig{}, errors.New("operator config unavailable")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var config operatorConfig
	if err := decoder.Decode(&config); err != nil {
		return operatorConfig{}, errors.New("operator config unavailable")
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return operatorConfig{}, errors.New("operator config unavailable")
	}
	if config.SchemaVersion != operatorConfigSchemaVersion ||
		len(config.Webhooks) > 32 {
		return operatorConfig{}, errors.New("operator config unavailable")
	}
	return config, nil
}

// renderOperatorConfigBody renders the canonical pretty scaffold shape with
// the optional telemetry blocks appended. omitempty keeps the bare default
// byte-identical to buildDefaultOperatorConfig.
func renderOperatorConfigBody(config operatorConfig) []byte {
	body, err := json.MarshalIndent(struct {
		SchemaVersion string                  `json:"schema_version"`
		Local         operatorLocalConfig     `json:"local"`
		Public        *operatorPublicConfig   `json:"public,omitempty"`
		Webhooks      []operatorWebhookConfig `json:"webhooks,omitempty"`
		OTel          *observe.Config         `json:"otel,omitempty"`
		Share         *observe.ShareConfig    `json:"share,omitempty"`
	}{
		SchemaVersion: config.SchemaVersion,
		Local:         config.Local,
		Public:        config.Public,
		Webhooks:      config.Webhooks,
		OTel:          config.OTel,
		Share:         config.Share,
	}, "", "  ")
	if err != nil {
		return buildDefaultOperatorConfig()
	}
	return append(body, '\n')
}

func renderDiff(existing, proposed []byte, existingMode, expectedMode os.FileMode) string {
	var sb strings.Builder
	if existingMode != 0 && existingMode.Perm() != expectedMode.Perm() {
		sb.WriteString(fmt.Sprintf("  --- mode: 0%o\n  +++ mode: 0%o\n", existingMode.Perm(), expectedMode.Perm()))
	}
	existingStr := strings.TrimRight(string(existing), "\n")
	proposedStr := strings.TrimRight(string(proposed), "\n")
	if existingStr == proposedStr {
		return sb.String()
	}
	sb.WriteString("  --- existing\n  +++ proposed\n")
	if len(existingStr) > 0 {
		for _, line := range strings.Split(existingStr, "\n") {
			sb.WriteString("  - " + line + "\n")
		}
	}
	if len(proposedStr) > 0 {
		for _, line := range strings.Split(proposedStr, "\n") {
			sb.WriteString("  + " + line + "\n")
		}
	}
	return sb.String()
}

type detectedAgent struct {
	initAgent
	binary  string
	digest  string
	version string
	output  string
}

func detectInitAgent() (detectedAgent, error) {
	var looked []string
	for _, agent := range initAgents() {
		looked = append(looked, agent.command)
		found, err := exec.LookPath(agent.command)
		if err != nil {
			continue
		}
		binary, err := agent.resolve(found)
		if err != nil {
			continue
		}
		digest, err := fileDigest(binary)
		if err != nil {
			continue
		}
		output, err := agentVersionOutput(binary)
		if err != nil {
			continue
		}
		version, ok := agentReportedVersion(agent.family, output)
		if !ok {
			continue
		}
		return detectedAgent{
			initAgent: agent, binary: binary, digest: digest,
			version: version, output: strings.TrimSuffix(output, "\n"),
		}, nil
	}
	return detectedAgent{}, fmt.Errorf(
		"No supported agent command was found on PATH.\n"+
			"Sworn looked for: %s.\n"+
			"Install one and run sworn init again.",
		strings.Join(looked, ", "),
	)
}

func buildDriverConfig(agent detectedAgent) ([]byte, error) {
	runtimeFiles := make([]driver.PinnedRuntimeFile, 0, len(initRuntimeTargets))
	required := make([]string, 0, len(initRuntimeTargets))
	for _, target := range initRuntimeTargets {
		resolved, err := filepath.EvalSymlinks(target)
		if err != nil {
			return nil, fmt.Errorf(
				"This machine is missing %s, which the sandboxed agent needs.", target,
			)
		}
		digest, err := fileDigest(resolved)
		if err != nil {
			return nil, fmt.Errorf("%s could not be read.", target)
		}
		runtimeFiles = append(runtimeFiles, driver.PinnedRuntimeFile{
			Path: resolved, Target: target, Digest: digest,
		})
		required = append(required, target)
	}
	credential := agentCredentialSource(agent.family)
	if _, err := os.Stat(credential); err != nil {
		return nil, fmt.Errorf(
			"%s is installed but not signed in: %s is missing.\n"+
				"Sign in with %s, then run sworn init again.",
			agent.name, credential, agent.command,
		)
	}
	reference := "agent-credential"
	config := driver.DriverConfig{
		SchemaVersion: driver.DriverConfigSchemaVersion,
		Credentials: []driver.DriverCredentialSource{{
			Key: reference, Kind: driver.CredentialFile, Reference: credential,
		}},
		Adapters: []driver.DriverAdapterConfig{{
			Native: &driver.NativeAdapterConfig{
				Key: "agent-native", ID: "sworn." + agent.command, Version: "1.0.0",
				Family: agent.family,
				CLI: driver.ExecutableIdentity{
					Path: agent.binary, Digest: agent.digest,
				},
				CLIVersion:             agent.version,
				VersionOutput:          agent.output,
				RuntimeFiles:           runtimeFiles,
				RequiredRuntimeTargets: required,
				CredentialTarget:       agent.target,
				CredentialRefs:         []string{reference},
				MaxCredentialBytes:     65_536,
			},
		}},
		Profiles: []driver.DriverProfile{{
			Key: agent.command, Adapter: "agent-native",
			Network: driver.NetworkRequired, CredentialSource: &reference,
			CertificationModels: []string{initDefaultModel(agent.family)},
		}},
	}
	body, err := driver.EncodeDriverConfig(config)
	if err != nil {
		return nil, fmt.Errorf(
			"%s %s is installed, but this Sworn build does not admit it.\n"+
				"Technical code: %s",
			agent.name, agent.version, commandErrorCode(err),
		)
	}
	return body, nil
}

func reportProjectReleases(out io.Writer, root string) {
	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		return
	}
	repository, err := gitx.Open(root, gitExecutable)
	if err != nil {
		return
	}
	refs, err := baton.ListReleaseRefs(baton.UseGitRepository(repository))
	if err != nil {
		fmt.Fprintln(out, "\nDelivery releases: could not be read.")
		return
	}
	if len(refs) == 0 {
		fmt.Fprintln(out, "\nDelivery releases: none in this project.")
		fmt.Fprintln(
			out,
			"Sworn needs an approved release recorded under .sworn/records before a run can start.",
		)
		return
	}
	fmt.Fprintf(out, "\nDelivery releases: %d\n", len(refs))
	for _, ref := range refs {
		fmt.Fprintf(out, "  %s\n", ref.Release)
	}
}

func reportNextStep(out io.Writer, paths projectPaths) {
	entries, err := os.ReadDir(paths.manifestDir)
	definitions := 0
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
				definitions++
			}
		}
	}
	fmt.Fprintln(out, "\nNext:")
	fmt.Fprintln(out, "  sworn driver doctor  confirms the AI connection works")
	if definitions == 0 {
		fmt.Fprintf(
			out,
			"  a run definition is still required in %s before a run can start\n",
			paths.manifestDir,
		)
		return
	}
	fmt.Fprintln(out, "  sworn                opens this project")
}

func resolveDirectBinary(found string) (string, error) {
	return filepath.EvalSymlinks(found)
}

// resolveCodexBinary walks from the launcher script installed on PATH to the
// platform build it executes. The launcher is a Node script and cannot itself be
// run under Sworn's sandbox.
func resolveCodexBinary(found string) (string, error) {
	resolved, err := filepath.EvalSymlinks(found)
	if err != nil {
		return "", err
	}
	if !strings.HasSuffix(resolved, ".js") {
		return resolved, nil
	}
	packageRoot := filepath.Dir(filepath.Dir(resolved))
	for _, platform := range []string{
		"codex-linux-x64/vendor/x86_64-unknown-linux-musl/bin/codex",
		"codex-linux-arm64/vendor/aarch64-unknown-linux-musl/bin/codex",
	} {
		candidate := filepath.Join(
			packageRoot, "node_modules", "@openai", platform,
		)
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no platform build found beside %s", resolved)
}

func agentVersionOutput(binary string) (string, error) {
	command := exec.Command(binary, "--version")
	command.Env = []string{"HOME=/tmp", "LANG=C", "LC_ALL=C", "TZ=UTC"}
	output, err := command.Output()
	if err != nil {
		return "", err
	}
	if len(output) > 256 {
		return "", fmt.Errorf("version output too long")
	}
	return string(output), nil
}

func agentReportedVersion(
	family driver.ProfileFamily,
	output string,
) (string, bool) {
	trimmed := strings.TrimSuffix(output, "\n")
	var value string
	var ok bool
	switch family {
	case driver.ProfileCodex:
		value, ok = strings.CutPrefix(trimmed, "codex-cli ")
	case driver.ProfileClaude:
		value, ok = strings.CutSuffix(trimmed, " (Claude Code)")
	}
	// Cut returns the input unchanged when it does not match, which would hand
	// a caller a plausible-looking version that was never reported.
	if !ok || value == "" {
		return "", false
	}
	return value, true
}

func agentCredentialSource(family driver.ProfileFamily) string {
	// The configured machine/user credentials directory (SWORN_CREDENTIALS_DIR
	// or the XDG-conformant $XDG_CONFIG_HOME/sworn default) is the base where
	// Sworn looks for agent credential files. The approved XDG default is
	// always effective: it is never bypassed in favour of the user home, so an
	// operator who keeps agent-owned files at their standard locations sets
	// SWORN_CREDENTIALS_DIR to the parent directory that holds .codex/.claude.
	base, err := gitx.LoadHostPaths()
	if err != nil {
		return ""
	}
	credentialsBase := base.CredentialsDir
	switch family {
	case driver.ProfileCodex:
		return filepath.Join(credentialsBase, ".codex", "auth.json")
	default:
		return filepath.Join(credentialsBase, ".claude", ".credentials.json")
	}
}

func fileDigest(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	sum := sha256.New()
	if _, err := io.Copy(sum, file); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(sum.Sum(nil)), nil
}

func initDefaultModel(family driver.ProfileFamily) string {
	if family == driver.ProfileCodex {
		return "gpt-5.6-sol"
	}
	return "claude-opus-4-6"
}
