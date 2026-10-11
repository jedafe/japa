package widgets

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/media"
	"github.com/jedafe/japa/internal/sidebars"
)

// Renderer executes the data-source query for a widget type.
// Each widget type has a dedicated renderer that reads from the appropriate
// table(s) and returns JSON-serializable data.
type Renderer struct {
	reader *pgxpool.Pool
}

// NewRenderer creates a Renderer from the reader pool.
func NewRenderer(reader *pgxpool.Pool) *Renderer {
	return &Renderer{reader: reader}
}

// coverImageURL builds a signed imgproxy URL (or local serve fallback) from
// the media storage bucket and path. Returns empty string if either is nil
// or empty. Used by content-based widget renderers to surface cover_image_url
// alongside item data so the frontend can render card thumbnails.
func coverImageURL(bucket, path *string) string {
	if bucket == nil || path == nil || *bucket == "" || *path == "" {
		return ""
	}

	return media.MediaURL(*bucket, *path, media.ImageProxyOpts{})
}

// Render dispatches to the correct per-type renderer and returns the data.
func (r *Renderer) Render(ctx context.Context, w Widget) (json.RawMessage, error) {
	switch w.WidgetType {
	// --- Original 7 (Section 16) ---
	case "latest_content":
		return r.renderLatestContent(ctx, w)
	case "trending_tags":
		return r.renderTrendingTags(ctx, w)
	case "space_members":
		return r.renderSpaceMembers(ctx, w)
	case "user_leaderboard":
		return r.renderUserLeaderboard(ctx, w)
	case "upcoming_events":
		return r.renderUpcomingEvents(ctx, w)
	case "featured_spaces":
		return r.renderFeaturedSpaces(ctx, w)
	case "custom_html":
		return r.renderCustomHTML(ctx, w)
	// --- Extended: content-curation types ---
	case "featured_content":
		return r.renderFeaturedContent(ctx, w)
	case "blog_posts":
		return r.renderBlogPosts(ctx, w)
	case "forum_threads":
		return r.renderForumThreads(ctx, w)
	case "job_listings":
		return r.renderJobListings(ctx, w)
	case "achievements":
		return r.renderAchievements(ctx, w)
	// --- Extended: domain-specific types ---
	case "academy_tracks":
		return r.renderAcademyTracks(ctx, w)
	case "community_digest":
		return r.renderCommunityDigest(ctx, w)
	case "reviews":
		return r.renderReviews(ctx, w)
	case "comparisons":
		return r.renderComparisons(ctx, w)
	case "community_organisations":
		return r.renderCommunityOrganisations(ctx, w)
	case "mentor_spotlight":
		return r.renderMentorSpotlight(ctx, w)
	// --- Extended: form/widget types ---
	case "newsletter_signup":
		return r.renderNewsletterSignup(ctx, w)
	default:
		// Sidebar widget types (Section 59): the config IS the props — echo it
		// back (HTML-bearing keys sanitized) so a sidebar configured_widget
		// renders the type's native UI client-side. configured_widget itself
		// never validates via IsValidWidgetType, so no self-nesting is possible.
		if sidebars.IsValidWidgetType(w.WidgetType) {
			return configEcho(w.Config), nil
		}
		return nil, fmt.Errorf("unsupported widget type: %s", w.WidgetType)
	}
}

// configEcho returns a sidebar-typed widget's config as the render data —
// for chrome/data sidebar types the configuration IS the props object, and the
// client-side SidebarWidgetRenderer performs any data fetching. HTML-bearing
// keys are sanitized server-side, mirroring sidebars.sanitizeWidgetProps
// (defense in depth on top of the client-side sanitizeSidebarHtml).
func configEcho(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage("{}")
	}

	var props map[string]any
	if err := json.Unmarshal(raw, &props); err != nil {
		return json.RawMessage("{}")
	}

	if v, ok := props["content"].(string); ok {
		props["content"] = content.SanitizeHTML(v)
	}
	if v, ok := props["html"].(string); ok {
		props["html"] = content.SanitizeHTML(v)
	}

	out, err := json.Marshal(props)
	if err != nil {
		return json.RawMessage("{}")
	}

	return out
}

// widgetConfigLimit extracts the "limit" from the widget config JSON,
// defaulting to 5.
func widgetConfigLimit(config json.RawMessage) int32 {
	var c struct {
		Limit int32 `json:"limit"`
	}

	if len(config) > 0 {
		_ = json.Unmarshal(config, &c)
	}

	if c.Limit <= 0 || c.Limit > 50 {
		return 5
	}

	return c.Limit
}

func widgetConfigSpaceID(config json.RawMessage) *pgtype.UUID {
	var c struct {
		SpaceID string `json:"space_id"`
	}

	if len(config) > 0 {
		_ = json.Unmarshal(config, &c)
	}

	if c.SpaceID == "" {
		return nil
	}

	var u pgtype.UUID
	if err := u.Scan(c.SpaceID); err != nil {
		return nil
	}

	return &u
}

// widgetConfigString extracts a string field from the widget config JSON.
// Returns empty string if the field is absent or the config is empty.
func widgetConfigString(config json.RawMessage, key string) string {
	var c map[string]json.RawMessage
	if len(config) > 0 {
		_ = json.Unmarshal(config, &c)
	}

	raw, ok := c[key]
	if !ok {
		return ""
	}

	var s string
	_ = json.Unmarshal(raw, &s)

	return s
}

