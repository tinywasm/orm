# PLAN — orm: Kind unification (phase B, runtime only) + post-split cleanup

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.
> Phase B of `tinywasm/docs/KIND_UNIFICATION_MASTER_PLAN.md` (Kind unification wave).
> Requires the published phase-A `tinywasm/model` (Kind interface).
> The generator stages of the former orm plan moved to `tinywasm/ormc`'s plan
> after the repo split (2026-07-10).

## Prerequisite (run first)

```bash
go install github.com/tinywasm/devflow/cmd/gotest@latest
```

Run all tests with `gotest` (never plain `go test`).

## Context (zero-context summary)

This repo was just split (2026-07-10). `github.com/tinywasm/orm` is now the
**runtime only**: query building (`qb.go`, `query.go`, `conditions.go`),
scanning (`scan.go`), schema sync (`sync.go`, `schema.go`), DB lifecycle
(`db.go`, `open.go`, `tx.go`). What left the repo:

| Was here | Now lives in |
|---|---|
| `ddl/` (Exporter, TopologicalSort) + `field_ext.go` (FieldExt) | `github.com/tinywasm/ddlc` |
| `ormc/` (generator) + `cmd/ormc` | `github.com/tinywasm/ormc` |
| `ormcp/` (MCP daemon provider) | `github.com/tinywasm/ormcp` |
| `cmd/ddlc` (CLI) | `github.com/tinywasm/ddlc/cmd/ddlc` |

The split is mechanical and done: those dirs are deleted here, orm compiles,
and `gotest ./...` is green with `model` pinned to v0.0.6 via `replace`.
Generated test fixtures (`tests/models_orm.go`) already reference
`ddlc.FieldExt`.

Phase A changed `tinywasm/model`: `Field.Type` is now the `Kind` interface
(`Storage() FieldType`, `Name()`, `Validate(string) error`) instead of the
`FieldType` enum, and `Field.Widget` was deleted. Definitions declare
constructors: `{Name: "email", Type: input.Email()}`. Settled rationale:
`model/docs/ARCHITECTURE.md` §8.

**Ecosystem rules:** no stdlib in WASM-shared code (`tinywasm/fmt`), no
`any`/`map` in public APIs, typed constants, errors propagate, `gotest` only.

## Stage 1 — runtime migration to `.Storage()`

Mechanical, whole-repo: every `f.Type == model.FieldX` / `switch f.Type`
becomes `f.Type.Storage()`. Grep for `\.Type` across the module (~7 files:
query building, scanning, schema sync/DDL glue toward sqlt) and migrate each
site. No behavior change intended — the enum values compared against are the
same.

Then:

- Remove the `replace github.com/tinywasm/model => ... v0.0.6` from
  `go.mod` (root AND `tests/`) and require the published phase-A model.
- Regenerate `tests/models.go` fixtures to the new authoring form
  (`Type: model.Text()` etc.) using the phase-B `ormc` — if phase-B ormc is
  not yet published, STOP and report instead of hand-writing shims.

## Stage 2 — post-split documentation

- `README.md`: remove every mention of `orm/ddl`, `orm/ormc`, `orm/ormcp`,
  `cmd/ddlc`, `cmd/ormc`; point to the new repos
  (`tinywasm/ddlc`, `tinywasm/ormc`, `tinywasm/ormcp`). Quick-start examples
  updated to the single-slot `Type:` authoring form.
- `docs/ARQUITECTURE.md`: runtime-only scope statement + the table above
  (what moved where, and why: consumers of the runtime no longer pull
  generator/DDL deps and vice versa). `FieldExt` note: FK metadata belongs
  to `ddlc`; the runtime never used it.
- `docs/WHY_GENERATED_CODE_IS_FREE.md` and `docs/SYNC_DESIGN.md` were already
  moved to `tinywasm/ormc/docs/` (their content is entirely about the
  generator/sync contract ormc owns). Fix the two dangling links left behind:
  `docs/IMPROVE.md:23` (`[WHY_GENERATED_CODE_IS_FREE.md](WHY_GENERATED_CODE_IS_FREE.md)`
  → point to the ormc repo doc, or drop the link and keep the prose) and
  `docs/ARQUITECTURE.md:9` (same link, in the doc index). `AGENTS.md`: fix
  any remaining stale references to the moved packages (mentions of
  `ormc`/`ddl` paths inside this repo).

## Harness checklist (mandatory)

- Errors propagate; nothing swallowed. Typed constants for repeated strings.
- Breaking change: next minor version. No deprecated shims (no `FieldExt`
  alias left behind).
- If the phase-A `Kind` contract is missing something the runtime needs,
  **STOP and report** — the fix lands in `tinywasm/model`'s plan.

## Acceptance criteria

1. `grep -rn "model.Field[A-Z]" --include=*.go .` shows comparisons only via
   `.Storage()` (enum constants may remain as the right-hand side).
2. `go.mod` (root and `tests/`) has no model v0.0.6 replace.
3. `grep -rn "orm/ddl\|orm/ormc\|orm/ormcp" .` → empty (code and docs).
4. `gotest ./...` green (root module and `tests/`, native + wasm).
5. Docs updated as specified.

## Stages

| Stage | File(s) | Action |
|---|---|---|
| 1 | ~7 runtime files, `go.mod`, `tests/` | `.Storage()` migration + unpin model + regen fixtures |
| 2 | `README.md`, `docs/*`, `AGENTS.md` | runtime-only scope, new repo pointers |
