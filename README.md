<p align="center">
  <img src="docs/assets/canery-logo.png" alt="canery logo">
</p>

[![CI](https://github.com/rluders/canery/actions/workflows/ci.yml/badge.svg)](https://github.com/rluders/canery/actions/workflows/ci.yml)
![Go](https://img.shields.io/badge/go-1.24+-00ADD8?logo=go&logoColor=white)
[![Go Reference](https://pkg.go.dev/badge/github.com/rluders/canery/v2.svg)](https://pkg.go.dev/github.com/rluders/canery/v2)
[![Go Report Card](https://goreportcard.com/badge/github.com/rluders/canery)](https://goreportcard.com/report/github.com/rluders/canery)
[![GitHub release](https://img.shields.io/github/v/release/rluders/canery?sort=semver)](https://github.com/rluders/canery/releases)
[![codecov](https://codecov.io/gh/rluders/canery/graph/badge.svg)](https://codecov.io/gh/rluders/canery)
[![License](https://img.shields.io/github/license/rluders/canery)](LICENSE)
![stdlib-only](https://img.shields.io/badge/dependencies-stdlib--only-blue)

## 🧠 What is canery?

`canery` is a **minimal, generic authorization kernel** built around a simple and explicit model:

* **subject**
* **action**
* **resource**
* **scope**

It focuses purely on **authorization evaluation**, not storage or product semantics.

> It answers: *“Can X do Y on Z within W?”*

canery evaluates authorization; it does not own the systems that provide identity, resources, or authorization data. See [docs/architecture.md](docs/architecture.md).

## ⚡ Why use it?

Most authorization solutions are either:

* too **opinionated** (roles, orgs, workspaces baked in)
* too **heavy** (full IAM platforms)

`canery` stays in the middle:

* ✅ explicit and predictable request model
* ✅ no framework or product lock-in
* ✅ pluggable data sources
* ✅ small, composable core
* ✅ easy to test

## 🚫 What it does NOT do

* ❌ does not store permissions
* ❌ does not define roles or org models
* ❌ does not include a database layer
* ❌ does not enforce product semantics

You bring those via adapters.

## 📥 Install

```sh
go get github.com/rluders/canery/v2
```

```go
import "github.com/rluders/canery/v2" // package canery
```

## 🚀 Quick Example

```go
ok, err := authorizer.Check(ctx, canery.Request{
  Subject:  canery.Actor("user", userID),
  Action:   canery.Action("delete"),
  Resource: canery.Resource("document", documentID),
  Scope:    canery.Scope("project", projectID),
})
```

## 🔍 Decision Details

```go
decision, err := authorizer.CheckDecision(ctx, canery.Request{
  Subject:  canery.Actor("user", userID),
  Action:   canery.Action("delete"),
  Resource: canery.Resource("document", documentID),
  Scope:    canery.Scope("project", projectID),
})

if decision.Allowed {
  fmt.Println(decision.Source) // canery.DecisionSourceDirect or canery.DecisionSourceGroup
  principal, _ := decision.EffectivePrincipal()
  fmt.Println(principal) // the subject, or the first matching group
} else {
  fmt.Println(decision.Source) // "none"
  fmt.Println(decision.Reason) // e.g. "no matching permission"
}
```

## 🧱 Fluent API

```go
ok, err := authorizer.
  For(canery.Actor("user", userID)).
  Can(canery.Action("delete")).
  Target(canery.Resource("document", documentID)).
  In(canery.Scope("project", projectID)).
  Check(ctx)
```

## 🔁 Multiple Actions

```go
result, err := authorizer.
  For(canery.Actor("user", userID)).
  CanMany(
    canery.Action("view"),
    canery.Action("update"),
    canery.Action("delete"),
  ).
  Target(canery.Resource("document", documentID)).
  In(canery.Scope("project", projectID)).
  Check(ctx)

canUpdate, _ := result.Allowed(canery.Action("update"))
```

## 📦 Batch Checks

Batches are evaluated together: identical membership, group, resource-scope, and permission lookups run once, and permission readers that implement the optional `BatchPermissionReader` answer many checks in one call.

```go
decisions, err := engine.BatchCheck(ctx, []canery.Request{
  {
    Subject:  canery.Actor("user", userID),
    Action:   canery.Action("view"),
    Resource: canery.Resource("document", documentID),
    Scope:    canery.Scope("project", projectID),
  },
  {
    Subject:  canery.Actor("user", userID),
    Action:   canery.Action("delete"),
    Resource: canery.Resource("document", documentID),
    Scope:    canery.Scope("project", projectID),
  },
})
// All-or-nothing: on failure, err is a canery.BatchCheckError for the lowest failing index.

// Keep partial results instead:
for _, result := range engine.BatchCheckResults(ctx, requests) {
  if result.Err != nil { /* could not be evaluated */ continue }
  fmt.Println(result.Decision.Allowed)
}
```

See [docs/batch.md](docs/batch.md) for the full failure contract.

## 🧪 Debugging (Trace)

```go
decision, trace, err := authorizer.CheckTrace(ctx, canery.Request{
  Subject:  canery.Actor("user", userID),
  Action:   canery.Action("delete"),
  Resource: canery.Resource("document", documentID),
  Scope:    canery.Scope("project", projectID),
})

for _, step := range trace.Steps {
  fmt.Println(step.Name, step.Result, step.Principal, step.Source)
}

_ = decision
```

Traces never include context values or reader error details. Reader failures surface as `canery.EvaluationError` (inspect the cause with `errors.Is` / `errors.As`). See [docs/diagnostics.md](docs/diagnostics.md).

## ⚙️ Default Evaluation Strategy

The default `Engine` uses a **membership-first evaluation flow**:

```mermaid
flowchart TD
  A[Check request] --> B[Validate request]
  B -->|invalid| X[Return validation error]
  B --> C{Resource ID present?}
  C -- yes --> D[Verify resource belongs to scope]
  D -- no --> L[Deny]
  C -- no --> E[Verify subject belongs to scope]
  D --> E
  E -- no --> L
  E --> F[Check direct subject permission]
  F --> G{Allowed?}
  G -- yes --> H[Allow]
  G -- no --> I[Resolve groups in scope]
  I --> J[Check group permissions]
  J --> K{Any allowed?}
  K -- yes --> H
  K -- no --> L[Deny]
```

This is just the default engine — other evaluation strategies can be implemented.

## 🧩 Architecture

```mermaid
flowchart LR
  App["Application"] -->|"For(subject)...Check(ctx)"| B["Builder"]
  App -->|"Check(ctx, Request)"| A
  B -->|"builds Request"| A
  subgraph Authorizers["Authorizer implementations"]
    A{{"Authorizer interface"}}
    PA["PolicyAuthorizer"]
    E["Engine"]
  end
  A -.->|"implemented by"| PA
  A -.->|"implemented by"| E
  PA -->|"Policies, then next"| E
  subgraph Readers["Readers (your adapters)"]
    M["MembershipReader"]
    G["GroupReader"]
    PR["PermissionReader / BatchPermissionReader"]
    S["ResourceScopeResolver"]
  end
  E --> M
  E --> G
  E --> PR
  E --> S
```

## 🔌 Extensibility

All data access is delegated to interfaces:

* `MembershipReader`
* `GroupReader`
* `PermissionReader`
* `ResourceScopeResolver`
* `BatchPermissionReader` (optional)

This keeps the core:

* stateless
* storage-agnostic
* easy to test

## 🧠 Policies (Optional)

```go
authorizer := canery.NewPolicyAuthorizer(
  baseAuthorizer,
  canery.ForResourceType("document", canery.PolicyFunc(func(ctx context.Context, request canery.Request, next canery.DecisionEvaluator) (canery.Decision, error) {
    if request.Action == canery.Action("archive") {
      return canery.Decision{
        Allowed: false,
        Reason:  "policy matched",
        Source:  canery.DecisionSourceNone,
      }, nil
    }
    return next.CheckDecision(ctx, request)
  })),
)
```

Policies are:

* middleware: delegate, short-circuit, or transform decisions
* matcher-based and run in configured order
* optional

There is **no** built-in deny/allow precedence. See [docs/policies.md](docs/policies.md).

### Authorization context

Policies can read application-defined values without framework types:

```go
type authMethodKey struct{}

ok, err := authorizer.
  For(canery.Actor("user", userID)).
  Can(canery.Action("delete")).
  Target(canery.Resource("document", documentID)).
  In(canery.Scope("project", projectID)).
  WithValue(authMethodKey{}, "mfa").
  Check(ctx)

// inside a Policy:
method, _ := canery.ContextValue[string](request.Context, authMethodKey{})
```

The default engine never interprets context values, and they never appear in decisions, traces, or errors.

## 🧱 Adapter Pattern (Recommended)

Keep your application semantics outside the core:

```go
package projectauthz

import "github.com/rluders/canery/v2"

const EditDocument = canery.Action("edit")

func User(id string) canery.Subject {
  return canery.Actor("user", id)
}

func ProjectScope(id string) canery.ScopeRef {
  return canery.Scope("project", id)
}

func Document(id string) canery.ResourceRef {
  return canery.Resource("document", id)
}
```

Usage:

```go
ok, err := authorizer.
  For(projectauthz.User(userID)).
  Can(projectauthz.EditDocument).
  Target(projectauthz.Document(documentID)).
  In(projectauthz.ProjectScope(projectID)).
  Check(ctx)
```

## 🔒 Concurrency

* `Engine` and `PolicyAuthorizer` are immutable after construction and safe to share across goroutines.
* `Builder` and `MultiActionBuilder` are value types; every method returns a copy, so they are safe to reuse and branch.
* `BatchCheck` and multi-action checks run sequentially; canery does not call readers concurrently on its own.
* `Request`, `Decision`, and `Trace` are plain values owned by the caller; canery never retains or mutates them after returning.
* Readers, resolvers, wrapped authorizers, and Policies must be concurrency-safe themselves when shared. canery does not serialize access to them. Policies should be stateless and must not assume sequential or goroutine-affine invocation.

## 📚 Documentation

* [Authorization model & invariants](docs/authorization-model.md)
* [Architecture](docs/architecture.md) · [Policies](docs/policies.md) · [Batch](docs/batch.md)
* [Diagnostics](docs/diagnostics.md) · [Adapters & performance](docs/adapters.md)
* [Architecture Decision Records](docs/adr/README.md) · [Migrating to v2](docs/migrating-to-v2.md)

## 📂 Examples

* `examples/simple` → minimal core usage
* `examples/advanced` → realistic app setup (users, projects, roles, etc.)

Each example is a standalone Go module.

## ⚠️ Validation Rules

Checked in this order; the first failure is returned:

| `Path()` | Rule |
| --- | --- |
| `subject.type`, `subject.id` | non-blank |
| `action` | non-blank |
| `resource.type` | non-blank (`ID` optional: empty means a type-level check) |
| `scope.type`, `scope.id` | non-blank |

Invalid requests return a `ValidationError{Field, Property, Code}`
(`errors.Is(err, canery.ErrInvalidRequest)` works from every entry point).

## 🎯 When to use canery

Use it when you want:

* explicit, testable authorization logic
* full control over your data model
* no framework constraints

Avoid it if you need:

* a complete IAM system
* UI, policy language, or managed service
* out-of-the-box RBAC/ABAC models

## 📄 License

MIT
