package feed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/media"
	vkcache "github.com/jedafe/japa/internal/valkey"
	"github.com/jedafe/japa/internal/visitor"
	"github.com/jedafe/japa/internal/worker"
)

// FeedItem represents a single item in a feed response, normalized across
// all feed types (explore, primary, space, profile) for the daily card.
// Ref: japaconnect-engineering/sections/32-activity-feed-timeline.md
type FeedItem struct { //nolint:revive // stuttering type name preserved for API compatibility
	ID                 string   `json:"id"`
	Title              string   `json:"title"`
	Slug               string   `json:"slug"`
	ContentType        string   `json:"content_type"`
	AuthorID           string   `json:"author_id"`
	AuthorUsername     string   `json:"author_username,omitempty"`
	AuthorName         string   `json:"author_name,omitempty"`
	AuthorAvatarURL    string   `json:"author_avatar_url,omitempty"`
	SpaceID            *string  `json:"space_id,omitempty"`
	SpaceName          string   `json:"space_name,omitempty"`
	SpaceSlug          string   `json:"space_slug,omitempty"`
	SpaceAvatarURL     string   `json:"space_avatar_url,omitempty"`
	Organization       *OrgRef  `json:"organization,omitempty"`
	CreatedAt          string   `json:"created_at"`
	ViewCount          int32    `json:"view_count"`
	CommentCount       int32    `json:"comment_count"`
	ReactionCount      int32    `json:"reaction_count"`
	VoteScore          int32    `json:"vote_score"`
	VelocityScore      float64  `json:"velocity_score"`
	Excerpt            string   `json:"excerpt,omitempty"`
	Tags               []string `json:"tags,omitempty"`
	ReadingTimeMinutes int      `json:"reading_time_minutes,omitempty"`
	CoverImageURL      string   `json:"cover_image_url,omitempty"`
	BookmarkCount      int32    `json:"bookmark_count,omitempty"`
	// Journey-aware recommendation metadata surfaced on feed cards.
	JourneyStage  *string `json:"journey_stage,omitempty"`
	VisaRoute     *string `json:"visa_route,omitempty"`
	SourceCountry *string `json:"source_country,omitempty"`
	TargetCountry *string `json:"target_country,omitempty"`
	Purpose       *string `json:"purpose,omitempty"`
	// §14 guide amendment: the guide category slug so feed cards can render
	// a category badge for content_type='guide' items.
	GuideCategory *string `json:"guide_category,omitempty"`
	// Video-aware fields for daily video cards.
	VideoSourceID     string `json:"video_source_id,omitempty"`
	VideoSourceURL    string `json:"video_source_url,omitempty"`
	VideoSourceType   string `json:"video_source_type,omitempty"`
	VideoDurationSecs int32  `json:"video_duration_secs,omitempty"`
	VideoThumbnailURL string `json:"video_thumbnail_url,omitempty"`
}

// OrgRef is the lean org badge surfaced on daily feed cards when a post was
// published on behalf of an organization.
// Ref: japaconnect-engineering/sections/07-organization-profiles.md
type OrgRef struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Slug    string `json:"slug"`
	LogoURL string `json:"logo_url,omitempty"`
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
func (s *Service) GetFeed(ctx context.Context, tenantID pgtype.UUID, timeline string, userID, spaceID, authorID *pgtype.UUID, sort string, limit, offset int32, visitorCountryCode, visitorRegion string) ([]FeedItem, error) {
	// Clamp pagination limits.
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	if offset < 0 {
		offset = 0
	}

	switch timeline {
	case "explore":
		return s.getExploreFeed(ctx, tenantID, sort, visitorCountryCode, visitorRegion, limit, offset)
	case "primary":
		if userID == nil {
			return nil, errors.New("primary feed requires user_id")
		}

		return s.getPrimaryFeedItems(ctx, tenantID, *userID, sort, limit, offset)
	case "space":
		if spaceID == nil {
			return nil, errors.New("space feed requires space_id")
		}

		// viewerID is the authenticated user (may be nil for guests); used
		// for mutual user_blocks exclusion per §41.
		var viewerID pgtype.UUID
		if userID != nil {
			viewerID = *userID
		}

		return s.getSpaceFeedItems(ctx, tenantID, *spaceID, viewerID, sort, limit, offset)
	case "profile":
		if authorID == nil {
			return nil, errors.New("profile feed requires author_id")
		}

		// viewerID is the authenticated user (may be nil for guests); used
		// for mutual user_blocks exclusion per §41.
		var viewerID pgtype.UUID
		if userID != nil {
			viewerID = *userID
		}

		return s.getProfileFeedItems(ctx, tenantID, *authorID, viewerID, sort, limit, offset)
	default:
		return nil, fmt.Errorf("unknown timeline: %s", timeline)
	}
}

