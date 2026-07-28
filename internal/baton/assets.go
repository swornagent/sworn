// Package baton admits and exposes the exact Baton release used by Sworn.
//
// It owns compiled protocol bytes only. Runtime records remain in Git under
// .baton/releases and are never embedded or read through this package.
package baton

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const (
	PackageVersion       = "1.0.0-rc.8"
	TagName              = "v1.0.0-rc.8"
	TagObject            = "749714b60ac6356fbeb43d91ee3ad478820f2ad8"
	ReleaseCommit        = "a8fdb397e0839bdc58ad4b865e163dd37654752c"
	ReleaseTree          = "b39fe4c538a06ce7f28b70edd551395f99a8373c"
	Commit               = "a8fdb397e0839bdc58ad4b865e163dd37654752c"
	Tree                 = "b39fe4c538a06ce7f28b70edd551395f99a8373c"
	ArchiveSHA256        = "sha256:bcbc310c2c5c98f82c721968ced7929ec58b0cdc2ab531a615fec706fe863582"
	SupportPackageSHA256 = "sha256:339799b218d4f8846cec1114a9756dda96a51744a72eb975bb9b632c4e349726"
	ManifestSHA256       = "sha256:f0f39ee622a7154773da4400f9bc1470cb0178121173152a234aee4d182b12c1"
	AssetCount           = 24
	AssetBytes           = int64(362962)

	releaseDocumentSHA256 = "sha256:541f53157be8578ca75753e34246831cb70bce3ef052c882d0b62763f64f0bd3"
	releaseSchema         = "sworn.baton-release/v1"
	manifestSchema        = "sworn.baton-assets/v1"
	operationVersion      = "baton.operation/v2"
)

//go:embed all:release.json all:snapshot/manifest.json all:snapshot/assets
var embeddedPackage embed.FS

var expectedAssetPaths = []string{
	"VERSION",
	"baton/ASSURANCE.md",
	"baton/CONFORMANCE.md",
	"baton/CORE.md",
	"baton/PROTOCOL.md",
	"baton/RATIONALE.md",
	"baton/README.md",
	"conformance/engine-adapter.md",
	"conformance/manifest.json",
	"operations/baton-design-review.md",
	"operations/baton-implement.md",
	"operations/baton-merge.md",
	"operations/baton-plan.md",
	"operations/baton-verify.md",
	"reference/board/oracle.mjs",
	"reference/board/terminal.mjs",
	"reference/board/web.mjs",
	"reference/records/README.md",
	"reference/records/actions.mjs",
	"reference/records/git.mjs",
	"reference/records/receipts.mjs",
	"reference/records/state.mjs",
	"schemas/receipt-v1.json",
	"templates/plan.md",
}

// supportAssetPaths is the exact SUPPORT_FILES catalog at the admitted snapshot
// source. The two conformance documents are admitted by the snapshot but are
// not installed as generated support, so they do not participate in the
// support package digest.
var supportAssetPaths = []string{
	"VERSION",
	"baton/ASSURANCE.md",
	"baton/CONFORMANCE.md",
	"baton/CORE.md",
	"baton/PROTOCOL.md",
	"baton/RATIONALE.md",
	"baton/README.md",
	"operations/baton-design-review.md",
	"operations/baton-implement.md",
	"operations/baton-merge.md",
	"operations/baton-plan.md",
	"operations/baton-verify.md",
	"reference/board/oracle.mjs",
	"reference/board/terminal.mjs",
	"reference/board/web.mjs",
	"reference/records/README.md",
	"reference/records/actions.mjs",
	"reference/records/git.mjs",
	"reference/records/receipts.mjs",
	"reference/records/state.mjs",
	"schemas/receipt-v1.json",
	"templates/plan.md",
}

// Identity is the immutable release identity reported by Sworn.
type Identity struct {
	PackageVersion       string `json:"package_version"`
	TagName              string `json:"tag_name"`
	TagObject            string `json:"tag_object"`
	Commit               string `json:"commit"`
	Tree                 string `json:"tree"`
	ArchiveSHA256        string `json:"archive_sha256"`
	SupportPackageSHA256 string `json:"support_package_sha256"`
	ManifestSHA256       string `json:"manifest_sha256"`
	AssetCount           int    `json:"asset_count"`
	AssetBytes           int64  `json:"asset_bytes"`
}

