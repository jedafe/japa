package feed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/media"
	"github.com/jedafe/japa/internal/worker"
)

// FeedItem represents a single item in a feed response, normalized across
// all feed types (explore, primary, space, profile).
type FeedItem struct { //nolint:revive // stuttering type name preserved for API compatibility
	ID                 string   `json:"id"`
	Title              string   `json:"title"`
	Slug               string   `json:"slug"`
	ContentType        string   `json:"content_type"`
	AuthorID           string   `json:"author_id"`
	SpaceID            *string  `json:"space_id,omitempty"`
	SpaceName          string   `json:"space_name,omitempty"`
	SpaceSlug          string   `json:"space_slug,omitempty"`
	CreatedAt          string   `json:"created_at"`
	ViewCount          int32    `json:"view_count"`
	CommentCount       int32    `json:"comment_count"`
	VoteScore          int32    `json:"vote_score"`
	VelocityScore      float64  `json:"velocity_score"`
	AuthorName         string   `json:"author_name,omitempty"`
	AuthorAvatarURL    string   `json:"author_avatar_url,omitempty"`
	Excerpt            string   `json:"excerpt,omitempty"`
	Tags               []string `json:"tags,omitempty"`
	ReadingTimeMinutes int      `json:"reading_time_minutes,omitempty"`
	CoverImageURL      string   `json:"cover_image_url,omitempty"`
	BookmarkCount      int32    `json:"bookmark_count,omitempty"`
}

// PrimaryFeedItem represents a feed item from the personalized (push) feed.
type PrimaryFeedItem struct {
	ID               string  `json:"id"`
	ContentID        string  `json:"content_id"`
	SpaceID          *string `json:"space_id,omitempty"`
	ContentType      string  `json:"content_type"`
	Score            float64 `json:"score"`
	CreatedAt        string  `json:"created_at"`
	Title            string  `json:"title"`
	Slug             string  `json:"slug"`
	AuthorID         string  `json:"author_id"`
	ContentCreatedAt string  `json:"content_created_at"`
}

// FanOutPost distributes a post to follower feeds asynchronously via Asynq.
// If space member count ≤ FanOutThreshold: push mode (write feeds rows).
// If larger: pull mode (mark in hot cache, followers pull on next request).
// The fan-out is handled by the worker.TypeContentFanOutToFeeds task.
func (s *Service) FanOutPost(_ context.Context, tenantID, contentID, spaceID pgtype.UUID, contentType string) error {
	// Determine fan-out type based on space membership.
	fanOutType := "followers" // default: fan out to author's followers
	if spaceID.Valid {
		fanOutType = "space" // space posts go to all space members
	}

	// Enqueue the fan-out task for async processing.
	// The worker will handle the actual fan-out logic.
	payload, err := json.Marshal(map[string]string{
		"content_id":   contentID.String(),
		"tenant_id":    tenantID.String(),
		"space_id":     spaceID.String(),
		"content_type": contentType,
		"fan_out_type": fanOutType,
	})
	if err != nil {
		return fmt.Errorf("marshaling fan-out payload: %w", err)
	}

	if s.enqueuer != nil {
		if err := s.enqueuer.Enqueue(worker.TypeContentFanOutToFeeds, payload); err != nil {
			return fmt.Errorf("enqueueing fan-out task: %w", err)
		}
		return nil
	}

	slog.Info("fan_out_post: no enqueuer wired, skipping fan-out",
		"content_id", contentID.String(),
		"fan_out_type", fanOutType)

	return nil
}

// GetFeed returns paginated feed items for the specified timeline and sort.
// timeline: "explore", "primary", "space", "profile"
// sort: "latest", "top", "rising", "relevant"
func (s *Service) GetFeed(ctx context.Context, tenantID pgtype.UUID, timeline string, userID, spaceID, authorID *pgtype.UUID, sort string, limit, offset int32) ([]FeedItem, error) {
	// Clamp pagination limits.
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	if offset < 0 {
		offset = 0
	}

	switch timeline {
	case "explore":
		return s.getExploreFeed(ctx, tenantID, sort, limit, offset)
	case "primary":
		if userID == nil {
			return nil, errors.New("primary feed requires user_id")
		}

		return s.getPrimaryFeedItems(ctx, tenantID, *userID, sort, limit, offset)
	case "space":
		if spaceID == nil {
			return nil, errors.New("space feed requires space_id")
		}

		return s.getSpaceFeedItems(ctx, tenantID, *spaceID, sort, limit, offset)
	case "profile":
		if authorID == nil {
			return nil, errors.New("profile feed requires author_id")
		}

		return s.getProfileFeedItems(ctx, tenantID, *authorID, sort, limit, offset)
	default:
		return nil, fmt.Errorf("unknown timeline: %s", timeline)
	}
}

