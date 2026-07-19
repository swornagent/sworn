package repo

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/swornagent/sworn/internal/workspace"
)

const candidateRefPrefix = "refs/sworn/v1/candidates/"

func (repository *Repository) Materialize(
	ctx context.Context,
	target Target,
	destination string,
) (workspace Workspace, err error) {
	if err := repository.validateTarget(target); err != nil {
		return Workspace{}, err
	}
	if err := repository.AssertTarget(ctx, target); err != nil {
		return Workspace{}, err
	}
	if err := repository.rejectGitlinks(ctx, target.Tree); err != nil {
		return Workspace{}, err
	}
	destination, err = repository.materializeTree(ctx, target.Tree, destination)
	if err != nil {
		return Workspace{}, err
	}
	return Workspace{
		RepositoryID: repository.binding.RepositoryID,
		Path:         destination,
		Target:       target,
	}, nil
}

// MaterializeCandidate recreates the exact retained candidate in a fresh
// plain workspace. It validates candidate objects, diff facts, and the durable
// retention ref, but deliberately does not require the mutable target branch to
// remain at the candidate's base commit.
func (repository *Repository) MaterializeCandidate(
	ctx context.Context,
	candidate Candidate,
	destination string,
	limits MaterializeLimits,
) (CandidateWorkspace, error) {
	if err := limits.validate(); err != nil {
		return CandidateWorkspace{}, err
	}
	if err := repository.verifyCandidateObjects(ctx, candidate); err != nil {
		return CandidateWorkspace{}, err
	}
	if err := repository.assertCandidateRetained(ctx, candidate); err != nil {
		return CandidateWorkspace{}, err
	}
	if err := repository.rejectGitlinks(ctx, candidate.Tree); err != nil {
		return CandidateWorkspace{}, err
	}
	if err := repository.preflightTree(ctx, candidate.Tree, limits); err != nil {
		return CandidateWorkspace{}, err
	}
	destination, err := repository.materializeTree(ctx, candidate.Tree, destination)
	if err != nil {
		return CandidateWorkspace{}, err
	}
	manifest, _, err := workspace.Measure(ctx, destination, limits.Bytes)
	if err != nil {
		_ = os.RemoveAll(destination)
		return CandidateWorkspace{}, fmt.Errorf("measure fresh candidate workspace: %w", err)
	}
	return CandidateWorkspace{
		repositoryID: repository.binding.RepositoryID,
		path:         destination,
		candidate:    cloneCandidate(candidate),
		manifest:     manifest,
	}, nil
}

// VerifyCandidate rederives immutable Git, retention, changed-path, and scope
// facts immediately before another protocol boundary admits the candidate.
func (repository *Repository) VerifyCandidate(
	ctx context.Context,
	candidate Candidate,
	scope Scope,
) error {
	if err := scope.Validate(); err != nil {
		return err
	}
	if err := repository.verifyCandidateObjects(ctx, candidate); err != nil {
		return err
	}
	if err := repository.assertCandidateRetained(ctx, candidate); err != nil {
		return err
	}
	if err := repository.rejectGitlinks(ctx, candidate.Tree); err != nil {
		return err
	}
	return outOfScope(scope, candidate.ChangedPaths)
}

// VerifyCandidateWorkspace revalidates the opaque materialization handle and
// its immutable Git facts. The contained executor performs the definitive
// current-byte proof while staging and must match workspace.manifest before it
// starts a subprocess.
func (repository *Repository) VerifyCandidateWorkspace(
	ctx context.Context,
	workspace CandidateWorkspace,
) error {
	if workspace.repositoryID != repository.binding.RepositoryID ||
		workspace.candidate.RepositoryID != repository.binding.RepositoryID {
		return errors.New("candidate workspace repository identity does not match binding")
	}
	workspacePath, err := canonicalDirectory(workspace.path)
	if err != nil {
		return fmt.Errorf("resolve checked candidate workspace: %w", err)
	}
	if workspacePath != workspace.path {
		return errors.New("candidate workspace path no longer matches its canonical binding")
	}
	if err := repository.rejectWorkspaceOverlap(workspacePath); err != nil {
		return err
	}
	if err := repository.verifyCandidateObjects(ctx, workspace.candidate); err != nil {
		return err
	}
	if err := repository.assertCandidateRetained(ctx, workspace.candidate); err != nil {
		return err
	}
	if err := repository.rejectGitlinks(ctx, workspace.candidate.Tree); err != nil {
		return err
	}
	if len(workspace.manifest) != len("sha256:")+64 || !strings.HasPrefix(workspace.manifest, "sha256:") {
		return errors.New("candidate workspace lacks its materialization manifest binding")
	}
	return nil
}

