package coupons

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/valkey"
	"github.com/jedafe/japa/internal/worker"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// Cache TTLs (Section 55 Valkey Caching / catalog: jc:coupon:{id}, jc:coupon_list:{page_size}).
const (
	couponCacheTTL     = 15 * time.Minute
	couponListCacheTTL = 5 * time.Minute
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
	vk       valkeycompat.Cmdable
}

// NewService is an exported function.
func NewService(database *db.DB) *Service {
	return &Service{
		store:    newStore(gen.New(database.Reader), gen.New(database.Writer)),
		db:       database,
		enqueuer: &noopEnqueuer{},
	}
}

// NewServiceWithEnqueuer is an exported function.
func NewServiceWithEnqueuer(database *db.DB, enqueuer Enqueuer) *Service {
	svc := NewService(database)
	svc.enqueuer = enqueuer

	return svc
}

// SetCache injects a Valkey cache client for coupon detail/list caching
// (Section 55). When nil, the service falls back to direct DB reads.
func (s *Service) SetCache(vk valkeycompat.Cmdable) { s.vk = vk }

type noopEnqueuer struct{}

func (n *noopEnqueuer) Enqueue(_ string, _ []byte, _ ...interface{}) error { return nil }

// ── Provider Operations ─────────────────────────────────────────────

// CreateProviderInput is an exported type.
type CreateProviderInput struct {
	Name, Slug, ProviderType              string
	Description                           *string
	LogoMediaID                           *pgtype.UUID
	BaseURL, TrackingParam, TrackingValue *string
	CommissionRate                        *pgtype.Numeric
	IsActive, IsFeatured                  bool
	Category                              *string
}

// CreateProvider is an exported method.
func (s *Service) CreateProvider(ctx context.Context, input CreateProviderInput) (gen.AffiliateProvider, error) {
	p, err := s.store.createProvider(ctx, gen.CreateAffiliateProviderParams{

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
		Category:       input.Category,
	})
	if err != nil {
		return gen.AffiliateProvider{}, err
	}

	// Provider fields render inside coupon payloads — flush list/slug caches.
	s.deleteKeysByPattern(ctx, valkey.MustKey("coupon_list:*"))
	s.deleteKeysByPattern(ctx, valkey.MustKey("coupon_slug:*"))

	return p, nil
}

// GetProvider is an exported method.
func (s *Service) GetProvider(ctx context.Context, id pgtype.UUID) (gen.AffiliateProvider, error) {
	p, err := s.store.getProviderByID(ctx, id)
	if err != nil {
		if isNoRows(err) {
			return gen.AffiliateProvider{}, ErrProviderNotFound
		}

		return gen.AffiliateProvider{}, fmt.Errorf("getting provider: %w", err)
	}

	return p, nil
}

// ListProviders is an exported method.
func (s *Service) ListProviders(ctx context.Context, page, pageSize int32) ([]gen.AffiliateProvider, error) {
	limit, offset := paginate(page, pageSize)
	return s.store.listProviders(ctx, true, limit, offset)
}

// UpdateProviderInput is an exported type.
type UpdateProviderInput = CreateProviderInput

// UpdateProvider is an exported method.
func (s *Service) UpdateProvider(ctx context.Context, id pgtype.UUID, input UpdateProviderInput) (gen.AffiliateProvider, error) {
	_, err := s.store.getProviderByID(ctx, id)
	if err != nil {
		if isNoRows(err) {
			return gen.AffiliateProvider{}, ErrProviderNotFound
		}

		return gen.AffiliateProvider{}, fmt.Errorf("checking provider: %w", err)
	}

	p, err := s.store.updateProvider(ctx, gen.UpdateAffiliateProviderParams{
		ID: id, Name: input.Name, Description: input.Description,
		ProviderType: input.ProviderType, LogoMediaID: toNullableUUID(input.LogoMediaID),
		BaseUrl: input.BaseURL, TrackingParam: input.TrackingParam, TrackingValue: input.TrackingValue,
		CommissionRate: toNullableNumeric(input.CommissionRate), IsActive: input.IsActive, IsFeatured: input.IsFeatured,
		Category: input.Category,
	})
	if err != nil {
		return gen.AffiliateProvider{}, err
	}

	// Provider fields render inside coupon payloads — flush list/slug caches.
	s.deleteKeysByPattern(ctx, valkey.MustKey("coupon_list:*"))
	s.deleteKeysByPattern(ctx, valkey.MustKey("coupon_slug:*"))

	return p, nil
}

