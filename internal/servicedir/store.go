// Package servicedir implements the Service Directory (§98, ADR-106) and
// Service Packages & Bookings (§111, ADR-123).
// // Service listings are records in the contents table with
// content_type = 'service_listing', extended by the
// service_listing_meta companion table for structured fields
// (service category, destination/city spaces, origin communities,
// remote flag, website/booking URLs, price range, verification,
// organization claim tracking, and the §111 package pricing/purchase
// fields). Packages additionally use service_package_items,
// service_package_departures, and service_bookings.
// // All data access goes through sqlc-generated queries
// (internal/db/queries/services.sql) — no hand-written SQL.
// // Ref: japaconnect-engineering/sections/98-service-directory.md
// //	japaconnect-engineering/sections/111-service-packages-bookings.md
// //	japaconnect-engineering/adr/ADR-106-service-directory.md
// //	japaconnect-engineering/adr/ADR-123-service-packages.md
package servicedir

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// errNotFound is returned by guard-style mutations (:execrows with zero
// rows affected) — maps to pgx.ErrNoRows semantics for the service layer.
var errNotFound = pgx.ErrNoRows

// Service categories (§98 schema: immigration_legal|relocation|finance|education|health|community|other).
const (
	CategoryImmigrationLegal = "immigration_legal"
	CategoryRelocation       = "relocation"
	CategoryFinance          = "finance"
	CategoryEducation        = "education"
	CategoryHealth           = "health"
	CategoryCommunity        = "community"
	CategoryOther            = "other"
)

// validCategories is the canonical set used for input validation (BUG-013).
var validCategories = map[string]struct{}{
	CategoryImmigrationLegal: {},
	CategoryRelocation:       {},
	CategoryFinance:          {},
	CategoryEducation:        {},
	CategoryHealth:           {},
	CategoryCommunity:        {},
	CategoryOther:            {},
}

// IsValidCategory reports whether s is a canonical §98 service_category.
func IsValidCategory(s string) bool {
	_, ok := validCategories[s]
	return ok
}

// IsValidVerificationType reports whether s is a canonical §98
// verification_type.
func IsValidVerificationType(s string) bool {
	_, ok := validVerificationTypes[s]
	return ok
}

// Claim statuses (§98 schema: unclaimed|pending|approved|rejected).
const (
	ClaimStatusUnclaimed = "unclaimed"
	ClaimStatusPending   = "pending"
	ClaimStatusApproved  = "approved"
	ClaimStatusRejected  = "rejected"
)

// Verification types (§98 schema: self|community|admin|professional_body).
const (
	VerificationTypeSelf             = "self"
	VerificationTypeCommunity        = "community"
	VerificationTypeAdmin            = "admin"
	VerificationTypeProfessionalBody = "professional_body"
)

var validVerificationTypes = map[string]struct{}{
	VerificationTypeSelf:             {},
	VerificationTypeCommunity:        {},
	VerificationTypeAdmin:            {},
	VerificationTypeProfessionalBody: {},
}

// §111 listing kinds.
const (
	ListingKindService = "service"
	ListingKindPackage = "package"
)

// IsValidListingKind reports whether s is a canonical §111 listing_kind.
func IsValidListingKind(s string) bool {
	return s == ListingKindService || s == ListingKindPackage
}

// §111 purchase modes. 'full' is intentionally absent — deferred until
// agent payout infrastructure exists (§111 Non-Goals).
const (
	PurchaseModeInquiry  = "inquiry"
	PurchaseModeExternal = "external"
	PurchaseModePaid     = "paid"
)

// IsValidPurchaseMode reports whether s is a canonical §111 purchase_mode.
// 'full' is deliberately not accepted — the API returns
// 403 purchase_mode_unavailable instead (§111).
func IsValidPurchaseMode(s string) bool {
	return s == PurchaseModeInquiry || s == PurchaseModeExternal || s == PurchaseModePaid
}

// §111 price_per units.
var validPricePer = map[string]struct{}{
	"person":  {},
	"package": {},
	"group":   {},
}

// IsValidPricePer reports whether s is a canonical §111 price_per value.
func IsValidPricePer(s string) bool {
	_, ok := validPricePer[s]
	return ok
}

// §111 booking intents.
const (
	BookingIntentInquiry = "inquiry"
	BookingIntentBooking = "booking"
)

// §111 booking statuses.
const (
	BookingStatusPending   = "pending"
	BookingStatusConfirmed = "confirmed"
	BookingStatusFulfilled = "fulfilled"
	BookingStatusCancelled = "cancelled"
	BookingStatusDeclined  = "declined"
	BookingStatusDisputed  = "disputed"
	BookingStatusRefunded  = "refunded"
)

// IsValidBookingStatus reports whether s is a canonical §111 booking status.
func IsValidBookingStatus(s string) bool {
	switch s {
	case BookingStatusPending, BookingStatusConfirmed, BookingStatusFulfilled,
		BookingStatusCancelled, BookingStatusDeclined, BookingStatusDisputed,
		BookingStatusRefunded:
		return true
	}

	return false
}

