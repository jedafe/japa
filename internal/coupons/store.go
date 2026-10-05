package coupons

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store provides database access for coupons and affiliate providers.
type Store struct {
	read  *gen.Queries
	write *gen.Queries
}

func newStore(read, write *gen.Queries) *Store {
	return &Store{read: read, write: write}
}

// ── Affiliate Providers ──────────────────────────────────────────────

// rowToProvider converts any sqlc-generated affiliate provider row type to the
// base AffiliateProvider model. All row types share the same columns; the only
// difference is struct field order which sqlc generates per-query.
func (s *Store) createProvider(ctx context.Context, p gen.CreateAffiliateProviderParams) (gen.AffiliateProvider, error) {
	row, err := s.write.CreateAffiliateProvider(ctx, p)
	if err != nil {
		return gen.AffiliateProvider{}, err
	}

	return rowToProvider(row), nil
}

func (s *Store) getProviderByID(ctx context.Context, id pgtype.UUID) (gen.AffiliateProvider, error) {
	row, err := s.read.GetAffiliateProviderByID(ctx, id)
	if err != nil {
		return gen.AffiliateProvider{}, err
	}

	return rowToProvider(row), nil
}

func (s *Store) getProviderBySlug(ctx context.Context, slug string) (gen.AffiliateProvider, error) {
	row, err := s.read.GetAffiliateProviderBySlug(ctx, slug)
	if err != nil {
		return gen.AffiliateProvider{}, err
	}

	return rowToProvider(row), nil
}

func (s *Store) getProviderByOrgID(ctx context.Context, orgID pgtype.UUID) (gen.AffiliateProvider, error) {
	row, err := s.read.GetAffiliateProviderByOrgID(ctx, orgID)
	if err != nil {
		return gen.AffiliateProvider{}, err
	}
	return rowToProvider(row), nil
}

func (s *Store) listActiveDealPartners(ctx context.Context) ([]gen.ListActiveDealPartnersRow, error) {
	return s.read.ListActiveDealPartners(ctx)
}

