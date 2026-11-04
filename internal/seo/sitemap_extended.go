package seo

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jedafe/japa/internal/bots"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// This file holds the extended sub-sitemap handlers (universities, courses,
// jobs, scholarships, services, wiki, calendar, scam-alerts, comparisons,
// tools). Each follows the ServeSpacesSitemap pattern: raw SQL via h.reader
// (or the shared ListPublishedSlugsForSitemap query where the rows already
// carry the gating fields), Valkey cache key jc:cache:sitemap:{type} and
// serveSitemapBytes.
//
// Queries are deliberately simple and defensive: every filter references only
// columns verified against migrations/000735_baseline_schema.up.sql (plus the
// 000742 wiki soft-delete and 000800 academy rename migrations).

// latestSimpleLastMod runs a scalar updated_at query and formats it RFC3339,
// returning "" when the query fails or yields no row.
func (h *SitemapHandler) latestSimpleLastMod(ctx context.Context, query string, args ...any) string {
	var ts time.Time

	if err := h.reader.QueryRow(ctx, query, args...).Scan(&ts); err != nil {
		return ""
	}

	return ts.Format(time.RFC3339)
}

// ServeUniversitiesSitemap serves university sitemap entries — one URL per
// active, public university space (/universities/{slug}). Universities are
// spaces joined to the 'university' space_type (Section 39 / ADR-039); the
// frontend detail route is apps/main/src/routes/(app)/universities/[slug].
func (h *SitemapHandler) ServeUniversitiesSitemap(w http.ResponseWriter, r *http.Request) {
	if h.cachedSitemapXML(w, r, "universities") {
		return
	}

	// Universities are auto-seeded wiki spaces — member gating is meaningless
	// for them, but spaces with no content activity are thin pages. Gate on
	// content (posts + forums + questions) against the same space_min_posts
	// threshold the spaces sitemap uses.
	cfg := h.sitemapQualityConfig(r.Context())
	minContent := cfg.MinPosts
	if minContent < 0 {
		minContent = bots.DefaultSitemapQualityConfig().MinPosts
	}

	// minContent == 0 disables the gate (list all active universities).
	gate := ``
	args := []any{}
	if minContent > 0 {
		gate = ` AND s.post_count + (
		       SELECT count(*) FROM contents c JOIN forum_threads ft ON ft.content_id = c.id
		       WHERE c.space_id = s.id AND c.status = 'published'
		         AND c.deleted_at IS NULL AND c.visibility = 'public'
		   ) >= $1`
		args = append(args, minContent)
	}

	rows, err := h.reader.Query(r.Context(), `SELECT s.slug, s.updated_at
		 FROM spaces s
		 JOIN space_types st ON st.id = s.space_type_id AND s.level = 'university'
		 WHERE s.status = 'active' AND s.deleted_at IS NULL AND s.visibility = 'public'`+
		gate+`
		 ORDER BY s.updated_at DESC
		 LIMIT 50000`, args...)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to generate sitemap")
		return
	}

	defer rows.Close()

	base := h.baseURL(r)
	entries := make([]urlEntry, 0)

	for rows.Next() {
		var slug string

		var updatedAt time.Time
		if err := rows.Scan(&slug, &updatedAt); err != nil {
			continue
		}

		entries = append(entries, urlEntry{
			Loc:        fmt.Sprintf("%s/universities/%s", base, slug),
			LastMod:    updatedAt.Format(time.RFC3339),
			ChangeFreq: "weekly",
			Priority:   "0.7",
		})
	}

	h.serveSitemapBytes(w, r, "universities", renderURLSetXML(entries))
}

// latestUniversitiesLastMod returns the most recent university space
// updated_at, or empty.
func (h *SitemapHandler) latestUniversitiesLastMod(ctx context.Context) string {
	return h.latestSimpleLastMod(ctx, `SELECT s.updated_at
		 FROM spaces s
		 JOIN space_types st ON st.id = s.space_type_id AND s.level = 'university'
		 WHERE s.status = 'active' AND s.deleted_at IS NULL AND s.visibility = 'public'
		 ORDER BY s.updated_at DESC LIMIT 1`)
}

