// Package remittance — service.go holds the business logic for the Remittance
// Rate Tracker: latest-rate resolution with Valkey caching, the send-amount
// calculator, historical chart data, rate-alert CRUD, and admin operations.
// It reads remittance.* system_settings via the hot-reloadable config.Settings.
//
// Ref: japaconnect-engineering/sections/74-remittance-tracker.md
//      japaconnect-engineering/adr/ADR-075-remittance-rate-tracker.md
package remittance

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/valkey-io/valkey-go/valkeycompat"

	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/crypto"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/valkey"
	"github.com/jedafe/japa/internal/worker"
)

// Direction constants for rate alerts.
const (
	DirectionAbove = "above"
	DirectionBelow = "below"
)

// DataSource constants for remittance providers.
const (
	DataSourceAPI    = "api"
	DataSourceManual = "manual"
)

// Valkey cache TTL for corridor rate caches (ADR-075: 15 min).
const rateCacheTTL = 15 * time.Minute

// TaskEnqueuer is the minimal interface for enqueueing Asynq tasks.
type TaskEnqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// RateSnapshot is a single rate quote returned by a RateProvider for a
// corridor. The provider adapter normalises the provider-specific response
// into this shape so the service can insert it without knowing the provider
// API details (ADR-075 §2).
type RateSnapshot struct {
	SendCurrency    string
	ReceiveCurrency string
	Rate            float64
	FeeFlat         float64
	FeePct          float64
	TransferTime    string
	SourceURL       string
}

// RateProvider is the pluggable interface for live rate-fetching adapters
// (ADR-075 §2). Each API-sourced provider registers an implementation that
// knows how to call the provider's API, normalise the response, and return
// one or more RateSnapshots. The manual data_source returns nil (no live
// fetch — rates are entered by admins).
type RateProvider interface {
	// FetchRates calls the provider's API and returns rate snapshots for the
	// given corridors. If corridors is empty/nil, the provider should fetch
	// all configured corridors. apiConfig is the decrypted provider config.
	FetchRates(ctx context.Context, apiConfig []byte, corridors []Corridor) ([]RateSnapshot, error)
}

// Notifier is the minimal interface for dispatching rate-alert notifications.
// The notifications.Service satisfies this. Used by CheckAndDispatchAlerts.
type Notifier interface {
	CreateNotification(ctx context.Context, tenantID, recipientID pgtype.UUID, nType, actorType string, actorID pgtype.UUID, subjectType string, subjectID pgtype.UUID, title, body, actionURL string) error
}

// Service holds the business logic for the Remittance Rate Tracker.
type Service struct {
	store     *Store
	settings  *config.Settings
	cache     valkeycompat.Cmdable // nil = caching disabled
	enqueuer  TaskEnqueuer
	providers map[string]RateProvider // keyed by provider slug
	notifier  Notifier
}

// NewService creates a remittance service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{
		store:    store,
		settings: settings,
	}
}

// SetCache injects the Valkey client for rate caching.
func (s *Service) SetCache(c valkeycompat.Cmdable) { s.cache = c }

// SetEnqueuer injects the Asynq task enqueuer for fetch/check tasks.
func (s *Service) SetEnqueuer(e TaskEnqueuer) { s.enqueuer = e }

// SetProviders injects the rate-provider adapters keyed by provider slug.
// A nil map is acceptable — API-sourced providers will fail with a clear
// "no provider adapter registered" error until one is configured.
func (s *Service) SetProviders(p map[string]RateProvider) { s.providers = p }

// SetNotifier injects the notification service for rate-alert dispatch.
func (s *Service) SetNotifier(n Notifier) { s.notifier = n }

// ---------------------------------------------------------------------------
// DTOs
// ---------------------------------------------------------------------------

// RateInfo is the JSON-serialisable representation of a single provider's
// latest rate for a corridor.
type RateInfo struct {
	ID              string  `json:"id"`
	ProviderID      string  `json:"provider_id"`
	ProviderName    string  `json:"provider_name"`
	ProviderSlug    string  `json:"provider_slug"`
	ProviderWebsite string  `json:"provider_website,omitempty"`
	SendCurrency    string  `json:"send_currency"`
	ReceiveCurrency string  `json:"receive_currency"`
	Rate            float64 `json:"rate"`
	FeeFlat         float64 `json:"fee_flat,omitempty"`
	FeePct          float64 `json:"fee_pct,omitempty"`
	TransferTime    string  `json:"transfer_time,omitempty"`
	FetchedAt       string  `json:"fetched_at"`
	IsManual        bool    `json:"is_manual"`
	SourceURL       string  `json:"source_url,omitempty"`
}

// BestRate is the JSON-serialisable representation of the best rate for a
// corridor — the provider with the highest effective receive amount.
type BestRate struct {
	ProviderID   string  `json:"provider_id"`
	ProviderName string  `json:"provider_name"`
	Rate         float64 `json:"rate"`
	ReceiveAmount float64 `json:"receive_amount"`
}

