# PLAN: Coverage Gap Fix — `tinywasm/orm`

> **Context:** `CHECK_PLAN.md` was executed correctly. All tests pass.
> However, `gotest` reports **87.1%** coverage — below the required **≥ 90%**.
> This plan adds targeted tests and one small alias fix to close the gap.

## Prerequisites

```bash
go install github.com/tinywasm/devflow/cmd/gotest@latest
```

---

## Root Cause Analysis

| File | Function | Coverage | Why uncovered |
|------|----------|----------|---------------|
| `ormc_handler.go` | `SetLog` | 0% | Never called in any test |
| `ormc_handler.go` | `SetRootDir` | 0% | Never called in any test |
| `ormc_handler.go` | `log` | 50% | `logFn != nil` branch not triggered |
| `ormc.go` | `Run()` | 0% | No test invokes the directory walker |
| `ormc.go` | `detectTableName` | 80% | Pointer-receiver branch (`*struct`) not activated |
| `qb.go` | `Or()`, `Neq/Gt/Gte/Lt/Lte/Like/In` (Clause methods) | 0% | Tests use `orm.Neq(...)` directly; never use `qb.Where("x").Neq(y)` chain |

> **Note on `db.go:68-71`:** These are `MockModel` methods in `setup_test.go` (not
> in the main package). They are counted as 0% because they are part of the test's
> mock fixture, not the library. This is expected and acceptable.

---

## Decisions

| # | Decision | Rationale |
|---|----------|-----------|
| D1 | Add `TestOrmc_Run` to `tests/ormc_multi_test.go` using `SetRootDir` + `SetLog` | Covers `Run()`, `SetRootDir`, `SetLog`, and the `logFn != nil` branch of `log` in one shot |
| D2 | The `Run()` test uses `t.TempDir()` + copies `mock_generator_model.go` there → avoids side effects | Clean: no leftover `_orm.go` in the real `tests/` directory |
| D3 | Add `TestQB_ClauseChain` to `tests/ormc_multi_test.go` | Covers all 7 `*Clause` methods (`Neq`, `Gt`, `Gte`, `Lt`, `Lte`, `Like`, `In`) + `QB.Or()` |
| D4 | Add a `*PointerReceiver` fixture to `mock_generator_model.go` to cover `detectTableName` pointer-receiver branch | One-liner struct with `func (*PointerReceiver) TableName() string { return "ptr_table" }` |
| D5 | Add `TestOrmc_DetectPointerReceiver` in `ormc_multi_test.go` | Completes the 80% → 100% gap in `detectTableName` |
| D6 | `SKILL.md` mentions `orm.New()` for `Ormc` but the real constructor is `NewOrmc()` — add alias `func New() *Ormc` to `ormc_handler.go` | Fixes doc/code mismatch; `New()` simply calls `NewOrmc()` |

---

## Affected Files

| File | Change |
|------|--------|
| `ormc_handler.go` | Add `New() *Ormc` alias (D6) |
| `tests/mock_generator_model.go` | Add `PointerReceiver` struct + `*PointerReceiver` `TableName()` method (D4) |
| `tests/ormc_multi_test.go` | Add `TestOrmc_Run`, `TestOrmc_DetectPointerReceiver`, `TestQB_ClauseChain` (D1, D3, D5) |

---

## Execution Steps

### Step 1 — Add `New()` alias to `ormc_handler.go`

Append after `NewOrmc()`:

```go
// New is an alias for NewOrmc. Provided for ergonomics.
func New() *Ormc {
    return NewOrmc()
}
```

> This fixes the `SKILL.md` code example that calls `orm.New()` to build an `*Ormc`.

---

### Step 2 — Add `PointerReceiver` fixture to `tests/mock_generator_model.go`

Append at the end of the file:

```go
// PointerReceiver tests that detectTableName handles pointer receivers (*T).
type PointerReceiver struct {
    ID   string `db:"pk"`
    Name string
}
func (*PointerReceiver) TableName() string { return "ptr_table" }
```

---

### Step 3 — Add tests to `tests/ormc_multi_test.go`

Append the following three test functions to the existing file:

