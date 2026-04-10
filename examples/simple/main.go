package main

import (
	"context"
	"fmt"

	"github.com/rluders/canery"
)

type membershipReader struct{}

func (membershipReader) HasMembership(context.Context, canery.Subject, canery.ScopeRef) (bool, error) {
	return true, nil
}

type groupReader struct{}

func (groupReader) GroupsForSubject(context.Context, canery.Subject, canery.ScopeRef) ([]canery.GroupRef, error) {
	return []canery.GroupRef{{Type: "role", ID: "editor"}}, nil
}

type permissionReader struct{}

func (permissionReader) HasPermission(_ context.Context, principal canery.PrincipalRef, request canery.Request) (bool, error) {
	switch {
	case principal.Kind == canery.PrincipalKindSubject && request.Action == canery.Action("view"):
		return true, nil
	case principal.Kind == canery.PrincipalKindGroup &&
		principal.Type == "role" &&
		principal.ID == "editor" &&
		request.Action == canery.Action("update"):
		return true, nil
	default:
		return false, nil
	}
}

type scopeResolver struct{}

func (scopeResolver) ResourceInScope(context.Context, canery.ResourceRef, canery.ScopeRef) (bool, error) {
	return true, nil
}

func main() {
	engine := canery.NewEngine(
		membershipReader{},
		groupReader{},
		permissionReader{},
		scopeResolver{},
	)

	viewAllowed, err := engine.Check(context.Background(), canery.Request{
		Subject:  canery.Actor("user", "user-1"),
		Action:   canery.Action("view"),
		Resource: canery.Resource("document", "doc-1"),
		Scope:    canery.Scope("project", "project-1"),
	})
	if err != nil {
		panic(err)
	}

	updateRequest := engine.
		For(canery.User("user-1")).
		Can(canery.Action("update")).
		Target(canery.Resource("document", "doc-1")).
		In(canery.Scope("project", "project-1")).
		Request()

	updateDecision, err := engine.CheckDecision(context.Background(), updateRequest)
	if err != nil {
		panic(err)
	}

	decisions, err := engine.BatchCheck(context.Background(), []canery.Request{
		{
			Subject:  canery.User("user-1"),
			Action:   canery.Action("view"),
			Resource: canery.Resource("document", "doc-1"),
			Scope:    canery.Scope("project", "project-1"),
		},
		{
			Subject:  canery.User("user-1"),
			Action:   canery.Action("delete"),
			Resource: canery.Resource("document", "doc-1"),
			Scope:    canery.Scope("project", "project-1"),
		},
	})
	if err != nil {
		panic(err)
	}

	fmt.Printf("direct check allowed=%t\n", viewAllowed)
	fmt.Printf("builder decision allowed=%t source=%s reason=%s\n", updateDecision.Allowed, updateDecision.Source, updateDecision.Reason)
	for _, decision := range decisions {
		fmt.Printf("batch decision allowed=%t source=%s\n", decision.Allowed, decision.Source)
	}
}
