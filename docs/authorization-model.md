# Authorization model and invariants

This document describes the model implemented by the default `Engine`. The
implementation is the source of truth; the tests in `model_test.go`,
`validation_test.go`, and `batch_test.go` pin the invariants listed here.

## Request

A `Request` is one authorization question:

> Can **Subject** perform **Action** on **Resource** within **Scope**?

| Field | Meaning |
| --- | --- |
| `Subject` (`Type`, `ID`) | The actor asking. Already authenticated by the application. `ActorRef` is an alias. |
| `Action` | The operation, as an application-defined string (`"read"`, `"invoice.approve"`). canery attaches no meaning to it. |
| `Resource` (`Type`, `ID`) | The target. `ID` may be empty (see below). |
| `Scope` (`Type`, `ID`) | The single boundary in which the question is asked, such as a project or tenant. |
| `Context` | Optional `AuthorizationContext` for Policies and custom evaluators. The `Engine` never reads it. |

Requests are values. canery copies them and never mutates the caller's copy.

## Resource type vs resource instance

| Form | Example | Meaning |
| --- | --- | --- |
| Type (empty `ID`) | `document` | Authorization over the resource *type* within the scope — typically create, list, or other operations where no instance exists yet. |
| Instance (non-empty `ID`) | `document:123` | Authorization over one specific resource. |

The `Engine` treats the two forms differently in exactly one place:

- An **instance** request first asks `ResourceScopeResolver.ResourceInScope`
  whether the resource belongs to the requested scope. If it does not, the
  request is denied (`"resource outside scope"`). If no resolver is configured,
  evaluation fails with `ErrMissingScopeResolver`.
- A **type** request skips this check (trace: `resource_scope_verified` /
  `skipped`), and needs no resolver.

The `Engine` passes the `Request` to `PermissionReader` unchanged. **Whether a
type-level grant satisfies an instance request (or vice versa) is decided by
the reader**, not the engine. canery provides no wildcard or inheritance
semantics. For example, the `examples/advanced` store matches grants by
resource type only, so a project-level `document` grant covers every
document instance, while a reader that matches type and ID exactly would not.

| Question | Form |
| --- | --- |
| Can the subject create a document within the project? | type (`document`) |
| Can the subject read / update / delete `document:123` within the project? | instance |

## Scope

A scope is a single, independent boundary. Hierarchical scopes are **not**
part of the current semantics: `project:1` does not imply anything about its
workspace or organization. See [design/hierarchical-scopes.md](design/hierarchical-scopes.md)
for the proposed evolution.

## Membership

Membership is a **prerequisite**, not a permission grant:

```text
not a member  →  denied ("subject not in scope"); permissions are never consulted
member        →  permission evaluation proceeds; membership alone grants nothing
```

- `MembershipReader.HasMembership(subject, scope)` defines membership; the
  scope is the request's `Scope`.
- Membership is required for every request that reaches the membership stage
  (i.e. valid and, for instance requests, inside the scope).
- A non-member denial is `Decision{Allowed: false, Reason: "subject not in scope", Source: "none"}`
  with trace step `membership_confirmed` / `denied`.
- A membership **reader failure** is not a denial: it returns an
  `EvaluationError{Stage: "membership_confirmed"}` wrapping the reader's error,
  a zero `Decision`, and trace step `membership_confirmed` / `error`.
- No direct or group permission can bypass membership.

All entry points (`Check`, `CheckDecision`, `CheckTrace`, the fluent builders,
`BatchCheck`, `BatchCheckResults`, and `PolicyAuthorizer` without overriding
Policies) apply the same semantics because they share one pipeline.

## Permissions

1. **Direct**: `HasPermission(SubjectPrincipal(subject), request)`.
2. **Group-derived**: `GroupsForSubject(subject, scope)`, then
   `HasPermission(GroupPrincipal(group), request)` for each group in the order
   the reader returned them, stopping at the first match.

Direct permissions are always evaluated first. Being in a group grants
nothing without a matching group permission.

## Evaluation order

```text
1. validate request                          invalid  → ValidationError
2. resource ∈ scope   (instance requests)    no       → deny "resource outside scope"
3. subject ∈ scope                           no       → deny "subject not in scope"
4. direct permission                         yes      → allow (source=direct)
5. resolve groups, check each in order       yes      → allow (source=group)
6.                                           none     → deny "no matching permission"
```

Any reader failure stops evaluation with an `EvaluationError`. If the context
is cancelled, evaluation stops with the context's error.

## Decision

| Field | Guarantee |
| --- | --- |
| `Allowed` | The only field with authorization meaning. |
| `Source` (`DecisionSource`) | `direct`, `group`, or `none` from the `Engine`. Always `none` for denials. |
| `Reason` | Human-readable explanation, stable for the `Engine`: `direct permission matched`, `group permission matched`, `resource outside scope`, `subject not in scope`, `no matching permission`. Policies may return their own. |
| `Principal` / `EffectivePrincipal()` | The principal whose permission matched: the subject for `direct`, the first matching group for `group`. Zero for denials. |

A `Decision` does not represent *all* reasons a request might be allowed (other
groups may also match), does not contain permission records, and never carries
context values or diagnostics. When an error is returned, the `Decision` is zero
and must be ignored.

## Validation contract

Validation runs before any reader is called and stops at the first failure, in
this order:

| Order | `Field` | `Property` | `Path()` | Rule |
| --- | --- | --- | --- | --- |
| 1 | `subject` | `type` | `subject.type` | non-blank |
| 2 | `subject` | `id` | `subject.id` | non-blank |
| 3 | `action` | | `action` | non-blank |
| 4 | `resource` | `type` | `resource.type` | non-blank (`ID` is optional) |
| 5 | `scope` | `type` | `scope.type` | non-blank |
| 6 | `scope` | `id` | `scope.id` | non-blank |

"Blank" means empty or whitespace only. `Code` is always `ValidationCodeRequired`
(`"required"`). Errors satisfy `errors.Is(err, ErrInvalidRequest)` and
`errors.As(err, &ValidationError{})` from every entry point; batch APIs wrap
them in `BatchCheckError` (see [batch.md](batch.md)). `Error()` reads
`canery: invalid request: <path>: <code>`.

## Invariants

1. Invalid requests never produce an allow, and no reader is called for them.
2. An instance request must be inside the requested scope before membership or permissions are evaluated.
3. The subject must be a member of the requested scope before permissions are evaluated.
4. Membership, group membership, and resource containment never grant access on their own.
5. Direct permissions are evaluated before group-derived permissions.
6. Group permissions are evaluated in reader order; the first match decides and is reported.
7. No match results in denial with `Source = none`.
8. Reader failures and cancellation are errors, never denials.
9. Decisions do not depend on state retained between calls; lookups are memoised only within one call.
10. A batch result equals the result of evaluating the same request alone.
11. The `Engine` does not interpret `AuthorizationContext`.

## Known gaps (intentionally unchanged)

- `PolicyAuthorizer` runs Policies *before* the wrapped `Engine` validates the
  request, so Policies can observe invalid requests. See [policies.md](policies.md).
- There are no deny rules, precedence rules, hierarchical scopes, roles, or
  relationship-based semantics in the core.
