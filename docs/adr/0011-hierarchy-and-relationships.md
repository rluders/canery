# 0011 — Hierarchical scopes and relationships as optional capabilities

**Status:** Proposed

**Context.** Applications have scope trees and relationship facts
(owner, editor). The core currently treats scopes as independent and knows no
relationships.

**Decision (proposed).** Add these only as optional reader capabilities that
supply facts — `ScopeHierarchyResolver`, `ResourceContextResolver`,
`RelationshipReader` — consumed by evaluators or Policies that define their
meaning. No inheritance or relation-to-permission mapping in the default
`Engine`. Designs: [hierarchical scopes](../design/hierarchical-scopes.md),
[resource context](../design/resource-context.md),
[relationships](../design/relationships.md).

**Consequences.** Existing adapters and decisions are unaffected; inheritance
semantics require a separate, accepted ADR before implementation.

**Open.** Membership semantics across ancestors; whether ancestor grants
propagate; batch variants.