// HistoryPoint is a single day in the rate history time-series.
type HistoryPoint struct {
	Day     string  `json:"day"`
	MaxRate float64 `json:"max_rate"`
	MinRate float64 `json:"min_rate"`
	AvgRate float64 `json:"avg_rate"`
}

// Corridor is a supported send/receive currency pair.
type Corridor struct {
	SendCurrency    string `json:"send_currency"`
	ReceiveCurrency string `json:"receive_currency"`
}

// ProviderInfo is the JSON-serialisable representation of a provider.
type ProviderInfo struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Slug         string `json:"slug"`
	WebsiteURL   string `json:"website_url,omitempty"`
	DataSource   string `json:"data_source"`
	IsActive     bool   `json:"is_active"`
	DisplayOrder int32  `json:"display_order"`
	CreatedAt    string `json:"created_at"`
}

// AlertInfo is the JSON-serialisable representation of a rate alert.
type AlertInfo struct {
	ID              string  `json:"id"`
	SendCurrency    string  `json:"send_currency"`
	ReceiveCurrency string  `json:"receive_currency"`
	TargetRate      float64 `json:"target_rate"`
	Direction       string  `json:"direction"`
	IsActive        bool    `json:"is_active"`
	LastTriggeredAt string  `json:"last_triggered_at,omitempty"`
	CreatedAt       string  `json:"created_at"`
}

// ProviderCalculation is a single provider's result in the calculator output.
type ProviderCalculation struct {
	ProviderID     string  `json:"provider_id"`
	ProviderName   string  `json:"provider_name"`
	Rate           float64 `json:"rate"`
	ReceiveAmount  float64 `json:"receive_amount"`
	FeeFlat        float64 `json:"fee_flat"`
	FeePct         float64 `json:"fee_pct"`
	TotalFees      float64 `json:"total_fees"`
	EffectiveRate  float64 `json:"effective_rate"`
	TransferTime   string  `json:"transfer_time,omitempty"`
}

// CalculateResult is the output of the send-amount calculator: ranked
// providers with the best provider highlighted. The DTO includes the input
// parameters (send_amount, send_currency, receive_currency) so the frontend
// can render the result without echoing the request separately.
type CalculateResult struct {
	SendAmount       float64               `json:"send_amount"`
	SendCurrency     string                `json:"send_currency"`
	ReceiveCurrency  string                `json:"receive_currency"`
	Results          []ProviderCalculation `json:"results"`
	BestProviderID   string                `json:"best_provider_id,omitempty"`
	BestReceiveAmount float64              `json:"best_receive_amount,omitempty"`
}

// ---------------------------------------------------------------------------
// Numeric / time helpers
// ---------------------------------------------------------------------------

// numericToFloat64 converts a pgtype.Numeric to float64, returning 0 for NULL.
func numericToFloat64(n pgtype.Numeric) float64 {
	if !n.Valid {
		return 0
	}
	f, err := n.Float64Value()
	if err != nil {
		return 0
	}
	return f.Float64
}

// numericFromFloat converts a float64 to a pgtype.Numeric.
func numericFromFloat(f float64) pgtype.Numeric {
	n := pgtype.Numeric{}
	_ = n.Scan(strconv.FormatFloat(f, 'f', -1, 64))
	return n
}

// ptrString dereferences a *string safely, returning "" for nil.
func ptrString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// rfc3339 formats a pgtype.Timestamptz as RFC 3339 UTC, or "" if invalid.
func rfc3339(t pgtype.Timestamptz) string {
	if !t.Valid {
		return ""
	}
	return t.Time.UTC().Format(time.RFC3339)
}

// rfc3339Date formats a pgtype.Date as an RFC 3339 date string, or "" if invalid.
func rfc3339Date(d pgtype.Date) string {
	if !d.Valid {
		return ""
	}
	return d.Time.UTC().Format("2006-01-02")
}

// ---------------------------------------------------------------------------
// Settings helpers
// ---------------------------------------------------------------------------

// IsEnabled returns whether the remittance tracker is globally enabled.
func (s *Service) IsEnabled(ctx context.Context, tenantID pgtype.UUID) bool {
	if s.settings == nil {
		return true
	}
	return s.settings.GetBoolOr(ctx, tenantID, "remittance.enabled", true)
}

// manualStaleHours returns the configured manual-staleness threshold.
func (s *Service) manualStaleHours(ctx context.Context, tenantID pgtype.UUID) int {
	if s.settings == nil {
		return 24
	}
	return s.settings.GetIntOr(ctx, tenantID, "remittance.manual_stale_hours", 24)
}

// HistoryRetentionDays returns the configured history retention period.
func (s *Service) HistoryRetentionDays(ctx context.Context, tenantID pgtype.UUID) int {
	if s.settings == nil {
		return 365
	}
	return s.settings.GetIntOr(ctx, tenantID, "remittance.history_retention_days", 365)
}

// DefaultSendCurrency returns the pre-selected send currency for the UI.
func (s *Service) DefaultSendCurrency(ctx context.Context, tenantID pgtype.UUID) string {
	if s.settings == nil {
		return "GBP"
	}
	return s.settings.GetStringOr(ctx, tenantID, "remittance.default_send_currency", "GBP")
}

