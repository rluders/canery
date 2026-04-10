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

	for _, decision := range decisions {
		fmt.Printf("batch decision allowed=%t source=%s\n", decision.Allowed, decision.Source)
	}
}
