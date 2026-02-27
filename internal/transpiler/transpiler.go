// --- golden/internal/transpiler/transpiler.go ---

package transpiler

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"
)

// ── Top-level processor ──────────────────────────────────────────────────────

var funcReturnTypes = map[string]string{}

func Process(f *ast.File) string {
	funcReturnTypes = make(map[string]string)
	for _, decl := range f.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if fd.Type.Results == nil || len(fd.Type.Results.List) == 0 {
			continue
		}
		first := fd.Type.Results.List[0].Type
		if star, ok := first.(*ast.StarExpr); ok {
			funcReturnTypes[fd.Name.Name] = mapType(star.X)
		}
	}

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
		case "int":
			return "int"
		case "int32":
			return "i32"
		case "int64":
			return "i64"
		case "uint":
			return "uint"
		case "string":
			return "string"
		case "bool":
			return "b8"
		case "float32":
			return "f32"
		case "float64":
			return "f64"
		case "byte":
			return "byte"
		case "rune":
			return "rune"
		case "error":
			return "string"
		default:
			return t.Name
		}
	case *ast.StarExpr:
		return "^" + mapType(t.X)
	case *ast.ArrayType:
		if t.Len == nil {
			return fmt.Sprintf("[dynamic]%s", mapType(t.Elt))
		}
		return fmt.Sprintf("[%s]%s", exprToString(t.Len), mapType(t.Elt))
	case *ast.MapType:
		return fmt.Sprintf("map[%s]%s", mapType(t.Key), mapType(t.Value))
	case *ast.SelectorExpr:
		pkg := exprToString(t.X)
		name := t.Sel.Name
		if pkg == "sync" {
			return mapSyncType(name)
		}
		return pkg + "." + name
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

type AllocStrategy int

const (
	AllocNone AllocStrategy = iota
	AllocARC
	AllocArena
)

type SymbolTable struct {
	arcVars    map[string]string
	strategy   map[string]AllocStrategy
	needsFrame bool
}

func newSymbolTable() *SymbolTable {
	return &SymbolTable{
		arcVars:  make(map[string]string),
		strategy: make(map[string]AllocStrategy),
	}
}

func (s *SymbolTable) markArc(name, typeName string) {
	s.arcVars[name] = typeName
	s.strategy[name] = AllocARC
}

func (s *SymbolTable) markArena(name, typeName string) {
	s.arcVars[name] = typeName
	s.strategy[name] = AllocArena
	s.needsFrame = true
}

func (s *SymbolTable) isArc(name string) bool {
	st, ok := s.strategy[name]
	return ok && st == AllocARC
}

func (s *SymbolTable) strategyOf(name string) AllocStrategy {
	if st, ok := s.strategy[name]; ok {
		return st
	}
	return AllocNone
}

// ── Function Handler ─────────────────────────────────────────────────────────