// ServeUniversityCoursesSitemap serves university course-offering sitemap
// entries — one URL per published offering
// (/universities/{slug}/courses/{courseSlug}). The nested route is canonical
// (GetPublishedCourseOfferingBySlug scopes the offering to its university),
// so both slugs come from the same join used by the public API.
//
// These offerings are part of the Section 39 university surface, not the
// feature.academy learning platform — the public routes
// (/universities/{slug}/courses[/{offeringSlug}]) carry no feature Gate, so
// the sitemap carries none either (§42: sitemap gating mirrors route gating).
// is_published on the offering and catalog rows is the only inclusion gate.
func (h *SitemapHandler) ServeUniversityCoursesSitemap(w http.ResponseWriter, r *http.Request) {
	if h.cachedSitemapXML(w, r, "university-courses") {
		return
	}

	rows, err := h.reader.Query(r.Context(), `SELECT s.slug, o.slug, o.updated_at
		 FROM university_course_offerings o
		 JOIN spaces s ON s.id = o.university_id AND s.deleted_at IS NULL
		 JOIN university_course_catalog c ON c.id = o.course_id AND c.deleted_at IS NULL
		 WHERE o.deleted_at IS NULL AND o.is_published = true AND c.is_published = true
		 ORDER BY o.updated_at DESC
		 LIMIT 50000`)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to generate sitemap")
		return
	}

	defer rows.Close()

	base := h.baseURL(r)
	entries := make([]urlEntry, 0)

	for rows.Next() {
		var uniSlug, offeringSlug string

		var updatedAt time.Time
		if err := rows.Scan(&uniSlug, &offeringSlug, &updatedAt); err != nil {
			continue
		}

		entries = append(entries, urlEntry{
			Loc:        fmt.Sprintf("%s/universities/%s/courses/%s", base, uniSlug, offeringSlug),
			LastMod:    updatedAt.Format(time.RFC3339),
			ChangeFreq: "weekly",
			Priority:   "0.6",
		})
	}

	h.serveSitemapBytes(w, r, "university-courses", renderURLSetXML(entries))
}

// latestUniversityCoursesLastMod returns the most recent published offering
// updated_at, or empty.
func (h *SitemapHandler) latestUniversityCoursesLastMod(ctx context.Context) string {
	return h.latestSimpleLastMod(ctx, `SELECT o.updated_at
		 FROM university_course_offerings o
		 JOIN university_course_catalog c ON c.id = o.course_id AND c.deleted_at IS NULL
		 WHERE o.deleted_at IS NULL AND o.is_published = true AND c.is_published = true
		 ORDER BY o.updated_at DESC LIMIT 1`)
}

// expiredJobSlugs returns the slugs of job_listing contents whose
// application_deadline custom-field value is in the past. Expiry lives in the
// EAV custom_field_values table (there is no expires_at column on contents);
// the same predicate (with the ISO-date regex guard) backs the
// ExpireJobListingsPastDeadline archival query. On error it returns nil and
// the caller proceeds without expiry exclusion (logged).
func (h *SitemapHandler) expiredJobSlugs(ctx context.Context) map[string]struct{} {
	rows, err := h.reader.Query(ctx, `SELECT c.slug
		 FROM contents c
		 JOIN custom_field_values cfv ON cfv.entity_id = c.id AND cfv.entity_type = 'content'
		 JOIN custom_fields cf ON cf.id = cfv.field_id
		 WHERE c.content_type = 'job_listing'
		   AND cf.key = 'application_deadline'
		   AND (cfv.value_date < now()
		        OR (cfv.value_text ~ '^\d{4}-\d{2}-\d{2}'
		            AND pg_input_is_valid('timestamptz', cfv.value_text)
		            AND cfv.value_text::timestamptz < now()))`)
	if err != nil {
		slog.Warn("sitemap: failed to list expired jobs, skipping expiry filter", "error", err)
		return nil
	}

	defer rows.Close()

	expired := make(map[string]struct{})

	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			continue
		}

		expired[slug] = struct{}{}
	}

	return expired
}

// ServeJobsSitemap serves job-listing sitemap entries (/jobs/{slug}),
// filtered through the Section 43 quality gates and excluding listings whose
// application_deadline has passed (Section 63 / ADR-081).
func (h *SitemapHandler) ServeJobsSitemap(w http.ResponseWriter, r *http.Request) {
	if h.cachedSitemapXML(w, r, "jobs") {
		return
	}

	rows, err := h.queries.ListPublishedSlugsForSitemap(r.Context(), gen.ListPublishedSlugsForSitemapParams{ChunkLimit: sitemapChunkSize, TypeFilter: "job_listing"})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to generate sitemap")
		return
	}

	cfg := h.sitemapQualityConfig(r.Context())
	expired := h.expiredJobSlugs(r.Context())

	base := h.baseURL(r)
	entries := make([]urlEntry, 0)

	for _, c := range rows {
		if c.ContentType != "job_listing" {
			continue
		}

		if _, isExpired := expired[c.Slug]; isExpired {
			continue
		}

		isQuestion := c.IsQuestion != nil && *c.IsQuestion
		if !bots.IsContentIndexableByType(bots.ContentIndexableInput{
			ContentType:   c.ContentType,
			Status:        "published",
			Visibility:    c.Visibility,
			WordCount:     int(c.WordCount),
			CommentCount:  int(c.CommentCount),
			ShareCount:    int(c.ShareCount),
			ReactionCount: int(c.ReactionCount),
			ReplyCount:    int(c.ReplyCount),
			IsQuestion:    isQuestion,
			SkipWordGate:  c.ContentType == "forum_thread" || c.ContentType == "discussion", Solved: c.Solved,
			SeoIndexable: c.SeoIndexable,
		}, cfg) {
			continue
		}

		entries = append(entries, urlEntry{
			Loc:        fmt.Sprintf("%s/jobs/%s", base, c.Slug),
			LastMod:    c.UpdatedAt.Time.Format(time.RFC3339),
			ChangeFreq: "weekly",
			Priority:   "0.7",
		})
	}

	h.serveSitemapBytes(w, r, "jobs", renderURLSetXML(entries))
}

