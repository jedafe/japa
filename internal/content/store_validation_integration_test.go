//go:build integration

package content

import (
	"context"
	"errors"
	"testing"

	"github.com/jedafe/japa/internal/testdb"
)

// TestCreateContent_InvalidStatusVisibility verifies the store-level guard
// that protects non-HTTP callers (agent tools, imports, domain services like
// comparison) from reaching the contents_status_check / contents_visibility_check
// constraints. Before this guard, an invalid value surfaced as a raw SQLSTATE
// 23514 instead of a sentinel error.
func TestCreateContent_InvalidStatusVisibility(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	svc := NewService(database)
	author := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "vis-author"})

	base := CreateContentInput{
		AuthorID:    author,
		ContentType: "discussion",
		Title:       "validation guard",
		Body:        "body",
	}

	_, err := svc.CreateContent(context.Background(), tenantID, CreateContentInput{
		AuthorID:    base.AuthorID,
		ContentType: base.ContentType,
		Title:       "bad status",
		Body:        base.Body,
		Status:      "pending_moderation",
	})
	if !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("expected ErrInvalidStatus, got %v", err)
	}

	_, err = svc.CreateContent(context.Background(), tenantID, CreateContentInput{
		AuthorID:    base.AuthorID,
		ContentType: base.ContentType,
		Title:       "bad visibility",
		Body:        base.Body,
		Visibility:  "members_only",
	})
	if !errors.Is(err, ErrInvalidVisibility) {
		t.Fatalf("expected ErrInvalidVisibility, got %v", err)
	}
}

// TestUpdateContent_InvalidStatusVisibility covers the same guard on the
// update path, where a non-empty but invalid value previously overwrote a
// valid stored value and crashed on the CHECK constraint.
func TestUpdateContent_InvalidStatusVisibility(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	svc := NewService(database)
	author := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "vis-upd-author"})

	row, err := svc.CreateContent(context.Background(), tenantID, CreateContentInput{
		AuthorID:    author,
		ContentType: "discussion",
		Title:       "update guard target",
		Body:        "body",
	})
	if err != nil {
		t.Fatalf("seed CreateContent: %v", err)
	}

	_, err = svc.UpdateContent(context.Background(), row.ID, tenantID, author, UpdateContentInput{
		Status: "pending_moderation",
	}, false)
	if !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("expected ErrInvalidStatus on update, got %v", err)
	}

	_, err = svc.UpdateContent(context.Background(), row.ID, tenantID, author, UpdateContentInput{
		Visibility: "members_only",
	}, false)
	if !errors.Is(err, ErrInvalidVisibility) {
		t.Fatalf("expected ErrInvalidVisibility on update, got %v", err)
	}
}

// TestUpdateContentStatus_InvalidStatus covers the status-transition path
// used by the agent lifecycle tools (content.publish et al.).
func TestUpdateContentStatus_InvalidStatus(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	svc := NewService(database)
	author := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "vis-status-author"})

	row, err := svc.CreateContent(context.Background(), tenantID, CreateContentInput{
		AuthorID:    author,
		ContentType: "discussion",
		Title:       "status guard target",
		Body:        "body",
	})
	if err != nil {
		t.Fatalf("seed CreateContent: %v", err)
	}

	if _, err := svc.UpdateContentStatus(context.Background(), row.ID, tenantID, "pending_moderation"); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("expected ErrInvalidStatus from UpdateContentStatus, got %v", err)
	}
}
