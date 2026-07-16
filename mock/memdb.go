package mock

import (
	"github.com/tinywasm/fmt"
	"github.com/tinywasm/model"
	"github.com/tinywasm/orm"
)

// NewDB returns a functional in-memory *orm.DB. It interprets the structured orm.Query
// (Create/ReadOne/ReadAll/Update/Delete + Conditions/OrderBy/Limit/Offset). It is THE double a
// leaf module uses to test round-trips without importing a real driver, and it proves
// orm/conformance exactly like the real backends do.
func NewDB() *orm.DB {
	e := &engine{}
	return orm.New(e, e)
}

// dbCell is one column/value pair. Rows and tables are plain slices scanned linearly — no Go
// map anywhere: TinyGo's map runtime is heavy and bloats the wasm binary, so it's prohibited
// across tinywasm (see AGENTS.md). Table/row counts here are tiny (test fixtures), so a linear
// scan costs nothing in practice.
type dbCell struct {
	col string
	val any
}

// dbRow is an ordered set of cells. A dbRow value shares its backing array with whatever it
// was copied from, so rows handed back by match() alias the stored data: set() mutates them
// in place, giving Update/Delete the same "reference into storage" behavior a
// map[string]any used to give for free.
type dbRow []dbCell

func (r dbRow) get(col string) (any, bool) {
	for _, c := range r {
		if c.col == col {
			return c.val, true
		}
	}
	return nil, false
}

// set overwrites an existing column in place. Callers only ever set columns that Create
// already wrote, so every call takes the "found" branch — there is no append fallback.
func (r dbRow) set(col string, val any) {
	for i := range r {
		if r[i].col == col {
			r[i].val = val
			return
		}
	}
}

// dbTable is one named table's rows.
type dbTable struct {
	name string
	rows []dbRow
}

type engine struct {
	tables []dbTable
	lastQ  orm.Query
	lastM  model.Model
}

func (e *engine) tableIndex(name string) int {
	for i := range e.tables {
		if e.tables[i].name == name {
			return i
		}
	}
	return -1
}

func (e *engine) Compile(q orm.Query, m model.Model) (orm.Plan, error) {
	e.lastQ, e.lastM = q, m
	return orm.Plan{Mode: q.Action, Query: "mock", Args: q.Values}, nil
}

func (e *engine) Close() error { return nil }

func (e *engine) BeginTx() (orm.TxBoundExecutor, error) {
	return e, nil
}

func (e *engine) Commit() error   { return nil }
func (e *engine) Rollback() error { return nil }

func (e *engine) Exec(query string, args ...any) error {
	q := e.lastQ
	switch q.Action {
	case orm.ActionCreate:
		newRow := make(dbRow, 0, len(q.Columns))
		for i, col := range q.Columns {
			if i < len(q.Values) {
				newRow = append(newRow, dbCell{col: col, val: q.Values[i]})
			}
		}
		idx := e.tableIndex(q.Table)
		if idx == -1 {
			// Auto-vivifies the table on first insert. This is why the mock Factory in
			// orm/conformance needs no DDL — it just returns NewDB().
			e.tables = append(e.tables, dbTable{name: q.Table})
			idx = len(e.tables) - 1
		}
		e.tables[idx].rows = append(e.tables[idx].rows, newRow)
	case orm.ActionUpdate:
		// db.Update builds q.Columns from m.Schema() in order (see orm/db.go), so
		// q.Columns[i] and schema[i] always name the same field — no lookup needed.
		schema := e.lastM.Schema()
		for _, row := range e.match(q.Table, q.Conditions) { // match returns rows aliasing storage
			for i, col := range q.Columns {
				if i < len(schema) && schema[i].IsPK() {
					continue // do not overwrite PK on update
				}
				if i < len(q.Values) {
					row.set(col, q.Values[i])
				}
			}
		}
	case orm.ActionDelete:
		idx := e.tableIndex(q.Table)
		if idx == -1 {
			return nil
		}
		kept := e.tables[idx].rows[:0:0]
		for _, row := range e.tables[idx].rows {
			if !matchRow(row, q.Conditions) {
				kept = append(kept, row)
			}
		}
		e.tables[idx].rows = kept
	}
	return nil
}

