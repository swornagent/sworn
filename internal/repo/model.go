// Package repo owns measured Git repository, workspace, and candidate facts.
package repo

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const BindingSchemaVersion = "sworn-repository-binding-v1"

var idPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// Binding is the immutable local mapping that a future runtime configuration
// persists. Open verifies it against live Git facts on every use.
type Binding struct {
	SchemaVersion string `json:"schema_version"`
	RepositoryID  string `json:"repository_id"`
	CommonDir     string `json:"common_dir"`
	ObjectDir     string `json:"object_dir"`
	ObjectFormat  string `json:"object_format"`
}

func (binding Binding) Validate() error {
	if binding.SchemaVersion != BindingSchemaVersion {
		return fmt.Errorf("unknown repository binding schema %q", binding.SchemaVersion)
	}
	if !idPattern.MatchString(binding.RepositoryID) {
		return errors.New("invalid repository id")
	}
	if !filepath.IsAbs(binding.CommonDir) || filepath.Clean(binding.CommonDir) != binding.CommonDir {
		return errors.New("repository common directory must be a clean absolute path")
	}
	if !filepath.IsAbs(binding.ObjectDir) || filepath.Clean(binding.ObjectDir) != binding.ObjectDir {
		return errors.New("repository object directory must be a clean absolute path")
	}
	if binding.ObjectFormat != "sha1" && binding.ObjectFormat != "sha256" {
		return fmt.Errorf("unsupported Git object format %q", binding.ObjectFormat)
	}
	return nil
}

type Target struct {
	RepositoryID string `json:"repository_id"`
	Ref          string `json:"target_ref"`
	Commit       string `json:"commit"`
	Tree         string `json:"tree"`
}

type Workspace struct {
	RepositoryID string `json:"repository_id"`
	Path         string `json:"path"`
	Target       Target `json:"target"`
}

type Candidate struct {
	RepositoryID string   `json:"repository_id"`
	TargetRef    string   `json:"target_ref"`
	BaseCommit   string   `json:"base_commit"`
	BaseTree     string   `json:"base_tree"`
	Commit       string   `json:"commit"`
	Tree         string   `json:"tree"`
	Ref          string   `json:"retention_ref"`
	ChangedPaths []string `json:"changed_paths"`
}

// AttemptUnpublishedProof is an opaque, live proof that one exact builder
// attempt has no engine publication ref. It is useful only while the caller
// retains exclusive controller ownership.
type AttemptUnpublishedProof struct {
	repositoryID string
	attemptID    string
}

func (proof AttemptUnpublishedProof) RepositoryID() string { return proof.repositoryID }
func (proof AttemptUnpublishedProof) AttemptID() string    { return proof.attemptID }

// CandidateWorkspace is a fresh plain-tree materialization for read-only
// checks or review. Unlike Workspace, it is not a builder input and cannot be
// passed back to Capture as a new candidate.
type CandidateWorkspace struct {
	repositoryID string
	path         string
	candidate    Candidate
	manifest     string
}

func (workspace CandidateWorkspace) RepositoryID() string { return workspace.repositoryID }
func (workspace CandidateWorkspace) Path() string         { return workspace.path }
func (workspace CandidateWorkspace) Candidate() Candidate { return cloneCandidate(workspace.candidate) }
func (workspace CandidateWorkspace) Manifest() string     { return workspace.manifest }

func cloneCandidate(candidate Candidate) Candidate {
	candidate.ChangedPaths = append([]string(nil), candidate.ChangedPaths...)
	return candidate
}

type MaterializeLimits struct {
	Bytes   uint64
	Entries uint64
}

func (limits MaterializeLimits) validate() error {
	if limits.Bytes == 0 || limits.Entries == 0 {
		return errors.New("candidate materialization requires byte and entry ceilings")
	}
	return nil
}

// CaptureOptions contains only engine-owned commit metadata and approved path
// scope. Candidate identities and changed paths always come from Git.
type CaptureOptions struct {
	Scope     Scope
	Timestamp time.Time
}

func (options CaptureOptions) validate() error {
	if err := options.Scope.Validate(); err != nil {
		return err
	}
	if options.Timestamp.IsZero() {
		return errors.New("candidate timestamp is required")
	}
	return nil
}

type Scope struct {
	Include []string `json:"include"`
	Exclude []string `json:"exclude"`
}

func (scope Scope) Validate() error {
	if len(scope.Include) == 0 {
		return errors.New("scope requires at least one included prefix")
	}
	for _, group := range [][]string{scope.Include, scope.Exclude} {
		seen := make(map[string]struct{}, len(group))
		for _, prefix := range group {
			if err := validatePathPrefix(prefix); err != nil {
				return fmt.Errorf("invalid scope prefix %q: %w", prefix, err)
			}
			if _, exists := seen[prefix]; exists {
				return fmt.Errorf("duplicate scope prefix %q", prefix)
			}
			seen[prefix] = struct{}{}
		}
	}
	return nil
}

func (scope Scope) Allows(gitPath string) bool {
	if !validGitPath(gitPath) {
		return false
	}
	for _, prefix := range scope.Exclude {
		if prefixMatches(prefix, gitPath) {
			return false
		}
	}
	for _, prefix := range scope.Include {
		if prefixMatches(prefix, gitPath) {
			return true
		}
	}
	return false
}

type ScopeError struct {
	Paths []string
}

func (err *ScopeError) Error() string {
	return "candidate changes paths outside approved scope: " + strings.Join(err.Paths, ", ")
}

func outOfScope(scope Scope, paths []string) error {
	var denied []string
	for _, gitPath := range paths {
		if !scope.Allows(gitPath) {
			denied = append(denied, gitPath)
		}
	}
	if len(denied) == 0 {
		return nil
	}
	sort.Strings(denied)
	return &ScopeError{Paths: denied}
}

func validatePathPrefix(prefix string) error {
	if prefix == "." {
		return nil
	}
	if !validGitPath(prefix) {
		return errors.New("prefix is not a normalized Git path")
	}
	if strings.ContainsAny(prefix, "\r\n\u2028\u2029") || strings.TrimSpace(prefix) == "" {
		return errors.New("prefix contains only whitespace or a line terminator")
	}
	if strings.ContainsAny(prefix, `*?[]\`) {
		return errors.New("glob metacharacters and backslashes are forbidden")
	}
	return nil
}

func validGitPath(gitPath string) bool {
	if gitPath == "" || gitPath == "." || !utf8.ValidString(gitPath) ||
		strings.HasPrefix(gitPath, "/") || strings.HasSuffix(gitPath, "/") ||
		strings.Contains(gitPath, "\x00") || strings.Contains(gitPath, "//") {
		return false
	}
	for _, segment := range strings.Split(gitPath, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

func prefixMatches(prefix, gitPath string) bool {
	return prefix == "." || gitPath == prefix || strings.HasPrefix(gitPath, prefix+"/")
}
