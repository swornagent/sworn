package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/driver"
	runtimepkg "github.com/swornagent/sworn/internal/runtime"
)

func manifestCanonicalDriverBytes(t *testing.T) []byte {
	t.Helper()
	credential := "openai-environment"
	body, err := driver.EncodeDriverConfig(driver.DriverConfig{
		SchemaVersion: driver.DriverConfigSchemaVersion,
		Credentials: []driver.DriverCredentialSource{{
			Key:       credential,
			Kind:      driver.CredentialEnvironment,
			Reference: "SWORN_TEST_OPENAI_KEY",
		}},
		Adapters: []driver.DriverAdapterConfig{{
			OpenAI: &driver.OpenAIProfileConfig{
				HTTPProfileConfig: driver.HTTPProfileConfig{
					Key:              "openai-adapter",
					ID:               "sworn.openai",
					Version:          "1.0.0",
					Endpoint:         "https://example.invalid/v1/responses",
					CredentialHeader: "Authorization",
					CredentialPrefix: "Bearer ",
					CredentialRefs:   []string{credential},
					ResponseBytes:    driver.MaxProviderResponseBytes,
				},
				API:             driver.OpenAIResponsesAPI,
				ReasoningEffort: "medium",
			},
		}},
		Profiles: []driver.DriverProfile{{
			Key:                 "openai",
			Adapter:             "openai-adapter",
			Network:             driver.NetworkRequired,
			CredentialSource:    &credential,
			CertificationModels: []string{"model-one"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := driver.DecodeDriverConfig(body); err != nil {
		t.Fatalf("driver fixture: %v", err)
	}
	return body
}

func TestManifestCanonicalRuntimePrintsExactBytes(t *testing.T) {
	t.Parallel()
	canonical := operatorManifestBody(t, "run-manifest-canonical", "canonical bytes")
	var decoded map[string]any
	if err := json.Unmarshal(canonical, &decoded); err != nil {
		t.Fatal(err)
	}
	pretty, err := json.MarshalIndent(decoded, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	pretty = append(pretty, '\n')
	if _, err := runtimepkg.ParseManifest(pretty); err == nil || !runtimepkg.IsCode(err, "NONCANONICAL_MANIFEST") {
		t.Fatalf("pretty err = %v, want NONCANONICAL_MANIFEST", err)
	}
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(path, pretty, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"manifest", "canonical", "--manifest", path}, &stdout, &stderr); code != 0 {
		t.Fatalf("manifest canonical = %d, stderr=%s", code, stderr.String())
	}
	if !bytes.Equal(stdout.Bytes(), canonical) {
		t.Fatalf("stdout = %s, want canonical %s", stdout.Bytes(), canonical)
	}
	if !bytes.HasSuffix(stdout.Bytes(), []byte("\n")) || bytes.HasSuffix(stdout.Bytes(), []byte("\n\n")) {
		t.Fatalf("runtime canonical must end with exactly one newline: %q", stdout.Bytes())
	}
	if _, err := runtimepkg.ParseManifest(stdout.Bytes()); err != nil {
		t.Fatalf("canonical output not admitted: %v", err)
	}
	if !strings.Contains(stderr.String(), "printed canonical manifest to stdout (--manifest)") {
		t.Fatalf("stderr missing printed line: %q", stderr.String())
	}
	if strings.Contains(stderr.String(), path) {
		t.Fatalf("stderr echoed path: %q", stderr.String())
	}
	// Already-canonical input is byte-identical (digest unchanged).
	canonicalPath := filepath.Join(t.TempDir(), "canonical.json")
	if err := os.WriteFile(canonicalPath, canonical, 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"manifest", "canonical", "--manifest", canonicalPath}, &stdout, &stderr); code != 0 {
		t.Fatalf("canonical input = %d, stderr=%s", code, stderr.String())
	}
	if !bytes.Equal(stdout.Bytes(), canonical) {
		t.Fatalf("already-canonical stdout differs")
	}
	if got, want := driver.Digest(stdout.Bytes()), driver.Digest(canonical); got != want {
		t.Fatalf("digest changed: %s vs %s", got, want)
	}
}

func TestManifestCanonicalRuntimeRefusesValidationWithAdmissionCode(t *testing.T) {
	t.Parallel()
	canonical := operatorManifestBody(t, "run-manifest-invalid", "invalid")
	var decoded map[string]any
	if err := json.Unmarshal(canonical, &decoded); err != nil {
		t.Fatal(err)
	}
	decoded["run_id"] = "INVALID RUN WITH SPACES"
	pretty, err := json.MarshalIndent(decoded, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	pretty = append(pretty, '\n')
	path := filepath.Join(t.TempDir(), "invalid.json")
	if err := os.WriteFile(path, pretty, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"manifest", "canonical", "--manifest", path}, &stdout, &stderr); code != 1 {
		t.Fatalf("invalid manifest = %d, want 1; stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout must be empty on refusal: %q", stdout.String())
	}
	out := stderr.String()
	if !strings.Contains(out, "Technical code: INVALID_RUN") {
		t.Fatalf("stderr missing INVALID_RUN:\n%s", out)
	}
	if strings.Contains(out, "NONCANONICAL") {
		t.Fatalf("validation refusal must not be NONCANONICAL:\n%s", out)
	}
	if strings.Contains(out, "INVALID RUN") || strings.Contains(out, path) {
		t.Fatalf("stderr echoed value or path:\n%s", out)
	}
}

func TestManifestCanonicalRuntimeLegacyVersions(t *testing.T) {
	t.Parallel()
	canonical := operatorManifestBody(t, "run-manifest-legacy", "legacy")
	for _, tc := range []struct {
		version string
		code    string
	}{
		{"sworn.runtime-manifest/v2", "MIGRATION_REQUIRED"},
		{"sworn.runtime-manifest/v3", "MIGRATION_REQUIRED"},
		{"sworn.runtime-manifest/v4", "MIGRATION_REQUIRED"},
		{"sworn.runtime-manifest/v99", "INVALID_MANIFEST_VERSION"},
	} {
		var decoded map[string]any
		if err := json.Unmarshal(canonical, &decoded); err != nil {
			t.Fatal(err)
		}
		decoded["schema_version"] = tc.version
		pretty, err := json.MarshalIndent(decoded, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		pretty = append(pretty, '\n')
		// Direct helper check.
		if _, err := runtimepkg.CanonicalManifestBytes(pretty); !runtimepkg.IsCode(err, tc.code) {
			t.Fatalf("version %s helper err = %v, want %s", tc.version, err, tc.code)
		}
		// CLI check.
		path := filepath.Join(t.TempDir(), "legacy.json")
		if err := os.WriteFile(path, pretty, 0o600); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		if code := run([]string{"manifest", "canonical", "--manifest", path}, &stdout, &stderr); code != 1 {
			t.Fatalf("version %s cli = %d, want 1", tc.version, code)
		}
		if !strings.Contains(stderr.String(), "Technical code: "+tc.code) {
			t.Fatalf("version %s stderr missing %s:\n%s", tc.version, tc.code, stderr.String())
		}
		if stdout.Len() != 0 {
			t.Fatalf("version %s stdout must be empty", tc.version)
		}
	}
}

func TestManifestCanonicalRuntimeWriteIsAtomicStableAndPreservesMode(t *testing.T) {
	t.Parallel()
	canonical := operatorManifestBody(t, "run-manifest-write", "write")
	var decoded map[string]any
	if err := json.Unmarshal(canonical, &decoded); err != nil {
		t.Fatal(err)
	}
	pretty, err := json.MarshalIndent(decoded, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	pretty = append(pretty, '\n')
	for _, mode := range []os.FileMode{0o600, 0o644} {
		dir := t.TempDir()
		path := filepath.Join(dir, "manifest.json")
		if err := os.WriteFile(path, pretty, mode); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		if code := run([]string{"manifest", "canonical", "--manifest", path, "--write"}, &stdout, &stderr); code != 0 {
			t.Fatalf("mode %o write = %d, stderr=%s", mode, code, stderr.String())
		}
		if stdout.Len() != 0 {
			t.Fatalf("mode %o --write stdout must be empty: %q", mode, stdout.String())
		}
		if !strings.Contains(stderr.String(), "wrote canonical manifest (--manifest)") {
			t.Fatalf("mode %o stderr missing wrote line: %q", mode, stderr.String())
		}
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(body, canonical) {
			t.Fatalf("mode %o file not canonical", mode)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != mode.Perm() {
			t.Fatalf("mode %o preserved as %o", mode, info.Mode().Perm())
		}
		// Second run is stable.
		stdout.Reset()
		stderr.Reset()
		if code := run([]string{"manifest", "canonical", "--manifest", path, "--write"}, &stdout, &stderr); code != 0 {
			t.Fatalf("mode %o second write = %d", mode, code)
		}
		second, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(second, canonical) {
			t.Fatalf("mode %o second write changed bytes", mode)
		}
		// No sibling temp left behind.
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".sworn-") {
				t.Fatalf("mode %o temp left behind: %s", mode, entry.Name())
			}
		}
	}
}

func TestManifestCanonicalRefusesSymlinkAndNonRegular(t *testing.T) {
	t.Parallel()
	canonical := operatorManifestBody(t, "run-manifest-symlink", "symlink")
	dir := t.TempDir()
	target := filepath.Join(dir, "target.json")
	if err := os.WriteFile(target, canonical, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"manifest", "canonical", "--manifest", link},
		{"manifest", "canonical", "--manifest", link, "--write"},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code != 1 {
			t.Fatalf("symlink %v = %d, want 1", args, code)
		}
		if stdout.Len() != 0 {
			t.Fatalf("symlink %v stdout must be empty", args)
		}
		if strings.Contains(stderr.String(), link) || strings.Contains(stderr.String(), target) {
			t.Fatalf("symlink %v echoed path:\n%s", args, stderr.String())
		}
	}
	body, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body, canonical) {
		t.Fatalf("symlink write clobbered target")
	}
	// Non-regular target: a directory.
	subdir := filepath.Join(dir, "subdir")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"manifest", "canonical", "--manifest", subdir}, &stdout, &stderr); code != 1 {
		t.Fatalf("directory read = %d, want 1", code)
	}
	if code := run([]string{"manifest", "canonical", "--manifest", subdir, "--write"}, &stdout, &stderr); code != 1 {
		t.Fatalf("directory write path read = %d, want 1", code)
	}
}

func TestManifestCanonicalDriverPrintsExactBytes(t *testing.T) {
	t.Parallel()
	canonical := manifestCanonicalDriverBytes(t)
	if bytes.HasSuffix(canonical, []byte("\n")) {
		t.Fatalf("driver canonical must not end with newline: %q", canonical)
	}
	var decoded map[string]any
	if err := json.Unmarshal(canonical, &decoded); err != nil {
		t.Fatal(err)
	}
	pretty, err := json.MarshalIndent(decoded, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	pretty = append(pretty, '\n')
	if _, err := driver.DecodeDriverConfig(pretty); !driver.IsCode(err, "NONCANONICAL_JSON") {
		t.Fatalf("pretty driver err = %v, want NONCANONICAL_JSON", err)
	}
	path := filepath.Join(t.TempDir(), "drivers.json")
	if err := os.WriteFile(path, pretty, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"manifest", "canonical", "--driver-config", path}, &stdout, &stderr); code != 0 {
		t.Fatalf("driver canonical = %d, stderr=%s", code, stderr.String())
	}
	if !bytes.Equal(stdout.Bytes(), canonical) {
		t.Fatalf("driver stdout differs:\n%s\nwant:\n%s", stdout.Bytes(), canonical)
	}
	if bytes.HasSuffix(stdout.Bytes(), []byte("\n")) {
		t.Fatalf("driver canonical must have no trailing newline")
	}
	if _, err := driver.DecodeDriverConfig(stdout.Bytes()); err != nil {
		t.Fatalf("driver canonical output not admitted: %v", err)
	}
	if !strings.Contains(stderr.String(), "printed canonical driver config to stdout (--driver-config)") {
		t.Fatalf("stderr missing printed line: %q", stderr.String())
	}
	if got, want := driver.Digest(stdout.Bytes()), driver.Digest(canonical); got != want {
		t.Fatalf("driver digest changed")
	}
	// Already-canonical byte-identical.
	canonicalPath := filepath.Join(t.TempDir(), "canonical.json")
	if err := os.WriteFile(canonicalPath, canonical, 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"manifest", "canonical", "--driver-config", canonicalPath}, &stdout, &stderr); code != 0 {
		t.Fatalf("already-canonical driver = %d", code)
	}
	if !bytes.Equal(stdout.Bytes(), canonical) {
		t.Fatalf("already-canonical driver stdout differs")
	}
}

