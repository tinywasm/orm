# PLAN — ormc: inferir el rol del struct desde los tags de campo (eliminar directivos de rol)

> Este plan se despacha vía el workflow CodeJob. Ver skill: **agents-workflow**.
> Módulo: `github.com/tinywasm/orm`. Generador: `ormc/` (CLI `cmd/ormc`). Salida: `model_orm.go`.
> Arquitectura de forms: ver skill **form-codegen** (resumen inline abajo — el agente NO tiene ese contexto).
>
> **Precondición:** `github.com/tinywasm/fmt` ya expone `Decodable.DecodeFields(r FieldReader)` **sin
> `error`** (simétrico con `EncodeFields(w FieldWriter)`). Empezar con `go get github.com/tinywasm/fmt@latest`
> y verificar la firma de `fmt.Decodable` antes de generar (Stage 0). Si `DecodeFields` aún devuelve
> `error`, **detener**: la precondición no se cumple.

## Contexto y problema

`ormc` genera `model_orm.go` a partir de los structs en `model.go`/`models.go`. Hoy el **rol** de cada
struct (modelo DB, formulario, DTO de transporte) se decide con **directivos de comentario a nivel de
struct**, y eso es frágil:

- `tinywasm/orm` se **refactorizó** y el ecosistema quedó **desfasado** (sus `model.go` son los que están
  deprecados, no el revés). El generador reconoce hoy (`ormc/generator.go:172-184`):
  - `orm:form_widgets` → `isForm=true` (genera widgets)
  - `orm:no_db` **o** `ormc:formonly` → `noDB=true`
  - `orm:typed_fields` → FK tipados
- Los `model.go` consumidores (p.ej. `goflare-demo/modules/contact`) llevan `// ormc:form`, que el
  generador actual **no interpreta como widgets** → `isForm=false` → schema con `Widget: nil` → `form.New`
  salta todos los campos (`tinywasm/form/form.go:125 if field.Widget == nil { continue }`) → los
  formularios renderizan **solo el botón submit**. Regresión real: el `model_orm.go` previo de `contact`
  sí tenía `Widget: input.Text()/Email()/Textarea()`.

**El propio sistema de directivos a nivel de struct es el frágil: dos vocabularios y un directivo que
olvidar/desfasar = clase de bug recurrente.**

> Nota: `ModelName()` se genera siempre y **es correcto** — útil para identidad/logs, y NO causa tablas
> espurias: `ScanModules` gatea la creación de tabla con `if !info.NoDB` (`ormc/scan.go:75`), no con la
> presencia de `ModelName`.

## Objetivo

Eliminar los directivos de **rol** a nivel de struct. El rol se **infiere de los tags de campo** (una sola
fuente de verdad, local al dato). Regla central:

| Señal en los campos del struct | Rol | Qué AÑADE sobre el codec base |
|---|---|---|
| ≥1 campo con tag `input:` cuyo valor ≠ `-` | **formulario** | `Widget` en cada campo (default por tipo Go + override por tag) |
| ≥1 campo con tag `db:` cuyo valor ≠ `-` **o** PK detectada por convención `ID` | **modelo DB** | `DB: &fmt.FieldDB{...}` + se sincroniza como tabla en `ScanModules` |
| ninguna de las anteriores | **solo-codec (DTO de transporte)** | nada extra |

- **Codec base siempre**: `ModelName()`, `Schema()`, `Pointers()`, `IsNil()`, `EncodeFields()`,
  `DecodeFields()`, tipo `*List` se generan para **todo** struct procesado (no cambia). `ModelName()` es
  útil siempre (identidad del struct, logs, etc.) → **se genera siempre**.
- **`form/input` se importa solo si hay ≥1 widget** (ya está gateado así en `generate.go` vía `hasWidget`).
  Un modelo DB-only NO arrastra `form/input` → pureza backend / tamaño WASM.
- **La DB-ness es `isDB` (`!NoDB`), no la presencia de `ModelName()`.** `ScanModules` ya gatea la creación
  de tabla con `if !info.NoDB` (`ormc/scan.go:75`), no con `ModelName`. Por eso `ModelName()` siempre se
  genera sin crear tablas espurias: un DTO solo-codec tiene `ModelName()` pero `NoDB=true` → no se sincroniza.

