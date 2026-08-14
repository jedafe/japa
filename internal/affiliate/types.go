// Package affiliate implements the User Affiliate Program for JapaConnect (Section 56).
//
// Users earn commission (credited as Points or Credits) by referring others
// to JapaConnect paid products. Each user can generate product-specific affiliate
// links. Clicks are tracked via a redirect endpoint; conversions are attributed
// when a payment is completed.
//
// Distinct from internal/coupons: that package covers JapaConnect acting as an
// affiliate for external providers. This package covers users acting as
// affiliates for JapaConnect's own products.
package affiliate

import "errors"

var (
	ErrMemberNotFound     = errors.New("affiliate member not found")
	ErrAlreadyEnrolled    = errors.New("user is already enrolled in the affiliate program")
	ErrNotEnrolled        = errors.New("user is not enrolled in the affiliate program")
	ErrLinkNotFound       = errors.New("affiliate link not found")
	ErrLinkNotActive      = errors.New("affiliate link is not active")
	ErrTooManyLinks       = errors.New("affiliate link limit reached")
	ErrAccountTooNew      = errors.New("account does not meet minimum age requirement")
	ErrFeatureDisabled    = errors.New("affiliate program is not enabled")
	ErrInvalidProductType = errors.New("invalid product type")
	ErrMemberSuspended    = errors.New("affiliate member is suspended or banned")
)

// MemberStatus represents the enrollment status of an affiliate member.
type MemberStatus string

const (
	MemberStatusActive    MemberStatus = "active"
	MemberStatusSuspended MemberStatus = "suspended"
	MemberStatusBanned    MemberStatus = "banned"
)

// ProductType represents the type of product an affiliate link targets.
type ProductType string

const (
	ProductTypeProSubscription ProductType = "pro_subscription"
	ProductTypeCourse          ProductType = "course"
	ProductTypeGeneral         ProductType = "general"
)

// EventType represents the type of affiliate link event.
type EventType string

const (
	EventTypeClick      EventType = "click"
	EventTypeConversion EventType = "conversion"
)

// ValidProductTypes returns all valid product types.
func ValidProductTypes() []string {
	return []string{
		string(ProductTypeProSubscription),
		string(ProductTypeCourse),
		string(ProductTypeGeneral),
	}
}

// Pagination defaults.
const (
	DefaultPageSize = 20
	MaxPageSize     = 100
)
