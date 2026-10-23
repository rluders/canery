# Migrating from v1 to v2

## Import path

```sh
go get github.com/rluders/canery/v2
```

```go
import "github.com/rluders/canery/v2" // package name is still canery
```

## Breaking changes

| Area | v1 | v2 | What to do |
| --- | --- | --- | --- |
| `Decision.Source` | `string` | `DecisionSource` (values unchanged: `direct`, `group`, `none`) | Comparisons with the constants keep working. Convert with `string(d.Source)` where a `string` is required; use `canery.DecisionSource("x")` for custom sources from variables. |
| Validation | `Field: "subject"`, `Code: "required"` for both type and ID | adds `Property` (`"type"`/`"id"`) and `Path()`; `Error()` reads `…: subject.id: required` | `Field` and `Code` keep their values; switch on `Path()` for detail. Update tests that compare error strings. |
| Reader errors | returned as-is | wrapped in `EvaluationError{Stage, Err}`; message no longer contains the reader's text | Use `errors.Is` / `errors.As` instead of `==` or string matching. |
| `BatchCheck` | evaluated item by item; only validation errors were wrapped | validates all items first; every failure is a `BatchCheckError` (lowest index) | `errors.Is`/`errors.As` still reach the cause. |
| Cancellation | a cancelled context could still evaluate with in-memory readers | returns `ctx.Err()` before calling readers | Treat it as an error (it already was for I/O-backed readers). |
| `Decision` | `Allowed`, `Reason`, `Source` | adds `Principal` | Struct equality in tests must include `Principal` for allows. |
| `TraceStep` | `Name`, `Result` | adds `Principal`, `Source`; new `skipped`, `error`, `overridden` results; partial traces on failure | Update tests asserting exact steps. |
| `MultiActionBuilder.Check` | called `Check` per action | uses `BatchCheckResults` when available, otherwise `CheckDecision` per action; `ActionDecision` adds `Decision` | Custom `Authorizer`s should keep `Check` and `CheckDecision` consistent. |
| `Request` | four fields | adds `Context AuthorizationContext` | Unkeyed composite literals (`Request{s, a, r, sc}`) must use field names. |

## New, additive APIs

`BatchCheckResults`, `BatchResult`, `BatchAuthorizer`, `BatchPermissionReader`,
`PermissionCheck`, `PermissionResult`, `ErrBatchResultMismatch`,
`EvaluationError`, `AuthorizationContext`, `ContextValue`,
`Builder.WithContext`/`WithValue`, `Decision.EffectivePrincipal`,
`ValidationError.Path`, `ValidationCodeRequired`, `TraceStep*` and
`TraceResult*` constants, `PolicyAuthorizer.BatchCheck`/`BatchCheckResults`.

Authorization decisions are unchanged: the same requests and reader data
produce the same `Allowed`, `Reason`, and `Source` values as in v1.