// latestJobsLastMod returns the most recent published job_listing updated_at,
// or empty.
func (h *SitemapHandler) latestJobsLastMod(ctx context.Context) string {
	rows, err := h.queries.ListPublishedSlugsForSitemap(ctx, gen.ListPublishedSlugsForSitemapParams{ChunkLimit: sitemapChunkSize, TypeFilter: "job_listing"})
	if err != nil {
		return ""
	}

	for _, c := range rows {
		if c.ContentType == "job_listing" {
			return c.UpdatedAt.Time.Format(time.RFC3339)
		}
	}

	return ""
}

// ServeScholarshipsSitemap serves scholarship sitemap entries
// (/scholarships/{guide_id}). The frontend detail routes on both the main
// app and the tools app are keyed by guide_id (the contents id backing the
// scholarship), and GET /scholarships/:id resolves the same key — so the
// sitemap emits the guide_id, matching the canonical frontend URL.
func (h *SitemapHandler) ServeScholarshipsSitemap(w http.ResponseWriter, r *http.Request) {
	if h.cachedSitemapXML(w, r, "scholarships") {
		return
	}

	rows, err := h.reader.Query(r.Context(), `SELECT s.guide_id, c.updated_at
		 FROM scholarships s
		 JOIN contents c ON c.id = s.guide_id
		 WHERE c.status = 'published' AND c.deleted_at IS NULL
		   AND c.visibility = 'public' AND c.seo_indexable = true
		   AND c.guide_category = 'scholarship'
		 ORDER BY c.updated_at DESC
		 LIMIT 50000`)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to generate sitemap")
		return
	}

	defer rows.Close()

	base := h.baseURL(r)
	entries := make([]urlEntry, 0)

	for rows.Next() {
		var guideID string

		var updatedAt time.Time
		if err := rows.Scan(&guideID, &updatedAt); err != nil {
			continue
		}

		entries = append(entries, urlEntry{
			Loc:        fmt.Sprintf("%s/scholarships/%s", base, guideID),
			LastMod:    updatedAt.Format(time.RFC3339),
			ChangeFreq: "weekly",
			Priority:   "0.6",
		})
	}

	h.serveSitemapBytes(w, r, "scholarships", renderURLSetXML(entries))
}

// latestScholarshipsLastMod returns the most recent published scholarship
// guide updated_at, or empty.
func (h *SitemapHandler) latestScholarshipsLastMod(ctx context.Context) string {
	return h.latestSimpleLastMod(ctx, `SELECT c.updated_at
		 FROM scholarships s
		 JOIN contents c ON c.id = s.guide_id
		 WHERE c.status = 'published' AND c.deleted_at IS NULL
		   AND c.visibility = 'public' AND c.seo_indexable = true
		   AND c.guide_category = 'scholarship'
		 ORDER BY c.updated_at DESC LIMIT 1`)
}

// ServeServicesSitemap serves service-listing sitemap entries
// (/services/{slug}). Service listings live in contents
// (content_type='service_listing'); the slug column on contents is the
// canonical detail key (GET /services/:slug, ServiceCard links to
// /services/{slug}). service_listing_meta carries no slug of its own.
func (h *SitemapHandler) ServeServicesSitemap(w http.ResponseWriter, r *http.Request) {
	if h.cachedSitemapXML(w, r, "services") {
		return
	}

	rows, err := h.queries.ListPublishedSlugsForSitemap(r.Context(), gen.ListPublishedSlugsForSitemapParams{ChunkLimit: sitemapChunkSize, TypeFilter: "service_listing"})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to generate sitemap")
		return
	}

	cfg := h.sitemapQualityConfig(r.Context())

	base := h.baseURL(r)
	entries := make([]urlEntry, 0)

	for _, c := range rows {
		if c.ContentType != "service_listing" {
			continue
		}

		isQuestion := c.IsQuestion != nil && *c.IsQuestion
		if !bots.IsContentIndexableByType(bots.ContentIndexableInput{
			ContentType:   c.ContentType,
			Status:        "published",
			Visibility:    c.Visibility,
			WordCount:     int(c.WordCount),
			CommentCount:  int(c.CommentCount),
			ShareCount:    int(c.ShareCount),
			ReactionCount: int(c.ReactionCount),
			ReplyCount:    int(c.ReplyCount),
			IsQuestion:    isQuestion,
			SkipWordGate:  c.ContentType == "forum_thread" || c.ContentType == "discussion", Solved: c.Solved,
			SeoIndexable: c.SeoIndexable,
		}, cfg) {
			continue
		}

		entries = append(entries, urlEntry{
			Loc:        fmt.Sprintf("%s/services/%s", base, c.Slug),
			LastMod:    c.UpdatedAt.Time.Format(time.RFC3339),
			ChangeFreq: "weekly",
			Priority:   "0.6",
		})
	}

	h.serveSitemapBytes(w, r, "services", renderURLSetXML(entries))
}

