// Package canery provides a small, reusable authorization core built around
// generic subjects, actions, resources, and scopes.
//
// The primitive API is request-based: callers build a Request and ask an
// Authorizer to evaluate it. A fluent Builder is also provided for readability,
// but it is only a thin layer over the same Request model.
// Small helper constructors such as Actor, Resource, and Scope are also
// available for ergonomics, but they remain thin wrappers over the exported
// structs. User is available as a convenience helper, not as the preferred
// generic entrypoint.
//
// Authorization state is not stored inside the package. Evaluation is delegated
// to pluggable readers and resolvers so the package can stay storage-agnostic
// and reusable across projects.
//
// canery is not a full IAM system. It does not model tenants, roles,
// directories, or product semantics by itself. Those concerns belong in
// adapters and backends built on top of the core package.
//
// Engine is the default shipped evaluator. It applies one membership-scoped
// evaluation pipeline over the shared request model, but the model itself is
// intended to remain broad enough for alternate evaluators later.
//
// # Concurrency and thread safety
//
// Engine and PolicyAuthorizer hold no mutable state after construction and are
// safe for concurrent use by multiple goroutines, so a single instance can be
// shared across HTTP requests, workers, and batch evaluations. Each call
// evaluates independently; no decision or trace is cached between calls.
//
// Builder and MultiActionBuilder are value types. Every fluent method returns
// a modified copy and never mutates the receiver, so a partially configured
// builder can be reused or branched from several goroutines.
//
// Engine.BatchCheck and MultiActionBuilder.Check evaluate their items
// sequentially on the calling goroutine, so canery itself never issues
// concurrent reader calls for a single batch.
//
// These guarantees cover canery's own types only. Implementations of
// MembershipReader, GroupReader, PermissionReader, ResourceScopeResolver, the
// Authorizer wrapped by NewPolicyAuthorizer, and every Policy must be safe for
// concurrent use when the evaluator is shared, because canery calls them from
// whichever goroutines call it. Policies should be stateless or guard their own
// state. Values handed to NewEngine and NewPolicyAuthorizer must not be
// mutated while evaluation is in progress.
package canery
