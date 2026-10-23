# 0007 — No built-in deny/allow precedence

**Status:** Accepted

**Context.** Combining algorithms (deny-overrides, allow-overrides, first
applicable) are where many policy systems become hard to reason about.

**Decision.** The core has no deny rules and no precedence model. The `Engine`
only grants on a matching permission and otherwise denies. In a Policy chain,
the outermost matching Policy's returned decision is the result.

**Consequences.** Simple, local reasoning. Applications needing explicit deny
or a combining algorithm implement it in a Policy and own its semantics.

**Alternatives.** Built-in explicit deny (rejected for now: requires a
precedence model, storage conventions, and new decision sources; would need a
new ADR).
