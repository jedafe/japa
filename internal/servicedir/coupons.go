// Package servicedir — coupons.go implements the Services Marketplace
// listing↔coupon attach surface: the editor attach API
// (GET/PUT /services/{id}/coupons), the public coupons payload on
// GET /services/:slug, and the store-facing mappers Task 9's redemption
// path consumes. Org coupons themselves live in internal/coupons (§55);
// servicedir owns the service_listing_coupons link + coupon_redemptions
// tables via its own sqlc queries so a booking + redemption commit in one
// WithTxResult.
//
// Ref: docs/superpowers/specs/2026-10-07-services-marketplace-upgrade-design.md
package servicedir

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
)

// Coupon error sentinels. ErrCouponsDisabled and ErrTooManyCoupons are
// raised by the attach API; the rest are produced by the Task 9
// redemption/pricing path — they are declared here so both tasks share
// one definition (mapped to 400 codes in writeServiceError).
var (
	// ErrCouponsDisabled — service_coupons.enabled flag is off.
	ErrCouponsDisabled = errors.New("service coupons are not enabled")
	// ErrInvalidCoupon — unknown code, inactive/expired coupon, or a
	// coupon payload on an inquiry-intent booking (400 invalid_coupon).
	ErrInvalidCoupon = errors.New("invalid_coupon")
	// ErrCouponNotApplicable — coupon not owned by the listing's org, or
	// its applies_to does not cover the listing kind
	// (400 coupon_not_applicable).
	ErrCouponNotApplicable = errors.New("coupon_not_applicable")
	// ErrCouponMinSpend — booking total below the coupon's min_spend
	// (400 coupon_min_spend).
	ErrCouponMinSpend = errors.New("coupon_min_spend")
	// ErrCouponCapReached — max_redemptions exhausted
	// (400 coupon_cap_reached).
	ErrCouponCapReached = errors.New("coupon_cap_reached")
	// ErrCouponUserLimit — the booker hit the coupon's per_user_limit
	// (400 coupon_user_limit).
	ErrCouponUserLimit = errors.New("coupon_user_limit")
	// ErrCouponFirstTime — first_time_only coupon on a repeat-customer
	// booking (400 coupon_first_time_only).
	ErrCouponFirstTime = errors.New("coupon_first_time_only")
	// ErrInvalidTier — unknown/inactive/foreign package tier
	// (400 invalid_tier).
	ErrInvalidTier = errors.New("invalid_tier")
	// ErrTooManyCoupons — attach exceeds
	// service_packages.coupons.max_per_listing (400 too_many_coupons).
	ErrTooManyCoupons = errors.New("too_many_coupons")
)

// ListingCoupon is the DTO for a coupon attached to a listing. The JSON
// shape is the public contract on GET /services/:slug (coupons[]); the
// redemption-rule fields are internal-only (Task 9 consumes them via
// getListingCouponByCode) and never serialize.
type ListingCoupon struct {
	ID            string  `json:"id"`
	Title         string  `json:"title"`
	Code          string  `json:"code"`
	DiscountType  string  `json:"discount_type"`
	DiscountValue float64 `json:"discount_value"` // percent 0-100 or minor units per DiscountType
	Currency      string  `json:"currency"`
	ExpiresAt     string  `json:"expires_at"` // RFC3339, "" when NULL
	TermsURL      string  `json:"terms_url"`
	ReferralURL   string  `json:"referral_url"`
	AppliesTo     string  `json:"applies_to"` // service|package|both

	// Internal fields — redemption enforcement + attach ordering. Never
	// serialized: the public payload is the field set above only.
	Status         string `json:"-"`
	MaxRedemptions *int64 `json:"-"`
	PerUserLimit   *int64 `json:"-"`
	MinSpend       *int64 `json:"-"`
	FirstTimeOnly  bool   `json:"-"`
	OrgID          string `json:"-"` // coupons.organization_id — must equal listing's org
	Position       int32  `json:"-"`
}

