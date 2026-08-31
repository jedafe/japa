//go:build integration

package content

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/testdb"
)

func TestCreateContent_OrgAttribution_Permission(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	svc := NewService(database)

	owner := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "org-owner"})
	editor := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "org-editor"})
	plain := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "org-plain"})
	outsider := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "org-outsider"})

	spaceID := testdb.SeedSpaceMembership(t, database, tenantID, owner, testdb.SpaceOpts{})

	orgID := testdb.SeedOrganization(t, database, tenantID, testdb.OrgOpts{Slug: "acme", Status: "active", OwnerUserID: owner})
	testdb.SeedOrgMembership(t, database, orgID, owner, "owner")
	testdb.SeedOrgMembership(t, database, orgID, editor, "editor")
	testdb.SeedOrgMembership(t, database, orgID, plain, "member")

	newInput := func(author pgtype.UUID) CreateContentInput {
		return CreateContentInput{
			AuthorID:       author,
			OrganizationID: orgID,
			SpaceID:        spaceID,
			ContentType:    "discussion",
			Title:          "Hello from Acme",
			Body:           "body",
			BodyHtml:       "<p>body</p>",
			SeoIndexable:   true,
			Status:         "published",
			Visibility:     "public",
		}
	}

	// editor may post as org
	row, err := svc.CreateContent(context.Background(), tenantID, newInput(editor))
	if err != nil {
		t.Fatalf("editor CreateContent: %v", err)
	}
	if !row.OrganizationID.Valid || row.OrganizationID != orgID {
		t.Fatalf("org post organization_id not recorded: %+v", row.OrganizationID)
	}
	if row.AuthorID != editor {
		t.Fatalf("author_id must be the human editor, got %+v", row.AuthorID)
	}

	// plain member → forbidden
	if _, err := svc.CreateContent(context.Background(), tenantID, newInput(plain)); !errors.Is(err, ErrOrgPostForbidden) {
		t.Fatalf("plain member: err=%v want ErrOrgPostForbidden", err)
	}
	// non-member → forbidden
	if _, err := svc.CreateContent(context.Background(), tenantID, newInput(outsider)); !errors.Is(err, ErrOrgPostForbidden) {
		t.Fatalf("outsider: err=%v want ErrOrgPostForbidden", err)
	}
}

func TestUpdateContent_OrgAttribution_Permission(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	svc := NewService(database)

	admin := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "up-admin"})
	member := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "up-member"})
	spaceID := testdb.SeedSpaceMembership(t, database, tenantID, admin, testdb.SpaceOpts{})
	orgID := testdb.SeedOrganization(t, database, tenantID, testdb.OrgOpts{Slug: "beta", Status: "active", OwnerUserID: admin})
	testdb.SeedOrgMembership(t, database, orgID, admin, "admin")
	testdb.SeedOrgMembership(t, database, orgID, member, "member")

	// seed a plain (non-org) post authored by admin
	postID := testdb.SeedContent(t, database, tenantID, admin, testdb.ContentOpts{Title: "draft", Status: "draft", Visibility: "public"})

	base := UpdateContentInput{
		Title: "attributed to org", Body: "b", BodyHtml: "<p>b</p>",
		Status: "published", Visibility: "public", OrganizationID: orgID, SpaceID: spaceID,
	}

	// admin may attribute the post to the org
	if _, err := svc.UpdateContent(context.Background(), postID, tenantID, admin, base, false); err != nil {
		t.Fatalf("admin UpdateContent: %v", err)
	}
	// member may not
	if _, err := svc.UpdateContent(context.Background(), postID, tenantID, member, base, false); !errors.Is(err, ErrOrgPostForbidden) {
		t.Fatalf("member update: err=%v want ErrOrgPostForbidden", err)
	}
}
