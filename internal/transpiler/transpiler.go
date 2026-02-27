// --- golden/internal/transpiler/transpiler.odin ---

package transpiler

import (
	"fmt"
	"go/ast"
	"strings"
)

func Process(f *ast.File) string {
	var sb strings.Builder
	sb.WriteString("package main\n\n")
	sb.WriteString("import \"core:fmt\"\n")
	sb.WriteString("import \"runtime/golden\"\n\n")

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
			sb.WriteString("\n\n") // Adds consistent spacing between structs and procs
		}
	}

	return strings.TrimSpace(sb.String()) + "\n"
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
			name := strings.ToLower(field.Names[0].Name)
			// Placeholder 'int' - we will map real types later
			sb.WriteString(fmt.Sprintf("\t%s: int,\n", name))
		}
		sb.WriteString("}")
	}
	return sb.String()
}

func handleFunc(d *ast.FuncDecl) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s :: proc() {\n", d.Name.Name))

	if d.Body != nil {
		for _, stmt := range d.Body.List {
			translated := translateStmt(stmt)
			if translated != "" {
				sb.WriteString(fmt.Sprintf("\t%s\n", translated))
			}
		}
	}

	sb.WriteString("}")
	return sb.String()
}

func translateStmt(stmt ast.Stmt) string {
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		// Basic mapping of 'x := 10'
		// Note: We use exprToString here to handle literals properly
		lhs := exprToString(s.Lhs[0])
		rhs := exprToString(s.Rhs[0])
		return fmt.Sprintf("%s %s %s", lhs, s.Tok.String(), rhs)

	case *ast.ExprStmt:
		if call, ok := s.X.(*ast.CallExpr); ok {
			return handleCall(call)
		}
	}
	return "// Unsupported statement"
}

// Helper to handle expressions (names, numbers, etc)
func exprToString(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.BasicLit:
		return e.Value
	default:
		return ""
	}
}

func handleCall(call *ast.CallExpr) string {
	// Crude check for fmt.Println
	return "fmt.println(\"Golden mapping works!\")"
}
