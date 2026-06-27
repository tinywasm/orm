> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.
> Master plan: tinywasm/docs/MASTER_PLAN_SCHEMA_SQL_EXPORT.md
>
> Este plan cubre TODO el trabajo en tinywasm/orm. Se despacha en dos momentos:
>   Parte 1 (ahora)  — Stages S1-S11: orm/ddl + model registry + FieldExt + ormc
>   Parte 2 (después de publicar sqlt y postgres) — Stages S12-S18: modelStub + cmd/ddlc + ormcp tool

# PLAN: orm — Schema SQL Export (completo)

## Preconditions

- **Parte 1**: sin precondiciones externas.
- **Parte 2**: `tinywasm/sqlt` y `tinywasm/postgres` deben estar publicados con `ExportDDL`.
  Actualizar `go.mod` en `orm/` a las versiones publicadas antes de ejecutar Parte 2.

---

# Parte 1 — orm/ddl + model registry + FieldExt + ormc

## Context

Cuatro adiciones al módulo `tinywasm/orm`:

1. Sub-paquete `orm/ddl/` — interfaz `ddl.Exporter` + `TopologicalSort`. Sin SQL, sin adaptadores.
2. Model registry en `orm.DB` — `RegisteredModels()` + `Compiler()` para uso en `ormcp` en runtime.
3. `orm/field_ext.go` — agregar `OnDelete string` a `FieldExt`; parsear `db:"on_delete=cascade"` en `ormc`.
4. `orm/ormc` — `Permitted.Maximum` ya está en `fmt.Field`; los adaptadores lo leen directo.

### Tag `on_delete=`

Snake_case consistente con `not_null`, `old_name=`, `ref=`.
**Default: `CASCADE`** — cuando un campo tiene `ref=`, se emite `ON DELETE CASCADE` automáticamente.
Solo se escribe `on_delete=` para cambiar ese comportamiento.

Valores válidos: `restrict`, `set_null`, `no_action`. `cascade` también es aceptado explícitamente.

```go
// No requiere on_delete= — CASCADE es el default:
type Session struct {
    UserID int64 `db:"ref=users"`
}

// Solo cuando se quiere comportamiento diferente:
type AuditLog struct {
    UserID int64 `db:"ref=users,on_delete=restrict"`
}
```

---

## S1 — `orm/ddl/exporter.go` (nuevo)

```go
package ddl

import "github.com/tinywasm/fmt"

// Exporter is implemented by SQL adapter compilers (sqlt, postgres).
// ExportDDL returns CREATE TABLE + index statements for all models, in FK dependency order.
type Exporter interface {
    ExportDDL(models []fmt.Model) (string, error)
}
```

## S2 — `orm/ddl/sort.go` (nuevo)

```go
package ddl

import (
    "github.com/tinywasm/fmt"
    "github.com/tinywasm/orm"
)

// TopologicalSort returns models sorted so parents come before children (Kahn's BFS).
// Models not implementing SchemaExt() are treated as having no FK deps.
// Returns error on circular FK dependency.
func TopologicalSort(models []fmt.Model) ([]fmt.Model, error) {
    byName := make(map[string]fmt.Model, len(models))
    rdeps  := make(map[string][]string)
    inDeg  := make(map[string]int, len(models))

    for _, m := range models {
        name := m.ModelName()
        byName[name] = m
        inDeg[name] = inDeg[name]
        if ext, ok := m.(interface{ SchemaExt() []orm.FieldExt }); ok {
            for _, f := range ext.SchemaExt() {
                if f.Ref != "" {
                    rdeps[f.Ref] = append(rdeps[f.Ref], name)
                    inDeg[name]++
                }
            }
        }
    }

    var queue []string
    for _, m := range models {
        if inDeg[m.ModelName()] == 0 {
            queue = append(queue, m.ModelName())
        }
    }

    result := make([]fmt.Model, 0, len(models))
    for len(queue) > 0 {
        name := queue[0]
        queue = queue[1:]
        result = append(result, byName[name])
        for _, dep := range rdeps[name] {
            inDeg[dep]--
            if inDeg[dep] == 0 {
                queue = append(queue, dep)
            }
        }
    }

    if len(result) != len(models) {
        return nil, fmt.Err("ddl: circular FK dependency detected")
    }
    return result, nil
}
```

**Sin stdlib**. Usa `github.com/tinywasm/fmt` para errores.
`orm/ddl` es sub-paquete del módulo `github.com/tinywasm/orm` — sin `go.mod` propio.

## S3 — Model registry en `orm/db.go`

Agregar campo `models []fmt.Model` a la struct `DB` (appended por Sync/SyncSchema):

