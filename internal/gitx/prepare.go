package gitx

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type Identity struct{ Name, Email string }
type BlobChange struct {
	Path   string
	Bytes  []byte
	Delete bool
}
type RecordRequest struct {
	Parent    OID
	Changes   []BlobChange
	Message   string
	Identity  Identity
	Timestamp int64
}
type PreparedCommit struct {
	Commit  OID
	Tree    OID
	Parents []OID
}

func validateIdentity(identity Identity) error {
	if identity.Name == "" || identity.Email == "" || len(identity.Name) > 200 || len(identity.Email) > 320 ||
		strings.ContainsAny(identity.Name, "\x00\n<>") || strings.ContainsAny(identity.Email, "\x00\n<>") {
		return fail("INVALID_COMMIT_IDENTITY", "validate commit identity", errors.New("identity is not a closed Git identity"))
	}
	return nil
}
func validateMessage(message string) error {
	if message == "" || len([]byte(message)) > MaxMessageBytes || strings.ContainsRune(message, 0) {
		return fail("INVALID_COMMIT_MESSAGE", "validate commit message", fmt.Errorf("message must contain 1-%d bytes", MaxMessageBytes))
	}
	return nil
}
func commitEnvironment(identity Identity, timestamp int64, extra ...string) []string {
	date := strconv.FormatInt(timestamp, 10) + " +0000"
	result := []string{
		"GIT_AUTHOR_NAME=" + identity.Name,
		"GIT_AUTHOR_EMAIL=" + identity.Email,
		"GIT_AUTHOR_DATE=" + date,
		"GIT_COMMITTER_NAME=" + identity.Name,
		"GIT_COMMITTER_EMAIL=" + identity.Email,
		"GIT_COMMITTER_DATE=" + date,
	}
	return append(result, extra...)
}
func (r *Repository) PrepareRecord(request RecordRequest) (PreparedCommit, error) {
	if err := r.validateOID(request.Parent); err != nil {
		return PreparedCommit{}, err
	}
	if len(request.Changes) == 0 || len(request.Changes) > MaxBatchPaths {
		return PreparedCommit{}, fail("RESOURCE_LIMIT", "prepare record", fmt.Errorf("requires 1-%d changes", MaxBatchPaths))
	}
	if err := validateMessage(request.Message); err != nil {
		return PreparedCommit{}, err
	}
	if err := validateIdentity(request.Identity); err != nil {
		return PreparedCommit{}, err
	}
	if request.Timestamp < 0 {
		return PreparedCommit{}, fail("INVALID_TIMESTAMP", "prepare record", errors.New("timestamp must be non-negative"))
	}
	changes := make([]BlobChange, len(request.Changes))
	copy(changes, request.Changes)
	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	seen := make(map[string]struct{}, len(changes))
	total := 0
	for index := range changes {
		if err := ValidatePath(changes[index].Path, false); err != nil {
			return PreparedCommit{}, err
		}
		if _, duplicate := seen[changes[index].Path]; duplicate {
			return PreparedCommit{}, fail("DUPLICATE_PATH", "prepare record", fmt.Errorf("%s is repeated", changes[index].Path))
		}
		seen[changes[index].Path] = struct{}{}
		changes[index].Bytes = append([]byte(nil), changes[index].Bytes...)
		if !changes[index].Delete {
			if len(changes[index].Bytes) > MaxFileBytes {
				return PreparedCommit{}, fail("RESOURCE_LIMIT", "prepare record", fmt.Errorf("%s exceeds %d bytes", changes[index].Path, MaxFileBytes))
			}
			total += len(changes[index].Bytes)
		}
	}
	if total > MaxBatchBytes {
		return PreparedCommit{}, fail("RESOURCE_LIMIT", "prepare record", fmt.Errorf("changes exceed %d bytes", MaxBatchBytes))
	}
	temp, err := os.MkdirTemp("", "sworn-git-index-*")
	if err != nil {
		return PreparedCommit{}, fail("GIT_EXECUTION_FAILED", "prepare record index", err)
	}
	defer os.RemoveAll(temp)
	indexPath := filepath.Join(temp, "index")
	indexEnv := []string{"GIT_INDEX_FILE=" + indexPath}
	if _, err := r.run(nil, indexEnv, "read-tree", request.Parent.String()+"^{tree}"); err != nil {
		return PreparedCommit{}, err
	}
	for _, change := range changes {
		if change.Delete {
			if _, err := r.run(nil, indexEnv, "update-index", "--force-remove", "--", change.Path); err != nil {
				return PreparedCommit{}, err
			}
			continue
		}
		rawOID, err := r.run(change.Bytes, nil, "hash-object", "-w", "--stdin")
		if err != nil {
			return PreparedCommit{}, err
		}
		blob, err := r.parseOID(string(rawOID))
		if err != nil {
			return PreparedCommit{}, err
		}
		if _, err := r.run(nil, indexEnv, "update-index", "--add", "--cacheinfo", "100644,"+blob.String()+","+change.Path); err != nil {
			return PreparedCommit{}, err
		}
	}
	rawTree, err := r.run(nil, indexEnv, "write-tree")
	if err != nil {
		return PreparedCommit{}, err
	}
	tree, err := r.parseOID(string(rawTree))
	if err != nil {
		return PreparedCommit{}, err
	}
	rawCommit, err := r.run([]byte(request.Message), commitEnvironment(request.Identity, request.Timestamp), "commit-tree", tree.String(), "-p", request.Parent.String())
	if err != nil {
		return PreparedCommit{}, err
	}
	commit, err := r.parseOID(string(rawCommit))
	if err != nil {
		return PreparedCommit{}, err
	}
	return PreparedCommit{Commit: commit, Tree: tree, Parents: []OID{request.Parent}}, nil
}