// getExploreFeed returns the explore feed with the given sort mode.
func (s *Service) getExploreFeed(ctx context.Context, tenantID pgtype.UUID, sort string, limit, offset int32) ([]FeedItem, error) {
	switch sort {
	case SortTop:
		rows, err := s.store.getTopFeed(ctx, tenantID, limit, offset)
		if err != nil {
			return nil, err
		}

		return topRowsToItems(rows), nil
	case SortRising:
		rows, err := s.store.getRisingFeed(ctx, tenantID, limit, offset)
		if err != nil {
			return nil, err
		}

		return risingRowsToItems(rows), nil
	case SortRelevant:
		// Explore is guest-facing — there is no user interest embedding
		// available for pgvector cosine similarity (Section 32 L120–125
		// requires a $user_embedding). Fall back to the top feed (7-day
		// rolling window) as the closest relevance proxy for guests.
		// The for-you feed handles true pgvector relevance for authed users.
		rows, err := s.store.getTopFeed(ctx, tenantID, limit, offset)
		if err != nil {
			return nil, err
		}

		return topRowsToItems(rows), nil
	default: // "latest" uses velocity_score DESC
		rows, err := s.store.getExploreFeed(ctx, tenantID, limit, offset)
		if err != nil {
			return nil, err
		}

		return exploreRowsToItems(rows), nil
	}
}

// getPrimaryFeedItems returns the user's personalized feed with sort.
// sort: 'latest' (default), 'top' (vote_score DESC), 'rising' (velocity_score DESC).
// 'relevant' falls back to latest (pgvector relevance is only in the for-you feed).
func (s *Service) getPrimaryFeedItems(ctx context.Context, tenantID, userID pgtype.UUID, sort string, limit, offset int32) ([]FeedItem, error) {
	rows, err := s.store.getPrimaryFeed(ctx, tenantID, userID, sort, limit, offset)
	if err != nil {
		return nil, err
	}

	return primaryRowsToItems(rows), nil
}

// getSpaceFeedItems returns space-specific feed with sort.
// sort: 'latest' (default), 'top' (vote_score DESC), 'rising' (velocity_score DESC).
// 'relevant' falls back to latest (pgvector relevance is only in the for-you feed).
func (s *Service) getSpaceFeedItems(ctx context.Context, tenantID, spaceID pgtype.UUID, sort string, limit, offset int32) ([]FeedItem, error) {
	rows, err := s.store.getSpaceFeed(ctx, tenantID, spaceID, sort, limit, offset)
	if err != nil {
		return nil, err
	}

	return spaceRowsToItems(rows), nil
}

// getProfileFeedItems returns a user's public posts with sort.
// sort: 'latest' (default), 'top' (vote_score DESC), 'rising' (velocity_score DESC).
// 'relevant' falls back to latest (pgvector relevance is only in the for-you feed).
func (s *Service) getProfileFeedItems(ctx context.Context, tenantID, authorID pgtype.UUID, sort string, limit, offset int32) ([]FeedItem, error) {
	rows, err := s.store.getProfileFeed(ctx, tenantID, authorID, sort, limit, offset)
	if err != nil {
		return nil, err
	}

	return profileRowsToItems(rows), nil
}

// UpdateVelocityScores batch updates velocity scores for published content
// < 30 days old. gravity is the decay exponent (default 1.8), hot-reloadable
// via the feed.velocity.gravity system setting.
func (s *Service) UpdateVelocityScores(ctx context.Context, tenantID pgtype.UUID, gravity float64) error {
	return s.store.updateVelocityScores(ctx, tenantID, gravity)
}

