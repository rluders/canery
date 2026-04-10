package canery

import (
	"context"
	"errors"
	"testing"
)

type recordingMembershipReader struct {
	steps *[]string
	ok    bool
}

func (r recordingMembershipReader) HasMembership(context.Context, Subject, ScopeRef) (bool, error) {
	*r.steps = append(*r.steps, "membership")
	return r.ok, nil
}

type recordingGroupReader struct {
	steps  *[]string
	groups []GroupRef
}

func (r recordingGroupReader) GroupsForSubject(context.Context, Subject, ScopeRef) ([]GroupRef, error) {
	*r.steps = append(*r.steps, "groups")
	return append([]GroupRef(nil), r.groups...), nil
}

type recordingPermissionReader struct {
	steps []string
	seen  *[]string
}

func (r recordingPermissionReader) HasPermission(_ context.Context, principal PrincipalRef, request Request) (bool, error) {
	*r.seen = append(*r.seen, "permission:"+string(principal.Kind)+":"+string(request.Action))
	return false, nil
}

type recordingScopeResolver struct {
	steps *[]string
	ok    bool
}

func (r recordingScopeResolver) ResourceInScope(context.Context, ResourceRef, ScopeRef) (bool, error) {
	*r.steps = append(*r.steps, "resource_scope")
	return r.ok, nil
}

type stubMembershipReader struct {
	ok bool
}

func (s stubMembershipReader) HasMembership(context.Context, Subject, ScopeRef) (bool, error) {
	return s.ok, nil
}

type stubGroupReader struct {
	groups []GroupRef
}

func (s stubGroupReader) GroupsForSubject(context.Context, Subject, ScopeRef) ([]GroupRef, error) {
	return append([]GroupRef(nil), s.groups...), nil
}

type stubPermissionReader struct {
	allowed        map[PrincipalRef]bool
	allowedByCheck map[stubPermissionKey]bool
}

type stubPermissionKey struct {
	Principal PrincipalRef
	Action    Action
}

func (s stubPermissionReader) HasPermission(_ context.Context, principal PrincipalRef, request Request) (bool, error) {
	if s.allowedByCheck != nil {
		return s.allowedByCheck[stubPermissionKey{
			Principal: principal,
			Action:    request.Action,
		}], nil
	}
	return s.allowed[principal], nil
}

type stubScopeResolver struct {
	ok bool
}

func (s stubScopeResolver) ResourceInScope(context.Context, ResourceRef, ScopeRef) (bool, error) {
	return s.ok, nil
}

func TestEngineCheckAllowsSubjectPermission(t *testing.T) {
	engine := NewEngine(
		stubMembershipReader{ok: true},
		stubGroupReader{},
		stubPermissionReader{
			allowed: map[PrincipalRef]bool{
				SubjectPrincipal(Subject{Type: "user", ID: "user-1"}): true,
			},
		},
		stubScopeResolver{ok: true},
	)

	ok, err := engine.Check(context.Background(), Request{
		Subject:  Subject{Type: "user", ID: "user-1"},
		Action:   Action("delete"),
		Resource: ResourceRef{Type: "task", ID: "task-1"},
		Scope:    ScopeRef{Type: "workspace", ID: "workspace-1"},
	})
	if err != nil {
		t.Fatalf("expected no error: %v", err)
	}
	if !ok {
		t.Fatalf("expected request to be allowed")
	}
}

