// --- golden/PoCs/012_boss_fight/worker.go ---

package main

import (
	"fmt"
	"sync"
)

type Pool struct {
	WorkerID int
}

// Value method: Reads from Pool, Mutates User, Syncs with WG and Channels
func (p Pool) ProcessUser(u *User, wg *sync.WaitGroup, ch chan int) {
	fmt.Println("Worker started for User:", u.ID)

	u.Activate() // Call the pointer method from models.go

	ch <- u.Score // Send the result across the thread boundary
	wg.Done()
}
