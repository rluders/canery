# 0008 — Batch evaluation with optional batch readers

**Status:** Accepted

**Context.** Listing pages and bulk APIs need many decisions at once;
evaluating them one by one causes N+1 reader calls.

**Decision.**
- One stage-structured pipeline evaluates any number of requests; single checks
  are a batch of one, which guarantees equivalence.
- Identical lookups are deduplicated within one call; nothing is cached across calls.
- `BatchPermissionReader` is an optional capability (type assertion) with
  positional, per-item results; readers without it keep working.
- `BatchCheckResults` returns per-item results (partial results kept);
  `BatchCheck` stays all-or-nothing, validates first, and reports the lowest
  failing index as `BatchCheckError`.
- Evaluation is sequential; canery issues no concurrent reader calls itself.

**Consequences.** Fewer round trips without changing semantics; batch
capabilities for membership, groups, and resolvers can be added per stage
later. See [batch.md](../batch.md).

**Alternatives.** Mandatory batch interfaces (rejected: breaks adapters);
built-in cache (rejected: ADR 0002); concurrent fan-out (rejected: imposes
concurrency on readers, nondeterministic call patterns).
