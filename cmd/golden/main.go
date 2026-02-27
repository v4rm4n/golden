// --- golden/cmd/golden/main.odin ---

package main

import (
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/v4rm4n/golden/internal/transpiler"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Usage: golden <input.go> [output-dir]")
	}

	// ── Parse input ─────────────────────────────────────────────────────────
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, os.Args[1], nil, parser.ParseComments)
	if err != nil {
		log.Fatal(err)
	}

	// ── Determine output directory ───────────────────────────────────────────
	outDir := "out"
	if len(os.Args) >= 3 {
		outDir = os.Args[2]
	}

	// ── Create output dir + golden/ subdir ───────────────────────────────────
	goldenDir := filepath.Join(outDir, "golden")
	if err := os.MkdirAll(goldenDir, 0755); err != nil {
		log.Fatal("Could not create output dir:", err)
	}

	// ── Write transpiled Odin file ───────────────────────────────────────────
	odinOutput := transpiler.Process(node)
	outFile := filepath.Join(outDir, "main.odin")
	if err := os.WriteFile(outFile, []byte(odinOutput), 0644); err != nil {
		log.Fatal("Could not write output:", err)
	}

	// ── Copy runtime/golden.odin → out/golden/golden.odin ───────────────────
	runtimeSrc := filepath.Join("runtime", "golden.odin")
	runtimeDst := filepath.Join(goldenDir, "golden.odin")
	if err := copyFile(runtimeSrc, runtimeDst); err != nil {
		log.Printf("Warning: could not copy runtime: %v", err)
		log.Printf("Manually copy runtime/golden.odin to %s", goldenDir)
	}

	// ── Done ─────────────────────────────────────────────────────────────────
	fmt.Printf("✓ Transpiled → %s\n", outFile)
	fmt.Printf("✓ Runtime    → %s\n", runtimeDst)
	fmt.Printf("\nTo compile:\n  cd %s && odin run .\n", outDir)
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