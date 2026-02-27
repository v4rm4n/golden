// --- golden/PoCs/007_channels.go ---

package main

import "fmt"

func main() {
	ch := make(chan int)

	go func() {
		fmt.Println("Worker: Sending data...")
		ch <- 42
	}()

	fmt.Println("Main: Waiting for data...")
	val := <-ch
	fmt.Println("Main: Received:", val)
}
