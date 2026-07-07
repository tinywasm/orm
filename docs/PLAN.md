# PLAN — tinywasm/orm: invertir el generador `ormc` (Definition → struct generado)

> This plan is dispatched via the CodeJob workflow. See skill: **agents-workflow**.

Eres un agente **sin contexto previo** y **solo tienes este repositorio** (`tinywasm/orm`). Este plan
es **autocontenido**: todo contrato, regla y ejemplo está inline. No enlaza a archivos de otros repos.

Requiere `github.com/tinywasm/model` con los tipos `Definition`, `Fields` y el campo `Field.Ref`
(ya publicados). Actualiza `go.mod` a esa versión con `go get github.com/tinywasm/model@latest`.

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
  `Definition`. NO inline un `[]model.Field{...}` nuevo por llamada.
- No emitir aserciones de interfaz (`var _ I = (*T)(nil)`) ni `init()`: romperían el dead-code
  elimination.

---

## 3. Contrato de `model` (inline — es una dependencia importada)

```go
package model

type FieldType int
const (
    FieldText FieldType = iota // string
    FieldInt                   // int
    FieldFloat                 // float64
    FieldBool                  // bool
    FieldBlob                  // []byte
    FieldStruct                // struct anidado (implementa Fielder) — requiere Ref
    FieldIntSlice              // []int
    FieldStructSlice           // []Fielder — requiere Ref
    FieldRaw                   // JSON pre-serializado (string / model.RawJSON)
)

type FieldDB struct { PK, Unique, AutoInc bool }

type Field struct {
    Name      string     // nombre en wire/DB (snake_case)
    Type      FieldType
    NotNull   bool
    OmitEmpty bool
    Widget    Widget     // interfaz; nil = sin binding de UI
    DB        *FieldDB   // nil para structs transporte/form-only
    Ref       *Definition // solo FieldStruct/FieldStructSlice: Definition anidada (referencia tipada).
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

---

## 4. Convenciones de generación (deterministas)

### 4.1 Descubrimiento de la entrada
- El watcher sigue reaccionando a los archivos `model.go` / `models.go`.
- En vez de buscar `type X struct{...}`, busca **declaraciones de variable de paquete** cuyo valor sea
  un composite literal `model.Definition{...}` (posiblemente con alias del import de `model`).
- Por cada `Definition` encontrada, lee: el **identificador de la variable**, el campo `Name`, y la
  lista `Fields` (cada `Field` es un composite literal; extrae `Name`, `Type`, `NotNull`, `OmitEmpty`,
  `DB{...}`, `Ref` (un `&XxxModel` para anidados), `Widget` (un `CallExpr` como `input.Email()`), y `Permitted{...}`).

### 4.2 Nombre del struct generado — convención de arnés
- La variable `Definition` **debe** llamarse `<StructName>Model` (sufijo `Model`).
- El struct generado = nombre de la variable sin el sufijo `Model`. Ej.: `UserModel` → `type User`.
- Si una `model.Definition` de paquete **no** termina en `Model`, **falla ruidosamente** (error de
  generación con mensaje claro). Nada de fallos silenciosos.

### 4.3 Identificador Go de cada campo (desde `Field.Name`, snake_case → PascalCase)
- Divide por `_`, capitaliza cada parte, une. `created_at` → `CreatedAt`.
- Trata iniciales conocidas en MAYÚSCULA: `{id, url, api, http, json, sql, uuid, db, ip, html, css, js}`.
  Ej.: `id` → `ID`, `api_key` → `APIKey`, `user_id` → `UserID`.

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
| `FieldStruct` | tipo del `Ref` (obligatorio) | anidado |
| `FieldStructSlice` | `[]` del tipo del `Ref` (obligatorio) | anidado |

Enteros y floats son de **64 bits** por defecto y sin variantes (consistencia; nada de overrides
string). Para `FieldStruct`/`FieldStructSlice`, el tipo Go se toma de `Field.Ref` (`*model.Definition`):
ormc lee del AST el identificador referenciado (`&AddressModel` → `AddressModel`), aplica la convención
de §4.2 (`<Struct>Model`→`Struct`) → tipo anidado `Address`. Si `Ref` es nil → **falla ruidoso**. Como
`Ref` es tipado, una referencia inexistente **no compila** (no llega al generador). Selector de otro
paquete (`pkg.AddressModel`) → tipo `pkg.Address`.

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
> autor) y que `Schema()` devuelve `<Var>.Fields` en vez de un `_schema<Name>` sintetizado. El resto
> del template (Pointers/Encode/Decode/List/Validate/ReadOne/ReadAll/SchemaExt/relaciones) se conserva.

Structs sin rol DB (equivalente al viejo `// ormc:formonly`): una `Definition` cuyos `Field` no tienen
`DB` → no se emiten `ReadOne/ReadAll/Validate` con PK; sí Schema/Pointers/codec/List. Mantén el
comportamiento actual para transporte/form-only, ahora deducido de la ausencia de `DB` en los campos.

