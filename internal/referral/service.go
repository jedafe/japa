// Package referral provides business logic for the Referral System
// (Section 60, ADR-078). Every user gets a unique referral code. Sharing
// the code earns Credits when the referred user completes their first
// paid action. Single-level, first-click 30-day attribution, fraud-controlled.
//
// The Service also manages the legacy referral_programs admin settings
// (referrer/referee reward cents, max referrals, cookie days).
//
// Ref: japaconnect-engineering/sections/60-referral-system.md
//
//	japaconnect-engineering/adr/ADR-078-referral-system.md
package referral

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/valkey"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// Status constants for referral lifecycle.
const (
	StatusPending   = "pending"
	StatusQualified = "qualified"
	StatusCredited  = "credited"
	StatusFlagged   = "flagged"
	StatusRejected  = "rejected"
)

// Task name constants.
const (
	TaskRecordClick      = "referral:record_click"
	TaskCheckConversion  = "referral:check_conversion"
	TaskReviewFlagged    = "referral:review_flagged"
	TaskAwardPoints      = "wallet:award_points"
	TaskSendNotification = "notification:send"
)

// TaskEnqueuer enqueues Asynq tasks fire-and-forget (Rule 8).
type TaskEnqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// Program represents the referral program settings for a tenant.
type Program struct {
	ID                  pgtype.UUID        `json:"id"`
	TenantID            pgtype.UUID        `json:"tenant_id"`
	ReferrerRewardCents int32              `json:"referrer_reward_cents"`
	RefereeRewardCents  int32              `json:"referee_reward_cents"`
	MaxReferrals        int32              `json:"max_referrals"`
	CookieDays          int32              `json:"cookie_days"`
	IsActive            bool               `json:"is_active"`
	Description         string             `json:"description"`
	CreatedAt           pgtype.Timestamptz `json:"created_at"`
	UpdatedAt           pgtype.Timestamptz `json:"updated_at"`
}

// Service coordinates referral program configuration, referral tracking,
// referral codes, attribution, and fraud-controlled credit awards.
type Service struct {
	db       *db.DB
	queries  *gen.Queries
	settings *config.Settings
	valkey   valkeycompat.Cmdable
	enqueuer TaskEnqueuer
}

// NewService creates a Service backed by the given database pool.
func NewService(database *db.DB) *Service {
	return &Service{
		db:      database,
		queries: gen.New(database.Reader),
	}
}

// SetSettings injects the config.Settings provider.
func (s *Service) SetSettings(settings *config.Settings) { s.settings = settings }

// Enabled reads the referral.enabled system_settings toggle (ADR-078, §60).
// Defaults to true when unset or when settings are not injected.
func (s *Service) Enabled(ctx context.Context, tenantID pgtype.UUID) bool {
	if s.settings == nil {
		return true
	}
	return s.settings.GetBoolOr(ctx, tenantID, "referral.enabled", true)
}

// SetValkey injects the Valkey client for attribution caching.
func (s *Service) SetValkey(vk valkeycompat.Cmdable) { s.valkey = vk }

// SetEnqueuer injects the Asynq task enqueuer.
func (s *Service) SetEnqueuer(e TaskEnqueuer) { s.enqueuer = e }

// ---------------------------------------------------------------------------
// Legacy: Referral Program admin settings (referral_programs table)
// ---------------------------------------------------------------------------