// DeleteProvider is an exported method.
func (s *Service) DeleteProvider(ctx context.Context, id pgtype.UUID) error {
	_, err := s.store.getProviderByID(ctx, id)
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

	if err := s.store.deleteProvider(ctx, id); err != nil {
		return err
	}

	// Provider fields render inside coupon payloads — flush list/slug caches.
	s.deleteKeysByPattern(ctx, valkey.MustKey("coupon_list:*"))
	s.deleteKeysByPattern(ctx, valkey.MustKey("coupon_slug:*"))

	return nil
}

// ── Coupon Operations ───────────────────────────────────────────────

// CreateCouponInput is an exported type.
type CreateCouponInput struct {
	ProviderID    *pgtype.UUID        `json:"provider_id,omitempty"`
	CouponType    string              `json:"coupon_type,omitempty"`
	Title         string              `json:"title"`
	Slug          *string             `json:"slug,omitempty"`
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
	Details       json.RawMessage     `json:"details,omitempty"`
}

// maxCouponDetailsBytes caps the coupons.details JSONB payload (§55).
const maxCouponDetailsBytes = 32 * 1024

// validateCouponDetails accepts nil/empty and JSON objects up to
// maxCouponDetailsBytes; anything else is ErrInvalidDetails.
func validateCouponDetails(raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	if len(raw) > maxCouponDetailsBytes {
		return ErrInvalidDetails
	}
	var v map[string]any
	if err := json.Unmarshal(raw, &v); err != nil || v == nil {
		return ErrInvalidDetails
	}
	return nil
}

// CreateCoupon is an exported method.
func (s *Service) CreateCoupon(ctx context.Context, userID pgtype.UUID, input CreateCouponInput) (gen.CreateCouponRow, error) {
	if err := validateCouponDetails(input.Details); err != nil {
		return gen.CreateCouponRow{}, err
	}

	slug, err := s.resolveCouponSlug(ctx, input.Slug, input.Title, pgtype.UUID{})
	if err != nil {
		return gen.CreateCouponRow{}, err
	}

	c, err := s.store.createCoupon(ctx, gen.CreateCouponParams{
		ProviderID: toNullableUUID(input.ProviderID), CouponType: input.CouponType,
		Title: input.Title, Slug: slug, Description: input.Description, Code: strPtrOrNil(input.Code),
		DiscountType: input.DiscountType, DiscountValue: toNullableNumeric(input.DiscountValue),
		Currency: input.Currency, TermsUrl: input.TermsURL, ReferralUrl: input.ReferralURL,
		IsVerified: input.IsVerified, Status: input.Status,
		ValidFrom: toTimestamptz(input.ValidFrom), ExpiresAt: toTimestamptz(input.ExpiresAt),
		Details: input.Details, CreatedBy: userID,
	})
	if err != nil {
		return gen.CreateCouponRow{}, err
	}

	s.invalidateCouponCaches(ctx, c.ID, c.Slug)

	return c, nil
}

// maxCouponSlugAttempts bounds the suffix loop in resolveCouponSlug so a
// pathological run of collisions cannot loop forever (mirrors
// content.MaxSlugAttempts).
const maxCouponSlugAttempts = 100

