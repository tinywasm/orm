# PLAN — tinywasm/orm: invertir el generador `ormc` (Definition → struct generado)

> This plan is dispatched via the CodeJob workflow. See skill: **agents-workflow**.

Eres un agente **sin contexto previo** y **solo tienes este repositorio** (`tinywasm/orm`). Este plan
es **autocontenido**: todo contrato, regla y ejemplo está inline. No enlaza a archivos de otros repos.

Requiere `github.com/tinywasm/model` en su versión más reciente (`Definition`, `Fields`, `Field.Ref`,
`Field.Exclude`, `FieldDB.RefColumn`/`OnDelete` — todos ya publicados). Actualiza `go.mod`:
`go get github.com/tinywasm/model@latest`.

**Este plan es una revisión de un intento previo que quedó atascado.** Léelo completo antes de tocar
código — la sección 0 explica exactamente los tres conceptos que se confundieron la primera vez y por
qué son distintos. Si en algún punto dudas si algo es "composición", "relación FK escalar (DDL)" o "el
loader has-many viejo", vuelve a la sección 0 antes de improvisar.

---

## 0. Los tres conceptos que NO debes mezclar (leer primero)

El repo tiene HOY tres mecanismos relacionados con "este modelo apunta a otro", con nombres
parecidos pero significados distintos. Antes de escribir una línea de código, entiende cuál es cuál:

### (A) Composición — `Field.Ref` cuando `Field.Type` es `FieldStruct`/`FieldStructSlice`
El campo **es parte del propio row/struct**: un valor anidado que viaja dentro del `Schema()` /
`Pointers()` / codec del padre. Ejemplo: `Order.ShippingAddress` es un `Address` embebido.
```go
{Name: "shipping_address", Type: model.FieldStruct, Ref: &AddressModel}
```
`Ref` apunta a la `Definition` cuyo tipo Go se usa para ese campo.

### (B) Relación FK escalar (metadata de DDL) — `Field.Ref` cuando `Field.Type` es escalar
El campo es una columna normal (`int64`/`string`/...) que además **referencia la fila de otra
tabla** — ej. `WorkCalendar.StaffID int64` apuntando a `Staff.ID`. NO cambia el tipo Go del campo
(sigue siendo `int64` según el mapeo de §4.4); solo añade metadata para que el DDL emita
`FOREIGN KEY (staff_id) REFERENCES staff(id) ON DELETE ...`.
```go
{Name: "staff_id", Type: model.FieldInt, NotNull: true, Ref: &StaffModel,
    DB: &model.FieldDB{RefColumn: "id", OnDelete: "CASCADE"}}
```
Esto **reemplaza** al viejo `db:"ref=staff"` (string, sin verificación) con una referencia tipada:
`&StaffModel` es un símbolo real — si no existe, no compila.

**(A) y (B) usan el mismo campo `Field.Ref *Definition`, pero `Field.Type` los desambigua sin
ambigüedad posible:** `FieldStruct`/`FieldStructSlice` → (A) composición. Cualquier otro `Type` con
`Ref` no-nil → (B) FK escalar. Nunca hace falta decidir "a mano" cuál es cuál.

### (C) Loader "has-many" — `ResolveRelations`/`RelationInfo`/`findFKField` (HOY EN EL REPO) — **ELIMINAR**
Mecanismo actual: si el struct **padre** tiene un campo Go `[]Child` (un slice-de-struct que **no**
es parte de su propio schema, solo una comodidad de código), `ResolveRelations` busca en el hijo un
campo cuyo `Ref` (hoy: **string**, tabla del padre) coincida, y genera un loader aparte
`ReadAllChildByParentID(db, parentID)`.

**Verificado exhaustivamente (búsqueda en todo `tinywasm/*` y `veltylabs/modules/*`): el loader
`ReadAllXByY` generado por este mecanismo NO tiene ningún consumidor real fuera de los propios tests
de `orm`.** Es una feature muerta. **Elimínala por completo** en esta migración (ver §6). No intentes
reconciliarla con `Ref` — son conceptos distintos y (C) simplemente se retira.

**Qué SÍ debes preservar de (C):** nada de su lógica; solo confirma (con el mismo tipo de búsqueda
de arriba, `grep -rn "ReadAll.*By[A-Z]"` fuera de `orm/`) que sigue sin haber consumidores antes de
borrarlo, por si algo cambió. Si encuentras un consumidor real, DETENTE y repórtalo — no lo borres.

