// --- golden/PoCs/002_multi_type_logic.go ---

package main

import "fmt"

// Test: Mapping multiple basic types
type Player struct {
	Health int
	Active bool
	Name   string
}

// Test: Function parameters and return types
func Add(a int, b int) int {
	return a + b
}

func main() {
	// Test the logic
	result := Add(10, 20)
	fmt.Println("10 + 20 =", result)

	// Test the struct and dynamic types
	p := Player{Health: 100, Active: true, Name: "Hero"}
	fmt.Println("Player:", p.Name)
	fmt.Println("Active:", p.Active)
	fmt.Println("Health:", p.Health)
}
