package driver

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/swornagent/sworn/internal/gitx"
)

func projectionInput(name, relative string, body []byte) (Input, InputContent) {
	input := Input{
		Name:   name,
		Path:   relative,
		Digest: Digest(body),
	}
	return input, InputContent{Input: input, Bytes: body}
}

// TestInputProjectionThreadsEngineReservedNames proves the engine-computed
// reserved set reaches the projection admission: a path under a relocated
// records root is admitted under the fixed default set but refused once the
// engine threads its configured reserved names.
func TestInputProjectionThreadsEngineReservedNames(t *testing.T) {
	t.Parallel()
	workspace := t.TempDir()
	first, firstContent := projectionInput("plan", ".records/plan.md", []byte("plan\n"))
	projection, err := StageInputProjection(
		workspace,
		[]Input{first},
		[]InputContent{firstContent},
	)
	if err != nil {
		t.Fatalf("default admission rejected configured-root path: %v", err)
	}
	t.Cleanup(func() { _ = projection.Close() })
	if _, err := StageInputProjection(
		workspace,
		[]Input{first},
		[]InputContent{firstContent},
		[]string{".git", ".records", ".journals"},
	); !IsCode(err, "INVALID_PRODUCTION_INPUT_PATH") {
		t.Fatalf("configured-root projection error = %v, want INVALID_PRODUCTION_INPUT_PATH", err)
	}
}

// TestStageInputProjectionRefusesUnavailableTempRoot is the A2 consumer
// proof for the projection: an invalid or uncreatable configured temp root
// fails the staging instead of silently falling back to the process/system
// temp directory.
func TestStageInputProjectionRefusesUnavailableTempRoot(t *testing.T) {
	input, content := projectionInput("plan", "plan.md", []byte("plan\n"))

	t.Run("invalid override", func(t *testing.T) {
		workspace := t.TempDir()
		t.Setenv(gitx.EnvTempRoot, "relative-tmp")
		if _, err := StageInputProjection(
			workspace,
			[]Input{input},
			[]InputContent{content},
		); err == nil {
			t.Fatal("invalid temp root silently escaped to the system temp directory")
		}
	})
	t.Run("uncreatable root", func(t *testing.T) {
		workspace := t.TempDir()
		blocker := filepath.Join(t.TempDir(), "blocker")
		if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Setenv(gitx.EnvTempRoot, filepath.Join(blocker, "tmp"))
		if _, err := StageInputProjection(
			workspace,
			[]Input{input},
			[]InputContent{content},
		); err == nil {
			t.Fatal("uncreatable temp root silently escaped to the system temp directory")
		}
	})
}

func TestInputProjectionStagesExactOrderedReadOnlyBytes(t *testing.T) {
	t.Parallel()
	workspace := t.TempDir()
	first, firstContent := projectionInput("plan", "plan.md", []byte("plan\n"))
	second, secondContent := projectionInput("status", "nested/status.json", []byte("{}\n"))
	projection, err := StageInputProjection(
		workspace,
		[]Input{first, second},
		[]InputContent{firstContent, secondContent},
	)
	if err != nil {
		t.Fatal(err)
	}
	root := projection.Root()
	for name, want := range map[string][]byte{
		"plan.md":            firstContent.Bytes,
		"nested/status.json": secondContent.Bytes,
	} {
		info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		if !info.Mode().IsRegular() || info.Mode().Perm()&0o222 != 0 {
			t.Fatalf("%s mode = %s", name, info.Mode())
		}
		got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(want) {
			t.Fatalf("%s = %q", name, got)
		}
	}
	if _, err := os.Lstat(filepath.Join(workspace, ".sworn-inputs")); !os.IsNotExist(err) {
		t.Fatal("projection mutated the underlying workspace")
	}
	if err := projection.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("projection root survived cleanup")
	}
}

