# PLAN: Respetar json tag como ColumnName

## Problema

`ormc.go` línea 260 siempre genera `ColumnName` con `SnakeLow()`, ignorando el tag `json:` del campo:

```go
colName := fmt.Convert(fieldName).SnakeLow().String()
```

Si el struct tiene `json:"protocolVersion"`, ese nombre se ignora y el campo queda como `"protocol_version"` en el schema. `tinywasm/json` hace match exacto → incompatibilidad con protocolos externos que usan camelCase.

## Fix

En `ormc.go`, si `jsonTag` tiene nombre (parte antes de la coma, distinto de `"-"`), usarlo como `colName`:

```go
colName := fmt.Convert(fieldName).SnakeLow().String()
if jsonTag != "" {
    name := fmt.Convert(jsonTag).Split(",")[0].String()
    if name != "" && name != "-" {
        colName = name
    }
}
```

El resto del código no cambia. Solo se reemplaza el `colName` derivado si el campo tiene tag json explícito.

## Impacto

- Campos sin tag `json:` → comportamiento actual sin cambios (`SnakeLow`)
- Campos con tag `json:"protocolVersion"` → `ColumnName = "protocolVersion"`
- Los consumidores que necesiten nombres específicos en JSON simplemente añaden el tag en el struct