// UpdateContentScore updates a single content's score.
func (s *Service) UpdateContentScore(ctx context.Context, contentID pgtype.UUID, voteScore, commentCount, viewCount, bookmarkCount int32) error {
	return s.store.updateContentScore(ctx, gen.UpdateContentScoreParams{
		ContentID:     contentID,
		VoteScore:     voteScore,
		CommentCount:  commentCount,
		ViewCount:     viewCount,
		BookmarkCount: bookmarkCount,
	})
}

// DeleteFeedItemsByContent removes all feed rows for a content ID (tenant-scoped).
func (s *Service) DeleteFeedItemsByContent(ctx context.Context, tenantID, contentID pgtype.UUID) error {
	return s.store.deleteFeedItemsByContent(ctx, tenantID, contentID)
}

// DeleteFeedItemsByUser removes all feed rows for a user (tenant-scoped).
func (s *Service) DeleteFeedItemsByUser(ctx context.Context, tenantID, userID pgtype.UUID) error {
	return s.store.deleteFeedItemsByUser(ctx, tenantID, userID)
}

// GetFeedItemCount returns the total number of feed items for a user (tenant-scoped).
func (s *Service) GetFeedItemCount(ctx context.Context, tenantID, userID pgtype.UUID) (int, error) {
	count, err := s.store.getFeedItemCount(ctx, tenantID, userID)
	if err != nil {
		return 0, err
	}

	return int(count), nil
}

// ---------------------------------------------------------------------------
// Row-to-item converters
// ---------------------------------------------------------------------------

// uuidToString converts a pgtype.UUID to its string representation.
func uuidToString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}

	return u.String()
}

// nullableUUIDToString converts a nullable pgtype.UUID to *string.
func nullableUUIDToString(u pgtype.UUID) *string {
	if !u.Valid {
		return nil
	}

	s := u.String()

	return &s
}

// splitTags splits a comma-separated tag string into a slice.
// Handles interface{} from sqlc's string_agg which may be nil or string.
func splitTags(v interface{}) []string {
	if v == nil {
		return nil
	}

	s, ok := v.(string)
	if !ok || s == "" {
		return nil
	}

	return strings.Split(s, ",")
}

// derefInt32 safely dereferences a nullable int32 pointer, returning 0 if nil.
func derefInt32(p *int32) int32 {
	if p == nil {
		return 0
	}

	return *p
}

// derefStr safely dereferences a *string, returning "" for nil.
func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// coverImageURL resolves the cover image URL from storage bucket and path.
func coverImageURL(bucket, path *string) string {
	if bucket == nil || path == nil || *bucket == "" || *path == "" {
		return ""
	}

	return media.MediaURL(*bucket, *path, media.ImageProxyOpts{})
}

func exploreRowsToItems(rows []gen.GetExploreFeedRow) []FeedItem {
	items := make([]FeedItem, len(rows))
	for i, r := range rows {
		items[i] = FeedItem{
			ID:                 uuidToString(r.ID),
			Title:              r.Title,
			Slug:               r.Slug,
			ContentType:        r.ContentType,
			AuthorID:           uuidToString(r.AuthorID),
			SpaceID:            nullableUUIDToString(r.SpaceID),
			CreatedAt:          r.CreatedAt.Time.String(),
			ViewCount:          r.ViewCount,
			CommentCount:       r.CommentCount,
			VoteScore:          r.VoteScore,
			VelocityScore:      pgNumericToFloat64(r.VelocityScore),
			AuthorName:         r.AuthorName,
			AuthorAvatarURL:    r.AuthorAvatarUrl,
			SpaceName:          r.SpaceName,
			SpaceSlug:          r.SpaceSlug,
			Excerpt:            r.Excerpt,
			Tags:               splitTags(r.Tags),
			ReadingTimeMinutes: int(r.ReadTimeMinutes),
			CoverImageURL:      coverImageURL(r.CoverStorageBucket, r.CoverStoragePath),
			BookmarkCount:      r.BookmarkCount,
		}
	}

	return items
}

