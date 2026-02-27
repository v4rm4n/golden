// --- golden/internal/transpiler/transpiler.odin ---

package transpiler

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"
)

// ── Top-level processor ──────────────────────────────────────────────────────

func Process(f *ast.File) string {
	var sb strings.Builder
	sb.WriteString("package main\n\n")
	sb.WriteString("import \"core:fmt\"\n")
	sb.WriteString("import golden \"golden\"\n\n")

	for _, decl := range f.Decls {
		var output string
		switch d := decl.(type) {
		case *ast.GenDecl:
			output = handleStruct(d)
		case *ast.FuncDecl:
			output = handleFunc(d)
		}
		if output != "" {
			sb.WriteString(output)
			sb.WriteString("\n\n")
		}
	}

	return strings.TrimSpace(sb.String()) + "\n"
}

// ── Type Mapping ─────────────────────────────────────────────────────────────

func mapType(expr ast.Expr) string {
	switch t := expr.(type) {

	case *ast.Ident:
		switch t.Name {
		case "int":     return "int"
		case "int32":   return "i32"
		case "int64":   return "i64"
		case "uint":    return "uint"
		case "string":  return "string"
		case "bool":    return "b8"
		case "float32": return "f32"
		case "float64": return "f64"
		case "byte":    return "byte"
		case "rune":    return "rune"
		case "error":   return "string" // simplified; full error interface comes later
		default:        return t.Name
		}

	// *T  →  ^T
	case *ast.StarExpr:
		return "^" + mapType(t.X)

	// []T  →  []T  (Odin dynamic array — close enough for now)
	case *ast.ArrayType:
		if t.Len == nil {
			return fmt.Sprintf("[dynamic]%s", mapType(t.Elt))
		}
		// [N]T fixed array
		return fmt.Sprintf("[%s]%s", exprToString(t.Len), mapType(t.Elt))

	// map[K]V  →  map[K]V
	case *ast.MapType:
		return fmt.Sprintf("map[%s]%s", mapType(t.Key), mapType(t.Value))
	}

	return "rawptr"
}

// ── Struct Handler ───────────────────────────────────────────────────────────

func handleStruct(d *ast.GenDecl) string {
	var sb strings.Builder
	for _, spec := range d.Specs {
		t, ok := spec.(*ast.TypeSpec)
		if !ok {
			continue
		}
		st, ok := t.Type.(*ast.StructType)
		if !ok {
			continue
		}

		sb.WriteString(fmt.Sprintf("%s :: struct {\n", t.Name.Name))
		for _, field := range st.Fields.List {
			typeName := mapType(field.Type)
			for _, name := range field.Names {
				sb.WriteString(fmt.Sprintf("\t%s: %s,\n", name.Name, typeName))
			}
		}
		sb.WriteString("}")
	}
	return sb.String()
}

// ── Symbol Table ─────────────────────────────────────────────────────────────

// SymbolTable tracks which variables are Arc-managed in the current function.
type SymbolTable struct {
	arcVars map[string]string // varName → underlying type e.g. "u" → "User"
}

func newSymbolTable() *SymbolTable {
	return &SymbolTable{arcVars: make(map[string]string)}
}

func (s *SymbolTable) markArc(name, typeName string) {
	s.arcVars[name] = typeName
}

func (s *SymbolTable) isArc(name string) bool {
	_, ok := s.arcVars[name]
	return ok
}

// ── Function Handler ─────────────────────────────────────────────────────────

func handleFunc(d *ast.FuncDecl) string {
	var sb strings.Builder

	// Symbol table is per-function scope
	syms := newSymbolTable()

	// Parameters — *T params become golden.Arc(T)
	var params []string
	if d.Type.Params != nil {
		for _, field := range d.Type.Params.List {
			if star, ok := field.Type.(*ast.StarExpr); ok {
				typeName := mapType(star.X)
				for _, pName := range field.Names {
					syms.markArc(pName.Name, typeName)
					params = append(params,
						fmt.Sprintf("%s: golden.Arc(%s)", pName.Name, typeName))
				}
			} else {
				pType := mapType(field.Type)
				for _, pName := range field.Names {
					params = append(params,
						fmt.Sprintf("%s: %s", pName.Name, pType))
				}
			}
		}
	}

	// Return types
	retType := ""
	if d.Type.Results != nil {
		var rets []string
		for _, r := range d.Type.Results.List {
			rets = append(rets, mapType(r.Type))
		}
		if len(rets) == 1 {
			retType = " -> " + rets[0]
		} else if len(rets) > 1 {
			retType = " -> (" + strings.Join(rets, ", ") + ")"
		}
	}

	sb.WriteString(fmt.Sprintf("%s :: proc(%s)%s {\n",
		d.Name.Name, strings.Join(params, ", "), retType))

	if d.Body != nil {
		writeStmtsWithSyms(&sb, d.Body.List, 1, syms)
	}

	sb.WriteString("}")
	return sb.String()
}