// IsManualRateStale returns true if a manual rate's fetched_at is older than
// the configured manual_stale_hours threshold.
func (s *Service) IsManualRateStale(fetchedAt time.Time) bool {
	ctx := context.Background()
	staleHours := s.manualStaleHours(ctx, pgtype.UUID{})
	return time.Since(fetchedAt) > time.Duration(staleHours)*time.Hour
}

// ---------------------------------------------------------------------------
// Cache key builders
// ---------------------------------------------------------------------------

// ratesCacheKey builds the tenant-scoped Valkey key for cached latest rates
// of a corridor. Tenant scoping prevents cross-tenant data bleed (Rule 4,
// §4.2). Ref: conventions/valkey-keys-catalog.md (jc:remit:rates).
func ratesCacheKey(tenantID pgtype.UUID, sendCurrency, receiveCurrency string) string {
	return valkey.MustKey("remit:rates:%s:%s:%s", tenantID.String(), sendCurrency, receiveCurrency)
}

// bestCacheKey builds the tenant-scoped Valkey key for the cached best rate
// of a corridor. Tenant scoping prevents cross-tenant data bleed (Rule 4,
// §4.2). Ref: conventions/valkey-keys-catalog.md (jc:remit:best).
func bestCacheKey(tenantID pgtype.UUID, sendCurrency, receiveCurrency string) string {
	return valkey.MustKey("remit:best:%s:%s:%s", tenantID.String(), sendCurrency, receiveCurrency)
}

// ---------------------------------------------------------------------------
// Core read operations
// ---------------------------------------------------------------------------

// GetRates returns the latest rates for a corridor across all active
// providers. Results are cached in Valkey (jc:remit:rates:{tenant}:{send}:{receive},
// 15-min TTL) and fall back to a live DB query on cache miss.
func (s *Service) GetRates(ctx context.Context, tenantID pgtype.UUID, sendCurrency, receiveCurrency string) ([]RateInfo, error) {
	if !s.IsEnabled(ctx, tenantID) {
		return nil, fmt.Errorf("remittance: tracker disabled")
	}

	key := ratesCacheKey(tenantID, sendCurrency, receiveCurrency)
	if s.cache != nil {
		if cached, err := s.cache.Get(ctx, key).Result(); err == nil && cached != "" {
			var rates []RateInfo
			if err := json.Unmarshal([]byte(cached), &rates); err == nil {
				return rates, nil
			}
		}
	}

	rows, err := s.store.GetLatestRates(ctx, tenantID, sendCurrency, receiveCurrency)
	if err != nil {
		return nil, fmt.Errorf("remittance: get rates: %w", err)
	}

	rates := make([]RateInfo, 0, len(rows))
	for _, r := range rows {
		rates = append(rates, RateInfo{
			ID:              r.ID.String(),
			ProviderID:      r.ProviderID.String(),
			ProviderName:    r.ProviderName,
			ProviderSlug:    r.ProviderSlug,
			ProviderWebsite: ptrString(r.ProviderWebsite),
			SendCurrency:    r.SendCurrency,
			ReceiveCurrency: r.ReceiveCurrency,
			Rate:            numericToFloat64(r.Rate),
			FeeFlat:         numericToFloat64(r.FeeFlat),
			FeePct:          numericToFloat64(r.FeePct),
			TransferTime:    ptrString(r.TransferTime),
			FetchedAt:       rfc3339(r.FetchedAt),
			IsManual:        r.IsManual,
			SourceURL:       ptrString(r.SourceUrl),
		})
	}

	if s.cache != nil && len(rates) > 0 {
		if data, err := json.Marshal(rates); err == nil {
			if err := s.cache.Set(ctx, key, data, rateCacheTTL).Err(); err != nil {
				slog.Warn("remittance: get rates: cache set failed", "error", err)
			}
		}
	}

	return rates, nil
}

// GetBestRate returns the provider with the best (highest) effective rate for
// a corridor. Results are cached in Valkey (jc:remit:best:{tenant}:{send}:{receive},
// 15-min TTL) and fall back to a live DB query on cache miss.
func (s *Service) GetBestRate(ctx context.Context, tenantID pgtype.UUID, sendCurrency, receiveCurrency string) (*BestRate, error) {
	if !s.IsEnabled(ctx, tenantID) {
		return nil, fmt.Errorf("remittance: tracker disabled")
	}

	key := bestCacheKey(tenantID, sendCurrency, receiveCurrency)
	if s.cache != nil {
		if cached, err := s.cache.Get(ctx, key).Result(); err == nil && cached != "" {
			var best BestRate
			if err := json.Unmarshal([]byte(cached), &best); err == nil {
				return &best, nil
			}
		}
	}

	rates, err := s.GetRates(ctx, tenantID, sendCurrency, receiveCurrency)
	if err != nil {
		return nil, err
	}
	if len(rates) == 0 {
		return nil, nil
	}

	best := rates[0]
	for _, r := range rates[1:] {
		if r.Rate > best.Rate {
			best = r
		}
	}

	result := &BestRate{
		ProviderID:    best.ProviderID,
		ProviderName:  best.ProviderName,
		Rate:          best.Rate,
		ReceiveAmount: best.Rate, // per 1 unit send
	}

	if s.cache != nil {
		if data, err := json.Marshal(result); err == nil {
			if err := s.cache.Set(ctx, key, data, rateCacheTTL).Err(); err != nil {
				slog.Warn("remittance: get best rate: cache set failed", "error", err)
			}
		}
	}

	return result, nil
}

