package baton

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/gitx"
)

// manifestContractBody returns one real, self-consistent sworn contract body
// for id at includePath. Its digest is computed from these exact bytes via
// ParseSliceContract, so callers control whether a manifest's declared
// digest and dependency lists agree with, or diverge from, the file this
// produces.
func manifestContractBody(id, includePath string) map[string]any {
	return map[string]any{
		"outcome":     "Deliver " + id + ".",
		"scope":       map[string]any{"include": []any{includePath}, "exclude": []any{}},
		"acceptance":  []any{map[string]any{"id": "A-" + id, "text": id + " is exact."}},
		"checks":      []any{"check " + id},
		"constraints": []any{"deterministic"},
		"depends_on":  []any{}, "consumes": []any{},
	}
}

func manifestContractRaw(t *testing.T, value map[string]any) []byte {
	t.Helper()
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// manifestActionPlanBytes builds one sworn.release-manifest/v1 revision 1
// plan admitting a single track T1 and slice S1 whose manifest entry names
// contractPath and digest, matching the repository/target_ref conventions
// actionPlanRevisionBytes already uses for legacy plans in this harness.
func manifestActionPlanBytes(t *testing.T, release, contractPath, touchpoint, digest string, dependsOn []any) []byte {
	t.Helper()
	entry := manifestSliceEntry("S1", contractPath, touchpoint, digest)
	entry["depends_on"] = dependsOn
	value := map[string]any{
		"schema_version": ManifestVersion, "release": release, "revision": int64(1),
		"previous_plan": nil, "repository": "golden/sworn", "target_ref": "refs/heads/main",
		"approval_ref": "golden://approval/" + release + "/1",
		"tracks": []any{
			map[string]any{
				"id": "T1", "depends_on": []any{},
				"slices": []any{entry},
			},
		},
	}
	return manifestRaw(t, value)
}

// prepareActionContractTree stages files on top of base's tree using the
// exact same read-tree/hash-object/update-index/write-tree/commit-tree
// sequence the engine's own record-transition path uses, producing one
// already-prepared Git commit RecordPlanRevisionInput.ContractTree can name.
func prepareActionContractTree(t *testing.T, repoPath, base string, files map[string][]byte) string {
	t.Helper()
	indexEnv := []string{"GIT_INDEX_FILE=" + t.TempDir() + "/index"}
	actionGit(t, repoPath, nil, indexEnv, "read-tree", base+"^{tree}")
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		blob := actionGit(t, repoPath, files[name], nil, "hash-object", "-w", "--stdin")
		actionGit(t, repoPath, nil, indexEnv, "update-index", "--add", "--cacheinfo", "100644,"+blob+","+name)
	}
	tree := actionGit(t, repoPath, nil, indexEnv, "write-tree")
	return actionGit(t, repoPath, []byte("prepare contracts\n"), nil, "commit-tree", tree, "-p", base)
}

func actionRefExists(t *testing.T, repoPath, ref string) bool {
	t.Helper()
	cmd := exec.Command(actionTestGit, "-C", repoPath, "show-ref", "--verify", "--quiet", ref)
	return cmd.Run() == nil
}

