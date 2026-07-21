//go:build linux

package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/app"
	configservice "github.com/swornagent/sworn/internal/config"
	"github.com/swornagent/sworn/internal/engine"
	"github.com/swornagent/sworn/internal/policy"
	"github.com/swornagent/sworn/internal/protocol"
	"github.com/swornagent/sworn/internal/repo"
	"github.com/swornagent/sworn/internal/store"
	"github.com/swornagent/sworn/internal/workspace"
)

const (
	requireLiveCodexDeliveryEnvironment = "SWORN_REQUIRE_CODEX_DELIVERY"
	liveCodexBinaryEnvironment          = "SWORN_CODEX_BINARY"
	liveCodexAuthFileEnvironment        = "SWORN_CODEX_AUTH_FILE"
	liveCodexModelEnvironment           = "SWORN_CODEX_MODEL"
	liveDeliveryRunID                   = "run-live-codex"
	liveDeliveryWorkID                  = "write-proof"
)

type liveDeliveryFixture struct {
	configPath      string
	controlDatabase string
	repositoryRoot  string
	baseCommit      string
	authFile        string
}

type liveAuthoritySource struct {
	Version       int64             `json:"version"`
	SourceID      string            `json:"source_id"`
	Status        string            `json:"status"`
	Repository    string            `json:"repository"`
	TargetRef     string            `json:"target_ref"`
	MaximumGrants []json.RawMessage `json:"maximum_grants"`
	AuthorizerRef string            `json:"authorizer_ref"`
	ValidFrom     string            `json:"valid_from"`
	ValidUntil    string            `json:"valid_until"`
}

type liveAuthorityProof struct {
	SchemaVersion   string `json:"schema_version"`
	SourceRef       string `json:"source_ref"`
	SourceDigest    string `json:"source_digest"`
	SourceVersion   int64  `json:"source_version"`
	PlanDigest      string `json:"plan_digest"`
	AuthorityDigest string `json:"authority_digest"`
	KeyID           string `json:"key_id"`
	ApprovedAt      string `json:"approved_at"`
	Signature       string `json:"signature"`
}

type liveUnsignedAuthorityProof struct {
	SchemaVersion   string `json:"schema_version"`
	SourceRef       string `json:"source_ref"`
	SourceDigest    string `json:"source_digest"`
	SourceVersion   int64  `json:"source_version"`
	PlanDigest      string `json:"plan_digest"`
	AuthorityDigest string `json:"authority_digest"`
	KeyID           string `json:"key_id"`
	ApprovedAt      string `json:"approved_at"`
}

func TestBuiltSwornBinaryCompletesLiveDelivery(t *testing.T) {
	if os.Getenv(requireLiveCodexDeliveryEnvironment) != "1" {
		t.Skipf("set %s=1 to spend one explicit live Codex turn", requireLiveCodexDeliveryEnvironment)
	}
	codexBinary := requireLiveExactPath(t, liveCodexBinaryEnvironment)
	authFile := requireLiveExactPath(t, liveCodexAuthFileEnvironment)
	model := os.Getenv(liveCodexModelEnvironment)
	if strings.TrimSpace(model) != model || model == "" {
		t.Fatalf("%s must select one explicit model", liveCodexModelEnvironment)
	}

	fixture := prepareLiveDeliveryFixture(t, codexBinary, authFile, model)
	beforeAuth, err := os.Stat(authFile)
	if err != nil {
		t.Fatal(err)
	}
	binary := buildSwornForProcessTest(t)

	first := runLiveSworn(t, binary, fixture.configPath)
	if first.BuildEffectID == "" || len(first.CheckEffectIDs) != 1 ||
		first.State != engine.WorkReviewable || first.Revision != 4 {
		t.Fatalf("first built-process result = %+v", first)
	}
	afterAuth, err := os.Stat(authFile)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(beforeAuth, afterAuth) {
		t.Fatal("Codex CLI authentication refresh replaced the configured host inode")
	}

	journal, err := store.OpenReadOnly(t.Context(), fixture.controlDatabase)
	if err != nil {
		t.Fatal(err)
	}
	state, stateErr := journal.State(t.Context(), liveDeliveryRunID)
	closeErr := journal.Close()
	if err := errors.Join(stateErr, closeErr); err != nil {
		t.Fatal(err)
	}
	if len(state.Work) != 1 || state.Work[0].State != engine.WorkReviewable ||
		state.Work[0].CandidateCommit == "" || state.Work[0].SubmissionDigest == "" {
		t.Fatalf("durable live delivery state = %+v", state)
	}
	proof := runLiveGit(t, fixture.repositoryRoot, "show", state.Work[0].CandidateCommit+":proof.txt")
	if proof != "ready\n" {
		t.Fatalf("live Codex candidate proof = %q, want exact ready line", proof)
	}
	if target := strings.TrimSpace(runLiveGit(t, fixture.repositoryRoot, "rev-parse", "refs/heads/main")); target != fixture.baseCommit {
		t.Fatalf("bounded run integrated target %q, want unchanged %q", target, fixture.baseCommit)
	}

	second := runLiveSworn(t, binary, fixture.configPath)
	if second.State != engine.WorkReviewable || second.Revision != first.Revision ||
		second.BuildEffectID != "" || len(second.CheckEffectIDs) != 0 ||
		second.Build != nil || second.Checks != nil || second.Admission != nil {
		t.Fatalf("converged second built-process result = %+v", second)
	}
}

