# PLAN: Missing corrections for FieldRaw support

## Context

The `json:",raw"` support was implemented in `ormc.go` and `ormc_generate.go` and works correctly. Three corrections are missing.

---

## 1. Bug: leading comma in rewritten tags without explicit name

### Problem

In `ormc_tags.go` lines 151-157, when a DB field (not formOnly) has `omitempty` or `raw` but no explicit json name, the rewritten tag has a leading comma:

```go
// current code — bug
tagVal := ""
if hasOmit {
    tagVal += ",omitempty"   // ← becomes ",omitempty" (leading comma)
}
if hasRaw {
    tagVal += ",raw"         // ← becomes ",raw" or ",omitempty,raw"
}
```

The test `ormc_tags_test.go` line 58 even accepts this as correct:
```go
if !strings.Contains(outStr, `json:",omitempty"`) { // ← validates the bug
```

The leading comma is invalid syntax — the empty name before the comma makes no semantic sense.

### Fix in `ormc_tags.go`

The `omitempty` and `raw` options **only make sense with a name**. If there's no name in the tag (`json:",omitempty"`), the entire tag should be removed — the ORM uses `SnakeLow()` of the field name as the default JSON name, no need to declare it:

```go
} else {
    // DB field without explicit json name: only preserve if there's a name
    // omitempty/raw without name add nothing — the ORM derives the name from the field
    // In this context, the json tag is completely discarded
}
```

### Fix in `ormc_tags_test.go`

Correct the test that validates the leading comma — it should verify that the tag is removed, not that it remains with comma:

```go
// BEFORE (validates the bug)
if !strings.Contains(outStr, `json:",omitempty"`) {
    t.Errorf(...)
}

// AFTER (correct)
if strings.Contains(outStr, `json:",`) {
    t.Errorf("json tag with leading comma should not exist, got: %s", outStr)
}
```

---

## 2. Bug: redundancy of tags in tests — `json:"name,raw"` when `raw` already implies the behavior

### Problem

In the tests (and in the previous plan) tags like this are used:
```go
Result string `json:"result,raw"`
Error  string `json:"error,omitempty,raw"`
```

But if the field already has an explicit name (`"result"`) and you just want to indicate it's raw, always adding the name is redundant when the name matches `SnakeLow()` of the Go field. The tag `json:"result,raw"` should only be used when:

1. The JSON name differs from the snake_case of the Go field (`ProtocolVersion` → `"protocolVersion,raw"`), **or**
2. `raw` is needed along with `omitempty`

When the json name matches what the ORM would generate by default, just use the minimal necessary tag. Tests/documentation should reflect this — don't add name in the tag if not necessary.

### Fix in `tests/models.go` and `tests/ormc_test.go`

```go
// REDUNDANT — "result" == SnakeLow("Result"), the name adds nothing
Result string `json:"result,raw"`

// CORRECT — only raw, no name or comma
Result string `json:"raw"`

// NECESSARY — name different from default snake_case
ProtocolVersion string `json:"protocolVersion,raw"`

// NECESSARY — raw + omitempty
Error string `json:"omitempty,raw"`
```

---

## 3. Regression test: fields without `raw` remain `FieldText`

Add struct in `tests/models.go`:

```go
// ormc:formonly
type PlainResponse struct {
    Message string `json:"message"`
    Code    string `json:"omitempty"`
}
```

Test in `tests/ormc_test.go`:

```go
t.Run("fields without raw stay FieldText", func(t *testing.T) {
    err := orm.NewOrmc().GenerateForStruct("PlainResponse", "models.go")
    // ...
    mustHave := []string{
        `{Name: "message", Type: fmt.FieldText`,
        `{Name: "code", Type: fmt.FieldText, OmitEmpty: true`,
    }
    mustNotHave := []string{
        `fmt.FieldRaw`,
    }
})
```

---

## 4. Update `README.md`

Add the `raw` option with examples showing when the name is necessary and when not:

| Tag | When to use it |
|---|---|
| `json:"raw"` | Only raw, default name (snake_case of the field) |
| `json:"omitempty,raw"` | Raw + omit if empty, default name |
| `json:"camelName,raw"` | Name different from snake_case + raw |
| `json:"camelName,omitempty,raw"` | Different name + omitempty + raw |
