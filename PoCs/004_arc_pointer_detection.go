// --- golden/PoCs/004_arc_pointer_detection.go ---

package main

import "fmt"

type User struct {
	Name   string
	Health int
}

func printUser(u *User) {
	fmt.Println(u.Name)
	fmt.Println(u.Health)
}

func main() {
	u := &User{Name: "Hero", Health: 100}
	fmt.Println(u.Name)
	fmt.Println(u.Health)
	printUser(u)
}

// Hero, 100, Hero, 100      ✓ correct values
// [golden] frame freed ×2   ✓ arena wiped, zero heap involvement
