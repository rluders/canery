# Architecture: canery as an authorization kernel

> canery evaluates authorization; it does not own the systems that provide
> identity, resources, or authorization data.

An *authorization kernel* is the smallest component that can answer an
authorization question — *can this subject perform this action on this
resource within this scope?* — given facts supplied by someone else. canery is
embedded in an application as a library. It is not a service, an IAM system,
or a policy-management platform.

```text
HTTP / gRPC / jobs
      │
Authentication (yours)  ──►  Subject
      │
canery  ── Policies (optional, yours)
      │
      ├─ MembershipReader       ┐
      ├─ GroupReader            │  adapters (yours), backed by any
      ├─ PermissionReader       │  database, cache, or remote service
      └─ ResourceScopeResolver  ┘
```

## Inside the kernel

- Representing authorization requests (`Request`, `Subject`, `Action`, `ResourceRef`, `ScopeRef`).
- Evaluating the default pipeline (`Engine`): validation, resource/scope
  containment, membership, direct permissions, groups, group permissions.
- Composing evaluation with Policies (`PolicyAuthorizer`).
- Batch evaluation and optional batch reader capabilities.
- Producing `Decision` values and structured `Trace` values.

## Outside the kernel

Identity and user lifecycle, authentication, credentials, sessions, JWT
issuance or validation, organization/workspace management, role and
permission administration (APIs or UI), persistence and schema ownership,
policy authoring, audit-log storage, telemetry, resource management,
infrastructure. canery offers extension points those systems can plug into,
but never owns them.

- **Authentication** happens before canery. canery receives an already
  established `Subject` and trusts it.
- **Persistence** is behind reader interfaces. The core imports only the
  standard library and makes no storage assumption.
- **Frameworks**: no HTTP, RPC, or ORM types appear in the API.
  `AuthorizationContext` carries framework data opaquely when Policies need it.

## Layer responsibilities

| Layer | Owns | Must not |
| --- | --- | --- |
| Core evaluator (`Engine`) | The canonical model and default pipeline, `Decision` construction, core trace steps | Interpret `AuthorizationContext`, apply application rules |
| Policies | Application-specific constraints, preconditions, short-circuits, decision transformation | Re-implement membership/permission evaluation; silently redefine core primitives |
| Readers / resolvers | Authorization **facts**: "is X a member", "does principal P hold permission for R" | Decide whether a request is allowed |
| Application | Authentication, data lifecycle, mapping domain concepts to subjects/resources/scopes, administration | — |

The distinction between **facts** and **decisions** is the central boundary:

```text
PermissionReader  →  "Does this principal have this permission?"   (fact)
Engine / Policy   →  "Should this request be allowed?"             (decision)
```

### Where does a new rule belong?

- Does it apply to every canery user and follow from subject/action/resource/scope? → core (requires an ADR).
- Does it depend on application data or context (MFA, time windows, feature flags, freezes)? → Policy.
- Is it a new kind of fact (relationships, scope ancestry)? → a new optional reader capability, consumed by an evaluator.
- Does it change the meaning of an existing primitive? → it is a new evaluator, not a Policy.

A Policy that needs core facts should delegate to `next` rather than query
readers and rebuild membership or permission semantics itself.

### Decision and trace ownership

The outermost matching Policy returns the final decision; without Policies,
the `Engine` does. Core trace steps (`request_validated` …
`group_permissions_checked`) are produced only by the `Engine` and keep stable
names. Policy activity appears only as `policy_evaluated` steps. Readers do not
write to traces. See [policies.md](policies.md) and [diagnostics.md](diagnostics.md).

## Guardrails for contributions

Before adding to the core, ask:

1. Is this authorization *evaluation*, or administration of authorization data?
2. Would canery need to own application state or storage?
3. Does it introduce a storage, framework, or identity-provider assumption?
4. Does it belong in an adapter, a Policy, or a separate evaluator instead?
5. Does it create a second authorization model alongside the existing one?
6. Does it change an existing invariant? (If so, write an ADR first.)

Additions that answer "yes" to 2–5 belong outside the core.
