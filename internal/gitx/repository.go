package gitx

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"
)

const (
	recordRoot             = DefaultRecordsRoot
	MaxHeadRefs            = 128
	MaxBatchPaths          = 1025
	MaxFileBytes           = 262_144
	MaxBatchBytes          = MaxBatchPaths * MaxFileBytes
	MaxTreeEntries         = 100_000
	MaxTreeBytes           = 64 * 1024 * 1024
	MaxChangedPaths        = 100_000
	MaxHistory             = 4_096
	MaxMessageBytes        = 2_097_152
	MaxRepositoryPath      = 1000
	MaxCommandOutput       = 64 * 1024 * 1024
	MaxDiagnostic          = 512 * 1024
	maxRecordRootDiffBytes = 8 * 1024 * 1024
	CommandTimeout         = 30 * time.Second
)

type Error struct {
	Code, Op string
	Err      error
}

func (e *Error) Error() string {
	if e.Err == nil {
		return e.Code + ": " + e.Op
	}
	return e.Code + ": " + e.Op + ": " + e.Err.Error()
}
func (e *Error) Unwrap() error              { return e.Err }
func fail(code, op string, err error) error { return &Error{Code: code, Op: op, Err: err} }

type ObjectFormat string

const (
	SHA1   ObjectFormat = "sha1"
	SHA256 ObjectFormat = "sha256"
)

func (f ObjectFormat) oidLength() int {
	if f == SHA1 {
		return 40
	}
	if f == SHA256 {
		return 64
	}
	return 0
}

type OID struct {
	format ObjectFormat
	hex    string
}

func (o OID) String() string       { return o.hex }
func (o OID) Format() ObjectFormat { return o.format }
func (o OID) IsZero() bool         { return o.hex == "" }
func ParseOID(format ObjectFormat, value string) (OID, error) {
	if len(value) != format.oidLength() || value == "" {
		return OID{}, fail("INVALID_OBJECT_ID", "parse OID", fmt.Errorf("expected %d lowercase hex characters", format.oidLength()))
	}
	for _, character := range value {
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f')) {
			return OID{}, fail("INVALID_OBJECT_ID", "parse OID", errors.New("OID is not lowercase hexadecimal"))
		}
	}
	return OID{format: format, hex: value}, nil
}

type TreeEntry struct {
	Path, Mode, Type string
	OID              OID
}

type RecordRootRequest struct {
	Kind       string
	Repository string
	RecordRoot string
	Commit     string
}

type RecordRootDecision struct {
	Kind       string
	Repository string
	RecordRoot string
	Commit     string
	Decision   string
}

type InertnessResolver func(RecordRootRequest) (RecordRootDecision, error)

type RecordPathAdmission struct {
	repository string
	root       string
}

// Root returns the admitted records root for this admission.
func (a *RecordPathAdmission) Root() string {
	if a == nil {
		return ""
	}
	return a.root
}

type ProductExclusionAdmission struct {
	repository string
	root       string
	record     *RecordPathAdmission
	resolver   InertnessResolver
	mu         sync.Mutex
	decisions  map[string]RecordRootDecision
}

type ProductIdentity struct {
	Candidate     OID
	CandidateTree OID
	ProductTree   string
	Entries       []TreeEntry
}

type Repository struct {
	root       string
	git        string
	commonDir  string
	format     ObjectFormat
	refFault   *refFault
	identityMu sync.Mutex
	identities map[string]Identity
	// recordRoot is the configured records root (default DefaultRecordsRoot),
	// resolved from the committed project config at Open. Every reserved-root
	// admission, exclusion and mask site reads this field so a configured
	// records root is honored everywhere. The historical LegacyRecordsRoot
	// stays reserved alongside it for the legacy read fallback.
	recordRoot string
	// config is the resolved project configuration (defaults when the
	// project file is absent); configured reports whether a project file was
	// present so commit-prefix consumers can preserve their unconfigured
	// subjects exactly.
	config     ProjectConfig
	configured bool
}

