---
PLAN: "refactor!: orm pasa a ser capa ergonómica sobre tinywasm/storage (contrato movido al puerto)"
TAG: v0.11.0
---

Orquestado por
[`DB_PORT_MASTER_PLAN.md`](https://github.com/tinywasm/app/blob/main/docs/DB_PORT_MASTER_PLAN.md)
— **pieza #2**. Autocontenido, en español. Eres un agente **sin contexto previo** y **solo tienes este
repo** (`github.com/tinywasm/orm`). Todo el contrato y el código exacto van inline.

> **Prerequisito (entorno del agente):**
> ```bash
> go install github.com/tinywasm/devflow/cmd/gotest@latest
> ```
> Tests SIEMPRE con `gotest` (no `go test`). Publica SIEMPRE con `gopush 'mensaje'`. El tag lo pone
> `gopush`. Este plan **requiere `github.com/tinywasm/storage@v0.0.1` ya publicado** — si no existe, para y
> repórtalo, no lo repliques localmente.

## 0. Qué cambia y por qué

`orm` definía hoy el contrato de almacenamiento (`Executor`/`Compiler`/`Query`/`Condition`/`Order`/
`Plan`, `ErrNoRows`, `ScanAny`) **y** la API ergonómica (`orm.DB`, query builder). Ese contrato se
extrajo a un puerto nuevo, `tinywasm/storage` — el equivalente de `database/sql/driver`. `orm` pasa a ser
el equivalente de `database/sql`: una capa ergonómica **opcional** sobre `storage`, sin definir ninguna
interfaz que un backend deba implementar.

Razonamiento completo: [`DB_PORT_PROPOSAL.md`](https://github.com/tinywasm/app-releases/blob/main/docs/DB_PORT_PROPOSAL.md).
No lo necesitas para ejecutar este plan (es autocontenido), pero si algo aquí te resulta arbitrario,
ahí está el porqué.

**Alcance: SOLO `tinywasm/orm`.** No toques `tinywasm/storage`, `tinywasm/ddl`, ni ningún backend — deben
estar publicados/planificados por separado. Si `github.com/tinywasm/storage` no resuelve en `go get`, el
plan no es ejecutable todavía; repórtalo.

## 1. Qué se mueve a `storage` (ya no vive en `orm` tras este plan)

| Símbolo | Antes (`orm`) | Ahora |
|---|---|---|
| `Executor`, `Scanner`, `Rows` | `executor.go` | `storage.Executor`, `storage.Scanner`, `storage.Rows` |
| `Compiler` | `compiler.go` | `storage.Compiler` |
| `TxExecutor`, `TxBoundExecutor` (interfaces) | `tx.go` | `storage.TxExecutor`, `storage.TxBoundExecutor` |
| `Query`, `Action`, `Order` | `query.go` | `storage.Query`, `storage.Action`, `storage.Order` |
| `Condition` + `Eq/Neq/Gt/Gte/Lt/Lte/Like/In/Or/IsNotNull` | `conditions.go` | `storage.Condition` + mismas funciones en `storage` |
| `Plan` | `execution_plan.go` | `storage.Plan` |
| `ErrNoRows` | `errors.go` | `storage.ErrNoRows` |
| `ScanAny` | `scan.go` | `storage.ScanAny` |
| `Factory`, `Register`, `Open`, `registry` | `open.go` | **Eliminado, no trasladado** (ver §2) |
| Recorders (`mock.Executor`, etc.) | `mock/recorders.go` | `storage/mock` |
| Motor en memoria (`mock.NewDB`) | `mock/memdb.go` | `storage/mem` (`mem.New() storage.Conn`) |
| `conformance.Widget`, `conformance.Run` | `conformance/` | `storage/conformance` (reescrito sobre `Query` cruda, sin builder) |

Estos 9 archivos/paquetes **se borran** de `orm` en este plan: `executor.go`, `compiler.go`,
`query.go`, `conditions.go`, `execution_plan.go`, `scan.go`, `open.go`, `mock/`, `conformance/`.
`tx.go` se **recorta** (las interfaces se van, el método `Tx` se queda). `errors.go` se recorta
(`ErrNoRows` se va).

## 2. Qué se elimina del todo (no se traslada a ningún lado)

- **El registro DSN (`Register`/`Open`/`Factory`).** Un lookup por string que falla en runtime viola
  el harness. Se reemplaza por construcción explícita: `conn, err := sqlt.Open(dsn); d := orm.New(conn,
  err-handled)`. No repliques este mecanismo en `storage` ni en `orm` — si un consumidor lo echa de menos,
  es una fase posterior (consumidores, #7 del master), no este plan.

## 3. Qué se queda en `orm` (y por qué)

Todo lo que es **glue invariante escrito una vez** — no varía por backend, así que no es contrato,
es ergonomía (ver DB_PORT_PROPOSAL.md §6.8):

- `orm.DB`: `New(conn storage.Conn) *DB`, `Create`/`Update`/`Delete`/`Query(m) *QB`/`Close`/`RawConn`/
  `SetLog`.
- `orm.QB`/`Clause`/`OrderClause`: `Where/Or/Limit/Offset/OrderBy/GroupBy/ReadOne/ReadAll`.
- `orm.Tx`: el método `(d *DB) Tx(fn func(tx *DB) error) error`, con rollback automático.
- `orm.ErrNotFound`, `orm.ErrValidation`, `orm.ErrEmptyTable`, `orm.ErrNoTxSupport`: errores del
  concepto ergonómico ("no hubo fila para tu `ReadOne`", "tu modelo no cuadra"), no del contrato
  crudo.
- **Re-exports de conveniencia** (`Condition`, `Order`, `Eq`, `Neq`, `Gt`, `Gte`, `Lt`, `Lte`, `Like`,
  `In`, `Or`, `IsNotNull`, `Asc`, `Desc`) — ver §5.2. `Update`/`Delete` toman `storage.Condition` como
  argumento explícito (`Update(m, cond storage.Condition, ...)`); sin un re-export, cada consumidor
  tendría que importar `storage` **solo** para llamar `orm.Eq(...)` al hacer un `Update`/`Delete` — mala
  ergonomía y una fuga de la costura hacia el consumidor. Un `type Condition = storage.Condition` (alias,
  no un tipo nuevo) más `var Eq = storage.Eq` no duplica nada: es exactamente el mismo tipo/función con otro
  nombre, cero conversión, cero mantenimiento doble.

## 4. Diseño de archivos

### 4.1 `go.mod`

```
go get github.com/tinywasm/storage@v0.0.1
go mod tidy   # esto debe QUITAR cualquier dependencia que orm tuviera solo para el contrato
```

`go.mod` final: `github.com/tinywasm/storage`, `github.com/tinywasm/model`, `github.com/tinywasm/fmt`.

### 4.2 `db.go`

```go
package orm

import (
	"github.com/tinywasm/storage"
	"github.com/tinywasm/model"
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
```

> **Nota sobre `RawExecutor` → `RawConn`.** Es un rename deliberado, no un descuido: el campo
> subyacente ya no es un `Executor` suelto, es un `storage.Conn`. Si algún test interno todavía necesita
> "solo la mitad Executor", haz `d.RawConn()` (que ya satisface `storage.Executor` por composición) — no
> añadas un segundo accessor.
>
> **`Compiler()` accessor eliminado.** Ya no hay un `compiler` separado que exponer — está fundido en
> `conn`. Si algo lo necesitaba, usa `d.RawConn()` (satisface `storage.Compiler` también).

### 4.3 `tx.go`

```go
package orm

import "github.com/tinywasm/storage"

// Tx executes a function within a transaction. The underlying storage.Conn must implement
// storage.TxExecutor (type-asserted here) — most backends do; mem.New() also implements it as a
// no-op so tests can exercise this path without a real transactional backend.
func (d *DB) Tx(fn func(tx *DB) error) error {
	txExec, ok := d.conn.(storage.TxExecutor)
	if !ok {
		return ErrNoTxSupport
	}

	bound, err := txExec.BeginTx()
	if err != nil {
		return err
	}

	// bound is a storage.TxBoundExecutor: Executor + Commit/Rollback. It does NOT satisfy
	// storage.Compiler on its own, so txDB.conn wraps it back together with the original
	// compiler half via boundConn (below) — the same "conn = exec+compile" pairing New()
	// enforces, kept intact across a transaction boundary.
	txDB := &DB{conn: boundConn{TxBoundExecutor: bound, Compiler: d.conn}, log: d.log}

	if err := fn(txDB); err != nil {
		bound.Rollback()
		return err
	}
	return bound.Commit()
}

// boundConn re-pairs a transaction-bound Executor with the original connection's Compiler
// (compiling doesn't depend on being inside a transaction — only executing does), so the
// nested *DB handed to fn still satisfies storage.Conn as a single value.
type boundConn struct {
	storage.TxBoundExecutor
	storage.Compiler
}
```

> **Corrección de diseño respecto al `orm` viejo.** El `orm.DB` anterior guardaba `exec`+`compiler`
> como dos campos separados, así que re-envolver una transacción era trivial (`&DB{exec: bound,
> compiler: db.compiler}`). Ahora `DB` guarda un único `conn storage.Conn`, y `storage.TxBoundExecutor` (lo que
> devuelve `BeginTx`) **no** trae consigo un `Compiler` — normal, el compilador no cambia dentro de
> una transacción, solo el executor. `boundConn` (arriba) resuelve esto componiendo el
> `TxBoundExecutor` de la transacción con el `Compiler` de la conexión original en un solo valor que
> vuelve a satisfacer `storage.Conn`. Sin este tipo, `Tx` no compila con la firma `New(conn storage.Conn)` de un
> solo argumento — no lo omitas ni vuelvas a partir `DB` en dos campos para evitarlo.

### 4.4 `qb.go`

```go
package orm

import (
	"github.com/tinywasm/storage"
	"github.com/tinywasm/model"
)

// QB represents a query builder.
// Consumers hold a *QB reference in variables for incremental building.
type QB struct {
	db      *DB
	model   model.Model
	conds   []storage.Condition
	orderBy []storage.Order
	groupBy []string
	limit   int
	offset  int
	nextOr  bool
}

// Clause represents an intermediate state for building a query condition.
type Clause struct {
	qb    *QB
	field string
}

// Where starts a new condition clause for the given column.
func (qb *QB) Where(column string) *Clause {
	return &Clause{qb: qb, field: column}
}

// Or sets the next condition to use OR logic instead of AND.
func (qb *QB) Or() *QB {
	qb.nextOr = true
	return qb
}

func (qb *QB) addCondition(c storage.Condition) *QB {
	if qb.nextOr {
		c = storage.Or(c)
		qb.nextOr = false
	}
	qb.conds = append(qb.conds, c)
	return qb
}

func (c *Clause) Eq(value any) *QB    { return c.qb.addCondition(storage.Eq(c.field, value)) }
func (c *Clause) Neq(value any) *QB   { return c.qb.addCondition(storage.Neq(c.field, value)) }
func (c *Clause) Gt(value any) *QB    { return c.qb.addCondition(storage.Gt(c.field, value)) }
func (c *Clause) Gte(value any) *QB   { return c.qb.addCondition(storage.Gte(c.field, value)) }
func (c *Clause) Lt(value any) *QB    { return c.qb.addCondition(storage.Lt(c.field, value)) }
func (c *Clause) Lte(value any) *QB   { return c.qb.addCondition(storage.Lte(c.field, value)) }
func (c *Clause) Like(value any) *QB  { return c.qb.addCondition(storage.Like(c.field, value)) }
func (c *Clause) In(value any) *QB    { return c.qb.addCondition(storage.In(c.field, value)) }

// Limit sets the limit for the query.
func (qb *QB) Limit(limit int) *QB {
	qb.limit = limit
	return qb
}

// Offset sets the offset for the query.
func (qb *QB) Offset(offset int) *QB {
	qb.offset = offset
	return qb
}

// OrderClause represents an intermediate state for building an order by clause.
type OrderClause struct {
	qb    *QB
	field string
}

// OrderBy starts a new order clause for the given column.
func (qb *QB) OrderBy(column string) *OrderClause {
	return &OrderClause{qb: qb, field: column}
}

// Asc sets the order direction to ascending.
func (o *OrderClause) Asc() *QB {
	o.qb.orderBy = append(o.qb.orderBy, storage.Asc(o.field))
	return o.qb
}

// Desc sets the order direction to descending.
func (o *OrderClause) Desc() *QB {
	o.qb.orderBy = append(o.qb.orderBy, storage.Desc(o.field))
	return o.qb
}

// GroupBy adds a group by clause to the query.
func (qb *QB) GroupBy(columns ...string) *QB {
	qb.groupBy = append(qb.groupBy, columns...)
	return qb
}

// ReadOne executes the query and returns a single result.
func (qb *QB) ReadOne() error {
	if err := validateQuery(storage.ActionReadOne, qb.model); err != nil {
		return err
	}
	q := storage.Query{
		Action:     storage.ActionReadOne,
		Table:      qb.model.ModelName(),
		Conditions: qb.conds,
		OrderBy:    qb.orderBy,
		GroupBy:    qb.groupBy,
		Limit:      1, // Force limit 1
		Offset:     qb.offset,
	}
	plan, err := qb.db.conn.Compile(q, qb.model)
	if err != nil {
		return err
	}

	row := qb.db.conn.QueryRow(plan.Query, plan.Args...)
	if err := row.Scan(qb.model.Pointers()...); err != nil {
		if err == storage.ErrNoRows {
			return ErrNotFound
		}
		return err
	}
	return nil
}

// ReadAll executes the query and returns all results.
func (qb *QB) ReadAll(new func() model.Model, onRow func(model.Model)) error {
	if err := validateQuery(storage.ActionReadAll, qb.model); err != nil {
		return err
	}
	q := storage.Query{
		Action:     storage.ActionReadAll,
		Table:      qb.model.ModelName(),
		Conditions: qb.conds,
		OrderBy:    qb.orderBy,
		GroupBy:    qb.groupBy,
		Limit:      qb.limit,
		Offset:     qb.offset,
	}
	plan, err := qb.db.conn.Compile(q, qb.model)
	if err != nil {
		return err
	}

	rows, err := qb.db.conn.Query(plan.Query, plan.Args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		m := new()
		if err := rows.Scan(m.Pointers()...); err != nil {
			return err
		}
		onRow(m)
	}
	return rows.Err()
}
```

> **Cambio de comportamiento en `addCondition`.** Antes fijaba `c.logic = "AND"` explícitamente en la
> rama sin `Or()` porque `Eq`/`Gt`/etc. ya ponían `"AND"` por defecto — ese `else` era redundante (lo
> quité: `storage.Eq(...)` etc. ya devuelven `Logic()=="AND"`). La rama `Or()` ahora usa `storage.Or(c)` (la
> función pública) en vez de mutar un campo privado — `Condition` es un tipo de `storage`, `qb.go` ya no
> puede tocar sus campos no exportados directamente. Verifica con un test que el comportamiento
> observable no cambió (mismo resultado en `readAllOrsConditions`-style).

### 4.5 `validate.go`

```go
package orm

import (
	"github.com/tinywasm/storage"
	"github.com/tinywasm/fmt"
	"github.com/tinywasm/model"
)

func validateQuery(action storage.Action, m model.Model) error {
	if m.ModelName() == "" {
		return ErrEmptyTable
	}
	if action == storage.ActionCreate || action == storage.ActionUpdate {
		if len(m.Schema()) != len(m.Pointers()) {
			return fmt.Err(ErrValidation, "schema and pointers length mismatch")
		}
	}
	return nil
}
```

### 4.6 `errors.go`

```go
package orm

import "github.com/tinywasm/fmt"

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
```

### 4.7 `reexport.go` — **nuevo archivo**, los alias de conveniencia de §3

```go
package orm

import "github.com/tinywasm/storage"

// Re-exports of storage's DML value types and condition/order constructors, so that a consumer
// calling Update/Delete (which take a storage.Condition explicitly) doesn't need a second import
// just for Eq/Gt/etc. These are aliases, not new types/wrappers — orm.Condition IS storage.Condition,
// orm.Eq IS storage.Eq. Zero duplication, zero conversion. See docs/PLAN.md §3.
type Condition = storage.Condition
type Order = storage.Order

var (
	Eq        = storage.Eq
	Neq       = storage.Neq
	Gt        = storage.Gt
	Gte       = storage.Gte
	Lt        = storage.Lt
	Lte       = storage.Lte
	Like      = storage.Like
	In        = storage.In
	Or        = storage.Or
	IsNotNull = storage.IsNotNull
	Asc       = storage.Asc
	Desc      = storage.Desc
)
```

### 4.8 Archivos que se borran

```
executor.go
compiler.go
query.go
conditions.go
execution_plan.go
scan.go
open.go
mock/           (todo el directorio)
conformance/    (todo el directorio)
```

## 5. Tests — reescritura completa de `tests/`

`orm/tests/` importaba `github.com/tinywasm/orm/mock`. Ahora importa `github.com/tinywasm/storage/mock` (los
recorders) y, para el round-trip real, `github.com/tinywasm/storage/mem`.

### 5.1 `tests/core_test.go` — reescribe imports y tipos, quita lo que ya no es de `orm`

- Cambia `"github.com/tinywasm/orm/mock"` → `"github.com/tinywasm/storage/mock"`; `mock.Compiler`,
  `mock.Executor`, etc. ahora capturan `storage.Query`/`storage.Plan` (mismos nombres de campo,
  `mockCompiler.LastQuery.Action != storage.ActionCreate` en vez de `orm.ActionCreate`).
- **Quita** los subtests "13. Condition Helpers" y la mitad de "14. Getters" que prueba
  `Condition`/`Order` directamente (`c.Field()`, `c.Operator()`, `o.Column()`, `o.Dir()`) — esos tipos
  ya no son de `orm`, se prueban en `storage` (`storage/tests/storage_helpers_test.go`, ya existe). Deja solo la parte de "Getters" que
  ejercita el builder (`d.Query(model).OrderBy("col").Asc().ReadOne()` y verifica
  `mockCompiler.LastQuery.OrderBy`) — **esa** sí es comportamiento de `orm`.
- El resto (Create/Update/Delete/ReadOne/ReadAll/Tx/Errors/Close) se queda igual en estructura,
  solo con los tipos calificados a `storage.` donde correspondía a `orm.` antes (`storage.Eq(...)` en vez de
  `orm.Eq(...)` si usas los recorders directos de `storage/mock`; si usas los re-exports de §4.7,
  `orm.Eq(...)` sigue funcionando igual que antes — usa lo que ya usaba el test, no fuerces el cambio
  a `storage.` donde `orm.` re-exportado ya compila).

### 5.2 `tests/new_sync_test.go`

`TestOpen` (el único test que quedaba en este archivo tras el split DDL/DML) **se borra entero**: era
la prueba de `orm.Register`/`orm.Open`, eliminados en este plan (§2). Si el archivo queda vacío,
bórralo.

### 5.3 `tests/scan_test.go`

`TestScanAny` **se borra**: `ScanAny` ya no es de `orm`, se prueba en `storage` (`storage/tests/storage_helpers_test.go`, ya existe).
Si el archivo queda vacío, bórralo.

### 5.4 `tests/orm_stlib_test.go` / `tests/orm_wasm_test.go`

Sin cambios de fondo — siguen llamando `RunCoreTests(t)`.

### 5.5 **Nuevo — `tests/roundtrip_test.go`**: la prueba *consumer-shaped* que exige el harness

CONSTRUCTION_HARNESS.md §127: *"An API is not published until a consumer-shaped test, inside the
library itself, proves it."* `storage/mock` prueba que el builder arma la `Query` correcta; esto prueba que
la `Query` correcta, ejecutada de verdad contra un backend real (`storage/mem`), da el resultado correcto —
el camino completo que un consumidor real recorre.

```go
package tests

import (
	"testing"

	"github.com/tinywasm/storage/conformance" // reusa el fixture Widget, no lo dupliques
	"github.com/tinywasm/storage/mem"
	"github.com/tinywasm/orm"
)

func TestBuilderRoundTripAgainstMem(t *testing.T) {
	d := orm.New(mem.New())

	w := &conformance.Widget{Id: "w1", Name: "alpha", Qty: 3, Active: true}
	if err := d.Create(w); err != nil {
		t.Fatalf("Create: %v", err)
	}

	var got conformance.Widget
	if err := d.Query(&got).Where("id").Eq("w1").ReadOne(); err != nil {
		t.Fatalf("ReadOne: %v", err)
	}
	if got.Name != "alpha" || got.Qty != 3 || !got.Active {
		t.Errorf("round-trip mismatch: got %+v", got)
	}

	if err := d.Update(&conformance.Widget{Name: "beta", Qty: 9, Active: false}, orm.Eq("id", "w1")); err != nil {
		t.Fatalf("Update: %v", err)
	}
	var updated conformance.Widget
	if err := d.Query(&updated).Where("id").Eq("w1").ReadOne(); err != nil {
		t.Fatalf("ReadOne after update: %v", err)
	}
	if updated.Name != "beta" || updated.Qty != 9 || updated.Active {
		t.Errorf("update mismatch: got %+v", updated)
	}

	if err := d.Delete(&conformance.Widget{}, orm.Eq("id", "w1")); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	err := d.Query(&conformance.Widget{}).Where("id").Eq("w1").ReadOne()
	if err == nil {
		t.Fatal("expected ErrNotFound after delete")
	}
	if err != orm.ErrNotFound {
		t.Errorf("expected orm.ErrNotFound, got %v", err)
	}

	// ReadAll + Where + OrderBy + Limit, para cubrir la otra mitad del builder.
	_ = d.Create(&conformance.Widget{Id: "a", Name: "x", Qty: 1, Active: true})
	_ = d.Create(&conformance.Widget{Id: "b", Name: "x", Qty: 2, Active: true})
	var all []*conformance.Widget
	err = d.Query(&conformance.Widget{}).Where("name").Eq("x").OrderBy("qty").Desc().Limit(1).
		ReadAll(func() any { return &conformance.Widget{} }, nil) // ajusta la firma real de ReadAll (model.Model, no any)
	// ^ NOTA: pseudo-código de forma — usa la firma real de QB.ReadAll (func() model.Model, func(model.Model))
	// tal como está en qb.go §4.4. No copies este bloque literal sin corregirlo.
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(all) != 1 || all[0].Id != "b" {
		t.Errorf("expected only b (qty desc, limit 1); got %+v", all)
	}
}
```

> El último bloque (`ReadAll`) está deliberadamente marcado como pseudo-código de forma — la firma
> real de `QB.ReadAll` es `func(new func() model.Model, onRow func(model.Model)) error` (ver §4.4).
> Escríbelo con esa firma exacta, acumulando en `all` dentro del callback `onRow`, igual que ya hace
> `readAll` en `storage/conformance` (`storage/conformance/conformance.go`) — es el mismo patrón, no lo reinventes.

## 6. `docs/ARQUITECTURE.md` — actualízalo, no lo dupliques

Reemplaza la sección "DML/DDL Split (2026-07-16)" (la que dice que `orm` sigue teniendo
`Executor`/`Compiler`/etc.) por una nueva sección corta:

```markdown
## Puerto de almacenamiento (2026-07-16, segunda pasada)

`orm` ya no define el contrato de almacenamiento. `tinywasm/storage` es el puerto (interfaces, tipos de
valor DML, conformance, mock, mem) — el equivalente de `database/sql/driver`. `orm` es la capa
ergonómica opcional encima (`orm.DB`, query builder), el equivalente de `database/sql`. Un backend
implementa `storage.Conn`; nunca importa `orm`. Ver `tinywasm/storage`'s AGENTS.md y
[DB_PORT_PROPOSAL.md](https://github.com/tinywasm/app-releases/blob/main/docs/DB_PORT_PROPOSAL.md).
```

## 7. `AGENTS.md` — actualízalo

- Sección "Mission of this package": ya no dice "runtime DML ORM" con contrato propio — di "capa
  ergonómica opcional sobre `tinywasm/storage`".
- Sección "No Go `map` anywhere in this ecosystem": la regla se hereda de `storage` (que es más estricto
  todavía, por ser isomórfico puro) — mantenla, pero corrige los ejemplos (`mock/memdb.go` ya no
  existe en este repo, ahora vive en `storage/mem`).
- Tabla "Code layout": quita las filas de `conformance/`, `mock/`, `open.go`, `executor.go`,
  `compiler.go`, `query.go`, `conditions.go`, `execution_plan.go`, `scan.go`. Añade una nota: "El
  contrato vive en `github.com/tinywasm/storage` — este repo no lo redefine."

## 8. Criterios de aceptación

- `orm.DB` tiene `New(conn storage.Conn) *DB`, `Create`/`Update`/`Delete`/`Query`/`Close`/`RawConn`/`Tx`/
  `SetLog`. **No** tiene `Executor`/`Compiler`/`Query`/`Condition`/`Order`/`Plan`/`ErrNoRows`/
  `ScanAny`/`Register`/`Open`/`Factory` propios — todos vienen de `storage` (algunos re-exportados, §4.7).
  `go.mod` no depende de nada más que `storage`+`model`+`fmt`.
- `tests/` compila e importa `github.com/tinywasm/storage/mock` (recorders) y
  `github.com/tinywasm/storage/mem` (round-trip real) — cero referencias a `orm/mock` o `orm/conformance`
  (ya no existen).
- `tests/roundtrip_test.go` existe y ejercita `Create`/`ReadOne`/`Update`/`Delete`/`ReadAll` contra
  `mem.New()` de principio a fin (la prueba *consumer-shaped* del harness).
- `gotest` verde en todo el módulo (incluye `gotest -tinygo`); `GOOS=js GOARCH=wasm go build ./...`
  limpio.
- `docs/ARQUITECTURE.md` y `AGENTS.md` actualizados (§6, §7).
- Publicado con `gopush` (breaking, minor bump — `v0.11.0` o el que corresponda tras el actual).

## 9. Etapas

| # | Etapa | Archivo(s) | Criterio |
|---|---|---|---|
| 1 | Dependencia | `go.mod` | `storage@v0.0.1` añadido, `go mod tidy` limpio |
| 2 | `DB` + `Tx` sobre `storage.Conn` | `db.go`, `tx.go` | `New(conn storage.Conn)`, `boundConn` (§4.3) |
| 3 | Query builder | `qb.go` | tipos calificados a `storage.`, `Or` vía `storage.Or` (§4.4) |
| 4 | Validación + errores | `validate.go`, `errors.go` | `ErrNoRows` fuera, resto igual |
| 5 | Re-exports | `reexport.go` (nuevo) | `Condition`/`Order`/`Eq`/.../`Desc` (§4.7) |
| 6 | Borrar contrato viejo | (§4.8) | `executor.go`…`conformance/` fuera del repo |
| 7 | Tests | `tests/*.go` | §5 completo, incl. `roundtrip_test.go` |
| 8 | Docs | `docs/ARQUITECTURE.md`, `AGENTS.md` | §6, §7 |
| 9 | Verificar + publicar | — | `gotest`+`gotest -tinygo` verdes; `gopush` |

## 10. Cierre

Tras `gopush`, **borra** `docs/PLAN.md`; el diseño duradero ya vive en `docs/ARQUITECTURE.md` (§6 lo
actualiza in situ, no hace falta un traslado adicional).
