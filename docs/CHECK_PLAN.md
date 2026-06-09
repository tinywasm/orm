# PLAN — Orthogonal Directive Scheme + Per-Struct `// orm:typed_fields`

> Master orchestrator for a coupled redesign of the `ormc` doc-comment directives in
> **tinywasm/orm only**:
>
> 1. **Replace the directive scheme** with **orthogonal, atomic `orm:` directives**
>    (breaking change, **no legacy aliases**). Each directive toggles exactly one layer
>    and always means the same thing; capabilities **compose** by stacking lines.
> 2. **Add `orm:typed_fields`** — per-struct opt-in generation of the `<Name>_`
>    field-descriptor metadata (replaces the removed global `-fields` flag).
>
> Consumers (e.g. `veltylabs/item-catalog`) self-correct by updating their directives +
> re-running `ormc` after this lands.

Related: [ARQUITECTURE.md](ARQUITECTURE.md) · [WHY_PACKAGE_LEVEL_SCHEMA.md](WHY_PACKAGE_LEVEL_SCHEMA.md) · [diagrams/](diagrams/)

---

## 1. Development Rules (constraints copied for execution context)

- **Backend-only generator.** All `ormc*.go` files carry `//go:build !wasm`. The
  generator runs at build time on the server; never in WASM.
- **Isomorphic output.** The generated `*_orm.go` has **no build tag** — it compiles
  for both Go backend and WASM frontend. Anything emitted (including `<Name>_`) ships
  in the WASM binary, so keep it minimal and opt-in.
- **Dependency boundary.** `orm` depends on `github.com/tinywasm/fmt` only. Generated
  code may additionally import `github.com/tinywasm/orm` and `github.com/tinywasm/form/input`.
- **Zero reflection.** Schema/metadata is interface-driven via `fmt.Field` / `fmt.Fielder`.
- **No `time.Time`.** Use `int64` + `tinywasm/time`. (Unchanged here; stated for context.)
- **Column names are derived from field names** for persisted structs; `json:` rename is
  only permitted on **non-persisted** structs (`// orm:no_db`).
- **Directive-driven behavior.** Per-struct generation is controlled by doc-comment
  directives under the **`orm:` prefix, snake_case**, each **atomic and composable**:
  `orm:form_widgets`, `orm:no_db`, `orm:typed_fields`. No legacy `ormc:` aliases (breaking change).

---

## 2. Problem

Re-running `ormc` against `item-catalog` produced compile errors with two **independent**
root causes:

### 2.1 In scope — `<Name>_` descriptors no longer generated
Commit `815b4af` made `<Name>_` field descriptors **opt-in via the global `-fields` flag**
(default `false`). The global flag is all-or-nothing per generation run, but `ormc` walks
the whole tree in one pass — so it cannot express *"this model needs typed descriptors,
that one doesn't"*. Consumers that build type-safe queries (`Where(CatalogItem_.ID)`,
`orm.Eq(CatalogItem_.ID, ...)`) need the descriptors **selectively, per model**.

### 2.2 Out of scope — `ReadAll<Name>` return shape
Commit `2e42145` changed `ReadAll<Name>` from `[]*Name` → `*<Name>List`. Call sites that
do `len(results)` / `range results` over the pointer break. **This is a consumer migration,
not an orm change.** Documented here only so it is not mistaken for a regression to fix in
orm. Item-catalog adapts its call site (`range *results`) when it regenerates.

---

## 3. Decision

### 3.0 Orthogonal directive scheme (breaking, no legacy)

Two capabilities are toggled **independently**, each by one atomic directive. The DB layer
is **on by default**; the form layer is off by default.

| Directive | Effect (single job) |
|---|---|
| `// orm:form_widgets` | **add** the form layer: input widgets + `Validate(action)` |
| `// orm:no_db` | **remove** the DB helpers (no `ReadOne`/`ReadAll`/`<Name>_`; `ModelName` stays — see note) |
| `// orm:typed_fields` | **add** `<Name>_` typed field accessors (needs DB layer) |

Compose by stacking lines:

| Directives | Generated |
|---|---|
| *(none)* | DB only (`Schema`, `Pointers`, `ModelName`, `ReadOne`/`ReadAll`) |
| `// orm:form_widgets` | DB + form (widgets + `Validate`) |
| `// orm:form_widgets` + `// orm:no_db` | form, **no** DB (transport/UI struct: login req, RPC params) |
| `// orm:no_db` | **no** DB, no form (pure JSON/transport: `Schema`+`Pointers` for `json`) |
| `// orm:typed_fields` | DB + typed accessors `User_.Name` |
| `// orm:form_widgets` + `// orm:typed_fields` | DB + form + typed accessors |

> **Always emitted, every struct:** `ModelName()` (shared model identity) and
> `Schema()`/`Pointers()`/`<Name>List` (the `fmt.Fielder` surface the `json` codec needs).
> `no_db` suppresses only the DB **helpers** (`ReadOne`/`ReadAll`/`<Name>_`), never the
> identity or the Fielder surface.

**Migration from the old scheme** (mechanical, breaking):

| Old | New |
|---|---|
| `// ormc:form` | `// orm:form_widgets` |
| `// ormc:formonly` | `// orm:form_widgets` + `// orm:no_db` |
| `-fields` flag | `// orm:typed_fields` |

Rationale:
- **`orm:` prefix** names the owning domain — not jargon like `ormc`, not over-generic like
  `gen` (ambiguous about which generator owns the directive).
- **Orthogonal/atomic** beats the old `form` / `formonly` pair: `formonly` baked *two*
  decisions (add form, drop DB) into a non-parallel "subtraction" suffix. Now each directive
  does one thing and never changes meaning; the form-without-DB case is an honest
  composition (`form_widgets` + `no_db`).
- **`typed_fields`** describes the artifact (typed field accessors `User_.Name`) and — unlike
  a `query` name — does not over-claim (DB structs are queryable via `ReadOne`/`ReadAll`
  regardless).

Deeper rationale / rejected alternatives (global flag, `form_only` pair, `gen:`/`ormc:`
prefixes, `query`/`fields` names): see [DESIGN.md](DESIGN.md) *(create on demand)*.

### 3.1 `orm:typed_fields` is per-struct, opt-in — no global flag

```go
// orm:typed_fields — emit the CatalogItem_ descriptor for type-safe queries
type CatalogItem struct { ... }
```

- **Single mechanism — the directive is the only way.** The global `-fields` CLI flag is
  **removed entirely** (flag, `SetFields`, `withFields`). Two ways to enable the same thing
  invites confusion; the directive wins because it is per-struct, which the flag can never be.
  Effective condition = `info.WantTypedFields`, nothing else.
- **Needs the DB layer** — meaningless on a `// orm:no_db` struct (see §5).

---

## 4. Implementation Steps

### Step 1 — Parsing: atomic directive detection + `StructInfo` fields
**Files:** [ormc.go](../ormc.go), [ormc_tags.go](../ormc_tags.go)

