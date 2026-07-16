package baton

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	batonschemas "github.com/swornagent/sworn/internal/baton/schemas"
)

func makeVendorGitAdmin(t *testing.T, repoRoot string) {
	t.Helper()
	if err := os.Mkdir(filepath.Join(repoRoot, ".git"), 0o755); err != nil && !os.IsExist(err) {
		t.Fatalf("create fake Git admin: %v", err)
	}
}

func TestVendorMappingsCoverEveryEmbeddedSchema(t *testing.T) {
	mapped := make(map[string]bool)
	for _, mapping := range batonFileMappings {
		mapped[filepath.ToSlash(mapping.Dest)] = true
	}

	for name := range batonschemas.SchemaMap {
		dest := "internal/baton/schemas/" + name + ".json"
		if !mapped[dest] {
			t.Errorf("embedded schema %s is outside Baton tag parity enforcement", dest)
		}
	}
}

func TestSortVendorCandidatesBytewise(t *testing.T) {
	candidates := []vendorCandidate{
		{path: "z"},
		{path: "a/b"},
		{path: "a"},
		{path: "a//b"},
		{path: "internal/é"},
		{path: "internal/z"},
	}
	sortVendorCandidatesBytewise(candidates)
	want := []string{"a", "a//b", "a/b", "internal/z", "internal/é", "z"}
	for i, candidate := range candidates {
		if candidate.path != want[i] {
			t.Fatalf("candidate %d = %q, want byte order %q", i, candidate.path, want[i])
		}
	}
}

func TestValidateSource(t *testing.T) {
	fixture := filepath.Join("testdata", "fixture")

	if err := ValidateSource(fixture); err != nil {
		t.Fatalf("ValidateSource(%q) = %v, want nil", fixture, err)
	}
}

func TestValidateSource_MissingFile(t *testing.T) {
	dir := t.TempDir()
	err := ValidateSource(dir)
	if err == nil {
		t.Fatal("ValidateSource(empty dir) = nil, want error")
	}
	if !strings.Contains(err.Error(), "source file missing") {
		t.Errorf("error = %v, want 'source file missing'", err)
	}
}

func TestVendorWritesTransformedEmbed(t *testing.T) {
	fixture, err := filepath.Abs(filepath.Join("testdata", "fixture"))
	if err != nil {
		t.Fatal(err)
	}

	tmpRepo := t.TempDir()
	makeVendorGitAdmin(t, tmpRepo)
	for _, m := range batonFileMappings {
		destDir := filepath.Join(tmpRepo, filepath.Dir(m.Dest))
		if err := os.MkdirAll(destDir, 0755); err != nil {
			t.Fatal(err)
		}
	}

	opts := VendorOpts{
		SourceDir: fixture,
		RepoRoot:  tmpRepo,
		CheckOnly: false,
	}

	result, err := Vendor(opts)
	if err != nil {
		t.Fatalf("Vendor() error = %v", err)
	}

	if result.FilesWritten == 0 {
		t.Error("Vendor() wrote 0 files, expected > 0")
	}

	// Spot-check: a rule file has been transformed.
	ruleFile := filepath.Join(tmpRepo, "internal/adopt/baton/rules/01-reachability-gate.md")
	content, err := os.ReadFile(ruleFile)
	if err != nil {
		t.Fatalf("cannot read %s: %v", ruleFile, err)
	}
	if strings.Contains(string(content), "release-verify.sh") {
		t.Error("rule file still contains release-verify.sh after Vendor")
	}
	if !strings.Contains(string(content), "sworn verify") {
		t.Error("rule file missing 'sworn verify' after Vendor")
	}

	// Spot-check: role prompt.
	implFile := filepath.Join(tmpRepo, "internal/prompt/implementer.md")
	content, err = os.ReadFile(implFile)
	if err != nil {
		t.Fatalf("cannot read %s: %v", implFile, err)
	}
	if strings.Contains(string(content), "release-verify.sh") {
		t.Error("implementer prompt still contains release-verify.sh after Vendor")
	}

	// Spot-check: the combined rules.md.
	rulesFile := filepath.Join(tmpRepo, "internal/prompt/baton/rules.md")
	content, err = os.ReadFile(rulesFile)
	if err != nil {
		t.Fatalf("cannot read %s: %v", rulesFile, err)
	}
	ruleCount := strings.Count(string(content), "# Rule:")
	if ruleCount != 10 {
		t.Errorf("rules.md contains %d rule headers, want 10", ruleCount)
	}

	// Verify no script refs survive in any output.
	for _, m := range batonFileMappings {
		if isSchemaSource(m.Source) {
			continue // normative schemas are deliberately copied verbatim
		}
		destAbs := filepath.Join(tmpRepo, m.Dest)
		content, err := os.ReadFile(destAbs)
		if err != nil {
			t.Errorf("cannot read %s: %v", m.Dest, err)
			continue
		}
		for _, r := range replacements {
			if strings.Contains(string(content), r.token) {
				t.Errorf("%s still contains unmapped ref %q", m.Dest, r.token)
			}
		}
	}

	t.Logf("Files written: %d", result.FilesWritten)
}

