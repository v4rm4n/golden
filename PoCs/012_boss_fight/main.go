// --- golden/PoCs/012_boss_fight/worker.go ---

package main

import (
	"fmt"
	"sync"
)

func main() {
	fmt.Println("=== Golden V1.0 Boss Fight ===")

	// 1. Slices (Auto-freed by Golden Arena!)
	users := make([]User, 0)
	users = append(users, User{ID: 1, Active: false, Score: 0})
	users = append(users, User{ID: 2, Active: false, Score: 50})

	// 2. Concurrency Primitives
	var wg sync.WaitGroup
	ch := make(chan int)

	// 3. Value Struct
	pool := Pool{WorkerID: 99}

	// 4. Goroutines with Dynamic Closure Captures
	wg.Add(1)
	go func() {
		// Passing slice element by reference
		pool.ProcessUser(&users[0], &wg, ch)
	}()

	wg.Add(1)
	go func() {
		pool.ProcessUser(&users[1], &wg, ch)
	}()

	// 5. Channel Rendezvous (Main thread sleeps until workers send data)
	score1 := <-ch
	score2 := <-ch

	// 6. Wait for exact cleanup
	wg.Wait()

	fmt.Println("All workers done.")
	fmt.Println("Combined Score:", score1+score2)
}