// resolveCouponSlug normalizes an optional admin-supplied slug (or derives
// one from the title via content.Slugify) and returns a unique slug for the
// coupons table. excludeID skips the coupon being updated so an unchanged
// slug still resolves. Reserved slugs that collide with /api/v1/deals
// static segments are suffixed like any other collision.
func (s *Service) resolveCouponSlug(ctx context.Context, input *string, title string, excludeID pgtype.UUID) (string, error) {
	base := content.Slugify(title)
	if input != nil && *input != "" {
		base = content.Slugify(*input)
	}

	for i := 0; i < maxCouponSlugAttempts; i++ {
		candidate := base
		if i > 0 {
			candidate = fmt.Sprintf("%s-%d", base, i+1)
		}
		if reservedCouponSlugs[candidate] {
			continue
		}

		existing, err := s.store.getCouponBySlug(ctx, candidate)
		if err != nil {
			if isNoRows(err) {
				return candidate, nil
			}

			return "", fmt.Errorf("checking coupon slug: %w", err)
		}
		if excludeID.Valid && existing.ID == excludeID {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("could not find unique coupon slug after %d attempts", maxCouponSlugAttempts)
}

// GetCoupon is an exported method.
func (s *Service) GetCoupon(ctx context.Context, id pgtype.UUID) (gen.GetCouponByIDRow, error) {
	// Try cache first (jc:coupon:{id}, TTL 15min — Section 55).
	cacheKey := valkey.MustKey("coupon:%s", id.String())
	if s.vk != nil {
		if cached, err := s.vk.Get(ctx, cacheKey).Result(); err == nil && cached != "" {
			var c gen.GetCouponByIDRow
			if jsonErr := json.Unmarshal([]byte(cached), &c); jsonErr == nil {
				return c, nil
			}
		}
	}

	c, err := s.store.getCouponByID(ctx, id)
	if err != nil {
		if isNoRows(err) {
			return gen.GetCouponByIDRow{}, ErrCouponNotFound
		}

		return gen.GetCouponByIDRow{}, fmt.Errorf("getting coupon: %w", err)
	}

	// Best-effort cache write.
	if s.vk != nil {
		if data, jsonErr := json.Marshal(c); jsonErr == nil {
			if err := s.vk.Set(ctx, cacheKey, string(data), couponCacheTTL).Err(); err != nil {
				slog.Warn("coupons: failed to cache coupon", "id", id.String(), "error", err)
			}
		}
	}

	return c, nil
}

// ListCoupons is an exported method.
func (s *Service) ListCoupons(ctx context.Context, status, couponType, search *string, page, pageSize int32) ([]gen.ListCouponsRow, error) {
	limit, offset := paginate(page, pageSize)
	return s.store.listCoupons(ctx, status, couponType, search, pgtype.UUID{}, limit, offset)
}

func (s *Service) ListActiveCoupons(ctx context.Context, providerID *pgtype.UUID, couponType, search *string, page, pageSize int32) ([]gen.ListActiveCouponsRow, error) {
	// Only cache unfiltered first-page lists (jc:coupon_list:{page_size}, TTL 5min — Section 55).
	canCache := providerID == nil && couponType == nil && search == nil && page == 1
	cacheKey := valkey.MustKey("coupon_list:%d", pageSize)

	if canCache && s.vk != nil {
		if cached, err := s.vk.Get(ctx, cacheKey).Result(); err == nil && cached != "" {
			var coupons []gen.ListActiveCouponsRow
			if jsonErr := json.Unmarshal([]byte(cached), &coupons); jsonErr == nil {
				return coupons, nil
			}
		}
	}

	limit, offset := paginate(page, pageSize)

	pid := pgtype.UUID{}
	if providerID != nil {
		pid = *providerID
	}

	coupons, err := s.store.listActiveCoupons(ctx, pid, couponType, search, limit, offset)
	if err != nil {
		return nil, err
	}

	// Best-effort cache write (only for unfiltered first page).
	if canCache && s.vk != nil {
		if data, jsonErr := json.Marshal(coupons); jsonErr == nil {
			if err := s.vk.Set(ctx, cacheKey, string(data), couponListCacheTTL).Err(); err != nil {
				slog.Warn("coupons: failed to cache coupon list", "platform", "", "error", err)
			}
		}
	}

	return coupons, nil
}

// UpdateCouponInput is an exported type.
type UpdateCouponInput = CreateCouponInput

// UpdateCoupon is an exported method.
func (s *Service) UpdateCoupon(ctx context.Context, id pgtype.UUID, input UpdateCouponInput) (gen.UpdateCouponRow, error) {
	if err := validateCouponDetails(input.Details); err != nil {
		return gen.UpdateCouponRow{}, err
	}

	current, err := s.store.getCouponByID(ctx, id)
	if err != nil {
		if isNoRows(err) {
			return gen.UpdateCouponRow{}, ErrCouponNotFound
		}

		return gen.UpdateCouponRow{}, fmt.Errorf("checking coupon: %w", err)
	}

	slug := current.Slug
	if input.Slug != nil && *input.Slug != "" && content.Slugify(*input.Slug) != current.Slug {
		if slug, err = s.resolveCouponSlug(ctx, input.Slug, input.Title, id); err != nil {
			return gen.UpdateCouponRow{}, err
		}
	}

	c, err := s.store.updateCoupon(ctx, gen.UpdateCouponParams{
		ID: id, Title: input.Title, Slug: slug, Description: input.Description, Code: strPtrOrNil(input.Code),
		ProviderID:   toNullableUUID(input.ProviderID),
		DiscountType: input.DiscountType, DiscountValue: toNullableNumeric(input.DiscountValue),
		Currency: input.Currency, TermsUrl: input.TermsURL, ReferralUrl: input.ReferralURL,
		IsVerified: input.IsVerified, Status: input.Status,
		ValidFrom: toTimestamptz(input.ValidFrom), ExpiresAt: toTimestamptz(input.ExpiresAt),
		Details: input.Details,
	})
	if err != nil {
		return gen.UpdateCouponRow{}, err
	}

	// Record a slug redirect so old /deal/{provider}/{slug} URLs keep
	// resolving (§55 Deal Pages). Best-effort, mirrors content.RecordSlugChange.
	if c.Slug != current.Slug {
		if err := s.store.createSlugHistory(ctx, id, current.Slug, c.Slug); err != nil {
			slog.Warn("coupons: failed to record slug history",
				"coupon_id", id.String(), "old_slug", current.Slug, "new_slug", c.Slug, "error", err)
		}
	}

	s.invalidateCouponCaches(ctx, c.ID, current.Slug, c.Slug)

	return c, nil
}

// DeleteCoupon is an exported method.
func (s *Service) DeleteCoupon(ctx context.Context, id pgtype.UUID) error {
	current, err := s.store.getCouponByID(ctx, id)
	if err != nil {
		if isNoRows(err) {
			return ErrCouponNotFound
		}

		return fmt.Errorf("checking coupon: %w", err)
	}

	if err := s.store.deleteCoupon(ctx, id); err != nil {
		return err
	}

	s.invalidateCouponCaches(ctx, id, current.Slug)

	return nil
}

// invalidateCouponCaches clears the single-coupon cache (jc:coupon:{id}),
// the slug cache (jc:coupon_slug:{slug}) for every slug given, and every
// coupon list cache (jc:coupon_list:*) after any coupon mutation
// (Section 55). Best-effort: errors are logged.
func (s *Service) invalidateCouponCaches(ctx context.Context, couponID pgtype.UUID, slugs ...string) {
	if s.vk == nil {
		return
	}

	keys := []string{valkey.MustKey("coupon:%s", couponID.String())}
	for _, slug := range slugs {
		if slug != "" {
			keys = append(keys, valkey.MustKey("coupon_slug:%s", slug))
		}
	}
	if err := s.vk.Del(ctx, keys...).Err(); err != nil {
		slog.Warn("coupons: failed to invalidate caches", "keys", keys, "error", err)
	}

	s.deleteKeysByPattern(ctx, valkey.MustKey("coupon_list:*"))
}

// deleteKeysByPattern SCAN-deletes all Valkey keys matching a glob pattern
// (same approach as config settings invalidation — Section 55).
func (s *Service) deleteKeysByPattern(ctx context.Context, pattern string) {
	if s.vk == nil {
		return
	}

	var cursor uint64

	for {
		keys, nextCursor, err := s.vk.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			slog.Warn("coupons: failed to scan valkey for cache invalidation", "pattern", pattern, "error", err)
			return
		}

		if len(keys) > 0 {
			_ = s.vk.Del(ctx, keys...).Err()
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
}

// ── Deal Pages (Section 55) ─────────────────────────────────────────

// ResolveCouponBySlug returns the coupon row for a deal slug, cached under
// jc:coupon_slug:{slug} (TTL 15min — Section 55). When the slug is not found
// it consults slug_history (entity_type 'coupon') so renamed deals keep
// resolving; the returned row always carries the canonical current slug.
func (s *Service) ResolveCouponBySlug(ctx context.Context, slug string) (gen.GetCouponBySlugRow, error) {
	cacheKey := valkey.MustKey("coupon_slug:%s", slug)
	if s.vk != nil {
		if cached, err := s.vk.Get(ctx, cacheKey).Result(); err == nil && cached != "" {
			var c gen.GetCouponBySlugRow
			if jsonErr := json.Unmarshal([]byte(cached), &c); jsonErr == nil {
				return c, nil
			}
		}
	}

	c, err := s.store.getCouponBySlug(ctx, slug)
	if err != nil {
		if !isNoRows(err) {
			return gen.GetCouponBySlugRow{}, fmt.Errorf("getting coupon by slug: %w", err)
		}

		// Renamed-deal redirect via slug_history.
		history, histErr := s.store.getSlugRedirect(ctx, slug)
		if histErr != nil {
			return gen.GetCouponBySlugRow{}, ErrCouponNotFound
		}

		c, err = s.store.getCouponBySlug(ctx, history.NewSlug)
		if err != nil {
			if isNoRows(err) {
				return gen.GetCouponBySlugRow{}, ErrCouponNotFound
			}

			return gen.GetCouponBySlugRow{}, fmt.Errorf("getting coupon by redirected slug: %w", err)
		}
	}

	// Best-effort cache write.
	if s.vk != nil {
		if data, jsonErr := json.Marshal(c); jsonErr == nil {
			if err := s.vk.Set(ctx, cacheKey, string(data), couponCacheTTL).Err(); err != nil {
				slog.Warn("coupons: failed to cache coupon by slug", "slug", slug, "error", err)
			}
		}
	}

	return c, nil
}

// DealPartner is the public payload for GET /deals/partners/{slug} — the
// provider profile plus its active coupons.
type DealPartner struct {
	Provider gen.AffiliateProvider      `json:"provider"`
	Coupons  []gen.ListActiveCouponsRow `json:"coupons"`
}

// GetDealPartner resolves an active affiliate provider by slug with its
// active coupons for the /deal/{provider} landing page (Section 55).
func (s *Service) GetDealPartner(ctx context.Context, slug string) (*DealPartner, error) {
	p, err := s.store.getProviderBySlug(ctx, slug)
	if err != nil {
		if isNoRows(err) {
			return nil, ErrProviderNotFound
		}

		return nil, fmt.Errorf("getting provider by slug: %w", err)
	}

	if !p.IsActive {
		return nil, ErrProviderNotFound
	}

	coupons, err := s.store.listActiveCoupons(ctx, p.ID, nil, nil, MaxPageSize, 0)
	if err != nil {
		return nil, fmt.Errorf("listing provider coupons: %w", err)
	}

	if coupons == nil {
		coupons = []gen.ListActiveCouponsRow{}
	}

	return &DealPartner{Provider: p, Coupons: coupons}, nil
}

// ResolveDealClick returns the click-through target for /deals/{slug}/go:
// the coupon's referral_url while the deal is live, otherwise the provider's
// base_url as a soft landing. Returns ErrDealRedirectUnavailable when the
// deal has no target at all (Section 55 Deal Pages).
func (s *Service) ResolveDealClick(ctx context.Context, slug string) (target string, coupon gen.GetCouponBySlugRow, err error) {
	coupon, err = s.ResolveCouponBySlug(ctx, slug)
	if err != nil {
		return "", coupon, err
	}

	live := coupon.Status == string(CouponStatusActive)
	if live && coupon.ExpiresAt.Valid && coupon.ExpiresAt.Time.Before(time.Now()) {
		live = false
	}

	if live && coupon.ReferralUrl != nil && *coupon.ReferralUrl != "" {
		return *coupon.ReferralUrl, coupon, nil
	}

	if coupon.ProviderBaseUrl != nil && *coupon.ProviderBaseUrl != "" {
		return *coupon.ProviderBaseUrl, coupon, nil
	}

	return "", coupon, ErrDealRedirectUnavailable
}

// ── Coupon Events ───────────────────────────────────────────────────

// RecordCouponEvent is an exported method.
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

// RenderedCoupon is an exported type.
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

// ResolveCouponForRender is an exported method.
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
		ID: r.ID.String(), Title: r.Title, Code: derefStr(r.Code),
		DiscountType: r.DiscountType, DiscountValue: ptrNumeric(r.DiscountValue),
		ProviderName: r.ProviderName, ProviderSlug: r.ProviderSlug,
		ProviderLogoID: ptrUUID(r.ProviderLogoMediaID), ExpiresAt: ptrTimestamptz(r.ExpiresAt),
		IsExpired: isExpired, ReferralURL: r.ReferralUrl, Status: r.Status,
	}, nil
}

// RenderedAffiliateLink is an exported type.
type RenderedAffiliateLink struct {
	BaseURL       string `json:"base_url"`
	TrackingParam string `json:"tracking_param"`
	TrackingValue string `json:"tracking_value"`
	ResolvedURL   string `json:"resolved_url"`
}

// ResolveAffiliateLinkForRender is an exported method.
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

// strPtrOrNil maps an empty string to nil — coupons.code is nullable (§55
// Deal Feed Import: "deal" offers may carry no code).
func strPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}

	return &s
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

