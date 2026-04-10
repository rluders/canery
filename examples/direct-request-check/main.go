package main

import (
	"context"
	"fmt"

	"serverdrivenui/backend/canery"
	"serverdrivenui/backend/canery/examples/support"
)

func main() {
	engine := support.NewEngine(support.Config{
		Member:  true,
		InScope: true,
		DirectAllowed: map[canery.Action]bool{
			canery.Action("view"): true,
		},
	})

	allowed, err := engine.Check(context.Background(), canery.Request{
		Subject:  canery.ActorRef{Type: "user", ID: "user-1"},
		Action:   canery.Action("view"),
		Resource: canery.ResourceRef{Type: "document", ID: "doc-1"},
		Scope:    canery.ScopeRef{Type: "project", ID: "project-1"},
	})
	if err != nil {
		panic(err)
	}

	fmt.Printf("direct request allowed: %t\n", allowed)
}
