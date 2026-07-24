package baton

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"testing/fstest"
)

func TestLoadAdmitsExactRC2(t *testing.T) {
	t.Parallel()

	pkg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	want := Identity{
		PackageVersion:       PackageVersion,
		TagName:              TagName,
		TagObject:            TagObject,
		Commit:               Commit,
		Tree:                 Tree,
		ArchiveSHA256:        ArchiveSHA256,
		SupportPackageSHA256: SupportPackageSHA256,
		ManifestSHA256:       ManifestSHA256,
		AssetCount:           AssetCount,
		AssetBytes:           AssetBytes,
	}
	got, err := pkg.Identity()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("Identity() = %#v, want %#v", got, want)
	}
	version, err := pkg.ReadAsset("VERSION")
	if err != nil {
		t.Fatal(err)
	}
	if string(version) != PackageVersion+"\n" {
		t.Fatalf("VERSION = %q", version)
	}
}

func TestReadAssetReturnsIndependentBytes(t *testing.T) {
	t.Parallel()

	pkg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	first, err := pkg.ReadAsset("VERSION")
	if err != nil {
		t.Fatal(err)
	}
	first[0] = 'x'
	second, err := pkg.ReadAsset("VERSION")
	if err != nil {
		t.Fatal(err)
	}
	if string(second) != PackageVersion+"\n" {
		t.Fatalf("mutating returned bytes changed the package: %q", second)
	}
	if _, err := pkg.ReadAsset("../release.json"); err == nil {
		t.Fatal("ReadAsset accepted a path outside the admitted inventory")
	}
	if _, err := (Package{}).ReadAsset("VERSION"); err == nil {
		t.Fatal("zero Package read an asset")
	}
	if _, err := (Package{}).Identity(); err == nil {
		t.Fatal("zero Package returned an identity")
	}
}

func TestValidatePackageRejectsMutations(t *testing.T) {
	t.Parallel()

	tests := map[string]func(fstest.MapFS){
		"release identity": func(files fstest.MapFS) {
			file := files["release.json"]
			file.Data = bytes.Replace(
				file.Data,
				[]byte(TagObject),
				[]byte("0000000000000000000000000000000000000000"),
				1,
			)
		},
		"manifest identity": func(files fstest.MapFS) {
			file := files["snapshot/manifest.json"]
			file.Data = bytes.Replace(
				file.Data,
				[]byte(Commit),
				[]byte("0000000000000000000000000000000000000000"),
				1,
			)
		},
		"asset bytes": func(files fstest.MapFS) {
			files["snapshot/assets/VERSION"].Data = []byte("1.0.0-rc.x\n")
		},
		"missing asset": func(files fstest.MapFS) {
			delete(files, "snapshot/assets/templates/proof.md")
		},
		"extra asset": func(files fstest.MapFS) {
			files["snapshot/assets/EXTRA"] = &fstest.MapFile{Data: []byte("extra\n"), Mode: 0o444}
		},
		"executable asset": func(files fstest.MapFS) {
			files["snapshot/assets/VERSION"].Mode = 0o555
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			files := copyEmbeddedPackage(t)
			mutate(files)
			if _, _, err := validatePackage(files); err == nil {
				t.Fatal("validatePackage accepted mutated release bytes")
			}
		})
	}
}

func TestCompiledInventoryIsClosed(t *testing.T) {
	t.Parallel()

	got, err := assetInventory(embeddedPackage)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, expectedAssetPaths) {
		t.Fatalf("asset inventory = %v, want %v", got, expectedAssetPaths)
	}
	for _, name := range got {
		if bytes.Contains([]byte(name), []byte(".baton/releases")) {
			t.Fatalf("compiled inventory includes Baton records: %q", name)
		}
	}
}