```go
func (db *DB) registerModel(m fmt.Model) {
    for _, existing := range db.models {
        if existing.ModelName() == m.ModelName() {
            return
        }
    }
    db.models = append(db.models, m)
}

func (db *DB) RegisteredModels() []fmt.Model { return db.models }

func (db *DB) Compiler() Compiler { return db.compiler }
```

## S4 — Llamar `registerModel` en `orm/sync.go`

Después de `db.CreateTable(m)` exitoso en `Sync()` → `db.registerModel(m)`.
Después de construir `schemaModel` en `SyncSchema()` → `db.registerModel(schemaModel)`.

## S5 — `OnDelete string` en `FieldExt` (`orm/field_ext.go`)

```go
type FieldExt struct {
    fmt.Field
    Ref       string // FK: target table name.
    RefColumn string // FK: target column. Empty = auto-detect PK.
    OnDelete  string // Override ON DELETE action. Empty = CASCADE (default for all FKs).
}
```

## S6 — Parsear `on_delete=` en `orm/ormc/generator.go` (líneas ~392-401)

Declarar `var onDelete string` junto a `ref`, `refCol`. Agregar al switch de tag parts:

```go
case fmt.HasPrefix(p, "on_delete="):
    onDelete = fmt.Convert(p).TrimPrefix("on_delete=").String()
    switch onDelete {
    case "cascade", "set_null", "restrict", "no_action":
    default:
        return StructInfo{}, fmt.Errf("on_delete= must be cascade|set_null|restrict|no_action, got %q", onDelete)
    }
```

Agregar `OnDelete string` a `FieldInfo`. Pasar a `SchemaExt()` generado.
`OnDelete` vacío significa CASCADE — los adaptadores lo resuelven, no el generador.

## S7 — Emitir `OnDelete` en `orm/ormc/generate.go`

En el cuerpo generado de `SchemaExt()`, incluir `OnDelete` cuando no vacío:

```go
{Field: schema[i], Ref: "users", RefColumn: "id", OnDelete: "cascade"},
```

## S8 — Tests Parte 1

### `orm/ddl/sort_test.go`
- `TestTopologicalSort_NoDeps` — `[users, roles]` sin FKs; ambos presentes sin error.
- `TestTopologicalSort_WithFK` — `sessions` FK→`users`; assert `users` index < `sessions` index.
- `TestTopologicalSort_Cycle` — `a`→`b`, `b`→`a`; assert error con `"circular"`.
- `TestTopologicalSort_NoSchemaExt` — modelo sin `SchemaExt()`; sin error, incluido en resultado.

### `orm/db_test.go`
- `TestModelRegistry_NoDuplicates` — `Sync` dos veces; `RegisteredModels()` retorna exactamente 2.
- `TestCompilerAccessor` — `db.Compiler()` retorna el mismo valor pasado a `orm.New()`.

### `orm/ormc/generator_test.go`
- `TestOnDelete_Default` — `db:"ref=users"` → `FieldExt.OnDelete == ""` (adaptador emite CASCADE).
- `TestOnDelete_Restrict` — `db:"ref=users,on_delete=restrict"` → `FieldExt.OnDelete == "restrict"`.
- `TestOnDelete_Invalid` — `db:"ref=users,on_delete=wipe"` → error.

## Constraints Parte 1

- RULE: `orm/ddl` NO importa `sqlt` ni `postgres` — solo interfaz.
- RULE: Sin stdlib en `orm/ddl`. Usar `github.com/tinywasm/fmt`.
- RULE: `TopologicalSort` maneja modelos sin `SchemaExt()` sin panic.
- RULE: `registerModel` usa `ModelName()` como identidad — sin reflect.

---

# Parte 2 — modelStub + cmd/ddlc + ormcp tool

*(Ejecutar solo después de publicar sqlt y postgres con ExportDDL)*

## S9 — `modelStub` y `ExportSQL` en `orm/ormc/generate.go`

`modelStub` convierte `StructInfo` → `fmt.Model` sin DB activa. `Pointers()` retorna nil.
`SchemaExt()` incluye `OnDelete` para que los adaptadores emitan ON DELETE y los índices correctos.