// ── Statement Writer (indent-aware) ─────────────────────────────────────────

// writeStmts writes statements with no symbol awareness (used in loops/if bodies
// where no new Arc declarations appear at the top level).
func writeStmts(sb *strings.Builder, stmts []ast.Stmt, depth int) {
	writeStmtsWithSyms(sb, stmts, depth, newSymbolTable())
}

// writeStmtsWithSyms is the real writer — symbol table flows through every stmt.
func writeStmtsWithSyms(sb *strings.Builder, stmts []ast.Stmt, depth int, syms *SymbolTable) {
	for _, stmt := range stmts {
		lines := translateStmtWithSyms(stmt, depth, syms)
		writeLines(sb, lines, depth)
	}
}

// writeLines writes pre-translated lines, applying depth-relative indentation.
// Lines that start with a tab already have relative indent from nested blocks.
// Lines that are empty are skipped.
func writeLines(sb *strings.Builder, lines []string, depth int) {
	base := strings.Repeat("\t", depth)
	for _, line := range lines {
		if line == "" {
			continue
		}
		sb.WriteString(base + line + "\n")
	}
}

// collectBody collects body lines without Arc symbol propagation.
func collectBody(stmts []ast.Stmt, depth int) []string {
	return collectBodyWithSyms(stmts, depth, newSymbolTable())
}

// collectBodyWithSyms collects body lines, passing the symbol table through
// so Arc variables declared in the outer scope are visible inside blocks.
func collectBodyWithSyms(stmts []ast.Stmt, depth int, syms *SymbolTable) []string {
	var sb strings.Builder
	writeStmtsWithSyms(&sb, stmts, depth+1, syms)
	raw := strings.TrimRight(sb.String(), "\n")
	if raw == "" {
		return nil
	}
	base := strings.Repeat("\t", depth+1)
	var lines []string
	for _, l := range strings.Split(raw, "\n") {
		lines = append(lines, strings.TrimPrefix(l, base))
	}
	return lines
}

// translateStmt wraps translateStmtWithSyms for call sites that don't need syms.
func translateStmt(stmt ast.Stmt, depth int) []string {
	return translateStmtWithSyms(stmt, depth, newSymbolTable())
}

