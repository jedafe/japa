package spam

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/valkey"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// --- Request / Response types ---

// SpamCheckRequest carries the inputs for a pipeline evaluation.
type SpamCheckRequest struct { //nolint:revive // stuttering type name preserved for API compatibility
	Content    string      // The text body to evaluate
	UserID     pgtype.UUID // Submitting user
	IP         string      // Client IP address
	UserType   string      // "new" or "existing"
	EntityType string      // "post", "comment", or "message"
}

// SpamCheckResult holds the combined output of all pipeline layers.
type SpamCheckResult struct { //nolint:revive // stuttering type name preserved for API compatibility
	Score   float64  // 0.0 = clean, 1.0 = definitely spam
	Blocked bool     // score >= blockThreshold
	Flagged bool     // score >= flagThreshold
	Reasons []string // human-readable reasons from every layer that fired
}

// --- Thresholds ---

const (
	// blockThreshold is the score at or above which content is rejected.
	blockThreshold = 0.7
	// flagThreshold is the score at or above which content is held for
	// moderation review.
	flagThreshold = 0.4
)

// --- Default layer weights ---

// DefaultWeights maps layer names to their contribution to the final score.
// A weight of 1.0 means the layer's score is added directly; 0.5 means it
// contributes half, etc. Weights are normalised so the worst-case score
// across all layers sums to 1.0.
var DefaultWeights = map[string]float64{
	"ip_reputation":  0.25,
	"rate_limit":     0.20,
	"content_hash":   0.20,
	"censored_words": 0.15,
	"link_analysis":  0.10,
	"user_age":       0.05,
	"content_length": 0.05,
}

// --- Pipeline ---

// SpamPipeline chains multiple detection layers and returns a single score.
type SpamPipeline struct { //nolint:revive // stuttering type name preserved for API compatibility
	store    *Store
	rdb      *valkey.Client
	db       *db.DB
	settings spamSettingsReader
}

// spamSettingsReader is the subset of config.Settings the spam pipeline uses
// for hot-reloadable moderation thresholds. Local interface to avoid import cycles.
type spamSettingsReader interface {
	GetIntOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback int) int
}

// SetSettings injects the hot-reloadable settings reader for spam thresholds.
func (p *SpamPipeline) SetSettings(s spamSettingsReader) { p.settings = s }

// NewPipeline creates a pipeline backed by the given dependencies.
func NewPipeline(store *Store, rdb *valkey.Client, database *db.DB) *SpamPipeline {
	return &SpamPipeline{store: store, rdb: rdb, db: database}
}

// Check runs every detection layer, collects reasons, computes the weighted
// score, and returns the final result. All layers execute even if an early
// layer would block — this ensures moderators see every contributing factor.
func (p *SpamPipeline) Check(ctx context.Context, tenantID pgtype.UUID, req SpamCheckRequest) (SpamCheckResult, error) {
	// Guard: nil pipeline
	if p == nil {
		return SpamCheckResult{}, nil
	}

	type layerResult struct {
		name    string
		score   float64
		reasons []string
	}

	layers := []func(ctx context.Context, tenantID pgtype.UUID, req SpamCheckRequest) (string, float64, []string){
		p.checkIPReputation,
		p.checkRateLimit,
		p.checkContentHash,
		p.checkCensoredWords,
		p.checkLinkAnalysis,
		p.checkUserAge,
		p.checkContentLength,
	}

	var results []layerResult

	for _, layer := range layers {
		name, score, reasons := layer(ctx, tenantID, req)
		results = append(results, layerResult{name: name, score: score, reasons: reasons})
	}

	// Aggregate: weighted sum across layers.
	var totalScore float64

	var allReasons []string

	for _, r := range results {
		weight := DefaultWeights[r.name]
		totalScore += r.score * weight
		allReasons = append(allReasons, r.reasons...)
	}

	// Clamp to [0, 1] — weights are designed so this stays in range, but
	// guard against misconfiguration.
	if totalScore > 1.0 {
		totalScore = 1.0
	}

	return SpamCheckResult{
		Score:   totalScore,
		Blocked: totalScore >= blockThreshold,
		Flagged: totalScore >= flagThreshold,
		Reasons: allReasons,
	}, nil
}

// --- Individual layers ---