func TestEngineCheckDecisionAllowsSubjectPermission(t *testing.T) {
	engine := NewEngine(
		stubMembershipReader{ok: true},
		stubGroupReader{},
		stubPermissionReader{
			allowed: map[PrincipalRef]bool{
				SubjectPrincipal(Subject{Type: "user", ID: "user-1"}): true,
			},
		},
		stubScopeResolver{ok: true},
	)

	decision, err := engine.CheckDecision(context.Background(), Request{
		Subject:  Subject{Type: "user", ID: "user-1"},
		Action:   Action("delete"),
		Resource: ResourceRef{Type: "task", ID: "task-1"},
		Scope:    ScopeRef{Type: "workspace", ID: "workspace-1"},
	})
	if err != nil {
		t.Fatalf("expected no error: %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("expected decision to allow request")
	}
	if decision.Source != DecisionSourceDirect {
		t.Fatalf("expected direct source, got %q", decision.Source)
	}
	if decision.Reason != decisionReasonDirectPermission {
		t.Fatalf("expected direct permission reason, got %q", decision.Reason)
	}
}

func TestEngineCheckTraceRecordsDirectAllowFlow(t *testing.T) {
	engine := NewEngine(
		stubMembershipReader{ok: true},
		stubGroupReader{},
		stubPermissionReader{
			allowed: map[PrincipalRef]bool{
				SubjectPrincipal(Subject{Type: "user", ID: "user-1"}): true,
			},
		},
		stubScopeResolver{ok: true},
	)

	decision, trace, err := engine.CheckTrace(context.Background(), Request{
		Subject:  Subject{Type: "user", ID: "user-1"},
		Action:   Action("delete"),
		Resource: ResourceRef{Type: "task", ID: "task-1"},
		Scope:    ScopeRef{Type: "workspace", ID: "workspace-1"},
	})
	if err != nil {
		t.Fatalf("expected no error: %v", err)
	}
	if !decision.Allowed || decision.Source != DecisionSourceDirect {
		t.Fatalf("expected direct allow decision, got %+v", decision)
	}
	assertTraceSteps(t, trace, []TraceStep{
		{Name: traceStepRequestValidated, Result: "passed"},
		{Name: traceStepResourceScopeVerified, Result: "passed"},
		{Name: traceStepMembershipConfirmed, Result: "passed"},
		{Name: traceStepDirectChecked, Result: "matched"},
	})
}

func TestEngineCheckDelegatesToCheckDecision(t *testing.T) {
	engine := NewEngine(
		stubMembershipReader{ok: true},
		stubGroupReader{},
		stubPermissionReader{
			allowed: map[PrincipalRef]bool{
				SubjectPrincipal(Subject{Type: "user", ID: "user-1"}): true,
			},
		},
		stubScopeResolver{ok: true},
	)

	request := Request{
		Subject:  Subject{Type: "user", ID: "user-1"},
		Action:   Action("delete"),
		Resource: ResourceRef{Type: "task", ID: "task-1"},
		Scope:    ScopeRef{Type: "workspace", ID: "workspace-1"},
	}
	decision, err := engine.CheckDecision(context.Background(), request)
	if err != nil {
		t.Fatalf("expected no error from CheckDecision: %v", err)
	}
	allowed, err := engine.Check(context.Background(), request)
	if err != nil {
		t.Fatalf("expected no error from Check: %v", err)
	}
	if allowed != decision.Allowed {
		t.Fatalf("expected Check to delegate to CheckDecision, got allowed=%v decision=%+v", allowed, decision)
	}
}

func TestEngineDefaultPipelineOrderRemainsStable(t *testing.T) {
	steps := []string{}
	engine := NewEngine(
		recordingMembershipReader{steps: &steps, ok: true},
		recordingGroupReader{steps: &steps},
		recordingPermissionReader{seen: &steps},
		recordingScopeResolver{steps: &steps, ok: true},
	)

	decision, err := engine.CheckDecision(context.Background(), Request{
		Subject:  Actor("service", "svc-1"),
		Action:   Action("sync"),
		Resource: Resource("document", "doc-1"),
		Scope:    Scope("project", "project-1"),
	})
	if err != nil {
		t.Fatalf("expected no error: %v", err)
	}
	if decision.Allowed {
		t.Fatalf("expected denied decision from recording readers")
	}

	want := []string{
		"resource_scope",
		"membership",
		"permission:subject:sync",
		"groups",
	}
	if len(steps) != len(want) {
		t.Fatalf("unexpected steps length got=%v want=%v", steps, want)
	}
	for index := range want {
		if steps[index] != want[index] {
			t.Fatalf("unexpected pipeline order at %d: got=%q want=%q full=%v", index, steps[index], want[index], steps)
		}
	}
}

func TestEngineCheckAllowsGroupPermission(t *testing.T) {
	group := GroupRef{Type: "workspace_role", ID: "admin"}
	engine := NewEngine(
		stubMembershipReader{ok: true},
		stubGroupReader{groups: []GroupRef{group}},
		stubPermissionReader{
			allowed: map[PrincipalRef]bool{
				GroupPrincipal(group): true,
			},
		},
		stubScopeResolver{ok: true},
	)

	ok, err := engine.For(Subject{Type: "user", ID: "user-1"}).
		Can(Action("manage_settings")).
		On(ResourceRef{Type: "workspace_settings", ID: "workspace-1"}).
		Within(ScopeRef{Type: "workspace", ID: "workspace-1"}).
		Check(context.Background())
	if err != nil {
		t.Fatalf("expected no error: %v", err)
	}
	if !ok {
		t.Fatalf("expected request to be allowed via group")
	}
}

func TestEngineCheckDecisionAllowsGroupPermission(t *testing.T) {
	group := GroupRef{Type: "workspace_role", ID: "admin"}
	engine := NewEngine(
		stubMembershipReader{ok: true},
		stubGroupReader{groups: []GroupRef{group}},
		stubPermissionReader{
			allowed: map[PrincipalRef]bool{
				GroupPrincipal(group): true,
			},
		},
		stubScopeResolver{ok: true},
	)

	decision, err := engine.CheckDecision(context.Background(), Request{
		Subject:  Subject{Type: "user", ID: "user-1"},
		Action:   Action("manage_settings"),
		Resource: ResourceRef{Type: "workspace_settings", ID: "workspace-1"},
		Scope:    ScopeRef{Type: "workspace", ID: "workspace-1"},
	})
	if err != nil {
		t.Fatalf("expected no error: %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("expected decision to allow via group")
	}
	if decision.Source != DecisionSourceGroup {
		t.Fatalf("expected group source, got %q", decision.Source)
	}
	if decision.Reason != decisionReasonGroupPermission {
		t.Fatalf("expected group permission reason, got %q", decision.Reason)
	}
}

func TestEngineCheckTraceRecordsGroupAllowFlow(t *testing.T) {
	group := GroupRef{Type: "workspace_role", ID: "admin"}
	engine := NewEngine(
		stubMembershipReader{ok: true},
		stubGroupReader{groups: []GroupRef{group}},
		stubPermissionReader{
			allowed: map[PrincipalRef]bool{
				GroupPrincipal(group): true,
			},
		},
		stubScopeResolver{ok: true},
	)

	decision, trace, err := engine.CheckTrace(context.Background(), Request{
		Subject:  Subject{Type: "user", ID: "user-1"},
		Action:   Action("manage_settings"),
		Resource: ResourceRef{Type: "workspace_settings", ID: "workspace-1"},
		Scope:    ScopeRef{Type: "workspace", ID: "workspace-1"},
	})
	if err != nil {
		t.Fatalf("expected no error: %v", err)
	}
	if !decision.Allowed || decision.Source != DecisionSourceGroup {
		t.Fatalf("expected group allow decision, got %+v", decision)
	}
	assertTraceSteps(t, trace, []TraceStep{
		{Name: traceStepRequestValidated, Result: "passed"},
		{Name: traceStepResourceScopeVerified, Result: "passed"},
		{Name: traceStepMembershipConfirmed, Result: "passed"},
		{Name: traceStepDirectChecked, Result: "not_matched"},
		{Name: traceStepGroupsResolved, Result: "resolved"},
		{Name: traceStepGroupChecked, Result: "matched"},
	})
}

func TestBuilderRequestMatchesDirectRequest(t *testing.T) {
	engine := NewEngine(nil, nil, nil, nil)
	request := engine.For(ActorRef{Type: "user", ID: "user-1"}).
		Can(Action("view")).
		Target(ResourceRef{Type: "task", ID: "task-1"}).
		In(ScopeRef{Type: "workspace", ID: "workspace-1"}).
		Request()

	if request.Subject.ID != "user-1" || request.Action != Action("view") || request.Resource.ID != "task-1" || request.Scope.ID != "workspace-1" {
		t.Fatalf("unexpected request %+v", request)
	}
}

func TestActorRefAliasMatchesSubject(t *testing.T) {
	var actor ActorRef = Subject{Type: "user", ID: "user-1"}
	if actor.Type != "user" || actor.ID != "user-1" {
		t.Fatalf("unexpected actor ref %+v", actor)
	}
}

func TestMultiActionBuilderBuildsRequests(t *testing.T) {
	engine := NewEngine(nil, nil, nil, nil)
	requests := engine.For(User("user-1")).
		CanMany(Action("view"), Action("update")).
		On(Resource("task", "task-1")).
		Within(Scope("workspace", "workspace-1")).
		Requests()

	if len(requests) != 2 {
		t.Fatalf("expected two requests, got %d", len(requests))
	}
	if requests[0].Action != Action("view") || requests[1].Action != Action("update") {
		t.Fatalf("unexpected multi-action requests %+v", requests)
	}
	if requests[0].Resource.ID != "task-1" || requests[1].Scope.ID != "workspace-1" {
		t.Fatalf("unexpected shared request context %+v", requests)
	}
}

func TestMultiActionBuilderCheckReturnsPerActionDecision(t *testing.T) {
	group := GroupRef{Type: "workspace_role", ID: "admin"}
	engine := NewEngine(
		stubMembershipReader{ok: true},
		stubGroupReader{groups: []GroupRef{group}},
		stubPermissionReader{
			allowed: map[PrincipalRef]bool{
				GroupPrincipal(group): true,
			},
		},
		stubScopeResolver{ok: true},
	)

	result, err := engine.For(User("user-1")).
		CanMany(Action("view"), Action("update")).
		On(Resource("task", "task-1")).
		Within(Scope("workspace", "workspace-1")).
		Check(context.Background())
	if err != nil {
		t.Fatalf("expected no error: %v", err)
	}

	if allowed, ok := result.Allowed(Action("view")); !ok || !allowed {
		t.Fatalf("expected view to be allowed, got ok=%v allowed=%v", ok, allowed)
	}
	if allowed, ok := result.Allowed(Action("update")); !ok || !allowed {
		t.Fatalf("expected update to be allowed, got ok=%v allowed=%v", ok, allowed)
	}
}

func TestMultiActionBuilderCapturesPerActionErrors(t *testing.T) {
	engine := NewEngine(stubMembershipReader{ok: true}, stubGroupReader{}, stubPermissionReader{}, stubScopeResolver{ok: true})

	result, err := engine.For(User("user-1")).
		CanMany(Action("view"), Action("")).
		On(Resource("task", "task-1")).
		Within(Scope("workspace", "workspace-1")).
		Check(context.Background())
	if err != nil {
		t.Fatalf("expected no top-level error: %v", err)
	}

	if allowed, ok := result.Allowed(Action("view")); !ok || allowed {
		t.Fatalf("expected view to be denied without error-producing permissions, got ok=%v allowed=%v", ok, allowed)
	}
	if actionErr, ok := result.Error(Action("")); !ok || !errors.Is(actionErr, ErrInvalidRequest) {
		t.Fatalf("expected invalid request error for empty action, got ok=%v err=%v", ok, actionErr)
	}
}

func TestHelpersBuildExpectedRefs(t *testing.T) {
	subject := User("user-1")
	if subject.Type != "user" || subject.ID != "user-1" {
		t.Fatalf("unexpected user subject %+v", subject)
	}

	actor := Actor("service_account", "svc-1")
	if actor.Type != "service_account" || actor.ID != "svc-1" {
		t.Fatalf("unexpected actor subject %+v", actor)
	}

	resource := Resource("task", "task-1")
	if resource.Type != "task" || resource.ID != "task-1" {
		t.Fatalf("unexpected resource %+v", resource)
	}

	scope := Scope("workspace", "workspace-1")
	if scope.Type != "workspace" || scope.ID != "workspace-1" {
		t.Fatalf("unexpected scope %+v", scope)
	}
}

func TestHelpersWorkWithBuilderRequest(t *testing.T) {
	engine := NewEngine(nil, nil, nil, nil)
	request := engine.For(User("user-1")).
		Can(Action("view")).
		On(Resource("task", "task-1")).
		Within(Scope("workspace", "workspace-1")).
		Request()

	if request.Subject.Type != "user" || request.Subject.ID != "user-1" {
		t.Fatalf("unexpected request subject %+v", request.Subject)
	}
	if request.Resource.Type != "task" || request.Scope.Type != "workspace" {
		t.Fatalf("unexpected helper-built request %+v", request)
	}
}

func TestEngineCheckRejectsInvalidRequest(t *testing.T) {
	engine := NewEngine(stubMembershipReader{ok: true}, stubGroupReader{}, stubPermissionReader{}, stubScopeResolver{ok: true})

	_, err := engine.Check(context.Background(), Request{})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected invalid request error, got %v", err)
	}
	var validationErr ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected validation error, got %T", err)
	}
	if validationErr.Field != "subject" || validationErr.Code != "required" {
		t.Fatalf("unexpected validation error %+v", validationErr)
	}
}