func (repository *Repository) preflightTree(
	ctx context.Context,
	tree string,
	limits MaterializeLimits,
) error {
	result, err := repository.git(ctx, nil, "ls-tree", "-r", "-t", "-l", "-z", tree)
	if err != nil {
		return fmt.Errorf("preflight candidate tree: %w", err)
	}
	var entries uint64
	var bytes uint64
	for _, raw := range strings.Split(strings.TrimSuffix(string(result.stdout), "\x00"), "\x00") {
		if raw == "" {
			continue
		}
		entries++
		if entries > limits.Entries {
			return fmt.Errorf("candidate tree exceeds %d-entry materialization ceiling", limits.Entries)
		}
		metadata, _, found := strings.Cut(raw, "\t")
		fields := strings.Fields(metadata)
		if !found || len(fields) != 4 || (fields[1] != "blob" && fields[1] != "tree") {
			return errors.New("Git returned an unsupported candidate tree entry")
		}
		if fields[1] == "tree" {
			if fields[3] != "-" {
				return errors.New("Git returned an invalid candidate tree size")
			}
			continue
		}
		if fields[0] == "120000" {
			return errors.New("candidate symlinks are outside the initial local-check capability")
		}
		size, parseErr := strconv.ParseUint(fields[3], 10, 64)
		if parseErr != nil || size > limits.Bytes-bytes {
			return fmt.Errorf("candidate tree exceeds %d-byte materialization ceiling", limits.Bytes)
		}
		bytes += size
	}
	return nil
}

func (repository *Repository) materializeTree(
	ctx context.Context,
	tree, destination string,
) (materialized string, err error) {
	destination, err = repository.prepareDestination(destination)
	if err != nil {
		return "", err
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(destination)
		}
	}()
	operation, cleanup, err := repository.newGitOperation(ctx)
	if err != nil {
		return "", err
	}
	defer cleanup()
	if _, err := operation.git(ctx, "", nil, nil, "read-tree", tree); err != nil {
		return "", fmt.Errorf("read tree into isolated index: %w", err)
	}
	prefix := destination + string(filepath.Separator)
	if _, err := operation.git(ctx, destination, nil, nil, "checkout-index", "--all", "--force", "--prefix="+prefix); err != nil {
		return "", fmt.Errorf("materialize tree: %w", err)
	}
	if err := scanWorkspace(destination); err != nil {
		return "", err
	}
	return destination, nil
}

