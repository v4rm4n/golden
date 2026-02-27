// --- golden/PoCs/005_escape_analysis.go ---

package main

import "fmt"

type User struct {
	Name   string
	Health int
}

type Request struct {
	Body []byte
	Size int
}

// u escapes — it is returned, so it must be ARC
func newUser(name string, health int) *User {
	u := &User{Name: name, Health: health}
	return u
}

// req does NOT escape — local only, never returned or sent anywhere
// Golden should use arena for this
func processRequest() {
	req := &Request{Size: 1024}
	fmt.Println(req.Size)
}

// mixed: tmp is local (arena), result escapes (ARC)
func transform(name string) *User {
	tmp := &Request{Size: 512}
	fmt.Println(tmp.Size)
	result := &User{Name: name, Health: 100}
	return result
}

func main() {
	u := newUser("Hero", 100)
	fmt.Println(u.Name)
	processRequest()
	r := transform("Mage")
	fmt.Println(r.Name)
}

// Hero                      ✓ ARC var accessed via .data
// 1024                      ✓ arena req.Size direct access
// [golden] frame freed      ✓ processRequest frame wiped
// 512                       ✓ arena tmp.Size direct access
// [golden] frame freed      ✓ transform frame wiped
// Mage                      ✓ ARC var accessed via .data