// checkIPReputation checks whether the IP falls within a blocked CIDR range.
func (p *SpamPipeline) checkIPReputation(ctx context.Context, tenantID pgtype.UUID, req SpamCheckRequest) (string, float64, []string) {
	const name = "ip_reputation"

	if req.IP == "" {
		return name, 0.0, nil
	}

	blocked, err := p.store.IsIPBlocked(ctx, tenantID, req.IP)
	if err != nil {
		slog.Warn("ip reputation check failed", "ip", req.IP, "error", err)
		// Degrade: treat unknown as low risk, not zero.
		return name, 0.1, []string{"ip reputation check unavailable"}
	}

	if blocked {
		return name, 1.0, []string{"IP is in blocked range"}
	}

	return name, 0.0, nil
}

// rateLimitKey builds the Valkey key for a user's content creation rate.
func rateLimitKey(tenantID, userID pgtype.UUID) string {
	return fmt.Sprintf("jc:spam:rate:%s:%s", tenantID.String(), userID.String())
}

// checkRateLimit counts recent content creations via a Valkey sorted set.
// Users with more than the threshold in the window receive a higher score.
func (p *SpamPipeline) checkRateLimit(ctx context.Context, tenantID pgtype.UUID, req SpamCheckRequest) (string, float64, []string) {
	const name = "rate_limit"

	if req.UserID == (pgtype.UUID{}) || p.rdb == nil {
		return name, 0.0, nil
	}

	key := rateLimitKey(tenantID, req.UserID)
	now := time.Now()
	windowStart := now.Add(-10 * time.Minute)

	// Remove entries outside the 10-minute window, then count.
	rdb := p.rdb.Redis()
	_ = rdb.ZRemRangeByScore(ctx, key, "0", strconv.FormatInt(windowStart.UnixMilli(), 10))

	count, err := rdb.ZCard(ctx, key).Result()
	if err != nil {
		slog.Warn("rate limit check failed", "user_id", req.UserID, "error", err)
		return name, 0.0, nil
	}

	// Thresholds: >10 posts in 10 min → high, >5 → medium, else zero.
	switch {
	case count > 10:
		return name, 0.8, []string{fmt.Sprintf("rate limit: %d posts in 10 min (high)", count)}
	case count > 5:
		return name, 0.4, []string{fmt.Sprintf("rate limit: %d posts in 10 min (medium)", count)}
	default:
		return name, 0.0, nil
	}
}

// RecordContentCreation adds a timestamp to the user's rate-limit sorted set.
// Call this after a content item is successfully persisted.
func RecordContentCreation(ctx context.Context, rdb *valkey.Client, tenantID, userID pgtype.UUID) error {
	if rdb == nil {
		return nil
	}

	key := rateLimitKey(tenantID, userID)
	member := time.Now().Format(time.RFC3339Nano)
	score := float64(time.Now().UnixMilli())

	rClient := rdb.Redis()

	if err := rClient.ZAdd(ctx, key, valkeycompat.Z{Score: score, Member: member}).Err(); err != nil {
		return err
	}

	// Attach a 1-hour TTL so the sorted set expires after the 10-min
	// sliding window plus ample buffer.
	if err := rClient.Expire(ctx, key, time.Hour).Err(); err != nil {
		return fmt.Errorf("setting spam rate TTL: %w", err)
	}

	return nil
}

// checkContentHash looks for duplicate content by comparing the SHA-256 hash
// against previously stored hashes in the contents table.
func (p *SpamPipeline) checkContentHash(ctx context.Context, tenantID pgtype.UUID, req SpamCheckRequest) (string, float64, []string) {
	const name = "content_hash"

	if req.Content == "" {
		return name, 0.0, nil
	}

	hash := ComputeContentHash(req.Content)

	duplicate, err := p.store.CheckDuplicateHash(ctx, tenantID, hash)
	if err != nil {
		slog.Warn("content hash check failed", "error", err)
		return name, 0.0, nil
	}

	if duplicate {
		return name, 0.9, []string{"duplicate content detected (exact hash match)"}
	}

	return name, 0.0, nil
}

// checkCensoredWords runs the content against the tenant's censored word list.
func (p *SpamPipeline) checkCensoredWords(ctx context.Context, tenantID pgtype.UUID, req SpamCheckRequest) (string, float64, []string) {
	const name = "censored_words"

	if req.Content == "" {
		return name, 0.0, nil
	}

	words, err := GetCachedCensoredWords(ctx, p.rdb, p.db, tenantID)
	if err != nil || len(words) == 0 {
		return name, 0.0, nil
	}

	result := CheckCensoredWords(words, req.Content)
	if result.Blocked {
		return name, 1.0, []string{"censored word blocked: " + result.FlagReason}
	}

	if result.Flagged {
		return name, 0.6, []string{"censored word flagged: " + result.FlagReason}
	}

	return name, 0.0, nil
}

