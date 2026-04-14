# Plan de Corrección: Eliminar campo `ModelName` de la estructura anónima de metadatos

## Objetivo
Eliminar la generación del campo `ModelName` dentro de la estructura anónima de metadatos (por ejemplo, `AgentSwitch_`) que es producida por `ormc`. Esto es redundante y código innecesario, debido a que el generador ya se encarga de crear el método `ModelName() string` en el modelo directamente.

## Pasos de Implementación

### 1. Modificar el generador en `ormc`
**Archivo a modificar:** `ormc_generate.go`
**Ubicación:** Alrededor de las líneas 131 - 141 (sección `// Metadata Descriptors`).

Hay que remover las instrucciones `buf.Write` que agregan la definición y la asignación del campo `ModelName`.

**Código a eliminar (o modificar):**
```go
// Metadata Descriptors
buf.Write(fmt.Sprintf("var %s_ = struct {\n", info.Name))
buf.Write("\tModelName string\n") // <-- ELIMINAR ESTA LÍNEA
for _, f := range info.Fields {
	buf.Write(fmt.Sprintf("\t%s string\n", f.Name))
}
buf.Write("}{\n")
buf.Write(fmt.Sprintf("\tModelName: \"%s\",\n", info.ModelName)) // <-- ELIMINAR ESTA LÍNEA
for _, f := range info.Fields {
	buf.Write(fmt.Sprintf("\t%s: \"%s\",\n", f.Name, f.ColumnName))
}
buf.Write("}\n\n")
```

De esta manera, la estructura anónima solo contendrá los nombres de los campos de la base de datos (e.g. `ID`, `IsEnabled`, etc).

### 2. Actualizar Tests del Generador
**Archivos a evaluar:** 
- `tests/ormc_test.go`
- `tests/ormc_multi_test.go`

**Acciones:**
Verificar si hay assertions (validaciones) sobre el código generado, en las cuales se espere literal que la cadena de texto incluya `ModelName string`. En dado caso, remover esas aserciones para que coincidan con la nueva salida sin el campo `ModelName`.

### 3. Testear, Instalar y Verificar
1. Ejecutar los tests en `tinywasm/orm`:
   ```bash
   go test ./...
   ```
2. Re-instalar el ejecutable `ormc` a nivel global (desde `/home/cesar/Dev/Project/tinywasm/orm`):
   ```bash
   cd cmd/ormc && go install
   ```
3. Ir a los módulos consumidores como `/home/cesar/Dev/Project/veltylabs/velty_modules/agent-switch` y ejecutar nuevamente el comando `ormc`.
4. Comprobar que en `model_orm.go` la estructura anónima ya no cuenta con el campo `ModelName`, pero manteniendo todo funcionando correctamente.
