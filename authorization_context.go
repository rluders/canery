package canery

import (
	"fmt"
	"reflect"
)

// AuthorizationContext carries optional, application-defined values alongside
// a Request, such as the authentication method, client identity, or claims
// the application has already validated.
//
// The zero value is an empty context and is ready to use. Values are stored by
// key, like context.Context; use unexported key types to avoid collisions.
// With returns a new context and never modifies the receiver, so a context can
// be shared between concurrent evaluations. Stored values themselves must be
// safe for concurrent reads if the context is shared.
//
// canery treats context values as opaque: the default Engine never reads
// them, and they never appear in Decision, Trace, or errors. Formatting an
// AuthorizationContext with the fmt package prints a redacted placeholder.
// Policies and custom evaluators that read context values own whatever
// semantics and exposure they give them.
//
// AuthorizationContext is comparable, so Request stays comparable. Two
// contexts are equal only if they are the same context value (or both empty),
// not if they merely hold equal entries.
type AuthorizationContext struct {
	node *authorizationContextNode
}

type authorizationContextNode struct {
	parent *authorizationContextNode
	key    any
	value  any
}

// With returns a copy of the context in which key is associated with value.
//
// It panics if key is nil or not comparable, matching context.WithValue.
func (c AuthorizationContext) With(key any, value any) AuthorizationContext {
	if key == nil {
		panic("canery: nil authorization context key")
	}
	if !reflect.TypeOf(key).Comparable() {
		panic("canery: authorization context key is not comparable")
	}
	return AuthorizationContext{node: &authorizationContextNode{
		parent: c.node,
		key:    key,
		value:  value,
	}}
}

// Value returns the value associated with key and whether it was present.
func (c AuthorizationContext) Value(key any) (any, bool) {
	for node := c.node; node != nil; node = node.parent {
		if node.key == key {
			return node.value, true
		}
	}
	return nil, false
}

// IsZero reports whether the context holds no values.
func (c AuthorizationContext) IsZero() bool {
	return c.node == nil
}

// String returns a redacted placeholder so context values are never printed.
func (c AuthorizationContext) String() string {
	return "canery.AuthorizationContext{redacted}"
}

// Format prints a redacted placeholder for every fmt verb.
func (c AuthorizationContext) Format(state fmt.State, _ rune) {
	_, _ = state.Write([]byte(c.String()))
}

// ContextValue returns the value stored under key as a T. It reports false
// when the key is missing or the stored value is not a T.
func ContextValue[T any](c AuthorizationContext, key any) (T, bool) {
	value, ok := c.Value(key)
	if !ok {
		var zero T
		return zero, false
	}
	typed, ok := value.(T)
	return typed, ok
}
