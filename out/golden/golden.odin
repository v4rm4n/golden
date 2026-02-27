package golden

import "core:fmt"
import "core:mem"

// ═══════════════════════════════════════════════════════════════════
// ARC — Automatic Reference Counting
// Used for allocations that ESCAPE their declaring function scope.
// ═══════════════════════════════════════════════════════════════════

Arc :: struct($T: typeid) {
    data:  ^T,
    count: ^int,
}

make_arc :: proc(value: $T) -> Arc(T) {
    d := new(T)
    d^ = value
    c := new(int)
    c^ = 1
    return Arc(T){data = d, count = c}
}

retain :: proc(a: Arc($T)) -> Arc(T) {
    a.count^ += 1
    return a
}

release :: proc(a: Arc($T)) {
    a.count^ -= 1
    if a.count^ == 0 {
        free(a.data)
        free(a.count)
        fmt.println("[golden] arc freed:", typeid_of(T))
    }
}

// ═══════════════════════════════════════════════════════════════════
// ARENA — Frame Allocator
// Used for LOCAL allocations that never escape their function scope.
// One pointer bump to alloc. One pointer reset to free ALL. O(1).
// ═══════════════════════════════════════════════════════════════════

FRAME_SIZE :: 1024 * 64  // 64KB per frame

Frame :: struct {
    buf:    [FRAME_SIZE]byte,
    offset: int,
}

frame_begin :: proc() -> Frame {
    return Frame{}
}

frame_end :: proc(f: ^Frame) {
    f.offset = 0
    fmt.println("[golden] frame freed")
}

// Allocate T inside the frame — bump pointer, no heap.
frame_new :: proc(value: $T, f: ^Frame) -> ^T {
    size  := size_of(T)
    align := align_of(T)
    aligned := (f.offset + align - 1) & ~(align - 1)

    if aligned + size > FRAME_SIZE {
        fmt.println("[golden] frame overflow → heap fallback for", typeid_of(T))
        d := new(T)
        d^ = value
        return d
    }

    ptr := cast(^T)&f.buf[aligned]
    ptr^ = value
    f.offset = aligned + size
    return ptr
}

// Set fields on an arena pointer after allocation.
frame_init :: proc(ptr: ^$T, value: T) {
    ptr^ = value
}
