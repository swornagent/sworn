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
	PackageVersion       = "1.0.0-rc.2"
	TagName              = "v1.0.0-rc.2"
	TagObject            = "b80f3e27f0e0a71a4883bcc282e4843e085f0e04"
	Commit               = "890238ef063bb53cf51fb3359f1ff527f14846c6"
	Tree                 = "97513f3e6f798f3ad04d5b510a49496a605a8ea4"
	ArchiveSHA256        = "sha256:968088ede0c3bfbafb0a9372d3abbf6853556cc2a6e85ffc25615d6332977e63"
	SupportPackageSHA256 = "sha256:676c630c6a4ef3f752d604efaa5e51958adec0d8580b74cec7fb1e689b1d3436"
	ManifestSHA256       = "sha256:74243a42dcbaa65eadac161126e9cfa8710803a136b827dd7001f5648459986c"
	AssetCount           = 14
	AssetBytes           = int64(50387)

	releaseDocumentSHA256 = "sha256:69f8fb5dc86d1a7926e8edc64cd028ec1ab3164b2b7a17e88a296b7d3aa4e861"
	releaseSchema         = "sworn.baton-release/v1"
	manifestSchema        = "sworn.baton-assets/v1"
	operationVersion      = "baton.operation/v1"
)

//go:embed all:release.json all:snapshot/manifest.json all:snapshot/assets
var embeddedPackage embed.FS

var expectedAssetPaths = []string{
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
		if got := digest(body); got != entry.SHA256 {
			return Identity{}, nil, fmt.Errorf("asset %q digest is %s", entry.Path, got)
		}
		paths[entry.Path] = struct{}{}
		digests[entry.Path] = entry.SHA256
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
	if err := validateReleaseBindings(source, release, digests); err != nil {
		return Identity{}, nil, err
	}
	versionBody, err := fs.ReadFile(source, "snapshot/assets/VERSION")
	if err != nil || string(versionBody) != PackageVersion+"\n" {
		return Identity{}, nil, errors.New("compiled VERSION does not identify Baton RC2")
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
		release.PublishedAt != "2026-07-24T09:46:02Z" {
		return errors.New("release metadata has an unexpected publication identity")
	}
	if release.Tag.Name != TagName ||
		release.Tag.Object != TagObject ||
		release.Tag.ObjectType != "tag" ||
		release.Tag.PeeledCommit != Commit ||
		release.Tag.PeeledTree != Tree {
		return errors.New("release metadata has an unexpected annotated tag identity")
	}
	if release.Archive.Name != "baton-1.0.0-rc.2.tar.gz" ||
		release.Archive.SHA256 != ArchiveSHA256 ||
		release.Archive.EmbeddedCommit != Commit {
		return errors.New("release metadata has an unexpected archive identity")
	}
	if release.GeneratedSupport.ManifestSchema != "baton.generated-adapters/v1" ||
		release.GeneratedSupport.GeneratorVersion != "baton.adapter-generator/v1" ||
		release.GeneratedSupport.OperationVersion != operationVersion ||
		release.GeneratedSupport.PackageDigest != SupportPackageSHA256 {
		return errors.New("release metadata has an unexpected generated-support identity")
	}
	if release.Snapshot.ManifestSchema != manifestSchema ||
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
		{"baton-design-review", "operations/baton-design-review.md", operationVersion, "sha256:ead3a7d0e22a794ca5430fdbaca5c29f3ae5d5f6fad7c102d1f2bd878f28e356"},
		{"baton-implement", "operations/baton-implement.md", operationVersion, "sha256:2444bead5b1a32188003ce515ac8862bd04d373b740bd89646a86ac5341c2f88"},
		{"baton-merge", "operations/baton-merge.md", operationVersion, "sha256:94b8fb6026c903569cd375cafd11d27868759072dde256265556c710387ae62c"},
		{"baton-plan", "operations/baton-plan.md", operationVersion, "sha256:e5c3ace4177cb10c9b0d3b5e569aa7cbe43bfdb3b7f4a17071a925a5ba3b77d3"},
		{"baton-verify", "operations/baton-verify.md", operationVersion, "sha256:a6f0e9b9bf95cb59e5030b7f95f72d8d3545b52ef771c7d20e7be44a20e45bed"},
	}
	expectedTemplates := []releaseTemplate{
		{"design", "templates/design.md", "sha256:10e4a2097bffab99464454f9389b5c72f8e3cb12680943ae945401e7b0ebc146"},
		{"plan", "templates/plan.md", "sha256:7caac5f8fc8baccacb2787902c1f86d97a92728db0a42b63a4674444886a276c"},
		{"proof", "templates/proof.md", "sha256:0bc58a34505859792ac734ff50a23420ad9f24e0227aee19c4e71d84ef9fd225"},
	}
	expectedContracts := []releaseContract{
		{"conformance_manifest", "conformance/manifest.json", "baton.conformance-manifest/v2", "sha256:3bf2535cc1e92ac132576dd0c646062b9d33a0ba33201823f1d92409a6387a92"},
		{"work_status", "schemas/work-status-v1.json", "baton.work-status/v1", "sha256:70219641e954afefa35fe20cf702eeabac3ce7c9290d09d5ce29082bf4a497c1"},
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
