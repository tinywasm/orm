# PLAN — Kind unification (phase B): ormc parses `Type:` constructor expressions, runtime uses `Storage()`

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.
> Phase B of `tinywasm/docs/KIND_UNIFICATION_MASTER_PLAN.md` (Kind unification wave). Requires
> the published phase-A `tinywasm/model` (Kind interface). Runs parallel to
> form/sqlt/postgres/mcp.

## Context (zero-context summary)

Phase A changed `tinywasm/model`:

```go
type Kind interface {
    Storage() FieldType          // deterministic storage mapping (enum survives behind this)
    Name() string
    Validate(value string) error // always present — fail-closed
}

type Field struct {
    Name string
    Type Kind   // was: Type FieldType + Widget Widget (Widget DELETED)
    ...
}
```

Definitions are now authored as:

```go
{Name: "email",  Type: input.Email(), NotNull: true}  // form kind (tinywasm/form/input)
{Name: "status", Type: model.Text()}                   // base kind
{Name: "address", Type: model.Struct(&AddressModel)}   // composition: ref in the constructor
{Name: "staff_id", Type: model.Int(), Ref: &StaffModel} // scalar FK: Ref keeps ONLY this meaning
```

**Composition follow-up (already merged in `model`):** `Struct(ref)` /
`StructSlice(ref)` take the nested `*Definition` as a REQUIRED constructor
argument; the `RefKind` interface (`Kind` + `Ref() *Definition`) exposes it.
`Field.Ref` no longer carries composition — its single remaining meaning is
the scalar foreign key (drives `SchemaExt()`/DDL FK, Go type stays scalar).
Settled rationale: `model/docs/ARCHITECTURE.md` §8.

This repo owns two affected surfaces:

1. **`ormc` (the generator)** parses `model.Definition` literals from source
   (`ormc/parse_definition.go`) — it currently reads `Type:` as an enum
   selector (`model.FieldText`) and `Widget:` as a constructor expression
   (`WidgetConstructor`, via `exprToString`). It must now read `Type:` as a
   constructor expression and reject `Widget:`.
2. **`orm` runtime** compares `f.Type` against enum values in ~7 files
   (query building, scanning, DDL via sqlt). Every such site becomes
   `f.Type.Storage()`.

**Ecosystem rules:** no stdlib in WASM-shared code (`tinywasm/fmt`), no
`any`/`map` in public APIs, typed constants, errors propagate, `gotest` only.

## Stage 1 — parser: `Type:` is a constructor expression, `Widget:` is an error

In `ormc/parse_definition.go`:

- The `Type` case captures the expression verbatim (reuse the `exprToString`
  machinery that `WidgetConstructor` uses today) into `KindConstructor`.
- The `Widget` case becomes a **hard generation error** with an actionable
  message: `Field.Widget was removed (Kind unification): declare the kind in
  Type — e.g. Type: input.Email()`. Never silently ignore it.
- A field without `Type:` is a hard generation error (`field X: kind
  required`) — this is the compile-time guard for the fail-closed contract;
  model's runtime error is only the backstop.
- **Composition refs come from the constructor argument, not `Ref:`**: for
  `model.Struct(<arg>)` / `model.StructSlice(<arg>)`, capture the argument
  expression (e.g. `&AddressModel`) — it replaces today's `Ref:`-based
  nested-type resolution (`parse_definition.go` currently errors when
  FieldStruct/FieldStructSlice has empty `fi.Ref`; that check retargets to
  the constructor arg). Missing or `nil` argument = hard generation error.
- **Contradiction rule** (from the model follow-up): `Ref:` set on a field
  whose `Type:` is a composition constructor = hard generation error. `Ref:`
  on scalar fields keeps today's parse and meaning (FK → `SchemaExt()`).
- Delete `WidgetConstructor` once nothing reads it.

## Stage 2 — storage resolution (generation-time `Storage()`)

ormc emits Go struct fields, so it needs each kind's `FieldType` **at
generation time** — it parses source and cannot call `Storage()`. Resolution
rules, in order:

1. **Built-in table for `model.*` base kinds** (closed set from phase A):
   `model.Text→FieldText`, `model.Int→FieldInt`, `model.Float→FieldFloat`,
   `model.Bool→FieldBool`, `model.Blob→FieldBlob`, `model.Raw→FieldRaw`,
   `model.Struct→FieldStruct`, `model.IntSlice→FieldIntSlice`,
   `model.StructSlice→FieldStructSlice`. The two composition entries are
   parameterized: their generated Go field type derives from the constructor
   argument captured in stage 1 (today `FieldTypeToGoType(fi.Type, fi.Ref)`
   builds it from the parsed `Ref:` string — same mapping, new source).
