// --- golden/internal/transpiler/resolver.go ---

package transpiler

import (
	"go/ast"
	"go/token"
	"strings"
)

type AllocStrategy int

const (
	AllocNone AllocStrategy = iota
	AllocARC
	AllocArena
)

type Symbol struct {
	Name     string
	GoType   string // e.g., "int", "*User", "chan int"
	IsPtr    bool
	Escapes  bool // Result of Escape Analysis
	IsGlobal bool
	Strategy AllocStrategy
}

type Scope struct {
	Parent  *Scope
	Symbols map[string]*Symbol
}

type Resolver struct {
	File        *ast.File
	PackageName string
	Imports     map[string]string // Key: Alias/Name (os), Value: Path ("os")
	GlobalScope *Scope
	Current     *Scope
}

func NewResolver() *Resolver {
	global := &Scope{Symbols: make(map[string]*Symbol)}
	return &Resolver{
		Imports:     make(map[string]string),
		GlobalScope: global,
		Current:     global,
	}
}

func (r *Resolver) EnterScope() {
	r.Current = &Scope{
		Parent:  r.Current,
		Symbols: make(map[string]*Symbol),
	}
}

func (r *Resolver) ExitScope() {
	if r.Current.Parent != nil {
		r.Current = r.Current.Parent
	}
}

func (r *Resolver) Define(name string, sym *Symbol) {
	r.Current.Symbols[name] = sym
}

func (r *Resolver) Lookup(name string) (*Symbol, bool) {
	curr := r.Current
	for curr != nil {
		if sym, ok := curr.Symbols[name]; ok {
			return sym, true
		}
		curr = curr.Parent
	}
	return nil, false
}

func (r *Resolver) PopulateImports(f *ast.File) {
	// FIX: Iterate through Decls because merged synthetic files
	// might not have the f.Imports slice populated.
	for _, decl := range f.Decls {
		if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.IMPORT {
			for _, spec := range genDecl.Specs {
				if imp, ok := spec.(*ast.ImportSpec); ok {
					path := strings.Trim(imp.Path.Value, `"`)
					name := ""
					if imp.Name != nil {
						name = imp.Name.Name // Handle alias like: import g "github.com/..."
					} else {
						// Default: "os" from "os", "fmt" from "fmt"
						parts := strings.Split(path, "/")
						name = parts[len(parts)-1]
					}
					r.Imports[name] = path
				}
			}
		}
	}
}