func TestRecordPlanRevisionManifestAtomicRecordAndReread(t *testing.T) {
	t.Parallel()
	repoPath, _, actions := createActionHarness(t)
	release := "manifest-atomic"
	contractPath := "contracts/S1.json"
	contractRaw := manifestContractRaw(t, manifestContractBody("S1", "one.txt"))
	_, digest, err := ParseSliceContract(contractRaw, "S1", "T1")
	if err != nil {
		t.Fatal(err)
	}

	// The contract is ordinary product-tree content (contract_path may never
	// name RecordRoot): it is committed to the target branch by an ordinary
	// commit, exactly as a Planner would commit it alongside real code,
	// before any plan revision names it. That commit is the "already-
	// prepared Git tree" RecordPlanRevisionInput.ContractTree names.
	base := actionGit(t, repoPath, nil, nil, "rev-parse", "refs/heads/main")
	withContract := prepareActionContractTree(t, repoPath, base, map[string][]byte{contractPath: contractRaw})
	actionGit(t, repoPath, nil, nil, "update-ref", "refs/heads/main", withContract, base)

	planBytes := manifestActionPlanBytes(t, release, contractPath, "one.txt", digest, []any{})
	result, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: planBytes, ContractTree: withContract,
		Summary: "Approve manifest revision.", Detail: []byte("approval"),
	})
	if err != nil || !result.Changed {
		t.Fatalf("result = %#v, err = %v", result, err)
	}

	planFile, err := actions.repository.file(result.Head, planPath(RecordRoot, release))
	if err != nil || !planFile.Present || !bytes.Equal(planFile.Bytes, planBytes) {
		t.Fatalf("plan file = %#v, err = %v", planFile, err)
	}
	contractFile, err := actions.repository.file(result.Head, contractPath)
	if err != nil || !contractFile.Present || !bytes.Equal(contractFile.Bytes, contractRaw) {
		t.Fatalf("contract file = %#v, err = %v", contractFile, err)
	}

	reread, err := ParsePlan(planFile.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := reread.ResolveSliceContract("S1", contractFile.Bytes)
	if err != nil || resolved.Outcome != "Deliver S1." {
		t.Fatalf("reread resolution = %#v, err = %v", resolved, err)
	}

	// Recording touches the manifest record, the digest-addressed contract
	// store, and the authored documents (A1): the contract bytes are written
	// to the digest-addressed store under the record root (the engine-read
	// authority for digest-addressed resolution), and the authored copy plus
	// the authored plan are published under the documents root in the same
	// commit. The pre-existing contract at its target path is proven in place
	// and never duplicated into a new product-tree blob.
	changed := strings.Fields(actionGit(t, repoPath, nil, nil, "diff", "--name-only", result.Target, result.Head))
	expected := []string{
		contractStorePath(RecordRoot, release, digest),
		planPath(RecordRoot, release),
		documentContractPath(gitx.DefaultDocumentsRoot, release, "S1"),
		documentPlanPath(gitx.DefaultDocumentsRoot, release),
	}
	if !reflect.DeepEqual(changed, expected) {
		t.Fatalf("changed paths at admission = %v, want exactly %v", changed, expected)
	}
	authoredPlan, err := actions.repository.file(
		result.Head,
		documentPlanPath(gitx.DefaultDocumentsRoot, release),
	)
	if err != nil || !authoredPlan.Present || !bytes.Equal(authoredPlan.Bytes, planBytes) {
		t.Fatalf("authored plan = %#v, err = %v", authoredPlan, err)
	}
	authoredContract, err := actions.repository.file(
		result.Head,
		documentContractPath(gitx.DefaultDocumentsRoot, release, "S1"),
	)
	if err != nil || !authoredContract.Present || !bytes.Equal(authoredContract.Bytes, contractRaw) {
		t.Fatalf("authored contract = %#v, err = %v", authoredContract, err)
	}
	// The digest-addressed contract store under the record root carries the
	// engine-read authority: its bytes match the original contract bytes.
	storeFile, err := actions.repository.file(result.Head, contractStorePath(RecordRoot, release, digest))
	if err != nil || !storeFile.Present || !bytes.Equal(storeFile.Bytes, contractRaw) {
		t.Fatalf("digest store = %#v, err = %v", storeFile, err)
	}
}