### Resumen de la tabla de verdad

| Quién usa `Ref` | `Field.Type` | Significado | Vive en |
|---|---|---|---|
| (A) Composición | `FieldStruct`/`FieldStructSlice` | tipo Go anidado, parte del propio row | `model.Field.Ref` |
| (B) FK escalar/DDL | escalar (`FieldInt`, `FieldText`, ...) | metadata para `ON DELETE`/constraint | `model.Field.Ref` + `model.FieldDB.RefColumn/OnDelete` |
| (C) Loader has-many | (cualquiera, campo `[]Child` en el padre) | **ELIMINAR** — sin consumidores | nada (se borra) |

---

## 1. Qué cambia y por qué

`ormc` es el generador de código del ecosistema. **Hoy** el autor escribe un struct Go + tags string
frágiles y `ormc` lo parsea (AST + split de strings) para generar `<file>_orm.go`:

```go
// HOY (entrada): model.go — struct plano + tags string (FRÁGIL: un typo no compila mal, compila y falla en runtime)
// ormc:form
type User struct {
    ID    int    `db:"pk,autoinc"`
    Name  string `input:"required,min=2"`
    Email string `input:"email,required"`
}
```

**Invertimos la flecha.** El autor escribe la definición **tipada a mano** (que es justo lo que hoy
`ormc` genera como `_schemaX`), y `ormc` genera el **struct concreto** + toda la plomería:

```go
// MAÑANA (entrada): model.go — definición tipada, fuente de verdad
var UserModel = model.Definition{
    Name: "user",
    Fields: model.Fields{
        {Name: "id",    Type: model.FieldInt,  DB: &model.FieldDB{PK: true, AutoInc: true}},
        {Name: "name",  Type: model.FieldText, NotNull: true, Widget: input.Text(), Permitted: model.Permitted{Minimum: 2}},
        {Name: "email", Type: model.FieldText, NotNull: true, Widget: input.Email()},
    },
}
```

El `Type` es una constante real y `input.Email()` es un símbolo real → un error **no compila**. Eso
cierra el arnés. Tu trabajo es cambiar el **lado de lectura** de `ormc` (de struct+tags a leer el
literal `model.Definition`) y ampliar el **lado de emisión** para que además emita el `type struct`.

---

## 2. Reglas obligatorias

### Sobre el código de `ormc` (herramienta, corre en host)
- Puede usar stdlib de parsing: `go/ast`, `go/parser`, `go/token`, `os`. **Mantén el parseo AST**
  (no compiles ni importes el paquete del usuario: `ormc` corre en un watcher en vivo y el paquete a
  menudo no compila a mitad de edición).
- Donde el código ya usa `github.com/tinywasm/fmt`, síguelo usando (consistencia).

### Sobre el código GENERADO (compila a WASM — reglas estrictas)
- **Sin stdlib** en lo emitido: nada de `errors`, `strconv`, `strings`, `fmt`. Usar `github.com/tinywasm/fmt` si hace falta.
- **Sin `reflect`.**
- **Embeber por valor**, nunca `*T` embebido.
- `Schema()` devuelve una **variable a nivel de paquete** (zero-alloc): reutiliza el `.Fields` de la
  `Definition` (o una variable filtrada si hay `Exclude`, ver §4.5). NO inline un `[]model.Field{...}`
  nuevo por llamada.
- No emitir aserciones de interfaz (`var _ I = (*T)(nil)`) ni `init()`: romperían el dead-code
  elimination.

---

## 3. Contrato de `model` (inline — es una dependencia importada)

