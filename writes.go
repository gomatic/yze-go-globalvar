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
		root := rootIdent(pass, target)
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
// when the target is rooted at no identifier. Indexing, slicing, field
// selection, dereference, type assertion and an inline address-of each name
// storage reached from their operand rather than a new value, so the operand is
// what the write is attributed to and the walk continues into it. A shadowing
// local resolves to a different object, so it is not reported.
//
// Slicing is here because it is the ONLY way to write into an array-typed var
// (copy(v[:], src)) and the ordinary spelling of a partial write, and because
// without it copy(v[:], src) would silence copy(v, src) with two characters.
// The address-of is here for the same reason: (&v).f = x is v.f = x, and a
// limitation a pair of parentheses can invoke is not a limitation.
//
// A target rooted at anything else yields nil: a call (f().x = 1), and a unary
// expression that is not an address-of — most importantly a channel receive
// ((<-ch).f = 1), which writes the value the receive produced, so reporting ch
// would be reporting a read of it.
func rootIdent(pass *analysis.Pass, target ast.Expr) *ast.Ident {
	switch expr := ast.Unparen(target).(type) {
	case *ast.Ident:
		return expr
	case *ast.IndexExpr:
		return rootIdent(pass, expr.X)
	case *ast.SliceExpr:
		return rootIdent(pass, expr.X)
	case *ast.SelectorExpr:
		return rootIdent(pass, expr.X)
	case *ast.StarExpr:
		return rootIdent(pass, expr.X)
	case *ast.TypeAssertExpr:
		return rootIdent(pass, expr.X)
	case *ast.UnaryExpr:
		return addressedIdent(pass, expr)
	}
	return nil
}

// addressedIdent returns the identifier whose address expr takes, or nil when
// expr is any other unary expression. Go's unary operators are not separable by
// shape and the operator token cannot be compared without discriminating a
// 90-member const group, so the type checker is asked instead: taking an
// address is exactly the operation whose result is a pointer to its OWN
// operand's type, which no other unary operator produces.
func addressedIdent(pass *analysis.Pass, expr *ast.UnaryExpr) *ast.Ident {
	pointer, isPointer := pass.TypesInfo.TypeOf(expr).(*types.Pointer)
	if !isPointer || !types.Identical(pointer.Elem(), pass.TypesInfo.TypeOf(expr.X)) {
		return nil
	}
	return rootIdent(pass, expr.X)
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
