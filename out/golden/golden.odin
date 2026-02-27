package golden

import "core:mem"
import "core:fmt"

// The wrapper for Go pointers
Arc :: struct($T: typeid) {
    data:  ^T,
    count: ^int,
}

// Replaces Go's "&Struct{...}"
// Usage: make_arc(User{Name = "Hero", Health = 100})
make_arc :: proc(value: $T) -> Arc(T) {
    d := new(T)
    d^ = value
    c := new(int)
    c^ = 1
    return Arc(T){data = d, count = c}
}

// Retain increments the ref count (used when passing Arc to another scope)
retain :: proc(a: Arc($T)) -> Arc(T) {
    a.count^ += 1
    return a
}

// Injected by the transpiler via defer at end of scope
release :: proc(a: Arc($T)) {
    a.count^ -= 1
    if a.count^ == 0 {
        free(a.data)
        free(a.count)
        fmt.println("[golden] freed:", typeid_of(T))
    }
}
