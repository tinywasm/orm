# ORM Architecture

The `tinywasm/orm` package is an ultra-lightweight, strongly-typed, zero-magic (no `reflect`), and isomorphic (WASM/Backend) ORM, strictly following the architectural mandates of the `tinywasm` ecosystem.

## Background

- [Why use this ORM?](WHY.md)
- [Why package-level schema variables?](WHY_PACKAGE_LEVEL_SCHEMA.md)
- [Why ormc generates so much code (and why it's free)](WHY_GENERATED_CODE_IS_FREE.md)


## 1. Primary Architectural Pattern: Inverted Declarative Workflow

Unlike traditional ORMs that use `reflect` or parse structs with tags to generate metadata, `tinywasm/orm` inverts the flow. The developer writes a **typed definition** (`model.Definition`), and the generator (`ormc`) produces the **concrete Go struct** and all required interface implementations.

1. **Zero Runtime Reflection:** All metadata is available at compile-time.
2. **O(1) Performance in WASM:** No reflective processing overhead.
3. **Strict Type Safety:** Typos in field names or types are caught by the compiler because definitions use real Go symbols.
4. **Isomorphic by Design:** The same generated code works in Go (backend) and WASM (frontend).

---

## 2. Fundamental Components

### 3.1. `model.Definition` (Source of Truth)

The application developer defines models as package-level variables ending in `Model`.

```go
var UserModel = model.Definition{
    Name: "user",
    Fields: model.Fields{
        {Name: "id", Type: model.FieldInt, DB: &model.FieldDB{PK: true}},
        {Name: "name", Type: model.FieldText, NotNull: true},
    },
}
```

### 3.2. `orm.Model` Interface

The generated structs implement this interface, which embeds `fmt.Fielder` and adds `ModelName()`.

```go
type Model interface {
    fmt.Fielder
    ModelName() string
}

// fmt.Fielder provides:
type Fielder interface {
    Schema() []model.Field   // column metadata
    Pointers() []any         // field pointers for DB scanning
}
```

### 3.3. Typed Serialization Codec (`Encodable`/`Decodable`)

`ormc` generates reflection-free, 0-allocation methods for serialization (used by `tinywasm/json`):

```go
func (m *User) EncodeFields(w model.FieldWriter) {
    w.Int("id", int64(m.ID))
    w.String("name", m.Name)
}

func (m *User) DecodeFields(r model.FieldReader) {
    if v, ok := r.Int("id"); ok { m.ID = v }
    if v, ok := r.String("name"); ok { m.Name = v }
}
```

---

## 4. `ormc` Code Generator

`ormc` parses `model.go` / `models.go` files and generates `*_orm.go`.

### 4.1. Role Inference

`ormc` infers the role of each definition based on the presence of `DB` metadata or `Widget` bindings:

- **DB Model**: If at least one field has a `DB` configuration. Generates CRUD helpers (`ReadOne`, `ReadAll`) and a `*List` type.
- **Form**: If fields have `Widget` or validation rules. Generates a `Validate()` method.
- **DTO**: If it's just a data structure. Generates the struct and basic codec.

### 4.2. Relations

1.  **Composition**: When `Type` is `FieldStruct` or `FieldStructSlice`. The generated struct will embed the referenced type (or a slice of it).
2.  **Scalar Foreign Key**: When `Type` is a scalar and `Ref` is non-nil. This generates `SchemaExt()` metadata for DDL constraints.

### 4.3. Module Scanning & Schema Sync

`ormc` provides `ScanModules(rootDir string)` for centralized startup schema reconciliation across the entire module graph.

- **Writable Modules**: Regenerated in-place and synced.
- **Read-only Modules**: Schema is recovered by parsing published `*_orm.go` files.

---

## 5. Execution Pipeline

`Model` → `Query` → `Compiler` → `Plan` → `Executor`

1. **`Compiler`**: Translates agnostic ORM queries into engine-specific strings (SQL, etc.).
2. **`Executor`**: Standardized interface for running queries and commands, compatible with `database/sql` but engine-independent.

---

## 6. DDL Tooling

- `ddlc`: CLI tool to generate SQL schemas from model definitions.
- `ddl.TopologicalSort`: Orders tables based on foreign key dependencies for safe creation/deletion.
