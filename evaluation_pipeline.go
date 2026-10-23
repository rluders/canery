package canery

import "context"

// evaluationItem is the per-request state of one pipeline run.
type evaluationItem struct {
	request  Request
	scope    scopeContext
	trace    Trace
	decision Decision
	err      error
	done     bool
	groups   []GroupRef
}

// evaluationRun applies the default Engine pipeline to one or more requests.
//
// Single checks are a run of one, so Check, CheckDecision, CheckTrace,
// BatchCheck, and BatchCheckResults share exactly one implementation of the
// authorization model. Each stage processes every undecided request before the
// next stage starts, which lets identical lookups be issued once per run.
// Lookups are memoised only for the lifetime of the run.
type evaluationRun struct {
	ctx       context.Context
	engine    *Engine
	items     []evaluationItem
	withTrace bool
}

type lookupResult struct {
	ok  bool
	err error
}

type subjectScopeKey struct {
	subject Subject
	scope   ScopeRef
}

type resourceScopeKey struct {
	resource ResourceRef
	scope    ScopeRef
}

type groupsResult struct {
	groups []GroupRef
	err    error
}

func (e *Engine) run(ctx context.Context, requests []Request, withTrace bool) []evaluationItem {
	run := evaluationRun{
		ctx:       ctx,
		engine:    e,
		items:     make([]evaluationItem, len(requests)),
		withTrace: withTrace,
	}
	for index, request := range requests {
		run.items[index] = evaluationItem{request: request, scope: newScopeContext(request)}
	}
	stages := []func(){
		run.validate,
		run.checkConfiguration,
		run.verifyResourceScopes,
		run.confirmMemberships,
		run.checkDirectPermissions,
		run.resolveGroups,
		run.checkGroupPermissions,
	}
	for _, stage := range stages {
		stage()
	}
	return run.items
}

func (r *evaluationRun) pending() []int {
	indexes := make([]int, 0, len(r.items))
	for index := range r.items {
		if !r.items[index].done {
			indexes = append(indexes, index)
		}
	}
	return indexes
}

func (r *evaluationRun) step(index int, step TraceStep) {
	if r.withTrace {
		r.items[index].trace.Steps = append(r.items[index].trace.Steps, step)
	}
}

func (r *evaluationRun) fail(index int, err error) {
	r.items[index].err = err
	r.items[index].done = true
}

func (r *evaluationRun) decide(index int, decision Decision) {
	r.items[index].decision = decision
	r.items[index].done = true
}

func (r *evaluationRun) deny(index int, stage string, reason string) {
	r.step(index, TraceStep{Name: stage, Result: TraceResultDenied, Source: DecisionSourceNone})
	r.decide(index, Decision{Allowed: false, Reason: reason, Source: DecisionSourceNone})
}

// readerFailure records a failed lookup. Cancellation observed by the engine
// is returned as the bare context error; reader errors are wrapped in an
// EvaluationError.
func (r *evaluationRun) readerFailure(index int, stage string, principal PrincipalRef, err error) {
	r.step(index, TraceStep{Name: stage, Result: TraceResultError, Principal: principal})
	if err == r.ctx.Err() {
		r.fail(index, err)
		return
	}
	r.fail(index, EvaluationError{Stage: stage, Err: err})
}

// cancelled fails every pending item when the context is done, so a cancelled
// evaluation is never reported as a denial.
func (r *evaluationRun) cancelled() bool {
	err := r.ctx.Err()
	if err == nil {
		return false
	}
	for _, index := range r.pending() {
		r.fail(index, err)
	}
	return true
}

func (r *evaluationRun) validate() {
	for index := range r.items {
		if err := validateRequest(r.items[index].request); err != nil {
			r.step(index, TraceStep{Name: TraceStepRequestValidated, Result: TraceResultFailed})
			r.fail(index, err)
			continue
		}
		r.step(index, TraceStep{Name: TraceStepRequestValidated, Result: TraceResultPassed})
	}
}

func (r *evaluationRun) checkConfiguration() {
	var err error
	switch {
	case r.engine.memberships == nil:
		err = ErrMissingMembership
	case r.engine.groups == nil:
		err = ErrMissingGroupReader
	case r.engine.permissions == nil:
		err = ErrMissingPermissions
	default:
		return
	}
	for _, index := range r.pending() {
		r.fail(index, err)
	}
}