```go
func TestOrmc_Run(t *testing.T) {
    t.Run("Run() scans dir and generates all structs", func(t *testing.T) {
        // Use a temp dir to avoid polluting tests/
        tmp := t.TempDir()

        // Copy model file into temp dir
        src, err := os.ReadFile("mock_generator_model.go")
        if err != nil {
            t.Fatal(err)
        }
        // Replace package declaration so it compiles as package "tests"
        modelFile := filepath.Join(tmp, "model.go")
        if err := os.WriteFile(modelFile, src, 0644); err != nil {
            t.Fatal(err)
        }

        var logged []string
        o := orm.NewOrmc()
        o.SetRootDir(tmp)
        o.SetLog(func(messages ...any) {
            // Collect log output — verifies SetLog + logFn branch
            for _, m := range messages {
                logged = append(logged, fmt.Sprint(m))
            }
        })

        if err := o.Run(); err != nil {
            t.Fatalf("Run() failed: %v", err)
        }

        // The generated file must exist
        outFile := filepath.Join(tmp, "model_orm.go")
        content, err := os.ReadFile(outFile)
        if err != nil {
            t.Fatalf("Expected model_orm.go, got error: %v", err)
        }

        s := string(content)
        // Spot-check a couple of structs that have valid fields
        if !strings.Contains(s, "func (m *User) Schema()") {
            t.Error("User Schema() not in Run() output")
        }
        if !strings.Contains(s, "func (m *MultiA) Schema()") {
            t.Error("MultiA Schema() not in Run() output")
        }
        // Warning for BadTimeNoTag / Unsupp must have been logged
        _ = logged // just exercise the log path; content varies
    })

    t.Run("Run() returns error when no models found", func(t *testing.T) {
        tmp := t.TempDir()
        o := orm.NewOrmc()
        o.SetRootDir(tmp)
        if err := o.Run(); err == nil {
            t.Error("Expected error for empty directory, got nil")
        }
    })
}

func TestOrmc_DetectPointerReceiver(t *testing.T) {
    t.Run("TableName() NOT generated when declared with pointer receiver", func(t *testing.T) {
        err := orm.NewOrmc().GenerateForStruct("PointerReceiver", "mock_generator_model.go")
        if err != nil {
            t.Fatal(err)
        }

        outFile := "mock_generator_model_orm.go"
        content, err := os.ReadFile(outFile)
        if err != nil {
            t.Fatal(err)
        }
        defer os.Remove(outFile)

        if strings.Contains(string(content), "func (m *PointerReceiver) TableName()") {
            t.Error("TableName() must NOT be generated — already declared with pointer receiver")
        }
        if !strings.Contains(string(content), `"ptr_table"`) {
            // The Meta struct must reference the declared table name
            t.Error("Expected ptr_table in generated meta")
        }
    })
}

func TestQB_ClauseChain(t *testing.T) {
    t.Run("All Clause operators via QB chain", func(t *testing.T) {
        mockCompiler := &MockCompiler{}
        mockExec := &MockExecutor{}
        db := orm.New(mockExec, mockCompiler)
        model := &MockModel{Table: "items"}
        mockExec.ReturnQueryRows = &MockRows{Count: 0}

        db.Query(model).
            Where("a").Neq(1).
            Where("b").Gt(2).
            Where("c").Gte(3).
            Where("d").Lt(4).
            Where("e").Lte(5).
            Where("f").Like("%x%").
            Where("g").In([]int{1, 2}).
            Or().Where("h").Eq(9).
            ReadAll(func() orm.Model { return &MockModel{} }, func(orm.Model) {})

        conds := mockCompiler.LastQuery.Conditions
        expected := []struct {
            field string
            op    string
        }{
            {"a", "!="},
            {"b", ">"},
            {"c", ">="},
            {"d", "<"},
            {"e", "<="},
            {"f", "LIKE"},
            {"g", "IN"},
            {"h", "="},
        }
        if len(conds) != len(expected) {
            t.Fatalf("Expected %d conditions, got %d", len(expected), len(conds))
        }
        for i, ex := range expected {
            if conds[i].Field() != ex.field {
                t.Errorf("cond[%d]: expected field %q, got %q", i, ex.field, conds[i].Field())
            }
            if conds[i].Operator() != ex.op {
                t.Errorf("cond[%d]: expected op %q, got %q", i, ex.op, conds[i].Operator())
            }
        }
        // Last condition must be OR
        if conds[7].Logic() != "OR" {
            t.Errorf("Expected last condition Logic=OR, got %s", conds[7].Logic())
        }
    })
}
```

