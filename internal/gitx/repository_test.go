package gitx

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const testGit = "/usr/bin/git"

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

func requireGitxErrorCode(t *testing.T, err error, code string) {
	t.Helper()
	var typed *Error
	if !errors.As(err, &typed) || typed.Code != code {
		t.Fatalf("error = %#v, want gitx code %s", err, code)
	}
}

func nextRecord(t *testing.T, repository *Repository, parent OID, name string) OID {
	t.Helper()
	record, product := inertAdmissions(t, repository, nil)
	prepared, err := repository.PrepareRecordTransition(RecordTransitionRequest{
		ExpectedHead: parent,
		Changes: []BlobChange{{
			Path:  ".baton/releases/fixture/" + name + ".txt",
			Bytes: []byte(name + "\n"),
		}},
		Message: name, RecordAdmission: record, ProductAdmission: product,
	})
	if err != nil {
		t.Fatal(err)
	}
	return prepared.Commit
}

func inertAdmissions(
	t *testing.T,
	repository *Repository,
	observe func(RecordRootRequest),
) (*RecordPathAdmission, *ProductExclusionAdmission) {
	t.Helper()
	record, err := repository.ResolveRecordPathAdmission()
	if err != nil {
		t.Fatal(err)
	}
	product, err := repository.ResolveProductExclusion(record, func(request RecordRootRequest) (RecordRootDecision, error) {
		if observe != nil {
			observe(request)
		}
		return RecordRootDecision{
			Kind: request.Kind, Repository: request.Repository,
			RecordRoot: request.RecordRoot, Commit: request.Commit,
			Decision: "inert",
		}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return record, product
}

func captureRefs(t *testing.T, repository *Repository, refs ...string) []RefHead {
	t.Helper()
	result, err := repository.CaptureHeadRefs(refs)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func assertQuiescentTrace(t *testing.T, trace refTrace) {
	t.Helper()
	if trace.pid <= 0 || trace.group != trace.pid || !trace.started || !trace.waited ||
		!trace.reaped || !trace.groupQuiet || !trace.locksReleased || trace.attempts != 1 {
		t.Fatalf("non-quiescent transaction trace: %#v", trace)
	}
	t.Logf("CASE PASS pid=%d group=%d waited=%t reaped=%t quiet=%t locks=%t attempts=%d",
		trace.pid, trace.group, trace.waited, trace.reaped, trace.groupQuiet, trace.locksReleased, trace.attempts)
}

func TestRC3SHA256OIDWidthsAndInheritedEnvironmentIgnored(t *testing.T) {
	for _, format := range []ObjectFormat{SHA1, SHA256} {
		format := format
		t.Run(string(format), func(t *testing.T) {
			repository, head := newRepository(t, format)
			tree, err := repository.TreeOID(head)
			if err != nil {
				t.Fatal(err)
			}
			t.Setenv("GIT_DIR", "/definitely/not/the/repository")
			t.Setenv("GIT_OBJECT_DIRECTORY", "/definitely/not/the/object-store")
			t.Setenv("GIT_CONFIG_COUNT", "1")
			t.Setenv("GIT_CONFIG_KEY_0", "core.fsmonitor")
			t.Setenv("GIT_CONFIG_VALUE_0", "malicious")
			observed, err := repository.TreeOID(head)
			if err != nil || observed != tree {
				t.Fatalf("%s inherited environment affected literal Git: %s %v", format, observed, err)
			}
			if len(head.String()) != format.oidLength() {
				t.Fatalf("%s OID width = %d", format, len(head.String()))
			}
		})
	}
	repository, head := newRepository(t, SHA1)
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

func TestRC3ExactCreateUpdateVerifyReconcileAndRetry(t *testing.T) {
	t.Parallel()
	for _, format := range []ObjectFormat{SHA1, SHA256} {
		format := format
		t.Run(string(format), func(t *testing.T) {
			t.Parallel()
			repository, base := newRepository(t, format)
			recordAdmission, productAdmission := inertAdmissions(t, repository, nil)
			request := RecordTransitionRequest{
				ExpectedHead: base,
				Changes: []BlobChange{{
					Path: ".baton/releases/demo/metadata", Bytes: []byte("metadata\n"),
				}},
				Message:         "Record fixture",
				RecordAdmission: recordAdmission, ProductAdmission: productAdmission,
			}
			first, err := repository.PrepareRecordTransition(request)
			if err != nil {
				t.Fatal(err)
			}
			second, err := repository.PrepareRecordTransition(request)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(first, second) {
				t.Fatalf("deterministic preparation differs: %#v != %#v", first, second)
			}
			before, err := repository.ProductTreeIdentity(base, productAdmission)
			if err != nil {
				t.Fatal(err)
			}
			after, err := repository.ProductTreeIdentity(first.Commit, productAdmission)
			if err != nil {
				t.Fatal(err)
			}
			if before.ProductTree != after.ProductTree {
				t.Fatalf("record-only product identity changed: %s != %s", before.ProductTree, after.ProductTree)
			}
			ref := "refs/heads/result/demo"
			if err := repository.AtomicUpdateRefs([]RefOperation{
				{Kind: CreateRef, Ref: ref, NewHead: &first.Commit},
				{Kind: VerifyRef, Ref: "refs/heads/main", Expected: &base},
			}); err != nil {
				t.Fatal(err)
			}
			if err := repository.AtomicUpdateRefs([]RefOperation{
				{Kind: CreateRef, Ref: ref, NewHead: &first.Commit},
				{Kind: VerifyRef, Ref: "refs/heads/main", Expected: &base},
			}); err != nil {
				t.Fatalf("exact create retry: %v", err)
			}
			captured, err := repository.CaptureHeadRefs([]string{ref, "refs/heads/missing"})
			if err != nil {
				t.Fatal(err)
			}
			if captured[0].Ref != "refs/heads/missing" || captured[0].State != RefAbsent ||
				captured[1].Ref != ref || captured[1].Head != first.Commit {
				t.Fatalf("captured refs = %#v", captured)
			}
			next := nextRecord(t, repository, first.Commit, "second")
			snapshot := captureRefs(t, repository, ref, "refs/heads/main", "refs/heads/missing")
			operations := []RefOperation{
				{Kind: UpdateRef, Ref: ref, NewHead: &next, Expected: &first.Commit},
				{Kind: VerifyRef, Ref: "refs/heads/main", Expected: &base},
				{Kind: VerifyRef, Ref: "refs/heads/missing"},
			}
			if err := repository.ApplyRefTransaction(snapshot, operations); err != nil {
				t.Fatal(err)
			}
			desired := captureRefs(t, repository, ref, "refs/heads/main", "refs/heads/missing")
			if err := repository.ApplyRefTransaction(desired, operations); err != nil {
				t.Fatalf("exact update/verify retry: %v", err)
			}
			wrong := base
			err = repository.AtomicUpdateRefs([]RefOperation{
				{Kind: UpdateRef, Ref: ref, NewHead: &base, Expected: &wrong},
				{Kind: VerifyRef, Ref: "refs/heads/main", Expected: &first.Commit},
			})
			if err == nil {
				t.Fatal("contention transaction unexpectedly passed")
			}
			observed, err := repository.CaptureHeadRefs([]string{ref})
			if err != nil || len(observed) != 1 || observed[0].Head != next {
				t.Fatalf("failed transaction moved ref: %v %v", observed, err)
			}
		})
	}
}

func TestRC3CapturePreservesDirectMissingAndRefusesInvalidStates(t *testing.T) {
	t.Parallel()
	for _, format := range []ObjectFormat{SHA1, SHA256} {
		format := format
		t.Run(string(format), func(t *testing.T) {
			t.Parallel()
			repository, _ := newRepository(t, format)
			blobText := runTestGit(t, repository.Root(), []byte("not a commit\n"), "hash-object", "-w", "--stdin")
			blob, err := ParseOID(format, blobText)
			if err != nil {
				t.Fatal(err)
			}
			// Git's porcelain refuses a non-commit branch, so model a corrupt
			// or externally-written loose ref directly.
			if err := os.WriteFile(
				filepath.Join(repository.Root(), ".git", "refs", "heads", "blob"),
				[]byte(blob.String()+"\n"),
				0o644,
			); err != nil {
				t.Fatal(err)
			}
			runTestGit(t, repository.Root(), nil, "symbolic-ref", "refs/heads/alias", "refs/heads/main")
			runTestGit(t, repository.Root(), nil, "symbolic-ref", "refs/heads/broken-alias", "refs/heads/missing")
			if err := os.WriteFile(
				filepath.Join(repository.Root(), ".git", "refs", "heads", "broken"),
				[]byte("not-an-object-id\n"), 0o644,
			); err != nil {
				t.Fatal(err)
			}
			captured, err := repository.CaptureHeadRefs([]string{
				"refs/heads/missing", "refs/heads/main", "refs/heads/blob",
				"refs/heads/alias", "refs/heads/broken-alias", "refs/heads/broken",
			})
			if err != nil {
				t.Fatal(err)
			}
			states := make(map[string]RefHead, len(captured))
			for _, value := range captured {
				states[value.Ref] = value
			}
			if states["refs/heads/main"].State != RefDirect ||
				states["refs/heads/main"].Head.IsZero() ||
				states["refs/heads/missing"].State != RefAbsent ||
				states["refs/heads/blob"].State != RefNonCommit ||
				states["refs/heads/alias"].State != RefSymbolic ||
				states["refs/heads/alias"].Target != "refs/heads/main" ||
				states["refs/heads/broken-alias"].State != RefSymbolic ||
				states["refs/heads/broken-alias"].Target != "refs/heads/missing" ||
				states["refs/heads/broken"].State != RefBroken {
				t.Fatalf("typed capture = %#v", states)
			}
		})
	}
}

func TestRC3TransactionInputsClosedAndBoundedBeforeEffects(t *testing.T) {
	t.Parallel()
	repository, _ := newRepository(t, SHA1)
	blobText := runTestGit(t, repository.Root(), []byte("not a commit\n"), "hash-object", "-w", "--stdin")
	blob, err := ParseOID(SHA1, blobText)
	if err != nil {
		t.Fatal(err)
	}
	attempts := 0
	repository.refFault = &refFault{observe: func(refTrace) { attempts++ }}
	err = repository.AtomicUpdateRefs([]RefOperation{{
		Kind: CreateRef, Ref: "refs/heads/result/blob", NewHead: &blob,
	}})
	requireGitxErrorCode(t, err, "NON_COMMIT_OBJECT")
	if attempts != 0 {
		t.Fatalf("invalid object started %d transaction helpers", attempts)
	}
	buffer := &boundedBuffer{limit: 8}
	if count, err := buffer.Write([]byte("0123456789")); err != nil || count != 10 ||
		!buffer.overflow || buffer.Len() != 8 {
		t.Fatalf("bounded buffer = len %d overflow %t count %d err %v", buffer.Len(), buffer.overflow, count, err)
	}
	longRef := "refs/heads/" + strings.Repeat("x", 251)
	if _, err := repository.CaptureHeadRefs([]string{longRef}); err == nil {
		t.Fatal("oversized ref reached Git")
	}
	if MaxHeadRefs*(250+2*SHA256.oidLength()+32) >= MaxRefProtocolBytes {
		t.Fatal("admitted transaction input can exceed its aggregate protocol bound")
	}
	captured, err := repository.CaptureHeadRefs([]string{"refs/heads/result/blob"})
	if err != nil {
		t.Fatal(err)
	}
	if !captured[0].Head.IsZero() {
		t.Fatalf("non-commit transaction created %s", captured[0].Head)
	}
}

func TestRC3AliasesRefuseCreateUpdateVerifyAtomically(t *testing.T) {
	tests := []struct {
		name      string
		operation func(ref string, base, next OID) RefOperation
	}{
		{
			name: "create",
			operation: func(ref string, _, next OID) RefOperation {
				return RefOperation{Kind: CreateRef, Ref: ref, NewHead: &next}
			},
		},
		{
			name: "update",
			operation: func(ref string, base, next OID) RefOperation {
				return RefOperation{Kind: UpdateRef, Ref: ref, NewHead: &next, Expected: &base}
			},
		},
		{
			name: "verify",
			operation: func(ref string, base, _ OID) RefOperation {
				return RefOperation{Kind: VerifyRef, Ref: ref, Expected: &base}
			},
		},
	}
	for _, test := range tests {
		for _, target := range []string{"refs/heads/main", "refs/heads/missing"} {
			test, target := test, target
			t.Run(test.name+"/"+filepath.Base(target), func(t *testing.T) {
				t.Parallel()
				repository, base := newRepository(t, SHA1)
				next := nextRecord(t, repository, base, "next")
				ref := "refs/heads/alias"
				runTestGit(t, repository.Root(), nil, "symbolic-ref", ref, target)
				indexBefore := runTestGit(t, repository.Root(), nil, "write-tree")
				statusBefore := runTestGit(t, repository.Root(), nil, "status", "--porcelain=v1", "--untracked-files=all")
				if err := repository.AtomicUpdateRefs([]RefOperation{test.operation(ref, base, next)}); err == nil {
					t.Fatalf("%s against %s alias passed", test.name, target)
				}
				if got := runTestGit(t, repository.Root(), nil, "symbolic-ref", "--no-recurse", ref); got != target {
					t.Fatalf("symbolic alias target = %q", got)
				}
				if got := runTestGit(t, repository.Root(), nil, "rev-parse", "refs/heads/main"); got != base.String() {
					t.Fatalf("symbolic referent moved to %s", got)
				}
				if got := runTestGit(t, repository.Root(), nil, "write-tree"); got != indexBefore {
					t.Fatalf("attached index changed: %s != %s", got, indexBefore)
				}
				if got := runTestGit(t, repository.Root(), nil, "status", "--porcelain=v1", "--untracked-files=all"); got != statusBefore {
					t.Fatalf("attached worktree changed: %q != %q", got, statusBefore)
				}
			})
		}
	}
}

func TestRC3AfterCaptureAliasRacesRefuseUnderPreparedLocks(t *testing.T) {
	for _, kind := range []RefOperationKind{CreateRef, UpdateRef, VerifyRef} {
		for _, target := range []string{"refs/heads/main", "refs/heads/missing"} {
			kind, target := kind, target
			t.Run(string(kind)+"/"+filepath.Base(target), func(t *testing.T) {
				t.Parallel()
				repository, base := newRepository(t, SHA1)
				next := nextRecord(t, repository, base, "next")
				ref := "refs/heads/race"
				var operation RefOperation
				switch kind {
				case CreateRef:
					operation = RefOperation{Kind: kind, Ref: ref, NewHead: &next}
				case UpdateRef:
					runTestGit(t, repository.Root(), nil, "update-ref", ref, base.String())
					operation = RefOperation{Kind: kind, Ref: ref, NewHead: &next, Expected: &base}
				case VerifyRef:
					runTestGit(t, repository.Root(), nil, "update-ref", ref, base.String())
					operation = RefOperation{Kind: kind, Ref: ref, Expected: &base}
				}
				snapshot := captureRefs(t, repository, ref)
				var rewriteErr error
				var rewriteOutput []byte
				var trace refTrace
				repository.refFault = &refFault{
					force: kind == VerifyRef,
					afterPrepare: func() {
						command := exec.Command(testGit, "-C", repository.Root(), "symbolic-ref", ref, target)
						rewriteOutput, rewriteErr = command.CombinedOutput()
					},
					observe: func(value refTrace) { trace = value },
				}
				if err := repository.ApplyRefTransaction(snapshot, []RefOperation{operation}); err != nil {
					t.Fatal(err)
				}
				if rewriteErr == nil || !bytes.Contains(rewriteOutput, []byte("race.lock")) {
					t.Fatalf("alias rewrite crossed lock: err=%v output=%q", rewriteErr, rewriteOutput)
				}
				expected := base
				if kind != VerifyRef {
					expected = next
				}
				observed := captureRefs(t, repository, ref)[0]
				if observed.State != RefDirect || observed.Head != expected {
					t.Fatalf("prepared result = %#v, want %s", observed, expected)
				}
				assertQuiescentTrace(t, trace)
			})
		}
	}
}

func TestRC3MixedAndInvalidReconciliationIsAmbiguousWithoutRetry(t *testing.T) {
	for _, name := range []string{
		"mixed", "third-oid", "alias", "unexpected-absence", "broken",
		"non-commit", "desired-lock-uncertainty", "pre-group-uncertainty",
	} {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			repository, base := newRepository(t, SHA1)
			next := nextRecord(t, repository, base, "next")
			third := nextRecord(t, repository, base, "third")
			blobText := runTestGit(t, repository.Root(), []byte("blob\n"), "hash-object", "-w", "--stdin")
			one, two := "refs/heads/result/one", "refs/heads/result/two"
			runTestGit(t, repository.Root(), nil, "update-ref", one, base.String())
			runTestGit(t, repository.Root(), nil, "update-ref", two, base.String())
			snapshot := captureRefs(t, repository, one, two)
			operations := []RefOperation{
				{Kind: UpdateRef, Ref: one, NewHead: &next, Expected: &base},
				{Kind: UpdateRef, Ref: two, NewHead: &next, Expected: &base},
			}
			var trace refTrace
			fault := &refFault{failStage: "inspection", observe: func(value refTrace) { trace = value }}
			switch name {
			case "mixed":
				fault.afterProcess = func() { runTestGit(t, repository.Root(), nil, "update-ref", one, next.String()) }
			case "third-oid":
				fault.afterProcess = func() { runTestGit(t, repository.Root(), nil, "update-ref", one, third.String()) }
			case "alias":
				fault.afterProcess = func() { runTestGit(t, repository.Root(), nil, "symbolic-ref", one, "refs/heads/main") }
			case "unexpected-absence":
				fault.afterProcess = func() { runTestGit(t, repository.Root(), nil, "update-ref", "-d", one) }
			case "broken":
				fault.afterProcess = func() {
					if err := os.WriteFile(filepath.Join(repository.commonDir, filepath.FromSlash(one)), []byte("broken\n"), 0o644); err != nil {
						t.Fatal(err)
					}
				}
			case "non-commit":
				fault.afterProcess = func() {
					if err := os.WriteFile(filepath.Join(repository.commonDir, filepath.FromSlash(one)), []byte(blobText+"\n"), 0o644); err != nil {
						t.Fatal(err)
					}
				}
			case "desired-lock-uncertainty":
				fault.failStage, fault.uncertain = "", "lock"
			case "pre-group-uncertainty":
				fault.uncertain = "group"
			}
			repository.refFault = fault
			err := repository.ApplyRefTransaction(snapshot, operations)
			requireGitxErrorCode(t, err, "REF_TRANSACTION_RECOVERY_REQUIRED")
			if trace.attempts != 1 {
				t.Fatalf("ambiguous outcome attempted %d helpers: %#v", trace.attempts, trace)
			}
			if !strings.Contains(name, "uncertainty") {
				assertQuiescentTrace(t, trace)
			}
			t.Logf("CASE %s PASS attempts=%d", name, trace.attempts)
		})
	}
}

func TestRC3PreCommitFaultsAbortWaitReapAndReleaseLocks(t *testing.T) {
	for _, fault := range []string{
		"timeout", "sigkill", "early-exit", "missing-ack", "malformed-ack",
		"extra-ack", "inspection", "stdout-overflow", "stderr-overflow",
	} {
		fault := fault
		t.Run(fault, func(t *testing.T) {
			t.Parallel()
			repository, base := newRepository(t, SHA1)
			next := nextRecord(t, repository, base, "next")
			ref := "refs/heads/result/fault"
			runTestGit(t, repository.Root(), nil, "update-ref", ref, base.String())
			snapshot := captureRefs(t, repository, ref)
			operation := RefOperation{Kind: UpdateRef, Ref: ref, NewHead: &next, Expected: &base}
			var trace refTrace
			repository.refFault = &refFault{failStage: fault, observe: func(value refTrace) { trace = value }}
			err := repository.ApplyRefTransaction(snapshot, []RefOperation{operation})
			requireGitxErrorCode(t, err, "REF_TRANSACTION_NOT_APPLIED")
			assertQuiescentTrace(t, trace)
			observed := captureRefs(t, repository, ref)[0]
			if observed.Head != base {
				t.Fatalf("%s fault changed pre-vector: %#v", fault, observed)
			}
			repository.refFault = nil
			if err := repository.ApplyRefTransaction(captureRefs(t, repository, ref), []RefOperation{operation}); err != nil {
				t.Fatalf("%s follow-on fresh CAS: %v", fault, err)
			}
		})
	}
}

func TestRC3PostCommitAndInertCleanupFaultsReconcileAfterQuiescence(t *testing.T) {
	tests := []struct {
		name, fault string
		cleanup     bool
	}{
		{name: "nonzero-exit", fault: "nonzero-exit"},
		{name: "signal", fault: "signal"},
		{name: "timeout", fault: "post-timeout"},
		{name: "extra-output", fault: "post-extra"},
		{name: "oversized-output", fault: "post-overflow"},
		{name: "parser-failure", fault: "parser-failure"},
		{name: "ack-loss", fault: "ack-loss"},
		{name: "inert-cleanup", cleanup: true},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			repository, base := newRepository(t, SHA1)
			next := nextRecord(t, repository, base, "next")
			ref := "refs/heads/result/post"
			runTestGit(t, repository.Root(), nil, "update-ref", ref, base.String())
			beforeLog := strings.Fields(runTestGit(t, repository.Root(), nil, "reflog", "show", "--format=%H", ref))
			operation := RefOperation{Kind: UpdateRef, Ref: ref, NewHead: &next, Expected: &base}
			var trace refTrace
			repository.refFault = &refFault{
				failStage: test.fault, cleanup: test.cleanup,
				observe: func(value refTrace) { trace = value },
			}
			if err := repository.ApplyRefTransaction(captureRefs(t, repository, ref), []RefOperation{operation}); err != nil {
				t.Fatal(err)
			}
			assertQuiescentTrace(t, trace)
			afterLog := strings.Fields(runTestGit(t, repository.Root(), nil, "reflog", "show", "--format=%H", ref))
			repository.refFault = &refFault{}
			if err := repository.ApplyRefTransaction(captureRefs(t, repository, ref), []RefOperation{operation}); err != nil {
				t.Fatal(err)
			}
			retryLog := strings.Fields(runTestGit(t, repository.Root(), nil, "reflog", "show", "--format=%H", ref))
			if len(afterLog) != len(beforeLog)+1 || len(retryLog) != len(afterLog) {
				t.Fatalf("reflog mutation counts before=%d after=%d retry=%d", len(beforeLog), len(afterLog), len(retryLog))
			}
		})
	}
}