// couponLinkRow is the common column set shared by the three coupon-join
// gen rows (service_listing_coupons lc JOIN coupons c) — the store layer
// projects each gen type onto this shape so one mapper serves all three.
type couponLinkRow struct {
	ID             pgtype.UUID
	Title          string
	Code           *string
	DiscountType   *string
	DiscountValue  pgtype.Numeric
	Currency       *string
	TermsURL       *string
	ReferralURL    *string
	Status         string
	ValidFrom      pgtype.Timestamptz
	ExpiresAt      pgtype.Timestamptz
	AppliesTo      string
	MaxRedemptions *int32
	PerUserLimit   *int32
	MinSpend       *int64
	FirstTimeOnly  bool
	OrganizationID pgtype.UUID
	Position       int32
	Overrides      []byte
}

func linkRowFromAll(r gen.ListServiceListingCouponsRow) couponLinkRow {
	return couponLinkRow{
		ID: r.ID, Title: r.Title, Code: r.Code, DiscountType: r.DiscountType,
		DiscountValue: r.DiscountValue, Currency: r.Currency,
		TermsURL: r.TermsUrl, ReferralURL: r.ReferralUrl, Status: r.Status,
		ValidFrom: r.ValidFrom, ExpiresAt: r.ExpiresAt, AppliesTo: r.AppliesTo,
		MaxRedemptions: r.MaxRedemptions, PerUserLimit: r.PerUserLimit,
		MinSpend: r.MinSpend, FirstTimeOnly: r.FirstTimeOnly,
		OrganizationID: r.OrganizationID, Position: r.Position,
		Overrides: r.Overrides,
	}
}

func linkRowFromActive(r gen.ListActiveServiceListingCouponsRow) couponLinkRow {
	return couponLinkRow{
		ID: r.ID, Title: r.Title, Code: r.Code, DiscountType: r.DiscountType,
		DiscountValue: r.DiscountValue, Currency: r.Currency,
		TermsURL: r.TermsUrl, ReferralURL: r.ReferralUrl, Status: r.Status,
		ValidFrom: r.ValidFrom, ExpiresAt: r.ExpiresAt, AppliesTo: r.AppliesTo,
		MaxRedemptions: r.MaxRedemptions, PerUserLimit: r.PerUserLimit,
		MinSpend: r.MinSpend, FirstTimeOnly: r.FirstTimeOnly,
		OrganizationID: r.OrganizationID, Position: r.Position,
		Overrides: r.Overrides,
	}
}

func linkRowFromCode(r gen.GetListingCouponByCodeRow) couponLinkRow {
	return couponLinkRow{
		ID: r.ID, Title: r.Title, Code: r.Code, DiscountType: r.DiscountType,
		DiscountValue: r.DiscountValue, Currency: r.Currency,
		TermsURL: r.TermsUrl, ReferralURL: r.ReferralUrl, Status: r.Status,
		ValidFrom: r.ValidFrom, ExpiresAt: r.ExpiresAt, AppliesTo: r.AppliesTo,
		MaxRedemptions: r.MaxRedemptions, PerUserLimit: r.PerUserLimit,
		MinSpend: r.MinSpend, FirstTimeOnly: r.FirstTimeOnly,
		OrganizationID: r.OrganizationID, Position: r.Position,
		Overrides: r.Overrides,
	}
}

// couponOverrides mirrors the service_listing_coupons.overrides jsonb
// document — a per-listing re-scope of the coupon's economics. Only the
// whitelisted keys apply; anything else in the blob is ignored.
type couponOverrides struct {
	DiscountValue *float64 `json:"discount_value"`
	MinSpend      *int64   `json:"min_spend"`
}

// numericToFloat64 extracts a finite float from a nullable numeric column.
// Invalid (NULL) numerics yield 0 — a coupon row without discount_value is
// informational only (terms_url/referral_url carry the offer).
func numericToFloat64(n pgtype.Numeric) float64 {
	if !n.Valid {
		return 0
	}

	v, err := n.Float64Value()
	if err != nil || !v.Valid {
		return 0
	}

	return v.Float64
}

