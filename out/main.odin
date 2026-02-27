package main

import "core:fmt"
import golden "golden"

User :: struct {
	Name: string,
	Health: int,
}

printUser :: proc(u: golden.Arc(User)) {
	fmt.println(u.data.Name)
	fmt.println(u.data.Health)
}

main :: proc() {
	u := golden.make_arc(User{Name = "Hero", Health = 100})
	defer golden.release(u)
	fmt.println(u.data.Name)
	fmt.println(u.data.Health)
	printUser(u)
}
