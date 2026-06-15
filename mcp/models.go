//go:build !wasm

package mcporm

// ormc:formonly
type QueryArgs struct {
    SQL  string `db:"not_null" input:"-"`
    Args string `input:"-"` // JSON array, e.g. ["val1", 2]
}

// ormc:formonly
type ExecArgs struct {
    SQL  string `db:"not_null" input:"-"`
    Args string `input:"-"` // JSON array, e.g. ["val1", 2]
}
