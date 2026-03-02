# PLAN: `ormc` Multi-Struct Bug Fix — `tinywasm/orm`

> ⚠️ **Prerequisite:** Complete [ORMC_HANDLER.md](ORMC_HANDLER.md) **first**.
> This plan assumes `Ormc` struct handler and shims are already in place.

## Prerequisites

```bash
go install github.com/tinywasm/devflow/cmd/gotest@latest
```

---

## Bug Summary

**Root cause:** `GenerateCodeForStruct` generates a complete output file for a single struct
and writes it immediately via `os.WriteFile`. When `RunOrmcCLI` finds multiple structs
in the same `model.go` / `models.go` file, it calls `GenerateCodeForStruct` once per struct,
each call **overwrites** the previous output. Only the last struct processed survives.

**Reported symptom:** `ormc` on a `models.go` with 9 structs only generates code for
`OAuthState` (the last one). The other 8 structs are missing `Schema()`, `Values()`,
`Pointers()` → compiler error: structs do not implement `orm.Model`.

**Secondary bugs fixed in this plan:**
- `db:"-"` not supported → unsupported-type warning for slice/struct fields
- `TableName()` duplicated when the user already defines it manually
- `time.Time` causes fatal error even when it can simply be skipped

---

## Decisions

| # | Decision | Rationale |
|---|----------|-----------|
| D1 | Split `ParseStruct` (parse only) + `GenerateForFile` (write) on `*Ormc` | SRP: parsing and I/O are independent concerns |
| D2 | `*Ormc.Run()` accumulates all `StructInfo` per file, then calls `GenerateForFile` once | Fixes the overwrite bug at the root |
| D3 | `db:"-"` detected **before** type resolution → silent skip, no warning | Explicit developer intent ≠ type error. Convention: GORM/sqlx/entgo |
| D4 | Fields with `db:"-"` excluded from `Schema()`, `Values()`, AND `Pointers()` | Invariant: all three must list exactly the same fields in the same order |
| D5 | `TableName()` only generated if **not already declared** in the source file | Prevents `duplicate method` compiler error |
| D6 | **Breaking change**: old package-level functions deleted; tests rewritten to `orm.New().<Method>(...)` | No dead code; clean API |
| D7 | Struct with zero mappable fields → **skip entirely**, `o.log(...)` warning | A structurally empty struct cannot implement `orm.Model` usefully |
| D8 | `time.Time` **without** `db:"-"` → `o.log(...)` warning + skip (not fatal, not error) | Vision: minimum tags. Developer can add `db:"-"` to suppress warning |
| D9 | `time.Time` **with** `db:"-"` → silent skip (covered by D3) | Consistent with all other explicitly ignored fields |
| D10 | Remove per-struct `//go:generate ormc -struct X`; use single `//go:generate ormc` at project root | `ormc` does not accept `-struct` flag; per-struct directives are legacy |
| D11 | File with zero mappable infos → skip + `o.log(...)` warning, no `_orm.go` written | An empty output file is valid Go but useless |

---

## Architecture Change

### Before (broken)

```
ormc CLI
  └─ per struct → GenerateCodeForStruct(name, file)
                      └─ parse + write file  ← OVERWRITES previous struct
```

### After (fixed)

```
ormc CLI
  └─ orm.New().Run()             ← no dir param, uses o.rootDir (default ".")
       └─ per file → o.ParseStruct() × N → []StructInfo
                   → o.GenerateForFile([]StructInfo, file)  ← writes ONCE
```

---

## Affected Files

| File | Change |
|------|---------|
| `ormc.go` | All functions become methods on `*Ormc` (from ORMC_HANDLER); fix `Run()` to accumulate + write once; add `db:"-"` / `time.Time` / `TableName()` detection |
| `ormc_handler.go` | Already created in ORMC_HANDLER prerequisite |
| `tests/mock_generator_model.go` | Add `ModelWithIgnored`, `MultiA`, `MultiB`, `BadTimeNoTag`; delete `BadTime` |
| `tests/ormc_test.go` | Update `Bad Time Type` test → now expects skip (no error) |
| `tests/ormc_multi_test.go` | **New file**: multi-struct, `db:"-"`, `TableName` detection tests |

---

## Execution Steps

### Step 1 — Add `(o *Ormc) ParseStruct` to `ormc.go`

