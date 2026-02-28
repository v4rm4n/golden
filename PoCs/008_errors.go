// --- golden/PoCs/008_errors.go ---

package main

import (
	"errors"
	"fmt"
)

func Divide(a int, b int) (int, error) {
	if b == 0 {
		return 0, errors.New("cannot divide by zero")
	}
	// Return nil to prove our cstring mapping works!
	return a / b, nil
}

func main() {
	// 1. Test a failing case
	val, err := Divide(10, 0)
	if err != nil {
		fmt.Println("Math Error:", err)
	} else {
		fmt.Println("Success:", val)
	}

	// 2. Test a passing case
	val2, err2 := Divide(10, 2)
	if err2 != nil {
		fmt.Println("Math Error:", err2)
	} else {
		fmt.Println("Success:", val2)
	}
}