func TestSourceInventoryHasExactPathsAndModes(t *testing.T) {
	t.Parallel()

	var got []string
	err := filepath.WalkDir("snapshot/assets", func(name string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel("snapshot/assets", name)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 != 0 {
			t.Fatalf("source asset %q mode = %s", relative, info.Mode())
		}
		got = append(got, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(got)
	if !reflect.DeepEqual(got, expectedAssetPaths) {
		t.Fatalf("source asset inventory = %v, want %v", got, expectedAssetPaths)
	}
}

func TestReleaseIdentityComparisonsAreIndependentlyEnforced(t *testing.T) {
	t.Parallel()

	tests := map[string]func(*releaseFile){
		"schema":          func(value *releaseFile) { value.Schema = "other" },
		"package version": func(value *releaseFile) { value.PackageVersion = "other" },
		"source":          func(value *releaseFile) { value.SourceRepository = "other" },
		"release URL":     func(value *releaseFile) { value.ReleaseURL = "other" },
		"publication":     func(value *releaseFile) { value.PublishedAt = "other" },
		"tag name":        func(value *releaseFile) { value.Tag.Name = "other" },
		"tag object":      func(value *releaseFile) { value.Tag.Object = "other" },
		"tag type":        func(value *releaseFile) { value.Tag.ObjectType = "commit" },
		"tag commit":      func(value *releaseFile) { value.Tag.PeeledCommit = "other" },
		"tag tree":        func(value *releaseFile) { value.Tag.PeeledTree = "other" },
		"archive name":    func(value *releaseFile) { value.Archive.Name = "other" },
		"archive digest":  func(value *releaseFile) { value.Archive.SHA256 = "other" },
		"archive commit":  func(value *releaseFile) { value.Archive.EmbeddedCommit = "other" },
		"support schema": func(value *releaseFile) {
			value.GeneratedSupport.ManifestSchema = "other"
		},
		"support generator": func(value *releaseFile) {
			value.GeneratedSupport.GeneratorVersion = "other"
		},
		"support operation": func(value *releaseFile) {
			value.GeneratedSupport.OperationVersion = "other"
		},
		"support digest": func(value *releaseFile) {
			value.GeneratedSupport.PackageDigest = "other"
		},
		"snapshot schema": func(value *releaseFile) { value.Snapshot.ManifestSchema = "other" },
		"manifest digest": func(value *releaseFile) { value.Snapshot.ManifestSHA256 = "other" },
		"asset count":     func(value *releaseFile) { value.Snapshot.AssetCount++ },
		"asset bytes":     func(value *releaseFile) { value.Snapshot.TotalBytes++ },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			value := readReleaseFile(t)
			mutate(&value)
			if err := validateReleaseIdentity(value); err == nil {
				t.Fatal("validateReleaseIdentity accepted a mutated field")
			}
		})
	}
}

func TestManifestAndReleaseBindingComparisonsAreIndependentlyEnforced(t *testing.T) {
	t.Parallel()

	t.Run("manifest schema", func(t *testing.T) {
		value := readAssetManifest(t)
		value.Schema = "other"
		if err := validateManifestIdentity(value); err == nil {
			t.Fatal("validateManifestIdentity accepted a changed schema")
		}
	})
	t.Run("manifest commit", func(t *testing.T) {
		value := readAssetManifest(t)
		value.Commit = "other"
		if err := validateManifestIdentity(value); err == nil {
			t.Fatal("validateManifestIdentity accepted a changed commit")
		}
	})
	t.Run("manifest count", func(t *testing.T) {
		value := readAssetManifest(t)
		value.Assets = value.Assets[:len(value.Assets)-1]
		if err := validateManifestIdentity(value); err == nil {
			t.Fatal("validateManifestIdentity accepted a changed count")
		}
	})

	manifest := readAssetManifest(t)
	digests := make(map[string]string, len(manifest.Assets))
	for _, asset := range manifest.Assets {
		digests[asset.Path] = asset.SHA256
	}
	tests := map[string]func(*releaseFile){
		"operation": func(value *releaseFile) { value.Operations[0].SHA256 = "other" },
		"template":  func(value *releaseFile) { value.Templates[0].SHA256 = "other" },
		"contract":  func(value *releaseFile) { value.Contracts[0].SHA256 = "other" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			value := readReleaseFile(t)
			mutate(&value)
			if err := validateReleaseBindings(embeddedPackage, value, digests); err == nil {
				t.Fatal("validateReleaseBindings accepted a changed binding")
			}
		})
	}
}

func TestDecodeClosedJSONRejectsAmbiguousObjects(t *testing.T) {
	t.Parallel()

	type document struct {
		Schema string `json:"schema"`
	}
	for name, body := range map[string]string{
		"duplicate":    `{"schema":"one","schema":"two"}`,
		"case folded":  `{"Schema":"one"}`,
		"unknown":      `{"schema":"one","extra":"two"}`,
		"missing":      `{}`,
		"trailing":     `{"schema":"one"} {}`,
		"wrong scalar": `{"schema":1}`,
	} {
		t.Run(name, func(t *testing.T) {
			var value document
			if err := decodeClosedJSON([]byte(body), &value); err == nil {
				t.Fatalf("decodeClosedJSON accepted %s", body)
			}
		})
	}
}

func copyEmbeddedPackage(t *testing.T) fstest.MapFS {
	t.Helper()

	files := make(fstest.MapFS)
	err := fs.WalkDir(embeddedPackage, ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		body, err := fs.ReadFile(embeddedPackage, name)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		files[name] = &fstest.MapFile{
			Data: append([]byte(nil), body...),
			Mode: info.Mode(),
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func readReleaseFile(t *testing.T) releaseFile {
	t.Helper()
	body, err := fs.ReadFile(embeddedPackage, "release.json")
	if err != nil {
		t.Fatal(err)
	}
	var value releaseFile
	if err := decodeClosedJSON(body, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func readAssetManifest(t *testing.T) assetManifest {
	t.Helper()
	body, err := fs.ReadFile(embeddedPackage, "snapshot/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var value assetManifest
	if err := decodeClosedJSON(body, &value); err != nil {
		t.Fatal(err)
	}
	return value
}
