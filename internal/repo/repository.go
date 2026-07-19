package repo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Repository struct {
	binding   Binding
	root      string
	commonDir string
	objectDir string
	gitPath   string
	zeroOID   string
}

// Discover measures the immutable local Git mapping that configuration must
// persist before a delivery is admitted. It does not derive repository identity
// from a path or remote URL.
func Discover(ctx context.Context, workspace, repositoryID string) (Binding, error) {
	if !idPattern.MatchString(repositoryID) {
		return Binding{}, errors.New("valid repository id is required")
	}
	discovery, err := discover(ctx, workspace)
	if err != nil {
		return Binding{}, err
	}
	binding := Binding{
		SchemaVersion: BindingSchemaVersion,
		RepositoryID:  repositoryID,
		CommonDir:     discovery.commonDir,
		ObjectDir:     discovery.objectDir,
		ObjectFormat:  discovery.objectFormat,
	}
	if err := binding.Validate(); err != nil {
		return Binding{}, err
	}
	return binding, nil
}

// Open fails if the current workspace no longer maps to the exact persisted
// common directory and object format. Any remap requires a new repository ID.
func Open(ctx context.Context, workspace string, binding Binding) (*Repository, error) {
	if err := binding.Validate(); err != nil {
		return nil, err
	}
	discovery, err := discover(ctx, workspace)
	if err != nil {
		return nil, err
	}
	if discovery.commonDir != binding.CommonDir {
		return nil, fmt.Errorf(
			"repository binding drift: observed common directory %q, want %q",
			discovery.commonDir,
			binding.CommonDir,
		)
	}
	if discovery.objectFormat != binding.ObjectFormat {
		return nil, fmt.Errorf(
			"repository binding drift: observed object format %q, want %q",
			discovery.objectFormat,
			binding.ObjectFormat,
		)
	}
	if discovery.objectDir != binding.ObjectDir {
		return nil, fmt.Errorf(
			"repository binding drift: observed object directory %q, want %q",
			discovery.objectDir,
			binding.ObjectDir,
		)
	}
	if err := rejectUnsupportedObjectStores(discovery.commonDir, discovery.objectDir); err != nil {
		return nil, err
	}
	zeroLength := 40
	if binding.ObjectFormat == "sha256" {
		zeroLength = 64
	}
	return &Repository{
		binding:   binding,
		root:      discovery.root,
		commonDir: discovery.commonDir,
		objectDir: discovery.objectDir,
		gitPath:   discovery.gitPath,
		zeroOID:   strings.Repeat("0", zeroLength),
	}, nil
}

func (repository *Repository) Binding() Binding { return repository.binding }

func (repository *Repository) Root() string { return repository.root }

type discovery struct {
	root         string
	commonDir    string
	objectDir    string
	objectFormat string
	gitPath      string
}