// translateStmtWithSyms is the core statement translator with full Arc awareness.
func translateStmtWithSyms(stmt ast.Stmt, depth int, syms *SymbolTable) []string {
	switch s := stmt.(type) {

	// ── Assignment — detect &T{} on RHS ─────────────────────────────────────
	case *ast.AssignStmt:
		// Check if RHS is &T{} — Arc allocation
		if len(s.Lhs) == 1 && len(s.Rhs) == 1 {
			if unary, ok := s.Rhs[0].(*ast.UnaryExpr); ok && unary.Op == token.AND {
				if lit, ok := unary.X.(*ast.CompositeLit); ok {
					varName := exprToString(s.Lhs[0])
					typeName := mapType(lit.Type)
					litStr := handleCompositeLit(lit)
					// Register as Arc-managed
					syms.markArc(varName, typeName)
					return []string{
						fmt.Sprintf("%s %s golden.make_arc(%s)", varName, s.Tok.String(), litStr),
						fmt.Sprintf("defer golden.release(%s)", varName),
					}
				}
			}
		}
		// Normal assignment — but unwrap Arc vars on RHS
		var lhs, rhs []string
		for _, l := range s.Lhs {
			lhs = append(lhs, exprToString(l))
		}
		for _, r := range s.Rhs {
			rhs = append(rhs, exprToStringWithSyms(r, syms))
		}
		return []string{fmt.Sprintf("%s %s %s",
			strings.Join(lhs, ", "), s.Tok.String(), strings.Join(rhs, ", "))}

	// ── var declarations ─────────────────────────────────────────────────────
	case *ast.DeclStmt:
		return translateDecl(s.Decl)

	// ── Expression statement ─────────────────────────────────────────────────
	case *ast.ExprStmt:
		if call, ok := s.X.(*ast.CallExpr); ok {
			return []string{handleCallWithSyms(call, syms)}
		}
		return []string{exprToStringWithSyms(s.X, syms)}

	// ── return ───────────────────────────────────────────────────────────────
	case *ast.ReturnStmt:
		if len(s.Results) == 0 {
			return []string{"return"}
		}
		var parts []string
		for _, r := range s.Results {
			parts = append(parts, exprToStringWithSyms(r, syms))
		}
		return []string{"return " + strings.Join(parts, ", ")}

	// ── if / else if / else ──────────────────────────────────────────────────
	case *ast.IfStmt:
		return translateIfWithSyms(s, depth, syms)

	// ── for loop ─────────────────────────────────────────────────────────────
	case *ast.ForStmt:
		return translateFor(s, depth)

	// ── for range ────────────────────────────────────────────────────────────
	case *ast.RangeStmt:
		return translateRange(s, depth)

	// ── defer ─────────────────────────────────────────────────────────────────
	case *ast.DeferStmt:
		return []string{"defer " + handleCall(s.Call)}

	// ── increment / decrement ─────────────────────────────────────────────────
	case *ast.IncDecStmt:
		op := "+="
		if s.Tok == token.DEC {
			op = "-="
		}
		return []string{fmt.Sprintf("%s %s 1", exprToString(s.X), op)}

	// ── block ─────────────────────────────────────────────────────────────────
	case *ast.BlockStmt:
		var lines []string
		lines = append(lines, "{")
		var inner strings.Builder
		writeStmtsWithSyms(&inner, s.List, depth+1, syms)
		lines = append(lines, inner.String())
		lines = append(lines, "}")
		return lines
	}

	return []string{"// TODO: unsupported statement"}
}

// ── If / Else ────────────────────────────────────────────────────────────────

// translateIf returns lines relative to the current depth (no leading indent).
// The caller (writeStmts/writeLines) applies the base indent.
func translateIf(s *ast.IfStmt, depth int) []string {
	var lines []string

	cond := exprToString(s.Cond)
	lines = append(lines, fmt.Sprintf("if %s {", cond))

	inner := strings.Repeat("\t", 1) // one level relative
	for _, l := range collectBody(s.Body.List, depth) {
		lines = append(lines, inner+l)
	}

	if s.Else == nil {
		lines = append(lines, "}")
		return lines
	}

	switch el := s.Else.(type) {
	case *ast.IfStmt:
		// else if — recurse and attach
		elseLines := translateIf(el, depth)
		lines = append(lines, "} else "+elseLines[0])
		lines = append(lines, elseLines[1:]...)

	case *ast.BlockStmt:
		lines = append(lines, "} else {")
		for _, l := range collectBody(el.List, depth) {
			lines = append(lines, inner+l)
		}
		lines = append(lines, "}")
	}

	return lines
}

// ── For Loop ─────────────────────────────────────────────────────────────────

func translateFor(s *ast.ForStmt, depth int) []string {
	var lines []string
	inner := strings.Repeat("\t", 1) // one level relative

	// Bare `for { }`
	if s.Init == nil && s.Cond == nil && s.Post == nil {
		lines = append(lines, "for {")
		for _, l := range collectBody(s.Body.List, depth) {
			lines = append(lines, inner+l)
		}
		lines = append(lines, "}")
		return lines
	}

	// `for cond { }`
	if s.Init == nil && s.Post == nil {
		lines = append(lines, fmt.Sprintf("for %s {", exprToString(s.Cond)))
		for _, l := range collectBody(s.Body.List, depth) {
			lines = append(lines, inner+l)
		}
		lines = append(lines, "}")
		return lines
	}

	// C-style `for init; cond; post { }`
	// Odin has no C-style for, so emit: init \n for cond { body \n post }
	initLines := translateStmt(s.Init, depth)
	postLines := translateStmt(s.Post, depth)

	lines = append(lines, initLines...)
	lines = append(lines, fmt.Sprintf("for %s {", exprToString(s.Cond)))
	for _, l := range collectBody(s.Body.List, depth) {
		lines = append(lines, inner+l)
	}
	for _, pl := range postLines {
		lines = append(lines, inner+pl)
	}
	lines = append(lines, "}")
	return lines
}

