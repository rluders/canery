package canery

import "context"

// MembershipReader reports whether a subject belongs to a given scope.
//
// The public API stays single-scope for now. Implementations can already treat
// that scope as the current boundary in a broader hierarchy if they need to
// prepare for future nesting support.
type MembershipReader interface {
	HasMembership(ctx context.Context, subject Subject, scope ScopeRef) (bool, error)
}

// GroupReader resolves the groups a subject belongs to within a given scope.
//
// Today the scope is a single ScopeRef. A future hierarchical model may use
// that scope as the leaf context that determines which inherited or local
// groups should be visible.
type GroupReader interface {
	GroupsForSubject(ctx context.Context, subject Subject, scope ScopeRef) ([]GroupRef, error)
}

// PermissionReader reports whether a principal is allowed to perform a request.
//
// The principal may represent either the subject directly or a group previously
// resolved for that subject.
type PermissionReader interface {
	HasPermission(ctx context.Context, principal PrincipalRef, request Request) (bool, error)
}

// ResourceScopeResolver verifies that a resource belongs to the provided scope.
//
// It is only required when a request targets a concrete resource ID.
// Implementations should treat the provided scope as the request's current
// boundary. That leaves room for future evolution toward ancestor-aware scope
// resolution without changing the request model today.
type ResourceScopeResolver interface {
	ResourceInScope(ctx context.Context, resource ResourceRef, scope ScopeRef) (bool, error)
}