func discover(ctx context.Context, workspace string) (discovery, error) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return discovery{}, errors.New("Git executable is required")
	}
	gitPath, err = filepath.Abs(gitPath)
	if err != nil {
		return discovery{}, fmt.Errorf("resolve Git executable: %w", err)
	}
	workspace, err = canonicalDirectory(workspace)
	if err != nil {
		return discovery{}, fmt.Errorf("resolve repository workspace: %w", err)
	}
	args := []string{
		"-C", workspace,
		"-c", "core.hooksPath=/dev/null",
		"-c", "core.fsmonitor=false",
		"rev-parse", "--path-format=absolute", "--show-toplevel", "--git-common-dir", "--show-object-format",
	}
	result, err := executeGit(ctx, gitPath, baseGitEnvironment(""), nil, args...)
	if err != nil {
		return discovery{}, fmt.Errorf("discover Git repository: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(string(result.stdout)), "\n")
	if len(lines) != 3 {
		return discovery{}, fmt.Errorf("Git repository discovery returned %d fields, want 3", len(lines))
	}
	root, err := canonicalDirectory(lines[0])
	if err != nil {
		return discovery{}, fmt.Errorf("resolve Git worktree root: %w", err)
	}
	commonDir, err := canonicalDirectory(lines[1])
	if err != nil {
		return discovery{}, fmt.Errorf("resolve Git common directory: %w", err)
	}
	objectFormat := strings.TrimSpace(lines[2])
	if objectFormat != "sha1" && objectFormat != "sha256" {
		return discovery{}, fmt.Errorf("unsupported Git object format %q", objectFormat)
	}
	objectResult, err := executeGit(ctx, gitPath, baseGitEnvironment(""), nil,
		"-C", workspace, "rev-parse", "--path-format=absolute", "--git-path", "objects",
	)
	if err != nil {
		return discovery{}, fmt.Errorf("discover Git object directory: %w", err)
	}
	objectDir, err := canonicalDirectory(strings.TrimSpace(string(objectResult.stdout)))
	if err != nil {
		return discovery{}, fmt.Errorf("resolve Git object directory: %w", err)
	}
	return discovery{
		root:         root,
		commonDir:    commonDir,
		objectDir:    objectDir,
		objectFormat: objectFormat,
		gitPath:      gitPath,
	}, nil
}

func rejectUnsupportedObjectStores(commonDir, objectDir string) error {
	for _, path := range []string{
		filepath.Join(objectDir, "info", "alternates"),
		filepath.Join(commonDir, "info", "grafts"),
	} {
		if _, err := os.Lstat(path); err == nil {
			return fmt.Errorf("unsupported Git object indirection at %q", path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect Git object indirection %q: %w", path, err)
		}
	}
	return nil
}

func (repository *Repository) BindTarget(ctx context.Context, targetRef string) (Target, error) {
	if err := repository.validateTargetRef(ctx, targetRef); err != nil {
		return Target{}, err
	}
	commit, err := repository.resolveCommit(ctx, targetRef)
	if err != nil {
		return Target{}, fmt.Errorf("resolve target %q: %w", targetRef, err)
	}
	tree, err := repository.resolveTree(ctx, commit)
	if err != nil {
		return Target{}, fmt.Errorf("resolve target tree: %w", err)
	}
	return Target{
		RepositoryID: repository.binding.RepositoryID,
		Ref:          targetRef,
		Commit:       commit,
		Tree:         tree,
	}, nil
}

func (repository *Repository) AssertTarget(ctx context.Context, target Target) error {
	if err := repository.validateTarget(target); err != nil {
		return err
	}
	observed, err := repository.BindTarget(ctx, target.Ref)
	if err != nil {
		return err
	}
	if observed.Commit != target.Commit || observed.Tree != target.Tree {
		return fmt.Errorf(
			"target moved: %s now resolves to %s (%s), want %s (%s)",
			target.Ref, observed.Commit, observed.Tree, target.Commit, target.Tree,
		)
	}
	return nil
}

func (repository *Repository) validateTarget(target Target) error {
	if target.RepositoryID != repository.binding.RepositoryID {
		return errors.New("target repository identity does not match binding")
	}
	if target.Commit == "" || target.Tree == "" {
		return errors.New("target requires commit and tree identities")
	}
	return nil
}

func (repository *Repository) validateTargetRef(ctx context.Context, targetRef string) error {
	if !strings.HasPrefix(targetRef, "refs/heads/") || len(targetRef) <= len("refs/heads/") {
		return errors.New("target must be a full refs/heads/... ref")
	}
	_, err := repository.git(ctx, nil, "check-ref-format", targetRef)
	if err != nil {
		return fmt.Errorf("invalid target ref %q: %w", targetRef, err)
	}
	return nil
}

func (repository *Repository) resolveCommit(ctx context.Context, revision string) (string, error) {
	result, err := repository.git(ctx, nil, "rev-parse", "--verify", "--end-of-options", revision+"^{commit}")
	if err != nil {
		return "", err
	}
	commit := strings.TrimSpace(string(result.stdout))
	if !repository.validOID(commit) {
		return "", fmt.Errorf("Git returned invalid commit object id %q", commit)
	}
	return commit, nil
}

func (repository *Repository) resolveTree(ctx context.Context, revision string) (string, error) {
	result, err := repository.git(ctx, nil, "rev-parse", "--verify", "--end-of-options", revision+"^{tree}")
	if err != nil {
		return "", err
	}
	tree := strings.TrimSpace(string(result.stdout))
	if !repository.validOID(tree) {
		return "", fmt.Errorf("Git returned invalid tree object id %q", tree)
	}
	return tree, nil
}

func (repository *Repository) validOID(value string) bool {
	if len(value) != len(repository.zeroOID) {
		return false
	}
	for _, char := range value {
		if !(char >= '0' && char <= '9') && !(char >= 'a' && char <= 'f') {
			return false
		}
	}
	return true
}
