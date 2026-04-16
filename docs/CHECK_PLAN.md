# PLAN: Reconocer json:",raw" y generar FieldRaw

## Contexto

`tinywasm/fmt` v0.23.3 añadió `FieldRaw`. El ORM debe detectar cuándo un campo string debe mapearse a `fmt.FieldRaw` en el schema generado, en lugar de `fmt.FieldText`.

## Restricciones del ecosistema

- Solo se usa `github.com/tinywasm/fmt` — sin imports de stdlib adicionales
- Tamaño de binario mínimo: el cambio reutiliza los mismos loops de parsing de tags ya existentes, sin lógica nueva

## Reutilización de código existente

El parser de `jsonTag` ya recorre las opciones en `ormc.go` línea 317 para detectar `omitempty`. `raw` se detecta en el **mismo loop**, sin nuevo código de parsing:

```go
// ANTES — solo omitempty
if jsonTag != "" {
    parts := fmt.Convert(jsonTag).Split(",")
    for _, p := range parts {
        if p == "omitempty" {
            omitEmpty = true
        }
    }
}

// DESPUÉS — omitempty + raw en el mismo loop
if jsonTag != "" {
    parts := fmt.Convert(jsonTag).Split(",")
    for _, p := range parts {
        if p == "omitempty" {
            omitEmpty = true
        }
        if p == "raw" {
            fieldType = fmt.FieldRaw
        }
    }
}
```

`fieldType` ya es una variable local mutable en ese punto — es exactamente como `omitEmpty`.

## Cambio en `ormc_generate.go`

El switch de `typeStr` ya cubre todos los `FieldType`. Añadir el case `FieldRaw`:

```go
// ANTES
typeStr := "fmt.FieldText"
switch f.Type {
case fmt.FieldInt:   typeStr = "fmt.FieldInt"
case fmt.FieldFloat: typeStr = "fmt.FieldFloat"
case fmt.FieldBool:  typeStr = "fmt.FieldBool"
case fmt.FieldBlob:  typeStr = "fmt.FieldBlob"
case fmt.FieldStruct: typeStr = "fmt.FieldStruct"
}

// DESPUÉS — añadir FieldRaw
case fmt.FieldRaw: typeStr = "fmt.FieldRaw"
```

## Tests a añadir en `tests/`

Siguiendo la convención de `ormc_test.go` (struct en `models.go`, generación y verificación del output):

### Caso: campo con `json:",raw"` genera `fmt.FieldRaw`

```go
t.Run("json raw tag generates FieldRaw", func(t *testing.T) {
    // struct en models.go:
    // // ormc:formonly
    // type MCPResponse struct {
    //     Result string `json:"result,raw"`
    //     Error  string `json:"error,omitempty,raw"`
    // }

    err := orm.NewOrmc().GenerateForStruct("MCPResponse", "models.go")
    // ...
    mustHave := []string{
        `{Name: "result", Type: fmt.FieldRaw`,
        `{Name: "error", Type: fmt.FieldRaw, OmitEmpty: true`,
    }
})
```

### Caso: `raw` y `omitempty` combinados funcionan juntos

Verificar que un campo con `json:"result,omitempty,raw"` genera `OmitEmpty: true` Y `Type: fmt.FieldRaw` — ambas opciones del mismo loop actúan sin interferir.

### Caso: campo sin tag `raw` sigue siendo `FieldText`

Regresión: `json:"name"` y `json:"name,omitempty"` siguen generando `fmt.FieldText`.

## Documentación

Actualizar `docs/STRUCT_TAGS.md` con la nueva opción `raw` en la tabla de opciones del tag `json:`:

| Opción | Efecto en schema generado |
|---|---|
| `json:"name"` | `Name: "name"`, `Type: fmt.FieldText` |
| `json:"name,omitempty"` | `Name: "name"`, `OmitEmpty: true` |
| `json:"name,raw"` | `Name: "name"`, `Type: fmt.FieldRaw` |
| `json:"name,omitempty,raw"` | `Name: "name"`, `OmitEmpty: true`, `Type: fmt.FieldRaw` |