func (r *evaluationRun) verifyResourceScopes() {
	if r.cancelled() {
		return
	}
	seen := map[resourceScopeKey]lookupResult{}
	for _, index := range r.pending() {
		item := &r.items[index]
		resource := item.request.Resource
		if !item.scope.requiresResourceScopeCheck(resource) {
			r.step(index, TraceStep{Name: TraceStepResourceScopeVerified, Result: TraceResultSkipped})
			continue
		}
		if r.engine.resolver == nil {
			r.step(index, TraceStep{Name: TraceStepResourceScopeVerified, Result: TraceResultError})
			r.fail(index, ErrMissingScopeResolver)
			continue
		}
		key := resourceScopeKey{resource: resource, scope: item.scope.targetScope()}
		result, ok := seen[key]
		if !ok {
			result = r.lookup(func() (bool, error) {
				return r.engine.resolver.ResourceInScope(r.ctx, resource, key.scope)
			})
			seen[key] = result
		}
		switch {
		case result.err != nil:
			r.readerFailure(index, TraceStepResourceScopeVerified, PrincipalRef{}, result.err)
		case !result.ok:
			r.deny(index, TraceStepResourceScopeVerified, decisionReasonResourceScope)
		default:
			r.step(index, TraceStep{Name: TraceStepResourceScopeVerified, Result: TraceResultPassed})
		}
	}
}

func (r *evaluationRun) confirmMemberships() {
	if r.cancelled() {
		return
	}
	seen := map[subjectScopeKey]lookupResult{}
	for _, index := range r.pending() {
		item := &r.items[index]
		key := subjectScopeKey{subject: item.request.Subject, scope: item.scope.targetScope()}
		result, ok := seen[key]
		if !ok {
			result = r.lookup(func() (bool, error) {
				return r.engine.memberships.HasMembership(r.ctx, key.subject, key.scope)
			})
			seen[key] = result
		}
		switch {
		case result.err != nil:
			r.readerFailure(index, TraceStepMembershipConfirmed, PrincipalRef{}, result.err)
		case !result.ok:
			r.deny(index, TraceStepMembershipConfirmed, decisionReasonMembership)
		default:
			r.step(index, TraceStep{Name: TraceStepMembershipConfirmed, Result: TraceResultPassed})
		}
	}
}

func (r *evaluationRun) checkDirectPermissions() {
	if r.cancelled() {
		return
	}
	pending := r.pending()
	checks := make([]PermissionCheck, 0, len(pending))
	for _, index := range pending {
		request := r.items[index].request
		checks = append(checks, PermissionCheck{Principal: SubjectPrincipal(request.Subject), Request: request})
	}
	results := r.permissionResults(checks)
	for position, index := range pending {
		check := checks[position]
		result := results[check]
		switch {
		case result.err != nil:
			r.readerFailure(index, TraceStepDirectChecked, check.Principal, result.err)
		case result.ok:
			r.step(index, TraceStep{Name: TraceStepDirectChecked, Result: TraceResultMatched, Principal: check.Principal, Source: DecisionSourceDirect})
			r.decide(index, Decision{
				Allowed:   true,
				Reason:    decisionReasonDirectPermission,
				Source:    DecisionSourceDirect,
				Principal: check.Principal,
			})
		default:
			r.step(index, TraceStep{Name: TraceStepDirectChecked, Result: TraceResultNotMatched, Principal: check.Principal})
		}
	}
}

func (r *evaluationRun) resolveGroups() {
	if r.cancelled() {
		return
	}
	seen := map[subjectScopeKey]groupsResult{}
	for _, index := range r.pending() {
		item := &r.items[index]
		key := subjectScopeKey{subject: item.request.Subject, scope: item.scope.targetScope()}
		result, ok := seen[key]
		if !ok {
			if err := r.ctx.Err(); err != nil {
				result = groupsResult{err: err}
			} else {
				groups, err := r.engine.groups.GroupsForSubject(r.ctx, key.subject, key.scope)
				result = groupsResult{groups: groups, err: err}
			}
			seen[key] = result
		}
		if result.err != nil {
			r.readerFailure(index, TraceStepGroupsResolved, PrincipalRef{}, result.err)
			continue
		}
		item.groups = result.groups
		if len(result.groups) == 0 {
			r.step(index, TraceStep{Name: TraceStepGroupsResolved, Result: TraceResultNone})
		} else {
			r.step(index, TraceStep{Name: TraceStepGroupsResolved, Result: TraceResultResolved})
		}
	}
}