```go
package model

type FieldType int
const (
    FieldText FieldType = iota // string
    FieldInt                   // int64
    FieldFloat                 // float64
    FieldBool                  // bool
    FieldBlob                  // []byte
    FieldStruct                // struct anidado (implementa Fielder) — requiere Ref, ver (A) en §0
    FieldIntSlice              // []int
    FieldStructSlice           // []Fielder — requiere Ref, ver (A) en §0
    FieldRaw                   // JSON pre-serializado (string / model.RawJSON)
)

type FieldDB struct {
    PK        bool
    Unique    bool
    AutoInc   bool
    RefColumn string // solo FK escalar (B): columna en la tabla de Ref. Vacío = pasa vacío (ver §4.6).
    OnDelete  string // solo FK escalar (B): acción ON DELETE. Vacío = pasa vacío (ver §4.6).
}

type Field struct {
    Name      string     // nombre en wire/DB (snake_case)
    Type      FieldType
    NotNull   bool
    OmitEmpty bool
    Widget    Widget     // interfaz; nil = sin binding de UI
    DB        *FieldDB   // nil para structs transporte/form-only
    Ref       *Definition // (A) composición si Type es FieldStruct/FieldStructSlice;
                          // (B) FK escalar (DDL) si Type es escalar. Ver §0.
    Exclude   bool        // el campo existe en el struct generado pero NO entra en
                          // Pointers()/EncodeFields()/DecodeFields() — sin persistencia, sin codec.
    Permitted            // reglas de validación (embebido)
}

type Fields = []Field

type Definition struct {
    Name   string // identidad del modelo: nombre de tabla, ModelName()
    Fields Fields
}
func (d Definition) Field(name string) (Field, bool)

// Interfaces que el código generado debe satisfacer:
type Fielder interface { Schema() []Field; Pointers() []any }
type Widget interface { Type() string; Validate(string) error; Clone(parentID, name string) Widget }

// Codec 0-alloc (métodos por primitivo). Firmas relevantes de model.FieldWriter / model.FieldReader:
//   w.String(name string, v string)         r.String(name string) (string, bool)
//   w.Int(name string, v int64)             r.Int(name string) (int64, bool)
//   w.Float(name string, v float64)         r.Float(name string) (float64, bool)
//   w.Bool(name string, v bool)             r.Bool(name string) (bool, bool)
//   w.Blob(name string, v []byte)           r.Blob(name string) ([]byte, bool)
// (Usa las firmas reales del paquete importado; arriba es la forma.)

func ValidateFields(action byte, f Fielder) error // valida un Fielder según acción 'c'|'u'|'d'
```

`orm.FieldExt` (en este repo, `field_ext.go`, ya existe, NO la toques) es la forma que consume el DDL:
```go
type FieldExt struct {
    model.Field
    Ref       string // FK: nombre de tabla destino
    RefColumn string
    OnDelete  string
}
```
Tu trabajo en §4.6 es poblarla a partir de `Field.Ref.Name` (antes venía de un string de tag).

---

## 4. Convenciones de generación (deterministas)

### 4.1 Descubrimiento de la entrada
- El watcher sigue reaccionando a los archivos `model.go` / `models.go`.
- En vez de buscar `type X struct{...}`, busca **declaraciones de variable de paquete** cuyo valor sea
  un composite literal `model.Definition{...}` (posiblemente con alias del import de `model`).
- Por cada `Definition` encontrada, lee: el **identificador de la variable**, el campo `Name`, y la
  lista `Fields` (cada `Field` es un composite literal; extrae `Name`, `Type`, `NotNull`, `OmitEmpty`,
  `DB{...}` incluyendo `RefColumn`/`OnDelete`, `Ref` (un `&XxxModel`, resuelto según §0/§4.6), `Widget`
  (un `CallExpr` como `input.Email()`), `Exclude`, y `Permitted{...}`).

### 4.2 Nombre del struct generado — convención de arnés
- La variable `Definition` **debe** llamarse `<StructName>Model` (sufijo `Model`).
- El struct generado = nombre de la variable sin el sufijo `Model`. Ej.: `UserModel` → `type User`.
- Si una `model.Definition` de paquete **no** termina en `Model`, **falla ruidosamente** (error de
  generación con mensaje claro). Nada de fallos silenciosos.

### 4.3 Identificador Go de cada campo (desde `Field.Name`, snake_case → PascalCase)
- Divide por `_`, capitaliza cada parte, une. `created_at` → `CreatedAt`.
- Trata iniciales conocidas en MAYÚSCULA: `{id, url, api, http, json, sql, uuid, db, ip, html, css, js}`.
  Ej.: `id` → `ID`, `api_key` → `APIKey`, `user_id` → `UserID`.
- Esta conversión es **determinista pero no siempre reversible** desde nombres de columna irregulares
  ya existentes (ej. `staff_idsnapshot` sin guión bajo). Si un consumidor te reporta un nombre de
  columna que no sigue snake_case estándar, NO lo "corrijas" (renombrarías la columna en la DB) —
  genera el identificador que resulte de la conversión determinista y déjalo así.

