package canery

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

var (
	batchUser    = User("user-1")
	batchOther   = User("user-2")
	batchProject = Scope("project", "p1")
	batchEditors = GroupRef{Type: "team", ID: "editors"}
	batchViewers = GroupRef{Type: "team", ID: "viewers"}
)

func batchRequest(subject Subject, action Action, resource ResourceRef) Request {
	return Request{Subject: subject, Action: action, Resource: resource, Scope: batchProject}
}

// batchFixture returns readers with a mix of direct, group, and missing grants.
func batchFixture() *countingReaders {
	readers := newCountingReaders()
	readers.member(batchUser, batchProject, batchViewers, batchEditors)
	readers.grant(SubjectPrincipal(batchUser), "view", Resource("doc", "d1"))
	readers.grant(GroupPrincipal(batchEditors), "edit", Resource("doc", "d1"))
	readers.grant(GroupPrincipal(batchViewers), "view", Resource("doc", "d2"))
	readers.outOfScope[Resource("doc", "foreign")] = true
	return readers
}

func batchFixtureRequests() []Request {
	return []Request{
		batchRequest(batchUser, "view", Resource("doc", "d1")),      // direct allow
		batchRequest(batchUser, "edit", Resource("doc", "d1")),      // group allow (editors)
		batchRequest(batchUser, "view", Resource("doc", "d2")),      // group allow (viewers)
		batchRequest(batchUser, "delete", Resource("doc", "d1")),    // no permission
		batchRequest(batchOther, "view", Resource("doc", "d1")),     // not a member
		batchRequest(batchUser, "view", Resource("doc", "foreign")), // outside scope
		batchRequest(batchUser, "view", Resource("doc", "d1")),      // duplicate of 0
		batchRequest(batchUser, "create", Resource("doc", "")),      // type-level, no permission
	}
}

func TestBatchCheckResultsMatchIndependentChecks(t *testing.T) {
	for _, withBatchReader := range []bool{false, true} {
		t.Run(fmt.Sprintf("batchReader=%v", withBatchReader), func(t *testing.T) {
			readers := batchFixture()
			engine := readers.engine()
			if withBatchReader {
				engine = batchReaders{readers}.engine()
			}
			requests := batchFixtureRequests()
			results := engine.BatchCheckResults(context.Background(), requests)
			if len(results) != len(requests) {
				t.Fatalf("got %d results for %d requests", len(results), len(requests))
			}

			independent := batchFixture().engine()
			for index, request := range requests {
				want, wantErr := independent.CheckDecision(context.Background(), request)
				got := results[index]
				if got.Request != request {
					t.Fatalf("result %d is associated with %+v, want %+v", index, got.Request, request)
				}
				if got.Decision != want || !errors.Is(got.Err, wantErr) {
					t.Fatalf("result %d = %+v / %v, want %+v / %v", index, got.Decision, got.Err, want, wantErr)
				}
			}

			decisions, err := engine.BatchCheck(context.Background(), requests)
			if err != nil {
				t.Fatalf("BatchCheck: %v", err)
			}
			for index := range decisions {
				if decisions[index] != results[index].Decision {
					t.Fatalf("BatchCheck[%d] = %+v, want %+v", index, decisions[index], results[index].Decision)
				}
			}
		})
	}
}

func TestBatchCheckResultsDeduplicateSharedLookups(t *testing.T) {
	readers := batchFixture()
	readers.engine().BatchCheckResults(context.Background(), batchFixtureRequests())

	// Two distinct subject/scope pairs, six distinct concrete resources.
	if readers.membershipCalls != 2 {
		t.Fatalf("membership calls = %d, want 2", readers.membershipCalls)
	}
	if readers.resolverCalls != 3 {
		t.Fatalf("resolver calls = %d, want 3 (d1, d2, foreign)", readers.resolverCalls)
	}
	if readers.groupCalls != 1 {
		t.Fatalf("group calls = %d, want 1", readers.groupCalls)
	}
	// Direct: view:d1, edit:d1, view:d2, delete:d1, create:doc = 5 distinct.
	// Groups (viewers then editors, stop at first match):
	//   edit:d1   -> viewers(no), editors(yes)  = 2
	//   view:d2   -> viewers(yes)               = 1
	//   delete:d1 -> viewers(no), editors(no)   = 2
	//   create    -> viewers(no), editors(no)   = 2
	if readers.permissionCalls != 12 {
		t.Fatalf("permission calls = %d, want 12", readers.permissionCalls)
	}
}

