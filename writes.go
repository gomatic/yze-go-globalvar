// Write detection: what counts as writing to a package-level var, and which
// identifier a write is attributed to.
package globalvar

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

const (
	reassignedMessage = "package-level var %q is reassigned outside tests; package state must be immutable (inject the dependency instead)"
	mutatedMessage    = "package-level var %q is mutated outside tests; package state must be immutable (inject the dependency instead)"
)

// checkWrites reports each write to a watched var in file: plain and compound
// assignments, increment/decrement statements, range clauses that assign (for v
// = range ...), and calls to the builtins that write into a container.
func checkWrites(pass *analysis.Pass, watched map[types.Object]bool, file *ast.File) {
	ast.Inspect(file, func(n ast.Node) bool {
		switch stmt := n.(type) {
		case *ast.AssignStmt:
			if stmt.Tok != token.DEFINE {
				reportTargets(pass, watched, stmt.Lhs)
			}
		case *ast.IncDecStmt:
			reportTargets(pass, watched, []ast.Expr{stmt.X})
		case *ast.RangeStmt:
			reportTargets(pass, watched, rangeTargets(stmt))
		case *ast.CallExpr:
			reportWrites(pass, watched, builtinWriteTargets(pass, stmt), mutationMessage)
		}
		return true
	})
}

// rangeTargets returns the assignment targets of a range clause that rebinds
// existing variables (Tok == token.ASSIGN). A := clause declares new
// (shadowing) locals and a bare `for range` binds nothing, so both yield nil.
func rangeTargets(stmt *ast.RangeStmt) []ast.Expr {
	if stmt.Tok != token.ASSIGN {
		return nil
	}
	targets := make([]ast.Expr, 0, 2)
	if stmt.Key != nil {
		targets = append(targets, stmt.Key)
	}
	if stmt.Value != nil {
		targets = append(targets, stmt.Value)
	}
	return targets
}

// reportTargets reports each assignment target rooted at a watched package-level
// var. A bare identifier rebinds the var; an index, field or dereference target
// writes through it into the state it holds. Both are writes to package state,
// and they are reported with different messages because the remedy differs.
func reportTargets(pass *analysis.Pass, watched map[types.Object]bool, targets []ast.Expr) {
	reportWrites(pass, watched, targets, assignmentMessage)
}

// reportWrites reports each expression rooted at a watched package-level var,
// at the var it reaches, with the message describe chooses for it.
func reportWrites(pass *analysis.Pass, watched map[types.Object]bool, targets []ast.Expr, describe writeMessage) {
	for _, target := range targets {
		root := rootIdent(target)
		if root != nil && watched[pass.TypesInfo.ObjectOf(root)] {
			pass.Reportf(root.Pos(), describe(target), root.Name)
		}
	}
}

// writeMessage chooses the diagnostic format for one written expression.
type writeMessage func(target ast.Expr) string

// assignmentMessage distinguishes rebinding the var from writing through it. A
// bare identifier target (parentheses aside — (v) = x rebinds v) replaces the
// binding; anything else leaves the binding alone and writes into its state.
func assignmentMessage(target ast.Expr) string {
	if _, rebound := ast.Unparen(target).(*ast.Ident); rebound {
		return reassignedMessage
	}
	return mutatedMessage
}

// mutationMessage is the message for a builtin container write, which names the
// container it writes into rather than replacing the binding.
func mutationMessage(ast.Expr) string { return mutatedMessage }

// rootIdent returns the identifier an assignment target is rooted at, or nil
// when the target is rooted at no identifier. Indexing, field selection and
// dereference all write into the state reached from their base, so the base is
// what the write is attributed to; a shadowing local resolves to a different
// object, so it is not reported.
//
// A target rooted at anything else yields nil and is not reported: a call
// (f().x = 1), and a unary expression — an inline address-of ((&v).f = 1) or a
// channel receive ((<-ch).f = 1). The address-of is the pointer-alias
// limitation written on one line instead of two, and stops here for the same
// reason: what this resolves is the identifier a target is ROOTED at, and &v is
// not that identifier.
func rootIdent(target ast.Expr) *ast.Ident {
	switch expr := ast.Unparen(target).(type) {
	case *ast.Ident:
		return expr
	case *ast.IndexExpr:
		return rootIdent(expr.X)
	case *ast.SelectorExpr:
		return rootIdent(expr.X)
	case *ast.StarExpr:
		return rootIdent(expr.X)
	}
	return nil
}

// mutatingBuiltins are the predeclared functions that write into the container
// they are given rather than returning a new one. append is absent on purpose:
// it returns the result and is not specified to write into its argument.
var mutatingBuiltins = map[string]bool{"delete": true, "clear": true, "copy": true}

// builtinWriteTargets returns the container a mutating builtin call writes into.
// delete(m, k), clear(m) and copy(dst, src) all write through their FIRST
// argument and read every later one, so only the first is a write target. A
// call to a function merely spelled like one of them is an ordinary call — the
// object is what is matched, never the name alone.
func builtinWriteTargets(pass *analysis.Pass, call *ast.CallExpr) []ast.Expr {
	fn, ok := ast.Unparen(call.Fun).(*ast.Ident)
	if !ok || !mutatingBuiltins[fn.Name] {
		return nil
	}
	if _, predeclared := pass.TypesInfo.ObjectOf(fn).(*types.Builtin); !predeclared {
		return nil
	}
	return call.Args[:min(1, len(call.Args))]
}
