package baton

import (
	"errors"
	"fmt"
	"github.com/swornagent/sworn/internal/gitx"
)

type CapturedRef struct{ Ref, State, Head, Target string }
type RepositoryTreeEntry struct{ Path, Mode, Type, OID string }
type RepositoryChange struct {
	Path   string
	Bytes  []byte
	Delete bool
}
type PrepareRecordRequest struct {
	Parent    string
	Changes   []RepositoryChange
	Message   string
	Author    string
	Email     string
	Timestamp int64
}
type PreparedRecord struct {
	Commit  string
	Tree    string
	Parents []string
}
type PrepareCompositionRequest struct{ Expected, Candidate, Message, Author, Email string }
type PreparedComposition struct {
	Mode    string
	Commit  string
	Tree    string
	Parents []string
}
type RefOperation struct{ Kind, Ref, NewHead, ExpectedHead string }
type Repository interface {
	Root() string
	GitExecutable() string
	ObjectFormat() string
	CaptureHeadRefs([]string) ([]CapturedRef, error)
	ReadBlob(commit, path string) ([]byte, error)
	ReadBlobs(commit string, paths []string) (map[string][]byte, error)
	ListTree(commit string) ([]RepositoryTreeEntry, error)
	TreeOID(commit string) (string, error)
	ProductTreeIdentity(commit string) (string, error)
	Parents(commit string) ([]string, error)
	CommitTimestamp(commit string) (int64, error)
	IsAncestor(ancestor, descendant string) (bool, error)
	ChangedPaths(base, candidate string) ([]string, error)
	FirstParentHistory(base, head string) ([]string, error)
	PrepareRecord(PrepareRecordRequest) (PreparedRecord, error)
	PrepareComposition(PrepareCompositionRequest) (PreparedComposition, error)
	AtomicUpdateRefs([]CapturedRef, []RefOperation) error
}
type gitRepository struct {
	repository *gitx.Repository
}