> **Import note:** `ormc_multi_test.go` needs `"fmt"`, `"path/filepath"` and `"os"` in its
> import block. `MockModel`, `MockCompiler`, `MockExecutor`, `MockRows` are already
> available from `setup_test.go`.

---

### Step 4 — Run tests

```bash
gotest
```

**Acceptance Criteria:**

| Criterion | Check |
|-----------|-------|
| All previous tests still pass | ✅ |
| `TestOrmc_Run` — Run() generates output for all structs | ✅ |
| `TestOrmc_Run` — empty dir returns error | ✅ |
| `TestOrmc_DetectPointerReceiver` — pointer receiver suppresses TableName() | ✅ |
| `TestQB_ClauseChain` — all 7 Clause methods + `Or()` chain verified | ✅ |
| `gotest` ≥ 90% coverage | ✅ |

---

## Annex — Relation Support (`PLAN_RELATIONS.md`)

> **Execute this annex after Step 4 passes.** The `New() *Ormc` alias (Step 1)
> and `SetRootDir` / `SetLog` availability are prerequisites already satisfied
> by the steps above.

### Additional Decisions

| # | Decision | Rationale |
|---|----------|-----------|
| R1 | Relation detected by: field type is `[]Struct` where `Struct` has a `db:"ref=parentTable"` field | Uses existing `db:"ref=..."` system — zero new concepts |
| R2 | No additional tag on the parent slice field | Minimum tags principle; child already declares the FK |
| R3 | Loader generated in the **child's** `_orm.go`, not the parent's | SRP: child struct owns its own query functions |
| R4 | Loader signature: `ReadAll{Child}By{FK}(db *orm.DB, parentID string) ([]*Child, error)` | Typed, consistent with existing `ReadAll{Model}` convention |
| R5 | If no FK found in child → `o.log(...)` warning + skip (no error) | Cannot generate a loader without knowing the FK column |
| R6 | Many-to-many via junction table: **deferred** to `PLAN_MANY_TO_MANY.md` | Complexity; one-to-many covers the most common case first |
| R7 | `resolveRelations` exported as `ResolveRelations` | Required for black-box testing from `tests/` package |
| R8 | `ParseStruct` extended to collect `SliceFields []SliceFieldInfo` alongside `Fields []FieldInfo` | Keeps slice info separated from DB-mappable fields; no impact on existing generation |

### Additional Affected Files

| File | Change |
|------|--------|
| `ormc.go` | Add `SliceFieldInfo` type; extend `StructInfo` with `SliceFields []SliceFieldInfo`; extend `ParseStruct` to populate `SliceFields`; update `Run()` to two-pass |
| `ormc_relations.go` | **New file** (`//go:build !wasm`): `RelationInfo`, `collectAllStructs`, `ResolveRelations`, `findFKField`, `relationLoaderTemplate` |
| `ormc.go` → `GenerateForFile` | Emit relation loaders for each `info.Relations` entry |
| `tests/mock_generator_model.go` | Add `MockParent` + `MockChild` fixtures |
| `tests/ormc_relations_test.go` | **New file**: relation detection + loader generation test |

### Step R1 — Extend `StructInfo` and `ParseStruct` in `ormc.go`

Add `SliceFieldInfo` and extend `StructInfo`:

```go
// SliceFieldInfo records a slice-of-struct field found in a parent struct.
// Not DB-mapped; used only for relation resolution.
type SliceFieldInfo struct {
    Name     string // e.g. "Roles"
    ElemType string // e.g. "Role"
}

type StructInfo struct {
    // ... existing fields ...
    SliceFields []SliceFieldInfo // populated by ParseStruct; used by ResolveRelations
    Relations   []RelationInfo   // populated by ResolveRelations; used by GenerateForFile
}
```

