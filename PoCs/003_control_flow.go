// --- golden/PoCs/003_control_flow.go ---

package main

import "fmt"

type Player struct {
	Name   string
	Health int
	Score  float64
	Alive  bool
}

func add(a int, b int) int {
	return a + b
}

func isAlive(health int) bool {
	return health > 0
}

func describe(p Player) {
	if p.Alive {
		fmt.Println("Player is alive")
	} else {
		fmt.Println("Player is dead")
	}
}

func countdown(from int) {
	i := from
	for i > 0 {
		fmt.Println(i)
		i = i - 1
	}
}

func sumSlice(nums []int) int {
	total := 0
	for _, n := range nums {
		total = total + n
	}
	return total
}

func clamp(val int, min int, max int) int {
	if val < min {
		return min
	} else if val > max {
		return max
	} else {
		return val
	}
}

func main() {
	p := Player{Name: "Hero", Health: 100, Score: 9.5, Alive: true}
	describe(p)

	result := add(10, 32)
	fmt.Println(result)

	alive := isAlive(p.Health)
	fmt.Println(alive)

	countdown(5)

	clamped := clamp(150, 0, 100)
	fmt.Println(clamped)
}