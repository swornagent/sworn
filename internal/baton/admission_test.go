package baton

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

const (
	rc2Commit          = "890238ef063bb53cf51fb3359f1ff527f14846c6"
	rc2ManifestDigest  = "sha256:74243a42dcbaa65eadac161126e9cfa8710803a136b827dd7001f5648459986c"
	rc2OperationSchema = "baton.operation/v1"
)

type releaseIdentity struct {
	Schema           string `json:"schema"`
	PackageVersion   string `json:"package_version"`
	SourceRepository string `json:"source_repository"`
	ReleaseURL       string `json:"release_url"`
	PublishedAt      string `json:"published_at"`
	Tag              struct {
		Name         string `json:"name"`
		Object       string `json:"object"`
		ObjectType   string `json:"object_type"`
		PeeledCommit string `json:"peeled_commit"`
		PeeledTree   string `json:"peeled_tree"`
	} `json:"tag"`
	Archive struct {
		Name           string `json:"name"`
		SHA256         string `json:"sha256"`
		EmbeddedCommit string `json:"embedded_commit"`
	} `json:"archive"`
	GeneratedSupport struct {
		ManifestSchema   string `json:"manifest_schema"`
		GeneratorVersion string `json:"generator_version"`
		OperationVersion string `json:"operation_version"`
		PackageDigest    string `json:"package_digest"`
	} `json:"generated_support"`
	Snapshot struct {
		ManifestSchema string `json:"manifest_schema"`
		ManifestSHA256 string `json:"manifest_sha256"`
		AssetCount     int    `json:"asset_count"`
		TotalBytes     int64  `json:"total_bytes"`
	} `json:"snapshot"`
	Operations []releaseOperation `json:"operations"`
	Templates  []releaseTemplate  `json:"templates"`
	Contracts  []releaseContract  `json:"contracts"`
}

type releaseOperation struct {
	Name    string `json:"name"`
	Source  string `json:"source"`
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
}

type releaseContract struct {
	Kind    string `json:"kind"`
	Source  string `json:"source"`
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
}

type releaseTemplate struct {
	Name   string `json:"name"`
	Source string `json:"source"`
	SHA256 string `json:"sha256"`
}

type assetManifest struct {
	Schema string       `json:"schema"`
	Commit string       `json:"commit"`
	Assets []assetEntry `json:"assets"`
}

type assetEntry struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

