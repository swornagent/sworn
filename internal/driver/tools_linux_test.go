//go:build linux

package driver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/gitx"
)

// TestNewToolSessionRefusesUnavailableTempRoot is the A2 consumer proof for
// the invocation session: an invalid configured temp root fails session
// creation instead of silently staging scratch in the process/system temp
// directory.
func TestNewToolSessionRefusesUnavailableTempRoot(t *testing.T) {
	t.Setenv(gitx.EnvTempRoot, "relative-tmp")
	invocation, _, _ := memoryInvocationFixture(t)
	if _, err := newToolSession(invocation); err == nil {
		t.Fatal("invalid temp root silently escaped for the invocation session")
	}
}

func TestSwornSubmitToolSchemaIsCompletePortableAndClosed(t *testing.T) {
	definitions := toolDefinitions(ReadOnly)
	submit := definitions[len(definitions)-1]
	var root map[string]any
	if submit.Name != "sworn_submit" ||
		json.Unmarshal(submit.InputSchema, &root) != nil {
		t.Fatalf("invalid submission tool: %#v", submit)
	}
	object := func(value any) map[string]any {
		result, ok := value.(map[string]any)
		if !ok {
			t.Fatalf("schema value is not an object: %#v", value)
		}
		return result
	}
	submission := object(object(root["properties"])["submission"])
	properties := object(submission["properties"])
	if root["additionalProperties"] != false ||
		submission["additionalProperties"] != false ||
		len(properties) != 8 {
		t.Fatalf("submission schema is incomplete or open: %s", submit.InputSchema)
	}
	for _, name := range []string{"plan", "checks", "decision"} {
		if object(properties[name])["additionalProperties"] != false {
			t.Fatalf("%s schema is open", name)
		}
	}
	for _, keyword := range []string{"oneOf", "anyOf", "$ref", "const"} {
		if strings.Contains(string(submit.InputSchema), `"`+keyword+`"`) {
			t.Fatalf("non-portable schema keyword %s", keyword)
		}
	}
}

func TestSparseToolSubmissionNormalizesAndRemainsFailClosed(t *testing.T) {
	sparse := map[string]any{
		"schema_version": SubmissionSchemaVersion,
		"invocation_id":  "sparse-design",
		"responsibility": ImplementerDesign,
		"summary":        "Design complete.",
		"detail":         "",
	}
	submission, err := decodeToolSubmission(sparse)
	if err != nil || submission.Plan != nil ||
		submission.Checks != nil || submission.Decision != nil {
		t.Fatalf("sparse submission = %#v, error=%v", submission, err)
	}
	sparse["decision"] = map[string]any{"outcome": string(DecisionProceed)}
	withoutAuthority, err := decodeToolSubmission(sparse)
	if err != nil || withoutAuthority.Decision != nil {
		t.Fatalf("role-irrelevant field = %#v, error=%v", withoutAuthority, err)
	}
	delete(sparse, "decision")
	sparse["responsibility"] = CaptainReview
	if _, err := decodeToolSubmission(sparse); !IsCode(err, "SUBMISSION_SHAPE_MISMATCH") {
		t.Fatalf("missing required Captain decision error = %v", err)
	}
	sparse["responsibility"] = ImplementerDesign
	delete(sparse, "detail")
	if _, err := decodeToolSubmission(sparse); !IsCode(err, "MISSING_FIELD") {
		t.Fatalf("missing common field error = %v", err)
	}
}

