# Diagnostics: traces, attribution, errors, and data exposure

canery performs **no logging**. Everything it reveals is in the values it
returns, and this document defines what each one may contain.

## Data classes

| Class | Examples | Exposed by canery? |
| --- | --- | --- |
| Authorization identifiers | subject/group/resource/scope type and ID | Yes, where needed for diagnostics (`Decision.Principal`, `TraceStep.Principal`, `BatchCheckError.Request`). Treat them with the same care as your own IDs. |
| Evaluation metadata | stage names, results, decision source, matched principal | Yes. |
| Reader internals | SQL, hosts, connection strings, storage errors | Never in error *messages*; available only via `errors.Unwrap`/`errors.As`. |
| Sensitive context | tokens, credentials, sessions, IPs, device data, claims | Never. `AuthorizationContext` is opaque and prints as `canery.AuthorizationContext{redacted}`. |

## Trace contract

`CheckTrace` returns a `Trace` whose `Steps` follow the evaluation path:

| Step (`Name`) | Results |
| --- | --- |
| `request_validated` | `passed`, `failed` |
| `resource_scope_verified` | `passed`, `denied`, `skipped` (type-level request), `error` |
| `membership_confirmed` | `passed`, `denied`, `error` |
| `direct_permission_checked` | `matched`, `not_matched`, `error` |
| `groups_resolved` | `resolved`, `none`, `error` |
| `group_permissions_checked` | `matched`, `not_matched`, `error` |
| `policy_evaluated` | `delegated`, `handled`, `overridden`, `error` |

Names and results are exported as `TraceStep*` and `TraceResult*` constants.

Structured fields:

- `Principal` — on permission steps, the principal checked (the subject for the
  direct step; the matching or failing group for the group step).
- `Source` — set only on steps that decided the outcome. The **last step with a
  non-empty `Source`** equals `Decision.Source`.

Correlation example (group allow):

```text
request_validated          passed
resource_scope_verified    passed
membership_confirmed       passed
direct_permission_checked  not_matched  principal=subject:user:1
groups_resolved            resolved
group_permissions_checked  matched      principal=group:team:editors  source=group
```

`error` always means evaluation failed and an error was returned; it never
represents a denial. On failure the trace contains the steps completed so far,
ending with the failing step.

**Not included, by design:** request fields (the caller already has the
request), context values, reader error text, timing. Durations were
considered and left out: they would add overhead to every traced check, are a
metrics concern rather than an explanation of a decision, and can be measured
around reader calls by adapters (see [adapters.md](adapters.md#observability)).

## Decision attribution

`Decision.Principal` / `Decision.EffectivePrincipal()` name the principal that
produced an allow:

- `direct` → the requesting subject (`PrincipalKind = subject`).
- `group` → the **first** matching group in `GroupReader` order. Other groups
  may hold the same permission; the reported one is the one that produced the
  match, not necessarily the only one.
- Denials → none (`EffectivePrincipal` reports `false`). An attributed
  principal with an empty ID is still reported, so "absent" and "empty ID" are
  distinguishable.

Decision and trace attribution never contradict each other.

## Error contract

| Error | When | Message exposes |
| --- | --- | --- |
| `ValidationError` | invalid request | field path and code only |
| `EvaluationError{Stage, Err}` | a reader/resolver returned an error | stage only; cause via `Unwrap` |
| `ErrMissing*` | engine misconfigured | fixed text |
| `ErrBatchResultMismatch` | batch reader returned the wrong number of results | fixed text |
| `BatchCheckError{Index, Request, Err}` | batch item failed | index and the cause's message (already sanitized) |
| `context.Canceled` / `DeadlineExceeded` | context done before a lookup | standard text |
| Policy errors | returned by your Policy | whatever your Policy returns, unchanged |

Readers should still avoid putting secrets in their errors, because callers can
unwrap them. Policies are responsible for what their own errors and decisions
(including `Reason`) reveal.

## Guidance for applications

- Do not return `Trace`, `BatchCheckError.Request`, or unwrapped reader errors
  to untrusted clients. Map denials to a generic 403/404.
- Logging `Decision` and `Trace` is safe from canery's side; whether your IDs
  are sensitive is your call.
- Never put credentials in subject/resource/scope IDs or in `Action`.
