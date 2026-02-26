# ORM Architecture

The `tinywasm/orm` package is an ultra-lightweight, strongly-typed, zero-magic (no `reflect`), and isomorphic (WASM/Backend) ORM, strictly following the architectural mandates of the `tinywasm` ecosystem.

## 1. Primary Architectural Pattern: Dependency Injection and Explicit Models

Unlike traditional ORMs (GORM, ObjectDB) that use `reflect` to infer tables and columns at runtime, `tinywasm/orm` requires **Entities (Structs)** to implement an explicit interface (`orm.Model`). This ensures:

1. **O(1) Performance in WASM:** Reflective processing overhead is eliminated.
2. **Reduced Binary Size:** Importing `reflect` inflates the WASM binary.
3. **Strict Type Safety:** The compiler detects errors before runtime.

The ORM acts as an agnostic orchestrator. It is unaware of whether the destination is Postgres, SQLite, or IndexedDB. It delegates the actual execution to an **Adapter** injected from `cmd/main.go`.

---

## 2. Flowchart (The Lifecycle of a Query)

[The Lifecycle of a Query: ORM_FLOW.md](diagrams/ORM_FLOW.md)

---

## 3. Fundamental Components

### 3.1. `Model` Interface (User Entity)

The application developer must make their structs implement this interface. The ORM extracts everything it needs to build a `Query` from here.

```go
type Model interface {
    // TableName returns the physical name of the table or store.
    TableName() string
    // Columns returns the ordered list of column names.
    Columns() []string
    // Values returns the entity values in the same order as Columns(). Used in Create/Update.
    Values() []any
    // Pointers returns pointers to the entity fields to inject data during a Read.
    Pointers() []any
}
```

> `Values()` and `Pointers()` are only called by the Adapter for the operations that require them. The Adapter must not call `Values()` on a DELETE, nor `Pointers()` on a CREATE.

---

### 3.2. Agnostic Structures

#### `Action` (Typed Constant)

```go
type Action int

const (
    ActionCreate  Action = iota
    ActionReadOne
    ActionUpdate
    ActionDelete
    ActionReadAll
)
```

Using a typed constant (not a raw string) ensures the compiler catches any invalid action at compile time.

#### `Condition` (Filter)

A sealed value type. Consumers **must** construct via helpers (`Eq`, `Gt`, `Or`, etc.) — direct struct literal construction from outside the package is a compile error due to unexported fields. Adapters read values via getter methods.

```go
type Condition struct {
    field    string
    operator string  // "=", "!=", ">", ">=", "<", "<=", "LIKE"
    value    any
    logic    string  // "AND" (default) | "OR" — applies between this and the NEXT condition
}

func (c Condition) Field() string    { return c.field }
func (c Condition) Operator() string { return c.operator }
func (c Condition) Value() any       { return c.value }
func (c Condition) Logic() string    { return c.logic }
```

#### `Order` (Sorting)

A sealed value type. Constructed **only** internally by `QB.OrderBy()` — never by consumers or adapters directly. Adapters read values via getter methods.

```go
type Order struct {
    column string
    dir    string  // "ASC" | "DESC"
}

func (o Order) Column() string { return o.column }
func (o Order) Dir() string    { return o.dir }
```

#### `Query` (Agnostic Request)

The central struct passed from the ORM core to the Adapter. Contains everything the Adapter needs to translate into a native operation.

```go
type Query struct {
    Action     Action
    Table      string
    Columns    []string
    Values     []any
    Conditions []Condition
    OrderBy    []Order
    GroupBy    []string
    Limit      int
    Offset     int
}
```

> There is no `Args []any` field. All condition values live inside `Conditions[].Value` to avoid redundancy.

---

### 3.3. `Adapter` Interface (The Injection)

The interface that Postgres, SQLite, or IndexedDB adapters must implement. Responsible for translating a `Query` into a native language and executing it.

```go
type Adapter interface {
    // Execute handles all CRUD operations with a single method.
    // For single-row reads (ActionRead):   factory and each are nil; m is mutated via Pointers().
    // For multi-row reads (ActionReadAll): m is nil; factory creates each instance; each receives it.
    // For write operations:                factory and each are nil; m provides TableName/Columns/Values.
    Execute(q Query, m Model, factory func() Model, each func(Model)) error
}
```

This unified signature avoids splitting the interface into multiple methods while making the contract explicit via documentation.

---

### 3.4. Transaction Interfaces (Optional Extension)

Not all database engines support transactions (e.g., some IndexedDB patterns). Transaction support is **optional**: adapters implement these interfaces only if their engine supports atomic operations.

