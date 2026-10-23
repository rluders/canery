package canery

import (
	"context"
	"sync"
	"testing"
)

func TestSharedEngineConcurrentUse(t *testing.T) {
	user := Subject{Type: "user", ID: "u1"}
	engine := NewEngine(
		stubMembershipReader{ok: true},
		stubGroupReader{},
		stubPermissionReader{allowed: map[PrincipalRef]bool{SubjectPrincipal(user): true}},
		stubScopeResolver{ok: true},
	)
	authorizer := NewPolicyAuthorizer(engine)
	scope := ScopeRef{Type: "project", ID: "p1"}

	// A partially configured builder is shared and branched by every goroutine.
	base := authorizer.For(user).On(ResourceRef{Type: "doc"}).In(scope)

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := context.Background()
			allowed, err := base.Can("read").Check(ctx)
			if err != nil || !allowed {
				t.Errorf("builder check = %v, %v; want allowed", allowed, err)
			}
			request := base.Can("write").Request()
			if request.Action != "write" {
				t.Errorf("action = %q, want write", request.Action)
			}
			decisions, err := engine.BatchCheck(ctx, []Request{request, base.Can("read").Request()})
			if err != nil || len(decisions) != 2 {
				t.Errorf("batch = %v, %v", decisions, err)
			}
			if _, err := base.CanMany("a", "b").Check(ctx); err != nil {
				t.Errorf("multi check: %v", err)
			}
		}()
	}
	wg.Wait()

	if base.Request().Action != "" {
		t.Fatalf("shared builder was mutated: %+v", base.Request())
	}
}

func TestSharedPolicyAuthorizerConcurrentTrace(t *testing.T) {
	user := Subject{Type: "user", ID: "u1"}
	group := GroupRef{Type: "team", ID: "t1"}
	engine := NewEngine(
		stubMembershipReader{ok: true},
		stubGroupReader{groups: []GroupRef{group}},
		stubPermissionReader{allowed: map[PrincipalRef]bool{GroupPrincipal(group): true}},
		stubScopeResolver{ok: true},
	)
	delegate := PolicyFunc(func(ctx context.Context, request Request, next DecisionEvaluator) (Decision, error) {
		return next.CheckDecision(ctx, request)
	})
	authorizer := NewPolicyAuthorizer(engine, ForResourceType("doc", delegate), ForAction("read", delegate))
	request := Request{Subject: user, Action: "read", Resource: ResourceRef{Type: "doc", ID: "d1"}, Scope: ScopeRef{Type: "project", ID: "p1"}}

	_, want, err := authorizer.CheckTrace(context.Background(), request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			decision, trace, err := authorizer.CheckTrace(context.Background(), request)
			if err != nil || !decision.Allowed {
				t.Errorf("trace check = %+v, %v; want allowed", decision, err)
				return
			}
			if len(trace.Steps) != len(want.Steps) {
				t.Errorf("trace steps = %d, want %d", len(trace.Steps), len(want.Steps))
				return
			}
			for i := range want.Steps {
				if trace.Steps[i] != want.Steps[i] {
					t.Errorf("trace step %d = %+v, want %+v", i, trace.Steps[i], want.Steps[i])
				}
			}
		}()
	}
	wg.Wait()
}
