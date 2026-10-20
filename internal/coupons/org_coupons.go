package coupons

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// ── Org-Owned Service Coupons (Services Marketplace) ────────────────
//
// Org coupons differ from §55 deal coupons: they are coupon_type='internal',
// owned by an organization (organization_id), live-created as active, and
// carry a redemption rule set (max_redemptions, per_user_limit, min_spend,
// first_time_only, applies_to) enforced at booking time by the services
// package. Manage API: /organizations/{id}/coupons, gated by
// requireOrgEditor in handlers.go.

// OrgCouponInput is the payload for POST/PATCH
// /organizations/{id}/coupons. coupon_type, organization_id and is_verified
// are server-controlled (create pins 'internal'/org/false); status is
// pinned 'active' on create and PATCH accepts only the §2.2 lifecycle
// transitions 'active' ⇄ 'suspended'. On PATCH, absent fields keep the
// stored value; a blank string clears the optional text fields.
type OrgCouponInput struct {
	Title          string              `json:"title"`
	Slug           *string             `json:"slug,omitempty"`
	Code           string              `json:"code"`
	Status         *string             `json:"status,omitempty"`
	Description    *string             `json:"description,omitempty"`
	DiscountType   string              `json:"discount_type,omitempty"`
	DiscountValue  *pgtype.Numeric     `json:"discount_value,omitempty"`
	Currency       *string             `json:"currency,omitempty"`
	TermsURL       *string             `json:"terms_url,omitempty"`
	ReferralURL    *string             `json:"referral_url,omitempty"`
	ValidFrom      *pgtype.Timestamptz `json:"valid_from,omitempty"`
	ExpiresAt      *pgtype.Timestamptz `json:"expires_at,omitempty"`
	MaxRedemptions *int32              `json:"max_redemptions,omitempty"`
	PerUserLimit   *int32              `json:"per_user_limit,omitempty"`
	MinSpend       *int64              `json:"min_spend,omitempty"`
	FirstTimeOnly  *bool               `json:"first_time_only,omitempty"`
	AppliesTo      *string             `json:"applies_to,omitempty"`
}

// orgCouponCodeRe is the stored-code alphabet — codes are normalized to
// uppercase server-side before this check runs (so "save10" is accepted and
// stored as "SAVE10").
var orgCouponCodeRe = regexp.MustCompile(`^[A-Z0-9_-]{3,32}$`)

// normalizeOrgCouponCode uppercases and trims a coupon code for storage and
// comparison — the code is case-insensitive at redemption time.
func normalizeOrgCouponCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

// blankToNil maps a provided-but-blank optional string to nil so PATCH
// payloads like {"terms_url": ""} clear the column instead of storing ”.
func blankToNil(s *string) *string {
	if s != nil && strings.TrimSpace(*s) == "" {
		return nil
	}

	return s
}

// numericPtr returns a copy of n when it is valid, else nil — for merging a
// stored (possibly NULL) numeric into an update input.
func numericPtr(n pgtype.Numeric) *pgtype.Numeric {
	if !n.Valid {
		return nil
	}

	return &n
}

// timestamptzPtr returns a copy of t when it is valid, else nil.
func timestamptzPtr(t pgtype.Timestamptz) *pgtype.Timestamptz {
	if !t.Valid {
		return nil
	}

	return &t
}

// numericToFloat extracts a finite float from an optional numeric input.
// pgtype.Numeric accepts NaN/±Infinity (Postgres numeric does too); every
// comparison against them is false, so they would slip through the >0/<=100
// checks — reject non-finite values outright.
func numericToFloat(n *pgtype.Numeric) (float64, error) {
	if n == nil || !n.Valid {
		return 0, errors.New("missing numeric value")
	}

	v, err := n.Float64Value()
	if err != nil || !v.Valid {
		return 0, errors.New("invalid numeric value")
	}

	if math.IsNaN(v.Float64) || math.IsInf(v.Float64, 0) {
		return 0, errors.New("invalid numeric value")
	}

	return v.Float64, nil
}

