# PLAN: Corregir pérdida de coma inicial en reescritura de tags json

## Bug encontrado

`ormc` reescribe el tag `json:",omitempty"` como `json:"omitempty"` — elimina la coma inicial.

**Impacto crítico:** Sin la coma inicial, `"omitempty"` deja de ser una *opción* y se convierte en el *nombre del campo JSON*. Cuando dos campos tienen el mismo tag, `go vet` falla con:

```
struct field Error repeats json tag "omitempty" also at model.go:117
```

Esto bloquea `gopush` y rompe compilaciones que corren `go vet`.

## Regla: cuándo va la coma inicial

El formato del tag `json` en Go es: `json:"[nombre][,opción,opción...]"`

| Tag escrito | Significado |
|-------------|-------------|
| `json:"myfield"` | nombre JSON = `myfield`, sin opciones |
| `json:",omitempty"` | nombre JSON = nombre del campo Go (por defecto), opción `omitempty` |
| `json:",raw"` | nombre JSON = nombre del campo Go, opción `raw` |
| `json:",raw,omitempty"` | nombre JSON = nombre del campo Go, opciones `raw` + `omitempty` |
| `json:"myfield,omitempty"` | nombre JSON = `myfield`, opción `omitempty` |

**La coma inicial es obligatoria cuando no se quiere sobreescribir el nombre del campo JSON.**
Si se omite la coma, la opción (`omitempty`, `raw`) se interpreta como el nombre del campo.

## Por qué existe `FieldRaw` — no es un bug, es una consecuencia del diseño

### El problema de fondo

`tinywasm/json` no usa reflexión. En vez de inspeccionar los tipos Go en runtime, usa `Schema()` + `Pointers()` declarados explícitamente por el desarrollador (generados por `ormc`). Esta decisión es **intencional** — reflexión es incompatible con TinyGo/WASM y genera binarios grandes.

### Cómo maneja `encoding/json` este problema

La stdlib tiene `json.RawMessage` (`[]byte`): si un campo es de ese tipo, el serializer lo emite directamente como JSON inline sin quotes. Lo puede hacer porque **tiene reflexión** — sabe en runtime que el tipo es `json.RawMessage` y lo trata de forma especial.

```go
// stdlib: funciona sin declaración extra
type Response struct {
    Result json.RawMessage `json:"result"` // emitido inline automáticamente
}
```

### Por qué tinywasm/json necesita `FieldRaw` declarado

Sin reflexión, el serializer solo conoce los tipos primitivos del Schema (`FieldText`, `FieldInt`, `FieldBool`...). Un campo `string` que contiene JSON pre-serializado es **indistinguible** de un string normal — ambos son `string` en Go.

```go
// Sin FieldRaw: tinywasm/json no sabe que este string es JSON
Result string  // podría ser "hello" o {"key":"value"} — no hay diferencia de tipo
```

`FieldRaw` es el mecanismo explícito que le dice al serializer: _"este string no es texto, es JSON crudo — emítelo sin quotes"_. Es el equivalente a `json.RawMessage` pero declarado en el Schema en vez de en el tipo.

### ¿Es una falla de diseño?

No es un bug — es el **precio del diseño sin reflexión**. La stdlib puede inferir comportamiento especial del tipo (`RawMessage`). tinywasm/json no puede, así que requiere declaración explícita (`FieldRaw`).

El trade-off es explícito en [WHY.md](WHY.md):
> _"More Explicit Code: Models must define their schema manually, which adds some boilerplate but improves performance and clarity."_

`FieldRaw` es una instancia de ese boilerplate explícito.

### ¿Es suficiente `FieldRaw`?

Sí. `FieldRaw` en `tinywasm/fmt` (`// Pre-serialized JSON — emitted inline, no quoting`) es la solución correcta y completa para este problema dentro del modelo sin reflexión. No es un workaround ni una solución temporal — es el mecanismo explícito diseñado exactamente para este caso.

El único trabajo pendiente es corregir el bug en `ormc` que elimina la coma inicial al reescribir los tags, para que `FieldRaw` pueda declararse sin friction y sin romper `go vet`.

---

## Compatibilidad con `encoding/json` (stdlib)

`raw` es una opción **exclusiva de tinywasm/orm** — la librería estándar no la reconoce.

Comportamiento al usar `json:",raw"` con cada serializador:

| Serializador | Comportamiento con `json:",raw"` |
|---|---|
| `encoding/json` (stdlib) | Ignora la opción `raw`, serializa el campo como string normal → **double-encoding** |
| `tinywasm/json` | Detecta `FieldRaw` en Schema(), emite el string como JSON inline → **sin double-encoding** |

`go vet` **no genera warnings** por `raw` — lo trata como opción desconocida pero válida.

**Consecuencia:** los structs que usan `json:",raw"` no son compatibles con `encoding/json` si el campo contiene JSON pre-serializado. Esto es intencional — `tinywasm/mcp` nunca usa `encoding/json` en el path de serialización de respuestas.

Si algún consumidor necesita usar `encoding/json` con estos tipos, debe manejar el campo como `json.RawMessage` en vez de `string`.

## Decisión: ¿debe ormc agregar la coma automáticamente?

**Sí.** ormc debe agregar la coma inicial automáticamente cuando el nombre del campo JSON está vacío (se usa el nombre Go por defecto) y hay opciones (`omitempty`, `raw`).

Regla de reescritura en `rewriteRawTag`:
- Si el tag tiene nombre de campo explícito → mantener tal cual: `json:"myfield,omitempty"`
- Si el tag **no** tiene nombre de campo → agregar coma inicial: `json:",omitempty"`, `json:",raw"`, `json:",raw,omitempty"`
- Si el campo no tiene tag json pero ormc detecta opciones (OmitEmpty, FieldRaw) → generar tag con coma inicial

## Reproducción del bug actual

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

En `ormc_tags.go`, la función `rewriteRawTag` no detecta si el primer segmento del tag es un nombre de campo o una opción. Cuando el primer segmento es vacío (coma inicial), lo elimina.

## Corrección en `rewriteRawTag`

```go
// Leer el nombre de campo del tag original
parts := strings.SplitN(originalTag, ",", 2)
fieldName := parts[0]  // "" si tiene coma inicial, "myfield" si tiene nombre

// Al reconstruir, preservar el nombre (vacío o no)
newTag := fieldName + "," + strings.Join(options, ",")
// Si fieldName es "" → resultado: ",omitempty,raw"  ✅
// Si fieldName es "myfield" → resultado: "myfield,omitempty,raw"  ✅
```

## Verificación

Después del fix, el siguiente struct debe pasar `go vet` sin warnings:

```go
type Foo struct {
    A string `json:",omitempty"`
    B string `json:",omitempty"`
    C string `json:",raw,omitempty"`
    D string `json:",raw,omitempty"`
    E string `json:"myfield,omitempty"`
}
```

```bash
ormc .
go vet ./...  # debe pasar sin errores ni warnings de duplicate json tag
```

## Workaround actual (tinywasm/mcp)

Mientras el bug no esté corregido, los campos `FieldRaw` en `JSONRPCResponseStruct` se mantienen sin struct tag y se edita `model_orm.go` manualmente para establecer `FieldRaw`. **Esto se pierde si se vuelve a correr `ormc .`** — no correr ormc en `tinywasm/mcp` hasta que este fix esté publicado.
