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
	PackageVersion       = "1.0.0-rc.5"
	TagName              = "v1.0.0-rc.5"
	TagObject            = "306ed09c3152e8a7413e6b9d09d63d00ee12ff4a"
	Commit               = "b0133b9e53755484f7aa9140fc3c1b349e2f50dd"
	Tree                 = "c079d41d3955d9690a9be39d1711ef45fa3625f3"
	ArchiveSHA256        = "sha256:8fea81036dc678e9a0aa4c2d1fb0c8ed016c23b9e7d77c183f3f168467002dd5"
	SupportPackageSHA256 = "sha256:cd3f1285318820ca5ee3a96785ab40915f7b2970ec14d9e3f578898de4a953c1"
	ManifestSHA256       = "sha256:5af96cf4fae812a49b63328e3bae94d87a2332ea6bb7d022bcc35e00fe07da53"
	AssetCount           = 24
	AssetBytes           = int64(290743)

	releaseDocumentSHA256 = "sha256:31e3c1eb1722bb0da0822891fb013be232ab67cbde7a0b9ba3a5a739822fa799"
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

// supportAssetPaths is Baton's exact RC5 SUPPORT_FILES catalog. The two
// conformance documents are admitted by the snapshot but are not installed as
// generated support, so they do not participate in the support package digest.
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
		return Identity{}, nil, errors.New("compiled VERSION does not identify Baton RC5")
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
		release.PublishedAt != "2026-07-25T23:14:50Z" {
		return errors.New("release metadata has an unexpected publication identity")
	}
	if release.Tag.Name != TagName ||
		release.Tag.Object != TagObject ||
		release.Tag.ObjectType != "tag" ||
		release.Tag.PeeledCommit != Commit ||
		release.Tag.PeeledTree != Tree {
		return errors.New("release metadata has an unexpected annotated tag identity")
	}
	if release.Archive.Name != "baton-1.0.0-rc.5.tar.gz" ||
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
		{"baton-plan", "operations/baton-plan.md", operationVersion, "sha256:91197ccfdda4475b09f70d50e6dd1fe248f7135625172618051b81dc98016088"},
		{"baton-implement", "operations/baton-implement.md", operationVersion, "sha256:8e3a056fe6b0a0db5f30679b4f6cc2a2ba44d53c33538a0ec3d2db04cba7f5f1"},
		{"baton-design-review", "operations/baton-design-review.md", operationVersion, "sha256:cef3db42acdeca0696e939e5cc58b2628469992dbefaa3b0e2429987903b9381"},
		{"baton-verify", "operations/baton-verify.md", operationVersion, "sha256:080034f552086a7e73fc27fb9f155320ac7638749481b477d16af4afdc59afaf"},
		{"baton-merge", "operations/baton-merge.md", operationVersion, "sha256:ccc995fd1b38814858f5fcf1122c61409acbbd281596cb259c6de4ba18a6df1b"},
	}
	expectedTemplates := []releaseTemplate{
		{"plan", "templates/plan.md", "sha256:9228d93d050071faa6269db913a2b8373f24b7b27813f08a97478e42c2090913"},
	}
	expectedContracts := []releaseContract{
		{"engine_adapter", "conformance/engine-adapter.md", "baton.engine-conformance/v1", "sha256:8946bcb51b0ce8349617c5d7a65cb3835445c9fadb14d15e62e901c6a8b83629"},
		{"conformance_manifest", "conformance/manifest.json", "baton.conformance-manifest/v2", "sha256:5ab96576337d1617dbcd50b1405b1d174f5595e44d364fe04a71f1c9d0275833"},
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
