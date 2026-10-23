# canery documentation

| Document | Covers |
| --- | --- |
| [architecture.md](architecture.md) | What canery is (an authorization kernel), layer responsibilities, contribution guardrails |
| [authorization-model.md](authorization-model.md) | Request model, resource type vs instance, membership, permissions, decisions, validation, invariants |
| [policies.md](policies.md) | Policy semantics: ordering, short-circuiting, transformation, errors, tracing |
| [batch.md](batch.md) | Batch evaluation, deduplication, batch readers, failure and partial-result semantics |
| [diagnostics.md](diagnostics.md) | Trace contract, decision attribution, error contract, data-exposure boundaries |
| [adapters.md](adapters.md) | Reader/resolver contracts, invocation patterns, performance and caching guidance |
| [design/](design/) | Proposed designs: hierarchical scopes, resource context, relationships |
| [adr/](adr/) | Architecture Decision Records |

Concurrency guarantees are documented in the [package documentation](https://pkg.go.dev/github.com/rluders/canery/v2).
