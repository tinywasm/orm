# PLAN — Generar `inputSchema` JSON Schema válido en las MCP tools de ormcp (`db_*`)

> This plan is dispatched via the CodeJob workflow. See skill: `agents-workflow`.
> **Módulo:** `github.com/tinywasm/orm/ormcp` (go.mod propio, junto a este `docs/`).

You are an external agent with **zero prior context** about this project. Everything you
need is in this file. Read it fully before writing code.

---

## 1. Problema

Las MCP tools `db_query`, `db_exec`, `db_schema` y `db_export_schema` exponen un `inputSchema`
**inválido**. Un cliente MCP como Claude Code valida la respuesta de `tools/list` contra JSON
Schema (Zod); si **una sola** tool es inválida, **descarta el array COMPLETO** → el servidor MCP
aparece "Connected" pero el agente **no ve ninguna tool**.

Evidencia real (log de Claude Code):

```
Failed to fetch tools: [
  { "path": ["tools", 12, "inputSchema", "type"], "message": "Invalid input: expected \"object\"" },
  { "path": ["tools", 18, "inputSchema"], "message": "Invalid input: expected object, received null" },
  ...
]
```

### Dos causas

**(a) Tools con argumentos** — `encodeSchema` en `provider.go` serializa el struct con valores
cero en vez de generar JSON Schema:

```go
// provider.go  (ROTO)
func encodeSchema(f model.Encodable) string {
	var s string
	_ = json.Encode(f, &s)   // ❌ produce {"SQL":""}, no un JSON Schema
	return s
}
```

Para `db_query` esto emite `{"SQL":""}`. Válido sería:
`{"type":"object","properties":{"SQL":{"type":"string"}},"required":["SQL"]}`.

**(b) Tools sin argumentos** — `db_schema` y `db_export_schema` ponen `InputSchema: ""`, que se
emite como `null` en la respuesta. Válido sería `{"type":"object","properties":{}}`.

Los tipos de args (`QueryArgs`, `ExecArgs`) son modelos ormc que **ya exponen**
`Schema() []model.Field`:

```go
func (m *QueryArgs) Schema() []model.Field { return _schemaQueryArgs }
```

Por lo tanto el JSON Schema válido se genera desde `Schema()`, sin reflection ni hardcodeo.

---

## 2. Cambios

### 2.1 `provider.go` — reescribir `encodeSchema` + añadir mapeo + constante vacía

**Reglas del ecosistema TinyWasm (OBLIGATORIAS):**
- **Sin stdlib**: usar `github.com/tinywasm/fmt` (buffer `fmt.Conv` con `.Write(string)` /
  `.String()`, ya usado en este archivo en `scanRowsToText`). NO `strings`/`strconv`/`bytes`/`encoding/json`.
- **Sin strings mágicos duplicados**: fragmentos de schema centralizados en `jsonSchemaType`;
  schema vacío en constante nombrada `EmptyInputSchema`.

`provider.go` ya importa `fmt`, `json`, `mcp`, `orm` y `model`. Reemplazar la función
`encodeSchema` por lo siguiente (y añadir la constante y el mapeo en el mismo archivo):

```go
// EmptyInputSchema is the JSON Schema for a tool that takes no arguments.
// MCP clients require inputSchema to be a valid JSON Schema object; an empty
// string or null is rejected and causes the ENTIRE tools/list to be discarded
// (Claude Code validates tools/list with Zod).
const EmptyInputSchema = `{"type":"object","properties":{}}`

// encodeSchema builds a valid JSON Schema "object" string for an MCP tool's
// inputSchema, derived from the args model's Schema() field metadata. Replaces
// the previous broken behavior that json-encoded the struct's zero values
// (e.g. {"SQL":""}), which is NOT a JSON Schema and is rejected by MCP clients.
// Returns EmptyInputSchema for a nil model or one with no fields.
func encodeSchema(m model.Fielder) string {
	if m == nil {
		return EmptyInputSchema
	}
	fields := m.Schema()
	if len(fields) == 0 {
		return EmptyInputSchema
	}
	var b fmt.Conv
	b.Write(`{"type":"object","properties":{`)
	var required []string
	for i, f := range fields {
		if i > 0 {
			b.Write(",")
		}
		b.Write(`"`)
		b.Write(f.Name)
		b.Write(`":`)
		b.Write(jsonSchemaType(f.Type))
		if f.NotNull {
			required = append(required, f.Name)
		}
	}
	b.Write("}")
	if len(required) > 0 {
		b.Write(`,"required":[`)
		for i, name := range required {
			if i > 0 {
				b.Write(",")
			}
			b.Write(`"`)
			b.Write(name)
			b.Write(`"`)
		}
		b.Write("]")
	}
	b.Write("}")
	return b.String()
}