// Open admits one canonical repository and literal absolute Git executable.
func Open(repository, gitExecutable string) (*Repository, error) {
	git, err := admitGitExecutable(gitExecutable)
	if err != nil {
		return nil, err
	}
	if repository == "" || !filepath.IsAbs(repository) {
		return nil, fail("INVALID_REPOSITORY", "open repository", errors.New("repository path must be absolute"))
	}
	rootCandidate, err := filepath.EvalSymlinks(repository)
	if err != nil {
		return nil, fail("INVALID_REPOSITORY", "resolve repository", err)
	}
	info, err := os.Stat(rootCandidate)
	if err != nil || !info.IsDir() {
		return nil, fail("INVALID_REPOSITORY", "inspect repository", err)
	}
	repo := &Repository{
		root: filepath.Clean(rootCandidate), git: git,
		identities: make(map[string]Identity),
	}
	rootBytes, err := repo.run(nil, nil, "rev-parse", "--path-format=absolute", "--show-toplevel")
	if err != nil {
		rootBytes, err = repo.run(nil, nil, "rev-parse", "--path-format=absolute", "--git-dir")
		if err != nil {
			return nil, fail("INVALID_REPOSITORY", "resolve Git root", err)
		}
	}
	root := strings.TrimSuffix(string(rootBytes), "\n")
	if !filepath.IsAbs(root) || strings.ContainsRune(root, 0) {
		return nil, fail("INVALID_REPOSITORY", "resolve Git root", errors.New("Git returned a non-absolute root"))
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fail("INVALID_REPOSITORY", "canonicalize Git root", err)
	}
	repo.root = filepath.Clean(root)
	formatBytes, err := repo.run(nil, nil, "rev-parse", "--show-object-format")
	if err != nil {
		return nil, fail("UNSUPPORTED_OBJECT_FORMAT", "read object format", err)
	}
	repo.format = ObjectFormat(strings.TrimSpace(string(formatBytes)))
	if repo.format != SHA1 && repo.format != SHA256 {
		return nil, fail("UNSUPPORTED_OBJECT_FORMAT", "read object format", fmt.Errorf("%q", repo.format))
	}
	commonBytes, err := repo.run(nil, nil, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return nil, fail("INVALID_REPOSITORY", "resolve Git common directory", err)
	}
	common := strings.TrimSuffix(string(commonBytes), "\n")
	common, err = filepath.EvalSymlinks(common)
	if err != nil {
		return nil, fail("INVALID_REPOSITORY", "canonicalize Git common directory", err)
	}
	info, err = os.Stat(common)
	if err != nil || !info.IsDir() {
		return nil, fail("INVALID_REPOSITORY", "inspect Git common directory", err)
	}
	repo.commonDir = filepath.Clean(common)
	config, configured, err := LoadProjectConfig(repo.root)
	if err != nil {
		return nil, fail("INVALID_PROJECT_CONFIG", "load project configuration", err)
	}
	repo.config = config
	repo.configured = configured
	repo.recordRoot = config.RecordsRoot
	return repo, nil
}

// ProjectConfig returns the resolved project configuration for this
// repository (documented defaults when the committed project file is absent).
func (r *Repository) ProjectConfig() ProjectConfig {
	if r == nil {
		return DefaultProjectConfig()
	}
	return r.config
}

// DocumentsRoot returns the resolved documents root for this repository
// (documented default when the committed project file is absent).
func (r *Repository) DocumentsRoot() string {
	if r == nil || !r.configured {
		return DefaultDocumentsRoot
	}
	return r.config.DocumentsRoot
}

// LegacyRecordRoot returns the historical records root that remains readable
// and reserved for releases recorded before the relocation.
func (r *Repository) LegacyRecordRoot() string {
	return LegacyRecordsRoot
}

// CommitPrefix returns the configured commit-message prefix for plan and
// receipt actions. An unconfigured repository keeps the documented default
// "sworn" prefix.
func (r *Repository) CommitPrefix() string {
	if r == nil || !r.configured {
		return DefaultCommitPrefix
	}
	return r.config.CommitPrefix
}

// CandidateCommitPrefix returns the prefix for engine implementation-candidate
// commits. An unconfigured repository uses the same documented default as the
// plan/receipt actions, so an unconfigured project writes one consistent
// prefix; a configured repository follows the configured prefix so a
// configured prefix never yields a mixed git log.
func (r *Repository) CandidateCommitPrefix() string {
	if r == nil || !r.configured {
		return candidateCommitPrefixDefault
	}
	return r.config.CommitPrefix
}