// validateOrgCouponInput enforces the services-marketplace coupon rules on a
// fully-populated input — create validates the payload directly; update
// merges the PATCH over the stored row first (mergeOrgCouponInput) so the
// same rules apply to the effective row.
func validateOrgCouponInput(input OrgCouponInput) error {
	if strings.TrimSpace(input.Title) == "" {
		return fmt.Errorf("%w: title is required", ErrInvalidCouponInput)
	}

	code := normalizeOrgCouponCode(input.Code)
	if code == "" {
		return fmt.Errorf("%w: code is required", ErrInvalidCouponInput)
	}

	if !orgCouponCodeRe.MatchString(code) {
		return fmt.Errorf("%w: code must be 3-32 characters (A-Z, 0-9, _ or -)", ErrInvalidCouponInput)
	}

	switch DiscountType(input.DiscountType) {
	case "":
		if input.DiscountValue != nil {
			return fmt.Errorf("%w: discount_value requires discount_type", ErrInvalidCouponInput)
		}
	case DiscountTypePercent:
		v, err := numericToFloat(input.DiscountValue)
		if err != nil || v <= 0 || v > 100 {
			return fmt.Errorf("%w: percent discount_value must be >0 and <=100", ErrInvalidCouponInput)
		}
	case DiscountTypeFixed:
		v, err := numericToFloat(input.DiscountValue)
		if err != nil || v <= 0 {
			return fmt.Errorf("%w: fixed discount_value must be >0", ErrInvalidCouponInput)
		}
	case DiscountTypeFreeShipping:
		// free_shipping is meaningless for services — reject explicitly so
		// it does not silently pass through to the CHECK-less column.
		return fmt.Errorf("%w: free_shipping", ErrUnsupportedDiscount)
	default:
		return fmt.Errorf("%w: unknown discount_type %q", ErrInvalidCouponInput, input.DiscountType)
	}

	if input.ValidFrom != nil && input.ExpiresAt != nil &&
		input.ExpiresAt.Time.Before(input.ValidFrom.Time) {
		return fmt.Errorf("%w: expires_at must not precede valid_from", ErrInvalidCouponInput)
	}

	if input.MaxRedemptions != nil && *input.MaxRedemptions < 0 {
		return fmt.Errorf("%w: max_redemptions must be >= 0", ErrInvalidCouponInput)
	}

	if input.PerUserLimit != nil && *input.PerUserLimit < 0 {
		return fmt.Errorf("%w: per_user_limit must be >= 0", ErrInvalidCouponInput)
	}

	if input.MinSpend != nil && *input.MinSpend < 0 {
		return fmt.Errorf("%w: min_spend must be >= 0", ErrInvalidCouponInput)
	}

	if input.AppliesTo != nil && *input.AppliesTo != "" {
		switch CouponAppliesTo(*input.AppliesTo) {
		case AppliesToService, AppliesToPackage, AppliesToBoth:
		default:
			return fmt.Errorf("%w: applies_to must be 'service', 'package' or 'both'", ErrInvalidCouponInput)
		}
	}

	if err := validateOutboundURL("terms_url", input.TermsURL); err != nil {
		return err
	}

	return validateOutboundURL("referral_url", input.ReferralURL)
}

// mapOrgCouponWriteErr translates unique-violation errors on the org coupon
// write path: a 23505 on the global coupons_code_key index is a duplicate
// code (coupon_code_taken → 409); a slug 23505 means a concurrent claim
// (slugs are pre-resolved by resolveCouponSlug).
func mapOrgCouponWriteErr(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		switch pgErr.ConstraintName {
		case "coupons_code_key":
			return ErrCouponCodeTaken
		case "idx_coupons_slug":
			return ErrCouponAlreadyExists
		}
	}

	return err
}