// Package is a validated handle to the compiled Baton bytes.
type Package struct {
	admitted bool
}

var (
	admitOnce     sync.Once
	admitted      Package
	admittedID    Identity
	admittedPaths map[string]struct{}
	admitErr      error
)

// Load validates every compiled release binding exactly once.
func Load() (Package, error) {
	admitOnce.Do(func() {
		var paths map[string]struct{}
		admittedID, paths, admitErr = validatePackage(embeddedPackage)
		if admitErr == nil {
			admitted = Package{admitted: true}
			admittedPaths = paths
		}
	})
	if admitErr != nil {
		return Package{}, fmt.Errorf("admit Baton package: %w", admitErr)
	}
	return admitted, nil
}

// Identity returns a copy of the admitted release identity.
func (pkg Package) Identity() (Identity, error) {
	if !pkg.admitted {
		return Identity{}, errors.New("Baton package is not admitted")
	}
	return admittedID, nil
}

// ReadAsset returns a fresh copy of one path from the admitted inventory.
func (pkg Package) ReadAsset(name string) ([]byte, error) {
	if !pkg.admitted {
		return nil, errors.New("Baton package is not admitted")
	}
	if _, ok := admittedPaths[name]; !ok {
		return nil, fmt.Errorf("Baton asset %q is not admitted", name)
	}
	body, err := embeddedPackage.ReadFile("snapshot/assets/" + name)
	if err != nil {
		return nil, fmt.Errorf("read admitted Baton asset %q: %w", name, err)
	}
	return append([]byte(nil), body...), nil
}