func TestInputProjectionRejectsOrderDigestPathAndReservedConflicts(t *testing.T) {
	t.Parallel()
	first, firstContent := projectionInput("plan", "plan.md", []byte("plan\n"))
	second, secondContent := projectionInput("status", "status.json", []byte("{}\n"))

	t.Run("order", func(t *testing.T) {
		workspace := t.TempDir()
		if _, err := StageInputProjection(
			workspace,
			[]Input{first, second},
			[]InputContent{secondContent, firstContent},
		); !IsCode(err, "INPUT_BINDING_MISMATCH") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("digest", func(t *testing.T) {
		workspace := t.TempDir()
		changed := firstContent
		changed.Bytes = []byte("changed\n")
		if _, err := StageInputProjection(
			workspace,
			[]Input{first},
			[]InputContent{changed},
		); !IsCode(err, "INPUT_BINDING_MISMATCH") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("per-file limit", func(t *testing.T) {
		workspace := t.TempDir()
		body := make([]byte, MaxInputFileBytes+1)
		input, content := projectionInput("large", "large.bin", body)
		if _, err := StageInputProjection(
			workspace,
			[]Input{input},
			[]InputContent{content},
		); !IsCode(err, "RESOURCE_LIMIT") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("aggregate limit", func(t *testing.T) {
		workspace := t.TempDir()
		var inputs []Input
		var contents []InputContent
		for index := 0; index < 9; index++ {
			body := make([]byte, MaxInputFileBytes)
			body[0] = byte(index)
			input, content := projectionInput(
				fmt.Sprintf("input-%d", index),
				fmt.Sprintf("input-%d.bin", index),
				body,
			)
			inputs = append(inputs, input)
			contents = append(contents, content)
		}
		if _, err := StageInputProjection(
			workspace,
			inputs,
			contents,
		); !IsCode(err, "RESOURCE_LIMIT") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("generic portable path", func(t *testing.T) {
		workspace := t.TempDir()
		generic := first
		generic.Path = "docs/plan.md"
		content := InputContent{Input: generic, Bytes: firstContent.Bytes}
		projection, err := StageInputProjection(
			workspace,
			[]Input{generic},
			[]InputContent{content},
		)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = projection.Close() })
	})
	for name, setup := range map[string]func(string) error{
		"absolute": func(workspace string) error {
			return os.Symlink("/etc/passwd", filepath.Join(workspace, "escape"))
		},
		"escaping": func(workspace string) error {
			return os.Symlink("../outside", filepath.Join(workspace, "escape"))
		},
		"reserved alias": func(workspace string) error {
			if err := os.MkdirAll(filepath.Join(workspace, ".baton", "releases"), 0o700); err != nil {
				return err
			}
			return os.Symlink(".baton/releases", filepath.Join(workspace, "records"))
		},
	} {
		setup := setup
		t.Run("unsafe symlink "+name, func(t *testing.T) {
			workspace := t.TempDir()
			if err := setup(workspace); err != nil {
				t.Fatal(err)
			}
			if _, err := StageInputProjection(
				workspace,
				[]Input{first},
				[]InputContent{firstContent},
			); !IsCode(err, "UNSAFE_WORKSPACE_SYMLINK") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestWorkspaceManifestDetectsFileModeAndSymlinkMutation(t *testing.T) {
	t.Parallel()
	workspace := t.TempDir()
	file := filepath.Join(workspace, "file")
	if err := os.WriteFile(file, []byte("before\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := captureWorkspaceManifest(workspace)
	if err != nil {
		t.Fatal(err)
	}
	after, err := captureWorkspaceManifest(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if !equalManifest(before, after) {
		t.Fatal("unchanged manifests differ")
	}
	if err := os.Chmod(file, 0o400); err != nil {
		t.Fatal(err)
	}
	after, err = captureWorkspaceManifest(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if equalManifest(before, after) {
		t.Fatal("mode mutation was not detected")
	}
}

func TestWorkspaceManifestStatesPersistentDeltaNotTransientWriteAudit(t *testing.T) {
	t.Parallel()
	workspace := t.TempDir()
	file := filepath.Join(workspace, "file")
	original := []byte("original\n")
	if err := os.WriteFile(file, original, 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := captureWorkspaceManifest(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("transient\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, original, 0o600); err != nil {
		t.Fatal(err)
	}
	after, err := captureWorkspaceManifest(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if !equalManifest(before, after) {
		t.Fatal("restored bytes should demonstrate the documented observation limit")
	}
}
