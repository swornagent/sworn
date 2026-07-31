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
	PackageVersion = "1.0.0-rc.12"
	TagName        = "v1.0.0-rc.12"
	TagObject      = "caac9f0ab32a596600874f911c7f2a5cd24b6552"
	ReleaseCommit  = "5bc374451d0e31d74948ea63010f87d017a3abd5"
	ReleaseTree    = "27297a37e7efd0154c487abfc5bae98fe711a8df"
	Commit         = "5bc374451d0e31d74948ea63010f87d017a3abd5"
	Tree           = "27297a37e7efd0154c487abfc5bae98fe711a8df"
	ArchiveSHA256  = "sha256:620e0f04ddcfa10067a8519d23b169d5e3fcc2751f28652990c889b72e0e4afb"
	// SupportPackageSHA256 retains the v1 wire name while binding RC12's
	// published skills payload. Sworn neither embeds nor recomputes that payload.
	SupportPackageSHA256 = "sha256:f2db06b64a31403e7a864816a3b278a48578a5788eed3235d2be95cfbf093ef2"
	ManifestSHA256       = "sha256:61e9760ce782c754cc766920937c0d7fd3ff592db157dea42ca9de0475b0d2ab"
	AssetCount           = 25
	AssetBytes           = int64(371555)

	releaseDocumentSHA256 = "sha256:b0ed4b06d28f371f57c673bafc625c63b9d657d839f1e25354c27a2c7e04569d"
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
	"reference/board/presentation.mjs",
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
	if err := validateReleaseBindings(source, release, digests); err != nil {
		return Identity{}, nil, err
	}
	versionBody, err := fs.ReadFile(source, "snapshot/assets/VERSION")
	if err != nil || string(versionBody) != PackageVersion+"\n" {
		return Identity{}, nil, errors.New("compiled VERSION does not identify Baton RC12")
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
		release.PublishedAt != "2026-07-31T04:32:53Z" {
		return errors.New("release metadata has an unexpected publication identity")
	}
	if release.Tag.Name != TagName ||
		release.Tag.Object != TagObject ||
		release.Tag.ObjectType != "tag" ||
		release.Tag.PeeledCommit != ReleaseCommit ||
		release.Tag.PeeledTree != ReleaseTree {
		return errors.New("release metadata has an unexpected annotated tag identity")
	}
	if release.Archive.Name != "baton-1.0.0-rc.12.tar.gz" ||
		release.Archive.SHA256 != ArchiveSHA256 ||
		release.Archive.EmbeddedCommit != ReleaseCommit {
		return errors.New("release metadata has an unexpected archive identity")
	}
	if release.GeneratedSupport.ManifestSchema != "baton.skills-payload/v1" ||
		release.GeneratedSupport.GeneratorVersion != "baton.skill-generator/v1" ||
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
		{"baton-plan", "operations/baton-plan.md", operationVersion, "sha256:443f8bbce2914f2586de8ae7796b346554097421742071e8494d459673b82760"},
		{"baton-implement", "operations/baton-implement.md", operationVersion, "sha256:c274017d47d9dd7bc86ff1188cab1b688f7df73500b3bacdb4244bf496c8c473"},
		{"baton-design-review", "operations/baton-design-review.md", operationVersion, "sha256:ecfecf92a1858db9a27de6105ccf647f5a15ec85ed76a346072182e22e99a6d5"},
		{"baton-verify", "operations/baton-verify.md", operationVersion, "sha256:8ca4dff1ab2c607cd23ea2828daf11dc88a7dbeb3194229f2ff5c3c83f510014"},
		{"baton-merge", "operations/baton-merge.md", operationVersion, "sha256:4056008e46a987a9c08cc69f297acfe08c1844eb1edcadf146096ea9bd23ada3"},
	}
	expectedTemplates := []releaseTemplate{
		{"plan", "templates/plan.md", "sha256:ec9571a105445875c3b94f6303035c2dfc9985f39972790ea6473d39e96c9ba5"},
	}
	expectedContracts := []releaseContract{
		{"engine_adapter", "conformance/engine-adapter.md", "baton.engine-conformance/v1", "sha256:5dd917443421a6f79f9fe231cd92b83252bcf2014d61a365f86d394fceb9a440"},
		{"conformance_manifest", "conformance/manifest.json", "baton.conformance-manifest/v2", "sha256:8c3b7247a782a55a08c2ca09226123e4b96b8e80f4c2649a950a0df699988018"},
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
