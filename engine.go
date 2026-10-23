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
	// ErrBatchResultMismatch indicates that a BatchPermissionReader returned a
	// different number of results than checks.
	ErrBatchResultMismatch = errors.New("canery: batch permission reader returned mismatched results")
)

const (
	decisionReasonDirectPermission = "direct permission matched"
	decisionReasonGroupPermission  = "group permission matched"
	decisionReasonResourceScope    = "resource outside scope"
	decisionReasonMembership       = "subject not in scope"
	decisionReasonNoPermission     = "no matching permission"
)

// ValidationCodeRequired is the ValidationError code for a missing or
// whitespace-only field.
const ValidationCodeRequired = "required"

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
// field/property/code triple.
//
// Field is the request component ("subject", "action", "resource", or
// "scope"). Property is the component property ("type" or "id"), or empty when
// the component itself is the value (action). Code is ValidationCodeRequired.
// Validation stops at the first failure, checked in this order: subject.type,
// subject.id, action, resource.type, scope.type, scope.id.
type ValidationError struct {
	Field    string
	Property string
	Code     string
}

// Path returns the dotted field path, such as "subject.id" or "action".
func (e ValidationError) Path() string {
	if e.Property == "" {
		return e.Field
	}
	return e.Field + "." + e.Property
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("canery: invalid request: %s: %s", e.Path(), e.Code)
}

func (e ValidationError) Unwrap() error {
	return ErrInvalidRequest
}

// EvaluationError reports that a reader or resolver failed while evaluating a
// request.
//
// Stage is the TraceStep* name of the failing stage. Error() deliberately
// omits the underlying message so infrastructure details (queries, hosts,
// credentials) do not leak through canery's error strings; use errors.Is,
// errors.As, or Unwrap to inspect the cause.
type EvaluationError struct {
	Stage string
	Err   error
}

func (e EvaluationError) Error() string {
	return "canery: evaluation failed at " + e.Stage
}

func (e EvaluationError) Unwrap() error {
	return e.Err
}

// BatchResult is the outcome of one request in a batch. Err is nil when
// Decision holds an evaluated outcome (allowed or denied); otherwise the
// request could not be evaluated and Decision is zero.
type BatchResult struct {
	Request  Request
	Decision Decision
	Err      error
}

// BatchAuthorizer is implemented by authorizers that can evaluate several
// requests together. Engine and PolicyAuthorizer implement it, and
// MultiActionBuilder.Check uses it when available.
type BatchAuthorizer interface {
	BatchCheckResults(ctx context.Context, requests []Request) []BatchResult
}

// Engine is the default Authorizer implementation backed by pluggable readers
// and a resource scope resolver.
//
// Engine intentionally implements one default evaluation strategy: a
// membership-scoped pipeline that validates the request, optionally checks a
// concrete resource against the requested scope, confirms scope membership, and
// then evaluates direct and group-based permissions. See
// docs/authorization-model.md for the full contract.
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
//  5. resolve groups and check group permissions in reader order
//
// The first matching allow returns an allowed Decision. Missing matches return
// a denied Decision. Reader failures return an EvaluationError and never a
// denial. The engine does not implement deny rules or policy precedence beyond
// this order.
func (e *Engine) CheckDecision(ctx context.Context, request Request) (Decision, error) {
	decision, _, err := e.evaluate(ctx, request, false)
	return decision, err
}

// CheckTrace evaluates a request and also returns a high-level trace that can
// help callers debug how the final decision was reached. On error the trace
// contains the steps completed so far, ending with the failing step.
func (e *Engine) CheckTrace(ctx context.Context, request Request) (Decision, Trace, error) {
	return e.evaluate(ctx, request, true)
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

// BatchCheck evaluates a slice of requests and returns one decision per
// request, in input order.
//
// It is all-or-nothing: every request is validated before any reader is
// called, and if any request is invalid or cannot be evaluated, BatchCheck
// returns nil decisions and a BatchCheckError for the lowest failing index.
// Denials are not failures. Use BatchCheckResults to keep partial results.
func (e *Engine) BatchCheck(ctx context.Context, requests []Request) ([]Decision, error) {
	for index, request := range requests {
		if err := validateRequest(request); err != nil {
			return nil, BatchCheckError{Index: index, Request: request, Err: err}
		}
	}
	return decisionsOrError(e.BatchCheckResults(ctx, requests))
}

// BatchCheckResults evaluates requests together and returns one BatchResult
// per request, in input order.
//
// Identical membership, group, resource-scope, and permission lookups are
// performed once per call, and permission checks use BatchPermissionReader
// when available. Each result is equivalent to calling CheckDecision on that
// request alone. A failure affects only the requests that depended on the
// failed lookup; other results are still returned. If ctx is cancelled,
// requests that were not yet decided report the context error.
func (e *Engine) BatchCheckResults(ctx context.Context, requests []Request) []BatchResult {
	items := e.run(ctx, requests, false)
	results := make([]BatchResult, len(items))
	for index, item := range items {
		results[index] = BatchResult{
			Request:  item.request,
			Decision: item.decision,
			Err:      item.err,
		}
	}
	return results
}

func (e *Engine) evaluate(ctx context.Context, request Request, withTrace bool) (Decision, Trace, error) {
	item := e.run(ctx, []Request{request}, withTrace)[0]
	return item.decision, item.trace, item.err
}

func decisionsOrError(results []BatchResult) ([]Decision, error) {
	decisions := make([]Decision, len(results))
	for index, result := range results {
		if result.Err != nil {
			return nil, BatchCheckError{Index: index, Request: result.Request, Err: result.Err}
		}
		decisions[index] = result.Decision
	}
	return decisions, nil
}

func validateRequest(request Request) error {
	switch {
	case blank(request.Subject.Type):
		return ValidationError{Field: "subject", Property: "type", Code: ValidationCodeRequired}
	case blank(request.Subject.ID):
		return ValidationError{Field: "subject", Property: "id", Code: ValidationCodeRequired}
	case blank(string(request.Action)):
		return ValidationError{Field: "action", Code: ValidationCodeRequired}
	case blank(request.Resource.Type):
		return ValidationError{Field: "resource", Property: "type", Code: ValidationCodeRequired}
	case blank(request.Scope.Type):
		return ValidationError{Field: "scope", Property: "type", Code: ValidationCodeRequired}
	case blank(request.Scope.ID):
		return ValidationError{Field: "scope", Property: "id", Code: ValidationCodeRequired}
	}
	return nil
}

func blank(value string) bool {
	return strings.TrimSpace(value) == ""
}
