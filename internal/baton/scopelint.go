package baton

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/swornagent/sworn/internal/gitx"
)

// FixturePin declares a deterministic relationship where a tooling package
// generates or manages committed test fixtures pinning another package.
type FixturePin struct {
	ToolPackage   string `json:"tool_package"`
	PinnedPackage string `json:"pinned_package"`
	FixturePath   string `json:"fixture_path"`
}

// DeclaredFixturePins defines the known deterministic fixture-tooling relationships
// across the repository.
var DeclaredFixturePins = []FixturePin{
	{
		ToolPackage:   "tools/batongolden",
		PinnedPackage: "internal/baton",
		FixturePath:   "tools/batongolden/testdata/corpus",
	},
}

// PackageInfo holds the analyzed dependency and import facts for one package.
type PackageInfo struct {
	Path        string
	AllImports  []string          // internal packages imported by all .go files in this package (prod + test)
	ProdImports []string          // internal packages imported by production .go files
	TestDeps    map[string]bool   // all internal packages required by the test binary (direct + transitive prod)
	DepPaths    map[string]string // maps each dependency in TestDeps to the direct import that pulled it in
}

// PackageGraph represents the module-wide package import graph and closure facts.
type PackageGraph struct {
	Module   string
	Modules  map[string]string
	Packages map[string]*PackageInfo
}

func hasNoiseSegment(p string) bool {
	clean := path.Clean(filepath.ToSlash(p))
	for _, seg := range strings.Split(clean, "/") {
		if seg == "." || seg == "" {
			continue
		}
		if seg == "testdata" || seg == "test" || strings.HasPrefix(seg, ".") {
			return true
		}
	}
	return false
}

// isCandidateSourcePath filters repository paths to candidate .go files,
// excluding noise segments (testdata, test, hidden directories) at any depth.
func isCandidateSourcePath(p string) bool {
	clean := path.Clean(filepath.ToSlash(p))
	if !strings.HasSuffix(clean, ".go") {
		return false
	}
	return !hasNoiseSegment(clean)
}

func governingModuleForPath(modules map[string]string, p string) (string, string, bool) {
	curr := path.Clean(filepath.ToSlash(p))
	for {
		if modPath, ok := modules[curr]; ok {
			return curr, modPath, true
		}
		if curr == "." || curr == "/" || curr == "" {
			break
		}
		curr = path.Dir(curr)
	}
	return "", "", false
}

// BuildPackageGraphFS computes the package import graph from an fs.FS abstraction.
func BuildPackageGraphFS(sys fs.FS) (*PackageGraph, error) {
	modules := make(map[string]string)
	files := make(map[string][]byte)
	err := fs.WalkDir(sys, ".", func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if p == "." {
				return nil
			}
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "testdata" || name == "test" {
				return filepath.SkipDir
			}
			return nil
		}
		cleanPath := filepath.ToSlash(filepath.Clean(p))
		if path.Base(cleanPath) == "go.mod" && !hasNoiseSegment(cleanPath) {
			if modBytes, err := fs.ReadFile(sys, p); err == nil {
				if mod := parseModulePath(modBytes); mod != "" {
					dir := path.Dir(cleanPath)
					modules[dir] = mod
				}
			}
			return nil
		}
		if !isCandidateSourcePath(cleanPath) {
			return nil
		}
		body, err := fs.ReadFile(sys, p)
		if err != nil {
			return err
		}
		files[cleanPath] = body
		return nil
	})
	if err != nil {
		return nil, recordWrap("SCOPE_LINT_FAILED", "walk module files", err)
	}

	return computePackageGraph(modules, files)
}

// BuildPackageGraphAt computes the package import graph from an admitted GitRepository at commit.
func BuildPackageGraphAt(gitRepo GitRepository, commit string) (*PackageGraph, error) {
	if gitRepo.value == nil {
		return nil, recordFail("INVALID_REPOSITORY", "one admitted Git repository is required")
	}
	oid, err := gitx.ParseOID(gitRepo.value.ObjectFormat(), commit)
	if err != nil {
		return nil, translateGitError("parse commit identity", err)
	}

	entries, err := gitRepo.value.ListTree(oid)
	if err != nil {
		return nil, translateGitError("inventory tree", err)
	}

	modules := make(map[string]string)
	var modFiles []string
	var goFiles []string
	for _, entry := range entries {
		if entry.Type != "blob" {
			continue
		}
		cleanPath := path.Clean(entry.Path)
		if hasNoiseSegment(cleanPath) {
			continue
		}
		if path.Base(cleanPath) == "go.mod" {
			modFiles = append(modFiles, cleanPath)
			continue
		}
		if isCandidateSourcePath(cleanPath) {
			goFiles = append(goFiles, cleanPath)
		}
	}

	for _, modPath := range modFiles {
		if modBytes, err := gitRepo.value.ReadBlob(oid, modPath); err == nil {
			if mod := parseModulePath(modBytes); mod != "" {
				dir := path.Dir(modPath)
				modules[dir] = mod
			}
		}
	}

	files := make(map[string][]byte, len(goFiles))
	const batchSize = 1000
	for i := 0; i < len(goFiles); i += batchSize {
		end := i + batchSize
		if end > len(goFiles) {
			end = len(goFiles)
		}
		chunk := goFiles[i:end]
		blobs, err := gitRepo.value.ReadBlobs(oid, chunk)
		if err != nil {
			return nil, translateGitError("read go files", err)
		}
		for k, v := range blobs {
			files[k] = v
		}
	}

	return computePackageGraph(modules, files)
}

