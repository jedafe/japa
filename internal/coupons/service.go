package coupons

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/worker"
)

// Enqueuer is the interface for enqueuing background tasks.
type Enqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// Service provides coupon and affiliate provider operations.
type Service struct {
	store    *Store
	db       *db.DB
	enqueuer Enqueuer
}

func NewService(database *db.DB) *Service {
	return &Service{
		store:    newStore(gen.New(database.Reader), gen.New(database.Writer)),
		db:       database,
		enqueuer: &noopEnqueuer{},
	}
}

func NewServiceWithEnqueuer(database *db.DB, enqueuer Enqueuer) *Service {
	svc := NewService(database)
	svc.enqueuer = enqueuer
	return svc
}

type noopEnqueuer struct{}

func (n *noopEnqueuer) Enqueue(_ string, _ []byte, _ ...interface{}) error { return nil }

// ── Provider Operations ─────────────────────────────────────────────

type CreateProviderInput struct {
	Name, Slug, ProviderType string
	Description              *string
	LogoMediaID              *pgtype.UUID
	BaseURL, TrackingParam, TrackingValue *string
	CommissionRate                        *pgtype.Numeric
	IsActive, IsFeatured                  bool
}

func (s *Service) CreateProvider(ctx context.Context, tenantID pgtype.UUID, input CreateProviderInput) (gen.AffiliateProvider, error) {
	return s.store.createProvider(ctx, gen.CreateAffiliateProviderParams{
		TenantID:       tenantID,
		Name:           input.Name,
		Slug:           input.Slug,
		Description:    input.Description,
		ProviderType:   input.ProviderType,
		LogoMediaID:    toNullableUUID(input.LogoMediaID),
		BaseUrl:        input.BaseURL,
		TrackingParam:  input.TrackingParam,
		TrackingValue:  input.TrackingValue,
		CommissionRate: toNullableNumeric(input.CommissionRate),
		IsActive:       input.IsActive,
		IsFeatured:     input.IsFeatured,
	})
}

func (s *Service) GetProvider(ctx context.Context, id, tenantID pgtype.UUID) (gen.AffiliateProvider, error) {
	p, err := s.store.getProviderByID(ctx, id, tenantID)
	if err != nil {
		if isNoRows(err) {
			return gen.AffiliateProvider{}, ErrProviderNotFound
		}
		return gen.AffiliateProvider{}, fmt.Errorf("getting provider: %w", err)
	}
	return p, nil
}

func (s *Service) ListProviders(ctx context.Context, tenantID pgtype.UUID, page, pageSize int32) ([]gen.AffiliateProvider, error) {
	limit, offset := paginate(page, pageSize)
	return s.store.listProviders(ctx, tenantID, true, limit, offset)
}

type UpdateProviderInput = CreateProviderInput

func (s *Service) UpdateProvider(ctx context.Context, id, tenantID pgtype.UUID, input UpdateProviderInput) (gen.AffiliateProvider, error) {
	_, err := s.store.getProviderByID(ctx, id, tenantID)
	if err != nil {
		if isNoRows(err) {
			return gen.AffiliateProvider{}, ErrProviderNotFound
		}
		return gen.AffiliateProvider{}, fmt.Errorf("checking provider: %w", err)
	}
	return s.store.updateProvider(ctx, gen.UpdateAffiliateProviderParams{
		ID: id, TenantID: tenantID, Name: input.Name, Description: input.Description,
		ProviderType: input.ProviderType, LogoMediaID: toNullableUUID(input.LogoMediaID),
		BaseUrl: input.BaseURL, TrackingParam: input.TrackingParam, TrackingValue: input.TrackingValue,
		CommissionRate: toNullableNumeric(input.CommissionRate), IsActive: input.IsActive, IsFeatured: input.IsFeatured,
	})
}

func (s *Service) DeleteProvider(ctx context.Context, id, tenantID pgtype.UUID) error {
	_, err := s.store.getProviderByID(ctx, id, tenantID)
	if err != nil {
		if isNoRows(err) {
			return ErrProviderNotFound
		}
		return fmt.Errorf("checking provider: %w", err)
	}
	count, err := s.store.countCouponsByProvider(ctx, id)
	if err != nil {
		return fmt.Errorf("counting provider coupons: %w", err)
	}
	if count > 0 {
		return ErrProviderHasCoupons
	}
	return s.store.deleteProvider(ctx, id, tenantID)
}

// ── Coupon Operations ───────────────────────────────────────────────