// widgetConfigBool extracts a bool field from the widget config JSON.
func widgetConfigBool(config json.RawMessage, key string) bool {
	var c map[string]json.RawMessage
	if len(config) > 0 {
		_ = json.Unmarshal(config, &c)
	}

	raw, ok := c[key]
	if !ok {
		return false
	}

	var b bool
	_ = json.Unmarshal(raw, &b)

	return b
}

// renderLatestContent returns the most recent published content with flexible
// filtering. Config options:
// - content_type: filter by contents.content_type (e.g. "blog", "job_listing",
// "achievement", "discussion"). Empty = all types.
// - space_id: filter by contents.space_id. Empty = all spaces.
// - featured: if true, only return is_pinned=true contents.
// - sort: "recent" (default), "popular" (view_count DESC), "discussed"
// (comment_count DESC).
// // This single renderer covers blog_posts, job_listings, and achievements via
// the content_type filter, but those also have dedicated first-class
// renderers for domain-specific enrichment.
func (r *Renderer) renderLatestContent(ctx context.Context, w Widget) (json.RawMessage, error) {
	limit := widgetConfigLimit(w.Config)
	spaceID := widgetConfigSpaceID(w.Config)
	contentType := widgetConfigString(w.Config, "content_type")
	featured := widgetConfigBool(w.Config, "featured")

	sortBy := widgetConfigString(w.Config, "sort")
	if sortBy == "" {
		sortBy = "recent"
	}

	orderClause := "c.created_at DESC"

	switch sortBy {
	case "popular":
		orderClause = "c.view_count DESC, c.created_at DESC"
	case "discussed":
		orderClause = "c.comment_count DESC, c.created_at DESC"
	}

	// Build the query dynamically — we use a fixed parameter set based on
	// which filters are present to avoid SQL injection.
	query := `SELECT c.id, c.title, c.slug, c.author_id, c.space_id, c.content_type,
	                 c.view_count, c.comment_count, c.created_at,
	                 m.storage_bucket AS cover_storage_bucket,
	                 m.storage_path AS cover_storage_path
	          FROM contents c
	          LEFT JOIN media m ON m.id = COALESCE(c.featured_image_id, c.header_image_id)
	            AND m.deleted_at IS NULL
	          WHERE c.deleted_at IS NULL
	            AND c.status = 'published'`
	args := []any{}
	argIdx := 1

	if spaceID != nil {
		query += fmt.Sprintf(" AND c.space_id = $%d", argIdx)

		args = append(args, *spaceID)
		argIdx++
	}

	if contentType != "" {
		query += fmt.Sprintf(" AND c.content_type = $%d", argIdx)

		args = append(args, contentType)
		argIdx++
	}

	if featured {
		query += " AND c.is_pinned = true"
	}

	query += fmt.Sprintf(" ORDER BY %s LIMIT $%d", orderClause, argIdx)

	args = append(args, limit)

	rows, err := r.reader.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("rendering latest_content: %w", err)
	}
	defer rows.Close()

	items := make([]map[string]any, 0)

	for rows.Next() {
		var id, authorID, spaceID pgtype.UUID

		var title, slug, ct string

		var viewCount, commentCount int32

		var createdAt interface{}

		var coverBucket, coverPath *string
		if err := rows.Scan(&id, &title, &slug, &authorID, &spaceID, &ct,
			&viewCount, &commentCount, &createdAt, &coverBucket, &coverPath); err != nil {
			return nil, fmt.Errorf("scanning latest_content: %w", err)
		}

		item := map[string]any{
			"id":            id,
			"title":         title,
			"slug":          slug,
			"author_id":     authorID,
			"space_id":      spaceID,
			"content_type":  ct,
			"view_count":    viewCount,
			"comment_count": commentCount,
			"created_at":    createdAt,
		}
		if url := coverImageURL(coverBucket, coverPath); url != "" {
			item["cover_image_url"] = url
		}

		items = append(items, item)
	}

	return json.Marshal(items)
}

// renderTrendingTags returns the most-used tags for the platform.
func (r *Renderer) renderTrendingTags(ctx context.Context, w Widget) (json.RawMessage, error) {
	limit := widgetConfigLimit(w.Config)

	rows, err := r.reader.Query(ctx, `
		SELECT t.id, t.name, t.slug, COUNT(ct.content_id) AS usage_count
		FROM tags t
		LEFT JOIN content_tags ct ON ct.tag_id = t.id
		
		GROUP BY t.id, t.name, t.slug
		ORDER BY usage_count DESC, t.name ASC
		LIMIT $1
	`,		limit)
	if err != nil {
		return nil, fmt.Errorf("rendering trending_tags: %w", err)
	}

	defer rows.Close()

	items := make([]map[string]any, 0)

	for rows.Next() {
		var id pgtype.UUID

		var name, slug string

		var count int64
		if err := rows.Scan(&id, &name, &slug, &count); err != nil {
			return nil, fmt.Errorf("scanning trending_tags: %w", err)
		}

		items = append(items, map[string]any{
			"id":          id,
			"name":        name,
			"slug":        slug,
			"usage_count": count,
		})
	}

	return json.Marshal(items)
}