```go
type modelStub struct {
    name   string
    schema []fmt.Field
    exts   []orm.FieldExt
}

func (m *modelStub) ModelName() string             { return m.name }
func (m *modelStub) Schema() []fmt.Field           { return m.schema }
func (m *modelStub) Pointers() []any               { return nil }
func (m *modelStub) IsNil() bool                   { return m == nil }
func (m *modelStub) EncodeFields(_ fmt.FieldWriter) {}
func (m *modelStub) DecodeFields(_ fmt.FieldReader) {}
func (m *modelStub) SchemaExt() []orm.FieldExt     { return m.exts }

func newModelStub(info StructInfo) *modelStub {
    stub := &modelStub{name: info.ModelName}
    for _, f := range info.Fields {
        field := fmt.Field{
            Name:    f.ColumnName,
            Type:    goTypeToFieldType(f.GoType),
            NotNull: f.NotNull,
        }
        if f.IsPK || f.IsUnique || f.AutoInc {
            field.DB = &fmt.FieldDB{PK: f.IsPK, Unique: f.IsUnique, AutoInc: f.AutoInc}
        }
        if f.Maximum > 0 {
            if field.DB == nil {
                field.DB = &fmt.FieldDB{}
            }
            field.Permitted.Maximum = f.Maximum
        }
        stub.schema = append(stub.schema, field)
        if f.Ref != "" {
            stub.exts = append(stub.exts, orm.FieldExt{
                Field:     field,
                Ref:       f.Ref,
                RefColumn: f.RefColumn,
                OnDelete:  f.OnDelete,
            })
        }
    }
    return stub
}

func goTypeToFieldType(goType string) fmt.FieldType {
    switch goType {
    case "int", "int8", "int16", "int32", "int64",
         "uint", "uint8", "uint16", "uint32", "uint64":
        return fmt.FieldInt
    case "float32", "float64":
        return fmt.FieldFloat
    case "bool":
        return fmt.FieldBool
    case "[]byte":
        return fmt.FieldBlob
    default:
        return fmt.FieldText
    }
}

func (g *Generator) ExportSQL(root string, exporter ddl.Exporter) (string, error) {
    infos, err := g.parseDir(root)
    if err != nil {
        return "", err
    }
    var models []fmt.Model
    for _, info := range infos {
        if info.NoDB {
            continue
        }
        models = append(models, newModelStub(info))
    }
    if len(models) == 0 {
        return "", nil
    }
    return exporter.ExportDDL(models)
}
```

## S10 — `orm/cmd/ddlc/` — módulo separado

**CRÍTICO: `cmd/ddlc` debe tener su propio `go.mod`.**
`ormcp` ya sigue este patrón (tiene `ormcp/go.mod` con `replace ../`).
Si `cmd/ddlc` NO tiene su propio `go.mod`, el módulo raíz `orm/go.mod` heredará
`sqlt` y `postgres` como dependencias — violando la regla de que `orm` no depende de sus adaptadores.

### `orm/cmd/ddlc/go.mod` (nuevo)

```
module github.com/tinywasm/orm/cmd/ddlc

go 1.25

require (
    github.com/tinywasm/fmt v0.x.x
    github.com/tinywasm/orm v0.x.x
    github.com/tinywasm/orm/ormc v0.x.x   // si ormc tiene go.mod propio, sino usar replace
    github.com/tinywasm/postgres v0.x.x
    github.com/tinywasm/sqlt v0.x.x
)

replace github.com/tinywasm/orm => ../../
```

> Si `ormc` no tiene `go.mod` propio (es sub-paquete del módulo `orm`), usar solo
> `replace github.com/tinywasm/orm => ../../` y acceder `ormc` como `github.com/tinywasm/orm/ormc`.

### `orm/cmd/ddlc/main.go` (nuevo)

```go
package main

import (
    "flag"
    "os"

    "github.com/tinywasm/fmt"
    "github.com/tinywasm/orm/ddl"
    "github.com/tinywasm/orm/ormc"
    "github.com/tinywasm/postgres"
    "github.com/tinywasm/sqlt"
)

var (
    rootFlag    = flag.String("root", ".", "Directory to scan for model.go / models.go")
    outFlag     = flag.String("out", "-", "Output file. Use \"-\" for stdout.")
    dialectFlag = flag.String("dialect", "sqlite", "SQL dialect: sqlite | postgres")
)

func main() {
    flag.Parse()
    var exporter ddl.Exporter
    switch *dialectFlag {
    case "postgres":
        exporter = postgres.NewCompiler()
    default:
        exporter = sqlt.NewCompiler()
    }
    g := ormc.New()
    sql, err := g.ExportSQL(*rootFlag, exporter)
    if err != nil {
        fmt.Println("ddlc:", err)
        os.Exit(1)
    }
    if *outFlag == "-" {
        fmt.Print(sql)
        return
    }
    if err := os.WriteFile(*outFlag, []byte(sql), 0644); err != nil {
        fmt.Println("ddlc:", err)
        os.Exit(1)
    }
}
```

**Verificación obligatoria tras implementar:**
`cat orm/go.mod` NO debe contener `sqlt` ni `postgres`.
Solo `orm/cmd/ddlc/go.mod` los referencia.

## S11 — `orm/ormcp/tool_export_schema.go` (nuevo)

