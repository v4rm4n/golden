// --- golden/internal/transpiler/escape.go ---

package transpiler

import (
	"go/ast"
	"go/token"
)

// ── Escape Analysis ───────────────────────────────────────────────────────────
//
// EscapeSet is computed once per function before any code is emitted.
// It answers: "does this variable name escape its declaring function?"
//
// A variable ESCAPES if it is:
//   1. Returned from the function
//   2. Passed to a goroutine (go stmt)
//   3. Assigned into a field of another struct (stored)
//   4. Assigned to a package-level var (global store)
//
// Everything else is LOCAL → safe for arena allocation.

type EscapeSet map[string]bool

// AnalyzeFunc runs escape analysis on a function body and returns
// the set of variable names that escape.
func AnalyzeFunc(body *ast.BlockStmt) EscapeSet {
	esc := make(EscapeSet)
	if body == nil {
		return esc
	}

	// First pass: collect all &T{} declarations so we know which
	// names to watch.
	arcVars := collectArcDecls(body.List)

	// Second pass: walk all statements looking for escape sites.
	for _, stmt := range body.List {
		walkForEscapes(stmt, arcVars, esc)
	}

	return esc
}

// collectArcDecls returns the set of variable names declared as &T{}.
func collectArcDecls(stmts []ast.Stmt) map[string]bool {
	vars := make(map[string]bool)
	for _, stmt := range stmts {
		assign, ok := stmt.(*ast.AssignStmt)
		if !ok {
			continue
		}
		if len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			continue
		}
		unary, ok := assign.Rhs[0].(*ast.UnaryExpr)
		if !ok || unary.Op != token.AND {
			continue
		}
		if _, ok := unary.X.(*ast.CompositeLit); ok {
			if ident, ok := assign.Lhs[0].(*ast.Ident); ok {
				vars[ident.Name] = true
			}
		}
	}
	return vars
}

// walkForEscapes checks a statement for all escape patterns.
func walkForEscapes(stmt ast.Stmt, arcVars map[string]bool, esc EscapeSet) {
	switch s := stmt.(type) {

	// return x  →  x escapes
	case *ast.ReturnStmt:
		for _, r := range s.Results {
			markEscapingIdents(r, arcVars, esc)
		}

	// go func() { use(x) }  →  x escapes
	case *ast.GoStmt:
		ast.Inspect(s.Call, func(n ast.Node) bool {
			if ident, ok := n.(*ast.Ident); ok && arcVars[ident.Name] {
				esc[ident.Name] = true
			}
			return true
		})

	// a.Field = x  →  x escapes (stored into struct)
	case *ast.AssignStmt:
		for _, lhs := range s.Lhs {
			if _, ok := lhs.(*ast.SelectorExpr); ok {
				// RHS is being stored into a field — it escapes
				for _, rhs := range s.Rhs {
					markEscapingIdents(rhs, arcVars, esc)
				}
			}
		}

	// Recurse into blocks, if/else, for bodies
	case *ast.BlockStmt:
		for _, inner := range s.List {
			walkForEscapes(inner, arcVars, esc)
		}
	case *ast.IfStmt:
		walkForEscapes(s.Body, arcVars, esc)
		if s.Else != nil {
			walkForEscapes(s.Else, arcVars, esc)
		}
	case *ast.ForStmt:
		walkForEscapes(s.Body, arcVars, esc)
	case *ast.RangeStmt:
		walkForEscapes(s.Body, arcVars, esc)
	}
}

// markEscapingIdents marks any arc-managed ident in expr as escaping.
func markEscapingIdents(expr ast.Expr, arcVars map[string]bool, esc EscapeSet) {
	ast.Inspect(expr, func(n ast.Node) bool {
		if ident, ok := n.(*ast.Ident); ok && arcVars[ident.Name] {
			esc[ident.Name] = true
		}
		return true
	})
}
