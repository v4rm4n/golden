// --- golden/internal/transpiler/transpiler.go ---

package transpiler

import (
	"fmt"
	"go/ast"
	"go/token"
	"regexp"
	"strings"
)

var funcReturnTypes = map[string]string{}
var methodIsPointer = map[string]bool{}

// ── Top-level processor ──────────────────────────────────────────────────────

func Process(f *ast.File) string {
	funcReturnTypes = make(map[string]string)
	methodIsPointer = make(map[string]bool)

	res := NewResolver()
	res.File = f
	res.PopulateImports(f)

	// PASS 1: The Census (Global Symbol Registration & Method Tracking)
	for _, decl := range f.Decls {
		if fd, ok := decl.(*ast.FuncDecl); ok {
			// Track if methods use pointer receivers
			if fd.Recv != nil && len(fd.Recv.List) > 0 {
				_, isPtr := fd.Recv.List[0].Type.(*ast.StarExpr)
				methodIsPointer[fd.Name.Name] = isPtr
			}
			// Track return types for ARC routing
			if fd.Type.Results != nil && len(fd.Type.Results.List) > 0 {
				first := fd.Type.Results.List[0].Type
				if star, ok := first.(*ast.StarExpr); ok {
					funcReturnTypes[fd.Name.Name] = mapType(star.X)
				}
			}
			// Register Function in Resolver
			res.Define(fd.Name.Name, &Symbol{Name: fd.Name.Name, GoType: "proc", IsGlobal: true})

		} else if gd, ok := decl.(*ast.GenDecl); ok {
			for _, spec := range gd.Specs {
				if ts, ok := spec.(*ast.TypeSpec); ok {
					res.Define(ts.Name.Name, &Symbol{Name: ts.Name.Name, GoType: "struct", IsGlobal: true})
				}
			}
		}
	}

	// PASS 2: The Alchemy (Translation)
	var sb strings.Builder
	sb.WriteString("package main\n\n")

	// IMPORT CORE:MEM ALWAYS (since main uses it now)
	sb.WriteString("import \"core:mem\"\n")
	sb.WriteString("import \"core:fmt\"\n")

	if _, hasOs := res.Imports["os"]; hasOs {
		sb.WriteString("import \"core:os\"\n")
		sb.WriteString("import golden_os \"golden/runtime\"\n")
	}
	if _, hasSync := res.Imports["sync"]; hasSync {
		sb.WriteString("import \"core:sync\"\n")
	}
	sb.WriteString("import golden \"golden\"\n\n")

	for _, decl := range f.Decls {
		var output string
		switch d := decl.(type) {
		case *ast.GenDecl:
			output = handleStruct(d)
		case *ast.FuncDecl:
			output = handleFuncWithResolver(d, res)
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
			return "cstring"
		default:
			return t.Name
		}
	case *ast.StarExpr:
		return "^" + mapType(t.X)
	case *ast.ArrayType:
		if t.Len == nil {
			return fmt.Sprintf("[dynamic]%s", mapType(t.Elt))
		}
		return fmt.Sprintf("[%s]%s", exprToStrBasic(t.Len), mapType(t.Elt))
	case *ast.MapType:
		return fmt.Sprintf("map[%s]%s", mapType(t.Key), mapType(t.Value))
	case *ast.SelectorExpr:
		pkg := exprToStrBasic(t.X)
		name := t.Sel.Name
		if pkg == "sync" {
			return mapSyncType(name)
		}
		return pkg + "." + name
	case *ast.ChanType:
		return fmt.Sprintf("^golden.Channel(%s)", mapType(t.Value))
	}
	return "rawptr"
}

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

// ── Function Handler ─────────────────────────────────────────────────────────