### 4.4 `Field.Type` → tipo Go (mapeo FIJO, sin overrides)
Los tipos soportados son EXACTAMENTE los del enum — **no hay más**. Cada escalar mapea a **UN** tipo Go:

| `Field.Type` | Tipo Go | Método codec |
|---|---|---|
| `FieldText`, `FieldRaw` | `string` | `String` |
| `FieldInt` | `int64` | `Int` |
| `FieldFloat` | `float64` | `Float` |
| `FieldBool` | `bool` | `Bool` |
| `FieldBlob` | `[]byte` | `Blob` |
| `FieldIntSlice` | `[]int` | (según codec existente) |
| `FieldStruct` | tipo del `Ref` (composición, (A) en §0) | anidado |
| `FieldStructSlice` | `[]` del tipo del `Ref` (composición, (A) en §0) | anidado |

Enteros y floats son de **64 bits** por defecto y sin variantes (consistencia; nada de overrides
string). Para `FieldStruct`/`FieldStructSlice`, el tipo Go se toma de `Field.Ref` (`*model.Definition`):
ormc lee del AST el identificador referenciado (`&AddressModel` → `AddressModel`), aplica la convención
de §4.2 (`<Struct>Model`→`Struct`) → tipo anidado `Address`. Si `Ref` es nil → **falla ruidoso**. Como
`Ref` es tipado, una referencia inexistente **no compila** (no llega al generador). Selector de otro
paquete (`pkg.AddressModel`) → tipo `pkg.Address`.

**Importante:** si `Field.Type` es escalar (no `FieldStruct`/`FieldStructSlice`) y `Ref` NO es nil, el
tipo Go **no cambia** — sigue siendo el escalar de esta tabla. Es el caso (B) de §0 (FK), tratado en
§4.6, no aquí.

### 4.5 `Field.Exclude` — campo en el struct, fuera del codec

Caso real que motiva este campo: un `password_hash` que el struct debe poder llevar en memoria (código
de aplicación lo setea/compara por un canal propio), pero que **nunca** debe pasar por scan de DB ni
por el codec JSON.

```go
{Name: "password_hash", Type: model.FieldText, Exclude: true}
```

Reglas de emisión cuando `Exclude == true`. **Invariante que no se puede romper:** `Schema()` y
`Pointers()` deben seguir siendo paralelos — misma longitud, mismo índice i-ésimo referido al mismo
campo (contrato de `model.Fielder`, del que dependen `orm` scan, `form.sync`, `json` codec). Por tanto:

- **Sí** emitir el campo en el `type struct` generado (con su tipo Go normal según §4.4) — vive en el
  struct, se puede leer/escribir desde código de aplicación.
- **`Schema()` NO incluye** los campos con `Exclude == true`: devuelve solo el subconjunto no
  excluido, en su orden relativo original.
- **`Pointers()` NO incluye** el puntero de esos campos: mismo subconjunto, mismo orden — preservando
  el paralelismo con `Schema()`.
- **`EncodeFields()`/`DecodeFields()` NO los tocan.**
- `Definition.Fields` (la fuente de verdad que el autor escribió) SÍ conserva el campo completo con
  `Exclude: true` — es metadata de generación, no se filtra ahí. El filtro ocurre solo en lo que
  `ormc` emite como `Schema()`/`Pointers()`.

**Zero-alloc con exclusión:** si ninguna `Field` tiene `Exclude: true`, `Schema()` sigue siendo
`return XModel.Fields` directo (caso de hoy). Si **alguna** lo tiene, `ormc` debe emitir además una
variable de paquete filtrada en tiempo de generación (no en runtime) y devolverla:

```go
var _schemaStaff = []model.Field{ /* subconjunto de StaffModel.Fields sin password_hash */ }
func (m *Staff) Schema() []model.Field { return _schemaStaff }
```

Sigue siendo zero-alloc (variable de paquete); el filtrado ocurre una vez, en build-time, no en cada
llamada.

### 4.6 FK escalar (caso B de §0) → `SchemaExt()`/`orm.FieldExt`

Este mecanismo **existe hoy y tiene consumidores reales** (`tinywasm/user`: `Session`, `Identity`,
`UserRole`, `RolePermission`, `LANIP` — 5 relaciones FK activas; también usado por `postgres`/`sqlt`
para DDL). No lo elimines — solo cambia su **entrada** de string (`db:"ref=staff"`) a tipada (`Ref`).

