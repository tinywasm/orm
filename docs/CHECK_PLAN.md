# PLAN — ormc: always generate `Validate` (secure by default)

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.

You are an external agent with **zero prior context** about this project. Everything you
need is in this file. Read it fully before writing code.

---

## Development Rules

- **tinywasm ecosystem style:** this repo uses `github.com/tinywasm/fmt` instead of the
  standard `fmt`/`strings`/`strconv`. Follow the existing imports in each file — do not
  introduce standard-library packages into files that don't already use them.
- **No hardcoded strings:** repeated string literals (paths, prefixes, names) must be named
  constants. This plan does not add any new strings, so no new constants are needed.
- **Generated code is disposable:** files ending in `_orm.go` are generator output
  (`// DO NOT EDIT`). Never hand-edit them; change the generator and regenerate.
- **Do not run `gopush` or `codejob`** — those are local developer tools managed outside
  this task. You only modify code, tests, and docs, and run `go test ./...`.
- **Documentation first:** the doc change in Stage 3 is part of the deliverable, not optional.

---

## 1. Context and problem

`ormc` (package `ormc/` in this module) generates ORM/codec plumbing for models declared as
`model.Definition` literals. For each model it emits into `*_orm.go`: the struct, `Schema()`,
`Pointers()`, `EncodeFields`/`DecodeFields`, `IsNil()`, `ModelName()`, list types — and,
**only conditionally**, a `Validate(action byte) error` method.

The condition lives in `ormc/generate.go` (~line 308):

```go
hasValidation := info.IsForm || info.HasAnyInputTag
if !hasValidation {
    for _, f := range info.Fields {
        if f.NotNull || f.Letters || f.Numbers || f.Tilde || f.Spaces ||
            len(f.Extra) > 0 || f.Minimum > 0 || f.Maximum > 0 {
            hasValidation = true
            break
        }
    }
}

if hasValidation {
    buf.Write(fmt.Sprintf("func (m *%s) Validate(action byte) error {\n", info.Name))
    buf.Write("\treturn model.ValidateFields(action, m)\n")
    buf.Write("}\n\n")
}
```

This conditional generation is a defect, for four confirmed reasons:

1. **It breaks a compile-time contract.** `github.com/tinywasm/mcp` binds tool arguments
   through this interface (`mcp/tools.go`):

   ```go
   type DecodableFields interface {
       model.Decodable
       Validate(action byte) error
   }

   func (r *Request) Bind(target DecodableFields) error { ... }
   ```

   Any ormc-generated model used as MCP tool args **must** have `Validate`. Models whose
   definitions carry no validation rules (e.g. `{Name: "lines", Type: model.FieldInt}` only)
   get no `Validate` and **fail to compile** at every `req.Bind(&args)` call site. This is
   currently breaking the build of `github.com/tinywasm/devbrowser`.

2. **It violates the ecosystem's secure-by-default principle.** Validation must be the
   default path. If a developer later adds `NotNull: true` to a field, validation must
   already be wired — not depend on regenerating a method that appears and disappears.

3. **The condition is incomplete and unmaintainable.** `model.Field.Validate` also honors
   `BreakLine`, `Tab`, `NotAllowed`, and `StartWith` rules, none of which are checked by the
   `hasValidation` loop — a model using only those rules would silently get no `Validate`.
   Every new rule added to `model.Permitted` would have to be mirrored here.

4. **The docs already promise it unconditionally.** `README.md` ("Generated per model")
   lists `Validate(action byte) error` with no condition.

Always generating `Validate` is free: `model.ValidateFields` iterates the schema and, for
fields with no rules, does nothing and returns `nil` (verified in
`github.com/tinywasm/model/field.go` — `Field.Validate` returns `nil` when no rules are
configured). It is one tiny method per model, and generated code costs nothing
(see `docs/WHY_GENERATED_CODE_IS_FREE.md`).

**Decision (already resolved — do not re-litigate):** `Validate` is generated for **every**
model, always. The rejected alternative — declaring a fake `Widget:` on consumer models to
trip the `IsForm` flag — would pull the `tinywasm/form/input` UI dependency into headless
server tools and put a lie into every consumer's model definitions to work around a
generator bug.

---

## 2. Changes

### 2.1 `ormc/generate.go` — emit `Validate` unconditionally

Delete the entire `hasValidation` computation and the surrounding `if`. Replace the block
quoted in section 1 with:

```go
buf.Write(fmt.Sprintf("func (m *%s) Validate(action byte) error {\n", info.Name))
buf.Write("\treturn model.ValidateFields(action, m)\n")
buf.Write("}\n\n")
```

Only the single-model `Validate` is affected. Do **not** add `Validate` to the generated
`TList` types.

### 2.2 `ormc/generator.go` — remove dead field

`StructInfo` has a field that is read by the old condition but **never assigned anywhere**
(leftover from the removed struct-tag parser):

```go
HasAnyInputTag bool // true when ≥1 field has input: tag (including input:"-")
```

Delete it. After 2.1 it has no remaining readers; verify with a project-wide grep that no
other reference exists before and after removal.

### 2.3 Tests

In `ormc/generator_test.go` (or `tests/ormc_test.go`, whichever already exercises
`GenerateForFile` output — inspect both and follow the existing pattern):

- Add/extend a test with a model definition that has **zero** validation rules
  (no `NotNull`, no `Permitted`, no `Widget`), e.g.:

  ```go
  var PingArgsModel = model.Definition{
      Name: "ping_args",
      Fields: model.Fields{
          {Name: "count", Type: model.FieldInt},
      },
  }
  ```

- Assert the generated output contains
  `func (m *PingArgs) Validate(action byte) error` and
  `return model.ValidateFields(action, m)`.
- Keep/verify an existing case with rules (e.g. `NotNull: true`) still generates `Validate`.
- Run the full suite: `go test ./...` from the module root. All packages must pass,
  including `tests/` (round-trip generation) and `ormc/parse_generated_test.go`
  (sync/idempotency parsing) — the parser must tolerate the now always-present method.

### 2.4 `README.md` — one-line doc touch

In the "Generated per model" list, change the `Validate` bullet to make the guarantee
explicit:

```markdown
- `Validate(action byte) error` (calling `model.ValidateFields`). Always generated —
  validation is secure by default; models with no rules get a cheap no-op.
```

---

## 3. Acceptance criteria

1. `GenerateForFile` output contains a `Validate` method for **every** model info,
   regardless of rules, widgets, or `NoDB`.
2. `HasAnyInputTag` no longer exists in the codebase.
3. `go test ./...` passes at the module root.
4. `README.md` documents the always-generated guarantee.

---

## 4. Out of scope (do NOT do here)

- Regenerating `*_orm.go` files in consumer repos (`devbrowser`, `ormcp` models, etc.).
  Consumers regenerate after this module is published.
- Fixing devbrowser's unrelated pre-existing compile errors (`int` vs `int64` mismatches,
  `Url` vs `URL`). Those belong to devbrowser's own plan.
- Changing `model.ValidateFields` semantics in `tinywasm/model`.

---

## 5. Stages

| # | Stage | Files | Output |
|---|-------|-------|--------|
| 1 | Unconditional `Validate` emission | `ormc/generate.go` | `hasValidation` removed; method always emitted |
| 2 | Dead-code removal | `ormc/generator.go` | `HasAnyInputTag` deleted |
| 3 | Tests | `ormc/generator_test.go` / `tests/ormc_test.go` | Rule-less model asserts `Validate` present; `go test ./...` green |
| 4 | Docs | `README.md` | "Always generated" guarantee documented |
