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
	for _, prefix := range []string{"", "/abs", "trailing/", "a//b", "a/./b", "a/../b", "*", `a\\b`, "   "} {
		if err := (Scope{Include: []string{prefix}}).Validate(); err == nil {
			t.Errorf("prefix %q was accepted", prefix)
		}
	}
	if err := (Scope{Include: []string{"src"}, Exclude: []string{"src"}}).Validate(); err == nil {
		t.Fatal("duplicate prefix was accepted")
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