// ---------------------------------------------------------------------------
// Affiliate commission recording (Section 55 — Author Revenue Sharing)
// ---------------------------------------------------------------------------

// RecordAffiliateCommissionInput is the admin payload for recording an external
// affiliate commission earned from a content's affiliate link.
type RecordAffiliateCommissionInput struct {
	ProviderID         pgtype.UUID
	ContentID          pgtype.UUID
	ExternalCommission float64 // major currency units (e.g. NGN 1500.00)
	EventDescription   *string // optional description of the commission event
}

// RecordAffiliateCommission records an external affiliate commission for a
// content's affiliate link, computes the author's revenue share from the
// provider's author_commission_pct, inserts the commission row, and enqueues
// the affiliate:credit_author_commission task to credit the author's wallet.
//
// The flow (Section 55):
//  1. Look up the content to get the author_id.
//  2. Look up the provider to get author_commission_pct.
//  3. Compute author_share = external_commission * author_commission_pct / 100.
//  4. INSERT into affiliate_content_commissions.
//  5. Enqueue affiliate:credit_author_commission (fire-and-forget per Rule 8).
//
// AUDIT-BE-TASK-001: this is the missing INSERT site that triggers the
// affiliate:credit_author_commission task.
func (s *Service) RecordAffiliateCommission(ctx context.Context, in RecordAffiliateCommissionInput) (gen.AffiliateContentCommission, error) {
	// 1. Look up the content to get the author_id.
	content, err := s.store.read.GetContentByID(ctx,
		in.ContentID,
	)
	if err != nil {
		return gen.AffiliateContentCommission{}, fmt.Errorf("affiliate: content not found: %w", err)
	}

	if !content.AuthorID.Valid {
		return gen.AffiliateContentCommission{}, errors.New("affiliate: content has no author_id")
	}

	// 2. Look up the provider to get author_commission_pct.
	provider, err := s.store.read.GetAffiliateProviderByID(ctx,
		in.ProviderID,
	)
	if err != nil {
		return gen.AffiliateContentCommission{}, fmt.Errorf("affiliate: provider not found: %w", err)
	}

	// 3. Compute the author share.
	pctVal, err := provider.AuthorCommissionPct.Float64Value()
	if err != nil || !pctVal.Valid {
		pctVal.Float64 = 0
	}

	authorPct := pctVal.Float64
	authorShare := in.ExternalCommission * authorPct / 100.0

	// 4. Insert the commission row.
	commission, err := s.store.write.InsertAffiliateContentCommission(ctx, gen.InsertAffiliateContentCommissionParams{

		ContentID:           in.ContentID,
		ProviderID:          in.ProviderID,
		AuthorUserID:        content.AuthorID,
		ExternalCommission:  numericFromFloat(in.ExternalCommission),
		AuthorShare:         numericFromFloat(authorShare),
		AuthorCommissionPct: numericFromFloat(authorPct),
		EventDescription:    in.EventDescription,
	})
	if err != nil {
		return gen.AffiliateContentCommission{}, fmt.Errorf("affiliate: insert commission: %w", err)
	}

	// 5. Enqueue the credit task (fire-and-forget per Rule 8).
	if s.enqueuer != nil {
		payload, _ := json.Marshal(map[string]string{
			"commission_id": commission.ID.String(),
		})
		if err := s.enqueuer.Enqueue(worker.TypeAffiliateCreditAuthorCommission, payload); err != nil {
			// Non-fatal — the commission row is recorded. The admin can
			// re-trigger crediting via the pending commissions list.
			slog.Error("affiliate: failed to enqueue credit_author_commission",
				"commission_id", commission.ID.String(), "error", err)
		} else {
			slog.Info("affiliate: recorded commission + enqueued credit task",
				"commission_id", commission.ID.String(),
				"author_user_id", content.AuthorID.String(),
				"author_share", authorShare, "author_pct", authorPct)
		}
	}

	return commission, nil
}

