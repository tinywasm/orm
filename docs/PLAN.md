> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.

# PLAN: orm — fix SyncSchema missing registerModel call

## Context

La Parte 1 del plan anterior fue ejecutada correctamente excepto un punto:
`SyncSchema` no llama `db.registerModel` al final, aunque `Sync` sí lo hace (línea 66).

El plan original (S4) especificaba:
> Después de construir `schemaModel` en `SyncSchema()` → `db.registerModel(schemaModel)`.

## S1 — Agregar `registerModel` en `orm/sync.go`

`SyncSchema` construye un modelo implícito a partir de `table` y `fields`.
Al final del método (tras el sync exitoso), registrarlo:

```go
// Al final de SyncSchema, antes del return nil:
db.registerModel(&schemaModel{table: table, fields: fields})
```

Si no existe un tipo `schemaModel` interno, crear uno mínimo que implemente `fmt.Model`:

```go
type schemaModel struct {
    table  string
    fields []fmt.Field
}
func (m *schemaModel) ModelName() string    { return m.table }
func (m *schemaModel) Schema() []fmt.Field  { return m.fields }
func (m *schemaModel) Pointers() []any      { return nil }
func (m *schemaModel) IsNil() bool          { return m == nil }
func (m *schemaModel) EncodeFields(fmt.FieldWriter) {}
func (m *schemaModel) DecodeFields(fmt.FieldReader) {}
```

> Verificar primero si ya existe algún tipo equivalente en el paquete antes de crear uno nuevo.

## S2 — Test en `orm/db_test.go`

**`TestSyncSchema_RegistersModel`**
```go
// Llamar db.SyncSchema("logs", fields)
// Assert: db.RegisteredModels() contiene un modelo con ModelName() == "logs"
```

## Stages summary

| Stage | Archivo | Cambio |
|---|---|---|
| S1 | `orm/sync.go` | Llamar `db.registerModel` al final de `SyncSchema` |
| S2 | `orm/db_test.go` | `TestSyncSchema_RegistersModel` |