// latestServicesLastMod returns the most recent published service_listing
// updated_at, or empty.
func (h *SitemapHandler) latestServicesLastMod(ctx context.Context) string {
	rows, err := h.queries.ListPublishedSlugsForSitemap(ctx, gen.ListPublishedSlugsForSitemapParams{ChunkLimit: sitemapChunkSize, TypeFilter: "service_listing"})
	if err != nil {
		return ""
	}

	for _, c := range rows {
		if c.ContentType == "service_listing" {
			return c.UpdatedAt.Time.Format(time.RFC3339)
		}
	}

	return ""
}

// ServeWikiSitemap serves wiki sitemap entries — one URL per active,
// non-deleted page (/wiki/{space_slug}/{page_slug}) plus one index URL per
// space (/wiki/{space_slug}). Matches the public wiki handlers (HandleGetPage
// / HandleGetIndex) and the frontend routes.
//
// The same indexability contract as the rendered page applies: a page only
// makes the sitemap when its latest approved prose revision meets the
// seo.sitemap.wiki_min_words floor — the page itself renders noindex,follow
// below that threshold (wiki.Service dto.Indexable), and sitemaps must not
// advertise noindexed URLs.
func (h *SitemapHandler) ServeWikiSitemap(w http.ResponseWriter, r *http.Request) {
	if h.cachedSitemapXML(w, r, "wiki") {
		return
	}

	// Only pages with an approved prose revision are indexable — stubs
	// (seeded pages still awaiting first content) return 200 but carry no
	// content, so listing them would push thin pages at crawlers.
	// kind='fields' revisions hold field ops only, never page prose (§80).
	rows, err := h.reader.Query(r.Context(), `SELECT sp.slug, wp.slug, wp.updated_at,
		        COALESCE(rev.content, '{}'::jsonb)
		 FROM wiki_pages wp
		 JOIN spaces sp ON sp.id = wp.space_id AND sp.deleted_at IS NULL
		 JOIN LATERAL (
		     SELECT wr.content FROM wiki_revisions wr
		     WHERE wr.wiki_page_id = wp.id AND wr.status = 'approved'
		       AND wr.kind <> 'fields' AND wr.deleted_at IS NULL
		     ORDER BY wr.created_at DESC LIMIT 1
		 ) rev ON true
		 WHERE wp.is_active = true AND wp.deleted_at IS NULL
		 ORDER BY wp.updated_at DESC
		 LIMIT 50000`)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to generate sitemap")
		return
	}

	defer rows.Close()

	minWords := 500
	if h.settings != nil {
		minWords = h.settings.GetIntOr(r.Context(), "seo.sitemap.wiki_min_words", 500)
	}

	base := h.baseURL(r)
	entries := make([]urlEntry, 0)

	seenSpaces := make(map[string]string)

	for rows.Next() {
		var spaceSlug, pageSlug string

		var updatedAt time.Time

		var revContent []byte
		if err := rows.Scan(&spaceSlug, &pageSlug, &updatedAt, &revContent); err != nil {
			continue
		}

		// Word floor mirrors wiki.Service's dto.Indexable contract — pages
		// under the floor render noindex,follow and must not be advertised.
		if len(strings.Fields(bots.TipTapText(revContent))) < minWords {
			continue
		}

		lastMod := updatedAt.Format(time.RFC3339)
		if _, ok := seenSpaces[spaceSlug]; !ok {
			// Rows arrive newest-first, so the first sighting carries the
			// freshest lastmod for the space index page.
			seenSpaces[spaceSlug] = lastMod
		}

		entries = append(entries, urlEntry{
			Loc:        fmt.Sprintf("%s/wiki/%s/%s", base, spaceSlug, pageSlug),
			LastMod:    lastMod,
			ChangeFreq: "weekly",
			Priority:   "0.6",
		})
	}

	// Space index pages first (stable order is not required for sitemaps,
	// but index-before-pages reads better in Search Console).
	indexEntries := make([]urlEntry, 0, len(seenSpaces))
	for spaceSlug, lastMod := range seenSpaces {
		indexEntries = append(indexEntries, urlEntry{
			Loc:        fmt.Sprintf("%s/wiki/%s", base, spaceSlug),
			LastMod:    lastMod,
			ChangeFreq: "weekly",
			Priority:   "0.6",
		})
	}

	entries = append(indexEntries, entries...)
	entries = append(entries, h.wikiEntitySitemapEntries(r.Context(), base, minWords)...)

	h.serveSitemapBytes(w, r, "wiki", renderURLSetXML(entries))
}

