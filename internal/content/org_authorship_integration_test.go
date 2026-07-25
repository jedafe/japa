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

	orgID := testdb.SeedOrganization(t, database, tenantID, testdb.OrgOpts{Slug: "acme", Status: "active", OwnerUserID: owner})
	testdb.SeedOrgMembership(t, database, orgID, owner, "owner")
	testdb.SeedOrgMembership(t, database, orgID, editor, "editor")
	testdb.SeedOrgMembership(t, database, orgID, plain, "member")

	newInput := func(author pgtype.UUID) CreateContentInput {
		return CreateContentInput{
			AuthorID:       author,
			OrganizationID: orgID,
			ContentType:    "discussion",
			Title:          "Hello from Acme",
			Body:           "body",
			BodyHtml:       "<p>body</p>",
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
