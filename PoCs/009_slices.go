// --- golden/PoCs/009_slices.go ---

package main

import "fmt"

func main() {
	// 1. Create a slice (capacity 0)
	nums := make([]int, 0)

	// 2. Append values (Go style requires reassignment)
	nums = append(nums, 10)
	nums = append(nums, 20)
	nums = append(nums, 30)

	// 3. Read length, capacity, and index
	fmt.Println("Len:", len(nums))
	fmt.Println("Cap:", cap(nums))
	fmt.Println("First:", nums[0])
	fmt.Println("Last:", nums[2])
}
