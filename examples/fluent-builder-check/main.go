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
			canery.Action("update"): true,
		},
	})

	allowed, err := engine.
		For(canery.User("user-1")).
		Can(canery.Action("update")).
		Target(canery.Resource("document", "doc-1")).
		In(canery.Scope("project", "project-1")).
		Check(context.Background())
	if err != nil {
		panic(err)
	}

	fmt.Printf("fluent builder allowed: %t\n", allowed)
}
