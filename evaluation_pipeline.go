package canery

import "context"

// defaultEvaluationPipeline captures the default Engine evaluation strategy in
// one place.
//
// It is intentionally private. The goal is to make the shipped Engine's
// behavior explicit and easier to evolve without changing the public request
// model or reader interfaces.
type defaultEvaluationPipeline struct {
	engine  *Engine
	request Request
	scope   scopeContext
	trace   *Trace
}

func newDefaultEvaluationPipeline(engine *Engine, request Request, scope scopeContext, trace *Trace) defaultEvaluationPipeline {
	return defaultEvaluationPipeline{
		engine:  engine,
		request: request,
		scope:   scope,
		trace:   trace,
	}
}

func (p defaultEvaluationPipeline) requiresResourceScopeCheck() bool {
	return p.scope.requiresResourceScopeCheck(p.request.Resource)
}

func (p defaultEvaluationPipeline) ensureResourceInScope(ctx context.Context) (bool, error) {
	return p.engine.ensureResourceInScope(ctx, p.scope, p.request.Resource)
}

func (p defaultEvaluationPipeline) hasSubjectMembership(ctx context.Context) (bool, error) {
	return p.engine.hasSubjectMembership(ctx, p.request.Subject, p.scope)
}

func (p defaultEvaluationPipeline) groupsForSubject(ctx context.Context) ([]GroupRef, error) {
	return p.engine.groupsForSubject(ctx, p.request.Subject, p.scope)
}