// wikiEntitySitemapQueries resolve entity_id → (slug, discriminator) per
// wiki entity kind for the sitemap. Table and filter choices mirror the
// per-kind *ForWiki queries in internal/db/queries/wiki.sql — a URL is
// only advertised when the entity itself resolves publicly.
var wikiEntitySitemapQueries = map[string]string{
	"visa_route":   `SELECT id::text, slug, COALESCE(visa_category, '') FROM visa_routes WHERE id::text = ANY($1) AND is_active = true AND slug <> ''`,
	"organization": `SELECT id::text, slug, COALESCE(org_type, '') FROM organizations WHERE id::text = ANY($1) AND deleted_at IS NULL AND status = 'active' AND slug <> ''`,
	"embassy":      `SELECT id::text, slug, '' AS disc FROM embassies WHERE id::text = ANY($1) AND is_active = true AND slug <> ''`,
	"course":       `SELECT id::text, slug, '' AS disc FROM university_course_offerings WHERE id::text = ANY($1) AND deleted_at IS NULL AND is_published = true AND slug <> ''`,
	"program":      `SELECT id::text, slug, '' AS disc FROM university_course_catalog WHERE id::text = ANY($1) AND deleted_at IS NULL AND is_published = true AND slug <> ''`,
	"scholarship":  `SELECT id::text, slug, '' AS disc FROM scholarships WHERE id::text = ANY($1) AND slug <> ''`,
}

// wikiEntityTypeDisc mirrors wiki.EntityTypeDisc — kept local because
// importing internal/wiki here would cycle (wiki → ai → auth → worker →
// seo). The canonical pick below mirrors wiki.CanonicalEntityTypeSlug
// and must stay in lockstep with it: both resolve the same type key for
// the same (entityDisc, typeSet) input.
type wikiEntityTypeDisc struct {
	Slug               string
	DiscriminatorValue string
}

// entityDiscriminatorValue mirrors wiki.DiscriminatorValue — first value
// of an entity_discriminator {column: value} map; "" when empty/malformed.
func entityDiscriminatorValue(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	for _, v := range m {
		return v
	}
	return ""
}

// canonicalEntityTypeSlug mirrors wiki.CanonicalEntityTypeSlug: the
// discriminator-matching type wins, then the generic undiscriminated
// type, then the first by seed order.
func canonicalEntityTypeSlug(entityDiscriminator string, types []wikiEntityTypeDisc) string {
	generic := ""
	for _, t := range types {
		if t.DiscriminatorValue == "" {
			if generic == "" {
				generic = t.Slug
			}
			continue
		}
		if entityDiscriminator != "" && t.DiscriminatorValue == entityDiscriminator {
			return t.Slug
		}
	}
	if generic != "" {
		return generic
	}
	if len(types) > 0 {
		return types[0].Slug
	}
	return ""
}

