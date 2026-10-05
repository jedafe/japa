// Deal feed import (Section 55 — Deal Feed Import). Registered
// coupon_feed_sources are fetched on the coupon:import_feed Asynq task,
// normalized to feedOffer, and upserted into coupons on
// (feed_id, external_id) so repeated syncs are idempotent.
package coupons

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/crypto"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/valkey"
	"github.com/jedafe/japa/internal/worker"
)

// Feed source types — must match the coupon_feed_sources.source_type CHECK.
const (
	FeedSourceCouponAPI = "couponapi" // CouponAPI.org incremental feed
	FeedSourceGeneric   = "generic"   // full-snapshot JSON feed
)

// feedHTTPTimeout bounds a single upstream feed fetch (§55).
const feedHTTPTimeout = 30 * time.Second

// feedMaxBody caps feed response bodies (§55 — 20 MiB).
const feedMaxBody = 20 << 20

// feedLockTTL is the Valkey SET NX lock lifetime (jc:coupon_feed_lock:{feed_id}).
const feedLockTTL = 10 * time.Minute

// Sentinel errors for the feed-import path.
var (
	ErrFeedSourceNotFound = errors.New("coupon feed source not found")
	ErrFeedSyncLocked     = errors.New("feed sync already in progress")
	ErrFeedFetch          = errors.New("feed fetch failed")
	ErrInvalidFeedSource  = errors.New("invalid feed source")
)

// feedOffer is the normalized offer shape both adapters decode into.
type feedOffer struct {
	ExternalID    string // required — feeds store it in coupons.external_id
	Title         string // required
	Description   string
	Code          string
	Store         string // merchant name → provider resolution
	MerchantURL   string // provider base_url fallback
	URL           string // naked offer URL
	AffiliateLink string // tracked link — preferred referral_url
	StartDate     string // yyyy-mm-dd
	EndDate       string
	Status        string // 'suspended' suspends; anything else stays active
}

// couponAPIOffer mirrors CouponAPI.org's offer record.
type couponAPIOffer struct {
	OfferID     json.Number `json:"offer_id"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Code        string      `json:"code"`
	Store       string      `json:"store"`
	MerchantURL string      `json:"merchant_home_page"`
	URL         string      `json:"url"`
	Affiliate   string      `json:"affiliate_link"`
	StartDate   string      `json:"start_date"`
	EndDate     string      `json:"end_date"`
	Status      string      `json:"status"`
}

// genericOffer is the documented field set for 'generic' feeds.
type genericOffer struct {
	ID          string `json:"id"`
	OfferID     string `json:"offer_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Code        string `json:"code"`
	Store       string `json:"store"`
	MerchantURL string `json:"merchant_home_page"`
	URL         string `json:"url"`
	Affiliate   string `json:"affiliate_link"`
	StartDate   string `json:"start_date"`
	EndDate     string `json:"end_date"`
	Status      string `json:"status"`
}

// FeedStats records the outcome of one sync run on coupon_feed_sources.stats.
type FeedStats struct {
	Seen      int `json:"seen"`
	Created   int `json:"created"`
	Updated   int `json:"updated"`
	Suspended int `json:"suspended"`
	Expired   int `json:"expired"`
	Skipped   int `json:"skipped"`
}

// FeedSourceInput is the admin create/update payload. APIKey is write-only:
// nil keeps the stored key, non-empty replaces it (encrypted at rest).
type FeedSourceInput struct {
	Name              string       `json:"name"`
	SourceType        string       `json:"source_type"`
	EndpointURL       string       `json:"endpoint_url"`
	APIKey            *string      `json:"api_key"`
	DefaultProviderID *pgtype.UUID `json:"default_provider_id"`
	Enabled           bool         `json:"enabled"`
}

// FeedSourceView is the admin API representation — api_key_encrypted is
// never exposed; has_api_key reports whether a key is configured.
type FeedSourceView struct {
	ID                string          `json:"id"`
	Name              string          `json:"name"`
	SourceType        string          `json:"source_type"`
	EndpointURL       string          `json:"endpoint_url"`
	HasAPIKey         bool            `json:"has_api_key"`
	DefaultProviderID *string         `json:"default_provider_id"`
	Enabled           bool            `json:"enabled"`
	LastSyncedAt      *string         `json:"last_synced_at"`
	LastStatus        *string         `json:"last_status"`
	LastError         *string         `json:"last_error"`
	Stats             json.RawMessage `json:"stats"`
	CouponCount       *int64          `json:"coupon_count,omitempty"`
	CreatedAt         string          `json:"created_at"`
	UpdatedAt         string          `json:"updated_at"`
}

