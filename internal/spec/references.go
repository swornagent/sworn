package spec

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/swornagent/sworn/internal/baton"
)

const (
	FailureReviewedSpecSchemaInvalid = "reviewed-spec-schema-invalid"
	FailureWorkspaceRootUnavailable  = "workspace-root-unavailable"
	FailureReviewedSpecSourcePath    = "reviewed-spec-source-path-mismatch"
	FailureReferencePathInvalid      = "reference-path-invalid"
	FailureReferencePathEscape       = "reference-path-escape"
)

// ReferenceResolutionError is a pre-dispatch C-02 failure. Its class is
// intentionally the complete public error: it never includes artefact bytes.
type ReferenceResolutionError struct {
	Class string
}

func (e *ReferenceResolutionError) Error() string {
	return "spec references: " + e.Class
}

func preDispatchFailure(class string) error {
	return &ReferenceResolutionError{Class: class}
}

// ResolvedArtifact is one safe, explicit input to spec-ambiguity.
type ResolvedArtifact struct {
	Path     string
	Contents string
}

// UnresolvedReference is a safely confined but unavailable typed input. The
// ambiguity model receives it verbatim, rather than the engine silently
// dropping the evidence.
type UnresolvedReference struct {
	Key    string
	Reason string
}

// ReferenceResolution is the complete C-02 pre-dispatch result. A non-nil
// result has passed every unsafe path check; only its Unresolved entries may be
// incomplete and those are safe to show to the model.
type ReferenceResolution struct {
	Record        *Record
	WorkspaceRoot string
	Artifacts     []ResolvedArtifact
	Unresolved    []UnresolvedReference
}

// Render emits the exact referenced-artifact payload fragment defined by
// Baton. Artifact contents are already validated UTF-8 and are never escaped
// or normalised.
func (r *ReferenceResolution) Render() string {
	if r == nil {
		return ""
	}
	var b strings.Builder
	for _, artifact := range r.Artifacts {
		fmt.Fprintf(&b, "--- ARTIFACT %s ---\n", artifact.Path)
		b.WriteString(artifact.Contents)
		b.WriteByte('\n')
	}
	for _, unresolved := range r.Unresolved {
		fmt.Fprintf(&b, "UNRESOLVED %s: %s\n", unresolved.Key, unresolved.Reason)
	}
	return b.String()
}

// DecodeJSONNoDuplicate parses exactly one JSON document, rejecting duplicate
// raw object keys at every depth before decoding into dst. encoding/json alone
// would silently retain the last duplicate and make an adversarial payload look
// like a different record.
func DecodeJSONNoDuplicate(data []byte, dst any) error {
	if !utf8.Valid(data) {
		return fmt.Errorf("invalid UTF-8 JSON")
	}
	stream := json.NewDecoder(bytes.NewReader(data))
	stream.UseNumber()
	if err := consumeJSONValue(stream); err != nil {
		return err
	}
	if _, err := stream.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return fmt.Errorf("trailing JSON: %w", err)
	}
	if err := json.Unmarshal(data, dst); err != nil {
		return err
	}
	return nil
}

func consumeJSONValue(dec *json.Decoder) error {
	token, err := dec.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := make(map[string]struct{})
		for dec.More() {
			keyToken, err := dec.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("object key is not a string")
			}
			if _, duplicate := seen[key]; duplicate {
				return fmt.Errorf("duplicate JSON key %q", key)
			}
			seen[key] = struct{}{}
			if err := consumeJSONValue(dec); err != nil {
				return err
			}
		}
		end, err := dec.Token()
		if err != nil {
			return err
		}
		if end != json.Delim('}') {
			return fmt.Errorf("object did not close")
		}
	case '[':
		for dec.More() {
			if err := consumeJSONValue(dec); err != nil {
				return err
			}
		}
		end, err := dec.Token()
		if err != nil {
			return err
		}
		if end != json.Delim(']') {
			return fmt.Errorf("array did not close")
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delim)
	}
	return nil
}