**All old package-level functions are deleted** (breaking change, per ORMC_HANDLER.md DE).
Add the `ParseStruct` method on `*Ormc`:

```go
// ParseStruct parses a single struct from a Go file and returns its metadata.
func (o *Ormc) ParseStruct(structName string, goFile string) (StructInfo, error) {
    if structName == "" {
        return StructInfo{}, Err("Please provide a struct name")
    }
    if goFile == "" {
        return StructInfo{}, Err("goFile path cannot be empty")
    }

    fset := token.NewFileSet()
    node, err := parser.ParseFile(fset, goFile, nil, parser.ParseComments)
    if err != nil {
        return StructInfo{}, Err(err, "Failed to parse file")
    }

    // ... (field parsing logic, see Step 2 for updated field loop) ...

    tableName := detectTableName(node, structName) // Step 3
    declared := tableName != ""
    if !declared {
        tableName = Convert(structName + "s").SnakeLow().String()
    }

    info := StructInfo{
        Name:              structName,
        TableName:         tableName,
        PackageName:       node.Name.Name,
        TableNameDeclared: declared,
    }

    // field loop — see Step 2

    return info, nil
}
```

And `GenerateForStruct` on `*Ormc` uses `o.ParseStruct` + `o.GenerateForFile`:

```go
func (o *Ormc) GenerateForStruct(structName string, goFile string) error {
    info, err := o.ParseStruct(structName, goFile)
    if err != nil {
        return err
    }
    if len(info.Fields) == 0 {
        o.log("Warning: struct", structName, "has no mappable fields; skipping output")
        return nil // D14
    }
    return o.GenerateForFile([]StructInfo{info}, goFile)
}
```

Update `StructInfo` to carry the `TableNameDeclared` flag:

```go
type StructInfo struct {
    Name              string
    TableName         string
    PackageName       string
    Fields            []FieldInfo
    TableNameDeclared bool // true = source already declares TableName(); do not generate
}
```

### Step 2 — Update field loop in `(o *Ormc) ParseStruct`: `db:"-"` + unsupported types

The field loop must check `db:"-"` **first**, before resolving the Go type:

```go
for _, field := range targetStruct.Fields.List {
    if len(field.Names) == 0 {
        continue // anonymous/embedded field
    }
    fieldName := field.Names[0].Name
    if !ast.IsExported(fieldName) {
        continue
    }

    // ── 1. Check db tag FIRST ──────────────────────────────────────────────
    dbTag := extractDbTag(field) // helper: returns the db:"..." value or ""
    if dbTag == "-" {
        continue // D3: silent skip, no warning — explicit developer intent
    }

    // ── 2. Resolve Go type ─────────────────────────────────────────────────
    typeStr := resolveTypeStr(field) // helper: returns "string", "int64", "[]byte", etc.

    if typeStr == "time.Time" {
        // D8: warn + skip — developer should use int64 + tinywasm/time
        // D9: if db:"-" was set above, we already continued before reaching here
        log.Printf("Warning: time.Time not allowed for field %s.%s; use int64+tinywasm/time. Skipping.", structName, fieldName)
        continue
    }

    var fieldType FieldType
    switch typeStr {
    case "string":
        fieldType = TypeText
    case "int", "int32", "int64", "uint", "uint32", "uint64":
        fieldType = TypeInt64
    case "float32", "float64":
        fieldType = TypeFloat64
    case "bool":
        fieldType = TypeBool
    case "[]byte":
        fieldType = TypeBlob
    default:
        // Unsupported type without db:"-" → warn + skip (not fatal)
        // Tip: add db:"-" to suppress this warning
        log.Printf("Warning: unsupported type %q for field %s.%s; skipping. Add db:\"-\" to suppress.", typeStr, structName, fieldName)
        continue
    }

    // ── 3. Process constraints ─────────────────────────────────────────────
    // ... (existing constraint logic for pk, unique, not_null, ref, etc.) ...
}
```

> **Invariant (D4):** Only fields that pass the type check above are added to
> `FieldInfo`. `Schema()`, `Values()`, and `Pointers()` all iterate over `info.Fields`
> → they are always in sync.

### Step 3 — Add `detectTableName` helper

