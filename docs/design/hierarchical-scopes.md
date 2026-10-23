# Design: hierarchical scope resolution

Status: **Proposed** (no runtime behaviour yet). Tracks rluders/canery#12.

## Baseline

Today a request has one `ScopeRef`, evaluated as an independent boundary. The
`Engine` wraps it in an internal `scopeContext` that only holds the target
scope. Every reader receives that single scope.

```go
Scope("project", "123") // behaves exactly as it does today, forever
```

## Goal

Let a future evaluator *know* the ancestry of a scope without deciding what
that ancestry *means* for permissions:

```text
organization:1      ← root
  └─ workspace:42
       └─ project:123  ← requested scope
```

## Proposed abstraction

An optional resolver capability, detected by type assertion like
`BatchPermissionReader`:

```go
// ScopeHierarchyResolver reports a scope's ancestors, nearest first.
// A root or independent scope returns an empty slice.
type ScopeHierarchyResolver interface {
    ScopeAncestors(ctx context.Context, scope ScopeRef) ([]ScopeRef, error)
}
```

and an evolution of the internal `scopeContext`:

```go
type scopeContext struct {
    target    ScopeRef   // requested scope (unchanged)
    ancestors []ScopeRef // nearest first; nil when not resolved
    resolved  bool       // distinguishes "not asked" from "root"
}
```

The chain is resolved **lazily**, only by an evaluator stage that needs it, so
single-level applications never pay for it.

| Case | `ancestors` |
| --- | --- |
| Root scope | `[]` (resolved) |
| Single-level / independent scope | `[]` (resolved) or never resolved |
| Multi-level | `[workspace:42, organization:1]` |

## Responsibilities

- **Scope resolution** (the new resolver): structural facts — "what contains
  this scope?". Storage-agnostic; may be backed by a table, a path column, or a
  remote directory.
- **Resource resolution** (`ResourceScopeResolver`, see
  [resource-context.md](resource-context.md)): "does this resource belong to
  this scope?" — unchanged.
- **Permission evaluation**: unchanged. Ancestry does **not** imply
  inheritance. Whether an organization-level grant applies to a project is a
  separate, explicit decision for a future evaluator or Policy (and an ADR).

## Compatibility

- `ScopeRef`, `MembershipReader`, `GroupReader`, `PermissionReader`, and
  `ResourceScopeResolver` stay as they are.
- Adapters opt in by implementing the extra interface.
- The default `Engine` pipeline does not change when the interface exists; it
  will only be consumed by an evaluator that defines inheritance semantics.

## Example (illustrative)

```go
type orgTree struct{ parent map[canery.ScopeRef]canery.ScopeRef }

func (t orgTree) ScopeAncestors(_ context.Context, scope canery.ScopeRef) ([]canery.ScopeRef, error) {
    var chain []canery.ScopeRef
    for current, ok := t.parent[scope]; ok; current, ok = t.parent[current] {
        chain = append(chain, current)
    }
    return chain, nil
}
```

## Open questions

- Should membership be checked against the requested scope only, or any ancestor? (Semantics decision — out of scope here.)
- Batch variant (`ScopeAncestorsBatch`) once a consumer exists.
