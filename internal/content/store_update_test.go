package content

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// buildUpdateContentParams must never let an omitted input field NULL or zero
// a stored value: every unconditional sqlc.narg column is seeded from the
// existing row. These tables pin that contract per field.
func TestBuildUpdateContentParams_PreservesOmittedFields(t *testing.T) {
	t.Parallel()

	contentID := mustUUID(t, "550e8400-e29b-41d4-a716-446655440001")
	orgID := mustUUID(t, "550e8400-e29b-41d4-a716-446655440002")
	headerID := mustUUID(t, "550e8400-e29b-41d4-a716-446655440003")
	featuredID := mustUUID(t, "550e8400-e29b-41d4-a716-446655440004")
	spaceID := mustUUID(t, "550e8400-e29b-41d4-a716-446655440005")

	storedPublishAt := pgtype.Timestamptz{Time: time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC), Valid: true}
	storedPublishedAt := pgtype.Timestamptz{Time: time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC), Valid: true}
	storedCanonical := strPtr("https://example.com/canonical")
	storedMetaTitle := strPtr("stored meta title")
	storedMetaDesc := strPtr("stored meta description")
	storedOgImage := strPtr("https://example.com/og.png")
	storedFieldValues := []byte(`{"k":"v"}`)

	existing := gen.GetContentByIDRow{
		ID:              contentID,
		Title:           "Stored title",
		Slug:            "stored-slug",
		Body:            "stored body",
		FieldValues:     storedFieldValues,
		GeoSpaceID:      spaceID,
		OrganizationID:  orgID,
		HeaderImageID:   headerID,
		FeaturedImageID: featuredID,
		CanonicalUrl:    storedCanonical,
		MetaTitle:       storedMetaTitle,
		MetaDescription: storedMetaDesc,
		OgImageUrl:      storedOgImage,
		Status:          StatusPublished,
		Visibility:      VisibilityPublic,
		PublishAt:       storedPublishAt,
		PublishedAt:     storedPublishedAt,
		IsLocked:        true,
		IsPinned:        true,
	}

	t.Run("omitted fields preserve stored values", func(t *testing.T) {
		t.Parallel()

		params := buildUpdateContentParams(contentID, UpdateContentInput{
			Title:      "New title",
			Body:       "new body",
			Status:     StatusPublished,
			Visibility: VisibilityPublic,
		}, existing, "new-slug", spaceID)

		if params.OrganizationID != orgID {
			t.Errorf("OrganizationID = %v, want stored %v", params.OrganizationID, orgID)
		}
		if params.HeaderImageID != headerID {
			t.Errorf("HeaderImageID = %v, want stored %v", params.HeaderImageID, headerID)
		}
		if params.FeaturedImageID != featuredID {
			t.Errorf("FeaturedImageID = %v, want stored %v", params.FeaturedImageID, featuredID)
		}
		if params.CanonicalUrl != storedCanonical {
			t.Errorf("CanonicalUrl = %v, want stored %v", params.CanonicalUrl, storedCanonical)
		}
		if params.MetaTitle != storedMetaTitle {
			t.Errorf("MetaTitle = %v, want stored %v", params.MetaTitle, storedMetaTitle)
		}
		if params.MetaDescription != storedMetaDesc {
			t.Errorf("MetaDescription = %v, want stored %v", params.MetaDescription, storedMetaDesc)
		}
		if params.OgImageUrl != storedOgImage {
			t.Errorf("OgImageUrl = %v, want stored %v", params.OgImageUrl, storedOgImage)
		}
		if params.PublishAt != storedPublishAt {
			t.Errorf("PublishAt = %v, want stored %v", params.PublishAt, storedPublishAt)
		}
		if params.PublishedAt != storedPublishedAt {
			t.Errorf("PublishedAt = %v, want stored %v", params.PublishedAt, storedPublishedAt)
		}
		if !params.IsLocked {
			t.Error("IsLocked = false, want stored true")
		}
		if !params.IsPinned {
			t.Error("IsPinned = false, want stored true")
		}
		if string(params.FieldValues) != string(storedFieldValues) {
			t.Errorf("FieldValues = %s, want stored %s", params.FieldValues, storedFieldValues)
		}
	})

	t.Run("explicit sets write through", func(t *testing.T) {
		t.Parallel()

		newOrg := mustUUID(t, "550e8400-e29b-41d4-a716-446655440010")
		newHeader := mustUUID(t, "550e8400-e29b-41d4-a716-446655440011")
		newFeatured := mustUUID(t, "550e8400-e29b-41d4-a716-446655440012")
		newPublishAt := pgtype.Timestamptz{Time: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), Valid: true}
		unlock, unpin := false, false

		params := buildUpdateContentParams(contentID, UpdateContentInput{
			Title:           "New title",
			Body:            "new body",
			Status:          StatusDraft,
			Visibility:      VisibilityMembers,
			OrganizationID:  newOrg,
			HeaderImageID:   newHeader,
			FeaturedImageID: newFeatured,
			CanonicalUrl:    strPtr("https://example.com/new-canonical"),
			MetaTitle:       strPtr("new meta title"),
			MetaDescription: strPtr("new meta description"),
			OgImageUrl:      strPtr("https://example.com/new-og.png"),
			PublishAt:       newPublishAt,
			IsLocked:        &unlock,
			IsPinned:        &unpin,
			FieldValues:     []byte(`{"a":1}`),
		}, existing, "new-slug", spaceID)

		if params.OrganizationID != newOrg {
			t.Errorf("OrganizationID = %v, want %v", params.OrganizationID, newOrg)
		}
		if params.HeaderImageID != newHeader {
			t.Errorf("HeaderImageID = %v, want %v", params.HeaderImageID, newHeader)
		}
		if params.FeaturedImageID != newFeatured {
			t.Errorf("FeaturedImageID = %v, want %v", params.FeaturedImageID, newFeatured)
		}
		if params.CanonicalUrl == nil || *params.CanonicalUrl != "https://example.com/new-canonical" {
			t.Errorf("CanonicalUrl = %v, want new canonical", params.CanonicalUrl)
		}
		if params.MetaTitle == nil || *params.MetaTitle != "new meta title" {
			t.Errorf("MetaTitle = %v, want new meta title", params.MetaTitle)
		}
		if params.MetaDescription == nil || *params.MetaDescription != "new meta description" {
			t.Errorf("MetaDescription = %v, want new meta description", params.MetaDescription)
		}
		if params.OgImageUrl == nil || *params.OgImageUrl != "https://example.com/new-og.png" {
			t.Errorf("OgImageUrl = %v, want new og image", params.OgImageUrl)
		}
		if params.PublishAt != newPublishAt {
			t.Errorf("PublishAt = %v, want %v", params.PublishAt, newPublishAt)
		}
		if params.PublishedAt != storedPublishedAt {
			t.Errorf("PublishedAt = %v, must stay %v (stamped by UpdateContentStatus only)", params.PublishedAt, storedPublishedAt)
		}
		if params.IsLocked {
			t.Error("IsLocked = true, want explicit false")
		}
		if params.IsPinned {
			t.Error("IsPinned = true, want explicit false")
		}
		if string(params.FieldValues) != `{"a":1}` {
			t.Errorf("FieldValues = %s, want explicit payload", params.FieldValues)
		}
	})

	t.Run("clear flags NULL their columns", func(t *testing.T) {
		t.Parallel()

		params := buildUpdateContentParams(contentID, UpdateContentInput{
			Title:                "New title",
			Status:               StatusDraft,
			Visibility:           VisibilityPublic,
			ClearPublishAt:       true,
			ClearHeaderImageID:   true,
			ClearFeaturedImageID: true,
		}, existing, "new-slug", spaceID)

		if params.PublishAt.Valid {
			t.Errorf("PublishAt = %v, want NULL after ClearPublishAt", params.PublishAt)
		}
		if params.HeaderImageID.Valid {
			t.Errorf("HeaderImageID = %v, want NULL after ClearHeaderImageID", params.HeaderImageID)
		}
		if params.FeaturedImageID.Valid {
			t.Errorf("FeaturedImageID = %v, want NULL after ClearFeaturedImageID", params.FeaturedImageID)
		}
		// Clears must not disturb other preserved fields.
		if params.PublishedAt != storedPublishedAt {
			t.Errorf("PublishedAt = %v, want stored %v", params.PublishedAt, storedPublishedAt)
		}
		if params.OrganizationID != orgID {
			t.Errorf("OrganizationID = %v, want stored %v", params.OrganizationID, orgID)
		}
	})

	t.Run("clear flag does not override an explicit set", func(t *testing.T) {
		t.Parallel()

		newFeatured := mustUUID(t, "550e8400-e29b-41d4-a716-446655440020")
		newPublishAt := pgtype.Timestamptz{Time: time.Date(2027, 2, 1, 0, 0, 0, 0, time.UTC), Valid: true}

		params := buildUpdateContentParams(contentID, UpdateContentInput{
			Title:                "New title",
			Status:               StatusDraft,
			Visibility:           VisibilityPublic,
			FeaturedImageID:      newFeatured,
			ClearFeaturedImageID: true,
			PublishAt:            newPublishAt,
			ClearPublishAt:       true,
		}, existing, "new-slug", spaceID)

		if params.FeaturedImageID != newFeatured {
			t.Errorf("FeaturedImageID = %v, want explicit %v (set wins over clear)", params.FeaturedImageID, newFeatured)
		}
		if params.PublishAt != newPublishAt {
			t.Errorf("PublishAt = %v, want explicit %v (set wins over clear)", params.PublishAt, newPublishAt)
		}
	})

	t.Run("published_at passes through even when stored NULL", func(t *testing.T) {
		t.Parallel()

		draft := existing
		draft.PublishedAt = pgtype.Timestamptz{}

		params := buildUpdateContentParams(contentID, UpdateContentInput{
			Title:      "New title",
			Status:     StatusDraft,
			Visibility: VisibilityPublic,
		}, draft, "new-slug", spaceID)

		if params.PublishedAt.Valid {
			t.Errorf("PublishedAt = %v, want NULL passthrough for a draft", params.PublishedAt)
		}
	})
}