// Calculate returns a ranked list of providers with estimated receive amount,
// total fees, and effective rate for a given send amount. Reads from the
// Valkey cache of latest rates, falling back to a live DB query.
func (s *Service) Calculate(ctx context.Context, tenantID pgtype.UUID, sendAmount float64, sendCurrency, receiveCurrency string) (*CalculateResult, error) {
	if !s.IsEnabled(ctx, tenantID) {
		return nil, fmt.Errorf("remittance: tracker disabled")
	}
	if sendAmount <= 0 {
		return nil, fmt.Errorf("remittance: send amount must be positive")
	}

	rates, err := s.GetRates(ctx, tenantID, sendCurrency, receiveCurrency)
	if err != nil {
		return nil, err
	}
	if len(rates) == 0 {
		return &CalculateResult{
			SendAmount:      sendAmount,
			SendCurrency:    sendCurrency,
			ReceiveCurrency: receiveCurrency,
			Results:         []ProviderCalculation{},
		}, nil
	}

	providers := make([]ProviderCalculation, 0, len(rates))
	for _, r := range rates {
		pc := computeProviderCalculation(r, sendAmount)
		providers = append(providers, pc)
	}

	// Rank by receive amount descending.
	sort.Slice(providers, func(i, j int) bool {
		return providers[i].ReceiveAmount > providers[j].ReceiveAmount
	})

	result := &CalculateResult{
		SendAmount:      sendAmount,
		SendCurrency:    sendCurrency,
		ReceiveCurrency: receiveCurrency,
		Results:         providers,
	}
	if len(providers) > 0 {
		result.BestProviderID = providers[0].ProviderID
		result.BestReceiveAmount = providers[0].ReceiveAmount
	}

	return result, nil
}

// computeProviderCalculation computes the receive amount, fees, and effective
// rate for a single provider given a send amount.
func computeProviderCalculation(r RateInfo, sendAmount float64) ProviderCalculation {
	// Flat fee is in send_currency; percentage fee is a fraction of send amount.
	flatFee := r.FeeFlat
	pctFee := r.FeePct / 100.0 * sendAmount
	totalFees := flatFee + pctFee

	// Net send amount after fees (fees deducted from send side).
	netSend := sendAmount - totalFees
	if netSend < 0 {
		netSend = 0
	}

	receiveAmount := r.Rate * netSend

	// Effective rate = receive amount / original send amount.
	effectiveRate := 0.0
	if sendAmount > 0 {
		effectiveRate = receiveAmount / sendAmount
	}

	return ProviderCalculation{
		ProviderID:    r.ProviderID,
		ProviderName:  r.ProviderName,
		Rate:          r.Rate,
		ReceiveAmount: receiveAmount,
		FeeFlat:       flatFee,
		FeePct:        r.FeePct,
		TotalFees:     totalFees,
		EffectiveRate: effectiveRate,
		TransferTime:  r.TransferTime,
	}
}

// GetHistory returns a time-series of the best rate per day for a corridor
// over the past N days (default 30, max 365).
func (s *Service) GetHistory(ctx context.Context, tenantID pgtype.UUID, sendCurrency, receiveCurrency string, days int) ([]HistoryPoint, error) {
	if !s.IsEnabled(ctx, tenantID) {
		return nil, fmt.Errorf("remittance: tracker disabled")
	}

	maxDays := s.HistoryRetentionDays(ctx, tenantID)
	if days <= 0 {
		days = 30
	}
	if days > maxDays {
		days = maxDays
	}

	fromDate := time.Now().UTC().AddDate(0, 0, -days)

	rows, err := s.store.GetHistory(ctx, tenantID, sendCurrency, receiveCurrency, fromDate)
	if err != nil {
		return nil, fmt.Errorf("remittance: get history: %w", err)
	}

	points := make([]HistoryPoint, 0, len(rows))
	for _, r := range rows {
		// MaxRate and MinRate come back as interface{} — typically a string
		// representation of the numeric value from pgx.
		points = append(points, HistoryPoint{
			Day:     rfc3339Date(r.Day),
			MaxRate: interfaceToFloat64(r.MaxRate),
			MinRate: interfaceToFloat64(r.MinRate),
			AvgRate: numericToFloat64(r.AvgRate),
		})
	}

	return points, nil
}

