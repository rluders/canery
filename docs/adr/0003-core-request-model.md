# 0003 — Subject · Action · Resource · Scope request model

**Status:** Accepted

**Context.** A request model must be small enough to stay generic and rich
enough for multi-tenant applications.

**Decision.** A `Request` is (Subject, Action, Resource, Scope), with typed
references of `Type` + `ID`. `Resource.ID` may be empty to express type-level
questions. One explicit scope per request. Optional, opaque
`AuthorizationContext` carries extra inputs without widening the model (ADR 0010).

**Consequences.** The same model serves direct, group, and future evaluators.
Hierarchy and relationships require extra capabilities (ADR 0011), not new
request fields.

**Alternatives.** Free-form attribute maps (rejected: unvalidated and
untyped); domain-specific fields such as tenant or role (rejected: product
semantics in the core).