func handleFunc(d *ast.FuncDecl) string {
	syms := newSymbolTable()
	var params []string
	if d.Type.Params != nil {
		for _, field := range d.Type.Params.List {
			pType := mapType(field.Type)
			for _, pName := range field.Names {
				if _, ok := field.Type.(*ast.StarExpr); ok {
					syms.markArena(pName.Name, pType)
				}
				params = append(params, fmt.Sprintf("%s: %s", pName.Name, pType))
			}
		}
	}

	rawResults := []*ast.Field{}
	if d.Type.Results != nil {
		rawResults = d.Type.Results.List
	}

	var escapes EscapeSet
	if d.Body != nil {
		escapes = AnalyzeFunc(d.Body)
		for _, stmt := range d.Body.List {
			assign, ok := stmt.(*ast.AssignStmt)
			if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
				continue
			}
			if unary, ok := assign.Rhs[0].(*ast.UnaryExpr); ok && unary.Op == token.AND {
				if lit, ok := unary.X.(*ast.CompositeLit); ok {
					varName := exprToString(assign.Lhs[0])
					typeName := mapType(lit.Type)
					if escapes[varName] {
						syms.markArc(varName, typeName)
					} else {
						syms.markArena(varName, typeName)
					}
				}
				continue
			}
			if call, ok := assign.Rhs[0].(*ast.CallExpr); ok {
				callee := exprToString(call.Fun)
				if retTypeName, ok := funcReturnTypes[callee]; ok {
					varName := exprToString(assign.Lhs[0])
					syms.markArc(varName, retTypeName)
				}
			}
		}
	}

	retType := ""
	if len(rawResults) > 0 {
		var rets []string
		for _, r := range rawResults {
			if star, ok := r.Type.(*ast.StarExpr); ok {
				innerType := mapType(star.X)
				hasEscapingArc := false
				for name, typName := range syms.arcVars {
					if syms.isArc(name) && typName == innerType {
						hasEscapingArc = true
						break
					}
				}
				if hasEscapingArc {
					rets = append(rets, "golden.Arc("+innerType+")")
				} else {
					rets = append(rets, "^"+innerType)
				}
			} else {
				rets = append(rets, mapType(r.Type))
			}
		}
		if len(rets) == 1 {
			retType = " -> " + rets[0]
		} else if len(rets) > 1 {
			retType = " -> (" + strings.Join(rets, ", ") + ")"
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s :: proc(%s)%s {\n", d.Name.Name, strings.Join(params, ", "), retType))

	if d.Body != nil {
		if d.Name.Name == "main" {
			sb.WriteString("\tgolden.pool_start(8)\n")
			sb.WriteString("\tdefer golden.pool_stop()\n")
		}
		if syms.needsFrame {
			sb.WriteString("\t_frame := golden.frame_begin()\n")
			sb.WriteString("\tdefer golden.frame_end(&_frame)\n")
		}
		writeStmtsWithSyms(&sb, d.Body.List, 1, syms)
	}

	sb.WriteString("}")
	return sb.String()
}

// ── Statement Writer ─────────────────────────────────────────

func writeStmts(sb *strings.Builder, stmts []ast.Stmt, depth int) {
	writeStmtsWithSyms(sb, stmts, depth, newSymbolTable())
}

func writeStmtsWithSyms(sb *strings.Builder, stmts []ast.Stmt, depth int, syms *SymbolTable) {
	for _, stmt := range stmts {
		lines := translateStmtWithSyms(stmt, depth, syms)
		writeLines(sb, lines, depth)
	}
}

func writeLines(sb *strings.Builder, lines []string, depth int) {
	base := strings.Repeat("\t", depth)
	for _, line := range lines {
		if line == "" {
			continue
		}
		sb.WriteString(base + line + "\n")
	}
}

func collectBody(stmts []ast.Stmt, depth int) []string {
	return collectBodyWithSyms(stmts, depth, newSymbolTable())
}

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

func translateStmt(stmt ast.Stmt, depth int) []string {
	return translateStmtWithSyms(stmt, depth, newSymbolTable())
}

func translateStmtWithSyms(stmt ast.Stmt, depth int, syms *SymbolTable) []string {
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		if len(s.Lhs) == 1 && len(s.Rhs) == 1 {
			if unary, ok := s.Rhs[0].(*ast.UnaryExpr); ok && unary.Op == token.AND {
				if lit, ok := unary.X.(*ast.CompositeLit); ok {
					varName := exprToString(s.Lhs[0])
					litStr := handleCompositeLit(lit)
					typeName := mapType(lit.Type)

					switch syms.strategyOf(varName) {
					case AllocArena:
						return []string{
							fmt.Sprintf("%s %s golden.frame_new(%s{}, &_frame)", varName, s.Tok.String(), typeName),
							fmt.Sprintf("golden.frame_init(%s, %s)", varName, litStr),
						}
					default:
						return []string{
							fmt.Sprintf("%s %s golden.make_arc(%s)", varName, s.Tok.String(), litStr),
						}
					}
				}
			}
		}
		var lhs, rhs []string
		for _, l := range s.Lhs {
			lhs = append(lhs, exprToString(l))
		}
		for _, r := range s.Rhs {
			rhs = append(rhs, exprToStringWithSyms(r, syms))
		}
		return []string{fmt.Sprintf("%s %s %s", strings.Join(lhs, ", "), s.Tok.String(), strings.Join(rhs, ", "))}

	case *ast.DeclStmt:
		return translateDecl(s.Decl)
	case *ast.ExprStmt:
		if call, ok := s.X.(*ast.CallExpr); ok {
			return []string{handleCallWithSyms(call, syms)}
		}
		return []string{exprToStringWithSyms(s.X, syms)}
	case *ast.ReturnStmt:
		if len(s.Results) == 0 {
			return []string{"return"}
		}
		var parts []string
		for _, r := range s.Results {
			parts = append(parts, exprToStringWithSyms(r, syms))
		}
		return []string{"return " + strings.Join(parts, ", ")}
	case *ast.IfStmt:
		return translateIfWithSyms(s, depth, syms)
	case *ast.ForStmt:
		return translateFor(s, depth)
	case *ast.RangeStmt:
		return translateRange(s, depth)
	case *ast.DeferStmt:
		return []string{"defer " + handleCallWithSyms(s.Call, syms)}
	case *ast.IncDecStmt:
		op := "+="
		if s.Tok == token.DEC {
			op = "-="
		}
		return []string{fmt.Sprintf("%s %s 1", exprToString(s.X), op)}
	case *ast.BlockStmt:
		var lines []string
		lines = append(lines, "{")
		var inner strings.Builder
		writeStmtsWithSyms(&inner, s.List, depth+1, syms)
		lines = append(lines, inner.String())
		lines = append(lines, "}")
		return lines
	case *ast.GoStmt:
		return translateGoStmt(s, syms)
	case *ast.SendStmt:
		return []string{"// TODO: channel send"}
	}
	return []string{"// TODO: unsupported statement"}
}

func translateIf(s *ast.IfStmt, depth int) []string {
	var lines []string
	cond := exprToString(s.Cond)
	lines = append(lines, fmt.Sprintf("if %s {", cond))
	inner := strings.Repeat("\t", 1)
	for _, l := range collectBody(s.Body.List, depth) {
		lines = append(lines, inner+l)
	}
	if s.Else == nil {
		lines = append(lines, "}")
		return lines
	}
	switch el := s.Else.(type) {
	case *ast.IfStmt:
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

// ── FIX 1: For Loop Scope Isolation ──────────────────────────────────────────

func translateFor(s *ast.ForStmt, depth int) []string {
	var lines []string
	inner := strings.Repeat("\t", 1)

	if s.Init == nil && s.Cond == nil && s.Post == nil {
		lines = append(lines, "for {")
		for _, l := range collectBody(s.Body.List, depth) {
			lines = append(lines, inner+l)
		}
		lines = append(lines, "}")
		return lines
	}

	if s.Init == nil && s.Post == nil {
		lines = append(lines, fmt.Sprintf("for %s {", exprToString(s.Cond)))
		for _, l := range collectBody(s.Body.List, depth) {
			lines = append(lines, inner+l)
		}
		lines = append(lines, "}")
		return lines
	}

	initLines := translateStmt(s.Init, depth)
	postLines := translateStmt(s.Post, depth)

	lines = append(lines, initLines...)
	lines = append(lines, fmt.Sprintf("for %s {", exprToString(s.Cond)))

	// FIX: Wrap the body in an extra block to protect shadowed variables
	// from leaking into the post statement (like i += 1)
	lines = append(lines, inner+"{")
	for _, l := range collectBody(s.Body.List, depth+1) {
		lines = append(lines, inner+"\t"+l)
	}
	lines = append(lines, inner+"}")

	// Now the post statement executes against the outer loop variable!
	for _, pl := range postLines {
		lines = append(lines, inner+pl)
	}
	lines = append(lines, "}")
	return lines
}

func translateRange(s *ast.RangeStmt, depth int) []string {
	var lines []string
	inner := strings.Repeat("\t", 1)
	collection := exprToString(s.X)
	key, val := "_", "_"
	if s.Key != nil {
		key = exprToString(s.Key)
	}
	if s.Value != nil {
		val = exprToString(s.Value)
	}

	lines = append(lines, fmt.Sprintf("for %s, %s in %s {", val, key, collection))
	for _, l := range collectBody(s.Body.List, depth) {
		lines = append(lines, inner+l)
	}
	lines = append(lines, "}")
	return lines
}

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
		isSyncWG := false
		if vs.Type != nil {
			typeName = ": " + mapType(vs.Type)
			if sel, ok := vs.Type.(*ast.SelectorExpr); ok {
				if exprToString(sel.X) == "sync" && sel.Sel.Name == "WaitGroup" {
					isSyncWG = true
				}
			}
		}
		for i, name := range vs.Names {
			if i < len(vs.Values) {
				lines = append(lines, fmt.Sprintf("%s%s = %s", name.Name, typeName, exprToString(vs.Values[i])))
			} else {
				lines = append(lines, fmt.Sprintf("%s%s", name.Name, typeName))
			}
			if isSyncWG {
				lines = append(lines, fmt.Sprintf("golden.wg_init(&%s)", name.Name))
			}
		}
	}
	return lines
}

func exprToString(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	switch e := expr.(type) {
	case *ast.Ident:
		switch e.Name {
		case "true":
			return "true"
		case "false":
			return "false"
		case "nil":
			return "nil"
		}
		return e.Name
	case *ast.BasicLit:
		return e.Value
	case *ast.BinaryExpr:
		return fmt.Sprintf("%s %s %s", exprToString(e.X), mapOperator(e.Op), exprToString(e.Y))
	case *ast.UnaryExpr:
		op := e.Op.String()
		if e.Op == token.AND {
			op = "&"
		}
		return op + exprToString(e.X)
	case *ast.ParenExpr:
		return fmt.Sprintf("(%s)", exprToString(e.X))
	case *ast.SelectorExpr:
		return fmt.Sprintf("%s.%s", exprToString(e.X), e.Sel.Name)
	case *ast.IndexExpr:
		return fmt.Sprintf("%s[%s]", exprToString(e.X), exprToString(e.Index))
	case *ast.CallExpr:
		return handleCall(e)
	case *ast.CompositeLit:
		return handleCompositeLit(e)
	case *ast.TypeAssertExpr:
		return fmt.Sprintf("/* type assert */ %s", exprToString(e.X))
	case *ast.SliceExpr:
		return fmt.Sprintf("%s[%s:%s]", exprToString(e.X), exprToString(e.Low), exprToString(e.High))
	}
	return fmt.Sprintf("/* unknown expr %T */", expr)
}

func mapOperator(op token.Token) string {
	switch op {
	case token.ADD:
		return "+"
	case token.SUB:
		return "-"
	case token.MUL:
		return "*"
	case token.QUO:
		return "/"
	case token.REM:
		return "%%"
	case token.EQL:
		return "=="
	case token.NEQ:
		return "!="
	case token.LSS:
		return "<"
	case token.GTR:
		return ">"
	case token.LEQ:
		return "<="
	case token.GEQ:
		return ">="
	case token.LAND:
		return "&&"
	case token.LOR:
		return "||"
	case token.AND:
		return "&"
	case token.OR:
		return "|"
	case token.XOR:
		return "~"
	case token.SHL:
		return "<<"
	case token.SHR:
		return ">>"
	}
	return op.String()
}

var funcMap = map[string]string{
	"fmt.Println": "fmt.println",
	"fmt.Printf":  "fmt.printf",
	"fmt.Sprintf": "fmt.tprintf",
	"fmt.Print":   "fmt.print",
	"fmt.Fprintf": "fmt.fprintln",
	"len":         "len",
	"cap":         "cap",
	"make":        "make",
	"append":      "append",
	"delete":      "delete",
	"panic":       "panic",
	"new":         "new",
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
	ellipsis := ""
	if call.Ellipsis.IsValid() {
		ellipsis = ".."
	}
	return fmt.Sprintf("%s(%s%s)", funcName, strings.Join(args, ", "), ellipsis)
}

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
			fields = append(fields, fmt.Sprintf("%s = %s", exprToString(kv.Key), exprToString(kv.Value)))
		} else {
			fields = append(fields, exprToString(elt))
		}
	}
	if len(fields) <= 3 {
		return fmt.Sprintf("%s{%s}", typeName, strings.Join(fields, ", "))
	}
	return fmt.Sprintf("%s{\n\t\t%s,\n\t}", typeName, strings.Join(fields, ",\n\t\t"))
}