```go
package ormcp

import (
    "github.com/tinywasm/context"
    "github.com/tinywasm/fmt"
    "github.com/tinywasm/mcp"
    "github.com/tinywasm/orm"
    "github.com/tinywasm/orm/ddl"
)

var toolExportSchema = mcp.Tool{
    Name:        "db_export_schema",
    Description: "Export the full CREATE TABLE DDL for all synced tables as SQL text.",
    Resource:    "database",
    Action:      'r',
    Execute: func(ctx *context.Context, req mcp.Request) (*mcp.Result, error) {
        return executeExportSchema(daemonDB, req)
    },
}

func executeExportSchema(db *orm.DB, _ mcp.Request) (*mcp.Result, error) {
    if db == nil {
        return nil, fmt.Err("no database configured: call start_development first")
    }
    exporter, ok := db.Compiler().(ddl.Exporter)
    if !ok {
        return nil, fmt.Err("adapter does not support DDL export")
    }
    models := db.RegisteredModels()
    if len(models) == 0 {
        return mcp.Text("-- no tables registered"), nil
    }
    sql, err := exporter.ExportDDL(models)
    if err != nil {
        return nil, err
    }
    return mcp.Text(sql), nil
}
```

Registrar `toolExportSchema` en `orm/ormcp/provider.go` (slice de `Tools()`).

## S12 — Tests Parte 2

### `orm/ormc/generate_test.go`
- `TestExportSQL_TwoTablesWithFK` — dos structs, mock exporter; assert 2 modelos, `SchemaExt` con `Ref` y `OnDelete`.
- `TestExportSQL_SkipsNoDB` — struct sin `db:` tag; assert solo 1 modelo pasa al exporter.
- `TestModelStub_FieldTypes` — campos int/float/bool/string/[]byte; assert `FieldType` correcto.
- `TestModelStub_MaximumPropagated` — campo con `Maximum=50`; assert `field.Permitted.Maximum == 50`.

### `orm/ormcp/daemon_test.go`
- `TestExportSchema_NoDB` — `daemonDB == nil` → error `"no database configured"`.
- `TestExportSchema_NoAdapter` — compiler sin `ddl.Exporter` → error `"adapter does not support"`.
- `TestExportSchema_WithAdapter` — mock exporter retorna SQL fijo → resultado correcto.

## Constraints Parte 2

- RULE: `cmd/ddlc` DEBE tener su propio `go.mod` (igual que `ormcp`). Sin él, `orm/go.mod` heredará `sqlt` y `postgres` — incorrecto.
- RULE: `orm/go.mod` NO debe contener `sqlt` ni `postgres` al terminar. Verificar con `cat orm/go.mod`.
- RULE: `ormc` NO importa `sqlt` ni `postgres`. Exporter inyectado por `cmd/ddlc`.
- RULE: `modelStub.Pointers()` retorna nil — DDL nunca llama Pointers.
- RULE: `modelStub` implementa `SchemaExt()` con `OnDelete` para que FKs e índices sean correctos.
- RULE: Sin stdlib en `ormcp`. Usar `github.com/tinywasm/fmt`.

---

## Stages summary

| Stage | Parte | Archivo | Cambio |
|---|---|---|---|
| S1 | 1 | `orm/ddl/exporter.go` (nuevo) | Interfaz `ddl.Exporter` |
| S2 | 1 | `orm/ddl/sort.go` (nuevo) | `TopologicalSort` |
| S3 | 1 | `orm/db.go` | Campo `models` + `registerModel` + `RegisteredModels` + `Compiler()` |
| S4 | 1 | `orm/sync.go` | Llamar `registerModel` tras cada CreateTable exitoso |
| S5 | 1 | `orm/field_ext.go` | Agregar `OnDelete string` a `FieldExt` |
| S6 | 1 | `orm/ormc/generator.go` | Agregar `OnDelete` a `FieldInfo`; parsear `on_delete=` |
| S7 | 1 | `orm/ormc/generate.go` | Emitir `OnDelete` en `SchemaExt()` generado |
| S8 | 1 | tests (`sort_test.go`, `db_test.go`, `generator_test.go`) | Tests Parte 1 |
| S9 | 2 | `orm/ormc/generate.go` | `modelStub`, `newModelStub`, `goTypeToFieldType`, `ExportSQL` |
| S10 | 2 | `orm/cmd/ddlc/main.go` (nuevo) | CLI: flags + wiring sqlt/postgres |
| S11 | 2 | `orm/ormcp/tool_export_schema.go` (nuevo) | Tool `db_export_schema` |
| S11b | 2 | `orm/ormcp/provider.go` | Registrar `toolExportSchema` |
| S12 | 2 | tests (`generate_test.go`, `daemon_test.go`) | Tests Parte 2 |
| S13 | 2 | `orm/docs/ARQUITECTURE.md` | Documentar todo: `orm/ddl`, `on_delete=`, `VARCHAR(n)`, `ddlc`, `db_export_schema` |
