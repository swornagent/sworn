package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/gitx"
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

func runInit(args []string, stdout, stderr io.Writer) int {
	options, ok := parseOptionsWithOptionalValues(
		args,
		nil,
		[]string{"--project"},
		nil,
		[]string{"--force"},
	)
	if !ok {
		fmt.Fprintln(stderr, "usage: sworn init [--project ABS] [--force]")
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

	created, err := prepareProjectDirectories(paths)
	if err != nil {
		writeKnownFailure(
			stderr,
			"init",
			"Sworn could not prepare the project directory .sworn.",
			"",
		)
		return 1
	}

	fmt.Fprintf(stdout, "Project: %s\n", root)
	for _, line := range created {
		fmt.Fprintf(stdout, "  created %s\n", line)
	}

	_, forced := options["--force"]
	wrote, err := writeProjectDriverConfig(paths.config, forced)
	if err != nil {
		fmt.Fprintln(stdout)
		writeKnownFailure(stderr, "init", err.Error(), "")
		reportProjectReleases(stdout, root)
		if _, statErr := os.Stat(paths.config); os.IsNotExist(statErr) {
			fmt.Fprintln(
				stdout,
				"\nSworn cannot start a run until an AI connection file exists at",
			)
			fmt.Fprintf(stdout, "  %s\n", paths.config)
		}
		return 1
	}
	if wrote != "" {
		fmt.Fprintf(stdout, "  %s\n", wrote)
	}

	reportProjectReleases(stdout, root)
	reportNextStep(stdout, paths)
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

// prepareProjectDirectories creates .sworn and its manifest directory, and
// ensures nothing inside .sworn is ever committed. The directory holds absolute
// host paths, binary digests, and the run journal, none of which belong in a
// repository that other people clone.
func prepareProjectDirectories(paths projectPaths) ([]string, error) {
	var created []string
	home := filepath.Dir(paths.config)
	for _, directory := range []string{home, paths.manifestDir} {
		info, err := os.Stat(directory)
		switch {
		case err == nil && info.IsDir():
		case os.IsNotExist(err):
			if err := os.MkdirAll(directory, 0o755); err != nil {
				return nil, err
			}
			created = append(created, directory+"/")
		default:
			return nil, fmt.Errorf("%s is not a directory", directory)
		}
	}
	ignore := filepath.Join(home, ".gitignore")
	if _, err := os.Stat(ignore); os.IsNotExist(err) {
		if err := os.WriteFile(ignore, []byte("*\n"), 0o644); err != nil {
			return nil, err
		}
		created = append(created, ignore)
	}
	return created, nil
}

// writeProjectDriverConfig derives an AI connection file from the agent CLI
// installed on this machine. It returns the reported action, or an error whose
// message is written for a person to act on.
func writeProjectDriverConfig(path string, force bool) (string, error) {
	existing, err := os.ReadFile(path)
	switch {
	case err == nil && !force:
		return "", fmt.Errorf(
			"An AI connection file already exists at %s.\n"+
				"Sworn will not change it, because run definitions record its exact fingerprint\n"+
				"and rewriting it would invalidate them. Re-run with --force to replace it.",
			path,
		)
	case err != nil && !os.IsNotExist(err):
		return "", fmt.Errorf("The AI connection file at %s could not be read.", path)
	}

	agent, err := detectInitAgent()
	if err != nil {
		return "", err
	}
	body, err := buildDriverConfig(agent)
	if err != nil {
		return "", err
	}
	if len(existing) != 0 && string(existing) == string(body) {
		return "AI connection file already current: " + path, nil
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		return "", fmt.Errorf("The AI connection file at %s could not be written.", path)
	}
	action := "wrote"
	if len(existing) != 0 {
		action = "replaced"
	}
	return fmt.Sprintf(
		"%s %s (%s %s)", action, path, agent.name, agent.version,
	), nil
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
			"Sworn needs an approved release recorded under .baton/releases before a run can start.",
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
	// or the XDG-conformant default) is the base where Sworn looks for agent
	// credential files. When the operator has not relocated it, Sworn keeps
	// reading the agent-owned files at their standard locations under the
	// user home so a signed-in agent is always found.
	base, err := gitx.LoadHostPaths()
	if err != nil {
		base = gitx.HostPaths{}
	}
	credentialsBase := base.CredentialsDir
	if os.Getenv(gitx.EnvCredentialsDir) == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		credentialsBase = home
	}
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