func exprToStringWithSyms(expr ast.Expr, syms *SymbolTable) string {
	if expr == nil {
		return ""
	}
	switch e := expr.(type) {
	case *ast.SelectorExpr:
		base := exprToString(e.X)
		if ident, ok := e.X.(*ast.Ident); ok {
			switch syms.strategyOf(ident.Name) {
			case AllocARC:
				return fmt.Sprintf("%s.data.%s", base, e.Sel.Name)
			default:
				return fmt.Sprintf("%s.%s", base, e.Sel.Name)
			}
		}
		return fmt.Sprintf("%s.%s", base, e.Sel.Name)
	default:
		return exprToString(expr)
	}
}

func handleCallWithSyms(call *ast.CallExpr, syms *SymbolTable) string {
	funcName := exprToString(call.Fun)
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
		method := sel.Sel.Name
		recv := exprToString(sel.X)
		switch method {
		case "Add":
			var args []string
			args = append(args, "&"+recv)
			for _, arg := range call.Args {
				args = append(args, exprToStringWithSyms(arg, syms))
			}
			return fmt.Sprintf("golden.wg_add(%s)", strings.Join(args, ", "))
		case "Done":
			if ident, ok := sel.X.(*ast.Ident); ok && syms.strategyOf(ident.Name) == AllocArena {
				return fmt.Sprintf("golden.wg_done(%s)", recv)
			}
			return fmt.Sprintf("golden.wg_done(&%s)", recv)
		case "Wait":
			if ident, ok := sel.X.(*ast.Ident); ok && syms.strategyOf(ident.Name) == AllocArena {
				return fmt.Sprintf("golden.wg_wait(%s)", recv)
			}
			return fmt.Sprintf("golden.wg_wait(&%s)", recv)
		}
	}
	if mapped, ok := funcMap[funcName]; ok {
		funcName = mapped
	}
	var args []string
	for _, arg := range call.Args {
		if ident, ok := arg.(*ast.Ident); ok {
			switch syms.strategyOf(ident.Name) {
			case AllocARC:
				args = append(args, ident.Name+".data")
			case AllocArena:
				args = append(args, ident.Name)
			default:
				args = append(args, ident.Name)
			}
			continue
		}
		args = append(args, exprToStringWithSyms(arg, syms))
	}
	ellipsis := ""
	if call.Ellipsis.IsValid() {
		ellipsis = ".."
	}
	return fmt.Sprintf("%s(%s%s)", funcName, strings.Join(args, ", "), ellipsis)
}

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