// getExploreFeed returns the explore feed with the given sort mode.
func (s *Service) getExploreFeed(ctx context.Context, tenantID pgtype.UUID, sort, visitorCountryCode, visitorRegion string, limit, offset int32) ([]FeedItem, error) {
	switch sort {
	case SortTop:
		rows, err := s.store.getTopFeed(ctx, tenantID, visitorCountryCode, visitorRegion, limit, offset)
		if err != nil {
			return nil, err
		}

		return topRowsToItems(rows), nil
	case SortRising:
		rows, err := s.store.getRisingFeed(ctx, tenantID, visitorCountryCode, visitorRegion, limit, offset)
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
		rows, err := s.store.getTopFeed(ctx, tenantID, visitorCountryCode, visitorRegion, limit, offset)
		if err != nil {
			return nil, err
		}

		return topRowsToItems(rows), nil
	default: // "latest" uses velocity_score DESC
		rows, err := s.store.getExploreFeed(ctx, tenantID, visitorCountryCode, visitorRegion, limit, offset)
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
// viewerID is nullable — when valid, mutual user_blocks are excluded (§41).
func (s *Service) getSpaceFeedItems(ctx context.Context, tenantID, spaceID, viewerID pgtype.UUID, sort string, limit, offset int32) ([]FeedItem, error) {
	rows, err := s.store.getSpaceFeed(ctx, tenantID, spaceID, viewerID, sort, limit, offset)
	if err != nil {
		return nil, err
	}

	return spaceRowsToItems(rows), nil
}

// getProfileFeedItems returns a user's public posts with sort.
// sort: 'latest' (default), 'top' (vote_score DESC), 'rising' (velocity_score DESC).
// 'relevant' falls back to latest (pgvector relevance is only in the for-you feed).
// viewerID is nullable — when valid, mutual user_blocks are excluded (§41).
func (s *Service) getProfileFeedItems(ctx context.Context, tenantID, authorID, viewerID pgtype.UUID, sort string, limit, offset int32) ([]FeedItem, error) {
	rows, err := s.store.getProfileFeed(ctx, tenantID, authorID, viewerID, sort, limit, offset)
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
func (s *Service) UpdateContentScore(ctx context.Context, contentID pgtype.UUID, voteScore, commentCount, viewCount, bookmarkCount, reactionCount int32) error {
	return s.store.updateContentScore(ctx, gen.UpdateContentScoreParams{
		ContentID:     contentID,
		VoteScore:     voteScore,
		CommentCount:  commentCount,
		ViewCount:     viewCount,
		BookmarkCount: bookmarkCount,
		ReactionCount: reactionCount,
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

// asString normalizes a string or *string value to a plain string.
// sqlc may emit either type depending on whether the source column comes
// from an OUTER JOIN; this avoids compile errors from row to row.
func asString(v any) string {
	if v == nil {
		return ""
	}

	switch s := v.(type) {
	case string:
		return s
	case *string:
		if s == nil {
			return ""
		}

		return *s
	}

	return ""
}

// asStringPtr returns a pointer to a non-empty string, or nil for nil/empty values.
func asStringPtr(v any) *string {
	s := asString(v)
	if s == "" {
		return nil
	}

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

// orgRefFrom builds a feed OrgRef from the joined organization columns, or nil
// when the post is not org-attributed.
func orgRefFrom(orgID pgtype.UUID, name, slug, logoBucket, logoPath *string) *OrgRef {
	if !orgID.Valid {
		return nil
	}

	ref := &OrgRef{ID: uuidToString(orgID)}
	if name != nil {
		ref.Name = *name
	}
	if slug != nil {
		ref.Slug = *slug
	}
	ref.LogoURL = mediaURLFromPtr(logoBucket, logoPath)

	return ref
}

// mediaURLFromPtr returns a media URL from nullable storage refs, or "".
func mediaURLFromPtr(bucket, path *string) string {
	if bucket == nil || path == nil || *bucket == "" || *path == "" {
		return ""
	}

	return media.MediaURL(*bucket, *path, media.ImageProxyOpts{})
}

// coverImageURL resolves the cover image URL from storage bucket and path,
// falling back to the provided URL (e.g. a video thumbnail) when no stored
// media cover is present.
func coverImageURL(bucket, path *string, fallback ...*string) string {
	if bucket != nil && path != nil && *bucket != "" && *path != "" {
		return media.MediaURL(*bucket, *path, media.ImageProxyOpts{})
	}

	if len(fallback) > 0 && fallback[0] != nil && *fallback[0] != "" {
		return *fallback[0]
	}

	return ""
}

func roadmapRowsToItems(rows []gen.ListContentsBySlugsRow) []FeedItem {
	items := make([]FeedItem, len(rows))

	for i, r := range rows {
		authorName := ""
		if r.AuthorDisplayName != nil && *r.AuthorDisplayName != "" {
			authorName = *r.AuthorDisplayName
		} else if r.AuthorUsername != nil {
			authorName = *r.AuthorUsername
		}

		excerpt := strings.TrimSpace(r.Excerpt)

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
			AuthorName:         authorName,
			AuthorAvatarURL:    r.AuthorAvatarUrl,
			SpaceName:          asString(r.SpaceName),
			SpaceSlug:          r.SpaceSlug,
			Excerpt:            excerpt,
			ReadingTimeMinutes: int(r.ReadTimeMinutes),
			CoverImageURL:      coverImageURL(r.CoverStorageBucket, r.CoverStoragePath),
			BookmarkCount:      r.BookmarkCount,
			Tags:               splitTags(r.Tags),
			JourneyStage:       r.JourneyStage,
			VisaRoute:          r.VisaRoute,
			SourceCountry:      r.SourceCountry,
			TargetCountry:      r.TargetCountry,
			Purpose:            asStringPtr(r.Purpose),
		}
	}

	return items
}

// GetRoadmapFeed returns the readiness roadmap contents for the visitor, in the
// order returned by the assessment. Empty roadmap slugs produce an empty list.
// Public/members content is returned; private content is only returned for the
// author or someone with can_see_private.
func (s *Service) GetRoadmapFeed(ctx context.Context, tenantID, requesterID pgtype.UUID, slugs []string, limit, offset int32) ([]FeedItem, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	if len(slugs) == 0 {
		return []FeedItem{}, nil
	}

	rows, err := s.store.getContentsBySlugs(ctx, tenantID, requesterID, slugs, limit, offset)
	if err != nil {
		return nil, err
	}

	return roadmapRowsToItems(rows), nil
}

// GetRoadmapFeedWithContext returns the roadmap contents for the supplied slugs
// and reorders them by how well they match the visitor's journey context.
// This is used by the public assessment result roadmap endpoint to surface
// destination-, stage-, and visa-relevant content first while preserving the
// roadmap's overall order as a tie-breaker.
func (s *Service) GetRoadmapFeedWithContext(ctx context.Context, tenantID, requesterID pgtype.UUID, slugs []string, limit, offset int32, vc *visitor.VisitorContext) ([]FeedItem, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	if vc == nil || (vc.DestinationCountry == "" && vc.JourneyStage == "" && vc.VisaRoute == "" && vc.OriginCountry == "") {
		return s.GetRoadmapFeed(ctx, tenantID, requesterID, slugs, limit, offset)
	}

	// Only the result's own roadmap slugs are returned — the numbered "next
	// steps" list must align 1:1 with the stored roadmap steps, so no extra
	// corpus content is merged in. Slugs are deduplicated while preserving
	// the roadmap order (kept as the re-rank tie-breaker).
	seen := make(map[string]struct{}, len(slugs))
	merged := make([]string, 0, len(slugs))

	for _, slug := range slugs {
		if slug == "" {
			continue
		}

		if _, ok := seen[slug]; !ok {
			seen[slug] = struct{}{}

			merged = append(merged, slug)
		}
	}

	if len(merged) == 0 {
		return []FeedItem{}, nil
	}

	fetchLimit := int32(len(merged)) // #nosec
	if fetchLimit > MaxPageSize {
		fetchLimit = MaxPageSize
	}

	rows, err := s.store.getContentsBySlugs(ctx, tenantID, requesterID, merged, fetchLimit, 0)
	if err != nil {
		return nil, err
	}

	items := roadmapRowsToItems(rows)

	type scoredItem struct {
		item  FeedItem
		score int
		index int
	}

	scored := make([]scoredItem, len(items))

	for i, item := range items {
		score := 0
		if item.TargetCountry != nil && *item.TargetCountry == vc.DestinationCountry {
			score += 4
		}

		if item.SourceCountry != nil && *item.SourceCountry == vc.OriginCountry {
			score += 1
		}

		if item.JourneyStage != nil && *item.JourneyStage == vc.JourneyStage {
			score += 2
		}

		if item.VisaRoute != nil && *item.VisaRoute == vc.VisaRoute {
			score += 2
		}

		if item.Purpose != nil && *item.Purpose == vc.Purpose {
			score += 1
		}

		scored[i] = scoredItem{item: item, score: score, index: i}
	}

	// Stable sort: higher match scores first, original roadmap order for ties.
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}

		return scored[i].index < scored[j].index
	})

	out := make([]FeedItem, len(scored))
	for i, si := range scored {
		out[i] = si.item
	}

	// Apply the original pagination window.
	start := offset
	//nolint:gosec // G115: bounded
	if start >= int32(len(out)) { // #nosec //nolint:gosec // G115: bounded
		return []FeedItem{}, nil
	}

	end := start + limit
	//nolint:gosec // G115: bounded
	if end > int32(len(out)) { // #nosec //nolint:gosec // G115: bounded
		end = int32(len(out)) // #nosec
	}

	return out[start:end], nil
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
			AuthorUsername:     r.AuthorUsername,
			AuthorName:         r.AuthorName,
			AuthorAvatarURL:    r.AuthorAvatarUrl,
			SpaceID:            nullableUUIDToString(r.SpaceID),
			SpaceName:          asString(r.SpaceName),
			SpaceSlug:          r.SpaceSlug,
			SpaceAvatarURL:     mediaURLFromPtr(r.SpaceAvatarStorageBucket, r.SpaceAvatarStoragePath),
			Organization:       orgRefFrom(r.OrganizationID, r.OrganizationName, r.OrganizationSlug, r.OrganizationLogoStorageBucket, r.OrganizationLogoStoragePath),
			CreatedAt:          r.CreatedAt.Time.String(),
			ViewCount:          r.ViewCount,
			CommentCount:       r.CommentCount,
			ReactionCount:      r.ReactionCount,
			VoteScore:          r.VoteScore,
			VelocityScore:      pgNumericToFloat64(r.VelocityScore),
			Excerpt:            r.Excerpt,
			Tags:               splitTags(r.Tags),
			ReadingTimeMinutes: int(r.ReadTimeMinutes),
			CoverImageURL:      coverImageURL(r.CoverStorageBucket, r.CoverStoragePath, r.VideoThumbnailUrl),
			BookmarkCount:      r.BookmarkCount,
			JourneyStage:       r.JourneyStage,
			VisaRoute:          r.VisaRoute,
			SourceCountry:      r.SourceCountry,
			TargetCountry:      r.TargetCountry,
			GuideCategory:      r.GuideCategory,
			VideoSourceURL:     derefStr(r.VideoSourceUrl),
			VideoSourceType:    derefStr(r.VideoSourceType),
			VideoDurationSecs:  derefInt32(r.VideoDurationSecs),
			VideoThumbnailURL:  derefStr(r.VideoThumbnailUrl),
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
			AuthorUsername:     r.AuthorUsername,
			AuthorName:         r.AuthorName,
			AuthorAvatarURL:    r.AuthorAvatarUrl,
			SpaceID:            nullableUUIDToString(r.SpaceID),
			SpaceName:          asString(r.SpaceName),
			SpaceSlug:          r.SpaceSlug,
			SpaceAvatarURL:     mediaURLFromPtr(r.SpaceAvatarStorageBucket, r.SpaceAvatarStoragePath),
			Organization:       orgRefFrom(r.OrganizationID, r.OrganizationName, r.OrganizationSlug, r.OrganizationLogoStorageBucket, r.OrganizationLogoStoragePath),
			CreatedAt:          r.CreatedAt.Time.String(),
			ViewCount:          r.ViewCount,
			CommentCount:       r.CommentCount,
			ReactionCount:      r.ReactionCount,
			VoteScore:          r.VoteScore,
			VelocityScore:      pgNumericToFloat64(r.VelocityScore),
			Excerpt:            r.Excerpt,
			Tags:               splitTags(r.Tags),
			ReadingTimeMinutes: int(r.ReadTimeMinutes),
			CoverImageURL:      coverImageURL(r.CoverStorageBucket, r.CoverStoragePath, r.VideoThumbnailUrl),
			BookmarkCount:      r.BookmarkCount,
			JourneyStage:       r.JourneyStage,
			VisaRoute:          r.VisaRoute,
			SourceCountry:      r.SourceCountry,
			TargetCountry:      r.TargetCountry,
			GuideCategory:      r.GuideCategory,
			VideoSourceURL:     derefStr(r.VideoSourceUrl),
			VideoSourceType:    derefStr(r.VideoSourceType),
			VideoDurationSecs:  derefInt32(r.VideoDurationSecs),
			VideoThumbnailURL:  derefStr(r.VideoThumbnailUrl),
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
			AuthorUsername:     r.AuthorUsername,
			AuthorName:         r.AuthorName,
			AuthorAvatarURL:    r.AuthorAvatarUrl,
			SpaceID:            nullableUUIDToString(r.SpaceID),
			SpaceName:          asString(r.SpaceName),
			SpaceSlug:          r.SpaceSlug,
			SpaceAvatarURL:     mediaURLFromPtr(r.SpaceAvatarStorageBucket, r.SpaceAvatarStoragePath),
			Organization:       orgRefFrom(r.OrganizationID, r.OrganizationName, r.OrganizationSlug, r.OrganizationLogoStorageBucket, r.OrganizationLogoStoragePath),
			CreatedAt:          r.CreatedAt.Time.String(),
			ViewCount:          r.ViewCount,
			CommentCount:       r.CommentCount,
			ReactionCount:      r.ReactionCount,
			VoteScore:          r.VoteScore,
			VelocityScore:      pgNumericToFloat64(r.VelocityScore),
			Excerpt:            r.Excerpt,
			Tags:               splitTags(r.Tags),
			ReadingTimeMinutes: int(r.ReadTimeMinutes),
			CoverImageURL:      coverImageURL(r.CoverStorageBucket, r.CoverStoragePath, r.VideoThumbnailUrl),
			BookmarkCount:      r.BookmarkCount,
			JourneyStage:       r.JourneyStage,
			VisaRoute:          r.VisaRoute,
			SourceCountry:      r.SourceCountry,
			TargetCountry:      r.TargetCountry,
			GuideCategory:      r.GuideCategory,
			VideoSourceURL:     derefStr(r.VideoSourceUrl),
			VideoSourceType:    derefStr(r.VideoSourceType),
			VideoDurationSecs:  derefInt32(r.VideoDurationSecs),
			VideoThumbnailURL:  derefStr(r.VideoThumbnailUrl),
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
			AuthorUsername:     r.AuthorUsername,
			AuthorName:         r.AuthorName,
			AuthorAvatarURL:    r.AuthorAvatarUrl,
			SpaceID:            nullableUUIDToString(r.SpaceID),
			SpaceName:          asString(r.SpaceName),
			SpaceSlug:          r.SpaceSlug,
			SpaceAvatarURL:     mediaURLFromPtr(r.SpaceAvatarStorageBucket, r.SpaceAvatarStoragePath),
			Organization:       orgRefFrom(r.OrganizationID, r.OrganizationName, r.OrganizationSlug, r.OrganizationLogoStorageBucket, r.OrganizationLogoStoragePath),
			CreatedAt:          r.ContentCreatedAt.Time.String(),
			ViewCount:          r.ViewCount,
			CommentCount:       r.CommentCount,
			ReactionCount:      r.ReactionCount,
			VoteScore:          derefInt32(r.VoteScore),
			VelocityScore:      pgNumericToFloat64(r.VelocityScore),
			Excerpt:            r.Excerpt,
			Tags:               splitTags(r.Tags),
			ReadingTimeMinutes: int(r.ReadTimeMinutes),
			CoverImageURL:      coverImageURL(r.CoverStorageBucket, r.CoverStoragePath, r.VideoThumbnailUrl),
			BookmarkCount:      derefInt32(r.BookmarkCount),
			JourneyStage:       r.JourneyStage,
			VisaRoute:          r.VisaRoute,
			SourceCountry:      r.SourceCountry,
			TargetCountry:      r.TargetCountry,
			GuideCategory:      r.GuideCategory,
			VideoSourceURL:     derefStr(r.VideoSourceUrl),
			VideoSourceType:    derefStr(r.VideoSourceType),
			VideoDurationSecs:  derefInt32(r.VideoDurationSecs),
			VideoThumbnailURL:  derefStr(r.VideoThumbnailUrl),
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
			AuthorUsername:     r.AuthorUsername,
			AuthorName:         r.AuthorName,
			AuthorAvatarURL:    r.AuthorAvatarUrl,
			SpaceID:            nil,
			SpaceName:          asString(r.SpaceName),
			SpaceSlug:          r.SpaceSlug,
			SpaceAvatarURL:     mediaURLFromPtr(r.SpaceAvatarStorageBucket, r.SpaceAvatarStoragePath),
			Organization:       orgRefFrom(r.OrganizationID, r.OrganizationName, r.OrganizationSlug, r.OrganizationLogoStorageBucket, r.OrganizationLogoStoragePath),
			CreatedAt:          r.CreatedAt.Time.String(),
			ViewCount:          r.ViewCount,
			CommentCount:       r.CommentCount,
			ReactionCount:      r.ReactionCount,
			VoteScore:          r.VoteScore,
			VelocityScore:      pgNumericToFloat64(r.VelocityScore),
			Excerpt:            r.Excerpt,
			Tags:               splitTags(r.Tags),
			ReadingTimeMinutes: int(r.ReadTimeMinutes),
			CoverImageURL:      coverImageURL(r.CoverStorageBucket, r.CoverStoragePath, r.VideoThumbnailUrl),
			BookmarkCount:      r.BookmarkCount,
			JourneyStage:       r.JourneyStage,
			VisaRoute:          r.VisaRoute,
			SourceCountry:      r.SourceCountry,
			TargetCountry:      r.TargetCountry,
			GuideCategory:      r.GuideCategory,
			VideoSourceURL:     derefStr(r.VideoSourceUrl),
			VideoSourceType:    derefStr(r.VideoSourceType),
			VideoDurationSecs:  derefInt32(r.VideoDurationSecs),
			VideoThumbnailURL:  derefStr(r.VideoThumbnailUrl),
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
			AuthorUsername:     r.AuthorUsername,
			AuthorName:         r.AuthorName,
			AuthorAvatarURL:    r.AuthorAvatarUrl,
			SpaceID:            nullableUUIDToString(r.SpaceID),
			SpaceName:          asString(r.SpaceName),
			SpaceSlug:          r.SpaceSlug,
			SpaceAvatarURL:     mediaURLFromPtr(r.SpaceAvatarStorageBucket, r.SpaceAvatarStoragePath),
			Organization:       orgRefFrom(r.OrganizationID, r.OrganizationName, r.OrganizationSlug, r.OrganizationLogoStorageBucket, r.OrganizationLogoStoragePath),
			CreatedAt:          r.CreatedAt.Time.String(),
			ViewCount:          r.ViewCount,
			CommentCount:       r.CommentCount,
			ReactionCount:      r.ReactionCount,
			VoteScore:          r.VoteScore,
			VelocityScore:      pgNumericToFloat64(r.VelocityScore),
			Excerpt:            r.Excerpt,
			Tags:               splitTags(r.Tags),
			ReadingTimeMinutes: int(r.ReadTimeMinutes),
			CoverImageURL:      coverImageURL(r.CoverStorageBucket, r.CoverStoragePath, r.VideoThumbnailUrl),
			BookmarkCount:      r.BookmarkCount,
			JourneyStage:       r.JourneyStage,
			VisaRoute:          r.VisaRoute,
			SourceCountry:      r.SourceCountry,
			TargetCountry:      r.TargetCountry,
			GuideCategory:      r.GuideCategory,
			VideoSourceURL:     derefStr(r.VideoSourceUrl),
			VideoSourceType:    derefStr(r.VideoSourceType),
			VideoDurationSecs:  derefInt32(r.VideoDurationSecs),
			VideoThumbnailURL:  derefStr(r.VideoThumbnailUrl),
		}
	}

	return items
}

// ComputeVelocity calculates the velocity score for a post.
// This Go helper mirrors the SQL formula in feeds.sql (UpdateVelocityScores)
// so the cron-written content_scores.velocity_score and any in-process
// computation agree. Formula:
//
//	(reaction_count*2 + comment_count + view_count/100) / hours^gravity
//
// Ref: japaconnect-engineering/sections/32-activity-feed-timeline.md.
func ComputeVelocity(reactionCount, commentCount, viewCount, bookmarkCount int32, hoursOld float64) float64 {
	// Match the SQL formula: reactions*2 + comment + view/100.
	// bookmark_count is not in the SQL formula; kept as a param for API compat.
	engagement := float64(reactionCount*2) + float64(commentCount) + float64(viewCount)/100.0

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

// interfaceToFloat64 converts the sqlc-generated interface{} value (used for
// the MAX/CASE geo_proximity expression) into a float64 for ranking.
func interfaceToFloat64(v interface{}) float64 {
	switch n := v.(type) {
	case int64:
		return float64(n)
	case int32:
		return float64(n)
	case int:
		return float64(n)
	case float64:
		return n
	case string:
		f, err := strconv.ParseFloat(n, 64)
		if err == nil {
			return f
		}
	}

	return 0
}

// For-You feed bounds and cache tuning (v1 constants).
const (
	forYouWindowDaysDefault = 30
	forYouAffinityCap       = 300
	forYouVelocityCap       = 300
	forYouCandidateCap      = 600
	forYouCacheTTLDefault   = 90 * time.Second
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
// vc is the visitor's resolved context (ADR-064 + journey-aware). It provides
// the geo country/region for the bounded geo boost and the journey signals
// (destination, stage, visa, roadmap) for journey-aware ranking. A nil vc
// disables both features and falls back to the follow/velocity/recency mix.
// The geo boost is also disabled when the tenant setting
// geo_personalization.enabled is false (Section 32 L147–149).
func (s *Service) GetForYouFeed(
	ctx context.Context, tenantID, userID pgtype.UUID, sortMode string, limit, offset int32,
	vc *visitor.VisitorContext,
) ([]FeedItem, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	if offset < 0 {
		offset = 0
	}

	contentCountry := ""
	contentRegion := ""

	if vc != nil {
		contentCountry = vc.ContentCountry
		contentRegion = vc.ContentRegion
	}

	// BUG-007: honor geo_personalization.enabled. When disabled, clear
	// contentCountry/contentRegion so no geo boost is applied. Ref: Section 32 L147–149.
	if s.settings != nil && (contentCountry != "" || contentRegion != "") {
		if !s.settings.GetBoolOr(ctx, tenantID, "geo_personalization.enabled", true) {
			contentCountry = ""
			contentRegion = ""
		}
	}

	// Build the lookup map for readiness roadmap slug boosts.
	roadmapSlugs := make(map[string]bool)

	if vc != nil {
		for _, slug := range vc.RoadmapSlugs {
			if slug != "" {
				roadmapSlugs[slug] = true
			}
		}
	}

	// Load scoring weights from system_settings. Defaults are provided by
	// defaultRankingWeights() so new tenants work without explicit config.
	w := defaultRankingWeights()
	if s.settings != nil {
		w.Velocity = s.settings.GetFloatOr(ctx, tenantID, "feed.for_you_weight_velocity", w.Velocity)
		w.Author = s.settings.GetFloatOr(ctx, tenantID, "feed.for_you_weight_author", w.Author)
		w.Space = s.settings.GetFloatOr(ctx, tenantID, "feed.for_you_weight_space", w.Space)
		w.Tag = s.settings.GetFloatOr(ctx, tenantID, "feed.for_you_weight_tag", w.Tag)
		w.Recency = s.settings.GetFloatOr(ctx, tenantID, "feed.for_you_weight_recency", w.Recency)
		w.Geo = s.settings.GetFloatOr(ctx, tenantID, "feed.for_you_geo_weight", w.Geo)
		w.GeoBoostCap = s.settings.GetFloatOr(ctx, tenantID, "feed.for_you_geo_boost_cap", w.GeoBoostCap)
		w.JourneyDest = s.settings.GetFloatOr(ctx, tenantID, "feed.for_you_journey_weight_destination", w.JourneyDest)
		w.JourneyStage = s.settings.GetFloatOr(ctx, tenantID, "feed.for_you_journey_weight_stage", w.JourneyStage)
		w.JourneyVisa = s.settings.GetFloatOr(ctx, tenantID, "feed.for_you_journey_weight_visa", w.JourneyVisa)
		w.JourneySource = s.settings.GetFloatOr(ctx, tenantID, "feed.for_you_journey_weight_source", w.JourneySource)
		w.JourneyRoad = s.settings.GetFloatOr(ctx, tenantID, "feed.for_you_journey_weight_roadmap", w.JourneyRoad)
		w.JourneyCap = s.settings.GetFloatOr(ctx, tenantID, "feed.for_you_journey_boost_cap", w.JourneyCap)
		w.TagOverlapCap = s.settings.GetIntOr(ctx, tenantID, "feed.for_you_tag_overlap_cap", w.TagOverlapCap)
		w.HalfLifeHours = s.settings.GetFloatOr(ctx, tenantID, "feed.for_you_half_life_hours", w.HalfLifeHours)
	}

	key := forYouCacheKey(tenantID, userID, sortMode, vc)

	// Use the shared cache helper with distributed lock stampede protection
	// (BUG-008). On cache miss, the lock holder loads affinity, fetches
	// candidates, and ranks them; concurrent requests wait for the result.
	// Read cache TTL and window days from system_settings (Rule 3).
	cacheTTL := forYouCacheTTLDefault

	windowDays := forYouWindowDaysDefault
	if s.settings != nil {
		windowDays = s.settings.GetIntOr(ctx, tenantID, "feed.for_you_window_days", forYouWindowDaysDefault)
	}

	ranked, err := vkcache.GetOrComputeCompat(ctx, s.vk, key, cacheTTL, func(ctx context.Context) ([]FeedItem, error) {
		authorIDs, spaceIDs, tagIDs, err := s.loadAffinity(ctx, tenantID, userID)
		if err != nil {
			return nil, err
		}

		var windowStart pgtype.Timestamptz
		_ = windowStart.Scan(time.Now().Add(-time.Duration(windowDays) * 24 * time.Hour))

		visitorCountryCode := contentCountry
		visitorRegion := contentRegion

		rows, err := s.store.getForYouCandidates(ctx, gen.GetForYouCandidatesParams{
			TenantID:           tenantID,
			WindowStart:        windowStart,
			ExcludeAuthorID:    userID,
			AuthorIds:          authorIDs,
			SpaceIds:           spaceIDs,
			TagIds:             tagIDs,
			AffinityLimit:      forYouAffinityCap,
			VelocityLimit:      forYouVelocityCap,
			CandidateCap:       forYouCandidateCap,
			VisitorCountryCode: &visitorCountryCode,
			VisitorRegion:      &visitorRegion,
		})
		if err != nil {
			return nil, err
		}

		// Score + rank in pure Go.
		cands := make([]Candidate, len(rows))
		for i, r := range rows {
			cands[i] = candidateFromRow(r)
		}

		result := make([]FeedItem, 0, len(cands))
		for _, c := range rankCandidates(cands, sortMode, time.Now(), vc, roadmapSlugs, w) {
			result = append(result, c.Item)
		}

		return result, nil
	})
	if err != nil {
		return nil, err
	}

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

// writeForYouCache stores the ranked feed. Best-effort:
// cache write failures never fail the request.
func (s *Service) writeForYouCache(ctx context.Context, key string, ranked []FeedItem) {
	if s.vk == nil {
		return
	}

	data, err := json.Marshal(ranked)
	if err != nil {
		return
	}

	_ = s.vk.Set(ctx, key, data, forYouCacheTTLDefault).Err()
}
