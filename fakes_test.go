package canery

import (
	"context"
	"sync"
)

// countingReaders is a configurable in-memory backend that records every
// reader call so tests can assert both decisions and call patterns.
type countingReaders struct {
	mu sync.Mutex

	members     map[subjectScopeKey]bool
	groups      map[subjectScopeKey][]GroupRef
	permissions map[permissionKey]bool
	outOfScope  map[ResourceRef]bool

	membershipErr error
	groupsErr     error
	permissionErr map[PrincipalRef]error
	resolverErr   error

	membershipCalls int
	groupCalls      int
	permissionCalls int
	resolverCalls   int
	batchCalls      int
	batchSizes      []int
	batchErr        error
	batchShort      bool
}

type permissionKey struct {
	principal PrincipalRef
	action    Action
	resource  ResourceRef
}

func newCountingReaders() *countingReaders {
	return &countingReaders{
		members:       map[subjectScopeKey]bool{},
		groups:        map[subjectScopeKey][]GroupRef{},
		permissions:   map[permissionKey]bool{},
		outOfScope:    map[ResourceRef]bool{},
		permissionErr: map[PrincipalRef]error{},
	}
}

func (c *countingReaders) member(subject Subject, scope ScopeRef, groups ...GroupRef) {
	key := subjectScopeKey{subject: subject, scope: scope}
	c.members[key] = true
	c.groups[key] = groups
}

func (c *countingReaders) grant(principal PrincipalRef, action Action, resource ResourceRef) {
	c.permissions[permissionKey{principal: principal, action: action, resource: resource}] = true
}

func (c *countingReaders) HasMembership(_ context.Context, subject Subject, scope ScopeRef) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.membershipCalls++
	if c.membershipErr != nil {
		return false, c.membershipErr
	}
	return c.members[subjectScopeKey{subject: subject, scope: scope}], nil
}

func (c *countingReaders) GroupsForSubject(_ context.Context, subject Subject, scope ScopeRef) ([]GroupRef, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.groupCalls++
	if c.groupsErr != nil {
		return nil, c.groupsErr
	}
	return c.groups[subjectScopeKey{subject: subject, scope: scope}], nil
}

func (c *countingReaders) HasPermission(_ context.Context, principal PrincipalRef, request Request) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.permissionCalls++
	return c.permission(principal, request)
}

func (c *countingReaders) permission(principal PrincipalRef, request Request) (bool, error) {
	if err := c.permissionErr[principal]; err != nil {
		return false, err
	}
	return c.permissions[permissionKey{principal: principal, action: request.Action, resource: request.Resource}], nil
}

func (c *countingReaders) ResourceInScope(_ context.Context, resource ResourceRef, _ ScopeRef) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.resolverCalls++
	if c.resolverErr != nil {
		return false, c.resolverErr
	}
	return !c.outOfScope[resource], nil
}

func (c *countingReaders) engine() *Engine {
	return NewEngine(c, c, c, c)
}

// batchReaders adds the optional BatchPermissionReader capability.
type batchReaders struct {
	*countingReaders
}

func (b batchReaders) HasPermissions(_ context.Context, checks []PermissionCheck) ([]PermissionResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.batchCalls++
	b.batchSizes = append(b.batchSizes, len(checks))
	if b.batchErr != nil {
		return nil, b.batchErr
	}
	results := make([]PermissionResult, 0, len(checks))
	for _, check := range checks {
		allowed, err := b.permission(check.Principal, check.Request)
		results = append(results, PermissionResult{Allowed: allowed, Err: err})
	}
	if b.batchShort {
		return results[:len(results)-1], nil
	}
	return results, nil
}

func (b batchReaders) engine() *Engine {
	return NewEngine(b, b, b, b)
}
