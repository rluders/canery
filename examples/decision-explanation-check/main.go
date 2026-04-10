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
		Groups: []canery.GroupRef{
			{Type: "role", ID: "editor"},
		},
		GroupAllowed: map[canery.Action]bool{
			canery.Action("publish"): true,
		},
	})

	decision, err := engine.CheckDecision(context.Background(), canery.Request{
		Subject:  canery.User("user-1"),
		Action:   canery.Action("publish"),
		Resource: canery.Resource("document", "doc-1"),
		Scope:    canery.Scope("project", "project-1"),
	})
	if err != nil {
		panic(err)
	}

	fmt.Printf("allowed=%t source=%s reason=%s\n", decision.Allowed, decision.Source, decision.Reason)
}