func feedSourceToView(src gen.CouponFeedSource, couponCount *int64) FeedSourceView {
	v := FeedSourceView{
		ID:          src.ID.String(),
		Name:        src.Name,
		SourceType:  src.SourceType,
		EndpointURL: src.EndpointUrl,
		HasAPIKey:   src.ApiKeyEncrypted != nil && *src.ApiKeyEncrypted != "",
		Enabled:     src.Enabled,
		LastStatus:  src.LastStatus,
		LastError:   src.LastError,
		Stats:       src.Stats,
		CouponCount: couponCount,
		CreatedAt:   src.CreatedAt.Time.Format(time.RFC3339),
		UpdatedAt:   src.UpdatedAt.Time.Format(time.RFC3339),
	}
	if src.DefaultProviderID.Valid {
		s := src.DefaultProviderID.String()
		v.DefaultProviderID = &s
	}
	if src.LastSyncedAt.Valid {
		s := src.LastSyncedAt.Time.Format(time.RFC3339)
		v.LastSyncedAt = &s
	}
	return v
}

// ListFeedSources returns all feed sources with coupon counts for the admin table.
func (s *Service) ListFeedSources(ctx context.Context) ([]FeedSourceView, error) {
	rows, err := s.store.listFeedSources(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing feed sources: %w", err)
	}

	views := make([]FeedSourceView, 0, len(rows))
	for i := range rows {
		r := rows[i]
		count := r.CouponCount
		views = append(views, feedSourceToView(gen.CouponFeedSource{
			ID: r.ID, Name: r.Name, SourceType: r.SourceType, EndpointUrl: r.EndpointUrl,
			ApiKeyEncrypted: r.ApiKeyEncrypted, DefaultProviderID: r.DefaultProviderID,
			Enabled: r.Enabled, LastSyncedAt: r.LastSyncedAt, LastStatus: r.LastStatus,
			LastError: r.LastError, Stats: r.Stats, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		}, &count))
	}
	return views, nil
}

// GetFeedSource returns one source by ID.
func (s *Service) GetFeedSource(ctx context.Context, id pgtype.UUID) (FeedSourceView, error) {
	src, err := s.store.getFeedSourceByID(ctx, id)
	if err != nil {
		if isNoRows(err) {
			return FeedSourceView{}, ErrFeedSourceNotFound
		}
		return FeedSourceView{}, fmt.Errorf("getting feed source: %w", err)
	}
	return feedSourceToView(src, nil), nil
}

// CreateFeedSource registers a new feed source. APIKey is encrypted via
// crypto.Encrypt before storage (§55).
func (s *Service) CreateFeedSource(ctx context.Context, input FeedSourceInput) (FeedSourceView, error) {
	if !validSourceType(input.SourceType) {
		return FeedSourceView{}, fmt.Errorf("%w: source_type must be couponapi or generic", ErrInvalidFeedSource)
	}
	if !validFeedURL(input.EndpointURL) {
		return FeedSourceView{}, fmt.Errorf("%w: endpoint_url must be an http(s) URL", ErrInvalidFeedSource)
	}

	var encKey *string
	if input.APIKey != nil && *input.APIKey != "" {
		enc, err := crypto.Encrypt(*input.APIKey)
		if err != nil {
			return FeedSourceView{}, fmt.Errorf("encrypting api key: %w", err)
		}
		encKey = &enc
	}

	src, err := s.store.createFeedSource(ctx, gen.CreateCouponFeedSourceParams{
		Name:              input.Name,
		SourceType:        input.SourceType,
		EndpointUrl:       input.EndpointURL,
		ApiKeyEncrypted:   encKey,
		DefaultProviderID: toNullableUUID(input.DefaultProviderID),
		Enabled:           input.Enabled,
	})
	if err != nil {
		return FeedSourceView{}, fmt.Errorf("creating feed source: %w", err)
	}
	return feedSourceToView(src, nil), nil
}