func (e *engine) QueryRow(query string, args ...any) orm.Scanner {
	q := e.lastQ
	rows := applyOffsetLimit(applyOrder(e.match(q.Table, q.Conditions), q.OrderBy), q.Offset, 1)
	if len(rows) == 0 {
		return &memScanner{err: orm.ErrNoRows}
	}
	return &memScanner{row: rows[0], schema: e.lastM.Schema()}
}

func (e *engine) Query(query string, args ...any) (orm.Rows, error) {
	q := e.lastQ
	rows := applyOffsetLimit(applyOrder(e.match(q.Table, q.Conditions), q.OrderBy), q.Offset, q.Limit)
	return &memRows{rows: rows, schema: e.lastM.Schema(), idx: -1}, nil
}

func (e *engine) match(table string, conds []orm.Condition) []dbRow {
	idx := e.tableIndex(table)
	if idx == -1 {
		return nil
	}
	var out []dbRow
	for _, row := range e.tables[idx].rows {
		if matchRow(row, conds) {
			out = append(out, row)
		}
	}
	return out
}

// matchRow evaluates conds left-to-right; the first Logic() is ignored (mirrors real adapters).
func matchRow(row dbRow, conds []orm.Condition) bool {
	if len(conds) == 0 {
		return true
	}
	res := evalCond(row, conds[0])
	for _, c := range conds[1:] {
		if c.Logic() == "OR" {
			res = res || evalCond(row, c)
		} else {
			res = res && evalCond(row, c)
		}
	}
	return res
}

func evalCond(row dbRow, c orm.Condition) bool {
	v, ok := row.get(c.Field())
	switch c.Operator() {
	case "IS NOT NULL":
		return ok && v != nil
	case "IN":
		// Handle slices of any type by mapping to a helper
		return inSlice(v, c.Value())
	case "LIKE":
		return likeMatch(toStr(v), toStr(c.Value()))
	case "=":
		return equalAny(v, c.Value())
	case "!=":
		return !equalAny(v, c.Value())
	case ">":
		return compareAny(v, c.Value()) > 0
	case ">=":
		return compareAny(v, c.Value()) >= 0
	case "<":
		return compareAny(v, c.Value()) < 0
	case "<=":
		return compareAny(v, c.Value()) <= 0
	}
	return false
}

func inSlice(v any, listVal any) bool {
	if listVal == nil {
		return false
	}
	switch l := listVal.(type) {
	case []any:
		for _, it := range l {
			if equalAny(v, it) {
				return true
			}
		}
	case []string:
		vs := toStr(v)
		for _, it := range l {
			if vs == it {
				return true
			}
		}
	case []int:
		vf, ok := toFloat(v)
		if !ok {
			return false
		}
		for _, it := range l {
			if vf == float64(it) {
				return true
			}
		}
	case []int64:
		vf, ok := toFloat(v)
		if !ok {
			return false
		}
		for _, it := range l {
			if vf == float64(it) {
				return true
			}
		}
	}
	return false
}

func applyOrder(rows []dbRow, orders []orm.Order) []dbRow {
	for oi := len(orders) - 1; oi >= 0; oi-- { // stable, last key least significant
		col, desc := orders[oi].Column(), orders[oi].Dir() == "DESC"
		for i := 1; i < len(rows); i++ {
			for j := i; j > 0; j-- {
				lv, _ := rows[j-1].get(col)
				rv, _ := rows[j].get(col)
				cmp := compareAny(lv, rv)
				if desc {
					cmp = -cmp
				}
				if cmp <= 0 {
					break
				}
				rows[j-1], rows[j] = rows[j], rows[j-1]
			}
		}
	}
	return rows
}

func applyOffsetLimit(rows []dbRow, offset, limit int) []dbRow {
	if offset > 0 {
		if offset >= len(rows) {
			return nil
		}
		rows = rows[offset:]
	}
	if limit > 0 && limit < len(rows) {
		rows = rows[:limit]
	}
	return rows
}

