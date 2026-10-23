package canery

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
