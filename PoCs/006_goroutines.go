// --- golden/PoCs/006_goroutines.go ---

package main

import (
	"fmt"
	"sync"
)

func worker(id int, wg *sync.WaitGroup) {
	defer wg.Done()
	fmt.Println("worker", id)
}

func main() {
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		i := i // shadow i to capture current value
		go func() {
			worker(i, &wg)
		}()
	}
	wg.Wait()
}