type releaseFile struct {
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
		SourceCommit   string `json:"source_commit"`
		SourceTree     string `json:"source_tree"`
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

type releaseTemplate struct {
	Name   string `json:"name"`
	Source string `json:"source"`
	SHA256 string `json:"sha256"`
}

type releaseContract struct {
	Kind    string `json:"kind"`
	Source  string `json:"source"`
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
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

type supportDigestEntry struct {
	Path   string `json:"path"`
	Mode   string `json:"mode"`
	Digest string `json:"digest"`
}

func validatePackage(source fs.FS) (Identity, map[string]struct{}, error) {
	releaseBody, err := fs.ReadFile(source, "release.json")
	if err != nil {
		return Identity{}, nil, fmt.Errorf("read release metadata: %w", err)
	}
	if got := digest(releaseBody); got != releaseDocumentSHA256 {
		return Identity{}, nil, fmt.Errorf("release metadata digest is %s", got)
	}
	var release releaseFile
	if err := decodeClosedJSON(releaseBody, &release); err != nil {
		return Identity{}, nil, fmt.Errorf("decode release metadata: %w", err)
	}
	if err := validateReleaseIdentity(release); err != nil {
		return Identity{}, nil, err
	}

	manifestBody, err := fs.ReadFile(source, "snapshot/manifest.json")
	if err != nil {
		return Identity{}, nil, fmt.Errorf("read asset manifest: %w", err)
	}
	if got := digest(manifestBody); got != ManifestSHA256 {
		return Identity{}, nil, fmt.Errorf("asset manifest digest is %s", got)
	}
	var manifest assetManifest
	if err := decodeClosedJSON(manifestBody, &manifest); err != nil {
		return Identity{}, nil, fmt.Errorf("decode asset manifest: %w", err)
	}
	if err := validateManifestIdentity(manifest); err != nil {
		return Identity{}, nil, err
	}

	paths := make(map[string]struct{}, AssetCount)
	digests := make(map[string]string, AssetCount)
	var total int64
	for index, entry := range manifest.Assets {
		if err := validateAssetPath(entry.Path); err != nil {
			return Identity{}, nil, fmt.Errorf("asset %d: %w", index, err)
		}
		if entry.Path != expectedAssetPaths[index] {
			return Identity{}, nil, fmt.Errorf(
				"asset %d path is %q, expected %q",
				index,
				entry.Path,
				expectedAssetPaths[index],
			)
		}
		if _, exists := paths[entry.Path]; exists {
			return Identity{}, nil, fmt.Errorf("asset path %q is duplicated", entry.Path)
		}
		body, err := fs.ReadFile(source, "snapshot/assets/"+entry.Path)
		if err != nil {
			return Identity{}, nil, fmt.Errorf("read asset %q: %w", entry.Path, err)
		}
		info, err := fs.Stat(source, "snapshot/assets/"+entry.Path)
		if err != nil {
			return Identity{}, nil, fmt.Errorf("inspect asset %q: %w", entry.Path, err)
		}
		if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 != 0 {
			return Identity{}, nil, fmt.Errorf("asset %q is not a non-executable regular file", entry.Path)
		}
		if entry.Size < 0 || int64(len(body)) != entry.Size {
			return Identity{}, nil, fmt.Errorf("asset %q size does not match", entry.Path)
		}
		observedDigest := digest(body)
		if observedDigest != entry.SHA256 {
			return Identity{}, nil, fmt.Errorf("asset %q digest is %s", entry.Path, observedDigest)
		}
		paths[entry.Path] = struct{}{}
		digests[entry.Path] = observedDigest
		total += entry.Size
	}
	if total != AssetBytes {
		return Identity{}, nil, fmt.Errorf("asset inventory contains %d bytes", total)
	}
	inventory, err := assetInventory(source)
	if err != nil {
		return Identity{}, nil, err
	}
	if !slices.Equal(inventory, expectedAssetPaths) {
		return Identity{}, nil, fmt.Errorf("compiled asset inventory is %q", inventory)
	}
	supportDigest, err := calculateSupportPackageDigest(digests)
	if err != nil {
		return Identity{}, nil, err
	}
	if supportDigest != SupportPackageSHA256 {
		return Identity{}, nil, fmt.Errorf("generated support package digest is %s", supportDigest)
	}
	if err := validateReleaseBindings(source, release, digests); err != nil {
		return Identity{}, nil, err
	}
	versionBody, err := fs.ReadFile(source, "snapshot/assets/VERSION")
	if err != nil || string(versionBody) != PackageVersion+"\n" {
		return Identity{}, nil, errors.New("compiled VERSION does not identify Baton RC8")
	}

	return Identity{
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
	}, paths, nil
}

func validateReleaseIdentity(release releaseFile) error {
	if release.Schema != releaseSchema ||
		release.PackageVersion != PackageVersion ||
		release.SourceRepository != "https://github.com/sawy3r/baton" ||
		release.ReleaseURL != "https://github.com/sawy3r/baton/releases/tag/"+TagName ||
		release.PublishedAt != "2026-07-28T13:51:48Z" {
		return errors.New("release metadata has an unexpected publication identity")
	}
	if release.Tag.Name != TagName ||
		release.Tag.Object != TagObject ||
		release.Tag.ObjectType != "tag" ||
		release.Tag.PeeledCommit != ReleaseCommit ||
		release.Tag.PeeledTree != ReleaseTree {
		return errors.New("release metadata has an unexpected annotated tag identity")
	}
	if release.Archive.Name != "baton-1.0.0-rc.8.tar.gz" ||
		release.Archive.SHA256 != ArchiveSHA256 ||
		release.Archive.EmbeddedCommit != ReleaseCommit {
		return errors.New("release metadata has an unexpected archive identity")
	}
	if release.GeneratedSupport.ManifestSchema != "baton.generated-adapters/v1" ||
		release.GeneratedSupport.GeneratorVersion != "baton.adapter-generator/v1" ||
		release.GeneratedSupport.OperationVersion != operationVersion ||
		release.GeneratedSupport.PackageDigest != SupportPackageSHA256 {
		return errors.New("release metadata has an unexpected generated-support identity")
	}
	if release.Snapshot.ManifestSchema != manifestSchema ||
		release.Snapshot.SourceCommit != Commit ||
		release.Snapshot.SourceTree != Tree ||
		release.Snapshot.ManifestSHA256 != ManifestSHA256 ||
		release.Snapshot.AssetCount != AssetCount ||
		release.Snapshot.TotalBytes != AssetBytes {
		return errors.New("release metadata has an unexpected snapshot identity")
	}
	return nil
}

func validateManifestIdentity(manifest assetManifest) error {
	if manifest.Schema != manifestSchema || manifest.Commit != Commit {
		return fmt.Errorf(
			"asset manifest identifies %q at %q",
			manifest.Schema,
			manifest.Commit,
		)
	}
	if len(manifest.Assets) != AssetCount {
		return fmt.Errorf("asset manifest has %d entries", len(manifest.Assets))
	}
	return nil
}

func validateReleaseBindings(source fs.FS, release releaseFile, digests map[string]string) error {
	expectedOperations := []releaseOperation{
		{"baton-plan", "operations/baton-plan.md", operationVersion, "sha256:3385b9bd62eee8cbe8b7e23e04abe872e133aa113d2c9ca0b7da3454a17bd413"},
		{"baton-implement", "operations/baton-implement.md", operationVersion, "sha256:30061e6ea64004237f17c1bf51a279a76bd7efff5d1cd39b12016bab942d5efc"},
		{"baton-design-review", "operations/baton-design-review.md", operationVersion, "sha256:71cf67af0b9f3089a58bd6dc9d4c4054a41643135b89bf5bc332a2861d68ea84"},
		{"baton-verify", "operations/baton-verify.md", operationVersion, "sha256:2a6b14e214b6aea9c7d2c27072289735085626d3f4fc81f3a0fe76af3d2353d4"},
		{"baton-merge", "operations/baton-merge.md", operationVersion, "sha256:d2c1b324ff251c3abe8e1e32a7651b9896b69271a6d7b37107efcd09e2da141d"},
	}
	expectedTemplates := []releaseTemplate{
		{"plan", "templates/plan.md", "sha256:ec9571a105445875c3b94f6303035c2dfc9985f39972790ea6473d39e96c9ba5"},
	}
	expectedContracts := []releaseContract{
		{"engine_adapter", "conformance/engine-adapter.md", "baton.engine-conformance/v1", "sha256:8946bcb51b0ce8349617c5d7a65cb3835445c9fadb14d15e62e901c6a8b83629"},
		{"conformance_manifest", "conformance/manifest.json", "baton.conformance-manifest/v2", "sha256:a53ae10a76dcca1f1e426f16385cb1487c9a1f690e2ab5ebb21463ec74cbea73"},
		{"receipt", "schemas/receipt-v1.json", "receipt-v1", "sha256:9c297f6435714ebe05261663ffbbad31998de41cb091db1cc7e8a94d77aa0035"},
	}
	if !slices.Equal(release.Operations, expectedOperations) {
		return errors.New("release operation bindings are not exact")
	}
	if !slices.Equal(release.Templates, expectedTemplates) {
		return errors.New("release template bindings are not exact")
	}
	if !slices.Equal(release.Contracts, expectedContracts) {
		return errors.New("release contract bindings are not exact")
	}
	for _, operation := range release.Operations {
		if digests[operation.Source] != operation.SHA256 {
			return fmt.Errorf("operation %q does not bind an admitted asset", operation.Name)
		}
		body, err := fs.ReadFile(source, "snapshot/assets/"+operation.Source)
		if err != nil {
			return fmt.Errorf("read operation %q: %w", operation.Name, err)
		}
		header := "operation: " + operation.Name + "\nversion: " + operationVersion + "\n"
		if !bytes.Contains(body, []byte(header)) {
			return fmt.Errorf("operation %q does not declare the admitted version", operation.Name)
		}
	}
	for _, template := range release.Templates {
		if digests[template.Source] != template.SHA256 {
			return fmt.Errorf("template %q does not bind an admitted asset", template.Name)
		}
	}
	for _, contract := range release.Contracts {
		if digests[contract.Source] != contract.SHA256 {
			return fmt.Errorf("contract %q does not bind an admitted asset", contract.Kind)
		}
	}
	return nil
}

func calculateSupportPackageDigest(digests map[string]string) (string, error) {
	entries := make([]supportDigestEntry, 0, len(supportAssetPaths))
	for _, name := range supportAssetPaths {
		assetDigest, ok := digests[name]
		if !ok {
			return "", fmt.Errorf("generated support asset %q is not admitted", name)
		}
		entries = append(entries, supportDigestEntry{
			Path:   name,
			Mode:   "0644",
			Digest: assetDigest,
		})
	}
	sort.Slice(entries, func(left, right int) bool {
		return entries[left].Path < entries[right].Path
	})
	canonical, err := json.Marshal(entries)
	if err != nil {
		return "", fmt.Errorf("encode generated support package identity: %w", err)
	}
	return digest(append(canonical, '\n')), nil
}

func validateAssetPath(name string) error {
	if name == "." || !fs.ValidPath(name) || path.Clean(name) != name || strings.Contains(name, `\`) {
		return fmt.Errorf("path %q is not a canonical relative path", name)
	}
	return nil
}

func assetInventory(source fs.FS) ([]string, error) {
	var paths []string
	err := fs.WalkDir(source, "snapshot/assets", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative := strings.TrimPrefix(name, "snapshot/assets/")
		if relative == name {
			return fmt.Errorf("asset path %q is outside the inventory root", name)
		}
		if err := validateAssetPath(relative); err != nil {
			return err
		}
		paths = append(paths, relative)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk compiled asset inventory: %w", err)
	}
	sort.Strings(paths)
	return paths, nil
}

func decodeClosedJSON(body []byte, target any) error {
	targetType := reflect.TypeOf(target)
	if targetType == nil || targetType.Kind() != reflect.Pointer || targetType.Elem().Kind() != reflect.Struct {
		return errors.New("closed JSON target must be a pointer to a struct")
	}
	shape := json.NewDecoder(bytes.NewReader(body))
	shape.UseNumber()
	if err := validateJSONValue(shape, targetType.Elem()); err != nil {
		return err
	}
	if _, err := shape.Token(); !errors.Is(err, io.EOF) {
		return fmt.Errorf("trailing JSON value: %v", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return fmt.Errorf("trailing JSON value: %v", err)
	}
	return nil
}

func validateJSONValue(decoder *json.Decoder, valueType reflect.Type) error {
	switch valueType.Kind() {
	case reflect.Struct:
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		if delimiter, ok := token.(json.Delim); !ok || delimiter != '{' {
			return fmt.Errorf("expected JSON object for %s", valueType)
		}
		fields := make(map[string]reflect.Type, valueType.NumField())
		for index := range valueType.NumField() {
			field := valueType.Field(index)
			name := field.Name
			if tag := field.Tag.Get("json"); tag != "" {
				name, _, _ = strings.Cut(tag, ",")
			}
			if name == "-" {
				continue
			}
			fields[name] = field.Type
		}
		seen := make(map[string]struct{}, len(fields))
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("JSON object key is not a string")
			}
			fieldType, ok := fields[key]
			if !ok {
				return fmt.Errorf("unknown or case-mismatched JSON field %q", key)
			}
			if _, duplicate := seen[key]; duplicate {
				return fmt.Errorf("duplicate JSON field %q", key)
			}
			seen[key] = struct{}{}
			if err := validateJSONValue(decoder, fieldType); err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}
		}
		end, err := decoder.Token()
		if err != nil {
			return err
		}
		if delimiter, ok := end.(json.Delim); !ok || delimiter != '}' {
			return errors.New("JSON object has no closing delimiter")
		}
		if len(seen) != len(fields) {
			var missing []string
			for name := range fields {
				if _, ok := seen[name]; !ok {
					missing = append(missing, name)
				}
			}
			sort.Strings(missing)
			return fmt.Errorf("missing JSON fields %q", missing)
		}
		return nil
	case reflect.Slice:
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		if delimiter, ok := token.(json.Delim); !ok || delimiter != '[' {
			return fmt.Errorf("expected JSON array for %s", valueType)
		}
		for decoder.More() {
			if err := validateJSONValue(decoder, valueType.Elem()); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil {
			return err
		}
		if delimiter, ok := end.(json.Delim); !ok || delimiter != ']' {
			return errors.New("JSON array has no closing delimiter")
		}
		return nil
	case reflect.String:
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		if _, ok := token.(string); !ok {
			return fmt.Errorf("expected JSON string for %s", valueType)
		}
		return nil
	case reflect.Int, reflect.Int64:
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		number, ok := token.(json.Number)
		if !ok {
			return fmt.Errorf("expected JSON integer for %s", valueType)
		}
		if _, err := strconv.ParseInt(number.String(), 10, valueType.Bits()); err != nil {
			return fmt.Errorf("invalid JSON integer %q: %w", number, err)
		}
		return nil
	default:
		return fmt.Errorf("unsupported closed JSON type %s", valueType)
	}
}

func digest(body []byte) string {
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}
