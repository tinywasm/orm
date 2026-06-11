# PLAN — ormc centralized module scan (external + replace modules → schema sync)

> **Problem this adds.** The tool-driven sync ([CHECK_PLAN.md](CHECK_PLAN.md)) wires `ormc` to sync
> the schema of every `model.go` the **watcher** hands it via `NewFileEvent`. But the watcher only
> sees the **local project tree** (+ `replace` dirs). A destination project (e.g. `mjosefa-cms`) also
> depends on **external modules** (e.g. `github.com/veltylabs/item-catalog`) that live in the
> **read-only module cache** and ship a **committed `model_orm.go`**. Their tables
> (`catalog_item`, …) must be created in the destination's single DB, but their `model.go` never
> reaches `NewFileEvent`. This plan adds a **centralized startup scan** of all modules — mirroring how
> `ssr`/`image` extract assets from every module — so external schemas are synced too.
>
> **Self-contained, single-module plan** (`tinywasm/orm`, package `ormc`). Prerequisites:
> - The tool-driven seam from [CHECK_PLAN.md](CHECK_PLAN.md) (`ormc.SchemaSyncer`, `SetSyncer`,
>   `asFields()`) — **already implemented**.
> - `tinywasm/modfind` published. Its contract, inlined so this plan needs no external file:
>   `modfind.New() *Finder`; `(*Finder).Discover(rootDir) ([]Module, error)` runs `go list -m -json
>   all` once and caches; `Module{Path, Dir, Version string; IsMain, IsReplace, Indirect bool}` with
>   `(Module).Writable() bool` (true for the main module and local `replace` targets — writable; false
>   for the read-only cache).

Related (same repo): [CHECK_PLAN.md](CHECK_PLAN.md) (the per-file seam), [ARQUITECTURE.md](ARQUITECTURE.md).

---

## 1. Development Rules (constraints copied for execution context)

- **`ormc` stays blind to the database.** It still only calls the injected `SchemaSyncer.SyncSchema(
  table, []fmt.Field)`. The scan adds module-awareness, not DB-awareness.
- **`ormc` may import `modfind`, never the `orm` runtime.** `modfind` is stdlib + `tinywasm/fmt`
  only; importing it does not break the "ormc does not import orm" rule. (`fmt.Field` comes from
  `tinywasm/fmt`, already imported.)
- **REUSE `modfind` for discovery — do NOT reimplement `go list`.** Module enumeration + replace/cache
  classification already exist, tested, in the **published** `github.com/tinywasm/modfind`. `ormc`
  consumes `(*modfind.Finder).Discover`/`Module.Writable()`; it must **not** write its own `go list` /
  `os/exec` walk. Only `ScanModules`, `scanWritableModule`, `scanReadonlyModule`, and `parseGenerated`
  are new code here.