// mergeOrgCouponInput overlays a PATCH payload onto the stored row: blank
// title/code/discount_type mean "keep current", and nil pointers mean "keep
// current" for every optional field — UpdateOrgCoupon's plain narg params
// would otherwise NULL the column. FirstTimeOnly and AppliesTo are merged
// too so validation sees the effective row.
func mergeOrgCouponInput(input OrgCouponInput, cur gen.Coupon) OrgCouponInput {
	merged := input
	if merged.Title = strings.TrimSpace(merged.Title); merged.Title == "" {
		merged.Title = cur.Title
	}

	if merged.Code = normalizeOrgCouponCode(merged.Code); merged.Code == "" {
		merged.Code = derefStr(cur.Code)
	}

	if merged.Description == nil {
		merged.Description = cur.Description
	}

	if merged.DiscountType = strings.TrimSpace(merged.DiscountType); merged.DiscountType == "" {
		merged.DiscountType = derefStr(cur.DiscountType)
	}

	if merged.DiscountValue == nil {
		merged.DiscountValue = numericPtr(cur.DiscountValue)
	}

	if merged.Currency == nil {
		merged.Currency = cur.Currency
	}

	if merged.TermsURL == nil {
		merged.TermsURL = cur.TermsUrl
	}

	if merged.ReferralURL == nil {
		merged.ReferralURL = cur.ReferralUrl
	}

	if merged.ValidFrom == nil {
		merged.ValidFrom = timestamptzPtr(cur.ValidFrom)
	}

	if merged.ExpiresAt == nil {
		merged.ExpiresAt = timestamptzPtr(cur.ExpiresAt)
	}

	if merged.MaxRedemptions == nil {
		merged.MaxRedemptions = cur.MaxRedemptions
	}

	if merged.PerUserLimit == nil {
		merged.PerUserLimit = cur.PerUserLimit
	}

	if merged.MinSpend == nil {
		merged.MinSpend = cur.MinSpend
	}

	if merged.FirstTimeOnly == nil {
		b := cur.FirstTimeOnly
		merged.FirstTimeOnly = &b
	}

	if merged.AppliesTo == nil || *merged.AppliesTo == "" {
		a := cur.AppliesTo
		merged.AppliesTo = &a
	}

	return merged
}

// resolveOrgCouponStatus validates a PATCH status request against the
// stored row — the org coupon lifecycle (spec §2.2) is active ⇄ suspended,
// so any other target is invalid_input. Expired coupons cannot be
// re-activated by a status flip: a stored 'expired' status or a lapsed
// expires_at (stored, or set by this same PATCH) keeps them out of
// circulation. A nil request means "keep current".
func resolveOrgCouponStatus(requested *string, current gen.Coupon, mergedExpiresAt *pgtype.Timestamptz) (string, error) {
	if requested == nil {
		return current.Status, nil
	}

	target := CouponStatus(strings.ToLower(strings.TrimSpace(*requested)))
	if target != CouponStatusActive && target != CouponStatusSuspended {
		return "", fmt.Errorf("%w: status must be 'active' or 'suspended'", ErrInvalidCouponInput)
	}

	if target == CouponStatusActive {
		now := time.Now()
		if current.Status == string(CouponStatusExpired) ||
			(current.ExpiresAt.Valid && !current.ExpiresAt.Time.After(now)) ||
			(mergedExpiresAt != nil && !mergedExpiresAt.Time.After(now)) {
			return "", fmt.Errorf("%w: expired coupons cannot be reactivated", ErrInvalidCouponInput)
		}
	}

	return string(target), nil
}

