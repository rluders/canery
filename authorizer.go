package canery

import "context"

// Authorizer evaluates authorization requests and exposes a fluent request
// builder rooted on a subject.
//
// Engine is the default implementation shipped by the package, but the
// interface is intentionally broad enough for alternate evaluators over the
// same request model.
type Authorizer interface {
	// CheckDecision evaluates a low-level request directly and returns the
	// explicit decision object.
	CheckDecision(ctx context.Context, request Request) (Decision, error)
	// CheckTrace evaluates a low-level request directly and also returns a
	// high-level trace of the evaluation flow for debugging.
	CheckTrace(ctx context.Context, request Request) (Decision, Trace, error)
	// Check evaluates a low-level request directly.
	Check(ctx context.Context, request Request) (bool, error)
	// For starts a fluent builder for the given subject.
	For(subject Subject) Builder
}
