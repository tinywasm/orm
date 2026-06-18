# PLAN — `ormc` genera `EncodeFields`/`DecodeFields` tipados (codec 0-alloc, map-free) · BREAKING

> Este plan se despacha vía el workflow CodeJob. Ver skill: `agents-workflow`.
> **Estado:** LISTO PARA REVISIÓN DEL USUARIO.
> **Repo objetivo:** `github.com/tinywasm/orm` (el generador `ormc` vive en `orm/ormc/`).
> **Depende de (GATE):** `tinywasm/fmt` con el contrato `Encodable`/`Decodable`/`FieldWriter`/
> `FieldReader` ya publicado (ver `fmt/docs/PLAN.md`).
> **Tipo:** breaking change (los modelos generados ganan 2 métodos nuevos; se regeneran).

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

`fmt.Field` tiene `Name` (clave de serialización, p.ej. `"nombre"`) y `Type` (`fmt.FieldType`).
Estos métodos **se conservan** (sirven al scan posicional de SQL `row.Scan(Pointers()...)`,
schema y validación). Este plan **agrega** dos métodos tipados para el codec de serialización
0-alloc (definido en `fmt`):

```go
// Contrato en tinywasm/fmt (ya publicado):
type FieldWriter interface {
	String(name, val string); Int(name string, val int64); Uint(name string, val uint64)
	Float(name string, val float64); Bool(name string, val bool); Bytes(name string, val []byte)
	Null(name string); Object(name string, val Encodable); Array(name string, n int, each func(i int, a ArrayWriter))
}
type Encodable interface { EncodeFields(w FieldWriter) }
type FieldReader interface {
	String(name string)(string,bool); Int(name string)(int64,bool); Uint(name string)(uint64,bool)
	Float(name string)(float64,bool); Bool(name string)(bool,bool); Bytes(name string)([]byte,bool)
	Object(name string, into Decodable) bool; Array(name string)(ArrayReader,bool)
}
type Decodable interface { DecodeFields(r FieldReader) error }
```

## Objetivo

