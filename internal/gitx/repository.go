package gitx

import (
	"bytes"
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
	"unicode/utf8"
)

const (
	MaxHeadRefs       = 128
	MaxBatchPaths     = 1025
	MaxFileBytes      = 262_144
	MaxBatchBytes     = MaxBatchPaths * MaxFileBytes
	MaxTreeEntries    = 100_000
	MaxTreeBytes      = 64 * 1024 * 1024
	MaxChangedPaths   = 100_000
	MaxHistory        = 10_000
	MaxMessageBytes   = 1000
	MaxRepositoryPath = 1000
)

// Error is a stable, typed failure at the literal Git boundary.
type Error struct {
	Code string
	Op   string
	Err  error
}

func (e *Error) Error() string {
	if e.Err == nil {
		return e.Code + ": " + e.Op
	}
	return e.Code + ": " + e.Op + ": " + e.Err.Error()
}

func (e *Error) Unwrap() error { return e.Err }

func fail(code, op string, err error) error {
	return &Error{Code: code, Op: op, Err: err}
}

// ObjectFormat fixes the OID width for one opened repository.
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

// OID is an immutable, object-format-bound object identifier.
type OID struct {
	format ObjectFormat
	hex    string
}

func (o OID) String() string       { return o.hex }
func (o OID) Format() ObjectFormat { return o.format }
func (o OID) IsZero() bool         { return o.hex == "" }

// ParseOID validates an OID against a fixed repository object format.
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

// TreeEntry is one recursive leaf entry in a captured tree.
type TreeEntry struct {
	Path string
	Mode string
	Type string
	OID  OID
}

// Repository is an admitted literal Git boundary.
type Repository struct {
	root   string
	git    string
	format ObjectFormat

	// afterRefPrepare is a same-package test seam. Production repositories
	// leave it nil; it lets tests challenge the prepared ref locks without
	// adding a callback to the exported mechanical boundary.
	afterRefPrepare func()
}

// Open admits an existing repository and an explicit absolute Git executable.
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
	repo := &Repository{root: filepath.Clean(rootCandidate), git: git}
	rootBytes, err := repo.run(nil, nil, "rev-parse", "--path-format=absolute", "--show-toplevel")
	if err != nil {
		// Bare repositories have no worktree but remain valid mechanical stores.
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
	return repo, nil
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
		"HOME=" + home,
		"XDG_CONFIG_HOME=" + filepath.Join(home, "xdg"),
		"LANG=C",
		"LC_ALL=C",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_ATTR_NOSYSTEM=1",
		"GIT_NO_REPLACE_OBJECTS=1",
		"GIT_LITERAL_PATHSPECS=1",
		"GIT_PROTOCOL_FROM_USER=0",
		"GIT_TERMINAL_PROMPT=0",
		"GCM_INTERACTIVE=never",
		"GIT_PAGER=cat",
		"PAGER=cat",
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_CONFIG_COUNT=4",
		"GIT_CONFIG_KEY_0=core.hooksPath",
		"GIT_CONFIG_VALUE_0=" + filepath.Join(home, "hooks"),
		"GIT_CONFIG_KEY_1=core.fsmonitor",
		"GIT_CONFIG_VALUE_1=false",
		"GIT_CONFIG_KEY_2=core.quotePath",
		"GIT_CONFIG_VALUE_2=false",
		"GIT_CONFIG_KEY_3=core.attributesFile",
		"GIT_CONFIG_VALUE_3=" + attributesFile,
	}
	return append(environment, extra...)
}

func (r *Repository) run(stdin []byte, extraEnv []string, args ...string) ([]byte, error) {
	return r.runWithAttributes(stdin, extraEnv, "/dev/null", args...)
}

func (r *Repository) newLiteralCommand(extraEnv []string, attributesFile string, args ...string) (*exec.Cmd, func(), error) {
	home, err := os.MkdirTemp("", "sworn-git-home-*")
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() { _ = os.RemoveAll(home) }
	hooks := filepath.Join(home, "hooks")
	if err := os.Mkdir(hooks, 0o700); err != nil {
		cleanup()
		return nil, nil, err
	}
	commandArgs := append([]string{"-C", r.root, "-c", "core.hooksPath=" + hooks, "-c", "core.fsmonitor=false", "-c", "core.quotePath=false"}, args...)
	command := exec.Command(r.git, commandArgs...)
	command.Env = literalEnvironment(home, extraEnv, attributesFile)
	return command, cleanup, nil
}