// renderSpaceMembers returns members of a configured space with display info.
func (r *Renderer) renderSpaceMembers(ctx context.Context, w Widget) (json.RawMessage, error) {
	limit := widgetConfigLimit(w.Config)

	spaceID := widgetConfigSpaceID(w.Config)
	if spaceID == nil {
		return json.Marshal([]map[string]any{})
	}

	rows, err := r.reader.Query(ctx, `
		SELECT sm.user_id, sm.joined_at,
		       u.username, COALESCE(up.display_name, '') AS display_name
		FROM space_members sm
		JOIN users u ON u.id = sm.user_id AND u.status = 'active'
		LEFT JOIN user_profiles up ON up.user_id = u.id
		WHERE sm.space_id = $1 AND sm.status = 'active'
		ORDER BY sm.joined_at DESC
		LIMIT $2
	`, *spaceID, limit)
	if err != nil {
		return nil, fmt.Errorf("rendering space_members: %w", err)
	}

	defer rows.Close()

	items := make([]map[string]any, 0)

	for rows.Next() {
		var userID pgtype.UUID

		var username, displayName string

		var joinedAt interface{}
		if err := rows.Scan(&userID, &joinedAt, &username, &displayName); err != nil {
			return nil, fmt.Errorf("scanning space_members: %w", err)
		}

		items = append(items, map[string]any{
			"user_id":      userID,
			"username":     username,
			"display_name": displayName,
			"joined_at":    joinedAt,
		})
	}

	return json.Marshal(items)
}

// renderUserLeaderboard returns top users by a metric (default: points).
// Uses the same aggregation as GET /gamification/leaderboard — SUM of
// positive 'points' earns in wallet_ledger (lifetime-points semantics,
// Section 64). display_name/avatar come from user_profiles (they do not
// exist on users; users have no deleted_at — status='active' is the
// soft-delete gate).
func (r *Renderer) renderUserLeaderboard(ctx context.Context, w Widget) (json.RawMessage, error) {
	limit := widgetConfigLimit(w.Config)

	rows, err := r.reader.Query(ctx, `
		SELECT u.id, u.username,
		       COALESCE(up.display_name, '') AS display_name,
		       COALESCE(up.avatar_url, '')  AS avatar_url,
		       COALESCE(SUM(wl.amount) FILTER (WHERE wl.amount > 0), 0)::bigint AS points
		FROM wallet_ledger wl
		JOIN users u ON u.id = wl.user_id
		LEFT JOIN user_profiles up ON up.user_id = u.id
		WHERE wl.ledger_type = 'points'
		  AND u.is_bot = false AND u.status = 'active'
		GROUP BY u.id, u.username, up.display_name, up.avatar_url
		ORDER BY points DESC
		LIMIT $1
	`,		limit)
	if err != nil {
		return nil, fmt.Errorf("rendering user_leaderboard: %w", err)
	}

	defer rows.Close()

	items := make([]map[string]any, 0)

	for rows.Next() {
		var id pgtype.UUID

		var username, displayName, avatarURL string

		var points int64
		if err := rows.Scan(&id, &username, &displayName, &avatarURL, &points); err != nil {
			return nil, fmt.Errorf("scanning user_leaderboard: %w", err)
		}

		items = append(items, map[string]any{
			"id":           id,
			"username":     username,
			"display_name": displayName,
			"avatar_url":   avatarURL,
			"points":       points,
		})
	}

	return json.Marshal(items)
}