func (repository *Repository) Capture(
	ctx context.Context,
	workspace Workspace,
	options CaptureOptions,
) (Candidate, error) {
	if err := options.validate(); err != nil {
		return Candidate{}, err
	}
	if workspace.RepositoryID != repository.binding.RepositoryID ||
		workspace.Target.RepositoryID != repository.binding.RepositoryID {
		return Candidate{}, errors.New("workspace repository identity does not match binding")
	}
	workspacePath, err := canonicalDirectory(workspace.Path)
	if err != nil {
		return Candidate{}, fmt.Errorf("resolve candidate workspace: %w", err)
	}
	if workspacePath != workspace.Path {
		return Candidate{}, errors.New("workspace path no longer matches its canonical binding")
	}
	if err := repository.rejectWorkspaceOverlap(workspacePath); err != nil {
		return Candidate{}, err
	}
	if err := scanWorkspace(workspacePath); err != nil {
		return Candidate{}, err
	}
	if err := repository.AssertTarget(ctx, workspace.Target); err != nil {
		return Candidate{}, err
	}
	operation, cleanup, err := repository.newGitOperation(ctx)
	if err != nil {
		return Candidate{}, err
	}
	defer cleanup()
	if _, err := operation.git(ctx, workspacePath, nil, nil, "read-tree", workspace.Target.Tree); err != nil {
		return Candidate{}, fmt.Errorf("read base tree into isolated index: %w", err)
	}
	if _, err := operation.git(ctx, workspacePath, nil, nil, "add", "--all", "--", "."); err != nil {
		return Candidate{}, fmt.Errorf("capture workspace bytes: %w", err)
	}
	if err := repository.assertIndexMatchesWorkspace(ctx, operation, workspacePath); err != nil {
		return Candidate{}, err
	}
	result, err := operation.git(ctx, workspacePath, nil, nil, "write-tree")
	if err != nil {
		return Candidate{}, fmt.Errorf("write candidate tree: %w", err)
	}
	tree := strings.TrimSpace(string(result.stdout))
	if !repository.validOID(tree) {
		return Candidate{}, fmt.Errorf("Git returned invalid candidate tree %q", tree)
	}
	changedPaths, err := repository.changedPaths(ctx, workspace.Target.Tree, tree)
	if err != nil {
		return Candidate{}, err
	}
	if err := outOfScope(options.Scope, changedPaths); err != nil {
		return Candidate{}, err
	}
	commit := workspace.Target.Commit
	if tree != workspace.Target.Tree {
		commit, err = repository.commitTree(ctx, operation, workspace.Target.Commit, tree, options.Timestamp)
		if err != nil {
			return Candidate{}, err
		}
	}
	candidate := Candidate{
		RepositoryID: repository.binding.RepositoryID,
		TargetRef:    workspace.Target.Ref,
		BaseCommit:   workspace.Target.Commit,
		BaseTree:     workspace.Target.Tree,
		Commit:       commit,
		Tree:         tree,
		Ref:          candidateRefPrefix + commit,
		ChangedPaths: changedPaths,
	}
	if err := repository.verifyCandidateObjects(ctx, candidate); err != nil {
		return Candidate{}, err
	}
	// Recheck immediately before making the candidate durable. A later move is
	// still caught by the integration compare-and-swap boundary.
	if err := repository.AssertTarget(ctx, workspace.Target); err != nil {
		return Candidate{}, err
	}
	if err := repository.retainCandidate(ctx, candidate); err != nil {
		return Candidate{}, err
	}
	return candidate, nil
}

// EnsureCandidate reconciles a recorded candidate after interruption. It
// recreates only the deterministic retention ref and only while all bound Git
// objects and diff facts still verify exactly.
func (repository *Repository) EnsureCandidate(ctx context.Context, candidate Candidate) error {
	if err := repository.verifyCandidateObjects(ctx, candidate); err != nil {
		return err
	}
	return repository.retainCandidate(ctx, candidate)
}

func (repository *Repository) assertCandidateRetained(ctx context.Context, candidate Candidate) error {
	commit, found, err := repository.readRef(ctx, candidate.Ref)
	if err != nil {
		return err
	}
	if !found {
		return errors.New("candidate retention ref is missing")
	}
	if commit != candidate.Commit {
		return fmt.Errorf("candidate retention ref points to %s, want %s", commit, candidate.Commit)
	}
	return nil
}

