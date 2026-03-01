# ORM Skill

## Installation

```bash
go get github.com/tinywasm/orm
go install github.com/tinywasm/orm/cmd/ormc@latest
```

## Public API Contract

### Interfaces
- `Model`: `TableName()`, `Schema()`, `Values()`, `Pointers()` *(auto-implemented by `ormc`)*
- `Compiler`: `Compile(Query, Model) (Plan, error)`
- `Executor`: `Exec()`, `QueryRow()`, `Query()`, `Close()`
- `TxExecutor`: `BeginTx()`
- `TxBoundExecutor`: Embeds `Executor`, `Commit()`, `Rollback()`

### Model Interface
`Columns() []string` has been **replaced** by `Schema() []orm.Field`:
```go
type Model interface {
    TableName() string
    Schema() []orm.Field   // Key: column name, includes type + constraints
    Values() []any
    Pointers() []any
}
```

### Schema Field Types (`orm.FieldType`)
| Go Type | FieldType |
|---|---|
| `string` | `TypeText` |
| `int`, `int32`, `int64` | `TypeInt64` |
| `float32`, `float64` | `TypeFloat64` |
| `bool` | `TypeBool` |
| `[]byte` | `TypeBlob` |
| `time.Time` | ❌ **not allowed** — use `int64` + `tinywasm/time` |

### Schema Constraints (`orm.Constraint`, bitmask)
| Constant | db tag | Notes |
|---|---|---|
| `ConstraintPK` | `db:"pk"` | Auto-detected via `tinywasm/fmt.IDorPrimaryKey` |
| `ConstraintUnique` | `db:"unique"` | |
| `ConstraintNotNull` | `db:"not_null"` | |
| `ConstraintAutoIncrement` | `db:"autoincrement"` | Numeric fields only |
| FK reference | `db:"ref=table"` or `db:"ref=table:column"` | `Field.Ref` + `Field.RefColumn` |

> **String PKs:** must be set by caller via `github.com/tinywasm/unixid` before calling `db.Create()`. The ORM does not generate IDs.

### Auto-Generated Code (`cmd/ormc`)

Run `ormc` from the **project root**. It recursively scans all subdirectories looking for `model.go` or `models.go`, and generates `model_orm.go` **next to each source file found** (always overwritten). Fatal error if no model files are found anywhere.

```bash
ormc
```

Typical project structure:
```
project/
  modules/
    user/model.go      → generates modules/user/model_orm.go
    product/models.go  → generates modules/product/model_orm.go
```

Or trigger via `//go:generate` at the project root:
```go
//go:generate ormc
```

For a `struct User`, `ormc` generates in `model_orm.go`:
- `func (m *User) Schema() []orm.Field`
- `func (m *User) Values() []any`
- `func (m *User) Pointers() []any`
- `func (m *User) TableName() string`
- `UserMeta` struct with typed column name constants
- `ReadOneUser(qb *orm.QB, model *User) (*User, error)`
- `ReadAllUser(qb *orm.QB) ([]*User, error)`

### Core Structs
- `DB`: `New(Executor, Compiler)`, `Create`, `Update`, `Delete`, `Query`, `Tx`, `Close`, `RawExecutor`, `CreateTable`, `DropTable`, `CreateDatabase`
- `QB` (Fluent API): `Where("col")`, `Limit(n)`, `Offset(n)`, `OrderBy("col")`, `GroupBy("cols...")`
- `Clause` (Chainable): `.Eq()`, `.Neq()`, `.Gt()`, `.Gte()`, `.Lt()`, `.Lte()`, `.Like()`, `.In()`
- `OrderClause` (Chainable): `.Asc()`, `.Desc()`
- `Plan`: `Mode`, `Query`, `Args`

### Constants
- `Action`: `ActionCreate`, `ActionReadOne`, `ActionUpdate`, `ActionDelete`, `ActionReadAll`, `ActionCreateTable`, `ActionDropTable`, `ActionCreateDatabase`

## Usage Snippet

```go
// 1. Where clauses use generated Meta descriptors (no magic strings)
// 2. Query builder uses a Fluent API chain
// 3. Results are executed and cast by auto-generated typed functions
qb := db.Query(m).
    Where(UserMeta.Age).Eq(18).
    Or().Where(UserMeta.Name).Like("A%").
    OrderBy(UserMeta.CreatedAt).Desc().
    Limit(10)

users, err := models.ReadAllUser(qb)

// Schema DDL (adapter handles IF NOT EXISTS / IF EXISTS internally)
if err := db.CreateTable(&User{}); err != nil { ... }
if err := db.DropTable(&User{}); err != nil { ... }
if err := db.CreateDatabase("myapp"); err != nil { ... }
```