```go
// detectTableName scans the AST for func (X) TableName() string on structName.
// Returns the literal return value if found, "" otherwise.
func detectTableName(node *ast.File, structName string) string {
    for _, decl := range node.Decls {
        funcDecl, ok := decl.(*ast.FuncDecl)
        if !ok || funcDecl.Recv == nil || len(funcDecl.Recv.List) == 0 {
            continue
        }
        if funcDecl.Name.Name != "TableName" {
            continue
        }
        recv := funcDecl.Recv.List[0].Type
        recvName := ""
        if ident, ok := recv.(*ast.Ident); ok {
            recvName = ident.Name
        } else if star, ok := recv.(*ast.StarExpr); ok {
            if ident, ok := star.X.(*ast.Ident); ok {
                recvName = ident.Name
            }
        }
        if recvName != structName {
            continue
        }
        if funcDecl.Body != nil && len(funcDecl.Body.List) == 1 {
            if ret, ok := funcDecl.Body.List[0].(*ast.ReturnStmt); ok && len(ret.Results) == 1 {
                if lit, ok := ret.Results[0].(*ast.BasicLit); ok {
                    return strings.Trim(lit.Value, `"`)
                }
            }
        }
    }
    return ""
}
```

`(o *Ormc) GenerateForFile` writes ORM implementations for all infos into one file:

```go
// (o *Ormc) GenerateForFile writes ORM implementations for all infos into one file.
func (o *Ormc) GenerateForFile(infos []StructInfo, sourceFile string) error {
    if len(infos) == 0 {
        return nil
    }
    buf := Convert()

    // File header — written once
    buf.Write("// Code generated by ormc; DO NOT EDIT.\n")
    buf.Write("// NOTE: Schema() and Values() must always be in the same field order.\n")
    buf.Write("// String PK: set via github.com/tinywasm/unixid before calling db.Create().\n")
    buf.Write(Sprintf("package %s\n\n", infos[0].PackageName))
    buf.Write("import (\n\t\"github.com/tinywasm/orm\"\n)\n\n")

    for _, info := range infos {
        // TableName() — only if not already declared in source (D5)
        if !info.TableNameDeclared {
            buf.Write(Sprintf("func (m *%s) TableName() string {\n", info.Name))
            buf.Write(Sprintf("\treturn \"%s\"\n", info.TableName))
            buf.Write("}\n\n")
        }
        // Schema(), Values(), Pointers(), Meta, ReadOne*, ReadAll*
        // ... (existing generation logic) ...
    }

    outName := Convert(sourceFile).TrimSuffix(".go").String() + "_orm.go"
    return os.WriteFile(outName, buf.Bytes(), 0644)
}
```

`Run()` on `*Ormc` uses `o.rootDir` (set by `SetRootDir`, default `"."`).
Inside the file-scanning loop, use `o.ParseStruct` and `o.GenerateForFile`
— and replace all `log.Printf` with `o.log(...)`:

```go
func (o *Ormc) Run() error {
    foundAny := false

    err := filepath.Walk(o.rootDir, func(path string, info os.FileInfo, err error) error {
        // ... dir skip logic unchanged ...

        if fileName == "model.go" || fileName == "models.go" {
            var infos []StructInfo

            for _, decl := range node.Decls {
                if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.TYPE {
                    for _, spec := range genDecl.Specs {
                        if typeSpec, ok := spec.(*ast.TypeSpec); ok {
                            if _, ok := typeSpec.Type.(*ast.StructType); ok {
                                info, err := o.ParseStruct(typeSpec.Name.Name, path)
                                if err != nil {
                                    o.log("Skipping", typeSpec.Name.Name, "in", path+":", err)
                                    continue
                                }
                                if len(info.Fields) == 0 {
                                    o.log("Warning:", typeSpec.Name.Name, "has no mappable fields; skipping")
                                    continue
                                }
                                infos = append(infos, info)
                            }
                        }
                    }
                }
            }

            if len(infos) == 0 {
                o.log("Warning: no mappable structs found in", path+"; no output generated")
                return nil
            }

            if err := o.GenerateForFile(infos, path); err != nil {
                o.log("Failed to write output for", path+":", err)
            } else {
                foundAny = true
            }
        }
        return nil
    })

    if err != nil {
        return Err(err, "error walking directory")
    }
    if !foundAny {
        return Err("no models found")
    }
    return nil
}
```

### Step 6 — Add test fixtures to `tests/mock_generator_model.go`

```go
// ModelWithIgnored tests db:"-" silent exclusion from Schema/Values/Pointers.
type ModelWithIgnored struct {
    ID      string   `db:"pk"`
    Name    string
    Tags    []string `db:"-"` // slice: silently ignored
    Friends []User   `db:"-"` // struct slice: silently ignored
    Score   float64
}