In `StructInfo` (`ormc.go:48-59`): **rename `FormOnly` → `NoDB`** (it always meant "no DB
layer"; the new name matches the directive) and **add** `WantTypedFields`:
```go
type StructInfo struct {
    ...
    IsForm          bool // has // orm:form_widgets  → widgets + Validate
    NoDB            bool // has // orm:no_db → suppress DB layer (was FormOnly)
    WantTypedFields bool // has // orm:typed_fields → emit <Name>_ descriptors
    ...
}
```

In `ParseStruct`:
- Replace the two locals `var isForm bool` / `var formOnly bool` (`ormc.go:113-114`) with
  `var isForm, noDB, wantTypedFields bool`.
- **Replace** the matching loop nested inside `ast.Inspect` (`ormc.go:124-135`). All three
  directives are independent — none is a substring of another, so no ordering hack, no `break`:
  ```go
  if genDecl.Doc != nil {
      for _, comment := range genDecl.Doc.List {
          if fmt.Contains(comment.Text, "orm:typed_fields") {
              wantTypedFields = true
          }
          if fmt.Contains(comment.Text, "orm:no_db") {
              noDB = true
          }
          if fmt.Contains(comment.Text, "orm:form_widgets") {
              isForm = true
          }
      }
  }
  ```
- Populate `info.IsForm`, `info.NoDB`, `info.WantTypedFields` in the `StructInfo` literal
  (`ormc.go:155-162`).
- **json-rename gate** (`ormc.go:271`): change `if !formOnly {` → `if !noDB {` — `json:` column
  rename is allowed only when the struct is **not** persisted.
- **json-rename error message** (`ormc.go:274`): the text says *"declare the struct as
  ormc:formonly"* → update to *"…as orm:no_db"*.

**Second detection site — `ormc_tags.go:42-50`.** The tag-rewrite pass (`RewriteModelTags`)
independently scans `genDecl.Doc` for `ormc:formonly` to set its local `formOnly`. Rename that
match to `orm:no_db` (rename the local to `noDB` for consistency). Without this, tag cleanup
would mis-handle non-persisted structs under the new scheme.

> Substring check: `orm:form_widgets` is **not** a substring of `orm:no_db` or `orm:typed_fields`,
> and vice-versa → three independent `if`s are safe. (Removing `formonly` eliminates the old
> `form`-prefix collision entirely.)

### Step 2 — Generation: rename gate + gate descriptors on the per-struct flag
**File:** [ormc_generate.go](../ormc_generate.go)

- **Rename** every `info.FormOnly` → `info.NoDB` (occurrences: `~25-26`, `~74`, `~144`).
  Semantics unchanged — these already mean "skip the DB layer".
- **Always emit `ModelName()`** (behavior change): the `ModelName` block is wrapped by
  `if !info.FormOnly {` at `ormc_generate.go:46-53` — drop that wrapper so `ModelName` is
  emitted for every struct (keep the inner `!info.ModelNameDeclared` guard). Model identity is shared by
  the DB, form, and json/transport layers. Keep the existing `!info.ModelNameDeclared`
  guard (don't re-emit if the user declared it by hand). This **diverges from old
  `formonly`**, which omitted `ModelName` — intended.
- Gate the descriptor block (`ormc_generate.go:146`):
  ```go
  if o.withFields {         // before
  if info.WantTypedFields { // after
  ```
  The emitted `var %s_ = struct{...}{...}` block is unchanged.

### Step 3 — Make relation loaders descriptor-aware per child
**Files:** [ormc_relations.go](../ormc_relations.go), [ormc_generate.go](../ormc_generate.go)

Relation loaders reference `ChildStruct_.FKField`, which only exists if the **child** opted
in. Today this is the global `o.withFields` (`ormc_generate.go:178`). Move the decision to
resolution time, where the child's `StructInfo` is available:

In `RelationInfo` (`ormc_relations.go:12-18`) add:
```go
UseFieldDescriptor bool // child has // orm:typed_fields: use ChildStruct_.FKField
```
In `ResolveRelations` (`ormc_relations.go:47-53`):
```go
rel := RelationInfo{
    ...
    UseFieldDescriptor: childInfo.WantTypedFields,
}
```
In `GenerateForFile` (`ormc_generate.go:176-179`), replace `if o.withFields` with
`if rel.UseFieldDescriptor`.

### Step 4 — Remove the global `-fields` flag (single-mechanism cleanup)
**Files:** [ormc_handler.go](../ormc_handler.go), [cmd/ormc/main.go](../cmd/ormc/main.go)

- `ormc_handler.go`: delete the `withFields` field from `Ormc` and the `SetFields` method.
- `cmd/ormc/main.go`: delete the `flag.Bool("fields", ...)` declaration and the
  `o.SetFields(*fields)` call. Remaining flags (e.g. `-root`) untouched.
- Grep `o.withFields` across the package → must be **zero** references after Steps 2-3.

### Step 5 — Rename directives in test fixtures
**Files:** `tests/models.go` (fixtures), `tests/ormc_tags_test.go` (inline `// ormc:formonly`),
plus any other `tests/*.go` that string-matches the old directives.

Migrate **all** fixtures and assertions to the new scheme: `ormc:form` → `orm:form_widgets`,
`ormc:formonly` → `orm:form_widgets` + `orm:no_db`. Part of the breaking rename, not optional.
Relation cases (T8/T9) live in `tests/ormc_relations_test.go`.

### Step 6 — Update documentation
**Files:** [ARQUITECTURE.md](ARQUITECTURE.md), [README.md](../README.md), [AGENTS.md](../AGENTS.md)

- Replace directive references `ormc:form`→`orm:form_widgets`; replace every `ormc:formonly` usage
  with the `orm:form_widgets` + `orm:no_db` composition (directive references only — **not** file
  paths like `ormc.go`).
- Also fix **bare prose mentions** of `formonly` that don't carry the `ormc:` prefix
  (README `:101`, `:217`; ARQUITECTURE `:60`, `:315`, `:330`) — these now mean "non-persisted /
  `no_db` structs".
- ARQUITECTURE §4.1 generation table: rebuild around the orthogonal model; `<Name>_`
  condition → `DB layer + // orm:typed_fields`.
- ARQUITECTURE §4.2 CLI flags: **remove the `-fields` row** (flag no longer exists).
- ARQUITECTURE directives table: replace with `orm:form_widgets` / `orm:no_db` / `orm:typed_fields`
  + the composition table from §3.0.
- ARQUITECTURE §"Active Plans" index (`ARQUITECTURE.md:12`): replace the stale `PLAN.md`
  description with "orthogonal `orm:` directive scheme; per-struct `// orm:typed_fields`;
  remove global `-fields` flag".
- README directives table and `T_` row: reflect the new names.

> **Out of orm repo (follow-up, track separately):** the `form-codegen` skill
> (`~/.claude/skills/form-codegen/SKILL.md`) documents `ormc:form`/`ormc:formonly` and must
> be re-synced via the `llmskill` workflow after this lands.

---

## 5. Edge Cases

- **`// orm:typed_fields` + `// orm:no_db`** → typed accessors need the DB layer, so they are
  **not** generated. Emit a `log` warning: *"orm:typed_fields ignored on no_db struct <Name>"*.
- **`// orm:no_db` alone (no `orm:form_widgets`)** → valid: a pure transport struct. Emits
  `ModelName` + `Schema`/`Pointers`/`<Name>List` (for the `json` codec) but **no** DB helpers
  and **no** widgets. Not an error.
- **`// orm:form_widgets` + `// orm:no_db`** → like the old `formonly` (widgets + `Validate`, no DB),
  **plus `ModelName`** which old `formonly` omitted — see the ModelName change in Step 2.
- **Child of a relation lacks `// orm:typed_fields`** → loader must fall back to the string
  column (`"user_id"`) instead of `ChildStruct_.FKField`. The string-column branch already
  exists in `GenerateForFile`; Step 3 routes it via `rel.UseFieldDescriptor` (new). Verify the
  fallback still compiles when the child has no descriptor.
- **No struct opts in to typed_fields** → no `<Name>_` anywhere. New default (minimal output);
  no flag can force-generate. Consumers needing it add the one-line directive.
- **Multiple directives, any order** → independent `if`s (Step 1), so any combination on
  separate lines registers regardless of order.

---

## 6. Test Strategy

Run via `gotest` in `tinywasm/orm/tests/`. Extend `ormc_test.go` / `ormc_multi_test.go`.

| # | Case | Assert |
|---|------|--------|
| T1 | DB struct **with** `// orm:typed_fields` | output contains `var CatalogItem_ = struct {` + `CatalogItem_.ID` mapping |
| T2 | DB struct **without** directive | output does **not** contain `<Name>_` |
| T3 | `// orm:form_widgets` only | widgets in schema + `Validate`; DB helpers present; no `<Name>_` |
| T4 | `// orm:form_widgets` + `// orm:no_db` | widgets + `Validate` + **`ModelName`**; **no** `ReadOne`/`ReadAll`/`<Name>_` |
| T5 | `// orm:no_db` only | `ModelName` + `Schema`/`Pointers`/`List` present; no DB helpers; no widgets; no warning |
| T6 | `// orm:no_db` + `// orm:typed_fields` | no `<Name>_`; warning logged |
| T7 | `// orm:form_widgets` + `// orm:typed_fields` | widgets AND `<Name>_` both generated |
| T8 | Relation: child **with** `// orm:typed_fields` | loader uses `Child_.FKField` |
| T9 | Relation: child **without** the directive | loader uses `"fk_column"` string, compiles |
| T10 | json `name` tag on a `// orm:no_db` struct | accepted; on a persisted struct → parse error |

Golden check: generated files must `gofmt`-compile (the tests already build the output).

---

## 7. Validation (consumer self-correction)

After orm changes land + publish (`gopush`):
1. Add `// orm:typed_fields` above `CatalogItem` in `item-catalog/model.go`.
2. Re-run `ormc` in `item-catalog`.
3. Adapt the one out-of-scope call site (`mcp.go:85-92`): `len(*results)` / `range *results`
   (consequence of `2e42145`, see §2.2).
4. `gotest` in `item-catalog` → green.

> These steps are **not** part of this plan's deliverable; they confirm the design end-to-end.