type CreateCouponInput struct {
	ProviderID    *pgtype.UUID        `json:"provider_id,omitempty"`
	CouponType    string              `json:"coupon_type,omitempty"`
	Title         string              `json:"title"`
	Code          string              `json:"code"`
	Status        string              `json:"status,omitempty"`
	Description   *string             `json:"description,omitempty"`
	DiscountType  *string             `json:"discount_type,omitempty"`
	DiscountValue *pgtype.Numeric     `json:"discount_value,omitempty"`
	Currency      *string             `json:"currency,omitempty"`
	TermsURL      *string             `json:"terms_url,omitempty"`
	ReferralURL   *string             `json:"referral_url,omitempty"`
	IsVerified    bool                `json:"is_verified,omitempty"`
	ValidFrom     *pgtype.Timestamptz `json:"valid_from,omitempty"`
	ExpiresAt     *pgtype.Timestamptz `json:"valid_until,omitempty"`
}

func (s *Service) CreateCoupon(ctx context.Context, tenantID, userID pgtype.UUID, input CreateCouponInput) (gen.Coupon, error) {
	return s.store.createCoupon(ctx, gen.CreateCouponParams{
		TenantID: tenantID, ProviderID: toNullableUUID(input.ProviderID), CouponType: input.CouponType,
		Title: input.Title, Description: input.Description, Code: input.Code,
		DiscountType: input.DiscountType, DiscountValue: toNullableNumeric(input.DiscountValue),
		Currency: input.Currency, TermsUrl: input.TermsURL, ReferralUrl: input.ReferralURL,
		IsVerified: input.IsVerified, Status: input.Status,
		ValidFrom: toTimestamptz(input.ValidFrom), ExpiresAt: toTimestamptz(input.ExpiresAt), CreatedBy: userID,
	})
}

func (s *Service) GetCoupon(ctx context.Context, id, tenantID pgtype.UUID) (gen.GetCouponByIDRow, error) {
	c, err := s.store.getCouponByID(ctx, id, tenantID)
	if err != nil {
		if isNoRows(err) {
			return gen.GetCouponByIDRow{}, ErrCouponNotFound
		}
		return gen.GetCouponByIDRow{}, fmt.Errorf("getting coupon: %w", err)
	}
	return c, nil
}

func (s *Service) ListCoupons(ctx context.Context, tenantID pgtype.UUID, status, couponType, search *string, page, pageSize int32) ([]gen.ListCouponsByTenantRow, error) {
	limit, offset := paginate(page, pageSize)
	return s.store.listCoupons(ctx, tenantID, status, couponType, search, pgtype.UUID{}, limit, offset)
}

func (s *Service) ListActiveCoupons(ctx context.Context, tenantID pgtype.UUID, providerID *pgtype.UUID, couponType, search *string, page, pageSize int32) ([]gen.ListActiveCouponsByTenantRow, error) {
	limit, offset := paginate(page, pageSize)
	pid := pgtype.UUID{}
	if providerID != nil {
		pid = *providerID
	}
	return s.store.listActiveCoupons(ctx, tenantID, pid, couponType, search, limit, offset)
}

type UpdateCouponInput = CreateCouponInput

func (s *Service) UpdateCoupon(ctx context.Context, id, tenantID pgtype.UUID, input UpdateCouponInput) (gen.Coupon, error) {
	_, err := s.store.getCouponByID(ctx, id, tenantID)
	if err != nil {
		if isNoRows(err) {
			return gen.Coupon{}, ErrCouponNotFound
		}
		return gen.Coupon{}, fmt.Errorf("checking coupon: %w", err)
	}
	return s.store.updateCoupon(ctx, gen.UpdateCouponParams{
		ID: id, TenantID: tenantID, Title: input.Title, Description: input.Description, Code: input.Code,
		DiscountType: input.DiscountType, DiscountValue: toNullableNumeric(input.DiscountValue),
		Currency: input.Currency, TermsUrl: input.TermsURL, ReferralUrl: input.ReferralURL,
		IsVerified: input.IsVerified, Status: input.Status,
		ValidFrom: toTimestamptz(input.ValidFrom), ExpiresAt: toTimestamptz(input.ExpiresAt),
	})
}

func (s *Service) DeleteCoupon(ctx context.Context, id, tenantID pgtype.UUID) error {
	_, err := s.store.getCouponByID(ctx, id, tenantID)
	if err != nil {
		if isNoRows(err) {
			return ErrCouponNotFound
		}
		return fmt.Errorf("checking coupon: %w", err)
	}
	return s.store.deleteCoupon(ctx, id, tenantID)
}

// ── Coupon Events ───────────────────────────────────────────────────

func (s *Service) RecordCouponEvent(ctx context.Context, couponID pgtype.UUID, eventType string, userID, contentID *pgtype.UUID) error {
	var uid, cid pgtype.UUID
	if userID != nil {
		uid = *userID
	}
	if contentID != nil {
		cid = *contentID
	}
	_, err := s.store.createCouponEvent(ctx, gen.CreateCouponEventParams{
		CouponID: couponID, EventType: eventType, UserID: uid, ContentID: cid,
	})
	if err != nil {
		return fmt.Errorf("recording coupon event: %w", err)
	}
	payload := fmt.Sprintf(`{"coupon_id":"%s"}`, couponID.String())
	if err := s.enqueuer.Enqueue(worker.TypeCouponTrackEvents, []byte(payload)); err != nil {
		slog.Warn("failed to enqueue coupon:track_events", "error", err)
	}
	return nil
}

