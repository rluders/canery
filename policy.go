package canery

import "context"

const traceStepPolicyEvaluated = "policy_evaluated"

// DecisionEvaluator is the minimal continuation interface used by policies to
// delegate back to the next policy or the wrapped authorizer.
type DecisionEvaluator interface {
	// CheckDecision evaluates a low-level request directly and returns the
	// explicit decision object.
	CheckDecision(ctx context.Context, request Request) (Decision, error)
}

// Policy can wrap authorization decisions for a matched request before
// delegating to the next evaluator.
//
// Policies are optional and compose on top of the core request-driven
// authorizer. They can handle a request directly or call next.CheckDecision to
// continue the evaluation chain.
type Policy interface {
	CheckDecision(ctx context.Context, request Request, next DecisionEvaluator) (Decision, error)
}

// PolicyFunc adapts a function to the Policy interface.
type PolicyFunc func(ctx context.Context, request Request, next DecisionEvaluator) (Decision, error)

// CheckDecision calls f(ctx, request, next).
func (f PolicyFunc) CheckDecision(ctx context.Context, request Request, next DecisionEvaluator) (Decision, error) {
	return f(ctx, request, next)
}

// RequestMatcher decides whether a policy binding applies to a request.
type RequestMatcher func(Request) bool

// PolicyBinding associates a request matcher with a policy.
type PolicyBinding struct {
	Match  RequestMatcher
	Policy Policy
}

// MatchRequests creates a policy binding backed by an arbitrary request
// matcher.
func MatchRequests(match RequestMatcher, policy Policy) PolicyBinding {
	return PolicyBinding{
		Match:  match,
		Policy: policy,
	}
}

// ForAction creates a policy binding that applies to one action.
func ForAction(action Action, policy Policy) PolicyBinding {
	return MatchRequests(func(request Request) bool {
		return request.Action == action
	}, policy)
}

// ForResourceType creates a policy binding that applies to one resource type.
func ForResourceType(resourceType string, policy Policy) PolicyBinding {
	return MatchRequests(func(request Request) bool {
		return request.Resource.Type == resourceType
	}, policy)
}

// ForScopeType creates a policy binding that applies to one scope type.
func ForScopeType(scopeType string, policy Policy) PolicyBinding {
	return MatchRequests(func(request Request) bool {
		return request.Scope.Type == scopeType
	}, policy)
}

// ForActionOnResourceType creates a policy binding that applies when both the
// action and resource type match.
func ForActionOnResourceType(action Action, resourceType string, policy Policy) PolicyBinding {
	return MatchRequests(func(request Request) bool {
		return request.Action == action && request.Resource.Type == resourceType
	}, policy)
}

// ForActionInScopeType creates a policy binding that applies when both the
// action and scope type match.
func ForActionInScopeType(action Action, scopeType string, policy Policy) PolicyBinding {
	return MatchRequests(func(request Request) bool {
		return request.Action == action && request.Scope.Type == scopeType
	}, policy)
}

// PolicyAuthorizer wraps an Authorizer with an ordered set of optional
// policies.
//
// Policies run only when their binding matches the request. A matched policy
// can return a decision directly or delegate to the next evaluator.
type PolicyAuthorizer struct {
	base     Authorizer
	bindings []PolicyBinding
}

// NewPolicyAuthorizer constructs an Authorizer that evaluates matched policies
// before falling back to the wrapped base authorizer.
func NewPolicyAuthorizer(base Authorizer, bindings ...PolicyBinding) *PolicyAuthorizer {
	return &PolicyAuthorizer{
		base:     base,
		bindings: append([]PolicyBinding(nil), bindings...),
	}
}

// For starts a fluent authorization request for the given subject.
func (a *PolicyAuthorizer) For(subject Subject) Builder {
	return Builder{
		authorizer: a,
		request: Request{
			Subject: subject,
		},
	}
}

// CheckDecision evaluates the request through the matching policy chain and
// then the wrapped authorizer.
func (a *PolicyAuthorizer) CheckDecision(ctx context.Context, request Request) (Decision, error) {
	return a.checkDecisionFrom(ctx, request, 0)
}

// CheckTrace evaluates the request through the matching policy chain and also
// returns a high-level trace for debugging.
func (a *PolicyAuthorizer) CheckTrace(ctx context.Context, request Request) (Decision, Trace, error) {
	return a.checkTraceFrom(ctx, request, 0)
}

// Check preserves the original boolean API for backward compatibility.
func (a *PolicyAuthorizer) Check(ctx context.Context, request Request) (bool, error) {
	decision, err := a.CheckDecision(ctx, request)
	if err != nil {
		return false, err
	}
	return decision.Allowed, nil
}

func (a *PolicyAuthorizer) checkDecisionFrom(ctx context.Context, request Request, start int) (Decision, error) {
	if a.base == nil {
		return Decision{}, ErrMissingAuthorizer
	}
	index, binding, ok := a.nextBinding(request, start)
	if !ok {
		return a.base.CheckDecision(ctx, request)
	}
	return binding.Policy.CheckDecision(ctx, request, decisionContinuation{
		check: func(ctx context.Context, request Request) (Decision, error) {
			return a.checkDecisionFrom(ctx, request, index+1)
		},
	})
}

func (a *PolicyAuthorizer) checkTraceFrom(ctx context.Context, request Request, start int) (Decision, Trace, error) {
	if a.base == nil {
		return Decision{}, Trace{}, ErrMissingAuthorizer
	}
	index, binding, ok := a.nextBinding(request, start)
	if !ok {
		return a.base.CheckTrace(ctx, request)
	}

	next := &traceContinuation{
		check: func(ctx context.Context, request Request) (Decision, Trace, error) {
			return a.checkTraceFrom(ctx, request, index+1)
		},
	}
	decision, err := binding.Policy.CheckDecision(ctx, request, next)
	if err != nil {
		return Decision{}, Trace{}, err
	}

	trace := Trace{}
	if next.called {
		trace.add(traceStepPolicyEvaluated, "delegated")
		trace.Steps = append(trace.Steps, next.trace.Steps...)
	} else {
		trace.add(traceStepPolicyEvaluated, "handled")
	}
	return decision, trace, nil
}

func (a *PolicyAuthorizer) nextBinding(request Request, start int) (int, PolicyBinding, bool) {
	for index := start; index < len(a.bindings); index++ {
		binding := a.bindings[index]
		if binding.Policy == nil {
			continue
		}
		if binding.Match == nil || binding.Match(request) {
			return index, binding, true
		}
	}
	return 0, PolicyBinding{}, false
}

type decisionContinuation struct {
	check func(context.Context, Request) (Decision, error)
}

func (c decisionContinuation) CheckDecision(ctx context.Context, request Request) (Decision, error) {
	return c.check(ctx, request)
}

type traceContinuation struct {
	check  func(context.Context, Request) (Decision, Trace, error)
	called bool
	trace  Trace
}

func (c *traceContinuation) CheckDecision(ctx context.Context, request Request) (Decision, error) {
	c.called = true
	decision, trace, err := c.check(ctx, request)
	c.trace = trace
	return decision, err
}
