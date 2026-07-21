package protocol

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/swornagent/sworn/internal/executor"
)

func TestLocalCheckDefinitionAllowsStrictNonCanonicalJSON(t *testing.T) {
	t.Parallel()
	contents := []byte(`{
		"schema_version":"sworn-local-check-v1",
		"argv":["/usr/bin/true"],
		"working_directory":".",
		"timeout_seconds":1,
		"evidence":{"id":"e","acceptance_ids":["AC1"],"boundary":"component","uses_mocks":false,"observed":"ok"}
	}`)
	if _, err := ParseLocalCheckDefinition(contents); err != nil {
		t.Fatalf("strict non-canonical definition: %v", err)
	}
}

func TestLocalEnvironmentRuntimeSchemasAreClosed(t *testing.T) {
	t.Parallel()
	v1 := validLocalEnvironment()
	v1Bytes, err := EncodeCanonical(v1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseLocalEnvironment(append([]byte(" "), v1Bytes...)); err == nil {
		t.Fatal("non-canonical local environment was accepted")
	}
	if parsed, err := ParseLocalEnvironment(v1Bytes); err != nil ||
		parsed.SchemaVersion != LocalEnvironmentSchemaVersion ||
		bytes.Contains(v1Bytes, []byte("runtime_manifest_digest")) {
		t.Fatalf("legacy environment = %#v, %v", parsed, err)
	}
	v2 := v1
	v2.SchemaVersion = ContentEnvironmentSchemaVersion
	v2.RuntimeManifestDigest = testProtocolDigest("b")
	v2Bytes, err := EncodeCanonical(v2)
	if err != nil {
		t.Fatal(err)
	}
	if parsed, err := ParseLocalEnvironment(v2Bytes); err != nil ||
		parsed.RuntimeManifestDigest != v2.RuntimeManifestDigest {
		t.Fatalf("content environment = %#v, %v", parsed, err)
	}

	for name, mutate := range map[string]func(*LocalEnvironment){
		"v1 with digest": func(value *LocalEnvironment) { value.RuntimeManifestDigest = testProtocolDigest("c") },
		"v2 missing digest": func(value *LocalEnvironment) {
			value.SchemaVersion = ContentEnvironmentSchemaVersion
		},
		"v2 malformed digest": func(value *LocalEnvironment) {
			value.SchemaVersion, value.RuntimeManifestDigest = ContentEnvironmentSchemaVersion, "sha256:no"
		},
		"unknown schema": func(value *LocalEnvironment) { value.SchemaVersion = "sworn-local-environment-v3" },
		"hermetic overclaim": func(value *LocalEnvironment) {
			value.SchemaVersion, value.RuntimeManifestDigest = ContentEnvironmentSchemaVersion, testProtocolDigest("d")
			value.HermeticToolchain = true
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := v1
			mutate(&changed)
			encoded, err := EncodeCanonical(changed)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ParseLocalEnvironment(encoded); err == nil {
				t.Fatal("invalid local environment was accepted")
			}
		})
	}

	var unknown map[string]any
	if err := json.Unmarshal(v2Bytes, &unknown); err != nil {
		t.Fatal(err)
	}
	unknown["surprise"] = true
	unknownBytes, err := EncodeCanonical(unknown)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseLocalEnvironment(unknownBytes); err == nil {
		t.Fatal("unknown local environment field was accepted")
	}
	for name, field := range map[string]any{"empty": "", "null": nil} {
		t.Run("v1 explicit "+name+" runtime field", func(t *testing.T) {
			var legacy map[string]any
			if err := json.Unmarshal(v1Bytes, &legacy); err != nil {
				t.Fatal(err)
			}
			legacy["runtime_manifest_digest"] = field
			encoded, err := EncodeCanonical(legacy)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ParseLocalEnvironment(encoded); err == nil {
				t.Fatal("v1 environment carried a v2-only field")
			}
		})
	}
}

func validLocalEnvironment() LocalEnvironment {
	return LocalEnvironment{
		SchemaVersion: LocalEnvironmentSchemaVersion, ProtocolSnapshotDigest: testProtocolDigest("a"),
		EngineRuntime: "go1.26", OS: "linux", Architecture: "amd64",
		Executor: LocalExecutorProbe{
			BubblewrapVersion: "bubblewrap 0.9.0", SystemdVersion: "systemd 255", CgroupV2: true,
			UserManager: "running", Controllers: []string{"cpu", "memory", "pids"},
		},
		ExecutorPolicyVersion: executor.ContainmentPolicyVersion,
		Limits: LocalExecutionLimits{
			RuntimeNanoseconds: 1, MemoryBytes: 1, Tasks: 1, CPUPercent: 1, FileBytes: 1,
			TempBytes: 1, HomeBytes: 1, InputBytes: 1, WorkspaceBytes: 1, StdoutBytes: 1, StderrBytes: 1,
		},
		RuntimeTrustRoot: "/usr", HermeticToolchain: false,
		WorkspaceAccess: "read_only", Network: "none",
	}
}

func testProtocolDigest(character string) string {
	return "sha256:" + string(bytes.Repeat([]byte(character), 64))
}
