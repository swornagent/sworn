package main

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"testing/fstest"

	"github.com/swornagent/sworn/internal/baton"
)

func TestVerifyReportsExactRC5Admission(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	if code := run([]string{"verify"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run = %d, stderr = %q", code, stderr.String())
	}
	var got verification
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Schema != verificationSchema || got.Identity.PackageVersion != baton.PackageVersion ||
		got.VectorFiles != 4 || got.VectorBytes <= 0 ||
		len(got.CorpusManifest) != len("sha256:")+64 {
		t.Fatalf("verification = %#v", got)
	}
	var second bytes.Buffer
	if code := run([]string{"verify"}, &second, &stderr); code != 0 {
		t.Fatalf("second run = %d", code)
	}
	if !bytes.Equal(stdout.Bytes(), second.Bytes()) {
		t.Fatal("verification output is not deterministic")
	}
}

func TestCorpusRejectsInventoryAndByteMutation(t *testing.T) {
	t.Parallel()
	copyCorpus := func() fstest.MapFS {
		result := make(fstest.MapFS)
		err := fs.WalkDir(embeddedCorpus, ".", func(name string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil || entry.IsDir() {
				return walkErr
			}
			body, err := fs.ReadFile(embeddedCorpus, name)
			if err != nil {
				return err
			}
			result[name] = &fstest.MapFile{Data: append([]byte(nil), body...), Mode: 0o444}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	pkg, err := baton.Load()
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(fstest.MapFS){
		"missing": func(files fstest.MapFS) { delete(files, corpusRoot+"/actions.json") },
		"extra": func(files fstest.MapFS) {
			files[corpusRoot+"/extra.json"] = &fstest.MapFile{Data: []byte("{}\n")}
		},
		"changed": func(files fstest.MapFS) {
			files[corpusRoot+"/state.json"].Data[0] = '['
		},
	} {
		t.Run(name, func(t *testing.T) {
			files := copyCorpus()
			mutate(files)
			if _, _, _, err := verifyCorpus(files, corpusRoot, pkg); err == nil {
				t.Fatal("mutated corpus was admitted")
			}
		})
	}
}

func TestOracleTwinGenerationIsByteModeAndInventoryIdentical(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	node, err = filepath.Abs(node)
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	first := filepath.Join(t.TempDir(), "first")
	second := filepath.Join(t.TempDir(), "second")
	if err := generateCorpus(root, node, first); err != nil {
		t.Fatal(err)
	}
	if err := generateCorpus(root, node, second); err != nil {
		t.Fatal(err)
	}
	inventory := func(directory string) []string {
		entries, err := os.ReadDir(directory)
		if err != nil {
			t.Fatal(err)
		}
		result := make([]string, 0, len(entries))
		for _, entry := range entries {
			info, err := entry.Info()
			if err != nil {
				t.Fatal(err)
			}
			result = append(result, entry.Name()+" "+info.Mode().Perm().String())
		}
		sort.Strings(result)
		return result
	}
	if left, right := inventory(first), inventory(second); !reflect.DeepEqual(left, right) {
		t.Fatalf("inventory/modes differ: %v != %v", left, right)
	}
	for _, name := range []string{"actions.json", "git.json", "manifest.json", "receipts.json", "state.json"} {
		left, err := os.ReadFile(filepath.Join(first, name))
		if err != nil {
			t.Fatal(err)
		}
		right, err := os.ReadFile(filepath.Join(second, name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(left, right) {
			t.Fatalf("%s differs across independent generations", name)
		}
	}
}

func TestGenerateRequiresLiteralAbsoluteInputs(t *testing.T) {
	t.Parallel()
	if err := generateCorpus("relative", "/absolute/node", filepath.Join(t.TempDir(), "out")); err == nil {
		t.Fatal("relative root was accepted")
	}
	if err := generateCorpus(t.TempDir(), "node", filepath.Join(t.TempDir(), "out")); err == nil {
		t.Fatal("PATH lookup was accepted")
	}
}
