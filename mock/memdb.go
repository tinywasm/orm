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
	e := &engine{tables: map[string][]map[string]any{}}
	return orm.New(e, e)
}

type engine struct {
	tables map[string][]map[string]any // table -> rows (column -> value)
	lastQ  orm.Query
	lastM  model.Model
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
	case orm.ActionCreateTable:
		if _, ok := e.tables[q.Table]; !ok {
			e.tables[q.Table] = nil
		}
	case orm.ActionDropTable:
		delete(e.tables, q.Table)
	case orm.ActionCreate:
		row := map[string]any{}
		for i, col := range q.Columns {
			if i < len(q.Values) {
				row[col] = q.Values[i]
			}
		}
		// Auto-vivifies the table: append sets the map key even if CreateTable was never called.
		// This is why the mock Factory in orm/conformance needs no DDL — it just returns NewDB().
		e.tables[q.Table] = append(e.tables[q.Table], row)
	case orm.ActionUpdate:
		isPK := map[string]bool{}
		if e.lastM != nil {
			for _, f := range e.lastM.Schema() {
				if f.IsPK() {
					isPK[f.Name] = true
				}
			}
		}
		for _, row := range e.match(q.Table, q.Conditions) { // match returns stored map refs
			for i, col := range q.Columns {
				if isPK[col] {
					continue // do not overwrite PK on update
				}
				if i < len(q.Values) {
					row[col] = q.Values[i]
				}
			}
		}
	case orm.ActionDelete:
		kept := e.tables[q.Table][:0:0]
		for _, row := range e.tables[q.Table] {
			if !matchRow(row, q.Conditions) {
				kept = append(kept, row)
			}
		}
		e.tables[q.Table] = kept
	default: // CreateDatabase / AddColumn / RenameColumn / DropColumn: no-op
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

func (e *engine) match(table string, conds []orm.Condition) []map[string]any {
	var out []map[string]any
	for _, row := range e.tables[table] {
		if matchRow(row, conds) {
			out = append(out, row)
		}
	}
	return out
}

// matchRow evaluates conds left-to-right; the first Logic() is ignored (mirrors real adapters).
func matchRow(row map[string]any, conds []orm.Condition) bool {
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

func evalCond(row map[string]any, c orm.Condition) bool {
	v, ok := row[c.Field()]
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

func applyOrder(rows []map[string]any, orders []orm.Order) []map[string]any {
	for oi := len(orders) - 1; oi >= 0; oi-- { // stable, last key least significant
		col, desc := orders[oi].Column(), orders[oi].Dir() == "DESC"
		for i := 1; i < len(rows); i++ {
			for j := i; j > 0; j-- {
				cmp := compareAny(rows[j-1][col], rows[j][col])
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

func applyOffsetLimit(rows []map[string]any, offset, limit int) []map[string]any {
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
	row    map[string]any
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
	rows   []map[string]any
	schema []model.Field
	idx    int
}

func (r *memRows) Next() bool { r.idx++; return r.idx < len(r.rows) }
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

func scanInto(row map[string]any, schema []model.Field, dest []any) error {
	for i, f := range schema {
		if i >= len(dest) {
			break
		}
		if v, ok := row[f.Name]; ok {
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
