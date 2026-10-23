package canery

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

type authMethodKey struct{}
type tokenKey struct{}

func TestAuthorizationContextValues(t *testing.T) {
	var empty AuthorizationContext
	if !empty.IsZero() {
		t.Fatalf("zero context should be empty")
	}
	if _, ok := empty.Value(authMethodKey{}); ok {
		t.Fatalf("empty context returned a value")
	}

	base := empty.With(authMethodKey{}, "mfa")
	derived := base.With(authMethodKey{}, "password").With(tokenKey{}, "secret")
	if value, _ := ContextValue[string](base, authMethodKey{}); value != "mfa" {
		t.Fatalf("base was modified: %q", value)
	}
	if value, _ := ContextValue[string](derived, authMethodKey{}); value != "password" {
		t.Fatalf("derived lookup = %q", value)
	}
	if _, ok := ContextValue[int](derived, authMethodKey{}); ok {
		t.Fatalf("typed lookup ignored the value type")
	}
	if !empty.IsZero() || base.IsZero() {
		t.Fatalf("IsZero is wrong")
	}
}

func TestAuthorizationContextRejectsBadKeys(t *testing.T) {
	for _, key := range []any{nil, []string{"x"}} {
		func() {
			defer func() {
				if recover() == nil {
					t.Fatalf("expected panic for key %#v", key)
				}
			}()
			AuthorizationContext{}.With(key, 1)
		}()
	}
}

func TestPoliciesCanReadAuthorizationContext(t *testing.T) {
	readers, request := policyFixture()
	requireMFA := MatchRequests(nil, PolicyFunc(func(ctx context.Context, request Request, next DecisionEvaluator) (Decision, error) {
		if method, _ := ContextValue[string](request.Context, authMethodKey{}); method != "mfa" {
			return Decision{Allowed: false, Reason: "mfa required", Source: DecisionSourceNone}, nil
		}
		return next.CheckDecision(ctx, request)
	}))
	authorizer := NewPolicyAuthorizer(readers.engine(), requireMFA)

	without, _ := authorizer.CheckDecision(context.Background(), request)
	with := authorizer.For(request.Subject).Can(request.Action).On(request.Resource).In(request.Scope).
		WithValue(authMethodKey{}, "mfa").Request()
	allowed, err := authorizer.CheckDecision(context.Background(), with)
	if without.Allowed || err != nil || !allowed.Allowed {
		t.Fatalf("without=%+v with=%+v err=%v", without, allowed, err)
	}

	multi, err := authorizer.For(request.Subject).CanMany(request.Action).On(request.Resource).In(request.Scope).
		WithContext(with.Context).Check(context.Background())
	if ok, _ := multi.Allowed(request.Action); err != nil || !ok {
		t.Fatalf("multi-action context was not propagated")
	}
}

// contextCapturingReader records the context it receives without acting on it.
type contextCapturingReader struct {
	*countingReaders
	seen *[]AuthorizationContext
}

func (c contextCapturingReader) HasPermission(ctx context.Context, principal PrincipalRef, request Request) (bool, error) {
	*c.seen = append(*c.seen, request.Context)
	return c.countingReaders.HasPermission(ctx, principal, request)
}

func TestEngineCarriesButIgnoresAuthorizationContext(t *testing.T) {
	readers, request := policyFixture()
	seen := []AuthorizationContext{}
	engine := NewEngine(readers, readers, contextCapturingReader{readers, &seen}, readers)

	plain, _ := engine.CheckDecision(context.Background(), request)
	request.Context = AuthorizationContext{}.With(authMethodKey{}, "none").With("deny", true)
	withContext, _ := engine.CheckDecision(context.Background(), request)
	if plain != withContext {
		t.Fatalf("context changed the decision: %+v vs %+v", plain, withContext)
	}
	if len(seen) != 2 || seen[1] != request.Context {
		t.Fatalf("context was not carried to readers: %v", len(seen))
	}
}

func TestAuthorizationContextIsNeverExposed(t *testing.T) {
	const secret = "tok_live_SECRET"
	readers, request := policyFixture()
	request.Context = AuthorizationContext{}.With(tokenKey{}, secret)
	readers.permissionErr[SubjectPrincipal(batchUser)] = fmt.Errorf("reader failed")

	decision, trace, err := readers.engine().CheckTrace(context.Background(), request)
	invalid := request
	invalid.Action = ""
	_, _, validationErr := readers.engine().CheckTrace(context.Background(), invalid)
	_, batchErr := readers.engine().BatchCheck(context.Background(), []Request{invalid})

	outputs := []string{
		fmt.Sprintf("%v %+v %#v %s", request, request, request, request.Context),
		fmt.Sprintf("%+v %+v", decision, trace),
		err.Error(), validationErr.Error(), batchErr.Error(),
	}
	for _, output := range outputs {
		if strings.Contains(output, secret) {
			t.Fatalf("context value leaked: %s", output)
		}
	}
	if !strings.Contains(outputs[0], "redacted") {
		t.Fatalf("formatted context should be redacted: %s", outputs[0])
	}
}

func TestRequestRemainsComparable(t *testing.T) {
	ctx := AuthorizationContext{}.With(authMethodKey{}, "mfa")
	a := Request{Subject: User("u1"), Context: ctx}
	b := a
	if a != b {
		t.Fatalf("copied requests should be equal")
	}
	b.Context = AuthorizationContext{}.With(authMethodKey{}, "mfa")
	if a == b {
		t.Fatalf("distinct contexts compare by identity")
	}
	_ = map[Request]bool{a: true}
}