func TestSubmissionCorrectionsAreBoundedAndYieldCannotPromoteAuthority(
	t *testing.T,
) {
	t.Parallel()
	invocation, _, _ := memoryInvocationFixture(t)
	reservations := 0
	invocation.RecoveryStepHook = func(
		_ context.Context,
		kind RecoveryStepKind,
	) error {
		if kind != RecoveryStepSubmissionCorrection {
			t.Fatalf("reservation kind = %s", kind)
		}
		reservations++
		return nil
	}
	session, err := newToolSession(invocation)
	if err != nil {
		t.Fatal(err)
	}
	for correction := 1; correction <= MaxSubmissionCorrections; correction++ {
		result := executeToolJSON(
			t,
			session,
			"malformed-"+itoa(correction),
			"sworn_submit",
			map[string]any{"submission": map[string]any{}},
		)
		terminated, terminalErr := session.terminated()
		if !result.Failed || terminated || terminalErr != nil {
			t.Fatalf(
				"correction %d = %#v, terminated=%v, error=%v",
				correction,
				result,
				terminated,
				terminalErr,
			)
		}
	}
	exhausted := executeToolJSON(
		t,
		session,
		"malformed-exhausted",
		"sworn_submit",
		map[string]any{"submission": map[string]any{}},
	)
	terminated, terminalErr := session.terminated()
	if !exhausted.Failed || !terminated ||
		!IsCode(terminalErr, "SUBMISSION_CORRECTIONS_EXHAUSTED") ||
		session.handoff() != nil || session.yielded() != nil ||
		reservations != MaxSubmissionCorrections {
		t.Fatalf(
			"exhausted = %#v, terminated=%v, error=%v",
			exhausted,
			terminated,
			terminalErr,
		)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}

	invocation, _, _ = memoryInvocationFixture(t)
	reservations = 0
	invocation.RecoveryStepHook = func(
		_ context.Context,
		kind RecoveryStepKind,
	) error {
		if kind != RecoveryStepSubmissionCorrection {
			t.Fatalf("reservation kind = %s", kind)
		}
		reservations++
		return nil
	}
	session, err = newToolSession(invocation)
	if err != nil {
		t.Fatal(err)
	}
	for correction := 1; correction <= MaxSubmissionCorrections; correction++ {
		result := executeToolJSON(
			t,
			session,
			"correctable-"+itoa(correction),
			"sworn_submit",
			map[string]any{"submission": map[string]any{}},
		)
		if !result.Failed {
			t.Fatalf("correction %d accepted", correction)
		}
	}
	session.invocation.recoverableInput = &RecoverableTurnInput{
		SchemaVersion: RecoverableTurnInputSchemaVersion,
		Kind:          RecoverableInputAnswer,
		Answer:        "Continue with the approved planner turn.",
	}
	valid := executeToolJSON(
		t,
		session,
		"valid-after-corrections",
		"sworn_submit",
		map[string]any{"submission": submissionFixture(
			t,
			invocation.Request.InvocationID,
			PlannerProposal,
			"",
		)},
	)
	submitted, submitErr := session.submitted()
	if valid.Failed || !submitted || submitErr != nil ||
		session.handoff() == nil || session.yielded() != nil ||
		reservations != MaxSubmissionCorrections {
		t.Fatalf(
			"valid = %#v, submitted=%v, error=%v",
			valid,
			submitted,
			submitErr,
		)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}

	invocation, _, _ = memoryInvocationFixture(t)
	session, err = newToolSession(invocation)
	if err != nil {
		t.Fatal(err)
	}
	yielded := executeToolJSON(
		t,
		session,
		"yield",
		"sworn_yield",
		map[string]any{"yield": Yield{
			SchemaVersion: YieldSchemaVersion,
			InvocationID:  invocation.Request.InvocationID,
			Kind:          YieldQuestion,
			Message:       "Which admitted base should I use?",
		}},
	)
	submitted, submitErr = session.submitted()
	terminated, terminalErr = session.terminated()
	if yielded.Failed || !terminated || terminalErr != nil ||
		submitted || submitErr != nil ||
		session.handoff() != nil || session.yielded() == nil {
		t.Fatalf(
			"yield = %#v, terminated=%v, submitted=%v, errors=%v/%v",
			yielded,
			terminated,
			submitted,
			terminalErr,
			submitErr,
		)
	}
	afterYield := executeToolJSON(
		t,
		session,
		"submit-after-yield",
		"sworn_submit",
		map[string]any{"submission": submissionFixture(
			t,
			invocation.Request.InvocationID,
			PlannerProposal,
			"",
		)},
	)
	if !afterYield.Failed || session.handoff() != nil {
		t.Fatalf("submission promoted after yield: %#v", afterYield)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestModelPromptExplainsProjectedInputsAndExactSubmissionBinding(t *testing.T) {
	invocation, _, _ := memoryInvocationFixture(t)
	request, err := NewRequest(
		invocation.Request.InvocationID,
		RoleImplementer,
		invocation.Selected.Profile.Key,
		invocation.Selected.Model,
		invocation.Request.Workspace,
		[]Input{{
			Name:   "certification",
			Path:   "certification/request.json",
			Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		}},
		true,
		invocation.Request.Limits,
	)
	if err != nil {
		t.Fatal(err)
	}
	permission, err := NewSubmissionPermission(
		request,
		invocation.Selected,
		ContainmentReadWrite,
		ImplementerDesign,
	)
	if err != nil {
		t.Fatal(err)
	}
	invocation.Request, invocation.Permission = request, permission
	body, err := modelPrompt(invocation)
	if err != nil {
		t.Fatal(err)
	}
	var prompt struct {
		InvocationID   string         `json:"invocation_id"`
		Responsibility Responsibility `json:"responsibility"`
		ResultFields   []string       `json:"result_fields"`
		Instruction    string         `json:"instruction"`
	}
	if json.Unmarshal(body, &prompt) != nil ||
		prompt.InvocationID != request.InvocationID ||
		prompt.Responsibility != ImplementerDesign ||
		len(prompt.ResultFields) != 2 ||
		prompt.ResultFields[0] != "summary" ||
		prompt.ResultFields[1] != "detail" ||
		!strings.Contains(prompt.Instruction, "/sworn/inputs/") ||
		!strings.Contains(prompt.Instruction, "input's path") ||
		!strings.Contains(prompt.Instruction, "exact invocation_id and responsibility") ||
		!strings.Contains(prompt.Instruction, "exactly one terminal") {
		t.Fatalf("model prompt = %s", body)
	}
}

func TestSubmissionResultFieldsMatchResponsibility(t *testing.T) {
	for responsibility, want := range map[Responsibility]string{
		PlannerProposal:           "summary,detail,plan",
		ImplementerDesign:         "summary,detail",
		ImplementerImplementation: "summary,detail,checks",
		CaptainReview:             "summary,detail,decision",
		WorkVerification:          "summary,detail,checks,decision",
		AssemblyVerification:      "summary,detail,checks,decision",
	} {
		if got := strings.Join(submissionResultFields(responsibility), ","); got != want {
			t.Fatalf("%s result fields = %s, want %s", responsibility, got, want)
		}
	}
}

func TestCommonToolsAreDescriptorRootedAccessBoundedAndClosed(t *testing.T) {
	invocation, _, _ := memoryInvocationFixture(t)
	if err := os.MkdirAll(filepath.Join(invocation.HostWorkspace, "dir"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(invocation.HostWorkspace, "dir", "alpha.txt"),
		[]byte("alpha\nneedle\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	session, err := newToolSession(invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	read := executeToolJSON(t, session, "read-1", "Read", map[string]any{
		"path": GuestWorkspacePath + "/dir/alpha.txt",
	})
	if read.Failed || string(read.Content) != "alpha\nneedle\n" {
		t.Fatalf("Read = %#v", read)
	}
	write := executeToolJSON(t, session, "write-1", "Write", map[string]any{
		"path": GuestWorkspacePath + "/new/nested.txt", "content": "first\n",
	})
	if write.Failed {
		t.Fatalf("Write = %#v", write)
	}
	edit := executeToolJSON(t, session, "edit-1", "Edit", map[string]any{
		"path":       GuestWorkspacePath + "/new/nested.txt",
		"old_string": "first", "new_string": "second",
	})
	if edit.Failed {
		t.Fatalf("Edit = %#v", edit)
	}
	glob := executeToolJSON(t, session, "glob-1", "Glob", map[string]any{
		"path": GuestWorkspacePath, "pattern": "dir/*.txt",
	})
	if glob.Failed || string(glob.Content) != GuestWorkspacePath+"/dir/alpha.txt" {
		t.Fatalf("Glob = %#v", glob)
	}
	grep := executeToolJSON(t, session, "grep-1", "Grep", map[string]any{
		"path": GuestWorkspacePath, "pattern": "needle",
	})
	if grep.Failed ||
		string(grep.Content) != GuestWorkspacePath+"/dir/alpha.txt:2:needle" {
		t.Fatalf("Grep = %#v", grep)
	}
	if err := os.Symlink(outside, filepath.Join(invocation.HostWorkspace, "escape")); err != nil {
		t.Fatal(err)
	}
	for index, call := range []providerToolCall{
		{ID: "unsafe-symlink", Name: "Read", Arguments: json.RawMessage(
			`{"path":"/workspace/escape"}`,
		)},
		{ID: "unsafe-traversal", Name: "Read", Arguments: json.RawMessage(
			`{"path":"/workspace/../outside"}`,
		)},
		{ID: "unsafe-authority", Name: "Read", Arguments: json.RawMessage(
			`{"path":"/workspace/.git/config"}`,
		)},
	} {
		result := session.execute(context.Background(), call)
		if !result.Failed {
			t.Fatalf("unsafe call %d accepted: %#v", index, result)
		}
	}
}

func TestToolBashIsNetworkCredentialAndAuthorityBlind(t *testing.T) {
	requireTrustedContainment(t)
	invocation, _, _ := memoryInvocationFixture(t)
	if err := os.MkdirAll(filepath.Join(invocation.HostWorkspace, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(invocation.HostWorkspace, ".git", "secret"),
		[]byte("authority-canary"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AWS_SECRET_ACCESS_KEY", "host-credential-canary")
	session, err := newToolSession(invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	result := executeToolJSON(t, session, "bash-1", "Bash", map[string]any{
		"script": `test ! -e .git/secret
test ! -e /sworn/inputs/missing
if env | grep -E 'AWS_|SWORN_MCP|host-credential-canary' >/dev/null; then exit 9; fi
if /usr/bin/bash -c 'exec 3<>/dev/tcp/1.1.1.1/53' 2>/dev/null; then exit 10; fi
printf 'isolated'`,
	})
	if result.Failed || string(result.Content) != "isolated" {
		t.Fatalf("Bash = %#v", result)
	}
}

func TestToolBashScratchPersistsAcrossCommandsWithinInvocationOnly(
	t *testing.T,
) {
	requireTrustedContainment(t)
	invocation, _, _ := memoryInvocationFixture(t)
	session, err := newToolSession(invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	first := executeToolJSON(t, session, "bash-stash", "Bash", map[string]any{
		"script": `printf cache > /tmp/cache-canary
mkdir -p "$HOME/.cache" && printf home > "$HOME/.cache/home-canary"`,
	})
	if first.Failed {
		t.Fatalf("stash = %#v", first)
	}
	second := executeToolJSON(t, session, "bash-reuse", "Bash", map[string]any{
		"script": `cat /tmp/cache-canary "$HOME/.cache/home-canary"`,
	})
	if second.Failed || string(second.Content) != "cachehome" {
		t.Fatalf("scratch did not persist across commands = %#v", second)
	}
	scratch := session.scratch
	if scratch == "" {
		t.Fatal("session has no scratch directory")
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(scratch); !os.IsNotExist(err) {
		t.Fatalf("scratch survived Close: %v", err)
	}
	fresh, err := newToolSession(invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	leak := executeToolJSON(t, fresh, "bash-leak", "Bash", map[string]any{
		"script": `test ! -e /tmp/cache-canary && test ! -e "$HOME/.cache/home-canary" && printf clean`,
	})
	if leak.Failed || string(leak.Content) != "clean" {
		t.Fatalf("scratch leaked into a fresh invocation = %#v", leak)
	}
}

func TestSwornSubmitAcceptsExactBytesByScratchPath(t *testing.T) {
	invocation, _, _ := memoryInvocationFixture(t)
	invocation.recoverableInput = &RecoverableTurnInput{
		SchemaVersion: RecoverableTurnInputSchemaVersion,
		Kind:          RecoverableInputAnswer,
		Answer:        "Continue with the approved planner turn.",
	}
	session, err := newToolSession(invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	plan := validPlanBytes()
	if err := os.WriteFile(
		filepath.Join(session.scratch, "tmp", "plan.md"), plan, 0o600,
	); err != nil {
		t.Fatal(err)
	}
	result := executeToolJSON(t, session, "submit-by-path", "sworn_submit",
		map[string]any{"submission": map[string]any{
			"schema_version": SubmissionSchemaVersion,
			"invocation_id":  invocation.Request.InvocationID,
			"responsibility": string(PlannerProposal),
			"summary":        "Compact responsibility summary.",
			"detail":         "",
			"plan": map[string]any{
				"byte_count": len(plan),
				"digest":     Digest(plan),
				"path":       "/tmp/plan.md",
			},
		}},
	)
	submitted, submitErr := session.submitted()
	if result.Failed || !submitted || submitErr != nil ||
		session.handoff() == nil {
		t.Fatalf(
			"path submission = %#v, submitted=%v, error=%v",
			result, submitted, submitErr,
		)
	}
}

func TestSwornSubmitPathRefusesSymlinkEscapeFromScratch(t *testing.T) {
	invocation, _, _ := memoryInvocationFixture(t)
	invocation.RecoveryStepHook = func(
		context.Context, RecoveryStepKind,
	) error {
		return nil
	}
	session, err := newToolSession(invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if err := os.Symlink(
		"/etc/hostname", filepath.Join(session.scratch, "tmp", "escape"),
	); err != nil {
		t.Fatal(err)
	}
	plan := validPlanBytes()
	result := executeToolJSON(t, session, "submit-escape", "sworn_submit",
		map[string]any{"submission": map[string]any{
			"schema_version": SubmissionSchemaVersion,
			"invocation_id":  invocation.Request.InvocationID,
			"responsibility": string(PlannerProposal),
			"summary":        "Compact responsibility summary.",
			"detail":         "",
			"plan": map[string]any{
				"byte_count": len(plan),
				"digest":     Digest(plan),
				"path":       "/tmp/escape",
			},
		}},
	)
	if !result.Failed ||
		!strings.Contains(string(result.Content), "TOOL_PATH_INVALID") {
		t.Fatalf("symlink escape submission = %#v", result)
	}
}

func TestToolBashNonZeroExitReturnsOutputAndExitCode(t *testing.T) {
	requireTrustedContainment(t)
	invocation, _, _ := memoryInvocationFixture(t)
	session, err := newToolSession(invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	result := executeToolJSON(t, session, "bash-exit", "Bash", map[string]any{
		"script": `printf 'stdout-evidence\n'
printf 'stderr-evidence\n' >&2
exit 42`,
	})
	if !result.Failed {
		t.Fatalf("non-zero exit result = %#v", result)
	}
	content := string(result.Content)
	if !strings.HasPrefix(content, "error:PROCESS_FAILED exit_code=42\n") {
		t.Fatalf("non-zero exit content = %q", content)
	}
	if !strings.Contains(content, "stdout-evidence") ||
		!strings.Contains(content, "stderr-evidence") {
		t.Fatalf("non-zero exit output discarded = %q", content)
	}
}

func TestReadOnlyToolSurfaceOmitsAndRejectsMutation(t *testing.T) {
	invocation, _, _ := memoryInvocationFixture(t)
	request, err := NewRequest(
		"readonly-tool-invocation",
		RoleVerifier,
		invocation.Selected.Profile.Key,
		invocation.Selected.Model,
		Workspace{Path: GuestWorkspacePath, Access: ReadOnly},
		nil,
		true,
		Limits{TimeoutMillis: 5_000, OutputBytes: 65_536},
	)
	if err != nil {
		t.Fatal(err)
	}
	permission, err := NewSubmissionPermission(
		request,
		invocation.Selected,
		ContainmentReadOnly,
		WorkVerification,
	)
	if err != nil {
		t.Fatal(err)
	}
	invocation.Request = request
	invocation.Permission = permission
	session, err := newToolSession(invocation)
	if err != nil {
		t.Fatal(err)
	}
	definitions := toolDefinitions(ReadOnly)
	for _, definition := range definitions {
		if definition.Name == "Write" || definition.Name == "Edit" {
			t.Fatalf("read-only definition exposed %s", definition.Name)
		}
	}
	write := executeToolJSON(t, session, "write-ro", "Write", map[string]any{
		"path": GuestWorkspacePath + "/forbidden", "content": "no",
	})
	if !write.Failed || !strings.Contains(string(write.Content), "TOOL_NOT_ALLOWED") {
		t.Fatalf("read-only Write = %#v", write)
	}
	bash := executeToolJSON(t, session, "bash-ro", "Bash", map[string]any{
		"script": `printf no > /workspace/forbidden`,
	})
	if !bash.Failed {
		t.Fatalf("read-only Bash mutation = %#v", bash)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(invocation.HostWorkspace, "forbidden")); !os.IsNotExist(err) {
		t.Fatalf("read-only mutation exists: %v", err)
	}
}

func executeToolJSON(
	t *testing.T,
	session *toolSession,
	id string,
	name string,
	arguments any,
) providerToolResult {
	t.Helper()
	body, err := json.Marshal(arguments)
	if err != nil {
		t.Fatal(err)
	}
	return session.execute(context.Background(), providerToolCall{
		ID: id, Name: name, Arguments: body,
	})
}
