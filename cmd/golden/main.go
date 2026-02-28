// --- golden/cmd/golden/main.go ---

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/v4rm4n/golden/internal/transpiler"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Usage: golden <input.go | input_dir> [output-dir]")
	}

	inputPath := os.Args[1]
	outDir := "out"
	if len(os.Args) >= 3 {
		outDir = os.Args[2]
	}

	// 1. Determine if input is a file or a directory
	info, err := os.Stat(inputPath)
	if err != nil {
		log.Fatalf("Could not read input path: %v", err)
	}

	fset := token.NewFileSet()
	var mergedDecls []ast.Decl

	if info.IsDir() {
		// 2A. Directory Mode: Parse all files in the package
		pkgs, err := parser.ParseDir(fset, inputPath, nil, parser.ParseComments)
		if err != nil {
			log.Fatalf("Failed to parse directory: %v", err)
		}

		// Look for the "main" package
		mainPkg, ok := pkgs["main"]
		if !ok {
			log.Fatalf("No 'main' package found in directory %s", inputPath)
		}

		// Merge all declarations from all files in the package
		for _, fileNode := range mainPkg.Files {
			mergedDecls = append(mergedDecls, fileNode.Decls...)
		}
		fmt.Printf("Parsed %d files from directory: %s\n", len(mainPkg.Files), inputPath)

	} else {
		// 2B. File Mode: Parse just the single file
		node, err := parser.ParseFile(fset, inputPath, nil, parser.ParseComments)
		if err != nil {
			log.Fatalf("Failed to parse file: %v", err)
		}
		mergedDecls = node.Decls
		fmt.Printf("Parsed single file: %s\n", inputPath)
	}

	// 3. Create a synthetic AST File containing all merged declarations
	mergedFile := &ast.File{
		Name:  &ast.Ident{Name: "main"},
		Decls: mergedDecls,
	}

	// 4. Setup output directories
	goldenDir := filepath.Join(outDir, "golden")
	if err := os.MkdirAll(goldenDir, 0755); err != nil {
		log.Fatal("Could not create output dir:", err)
	}

	// 5. Transpile
	odinOutput := transpiler.Process(mergedFile)

	// Optional Polish: Clean up duplicate imports if files had them
	odinOutput = cleanDuplicateImports(odinOutput)

	outFile := filepath.Join(outDir, "main.odin")
	if err := os.WriteFile(outFile, []byte(odinOutput), 0644); err != nil {
		log.Fatal("Could not write output:", err)
	}

	// 6. Copy Runtime
	runtimeSrc := filepath.Join("runtime", "golden.odin")
	runtimeDst := filepath.Join(goldenDir, "golden.odin")
	if err := copyFile(runtimeSrc, runtimeDst); err != nil {
		log.Printf("Warning: could not copy runtime: %v", err)
	}

	// 7. Done
	fmt.Printf("✓ Transpiled → %s\n", outFile)
	fmt.Printf("✓ Runtime    → %s\n", runtimeDst)
	fmt.Printf("\nTo compile:\n  cd %s && odin run .\n", outDir)
}

// cleanDuplicateImports removes duplicate import statements that might occur
// when merging multiple Go files that all import "fmt"
func cleanDuplicateImports(code string) string {
	lines := strings.Split(code, "\n")
	seen := make(map[string]bool)
	var out []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "import ") {
			if seen[trimmed] {
				continue
			}
			seen[trimmed] = true
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
