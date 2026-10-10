// Package servicedir — store_mentorship.go holds the data-access methods
// for the mentor shadow listing (000824 unification): a mentor's public
// presence in the service directory is a contents row
// (content_type='service_listing', slug 'mentor-<username>') plus a
// service_listing_meta row with listing_kind='mentorship'. These queries
// mirror the 000824 migration exactly — shadow rows are spaceless, so the
// contents insert deliberately bypasses content.Service.CreateContent
// (which requires a primary space for published rows).
//
// Ref: docs/superpowers/plans/2026-10-09-services-mentorship-unification.md
package servicedir

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// GetMentorshipListing returns the mentor's shadow listing — any status
// (draft fallback shadows created by the migration are returned too, so
// the ensure path can republish them), excluding soft-deleted rows.
// pgx.ErrNoRows when the mentor has no live shadow.
func (s *Store) GetMentorshipListing(ctx context.Context, authorID pgtype.UUID) (ServiceRow, error) {
	row, err := s.read.GetMentorshipListingByAuthor(ctx, authorID)
	if err != nil {
		return ServiceRow{}, fmt.Errorf("servicedir: get mentorship listing: %w", err)
	}

	return rowFromMentorShadow(row), nil
}

// CreateMentorShadow inserts the shadow contents row, its meta, and the
// user_profiles link in one transaction. The contents INSERT is
// ON CONFLICT DO NOTHING, so a slug/hash collision (a concurrent ensure)
// surfaces as pgx.ErrNoRows — the service layer re-reads the winning row.
func (s *Store) CreateMentorShadow(ctx context.Context, authorID pgtype.UUID, title, slug, body, contentHash string) (pgtype.UUID, error) {
	return db.WithTxResult(ctx, s.pool.Writer, func(q *gen.Queries) (pgtype.UUID, error) {
		contentID, err := q.CreateMentorshipShadowContent(ctx, gen.CreateMentorshipShadowContentParams{
			AuthorID:    authorID,
			Title:       title,
			Slug:        slug,
			Body:        body,
			ContentHash: &contentHash,
		})
		if err != nil {
			return pgtype.UUID{}, err
		}

		if err := q.EnsureMentorshipShadowMeta(ctx, contentID); err != nil {
			return pgtype.UUID{}, fmt.Errorf("servicedir: mentor shadow meta: %w", err)
		}

		if err := q.SetMentorListingContentID(ctx, gen.SetMentorListingContentIDParams{
			ContentID: contentID, UserID: authorID,
		}); err != nil {
			return pgtype.UUID{}, fmt.Errorf("servicedir: mentor listing link: %w", err)
		}

		return contentID, nil
	})
}

// UpdateMentorShadow refreshes an existing shadow in place (title/body/
// content_hash, republish a preserved draft) and re-asserts the profile
// link — all in one transaction.
func (s *Store) UpdateMentorShadow(ctx context.Context, contentID, authorID pgtype.UUID, title, body, contentHash string) error {
	return db.WithTx(ctx, s.pool.Writer, func(q *gen.Queries) error {
		n, err := q.UpdateMentorshipShadowContent(ctx, gen.UpdateMentorshipShadowContentParams{
			ID:          contentID,
			Title:       title,
			Body:        body,
			ContentHash: &contentHash,
		})
		if err != nil {
			return fmt.Errorf("servicedir: update mentor shadow: %w", err)
		}

		if n == 0 {
			return errNotFound
		}

		if err := q.EnsureMentorshipShadowMeta(ctx, contentID); err != nil {
			return fmt.Errorf("servicedir: mentor shadow meta: %w", err)
		}

		return q.SetMentorListingContentID(ctx, gen.SetMentorListingContentIDParams{
			ContentID: contentID, UserID: authorID,
		})
	})
}

// RetireMentorShadow soft-deletes the mentor's live shadow contents row
// (Rule 11) and clears user_profiles.mentor_listing_content_id in one
// transaction. Migrated/live bookings keep pointing at the meta row, which
// survives the contents soft-delete for audit.
func (s *Store) RetireMentorShadow(ctx context.Context, authorID pgtype.UUID) error {
	return db.WithTx(ctx, s.pool.Writer, func(q *gen.Queries) error {
		if err := q.SoftDeleteMentorshipShadow(ctx, authorID); err != nil {
			return fmt.Errorf("servicedir: retire mentor shadow: %w", err)
		}

		return q.SetMentorListingContentID(ctx, gen.SetMentorListingContentIDParams{
			ContentID: pgtype.UUID{}, UserID: authorID,
		})
	})
}

// rowFromMentorShadow maps the shadow-listing row to the shared domain
// row — same SELECT shape as GetServiceListingByContentID.
func rowFromMentorShadow(r gen.GetMentorshipListingByAuthorRow) ServiceRow {
	return ServiceRow{
		ID: r.ID, AuthorID: r.AuthorID, Title: r.Title, Slug: r.Slug,
		Body: r.Body, BodyHTML: r.BodyHtml, Status: r.Status,
		Visibility: r.Visibility, ViewCount: r.ViewCount,
		CommentCount: r.CommentCount, PublishedAt: r.PublishedAt,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		FeaturedImageID:       r.FeaturedImageID,
		FeaturedStorageBucket: r.FeaturedStorageBucket,
		FeaturedStoragePath:   r.FeaturedStoragePath,
		RatingCount:           r.RatingCount,
		RatingAvg:             r.RatingAvg,
		MetaID:                r.MetaID, OrgID: r.OrgID, OrgName: r.OrgName, OrgSlug: r.OrgSlug,
		ServiceCategory: &r.ServiceCategory,
		DestSpaceID:     r.DestinationSpaceID, DestSpaceName: r.DestSpaceName, DestSpaceCode: r.DestSpaceCode,
		CitySpaceID: r.CitySpaceID, CitySpaceName: r.CitySpaceName, CitySpaceCode: r.CitySpaceCode,
		OriginCommunities: r.OriginCommunities, IsRemote: r.IsRemote,
		WebsiteURL: r.WebsiteUrl, BookingURL: r.BookingUrl,
		PriceRange: r.PriceRange, IsVerified: r.IsVerified,
		VerificationType: r.VerificationType,
		ClaimStatus:      &r.ClaimStatus, ClaimedByOrgID: r.ClaimedByOrgID,
		ClaimedOrgName: r.ClaimedOrgName, VerifiedBy: r.VerifiedBy,
		VerifiedAt: r.VerifiedAt, AuthorName: r.AuthorName, AuthorAvatar: r.AuthorAvatar,
		AuthorDisplayName: r.AuthorDisplayName, MentorIsMentor: r.IsMentor,
		MentorCapacity: r.MentorCapacity, MentorActiveMenteeCount: r.ActiveMenteeCount,
		MentorAreas: r.MentorAreas, MentorSince: r.MentorSince,
		ListingKind: r.ListingKind, PriceAmount: r.PriceAmount,
		PriceCurrency: r.PriceCurrency, PricePer: r.PricePer,
		PurchaseMode: r.PurchaseMode, Capacity: r.Capacity,
		AvailableFrom: r.AvailableFrom, AvailableUntil: r.AvailableUntil,
		Faqs: r.Faqs, Steps: r.Steps,
	}
}