func TestBatchPermissionReaderReplacesPerCheckCalls(t *testing.T) {
	readers := batchFixture()
	engine := batchReaders{readers}.engine()
	engine.BatchCheckResults(context.Background(), batchFixtureRequests())

	if readers.permissionCalls != 0 {
		t.Fatalf("single permission calls = %d, want 0", readers.permissionCalls)
	}
	// One call for direct checks, one for every group check of the run.
	if readers.batchCalls != 2 {
		t.Fatalf("batch calls = %d, want 2 (sizes %v)", readers.batchCalls, readers.batchSizes)
	}
	if readers.batchSizes[0] != 5 || readers.batchSizes[1] != 8 {
		t.Fatalf("batch sizes = %v, want [5 8]", readers.batchSizes)
	}
}

func TestBatchPermissionReaderErrorsFailOnlyDependentItems(t *testing.T) {
	readers := batchFixture()
	readers.batchErr = errors.New("db down")
	engine := batchReaders{readers}.engine()
	results := engine.BatchCheckResults(context.Background(), batchFixtureRequests())

	for index, result := range results {
		switch index {
		case 4, 5: // decided before permissions were needed
			if result.Err != nil || result.Decision.Allowed {
				t.Fatalf("result %d = %+v, want denial without error", index, result)
			}
		default:
			if !errors.Is(result.Err, readers.batchErr) {
				t.Fatalf("result %d err = %v, want db down", index, result.Err)
			}
			if result.Decision != (Decision{}) {
				t.Fatalf("failed result %d carries decision %+v", index, result.Decision)
			}
		}
	}
}

func TestBatchPermissionReaderResultMismatch(t *testing.T) {
	readers := batchFixture()
	readers.batchShort = true
	engine := batchReaders{readers}.engine()
	_, err := engine.BatchCheck(context.Background(), batchFixtureRequests())
	if !errors.Is(err, ErrBatchResultMismatch) {
		t.Fatalf("err = %v, want ErrBatchResultMismatch", err)
	}
}

func TestBatchPermissionReaderPerItemError(t *testing.T) {
	readers := batchFixture()
	readers.permissionErr[GroupPrincipal(batchEditors)] = errors.New("editors shard down")
	engine := batchReaders{readers}.engine()
	results := engine.BatchCheckResults(context.Background(), batchFixtureRequests())

	// view:d2 matches viewers before editors is consulted, so it still allows.
	if results[2].Err != nil || !results[2].Decision.Allowed {
		t.Fatalf("view:d2 = %+v, want allow", results[2])
	}
	// edit:d1 reaches editors and must fail rather than deny.
	if !errors.Is(results[1].Err, readers.permissionErr[GroupPrincipal(batchEditors)]) {
		t.Fatalf("edit:d1 err = %v, want shard error", results[1].Err)
	}
}

func TestBatchCheckIsAllOrNothing(t *testing.T) {
	readers := batchFixture()
	readers.permissionErr[GroupPrincipal(batchViewers)] = errors.New("boom")
	requests := batchFixtureRequests()

	decisions, err := readers.engine().BatchCheck(context.Background(), requests)
	if decisions != nil {
		t.Fatalf("expected no decisions on failure, got %+v", decisions)
	}
	var batchErr BatchCheckError
	if !errors.As(err, &batchErr) || batchErr.Index != 1 {
		t.Fatalf("err = %v, want BatchCheckError at index 1", err)
	}
	var evalErr EvaluationError
	if !errors.As(err, &evalErr) || evalErr.Stage != TraceStepGroupChecked {
		t.Fatalf("err = %v, want EvaluationError at group stage", err)
	}
}

func TestBatchCheckValidatesBeforeCallingReaders(t *testing.T) {
	readers := batchFixture()
	requests := batchFixtureRequests()
	requests[3].Scope.ID = " "

	_, err := readers.engine().BatchCheck(context.Background(), requests)
	var batchErr BatchCheckError
	if !errors.As(err, &batchErr) || batchErr.Index != 3 {
		t.Fatalf("err = %v, want BatchCheckError at index 3", err)
	}
	assertValidationError(t, batchErr.Err, "scope", "id", ValidationCodeRequired)
	if total := readers.membershipCalls + readers.permissionCalls + readers.resolverCalls + readers.groupCalls; total != 0 {
		t.Fatalf("readers were called %d times before validation failed", total)
	}
}

