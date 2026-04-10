package store

import (
	"context"
	"fmt"
	"testing"

	"github.com/rluders/canery"
)

func seededStore() *Store {
	return NewStore(SeedDemoData())
}

func TestDocumentAuthorizerAllowsDirectUserPermission(t *testing.T) {
	store := seededStore()
	users := UsersByEmail(store.users)
	documents := DocumentsByTitle(store.documents)

	decision, err := NewDocumentAuthorizer(store).CheckDocumentAction(context.Background(), users["ada@example.com"], canery.Action("delete"), documents["Launch Plan"])
	if err != nil {
		t.Fatalf("CheckDocumentAction returned error: %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("Allowed = false, want true")
	}
	if decision.Source != canery.DecisionSourceDirect {
		t.Fatalf("Source = %q, want %q", decision.Source, canery.DecisionSourceDirect)
	}
}

func TestDocumentAuthorizerAllowsRolePermission(t *testing.T) {
	store := seededStore()
	users := UsersByEmail(store.users)
	documents := DocumentsByTitle(store.documents)

	decision, err := NewDocumentAuthorizer(store).CheckDocumentAction(context.Background(), users["grace@example.com"], canery.Action("update"), documents["Launch Plan"])
	if err != nil {
		t.Fatalf("CheckDocumentAction returned error: %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("Allowed = false, want true")
	}
	if decision.Source != canery.DecisionSourceGroup {
		t.Fatalf("Source = %q, want %q", decision.Source, canery.DecisionSourceGroup)
	}
}

func TestDocumentAuthorizerDeniesMemberWithoutPermission(t *testing.T) {
	store := seededStore()
	users := UsersByEmail(store.users)
	documents := DocumentsByTitle(store.documents)

	decision, err := NewDocumentAuthorizer(store).CheckDocumentAction(context.Background(), users["linus@example.com"], canery.Action("publish"), documents["Launch Plan"])
	if err != nil {
		t.Fatalf("CheckDocumentAction returned error: %v", err)
	}
	if decision.Allowed {
		t.Fatalf("Allowed = true, want false")
	}
	if decision.Reason != "no matching permission" {
		t.Fatalf("Reason = %q, want %q", decision.Reason, "no matching permission")
	}
}

func TestDocumentAuthorizerDeniesUserOutsideProjectMembership(t *testing.T) {
	store := seededStore()
	users := UsersByEmail(store.users)
	documents := DocumentsByTitle(store.documents)

	decision, err := NewDocumentAuthorizer(store).CheckDocumentAction(context.Background(), users["ada@example.com"], canery.Action("delete"), documents["Budget Draft"])
	if err != nil {
		t.Fatalf("CheckDocumentAction returned error: %v", err)
	}
	if decision.Allowed {
		t.Fatalf("Allowed = true, want false")
	}
	if decision.Reason != "subject not in scope" {
		t.Fatalf("Reason = %q, want %q", decision.Reason, "subject not in scope")
	}
}

func TestDocumentAuthorizerDeniesDocumentOutsideRequestedProject(t *testing.T) {
	store := seededStore()
	users := UsersByEmail(store.users)

	decision, err := NewEngine(store).CheckDecision(context.Background(), canery.Request{
		Subject:  UserSubject(users["ada@example.com"]),
		Action:   canery.Action("delete"),
		Resource: canery.Resource("document", "f37a9064-91d2-42c0-a95c-d64d55773002"),
		Scope:    canery.Scope("project", "c0d8db43-31b2-4d18-bc84-6dc25c0a1001"),
	})
	if err != nil {
		t.Fatalf("CheckDecision returned error: %v", err)
	}
	if decision.Allowed {
		t.Fatalf("Allowed = true, want false")
	}
	if decision.Reason != "resource outside scope" {
		t.Fatalf("Reason = %q, want %q", decision.Reason, "resource outside scope")
	}
}

func TestDocumentAuthorizerAllowsCreateStyleActionInsideProject(t *testing.T) {
	store := seededStore()
	users := UsersByEmail(store.users)
	projects := ProjectsByName(store.projects)

	decision, err := NewDocumentAuthorizer(store).CheckProjectDocumentAction(context.Background(), users["ada@example.com"], canery.Action("create"), projects["Canery"])
	if err != nil {
		t.Fatalf("CheckProjectDocumentAction returned error: %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("Allowed = false, want true")
	}
	if decision.Source != canery.DecisionSourceDirect {
		t.Fatalf("Source = %q, want %q", decision.Source, canery.DecisionSourceDirect)
	}
}

func ExampleDocumentAuthorizer() {
	store := seededStore()
	users := UsersByEmail(store.users)
	documents := DocumentsByTitle(store.documents)
	authorizer := NewDocumentAuthorizer(store)

	direct, _ := authorizer.CheckDocumentAction(context.Background(), users["ada@example.com"], canery.Action("delete"), documents["Launch Plan"])
	group, _ := authorizer.CheckDocumentAction(context.Background(), users["grace@example.com"], canery.Action("update"), documents["Launch Plan"])
	denied, _ := authorizer.CheckDocumentAction(context.Background(), users["linus@example.com"], canery.Action("publish"), documents["Launch Plan"])

	fmt.Printf("ada@example.com delete Launch Plan => allowed=%t source=%s\n", direct.Allowed, direct.Source)
	fmt.Printf("grace@example.com update Launch Plan => allowed=%t source=%s\n", group.Allowed, group.Source)
	fmt.Printf("linus@example.com publish Launch Plan => allowed=%t source=%s\n", denied.Allowed, denied.Source)
	// Output:
	// ada@example.com delete Launch Plan => allowed=true source=direct
	// grace@example.com update Launch Plan => allowed=true source=group
	// linus@example.com publish Launch Plan => allowed=false source=none
}