Cuándo emitir `SchemaExt()`: si algún `Field` del struct tiene `Ref != nil` y `Type` **no** es
`FieldStruct`/`FieldStructSlice` (i.e., es el caso B, FK escalar — no confundir con composición, caso
A, que no genera `SchemaExt`).

```go
func (m *WorkCalendar) SchemaExt() []orm.FieldExt {
    return []orm.FieldExt{
        {Field: _schemaWorkCalendar[1], Ref: "staff", RefColumn: "id", OnDelete: "CASCADE"},
    }
}
```

- `Ref` (string en `FieldExt`) = `Field.Ref.Name` (el `Name` de la `Definition` referenciada — NO el
  nombre de la variable Go).
- `RefColumn` = `Field.DB.RefColumn` **tal cual, incluyendo string vacío si no se especifica**. `ormc`
  NO autodetecta ni reemplaza nada aquí — verificado en el código actual: `ormc` solo transporta el
  valor (vacío o no) hasta `FieldExt.RefColumn`. Es el **compilador DDL** (`postgres/translate.go`)
  quien, al construir el `FOREIGN KEY`, aplica el fallback: `if refCol == "" { refCol = "id" }` (un
  literal `"id"`, NO una búsqueda dinámica de la PK de la tabla referenciada). No cambies ese
  fallback ni lo dupliques en `ormc` — se queda donde está, en el compilador.
- `OnDelete` = `Field.DB.OnDelete` **tal cual, incluyendo string vacío si no se especifica**. Mismo
  patrón: `ormc` NO decide el default — pasa el valor tal cual, y es el compilador DDL quien interpreta
  el vacío (revisa `onDeleteSQL(...)` en `postgres/translate.go` para el comportamiento exacto). No
  hardcodees `"CASCADE"` ni ningún otro valor en `ormc`.

---

## 5. Salida esperada (ejemplo completo)

Para la `Definition` de §1, `ormc` genera `model_orm.go` (junto al `model.go` de entrada):

```go
// DO NOT EDIT. generated by github.com/tinywasm/orm

package user

import (
    "github.com/tinywasm/model"
    "github.com/tinywasm/orm"
)

// —— struct concreto DERIVADO de UserModel (antes lo escribía el autor a mano) ——
type User struct {
    ID    int64
    Name  string
    Email string
}

func (m *User) ModelName() string { return "user" }

func (m *User) Schema() []model.Field { return UserModel.Fields } // zero-alloc: reutiliza la var

func (m *User) Pointers() []any { return []any{&m.ID, &m.Name, &m.Email} }

func (m *User) IsNil() bool { return m == nil }

func (m *User) EncodeFields(w model.FieldWriter) {
    w.Int("id", m.ID)
    w.String("name", m.Name)
    w.String("email", m.Email)
}

func (m *User) DecodeFields(r model.FieldReader) {
    if v, ok := r.Int("id"); ok { m.ID = v }
    if v, ok := r.String("name"); ok { m.Name = v }
    if v, ok := r.String("email"); ok { m.Email = v }
}

type UserList []*User

func (s *UserList) Schema() []model.Field    { return nil }
func (s *UserList) Pointers() []any          { return nil }
func (s *UserList) Len() int                 { return len(*s) }
func (s *UserList) At(i int) model.Fielder   { return (*s)[i] }
func (s *UserList) Append() model.Fielder    { v := &User{}; *s = append(*s, v); return v }
func (s *UserList) IsNil() bool              { return s == nil }
func (s *UserList) EncodeFields(_ model.FieldWriter) {}
func (s *UserList) DecodeFields(_ model.FieldReader) {}

func (m *User) Validate(action byte) error { return model.ValidateFields(action, m) }

func ReadOneUser(qb *orm.QB, m *User) (*User, error) {
    if err := qb.ReadOne(); err != nil { return nil, err }
    return m, nil
}

func ReadAllUser(qb *orm.QB) (UserList, error) {
    var results UserList
    err := qb.ReadAll(
        func() model.Model { return &User{} },
        func(m model.Model) { results = append(results, m.(*User)) },
    )
    return results, err
}
```

