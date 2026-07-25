package gitx

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const testGit = "/usr/bin/git"

type gitGoldenComposition struct {
	Mode      string `json:"mode"`
	Expected  string `json:"expected"`
	Candidate string `json:"candidate"`
	Result    string `json:"result"`
}

type gitGoldenFormat struct {
	ObjectFormat      string               `json:"object_format"`
	OIDHexLength      int                  `json:"oid_hex_length"`
	Base              string               `json:"base"`
	Left              string               `json:"left"`
	Right             string               `json:"right"`
	FastForward       gitGoldenComposition `json:"fast_forward"`
	TwoParent         gitGoldenComposition `json:"two_parent"`
	ContainedOutcome  string               `json:"contained_outcome"`
	ResultProductTree string               `json:"result_product_tree"`
}

func loadGitGolden(t *testing.T, format ObjectFormat) gitGoldenFormat {
	t.Helper()
	file, err := os.Open(filepath.Join("..", "..", "tools", "batongolden", "testdata", "corpus", "git.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var corpus struct {
		Schema       string            `json:"schema"`
		ProductTuple string            `json:"product_tuple"`
		Formats      []gitGoldenFormat `json:"formats"`
	}
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&corpus); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		t.Fatalf("Git golden has trailing JSON: %v", err)
	}
	if corpus.Schema != "sworn.baton-golden-git/v1" ||
		corpus.ProductTuple != "path NUL mode NUL type NUL object LF" {
		t.Fatalf("foreign Git golden identity: %#v", corpus)
	}
	for _, candidate := range corpus.Formats {
		if candidate.ObjectFormat == string(format) {
			return candidate
		}
	}
	t.Fatalf("Git golden lacks %s", format)
	return gitGoldenFormat{}
}