func TestManifestCanonicalDriverRefusesValidationWithAdmissionCode(t *testing.T) {
	t.Parallel()
	canonical := manifestCanonicalDriverBytes(t)
	unknown := bytes.Replace(
		canonical,
		[]byte(`"schema_version":"sworn.driver-config/v1"`),
		[]byte(`"schema_version":"sworn.driver-config/v1","fallback_profile":"other"`),
		1,
	)
	path := filepath.Join(t.TempDir(), "unknown.json")
	if err := os.WriteFile(path, unknown, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"manifest", "canonical", "--driver-config", path}, &stdout, &stderr); code != 1 {
		t.Fatalf("unknown field = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "Technical code: UNKNOWN_FIELD") {
		t.Fatalf("stderr missing UNKNOWN_FIELD:\n%s", stderr.String())
	}
	if strings.Contains(stderr.String(), "NONCANONICAL") {
		t.Fatalf("validation refusal must not be NONCANONICAL:\n%s", stderr.String())
	}
	duplicate := bytes.Replace(
		canonical,
		[]byte(`"schema_version":"sworn.driver-config/v1"`),
		[]byte(`"schema_version":"sworn.driver-config/v1","schema_version":"sworn.driver-config/v1"`),
		1,
	)
	dupPath := filepath.Join(t.TempDir(), "dup.json")
	if err := os.WriteFile(dupPath, duplicate, 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"manifest", "canonical", "--driver-config", dupPath}, &stdout, &stderr); code != 1 {
		t.Fatalf("duplicate = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "Technical code: DUPLICATE_NAME") {
		t.Fatalf("stderr missing DUPLICATE_NAME:\n%s", stderr.String())
	}
}

