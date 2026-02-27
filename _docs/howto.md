```powershell
# Tell Go to use a local tmp dir it writes through its own trusted process
$env:GOFLAGS="-trimpath"
$env:GOTMPDIR="$PWD\tmp"
New-Item -ItemType Directory -Force tmp
go run .\cmd\golden\ .\PoCs\003_control_flow.go
```