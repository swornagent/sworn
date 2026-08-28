//go:build linux

package driver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

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
		len(properties) != 9 {
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

// TestPlannerPlanOnFirstTerminalIsYieldFirstAndSessionStaysOpen pins A1:
// a planner_proposal that already carries plan bytes on a first terminal is
// refused as YIELD_FIRST_REQUIRED. The session stays open, no handoff is
// sealed, the violation is one bounded correction (no try consumed), and a
// later yield in the same session still works.
func TestPlannerPlanOnFirstTerminalIsYieldFirstAndSessionStaysOpen(
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
	defer session.Close()

	refused := executeToolJSON(
		t,
		session,
		"first-terminal-plan",
		"sworn_submit",
		map[string]any{"submission": submissionFixture(
			t,
			invocation.Request.InvocationID,
			PlannerProposal,
			"",
		)},
	)
	terminated, terminalErr := session.terminated()
	submitted, submitErr := session.submitted()
	if !refused.Failed ||
		!strings.Contains(string(refused.Content), "YIELD_FIRST_REQUIRED") ||
		terminated || terminalErr != nil ||
		submitted || submitErr != nil ||
		session.handoff() != nil || session.yielded() != nil ||
		reservations != 1 {
		t.Fatalf(
			"first-terminal plan = %#v terminated=%v submitted=%v errors=%v/%v reservations=%d",
			refused,
			terminated,
			submitted,
			terminalErr,
			submitErr,
			reservations,
		)
	}

	yielded := executeToolJSON(
		t,
		session,
		"yield-after-refusal",
		"sworn_yield",
		map[string]any{"yield": Yield{
			SchemaVersion: YieldSchemaVersion,
			InvocationID:  invocation.Request.InvocationID,
			Kind:          YieldQuestion,
			Message:       "Here is the summary I must confirm before planning.",
		}},
	)
	submitted, submitErr = session.submitted()
	terminated, terminalErr = session.terminated()
	if yielded.Failed || !terminated || terminalErr != nil ||
		submitted || submitErr != nil ||
		session.handoff() != nil || session.yielded() == nil ||
		reservations != 1 {
		t.Fatalf(
			"yield after refusal = %#v terminated=%v submitted=%v errors=%v/%v",
			yielded,
			terminated,
			submitted,
			terminalErr,
			submitErr,
		)
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

func TestModelPromptCarriesRoleAssetAddendumForNonPlannerRolesAndOmitsItForPlanner(t *testing.T) {
	invocation, _, _ := memoryInvocationFixture(t)
	for _, tc := range []struct {
		role           Role
		responsibility Responsibility
		access         WorkspaceAccess
		freshContext   bool
	}{
		{RolePlanner, PlannerProposal, ReadWrite, true},
		{RoleImplementer, ImplementerImplementation, ReadWrite, true},
		{RoleCaptain, CaptainReview, ReadOnly, true},
		{RoleVerifier, WorkVerification, ReadOnly, false},
	} {
		tc := tc
		t.Run(string(tc.role), func(t *testing.T) {
			request, err := NewRequest(
				invocation.Request.InvocationID,
				tc.role,
				invocation.Selected.Profile.Key,
				invocation.Selected.Model,
				Workspace{Path: GuestWorkspacePath, Access: tc.access},
				[]Input{},
				tc.freshContext,
				invocation.Request.Limits,
			)
			if err != nil {
				t.Fatal(err)
			}
			containment := ContainmentReadWrite
			if tc.access == ReadOnly {
				containment = ContainmentReadOnly
			}
			permission, err := NewSubmissionPermission(
				request, invocation.Selected, containment, tc.responsibility,
			)
			if err != nil {
				t.Fatal(err)
			}
			scoped := invocation
			scoped.Request, scoped.Permission = request, permission
			body, err := modelPrompt(scoped)
			if err != nil {
				t.Fatal(err)
			}
			var prompt struct {
				RoleAssetAddendum *struct {
					Version string `json:"version"`
					Digest  string `json:"digest"`
					Text    string `json:"text"`
				} `json:"role_asset_addendum"`
			}
			if json.Unmarshal(body, &prompt) != nil {
				t.Fatalf("model prompt = %s", body)
			}
			if tc.role == RolePlanner {
				if prompt.RoleAssetAddendum != nil {
					t.Fatalf("planner prompt carries addendum: %s", body)
				}
				return
			}
			if prompt.RoleAssetAddendum == nil {
				t.Fatalf("%s prompt is missing addendum: %s", tc.role, body)
			}
			addendum := RoleAssetAddendum(tc.role)
			if prompt.RoleAssetAddendum.Version != addendum.Version ||
				prompt.RoleAssetAddendum.Digest != addendum.Digest ||
				prompt.RoleAssetAddendum.Text != addendum.Text {
				t.Fatalf("%s prompt addendum = %#v", tc.role, prompt.RoleAssetAddendum)
			}
		})
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

// TestBashAcceptsCommandAliasExactlyOnce is the A1 proof: command is an
// exact alias for script, exactly one of the two is required, and both or
// neither is refused with the same INVALID_TOOL_ARGUMENT a model sees today.
func TestBashAcceptsCommandAliasExactlyOnce(t *testing.T) {
	requireTrustedContainment(t)
	invocation, _, _ := memoryInvocationFixture(t)
	session, err := newToolSession(invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	scriptOnly := executeToolJSON(t, session, "bash-script", "Bash", map[string]any{
		"script": "printf script-ok",
	})
	if scriptOnly.Failed || string(scriptOnly.Content) != "script-ok" {
		t.Fatalf("script-only Bash = %#v", scriptOnly)
	}
	commandOnly := executeToolJSON(t, session, "bash-command", "Bash", map[string]any{
		"command": "printf command-ok",
	})
	if commandOnly.Failed || string(commandOnly.Content) != "command-ok" {
		t.Fatalf("command-only Bash = %#v", commandOnly)
	}
	both := executeToolJSON(t, session, "bash-both", "Bash", map[string]any{
		"script": "printf a", "command": "printf b",
	})
	if !both.Failed || !strings.Contains(string(both.Content), "INVALID_TOOL_ARGUMENT") {
		t.Fatalf("both-present Bash = %#v", both)
	}
	neither := executeToolJSON(t, session, "bash-neither", "Bash", map[string]any{})
	if !neither.Failed || !strings.Contains(string(neither.Content), "INVALID_TOOL_ARGUMENT") {
		t.Fatalf("neither-present Bash = %#v", neither)
	}
}

// TestReadWindowsOffsetAndLimit is the A2 windowing proof: a plain path-only
// call stays byte-identical, offset/limit windows a large file within the
// existing budget with a stated truncation marker when content is left
// unread, and offset/limit alongside paths (or path and paths together, or
// neither) is refused before any file is touched.
func TestReadWindowsOffsetAndLimit(t *testing.T) {
	invocation, _, _ := memoryInvocationFixture(t)
	content := "line1\nline2\nline3\nline4\nline5"
	if err := os.WriteFile(
		filepath.Join(invocation.HostWorkspace, "multi.txt"),
		[]byte(content), 0o600,
	); err != nil {
		t.Fatal(err)
	}
	session, err := newToolSession(invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	plain := executeToolJSON(t, session, "read-plain", "Read", map[string]any{
		"path": GuestWorkspacePath + "/multi.txt",
	})
	if plain.Failed || string(plain.Content) != content {
		t.Fatalf("plain Read = %#v, want byte-identical %q", plain, content)
	}

	windowed := executeToolJSON(t, session, "read-window", "Read", map[string]any{
		"path": GuestWorkspacePath + "/multi.txt", "offset": 4, "limit": 10,
	})
	if windowed.Failed || string(windowed.Content) != "line4\nline5" {
		t.Fatalf("windowed Read = %#v", windowed)
	}

	truncated := executeToolJSON(t, session, "read-truncated", "Read", map[string]any{
		"path": GuestWorkspacePath + "/multi.txt", "offset": 1, "limit": 2,
	})
	if truncated.Failed ||
		string(truncated.Content) != "line1\nline2\n[truncated: 3 more lines]" {
		t.Fatalf("truncated Read = %#v", truncated)
	}

	for name, arguments := range map[string]map[string]any{
		"path-and-paths": {
			"path":  GuestWorkspacePath + "/multi.txt",
			"paths": []string{GuestWorkspacePath + "/multi.txt"},
		},
		"offset-with-paths": {
			"paths": []string{GuestWorkspacePath + "/multi.txt"}, "offset": 1,
		},
		"neither-path-nor-paths": {"offset": 1},
	} {
		result := executeToolJSON(t, session, "read-invalid-"+name, "Read", arguments)
		if !result.Failed || !strings.Contains(string(result.Content), "INVALID_TOOL_ARGUMENT") {
			t.Fatalf("%s = %#v", name, result)
		}
	}
}

// TestReadBatchesMultiplePathsAndTruncatesAtAggregateBudget is the A2
// batching proof: several paths batch into one call with per-path headers,
// each path still subject to today's per-file resolution and containment,
// and the aggregate result stays under MaxToolResultBytes with a stated
// truncation marker rather than the outer RESOURCE_LIMIT hard-fail.
func TestReadBatchesMultiplePathsAndTruncatesAtAggregateBudget(t *testing.T) {
	invocation, _, _ := memoryInvocationFixture(t)
	if err := os.WriteFile(
		filepath.Join(invocation.HostWorkspace, "alpha.txt"),
		[]byte("alpha-body\n"), 0o600,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(invocation.HostWorkspace, "beta.txt"),
		[]byte("beta-body\n"), 0o600,
	); err != nil {
		t.Fatal(err)
	}
	session, err := newToolSession(invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	batch := executeToolJSON(t, session, "read-batch", "Read", map[string]any{
		"paths": []string{
			GuestWorkspacePath + "/alpha.txt",
			GuestWorkspacePath + "/beta.txt",
		},
	})
	if batch.Failed ||
		!strings.Contains(string(batch.Content), "==> "+GuestWorkspacePath+"/alpha.txt <==\nalpha-body") ||
		!strings.Contains(string(batch.Content), "==> "+GuestWorkspacePath+"/beta.txt <==\nbeta-body") {
		t.Fatalf("batch Read = %#v", batch)
	}

	empty := executeToolJSON(t, session, "read-batch-empty", "Read", map[string]any{
		"paths": []string{},
	})
	if !empty.Failed {
		t.Fatalf("empty paths batch accepted: %#v", empty)
	}
	tooMany := make([]string, MaxToolReadBatchPaths+1)
	for index := range tooMany {
		tooMany[index] = GuestWorkspacePath + "/alpha.txt"
	}
	over := executeToolJSON(t, session, "read-batch-over", "Read", map[string]any{
		"paths": tooMany,
	})
	if !over.Failed {
		t.Fatalf("over-cap paths batch accepted: %#v", over)
	}

	names := []string{"budget-a.txt", "budget-b.txt", "budget-c.txt"}
	markers := []string{"FILE-A", "FILE-B", "FILE-C"}
	budgetPaths := make([]string, len(names))
	for index, name := range names {
		body := markers[index] + "\n" + strings.Repeat("x", 89_990)
		if err := os.WriteFile(
			filepath.Join(invocation.HostWorkspace, name), []byte(body), 0o600,
		); err != nil {
			t.Fatal(err)
		}
		budgetPaths[index] = GuestWorkspacePath + "/" + name
	}
	budgetBatch := executeToolJSON(t, session, "read-batch-budget", "Read", map[string]any{
		"paths": budgetPaths,
	})
	if budgetBatch.Failed || len(budgetBatch.Content) > MaxToolResultBytes {
		t.Fatalf("budget-edge batch = %#v", budgetBatch)
	}
	body := string(budgetBatch.Content)
	if !strings.Contains(body, "FILE-A") || !strings.Contains(body, "FILE-B") ||
		strings.Contains(body, "FILE-C") ||
		!strings.Contains(body, "[truncated: 1 more path(s) not read]") {
		t.Fatalf("budget-edge batch did not truncate at the aggregate budget: len=%d", len(body))
	}
}

// TestBashGitMaskMaterializesAsHonestEmptyRegularFile is the A3 proof: a
// worktree-style .git pointer file (a regular file, not a directory) masks
// as a genuine empty regular file inside the Bash tool sandbox instead of
// the character device /dev/null used to produce today, while a masked
// directory (.sworn) stays the unaffected tmpfs+remount-ro branch.
func TestBashGitMaskMaterializesAsHonestEmptyRegularFile(t *testing.T) {
	requireTrustedContainment(t)
	invocation, _, _ := memoryInvocationFixture(t)
	if err := os.WriteFile(
		filepath.Join(invocation.HostWorkspace, ".git"),
		[]byte("gitdir: /somewhere/does/not/matter\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(
		filepath.Join(invocation.HostWorkspace, ".sworn"), 0o700,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(invocation.HostWorkspace, ".sworn", "authority-canary"),
		[]byte("forbidden\n"), 0o600,
	); err != nil {
		t.Fatal(err)
	}
	session, err := newToolSession(invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	result := executeToolJSON(t, session, "bash-git-mask", "Bash", map[string]any{
		"script": `test -f .git || exit 1
test ! -s .git || exit 2
test -d .sworn || exit 3
test ! -e .sworn/authority-canary || exit 4
printf ok`,
	})
	if result.Failed || string(result.Content) != "ok" {
		t.Fatalf("git mask honesty = %#v", result)
	}
}

// TestModelPromptEnvironmentFactsAreDerivedAndMatchWorkspaceAccess is the A4
// proof: the environment-facts block reads MaxToolResultBytes,
// ToolSandboxPath and the reserved mask set directly, so it can never claim
// a fact its own Bash/Read sandbox does not enforce; a read-only workspace's
// block states .git as not masked (GitReadOnly) exactly as runToolBash's own
// withoutGit branch exposes it read-only instead.
func TestModelPromptEnvironmentFactsAreDerivedAndMatchWorkspaceAccess(t *testing.T) {
	invocation, _, _ := memoryInvocationFixture(t)
	body, err := modelPrompt(invocation)
	if err != nil {
		t.Fatal(err)
	}
	var prompt struct {
		Environment EnvironmentFacts `json:"environment"`
	}
	if json.Unmarshal(body, &prompt) != nil {
		t.Fatalf("model prompt = %s", body)
	}
	want := buildEnvironmentFacts(invocation)
	if prompt.Environment.ToolResultByteBudget != MaxToolResultBytes ||
		prompt.Environment.ToolSandboxPath != ToolSandboxPath ||
		prompt.Environment.GitReadOnly != want.GitReadOnly ||
		prompt.Environment.Note == "" ||
		strings.Join(prompt.Environment.MaskedPaths, ",") != strings.Join(want.MaskedPaths, ",") {
		t.Fatalf("environment facts = %#v, want %#v", prompt.Environment, want)
	}
	for _, name := range prompt.Environment.MaskedPaths {
		if name != ".git" {
			continue
		}
		if prompt.Environment.GitReadOnly {
			t.Fatalf("git masked but GitReadOnly=true: %#v", prompt.Environment)
		}
	}

	request, err := NewRequest(
		invocation.Request.InvocationID,
		RoleVerifier,
		invocation.Selected.Profile.Key,
		invocation.Selected.Model,
		Workspace{Path: GuestWorkspacePath, Access: ReadOnly},
		nil,
		true,
		invocation.Request.Limits,
	)
	if err != nil {
		t.Fatal(err)
	}
	permission, err := NewSubmissionPermission(
		request, invocation.Selected, ContainmentReadOnly, WorkVerification,
	)
	if err != nil {
		t.Fatal(err)
	}
	readOnly := invocation
	readOnly.Request, readOnly.Permission = request, permission
	roBody, err := modelPrompt(readOnly)
	if err != nil {
		t.Fatal(err)
	}
	var roPrompt struct {
		Environment EnvironmentFacts `json:"environment"`
	}
	if json.Unmarshal(roBody, &roPrompt) != nil {
		t.Fatalf("read-only model prompt = %s", roBody)
	}
	if !roPrompt.Environment.GitReadOnly {
		t.Fatalf("read-only environment facts = %#v", roPrompt.Environment)
	}
	for _, name := range roPrompt.Environment.MaskedPaths {
		if name == ".git" {
			t.Fatalf("read-only environment facts still mask .git: %#v", roPrompt.Environment)
		}
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

// withReducedNoFileLimit lowers RLIMIT_NOFILE (process-wide) so exactly
// extra new file descriptors are admitted beyond whatever is open right
// now, restoring the original limit on cleanup. The anchor is a freshly
// opened descriptor's own number (via Fd()), not a count of entries under
// /proc/self/fd: re-reading that directory to count descriptors opens (and
// by the time it returns, closes) one of its own, which measures a moving
// target rather than a stable one. The brief sleep before opening the
// anchor lets the Go runtime's own lazy first-use descriptors (the
// netpoller epoll instance, allocated on a process's first blocking
// operation) appear before measurement, so they are never mistaken for
// budget available to the code under test - both effects were observed
// directly while calibrating this helper. Callers must not run in
// parallel with other tests: the limit is process-global for the whole
// test binary.
func withReducedNoFileLimit(t *testing.T, extra uint64) {
	t.Helper()
	time.Sleep(5 * time.Millisecond)
	anchor, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("open anchor descriptor: %v", err)
	}
	defer anchor.Close()
	var original syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &original); err != nil {
		t.Fatalf("getrlimit RLIMIT_NOFILE: %v", err)
	}
	t.Cleanup(func() {
		if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &original); err != nil {
			t.Fatalf("restore RLIMIT_NOFILE: %v", err)
		}
	})
	reduced := syscall.Rlimit{Cur: uint64(anchor.Fd()) + 1 + extra, Max: original.Max}
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &reduced); err != nil {
		t.Fatalf("setrlimit RLIMIT_NOFILE: %v", err)
	}
}

// warmUpBashSandbox runs one trivial Bash call under the unrestricted
// limit so trustedBubblewrap's sync.Once capability probe (process_linux.go)
// has already fired before a test tightens RLIMIT_NOFILE: the probe spawns
// its own bwrap subprocess and would otherwise compete with the test's own
// tight fd budget and fail at the wrong site.
func warmUpBashSandbox(t *testing.T, session *toolSession) {
	t.Helper()
	result := executeToolJSON(t, session, "warmup", "Bash", map[string]any{
		"script": "true",
	})
	if result.Failed {
		t.Fatalf("warm-up Bash call failed: %#v", result)
	}
}

func sandboxStartDetailFromResult(t *testing.T, result providerToolResult) sandboxStartDetail {
	t.Helper()
	if !result.Failed {
		t.Fatalf("expected a failed result, got %#v", result)
	}
	content := string(result.Content)
	prefix := "error:PROCESS_START_FAILED detail="
	if !strings.HasPrefix(content, prefix) {
		t.Fatalf("content = %q, want prefix %q", content, prefix)
	}
	var envelope sandboxStartDetail
	if err := json.Unmarshal([]byte(strings.TrimPrefix(content, prefix)), &envelope); err != nil {
		t.Fatalf("content = %q: %v", content, err)
	}
	return envelope
}

// TestToolBashNamesMaskDevnullOpenSite is the A1/A2/A3 proof for
// tools_linux.go:505: a masked reserved name that exists as a regular file
// (the same fixture TestBashGitMaskMaterializesAsHonestEmptyRegularFile
// uses to reach the mask-open branch at all) combined with a RLIMIT_NOFILE
// budget that admits exactly the two directory pins runToolBash opens
// first (workspace, inputs) fails the mask's os.Open(os.DevNull) with a
// real kernel EMFILE, named on the tool result surface the worker sees.
func TestToolBashNamesMaskDevnullOpenSite(t *testing.T) {
	requireTrustedContainment(t)
	invocation, _, _ := memoryInvocationFixture(t)
	if err := os.WriteFile(
		filepath.Join(invocation.HostWorkspace, ".git"), []byte("x"), 0o600,
	); err != nil {
		t.Fatal(err)
	}
	session, err := newToolSession(invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	warmUpBashSandbox(t, session)
	withReducedNoFileLimit(t, 2)
	result := executeToolJSON(t, session, "bash-mask-devnull", "Bash", map[string]any{
		"script": "true",
	})
	envelope := sandboxStartDetailFromResult(t, result)
	if envelope.Check != "sandbox_start.mask_devnull_open" {
		t.Fatalf("check = %q, content = %q", envelope.Check, result.Content)
	}
	if envelope.Cause == "" {
		t.Fatalf("no kernel cause carried: %q", result.Content)
	}
}

// TestToolBashNamesStatusPipeCreateSite is the A1/A2/A3 proof for
// tools_linux.go:544: with no masked regular file in the workspace (the
// mask loop makes no extra opens), a RLIMIT_NOFILE budget admitting exactly
// the two directory pins fails the status-pipe os.Pipe with a real kernel
// EMFILE, named on the tool result surface the worker sees.
func TestToolBashNamesStatusPipeCreateSite(t *testing.T) {
	requireTrustedContainment(t)
	invocation, _, _ := memoryInvocationFixture(t)
	session, err := newToolSession(invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	warmUpBashSandbox(t, session)
	withReducedNoFileLimit(t, 2)
	result := executeToolJSON(t, session, "bash-status-pipe", "Bash", map[string]any{
		"script": "true",
	})
	envelope := sandboxStartDetailFromResult(t, result)
	if envelope.Check != "sandbox_start.status_pipe_create" {
		t.Fatalf("check = %q, content = %q", envelope.Check, result.Content)
	}
	if envelope.Cause == "" {
		t.Fatalf("no kernel cause carried: %q", result.Content)
	}
}

// TestToolBashNamesBwrapExecStartSite is the A1/A2/A3 proof for
// tools_linux.go:559: a RLIMIT_NOFILE budget admitting the two directory
// pins plus the status pipe pair, but nothing more, fails command.Start
// itself (its own internal stdout/stderr pipe setup) with a real kernel
// EMFILE, named on the tool result surface the worker sees.
func TestToolBashNamesBwrapExecStartSite(t *testing.T) {
	requireTrustedContainment(t)
	invocation, _, _ := memoryInvocationFixture(t)
	session, err := newToolSession(invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	warmUpBashSandbox(t, session)
	withReducedNoFileLimit(t, 4)
	result := executeToolJSON(t, session, "bash-exec-start", "Bash", map[string]any{
		"script": "true",
	})
	envelope := sandboxStartDetailFromResult(t, result)
	if envelope.Check != "sandbox_start.bwrap_exec_start" {
		t.Fatalf("check = %q, content = %q", envelope.Check, result.Content)
	}
}

// TestToolBashNamesProcessGroupHandshakeReadSite is the A1 proof for
// tools_linux.go:570: a per-call context that expires a few milliseconds
// after the sandboxed process starts - long enough for command.Start to
// succeed, far short of readSandboxProcessGroup's processTerminationGrace
// window - kills bwrap before it reports its child PID, so the handshake
// read fails distinctly from a failed exec (site 5). Unlike every other
// site, no cause is asserted: readSandboxProcessGroup's own raise sites
// live in aws_chain_linux.go, out of this slice's scope, and already
// flatten to a bare PROCESS_START_FAILED before this call site ever sees
// the underlying JSON/syscall error (see the comment at the call site).
func TestToolBashNamesProcessGroupHandshakeReadSite(t *testing.T) {
	requireTrustedContainment(t)
	invocation, _, _ := memoryInvocationFixture(t)
	session, err := newToolSession(invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	warmUpBashSandbox(t, session)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	body, err := json.Marshal(map[string]any{"script": "sleep 1"})
	if err != nil {
		t.Fatal(err)
	}
	result := session.execute(ctx, providerToolCall{
		ID: "bash-handshake", Name: "Bash", Arguments: body,
	})
	envelope := sandboxStartDetailFromResult(t, result)
	if envelope.Check != "sandbox_start.process_group_handshake_read" {
		t.Fatalf("check = %q, content = %q", envelope.Check, result.Content)
	}
}
