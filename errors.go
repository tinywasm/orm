package orm

import "webtyp.com/fmt"

// ErrNotFound is returned when ReadOne() finds no matching row. Translates storage.ErrNoRows —
// storage itself has no concept of "not found", only "no rows" (see qb.go's ReadOne).
var ErrNotFound = fmt.Err("record", "not", "found")

// ErrValidation is returned when validate() finds a mismatch.
var ErrValidation = fmt.Err("error", "validation")

// ErrEmptyTable is returned when ModelName() returns an empty string.
var ErrEmptyTable = fmt.Err("name", "table", "empty")

// ErrNoTxSupport is returned by DB.Tx() when the underlying storage.Conn does not implement
// storage.TxExecutor.
var ErrNoTxSupport = fmt.Err("transaction", "not", "supported")
