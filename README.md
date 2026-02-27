#  Golden

![Golden](/_docs/golden_logoonly_nobg.png)

**Golden: Authentic Go syntax, zero garbage collection.** A high-performance transpiler targeting Odin with ARC for deterministic, systems-level power.

## 📁 Project Structure

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

### Phase 1: The Translator

[x] Authentic Go AST parsing

[x] Struct and dynamic Type mapping (int, string, bool -> b8, etc.)

[x] Control Flow (if/else, for loops, range)

### Phase 2: The Alchemist (Memory)

[x] Automatic Reference Counting (ARC) for escaping pointers

[x] Arena Frame Allocators for local-scoped structs

[x] Escape Analysis (Dynamically routes to ARC or Arena)

[x] Auto-injected defer statements for deterministic GC-free cleanup

### Phase 3: The Engine (Concurrency)

[x] Custom Odin Work-Stealing Scheduler (Task Pool)

[x] Goroutines (go func()) mapped to thread-pool tasks

[x] Dynamic Closure Capture (AST Walker auto-packs local variables into structs)

[x] WaitGroups (sync.WaitGroup -> golden.WaitGroup)

### Phase 4: Up Next

[ ] Channels (chan) and select

[ ] Error Handling paradigms