func TestBatchCheckResultsKeepPartialResults(t *testing.T) {
	readers := batchFixture()
	requests := batchFixtureRequests()
	requests[2].Action = ""
	requests[3].Subject.ID = ""

	results := readers.engine().BatchCheckResults(context.Background(), requests)
	assertValidationError(t, results[2].Err, "action", "", ValidationCodeRequired)
	assertValidationError(t, results[3].Err, "subject", "id", ValidationCodeRequired)
	if results[0].Err != nil || !results[0].Decision.Allowed {
		t.Fatalf("result 0 = %+v, want allow", results[0])
	}
	if results[4].Err != nil || results[4].Decision.Allowed || results[4].Decision.Reason != decisionReasonMembership {
		t.Fatalf("result 4 = %+v, want membership denial", results[4])
	}
}

func TestBatchCheckEmpty(t *testing.T) {
	decisions, err := batchFixture().engine().BatchCheck(context.Background(), nil)
	if err != nil || len(decisions) != 0 {
		t.Fatalf("empty batch = %v, %v", decisions, err)
	}
}

func TestBatchCancellationIsNotADenial(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	readers := batchFixture()
	results := readers.engine().BatchCheckResults(ctx, batchFixtureRequests())
	for index, result := range results {
		if !errors.Is(result.Err, context.Canceled) {
			t.Fatalf("result %d err = %v, want context.Canceled", index, result.Err)
		}
	}
	if readers.membershipCalls+readers.resolverCalls != 0 {
		t.Fatalf("readers called after cancellation")
	}

	if _, err := readers.engine().Check(ctx, batchFixtureRequests()[0]); !errors.Is(err, context.Canceled) {
		t.Fatalf("single check err = %v, want context.Canceled", err)
	}
}

// cancellingReaders cancels the context during the membership stage.
type cancellingReaders struct {
	*countingReaders
	cancel context.CancelFunc
}

func (c cancellingReaders) HasMembership(ctx context.Context, subject Subject, scope ScopeRef) (bool, error) {
	c.cancel()
	return c.countingReaders.HasMembership(ctx, subject, scope)
}

func TestBatchCancellationDuringEvaluation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	readers := cancellingReaders{countingReaders: batchFixture(), cancel: cancel}
	engine := NewEngine(readers, readers, readers, readers)

	results := engine.BatchCheckResults(ctx, batchFixtureRequests())
	for index, result := range results {
		if index == 5 { // decided (outside scope) before cancellation
			if result.Err != nil {
				t.Fatalf("result 5 err = %v", result.Err)
			}
			continue
		}
		if !errors.Is(result.Err, context.Canceled) {
			t.Fatalf("result %d = %+v, want context.Canceled", index, result)
		}
	}
	if readers.permissionCalls != 0 {
		t.Fatalf("permission reader called after cancellation")
	}
}

func TestSingleCheckUsesBatchReaderForGroups(t *testing.T) {
	readers := newCountingReaders()
	groups := []GroupRef{{Type: "team", ID: "a"}, {Type: "team", ID: "b"}, {Type: "team", ID: "c"}}
	readers.member(batchUser, batchProject, groups...)
	readers.grant(GroupPrincipal(groups[2]), "view", Resource("doc", "d1"))
	engine := batchReaders{readers}.engine()

	decision, err := engine.CheckDecision(context.Background(), batchRequest(batchUser, "view", Resource("doc", "d1")))
	if err != nil || !decision.Allowed {
		t.Fatalf("decision = %+v, %v", decision, err)
	}
	if principal, _ := decision.EffectivePrincipal(); principal != GroupPrincipal(groups[2]) {
		t.Fatalf("effective principal = %+v", principal)
	}
	// Direct check: one distinct check -> HasPermission. Groups: one batch call.
	if readers.permissionCalls != 1 || readers.batchCalls != 1 || readers.batchSizes[0] != 3 {
		t.Fatalf("calls: single=%d batch=%d sizes=%v", readers.permissionCalls, readers.batchCalls, readers.batchSizes)
	}
}

func TestGroupEvaluationStopsAtFirstMatchWithoutBatchReader(t *testing.T) {
	cases := []struct {
		name      string
		match     int // index of the matching group, -1 for none
		wantCalls int // permission calls including the direct check
	}{
		{name: "first group", match: 0, wantCalls: 2},
		{name: "later group", match: 2, wantCalls: 4},
		{name: "no group", match: -1, wantCalls: 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			readers := newCountingReaders()
			groups := []GroupRef{{Type: "team", ID: "a"}, {Type: "team", ID: "b"}, {Type: "team", ID: "c"}}
			readers.member(batchUser, batchProject, groups...)
			if tc.match >= 0 {
				readers.grant(GroupPrincipal(groups[tc.match]), "view", Resource("doc", "d1"))
			}
			decision, err := readers.engine().CheckDecision(context.Background(), batchRequest(batchUser, "view", Resource("doc", "d1")))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if decision.Allowed != (tc.match >= 0) {
				t.Fatalf("decision = %+v", decision)
			}
			if readers.permissionCalls != tc.wantCalls {
				t.Fatalf("permission calls = %d, want %d", readers.permissionCalls, tc.wantCalls)
			}
		})
	}
}