// wikiEntitySitemapEntries returns sitemap entries for entity-bound wiki
// pages (/wiki/e/{type_slug}/{entity}/{page_slug}) — the wiki_pages rows
// whose parent is an entity row rather than a space, so the space join
// above never sees them. Same indexability contract (approved prose at
// the min-words floor); the URL's type slug is the entity's canonical
// type key — several type keys can serve one entity page (a student visa
// route resolves under both 'visa_route' and 'student_visa'), and
// advertising all of them would mint duplicate entries, so the pick
// mirrors the rendered page's canonical (wiki.CanonicalEntityTypeSlug).
func (h *SitemapHandler) wikiEntitySitemapEntries(ctx context.Context, base string, minWords int) []urlEntry {
	// Entity-bound type set per kind, in the same deterministic order the
	// canonical pick expects.
	typeRows, err := h.reader.Query(ctx, `SELECT st.slug, wtc.entity_kind, wtc.entity_discriminator
		 FROM wiki_type_configs wtc
		 JOIN space_types st ON st.id = wtc.space_type_id
		 WHERE wtc.entity_kind IS NOT NULL AND wtc.enabled = true
		 ORDER BY wtc.position, st.slug`)
	if err != nil {
		return nil
	}
	kindTypes := map[string][]wikiEntityTypeDisc{}
	for typeRows.Next() {
		var slug, kind string
		var disc []byte
		if err := typeRows.Scan(&slug, &kind, &disc); err != nil {
			continue
		}
		kindTypes[kind] = append(kindTypes[kind], wikiEntityTypeDisc{
			Slug:               slug,
			DiscriminatorValue: entityDiscriminatorValue(disc),
		})
	}
	typeRows.Close()

	rows, err := h.reader.Query(ctx, `SELECT wp.entity_type, wp.entity_id::text, wp.slug, wp.updated_at,
		        COALESCE(rev.content, '{}'::jsonb)
		 FROM wiki_pages wp
		 JOIN LATERAL (
		     SELECT wr.content FROM wiki_revisions wr
		     WHERE wr.wiki_page_id = wp.id AND wr.status = 'approved'
		       AND wr.kind <> 'fields' AND wr.deleted_at IS NULL
		     ORDER BY wr.created_at DESC LIMIT 1
		 ) rev ON true
		 WHERE wp.entity_id IS NOT NULL AND wp.is_active = true AND wp.deleted_at IS NULL
		 ORDER BY wp.updated_at DESC
		 LIMIT 50000`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	type entityPage struct {
		slug      string
		updatedAt time.Time
	}
	entityIDs := map[string][]string{} // kind → entity ids needing a slug lookup
	pagesByEntity := map[string]map[string][]entityPage{}
	for rows.Next() {
		var kind, entityID, slug string
		var updatedAt time.Time
		var revContent []byte
		if err := rows.Scan(&kind, &entityID, &slug, &updatedAt, &revContent); err != nil {
			continue
		}
		// Same word floor as the space join — stubs stay out.
		if len(strings.Fields(bots.TipTapText(revContent))) < minWords {
			continue
		}
		if _, ok := pagesByEntity[kind]; !ok {
			pagesByEntity[kind] = map[string][]entityPage{}
		}
		if _, ok := pagesByEntity[kind][entityID]; !ok {
			entityIDs[kind] = append(entityIDs[kind], entityID)
		}
		pagesByEntity[kind][entityID] = append(pagesByEntity[kind][entityID], entityPage{slug, updatedAt})
	}

	entries := make([]urlEntry, 0)
	for kind, ids := range entityIDs {
		q, ok := wikiEntitySitemapQueries[kind]
		if !ok {
			// A kind with indexable pages but no slug-lookup entry silently
			// drops out of the sitemap — warn so a new entity kind can't
			// go unnoticed (registry drift guard).
			slog.Warn("sitemap: entity kind has wiki pages but no slug query",
				"kind", kind, "entities", len(ids))
			continue
		}
		if len(kindTypes[kind]) == 0 {
			slog.Warn("sitemap: entity kind has wiki pages but no enabled type",
				"kind", kind)
			continue
		}
		erows, err := h.reader.Query(ctx, q, ids)
		if err != nil {
			slog.Warn("sitemap: entity slug lookup failed", "kind", kind, "error", err)
			continue
		}
		for erows.Next() {
			var entityID, entitySlug, entityDisc string
			if err := erows.Scan(&entityID, &entitySlug, &entityDisc); err != nil {
				continue
			}
			typeSlug := canonicalEntityTypeSlug(entityDisc, kindTypes[kind])
			if typeSlug == "" {
				continue
			}
			for _, p := range pagesByEntity[kind][entityID] {
				entries = append(entries, urlEntry{
					Loc:        fmt.Sprintf("%s/wiki/e/%s/%s/%s", base, typeSlug, entitySlug, p.slug),
					LastMod:    p.updatedAt.Format(time.RFC3339),
					ChangeFreq: "weekly",
					Priority:   "0.6",
				})
			}
		}
		erows.Close()
	}
	return entries
}

// latestWikiLastMod returns the most recent active wiki page updated_at, or
// empty.
func (h *SitemapHandler) latestWikiLastMod(ctx context.Context) string {
	return h.latestSimpleLastMod(ctx, `SELECT wp.updated_at
		 FROM wiki_pages wp
		 WHERE wp.is_active = true AND wp.deleted_at IS NULL
		 ORDER BY wp.updated_at DESC LIMIT 1`)
}

// ServeCalendarSitemap serves community-calendar sitemap entries
// (/calendar/{id}). Only published, non-deleted entries are listed,
// matching CalendarGetEntry and the frontend /calendar/[id] route.
func (h *SitemapHandler) ServeCalendarSitemap(w http.ResponseWriter, r *http.Request) {
	if h.cachedSitemapXML(w, r, "calendar") {
		return
	}

	rows, err := h.reader.Query(r.Context(), `SELECT id, updated_at
		 FROM calendar_entries
		 WHERE status = 'published' AND deleted_at IS NULL
		 ORDER BY updated_at DESC
		 LIMIT 50000`)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to generate sitemap")
		return
	}

	defer rows.Close()

	base := h.baseURL(r)
	entries := make([]urlEntry, 0)

	for rows.Next() {
		var id string

		var updatedAt time.Time
		if err := rows.Scan(&id, &updatedAt); err != nil {
			continue
		}

		entries = append(entries, urlEntry{
			Loc:        fmt.Sprintf("%s/calendar/%s", base, id),
			LastMod:    updatedAt.Format(time.RFC3339),
			ChangeFreq: "weekly",
			Priority:   "0.5",
		})
	}

	h.serveSitemapBytes(w, r, "calendar", renderURLSetXML(entries))
}

