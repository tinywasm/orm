# orm — MCP Database Tools

Exponer `*orm.DB` como `mcp.ToolProvider` para que el LLM interactúe
directamente con la base de datos de desarrollo: inspeccionar esquema, ejecutar
queries de lectura y ejecutar mutaciones SQL libres.

---

## Contexto

Durante desarrollo el LLM necesita:
1. **Conocer el esquema real** — contrastar la DB con el código para detectar
   migraciones faltantes o divergencias.
2. **Leer datos** — reproducir bugs, verificar inserciones, depurar queries.
3. **Mutar datos** — crear fixtures, corregir datos corruptos, ejecutar DDL manual.

Acceso **raw SQL libre** separando lectura (`db_query`) de mutación (`db_exec`)
para que el LLM elija conscientemente si está leyendo o escribiendo.

---

## Dependencias a agregar en `go.mod`

```bash
go get github.com/tinywasm/mcp
go get github.com/tinywasm/json
go get github.com/tinywasm/context
```

---

## Paso 1 — Interfaz `SchemaInspector` en `orm/schema.go`

Archivo nuevo con build tag backend-only. Define la interfaz que los adaptadores
(sqlite, postgres) implementan para introspección de esquema.

**`orm/schema.go`:**
```go
//go:build !wasm

package orm

// SchemaInspector is optionally implemented by Executor adapters to expose
// database schema introspection. If the adapter does not implement it,
// the db_schema MCP tool is not registered.
type SchemaInspector interface {
    Tables() ([]string, error)
    Columns(table string) ([]ColumnInfo, error)
}

// ColumnInfo describes a single column returned by SchemaInspector.
type ColumnInfo struct {
    Name    string
    Type    string
    NotNull bool
    PK      bool
}
```

> **Nota:** `TableIntrospector` ya existe en `sync.go` y solo devuelve nombres
> de columna para el sync. `SchemaInspector` es una interfaz separada más rica
> destinada al MCP — no reemplaza a `TableIntrospector`.

---

## Paso 2 — Subpaquete `orm/mcp/`

Backend-only por naturaleza (importa `tinywasm/mcp` que no compila en WASM).
No necesita `//go:build !wasm` porque `go/ast` y las dependencias MCP ya lo
hacen imposible en WASM.

### Estructura de archivos

```
orm/mcp/
  models.go       ← Args structs con // ormc:formonly
  model_orm.go    ← generado por ormc (NO editar a mano)
  provider.go     ← Provider type, NewProvider, Tools()
  tool_schema.go  ← db_schema tool
  tool_query.go   ← db_query tool
  tool_exec.go    ← db_exec tool
```

---

### `orm/mcp/models.go`

Define los structs de entrada para las tools. El `// ormc:formonly` indica a
`ormc` que genere `Schema()` sin helpers de DB (no hay tabla real).

```go
package mcporm

// ormc:formonly
type QueryArgs struct {
    SQL  string `db:"not_null" input:"-"`
    Args string `input:"-"` // JSON array, e.g. ["val1", 2]
}

// ormc:formonly
type ExecArgs struct {
    SQL  string `db:"not_null" input:"-"`
    Args string `input:"-"` // JSON array, e.g. ["val1", 2]
}
```

Después de crear este archivo, ejecutar `ormc` en el directorio `orm/mcp/`:

```bash
cd orm/mcp && ormc
```

Esto genera `model_orm.go` con `Schema() []fmt.Field` y `Validate()` para cada
struct — necesario para `EncodeSchema`.

---

### `orm/mcp/provider.go`

```go
package mcporm

import (
    "github.com/tinywasm/fmt"
    "github.com/tinywasm/json"
    "github.com/tinywasm/mcp"
    "github.com/tinywasm/orm"
)

// Provider implements mcp.ToolProvider for a live *orm.DB connection.
type Provider struct {
    db *orm.DB
}

// NewProvider creates a new MCP tool provider wrapping the given DB.
func NewProvider(db *orm.DB) *Provider {
    return &Provider{db: db}
}

// Tools returns the MCP tools available for this DB connection.
// db_schema is only included if the underlying executor implements orm.SchemaInspector.
func (p *Provider) Tools() []mcp.Tool {
    tools := []mcp.Tool{
        queryTool(p.db),
        execTool(p.db),
    }
    if _, ok := p.db.RawExecutor().(orm.SchemaInspector); ok {
        tools = append([]mcp.Tool{schemaTool(p.db)}, tools...)
    }
    return tools
}

// encodeSchema encodes a fmt.Fielder as a JSON schema string for InputSchema.
func encodeSchema(f fmt.Fielder) string {
    var s string
    _ = json.Encode(f, &s)
    return s
}
```

