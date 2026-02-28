// --- golden/PoCs/011_methods.go ---

package main

import "fmt"

type Counter struct {
	Value int
}

// 1. Pointer receiver (mutates data)
func (c *Counter) Increment() {
	c.Value++
}

// 2. Value receiver (reads data)
func (c Counter) Print() {
	fmt.Println("Count is:", c.Value)
}

func main() {
	c := Counter{Value: 0}

	c.Increment()
	c.Increment()
	c.Print()
}