func TestEngineCheckReturnsValidationErrorForInvalidAction(t *testing.T) {
	engine := NewEngine(stubMembershipReader{ok: true}, stubGroupReader{}, stubPermissionReader{}, stubScopeResolver{ok: true})

	_, err := engine.Check(context.Background(), Request{
		Subject:  User("user-1"),
		Action:   Action(""),
		Resource: Resource("task", "task-1"),
		Scope:    Scope("workspace", "workspace-1"),
	})
	assertValidationError(t, err, "action", "required")
}

func TestEngineCheckReturnsValidationErrorForInvalidResource(t *testing.T) {
	engine := NewEngine(stubMembershipReader{ok: true}, stubGroupReader{}, stubPermissionReader{}, stubScopeResolver{ok: true})

	_, err := engine.Check(context.Background(), Request{
		Subject:  User("user-1"),
		Action:   Action("view"),
		Resource: ResourceRef{},
		Scope:    Scope("workspace", "workspace-1"),
	})
	assertValidationError(t, err, "resource", "required")
}

func TestEngineCheckReturnsValidationErrorForInvalidScope(t *testing.T) {
	engine := NewEngine(stubMembershipReader{ok: true}, stubGroupReader{}, stubPermissionReader{}, stubScopeResolver{ok: true})

	_, err := engine.Check(context.Background(), Request{
		Subject:  User("user-1"),
		Action:   Action("view"),
		Resource: Resource("task", "task-1"),
		Scope:    ScopeRef{Type: "workspace"},
	})
	assertValidationError(t, err, "scope", "required")
}

