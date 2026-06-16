# AGENTS.md — tinywasm/orm

Working notes for AI agents operating in this library. For end-user docs see [README.md](README.md). For tag reference see [docs/](docs/).

## Mission of this package

`tinywasm/orm` provides:

1. **A runtime ORM** (`db.go`, `tx.go`, `sync.go`, `query.go`, `executor.go`, `qb.go`, `conditions.go`) — `*orm.DB`, query builder, CRUD, and dev-time schema sync (`db.Sync`). Works in Go and WASM (sync is backend-only).
2. **`ormc`, a build-time code generator** (located in `ormc/`, `cmd/ormc/`) — parses Go source files, reads struct tags (`db:`, `json:`, `input:`), emits `model_orm.go` files next to each source with:
3. **`ormcp`, an MCP tool provider** (located in `ormcp/`) — backend-only subpackage that exposes `db_schema`, `db_query`, and `db_exec` MCP tools for LLM interaction with a live `*orm.DB` during development. Register via `ormcp.NewProvider(db)`. `db_schema` is only available if the executor implements `orm.SchemaInspector`.
   - `Schema() []fmt.Field` — runtime schema with `Widget`, `Permitted`, `DB`, etc.
   - `ModelName()`, `<Name>_` table tokens, `FieldDB` constants
   - `ReadOne*`, `ReadAll*` typed query helpers
   - `Validate(action byte)` for CRUD validation

The runtime is reflection-free — `Fielder` interface (defined in `tinywasm/fmt`) is the only contract. All struct introspection happens at codegen time.

## Architectural rules (do not violate)

### Root package (`orm`) — isomorphic, zero dialect

- **No `database/sql` import in the root package.** The root `orm` package compiles for both Go and WASM. Never import `database/sql`, `database/sql/driver`, or any DB driver. Use only `github.com/tinywasm/fmt` and the stdlib.
- **Agnostic API only.** `query.go`, `qb.go`, `db.go`, `executor.go`, `sync.go` must never contain dialect-specific SQL, driver types, or engine-specific error values (e.g. `sql.ErrNoRows`).
- **Use `orm`-owned sentinels.** `errors.go` defines `ErrNotFound`, `ErrNoRows`, `ErrSyncFailed`, etc. `qb.go` compares against these. **Executor adapters** (`tinywasm/postgres`, `tinywasm/sqlt`) are responsible for mapping their driver-specific errors (e.g. `sql.ErrNoRows`) to `orm.ErrNoRows` inside their `Scanner` implementation.
- **Executor contract.** Adapters implementing `orm.Executor` must: (1) wrap `sql.ErrNoRows` → `orm.ErrNoRows`; (2) never leak `database/sql` types into `orm` core.
- **`//go:build !wasm` on tool-side files.** Files that are **never needed by the WASM target** (e.g. `open.go` with the engine registry, `sync.go` with `db.Sync`/`db.SyncSchema`) must carry a `//go:build !wasm` build constraint at the top. The standard Go build tag for WebAssembly is `GOARCH=wasm` which activates the `wasm` constraint; `!wasm` excludes the file from WASM builds. Reason: `open.go` contains `var registry = make(map[string]Factory)` — a package-level init that always runs, even if no WASM code ever calls `orm.Open`. Map support in WASM adds meaningful binary overhead. `sync.go` likewise uses `make(map[string]bool)` twice and is entirely tool-side (WASM code never calls `db.Sync`). The runtime files (`db.go`, `qb.go`, `conditions.go`, `executor.go`) remain unconstrained. **Do not move these tool-side files to `ormc`** — `ormc` must not import the `orm` runtime (cycle risk), and `orm.Open` must stay in the `orm` package so the tool gets `*orm.DB` back without a subpackage import. **Do not use `tinygo` as a build tag** — it is not a standard Go constraint recognized by the Go toolchain.
- **Struct tags are processed at build-time, not runtime.** `fmt.Field` has no `Tag` field. If you ever feel you need to inspect a tag in WASM code, you are wrong — push the work into `ormc`.
- **`ormc` is the single producer of `fmt.Field` literals.** Hand-writing `Schema()` is allowed only in tests.

### `ormc` subpackage — backend-only codegen

- **`orm/ormc` is a separate Go subpackage** (package `ormc`). It is backend-only by nature: its stdlib deps (`go/ast`, `go/parser`, `os/exec`) make it impossible to import from WASM — no `//go:build` tags needed or allowed in the subpackage.
- **`orm/ormc` does not import the root `orm` package.** It emits runtime type names as string literals inside generated code (`"orm.QB"`, `"orm.DB"`). This keeps the dependency graph cycle-free.
- **No new constructors per flag combination.** If a tag changes one boolean, expose a setter on the widget (`SetTilde(bool) *text`) and have `ormc` emit it. Don't create `TextNoTilde()`, `TextNoTildeNoSpaces()`, etc.
- **Directives are orthogonal and composable.** Use atomic directives: `orm:form_widgets` for the form layer, `orm:no_db` for suppressing DB helpers, `orm:typed_fields` for field accessors.
- **One source of truth per concern.** Widget defaults live in the widget. Tag → setter mapping lives in `ormc`. Validation rules live in `fmt.Permitted`. Do not duplicate.

## Code layout