func TestVendorIsIdempotent(t *testing.T) {
	fixture, err := filepath.Abs(filepath.Join("testdata", "fixture"))
	if err != nil {
		t.Fatal(err)
	}

	tmpRepo := t.TempDir()
	makeVendorGitAdmin(t, tmpRepo)
	for _, m := range batonFileMappings {
		destDir := filepath.Join(tmpRepo, filepath.Dir(m.Dest))
		if err := os.MkdirAll(destDir, 0755); err != nil {
			t.Fatal(err)
		}
	}

	opts := VendorOpts{
		SourceDir: fixture,
		RepoRoot:  tmpRepo,
		CheckOnly: false,
	}

	result1, err := Vendor(opts)
	if err != nil {
		t.Fatalf("first Vendor() error = %v", err)
	}

	result2, err := Vendor(opts)
	if err != nil {
		t.Fatalf("second Vendor() error = %v", err)
	}

	if result2.FilesWritten != 0 {
		t.Errorf("second Vendor() wrote %d files, want 0 (idempotent)", result2.FilesWritten)
	}

	if result2.Diff != "" {
		t.Errorf("second Vendor() diff = %q, want empty (idempotent)", result2.Diff)
	}

	t.Logf("First run: %d files, second run: %d files", result1.FilesWritten, result2.FilesWritten)
}

func TestVendorCheckOnlyDoesNotWrite(t *testing.T) {
	fixture, err := filepath.Abs(filepath.Join("testdata", "fixture"))
	if err != nil {
		t.Fatal(err)
	}

	tmpRepo := t.TempDir()
	for _, m := range batonFileMappings {
		destDir := filepath.Join(tmpRepo, filepath.Dir(m.Dest))
		if err := os.MkdirAll(destDir, 0755); err != nil {
			t.Fatal(err)
		}
	}

	checkOpts := VendorOpts{
		SourceDir: fixture,
		RepoRoot:  tmpRepo,
		CheckOnly: true,
	}

	result, err := Vendor(checkOpts)
	if err != nil {
		t.Fatalf("Vendor(check) error = %v", err)
	}

	if result.FilesWritten != 0 {
		t.Errorf("CheckOnly wrote %d files, want 0", result.FilesWritten)
	}

	for _, m := range batonFileMappings {
		destAbs := filepath.Join(tmpRepo, m.Dest)
		if _, err := os.Stat(destAbs); err == nil {
			t.Errorf("CheckOnly wrote %s but should not have", m.Dest)
		}
	}

	realOpts := VendorOpts{
		SourceDir: fixture,
		RepoRoot:  tmpRepo,
		CheckOnly: false,
	}
	makeVendorGitAdmin(t, tmpRepo)
	if _, err := Vendor(realOpts); err != nil {
		t.Fatalf("Vendor(real) error = %v", err)
	}

	result2, err := Vendor(checkOpts)
	if err != nil {
		t.Fatalf("Vendor(check after real) error = %v", err)
	}

	if result2.Diff != "" {
		t.Errorf("check after real vendor should have no diff, got: %s", result2.Diff)
	}
}