// orgCouponUpdateParams projects a validated OrgCouponInput onto the
// UpdateOrgCoupon query params. Status is lifecycle-controlled: the stored
// status passes through unless a PATCH supplied an 'active'/'suspended'
// transition already vetted by resolveOrgCouponStatus.
func orgCouponUpdateParams(input OrgCouponInput, status string, id, orgID pgtype.UUID) gen.UpdateOrgCouponParams {
	return gen.UpdateOrgCouponParams{
		Title:          input.Title,
		Description:    blankToNil(input.Description),
		Code:           strPtrOrNil(normalizeOrgCouponCode(input.Code)),
		DiscountType:   strPtrOrNil(input.DiscountType),
		DiscountValue:  toNullableNumeric(input.DiscountValue),
		Currency:       blankToNil(input.Currency),
		TermsUrl:       blankToNil(input.TermsURL),
		ReferralUrl:    blankToNil(input.ReferralURL),
		Status:         status,
		ValidFrom:      toTimestamptz(input.ValidFrom),
		ExpiresAt:      toTimestamptz(input.ExpiresAt),
		MaxRedemptions: input.MaxRedemptions,
		PerUserLimit:   input.PerUserLimit,
		MinSpend:       input.MinSpend,
		FirstTimeOnly:  input.FirstTimeOnly,
		AppliesTo:      input.AppliesTo,
		ID:             id,
		OrganizationID: orgID,
	}
}

// CreateOrgCoupon inserts an org-owned internal coupon pinned to
// status='active' and is_verified=false (CreateOrgCoupon query). The slug
// derives from the normalized code via resolveCouponSlug ('<code-lower>'
// plus a numeric suffix on collision), or from an explicit input slug.
func (s *Service) CreateOrgCoupon(ctx context.Context, orgID, userID pgtype.UUID, input OrgCouponInput) (gen.Coupon, error) {
	input.Title = strings.TrimSpace(input.Title)
	input.Code = normalizeOrgCouponCode(input.Code)
	input.DiscountType = strings.TrimSpace(input.DiscountType)

	if err := validateOrgCouponInput(input); err != nil {
		return gen.Coupon{}, err
	}

	slug, err := s.resolveCouponSlug(ctx, input.Slug, input.Code, pgtype.UUID{})
	if err != nil {
		return gen.Coupon{}, err
	}

	var firstTimeOnly any
	if input.FirstTimeOnly != nil {
		firstTimeOnly = *input.FirstTimeOnly
	}

	var appliesTo any
	if input.AppliesTo != nil && *input.AppliesTo != "" {
		appliesTo = *input.AppliesTo
	}

	c, err := s.store.createOrgCoupon(ctx, gen.CreateOrgCouponParams{
		Title: input.Title, Slug: slug,
		Description: blankToNil(input.Description), Code: strPtrOrNil(input.Code),
		DiscountType: strPtrOrNil(input.DiscountType), DiscountValue: toNullableNumeric(input.DiscountValue),
		Currency: blankToNil(input.Currency), TermsUrl: blankToNil(input.TermsURL),
		ReferralUrl: blankToNil(input.ReferralURL),
		ValidFrom:   toTimestamptz(input.ValidFrom), ExpiresAt: toTimestamptz(input.ExpiresAt),
		CreatedBy: userID, OrganizationID: orgID,
		MaxRedemptions: input.MaxRedemptions, PerUserLimit: input.PerUserLimit,
		MinSpend: input.MinSpend, FirstTimeOnly: firstTimeOnly, AppliesTo: appliesTo,
	})
	if err != nil {
		return gen.Coupon{}, mapOrgCouponWriteErr(err)
	}

	s.invalidateCouponCaches(ctx, c.ID, c.Slug)

	return c, nil
}