// UpdateFeedSource updates an existing source. A nil/empty APIKey keeps the
// stored key; the COALESCE in the update query enforces this.
func (s *Service) UpdateFeedSource(ctx context.Context, id pgtype.UUID, input FeedSourceInput) (FeedSourceView, error) {
	if _, err := s.GetFeedSource(ctx, id); err != nil {
		return FeedSourceView{}, err
	}
	if !validSourceType(input.SourceType) {
		return FeedSourceView{}, fmt.Errorf("%w: source_type must be couponapi or generic", ErrInvalidFeedSource)
	}
	if !validFeedURL(input.EndpointURL) {
		return FeedSourceView{}, fmt.Errorf("%w: endpoint_url must be an http(s) URL", ErrInvalidFeedSource)
	}

	var encKey *string
	if input.APIKey != nil && *input.APIKey != "" {
		enc, err := crypto.Encrypt(*input.APIKey)
		if err != nil {
			return FeedSourceView{}, fmt.Errorf("encrypting api key: %w", err)
		}
		encKey = &enc
	}

	src, err := s.store.updateFeedSource(ctx, gen.UpdateCouponFeedSourceParams{
		ID:                id,
		Name:              input.Name,
		SourceType:        input.SourceType,
		EndpointUrl:       input.EndpointURL,
		ApiKeyEncrypted:   encKey,
		DefaultProviderID: toNullableUUID(input.DefaultProviderID),
		Enabled:           input.Enabled,
	})
	if err != nil {
		return FeedSourceView{}, fmt.Errorf("updating feed source: %w", err)
	}
	return feedSourceToView(src, nil), nil
}

// DeleteFeedSource removes a source. Imported coupons are detached
// (feed_id → NULL via ON DELETE SET NULL), never deleted (§55).
func (s *Service) DeleteFeedSource(ctx context.Context, id pgtype.UUID) error {
	if _, err := s.GetFeedSource(ctx, id); err != nil {
		return err
	}
	return s.store.deleteFeedSource(ctx, id)
}

// EnqueueFeedSync queues a coupon:import_feed task for one source —
// fire-and-forget per Rule 8. Called by POST /admin/coupon-feeds/:id/sync.
func (s *Service) EnqueueFeedSync(ctx context.Context, id pgtype.UUID) error {
	src, err := s.store.getFeedSourceByID(ctx, id)
	if err != nil {
		if isNoRows(err) {
			return ErrFeedSourceNotFound
		}
		return fmt.Errorf("getting feed source: %w", err)
	}
	if !src.Enabled {
		return fmt.Errorf("%w: feed source is disabled", ErrInvalidFeedSource)
	}

	payload, _ := json.Marshal(map[string]string{"feed_id": id.String()})
	if err := s.enqueuer.Enqueue(worker.TypeCouponImportFeed, payload); err != nil {
		return fmt.Errorf("enqueueing %s: %w", worker.TypeCouponImportFeed, err)
	}
	return nil
}

// SyncEnabledFeeds is the coupon:sync_feeds cron body — fans out one
// coupon:import_feed task per enabled source (§11).
func (s *Service) SyncEnabledFeeds(ctx context.Context) error {
	feeds, err := s.store.listEnabledFeedSources(ctx)
	if err != nil {
		return fmt.Errorf("listing enabled feeds: %w", err)
	}

	for _, f := range feeds {
		payload, _ := json.Marshal(map[string]string{"feed_id": f.ID.String()})
		if err := s.enqueuer.Enqueue(worker.TypeCouponImportFeed, payload); err != nil {
			slog.Error("coupon:sync_feeds: enqueue failed", "feed_id", f.ID.String(), "error", err)
		}
	}
	return nil
}

