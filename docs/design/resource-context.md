# Design: from resource ∈ scope to resource context

Status: **Proposed** (no runtime behaviour yet). Tracks rluders/canery#13.

## Current responsibility

```go
ResourceInScope(ctx, resource, scope) (bool, error)
```

`ResourceScopeResolver` answers one containment fact for instance requests:
`true` → continue, `false` → deny "resource outside scope", error →
`EvaluationError` (evaluation failure, never a denial).

**Limitation:** a boolean cannot say *where* a resource lives. With
hierarchical scopes, a request for `document:123` in `workspace:42` needs to
know the document sits in `project:456`, which sits in `workspace:42`.

## Proposed additive capability

Keep `ResourceScopeResolver` as the minimal contract. Add an optional
interface for adapters that can supply structure:

```go
// ResourceContext is a set of structural facts about one resource.
type ResourceContext struct {
    Resource       ResourceRef
    ContainedIn    ScopeRef   // immediate scope; zero if unknown
    Ancestors      []ScopeRef // ancestors of ContainedIn, nearest first
}

// ResourceContextResolver resolves structural facts about a resource.
// ok=false means the resource does not exist (or is not visible) and is an
// authorization-relevant fact; an error means resolution failed.
type ResourceContextResolver interface {
    ResourceContext(ctx context.Context, resource ResourceRef) (ResourceContext, bool, error)
}
```

Distinctions the type keeps explicit:

```text
Requested scope:          workspace:42     (Request.Scope)
Resource:                 document:123     (ResourceContext.Resource)
Resource's immediate:     project:456      (ContainedIn)
Resource's ancestors:     workspace:42, organization:1
```

A future evaluator derives containment from it
(`requested == ContainedIn || requested ∈ Ancestors`) and can fall back to
`ResourceInScope` when only the minimal capability exists.

## Rules

- Resolvers provide facts; they never decide whether the subject is allowed.
- "Not in the requested scope" stays a denial; "could not resolve" stays an error.
- No inheritance or propagation is implied by `Ancestors`.

## Performance

- Simple containment must not require loading the resource or its hierarchy:
  `ResourceInScope` remains the fast path.
- Richer context is fetched only by evaluators that need it, never eagerly.
- Batch variants can follow the `BatchPermissionReader` pattern (positional
  results, per-item errors) when a consumer exists.

## Compatibility

Existing resolvers keep working unchanged; the default `Engine` keeps using
`ResourceInScope`.
