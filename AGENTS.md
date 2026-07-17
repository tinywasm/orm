# AGENTS.md — tinywasm/orm

Working notes for AI agents operating in this library. For end-user docs see [README.md](README.md). For tag reference see [docs/](docs/).

## Mission of this package

`tinywasm/orm` provides:

1. **A runtime DML ORM** (`db.go`, `tx.go`, `query.go`, `executor.go`, `qb.go`, `conditions.go`) — `*orm.DB`, query builder, `Create`/`Update`/`Delete`/`Query(ReadOne/ReadAll)`. Isomorphic: compiles for Go and WASM.

The runtime is reflection-free — `Fielder` interface (defined in `tinywasm/fmt`) is the only contract. All struct introspection happens at codegen time.

**DML/DDL split (2026-07-16).** `orm` is DML-only. Schema management (`CreateTable`/`DropTable`/
`CreateDatabase`/`Sync`/`SyncSchema`, `SchemaInspector`, `TableIntrospector`) lives in the sibling repo
[`tinywasm/ddl`](https://github.com/tinywasm/ddl). Do not add DDL surface back to this package — `orm`
never depends on `ddl`/`ddlc` (one-directional). See `docs/ARQUITECTURE.md`.

## Architectural rules (do not violate)

### No Go `map` anywhere in this ecosystem

**Never use a built-in `map[K]V`, in any file, wasm-gated or not.** TinyGo's map runtime is heavy and
adds meaningful, unavoidable size to every wasm binary that ends up importing this code — and because
`orm`/`mock` are meant to be imported by wasm frontends (leaf modules use `mock.NewDB()` to test
round-trips without a real driver), there is no "backend-only" escape hatch via `//go:build !wasm`
anymore: the map has to not exist at all, not just be excluded from one build target.

- For a **string→string** pair, use `github.com/tinywasm/fmt.KeyValue{Key, Value string}`.
- For anything else (typed values, non-string keys, or an in-memory "table" of rows), use a small
  local slice-of-structs and scan it linearly — see `mock/memdb.go`'s `dbCell`/`dbRow`/`dbTable` and
  `open.go`'s `factoryEntry`/`registry` for the pattern. These collections are always small (a handful
  of registered adapters, a handful of schema columns), so a linear scan costs nothing in practice and
  it's what the whole ecosystem already does (e.g. `tinywasm/fmt.TagPairs` returns `[]KeyValue`, not a
  map).
- If you're tempted to add a map "just for a lookup cache," don't — reach for a linear scan first, and
  only reconsider with a profiler backing you up, never on a hunch.

### Root package (`orm`) — isomorphic, zero dialect

- **No `database/sql` import in the root package.** The root `orm` package compiles for both Go and WASM. Never import `database/sql`, `database/sql/driver`, or any DB driver. Use only `github.com/tinywasm/fmt` and the stdlib.
- **Agnostic API only.** `query.go`, `qb.go`, `db.go`, `executor.go` must never contain dialect-specific SQL, driver types, or engine-specific error values (e.g. `sql.ErrNoRows`).
- **Use `orm`-owned sentinels.** `errors.go` defines `ErrNotFound`, `ErrNoRows`, `ErrNoTxSupport`. `qb.go` compares against these. **Executor adapters** (`tinywasm/postgres`, `tinywasm/sqlt`) are responsible for mapping their driver-specific errors (e.g. `sql.ErrNoRows`) to `orm.ErrNoRows` inside their `Scanner` implementation.
- **Executor contract.** Adapters implementing `orm.Executor` must: (1) wrap `sql.ErrNoRows` → `orm.ErrNoRows`; (2) never leak `database/sql` types into `orm` core.
- **Do not use `tinygo` as a build tag** — it is not a standard Go constraint recognized by the Go toolchain. Use `GOOS=js GOARCH=wasm` to build for wasm, and `gotest -tinygo` to test against the TinyGo compiler specifically.
- **Struct tags are processed at build-time, not runtime.** `fmt.Field` has no `Tag` field. If you ever feel you need to inspect a tag in WASM code, you are wrong — push the work into `ormc`.

## Code layout

| File / Dir | Role |
|------------|------|
| `db.go`, `tx.go` | `*orm.DB`, `*orm.Tx` — DML runtime entry points (`Create`/`Update`/`Delete`/`Query`) |
| `query.go`, `qb.go`, `conditions.go` | Query builder (`Where`, `Like`, `Limit`, ...) |
| `executor.go`, `execution_plan.go` | Query execution; `Rows` interface includes `Columns() ([]string, error)` |
| `validate.go` | Runtime validation glue (delegates to `fmt.ValidateFields`) |
| `open.go` | `Register`/`Open` — DSN-scheme → `Factory` registry (slice-based, no map) |
| `conformance/` | Executable DML contract (`Run(t, Factory)`) every backend proves itself against |
| `mock/` | Recorders (`mock.Executor`, etc.) + `mock.NewDB()` in-memory engine — no driver, no map |
| `tests/` | Test fixtures + `_test.go` files (separate module with `replace ../`) |
| `docs/` | Architecture, design rationale, tag reference |

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

## Common mistakes to avoid

- Trying to parse struct tags from `fmt.Field` at runtime → impossible, the field has no `Tag`.
- Writing into `model_orm.go` by hand → always run `ormc`.
- Reaching for `map[K]V` for a lookup table or registry → use `fmt.KeyValue` (string/string) or a small
  local slice-of-structs scanned linearly instead. No exceptions, no `//go:build !wasm` escape hatch.