// ── Render Helpers ──────────────────────────────────────────────────

type RenderedCoupon struct {
	ID             string  `json:"id"`
	Title          string  `json:"title"`
	Code           string  `json:"code"`
	DiscountType   *string `json:"discount_type,omitempty"`
	DiscountValue  *string `json:"discount_value,omitempty"`
	ProviderName   *string `json:"provider_name,omitempty"`
	ProviderSlug   *string `json:"provider_slug,omitempty"`
	ProviderLogoID *string `json:"provider_logo_id,omitempty"`
	ExpiresAt      *string `json:"expires_at,omitempty"`
	IsExpired      bool    `json:"is_expired"`
	ReferralURL    *string `json:"referral_url,omitempty"`
	Status         string  `json:"status"`
}

func (s *Service) ResolveCouponForRender(ctx context.Context, id pgtype.UUID) (*RenderedCoupon, error) {
	r, err := s.store.getCouponForRender(ctx, id)
	if err != nil {
		if isNoRows(err) {
			return nil, ErrCouponNotFound
		}
		return nil, fmt.Errorf("resolving coupon: %w", err)
	}
	isExpired := r.Status != "active"
	if r.ExpiresAt.Valid && r.ExpiresAt.Time.Before(time.Now()) {
		isExpired = true
	}
	return &RenderedCoupon{
		ID: r.ID.String(), Title: r.Title, Code: r.Code,
		DiscountType: r.DiscountType, DiscountValue: ptrNumeric(r.DiscountValue),
		ProviderName: r.ProviderName, ProviderSlug: r.ProviderSlug,
		ProviderLogoID: ptrUUID(r.ProviderLogoMediaID), ExpiresAt: ptrTimestamptz(r.ExpiresAt),
		IsExpired: isExpired, ReferralURL: r.ReferralUrl, Status: r.Status,
	}, nil
}

type RenderedAffiliateLink struct {
	BaseURL       string `json:"base_url"`
	TrackingParam string `json:"tracking_param"`
	TrackingValue string `json:"tracking_value"`
	ResolvedURL   string `json:"resolved_url"`
}

func (s *Service) ResolveAffiliateLinkForRender(ctx context.Context, providerID pgtype.UUID, originalURL string) (*RenderedAffiliateLink, error) {
	r, err := s.store.getAffiliateProviderForRender(ctx, providerID)
	if err != nil {
		if isNoRows(err) {
			return nil, ErrProviderNotFound
		}
		return nil, fmt.Errorf("resolving affiliate link: %w", err)
	}
	resolvedURL := originalURL
	if r.TrackingParam != nil && r.TrackingValue != nil && *r.TrackingParam != "" && *r.TrackingValue != "" {
		separator := "?"
		for i := len(originalURL) - 1; i >= 0; i-- {
			if originalURL[i] == '?' {
				separator = "&"
				break
			}
		}
		resolvedURL = originalURL + separator + *r.TrackingParam + "=" + *r.TrackingValue
	}
	return &RenderedAffiliateLink{
		BaseURL:       derefStr(r.BaseUrl),
		TrackingParam: derefStr(r.TrackingParam),
		TrackingValue: derefStr(r.TrackingValue),
		ResolvedURL:   resolvedURL,
	}, nil
}

// ── Helpers ─────────────────────────────────────────────────────────

func paginate(page, pageSize int32) (limit, offset int32) {
	if pageSize <= 0 || pageSize > MaxPageSize {
		pageSize = DefaultPageSize
	}
	if page <= 0 {
		page = 1
	}
	return pageSize, (page - 1) * pageSize
}

func toNullableUUID(u *pgtype.UUID) pgtype.UUID {
	if u == nil {
		return pgtype.UUID{}
	}
	return *u
}

func toNullableNumeric(n *pgtype.Numeric) pgtype.Numeric {
	if n == nil {
		return pgtype.Numeric{}
	}
	return *n
}

func toTimestamptz(t *pgtype.Timestamptz) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return *t
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func ptrNumeric(n pgtype.Numeric) *string {
	if !n.Valid {
		return nil
	}
	s := fmt.Sprintf("%v", n)
	return &s
}

func ptrUUID(u pgtype.UUID) *string {
	if !u.Valid {
		return nil
	}
	s := u.String()
	return &s
}

func ptrTimestamptz(t pgtype.Timestamptz) *string {
	if !t.Valid {
		return nil
	}
	s := t.Time.Format(time.RFC3339)
	return &s
}

var _ = errors.New
