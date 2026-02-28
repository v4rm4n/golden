// --- golden/PoCs/showcase_01.go ---

package main

import "fmt"

type Worker struct{ ID int }

// Method attached to struct
func (w Worker) Process(ch chan int) {
	ch <- w.ID * 10
}

func main() {
	ch := make(chan int)
	worker := Worker{ID: 42}

	// Goroutine capturing local variables
	go func() {
		worker.Process(ch)
	}()

	fmt.Println("Result:", <-ch)
}
