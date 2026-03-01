// --- golden/runtime/std_os.odin ---

package golden_os

import "core:os"
import "core:fmt"

File :: struct {
    handle: os.Handle,
}

// Map Go's os.Open to Odin
open :: proc(path: string) -> (^File, cstring) {
    h, err := os.open(path, os.O_RDONLY)
    if err != .None {
        return nil, "could not open file"
    }
    f := new(File)
    f.handle = h
    return f, nil
}

// Map Go's os.ReadFile
read_file :: proc(path: string) -> ([]byte, cstring) {
    data, ok := os.read_entire_file_from_path(path)
    if !ok {
        return nil, "read file failed"
    }
    return data, nil
}