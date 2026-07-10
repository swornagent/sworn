package run

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// wireTypes are the four internal/model wire types AC-04 bans from the
// orchestration path. The boundary is deliberately the TYPE list, not the
// package: internal/run and internal/verify still legitimately reference
// non-wire model identifiers (the model.Verifier interface,
// model.ProviderConfigFromEnv) — a plain import ban would outlaw those, and
// an ImportsOnly scan would miss a wire type smuggled in through an
// otherwise-legal import (S06 design D8).
var wireTypes = map[string]bool{
	"ChatMessage":  true,
	"ToolDef":      true,
	"ChatResponse": true,
	"ToolCall":     true,
}

// wirePackage is the import path whose selectors are scanned.
const wirePackage = "github.com/swornagent/sworn/internal/model"

// scannedPackages are the orchestration-path packages AC-04 covers,
// relative to this test file's directory (internal/run). cmd/sworn was
// added by S07 AC-02: the retired newAgentFromModel/newVerifierFromModel
// factory helpers were deleted from cmd/sworn/run.go by S06, and this
// extends the import-boundary net to cmd/sworn's loop wiring so no future
// edit can reintroduce a wire-type-holding construction path there.
var scannedPackages = []string{".", "../verify", "../scheduler", "../../cmd/sworn"}

// TestNoWireImports parses every .go file — INCLUDING _test.go files — in
// internal/run, internal/verify, internal/scheduler, and cmd/sworn (S07
// AC-02) and fails, naming the package, file, and identifier, on any
// selector expression <alias>.<WireType> where <alias> binds to an
// internal/model import (AC-04). After the S06 rewire every model dispatch
// crosses the driver.Dispatch seam; wire formats are a driver implementation
// detail the orchestration path can never see again.
func TestNoWireImports(t *testing.T) {
	for _, dir := range scannedPackages {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatalf("glob %s/*.go: %v", dir, err)
		}
		if len(files) == 0 {
			t.Fatalf("no .go files found in %s — glob pattern broken?", dir)
		}

		fset := token.NewFileSet()
		for _, f := range files {
			src, err := parser.ParseFile(fset, f, nil, 0)
			if err != nil {
				t.Fatalf("parse %s: %v", f, err)
			}

			// Collect the local alias(es) bound to the wire package in
			// this file. Default alias is the package name ("model").
			aliases := map[string]bool{}
			for _, imp := range src.Imports {
				path := strings.Trim(imp.Path.Value, `"`)
				if path != wirePackage {
					continue
				}
				alias := "model"
				if imp.Name != nil {
					alias = imp.Name.Name
				}
				aliases[alias] = true
			}
			if len(aliases) == 0 {
				continue
			}

			ast.Inspect(src, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				ident, ok := sel.X.(*ast.Ident)
				if !ok {
					return true
				}
				if aliases[ident.Name] && wireTypes[sel.Sel.Name] {
					pos := fset.Position(sel.Pos())
					t.Errorf("package %s: %s:%d references wire type %s.%s — internal/run, internal/verify, and internal/scheduler must not use internal/model wire types (S06 AC-04); dispatch through driver.Dispatch instead",
						dir, pos.Filename, pos.Line, ident.Name, sel.Sel.Name)
				}
				return true
			})
		}
	}
}
