package canery

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrInvalidRequest indicates that a request is missing required fields.
	ErrInvalidRequest = errors.New("canery: invalid request")
	// ErrMissingMembership indicates that the engine has no MembershipReader.
	ErrMissingMembership = errors.New("canery: missing membership reader")
	// ErrMissingGroupReader indicates that the engine has no GroupReader.
	ErrMissingGroupReader = errors.New("canery: missing group reader")
	// ErrMissingPermissions indicates that the engine has no PermissionReader.
	ErrMissingPermissions = errors.New("canery: missing permission reader")
	// ErrMissingScopeResolver indicates that the engine cannot validate a
	// concrete resource against a scope because no resolver was configured.
	ErrMissingScopeResolver = errors.New("canery: missing resource scope resolver")
)

const (
	// DecisionSourceDirect indicates that a direct subject permission matched.
	DecisionSourceDirect = "direct"
	// DecisionSourceGroup indicates that a group-derived permission matched.
	DecisionSourceGroup = "group"
	// DecisionSourceNone indicates that no permission source produced an allow.
	DecisionSourceNone = "none"
)

const (
	decisionReasonDirectPermission = "direct permission matched"
	decisionReasonGroupPermission  = "group permission matched"
	decisionReasonResourceScope    = "resource outside scope"
	decisionReasonMembership       = "subject not in scope"
	decisionReasonNoPermission     = "no matching permission"
)

const (
	traceStepRequestValidated      = "request_validated"
	traceStepResourceScopeVerified = "resource_scope_verified"
	traceStepMembershipConfirmed   = "membership_confirmed"
	traceStepDirectChecked         = "direct_permission_checked"
	traceStepGroupsResolved        = "groups_resolved"
	traceStepGroupChecked          = "group_permissions_checked"
)

// BatchCheckError identifies the request that caused a batch evaluation to
// fail.
type BatchCheckError struct {
	Index   int
	Request Request
	Err     error
}

func (e BatchCheckError) Error() string {
	return fmt.Sprintf("canery: batch item %d failed: %v", e.Index, e.Err)
}

func (e BatchCheckError) Unwrap() error {
	return e.Err
}

// ValidationError identifies a specific invalid request field with a stable
// field/code pair.
type ValidationError struct {
	Field string
	Code  string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("canery: invalid request: %s: %s", e.Field, e.Code)
}

func (e ValidationError) Unwrap() error {
	return ErrInvalidRequest
}

// Engine is the default Authorizer implementation backed by pluggable readers
// and a resource scope resolver.
//
// Engine intentionally implements one default evaluation strategy: a
// membership-scoped pipeline that validates the request, optionally checks a
// concrete resource against the requested scope, confirms scope membership, and
// then evaluates direct and group-based permissions.
//
// The request model and readers are broader than this one strategy. Future
// evaluators can reuse the same Request, Decision, and reader interfaces while
// applying a different evaluation order.
type Engine struct {
	memberships MembershipReader
	groups      GroupReader
	permissions PermissionReader
	resolver    ResourceScopeResolver
}

// NewEngine constructs an Engine that evaluates requests using the provided
// membership, group, permission, and resource-scope backends.
func NewEngine(memberships MembershipReader, groups GroupReader, permissions PermissionReader, resolver ResourceScopeResolver) *Engine {
	return &Engine{
		memberships: memberships,
		groups:      groups,
		permissions: permissions,
		resolver:    resolver,
	}
}

// For starts a fluent authorization request for the given subject.
func (e *Engine) For(subject Subject) Builder {
	return Builder{
		authorizer: e,
		request: Request{
			Subject: subject,
		},
	}
}

// CheckDecision evaluates a request in this order:
//  1. validate required request fields
//  2. verify resource-to-scope membership when a resource ID is provided
//  3. verify subject membership in the scope
//  4. check direct subject permissions
//  5. resolve groups and check group permissions
//
// The first matching allow returns an allowed Decision. Missing matches return
// a denied Decision. The engine does not implement deny rules or policy
// precedence beyond this order.
func (e *Engine) CheckDecision(ctx context.Context, request Request) (Decision, error) {
	decision, _, err := e.evaluate(ctx, request)
	return decision, err
}

// CheckTrace evaluates a request and also returns a high-level trace that can
// help callers debug how the final decision was reached.
func (e *Engine) CheckTrace(ctx context.Context, request Request) (Decision, Trace, error) {
	return e.evaluate(ctx, request)
}

// Check evaluates a low-level request directly and preserves the original
// boolean API for backward compatibility.
func (e *Engine) Check(ctx context.Context, request Request) (bool, error) {
	decision, err := e.CheckDecision(ctx, request)
	if err != nil {
		return false, err
	}
	return decision.Allowed, nil
}

