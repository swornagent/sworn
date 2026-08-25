package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func planTestRepo(t *testing.T) string {
	t.Helper()
	git, err := gitExecutablePath()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	command := exec.Command(git, "init", "--quiet", "--initial-branch=main", root)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	writeFile := func(rel string, body []byte) {
		t.Helper()
		target := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeFile("base.txt", []byte("base\n"))
	env := planGitEnv(root)
	for _, args := range [][]string{
		{"-C", root, "add", "--", "base.txt"},
		{"-C", root, "commit", "--quiet", "-m", "base"},
	} {
		cmd := exec.Command(git, args...)
		cmd.Env = env
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	return root
}

func planGitEnv(root string) []string {
	return append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid",
		"GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid",
	)
}

func planContractBody(sliceID, includePath string) map[string]any {
	return map[string]any{
		"outcome":     "Deliver " + sliceID + ".",
		"scope":       map[string]any{"include": []any{includePath}, "exclude": []any{}},
		"acceptance":  []any{map[string]any{"id": "A-" + sliceID, "text": sliceID + " is exact."}},
		"checks":      []any{"go test ./..."},
		"constraints": []any{"deterministic"},
		"depends_on":  []any{}, "consumes": []any{},
	}
}

func planContractRaw(t *testing.T, body map[string]any) []byte {
	t.Helper()
	raw, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func planManifestBytes(t *testing.T, release, contractPath, touchpoint, digest string) []byte {
	t.Helper()
	entry := map[string]any{
		"id": "S1", "outcome": "Deliver S1.",
		"contract_path": contractPath, "digest": digest,
		"depends_on": []any{}, "consumes": []any{},
		"touchpoints": []any{touchpoint},
	}
	value := map[string]any{
		"schema_version": "sworn.release-manifest/v1", "release": release, "revision": int64(1),
		"previous_plan": nil, "repository": "sworn/test", "target_ref": "refs/heads/main",
		"approval_ref": "sworn://approval/" + release + "/1",
		"tracks": []any{
			map[string]any{
				"id": "T1", "depends_on": []any{},
				"slices": []any{entry},
			},
		},
	}
	metadata, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(append([]byte("```sworn-release-manifest-v1\n"), metadata...), []byte("\n```\nbody\n")...)
}

func TestPlanPinRewritesDriftedManifestFromContractBytes(t *testing.T) {
	t.Parallel()
	root := planTestRepo(t)
	contractPath := "contracts/S1.json"
	contractRaw := planContractRaw(t, planContractBody("S1", "one/file.go"))

	// Compute the real digest by importing the baton package's logic
	// indirectly: we use a second manifest that carries the real digest.
	// ParseSliceContract is in internal/baton; we replicate the digest
	// computation by using sworn plan pin itself: write the contract and
	// a manifest with a placeholder digest, run pin, and verify the output
	// carries the correct digest and facts.
	if err := os.MkdirAll(filepath.Join(root, filepath.Dir(contractPath)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, contractPath), contractRaw, 0o644); err != nil {
		t.Fatal(err)
	}

	manifestPath := filepath.Join(root, "manifest.md")
	drifted := planManifestBytes(t, "pin-cli", contractPath, "wrong/path.go",
		"sha256:"+strings.Repeat("0", 64))
	if err := os.WriteFile(manifestPath, drifted, 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := runPlan([]string{"pin", "--manifest", manifestPath, "--project", root}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("plan pin: code=%d stderr=%s", code, stderr.String())
	}

	pinned := stdout.Bytes()
	// The pinned output must contain the correct outcome and touchpoints.
	if !bytes.Contains(pinned, []byte("Deliver S1.")) {
		t.Fatalf("pinned manifest missing correct outcome: %s", pinned)
	}
	if !bytes.Contains(pinned, []byte("one/file.go")) {
		t.Fatalf("pinned manifest missing correct touchpoint: %s", pinned)
	}
	if bytes.Contains(pinned, []byte("wrong/path.go")) {
		t.Fatalf("pinned manifest still has drifted touchpoint: %s", pinned)
	}
	// The trailing prose must survive.
	if !bytes.Contains(pinned, []byte("body\n")) {
		t.Fatalf("pinned manifest missing prose: %s", pinned)
	}
}

func TestPlanPinRejectsBadArguments(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{"pin"},
		{"pin", "--manifest", "/blocking"},
		{"pin", "--manifest", "/abs/manifest", "--project", "relative"},
	} {
		var stdout, stderr bytes.Buffer
		code := runPlan(args, &stdout, &stderr)
		if code != 2 {
			t.Fatalf("runPlan(%v) = %d, want 2", args, code)
		}
		if stdout.Len() != 0 {
			t.Fatalf("runPlan(%v) stdout = %q", args, stdout.String())
		}
	}
}

func TestPlanLintPassesForWellDerivedPlan(t *testing.T) {
	t.Parallel()
	root := planTestRepo(t)
	contractPath := "contracts/S1.json"
	contractRaw := planContractRaw(t, planContractBody("S1", "one/file.go"))
	if err := os.MkdirAll(filepath.Join(root, filepath.Dir(contractPath)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, contractPath), contractRaw, 0o644); err != nil {
		t.Fatal(err)
	}

	// We need the real digest for the manifest. Use pin to compute it
	// first, then lint the pinned output.
	manifestPath := filepath.Join(root, "manifest.md")
	drifted := planManifestBytes(t, "lint-cli", contractPath, "one/file.go",
		"sha256:"+strings.Repeat("0", 64))
	if err := os.WriteFile(manifestPath, drifted, 0o644); err != nil {
		t.Fatal(err)
	}

	var pinOut, pinErr bytes.Buffer
	if code := runPlan([]string{"pin", "--manifest", manifestPath, "--project", root}, &pinOut, &pinErr); code != 0 {
		t.Fatalf("plan pin: code=%d stderr=%s", code, pinErr.String())
	}
	pinned := pinOut.Bytes()
	pinnedPath := filepath.Join(root, "pinned.md")
	if err := os.WriteFile(pinnedPath, pinned, 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := runPlan([]string{"lint", "--manifest", pinnedPath, "--project", root}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("plan lint: code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "S1: PASS") {
		t.Fatalf("lint output missing S1: PASS: %s", stdout.String())
	}
}

func TestPlanLintFailsOnStaleBinding(t *testing.T) {
	t.Parallel()
	root := planTestRepo(t)
	contractPath := "contracts/S1.json"
	contractRaw := planContractRaw(t, planContractBody("S1", "one/file.go"))
	if err := os.MkdirAll(filepath.Join(root, filepath.Dir(contractPath)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, contractPath), contractRaw, 0o644); err != nil {
		t.Fatal(err)
	}

	manifestPath := filepath.Join(root, "manifest.md")
	stale := planManifestBytes(t, "lint-stale", contractPath, "one/file.go",
		"sha256:"+strings.Repeat("9", 64))
	if err := os.WriteFile(manifestPath, stale, 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := runPlan([]string{"lint", "--manifest", manifestPath, "--project", root}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("stale lint should fail")
	}
	if !strings.Contains(stderr.String(), "STALE_BINDING") {
		t.Fatalf("stderr missing STALE_BINDING: %s", stderr.String())
	}
}

func TestPlanLintRejectsBadArguments(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{"lint"},
		{"lint", "--manifest", "/blocking"},
	} {
		var stdout, stderr bytes.Buffer
		code := runPlan(args, &stdout, &stderr)
		if code != 2 {
			t.Fatalf("runPlan(%v) = %d, want 2", args, code)
		}
		if stdout.Len() != 0 {
			t.Fatalf("runPlan(%v) stdout = %q", args, stdout.String())
		}
	}
}

func TestPlanRecordRejectsBadArguments(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{"record"},
		{"record", "--manifest", "/abs/manifest"},
		{"record", "--manifest", "/abs/manifest", "--project", "relative", "--summary", "s"},
	} {
		var stdout, stderr bytes.Buffer
		code := runPlan(args, &stdout, &stderr)
		if code != 2 {
			t.Fatalf("runPlan(%v) = %d, want 2", args, code)
		}
		if stdout.Len() != 0 {
			t.Fatalf("runPlan(%v) stdout = %q", args, stdout.String())
		}
	}
}

func TestPlanIsRegisteredInUsageAndDispatch(t *testing.T) {
	t.Parallel()
	// The usage text must mention the plan command.
	if !strings.Contains(usage, "plan ") {
		t.Fatalf("usage text does not mention plan")
	}
	if !strings.Contains(usage, "sworn plan pin") {
		t.Fatalf("usage text does not mention plan pin")
	}
	if !strings.Contains(usage, "sworn plan lint") {
		t.Fatalf("usage text does not mention plan lint")
	}
	if !strings.Contains(usage, "sworn plan record") {
		t.Fatalf("usage text does not mention plan record")
	}

	// The dispatch must route to runPlan.
	var stdout, stderr bytes.Buffer
	code := run([]string{"plan"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run(plan) = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "sworn plan") {
		t.Fatalf("run(plan) stderr = %q", stderr.String())
	}
}

func TestPlanUnknownVerbFailsClosed(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	code := runPlan([]string{"bogus"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("runPlan(bogus) = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "unknown verb") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestPlanRecordRecordsRevisionEndToEnd(t *testing.T) {
	t.Parallel()
	root := planTestRepo(t)
	git, err := gitExecutablePath()
	if err != nil {
		t.Fatal(err)
	}
	env := planGitEnv(root)

	// Commit the contract to the target branch (main), exactly as a
	// planner would commit it alongside real code before recording.
	contractPath := "contracts/S1.json"
	contractRaw := planContractRaw(t, planContractBody("S1", "one/file.go"))
	if err := os.MkdirAll(filepath.Join(root, filepath.Dir(contractPath)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, contractPath), contractRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	runGit := func(args ...string) string {
		cmd := exec.Command(git, append([]string{"-C", root}, args...)...)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	runGit("add", "--", contractPath)
	runGit("commit", "--quiet", "-m", "add contract")

	// Build a manifest with the correct digest. We use pin to compute the
	// real digest: write a drifted manifest, pin it, and use the pinned
	// output as the record input.
	manifestPath := filepath.Join(root, "manifest.md")
	drifted := planManifestBytes(t, "record-cli", contractPath, "one/file.go",
		"sha256:"+strings.Repeat("0", 64))
	if err := os.WriteFile(manifestPath, drifted, 0o644); err != nil {
		t.Fatal(err)
	}

	var pinOut, pinErr bytes.Buffer
	if code := runPlan([]string{"pin", "--manifest", manifestPath, "--project", root}, &pinOut, &pinErr); code != 0 {
		t.Fatalf("plan pin: code=%d stderr=%s", code, pinErr.String())
	}
	pinned := pinOut.Bytes()
	pinnedPath := filepath.Join(root, "pinned.md")
	if err := os.WriteFile(pinnedPath, pinned, 0o644); err != nil {
		t.Fatal(err)
	}

	// Record the plan revision. The default ContractTree resolves to the
	// working repository HEAD (the commit that added the contract).
	var stdout, stderr bytes.Buffer
	code := runPlan([]string{
		"record", "--manifest", pinnedPath, "--project", root, "--summary", "Record revision 1.",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("plan record: code=%d stderr=%s", code, stderr.String())
	}

	output := stdout.String()
	if !strings.Contains(output, "Recorded plan revision 1") {
		t.Fatalf("record output missing revision: %s", output)
	}
	if !strings.Contains(output, "ref:") {
		t.Fatalf("record output missing ref: %s", output)
	}

	// The release-wt ref must now exist.
	if runGit("show-ref", "--verify", "--quiet", "refs/heads/release-wt/record-cli") != "" {
		t.Fatal("release-wt ref was not created")
	}
}
