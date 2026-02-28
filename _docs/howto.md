```powershell
# Tell Go to use a local tmp dir it writes through its own trusted process
$env:GOFLAGS="-trimpath"
$env:GOTMPDIR="$PWD\tmp"
New-Item -ItemType Directory -Force tmp
go run .\cmd\golden\ .\PoCs\003_control_flow.go
```

Slices & Maps ([]T and map[K]V): Right now, we map them to Odin's [dynamic]T, but we aren't auto-freeing them yet. We need to tie Odin's delete() into our Arena/ARC system so dynamically sized arrays don't leak memory.

Memory Polish for Errors: strings.clone_to_cstring allocates memory. Right now, that memory leaks because we never call free(err). We need to teach the transpiler to auto-inject a defer free() or allocate errors on the Arena.

Interfaces (any and vtables): Translating Go interfaces to Odin requires some hardcore struct manipulation.

What is the vibe? Do you want to dive into Step 009: Slices and Arrays, do you want to fix the cstring memory leak, or do you want to update your README.md and take a victory lap for the day?