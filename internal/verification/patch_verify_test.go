package verification

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// Integration check for PATCH /admin/verification-types/{id} semantics —
// skips when the dev DB (docker-compose postgres on :5433) is unreachable.
func devStore(t *testing.T) (*Store, *pgx.Conn) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, "postgresql://japa:japa@localhost:5433/japa?sslmode=disable")
	if err != nil {
		t.Skipf("dev DB unreachable: %v", err)
	}
	t.Cleanup(func() { conn.Close(ctx) })
	return NewStore(gen.New(conn), gen.New(conn)), conn
}

func TestAdminUpdateType_BadgeOnlyPatch(t *testing.T) {
	ctx := context.Background()
	store, conn := devStore(t)
	s := &Service{store: store}

	types, err := s.store.ListTypes(ctx, "user")
	if err != nil || len(types) == 0 {
		t.Skipf("no seeded types: %v", err)
	}
	row := types[0]

	var mediaID pgtype.UUID
	if err := conn.QueryRow(ctx, "SELECT id FROM media LIMIT 1").Scan(&mediaID); err != nil {
		t.Skipf("no media rows for badge FK: %v", err)
	}
	mediaStr := mediaID.String()

	// The exact payload the admin "Set badge" action sends — previously 400.
	got, err := s.AdminUpdateType(ctx, row.ID, TypePatch{BadgeMediaID: &mediaStr, BadgeSet: true})
	if err != nil {
		t.Fatalf("badge-only patch failed: %v", err)
	}
	if got.Label != row.Label {
		t.Fatalf("label changed: %q -> %q", row.Label, got.Label)
	}
	if got.BadgeMediaID == nil || *got.BadgeMediaID != mediaStr {
		t.Fatalf("badge not set: %+v", got.BadgeMediaID)
	}

	// The "Remove badge" action sends badge_media_id: null.
	got, err = s.AdminUpdateType(ctx, row.ID, TypePatch{BadgeSet: true, BadgeMediaID: nil})
	if err != nil {
		t.Fatalf("badge-clear patch failed: %v", err)
	}
	if got.BadgeMediaID != nil {
		t.Fatalf("badge not cleared: %+v", got.BadgeMediaID)
	}
}

func TestAdminUpdateType_OmittedFieldsPreserved(t *testing.T) {
	ctx := context.Background()
	store, conn := devStore(t)
	s := &Service{store: store}

	types, err := s.store.ListTypes(ctx, "user")
	if err != nil || len(types) == 0 {
		t.Skipf("no seeded types: %v", err)
	}
	row := types[0]
	t.Cleanup(func() {
		// restore original description + label
		conn.Exec(context.Background(),
			"UPDATE verification_types SET description = $1, label = $2 WHERE id = $3",
			row.Description, row.Label, row.ID)
	})

	// Set a description, then PATCH label only — description must survive.
	desc := "integration-test description"
	if _, err := s.AdminUpdateType(ctx, row.ID, TypePatch{Description: &desc}); err != nil {
		t.Fatalf("set description: %v", err)
	}
	newLabel := "Patched Label"
	got, err := s.AdminUpdateType(ctx, row.ID, TypePatch{Label: &newLabel})
	if err != nil {
		t.Fatalf("label-only patch: %v", err)
	}
	after, err := s.store.GetTypeByID(ctx, row.ID)
	if err != nil {
		t.Fatalf("refetch: %v", err)
	}
	if after.Description == nil || *after.Description != desc {
		t.Fatalf("description wiped by label-only patch: %+v", after.Description)
	}
	if got.Label != newLabel {
		t.Fatalf("label not updated: %q", got.Label)
	}

	// Explicit empty label is still rejected.
	empty := ""
	if _, err := s.AdminUpdateType(ctx, row.ID, TypePatch{Label: &empty}); err == nil {
		t.Fatal("empty label should be rejected")
	}
}
