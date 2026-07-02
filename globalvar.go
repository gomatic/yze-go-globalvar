// Package globalvar provides a go/analysis analyzer enforcing the gomatic
// immutability/DI standard: package-level state is immutable outside tests. An
// exported package-level var — a binding any importer can rebind — is reported
// at its declaration unless allow-listed; an unexported var is reported at each
// reassignment in non-test code. An unexported var never reassigned outside
// tests is either an immutable binding (a value Go cannot express as a const —
// a lookup table, a func value, a //go:embed FS) or a dependency-injection seam
// reassigned only by the package's tests; both are sanctioned gomatic patterns.
package globalvar

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	goyze "github.com/gomatic/go-yze"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
)

const (
	exportedMessage   = "exported package-level var %q is not permitted; export a constant or a constructor instead"
	reassignedMessage = "package-level var %q is reassigned outside tests; package state must be immutable (inject the dependency instead)"
)

// defaultAllow is the baked-in set of sanctioned exported package-level var
// names that are standard across the gomatic ecosystem (an analyzer's exported
// Analyzer and Registration, and the version stamped via -ldflags).
var defaultAllow = map[string]bool{
	"version":      true,
	"Analyzer":     true,
	"Registration": true,
}

// allowExtra is the configurable allow-list of additional permitted package-level
// var names, set via the -allow flag or analyzer config.
var allowExtra string

// Analyzer reports package-level vars that are exported or reassigned outside tests.
var Analyzer = newAnalyzer()

func newAnalyzer() *analysis.Analyzer {
	a := &analysis.Analyzer{
		Name:     "globalvar",
		Doc:      "reports exported package-level vars and package-level vars reassigned outside tests, which the gomatic immutability/DI standard forbids",
		Requires: []*analysis.Analyzer{inspect.Analyzer},
		Run:      run,
	}
	a.Flags.StringVar(&allowExtra, "allow", "", "comma-separated extra permitted package-level var names")
	return a
}

// Registration declares this analyzer to the yze framework.
var Registration = goyze.Registration{
	Name:       "globalvar",
	Categories: []goyze.Category{"immutability"},
	URL:        "https://docs.gomatic.dev/yze/globalvar",
	Analyzer:   Analyzer,
}

// run reports exported package-level vars at their declaration and watched
// unexported ones at each non-test reassignment.
func run(pass *analysis.Pass) (any, error) {
	allow := buildAllow(allowCSV(allowExtra))
	watched := packageVars(pass, allow)
	for _, file := range pass.Files {
		if !isTestFile(pass, file) {
			checkReassignments(pass, watched, file)
		}
	}
	return nil, nil
}

// allowCSV is the raw comma-separated -allow flag value listing extra
// permitted package-level var names.
type allowCSV string

// buildAllow merges the baked-in allow-set with the configured extras.
func buildAllow(extra allowCSV) map[string]bool {
	allow := make(map[string]bool, len(defaultAllow))
	for name := range defaultAllow {
		allow[name] = true
	}
	for _, name := range splitNonEmpty(extra) {
		allow[name] = true
	}
	return allow
}

func splitNonEmpty(value allowCSV) []string {
	if value == "" {
		return nil
	}
	return strings.Split(string(value), ",")
}

// packageVars reports exported non-allow-listed package-level vars and returns
// the set of unexported package-level var objects to watch for reassignment.
func packageVars(pass *analysis.Pass, allow map[string]bool) map[types.Object]bool {
	watched := make(map[types.Object]bool)
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			collectDecl(pass, allow, decl, watched)
		}
	}
	return watched
}

// collectDecl folds one top-level declaration into the watch set when it is a
// var block. Function-local vars are *ast.DeclStmt inside func bodies, not
// file.Decls, so this restricts the check to package scope.
func collectDecl(pass *analysis.Pass, allow map[string]bool, decl ast.Decl, watched map[types.Object]bool) {
	gen, ok := decl.(*ast.GenDecl)
	if !ok || gen.Tok != token.VAR {
		return
	}
	for _, spec := range gen.Specs {
		for _, name := range spec.(*ast.ValueSpec).Names {
			checkVarName(pass, allow, name, watched)
		}
	}
}

// checkVarName reports an exported var at its declaration and adds an
// unexported one to the watch set; the blank identifier and allow-listed names
// are skipped.
func checkVarName(pass *analysis.Pass, allow map[string]bool, name *ast.Ident, watched map[types.Object]bool) {
	if name.Name == "_" || allow[name.Name] {
		return
	}
	if name.IsExported() {
		pass.Reportf(name.Pos(), exportedMessage, name.Name)
		return
	}
	watched[pass.TypesInfo.ObjectOf(name)] = true
}

// isTestFile reports whether file is a _test.go file, where reassigning a
// package-level var is the sanctioned dependency-injection seam.
func isTestFile(pass *analysis.Pass, file *ast.File) bool {
	return strings.HasSuffix(pass.Fset.File(file.Pos()).Name(), "_test.go")
}

// checkReassignments reports each rebinding of a watched var in file: plain and
// compound assignments, and increment/decrement statements.
func checkReassignments(pass *analysis.Pass, watched map[types.Object]bool, file *ast.File) {
	ast.Inspect(file, func(n ast.Node) bool {
		switch stmt := n.(type) {
		case *ast.AssignStmt:
			if stmt.Tok != token.DEFINE {
				reportTargets(pass, watched, stmt.Lhs)
			}
		case *ast.IncDecStmt:
			reportTargets(pass, watched, []ast.Expr{stmt.X})
		}
		return true
	})
}

// reportTargets reports each assignment target that names a watched
// package-level var. A shadowing local resolves to a different object, so it is
// not reported.
func reportTargets(pass *analysis.Pass, watched map[types.Object]bool, targets []ast.Expr) {
	for _, target := range targets {
		if ident, ok := target.(*ast.Ident); ok && watched[pass.TypesInfo.ObjectOf(ident)] {
			pass.Reportf(ident.Pos(), reassignedMessage, ident.Name)
		}
	}
}
