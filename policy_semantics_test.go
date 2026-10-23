package canery

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func policyFixture() (*countingReaders, Request) {
	readers := newCountingReaders()
	readers.member(batchUser, modelProject)
	readers.grant(SubjectPrincipal(batchUser), "read", Resource("document", "1"))
	return readers, modelRequest("read", Resource("document", "1"))
}

func recordingPolicy(name string, calls *[]string) PolicyFunc {
	return func(ctx context.Context, request Request, next DecisionEvaluator) (Decision, error) {
		*calls = append(*calls, name+":before")
		decision, err := next.CheckDecision(ctx, request)
		*calls = append(*calls, name+":after")
		return decision, err
	}
}

func TestPoliciesRunInConfiguredOrder(t *testing.T) {
	readers, request := policyFixture()
	calls := []string{}
	authorizer := NewPolicyAuthorizer(readers.engine(),
		MatchRequests(nil, recordingPolicy("a", &calls)),
		ForAction("write", recordingPolicy("skipped", &calls)),
		ForResourceType("document", recordingPolicy("b", &calls)),
		PolicyBinding{Policy: nil},
		ForAction("read", recordingPolicy("c", &calls)),
	)
	decision, err := authorizer.CheckDecision(context.Background(), request)
	if err != nil || !decision.Allowed {
		t.Fatalf("decision = %+v, %v", decision, err)
	}
	want := []string{"a:before", "b:before", "c:before", "c:after", "b:after", "a:after"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestPolicyShortCircuitIsFinal(t *testing.T) {
	readers, request := policyFixture()
	calls := []string{}
	handled := Decision{Allowed: false, Reason: "maintenance window", Source: DecisionSourceNone}
	authorizer := NewPolicyAuthorizer(readers.engine(),
		MatchRequests(nil, PolicyFunc(func(context.Context, Request, DecisionEvaluator) (Decision, error) {
			return handled, nil
		})),
		MatchRequests(nil, recordingPolicy("inner", &calls)),
	)
	decision, trace, err := authorizer.CheckTrace(context.Background(), request)
	if err != nil || decision != handled {
		t.Fatalf("decision = %+v, %v", decision, err)
	}
	if len(calls) != 0 || readers.membershipCalls != 0 {
		t.Fatalf("evaluation continued after short-circuit")
	}
	assertTraceSteps(t, trace, []TraceStep{{Name: TraceStepPolicyEvaluated, Result: TraceResultHandled, Source: DecisionSourceNone}})
}

func TestPolicyCanTransformDecisionsBothWays(t *testing.T) {
	readers, request := policyFixture()
	invert := PolicyFunc(func(ctx context.Context, request Request, next DecisionEvaluator) (Decision, error) {
		decision, err := next.CheckDecision(ctx, request)
		if err != nil {
			return Decision{}, err
		}
		if decision.Allowed {
			return Decision{Allowed: false, Reason: "policy revoked", Source: DecisionSourceNone}, nil
		}
		return Decision{Allowed: true, Reason: "policy granted", Source: DecisionSourceNone}, nil
	})
	authorizer := NewPolicyAuthorizer(readers.engine(), MatchRequests(nil, invert))

	revoked, trace, err := authorizer.CheckTrace(context.Background(), request)
	if err != nil || revoked.Allowed {
		t.Fatalf("allow->deny = %+v, %v", revoked, err)
	}
	if trace.Steps[0].Result != TraceResultDelegated {
		t.Fatalf("first step = %+v", trace.Steps[0])
	}
	if last := trace.Steps[len(trace.Steps)-1]; last != (TraceStep{Name: TraceStepPolicyEvaluated, Result: TraceResultOverridden, Source: DecisionSourceNone}) {
		t.Fatalf("last step = %+v, want overridden", last)
	}

	request.Action = "delete"
	granted, err := authorizer.CheckDecision(context.Background(), request)
	if err != nil || !granted.Allowed {
		t.Fatalf("deny->allow = %+v, %v", granted, err)
	}
}

func TestPolicyErrorsPropagateUnchanged(t *testing.T) {
	readers, request := policyFixture()
	policyErr := errors.New("policy backend unavailable")
	failing := MatchRequests(nil, PolicyFunc(func(context.Context, Request, DecisionEvaluator) (Decision, error) {
		return Decision{Allowed: true}, policyErr
	}))
	authorizer := NewPolicyAuthorizer(readers.engine(), MatchRequests(nil, recordingPolicy("outer", &[]string{})), failing)

	decision, trace, err := authorizer.CheckTrace(context.Background(), request)
	if err != policyErr {
		t.Fatalf("err = %v, want the policy error itself", err)
	}
	if decision.Allowed {
		t.Fatalf("errored evaluation allowed")
	}
	assertTraceSteps(t, trace, []TraceStep{
		{Name: TraceStepPolicyEvaluated, Result: TraceResultError},
		{Name: TraceStepPolicyEvaluated, Result: TraceResultError},
	})
	if allowed, err := authorizer.Check(context.Background(), request); allowed || err != policyErr {
		t.Fatalf("Check = %v, %v", allowed, err)
	}
}

func TestEvaluatorErrorsPropagateThroughPolicies(t *testing.T) {
	readers, request := policyFixture()
	readers.membershipErr = errors.New("membership down")
	authorizer := NewPolicyAuthorizer(readers.engine(), MatchRequests(nil, recordingPolicy("a", &[]string{})))

	_, trace, err := authorizer.CheckTrace(context.Background(), request)
	if !errors.Is(err, readers.membershipErr) {
		t.Fatalf("err = %v", err)
	}
	if trace.Steps[0] != (TraceStep{Name: TraceStepPolicyEvaluated, Result: TraceResultError}) {
		t.Fatalf("first step = %+v", trace.Steps[0])
	}
	if last := trace.Steps[len(trace.Steps)-1]; last.Result != TraceResultError || last.Name != TraceStepMembershipConfirmed {
		t.Fatalf("last step = %+v", last)
	}
}

func TestPolicyCheckAndTraceAgree(t *testing.T) {
	readers, request := policyFixture()
	authorizer := NewPolicyAuthorizer(readers.engine(), MatchRequests(nil, recordingPolicy("a", &[]string{})))
	decision, _ := authorizer.CheckDecision(context.Background(), request)
	traced, _, _ := authorizer.CheckTrace(context.Background(), request)
	if decision != traced {
		t.Fatalf("CheckDecision %+v != CheckTrace %+v", decision, traced)
	}
}

func TestPolicyAuthorizerWithoutBase(t *testing.T) {
	authorizer := NewPolicyAuthorizer(nil)
	if _, err := authorizer.CheckDecision(context.Background(), Request{}); !errors.Is(err, ErrMissingAuthorizer) {
		t.Fatalf("err = %v", err)
	}
	results := authorizer.BatchCheckResults(context.Background(), []Request{{}})
	if !errors.Is(results[0].Err, ErrMissingAuthorizer) {
		t.Fatalf("batch err = %v", results[0].Err)
	}
}