// BadTimeNoTag tests that time.Time WITHOUT db:"-" → warning + skip (not fatal).
type BadTimeNoTag struct {
    ID        string    `db:"pk"`
    Name      string
    CreatedAt time.Time // no db tag → warning + skip
}

// MultiA and MultiB: multi-struct generation in same file.
// Both must appear in the generated _orm.go (D2 fix).
type MultiA struct {
    ID   string `db:"pk"`
    Name string
}
func (MultiA) TableName() string { return "multi_a_records" } // manually declared → D5

type MultiB struct {
    ID    string `db:"pk"`
    Value int64
}
// MultiB has NO TableName() → ormc must generate it
```

### Step 7 — Update `tests/ormc_test.go`: fix `Bad Time Type` test

The existing test expects a fatal error. With D8, `time.Time` is now a warning+skip:

```go
t.Run("Bad Time Type — now a warning, not fatal", func(t *testing.T) {
    // D8: time.Time without db:"-" → warning + skip, not error
    err := orm.GenerateCodeForStruct("BadTimeNoTag", "mock_generator_model.go")
    if err != nil {
        t.Fatalf("Expected no error for time.Time (warn+skip), got: %v", err)
    }

    outFile := "mock_generator_model_orm.go"
    content, err := os.ReadFile(outFile)
    if err != nil {
        t.Fatalf("Failed to read generated file: %v", err)
    }
    defer os.Remove(outFile)

    s := string(content)
    // CreatedAt must be absent (skipped)
    if strings.Contains(s, "CreatedAt") || strings.Contains(s, "created_at") {
        t.Error("time.Time field must be absent from generated output")
    }
    // ID and Name must be present
    if !strings.Contains(s, `"id"`) || !strings.Contains(s, `"name"`) {
        t.Error("Other fields must still be generated")
    }
})
```

> **Note:** Delete the existing `BadTime` struct from `mock_generator_model.go`
> (it only had `time.Time`, so it is now D7-skipped entirely).
> Replace with `BadTimeNoTag` (has other valid fields alongside `time.Time`).

### Step 8 — Add `tests/ormc_multi_test.go`

```go
//go:build !wasm

package tests

import (
    "os"
    "strings"
    "testing"

    "github.com/tinywasm/orm"
)

func TestOrmc_MultiStruct(t *testing.T) {
    t.Run("Both structs appear in a single output file", func(t *testing.T) {
        o := orm.New()
        infoA, err := o.ParseStruct("MultiA", "mock_generator_model.go")
        if err != nil { t.Fatal(err) }

        infoB, err := o.ParseStruct("MultiB", "mock_generator_model.go")
        if err != nil { t.Fatal(err) }

        err = o.GenerateForFile([]orm.StructInfo{infoA, infoB}, "mock_generator_model.go")
        if err != nil { t.Fatal(err) }

        outFile := "mock_generator_model_orm.go"
        content, err := os.ReadFile(outFile)
        if err != nil { t.Fatal(err) }
        defer os.Remove(outFile)

        s := string(content)

        // Both schemas must be present
        if !strings.Contains(s, "func (m *MultiA) Schema()") {
            t.Error("MultiA Schema() not generated")
        }
        if !strings.Contains(s, "func (m *MultiB) Schema()") {
            t.Error("MultiB Schema() not generated")
        }
    })
}

func TestOrmc_TableNameDetection(t *testing.T) {
    t.Run("TableName() NOT generated when already declared (D5)", func(t *testing.T) {
        err := orm.GenerateCodeForStruct("MultiA", "mock_generator_model.go")
        if err != nil { t.Fatal(err) }

        outFile := "mock_generator_model_orm.go"
        content, err := os.ReadFile(outFile)
        if err != nil { t.Fatal(err) }
        defer os.Remove(outFile)

        if strings.Contains(string(content), "func (m *MultiA) TableName()") {
            t.Error("TableName() must NOT be generated — already declared in source")
        }
    })

    t.Run("TableName() IS generated when not declared (D5)", func(t *testing.T) {
        err := orm.GenerateCodeForStruct("MultiB", "mock_generator_model.go")
        if err != nil { t.Fatal(err) }

        outFile := "mock_generator_model_orm.go"
        content, err := os.ReadFile(outFile)
        if err != nil { t.Fatal(err) }
        defer os.Remove(outFile)

        if !strings.Contains(string(content), "func (m *MultiB) TableName()") {
            t.Error("TableName() must be generated — not declared in source")
        }
    })
}

