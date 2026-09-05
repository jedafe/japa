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

func (s *Store) getProviderByID(ctx context.Context, id, tenantID pgtype.UUID) (gen.AffiliateProvider, error) {
	row, err := s.read.GetAffiliateProviderByID(ctx, gen.GetAffiliateProviderByIDParams{ID: id, TenantID: tenantID})
	if err != nil {
		return gen.AffiliateProvider{}, err
	}

	return rowToProvider(row), nil
}

func (s *Store) listProviders(ctx context.Context, tenantID pgtype.UUID, isActive bool, limit, offset int32) ([]gen.AffiliateProvider, error) {
	rows, err := s.read.ListAffiliateProvidersByTenant(ctx, gen.ListAffiliateProvidersByTenantParams{
		TenantID:   tenantID,
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

func (s *Store) deleteProvider(ctx context.Context, id, tenantID pgtype.UUID) error {
	return s.write.DeleteAffiliateProvider(ctx, gen.DeleteAffiliateProviderParams{ID: id, TenantID: tenantID})
}

func (s *Store) countCouponsByProvider(ctx context.Context, providerID pgtype.UUID) (int64, error) {
	return s.read.CountCouponsByProvider(ctx, providerID)
}

// ── Coupons ─────────────────────────────────────────────────────────

func (s *Store) createCoupon(ctx context.Context, c gen.CreateCouponParams) (gen.Coupon, error) {
	return s.write.CreateCoupon(ctx, c)
}

func (s *Store) getCouponByID(ctx context.Context, id, tenantID pgtype.UUID) (gen.GetCouponByIDRow, error) {
	return s.read.GetCouponByID(ctx, gen.GetCouponByIDParams{ID: id, TenantID: tenantID})
}

func (s *Store) getCouponByCode(ctx context.Context, code string, tenantID pgtype.UUID) (gen.GetCouponByCodeRow, error) {
	return s.read.GetCouponByCode(ctx, gen.GetCouponByCodeParams{Code: code, TenantID: tenantID})
}

func (s *Store) listCoupons(ctx context.Context, tenantID pgtype.UUID, status, couponType, search *string, providerID pgtype.UUID, limit, offset int32) ([]gen.ListCouponsByTenantRow, error) {
	return s.read.ListCouponsByTenant(ctx, gen.ListCouponsByTenantParams{
		TenantID:   tenantID,
		Status:     status,
		CouponType: couponType,
		ProviderID: providerID,
		Search:     search,
		PageLimit:  limit,
		PageOffset: offset,
	})
}

func (s *Store) listActiveCoupons(ctx context.Context, tenantID, providerID pgtype.UUID, couponType, search *string, limit, offset int32) ([]gen.ListActiveCouponsByTenantRow, error) {
	return s.read.ListActiveCouponsByTenant(ctx, gen.ListActiveCouponsByTenantParams{
		TenantID:   tenantID,
		ProviderID: providerID,
		CouponType: couponType,
		Search:     search,
		PageLimit:  limit,
		PageOffset: offset,
	})
}

func (s *Store) updateCoupon(ctx context.Context, c gen.UpdateCouponParams) (gen.Coupon, error) {
	return s.write.UpdateCoupon(ctx, c)
}

func (s *Store) deleteCoupon(ctx context.Context, id, tenantID pgtype.UUID) error {
	return s.write.DeleteCoupon(ctx, gen.DeleteCouponParams{ID: id, TenantID: tenantID})
}

// ── Coupon Events ───────────────────────────────────────────────────

func (s *Store) createCouponEvent(ctx context.Context, e gen.CreateCouponEventParams) (gen.CouponEvent, error) {
	return s.write.CreateCouponEvent(ctx, e)
}

// ── Render Helpers ──────────────────────────────────────────────────

func (s *Store) getCouponForRender(ctx context.Context, id, tenantID pgtype.UUID) (gen.GetCouponForRenderRow, error) {
	return s.read.GetCouponForRender(ctx, gen.GetCouponForRenderParams{ID: id, TenantID: tenantID})
}

func (s *Store) getAffiliateProviderForRender(ctx context.Context, id, tenantID pgtype.UUID) (gen.GetAffiliateProviderForRenderRow, error) {
	return s.read.GetAffiliateProviderForRender(ctx, gen.GetAffiliateProviderForRenderParams{ID: id, TenantID: tenantID})
}

// ── Count ───────────────────────────────────────────────────────────

func (s *Store) countCouponsByTenant(ctx context.Context, tenantID pgtype.UUID) (int64, error) {
	return s.read.CountCouponsByTenant(ctx, tenantID)
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

// ── Error helpers ───────────────────────────────────────────────────

func isNoRows(err error) bool {
	return err != nil && errors.Is(err, pgx.ErrNoRows)
}

// rowToProvider converts any affiliate provider row type to the canonical model.
func rowToProvider[T any](row T) gen.AffiliateProvider {
	switch r := any(row).(type) {
	case gen.CreateAffiliateProviderRow:
		return gen.AffiliateProvider{
			ID: r.ID, TenantID: r.TenantID, Name: r.Name, Slug: r.Slug,
			Description: r.Description, ProviderType: r.ProviderType, LogoMediaID: r.LogoMediaID,
			BaseUrl: r.BaseUrl, TrackingParam: r.TrackingParam, TrackingValue: r.TrackingValue,
			CommissionRate: r.CommissionRate, IsActive: r.IsActive, IsFeatured: r.IsFeatured,
			AuthorCommissionPct: r.AuthorCommissionPct, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		}
	case gen.GetAffiliateProviderByIDRow:
		return gen.AffiliateProvider{
			ID: r.ID, TenantID: r.TenantID, Name: r.Name, Slug: r.Slug,
			Description: r.Description, ProviderType: r.ProviderType, LogoMediaID: r.LogoMediaID,
			BaseUrl: r.BaseUrl, TrackingParam: r.TrackingParam, TrackingValue: r.TrackingValue,
			CommissionRate: r.CommissionRate, IsActive: r.IsActive, IsFeatured: r.IsFeatured,
			AuthorCommissionPct: r.AuthorCommissionPct, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		}
	case gen.ListAffiliateProvidersByTenantRow:
		return gen.AffiliateProvider{
			ID: r.ID, TenantID: r.TenantID, Name: r.Name, Slug: r.Slug,
			Description: r.Description, ProviderType: r.ProviderType, LogoMediaID: r.LogoMediaID,
			BaseUrl: r.BaseUrl, TrackingParam: r.TrackingParam, TrackingValue: r.TrackingValue,
			CommissionRate: r.CommissionRate, IsActive: r.IsActive, IsFeatured: r.IsFeatured,
			AuthorCommissionPct: r.AuthorCommissionPct, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		}
	case gen.UpdateAffiliateProviderRow:
		return gen.AffiliateProvider{
			ID: r.ID, TenantID: r.TenantID, Name: r.Name, Slug: r.Slug,
			Description: r.Description, ProviderType: r.ProviderType, LogoMediaID: r.LogoMediaID,
			BaseUrl: r.BaseUrl, TrackingParam: r.TrackingParam, TrackingValue: r.TrackingValue,
			CommissionRate: r.CommissionRate, IsActive: r.IsActive, IsFeatured: r.IsFeatured,
			AuthorCommissionPct: r.AuthorCommissionPct, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		}
	default:
		return gen.AffiliateProvider{}
	}
}

// Unused but referenced.
var _ = fmt.Sprintf