func TestRC2ReleaseAdmission(t *testing.T) {
	releaseBody := readFile(t, "release.json")
	var release releaseIdentity
	decodeClosedJSON(t, releaseBody, &release)

	if release.Schema != "sworn.baton-release/v1" ||
		release.PackageVersion != "1.0.0-rc.2" ||
		release.SourceRepository != "https://github.com/sawy3r/baton" ||
		release.ReleaseURL != "https://github.com/sawy3r/baton/releases/tag/v1.0.0-rc.2" ||
		release.PublishedAt != "2026-07-24T09:46:02Z" {
		t.Fatalf("unexpected release identity: %#v", release)
	}
	if release.Tag.Name != "v1.0.0-rc.2" ||
		release.Tag.Object != "b80f3e27f0e0a71a4883bcc282e4843e085f0e04" ||
		release.Tag.ObjectType != "tag" ||
		release.Tag.PeeledCommit != rc2Commit ||
		release.Tag.PeeledTree != "97513f3e6f798f3ad04d5b510a49496a605a8ea4" {
		t.Fatalf("unexpected annotated tag binding: %#v", release.Tag)
	}
	if release.Archive.Name != "baton-1.0.0-rc.2.tar.gz" ||
		release.Archive.SHA256 != "sha256:968088ede0c3bfbafb0a9372d3abbf6853556cc2a6e85ffc25615d6332977e63" ||
		release.Archive.EmbeddedCommit != rc2Commit {
		t.Fatalf("unexpected archive binding: %#v", release.Archive)
	}
	if release.GeneratedSupport.ManifestSchema != "baton.generated-adapters/v1" ||
		release.GeneratedSupport.GeneratorVersion != "baton.adapter-generator/v1" ||
		release.GeneratedSupport.OperationVersion != rc2OperationSchema ||
		release.GeneratedSupport.PackageDigest != "sha256:676c630c6a4ef3f752d604efaa5e51958adec0d8580b74cec7fb1e689b1d3436" {
		t.Fatalf("unexpected generated-support binding: %#v", release.GeneratedSupport)
	}

	manifestBody := readFile(t, filepath.Join("snapshot", "manifest.json"))
	if digest(manifestBody) != rc2ManifestDigest {
		t.Fatalf("asset manifest digest = %q, want %q", digest(manifestBody), rc2ManifestDigest)
	}
	var manifest assetManifest
	decodeClosedJSON(t, manifestBody, &manifest)
	if manifest.Schema != "sworn.baton-assets/v1" || manifest.Commit != rc2Commit {
		t.Fatalf("unexpected asset manifest identity: %#v", manifest)
	}
	if release.Snapshot.ManifestSchema != manifest.Schema ||
		release.Snapshot.ManifestSHA256 != rc2ManifestDigest ||
		release.Snapshot.AssetCount != len(manifest.Assets) {
		t.Fatalf("snapshot release binding does not match manifest: %#v", release.Snapshot)
	}

	expectedPaths := []string{
		"VERSION",
		"baton/PROTOCOL.md",
		"conformance/engine-adapter.md",
		"conformance/manifest.json",
		"operations/baton-design-review.md",
		"operations/baton-implement.md",
		"operations/baton-merge.md",
		"operations/baton-plan.md",
		"operations/baton-verify.md",
		"reference/driver/contract.md",
		"schemas/work-status-v1.json",
		"templates/design.md",
		"templates/plan.md",
		"templates/proof.md",
	}
	var gotPaths []string
	var total int64
	digests := make(map[string]string, len(manifest.Assets))
	for _, entry := range manifest.Assets {
		gotPaths = append(gotPaths, entry.Path)
		body := readFile(t, filepath.Join("snapshot", "assets", filepath.FromSlash(entry.Path)))
		info, err := os.Stat(filepath.Join("snapshot", "assets", filepath.FromSlash(entry.Path)))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o644 {
			t.Fatalf("%s mode = %o, want 644", entry.Path, info.Mode().Perm())
		}
		if int64(len(body)) != entry.Size || digest(body) != entry.SHA256 {
			t.Fatalf("%s does not match its manifest entry", entry.Path)
		}
		total += entry.Size
		digests[entry.Path] = entry.SHA256
	}
	if !reflect.DeepEqual(gotPaths, expectedPaths) {
		t.Fatalf("asset paths = %v, want %v", gotPaths, expectedPaths)
	}
	if total != 50387 || total != release.Snapshot.TotalBytes {
		t.Fatalf("asset bytes = %d, release says %d", total, release.Snapshot.TotalBytes)
	}
	if got := inventory(t, filepath.Join("snapshot", "assets")); !reflect.DeepEqual(got, expectedPaths) {
		t.Fatalf("on-disk asset inventory = %v, want %v", got, expectedPaths)
	}
	if string(readFile(t, filepath.Join("snapshot", "assets", "VERSION"))) != "1.0.0-rc.2\n" {
		t.Fatal("embedded VERSION does not identify Baton 1.0.0-rc.2")
	}

	if len(release.Operations) != 5 {
		t.Fatalf("operation count = %d, want 5", len(release.Operations))
	}
	var operationSources []string
	for _, operation := range release.Operations {
		operationSources = append(operationSources, operation.Source)
		if operation.Version != rc2OperationSchema ||
			digests[operation.Source] != operation.SHA256 ||
			!bytes.Contains(
				readFile(t, filepath.Join("snapshot", "assets", filepath.FromSlash(operation.Source))),
				[]byte("operation: "+operation.Name+"\nversion: "+rc2OperationSchema+"\n"),
			) {
			t.Fatalf("operation binding is not exact: %#v", operation)
		}
	}
	if !reflect.DeepEqual(operationSources, expectedPaths[4:9]) {
		t.Fatalf("operation sources = %v, want %v", operationSources, expectedPaths[4:9])
	}
	expectedTemplates := []releaseTemplate{
		{
			Name: "design", Source: "templates/design.md",
			SHA256: "sha256:10e4a2097bffab99464454f9389b5c72f8e3cb12680943ae945401e7b0ebc146",
		},
		{
			Name: "plan", Source: "templates/plan.md",
			SHA256: "sha256:7caac5f8fc8baccacb2787902c1f86d97a92728db0a42b63a4674444886a276c",
		},
		{
			Name: "proof", Source: "templates/proof.md",
			SHA256: "sha256:0bc58a34505859792ac734ff50a23420ad9f24e0227aee19c4e71d84ef9fd225",
		},
	}
	if !reflect.DeepEqual(release.Templates, expectedTemplates) {
		t.Fatalf("template bindings = %#v, want %#v", release.Templates, expectedTemplates)
	}
	for _, template := range release.Templates {
		if digests[template.Source] != template.SHA256 {
			t.Fatalf("template binding is not exact: %#v", template)
		}
	}
	if len(release.Contracts) != 2 {
		t.Fatalf("contract count = %d, want 2", len(release.Contracts))
	}
	expectedContracts := []releaseContract{
		{
			Kind: "conformance_manifest", Source: "conformance/manifest.json",
			Version: "baton.conformance-manifest/v2",
			SHA256:  "sha256:3bf2535cc1e92ac132576dd0c646062b9d33a0ba33201823f1d92409a6387a92",
		},
		{
			Kind: "work_status", Source: "schemas/work-status-v1.json",
			Version: "baton.work-status/v1",
			SHA256:  "sha256:70219641e954afefa35fe20cf702eeabac3ce7c9290d09d5ce29082bf4a497c1",
		},
	}
	if !reflect.DeepEqual(release.Contracts, expectedContracts) {
		t.Fatalf("contract bindings = %#v, want %#v", release.Contracts, expectedContracts)
	}
	for _, contract := range release.Contracts {
		if digests[contract.Source] != contract.SHA256 {
			t.Fatalf("contract binding is not exact: %#v", contract)
		}
	}
}

func decodeClosedJSON(t *testing.T, body []byte, target any) {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		t.Fatalf("trailing JSON: %v", err)
	}
}

func readFile(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func digest(body []byte) string {
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func inventory(t *testing.T, root string) []string {
	t.Helper()
	var names []string
	err := filepath.WalkDir(root, func(name string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		names = append(names, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(names)
	for index, name := range names {
		if strings.TrimSpace(name) != name {
			t.Fatalf("asset path %d is not canonical: %q", index, name)
		}
	}
	return names
}

func TestRC2ReleaseMetadataIsCanonicalJSON(t *testing.T) {
	body := readFile(t, "release.json")
	var value releaseIdentity
	decodeClosedJSON(t, body, &value)
	canonical, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	canonical = append(canonical, '\n')
	if !bytes.Equal(body, canonical) {
		t.Fatalf("release metadata is not deterministic canonical JSON\n%s", canonical)
	}
}
