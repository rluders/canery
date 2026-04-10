package main

import (
	"context"
	"fmt"

	"github.com/rluders/canery"
	db "github.com/rluders/canery-examples/advanced/store"
)

func main() {
	data := db.SeedDemoData()
	store := db.NewStore(data)
	users := db.UsersByEmail(data.Users)
	documents := db.DocumentsByTitle(data.Documents)
	authorizer := db.NewDocumentAuthorizer(store)

	checks := []struct {
		user     db.User
		action   canery.Action
		document db.Document
	}{
		{user: users["ada@example.com"], action: canery.Action("delete"), document: documents["Launch Plan"]},
		{user: users["grace@example.com"], action: canery.Action("update"), document: documents["Launch Plan"]},
		{user: users["linus@example.com"], action: canery.Action("publish"), document: documents["Launch Plan"]},
	}

	for _, check := range checks {
		decision, err := authorizer.CheckDocumentAction(context.Background(), check.user, check.action, check.document)
		if err != nil {
			panic(err)
		}
		fmt.Printf("%s (%s) %s %q => allowed=%t source=%s reason=%s\n", check.user.Name, check.user.Email, check.action, check.document.Title, decision.Allowed, decision.Source, decision.Reason)
	}
}