func TestRC3PureVerifySucceedsAfterLostCommitAcknowledgement(t *testing.T) {
	repository, base := newRepository(t, SHA1)
	ref := "refs/heads/main"
	beforeLog := runTestGit(t, repository.Root(), nil, "reflog", "show", "--format=%H", ref)
	var trace refTrace
	repository.refFault = &refFault{
		force: true, failStage: "ack-loss", observe: func(value refTrace) { trace = value },
	}
	if err := repository.ApplyRefTransaction(captureRefs(t, repository, ref), []RefOperation{{
		Kind: VerifyRef, Ref: ref, Expected: &base,
	}}); err != nil {
		t.Fatal(err)
	}
	assertQuiescentTrace(t, trace)
	if afterLog := runTestGit(t, repository.Root(), nil, "reflog", "show", "--format=%H", ref); afterLog != beforeLog {
		t.Fatalf("pure verify changed reflog: %q != %q", afterLog, beforeLog)
	}
}

func TestRC3AllPreReconciliationIsSnapshotScopedWithoutABAClaim(t *testing.T) {
	repository, base := newRepository(t, SHA1)
	next := nextRecord(t, repository, base, "next")
	ref := "refs/heads/result/aba"
	runTestGit(t, repository.Root(), nil, "update-ref", ref, base.String())
	before := strings.Fields(runTestGit(t, repository.Root(), nil, "reflog", "show", "--format=%H", ref))
	var trace refTrace
	repository.refFault = &refFault{
		failStage: "inspection",
		afterProcess: func() {
			runTestGit(t, repository.Root(), nil, "update-ref", ref, next.String())
			runTestGit(t, repository.Root(), nil, "update-ref", ref, base.String())
		},
		observe: func(value refTrace) { trace = value },
	}
	err := repository.ApplyRefTransaction(captureRefs(t, repository, ref), []RefOperation{{
		Kind: UpdateRef, Ref: ref, NewHead: &next, Expected: &base,
	}})
	requireGitxErrorCode(t, err, "REF_TRANSACTION_NOT_APPLIED")
	if !strings.Contains(err.Error(), "ABA") {
		t.Fatalf("all-pre result concealed ABA limitation: %v", err)
	}
	assertQuiescentTrace(t, trace)
	after := strings.Fields(runTestGit(t, repository.Root(), nil, "reflog", "show", "--format=%H", ref))
	if len(after) != len(before)+2 || captureRefs(t, repository, ref)[0].Head != base {
		t.Fatalf("ABA fixture/ref = %d -> %d / %#v", len(before), len(after), captureRefs(t, repository, ref))
	}
}

