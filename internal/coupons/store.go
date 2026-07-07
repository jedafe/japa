package coupons

import (
	"context"
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

func (s *Store) createProvider(ctx context.Context, p gen.CreateAffiliateProviderParams) (gen.AffiliateProvider, error) {
	return s.write.CreateAffiliateProvider(ctx, p)
}

func (s *Store) getProviderByID(ctx context.Context, id pgtype.UUID, tenantID pgtype.UUID) (gen.AffiliateProvider, error) {
	return s.read.GetAffiliateProviderByID(ctx, gen.GetAffiliateProviderByIDParams{ID: id, TenantID: tenantID})
}

func (s *Store) listProviders(ctx context.Context, tenantID pgtype.UUID, isActive bool, limit, offset int32) ([]gen.AffiliateProvider, error) {
	return s.read.ListAffiliateProvidersByTenant(ctx, gen.ListAffiliateProvidersByTenantParams{
		TenantID:   tenantID,
		IsActive:   isActive,
		PageLimit:  limit,
		PageOffset: offset,
	})
}

func (s *Store) updateProvider(ctx context.Context, p gen.UpdateAffiliateProviderParams) (gen.AffiliateProvider, error) {
	return s.write.UpdateAffiliateProvider(ctx, p)
}

func (s *Store) deleteProvider(ctx context.Context, id pgtype.UUID, tenantID pgtype.UUID) error {
	return s.write.DeleteAffiliateProvider(ctx, gen.DeleteAffiliateProviderParams{ID: id, TenantID: tenantID})
}

func (s *Store) countCouponsByProvider(ctx context.Context, providerID pgtype.UUID) (int64, error) {
	return s.read.CountCouponsByProvider(ctx, providerID)
}

// ── Coupons ─────────────────────────────────────────────────────────

func (s *Store) createCoupon(ctx context.Context, c gen.CreateCouponParams) (gen.Coupon, error) {
	return s.write.CreateCoupon(ctx, c)
}

func (s *Store) getCouponByID(ctx context.Context, id pgtype.UUID, tenantID pgtype.UUID) (gen.GetCouponByIDRow, error) {
	return s.read.GetCouponByID(ctx, gen.GetCouponByIDParams{ID: id, TenantID: tenantID})
}

func (s *Store) getCouponByCode(ctx context.Context, code string, tenantID pgtype.UUID) (gen.GetCouponByCodeRow, error) {
	return s.read.GetCouponByCode(ctx, gen.GetCouponByCodeParams{Code: code, TenantID: tenantID})
}

func (s *Store) listCoupons(ctx context.Context, tenantID pgtype.UUID, status, couponType string, providerID pgtype.UUID, search string, limit, offset int32) ([]gen.ListCouponsByTenantRow, error) {
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

func (s *Store) listActiveCoupons(ctx context.Context, tenantID pgtype.UUID, providerID pgtype.UUID, couponType string, search string, limit, offset int32) ([]gen.ListActiveCouponsByTenantRow, error) {
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

func (s *Store) deleteCoupon(ctx context.Context, id pgtype.UUID, tenantID pgtype.UUID) error {
	return s.write.DeleteCoupon(ctx, gen.DeleteCouponParams{ID: id, TenantID: tenantID})
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

func (s *Store) countCouponsByTenant(ctx context.Context, tenantID pgtype.UUID) (int64, error) {
	return s.read.CountCouponsByTenant(ctx, tenantID)
}

// ── Error helpers ───────────────────────────────────────────────────

func isNoRows(err error) bool {
	return err != nil && err == pgx.ErrNoRows
}

// Unused but referenced.
var _ = fmt.Sprintf
