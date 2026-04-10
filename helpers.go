package canery

// User returns a Subject for a human user actor.
//
// It is a convenience helper. Generic package usage should prefer Actor when a
// user-specific helper is not materially clearer.
func User(id string) Subject {
	return Actor("user", id)
}

// Actor returns a Subject for the given actor kind and identifier.
func Actor(kind string, id string) Subject {
	return Subject{
		Type: kind,
		ID:   id,
	}
}

// Resource returns a ResourceRef for the given resource kind and identifier.
func Resource(kind string, id string) ResourceRef {
	return ResourceRef{
		Type: kind,
		ID:   id,
	}
}

// Scope returns a ScopeRef for the given scope kind and identifier.
func Scope(kind string, id string) ScopeRef {
	return ScopeRef{
		Type: kind,
		ID:   id,
	}
}
