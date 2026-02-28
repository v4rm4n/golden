// --- golden/PoCs/010_multifile/user.go ---

package main

type User struct {
	Name string
	Age  int
}

func NewUser(name string, age int) User {
	return User{Name: name, Age: age}
}
