// --- golden/PoCs/002_multi_type_logic.go ---

package main

// Test: Mapping multiple basic types
type Player struct {
    Health  int
    Active  bool
    Name    string
}

// Test: Function parameters and return types
func Add(a int, b int) int {
    return a + b
}

func main() {
    result := Add(10, 20)
}