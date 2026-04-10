package canery

import "context"

// scopeContext is the engine's internal representation of the request scope.
//
// Today it is just a thin wrapper around one ScopeRef. Keeping it separate from
// the public request model makes it easier to evolve toward hierarchical scopes
// later, where a single authorization check may need the current scope plus
// ancestor context such as organization -> workspace -> project.
type scopeContext struct {
	target ScopeRef
}

func newScopeContext(request Request) scopeContext {
	return scopeContext{
		target: request.Scope,
	}
}

func (s scopeContext) targetScope() ScopeRef {
	return s.target
}

// requiresResourceScopeCheck reports whether the request targets a concrete
// resource instance that must be resolved inside the current scope context.
//
// In a future hierarchical model this check may need to validate resource
// membership against an ancestor chain rather than only the immediate target
// scope.
func (s scopeContext) requiresResourceScopeCheck(resource ResourceRef) bool {
	return resource.ID != ""
}

func (e *Engine) ensureResourceInScope(ctx context.Context, scope scopeContext, resource ResourceRef) (bool, error) {
	if !scope.requiresResourceScopeCheck(resource) {
		return true, nil
	}
	if e.resolver == nil {
		return false, ErrMissingScopeResolver
	}
	return e.resolver.ResourceInScope(ctx, resource, scope.targetScope())
}

func (e *Engine) hasSubjectMembership(ctx context.Context, subject Subject, scope scopeContext) (bool, error) {
	return e.memberships.HasMembership(ctx, subject, scope.targetScope())
}

func (e *Engine) groupsForSubject(ctx context.Context, subject Subject, scope scopeContext) ([]GroupRef, error) {
	return e.groups.GroupsForSubject(ctx, subject, scope.targetScope())
}
