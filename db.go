package orm

import (
	"github.com/tinywasm/model"
	"github.com/tinywasm/storage"
)

// DB represents an ergonomic handle over a storage backend (a storage.Conn). Consumers instantiate
// it via New(). This type owns no contract — storage.Conn is the contract; DB is the fluent layer
// on top of it (see docs/ARQUITECTURE.md).
type DB struct {
	conn storage.Conn
	log  func(messages ...any)
}

// New wraps a storage.Conn (a backend's Executor+Compiler pair, e.g. sqlt.Open(dsn) or mem.New())
// in the ergonomic DB handle. One argument, not two: storage.Conn already unifies Executor+Compiler
// so an Executor from one backend can never be paired with a Compiler from another.
func New(conn storage.Conn) *DB {
	return &DB{conn: conn}
}

// SetLog sets the log function for warnings and informational messages.
// If not set, messages are silently discarded.
func (d *DB) SetLog(fn func(messages ...any)) {
	d.log = fn
}

func (d *DB) logw(messages ...any) {
	if d.log != nil {
		d.log(messages...)
	}
}

// Create inserts a new model into the database.
func (d *DB) Create(m model.Model) error {
	if err := validateQuery(storage.ActionCreate, m); err != nil {
		return err
	}
	schema := m.Schema()
	ptrs := m.Pointers()
	allValues := model.ReadValues(schema, ptrs)
	var columns []string
	var values []any
	for i, f := range schema {
		// Skip autoincrement PK fields with zero value — let the DB assign them.
		if f.IsPK() && f.IsAutoInc() {
			if v, ok := allValues[i].(int); ok && v == 0 {
				continue
			}
		}
		columns = append(columns, f.Name)
		values = append(values, allValues[i])
	}
	q := storage.Query{
		Action:  storage.ActionCreate,
		Table:   m.ModelName(),
		Columns: columns,
		Values:  values,
	}
	plan, err := d.conn.Compile(q, m)
	if err != nil {
		return err
	}
	return d.conn.Exec(plan.Query, plan.Args...)
}

// Update modifies an existing row. At least one Condition is required.
// Providing zero conditions is a compile-time error — there is no variadic
// fallback — preventing accidental full-table UPDATE statements.
func (d *DB) Update(m model.Model, cond storage.Condition, rest ...storage.Condition) error {
	if err := validateQuery(storage.ActionUpdate, m); err != nil {
		return err
	}
	conds := append([]storage.Condition{cond}, rest...)
	schema := m.Schema()
	columns := make([]string, len(schema))
	for i, f := range schema {
		columns[i] = f.Name
	}
	q := storage.Query{
		Action:     storage.ActionUpdate,
		Table:      m.ModelName(),
		Columns:    columns,
		Values:     model.ReadValues(schema, m.Pointers()),
		Conditions: conds,
	}
	plan, err := d.conn.Compile(q, m)
	if err != nil {
		return err
	}
	return d.conn.Exec(plan.Query, plan.Args...)
}

// Delete deletes a model from the database.
// At least one Condition is required. Providing zero conditions is a compile-time
// error, preventing accidental full-table DELETE statements.
func (d *DB) Delete(m model.Model, cond storage.Condition, rest ...storage.Condition) error {
	if err := validateQuery(storage.ActionDelete, m); err != nil {
		return err
	}
	conds := append([]storage.Condition{cond}, rest...)
	q := storage.Query{
		Action:     storage.ActionDelete,
		Table:      m.ModelName(),
		Conditions: conds,
	}
	plan, err := d.conn.Compile(q, m)
	if err != nil {
		return err
	}
	return d.conn.Exec(plan.Query, plan.Args...)
}

// Query creates a new QB instance.
func (d *DB) Query(m model.Model) *QB {
	return &QB{db: d, model: m}
}

// Close closes the underlying connection.
func (d *DB) Close() error {
	return d.conn.Close()
}

// RawConn returns the underlying storage.Conn. Renamed from RawExecutor: what's underneath is a
// full Conn (Executor+Compiler), not just an Executor.
func (d *DB) RawConn() storage.Conn {
	return d.conn
}
