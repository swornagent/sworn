package driver

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func projectionInput(name, relative string, body []byte) (Input, InputContent) {
	input := Input{
		Name:   name,
		Path:   InputProjectionPrefix + relative,
		Digest: Digest(body),
	}
	return input, InputContent{Input: input, Bytes: body}
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
		if _, err := StageInputProjection(
			workspace,
			[]Input{generic},
			[]InputContent{content},
		); !IsCode(err, "INVALID_PRODUCTION_INPUT_PATH") {
			t.Fatalf("error = %v", err)
		}
	})
	for _, kind := range []string{"file", "directory", "symlink"} {
		kind := kind
		t.Run("reserved "+kind, func(t *testing.T) {
			workspace := t.TempDir()
			reserved := filepath.Join(workspace, ".sworn-inputs")
			switch kind {
			case "file":
				if err := os.WriteFile(reserved, []byte("conflict"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Mkdir(reserved, 0o700); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Symlink("elsewhere", reserved); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := StageInputProjection(
				workspace,
				[]Input{first},
				[]InputContent{firstContent},
			); !IsCode(err, "INPUT_PROJECTION_CONFLICT") {
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