// BatchCheck evaluates a slice of requests and returns one explicit decision
// per request.
//
// It reuses the same validation and evaluation path as Check. If a request is
// invalid, BatchCheck stops and returns a BatchCheckError identifying the
// failing item.
func (e *Engine) BatchCheck(ctx context.Context, requests []Request) ([]Decision, error) {
	decisions := make([]Decision, 0, len(requests))
	for index, request := range requests {
		decision, err := e.CheckDecision(ctx, request)
		if err != nil {
			if errors.Is(err, ErrInvalidRequest) {
				return nil, BatchCheckError{
					Index:   index,
					Request: request,
					Err:     err,
				}
			}
			return nil, err
		}
		decisions = append(decisions, decision)
	}
	return decisions, nil
}

func (e *Engine) evaluate(ctx context.Context, request Request) (Decision, Trace, error) {
	trace := Trace{}
	if err := validateRequest(request); err != nil {
		trace.add(traceStepRequestValidated, "failed")
		return Decision{}, trace, err
	}
	trace.add(traceStepRequestValidated, "passed")

	scope := newScopeContext(request)
	pipeline := newDefaultEvaluationPipeline(e, request, scope, &trace)

	if e.memberships == nil {
		return Decision{}, trace, ErrMissingMembership
	}
	if e.groups == nil {
		return Decision{}, trace, ErrMissingGroupReader
	}
	if e.permissions == nil {
		return Decision{}, trace, ErrMissingPermissions
	}
	if pipeline.requiresResourceScopeCheck() {
		inScope, err := pipeline.ensureResourceInScope(ctx)
		if err != nil {
			return Decision{}, trace, err
		}
		if !inScope {
			trace.add(traceStepResourceScopeVerified, "denied")
			return Decision{
				Allowed: false,
				Reason:  decisionReasonResourceScope,
				Source:  DecisionSourceNone,
			}, trace, nil
		}
		trace.add(traceStepResourceScopeVerified, "passed")
	}

	member, err := pipeline.hasSubjectMembership(ctx)
	if err != nil {
		return Decision{}, trace, err
	}
	if !member {
		trace.add(traceStepMembershipConfirmed, "denied")
		return Decision{
			Allowed: false,
			Reason:  decisionReasonMembership,
			Source:  DecisionSourceNone,
		}, trace, nil
	}
	trace.add(traceStepMembershipConfirmed, "passed")

	allowed, err := e.permissions.HasPermission(ctx, SubjectPrincipal(request.Subject), request)
	if err != nil {
		return Decision{}, trace, err
	}
	if allowed {
		trace.add(traceStepDirectChecked, "matched")
		return Decision{
			Allowed: true,
			Reason:  decisionReasonDirectPermission,
			Source:  DecisionSourceDirect,
		}, trace, nil
	}
	trace.add(traceStepDirectChecked, "not_matched")

	groups, err := pipeline.groupsForSubject(ctx)
	if err != nil {
		return Decision{}, trace, err
	}
	if len(groups) == 0 {
		trace.add(traceStepGroupsResolved, "none")
	} else {
		trace.add(traceStepGroupsResolved, "resolved")
	}

	groupMatched := false
	for _, group := range groups {
		allowed, err := e.permissions.HasPermission(ctx, GroupPrincipal(group), request)
		if err != nil {
			return Decision{}, trace, err
		}
		if allowed {
			groupMatched = true
			break
		}
	}
	if groupMatched {
		trace.add(traceStepGroupChecked, "matched")
		return Decision{
			Allowed: true,
			Reason:  decisionReasonGroupPermission,
			Source:  DecisionSourceGroup,
		}, trace, nil
	}
	trace.add(traceStepGroupChecked, "not_matched")

	return Decision{
		Allowed: false,
		Reason:  decisionReasonNoPermission,
		Source:  DecisionSourceNone,
	}, trace, nil
}

func (t *Trace) add(name string, result string) {
	t.Steps = append(t.Steps, TraceStep{
		Name:   name,
		Result: result,
	})
}

func validateRequest(request Request) error {
	if strings.TrimSpace(request.Subject.Type) == "" || strings.TrimSpace(request.Subject.ID) == "" {
		return ValidationError{
			Field: "subject",
			Code:  "required",
		}
	}
	if strings.TrimSpace(string(request.Action)) == "" {
		return ValidationError{
			Field: "action",
			Code:  "required",
		}
	}
	if strings.TrimSpace(request.Resource.Type) == "" {
		return ValidationError{
			Field: "resource",
			Code:  "required",
		}
	}
	if strings.TrimSpace(request.Scope.Type) == "" || strings.TrimSpace(request.Scope.ID) == "" {
		return ValidationError{
			Field: "scope",
			Code:  "required",
		}
	}
	return nil
}