---

### `orm/mcp/tool_schema.go`

```go
package mcporm

import (
    "github.com/tinywasm/context"
    "github.com/tinywasm/fmt"
    "github.com/tinywasm/mcp"
    "github.com/tinywasm/orm"
)

func schemaTool(db *orm.DB) mcp.Tool {
    return mcp.Tool{
        Name:        "db_schema",
        Description: "List all tables and their columns with types and constraints. Use this first to understand the database structure before writing queries.",
        InputSchema: "",  // no input args
        Resource:    "database",
        Action:      'r',
        Execute: func(ctx *context.Context, req mcp.Request) (*mcp.Result, error) {
            inspector := db.RawExecutor().(orm.SchemaInspector)

            tables, err := inspector.Tables()
            if err != nil {
                return nil, err
            }

            var out fmt.Conv
            for _, table := range tables {
                out.Write(table + ":\n")
                cols, err := inspector.Columns(table)
                if err != nil {
                    out.Write("  (error reading columns: " + err.Error() + ")\n")
                    continue
                }
                for _, col := range cols {
                    line := "  " + col.Name + " " + col.Type
                    if col.PK {
                        line += " PK"
                    }
                    if col.NotNull {
                        line += " NOT NULL"
                    }
                    out.Write(line + "\n")
                }
            }
            return mcp.Text(out.String()), nil
        },
    }
}
```

---

### `orm/mcp/tool_query.go`

```go
package mcporm

import (
    "strings"

    "github.com/tinywasm/context"
    "github.com/tinywasm/fmt"
    "github.com/tinywasm/mcp"
    "github.com/tinywasm/orm"
)

func queryTool(db *orm.DB) mcp.Tool {
    return mcp.Tool{
        Name:        "db_query",
        Description: "Execute a read-only SQL query (SELECT/WITH) and return the results as text. Use db_exec for INSERT, UPDATE, DELETE, or DDL.",
        InputSchema: encodeSchema(new(QueryArgs)),
        Resource:    "database",
        Action:      'r',
        Execute: func(ctx *context.Context, req mcp.Request) (*mcp.Result, error) {
            var args QueryArgs
            if err := req.Bind(&args); err != nil {
                return nil, err
            }
            upper := strings.ToUpper(strings.TrimSpace(args.SQL))
            if !strings.HasPrefix(upper, "SELECT") && !strings.HasPrefix(upper, "WITH") {
                return nil, fmt.Err("db_query only accepts SELECT or WITH statements; use db_exec for mutations")
            }

            rows, err := db.RawExecutor().Query(args.SQL)
            if err != nil {
                return nil, err
            }
            defer rows.Close()

            return mcp.Text(scanRowsToText(rows)), nil
        },
    }
}
```

---

### `orm/mcp/tool_exec.go`

```go
package mcporm

import (
    "github.com/tinywasm/context"
    "github.com/tinywasm/mcp"
    "github.com/tinywasm/orm"
)

func execTool(db *orm.DB) mcp.Tool {
    return mcp.Tool{
        Name:        "db_exec",
        Description: "Execute a SQL statement that modifies data or schema: INSERT, UPDATE, DELETE, CREATE TABLE, ALTER TABLE, DROP TABLE, etc.",
        InputSchema: encodeSchema(new(ExecArgs)),
        Resource:    "database",
        Action:      'u',
        Execute: func(ctx *context.Context, req mcp.Request) (*mcp.Result, error) {
            var args ExecArgs
            if err := req.Bind(&args); err != nil {
                return nil, err
            }
            if err := db.RawExecutor().Exec(args.SQL); err != nil {
                return nil, err
            }
            return mcp.Text("OK"), nil
        },
    }
}
```

---

### Helper `scanRowsToText` — en `provider.go` o archivo util

