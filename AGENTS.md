# AGENTS.md — tinywasm/orm

Working notes for AI agents operating in this library. For end-user docs see [README.md](README.md). For tag reference see [docs/](docs/).

## Mission of this package

`tinywasm/orm` provides:

1. **A runtime ORM** (`db.go`, `tx.go`, `sync.go`, `query.go`, `executor.go`, `qb.go`, `conditions.go`) — `*orm.DB`, query builder, CRUD, and dev-time schema sync (`db.Sync`). Works in Go and WASM (sync is backend-only).

The runtime is reflection-free — `Fielder` interface (defined in `tinywasm/fmt`) is the only contract. All struct introspection happens at codegen time.

## Architectural rules (do not violate)

### Root package (`orm`) — isomorphic, zero dialect

- **No `database/sql` import in the root package.** The root `orm` package compiles for both Go and WASM. Never import `database/sql`, `database/sql/driver`, or any DB driver. Use only `github.com/tinywasm/fmt` and the stdlib.
- **Agnostic API only.** `query.go`, `qb.go`, `db.go`, `executor.go`, `sync.go` must never contain dialect-specific SQL, driver types, or engine-specific error values (e.g. `sql.ErrNoRows`).
- **Use `orm`-owned sentinels.** `errors.go` defines `ErrNotFound`, `ErrNoRows`, `ErrSyncFailed`, etc. `qb.go` compares against these. **Executor adapters** (`tinywasm/postgres`, `tinywasm/sqlt`) are responsible for mapping their driver-specific errors (e.g. `sql.ErrNoRows`) to `orm.ErrNoRows` inside their `Scanner` implementation.
- **Executor contract.** Adapters implementing `orm.Executor` must: (1) wrap `sql.ErrNoRows` → `orm.ErrNoRows`; (2) never leak `database/sql` types into `orm` core.
- **`//go:build !wasm` on tool-side files.** Files that are **never needed by the WASM target** (e.g. `open.go` with the engine registry, `sync.go` with `db.Sync`/`db.SyncSchema`) must carry a `//go:build !wasm` build constraint at the top. The standard Go build tag for WebAssembly is `GOARCH=wasm` which activates the `wasm` constraint; `!wasm` excludes the file from WASM builds. Reason: `open.go` contains `var registry = make(map[string]Factory)` — a package-level init that always runs, even if no WASM code ever calls `orm.Open`. Map support in WASM adds meaningful binary overhead. `sync.go` likewise uses `make(map[string]bool)` twice and is entirely tool-side (WASM code never calls `db.Sync`). The runtime files (`db.go`, `qb.go`, `conditions.go`, `executor.go`) remain unconstrained. **Do not use `tinygo` as a build tag** — it is not a standard Go constraint recognized by the Go toolchain.
- **Struct tags are processed at build-time, not runtime.** `fmt.Field` has no `Tag` field. If you ever feel you need to inspect a tag in WASM code, you are wrong — push the work into `ormc`.

## Code layout

| File / Dir | Role |
|------------|------|
| `db.go`, `tx.go`, `sync.go` | `*orm.DB`, `*orm.Tx`, `db.Sync` — runtime entry points |
| `query.go`, `qb.go`, `conditions.go` | Query builder (`Where`, `Like`, `Limit`, ...) |
| `executor.go`, `execution_plan.go` | Query execution; `Rows` interface includes `Columns() ([]string, error)` |
| `schema.go` (`//go:build !wasm`) | `SchemaInspector` interface + `ColumnInfo` struct — implemented by DB adapters (sqlite, postgres) |
| `validate.go` | Runtime validation glue (delegates to `fmt.ValidateFields`) |
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