func UseGitRepository(repository *gitx.Repository) Repository {
	if repository == nil {
		return nil
	}
	return &gitRepository{repository: repository}
}
func (r *gitRepository) Root() string          { return r.repository.Root() }
func (r *gitRepository) GitExecutable() string { return r.repository.GitExecutable() }
func (r *gitRepository) ObjectFormat() string  { return string(r.repository.ObjectFormat()) }
func (r *gitRepository) oid(value string) (gitx.OID, error) {
	return gitx.ParseOID(r.repository.ObjectFormat(), value)
}
func (r *gitRepository) CaptureHeadRefs(refs []string) ([]CapturedRef, error) {
	values, err := r.repository.CaptureHeadRefs(refs)
	if err != nil {
		return nil, err
	}
	result := make([]CapturedRef, len(values))
	for index, value := range values {
		result[index] = CapturedRef{Ref: value.Ref, State: string(value.State), Target: value.Target}
		if !value.Head.IsZero() {
			result[index].Head = value.Head.String()
		}
	}
	return result, nil
}
func (r *gitRepository) ReadBlob(commit, path string) ([]byte, error) {
	oid, err := r.oid(commit)
	if err != nil {
		return nil, err
	}
	return r.repository.ReadBlob(oid, path)
}
func (r *gitRepository) ReadBlobs(commit string, paths []string) (map[string][]byte, error) {
	oid, err := r.oid(commit)
	if err != nil {
		return nil, err
	}
	return r.repository.ReadBlobs(oid, paths)
}
func (r *gitRepository) ListTree(commit string) ([]RepositoryTreeEntry, error) {
	oid, err := r.oid(commit)
	if err != nil {
		return nil, err
	}
	values, err := r.repository.ListTree(oid)
	if err != nil {
		return nil, err
	}
	result := make([]RepositoryTreeEntry, len(values))
	for index, value := range values {
		result[index] = RepositoryTreeEntry{Path: value.Path, Mode: value.Mode, Type: value.Type, OID: value.OID.String()}
	}
	return result, nil
}
func (r *gitRepository) TreeOID(commit string) (string, error) {
	oid, err := r.oid(commit)
	if err != nil {
		return "", err
	}
	tree, err := r.repository.TreeOID(oid)
	if err != nil {
		return "", err
	}
	return tree.String(), nil
}
func (r *gitRepository) ProductTreeIdentity(commit string) (string, error) {
	oid, err := r.oid(commit)
	if err != nil {
		return "", err
	}
	return r.repository.ProductTreeIdentity(oid)
}
func (r *gitRepository) Parents(commit string) ([]string, error) {
	oid, err := r.oid(commit)
	if err != nil {
		return nil, err
	}
	values, err := r.repository.Parents(oid)
	if err != nil {
		return nil, err
	}
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = value.String()
	}
	return result, nil
}
func (r *gitRepository) CommitTimestamp(commit string) (int64, error) {
	oid, err := r.oid(commit)
	if err != nil {
		return 0, err
	}
	return r.repository.CommitTimestamp(oid)
}
func (r *gitRepository) IsAncestor(ancestor, descendant string) (bool, error) {
	left, err := r.oid(ancestor)
	if err != nil {
		return false, err
	}
	right, err := r.oid(descendant)
	if err != nil {
		return false, err
	}
	return r.repository.IsAncestor(left, right)
}
func (r *gitRepository) ChangedPaths(base, candidate string) ([]string, error) {
	left, err := r.oid(base)
	if err != nil {
		return nil, err
	}
	right, err := r.oid(candidate)
	if err != nil {
		return nil, err
	}
	return r.repository.ChangedPaths(left, right)
}
func (r *gitRepository) FirstParentHistory(base, head string) ([]string, error) {
	left, err := r.oid(base)
	if err != nil {
		return nil, err
	}
	right, err := r.oid(head)
	if err != nil {
		return nil, err
	}
	values, err := r.repository.FirstParentHistory(left, right)
	if err != nil {
		return nil, err
	}
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = value.String()
	}
	return result, nil
}
func (r *gitRepository) PrepareRecord(request PrepareRecordRequest) (PreparedRecord, error) {
	parent, err := r.oid(request.Parent)
	if err != nil {
		return PreparedRecord{}, err
	}
	changes := make([]gitx.BlobChange, len(request.Changes))
	for index, value := range request.Changes {
		changes[index] = gitx.BlobChange{Path: value.Path, Bytes: append([]byte(nil), value.Bytes...), Delete: value.Delete}
	}
	value, err := r.repository.PrepareRecord(gitx.RecordRequest{
		Parent: parent, Changes: changes, Message: request.Message,
		Identity:  gitx.Identity{Name: request.Author, Email: request.Email},
		Timestamp: request.Timestamp,
	})
	if err != nil {
		return PreparedRecord{}, err
	}
	result := PreparedRecord{Commit: value.Commit.String(), Tree: value.Tree.String(), Parents: make([]string, len(value.Parents))}
	for index, parent := range value.Parents {
		result.Parents[index] = parent.String()
	}
	return result, nil
}
func (r *gitRepository) PrepareComposition(request PrepareCompositionRequest) (PreparedComposition, error) {
	expected, err := r.oid(request.Expected)
	if err != nil {
		return PreparedComposition{}, err
	}
	candidate, err := r.oid(request.Candidate)
	if err != nil {
		return PreparedComposition{}, err
	}
	value, err := r.repository.PrepareComposition(gitx.CompositionRequest{
		Expected: expected, Candidate: candidate, Message: request.Message,
		Identity: gitx.Identity{Name: request.Author, Email: request.Email},
	})
	if err != nil {
		return PreparedComposition{}, err
	}
	result := PreparedComposition{Mode: string(value.Mode), Commit: value.Commit.String(), Tree: value.Tree.String(), Parents: make([]string, len(value.Parents))}
	for index, parent := range value.Parents {
		result.Parents[index] = parent.String()
	}
	return result, nil
}
func (r *gitRepository) AtomicUpdateRefs(snapshot []CapturedRef, operations []RefOperation) error {
	captured := make([]gitx.RefHead, len(snapshot))
	for index, value := range snapshot {
		captured[index] = gitx.RefHead{Ref: value.Ref, State: gitx.RefState(value.State), Target: value.Target}
		if value.Head != "" {
			head, err := r.oid(value.Head)
			if err != nil {
				return err
			}
			captured[index].Head = head
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
			newHead, err := r.oid(operation.NewHead)
			if err != nil {
				return err
			}
			expected, err := r.oid(operation.ExpectedHead)
			if err != nil {
				return err
			}
			values[index].NewHead = &newHead
			values[index].Expected = &expected
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
			return fmt.Errorf("unknown ref operation %q", operation.Kind)
		}
	}
	if err := r.repository.ApplyRefTransaction(captured, values); err != nil {
		var typed *gitx.Error
		if errors.As(err, &typed) {
			return recordWrap(typed.Code, "exact ref transaction", err)
		}
		return err
	}
	return nil
}
