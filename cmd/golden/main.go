// --- golden/cmd/golden/main.odin ---

package main

import (
	"fmt"
	"go/parser"
	"go/token"
	"log"
	"os"

	"github.com/v4rm4n/golden/internal/transpiler"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Please provide a .go file")
	}

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, os.Args[1], nil, parser.ParseComments)
	if err != nil {
		log.Fatal(err)
	}

	output := transpiler.Process(node)
	fmt.Println(output)
}
