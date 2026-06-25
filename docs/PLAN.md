# Plan: `ormc` debe honrar `omitempty` en `EncodeFields` (fix raíz del JSON-RPC de MCP)

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.
> **Plan autónomo y autocontenido.** Todo el trabajo es dentro de `github.com/tinywasm/orm`. Reglas
> permanentes del repo en `AGENTS.md` (auto-cargado). Coordinación general del monorepo (no necesaria
> para ejecutar este plan): `tinywasm/docs/REGRESSION_FIX_MASTER_PLAN.md`.

## Contexto (causa raíz)

El servidor MCP emite respuestas JSON-RPC con `error:null` **junto a** `result`, lo que el cliente
(IDE) rechaza con error de unión Zod. El origen está en el **generador `ormc` de este repo**, no en
`mcp`: el `EncodeFields` generado escribe **todos los campos siempre**, ignorando `omitempty`.

Ejemplo del código generado hoy (`mcp/model_orm.go`), donde `Result`/`Error` son `omitempty`:

```go
func (m *JSONRPCResponseStruct) EncodeFields(w fmt.FieldWriter) {
	w.String("jsonrpc", m.JSONRPC)
	w.String("id", m.ID)
	w.Raw("result", m.Result)
	w.Raw("error", m.Error)   // ← debería omitirse cuando m.Error está vacío
}
```

El modelo declara la intención con un tag custom:

```go
// mcp/model.go
type JSONRPCResponseStruct struct {
	JSONRPC string
	ID      string
	Result  fmt.RawJSON `omitempty:"true"`
	Error   fmt.RawJSON `omitempty:"true"`
}
```

Hay **dos huecos** en `ormc`:

1. **No se parsea el tag `omitempty:"true"`.** La extracción de tags en `ormc/generator.go` solo
   reconoce `db:"…"`, `json:"…"`, `input:"…"`. El tag custom queda ignorado → `FieldInfo.OmitEmpty`
   se queda `false`. (Sí se computa `OmitEmpty` desde `json:",omitempty"`, pero los modelos MCP usan el
   tag custom — decisión del usuario: **honrar `omitempty:"true"`**.)
2. **`EncodeFields` ignora `OmitEmpty`.** Aunque el flag estuviera `true`, el generador emite el campo
   incondicionalmente.

## Objetivo

Que `ormc`:
- parsee el tag `omitempty:"true"` y marque `FieldInfo.OmitEmpty = true`;
- genere en `EncodeFields` una **guarda de zero-value** para los campos `OmitEmpty`, de modo que un
  valor vacío **no se emita** (ni como `null` ni como cadena vacía).

## Restricciones

- Ver reglas permanentes del repo en `AGENTS.md` (no stdlib en código agnóstico, `gotest`, etc.).
- Usar `tinywasm/fmt` (no `strings`/`strconv` stdlib) en el generador, siguiendo el estilo existente.
- **No** cambiar la semántica de los campos sin `omitempty`: siguen emitiéndose siempre.
- `DecodeFields` no cambia (leer un campo ausente ya es no-op por el `if v, ok := …`).
- Mantener determinismo del output (orden de campos intacto).

## Paso 1 — Parsear el tag `omitempty:"true"` (`ormc/generator.go`)

En el bloque de extracción de tags (hoy ~líneas 232–247), además de `db:`/`json:`/`input:`, reconocer
el tag custom `omitempty`:

```go
dbTag := ""
jsonTag := ""
inputTag := ""
omitEmptyTag := false
if field.Tag != nil {
	tagVal := fmt.Convert(field.Tag.Value).TrimPrefix("`").TrimSuffix("`").String()
	parts := fmt.Convert(tagVal).Split(" ")
	for _, p := range parts {
		switch {
		case fmt.HasPrefix(p, `db:"`):
			dbTag = fmt.Convert(p).TrimPrefix(`db:"`).TrimSuffix(`"`).String()
		case fmt.HasPrefix(p, `json:"`):
			jsonTag = fmt.Convert(p).TrimPrefix(`json:"`).TrimSuffix(`"`).String()
		case fmt.HasPrefix(p, `input:"`):
			inputTag = fmt.Convert(p).TrimPrefix(`input:"`).TrimSuffix(`"`).String()
		case fmt.HasPrefix(p, `omitempty:"`):
			v := fmt.Convert(p).TrimPrefix(`omitempty:"`).TrimSuffix(`"`).String()
			omitEmptyTag = (v == "true")
		}
	}
}
```

Y donde hoy se computa `omitEmpty` (hoy ~líneas 398–409, desde `json:",omitempty"`), combinar ambas
fuentes:

```go
omitEmpty := omitEmptyTag   // tag custom omitempty:"true"
if jsonTag != "" {
	parts := fmt.Convert(jsonTag).Split(",")
	for _, p := range parts {
		if p == "omitempty" {
			omitEmpty = true
		}
		if p == "raw" {
			fieldType = fmt.FieldRaw
		}
	}
}
```

`FieldInfo.OmitEmpty` ya existe y ya se propaga al schema literal (`generate.go` escribe
`OmitEmpty: true`) y a `parse_generated.go`; no hay que tocar esa parte.

## Paso 2 — Guardas de zero-value en `EncodeFields` (`ormc/generate.go`)

En la generación de `EncodeFields` (hoy ~líneas 123–157), cuando `f.OmitEmpty` sea `true`, envolver la
escritura del campo en una guarda según el tipo. Estructura sugerida (preservando el output actual
cuando `!f.OmitEmpty`):

```go
for _, f := range info.Fields {
	// Construir la línea de escritura como hoy:
	var line string
	switch f.Type {
	case fmt.FieldText:
		line = fmt.Sprintf("w.String(\"%s\", m.%s)", f.ColumnName, f.Name)
	case fmt.FieldRaw:
		line = fmt.Sprintf("w.Raw(\"%s\", m.%s)", f.ColumnName, f.Name)
	case fmt.FieldInt:
		line = fmt.Sprintf("w.Int(\"%s\", int64(m.%s))", f.ColumnName, f.Name)
	case fmt.FieldFloat:
		line = fmt.Sprintf("w.Float(\"%s\", float64(m.%s))", f.ColumnName, f.Name)
	case fmt.FieldBool:
		line = fmt.Sprintf("w.Bool(\"%s\", m.%s)", f.ColumnName, f.Name)
	case fmt.FieldBlob:
		line = fmt.Sprintf("w.Bytes(\"%s\", m.%s)", f.ColumnName, f.Name)
	// FieldStruct / *Slice: ver nota abajo
	}

	if f.OmitEmpty && line != "" {
		guard := omitEmptyGuard(f) // expresión booleana "campo NO vacío"
		if guard != "" {
			buf.Write(fmt.Sprintf("\tif %s { %s }\n", guard, line))
			continue
		}
	}
	if line != "" {
		buf.Write("\t" + line + "\n")
		continue
	}
	// Slices/struct: dejar la generación actual (no guardar, o guardar por nil/len — ver nota)
}
```

Helper de guarda por tipo:

```go
func omitEmptyGuard(f FieldInfo) string {
	switch f.Type {
	case fmt.FieldText:
		return fmt.Sprintf("m.%s != \"\"", f.Name)
	case fmt.FieldRaw, fmt.FieldBlob:
		return fmt.Sprintf("len(m.%s) != 0", f.Name)
	case fmt.FieldInt:
		return fmt.Sprintf("m.%s != 0", f.Name)
	case fmt.FieldFloat:
		return fmt.Sprintf("m.%s != 0", f.Name)
	case fmt.FieldBool:
		return fmt.Sprintf("m.%s", f.Name)
	}
	return "" // tipos compuestos: sin guarda en este plan
}
```

Notas:
- **`fmt.FieldRaw` con `len(...) != 0`** es el caso que arregla el bug de MCP: `Error`/`Result` vacíos
  dejan de emitir `null`.
- Para `FieldStruct`/`FieldStructSlice`/`FieldIntSlice` con `omitempty`: fuera de alcance (los modelos
  MCP afectados son `FieldRaw`/`FieldText`). Mantener su generación actual.
- Mantener `DecodeFields` sin cambios.

## Paso 3 — Test del generador

Agregar un caso en los tests de `ormc` (junto a los existentes de `EncodeFields`):

- Un struct con un campo `fmt.RawJSON` con tag `omitempty:"true"` y un `string` con `omitempty:"true"`.
- Verificar que el `EncodeFields` generado contiene `if len(m.X) != 0 { w.Raw("x", m.X) }` y
  `if m.S != "" { w.String("s", m.S) }`.
- Verificar (encode runtime) que con el campo vacío la salida JSON **no** contiene la clave; con valor,
  sí la contiene.

`gotest` verde en el módulo.

## Código de referencia (estado actual a recciclar)

- Extracción de tags: `ormc/generator.go` ~232–247.
- Cómputo de `omitEmpty`: `ormc/generator.go` ~398–409 (hoy solo `json:",omitempty"`).
- `EncodeFields`: `ormc/generate.go` ~123–157 (hoy sin guardas).
- `FieldInfo.OmitEmpty` ya existe: `ormc/generator.go:29`; se serializa en el schema en
  `ormc/generate.go:93`.