func TestRecordPlanRevisionManifestContractFailuresLeaveNoTrace(t *testing.T) {
	t.Parallel()
	contractPath := "contracts/S1.json"
	contractRaw := manifestContractRaw(t, manifestContractBody("S1", "one.txt"))
	_, digest, err := ParseSliceContract(contractRaw, "S1", "T1")
	if err != nil {
		t.Fatal(err)
	}
	substituted := manifestContractRaw(t, manifestContractBody("S1", "one.txt"))
	var substitutedValue map[string]any
	if err := json.Unmarshal(substituted, &substitutedValue); err != nil {
		t.Fatal(err)
	}
	substitutedValue["outcome"] = "Deliver a different outcome."
	substitutedRaw := manifestContractRaw(t, substitutedValue)

	mismatchedValue := manifestContractBody("S1", "one.txt")
	mismatchedValue["depends_on"] = []any{"ghost"}
	mismatchedRaw := manifestContractRaw(t, mismatchedValue)
	_, mismatchedDigest, err := ParseSliceContract(mismatchedRaw, "S1", "T1")
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name      string
		digest    string
		dependsOn []any
		buildTree func(t *testing.T, repoPath, base string) string
		code      string
	}{
		{
			name: "missing_source", digest: digest,
			buildTree: func(t *testing.T, repoPath, base string) string { return "" },
			code:      "CONTRACT_SOURCE_REQUIRED",
		},
		{
			name: "missing_path", digest: digest,
			buildTree: func(t *testing.T, repoPath, base string) string {
				return prepareActionContractTree(t, repoPath, base, map[string][]byte{"contracts/OTHER.json": contractRaw})
			},
			code: "CONTRACT_NOT_FOUND",
		},
		{
			name: "substituted_content", digest: digest,
			buildTree: func(t *testing.T, repoPath, base string) string {
				return prepareActionContractTree(t, repoPath, base, map[string][]byte{contractPath: substitutedRaw})
			},
			code: "STALE_BINDING",
		},
		{
			// The manifest's own depends_on ([]) disagrees with the real
			// contract's depends_on (["ghost"]) even though the declared
			// digest exactly matches that real contract's bytes: the digest
			// check alone is not sufficient, so ResolveSliceContract's
			// dependency cross-check must independently fail closed.
			name: "mismatched_dependency", digest: mismatchedDigest, dependsOn: []any{},
			buildTree: func(t *testing.T, repoPath, base string) string {
				return prepareActionContractTree(t, repoPath, base, map[string][]byte{contractPath: mismatchedRaw})
			},
			code: "STALE_BINDING",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			repoPath, _, actions := createActionHarness(t)
			release := "manifest-fail-" + test.name
			base := actionGit(t, repoPath, nil, nil, "rev-parse", "refs/heads/main")
			tree := test.buildTree(t, repoPath, base)
			dependsOn := test.dependsOn
			if dependsOn == nil {
				dependsOn = []any{}
			}
			planBytes := manifestActionPlanBytes(t, release, contractPath, "one.txt", test.digest, dependsOn)

			_, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
				PlanBytes: planBytes, ContractTree: tree,
				Summary: "Approve.", Detail: []byte("approval"),
			})
			if code := ErrorCode(err); code != test.code {
				t.Fatalf("code = %q (%v), want %q", code, err, test.code)
			}
			if actionRefExists(t, repoPath, releaseRef(release)) {
				t.Fatal("release ref must not exist after a rejected manifest revision")
			}
			if after := actionGit(t, repoPath, nil, nil, "rev-parse", "refs/heads/main"); after != base {
				t.Fatal("target ref must not move after a rejected manifest revision")
			}
		})
	}
}

func TestRecordPlanRevisionManifestEscapingContractPathRejectedBeforeAnyRecord(t *testing.T) {
	t.Parallel()
	repoPath, _, actions := createActionHarness(t)
	release := "manifest-escape"
	placeholder := "sha256:" + strings.Repeat("a", 64)
	value := map[string]any{
		"schema_version": ManifestVersion, "release": release, "revision": int64(1),
		"previous_plan": nil, "repository": "golden/sworn", "target_ref": "refs/heads/main",
		"approval_ref": "golden://approval/" + release + "/1",
		"tracks": []any{
			map[string]any{
				"id": "T1", "depends_on": []any{},
				"slices": []any{manifestSliceEntry("S1", "../escape.json", "one.txt", placeholder)},
			},
		},
	}
	planBytes := manifestRaw(t, value)
	base := actionGit(t, repoPath, nil, nil, "rev-parse", "refs/heads/main")

	_, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: planBytes, Summary: "Approve.", Detail: []byte("approval"),
	})
	if code := ErrorCode(err); code != "INVALID_PATH" {
		t.Fatalf("code = %q (%v), want INVALID_PATH", code, err)
	}
	if actionRefExists(t, repoPath, releaseRef(release)) {
		t.Fatal("release ref must not exist after an escaping contract path")
	}
	if after := actionGit(t, repoPath, nil, nil, "rev-parse", "refs/heads/main"); after != base {
		t.Fatal("target ref must not move after an escaping contract path")
	}
}