// latestCalendarLastMod returns the most recent published calendar entry
// updated_at, or empty.
func (h *SitemapHandler) latestCalendarLastMod(ctx context.Context) string {
	return h.latestSimpleLastMod(ctx, `SELECT updated_at
		 FROM calendar_entries
		 WHERE status = 'published' AND deleted_at IS NULL
		 ORDER BY updated_at DESC LIMIT 1`)
}

// ServeScamAlertsSitemap serves scam-alert sitemap entries
// (/scam-alerts/{id}). Only confirmed, non-deleted reports are listed —
// under_review/debunked/duplicate reports must never be indexed (Section 81
// status machine). Matches the public HandleGet route and frontend.
func (h *SitemapHandler) ServeScamAlertsSitemap(w http.ResponseWriter, r *http.Request) {
	if h.cachedSitemapXML(w, r, "scam-alerts") {
		return
	}

	rows, err := h.reader.Query(r.Context(), `SELECT id, updated_at
		 FROM scam_reports
		 WHERE status = 'confirmed' AND deleted_at IS NULL
		 ORDER BY updated_at DESC
		 LIMIT 50000`)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to generate sitemap")
		return
	}

	defer rows.Close()

	base := h.baseURL(r)
	entries := make([]urlEntry, 0)

	for rows.Next() {
		var id string

		var updatedAt time.Time
		if err := rows.Scan(&id, &updatedAt); err != nil {
			continue
		}

		entries = append(entries, urlEntry{
			Loc:        fmt.Sprintf("%s/scam-alerts/%s", base, id),
			LastMod:    updatedAt.Format(time.RFC3339),
			ChangeFreq: "weekly",
			Priority:   "0.5",
		})
	}

	h.serveSitemapBytes(w, r, "scam-alerts", renderURLSetXML(entries))
}

// latestScamAlertsLastMod returns the most recent confirmed scam report
// updated_at, or empty.
func (h *SitemapHandler) latestScamAlertsLastMod(ctx context.Context) string {
	return h.latestSimpleLastMod(ctx, `SELECT updated_at
		 FROM scam_reports
		 WHERE status = 'confirmed' AND deleted_at IS NULL
		 ORDER BY updated_at DESC LIMIT 1`)
}

// ServeComparisonsSitemap serves comparison sitemap entries. Published
// comparison contents are listed at their canonical frontend route
// (/comparisons/{slug}, per contentLink.ts) plus the /compare/{slug} JCE
// workspace route, which resolves the same backing slug
// (GET /api/v1/jce/comparisons/{id|slug}). Rows pass the same Section 43
// quality gate as the content sitemap.
func (h *SitemapHandler) ServeComparisonsSitemap(w http.ResponseWriter, r *http.Request) {
	if h.cachedSitemapXML(w, r, "comparisons") {
		return
	}

	rows, err := h.queries.ListPublishedSlugsForSitemap(r.Context(), gen.ListPublishedSlugsForSitemapParams{ChunkLimit: sitemapChunkSize, TypeFilter: "comparison"})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to generate sitemap")
		return
	}

	cfg := h.sitemapQualityConfig(r.Context())

	base := h.baseURL(r)
	entries := make([]urlEntry, 0)

	for _, c := range rows {
		if c.ContentType != "comparison" {
			continue
		}

		isQuestion := c.IsQuestion != nil && *c.IsQuestion
		if !bots.IsContentIndexableByType(bots.ContentIndexableInput{
			ContentType:   c.ContentType,
			Status:        "published",
			Visibility:    c.Visibility,
			WordCount:     int(c.WordCount),
			CommentCount:  int(c.CommentCount),
			ShareCount:    int(c.ShareCount),
			ReactionCount: int(c.ReactionCount),
			ReplyCount:    int(c.ReplyCount),
			IsQuestion:    isQuestion,
			SkipWordGate:  c.ContentType == "forum_thread" || c.ContentType == "discussion", Solved: c.Solved,
			SeoIndexable: c.SeoIndexable,
		}, cfg) {
			continue
		}

		lastMod := c.UpdatedAt.Time.Format(time.RFC3339)
		// Only the canonical /comparisons/{slug} route is listed — the
		// /compare/{slug} JCE workspace renders the same backing slug and is
		// noindexed + canonicalized to /comparisons/{slug}, so emitting both
		// would be a sitemap-level duplicate.
		entries = append(entries, urlEntry{
			Loc:        fmt.Sprintf("%s/comparisons/%s", base, c.Slug),
			LastMod:    lastMod,
			ChangeFreq: "weekly",
			Priority:   "0.6",
		})
	}

	h.serveSitemapBytes(w, r, "comparisons", renderURLSetXML(entries))
}

