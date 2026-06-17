# orm/ormcp — PLAN: DaemonProvider (registro estático con DB en runtime)

## Problema

`ormcp.Provider` requiere un `*orm.DB` concreto en la construcción. En el daemon MCP de `app`,
las tools se registran **una sola vez al arrancar** (patrón singleton, igual que `devbrowser`),
pero la conexión DB llega por-proyecto después de `start_development`. Resultado: las tools
`db_schema`, `db_query`, `db_exec` nunca aparecen en el daemon.

## Solución

Nuevo tipo `DaemonProvider` en `orm/ormcp/daemon_provider.go`. Mismo patrón que `BrowserAdapter`
en `app`: registra las tools al arrancar con schemas fijos, acepta la DB vía `SetDB()` en
runtime, devuelve error "no listo" cuando la DB es nil.

## Archivos a tocar (solo en `orm/ormcp/`)

- **`daemon_provider.go`** — NUEVO
- `provider.go` — sin cambios
- `tool_schema.go`, `tool_query.go`, `tool_exec.go` — sin cambios

## Implementación: `daemon_provider.go`

```go
//go:build !wasm

package ormcp

import (
    "sync"
    "github.com/tinywasm/context"
    "github.com/tinywasm/mcp"
    "github.com/tinywasm/orm"
)

// DaemonProvider implements mcp.ToolProvider for the MCP daemon.
// Tools are registered at startup; SetDB wires the live connection at runtime.
type DaemonProvider struct {
    mu sync.RWMutex
    db *orm.DB
}

func NewDaemonProvider() *DaemonProvider { return &DaemonProvider{} }

// SetDB swaps the active DB. Call with nil when the project stops.
func (p *DaemonProvider) SetDB(db *orm.DB) {
    p.mu.Lock()
    p.db = db
    p.mu.Unlock()
}

// Tools returns db_schema (always), db_query, db_exec — fixed schemas, no DB required.
func (p *DaemonProvider) Tools() []mcp.Tool {
    return []mcp.Tool{
        p.schemaToolD(),
        p.queryToolD(),
        p.execToolD(),
    }
}
```

### Helpers internos

Cada tool delegate hace `p.mu.RLock(); db := p.db; p.mu.RUnlock()` y luego:

- `db == nil` → `return nil, fmt.Err("no database configured: call start_development first")`
- `db_schema`: además verifica type-assert a `orm.SchemaInspector`; si falla →
  `"schema inspection not supported by the current database driver"`
- `db_query` / `db_exec`: lógica idéntica a `tool_query.go` / `tool_exec.go` pero leyendo
  la DB del campo en vez de capturarla en el closure.

> **No duplicar lógica**: extraer helpers privados de `tool_query.go` y `tool_exec.go`
> (`executeQuery(db, sql)`, `executeExec(db, sql)`) que usen ambos `Provider` y `DaemonProvider`.
> Si el refactor se complica, duplicar solo el Execute mínimo es aceptable para la primera versión.

## Invariantes

- `DaemonProvider.Tools()` devuelve **siempre 3 tools** (no condicional en `db_schema` como
  en `Provider`). La guardia "driver no soporta SchemaInspector" está dentro del Execute.
- Thread-safe: `SetDB` y `Execute` usan el mismo `sync.RWMutex`.
- Build tag `!wasm` (igual que el resto de `ormcp`).
- No importar `app` ni nada fuera de `orm`, `mcp`, `context`, `fmt`.

## Tests (en `orm/ormcp/`)

1. `DaemonProvider.Tools()` retorna exactamente 3 nombres: `db_schema`, `db_query`, `db_exec`.
2. Con DB nil → cada Execute retorna error "no database configured", sin panic.
3. Tras `SetDB(db)` → `db_query("SELECT 1")` retorna resultado (con DB de test in-memory SQLite).
4. `SetDB(nil)` después de `SetDB(db)` → vuelve al error "no listo".