func TestManifestCanonicalDriverWriteIsAtomicStableAndPreservesMode(t *testing.T) {
	t.Parallel()
	canonical := manifestCanonicalDriverBytes(t)
	var decoded map[string]any
	if err := json.Unmarshal(canonical, &decoded); err != nil {
		t.Fatal(err)
	}
	pretty, err := json.MarshalIndent(decoded, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	pretty = append(pretty, '\n')
	dir := t.TempDir()
	path := filepath.Join(dir, "drivers.json")
	if err := os.WriteFile(path, pretty, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"manifest", "canonical", "--driver-config", path, "--write"}, &stdout, &stderr); code != 0 {
		t.Fatalf("driver write = %d, stderr=%s", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("--write stdout must be empty")
	}
	if !strings.Contains(stderr.String(), "wrote canonical driver config (--driver-config)") {
		t.Fatalf("stderr missing wrote line: %q", stderr.String())
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body, canonical) {
		t.Fatalf("driver file not canonical")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("driver mode preserved as %o, want 600", info.Mode().Perm())
	}
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"manifest", "canonical", "--driver-config", path, "--write"}, &stdout, &stderr); code != 0 {
		t.Fatalf("second driver write = %d", code)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(second, canonical) {
		t.Fatalf("second driver write changed bytes")
	}
}

