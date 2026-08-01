package gitx

import (
	"fmt"
	"strings"
	"testing"
)

func catalogRow(ref, objectID, objectType, target string) string {
	return strings.Join([]string{ref, objectID, objectType, target}, "\t") + "\n"
}

func TestListHeadRefsUnderIsSortedAndDoesNotMutateRepository(t *testing.T) {
	t.Parallel()

	repository, head := newRepository(t, SHA1)
	prefix := "refs/heads/release-wt/"
	for _, name := range []string{"zeta", "alpha"} {
		runTestGit(t, repository.Root(), nil, "update-ref", prefix+name, head.String())
	}
	beforeStatus := runTestGit(t, repository.Root(), nil, "status", "--porcelain=v2", "--branch")
	beforeRefs := runTestGit(t, repository.Root(), nil, "for-each-ref", "--format=%(refname)%09%(objectname)")

	got, err := repository.ListHeadRefsUnder(prefix)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 ||
		got[0].Ref != prefix+"alpha" || got[1].Ref != prefix+"zeta" ||
		got[0].State != RefDirect || got[1].State != RefDirect ||
		got[0].Head != head || got[1].Head != head {
		t.Fatalf("listed refs = %#v", got)
	}
	afterStatus := runTestGit(t, repository.Root(), nil, "status", "--porcelain=v2", "--branch")
	afterRefs := runTestGit(t, repository.Root(), nil, "for-each-ref", "--format=%(refname)%09%(objectname)")
	if beforeStatus != afterStatus || beforeRefs != afterRefs {
		t.Fatalf("ref discovery mutated repository:\nstatus %q -> %q\nrefs %q -> %q", beforeStatus, afterStatus, beforeRefs, afterRefs)
	}
}

func TestHeadRefCatalogRejectsDuplicateInvalidAndOversizedOutput(t *testing.T) {
	t.Parallel()

	prefix := "refs/heads/release-wt/"
	oid := strings.Repeat("a", SHA1.oidLength())
	valid := catalogRow(prefix+"alpha", oid, "commit", "")
	cases := []struct {
		name string
		raw  string
		code string
	}{
		{name: "duplicate", raw: valid + valid, code: "INVALID_GIT_OUTPUT"},
		{
			name: "out of order",
			raw:  catalogRow(prefix+"zeta", oid, "commit", "") + valid,
			code: "INVALID_GIT_OUTPUT",
		},
		{
			name: "invalid ref",
			raw:  catalogRow(prefix+".hidden", oid, "commit", ""),
			code: "INVALID_GIT_OUTPUT",
		},
		{name: "missing newline", raw: strings.TrimSuffix(valid, "\n"), code: "INVALID_GIT_OUTPUT"},
	}
	var oversized strings.Builder
	for index := 0; index <= MaxHeadRefs; index++ {
		oversized.WriteString(catalogRow(
			fmt.Sprintf("%srelease-%03d", prefix, index),
			oid,
			"commit",
			"",
		))
	}
	cases = append(cases, struct {
		name string
		raw  string
		code string
	}{name: "resource limit", raw: oversized.String(), code: "RESOURCE_LIMIT"})

	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := parseHeadRefCatalog([]byte(test.raw), SHA1, prefix)
			requireGitxErrorCode(t, err, test.code)
		})
	}
}

func TestListHeadRefsUnderRejectsInvalidPrefix(t *testing.T) {
	t.Parallel()

	repository, _ := newRepository(t, SHA1)
	for _, prefix := range []string{
		"refs/heads/release-wt",
		"refs/tags/release-wt/",
		"refs/heads/release*/",
	} {
		if _, err := repository.ListHeadRefsUnder(prefix); err == nil {
			t.Fatalf("prefix %q was accepted", prefix)
		} else {
			requireGitxErrorCode(t, err, "INVALID_REF")
		}
	}
}