> La única diferencia de emisión frente a hoy es la **adición del `type … struct`** (antes venía del
> autor) y que `Schema()` devuelve `<Var>.Fields` en vez de un `_schema<Name>` sintetizado (salvo con
> `Exclude`, ver §4.5). El resto del template (Pointers/Encode/Decode/List/Validate/ReadOne/ReadAll) se
> conserva. `SchemaExt()` se emite solo si aplica (§4.6) — NO se emite ningún loader `ReadAllXByY`
> (esa feature se elimina, ver §6).

Structs sin rol DB (equivalente al viejo `// ormc:formonly`): una `Definition` cuyos `Field` no tienen
`DB` → no se emiten `ReadOne/ReadAll/Validate` con PK; sí Schema/Pointers/codec/List. Mantén el
comportamiento actual para transporte/form-only, ahora deducido de la ausencia de `DB` en los campos.

---

## 6. Qué RETIRAR (dos mecanismos distintos, no los mezcles)

### 6.1 El canal string frágil (struct+tags)
- `ormc/tags.go`: `RewriteModelTags`, `extractTag`, `rewriteRawTag` (reescritura de tags string).
- En `ormc/generator.go`: el parseo de `type struct` y de tags (`ParseStruct`, `parseInputModifiers`,
  `isModifier`, el mapeo por `input:`/`db:`), y `goTypeToFieldType` (dirección inversa a la nueva).
- Comentarios mágicos `// ormc:form` / `// ormc:formonly`: se sustituyen por la deducción a partir de
  la presencia/ausencia de `DB` en los campos de la `Definition`.

### 6.2 El loader has-many (caso C de §0) — confirmado sin consumidores, eliminar completo
- `ormc/relations.go` completo: `RelationInfo`, `ResolveRelations`, `findFKField`.
- En `ormc/generator.go`: el campo `SliceFields []SliceFieldInfo` y `Relations []RelationInfo` de
  `StructInfo` (ya no se poblarán ni se usarán).
- En `ormc/generate.go`: el bloque que itera `info.Relations` y emite `ReadAll%sBy%s(...)`.
- En `ormc/watch.go` y `ormc/scan.go`: las llamadas a `g.ResolveRelations(...)`.
- `orm/tests/ormc_relations_test.go`: bórralo (prueba una feature que ya no existe).

**Antes de borrar 6.2, repite la verificación** (`grep -rn "ReadAll.*By[A-Z]" --include='*.go' .` en
todo `tinywasm/*` y, si tienes acceso, `veltylabs/modules/*`) para confirmar que sigue sin haber
consumidores. Si encuentras uno, DETENTE y no borres — repórtalo.

Sustituye el lado de lectura por un lector de composite literals `model.Definition` (nuevo, en un
archivo p.ej. `ormc/parse_definition.go`). Reaprovecha intacto el resto del lado de emisión
(`generate.go`) adaptando: (a) que la fuente de campos es la `Definition` leída, (b) emitir el
`type struct`, (c) `Schema()` → `<Var>.Fields` (o filtrado, §4.5), (d) `SchemaExt()` desde `Ref`
escalar (§4.6, NO desde el loader de 6.2).

---

## 7. Migrar las fuentes de modelo propias de este repo

- `orm/tests/models.go`: reescribe sus structs+tags como `model.Definition` (para probar el nuevo
  flujo). Incluye al menos un caso de FK escalar (B) y uno de composición (A) para cubrir ambos en
  los tests, además de un caso `Exclude`.
- Regenera cualquier `*_orm.go` del repo (p.ej. `orm/ormcp/models_orm.go`) con el nuevo generador.
- Ajusta los tests de generación existentes al nuevo contrato: entrada = `Definition`, salida =
  struct + plomería. Aplica esta regla al decidir qué hacer con cada test:
  - **Poda** (borra sin reemplazo) los tests que solo ejercitaban el parseo de tags string ahora
    eliminado (ej. cualquier test que construya fixtures con `db:"..."`/`input:"..."` o llame
    directamente a las funciones borradas en §6.1) y los que ejercitaban el loader has-many de §6.2
    (`ormc_relations_test.go`).
  - **Migra** (cambia el fixture de entrada a `Definition`, conserva las aserciones) los tests que
    verifican comportamiento de salida vigente: forma de `Schema()`, `Pointers()`, codec, `*List`,
    `Validate`, `ReadOne`/`ReadAll`, y (nuevo) `SchemaExt()` para el caso FK.
  - **Añade** cobertura explícita nueva para: convención de nombre `<Struct>Model` violada (error
    ruidoso), `Ref` nil en `FieldStruct`/`FieldStructSlice` (error ruidoso), `Exclude` (invariante de
    paralelismo Schema/Pointers), y FK escalar → `SchemaExt()` correcto (RefColumn/OnDelete transportados tal cual, sin
    autodetección en ormc — el fallback vive en el compilador DDL, ver §4.6).

