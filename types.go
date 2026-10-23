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
//
// A Request asks one question: can Subject perform Action on Resource within
// Scope? Context optionally carries application-defined values for Policies
// and custom evaluators; the default Engine never interprets it.
type Request struct {
	Subject  Subject
	Action   Action
	Resource ResourceRef
	Scope    ScopeRef
	Context  AuthorizationContext
}

// DecisionSource identifies which evaluation path produced a Decision.
//
// The default Engine only produces DecisionSourceDirect, DecisionSourceGroup,
// and DecisionSourceNone. The underlying string values are stable and safe to
// log or serialize.
type DecisionSource string

const (
	// DecisionSourceDirect indicates that a direct subject permission matched.
	DecisionSourceDirect DecisionSource = "direct"
	// DecisionSourceGroup indicates that a group-derived permission matched.
	DecisionSourceGroup DecisionSource = "group"
	// DecisionSourceNone indicates that no permission source produced an allow.
	DecisionSourceNone DecisionSource = "none"
)

// String returns the stable textual representation of the source.
func (s DecisionSource) String() string {
	return string(s)
}

// Decision is the explicit authorization outcome for a single request.
//
// Allowed is the only field that carries authorization meaning. Reason and
// Source explain how the outcome was reached without exposing backend-specific
// storage details. Principal identifies the principal whose permission
// produced an allow; it is left zero for denials. A Decision never carries
// arbitrary diagnostic payloads or AuthorizationContext values.
type Decision struct {
	Allowed bool
	Reason  string
	Source  DecisionSource
	// Principal is the principal whose permission matched: the requesting
	// subject for DecisionSourceDirect, or the first matching group in
	// evaluation order for DecisionSourceGroup. Other groups may also hold the
	// permission; the reported group is the one that produced the match, not
	// necessarily the only one.
	Principal PrincipalRef
}

// EffectivePrincipal returns the principal that produced an allow decision and
// whether one is known. It reports false for denials and for decisions whose
// producer did not attribute a principal.
func (d Decision) EffectivePrincipal() (PrincipalRef, bool) {
	if !d.Allowed || d.Principal.Kind == "" {
		return PrincipalRef{}, false
	}
	return d.Principal, true
}

// Trace captures high-level evaluation steps for a single authorization check.
//
// It is intended for debugging and inspection only. Steps only reference
// identifiers already present in the authorization model (principals and
// decision sources); the trace never includes AuthorizationContext values,
// reader error messages, or storage-specific details. See TraceStep for the
// step vocabulary.
type Trace struct {
	Steps []TraceStep
}

// TraceStep records one high-level step in an authorization evaluation.
//
// Name is one of the TraceStep* constants and Result one of the TraceResult*
// constants. Principal is set on permission steps to the principal that was
// evaluated (or matched). Source is set only on steps that determined a
// decision; the last step with a non-empty Source always matches the returned
// Decision.Source.
type TraceStep struct {
	Name      string
	Result    string
	Principal PrincipalRef
	Source    DecisionSource
}

// Trace step names recorded by Engine and PolicyAuthorizer.
const (
	TraceStepRequestValidated      = "request_validated"
	TraceStepResourceScopeVerified = "resource_scope_verified"
	TraceStepMembershipConfirmed   = "membership_confirmed"
	TraceStepDirectChecked         = "direct_permission_checked"
	TraceStepGroupsResolved        = "groups_resolved"
	TraceStepGroupChecked          = "group_permissions_checked"
	TraceStepPolicyEvaluated       = "policy_evaluated"
)

// Trace step results. TraceResultError always means evaluation failed and an
// error was returned; it is never used for an ordinary denial.
const (
	TraceResultPassed     = "passed"
	TraceResultFailed     = "failed"
	TraceResultDenied     = "denied"
	TraceResultSkipped    = "skipped"
	TraceResultMatched    = "matched"
	TraceResultNotMatched = "not_matched"
	TraceResultResolved   = "resolved"
	TraceResultNone       = "none"
	TraceResultDelegated  = "delegated"
	TraceResultHandled    = "handled"
	TraceResultOverridden = "overridden"
	TraceResultError      = "error"
)

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