// SyncFeedSource runs one feed import end-to-end: lock → fetch → map →
// upsert → sweep. Called by the coupon:import_feed task handler.
func (s *Service) SyncFeedSource(ctx context.Context, feedID pgtype.UUID) error {
	src, err := s.store.getFeedSourceByID(ctx, feedID)
	if err != nil {
		if isNoRows(err) {
			return ErrFeedSourceNotFound
		}
		return fmt.Errorf("getting feed source: %w", err)
	}
	if !src.Enabled {
		slog.Info("coupon:import_feed: source disabled, skipping", "feed_id", feedID.String())
		return nil
	}

	// Per-source SET NX lock — a second sync while one is in flight is skipped.
	lockKey := valkey.MustKey("coupon_feed_lock:%s", feedID.String())
	if s.vk != nil {
		ok, lerr := s.vk.SetNX(ctx, lockKey, "1", feedLockTTL).Result()
		if lerr != nil {
			slog.Warn("coupon:import_feed: lock check failed, proceeding unlocked", "feed_id", feedID.String(), "error", lerr)
		} else if !ok {
			return ErrFeedSyncLocked
		} else {
			defer func() {
				if derr := s.vk.Del(ctx, lockKey).Err(); derr != nil {
					slog.Warn("coupon:import_feed: lock release failed", "feed_id", feedID.String(), "error", derr)
				}
			}()
		}
	}

	stats, syncErr := s.runFeedSync(ctx, src)

	status := "ok"
	var lastErr *string
	if syncErr != nil {
		status = "error"
		msg := syncErr.Error()
		lastErr = &msg
	}
	statsJSON, _ := json.Marshal(stats)
	if uerr := s.store.updateFeedSourceSyncStatus(ctx, gen.UpdateCouponFeedSourceSyncStatusParams{
		ID: feedID, LastStatus: &status, LastError: lastErr, Stats: statsJSON,
	}); uerr != nil {
		slog.Warn("coupon:import_feed: failed to record sync status", "feed_id", feedID.String(), "error", uerr)
	}

	return syncErr
}

// runFeedSync performs fetch/map/upsert/sweep for one source and returns the
// run stats. Errors abort the run before the sweep so a transient fetch
// failure never mass-expires imported coupons.
func (s *Service) runFeedSync(ctx context.Context, src gen.CouponFeedSource) (FeedStats, error) {
	stats := FeedStats{}

	body, err := s.fetchFeed(ctx, src)
	if err != nil {
		return stats, err
	}

	offers, err := decodeFeed(src.SourceType, body)
	if err != nil {
		return stats, fmt.Errorf("%w: %v", ErrFeedFetch, err)
	}
	stats.Seen = len(offers)

	// seen external IDs — the absence sweep only applies to full-snapshot feeds.
	seen := make([]string, 0, len(offers))

	for i := range offers {
		o := offers[i]
		if o.ExternalID == "" || o.Title == "" {
			stats.Skipped++
			continue
		}
		seen = append(seen, o.ExternalID)

		inserted, err := s.upsertOffer(ctx, src, o)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" && o.Code != "" {
				// coupons_code_key collision on a non-conflict path (code belongs
				// to a different coupon) — retry once with a suffixed code.
				retry := o
				retry.Code = dedupeCode(o.Code, src.ID.String())
				inserted, err = s.upsertOffer(ctx, src, retry)
			}
			if err != nil {
				slog.Warn("coupon:import_feed: upsert failed", "feed_id", src.ID.String(), "external_id", o.ExternalID, "error", err)
				stats.Skipped++
				continue
			}
		}
		if inserted {
			stats.Created++
		} else {
			stats.Updated++
		}
		if strings.EqualFold(o.Status, "suspended") {
			stats.Suspended++
		}
	}

	// Full-snapshot feeds sweep active coupons absent from this run.
	if src.SourceType == FeedSourceGeneric && len(seen) > 0 {
		expired, serr := s.store.expireFeedCouponsNotSeen(ctx, src.ID, seen)
		if serr != nil {
			return stats, fmt.Errorf("absence sweep: %w", serr)
		}
		stats.Expired = int(expired)
	}

	// Imported rows changed — flush the public list cache.
	if stats.Updated > 0 || stats.Expired > 0 {
		s.invalidateCouponCaches(ctx, pgtype.UUID{})
	}

	return stats, nil
}

