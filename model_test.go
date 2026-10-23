package canery

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

var modelProject = Scope("project", "p1")

func modelRequest(action Action, resource ResourceRef) Request {
	return Request{Subject: batchUser, Action: action, Resource: resource, Scope: modelProject}
}

// Resource-type requests (empty ID) skip resource/scope resolution; instance
// requests require it. The permission reader always receives the request
// unchanged, so type/instance matching is owned by the reader.
func TestResourceTypeVersusInstanceRequests(t *testing.T) {
	cases := []struct {
		name         string
		request      Request
		wantResolver int
		wantAllowed  bool
	}{
		{name: "create document (type-level)", request: modelRequest("create", Resource("document", "")), wantResolver: 0, wantAllowed: true},
		{name: "read document:123 (instance)", request: modelRequest("read", Resource("document", "123")), wantResolver: 1, wantAllowed: true},
		{name: "update document:123 (instance)", request: modelRequest("update", Resource("document", "123")), wantResolver: 1, wantAllowed: false},
		{name: "delete document:123 (instance)", request: modelRequest("delete", Resource("document", "123")), wantResolver: 1, wantAllowed: false},
		{name: "type grant does not cover instance", request: modelRequest("create", Resource("document", "123")), wantResolver: 1, wantAllowed: false},
		{name: "instance grant does not cover type", request: modelRequest("read", Resource("document", "")), wantResolver: 0, wantAllowed: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			readers := newCountingReaders()
			readers.member(batchUser, modelProject)
			readers.grant(SubjectPrincipal(batchUser), "create", Resource("document", ""))
			readers.grant(SubjectPrincipal(batchUser), "read", Resource("document", "123"))

			decision, err := readers.engine().CheckDecision(context.Background(), tc.request)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if decision.Allowed != tc.wantAllowed {
				t.Fatalf("allowed = %v, want %v", decision.Allowed, tc.wantAllowed)
			}
			if readers.resolverCalls != tc.wantResolver {
				t.Fatalf("resolver calls = %d, want %d", readers.resolverCalls, tc.wantResolver)
			}
		})
	}
}

func TestTypeLevelRequestNeedsNoResolver(t *testing.T) {
	readers := newCountingReaders()
	readers.member(batchUser, modelProject)
	readers.grant(SubjectPrincipal(batchUser), "create", Resource("document", ""))
	engine := NewEngine(readers, readers, readers, nil)

	if ok, err := engine.Check(context.Background(), modelRequest("create", Resource("document", ""))); err != nil || !ok {
		t.Fatalf("type-level check = %v, %v", ok, err)
	}
	if _, err := engine.Check(context.Background(), modelRequest("read", Resource("document", "1"))); !errors.Is(err, ErrMissingScopeResolver) {
		t.Fatalf("instance check err = %v, want ErrMissingScopeResolver", err)
	}
}

func TestMembershipIsAPrerequisiteNotAGrant(t *testing.T) {
	request := modelRequest("read", Resource("document", "1"))

	t.Run("member without permission is denied", func(t *testing.T) {
		readers := newCountingReaders()
		readers.member(batchUser, modelProject)
		decision, trace, err := readers.engine().CheckTrace(context.Background(), request)
		if err != nil || decision.Allowed || decision.Reason != decisionReasonNoPermission || decision.Source != DecisionSourceNone {
			t.Fatalf("decision = %+v, %v", decision, err)
		}
		if _, ok := decision.EffectivePrincipal(); ok {
			t.Fatalf("denial reported an effective principal")
		}
		if last := trace.Steps[len(trace.Steps)-1]; last.Name != TraceStepGroupChecked || last.Source != DecisionSourceNone {
			t.Fatalf("last step = %+v", last)
		}
	})

	t.Run("non-member with matching permission is denied", func(t *testing.T) {
		readers := newCountingReaders()
		readers.grant(SubjectPrincipal(batchUser), "read", Resource("document", "1"))
		decision, trace, err := readers.engine().CheckTrace(context.Background(), request)
		if err != nil || decision.Allowed || decision.Reason != decisionReasonMembership || decision.Source != DecisionSourceNone {
			t.Fatalf("decision = %+v, %v", decision, err)
		}
		if readers.permissionCalls != 0 || readers.groupCalls != 0 {
			t.Fatalf("permissions consulted for a non-member")
		}
		want := TraceStep{Name: TraceStepMembershipConfirmed, Result: TraceResultDenied, Source: DecisionSourceNone}
		if last := trace.Steps[len(trace.Steps)-1]; last != want {
			t.Fatalf("last step = %+v, want %+v", last, want)
		}
	})

	t.Run("membership reader failure is an error, not a denial", func(t *testing.T) {
		readers := newCountingReaders()
		readers.member(batchUser, modelProject)
		readers.membershipErr = errors.New("ldap unreachable")
		decision, trace, err := readers.engine().CheckTrace(context.Background(), request)
		if !errors.Is(err, readers.membershipErr) {
			t.Fatalf("err = %v, want reader error", err)
		}
		var evalErr EvaluationError
		if !errors.As(err, &evalErr) || evalErr.Stage != TraceStepMembershipConfirmed {
			t.Fatalf("err = %#v, want EvaluationError at membership", err)
		}
		if decision != (Decision{}) {
			t.Fatalf("failed check returned decision %+v", decision)
		}
		if last := trace.Steps[len(trace.Steps)-1]; last.Name != TraceStepMembershipConfirmed || last.Result != TraceResultError {
			t.Fatalf("last step = %+v", last)
		}
	})
}