// numericFromFloat converts a float64 to pgtype.Numeric.
func numericFromFloat(f float64) pgtype.Numeric {
	n := pgtype.Numeric{}
	_ = n.Scan(fmt.Sprintf("%.2f", f))

	return n
}

// ── Org Deal Submissions (Section 55 Deals Redesign) ────────────────

// SubmitDealInput is the payload for POST /organizations/{id}/deals —
// provider_id/status/is_verified are server-controlled for org submissions.
type SubmitDealInput = CreateCouponInput

// SubmitOrgDeal creates a pending coupon under the organization's own
// affiliate provider (auto-created from the org on first submission).
// Ref: japaconnect-engineering/sections/55-coupons-external-affiliates.md
func (s *Service) SubmitOrgDeal(ctx context.Context, org gen.Organization, userID pgtype.UUID, input SubmitDealInput) (gen.CreateCouponRow, error) {
	if err := validateCouponDetails(input.Details); err != nil {
		return gen.CreateCouponRow{}, err
	}
	provider, err := s.providerForOrg(ctx, org)
	if err != nil {
		return gen.CreateCouponRow{}, err
	}
	slug, err := s.resolveCouponSlug(ctx, input.Slug, input.Title, pgtype.UUID{})
	if err != nil {
		return gen.CreateCouponRow{}, err
	}
	c, err := s.store.createCoupon(ctx, gen.CreateCouponParams{
		ProviderID: provider.ID, CouponType: string(CouponTypeExternal),
		Title: input.Title, Slug: slug, Description: input.Description,
		Code: strPtrOrNil(input.Code), DiscountType: input.DiscountType,
		DiscountValue: toNullableNumeric(input.DiscountValue), Currency: input.Currency,
		TermsUrl: input.TermsURL, ReferralUrl: input.ReferralURL,
		IsVerified: false, Status: string(CouponStatusPending),
		ValidFrom: toTimestamptz(input.ValidFrom), ExpiresAt: toTimestamptz(input.ExpiresAt),
		Details: input.Details, CreatedBy: userID,
	})
	if err != nil {
		return gen.CreateCouponRow{}, err
	}
	s.invalidateCouponCaches(ctx, c.ID, c.Slug)
	return c, nil
}

