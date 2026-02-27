package main

import "core:fmt"
import golden "golden"

User :: struct {
	Name: string,
	Health: int,
}

Request :: struct {
	Body: [dynamic]byte,
	Size: int,
}

newUser :: proc(name: string, health: int) -> golden.Arc(User) {
	u := golden.make_arc(User{Name = name, Health = health})
	return u
}

processRequest :: proc() {
	_frame := golden.frame_begin()
	defer golden.frame_end(&_frame)
	req := golden.frame_new(Request{}, &_frame)
	golden.frame_init(req, Request{Size = 1024})
	fmt.println(req.Size)
}

transform :: proc(name: string) -> golden.Arc(User) {
	_frame := golden.frame_begin()
	defer golden.frame_end(&_frame)
	tmp := golden.frame_new(Request{}, &_frame)
	golden.frame_init(tmp, Request{Size = 512})
	fmt.println(tmp.Size)
	result := golden.make_arc(User{Name = name, Health = 100})
	return result
}

main :: proc() {
	u := newUser("Hero", 100)
	fmt.println(u.data.Name)
	processRequest()
	r := transform("Mage")
	fmt.println(r.data.Name)
}
