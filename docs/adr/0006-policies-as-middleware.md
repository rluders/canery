# 0006 — Policies are middleware, not a policy language

**Status:** Accepted

**Context.** Applications need rules the core cannot know (MFA, freezes,
ownership) without forking the evaluator.

**Decision.** A `Policy` wraps evaluation with a `next` continuation. It can
delegate, short-circuit, transform, or fail. Bindings run in configured order
and only when their matcher accepts the request. Policies are Go code, not a
DSL. See [policies.md](../policies.md).

**Consequences.** Maximum flexibility and no new language to learn; Policy
correctness is the application's responsibility. Policies run before the base
validates the request, so they can observe invalid requests (documented
discrepancy; changing it would alter Policy semantics and needs its own ADR).

**Alternatives.** Rego/Cedar-style language (rejected: heavy, out of kernel
scope); hooks only after evaluation (rejected: cannot short-circuit).