func runTestGit(t *testing.T, repository string, stdin []byte, args ...string) string {
	t.Helper()
	commandArgs := append([]string{"-C", repository}, args...)
	command := exec.Command(testGit, commandArgs...)
	command.Env = append(os.Environ(),
		"LANG=C", "LC_ALL=C",
		"GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid",
		"GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid",
		"GIT_AUTHOR_DATE=@1000000000 +0000", "GIT_COMMITTER_DATE=@1000000000 +0000",
	)
	command.Stdin = bytes.NewReader(stdin)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func newRepository(t *testing.T, format ObjectFormat) (*Repository, OID) {
	t.Helper()
	directory := t.TempDir()
	command := exec.Command(testGit, "init", "--quiet", "--initial-branch=main", "--object-format="+string(format), directory)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git init %s: %v\n%s", format, err, output)
	}
	if err := os.WriteFile(filepath.Join(directory, "product.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, directory, nil, "add", "--", "product.txt")
	runTestGit(t, directory, nil, "commit", "--quiet", "-m", "base")
	repository, err := Open(directory, testGit)
	if err != nil {
		t.Fatal(err)
	}
	headText := runTestGit(t, directory, nil, "rev-parse", "HEAD")
	head, err := ParseOID(format, headText)
	if err != nil {
		t.Fatal(err)
	}
	return repository, head
}

func TestOpenRequiresLiteralExecutableAndFixesObjectFormat(t *testing.T) {
	t.Parallel()
	repository, head := newRepository(t, SHA1)
	originalTree, err := repository.TreeOID(head)
	if err != nil {
		t.Fatal(err)
	}
	timestamp, err := repository.CommitTimestamp(head)
	if err != nil {
		t.Fatal(err)
	}
	replacement, err := repository.PrepareRecord(RecordRequest{
		Parent: head, Changes: []BlobChange{{Path: "product.txt", Bytes: []byte("replacement\n")}},
		Message: "replacement\n", Identity: Identity{Name: "Fixture", Email: "fixture@example.invalid"},
		Timestamp: timestamp + 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	runTestGit(t, repository.Root(), nil, "update-ref", "refs/replace/"+head.String(), replacement.Commit.String())
	observedTree, err := repository.TreeOID(head)
	if err != nil {
		t.Fatal(err)
	}
	if observedTree != originalTree {
		t.Fatalf("replace ref changed captured tree: %s != %s", observedTree, originalTree)
	}
	if repository.ObjectFormat() != SHA1 || len(head.String()) != 40 {
		t.Fatalf("format/head = %s/%s", repository.ObjectFormat(), head)
	}
	if _, err := Open(repository.Root(), "git"); err == nil {
		t.Fatal("Open accepted PATH lookup")
	}
	if _, err := ParseOID(SHA1, strings.Repeat("A", 40)); err == nil {
		t.Fatal("ParseOID accepted uppercase")
	}
	if _, err := ParseOID(SHA256, head.String()); err == nil {
		t.Fatal("ParseOID accepted cross-format width")
	}
}

func TestProductIdentityRecordPreparationAndAtomicCASBothFormats(t *testing.T) {
	t.Parallel()
	for _, format := range []ObjectFormat{SHA1, SHA256} {
		format := format
		t.Run(string(format), func(t *testing.T) {
			t.Parallel()
			repository, base := newRepository(t, format)
			timestamp, err := repository.CommitTimestamp(base)
			if err != nil {
				t.Fatal(err)
			}
			request := RecordRequest{
				Parent: base,
				Changes: []BlobChange{{
					Path: ".control/records/demo/metadata", Bytes: []byte("metadata\n"),
				}},
				Message:   "Record fixture\n",
				Identity:  Identity{Name: "Record Engine", Email: "records@example.invalid"},
				Timestamp: timestamp + 1,
			}
			first, err := repository.PrepareRecord(request)
			if err != nil {
				t.Fatal(err)
			}
			second, err := repository.PrepareRecord(request)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(first, second) {
				t.Fatalf("deterministic preparation differs: %#v != %#v", first, second)
			}
			before, err := repository.ProductTreeIdentity(base, ".control/records")
			if err != nil {
				t.Fatal(err)
			}
			after, err := repository.ProductTreeIdentity(first.Commit, ".control/records")
			if err != nil {
				t.Fatal(err)
			}
			if before != after {
				t.Fatalf("record-only product identity changed: %s != %s", before, after)
			}
			ref := "refs/heads/result/demo"
			if err := repository.AtomicUpdateRefs([]RefOperation{
				{Kind: CreateRef, Ref: ref, NewHead: &first.Commit},
				{Kind: VerifyRef, Ref: "refs/heads/main", Expected: &base},
			}); err != nil {
				t.Fatal(err)
			}
			captured, err := repository.CaptureHeadRefs([]string{ref, "refs/heads/missing"})
			if err != nil {
				t.Fatal(err)
			}
			if captured[0].Head == nil || *captured[0].Head != first.Commit || captured[1].Head != nil {
				t.Fatalf("captured refs = %#v", captured)
			}
			wrong := base
			err = repository.AtomicUpdateRefs([]RefOperation{
				{Kind: UpdateRef, Ref: ref, NewHead: &base, Expected: &wrong},
				{Kind: VerifyRef, Ref: "refs/heads/main", Expected: &first.Commit},
			})
			if err == nil {
				t.Fatal("contention transaction unexpectedly passed")
			}
			again, err := repository.resolveHead(ref)
			if err != nil || again == nil || *again != first.Commit {
				t.Fatalf("failed transaction moved ref: %v %v", again, err)
			}
		})
	}
}

func TestCapturedReadsAndDeterministicComposition(t *testing.T) {
	for _, format := range []ObjectFormat{SHA1, SHA256} {
		format := format
		t.Run(string(format), func(t *testing.T) {
			t.Parallel()
			golden := loadGitGolden(t, format)
			repository, base := newRepository(t, format)
			if base.String() != golden.Base || len(base.String()) != golden.OIDHexLength {
				t.Fatalf("base = %s, oracle = %#v", base, golden)
			}
			timestamp, err := repository.CommitTimestamp(base)
			if err != nil {
				t.Fatal(err)
			}
			left, err := repository.PrepareRecord(RecordRequest{
				Parent: base, Changes: []BlobChange{{Path: "left.txt", Bytes: []byte("left\n")}},
				Message: "left\n", Identity: Identity{Name: "Fixture", Email: "fixture@example.invalid"}, Timestamp: timestamp + 1,
			})
			if err != nil {
				t.Fatal(err)
			}
			right, err := repository.PrepareRecord(RecordRequest{
				Parent: base, Changes: []BlobChange{{Path: "right.txt", Bytes: []byte("right\n")}},
				Message: "right\n", Identity: Identity{Name: "Fixture", Email: "fixture@example.invalid"}, Timestamp: timestamp + 1,
			})
			if err != nil {
				t.Fatal(err)
			}
			if left.Commit.String() != golden.Left || right.Commit.String() != golden.Right {
				t.Fatalf("fixture commits differ from oracle: %s/%s want %s/%s", left.Commit, right.Commit, golden.Left, golden.Right)
			}
			fastForward, err := repository.PrepareComposition(CompositionRequest{
				Expected: base, Candidate: left.Commit,
				Message:  "Baton exact composition of " + left.Commit.String() + " into refs/heads/result/demo\n",
				Identity: Identity{Name: "Baton Merge", Email: "merge@baton.invalid"},
			})
			if err != nil {
				t.Fatal(err)
			}
			if string(fastForward.Mode) != golden.FastForward.Mode ||
				fastForward.Commit.String() != golden.FastForward.Result {
				t.Fatalf("fast-forward = %#v, oracle = %#v", fastForward, golden.FastForward)
			}
			request := CompositionRequest{
				Expected: left.Commit, Candidate: right.Commit,
				Message:  "Baton exact composition of " + right.Commit.String() + " into refs/heads/result/demo\n",
				Identity: Identity{Name: "Baton Merge", Email: "merge@baton.invalid"},
			}
			first, err := repository.PrepareComposition(request)
			if err != nil {
				t.Fatal(err)
			}
			second, err := repository.PrepareComposition(request)
			if err != nil {
				t.Fatal(err)
			}
			if first.Mode != TwoParent || first.Commit != second.Commit ||
				first.Commit.String() != golden.TwoParent.Result ||
				!reflect.DeepEqual(first.Parents, []OID{left.Commit, right.Commit}) {
				t.Fatalf("composition = %#v / %#v, oracle = %#v", first, second, golden.TwoParent)
			}
			product, err := repository.ProductTreeIdentity(first.Commit, ".baton/releases")
			if err != nil {
				t.Fatal(err)
			}
			if product != golden.ResultProductTree {
				t.Fatalf("product identity = %s, oracle = %s", product, golden.ResultProductTree)
			}
			entries, err := repository.ListTree(first.Commit)
			if err != nil {
				t.Fatal(err)
			}
			var paths []string
			for _, entry := range entries {
				paths = append(paths, entry.Path)
			}
			if !reflect.DeepEqual(paths, []string{"left.txt", "product.txt", "right.txt"}) {
				t.Fatalf("paths = %v", paths)
			}
			if _, err := repository.ReadBlob(first.Commit, "../escape"); err == nil {
				t.Fatal("ReadBlob accepted noncanonical path")
			}
			var typed *Error
			if _, err := repository.PrepareComposition(CompositionRequest{
				Expected: first.Commit, Candidate: right.Commit, Message: "contained\n",
				Identity: request.Identity,
			}); !errors.As(err, &typed) || typed.Code != golden.ContainedOutcome {
				t.Fatalf("contained error = %#v, oracle = %s", err, golden.ContainedOutcome)
			}
		})
	}
}

func TestCompositionRejectsNestedCustomMergeDriver(t *testing.T) {
	t.Parallel()
	repository, base := newRepository(t, SHA1)
	timestamp, err := repository.CommitTimestamp(base)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := repository.PrepareRecord(RecordRequest{
		Parent: base, Changes: []BlobChange{{Path: "left.txt", Bytes: []byte("left\n")}},
		Message: "left\n", Identity: Identity{Name: "Fixture", Email: "fixture@example.invalid"}, Timestamp: timestamp + 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := repository.PrepareRecord(RecordRequest{
		Parent: base,
		Changes: []BlobChange{
			{Path: "nested/.gitattributes", Bytes: []byte("*.txt merge=hostile\n")},
			{Path: "nested/right.txt", Bytes: []byte("right\n")},
		},
		Message: "right\n", Identity: Identity{Name: "Fixture", Email: "fixture@example.invalid"}, Timestamp: timestamp + 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = repository.PrepareComposition(CompositionRequest{
		Expected: expected.Commit, Candidate: candidate.Commit, Message: "compose\n",
		Identity: Identity{Name: "Merge Engine", Email: "merge@example.invalid"},
	})
	var typed *Error
	if !errors.As(err, &typed) || typed.Code != "CUSTOM_MERGE_DRIVER" {
		t.Fatalf("custom merge driver error = %#v", err)
	}
}
