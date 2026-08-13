package baton

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/swornagent/sworn/internal/gitx"
)

type InertnessRequest = gitx.RecordRootRequest
type InertnessDecision = gitx.RecordRootDecision
type InertnessResolver = gitx.InertnessResolver

type CapturedRef struct {
	Ref    string
	State  string
	Head   string
	Target string
}

type repositoryFile struct {
	Path    string
	Mode    string
	Type    string
	Object  string
	Bytes   []byte
	Present bool
}

type historyRow struct {
	OID     string
	Parents []string
	Tree    string
	Message []byte
}

type preparedCommit struct {
	Commit  string
	Tree    string
	Parents []string
}

type preparedComposition struct {
	Mode    string
	Result  string
	Tree    string
	Parents []string
}

type refOperation struct {
	Kind         string
	Ref          string
	NewHead      string
	ExpectedHead string
}

type repository struct {
	git        *gitx.Repository
	record     *gitx.RecordPathAdmission
	product    *gitx.ProductExclusionAdmission
	identity   gitx.Identity
	identities *historicalIdentityCache
}

type historicalIdentityCache struct {
	mu     sync.Mutex
	values map[string]gitx.Identity
}

// GitRepository is an admitted handle, not a raw command surface. It exposes
// no methods by design (TestExternalActionSurfaceIsExactlyTheFourBatonFacades
// pins the zero-method surface); contract reads go through free functions and
// Plan.ResolveSliceContractAt.
type GitRepository struct {
	value *gitx.Repository
}

func UseGitRepository(value *gitx.Repository) GitRepository {
	return GitRepository{value: value}
}

func (r GitRepository) repository() *gitx.Repository {
	return r.value
}

// readGitFileAt returns the exact bytes of one path at an exact commit, with a
// present flag distinguishing a missing path from an I/O failure. It is the
// read-only surface the host-check runner uses to resolve the human-approved
// slice contract at the exact captured head: the bytes it returns are then
// proven against the plan's declared digest through
// Plan.ResolveSliceContract, never trusted on their own. It performs no
// mutation and admits nothing.
func readGitFileAt(repository GitRepository, commit, path string) ([]byte, bool, error) {
	if repository.value == nil {
		return nil, false, recordFail("INVALID_REPOSITORY", "one admitted Git repository is required")
	}
	oid, err := gitx.ParseOID(repository.value.ObjectFormat(), commit)
	if err != nil {
		return nil, false, translateGitError("parse commit identity", err)
	}
	entries, err := repository.value.ListTree(oid)
	if err != nil {
		return nil, false, translateGitError("inventory tree", err)
	}
	for _, entry := range entries {
		if entry.Path != path {
			continue
		}
		if entry.Type != "blob" {
			return nil, false, recordFail("NONREGULAR_RECORD", "path "+path+" is not a blob")
		}
		body, err := repository.value.ReadBlob(oid, path)
		if err != nil {
			return nil, false, translateGitError("read file", err)
		}
		return append([]byte(nil), body...), true, nil
	}
	return nil, false, nil
}

func newRepository(value *gitx.Repository, resolver InertnessResolver, identities ...gitx.Identity) (*repository, error) {
	if value == nil {
		return nil, recordFail("INVALID_REPOSITORY", "one admitted Git repository is required")
	}
	if len(identities) > 1 {
		return nil, recordFail("INVALID_GIT_IDENTITY", "only one Git identity is accepted")
	}
	var identity gitx.Identity
	if len(identities) == 1 {
		identity = identities[0]
		if err := gitx.ValidateIdentity(identity); err != nil {
			return nil, recordWrap("INVALID_GIT_IDENTITY", "admit Git identity", err)
		}
	}
	record, err := value.ResolveRecordPathAdmission()
	if err != nil {
		return nil, translateGitError("admit record path", err)
	}
	product, err := value.ResolveProductExclusion(record, resolver)
	if err != nil {
		return nil, translateGitError("admit product exclusion", err)
	}
	return &repository{
		git: value, record: record, product: product, identity: identity,
		identities: &historicalIdentityCache{values: make(map[string]gitx.Identity)},
	}, nil
}

func (r *repository) root() string { return r.git.Root() }

func (r *repository) objectFormat() string { return string(r.git.ObjectFormat()) }

func (r *repository) oid(value string) (gitx.OID, error) {
	oid, err := gitx.ParseOID(r.git.ObjectFormat(), value)
	if err != nil {
		return gitx.OID{}, translateGitError("parse object identity", err)
	}
	return oid, nil
}

