# Implementation Plan: Schema Definition (DDL) in ORM

## Development Rules
- **WASM Environment (`tinywasm`):** Use `tinywasm/fmt`, `tinywasm/time`. **NO `time.Time`** — use `int64`. TinyGo cannot use the standard `time` package due to binary size.
- **Single Responsibility Principle (SRP):** Every file must have a single, well-defined purpose.
- **Mandatory Dependency Injection (DI):** No global state. Interfaces for external dependencies.
- **No `map` in WASM code:** Maps cause binary bloat in TinyGo. Use `[]struct` instead.
- **Testing Runner (`gotest`):** ALWAYS use the globally installed `gotest` CLI command.
- **Standard Library Only in Tests:** NEVER use external assertion libraries.

## Goal
Add abstract schema definition routing (DDL) to `tinywasm/orm` using a `FieldType` iota and a `Field` struct. The `Model` interface replaces `Columns() []string` with `Schema() []Field`. Each adapter handles its own DDL. FK support via `Field.Ref` and `Field.RefColumn`. String PKs use `github.com/tinywasm/unixid` (caller's responsibility). This is a **breaking change** — separate update plans per adapter after ORM is published.

## Decisions Table

| Topic | Decision |
|---|---|
| Type representation | `FieldType` iota (see FieldType Mapping Table) |
| Unsupported Go types | Skip silently with a logged warning. Test asserts field absent in `Schema()` output |
| Struct tag name | `db:"..."` |
| Tag purpose | Constraints only. Never for renaming columns |
| PK auto-detection | `fmt.IDorPrimaryKey(tableName, fieldName)` → `isPK=true` → auto `ConstraintPK`. `db:"pk"` only for non-conventional names |
| `time.Time` | **Fatal error** in `ormc`. Use `int64` + `tinywasm/time` |
| String PKs | Caller sets ID via `github.com/tinywasm/unixid` before `db.Create()`. ORM does not generate IDs. Note added in generated file |
| `ConstraintAutoIncrement` | Only for numeric fields (`TypeInt64`, `TypeFloat64`). `ormc` emits fatal error if applied to `TypeText` |
| FK support | `Field.Ref string` = target table name. `Field.RefColumn string` = target column (empty = autodetect PK of ref table). SQL: `CONSTRAINT fk_table_col FOREIGN KEY (col) REFERENCES ref(refcol)`. IndexedDB: **ignores** Ref (field exists for API compatibility, no error) |
| `ActionCreateDatabase` | Included. Uses `Query.Database string`. In `db.CreateDatabase(name)`, `m` passed to `Compiler.Compile()` must be a zero-value sentinel (empty struct implementing Model) — adapter ignores `m`, uses `q.Database` |
| `ormc` scan mode | **Automatic recursive scan** from working directory. Finds `model.go` or `models.go` in any subdirectory (excludes `vendor/`, `.git/`, `testdata/`). Generates `model_orm.go` next to each found file. Fatal error if none found. No `-struct` flag |
| `Values()` / `Schema()` alignment | Guaranteed by `ormc`. Comment in generated file. Manual models: developer's responsibility |
| `DropTable` / `CreateTable` model arg | Always pass a non-nil model (even `&User{}`). Compiler uses `m.TableName()` and `m.Schema()` |
| `Query.Columns` population | `db.go` fills `q.Columns` by iterating `m.Schema()` and extracting `f.Name` before calling `compiler.Compile()`. Field still exists in Query for DML use |
| `ormc` tests | Use `GenerateCodeForStruct()` directly (public func), not the CLI, to allow standard Go unit tests |
| Breaking change scope | Separate `CHECK_PLAN.md` per adapter after ORM is published |

## FieldType Mapping Table

| Go Type | FieldType | Notes |
|---|---|---|
| `string` | `TypeText` | |
| `int`, `int32`, `int64` | `TypeInt64` | All map to 64-bit signed storage |
| `uint`, `uint32`, `uint64` | `TypeInt64` | Stored as signed 64-bit; adapter responsible for range constraints |
| `float32`, `float64` | `TypeFloat64` | All map to 64-bit storage |
| `bool` | `TypeBool` | |
| `[]byte` | `TypeBlob` | |
| `time.Time` | **fatal error** | "time.Time not allowed, use int64 with tinywasm/time" |
| Any other type | **skip + warning** | Field omitted from Schema() |

## Files to Modify / Create

### 1. `field_type.go` (New File)
```go
package orm

// FieldType represents the abstract storage type of a model field.
type FieldType int

const (
    TypeText    FieldType = iota
    TypeInt64
    TypeFloat64
    TypeBool
    TypeBlob
)

// Constraint is a bitmask of column-level constraints.
// ConstraintNone = 0 is defined separately to avoid shifting iota off-by-one.
type Constraint int

const ConstraintNone Constraint = 0

const (
    ConstraintPK            Constraint = 1 << iota // 1: Primary Key (auto-detected via fmt.IDorPrimaryKey)
    ConstraintUnique                                // 2: UNIQUE
    ConstraintNotNull                               // 4: NOT NULL
    ConstraintAutoIncrement                         // 8: SERIAL / AUTOINCREMENT / {autoIncrement: true}
)

// Field describes a single column in a model's schema.
// Schema() and Values() MUST always be in the same field order.
// Field.Ref is present in all adapters for API compatibility; adapters that
// do not support FKs (e.g. IndexedDB) silently ignore it without error.
type Field struct {
    Name        string
    Type        FieldType
    Constraints Constraint
    Ref         string // FK: target table name. Empty = no FK.
    RefColumn   string // FK: target column. Empty = auto-detect PK of Ref table.
}
```

### 2. `model.go`
```go
type Model interface {
    TableName() string
    Schema() []Field   // Replaces Columns() []string
    Values() []any
    Pointers() []any
}
```

### 3. `query.go`
```go
const (
    ActionCreate Action = iota
    ActionReadOne
    ActionUpdate
    ActionDelete
    ActionReadAll
    ActionCreateTable    // NEW
    ActionDropTable      // NEW
    ActionCreateDatabase // NEW
)

type Query struct {
    Action     Action
    Table      string
    Database   string     // NEW: used by ActionCreateDatabase
    Columns    []string   // filled by db.go from m.Schema() before Compile()
    Values     []any
    Conditions []Condition
    OrderBy    []Order
    GroupBy    []string
    Limit      int
    Offset     int
}
```

### 4. `db.go`
- In `Create` and `Update`: replace `.Columns()` call with a loop over `m.Schema()` extracting `f.Name` into `q.Columns`.
- Add DDL methods:
```go
func (db *DB) CreateTable(m Model) error
func (db *DB) DropTable(m Model) error
func (db *DB) CreateDatabase(name string) error  // passes emptyModel{} as m
```
- `emptyModel` is a private zero-value type in `db.go` implementing `Model` with no-op methods, used only as the `m` argument for `CreateDatabase`.

### 5. `ormc.go` (Code Generator Updates)
- **Scan mode:** Recursively walk from the working directory. Skip `vendor/`, `.git/`, `testdata/`. For each `model.go` or `models.go` found, generate `model_orm.go` in the same directory (always overwrite). Fatal error if no model files found anywhere.
- **Invocation:** `ormc` CLI (`go install github.com/tinywasm/orm/cmd/ormc@latest`), run from project root:
  ```
  project/
    modules/
      user/model.go      → generates modules/user/model_orm.go
      product/models.go  → generates modules/product/model_orm.go
  ```
- **Type → FieldType:** per mapping table (includes `uint*` → `TypeInt64`).
- **Constraint resolution per field:**
  1. Auto-detect PK via `fmt.IDorPrimaryKey(tableName, fieldName)`.
  2. Parse `db:"..."` tag: `pk`, `unique`, `not_null`, `autoincrement`, `ref=table`, `ref=table:column`. Multiple: `db:"unique,not_null"`.
  3. Fatal error: `db:"autoincrement"` on `TypeText` field.
  4. Fatal error: `time.Time` field type.
  5. Warning + skip: any other unsupported type.
- **Generated file includes all Model interface methods:** `Schema()`, `Values()`, `Pointers()`, `TableName()`, plus `Meta` struct and typed `ReadOne`/`ReadAll` helpers.
- **Note in generated file** for string PKs: "String PK: set via github.com/tinywasm/unixid before calling db.Create()."

**Full example** — Given:
```go
type Order struct {
    IDOrder   string
    Total     float64 `db:"not_null"`
    UserID    string  `db:"ref=users"`
    CreatedAt int64
}
```
**Generates `order_orm.go`:**
```go
// Code generated by ormc; DO NOT EDIT.
// NOTE: Schema() and Values() must always be in the same field order.
// String PK: set via github.com/tinywasm/unixid before calling db.Create().
package yourpackage

import "github.com/tinywasm/orm"

func (m *Order) TableName() string { return "orders" }

func (m *Order) Schema() []orm.Field {
    return []orm.Field{
        {Name: "id_order", Type: orm.TypeText, Constraints: orm.ConstraintPK},
        {Name: "total", Type: orm.TypeFloat64, Constraints: orm.ConstraintNotNull},
        {Name: "user_id", Type: orm.TypeText, Constraints: orm.ConstraintNone, Ref: "users"},
        {Name: "created_at", Type: orm.TypeInt64},
    }
}

func (m *Order) Values() []any {
    return []any{m.IDOrder, m.Total, m.UserID, m.CreatedAt}
}

func (m *Order) Pointers() []any {
    return []any{&m.IDOrder, &m.Total, &m.UserID, &m.CreatedAt}
}

var OrderMeta = struct {
    TableName string
    IDOrder   string
    Total     string
    UserID    string
    CreatedAt string
}{
    TableName: "orders",
    IDOrder:   "id_order",
    Total:     "total",
    UserID:    "user_id",
    CreatedAt: "created_at",
}

func ReadOneOrder(qb *orm.QB, model *Order) (*Order, error) { ... }
func ReadAllOrder(qb *orm.QB) ([]*Order, error) { ... }
```

## Verification Plan

### Run: `gotest` inside `tinywasm/orm`
- Update all mock models in `tests/` to implement `Schema() []orm.Field`.
- `CreateTable` / `DropTable` / `CreateDatabase` routing tests (mock compiler intercepts the Action).
- `ormc` tests **via `GenerateCodeForStruct()` directly** (not CLI), covering:
  - `time.Time` field → fatal
  - `chan int` → skipped, field absent in output
  - `int32` → `TypeInt64`
  - `uint64` → `TypeInt64`
  - `float32` → `TypeFloat64`
  - `db:"not_null"` → `ConstraintNotNull`
  - `db:"autoincrement"` on string field → fatal
  - `db:"ref=orders"` → `Field.Ref = "orders"`, `Field.RefColumn = ""`
  - `db:"ref=orders:order_id"` → `Field.Ref = "orders"`, `Field.RefColumn = "order_id"`
  - PK auto-detection via `IDorPrimaryKey` → `ConstraintPK` without tag
  - Bitmask correctness: `ConstraintPK | ConstraintNotNull` = `5`
  - `emptyModel{}` passed to `CreateDatabase` Compiler → no panic
- Coverage ≥ 90%.

### Adapter Update Plans (Separate, After ORM Publish)
- `sqlite` — `FieldType` → SQLite types; `AUTOINCREMENT`; FK `REFERENCES`; `vendor/`-excluded scan
- `postgres` — `FieldType` → PG types; `SERIAL`/`BIGSERIAL`; `CONSTRAINT fk_...`
- `indexdb` — `ConstraintPK` → `keyPath`; `ConstraintAutoIncrement` → `{autoIncrement: true}`; `ConstraintUnique` → `{unique: true}`; `Field.Ref` silently ignored
