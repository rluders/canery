# 0002 — Storage-agnostic core

**Status:** Accepted

**Context.** Authorization data lives in many places: SQL, caches, directories,
remote services.

**Decision.** The core depends only on the standard library and accesses data
through `MembershipReader`, `GroupReader`, `PermissionReader`, and
`ResourceScopeResolver` (plus optional capabilities such as
`BatchPermissionReader`). The core never stores authorization state.

**Consequences.** Any backend works; performance depends on adapters, so
canery documents invocation patterns ([adapters.md](../adapters.md)) and
offers batch capabilities instead of built-in caching.

**Alternatives.** Shipping a database layer or cache (rejected: imposes
schema, consistency, and invalidation choices).