func parseModulePath(content []byte) string {
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module"))
		}
	}
	return ""
}

func computePackageGraph(modules map[string]string, files map[string][]byte) (*PackageGraph, error) {
	fset := token.NewFileSet()
	packages := make(map[string]*PackageInfo)

	for filePath, content := range files {
		dir := path.Dir(filePath)
		isTest := strings.HasSuffix(path.Base(filePath), "_test.go")

		file, err := parser.ParseFile(fset, filePath, content, parser.ImportsOnly)
		if err != nil {
			return nil, recordWrap("SCOPE_LINT_FAILED", "parse go file "+filePath, err)
		}

		info, ok := packages[dir]
		if !ok {
			info = &PackageInfo{
				Path: dir,
			}
			packages[dir] = info
		}

		modDir, modPath, hasMod := governingModuleForPath(modules, dir)
		if !hasMod {
			continue
		}

		for _, spec := range file.Imports {
			if spec.Path == nil {
				continue
			}
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				continue
			}
			var internalPkg string
			if importPath == modPath {
				internalPkg = modDir
			} else if strings.HasPrefix(importPath, modPath+"/") {
				suffix := strings.TrimPrefix(importPath, modPath+"/")
				if modDir == "." {
					internalPkg = suffix
				} else {
					internalPkg = path.Join(modDir, suffix)
				}
			} else {
				continue
			}

			info.AllImports = unique(append(info.AllImports, internalPkg))
			if !isTest {
				info.ProdImports = unique(append(info.ProdImports, internalPkg))
			}
		}
	}

	// Compute transitive production closures for each package.
	prodClosure := make(map[string]map[string]string)
	for p := range packages {
		visited := make(map[string]string)
		var dfs func(curr, via string)
		dfs = func(curr, via string) {
			info := packages[curr]
			if info == nil {
				return
			}
			for _, imp := range info.ProdImports {
				if _, seen := visited[imp]; !seen && imp != p {
					nextVia := via
					if nextVia == "" {
						nextVia = imp
					}
					visited[imp] = nextVia
					dfs(imp, nextVia)
				}
			}
		}
		dfs(p, "")
		prodClosure[p] = visited
	}

	// Compute TestDeps for each package: direct imports + transitive prod imports.
	for _, info := range packages {
		info.TestDeps = make(map[string]bool)
		info.DepPaths = make(map[string]string)

		for _, direct := range info.AllImports {
			info.TestDeps[direct] = true
			info.DepPaths[direct] = direct

			for trans := range prodClosure[direct] {
				info.TestDeps[trans] = true
				if _, exists := info.DepPaths[trans]; !exists {
					info.DepPaths[trans] = direct
				}
			}
		}
	}

	return &PackageGraph{
		Module:   modules["."],
		Modules:  modules,
		Packages: packages,
	}, nil
}

func packageInScope(pkg string, scope Scope) bool {
	included := false
	for _, inc := range scope.Include {
		if pkg == inc || strings.HasPrefix(pkg, inc+"/") {
			included = true
			break
		}
	}
	if !included {
		return false
	}
	for _, exc := range scope.Exclude {
		if pkg == exc || strings.HasPrefix(pkg, exc+"/") {
			return false
		}
	}
	return true
}

func isWaived(pkg string, waivers []ScopeWaiver) bool {
	for _, w := range waivers {
		if pkg == w.Package || strings.HasPrefix(pkg, w.Package+"/") {
			return true
		}
	}
	return false
}

