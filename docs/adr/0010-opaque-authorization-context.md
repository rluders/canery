# 0010 — Opaque authorization context and safe diagnostics

**Status:** Accepted

**Context.** Policies need inputs such as authentication method or client
identity; traces and errors must be safe in production.

**Decision.**
- `Request.Context` is an immutable, comparable, key-based
  `AuthorizationContext`. The `Engine` never reads it; it is never copied into
  `Decision`, `Trace`, or errors, and it formats as a redacted placeholder.
- Traces contain only model identifiers and evaluation metadata.
- Reader failures are wrapped in `EvaluationError`, whose message names only
  the stage; the cause is available through `Unwrap`.
- canery never logs.

**Consequences.** Contextual Policies without framework types; diagnostics
safe by default; callers who need details unwrap explicitly. See
[diagnostics.md](../diagnostics.md).

**Alternatives.** A predefined context struct (rejected: grows with every
application); `map[string]any` (rejected: makes `Request` non-comparable and
mutable); passing values via `context.Context` only (rejected: invisible to
Policies' request-level contract and batch deduplication).