```go
func scanRowsToText(rows orm.Rows) string {
    var out fmt.Conv
    for rows.Next() {
        // orm.Rows.Scan requires known destinations — use raw scanning via RawExecutor
        // This helper is a placeholder; actual implementation depends on whether
        // orm.Rows exposes column names. If not, return raw line count.
        out.Write("row\n")
    }
    if err := rows.Err(); err != nil {
        out.Write("error: " + err.Error())
    }
    return out.String()
}
```

> **Bloqueante:** `orm.Rows` no expone nombres de columnas. Para devolver
> resultados legibles, se necesita una de estas opciones:
> - Extender `orm.Rows` con `Columns() ([]string, error)` (cambio en el root orm)
> - Acceder al executor subyacente y hacer type-assert a `*sql.Rows`
>
> Recomendado: agregar `Columns() ([]string, error)` a la interfaz `orm.Rows`
> en `executor.go`. Los adaptadores (sqlite, postgres, sqlt) ya implementan
> `*sql.Rows` que tiene ese método. Ver sección siguiente.

---

## Paso 3 — Extender `orm.Rows` con `Columns()`

**`orm/executor.go`** — agregar al interface `Rows`:

```go
// Rows represents an iterator over query results.
type Rows interface {
    Next() bool
    Scan(dest ...any) error
    Columns() ([]string, error)  // ← agregar esta línea
    Close() error
    Err() error
}
```

Los adaptadores que wrappean `*sql.Rows` ya tienen `Columns()` — no requieren
cambio. Solo los mocks de test necesitarían implementarlo.

Con `Columns()` disponible, `scanRowsToText` puede construir una tabla legible:

```go
func scanRowsToText(rows orm.Rows) string {
    cols, _ := rows.Columns()
    var out fmt.Conv
    out.Write(strings.Join(cols, " | ") + "\n")
    vals := make([]any, len(cols))
    ptrs := make([]any, len(cols))
    for i := range vals {
        ptrs[i] = &vals[i]
    }
    for rows.Next() {
        rows.Scan(ptrs...)
        var parts []string
        for _, v := range vals {
            parts = append(parts, fmt.Sprint(v))
        }
        out.Write(strings.Join(parts, " | ") + "\n")
    }
    return out.String()
}
```

---

## Archivos afectados en este paquete

| Archivo | Cambio |
|---------|--------|
| `orm/schema.go` | Nuevo — interfaz `SchemaInspector` + `ColumnInfo` |
| `orm/executor.go` | Agregar `Columns() ([]string, error)` a `Rows` |
| `orm/mcp/models.go` | Nuevo — `QueryArgs`, `ExecArgs` con `// ormc:formonly` |
| `orm/mcp/model_orm.go` | Generado por `ormc` — NO editar |
| `orm/mcp/provider.go` | Nuevo — `Provider`, `NewProvider`, `Tools()`, `encodeSchema`, `scanRowsToText` |
| `orm/mcp/tool_schema.go` | Nuevo — tool `db_schema` |
| `orm/mcp/tool_query.go` | Nuevo — tool `db_query` |
| `orm/mcp/tool_exec.go` | Nuevo — tool `db_exec` |

---

## Adaptadores externos requeridos

`tinywasm/sqlite` y `tinywasm/postgres` deben implementar `orm.SchemaInspector`
para que `db_schema` se registre. Cada uno tiene su propio `docs/PLAN.md`.

---

## Orden de ejecución

1. Editar `orm/executor.go` — agregar `Columns()` a `Rows`
2. Crear `orm/schema.go` — `SchemaInspector` + `ColumnInfo`
3. Crear `orm/mcp/models.go` — `QueryArgs`, `ExecArgs`
4. Ejecutar `ormc` en `orm/mcp/` → genera `model_orm.go`
5. Crear `orm/mcp/provider.go`, `tool_schema.go`, `tool_query.go`, `tool_exec.go`
6. Publicar con `gopush`

---

## Verificación

```bash
gotest   # todos los tests deben pasar incluyendo los de orm.Rows
```

Confirmar que `orm/mcp.NewProvider(db).Tools()` devuelve 2 o 3 tools según si
el executor implementa `SchemaInspector`.
