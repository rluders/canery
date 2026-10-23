# 0005 — Membership is a prerequisite, not a grant

**Status:** Accepted

**Context.** Scopes model tenants/projects. Leaking access across scopes is the
most damaging authorization bug in multi-tenant systems.

**Decision.** In the default pipeline a subject must be a member of the
requested scope before any permission is evaluated. Membership alone grants
nothing. Non-membership is a denial (`subject not in scope`); a membership
reader failure is an error.

**Consequences.** A stray permission row cannot grant access outside a scope
the subject belongs to. Cross-scope access (e.g. support staff) must be
modelled as membership or handled in a Policy.

**Alternatives.** Membership as an implicit grant (rejected: conflates
belonging with capability); no membership stage (rejected: loses tenant
isolation by default).