// mapListingCoupon projects a couponLinkRow onto the DTO, applying the
// per-listing overrides jsonb when present (discount_value / min_spend may
// be re-scoped for this listing). code/discount_value are nullable on
// coupons — nil-safe throughout.
func mapListingCoupon(r couponLinkRow) ListingCoupon {
	c := ListingCoupon{
		ID:            r.ID.String(),
		Title:         r.Title,
		DiscountValue: numericToFloat64(r.DiscountValue),
		ExpiresAt:     formatTs(r.ExpiresAt),
		Status:        r.Status,
		FirstTimeOnly: r.FirstTimeOnly,
		AppliesTo:     r.AppliesTo,
		Position:      r.Position,
	}
	if r.Code != nil {
		c.Code = *r.Code
	}

	if r.DiscountType != nil {
		c.DiscountType = *r.DiscountType
	}

	if r.Currency != nil {
		c.Currency = *r.Currency
	}

	if r.TermsURL != nil {
		c.TermsURL = *r.TermsURL
	}

	if r.ReferralURL != nil {
		c.ReferralURL = *r.ReferralURL
	}

	if r.MaxRedemptions != nil {
		v := int64(*r.MaxRedemptions)
		c.MaxRedemptions = &v
	}

	if r.PerUserLimit != nil {
		v := int64(*r.PerUserLimit)
		c.PerUserLimit = &v
	}

	c.MinSpend = r.MinSpend

	if r.OrganizationID.Valid {
		c.OrgID = r.OrganizationID.String()
	}

	// Per-listing overrides re-scope the discount economics — applied last
	// so they win over the coupon row's values.
	if len(r.Overrides) > 0 {
		var ov couponOverrides
		if err := json.Unmarshal(r.Overrides, &ov); err == nil {
			if ov.DiscountValue != nil {
				c.DiscountValue = *ov.DiscountValue
			}

			if ov.MinSpend != nil {
				c.MinSpend = ov.MinSpend
			}
		}
	}

	return c
}

// CouponsEnabled reads the service_coupons.enabled flag — gates the attach
// API, the public coupons payload, and the has_coupon list badge.
// Independent of the packages flag so coupons can ship against plain
// service listings.
func (s *Service) CouponsEnabled(ctx context.Context) bool {
	if s.checker != nil {
		return s.checker.IsEnabled(ctx, "service_coupons.enabled")
	}

	if s.settings == nil {
		return false
	}

	return s.settings.GetBoolOr(ctx, "service_coupons.enabled", false)
}

// loadListingForEdit resolves the contents row + meta for a listing-scoped
// read/write and enforces the EditOwn permission shape: the caller must be
// the author, a member of the owning org, or hold service.edit_any
// (computed at the handler). Returns the loaded rows so callers skip a
// second fetch.
func (s *Service) loadListingForEdit(ctx context.Context, contentID, userID pgtype.UUID, isEditAny bool) (gen.GetContentByIDRow, gen.ServiceListingMetum, error) {
	existing, err := s.content.GetContent(ctx, contentID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return gen.GetContentByIDRow{}, gen.ServiceListingMetum{}, pgx.ErrNoRows
		}

		return gen.GetContentByIDRow{}, gen.ServiceListingMetum{}, fmt.Errorf("servicedir: get content: %w", err)
	}

	meta, err := s.store.GetMeta(ctx, contentID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return gen.GetContentByIDRow{}, gen.ServiceListingMetum{}, pgx.ErrNoRows
		}

		return gen.GetContentByIDRow{}, gen.ServiceListingMetum{}, fmt.Errorf("servicedir: get meta: %w", err)
	}

	// service.edit_any holders (admins) bypass the ownership check — same
	// rule EditOwn applies (BUG-009).
	if !isEditAny && existing.AuthorID != userID {
		if !meta.OrgID.Valid {
			return gen.GetContentByIDRow{}, gen.ServiceListingMetum{}, ErrNotOwner
		}

		isMember, err := s.store.IsOrgMember(ctx, meta.OrgID, userID)
		if err != nil {
			return gen.GetContentByIDRow{}, gen.ServiceListingMetum{}, fmt.Errorf("servicedir: check org membership: %w", err)
		}

		if !isMember {
			return gen.GetContentByIDRow{}, gen.ServiceListingMetum{}, ErrNotOwner
		}
	}

	return existing, meta, nil
}