func TestBatchSharesGroupChecksAcrossRequests(t *testing.T) {
	readers := newCountingReaders()
	readers.member(batchUser, batchProject, batchViewers)
	readers.grant(GroupPrincipal(batchViewers), "view", Resource("doc", "d1"))
	request := batchRequest(batchUser, "view", Resource("doc", "d1"))

	readers.engine().BatchCheckResults(context.Background(), []Request{request, request, request})
	// One direct and one group check, despite three identical requests.
	if readers.permissionCalls != 2 || readers.groupCalls != 1 || readers.membershipCalls != 1 {
		t.Fatalf("calls: permission=%d group=%d membership=%d", readers.permissionCalls, readers.groupCalls, readers.membershipCalls)
	}
}

func TestMultiActionBuilderUsesBatchEvaluation(t *testing.T) {
	readers := batchFixture()
	result, err := readers.engine().For(batchUser).
		CanMany("view", "edit", "delete").
		Target(Resource("doc", "d1")).
		In(batchProject).
		Check(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for action, want := range map[Action]bool{"view": true, "edit": true, "delete": false} {
		got, ok := result.Allowed(action)
		if !ok || got != want {
			t.Fatalf("%s allowed = %v (present %v), want %v", action, got, ok, want)
		}
	}
	if result.Decisions["edit"].Decision.Source != DecisionSourceGroup {
		t.Fatalf("edit decision = %+v", result.Decisions["edit"].Decision)
	}
	if readers.membershipCalls != 1 || readers.resolverCalls != 1 || readers.groupCalls != 1 {
		t.Fatalf("multi-action lookups were not shared: membership=%d resolver=%d groups=%d",
			readers.membershipCalls, readers.resolverCalls, readers.groupCalls)
	}
}

func TestPolicyAuthorizerBatchCheckResults(t *testing.T) {
	readers := batchFixture()
	denyDelete := ForAction("delete", PolicyFunc(func(context.Context, Request, DecisionEvaluator) (Decision, error) {
		return Decision{Allowed: false, Reason: "deletes frozen", Source: DecisionSourceNone}, nil
	}))
	authorizer := NewPolicyAuthorizer(readers.engine(), denyDelete)
	requests := batchFixtureRequests()

	results := authorizer.BatchCheckResults(context.Background(), requests)
	for index, request := range requests {
		want, wantErr := authorizer.CheckDecision(context.Background(), request)
		if results[index].Decision != want || !errors.Is(results[index].Err, wantErr) || results[index].Request != request {
			t.Fatalf("result %d = %+v, want %+v / %v", index, results[index], want, wantErr)
		}
	}
	if results[3].Decision.Reason != "deletes frozen" {
		t.Fatalf("policy did not run for delete: %+v", results[3])
	}

	decisions, err := authorizer.BatchCheck(context.Background(), requests)
	if err != nil || len(decisions) != len(requests) {
		t.Fatalf("BatchCheck = %v, %v", decisions, err)
	}
}

func BenchmarkGroupEvaluation(b *testing.B) {
	for _, groupCount := range []int{1, 10, 100} {
		readers := newCountingReaders()
		groups := make([]GroupRef, groupCount)
		for index := range groups {
			groups[index] = GroupRef{Type: "team", ID: fmt.Sprint(index)}
		}
		readers.member(batchUser, batchProject, groups...)
		readers.grant(GroupPrincipal(groups[groupCount-1]), "view", Resource("doc", "d1"))
		request := batchRequest(batchUser, "view", Resource("doc", "d1"))

		for _, variant := range []struct {
			name   string
			engine *Engine
		}{
			{name: "single", engine: readers.engine()},
			{name: "batch", engine: batchReaders{readers}.engine()},
		} {
			b.Run(fmt.Sprintf("groups=%d/%s", groupCount, variant.name), func(b *testing.B) {
				readers.permissionCalls, readers.batchCalls = 0, 0
				for i := 0; i < b.N; i++ {
					if _, err := variant.engine.CheckDecision(context.Background(), request); err != nil {
						b.Fatal(err)
					}
				}
				b.ReportMetric(float64(readers.permissionCalls+readers.batchCalls)/float64(b.N), "reader-calls/op")
			})
		}
	}
}
