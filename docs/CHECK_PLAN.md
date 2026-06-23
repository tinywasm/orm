# PLAN — `ormc`: fix generación de `EncodeFields`/`DecodeFields` para campos struct-valor y type aliases

> **Repo objetivo:** `github.com/tinywasm/orm`
> **Archivo afectado:** `ormc/generator.go` + `ormc/generate.go` (+ test)
> **Tipo:** bug fix en el generador de código
> **Prerequisito:** `gotest` (no `go test`)

## Contexto

`ormc` genera `EncodeFields`/`DecodeFields` incorrectos en dos casos:

### Bug 1 — Campos struct por valor (no puntero)

Dado:
```go
type errorResponse struct {
    Error jsonRPCError  // valor, no *jsonRPCError
}
```

`ormc` genera:
```go
// INCORRECTO — nil check inválido para tipo valor
if m.Error != nil { w.Object("error", m.Error) } else { w.Null("error") }
// decode:
if m.Error == nil { m.Error = new(jsonRPCError) }
if !r.Object("error", m.Error) { m.Error = nil }
```

Debe generar:
```go
// CORRECTO — puntero al campo valor
w.Object("error", &m.Error)
// decode:
r.Object("error", &m.Error)
```

**Raíz:** `FieldInfo` no almacena si el campo es puntero (`IsPointer bool`). El generador asume siempre que `FieldStruct` es puntero.

### Bug 2 — Type aliases de primitivos

Dado:
```go
type RequestId = string  // alias, no named type

type JSONRPCRequest struct {
    ID RequestId
}
```

`ormc` produce `typeStr = "RequestId"`, no hace match en el `switch`, cae al `default` y genera `FieldStruct`. Debe producir `FieldText`.

**Raíz:** el parser lee el nombre del tipo pero no resuelve aliases del mismo paquete.

## Fix (dos cambios en `ormc/generator.go` + uno en `ormc/generate.go`)

### 1. Agregar `IsPointer bool` a `FieldInfo` (`generator.go`)

```go
type FieldInfo struct {
    // ...
    IsPointer bool  // true si el campo original es *T (solo aplica a FieldStruct)
    // ...
}
```

Propagarlo al crear `fi`:
```go
fi := FieldInfo{
    // ...
    IsPointer: isPointer,
    // ...
}
```

### 2. Resolver type aliases antes del switch (`generator.go`)

Justo antes del `switch typeStr`, buscar si `typeStr` es un alias de tipo primitivo declarado en el archivo actual via `ast.Inspect`:

```go
typeStr = resolveTypeAlias(node, typeStr)
```

Implementar `resolveTypeAlias(node *ast.File, name string) string`:
- Recorre `node.Decls` buscando `*ast.GenDecl` con specs `*ast.TypeSpec`
- Si encuentra `type name = underlying` donde `underlying` es un `*ast.Ident`, devuelve `underlying.Name`
- Si no encuentra, devuelve `name` sin cambios
- Solo resuelve un nivel (no recursivo — los aliases de aliases son raros y no se usan en el ecosistema)

### 3. Usar `IsPointer` en la generación de `EncodeFields`/`DecodeFields` (`generate.go`)

Para `FieldStruct` en `EncodeFields`:
```go
case fmt.FieldStruct:
    if f.IsPointer {
        buf.Write(fmt.Sprintf("\tif m.%s != nil { w.Object(\"%s\", m.%s) } else { w.Null(\"%s\") }\n", f.Name, f.ColumnName, f.Name, f.ColumnName))
    } else {
        buf.Write(fmt.Sprintf("\tw.Object(\"%s\", &m.%s)\n", f.ColumnName, f.Name))
    }
```

Para `FieldStruct` en `DecodeFields`:
```go
case fmt.FieldStruct:
    elemType := f.GoType
    if f.IsPointer {
        buf.Write(fmt.Sprintf("\tif m.%s == nil { m.%s = new(%s) }\n", f.Name, f.Name, elemType))
        buf.Write(fmt.Sprintf("\tif !r.Object(\"%s\", m.%s) { m.%s = nil }\n", f.ColumnName, f.Name, f.Name))
    } else {
        buf.Write(fmt.Sprintf("\tr.Object(\"%s\", &m.%s)\n", f.ColumnName, f.Name))
    }
```

## Test a agregar

En `ormc/generator_test.go` (o el archivo de test existente), agregar casos:

```go
// Bug 1: campo struct por valor
type parentValue struct {
    Name  string
    Child childStruct  // valor, no puntero
}
type childStruct struct { X string }

// Bug 2: type alias
type MyString = string
type withAlias struct {
    ID MyString
}
```

Verificar que:
- `parentValue` genera `w.Object("child", &m.Child)` (sin nil check)
- `withAlias` genera `FieldText` para `ID` (no `FieldStruct`)

## Verificación

```bash
# 1. Correr tests de ormc:
gotest

# 2. Regenerar mcp y verificar que compila:
cd ~/Dev/Project/tinywasm/mcp && ormc && go vet ./...
```

## Checklist

- [ ] `IsPointer bool` en `FieldInfo`, propagado al `fi` en `ParseStruct`
- [ ] `resolveTypeAlias` implementado; llamado antes del `switch typeStr`
- [ ] `generate.go` usa `f.IsPointer` para el caso `FieldStruct`
- [ ] Test cubre ambos bugs (campo valor + alias de string)
- [ ] `gotest` verde en `tinywasm/orm`
- [ ] `ormc` en `tinywasm/mcp` produce código que compila sin errores (`go vet ./...`)