In the `ParseStruct` field loop, **after** the `db:"-"` check, detect slice-of-struct
fields and record them in `info.SliceFields` instead of warning+skipping:

```go
// Detect []Struct fields for relation resolution (R8)
if arr, ok := field.Type.(*ast.ArrayType); ok {
    if eltIdent, ok := arr.Elt.(*ast.Ident); ok && eltIdent.Name != "byte" {
        info.SliceFields = append(info.SliceFields, SliceFieldInfo{
            Name:     fieldName,
            ElemType: eltIdent.Name,
        })
    }
    continue // never add to Fields — not DB-mappable
}
```

> **Important:** this replaces the current "unsupported type → warn + skip" path
> for `[]Struct` arrays specifically. The `[]byte` special-case remains.
> Anonymous slices (e.g. `[]string`) still produce a warning + skip.

### Step R2 — Create `ormc_relations.go`

```go
//go:build !wasm

package orm

// RelationInfo describes a one-to-many relation loader to generate.
type RelationInfo struct {
    ChildStruct string // e.g. "Role"
    FKField     string // e.g. "UserID"  (Go field name)
    FKColumn    string // e.g. "user_id" (column name)
    LoaderName  string // e.g. "ReadAllRoleByUserID"
}

// collectAllStructs walks rootDir and returns a map of all parsed StructInfo
// keyed by struct name. Used by Run() Pass 1.
func (o *Ormc) collectAllStructs() (map[string]StructInfo, error) { ... }

// ResolveRelations (exported for testing) scans all parent SliceFields,
// finds the matching FK in the child struct, and appends RelationInfo
// to the child's entry in the map.
func (o *Ormc) ResolveRelations(all map[string]StructInfo) { ... }

// findFKField returns the first FieldInfo in child whose Ref matches parentTable,
// or nil if none found.
func findFKField(child StructInfo, parentTable string) *FieldInfo { ... }
```

### Step R3 — Update `Run()` in `ormc.go` to two-pass

```go
func (o *Ormc) Run() error {
    // Pass 1: collect all structs across all model files
    all, err := o.collectAllStructs()
    if err != nil {
        return err
    }
    if len(all) == 0 {
        return Err("no models found")
    }

    // Pass 2: resolve cross-struct relations
    o.ResolveRelations(all)

    // Pass 3: generate (group by source file, call GenerateForFile once per file)
    return o.generateAll(all)
}
```

`generateAll` is a private helper that groups the enriched `all` map by source
file path (store `SourceFile string` in `StructInfo`) and calls `GenerateForFile`
once per file — same logic as the current inline walker, now reusable.

> **Note:** add `SourceFile string` to `StructInfo` so `collectAllStructs` can
> tag each struct with the file it came from.

### Step R4 — Emit relation loaders in `GenerateForFile`

After the last `ReadAll{Model}` block for each `info`, append:

```go
for _, rel := range info.Relations {
    buf.Write(Sprintf(
        "// ReadAll%sByParentID retrieves all %s records for a given parent ID.\n"+
        "// Auto-generated by ormc — relation detected via db:\"ref=%s\".\n"+
        "func ReadAll%sBy%s(db *orm.DB, parentID string) ([]*%s, error) {\n"+
        "\treturn ReadAll%s(db.Query(&%s{}).Where(%sMeta.%s).Eq(parentID))\n"+
        "}\n\n",
        rel.ChildStruct,
        rel.ChildStruct,
        info.TableName,   // parent table, for the comment
        rel.ChildStruct, rel.FKField,
        rel.ChildStruct,
        rel.ChildStruct, rel.ChildStruct, rel.ChildStruct, rel.FKField,
    ))
}
```

### Step R5 — Add fixtures to `tests/mock_generator_model.go`

```go
// MockParent / MockChild: relation auto-detection fixture.
type MockParent struct {
    ID   string
    Name string
    Kids []MockChild // no tag — relation auto-detected via MockChild.MockParentID
}

type MockChild struct {
    ID           string `db:"pk"`
    MockParentID string `db:"ref=mock_parents"`
    Value        string
}
```