func topRowsToItems(rows []gen.GetTopFeedRow) []FeedItem {
	items := make([]FeedItem, len(rows))
	for i, r := range rows {
		items[i] = FeedItem{
			ID:                 uuidToString(r.ID),
			Title:              r.Title,
			Slug:               r.Slug,
			ContentType:        r.ContentType,
			AuthorID:           uuidToString(r.AuthorID),
			SpaceID:            nullableUUIDToString(r.SpaceID),
			CreatedAt:          r.CreatedAt.Time.String(),
			ViewCount:          r.ViewCount,
			CommentCount:       r.CommentCount,
			VoteScore:          r.VoteScore,
			VelocityScore:      pgNumericToFloat64(r.VelocityScore),
			AuthorName:         r.AuthorName,
			AuthorAvatarURL:    r.AuthorAvatarUrl,
			SpaceName:          r.SpaceName,
			SpaceSlug:          r.SpaceSlug,
			Excerpt:            r.Excerpt,
			Tags:               splitTags(r.Tags),
			ReadingTimeMinutes: int(r.ReadTimeMinutes),
			CoverImageURL:      coverImageURL(r.CoverStorageBucket, r.CoverStoragePath),
			BookmarkCount:      r.BookmarkCount,
		}
	}

	return items
}

func risingRowsToItems(rows []gen.GetRisingFeedRow) []FeedItem {
	items := make([]FeedItem, len(rows))
	for i, r := range rows {
		items[i] = FeedItem{
			ID:                 uuidToString(r.ID),
			Title:              r.Title,
			Slug:               r.Slug,
			ContentType:        r.ContentType,
			AuthorID:           uuidToString(r.AuthorID),
			SpaceID:            nullableUUIDToString(r.SpaceID),
			CreatedAt:          r.CreatedAt.Time.String(),
			ViewCount:          r.ViewCount,
			CommentCount:       r.CommentCount,
			VoteScore:          r.VoteScore,
			VelocityScore:      pgNumericToFloat64(r.VelocityScore),
			AuthorName:         r.AuthorName,
			AuthorAvatarURL:    r.AuthorAvatarUrl,
			SpaceName:          r.SpaceName,
			SpaceSlug:          r.SpaceSlug,
			Excerpt:            r.Excerpt,
			Tags:               splitTags(r.Tags),
			ReadingTimeMinutes: int(r.ReadTimeMinutes),
			CoverImageURL:      coverImageURL(r.CoverStorageBucket, r.CoverStoragePath),
			BookmarkCount:      r.BookmarkCount,
		}
	}

	return items
}

func primaryRowsToItems(rows []gen.GetPrimaryFeedRow) []FeedItem {
	items := make([]FeedItem, len(rows))
	for i, r := range rows {
		items[i] = FeedItem{
			ID:                 uuidToString(r.ContentID),
			Title:              r.Title,
			Slug:               r.Slug,
			ContentType:        r.ContentType,
			AuthorID:           uuidToString(r.AuthorID),
			SpaceID:            nullableUUIDToString(r.SpaceID),
			CreatedAt:          r.ContentCreatedAt.Time.String(),
			ViewCount:          r.ViewCount,
			CommentCount:       r.CommentCount,
			VoteScore:          derefInt32(r.VoteScore),
			AuthorName:         r.AuthorName,
			AuthorAvatarURL:    r.AuthorAvatarUrl,
			SpaceName:          r.SpaceName,
			SpaceSlug:          r.SpaceSlug,
			Excerpt:            r.Excerpt,
			Tags:               splitTags(r.Tags),
			ReadingTimeMinutes: int(r.ReadTimeMinutes),
			CoverImageURL:      coverImageURL(r.CoverStorageBucket, r.CoverStoragePath),
			BookmarkCount:      derefInt32(r.BookmarkCount),
		}
	}

	return items
}

func spaceRowsToItems(rows []gen.GetSpaceFeedRow) []FeedItem {
	items := make([]FeedItem, len(rows))
	for i, r := range rows {
		items[i] = FeedItem{
			ID:                 uuidToString(r.ID),
			Title:              r.Title,
			Slug:               r.Slug,
			ContentType:        r.ContentType,
			AuthorID:           uuidToString(r.AuthorID),
			CreatedAt:          r.CreatedAt.Time.String(),
			ViewCount:          r.ViewCount,
			CommentCount:       r.CommentCount,
			VoteScore:          r.VoteScore,
			VelocityScore:      pgNumericToFloat64(r.VelocityScore),
			AuthorName:         r.AuthorName,
			AuthorAvatarURL:    r.AuthorAvatarUrl,
			SpaceName:          r.SpaceName,
			SpaceSlug:          r.SpaceSlug,
			Excerpt:            r.Excerpt,
			Tags:               splitTags(r.Tags),
			ReadingTimeMinutes: int(r.ReadTimeMinutes),
			CoverImageURL:      coverImageURL(r.CoverStorageBucket, r.CoverStoragePath),
			BookmarkCount:      r.BookmarkCount,
		}
	}

	return items
}