| File / Dir | Role |
|------------|------|
| `db.go`, `tx.go`, `sync.go` | `*orm.DB`, `*orm.Tx`, `db.Sync` — runtime entry points |
| `query.go`, `qb.go`, `conditions.go` | Query builder (`Where`, `Like`, `Limit`, ...) |
| `executor.go`, `execution_plan.go` | Query execution; `Rows` interface includes `Columns() ([]string, error)` |
| `schema.go` (`//go:build !wasm`) | `SchemaInspector` interface + `ColumnInfo` struct — implemented by DB adapters (sqlite, postgres) |
| `validate.go` | Runtime validation glue (delegates to `fmt.ValidateFields`) |
| `field_ext.go` | Field utilities used by generated code |
| `ormc/` | Codegen subpackage |
| `ormc/generator.go` | `Generator` type, file walker, top-level codegen orchestrator |
| `ormc/generate.go` | Emits `model_orm.go` content (Schema, helpers, validation) |
| `ormc/tags.go` | Parses `db:`, `json:`, `input:` tags into `FieldInfo` |
| `ormc/handler.go` | Project-wide handler registration |
| `ormc/relations.go` | Foreign-key resolution between structs |
| `ormc/watch.go` | File-event watcher handler |
| `ormcp/` | MCP tool provider subpackage (backend-only) |
| `ormcp/provider.go` | `Provider`, `NewProvider(*orm.DB)`, `Tools()`, `encodeSchema`, `scanRowsToText` |
| `ormcp/models.go` | `QueryArgs`, `ExecArgs` — manual `Schema()`/`Validate()` (no ormc, avoids spurious table sync) |
| `ormcp/tool_schema.go` | `db_schema` tool — lists tables + columns (requires `SchemaInspector`) |
| `ormcp/tool_query.go` | `db_query` tool — SELECT/WITH only |
| `ormcp/tool_exec.go` | `db_exec` tool — INSERT/UPDATE/DELETE/DDL |
| `cmd/ormc/` | CLI entrypoint |
| `tests/` | Test fixtures + `_test.go` files (separate module with `replace ../`) |
| `docs/` | Architecture, design rationale, tag reference (see `STRUCT_TAGS.md` mirror in `fmt/docs`) |

## ormcp — reglas

- **No usar `ormc` en `ormcp/models.go`.** El generador emite `ModelName()` aunque se use `// orm:no_db`, lo que haría que `ScanModules` intentara crear tablas `query_args`/`exec_args` espurias en la DB del usuario. `Schema()` y `Validate()` se implementan a mano — los structs son triviales (1 campo).
- **Sin `encoding/json` ni `strings`.** Usar `fmt.Convert()`, `fmt.HasPrefix()`, `fmt.Convert(slice).Join()` del ecosistema.
- **Sin parámetros SQL (`Args`).** El LLM escribe SQL completo con valores embebidos. Los parámetros existen para proteger input de usuarios externos — no aplica en contexto de desarrollo.

## How `ormc` adds support for a new `input:` tag

1. **If the tag is aditive (length, presence)** → maps to `fmt.Permitted` or `Field.NotNull`:
   - Add the keyword to `isModifier()` in `ormc.go` (or wherever lives).
   - Extend `parseInputModifiers()` to write into `FieldInfo` / `Permitted`.

2. **If the tag is sustractive (revokes a widget default, e.g. `notilde`)** → maps to a widget setter call:
   - Add a setter to the widget (`func (t *text) SetTilde(v bool) *text` in `tinywasm/form/input/text.go`).
   - Add entry to `tagSetters` map in `ormc.go` (e.g. `"notilde": ".SetTilde(false)"`).
   - The emitted Widget literal becomes `input.Text().SetTilde(false)`.
   - Document the tag in `tinywasm/form/docs/TAGS.md`.

3. **If the tag selects a widget type** → add to the widget-type table (`inputWidgets` map in `ormc.go`).

Always update [`tinywasm/form/docs/TAGS.md`](../form/docs/TAGS.md) — single user-facing catalog.

## Testing

Install once:

```bash
go install github.com/tinywasm/devflow/cmd/gotest@latest
```

Run:

```bash
gotest              # vet + race + cover + wasm + badges
gotest -no-cache    # force re-run
gotest -run TestX   # filter
```

`ormc` tests use the `tests/` directory: fixture structs in `.go` files, golden expectations in `*_test.go`. Treat generated `model_orm.go` files inside `tests/` as test artifacts.

## Publishing

```bash
gopush
```

Handles git commit, tag, push, dependency bumps. Coordinate cross-package: if a change requires updating widgets in `tinywasm/form/input` first, publish that, then publish `tinywasm/orm`.

## Common mistakes to avoid

- Trying to parse struct tags from `fmt.Field` at runtime → impossible, the field has no `Tag`.
- Duplicating widget defaults between `ormc` and the widget → keep them in the widget; if `ormc` needs them, expose via getter or hardcode in a clearly-named table with a `// keep in sync with X` comment.
- Adding fields to `fmt.Permitted` to express widget-specific overrides → use a widget setter instead.
- Emitting `Widget` and `Permitted` redundantly (e.g. encoding char rules into both) → `Permitted` carries additive validation (length, extra forbidden substrings); the widget owns its char defaults and is mutated via setters.
- Writing into `model_orm.go` by hand → always run `ormc`.

## Related packages

- [`tinywasm/fmt`](https://github.com/tinywasm/fmt) — `Field`, `Permitted`, `Widget` interface, `ValidateFields`. Schema contracts live here.
- [`tinywasm/form`](https://github.com/tinywasm/form) — UI runtime: widgets, mount, render. ormc emits `input.X()` constructors from this package.
- [`tinywasm/devflow`](https://github.com/tinywasm/devflow) — `gotest`, `gopush` tooling.
