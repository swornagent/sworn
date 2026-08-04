// Package baton admits and exposes the exact Baton release used by Sworn.
//
// It owns compiled protocol bytes only. Runtime records remain in Git under
// .baton/releases and are never embedded or read through this package.
package baton

import (
	"bytes"
	"crypto/sha1"
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
	PackageVersion = "1.0.0-rc.14"
	TagName        = "v1.0.0-rc.14"
	TagObject      = "cf3a3822cebedf7e53a067f8966ecb17f238c8df"
	ReleaseCommit  = "efacafb2579e99b9d291b2ad27d41df26fbb9d79"
	ReleaseTree    = "a92479268e0874f0d262ad80703fee489b5d6572"
	Commit         = ReleaseCommit
	Tree           = ReleaseTree
	// SupportPackageSHA256 binds the generated support shipped by the tag.
	SupportPackageSHA256 = "sha256:6a1528cbaf357eb9ffc9e494d55f1de86cbb43ee220848cc9fb65227b9fd0452"
	ManifestSHA256       = "sha256:3ee5d18eb6bc38bc3694bfe6ad12a6d45dac3586378f4f4fd572b560aaa9755e"
	AssetCount           = 25
	AssetBytes           = int64(379571)

	releaseDocumentSHA256 = "sha256:2995eca4ebd0d234b0a0c760dfb2aa3242072b05095a9ace5a421fd75793e003"
	releaseSchema         = "sworn.baton-release/v2"
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
	Schema         string `json:"schema"`
	PackageVersion string `json:"package_version"`
	Tag            struct {
		Name         string `json:"name"`
		Object       string `json:"object"`
		ObjectType   string `json:"object_type"`
		PeeledCommit string `json:"peeled_commit"`
		PeeledTree   string `json:"peeled_tree"`
	} `json:"tag"`
	TaggedBlobs      []taggedBlob `json:"tagged_blobs"`
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

type taggedBlob struct {
	Path   string `json:"path"`
	Object string `json:"object"`
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
		return Identity{}, nil, errors.New("compiled VERSION does not identify Baton RC14")
	}

	return Identity{
		PackageVersion:       PackageVersion,
		TagName:              TagName,
		TagObject:            TagObject,
		Commit:               Commit,
		Tree:                 Tree,
		SupportPackageSHA256: SupportPackageSHA256,
		ManifestSHA256:       ManifestSHA256,
		AssetCount:           AssetCount,
		AssetBytes:           AssetBytes,
	}, paths, nil
}

func validateReleaseIdentity(release releaseFile) error {
	if release.Schema != releaseSchema || release.PackageVersion != PackageVersion {
		return errors.New("release metadata has an unexpected tag-native identity")
	}
	if release.Tag.Name != TagName ||
		release.Tag.Object != TagObject ||
		release.Tag.ObjectType != "tag" ||
		release.Tag.PeeledCommit != ReleaseCommit ||
		release.Tag.PeeledTree != ReleaseTree {
		return errors.New("release metadata has an unexpected annotated tag identity")
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
		{"baton-merge", "operations/baton-merge.md", operationVersion, "sha256:f4856ed3c8475fffb316c7296bd38ad6ab5937c757edfe361f20979a45ceaf26"},
	}
	expectedTemplates := []releaseTemplate{
		{"plan", "templates/plan.md", "sha256:ec9571a105445875c3b94f6303035c2dfc9985f39972790ea6473d39e96c9ba5"},
	}
	expectedContracts := []releaseContract{
		{"engine_adapter", "conformance/engine-adapter.md", "baton.engine-conformance/v1", "sha256:5dd917443421a6f79f9fe231cd92b83252bcf2014d61a365f86d394fceb9a440"},
		{"conformance_manifest", "conformance/manifest.json", "baton.conformance-manifest/v2", "sha256:cc1f60350ee7b2eb975d5ee79e6d7df7f39b22921020389324f4f63bc4e613c2"},
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
	if !slices.Equal(release.TaggedBlobs, expectedTaggedBlobs) {
		return errors.New("release tagged blob bindings are not exact")
	}
	for _, tagged := range release.TaggedBlobs {
		body, err := fs.ReadFile(source, "snapshot/assets/"+tagged.Path)
		if err != nil || gitBlobObject(body) != tagged.Object {
			return fmt.Errorf("tagged blob %q does not bind an admitted asset", tagged.Path)
		}
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

func gitBlobObject(body []byte) string {
	hash := sha1.New()
	_, _ = io.WriteString(hash, "blob "+strconv.Itoa(len(body))+"\x00")
	_, _ = hash.Write(body)
	return hex.EncodeToString(hash.Sum(nil))
}

var expectedTaggedBlobs = []taggedBlob{
	{"VERSION", "e899313dba810c02edaac0a1347eaa25199e75fd"},
	{"baton/ASSURANCE.md", "86143d6e590879f36e2108d0ed223f060a08d284"},
	{"baton/CONFORMANCE.md", "4e20f69bae93c23c97800f324578c55025aca5b9"},
	{"baton/CORE.md", "2aaf79fb45b558484f0a3f140deb78af3968895e"},
	{"baton/PROTOCOL.md", "1f67f0f966573019a2e4224a3e68285835a1f786"},
	{"baton/RATIONALE.md", "611831de2770f85ef88447d3f65b4a508d63ee62"},
	{"baton/README.md", "450656fe59398deff00cf7577cb0522dfc5797fb"},
	{"conformance/engine-adapter.md", "f3edb12c57134baf022d14e8c1a3fe2df62c0047"},
	{"conformance/manifest.json", "480ab7447c5d6a1c2a8da3858ece16af46aad609"},
	{"operations/baton-design-review.md", "66665581b7ab38b582df88082863a8a8d2309c55"},
	{"operations/baton-implement.md", "e4d27b28a1b1dd1609e1ed8a76b24ffb202d7878"},
	{"operations/baton-merge.md", "acdf2c280c1f1e3df7891c84d8f83b32470e0bd6"},
	{"operations/baton-plan.md", "a7a46bb6da6c876727a739041856642b3359e0cb"},
	{"operations/baton-verify.md", "3f2d5a428f509bf3c585ce01d391129988975c78"},
	{"reference/board/oracle.mjs", "d3db1e0eb9e35c41e13743208887ba33ff58566c"},
	{"reference/board/presentation.mjs", "fe4ba5a620db1702ff403ac1cbbf856a427ce2d6"},
	{"reference/board/terminal.mjs", "9410f7713f63a86672c592a7bfb851b99d4ed5c9"},
	{"reference/board/web.mjs", "e86ac21731448f1b17aaa7f638227636168de89c"},
	{"reference/records/README.md", "036c4b74d03861ae5544aa1faaf04a741210213b"},
	{"reference/records/actions.mjs", "7b21f328138da39ae8fa7758d698f8705ff5ca04"},
	{"reference/records/git.mjs", "2072b2dba72241ea21e655762eb075dce52fdbd0"},
	{"reference/records/receipts.mjs", "04f119c152c42cdcef1fb6f98c4dd148f54e68c9"},
	{"reference/records/state.mjs", "6863bb52c87ea31ba6117064d5a1a35e50a4628f"},
	{"schemas/receipt-v1.json", "68a27a38b9a5be6118675efd9b5c7d67acd6df0d"},
	{"templates/plan.md", "b6b1615101fcdf16d8dc831aefa5a3ff9766a8b0"},
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
