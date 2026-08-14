package affiliate

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

type Enqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// SettingsProvider abstracts system_settings access for affiliate config
// (commission rates, link limits, min account age, attribution window,
// feature flag). *config.Settings satisfies this structurally. When nil, the
// service falls back to compiled-in defaults (Rule 3 / SETTING-002/003).
type SettingsProvider interface {
	GetIntOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback int) int
	GetFloatOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback float64) float64
	// SetSetting upserts a setting (system_settings is the single source of
	// truth for affiliate.commission.* per Section 56).
	SetSetting(ctx context.Context, tenantID pgtype.UUID, key, value string) error
	// GetSettingBool reads a boolean setting (feature.affiliate_program flag).
	GetSettingBool(ctx context.Context, tenantID pgtype.UUID, key string) (bool, error)
}

type Service struct {
	store    *Store
	db       *db.DB
	enqueuer Enqueuer
	settings SettingsProvider
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

// SetSettings wires the hot-reloadable settings reader for affiliate config
// (SETTING-002/003). When wired, commission rates and link limits are read
// from system_settings; otherwise compiled-in defaults are used.
func (s *Service) SetSettings(sp SettingsProvider) { s.settings = sp }

type noopEnqueuer struct{}

func (n *noopEnqueuer) Enqueue(_ string, _ []byte, _ ...interface{}) error { return nil }

// ── Program Info ────────────────────────────────────────────────────

type ProgramInfo struct {
	CommissionRates    CommissionRates `json:"commission_rates"`
	TermsAndConditions string          `json:"terms_and_conditions"`
	MinAccountDays     int             `json:"min_account_days"`
	MaxLinksPerUser    int             `json:"max_links_per_user"`
}

type CommissionRates struct {
	ProMonthly float64 `json:"pro_monthly"`
	ProAnnual  float64 `json:"pro_annual"`
	Course     float64 `json:"course"`
}

func (s *Service) GetProgramInfo(ctx context.Context) *ProgramInfo {
	// Defaults match the system_settings catalog (SETTING-003).
	proMonthly := 15.0
	proAnnual := 20.0
	course := 10.0
	minAccountDays := 30
	maxLinksPerUser := 10
	if s.settings != nil {
		// tenantID is zero-valued — affiliate settings are global (tenant_id
		// is required by the interface but affiliate config is not per-tenant).
		proMonthly = s.settings.GetFloatOr(ctx, pgtype.UUID{}, "affiliate.commission.pro_monthly", 15)
		proAnnual = s.settings.GetFloatOr(ctx, pgtype.UUID{}, "affiliate.commission.pro_annual", 20)
		course = s.settings.GetFloatOr(ctx, pgtype.UUID{}, "affiliate.commission.course", 10)
		minAccountDays = s.settings.GetIntOr(ctx, pgtype.UUID{}, "affiliate.min_account_days", 30)
		maxLinksPerUser = s.settings.GetIntOr(ctx, pgtype.UUID{}, "affiliate.max_links_per_user", 10)
	}
	return &ProgramInfo{
		CommissionRates:    CommissionRates{ProMonthly: proMonthly, ProAnnual: proAnnual, Course: course},
		TermsAndConditions: "By enrolling, you agree to our affiliate program terms.",
		MinAccountDays:     minAccountDays,
		MaxLinksPerUser:    maxLinksPerUser,
	}
}

// AttributionWindowDays returns the click attribution window in days from
// system_settings (affiliate.attribution_window_days, default 30). Used by the
// redirect handler to TTL the jc:aff_click attribution key (Section 56).
func (s *Service) AttributionWindowDays(ctx context.Context) int {
	if s.settings != nil {
		return s.settings.GetIntOr(ctx, pgtype.UUID{}, "affiliate.attribution_window_days", 30)
	}
	return 30
}

// MinAccountDays returns the minimum account age (days) required to enroll,
// from system_settings affiliate.min_account_days (default 30).
func (s *Service) MinAccountDays(ctx context.Context) int {
	if s.settings != nil {
		return s.settings.GetIntOr(ctx, pgtype.UUID{}, "affiliate.min_account_days", 30)
	}
	return 30
}

// lookupUsername returns the username for a user id (tenant-scoped), best
// effort: returns nil when the user is gone (anonymized) so the admin UI can
// fall back to the id.
func (s *Service) lookupUsername(ctx context.Context, userID, tenantID pgtype.UUID) (*string, error) {
	return s.store.lookupUsername(ctx, userID, tenantID)
}

// FeatureEnabled reports whether the affiliate program feature flag is on.
// Reads feature.affiliate_program from system_settings (default false in V1,
// per Section 56). When settings are unwired, the program is treated as
// enabled to preserve existing behavior for deployments that opt in by
// configuring the flag.
func (s *Service) FeatureEnabled(ctx context.Context) bool {
	if s.settings == nil {
		return true
	}
	v, err := s.settings.GetSettingBool(ctx, pgtype.UUID{}, "feature.affiliate_program")
	if err != nil {
		return false
	}
	return v
}

// ── Member Operations ───────────────────────────────────────────────

type EnrollInput struct {
	TenantID pgtype.UUID
	UserID   pgtype.UUID
}

func (s *Service) Enroll(ctx context.Context, input EnrollInput) (gen.AffiliateProgramMember, error) {
	// Feature flag gate (Section 56): affiliate program is opt-in.
	if !s.FeatureEnabled(ctx) {
		return gen.AffiliateProgramMember{}, ErrFeatureDisabled
	}
	existing, err := s.store.getMember(ctx, input.UserID, input.TenantID)
	if err == nil && existing.UserID.Valid {
		return gen.AffiliateProgramMember{}, ErrAlreadyEnrolled
	}
	// Minimum account age eligibility (Section 56): account age must be ≥
	// affiliate.min_account_days (default 30).
	createdAt, cerr := s.store.getUserCreatedAt(ctx, input.UserID, input.TenantID)
	if cerr != nil {
		return gen.AffiliateProgramMember{}, fmt.Errorf("checking account age: %w", cerr)
	}
	if createdAt.Valid {
		minDays := s.MinAccountDays(ctx)
		if time.Since(createdAt.Time) < time.Duration(minDays)*24*time.Hour {
			return gen.AffiliateProgramMember{}, ErrAccountTooNew
		}
	}
	return s.store.createMember(ctx, gen.CreateAffiliateMemberParams{
		UserID: input.UserID, TenantID: input.TenantID, Status: string(MemberStatusActive),
	})
}

func (s *Service) GetMember(ctx context.Context, userID, tenantID pgtype.UUID) (gen.AffiliateProgramMember, error) {
	m, err := s.store.getMember(ctx, userID, tenantID)
	if err != nil {
		if isNoRows(err) {
			return gen.AffiliateProgramMember{}, ErrMemberNotFound
		}
		return gen.AffiliateProgramMember{}, fmt.Errorf("getting member: %w", err)
	}
	return m, nil
}

func (s *Service) ListMembers(ctx context.Context, tenantID pgtype.UUID, status *string, page, pageSize int32) ([]gen.AffiliateProgramMember, error) {
	limit, offset := paginate(page, pageSize)
	st := ""
	if status != nil {
		st = *status
	}
	return s.store.listMembers(ctx, tenantID, st, limit, offset)
}

type UpdateMemberStatusInput struct {
	Status string  `json:"status"`
	Notes  *string `json:"notes,omitempty"`
}

func (s *Service) UpdateMemberStatus(ctx context.Context, userID, tenantID pgtype.UUID, input UpdateMemberStatusInput) (gen.AffiliateProgramMember, error) {
	member, err := s.store.updateMemberStatus(ctx, userID, tenantID, input.Status, input.Notes)
	if err != nil {
		return gen.AffiliateProgramMember{}, fmt.Errorf("updating member status: %w", err)
	}
	if input.Status == string(MemberStatusSuspended) || input.Status == string(MemberStatusBanned) {
		if err := s.store.deactivateLinksByMemberStatus(ctx, userID); err != nil {
			slog.Error("failed to deactivate links for suspended member", "user_id", userID, "error", err)
		}
	}
	return member, nil
}

// ── Link Operations ─────────────────────────────────────────────────

type CreateLinkInput struct {
	ProductType    string       `json:"product_type"`
	ProductID      *pgtype.UUID `json:"product_id,omitempty"`
	Description    *string      `json:"description,omitempty"`
	DestinationURL string       `json:"destination_url"`
}

func (s *Service) CreateLink(ctx context.Context, tenantID, userID pgtype.UUID, input CreateLinkInput) (gen.UserAffiliateLink, error) {
	member, err := s.store.getMember(ctx, userID, tenantID)
	if err != nil || !member.UserID.Valid {
		return gen.UserAffiliateLink{}, ErrNotEnrolled
	}
	if member.Status != string(MemberStatusActive) {
		return gen.UserAffiliateLink{}, ErrMemberSuspended
	}
	count, err := s.store.countActiveLinksByUser(ctx, userID)
	if err != nil {
		return gen.UserAffiliateLink{}, fmt.Errorf("counting links: %w", err)
	}
	// Max links per user is configurable via system_settings
	// (affiliate.max_links_per_user, default 10) — SETTING-002.
	maxLinks := 10
	if s.settings != nil {
		maxLinks = s.settings.GetIntOr(ctx, tenantID, "affiliate.max_links_per_user", 10)
	}
	if count >= int64(maxLinks) {
		return gen.UserAffiliateLink{}, ErrTooManyLinks
	}
	validType := false
	for _, vt := range ValidProductTypes() {
		if input.ProductType == vt {
			validType = true
			break
		}
	}
	if !validType {
		return gen.UserAffiliateLink{}, ErrInvalidProductType
	}
	code, err := generateUniqueCode()
	if err != nil {
		return gen.UserAffiliateLink{}, fmt.Errorf("generating code: %w", err)
	}
	return s.store.createLink(ctx, gen.CreateAffiliateLinkParams{
		TenantID: tenantID, UserID: userID, ProductType: input.ProductType,
		ProductID: toNullableUUID(input.ProductID), Code: code,
		Description: input.Description, DestinationUrl: input.DestinationURL,
	})
}

func (s *Service) GetLink(ctx context.Context, id, tenantID pgtype.UUID) (gen.UserAffiliateLink, error) {
	l, err := s.store.getLinkByID(ctx, id, tenantID)
	if err != nil {
		if isNoRows(err) {
			return gen.UserAffiliateLink{}, ErrLinkNotFound
		}
		return gen.UserAffiliateLink{}, fmt.Errorf("getting link: %w", err)
	}
	return l, nil
}

func (s *Service) GetLinkByCode(ctx context.Context, code string) (gen.UserAffiliateLink, error) {
	l, err := s.store.getLinkByCode(ctx, code)
	if err != nil {
		if isNoRows(err) {
			return gen.UserAffiliateLink{}, ErrLinkNotFound
		}
		return gen.UserAffiliateLink{}, fmt.Errorf("getting link by code: %w", err)
	}
	if !l.IsActive {
		return gen.UserAffiliateLink{}, ErrLinkNotActive
	}
	return l, nil
}

func (s *Service) ListUserLinks(ctx context.Context, userID, tenantID pgtype.UUID, page, pageSize int32) ([]gen.UserAffiliateLink, error) {
	limit, offset := paginate(page, pageSize)
	return s.store.listLinksByUser(ctx, userID, tenantID, true, limit, offset)
}

func (s *Service) ListAllLinks(ctx context.Context, tenantID pgtype.UUID, page, pageSize int32) ([]gen.UserAffiliateLink, error) {
	limit, offset := paginate(page, pageSize)
	return s.store.listAllLinks(ctx, tenantID, limit, offset)
}

type UpdateLinkInput struct {
	Description *string `json:"description,omitempty"`
	IsActive    bool    `json:"is_active"`
}

func (s *Service) UpdateLink(ctx context.Context, id, userID, tenantID pgtype.UUID, input UpdateLinkInput) (gen.UserAffiliateLink, error) {
	l, err := s.store.getLinkByID(ctx, id, tenantID)
	if err != nil {
		if isNoRows(err) {
			return gen.UserAffiliateLink{}, ErrLinkNotFound
		}
		return gen.UserAffiliateLink{}, fmt.Errorf("checking link: %w", err)
	}
	if l.UserID != userID {
		return gen.UserAffiliateLink{}, ErrLinkNotFound
	}
	return s.store.updateLink(ctx, id, userID, tenantID, input.Description, input.IsActive)
}

func (s *Service) DeleteLink(ctx context.Context, id, userID, tenantID pgtype.UUID) error {
	l, err := s.store.getLinkByID(ctx, id, tenantID)
	if err != nil {
		if isNoRows(err) {
			return ErrLinkNotFound
		}
		return fmt.Errorf("checking link: %w", err)
	}
	if l.UserID != userID {
		return ErrLinkNotFound
	}
	return s.store.deactivateLink(ctx, id, userID, tenantID)
}

// ── Click Recording ─────────────────────────────────────────────────

type RecordClickInput struct {
	LinkID      pgtype.UUID
	IPHash      string
	UAHash      string
	ReferrerURL *string
	TenantID    pgtype.UUID
}

func (s *Service) RecordClick(ctx context.Context, input RecordClickInput) error {
	ipHash := input.IPHash
	uaHash := input.UAHash
	_, err := s.store.createEvent(ctx, gen.CreateAffiliateLinkEventParams{
		TenantID: input.TenantID, LinkID: input.LinkID, EventType: string(EventTypeClick),
		IpHash: &ipHash, UserAgentHash: &uaHash, ReferrerUrl: input.ReferrerURL,
	})
	if err != nil {
		return fmt.Errorf("recording click: %w", err)
	}
	if err := s.store.incrementLinkClicks(ctx, input.LinkID); err != nil {
		slog.Error("failed to increment link clicks", "link_id", input.LinkID, "error", err)
	}
	link, err := s.store.getLinkByID(ctx, input.LinkID, input.TenantID)
	if err != nil {
		slog.Error("failed to load link for member click increment", "link_id", input.LinkID, "error", err)
	} else if err := s.store.incrementMemberClicks(ctx, link.UserID); err != nil {
		slog.Error("failed to increment member clicks", "user_id", link.UserID, "error", err)
	}
	return nil
}

// ── Earnings ────────────────────────────────────────────────────────

type EarningsSummary struct {
	TotalClicks      int64               `json:"total_clicks"`
	TotalConversions int64               `json:"total_conversions"`
	TotalEarned      string              `json:"total_earned"`
	ByProduct        []EarningsByProduct `json:"by_product"`
	RecentEvents     []RecentEvent       `json:"recent_events"`
}

type EarningsByProduct struct {
	ProductType string `json:"product_type"`
	Clicks      int64  `json:"clicks"`
	Conversions int64  `json:"conversions"`
	Earned      string `json:"earned"`
}

type RecentEvent struct {
	ID          string  `json:"id"`
	EventType   string  `json:"event_type"`
	Commission  *string `json:"commission_amount,omitempty"`
	CreatedAt   string  `json:"created_at"`
	LinkCode    string  `json:"link_code"`
	ProductType string  `json:"product_type"`
}

func (s *Service) GetEarnings(ctx context.Context, userID, tenantID pgtype.UUID) (*EarningsSummary, error) {
	earnings, err := s.store.getUserEarnings(ctx, userID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("getting earnings: %w", err)
	}
	byProduct, err := s.store.getUserEarningsByProduct(ctx, userID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("getting earnings by product: %w", err)
	}
	recent, err := s.store.getRecentEvents(ctx, userID, tenantID, 20)
	if err != nil {
		return nil, fmt.Errorf("getting recent events: %w", err)
	}

	bp := make([]EarningsByProduct, len(byProduct))
	for i, row := range byProduct {
		bp[i] = EarningsByProduct{
			ProductType: row.ProductType, Clicks: row.Clicks,
			Conversions: row.Conversions, Earned: formatNumeric(row.Earned),
		}
	}
	re := make([]RecentEvent, len(recent))
	for i, row := range recent {
		re[i] = RecentEvent{
			ID: row.ID.String(), EventType: row.EventType,
			Commission: formatNumericPtr(row.CommissionAmount),
			CreatedAt:  row.CreatedAt.Time.Format(time.RFC3339),
			LinkCode:   row.LinkCode, ProductType: row.ProductType,
		}
	}

	totalClicks, _ := earnings.TotalClicks.(int64)
	totalConversions, _ := earnings.TotalConversions.(int64)

	return &EarningsSummary{
		TotalClicks: totalClicks, TotalConversions: totalConversions,
		TotalEarned: formatNumeric(earnings.TotalEarned),
		ByProduct:   bp, RecentEvents: re,
	}, nil
}

// formatNumeric renders a pgtype.Numeric (or any numeric interface{} coming
// back from a SUM/COALESCE aggregate) as a currency string with 2 decimals.
// Returns "0.00" for nil/invalid values so the UI always shows a parseable
// number — never the pgtype.Numeric struct dump that %v would produce.
func formatNumeric(v interface{}) string {
	if v == nil {
		return "0.00"
	}
	if n, ok := v.(pgtype.Numeric); ok {
		if !n.Valid {
			return "0.00"
		}
		f, err := n.Float64Value()
		if err != nil || !f.Valid {
			return "0.00"
		}
		return fmt.Sprintf("%.2f", f.Float64)
	}
	return fmt.Sprintf("%.2f", v)
}

// formatNumericPtr is formatNumeric for nullable columns — returns nil when
// the value is NULL/invalid so the JSON field is omitted (omitempty).
func formatNumericPtr(n pgtype.Numeric) *string {
	if !n.Valid {
		return nil
	}
	f, err := n.Float64Value()
	if err != nil || !f.Valid {
		return nil
	}
	s := fmt.Sprintf("%.2f", f.Float64)
	return &s
}

// numericToFloat converts a pgtype.Numeric to a float64 for JSON serialization
// in admin endpoints where the frontend expects a number (not a formatted
// string). Returns 0 for nil/invalid values (BUG-005).
func numericToFloat(n pgtype.Numeric) float64 {
	if !n.Valid {
		return 0
	}
	f, err := n.Float64Value()
	if err != nil || !f.Valid {
		return 0
	}
	return f.Float64
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

func generateUniqueCode() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "jph-" + hex.EncodeToString(b), nil
}
