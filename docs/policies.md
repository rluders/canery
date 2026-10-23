# Policy semantics

A `Policy` is **evaluation middleware** around an `Authorizer`. It is not a
policy language, a rule engine, or a precedence system.

```go
type Policy interface {
    CheckDecision(ctx context.Context, request Request, next DecisionEvaluator) (Decision, error)
}
```

## Chaining

`NewPolicyAuthorizer(base, bindings...)` evaluates bindings in the order given.
For a request, only bindings whose `Match` accepts it run (a nil `Match`
matches everything; a nil `Policy` is skipped). The first matching binding
runs first; its `next` continues with the next matching binding; the last
`next` calls `base.CheckDecision`.

```text
Policy A ─next→ Policy B ─next→ Policy C ─next→ base (Engine)
   ↑ result returned outward through C, B, A
```

The order is deterministic.

## What a Policy may do

| Behaviour | How | Semantics |
| --- | --- | --- |
| Delegate | `return next.CheckDecision(ctx, request)` | Result of the rest of the chain. |
| Short-circuit | Return a `Decision` without calling `next` | A valid, final decision (allow **or** deny). Inner Policies and the base are not called. |
| Precondition | Check something, then delegate or return a denial | Common pattern for MFA, freezes, feature flags. |
| Transform | Call `next`, return a different `Decision` | Allowed in both directions (allow→deny, deny→allow). |
| Delegate with a different request | Call `next` with a modified request | Allowed; only the bindings after this one are considered, matched against the modified request. |
| Fail | Return an error | Propagates unchanged to the caller. |

There is **no** built-in deny-overrides or allow-overrides: the value returned
by the outermost matching Policy *is* the result. If you need precedence,
implement it explicitly in a Policy and document it in your application.

## Errors

- A Policy error is returned to the caller unchanged (not wrapped), and the
  returned `Decision` must be ignored.
- Errors from `next` (inner Policies or the base) are whatever the Policy
  returns; Policies should normally propagate them unchanged.
- Never convert an error into a denial silently; a denial means "evaluated and
  not allowed", an error means "could not be evaluated".

## Tracing

`PolicyAuthorizer.CheckTrace` records one `policy_evaluated` step per matching
Policy that ran:

| Result | Meaning |
| --- | --- |
| `delegated` | The Policy called `next`; the steps of the delegated evaluation follow. |
| `handled` | The Policy short-circuited; `Source` is the returned decision's source. |
| `overridden` | Appended after delegated steps when the Policy returned a decision different from `next`'s. |
| `error` | The Policy (or something it delegated to) returned an error. |

If a Policy calls `next` more than once, the trace contains the steps of the
last call. Policies cannot add custom steps; Policy-local data never enters the
trace. `CheckDecision` and `CheckTrace` return the same decision.

## Validation

Policies run before the base `Engine` validates the request, so a Policy can
receive an invalid request. A Policy that short-circuits with an allow on an
invalid request would bypass validation; Policies that allow should do so only
after delegating, or validate inputs they rely on.

## Concurrency

Policies are shared across goroutines and must be safe for concurrent use.
Do not store per-request state on the Policy; use the request,
`AuthorizationContext`, or local variables. Do not call `next` concurrently
from multiple goroutines within one evaluation.

## Batches

`PolicyAuthorizer.BatchCheckResults` forwards requests that match no binding
to the base as one batch (when the base implements `BatchAuthorizer`) and
evaluates matched requests through their chain individually. Results keep
input order.

## Core vs Policy responsibilities

See [architecture.md](architecture.md#layer-responsibilities). In short:
Policies add application rules around the canonical model; they should not
re-query readers to rebuild membership or permission semantics. Use
`AuthorizationContext` for contextual inputs (authentication method, client,
validated claims) instead of global state.