func profileRowsToItems(rows []gen.GetProfileFeedRow) []FeedItem {
	items := make([]FeedItem, len(rows))
	for i, r := range rows {
		items[i] = FeedItem{
			ID:                 uuidToString(r.ID),
			Title:              r.Title,
			Slug:               r.Slug,
			ContentType:        r.ContentType,
			AuthorID:           uuidToString(r.AuthorID),
			SpaceID:            nullableUUIDToString(r.SpaceID),
			CreatedAt:          r.CreatedAt.Time.String(),
			ViewCount:          r.ViewCount,
			CommentCount:       r.CommentCount,
			VoteScore:          r.VoteScore,
			VelocityScore:      pgNumericToFloat64(r.VelocityScore),
			AuthorName:         r.AuthorName,
			AuthorAvatarURL:    r.AuthorAvatarUrl,
			SpaceName:          r.SpaceName,
			SpaceSlug:          r.SpaceSlug,
			Excerpt:            r.Excerpt,
			Tags:               splitTags(r.Tags),
			ReadingTimeMinutes: int(r.ReadTimeMinutes),
			CoverImageURL:      coverImageURL(r.CoverStorageBucket, r.CoverStoragePath),
			BookmarkCount:      r.BookmarkCount,
		}
	}

	return items
}

// ComputeVelocity calculates the velocity score for a post.
// This Go helper mirrors the SQL formula in feeds.sql (UpdateVelocityScores)
// so the cron-written content_scores.velocity_score and any in-process
// computation agree. Formula:
//   (vote_score*2 + comment_count + view_count/100) / hours^gravity
// Ref: japaconnect-engineering/sections/32-activity-feed-timeline.md
func ComputeVelocity(voteScore, commentCount, viewCount, bookmarkCount int32, hoursOld float64) float64 {
	// Match the SQL formula: vote*2 + comment + view/100.
	// bookmark_count is not in the SQL formula; kept as a param for API compat.
	engagement := float64(voteScore*2) + float64(commentCount) + float64(viewCount)/100.0

	if hoursOld <= 0 {
		hoursOld = 1 // avoid division by zero for very fresh posts
	}
	denominator := math.Pow(hoursOld, 1.8)
	if denominator == 0 {
		return 0
	}

	return engagement / denominator
}

// pgNumericToFloat64 converts pgtype.Numeric to float64.
func pgNumericToFloat64(n pgtype.Numeric) float64 {
	if !n.Valid {
		return 0
	}

	f, err := n.Float64Value()
	if err != nil {
		return 0
	}

	return f.Float64
}

// For-You feed bounds and cache tuning (v1 constants).
const (
	forYouWindowDays   = 30
	forYouAffinityCap  = 300
	forYouVelocityCap  = 300
	forYouCandidateCap = 600
	forYouCacheTTL     = 90 * time.Second
)

// loadAffinity fetches the user's followed authors, spaces (followed ∪ member),
// and followed tags as ID arrays for the candidate query.
func (s *Service) loadAffinity(
	ctx context.Context, tenantID, userID pgtype.UUID,
) (authorIDs, spaceIDs, tagIDs []pgtype.UUID, err error) {
	authorIDs, err = s.store.getFollowedEntityIDs(ctx, tenantID, userID, "user")
	if err != nil {
		return nil, nil, nil, err
	}

	followedSpaces, err := s.store.getFollowedEntityIDs(ctx, tenantID, userID, "space")
	if err != nil {
		return nil, nil, nil, err
	}

	memberSpaces, err := s.store.getMemberSpaceIDs(ctx, tenantID, userID)
	if err != nil {
		return nil, nil, nil, err
	}

	tagIDs, err = s.store.getFollowedEntityIDs(ctx, tenantID, userID, "tag")
	if err != nil {
		return nil, nil, nil, err
	}

	spaceIDs = dedupeUUIDs(append(followedSpaces, memberSpaces...))

	// Cap the number of spaces contributing to the feed to prevent a user
	// with thousands of followed spaces from making the candidate query
	// unbounded. Hot-reloadable via feed.max_followed_spaces_in_feed.
	if s.settings != nil {
		maxSpaces := s.settings.GetIntOr(ctx, tenantID, "feed.max_followed_spaces_in_feed", 50)
		if maxSpaces > 0 && len(spaceIDs) > maxSpaces {
			spaceIDs = spaceIDs[:maxSpaces]
		}
	}

	return authorIDs, spaceIDs, tagIDs, nil
}

