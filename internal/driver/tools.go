package driver

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"
)

const (
	MaxToolCalls          = 256
	MaxToolArgumentBytes  = 262_144
	MaxToolResultBytes    = 262_144
	MaxToolPathBytes      = 4_096
	MaxToolMatches        = 256
	MaxBashScriptBytes    = 131_072
	MaxBashCombinedOutput = 262_144
	MaxToolWalkEntries    = 4_096
	MaxToolScanBytes      = 4_194_304
)

const swornSubmitInputSchema = `{"type":"object","properties":{"submission":{"type":"object","properties":{"schema_version":{"type":"string","enum":["sworn.submission/v1"]},"invocation_id":{"type":"string"},"responsibility":{"type":"string","enum":["planner_proposal","implementer_design","implementer_implementation","captain_review","work_verification","assembly_verification"]},"summary":{"type":"string"},"detail":{"type":"string"},"plan":{"type":"object","properties":{"byte_count":{"type":"integer"},"digest":{"type":"string"},"bytes":{"type":"string"}},"required":["byte_count","digest","bytes"],"additionalProperties":false},"checks":{"type":"object","properties":{"byte_count":{"type":"integer"},"digest":{"type":"string"},"bytes":{"type":"string"}},"required":["byte_count","digest","bytes"],"additionalProperties":false},"decision":{"type":"object","properties":{"outcome":{"type":"string","enum":["proceed","revise","escalate","pass","fail","blocked"]}},"required":["outcome"],"additionalProperties":false}},"required":["schema_version","invocation_id","responsibility","summary","detail"],"additionalProperties":false}},"required":["submission"],"additionalProperties":false}`

type toolPathEntry struct {
	Relative  string
	Directory bool
	Body      []byte
}

type providerToolDefinition struct {
	Name        string
	Description string
	InputSchema json.RawMessage
}

type providerToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

type providerToolResult struct {
	ID      string
	Name    string
	Content []byte
	Failed  bool
}

type toolSession struct {
	mu         sync.Mutex
	invocation Invocation
	projection *InputProjection
	before     workspaceManifest
	server     *submissionServer
	calls      int
	terminal   bool
	submitErr  error
	submission []byte
	seal       []byte
	closed     bool
}

func newToolSession(invocation Invocation) (*toolSession, error) {
	projection, err := StageInputProjection(
		invocation.HostWorkspace,
		invocation.Request.Inputs,
		invocation.Inputs,
	)
	if err != nil {
		return nil, err
	}
	session := &toolSession{invocation: invocation, projection: projection}
	ok := false
	defer func() {
		if !ok {
			_ = projection.Close()
		}
	}()
	if invocation.Request.Workspace.Access == ReadOnly {
		session.before, err = captureWorkspaceManifest(invocation.HostWorkspace)
		if err != nil {
			return nil, err
		}
	}
	session.server, err = newSubmissionServer(invocation.Permission)
	if err != nil {
		return nil, err
	}
	ok = true
	return session, nil
}

func toolDefinitions(access WorkspaceAccess) []providerToolDefinition {
	definitions := []providerToolDefinition{
		{Name: "Bash", Description: "Run one bounded networkless command in the workspace.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"script":{"type":"string"}},"required":["script"],"additionalProperties":false}`)},
		{Name: "Read", Description: "Read one bounded workspace or projected-input file.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"],"additionalProperties":false}`)},
		{Name: "Glob", Description: "List bounded workspace or projected-input paths matching a pattern.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"pattern":{"type":"string"},"path":{"type":"string"}},"required":["pattern","path"],"additionalProperties":false}`)},
		{Name: "Grep", Description: "Search bounded workspace or projected-input text files.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"pattern":{"type":"string"},"path":{"type":"string"}},"required":["pattern","path"],"additionalProperties":false}`)},
	}
	if access == ReadWrite {
		definitions = append(definitions,
			providerToolDefinition{Name: "Write", Description: "Write one bounded workspace file.",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}},"required":["path","content"],"additionalProperties":false}`)},
			providerToolDefinition{Name: "Edit", Description: "Replace one exact occurrence in a bounded workspace file.",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"old_string":{"type":"string"},"new_string":{"type":"string"}},"required":["path","old_string","new_string"],"additionalProperties":false}`)},
		)
	}
	definitions = append(definitions, providerToolDefinition{
		Name:        "sworn_submit",
		Description: "Include only the prompt's result_fields. For plan/checks use decoded byte_count, sha256:<64 lowercase hex> digest, and base64 bytes; detail may be empty.",
		InputSchema: json.RawMessage(swornSubmitInputSchema),
	})
	return definitions
}

