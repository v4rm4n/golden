// --- golden/PoCs/001_variable_assignment.go ---

package main

import "fmt"

// Test: Struct mapping with basic types
type User struct {
	ID  int
	Age int
}

// Test: Function mapping and local variable assignment
func main() {
	score := 100
	health := 50

	fmt.Println("Score:", score)
	fmt.Println("Health:", health)
}