func (r *repository) capture(refs []string) ([]CapturedRef, error) {
	values, err := r.git.CaptureHeadRefs(refs)
	if err != nil {
		return nil, translateGitError("capture refs", err)
	}
	result := make([]CapturedRef, len(values))
	for index, value := range values {
		result[index] = CapturedRef{
			Ref: value.Ref, State: string(value.State), Target: value.Target,
		}
		if !value.Head.IsZero() {
			result[index].Head = value.Head.String()
		}
	}
	return result, nil
}

func directCommit(value CapturedRef) bool {
	return value.State == string(gitx.RefDirect) && value.Head != "" && value.Target == ""
}

func absentRef(value CapturedRef) bool {
	return value.State == string(gitx.RefAbsent) && value.Head == "" && value.Target == ""
}

func (r *repository) files(commit string, paths []string) ([]repositoryFile, error) {
	oid, err := r.oid(commit)
	if err != nil {
		return nil, err
	}
	entries, err := r.git.ListTree(oid)
	if err != nil {
		return nil, translateGitError("inventory tree", err)
	}
	byPath := make(map[string]gitx.TreeEntry, len(entries))
	for _, entry := range entries {
		byPath[entry.Path] = entry
	}
	result := make([]repositoryFile, len(paths))
	var presentPaths []string
	for index, name := range paths {
		result[index].Path = name
		entry, ok := byPath[name]
		if !ok {
			continue
		}
		result[index].Present = true
		result[index].Mode, result[index].Type = entry.Mode, entry.Type
		result[index].Object = entry.OID.String()
		if entry.Type != "blob" {
			return nil, recordFail("NONREGULAR_RECORD", "record "+name+" is not a blob")
		}
		presentPaths = append(presentPaths, name)
	}
	if len(presentPaths) == 0 {
		return result, nil
	}
	bodies, err := r.git.ReadBlobs(oid, presentPaths)
	if err != nil {
		return nil, translateGitError("read files", err)
	}
	for index := range result {
		if result[index].Present {
			result[index].Bytes = append([]byte(nil), bodies[result[index].Path]...)
		}
	}
	return result, nil
}

func (r *repository) file(commit, path string) (repositoryFile, error) {
	values, err := r.files(commit, []string{path})
	if err != nil {
		return repositoryFile{}, err
	}
	return values[0], nil
}

func (r *repository) history(head string) ([]historyRow, error) {
	oid, err := r.oid(head)
	if err != nil {
		return nil, err
	}
	rows, err := r.git.ReadFirstParentHistory(oid, gitx.MaxHistory)
	if err != nil {
		return nil, translateGitError("read first-parent history", err)
	}
	result := make([]historyRow, len(rows))
	for index, row := range rows {
		engineCommit := bytes.HasPrefix(row.Message, []byte("baton(")) ||
			bytes.HasPrefix(row.Message, []byte("Baton exact ")) ||
			bytes.HasPrefix(row.Message, []byte("Baton engine-owned ")) ||
			bytes.HasPrefix(row.Message, []byte("sworn("))
		if engineCommit {
			if row.AuthorIdentity != row.CommitterIdentity {
				return nil, recordFail(
					"INVALID_COMMIT_IDENTITY",
					"historical engine author and committer identities differ",
				)
			}
			if err := gitx.ValidateIdentity(row.AuthorIdentity); err != nil {
				return nil, translateGitError("validate historical engine identity", err)
			}
		}
		if row.AuthorIdentity == row.CommitterIdentity &&
			gitx.ValidateIdentity(row.AuthorIdentity) == nil {
			r.identities.mu.Lock()
			r.identities.values[row.OID.String()] = row.AuthorIdentity
			r.identities.mu.Unlock()
		}
		result[index] = historyRow{
			OID: row.OID.String(), Tree: row.Tree.String(),
			Message: append([]byte(nil), row.Message...),
			Parents: make([]string, len(row.Parents)),
		}
		for parentIndex, parent := range row.Parents {
			result[index].Parents[parentIndex] = parent.String()
		}
	}
	return result, nil
}

func (r *repository) withHistoricalIdentity(commit string) (*repository, error) {
	r.identities.mu.Lock()
	identity, present := r.identities.values[commit]
	r.identities.mu.Unlock()
	if present {
		copy := *r
		copy.identity = identity
		return &copy, nil
	}
	oid, err := r.oid(commit)
	if err != nil {
		return nil, err
	}
	identity, err = r.git.CommitIdentity(oid)
	if err != nil {
		return nil, translateGitError("recover historical Git identity", err)
	}
	copy := *r
	copy.identity = identity
	r.identities.mu.Lock()
	r.identities.values[commit] = identity
	r.identities.mu.Unlock()
	return &copy, nil
}

func (r *repository) firstParentPathChange(head, path string) (string, error) {
	oid, err := r.oid(head)
	if err != nil {
		return "", err
	}
	change, present, err := r.git.FirstParentPathChange(oid, path)
	if err != nil {
		return "", translateGitError("read first-parent path history", err)
	}
	if !present {
		return "", nil
	}
	return change.String(), nil
}

