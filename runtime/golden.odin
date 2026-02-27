// --- golden/runtime/golden.odin ---

package golden

import "core:mem"
import "core:fmt"

// The wrapper for Go pointers
Arc :: struct($T: typeid) {
    data:  ^T,
    count: ^int,
}

// Replaces Go's "new" or "&Struct{}"
make_arc :: proc($T: typeid) -> Arc(T) {
    d := new(T)
    c := new(int)
    c^ = 1
    return Arc(T){data = d, count = c}
}

// Injected by the transpiler at end of scope
release :: proc(a: Arc($T)) {
    a.count^ -= 1
    if a.count^ == 0 {
        free(a.data)
        free(a.count)
        fmt.println("Golden: Memory released")
    }
}