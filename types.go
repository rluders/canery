package canery

// Action identifies the operation being requested on a resource.
type Action string

// Subject identifies the actor asking to perform an action.
type Subject struct {
	Type string
	ID   string
}

// ActorRef is a readability alias for Subject.
//
// Subject remains the core request field name for backward compatibility.
type ActorRef = Subject

// ResourceRef identifies the target resource for a check.
//
// ID may be left empty for create-style checks where the resource does not yet
// exist and authorization is based on resource type and scope alone.
type ResourceRef struct {
	Type string
	ID   string
}

// ScopeRef identifies the boundary in which the request is evaluated.
//
// The current API models one explicit scope per request. The engine keeps its
// internals structured so that this boundary can evolve toward hierarchical
// scope evaluation later without changing the request shape immediately.
type ScopeRef struct {
	Type string
	ID   string
}

// GroupRef identifies a derived or persisted group that can carry permissions.
type GroupRef struct {
	Type string
	ID   string
}

// PrincipalKind distinguishes direct subject checks from group-based checks.
type PrincipalKind string

const (
	// PrincipalKindSubject represents a direct subject principal.
	PrincipalKindSubject PrincipalKind = "subject"
	// PrincipalKindGroup represents a group principal.
	PrincipalKindGroup PrincipalKind = "group"
)

// PrincipalRef identifies the principal used by a PermissionReader.
//
// Principals can represent either the subject directly or a group resolved for
// that subject within a scope.
type PrincipalRef struct {
	Kind PrincipalKind
	Type string
	ID   string
}

// Request is the low-level authorization primitive evaluated by an Authorizer.
type Request struct {
	Subject  Subject
	Action   Action
	Resource ResourceRef
	Scope    ScopeRef
}

// Decision is the explicit authorization outcome for a single request.
//
// Reason and Source provide generic explanation metadata about how the
// decision was reached without exposing backend-specific storage details.
type Decision struct {
	Allowed bool
	Reason  string
	Source  string
}

// Trace captures high-level evaluation steps for a single authorization check.
//
// It is intended for debugging and inspection only. The trace stays generic
// and does not expose storage-specific details.
type Trace struct {
	Steps []TraceStep
}

// TraceStep records one high-level step in an authorization evaluation.
type TraceStep struct {
	Name   string
	Result string
}

// SubjectPrincipal converts a subject into a principal for direct permission
// evaluation.
func SubjectPrincipal(subject Subject) PrincipalRef {
	return PrincipalRef{
		Kind: PrincipalKindSubject,
		Type: subject.Type,
		ID:   subject.ID,
	}
}

// GroupPrincipal converts a group into a principal for group-based permission
// evaluation.
func GroupPrincipal(group GroupRef) PrincipalRef {
	return PrincipalRef{
		Kind: PrincipalKindGroup,
		Type: group.Type,
		ID:   group.ID,
	}
}
