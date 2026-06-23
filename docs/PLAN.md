# PLAN — Separar `FieldRaw` de `FieldText` en `ormc/generate.go`

> **Repo:** `github.com/tinywasm/orm`
> **Archivo:** `ormc/generate.go`
> **Tipo:** bug fix en generador de código
> **Prerequisito:** `tinywasm/fmt` publicado con `Raw()` en `FieldWriter`/`FieldReader`

## Contexto

`ormc` genera `EncodeFields`/`DecodeFields` para structs marcados con `// ormc:formonly`.
Los campos de tipo `fmt.RawJSON` se marcan en el schema como `fmt.FieldRaw`, pero el
generador los trata igual que `FieldText` — produciendo `w.String()` / `r.String()`,
lo que **entrecomilla** el JSON inline en lugar de emitirlo directamente.

Esto causa doble-serialización en todos los campos `fmt.RawJSON` del ecosistema
(actualmente, el paquete `tinywasm/mcp` es el principal afectado).

## Bug actual en `generate.go`

### `EncodeFields` (línea ~126)

```go
// ANTES — FieldRaw tratado igual que FieldText:
case fmt.FieldText, fmt.FieldRaw:
    buf.Write(fmt.Sprintf("\tw.String(\"%s\", m.%s)\n", f.ColumnName, f.Name))
```

### `DecodeFields` (línea ~160)

```go
// ANTES:
case fmt.FieldText, fmt.FieldRaw:
    buf.Write(fmt.Sprintf("\tif v, ok := r.String(\"%s\"); ok { m.%s = v }\n", f.ColumnName, f.Name))
```

## Fix requerido

### `EncodeFields` — separar el caso `FieldRaw`

```go
case fmt.FieldText:
    buf.Write(fmt.Sprintf("\tw.String(\"%s\", m.%s)\n", f.ColumnName, f.Name))
case fmt.FieldRaw:
    buf.Write(fmt.Sprintf("\tw.Raw(\"%s\", m.%s)\n", f.ColumnName, f.Name))
```

### `DecodeFields` — separar el caso `FieldRaw`

```go
case fmt.FieldText:
    buf.Write(fmt.Sprintf("\tif v, ok := r.String(\"%s\"); ok { m.%s = v }\n", f.ColumnName, f.Name))
case fmt.FieldRaw:
    buf.Write(fmt.Sprintf("\tif v, ok := r.Raw(\"%s\"); ok { m.%s = v }\n", f.ColumnName, f.Name))
```

## Actualizar dependencia

```bash
go get github.com/tinywasm/fmt@latest
go mod tidy
```

## Verificación

```bash
cd ~/Dev/Project/tinywasm/orm
gotest
```

## Checklist

- [ ] `go get github.com/tinywasm/fmt@latest` actualizado
- [ ] `EncodeFields`: `FieldRaw` separado de `FieldText` → genera `w.Raw()`
- [ ] `DecodeFields`: `FieldRaw` separado de `FieldText` → genera `r.Raw()`
- [ ] `gotest` verde en `tinywasm/orm` (sin dep mcp actualizada aún)
