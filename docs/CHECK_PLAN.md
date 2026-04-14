# PLAN: Generate `FielderSlice` for every DB-backed struct

## Problem

`ormc` generates `ReadAll{Name}` returning `([]*{Name}, error)` — a plain Go slice.
This type does not implement `fmt.FielderSlice`, so callers cannot pass list results
to `json.EncodeArray` without writing boilerplate by hand.

`fmt.FielderSlice` (defined in `tinywasm/fmt`) is the interface required by
`json.EncodeArray` / `json.DecodeArray` (added in tinywasm/json v0.5.0):

```go
type FielderSlice interface {
    Len() int
    At(i int) Fielder
    Append() Fielder
}
```

## Breaking Change

`ReadAll{Name}` return type changes from `([]*{Name}, error)` to `(*{Name}List, error)`.

`{Name}List` is a named type `type {Name}List []*{Name}` that supports all the same
range loops and indexing as `[]*{Name}`. Callers only break if they assign the result
to an explicitly typed `[]*{Name}` variable — those sites need a one-line update.

**formonly structs are excluded** — they have no `ReadAll*` and are never used as
list results in the ORM layer.

---

## Changes to `ormc_generate.go`

### 1. Generate `{Name}List` type after `Pointers()`

Insert immediately after the `Pointers()` block and before `Validate()`.
`Schema()` and `Pointers()` return `nil` — required because `fmt.FielderSlice`
now embeds `fmt.Fielder` (see fmt/docs/PLAN.md). They are never called for list
encoding because `json.Encode` type-asserts to `FielderSlice` first.

```go
buf.Write(fmt.Sprintf("type %sList []*%s\n\n", info.Name, info.Name))
buf.Write(fmt.Sprintf("func (s *%sList) Schema() []fmt.Field { return nil }\n", info.Name))
buf.Write(fmt.Sprintf("func (s *%sList) Pointers() []any     { return nil }\n", info.Name))
buf.Write(fmt.Sprintf("func (s *%sList) Len() int             { return len(*s) }\n", info.Name))
buf.Write(fmt.Sprintf("func (s *%sList) At(i int) fmt.Fielder { return (*s)[i] }\n", info.Name))
buf.Write(fmt.Sprintf("func (s *%sList) Append() fmt.Fielder  { v := &%s{}; *s = append(*s, v); return v }\n\n", info.Name, info.Name))
```

Condition: `!info.FormOnly` (same guard as `ModelName`, `ReadOne*`, `ReadAll*`).

### 2. Update `ReadAll{Name}` to return `*{Name}List`

Replace the current generated body:

```go
// BEFORE
func ReadAll{Name}(qb *orm.QB) ([]*{Name}, error) {
    var results []*{Name}
    err := qb.ReadAll(
        func() fmt.Model { return &{Name}{} },
        func(m fmt.Model) { results = append(results, m.(*{Name})) },
    )
    return results, err
}

// AFTER
func ReadAll{Name}(qb *orm.QB) (*{Name}List, error) {
    var results {Name}List
    err := qb.ReadAll(
        func() fmt.Model { return &{Name}{} },
        func(m fmt.Model) { results = append(results, m.(*{Name})) },
    )
    return &results, err
}
```

In `ormc_generate.go` the relevant section is around line 149. Replace:

```go
buf.Write(fmt.Sprintf("func ReadAll%s(qb *orm.QB) ([]*%s, error) {\n", info.Name, info.Name))
buf.Write(fmt.Sprintf("\tvar results []*%s\n", info.Name))
```

With:

```go
buf.Write(fmt.Sprintf("func ReadAll%s(qb *orm.QB) (*%sList, error) {\n", info.Name, info.Name))
buf.Write(fmt.Sprintf("\tvar results %sList\n", info.Name))
```

And replace the return:

```go
// BEFORE
buf.Write("\treturn results, err\n")

// AFTER
buf.Write("\treturn &results, err\n")
```

### 3. Relation helper — same update

`ReadAll{Child}By{Parent}` delegates to `ReadAll{Child}`, so its return type updates
automatically. No template change needed.

---

## Migration impact on callers

Every module that calls `ReadAll*` and assigns to `[]*{Name}` must update.
The typical change is trivial:

```go
// BEFORE
rows, err := ReadAllReservation(qb)   // rows: []*Reservation
for _, r := range rows { ... }

// AFTER
rows, err := ReadAllReservation(qb)   // rows: *ReservationList
for _, r := range *rows { ... }       // dereference once, or use rows.Len()/rows.At(i)
```

Alternatively, assign to the concrete type:

```go
var rows *ReservationList
rows, err = ReadAllReservation(qb)
```

---

## Tests to update

`tests/ormc_test.go` and `tests/ormc_multi_test.go` assert the generated output as
strings. Update expected strings to reflect:
- New `type {Name}List` block
- Updated `ReadAll*` signature and body

---

## Checklist

- [ ] Update `ormc_generate.go`: generate `{Name}List` type after `Pointers()`
- [ ] Update `ormc_generate.go`: change `ReadAll{Name}` return type and body
- [ ] Run `ormc` in all dependent modules to regenerate `*_orm.go` files
- [ ] Update `tests/ormc_test.go` and `tests/ormc_multi_test.go` expected output
- [ ] `go test ./...` passes
- [ ] Bump minor version (breaking change for callers of `ReadAll*`)