func TestEngineCheckAllowsCreateStyleRequestWithEmptyResourceID(t *testing.T) {
	engine := NewEngine(
		stubMembershipReader{ok: true},
		stubGroupReader{},
		stubPermissionReader{
			allowed: map[PrincipalRef]bool{
				SubjectPrincipal(User("user-1")): true,
			},
		},
		nil,
	)

	ok, err := engine.Check(context.Background(), Request{
		Subject:  User("user-1"),
		Action:   Action("create"),
		Resource: ResourceRef{Type: "task"},
		Scope:    Scope("workspace", "workspace-1"),
	})
	if err != nil {
		t.Fatalf("expected no error: %v", err)
	}
	if !ok {
		t.Fatalf("expected create-style request to remain valid")
	}
}

func TestEngineCheckRejectsResourceOutsideScope(t *testing.T) {
	engine := NewEngine(
		stubMembershipReader{ok: true},
		stubGroupReader{},
		stubPermissionReader{},
		stubScopeResolver{ok: false},
	)

	ok, err := engine.Check(context.Background(), Request{
		Subject:  Subject{Type: "user", ID: "user-1"},
		Action:   Action("view"),
		Resource: ResourceRef{Type: "task", ID: "task-1"},
		Scope:    ScopeRef{Type: "workspace", ID: "workspace-1"},
	})
	if err != nil {
		t.Fatalf("expected no error: %v", err)
	}
	if ok {
		t.Fatalf("expected request to be denied")
	}
}

