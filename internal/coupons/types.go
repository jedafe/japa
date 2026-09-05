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
)

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
