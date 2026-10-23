# 0004 — Default evaluation pipeline

**Status:** Accepted

**Context.** Consumers need a predictable default evaluator.

**Decision.** `Engine` evaluates: validation → resource ∈ scope (instance
requests) → membership → direct permission → groups → group permissions (reader
order, first match) → deny. Cheap, decisive checks come first; containment and
membership guard all permission lookups; direct grants are checked before the
more expensive group path. Failures are errors, never denials. Single checks
and batches share one implementation.

**Consequences.** Deterministic, explainable decisions (`Source`, `Principal`,
`Trace`). Alternate orders require a different evaluator, not configuration.

**Alternatives.** Configurable stage order (rejected: weakens invariants);
groups before direct (rejected: costlier, no semantic gain).
