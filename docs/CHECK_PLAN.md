# PLAN — Tool-driven schema sync contract (`Open`/`Register`, `SyncSchema`, `ormc.SchemaSyncer`)

> The dev schema-sync engine already exists in `tinywasm/orm`: `db.Sync(models...)` reconciles a DB
> additively (CreateTable + AddColumn), with introspection-based renames and safe drops
> ([sync.go](../sync.go)), and the `Action` contract (`ActionAddColumn`/`RenameColumn`/`DropColumn`,
> [query.go](../query.go)). The codegen is the `orm/ormc` subpackage with a watch handler
> ([ormc/watch.go](../ormc/watch.go)).
>
> **What's missing is the seam that lets the dev TOOL drive the sync instead of the user's project,
> plus two fixes to the existing code.** `tinywasm/app` watches `model.go`, and — because `ormc`
> already parses the schema from AST — the tool must apply that schema to a DB **it** owns. This plan
> adds:
> 1. an agnostic engine **registry** (`orm.Open`/`orm.Register`) so the tool picks postgres/sqlite
>    from a connection string (Step 1),
> 2. a schema-level **`db.SyncSchema(table, fields)`** entry that needs no model instance (Step 2),
> 3. **observability** — `db.SetLog` + de-swallowing `sync.go` so no error is hidden (Step 2b),
> 4. an **`ormc.SchemaSyncer`** injection so `ormc.Generator` applies the parsed schema, staying
>    blind to the database — driven **per-file** (replacing the `Run()` re-walk), Step 3.
>
> **The sync runs in the tool, never in the user's project.** The project imports only `orm`
> (agnostic runtime); it contains no sync bootstrap. See `tinywasm/app/docs/PLAN.md` (consumer).

Related: [ARQUITECTURE.md](ARQUITECTURE.md) · consumer: `tinywasm/app/docs/PLAN.md`

---

## 1. Development Rules (constraints copied for execution context)

- **`orm` root stays agnostic — no `database/sql`.** The root package compiles for Go and WASM.
  `Open`/`Register` only parse a scheme string and call a registered factory; they import **no**
  adapter and no `database/sql`. (qb.go was just cleaned of `sql.ErrNoRows` → `orm.ErrNoRows`.)
- **Registry is the one allowed global.** A `map[string]Factory` mirrors `database/sql`'s driver
  registry — the canonical exception to "no global state". Adapters self-register in `init()`.
- **`ormc` is blind to the database.** It parses `model.go` (AST) and holds the schema in
  `StructInfo.Fields`. It applies it through an **injected** `SchemaSyncer` interface — never knowing
  postgres vs sqlite, never importing `orm`'s DB runtime beyond the interface it defines locally.
- **No `fmt.Model` instance required for sync.** `ormc` cannot produce `Pointers()` from AST.
  `SyncSchema(table, fields)` takes exactly what `ormc` has (table name + `[]fmt.Field`) and wraps it
  in a synthetic model. `Pointers()` is never invoked during sync (verified: the safe-drop check uses
  `rows.Next()`, not `Scan`).
- **Additive + introspective reconcile already implemented.** Do not rewrite `sync.go`'s algorithm;
  only add the `SyncSchema` entry that funnels into it.
- **Zero reflection. `gotest` (not `go test`). Documentation first.**

---

## 2. Problem

`db.Sync(models ...fmt.Model)` exists, but it requires **compiled model instances**. The dev tool
(`tinywasm/app`) runs the user's server as a separate process and **cannot instantiate the user's
structs**. What the tool *does* have, via `ormc`, is the **AST-parsed schema** of every `model.go`.

Gaps that block the tool-driven hot loop (plus two fixes to existing code):
1. The tool has a **connection string** (`DATABASE_CONNECTION` from `.env`) but no agnostic way to
   turn it into a `*orm.DB` for the right engine. `orm.New(exec, compiler)` is pure DI; nothing maps
   a DSN → adapter.
2. `db.Sync` wants `fmt.Model`; `ormc` has `table + []fmt.Field`. No schema-level entry exists.
3. `ormc.Generator` regenerates `*_orm.go` but has **no hook** to apply the schema to a DB; and it
   must do so **without** learning about databases.
