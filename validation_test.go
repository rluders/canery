package canery

import (
	"context"
	"errors"
	"testing"
)

func TestValidationErrorsAreGranular(t *testing.T) {
	valid := Request{Subject: User("u1"), Action: "read", Resource: Resource("doc", "d1"), Scope: Scope("project", "p1")}
	cases := []struct {
		name     string
		mutate   func(*Request)
		field    string
		property string
	}{
		{name: "subject type", mutate: func(r *Request) { r.Subject.Type = "" }, field: "subject", property: "type"},
		{name: "subject id", mutate: func(r *Request) { r.Subject.ID = "" }, field: "subject", property: "id"},
		{name: "subject id whitespace", mutate: func(r *Request) { r.Subject.ID = " \t" }, field: "subject", property: "id"},
		{name: "action", mutate: func(r *Request) { r.Action = " " }, field: "action"},
		{name: "resource type", mutate: func(r *Request) { r.Resource.Type = "" }, field: "resource", property: "type"},
		{name: "scope type", mutate: func(r *Request) { r.Scope.Type = "" }, field: "scope", property: "type"},
		{name: "scope id", mutate: func(r *Request) { r.Scope.ID = "" }, field: "scope", property: "id"},
		{name: "first failure wins", mutate: func(r *Request) { r.Subject.ID = ""; r.Scope = ScopeRef{} }, field: "subject", property: "id"},
		{name: "empty request", mutate: func(r *Request) { *r = Request{} }, field: "subject", property: "type"},
	}
	readers := newCountingReaders()
	engine := readers.engine()
	policy := NewPolicyAuthorizer(engine)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := valid
			tc.mutate(&request)
			ctx := context.Background()

			_, err := engine.Check(ctx, request)
			assertValidationError(t, err, tc.field, tc.property, ValidationCodeRequired)
			_, err = engine.CheckDecision(ctx, request)
			assertValidationError(t, err, tc.field, tc.property, ValidationCodeRequired)
			_, trace, err := engine.CheckTrace(ctx, request)
			assertValidationError(t, err, tc.field, tc.property, ValidationCodeRequired)
			assertTraceSteps(t, trace, []TraceStep{{Name: TraceStepRequestValidated, Result: TraceResultFailed}})
			_, err = engine.For(request.Subject).Can(request.Action).On(request.Resource).In(request.Scope).Check(ctx)
			assertValidationError(t, err, tc.field, tc.property, ValidationCodeRequired)
			_, err = policy.CheckDecision(ctx, request)
			assertValidationError(t, err, tc.field, tc.property, ValidationCodeRequired)

			_, err = engine.BatchCheck(ctx, []Request{valid, request})
			var batchErr BatchCheckError
			if !errors.As(err, &batchErr) || batchErr.Index != 1 {
				t.Fatalf("batch err = %v", err)
			}
			assertValidationError(t, batchErr.Err, tc.field, tc.property, ValidationCodeRequired)
			assertValidationError(t, engine.BatchCheckResults(ctx, []Request{request})[0].Err, tc.field, tc.property, ValidationCodeRequired)
		})
	}
	if readers.membershipCalls != 0 {
		t.Fatalf("readers called for invalid requests")
	}
}

func TestResourceIDIsOptional(t *testing.T) {
	request := Request{Subject: User("u1"), Action: "create", Resource: ResourceRef{Type: "doc"}, Scope: Scope("project", "p1")}
	if err := validateRequest(request); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}