// providerForOrg returns the org's affiliate provider, creating it from the
// org's public profile on first use. affiliate_providers.slug is unique, so
// collisions suffix -2/-3… like resolveCouponSlug.
func (s *Service) providerForOrg(ctx context.Context, org gen.Organization) (gen.AffiliateProvider, error) {
	p, err := s.store.getProviderByOrgID(ctx, org.ID)
	if err == nil {
		return p, nil
	}
	if !isNoRows(err) {
		return gen.AffiliateProvider{}, fmt.Errorf("getting org provider: %w", err)
	}
	base := content.Slugify(org.Slug)
	if base == "" {
		base = content.Slugify(org.Name)
	}
	for i := 0; i < maxCouponSlugAttempts; i++ {
		candidate := base
		if i > 0 {
			candidate = fmt.Sprintf("%s-%d", base, i+1)
		}
		if _, err := s.store.getProviderBySlug(ctx, candidate); err == nil {
			continue
		} else if !isNoRows(err) {
			return gen.AffiliateProvider{}, fmt.Errorf("checking provider slug: %w", err)
		}
		return s.store.createProvider(ctx, gen.CreateAffiliateProviderParams{
			Name: org.Name, Slug: candidate, Description: org.Description,
			ProviderType: string(ProviderTypeBoth), BaseUrl: org.WebsiteUrl,
			LogoMediaID: org.LogoMediaID, IsActive: true,
			OrganizationID: org.ID,
		})
	}
	return gen.AffiliateProvider{}, fmt.Errorf("could not find unique provider slug after %d attempts", maxCouponSlugAttempts)
}

// ListOrgDeals lists an org's coupons in all statuses (submissions view).
func (s *Service) ListOrgDeals(ctx context.Context, orgID pgtype.UUID, page, pageSize int32) ([]gen.ListCouponsByOrganizationRow, error) {
	limit, offset := paginate(page, pageSize)
	return s.store.listCouponsByOrganization(ctx, orgID, limit, offset)
}

// ListDealPartners returns active providers for GET /deals/partners.
func (s *Service) ListDealPartners(ctx context.Context) ([]DealPartnerRef, error) {
	rows, err := s.store.listActiveDealPartners(ctx)
	if err != nil {
		return nil, err
	}
	refs := make([]DealPartnerRef, 0, len(rows))
	for _, r := range rows {
		refs = append(refs, DealPartnerRef{
			ID: r.ID, Name: r.Name, Slug: r.Slug, Description: r.Description,
			Category: r.Category, LogoMediaID: r.LogoMediaID, BaseURL: r.BaseUrl,
			IsFeatured: r.IsFeatured, OrganizationID: r.OrganizationID,
		})
	}
	return refs, nil
}
