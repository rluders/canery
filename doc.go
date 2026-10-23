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
// intended to remain broad enough for alternate evaluators later. Single
// checks and batches (BatchCheck, BatchCheckResults) share that pipeline;
// batches deduplicate identical reader lookups and use BatchPermissionReader
// when a permission reader provides it.
//
// Decisions explain themselves through Decision.Source, Decision.Reason, and
// Decision.Principal; CheckTrace returns a structured Trace. Reader failures
// are reported as EvaluationError and are never converted into denials.
// Request.Context carries an opaque AuthorizationContext for Policies; the
// Engine never interprets it and it never appears in decisions, traces, or
// errors.
//
// Detailed contracts live in the repository's docs directory: the
// authorization model and invariants, Policy semantics, batch semantics,
// diagnostics, adapter guidance, and Architecture Decision Records.
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
// Request, Decision, Trace, and MultiActionResult are plain values owned by
// the caller once returned. canery never retains or mutates them after a call
// returns, and it never reads a Trace back, so callers may modify them freely.
// Request values passed into a call are copied; readers, resolvers, and
// Policies receive their own copies and must treat them as read-only inputs.
//
// Engine.BatchCheck, Engine.BatchCheckResults, and MultiActionBuilder.Check
// evaluate their items sequentially on the calling goroutine, so canery itself
// never issues concurrent reader calls for a single batch. Lookups are
// memoised only for the duration of one call.
//
// These guarantees cover canery's own types only. Implementations of
// MembershipReader, GroupReader, PermissionReader, ResourceScopeResolver, the
// Authorizer wrapped by NewPolicyAuthorizer, and every Policy must be safe for
// concurrent use when the evaluator is shared, because canery calls them from
// whichever goroutines call it. Policies should be stateless or guard their own
// state, and must not rely on sequential invocation, request affinity, or
// goroutine affinity. A Policy must not call next concurrently from multiple
// goroutines within a single evaluation. Values handed to NewEngine and
// NewPolicyAuthorizer must not be mutated while evaluation is in progress.
// canery does not serialize access to readers or resolvers; storage-backed
// implementations provide whatever synchronization they need.
package canery