// Departure statuses (§111).
const (
	DepartureStatusOpen      = "open"
	DepartureStatusFull      = "full"
	DepartureStatusCancelled = "cancelled"
)

// Store is the data-access layer for service_listing_meta, package
// items/departures, and service bookings. It wraps sqlc-generated queries
// (read/write pool split) — no business logic here.
type Store struct {
	read  *gen.Queries
	write *gen.Queries
	pool  *db.DB // retained for transaction boundaries (capacity-guarded confirm)
}

// NewStore creates a service directory store from sqlc query handles.
func NewStore(read, write *gen.Queries, database *db.DB) *Store {
	return &Store{read: read, write: write, pool: database}
}

// ---------------------------------------------------------------------------
// Domain row types (mapped from the sqlc joined rows — a single shared shape)
// ---------------------------------------------------------------------------

// ServiceRow is the joined result of contents + service_listing_meta +
// spaces + organizations, returned by list and detail queries.
type ServiceRow struct {
	ID pgtype.UUID

	AuthorID          pgtype.UUID
	Title             string
	Slug              string
	Body              string
	BodyHTML          string
	Status            string
	Visibility        string
	ViewCount         int32
	CommentCount      int32
	PublishedAt       pgtype.Timestamptz
	CreatedAt         pgtype.Timestamptz
	UpdatedAt         pgtype.Timestamptz
	MetaID            pgtype.UUID
	OrgID             pgtype.UUID
	OrgName           *string
	OrgSlug           *string
	ServiceCategory   *string
	DestSpaceID       pgtype.UUID
	DestSpaceName     *string
	DestSpaceCode     *string
	CitySpaceID       pgtype.UUID
	CitySpaceName     *string
	CitySpaceCode     *string
	OriginCommunities []string
	IsRemote          bool
	WebsiteURL        *string
	BookingURL        *string
	PriceRange        *string
	IsVerified        bool
	VerificationType  *string
	ClaimStatus       *string
	ClaimedByOrgID    pgtype.UUID
	ClaimedOrgName    *string
	VerifiedBy        pgtype.UUID
	VerifiedAt        pgtype.Timestamptz
	AuthorName        *string
	AuthorAvatar      *string

	// §111 package fields
	ListingKind    string
	PriceAmount    *int64
	PriceCurrency  *string
	PricePer       *string
	PurchaseMode   string
	Capacity       *int32
	AvailableFrom  pgtype.Date
	AvailableUntil pgtype.Date

	// HasCoupon is the list-query badge column — true when the listing has
	// at least one currently-active attached coupon. Populated only by the
	// list/org-list queries (the EXISTS projection); detail rows leave it
	// false.
	HasCoupon bool

	// MinPrice is the list-query "From $X" column — the lowest active
	// service_package_tiers price, falling back to the meta row's
	// price_amount. Populated only by the list/org-list queries (the
	// COALESCE subquery projection); detail rows leave it nil.
	MinPrice *int64
}

// rowFromList maps a generated list row to the shared domain row.
func rowFromList(r gen.ListServiceListingsRow) ServiceRow {
	return ServiceRow{
		ID: r.ID, AuthorID: r.AuthorID, Title: r.Title, Slug: r.Slug,
		Body: r.Body, BodyHTML: r.BodyHtml, Status: r.Status,
		Visibility: r.Visibility, ViewCount: r.ViewCount,
		CommentCount: r.CommentCount, PublishedAt: r.PublishedAt,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		MetaID: r.MetaID, OrgID: r.OrgID, OrgName: r.OrgName, OrgSlug: r.OrgSlug,
		ServiceCategory: &r.ServiceCategory,
		DestSpaceID:     r.DestinationSpaceID, DestSpaceName: r.DestSpaceName, DestSpaceCode: r.DestSpaceCode,
		CitySpaceID: r.CitySpaceID, CitySpaceName: r.CitySpaceName, CitySpaceCode: r.CitySpaceCode,
		OriginCommunities: r.OriginCommunities, IsRemote: r.IsRemote,
		WebsiteURL: r.WebsiteUrl, BookingURL: r.BookingUrl,
		PriceRange: r.PriceRange, IsVerified: r.IsVerified,
		VerificationType: r.VerificationType,
		ClaimStatus:      &r.ClaimStatus, ClaimedByOrgID: r.ClaimedByOrgID,
		ClaimedOrgName: r.ClaimedOrgName, VerifiedBy: r.VerifiedBy,
		VerifiedAt: r.VerifiedAt, AuthorName: r.AuthorName, AuthorAvatar: r.AuthorAvatar,
		ListingKind: r.ListingKind, PriceAmount: r.PriceAmount,
		PriceCurrency: r.PriceCurrency, PricePer: r.PricePer,
		PurchaseMode: r.PurchaseMode, Capacity: r.Capacity,
		AvailableFrom: r.AvailableFrom, AvailableUntil: r.AvailableUntil,
		HasCoupon: r.HasCoupon, MinPrice: r.MinPrice,
	}
}