---

## 6. Qué RETIRAR (el canal string frágil)

Elimina el camino de lectura basado en struct+tags, ahora muerto:

- `ormc/tags.go`: `RewriteModelTags`, `extractTag`, `rewriteRawTag` (reescritura de tags string).
- En `ormc/generator.go`: el parseo de `type struct` y de tags (`ParseStruct`, `parseInputModifiers`,
  `isModifier`, el mapeo por `input:`/`db:`), y `goTypeToFieldType` (dirección inversa a la nueva).
- Comentarios mágicos `// ormc:form` / `// ormc:formonly`: se sustituyen por la deducción a partir de
  la presencia/ausencia de `DB` en los campos de la `Definition`.

Sustituye ese lado por un lector de composite literals `model.Definition` (nuevo, en un archivo p.ej.
`ormc/parse_definition.go`). Reaprovecha intacto el lado de emisión (`generate.go`) adaptando: (a) que
la fuente de campos es la `Definition` leída, (b) emitir el `type struct`, (c) `Schema()` → `<Var>.Fields`.

---

## 7. Migrar las fuentes de modelo propias de este repo

- `orm/tests/models.go`: reescribe sus structs+tags como `model.Definition` (para probar el nuevo flujo).
- Regenera cualquier `*_orm.go` del repo (p.ej. `orm/ormcp/models_orm.go`) con el nuevo generador.
- Ajusta los tests de generación (`generator_test.go`, `parse_generated_test.go`, `scan_test.go`,
  `inference_test.go`) al nuevo contrato: entrada = `Definition`, salida = struct + plomería.

---

## 8. Documentación

- `docs/ARQUITECTURE.md` (ormc): actualizar el flujo (entrada `Definition`, salida struct+plomería).
- `docs/WHY_GENERATED_CODE_IS_FREE.md`: sigue vigente; añade que ahora también se genera el `type struct`.
- Reescribe/retira `SYNC_DESIGN.md` o cualquier doc que describa el parseo de tags.
- `README.md`: ejemplo de autoría con `model.Definition` (reemplaza el ejemplo struct+tags).

---

## 9. Criterio de aceptación

- `gotest ./...` verde en `tinywasm/orm`.
- Dado un `model.go` con `var XModel = model.Definition{...}`, `ormc` genera `model_orm.go` con el
  `type X struct`, `Schema()` (→ `XModel.Fields`), `Pointers()`, `EncodeFields`/`DecodeFields`,
  `Validate`, `XList` (+8 métodos), `ReadOneX`/`ReadAllX`, `ModelName()`.
- Una `Definition` cuya variable no termina en `Model`, o un `FieldStruct`/`FieldStructSlice` con
  `Ref` nil, produce un **error de generación ruidoso** (no un fallo silencioso).
- Un campo con `Exclude: true` aparece en el `type struct` generado pero **no** en `Pointers()` ni en
  `EncodeFields()`/`DecodeFields()`; `Schema()` y `Pointers()` siguen siendo paralelos (misma longitud,
  mismo índice) tras el filtrado.
- No queda parseo de tags string ni comentarios `// ormc:form*` en el camino de lectura.
- El código generado no importa stdlib prohibida ni usa `reflect`.

---

## 10. Etapas

| # | Etapa | Salida | Criterio |
|---|---|---|---|
| 1 | Lector de `Definition` | `parse_definition.go`: AST → estructura interna (var, Name, Fields) | lee el literal de §1 correctamente |
| 2 | Mapeos | `Field.Type`(+`Ref` para anidados)→tipo Go; `Field.Name`→ident Go; nombre struct desde var | tablas de §4 cubiertas + errores ruidosos |
| 3 | Emisión | adaptar `generate.go`: emitir `type struct` + `Schema()`→`Var.Fields` (o filtrado si hay `Exclude`) + resto | salida == §5, `Exclude` respetado (§4.5) |
| 4 | Retirar canal viejo | borrar tags/struct-parse (§6) | no quedan referencias; compila |
| 5 | Migrar fuentes propias | `tests/models.go`, regenerar `*_orm.go`, ajustar tests | `gotest ./...` verde |
| 6 | Docs | ARQUITECTURE/README/WHY actualizados | reflejan el flujo invertido |
