package orm

import "errors"

// ErrNotFound is returned when ReadOne() finds no matching row.
var ErrNotFound = errors.New("record not found")

// ErrValidation is returned when validate() finds a mismatch.
var ErrValidation = errors.New("validation error")

// ErrEmptyTable is returned when TableName() returns an empty string.
var ErrEmptyTable = errors.New("empty table name")

// ErrNoTxSupport is returned by DB.Tx() when the adapter does not implement TxAdapter.
var ErrNoTxSupport = errors.New("transaction not supported")
