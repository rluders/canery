package canery

import "context"

// DecisionEvaluator is the minimal continuation interface used by policies to
// delegate back to the next policy or the wrapped authorizer.
type DecisionEvaluator interface {
	// CheckDecision evaluates a low-level request directly and returns the
	// explicit decision object.
	CheckDecision(ctx context.Context, request Request) (Decision, error)
}

// Policy is evaluation middleware around an Authorizer.
//
// A Policy receives the request and a continuation (next) and returns the
// final Decision for its part of the chain. It may:
//   - delegate: return next.CheckDecision(ctx, request) unchanged;
//   - short-circuit: return its own Decision without calling next, which is a
//     valid, final decision (allow or deny) for that request;
//   - transform: call next and return a different Decision, in either
//     direction (allow to deny or deny to allow);
//   - fail: return an error, which propagates unchanged to the caller and is
//     never converted into a denial.
//
// Policies run before the wrapped Authorizer, so they see requests that have
// not been validated yet. There is no built-in deny-overrides or
// allow-overrides precedence: the outermost matching Policy's returned value
// is the result. Policies must be safe for concurrent use and must not call
// next concurrently from multiple goroutines. See docs/policies.md.
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
// Bindings are evaluated in the order given to NewPolicyAuthorizer. Only
// bindings whose matcher accepts the request run (a nil Match matches every
// request; a nil Policy is skipped). The first matching binding runs first and
// its next continues with the following matching binding, ending at the
// wrapped Authorizer.
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

	trace := Trace{}
	switch {
	case err != nil:
		trace.Steps = append(trace.Steps, TraceStep{Name: TraceStepPolicyEvaluated, Result: TraceResultError})
	case next.called:
		trace.Steps = append(trace.Steps, TraceStep{Name: TraceStepPolicyEvaluated, Result: TraceResultDelegated})
	default:
		trace.Steps = append(trace.Steps, TraceStep{Name: TraceStepPolicyEvaluated, Result: TraceResultHandled, Source: decision.Source})
	}
	if next.called {
		trace.Steps = append(trace.Steps, next.trace.Steps...)
		if err == nil && decision != next.decision {
			trace.Steps = append(trace.Steps, TraceStep{Name: TraceStepPolicyEvaluated, Result: TraceResultOverridden, Source: decision.Source})
		}
	}
	if err != nil {
		return Decision{}, trace, err
	}
	return decision, trace, nil
}

// BatchCheckResults evaluates requests one result per request, in input
// order. Requests matched by no binding are forwarded together to the wrapped
// Authorizer when it implements BatchAuthorizer; every other request runs
// through its policy chain individually.
func (a *PolicyAuthorizer) BatchCheckResults(ctx context.Context, requests []Request) []BatchResult {
	results := make([]BatchResult, len(requests))
	direct := []int{}
	for index, request := range requests {
		results[index].Request = request
		if _, _, matched := a.nextBinding(request, 0); !matched {
			direct = append(direct, index)
			continue
		}
		results[index].Decision, results[index].Err = a.CheckDecision(ctx, request)
	}
	if len(direct) == 0 {
		return results
	}
	if batch, ok := a.base.(BatchAuthorizer); ok {
		forwarded := make([]Request, len(direct))
		for position, index := range direct {
			forwarded[position] = requests[index]
		}
		for position, result := range batch.BatchCheckResults(ctx, forwarded) {
			results[direct[position]] = result
		}
		return results
	}
	for _, index := range direct {
		results[index].Decision, results[index].Err = a.CheckDecision(ctx, requests[index])
	}
	return results
}

// BatchCheck evaluates requests and returns one decision per request, in
// input order. Like Engine.BatchCheck it is all-or-nothing and reports the
// lowest failing index as a BatchCheckError.
func (a *PolicyAuthorizer) BatchCheck(ctx context.Context, requests []Request) ([]Decision, error) {
	return decisionsOrError(a.BatchCheckResults(ctx, requests))
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
	check    func(context.Context, Request) (Decision, Trace, error)
	called   bool
	trace    Trace
	decision Decision
}

// CheckDecision records the trace of the delegated evaluation. If a Policy
// calls next more than once, the trace reflects the last call.
func (c *traceContinuation) CheckDecision(ctx context.Context, request Request) (Decision, error) {
	c.called = true
	decision, trace, err := c.check(ctx, request)
	c.trace = trace
	c.decision = decision
	return decision, err
}