// fetchFeed GETs the feed endpoint with the source's auth and returns the
// body. couponapi appends API_KEY/format/last_extract query params; generic
// uses an Authorization bearer header when a key is configured.
func (s *Service) fetchFeed(ctx context.Context, src gen.CouponFeedSource) ([]byte, error) {
	var apiKey string
	if src.ApiKeyEncrypted != nil && *src.ApiKeyEncrypted != "" {
		key, err := crypto.Decrypt(*src.ApiKeyEncrypted)
		if err != nil {
			return nil, fmt.Errorf("%w: decrypting api key: %v", ErrFeedFetch, err)
		}
		apiKey = key
	}

	endpoint := src.EndpointUrl
	if src.SourceType == FeedSourceCouponAPI {
		u, err := url.Parse(endpoint)
		if err != nil {
			return nil, fmt.Errorf("%w: bad endpoint_url: %v", ErrFeedFetch, err)
		}
		q := u.Query()
		if apiKey != "" {
			q.Set("API_KEY", apiKey)
		}
		q.Set("format", "json")
		if src.LastSyncedAt.Valid {
			q.Set("last_extract", fmt.Sprintf("%d", src.LastSyncedAt.Time.Unix()))
		}
		u.RawQuery = q.Encode()
		endpoint = u.String()
	}

	reqCtx, cancel := context.WithTimeout(ctx, feedHTTPTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFeedFetch, err)
	}
	if src.SourceType == FeedSourceGeneric && apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "JapaConnect-DealFeed/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFeedFetch, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: upstream returned %d", ErrFeedFetch, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, feedMaxBody))
	if err != nil {
		return nil, fmt.Errorf("%w: reading body: %v", ErrFeedFetch, err)
	}
	return body, nil
}

// decodeFeed normalizes a raw feed body into []feedOffer per source_type.
func decodeFeed(sourceType string, body []byte) ([]feedOffer, error) {
	switch sourceType {
	case FeedSourceCouponAPI:
		var res struct {
			Result bool             `json:"result"`
			Offers []couponAPIOffer `json:"offers"`
		}
		if err := json.Unmarshal(body, &res); err != nil {
			return nil, err
		}
		out := make([]feedOffer, 0, len(res.Offers))
		for _, o := range res.Offers {
			out = append(out, feedOffer{
				ExternalID: o.OfferID.String(), Title: o.Title, Description: o.Description,
				Code: o.Code, Store: o.Store, MerchantURL: o.MerchantURL,
				URL: o.URL, AffiliateLink: o.Affiliate,
				StartDate: o.StartDate, EndDate: o.EndDate, Status: o.Status,
			})
		}
		return out, nil

	case FeedSourceGeneric:
		// Accept {"offers":[...]} or a bare array.
		var wrapped struct {
			Offers []genericOffer `json:"offers"`
		}
		var raw []genericOffer
		if err := json.Unmarshal(body, &wrapped); err == nil && wrapped.Offers != nil {
			raw = wrapped.Offers
		} else if err := json.Unmarshal(body, &raw); err != nil {
			return nil, err
		}
		out := make([]feedOffer, 0, len(raw))
		for _, o := range raw {
			id := o.ID
			if id == "" {
				id = o.OfferID
			}
			out = append(out, feedOffer{
				ExternalID: id, Title: o.Title, Description: o.Description,
				Code: o.Code, Store: o.Store, MerchantURL: o.MerchantURL,
				URL: o.URL, AffiliateLink: o.Affiliate,
				StartDate: o.StartDate, EndDate: o.EndDate, Status: o.Status,
			})
		}
		return out, nil
	}
	return nil, fmt.Errorf("unsupported source_type %q", sourceType)
}