4. **Fix:** `sync.go` swallows every per-column error with `_ = …` (no visibility) — §3.2.1.
5. **Fix:** `ormc.NewFileEvent` calls `Run()` (full-project re-walk) on every event instead of
   processing the one file the watcher handed it — §3.4.

> The previous plan said "the user's server owns the `db.Sync` call". That is **superseded**: the
> sync is tool-side. Fix any doc that still states otherwise (Step 4).

---

## 3. Decision

### 3.1 Agnostic engine registry — `orm.Open` / `orm.Register`

```go
// open.go (root package — no database/sql, no adapter import)

// Factory builds a ready *DB from a DSN. It matches the executor adapters'
// existing constructors (postgres.New and sqlite.Open are already
// `func(string) (*DB, error)`), so registration is a one-liner with zero new
// adapter code beyond init().
type Factory func(dsn string) (*DB, error)

// Register binds a URL scheme (e.g. "postgres", "sqlite") to a Factory.
// Adapters call this from init(). Last registration for a scheme wins.
func Register(scheme string, f Factory)

// Open parses the scheme of dsn, looks up the registered Factory, and
// returns a ready *DB. Errors if the scheme is unknown (adapter not imported).
func Open(dsn string) (*DB, error)
```

- The DSN scheme is the part before `://` (`postgres://…`, `sqlite://…`, `sqlite::memory:`).
- `orm` imports nothing engine-specific. The **executor adapters** register their existing
  constructor in their own `init()`: `orm.Register("postgres", postgres.New)` in `tinywasm/postgres`,
  `orm.Register("sqlite", sqlite.Open)` in **`tinywasm/sqlite`** (the SQLite executor module; the
  `tinywasm/sqlt` module is **compiler-only** and does no registration). That work lives in **their**
  plans; this plan only declares the `Factory` contract.
- The **tool** imports the adapters via side-effect (`import _ "github.com/tinywasm/postgres"`); the
  **user's project never does**.

### 3.2 Schema-level sync — `db.SyncSchema`

```go
// sync.go (root package, isomorphic)

// SyncSchema reconciles one table to the given fields, with no model instance.
// Wraps (table, fields) in a synthetic model and delegates to the existing
// Sync algorithm (CreateTable + AddColumn + introspective rename/safe-drop).
func (db *DB) SyncSchema(table string, fields []fmt.Field) error {
    return db.Sync(schemaModel{name: table, fields: fields})
}

// schemaModel is a sync-only fmt.Model carrying just name + schema.
// Pointers() is never called during Sync, so it returns nil.
type schemaModel struct {
    name   string
    fields []fmt.Field
}
func (s schemaModel) ModelName() string   { return s.name }
func (s schemaModel) Schema() []fmt.Field { return s.fields }
func (s schemaModel) Pointers() []any     { return nil }
```

- Reuses **all** of `syncModel`'s logic (introspection, rename via `RenameProvider`, safe drops).
  Rename support for the AST path can come later (the synthetic model may also implement
  `RenameProvider` from `old_name` tags — deferred unless needed).
- `db.Sync(models...)` stays for any runtime caller; `SyncSchema` is the tool/AST entry.

### 3.2.1 Observability — **stop swallowing errors** in `sync.go`

The current [sync.go](../sync.go) discards every additive/reconcile error with `_ = db.execQuery(…)`
(lines ~63, ~99, ~106, ~156). That violates the "omit no error" rule — a failed `ADD COLUMN`,
`RENAME`, or `DROP` vanishes silently. Fix it so every swallowed/skipped case is **logged**, then the
loop continues (log-and-continue must *log*, not just *continue*):

