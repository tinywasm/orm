package orm

// TxBound represents a transaction bound to an adapter.
type TxBound interface {
	Adapter
	Commit() error
	Rollback() error
}

// TxAdapter represents an adapter that supports transactions.
type TxAdapter interface {
	Adapter
	BeginTx() (TxBound, error)
}

// Tx executes a function within a transaction.
func (db *DB) Tx(fn func(tx *DB) error) error {
	txAdapter, ok := db.adapter.(TxAdapter)
	if !ok {
		return ErrNoTxSupport
	}

	bound, err := txAdapter.BeginTx()
	if err != nil {
		return err
	}

	txDB := &DB{adapter: bound}

	if err := fn(txDB); err != nil {
		bound.Rollback()
		return err
	}

	return bound.Commit()
}
