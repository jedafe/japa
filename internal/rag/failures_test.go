package rag

import (
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
)

func TestIndexFailureRoundtrip(t *testing.T) {
	dsn := os.Getenv("JC_DATABASE_URL")
	if dsn == "" {
		t.Skip("JC_DATABASE_URL not set; skipping integration test")
	}

	database, err := db.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}

	t.Cleanup(database.Writer.Close)
	t.Cleanup(database.Reader.Close)

	ctx := t.Context()
	store := NewStore(database)

	var entityID pgtype.UUID
	if err := entityID.Scan("11111111-1111-1111-1111-111111111111"); err != nil {
		t.Fatalf("scan uuid: %v", err)
	}

	t.Cleanup(func() {
		_ = store.ClearIndexFailures(t.Context(), "")
	})

	if err := store.RecordIndexFailure(ctx, "space", entityID, "index", errors.New("smoke failure")); err != nil {
		t.Fatalf("RecordIndexFailure: %v", err)
	}

	failures, err := store.ListIndexFailures(ctx, 50)
	if err != nil {
		t.Fatalf("ListIndexFailures: %v", err)
	}

	found := false

	for _, f := range failures {
		if f.EntityID == entityID.String() && f.EntityType == "space" && f.Operation == "index" {
			found = true

			if f.Error != "smoke failure" {
				t.Errorf("failure error = %q, want %q", f.Error, "smoke failure")
			}
		}
	}

	if !found {
		t.Fatal("recorded failure not found in ListIndexFailures")
	}

	n, err := store.CountIndexFailures24h(ctx)
	if err != nil {
		t.Fatalf("CountIndexFailures24h: %v", err)
	}

	if n < 1 {
		t.Errorf("CountIndexFailures24h = %d, want >= 1", n)
	}

	if err := store.ClearIndexFailure(ctx, "space", entityID, "index"); err != nil {
		t.Fatalf("ClearIndexFailure: %v", err)
	}

	failures, err = store.ListIndexFailures(ctx, 50)
	if err != nil {
		t.Fatalf("ListIndexFailures after clear: %v", err)
	}

	for _, f := range failures {
		if f.EntityID == entityID.String() {
			t.Fatal("failure row still present after ClearIndexFailure")
		}
	}
}