```go
// TxBound is an Adapter that is already bound to an active transaction.
// Returned by TxAdapter.BeginTx(). Carries Commit and Rollback alongside Execute.
type TxBound interface {
    Adapter           // Execute operates within the active transaction
    Commit() error
    Rollback() error
}

// TxAdapter is an optional extension for adapters that support transactions.
// If the underlying adapter does not implement TxAdapter, DB.Tx() returns ErrNoTxSupport.
type TxAdapter interface {
    BeginTx() (TxBound, error)
}
```

**Why two interfaces instead of one?**
`TxAdapter` is the **capability check** (does this adapter support transactions?). `TxBound` is the **active handle** returned after `BEGIN` — it implements `Adapter` so the same `DB` methods (`Create`, `Update`, `Delete`, `Query`) work transparently inside a transaction, plus `Commit`/`Rollback` to control the boundary.

---

### 3.5. The Core of the ORM (Public API)

The `DB` struct is instantiated from `cmd/main.go` and injected into handler/logic layers.

#### Write Operations (Direct — no builder)

```go
type DB struct { adapter Adapter }

func New(adapter Adapter) *DB

func (db *DB) Create(m Model) error
func (db *DB) Update(m Model, conds ...Condition) error
func (db *DB) Delete(m Model, conds ...Condition) error

// Tx executes fn inside an atomic transaction.
// On fn error → automatic Rollback. On nil → automatic Commit.
// Returns ErrNoTxSupport if the adapter does not implement TxAdapter.
func (db *DB) Tx(fn func(tx *DB) error) error
```

#### Read Operations (Builder/Chain)

```go
// Query returns a QueryBuilder scoped to the given model.
func (db *DB) Query(m Model) *QB

type QB struct {
    db      *DB
    model   Model
    conds   []Condition
    orderBy []Order
    groupBy []string
    limit   int
    offset  int
}

func (q *QB) Where(conds ...Condition) *QB
func (q *QB) Limit(n int) *QB
func (q *QB) Offset(n int) *QB
func (q *QB) OrderBy(col, dir string) *QB
func (q *QB) GroupBy(cols ...string) *QB

// ReadOne executes the query and fills m (passed to db.Query) via Model.Pointers().
// Returns orm.ErrNotFound if no row matches.
func (q *QB) ReadOne() error

// ReadAll executes the query; for each row it calls factory() to get a fresh Model,
// scans into its Pointers(), then calls each(m). The caller owns accumulation.
func (q *QB) ReadAll(factory func() Model, each func(Model)) error
```

#### Condition Helpers

```go
func Eq(field string, val any) Condition   // field = val
func Neq(field string, val any) Condition  // field != val
func Gt(field string, val any) Condition   // field > val
func Gte(field string, val any) Condition  // field >= val
func Lt(field string, val any) Condition   // field < val
func Lte(field string, val any) Condition  // field <= val
func Like(field string, val any) Condition // field LIKE val
func Or(c Condition) Condition             // wraps c with Logic = "OR"
```

---

### 3.6. Sentinel Errors

```go
var (
    ErrNotFound     = errors.New("orm: record not found")
    ErrValidation   = errors.New("orm: model validation failed")
    ErrEmptyTable   = errors.New("orm: model returned empty table name")
    ErrNoTxSupport  = errors.New("orm: adapter does not support transactions")
)
```

Callers use `errors.Is(err, orm.ErrNotFound)` to branch on error type without string parsing.

---

## 4. Advantages of this Design

1. **Fully Stdlib & WASM Compatible:** `tinywasm/orm` core does not import `database/sql` nor interact with the OS or Network.
2. **Separation of Concerns:** The ORM packs/unpacks data (Model → Query). Adapters (external repos like `tinywasm/postgre` or `tinywasm/sqlite`) only know how to execute (Query → DB Engine).
3. **Zero-Alloc ReadMany:** The `Many(factory, each)` push-based pattern avoids internal slice management; the caller decides whether to accumulate, stream, or discard rows.
4. **Type-Safe Actions:** `Action int` constants prevent adapter logic errors from typos at compile time.
5. **Composable Queries:** The `QB` builder allows incremental construction of complex queries in handler logic (conditional `Where`, dynamic `OrderBy`) without string concatenation.
6. **Testable without Real Databases:** A `MockAdapter` (stores the received `Query` in memory) can validate all business logic without touching disks or ports.
7. **Optional Transactions:** The `TxAdapter`/`TxBound` pattern allows adapters to opt-in to transaction support without modifying the core `Adapter` interface. Adapters that don't support transactions (e.g., simple IndexedDB patterns) are unaffected.
