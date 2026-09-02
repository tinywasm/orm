---
PLAN: "feat: UpdateFields writes a column subset (PATCH semantics)"
TAG: v0.12.0
EXECUTOR: jules
REVIEWER: none
STATUS: running
SESSION: 17202710894724034611
---

> Este plan se despacha con el flujo CodeJob. Ver skill: agents-workflow.
>
> Forma parte de una ola: `docs/BULK_ACTIONS_MASTER_PLAN.md` en la raíz del
> monorepo. Este plan es **independiente** — no espera a ningún otro.

# Plan — `UpdateFields`: escribir un subconjunto de columnas

## 0. Prerrequisito

```bash
go install github.com/tinywasm/devflow/cmd/gotest@latest
```

Los tests se ejecutan con `gotest` (sin argumentos para la suite completa,
`gotest -run TestX` para uno). **Nunca `go test`.**

## 1. Contexto: qué hace hoy `Update` y por qué no basta

`orm/db.go` expone:

```go
func (d *DB) Update(m model.Model, cond storage.Condition, rest ...storage.Condition) error
```

Su cuerpo construye la lista de columnas a partir de **todo** el esquema:

```go
schema := m.Schema()
columns := make([]string, len(schema))
for i, f := range schema {
    columns[i] = f.Name
}
q := storage.Query{
    Action:     storage.ActionUpdate,
    Table:      m.ModelName(),
    Columns:    columns,
    Values:     model.ReadValues(schema, m.Pointers()),
    Conditions: conds,
}
```

Es decir: **escribe siempre todas las columnas**. Eso es semántica `PUT`, y
tiene dos consecuencias malas para el caso que este plan habilita (corregir un
campo en varios registros a la vez):

1. Pisa columnas que el usuario no tocó. Si otro usuario cambió otro campo del
   mismo registro entremedio, ese cambio se revierte en silencio (*lost
   update*).
2. Obliga a tener el registro completo y fresco, cuando la intención es
   "pon `date` a este valor en estos tres ids".

Hace falta una variante que escriba **sólo las columnas nombradas**.

## 2. Lo que NO hay que tocar (verificado)

- `storage.Query` **ya** lleva `Columns []string` y `Values`, y el compilador
  ya maneja listas de columnas arbitrarias. No hay que cambiar `storage`.
- `sqlt/translate.go` ya compila el operador `IN` a `IN (?, ?, ?)` con los
  argumentos expandidos. No hay que cambiar `sqlt`.
- `orm.DB.Delete(m, cond, rest...)` ya acepta condiciones, así que el borrado
  masivo (`db.Delete(&M{}, storage.In("id", ids))`) **ya funciona**. No lo
  toques; sólo añade el test de la Etapa 3 que lo deja fijado.
- `orm/tx.go` ya expone `Tx(fn)`. Este plan **no lo necesita**: una sola
  sentencia es atómica por definición. No envuelvas nada en `Tx`.

## 3. Etapa 1 — `UpdateFields`

Fichero: **`orm/db.go`** (el mismo donde vive `Update`, justo debajo de él).

```go
// UpdateFields updates ONLY the named columns, leaving every other column of
// the matched rows untouched. It is the PATCH counterpart of Update, which
// writes the whole schema and therefore overwrites columns the caller never
// meant to touch — a lost update whenever anyone else changed one of them in
// the meantime.
//
// fields holds Schema() field names. Order is irrelevant; duplicates are
// rejected. An empty fields slice is an error, not a silent no-op: a caller
// that computed an empty change set has a bug, and a no-op UPDATE would hide
// it.
//
// At least one Condition is required, same as Update — there is no variadic
// fallback, which is what makes an accidental whole-table UPDATE a
// compile-time error rather than a production incident.
func (d *DB) UpdateFields(m model.Model, fields []string, cond storage.Condition, rest ...storage.Condition) error {
```

Cuerpo, en este orden exacto:

1. `if err := validateQuery(storage.ActionUpdate, m); err != nil { return err }`
   — igual que `Update`.
2. Si `len(fields) == 0`, devolver
   `fmt.Err("orm: UpdateFields requires at least one field")`.
