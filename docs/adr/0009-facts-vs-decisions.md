# 0009 — Adapters provide facts; evaluators decide

**Status:** Accepted

**Context.** If adapters start answering "is this allowed?", authorization
semantics fragment across storage code.

**Decision.** Readers and resolvers answer factual questions (membership,
groups, whether a principal holds a permission, containment). The evaluator
and Policies own decisions. Matching details inside a fact (for example,
whether a type-level grant covers an instance) belong to the reader and must be
documented by it.

**Consequences.** Readers stay simple and reusable; semantics stay in one place.
Readers must not encode allow/deny logic beyond the fact they report.

**Alternatives.** "Decision readers" (rejected: hides semantics in storage).