### Decisión tomada (Opción A): el formulario se marca con tags `input:`, no con directivos

Para que un struct sea formulario debe tener **≥1 tag `input:`** en algún campo. En la práctica el costo es
nulo: toda validación (`required`, `min=`, `email`, `textarea`) ya es un modificador `input:`. Esto elimina
la clase de bug "directivo no reconocido / mal escrito". No se conservan `ormc:form`, `ormc:formonly`,
`orm:form_widgets`, `orm:no_db` como señales de rol.

## Reglas de inferencia (precisas)

Calcular DESPUÉS de parsear los campos (no desde el `genDecl.Doc`):

1. `hasForm` := existe un campo cuyo tag `input:"..."` tiene valor ≠ `"-"`.
   - `input:"-"` NO cuenta (es exclusión de ese campo del form).
2. `hasDB` := (existe un campo con tag `db:"..."` cuyo valor ≠ `"-"`) **OR** (se detectó una PK por la
   convención `ID`, ver `fmt.IDorPrimaryKey` usado en `generator.go:358`).
   - `db:"-"` NO cuenta para `hasDB` (exclusión de columna).
3. Rol:
   - `isForm := hasForm`  → controla la asignación de widgets (`generator.go:432 if isForm`).
   - `isDB := hasDB`      → controla `ModelName()` y `DB: &fmt.FieldDB{...}`.
   - `codecOnly := !hasForm && !hasDB` → solo codec.

### Asignación de widgets (cuando `isForm`), sin cambios respecto a hoy salvo el gate

Por cada campo (excepto PK autoinc, que se omite del render pero puede llevar widget):
- `input:"-"` → sin widget.
- `input:"<tipo>,<modificadores>"` con `<tipo>` en `inputWidgets` (`generator.go:481`) → ese widget.
- `input:"<modificadores>"` (solo modificadores, sin tipo) → widget por defecto del tipo Go
  (`defaultWidgets`, `generator.go:468`) + modificadores (`parseInputModifiers`).
- sin tag `input:` pero el struct es form → widget por defecto del tipo Go.

### `ModelName()`, `FieldDB` y sync (rol DB)

- `ModelName()` (`generate.go:46-50`): **no cambia** — se sigue generando siempre (salvo `ModelNameDeclared`,
  que respeta una implementación a mano). Es útil para todos los roles.
- `DB: &fmt.FieldDB{...}` (`generate.go:75`): solo si `isDB` (ya usa `!NoDB`; basta con setear `NoDB := !hasDB`).
- Sync de tabla (`ScanModules`, `ormc/scan.go:75`): ya gateado por `if !info.NoDB`. Al derivar `NoDB` de la
  inferencia, un DTO solo-codec o un form-only **no** se sincroniza (no crea tabla), aunque tenga `ModelName()`.

## Matriz de casos (qué cubre y por qué)

Todos incluyen el **codec base** (incl. `ModelName()`). La columna muestra qué se AÑADE / si se sincroniza:

| Caso | Ejemplo | Señales | Añade |
|---|---|---|---|
| Form + DB | `Contact` (`input:` + `db:"pk,autoinc"`) | hasForm, hasDB | widgets + `FieldDB` + **se sincroniza** (tabla) |
| Form-only | login / filtro de búsqueda (no persistido) | hasForm | widgets; **NoDB → no se sincroniza** |
| DB-only | tabla de unión, log interno, modelo de servidor | hasDB | `FieldDB` + **se sincroniza**; **sin** widgets (no importa `form/input`) |
| Codec-only | `rpcRequest`, `EmailPayload`, payloads JSON-RPC/HTTP | ninguna | nada extra; **NoDB → no se sincroniza** |
| Campo excluido del form | secreto/computado con `input:"-"` | — | ese campo sin widget |
| Campo excluido de DB | derivado con `db:"-"` | — | ese campo no es columna y no cuenta para `hasDB` |

**Por qué importa cada distinción:**
- form vs db controla si se importa `form/input` → tamaño del binario WASM y pureza del backend.
- codec-only sin `ModelName()` → evita que `ScanModules` cree tablas espurias.
- la convención `ID` debe seguir marcando DB aunque no haya tags `db:` → no romper el patrón cero-config.

