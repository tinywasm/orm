# Why `ormc` Generates So Much Code (and Why It's Free)

Each struct in `model.go` produces a sizable `model_orm.go`: `ModelName()`, `Schema()`, `Pointers()`,
`IsNil()`, `EncodeFields()`, `DecodeFields()`, plus a `*List` type with eight methods. A fair question —
especially since the target is WASM, where binary size matters — is: *isn't this a lot of dead code?*

Short answer: the volume is intentional, and the parts a given program doesn't use cost **zero bytes** in
the binary.

## What gets generated, and why

| Generated | Purpose |
|---|---|
| `ModelName()` | Stable identity (table name, logs, tool names). Useful for every role. |
| `Schema()` | Field metadata (type, widget, DB/validation) as a package-level variable — zero alloc per call (see [WHY_PACKAGE_LEVEL_SCHEMA.md](WHY_PACKAGE_LEVEL_SCHEMA.md)). |
| `Pointers()` | Field addresses for zero-reflection scan/bind. |
| `EncodeFields()` / `DecodeFields()` | Zero-reflection JSON/DB codec. |
| `IsNil()` | Nil-safety inside the codec. |
| `*List` (`Len/At/Append/...`) | Zero-reflection codec for **arrays/collections** of the type. |

The ecosystem forbids `reflect` (O(1) in WASM, smaller binary, compile-time type safety — see
[ARQUITECTURE.md](ARQUITECTURE.md)). The price of "no reflect" is that the codec must exist as **concrete
generated code**. So the volume is the deliberate trade for reflection-free serialization: what other ORMs
do at runtime with reflection, `tinywasm/orm` does at build time as plain methods.

## Why unused generation does **not** bloat the binary

`ormc` generates **uniformly**: every struct gets the full set, including `*List`, whether or not a given
consumer uses it. This keeps the generator simple and predictable — no per-struct heuristics that could
desync between generator and consumers (the same fragility avoided elsewhere in the design).

Unused generated code is removed by the linker's **dead-code elimination (DCE)**. The generated code emits
no interface assertions (`var _ I = (*T)(nil)`) and no `init()` registration, so nothing forces retention;
reachability analysis strips anything never referenced.

Measured — a `*List` that implements the codec interface but is never referenced anywhere:

| Compiler | Without the `*List` | With the unused `*List` | Δ |
|---|---|---|---|
| TinyGo `-opt=z` (production / mode S) | 22,161 B | 22,161 B | **0 B** |
| Go `GOOS=js` (mode L) | 1,661,289 B | 1,661,293 B | 4 B (alignment noise) |

`go tool nm` finds no trace of the unused type or its methods in the binary.

## The trade-off

- **Cost:** source volume + a little compile time.
- **Benefit:** reflection-free serialization, uniform/predictable generation, and **zero binary cost** for
  the parts you don't use.

So the "lots of code" is generated on purpose and is free where it isn't used. Making generation
conditional to "save size" would be a net loss: it adds fragility (the generator cannot know whether a
consumer will use, say, the `*List`) and would break the array codec for collection types — while the
linker already removes whatever you don't reference.

## `Pointers()` and the `any` myth

`Pointers()` returns `[]any` (a slice of `&field` addresses) for the DB scan/bind path
(`rows.Scan(m.Pointers()...)`). It is tempting to assume the `any` boxing allocates. It does not.

Measured with `testing.AllocsPerRun`:

| Operation | allocs/op |
|---|---|
| Box a pointer (`&m.Key`, a `*string`) into `any` | 0 |
| `Scan(m.Pointers()...)` — slice escapes at the opaque driver boundary | 1 |
| `Scan(AppendPointers(buf)...)` — caller-reused buffer | 0 |

- **Boxing a pointer into `any` is free**: the pointer lives in the interface's data word — no heap object.
  (Boxing a *non-pointer* value can allocate, but every entry here is a pointer.)
- **The cost is the `[]any` slice itself** — one allocation per `Pointers()` call. It escapes because
  `Scanner.Scan(dest ...any)` is an opaque interface with multiple driver adapters (sqlite, postgres), so
  escape analysis cannot prove non-escape. A query over N rows pays N allocations.
- Unlike `Schema()`, `Pointers()` **cannot** be a package-level variable: the addresses are per-instance.

Why `[]any` at all? The DB scan contract (`Scan(dest ...any)`, like `database/sql`) needs a heterogeneous
container for heterogeneous columns. The JSON path avoids `any` entirely via the typed `FieldWriter` /
`FieldReader` codec (0 allocs) — which is why `EncodeFields` / `DecodeFields` never touch `Pointers()`.

**Can the `any` be removed?** Not for SQL drivers: the driver's `Scan(...any)` requires `[]any`; wrapping it
in a typed reader only *moves* the slice into the adapter. And the `any` was never the cost. The slice
allocation *is* reducible — to ~1 per query via a caller-reused buffer (see [IMPROVE.md](IMPROVE.md)). True
zero-alloc DB scan would require an executor exposing typed column access (a `RowReader`), worthwhile only
for backends that provide it.