func TestCapturedReadsAndDeterministicComposition(t *testing.T) {
	for _, format := range []ObjectFormat{SHA1, SHA256} {
		format := format
		t.Run(string(format), func(t *testing.T) {
			t.Parallel()
			repository, base := newRepository(t, format)
			_, productAdmission := inertAdmissions(t, repository, nil)
			left := prepareProduct(t, repository, base, []BlobChange{{Path: "left.txt", Bytes: []byte("left\n")}}, "left")
			right := prepareProduct(t, repository, base, []BlobChange{{Path: "right.txt", Bytes: []byte("right\n")}}, "right")
			fastForward, err := repository.PrepareComposition(CompositionRequest{
				Expected: base, Candidate: left.Commit, TargetRef: "refs/heads/result/demo",
				ProductAdmission: productAdmission,
			})
			if err != nil {
				t.Fatal(err)
			}
			if fastForward.Mode != FastForward || fastForward.Commit != left.Commit {
				t.Fatalf("fast-forward = %#v", fastForward)
			}
			request := CompositionRequest{
				Expected: left.Commit, Candidate: right.Commit, TargetRef: "refs/heads/result/demo",
				ProductAdmission: productAdmission,
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
				!reflect.DeepEqual(first.Parents, []OID{left.Commit, right.Commit}) {
				t.Fatalf("composition = %#v / %#v", first, second)
			}
			if err := repository.VerifyExactComposition(left.Commit, right.Commit, first.Commit); err != nil {
				t.Fatal(err)
			}
			product, err := repository.ProductTreeIdentity(first.Commit, productAdmission)
			if err != nil {
				t.Fatal(err)
			}
			if product.ProductTree == "" || len(product.Entries) != 3 {
				t.Fatalf("product identity = %#v", product)
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
				Expected: first.Commit, Candidate: right.Commit,
				TargetRef: "refs/heads/result/demo", ProductAdmission: productAdmission,
			}); !errors.As(err, &typed) || typed.Code != "CANDIDATE_ALREADY_CONTAINED" {
				t.Fatalf("contained error = %#v", err)
			}
			history, err := repository.ReadFirstParentHistory(first.Commit, 3)
			if err != nil {
				t.Fatal(err)
			}
			if len(history) != 3 || history[0].OID != first.Commit ||
				len(history[0].Parents) != 2 || history[1].OID != left.Commit {
				t.Fatalf("first-parent history = %#v", history)
			}
		})
	}
}

