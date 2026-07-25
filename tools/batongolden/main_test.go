package main

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"reflect"
	"testing"
	"testing/fstest"

	"github.com/swornagent/sworn/internal/baton"
)

func TestVerifyReportsExactAdmission(t *testing.T) {
	t.Parallel()

	var firstOut, firstErr bytes.Buffer
	if code := run([]string{"verify"}, &firstOut, &firstErr); code != 0 {
		t.Fatalf("run() = %d, stderr = %q", code, firstErr.String())
	}
	var got verification
	if err := json.Unmarshal(firstOut.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	want := verification{
		Schema: goldenSchema,
		Identity: baton.Identity{
			PackageVersion:       baton.PackageVersion,
			TagName:              baton.TagName,
			TagObject:            baton.TagObject,
			Commit:               baton.Commit,
			Tree:                 baton.Tree,
			ArchiveSHA256:        baton.ArchiveSHA256,
			SupportPackageSHA256: baton.SupportPackageSHA256,
			ManifestSHA256:       baton.ManifestSHA256,
			AssetCount:           baton.AssetCount,
			AssetBytes:           baton.AssetBytes,
		},
		CorpusManifest: got.CorpusManifest,
		VectorFiles:    4,
		VectorBytes:    82918,
	}
	if len(got.CorpusManifest) != len("sha256:")+64 {
		t.Fatalf("corpus manifest = %q", got.CorpusManifest)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("verification = %#v, want %#v", got, want)
	}

	var secondOut, secondErr bytes.Buffer
	if code := run([]string{"verify"}, &secondOut, &secondErr); code != 0 {
		t.Fatalf("second run() = %d, stderr = %q", code, secondErr.String())
	}
	if !bytes.Equal(firstOut.Bytes(), secondOut.Bytes()) {
		t.Fatal("verification output is not deterministic")
	}
}

func TestVerifyRejectsEveryOtherInvocation(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{nil, {"verify", "again"}, {"generate"}} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code != 2 {
			t.Fatalf("run(%v) = %d, want 2", args, code)
		}
		if stdout.Len() != 0 || stderr.String() != "usage: batongolden verify\n       batongolden generate --reference-root ABS --node ABS --git ABS --output ABS\n" {
			t.Fatalf("run(%v) stdout = %q, stderr = %q", args, stdout.String(), stderr.String())
		}
	}
}

func TestCorpusRejectsMissingExtraAndChangedVectors(t *testing.T) {
	t.Parallel()
	copyCorpus := func(t *testing.T) fstest.MapFS {
		t.Helper()
		result := make(fstest.MapFS)
		err := fs.WalkDir(embeddedCorpus, ".", func(name string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
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
	tests := map[string]func(fstest.MapFS){
		"missing": func(files fstest.MapFS) {
			delete(files, corpusRoot+"/actions.json")
		},
		"extra": func(files fstest.MapFS) {
			files[corpusRoot+"/extra.json"] = &fstest.MapFile{Data: []byte("{}\n"), Mode: 0o444}
		},
		"changed": func(files fstest.MapFS) {
			files[corpusRoot+"/records.json"].Data[0] = '['
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			files := copyCorpus(t)
			mutate(files)
			if _, _, _, err := verifyCorpus(files); err == nil {
				t.Fatal("verifyCorpus admitted a mutated corpus")
			}
		})
	}
}

func TestExplicitReproductionRejectsPathLookup(t *testing.T) {
	t.Parallel()
	if err := generateCorpus(
		"relative/reference",
		"/absolute/node",
		"/usr/bin/git",
		t.TempDir()+"/corpus",
	); err == nil {
		t.Fatal("generateCorpus accepted a relative reference root")
	}
	if err := generateCorpus(
		t.TempDir(),
		"/absolute/node",
		"git",
		t.TempDir()+"/bad",
	); err == nil {
		t.Fatal("generateCorpus accepted PATH lookup")
	}
}