type CompositionMode string

const (
	FastForward CompositionMode = "fast-forward"
	TwoParent   CompositionMode = "two-parent"
)

type CompositionRequest struct {
	Expected  OID
	Candidate OID
	Message   string
	Identity  Identity
}
type PreparedComposition struct {
	Mode    CompositionMode
	Commit  OID
	Tree    OID
	Parents []OID
}
type compositionContext struct{ directory, gitDirectory, index, attributesFile, objectDirectory string }

func (r *Repository) repositoryObjectDirectory() (string, error) {
	raw, err := r.run(nil, nil, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", err
	}
	common := strings.TrimSpace(string(raw))
	if !filepath.IsAbs(common) {
		return "", fail("INVALID_GIT_OBJECT_DIRECTORY", "resolve object directory", errors.New("Git common directory is not absolute"))
	}
	objects, err := filepath.EvalSymlinks(filepath.Join(common, "objects"))
	if err != nil {
		return "", fail("INVALID_GIT_OBJECT_DIRECTORY", "resolve object directory", err)
	}
	info, err := os.Stat(objects)
	if err != nil || !info.IsDir() {
		return "", fail("INVALID_GIT_OBJECT_DIRECTORY", "inspect object directory", err)
	}
	return objects, nil
}
func (r *Repository) newCompositionContext() (*compositionContext, error) {
	directory, err := os.MkdirTemp("", "sworn-git-context-*")
	if err != nil {
		return nil, fail("GIT_EXECUTION_FAILED", "create composition context", err)
	}
	context := &compositionContext{
		directory: directory, gitDirectory: filepath.Join(directory, "repository.git"),
		index: filepath.Join(directory, "index"), attributesFile: filepath.Join(directory, "attributes"),
	}
	context.objectDirectory, err = r.repositoryObjectDirectory()
	if err != nil {
		os.RemoveAll(directory)
		return nil, err
	}
	if _, err := r.run(nil, nil, "init", "--quiet", "--bare", "--object-format="+string(r.format), context.gitDirectory); err != nil {
		os.RemoveAll(directory)
		return nil, err
	}
	return context, nil
}
func (context *compositionContext) environment() []string {
	return []string{
		"GIT_DIR=" + context.gitDirectory,
		"GIT_OBJECT_DIRECTORY=" + context.objectDirectory,
		"GIT_INDEX_FILE=" + context.index,
		"GIT_NO_REPLACE_OBJECTS=1",
	}
}
func (r *Repository) contextRun(context *compositionContext, stdin []byte, attributesFile string, args ...string) ([]byte, error) {
	return r.runWithAttributes(stdin, context.environment(), attributesFile, args...)
}
func unionTreePaths(left, right []TreeEntry) []string {
	values := make(map[string]struct{}, len(left)+len(right))
	for _, entry := range append(append([]TreeEntry(nil), left...), right...) {
		values[entry.Path] = struct{}{}
	}
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
func (r *Repository) mergeAttributesAtSource(context *compositionContext, source OID, paths []string) (map[string]string, error) {
	if len(paths) == 0 {
		return map[string]string{}, nil
	}
	var input bytes.Buffer
	for _, name := range paths {
		input.WriteString(name)
		input.WriteByte(0)
	}
	raw, err := r.contextRun(context, input.Bytes(), "/dev/null", "check-attr", "-z", "--stdin", "--source="+source.String(), "merge")
	if err != nil {
		return nil, fail("UNTRUSTED_MERGE_ATTRIBUTES", "inspect merge attributes", err)
	}
	fields := bytes.Split(raw, []byte{0})
	if len(fields) > 0 && len(fields[len(fields)-1]) == 0 {
		fields = fields[:len(fields)-1]
	}
	if len(fields)%3 != 0 {
		return nil, fail("INVALID_GIT_OUTPUT", "inspect merge attributes", errors.New("Git returned malformed attribute triples"))
	}
	allowed := map[string]bool{
		"unspecified": true, "set": true, "unset": true,
		"text": true, "binary": true, "union": true,
	}
	result := make(map[string]string, len(fields)/3)
	for index := 0; index < len(fields); index += 3 {
		name, attribute, value := string(fields[index]), string(fields[index+1]), string(fields[index+2])
		if attribute != "merge" || !allowed[value] {
			return nil, fail("CUSTOM_MERGE_DRIVER", "prepare composition", fmt.Errorf("custom merge driver %q applies to %s", value, name))
		}
		if err := ValidatePath(name, false); err != nil {
			return nil, err
		}
		result[name] = value
	}
	return result, nil
}
func renderMergeAttributes(attributes map[string]string) []byte {
	paths := make([]string, 0, len(attributes))
	for name, value := range attributes {
		if value != "unspecified" {
			paths = append(paths, name)
		}
	}
	sort.Strings(paths)
	var result strings.Builder
	for _, name := range paths {
		attribute := "merge=" + attributes[name]
		if attributes[name] == "set" {
			attribute = "merge"
		} else if attributes[name] == "unset" {
			attribute = "-merge"
		}
		result.WriteString(strconv.Quote(name))
		result.WriteByte(' ')
		result.WriteString(attribute)
		result.WriteByte('\n')
	}
	return []byte(result.String())
}
func (r *Repository) deterministicMergeTree(expected, candidate OID) (OID, error) {
	context, err := r.newCompositionContext()
	if err != nil {
		return OID{}, err
	}
	defer os.RemoveAll(context.directory)
	left, err := r.ListTree(expected)
	if err != nil {
		return OID{}, err
	}
	right, err := r.ListTree(candidate)
	if err != nil {
		return OID{}, err
	}
	paths := unionTreePaths(left, right)
	expectedAttributes, err := r.mergeAttributesAtSource(context, expected, paths)
	if err != nil {
		return OID{}, err
	}
	if _, err := r.mergeAttributesAtSource(context, candidate, paths); err != nil {
		return OID{}, err
	}
	if err := os.WriteFile(context.attributesFile, renderMergeAttributes(expectedAttributes), 0o600); err != nil {
		return OID{}, fail("GIT_EXECUTION_FAILED", "install merge attributes", err)
	}
	if _, err := r.contextRun(context, nil, context.attributesFile, "read-tree", expected.String()); err != nil {
		return OID{}, err
	}
	rawTree, err := r.contextRun(context, nil, context.attributesFile, "merge-tree", "--write-tree", "--no-messages", expected.String(), candidate.String())
	if err != nil {
		return OID{}, fail("MERGE_CONFLICT", "prepare composition", err)
	}
	treeText := strings.TrimSpace(string(rawTree))
	if strings.ContainsAny(treeText, " \t\n") {
		return OID{}, fail("INVALID_GIT_OUTPUT", "prepare composition", errors.New("merge-tree returned extra output"))
	}
	return r.parseOID(treeText)
}
func (r *Repository) PrepareComposition(request CompositionRequest) (PreparedComposition, error) {
	if err := r.validateOID(request.Expected); err != nil {
		return PreparedComposition{}, err
	}
	if err := r.validateOID(request.Candidate); err != nil {
		return PreparedComposition{}, err
	}
	if err := validateMessage(request.Message); err != nil {
		return PreparedComposition{}, err
	}
	if err := validateIdentity(request.Identity); err != nil {
		return PreparedComposition{}, err
	}
	forward, err := r.IsAncestor(request.Expected, request.Candidate)
	if err != nil {
		return PreparedComposition{}, err
	}
	if forward {
		tree, err := r.TreeOID(request.Candidate)
		if err != nil {
			return PreparedComposition{}, err
		}
		parents, err := r.Parents(request.Candidate)
		if err != nil {
			return PreparedComposition{}, err
		}
		return PreparedComposition{Mode: FastForward, Commit: request.Candidate, Tree: tree, Parents: parents}, nil
	}
	contained, err := r.IsAncestor(request.Candidate, request.Expected)
	if err != nil {
		return PreparedComposition{}, err
	}
	if contained {
		return PreparedComposition{}, fail("CANDIDATE_ALREADY_CONTAINED", "prepare composition", errors.New("candidate is already contained by expected"))
	}
	tree, err := r.deterministicMergeTree(request.Expected, request.Candidate)
	if err != nil {
		return PreparedComposition{}, err
	}
	expectedTime, err := r.CommitTimestamp(request.Expected)
	if err != nil {
		return PreparedComposition{}, err
	}
	candidateTime, err := r.CommitTimestamp(request.Candidate)
	if err != nil {
		return PreparedComposition{}, err
	}
	timestamp := expectedTime
	if candidateTime > timestamp {
		timestamp = candidateTime
	}
	timestamp++
	rawCommit, err := r.run(
		[]byte(request.Message),
		commitEnvironment(request.Identity, timestamp),
		"commit-tree", tree.String(), "-p", request.Expected.String(), "-p", request.Candidate.String(),
	)
	if err != nil {
		return PreparedComposition{}, err
	}
	commit, err := r.parseOID(string(rawCommit))
	if err != nil {
		return PreparedComposition{}, err
	}
	return PreparedComposition{
		Mode:    TwoParent,
		Commit:  commit,
		Tree:    tree,
		Parents: []OID{request.Expected, request.Candidate},
	}, nil
}
