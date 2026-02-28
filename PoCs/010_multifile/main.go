// --- golden/PoCs/010_multifile/main.go ---

package main

import "fmt"

func main() {
	fmt.Println("Starting Multi-File Project...")

	// Create a user (defined in user.go)
	user := NewUser("Alice", 25)

	// Do some math (defined in math.go)
	targetAge := Add(user.Age, 5)

	fmt.Println("User:", user.Name)
	fmt.Println("Age in 5 years:", targetAge)
}
