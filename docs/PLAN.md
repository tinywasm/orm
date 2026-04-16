# PLAN: Correcciones faltantes del soporte FieldRaw

## Contexto

El soporte `json:",raw"` fue implementado en `ormc.go` y `ormc_generate.go` y funciona correctamente. Faltan dos elementos del plan original.

## 1. Test de regresión en `tests/ormc_test.go`

Verificar que campos sin tag `raw` siguen generando `fmt.FieldText`. Añadir dentro del test existente `TestOrmc` (o como sub-test independiente):

```go
t.Run("json tag without raw stays FieldText", func(t *testing.T) {
    err := orm.NewOrmc().GenerateForStruct("MCPResponse", "models.go")
    // ...
    mustHave := []string{
        // result y error son FieldRaw — ya testeado
        `{Name: "result", Type: fmt.FieldRaw`,
    }
    // Regresión: un campo string normal sin raw debe ser FieldText
    // Usar otro struct de models.go que tenga campos string sin raw
    // y verificar que genera fmt.FieldText, no fmt.FieldRaw
})
```

La forma más limpia: añadir un struct en `tests/models.go`:

```go
// ormc:formonly
type PlainResponse struct {
    Message string `json:"message"`
    Code    string `json:"code,omitempty"`
}
```

Y verificar que genera `fmt.FieldText` para ambos campos — regresión que `raw` no contamina campos normales.

## 2. Actualizar `docs/STRUCT_TAGS.md`

Añadir sección sobre la opción `raw` del tag `json:`. Buscar la sección del tag `json:` y añadir:

```markdown
### Opción `raw`

Indica que el campo string contiene JSON pre-serializado. El ORM genera `fmt.FieldRaw`
en lugar de `fmt.FieldText`. `tinywasm/json` lo emite inline sin quotes.

| Tag | Schema generado |
|---|---|
| `json:"name"` | `Type: fmt.FieldText` |
| `json:"name,omitempty"` | `Type: fmt.FieldText, OmitEmpty: true` |
| `json:"name,raw"` | `Type: fmt.FieldRaw` |
| `json:"name,omitempty,raw"` | `Type: fmt.FieldRaw, OmitEmpty: true` |

Caso de uso: campos que contienen objetos o arrays JSON (respuestas MCP, payloads HTTP).
```
