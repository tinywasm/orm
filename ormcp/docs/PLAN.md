# PLAN — ormcp: declarar `Args` en las tools `db_*` (sin generar schema) + test de respaldo

> This plan is dispatched via the CodeJob workflow. See skill: `agents-workflow`.
> **Módulo:** `github.com/tinywasm/orm/ormcp` (go.mod propio, junto a este `docs/`).
> **Depende de:** `github.com/tinywasm/mcp` con `Tool.Args model.Fielder` (ver MASTER_PLAN, gate mcp).

You are an external agent with **zero prior context** about this project. Everything you
need is in this file. Read it fully before writing code.

---

## 1. Problema

Las tools `db_query`, `db_exec`, `db_schema`, `db_export_schema` exponen un `inputSchema`
**inválido**: `encodeSchema` (en `provider.go`) serializa el struct de args con valores cero
(`{"SQL":""}`), y las tools sin args ponen `InputSchema: ""` (→ `null`). Clientes MCP como Claude
Code descartan TODO el `tools/list` si una tool es inválida → el agente no ve ninguna tool.

**Generar el JSON Schema NO es responsabilidad de ormcp.** Ahora `tinywasm/mcp` lo genera desde el
modelo de args (`Tool.Args model.Fielder` → `Schema()`). ormcp solo declara `Args`.

Los modelos ya están al estándar nuevo (`models.go`: `QueryArgs`/`ExecArgs` con `Validate()`; ormc
genera `Schema() []model.Field`). No hay que tocar los modelos.

---

## 2. Cambios

### 2.1 Borrar `encodeSchema` de `provider.go`

Elimina la función `encodeSchema` (serializaba el struct). ormcp ya no genera JSON Schema.

### 2.2 Declarar `Args` en cada tool

En los 6 sitios `InputSchema:` de las tools (ver abajo), reemplaza:

| Archivo | Tool | Antes | Después |
|---|---|---|---|
| `tool_query.go` | db_query | `InputSchema: encodeSchema(new(QueryArgs))` | `Args: new(QueryArgs)` |
| `tool_exec.go` | db_exec | `InputSchema: encodeSchema(new(ExecArgs))` | `Args: new(ExecArgs)` |
| `daemon_provider.go` | db_query | `InputSchema: encodeSchema(new(QueryArgs))` | `Args: new(QueryArgs)` |
| `daemon_provider.go` | db_exec | `InputSchema: encodeSchema(new(ExecArgs))` | `Args: new(ExecArgs)` |
| `tool_schema.go` | db_schema | `InputSchema: ""` | (quitar la línea; `Args` nil → mcp emite objeto vacío) |
| `daemon_provider.go` | db_schema (`schemaToolD`) | `InputSchema: ""` | (quitar la línea) |
| `tool_export_schema.go` / `exportToolD` | db_export_schema | (sin `InputSchema`) | (dejar sin `Args`; mcp emite objeto vacío) |

Para las tools sin argumentos, NO pongas `InputSchema` ni `Args`: mcp genera
`{"type":"object","properties":{}}` por defecto.

### 2.3 Bump de `mcp`

Sube `github.com/tinywasm/mcp` en `go.mod` a la versión con `Tool.Args`. `go mod tidy`.

---

## 3. Test de respaldo

Ya existe `mcp_inputschema_test.go` (paquete `ormcp`, NO lo borres): construye un `mcp.Server` con
`NewDaemonProvider().Tools()`, llama `tools/list` y exige que CADA tool tenga
`inputSchema = {"type":"object",...}` (nunca `null` ni el struct serializado), y que `db_query`
exponga `"SQL":{"type":"string"}`. Usa **solo `tinywasm/json`**. Debe **pasar** tras estos cambios.

Ejecuta `go test ./...` (o `gotest ./...`): todo verde.

---

## 4. Documentación

- Actualiza `docs/`/`README.md` de ormcp si describen la generación del `inputSchema`: ahora la
  hace `mcp` desde `Tool.Args`; ormcp solo declara los modelos.

---

## Reglas de calidad

- Sin stdlib: `tinywasm/fmt`, `tinywasm/json`, `tinywasm/model`, `tinywasm/orm`, `tinywasm/mcp`.
  Para JSON en tests, **solo `tinywasm/json`**.
- Nada de lógica de JSON Schema en ormcp (ni `encodeSchema`, ni `""` como inputSchema).

---

## Stages

| # | Stage | Output |
|---|-------|--------|
| 1 | Borrar `encodeSchema` de `provider.go` | sin generación en el provider |
| 2 | Cambiar los 6 sitios: args → `Args: new(XxxArgs)`; no-arg → quitar `InputSchema` | tools model-driven |
| 3 | Bump `mcp` + `go mod tidy` | dependencia nueva |
| 4 | `go test ./...` verde (incl. `mcp_inputschema_test.go`) | acceptance test pasa |
