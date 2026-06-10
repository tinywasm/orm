# DB Sync — visión tool-driven (la herramienta sincroniza, no el proyecto)

> El proyecto del usuario importa **solo `orm`** (agnóstico) y **no contiene código de sync**.
> `tinywasm/app` (la herramienta) observa los ficheros y aplica el schema a una DB que **ella** posee.
> `ormc` es **ciego a la base de datos**: opera solo con la interfaz `SchemaSyncer` inyectada.
>
> **Una sola lectura.** Al arrancar, `devwatch` hace **un único `filepath.Walk`** del proyecto y va
> pasando **cada fichero** a los manejadores vía `NewFileEvent`. `ormc` procesa **solo el fichero
> recibido** — nunca re-camina el proyecto. La selección de "qué manejador es dueño de qué fichero"
> ya la resuelve **depfind** (`ThisFileIsMine`). El walk propio solo ocurre en el CLI standalone.

## 1. Setup de conexión (una vez, al arrancar app)

```mermaid
flowchart TD
    Env["tinywasm/app lee .env<br/>DATABASE_CONNECTION=postgres://... | sqlite://..."]
    Env --> Open["orm.Open(dsn)<br/>(raíz, agnóstico — parsea el scheme)"]
    Open --> Registry["Registry: busca el Factory del scheme<br/>(adapters se auto-registran en init)"]
    Registry --> Pick{"¿scheme?"}
    Pick -- "postgres" --> PG["tinywasm/postgres<br/>Executor + Compiler"]
    Pick -- "sqlite / in-memory" --> SQ["tinywasm/sqlt<br/>Executor + Compiler"]
    PG --> DB["*orm.DB listo"]
    SQ --> DB
    DB --> Inject["app envuelve db en dbSyncer (SchemaSyncer)<br/>y lo inyecta: ormc.Generator.SetSyncer(dbSyncer)"]
```

## 2. Dos modos de ejecución de ormc (de dónde vienen los ficheros)

```mermaid
flowchart TD
    Mode{"¿Cómo se ejecuta ormc?"}

    Mode -- "A) Integrado en tinywasm/app" --> WatchScan["devwatch: UN solo filepath.Walk al arrancar"]
    WatchScan --> DepGate["depfind: ThisFileIsMine?<br/>(gate de propiedad por manejador)"]
    DepGate --> PerFile["Por cada fichero: handler.NewFileEvent(file, ext, path, evt)"]
    PerFile --> LiveEdit["Edición en vivo: el watcher emite<br/>UN evento por el fichero que cambió"]
    LiveEdit --> ProcessOne["ormc procesa SOLO ese fichero<br/>(NO re-camina el proyecto)"]

    Mode -- "B) CLI standalone (cmd/ormc/main.go)" --> CliWalk["No hay watcher → ormc.Run()<br/>camina el proyecto por su cuenta (única vez)"]
    CliWalk --> ProcessAll["Procesa todos los model.go encontrados"]

    ProcessOne --> Done(["→ §3 procesamiento por fichero"])
    ProcessAll --> Done
```

## 3. Procesamiento por fichero (modo app — sin re-walk)

```mermaid
flowchart TD
    Event(["NewFileEvent(fileName, ext, filePath, evt)"]) --> IsModel{"¿fileName es<br/>model.go / models.go?"}
    IsModel -- "No" --> Skip(["return nil (otro manejador lo maneja)"])
    IsModel -- "Sí" --> ParseOne["Parsear SOLO filePath (AST)<br/>→ structs de ese fichero"]
    ParseOne --> Cache["Actualizar cache en memoria de structs<br/>(acumulado entre eventos)"]
    Cache --> Relations["Resolver relaciones contra el cache<br/>(refs FK a otras tablas ya vistas)"]
    Relations --> GenFile["Generar <file>_orm.go<br/>(Schema() refleja el nuevo campo)"]
    GenFile --> HasSyncer{"¿syncer inyectado?"}
    HasSyncer -- "No (CLI ormc)" --> OnlyGen(["Solo codegen, sin tocar DB"])
    HasSyncer -- "Sí (modo app)" --> LoopStructs["Por cada struct DB del fichero (!NoDB):<br/>syncer.SyncSchema(table, []fmt.Field)"]
    LoopStructs --> Blind["ormc ciego a la DB:<br/>solo conoce la interfaz SchemaSyncer"]
    Blind --> AppSync["app dbSyncer → db.SyncSchema(table, fields)<br/>envuelve en modelo sintético → db.Sync"]
    AppSync --> Reconcile["db.Sync reconcilia (ver §4)<br/>emite Actions agnósticas"]
    Reconcile --> Translate["adapter (postgres/sqlt):<br/>traduce Actions → SQL de dialecto"]
    Translate --> Applied["DB actualizada en tiempo real"]

    GenFile -. en paralelo .-> Recompile["WasmClient / Server recompilan<br/>contra el nuevo <file>_orm.go"]
```