func (r *repository) productTree(commit string) (string, error) {
	oid, err := r.oid(commit)
	if err != nil {
		return "", err
	}
	value, err := r.git.ProductTreeIdentity(oid, r.product)
	if err != nil {
		return "", translateGitError("derive product identity", err)
	}
	return value.ProductTree, nil
}

func (r *repository) tree(commit string) (string, error) {
	oid, err := r.oid(commit)
	if err != nil {
		return "", err
	}
	value, err := r.git.TreeOID(oid)
	if err != nil {
		return "", translateGitError("resolve tree", err)
	}
	return value.String(), nil
}

func (r *repository) parents(commit string) ([]string, error) {
	oid, err := r.oid(commit)
	if err != nil {
		return nil, err
	}
	values, err := r.git.Parents(oid)
	if err != nil {
		return nil, translateGitError("read parents", err)
	}
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = value.String()
	}
	return result, nil
}

func (r *repository) isAncestor(ancestor, descendant string) (bool, error) {
	left, err := r.oid(ancestor)
	if err != nil {
		return false, err
	}
	right, err := r.oid(descendant)
	if err != nil {
		return false, err
	}
	value, err := r.git.IsAncestor(left, right)
	if err != nil {
		return false, translateGitError("check ancestry", err)
	}
	return value, nil
}

func (r *repository) changedPaths(base, candidate string) ([]string, error) {
	left, err := r.oid(base)
	if err != nil {
		return nil, err
	}
	right, err := r.oid(candidate)
	if err != nil {
		return nil, err
	}
	value, err := r.git.ChangedPaths(left, right)
	if err != nil {
		return nil, translateGitError("read changed paths", err)
	}
	return value, nil
}

func (r *repository) assertCandidateRecordRootUnchanged(base, candidate string) error {
	left, err := r.oid(base)
	if err != nil {
		return err
	}
	right, err := r.oid(candidate)
	if err != nil {
		return err
	}
	if err := r.git.AssertCandidateRecordRootUnchanged(left, right); err != nil {
		return translateGitError("compare candidate reserved record root", err)
	}
	return nil
}

func (r *repository) prepareRecord(parent, message string, changes map[string][]byte) (preparedCommit, error) {
	expected, err := r.oid(parent)
	if err != nil {
		return preparedCommit{}, err
	}
	names := make([]string, 0, len(changes))
	for name := range changes {
		names = append(names, name)
	}
	sort.Strings(names)
	values := make([]gitx.BlobChange, 0, len(names))
	for _, name := range names {
		bytes := changes[name]
		values = append(values, gitx.BlobChange{
			Path: name, Bytes: append([]byte(nil), bytes...), Delete: bytes == nil,
		})
	}
	prepared, err := r.git.PrepareRecordTransition(gitx.RecordTransitionRequest{
		ExpectedHead: expected, Changes: values, Message: message,
		Identity:        r.identity,
		RecordAdmission: r.record, ProductAdmission: r.product,
	})
	if err != nil {
		return preparedCommit{}, translateGitError("prepare record transition", err)
	}
	return convertPrepared(prepared), nil
}

func (r *repository) prepareMetadata(parent string, message []byte) (preparedCommit, error) {
	expected, err := r.oid(parent)
	if err != nil {
		return preparedCommit{}, err
	}
	prepared, err := r.git.PrepareMetadataCommit(gitx.MetadataRequest{
		ExpectedHead: expected, Message: append([]byte(nil), message...), Identity: r.identity,
	})
	if err != nil {
		return preparedCommit{}, translateGitError("prepare metadata commit", err)
	}
	return convertPrepared(prepared), nil
}

func convertPrepared(value gitx.PreparedCommit) preparedCommit {
	result := preparedCommit{
		Commit: value.Commit.String(), Tree: value.Tree.String(),
		Parents: make([]string, len(value.Parents)),
	}
	for index, parent := range value.Parents {
		result.Parents[index] = parent.String()
	}
	return result
}

func (r *repository) prepareComposition(targetRef, expected, candidate string) (preparedComposition, error) {
	expectedOID, err := r.oid(expected)
	if err != nil {
		return preparedComposition{}, err
	}
	candidateOID, err := r.oid(candidate)
	if err != nil {
		return preparedComposition{}, err
	}
	prepared, err := r.git.PrepareComposition(gitx.CompositionRequest{
		Expected: expectedOID, Candidate: candidateOID, TargetRef: targetRef,
		Identity:         r.identity,
		ProductAdmission: r.product,
	})
	if err != nil {
		return preparedComposition{}, translateGitError("prepare exact composition", err)
	}
	return convertComposition(prepared), nil
}

