# PLAN: Bugs e inconsistencias encontrados en tinywasm/orm

## Bug 1: ormc stripeaba la coma inicial de tags json (RESUELTO)

**Commit:** `73e7d7b`

**Síntoma:** `json:",omitempty"` era reescrito como `json:"omitempty"` por `RewriteModelTags`.

**Causa raíz:** En `rewriteRawTag` (ormc_tags.go), al reconstruir el tag json para structs `formonly`, el `name` (primer elemento) podía ser string vacío. Al hacer `Join(",")` sin distinguir nombre vacío de ausente, se generaba `json:"omitempty"` (nombre="omitempty") en lugar de `json:",omitempty"` (sin nombre, opción omitempty).

**Cuándo NO se añade la coma:** Si el campo tiene nombre json explícito no-vacío, no debe haber coma inicial: `json:"myField,omitempty"`. La coma solo se omite cuando el nombre está presente. Si el nombre es vacío string `""`, la coma SÍ debe aparecer al inicio: `json:",omitempty"`.

**Fix aplicado:** Preservar el slot del nombre aunque sea vacío, reconstruyendo con `strings.Join(parts, ",")` donde `parts[0]` puede ser `""`.

---

## Bug 2: opción `raw` colocada en tag `json:` genera warnings de linter (PENDIENTE)

**Síntoma:** El linter (`go vet`, staticcheck) reporta:
```
Capabilities string `json:",raw"`      unknown JSON option "raw"
Result       string `json:",omitempty,raw"` unknown JSON option "raw"
```

**Causa raíz:** `raw` es una opción propietaria de tinywasm/json — indica que el campo contiene JSON pre-serializado y debe emitirse sin comillas (equivalente a `json.RawMessage`). Esta opción fue colocada en el tag estándar `json:`, pero Go solo reconoce: `omitempty`, `string`, `-`. Cualquier otro valor es inválido según el spec y el toolchain lo reporta como error.

No es un bug en el parsing de ormc — ormc lee `raw` correctamente. El problema es de diseño: se usó el namespace equivocado para información propietaria.

**Solución propuesta:** Mover `raw` al tag `db:` que ya pertenece al ecosistema tinywasm y el linter no valida:

```go
// Antes (linter warning):
Result string `json:",omitempty,raw"`

// Después (correcto):
Result string `db:"raw" json:",omitempty"`
```

**Solución adoptada:** introducir `fmt.RawJSON` como type alias `= string`, siguiendo el patrón de `json.RawMessage` en stdlib. Ver plan detallado en [tinywasm/fmt/docs/PLAN_RAW_JSON_TYPE.md](../../fmt/docs/PLAN_RAW_JSON_TYPE.md).

**Cambios requeridos en tinywasm/orm:**

1. **`ormc.go`** — detectar `typeStr == "RawJSON"` → `fieldType = fmt.FieldRaw`
2. **`ormc.go`** — mantener backward compat: seguir leyendo `raw` desde `json:` tag durante migración
3. **`ormc_tags.go` `rewriteRawTag`** — strip `raw` de json tags (ya no es opción json válida)
4. **`tests/models.go`** — actualizar `MCPResponse`: `json:"raw"` → `fmt.RawJSON`
5. **`tests/ormc_test.go`** — agregar test que valida `fmt.RawJSON` genera `Type: fmt.FieldRaw`

**Impacto:** Backward compat — seguir leyendo `raw` desde json tag hasta que todos los modelos estén migrados.

---

## Inconsistencia 3: test expectations desincronizadas tras cambio en models.go (RESUELTO)

**Commit de origen del bug:** `878cffd` — eliminó `json:",omitempty"` de `UserForm.Email`, `UserForm.Bio` y `UserWithJSON.Bio` en `models.go` pero no actualizó las expectativas de `ormc_test.go`.

**Síntoma:** `TestOrmc/Validate_tags_and_Permitted` y `TestOrmc/JSON_tags_and_Nested_structs` fallaban esperando `OmitEmpty: true` en campos sin tag json.

**Fix:** Eliminar `OmitEmpty: true` de las líneas de test correspondientes (los campos no tienen json tag, por lo tanto OmitEmpty=false es correcto).