// rowFromSlug maps a generated detail row to the shared domain row.
func rowFromSlug(r gen.GetServiceListingBySlugRow) ServiceRow {
	return ServiceRow{
		ID: r.ID, AuthorID: r.AuthorID, Title: r.Title, Slug: r.Slug,
		Body: r.Body, BodyHTML: r.BodyHtml, Status: r.Status,
		Visibility: r.Visibility, ViewCount: r.ViewCount,
		CommentCount: r.CommentCount, PublishedAt: r.PublishedAt,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		MetaID: r.MetaID, OrgID: r.OrgID, OrgName: r.OrgName, OrgSlug: r.OrgSlug,
		ServiceCategory: &r.ServiceCategory,
		DestSpaceID:     r.DestinationSpaceID, DestSpaceName: r.DestSpaceName, DestSpaceCode: r.DestSpaceCode,
		CitySpaceID: r.CitySpaceID, CitySpaceName: r.CitySpaceName, CitySpaceCode: r.CitySpaceCode,
		OriginCommunities: r.OriginCommunities, IsRemote: r.IsRemote,
		WebsiteURL: r.WebsiteUrl, BookingURL: r.BookingUrl,
		PriceRange: r.PriceRange, IsVerified: r.IsVerified,
		VerificationType: r.VerificationType,
		ClaimStatus:      &r.ClaimStatus, ClaimedByOrgID: r.ClaimedByOrgID,
		ClaimedOrgName: r.ClaimedOrgName, VerifiedBy: r.VerifiedBy,
		VerifiedAt: r.VerifiedAt, AuthorName: r.AuthorName, AuthorAvatar: r.AuthorAvatar,
		ListingKind: r.ListingKind, PriceAmount: r.PriceAmount,
		PriceCurrency: r.PriceCurrency, PricePer: r.PricePer,
		PurchaseMode: r.PurchaseMode, Capacity: r.Capacity,
		AvailableFrom: r.AvailableFrom, AvailableUntil: r.AvailableUntil,
	}
}

// rowFromContentID maps a generated content-id detail row to the shared
// domain row.
func rowFromContentID(r gen.GetServiceListingByContentIDRow) ServiceRow {
	return ServiceRow{
		ID: r.ID, AuthorID: r.AuthorID, Title: r.Title, Slug: r.Slug,
		Body: r.Body, BodyHTML: r.BodyHtml, Status: r.Status,
		Visibility: r.Visibility, ViewCount: r.ViewCount,
		CommentCount: r.CommentCount, PublishedAt: r.PublishedAt,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		MetaID: r.MetaID, OrgID: r.OrgID, OrgName: r.OrgName, OrgSlug: r.OrgSlug,
		ServiceCategory: &r.ServiceCategory,
		DestSpaceID:     r.DestinationSpaceID, DestSpaceName: r.DestSpaceName, DestSpaceCode: r.DestSpaceCode,
		CitySpaceID: r.CitySpaceID, CitySpaceName: r.CitySpaceName, CitySpaceCode: r.CitySpaceCode,
		OriginCommunities: r.OriginCommunities, IsRemote: r.IsRemote,
		WebsiteURL: r.WebsiteUrl, BookingURL: r.BookingUrl,
		PriceRange: r.PriceRange, IsVerified: r.IsVerified,
		VerificationType: r.VerificationType,
		ClaimStatus:      &r.ClaimStatus, ClaimedByOrgID: r.ClaimedByOrgID,
		ClaimedOrgName: r.ClaimedOrgName, VerifiedBy: r.VerifiedBy,
		VerifiedAt: r.VerifiedAt, AuthorName: r.AuthorName, AuthorAvatar: r.AuthorAvatar,
		ListingKind: r.ListingKind, PriceAmount: r.PriceAmount,
		PriceCurrency: r.PriceCurrency, PricePer: r.PricePer,
		PurchaseMode: r.PurchaseMode, Capacity: r.Capacity,
		AvailableFrom: r.AvailableFrom, AvailableUntil: r.AvailableUntil,
	}
}