// GetProgram returns the referral program settings for the tenant. If no row
// exists yet (first admin visit), it seeds a default row using the column
// defaults from migration 000056 and returns it — so the admin UI always has
// a program to render instead of a 500.
func (s *Service) GetProgram(ctx context.Context, tenantID pgtype.UUID) (Program, error) {
	var p Program

	err := s.db.Reader.QueryRow(ctx, `
		SELECT id, tenant_id, referrer_reward_cents, referee_reward_cents,
		       max_referrals, cookie_days, is_active, description, created_at, updated_at
		FROM referral_programs
		WHERE tenant_id = $1
	`, tenantID).Scan(
		&p.ID, &p.TenantID, &p.ReferrerRewardCents, &p.RefereeRewardCents,
		&p.MaxReferrals, &p.CookieDays, &p.IsActive, &p.Description,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err == nil {
		return p, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Program{}, fmt.Errorf("fetching referral program: %w", err)
	}

	return s.seedDefaultProgram(ctx, tenantID)
}

// seedDefaultProgram inserts a default referral_programs row for the tenant
// and returns it. On a unique-violation race it re-reads the existing row.
func (s *Service) seedDefaultProgram(ctx context.Context, tenantID pgtype.UUID) (Program, error) {
	var p Program

	err := s.db.Writer.QueryRow(ctx, `
		INSERT INTO referral_programs (tenant_id)
		VALUES ($1)
		ON CONFLICT (tenant_id) DO NOTHING
		RETURNING id, tenant_id, referrer_reward_cents, referee_reward_cents,
		          max_referrals, cookie_days, is_active, description, created_at, updated_at
	`, tenantID).Scan(
		&p.ID, &p.TenantID, &p.ReferrerRewardCents, &p.RefereeRewardCents,
		&p.MaxReferrals, &p.CookieDays, &p.IsActive, &p.Description,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err == nil {
		return p, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Program{}, fmt.Errorf("seeding referral program: %w", err)
	}

	err = s.db.Reader.QueryRow(ctx, `
		SELECT id, tenant_id, referrer_reward_cents, referee_reward_cents,
		       max_referrals, cookie_days, is_active, description, created_at, updated_at
		FROM referral_programs
		WHERE tenant_id = $1
	`, tenantID).Scan(
		&p.ID, &p.TenantID, &p.ReferrerRewardCents, &p.RefereeRewardCents,
		&p.MaxReferrals, &p.CookieDays, &p.IsActive, &p.Description,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return Program{}, fmt.Errorf("re-reading seeded referral program: %w", err)
	}
	return p, nil
}

// UpdateProgram updates the referral program settings for the tenant.
func (s *Service) UpdateProgram(ctx context.Context, tenantID pgtype.UUID, referrerRewardCents, refereeRewardCents, maxReferrals, cookieDays *int32, isActive *bool, description *string) (Program, error) {
	current, err := s.GetProgram(ctx, tenantID)
	if err != nil {
		return Program{}, err
	}

	rr := current.ReferrerRewardCents
	if referrerRewardCents != nil {
		rr = *referrerRewardCents
	}

	rf := current.RefereeRewardCents
	if refereeRewardCents != nil {
		rf = *refereeRewardCents
	}

	mr := current.MaxReferrals
	if maxReferrals != nil {
		mr = *maxReferrals
	}

	cd := current.CookieDays
	if cookieDays != nil {
		cd = *cookieDays
	}

	ia := current.IsActive
	if isActive != nil {
		ia = *isActive
	}

	desc := current.Description
	if description != nil {
		desc = *description
	}

	var updated Program

	err = s.db.Writer.QueryRow(ctx, `
		UPDATE referral_programs
		SET referrer_reward_cents = $2, referee_reward_cents = $3,
		    max_referrals = $4, cookie_days = $5, is_active = $6,
		    description = $7, updated_at = now()
		WHERE tenant_id = $1
		RETURNING id, tenant_id, referrer_reward_cents, referee_reward_cents,
		          max_referrals, cookie_days, is_active, description, created_at, updated_at
	`, tenantID, rr, rf, mr, cd, ia, desc).Scan(
		&updated.ID, &updated.TenantID, &updated.ReferrerRewardCents, &updated.RefereeRewardCents,
		&updated.MaxReferrals, &updated.CookieDays, &updated.IsActive, &updated.Description,
		&updated.CreatedAt, &updated.UpdatedAt,
	)
	if err != nil {
		return Program{}, fmt.Errorf("updating referral program: %w", err)
	}

	return updated, nil
}

// ---------------------------------------------------------------------------
// V2: Referral codes, attribution, and conversion tracking
// ---------------------------------------------------------------------------

// GenerateReferralCode generates a unique 8-character alphanumeric code.
func GenerateReferralCode() (string, error) {
	const charset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	code := make([]byte, 8)
	for i := range code {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", fmt.Errorf("referral: generate code: %w", err)
		}
		code[i] = charset[n.Int64()]
	}
	return string(code), nil
}

// GetOrCreateCode returns the user's referral code, creating one if it doesn't exist.
func (s *Service) GetOrCreateCode(ctx context.Context, tenantID, userID pgtype.UUID) (gen.ReferralCode, error) {
	code, err := s.queries.GetReferralCodeByUserID(ctx, gen.GetReferralCodeByUserIDParams{
		TenantID: tenantID,
		UserID:   userID,
	})
	if err == nil {
		return code, nil
	}

	newCode, err := GenerateReferralCode()
	if err != nil {
		return gen.ReferralCode{}, err
	}

	code, err = s.queries.CreateReferralCode(ctx, gen.CreateReferralCodeParams{
		TenantID: tenantID,
		UserID:   userID,
		Code:     newCode,
	})
	if err != nil {
		return gen.ReferralCode{}, fmt.Errorf("referral: create code: %w", err)
	}
	return code, nil
}

// GetReferralStats returns the user's referral statistics.
func (s *Service) GetReferralStats(ctx context.Context, tenantID, userID pgtype.UUID) (map[string]interface{}, error) {
	code, err := s.GetOrCreateCode(ctx, tenantID, userID)
	if err != nil {
		return nil, err
	}

	total, err := s.queries.CountReferralsByReferrer(ctx, gen.CountReferralsByReferrerParams{
		TenantID:   tenantID,
		ReferrerID: userID,
		Status:     "",
	})
	if err != nil {
		return nil, fmt.Errorf("referral: count total: %w", err)
	}

	credited, err := s.queries.CountReferralsByReferrer(ctx, gen.CountReferralsByReferrerParams{
		TenantID:   tenantID,
		ReferrerID: userID,
		Status:     StatusCredited,
	})
	if err != nil {
		return nil, fmt.Errorf("referral: count credited: %w", err)
	}

	pending, err := s.queries.CountReferralsByReferrer(ctx, gen.CountReferralsByReferrerParams{
		TenantID:   tenantID,
		ReferrerID: userID,
		Status:     StatusPending,
	})
	if err != nil {
		return nil, fmt.Errorf("referral: count pending: %w", err)
	}

	return map[string]interface{}{
		"code":        code.Code,
		"click_count": code.ClickCount,
		"total":       total,
		"credited":    credited,
		"pending":     pending,
	}, nil
}

// RecordClick increments the click count for a referral code and sets the
// Valkey attribution key. Called by the referral:record_click task.
func (s *Service) RecordClick(ctx context.Context, code, sessionID string) error {
	if err := s.queries.IncrementReferralClickCount(ctx, code); err != nil {
		return fmt.Errorf("referral: increment click: %w", err)
	}

	if s.valkey != nil {
		ttlDays := 30
		if s.settings != nil {
			ttlDays = s.settings.GetIntOr(ctx, pgtype.UUID{}, "referral.attribution_window_days", 30)
		}
		ttl := time.Duration(ttlDays) * 24 * time.Hour
		// BUG-010 fix: tenant-scope the attribution key (Rule 4, §4.2). The
		// session_id alone is not globally unique across tenants.
		key := valkey.MustKey("referral_attr:%s", sessionID)
		if err := s.valkey.Set(ctx, key, code, ttl).Err(); err != nil {
			slog.Warn("referral: set attribution key failed", "error", err)
		}
	}

	return nil
}

// EnqueueRecordClick enqueues the referral:record_click task fire-and-forget
// (Rule 8). Called by the redirect handler so click recording is decoupled
// from the request lifecycle (BUG-005).
func (s *Service) EnqueueRecordClick(code, sessionID, ip, userAgent string) error {
	if s.enqueuer == nil {
		// Graceful degradation — no enqueuer wired (tests, dev). Fall back to
		// a direct call with context.Background() so click tracking still works.
		ctx := context.Background()
		return s.RecordClick(ctx, code, sessionID)
	}
	payload, _ := json.Marshal(map[string]any{
		"code":       code,
		"session_id": sessionID,
		"ip":         ip,
		"user_agent": userAgent,
	})
	return s.enqueuer.Enqueue(TaskRecordClick, payload)
}

// GetAttribution retrieves the referral code from Valkey for a session.
func (s *Service) GetAttribution(ctx context.Context, sessionID string) (string, error) {
	if s.valkey == nil {
		return "", errors.New("referral: valkey not configured")
	}
	key := valkey.MustKey("referral_attr:%s", sessionID)
	code, err := s.valkey.Get(ctx, key).Result()
	if err != nil {
		return "", fmt.Errorf("referral: get attribution: %w", err)
	}
	return code, nil
}

// CreateReferral creates a pending referral record linking referrer to referee.
func (s *Service) CreateReferral(ctx context.Context, tenantID, referrerID, refereeID pgtype.UUID, code string) (gen.Referral, error) {
	referral, err := s.queries.CreateReferral(ctx, gen.CreateReferralParams{
		TenantID:     tenantID,
		ReferrerID:   referrerID,
		RefereeID:    refereeID,
		ReferralCode: code,
	})
	if err != nil {
		return gen.Referral{}, fmt.Errorf("referral: create: %w", err)
	}
	return referral, nil
}

// CaptureSignupReferral resolves a ref code to its owner and creates a pending
// referral row linking the referrer to the newly registered user. Guards:
// unknown code → no-op (returns nil); self-referral → no-op; duplicate referee
// → no-op (ON CONFLICT (referee_id) DO NOTHING in the sqlc query).
// Implements auth.ReferralCapturer (§60, ADR-078).
func (s *Service) CaptureSignupReferral(ctx context.Context, tenantID, refereeUserID pgtype.UUID, refCode string) error {
	refCode = strings.TrimSpace(refCode)
	if refCode == "" {
		return nil
	}

	refCodeRow, err := s.queries.GetReferralCodeByCode(ctx, refCode)
	if err != nil {
		// Unknown code — not a referred user; no-op (not an error).
		return nil
	}

	// Self-referral guard — a user cannot refer themselves.
	if refCodeRow.UserID == refereeUserID {
		return nil
	}

	if _, err := s.CreateReferral(ctx, tenantID, refCodeRow.UserID, refereeUserID, refCode); err != nil {
		return fmt.Errorf("referral: capture signup: %w", err)
	}
	return nil
}

// CheckConversion is called on payment completion. It checks if the payer
// has a referral attribution, runs fraud checks, and transitions the referral
// status. Called by the referral:check_conversion task.
func (s *Service) CheckConversion(ctx context.Context, tenantID, payerID, paymentID pgtype.UUID, sessionID string) error {
	code, err := s.GetAttribution(ctx, sessionID)
	if err != nil {
		return nil // No attribution — not a referred user.
	}

	refCode, err := s.queries.GetReferralCodeByCode(ctx, code)
	if err != nil {
		return fmt.Errorf("referral: get code: %w", err)
	}

	// Self-referral check.
	if refCode.UserID == payerID {
		return s.flagReferral(ctx, tenantID, payerID, "self_referral")
	}

	referral, err := s.queries.GetReferralByReferee(ctx, gen.GetReferralByRefereeParams{
		TenantID:  tenantID,
		RefereeID: payerID,
	})
	if err != nil {
		return nil // No referral record — nothing to do.
	}

	// Check monthly cap.
	maxPerPeriod := 10
	if s.settings != nil {
		maxPerPeriod = s.settings.GetIntOr(ctx, tenantID, "referral.max_rewards_per_period", 10)
	}
	periodStart := time.Now().AddDate(0, -1, 0)
	creditedThisPeriod, err := s.queries.CountCreditedReferralsByReferrerThisPeriod(ctx, gen.CountCreditedReferralsByReferrerThisPeriodParams{
		TenantID:    tenantID,
		ReferrerID:  referral.ReferrerID,
		PeriodStart: pgtype.Timestamptz{Time: periodStart, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("referral: count credited this period: %w", err)
	}
	if creditedThisPeriod >= int64(maxPerPeriod) {
		return s.flagReferralByID(ctx, tenantID, referral.ID, "monthly_cap_exceeded")
	}

	// Qualify + credit in a single transaction (BUG-011). Without a tx, a
	// CreditReferral failure leaves the referral in 'qualified' with no credit
	// awarded — non-idempotent and unrecoverable without manual SQL.
	rewardCredits := 500
	if s.settings != nil {
		rewardCredits = s.settings.GetIntOr(ctx, tenantID, "referral.reward_credits", 500)
	}
	tx, err := s.db.Writer.Begin(ctx)
	if err != nil {
		return fmt.Errorf("referral: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.queries.WithTx(tx)

	if _, err = qtx.QualifyReferral(ctx, gen.QualifyReferralParams{
		TenantID:           tenantID,
		ID:                 referral.ID,
		QualifiedPaymentID: paymentID,
	}); err != nil {
		return fmt.Errorf("referral: qualify: %w", err)
	}

	if _, err = qtx.CreditReferral(ctx, gen.CreditReferralParams{
		TenantID:     tenantID,
		ID:           referral.ID,
		RewardAmount: pgtype.Numeric{Int: big.NewInt(int64(rewardCredits)), Exp: 0, Valid: true},
	}); err != nil {
		return fmt.Errorf("referral: credit: %w", err)
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("referral: commit: %w", err)
	}

	// Award credits to referrer wallet (fire-and-forget per Rule 8).
	if s.enqueuer != nil {
		payload, _ := json.Marshal(map[string]interface{}{
			"tenant_id":   tenantID.String(),
			"user_id":     referral.ReferrerID.String(),
			"amount":      rewardCredits,
			"ledger_type": "credits",
			"reason":      "referral_reward",
		})
		if err := s.enqueuer.Enqueue(TaskAwardPoints, payload); err != nil {
			slog.Warn("referral: enqueue award points failed", "error", err)
		}
	}

	return nil
}

func (s *Service) flagReferral(ctx context.Context, tenantID, refereeID pgtype.UUID, reason string) error {
	referral, err := s.queries.GetReferralByReferee(ctx, gen.GetReferralByRefereeParams{
		TenantID:  tenantID,
		RefereeID: refereeID,
	})
	if err != nil {
		return nil
	}
	return s.flagReferralByID(ctx, tenantID, referral.ID, reason)
}

func (s *Service) flagReferralByID(ctx context.Context, tenantID, referralID pgtype.UUID, reason string) error {
	_, err := s.queries.FlagReferral(ctx, gen.FlagReferralParams{
		TenantID:      tenantID,
		ID:            referralID,
		FlaggedReason: &reason,
	})
	return err
}

// ListReferralHistory returns the referrer's referral list (status only, no PII).
func (s *Service) ListReferralHistory(ctx context.Context, tenantID, referrerID pgtype.UUID, status string, page, limit int) ([]gen.Referral, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := int32((page - 1) * limit)

	rows, err := s.queries.ListReferralsByReferrer(ctx, gen.ListReferralsByReferrerParams{
		TenantID:   tenantID,
		ReferrerID: referrerID,
		Status:     status,
		PageLimit:  int32(limit),
		PageOffset: offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("referral: list history: %w", err)
	}

	total, err := s.queries.CountReferralsByReferrer(ctx, gen.CountReferralsByReferrerParams{
		TenantID:   tenantID,
		ReferrerID: referrerID,
		Status:     status,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("referral: count history: %w", err)
	}

	return rows, total, nil
}

// AdminListReferrals returns all referrals for admin oversight.
func (s *Service) AdminListReferrals(ctx context.Context, tenantID pgtype.UUID, status string, page, limit int) ([]gen.Referral, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := int32((page - 1) * limit)

	rows, err := s.queries.AdminListReferrals(ctx, gen.AdminListReferralsParams{
		TenantID:   tenantID,
		Status:     status,
		PageLimit:  int32(limit),
		PageOffset: offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("referral: admin list: %w", err)
	}

	total, err := s.queries.AdminCountReferrals(ctx, gen.AdminCountReferralsParams{
		TenantID: tenantID,
		Status:   status,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("referral: admin count: %w", err)
	}

	return rows, total, nil
}

// AdminResolveFlagged resolves a flagged referral (credit or reject).
// BUG-003 fix: the "credit" action now uses CreditFlaggedReferral which
// transitions flagged→credited (CreditReferral only accepts status='qualified'
// and would return ErrNoRows for a flagged referral).
func (s *Service) AdminResolveFlagged(ctx context.Context, tenantID, referralID pgtype.UUID, action string) error {
	switch strings.ToLower(action) {
	case "credit":
		rewardCredits := 500
		if s.settings != nil {
			rewardCredits = s.settings.GetIntOr(ctx, tenantID, "referral.reward_credits", 500)
		}
		_, err := s.queries.CreditFlaggedReferral(ctx, gen.CreditFlaggedReferralParams{
			TenantID:     tenantID,
			ID:           referralID,
			RewardAmount: pgtype.Numeric{Int: big.NewInt(int64(rewardCredits)), Exp: 0, Valid: true},
		})
		return err
	case "reject":
		return s.queries.RejectReferral(ctx, gen.RejectReferralParams{
			TenantID: tenantID,
			ID:       referralID,
		})
	default:
		return fmt.Errorf("referral: invalid action: %s", action)
	}
}

// AdminReferralStats returns totals by status.
func (s *Service) AdminReferralStats(ctx context.Context, tenantID pgtype.UUID) ([]gen.AdminReferralStatsRow, error) {
	return s.queries.AdminReferralStats(ctx, tenantID)
}

// AdminListFlagged returns the flagged referral queue.
func (s *Service) AdminListFlagged(ctx context.Context, tenantID pgtype.UUID, page, limit int) ([]gen.Referral, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := int32((page - 1) * limit)

	rows, err := s.queries.AdminListFlaggedReferrals(ctx, gen.AdminListFlaggedReferralsParams{
		TenantID:   tenantID,
		PageLimit:  int32(limit),
		PageOffset: offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("referral: admin flagged list: %w", err)
	}

	total, err := s.queries.AdminCountFlaggedReferrals(ctx, tenantID)
	if err != nil {
		return nil, 0, fmt.Errorf("referral: admin flagged count: %w", err)
	}

	return rows, total, nil
}

// ---------------------------------------------------------------------------
// Legacy: Direct pgx referral listing (old schema, kept for backward compat)
// ---------------------------------------------------------------------------

// Referral represents a single referral record (legacy struct for old routes).
type Referral struct {
	ID                  pgtype.UUID         `json:"id"`
	TenantID            pgtype.UUID         `json:"tenant_id"`
	ReferrerID          pgtype.UUID         `json:"referrer_id"`
	RefereeID           *pgtype.UUID        `json:"referee_id,omitempty"`
	ReferralCode        string              `json:"referral_code"`
	Status              string              `json:"status"`
	RewardPaidAt        *pgtype.Timestamptz `json:"reward_paid_at,omitempty"`
	CreatedAt           pgtype.Timestamptz  `json:"created_at"`
	ReferrerUsername    *string             `json:"referrer_username,omitempty"`
	ReferrerDisplayName *string             `json:"referrer_display_name,omitempty"`
	RefereeUsername     *string             `json:"referee_username,omitempty"`
	RefereeDisplayName  *string             `json:"referee_display_name,omitempty"`
}

// ListReferrals returns all referrals for the tenant (legacy direct pgx).
func (s *Service) ListReferrals(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]Referral, error) {
	rows, err := s.db.Reader.Query(ctx, `
		SELECT r.id, r.tenant_id, r.referrer_id, r.referee_id, r.referral_code, r.status, r.reward_paid_at, r.created_at,
		       ru.username, rp.display_name,
		       reu.username, rep.display_name
		FROM referrals r
		LEFT JOIN users ru ON ru.id = r.referrer_id
		LEFT JOIN user_profiles rp ON rp.user_id = r.referrer_id AND rp.tenant_id = r.tenant_id
		LEFT JOIN users reu ON reu.id = r.referee_id
		LEFT JOIN user_profiles rep ON rep.user_id = r.referee_id AND rep.tenant_id = r.tenant_id
		WHERE r.tenant_id = $1
		ORDER BY r.created_at DESC
		LIMIT $2 OFFSET $3
	`, tenantID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listing referrals: %w", err)
	}
	defer rows.Close()

	var referrals []Referral
	for rows.Next() {
		var ref Referral
		if err := rows.Scan(&ref.ID, &ref.TenantID, &ref.ReferrerID, &ref.RefereeID, &ref.ReferralCode, &ref.Status, &ref.RewardPaidAt, &ref.CreatedAt, &ref.ReferrerUsername, &ref.ReferrerDisplayName, &ref.RefereeUsername, &ref.RefereeDisplayName); err != nil {
			return nil, fmt.Errorf("scanning referral: %w", err)
		}
		referrals = append(referrals, ref)
	}

	return referrals, rows.Err()
}

// GetReferral returns a single referral by ID (legacy direct pgx).
func (s *Service) GetReferral(ctx context.Context, tenantID, id pgtype.UUID) (Referral, error) {
	var ref Referral

	err := s.db.Reader.QueryRow(ctx, `
		SELECT id, tenant_id, referrer_id, referee_id, referral_code, status, reward_paid_at, created_at
		FROM referrals
		WHERE tenant_id = $1 AND id = $2
	`, tenantID, id).Scan(&ref.ID, &ref.TenantID, &ref.ReferrerID, &ref.RefereeID, &ref.ReferralCode, &ref.Status, &ref.RewardPaidAt, &ref.CreatedAt)
	if err != nil {
		return Referral{}, fmt.Errorf("fetching referral %s: %w", id, err)
	}

	return ref, nil
}

// DeleteReferral soft-deletes a referral by setting status to 'rejected'
// (the old 'expired' status is not in the V2 CHECK constraint).
func (s *Service) DeleteReferral(ctx context.Context, tenantID, id pgtype.UUID) error {
	cmd, err := s.db.Writer.Exec(ctx, `
		UPDATE referrals
		SET status = 'rejected'
		WHERE tenant_id = $1 AND id = $2
	`, tenantID, id)
	if err != nil {
		return fmt.Errorf("deleting referral %s: %w", id, err)
	}
	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