func TestCompositionRejectsNestedCustomMergeDriver(t *testing.T) {
	t.Parallel()
	repository, base := newRepository(t, SHA1)
	_, productAdmission := inertAdmissions(t, repository, nil)
	expected := prepareProduct(t, repository, base, []BlobChange{
		{Path: "left.txt", Bytes: []byte("left\n")},
	}, "left")
	candidate := prepareProduct(
		t, repository, base,
		[]BlobChange{
			{Path: "nested/.gitattributes", Bytes: []byte("*.txt merge=hostile\n")},
			{Path: "nested/right.txt", Bytes: []byte("right\n")},
		},
		"right",
	)
	_, err := repository.PrepareComposition(CompositionRequest{
		Expected: expected.Commit, Candidate: candidate.Commit,
		TargetRef: "refs/heads/result/demo", ProductAdmission: productAdmission,
	})
	var typed *Error
	if !errors.As(err, &typed) || typed.Code != "CUSTOM_MERGE_DRIVER" {
		t.Fatalf("custom merge driver error = %#v", err)
	}
}

func prepareProduct(
	t *testing.T,
	repository *Repository,
	parent OID,
	changes []BlobChange,
	message string,
) PreparedCommit {
	t.Helper()
	timestamp, err := repository.CommitTimestamp(parent)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := repository.prepareRecord(RecordRequest{
		Parent: parent, Changes: changes, Message: message + "\n",
		Identity:  Identity{Name: "Fixture", Email: "fixture@example.invalid"},
		Timestamp: timestamp + 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	return prepared
}

func TestProductExclusionPolicyIsExactCachedAndFailClosed(t *testing.T) {
	t.Parallel()
	repository, base := newRepository(t, SHA1)
	var requests []RecordRootRequest
	_, product := inertAdmissions(t, repository, func(request RecordRootRequest) {
		requests = append(requests, request)
	})
	first, err := repository.ProductTreeIdentity(base, product)
	if err != nil {
		t.Fatal(err)
	}
	second, err := repository.ProductTreeIdentity(base, product)
	if err != nil {
		t.Fatal(err)
	}
	if first.ProductTree != second.ProductTree || len(requests) != 1 ||
		requests[0].Commit != base.String() || requests[0].Repository != repository.Root() ||
		requests[0].RecordRoot != recordRoot {
		t.Fatalf("cached exact policy = %#v / %#v / %#v", first, second, requests)
	}

	record, err := repository.ResolveRecordPathAdmission()
	if err != nil {
		t.Fatal(err)
	}
	consumed, err := repository.ResolveProductExclusion(record, func(request RecordRootRequest) (RecordRootDecision, error) {
		return RecordRootDecision{
			Kind: request.Kind, Repository: request.Repository,
			RecordRoot: request.RecordRoot, Commit: request.Commit,
			Decision: "consumed",
		}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ProductTreeIdentity(base, consumed); err == nil {
		t.Fatal("behaviorally consumed record root was excluded")
	} else {
		requireGitxErrorCode(t, err, "RECORD_ROOT_CONSUMED")
	}
	foreign, foreignBase := newRepository(t, SHA1)
	if _, err := foreign.ProductTreeIdentity(foreignBase, product); err == nil {
		t.Fatal("cross-repository admission was accepted")
	} else {
		requireGitxErrorCode(t, err, "RECORD_ROOT_ADMISSION_MISMATCH")
	}
}

func TestRecordAndMetadataTransitionsAreProductInert(t *testing.T) {
	t.Parallel()
	repository, base := newRepository(t, SHA1)
	record, product := inertAdmissions(t, repository, nil)
	prepared, err := repository.PrepareRecordTransition(RecordTransitionRequest{
		ExpectedHead: base,
		Changes: []BlobChange{{
			Path: ".baton/releases/demo/plan.md", Bytes: []byte("plan\n"),
		}},
		Message: "record plan", RecordAdmission: record, ProductAdmission: product,
	})
	if err != nil {
		t.Fatal(err)
	}
	before, err := repository.ProductTreeIdentity(base, product)
	if err != nil {
		t.Fatal(err)
	}
	after, err := repository.ProductTreeIdentity(prepared.Commit, product)
	if err != nil {
		t.Fatal(err)
	}
	if before.ProductTree != after.ProductTree {
		t.Fatalf("record transition changed product: %s != %s", before.ProductTree, after.ProductTree)
	}
	metadata, err := repository.PrepareMetadataCommit(MetadataRequest{
		ExpectedHead: prepared.Commit, Message: []byte("metadata-only\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Tree != prepared.Tree || !reflect.DeepEqual(metadata.Parents, []OID{prepared.Commit}) {
		t.Fatalf("metadata transition = %#v", metadata)
	}
	if _, err := repository.PrepareRecordTransition(RecordTransitionRequest{
		ExpectedHead: base,
		Changes:      []BlobChange{{Path: "product.txt", Bytes: []byte("changed\n")}},
		Message:      "escape", RecordAdmission: record, ProductAdmission: product,
	}); err == nil {
		t.Fatal("record transition admitted a product path")
	} else {
		requireGitxErrorCode(t, err, "NON_RECORD_CHANGE")
	}
}

func TestCandidateRecordRootMustMatchExactBase(t *testing.T) {
	for _, format := range []ObjectFormat{SHA1, SHA256} {
		format := format
		t.Run(string(format), func(t *testing.T) {
			repository, base := newRepository(t, format)
			root := repository.Root()

			if err := os.WriteFile(filepath.Join(root, "product.txt"), []byte("candidate\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			runTestGit(t, root, nil, "add", "--", "product.txt")
			runTestGit(t, root, nil, "commit", "--quiet", "-m", "product candidate")
			productCandidate, err := ParseOID(
				format,
				runTestGit(t, root, nil, "rev-parse", "HEAD"),
			)
			if err != nil {
				t.Fatal(err)
			}
			if err := repository.AssertCandidateRecordRootUnchanged(base, productCandidate); err != nil {
				t.Fatalf("product-only candidate was rejected: %v", err)
			}

			recordPath := filepath.Join(root, ".baton", "releases", "demo", "plan.md")
			if err := os.MkdirAll(filepath.Dir(recordPath), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(recordPath, []byte("reserved\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			runTestGit(t, root, nil, "add", "--", ".baton/releases/demo/plan.md")
			runTestGit(t, root, nil, "commit", "--quiet", "-m", "reserved root change")
			recordCandidate, err := ParseOID(
				format,
				runTestGit(t, root, nil, "rev-parse", "HEAD"),
			)
			if err != nil {
				t.Fatal(err)
			}
			err = repository.AssertCandidateRecordRootUnchanged(base, recordCandidate)
			requireGitxErrorCode(t, err, "RESERVED_RECORD_ROOT_CHANGED")
		})
	}
}

func TestRecordRootDiffOutputHasPrivateEightMiBBound(t *testing.T) {
	if err := validateRecordRootDiffOutput(make([]byte, maxRecordRootDiffBytes)); err != nil {
		t.Fatalf("exact private bound was rejected: %v", err)
	}
	err := validateRecordRootDiffOutput(make([]byte, maxRecordRootDiffBytes+1))
	requireGitxErrorCode(t, err, "RECORD_TREE_INVENTORY_LIMIT")
	if MaxTreeBytes != 64*1024*1024 {
		t.Fatalf("whole-tree bound changed to %d", MaxTreeBytes)
	}
}
