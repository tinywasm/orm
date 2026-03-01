# Coverage Plan: Missing `ormc` Test Cases

## Development Rules
- **Standard Library Only in Tests:** NEVER use external assertion libraries. Use only `testing`, `strings`, `os`.
- **Testing Runner (`gotest`):** Install first, then run with `gotest` (no arguments). Prerequisite:
  ```bash
  go install github.com/tinywasm/devflow/cmd/gotest@latest
  ```
- **No new files:** All changes go into existing files (`mock_generator_model.go` and `ormc_test.go`).
- **Black-Box via `GenerateCodeForStruct()`:** Test `ormc` logic through the public function, never via CLI.

## Goal

Reach **coverage ≥ 90%** in `tinywasm/orm` by adding the missing `ormc_test.go` cases identified during the
post-execution review of `CHECK_PLAN.md`. Current coverage: **87.7%**. No logic changes — only test additions.

## Gap Summary

| Missing Case | File to Edit | Action |
|---|---|---|
| `int32` → `TypeInt64` | `mock_generator_model.go` + `ormc_test.go` | Add struct + assertion |
| `uint64` → `TypeInt64` | `mock_generator_model.go` + `ormc_test.go` | Add struct + assertion |
| `float32` → `TypeFloat64` | `mock_generator_model.go` + `ormc_test.go` | Add struct + assertion |
| Bitmask: `ConstraintPK \| ConstraintNotNull = 5` | `ormc_test.go` | Assert numeric value |
| `db:"ref=table"` (no column) → `RefColumn = ""` | `mock_generator_model.go` + `ormc_test.go` | Add struct + assertion |

## Files to Modify

### 1. `tests/mock_generator_model.go`

Add the following structs at the bottom of the file:

```go
// NumericTypes covers int32, uint64, float32 mapping and bitmask constraints.
type NumericTypes struct {
    IDNumeric int32   `db:"pk,not_null"` // PK + NotNull → bitmask 5
    CountUint uint64
    RatioF32  float32
}

// RefNoColumn covers db:"ref=table" without a specific column (RefColumn must be "").
type RefNoColumn struct {
    IDRef    string `db:"pk"`
    ParentID int64  `db:"ref=parents"`
}
```

### 2. `tests/ormc_test.go`

Append the following sub-tests inside `TestOrmc`:

```go
t.Run("Numeric Type Mapping and Bitmask", func(t *testing.T) {
    err := orm.GenerateCodeForStruct("NumericTypes", "mock_generator_model.go")
    if err != nil {
        t.Fatalf("Failed to generate code for NumericTypes: %v", err)
    }

    outFile := "mock_generator_model_orm.go"
    contentBytes, err := os.ReadFile(outFile)
    if err != nil {
        t.Fatalf("Failed to read generated file: %v", err)
    }
    defer os.Remove(outFile)

    content := string(contentBytes)

    expectedStrings := []string{
        // int32 → TypeInt64
        `{Name: "id_numeric", Type: orm.TypeInt64, Constraints: orm.ConstraintPK | orm.ConstraintNotNull}`,
        // uint64 → TypeInt64
        `{Name: "count_uint", Type: orm.TypeInt64, Constraints: orm.ConstraintNone}`,
        // float32 → TypeFloat64
        `{Name: "ratio_f32", Type: orm.TypeFloat64, Constraints: orm.ConstraintNone}`,
    }

    for _, expected := range expectedStrings {
        if !strings.Contains(content, expected) {
            t.Errorf("Generated file missing expected string: %s", expected)
        }
    }

    // Bitmask correctness: ConstraintPK | ConstraintNotNull = 1 | 4 = 5
    pkConstraint := orm.ConstraintPK
    notNullConstraint := orm.ConstraintNotNull
    combined := pkConstraint | notNullConstraint
    if combined != 5 {
        t.Errorf("Expected ConstraintPK | ConstraintNotNull = 5, got %d", combined)
    }
})

t.Run("Ref Without Column", func(t *testing.T) {
    err := orm.GenerateCodeForStruct("RefNoColumn", "mock_generator_model.go")
    if err != nil {
        t.Fatalf("Failed to generate code for RefNoColumn: %v", err)
    }

    outFile := "mock_generator_model_orm.go"
    contentBytes, err := os.ReadFile(outFile)
    if err != nil {
        t.Fatalf("Failed to read generated file: %v", err)
    }
    defer os.Remove(outFile)

    content := string(contentBytes)

    // Ref present, RefColumn must be absent (empty string omitted from generated code)
    if !strings.Contains(content, `Ref: "parents"`) {
        t.Errorf("Expected Ref=parents in generated file")
    }
    if strings.Contains(content, "RefColumn") {
        t.Errorf("RefColumn must be absent when ref tag has no column")
    }
})
```

## Verification

### Run: `gotest` inside `tinywasm/orm`

```bash
cd /path/to/tinywasm/orm
gotest
```

Expected output:
```
✅ vet ok, ✅ race detection ok, ✅ tests stdlib ok, ✅ tests wasm ok, ✅ coverage: ≥90.0%
```

### Checklist

- [ ] `NumericTypes` struct added to `mock_generator_model.go`
- [ ] `RefNoColumn` struct added to `mock_generator_model.go`
- [ ] `"Numeric Type Mapping and Bitmask"` sub-test added to `TestOrmc`
- [ ] `"Ref Without Column"` sub-test added to `TestOrmc`
- [ ] `gotest` passes with coverage ≥ 90%
- [ ] No new files created — changes confined to existing test files
