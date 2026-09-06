# Improvement Roadmap

## 1. Generic Collect[T]
Explore using Go 1.18+ generics for the `ReadAll` pattern to avoid casting.

## 2. Eager Loading
Support for automatic loading of relations to avoid N+1 queries.

## 3. Schema Evolution (v1) — DONE
Dev-time schema sync (`db.Sync`) is now implemented. It provides:
- Additive sync (ADD COLUMN).
- Rename support via `db:"old_name=X"`.
- Safe deletions (DROP COLUMN if no data).

## 4. Production Migrations
Deferred. Future plan will cover versioned, file-based migrations for production deployments.
- Build-time diffing against baseline.
- Migration tracking table (`schema_migrations`).
- Destructive operations support.

## 5. Zero-per-row scan via reused pointer buffer
`Pointers()` allocates one `[]any` per row scanned — measured at 1 alloc/op once the slice escapes the
opaque `Scanner.Scan(dest ...any)` boundary (see [webtyp/ormc docs](https://github.com/webtyp/ormc/blob/main/docs/WHY_GENERATED_CODE_IS_FREE.md)).
A query over N rows pays N allocations.

Plan: generate an `AppendPointers(dst []any) []any` method alongside `Pointers()`, and have the scan loop
(`qb.go`) reuse a single buffer across rows (`buf = m.AppendPointers(buf[:0]); rows.Scan(buf...)`). This
turns N allocations into 1 per query. Safe because `database/sql`-style `Scan` does not retain `dest` past
the call (document this invariant on the `Scanner` interface).

Note: this reduces slice allocations; it does **not** remove `any` (forced by the driver contract, and not
the allocation source). Full zero-alloc scan would require a typed-column `RowReader` contract — out of
scope unless a backend exposes typed columns.
