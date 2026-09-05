package gitx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func productFileAt(
	t *testing.T,
	repository *Repository,
	commit OID,
	path string,
) string {
	t.Helper()
	raw, err := repository.run(
		nil,
		nil,
		"show",
		commit.String()+":"+path,
	)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestPrepareProductCompositionResolvesBaseOnlyAfterConflict(t *testing.T) {
	t.Run("clean-path-is-lazy", func(t *testing.T) {
		repository, historical := newRepository(t, SHA1)
		_, product := inertAdmissions(t, repository, nil)
		expected := prepareProduct(
			t,
			repository,
			historical,
			[]BlobChange{{Path: "expected.txt", Bytes: []byte("expected\n")}},
			"expected",
		)
		candidate := prepareProduct(
			t,
			repository,
			historical,
			[]BlobChange{{Path: "candidate.txt", Bytes: []byte("candidate\n")}},
			"candidate",
		)
		resolutions := 0
		prepared, err := repository.PrepareProductComposition(
			CompositionRequest{Identity: testIdentity,
				Expected: expected.Commit, Candidate: candidate.Commit,
				TargetRef:        "refs/heads/result/clean",
				ProductAdmission: product,
			},
			func() (OID, error) {
				resolutions++
				return historical, nil
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if resolutions != 0 || prepared.Mode != TwoParent {
			t.Fatalf("clean composition = %#v, resolver calls = %d", prepared, resolutions)
		}
	})

	t.Run("false-history-replay", func(t *testing.T) {
		repository, historical := newRepository(t, SHA1)
		_, product := inertAdmissions(t, repository, nil)
		productBase := prepareProduct(
			t,
			repository,
			historical,
			[]BlobChange{{Path: "shared.txt", Bytes: []byte("reviewed foundation\n")}},
			"reviewed product base",
		)
		expected := prepareProduct(
			t,
			repository,
			historical,
			[]BlobChange{
				{Path: "shared.txt", Bytes: []byte("current consumer foundation\n")},
				{Path: "consumer.txt", Bytes: []byte("consumer\n")},
			},
			"consumer",
		)
		candidate := prepareProduct(
			t,
			repository,
			historical,
			[]BlobChange{
				{Path: "shared.txt", Bytes: []byte("reviewed foundation\n")},
				{Path: "producer.txt", Bytes: []byte("producer\n")},
			},
			"producer",
		)
		request := CompositionRequest{Identity: testIdentity,
			Expected: expected.Commit, Candidate: candidate.Commit,
			TargetRef:        "refs/heads/result/replay",
			ProductAdmission: product,
		}
		_, err := repository.PrepareComposition(request)
		requireGitxErrorCode(t, err, "MERGE_CONFLICT")

		resolutions := 0
		prepared, err := repository.PrepareProductComposition(
			request,
			func() (OID, error) {
				resolutions++
				return productBase.Commit, nil
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if resolutions != 1 ||
			len(prepared.Parents) != 2 ||
			prepared.Parents[0] != expected.Commit ||
			prepared.Parents[1] != candidate.Commit {
			t.Fatalf("replayed composition = %#v, resolver calls = %d", prepared, resolutions)
		}
		if got := productFileAt(t, repository, prepared.Commit, "shared.txt"); got != "current consumer foundation\n" {
			t.Fatalf("shared.txt = %q", got)
		}
		if got := productFileAt(t, repository, prepared.Commit, "producer.txt"); got != "producer\n" {
			t.Fatalf("producer.txt = %q", got)
		}
	})
}

func TestPrepareProductCompositionRejectsHostileBaseWithoutMovingRef(t *testing.T) {
	repository, historical := newRepository(t, SHA1)
	_, product := inertAdmissions(t, repository, nil)
	productBase := prepareProduct(
		t,
		repository,
		historical,
		[]BlobChange{
			{Path: ".gitattributes", Bytes: []byte("shared.txt merge=hostile\n")},
			{Path: "shared.txt", Bytes: []byte("foundation\n")},
		},
		"hostile product base",
	)
	expected := prepareProduct(
		t,
		repository,
		historical,
		[]BlobChange{{Path: "shared.txt", Bytes: []byte("left\n")}},
		"left",
	)
	candidate := prepareProduct(
		t,
		repository,
		historical,
		[]BlobChange{{Path: "shared.txt", Bytes: []byte("right\n")}},
		"right",
	)
	targetRef := "refs/heads/result/hostile"
	if err := repository.AtomicUpdateRefs([]RefOperation{{
		Kind: CreateRef, Ref: targetRef, NewHead: &expected.Commit,
	}}); err != nil {
		t.Fatal(err)
	}
	_, err := repository.PrepareProductComposition(
		CompositionRequest{Identity: testIdentity,
			Expected: expected.Commit, Candidate: candidate.Commit,
			TargetRef: targetRef, ProductAdmission: product,
		},
		func() (OID, error) { return productBase.Commit, nil },
	)
	requireGitxErrorCode(t, err, "CUSTOM_MERGE_DRIVER")
	requireDirectHead(t, repository, targetRef, expected.Commit)
}

func TestPrepareApprovedTargetBasePreservesFirstParentAuthority(t *testing.T) {
	repository, historical := newRepository(t, SHA1)
	_, product := inertAdmissions(t, repository, nil)
	authority := prepareProduct(
		t,
		repository,
		historical,
		[]BlobChange{{Path: "authority.txt", Bytes: []byte("authority\n")}},
		"authority",
	)
	targetFirst := prepareProduct(
		t,
		repository,
		historical,
		[]BlobChange{{Path: "target.txt", Bytes: []byte("target\n")}},
		"target first parent",
	)
	targetMerge, err := repository.prepareTwoParentComposition(
		CompositionRequest{Identity: testIdentity,
			Expected: targetFirst.Commit, Candidate: authority.Commit,
			TargetRef:        "refs/heads/main",
			ProductAdmission: product,
		},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := repository.PrepareApprovedTargetBase(
		CompositionRequest{Identity: testIdentity,
			Expected: authority.Commit, Candidate: targetMerge.Commit,
			TargetRef:        "refs/heads/tracks/consumer",
			ProductAdmission: product,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Mode != TwoParent ||
		len(prepared.Parents) != 2 ||
		prepared.Parents[0] != authority.Commit ||
		prepared.Parents[1] != targetMerge.Commit {
		t.Fatalf("approved target base = %#v", prepared)
	}
	history, err := repository.ReadFirstParentHistory(prepared.Commit, MaxHistory)
	if err != nil {
		t.Fatal(err)
	}
	foundAuthority := false
	foundTargetMerge := false
	for _, entry := range history {
		foundAuthority = foundAuthority || entry.OID == authority.Commit
		foundTargetMerge = foundTargetMerge || entry.OID == targetMerge.Commit
	}
	if !foundAuthority || foundTargetMerge {
		t.Fatalf("first-parent history authority=%t target=%t", foundAuthority, foundTargetMerge)
	}
}

func TestPrepareRecordTransitionRefusesSymlinkOrBlobRootsAndAncestors(t *testing.T) {
	t.Run("documents_root ancestor is symlink", func(t *testing.T) {
		repository, _ := newRepository(t, SHA1)
		root := repository.Root()
		record, product := inertAdmissions(t, repository, nil)

		if err := os.Symlink("target", filepath.Join(root, "docs")); err != nil {
			t.Fatal(err)
		}
		runTestGit(t, root, nil, "add", "--", "docs")
		runTestGit(t, root, nil, "commit", "--quiet", "-m", "symlinked docs ancestor")
		base, err := ParseOID(SHA1, runTestGit(t, root, nil, "rev-parse", "HEAD"))
		if err != nil {
			t.Fatal(err)
		}

		countBefore := runTestGit(t, root, nil, "count-objects")
		_, err = repository.PrepareRecordTransition(RecordTransitionRequest{
			Identity:         testIdentity,
			ExpectedHead:     base,
			Documents:        map[string][]byte{"docs/sworn/plan.md": []byte("doc content\n")},
			Message:          "record transition",
			RecordAdmission:  record,
			ProductAdmission: product,
		})
		requireGitxErrorCode(t, err, "SYMLINKED_RECORD_ROOT")
		if !strings.Contains(err.Error(), "docs is a symlink; declare documents_root in docs/sworn/sworn.json") {
			t.Fatalf("unexpected error message: %v", err)
		}
		countAfter := runTestGit(t, root, nil, "count-objects")
		if countBefore != countAfter {
			t.Fatalf("objects written: before=%s, after=%s", countBefore, countAfter)
		}
	})

	t.Run("documents_root ancestor is blob", func(t *testing.T) {
		repository, _ := newRepository(t, SHA1)
		root := repository.Root()
		record, product := inertAdmissions(t, repository, nil)

		if err := os.WriteFile(filepath.Join(root, "docs"), []byte("regular file\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		runTestGit(t, root, nil, "add", "--", "docs")
		runTestGit(t, root, nil, "commit", "--quiet", "-m", "blob docs ancestor")
		base, err := ParseOID(SHA1, runTestGit(t, root, nil, "rev-parse", "HEAD"))
		if err != nil {
			t.Fatal(err)
		}

		countBefore := runTestGit(t, root, nil, "count-objects")
		_, err = repository.PrepareRecordTransition(RecordTransitionRequest{
			Identity:         testIdentity,
			ExpectedHead:     base,
			Documents:        map[string][]byte{"docs/sworn/plan.md": []byte("doc content\n")},
			Message:          "record transition",
			RecordAdmission:  record,
			ProductAdmission: product,
		})
		requireGitxErrorCode(t, err, "INVALID_RECORD_ROOT")
		if !strings.Contains(err.Error(), "docs is not a tree; declare documents_root in docs/sworn/sworn.json") {
			t.Fatalf("unexpected error message: %v", err)
		}
		countAfter := runTestGit(t, root, nil, "count-objects")
		if countBefore != countAfter {
			t.Fatalf("objects written: before=%s, after=%s", countBefore, countAfter)
		}
	})

	t.Run("records_root ancestor is symlink", func(t *testing.T) {
		repository, _ := newRepository(t, SHA1)
		root := repository.Root()
		record, product := inertAdmissions(t, repository, nil)

		if err := os.Symlink("target", filepath.Join(root, ".sworn")); err != nil {
			t.Fatal(err)
		}
		runTestGit(t, root, nil, "add", "--", ".sworn")
		runTestGit(t, root, nil, "commit", "--quiet", "-m", "symlinked .sworn ancestor")
		base, err := ParseOID(SHA1, runTestGit(t, root, nil, "rev-parse", "HEAD"))
		if err != nil {
			t.Fatal(err)
		}

		countBefore := runTestGit(t, root, nil, "count-objects")
		_, err = repository.PrepareRecordTransition(RecordTransitionRequest{
			Identity:     testIdentity,
			ExpectedHead: base,
			Changes: []BlobChange{{
				Path:  DefaultRecordsRoot + "/release/plan.md",
				Bytes: []byte("plan content\n"),
			}},
			Message:          "record transition",
			RecordAdmission:  record,
			ProductAdmission: product,
		})
		requireGitxErrorCode(t, err, "SYMLINKED_RECORD_ROOT")
		if !strings.Contains(err.Error(), ".sworn is a symlink; declare records_root in docs/sworn/sworn.json") {
			t.Fatalf("unexpected error message: %v", err)
		}
		countAfter := runTestGit(t, root, nil, "count-objects")
		if countBefore != countAfter {
			t.Fatalf("objects written: before=%s, after=%s", countBefore, countAfter)
		}
	})

	t.Run("documents_root itself is symlink", func(t *testing.T) {
		repository, _ := newRepository(t, SHA1)
		root := repository.Root()
		record, product := inertAdmissions(t, repository, nil)

		if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("target", filepath.Join(root, "docs", "sworn")); err != nil {
			t.Fatal(err)
		}
		runTestGit(t, root, nil, "add", "--", "docs/sworn")
		runTestGit(t, root, nil, "commit", "--quiet", "-m", "symlinked docs/sworn root")
		base, err := ParseOID(SHA1, runTestGit(t, root, nil, "rev-parse", "HEAD"))
		if err != nil {
			t.Fatal(err)
		}

		countBefore := runTestGit(t, root, nil, "count-objects")
		_, err = repository.PrepareRecordTransition(RecordTransitionRequest{
			Identity:         testIdentity,
			ExpectedHead:     base,
			Documents:        map[string][]byte{"docs/sworn/plan.md": []byte("doc content\n")},
			Message:          "record transition",
			RecordAdmission:  record,
			ProductAdmission: product,
		})
		requireGitxErrorCode(t, err, "SYMLINKED_RECORD_ROOT")
		if !strings.Contains(err.Error(), "docs/sworn is a symlink; declare documents_root in docs/sworn/sworn.json") {
			t.Fatalf("unexpected error message: %v", err)
		}
		countAfter := runTestGit(t, root, nil, "count-objects")
		if countBefore != countAfter {
			t.Fatalf("objects written: before=%s, after=%s", countBefore, countAfter)
		}
	})

	t.Run("records_root itself is blob", func(t *testing.T) {
		repository, _ := newRepository(t, SHA1)
		root := repository.Root()
		record, product := inertAdmissions(t, repository, nil)

		if err := os.MkdirAll(filepath.Join(root, ".sworn"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, ".sworn", "records"), []byte("blob\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		runTestGit(t, root, nil, "add", "--", ".sworn/records")
		runTestGit(t, root, nil, "commit", "--quiet", "-m", "blob .sworn/records root")
		base, err := ParseOID(SHA1, runTestGit(t, root, nil, "rev-parse", "HEAD"))
		if err != nil {
			t.Fatal(err)
		}

		countBefore := runTestGit(t, root, nil, "count-objects")
		_, err = repository.PrepareRecordTransition(RecordTransitionRequest{
			Identity:     testIdentity,
			ExpectedHead: base,
			Changes: []BlobChange{{
				Path:  DefaultRecordsRoot + "/release/plan.md",
				Bytes: []byte("plan content\n"),
			}},
			Message:          "record transition",
			RecordAdmission:  record,
			ProductAdmission: product,
		})
		requireGitxErrorCode(t, err, "INVALID_RECORD_ROOT")
		if !strings.Contains(err.Error(), ".sworn/records is not a tree; declare records_root in docs/sworn/sworn.json") {
			t.Fatalf("unexpected error message: %v", err)
		}
		countAfter := runTestGit(t, root, nil, "count-objects")
		if countBefore != countAfter {
			t.Fatalf("objects written: before=%s, after=%s", countBefore, countAfter)
		}
	})
}
