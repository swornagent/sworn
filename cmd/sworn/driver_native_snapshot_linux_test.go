//go:build linux

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/gitx"
)

// driverSnapshotConfigFixture writes a run-snapshot Claude connection file
// whose host CLI is a launcher symlink to a scripted 2.1.280 build.
func driverSnapshotConfigFixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv(gitx.EnvArtefactHome, filepath.Join(root, "artefacts"))
	executable := filepath.Join(root, "claude-2.1.280")
	if err := os.WriteFile(
		executable,
		[]byte("#!/usr/bin/sh\necho '2.1.280 (Claude Code)'\n"),
		0o755,
	); err != nil {
		t.Fatal(err)
	}
	host := filepath.Join(root, "claude")
	if err := os.Symlink(executable, host); err != nil {
		t.Fatal(err)
	}
	var runtimeFiles []driver.PinnedRuntimeFile
	var required []string
	for _, target := range []string{
		"/etc/hosts",
		"/etc/nsswitch.conf",
		"/etc/resolv.conf",
		"/etc/ssl/certs/ca-certificates.crt",
	} {
		body := []byte("runtime " + target)
		pathValue := filepath.Join(
			root, "runtime-"+strings.ReplaceAll(target[1:], "/", "-"),
		)
		if err := os.WriteFile(pathValue, body, 0o600); err != nil {
			t.Fatal(err)
		}
		runtimeFiles = append(runtimeFiles, driver.PinnedRuntimeFile{
			Path: pathValue, Target: target, Digest: driver.Digest(body),
		})
		required = append(required, target)
	}
	credential := "claude-file"
	body, err := driver.EncodeDriverConfig(driver.DriverConfig{
		SchemaVersion: driver.DriverConfigSchemaVersion,
		Credentials: []driver.DriverCredentialSource{{
			Key: credential, Kind: driver.CredentialFile,
			Reference: filepath.Join(root, "claude.json"),
		}},
		Adapters: []driver.DriverAdapterConfig{{Native: &driver.NativeAdapterConfig{
			Key: "claude-adapter", ID: "sworn.claude", Version: "1.0.0",
			Family:                 driver.ProfileClaude,
			CLI:                    driver.ExecutableIdentity{Path: host},
			RuntimeFiles:           runtimeFiles,
			RequiredRuntimeTargets: required,
			CredentialTarget:       driver.ClaudeCredentialTarget,
			CredentialRefs:         []string{credential},
			MaxCredentialBytes:     1_048_576,
			CLIResolution:          driver.NativeCLIResolutionRunSnapshot,
		}}},
		Profiles: []driver.DriverProfile{{
			Key: "claude", Adapter: "claude-adapter",
			Network:             driver.NetworkRequired,
			CredentialSource:    &credential,
			CertificationModels: []string{"model-claude"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "drivers.json")
	if err := os.WriteFile(configPath, body, 0o600); err != nil {
		t.Fatal(err)
	}
	return configPath, host
}

// doctor and probe resolve a run-snapshot CLI from the host the way a run
// would and report the version it would take, tested or untested as usual.
func TestDriverReadinessReportsTheCLIARunSnapshotWouldTake(t *testing.T) {
	configPath, host := driverSnapshotConfigFixture(t)

	var doctorOut, doctorErr bytes.Buffer
	if code := run([]string{
		"driver", "doctor", "--config", configPath,
		"--profile", "claude", "--model", "model-claude", "--json",
	}, &doctorOut, &doctorErr); code != 0 {
		t.Fatalf("doctor = %d, stdout=%q stderr=%q",
			code, doctorOut.String(), doctorErr.String())
	}
	var readiness driverReadinessOutput
	if err := json.Unmarshal(doctorOut.Bytes(), &readiness); err != nil {
		t.Fatal(err)
	}
	if len(readiness.Reports) != 1 ||
		readiness.Reports[0].State != driver.ReadinessPass ||
		readiness.Reports[0].CLICompatibility == nil ||
		readiness.Reports[0].CLICompatibility.CLIVersion != "2.1.280" ||
		readiness.Reports[0].CLICompatibility.Status != driver.NativeCLIUntested {
		t.Fatalf("doctor output = %s", doctorOut.String())
	}

	var probeOut, probeErr bytes.Buffer
	run([]string{
		"driver", "probe", "--config", configPath,
		"--profile", "claude", "--model", "model-claude", "--json",
	}, &probeOut, &probeErr)
	var probe driverProbeOutput
	if err := json.Unmarshal(probeOut.Bytes(), &probe); err != nil {
		t.Fatalf("probe output = %q, stderr=%q: %v",
			probeOut.String(), probeErr.String(), err)
	}
	if probe.CLICompatibility == nil ||
		probe.CLICompatibility.CLIVersion != "2.1.280" {
		t.Fatalf("probe output = %s", probeOut.String())
	}

	if err := os.Remove(host); err != nil {
		t.Fatal(err)
	}
	var missingOut, missingErr bytes.Buffer
	if code := run([]string{
		"driver", "doctor", "--config", configPath,
		"--profile", "claude", "--model", "model-claude", "--json",
	}, &missingOut, &missingErr); code != 1 ||
		!strings.Contains(
			missingErr.String(), "Technical code: NATIVE_CLI_SNAPSHOT_UNAVAILABLE",
		) {
		t.Fatalf("doctor without a host CLI = %d, stderr=%q",
			code, missingErr.String())
	}
}
