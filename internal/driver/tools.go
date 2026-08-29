package driver

import (
	"bytes"
	"context"
	"encoding/base64"
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

	"github.com/swornagent/sworn/internal/baton"
)

const (
	MaxToolCalls = 256
	// MaxSessionToolCalls is a cumulative runaway guard sized so it never
	// binds before the invocation turn budget for a patient worker.
	MaxSessionToolCalls   = 10_000
	MaxToolArgumentBytes  = 262_144
	MaxToolResultBytes    = 262_144
	MaxToolPathBytes      = 4_096
	MaxToolMatches        = 256
	MaxToolReadBatchPaths = 64
	MaxBashScriptBytes    = 131_072
	MaxBashCombinedOutput = 262_144
	MaxToolWalkEntries    = 4_096
	MaxToolScanBytes      = 4_194_304
	// Corrections are bounded by the invocation's turn budget and timeout,
	// not by a per-type allowance; each one is durably accounted.
	MaxSubmissionCorrections = 1_000
)

// ToolSandboxPath is the PATH the Bash tool sandbox sets on every
// containment site (runToolBash, bubblewrapArguments); the environment-facts
// block reads this same constant so the two can never drift apart.
const ToolSandboxPath = "/usr/bin:/bin"

const swornSubmitInputSchema = `{"type":"object","properties":{"submission":{"type":"object","properties":{"schema_version":{"type":"string","enum":["sworn.submission/v1"]},"invocation_id":{"type":"string"},"responsibility":{"type":"string","enum":["planner_proposal","implementer_design","implementer_implementation","captain_review","captain_plan_review","work_verification","assembly_verification"]},"summary":{"type":"string"},"detail":{"type":"string"},"plan":{"type":"object","properties":{"byte_count":{"type":"integer"},"digest":{"type":"string"},"bytes":{"type":"string"},"path":{"type":"string"}},"required":["byte_count","digest"],"additionalProperties":false},"checks":{"type":"object","properties":{"byte_count":{"type":"integer"},"digest":{"type":"string"},"bytes":{"type":"string"},"path":{"type":"string"}},"required":["byte_count","digest"],"additionalProperties":false},"contracts":{"type":"object"},"decision":{"type":"object","properties":{"outcome":{"type":"string","enum":["proceed","revise","escalate","pass","fail","blocked"]}},"required":["outcome"],"additionalProperties":false}},"required":["schema_version","invocation_id","responsibility","summary","detail"],"additionalProperties":false}},"required":["submission"],"additionalProperties":false}`

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
	mu                sync.Mutex
	invocation        Invocation
	projection        *InputProjection
	before            workspaceManifest
	server            *submissionServer
	calls             int
	terminal          bool
	submitErr         error
	submitCorrections int
	submission        []byte
	seal              []byte
	yield             *Yield
	scratch           string
	closed            bool
	// observer emits the bounded tool-result projection on the
	// runtime-provided hook; nil when the invocation carries no hook.
	// redaction holds the credentials bound by runNative; they are
	// cleared with the session.
	observer  *toolResultObserver
	redaction [][]byte
}