func TestMembershipSemanticsAreConsistentAcrossEntryPoints(t *testing.T) {
	readers := newCountingReaders()
	readers.grant(SubjectPrincipal(batchUser), "read", Resource("document", "1"))
	engine := readers.engine()
	request := modelRequest("read", Resource("document", "1"))

	allowed, err := engine.Check(context.Background(), request)
	decision, _ := engine.CheckDecision(context.Background(), request)
	traced, _, _ := engine.CheckTrace(context.Background(), request)
	fluent, _ := engine.For(request.Subject).Can(request.Action).On(request.Resource).In(request.Scope).Check(context.Background())
	batch, _ := engine.BatchCheck(context.Background(), []Request{request})
	multi, _ := engine.For(request.Subject).CanMany(request.Action).On(request.Resource).In(request.Scope).Check(context.Background())
	multiAllowed, _ := multi.Allowed(request.Action)
	policy, _ := NewPolicyAuthorizer(engine).CheckDecision(context.Background(), request)

	if err != nil || allowed || decision.Allowed || traced.Allowed || fluent || batch[0].Allowed || multiAllowed || policy.Allowed {
		t.Fatalf("a non-member was allowed through some entry point")
	}
	if decision != traced || decision != batch[0] || decision != policy {
		t.Fatalf("entry points disagree: %+v %+v %+v %+v", decision, traced, batch[0], policy)
	}
}

func TestDirectPermissionsAreCheckedBeforeGroups(t *testing.T) {
	readers := newCountingReaders()
	readers.member(batchUser, modelProject, batchEditors)
	readers.grant(SubjectPrincipal(batchUser), "read", Resource("document", "1"))
	readers.grant(GroupPrincipal(batchEditors), "read", Resource("document", "1"))

	decision, err := readers.engine().CheckDecision(context.Background(), modelRequest("read", Resource("document", "1")))
	if err != nil || decision.Source != DecisionSourceDirect {
		t.Fatalf("decision = %+v, %v", decision, err)
	}
	if readers.groupCalls != 0 {
		t.Fatalf("groups resolved after a direct match")
	}
}

func TestGroupMembershipAloneDoesNotGrant(t *testing.T) {
	readers := newCountingReaders()
	readers.member(batchUser, modelProject, batchEditors, batchViewers)
	decision, err := readers.engine().CheckDecision(context.Background(), modelRequest("read", Resource("document", "1")))
	if err != nil || decision.Allowed {
		t.Fatalf("decision = %+v, %v", decision, err)
	}
}

func TestInvalidRequestsNeverAllow(t *testing.T) {
	allowAll := PolicyFunc(func(ctx context.Context, request Request, next DecisionEvaluator) (Decision, error) {
		return next.CheckDecision(ctx, request)
	})
	readers := newCountingReaders()
	readers.member(Subject{Type: "user"}, modelProject)
	readers.grant(SubjectPrincipal(Subject{Type: "user"}), "read", Resource("document", "1"))
	request := Request{Subject: Subject{Type: "user"}, Action: "read", Resource: Resource("document", "1"), Scope: modelProject}

	for _, authorizer := range []Authorizer{readers.engine(), NewPolicyAuthorizer(readers.engine(), MatchRequests(nil, allowAll))} {
		decision, err := authorizer.CheckDecision(context.Background(), request)
		if !errors.Is(err, ErrInvalidRequest) || decision.Allowed {
			t.Fatalf("decision = %+v, %v", decision, err)
		}
	}
	if readers.membershipCalls+readers.permissionCalls != 0 {
		t.Fatalf("readers called for an invalid request")
	}
}

func TestEvaluationIsStatelessBetweenRequests(t *testing.T) {
	readers := batchFixture()
	engine := readers.engine()
	requests := batchFixtureRequests()
	first := engine.BatchCheckResults(context.Background(), requests)
	for round := 0; round < 3; round++ {
		for index, request := range requests {
			decision, err := engine.CheckDecision(context.Background(), request)
			if decision != first[index].Decision || !errors.Is(err, first[index].Err) {
				t.Fatalf("round %d request %d changed: %+v vs %+v", round, index, decision, first[index])
			}
		}
	}
}