- Add a logger hook to `orm.DB` (tinywasm's universal `SetLog` pattern):
  ```go
  // db.go
  type DB struct { exec Executor; compiler Compiler; log func(...any) }
  func (db *DB) SetLog(fn func(...any)) { db.log = fn }
  func (db *DB) logw(a ...any) { if db.log != nil { db.log(a...) } }
  ```
- In `sync.go`, replace each `_ = db.execQuery(q, m)` with a logged variant, e.g.:
  ```go
  if err := db.execQuery(qAdd, m); err != nil {
      db.logw("sync:", tableName, "add column", field.Name, "skipped:", err)
  }
  ```
  …and likewise for rename, drop, and the additive fallback. Also `logw` the **safe-drop skip**
  ("column X kept: has data") so even the deliberate no-ops are visible.
- The logger is **injected by the consumer**: `tinywasm/app` calls `db.SetLog(<build-tab sink>)`
  after `orm.Open` (see app plan). Default is no-op (silent) for non-tool callers — but the tool
  always wires it, so in dev nothing is hidden.
- Keep `db.log` `func(...any)` (no `fmt`/IO deps) so the root package stays isomorphic.
- **Propagate `log` into transactions.** `db.Sync` runs inside `db.Tx`, which builds a fresh
  tx-scoped `*DB` ([tx.go:28](../tx.go)) that currently copies only `exec`/`compiler`. Add
  `log: db.log` there, or every logged warning inside the tx is dropped — defeating the fix.

### 3.3 `ormc.SchemaSyncer` injection — codegen applies, stays DB-blind

```go
// orm/ormc/sync.go (package ormc)

// SchemaSyncer applies a parsed table schema. Implemented by the consumer
// (tinywasm/app) over *orm.DB; ormc only ever sees this interface.
type SchemaSyncer interface {
    SyncSchema(table string, fields []fmt.Field) error
}

func (g *Generator) SetSyncer(s SchemaSyncer) { g.syncer = s }
```

- After a file is (re)generated, if `g.syncer != nil`, iterate the **DB structs of that file**
  (`!StructInfo.NoDB`) and call `g.syncer.SyncSchema(info.ModelName, info.asFields())`. **Per-file,
  not a full-project loop** (see §3.4).
- `info.asFields()` is a new helper mapping `[]FieldInfo` → `[]fmt.Field`:
  | FieldInfo | fmt.Field |
  |---|---|
  | `ColumnName` | `Name` |
  | `Type` | `Type` |
  | `NotNull` | `NotNull` |
  | `PK`/`Unique`/`AutoInc` | `DB: &fmt.FieldDB{PK, Unique, AutoInc}` |
  (Widget/Permitted are irrelevant to DDL — omit.)
- **No syncer injected (the `ormc` CLI binary)** → sync skipped; codegen only. CLI behavior
  unchanged.
- `orm/ormc` does **not** import the `orm` DB runtime — it defines `SchemaSyncer` locally and depends
  only on `github.com/tinywasm/fmt` (already imported).

### 3.4 Per-file event processing — **single read, no re-walk** (efficiency fix)

`tinywasm/app`'s `devwatch` does **one** `filepath.Walk` at startup and dispatches **each file** to
the handlers via `NewFileEvent` (and one event per live edit), with a **depfind** ownership gate
(`ThisFileIsMine`). So `ormc.NewFileEvent` must process **only the file it was handed** — it must
**not** call `Run()` (which re-walks the whole project via `collectAllStructs`). Re-walking on every
event duplicates the work `devwatch`+`depfind` already did.

Two execution modes:
- **Integrated in app (watcher):** `NewFileEvent(filePath)` parses **only `filePath`**, updates an
  in-memory struct cache, resolves relations against the cache, regenerates `<file>_orm.go`, and
  syncs that file's DB structs. No project walk.
- **Standalone CLI ([cmd/ormc/main.go](../cmd/ormc/main.go)):** no watcher feeds files, so `Run()`
  does the full `collectAllStructs` walk — the **only** place the walk remains.

New `Generator` pieces:
- `cache map[string]StructInfo` — accumulates parsed structs across events (for relation resolution).
- `parseStructsInFile(path) ([]StructInfo, error)` — the per-file parse extracted from the inline
  loop inside `collectAllStructs` (walk one file's `typeSpec`s → `ParseStruct`).

### 3.5 The seam (who owns what)

```
devwatch (app):       1 walk al arrancar + 1 evento por edición → NewFileEvent(file)  [depfind gate]
ormc.Generator:       parse ESE file → cache → regen <file>_orm.go → syncer.SyncSchema(table, fields)  [DB-blind, no re-walk]
tinywasm/app (tool):  orm.Open(dsn) → *orm.DB ; SetSyncer(dbSyncer{db})   [imports postgres+sqlite]
orm root:             SyncSchema → Sync → CreateTable + AddColumn Actions  [agnostic]
postgres / sqlite:    init() Register(...)  [executor adapters, their plans]
postgres / sqlt:      translate Actions → SQL  [compilers, their plans]
user project:         imports only orm. No sync code.
```

---

## 4. Implementation Steps

### 4.0 Cross-module order
1. **orm** — Steps 1, 2, 2b, 3, 4 → `gopush` `vX`.
2. **postgres** — `init()` `orm.Register("postgres", New)` + translate the new Actions; bump orm to
   `vX` → `gopush`.
3. **sqlt** (compiler) — translate the new Actions; **sqlite** (executor) — `init()`
   `orm.Register("sqlite", Open)` + map `sql.ErrNoRows` → `orm.ErrNoRows` + `TableIntrospector`; bump
   orm to `vX` → `gopush`.
4. **app** — bump orm to `vX`, import `_ postgres` + `_ sqlite`, wire `orm.Open` + `SetSyncer` (its
   plan).

### Step 1 — Registry
**File:** new [open.go](../open.go) (root package). Implement `Factory`, `Register`, `Open` (§3.1).
Parse the scheme before `://` (and the `sqlite::memory:` form). Return a clear error for an unknown
scheme that names the missing adapter import. `gotest`: register a fake factory, assert `Open`
resolves it and errors on unknown schemes.

### Step 2 — `db.SyncSchema` + synthetic model
**File:** [sync.go](../sync.go). Add `SyncSchema` and the unexported `schemaModel` (§3.2). `gotest`
with `MockExecutor`/`MockCompiler`: assert `SyncSchema("x", fields)` issues `CreateTable` + one
`AddColumn` per field (reuses existing Sync test scaffolding).

### Step 2b — `db.SetLog` + de-swallow `sync.go` (§3.2.1)
**Files:** [db.go](../db.go) (add `log func(...any)` field + `SetLog` + `logw` helper);
[tx.go](../tx.go) (propagate `log: db.log` into the tx-scoped `*DB` at line ~28 — else logs inside
the sync transaction are lost); [sync.go](../sync.go) (replace every `_ = db.execQuery(…)` with a
logged variant; log the safe-drop skip too). `gotest` with a recording logger: assert a failing
`AddColumn` is **logged** (not discarded) and the loop continues; assert the log still fires when the
sync runs **inside a Tx** (TxExecutor present); assert no-op when no logger is set.

### Step 3 — Per-file watch handler + `SchemaSyncer` + `asFields()` (**replace the `Run()` re-walk**)
**Files:** new [ormc/sync.go](../ormc/sync.go); [ormc/watch.go](../ormc/watch.go) (rewrite
`NewFileEvent`); [ormc/generator.go](../ormc/generator.go) (add `syncer SchemaSyncer` field, `cache
map[string]StructInfo`, `parseStructsInFile`); the `asFields()` mapping (§3.3).

The current [ormc/watch.go](../ormc/watch.go) `NewFileEvent` calls `g.Run()` — a **full-project
re-walk on every event**. Replace it with **per-file** processing (§3.4):
- `NewFileEvent(fileName, ext, filePath, evt)`: ignore non-`model.go`/`models.go`; otherwise:
  1. `infos := g.parseStructsInFile(filePath)` — parse **only that file** (extract the per-file loop
     from `collectAllStructs`; do not walk the project).
  2. Merge `infos` into `g.cache`; `g.ResolveRelations(g.cache)` so cross-file FKs still resolve.
  3. `g.GenerateForFile(infos, filePath)` — regenerate just `<file>_orm.go`.
  4. If `g.syncer != nil`, for each DB struct in `infos` (`!NoDB`): `g.syncer.SyncSchema(info.
     ModelName, info.asFields())`. Log (not fail) a per-table error via `g.log` — codegen already
     succeeded.
- **`Run()` stays unchanged** (full `collectAllStructs` walk) — it is the **CLI-only** path
  ([cmd/ormc/main.go](../cmd/ormc/main.go)); the watcher never calls it.
- `gotest`: a fake `SchemaSyncer` records calls; assert `NewFileEvent` on a `model.go` syncs that
  file's DB structs only, `NoDB` skipped, no call without a syncer, and that it does **not** trigger a
  project-wide walk (e.g. a sentinel file outside the event is not re-read).

### Step 4 — Documentation
**Files:** [ARQUITECTURE.md](ARQUITECTURE.md), [README.md](../README.md), [AGENTS.md](../AGENTS.md).
- **Correct the ownership statement**: schema sync is **tool-driven**, applied by the consumer
  (`tinywasm/app`) through `SchemaSyncer`/`SyncSchema`; the user's project owns no sync call. Remove
  any "the user's server owns `db.Sync`" wording.
- Document `orm.Open`/`Register` (the agnostic engine registry) and the `Factory` contract adapters
  implement.
- Document `db.SyncSchema` next to `db.Sync` in the public API.
- README: short "Tool-driven schema sync" note (the tool calls `orm.Open` + `SetSyncer`; the project
  just defines models).

---

## 5. Edge Cases

- **Unknown DSN scheme** (`Open`) → error naming the likely missing adapter import; the tool logs and
  disables sync (codegen still runs).
- **No syncer injected** → `ormc` codegen-only (the CLI path). No DB calls.
- **`NoDB` struct** → excluded from the sync loop (no table).
- **Non-`model.go` event** → `NewFileEvent` returns nil immediately (no parse, no sync).
- **Cross-file FK ref to a struct not yet seen** → resolved later as its file is dispatched; the
  cache accumulates across events (startup walk visits every file once).
- **Per-table sync error** → logged via `g.log`, codegen result preserved; other tables still
  attempted.
- **Per-column add/rename/drop error** (inside `db.Sync`) → logged via `db.log` (§3.2.1), **never
  discarded**; the loop continues. Visible in the build tab once `app` wires `db.SetLog`.
- **Safe-drop skip (column has data)** → logged via `db.log` so the deliberate no-op is visible.
- **Empty schema / no DB structs in the file** → no `SyncSchema` calls.
- **`Pointers()` on `schemaModel`** → returns nil; never invoked by `Sync` (safe-drop uses
  `rows.Next()`).

---

## 6. Test Strategy

`gotest` in `tinywasm/orm/tests/` and `tinywasm/orm/ormc/`.

| # | Case | Assert |
|---|------|--------|
| O1 | `Register("fake", f)` + `Open("fake://x")` | returns a `*DB` built from `f` |
| O2 | `Open("nope://x")` | error naming unknown scheme |
| O3 | `SyncSchema("x", [f1,f2])` | `CreateTable` + 2 `AddColumn` Actions (MockCompiler/Executor) |
| O4 | `SyncSchema` delegates to `Sync` | introspection/rename/drop path still exercised (existing S-tests) |
| O5 | failing `AddColumn` + `SetLog(rec)` | error **logged** to `rec` (not discarded), loop continues |
| O6 | failing `AddColumn` **inside a Tx** (TxExecutor) + `SetLog(rec)` | error still logged (tx-scoped `*DB` carries `log`) |
| O7 | no `SetLog` set | no panic; silent no-op |
| C1 | `SetSyncer(fake)` + `NewFileEvent("model.go", …)` with a DB struct | one `SyncSchema(ModelName, fields)` recorded for that file |
| C2 | `NewFileEvent` on a file with a `// orm:no_db` struct | no `SyncSchema` for it |
| C3 | `NewFileEvent` with **no** syncer | zero `SyncSchema` calls; `<file>_orm.go` still written |
| C4 | `asFields()` maps PK/Unique/AutoInc/NotNull | `fmt.Field.DB` + `NotNull` populated correctly |
| C5 | `NewFileEvent("handler.go", …)` | ignored; no parse, no sync |
| C6 | `NewFileEvent` processes **only** `filePath` | a second file outside the event is **not** re-read (no project walk) |
| C7 | `Run()` (CLI path) still does the full walk | all `model.go` generated (regression guard) |

Existing `db.Sync` tests (S1–S13) remain valid; `SyncSchema` is a thin entry over them.

---

## 7. Out of Scope

- Adapter `init()`/`Register` bodies + `ErrNoRows` mapping + `TableIntrospector` — `tinywasm/postgres`
  and `tinywasm/sqlite` plans (executor modules). SQL translation of the Actions — `tinywasm/postgres`
  and `tinywasm/sqlt` plans (compilers).
- The connection resolution + watcher wiring — `tinywasm/app` plan.
- Rename support on the AST path (synthetic `RenameProvider`) — deferred unless needed.
- Production/versioned migrations — deferred (the engine evolves additively in dev).