// rowFromAdmin maps a generated admin list row to the shared domain row.
func rowFromAdmin(r gen.AdminListServiceListingsRow) ServiceRow {
	return ServiceRow{
		ID: r.ID, AuthorID: r.AuthorID, Title: r.Title, Slug: r.Slug,
		Body: r.Body, BodyHTML: r.BodyHtml, Status: r.Status,
		Visibility: r.Visibility, ViewCount: r.ViewCount,
		CommentCount: r.CommentCount, PublishedAt: r.PublishedAt,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		MetaID: r.MetaID, OrgID: r.OrgID, OrgName: r.OrgName, OrgSlug: r.OrgSlug,
		ServiceCategory: &r.ServiceCategory,
		DestSpaceID:     r.DestinationSpaceID, DestSpaceName: r.DestSpaceName, DestSpaceCode: r.DestSpaceCode,
		CitySpaceID: r.CitySpaceID, CitySpaceName: r.CitySpaceName, CitySpaceCode: r.CitySpaceCode,
		OriginCommunities: r.OriginCommunities, IsRemote: r.IsRemote,
		WebsiteURL: r.WebsiteUrl, BookingURL: r.BookingUrl,
		PriceRange: r.PriceRange, IsVerified: r.IsVerified,
		VerificationType: r.VerificationType,
		ClaimStatus:      &r.ClaimStatus, ClaimedByOrgID: r.ClaimedByOrgID,
		ClaimedOrgName: r.ClaimedOrgName, VerifiedBy: r.VerifiedBy,
		VerifiedAt: r.VerifiedAt, AuthorName: r.AuthorName, AuthorAvatar: r.AuthorAvatar,
		ListingKind: r.ListingKind, PriceAmount: r.PriceAmount,
		PriceCurrency: r.PriceCurrency, PricePer: r.PricePer,
		PurchaseMode: r.PurchaseMode, Capacity: r.Capacity,
		AvailableFrom: r.AvailableFrom, AvailableUntil: r.AvailableUntil,
	}
}

// rowFromOrg maps a generated org list row to the shared domain row.
func rowFromOrg(r gen.ListServiceListingsByOrgRow) ServiceRow {
	return ServiceRow{
		ID: r.ID, AuthorID: r.AuthorID, Title: r.Title, Slug: r.Slug,
		Body: r.Body, BodyHTML: r.BodyHtml, Status: r.Status,
		Visibility: r.Visibility, ViewCount: r.ViewCount,
		CommentCount: r.CommentCount, PublishedAt: r.PublishedAt,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		MetaID: r.MetaID, OrgID: r.OrgID, OrgName: r.OrgName, OrgSlug: r.OrgSlug,
		ServiceCategory: &r.ServiceCategory,
		DestSpaceID:     r.DestinationSpaceID, DestSpaceName: r.DestSpaceName, DestSpaceCode: r.DestSpaceCode,
		CitySpaceID: r.CitySpaceID, CitySpaceName: r.CitySpaceName, CitySpaceCode: r.CitySpaceCode,
		OriginCommunities: r.OriginCommunities, IsRemote: r.IsRemote,
		WebsiteURL: r.WebsiteUrl, BookingURL: r.BookingUrl,
		PriceRange: r.PriceRange, IsVerified: r.IsVerified,
		VerificationType: r.VerificationType,
		ClaimStatus:      &r.ClaimStatus, ClaimedByOrgID: r.ClaimedByOrgID,
		ClaimedOrgName: r.ClaimedOrgName, VerifiedBy: r.VerifiedBy,
		VerifiedAt: r.VerifiedAt, AuthorName: r.AuthorName, AuthorAvatar: r.AuthorAvatar,
		ListingKind: r.ListingKind, PriceAmount: r.PriceAmount,
		PriceCurrency: r.PriceCurrency, PricePer: r.PricePer,
		PurchaseMode: r.PurchaseMode, Capacity: r.Capacity,
		AvailableFrom: r.AvailableFrom, AvailableUntil: r.AvailableUntil,
		HasCoupon: r.HasCoupon, MinPrice: r.MinPrice,
	}
}

// ---------------------------------------------------------------------------
// service_listing_meta CRUD
// ---------------------------------------------------------------------------

// CreateMeta inserts a service_listing_meta row linked to a contents row.
func (s *Store) CreateMeta(ctx context.Context, p gen.CreateServiceListingMetaParams) (gen.ServiceListingMetum, error) {
	row, err := s.write.CreateServiceListingMeta(ctx, p)
	if err != nil {
		return gen.ServiceListingMetum{}, fmt.Errorf("servicedir: create service_listing_meta: %w", err)
	}

	return row, nil
}

// GetMeta retrieves the service_listing_meta row for a given content_id.
func (s *Store) GetMeta(ctx context.Context, contentID pgtype.UUID) (gen.ServiceListingMetum, error) {
	row, err := s.read.GetServiceListingMetaByContentID(ctx, contentID)
	if err != nil {
		return gen.ServiceListingMetum{}, fmt.Errorf("servicedir: get service_listing_meta: %w", err)
	}

	return row, nil
}

// GetMetaByID retrieves a service_listing_meta row by its own id.
func (s *Store) GetMetaByID(ctx context.Context, id pgtype.UUID) (gen.ServiceListingMetum, error) {
	row, err := s.read.GetServiceListingMetaByID(ctx, id)
	if err != nil {
		return gen.ServiceListingMetum{}, fmt.Errorf("servicedir: get meta by id: %w", err)
	}

	return row, nil
}

