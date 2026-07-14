# PLAN (EJECUTADO 2026-07-14, LOCAL) — recompilado contra `model` v0.0.14

> Ejecutado directamente por el mantenedor (LOCAL, sin codejob). Fase D (propagación) de la
> ola CRUD Harness: https://github.com/tinywasm/app/blob/main/docs/CRUD_HARNESS_MASTER_PLAN.md

## El problema

`model` v0.0.14 amplía `model.Model` a `Fielder + ModuleNaming + Encodable + Decodable` (antes
solo `Fielder + ModuleNaming`). Este repo tenía dos implementadores **escritos a mano** de
`model.Model` que dejaron de compilar al ampliar el contrato:

- `emptyModel` (`db.go:95`) — sentinel privado usado solo por `CreateDatabase` (una operación
  sin modelo real). Nunca serializa nada.
- `MockModel` (`tests/setup_test.go:108`) — doble de test que solo ejercita construcción de
  queries (`Pointers()`/`Schema()`); nunca viaja por el wire.

Ninguno de los dos es un modelo de dominio que compita con `ormc` — son infraestructura interna
(sentinel y test double). El arreglo es mecánico: añadir los tres métodos que faltan como
no-ops, ya que ninguno de los dos serializa de verdad.

## Cambios ejecutados

| Archivo | Cambio |
|---|---|
| `go.mod` | `tinywasm/model` → v0.0.14 |
| `db.go` | `emptyModel` gana `IsNil() bool { return true }`, `EncodeFields`/`DecodeFields` no-op |
| `tests/setup_test.go` | `MockModel` gana `IsNil() bool { return false }`, `EncodeFields`/`DecodeFields` no-op |

`gotest ./...` verde (incluye race). Publicado con gopush como v0.9.28.

## Nota para el resto de la fase D

`user` y `sqlite` fallaban **solo por rebote** (importan `orm@v0.9.27`, que tenía el defecto de
arriba) — no tienen problema propio. Una vez publicado este fix, deberían bumpear limpio.