// latestComparisonsLastMod returns the most recent published comparison
// content updated_at, or empty.
func (h *SitemapHandler) latestComparisonsLastMod(ctx context.Context) string {
	rows, err := h.queries.ListPublishedSlugsForSitemap(ctx, gen.ListPublishedSlugsForSitemapParams{ChunkLimit: sitemapChunkSize, TypeFilter: "comparison"})
	if err != nil {
		return ""
	}

	for _, c := range rows {
		if c.ContentType == "comparison" {
			return c.UpdatedAt.Time.Format(time.RFC3339)
		}
	}

	return ""
}

// mainAppToolPaths is the static list of indexable tool pages served by the
// main SvelteKit app (apps/main/src/routes/(app)/tools/*). These are
// editorial SSR pages (not DB rows), so the sitemap emits the known paths
// directly. Per-tool dynamic sub-routes (e.g. /tools/visa-routes/[id]) are
// intentionally excluded — they are app-state URLs, not indexable content.
// toolSitemapFlag maps each /tools/{tool} sitemap path to its feature_flags
// key (mirrors apps/main/src/lib/tools/registry.ts enabledSetting). A tool
// drops out of the sitemap when its flag is off — the page 404s, so the
// sitemap must not advertise it.
var toolSitemapFlag = map[string]string{
	"arrival-checklist":  "arrival_checklist.enabled",
	"attestation-guide":  "attestation_guide.enabled",
	"community-locator":  "community_locator.enabled",
	"cost-of-living":     "col_calculator.enabled",
	"credential-checker": "credential_checker.enabled",
	"crs-calculator":     "crs_calculator.enabled",
	"currency-converter": "fx.enabled",
	"cv-formatter":       "cv_formatter.enabled",
	"eca-guide":          "eca_guide.enabled",
	"embassy-alerter":    "appointment_alert.enabled",
	"housing-guide":      "housing_guide.enabled",
	"language-test":      "language_test_hub.enabled",
	"pof-calculator":     "pof_calculator.enabled",
	"processing-tracker": "visa_timeline.enabled",
	"readiness":          "assessment.enabled",
	"remittance":         "remittance.enabled",
	"sop-generator":      "sop_generator.enabled",
	"tax-calculator":     "tax_calculator.enabled",
	"travel":             "travel.enabled",
	"visa-checklist":     "checklist.enabled",
	"visa-routes":        "visa_routes.enabled",
}

var mainAppToolPaths = []string{
	"arrival-checklist",
	"attestation-guide",
	"community-locator",
	"cost-of-living",
	"credential-checker",
	"crs-calculator",
	"currency-converter",
	"cv-formatter",
	"eca-guide",
	"embassy-alerter",
	"housing-guide",
	"language-test",
	"pof-calculator",
	"processing-tracker",
	"readiness",
	"remittance",
	"sop-generator",
	"tax-calculator",
	"travel",
	"visa-checklist",
	"visa-routes",
}

// ServeToolsSitemap serves the static tool-pages sitemap (/tools/{tool}).
func (h *SitemapHandler) ServeToolsSitemap(w http.ResponseWriter, r *http.Request) {
	if h.cachedSitemapXML(w, r, "tools") {
		return
	}

	base := h.baseURL(r)
	entries := make([]urlEntry, 0, len(mainAppToolPaths)+1)

	entries = append(entries, urlEntry{
		Loc:        base + "/tools",
		ChangeFreq: "weekly",
		Priority:   "0.6",
	})

	for _, tool := range mainAppToolPaths {
		// §42: skip tools whose feature flag is off — the page 404s.
		if flag, ok := toolSitemapFlag[tool]; ok && !h.flagOn(r.Context(), flag) {
			continue
		}
		entries = append(entries, urlEntry{
			Loc:        base + "/tools/" + tool,
			ChangeFreq: "monthly",
			Priority:   "0.5",
		})
	}

	h.serveSitemapBytes(w, r, "tools", renderURLSetXML(entries))
}
