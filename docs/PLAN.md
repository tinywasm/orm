# PLAN — `ormc` genera `EncodeFields`/`DecodeFields` tipados (codec 0-alloc, map-free) · BREAKING

> Este plan se despacha vía el workflow CodeJob. Ver skill: `agents-workflow`.
> **Estado:** LISTO PARA REVISIÓN DEL USUARIO.
> **Repo objetivo:** `github.com/tinywasm/orm` (el generador `ormc` vive en `orm/ormc/`).
> **Depende de (GATE):** `tinywasm/fmt` con el contrato del codec modificado (`fmt/docs/PLAN.md`).
> **Tipo:** breaking change (los modelos generados ganan 3 métodos nuevos; se regeneran).

## Reglas permanentes del repo → `AGENTS.md`

Las restricciones del ecosistema y de `ormc` (reflection-free; no `map` en WASM; no stdlib →
`tinywasm/fmt`; código generado agnóstico; la regla del codec `EncodeFields`/`DecodeFields`
0-alloc/map-free/any-free/reflect-free; `gotest`/`gopush`) están en [`AGENTS.md`](../AGENTS.md).
Este plan NO las repite completas; solo inlinea lo crítico de la tarea (ver Checklist).

## Prerequisito (PRIMERO — entorno del agente)

```bash
go install github.com/tinywasm/devflow/cmd/gotest@latest
```

Usar `gotest` (sin argumentos); **NO** `go test` directo.

## Contexto (autocontenido)

`ormc` (en `orm/ormc/generate.go`) genera, por cada struct de modelo, un archivo `*_orm.go` con:

```go
func (m *Contact) Schema() []fmt.Field { return _schemaContact }
func (m *Contact) Pointers() []any     { return []any{&m.ID, &m.Nombre, &m.Email, &m.Mensaje} }
```

Este plan **agrega** tres métodos tipados para el codec de serialización 0-alloc (definido en `fmt`):

```go
func (m *Contact) EncodeFields(w fmt.FieldWriter) {
	w.Int("id", int64(m.ID))
	w.String("nombre", m.Nombre)
	w.String("email", m.Email)
	w.String("mensaje", m.Mensaje)
}

func (m *Contact) DecodeFields(r fmt.FieldReader) error {
	if v, ok := r.Int("id"); ok { m.ID = int(v) }
	if v, ok := r.String("nombre"); ok { m.Nombre = v }
	if v, ok := r.String("email"); ok { m.Email = v }
	if v, ok := r.String("mensaje"); ok { m.Mensaje = v }
	return nil
}

func (m *Contact) IsNil() bool {
	return m == nil
}
```

**Cero `any`, cero `map`, cero asignaciones y cero reflexión** en estos métodos (llamadas tipadas directas).

## Mapeo `fmt.FieldType` → método del codec (con loops directos y sin closures)

| `FieldType` | Encode | Decode | Cast / Instanciación |
|---|---|---|---|
| `FieldText` (`FieldRaw`) | `w.String(name, m.F)` | `r.String` | `string` directo |
| `FieldInt` | `w.Int(name, int64(m.F))` | `r.Int` → `m.F = T(v)` | preservar el tipo Go (`int`/`int32`/`int64`) |
| `FieldFloat` | `w.Float(name, float64(m.F))` | `r.Float` → `m.F = T(v)` | `float32`/`float64` |
| `FieldBool` | `w.Bool(name, m.F)` | `r.Bool` | `bool` |
| `FieldBlob` | `w.Bytes(name, m.F)` | `r.Bytes` | `[]byte` |
| `FieldStruct` | `if m.F != nil { w.Object(name, m.F) } else { w.Null(name) }` | `if m.F == nil { m.F = new(T) }; if !r.Object(name, m.F) { m.F = nil }` | campo anidado `Encodable`/`Decodable` (chequeo nil en modelo) |
| `FieldIntSlice` | `arr := w.Array(name, len(m.F)); for _, x := range m.F { arr.Int(int64(x)) }` | `if arr, ok := r.Array(name); ok { n := arr.Len(); m.F = make([]int, n); for i := 0; i < n; i++ { m.F[i] = int(arr.Int(i)) } }` | `[]int` vía flujos directos (sin closures) |
| `FieldStructSlice` | `arr := w.Array(name, len(m.F)); for _, x := range m.F { if x != nil { arr.Object(x) } else { /* nil object handle */ } }` | `if arr, ok := r.Array(name); ok { n := arr.Len(); m.F = make([]*T, n); for i := 0; i < n; i++ { m.F[i] = new(T); arr.Object(i, m.F[i]) } }` | `[]*T` con instanciación antes de leer |

## Pasos de ejecución

### Stage 1 — emisión en el generador
1. En `orm/ormc/generate.go`, junto a la emisión de `Schema()`/`Pointers()`, agregar la emisión de `EncodeFields`, `DecodeFields` e `IsNil()` siguiendo el mapeo de la tabla.
2. Garantizar que se usen loops `for` planos en lugar de callbacks para serialización/deserialización de slices.

### Stage 2 — regenerar los modelos del repo `orm` y fixtures de test
3. Regenerar los `*_orm.go` de los tests/fixtures de `orm` para incluir `EncodeFields`, `DecodeFields` e `IsNil()`. Verificar que compilan.

### Stage 3 — tests del generador
4. Actualizar tests en `orm/ormc` que validen que el código generado para slices y structs anidados no use closures, map, reflect ni any en la serialización.
5. `gotest` verde.

### Stage 4 — documentación
6. Documentar en `README.md` que los modelos generados implementan el codec simétrico 0-alloc y el método `IsNil()`.
