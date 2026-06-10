package orm

import "github.com/tinywasm/fmt"

// ErrNotFound is returned when ReadOne() finds no matching row.
var ErrNotFound = fmt.Err("record", "not", "found")

// ErrNoRows is the agnostic sentinel for "query returned no rows".
// Executor adapters (postgres, sqlt) must map their driver-specific no-rows
// error to this value so orm.QB can detect it without importing database/sql.
var ErrNoRows = fmt.Err("no", "rows")

// ErrValidation is returned when validate() finds a mismatch.
var ErrValidation = fmt.Err("error", "validation")

// ErrEmptyTable is returned when ModelName() returns an empty string.
var ErrEmptyTable = fmt.Err("name", "table", "empty")

// ErrNoTxSupport is returned by DB.Tx() when the executor does not implement TxExecutor.
var ErrNoTxSupport = fmt.Err("transaction", "not", "supported")

// ErrSyncFailed is returned by db.Sync() on fatal schema reconciliation errors.
var ErrSyncFailed = fmt.Err("sync", "failed")

// wrapSyncErr returns an error that errors.Is(err, ErrSyncFailed) can detect.
func wrapSyncErr(cause error) error { return fmt.ErrType(cause, ErrSyncFailed) }
