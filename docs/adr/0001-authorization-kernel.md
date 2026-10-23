# 0001 — canery is an authorization kernel

**Status:** Accepted

**Context.** Authorization libraries tend to grow into IAM platforms (users,
roles, admin UIs, policy stores). Each addition imposes a model on every user.

**Decision.** canery only *evaluates* authorization questions from facts
supplied through interfaces. Identity, authentication, persistence,
administration, audit storage, and telemetry stay outside. See
[architecture.md](../architecture.md).

**Consequences.** Small, embeddable, framework-free core; applications write
adapters. Features such as role management are deliberately unavailable.

**Alternatives.** Full IAM (rejected: scope and lock-in); standalone
authorization service (rejected: canery is a library to embed).