// ListingCouponsDTO returns the listing's attached coupons in link-order.
// activeOnly narrows to currently-redeemable coupons (the public payload);
// the editor attach API passes false to see the full set including
// suspended/expired rows.
func (s *Service) ListingCouponsDTO(ctx context.Context, metaID pgtype.UUID, activeOnly bool) ([]ListingCoupon, error) {
	rows, err := s.store.listListingCoupons(ctx, metaID, activeOnly)
	if err != nil {
		return nil, err
	}

	out := make([]ListingCoupon, 0, len(rows))
	for _, r := range rows {
		out = append(out, mapListingCoupon(r))
	}

	return out, nil
}

// ListingCouponsForEdit returns the full attached coupon set (all
// statuses) for the listing editor — same authz shape as EditOwn.
func (s *Service) ListingCouponsForEdit(ctx context.Context, contentID, userID pgtype.UUID, isEditAny bool) ([]ListingCoupon, error) {
	if !s.CouponsEnabled(ctx) {
		return nil, ErrCouponsDisabled
	}

	_, meta, err := s.loadListingForEdit(ctx, contentID, userID, isEditAny)
	if err != nil {
		return nil, err
	}

	return s.ListingCouponsDTO(ctx, meta.ID, false)
}

// SetListingCoupons replaces the listing's attached coupon set
// (replace-on-save, ordered — position follows slice order). Every id
// must resolve to a coupon owned by the listing's org whose applies_to
// covers the listing kind. Authz mirrors EditOwn: author, owning-org
// member, or service.edit_any. Returns the resulting attach set so the
// handler can echo it without a second fetch round-trip.
func (s *Service) SetListingCoupons(ctx context.Context, contentID, userID pgtype.UUID, isEditAny bool, couponIDs []pgtype.UUID) ([]ListingCoupon, error) {
	if !s.CouponsEnabled(ctx) {
		return nil, ErrCouponsDisabled
	}

	_, meta, err := s.loadListingForEdit(ctx, contentID, userID, isEditAny)
	if err != nil {
		return nil, err
	}

	// Dedupe while preserving order — the link table is
	// UNIQUE(coupon_id, listing_meta_id), and a duplicated id would also
	// inflate the count check.
	uniqueIDs := make([]pgtype.UUID, 0, len(couponIDs))

	seen := make(map[pgtype.UUID]struct{}, len(couponIDs))
	for _, id := range couponIDs {
		if _, dup := seen[id]; dup {
			continue
		}

		seen[id] = struct{}{}

		uniqueIDs = append(uniqueIDs, id)
	}

	if maxCoupons := s.settings.GetIntOr(ctx, "service_packages.coupons.max_per_listing", 3); len(uniqueIDs) > maxCoupons {
		return nil, ErrTooManyCoupons
	}

	for _, cid := range uniqueIDs {
		// Coupons are org-owned; a listing with no owning org has nothing
		// a coupon can attach to.
		if !meta.OrgID.Valid {
			return nil, ErrCouponNotApplicable
		}

		c, err := s.store.getCouponByIDForOrg(ctx, cid, meta.OrgID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				// Missing or owned by another org reads the same — do not
				// leak which coupon ids exist elsewhere.
				return nil, ErrCouponNotApplicable
			}

			return nil, fmt.Errorf("servicedir: coupons: get coupon %s: %w", cid.String(), err)
		}

		if c.AppliesTo != "both" && c.AppliesTo != meta.ListingKind {
			return nil, ErrCouponNotApplicable
		}
	}

	if err := s.store.setListingCoupons(ctx, meta.ID, uniqueIDs); err != nil {
		return nil, err
	}

	return s.ListingCouponsDTO(ctx, meta.ID, false)
}
