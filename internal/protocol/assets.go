// Package protocol admits and exposes Sworn's own embedded role-asset bundle:
// the Planner, Implementer, Lead, and Verifier instructions and the
// protocol reference material they cite.
//
// It owns compiled protocol bytes only. Runtime records remain in Git under
// the configured records root (default .sworn/records) and are never embedded
// or read through this package.
package protocol

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
	// RoleAssetsVersion identifies this Sworn build's own bundle of role
	// instructions and protocol reference material. Sworn bumps it as an
	// ordinary product change; admission never requires a separately
	// installed, tagged, checked-out, or certified Protocol release.
	RoleAssetsVersion = "sworn.role-assets/v1"
	// LegacyProtocolVersion and LegacyProtocolCommit name the version and
	// commit of the retired Baton protocol package (repo decommissioned
	// 2026-09) whose prose and reference material this bundle still
	// carries, re-worded to Sworn's own role vocabulary (ADR-0014). They are
	// provenance facts recorded for truthful history, never an admission
	// gate against an external product.
	LegacyProtocolVersion = "1.0.0-rc.14"
	LegacyProtocolCommit  = "3dc5f2f0164ff379a3000fe25d2a323b4fe2e8ef"
	ManifestSHA256        = "sha256:a19f8c381689acd6d498b6b94d38b08f4be0b57182f5609845a0643172e82430"
	AssetCount            = 25
	AssetBytes            = int64(391691)

	releaseDocumentSHA256 = "sha256:f864d6086b6b1b212531e3cf7af1563af1c089df7818a9cf7877a81b84a9c455"
	manifestSchema        = "sworn.protocol-assets/v1"
	operationVersion      = "protocol.operation/v2"
)

//go:embed all:release.json all:snapshot/manifest.json all:snapshot/assets
var embeddedPackage embed.FS

var expectedAssetPaths = []string{
	"VERSION",
	"conformance/engine-adapter.md",
	"conformance/manifest.json",
	"operations/protocol-design-review.md",
	"operations/protocol-implement.md",
	"operations/protocol-merge.md",
	"operations/protocol-plan.md",
	"operations/protocol-verify.md",
	"protocol/ASSURANCE.md",
	"protocol/CONFORMANCE.md",
	"protocol/CORE.md",
	"protocol/PROTOCOL.md",
	"protocol/RATIONALE.md",
	"protocol/README.md",
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

// Identity is the immutable, Sworn-owned role-asset identity reported by
// Sworn. It never asserts a separately installed, tagged, or certified
// external Protocol release.
type Identity struct {
	RoleAssetsVersion     string `json:"role_assets_version"`
	LegacyProtocolVersion string `json:"legacy_protocol_version"`
	ManifestSHA256        string `json:"manifest_sha256"`
	AssetCount            int    `json:"asset_count"`
	AssetBytes            int64  `json:"asset_bytes"`
}

// Package is a validated handle to the compiled Protocol bytes.
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
		return Package{}, fmt.Errorf("admit Protocol package: %w", admitErr)
	}
	return admitted, nil
}

// Identity returns a copy of the admitted release identity.
func (pkg Package) Identity() (Identity, error) {
	if !pkg.admitted {
		return Identity{}, errors.New("Protocol package is not admitted")
	}
	return admittedID, nil
}

// ReadAsset returns a fresh copy of one path from the admitted inventory.
func (pkg Package) ReadAsset(name string) ([]byte, error) {
	if !pkg.admitted {
		return nil, errors.New("Protocol package is not admitted")
	}
	if _, ok := admittedPaths[name]; !ok {
		return nil, fmt.Errorf("Protocol asset %q is not admitted", name)
	}
	body, err := embeddedPackage.ReadFile("snapshot/assets/" + name)
	if err != nil {
		return nil, fmt.Errorf("read admitted Protocol asset %q: %w", name, err)
	}
	return append([]byte(nil), body...), nil
}

type releaseFile struct {
	Schema   string `json:"schema"`
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
	if err != nil || string(versionBody) != LegacyProtocolVersion+"\n" {
		return Identity{}, nil, errors.New("compiled VERSION does not identify the embedded legacy Protocol content")
	}

	return Identity{
		RoleAssetsVersion:     RoleAssetsVersion,
		LegacyProtocolVersion: LegacyProtocolVersion,
		ManifestSHA256:        ManifestSHA256,
		AssetCount:            AssetCount,
		AssetBytes:            AssetBytes,
	}, paths, nil
}

func validateReleaseIdentity(release releaseFile) error {
	if release.Schema != RoleAssetsVersion {
		return errors.New("release metadata has an unexpected Sworn role-assets identity")
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
	if manifest.Schema != manifestSchema || manifest.Commit != LegacyProtocolCommit {
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
		{"protocol-plan", "operations/protocol-plan.md", operationVersion, "sha256:9ca77fd2f390544416dbe9f2556d6cd7dcaf3a65bfe7cecb2bc514d05f09d32d"},
		{"protocol-implement", "operations/protocol-implement.md", operationVersion, "sha256:47a158b82bb97f886e73f30bfe5bc88e9b596c131ccee32107c7b859ef792b30"},
		{"protocol-design-review", "operations/protocol-design-review.md", operationVersion, "sha256:e3d5ad816e113ff0b5bc9f94d365419d0842cbe9cb641b91c783cd03c589765b"},
		{"protocol-verify", "operations/protocol-verify.md", operationVersion, "sha256:35027e7d633042a001ed7a24e72cd149a58d0964b83011d79881ae86dd5af826"},
		{"protocol-merge", "operations/protocol-merge.md", operationVersion, "sha256:8784299f6338fc5b92fd9d727db6ef50a7f6aff9226cd8ec7e6224e1c0b63c69"},
	}
	expectedTemplates := []releaseTemplate{
		{"plan", "templates/plan.md", "sha256:98b7e8037c665c50126482fde0df52516ccbc721544546abfc9f3d5eb00186f9"},
	}
	expectedContracts := []releaseContract{
		{"engine_adapter", "conformance/engine-adapter.md", "protocol.engine-conformance/v1", "sha256:7e42f65eb88783cbfc356416b5ac4c04ccdad21c06a2a30fc338fadfb240a42b"},
		{"conformance_manifest", "conformance/manifest.json", "protocol.conformance-manifest/v2", "sha256:21aacbd7cf7ad95f535dc54473b7f36cd4f2afa882d0c30a38d69f4e7adda916"},
		{"receipt", "schemas/receipt-v1.json", "receipt-v1", "sha256:496b862556cd60a191458106fc8ac589c39348893a2dfd023f9e76184216010c"},
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