// interfaceToFloat64 converts a pgx-returned interface{} (string, []byte, or
// numeric) to float64.
func interfaceToFloat64(v interface{}) float64 {
	switch val := v.(type) {
	case nil:
		return 0
	case float64:
		return val
	case float32:
		return float64(val)
	case int64:
		return float64(val)
	case int:
		return float64(val)
	case string:
		f, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return 0
		}
		return f
	case []byte:
		f, err := strconv.ParseFloat(string(val), 64)
		if err != nil {
			return 0
		}
		return f
	case pgtype.Numeric:
		return numericToFloat64(val)
	default:
		return 0
	}
}

// GetProviders lists remittance providers, optionally filtering to active only.
func (s *Service) GetProviders(ctx context.Context, tenantID pgtype.UUID, activeOnly bool) ([]ProviderInfo, error) {
	rows, err := s.store.ListProviders(ctx, tenantID, activeOnly)
	if err != nil {
		return nil, fmt.Errorf("remittance: get providers: %w", err)
	}

	providers := make([]ProviderInfo, 0, len(rows))
	for _, p := range rows {
		providers = append(providers, ProviderInfo{
			ID:           p.ID.String(),
			Name:         p.Name,
			Slug:         p.Slug,
			WebsiteURL:   ptrString(p.WebsiteUrl),
			DataSource:   p.DataSource,
			IsActive:     p.IsActive,
			DisplayOrder: p.DisplayOrder,
			CreatedAt:    rfc3339(p.CreatedAt),
		})
	}

	return providers, nil
}

// GetCorridors returns the supported send/receive currency pairs.
func (s *Service) GetCorridors(ctx context.Context, tenantID pgtype.UUID) ([]Corridor, error) {
	rows, err := s.store.GetCorridors(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("remittance: get corridors: %w", err)
	}

	corridors := make([]Corridor, 0, len(rows))
	for _, r := range rows {
		corridors = append(corridors, Corridor{
			SendCurrency:    r.SendCurrency,
			ReceiveCurrency: r.ReceiveCurrency,
		})
	}

	return corridors, nil
}

// ---------------------------------------------------------------------------
// Alert CRUD
// ---------------------------------------------------------------------------

// CreateAlert creates a new rate alert for a user.
func (s *Service) CreateAlert(ctx context.Context, tenantID, userID pgtype.UUID, sendCurrency, receiveCurrency, direction string, targetRate float64) (AlertInfo, error) {
	if direction != DirectionAbove && direction != DirectionBelow {
		return AlertInfo{}, fmt.Errorf("remittance: invalid direction %q", direction)
	}
	if targetRate <= 0 {
		return AlertInfo{}, fmt.Errorf("remittance: target rate must be positive")
	}

	alert, err := s.store.CreateAlert(ctx, gen.CreateRateAlertParams{
		TenantID:        tenantID,
		UserID:          userID,
		SendCurrency:    sendCurrency,
		ReceiveCurrency: receiveCurrency,
		TargetRate:      numericFromFloat(targetRate),
		Direction:       direction,
	})
	if err != nil {
		return AlertInfo{}, fmt.Errorf("remittance: create alert: %w", err)
	}

	return alertToInfo(alert), nil
}

// UpdateAlert updates a rate alert's mutable fields. Only non-nil fields are
// updated (COALESCE in the SQL).
func (s *Service) UpdateAlert(ctx context.Context, tenantID, userID, alertID pgtype.UUID, targetRate *float64, direction *string, isActive *bool) (AlertInfo, error) {
	if direction != nil && *direction != DirectionAbove && *direction != DirectionBelow {
		return AlertInfo{}, fmt.Errorf("remittance: invalid direction %q", *direction)
	}

	arg := gen.UpdateRateAlertParams{
		TenantID: tenantID,
		ID:       alertID,
		UserID:   userID,
	}
	if targetRate != nil {
		arg.TargetRate = numericFromFloat(*targetRate)
	}
	if direction != nil {
		arg.Direction = direction
	}
	if isActive != nil {
		arg.IsActive = isActive
	}

	alert, err := s.store.UpdateAlert(ctx, arg)
	if err != nil {
		return AlertInfo{}, fmt.Errorf("remittance: update alert: %w", err)
	}

	return alertToInfo(alert), nil
}

// DeleteAlert deletes a rate alert owned by the given user.
func (s *Service) DeleteAlert(ctx context.Context, tenantID, userID, alertID pgtype.UUID) error {
	if err := s.store.DeleteAlert(ctx, tenantID, alertID, userID); err != nil {
		return fmt.Errorf("remittance: delete alert: %w", err)
	}
	return nil
}

// ListUserAlerts lists a user's rate alerts.
func (s *Service) ListUserAlerts(ctx context.Context, tenantID, userID pgtype.UUID) ([]AlertInfo, error) {
	alerts, err := s.store.ListUserAlerts(ctx, tenantID, userID, false)
	if err != nil {
		return nil, fmt.Errorf("remittance: list alerts: %w", err)
	}

	results := make([]AlertInfo, 0, len(alerts))
	for _, a := range alerts {
		results = append(results, alertToInfo(a))
	}

	return results, nil
}

