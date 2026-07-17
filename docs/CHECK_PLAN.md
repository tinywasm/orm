---
PLAN: "feat: conformance (contrato ejecutable de backend) + subpaquete mock (recorders + *orm.DB en memoria)"
TAG: v0.10.0
---

# PLAN — `tinywasm/orm`: `conformance/` (solo DML) + `mock/`

Orquestado por [`DDL_DML_SPLIT_MASTER_PLAN.md`](https://github.com/tinywasm/app-releases/blob/main/docs/DDL_DML_SPLIT_MASTER_PLAN.md)
— **pieza #1, Ola A**. Autocontenido, en español. Eres un agente **sin contexto previo** y **solo tienes
este repo** (`github.com/tinywasm/orm`). Todo el contrato y el código exacto van inline.

> **Alcance DDL/DML.** `orm` es el runtime de **DML** (operar datos con la tabla ya lista). Todo lo de
> **DDL** (CreateTable/DropTable/Sync/esquema) vive en el repo hermano `tinywasm/ddl` y tiene su propio
> `ddl/conformance`. Esta fase **NO** toca los métodos DDL actuales de `orm.DB` (siguen ahí, deprecados,
> hasta la Ola C del master) — solo crea un `orm/conformance` que prueba **exclusivamente DML**, para que
> `indexdb` (cuyo esquema no es DDL SQL) conforme igual que los demás.

> **Prerequisito (entorno del agente):**
> ```bash
> go install github.com/tinywasm/devflow/cmd/gotest@latest
> ```
> Corre tests SIEMPRE con `gotest` (no `go test`). Publica SIEMPRE con `gopush 'mensaje'` (no
> `git commit`/`git push`). El tag lo pone `gopush`.

## 1. Qué se hace y por qué

Dos entregables en este repo, uno habilita al otro:

### A. `orm/conformance/` — el contrato ejecutable de un backend de almacenamiento

La interfaz `orm.Executor`+`orm.Compiler` dice las **firmas**; no dice el **comportamiento**. Dos
backends pueden satisfacer la interfaz (compilar sin queja) y discrepar en todo lo que importa: uno
filtra bien un `WHERE tenant_id = ? AND is_active = ?`, otro ignora la segunda condición; uno respeta
`ORDER BY ... DESC`, otro no; uno mapea "sin filas" a `ErrNoRows`, otro devuelve basura. El compilador
no puede atrapar "estos dos backends se comportan distinto" — ambos satisfacen la interfaz. Así que el
contrato tiene que volverse algo que **se pone en rojo**. Eso es `conformance`.

Es el mismo patrón que `github.com/tinywasm/router/conformance` (y que la stdlib usa para lo mismo:
`testing/fstest.TestFS`, `golang.org/x/net/nettest.TestConn`): un paquete **no-`_test`** que importa
`testing` a propósito (un `_test.go` no puede importarse desde otro repo), expone `Run(t, Factory)`, y
cada cláusula de comportamiento es un `t.Run`. Cada backend prueba conformidad desde su propio paquete
de test:

```go
func TestSqliteConformance(t *testing.T) {
    conformance.Run(t, conformance.Factory{Name: "sqlite", New: func(t *testing.T, models ...model.Model) *orm.DB {
        db, err := sqlite.Open(":memory:"); if err != nil { t.Fatal(err) }
        return db
    }})
}
```

Este repo (orm) es el dueño del contrato. Los backends que lo prueban en fases hermanas:
`tinywasm/mock` (aquí mismo, §5), `tinywasm/sqlt`, `tinywasm/postgres`, `tinywasm/indexdb`.

### B. `orm/mock/` — dobles de test reutilizables

Hoy los dobles viven **duplicados y privados** dentro de los tests: `tests/setup_test.go`
(`package tests`) define `MockExecutor`/`MockCompiler`/`MockScanner`/`MockRows`/`MockModel`/
`MockTxExecutor`/`MockTxBoundExecutor` (recorders: capturan la llamada, no almacenan datos), y
`db_test.go` (`package orm`) redeclara sus propios `mockCompiler`/`mockTxExecutor` privados. Ningún
consumidor externo puede reusarlos. Un módulo hoja (p. ej. `veltylabs/item_catalog`) que quiere probar
su lógica contra un `*orm.DB` **sin importar un driver real** (`tinywasm/sqlite`) no tiene con qué:
se acopla a `sqlite` solo en sus tests, contradiciendo el diseño (orm desacopla el motor).

`orm/mock` expone:
1. **Recorders** (se **trasladan** desde `tests/setup_test.go`, exportados sin *stutter*):
   `mock.Executor`, `mock.Compiler`, `mock.Scanner`, `mock.Rows`, `mock.Model`, `mock.TxExecutor`,
   `mock.TxBoundExecutor`. Verifican **delegación** (que orm arma el `Query`/`Plan` correcto).
2. **Motor en memoria funcional**: `mock.NewDB() *orm.DB` — almacena filas e interpreta el `orm.Query`
   estructurado (Create/ReadOne/ReadAll/Update/Delete + Conditions/OrderBy/Limit/Offset). **No parsea
   SQL.** Es lo que permite a un módulo hoja probar round-trips sin driver real. **Y prueba el contrato
   de §A**: `mock.NewDB` es un backend más que corre `conformance.Run`.

> **Fuera de alcance.** No se modifica `item_catalog` ni ningún consumidor; solo se publica orm.
> La migración de `item_catalog` (borrar `tinywasm/sqlite`, usar `mock.NewDB()`) es un cambio posterior
> en ese repo.

## 2. Contratos verificados en este repo (no supuestos)

- `orm.Executor` (`executor.go`): `Exec(q string, args ...any) error`; `QueryRow(q string, args ...any)
  orm.Scanner`; `Query(q string, args ...any) (orm.Rows, error)`; `Close() error`.
- `orm.Scanner`: `Scan(dest ...any) error`. `orm.Rows`: `Next() bool`; `Scan(...) error`;
  `Columns() ([]string, error)`; `Close() error`; `Err() error`.
- `orm.Compiler` (`compiler.go`): `Compile(q orm.Query, m model.Model) (orm.Plan, error)`.
- `orm.TxExecutor`/`orm.TxBoundExecutor` (`tx.go`).
- `orm.New(exec, compiler) *orm.DB` (`db.go`). Métodos públicos de `*orm.DB`: `Create`, `Update(m, cond,
  ...)`, `Delete(m, cond, ...)`, `CreateTable`, `DropTable`, `CreateDatabase`, `Query(m) *QB`, `Close`,
  `Tx(fn)`.
- `orm.QB` (`qb.go`): `Where(col).Eq/Neq/Gt/Gte/Lt/Lte/Like/In(v)`, `Or()`, `Limit`, `Offset`,
  `OrderBy(col).Asc()/.Desc()`, `ReadOne()`, `ReadAll(new, onRow)`. `ReadOne` mapea `ErrNoRows`→`ErrNotFound`.
- `orm.Query` (`query.go`): `Action`, `Table`, `Columns []string`, `Values []any`, `Conditions
  []orm.Condition`, `OrderBy []orm.Order`, `Limit`, `Offset`, `Database`.
- `orm.Action`: `ActionCreate/ReadOne/Update/Delete/ReadAll/CreateTable/DropTable/CreateDatabase/
  AddColumn/RenameColumn/DropColumn`.
- `orm.Condition` (`conditions.go`): `Field()/Operator()/Value()/Logic()`. Operadores: `=`, `!=`, `>`,
  `>=`, `<`, `<=`, `LIKE`, `IN`, `IS NOT NULL`. Lógica `"AND"`/`"OR"`; el `Logic()` de la **primera**
  condición se ignora (ver `sqlt/translate.go:buildConditions`, la condición 0 no antepone lógica).
- `orm.Order`: `Column()/Dir()` (`"ASC"`/`"DESC"`). `orm.Plan` (`execution_plan.go`): `Mode/Query/Args`.
- `orm.ErrNoRows`, `orm.ErrNotFound` (`errors.go`). `orm.ScanAny(v any, dest any) error` (`scan.go`) —
  reusar para copiar valor→puntero (`*string/*int/*int64/*float64/*bool/*[]byte/*any`).
- `model.Model` (`interface.go`): `Schema() []model.Field` + `Pointers() []any` + `ModelName() string` +
  `EncodeFields(FieldWriter)`/`DecodeFields(FieldReader)` + `IsNil() bool`.
- `model.Definition{Name string; Fields model.Fields}`, `model.Field{Name, Type Kind, NotNull, DB
  *FieldDB}`, `model.FieldDB{PK bool}`, kinds `model.Text()/Int()/Bool()/Float()`. `FieldWriter`:
  `String/Int/Bool/Float(name, v)`. `FieldReader`: `String/Int/Bool/Float(name) (v, ok)`.
- `model.ReadValues(schema, ptrs) []any` — cómo orm obtiene valores tipados
  (`string/int64/float64/bool/[]byte`).

## 3. `orm/conformance/` — el contrato

`package conformance`, importa `testing` + `orm` + `model`. Solo esos (todos wasm-safe, para que
`indexdb` pueda importarlo bajo `//go:build wasm`).

### 3.1 Modelo canónico (fixture con schema real, para que el DDL funcione en backends SQL)

```go
package conformance

import (
	"github.com/tinywasm/model"
	"github.com/tinywasm/orm"
)

// Widget is the canonical record every backend is driven with. Its schema carries real DB
// metadata (types + PK) so SQL backends can CREATE TABLE it; the mock ignores the metadata
// and stores by column name. Hand-written (conformance depends only on model, not ormc).
var WidgetModel = model.Definition{
	Name: "conformance_widget",
	Fields: model.Fields{
		{Name: "id", Type: model.Text(), DB: &model.FieldDB{PK: true}},
		{Name: "name", Type: model.Text(), NotNull: true},
		{Name: "qty", Type: model.Int(), NotNull: true},
		{Name: "active", Type: model.Bool(), NotNull: true},
	},
}

type Widget struct {
	Id     string
	Name   string
	Qty    int64
	Active bool
}

func (w *Widget) ModelName() string     { return WidgetModel.Name }
func (w *Widget) Schema() []model.Field  { return WidgetModel.Fields }
func (w *Widget) Pointers() []any        { return []any{&w.Id, &w.Name, &w.Qty, &w.Active} }
func (w *Widget) IsNil() bool            { return w == nil }
func (w *Widget) EncodeFields(wr model.FieldWriter) {
	wr.String("id", w.Id); wr.String("name", w.Name); wr.Int("qty", w.Qty); wr.Bool("active", w.Active)
}
func (w *Widget) DecodeFields(r model.FieldReader) {
	if v, ok := r.String("id"); ok { w.Id = v }
	if v, ok := r.String("name"); ok { w.Name = v }
	if v, ok := r.Int("qty"); ok { w.Qty = v }
	if v, ok := r.Bool("active"); ok { w.Active = v }
}

var _ model.Model = (*Widget)(nil)
```

> Verifica las firmas exactas de `model.Text()/Int()/Bool()` y `FieldWriter`/`FieldReader` con
> `go doc github.com/tinywasm/model` antes de compilar — usa las reales si difieren de arriba. La columna
> `qty` mapea a `int64` (storage de `FieldInt`); `Pointers()` debe apuntar a un `int64`, no `int`.

### 3.2 Factory + Run

```go
// Factory builds, for ONE clause, a fresh *orm.DB whose Widget table already exists and is EMPTY.
// Schema setup is the backend's job, DONE OUTSIDE orm's DML contract (this suite never calls DDL):
//   - mock:     the in-memory engine auto-creates the table on first Create — New just returns NewDB().
//   - sqlite/postgres: New runs the dialect's ddlc.ExportDDL(models) (DROP+CREATE) before returning.
//   - indexdb:  New declares `models` as IndexedDB object stores (structTables) up front.
// models are the record types the suite will exercise. Called once per clause → no cross-clause bleed.
type Factory struct {
	Name string
	New  func(t *testing.T, models ...model.Model) *orm.DB
}

func Run(t *testing.T, f Factory) {
	if f.New == nil {
		t.Fatal("conformance: Factory.New is required")
	}
	t.Run("create_then_read_one_by_pk", func(t *testing.T) { createThenReadOneByPK(t, f) })
	t.Run("read_one_no_match_is_not_found", func(t *testing.T) { readOneNoMatchIsNotFound(t, f) })
	t.Run("read_all_returns_every_row", func(t *testing.T) { readAllReturnsEveryRow(t, f) })
	t.Run("read_all_filters_by_eq", func(t *testing.T) { readAllFiltersByEq(t, f) })
	t.Run("read_all_ands_two_conditions", func(t *testing.T) { readAllAndsTwoConditions(t, f) })
	t.Run("read_all_ors_conditions", func(t *testing.T) { readAllOrsConditions(t, f) })
	t.Run("read_all_orders_asc_and_desc", func(t *testing.T) { readAllOrdersAscDesc(t, f) })
	t.Run("read_all_applies_limit_and_offset", func(t *testing.T) { readAllLimitOffset(t, f) })
	t.Run("comparison_operators_filter", func(t *testing.T) { comparisonOperatorsFilter(t, f) }) // > >= < <= !=
	t.Run("in_operator_filters", func(t *testing.T) { inOperatorFilters(t, f) })
	t.Run("update_changes_matched_rows_only", func(t *testing.T) { updateChangesMatchedOnly(t, f) })
	t.Run("delete_removes_matched_rows_only", func(t *testing.T) { deleteRemovesMatchedOnly(t, f) })
}
```

> **Sin DDL en la suite.** No hay cláusula `create_table`/`drop_table` — eso lo prueba
> `ddl/conformance` (repo `tinywasm/ddl`), solo para backends SQL. Aquí `orm` es puro DML: la tabla
> llega lista del `Factory`. El aislamiento entre cláusulas lo da que `New` se llama una vez por cláusula
> y devuelve una BD/tabla fresca (mock: `NewDB()` nuevo; sqlite: `:memory:` nuevo; postgres: DROP+CREATE;
> indexdb: `dbName` único).

### 3.3 Setup por-cláusula + 2 cláusulas de ejemplo

`setup` **no hace DDL**: pide al `Factory` una BD con la tabla ya lista y solo siembra filas vía `Create`
(que es DML):

```go
func setup(t *testing.T, f Factory, seed ...*Widget) *orm.DB {
	t.Helper()
	db := f.New(t, &Widget{}) // table already exists & empty — backend set it up, not this suite
	for _, w := range seed {
		if err := db.Create(w); err != nil {
			t.Fatalf("seed Create(%+v): %v", w, err)
		}
	}
	return db
}

func readAll(t *testing.T, qb *orm.QB) []*Widget {
	t.Helper()
	var out []*Widget
	err := qb.ReadAll(func() model.Model { return &Widget{} }, func(m model.Model) {
		out = append(out, m.(*Widget))
	})
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	return out
}

func createThenReadOneByPK(t *testing.T, f Factory) {
	db := setup(t, f, &Widget{Id: "w1", Name: "alpha", Qty: 3, Active: true})
	var got Widget
	err := db.Query(&got).Where("id").Eq("w1").ReadOne()
	if err != nil {
		t.Fatalf("ReadOne: %v", err)
	}
	if got.Name != "alpha" || got.Qty != 3 || got.Active != true {
		t.Errorf("round-trip mismatch: got %+v", got)
	}
}

func readAllAndsTwoConditions(t *testing.T, f Factory) {
	db := setup(t, f,
		&Widget{Id: "a", Name: "x", Qty: 1, Active: true},
		&Widget{Id: "b", Name: "x", Qty: 1, Active: false},
		&Widget{Id: "c", Name: "y", Qty: 1, Active: true},
	)
	got := readAll(t, db.Query(&Widget{}).Where("name").Eq("x").Where("active").Eq(true))
	if len(got) != 1 || got[0].Id != "a" {
		t.Errorf("AND of two conditions must return only {a}; got %+v", got)
	}
}
```

**Resto de cláusulas** (misma forma: `setup` con seed, ejerce el `QB`/método, asevera el conjunto
resultante):

| Cláusula | Qué prueba |
|---|---|
| `readOneNoMatchIsNotFound` | `ReadOne` sin match ⇒ `err == orm.ErrNotFound` (mapea `ErrNoRows`). |
| `readAllReturnsEveryRow` | sin `Where` ⇒ todas las filas sembradas. |
| `readAllFiltersByEq` | un `Where().Eq()` ⇒ solo las que igualan. |
| `readAllOrsConditions` | `Where(a).Eq(x).Or().Where(b).Eq(y)` ⇒ unión. |
| `readAllOrdersAscDesc` | `OrderBy("qty").Asc()`/`.Desc()` ⇒ orden correcto. |
| `readAllLimitOffset` | `Limit(2).Offset(1)` sobre orden estable ⇒ ventana correcta. |
| `comparisonOperatorsFilter` | `Gt/Gte/Lt/Lte/Neq` sobre `qty` ⇒ subconjunto correcto. |
| `inOperatorFilters` | `Where("id").In([]any{"a","c"})` ⇒ {a,c}. |
| `updateChangesMatchedOnly` | `Update` con cond ⇒ solo filas que matchean cambian; verifica con `ReadOne`. |
| `deleteRemovesMatchedOnly` | `Delete` con cond ⇒ solo filas que matchean desaparecen. |

> **Nota `In`:** `QB.In(value any)` recibe el valor tal cual; los adapters SQL esperan `[]any` (ver
> `sqlt/translate.go`). La cláusula pasa `[]any{...}` para ser fiel a todos los backends.

## 4. `orm/mock/` — recorders + motor en memoria

Estructura:
```
mock/
  recorders.go   // dobles que capturan llamadas (trasladados desde tests/setup_test.go)
  memdb.go       // motor en memoria funcional + NewDB()
  mock_test.go   // 100% cobertura + conformance.Run(t, {New: mock.NewDB adapter})
```
`package mock`, importa `orm` + `model` (+ `fmt` de tinywasm). Sin ciclo: orm no importa mock.

### 4.1 `recorders.go` — traslado + destutter

Copia las 7 definiciones de `tests/setup_test.go` a `mock/recorders.go`, renombrando para no tartamudear
(`mock.MockExecutor` → `mock.Executor`). El cuerpo de cada método es **idéntico** al actual (mismos
campos, misma lógica); solo cambia el nombre del tipo y el paquete:

| En `tests/setup_test.go` | En `mock/recorders.go` |
|---|---|
| `MockExecutor` | `Executor` |
| `MockCompiler` | `Compiler` |
| `MockScanner` | `Scanner` |
| `MockRows` | `Rows` |
| `MockModel` | `Model` |
| `MockTxExecutor` | `TxExecutor` |
| `MockTxBoundExecutor` | `TxBoundExecutor` |

No cambies los *receivers* de `Model` (value vs pointer) — solo el nombre. Añade al final:

```go
var (
	_ orm.Executor        = (*Executor)(nil)
	_ orm.Compiler        = (*Compiler)(nil)
	_ orm.Scanner         = (*Scanner)(nil)
	_ orm.Rows            = (*Rows)(nil)
	_ model.Model         = (*Model)(nil)
	_ orm.TxExecutor      = (*TxExecutor)(nil)
	_ orm.TxBoundExecutor = (*TxBoundExecutor)(nil)
)
```

### 4.2 `memdb.go` — motor en memoria funcional

Un único tipo `engine` implementa **a la vez** `orm.Compiler` y `orm.Executor`. Funciona porque orm
siempre llama `compiler.Compile(q, m)` **inmediatamente antes** de `exec.Exec/QueryRow/Query(...)` sobre
el mismo `*orm.DB` en la misma goroutine (`db.go`/`qb.go`): `Compile` guarda el `Query`+`model`; el
`Exec/Query` lo consume. Cero SQL.

```go
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

func (e *engine) Exec(query string, args ...any) error {
	q := e.lastQ
	switch q.Action {
	case orm.ActionCreateTable:
		if _, ok := e.tables[q.Table]; !ok { e.tables[q.Table] = nil }
	case orm.ActionDropTable:
		delete(e.tables, q.Table)
	case orm.ActionCreate:
		row := map[string]any{}
		for i, col := range q.Columns {
			if i < len(q.Values) { row[col] = q.Values[i] }
		}
		// Auto-vivifies the table: append sets the map key even if CreateTable was never called.
		// This is why the mock Factory in orm/conformance needs no DDL — it just returns NewDB().
		e.tables[q.Table] = append(e.tables[q.Table], row)
	case orm.ActionUpdate:
		for _, row := range e.match(q.Table, q.Conditions) { // match returns stored map refs
			for i, col := range q.Columns {
				if i < len(q.Values) { row[col] = q.Values[i] }
			}
		}
	case orm.ActionDelete:
		kept := e.tables[q.Table][:0:0]
		for _, row := range e.tables[q.Table] {
			if !matchRow(row, q.Conditions) { kept = append(kept, row) }
		}
		e.tables[q.Table] = kept
	default: // CreateDatabase / AddColumn / RenameColumn / DropColumn: no-op
	}
	return nil
}

func (e *engine) QueryRow(query string, args ...any) orm.Scanner {
	q := e.lastQ
	rows := applyOffsetLimit(applyOrder(e.match(q.Table, q.Conditions), q.OrderBy), q.Offset, 1)
	if len(rows) == 0 { return &memScanner{err: orm.ErrNoRows} }
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
		if matchRow(row, conds) { out = append(out, row) }
	}
	return out
}

// matchRow evaluates conds left-to-right; the first Logic() is ignored (mirrors real adapters).
func matchRow(row map[string]any, conds []orm.Condition) bool {
	if len(conds) == 0 { return true }
	res := evalCond(row, conds[0])
	for _, c := range conds[1:] {
		if c.Logic() == "OR" { res = res || evalCond(row, c) } else { res = res && evalCond(row, c) }
	}
	return res
}

func evalCond(row map[string]any, c orm.Condition) bool {
	v, ok := row[c.Field()]
	switch c.Operator() {
	case "IS NOT NULL": return ok && v != nil
	case "IN":
		list, _ := c.Value().([]any)
		for _, it := range list { if equalAny(v, it) { return true } }
		return false
	case "LIKE": return likeMatch(toStr(v), toStr(c.Value()))
	case "=":  return equalAny(v, c.Value())
	case "!=": return !equalAny(v, c.Value())
	case ">":  return compareAny(v, c.Value()) > 0
	case ">=": return compareAny(v, c.Value()) >= 0
	case "<":  return compareAny(v, c.Value()) < 0
	case "<=": return compareAny(v, c.Value()) <= 0
	}
	return false
}

func applyOrder(rows []map[string]any, orders []orm.Order) []map[string]any {
	for oi := len(orders) - 1; oi >= 0; oi-- { // stable, last key least significant
		col, desc := orders[oi].Column(), orders[oi].Dir() == "DESC"
		for i := 1; i < len(rows); i++ {
			for j := i; j > 0; j-- {
				cmp := compareAny(rows[j-1][col], rows[j][col])
				if desc { cmp = -cmp }
				if cmp <= 0 { break }
				rows[j-1], rows[j] = rows[j], rows[j-1]
			}
		}
	}
	return rows
}

func applyOffsetLimit(rows []map[string]any, offset, limit int) []map[string]any {
	if offset > 0 {
		if offset >= len(rows) { return nil }
		rows = rows[offset:]
	}
	if limit > 0 && limit < len(rows) { rows = rows[:limit] }
	return rows
}

type memScanner struct {
	row    map[string]any
	schema []model.Field
	err    error
}
func (s *memScanner) Scan(dest ...any) error {
	if s.err != nil { return s.err }
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
	for i, f := range r.schema { cols[i] = f.Name }
	return cols, nil
}
func (r *memRows) Close() error { return nil }
func (r *memRows) Err() error   { return nil }

func scanInto(row map[string]any, schema []model.Field, dest []any) error {
	for i, f := range schema {
		if i >= len(dest) { break }
		if v, ok := row[f.Name]; ok {
			if err := orm.ScanAny(v, dest[i]); err != nil { return err }
		}
	}
	return nil
}

var (
	_ orm.Executor = (*engine)(nil)
	_ orm.Compiler = (*engine)(nil)
	_ orm.Scanner  = (*memScanner)(nil)
	_ orm.Rows     = (*memRows)(nil)
)
```

Helpers de comparación (mismo archivo) — cubren los tipos que `model.ReadValues` produce
(`string/int64/float64/bool/[]byte`) y los literales de las condiciones:

```go
func toStr(v any) string {
	switch x := v.(type) {
	case string: return x
	case []byte: return string(x)
	default:     return fmt.Convert(x).String()
	}
}
func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case int:     return float64(x), true
	case int64:   return float64(x), true
	case float64: return x, true
	}
	return 0, false
}
func equalAny(a, b any) bool {
	if as, ok := a.(string); ok { return as == toStr(b) }
	if ab, ok := a.(bool); ok { bb, _ := b.(bool); return ab == bb }
	if af, aok := toFloat(a); aok { if bf, bok := toFloat(b); bok { return af == bf } }
	return false
}
func compareAny(a, b any) int {
	if af, aok := toFloat(a); aok {
		if bf, bok := toFloat(b); bok {
			switch { case af < bf: return -1; case af > bf: return 1; default: return 0 }
		}
	}
	sa, sb := toStr(a), toStr(b)
	switch { case sa < sb: return -1; case sa > sb: return 1; default: return 0 }
}
// likeMatch supports SQL LIKE with '%' wildcards. Implement with tinywasm/fmt string ops
// (Split on "%", check ordered literal segments, honor leading/trailing '%'). If tinywasm/fmt
// lacks an index/suffix helper, write a trivial private []byte loop — do NOT import stdlib "strings".
func likeMatch(s, pattern string) bool { /* ver §4.2 nota */ return false }
```

> **`fmt`/strings.** Este repo usa `github.com/tinywasm/fmt`, no stdlib. Usa `fmt.Convert(x).String()`,
> `fmt.Convert(p).Split("%")`, etc. Verifica su API con `go doc github.com/tinywasm/fmt`; si falta un
> helper (índice/sufijo), impleméntalo a mano con bucles `[]byte`. Prohibido importar stdlib
> `strings`/`fmt` o cualquier driver.

## 5. `mock_test.go` — 100% cobertura + prueba de conformidad

`package mock`. Debe dejar **`gotest` con 100% de cobertura del paquete `mock`** y correr la suite:

```go
func TestMockConformance(t *testing.T) {
	conformance.Run(t, conformance.Factory{
		Name: "mock",
		New:  func(t *testing.T, models ...model.Model) *orm.DB { return NewDB() },
	})
}
```

Además, tests directos hasta 100% (la conformance cubre el motor, pero **no** los recorders):
- **Recorders**: `Executor.Exec/QueryRow/Query/Close` (ramas `ReturnQueryRow==nil`, `ReturnQueryRows==nil`,
  y con valores inyectados); `Compiler.Compile` (rama `ReturnPlan.Query==""` y con plan puesto);
  `Scanner.Scan` (con/sin `ScanErr`); `Rows.Next/Scan/Columns/Close/Err`; `Model.*` (incl. `Validate`
  con/sin `ValidErr`); `TxExecutor.BeginTx` (ramas `BeginTxErr`, `Bound==nil`, `Bound` preexistente);
  `TxBoundExecutor.Commit/Rollback`.
- **Motor**: ramas que la conformance no toque — `LIKE` (patrón con `%` al inicio/medio/fin y literal
  exacto), `IS NOT NULL`, `compareAny`/`equalAny` con bool y con string, `toFloat` con `int`, `offset >=
  len(rows)` ⇒ nil, `default` de `Exec` (`db.CreateDatabase("x")` no-op), `Close()`.

> Comprueba con `gotest` + cobertura; añade casos hasta 100% del paquete `mock`. Los tests de
> `tests/`/`orm_test` no cuentan para ese 100%.

## 6. Reusar los recorders en los tests de orm (sin duplicados)

- **`tests/setup_test.go`**: borra las 7 definiciones (viven ahora en `mock/`). Añade
  `import "github.com/tinywasm/orm/mock"`. En `tests/*.go` reemplaza usos: `&MockExecutor{}` →
  `&mock.Executor{}`, `&MockModel{...}` → `&mock.Model{...}`, etc. Los **campos** no cambian, solo el
  tipo. Si `setup_test.go` queda vacío, elimínalo.
- **`db_test.go`** (`package orm`): no puede importar `orm/mock` (ciclo). Si solo ejercita la API pública
  (`New/Create/Update/Delete/Tx/Query`), conviértelo a `package orm_test` (test externo, mismo
  directorio) e importa `orm/mock`; borra sus `mockCompiler`/`mockTxExecutor` privados. Si toca internals
  no exportados, mantenlo en `package orm` con un doble mínimo local **solo** para ese internal — sin
  redeclarar los recorders. No debe quedar copia de recorders fuera de `mock/`.

## 7. Criterios de aceptación

- `github.com/tinywasm/orm/conformance` existe: `Run(t, Factory)`, `Factory{Name, New}`, modelo `Widget`
  exportado, **12 cláusulas solo-DML** (cero DDL — la suite nunca llama `CreateTable`/`DropTable`; la
  tabla llega lista del `Factory`). Importa **solo** `testing`+`orm`+`model` (compila bajo `//go:build wasm`).
- `github.com/tinywasm/orm/mock` existe: recorders exportados sin *stutter* (`var _ orm.X` asserts) +
  `mock.NewDB() *orm.DB` funcional. No importa ningún driver ni stdlib `strings`/`fmt`/`database/sql`.
- `mock_test.go` corre `conformance.Run` verde y deja **100% de cobertura** del paquete `mock`.
- `tests/setup_test.go` ya no define dobles; `db_test.go` no redeclara recorders. `grep -rn
  "MockExecutor\|MockCompiler\|MockModel\|MockScanner\|MockRows\|MockTxExecutor" .` fuera de `mock/`:
  vacío o solo `mock.*`.
- `gotest` verde en todo el módulo; `orm` compila y sus tests existentes pasan.
- Publicado con `gopush`. (Backends hermanos —sqlt/postgres/indexdb— consumen `orm@v0.10.0`+ en sus
  propias fases.)

## 8. Etapas

| # | Etapa | Archivo(s) | Criterio |
|---|---|---|---|
| 1 | Modelo canónico | `conformance/model.go` | `Widget`+`WidgetModel`, `var _ model.Model` |
| 2 | Factory + Run + cláusulas | `conformance/conformance.go` | `Run`/`Factory`/`setup`/12 `t.Run` DML |
| 3 | Recorders (traslado+destutter) | `mock/recorders.go` | 7 tipos + asserts |
| 4 | Motor en memoria | `mock/memdb.go` | `NewDB()`+`engine`+helpers |
| 5 | Test mock + conformance | `mock/mock_test.go` | conformance verde, cobertura 100% |
| 6 | Reusar en tests de orm | `tests/*.go`, `db_test.go` | sin duplicados |
| 7 | Verificar + publicar | — | `gotest` verde; `gopush 'feat: conformance + mock'` |

## 9. Cierre (ciclo de vida del plan)

 La parte duradera del diseño (qué es `conformance` y por
qué, el motor en memoria de `mock`, cómo un módulo hoja usa `mock.NewDB()` en vez de un driver) se
traslada como sección corta a `docs/ARQUITECTURE.md`.
