# PLAN: Corregir pérdida de coma inicial en reescritura de tags json

## Bug encontrado

`ormc` reescribe el tag `json:",omitempty"` como `json:"omitempty"` — elimina la coma inicial.

**Impacto crítico:** Sin la coma inicial, `"omitempty"` deja de ser una *opción* y se convierte en el *nombre del campo JSON*. Cuando dos campos tienen el mismo tag, `go vet` falla con:

```
struct field Error repeats json tag "omitempty" also at model.go:117
```

Esto bloquea `gopush` y rompe compilaciones que corren `go vet`.

## Reproducción

Dado el struct en `model.go`:
```go
type JSONRPCResponseStruct struct {
    Result string `json:",omitempty,raw"`
    Error  string `json:",omitempty,raw"`
}
```

Después de `ormc .` el archivo queda:
```go
type JSONRPCResponseStruct struct {
    Result string `json:"omitempty,raw"`  // ❌ coma eliminada
    Error  string `json:"omitempty,raw"`  // ❌ duplicate json tag "omitempty"
}
```

## Causa raíz

En `ormc_tags.go`, la función `rewriteRawTag` no preserva la coma inicial cuando el nombre del campo JSON es vacío (usa el nombre del campo Go por defecto).

## Corrección sugerida

En `rewriteRawTag` (o donde se construya el tag de salida), detectar si el nombre de campo era vacío en el tag original y preservar la coma inicial en la salida:

```go
// Antes (comportamiento actual):
// json:",omitempty" → json:"omitempty"

// Después (comportamiento correcto):
// json:",omitempty"     → json:",omitempty"
// json:",raw,omitempty" → json:",raw,omitempty"
// json:"myfield"        → json:"myfield"        (sin cambio)
```

## Verificación

Después del fix, el siguiente struct debe pasar `go vet` sin warnings:

```go
type Foo struct {
    A string `json:",omitempty"`
    B string `json:",omitempty"`
    C string `json:",raw,omitempty"`
    D string `json:",raw,omitempty"`
}
```

```bash
go vet ./...  # debe pasar sin errores
```

actueliza la docuemntacionnación para reflejar el cambio y evitar confusiones futuras sobre el formato correcto de los tags JSON.
