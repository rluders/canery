package main

import (
	"context"
	"fmt"

	"github.com/rluders/canery"
	"github.com/rluders/canery/examples/support"
)

func main() {
	engine := support.NewEngine(support.Config{
		Member:  true,
		InScope: true,
		DirectAllowed: map[canery.Action]bool{
			canery.Action("create"): true,
		},
	})

	allowed, err := engine.Check(context.Background(), canery.Request{
		Subject:  canery.User("user-1"),
		Action:   canery.Action("create"),
		Resource: canery.Resource("document", ""),
		Scope:    canery.Scope("project", "project-1"),
	})
	if err != nil {
		panic(err)
	}

	fmt.Printf("helper-based create allowed: %t\n", allowed)
}