func newToolSession(invocation Invocation) (*toolSession, error) {
	projection, err := StageInputProjection(
		invocation.HostWorkspace,
		invocation.Request.Inputs,
		invocation.Inputs,
		reservedMaskNames(invocation),
	)
	if err != nil {
		return nil, err
	}
	session := &toolSession{
		invocation: invocation,
		projection: projection,
		observer:   newToolResultObserver(invocation.ToolResultHook),
	}
	ok := false
	defer func() {
		if !ok {
			session.observer.close()
			_ = projection.Close()
			if session.scratch != "" {
				_ = os.RemoveAll(session.scratch)
			}
		}
	}()
	// One read-write scratch surface per invocation: /home/sworn and /tmp
	// persist across every command of this invocation (build caches survive
	// between tool calls) and are destroyed with the session. The isolation
	// boundary is between invocations and roles, never between consecutive
	// commands of the same worker.
	temp, tempErr := tempRoot()
	if tempErr != nil {
		return nil, tempErr
	}
	session.scratch, err = createInvocationScratchRoot(temp)
	if err != nil {
		return nil, err
	}
	if err := createInvocationScratchSurfaces(session.scratch); err != nil {
		return nil, err
	}
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

// createInvocationScratchRoot creates the per-invocation scratch directory
// under an already-resolved temp root. It is factored out of newToolSession
// so the invocation-scratch raise site (A1's :131) is independently testable
// against a caller-chosen temp root, distinct from StageInputProjection's
// own MkdirTemp call under the same root moments earlier.
func createInvocationScratchRoot(temp string) (string, error) {
	scratch, err := os.MkdirTemp(temp, "sworn-invocation-scratch-")
	if err != nil {
		return "", failSandboxStart("sandbox_start.invocation_scratch_create", err)
	}
	return scratch, nil
}

// createInvocationScratchSurfaces creates the per-invocation home and tmp
// directories inside an already-created scratch root. It is factored out of
// newToolSession so the home/tmp raise site (A1's :137) is independently
// testable against a caller-chosen scratch directory, without needing to
// race or predict os.MkdirTemp's own random name.
func createInvocationScratchSurfaces(scratch string) error {
	for _, surface := range []string{"home", "tmp"} {
		if err := os.Mkdir(
			filepath.Join(scratch, surface), 0o700,
		); err != nil {
			return failSandboxStart("sandbox_start.home_tmp_surface_create", err)
		}
	}
	return nil
}

// EnvironmentFacts states, for one invocation's actual Bash/Read tool
// sandbox, the facts a worker today re-derives by habit: every value is
// read from the constants and mask policy that enforce it, never a
// hand-maintained copy.
type EnvironmentFacts struct {
	ToolResultByteBudget int64    `json:"tool_result_byte_budget"`
	ToolSandboxPath      string   `json:"tool_sandbox_path"`
	MaskedPaths          []string `json:"masked_paths"`
	GitReadOnly          bool     `json:"git_read_only"`
	Note                 string   `json:"note"`
}

// buildEnvironmentFacts derives one invocation's EnvironmentFacts from the
// same values that enforce them: MaxToolResultBytes and ToolSandboxPath are
// the constants the Bash tool sandbox itself sets, and MaskedPaths/
// GitReadOnly mirror runToolBash's own reserved-name and withoutGit
// handling for the invocation's workspace access, so the block can never
// tell a worker something its own sandbox does not do.
func buildEnvironmentFacts(invocation Invocation) EnvironmentFacts {
	masked := reservedMaskNames(invocation)
	gitReadOnly := invocation.Request.Workspace.Access == ReadOnly
	if gitReadOnly {
		masked = withoutGit(masked)
	}
	return EnvironmentFacts{
		ToolResultByteBudget: MaxToolResultBytes,
		ToolSandboxPath:      ToolSandboxPath,
		MaskedPaths:          masked,
		GitReadOnly:          gitReadOnly,
		Note: "PATH is " + ToolSandboxPath +
			"; /usr is bound read-only from the host, so anything else under /usr must be invoked by absolute path.",
	}
}

func toolDefinitions(access WorkspaceAccess) []providerToolDefinition {
	definitions := []providerToolDefinition{
		{Name: "Bash", Description: "Run one bounded networkless command in the workspace. Pass exactly one of script or command with the shell command text.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"script":{"type":"string"},"command":{"type":"string"}},"additionalProperties":false}`)},
		{Name: "Read", Description: "Read one bounded workspace or projected-input file. Pass path with optional offset (1-based starting line) and limit (max lines) to window a large file within the byte budget; or pass paths (an array) to batch several files in one call - the same containment and byte budget apply to every path.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"offset":{"type":"integer"},"limit":{"type":"integer"},"paths":{"type":"array","items":{"type":"string"}}},"additionalProperties":false}`)},
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
	definitions = append(definitions,
		providerToolDefinition{
			Name:        "sworn_yield",
			Description: "Stop without Baton authority. Use question when a bounded answer may unblock this turn; use blocked when work cannot continue.",
			InputSchema: json.RawMessage(swornYieldInputSchema),
		},
		providerToolDefinition{
			Name:        "sworn_submit",
			Description: "Include only the prompt's result_fields. For plan/checks/contracts declare decoded byte_count and sha256:<64 lowercase hex> digest, then either inline base64 bytes or (preferred for large content) a path to a file holding the exact bytes — write it under /tmp or /home/sworn first; detail may be empty.",
			InputSchema: json.RawMessage(swornSubmitInputSchema),
		},
	)
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
	if session.calls > MaxSessionToolCalls {
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
		var exitCode int
		body, exitCode, err = session.bash(ctx, call.Arguments)
		if err == nil && exitCode != 0 {
			return bashFailureResult(result, body, exitCode)
		}
	case "sworn_submit":
		body, err = session.submit(ctx, call.Arguments)
	case "sworn_yield":
		body, err = session.yieldTurn(call.Arguments)
	default:
		err = fail("TOOL_NOT_ALLOWED")
	}
	if err != nil {
		result.Content = toolErrorContent(err)
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

// toolErrorContent renders a failed tool call's content for the model: the
// stable code, plus the failing call's own bounded Detail envelope when its
// raise site set one (PROCESS_START_FAILED's six Bash-tool sandbox start
// sites, and every submit-path refusal in the submission-refusal-detail
// family), in the same "key=value" suffix idiom bashFailureResult uses for
// exit_code. Every other tool error keeps the bare code unchanged.
func toolErrorContent(err error) []byte {
	code := contractCode(err)
	var contractErr *ContractError
	if errors.As(err, &contractErr) && contractErr.Detail != "" {
		return []byte("error:" + code + " detail=" + contractErr.Detail)
	}
	return []byte("error:" + code)
}

// bashFailureResult carries a completed-but-failing command's exit code and
// captured output back to the model as a failed tool result. Discarding them
// (the pre-F9 behavior) left the worker blind to why its own command failed.
func bashFailureResult(
	result providerToolResult,
	body []byte,
	exitCode int,
) providerToolResult {
	header := []byte("error:PROCESS_FAILED exit_code=" + itoa(exitCode) + "\n")
	body = bytes.ToValidUTF8(body, []byte("�"))
	if len(header)+len(body) > MaxToolResultBytes {
		body = bytes.ToValidUTF8(body[:MaxToolResultBytes-len(header)], nil)
	}
	result.Content = append(header, body...)
	result.Failed = true
	return result
}

func (session *toolSession) read(arguments []byte) ([]byte, error) {
	value, err := decodeStrict(arguments, MaxToolArgumentBytes)
	if err != nil {
		return nil, fail("INVALID_TOOL_ARGUMENT")
	}
	root, err := closedObject(value, nil, []string{"path", "offset", "limit", "paths"})
	if err != nil {
		return nil, fail("INVALID_TOOL_ARGUMENT")
	}
	pathValue, hasPath := root["path"]
	pathsValue, hasPaths := root["paths"]
	_, hasOffset := root["offset"]
	_, hasLimit := root["limit"]
	if hasPath == hasPaths || (hasPaths && (hasOffset || hasLimit)) {
		return nil, fail("INVALID_TOOL_ARGUMENT")
	}
	if hasPaths {
		paths, ok := pathsValue.([]any)
		if !ok {
			return nil, fail("INVALID_TOOL_ARGUMENT")
		}
		return session.readBatch(paths)
	}
	guest, ok := pathValue.(string)
	if !ok {
		return nil, fail("INVALID_TOOL_ARGUMENT")
	}
	target, base, _, err := session.resolve(guest, false, false)
	if err != nil {
		return nil, err
	}
	body, err := readToolPath(base, target)
	if err != nil {
		return nil, err
	}
	if !hasOffset && !hasLimit {
		return body, nil
	}
	offset, err := toolIntArgument(root, "offset", 1, 1)
	if err != nil {
		return nil, err
	}
	limit, err := toolIntArgument(root, "limit", -1, 1)
	if err != nil {
		return nil, err
	}
	window, remaining := windowLines(body, offset, limit)
	if remaining > 0 {
		window = append(window, []byte("\n[truncated: "+itoa(remaining)+" more lines]")...)
	}
	return window, nil
}

// toolIntArgument reads an optional non-negative integer tool argument,
// returning fallback when absent. minimum bounds a present value.
func toolIntArgument(
	root map[string]any,
	key string,
	fallback, minimum int64,
) (int64, error) {
	raw, present := root[key]
	if !present {
		return fallback, nil
	}
	number, ok := raw.(json.Number)
	if !ok {
		return 0, fail("INVALID_TOOL_ARGUMENT")
	}
	value, err := number.Int64()
	if err != nil || value < minimum {
		return 0, fail("INVALID_TOOL_ARGUMENT")
	}
	return value, nil
}

// windowLines returns the 1-based offset/limit line window of an
// already-read, already budget-checked body, plus the count of further
// lines the window left unread. A negative limit means unbounded.
func windowLines(body []byte, offset, limit int64) ([]byte, int) {
	lines := bytes.Split(body, []byte("\n"))
	start := offset - 1
	if start < 0 {
		start = 0
	}
	if start > int64(len(lines)) {
		start = int64(len(lines))
	}
	selected := lines[start:]
	remaining := 0
	if limit >= 0 && int64(len(selected)) > limit {
		remaining = len(selected) - int(limit)
		selected = selected[:limit]
	}
	return bytes.Join(selected, []byte("\n")), remaining
}

// readBatch resolves and reads each path exactly as a single Read, applying
// per-file containment and the same aggregate byte budget, and states a
// truncation marker instead of failing outright when the budget is crossed
// partway through the batch.
func (session *toolSession) readBatch(paths []any) ([]byte, error) {
	if len(paths) == 0 || len(paths) > MaxToolReadBatchPaths {
		return nil, fail("INVALID_TOOL_ARGUMENT")
	}
	var result []byte
	for index, raw := range paths {
		guest, ok := raw.(string)
		if !ok {
			return nil, fail("INVALID_TOOL_ARGUMENT")
		}
		target, base, display, err := session.resolve(guest, false, false)
		if err != nil {
			return nil, err
		}
		body, err := readToolPath(base, target)
		if err != nil {
			return nil, err
		}
		var entry []byte
		if index > 0 {
			entry = append(entry, '\n')
		}
		entry = append(entry, []byte("==> "+display+" <==\n")...)
		entry = append(entry, body...)
		if len(result)+len(entry) > MaxToolResultBytes {
			marker := []byte("\n[truncated: " + itoa(len(paths)-index) + " more path(s) not read]")
			if len(result)+len(marker) <= MaxToolResultBytes {
				result = append(result, marker...)
			}
			return result, nil
		}
		result = append(result, entry...)
	}
	return result, nil
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

func (session *toolSession) bash(ctx context.Context, arguments []byte) ([]byte, int, error) {
	value, err := decodeStrict(arguments, MaxToolArgumentBytes)
	if err != nil {
		return nil, 0, fail("INVALID_TOOL_ARGUMENT")
	}
	root, err := closedObject(value, nil, []string{"script", "command"})
	if err != nil {
		return nil, 0, fail("INVALID_TOOL_ARGUMENT")
	}
	scriptValue, hasScript := root["script"]
	commandValue, hasCommand := root["command"]
	if hasScript == hasCommand {
		return nil, 0, fail("INVALID_TOOL_ARGUMENT")
	}
	raw := scriptValue
	if hasCommand {
		raw = commandValue
	}
	script, ok := raw.(string)
	if !ok || script == "" || len(script) > MaxBashScriptBytes ||
		!utf8.ValidString(script) {
		return nil, 0, fail("INVALID_TOOL_ARGUMENT")
	}
	return runToolBash(
		ctx,
		session.invocation,
		session.projection.Root(),
		session.scratch,
		script,
	)
}

func (session *toolSession) submit(
	ctx context.Context,
	arguments []byte,
) ([]byte, error) {
	value, err := decodeStrict(arguments, MaxToolArgumentBytes)
	if err != nil {
		return nil, session.rejectSubmission(ctx, submitArgumentsDecodeDetail(err))
	}
	root, err := closedObject(value, []string{"submission"}, nil)
	if err != nil {
		return nil, session.rejectSubmission(ctx, submitRootObjectDetail(err))
	}
	if err := session.materializeExactBytesPaths(root["submission"]); err != nil {
		return nil, session.rejectSubmission(ctx, err)
	}
	submission, err := decodeToolSubmission(root["submission"])
	if err != nil {
		return nil, session.rejectSubmission(ctx, err)
	}
	if submission.Responsibility == PlannerProposal && submission.Plan != nil {
		planBody, _ := base64.StdEncoding.Strict().DecodeString(submission.Plan.Bytes)
		if plan, parseErr := baton.ParsePlan(planBody); parseErr == nil {
			if lintErr := baton.ValidatePlanScopeLintFS(os.DirFS(session.invocation.HostWorkspace), plan); lintErr != nil {
				code := baton.ErrorCode(lintErr)
				if code == "" {
					code = "UNDER_DERIVED_SCOPE"
				}
				return nil, session.rejectSubmission(ctx, submitPlanScopeLintError(code, lintErr))
			}
		}
	}
	body, err := EncodeSubmission(submission)
	if err != nil {
		return nil, session.rejectSubmission(ctx, submitEncodeDetail(err))
	}
	if err := session.invocation.Permission.validate(submission); err != nil {
		return nil, session.rejectSubmission(ctx, err)
	}
	// Planner plan bytes may leave the driver only from the answer-resume
	// shape. A fresh or nudge-shaped invocation must first yield its summary;
	// rejectSubmission keeps this session alive and accounts one bounded
	// correction without sealing a handoff or consuming a dispatch try.
	if submission.Responsibility == PlannerProposal && submission.Plan != nil &&
		(session.invocation.recoverableInput == nil ||
			session.invocation.recoverableInput.Kind != RecoverableInputAnswer) {
		return nil, session.rejectSubmission(ctx, submitYieldFirstRequiredError())
	}
	seal, sealBytes, submitErr := session.server.Submit(body)
	if submitErr != nil || !seal.Accepted {
		if submitErr == nil {
			submitErr = fail("SUBMISSION_REJECTED")
		}
		session.finishSubmission(body, sealBytes, submitErr)
		return nil, submitErr
	}
	if submission.Responsibility == PlannerProposal &&
		submission.Plan != nil && session.invocation.SealedProposalHook != nil {
		planBody, decodeErr := base64.StdEncoding.Strict().DecodeString(submission.Plan.Bytes)
		if decodeErr != nil {
			decodeErr = fail("INVALID_EXACT_BYTES")
			session.finishSubmission(body, sealBytes, decodeErr)
			return nil, decodeErr
		}
		contractBodies := make(map[string][]byte, len(submission.Contracts))
		for contractPath, contract := range submission.Contracts {
			contractBody, decodeErr := base64.StdEncoding.Strict().DecodeString(contract.Bytes)
			if decodeErr != nil {
				decodeErr = fail("INVALID_EXACT_BYTES")
				session.finishSubmission(body, sealBytes, decodeErr)
				return nil, decodeErr
			}
			contractBodies[contractPath] = contractBody
		}
		if hookErr := session.invocation.SealedProposalHook(
			ctx, planBody, contractBodies,
		); hookErr != nil {
			session.finishSubmission(body, sealBytes, hookErr)
			return nil, hookErr
		}
	}
	session.finishSubmission(body, sealBytes, nil)
	return []byte("accepted"), nil
}

func (session *toolSession) rejectSubmission(
	ctx context.Context,
	err error,
) error {
	session.mu.Lock()
	if session.closed || session.terminal {
		session.mu.Unlock()
		return fail("TOOL_SESSION_CLOSED")
	}
	session.submitCorrections++
	if session.submitCorrections > MaxSubmissionCorrections {
		session.terminal = true
		session.submitErr = fail("SUBMISSION_CORRECTIONS_EXHAUSTED")
		session.mu.Unlock()
		return session.submitErr
	}
	session.mu.Unlock()
	if reserveErr := reserveRecoveryStep(
		ctx,
		session.invocation.RecoveryStepHook,
		RecoveryStepSubmissionCorrection,
	); reserveErr != nil {
		session.mu.Lock()
		session.terminal = true
		session.submitErr = reserveErr
		session.mu.Unlock()
		return reserveErr
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.closed || session.terminal {
		return fail("TOOL_SESSION_CLOSED")
	}
	return err
}

func (session *toolSession) yieldTurn(arguments []byte) ([]byte, error) {
	value, err := decodeStrict(arguments, MaxToolArgumentBytes)
	if err != nil {
		session.finishYield(nil, err)
		return nil, err
	}
	root, err := closedObject(value, []string{"yield"}, nil)
	if err != nil {
		session.finishYield(nil, err)
		return nil, err
	}
	yield, err := decodeToolYield(root["yield"])
	if err != nil {
		session.finishYield(nil, err)
		return nil, err
	}
	if yield.InvocationID != session.invocation.Request.InvocationID {
		err = fail("YIELD_BINDING_MISMATCH")
		session.finishYield(nil, err)
		return nil, err
	}
	session.finishYield(&yield, nil)
	return []byte("accepted"), nil
}

// materializeExactBytesPaths lets a submission's plan/checks member name a
// file instead of inlining base64: {byte_count, digest, path} is replaced by
// {byte_count, digest, bytes} with the bytes read by the engine itself. The
// commitment is unchanged — the declared digest and byte count still bind
// the content through validateExactBytes — but the bytes no longer travel
// through the model's own token stream, which measurably corrupts multi-KB
// base64 (F10). Paths may name the workspace, projected inputs, or the
// invocation's persistent scratch surfaces (/tmp, /home/sworn).
func (session *toolSession) materializeExactBytesPaths(value any) error {
	submission, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	for _, name := range []string{"plan", "checks"} {
		member, ok := submission[name].(map[string]any)
		if !ok {
			continue
		}
		if err := session.materializeExactBytesMember(member, name); err != nil {
			return err
		}
	}
	if contracts, ok := submission["contracts"].(map[string]any); ok {
		for _, entry := range contracts {
			member, ok := entry.(map[string]any)
			if !ok {
				return submitExactBytesPathError("INVALID_EXACT_BYTES", "contracts entry")
			}
			if err := session.materializeExactBytesMember(member, "contracts entry"); err != nil {
				return err
			}
		}
	}
	return nil
}

// materializeExactBytesMember resolves one {byte_count, digest, path} member
// in place to {byte_count, digest, bytes}, shared by the submission's plan,
// checks, and every contracts entry. field names the member for
// submit.exact_bytes_path's Detail ("plan", "checks", or "contracts entry"),
// never the guest path text itself.
func (session *toolSession) materializeExactBytesMember(member map[string]any, field string) error {
	pathValue, present := member["path"]
	if !present {
		return nil
	}
	guest, ok := pathValue.(string)
	if _, inline := member["bytes"]; !ok || inline {
		return submitExactBytesPathError("INVALID_EXACT_BYTES", field)
	}
	body, err := session.readExactBytesFile(guest, field)
	if err != nil {
		return err
	}
	member["bytes"] = base64.StdEncoding.EncodeToString(body)
	clearBytes(body)
	delete(member, "path")
	return nil
}

func (session *toolSession) readExactBytesFile(guest string, field string) ([]byte, error) {
	if validateToolText(guest) != nil || !path.IsAbs(guest) ||
		path.Clean(guest) != guest {
		return nil, submitExactBytesPathError("TOOL_PATH_INVALID", field)
	}
	var host, root string
	switch {
	case guest == "/tmp" || strings.HasPrefix(guest, "/tmp/"):
		root = filepath.Join(session.scratch, "tmp")
		host = filepath.Join(root, strings.TrimPrefix(guest, "/tmp"))
	case guest == "/home/sworn" || strings.HasPrefix(guest, "/home/sworn/"):
		root = filepath.Join(session.scratch, "home")
		host = filepath.Join(root, strings.TrimPrefix(guest, "/home/sworn"))
	default:
		target, resolvedRoot, _, err := session.resolve(guest, false, false)
		if err != nil {
			return nil, submitExactBytesPathError("TOOL_PATH_INVALID", field)
		}
		host, root = target, resolvedRoot
	}
	if root == "" || !pathBeneath(root, host) {
		return nil, submitExactBytesPathError("TOOL_PATH_INVALID", field)
	}
	// The scratch surfaces are worker-writable, so a symlink there could
	// point the engine at host content the worker itself cannot read.
	// Resolve and re-check containment before trusting the path.
	resolved, err := filepath.EvalSymlinks(host)
	if err != nil {
		return nil, submitExactBytesPathError("TOOL_PATH_INVALID", field)
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil || !pathBeneath(resolvedRoot, resolved) {
		return nil, submitExactBytesPathError("TOOL_PATH_INVALID", field)
	}
	info, err := os.Lstat(resolved)
	if err != nil || !info.Mode().IsRegular() ||
		info.Size() < 1 || info.Size() > MaxPlanBytes {
		return nil, submitExactBytesPathError("INVALID_EXACT_BYTES", field)
	}
	body, err := os.ReadFile(resolved)
	if err != nil || int64(len(body)) != info.Size() {
		return nil, submitExactBytesPathError("INVALID_EXACT_BYTES", field)
	}
	return body, nil
}

func decodeToolSubmission(value any) (Submission, error) {
	root, err := decodeSubmissionObject(
		value,
		[]string{
			"schema_version", "invocation_id", "responsibility", "summary",
			"detail",
		},
		[]string{"plan", "checks", "decision", "contracts"},
		"submission",
	)
	if err != nil {
		return Submission{}, err
	}
	for _, name := range []string{"plan", "checks"} {
		if root[name] == nil {
			continue
		}
		if _, err := decodeSubmissionObject(
			root[name],
			[]string{"byte_count", "digest", "bytes"},
			nil,
			name,
		); err != nil {
			return Submission{}, err
		}
	}
	if root["contracts"] != nil {
		contracts, ok := root["contracts"].(map[string]any)
		if !ok {
			return Submission{}, submitDecodeError("INVALID_FIELD", "contracts")
		}
		for _, member := range contracts {
			if _, err := decodeSubmissionObject(
				member,
				[]string{"byte_count", "digest", "bytes"},
				nil,
				"contracts entry",
			); err != nil {
				return Submission{}, err
			}
		}
	}
	if root["decision"] != nil {
		if _, err := decodeSubmissionObject(
			root["decision"],
			[]string{"outcome"},
			nil,
			"decision",
		); err != nil {
			return Submission{}, err
		}
	}
	body, err := canonicalJSON(root)
	if err != nil {
		return Submission{}, submitDecodeError("INVALID_JSON", "")
	}
	defer clearBytes(body)
	var submission Submission
	if json.Unmarshal(body, &submission) != nil {
		return Submission{}, submitDecodeError("INVALID_SUBMISSION", "")
	}
	// Structurally valid fields outside this responsibility carry no authority.
	// Drop that model noise before the strict, permission-bound validation.
	switch submission.Responsibility {
	case PlannerProposal:
		submission.Checks, submission.Decision = nil, nil
	case ImplementerDesign:
		submission.Plan, submission.Checks, submission.Decision = nil, nil, nil
		submission.Contracts = nil
	case ImplementerImplementation:
		submission.Plan, submission.Decision = nil, nil
		submission.Contracts = nil
	case CaptainReview, CaptainPlanReview:
		submission.Plan, submission.Checks = nil, nil
		submission.Contracts = nil
	case WorkVerification, AssemblyVerification:
		submission.Plan = nil
		submission.Contracts = nil
	}
	if declared, bound := submissionDeclaresProbe(submission.Summary); declared {
		return Submission{}, submissionProbeError("summary", bound)
	}
	if declared, bound := submissionDeclaresProbe(submission.Detail); declared {
		return Submission{}, submissionProbeError("detail", bound)
	}
	if err := submissionFloorCheck(submission); err != nil {
		return Submission{}, err
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

func (session *toolSession) finishYield(value *Yield, err error) {
	session.mu.Lock()
	defer session.mu.Unlock()
	session.terminal = true
	session.submitErr = err
	if value != nil {
		copyValue := *value
		session.yield = &copyValue
	}
}

func (session *toolSession) submitted() (bool, error) {
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.terminal && session.submitErr == nil &&
		session.yield == nil && len(session.submission) != 0, session.submitErr
}

func (session *toolSession) terminated() (bool, error) {
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.terminal, session.submitErr
}

func (session *toolSession) yielded() *Yield {
	session.mu.Lock()
	defer session.mu.Unlock()
	if !session.terminal || session.submitErr != nil || session.yield == nil {
		return nil
	}
	copyValue := *session.yield
	return &copyValue
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
	redaction := session.redaction
	session.redaction = nil
	session.mu.Unlock()
	// Drain accepted tool-result events before the session tears down;
	// the bounded drain never fails or stalls delivery beyond its cap.
	session.observer.close()
	for _, secret := range redaction {
		clearBytes(secret)
	}
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
	if session.scratch != "" {
		if err := os.RemoveAll(session.scratch); err != nil && result == nil {
			result = err
		}
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
		if err := validateRepositoryPath(
			relative,
			reservedMaskNames(session.invocation),
		); err != nil {
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