// upsertOffer maps one normalized offer to a coupon row and upserts on
// (feed_id, external_id). Slugs resolve via the shared helper; provider
// resolution honors default_provider_id then store-name match-or-create.
// Returns whether the row was inserted (xmax = 0) vs updated.
func (s *Service) upsertOffer(ctx context.Context, src gen.CouponFeedSource, o feedOffer) (bool, error) {
	// On the update path the INSERT-branch slug is discarded anyway — reuse
	// the existing row's slug rather than resolving a fresh candidate that
	// would suffix against the row itself.
	slug := ""
	if existing, err := s.store.getCouponByFeedExternal(ctx, src.ID, o.ExternalID); err == nil {
		slug = existing.Slug
	} else if !isNoRows(err) {
		return false, fmt.Errorf("checking existing feed coupon: %w", err)
	}
	if slug == "" {
		var err error
		slug, err = s.resolveCouponSlug(ctx, nil, o.Title, pgtype.UUID{})
		if err != nil {
			return false, err
		}
	}

	providerID, err := s.resolveOfferProvider(ctx, src, o)
	if err != nil {
		return false, err
	}

	status := string(CouponStatusActive)
	if strings.EqualFold(o.Status, "suspended") {
		status = string(CouponStatusSuspended)
	}
	expiresAt := parseFeedDate(o.EndDate)
	if expiresAt.Valid && expiresAt.Time.Before(time.Now()) && status == string(CouponStatusActive) {
		status = string(CouponStatusExpired)
	}

	referral := o.AffiliateLink
	if referral == "" {
		referral = o.URL
	}

	row, err := s.store.upsertFeedCoupon(ctx, gen.UpsertFeedCouponParams{
		ProviderID:  providerID,
		Title:       o.Title,
		Slug:        slug,
		Description: strPtrOrNil(o.Description),
		Code:        strPtrOrNil(o.Code),
		ReferralUrl: strPtrOrNil(referral),
		Status:      status,
		ValidFrom:   parseFeedDate(o.StartDate),
		ExpiresAt:   expiresAt,
		FeedID:      src.ID,
		ExternalID:  &o.ExternalID,
	})
	if err != nil {
		return false, err
	}

	// The upsert leaves slug fixed on update; invalidate whichever slug the row has.
	s.invalidateCouponCaches(ctx, row.ID, row.Slug)

	// A feed's default_provider_id can point at an org-linked provider still
	// inactive (providerForOrg). Importing a live offer under it is equivalent
	// to approval — same activation hook as UpdateCoupon/CreateCoupon (§55).
	if status == string(CouponStatusActive) && providerID.Valid {
		s.activateOrgProvider(ctx, providerID)
	}

	return row.Inserted, nil
}

// resolveOfferProvider picks the coupon's provider: the feed's
// default_provider_id wins; otherwise the offer's store is slugified and
// matched against affiliate_providers.slug — a miss auto-creates an active
// provider so its /deal/{provider} page exists (§55).
func (s *Service) resolveOfferProvider(ctx context.Context, src gen.CouponFeedSource, o feedOffer) (pgtype.UUID, error) {
	if src.DefaultProviderID.Valid {
		return src.DefaultProviderID, nil
	}
	if o.Store == "" {
		return pgtype.UUID{}, nil
	}

	base := content.Slugify(o.Store)
	for i := 0; i < maxCouponSlugAttempts; i++ {
		candidate := base
		if i > 0 {
			candidate = fmt.Sprintf("%s-%d", base, i+1)
		}
		id, err := s.store.getProviderIDBySlug(ctx, candidate)
		if err == nil {
			return id, nil
		}
		if !isNoRows(err) {
			return pgtype.UUID{}, fmt.Errorf("matching provider by slug: %w", err)
		}
		// Candidate slug is free — create the provider under it.
		p, err := s.store.createProvider(ctx, gen.CreateAffiliateProviderParams{
			Name:         o.Store,
			Slug:         candidate,
			ProviderType: string(ProviderTypeCoupon),
			BaseUrl:      strPtrOrNil(o.MerchantURL),
			IsActive:     true,
			IsFeatured:   false,
		})
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				// Slug claimed by a concurrent create — retry the next suffix.
				continue
			}
			return pgtype.UUID{}, fmt.Errorf("auto-creating provider %q: %w", o.Store, err)
		}
		return p.ID, nil
	}

	return pgtype.UUID{}, fmt.Errorf("could not resolve provider for store %q", o.Store)
}

// parseFeedDate parses yyyy-mm-dd (or RFC3339) feed dates; empty → invalid.
func parseFeedDate(s string) pgtype.Timestamptz {
	s = strings.TrimSpace(s)
	if s == "" {
		return pgtype.Timestamptz{}
	}
	for _, layout := range []string{"2006-01-02", time.RFC3339, "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return pgtype.Timestamptz{Time: t, Valid: true}
		}
	}
	return pgtype.Timestamptz{}
}

// dedupeCode retries a code that collided with an unrelated coupon by
// appending a short feed-derived suffix (§55: {code}-x{N}).
func dedupeCode(code, feedID string) string {
	if code == "" {
		return ""
	}
	suffix := feedID
	if len(suffix) > 4 {
		suffix = suffix[:4]
	}
	return fmt.Sprintf("%s-x%s", code, suffix)
}

func validSourceType(t string) bool {
	return t == FeedSourceCouponAPI || t == FeedSourceGeneric
}

func validFeedURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != ""
}