// alertToInfo converts a gen.RemittanceRateAlert to the JSON DTO.
func alertToInfo(a gen.RemittanceRateAlert) AlertInfo {
	return AlertInfo{
		ID:              a.ID.String(),
		SendCurrency:    a.SendCurrency,
		ReceiveCurrency: a.ReceiveCurrency,
		TargetRate:      numericToFloat64(a.TargetRate),
		Direction:       a.Direction,
		IsActive:        a.IsActive,
		LastTriggeredAt: rfc3339(a.LastTriggeredAt),
		CreatedAt:       rfc3339(a.CreatedAt),
	}
}

// ---------------------------------------------------------------------------
// Admin operations
// ---------------------------------------------------------------------------

// CreateProvider creates a new remittance provider.
func (s *Service) CreateProvider(ctx context.Context, arg gen.CreateRemittanceProviderParams) (ProviderInfo, error) {
	if arg.DataSource != DataSourceAPI && arg.DataSource != DataSourceManual {
		return ProviderInfo{}, fmt.Errorf("remittance: invalid data_source %q", arg.DataSource)
	}

	p, err := s.store.CreateProvider(ctx, arg)
	if err != nil {
		return ProviderInfo{}, fmt.Errorf("remittance: create provider: %w", err)
	}

	return providerToInfo(p), nil
}

// UpdateProvider updates a remittance provider's mutable fields.
func (s *Service) UpdateProvider(ctx context.Context, arg gen.UpdateRemittanceProviderParams) (ProviderInfo, error) {
	p, err := s.store.UpdateProvider(ctx, arg)
	if err != nil {
		return ProviderInfo{}, fmt.Errorf("remittance: update provider: %w", err)
	}
	return providerToInfo(p), nil
}

// InsertManualRate inserts a manual rate for a provider and invalidates the
// corridor's rate cache.
func (s *Service) InsertManualRate(ctx context.Context, tenantID, providerID pgtype.UUID, sendCurrency, receiveCurrency string, rate, feeFlat, feePct float64, transferTime, sourceURL *string) (gen.RemittanceRate, error) {
	arg := gen.InsertRemittanceRateParams{
		TenantID:        tenantID,
		ProviderID:      providerID,
		SendCurrency:    sendCurrency,
		ReceiveCurrency: receiveCurrency,
		Rate:            numericFromFloat(rate),
		IsManual:        true,
		TransferTime:    transferTime,
		SourceUrl:       sourceURL,
	}
	if feeFlat > 0 {
		arg.FeeFlat = numericFromFloat(feeFlat)
	}
	if feePct > 0 {
		arg.FeePct = numericFromFloat(feePct)
	}

	r, err := s.store.InsertRate(ctx, arg)
	if err != nil {
		return gen.RemittanceRate{}, fmt.Errorf("remittance: insert manual rate: %w", err)
	}

	s.InvalidateRateCache(ctx, tenantID, sendCurrency, receiveCurrency)

	return r, nil
}

// ListProvidersPaginated returns a page of providers and the total count.
func (s *Service) ListProvidersPaginated(ctx context.Context, tenantID pgtype.UUID, activeOnly bool, limit, offset int32) ([]ProviderInfo, int64, error) {
	rows, total, err := s.store.ListProvidersPaginated(ctx, tenantID, activeOnly, limit, offset)
	if err != nil {
		return nil, 0, err
	}

	providers := make([]ProviderInfo, 0, len(rows))
	for _, p := range rows {
		providers = append(providers, providerToInfo(p))
	}

	return providers, total, nil
}

// providerToInfo converts a gen.RemittanceProvider to the JSON DTO.
func providerToInfo(p gen.RemittanceProvider) ProviderInfo {
	return ProviderInfo{
		ID:           p.ID.String(),
		Name:         p.Name,
		Slug:         p.Slug,
		WebsiteURL:   ptrString(p.WebsiteUrl),
		DataSource:   p.DataSource,
		IsActive:     p.IsActive,
		DisplayOrder: p.DisplayOrder,
		CreatedAt:    rfc3339(p.CreatedAt),
	}
}

// DecryptAPIConfig decrypts the encrypted api_config stored on a remittance
// provider, returning the original raw JSON. Use this when the rate-fetch
// logic needs the provider's API credentials (ADR-075).
func DecryptAPIConfig(p gen.RemittanceProvider) ([]byte, error) {
	decrypted, err := crypto.DecryptJSON(p.ApiConfig)
	if err != nil {
		return nil, fmt.Errorf("remittance: decrypt api_config: %w", err)
	}
	return decrypted, nil
}

// ---------------------------------------------------------------------------
// Background task helpers
// ---------------------------------------------------------------------------

// FindAlertsNeedingNotifications returns active alerts for a corridor that are
// eligible for notification (last_triggered_at is null or > 24h ago). Used by
// the remittance:check_alerts task.
func (s *Service) FindAlertsNeedingNotifications(ctx context.Context, tenantID pgtype.UUID, sendCurrency, receiveCurrency string) ([]gen.RemittanceRateAlert, error) {
	return s.store.ListActiveAlertsForCorridor(ctx, tenantID, sendCurrency, receiveCurrency)
}

