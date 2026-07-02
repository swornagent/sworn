package driver

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// forbiddenImports are the wire-type packages this contract package exists
// to keep out. A driver's own implementation wraps them; the contract stays
// provider-neutral so any driver — subprocess or in-process — can implement
// it without pulling in another driver's transport.
var forbiddenImports = []string{
	"github.com/swornagent/sworn/internal/model",
	"github.com/swornagent/sworn/internal/agent",
}

// TestNoWireImports parses every .go file in this package (including test
// files) and fails, naming the file and the import, if any forbidden
// package is imported. This is the AC-05 enforcement: a future edit that
// reintroduces the wire-type coupling this contract was built to remove
// fails the build's own test suite, not just a design review.
func TestNoWireImports(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob *.go: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no .go files found in internal/driver — glob pattern broken?")
	}

	fset := token.NewFileSet()
	for _, f := range files {
		src, err := parser.ParseFile(fset, f, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		for _, imp := range src.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			for _, forbidden := range forbiddenImports {
				if path == forbidden {
					t.Errorf("%s imports %q — internal/driver must not depend on wire-type packages (AC-05)", f, path)
				}
			}
		}
	}
}