### Casos borde y escape hatches (solo a nivel de campo)

- **Form-only con campo `ID`** → la convención lo marcaría DB. Para forzar no-DB: `db:"-"` en el `ID`.
- **Codec-only con campo `ID`** (raro) → `db:"-"` en el `ID`.
- No se necesitan directivos de struct para estos: el tag de campo manda.

## Robustez ("a prueba de cambios")

Hoy, meter en `model.go` un struct de runtime con campos no serializables (p.ej. `DevTUI` con
`[]*tabSection`, funcs, channels) hace que `ormc` emita codec que referencia tipos no-`Encodable` →
**rompe el build**. Nuevo comportamiento:

- Un campo de tipo no soportado y no excluido con `db:"-"`/`input:"-"` se **omite por completo** del
  schema, `Pointers`, `EncodeFields` y `DecodeFields` (consistente), con `Warning` claro.
- Si tras omitir queda **0 campos usables**, **saltar el struct entero** (no emitir nada) con un
  `Warning: struct X skipped (no serializable fields)`.
- Resultado: agregar un struct de runtime a `model.go` produce un warning, nunca un build roto.

## Decisión: el tipo `*List` se genera SIEMPRE (non-goal: NO hacerlo condicional)

El `*List` (`Schema/Pointers/Len/At/Append/IsNil/EncodeFields/DecodeFields`) es el **codec de arrays
zero-reflection**: `tinywasm/json` codifica/decodifica colecciones vía `Append()`/`At()`/`Len()`
(`json/decode.go:62`, `json/encode.go:212`). Se usa siempre que un tipo se maneja como colección
(p.ej. `ContactList` en `goflare-demo/web/client.go`).

**Para tipos que nunca son colección (p.ej. `ActionArgs`), el `*List` es código muerto que el linker
elimina por completo.** Medido (un `*List` que implementa la interfaz pero nunca se referencia):

| Compilador | Sin `*List` | Con `*List` no usado | Δ |
|---|---|---|---|
| TinyGo `-opt=z` (modo S) | 22 161 B | 22 161 B | **0 B** |
| Go `GOOS=js` (modo L) | 1 661 289 B | 1 661 293 B | 4 B (alineación) |

`go tool nm` no encuentra el `*List` no usado en el binario. El generado **no** emite asserts de interfaz
ni registro (`var _ I = (*List)(nil)`, `init()`), así que nada lo retiene → DCE perfecto.

**Por lo tanto NO hacer la generación de `*List` condicional.** Sería frágil (el generador no sabe si el
consumidor lo usará), rompería el codec de arrays de DTOs que sí son arrays, y el único costo (tamaño) ya
lo resuelve el linker. El costo restante es solo ruido de fuente — aceptable.

## Etapas de implementación

Todo en `tinywasm/orm/ormc`. No tocar el runtime del ORM.

### Stage 0 — Adaptar el generador a la firma de `DecodeFields` sin `error`
- `go get github.com/tinywasm/fmt@latest` y confirmar que `fmt.Decodable` declara
  `DecodeFields(r FieldReader)` (sin `error`). Si todavía lleva `error`, detener (precondición incumplida).
- `generate.go`: emitir `func (m *T) DecodeFields(r fmt.FieldReader) {` (sin `error`) y **sin** el
  `return nil` final. El `*List` pasa a `func (s *TList) DecodeFields(_ fmt.FieldReader) {}` (sin error).
  Mantener intactas las guardas de presencia por campo (`if v, ok := r.String(name); ok { ... }`).
- `go build ./... && go test ./ormc/...` verdes con la nueva firma antes de seguir.

### Stage 1 — Inferencia de rol desde campos
- En `generator.go`: borrar el seteo de `isForm`/`noDB` desde `genDecl.Doc` (líneas ~177-182). Mantener
  `orm:typed_fields` (es comportamiento de relación, no rol).
- Tras construir `info.Fields`, computar `hasForm`/`hasDB` según las reglas de arriba y setear
  `info.IsForm = hasForm`, `info.NoDB = !hasDB`.