func (s *Store) listProviders(ctx context.Context, isActive bool, limit, offset int32) ([]gen.AffiliateProvider, error) {
	rows, err := s.read.ListAffiliateProviders(ctx, gen.ListAffiliateProvidersParams{

		IsActive:   isActive,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, err
	}

	var result []gen.AffiliateProvider
	for _, r := range rows {
		result = append(result, rowToProvider(r))
	}

	return result, nil
}

func (s *Store) updateProvider(ctx context.Context, p gen.UpdateAffiliateProviderParams) (gen.AffiliateProvider, error) {
	row, err := s.write.UpdateAffiliateProvider(ctx, p)
	if err != nil {
		return gen.AffiliateProvider{}, err
	}

	return rowToProvider(row), nil
}

func (s *Store) deleteProvider(ctx context.Context, id pgtype.UUID) error {
	return s.write.DeleteAffiliateProvider(ctx, id)
}

func (s *Store) countCouponsByProvider(ctx context.Context, providerID pgtype.UUID) (int64, error) {
	return s.read.CountCouponsByProvider(ctx, providerID)
}

// ── Coupons ─────────────────────────────────────────────────────────

func (s *Store) createCoupon(ctx context.Context, c gen.CreateCouponParams) (gen.CreateCouponRow, error) {
	return s.write.CreateCoupon(ctx, c)
}

func (s *Store) getCouponByID(ctx context.Context, id pgtype.UUID) (gen.GetCouponByIDRow, error) {
	return s.read.GetCouponByID(ctx, id)
}

func (s *Store) getCouponByCode(ctx context.Context, code *string) (gen.GetCouponByCodeRow, error) {
	return s.read.GetCouponByCode(ctx, code)
}

func (s *Store) getCouponBySlug(ctx context.Context, slug string) (gen.GetCouponBySlugRow, error) {
	return s.read.GetCouponBySlug(ctx, slug)
}

func (s *Store) listCoupons(ctx context.Context, status, couponType, search *string, providerID pgtype.UUID, limit, offset int32) ([]gen.ListCouponsRow, error) {
	return s.read.ListCoupons(ctx, gen.ListCouponsParams{

		Status:     status,
		CouponType: couponType,
		ProviderID: providerID,
		Search:     search,
		PageLimit:  limit,
		PageOffset: offset,
	})
}

func (s *Store) listActiveCoupons(ctx context.Context, providerID pgtype.UUID, couponType, search *string, limit, offset int32) ([]gen.ListActiveCouponsRow, error) {
	return s.read.ListActiveCoupons(ctx, gen.ListActiveCouponsParams{

		ProviderID: providerID,
		CouponType: couponType,
		Search:     search,
		PageLimit:  limit,
		PageOffset: offset,
	})
}

func (s *Store) listCouponsByOrganization(ctx context.Context, orgID pgtype.UUID, limit, offset int32) ([]gen.ListCouponsByOrganizationRow, error) {
	return s.read.ListCouponsByOrganization(ctx, gen.ListCouponsByOrganizationParams{
		OrganizationID: orgID, PageLimit: limit, PageOffset: offset,
	})
}

func (s *Store) updateCoupon(ctx context.Context, c gen.UpdateCouponParams) (gen.UpdateCouponRow, error) {
	return s.write.UpdateCoupon(ctx, c)
}

func (s *Store) deleteCoupon(ctx context.Context, id pgtype.UUID) error {
	return s.write.DeleteCoupon(ctx, id)
}

// ── Slug History ────────────────────────────────────────────────────

// createSlugHistory records a coupon slug rename (entity_type 'coupon') so
// old /deal URLs keep resolving via the slug_history redirect table (§55).
func (s *Store) createSlugHistory(ctx context.Context, couponID pgtype.UUID, oldSlug, newSlug string) error {
	_, err := s.write.CreateSlugHistory(ctx, gen.CreateSlugHistoryParams{
		EntityType: "coupon",
		EntityID:   couponID,
		OldSlug:    oldSlug,
		NewSlug:    newSlug,
	})
	return err
}

// getSlugRedirect returns the most recent slug_history redirect for a
// coupon slug, or pgx.ErrNoRows when none exists.
func (s *Store) getSlugRedirect(ctx context.Context, oldSlug string) (gen.SlugHistory, error) {
	return s.read.GetSlugRedirect(ctx, gen.GetSlugRedirectParams{
		EntityType: "coupon",
		OldSlug:    oldSlug,
	})
}

// ── Coupon Events ───────────────────────────────────────────────────

func (s *Store) createCouponEvent(ctx context.Context, e gen.CreateCouponEventParams) (gen.CouponEvent, error) {
	return s.write.CreateCouponEvent(ctx, e)
}

// ── Render Helpers ──────────────────────────────────────────────────

func (s *Store) getCouponForRender(ctx context.Context, id pgtype.UUID) (gen.GetCouponForRenderRow, error) {
	return s.read.GetCouponForRender(ctx, id)
}

func (s *Store) getAffiliateProviderForRender(ctx context.Context, id pgtype.UUID) (gen.GetAffiliateProviderForRenderRow, error) {
	return s.read.GetAffiliateProviderForRender(ctx, id)
}

// ── Count ───────────────────────────────────────────────────────────

func (s *Store) countCouponsByPlatform(ctx context.Context) (int64, error) {
	return s.read.CountCoupons(ctx)
}

// ── Affiliate Enhancement Queries (BE-LOW-002) ─────────────────────
// These methods wrap sqlc-generated queries, replacing the raw SQL that
// was previously inlined in the handlers.

func (s *Store) listAffiliateScanResults(ctx context.Context, arg gen.ListAffiliateScanResultsParams) ([]gen.ListAffiliateScanResultsRow, error) {
	return s.read.ListAffiliateScanResults(ctx, arg)
}

func (s *Store) reviewAffiliateScanResult(ctx context.Context, arg gen.ReviewAffiliateScanResultParams) (int64, error) {
	return s.write.ReviewAffiliateScanResult(ctx, arg)
}

func (s *Store) listAffiliateCommissions(ctx context.Context, arg gen.ListAffiliateCommissionsParams) ([]gen.ListAffiliateCommissionsRow, error) {
	return s.read.ListAffiliateCommissions(ctx, arg)
}

func (s *Store) updateProviderAuthorCommission(ctx context.Context, arg gen.UpdateProviderAuthorCommissionParams) (int64, error) {
	return s.write.UpdateProviderAuthorCommission(ctx, arg)
}

// ── Deal Feed Sources (Section 55 Deal Feed Import) ─────────────────

func (s *Store) createFeedSource(ctx context.Context, p gen.CreateCouponFeedSourceParams) (gen.CouponFeedSource, error) {
	return s.write.CreateCouponFeedSource(ctx, p)
}

func (s *Store) getFeedSourceByID(ctx context.Context, id pgtype.UUID) (gen.CouponFeedSource, error) {
	return s.read.GetCouponFeedSourceByID(ctx, id)
}

func (s *Store) listFeedSources(ctx context.Context) ([]gen.ListCouponFeedSourcesRow, error) {
	return s.read.ListCouponFeedSources(ctx)
}

func (s *Store) listEnabledFeedSources(ctx context.Context) ([]gen.CouponFeedSource, error) {
	return s.read.ListEnabledCouponFeedSources(ctx)
}

func (s *Store) updateFeedSource(ctx context.Context, p gen.UpdateCouponFeedSourceParams) (gen.CouponFeedSource, error) {
	return s.write.UpdateCouponFeedSource(ctx, p)
}

func (s *Store) deleteFeedSource(ctx context.Context, id pgtype.UUID) error {
	return s.write.DeleteCouponFeedSource(ctx, id)
}

func (s *Store) updateFeedSourceSyncStatus(ctx context.Context, p gen.UpdateCouponFeedSourceSyncStatusParams) error {
	return s.write.UpdateCouponFeedSourceSyncStatus(ctx, p)
}

func (s *Store) upsertFeedCoupon(ctx context.Context, p gen.UpsertFeedCouponParams) (gen.UpsertFeedCouponRow, error) {
	return s.write.UpsertFeedCoupon(ctx, p)
}

func (s *Store) getCouponByFeedExternal(ctx context.Context, feedID pgtype.UUID, externalID string) (gen.GetCouponByFeedExternalRow, error) {
	return s.read.GetCouponByFeedExternal(ctx, gen.GetCouponByFeedExternalParams{
		FeedID: feedID, ExternalID: &externalID,
	})
}

func (s *Store) expireFeedCouponsNotSeen(ctx context.Context, feedID pgtype.UUID, seenIDs []string) (int64, error) {
	rows, err := s.write.ExpireFeedCouponsNotSeen(ctx, gen.ExpireFeedCouponsNotSeenParams{
		FeedID: feedID, SeenIds: seenIDs,
	})
	if err != nil {
		return 0, err
	}

	return int64(len(rows)), nil
}

func (s *Store) getProviderIDBySlug(ctx context.Context, slug string) (pgtype.UUID, error) {
	return s.read.GetAffiliateProviderIDBySlug(ctx, slug)
}

// ── Error helpers ───────────────────────────────────────────────────

func isNoRows(err error) bool {
	return err != nil && errors.Is(err, pgx.ErrNoRows)
}

// rowToProvider converts any affiliate provider row type to the canonical model.
func rowToProvider[T any](row T) gen.AffiliateProvider {
	switch r := any(row).(type) {
	case gen.CreateAffiliateProviderRow:
		return gen.AffiliateProvider{
			ID: r.ID, Name: r.Name, Slug: r.Slug,
			Description: r.Description, ProviderType: r.ProviderType, LogoMediaID: r.LogoMediaID,
			BaseUrl: r.BaseUrl, TrackingParam: r.TrackingParam, TrackingValue: r.TrackingValue,
			CommissionRate: r.CommissionRate, IsActive: r.IsActive, IsFeatured: r.IsFeatured,
			AuthorCommissionPct: r.AuthorCommissionPct, Category: r.Category, OrganizationID: r.OrganizationID,
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		}
	case gen.GetAffiliateProviderByIDRow:
		return gen.AffiliateProvider{
			ID: r.ID, Name: r.Name, Slug: r.Slug,
			Description: r.Description, ProviderType: r.ProviderType, LogoMediaID: r.LogoMediaID,
			BaseUrl: r.BaseUrl, TrackingParam: r.TrackingParam, TrackingValue: r.TrackingValue,
			CommissionRate: r.CommissionRate, IsActive: r.IsActive, IsFeatured: r.IsFeatured,
			AuthorCommissionPct: r.AuthorCommissionPct, Category: r.Category, OrganizationID: r.OrganizationID,
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		}
	case gen.GetAffiliateProviderByOrgIDRow:
		return gen.AffiliateProvider{
			ID: r.ID, Name: r.Name, Slug: r.Slug,
			Description: r.Description, ProviderType: r.ProviderType, LogoMediaID: r.LogoMediaID,
			BaseUrl: r.BaseUrl, TrackingParam: r.TrackingParam, TrackingValue: r.TrackingValue,
			CommissionRate: r.CommissionRate, IsActive: r.IsActive, IsFeatured: r.IsFeatured,
			AuthorCommissionPct: r.AuthorCommissionPct, Category: r.Category, OrganizationID: r.OrganizationID,
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		}
	case gen.ListAffiliateProvidersRow:
		return gen.AffiliateProvider{
			ID: r.ID, Name: r.Name, Slug: r.Slug,
			Description: r.Description, ProviderType: r.ProviderType, LogoMediaID: r.LogoMediaID,
			BaseUrl: r.BaseUrl, TrackingParam: r.TrackingParam, TrackingValue: r.TrackingValue,
			CommissionRate: r.CommissionRate, IsActive: r.IsActive, IsFeatured: r.IsFeatured,
			AuthorCommissionPct: r.AuthorCommissionPct, Category: r.Category, OrganizationID: r.OrganizationID,
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		}
	case gen.GetAffiliateProviderBySlugRow:
		return gen.AffiliateProvider{
			ID: r.ID, Name: r.Name, Slug: r.Slug,
			Description: r.Description, ProviderType: r.ProviderType, LogoMediaID: r.LogoMediaID,
			BaseUrl: r.BaseUrl, TrackingParam: r.TrackingParam, TrackingValue: r.TrackingValue,
			CommissionRate: r.CommissionRate, IsActive: r.IsActive, IsFeatured: r.IsFeatured,
			AuthorCommissionPct: r.AuthorCommissionPct, Category: r.Category, OrganizationID: r.OrganizationID,
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		}
	case gen.UpdateAffiliateProviderRow:
		return gen.AffiliateProvider{
			ID: r.ID, Name: r.Name, Slug: r.Slug,
			Description: r.Description, ProviderType: r.ProviderType, LogoMediaID: r.LogoMediaID,
			BaseUrl: r.BaseUrl, TrackingParam: r.TrackingParam, TrackingValue: r.TrackingValue,
			CommissionRate: r.CommissionRate, IsActive: r.IsActive, IsFeatured: r.IsFeatured,
			AuthorCommissionPct: r.AuthorCommissionPct, Category: r.Category, OrganizationID: r.OrganizationID,
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		}
	default:
		return gen.AffiliateProvider{}
	}
}

// Unused but referenced.
var _ = fmt.Sprintf
