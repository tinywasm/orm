# PLAN: ormc — restricciones, compactación y descriptores opcionales

## Estado

- [x] Fix base: `json:"name"` se usa como `ColumnName` cuando está presente
- [x] Stage 1 — Restringir `json:"name"` a structs `FormOnly` en `ParseStruct`
- [x] Stage 2 — Tests de restricción en `ormc_test.go`
- [x] Stage 3 — Tests de FormOnly permitido en `ormc_test.go`
- [x] Stage 4 — Compactar `Pointers()` a una sola línea en `ormc_generate.go`
- [x] Stage 5 — Flag `-fields` para descriptores opcionales
- [x] Stage 6 — Documentación actualizada en `ARQUITECTURE.md`

---

## Problema original (resuelto)

`ormc.go` ignoraba el tag `json:` al derivar `ColumnName`, usando siempre `SnakeLow()`.
Si el struct tenía `json:"protocolVersion"`, el campo quedaba como `"protocol_version"` → incompatible con protocolos externos que usan camelCase.

## Problema nuevo (pendiente)

El fix anterior se aplicó sin restricción de contexto. Para structs que mapean a DB (no `FormOnly`), permitir `json:"customName"` como override del `ColumnName` es un error silencioso: el nombre de columna SQL resultante puede ser camelCase, incompatible con convenciones y drivers.

**Escenario de bug:**
```go
// struct DB — NO formonly
type User struct {
    FirstName string `json:"firstName"` // → colName = "firstName" (camelCase en SQL = bug)
}
// genera: CREATE TABLE user (firstName TEXT) ← inválido
```

La regla correcta es:
- **`FormOnly`** (solo JSON/form, sin DB): `json:"name"` puede usarse para personalizar el nombre del campo.
- **DB model** (form+json+sql): `json:"name"` **no debe** cambiar el `ColumnName`. El nombre siempre se deriva de `SnakeLow(fieldName)`. Si alguien lo coloca, `ormc` debe retornar error explícito.

---

## Fix pendiente

### Stage 1. `ormc.go` — Condicionar el override a `FormOnly`

Ubicación: `ParseStruct`, alrededor de línea 260.

**Antes (actual):**
```go
colName := fmt.Convert(fieldName).SnakeLow().String()
if jsonTag != "" {
    name := fmt.Convert(jsonTag).Split(",")[0]
    if name != "" && name != "-" {
        colName = name
    }
}
```

**Después:**
```go
colName := fmt.Convert(fieldName).SnakeLow().String()
if jsonTag != "" {
    name := fmt.Convert(jsonTag).Split(",")[0]
    if name != "" && name != "-" {
        if !formOnly {
            return StructInfo{}, fmt.Err(
                "field", fieldName,
                "json name tag has no effect on DB structs: column name is always derived from the field name; remove the json name or declare the struct as ormc:formonly",
            )
        }
        colName = name
    }
}
```

### Stage 2. `ormc_test.go` — Test: DB struct rechaza json name

```go
func TestParseStructRejectsJsonNameOnDBModel(t *testing.T) {
    src := `package test

type User struct {
    FirstName string ` + "`" + `json:"firstName"` + "`" + `
}
`
    tmpDir := t.TempDir()
    path := filepath.Join(tmpDir, "model.go")
    os.WriteFile(path, []byte(src), 0644)

    o := orm.NewOrmc()
    _, err := o.ParseStruct("User", path)
    if err == nil {
        t.Fatal("expected error for json name override on DB struct, got nil")
    }
}
```

### Stage 3. `ormc_test.go` — Test: FormOnly permite json name

```go
func TestParseStructFormOnlyAllowsJsonName(t *testing.T) {
    src := `package test

// ormc:formonly
type ContactForm struct {
    FirstName string ` + "`" + `json:"firstName"` + "`" + `
}
`
    tmpDir := t.TempDir()
    path := filepath.Join(tmpDir, "model.go")
    os.WriteFile(path, []byte(src), 0644)

    o := orm.NewOrmc()
    info, err := o.ParseStruct("ContactForm", path)
    if err != nil {
        t.Fatalf("unexpected error for formonly struct: %v", err)
    }
    if info.Fields[0].ColumnName != "firstName" {
        t.Errorf("expected ColumnName %q, got %q", "firstName", info.Fields[0].ColumnName)
    }
}
```

### Stage 4. `ormc_generate.go` — Compactar `Pointers()` en una sola línea

Actualmente `GenerateForFile` genera `Pointers()` como función multilínea:

```go
func (m *User) Pointers() []any {
    return []any{
        &m.ID,
        &m.Name,
        &m.Email,
    }
}
```

Debe generarse en una sola línea, igual que `Schema()`:

```go
func (m *User) Pointers() []any { return []any{&m.ID, &m.Name, &m.Email} }
```

Ubicación: `ormc_generate.go` líneas 103-109. Reemplazar el bloque multilínea por:

