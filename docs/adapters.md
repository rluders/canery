# Adapter contracts and performance

Readers and resolvers supply **facts**; the engine makes **decisions**. A
reader answers "is this subject a member of this scope?" — never "should this
request be allowed?".

## Invocation pattern

For one request, the `Engine` performs at most:

```text
ResourceInScope      0–1   (only when Resource.ID is set)
HasMembership        0–1
HasPermission        0–1   (direct)
GroupsForSubject     0–1
HasPermission        0–G   (one per group until the first match)
  or HasPermissions  0–1   (BatchPermissionReader, when G > 1)
```

Evaluation stops at the first decisive stage, so most denials and direct
allows touch only two or three readers.

For a batch of N requests, each stage issues one call per **distinct** key
(see [batch.md](batch.md#how-the-pipeline-batches)). N requests for the same
subject and scope cost one membership and one group lookup in total.

Conceptually: a single check is O(1) evaluator stages plus reader cost; a batch
is O(N) logical decisions with shared reader work. Actual cost depends on your
readers.

## Per-interface expectations

| Interface | Called when | Guidance |
| --- | --- | --- |
| `ResourceScopeResolver` | instance requests that passed validation | Answer containment with the cheapest query possible (`EXISTS`, index lookup). Do not load the resource. `false` = not in scope (denial); error = could not determine. |
| `MembershipReader` | every request that reaches the membership stage | Usually the hottest call. Index by (subject, scope). |
| `GroupReader` | members without a direct match | Return groups in a **stable** order: it decides which group is reported. Do not return the caller-visible slice of a shared cache unless it is never mutated. |
| `PermissionReader` | direct check, then each group | Implement `BatchPermissionReader` if your store can answer many checks in one round trip. Type vs instance matching is yours to define and document. |
| `BatchPermissionReader` | stages with >1 distinct check | One result per check, same order. Use `PermissionResult.Err` for per-check failures. |

All methods:

- receive a `context.Context`; honour cancellation and deadlines;
- may be called **concurrently** when an `Engine` is shared; canery does not
  serialize access, so synchronize shared state yourself;
- receive values they must treat as read-only;
- return `(false, nil)` for "no", and errors only for failures.

## Request-local reuse vs persistent caching

canery memoises lookups **only within a single call** (`Check`, `BatchCheck`,
`BatchCheckResults`, `CanMany`). Nothing survives the call, so no
invalidation is needed and decisions always reflect what readers return.

Persistent caching (across calls) is an adapter concern. If you add it, you own
staleness, invalidation on permission/membership changes, and the security
consequences of serving stale grants. canery neither provides nor requires it.

## Implementation sketches

**SQL.** Membership and resolver checks as indexed `EXISTS` queries. Groups via
one join. Implement `HasPermissions` by sending all checks in one query, for
example joining the grants table against a `VALUES` list of
(principal kind, principal id, action, resource type, resource id, scope id),
then mapping rows back to check positions.

**In-memory.** Index grants in maps keyed by the fields you match on; see
`examples/advanced/store`, which implements `BatchPermissionReader` with one
pass over its grants.

**Remote service.** Implement `HasPermissions` with the service's bulk
endpoint, respect `ctx` deadlines, and translate transport errors into errors
(not `false`).

## Observability

canery has no telemetry dependency. Measure in your adapters:

- reader invocation counts per check / per batch,
- `HasPermissions` batch sizes,
- query or RPC count per authorization call,
- latency of membership, group, permission, and resolver lookups.

Wrapping each reader in a small decorator that records these is usually enough.
The `BenchmarkGroupEvaluation` benchmark reports `reader-calls/op` and shows
the effect of the batch capability:

```text
groups=100/single   101 reader-calls/op
groups=100/batch      2 reader-calls/op
```

## Concurrency

See the package documentation for the full contract: `Engine` and
`PolicyAuthorizer` are safe to share; your readers must be too.