func (r *repository) prepareProductComposition(
	targetRef, expected, candidate string,
	resolveProductBase func() (string, error),
) (preparedComposition, error) {
	expectedOID, err := r.oid(expected)
	if err != nil {
		return preparedComposition{}, err
	}
	candidateOID, err := r.oid(candidate)
	if err != nil {
		return preparedComposition{}, err
	}
	prepared, err := r.git.PrepareProductComposition(
		gitx.CompositionRequest{
			Expected: expectedOID, Candidate: candidateOID,
			TargetRef: targetRef, Identity: r.identity, ProductAdmission: r.product,
		},
		func() (gitx.OID, error) {
			if resolveProductBase == nil {
				return gitx.OID{}, recordFail(
					"PRODUCT_BASE_RESOLVER_REQUIRED",
					"product composition requires engine-derived evidence",
				)
			}
			value, err := resolveProductBase()
			if err != nil {
				return gitx.OID{}, err
			}
			return r.oid(value)
		},
	)
	if err != nil {
		return preparedComposition{}, translateGitError(
			"prepare product composition",
			err,
		)
	}
	return convertComposition(prepared), nil
}

func (r *repository) prepareApprovedTargetBase(
	targetRef, expected, approvedTarget string,
) (preparedComposition, error) {
	expectedOID, err := r.oid(expected)
	if err != nil {
		return preparedComposition{}, err
	}
	targetOID, err := r.oid(approvedTarget)
	if err != nil {
		return preparedComposition{}, err
	}
	prepared, err := r.git.PrepareApprovedTargetBase(
		gitx.CompositionRequest{
			Expected: expectedOID, Candidate: targetOID,
			TargetRef: targetRef, Identity: r.identity, ProductAdmission: r.product,
		},
	)
	if err != nil {
		return preparedComposition{}, translateGitError(
			"prepare approved target base",
			err,
		)
	}
	return convertComposition(prepared), nil
}

func convertComposition(
	prepared gitx.PreparedComposition,
) preparedComposition {
	result := preparedComposition{
		Mode: string(prepared.Mode), Result: prepared.Commit.String(),
		Tree: prepared.Tree.String(), Parents: make([]string, len(prepared.Parents)),
	}
	for index, parent := range prepared.Parents {
		result.Parents[index] = parent.String()
	}
	return result
}

func (r *repository) verifyComposition(expected, candidate, result string) error {
	left, err := r.oid(expected)
	if err != nil {
		return err
	}
	right, err := r.oid(candidate)
	if err != nil {
		return err
	}
	observed, err := r.oid(result)
	if err != nil {
		return err
	}
	if err := r.git.VerifyExactComposition(left, right, observed); err != nil {
		return translateGitError("verify exact composition", err)
	}
	return nil
}

func (r *repository) updateRefs(snapshot []CapturedRef, operations []refOperation) error {
	captured := make([]gitx.RefHead, len(snapshot))
	for index, value := range snapshot {
		captured[index] = gitx.RefHead{
			Ref: value.Ref, State: gitx.RefState(value.State), Target: value.Target,
		}
		if value.Head != "" {
			oid, err := r.oid(value.Head)
			if err != nil {
				return err
			}
			captured[index].Head = oid
		}
	}
	values := make([]gitx.RefOperation, len(operations))
	for index, operation := range operations {
		values[index].Ref = operation.Ref
		switch operation.Kind {
		case "create":
			values[index].Kind = gitx.CreateRef
			oid, err := r.oid(operation.NewHead)
			if err != nil {
				return err
			}
			values[index].NewHead = &oid
		case "update":
			values[index].Kind = gitx.UpdateRef
			next, err := r.oid(operation.NewHead)
			if err != nil {
				return err
			}
			expected, err := r.oid(operation.ExpectedHead)
			if err != nil {
				return err
			}
			values[index].NewHead, values[index].Expected = &next, &expected
		case "verify":
			values[index].Kind = gitx.VerifyRef
			if operation.ExpectedHead != "" {
				expected, err := r.oid(operation.ExpectedHead)
				if err != nil {
					return err
				}
				values[index].Expected = &expected
			}
		default:
			return recordFail("INVALID_REF_TRANSACTION", fmt.Sprintf("unknown ref operation %q", operation.Kind))
		}
	}
	if err := r.git.ApplyRefTransaction(captured, values); err != nil {
		return translateGitError("apply exact ref transaction", err)
	}
	return nil
}

func translateGitError(operation string, err error) error {
	var typed *gitx.Error
	if errors.As(err, &typed) {
		return recordWrap(typed.Code, operation, err)
	}
	return recordWrap("GIT_EXECUTION_FAILED", operation, err)
}
