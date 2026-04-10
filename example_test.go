package canery

import (
	"context"
	"fmt"
)

type exampleMembershipReader struct{}

func (exampleMembershipReader) HasMembership(context.Context, Subject, ScopeRef) (bool, error) {
	return true, nil
}

type exampleGroupReader struct{}

func (exampleGroupReader) GroupsForSubject(context.Context, Subject, ScopeRef) ([]GroupRef, error) {
	return []GroupRef{{Type: "role", ID: "editor"}}, nil
}

type examplePermissionReader struct{}

func (examplePermissionReader) HasPermission(_ context.Context, principal PrincipalRef, request Request) (bool, error) {
	return principal.Kind == PrincipalKindGroup &&
		principal.Type == "role" &&
		principal.ID == "editor" &&
		request.Action == Action("update"), nil
}

type exampleScopeResolver struct{}

func (exampleScopeResolver) ResourceInScope(context.Context, ResourceRef, ScopeRef) (bool, error) {
	return true, nil
}

func ExampleEngine_Check() {
	engine := NewEngine(
		exampleMembershipReader{},
		exampleGroupReader{},
		examplePermissionReader{},
		exampleScopeResolver{},
	)

	ok, err := engine.Check(context.Background(), Request{
		Subject:  ActorRef{Type: "user", ID: "user-1"},
		Action:   Action("update"),
		Resource: ResourceRef{Type: "document", ID: "doc-1"},
		Scope:    ScopeRef{Type: "project", ID: "project-1"},
	})
	fmt.Println(ok, err == nil)
	// Output:
	// true true
}

func ExampleActor() {
	engine := NewEngine(
		exampleMembershipReader{},
		exampleGroupReader{},
		examplePermissionReader{},
		exampleScopeResolver{},
	)

	ok, err := engine.Check(context.Background(), Request{
		Subject:  Actor("user", "user-1"),
		Action:   Action("update"),
		Resource: Resource("document", "doc-1"),
		Scope:    Scope("project", "project-1"),
	})
	fmt.Println(ok, err == nil)
	// Output:
	// true true
}

func ExampleEngine_For() {
	engine := NewEngine(
		exampleMembershipReader{},
		exampleGroupReader{},
		examplePermissionReader{},
		exampleScopeResolver{},
	)

	ok, err := engine.
		For(ActorRef{Type: "user", ID: "user-1"}).
		Can(Action("update")).
		Target(ResourceRef{Type: "document", ID: "doc-1"}).
		In(ScopeRef{Type: "project", ID: "project-1"}).
		Check(context.Background())
	fmt.Println(ok, err == nil)
	// Output:
	// true true
}

func ExampleBuilder_Request() {
	engine := NewEngine(
		exampleMembershipReader{},
		exampleGroupReader{},
		examplePermissionReader{},
		exampleScopeResolver{},
	)

	request := engine.
		For(ActorRef{Type: "user", ID: "user-1"}).
		Can(Action("create")).
		Target(ResourceRef{Type: "document"}).
		In(ScopeRef{Type: "project", ID: "project-1"}).
		Request()

	fmt.Println(request.Subject.Type, request.Action, request.Resource.Type, request.Resource.ID == "")
	// Output:
	// user create document true
}

func ExampleBuilder_CanMany() {
	engine := NewEngine(
		exampleMembershipReader{},
		exampleGroupReader{},
		examplePermissionReader{},
		exampleScopeResolver{},
	)

	result, err := engine.
		For(User("user-1")).
		CanMany(Action("update"), Action("delete")).
		Target(Resource("document", "doc-1")).
		In(Scope("project", "project-1")).
		Check(context.Background())

	canUpdate, _ := result.Allowed(Action("update"))
	canDelete, _ := result.Allowed(Action("delete"))
	fmt.Println(err == nil, canUpdate, canDelete)
	// Output:
	// true true false
}
