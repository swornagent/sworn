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
func (r *Repository) prepareRecord(request RecordRequest) (PreparedCommit, error) {
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

type RecordTransitionRequest struct {
	ExpectedHead     OID
	Changes          []BlobChange
	Message          string
	RecordAdmission  *RecordPathAdmission
	ProductAdmission *ProductExclusionAdmission
}

func (r *Repository) PrepareRecordTransition(request RecordTransitionRequest) (PreparedCommit, error) {
	if err := r.requireRecordAdmission(request.RecordAdmission); err != nil {
		return PreparedCommit{}, err
	}
	before, err := r.ProductTreeIdentity(request.ExpectedHead, request.ProductAdmission)
	if err != nil {
		return PreparedCommit{}, err
	}
	if len(request.Changes) == 0 || len(request.Changes) > MaxBatchPaths {
		return PreparedCommit{}, fail("EMPTY_RECORD_TRANSITION", "prepare record transition", errors.New("one bounded change set is required"))
	}
	for _, change := range request.Changes {
		if change.Path == recordRoot || !strings.HasPrefix(change.Path, recordRoot+"/") {
			return PreparedCommit{}, fail("NON_RECORD_CHANGE", "prepare record transition", fmt.Errorf("%s is outside %s", change.Path, recordRoot))
		}
	}
	if strings.TrimSpace(request.Message) == "" || len([]byte(strings.TrimSpace(request.Message))) > 1_000 {
		return PreparedCommit{}, fail("COMMIT_MESSAGE_LIMIT", "prepare record transition", errors.New("message must be 1-1000 UTF-8 bytes"))
	}
	timestamp, err := r.CommitTimestamp(request.ExpectedHead)
	if err != nil {
		return PreparedCommit{}, err
	}
	prepared, err := r.prepareRecord(RecordRequest{
		Parent: request.ExpectedHead, Changes: request.Changes,
		Message:   strings.TrimSpace(request.Message) + "\n",
		Identity:  Identity{Name: "Baton Records", Email: "records@baton.invalid"},
		Timestamp: timestamp + 1,
	})
	if err != nil {
		return PreparedCommit{}, err
	}
	if err := r.assertRecordRootAt(prepared.Commit, false); err != nil {
		return PreparedCommit{}, fail("RECORD_ROOT_REPLACED", "prepare record transition", err)
	}
	after, err := r.ProductTreeIdentity(prepared.Commit, request.ProductAdmission)
	if err != nil {
		return PreparedCommit{}, err
	}
	if before.ProductTree != after.ProductTree {
		return PreparedCommit{}, fail("PRODUCT_CHANGED_DURING_RECORD_TRANSITION", "prepare record transition", errors.New("product identity changed"))
	}
	return prepared, nil
}

type MetadataRequest struct {
	ExpectedHead OID
	Message      []byte
}

func (r *Repository) PrepareMetadataCommit(request MetadataRequest) (PreparedCommit, error) {
	if err := r.validateOID(request.ExpectedHead); err != nil {
		return PreparedCommit{}, err
	}
	if len(request.Message) == 0 || len(request.Message) > MaxMessageBytes ||
		bytes.IndexByte(request.Message, 0) >= 0 || bytes.IndexByte(request.Message, '\r') >= 0 {
		return PreparedCommit{}, fail("INVALID_COMMIT_MESSAGE", "prepare metadata commit", fmt.Errorf("message must be 1-%d LF-only bytes without NUL", MaxMessageBytes))
	}
	tree, err := r.TreeOID(request.ExpectedHead)
	if err != nil {
		return PreparedCommit{}, err
	}
	timestamp, err := r.CommitTimestamp(request.ExpectedHead)
	if err != nil {
		return PreparedCommit{}, err
	}
	raw, err := r.run(
		request.Message,
		commitEnvironment(
			Identity{Name: "Baton Receipts", Email: "receipts@baton.invalid"},
			timestamp+1,
		),
		"commit-tree", tree.String(), "-p", request.ExpectedHead.String(),
	)
	if err != nil {
		return PreparedCommit{}, err
	}
	commit, err := r.parseOID(string(raw))
	if err != nil {
		return PreparedCommit{}, err
	}
	parents, err := r.Parents(commit)
	if err != nil {
		return PreparedCommit{}, err
	}
	observedTree, err := r.TreeOID(commit)
	if err != nil {
		return PreparedCommit{}, err
	}
	if len(parents) != 1 || parents[0] != request.ExpectedHead || observedTree != tree {
		return PreparedCommit{}, fail("INVALID_METADATA_COMMIT", "prepare metadata commit", errors.New("prepared commit changed parent or tree"))
	}
	return PreparedCommit{Commit: commit, Tree: tree, Parents: parents}, nil
}

type CompositionMode string

const (
	FastForward CompositionMode = "fast-forward"
	TwoParent   CompositionMode = "two-parent"
)

type CompositionRequest struct {
	Expected         OID
	Candidate        OID
	TargetRef        string
	ProductAdmission *ProductExclusionAdmission
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
func unionTreePaths(groups ...[]TreeEntry) []string {
	values := make(map[string]struct{})
	for _, group := range groups {
		for _, entry := range group {
			values[entry.Path] = struct{}{}
		}
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
	// Apple Git does not provide check-attr --source. The composition index is
	// private, so loading the exact source tree and asking only that index is
	// equivalent without consulting the worktree or ambient attributes.
	if _, err := r.contextRun(context, nil, "/dev/null", "read-tree", source.String()); err != nil {
		return nil, fail("UNTRUSTED_MERGE_ATTRIBUTES", "load merge attribute source", err)
	}
	var input bytes.Buffer
	for _, name := range paths {
		input.WriteString(name)
		input.WriteByte(0)
	}
	raw, err := r.contextRun(context, input.Bytes(), "/dev/null", "check-attr", "-z", "--stdin", "--cached", "merge")
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

func (r *Repository) explicitBaseMergeHead(
	context *compositionContext,
	source OID,
	productBase OID,
	side string,
	timestamp int64,
) (OID, error) {
	tree, err := r.TreeOID(source)
	if err != nil {
		return OID{}, err
	}
	environment := append(
		context.environment(),
		commitEnvironment(
			Identity{Name: "Baton Merge Base", Email: "merge-base@baton.invalid"},
			timestamp,
		)...,
	)
	raw, err := r.runWithAttributes(
		[]byte("Baton explicit product base for "+side+"\n"),
		environment,
		"/dev/null",
		"commit-tree", tree.String(), "-p", productBase.String(),
	)
	if err != nil {
		return OID{}, fail("GIT_EXECUTION_FAILED", "prepare explicit merge base", err)
	}
	return r.parseOID(string(raw))
}

func (r *Repository) deterministicMergeTree(expected, candidate OID, productBase *OID) (OID, error) {
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
	var base []TreeEntry
	if productBase != nil {
		base, err = r.ListTree(*productBase)
		if err != nil {
			return OID{}, err
		}
	}
	paths := unionTreePaths(left, right, base)
	expectedAttributes, err := r.mergeAttributesAtSource(context, expected, paths)
	if err != nil {
		return OID{}, err
	}
	if _, err := r.mergeAttributesAtSource(context, candidate, paths); err != nil {
		return OID{}, err
	}
	if productBase != nil {
		if _, err := r.mergeAttributesAtSource(context, *productBase, paths); err != nil {
			return OID{}, err
		}
	}
	if err := os.WriteFile(context.attributesFile, renderMergeAttributes(expectedAttributes), 0o600); err != nil {
		return OID{}, fail("GIT_EXECUTION_FAILED", "install merge attributes", err)
	}
	if _, err := r.contextRun(context, nil, context.attributesFile, "read-tree", expected.String()); err != nil {
		return OID{}, err
	}
	args := []string{"merge-tree", "--write-tree", "--no-messages"}
	mergeExpected, mergeCandidate := expected, candidate
	if productBase != nil {
		baseTime, err := r.CommitTimestamp(*productBase)
		if err != nil {
			return OID{}, err
		}
		mergeExpected, err = r.explicitBaseMergeHead(
			context, expected, *productBase, "expected", baseTime+1,
		)
		if err != nil {
			return OID{}, err
		}
		mergeCandidate, err = r.explicitBaseMergeHead(
			context, candidate, *productBase, "candidate", baseTime+2,
		)
		if err != nil {
			return OID{}, err
		}
	}
	args = append(args, mergeExpected.String(), mergeCandidate.String())
	rawTree, err := r.contextRun(context, nil, context.attributesFile, args...)
	if err != nil {
		return OID{}, fail("MERGE_CONFLICT", "prepare composition", err)
	}
	treeText := strings.TrimSpace(string(rawTree))
	if strings.ContainsAny(treeText, " \t\n") {
		return OID{}, fail("INVALID_GIT_OUTPUT", "prepare composition", errors.New("merge-tree returned extra output"))
	}
	return r.parseOID(treeText)
}

func (r *Repository) validateCompositionRequest(request CompositionRequest) error {
	if err := r.validateOID(request.Expected); err != nil {
		return err
	}
	if err := r.validateOID(request.Candidate); err != nil {
		return err
	}
	if err := ValidateHeadRef(request.TargetRef); err != nil {
		return err
	}
	if _, err := r.ProductTreeIdentity(request.Expected, request.ProductAdmission); err != nil {
		return err
	}
	if _, err := r.ProductTreeIdentity(request.Candidate, request.ProductAdmission); err != nil {
		return err
	}
	return nil
}

func (r *Repository) PrepareComposition(request CompositionRequest) (PreparedComposition, error) {
	if err := r.validateCompositionRequest(request); err != nil {
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
		prepared := PreparedComposition{Mode: FastForward, Commit: request.Candidate, Tree: tree, Parents: parents}
		if _, err := r.ProductTreeIdentity(prepared.Commit, request.ProductAdmission); err != nil {
			return PreparedComposition{}, err
		}
		if err := r.VerifyExactComposition(request.Expected, request.Candidate, prepared.Commit); err != nil {
			return PreparedComposition{}, err
		}
		return prepared, nil
	}
	contained, err := r.IsAncestor(request.Candidate, request.Expected)
	if err != nil {
		return PreparedComposition{}, err
	}
	if contained {
		return PreparedComposition{}, fail("CANDIDATE_ALREADY_CONTAINED", "prepare composition", errors.New("candidate is already contained by expected"))
	}
	return r.prepareTwoParentComposition(request, nil)
}

func (r *Repository) prepareTwoParentComposition(
	request CompositionRequest,
	productBase *OID,
) (PreparedComposition, error) {
	tree, err := r.deterministicMergeTree(request.Expected, request.Candidate, productBase)
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
		[]byte(fmt.Sprintf("Baton exact composition of %s into %s\n", request.Candidate.String(), request.TargetRef)),
		commitEnvironment(Identity{Name: "Baton Merge", Email: "merge@baton.invalid"}, timestamp),
		"commit-tree", tree.String(), "-p", request.Expected.String(), "-p", request.Candidate.String(),
	)
	if err != nil {
		return PreparedComposition{}, err
	}
	commit, err := r.parseOID(string(rawCommit))
	if err != nil {
		return PreparedComposition{}, err
	}
	prepared := PreparedComposition{
		Mode:    TwoParent,
		Commit:  commit,
		Tree:    tree,
		Parents: []OID{request.Expected, request.Candidate},
	}
	if _, err := r.ProductTreeIdentity(prepared.Commit, request.ProductAdmission); err != nil {
		return PreparedComposition{}, err
	}
	if err := r.verifyExactComposition(request.Expected, request.Candidate, prepared.Commit, productBase); err != nil {
		return PreparedComposition{}, err
	}
	return prepared, nil
}

func (r *Repository) VerifyExactComposition(expected, candidate, result OID) error {
	return r.verifyExactComposition(expected, candidate, result, nil)
}

func (r *Repository) verifyExactComposition(expected, candidate, result OID, productBase *OID) error {
	if err := r.validateOID(expected); err != nil {
		return err
	}
	if err := r.validateOID(candidate); err != nil {
		return err
	}
	if err := r.validateOID(result); err != nil {
		return err
	}
	if result == candidate {
		ancestor, err := r.IsAncestor(expected, candidate)
		if err != nil {
			return err
		}
		if !ancestor {
			return fail("INVALID_COMPOSITION", "verify composition", errors.New("fast-forward result lacks expected ancestry"))
		}
		return nil
	}
	parents, err := r.Parents(result)
	if err != nil {
		return err
	}
	if len(parents) != 2 || parents[0] != expected || parents[1] != candidate {
		return fail("INVALID_COMPOSITION", "verify composition", errors.New("two-parent result has wrong parents"))
	}
	expectedTree, err := r.deterministicMergeTree(expected, candidate, productBase)
	if err != nil {
		return err
	}
	resultTree, err := r.TreeOID(result)
	if err != nil {
		return err
	}
	if resultTree != expectedTree {
		return fail("INVALID_COMPOSITION", "verify composition", errors.New("result tree is not the deterministic merge tree"))
	}
	return nil
}

// PrepareProductComposition first attempts an ordinary exact composition. The
// resolver is called only when that attempt reports MERGE_CONFLICT; callers
// cannot supply or override a raw merge base on the ordinary path.
func (r *Repository) PrepareProductComposition(
	request CompositionRequest,
	resolveProductBase func() (OID, error),
) (PreparedComposition, error) {
	prepared, err := r.PrepareComposition(request)
	if err == nil {
		return prepared, nil
	}
	var typed *Error
	if !errors.As(err, &typed) || typed.Code != "MERGE_CONFLICT" {
		return PreparedComposition{}, err
	}
	if resolveProductBase == nil {
		return PreparedComposition{}, fail(
			"PRODUCT_BASE_RESOLVER_REQUIRED",
			"prepare product composition",
			errors.New("conflicting product composition requires an engine-derived base"),
		)
	}
	productBase, err := resolveProductBase()
	if err != nil {
		return PreparedComposition{}, err
	}
	if err := r.validateOID(productBase); err != nil {
		return PreparedComposition{}, err
	}
	if _, err := r.ProductTreeIdentity(productBase, request.ProductAdmission); err != nil {
		return PreparedComposition{}, err
	}
	return r.prepareTwoParentComposition(request, &productBase)
}

// PrepareApprovedTargetBase incorporates the approved target while preserving
// Expected as first-parent authority when Candidate contains it only off the
// first-parent chain.
func (r *Repository) PrepareApprovedTargetBase(
	request CompositionRequest,
) (PreparedComposition, error) {
	if err := r.validateCompositionRequest(request); err != nil {
		return PreparedComposition{}, err
	}
	if request.Expected == request.Candidate {
		tree, err := r.TreeOID(request.Expected)
		if err != nil {
			return PreparedComposition{}, err
		}
		parents, err := r.Parents(request.Expected)
		if err != nil {
			return PreparedComposition{}, err
		}
		return PreparedComposition{
			Mode: FastForward, Commit: request.Expected,
			Tree: tree, Parents: parents,
		}, nil
	}
	targetContained, err := r.IsAncestor(request.Candidate, request.Expected)
	if err != nil {
		return PreparedComposition{}, err
	}
	if targetContained {
		tree, err := r.TreeOID(request.Expected)
		if err != nil {
			return PreparedComposition{}, err
		}
		parents, err := r.Parents(request.Expected)
		if err != nil {
			return PreparedComposition{}, err
		}
		return PreparedComposition{
			Mode: FastForward, Commit: request.Expected,
			Tree: tree, Parents: parents,
		}, nil
	}
	expectedContained, err := r.IsAncestor(request.Expected, request.Candidate)
	if err != nil {
		return PreparedComposition{}, err
	}
	if !expectedContained {
		return r.PrepareComposition(request)
	}
	history, err := r.ReadFirstParentHistory(request.Candidate, MaxHistory)
	if err != nil {
		return PreparedComposition{}, err
	}
	for _, entry := range history {
		if entry.OID == request.Expected {
			return r.PrepareComposition(request)
		}
	}
	return r.prepareTwoParentComposition(request, nil)
}