## 8. Documentación

- `docs/ARQUITECTURE.md` (ormc): actualizar el flujo (entrada `Definition`, salida struct+plomería);
  documentar los tres conceptos de §0 explícitamente (composición / FK escalar / loader eliminado).
- `docs/WHY_GENERATED_CODE_IS_FREE.md`: sigue vigente; añade que ahora también se genera el `type struct`.
- Reescribe/retira `SYNC_DESIGN.md` o cualquier doc que describa el parseo de tags o el loader has-many.
- `README.md`: ejemplo de autoría con `model.Definition` (reemplaza el ejemplo struct+tags); incluye
  un ejemplo de FK escalar.

---

## 9. Criterio de aceptación

- `gotest ./...` verde en `tinywasm/orm`.
- Dado un `model.go` con `var XModel = model.Definition{...}`, `ormc` genera `model_orm.go` con el
  `type X struct`, `Schema()` (→ `XModel.Fields` o filtrado), `Pointers()`, `EncodeFields`/
  `DecodeFields`, `Validate`, `XList` (+8 métodos), `ReadOneX`/`ReadAllX`, `ModelName()`.
- Una `Definition` cuya variable no termina en `Model`, o un `FieldStruct`/`FieldStructSlice` con
  `Ref` nil, produce un **error de generación ruidoso** (no un fallo silencioso).
- Un campo con `Exclude: true` aparece en el `type struct` generado pero **no** en `Pointers()` ni en
  `EncodeFields()`/`DecodeFields()`; `Schema()` y `Pointers()` siguen siendo paralelos (misma longitud,
  mismo índice) tras el filtrado.
- Un campo escalar con `Ref` no-nil genera `SchemaExt()` con el `orm.FieldExt` correcto (§4.6):
  `RefColumn`/`OnDelete` transportados tal cual (vacío si no se especifican) — sin autodetección ni
  defaults inventados en `ormc`.
- El helper `<Struct>_` (campos tipados, hoy vía `// orm:typed_fields` en la variable `Definition`)
  se sigue generando igual que antes — **no** es opcional, tiene consumidores reales
  (`item_catalog`, `service_catalog`, `appointment_booking` en `veltylabs/modules`).
- El loader has-many (`ReadAllXByY` vía `ResolveRelations`) **ya no existe** en el código generado ni
  en `ormc` — confirmado sin consumidores (§6.2).
- No queda parseo de tags string ni comentarios `// ormc:form*` en el camino de lectura.
- El código generado no importa stdlib prohibida ni usa `reflect`.

---

## 10. Etapas

| # | Etapa | Salida | Criterio |
|---|---|---|---|
| 1 | Lector de `Definition` | `parse_definition.go`: AST → estructura interna (var, Name, Fields, incl. `Ref`/`Exclude`/`DB.RefColumn`/`DB.OnDelete`) | lee el literal de §1 y de §0(B) correctamente |
| 2 | Mapeos | `Field.Type`→tipo Go (§4.4); `Ref` composición vs FK según `Type` (§0); `Field.Name`→ident Go (§4.3); nombre struct desde var (§4.2) | tablas de §4 cubiertas + errores ruidosos |
| 3 | Emisión | adaptar `generate.go`: emitir `type struct` + `Schema()`→`Var.Fields` (o filtrado, §4.5) + `SchemaExt()` desde FK escalar (§4.6) + resto | salida == §5, `Exclude` y FK respetados |
| 4 | Retirar canal viejo | borrar tags/struct-parse (§6.1) | no quedan referencias; compila |
| 5 | Retirar loader has-many | borrar `relations.go`/campos asociados tras confirmar sin consumidores (§6.2) | no quedan referencias; compila |
| 6 | Migrar fuentes propias + tests | `tests/models.go` (con caso A, B y Exclude), regenerar `*_orm.go`, podar/migrar/añadir tests según §7 | `gotest ./...` verde |
| 7 | Docs | ARQUITECTURE/README/WHY actualizados, documentando los 3 conceptos de §0 | reflejan el flujo invertido |
