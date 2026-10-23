package canery

import (
	"context"
	"errors"
)

var ErrMissingAuthorizer = errors.New("canery: missing authorizer")

// Builder incrementally assembles a Request before evaluating it through an
// Authorizer.
type Builder struct {
	authorizer Authorizer
	request    Request
}

// Can sets the action being requested.
func (b Builder) Can(action Action) Builder {
	b.request.Action = action
	return b
}

// CanMany starts a multi-action request for the same subject, resource, and
// scope.
func (b Builder) CanMany(actions ...Action) MultiActionBuilder {
	return MultiActionBuilder{
		authorizer: b.authorizer,
		request:    b.request,
		actions:    append([]Action(nil), actions...),
	}
}

// On sets the resource being targeted.
func (b Builder) On(resource ResourceRef) Builder {
	b.request.Resource = resource
	return b
}

// Target is a readability wrapper around On.
func (b Builder) Target(resource ResourceRef) Builder {
	return b.On(resource)
}

// Within sets the scope in which the request should be evaluated.
func (b Builder) Within(scope ScopeRef) Builder {
	b.request.Scope = scope
	return b
}

// In is a readability wrapper around Within.
func (b Builder) In(scope ScopeRef) Builder {
	return b.Within(scope)
}

// WithContext sets the authorization context carried by the request.
func (b Builder) WithContext(authorizationContext AuthorizationContext) Builder {
	b.request.Context = authorizationContext
	return b
}

// WithValue adds one value to the request's authorization context.
func (b Builder) WithValue(key any, value any) Builder {
	b.request.Context = b.request.Context.With(key, value)
	return b
}

// Request returns the low-level request represented by the builder.
func (b Builder) Request() Request {
	return b.request
}

// Check evaluates the built request through the configured Authorizer.
func (b Builder) Check(ctx context.Context) (bool, error) {
	if b.authorizer == nil {
		return false, ErrMissingAuthorizer
	}
	return b.authorizer.Check(ctx, b.request)
}

// ActionDecision contains the authorization outcome for a single action.
//
// Err is nil when the action was evaluated; Allowed=false with a nil Err is a
// denial. Decision holds the full decision when Err is nil.
type ActionDecision struct {
	Allowed  bool
	Err      error
	Decision Decision
}

// MultiActionResult contains the decision for each requested action.
type MultiActionResult struct {
	Decisions map[Action]ActionDecision
}

// Allowed returns the allow decision for a specific action and whether that
// action was present in the result.
func (r MultiActionResult) Allowed(action Action) (bool, bool) {
	decision, ok := r.Decisions[action]
	if !ok {
		return false, false
	}
	return decision.Allowed, true
}

// Error returns the evaluation error for a specific action and whether that
// action was present in the result.
func (r MultiActionResult) Error(action Action) (error, bool) {
	decision, ok := r.Decisions[action]
	if !ok {
		return nil, false
	}
	return decision.Err, true
}

// MultiActionBuilder incrementally assembles a repeated request over multiple
// actions for the same subject, resource, and scope.
type MultiActionBuilder struct {
	authorizer Authorizer
	request    Request
	actions    []Action
}

// On sets the resource being targeted for all actions.
func (b MultiActionBuilder) On(resource ResourceRef) MultiActionBuilder {
	b.request.Resource = resource
	return b
}

// Target is a readability wrapper around On.
func (b MultiActionBuilder) Target(resource ResourceRef) MultiActionBuilder {
	return b.On(resource)
}

// Within sets the scope in which all actions should be evaluated.
func (b MultiActionBuilder) Within(scope ScopeRef) MultiActionBuilder {
	b.request.Scope = scope
	return b
}

// In is a readability wrapper around Within.
func (b MultiActionBuilder) In(scope ScopeRef) MultiActionBuilder {
	return b.Within(scope)
}

// WithContext sets the authorization context carried by every request.
func (b MultiActionBuilder) WithContext(authorizationContext AuthorizationContext) MultiActionBuilder {
	b.request.Context = authorizationContext
	return b
}

// WithValue adds one value to the authorization context of every request.
func (b MultiActionBuilder) WithValue(key any, value any) MultiActionBuilder {
	b.request.Context = b.request.Context.With(key, value)
	return b
}

// Requests returns one low-level request per action represented by the builder.
func (b MultiActionBuilder) Requests() []Request {
	requests := make([]Request, 0, len(b.actions))
	for _, action := range b.actions {
		request := b.request
		request.Action = action
		requests = append(requests, request)
	}
	return requests
}

// Check evaluates each action through the configured Authorizer and returns one
// decision per action.
//
// Per-action failures are reported in ActionDecision.Err and never abort the
// other actions; the returned error is only set when no Authorizer is
// configured. When the Authorizer implements BatchAuthorizer, all actions are
// evaluated as one batch. If an action is repeated, its single result is
// reported once.
func (b MultiActionBuilder) Check(ctx context.Context) (MultiActionResult, error) {
	if b.authorizer == nil {
		return MultiActionResult{}, ErrMissingAuthorizer
	}
	requests := b.Requests()
	result := MultiActionResult{
		Decisions: make(map[Action]ActionDecision, len(requests)),
	}
	if batch, ok := b.authorizer.(BatchAuthorizer); ok {
		for _, item := range batch.BatchCheckResults(ctx, requests) {
			result.Decisions[item.Request.Action] = ActionDecision{
				Allowed:  item.Err == nil && item.Decision.Allowed,
				Err:      item.Err,
				Decision: item.Decision,
			}
		}
		return result, nil
	}
	for _, request := range requests {
		decision, err := b.authorizer.CheckDecision(ctx, request)
		if err != nil {
			decision = Decision{}
		}
		result.Decisions[request.Action] = ActionDecision{
			Allowed:  err == nil && decision.Allowed,
			Err:      err,
			Decision: decision,
		}
	}
	return result, nil
}
