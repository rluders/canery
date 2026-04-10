package store

import (
	"context"

	"github.com/rluders/canery"
)

const (
	subjectTypeUser      = "user"
	scopeTypeProject     = "project"
	resourceTypeDoc      = "document"
	groupTypeProjectRole = "project_role"
)

type User struct {
	ID    string
	Email string
	Name  string
}

type Project struct {
	ID   string
	Name string
}

type Document struct {
	ID        string
	ProjectID string
	Title     string
}

type ProjectMembership struct {
	UserID    string
	ProjectID string
}

type ProjectRole struct {
	ID          string
	DisplayName string
}

type ProjectRoleAssignment struct {
	UserID    string
	ProjectID string
	RoleID    string
}

type PermissionGrant struct {
	PrincipalKind canery.PrincipalKind
	PrincipalID   string
	Action        canery.Action
	ResourceType  string
	ProjectID     string
}

type SeedData struct {
	Users           []User
	Projects        []Project
	Documents       []Document
	Memberships     []ProjectMembership
	Roles           []ProjectRole
	RoleAssignments []ProjectRoleAssignment
	Permissions     []PermissionGrant
}

type Store struct {
	users           []User
	projects        []Project
	documents       []Document
	memberships     []ProjectMembership
	roles           []ProjectRole
	roleAssignments []ProjectRoleAssignment
	permissions     []PermissionGrant
}

func NewStore(data SeedData) *Store {
	return &Store{
		users:           append([]User(nil), data.Users...),
		projects:        append([]Project(nil), data.Projects...),
		documents:       append([]Document(nil), data.Documents...),
		memberships:     append([]ProjectMembership(nil), data.Memberships...),
		roles:           append([]ProjectRole(nil), data.Roles...),
		roleAssignments: append([]ProjectRoleAssignment(nil), data.RoleAssignments...),
		permissions:     append([]PermissionGrant(nil), data.Permissions...),
	}
}

func NewEngine(store *Store) *canery.Engine {
	return canery.NewEngine(store, store, store, store)
}

type DocumentAuthorizer struct {
	engine *canery.Engine
}

func NewDocumentAuthorizer(store *Store) DocumentAuthorizer {
	return DocumentAuthorizer{engine: NewEngine(store)}
}

func (a DocumentAuthorizer) CheckDocumentAction(ctx context.Context, user User, action canery.Action, document Document) (canery.Decision, error) {
	return a.engine.CheckDecision(ctx, canery.Request{
		Subject:  UserSubject(user),
		Action:   action,
		Resource: DocumentResource(document),
		Scope:    ProjectScope(Project{ID: document.ProjectID}),
	})
}

func (a DocumentAuthorizer) CheckProjectDocumentAction(ctx context.Context, user User, action canery.Action, project Project) (canery.Decision, error) {
	return a.engine.CheckDecision(ctx, canery.Request{
		Subject:  UserSubject(user),
		Action:   action,
		Resource: canery.Resource(resourceTypeDoc, ""),
		Scope:    ProjectScope(project),
	})
}

func (s *Store) HasMembership(_ context.Context, subject canery.Subject, scope canery.ScopeRef) (bool, error) {
	if subject.Type != subjectTypeUser || scope.Type != scopeTypeProject {
		return false, nil
	}
	for _, membership := range s.memberships {
		if membership.UserID == subject.ID && membership.ProjectID == scope.ID {
			return true, nil
		}
	}
	return false, nil
}

func (s *Store) GroupsForSubject(_ context.Context, subject canery.Subject, scope canery.ScopeRef) ([]canery.GroupRef, error) {
	if subject.Type != subjectTypeUser || scope.Type != scopeTypeProject {
		return nil, nil
	}
	groups := make([]canery.GroupRef, 0, len(s.roleAssignments))
	for _, assignment := range s.roleAssignments {
		if assignment.UserID == subject.ID && assignment.ProjectID == scope.ID {
			groups = append(groups, canery.GroupRef{Type: groupTypeProjectRole, ID: assignment.RoleID})
		}
	}
	return groups, nil
}

func (s *Store) HasPermission(_ context.Context, principal canery.PrincipalRef, request canery.Request) (bool, error) {
	if request.Scope.Type != scopeTypeProject {
		return false, nil
	}
	for _, grant := range s.permissions {
		if grant.PrincipalKind == principal.Kind &&
			grant.PrincipalID == principal.ID &&
			grant.Action == request.Action &&
			grant.ResourceType == request.Resource.Type &&
			grant.ProjectID == request.Scope.ID {
			return true, nil
		}
	}
	return false, nil
}