func TestEngineCheckDecisionReturnsDeniedMetadata(t *testing.T) {
	engine := NewEngine(
		stubMembershipReader{ok: true},
		stubGroupReader{},
		stubPermissionReader{},
		stubScopeResolver{ok: true},
	)

	decision, err := engine.CheckDecision(context.Background(), Request{
		Subject:  Subject{Type: "user", ID: "user-1"},
		Action:   Action("delete"),
		Resource: ResourceRef{Type: "task", ID: "task-1"},
		Scope:    ScopeRef{Type: "workspace", ID: "workspace-1"},
	})
	if err != nil {
		t.Fatalf("expected no error: %v", err)
	}
	if decision.Allowed {
		t.Fatalf("expected denied decision")
	}
	if decision.Source != DecisionSourceNone {
		t.Fatalf("expected none source, got %q", decision.Source)
	}
	if decision.Reason != decisionReasonNoPermission {
		t.Fatalf("expected no permission reason, got %q", decision.Reason)
	}
}

func TestEngineCheckTraceRecordsDenyFlow(t *testing.T) {
	engine := NewEngine(
		stubMembershipReader{ok: true},
		stubGroupReader{},
		stubPermissionReader{},
		stubScopeResolver{ok: true},
	)

	decision, trace, err := engine.CheckTrace(context.Background(), Request{
		Subject:  Subject{Type: "user", ID: "user-1"},
		Action:   Action("delete"),
		Resource: ResourceRef{Type: "task", ID: "task-1"},
		Scope:    ScopeRef{Type: "workspace", ID: "workspace-1"},
	})
	if err != nil {
		t.Fatalf("expected no error: %v", err)
	}
	if decision.Allowed || decision.Source != DecisionSourceNone {
		t.Fatalf("expected denied decision, got %+v", decision)
	}
	assertTraceSteps(t, trace, []TraceStep{
		{Name: traceStepRequestValidated, Result: "passed"},
		{Name: traceStepResourceScopeVerified, Result: "passed"},
		{Name: traceStepMembershipConfirmed, Result: "passed"},
		{Name: traceStepDirectChecked, Result: "not_matched"},
		{Name: traceStepGroupsResolved, Result: "none"},
		{Name: traceStepGroupChecked, Result: "not_matched"},
	})
}