// maxLinksPerPost is the number of URLs above which content is considered
// link spam. Posts with more links than text are a classic spam signal.
const maxLinksPerPost = 3

// checkLinkAnalysis counts HTTP(S) URLs in the content. Posts with many
// links relative to their length are a strong spam signal.
func (p *SpamPipeline) checkLinkAnalysis(ctx context.Context, tenantID pgtype.UUID, req SpamCheckRequest) (string, float64, []string) {
	const name = "link_analysis"

	if req.Content == "" {
		return name, 0.0, nil
	}

	maxLinks := maxLinksPerPost
	if p.settings != nil {
		maxLinks = p.settings.GetIntOr(ctx, tenantID, "moderation.max_links_per_post", maxLinksPerPost)
	}

	linkCount := countLinks(req.Content)
	wordCount := len(strings.Fields(req.Content))

	switch {
	case linkCount > maxLinks:
		return name, 0.7, []string{fmt.Sprintf("excessive links: %d links in %d words", linkCount, wordCount)}
	case linkCount > 0 && wordCount > 0 && float64(linkCount)/float64(wordCount) > 0.3:
		return name, 0.5, []string{fmt.Sprintf("high link density: %d links in %d words", linkCount, wordCount)}
	default:
		return name, 0.0, nil
	}
}

// countLinks extracts and counts HTTP(S) URLs from text.
func countLinks(text string) int {
	count := 0

	for _, word := range strings.Fields(text) {
		u, err := url.Parse(word)
		if err != nil {
			continue
		}

		scheme := strings.ToLower(u.Scheme)
		if scheme == "http" || scheme == "https" {
			count++
		}
	}

	return count
}

// newAccountThreshold is the age below which a user is considered "new" and
// receives a higher base spam score.
const newAccountThreshold = 7 * 24 * time.Hour // 7 days

// checkUserAge returns a score for new accounts. New accounts (< 7 days old)
// are more likely to be spam bots.
func (p *SpamPipeline) checkUserAge(ctx context.Context, tenantID pgtype.UUID, req SpamCheckRequest) (string, float64, []string) {
	const name = "user_age"

	if req.UserID == (pgtype.UUID{}) {
		return name, 0.0, nil
	}

	// If caller already classified the user as new, skip the DB lookup.
	if req.UserType == "new" {
		return name, 0.3, []string{"new user account (caller-reported)"}
	}

	if p.db == nil {
		return name, 0.0, nil
	}

	queries := gen.New(p.db.Reader)

	user, err := queries.GetUserByID(ctx, gen.GetUserByIDParams{
		TenantID: tenantID,
		UserID:   req.UserID,
	})
	if err != nil {
		slog.Warn("user age check failed", "user_id", req.UserID, "error", err)
		return name, 0.0, nil
	}

	if !user.CreatedAt.Valid {
		return name, 0.0, nil
	}

	age := time.Since(user.CreatedAt.Time)
	threshold := newAccountThreshold
	if p.settings != nil {
		days := p.settings.GetIntOr(ctx, tenantID, "moderation.new_account_threshold_days", 7)
		threshold = time.Duration(days) * 24 * time.Hour
	}
	if age < threshold {
		return name, 0.3, []string{fmt.Sprintf("new account: %s old", age.Round(time.Hour))}
	}

	return name, 0.0, nil
}

// minContentLength is the character count below which content is flagged as
// potentially low-quality or bot-generated.
const minContentLength = 10

// checkContentLength flags very short content. Single-word or near-empty
// posts are a common spam signal (especially combined with other layers).
func (p *SpamPipeline) checkContentLength(ctx context.Context, tenantID pgtype.UUID, req SpamCheckRequest) (string, float64, []string) {
	const name = "content_length"

	length := len(strings.TrimSpace(req.Content))
	if length == 0 {
		return name, 0.2, []string{"empty content"}
	}

	minLen := minContentLength
	if p.settings != nil {
		minLen = p.settings.GetIntOr(ctx, tenantID, "content.min_length", minContentLength)
	}

	if length < minLen {
		return name, 0.3, []string{fmt.Sprintf("very short content: %d chars", length)}
	}

	return name, 0.0, nil
}