// MarkAlertTriggered sets last_triggered_at to now() for the given alert.
func (s *Service) MarkAlertTriggered(ctx context.Context, id pgtype.UUID) error {
	return s.store.MarkAlertTriggered(ctx, id)
}

// PruneOldRates deletes rate rows older than the cutoff, keeping one snapshot
// per provider per corridor per day. Used by the remittance:prune_old_rates task.
func (s *Service) PruneOldRates(ctx context.Context, tenantID pgtype.UUID, cutoffDate time.Time) error {
	return s.store.PruneOldRates(ctx, tenantID, cutoffDate)
}

// InvalidateRateCache deletes the tenant-scoped Valkey cache keys for a
// corridor. Called after every successful rate fetch (API or manual insert).
func (s *Service) InvalidateRateCache(ctx context.Context, tenantID pgtype.UUID, sendCurrency, receiveCurrency string) {
	if s.cache == nil {
		return
	}
	keys := []string{
		ratesCacheKey(tenantID, sendCurrency, receiveCurrency),
		bestCacheKey(tenantID, sendCurrency, receiveCurrency),
	}
	if err := s.cache.Del(ctx, keys...).Err(); err != nil {
		slog.Warn("remittance: invalidate cache failed",
			"tenant_id", tenantID.String(),
			"send", sendCurrency, "receive", receiveCurrency, "error", err)
	}
}

// TriggerFetch enqueues a remittance:fetch_rates task for a specific provider.
// This is the admin-triggered entry point — the task handler then calls
// FetchProviderRates to perform the actual API call and rate insertion.
func (s *Service) TriggerFetch(ctx context.Context, tenantID, providerID pgtype.UUID) error {
	if s.enqueuer == nil {
		return fmt.Errorf("remittance: no enqueuer configured")
	}

	payload, err := json.Marshal(map[string]string{
		"tenant_id":   tenantID.String(),
		"provider_id": providerID.String(),
	})
	if err != nil {
		return fmt.Errorf("remittance: marshal fetch payload: %w", err)
	}

	if err := s.enqueuer.Enqueue(worker.TypeRemittanceFetchRates, payload); err != nil {
		return fmt.Errorf("remittance: enqueue fetch: %w", err)
	}

	return nil
}

// FetchProviderRates fetches live rates from a provider's API, inserts each
// rate as an append-only snapshot row, and invalidates the corridor cache.
// This is the actual rate-fetch implementation called by the
// remittance:fetch_rates task handler (ADR-075 §2). For manual providers
// (data_source='manual'), this is a no-op — manual rates are entered by
// admins via InsertManualRate.
func (s *Service) FetchProviderRates(ctx context.Context, tenantID, providerID pgtype.UUID) error {
	provider, err := s.store.GetProviderByID(ctx, tenantID, providerID)
	if err != nil {
		return fmt.Errorf("remittance: fetch provider rates: get provider: %w", err)
	}

	// Manual providers have no live API to call — rates are admin-entered.
	if provider.DataSource == DataSourceManual {
		slog.Info("remittance: fetch provider rates: skipping manual provider",
			"provider_id", providerID.String(), "slug", provider.Slug)
		return nil
	}

	// Resolve the registered RateProvider adapter for this provider slug.
	if s.providers == nil {
		return fmt.Errorf("remittance: no rate-provider adapters registered (provider %q)", provider.Slug)
	}
	adapter, ok := s.providers[provider.Slug]
	if !ok {
		return fmt.Errorf("remittance: no rate-provider adapter registered for slug %q", provider.Slug)
	}

	// Decrypt the provider's API credentials/config (ADR-075 §1).
	apiConfig, err := DecryptAPIConfig(provider)
	if err != nil {
		return fmt.Errorf("remittance: fetch provider rates: decrypt config: %w", err)
	}

	// Fetch all configured corridors for this tenant so the provider adapter
	// can fetch rates for each one.
	corridors, err := s.GetCorridors(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("remittance: fetch provider rates: get corridors: %w", err)
	}

	snapshots, err := adapter.FetchRates(ctx, apiConfig, corridors)
	if err != nil {
		return fmt.Errorf("remittance: fetch provider rates: adapter %q: %w", provider.Slug, err)
	}
	if len(snapshots) == 0 {
		slog.Info("remittance: fetch provider rates: adapter returned no snapshots",
			"provider_id", providerID.String(), "slug", provider.Slug)
		return nil
	}

	// Insert each snapshot as an append-only rate row and invalidate the
	// corridor cache. ADR-075 §3: rate history is append-only — never UPDATE.
	for _, snap := range snapshots {
		arg := gen.InsertRemittanceRateParams{
			TenantID:        tenantID,
			ProviderID:      providerID,
			SendCurrency:    snap.SendCurrency,
			ReceiveCurrency: snap.ReceiveCurrency,
			Rate:            numericFromFloat(snap.Rate),
			IsManual:        false,
		}
		if snap.FeeFlat > 0 {
			arg.FeeFlat = numericFromFloat(snap.FeeFlat)
		}
		if snap.FeePct > 0 {
			arg.FeePct = numericFromFloat(snap.FeePct)
		}
		if snap.TransferTime != "" {
			arg.TransferTime = &snap.TransferTime
		}
		if snap.SourceURL != "" {
			arg.SourceUrl = &snap.SourceURL
		}

		if _, err := s.store.InsertRate(ctx, arg); err != nil {
			slog.Warn("remittance: fetch provider rates: insert rate failed",
				"provider_id", providerID.String(),
				"send", snap.SendCurrency, "receive", snap.ReceiveCurrency,
				"error", err)
			continue
		}

		s.InvalidateRateCache(ctx, tenantID, snap.SendCurrency, snap.ReceiveCurrency)
	}

	slog.Info("remittance: fetch provider rates: completed",
		"provider_id", providerID.String(), "slug", provider.Slug,
		"snapshots", len(snapshots))
	return nil
}