func (session *toolSession) execute(
	ctx context.Context,
	call providerToolCall,
) providerToolResult {
	result := providerToolResult{ID: call.ID, Name: call.Name}
	session.mu.Lock()
	if session.closed || session.terminal {
		session.mu.Unlock()
		result.Content, result.Failed = []byte("error:TOOL_SESSION_CLOSED"), true
		return result
	}
	session.calls++
	if session.calls > MaxToolCalls {
		session.terminal = true
		session.submitErr = fail("RESOURCE_LIMIT")
		session.mu.Unlock()
		result.Content, result.Failed = []byte("error:RESOURCE_LIMIT"), true
		return result
	}
	session.mu.Unlock()
	if len(call.Arguments) == 0 || len(call.Arguments) > MaxToolArgumentBytes {
		result.Content, result.Failed = []byte("error:INVALID_TOOL_ARGUMENT"), true
		return result
	}
	var body []byte
	var err error
	switch call.Name {
	case "Read":
		body, err = session.read(call.Arguments)
	case "Write":
		body, err = session.write(call.Arguments)
	case "Edit":
		body, err = session.edit(call.Arguments)
	case "Glob":
		body, err = session.glob(call.Arguments)
	case "Grep":
		body, err = session.grep(call.Arguments)
	case "Bash":
		body, err = session.bash(ctx, call.Arguments)
	case "sworn_submit":
		body, err = session.submit(call.Arguments)
	default:
		err = fail("TOOL_NOT_ALLOWED")
	}
	if err != nil {
		result.Content = []byte("error:" + contractCode(err))
		result.Failed = true
		return result
	}
	if len(body) > MaxToolResultBytes || !utf8.Valid(body) {
		result.Content, result.Failed = []byte("error:RESOURCE_LIMIT"), true
		return result
	}
	result.Content = body
	return result
}

func (session *toolSession) read(arguments []byte) ([]byte, error) {
	var request struct {
		Path string `json:"path"`
	}
	if err := decodeToolArguments(arguments, []string{"path"}, &request); err != nil {
		return nil, err
	}
	target, root, _, err := session.resolve(request.Path, false, false)
	if err != nil {
		return nil, err
	}
	return readToolPath(root, target)
}

func (session *toolSession) write(arguments []byte) ([]byte, error) {
	if session.invocation.Request.Workspace.Access != ReadWrite {
		return nil, fail("TOOL_NOT_ALLOWED")
	}
	var request struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := decodeToolArguments(arguments, []string{"path", "content"}, &request); err != nil ||
		len(request.Content) > MaxToolResultBytes {
		return nil, fail("INVALID_TOOL_ARGUMENT")
	}
	target, root, _, err := session.resolve(request.Path, true, true)
	if err != nil {
		return nil, err
	}
	if err := writeToolPath(root, target, []byte(request.Content)); err != nil {
		return nil, fail("TOOL_WRITE_FAILED")
	}
	return []byte("ok"), nil
}

func (session *toolSession) edit(arguments []byte) ([]byte, error) {
	if session.invocation.Request.Workspace.Access != ReadWrite {
		return nil, fail("TOOL_NOT_ALLOWED")
	}
	var request struct {
		Path      string `json:"path"`
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
	}
	if err := decodeToolArguments(
		arguments,
		[]string{"path", "old_string", "new_string"},
		&request,
	); err != nil || request.OldString == "" {
		return nil, fail("INVALID_TOOL_ARGUMENT")
	}
	target, root, _, err := session.resolve(request.Path, false, true)
	if err != nil {
		return nil, err
	}
	if err := editToolPath(
		root,
		target,
		[]byte(request.OldString),
		[]byte(request.NewString),
	); err != nil {
		return nil, fail("TOOL_EDIT_FAILED")
	}
	return []byte("ok"), nil
}

func (session *toolSession) glob(arguments []byte) ([]byte, error) {
	var request struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
	}
	if err := decodeToolArguments(arguments, []string{"pattern", "path"}, &request); err != nil ||
		validateToolText(request.Pattern) != nil {
		return nil, fail("INVALID_TOOL_ARGUMENT")
	}
	target, root, guest, err := session.resolve(request.Path, false, false)
	if err != nil {
		return nil, err
	}
	entries, err := listToolDirectory(root, target)
	if err != nil {
		return nil, err
	}
	var matches []string
	for _, entry := range entries {
		matched, matchErr := path.Match(request.Pattern, entry.Relative)
		if matchErr != nil {
			return nil, fail("INVALID_TOOL_ARGUMENT")
		}
		if matched {
			matches = append(matches, path.Join(guest, entry.Relative))
			if len(matches) > MaxToolMatches {
				return nil, fail("RESOURCE_LIMIT")
			}
		}
	}
	sort.Strings(matches)
	return []byte(strings.Join(matches, "\n")), nil
}

