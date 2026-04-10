package support

import (
	"context"

	"serverdrivenui/backend/canery"
)

// Config controls the generic example engine behavior.
type Config struct {
	Member        bool
	InScope       bool
	Groups        []canery.GroupRef
	DirectAllowed map[canery.Action]bool
	GroupAllowed  map[canery.Action]bool
}

// NewEngine builds a small generic canery engine suitable for runnable
// examples.
func NewEngine(cfg Config) *canery.Engine {
	return canery.NewEngine(
		membershipReader{member: cfg.Member},
		groupReader{groups: cfg.Groups},
		permissionReader{
			directAllowed: cfg.DirectAllowed,
			groupAllowed:  cfg.GroupAllowed,
		},
		scopeResolver{inScope: cfg.InScope},
	)
}

type membershipReader struct {
	member bool
}

func (r membershipReader) HasMembership(context.Context, canery.Subject, canery.ScopeRef) (bool, error) {
	return r.member, nil
}

type groupReader struct {
	groups []canery.GroupRef
}

func (r groupReader) GroupsForSubject(context.Context, canery.Subject, canery.ScopeRef) ([]canery.GroupRef, error) {
	return append([]canery.GroupRef(nil), r.groups...), nil
}

type permissionReader struct {
	directAllowed map[canery.Action]bool
	groupAllowed  map[canery.Action]bool
}

func (r permissionReader) HasPermission(_ context.Context, principal canery.PrincipalRef, request canery.Request) (bool, error) {
	switch principal.Kind {
	case canery.PrincipalKindSubject:
		return r.directAllowed[request.Action], nil
	case canery.PrincipalKindGroup:
		return r.groupAllowed[request.Action], nil
	default:
		return false, nil
	}
}

type scopeResolver struct {
	inScope bool
}

func (r scopeResolver) ResourceInScope(context.Context, canery.ResourceRef, canery.ScopeRef) (bool, error) {
	return r.inScope, nil
}