// UpdateMeta applies a merged full-row update of the mutable §98/§111
// fields (the service layer overlays the patch onto the existing row
// first — partial-input semantics live there, not in SQL).
func (s *Store) UpdateMeta(ctx context.Context, p gen.UpdateServiceListingMetaParams) (gen.ServiceListingMetum, error) {
	row, err := s.write.UpdateServiceListingMeta(ctx, p)
	if err != nil {
		return gen.ServiceListingMetum{}, fmt.Errorf("servicedir: update service_listing_meta: %w", err)
	}

	return row, nil
}

// ---------------------------------------------------------------------------
// Listing reads
// ---------------------------------------------------------------------------

// ListServicesFilter holds the filter parameters for listing services.
type ListServicesFilter struct {
	DestSpaceID     pgtype.UUID // optional, ?dest=
	CitySpaceID     pgtype.UUID // optional, ?city= (BUG-005)
	ServiceCategory string      // optional, ?category=
	OriginCommunity string      // optional, ?origin= (BUG-005)
	IsVerified      *bool       // optional, ?verified=
	ListingKind     string      // optional, ?kind= (§111)
	PriceMax        *int64      // optional, ?price_max= minor units (§111)
	DateFrom        pgtype.Date // optional, ?date_from= (§111)
	Sort            string      // "top" | "latest" (default: latest)
	Status          string      // filter by status (default: published)
	Q               string      // optional, ?q= — title substring search
	Limit           int32
	Offset          int32
}

// ListServices returns a paginated, filtered list of service listings.
// Sort "top" orders by view_count + comment_count; "latest" orders by
// published_at DESC.
func (s *Store) ListServices(ctx context.Context, f ListServicesFilter) ([]ServiceRow, int64, error) {
	params := gen.ListServiceListingsParams{
		DestSpaceID: f.DestSpaceID, CitySpaceID: f.CitySpaceID,
		PageLimit: f.Limit, PageOffset: f.Offset,
	}
	if f.Status != "" {
		params.Status = &f.Status
	}

	if f.ServiceCategory != "" {
		params.ServiceCategory = &f.ServiceCategory
	}

	if f.OriginCommunity != "" {
		params.OriginCommunity = &f.OriginCommunity
	}

	params.IsVerified = f.IsVerified

	if f.ListingKind != "" {
		params.ListingKind = &f.ListingKind
	}

	params.PriceMax = f.PriceMax
	params.DateFrom = f.DateFrom

	if f.Sort != "" {
		params.Sort = &f.Sort
	}

	if f.Q != "" {
		params.Q = &f.Q
	}

	rows, err := s.read.ListServiceListings(ctx, params)
	if err != nil {
		return nil, 0, fmt.Errorf("servicedir: list services: %w", err)
	}

	total, err := s.read.CountServiceListings(ctx, gen.CountServiceListingsParams{
		Status:          params.Status,
		DestSpaceID:     params.DestSpaceID,
		CitySpaceID:     params.CitySpaceID,
		ServiceCategory: params.ServiceCategory,
		OriginCommunity: params.OriginCommunity,
		IsVerified:      params.IsVerified,
		ListingKind:     params.ListingKind,
		PriceMax:        params.PriceMax,
		DateFrom:        params.DateFrom,
		Q:               params.Q,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("servicedir: count services: %w", err)
	}

	out := make([]ServiceRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowFromList(r))
	}

	return out, total, nil
}

// GetBySlug returns a single service listing by its slug.
func (s *Store) GetBySlug(ctx context.Context, slug string) (ServiceRow, error) {
	row, err := s.read.GetServiceListingBySlug(ctx, slug)
	if err != nil {
		return ServiceRow{}, fmt.Errorf("servicedir: get by slug: %w", err)
	}

	return rowFromSlug(row), nil
}

// GetByContentID returns a single service listing by its content id.
func (s *Store) GetByContentID(ctx context.Context, contentID pgtype.UUID) (ServiceRow, error) {
	row, err := s.read.GetServiceListingByContentID(ctx, contentID)
	if err != nil {
		return ServiceRow{}, fmt.Errorf("servicedir: get by content id: %w", err)
	}

	return rowFromContentID(row), nil
}

// AdminListFilter holds filter parameters for the admin moderation list.
type AdminListFilter struct {
	DestSpaceID     pgtype.UUID // optional, ?dest= (BUG-011)
	Status          string      // optional: filter by content status
	IsVerified      *bool       // optional: filter by verified state
	ClaimStatus     string      // optional: filter by claim status
	ServiceCategory string      // optional: filter by service category
	ListingKind     string      // optional, ?kind= (§111)
	Q               string      // optional: search title
	Limit           int32
	Offset          int32
}

