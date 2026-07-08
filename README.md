# tinywasm/orm
<img src="docs/img/badges.svg">

**Ultra-lightweight, strongly-typed ORM engineered for WebAssembly and backend environments.**

## Features

- **Declarative Source of Truth**: Hand-written `model.Definition` literals as the source for code generation.
- **Zero Reflection**: Interface-driven schema via `github.com/tinywasm/model` with generated code.
- **Isomorphic**: Same generated code works in Go (backend) and WASM (frontend).
- **0-alloc Codec**: Symmetric, reflection-free serialization/deserialization methods generated for every model.
- **Code Generator**: `ormc` CLI generates concrete Go structs and all the plumbing.

## Installation

```bash
go get github.com/tinywasm/orm
go install github.com/tinywasm/orm/cmd/ormc@latest
```

## Declarative Workflow

In this ORM, you write the **typed definition** of your model, and `ormc` generates the **concrete struct** and the rest of the code. This ensures your schema is a compile-time checked symbol, eliminating errors from fragile string tags.

### 1. Define your models

Create a `model.go` or `models.go` file. Define your models as `model.Definition` variables ending in `Model`.

```go
package user

import (
    "github.com/tinywasm/model"
    "github.com/tinywasm/form/input"
)

var UserModel = model.Definition{
    Name: "user",
    Fields: model.Fields{
        {Name: "id",    Type: model.FieldInt,  DB: &model.FieldDB{PK: true, AutoInc: true}},
        {Name: "name",  Type: model.FieldText, NotNull: true, Widget: input.Text(), Permitted: model.Permitted{Minimum: 2}},
        {Name: "email", Type: model.FieldText, NotNull: true, Widget: input.Email(), DB: &model.FieldDB{Unique: true}},
        {Name: "bio",   Type: model.FieldText, Widget: input.Textarea(), OmitEmpty: true},
        {Name: "addr",  Type: model.FieldStruct, Ref: &AddressModel},
    },
}

var AddressModel = model.Definition{
    Name: "address",
    Fields: model.Fields{
        {Name: "street", Type: model.FieldText},
        {Name: "city",   Type: model.FieldText},
    },
}
```

### 2. Generate

```bash
ormc
```

Generates `model_orm.go` next to each source file, containing the `type User struct`, `type Address struct`, and all interface implementations.

### 3. Use it

```go
func GetUser(db *orm.DB, id int64) (*user.User, error) {
    return user.ReadOneUser(
        db.Query(&user.User{}).Where("id").Eq(id),
        &user.User{},
    )
}
```

## Definition Reference

### `model.Field` Options

| Field | Meaning |
|-------|---------|
| `Name` | wire/DB column name (snake_case). Becomes PascalCase in the generated struct. |
| `Type` | Enum: `FieldText`, `FieldInt`, `FieldFloat`, `FieldBool`, `FieldBlob`, `FieldStruct`, `FieldIntSlice`, `FieldStructSlice`, `FieldRaw`. |
| `NotNull` | `NOT NULL` constraint in DB. |
| `OmitEmpty` | Skips field if zero-value in codecs. |
| `Exclude` | Field exists in generated struct but is skipped in `Schema()`, `Pointers()`, and codecs (e.g. for `password_hash`). |
| `Widget` | Binding for UI rendering (requires `github.com/tinywasm/form/input`). |
| `DB` | Database-specific metadata (`PK`, `Unique`, `AutoInc`, `RefColumn`, `OnDelete`). |
| `Ref` | Points to another `*model.Definition`. Used for composition or scalar Foreign Keys. |
| `Permitted` | Validation rules (Minimum, Maximum, Letters, Numbers, etc.). |

### Relations

1.  **Composition**: When `Type` is `FieldStruct` or `FieldStructSlice`. The field becomes a nested struct or slice of structs part of the same row/JSON.
2.  **Scalar Foreign Key**: When `Type` is a scalar (e.g. `FieldInt`) and `Ref` is non-nil. Adds DDL metadata for `FOREIGN KEY` constraints.

```go
{Name: "user_id", Type: model.FieldInt, Ref: &UserModel, DB: &model.FieldDB{RefColumn: "id", OnDelete: "CASCADE"}}
```

## Schema Types Mapping

| `model.FieldType` | Generated Go Type | Codec Method |
|---|---|---|
| `FieldText`, `FieldRaw` | `string` | `String` / `Raw` |
| `FieldInt` | `int64` | `Int` |
| `FieldFloat` | `float64` | `Float` |
| `FieldBool` | `bool` | `Bool` |
| `FieldBlob` | `[]byte` | `Bytes` |
| `FieldIntSlice` | `[]int` | `Array` |
| `FieldStruct` | `T` (from `Ref`) | `Object` |
| `FieldStructSlice` | `[]*T` (from `Ref`) | `Array` |

## ormc — Code Generation

Run from the **project root**. Scans subdirectories for `model.go` / `models.go`.

**Generated per model:**

- `type T struct` declaration.
- `Schema() []model.Field`, `Pointers() []any` (parallel, zero-alloc).
- `EncodeFields(model.FieldWriter)`, `DecodeFields(model.FieldReader)` (symmetric codec).
- `IsNil() bool`.
- `Validate(action byte) error` (calling `model.ValidateFields`). Always generated —
  validation is secure by default; models with no rules get a cheap no-op.
- `ModelName() string`.
- `ReadOneT()`, `ReadAllT()`, `TList` type (for DB models).
- `SchemaExt() []orm.FieldExt` (for scalar FKs).

## More Documentation

- [Architecture](docs/ARQUITECTURE.md)
- [Improvement Roadmap](docs/IMPROVE.md)
