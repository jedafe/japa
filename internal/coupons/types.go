// Package coupons implements the Coupons & External Affiliate Providers
// system for JapaConnect (Section 55).
//
// Two related systems:
//  1. External Affiliate Providers — partnerships with external services
//  2. Coupon Codes — both external (partner) and internal (platform) discounts
//
// Architecture principle (P4 — Render-time over save-time): coupon and affiliate
// link entities store IDs, not resolved values. Resolution happens at render time.
package coupons

import "errors"

// Exported var values.
var (
	// ErrCouponNotFound is a sentinel error: coupon not found.
	ErrCouponNotFound      = errors.New("coupon not found")
	ErrProviderNotFound    = errors.New("affiliate provider not found")
	ErrProviderHasCoupons  = errors.New("cannot delete provider with existing coupons")
	ErrCouponAlreadyExists = errors.New("coupon code already exists")
	ErrInvalidStatus       = errors.New("invalid coupon status")
	// ErrDealRedirectUnavailable is a sentinel error: the deal has no
	// click-through target (no referral_url and no provider base_url).
	ErrDealRedirectUnavailable = errors.New("deal redirect unavailable")
)

// InternalDealProviderSlug is the reserved provider URL segment used by
// /deal/{provider}/{slug} for internal coupons that have no
// affiliate_providers row (provider_id IS NULL).
const InternalDealProviderSlug = "japaconnect"

// reservedCouponSlugs are coupon slug values that would collide with static
// route segments under /api/v1/deals/*.
var reservedCouponSlugs = map[string]bool{"partners": true}

// CouponStatus represents the lifecycle state of a coupon.
type CouponStatus string

// Exported const values.
const (
	// CouponStatusActive is an exported constant.
	CouponStatusActive    CouponStatus = "active"
	CouponStatusExpired   CouponStatus = "expired"
	CouponStatusSuspended CouponStatus = "suspended"
)

// CouponType distinguishes external partner coupons from internal platform coupons.
type CouponType string

// Exported const values.
const (
	// CouponTypeExternal is an exported constant.
	CouponTypeExternal CouponType = "external"
	CouponTypeInternal CouponType = "internal"
)

// DiscountType describes how the coupon discount is applied.
type DiscountType string

// Exported const values.
const (
	// DiscountTypePercent is an exported constant.
	DiscountTypePercent      DiscountType = "percent"
	DiscountTypeFixed        DiscountType = "fixed"
	DiscountTypeFreeShipping DiscountType = "free_shipping"
)

// ProviderType classifies affiliate providers.
type ProviderType string

// Exported const values.
const (
	// ProviderTypeAffiliate is an exported constant.
	ProviderTypeAffiliate ProviderType = "affiliate"
	ProviderTypeCoupon    ProviderType = "coupon"
	ProviderTypeBoth      ProviderType = "both"
)

// Pagination defaults.
const (
	DefaultPageSize = 20
	MaxPageSize     = 100
)
