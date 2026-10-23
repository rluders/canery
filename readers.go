package canery

import "context"

// Readers and resolvers supply authorization facts; they never decide whether
// a request is allowed. The Engine combines their answers into a Decision.
//
// All reader methods may be called concurrently when an Engine is shared
// across goroutines, and may be called several times while evaluating one
// batch (canery deduplicates identical lookups within a single call to Check,
// BatchCheck, or BatchCheckResults, but never caches across calls). Return
// (false, nil) for an ordinary "no" and reserve errors for lookups that could
// not be completed. See docs/adapters.md for performance guidance.

// MembershipReader reports whether a subject belongs to a given scope.
//
// Membership is a prerequisite in the default Engine, never a grant: a member
// still needs a matching permission, and a non-member is denied before any
// permission is checked. The scope is the request's single ScopeRef.
type MembershipReader interface {
	HasMembership(ctx context.Context, subject Subject, scope ScopeRef) (bool, error)
}

// GroupReader resolves the groups a subject belongs to within a given scope.
//
// The returned order is the order in which group permissions are evaluated,
// and therefore decides which group is reported when several match. Return a
// stable order. The Engine does not modify the returned slice.
type GroupReader interface {
	GroupsForSubject(ctx context.Context, subject Subject, scope ScopeRef) ([]GroupRef, error)
}

// PermissionReader reports whether a principal holds the permission described
// by a request.
//
// The principal may represent either the subject directly or a group
// previously resolved for that subject. Matching semantics between resource
// types and resource instances (for example whether a type-level grant covers
// document:123) are owned by the implementation; the Engine passes the request
// through unchanged.
type PermissionReader interface {
	HasPermission(ctx context.Context, principal PrincipalRef, request Request) (bool, error)
}

// PermissionCheck is one principal/request pair evaluated by a
// BatchPermissionReader. It is comparable and can be used as a map key.
type PermissionCheck struct {
	Principal PrincipalRef
	Request   Request
}

// PermissionResult is the outcome of one PermissionCheck. Allowed=false with a
// nil Err is an ordinary "no match"; a non-nil Err means the check could not be
// evaluated.
type PermissionResult struct {
	Allowed bool
	Err     error
}

// BatchPermissionReader is an optional capability for PermissionReader
// implementations that can answer many permission checks in one operation,
// such as a single SQL query.
//
// The Engine detects it with a type assertion and uses it whenever a stage of
// evaluation has more than one distinct check; otherwise it calls
// HasPermission. Implementations must return exactly one result per check, in
// the same order. The Engine guarantees that checks within one call are
// distinct. A non-nil error fails every check in the call; per-check errors go
// in PermissionResult.Err. Results must match what HasPermission would return
// for the same check.
type BatchPermissionReader interface {
	PermissionReader
	HasPermissions(ctx context.Context, checks []PermissionCheck) ([]PermissionResult, error)
}

// ResourceScopeResolver verifies that a resource belongs to the provided scope.
//
// It is only called when a request targets a concrete resource ID. It answers
// a containment fact; (false, nil) means "not in this scope" and yields a
// denial, while an error means the relationship could not be resolved. See
// docs/design/resource-context.md for the planned evolution toward richer
// resource context.
type ResourceScopeResolver interface {
	ResourceInScope(ctx context.Context, resource ResourceRef, scope ScopeRef) (bool, error)
}
