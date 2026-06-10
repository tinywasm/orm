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