Que `ormc` emita, junto a `Schema()`/`Pointers()`, los métodos:

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
```

**Cero `any`, cero `map`, cero asignación** en estos métodos (llamadas tipadas directas).

## Mapeo `fmt.FieldType` → método del codec (clave + cast por tipo Go real)

`ormc` ya conoce el nombre Go del campo (genera `&m.ID`) y su `FieldType`. Usar:

| `FieldType` | Encode | Decode | Cast |
|---|---|---|---|
| `FieldText` (`FieldRaw`) | `w.String(name, m.F)` | `r.String` | `string` directo |
| `FieldInt` | `w.Int(name, int64(m.F))` | `r.Int` → `m.F = T(v)` | preservar el tipo Go (`int`/`int32`/`int64`) |
| `FieldFloat` | `w.Float(name, float64(m.F))` | `r.Float` → `m.F = T(v)` | `float32`/`float64` |
| `FieldBool` | `w.Bool(name, m.F)` | `r.Bool` | `bool` |
| `FieldBlob` | `w.Bytes(name, m.F)` | `r.Bytes` | `[]byte` |
| `FieldStruct` | `w.Object(name, &m.F)` | `r.Object(name, &m.F)` | campo anidado `Encodable`/`Decodable` |
| `FieldIntSlice` | `w.Array(name, len, …)` empujando `a.Int(int64(x))` | `r.Array` → recorrer | `[]int` |
| `FieldStructSlice` | `w.Array(name, len, …)` empujando `a.Object(&x)` | `r.Array` → `Object` por elemento | `[]Fielder` |

Reusar el **mismo `Field.Name`** que ya usa el `Schema()` como clave (no inventar nombres).
`ormc` debe conocer el tipo Go exacto del campo para el cast correcto (ya lo parsea para
`Pointers()`); si hoy no guarda el tipo Go por campo, agregarlo a la info de parseo.

## Pasos de ejecución

### Stage 1 — emisión en el generador
1. En `orm/ormc/generate.go`, junto a la emisión de `Schema()`/`Pointers()`, agregar la emisión
   de `EncodeFields` y `DecodeFields` siguiendo el patrón de `buf.Write(fmt.Sprintf(...))` ya
   existente y el mapeo de la tabla. Para los `*List` (slices de modelos), NO emitir estos
   métodos (la serialización de listas la maneja el encoder vía `Array`/iteración del consumidor).
2. Asegurar que el tipo Go real de cada campo esté disponible en la info de generación para el
   cast (`int(v)`, `int32(v)`, `float32(v)`, etc.).

### Stage 2 — regenerar los modelos del repo `orm` y fixtures de test
3. Regenerar los `*_orm.go` de los tests/fixtures de `orm` (los que produce `ormc` en este repo)
   para que incluyan los nuevos métodos. Verificar que compilan e implementan `fmt.Encodable`/
   `fmt.Decodable`.

### Stage 3 — tests del generador
4. Actualizar/añadir tests en `orm/ormc` (p.ej. `parse_generated_test.go`/`ormc_test.go`) que
   verifiquen que el código generado contiene `EncodeFields`/`DecodeFields` correctos para cada
   `FieldType` (incluyendo int no-`int64`, float32, blob, struct anidado, slices). Verificar que
   el output NO contiene `map[` ni `reflect` ni `any` en esos métodos.
5. `gotest` verde.

### Stage 4 — documentación (OBLIGATORIO)
6. **`docs/ARCHITECTURE.md`** (o el doc de diseño de ormc): documentar que los modelos generados
   implementan ahora `fmt.Encodable`/`fmt.Decodable` (serialización tipada 0-alloc) además de
   `fmt.Fielder` (DB/schema). **`README.md`**: mencionar los nuevos métodos generados.

## Verificación (repo-local, ejecutable por el agente)

```bash
# 1. El código generado por los tests incluye los métodos tipados:
grep -rn 'func (m \*.*) EncodeFields(w fmt.FieldWriter)' . --include=*_orm.go | head
grep -rn 'func (m \*.*) DecodeFields(r fmt.FieldReader)' . --include=*_orm.go | head

# 2. Los métodos generados NO usan map/any/reflect:
for f in $(grep -rln 'EncodeFields(w fmt.FieldWriter)' --include=*_orm.go .); do
  awk '/EncodeFields|DecodeFields/{p=1} p&&/map\[|[^.]\bany\b|reflect/{print FILENAME": "$0} /^}/{if(p)p=0}' "$f"
done   # → vacío

# 3. Tests verdes:
gotest
```

## Checklist de calidad (obligatorio)

- **0 asignaciones / sin `map` / sin `any` / sin `reflect`** en `EncodeFields`/`DecodeFields`
  generados: llamadas tipadas directas usando `Field.Name` como clave.
- **Sin strings hardcodeados repetidos** en el generador: las claves del codec salen del
  `Field.Name` ya existente (no duplicar literales).
- **Sin duplicación lógica:** reusar la info de parseo de campos que ya alimenta `Pointers()`.
- Reglas genéricas del ecosistema (código generado agnóstico, sin stdlib pesado): ver
  [`AGENTS.md`](../AGENTS.md).

## Tabla de stages

| Stage | Objetivo | Entregable | Criterio de salida |
|---|---|---|---|
| 1 | Emisión | `generate.go` emite `EncodeFields`/`DecodeFields` | compila; patrón correcto por `FieldType` |
| 2 | Regenerar | `*_orm.go` de tests/fixtures regenerados | implementan `fmt.Encodable`/`Decodable` |
| 3 | Tests | tests del generador | `gotest` verde; sin map/any/reflect en output |
| 4 | Documentación | `ARCHITECTURE.md`/`README.md` | doc presente |

## Nota (coordinación)

`fmt` (contrato) es GATE de este plan. Tras publicar `orm`/`ormc`, los repos con modelos
generados (`goflare-demo`, `user`, `devbrowser`, `devtui`, `orm/ormcp`, …) deben **regenerar**
sus `*_orm.go` — eso se coordina en el master plan, no en este PLAN. `tinywasm/json` y
`fmt.Fielder` no cambian. Ver `~/Dev/Project/tinywasm/docs/SIZE_OPTIMIZATION_MASTER_PLAN.md`.
