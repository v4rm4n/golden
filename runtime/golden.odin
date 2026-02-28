// --- golden/runtime/golden.odin ---

package golden

import "core:fmt"
import "core:mem"
import "core:sync"
import "core:thread"
import "core:strings"

// ═══════════════════════════════════════════════════════════════════
// ARC — Automatic Reference Counting
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
// ═══════════════════════════════════════════════════════════════════

FRAME_SIZE :: 1024 * 64

Frame :: struct {
    buf:    [FRAME_SIZE]byte,
    offset: int,
}

frame_begin :: proc() -> Frame { return Frame{} }

frame_end :: proc(f: ^Frame) {
    f.offset = 0
}

frame_new :: proc(value: $T, f: ^Frame) -> ^T {
    size    := size_of(T)
    align   := align_of(T)
    aligned := (f.offset + align - 1) & ~(align - 1)
    if aligned + size > FRAME_SIZE {
        d := new(T)
        d^ = value
        return d
    }
    ptr := cast(^T)&f.buf[aligned]
    ptr^ = value
    f.offset = aligned + size
    return ptr
}

frame_init :: proc(ptr: ^$T, value: T) { ptr^ = value }

// ═══════════════════════════════════════════════════════════════════
// TASK POOL — Goroutine Scheduler
// ═══════════════════════════════════════════════════════════════════

MAX_TASKS   :: 1024
MAX_THREADS :: 32

Task :: struct {
    fn:   proc(rawptr),
    data: rawptr,
}

TaskQueue :: struct {
    tasks:     [MAX_TASKS]Task,
    head:      int,
    tail:      int,
    mu:        sync.Mutex,   // zero-init, no init call needed
    not_empty: sync.Cond,    // zero-init, no init call needed
    not_full:  sync.Cond,    // zero-init, no init call needed
    closed:    bool,
}

Pool :: struct {
    queue:   TaskQueue,
    threads: [MAX_THREADS]^thread.Thread,
    count:   int,
    running: bool,
}

_pool: Pool

queue_push :: proc(q: ^TaskQueue, t: Task) {
    sync.mutex_lock(&q.mu)
    for (q.head - q.tail) >= MAX_TASKS && !q.closed {
        sync.cond_wait(&q.not_full, &q.mu)
    }
    if !q.closed {
    // OPTIMIZATION: Bitwise AND for power-of-2 ring buffer
        q.tasks[q.head & (MAX_TASKS - 1)] = t
        q.head += 1
        sync.cond_signal(&q.not_empty)
    }
    sync.mutex_unlock(&q.mu)
}

queue_pop :: proc(q: ^TaskQueue) -> (Task, bool) {
    sync.mutex_lock(&q.mu)
    for q.head == q.tail && !q.closed {
        sync.cond_wait(&q.not_empty, &q.mu)
    }
    if q.head == q.tail {
        sync.mutex_unlock(&q.mu)
        return {}, false
    }
    // OPTIMIZATION: Bitwise AND for power-of-2 ring buffer
    t := q.tasks[q.tail & (MAX_TASKS - 1)]
    q.tail += 1
    sync.cond_signal(&q.not_full)
    sync.mutex_unlock(&q.mu)
    return t, true
}

queue_close :: proc(q: ^TaskQueue) {
    sync.mutex_lock(&q.mu)
    q.closed = true
    sync.cond_broadcast(&q.not_empty)
    sync.cond_broadcast(&q.not_full)
    sync.mutex_unlock(&q.mu)
}

_worker_proc :: proc(t: ^thread.Thread) {
    for {
        task, ok := queue_pop(&_pool.queue)
        if !ok { return }
        task.fn(task.data)
    }
}

pool_start :: proc(n: int) {
    _pool.count   = min(n, MAX_THREADS)
    _pool.running = true
    // TaskQueue is zero-initialized — Mutex and Cond need no explicit init
    for i in 0..<_pool.count {
        t := thread.create(_worker_proc)
        _pool.threads[i] = t
        thread.start(t)
    }
}

pool_stop :: proc() {
    queue_close(&_pool.queue)
    for i in 0..<_pool.count {
        thread.join(_pool.threads[i])
        thread.destroy(_pool.threads[i])
    }
    _pool.running = false
}

spawn :: proc(fn: proc()) {
    fn_copy := new(proc())
    fn_copy^ = fn
    queue_push(&_pool.queue, Task{
        fn = proc(data: rawptr) {
            f := cast(^proc())data
            f^()
            free(f)
        },
        data = fn_copy,
    })
}

// ═══════════════════════════════════════════════════════════════════
// WAITGROUP
// ═══════════════════════════════════════════════════════════════════

WaitGroup :: struct {
    mu:    sync.Mutex,   // zero-init
    cond:  sync.Cond,    // zero-init
    count: int,
}

// wg_init is a no-op — WaitGroup is zero-initialized.
// Kept so transpiler-injected calls compile without error.
wg_init :: proc(wg: ^WaitGroup) {}

wg_add :: proc(wg: ^WaitGroup, delta: int) {
    sync.mutex_lock(&wg.mu)
    wg.count += delta
    if wg.count == 0 {
        sync.cond_broadcast(&wg.cond)
    }
    sync.mutex_unlock(&wg.mu)
}

wg_done :: proc(wg: ^WaitGroup) {
    wg_add(wg, -1)
}

wg_wait :: proc(wg: ^WaitGroup) {
    sync.mutex_lock(&wg.mu)
    for wg.count > 0 {
        sync.cond_wait(&wg.cond, &wg.mu)
    }
    sync.mutex_unlock(&wg.mu)
}

// spawn_raw submits a proc(rawptr) with a data pointer.
// Used by the transpiler for goroutines that capture arguments.
spawn_raw :: proc(fn: proc(rawptr), data: rawptr) {
    queue_push(&_pool.queue, Task{fn = fn, data = data})
}

// ═══════════════════════════════════════════════════════════════════
// CHANNELS (Unbuffered Rendezvous)
// ═══════════════════════════════════════════════════════════════════

Channel :: struct($T: typeid) {
    data:      T,
    has_data:  bool,
    mu:        sync.Mutex,   // zero-init
    not_empty: sync.Cond,    // zero-init
    not_full:  sync.Cond,    // zero-init
}

// chan_make allocates a new generic channel on the heap
chan_make :: proc($T: typeid) -> ^Channel(T) {
    return new(Channel(T))
}

// chan_send blocks until the channel is empty, then writes data
chan_send :: proc(c: ^Channel($T), val: T) {
    sync.mutex_lock(&c.mu)
    for c.has_data {
        sync.cond_wait(&c.not_full, &c.mu)
    }
    c.data = val
    c.has_data = true
    sync.cond_signal(&c.not_empty)
    sync.mutex_unlock(&c.mu)
}

// chan_recv blocks until the channel has data, then reads it
chan_recv :: proc(c: ^Channel($T)) -> T {
    sync.mutex_lock(&c.mu)
    for !c.has_data {
        sync.cond_wait(&c.not_empty, &c.mu)
    }
    val := c.data
    c.has_data = false
    sync.cond_signal(&c.not_full)
    sync.mutex_unlock(&c.mu)
    return val
}

// ═══════════════════════════════════════════════════════════════════
// ERRORS
// ═══════════════════════════════════════════════════════════════════

// error_new converts an Odin string to a C-string so it can be nil-checked
error_new :: proc(msg: string) -> cstring {
    return strings.clone_to_cstring(msg)
}