func TestDecisionAttribution(t *testing.T) {
	request := modelRequest("read", Resource("document", "1"))

	t.Run("direct", func(t *testing.T) {
		readers := newCountingReaders()
		readers.member(batchUser, modelProject)
		readers.grant(SubjectPrincipal(batchUser), "read", Resource("document", "1"))
		decision, trace, _ := readers.engine().CheckTrace(context.Background(), request)
		principal, ok := decision.EffectivePrincipal()
		if !ok || principal != SubjectPrincipal(batchUser) {
			t.Fatalf("principal = %+v, %v", principal, ok)
		}
		last := trace.Steps[len(trace.Steps)-1]
		if last.Principal != principal || last.Source != decision.Source {
			t.Fatalf("trace %+v contradicts decision %+v", last, decision)
		}
	})

	t.Run("first matching group in reader order", func(t *testing.T) {
		readers := newCountingReaders()
		readers.member(batchUser, modelProject, batchViewers, batchEditors)
		readers.grant(GroupPrincipal(batchViewers), "read", Resource("document", "1"))
		readers.grant(GroupPrincipal(batchEditors), "read", Resource("document", "1"))
		for _, engine := range []*Engine{readers.engine(), batchReaders{readers}.engine()} {
			decision, trace, _ := engine.CheckTrace(context.Background(), request)
			principal, ok := decision.EffectivePrincipal()
			if !ok || principal != GroupPrincipal(batchViewers) || decision.Source != DecisionSourceGroup {
				t.Fatalf("decision = %+v", decision)
			}
			last := trace.Steps[len(trace.Steps)-1]
			if last.Principal != principal || last.Source != DecisionSourceGroup {
				t.Fatalf("trace %+v contradicts decision %+v", last, decision)
			}
		}
	})

	t.Run("empty identifier is still a principal", func(t *testing.T) {
		decision := Decision{Allowed: true, Principal: PrincipalRef{Kind: PrincipalKindGroup}}
		if _, ok := decision.EffectivePrincipal(); !ok {
			t.Fatalf("principal with empty ID should be reported")
		}
	})
}

func TestDecisionSourceIsTyped(t *testing.T) {
	values := map[DecisionSource]string{
		DecisionSourceDirect: "direct",
		DecisionSourceGroup:  "group",
		DecisionSourceNone:   "none",
	}
	for source, want := range values {
		if string(source) != want || source.String() != want || fmt.Sprint(source) != want {
			t.Fatalf("source %q does not render as %q", source, want)
		}
	}
}

func TestTraceDistinguishesSkippedStages(t *testing.T) {
	readers := newCountingReaders()
	readers.member(batchUser, modelProject)
	_, trace, _ := readers.engine().CheckTrace(context.Background(), modelRequest("create", Resource("document", "")))
	if trace.Steps[1] != (TraceStep{Name: TraceStepResourceScopeVerified, Result: TraceResultSkipped}) {
		t.Fatalf("step 1 = %+v, want skipped resource scope", trace.Steps[1])
	}
}

func TestTraceMarksReaderFailures(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*countingReaders)
		stage string
	}{
		{name: "resolver", setup: func(r *countingReaders) { r.resolverErr = errors.New("x") }, stage: TraceStepResourceScopeVerified},
		{name: "groups", setup: func(r *countingReaders) { r.groupsErr = errors.New("x") }, stage: TraceStepGroupsResolved},
		{name: "direct", setup: func(r *countingReaders) { r.permissionErr[SubjectPrincipal(batchUser)] = errors.New("x") }, stage: TraceStepDirectChecked},
		{name: "group permission", setup: func(r *countingReaders) { r.permissionErr[GroupPrincipal(batchEditors)] = errors.New("x") }, stage: TraceStepGroupChecked},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			readers := newCountingReaders()
			readers.member(batchUser, modelProject, batchEditors)
			tc.setup(readers)
			_, trace, err := readers.engine().CheckTrace(context.Background(), modelRequest("read", Resource("document", "1")))
			var evalErr EvaluationError
			if !errors.As(err, &evalErr) || evalErr.Stage != tc.stage {
				t.Fatalf("err = %v, want EvaluationError at %s", err, tc.stage)
			}
			last := trace.Steps[len(trace.Steps)-1]
			if last.Name != tc.stage || last.Result != TraceResultError || last.Source != "" {
				t.Fatalf("last step = %+v", last)
			}
		})
	}
}

func TestReaderErrorMessagesAreNotExposed(t *testing.T) {
	secret := "postgres://admin:hunter2@10.0.0.5/authz: SELECT * FROM grants"
	readers := newCountingReaders()
	readers.member(batchUser, modelProject)
	readers.permissionErr[SubjectPrincipal(batchUser)] = errors.New(secret)
	request := modelRequest("read", Resource("document", "1"))

	_, err := readers.engine().Check(context.Background(), request)
	_, batchErr := readers.engine().BatchCheck(context.Background(), []Request{request})
	for _, err := range []error{err, batchErr} {
		if err == nil || strings.Contains(err.Error(), "hunter2") || strings.Contains(err.Error(), "SELECT") {
			t.Fatalf("error leaks reader details: %v", err)
		}
		if !errors.Is(err, readers.permissionErr[SubjectPrincipal(batchUser)]) {
			t.Fatalf("underlying error is not inspectable: %v", err)
		}
	}
}