func handleFuncWithResolver(d *ast.FuncDecl, res *Resolver) string {
	res.EnterScope()
	defer res.ExitScope()

	var params []string
	funcName := d.Name.Name
	needsFrame := false

	// Handle Receiver
	if d.Recv != nil && len(d.Recv.List) > 0 {
		recv := d.Recv.List[0]
		recvType := mapType(recv.Type)
		recvName := "self"
		if len(recv.Names) > 0 {
			recvName = recv.Names[0].Name
		}
		structName := strings.TrimPrefix(recvType, "^")
		funcName = fmt.Sprintf("%s_%s", structName, d.Name.Name)
		params = append(params, fmt.Sprintf("%s: %s", recvName, recvType))
		res.Define(recvName, &Symbol{Name: recvName, GoType: recvType, Strategy: AllocNone})
	}

	// Handle Parameters
	if d.Type.Params != nil {
		for _, field := range d.Type.Params.List {
			pType := mapType(field.Type)
			for _, pName := range field.Names {
				strategy := AllocNone
				if _, ok := field.Type.(*ast.StarExpr); ok {
					strategy = AllocArena
					needsFrame = true
				}
				params = append(params, fmt.Sprintf("%s: %s", pName.Name, pType))
				res.Define(pName.Name, &Symbol{Name: pName.Name, GoType: pType, Strategy: strategy})
			}
		}
	}

	if d.Body != nil {
		escapes := AnalyzeFunc(d.Body)
		for _, stmt := range d.Body.List {
			// ... (existing logic for other statements) ...

			if assign, ok := stmt.(*ast.AssignStmt); ok {
				if len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
					continue
				}
				varName := exprToStrBasic(assign.Lhs[0])

				// A. Handle &Struct{} allocations
				if unary, ok := assign.Rhs[0].(*ast.UnaryExpr); ok && unary.Op == token.AND {
					if lit, ok := unary.X.(*ast.CompositeLit); ok {
						strat := AllocArena
						if escapes[varName] {
							strat = AllocARC
						}
						if strat == AllocArena {
							needsFrame = true
						}
						res.Define(varName, &Symbol{Name: varName, GoType: mapType(lit.Type), Strategy: strat})
					}
					continue
				}

				// B. Handle make() calls (like channels and slices)
				if call, ok := assign.Rhs[0].(*ast.CallExpr); ok {
					funcName := exprToStrBasic(call.Fun)
					if funcName == "make" && len(call.Args) > 0 {
						if chanType, isChan := call.Args[0].(*ast.ChanType); isChan {
							// Register the channel in the resolver!
							res.Define(varName, &Symbol{
								Name:     varName,
								GoType:   "^golden.Channel(" + mapType(chanType.Value) + ")",
								Strategy: AllocNone,
							})
							continue
						}
						if arrayType, isArray := call.Args[0].(*ast.ArrayType); isArray {
							res.Define(varName, &Symbol{
								Name:     varName,
								GoType:   "[dynamic]" + mapType(arrayType.Elt),
								Strategy: AllocNone,
							})
							continue
						}
					}
					// Handle normal function calls (check return type map)
					if retTypeName, ok := funcReturnTypes[funcName]; ok {
						res.Define(varName, &Symbol{Name: varName, GoType: retTypeName, Strategy: AllocARC})
						continue
					}
				}

				// C. Handle standard Struct{} initialization (worker := Worker{})
				if lit, ok := assign.Rhs[0].(*ast.CompositeLit); ok {
					res.Define(varName, &Symbol{
						Name:     varName,
						GoType:   mapType(lit.Type),
						Strategy: AllocNone,
					})
					continue
				}
			}
		}
	}

	retType := ""
	if d.Type.Results != nil && len(d.Type.Results.List) > 0 {
		var rets []string
		for _, r := range d.Type.Results.List {
			if star, ok := r.Type.(*ast.StarExpr); ok {
				innerType := mapType(star.X)

				// Scan current scope to see if we mapped an escaping var to ARC
				hasEscapingArc := false
				for _, sym := range res.Current.Symbols {
					if sym.Strategy == AllocARC && sym.GoType == innerType {
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
	sb.WriteString(fmt.Sprintf("%s :: proc(%s)%s {\n", funcName, strings.Join(params, ", "), retType))

	if d.Body != nil {
		if d.Name.Name == "main" {
			// INJECT ODIN'S TRACKING ALLOCATOR
			sb.WriteString("\ttrack: mem.Tracking_Allocator\n")
			sb.WriteString("\tmem.tracking_allocator_init(&track, context.allocator)\n")
			sb.WriteString("\tcontext.allocator = mem.tracking_allocator(&track)\n")
			sb.WriteString("\tdefer mem.tracking_allocator_destroy(&track)\n")
			sb.WriteString("\tdefer {\n")
			sb.WriteString("\t\tif len(track.allocation_map) > 0 {\n")
			sb.WriteString("\t\t\tfmt.eprintf(\"\\n=== MEMORY LEAK DETECTED: %v allocations not freed ===\\n\", len(track.allocation_map))\n")
			sb.WriteString("\t\t\tfor _, entry in track.allocation_map {\n")
			sb.WriteString("\t\t\t\tfmt.eprintf(\"- %v bytes @ %v\\n\", entry.size, entry.location)\n")
			sb.WriteString("\t\t\t}\n")
			sb.WriteString("\t\t}\n")
			sb.WriteString("\t\tif len(track.bad_free_array) > 0 {\n")
			sb.WriteString("\t\t\tfmt.eprintf(\"\\n=== BAD FREES DETECTED: %v incorrect frees ===\\n\", len(track.bad_free_array))\n")
			sb.WriteString("\t\t\tfor entry in track.bad_free_array {\n")
			sb.WriteString("\t\t\t\tfmt.eprintf(\"- %p @ %v\\n\", entry.memory, entry.location)\n")
			sb.WriteString("\t\t\t}\n")
			sb.WriteString("\t\t}\n")
			sb.WriteString("\t}\n\n")

			sb.WriteString("\tgolden.pool_start(8)\n\tdefer golden.pool_stop()\n")
		}
		if needsFrame {
			sb.WriteString("\t_frame := golden.frame_begin()\n\tdefer golden.frame_end(&_frame)\n")
		}
		writeStmtsWithResolver(&sb, d.Body.List, 1, res)
	}

	sb.WriteString("}")
	return sb.String()
}

// ── Statement Writer ─────────────────────────────────────────

func writeStmtsWithResolver(sb *strings.Builder, stmts []ast.Stmt, depth int, res *Resolver) {
	for _, stmt := range stmts {
		if block, ok := stmt.(*ast.BlockStmt); ok {
			res.EnterScope()
			writeStmtsWithResolver(sb, block.List, depth, res)
			res.ExitScope()
			continue
		}
		lines := translateStmtWithResolver(stmt, depth, res)
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

func collectBodyWithResolver(stmts []ast.Stmt, depth int, res *Resolver) []string {
	var sb strings.Builder
	writeStmtsWithResolver(&sb, stmts, depth+1, res)
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

func translateStmtWithResolver(stmt ast.Stmt, depth int, res *Resolver) []string {
	switch s := stmt.(type) {

	case *ast.AssignStmt:
		// Explicitly register short-variable declarations (:=) so closures can capture them
		if s.Tok == token.DEFINE {
			for i, l := range s.Lhs {
				if ident, ok := l.(*ast.Ident); ok {
					if _, exists := res.Current.Symbols[ident.Name]; !exists {
						goType := "int" // Default fallback
						if i < len(s.Rhs) {
							if lit, ok := s.Rhs[i].(*ast.BasicLit); ok {
								if lit.Kind == token.STRING {
									goType = "string"
								}
								if lit.Kind == token.FLOAT {
									goType = "f64"
								}
							} else if rhsId, ok := s.Rhs[i].(*ast.Ident); ok {
								if sym, ok := res.Lookup(rhsId.Name); ok {
									goType = sym.GoType
								}
							} else if call, ok := s.Rhs[i].(*ast.CallExpr); ok {
								funcName := exprToStrBasic(call.Fun)
								if retType, ok := funcReturnTypes[funcName]; ok {
									goType = retType
								}
							}
						}
						res.Define(ident.Name, &Symbol{Name: ident.Name, GoType: goType})
					}
				}
			}
		}

		if len(s.Lhs) == 1 && len(s.Rhs) == 1 {
			varName := exprToStr(s.Lhs[0], res)
			sym, exists := res.Lookup(varName)

			if unary, ok := s.Rhs[0].(*ast.UnaryExpr); ok && unary.Op == token.AND {
				if lit, ok := unary.X.(*ast.CompositeLit); ok {
					litStr := handleCompositeLit(lit, res)
					typeName := mapType(lit.Type)

					if exists && sym.Strategy == AllocArena {
						return []string{
							fmt.Sprintf("%s %s golden.frame_new(%s{}, &_frame)", varName, s.Tok.String(), typeName),
							fmt.Sprintf("golden.frame_init(%s, %s)", varName, litStr),
						}
					} else {
						// ARC Logic
						assignStr := fmt.Sprintf("%s %s golden.make_arc(%s)", varName, s.Tok.String(), litStr)
						out := []string{assignStr}

						// Get the parent function to check for returns
						if !isReturningVar(varName, getParentFunc(s, res.File)) { // Pass the whole function body
							out = append(out, fmt.Sprintf("defer golden.arc_release(&%s)", varName))
						}
						return out
					}
				}
			}

			if call, ok := s.Rhs[0].(*ast.CallExpr); ok {
				funcName := exprToStrBasic(call.Fun)
				if funcName == "append" {
					sliceName := exprToStr(call.Args[0], res)
					var args []string
					args = append(args, "&"+sliceName)
					for i := 1; i < len(call.Args); i++ {
						args = append(args, exprToStr(call.Args[i], res))
					}
					return []string{fmt.Sprintf("append(%s)", strings.Join(args, ", "))}
				}
				if funcName == "make" {
					if chanType, isChan := call.Args[0].(*ast.ChanType); isChan {
						res.Define(varName, &Symbol{
							Name:     varName,
							GoType:   "^golden.Channel(" + mapType(chanType.Value) + ")",
							Strategy: AllocNone,
						})
						assignStr := fmt.Sprintf("%s %s golden.chan_make(%s)", varName, s.Tok.String(), mapType(chanType.Value))
						return []string{assignStr, fmt.Sprintf("defer free(%s)", varName)}
					}
					if _, isArray := call.Args[0].(*ast.ArrayType); isArray {
						assignStr := fmt.Sprintf("%s %s %s", varName, s.Tok.String(), handleCallWithResolver(call, res))
						return []string{assignStr, fmt.Sprintf("defer delete(%s)", varName)}
					}
				}
				// Handle normal function calls returning ARC pointers
				if retTypeName, ok := funcReturnTypes[funcName]; ok {
					res.Define(varName, &Symbol{Name: varName, GoType: retTypeName, Strategy: AllocARC})
					assignStr := fmt.Sprintf("%s %s %s", varName, s.Tok.String(), handleCallWithResolver(call, res))
					out := []string{assignStr}

					// APPLY FIX HERE TOO:
					if !isReturningVar(varName, getParentFunc(s, res.File)) {
						out = append(out, fmt.Sprintf("defer golden.arc_release(&%s)", varName))
					}
					return out
				}
			}
		}

		var lhs, rhs []string
		var defers []string // Array to hold our injected cleanups

		for _, l := range s.Lhs {
			name := exprToStr(l, res)
			lhs = append(lhs, name)
			// FIX 3: If the variable looks like an error, auto-delete it at end of scope
			// In Odin, `delete(nil)` is perfectly safe, so this works flawlessly.
			if strings.Contains(name, "err") {
				defers = append(defers, fmt.Sprintf("defer delete(%s)", name))
			}
		}
		for _, r := range s.Rhs {
			rhs = append(rhs, exprToStr(r, res))
		}

		out := []string{fmt.Sprintf("%s %s %s", strings.Join(lhs, ", "), s.Tok.String(), strings.Join(rhs, ", "))}
		out = append(out, defers...) // Inject our cleanups right after the assignment
		return out

	case *ast.DeclStmt:
		return translateDecl(s.Decl, res)
	case *ast.ExprStmt:
		if call, ok := s.X.(*ast.CallExpr); ok {
			return []string{handleCallWithResolver(call, res)}
		}
		return []string{exprToStr(s.X, res)}
	case *ast.ReturnStmt:
		if len(s.Results) == 0 {
			return []string{"return"}
		}
		var parts []string
		for _, r := range s.Results {
			parts = append(parts, exprToStr(r, res))
		}
		return []string{"return " + strings.Join(parts, ", ")}
	case *ast.IfStmt:
		return translateIfWithResolver(s, depth, res)
	case *ast.ForStmt:
		return translateForWithResolver(s, depth, res)
	case *ast.RangeStmt:
		return translateRangeWithResolver(s, depth, res)
	case *ast.DeferStmt:
		return []string{"defer " + handleCallWithResolver(s.Call, res)}
	case *ast.IncDecStmt:
		op := "+="
		if s.Tok == token.DEC {
			op = "-="
		}
		return []string{fmt.Sprintf("%s %s 1", exprToStr(s.X, res), op)}
	case *ast.BlockStmt:
		var lines []string
		lines = append(lines, "{")
		res.EnterScope()
		var inner strings.Builder
		writeStmtsWithResolver(&inner, s.List, depth+1, res)
		res.ExitScope()
		lines = append(lines, inner.String())
		lines = append(lines, "}")
		return lines
	case *ast.GoStmt:
		return translateGoStmtWithResolver(s, res)
	case *ast.SendStmt:
		ch := exprToStr(s.Chan, res)
		val := exprToStr(s.Value, res)
		return []string{fmt.Sprintf("golden.chan_send(%s, %s)", ch, val)}
	}
	return []string{"// TODO: unsupported statement"}
}

func translateIfWithResolver(s *ast.IfStmt, depth int, res *Resolver) []string {
	var lines []string
	inner := strings.Repeat("\t", 1)
	cond := exprToStr(s.Cond, res)
	lines = append(lines, fmt.Sprintf("if %s {", cond))

	res.EnterScope()
	for _, l := range collectBodyWithResolver(s.Body.List, depth, res) {
		lines = append(lines, inner+l)
	}
	res.ExitScope()

	if s.Else == nil {
		lines = append(lines, "}")
		return lines
	}
	switch el := s.Else.(type) {
	case *ast.IfStmt:
		elseLines := translateIfWithResolver(el, depth, res)
		lines = append(lines, "} else "+elseLines[0])
		lines = append(lines, elseLines[1:]...)
	case *ast.BlockStmt:
		lines = append(lines, "} else {")
		res.EnterScope()
		for _, l := range collectBodyWithResolver(el.List, depth, res) {
			lines = append(lines, inner+l)
		}
		res.ExitScope()
		lines = append(lines, "}")
	}
	return lines
}

func translateForWithResolver(s *ast.ForStmt, depth int, res *Resolver) []string {
	var lines []string
	inner := strings.Repeat("\t", 1)

	res.EnterScope()
	defer res.ExitScope()

	if s.Init == nil && s.Cond == nil && s.Post == nil {
		lines = append(lines, "for {")
		for _, l := range collectBodyWithResolver(s.Body.List, depth, res) {
			lines = append(lines, inner+l)
		}
		lines = append(lines, "}")
		return lines
	}

	if s.Init == nil && s.Post == nil {
		lines = append(lines, fmt.Sprintf("for %s {", exprToStr(s.Cond, res)))
		for _, l := range collectBodyWithResolver(s.Body.List, depth, res) {
			lines = append(lines, inner+l)
		}
		lines = append(lines, "}")
		return lines
	}

	initLines := translateStmtWithResolver(s.Init, depth, res)
	postLines := translateStmtWithResolver(s.Post, depth, res)

	lines = append(lines, initLines...)
	lines = append(lines, fmt.Sprintf("for %s {", exprToStr(s.Cond, res)))

	lines = append(lines, inner+"{")
	for _, l := range collectBodyWithResolver(s.Body.List, depth+1, res) {
		lines = append(lines, inner+"\t"+l)
	}
	lines = append(lines, inner+"}")

	for _, pl := range postLines {
		lines = append(lines, inner+pl)
	}
	lines = append(lines, "}")
	return lines
}

func translateRangeWithResolver(s *ast.RangeStmt, depth int, res *Resolver) []string {
	var lines []string
	inner := strings.Repeat("\t", 1)
	collection := exprToStr(s.X, res)
	key, val := "_", "_"
	if s.Key != nil {
		key = exprToStr(s.Key, res)
	}
	if s.Value != nil {
		val = exprToStr(s.Value, res)
	}

	lines = append(lines, fmt.Sprintf("for %s, %s in %s {", val, key, collection))
	res.EnterScope()
	for _, l := range collectBodyWithResolver(s.Body.List, depth, res) {
		lines = append(lines, inner+l)
	}
	res.ExitScope()
	lines = append(lines, "}")
	return lines
}

func translateDecl(decl ast.Decl, res *Resolver) []string {
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
		mappedType := "" // We need to store this for the Resolver!
		isSyncWG := false

		if vs.Type != nil {
			mappedType = mapType(vs.Type)
			typeName = ": " + mappedType
			if sel, ok := vs.Type.(*ast.SelectorExpr); ok {
				if exprToStrBasic(sel.X) == "sync" && sel.Sel.Name == "WaitGroup" {
					isSyncWG = true
				}
			}
		}

		for i, name := range vs.Names {
			if i < len(vs.Values) {
				lines = append(lines, fmt.Sprintf("%s%s = %s", name.Name, typeName, exprToStr(vs.Values[i], res)))
			} else {
				lines = append(lines, fmt.Sprintf("%s%s", name.Name, typeName))
			}
			if isSyncWG {
				lines = append(lines, fmt.Sprintf("golden.wg_init(&%s)", name.Name))
			}

			// FIX: Actually register the GoType so closures can resolve it!
			res.Define(name.Name, &Symbol{Name: name.Name, GoType: mappedType})
		}
	}
	return lines
}

func exprToStrBasic(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	if ident, ok := expr.(*ast.Ident); ok {
		return ident.Name
	}
	if sel, ok := expr.(*ast.SelectorExpr); ok {
		return exprToStrBasic(sel.X) + "." + sel.Sel.Name
	}
	return ""
}

func exprToStr(expr ast.Expr, res *Resolver) string {
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
		return fmt.Sprintf("%s %s %s", exprToStr(e.X, res), mapOperator(e.Op), exprToStr(e.Y, res))
	case *ast.UnaryExpr:
		if e.Op == token.ARROW {
			return fmt.Sprintf("golden.chan_recv(%s)", exprToStr(e.X, res))
		}
		op := e.Op.String()
		if e.Op == token.AND {
			op = "&"
		}
		return op + exprToStr(e.X, res)
	case *ast.ParenExpr:
		return fmt.Sprintf("(%s)", exprToStr(e.X, res))
	case *ast.SelectorExpr:
		base := exprToStr(e.X, res)
		if ident, ok := e.X.(*ast.Ident); ok {
			if sym, ok := res.Lookup(ident.Name); ok && sym.Strategy == AllocARC {
				return fmt.Sprintf("%s.data.%s", base, e.Sel.Name) // ARC Deref
			}
		}
		return fmt.Sprintf("%s.%s", base, e.Sel.Name)
	case *ast.IndexExpr:
		return fmt.Sprintf("%s[%s]", exprToStr(e.X, res), exprToStr(e.Index, res))
	case *ast.CallExpr:
		return handleCallWithResolver(e, res)
	case *ast.CompositeLit:
		return handleCompositeLit(e, res)
	case *ast.TypeAssertExpr:
		return fmt.Sprintf("/* type assert */ %s", exprToStr(e.X, res))
	case *ast.SliceExpr:
		return fmt.Sprintf("%s[%s:%s]", exprToStr(e.X, res), exprToStr(e.Low, res), exprToStr(e.High, res))
	case *ast.ArrayType:
		return mapType(e)
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
	default:
		return op.String()
	}
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

func handleCallWithResolver(call *ast.CallExpr, res *Resolver) string {
	funcNameBasic := exprToStrBasic(call.Fun)

	// Channel Make Hook
	if funcNameBasic == "make" && len(call.Args) > 0 {
		if chanType, isChan := call.Args[0].(*ast.ChanType); isChan {
			return fmt.Sprintf("golden.chan_make(%s)", mapType(chanType.Value))
		}
	}

	// FIX 2: Intercept global overrides like 'errors.New' BEFORE treating them as struct methods
	if mapped, ok := funcMap[funcNameBasic]; ok {
		var args []string
		for _, arg := range call.Args {
			if ident, ok := arg.(*ast.Ident); ok {
				if sym, ok := res.Lookup(ident.Name); ok && sym.Strategy == AllocARC {
					args = append(args, ident.Name+".data")
					continue
				}
			}
			args = append(args, exprToStr(arg, res))
		}
		ellipsis := ""
		if call.Ellipsis.IsValid() {
			ellipsis = ".."
		}
		return fmt.Sprintf("%s(%s%s)", mapped, strings.Join(args, ", "), ellipsis)
	}

	// Parse Standard Struct Methods
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
		method := sel.Sel.Name
		recv := exprToStr(sel.X, res)
		recvBase := exprToStrBasic(sel.X)

		// 1. Check if it's an imported package like "os"
		if _, isImported := res.Imports[recvBase]; isImported {
			if recvBase == "os" {
				var args []string
				for _, arg := range call.Args {
					args = append(args, exprToStr(arg, res))
				}
				return fmt.Sprintf("golden_os.%s(%s)", toSnakeCase(method), strings.Join(args, ", "))
			}
		}

		// 2. WaitGroup Hacks
		switch method {
		case "Add":
			var args []string
			args = append(args, "&"+recv)
			for _, arg := range call.Args {
				args = append(args, exprToStr(arg, res))
			}
			return fmt.Sprintf("golden.wg_add(%s)", strings.Join(args, ", "))
		case "Done":
			if sym, ok := res.Lookup(recvBase); ok && sym.Strategy == AllocArena {
				return fmt.Sprintf("golden.wg_done(%s)", recv)
			}
			return fmt.Sprintf("golden.wg_done(&%s)", recv)
		case "Wait":
			if sym, ok := res.Lookup(recvBase); ok && sym.Strategy == AllocArena {
				return fmt.Sprintf("golden.wg_wait(%s)", recv)
			}
			return fmt.Sprintf("golden.wg_wait(&%s)", recv)
		default:
			// 3. Standard Method Call (Skipping standard packages)
			if recv == "fmt" || recv == "strings" || recv == "math" {
				break
			}

			structType := ""
			var args []string
			isPtr, known := methodIsPointer[method]

			if sym, ok := res.Lookup(recvBase); ok {
				// 1. Resolve Struct Name
				structType = strings.TrimPrefix(sym.GoType, "golden.Arc(")
				structType = strings.TrimSuffix(structType, ")")
				structType = strings.TrimPrefix(structType, "^")

				// 2. Resolve Receiver Argument
				isAlreadyPtr := strings.HasPrefix(sym.GoType, "^")

				if sym.Strategy == AllocARC {
					if known && !isPtr {
						args = append(args, recv+".data^") // Pass by value
					} else {
						args = append(args, recv+".data") // Pass the pointer
					}
				} else if isAlreadyPtr {
					// If it's already a pointer, just pass it. No '&' needed.
					args = append(args, recv)
				} else {
					// It's a value. Only add '&' if the method is known to want a pointer.
					if known && !isPtr {
						args = append(args, recv)
					} else {
						args = append(args, "&"+recv)
					}
				}
			} else {
				// Fallback for unknown symbols
				structType = strings.ToUpper(recvBase[:1]) + recvBase[1:]
				args = append(args, "&"+recv)
			}

			funcName := fmt.Sprintf("%s_%s", structType, method)
			for _, arg := range call.Args {
				args = append(args, exprToStr(arg, res))
			}
			return fmt.Sprintf("%s(%s)", funcName, strings.Join(args, ", "))
		}
	}

	// Unmapped Package Functions / Global Functions
	var args []string
	for _, arg := range call.Args {
		if ident, ok := arg.(*ast.Ident); ok {
			if sym, ok := res.Lookup(ident.Name); ok && sym.Strategy == AllocARC {
				args = append(args, ident.Name+".data")
				continue
			}
		}
		args = append(args, exprToStr(arg, res))
	}
	ellipsis := ""
	if call.Ellipsis.IsValid() {
		ellipsis = ".."
	}
	return fmt.Sprintf("%s(%s%s)", funcNameBasic, strings.Join(args, ", "), ellipsis)
}

func handleCompositeLit(lit *ast.CompositeLit, res *Resolver) string {
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
			fields = append(fields, fmt.Sprintf("%s = %s", exprToStr(kv.Key, res), exprToStr(kv.Value, res)))
		} else {
			fields = append(fields, exprToStr(elt, res))
		}
	}
	if len(fields) <= 3 {
		return fmt.Sprintf("%s{%s}", typeName, strings.Join(fields, ", "))
	}
	return fmt.Sprintf("%s{\n\t\t%s,\n\t}", typeName, strings.Join(fields, ",\n\t\t"))
}

// ── Dynamic Goroutine Capture Walker ──────────────────────────────────

// CaptureInfo holds the type mapping for the closure struct generator
type CaptureInfo struct {
	Type     string
	IsPtrRef bool
}

func translateGoStmtWithResolver(s *ast.GoStmt, res *Resolver) []string {
	call := s.Call
	if fn, ok := call.Fun.(*ast.FuncLit); ok {
		capturedVars := make(map[string]CaptureInfo)
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
				if name == "true" || name == "false" || name == "nil" || localVars[name] {
					return true
				}
				if _, isFunc := funcMap[name]; isFunc {
					return true
				}

				if sym, ok := res.Lookup(name); ok && !sym.IsGlobal {
					t := sym.GoType
					isPtrRef := false

					if t == "golden.WaitGroup" || t == "sync.Mutex" || t == "sync.RW_Mutex" {
						t = "^" + t
						isPtrRef = true
					}
					capturedVars[name] = CaptureInfo{Type: t, IsPtrRef: isPtrRef}
				}
			}
			return true
		})

		structName := fmt.Sprintf("_closure_ctx_%d", s.Go)
		wrapperName := fmt.Sprintf("_go_wrapper_%d", s.Go)
		ctxVar := fmt.Sprintf("_ctx_%d", s.Go)
		var lines []string

		lines = append(lines, fmt.Sprintf("%s :: struct {", structName))
		// FIX 1A: Pack the allocator into the context struct
		lines = append(lines, "\t_allocator: mem.Allocator,")
		for v, info := range capturedVars {
			lines = append(lines, fmt.Sprintf("\t%s: %s,", v, info.Type))
		}
		lines = append(lines, "}")

		lines = append(lines, fmt.Sprintf("%s := new(%s)", ctxVar, structName))
		// FIX 1B: Capture the main thread's allocator
		lines = append(lines, fmt.Sprintf("%s._allocator = context.allocator", ctxVar))

		for v, info := range capturedVars {
			if info.IsPtrRef {
				lines = append(lines, fmt.Sprintf("%s.%s = &%s", ctxVar, v, v))
			} else {
				lines = append(lines, fmt.Sprintf("%s.%s = %s", ctxVar, v, v))
			}
		}

		lines = append(lines, fmt.Sprintf("%s :: proc(data: rawptr) {", wrapperName))
		lines = append(lines, fmt.Sprintf("\tctx := cast(^%s)data", structName))

		bodyLines := collectBodyWithResolver(fn.Body.List, 0, res)
		for _, bl := range bodyLines {
			processedLine := bl
			for v, info := range capturedVars {
				re := regexp.MustCompile(`\b` + regexp.QuoteMeta(v) + `\b`)
				processedLine = re.ReplaceAllString(processedLine, "ctx."+v)
				if info.IsPtrRef || strings.HasPrefix(info.Type, "^") {
					processedLine = strings.ReplaceAll(processedLine, "&ctx."+v, "ctx."+v)
				}
			}
			lines = append(lines, "\t"+processedLine)
		}

		// FIX 1C: Tell the worker thread to free using the explicitly captured allocator
		lines = append(lines, "\tfree(ctx, ctx._allocator)")
		lines = append(lines, "}")
		lines = append(lines, fmt.Sprintf("golden.spawn_raw(%s, %s)", wrapperName, ctxVar))

		return lines
	}
	return []string{"// TODO: Unsupported goroutine pattern"}
}

func init() {
	funcMap["errors.New"] = "golden.error_new"
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

func toSnakeCase(str string) string {
	var b strings.Builder
	for i, r := range str {
		if i > 0 && r >= 'A' && r <= 'Z' {
			b.WriteRune('_')
		}
		b.WriteRune(r | 0x20)
	}
	return b.String()
}

func isReturningVar(name string, scopeStmt ast.Node) bool {
	isReturned := false

	// Walk the current function or block to see if 'name' is used in a ReturnStmt
	ast.Inspect(scopeStmt, func(n ast.Node) bool {
		if ret, ok := n.(*ast.ReturnStmt); ok {
			for _, res := range ret.Results {
				if ident, ok := res.(*ast.Ident); ok && ident.Name == name {
					isReturned = true
					return false // Found it, stop looking
				}
			}
		}
		return true
	})

	return isReturned
}

func getParentFunc(n ast.Node, file *ast.File) ast.Node {
	var parent ast.Node
	ast.Inspect(file, func(child ast.Node) bool {
		if fd, ok := child.(*ast.FuncDecl); ok {
			// Check if n is inside fd
			if n.Pos() >= fd.Pos() && n.End() <= fd.End() {
				parent = fd
			}
		}
		return true
	})
	return parent
}
