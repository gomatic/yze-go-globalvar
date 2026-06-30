// Package globalvar provides a go/analysis analyzer enforcing the gomatic
// immutability/DI standard: package-level mutable vars are forbidden. Prefer a
// constant or dependency injection. A small allow-listed set of sanctioned
// package vars (and any configured via the -allow flag) is permitted.
package globalvar

import (
	"go/ast"
	"go/token"
	"strings"

	goyze "github.com/gomatic/go-yze"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
)

const message = "package-level var %q is not permitted; prefer a constant or dependency injection"

// defaultAllow is the baked-in set of sanctioned package-level var names that are
// standard across the gomatic ecosystem (an analyzer's exported Analyzer and
// Registration, and the version stamped via -ldflags).
var defaultAllow = map[string]bool{
	"version":      true,
	"Analyzer":     true,
	"Registration": true,
}

// allowExtra is the configurable allow-list of additional permitted package-level
// var names, set via the -allow flag or analyzer config.
var allowExtra string

// Analyzer reports package-level mutable vars that are not allow-listed.
var Analyzer = newAnalyzer()

func newAnalyzer() *analysis.Analyzer {
	a := &analysis.Analyzer{
		Name:     "globalvar",
		Doc:      "reports package-level mutable vars, which the gomatic immutability/DI standard forbids except for an allow-listed set",
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

// run reports every non-allow-listed package-level var declaration.
func run(pass *analysis.Pass) (any, error) {
	allow := buildAllow(allowExtra)
	for _, file := range pass.Files {
		checkFile(pass, allow, file)
	}
	return nil, nil
}

// buildAllow merges the baked-in allow-set with the configured extras.
func buildAllow(extra string) map[string]bool {
	allow := make(map[string]bool, len(defaultAllow))
	for name := range defaultAllow {
		allow[name] = true
	}
	for _, name := range splitNonEmpty(extra) {
		allow[name] = true
	}
	return allow
}

func splitNonEmpty(value string) []string {
	if value == "" {
		return nil
	}
	return strings.Split(value, ",")
}

// checkFile reports each package-level var name in a file's top-level
// declarations. Function-local vars are *ast.DeclStmt inside func bodies, not
// file.Decls, so ranging file.Decls restricts the check to package scope.
func checkFile(pass *analysis.Pass, allow map[string]bool, file *ast.File) {
	for _, decl := range file.Decls {
		checkDecl(pass, allow, decl)
	}
}

// checkDecl reports each name in decl when decl is a package-level var block.
func checkDecl(pass *analysis.Pass, allow map[string]bool, decl ast.Decl) {
	gen, ok := decl.(*ast.GenDecl)
	if !ok || gen.Tok != token.VAR {
		return
	}
	for _, spec := range gen.Specs {
		checkSpec(pass, allow, spec)
	}
}

// checkSpec reports each declared name in a var spec.
func checkSpec(pass *analysis.Pass, allow map[string]bool, spec ast.Spec) {
	for _, name := range spec.(*ast.ValueSpec).Names {
		checkName(pass, allow, name)
	}
}

// checkName reports name unless it is the blank identifier or allow-listed.
func checkName(pass *analysis.Pass, allow map[string]bool, name *ast.Ident) {
	if name.Name == "_" || allow[name.Name] {
		return
	}
	pass.Reportf(name.Pos(), message, name.Name)
}
