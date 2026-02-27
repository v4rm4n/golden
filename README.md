#  Golden

![Golden](/_docs/golden_logoonly_nobg.png)

**Golden: Authentic Go syntax, zero garbage collection.** A high-performance transpiler targeting Odin with ARC for deterministic, systems-level power.

## 📁 Project Structure

```text
golden/
├── cmd/
│   └── golden/         # The CLI entry point (The "Brain")
├── internal/
│   └── transpiler/     # AST traversal and Odin code generation logic
├── runtime/
│   └── golden.odin     # The ARC-based memory management library
├── input.go            # Sample Go source for testing
└── go.mod              # Go module definition
```

## 🚀 Getting Started
### Prerequisites
- Go (to run the transpiler)
- Odin (to compile the generated output)

### Running the Transpiler
To transpile a Go file to Odin, run:

```bash
go run ./cmd/golden input.go
```

## 🛠 Current Status
[x] Authentic Go AST parsing

[x] Struct mapping (Go struct -> Odin struct)

[x] Variable assignment mapping (:=)

[x] Clean, formatted Odin output

[ ] ARC-managed pointers (&)

[ ] Concurrency (Goroutines to Task Pool)