func (r *Repository) runWithAttributes(stdin []byte, extraEnv []string, attributesFile string, args ...string) ([]byte, error) {
	command, cleanup, err := r.newLiteralCommand(extraEnv, attributesFile, args...)
	if err != nil {
		return nil, fail("GIT_EXECUTION_FAILED", "create isolated Git command", err)
	}
	defer cleanup()
	if stdin != nil {
		command.Stdin = bytes.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if len(message) > 4096 {
			message = message[:4096]
		}
		if message == "" {
			message = err.Error()
		}
		return nil, fail("GIT_EXECUTION_FAILED", strings.Join(args, " "), errors.New(message))
	}
	return stdout.Bytes(), nil
}

// ValidateHeadRef admits only canonical full refs/heads refs.
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

// ValidatePath admits one canonical repository-relative path.
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

// TreeOID returns the ordinary tree of an exact commit.
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

// ReadBlob reads one regular blob from an exact captured commit/tree.
func (r *Repository) ReadBlob(commit OID, relativePath string) ([]byte, error) {
	values, err := r.ReadBlobs(commit, []string{relativePath})
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), values[relativePath]...), nil
}

// ReadBlobs reads a bounded ordered batch at one captured OID.
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

// ListTree returns sorted recursive leaf entries from an exact captured tree.
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

// ProductTreeIdentity hashes path, mode, type and object identity outside one
// supplied canonical exclusion prefix.
func (r *Repository) ProductTreeIdentity(commit OID, excludePrefix string) (string, error) {
	if err := ValidatePath(excludePrefix, false); err != nil {
		return "", err
	}
	entries, err := r.ListTree(commit)
	if err != nil {
		return "", err
	}
	hasher := sha256.New()
	for _, entry := range entries {
		if entry.Path == excludePrefix || strings.HasPrefix(entry.Path, excludePrefix+"/") {
			continue
		}
		io.WriteString(hasher, entry.Path)
		hasher.Write([]byte{0})
		io.WriteString(hasher, entry.Mode)
		hasher.Write([]byte{0})
		io.WriteString(hasher, entry.Type)
		hasher.Write([]byte{0})
		io.WriteString(hasher, entry.OID.String())
		hasher.Write([]byte{'\n'})
	}
	return "sha256:" + hex.EncodeToString(hasher.Sum(nil)), nil
}

// Parents returns exact ordered parents for a commit.
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

// CommitTimestamp returns the integer committer timestamp.
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

// IsAncestor reports exact graph ancestry.
func (r *Repository) IsAncestor(ancestor, descendant OID) (bool, error) {
	if err := r.validateOID(ancestor); err != nil {
		return false, err
	}
	if err := r.validateOID(descendant); err != nil {
		return false, err
	}
	_, err := r.run(nil, nil, "merge-base", "--is-ancestor", ancestor.String(), descendant.String())
	if err == nil {
		return true, nil
	}
	var typed *Error
	if errors.As(err, &typed) && strings.Contains(typed.Error(), "exit status 1") {
		return false, nil
	}
	// Git stderr is normally empty for a valid non-ancestor. Re-run a
	// deterministic merge-base and classify its exit code through exec.
	command := exec.Command(r.git, "-C", r.root, "merge-base", "--is-ancestor", ancestor.String(), descendant.String())
	home, homeErr := os.MkdirTemp("", "sworn-git-home-*")
	if homeErr != nil {
		return false, err
	}
	defer os.RemoveAll(home)
	command.Env = literalEnvironment(home, nil, "/dev/null")
	runErr := command.Run()
	if exitErr := new(exec.ExitError); errors.As(runErr, &exitErr) && exitErr.ExitCode() == 1 {
		return false, nil
	}
	return false, err
}

// ChangedPaths returns sorted repository paths changed between two exact OIDs.
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

// FirstParentHistory returns commits after base through head, oldest first.
func (r *Repository) FirstParentHistory(base, head OID) ([]OID, error) {
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