// FetchAllProviderRates iterates all active API-sourced providers for a
// tenant and fetches their rates. Called by the scheduler sweep. Manual
// providers are skipped. Returns the number of providers processed.
// Ref: ADR-075 §2, §8.
func (s *Service) FetchAllProviderRates(ctx context.Context, tenantID pgtype.UUID) (int, error) {
	providers, err := s.store.ListProviders(ctx, tenantID, true)
	if err != nil {
		return 0, fmt.Errorf("remittance: fetch all provider rates: list providers: %w", err)
	}

	var total int
	for _, p := range providers {
		if p.DataSource != DataSourceAPI {
			continue // skip manual providers
		}
		if err := s.FetchProviderRates(ctx, tenantID, p.ID); err != nil {
			slog.Warn("remittance: fetch all provider rates: provider failed",
				"provider_id", p.ID.String(), "slug", p.Slug, "error", err)
			continue
		}
		total++
	}
	return total, nil
}

// CheckAndDispatchAlerts evaluates all active alerts for a tenant against the
// current best rate for each corridor. When an alert's threshold is met
// (rate above/below target) and the 24-hour debounce window has elapsed, a
// notification is dispatched and the alert's last_triggered_at is updated
// (ADR-075 §4). Returns the number of alerts that fired.
func (s *Service) CheckAndDispatchAlerts(ctx context.Context, tenantID pgtype.UUID) (int, error) {
	if s.notifier == nil {
		return 0, fmt.Errorf("remittance: no notifier configured")
	}

	// Query all active alerts for this tenant that are eligible for
	// notification (last_triggered_at is null or > 24h ago). Group by
	// corridor so we fetch the best rate once per corridor.
	alerts, err := s.store.ListAllActiveAlertsForTenant(ctx, tenantID)
	if err != nil {
		return 0, fmt.Errorf("remittance: check alerts: list alerts: %w", err)
	}

	// Cache best rates per corridor to avoid redundant lookups.
	type corridorKey struct{ send, recv string }
	rateCache := make(map[corridorKey]*BestRate)

	var fired int
	for _, alert := range alerts {
		key := corridorKey{alert.SendCurrency, alert.ReceiveCurrency}
		best, ok := rateCache[key]
		if !ok {
			best, err = s.GetBestRate(ctx, tenantID, alert.SendCurrency, alert.ReceiveCurrency)
			if err != nil {
				slog.Warn("remittance: check alerts: get best rate failed",
					"send", alert.SendCurrency, "receive", alert.ReceiveCurrency,
					"error", err)
				continue
			}
			rateCache[key] = best
		}
		if best == nil {
			continue // no rates for this corridor yet
		}

		// Evaluate the threshold condition (ADR-075 §4).
		met := false
		switch alert.Direction {
		case DirectionAbove:
			met = best.Rate >= numericToFloat64(alert.TargetRate)
		case DirectionBelow:
			met = best.Rate <= numericToFloat64(alert.TargetRate)
		}
		if !met {
			continue
		}

		// Dispatch the notification.
		title := fmt.Sprintf("Rate alert: %s→%s %s %.4f",
			alert.SendCurrency, alert.ReceiveCurrency, alert.Direction,
			numericToFloat64(alert.TargetRate))
		body := fmt.Sprintf("Current best rate is %.4f via %s.",
			best.Rate, best.ProviderName)
		actionURL := "/tools/remittance"

		var zeroUUID pgtype.UUID
		if err := s.notifier.CreateNotification(ctx, tenantID, alert.UserID,
			"remittance_alert", "", zeroUUID,
			"remittance_corridor", alert.ID,
			title, body, actionURL); err != nil {
			slog.Warn("remittance: check alerts: create notification failed",
				"alert_id", alert.ID.String(), "error", err)
			continue
		}

		// Mark the alert as triggered (24h debounce).
		if err := s.store.MarkAlertTriggered(ctx, alert.ID); err != nil {
			slog.Warn("remittance: check alerts: mark triggered failed",
				"alert_id", alert.ID.String(), "error", err)
			continue
		}
		fired++
	}

	return fired, nil
}