func (s *Store) ResourceInScope(_ context.Context, resource canery.ResourceRef, scope canery.ScopeRef) (bool, error) {
	if resource.Type != resourceTypeDoc || scope.Type != scopeTypeProject {
		return false, nil
	}
	for _, document := range s.documents {
		if document.ID == resource.ID && document.ProjectID == scope.ID {
			return true, nil
		}
	}
	return false, nil
}

func UserSubject(user User) canery.Subject {
	return canery.Actor(subjectTypeUser, user.ID)
}

func ProjectScope(project Project) canery.ScopeRef {
	return canery.Scope(scopeTypeProject, project.ID)
}

func DocumentResource(document Document) canery.ResourceRef {
	return canery.Resource(resourceTypeDoc, document.ID)
}

func SeedDemoData() SeedData {
	return SeedData{
		Users: []User{
			{ID: "5f4b6c8a-7d9e-4f3a-9b8d-2c1a6e7f9011", Email: "ada@example.com", Name: "Ada Lovelace"},
			{ID: "6a1f4920-3ef3-4fd4-9618-6c73277c1122", Email: "grace@example.com", Name: "Grace Hopper"},
			{ID: "7b2d9c41-8a56-4b26-9a9b-1f4420ab3344", Email: "linus@example.com", Name: "Linus Torvalds"},
		},
		Projects: []Project{
			{ID: "c0d8db43-31b2-4d18-bc84-6dc25c0a1001", Name: "Canery"},
			{ID: "d9eb6cc1-6e02-49a4-9f95-ae5bc8e71002", Name: "Side Quest"},
		},
		Documents: []Document{
			{ID: "e2fa1d77-2dbf-4378-996b-cc0f89152001", ProjectID: "c0d8db43-31b2-4d18-bc84-6dc25c0a1001", Title: "Launch Plan"},
			{ID: "f37a9064-91d2-42c0-a95c-d64d55773002", ProjectID: "d9eb6cc1-6e02-49a4-9f95-ae5bc8e71002", Title: "Budget Draft"},
		},
		Memberships: []ProjectMembership{
			{UserID: "5f4b6c8a-7d9e-4f3a-9b8d-2c1a6e7f9011", ProjectID: "c0d8db43-31b2-4d18-bc84-6dc25c0a1001"},
			{UserID: "6a1f4920-3ef3-4fd4-9618-6c73277c1122", ProjectID: "c0d8db43-31b2-4d18-bc84-6dc25c0a1001"},
			{UserID: "7b2d9c41-8a56-4b26-9a9b-1f4420ab3344", ProjectID: "c0d8db43-31b2-4d18-bc84-6dc25c0a1001"},
		},
		Roles: []ProjectRole{
			{ID: "editor", DisplayName: "Editor"},
		},
		RoleAssignments: []ProjectRoleAssignment{
			{UserID: "6a1f4920-3ef3-4fd4-9618-6c73277c1122", ProjectID: "c0d8db43-31b2-4d18-bc84-6dc25c0a1001", RoleID: "editor"},
		},
		Permissions: []PermissionGrant{
			{PrincipalKind: canery.PrincipalKindSubject, PrincipalID: "5f4b6c8a-7d9e-4f3a-9b8d-2c1a6e7f9011", Action: canery.Action("delete"), ResourceType: resourceTypeDoc, ProjectID: "c0d8db43-31b2-4d18-bc84-6dc25c0a1001"},
			{PrincipalKind: canery.PrincipalKindSubject, PrincipalID: "5f4b6c8a-7d9e-4f3a-9b8d-2c1a6e7f9011", Action: canery.Action("create"), ResourceType: resourceTypeDoc, ProjectID: "c0d8db43-31b2-4d18-bc84-6dc25c0a1001"},
			{PrincipalKind: canery.PrincipalKindGroup, PrincipalID: "editor", Action: canery.Action("update"), ResourceType: resourceTypeDoc, ProjectID: "c0d8db43-31b2-4d18-bc84-6dc25c0a1001"},
		},
	}
}

func UsersByEmail(users []User) map[string]User {
	out := make(map[string]User, len(users))
	for _, user := range users {
		out[user.Email] = user
	}
	return out
}

func ProjectsByName(projects []Project) map[string]Project {
	out := make(map[string]Project, len(projects))
	for _, project := range projects {
		out[project.Name] = project
	}
	return out
}

func DocumentsByTitle(documents []Document) map[string]Document {
	out := make(map[string]Document, len(documents))
	for _, document := range documents {
		out[document.Title] = document
	}
	return out
}