// ── Range Loop ───────────────────────────────────────────────────────────────

func translateRange(s *ast.RangeStmt, depth int) []string {
	var lines []string
	inner := strings.Repeat("\t", 1)

	collection := exprToString(s.X)
	key := "_"
	val := "_"
	if s.Key != nil {
		key = exprToString(s.Key)
	}
	if s.Value != nil {
		val = exprToString(s.Value)
	}

	// Odin: for value, index in collection
	lines = append(lines, fmt.Sprintf("for %s, %s in %s {", val, key, collection))
	for _, l := range collectBody(s.Body.List, depth) {
		lines = append(lines, inner+l)
	}
	lines = append(lines, "}")
	return lines
}

// ── Var Declarations ─────────────────────────────────────────────────────────

func translateDecl(decl ast.Decl) []string {
	gd, ok := decl.(*ast.GenDecl)
	if !ok {
		return nil
	}
	var lines []string
	for _, spec := range gd.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		typeName := ""
		if vs.Type != nil {
			typeName = ": " + mapType(vs.Type)
		}
		for i, name := range vs.Names {
			if i < len(vs.Values) {
				lines = append(lines, fmt.Sprintf("%s%s = %s",
					name.Name, typeName, exprToString(vs.Values[i])))
			} else {
				lines = append(lines, fmt.Sprintf("%s%s", name.Name, typeName))
			}
		}
	}
	return lines
}

// ── Expression → String ──────────────────────────────────────────────────────

func exprToString(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	switch e := expr.(type) {

	case *ast.Ident:
		// Map Go built-ins
		switch e.Name {
		case "true":  return "true"
		case "false": return "false"
		case "nil":   return "nil"
		}
		return e.Name

	case *ast.BasicLit:
		return e.Value

	// Binary expressions:  a + b,  a > b,  a == b …
	case *ast.BinaryExpr:
		left  := exprToString(e.X)
		right := exprToString(e.Y)
		op    := mapOperator(e.Op)
		return fmt.Sprintf("%s %s %s", left, op, right)

	// Unary:  -x,  !b,  &v
	case *ast.UnaryExpr:
		op := e.Op.String()
		if e.Op == token.AND {
			op = "&"
		}
		return op + exprToString(e.X)

	// Parenthesised expression
	case *ast.ParenExpr:
		return fmt.Sprintf("(%s)", exprToString(e.X))

	// pkg.Field or struct.field
	case *ast.SelectorExpr:
		return fmt.Sprintf("%s.%s", exprToString(e.X), e.Sel.Name)

	// a[i]
	case *ast.IndexExpr:
		return fmt.Sprintf("%s[%s]", exprToString(e.X), exprToString(e.Index))

	// f(args...)
	case *ast.CallExpr:
		return handleCall(e)

	// composite literal:  User{Name: "x", Age: 1}
	case *ast.CompositeLit:
		return handleCompositeLit(e)

	// type assertion:  x.(T)  — emit a comment for now
	case *ast.TypeAssertExpr:
		return fmt.Sprintf("/* type assert */ %s", exprToString(e.X))

	// slice expression: s[lo:hi]
	case *ast.SliceExpr:
		lo := exprToString(e.Low)
		hi := exprToString(e.High)
		return fmt.Sprintf("%s[%s:%s]", exprToString(e.X), lo, hi)
	}

	return fmt.Sprintf("/* unknown expr %T */", expr)
}

// ── Operator Mapping ─────────────────────────────────────────────────────────

func mapOperator(op token.Token) string {
	switch op {
	case token.ADD: return "+"
	case token.SUB: return "-"
	case token.MUL: return "*"
	case token.QUO: return "/"
	case token.REM: return "%%"
	case token.EQL: return "=="
	case token.NEQ: return "!="
	case token.LSS: return "<"
	case token.GTR: return ">"
	case token.LEQ: return "<="
	case token.GEQ: return ">="
	case token.LAND: return "&&"
	case token.LOR:  return "||"
	case token.AND:  return "&"
	case token.OR:   return "|"
	case token.XOR:  return "~"
	case token.SHL:  return "<<"
	case token.SHR:  return ">>"
	}
	return op.String()
}

// ── Function Call Mapping ────────────────────────────────────────────────────