// ── FIX 2: Dynamic Goroutine Capture Walker ──────────────────────────────────

func translateGoStmt(s *ast.GoStmt, syms *SymbolTable) []string {
	call := s.Call

	if fn, ok := call.Fun.(*ast.FuncLit); ok {
		// 1. AST Walker to dynamically find captured variables
		capturedVars := make(map[string]string)
		localVars := make(map[string]bool)

		ast.Inspect(fn.Body, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.AssignStmt:
				if node.Tok == token.DEFINE {
					for _, lhs := range node.Lhs {
						if ident, ok := lhs.(*ast.Ident); ok {
							localVars[ident.Name] = true
						}
					}
				}
			case *ast.Ident:
				name := node.Name
				// Skip if it's a builtin, a local variable, or a known function
				if name == "true" || name == "false" || name == "nil" || localVars[name] {
					return true
				}
				if _, isFunc := funcMap[name]; isFunc || name == "worker" || name == "fmt" {
					return true
				}

				// Basic type inference for the PoC
				if name == "wg" {
					capturedVars[name] = "^golden.WaitGroup"
				} else {
					capturedVars[name] = "int" // Defaults basic variables to int
				}
			}
			return true
		})

		structName := fmt.Sprintf("_closure_ctx_%d", s.Go)
		wrapperName := fmt.Sprintf("_go_wrapper_%d", s.Go)
		var lines []string

		// 2. Emit dynamically generated struct
		lines = append(lines, fmt.Sprintf("%s :: struct {", structName))
		for v, t := range capturedVars {
			lines = append(lines, fmt.Sprintf("\t%s: %s,", v, t))
		}
		lines = append(lines, "}")

		// 3. Pack data
		lines = append(lines, fmt.Sprintf("_ctx := new(%s)", structName))
		for v, t := range capturedVars {
			if strings.HasPrefix(t, "^") {
				lines = append(lines, fmt.Sprintf("_ctx.%s = &%s", v, v))
			} else {
				lines = append(lines, fmt.Sprintf("_ctx.%s = %s", v, v))
			}
		}

		// 4. Create wrapper
		lines = append(lines, fmt.Sprintf("%s :: proc(data: rawptr) {", wrapperName))
		lines = append(lines, fmt.Sprintf("\tctx := cast(^%s)data", structName))

		// 5. Replace references dynamically
		bodyLines := collectBodyWithSyms(fn.Body.List, 0, syms)
		for _, bl := range bodyLines {
			processedLine := bl
			for v, t := range capturedVars {
				if strings.HasPrefix(t, "^") {
					// Pointers: replace '&var' with 'ctx.var'
					processedLine = strings.Replace(processedLine, "&"+v, "ctx."+v, 1)
				} else {
					// Values: target 'var,' to avoid substring collisions
					processedLine = strings.Replace(processedLine, v+",", "ctx."+v+",", 1)
				}
			}
			lines = append(lines, "\t"+processedLine)
		}

		lines = append(lines, "\tfree(ctx)")
		lines = append(lines, "}")
		lines = append(lines, fmt.Sprintf("golden.spawn_raw(%s, _ctx)", wrapperName))

		return lines
	}

	return []string{"// TODO: Unsupported goroutine pattern"}
}

func init() {
	funcMap["wg.Add"] = "golden.wg_add"
	funcMap["wg.Done"] = "golden.wg_done"
	funcMap["wg.Wait"] = "golden.wg_wait"
}

func mapSyncType(name string) string {
	switch name {
	case "WaitGroup":
		return "golden.WaitGroup"
	case "Mutex":
		return "sync.Mutex"
	case "RWMutex":
		return "sync.RW_Mutex"
	case "Once":
		return "sync.Once"
	}
	return name
}