type memScanner struct {
	row    dbRow
	schema []model.Field
	err    error
}

func (s *memScanner) Scan(dest ...any) error {
	if s.err != nil {
		return s.err
	}
	return scanInto(s.row, s.schema, dest)
}

type memRows struct {
	rows   []dbRow
	schema []model.Field
	idx    int
}

func (r *memRows) Next() bool             { r.idx++; return r.idx < len(r.rows) }
func (r *memRows) Scan(dest ...any) error { return scanInto(r.rows[r.idx], r.schema, dest) }
func (r *memRows) Columns() ([]string, error) {
	cols := make([]string, len(r.schema))
	for i, f := range r.schema {
		cols[i] = f.Name
	}
	return cols, nil
}
func (r *memRows) Close() error { return nil }
func (r *memRows) Err() error   { return nil }

func scanInto(row dbRow, schema []model.Field, dest []any) error {
	for i, f := range schema {
		if i >= len(dest) {
			break
		}
		if v, ok := row.get(f.Name); ok {
			if err := orm.ScanAny(v, dest[i]); err != nil {
				return err
			}
		}
	}
	return nil
}

func toStr(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	default:
		return fmt.Convert(x).String()
	}
}

func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case int:
		return float64(x), true
	case int32:
		return float64(x), true
	case int64:
		return float64(x), true
	case float32:
		return float64(x), true
	case float64:
		return x, true
	}
	return 0, false
}

func equalAny(a, b any) bool {
	if as, ok := a.(string); ok {
		return as == toStr(b)
	}
	if ab, ok := a.(bool); ok {
		bb, _ := b.(bool)
		return ab == bb
	}
	if af, aok := toFloat(a); aok {
		if bf, bok := toFloat(b); bok {
			return af == bf
		}
	}
	return false
}

func compareAny(a, b any) int {
	if af, aok := toFloat(a); aok {
		if bf, bok := toFloat(b); bok {
			switch {
			case af < bf:
				return -1
			case af > bf:
				return 1
			default:
				return 0
			}
		}
	}
	sa, sb := toStr(a), toStr(b)
	switch {
	case sa < sb:
		return -1
	case sa > sb:
		return 1
	default:
		return 0
	}
}

// likeMatch supports SQL LIKE with '%' wildcards.
func likeMatch(s, pattern string) bool {
	if findHelper(pattern, "%") == -1 {
		return s == pattern
	}

	if pattern == "%" {
		return true
	}

	var segments []string
	var current []byte
	for i := 0; i < len(pattern); i++ {
		if pattern[i] == '%' {
			if len(current) > 0 {
				segments = append(segments, string(current))
				current = nil
			}
		} else {
			current = append(current, pattern[i])
		}
	}
	if len(current) > 0 {
		segments = append(segments, string(current))
	}

	hasPrefix := pattern[0] != '%'
	hasSuffix := pattern[len(pattern)-1] != '%'

	if len(segments) == 0 {
		return true
	}

	str := s
	for i, seg := range segments {
		if i == 0 && hasPrefix {
			if !hasPrefixHelper(str, seg) {
				return false
			}
			str = str[len(seg):]
			continue
		}

		if i == len(segments)-1 && hasSuffix {
			return hasSuffixHelper(str, seg)
		}

		idx := findHelper(str, seg)
		if idx == -1 {
			return false
		}
		str = str[idx+len(seg):]
	}

	return true
}

func hasPrefixHelper(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	return s[:len(prefix)] == prefix
}

func hasSuffixHelper(s, suffix string) bool {
	if len(s) < len(suffix) {
		return false
	}
	return s[len(s)-len(suffix):] == suffix
}

func findHelper(s, sub string) int {
	if len(sub) == 0 {
		return 0
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

var (
	_ orm.Executor        = (*engine)(nil)
	_ orm.Compiler        = (*engine)(nil)
	_ orm.TxExecutor      = (*engine)(nil)
	_ orm.TxBoundExecutor = (*engine)(nil)
	_ orm.Scanner         = (*memScanner)(nil)
	_ orm.Rows            = (*memRows)(nil)
)