- **Writable vs read-only decides the action (the user's rule):**
  - **`Writable()` (main module or local `replace`)** → parse `model.go` via the existing AST
    pipeline, **generate `model_orm.go` in place**, then `SyncSchema`. Agile local-dev loop.
  - **read-only (cache)** → **do not write**. Parse the **committed `model_orm.go`** to recover
    `(table, []fmt.Field)` and `SyncSchema` only. The schema source is the published generated file
    (authoritative; avoids re-deriving against a possibly different orm/tag version).
- **Single discovery, no re-walk.** The scan consumes a `modfind.Finder` (one `go list` shared with
  ssr/image via app injection). It does **not** run its own `go list` when a finder is injected.
- **The scan is a startup pass, distinct from the two existing modes.** `NewFileEvent` = per-file live
  edits (local tree); `Run()` = CLI full-walk; **`ScanModules` = startup multi-module schema sync**.
  They coexist.
- **Zero reflection. `gotest`. Documentation first.**

---

## 2. Problem (precise)

1. `ormc.Generator` has no entry that iterates **modules** — only per-file (`NewFileEvent`) and the
   CLI full-walk (`Run()`), both scoped to the local tree.
2. External modules are **read-only**; ormc must **not** try to regenerate `model_orm.go` there
   (write would fail), yet their schema must be synced.
3. ormc currently derives schema only from `model.go` (tags → `FieldInfo` → `asFields()`). For
   read-only modules there is no writable output and the committed `model_orm.go` is the chosen
   source — ormc has **no parser** for an already-generated `model_orm.go`.

---

## 3. Decision

### 3.1 `ScanModules` — the centralized entry

```go
// ormc/scan.go  (package ormc)

// SetFinder injects the shared modfind.Finder (one go list across ssr/image/ormc).
func (g *Generator) SetFinder(f *modfind.Finder) { g.finder = f }

// ScanModules syncs the DB schema of every discovered module to the injected
// SchemaSyncer. Called once at startup by the tool (app), after SetSyncer.
//   - Writable module (main / replace): regenerate <file>_orm.go from model.go,
//     then sync each DB struct.
//   - Read-only module (cache): parse the committed model_orm.go, then sync.
// No-op if no syncer is injected (CLI codegen-only path).
func (g *Generator) ScanModules(rootDir string) error {
    if g.syncer == nil { return nil }
    if g.finder == nil { g.finder = modfind.New() }
    mods, err := g.finder.Discover(rootDir)
    if err != nil { return err }
    for _, m := range mods {
        if m.Writable() {
            if err := g.scanWritableModule(m.Dir); err != nil {
                g.log("ormc scan (writable)", m.Path, ":", err) // log-and-continue
            }
        } else {
            if err := g.scanReadonlyModule(m.Dir); err != nil {
                g.log("ormc scan (readonly)", m.Path, ":", err)
            }
        }
    }
    return nil
}
```

### 3.2 Writable modules — reuse the existing pipeline

`scanWritableModule(dir)` walks `dir` for `model.go`/`models.go`, and for each runs the **same**
per-file logic `NewFileEvent` already uses: `parseStructsInFile` → cache → `ResolveRelations` →
`GenerateForFile` (writes `<file>_orm.go` in place) → for each `!NoDB` struct
`syncer.SyncSchema(info.ModelName, info.asFields())`. No new schema logic — just iterate the module's
model files. (Skip `_orm.go` outputs.)

### 3.3 Read-only modules — parse the committed `model_orm.go`

`scanReadonlyModule(dir)` reads `dir` for `*_orm.go` files (the committed generated output) and, for
each, extracts `(table, []fmt.Field)` **without** importing/compiling them — pure AST:

New `ormc/parse_generated.go`:

```go
// parseGenerated extracts (modelName → []fmt.Field) from a generated *_orm.go
// file by reading its `func (m *T) ModelName() string { return "<table>" }` and
// its `var _schema<T> = []fmt.Field{ … }` composite literal.
func parseGenerated(path string) (map[string][]fmt.Field, error)
```

What it reads from the literal (the shape ormc itself emits — see
[generate.go](../ormc/generate.go)):

| AST node in `model_orm.go` | → `fmt.Field` |
|---|---|
| `{Name: "id", …}` (basic string lit) | `Name` |
| `Type: fmt.FieldText` / `FieldInt` / `FieldFloat` / `FieldBool` (selector/ident) | `Type` via a `string→fmt.FieldType` map (the inverse of ormc's existing emit map) |
| `NotNull: true` | `NotNull` |
| `DB: &fmt.FieldDB{PK: true, Unique: …, AutoInc: …}` | `DB` |
| `func (m *T) ModelName() string { return "catalog_item" }` | the table name for `_schemaT` |

Then `scanReadonlyModule` calls `g.syncer.SyncSchema(table, fields)` for each model found. **It never
writes.** If a module has no `*_orm.go`, it is skipped (nothing to sync; the module ships no DB
models, or the author never ran ormc).

> **Why parse the generated file, not `model.go`** (the user's decision): the committed `model_orm.go`
> is the schema the module **published**. Re-deriving from `model.go` could diverge if the destination
> runs a different orm/tag version. Reading the generated literal is authoritative and write-free.

### 3.4 Inverse type map (the one new lookup)

ormc already emits `Type: fmt.FieldText` etc. from `FieldInfo`. Add the inverse used by
`parseGenerated`:

```go
var fieldTypeByName = map[string]fmt.FieldType{
    "FieldText": fmt.FieldText, "FieldInt": fmt.FieldInt,
    "FieldFloat": fmt.FieldFloat, "FieldBool": fmt.FieldBool,
    // keep in sync with the forward map in generate.go
}
```
Match on the selector's `.Sel.Name` (handles both `fmt.FieldText` and a dot-imported `FieldText`).

---

## 4. Implementation Steps

### Step 1 — Bump modfind
`go get github.com/tinywasm/modfind@vX` in `tinywasm/orm`.

### Step 2 — Finder field + ScanModules
New [ormc/scan.go](../ormc/scan.go): add `finder *modfind.Finder` to `Generator`, `SetFinder`,
`ScanModules`, `scanWritableModule`, `scanReadonlyModule` (§3.1–§3.3). `scanWritableModule` reuses the
existing `parseStructsInFile`/`GenerateForFile`/sync logic factored out of `NewFileEvent`.

### Step 3 — Generated-file parser
New [ormc/parse_generated.go](../ormc/parse_generated.go): `parseGenerated` + `fieldTypeByName`
(§3.3–§3.4). AST only (`go/parser`, `go/ast`), no compile/exec.

### Step 4 — Documentation
[ARQUITECTURE.md](ARQUITECTURE.md): add the third mode ("startup module scan") next to per-file and
CLI; state the writable→generate / read-only→parse rule. README: one line.

---

## 5. Edge Cases

- **Read-only module with no `*_orm.go`** → skipped (no DB models, or author didn't run ormc). Not an
  error.
- **Writable replace whose `model.go` changed since its committed `model_orm.go`** → regeneration
  brings it current before sync (that's the point of treating replace as writable).
- **`model_orm.go` literal uses an unknown `fmt.FieldXxx`** → `fieldTypeByName` miss → log a clear
  warning and skip that field (don't guess a type). Surfaces a version-skew between author and
  destination.
- **No syncer (CLI ormc)** → `ScanModules` is a no-op; codegen-only behavior preserved.
- **`go list` failure inside modfind** → `ScanModules` returns the error; app logs it and disables
  cross-module sync (local per-file sync still runs).
- **Same table from two modules** → last `SyncSchema` wins; `db.Sync` is additive/idempotent so this
  is safe, but log it as a warning (possible duplicate ownership).

---

## 6. Test Strategy

`gotest` in `tinywasm/orm/ormc/`.

| # | Case | Assert |
|---|------|--------|
| MS1 | `parseGenerated` on a local fixture `*_orm.go` (a committed-style generated file written into `ormc/testdata/`, with `ModelName()` + a `_schema<T> = []fmt.Field{…}` literal) | recovers the table name + every field with correct `Type`/`NotNull`/`PK` |
| MS2 | `parseGenerated` PK/Unique/AutoInc | `fmt.FieldDB` populated from `&fmt.FieldDB{…}` |
| MS3 | `ScanModules` with a seeded finder (1 writable + 1 read-only module) | writable → `<file>_orm.go` (re)written + synced; read-only → synced, **not** written |
| MS4 | read-only module dir is never written | file mtimes unchanged after scan |
| MS5 | read-only module with no `*_orm.go` | skipped, no sync, no error |
| MS6 | no syncer injected | `ScanModules` no-op |
| MS7 | unknown `fmt.FieldXxx` in literal | field skipped + warning logged; other fields synced |

> Use `modfind.New()` + `Seed(rootDir, []modfind.Module{{Dir: writableFixture, IsMain: true}, {Dir:
> cacheFixture}})` to drive `ScanModules` without a real `go list`. The cache fixture is a dir with a
> committed `*_orm.go` and **no** writable bit asserted via mtime checks.

---

## 7. Out of Scope

- `modfind` implementation — its own plan.
- The per-file `NewFileEvent` seam and `SchemaSyncer`/`asFields` — [CHECK_PLAN.md](CHECK_PLAN.md)
  (already implemented).
- App wiring (construct the shared finder, inject into ormc/ssr/image, call `ScanModules` at startup)
  — the `tinywasm/app` plan.
- Adapter DDL translation / `db.Sync` algorithm — orm core + adapter plans.