2. **Directive comment for every other constructor** (form/input kinds and
   project-custom kinds): the constructor declaration carries
   `//ormc:storage <text|int|float|bool|blob|raw|struct|intslice|structslice>`
   immediately above it. ormc locates the constructor's package source via
   `tinywasm/modfind` (the ecosystem's `go list -m` wrapper — do NOT shell
   out to `go list` directly or duplicate that logic) and reads the
   directive. Phase B of `tinywasm/form` adds these directives to every
   `input.*` constructor in the same wave.
3. **No directive found → hard generation error** naming the constructor and
   the expected directive syntax. Fail loud at generation, never guess a
   storage type.

The directive name is a typed constant shared by parser and error messages.

## Stage 3 — codegen output

In `ormc/generate.go`:

- Generated `Schema()` emits `Type: <constructor expr>` verbatim (exactly as
  it emits `Widget:` today), and the import collector gathers the
  constructors' packages (existing widget-import logic, retargeted).
- Struct field Go types come from the resolved storage (stage 2) — the
  deterministic mapping table is unchanged.
- `hasWidget`-style bookkeeping is retargeted: every field now has a kind;
  what varies is only which packages need importing.
- Generated `Validate()`/codec methods are unchanged in shape (they already
  delegate to `model.ValidateFields`, which phase A made fail-closed).
- The `<Struct>_.Campo` typed-field helpers (e.g. `ServiceItem_.SKU`, used
  by production queries `.Where(ServiceItem_.SKU).Eq(...)`) MUST keep being
  generated exactly as today — add a regression assertion if none exists.

## Stage 4 — orm runtime migration

Mechanical, whole-repo: every `f.Type == model.FieldX` /
`switch f.Type` becomes `f.Type.Storage()`. Grep for `\.Type` across the
module (≈7 files, includes DDL glue toward sqlt) and migrate each. No
behavior change intended — the enum values compared against are the same.

## Stage 5 — tests

- Parser: fixture Definition using `model.Text()`, `model.Int()`, and
  `input.Email()`; assert generated struct types, `Schema()` constructor
  round-trip, and imports.
- Error paths: `Widget:` present → error; missing `Type:` → error; unknown
  constructor without directive → error naming the directive syntax;
  `model.Struct(nil)` / composition constructor without argument → error;
  `Ref:` alongside a composition constructor → error (contradiction rule).
- Composition round-trip: fixture with `Type: model.Struct(&ChildModel)`
  generates the nested Go type and keeps `SchemaExt()`/FK output for a
  scalar `Ref:` field byte-identical to today.
- Directive resolution: a fixture custom kind with `//ormc:storage text`
  resolves; same kind without directive fails.
- Regenerate this repo's own test models; full `gotest ./...` green
  (native + wasm), including ormcp.

## Stage 6 — documentation

- `docs/ARQUITECTURE.md`: the `Type:`-expression rule, the storage
  resolution order (built-ins → directive → error), and the `Widget:`
  removal.
- `README.md` quick-start Definition examples updated to single-slot form.

## Harness checklist (mandatory)

- Pin the phase-A `tinywasm/model` version in `go.mod`.
- Directive keyword and error verbs are typed constants — no repeated string
  literals.
- Use `tinywasm/modfind` for package location — never a duplicated
  `go list` shell-out (ecosystem rule: no forked dependency logic).
- If the phase-A `Kind` contract is missing something this generator needs,
  **STOP and report** — the fix lands in `tinywasm/model`'s plan, never as a
  local workaround.
- Breaking change: next minor version. No deprecated shims.

## Acceptance criteria

1. A Definition with `Widget:` fails generation with the actionable message;
   one missing `Type:` fails naming the field.
2. `model.*` and `input.*` kinds resolve to correct struct field types;
   custom kinds resolve via directive; unknown kinds fail loud.
2b. Composition: nested Go types come from the `Struct(ref)`/`StructSlice(ref)`
   argument; nil/missing arg and `Ref:`+composition both fail generation;
   scalar-FK `Ref:` output (`SchemaExt()`) unchanged.
3. `grep -rn "WidgetConstructor" ormc/` → empty.
4. `gotest ./...` green (orm runtime behavior unchanged under `.Storage()`).
5. Docs updated as specified.

## Stages

| Stage | File(s) | Action |
|---|---|---|
| 1 | `ormc/parse_definition.go` | `Type:` as expr; `Widget:` hard error; kind required; composition ref from constructor arg; `Ref:`+composition contradiction error |
| 2 | `ormc/` (new resolution unit) | built-in table + `//ormc:storage` directive via modfind |
| 3 | `ormc/generate.go` | emit `Type:` constructors, imports, storage-mapped struct fields |
| 4 | orm runtime (~7 files) | `f.Type` enum sites → `f.Type.Storage()` |
| 5 | `ormc/*_test.go`, regenerated fixtures | parser/resolution/error tables |
| 6 | `docs/ARQUITECTURE.md`, `README.md` | authoring + resolution rules |
