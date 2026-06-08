# AGENTS.md — tinywasm/orm

Working notes for AI agents operating in this library. For end-user docs see [README.md](README.md). For tag reference see [docs/](docs/).

## Mission of this package

`tinywasm/orm` provides:

1. **A runtime ORM** (`db.go`, `query.go`, `executor.go`, `qb.go`, `conditions.go`, `tx.go`) — `*orm.DB`, query builder, CRUD. Works in Go and WASM.
2. **`ormc`, a build-time code generator** (`ormc*.go`, `cmd/ormc/`) — parses Go source files, reads struct tags (`db:`, `json:`, `input:`), emits `model_orm.go` files next to each source with:
   - `Schema() []fmt.Field` — runtime schema with `Widget`, `Permitted`, `DB`, etc.
   - `ModelName()`, `<Name>_` table tokens, `FieldDB` constants
   - `ReadOne*`, `ReadAll*` typed query helpers
   - `Validate(action byte)` for CRUD validation

The runtime is reflection-free — `Fielder` interface (defined in `tinywasm/fmt`) is the only contract. All struct introspection happens at codegen time.

## Architectural rules (do not violate)

- **Struct tags are processed at build-time, not runtime.** `fmt.Field` has no `Tag` field. If you ever feel you need to inspect a tag in WASM code, you are wrong — push the work into `ormc`.
- **`ormc` is the single producer of `fmt.Field` literals.** Hand-writing `Schema()` is allowed only in tests.
- **One source of truth per concern.** Widget defaults live in the widget. Tag → setter mapping lives in `ormc`. Validation rules live in `fmt.Permitted`. Do not duplicate.
- **Build tag discipline.** `ormc*.go` files are `//go:build !wasm` — they import `go/ast`, `go/parser`, etc. Never call them from WASM.
- **No new constructors per flag combination.** If a tag changes one boolean, expose a setter on the widget (`SetTilde(bool) *text`) and have `ormc` emit it. Don't create `TextNoTilde()`, `TextNoTildeNoSpaces()`, etc. — it doesn't scale.
- **Directives are orthogonal and composable.** Avoid monolithic directives like `formonly`. Use atomic ones: `orm:form_widgets` for the form layer, `orm:no_db` for suppressing DB helpers, `orm:typed_fields` for field accessors.

## Code layout

| File / Dir | Role |
|------------|------|
| `db.go`, `tx.go` | `*orm.DB`, `*orm.Tx` — runtime entry points |
| `query.go`, `qb.go`, `conditions.go` | Query builder (`Where`, `Like`, `Limit`, ...) |
| `executor.go`, `execution_plan.go` | Query execution |
| `validate.go` | Runtime validation glue (delegates to `fmt.ValidateFields`) |
| `field_ext.go` | Field utilities used by generated code |
| `ormc.go` | `Ormc` type, file walker, top-level codegen orchestrator |
| `ormc_generate.go` | Emits `model_orm.go` content (Schema, helpers, validation) |
| `ormc_tags.go` | Parses `db:`, `json:`, `input:` tags into `FieldInfo` |
| `ormc_handler.go` | Project-wide handler registration |
| `ormc_relations.go` | Foreign-key resolution between structs |
| `cmd/ormc/` | CLI entrypoint |
| `tests/` | Test fixtures + `_test.go` files |
| `docs/` | Architecture, design rationale, tag reference (see `STRUCT_TAGS.md` mirror in `fmt/docs`) |

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