// ResolveReferences resolves only the reviewed spec's typed references. It
// intentionally has no prose/touchpoint/test-ref discovery path.
func ResolveReferences(reviewedSpecPath string) (*ReferenceResolution, error) {
	rawSpec, err := os.ReadFile(reviewedSpecPath)
	if err != nil {
		return nil, preDispatchFailure(FailureReviewedSpecSchemaInvalid)
	}
	var record Record
	if err := DecodeJSONNoDuplicate(rawSpec, &record); err != nil {
		return nil, preDispatchFailure(FailureReviewedSpecSchemaInvalid)
	}
	if err := baton.ValidateSchema("spec-v1", rawSpec); err != nil {
		return nil, preDispatchFailure(FailureReviewedSpecSchemaInvalid)
	}

	root, err := physicalWorkspaceRoot(reviewedSpecPath)
	if err != nil {
		return nil, preDispatchFailure(FailureWorkspaceRootUnavailable)
	}
	physicalSpec, err := physicalPath(reviewedSpecPath)
	if err != nil || !strictlyBeneath(root, physicalSpec) {
		return nil, preDispatchFailure(FailureReviewedSpecSourcePath)
	}
	wantSpecPath := path.Join("docs", "release", record.Release, record.SliceID, "spec.json")
	gotSpecPath, err := repoRelativePath(root, physicalSpec)
	if err != nil || gotSpecPath != wantSpecPath {
		return nil, preDispatchFailure(FailureReviewedSpecSourcePath)
	}

	type preparedReference struct {
		Reference
		key        string
		targetPath string
		physical   string
		exists     bool
	}
	prepared := make([]preparedReference, 0, len(record.References))
	for _, reference := range record.References {
		key, targetPath, ok := referenceKeyAndTarget(record.Release, reference)
		if !ok || !validReferencePath(targetPath) {
			return nil, preDispatchFailure(FailureReferencePathInvalid)
		}
		prepared = append(prepared, preparedReference{
			Reference:  reference,
			key:        key,
			targetPath: targetPath,
		})
	}
	for i := range prepared {
		physical, exists, err := resolveConfinedPath(root, prepared[i].targetPath)
		if err != nil {
			return nil, preDispatchFailure(FailureReferencePathEscape)
		}
		prepared[i].physical = physical
		prepared[i].exists = exists
	}

	resolution := &ReferenceResolution{Record: &record, WorkspaceRoot: root}
	artefacts := make(map[string]ResolvedArtifact)
	for _, reference := range prepared {
		contents, reason := readSafeReference(reference.physical, reference.exists)
		if reason != "" {
			resolution.Unresolved = append(resolution.Unresolved, UnresolvedReference{Key: reference.key, Reason: reason})
			continue
		}
		switch reference.Kind {
		case "contract":
			if reason = validateContractReference(contents, record.Release, reference.ContractID); reason != "" {
				resolution.Unresolved = append(resolution.Unresolved, UnresolvedReference{Key: reference.key, Reason: reason})
				continue
			}
		case "slice":
			if reason = validateSliceReference(contents, record.Release, reference.SliceID); reason != "" {
				resolution.Unresolved = append(resolution.Unresolved, UnresolvedReference{Key: reference.key, Reason: reason})
				continue
			}
		}
		repoPath, err := repoRelativePath(root, reference.physical)
		if err != nil {
			return nil, preDispatchFailure(FailureReferencePathEscape)
		}
		artefacts[repoPath] = ResolvedArtifact{Path: repoPath, Contents: contents}
	}
	for _, artifact := range artefacts {
		resolution.Artifacts = append(resolution.Artifacts, artifact)
	}
	bytewiseSortArtifacts(resolution.Artifacts)
	bytewiseSortUnresolved(resolution.Unresolved)
	return resolution, nil
}

// bytewiseSortArtifacts and bytewiseSortUnresolved use a stable MSD radix sort
// rather than a comparison sort. C-02 requires bytewise output order while
// keeping resolution and sorting linear in reference count plus referenced
// bytes. The alphabet has a fixed 256-byte size, so each consumed key byte is
// processed once and no comparison sort appears in the dispatch path.
func bytewiseSortArtifacts(values []ResolvedArtifact) {
	buffer := make([]ResolvedArtifact, len(values))
	rawBytewiseSortArtifacts(values, buffer, 0, len(values), 0)
}

func rawBytewiseSortArtifacts(values, buffer []ResolvedArtifact, start, end, depth int) {
	if end-start < 2 {
		return
	}
	var counts [257]int
	for i := start; i < end; i++ {
		counts[radixBucket(values[i].Path, depth)]++
	}
	positions := counts
	next := start
	for bucket := range positions {
		positions[bucket], next = next, next+positions[bucket]
	}
	write := positions
	for i := start; i < end; i++ {
		bucket := radixBucket(values[i].Path, depth)
		buffer[write[bucket]] = values[i]
		write[bucket]++
	}
	copy(values[start:end], buffer[start:end])
	for bucket := 1; bucket < len(counts); bucket++ {
		bucketStart := positions[bucket]
		bucketEnd := bucketStart + counts[bucket]
		rawBytewiseSortArtifacts(values, buffer, bucketStart, bucketEnd, depth+1)
	}
}

func bytewiseSortUnresolved(values []UnresolvedReference) {
	buffer := make([]UnresolvedReference, len(values))
	rawBytewiseSortUnresolved(values, buffer, 0, len(values), 0)
}

func rawBytewiseSortUnresolved(values, buffer []UnresolvedReference, start, end, depth int) {
	if end-start < 2 {
		return
	}
	var counts [257]int
	for i := start; i < end; i++ {
		counts[radixBucket(values[i].Key, depth)]++
	}
	positions := counts
	next := start
	for bucket := range positions {
		positions[bucket], next = next, next+positions[bucket]
	}
	write := positions
	for i := start; i < end; i++ {
		bucket := radixBucket(values[i].Key, depth)
		buffer[write[bucket]] = values[i]
		write[bucket]++
	}
	copy(values[start:end], buffer[start:end])
	for bucket := 1; bucket < len(counts); bucket++ {
		bucketStart := positions[bucket]
		bucketEnd := bucketStart + counts[bucket]
		rawBytewiseSortUnresolved(values, buffer, bucketStart, bucketEnd, depth+1)
	}
}