// checkGroupPermissions evaluates each request's groups in reader order and
// stops at the first match, exactly like a sequential loop. With a
// BatchPermissionReader every group check of the run is issued in one call;
// otherwise checks are issued one at a time, in order, and memoised.
func (r *evaluationRun) checkGroupPermissions() {
	if r.cancelled() {
		return
	}
	pending := r.pending()
	var results map[PermissionCheck]lookupResult
	if _, ok := r.engine.permissions.(BatchPermissionReader); ok {
		checks := []PermissionCheck{}
		for _, index := range pending {
			item := r.items[index]
			for _, group := range item.groups {
				checks = append(checks, PermissionCheck{Principal: GroupPrincipal(group), Request: item.request})
			}
		}
		results = r.permissionResults(checks)
	} else {
		results = map[PermissionCheck]lookupResult{}
	}

	for _, index := range pending {
		item := r.items[index]
		decided := false
		for _, group := range item.groups {
			check := PermissionCheck{Principal: GroupPrincipal(group), Request: item.request}
			result, ok := results[check]
			if !ok {
				result = r.lookup(func() (bool, error) {
					return r.engine.permissions.HasPermission(r.ctx, check.Principal, check.Request)
				})
				results[check] = result
			}
			if result.err != nil {
				r.readerFailure(index, TraceStepGroupChecked, check.Principal, result.err)
				decided = true
				break
			}
			if result.ok {
				r.step(index, TraceStep{Name: TraceStepGroupChecked, Result: TraceResultMatched, Principal: check.Principal, Source: DecisionSourceGroup})
				r.decide(index, Decision{
					Allowed:   true,
					Reason:    decisionReasonGroupPermission,
					Source:    DecisionSourceGroup,
					Principal: check.Principal,
				})
				decided = true
				break
			}
		}
		if !decided {
			r.step(index, TraceStep{Name: TraceStepGroupChecked, Result: TraceResultNotMatched, Source: DecisionSourceNone})
			r.decide(index, Decision{Allowed: false, Reason: decisionReasonNoPermission, Source: DecisionSourceNone})
		}
	}
}

// lookup runs one reader call unless the context is already done.
func (r *evaluationRun) lookup(call func() (bool, error)) lookupResult {
	if err := r.ctx.Err(); err != nil {
		return lookupResult{err: err}
	}
	ok, err := call()
	return lookupResult{ok: ok, err: err}
}

// permissionResults answers every distinct check, using the batch reader when
// more than one distinct check is needed and the capability is available.
func (r *evaluationRun) permissionResults(checks []PermissionCheck) map[PermissionCheck]lookupResult {
	results := make(map[PermissionCheck]lookupResult, len(checks))
	unique := make([]PermissionCheck, 0, len(checks))
	for _, check := range checks {
		if _, ok := results[check]; !ok {
			results[check] = lookupResult{}
			unique = append(unique, check)
		}
	}

	batch, ok := r.engine.permissions.(BatchPermissionReader)
	if !ok || len(unique) < 2 {
		for _, check := range unique {
			results[check] = r.lookup(func() (bool, error) {
				return r.engine.permissions.HasPermission(r.ctx, check.Principal, check.Request)
			})
		}
		return results
	}

	if err := r.ctx.Err(); err != nil {
		for _, check := range unique {
			results[check] = lookupResult{err: err}
		}
		return results
	}
	batchResults, err := batch.HasPermissions(r.ctx, unique)
	if err == nil && len(batchResults) != len(unique) {
		err = ErrBatchResultMismatch
	}
	for position, check := range unique {
		if err != nil {
			results[check] = lookupResult{err: err}
			continue
		}
		results[check] = lookupResult{ok: batchResults[position].Allowed, err: batchResults[position].Err}
	}
	return results
}
