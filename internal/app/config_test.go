package app

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/executor"
	"github.com/swornagent/sworn/internal/repo"
)

func TestParseConfigAcceptsCompleteSecretFreeSchemaAndFixedDefaults(t *testing.T) {
	t.Parallel()

	encoded := encodeRunConfig(t, validRunConfig())
	configuration, err := ParseConfig(encoded)
	if err != nil {
		t.Fatal(err)
	}
	limits, err := configuration.executorLimits()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(limits, executor.DefaultLimits()) {
		t.Fatalf("default limits = %#v, want %#v", limits, executor.DefaultLimits())
	}
	sources, err := configuration.authoritySources()
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 || sources[0].SourceRef != "authority-source-1" ||
		sources[0].AuthorizerRef != "key-1" || len(sources[0].PublicKey) != 32 {
		t.Fatalf("authority sources = %#v", sources)
	}
	if bytes.Contains(encoded, []byte("secret-value")) {
		t.Fatal("parsed configuration fixture unexpectedly contains a credential value")
	}
}

func TestParseConfigRejectsSchemaLoopholes(t *testing.T) {
	t.Parallel()

	base := validRunConfig()
	valid := encodeRunConfig(t, base)
	withTopMember := mutateConfigObject(t, valid, func(top map[string]any) {
		top["api_key"] = "secret-value"
	})
	withCodexMember := mutateConfigObject(t, valid, func(top map[string]any) {
		top["codex"].(map[string]any)["provider"] = "ambient-provider"
	})
	partialLimits := mutateConfigObject(t, valid, func(top map[string]any) {
		top["executor"].(map[string]any)["limits"] = map[string]any{"runtime_seconds": 300}
	})
	nullLimits := mutateConfigObject(t, valid, func(top map[string]any) {
		top["executor"].(map[string]any)["limits"] = nil
	})
	badPublicKey := mutateConfigObject(t, valid, func(top map[string]any) {
		source := top["authority"].(map[string]any)["sources"].([]any)[0].(map[string]any)
		source["public_key"] = base64.StdEncoding.EncodeToString(make([]byte, 31))
	})
	relativeControl := mutateConfigObject(t, valid, func(top map[string]any) {
		top["control_database"] = "control.db"
	})
	duplicate := bytes.Replace(valid, []byte(`"schema_version":"sworn-run-config-v1"`),
		[]byte(`"schema_version":"sworn-run-config-v1","schema_version":"sworn-run-config-v1"`), 1)

	for _, test := range []struct {
		name    string
		encoded []byte
		want    string
	}{
		{"secret member", withTopMember, "unknown field"},
		{"provider member", withCodexMember, "unknown field"},
		{"partial limits", partialLimits, "missing required field"},
		{"null limits", nullLimits, "omitted or a complete object"},
		{"public key length", badPublicKey, "Ed25519 public key"},
		{"relative control", relativeControl, "clean absolute path"},
		{"duplicate member", duplicate, "duplicate object name"},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := ParseConfig(test.encoded); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ParseConfig() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestParseConfigAcceptsCompleteExplicitLimits(t *testing.T) {
	t.Parallel()

	configuration := validRunConfig()
	defaults := executor.DefaultLimits()
	configuration.Executor.Limits = &ExecutorLimits{
		RuntimeSeconds: uint64(defaults.Runtime.Seconds()),
		MemoryBytes:    defaults.MemoryBytes, SwapBytes: defaults.SwapBytes,
		Tasks: defaults.Tasks, CPUPercent: defaults.CPUPercent,
		FileBytes: defaults.FileBytes, TempBytes: defaults.TempBytes,
		HomeBytes: defaults.HomeBytes, InputBytes: defaults.InputBytes,
		WorkspaceBytes: defaults.WorkspaceBytes,
		StdoutBytes:    uint64(defaults.StdoutBytes), StderrBytes: uint64(defaults.StderrBytes),
	}
	parsed, err := ParseConfig(encodeRunConfig(t, configuration))
	if err != nil {
		t.Fatal(err)
	}
	limits, err := parsed.executorLimits()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(limits, defaults) {
		t.Fatalf("explicit limits = %#v, want %#v", limits, defaults)
	}
}

func TestLoadConfigRequiresExactPrivateRegularFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "run.json")
	if err := os.WriteFile(path, encodeRunConfig(t, validRunConfig()), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(path); err != nil {
		t.Fatalf("LoadConfig(private) = %v", err)
	}
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(path); err == nil || !strings.Contains(err.Error(), "private") {
		t.Fatalf("LoadConfig(exposed) error = %v", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias.json")
	if err := os.Symlink(path, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(alias); err == nil || !strings.Contains(err.Error(), "symbolic-link remap") {
		t.Fatalf("LoadConfig(alias) error = %v", err)
	}
	realParent := filepath.Join(root, "real-parent")
	if err := os.Mkdir(realParent, 0o700); err != nil {
		t.Fatal(err)
	}
	parentConfig := filepath.Join(realParent, "run.json")
	if err := os.WriteFile(parentConfig, encodeRunConfig(t, validRunConfig()), 0o600); err != nil {
		t.Fatal(err)
	}
	aliasParent := filepath.Join(root, "alias-parent")
	if err := os.Symlink(realParent, aliasParent); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(filepath.Join(aliasParent, "run.json")); err == nil ||
		!strings.Contains(err.Error(), "symbolic-link remap") {
		t.Fatalf("LoadConfig(parent alias) error = %v", err)
	}
}

func validRunConfig() Config {
	return Config{
		SchemaVersion:   RunConfigSchemaVersion,
		ControlDatabase: "/srv/sworn/control.db",
		Repository: RepositoryConfig{
			Root: "/srv/repository",
			Binding: repo.Binding{
				SchemaVersion: repo.BindingSchemaVersion,
				RepositoryID:  "repo-1", CommonDir: "/srv/repository/.git",
				ObjectDir: "/srv/repository/.git/objects", ObjectFormat: "sha1",
			},
		},
		Authority: AuthorityConfig{Sources: []AuthoritySource{{
			SourceRef: "authority-source-1", AuthorizerRef: "key-1",
			PublicKey:       base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32)),
			BundleDirectory: "/srv/sworn/authority",
		}}},
		Executor: ExecutorConfig{
			RuntimeRoot: "/srv/sworn/executor", WritableRoot: "/run/user/1000/sworn-writable",
			Bubblewrap: "/usr/bin/bwrap", SystemdRun: "/usr/bin/systemd-run",
			Systemctl: "/usr/bin/systemctl",
		},
		ContentRuntime: ContentRuntime{
			Source: "/srv/sworn/runtime", Digest: "sha256:" + strings.Repeat("a", 64),
			MaximumBytes: 1 << 30,
		},
		Workspaces: WorkspaceConfig{
			BuilderRoot: "/srv/sworn/builder", CheckRoot: "/srv/sworn/checks",
		},
		Codex: CodexConfig{
			Binary: "/srv/sworn/bin/codex", Model: "gpt-5.4",
			TimeoutSeconds: 300, CredentialEnvironment: "OPENAI_API_KEY",
		},
	}
}

func encodeRunConfig(t *testing.T, configuration Config) []byte {
	t.Helper()
	encoded, err := json.Marshal(configuration)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func mutateConfigObject(t *testing.T, encoded []byte, mutate func(map[string]any)) []byte {
	t.Helper()
	var object map[string]any
	if err := json.Unmarshal(encoded, &object); err != nil {
		t.Fatal(err)
	}
	mutate(object)
	changed, err := json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	return changed
}
