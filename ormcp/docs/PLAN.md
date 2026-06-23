# PLAN — Extraer `ormcp` a módulo independiente

> **Repo:** `github.com/tinywasm/orm` (sub-módulo `ormcp`)
> **Tipo:** refactor arquitectónico — romper dependencia circular
> **Prerequisito:** `tinywasm/orm` publicado sin dep `mcp`

## Problema actual

`ormcp` vive dentro del módulo `github.com/tinywasm/orm`, lo que obliga a que
`orm/go.mod` declare `require github.com/tinywasm/mcp`. Esto crea una dependencia
inversa: `orm → mcp`, cuando la dirección natural es `mcp → orm` (mcp usa ormc
para generar código).

`ormcp` es un **adaptador**: traduce operaciones ORM al protocolo MCP. Por naturaleza
debe conocer ambos lados. El error es alojar ese adaptador en el módulo upstream (`orm`).

## Solución: `orm/ormcp` como módulo Go separado

Igual que `orm/tests` ya tiene su propio `go.mod`, `ormcp` tendrá el suyo.

### Estructura resultante

```
tinywasm/orm/
├── go.mod                 ← module github.com/tinywasm/orm  (sin mcp)
├── ormc/
├── ormcp/
│   ├── go.mod             ← module github.com/tinywasm/orm/ormcp
│   │                         require orm + mcp
│   ├── daemon_provider.go
│   ├── provider.go
│   ├── tool_exec.go
│   ├── tool_query.go
│   ├── tool_schema.go
│   └── daemon_test.go
└── tests/
    └── go.mod             ← module github.com/tinywasm/orm/tests (ya existe)
```

## Pasos

### 1. Crear `orm/ormcp/go.mod`

```
module github.com/tinywasm/orm/ormcp

go 1.25.2

require (
    github.com/tinywasm/orm  v0.x.x   // versión nueva sin dep mcp
    github.com/tinywasm/mcp  v0.x.x   // versión nueva con Raw() fix
)
```

### 2. Eliminar `ormcp/` del módulo raíz `orm/go.mod`

Quitar de `orm/go.mod`:
```
- github.com/tinywasm/mcp v0.x.x
- github.com/tinywasm/context
- github.com/tinywasm/fetch
- github.com/tinywasm/unixid
```
(y cualquier otro transitivo que solo llegaba vía mcp)

### 3. Ajustar imports en `ormcp/*.go`

Los archivos Go no cambian. Solo cambia el módulo al que pertenecen.
Los imports `"github.com/tinywasm/mcp"` y `"github.com/tinywasm/orm/..."` siguen igual.

### 4. Verificar módulo raíz `orm` sin dep `mcp`

```bash
cd ~/Dev/Project/tinywasm/orm
go mod tidy
gotest   # debe pasar sin mcp en go.mod
```

### 5. Verificar módulo adaptador `orm/ormcp`

```bash
cd ~/Dev/Project/tinywasm/orm/ormcp
go mod tidy
gotest
```

### 6. Publicar ambos módulos

```bash
# Raíz primero (sin mcp)
cd ~/Dev/Project/tinywasm/orm
gopush

# Luego el adaptador
cd ~/Dev/Project/tinywasm/orm/ormcp
gopush
```

## Checklist

- [ ] `orm/ormcp/go.mod` creado con `require orm + mcp`
- [ ] `mcp` y sus transitivos eliminados de `orm/go.mod`
- [ ] `go mod tidy` limpio en módulo raíz `orm`
- [ ] `gotest` verde en módulo raíz `orm`
- [ ] `go mod tidy` limpio en `orm/ormcp`
- [ ] `gotest` verde en `orm/ormcp`
- [ ] Ambos módulos publicados con `gopush`