func TestEngineBatchCheckReturnsDecisions(t *testing.T) {
	subjectPrincipal := SubjectPrincipal(User("user-1"))
	engine := NewEngine(
		stubMembershipReader{ok: true},
		stubGroupReader{},
		stubPermissionReader{
			allowedByCheck: map[stubPermissionKey]bool{
				{Principal: subjectPrincipal, Action: Action("view")}:   true,
				{Principal: subjectPrincipal, Action: Action("delete")}: false,
			},
		},
		stubScopeResolver{ok: true},
	)

	decisions, err := engine.BatchCheck(context.Background(), []Request{
		{
			Subject:  User("user-1"),
			Action:   Action("view"),
			Resource: Resource("task", "task-1"),
			Scope:    Scope("workspace", "workspace-1"),
		},
		{
			Subject:  User("user-1"),
			Action:   Action("delete"),
			Resource: Resource("task", "task-1"),
			Scope:    Scope("workspace", "workspace-1"),
		},
	})
	if err != nil {
		t.Fatalf("expected no error: %v", err)
	}
	if len(decisions) != 2 {
		t.Fatalf("expected two decisions, got %d", len(decisions))
	}
	if !decisions[0].Allowed || decisions[1].Allowed {
		t.Fatalf("unexpected batch decisions %+v", decisions)
	}
}