// ReservedNames returns the workspace-relative names the containment mask
// must protect for this repository, derived from its resolved project
// configuration.
func (r *Repository) ReservedNames() []string {
	return ReservedNames(r.ProjectConfig())
}
func admitGitExecutable(value string) (string, error) {
	if value == "" || !filepath.IsAbs(value) {
		return "", fail("INVALID_GIT_EXECUTABLE", "admit Git", errors.New("Git executable must be absolute"))
	}
	resolved, err := filepath.EvalSymlinks(value)
	if err != nil {
		return "", fail("INVALID_GIT_EXECUTABLE", "resolve Git", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fail("INVALID_GIT_EXECUTABLE", "inspect Git", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return "", fail("INVALID_GIT_EXECUTABLE", "inspect Git", errors.New("Git is not an executable regular file"))
	}
	return filepath.Clean(resolved), nil
}
func (r *Repository) Root() string               { return r.root }
func (r *Repository) GitExecutable() string      { return r.git }
func (r *Repository) ObjectFormat() ObjectFormat { return r.format }
func (r *Repository) parseOID(value string) (OID, error) {
	return ParseOID(r.format, strings.TrimSpace(value))
}
func (r *Repository) validateOID(value OID) error {
	if value.format != r.format || value.hex == "" {
		return fail("OBJECT_FORMAT_MISMATCH", "validate OID", errors.New("OID does not belong to this repository format"))
	}
	_, err := ParseOID(r.format, value.hex)
	return err
}
func literalEnvironment(home string, extra []string, attributesFile string) []string {
	environment := []string{
		"HOME=" + home, "XDG_CONFIG_HOME=" + filepath.Join(home, "xdg"), "LANG=C", "LC_ALL=C",
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_SYSTEM=/dev/null", "GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_ATTR_NOSYSTEM=1", "GIT_NO_REPLACE_OBJECTS=1", "GIT_LITERAL_PATHSPECS=1",
		"GIT_PROTOCOL_FROM_USER=0", "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never",
		"GIT_PAGER=cat", "PAGER=cat", "GIT_OPTIONAL_LOCKS=0", "GIT_CONFIG_COUNT=4",
		"GIT_CONFIG_KEY_0=core.hooksPath", "GIT_CONFIG_VALUE_0=" + filepath.Join(home, "hooks"),
		"GIT_CONFIG_KEY_1=core.fsmonitor", "GIT_CONFIG_VALUE_1=false", "GIT_CONFIG_KEY_2=core.quotePath",
		"GIT_CONFIG_VALUE_2=false", "GIT_CONFIG_KEY_3=core.attributesFile",
		"GIT_CONFIG_VALUE_3=" + attributesFile,
	}
	return append(environment, extra...)
}

type boundedBuffer struct {
	bytes.Buffer
	limit    int
	overflow bool
}

func (b *boundedBuffer) Write(value []byte) (int, error) {
	if b.overflow {
		return len(value), nil
	}
	remaining := b.limit - b.Len()
	if remaining < len(value) {
		if remaining > 0 {
			_, _ = b.Buffer.Write(value[:remaining])
		}
		b.overflow = true
		return len(value), nil
	}
	return b.Buffer.Write(value)
}

type commandOutcome struct {
	stdout, stderr                                  []byte
	exitCode                                        int
	signal                                          string
	timedOut, overflow, started, reaped, groupQuiet bool
	waitErr, cleanupErr                             error
}

func (o commandOutcome) successful() bool {
	return o.started && o.reaped && o.groupQuiet && !o.timedOut && !o.overflow &&
		o.waitErr == nil && o.exitCode == 0 && o.signal == ""
}
func (r *Repository) command(home string, group int, extraEnv []string, attributesFile string, args ...string) *exec.Cmd {
	hooks := filepath.Join(home, "hooks")
	commandArgs := append([]string{"-C", r.root, "-c", "core.hooksPath=" + hooks, "-c", "core.fsmonitor=false", "-c", "core.quotePath=false"}, args...)
	command := exec.Command(r.git, commandArgs...)
	command.Env = literalEnvironment(home, extraEnv, attributesFile)
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pgid: group}
	return command
}
func processGroupQuiet(ctx context.Context, group int) bool {
	if group <= 0 {
		return false
	}
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for !errors.Is(syscall.Kill(-group, 0), syscall.ESRCH) {
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
		}
	}
	return true
}
func (r *Repository) runOutcome(
	ctx context.Context,
	home string,
	group int,
	stdin []byte,
	extraEnv []string,
	attributesFile string,
	outputLimit int,
	args ...string,
) (result commandOutcome) {
	result.exitCode = -1
	ownedHome := home == ""
	if ownedHome {
		tempRoot, tempErr := ResolveTempRoot()
		if tempErr != nil {
			result.waitErr = tempErr
			return
		}
		home, result.waitErr = os.MkdirTemp(tempRoot, "sworn-git-home-*")
		if result.waitErr != nil {
			return
		}
		defer func() { result.cleanupErr = os.RemoveAll(home) }()
	}
	if err := os.MkdirAll(filepath.Join(home, "hooks"), 0o700); err != nil {
		result.waitErr = err
		return
	}
	command := r.command(home, group, extraEnv, attributesFile, args...)
	if stdin != nil {
		command.Stdin = bytes.NewReader(stdin)
	}
	stdout := &boundedBuffer{limit: outputLimit}
	stderr := &boundedBuffer{limit: MaxDiagnostic}
	command.Stdout, command.Stderr = stdout, stderr
	if err := command.Start(); err != nil {
		result.waitErr = err
		return
	}
	result.started = true
	ownerGroup := group == 0
	if ownerGroup {
		group = command.Process.Pid
	}
	waited := make(chan error, 1)
	go func() { waited <- command.Wait() }()
	select {
	case result.waitErr = <-waited:
		result.reaped = true
	case <-ctx.Done():
		result.timedOut = true
		_ = syscall.Kill(-group, syscall.SIGKILL)
		result.waitErr = <-waited
		result.reaped = true
	}
	if state := command.ProcessState; state != nil {
		result.exitCode = state.ExitCode()
		if status, ok := state.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			result.signal = status.Signal().String()
		}
	}
	if ownerGroup && !processGroupQuiet(ctx, group) {
		_ = syscall.Kill(-group, syscall.SIGKILL)
		result.groupQuiet = processGroupQuiet(ctx, group)
	} else {
		result.groupQuiet = true
	}
	result.stdout, result.stderr = append([]byte(nil), stdout.Bytes()...), append([]byte(nil), stderr.Bytes()...)
	result.overflow = stdout.overflow || stderr.overflow
	return
}
func (r *Repository) runStatus(ctx context.Context, home string, group int, outputLimit int, args ...string) commandOutcome {
	return r.runOutcome(ctx, home, group, nil, nil, "/dev/null", outputLimit, args...)
}
func (r *Repository) run(stdin []byte, extraEnv []string, args ...string) ([]byte, error) {
	return r.runWithAttributes(stdin, extraEnv, "/dev/null", args...)
}
func (r *Repository) runWithAttributes(stdin []byte, extraEnv []string, attributesFile string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), CommandTimeout)
	defer cancel()
	result := r.runOutcome(ctx, "", 0, stdin, extraEnv, attributesFile, MaxCommandOutput, args...)
	if result.successful() && result.cleanupErr == nil {
		return result.stdout, nil
	}
	message := strings.TrimSpace(string(result.stderr))
	if message == "" && result.waitErr != nil {
		message = result.waitErr.Error()
	}
	if result.timedOut {
		message = "command deadline exceeded"
	} else if result.overflow {
		message = "command output exceeded its byte bound"
	} else if !result.groupQuiet || !result.reaped {
		message = "command process group was not confirmed quiescent"
	} else if result.cleanupErr != nil {
		message = "private command directory cleanup failed: " + result.cleanupErr.Error()
	}
	return nil, fail("GIT_EXECUTION_FAILED", strings.Join(args, " "), errors.New(message))
}
func ValidateHeadRef(value string) error {
	const prefix = "refs/heads/"
	if len(value) <= len(prefix) || len(value) > 250 || !strings.HasPrefix(value, prefix) {
		return fail("INVALID_REF", "validate head ref", errors.New("ref must be a full refs/heads ref"))
	}
	tail := strings.TrimPrefix(value, prefix)
	if strings.Contains(tail, "..") || strings.Contains(tail, "@{") {
		return fail("INVALID_REF", "validate head ref", errors.New("ref contains a forbidden sequence"))
	}
	for _, segment := range strings.Split(tail, "/") {
		if segment == "" || segment == "." || segment == ".." || strings.HasPrefix(segment, ".") ||
			strings.HasSuffix(segment, ".") || strings.HasSuffix(segment, ".lock") {
			return fail("INVALID_REF", "validate head ref", errors.New("ref has a noncanonical segment"))
		}
	}
	if strings.ContainsAny(tail, `\ ~^:?*[]`) {
		return fail("INVALID_REF", "validate head ref", errors.New("ref contains a forbidden character"))
	}
	for _, character := range tail {
		if character < 0x20 || character == 0x7f {
			return fail("INVALID_REF", "validate head ref", errors.New("ref contains a control character"))
		}
	}
	return nil
}
func ValidatePath(value string, allowRoot bool) error {
	if value == "." && allowRoot {
		return nil
	}
	if value == "" || len(value) > MaxRepositoryPath || filepath.IsAbs(value) ||
		strings.Contains(value, `\`) || !utf8.ValidString(value) {
		return fail("INVALID_PATH", "validate repository path", errors.New("path must be canonical repository-relative UTF-8"))
	}
	segments := strings.Split(value, "/")
	if segments[0] == ".git" {
		return fail("INVALID_PATH", "validate repository path", errors.New(".git is forbidden"))
	}
	for _, segment := range segments {
		if segment == "" || segment == "." || segment == ".." {
			return fail("INVALID_PATH", "validate repository path", errors.New("path has a noncanonical segment"))
		}
		for _, character := range segment {
			if character < 0x20 || character == 0x7f {
				return fail("INVALID_PATH", "validate repository path", errors.New("path contains a control character"))
			}
		}
	}
	return nil
}

func (r *Repository) ResolveRecordPathAdmission() (*RecordPathAdmission, error) {
	cursor := r.root
	for _, part := range strings.Split(r.recordRoot, "/") {
		cursor = filepath.Join(cursor, part)
		info, err := os.Lstat(cursor)
		if errors.Is(err, os.ErrNotExist) {
			break
		}
		if err != nil {
			return nil, fail("INVALID_RECORD_ROOT", "inspect record root", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fail("SYMLINKED_RECORD_ROOT", "inspect record root", fmt.Errorf("%s is a symlink", cursor))
		}
	}
	return &RecordPathAdmission{repository: r.root, root: r.recordRoot}, nil
}

func (r *Repository) ResolveProductExclusion(
	record *RecordPathAdmission,
	resolver InertnessResolver,
) (*ProductExclusionAdmission, error) {
	if record == nil || record.repository != r.root || record.root != r.recordRoot {
		return nil, fail("RECORD_PATH_ADMISSION_REQUIRED", "admit product exclusion", errors.New("fixed record-path admission required"))
	}
	if resolver == nil {
		return nil, fail("RECORD_ROOT_POLICY_REQUIRED", "admit product exclusion", errors.New("trusted inertness resolver required"))
	}
	return &ProductExclusionAdmission{
		repository: r.root, root: r.recordRoot, record: record, resolver: resolver,
		decisions: make(map[string]RecordRootDecision),
	}, nil
}

func (r *Repository) requireRecordAdmission(admission *RecordPathAdmission) error {
	if admission == nil {
		return fail("RECORD_PATH_ADMISSION_REQUIRED", "use record path", errors.New("fixed record-path admission required"))
	}
	if admission.repository != r.root || admission.root != r.recordRoot {
		return fail("RECORD_ROOT_ADMISSION_MISMATCH", "use record path", errors.New("admission belongs to another repository"))
	}
	return nil
}

func (r *Repository) requireProductAdmission(commit OID, admission *ProductExclusionAdmission) error {
	if admission == nil {
		return fail("PRODUCT_EXCLUSION_ADMISSION_REQUIRED", "exclude record root", errors.New("policy-bound admission required"))
	}
	if admission.repository != r.root || admission.root != r.recordRoot ||
		admission.record == nil || admission.record.repository != r.root {
		return fail("RECORD_ROOT_ADMISSION_MISMATCH", "exclude record root", errors.New("admission belongs to another repository"))
	}
	if err := r.validateOID(commit); err != nil {
		return err
	}
	admission.mu.Lock()
	defer admission.mu.Unlock()
	decision, ok := admission.decisions[commit.String()]
	if !ok {
		request := RecordRootRequest{
			Kind: "baton.record-root-inertness/v1", Repository: r.root,
			RecordRoot: r.recordRoot, Commit: commit.String(),
		}
		var err error
		decision, err = admission.resolver(request)
		if err != nil {
			return fail("RECORD_ROOT_POLICY_UNAVAILABLE", "resolve record-root inertness", err)
		}
		if decision.Kind != request.Kind || decision.Repository != request.Repository ||
			decision.RecordRoot != request.RecordRoot || decision.Commit != request.Commit ||
			(decision.Decision != "inert" && decision.Decision != "consumed") {
			return fail("UNTRUSTED_RECORD_ROOT_POLICY", "resolve record-root inertness", errors.New("decision does not bind the exact request"))
		}
		admission.decisions[commit.String()] = decision
	}
	if decision.Decision != "inert" {
		return fail("RECORD_ROOT_CONSUMED", "exclude record root", fmt.Errorf("%s affects product behavior at %s", r.recordRoot, commit.String()))
	}
	return nil
}

func (r *Repository) assertRecordRootAt(commit OID, allowMissing bool) error {
	return r.assertRecordRootAtPath(commit, allowMissing, r.recordRoot)
}

func (r *Repository) assertRecordRootAtLegacy(commit OID, allowMissing bool) error {
	return r.assertRecordRootAtPath(commit, allowMissing, LegacyRecordsRoot)
}

func (r *Repository) assertRecordRootAtPath(commit OID, allowMissing bool, recordRoot string) error {
	if err := r.validateOID(commit); err != nil {
		return err
	}
	parts := strings.Split(recordRoot, "/")
	for index := range parts {
		prefix := strings.Join(parts[:index+1], "/")
		raw, err := r.run(nil, nil, "ls-tree", "-z", commit.String(), "--", prefix)
		if err != nil {
			return err
		}
		if len(raw) == 0 && allowMissing {
			return nil
		}
		if len(raw) == 0 {
			return fail("RECORD_ROOT_NOT_FOUND", "inspect record root", fmt.Errorf("%s is absent", recordRoot))
		}
		if raw[len(raw)-1] != 0 || bytes.Count(raw, []byte{0}) != 1 {
			return fail("MALFORMED_GIT_TREE", "inspect record root", errors.New("ambiguous tree entry"))
		}
		entry := raw[:len(raw)-1]
		header, pathBytes, ok := bytes.Cut(entry, []byte{'\t'})
		fields := strings.Fields(string(header))
		if !ok || len(fields) != 3 || string(pathBytes) != prefix {
			return fail("MALFORMED_GIT_TREE", "inspect record root", errors.New("malformed tree entry"))
		}
		if fields[0] == "120000" {
			return fail("SYMLINKED_RECORD_ROOT", "inspect record root", fmt.Errorf("%s traverses a symlink", prefix))
		}
		if fields[1] != "tree" {
			return fail("INVALID_RECORD_ROOT", "inspect record root", fmt.Errorf("%s is not a tree", prefix))
		}
	}
	return nil
}
func (r *Repository) TreeOID(commit OID) (OID, error) {
	if err := r.validateOID(commit); err != nil {
		return OID{}, err
	}
	raw, err := r.run(nil, nil, "rev-parse", "--verify", commit.String()+"^{tree}")
	if err != nil {
		return OID{}, err
	}
	return r.parseOID(string(raw))
}
func (r *Repository) ReadBlob(commit OID, relativePath string) ([]byte, error) {
	values, err := r.ReadBlobs(commit, []string{relativePath})
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), values[relativePath]...), nil
}
func (r *Repository) ReadBlobs(commit OID, paths []string) (map[string][]byte, error) {
	if err := r.validateOID(commit); err != nil {
		return nil, err
	}
	if len(paths) == 0 || len(paths) > MaxBatchPaths {
		return nil, fail("RESOURCE_LIMIT", "read blobs", fmt.Errorf("requires 1-%d paths", MaxBatchPaths))
	}
	seen := make(map[string]struct{}, len(paths))
	var input strings.Builder
	for _, name := range paths {
		if err := ValidatePath(name, false); err != nil {
			return nil, err
		}
		if _, duplicate := seen[name]; duplicate {
			return nil, fail("DUPLICATE_PATH", "read blobs", fmt.Errorf("%s is repeated", name))
		}
		seen[name] = struct{}{}
		input.WriteString(commit.String())
		input.WriteByte(':')
		input.WriteString(name)
		input.WriteByte('\n')
	}
	raw, err := r.run([]byte(input.String()), nil, "cat-file", "--batch")
	if err != nil {
		return nil, err
	}
	result := make(map[string][]byte, len(paths))
	offset := 0
	total := 0
	for _, name := range paths {
		lineEnd := bytes.IndexByte(raw[offset:], '\n')
		if lineEnd < 0 {
			return nil, fail("INVALID_GIT_OUTPUT", "read blobs", errors.New("cat-file header is truncated"))
		}
		lineEnd += offset
		header := string(raw[offset:lineEnd])
		offset = lineEnd + 1
		if strings.HasSuffix(header, " missing") {
			return nil, fail("BLOB_NOT_FOUND", "read blobs", fmt.Errorf("%s is absent at %s", name, commit))
		}
		fields := strings.Fields(header)
		if len(fields) != 3 || fields[1] != "blob" {
			return nil, fail("INVALID_GIT_OUTPUT", "read blobs", fmt.Errorf("unexpected cat-file header %q", header))
		}
		if _, err := r.parseOID(fields[0]); err != nil {
			return nil, err
		}
		size, err := strconv.Atoi(fields[2])
		if err != nil || size < 0 {
			return nil, fail("INVALID_GIT_OUTPUT", "read blobs", fmt.Errorf("invalid blob size %q", fields[2]))
		}
		if size > MaxFileBytes {
			return nil, fail("RESOURCE_LIMIT", "read blobs", fmt.Errorf("%s exceeds %d bytes", name, MaxFileBytes))
		}
		total += size
		if total > MaxBatchBytes || offset+size >= len(raw) {
			return nil, fail("RESOURCE_LIMIT", "read blobs", fmt.Errorf("batch exceeds %d bytes or is truncated", MaxBatchBytes))
		}
		body := append([]byte(nil), raw[offset:offset+size]...)
		offset += size
		if raw[offset] != '\n' {
			return nil, fail("INVALID_GIT_OUTPUT", "read blobs", errors.New("cat-file body has no delimiter"))
		}
		offset++
		result[name] = body
	}
	if offset != len(raw) {
		return nil, fail("INVALID_GIT_OUTPUT", "read blobs", errors.New("cat-file returned trailing output"))
	}
	return result, nil
}
func (r *Repository) ListTree(commit OID) ([]TreeEntry, error) {
	if err := r.validateOID(commit); err != nil {
		return nil, err
	}
	raw, err := r.run(nil, nil, "ls-tree", "-r", "-z", "--full-tree", commit.String())
	if err != nil {
		return nil, err
	}
	if len(raw) > MaxTreeBytes {
		return nil, fail("RESOURCE_LIMIT", "list tree", fmt.Errorf("tree output exceeds %d bytes", MaxTreeBytes))
	}
	records := bytes.Split(raw, []byte{0})
	entries := make([]TreeEntry, 0, len(records))
	for _, record := range records {
		if len(record) == 0 {
			continue
		}
		header, nameBytes, ok := bytes.Cut(record, []byte{'\t'})
		if !ok || !utf8.Valid(nameBytes) {
			return nil, fail("INVALID_GIT_OUTPUT", "list tree", errors.New("malformed or non-UTF-8 tree entry"))
		}
		parts := strings.Split(string(header), " ")
		if len(parts) != 3 {
			return nil, fail("INVALID_GIT_OUTPUT", "list tree", errors.New("malformed tree header"))
		}
		name := string(nameBytes)
		if err := ValidatePath(name, false); err != nil {
			return nil, err
		}
		oid, err := r.parseOID(parts[2])
		if err != nil {
			return nil, err
		}
		if parts[1] != "blob" && parts[1] != "commit" {
			return nil, fail("INVALID_GIT_OUTPUT", "list tree", fmt.Errorf("unexpected leaf type %s", parts[1]))
		}
		entries = append(entries, TreeEntry{Path: name, Mode: parts[0], Type: parts[1], OID: oid})
		if len(entries) > MaxTreeEntries {
			return nil, fail("RESOURCE_LIMIT", "list tree", fmt.Errorf("tree exceeds %d entries", MaxTreeEntries))
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, nil
}
// isReservedRecordPath reports whether a repository-relative path names the
// configured records root or the historical legacy records root, either
// exactly or beneath them. Both stay reserved so neither the configured root
// nor the legacy fallback can be forged by a model-directed worker.
func (r *Repository) isReservedRecordPath(name string) bool {
	for _, root := range []string{r.recordRoot, LegacyRecordsRoot} {
		if name == root || strings.HasPrefix(name, root+"/") {
			return true
		}
	}
	return false
}

func (r *Repository) ProductTreeIdentity(
	commit OID,
	admission *ProductExclusionAdmission,
) (ProductIdentity, error) {
	if err := r.requireProductAdmission(commit, admission); err != nil {
		return ProductIdentity{}, err
	}
	if err := r.assertRecordRootAt(commit, true); err != nil {
		return ProductIdentity{}, err
	}
	if err := r.assertRecordRootAtLegacy(commit, true); err != nil {
		return ProductIdentity{}, err
	}
	tree, err := r.TreeOID(commit)
	if err != nil {
		return ProductIdentity{}, err
	}
	entries, err := r.ListTree(commit)
	if err != nil {
		return ProductIdentity{}, err
	}
	hasher := sha256.New()
	productEntries := make([]TreeEntry, 0, len(entries))
	for _, entry := range entries {
		if r.isReservedRecordPath(entry.Path) {
			continue
		}
		productEntries = append(productEntries, entry)
		io.WriteString(hasher, entry.Path)
		hasher.Write([]byte{0})
		io.WriteString(hasher, entry.Mode)
		hasher.Write([]byte{0})
		io.WriteString(hasher, entry.Type)
		hasher.Write([]byte{0})
		io.WriteString(hasher, entry.OID.String())
		hasher.Write([]byte{'\n'})
	}
	return ProductIdentity{
		Candidate: commit, CandidateTree: tree,
		ProductTree: "sha256:" + hex.EncodeToString(hasher.Sum(nil)),
		Entries:     productEntries,
	}, nil
}
func (r *Repository) Parents(commit OID) ([]OID, error) {
	if err := r.validateOID(commit); err != nil {
		return nil, err
	}
	raw, err := r.run(nil, nil, "show", "-s", "--format=%P", commit.String())
	if err != nil {
		return nil, err
	}
	fields := strings.Fields(string(raw))
	result := make([]OID, 0, len(fields))
	for _, field := range fields {
		oid, err := r.parseOID(field)
		if err != nil {
			return nil, err
		}
		result = append(result, oid)
	}
	return result, nil
}
func (r *Repository) CommitTimestamp(commit OID) (int64, error) {
	if err := r.validateOID(commit); err != nil {
		return 0, err
	}
	raw, err := r.run(nil, nil, "show", "-s", "--format=%ct", commit.String())
	if err != nil {
		return 0, err
	}
	value, err := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
	if err != nil || value < 0 {
		return 0, fail("INVALID_GIT_OUTPUT", "read commit timestamp", err)
	}
	return value, nil
}

// CommitIdentity returns the one validated author/committer identity on an
// engine-created commit. It rejects malformed UTF-8 and mixed attribution.
func (r *Repository) CommitIdentity(commit OID) (Identity, error) {
	if err := r.validateOID(commit); err != nil {
		return Identity{}, err
	}
	r.identityMu.Lock()
	identity, present := r.identities[commit.String()]
	r.identityMu.Unlock()
	if present {
		return identity, nil
	}
	raw, err := r.run(nil, nil, "show", "-s", "--format=%an%x00%ae%x00%cn%x00%ce", commit.String())
	if err != nil {
		return Identity{}, err
	}
	if !utf8.Valid(raw) {
		return Identity{}, fail("INVALID_COMMIT_IDENTITY", "read commit identity", errors.New("identity is malformed UTF-8"))
	}
	raw = bytes.TrimSuffix(raw, []byte{'\n'})
	fields := bytes.Split(raw, []byte{0})
	if len(fields) != 4 || !bytes.Equal(fields[0], fields[2]) || !bytes.Equal(fields[1], fields[3]) {
		return Identity{}, fail("INVALID_COMMIT_IDENTITY", "read commit identity", errors.New("author and committer identities differ"))
	}
	identity = Identity{Name: string(fields[0]), Email: string(fields[1])}
	if err := ValidateIdentity(identity); err != nil {
		return Identity{}, err
	}
	r.identityMu.Lock()
	r.identities[commit.String()] = identity
	r.identityMu.Unlock()
	return identity, nil
}
func (r *Repository) IsAncestor(ancestor, descendant OID) (bool, error) {
	if err := r.validateOID(ancestor); err != nil {
		return false, err
	}
	if err := r.validateOID(descendant); err != nil {
		return false, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), CommandTimeout)
	defer cancel()
	result := r.runStatus(ctx, "", 0, MaxDiagnostic, "merge-base", "--is-ancestor", ancestor.String(), descendant.String())
	if result.successful() && result.cleanupErr == nil {
		return true, nil
	}
	if result.started && result.reaped && result.groupQuiet && !result.timedOut &&
		!result.overflow && result.exitCode == 1 && result.signal == "" &&
		len(result.stdout) == 0 && len(result.stderr) == 0 && result.cleanupErr == nil {
		return false, nil
	}
	return false, fail("GIT_EXECUTION_FAILED", "check ancestry", result.waitErr)
}
func (r *Repository) ChangedPaths(base, candidate OID) ([]string, error) {
	if err := r.validateOID(base); err != nil {
		return nil, err
	}
	if err := r.validateOID(candidate); err != nil {
		return nil, err
	}
	raw, err := r.run(nil, nil, "diff", "--name-only", "-z", "--no-renames", base.String(), candidate.String(), "--")
	if err != nil {
		return nil, err
	}
	fields := bytes.Split(raw, []byte{0})
	result := make([]string, 0, len(fields))
	for _, field := range fields {
		if len(field) == 0 {
			continue
		}
		if !utf8.Valid(field) {
			return nil, fail("INVALID_GIT_OUTPUT", "changed paths", errors.New("path is not UTF-8"))
		}
		name := string(field)
		if err := ValidatePath(name, false); err != nil {
			return nil, err
		}
		result = append(result, name)
		if len(result) > MaxChangedPaths {
			return nil, fail("RESOURCE_LIMIT", "changed paths", fmt.Errorf("more than %d paths", MaxChangedPaths))
		}
	}
	sort.Strings(result)
	return result, nil
}

func validateRecordRootDiffOutput(raw []byte) error {
	if len(raw) > maxRecordRootDiffBytes {
		return fail(
			"RECORD_TREE_INVENTORY_LIMIT",
			"compare reserved record root",
			fmt.Errorf(
				"record-root comparison exceeds %d bytes",
				maxRecordRootDiffBytes,
			),
		)
	}
	return nil
}

func (r *Repository) AssertCandidateRecordRootUnchanged(base, candidate OID) error {
	if err := r.validateOID(base); err != nil {
		return err
	}
	if err := r.validateOID(candidate); err != nil {
		return err
	}
	// Both the configured records root and the historical legacy root stay
	// reserved: a model-directed candidate may never touch either, so the
	// legacy fallback can never be forged and the configured root is never a
	// product input.
	for _, root := range []string{r.recordRoot, LegacyRecordsRoot} {
		if err := r.assertRecordRootAtPath(base, true, root); err != nil {
			return err
		}
		if err := r.assertRecordRootAtPath(candidate, true, root); err != nil {
			return err
		}
		raw, err := r.run(
			nil,
			nil,
			"diff-tree", "--no-commit-id", "--raw", "-z",
			base.String(), candidate.String(), "--", root,
		)
		if err != nil {
			return err
		}
		if err := validateRecordRootDiffOutput(raw); err != nil {
			return err
		}
		if len(raw) != 0 {
			return fail(
				"RESERVED_RECORD_ROOT_CHANGED",
				"compare reserved record root",
				fmt.Errorf(
					"candidate %s changes reserved record root %s from base %s",
					candidate.String(),
					root,
					base.String(),
				),
			)
		}
	}
	return nil
}
func (r *Repository) FirstParentRange(base, head OID) ([]OID, error) {
	if err := r.validateOID(base); err != nil {
		return nil, err
	}
	if err := r.validateOID(head); err != nil {
		return nil, err
	}
	ancestor, err := r.IsAncestor(base, head)
	if err != nil {
		return nil, err
	}
	if !ancestor {
		return nil, fail("INVALID_HISTORY", "first-parent history", errors.New("base is not an ancestor of head"))
	}
	raw, err := r.run(nil, nil, "rev-list", "--first-parent", "--reverse", "--max-count="+strconv.Itoa(MaxHistory+1), head.String(), "^"+base.String())
	if err != nil {
		return nil, err
	}
	lines := strings.Fields(string(raw))
	if len(lines) > MaxHistory {
		return nil, fail("RESOURCE_LIMIT", "first-parent history", fmt.Errorf("more than %d commits", MaxHistory))
	}
	result := make([]OID, 0, len(lines))
	for _, line := range lines {
		oid, err := r.parseOID(line)
		if err != nil {
			return nil, err
		}
		result = append(result, oid)
	}
	return result, nil
}
