# Architecture Decision Records

Short records of decisions that constrain canery's architecture. Format:
**Status · Context · Decision · Consequences · Alternatives**. Statuses:
`Accepted` (reflects the implementation), `Proposed` (direction, not yet
implemented). Changing an `Accepted` decision requires a new ADR that
supersedes it.

| # | Title | Status |
| --- | --- | --- |
| [0001](0001-authorization-kernel.md) | canery is an authorization kernel | Accepted |
| [0002](0002-storage-agnostic-core.md) | Storage-agnostic core | Accepted |
| [0003](0003-core-request-model.md) | Subject · Action · Resource · Scope request model | Accepted |
| [0004](0004-default-evaluation-pipeline.md) | Default evaluation pipeline | Accepted |
| [0005](0005-membership-is-a-prerequisite.md) | Membership is a prerequisite, not a grant | Accepted |
| [0006](0006-policies-as-middleware.md) | Policies are middleware, not a policy language | Accepted |
| [0007](0007-no-built-in-precedence.md) | No built-in deny/allow precedence | Accepted |
| [0008](0008-batch-architecture.md) | Batch evaluation with optional batch readers | Accepted |
| [0009](0009-facts-vs-decisions.md) | Adapters provide facts; evaluators decide | Accepted |
| [0010](0010-opaque-authorization-context.md) | Opaque authorization context and safe diagnostics | Accepted |
| [0011](0011-hierarchy-and-relationships.md) | Hierarchical scopes and relationships as optional capabilities | Proposed |

## Discrepancies found while writing these ADRs

- Policies run before the base `Engine` validates the request (ADR 0006).
  Documented, not changed.
- `PolicyAuthorizer.CheckTrace` keeps only the last `next` call's steps when a
  Policy delegates more than once. Documented.
- The `examples/advanced` store treats type-level grants as covering all
  instances. This is reader-owned behaviour (ADR 0009), now documented in
  `authorization-model.md`.
- Before v2, `BatchCheck` returned raw reader errors and evaluated items one by
  one; the batch ADR (0008) describes the v2 behaviour.