var funcMap = map[string]string{
	"fmt.Println":  "fmt.println",
	"fmt.Printf":   "fmt.printf",
	"fmt.Sprintf":  "fmt.tprintf",  // closest Odin equivalent
	"fmt.Print":    "fmt.print",
	"fmt.Fprintf":  "fmt.fprintln", // approximate
	"len":          "len",
	"cap":          "cap",
	"make":         "make",
	"append":       "append",
	"delete":       "delete",
	"panic":        "panic",
	"new":          "new",
}

func handleCall(call *ast.CallExpr) string {
	funcName := exprToString(call.Fun)

	if mapped, ok := funcMap[funcName]; ok {
		funcName = mapped
	}

	var args []string
	for _, arg := range call.Args {
		args = append(args, exprToString(arg))
	}

	// Variadic spread: f(args...)
	ellipsis := ""
	if call.Ellipsis.IsValid() {
		ellipsis = ".."
	}

	return fmt.Sprintf("%s(%s%s)", funcName, strings.Join(args, ", "), ellipsis)
}

// ── Composite Literal ─────────────────────────────────────────────────────────

func handleCompositeLit(lit *ast.CompositeLit) string {
	typeName := ""
	if lit.Type != nil {
		typeName = mapType(lit.Type)
	}

	if len(lit.Elts) == 0 {
		return typeName + "{}"
	}

	var fields []string
	for _, elt := range lit.Elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			fields = append(fields, fmt.Sprintf("%s = %s",
				exprToString(kv.Key), exprToString(kv.Value)))
		} else {
			fields = append(fields, exprToString(elt))
		}
	}

	// Short literals on one line; longer ones multiline
	if len(fields) <= 3 {
		return fmt.Sprintf("%s{%s}", typeName, strings.Join(fields, ", "))
	}
	return fmt.Sprintf("%s{\n\t\t%s,\n\t}", typeName, strings.Join(fields, ",\n\t\t"))
}

// ── Arc-aware expression rendering ───────────────────────────────────────────

// exprToStringWithSyms renders an expression, auto-unwrapping Arc .data fields.
func exprToStringWithSyms(expr ast.Expr, syms *SymbolTable) string {
	if expr == nil {
		return ""
	}
	switch e := expr.(type) {

	// u.Name → u.data.Name  (if u is Arc-managed)
	case *ast.SelectorExpr:
		base := exprToString(e.X)
		if ident, ok := e.X.(*ast.Ident); ok && syms.isArc(ident.Name) {
			return fmt.Sprintf("%s.data.%s", base, e.Sel.Name)
		}
		return fmt.Sprintf("%s.%s", base, e.Sel.Name)

	// For all other exprs, fall back to the standard renderer
	default:
		return exprToString(expr)
	}
}

// handleCallWithSyms renders a function call, unwrapping Arc args correctly.
func handleCallWithSyms(call *ast.CallExpr, syms *SymbolTable) string {
	funcName := exprToString(call.Fun)
	if mapped, ok := funcMap[funcName]; ok {
		funcName = mapped
	}

	var args []string
	for _, arg := range call.Args {
		args = append(args, exprToStringWithSyms(arg, syms))
	}

	ellipsis := ""
	if call.Ellipsis.IsValid() {
		ellipsis = ".."
	}
	return fmt.Sprintf("%s(%s%s)", funcName, strings.Join(args, ", "), ellipsis)
}

// translateIfWithSyms passes the symbol table into if/else bodies.
func translateIfWithSyms(s *ast.IfStmt, depth int, syms *SymbolTable) []string {
	var lines []string
	inner := strings.Repeat("\t", 1)

	cond := exprToStringWithSyms(s.Cond, syms)
	lines = append(lines, fmt.Sprintf("if %s {", cond))

	for _, l := range collectBodyWithSyms(s.Body.List, depth, syms) {
		lines = append(lines, inner+l)
	}

	if s.Else == nil {
		lines = append(lines, "}")
		return lines
	}

	switch el := s.Else.(type) {
	case *ast.IfStmt:
		elseLines := translateIfWithSyms(el, depth, syms)
		lines = append(lines, "} else "+elseLines[0])
		lines = append(lines, elseLines[1:]...)
	case *ast.BlockStmt:
		lines = append(lines, "} else {")
		for _, l := range collectBodyWithSyms(el.List, depth, syms) {
			lines = append(lines, inner+l)
		}
		lines = append(lines, "}")
	}

	return lines
}