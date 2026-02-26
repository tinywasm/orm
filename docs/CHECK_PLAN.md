# ORM Documentation and Test Coverage Plan

This document serves as the **Master Prompt (PLAN.md)** for finalizing the documentation and achieving 100% test coverage for the `tinywasm/orm` ecosystem. Every execution agent must follow this plan sequentially.

## Development Rules

All actions must strictly adhere to the project constraints defined in `tinywasm/devflow/docs/DEFAULT_LLM_SKILL.md` and the initial `CHECK_PLAN` (e.g., zero stdlib I/O, no reflection, pure `gotest` runner, dual WASM/Stdlib build tags).

- **Documentation First:** The documentation must be updated and finalized before modifying the tests or codebase.
- **Dependency Rules:** No external assertion libraries. Continue using the standard `testing` package with the existing `MockAdapter` implementation.

---

## Phase 1: Documentation Completeness

**Goal:** Create an ultra-condensed, LLM-friendly documentation (minimal token footprint) representing the strict public API. No redundant texts.

1. **Restructure `README.md`:**
   - Keep it strictly as a central index. Keep the badges section.
   - Add a 1-line description and direct links to `docs/ARQUITECTURE.md` and `docs/SKILL.md`.

2. **Generate `docs/SKILL.md`:**
   - This file MUST be a highly condensed summary for LLM context injection.
   - Use raw data formats or extreme brevity (no filler words).
   - Outline the Public API Contract (Structs: `DB`, `QB`, `Condition`, `Order`. Interfaces: `Model`, `Adapter`. Constants & Helpers).
   - Provide only 1 minimal, multi-chained executable snippet (e.g., `db.Query(m).Where().Limit().ReadAll()`).

---

## Phase 2: 100% Test Coverage Completion

**Goal:** Exhaustive coverage across all remaining branches and unverified properties mapped in `core_test.go`.

1. **Cover Condition Helpers (`conditions.go`):**
   - Create test cases validating the returned properties (operator, field, value) for the missing helpers: `Neq()`, `Gte()`, `Lt()`, `Lte()`, `Like()`.

2. **Cover Value Getters (`orm.go`):**
   - Directly invoke and assert the output of all `Condition` module getters: `Field()`, `Operator()`, `Value()`, `Logic()`.
   - Directly invoke and assert the output of all `Order` module getters: `Column()`, `Dir()`.

3. **Cover Builder Chain Completeness (`orm.go`):**
   - Add a test invoking `Offset(n)` in the `QB` chain to ensure it propagates correctly.
   - Add a test invoking `GroupBy(cols...)` in the `QB` chain and validating the output slice.
   - Add a distinct test verifying `Limit(n)` explicitly coupled with a `ReadAll()` action (ensuring dynamic limits are handled aside from `ReadOne()`'s forced limit 1).

4. **Cover Edge Cases in Validation (`validate.go`):**
   - Assert `ErrValidation` effectively triggers during an `Update()` call (when simulating a mismatch between `Columns()` and `Values()`), ensuring the multi-action pipeline validates equally.

---

## Phase 3: Verification

1. Using headless or standard context, execute the internal runner tool `gotest` in the root folder of the package.
2. Confirm the runner explicitly outputs `coverage: 100%`.
3. Halt and request the final human sign-off once everything passes successfully.
