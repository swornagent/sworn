package repo

import (
	"errors"
	"reflect"
	"testing"
)

func TestScopeUsesLiteralPrefixSemantics(t *testing.T) {
	scope := Scope{Include: []string{"src", "literal[abc]"}, Exclude: []string{"src/private"}}
	if err := scope.Validate(); err == nil {
		t.Fatal("glob metacharacters must be rejected")
	}

	scope = Scope{Include: []string{"src", "docs/file.txt"}, Exclude: []string{"src/private"}}
	if err := scope.Validate(); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]bool{
		"src":                   true,
		"src/main.go":           true,
		"src2/main.go":          false,
		"src/private":           false,
		"src/private/secret.go": false,
		"docs/file.txt":         true,
		"docs/file.txt/child":   true,
		"docs/another-file.txt": false,
		"../outside":            false,
		"invalid\\windows-path": false,
	} {
		if got := scope.Allows(path); got != want {
			t.Errorf("Allows(%q) = %t, want %t", path, got, want)
		}
	}
}

func TestScopeRejectsInvalidAndDuplicatePrefixes(t *testing.T) {
	for _, prefix := range []string{"", "/abs", "trailing/", "a//b", "a/./b", "a/../b", "*", `a\\b`, "   ", "a\nb", "a\rb", "a\u2028b", "a\u2029b"} {
		if err := (Scope{Include: []string{prefix}}).Validate(); err == nil {
			t.Errorf("prefix %q was accepted", prefix)
		}
	}
	for _, scope := range []Scope{
		{Include: []string{"src", "src"}},
		{Include: []string{"src"}, Exclude: []string{"vendor", "vendor"}},
	} {
		if err := scope.Validate(); err == nil {
			t.Fatal("duplicate prefix within one list was accepted")
		}
	}
}

func TestScopeAllowsIncludeExcludeOverlapWithExclusionWinning(t *testing.T) {
	scope := Scope{Include: []string{"src"}, Exclude: []string{"src"}}
	if err := scope.Validate(); err != nil {
		t.Fatalf("Baton permits an include/exclude overlap: %v", err)
	}
	if scope.Allows("src/main.go") {
		t.Fatal("exclusion did not win over the overlapping inclusion")
	}
}

func TestScopeWholeRepositoryAllowsLiteralWhitespaceAndLineTerminatorPaths(t *testing.T) {
	scope := Scope{Include: []string{"."}}
	if err := scope.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"   ", "line\nbreak", "line\u2028break"} {
		if !scope.Allows(path) {
			t.Errorf("whole-repository scope rejected schema-valid Git path %q", path)
		}
	}
}

func TestOutOfScopeReportsSortedDeniedPaths(t *testing.T) {
	err := outOfScope(Scope{Include: []string{"src"}}, []string{"z.txt", "src/main.go", "a.txt"})
	var scopeErr *ScopeError
	if !errors.As(err, &scopeErr) {
		t.Fatalf("error = %v, want ScopeError", err)
	}
	if want := []string{"a.txt", "z.txt"}; !reflect.DeepEqual(scopeErr.Paths, want) {
		t.Fatalf("paths = %#v, want %#v", scopeErr.Paths, want)
	}
}
