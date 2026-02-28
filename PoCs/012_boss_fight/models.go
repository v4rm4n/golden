// --- golden/PoCs/012_boss_fight/models.go ---

package main

type User struct {
	ID     int
	Active bool
	Score  int
}

// Pointer method: Mutates the struct
func (u *User) Activate() {
	u.Active = true
	u.Score = u.Score + 100
}