func TestEngineBatchCheckReturnsStructuredInvalidRequestError(t *testing.T) {
	engine := NewEngine(stubMembershipReader{ok: true}, stubGroupReader{}, stubPermissionReader{}, stubScopeResolver{ok: true})

	_, err := engine.BatchCheck(context.Background(), []Request{
		{
			Subject:  User("user-1"),
			Action:   Action("view"),
			Resource: Resource("task", "task-1"),
			Scope:    Scope("workspace", "workspace-1"),
		},
		{
			Subject:  User("user-1"),
			Action:   Action(""),
			Resource: Resource("task", "task-1"),
			Scope:    Scope("workspace", "workspace-1"),
		},
	})
	var batchErr BatchCheckError
	if !errors.As(err, &batchErr) {
		t.Fatalf("expected batch check error, got %v", err)
	}
	if batchErr.Index != 1 {
		t.Fatalf("expected failing index 1, got %d", batchErr.Index)
	}
	if !errors.Is(batchErr.Err, ErrInvalidRequest) {
		t.Fatalf("expected invalid request cause, got %v", batchErr.Err)
	}
}

func TestEngineBatchCheckHandlesMixedOutcomes(t *testing.T) {
	group := GroupRef{Type: "workspace_role", ID: "admin"}
	groupPrincipal := GroupPrincipal(group)
	engine := NewEngine(
		stubMembershipReader{ok: true},
		stubGroupReader{groups: []GroupRef{group}},
		stubPermissionReader{
			allowedByCheck: map[stubPermissionKey]bool{
				{Principal: groupPrincipal, Action: Action("view")}:   true,
				{Principal: groupPrincipal, Action: Action("update")}: false,
			},
		},
		stubScopeResolver{ok: true},
	)

	decisions, err := engine.BatchCheck(context.Background(), []Request{
		{
			Subject:  User("user-1"),
			Action:   Action("view"),
			Resource: Resource("task", "task-1"),
			Scope:    Scope("workspace", "workspace-1"),
		},
		{
			Subject:  User("user-1"),
			Action:   Action("update"),
			Resource: Resource("task", "task-1"),
			Scope:    Scope("workspace", "workspace-1"),
		},
	})
	if err != nil {
		t.Fatalf("expected no error: %v", err)
	}
	if len(decisions) != 2 {
		t.Fatalf("expected two decisions, got %d", len(decisions))
	}
	if !decisions[0].Allowed || decisions[1].Allowed {
		t.Fatalf("expected mixed outcomes, got %+v", decisions)
	}
}

func assertTraceSteps(t *testing.T, trace Trace, want []TraceStep) {
	t.Helper()
	if len(trace.Steps) != len(want) {
		t.Fatalf("expected %d trace steps, got %d: %+v", len(want), len(trace.Steps), trace.Steps)
	}
	for index := range want {
		if trace.Steps[index] != want[index] {
			t.Fatalf("unexpected trace step at index %d: got %+v want %+v", index, trace.Steps[index], want[index])
		}
	}
}

func assertValidationError(t *testing.T, err error, field string, code string) {
	t.Helper()
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected invalid request error, got %v", err)
	}
	var validationErr ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected validation error, got %T", err)
	}
	if validationErr.Field != field || validationErr.Code != code {
		t.Fatalf("unexpected validation error %+v", validationErr)
	}
	if validationErr.Error() != "canery: invalid request: "+field+": "+code {
		t.Fatalf("unexpected validation error string %q", validationErr.Error())
	}
}