// GetForYouFeed returns the personalized, ranked feed for a user, cached 90s per sort.
// contentCountry is the visitor's resolved ContentCountry (ADR-064) used for
// the bounded geo boost in the ranking; empty string disables geo weighting.
// The geo boost is also disabled when the tenant setting
// geo_personalization.enabled is false (Section 32 L147–149).
func (s *Service) GetForYouFeed(
	ctx context.Context, tenantID, userID pgtype.UUID, sortMode string, limit, offset int32,
	contentCountry string,
) ([]FeedItem, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	if offset < 0 {
		offset = 0
	}

	// BUG-007: honor geo_personalization.enabled. When disabled, clear
	// contentCountry so no geo boost is applied. Ref: Section 32 L147–149.
	if s.settings != nil && contentCountry != "" {
		if !s.settings.GetBoolOr(ctx, tenantID, "geo_personalization.enabled", true) {
			contentCountry = ""
		}
	}

	key := forYouCacheKey(tenantID, userID, sortMode, contentCountry)

	// Cache hit → slice and return.
	if ranked, ok := s.readForYouCache(ctx, key); ok {
		return pageSlice(ranked, offset, limit), nil
	}

	// Miss → load affinity, fetch the bounded candidate pool.
	authorIDs, spaceIDs, tagIDs, err := s.loadAffinity(ctx, tenantID, userID)
	if err != nil {
		return nil, err
	}

	var windowStart pgtype.Timestamptz

	_ = windowStart.Scan(time.Now().Add(-forYouWindowDays * 24 * time.Hour))

	rows, err := s.store.getForYouCandidates(ctx, gen.GetForYouCandidatesParams{
		TenantID:        tenantID,
		WindowStart:     windowStart,
		ExcludeAuthorID: userID,
		AuthorIds:       authorIDs,
		SpaceIds:        spaceIDs,
		TagIds:          tagIDs,
		AffinityLimit:   forYouAffinityCap,
		VelocityLimit:   forYouVelocityCap,
		CandidateCap:    forYouCandidateCap,
	})
	if err != nil {
		return nil, err
	}

	// Score + rank in pure Go, then cache the ranked list (best-effort).
	cands := make([]Candidate, len(rows))
	for i, r := range rows {
		cands[i] = candidateFromRow(r)
	}

	ranked := make([]FeedItem, 0, len(cands))
	for _, c := range rankCandidates(cands, sortMode, time.Now(), contentCountry) {
		ranked = append(ranked, c.Item)
	}

	s.writeForYouCache(ctx, key, ranked)

	return pageSlice(ranked, offset, limit), nil
}

// readForYouCache returns the cached ranked feed and whether the lookup hit.
func (s *Service) readForYouCache(ctx context.Context, key string) ([]FeedItem, bool) {
	if s.vk == nil {
		return nil, false
	}

	data, err := s.vk.Get(ctx, key).Bytes()
	if err != nil || len(data) == 0 {
		return nil, false
	}

	var ranked []FeedItem

	if err := json.Unmarshal(data, &ranked); err != nil {
		return nil, false
	}

	return ranked, true
}

// writeForYouCache stores the ranked feed for forYouCacheTTL. Best-effort:
// cache write failures never fail the request.
func (s *Service) writeForYouCache(ctx context.Context, key string, ranked []FeedItem) {
	if s.vk == nil {
		return
	}

	data, err := json.Marshal(ranked)
	if err != nil {
		return
	}

	_ = s.vk.Set(ctx, key, data, forYouCacheTTL).Err()
}