func (session *toolSession) grep(arguments []byte) ([]byte, error) {
	var request struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
	}
	if err := decodeToolArguments(arguments, []string{"pattern", "path"}, &request); err != nil ||
		validateToolText(request.Pattern) != nil {
		return nil, fail("INVALID_TOOL_ARGUMENT")
	}
	pattern, err := regexp.Compile(request.Pattern)
	if err != nil {
		return nil, fail("INVALID_TOOL_ARGUMENT")
	}
	target, root, guest, err := session.resolve(request.Path, false, false)
	if err != nil {
		return nil, err
	}
	entries, err := scanToolText(root, target)
	if err != nil {
		return nil, err
	}
	defer clearToolEntries(entries)
	var matches []string
	for _, entry := range entries {
		if entry.Directory {
			continue
		}
		display := guest
		if entry.Relative != "" {
			display = path.Join(guest, entry.Relative)
		}
		for lineNumber, line := range strings.Split(string(entry.Body), "\n") {
			if pattern.MatchString(line) {
				matches = append(matches, display+":"+itoa(lineNumber+1)+":"+line)
				if len(matches) > MaxToolMatches {
					return nil, fail("RESOURCE_LIMIT")
				}
			}
		}
	}
	if len(strings.Join(matches, "\n")) > MaxToolResultBytes {
		return nil, fail("RESOURCE_LIMIT")
	}
	return []byte(strings.Join(matches, "\n")), nil
}

func (session *toolSession) bash(ctx context.Context, arguments []byte) ([]byte, error) {
	var request struct {
		Script string `json:"script"`
	}
	if err := decodeToolArguments(arguments, []string{"script"}, &request); err != nil ||
		request.Script == "" || len(request.Script) > MaxBashScriptBytes ||
		!utf8.ValidString(request.Script) {
		return nil, fail("INVALID_TOOL_ARGUMENT")
	}
	return runToolBash(ctx, session.invocation, session.projection.Root(), request.Script)
}

func (session *toolSession) submit(arguments []byte) ([]byte, error) {
	value, err := decodeStrict(arguments, MaxToolArgumentBytes)
	if err != nil {
		return nil, err
	}
	root, err := closedObject(value, []string{"submission"}, nil)
	if err != nil {
		return nil, err
	}
	submission, err := decodeToolSubmission(root["submission"])
	if err != nil {
		session.finishSubmission(nil, nil, err)
		return nil, err
	}
	body, err := EncodeSubmission(submission)
	if err != nil {
		session.finishSubmission(nil, nil, err)
		return nil, err
	}
	seal, sealBytes, submitErr := session.server.Submit(body)
	if submitErr != nil || !seal.Accepted {
		if submitErr == nil {
			submitErr = fail("SUBMISSION_REJECTED")
		}
		session.finishSubmission(body, sealBytes, submitErr)
		return nil, submitErr
	}
	session.finishSubmission(body, sealBytes, nil)
	return []byte("accepted"), nil
}

func decodeToolSubmission(value any) (Submission, error) {
	root, err := closedObject(
		value,
		[]string{
			"schema_version", "invocation_id", "responsibility", "summary",
			"detail",
		},
		[]string{"plan", "checks", "decision"},
	)
	if err != nil {
		return Submission{}, err
	}
	for _, name := range []string{"plan", "checks"} {
		if root[name] == nil {
			continue
		}
		if _, err := closedObject(
			root[name],
			[]string{"byte_count", "digest", "bytes"},
			nil,
		); err != nil {
			return Submission{}, err
		}
	}
	if root["decision"] != nil {
		if _, err := closedObject(
			root["decision"],
			[]string{"outcome"},
			nil,
		); err != nil {
			return Submission{}, err
		}
	}
	body, err := canonicalJSON(root)
	if err != nil {
		return Submission{}, err
	}
	defer clearBytes(body)
	var submission Submission
	if json.Unmarshal(body, &submission) != nil {
		return Submission{}, fail("INVALID_SUBMISSION")
	}
	// Structurally valid fields outside this responsibility carry no authority.
	// Drop that model noise before the strict, permission-bound validation.
	switch submission.Responsibility {
	case PlannerProposal:
		submission.Checks, submission.Decision = nil, nil
	case ImplementerDesign:
		submission.Plan, submission.Checks, submission.Decision = nil, nil, nil
	case ImplementerImplementation:
		submission.Plan, submission.Decision = nil, nil
	case CaptainReview:
		submission.Plan, submission.Checks = nil, nil
	case WorkVerification, AssemblyVerification:
		submission.Plan = nil
	}
	if err := ValidateSubmission(submission); err != nil {
		return Submission{}, err
	}
	return submission, nil
}

