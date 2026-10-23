# Batch evaluation

## APIs

| API | Kind | Failure model |
| --- | --- | --- |
| `Engine.BatchCheckResults(ctx, requests) []BatchResult` | True batch | Per-item results; partial results are kept. |
| `Engine.BatchCheck(ctx, requests) ([]Decision, error)` | True batch | All-or-nothing. |
| `PolicyAuthorizer.BatchCheckResults` / `BatchCheck` | Batch where no Policy matches, per-request otherwise | Same as the `Engine` equivalents. |
| `Builder.CanMany(...).Check(ctx)` | Uses `BatchCheckResults` when the authorizer implements `BatchAuthorizer` | Per-action results in a map. |

All batch APIs return results in **input order**: result `i` belongs to
request `i`. `BatchResult.Request` echoes the request for convenience.

## Equivalence

`Check`, `CheckDecision`, and `CheckTrace` are implemented as a batch of one
over the same pipeline. For any request and equivalent reader data, the
decision (and error class) in a batch is the same as evaluating the request
alone. `TestBatchCheckResultsMatchIndependentChecks` enforces this.

## How the pipeline batches

Each stage processes every undecided request before the next stage starts:

```text
validate all → resource ∈ scope → membership → direct permissions → groups → group permissions
```

Within one call, identical lookups are performed once:

| Stage | Deduplication key |
| --- | --- |
| Resource ∈ scope | resource + scope |
| Membership | subject + scope |
| Groups | subject + scope |
| Permissions | principal + request (`PermissionCheck`) |

Memoisation lasts only for the call. Nothing is cached between calls.

### Optional `BatchPermissionReader`

```go
type BatchPermissionReader interface {
    PermissionReader
    HasPermissions(ctx context.Context, checks []PermissionCheck) ([]PermissionResult, error)
}
```

- Detected by type assertion; implementing it is optional.
- Used for a stage when it needs **more than one distinct** check; a single
  check uses `HasPermission`.
- Direct checks for the whole batch go in one call; every group check of the
  batch goes in a second call. A single request with N groups therefore costs
  two permission reader calls instead of N+1.
- Checks in one call are distinct. Return exactly one `PermissionResult` per
  check, in the same order (`ErrBatchResultMismatch` otherwise).
- `PermissionResult{Allowed: false}` is "no match"; `PermissionResult.Err` is a
  per-check failure; a non-nil returned error fails every check in the call.

Without the capability, group permissions are checked one at a time in group
order with early termination, exactly like a sequential loop, and memoised
across the batch.

Group evaluation scans each request's groups in reader order: an error on a
group checked before any match fails the request; an error on a group after the
first match is ignored, because sequential evaluation would never have asked.

Membership, group, and resolver batch capabilities are not defined yet; the
pipeline's stage structure leaves room for them (see [ADR 0008](adr/0008-batch-architecture.md)).

### Concurrency

Batches are evaluated sequentially on the calling goroutine. canery never
issues concurrent reader calls on its own; concurrency only comes from callers
sharing an `Engine`.

## Outcomes

Every request in a batch ends in exactly one state:

| State | `BatchResult` | `BatchCheck` |
| --- | --- | --- |
| Allowed | `Err == nil`, `Decision.Allowed` | decision |
| Denied | `Err == nil`, `!Decision.Allowed` | decision |
| Not evaluated | `Err != nil`, zero `Decision` | whole call fails |

A denial is never an error, and an error is never reported as a denial.

### `BatchCheckResults`

- Invalid requests get their `ValidationError`; valid requests are still evaluated.
- A reader failure affects only the requests that depended on the failed lookup
  (for example, one membership failure fails the requests for that subject and
  scope only). A failed `HasPermissions` call fails the requests that needed it.
- Missing readers (`ErrMissingMembership`, …) fail every valid request.

### `BatchCheck`

1. Validates every request first. If any is invalid, returns
   `BatchCheckError{Index: <first invalid>}` without calling any reader.
2. Otherwise evaluates the batch. If any request failed, returns nil decisions
   and `BatchCheckError` for the **lowest** failing index.
3. Otherwise returns one decision per request.

`BatchCheckError` unwraps to the cause, so `errors.Is(err, ErrInvalidRequest)`,
`errors.As(err, &ValidationError{})`, `errors.As(err, &EvaluationError{})`, and
`errors.Is(err, context.Canceled)` all work.

### `CanMany`

`MultiActionResult.Decisions[action]` holds `Allowed`, `Err`, and the full
`Decision`. Per-action errors never abort other actions; the method's own
error is only `ErrMissingAuthorizer`.

## Cancellation

The context is checked before each stage and before each reader call. When it
is done, every request not yet decided fails with `ctx.Err()` (unwrapped), and
no further readers are called. Requests already decided keep their decisions.
Cancellation is never converted into a denial. Cancellation inside a Policy
is whatever that Policy returns.