```go
buf.Write(fmt.Sprintf("func (m *%s) Pointers() []any { return []any{", info.Name))
for i, f := range info.Fields {
    if i > 0 {
        buf.Write(", ")
    }
    buf.Write(fmt.Sprintf("&m.%s", f.Name))
}
buf.Write("} }\n\n")
```

### Stage 5. `ormc_handler.go` + `ormc_generate.go` + `cmd/ormc/main.go` — Flag `-fields` para descriptores opcionales

Los descriptores anónimos (`User_`, `EmployeeServiceConfig_`, etc.) se generan siempre en structs DB. Son útiles para construir queries type-safe (`User_.Name`), pero aumentan el binario final. Deben ser **opt-in**.

**Bandera:** `-fields`

- Por defecto: `false` (no se generan)
- Con `-fields`: se generan como hoy

#### `ormc_handler.go` — añadir campo

```go
type Ormc struct {
    logFn      func(messages ...any)
    rootDir    string
    withFields bool  // genera descriptores de campos tipo User_
}

func (o *Ormc) SetFields(v bool) { o.withFields = v }
```

#### `ormc_generate.go` — condicionar generación

```go
if !info.FormOnly && o.withFields {
    // Field Descriptors
    buf.Write(fmt.Sprintf("var %s_ = struct {\n", info.Name))
    ...
}
```

#### `cmd/ormc/main.go` — parsear flag

```go
import "flag"

func main() {
    fields := flag.Bool("fields", false, "generate field descriptor variables (e.g. User_.Name)")
    flag.Parse()

    o := orm.NewOrmc()
    o.SetFields(*fields)
    o.SetLog(func(messages ...any) { fmt.Fprintln(os.Stderr, messages...) })
    if err := o.Run(); err != nil {
        log.Fatalf("ormc: %v", err)
    }
}
```

#### `ormc_test.go` — Tests

**`TestGenerateWithoutFields`** — por defecto no debe generarse el descriptor:
```go
func TestGenerateWithoutFields(t *testing.T) {
    src := `package test

type User struct {
    ID   int    ` + "`" + `db:"pk"` + "`" + `
    Name string
}
`
    tmpDir := t.TempDir()
    os.WriteFile(filepath.Join(tmpDir, "model.go"), []byte(src), 0644)

    o := orm.NewOrmc()
    o.SetRootDir(tmpDir)
    // withFields = false por defecto
    if err := o.Run(); err != nil {
        t.Fatal(err)
    }
    output, _ := os.ReadFile(filepath.Join(tmpDir, "model_orm.go"))
    if strings.Contains(string(output), "var User_ =") {
        t.Error("field descriptor must not be generated without -fields flag")
    }
}
```

**`TestGenerateWithFields`** — con flag activo debe generarse con todos los campos:
```go
func TestGenerateWithFields(t *testing.T) {
    src := `package test

type User struct {
    ID   int    ` + "`" + `db:"pk"` + "`" + `
    Name string
}
`
    tmpDir := t.TempDir()
    os.WriteFile(filepath.Join(tmpDir, "model.go"), []byte(src), 0644)

    o := orm.NewOrmc()
    o.SetRootDir(tmpDir)
    o.SetFields(true)
    if err := o.Run(); err != nil {
        t.Fatal(err)
    }
    output, _ := os.ReadFile(filepath.Join(tmpDir, "model_orm.go"))
    out := string(output)
    if !strings.Contains(out, "var User_ =") {
        t.Error("field descriptor must be generated with -fields flag")
    }
    if !strings.Contains(out, `Name: "name"`) {
        t.Error("field descriptor must include field Name")
    }
}
```

**`TestGenerateFormOnlyNeverFields`** — FormOnly nunca genera descriptor aunque `-fields` esté activo:
```go
func TestGenerateFormOnlyNeverFields(t *testing.T) {
    src := `package test

// ormc:formonly
type ContactForm struct {
    Name string
}
`
    tmpDir := t.TempDir()
    os.WriteFile(filepath.Join(tmpDir, "model.go"), []byte(src), 0644)

    o := orm.NewOrmc()
    o.SetRootDir(tmpDir)
    o.SetFields(true)
    if err := o.Run(); err != nil {
        t.Fatal(err)
    }
    output, _ := os.ReadFile(filepath.Join(tmpDir, "model_orm.go"))
    if strings.Contains(string(output), "var ContactForm_ =") {
        t.Error("field descriptor must never be generated for formonly structs")
    }
}
```

---

## Impacto esperado

| Caso | Antes | Después |
|------|-------|---------|
| DB struct + `json:"name"` | acepta, colName = "name" (bug silencioso) | error explícito en ormc |
| DB struct sin json name | `SnakeLow(field)` | sin cambio |
| `FormOnly` + `json:"name"` | colName = "name" | sin cambio |
| `FormOnly` sin json name | `SnakeLow(field)` | sin cambio |
| DB struct, sin `-fields` | genera `User_` siempre | no genera `User_` |
| DB struct, con `-fields` | genera `User_` siempre | genera `User_` |
| `Pointers()` generado | multilínea | una sola línea |
