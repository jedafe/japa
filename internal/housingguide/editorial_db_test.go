package housingguide

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
)

// DB-backed coverage for the §96 editorial publish hook (GuidePublishHook
// fan-out). Skips when no test DSN is available, matching the repo's
// *_db_test.go convention.

func testDB(t *testing.T) *db.DB {
	t.Helper()
	dsn := os.Getenv("JC_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("JC_DATABASE_URL")
	}
	if dsn == "" {
		t.Skip("JC_TEST_DATABASE_URL not set — skipping DB integration test")
	}
	d, err := db.New(context.Background(), dsn)
	if err != nil {
		t.Skipf("cannot connect to test DB: %v", err)
	}
	t.Cleanup(func() { d.Writer.Close(); d.Reader.Close() })
	return d
}

type alwaysOn struct{}

func (alwaysOn) IsEnabled(_ context.Context, _ string) bool { return true }

// seedAccommodationGuide inserts a published guide-category content row with
// the given field_values and returns its id. The row is deleted on cleanup.
func seedAccommodationGuide(t *testing.T, ctx context.Context, d *db.DB, fieldValues string) pgtype.UUID {
	t.Helper()

	var authorID pgtype.UUID
	if err := d.Writer.QueryRow(ctx,
		`INSERT INTO users (username, email) VALUES ($1, $2) RETURNING id`,
		fmt.Sprintf("hgtest_%d", time.Now().UnixNano()),
		fmt.Sprintf("hgtest_%d@test.dev", time.Now().UnixNano()),
	).Scan(&authorID); err != nil {
		t.Fatalf("inserting user: %v", err)
	}

	var id pgtype.UUID
	if err := d.Writer.QueryRow(ctx, `
		INSERT INTO contents (author_id, title, slug, content_type, guide_category, status, field_values)
		VALUES ($1, 'Housing test guide', $2, 'guide', 'accommodation', 'published', $3::jsonb)
		RETURNING id`,
		authorID, fmt.Sprintf("hg-test-%d", time.Now().UnixNano()), fieldValues,
	).Scan(&id); err != nil {
		t.Fatalf("inserting content: %v", err)
	}

	t.Cleanup(func() {
		d.Writer.Exec(context.Background(), `DELETE FROM contents WHERE id = $1`, id)
	})

	return id
}

func cleanupGuide(t *testing.T, d *db.DB, id pgtype.UUID) {
	t.Helper()
	t.Cleanup(func() {
		d.Writer.Exec(context.Background(), `DELETE FROM housing_guides WHERE id = $1`, id)
	})
}

func TestOnGuidePublished_ProvisionsDraftOnNaturalKey(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	svc := NewService(NewStore(d), nil)
	svc.SetFeatureChecker(alwaysOn{})

	fv := `{
		"destination_country": "CO",
		"accommodation_type": "flatshare",
		"landlord_requirements": "ID + deposit",
		"scam_categories": "phantom listings\nadvance-fee fraud",
		"tips": "Always visit before paying.",
		"rent_ranges": [{"tier":"inner","studio":"900","one_bed":"1400","currency":"cop"}],
		"search_platforms": [{"name":"CompartoApto","url":"https://compartoapt.example","description":"local board"}],
		"last_verified": "2026-01-15"
	}`
	contentID := seedAccommodationGuide(t, ctx, d, fv)

	if err := svc.OnGuidePublished(ctx, contentID, "accommodation", []byte(fv)); err != nil {
		t.Fatalf("OnGuidePublished: %v", err)
	}

	var dest pgtype.UUID
	if err := d.Reader.QueryRow(ctx,
		`SELECT id FROM spaces WHERE level='country' AND UPPER(geo_code)='CO'`).Scan(&dest); err != nil {
		t.Fatalf("resolving CO space: %v", err)
	}

	id, err := svc.store.GuideIDForCombination(ctx, dest, pgtype.UUID{}, "flatshare")
	if err != nil {
		t.Fatalf("combination lookup after publish: %v", err)
	}
	cleanupGuide(t, d, id)

	var status, notes string
	var guideID *string
	var scam []string
	var platforms, ranges []byte
	if err := d.Reader.QueryRow(ctx, `
		SELECT status, COALESCE(hitl_review_notes,''), guide_id::text, scam_categories, search_platforms, rent_ranges
		FROM housing_guides WHERE id = $1`, id,
	).Scan(&status, &notes, &guideID, &scam, &platforms, &ranges); err != nil {
		t.Fatalf("reading provisioned row: %v", err)
	}

	if status != StatusDraft {
		t.Errorf("status = %q, want draft (ADR-088 HITL)", status)
	}
	if guideID == nil || *guideID != contentID.String() {
		t.Errorf("guide_id = %v, want %s", guideID, contentID)
	}
	if len(scam) != 2 || scam[0] != "phantom listings" {
		t.Errorf("scam_categories = %v", scam)
	}
	var rr map[string]map[string]any
	if err := jsonUnmarshal(ranges, &rr); err != nil || rr["inner"]["1bed"] != "1400" {
		t.Errorf("rent_ranges = %s (err %v)", ranges, err)
	}
	var sp []map[string]any
	if err := jsonUnmarshal(platforms, &sp); err != nil || len(sp) != 1 || sp[0]["name"] != "CompartoApto" {
		t.Errorf("search_platforms = %s (err %v)", platforms, err)
	}

	// Bidirectional link: field_values.guide_id backfilled on the content.
	var backfilled string
	if err := d.Reader.QueryRow(ctx,
		`SELECT field_values->>'guide_id' FROM contents WHERE id = $1`, contentID).Scan(&backfilled); err != nil {
		t.Fatalf("reading backfill: %v", err)
	}
	if backfilled != id.String() {
		t.Errorf("field_values.guide_id = %q, want %s", backfilled, id)
	}

	// Idempotent: republish claims the same row, never duplicates.
	if err := svc.OnGuidePublished(ctx, contentID, "accommodation", []byte(fv)); err != nil {
		t.Fatalf("second OnGuidePublished: %v", err)
	}
	var count int
	if err := d.Reader.QueryRow(ctx, `
		SELECT count(*) FROM housing_guides
		WHERE destination_space_id = $1 AND COALESCE(accommodation_type,'') = 'flatshare'`, dest,
	).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("rows after republish = %d, want 1 (idempotent claim)", count)
	}
}

func TestOnGuidePublished_SkipsForeignCategoryAndFeatureOff(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	svc := NewService(NewStore(d), nil)
	svc.SetFeatureChecker(alwaysOn{})
	if err := svc.OnGuidePublished(ctx, pgtype.UUID{Valid: true}, "attestation", []byte(`{}`)); err != nil {
		t.Errorf("foreign category returned error: %v", err)
	}

	off := NewService(NewStore(d), nil)
	off.SetFeatureChecker(featureOff{})
	if err := off.OnGuidePublished(ctx, pgtype.UUID{Valid: true}, "accommodation", []byte(`{"destination_country":"CO"}`)); err != nil {
		t.Errorf("feature-off returned error: %v", err)
	}
}

type featureOff struct{}

func (featureOff) IsEnabled(_ context.Context, _ string) bool { return false }

func jsonUnmarshal(b []byte, v any) error {
	if len(b) == 0 {
		return nil
	}
	return json.Unmarshal(b, v)
}
