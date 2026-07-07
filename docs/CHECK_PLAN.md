# PLAN — ormc: always-on typed-fields helper + delegate casing to fmt

> Dispatched via CodeJob. Skill: **agents-workflow**. Single repo: `github.com/tinywasm/orm`.
> Self-contained: every contract, rule and example is inline.

## 0. TL;DR

Two gaps in `ormc` block `veltylabs/modules/item_catalog`:

1. **Typed-fields helper is never generated.** The inverted-generator refactor
   (`e9fa3c2`) dropped the wiring of the old `// orm:typed_fields` directive. `StructInfo.WantTypedFields`
   is now an orphaned field that nothing sets, so `GenerateForFile`'s `if info.WantTypedFields`
   branch is dead and the `<Struct>_` accessor (`var CatalogItem_ = struct{…}{…}`) is never emitted.
   Any consumer referencing `CatalogItem_.Sku` fails to compile.
   **Decision: make the helper always-on for DB models and delete the directive concept entirely**
   (the directive comment is unwanted; the helper's value is real — `item_catalog/mcp.go` uses it in
   8 query sites for compile-time-safe column references).

2. **Local `knownUpper` acronym map.** `ormc` reconstructs Go field names with a hand-rolled
   `ToPascalCase` + a private `knownUpper` **map** (`parse_definition.go:354-386`). Case logic belongs
   in `fmt`, which now (v0.25.1) does snake→Pascal algorithmically. Delete the local copy and delegate.

## 1. Root cause (evidence)

- `ormc/generator.go:62` declares `WantTypedFields bool` — set by no one.
- `ormc/generate.go:325-337` emits the helper only inside `if !info.NoDB { if info.WantTypedFields { … } }`;
  the inner guard is permanently false. `grep -rn WantTypedFields ormc/*.go` → declaration + dead read only.
- `git log -p -S WantTypedFields -- ormc/` shows the old struct-comment parser
  (`fmt.Contains(comment.Text, "orm:typed_fields")`) was deleted in `e9fa3c2` and never re-added.
- `ormc/parse_definition.go:354` `ToPascalCase` + `:373` `knownUpper` duplicate case logic now covered
  by `fmt` v0.25.1 (`Convert(col).CamelUp()` is snake/kebab-aware).

## 2. Contract (target behavior)

`model.go` carries **no directive** — every DB model automatically gets its accessor:

```go
var CatalogItemModel = model.Definition{
	Name: "catalog_item",
	Fields: model.Fields{
		{Name: "id", Type: model.FieldText, DB: &model.FieldDB{PK: true}},
		{Name: "tenant_id", Type: model.FieldText, NotNull: true},
		{Name: "sku", Type: model.FieldText, NotNull: true},
		// ...
	},
}
```

ormc emits into `model_orm.go` (note **pure** casing — no acronym dictionary, see fmt PLAN §7):

```go
var CatalogItem_ = struct {
	Id, TenantId, Sku, Name, Description, Category, Type, Price, Currency, IsActive, UpdatedAt string
}{
	Id: "id", TenantId: "tenant_id", Sku: "sku", Name: "name", /* ... */
}
```

Rules:
- **Always emitted for DB models** (`!info.NoDB`). No-DB / codec-only definitions get no helper.
- Field key names are the generated Go field names (`fmt.Convert(col).CamelUp()`), byte-for-byte equal
  to the concrete struct field names, so `CatalogItem_.Sku` and `m.Sku` refer to the same field.

## 3. Changes

### 3.1 Make the helper always-on; remove `WantTypedFields`

- `generator.go`: delete the `WantTypedFields bool` field from `StructInfo`.
- `generate.go:325-337`: drop the `if info.WantTypedFields` gate; emit the `<Struct>_` block
  unconditionally inside the existing `if !info.NoDB { … }`. No comment parsing is added — this is
  strictly *less* code than the old directive path.

There is no directive to parse, no `g.log` warning path, and nothing to wire in `parse_definition.go`
for this feature.

### 3.2 Delete `knownUpper` + `ToPascalCase`; delegate to `fmt.CamelUp`

Requires `github.com/tinywasm/fmt` **v0.25.1** (published; `CamelUp`/`CamelLow` treat `_`/`-` as word
separators). Bump `go.mod`, then:

- **Delete** `ToPascalCase` **and** `knownUpper` from `parse_definition.go`.
- Replace every `ToPascalCase(fi.ColumnName)` with `fmt.Convert(fi.ColumnName).CamelUp().String()`.
- Casing is **pure algorithmic**: `id`→`Id`, `sku`→`Sku`, `tenant_id`→`TenantId`. The struct field,
  `Pointers`, `Encode/DecodeFields`, and the `<Struct>_` helper all inherit it consistently.

### 3.3 Docs — `ormc/AGENTS.md`

Remove the `orm:typed_fields` directive from the documented directives list (line ~36); the accessor
is now automatic for DB models. Keep other directives (`orm:form_widgets`, `orm:no_db`) as-is.

## 4. Non-goals

- Do **not** change `model.Definition` (no new struct field, no directive metadata).
- Do **not** reintroduce any `// orm:typed_fields` comment or opt-in flag.
- Do **not** add an acronym dictionary/heuristic (fmt PLAN §7: pure, rejected heuristics recorded).
- Do **not** fork or patch ormc outside this repo (`/tmp/orm` copies forbidden — fix upstream, publish).
- Do not alter column/table names or DDL behavior.

## 5. Tests

Add to `ormc/generator_test.go` (or a new `parse_definition_test.go`):

1. A DB-backed `model.Definition` (with a `sku` column) generates output containing
   `var CatalogItem_ = struct {` and the key `Sku: "sku"`.
2. The concrete struct field is `Sku string` (locks in §3.2 pure casing) and `Id`, `TenantId`.
3. A no-DB (codec-only) definition generates **no** `_` helper.
4. Round-trip sanity: helper key `TenantId` maps to column `tenant_id`.

## 6. Acceptance criteria

- `gotest ./...` green in `tinywasm/orm`; `go.mod` on `fmt v0.25.1`.
- Any DB `model.Definition` yields `model_orm.go` with `var <Struct>_ = struct{…}{…}` whose keys are
  the Go field names (`Id`, `Sku`, `TenantId`, …) and values are the columns.
- Struct field `Sku`, `m.Sku`, and `CatalogItem_.Sku` are the same field.
- No-DB definitions produce no helper.
- No `knownUpper`/`ToPascalCase`/`WantTypedFields` symbols remain in the package.
- Published (`gopush`) so `item_catalog` can `go get` it.

## 7. Stages

| # | Stage | Output | Gate |
|---|---|---|---|
| 1 | Always-on helper (§3.1) | `WantTypedFields` removed; helper emitted for `!NoDB` | helper present, no directive |
| 2 | Delegate casing to `fmt.CamelUp` (§3.2) | `knownUpper`+`ToPascalCase` deleted; `go.mod` → fmt v0.25.1 | struct field + helper key agree (`Sku`) |
| 3 | Docs (§3.3) + tests (§5) | AGENTS.md updated; test cases | `gotest ./...` green |
| 4 | Publish (§6) | tagged/pushed module | `item_catalog` unblocks |

## 8. Downstream unblock

Once published, `item_catalog` proceeds with **no directive**: rewrite `model.go` to `model.Definition`
form, regenerate `model_orm.go` (helper auto-generated), and update `mcp.go` + the `CatalogItem`
struct's public fields to the pure casing — `CatalogItem_.{Id,TenantId,Sku,Type,IsActive}` and
`item.Id`/`item.Sku`/`item.TenantId`.
