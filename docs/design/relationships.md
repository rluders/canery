# Design: preparing for relationship-based authorization

Status: **Proposed** (no runtime behaviour yet). Tracks rluders/canery#14.

## Limitation today

canery answers "does principal P hold permission for request R?", where P is
the subject or one of its groups. It cannot express facts such as:

```text
user:123        --owner-->     document:456
user:123        --editor-->    project:789
group:456       --member-->    workspace:42
service:billing --operator-->  account:123
```

## Minimal relationship model

```go
// Entity is any authorization entity: subject, group, resource, or scope.
type Entity struct {
    Type string
    ID   string
}

// Relationship is one structural fact: Subject --Relation--> Object.
type Relationship struct {
    Subject  Entity
    Relation string
    Object   Entity
    Scope    ScopeRef // optional; zero when the fact is global
}
```

A relationship is a **fact**, distinct from:

- a **permission** (which the engine treats as a grant for an action), and
- **membership** (the scope prerequisite of the default pipeline).

`owner` does not mean "can edit" until an evaluator says so.

## Extension point

```go
// RelationshipReader reports whether relationship facts hold.
type RelationshipReader interface {
    HasRelationship(ctx context.Context, relationship Relationship) (bool, error)
}

// BatchRelationshipReader is the optional bulk variant.
type BatchRelationshipReader interface {
    RelationshipReader
    HasRelationships(ctx context.Context, relationships []Relationship) ([]PermissionResult, error)
}
```

Optional, storage-agnostic, context-aware, and composable: a relationship-aware
evaluator (or Policy) would hold a `RelationshipReader` next to the existing
readers. Existing adapters implement nothing new.

## How an evaluator would consume it

```text
Relationship layer  → structural facts   ("user:123 owner document:456")
Authorization layer → semantics          ("owner ⇒ edit")
```

```go
// Illustrative Policy: owners may edit, otherwise defer to the Engine.
func ownersCanEdit(rel RelationshipReader) canery.Policy {
    return canery.PolicyFunc(func(ctx context.Context, r canery.Request, next canery.DecisionEvaluator) (canery.Decision, error) {
        owner, err := rel.HasRelationship(ctx, Relationship{
            Subject:  Entity(r.Subject),
            Relation: "owner",
            Object:   Entity(r.Resource),
        })
        if err != nil {
            return canery.Decision{}, err
        }
        if owner && r.Action == "edit" {
            return canery.Decision{Allowed: true, Reason: "owner", Source: canery.DecisionSource("relationship")}, nil
        }
        return next.CheckDecision(ctx, r)
    })
}
```

This already works today with an application-defined reader, which is why the
core does not need relationship types until a shared evaluator exists.

## Interactions

| Concept | Interaction |
| --- | --- |
| Subject | Usual origin of a relationship. |
| Group | May be subject or object (`group --member--> workspace`). |
| Resource | Usual target. |
| Scope | May qualify a relationship or help establish scope context. |
| Permission | Not implied; the evaluator maps relations to actions. |

## Batch

Batch evaluation can collect `Relationship` keys per stage exactly like
`PermissionCheck` and send them through `BatchRelationshipReader`.

## Non-decisions

No Zanzibar-style rewrites, no relation inheritance, no new `DecisionSource`.
Those would be separate ADRs.
