#  Golden

<p align="center">
  <img src="./_docs/golden_logoonly_nobg.png" alt="Golden" width="200"/>
</p>

<p style="font-size: 20px;">
  <strong>Authentic Go syntax, zero garbage collection.</strong>
  A high-performance transpiler targeting Odin with ARC for deterministic, systems-level power.
</p>

## 🎯 Vision

The systems programming world is currently split. On one side, you have Rust: undeniably powerful, but heavily bogged down by steep learning curves, slow compile times, and constant cognitive friction with the borrow checker. On the other side, you have Go: an absolute joy to write with a massive ecosystem, but disqualified from true real-time, high-performance, or game-engine domains due to the unpredictable latency of its Garbage Collector. 

**Golden is my ambition to merge these two worlds.** It bridges the gap between a ubiquitous, high-level language and a niche, brutally fast systems compiler. By taking Go's clean syntax and mapping it directly to Odin's low-level metal—swapping out the GC for deterministic ARC and Arena allocators—Golden gives you the developer experience and ecosystem of Go, backed by the uncompromising, hard-mode power of Odin.

## 📁 Project Structure

```text
golden/
├── cmd/golden/         # The CLI entry point (The "Brain")
├── internal/transpiler/# AST traversal and Odin code generation logic
├── runtime/            # ARC, Arena, and Task Pool library (golden.odin)
├── PoCs/               # Proof of Concepts & Regression Test Suite
└── go.mod              # Go module definition
```

## 🚀 Getting Started
### Prerequisites
- [Go](https://go.dev/doc/install) (to run the transpiler)
- [Odin](https://odin-lang.org/docs/install/) (to compile the generated output)

### Running the Transpiler
To transpile a Go file to Odin, run:

```bash
go run ./cmd/golden ./PoCs/006_goroutines.go ./out
cd out && odin run .
```

## 🛠 Current Status

### Phase 1: Translator

[x] Authentic Go AST parsing

[x] Struct and dynamic Type mapping (int, string, bool -> b8, etc.)

[x] Control Flow (if/else, for loops, range)

### Phase 2: The Alchemist (Memory)

[x] Automatic Reference Counting (ARC) for escaping pointers

[x] Arena Frame Allocators for local-scoped structs

[x] Escape Analysis (Dynamically routes to ARC or Arena)

[x] Auto-injected defer statements for deterministic GC-free cleanup

### Phase 3: Engine (Concurrency)

[x] Custom Odin Work-Stealing Scheduler (Task Pool)

[x] Goroutines (go func()) mapped to thread-pool tasks

[x] Dynamic Closure Capture (AST Walker auto-packs local variables into structs)

[x] WaitGroups (sync.WaitGroup -> golden.WaitGroup)

### Phase 4: Up Next

[ ] Channels (chan) and select

[ ] Error Handling paradigms