func prepareLiveDeliveryFixture(
	t *testing.T,
	codexBinary string,
	authFile string,
	model string,
) liveDeliveryFixture {
	t.Helper()
	ctx := t.Context()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatalf("seal live fixture root: %v", err)
	}
	repositoryRoot := filepath.Join(root, "repository")
	if err := os.Mkdir(repositoryRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	runLiveGit(t, repositoryRoot, "init", "-b", "main")
	runLiveGit(t, repositoryRoot, "config", "user.name", "Sworn Live Proof")
	runLiveGit(t, repositoryRoot, "config", "user.email", "sworn-live@example.invalid")
	if err := os.WriteFile(
		filepath.Join(repositoryRoot, "README.md"),
		[]byte("# Live Sworn delivery proof\n\nThe assigned work is defined only by the Baton plan.\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	runLiveGit(t, repositoryRoot, "add", "--all")
	runLiveGit(t, repositoryRoot, "commit", "-m", "base")
	baseCommit := strings.TrimSpace(runLiveGit(t, repositoryRoot, "rev-parse", "HEAD"))
	binding, err := repo.Discover(ctx, repositoryRoot, "repo-live-proof")
	if err != nil {
		t.Fatal(err)
	}

	contentRuntime := filepath.Join(root, "content-runtime")
	runtimeDigest := prepareLiveCheckRuntime(t, contentRuntime)
	writableRoot := executableTmpfsDirectory(t)
	privateRoots := map[string]string{
		"executor":  filepath.Join(root, "executor-runtime"),
		"builder":   filepath.Join(root, "builder-workspaces"),
		"checks":    filepath.Join(root, "check-workspaces"),
		"authority": filepath.Join(root, "authority-bundles"),
	}
	for label, path := range privateRoots {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatalf("create private %s root: %v", label, err)
		}
	}

	controlDatabase := filepath.Join(root, "control.db")
	journal, err := store.Open(ctx, controlDatabase)
	if err != nil {
		t.Fatal(err)
	}
	plan := prepareLivePlan(t, journal)
	publicKey, approval := publishLiveAuthority(t, journal, plan, privateRoots["authority"])
	applyLiveCommand(t, journal, engine.CommandCreate, engine.NoRevision, engine.CreatePayload{
		DeliveryID: plan.DeliveryID(), PlanDigest: plan.Record().Digest,
		Repository: plan.Target().Repository, TargetRef: plan.Target().Ref, Work: plan.WorkIDs(),
	})
	applyLiveCommand(t, journal, engine.CommandActivate, 0, engine.ActivatePayload{
		AuthorityReceiptDigest: approval.Facts().ReceiptDigest,
	})
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(controlDatabase, 0o600); err != nil {
		t.Fatal(err)
	}
	configuration := app.Config{
		SchemaVersion:   app.RunConfigSchemaVersion,
		ControlDatabase: controlDatabase,
		Repository:      app.RepositoryConfig{Root: repositoryRoot, Binding: binding},
		Authority: app.AuthorityConfig{Sources: []app.AuthoritySource{{
			SourceRef: plan.Authority().SourceRef, AuthorizerRef: "identity:sworn-live-proof",
			PublicKey:       base64.StdEncoding.EncodeToString(publicKey),
			BundleDirectory: privateRoots["authority"],
		}}},
		Executor: app.ExecutorConfig{
			RuntimeRoot: privateRoots["executor"], WritableRoot: writableRoot,
			Bubblewrap: exactExecutable(t, "bwrap"), SystemdRun: exactExecutable(t, "systemd-run"),
			Systemctl: exactExecutable(t, "systemctl"),
		},
		ContentRuntime: app.ContentRuntime{
			Source: contentRuntime, Digest: runtimeDigest, MaximumBytes: 16 << 20,
		},
		Workspaces: app.WorkspaceConfig{
			BuilderRoot: privateRoots["builder"], CheckRoot: privateRoots["checks"],
		},
		Codex: app.CodexConfig{
			Binary: codexBinary, ChatGPTAuthFile: authFile,
			Model: model, TimeoutSeconds: 180,
		},
	}
	encoded, err := json.Marshal(configuration)
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "run.json")
	if err := os.WriteFile(configPath, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	return liveDeliveryFixture{
		configPath: configPath, controlDatabase: controlDatabase,
		repositoryRoot: repositoryRoot, baseCommit: baseCommit, authFile: authFile,
	}
}

func prepareLivePlan(t *testing.T, journal *store.Store) protocol.ExactPlan {
	t.Helper()
	definition, err := protocol.EncodeCanonical(protocol.LocalCheckDefinition{
		SchemaVersion: protocol.LocalCheckDefinitionSchemaVersion,
		Argv: []string{
			"/usr/bin/sh", "-c",
			`test "$(/usr/bin/wc -c < proof.txt)" -eq 6 && test "$(/usr/bin/cat proof.txt)" = ready`,
		},
		WorkingDirectory: ".", TimeoutSeconds: 10,
		Evidence: protocol.LocalEvidenceDefinition{
			ID: "evidence-proof", AcceptanceIDs: []string{"AC1"},
			Boundary: "assembled", Observed: "the exact candidate contains the required proof line",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	definitionDigest, err := journal.PutArtifact(t.Context(), "application/json", definition)
	if err != nil {
		t.Fatal(err)
	}
	assurance, err := protocol.EncodeCanonical(map[string]any{
		"schema_version": protocol.AssurancePolicySchemaVersion,
		"policy_id":      "live-proof-standard",
		"checks": []any{map[string]any{
			"id": "proof", "definition": map[string]any{
				"ref": "policy/checks/proof.json", "media_type": "application/json",
				"digest": definitionDigest,
			},
		}},
		"packs": []any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	assuranceDigest, err := journal.PutArtifact(t.Context(), "application/json", assurance)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := protocol.SnapshotFS()
	if err != nil {
		t.Fatal(err)
	}
	planBytes, err := fs.ReadFile(snapshot, "examples/standard-plan.json")
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(planBytes, &document); err != nil {
		t.Fatal(err)
	}
	document["delivery_id"] = "live-codex-proof"
	document["outcome"] = "Create one exact proof file through the native Codex builder."
	document["created_at"] = time.Now().UTC().Add(-5 * time.Minute).Format(time.RFC3339Nano)
	document["assurance_policy"] = map[string]any{
		"ref": "policy/assurance.json", "digest": assuranceDigest,
	}
	document["target"].(map[string]any)["repository"] = "repo-live-proof"
	for _, raw := range document["authority"].(map[string]any)["grants"].([]any) {
		grant := raw.(map[string]any)
		if grant["action"] == "integrate" {
			grant["target"].(map[string]any)["repository"] = "repo-live-proof"
		}
	}
	work := document["work"].([]any)[0].(map[string]any)
	work["id"] = liveDeliveryWorkID
	work["outcome"] = "Create proof.txt containing exactly ready followed by one newline."
	work["scope"] = map[string]any{"include": []any{"proof.txt"}, "exclude": []any{"vendor"}}
	work["acceptance"] = []any{map[string]any{
		"id": "AC1", "criterion": "proof.txt contains exactly ready followed by one newline.",
		"evidence_level": "assembled",
	}}
	work["constraints"] = []any{
		"Change only proof.txt.", "Do not add a dependency.", "Do not modify repository instructions.",
	}
	canonical, err := protocol.EncodeCanonical(document)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := protocol.ParseDeliveryPlan(canonical)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func publishLiveAuthority(
	t *testing.T,
	journal *store.Store,
	plan protocol.ExactPlan,
	bundleDirectory string,
) (ed25519.PublicKey, policy.HistoricalApproval) {
	t.Helper()
	seed := sha256.Sum256([]byte("Sworn live built-process authority proof"))
	privateKey := ed25519.NewKeyFromSeed(seed[:])
	publicKey := privateKey.Public().(ed25519.PublicKey)
	root, err := policy.NewTrustRoot(
		plan.Authority().SourceRef, "identity:sworn-live-proof", publicKey,
	)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	grants := make([]json.RawMessage, 0, len(plan.Authority().Grants))
	for _, grant := range plan.Authority().Grants {
		grants = append(grants, json.RawMessage(grant.CanonicalJSON()))
	}
	source, err := protocol.EncodeCanonical(liveAuthoritySource{
		Version: 1, SourceID: "live-proof-source", Status: "active",
		Repository: plan.Target().Repository, TargetRef: plan.Target().Ref,
		MaximumGrants: grants, AuthorizerRef: "identity:sworn-live-proof",
		ValidFrom:  now.Add(-time.Hour).Format(time.RFC3339Nano),
		ValidUntil: now.Add(24 * time.Hour).Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatal(err)
	}
	proof := liveAuthorityProof{
		SchemaVersion: policy.AuthorityProofSchemaVersion,
		SourceRef:     plan.Authority().SourceRef, SourceDigest: protocol.CanonicalDigest(source),
		SourceVersion: 1, PlanDigest: plan.Record().Digest,
		AuthorityDigest: plan.Authority().Digest, KeyID: root.KeyID(),
		ApprovedAt: now.Add(-time.Minute).Format(time.RFC3339Nano),
	}
	unsigned, err := protocol.EncodeCanonical(liveUnsignedAuthorityProof{
		SchemaVersion: proof.SchemaVersion, SourceRef: proof.SourceRef,
		SourceDigest: proof.SourceDigest, SourceVersion: proof.SourceVersion,
		PlanDigest: proof.PlanDigest, AuthorityDigest: proof.AuthorityDigest,
		KeyID: proof.KeyID, ApprovedAt: proof.ApprovedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	message := append([]byte("sworn/authority-proof/v1\x00"), unsigned...)
	proof.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, message))
	proofBytes, err := protocol.EncodeCanonical(proof)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := protocol.EncodeCanonical(map[string]any{
		"schema_version": configservice.AuthorityBundleSchemaVersion,
		"source":         base64.RawURLEncoding.EncodeToString(source),
		"proof":          base64.RawURLEncoding.EncodeToString(proofBytes),
	})
	if err != nil {
		t.Fatal(err)
	}
	bundleName := strings.TrimPrefix(plan.Record().Digest, "sha256:") + ".json"
	if err := os.WriteFile(filepath.Join(bundleDirectory, bundleName), bundle, 0o600); err != nil {
		t.Fatal(err)
	}
	authority, err := configservice.OpenAuthority([]configservice.AuthoritySource{{
		SourceRef: plan.Authority().SourceRef, AuthorizerRef: "identity:sworn-live-proof",
		PublicKey: publicKey, BundleDirectory: bundleDirectory,
	}}, journal)
	if err != nil {
		t.Fatal(err)
	}
	approval, approvalErr := authority.Service().Approve(t.Context(), plan)
	closeErr := authority.Close()
	if err := errors.Join(approvalErr, closeErr); err != nil {
		t.Fatal(err)
	}
	return publicKey, approval
}

func prepareLiveCheckRuntime(t *testing.T, root string) string {
	t.Helper()
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	busybox, err := exec.LookPath("busybox")
	if err != nil {
		t.Skipf("live built-process proof requires static busybox: %v", err)
	}
	busybox, err = filepath.EvalSymlinks(busybox)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"sh", "cat", "wc"} {
		copyLiveExecutable(t, busybox, filepath.Join(bin, name))
	}
	digest, _, err := workspace.Measure(t.Context(), root, 16<<20)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func copyLiveExecutable(t *testing.T, source, target string) {
	t.Helper()
	input, err := os.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o555)
	if err != nil {
		_ = input.Close()
		t.Fatal(err)
	}
	_, copyErr := io.Copy(output, input)
	if err := errors.Join(copyErr, output.Sync(), output.Close(), input.Close()); err != nil {
		t.Fatal(err)
	}
}

func applyLiveCommand(
	t *testing.T,
	journal *store.Store,
	kind engine.CommandKind,
	revision int64,
	payload any,
) {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	result, err := journal.Apply(t.Context(), engine.Command{
		ID: "cmd-live-" + string(kind), RunID: liveDeliveryRunID,
		Kind: kind, ExpectedRevision: revision, Payload: encoded,
	})
	if err != nil || result.Outcome != store.OutcomeApplied {
		t.Fatalf("apply live %s = %+v, %v", kind, result, err)
	}
}

func runLiveSworn(t *testing.T, binary, configPath string) app.Result {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
	command := exec.CommandContext(
		ctx, binary, "run", liveDeliveryRunID, liveDeliveryWorkID,
		"--config", configPath, "--json",
	)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		t.Fatalf(
			"live built process: %v; stdout_bytes=%d stderr_bytes=%d classes=%q",
			err, stdout.Len(), stderr.Len(), liveFailureClasses(stderr.Bytes()),
		)
	}
	if stderr.Len() != 0 {
		t.Fatalf("live built process wrote %d stderr bytes", stderr.Len())
	}
	var result app.Result
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode live built-process result: %v; stdout_bytes=%d", err, stdout.Len())
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("validate live built-process result: %v; result=%+v", err, result)
	}
	return result
}

// liveFailureClasses intentionally exposes only fixed, source-controlled
// labels. Child output shares a process boundary with mutable authentication
// state and must never be echoed by this real-credential test.
func liveFailureClasses(output []byte) []string {
	phrases := []string{
		"resolve run config",
		"configure contained executor",
		"configure pinned Codex builder",
		"start recovered controller",
		"controller ownership requires",
		"retain control store",
		"inspect retained control store",
		"control store parent",
		"control store mode",
		"control store identity",
		"permissions",
		"unsafe for ownership",
		"group and world write bits",
		"owned by uid",
		"lacks Linux ownership facts",
		"changed identity",
		"was replaced",
		"acquire controller ownership",
		"validate acquired controller ownership",
		"controller recovery",
		"unresolved effects",
		"ownership",
		"hard links",
		"exactly one",
		"private",
		"advance selected work to reviewable",
		"credential",
		"authentication",
		"login",
		"model",
		"Codex",
		"builder",
		"contained target",
		"transient executor service",
		"Bubblewrap",
		"systemd",
		"permission denied",
		"No such file",
		"invalid",
		"failed",
		"non-zero",
	}
	classes := make([]string, 0, len(phrases))
	for _, phrase := range phrases {
		if bytes.Contains(output, []byte(phrase)) {
			classes = append(classes, phrase)
		}
	}
	return classes
}

func requireLiveExactPath(t *testing.T, environment string) string {
	t.Helper()
	path := os.Getenv(environment)
	if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		t.Fatalf("%s must be a clean absolute path", environment)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || resolved != path {
		t.Fatalf("%s must name an existing path without symbolic-link remaps: %v", environment, err)
	}
	return path
}

func runLiveGit(t *testing.T, root string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, arguments...)...)
	command.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + t.TempDir(),
		"LANG=C",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_ASKPASS=/bin/false",
		"SSH_ASKPASS=/bin/false",
		"GIT_CONFIG_COUNT=2",
		"GIT_CONFIG_KEY_0=commit.gpgSign",
		"GIT_CONFIG_VALUE_0=false",
		"GIT_CONFIG_KEY_1=core.hooksPath",
		"GIT_CONFIG_VALUE_1=/dev/null",
	}
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(arguments, " "), err, output)
	}
	return string(output)
}