// ListOrgCoupons lists an org's service coupons in all statuses with
// per-coupon redemption_count and listing_count rollups.
func (s *Service) ListOrgCoupons(ctx context.Context, orgID pgtype.UUID, page, pageSize int32) ([]gen.ListOrgCouponsRow, int64, error) {
	limit, offset := paginate(page, pageSize)
	rows, err := s.store.listOrgCoupons(ctx, orgID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.store.countOrgCoupons(ctx, orgID)
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// UpdateOrgCoupon PATCHes an org coupon — GetCouponByIDForOrg enforces org
// ownership (a coupon belonging to another org, or a non-org coupon, reads
// as ErrCouponNotFound), then the payload is merged over the stored row and
// re-validated before the full-column update.
func (s *Service) UpdateOrgCoupon(ctx context.Context, orgID, couponID pgtype.UUID, input OrgCouponInput) (gen.Coupon, error) {
	current, err := s.store.getCouponByIDForOrg(ctx, couponID, orgID)
	if err != nil {
		if isNoRows(err) {
			return gen.Coupon{}, ErrCouponNotFound
		}

		return gen.Coupon{}, fmt.Errorf("getting org coupon: %w", err)
	}

	merged := mergeOrgCouponInput(input, current)
	if err := validateOrgCouponInput(merged); err != nil {
		return gen.Coupon{}, err
	}

	status, err := resolveOrgCouponStatus(input.Status, current, merged.ExpiresAt)
	if err != nil {
		return gen.Coupon{}, err
	}

	c, err := s.store.updateOrgCoupon(ctx, orgCouponUpdateParams(merged, status, couponID, orgID))
	if err != nil {
		if isNoRows(err) {
			// The WHERE id+organization_id guard raced a delete/move.
			return gen.Coupon{}, ErrCouponNotFound
		}

		return gen.Coupon{}, mapOrgCouponWriteErr(err)
	}

	s.invalidateCouponCaches(ctx, c.ID, c.Slug)

	return c, nil
}

// DeleteOrgCoupon removes an org coupon. When the coupon has redemptions the
// row is suspended instead of hard-deleted: coupon_redemptions.coupon_id is
// ON DELETE CASCADE, so a hard delete would silently erase the audit trail.
// Zero-redemption coupons hard-delete (service_listing_coupons attachments
// cascade harmlessly).
func (s *Service) DeleteOrgCoupon(ctx context.Context, orgID, couponID pgtype.UUID) error {
	current, err := s.store.getCouponByIDForOrg(ctx, couponID, orgID)
	if err != nil {
		if isNoRows(err) {
			return ErrCouponNotFound
		}

		return fmt.Errorf("getting org coupon: %w", err)
	}

	redemptions, err := s.store.countOrgCouponRedemptions(ctx, couponID)
	if err != nil {
		return fmt.Errorf("counting org coupon redemptions: %w", err)
	}

	if redemptions > 0 {
		if current.Status == string(CouponStatusSuspended) {
			return nil
		}
		// Suspend-instead-of-delete: every stored field is passed back
		// unchanged except status (the query's plain narg params would NULL
		// them otherwise); the COALESCE'd flags stay at their current values.
		_, err = s.store.updateOrgCoupon(ctx, gen.UpdateOrgCouponParams{
			Title: current.Title, Description: current.Description,
			Code: current.Code, DiscountType: current.DiscountType,
			DiscountValue: current.DiscountValue, Currency: current.Currency,
			TermsUrl: current.TermsUrl, ReferralUrl: current.ReferralUrl,
			Status:    string(CouponStatusSuspended),
			ValidFrom: current.ValidFrom, ExpiresAt: current.ExpiresAt,
			MaxRedemptions: current.MaxRedemptions, PerUserLimit: current.PerUserLimit,
			MinSpend: current.MinSpend, FirstTimeOnly: nil, AppliesTo: nil,
			ID: couponID, OrganizationID: orgID,
		})
		if err != nil {
			if isNoRows(err) {
				// The WHERE id+organization_id guard raced a delete/move.
				return ErrCouponNotFound
			}

			return mapOrgCouponWriteErr(err)
		}

		s.invalidateCouponCaches(ctx, couponID, current.Slug)

		return nil
	}

	rows, err := s.store.deleteOrgCoupon(ctx, couponID, orgID)
	if err != nil {
		return err
	}

	if rows == 0 {
		return ErrCouponNotFound
	}

	s.invalidateCouponCaches(ctx, couponID, current.Slug)

	return nil
}
