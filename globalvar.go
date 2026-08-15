// Package globalvar provides a go/analysis analyzer enforcing the gomatic
// immutability/DI standard: package-level state is immutable outside tests. An
// exported package-level var — a binding any importer can rebind — is reported
// at its declaration unless allow-listed; an unexported var is reported at each
// write to it in non-test code.
//
// A write is either a rebinding of the name (v = x, v++, for v = range ...) or
// a write THROUGH the name into the state it holds: an index, field or
// dereference assignment target rooted at the var (v[k] = x, v.f = x, *v = x,
// and any nesting of those), and a call to one of the builtins that writes into
// the container it is given (delete(v, k), clear(v), copy(v, src)). append is
// not one: it returns the result rather than writing into its argument, so a
// package var grown by append is caught as the rebinding it must perform.
// Rebinding and writing through are reported with different messages because
// the remedy differs — one replaces the binding, the other mutates shared state
// the binding merely names.
//
// An unexported var never written to outside tests is either an immutable
// binding (a value Go cannot express as a const — a lookup table that is only
// read, a func value, a //go:embed FS) or a dependency-injection seam
// reassigned only by the package's tests; both are sanctioned gomatic patterns.
// A map or slice earns that sanction by being read-only in practice, never by
// being a map or a slice: declaring one is a binding Go cannot make constant,
// and writing into it afterwards is exactly the mutable package state the
// standard forbids.
//
// Test files (_test.go) are exempt from both checks: writing to package state
// there is the sanctioned dependency-injection seam, and an exported var
// declared in a test file (the export_test.go seam) is not importable, so the
// export rationale does not apply. An allow-listed name (version, Analyzer,
// Registration, or a -allow extra) is exempt from both the export check and the
// write watch.
//
// Two kinds of write are deliberately out of scope. The first is a write that
// reaches package state through something other than the var: a pointer alias
// (p := &v; *p = x), a local copy of a reference type (m := v; m[k] = x), and
// any function handed the var or its address. The analyzer resolves the
// identifier an assignment target is rooted at; it performs no escape or alias
// analysis, so a write whose target is rooted at a local is attributed to that
// local and reported nowhere.
//
// The second is a write performed by USING the value rather than by naming a
// target: a method call, whatever its receiver does (v.bump(), mu.Lock()), and
// a channel send (v <- x). Neither is an assignment target, and neither is
// separable from the ordinary use of the value — a channel exists to be sent
// on, and reporting the send would ban a package-level channel outright, which
// is a rule about the declaration and not one this analyzer makes.
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

// exportedMessage is the diagnostic for an exported package-level var, reported
// at its declaration.
const exportedMessage = "exported package-level var %q is not permitted; export a constant or a constructor instead"

// defaultAllow is the baked-in set of sanctioned package-level var names that
// are standard across the gomatic ecosystem (an analyzer's exported Analyzer
// and Registration, and the version stamped via -ldflags). An allow-listed
// name is exempt from BOTH checks: it may be exported AND it may be written to
// outside tests (it is never added to the watch set).
var defaultAllow = map[string]bool{
	"version":      true,
	"Analyzer":     true,
	"Registration": true,
}

// allowExtra is the configurable allow-list of additional permitted package-level
// var names, set via the -allow flag or analyzer config.
var allowExtra string

// Analyzer reports package-level vars that are exported or written to outside tests.
var Analyzer = newAnalyzer()

func newAnalyzer() *analysis.Analyzer {
	a := &analysis.Analyzer{
		Name:     "globalvar",
		Doc:      "reports exported package-level vars and package-level vars rebound or written through outside tests, which the gomatic immutability/DI standard forbids",
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
// unexported ones at each non-test write.
func run(pass *analysis.Pass) (any, error) {
	allow := buildAllow(allowCSV(allowExtra))
	watched := packageVars(pass, allow)
	for _, file := range pass.Files {
		if !isTestFile(pass, file) {
			checkWrites(pass, watched, file)
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
// the set of unexported package-level var objects to watch for writes.
// Test files are skipped entirely: an exported var there (the export_test.go
// seam) is not importable, and an unexported var there is test-only state that
// production code cannot reference.
func packageVars(pass *analysis.Pass, allow map[string]bool) map[types.Object]bool {
	watched := make(map[types.Object]bool)
	for _, file := range pass.Files {
		if isTestFile(pass, file) {
			continue
		}
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

// isTestFile reports whether file is a _test.go file, where writing to a
// package-level var is the sanctioned dependency-injection seam.
func isTestFile(pass *analysis.Pass, file *ast.File) bool {
	return strings.HasSuffix(pass.Fset.File(file.Pos()).Name(), "_test.go")
}