func TestOrmc_DbIgnoreTag(t *testing.T) {
    t.Run("db:\"-\" fields excluded from Schema, Values, Pointers (D3+D4)", func(t *testing.T) {
        err := orm.GenerateCodeForStruct("ModelWithIgnored", "mock_generator_model.go")
        if err != nil { t.Fatal(err) }

        outFile := "mock_generator_model_orm.go"
        content, err := os.ReadFile(outFile)
        if err != nil { t.Fatal(err) }
        defer os.Remove(outFile)

        s := string(content)

        for _, absent := range []string{"Tags", "Friends", "tags", "friends"} {
            if strings.Contains(s, absent) {
                t.Errorf("db:\"-\" field %q must be absent from ALL generated code", absent)
            }
        }
        for _, present := range []string{
            `"id"`, `"name"`, `"score"`,
            "m.ID", "m.Name", "m.Score",
            "&m.ID", "&m.Name", "&m.Score",
        } {
            if !strings.Contains(s, present) {
                t.Errorf("Non-ignored field %q must be present", present)
            }
        }
    })
}
```

### Step 9 — Update `docs/SKILL.md`

Add/update these sections:

```markdown
### `db:"-"` — Explicit field exclusion
Fields tagged `db:"-"` are **silently** excluded from `Schema()`, `Values()`,
and `Pointers()`. Use this for computed fields, relations, or any field whose
type is not supported and whose warning you want to suppress.

### Unsupported field types
Fields with unsupported types (slices of structs, maps, channels, etc.) produce
a **warning log** and are skipped. `time.Time` is treated the same way:
it produces a warning and is skipped — use `int64` + `tinywasm/time` instead.
Add `db:"-"` to suppress the warning.

### `TableName()` auto-detection
`ormc` checks the source file for an existing `TableName() string` method.
If found, it is **not generated** (prevents duplicate method compiler error).
If not found, `ormc` generates it as the snake_case plural of the struct name.

### `//go:generate` — canonical pattern
Do NOT use per-struct `//go:generate ormc -struct Name` directives.
Use a single directive at the project root:
```go
//go:generate ormc
```
`ormc` recursively scans all subdirectories for `model.go` / `models.go` files.
```

### Step 10 — Run tests

```bash
gotest
```

All existing `ormc_test.go` tests must pass (with the updated `Bad Time Type` test).
All new `ormc_multi_test.go` tests must pass.
Coverage target: ≥ 90%.

---

## Acceptance Criteria

| Criterion | Check |
|-----------|-------|
| `ormc` on a file with N structs generates all N in a single `_orm.go` | ✅ |
| `db:"-"` → silent skip from Schema, Values, AND Pointers | ✅ |
| `time.Time` without `db:"-"` → warning + skip (not fatal) | ✅ |
| `time.Time` with `db:"-"` → silent skip | ✅ |
| `TableName()` not generated when already in source file | ✅ |
| `TableName()` generated when absent from source file | ✅ |
| Struct with zero mappable fields → skipped entirely (no output) | ✅ |
| File with zero mappable structs → no `_orm.go` written | ✅ |
| All existing `ormc_test.go` tests pass | ✅ |
| New `ormc_multi_test.go` tests pass | ✅ |
| `gotest` ≥ 90% coverage | ✅ |

---

## Final Decisions (resolved)

| # | Decision |
|---|----------|
| D12 | `BadTime` struct **deleted** from `mock_generator_model.go`; replaced by `BadTimeNoTag` |
| D13 | Post-merge instructions for Jules in `tinywasm/user/docs/REPLY_JULES.md` (separate file, sent via web UI) |
| D14 | `GenerateCodeForStruct` with zero mappable fields → `return nil` (warning already logged per D8/D7) |