func radixBucket(value string, depth int) int {
	if depth >= len(value) {
		return 0
	}
	return int(value[depth]) + 1
}

func referenceKeyAndTarget(release string, reference Reference) (key, target string, ok bool) {
	switch reference.Kind {
	case "contract":
		return "contract:" + reference.ContractID, path.Join("docs", "release", release, "contracts.json"), true
	case "slice":
		return "slice:" + reference.SliceID, path.Join("docs", "release", release, reference.SliceID, "spec.json"), true
	case "file":
		return "file:" + reference.Path, reference.Path, true
	default:
		return "", "", false
	}
}

func validReferencePath(value string) bool {
	if value == "" || strings.HasPrefix(value, "/") || strings.HasSuffix(value, "/") || strings.ContainsRune(value, 0) || strings.Contains(value, "\\") {
		return false
	}
	if path.Clean(value) != value {
		return false
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

func physicalWorkspaceRoot(reviewedSpecPath string) (string, error) {
	cmd := exec.Command("git", "-C", filepath.Dir(reviewedSpecPath), "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	root := strings.TrimSpace(string(output))
	if root == "" {
		return "", fmt.Errorf("empty workspace root")
	}
	return physicalPath(root)
}

func physicalPath(value string) (string, error) {
	physical, err := filepath.EvalSymlinks(value)
	if err != nil {
		return "", err
	}
	return filepath.Abs(physical)
}

// resolveConfinedPath resolves existing components physically. A normal
// nonexistent final component remains a safe `missing` result; a symlink or
// inaccessible component whose physical confinement cannot be established is a
// pre-dispatch failure instead of a potentially unsafe read.
func resolveConfinedPath(root, repositoryPath string) (string, bool, error) {
	current := root
	segments := strings.Split(repositoryPath, "/")
	for i, segment := range segments {
		candidate := filepath.Join(current, segment)
		_, err := os.Lstat(candidate)
		if err != nil {
			if os.IsNotExist(err) {
				return filepath.Join(append([]string{current}, segments[i:]...)...), false, nil
			}
			return "", false, err
		}
		physical, err := physicalPath(candidate)
		if err != nil || !strictlyBeneathOrEqual(root, physical) {
			return "", false, fmt.Errorf("target escapes workspace")
		}
		current = physical
	}
	if !strictlyBeneath(root, current) {
		return "", false, fmt.Errorf("target escapes workspace")
	}
	return current, true, nil
}

func strictlyBeneath(root, candidate string) bool {
	return strictlyBeneathOrEqual(root, candidate) && root != candidate
}

func strictlyBeneathOrEqual(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return !filepath.IsAbs(rel)
}

func repoRelativePath(root, physical string) (string, error) {
	if !strictlyBeneath(root, physical) {
		return "", fmt.Errorf("not beneath workspace")
	}
	rel, err := filepath.Rel(root, physical)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

func readSafeReference(physical string, exists bool) (string, string) {
	if !exists {
		return "", "missing"
	}
	info, err := os.Stat(physical)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "missing"
		}
		return "", "unreadable"
	}
	if !info.Mode().IsRegular() {
		return "", "non-regular"
	}
	if info.Mode().Perm()&0o444 == 0 {
		return "", "unreadable"
	}
	contents, err := os.ReadFile(physical)
	if err != nil {
		return "", "unreadable"
	}
	if !utf8.Valid(contents) {
		return "", "invalid-utf8"
	}
	return string(contents), ""
}

func validateContractReference(contents, release, contractID string) string {
	raw := []byte(contents)
	var record struct {
		Release   string `json:"release"`
		Contracts []struct {
			ID string `json:"id"`
		} `json:"contracts"`
	}
	if err := DecodeJSONNoDuplicate(raw, &record); err != nil {
		return "invalid-json"
	}
	if err := baton.ValidateSchema("contracts-v1", raw); err != nil {
		return "schema-invalid"
	}
	if record.Release != release {
		return "record-release-mismatch"
	}
	count := 0
	for _, contract := range record.Contracts {
		if contract.ID == contractID {
			count++
		}
	}
	switch count {
	case 0:
		return "contract-id-missing"
	case 1:
		return ""
	default:
		return "contract-id-duplicate"
	}
}

func validateSliceReference(contents, release, sliceID string) string {
	raw := []byte(contents)
	var record Record
	if err := DecodeJSONNoDuplicate(raw, &record); err != nil {
		return "invalid-json"
	}
	if err := baton.ValidateSchema("spec-v1", raw); err != nil {
		return "schema-invalid"
	}
	if record.Release != release {
		return "record-release-mismatch"
	}
	if record.SliceID != sliceID {
		return "slice-id-mismatch"
	}
	return ""
}
