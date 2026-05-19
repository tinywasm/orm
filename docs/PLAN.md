# PLAN: tinywasm/orm — soporte de tag `notilde` en ormc

## Problema

`ormc` no reconoce la tag `notilde`. Hoy un campo declarado como
`Nombre string \`input:"required,min=2,notilde"\`` falla porque `notilde` no está en
`isModifier()` y se interpreta como tipo de widget.

Necesitamos que `notilde` revoque el flag `Tilde` del widget instance generado para ese
campo, sin tocar `tinywasm/fmt` ni la interface `Widget` ni reestructurar la validación.

## Modelo

Las tags se reparten en dos destinos según su naturaleza:

| Tag | Naturaleza | Destino en `model_orm.go` |
|-----|------------|---------------------------|
| `min=2`, `max=50` | Aditiva (longitud) | `Field.Permitted = fmt.Permitted{Minimum:2,...}` |
| `required` | Aditiva (presencia) | `Field.NotNull = true` |
| `notilde` (y futuros: `nonumbers`, `nospaces`, ...) | Sustractiva (revoca default del widget) | Mutación del widget instance vía setter |

Razón del split: las aditivas suman reglas en `Field.Permitted` (zero-value = no aplica,
sin ambigüedad). Las sustractivas necesitan apagar un flag que el widget tiene en `true`
por default — la instancia del widget ya es mutable post-`Clone`, así que basta un setter.

## Cambios

### 1. `tinywasm/form/input/text.go` — añadir setter

```go
// SetTilde configures whether accented characters (á, é, í, ó, ú, etc.) are allowed.
// Default for Text is true. Set to false via struct tag `input:"...,notilde"` — applied
// by ormc at code-generation time.
// Returns *text to allow chaining; satisfies fmt.Widget.
func (t *text) SetTilde(v bool) *text { t.Tilde = v; return t }
```

`*text` ya implementa `fmt.Widget` (vía `HTMLType` y `Clone`). El return permite emisión
inline desde ormc sin lambdas.

### 2. `tinywasm/orm/ormc.go` — reconocer y emitir

**a) `isModifier()`** — registrar `notilde`:

```go
func isModifier(s string) bool {
    return s == "required" || s == "letters" || s == "numbers" ||
        s == "tilde" || s == "notilde" ||  // ← nuevo
        s == "spaces" || s == "name" ||
        fmt.HasPrefix(s, "min=") || fmt.HasPrefix(s, "max=")
}
```

**b) Mapa tag → setter** — central, dentro de `ormc.go`:

```go
// tagSetters maps a struct-tag modifier to the widget method call that ormc emits
// when generating the Widget field of a Field literal.
// Add an entry here when a new sustractive tag is supported.
var tagSetters = map[string]string{
    "notilde": ".SetTilde(false)",
    // future: "nonumbers": ".SetNumbers(false)", etc.
}
```

**c) Emisión del Widget** — en el generador del literal `Field`:

```go
// fi.WidgetConstructor ya vale "input.Text()" (resuelto por inputWidgets).
out.Write("Widget: ", fi.WidgetConstructor)
for _, tag := range fi.Tags {
    if setter, ok := tagSetters[tag]; ok {
        out.Write(setter)
    }
}
out.Write(",")
```

Salida generada para `Nombre string \`input:"required,min=2,notilde"\``:

```go
{
    Name: "nombre", NotNull: true,
    Widget: input.Text().SetTilde(false),
    Permitted: fmt.Permitted{Minimum: 2},
}
```

## Archivos a modificar

| Librería | Archivo | Cambio |
|----------|---------|--------|
| `tinywasm/form/input` | `text.go` | añadir `SetTilde(bool) *text` |
| `tinywasm/form` | `docs/TAGS.md` | documentar tag `notilde` |
| `tinywasm/orm` | `ormc.go` | `notilde` en `isModifier`, mapa `tagSetters`, emisión |

**No se modifica `tinywasm/fmt`.** La interface `Widget`, `Field.Validate` y
`Permitted` quedan intactos. `Widget.Validate` sigue ejecutándose y validando los chars
del widget — para un widget con `Tilde:false` ya rechazará tildes correctamente.

## Instalación de gotest

```bash
go install github.com/tinywasm/devflow/cmd/gotest@latest
```

Uso: `gotest` — suite completa (vet + race + cover + wasm + badges).

## Tests

`tinywasm/form/input/text_test.go`:

| Test | Verifica |
|------|----------|
| `TestText_SetTildeFalseRejectsAccent` | `input.Text().SetTilde(false).Validate("María")` → error |
| `TestText_SetTildeFalsePreservesLetters` | `...Validate("Maria")` → nil |
| `TestText_DefaultAllowsAccent` | `input.Text().Validate("María")` → nil (regresión) |

`tinywasm/orm/ormc_notilde_test.go`:

| Test | Verifica |
|------|----------|
| `TestOrmc_NotildeIsModifier` | `notilde` no se interpreta como tipo de widget |
| `TestOrmc_NotildeEmitsSetter` | Código generado contiene `input.Text().SetTilde(false)` |
| `TestOrmc_WithoutNotilde_NoSetter` | Sin la tag, el Widget se emite como `input.Text()` plano |
| `TestOrmc_NotildeWithMin` | `notilde,min=2` emite el setter Y `Permitted{Minimum:2}` |

## Orden de ejecución

1. Añadir `SetTilde(bool) *text` en `tinywasm/form/input/text.go`.
2. Añadir entrada `notilde` en `tinywasm/form/docs/TAGS.md`.
3. `gotest` en `tinywasm/form` — debe pasar.
4. Publicar `tinywasm/form` via `gopush`.
5. Modificar `tinywasm/orm/ormc.go`: `isModifier`, `tagSetters`, emisión.
6. Añadir tests en `ormc_notilde_test.go`.
7. `gotest` en `tinywasm/orm` — debe pasar.
8. Publicar `tinywasm/orm` via `gopush`.
9. Regenerar `model_orm.go` en proyectos que usen `notilde`.