3. Recorrer `m.Schema()` **una sola vez** construyendo:
   - `columns []string` — los nombres presentes en `fields`
   - `values []any` — sus valores, leídos de `model.ReadValues(schema, m.Pointers())`
     en el índice correspondiente
   Se recorre el esquema (no `fields`) para que el orden de columnas y valores
   sea el mismo que produce `Update`, y para validar de paso.
4. Si algún nombre de `fields` no apareció en el esquema, devolver
   `fmt.Errf("orm: UpdateFields: unknown field %q", name)`.
5. Si algún nombre de `fields` aparece dos veces, devolver
   `fmt.Errf("orm: UpdateFields: duplicate field %q", name)`.
6. Construir la `storage.Query` igual que `Update` pero con las `columns` y
   `values` recortados, compilar con `d.conn.Compile(q, m)` y ejecutar con
   `d.conn.Exec(plan.Query, plan.Args...)`.

**Anti-footgun:** `orm` es backend. Usa la librería estándar con toda
legitimidad donde ya la usa; **no** conviertas sus imports a `tinywasm/fmt`
salvo que el fichero ya lo importe. Mira los imports del propio `db.go` y
sigue lo que ya haya ahí para construir los errores.

## 4. Etapa 2 — Tests de `UpdateFields`

Fichero: **`orm/tests/update_fields_test.go`** (o el directorio de tests que ya
use el paquete — mira dónde viven los tests de `Update` y ponlos al lado).

Casos, todos contra el driver en memoria que la suite ya usa:

| Test | Comprueba |
|---|---|
| `TestUpdateFieldsWritesOnlyTheNamedColumns` | Fila con 3 columnas; `UpdateFields` con 1 → esa cambia, las otras dos conservan su valor anterior |
| `TestUpdateFieldsRejectsAnEmptyFieldList` | Error, y el mensaje contiene `at least one field` |
| `TestUpdateFieldsRejectsAnUnknownField` | Error, y el mensaje contiene `unknown field` |
| `TestUpdateFieldsRejectsADuplicateField` | Error, y el mensaje contiene `duplicate field` |
| `TestUpdateFieldsAppliesToEveryMatchedRow` | 3 filas, condición `storage.In("id", …)` con los 3 ids → las 3 quedan con el valor nuevo, en **una** llamada |

El último es el que justifica el plan entero: deja fijado que el camino masivo
es una sola sentencia.

## 5. Etapa 3 — Test que fija el borrado masivo ya existente

Fichero: **el mismo de la Etapa 2**.

```
TestDeleteRemovesEveryMatchedRow
```

Inserta 3 filas, llama a `db.Delete(&M{}, storage.In("id", []any{...}))` una
sola vez, y comprueba que las 3 desaparecieron y que las no incluidas siguen
ahí.

Este test **no cambia código**: documenta y protege una capacidad que ya
existe y de la que va a depender toda la ola. Sin él, un refactor futuro de
`Delete` podría romper el borrado masivo sin que nada avise.

**Anti-footgun:** `storage.In` exige `[]any`, **no** `[]string`. Un
`[]string` compila (el parámetro es `any`) pero revienta en tiempo de
ejecución con `IN operator requires []any value, got []string`. Convierte
explícitamente. Y el slice vacío también es error
(`IN operator slice cannot be empty`), así que el llamante debe comprobarlo
antes.

## 6. Criterios de aceptación

- [ ] `gotest` en verde (vet, race, cover).
- [ ] `grep -n "func (d \*DB) UpdateFields" orm/db.go` → una línea.
- [ ] `grep -rn "orm.DB.Tx\|d.Tx(" orm/db.go` → sin resultados nuevos: este
      plan **no** introduce transacciones.
- [ ] `Update` sigue existiendo, con su firma y su cuerpo intactos:
      `grep -n "func (d \*DB) Update(" orm/db.go` → una línea.
- [ ] Ningún fichero de `storage/` ni de `sqlt/` modificado:
      `git diff --name-only` no menciona esos directorios.

## 7. Etapas

| # | Etapa | Ficheros | Depende de |
|---|---|---|---|
| 1 | `UpdateFields` | `orm/db.go` | — |
| 2 | Tests de `UpdateFields` | `orm/tests/update_fields_test.go` | 1 |
| 3 | Test del borrado masivo existente | `orm/tests/update_fields_test.go` | — |