func (session *toolSession) finishSubmission(body, seal []byte, err error) {
	session.mu.Lock()
	defer session.mu.Unlock()
	session.terminal = true
	session.submitErr = err
	session.submission = append([]byte(nil), body...)
	session.seal = append([]byte(nil), seal...)
}

func (session *toolSession) submitted() (bool, error) {
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.terminal && session.submitErr == nil, session.submitErr
}

func (session *toolSession) handoff() *SealedHandoff {
	session.mu.Lock()
	defer session.mu.Unlock()
	if !session.terminal || session.submitErr != nil ||
		len(session.submission) == 0 || len(session.seal) == 0 {
		return nil
	}
	return &SealedHandoff{
		SubmissionBytes:  append([]byte(nil), session.submission...),
		SubmissionDigest: Digest(session.submission),
		SealBytes:        append([]byte(nil), session.seal...),
		SealDigest:       Digest(session.seal),
	}
}

func (session *toolSession) Close() error {
	session.mu.Lock()
	if session.closed {
		session.mu.Unlock()
		return nil
	}
	session.closed = true
	session.mu.Unlock()
	var result error
	if session.invocation.Request.Workspace.Access == ReadOnly {
		after, err := captureWorkspaceManifest(session.invocation.HostWorkspace)
		if err != nil || !equalManifest(session.before, after) {
			result = fail("WORKSPACE_MUTATED")
		}
	}
	if err := session.projection.Close(); err != nil && result == nil {
		result = err
	}
	return result
}

func (session *toolSession) resolve(
	guest string,
	allowMissing bool,
	requireWorkspace bool,
) (string, string, string, error) {
	if validateToolText(guest) != nil || !path.IsAbs(guest) || path.Clean(guest) != guest {
		return "", "", "", fail("TOOL_PATH_INVALID")
	}
	var root, relative string
	switch {
	case guest == GuestWorkspacePath || strings.HasPrefix(guest, GuestWorkspacePath+"/"):
		root = session.invocation.HostWorkspace
		relative = strings.TrimPrefix(guest, GuestWorkspacePath)
	case !requireWorkspace && (guest == GuestInputPath || strings.HasPrefix(guest, GuestInputPath+"/")):
		root = session.projection.Root()
		relative = strings.TrimPrefix(guest, GuestInputPath)
	default:
		return "", "", "", fail("TOOL_PATH_INVALID")
	}
	relative = strings.TrimPrefix(relative, "/")
	if relative != "" {
		if err := validateRepositoryPath(relative); err != nil {
			return "", "", "", fail("TOOL_PATH_INVALID")
		}
	}
	target := filepath.Join(root, filepath.FromSlash(relative))
	if !pathBeneath(root, target) {
		return "", "", "", fail("TOOL_PATH_INVALID")
	}
	probe := target
	if allowMissing {
		probe = filepath.Dir(target)
		for !pathBeneath(root, probe) {
			return "", "", "", fail("TOOL_PATH_INVALID")
		}
		for {
			if _, err := os.Lstat(probe); err == nil {
				break
			}
			parent := filepath.Dir(probe)
			if parent == probe || !pathBeneath(root, parent) {
				return "", "", "", fail("TOOL_PATH_INVALID")
			}
			probe = parent
		}
	}
	resolved, err := filepath.EvalSymlinks(probe)
	if err != nil || !pathBeneath(root, resolved) {
		return "", "", "", fail("TOOL_PATH_INVALID")
	}
	return target, root, guest, nil
}

func decodeToolArguments(body []byte, required []string, target any) error {
	_, err := decodeTyped(body, MaxToolArgumentBytes, required, nil, target)
	if err != nil {
		return fail("INVALID_TOOL_ARGUMENT")
	}
	return nil
}

func validateToolText(value string) error {
	if value == "" || len(value) > MaxToolPathBytes || !utf8.ValidString(value) ||
		containsControlCharacter(value) {
		return fail("INVALID_TOOL_ARGUMENT")
	}
	return nil
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	var body [24]byte
	index := len(body)
	for value > 0 {
		index--
		body[index] = byte(value%10) + '0'
		value /= 10
	}
	return string(body[index:])
}

func joinErrors(first, second error) error {
	if first != nil {
		return first
	}
	return second
}

func isContextError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