// renderUpcomingEvents returns upcoming calendar entries.
func (r *Renderer) renderUpcomingEvents(ctx context.Context, w Widget) (json.RawMessage, error) {
	limit := widgetConfigLimit(w.Config)

	rows, err := r.reader.Query(ctx, `
		SELECT id, title, slug, start_time, end_time
		FROM calendar_entries
		WHERE status = 'approved' AND start_time >= now()
		  AND deleted_at IS NULL
		ORDER BY start_time ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("rendering upcoming_events: %w", err)
	}

	defer rows.Close()

	items := make([]map[string]any, 0)

	for rows.Next() {
		var id pgtype.UUID

		var title, slug string

		var startTime, endTime interface{}
		if err := rows.Scan(&id, &title, &slug, &startTime, &endTime); err != nil {
			return nil, fmt.Errorf("scanning upcoming_events: %w", err)
		}

		items = append(items, map[string]any{
			"id":         id,
			"title":      title,
			"slug":       slug,
			"start_time": startTime,
			"end_time":   endTime,
		})
	}

	return json.Marshal(items)
}

// renderFeaturedSpaces returns featured/public spaces with logo URLs.
func (r *Renderer) renderFeaturedSpaces(ctx context.Context, w Widget) (json.RawMessage, error) {
	limit := widgetConfigLimit(w.Config)

	rows, err := r.reader.Query(ctx, `
		SELECT s.id, s.name, s.slug, s.description,
		       m.storage_bucket AS logo_storage_bucket,
		       m.storage_path AS logo_storage_path
		FROM spaces s
		LEFT JOIN media m ON m.id = s.logo_media_id AND m.deleted_at IS NULL
		WHERE s.deleted_at IS NULL
		ORDER BY s.created_at DESC
		LIMIT $1
	`,		limit)
	if err != nil {
		return nil, fmt.Errorf("rendering featured_spaces: %w", err)
	}

	defer rows.Close()

	items := make([]map[string]any, 0)

	for rows.Next() {
		var id pgtype.UUID

		var name, slug string

		var description *string

		var logoBucket, logoPath *string
		if err := rows.Scan(&id, &name, &slug, &description, &logoBucket, &logoPath); err != nil {
			return nil, fmt.Errorf("scanning featured_spaces: %w", err)
		}

		item := map[string]any{
			"id":          id,
			"name":        name,
			"slug":        slug,
			"description": description,
		}
		if url := coverImageURL(logoBucket, logoPath); url != "" {
			item["logo_url"] = url
		}

		items = append(items, item)
	}

	return json.Marshal(items)
}

// renderCustomHTML returns the config's html field, sanitized server-side
// through content.SanitizeHTML to prevent stored XSS (§5.1/§14 — Renderer
// must allowlist tags, never trust client HTML).
func (r *Renderer) renderCustomHTML(ctx context.Context, w Widget) (json.RawMessage, error) {
	var c struct {
		HTML string `json:"html"`
	}

	if len(w.Config) > 0 {
		_ = json.Unmarshal(w.Config, &c)
	}

	return json.Marshal(map[string]any{"html": content.SanitizeHTML(c.HTML)})
}

// ─── Extended: content-curation types ───────────────────────────────────

// renderFeaturedContent returns admin-pinned content from content_featured,
// joined with the full content row. Ref: §14 content_featured table.
// // Config options (same filtering surface as latest_content):
// - content_type: filter by contents.content_type (e.g. "blog", "job_listing",
// "achievement", "discussion"). Empty = all types.
// - space_id: filter by contents.space_id. Empty = all spaces.
// - sort: "featured" (default, uses featured_order ASC), "recent"
// (published_at DESC), "popular" (view_count DESC), "discussed"
// (comment_count DESC).
func (r *Renderer) renderFeaturedContent(ctx context.Context, w Widget) (json.RawMessage, error) {
	limit := widgetConfigLimit(w.Config)
	spaceID := widgetConfigSpaceID(w.Config)
	contentType := widgetConfigString(w.Config, "content_type")

	sortBy := widgetConfigString(w.Config, "sort")
	if sortBy == "" {
		sortBy = "featured"
	}

	orderClause := "cf.featured_order ASC, c.published_at DESC"

	switch sortBy {
	case "recent":
		orderClause = "c.published_at DESC, cf.featured_order ASC"
	case "popular":
		orderClause = "c.view_count DESC, cf.featured_order ASC"
	case "discussed":
		orderClause = "c.comment_count DESC, cf.featured_order ASC"
	}

	query := `SELECT c.id, c.title, c.slug, c.author_id, c.space_id, c.content_type,
	                 c.view_count, c.comment_count, c.published_at,
	                 cf.featured_order, cf.featured_until,
	                 m.storage_bucket AS cover_storage_bucket,
	                 m.storage_path AS cover_storage_path
	          FROM content_featured cf
	          JOIN contents c ON c.id = cf.content_id
	          LEFT JOIN media m ON m.id = COALESCE(c.featured_image_id, c.header_image_id)
	            AND m.deleted_at IS NULL
	          WHERE c.deleted_at IS NULL
	            AND c.status = 'published'
	            AND (cf.featured_until IS NULL OR cf.featured_until > now())`
	args := []any{}
	argIdx := 1

	if spaceID != nil {
		query += fmt.Sprintf(" AND c.space_id = $%d", argIdx)

		args = append(args, *spaceID)
		argIdx++
	}

	if contentType != "" {
		query += fmt.Sprintf(" AND c.content_type = $%d", argIdx)

		args = append(args, contentType)
		argIdx++
	}

	query += fmt.Sprintf(" ORDER BY %s LIMIT $%d", orderClause, argIdx)

	args = append(args, limit)

	rows, err := r.reader.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("rendering featured_content: %w", err)
	}
	defer rows.Close()

	items := make([]map[string]any, 0)

	for rows.Next() {
		var id, authorID, spaceID pgtype.UUID

		var title, slug, ct string

		var viewCount, commentCount int32

		var featuredOrder int32

		var publishedAt, featuredUntil interface{}

		var coverBucket, coverPath *string
		if err := rows.Scan(&id, &title, &slug, &authorID, &spaceID, &ct,
			&viewCount, &commentCount, &publishedAt,
			&featuredOrder, &featuredUntil, &coverBucket, &coverPath); err != nil {
			return nil, fmt.Errorf("scanning featured_content: %w", err)
		}

		item := map[string]any{
			"id":             id,
			"title":          title,
			"slug":           slug,
			"author_id":      authorID,
			"space_id":       spaceID,
			"content_type":   ct,
			"view_count":     viewCount,
			"comment_count":  commentCount,
			"published_at":   publishedAt,
			"featured_order": featuredOrder,
			"featured_until": featuredUntil,
		}
		if url := coverImageURL(coverBucket, coverPath); url != "" {
			item["cover_image_url"] = url
		}

		items = append(items, item)
	}

	return json.Marshal(items)
}

// renderBlogPosts returns recent blog posts (content_type='blog_post') with
// blog-specific fields. Ref: §24 Blog Extension.
func (r *Renderer) renderBlogPosts(ctx context.Context, w Widget) (json.RawMessage, error) {
	limit := widgetConfigLimit(w.Config)
	spaceID := widgetConfigSpaceID(w.Config)

	sortBy := widgetConfigString(w.Config, "sort")
	if sortBy == "" {
		sortBy = "recent"
	}

	orderClause := "c.published_at DESC"

	switch sortBy {
	case "popular":
		orderClause = "c.view_count DESC, c.published_at DESC"
	case "discussed":
		orderClause = "c.comment_count DESC, c.published_at DESC"
	}

	query := `SELECT c.id, c.title, c.slug, c.author_id, c.space_id,
	                 c.view_count, c.comment_count, c.published_at,
	                 u.username AS author_username,
	                 m.storage_bucket AS featured_storage_bucket,
	                 m.storage_path AS featured_storage_path
	          FROM contents c
	          LEFT JOIN users u ON u.id = c.author_id
	          LEFT JOIN media m ON m.id = c.featured_image_id AND m.deleted_at IS NULL
	          WHERE c.deleted_at IS NULL
	            AND c.status = 'published' AND c.content_type = 'blog_post'`
	args := []any{}

	argIdx := 1
	if spaceID != nil {
		query += fmt.Sprintf(" AND c.space_id = $%d", argIdx)

		args = append(args, *spaceID)
		argIdx++
	}

	query += fmt.Sprintf(" ORDER BY %s LIMIT $%d", orderClause, argIdx)

	args = append(args, limit)

	rows, err := r.reader.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("rendering blog_posts: %w", err)
	}
	defer rows.Close()

	items := make([]map[string]any, 0)

	for rows.Next() {
		var id, authorID, spaceID pgtype.UUID

		var title, slug, authorUsername string

		var viewCount, commentCount int32

		var publishedAt interface{}

		var featuredBucket, featuredPath *string
		if err := rows.Scan(&id, &title, &slug, &authorID, &spaceID,
			&viewCount, &commentCount, &publishedAt, &authorUsername,
			&featuredBucket, &featuredPath); err != nil {
			return nil, fmt.Errorf("scanning blog_posts: %w", err)
		}

		item := map[string]any{
			"id":              id,
			"title":           title,
			"slug":            slug,
			"author_id":       authorID,
			"author_username": authorUsername,
			"space_id":        spaceID,
			"view_count":      viewCount,
			"comment_count":   commentCount,
			"published_at":    publishedAt,
		}
		if url := coverImageURL(featuredBucket, featuredPath); url != "" {
			item["featured_image_url"] = url
		}

		items = append(items, item)
	}

	return json.Marshal(items)
}

// renderForumThreads returns hot/recent forum threads. Ref: §08 Forum Q&A.
// Config: sort="hot" (vote_score DESC, default) or "recent" or "unanswered".
func (r *Renderer) renderForumThreads(ctx context.Context, w Widget) (json.RawMessage, error) {
	limit := widgetConfigLimit(w.Config)
	spaceID := widgetConfigSpaceID(w.Config)

	sortBy := widgetConfigString(w.Config, "sort")
	if sortBy == "" {
		sortBy = "hot"
	}

	orderClause := "vote_score DESC, created_at DESC"

	switch sortBy {
	case "recent":
		orderClause = "created_at DESC"
	case "unanswered":
		orderClause = "created_at DESC"
	}

	query := `SELECT ft.id, ft.title, ft.slug, ft.author_user_id, ft.space_id,
	                 ft.solved, ft.vote_score, ft.reply_count, ft.view_count,
	                 ft.created_at
	          FROM forum_threads ft
	          WHERE ft.deleted_at IS NULL`
	if sortBy == "unanswered" {
		query += " AND ft.solved = false AND ft.reply_count = 0"
	}

	args := []any{}

	argIdx := 1
	if spaceID != nil {
		query += fmt.Sprintf(" AND ft.space_id = $%d", argIdx)

		args = append(args, *spaceID)
		argIdx++
	}

	query += fmt.Sprintf(" ORDER BY %s LIMIT $%d", orderClause, argIdx)

	args = append(args, limit)

	rows, err := r.reader.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("rendering forum_threads: %w", err)
	}
	defer rows.Close()

	items := make([]map[string]any, 0)

	for rows.Next() {
		var id, authorUserID, spaceID pgtype.UUID

		var title, slug string

		var solved bool

		var voteScore, replyCount, viewCount int32

		var createdAt interface{}
		if err := rows.Scan(&id, &title, &slug, &authorUserID, &spaceID,
			&solved, &voteScore, &replyCount, &viewCount, &createdAt); err != nil {
			return nil, fmt.Errorf("scanning forum_threads: %w", err)
		}

		items = append(items, map[string]any{
			"id":          id,
			"title":       title,
			"slug":        slug,
			"author_id":   authorUserID,
			"space_id":    spaceID,
			"solved":      solved,
			"vote_score":  voteScore,
			"reply_count": replyCount,
			"view_count":  viewCount,
			"created_at":  createdAt,
		})
	}

	return json.Marshal(items)
}

// renderJobListings returns recent job listings (content_type='job_listing').
// Ref: §63 Jobs Board. Job-specific fields are in field_values JSONB.
func (r *Renderer) renderJobListings(ctx context.Context, w Widget) (json.RawMessage, error) {
	limit := widgetConfigLimit(w.Config)
	spaceID := widgetConfigSpaceID(w.Config)

	query := `SELECT c.id, c.title, c.slug, c.author_id, c.space_id,
	                 c.field_values, c.published_at,
	                 m.storage_bucket AS cover_storage_bucket,
	                 m.storage_path AS cover_storage_path
	          FROM contents c
	          LEFT JOIN media m ON m.id = COALESCE(c.featured_image_id, c.header_image_id)
	            AND m.deleted_at IS NULL
	          WHERE c.deleted_at IS NULL
	            AND c.status = 'published' AND c.content_type = 'job_listing'`
	args := []any{}

	argIdx := 1
	if spaceID != nil {
		query += fmt.Sprintf(" AND c.space_id = $%d", argIdx)

		args = append(args, *spaceID)
		argIdx++
	}

	query += fmt.Sprintf(" ORDER BY c.published_at DESC LIMIT $%d", argIdx)

	args = append(args, limit)

	rows, err := r.reader.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("rendering job_listings: %w", err)
	}
	defer rows.Close()

	items := make([]map[string]any, 0)

	for rows.Next() {
		var id, authorID, spaceID pgtype.UUID

		var title, slug string

		var fieldValues []byte

		var publishedAt interface{}

		var coverBucket, coverPath *string
		if err := rows.Scan(&id, &title, &slug, &authorID, &spaceID,
			&fieldValues, &publishedAt, &coverBucket, &coverPath); err != nil {
			return nil, fmt.Errorf("scanning job_listings: %w", err)
		}

		var fv map[string]any
		if len(fieldValues) > 0 {
			_ = json.Unmarshal(fieldValues, &fv)
		}

		item := map[string]any{
			"id":           id,
			"title":        title,
			"slug":         slug,
			"author_id":    authorID,
			"space_id":     spaceID,
			"field_values": fv,
			"published_at": publishedAt,
		}
		if url := coverImageURL(coverBucket, coverPath); url != "" {
			item["cover_image_url"] = url
		}

		items = append(items, item)
	}

	return json.Marshal(items)
}

// renderAchievements returns recent achievement posts (content_type=
// 'achievement') joined with achievement_meta for milestone info.
// Ref: §89 Community Achievement Feed.
func (r *Renderer) renderAchievements(ctx context.Context, w Widget) (json.RawMessage, error) {
	limit := widgetConfigLimit(w.Config)
	milestoneType := widgetConfigString(w.Config, "milestone_type")

	query := `SELECT c.id, c.title, c.slug, c.author_id, c.published_at,
	                 am.milestone_type, am.destination_space_id, am.is_anonymous,
	                 m.storage_bucket AS cover_storage_bucket,
	                 m.storage_path AS cover_storage_path
	          FROM contents c
	          JOIN achievement_meta am ON am.content_id = c.id
	          LEFT JOIN media m ON m.id = COALESCE(c.featured_image_id, c.header_image_id)
	            AND m.deleted_at IS NULL
	          WHERE c.deleted_at IS NULL
	            AND c.status = 'published' AND c.content_type = 'achievement'`
	args := []any{}

	argIdx := 1
	if milestoneType != "" {
		query += fmt.Sprintf(" AND am.milestone_type = $%d", argIdx)

		args = append(args, milestoneType)
		argIdx++
	}

	query += fmt.Sprintf(" ORDER BY c.published_at DESC LIMIT $%d", argIdx)

	args = append(args, limit)

	rows, err := r.reader.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("rendering achievements: %w", err)
	}
	defer rows.Close()

	items := make([]map[string]any, 0)

	for rows.Next() {
		var id, authorID, destSpaceID pgtype.UUID

		var title, slug, milestone string

		var isAnonymous bool

		var publishedAt interface{}

		var coverBucket, coverPath *string
		if err := rows.Scan(&id, &title, &slug, &authorID, &publishedAt,
			&milestone, &destSpaceID, &isAnonymous, &coverBucket, &coverPath); err != nil {
			return nil, fmt.Errorf("scanning achievements: %w", err)
		}

		item := map[string]any{
			"id":                   id,
			"title":                title,
			"slug":                 slug,
			"author_id":            authorID,
			"milestone_type":       milestone,
			"destination_space_id": destSpaceID,
			"is_anonymous":         isAnonymous,
			"published_at":         publishedAt,
		}
		if url := coverImageURL(coverBucket, coverPath); url != "" {
			item["cover_image_url"] = url
		}

		items = append(items, item)
	}

	return json.Marshal(items)
}

// ─── Extended: domain-specific types ────────────────────────────────────

// renderAcademyTracks returns published academy tracks.
// Ref: §36 Academy Platform.
func (r *Renderer) renderAcademyTracks(ctx context.Context, w Widget) (json.RawMessage, error) {
	limit := widgetConfigLimit(w.Config)
	difficulty := widgetConfigString(w.Config, "difficulty")

	query := `SELECT lt.id, lt.title, lt.slug, lt.description, lt.difficulty, lt.cover_media_id,
	                 lt.sort_order,
	                 m.storage_bucket AS cover_storage_bucket,
	                 m.storage_path AS cover_storage_path
	          FROM academy_tracks lt
	          LEFT JOIN media m ON m.id = lt.cover_media_id AND m.deleted_at IS NULL
	          WHERE lt.is_published = true`
	args := []any{}

	argIdx := 1
	if difficulty != "" {
		query += fmt.Sprintf(" AND lt.difficulty = $%d", argIdx)

		args = append(args, difficulty)
		argIdx++
	}

	query += fmt.Sprintf(" ORDER BY lt.sort_order ASC, lt.created_at DESC LIMIT $%d", argIdx)

	args = append(args, limit)

	rows, err := r.reader.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("rendering academy_tracks: %w", err)
	}
	defer rows.Close()

	items := make([]map[string]any, 0)

	for rows.Next() {
		var id pgtype.UUID

		var title, slug, difficulty string

		var description *string

		var coverMediaID pgtype.UUID

		var sortOrder int32

		var coverBucket, coverPath *string
		if err := rows.Scan(&id, &title, &slug, &description, &difficulty,
			&coverMediaID, &sortOrder, &coverBucket, &coverPath); err != nil {
			return nil, fmt.Errorf("scanning academy_tracks: %w", err)
		}

		item := map[string]any{
			"id":             id,
			"title":          title,
			"slug":           slug,
			"description":    description,
			"difficulty":     difficulty,
			"cover_media_id": coverMediaID,
			"sort_order":     sortOrder,
		}
		if url := coverImageURL(coverBucket, coverPath); url != "" {
			item["cover_image_url"] = url
		}

		items = append(items, item)
	}

	return json.Marshal(items)
}

// renderCommunityDigest returns the latest published digest editions.
// Ref: §84 Community Digest Widget.
func (r *Renderer) renderCommunityDigest(ctx context.Context, w Widget) (json.RawMessage, error) {
	limit := widgetConfigLimit(w.Config)
	spaceID := widgetConfigSpaceID(w.Config)

	query := `SELECT id, space_id, cadence, status, scheduled_for, published_at,
	                 content, email_sent
	          FROM digest_editions
	          WHERE status = 'published'`
	args := []any{}

	argIdx := 1
	if spaceID != nil {
		query += fmt.Sprintf(" AND space_id = $%d", argIdx)

		args = append(args, *spaceID)
		argIdx++
	}

	query += fmt.Sprintf(" ORDER BY published_at DESC LIMIT $%d", argIdx)

	args = append(args, limit)

	rows, err := r.reader.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("rendering community_digest: %w", err)
	}
	defer rows.Close()

	items := make([]map[string]any, 0)

	for rows.Next() {
		var id pgtype.UUID

		var spaceID pgtype.UUID

		var cadence, status string

		var emailSent bool

		var scheduledFor, publishedAt interface{}

		var content []byte
		if err := rows.Scan(&id, &spaceID, &cadence, &status, &scheduledFor,
			&publishedAt, &content, &emailSent); err != nil {
			return nil, fmt.Errorf("scanning community_digest: %w", err)
		}

		var contentJSON map[string]any
		if len(content) > 0 {
			_ = json.Unmarshal(content, &contentJSON)
		}

		items = append(items, map[string]any{
			"id":            id,
			"space_id":      spaceID,
			"cadence":       cadence,
			"status":        status,
			"scheduled_for": scheduledFor,
			"published_at":  publishedAt,
			"content":       contentJSON,
			"email_sent":    emailSent,
		})
	}

	return json.Marshal(items)
}

// renderReviews returns recent published reviews with aggregate ratings.
// Ref: §29 Universal Ratings & Reviews.
func (r *Renderer) renderReviews(ctx context.Context, w Widget) (json.RawMessage, error) {
	limit := widgetConfigLimit(w.Config)
	entityType := widgetConfigString(w.Config, "entity_type")

	query := `SELECT r.id, r.entity_type, r.entity_id, r.reviewer_user_id,
	                 r.rating, r.title, r.body, r.created_at,
	                 u.username AS reviewer_username
	          FROM reviews r
	          LEFT JOIN users u ON u.id = r.reviewer_user_id
	          WHERE r.status = 'published'`
	args := []any{}

	argIdx := 1
	if entityType != "" {
		query += fmt.Sprintf(" AND r.entity_type = $%d", argIdx)

		args = append(args, entityType)
		argIdx++
	}

	query += fmt.Sprintf(" ORDER BY r.created_at DESC LIMIT $%d", argIdx)

	args = append(args, limit)

	rows, err := r.reader.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("rendering reviews: %w", err)
	}
	defer rows.Close()

	items := make([]map[string]any, 0)

	for rows.Next() {
		var id, entityID, reviewerUserID pgtype.UUID

		var entityType, reviewerUsername string

		var rating int16

		var title, body *string

		var createdAt interface{}
		if err := rows.Scan(&id, &entityType, &entityID, &reviewerUserID,
			&rating, &title, &body, &createdAt, &reviewerUsername); err != nil {
			return nil, fmt.Errorf("scanning reviews: %w", err)
		}

		items = append(items, map[string]any{
			"id":                id,
			"entity_type":       entityType,
			"entity_id":         entityID,
			"reviewer_id":       reviewerUserID,
			"reviewer_username": reviewerUsername,
			"rating":            rating,
			"title":             title,
			"body":              body,
			"created_at":        createdAt,
		})
	}

	return json.Marshal(items)
}

// renderComparisons returns recent comparison items.
// Ref: §30 Generic Comparison System.
func (r *Renderer) renderComparisons(ctx context.Context, w Widget) (json.RawMessage, error) {
	limit := widgetConfigLimit(w.Config)

	rows, err := r.reader.Query(ctx, `
		SELECT ci.id, ci.content_id, ci.title, ci.description, ci.image_id,
		       ci.geo_space_id, ci.position, ci.created_at,
		       m.storage_bucket AS image_storage_bucket,
		       m.storage_path AS image_storage_path
		FROM comparison_items ci
		LEFT JOIN media m ON m.id = ci.image_id AND m.deleted_at IS NULL
		WHERE ci.deleted_at IS NULL
		ORDER BY ci.created_at DESC
		LIMIT $1
	`,		limit)
	if err != nil {
		return nil, fmt.Errorf("rendering comparisons: %w", err)
	}
	defer rows.Close()

	items := make([]map[string]any, 0)

	for rows.Next() {
		var id, contentID pgtype.UUID

		var title string

		var description *string

		var imageID, geoSpaceID pgtype.UUID

		var position int32

		var createdAt interface{}

		var imageBucket, imagePath *string
		if err := rows.Scan(&id, &contentID, &title, &description, &imageID,
			&geoSpaceID, &position, &createdAt, &imageBucket, &imagePath); err != nil {
			return nil, fmt.Errorf("scanning comparisons: %w", err)
		}

		item := map[string]any{
			"id":           id,
			"content_id":   contentID,
			"title":        title,
			"description":  description,
			"image_id":     imageID,
			"geo_space_id": geoSpaceID,
			"position":     position,
			"created_at":   createdAt,
		}
		if url := coverImageURL(imageBucket, imagePath); url != "" {
			item["image_url"] = url
		}

		items = append(items, item)
	}

	return json.Marshal(items)
}

// renderCommunityOrganisations returns active community organisations.
// Ref: §101 Diaspora Community Locator.
func (r *Renderer) renderCommunityOrganisations(ctx context.Context, w Widget) (json.RawMessage, error) {
	limit := widgetConfigLimit(w.Config)
	orgType := widgetConfigString(w.Config, "org_type")
	originCommunity := widgetConfigString(w.Config, "origin_community")

	query := `SELECT id, name, slug, org_type, origin_community,
	                 destination_space_id, city_space_id, address,
	                 website_url, is_verified, is_active
	          FROM community_organisations
	          WHERE is_active = true AND deleted_at IS NULL`
	args := []any{}

	argIdx := 1
	if orgType != "" {
		query += fmt.Sprintf(" AND org_type = $%d", argIdx)

		args = append(args, orgType)
		argIdx++
	}

	if originCommunity != "" {
		query += fmt.Sprintf(" AND origin_community = $%d", argIdx)

		args = append(args, originCommunity)
		argIdx++
	}

	query += fmt.Sprintf(" ORDER BY is_verified DESC, name ASC LIMIT $%d", argIdx)

	args = append(args, limit)

	rows, err := r.reader.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("rendering community_organisations: %w", err)
	}
	defer rows.Close()

	items := make([]map[string]any, 0)

	for rows.Next() {
		var id, destSpaceID, citySpaceID pgtype.UUID

		var name, slug, orgType, originCommunity string

		var address, websiteURL *string

		var isVerified, isActive bool
		if err := rows.Scan(&id, &name, &slug, &orgType, &originCommunity,
			&destSpaceID, &citySpaceID, &address, &websiteURL,
			&isVerified, &isActive); err != nil {
			return nil, fmt.Errorf("scanning community_organisations: %w", err)
		}

		items = append(items, map[string]any{
			"id":                   id,
			"name":                 name,
			"slug":                 slug,
			"org_type":             orgType,
			"origin_community":     originCommunity,
			"destination_space_id": destSpaceID,
			"city_space_id":        citySpaceID,
			"address":              address,
			"website_url":          websiteURL,
			"is_verified":          isVerified,
			"is_active":            isActive,
		})
	}

	return json.Marshal(items)
}

// renderMentorSpotlight returns available mentors with top ratings.
// Ref: §102 Mentorship Matching.
func (r *Renderer) renderMentorSpotlight(ctx context.Context, w Widget) (json.RawMessage, error) {
	limit := widgetConfigLimit(w.Config)

	rows, err := r.reader.Query(ctx, `
		SELECT mp.id, mp.user_id, mp.bio, mp.skills, mp.goals,
		       mp.rating_avg, mp.session_count,
		       u.username, COALESCE(up.display_name, '') AS display_name
		FROM mentorship_profiles mp
		JOIN users u ON u.id = mp.user_id AND u.status = 'active'
		LEFT JOIN user_profiles up ON up.user_id = u.id
		WHERE mp.is_mentor = true
		  AND mp.is_available = true
		ORDER BY mp.rating_avg DESC, mp.session_count DESC
		LIMIT $1
	`,		limit)
	if err != nil {
		return nil, fmt.Errorf("rendering mentor_spotlight: %w", err)
	}
	defer rows.Close()

	items := make([]map[string]any, 0)

	for rows.Next() {
		var id, userID pgtype.UUID

		var username, displayName string

		var bio *string

		var skills, goals []byte

		var ratingAvg float64

		var sessionCount int32
		if err := rows.Scan(&id, &userID, &bio, &skills, &goals,
			&ratingAvg, &sessionCount, &username, &displayName); err != nil {
			return nil, fmt.Errorf("scanning mentor_spotlight: %w", err)
		}

		var skillsArr, goalsArr []any
		if len(skills) > 0 {
			_ = json.Unmarshal(skills, &skillsArr)
		}

		if len(goals) > 0 {
			_ = json.Unmarshal(goals, &goalsArr)
		}

		items = append(items, map[string]any{
			"id":            id,
			"user_id":       userID,
			"username":      username,
			"display_name":  displayName,
			"bio":           bio,
			"skills":        skillsArr,
			"goals":         goalsArr,
			"rating_avg":    ratingAvg,
			"session_count": sessionCount,
		})
	}

	return json.Marshal(items)
}

// ─── Extended: form/widget types ────────────────────────────────────────

// renderNewsletterSignup returns the widget config for an email capture form.
// This is a form widget — it doesn't query the DB but returns the config so
// the frontend can render the signup form with the correct list_id and labels.
// Submissions go to POST /api/v1/newsletter/subscribe.
// Ref: §62 Newsletter.
func (r *Renderer) renderNewsletterSignup(ctx context.Context, w Widget) (json.RawMessage, error) {
	var c struct {
		ListID      string `json:"list_id"`
		Heading     string `json:"heading"`
		Subtext     string `json:"subtext"`
		ButtonText  string `json:"button_text"`
		Placeholder string `json:"placeholder"`
	}

	if len(w.Config) > 0 {
		_ = json.Unmarshal(w.Config, &c)
	}
	// Apply defaults.
	if c.Heading == "" {
		c.Heading = "Subscribe to our newsletter"
	}

	if c.ButtonText == "" {
		c.ButtonText = "Subscribe"
	}

	if c.Placeholder == "" {
		c.Placeholder = "Enter your email"
	}

	return json.Marshal(map[string]any{
		"list_id":     c.ListID,
		"heading":     c.Heading,
		"subtext":     c.Subtext,
		"button_text": c.ButtonText,
		"placeholder": c.Placeholder,
		"endpoint":    "/api/v1/newsletter/subscribe",
	})
}