// jsonSchemaType maps a model.FieldType to its JSON Schema fragment.
//   FieldText, FieldRaw, FieldBlob -> string
//   FieldInt                       -> integer
//   FieldFloat                     -> number
//   FieldBool                      -> boolean
//   FieldIntSlice                  -> array of integer
//   FieldStruct                    -> object
//   FieldStructSlice               -> array of object
func jsonSchemaType(t model.FieldType) string {
	switch t {
	case model.FieldInt:
		return `{"type":"integer"}`
	case model.FieldFloat:
		return `{"type":"number"}`
	case model.FieldBool:
		return `{"type":"boolean"}`
	case model.FieldIntSlice:
		return `{"type":"array","items":{"type":"integer"}}`
	case model.FieldStruct:
		return `{"type":"object"}`
	case model.FieldStructSlice:
		return `{"type":"array","items":{"type":"object"}}`
	default: // FieldText, FieldRaw, FieldBlob
		return `{"type":"string"}`
	}
}
```

> Si `encodeSchema` cambiaba de firma (`model.Encodable` → `model.Fielder`) rompe algún call
> site, ajusta el call site pasando el mismo `new(XxxArgs)` (todos satisfacen `model.Fielder`).

### 2.2 Reemplazar los sitios `InputSchema: ""` por `EmptyInputSchema`

Hay **tres** tools sin argumentos que deben usar la constante en vez de `""`:

- `tool_schema.go:14` → `InputSchema: EmptyInputSchema,`
- `daemon_provider.go:44` (`schemaToolD`) → `InputSchema: EmptyInputSchema,`
- `tool_export_schema.go` (`db_export_schema`) — actualmente **no fija** `InputSchema` (queda
  `""`). Añade explícitamente `InputSchema: EmptyInputSchema,` en ese `mcp.Tool{...}`.

> Revisa `daemon_provider.go` `exportToolD` también: si no fija `InputSchema`, añádelo con
> `EmptyInputSchema`. Cualquier `mcp.Tool` sin args debe llevar `EmptyInputSchema`, nunca `""`.

Las tools con args (`tool_query.go`, `tool_exec.go`, `daemon_provider.go` query/exec) ya llaman
`encodeSchema(new(QueryArgs))` / `encodeSchema(new(ExecArgs))` y quedan correctas automáticamente
tras 2.1 — no requieren cambios.

---

## 3. Tests

Añade `provider_schema_test.go` (paquete `ormcp`) que verifique:

1. `encodeSchema(new(QueryArgs))` produce:
   `{"type":"object","properties":{"SQL":{"type":"string"}},"required":["SQL"]}`
   (ajusta `required` según el flag `NotNull` real de `_schemaQueryArgs`; si `SQL` no es
   `NotNull`, omite `required` — verifica el schema generado del modelo).
2. `encodeSchema(nil)` devuelve `EmptyInputSchema`.
3. Para cada tool de **ambos** providers (`Provider` y `DaemonProvider`), su `InputSchema`:
   - No es `""` ni `"null"`.
   - Decodifica como JSON válido con `github.com/tinywasm/json`.
   - Contiene `"type":"object"` en la raíz.

Ejecutar: `go test ./...` (o `gotest ./...`). Todos deben pasar.

---

## 4. Documentación

- Actualiza `docs/ARCHITECTURE.md` / `README.md` de ormcp si describen cómo se genera el
  `inputSchema` de las tools `db_*`. Si no existe tal sección, no crees documentación nueva.

---

## Reglas de calidad (recordatorio)

- Sin stdlib: `tinywasm/fmt`, `tinywasm/json`, `tinywasm/model`, `tinywasm/orm`, `tinywasm/mcp`.
- Sin literales string repetidos en lógica: schema fragments en `jsonSchemaType`; vacío en
  `EmptyInputSchema` (nunca `""` inline en un `mcp.Tool`).
- No introducir `encoding/json`, `reflect`, `strings`, `strconv`, `bytes`.

---

## Stages

| # | Stage | Output |
|---|-------|--------|
| 1 | Reescribir `encodeSchema` + `jsonSchemaType` + `EmptyInputSchema` en `provider.go` | JSON Schema válido desde `Schema()` |
| 2 | Reemplazar los 3+ sitios `InputSchema: ""` (schema/export, daemon) por `EmptyInputSchema` | no-arg tools válidas |
| 3 | Compilar y ajustar call sites si el tipo del parámetro lo requiere | build verde |
| 4 | Añadir `provider_schema_test.go` con las aserciones de §3 | tests verdes |
| 5 | Actualizar docs si existen | docs consistentes |
