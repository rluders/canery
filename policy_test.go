package canery

import (
	"context"
	"testing"
)

func TestPolicyAuthorizerDelegatesToBaseForMatchedResourcePolicy(t *testing.T) {
	base := NewEngine(
		stubMembershipReader{ok: true},
		stubGroupReader{},
		stubPermissionReader{
			allowed: map[PrincipalRef]bool{
				SubjectPrincipal(User("user-1")): true,
			},
		},
		stubScopeResolver{ok: true},
	)

	authorizer := NewPolicyAuthorizer(base, ForResourceType("document", PolicyFunc(func(ctx context.Context, request Request, next DecisionEvaluator) (Decision, error) {
		return next.CheckDecision(ctx, request)
	})))

	decision, err := authorizer.CheckDecision(context.Background(), Request{
		Subject:  User("user-1"),
		Action:   Action("view"),
		Resource: Resource("document", "doc-1"),
		Scope:    Scope("project", "project-1"),
	})
	if err != nil {
		t.Fatalf("expected no error: %v", err)
	}
	if !decision.Allowed || decision.Source != DecisionSourceDirect {
		t.Fatalf("expected delegated base allow, got %+v", decision)
	}
}

func TestPolicyAuthorizerCanHandleRequestWithoutDelegating(t *testing.T) {
	base := NewEngine(
		stubMembershipReader{ok: true},
		stubGroupReader{},
		stubPermissionReader{},
		stubScopeResolver{ok: true},
	)

	authorizer := NewPolicyAuthorizer(base, ForResourceType("document", PolicyFunc(func(context.Context, Request, DecisionEvaluator) (Decision, error) {
		return Decision{
			Allowed: true,
			Reason:  "policy matched",
			Source:  DecisionSourceDirect,
		}, nil
	})))

	decision, err := authorizer.CheckDecision(context.Background(), Request{
		Subject:  User("user-1"),
		Action:   Action("archive"),
		Resource: Resource("document", "doc-1"),
		Scope:    Scope("project", "project-1"),
	})
	if err != nil {
		t.Fatalf("expected no error: %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("expected policy to allow request")
	}
	if decision.Reason != "policy matched" {
		t.Fatalf("expected policy reason, got %q", decision.Reason)
	}
}

func TestPolicyAuthorizerSupportsCustomDomainMatcher(t *testing.T) {
	base := NewEngine(
		stubMembershipReader{ok: true},
		stubGroupReader{},
		stubPermissionReader{},
		stubScopeResolver{ok: true},
	)

	authorizer := NewPolicyAuthorizer(base, MatchRequests(func(request Request) bool {
		return request.Scope.Type == "project" && request.Action == Action("archive")
	}, PolicyFunc(func(context.Context, Request, DecisionEvaluator) (Decision, error) {
		return Decision{
			Allowed: false,
			Reason:  "policy matched",
			Source:  DecisionSourceNone,
		}, nil
	})))

	decision, err := authorizer.CheckDecision(context.Background(), Request{
		Subject:  User("user-1"),
		Action:   Action("archive"),
		Resource: Resource("document", "doc-1"),
		Scope:    Scope("project", "project-1"),
	})
	if err != nil {
		t.Fatalf("expected no error: %v", err)
	}
	if decision.Allowed {
		t.Fatalf("expected custom matcher policy to deny request")
	}
	if decision.Source != DecisionSourceNone {
		t.Fatalf("expected none source, got %q", decision.Source)
	}
}

func TestPolicyAuthorizerSupportsScopeTypeMatcher(t *testing.T) {
	base := NewEngine(
		stubMembershipReader{ok: true},
		stubGroupReader{},
		stubPermissionReader{},
		stubScopeResolver{ok: true},
	)

	authorizer := NewPolicyAuthorizer(base, ForScopeType("project", PolicyFunc(func(context.Context, Request, DecisionEvaluator) (Decision, error) {
		return Decision{
			Allowed: true,
			Reason:  "policy matched",
			Source:  DecisionSourceDirect,
		}, nil
	})))

	decision, err := authorizer.CheckDecision(context.Background(), Request{
		Subject:  Actor("service", "svc-1"),
		Action:   Action("sync"),
		Resource: Resource("document", "doc-1"),
		Scope:    Scope("project", "project-1"),
	})
	if err != nil {
		t.Fatalf("expected no error: %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("expected scope-type policy to allow request")
	}
}

func TestPolicyAuthorizerSupportsActionAndResourceMatcher(t *testing.T) {
	base := NewEngine(
		stubMembershipReader{ok: true},
		stubGroupReader{},
		stubPermissionReader{},
		stubScopeResolver{ok: true},
	)

	authorizer := NewPolicyAuthorizer(base, ForActionOnResourceType(Action("archive"), "document", PolicyFunc(func(context.Context, Request, DecisionEvaluator) (Decision, error) {
		return Decision{
			Allowed: false,
			Reason:  "policy matched",
			Source:  DecisionSourceNone,
		}, nil
	})))

	decision, err := authorizer.CheckDecision(context.Background(), Request{
		Subject:  Actor("service", "svc-1"),
		Action:   Action("archive"),
		Resource: Resource("document", "doc-1"),
		Scope:    Scope("project", "project-1"),
	})
	if err != nil {
		t.Fatalf("expected no error: %v", err)
	}
	if decision.Allowed {
		t.Fatalf("expected action/resource policy to deny request")
	}
}

func TestPolicyAuthorizerCheckTraceIncludesPolicyStep(t *testing.T) {
	base := NewEngine(
		stubMembershipReader{ok: true},
		stubGroupReader{},
		stubPermissionReader{
			allowed: map[PrincipalRef]bool{
				SubjectPrincipal(User("user-1")): true,
			},
		},
		stubScopeResolver{ok: true},
	)

	authorizer := NewPolicyAuthorizer(base, ForResourceType("document", PolicyFunc(func(ctx context.Context, request Request, next DecisionEvaluator) (Decision, error) {
		return next.CheckDecision(ctx, request)
	})))

	decision, trace, err := authorizer.CheckTrace(context.Background(), Request{
		Subject:  User("user-1"),
		Action:   Action("view"),
		Resource: Resource("document", "doc-1"),
		Scope:    Scope("project", "project-1"),
	})
	if err != nil {
		t.Fatalf("expected no error: %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("expected trace path to allow request")
	}
	if len(trace.Steps) == 0 {
		t.Fatalf("expected trace steps")
	}
	if trace.Steps[0] != (TraceStep{Name: traceStepPolicyEvaluated, Result: "delegated"}) {
		t.Fatalf("unexpected first trace step %+v", trace.Steps[0])
	}
}