// TestRecordPlanRevisionLegacyV2IgnoresContractTree proves phase 2 left
// legacy baton.plan/v2 admission byte- and history-identical: it never
// consults ContractTree and records exactly the one plan.md path, matching
// pre-phase-2 behavior.
func TestRecordPlanRevisionPublishesAuthoredDocumentsUnderConfiguredRoot(t *testing.T) {
	t.Parallel()
	repoPath := createActionRepository(t, "sha1")
	if err := os.MkdirAll(filepath.Join(repoPath, "docs", "sworn"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, filepath.FromSlash(gitx.ProjectConfigPath)),
		[]byte(`{"records_root": ".sworn/records", "journals_root": ".sworn", "contracts_root": "contracts", "commit_prefix": "sworn", "documents_root": "docs/specs"}`),
		0o644); err != nil {
		t.Fatal(err)
	}
	actionGit(t, repoPath, nil, nil, "add", "--", filepath.FromSlash(gitx.ProjectConfigPath))
	actionGit(t, repoPath, nil, nil, "commit", "--quiet", "-m", "project config")

	repository, err := gitx.Open(repoPath, actionTestGit)
	if err != nil {
		t.Fatal(err)
	}
	actions, err := NewActions(UseGitRepository(repository), inertActionResolver, gitx.Identity{Name: "Golden Baton Engine", Email: "engine@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	if repository.DocumentsRoot() != "docs/specs" {
		t.Fatalf("documents root = %q", repository.DocumentsRoot())
	}

	release := "authored-docs"
	contractPath := "contracts/S1.json"
	contractRaw := manifestContractRaw(t, manifestContractBody("S1", "one.txt"))
	_, digest, err := ParseSliceContract(contractRaw, "S1", "T1")
	if err != nil {
		t.Fatal(err)
	}
	base := actionGit(t, repoPath, nil, nil, "rev-parse", "refs/heads/main")
	withContract := prepareActionContractTree(t, repoPath, base, map[string][]byte{contractPath: contractRaw})
	actionGit(t, repoPath, nil, nil, "update-ref", "refs/heads/main", withContract, base)

	planBytes := manifestActionPlanBytes(t, release, contractPath, "one.txt", digest, []any{})
	result, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: planBytes, ContractTree: withContract,
		Summary: "Approve.", Detail: []byte("approval"),
	})
	if err != nil || !result.Changed {
		t.Fatalf("result = %#v, err = %v", result, err)
	}

	// The authored documents live under the configured documents root.
	authoredPlan, err := actions.repository.file(result.Head, "docs/specs/"+release+"/plan.md")
	if err != nil || !authoredPlan.Present || !bytes.Equal(authoredPlan.Bytes, planBytes) {
		t.Fatalf("authored plan = %#v, err = %v", authoredPlan, err)
	}
	authoredContract, err := actions.repository.file(result.Head, "docs/specs/"+release+"/contracts/S1.json")
	if err != nil || !authoredContract.Present || !bytes.Equal(authoredContract.Bytes, contractRaw) {
		t.Fatalf("authored contract = %#v, err = %v", authoredContract, err)
	}
	// Nothing authored under the default docs/sworn root (the configured root
	// wins), and the engine reads the record root, never the documents root.
	if file, err := actions.repository.file(result.Head, "docs/sworn/"+release+"/plan.md"); err != nil || file.Present {
		t.Fatalf("default documents root copy present = %#v, err = %v", file, err)
	}
	state := readActionState(t, repository, release)
	if state.Plan.OID == "" || state.Refs.Release.Head != result.Head {
		t.Fatalf("engine state does not read the record root: %#v", state.Plan)
	}
}

func TestRecordPlanRevisionLegacyV2IgnoresContractTree(t *testing.T) {
	t.Parallel()
	repoPath, _, actions := createActionHarness(t)
	release := "legacy-v2-untouched"
	result, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: actionPlanBytes(release),
		Summary:   "Approve legacy revision.", Detail: []byte("approval"),
		ContractTree: "not-a-real-commit-and-must-be-ignored",
	})
	if err != nil || !result.Changed {
		t.Fatalf("result = %#v, err = %v", result, err)
	}
	changed := strings.Fields(actionGit(t, repoPath, nil, nil, "diff", "--name-only", result.Target, result.Head))
	expected := []string{
		planPath(RecordRoot, release),
		documentPlanPath(gitx.DefaultDocumentsRoot, release),
	}
	if !reflect.DeepEqual(changed, expected) {
		t.Fatalf("changed paths = %v, want exactly %v", changed, expected)
	}
}