// AdminList returns all service listings (any status) for the moderation
// queue.
func (s *Store) AdminList(ctx context.Context, f AdminListFilter) ([]ServiceRow, int64, error) {
	params := gen.AdminListServiceListingsParams{
		DestSpaceID: f.DestSpaceID, PageLimit: f.Limit, PageOffset: f.Offset,
	}
	if f.Status != "" {
		params.Status = &f.Status
	}

	params.IsVerified = f.IsVerified

	if f.ClaimStatus != "" {
		params.ClaimStatus = &f.ClaimStatus
	}

	if f.ServiceCategory != "" {
		params.ServiceCategory = &f.ServiceCategory
	}

	if f.ListingKind != "" {
		params.ListingKind = &f.ListingKind
	}

	if f.Q != "" {
		params.Q = &f.Q
	}

	rows, err := s.read.AdminListServiceListings(ctx, params)
	if err != nil {
		return nil, 0, fmt.Errorf("servicedir: admin list services: %w", err)
	}

	total, err := s.read.AdminCountServiceListings(ctx, gen.AdminCountServiceListingsParams{
		Status:          params.Status,
		DestSpaceID:     params.DestSpaceID,
		IsVerified:      params.IsVerified,
		ClaimStatus:     params.ClaimStatus,
		ServiceCategory: params.ServiceCategory,
		ListingKind:     params.ListingKind,
		Q:               params.Q,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("servicedir: admin count services: %w", err)
	}

	out := make([]ServiceRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowFromAdmin(r))
	}

	return out, total, nil
}

// ListByOrg returns an org's listings filtered by status (empty = all)
// and listing kind (§111: GET /org/:org_id/packages passes 'published').
func (s *Store) ListByOrg(ctx context.Context, orgID pgtype.UUID, status, listingKind string, limit, offset int32) ([]ServiceRow, error) {
	var statusPtr *string
	if status != "" {
		statusPtr = &status
	}

	params := gen.ListServiceListingsByOrgParams{
		Status: statusPtr,
		OrgID:  orgID, PageLimit: limit, PageOffset: offset,
	}
	if listingKind != "" {
		params.ListingKind = &listingKind
	}

	rows, err := s.read.ListServiceListingsByOrg(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("servicedir: list by org: %w", err)
	}

	out := make([]ServiceRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowFromOrg(r))
	}

	return out, nil
}

// ---------------------------------------------------------------------------
// Verification + claim workflow (§98)
// ---------------------------------------------------------------------------

// VerifyService marks a service listing as verified by an admin, recording
// the verification type and the admin user_id + timestamp for audit
// (ADR-088 HITL, BUG-015).
func (s *Store) VerifyService(ctx context.Context, contentID, verifiedBy pgtype.UUID, verificationType string) error {
	n, err := s.write.SetServiceListingVerified(ctx, gen.SetServiceListingVerifiedParams{
		ContentID:        contentID,
		VerifiedBy:       verifiedBy,
		VerificationType: &verificationType,
	})
	if err != nil {
		return fmt.Errorf("servicedir: verify service: %w", err)
	}

	if n == 0 {
		return fmt.Errorf("servicedir: verify service: %w", errNotFound)
	}

	return nil
}

// UnverifyService marks a service listing as unverified by an admin.
func (s *Store) UnverifyService(ctx context.Context, contentID pgtype.UUID) error {
	n, err := s.write.ClearServiceListingVerified(ctx, contentID)
	if err != nil {
		return fmt.Errorf("servicedir: unverify service: %w", err)
	}

	if n == 0 {
		return fmt.Errorf("servicedir: unverify service: %w", errNotFound)
	}

	return nil
}

// IsOrgMember checks whether a user is a member of the given organization
// (BUG-006).
func (s *Store) IsOrgMember(ctx context.Context, orgID, userID pgtype.UUID) (bool, error) {
	isMember, err := s.read.IsServiceOrgMember(ctx, gen.IsServiceOrgMemberParams{
		OrgID: orgID, UserID: userID,
	})
	if err != nil {
		return false, fmt.Errorf("servicedir: is org member: %w", err)
	}

	return isMember, nil
}

// ClaimIfUnclaimed atomically initiates a claim only if the listing is
// currently unclaimed (BUG-007). Returns errNotFound if the listing does
// not exist or is already claimed.
func (s *Store) ClaimIfUnclaimed(ctx context.Context, contentID, orgID pgtype.UUID) error {
	n, err := s.write.ClaimServiceListingIfUnclaimed(ctx, gen.ClaimServiceListingIfUnclaimedParams{
		ContentID: contentID, OrgID: orgID,
	})
	if err != nil {
		return fmt.Errorf("servicedir: claim if unclaimed: %w", err)
	}

	if n == 0 {
		return fmt.Errorf("servicedir: claim if unclaimed: %w", errNotFound)
	}

	return nil
}