## 4. Reconciliación interna de db.Sync (additiva + introspectiva)

```mermaid
flowchart TD
    Start(["db.Sync / db.SyncSchema"]) --> Create["Emitir ActionCreateTable<br/>(IF NOT EXISTS — idempotente)"]
    Create --> InspectDB["Introspección: columnas reales en la DB<br/>(si el Executor implementa TableIntrospector)"]
    InspectDB --> LoopFields["Iterar campos del Schema() en Go"]

    LoopFields --> HasRenameTag{"¿Tiene tag<br/>old_name=X?"}
    HasRenameTag -- "Sí" --> CheckOldCol{"¿Existe columna X en la DB<br/>y NO existe el nuevo campo?"}
    CheckOldCol -- "Sí" --> ExecRename["ActionRenameColumn<br/>ALTER TABLE RENAME COLUMN X TO nuevo"] --> LoopFields
    CheckOldCol -- "No" --> AddOrSkip
    HasRenameTag -- "No" --> AddOrSkip{"¿Existe la columna<br/>en la DB?"}

    AddOrSkip -- "No" --> ExecAdd["ActionAddColumn<br/>ALTER TABLE ADD COLUMN"] --> LoopFields
    AddOrSkip -- "Sí" --> LoopFields

    LoopFields -- "Fin de campos Go" --> FindObsoletes["Identificar columnas en DB<br/>que ya no están en Go"]
    FindObsoletes --> LoopObsoletes["Iterar columnas obsoletas"]

    LoopObsoletes --> CheckData{"¿Tiene datos?<br/>(SELECT 1 WHERE col IS NOT NULL LIMIT 1)"}
    CheckData -- "No" --> ExecDrop["ActionDropColumn<br/>ALTER TABLE DROP COLUMN"] --> LoopObsoletes
    CheckData -- "Sí" --> KeepAndWarn["Mantener columna + Log Warning"] --> LoopObsoletes

    LoopObsoletes -- "Fin de obsoletas" --> End(["Fin Sync"])
```

## Quién posee qué

| Capa | Responsabilidad | Conoce la DB |
|------|-----------------|--------------|
| **tinywasm/app** (herramienta) | Lee `.env`, `orm.Open`, construye `*orm.DB`, inyecta `dbSyncer` | Sí (elige motor) |
| **devwatch** | UN walk al arrancar + eventos en vivo; gate depfind por fichero | No |
| **ormc.Generator** | Procesa **el fichero recibido**, regenera `<file>_orm.go`, llama `SchemaSyncer` | **No** (ciego) |
| **orm raíz** | `Open`/`Register`, `SyncSchema`/`Sync`, emite `Action`s | No (agnóstico) |
| **postgres / sqlt** | `init()` registra factory; traduce `Action`s → SQL | Sí (dialecto) |
| **proyecto del usuario** | Define modelos. Importa solo `orm`. | **Sin código de sync** |

> **Nota de eficiencia:** el walk completo (`Run()` / `collectAllStructs`) queda **solo** en el CLI
> standalone. En modo app, cada `NewFileEvent` procesa un fichero y se apoya en el cache + depfind;
> nunca se re-lee el proyecto entero por evento.
