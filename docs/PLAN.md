> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.

# PLAN: Fix ReadAll* Return Type (value instead of pointer)

## Context

`ormc` currently generates `ReadAll<Name>` with this signature:

```go
func ReadAllSession(qb *orm.QB) (*SessionList, error) {
    var results SessionList
    err := qb.ReadAll(...)
    return &results, err   // returns pointer to slice
}
```

Returning `*SessionList` (pointer to slice) is non-idiomatic Go and causes two bugs in callers:

```go
sessions, err := ReadAllSession(qb)
for _, s := range sessions { ... }  // compile error: cannot range over *SessionList
if len(sessions) == 0 { ... }       // compile error: invalid arg for len
```

The correct return type is `SessionList` (value — a slice), which is how Go functions normally
return collections. This is the only pending change in `ormc` related to the `ReadAll` pattern.

> **Note on typed field accessors**: `// orm:typed_fields` is already fully implemented.
> See §4.2.1 in `docs/ARQUITECTURE.md`. The fix for `tinywasm/user` is to add
> `// orm:typed_fields` to the structs that use `Session_`, `Role_`, etc. in query conditions.
> That is tracked in `tinywasm/user/docs/PLAN.md`.

---

## Change

### `ormc/generate.go` — two lines

```go
// Before:
buf.Write(fmt.Sprintf("func ReadAll%s(qb *orm.QB) (*%sList, error) {\n", info.Name, info.Name))
// ...
buf.Write("\treturn &results, err\n")

// After:
buf.Write(fmt.Sprintf("func ReadAll%s(qb *orm.QB) (%sList, error) {\n", info.Name, info.Name))
// ...
buf.Write("\treturn results, err\n")
```

Remove `*` from the return type and `&` from the return statement.

---

## Impact on callers

All callers of any `ReadAll*` function across the ecosystem return a slice value, not a pointer.
The callers change as follows:

```go
// Before — broken (pointer):
results, err := ReadAllUser(qb)
if len(results) == 0 { ... }        // error: invalid arg
for _, u := range results { ... }   // error: cannot range

// After — correct (value):
results, err := ReadAllUser(qb)
if len(results) == 0 { ... }        // OK
for _, u := range results { ... }   // OK
```

Any caller that explicitly took `&results` or tested `results == nil` must also be adjusted.
In practice: iterate all `ReadAll*` call sites across dependent packages and fix.

---

## Known affected packages (must update after gopush)

| Package | Files to update |
|---|---|
| `tinywasm/user` | `crud.go`, `cache.go`, `sessions.go`, `identities.go`, `lan_ips.go` |
| any other package using `ReadAll*` | search with `grep -rn 'ReadAll' --include='*.go'` |

---

## Tests to update

In `orm/tests/ormc_test.go` and `orm/ormc/ormc_multi_test.go`, any assertion that checks for
`*<Name>List` in the generated output must be changed to `<Name>List` (no pointer):

```go
// Before:
assertContains(t, out, "func ReadAllUser(qb *orm.QB) (*UserList, error)")

// After:
assertContains(t, out, "func ReadAllUser(qb *orm.QB) (UserList, error)")
```

---

## Stages

| Stage | File | Action |
|---|---|---|
| S1 | `orm/ormc/generate.go` lines 313, 319 | Remove `*` from return type; remove `&` from return |
| S2 | `orm/tests/ormc_test.go`, `orm/ormc/ormc_multi_test.go` | Update assertions for new signature |
| S3 | `orm/tests/` | Run `go test ./...` — must pass |
| S4 | publish | `gopush` — updates all dependents |
| S5 | `tinywasm/user` | Re-run `ormc`; fix `ReadAll*` callers (see `user/docs/PLAN.md`) |

## Code Quality Checklist

- No standard library. Use `github.com/tinywasm/fmt` for all string ops.
- No logic duplication — the change is two characters in `generate.go`.
- All existing tests must pass after the change (only signature assertions need updating).
- Do not change `ReadOne<Name>` — it returns `*Name` (single model pointer), which is correct.