func TestVendorFailsOnUnmappedScriptInSource(t *testing.T) {
	tmpSource := t.TempDir()

	mustCreate := func(relPath, content string) {
		abs := filepath.Join(tmpSource, relPath)
		if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Build the source tree FROM the mapping, so a new mapping cannot silently
	// rot this test into a "source file missing" failure that says nothing about
	// what the test is actually for (an unmapped script reference).
	// rules.md is a concatenation target, not a source file — Vendor skips it.
	for _, m := range batonFileMappings {
		if m.Source == "baton/rules.md" {
			continue
		}
		content := "# Stub\nNo scripts here."
		if strings.HasSuffix(m.Source, ".json") {
			content = "{}"
		}
		mustCreate(m.Source, content)
	}

	// The point of the test: one source carries a script reference the transform
	// has no mapping for, so Vendor must fail closed.
	mustCreate("baton/role-prompts/verifier.md", "# Verifier\nRun `unknown-script.sh` for something.")

	tmpRepo := t.TempDir()
	makeVendorGitAdmin(t, tmpRepo)
	for _, m := range batonFileMappings {
		destDir := filepath.Join(tmpRepo, filepath.Dir(m.Dest))
		if err := os.MkdirAll(destDir, 0755); err != nil {
			t.Fatal(err)
		}
	}

	_, err := Vendor(VendorOpts{
		SourceDir: tmpSource,
		RepoRoot:  tmpRepo,
		CheckOnly: false,
	})
	if err == nil {
		t.Fatal("Vendor() with unmapped script = nil, want error")
	}
	if !strings.Contains(err.Error(), "unknown script reference") {
		t.Errorf("error = %v, want 'unknown script reference'", err)
	}
}

func TestVendorVersionCandidateSharesCheckAndRollbackTransaction(t *testing.T) {
	fixture, err := filepath.Abs(filepath.Join("testdata", "fixture"))
	if err != nil {
		t.Fatal(err)
	}
	repoRoot := t.TempDir()
	makeVendorGitAdmin(t, repoRoot)
	if _, err := Vendor(VendorOpts{SourceDir: fixture, RepoRoot: repoRoot}); err != nil {
		t.Fatalf("seed vendor tree: %v", err)
	}
	versionPath := filepath.Join(repoRoot, filepath.FromSlash(upstreamVersionPath))
	if err := os.MkdirAll(filepath.Dir(versionPath), 0o755); err != nil {
		t.Fatal(err)
	}
	originalVersion := []byte("baton-protocol: v0.13.1\nvendored: 2026-07-14\nupstream-sha: old\nupstream-digest: sha256:old\n")
	if err := os.WriteFile(versionPath, originalVersion, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(versionPath, 0o600); err != nil {
		t.Fatal(err)
	}
	candidate := &UpstreamVersionCandidate{
		Tag:        "v0.15.1",
		SHA:        strings.Repeat("a", 40),
		Digest:     "sha256:" + strings.Repeat("b", 64),
		CapturedAt: time.Date(2026, 7, 16, 9, 0, 0, 0, time.FixedZone("AEST", 10*60*60)),
	}

	checkResult, err := Vendor(VendorOpts{
		SourceDir:        fixture,
		RepoRoot:         repoRoot,
		CheckOnly:        true,
		VersionCandidate: candidate,
	})
	if err != nil {
		t.Fatalf("upstream check: %v", err)
	}
	if !strings.Contains(checkResult.Diff, "changed: "+upstreamVersionPath) {
		t.Fatalf("check drift omitted VERSION transaction member: %q", checkResult.Diff)
	}
	gotVersion, err := os.ReadFile(versionPath)
	if err != nil || !bytes.Equal(gotVersion, originalVersion) {
		t.Fatalf("check mutated VERSION: read=%v got=%q", err, gotVersion)
	}
	if _, err := os.Lstat(filepath.Join(repoRoot, ".git", "sworn")); !os.IsNotExist(err) {
		t.Fatalf("check created Git-admin transaction state: %v", err)
	}

	applyCount := 0
	result, err := Vendor(VendorOpts{
		SourceDir:        fixture,
		RepoRoot:         repoRoot,
		VersionCandidate: candidate,
		fileOps: &vendorFileOps{
			replace: func(repo vendorRepository, rel string, content []byte, mode os.FileMode) error {
				current := applyCount
				applyCount++
				if err := atomicReplaceVendorFile(repo, rel, content, mode); err != nil {
					return err
				}
				if current == 0 {
					return errors.New("injected first-apply failure")
				}
				return nil
			},
			restore: restoreVendorOriginal,
		},
	})
	if result != nil {
		t.Fatalf("failed transaction returned result: %#v", result)
	}
	var txErr *vendorError
	if !errors.As(err, &txErr) || txErr.class != "apply-failed" {
		t.Fatalf("write error = %v, want apply-failed", err)
	}
	gotVersion, err = os.ReadFile(versionPath)
	if err != nil || !bytes.Equal(gotVersion, originalVersion) {
		t.Fatalf("rollback did not restore VERSION bytes: read=%v got=%q", err, gotVersion)
	}
	info, err := os.Stat(versionPath)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("rollback did not restore VERSION mode: info=%v err=%v", info, err)
	}
}