- `hasDB` debe incluir la PK por convención (reutilizar la detección existente `fmt.IDorPrimaryKey`).
- Constantes con nombre para los tokens de tag (`const tagInput = "input:"`, `const tagDB = "db:"`,
  `const tagExclude = "-"`). Prohibido literales repetidos en la lógica.

### Stage 2 — Gate de DB por inferencia (NO tocar ModelName)
- `ModelName()` se sigue generando **siempre** (no cambiar `generate.go:46-50`).
- Setear `info.NoDB := !hasDB`. Con eso, `FieldDB` (`generate.go:75`) y el sync en `ScanModules`
  (`ormc/scan.go:75`, ya gateado por `!info.NoDB`) quedan correctos sin más cambios.

### Stage 3 — Robustez de campos no soportados
- Unificar el manejo: un campo no soportado (los `Warning`/`skip` de `generator.go:331,337` y el de
  relaciones) se omite de TODAS las secciones generadas. Si el struct queda sin campos usables, saltarlo.

### Stage 4 — Tests (en `ormc/`, package de test)
Casos mínimos, con structs de fixture y aserción sobre el `model_orm.go` generado (string match):
- Todos los structs procesados → tienen `ModelName()` (codec base siempre).
- Form+DB → tiene `Widget:` y `DB: &fmt.FieldDB`.
- Form-only (`input:` sin `db:` ni ID) → tiene `Widget:`, **no** `DB: &fmt.FieldDB` (NoDB).
- DB-only (`db:` o `ID`, sin `input:`) → tiene `DB: &fmt.FieldDB`, **no** `Widget:`, **no** import `form/input`.
- Codec-only (sin tags) → **no** `Widget:`, **no** `DB: &fmt.FieldDB` (NoDB); sí `ModelName()`.
- `input:"-"` y `db:"-"` → exclusiones correctas; `db:"-"` en `ID` no marca DB.
- Struct con campo `func()`/tipo desconocido → warning + omitido; struct sin campos usables → saltado.

## Reglas de calidad de código (obligatorias)
- **Sin strings hardcodeados** en la lógica: tokens de tag, nombres de widget y prefijos → constantes con
  nombre. `inputWidgets`/`defaultWidgets` ya son mapas; reusarlos.
- **`cmd/ormc/main.go` delgado**: solo `flag.Parse()` + `ormc.New().Run()`. Toda la lógica vive en `ormc/`
  como funciones exportables/testeables. No agregar lógica en `cmd/`.
- **Sin duplicación**: la detección de PK reutiliza `fmt.IDorPrimaryKey` existente; no reimplementar.
- El generador es CLI `//go:build !wasm` (puede usar `go/ast`, stdlib). El **código generado** debe seguir
  las reglas tinywasm: usa `github.com/tinywasm/fmt` y, si hay widgets, `github.com/tinywasm/form/input`.

## Fuera de alcance (downstream, otros repos)
Tras publicar `orm`, **regenerar consumidores** y quitar de sus `model.go` los directivos de rol obsoletos
(`ormc:form`/`ormc:formonly`/`orm:form_widgets`/`orm:no_db`): `goflare-demo/modules/*`, `mcp`, `devtui`,
`devbrowser`, etc. Eso se coordina en `tinywasm/docs/REGRESSION_FIX_MASTER_PLAN.md`, no en este plan.

## Tabla de etapas

| Stage | Archivo(s) | Foco | Salida verificable |
|---|---|---|---|
| 0 | `go.mod`, `ormc/generate.go` | `go get fmt@latest`; emitir `DecodeFields` sin `error` | `ormc` compila/testea con la firma nueva |
| 1 | `ormc/generator.go` | inferir rol de tags; quitar directivos de rol | `isForm`/`isDB` derivados de campos |
| 2 | `ormc/generator.go` | `NoDB := !hasDB` (gatea `FieldDB`+sync); `ModelName()` sin cambios (siempre) | DTO codec-only con `ModelName()` pero sin `FieldDB` ni sync |
| 3 | `ormc/generator.go`, `generate.go` | omitir campos no soportados; saltar struct vacío | meter struct de runtime no rompe build |
| 4 | `ormc/*_test.go` | cobertura de la matriz de casos | `go test ./ormc/...` verde |