func TestManifestCanonicalRejectsBadShapes(t *testing.T) {
	t.Parallel()
	canonical := operatorManifestBody(t, "run-manifest-shape", "shape")
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(path, canonical, 0o600); err != nil {
		t.Fatal(err)
	}
	driverPath := filepath.Join(t.TempDir(), "drivers.json")
	if err := os.WriteFile(driverPath, manifestCanonicalDriverBytes(t), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"manifest", "canonical"},
		{"manifest", "canonical", "--manifest", path, "--driver-config", driverPath},
		{"manifest", "canonical", "--manifest", "relative.json"},
		{"manifest", "canonical", "--driver-config", "relative.json"},
		{"manifest", "canonical", "--manifest", path, "--write", "--write"},
		{"manifest", "canonical", "--manifest", "--write"},
		{"manifest", "canonical", "--unknown", path},
		{"manifest"},
		{"manifest", "bogus", "--manifest", path},
	} {
		var stdout, stderr bytes.Buffer
		code := run(args, &stdout, &stderr)
		if code != 2 {
			t.Fatalf("run(%v) = %d, want 2; stderr=%s", args, code, stderr.String())
		}
		if stdout.Len() != 0 {
			t.Fatalf("run(%v) stdout must be empty: %q", args, stdout.String())
		}
	}
}

func TestManifestIsRegisteredInUsageAndDispatch(t *testing.T) {
	t.Parallel()
	if !strings.Contains(usage, "manifest ") {
		t.Fatalf("usage does not mention manifest")
	}
	if !strings.Contains(usage, "sworn manifest canonical") {
		t.Fatalf("usage does not mention manifest canonical")
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"manifest"}, &stdout, &stderr); code != 2 {
		t.Fatalf("run(manifest) = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "sworn manifest") {
		t.Fatalf("run(manifest) stderr = %q", stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"manifest", "bogus"}, &stdout, &stderr); code != 2 {
		t.Fatalf("run(manifest bogus) = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "unknown verb") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}