func (repository *Repository) verifyCandidateObjects(ctx context.Context, candidate Candidate) error {
	if candidate.RepositoryID != repository.binding.RepositoryID {
		return errors.New("candidate repository identity does not match binding")
	}
	if err := repository.validateTargetRef(ctx, candidate.TargetRef); err != nil {
		return err
	}
	for name, oid := range map[string]string{
		"base commit": candidate.BaseCommit,
		"base tree":   candidate.BaseTree,
		"commit":      candidate.Commit,
		"tree":        candidate.Tree,
	} {
		if !repository.validOID(oid) {
			return fmt.Errorf("candidate has invalid %s object id %q", name, oid)
		}
	}
	if candidate.Ref != candidateRefPrefix+candidate.Commit {
		return errors.New("candidate retention ref is not derived from its commit")
	}
	if _, err := repository.git(ctx, nil, "check-ref-format", candidate.Ref); err != nil {
		return fmt.Errorf("invalid candidate retention ref: %w", err)
	}
	baseTree, err := repository.resolveTree(ctx, candidate.BaseCommit)
	if err != nil || baseTree != candidate.BaseTree {
		return fmt.Errorf("candidate base tree mismatch: observed %q, want %q", baseTree, candidate.BaseTree)
	}
	tree, err := repository.resolveTree(ctx, candidate.Commit)
	if err != nil || tree != candidate.Tree {
		return fmt.Errorf("candidate tree mismatch: observed %q, want %q", tree, candidate.Tree)
	}
	if candidate.Commit == candidate.BaseCommit {
		if candidate.Tree != candidate.BaseTree || len(candidate.ChangedPaths) != 0 {
			return errors.New("unchanged candidate has changed tree or paths")
		}
	} else {
		result, err := repository.git(ctx, nil, "rev-list", "--parents", "-n", "1", candidate.Commit)
		if err != nil {
			return fmt.Errorf("read candidate parents: %w", err)
		}
		parents := strings.Fields(string(result.stdout))
		if len(parents) != 2 || parents[0] != candidate.Commit || parents[1] != candidate.BaseCommit {
			return errors.New("candidate is not an exact single-parent child of its base")
		}
	}
	paths, err := repository.changedPaths(ctx, candidate.BaseTree, candidate.Tree)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(paths, candidate.ChangedPaths) {
		return fmt.Errorf("candidate changed paths mismatch: observed %q, want %q", paths, candidate.ChangedPaths)
	}
	return nil
}

func (repository *Repository) commitTree(
	ctx context.Context,
	operation *gitOperation,
	baseCommit, tree string,
	timestamp time.Time,
) (string, error) {
	date := fmt.Sprintf("@%d +0000", timestamp.UTC().Unix())
	environment := []string{
		"GIT_AUTHOR_NAME=Sworn Engine",
		"GIT_AUTHOR_EMAIL=sworn@localhost",
		"GIT_AUTHOR_DATE=" + date,
		"GIT_COMMITTER_NAME=Sworn Engine",
		"GIT_COMMITTER_EMAIL=sworn@localhost",
		"GIT_COMMITTER_DATE=" + date,
	}
	result, err := operation.git(
		ctx, "", environment, strings.NewReader("Sworn candidate\n"),
		"commit-tree", tree, "-p", baseCommit,
	)
	if err != nil {
		return "", fmt.Errorf("create candidate commit: %w", err)
	}
	commit := strings.TrimSpace(string(result.stdout))
	if !repository.validOID(commit) {
		return "", fmt.Errorf("Git returned invalid candidate commit %q", commit)
	}
	return commit, nil
}

func (repository *Repository) retainCandidate(ctx context.Context, candidate Candidate) error {
	current, found, err := repository.readRef(ctx, candidate.Ref)
	if err != nil {
		return err
	}
	if found {
		if current != candidate.Commit {
			return fmt.Errorf("candidate ref collision: %s points to %s, want %s", candidate.Ref, current, candidate.Commit)
		}
		return nil
	}
	_, updateErr := repository.git(
		ctx, nil,
		"update-ref", "--no-deref", candidate.Ref, candidate.Commit, repository.zeroOID,
	)
	if updateErr != nil {
		current, found, readErr := repository.readRef(ctx, candidate.Ref)
		if readErr == nil && found && current == candidate.Commit {
			return nil
		}
		return fmt.Errorf("retain candidate ref: %w", updateErr)
	}
	current, found, err = repository.readRef(ctx, candidate.Ref)
	if err != nil {
		return err
	}
	if !found || current != candidate.Commit {
		return errors.New("candidate ref readback did not match candidate commit")
	}
	return nil
}

func (repository *Repository) readRef(ctx context.Context, ref string) (string, bool, error) {
	result, err := repository.git(ctx, nil, "rev-parse", "--verify", "--quiet", "--end-of-options", ref+"^{commit}")
	if err != nil {
		var commandErr *gitError
		if errors.As(err, &commandErr) && commandErr.exitCode == 1 {
			return "", false, nil
		}
		return "", false, fmt.Errorf("read candidate ref: %w", err)
	}
	oid := strings.TrimSpace(string(result.stdout))
	if !repository.validOID(oid) {
		return "", false, fmt.Errorf("candidate ref returned invalid object id %q", oid)
	}
	return oid, true, nil
}