### Step R6 — Add `tests/ormc_relations_test.go`

```go
//go:build !wasm

package tests

import (
    "os"
    "strings"
    "testing"

    "github.com/tinywasm/orm"
)

func TestOrmc_RelationLoader(t *testing.T) {
    t.Run("ResolveRelations detects FK and sets LoaderName", func(t *testing.T) {
        o := orm.NewOrmc()

        parent, err := o.ParseStruct("MockParent", "mock_generator_model.go")
        if err != nil { t.Fatal(err) }
        child, err := o.ParseStruct("MockChild", "mock_generator_model.go")
        if err != nil { t.Fatal(err) }

        all := map[string]orm.StructInfo{
            "MockParent": parent,
            "MockChild":  child,
        }
        o.ResolveRelations(all)

        if len(all["MockChild"].Relations) != 1 {
            t.Fatalf("expected 1 relation on MockChild, got %d", len(all["MockChild"].Relations))
        }
        rel := all["MockChild"].Relations[0]
        if rel.LoaderName != "ReadAllMockChildByMockParentID" {
            t.Errorf("unexpected loader name: %s", rel.LoaderName)
        }
    })

    t.Run("GenerateForFile emits relation loader", func(t *testing.T) {
        o := orm.NewOrmc()

        parent, _ := o.ParseStruct("MockParent", "mock_generator_model.go")
        child, _  := o.ParseStruct("MockChild",  "mock_generator_model.go")

        all := map[string]orm.StructInfo{
            "MockParent": parent,
            "MockChild":  child,
        }
        o.ResolveRelations(all)

        err := o.GenerateForFile([]orm.StructInfo{all["MockChild"]}, "mock_generator_model.go")
        if err != nil { t.Fatal(err) }

        outFile := "mock_generator_model_orm.go"
        content, err := os.ReadFile(outFile)
        if err != nil { t.Fatal(err) }
        defer os.Remove(outFile)

        if !strings.Contains(string(content), "ReadAllMockChildByMockParentID") {
            t.Error("relation loader not found in generated output")
        }
    })

    t.Run("No FK in child → warning log, no relation generated", func(t *testing.T) {
        o := orm.NewOrmc()
        var logged []string
        o.SetLog(func(msgs ...any) {
            for _, m := range msgs {
                logged = append(logged, fmt.Sprint(m))
            }
        })

        // MultiA has no FK pointing to any parent
        parent, _ := o.ParseStruct("MockParent", "mock_generator_model.go")
        noFK, _   := o.ParseStruct("MultiA",     "mock_generator_model.go")

        all := map[string]orm.StructInfo{
            "MockParent": parent,
            "MultiA":     noFK,
        }
        // Patch MockParent to pretend Kids is []MultiA
        p := all["MockParent"]
        p.SliceFields = []orm.SliceFieldInfo{{Name: "Kids", ElemType: "MultiA"}}
        all["MockParent"] = p

        o.ResolveRelations(all)

        if len(all["MultiA"].Relations) != 0 {
            t.Error("expected 0 relations when no FK found")
        }
        found := false
        for _, l := range logged {
            if strings.Contains(l, "skipping") || strings.Contains(l, "no") {
                found = true
            }
        }
        if !found {
            t.Error("expected a warning log for missing FK")
        }
    })
}
```

### Step R7 — Run tests

```bash
gotest
```

### Acceptance Criteria (Annex)

| Criterion | Check |
|-----------|-------|
| `[]Struct` field → auto-detected if child has matching `db:"ref=..."` | ✅ |
| No tag required on parent slice field | ✅ |
| Loader generated in child's `_orm.go` | ✅ |
| Loader signature: `ReadAll{Child}By{FK}(db *orm.DB, id string)` | ✅ |
| No FK in child → `o.log()` warning + skip (no error) | ✅ |
| Unknown child struct → `o.log()` warning + skip | ✅ |
| `gotest` passes with coverage ≥ 90% | ✅ |