// ResolveClaimIfPending atomically resolves a claim only if it is
// currently pending (BUG-015). Returns errNotFound if the listing does
// not exist or the claim is not pending.
func (s *Store) ResolveClaimIfPending(ctx context.Context, contentID, orgID pgtype.UUID, approve bool) error {
	var (
		n   int64
		err error
	)

	if approve {
		n, err = s.write.ApproveServiceClaimIfPending(ctx, gen.ApproveServiceClaimIfPendingParams{
			ContentID: contentID, OrgID: orgID,
		})
	} else {
		n, err = s.write.RejectServiceClaimIfPending(ctx, contentID)
	}

	if err != nil {
		return fmt.Errorf("servicedir: resolve claim if pending: %w", err)
	}

	if n == 0 {
		return fmt.Errorf("servicedir: resolve claim if pending: %w", errNotFound)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Org helpers
// ---------------------------------------------------------------------------

// OrgVerification holds the org trust fields checked by the §111
// package-creation gate.
type OrgVerification struct {
	Verified         bool
	VerificationType *string
}

// GetOrgVerification returns the org's verified flag + verification_type.
func (s *Store) GetOrgVerification(ctx context.Context, orgID pgtype.UUID) (OrgVerification, error) {
	row, err := s.read.GetServiceOrgVerification(ctx, orgID)
	if err != nil {
		return OrgVerification{}, fmt.Errorf("servicedir: get org verification: %w", err)
	}

	return OrgVerification{Verified: row.Verified, VerificationType: row.VerificationType}, nil
}

// GetListingOrgID returns the owning org_id and listing_kind of a service
// listing — used to validate that a package item's component_content_id
// links to a same-org service listing, not another package (§111).
func (s *Store) GetListingOrgID(ctx context.Context, contentID pgtype.UUID) (pgtype.UUID, string, error) {
	row, err := s.read.GetServiceListingOrgIDByContentID(ctx, contentID)
	if err != nil {
		return pgtype.UUID{}, "", fmt.Errorf("servicedir: get listing org id: %w", err)
	}

	return row.OrgID, row.ListingKind, nil
}

// ---------------------------------------------------------------------------
// service_categories — dynamic taxonomy (marketplace upgrade)
// ---------------------------------------------------------------------------

// listCategories returns service_categories rows in position order.
// activeOnly filters to is_active rows in Go — the single
// ListServiceCategories query serves both the public picker (active
// only) and the admin category manager (full set).
func (s *Store) listCategories(ctx context.Context, activeOnly bool) ([]gen.ServiceCategory, error) {
	rows, err := s.read.ListServiceCategories(ctx)
	if err != nil {
		return nil, fmt.Errorf("servicedir: list categories: %w", err)
	}

	if !activeOnly {
		return rows, nil
	}

	out := make([]gen.ServiceCategory, 0, len(rows))
	for _, r := range rows {
		if r.IsActive {
			out = append(out, r)
		}
	}

	return out, nil
}

// getCategoryBySlug returns a single service_categories row by slug.
func (s *Store) getCategoryBySlug(ctx context.Context, slug string) (gen.ServiceCategory, error) {
	row, err := s.read.GetServiceCategoryBySlug(ctx, slug)
	if err != nil {
		return gen.ServiceCategory{}, fmt.Errorf("servicedir: get category by slug: %w", err)
	}

	return row, nil
}

// createCategory inserts a service_categories row. A slug collision
// surfaces as a wrapped SQLSTATE 23505 — callers map it to
// ErrCategoryExists.
func (s *Store) createCategory(ctx context.Context, p gen.CreateServiceCategoryParams) (gen.ServiceCategory, error) {
	row, err := s.write.CreateServiceCategory(ctx, p)
	if err != nil {
		return gen.ServiceCategory{}, fmt.Errorf("servicedir: create category: %w", err)
	}

	return row, nil
}

// updateCategory applies the COALESCE patch to a service_categories row.
// No matching slug yields a wrapped pgx.ErrNoRows → 404 upstream.
func (s *Store) updateCategory(ctx context.Context, p gen.UpdateServiceCategoryParams) (gen.ServiceCategory, error) {
	row, err := s.write.UpdateServiceCategory(ctx, p)
	if err != nil {
		return gen.ServiceCategory{}, fmt.Errorf("servicedir: update category: %w", err)
	}

	return row, nil
}

// deleteCategory hard-deletes a service_categories row. Only reached for
// slugs with zero listing references — in-use slugs are soft-disabled
// (is_active=false) by the service layer instead.
func (s *Store) deleteCategory(ctx context.Context, slug string) error {
	if err := s.write.DeleteServiceCategory(ctx, slug); err != nil {
		return fmt.Errorf("servicedir: delete category: %w", err)
	}

	return nil
}

// countListingsByCategory returns how many service_listing_meta rows
// reference the slug. The query is verbatim — it does not join
// contents.deleted_at, so soft-deleted listings inflate the count; that
// overcount is accepted per the marketplace plan (it only makes the
// delete path prefer soft-disable).
func (s *Store) countListingsByCategory(ctx context.Context, slug string) (int64, error) {
	n, err := s.read.CountServiceListingsByCategory(ctx, slug)
	if err != nil {
		return 0, fmt.Errorf("servicedir: count listings by category: %w", err)
	}

	return n, nil
}

// ---------------------------------------------------------------------------
// service_listing_coupons / coupon_redemptions (marketplace — attach +
// redeem; servicedir owns these tables so booking+redemption commit in one
// transaction)
// ---------------------------------------------------------------------------

// listListingCoupons returns the coupon join rows for a listing, ordered
// by link position. activeOnly narrows to currently-redeemable coupons
// (status='active' inside the validity window) for the public payload.
func (s *Store) listListingCoupons(ctx context.Context, metaID pgtype.UUID, activeOnly bool) ([]couponLinkRow, error) {
	if activeOnly {
		rows, err := s.read.ListActiveServiceListingCoupons(ctx, metaID)
		if err != nil {
			return nil, fmt.Errorf("servicedir: list active listing coupons: %w", err)
		}

		out := make([]couponLinkRow, 0, len(rows))
		for _, r := range rows {
			out = append(out, linkRowFromActive(r))
		}

		return out, nil
	}

	rows, err := s.read.ListServiceListingCoupons(ctx, metaID)
	if err != nil {
		return nil, fmt.Errorf("servicedir: list listing coupons: %w", err)
	}

	out := make([]couponLinkRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, linkRowFromAll(r))
	}

	return out, nil
}

// getListingCouponByCode resolves an attached coupon by (case-insensitive)
// code scoped to the listing — the redemption lookup. Returns the raw join
// row unfiltered by status/window; the pricing path enforces the rule set
// so rejections can carry a specific sentinel.
func (s *Store) getListingCouponByCode(ctx context.Context, metaID pgtype.UUID, code string) (couponLinkRow, error) {
	row, err := s.read.GetListingCouponByCode(ctx, gen.GetListingCouponByCodeParams{
		ListingMetaID: metaID, Code: code,
	})
	if err != nil {
		return couponLinkRow{}, fmt.Errorf("servicedir: get listing coupon by code: %w", err)
	}

	return linkRowFromCode(row), nil
}

// getCouponByIDForOrg loads a coupon scoped to its owning organization —
// the attach validation lookup: a miss means the coupon does not exist or
// belongs to a different org (both surface as ErrCouponNotApplicable
// upstream).
func (s *Store) getCouponByIDForOrg(ctx context.Context, couponID, orgID pgtype.UUID) (gen.Coupon, error) {
	row, err := s.read.GetCouponByIDForOrg(ctx, gen.GetCouponByIDForOrgParams{
		ID: couponID, OrganizationID: orgID,
	})
	if err != nil {
		return gen.Coupon{}, fmt.Errorf("servicedir: get coupon for org: %w", err)
	}

	return row, nil
}

// setListingCoupons replaces the listing's coupon links inside one
// transaction — delete-all then re-insert in slice order (position follows
// the caller's ordering). Replace-on-save matches the §111
// items/departures convention.
func (s *Store) setListingCoupons(ctx context.Context, metaID pgtype.UUID, couponIDs []pgtype.UUID) error {
	return db.WithTx(ctx, s.pool.Writer, func(q *gen.Queries) error {
		if err := q.DeleteServiceListingCoupons(ctx, metaID); err != nil {
			return fmt.Errorf("servicedir: delete listing coupons: %w", err)
		}

		for i, id := range couponIDs {
			if _, err := q.CreateServiceListingCouponLink(ctx, gen.CreateServiceListingCouponLinkParams{
				CouponID:      id,
				ListingMetaID: metaID,
				Position:      int32(i), //nolint:gosec // bounded by max_per_listing check
			}); err != nil {
				return fmt.Errorf("servicedir: link listing coupon %d: %w", i, err)
			}
		}

		return nil
	})
}

// countCouponRedemptions returns live redemptions for a coupon — bookings
// that died (cancelled/declined/refunded) do not burn the cap.
func (s *Store) countCouponRedemptions(ctx context.Context, couponID pgtype.UUID) (int64, error) {
	n, err := s.read.CountCouponRedemptions(ctx, couponID)
	if err != nil {
		return 0, fmt.Errorf("servicedir: count coupon redemptions: %w", err)
	}

	return n, nil
}

// countCouponRedemptionsByUser returns a user's live redemptions of a
// coupon — the per_user_limit check.
func (s *Store) countCouponRedemptionsByUser(ctx context.Context, couponID, userID pgtype.UUID) (int64, error) {
	n, err := s.read.CountCouponRedemptionsByUser(ctx, gen.CountCouponRedemptionsByUserParams{
		CouponID: couponID, UserID: userID,
	})
	if err != nil {
		return 0, fmt.Errorf("servicedir: count coupon redemptions by user: %w", err)
	}

	return n, nil
}

// createCouponRedemption — the standalone redemption insert lives inside
// Store.createBookingWithRedemption (store_packages.go): redemptions are
// only ever written atomically with their booking, so there is no
// non-transactional wrapper.