// LintSlice evaluates the reverse-dependency closure and fixture tooling of one Slice.
func (graph *PackageGraph) LintSlice(slice Slice) error {
	if graph == nil {
		return recordFail("INVALID_GRAPH", "package graph is required")
	}
	if len(graph.Modules) == 0 {
		return nil
	}

	var ungoverned []string
	for _, inc := range slice.Scope.Include {
		if _, _, ok := governingModuleForPath(graph.Modules, inc); !ok {
			ungoverned = append(ungoverned, inc)
		}
	}
	if len(ungoverned) > 0 {
		sort.Strings(ungoverned)
		return &RecordError{
			Code:       "SCOPE_LINT_UNRESOLVED",
			Msg:        fmt.Sprintf("no go.mod governs scoped path %s", strings.Join(ungoverned, ", ")),
			Paths:      ungoverned,
			TotalPaths: len(ungoverned),
		}
	}

	type depFinding struct {
		pkg     string
		reasons []string
	}
	var findings []depFinding

	for pkgPath, info := range graph.Packages {
		if packageInScope(pkgPath, slice.Scope) {
			continue
		}

		var reasons []string

		// Check test binary import dependencies
		for depPkg := range info.TestDeps {
			if packageInScope(depPkg, slice.Scope) {
				via := info.DepPaths[depPkg]
				if via == depPkg {
					reasons = append(reasons, fmt.Sprintf("test imports %s", depPkg))
				} else {
					reasons = append(reasons, fmt.Sprintf("test imports %s (via %s)", depPkg, via))
				}
			}
		}

		// Check declared fixture tooling pins
		for _, pin := range DeclaredFixturePins {
			toolMatches := (pkgPath == pin.ToolPackage || strings.HasPrefix(pkgPath, pin.ToolPackage+"/"))
			if toolMatches && packageInScope(pin.PinnedPackage, slice.Scope) {
				reasons = append(reasons, fmt.Sprintf("fixture tooling pins %s (%s)", pin.PinnedPackage, pin.FixturePath))
			}
		}

		if len(reasons) == 0 {
			continue
		}

		if isWaived(pkgPath, slice.Scope.Waivers) {
			continue
		}

		sort.Strings(reasons)
		findings = append(findings, depFinding{
			pkg:     pkgPath,
			reasons: unique(reasons),
		})
	}

	if len(findings) == 0 {
		return nil
	}

	sort.Slice(findings, func(i, j int) bool {
		return findings[i].pkg < findings[j].pkg
	})

	missingPaths := make([]string, len(findings))
	missingDetails := make([]string, len(findings))
	for i, f := range findings {
		missingPaths[i] = f.pkg
		missingDetails[i] = fmt.Sprintf("%s (%s)", f.pkg, strings.Join(f.reasons, ", "))
	}

	return &RecordError{
		Code:       "UNDER_DERIVED_SCOPE",
		Msg:        fmt.Sprintf("slice %s scope is under-derived: %s", slice.ID, strings.Join(missingDetails, "; ")),
		Paths:      missingPaths,
		TotalPaths: len(missingPaths),
	}
}

// LintPlan validates all slices in plan against the package graph.
func (graph *PackageGraph) LintPlan(plan Plan) error {
	metadata := plan.Metadata()
	for _, track := range metadata.Tracks {
		for _, slice := range track.Slices {
			if err := graph.LintSlice(slice); err != nil {
				return err
			}
		}
	}
	return nil
}

// ValidatePlanScopeLintFS validates plan scope reverse dependencies against an fs.FS.
func ValidatePlanScopeLintFS(sys fs.FS, plan Plan) error {
	graph, err := BuildPackageGraphFS(sys)
	if err != nil {
		return err
	}
	return graph.LintPlan(plan)
}

// ValidatePlanScopeLintAt validates plan scope reverse dependencies against an admitted GitRepository at commit.
func ValidatePlanScopeLintAt(gitRepo GitRepository, plan Plan, commit string) error {
	graph, err := BuildPackageGraphAt(gitRepo, commit)
	if err != nil {
		return err
	}
	return graph.LintPlan(plan)
}

// ValidatePlanScopeLint validates plan scope reverse dependencies using an engine repository handle at commit.
func ValidatePlanScopeLint(repository *repository, plan Plan, commit string) error {
	if repository == nil || repository.git == nil {
		return recordFail("INVALID_REPOSITORY", "one admitted Git repository is required")
	}
	return ValidatePlanScopeLintAt(GitRepository{value: repository.git}, plan, commit)
}

// ValidateSliceScopeLint validates a single slice against an existing package graph.
func ValidateSliceScopeLint(graph *PackageGraph, slice Slice) error {
	if graph == nil {
		return recordFail("INVALID_GRAPH", "package graph is required")
	}
	return graph.LintSlice(slice)
}
