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

func TestLoadAdmitsSwornOwnedRoleAssets(t *testing.T) {
	t.Parallel()

	pkg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	want := Identity{
		RoleAssetsVersion:  RoleAssetsVersion,
		LegacyBatonVersion: LegacyBatonVersion,
		ManifestSHA256:     ManifestSHA256,
		AssetCount:         AssetCount,
		AssetBytes:         AssetBytes,
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
	if string(version) != LegacyBatonVersion+"\n" {
		t.Fatalf("VERSION = %q", version)
	}
	if _, err := pkg.ReadAsset("reference/board/presentation.mjs"); err != nil {
		t.Fatal("legacy board presentation dependency is absent:", err)
	}
}

// TestAdmissionNeverBindsAnExternalTagCommitOrSupportPackage guards A1 and A5:
// admitting the embedded bundle must never require matching a separately
// tagged, installed, or certified external Baton release. The absence of
// these fields on releaseFile is itself part of the regression proof.
func TestAdmissionNeverBindsAnExternalTagCommitOrSupportPackage(t *testing.T) {
	t.Parallel()

	shape := reflect.TypeOf(releaseFile{})
	for _, forbidden := range []string{"Tag", "TaggedBlobs", "GeneratedSupport"} {
		if _, ok := shape.FieldByName(forbidden); ok {
			t.Fatalf("releaseFile reintroduced external package field %q", forbidden)
		}
	}
	release := readReleaseFile(t)
	if release.Schema != RoleAssetsVersion {
		t.Fatalf("release.Schema = %q, want a Sworn-owned role-assets identity", release.Schema)
	}
}

func TestManifestIdentityBindsLegacyProvenanceNotAdmission(t *testing.T) {
	t.Parallel()

	manifest := readAssetManifest(t)
	if manifest.Commit != LegacyBatonCommit {
		t.Fatal("asset manifest does not preserve the legacy Baton source commit as provenance")
	}

	manifest.Commit = "other"
	if err := validateManifestIdentity(manifest); err == nil {
		t.Fatal("manifest identity accepted a changed legacy snapshot commit")
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
	if string(second) != LegacyBatonVersion+"\n" {
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
				[]byte(RoleAssetsVersion),
				[]byte("sworn.role-assets/v9"),
				1,
			)
		},
		"manifest identity": func(files fstest.MapFS) {
			file := files["snapshot/manifest.json"]
			file.Data = bytes.Replace(
				file.Data,
				[]byte(LegacyBatonCommit),
				[]byte("0000000000000000000000000000000000000000"),
				1,
			)
		},
		"asset bytes": func(files fstest.MapFS) {
			files["snapshot/assets/VERSION"].Data = []byte("1.0.0-rc.x\n")
		},
		"missing asset": func(files fstest.MapFS) {
			delete(files, "snapshot/assets/reference/records/actions.mjs")
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

// TestOperationEvolutionNeedsOnlyThisRepositorysOwnSourceAndAssets is the
// acceptance-linked A5 proof. validateReleaseBindings computes its expected
// per-operation digests from a Go literal in this package, not from a
// separately installed, tagged, or certified external Baton release; the
// admitted digest for an operation always equals the exact bytes shipped in
// this same commit. Evolving an operation's wording is therefore an ordinary,
// self-contained Sworn change (edit the .md file, this literal, and the
// asset manifest together), never a cross-repository or tag lookup.
func TestOperationEvolutionNeedsOnlyThisRepositorysOwnSourceAndAssets(t *testing.T) {
	t.Parallel()

	release := readReleaseFile(t)
	manifest := readAssetManifest(t)
	digests := make(map[string]string, len(manifest.Assets))
	for _, asset := range manifest.Assets {
		digests[asset.Path] = asset.SHA256
	}
	for _, operation := range release.Operations {
		body, err := fs.ReadFile(embeddedPackage, "snapshot/assets/"+operation.Source)
		if err != nil {
			t.Fatal(err)
		}
		if digest(body) != operation.SHA256 || digests[operation.Source] != operation.SHA256 {
			t.Fatalf("operation %q digest is not bound to this repository's own committed bytes", operation.Name)
		}
	}
	if err := validateReleaseBindings(embeddedPackage, release, digests); err != nil {
		t.Fatalf("validateReleaseBindings rejected this repository's own committed operations: %v", err)
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