func (repository *Repository) changedPaths(ctx context.Context, baseTree, candidateTree string) ([]string, error) {
	result, err := repository.git(
		ctx, nil,
		"diff-tree", "--no-commit-id", "--name-only", "--no-ext-diff", "--no-textconv", "--no-renames", "-r", "-z", baseTree, candidateTree,
	)
	if err != nil {
		return nil, fmt.Errorf("derive candidate changed paths: %w", err)
	}
	values := strings.Split(strings.TrimSuffix(string(result.stdout), "\x00"), "\x00")
	paths := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, gitPath := range values {
		if gitPath == "" {
			continue
		}
		if !validGitPath(gitPath) {
			return nil, fmt.Errorf("Git returned path that Baton cannot represent: %q", gitPath)
		}
		if _, exists := seen[gitPath]; exists {
			continue
		}
		seen[gitPath] = struct{}{}
		paths = append(paths, gitPath)
	}
	sort.Strings(paths)
	return paths, nil
}

func (repository *Repository) assertIndexMatchesWorkspace(
	ctx context.Context,
	operation *gitOperation,
	workspace string,
) error {
	if _, err := operation.git(ctx, workspace, nil, nil, "diff-files", "--quiet", "--"); err != nil {
		return fmt.Errorf("workspace changed while candidate was captured: %w", err)
	}
	result, err := operation.git(ctx, workspace, nil, nil, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return fmt.Errorf("inspect untracked candidate paths: %w", err)
	}
	if len(result.stdout) != 0 {
		return errors.New("workspace retained non-ignored untracked paths after capture")
	}
	return nil
}

func (repository *Repository) rejectGitlinks(ctx context.Context, tree string) error {
	result, err := repository.git(ctx, nil, "ls-tree", "-r", "-z", tree)
	if err != nil {
		return fmt.Errorf("inspect target tree entries: %w", err)
	}
	for _, entry := range strings.Split(strings.TrimSuffix(string(result.stdout), "\x00"), "\x00") {
		if strings.HasPrefix(entry, "160000 commit ") {
			return errors.New("Gitlinks are outside the initial exact-candidate capability")
		}
	}
	return nil
}

func (repository *Repository) prepareDestination(destination string) (string, error) {
	if strings.TrimSpace(destination) == "" {
		return "", errors.New("workspace destination is required")
	}
	absolute, err := filepath.Abs(destination)
	if err != nil {
		return "", fmt.Errorf("resolve workspace destination: %w", err)
	}
	absolute = filepath.Clean(absolute)
	parent, err := canonicalDirectory(filepath.Dir(absolute))
	if err != nil {
		return "", fmt.Errorf("resolve workspace parent: %w", err)
	}
	absolute = filepath.Join(parent, filepath.Base(absolute))
	if err := repository.rejectWorkspaceOverlap(absolute); err != nil {
		return "", err
	}
	if _, err := os.Lstat(absolute); err == nil {
		return "", fmt.Errorf("workspace destination %q already exists", absolute)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("inspect workspace destination: %w", err)
	}
	if err := os.Mkdir(absolute, 0o700); err != nil {
		return "", fmt.Errorf("create workspace destination: %w", err)
	}
	return absolute, nil
}

func (repository *Repository) rejectWorkspaceOverlap(workspace string) error {
	for name, protected := range map[string]string{
		"source worktree":      repository.root,
		"Git common directory": repository.commonDir,
	} {
		if pathWithin(workspace, protected) || pathWithin(protected, workspace) {
			return fmt.Errorf("candidate workspace overlaps %s", name)
		}
	}
	return nil
}

func scanWorkspace(root string) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		gitPath := filepath.ToSlash(relative)
		if !utf8.ValidString(gitPath) {
			return fmt.Errorf("workspace path is not valid UTF-8: %q", []byte(gitPath))
		}
		for _, segment := range strings.Split(gitPath, "/") {
			if segment == ".git" {
				return fmt.Errorf("workspace contains forbidden Git metadata path %q", gitPath)
			}
		}
		kind := entry.Type()
		if entry.IsDir() || kind.IsRegular() || kind&os.ModeSymlink != 0 {
			return nil
		}
		return fmt.Errorf("workspace contains unsupported special file %q", gitPath)
	})
}
