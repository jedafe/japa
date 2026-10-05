package spaces

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/worker"
)

// ErrWikiRateLimited indicates the Wikipedia/Wikidata API returned 429.
// Callers should retry with backoff (Asynq will retry the task).
var ErrWikiRateLimited = errors.New("wiki API rate limited")

// ErrWikiNotFound indicates the Wikipedia/Wikidata API returned 404 — no
// article exists for the given title. This is not retryable.
var ErrWikiNotFound = errors.New("wiki article not found")

// wikiRateLimiter enforces a minimum interval between external Wiki API
// calls (Wikidata + Wikipedia). Wikidata's SPARQL endpoint rate-limits
// aggressively (~1 request / 2s for anonymous clients), and Wikipedia's
// REST API has similar limits. The limiter is global so concurrent worker
// goroutines share a single queue. The interval is configurable at runtime
// via SetWikiRateInterval (BUG-005: spaces.wiki_rate_interval_ms setting).
var wikiRateLimiter = &wikiLimiter{interval: 2 * time.Second}

type wikiLimiter struct {
	mu       sync.Mutex
	interval time.Duration
	lastCall time.Time
}

// SetWikiRateInterval updates the minimum interval between Wiki API calls.
// Called from the worker bootstrap with the value from system_settings
// (spaces.wiki_rate_interval_ms). A value of 0 or less keeps the default.
func SetWikiRateInterval(d time.Duration) {
	if d <= 0 {
		return
	}

	wikiRateLimiter.mu.Lock()
	wikiRateLimiter.interval = d
	wikiRateLimiter.mu.Unlock()
}

// wait blocks until at least l.interval has elapsed since the last call.
// The lock is held during the wait so concurrent goroutines serialize —
// each call is spaced at least l.interval apart, regardless of how many
// workers are running.
func (l *wikiLimiter) wait(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.lastCall.IsZero() {
		l.lastCall = time.Now()
		return nil
	}

	remaining := l.interval - time.Since(l.lastCall)
	if remaining > 0 {
		// Use a context-aware wait so we don't block forever if the
		// task context is cancelled.
		timer := time.NewTimer(remaining)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	l.lastCall = time.Now()

	return nil
}

// CustomFieldsEnricher is the subset of customfields.Service the ProcessorAdapter
// needs to upsert university-specific field values (founded_year, student_population,
// official_website_url) from Wikidata enrichment data.
type CustomFieldsEnricher interface {
	UpsertFieldValue(ctx context.Context, params gen.UpsertCustomFieldValueParams) (gen.CustomFieldValue, error)
	GetFieldValue(ctx context.Context, fieldID pgtype.UUID, entityType string, entityID pgtype.UUID) (gen.CustomFieldValue, error)
}

// UniversityLogoEnqueuer is the subset of worker.Enqueuer the ProcessorAdapter
// needs to enqueue university:fetch_logo tasks when Wikidata returns a logo URL.
type UniversityLogoEnqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// MediaUploader is the subset of media.Service the ProcessorAdapter needs to
// ingest remote images (flagcdn flags, Wikimedia Commons files, Wikipedia
// article images) into platform media storage — spaces then serve their
// logo/header from our own store instead of hotlinking external sources at
// render time (Section 05 Auto-Population).
type MediaUploader interface {
	StoreFromURL(ctx context.Context, uploaderID pgtype.UUID, sourceURL, filename string) (pgtype.UUID, error)
}

// ProcessorAdapter implements worker.SpaceAutoPopulateProcessor by delegating
// to the spaces service's auto-population logic.
type ProcessorAdapter struct {
	svc           *Service
	customFields  CustomFieldsEnricher   // nil = skip custom field enrichment
	uniEnqueuer   UniversityLogoEnqueuer // nil = skip logo fetch enqueue
	mediaUploader MediaUploader          // nil = skip geo logo/header ingest
	systemUserID  pgtype.UUID            // for media attribution in university:fetch_logo
}

// NewProcessorAdapter creates a ProcessorAdapter backed by the given spaces service.
func NewProcessorAdapter(svc *Service) *ProcessorAdapter {
	return &ProcessorAdapter{svc: svc}
}

// SetCustomFieldsEnricher wires the custom fields service used to upsert
// university-specific field values (founded_year, student_population,
// official_website_url) from Wikidata enrichment data. Optional — if not
// called, enrichment data is stored only as wiki_content JSON.
func (a *ProcessorAdapter) SetCustomFieldsEnricher(cf CustomFieldsEnricher) {
	a.customFields = cf
}

// SetUniversityLogoEnqueuer wires the enqueuer used to trigger
// university:fetch_logo when Wikidata returns a logo URL. Optional — if not
// called, logo URLs are stored in wiki_content but not downloaded.
func (a *ProcessorAdapter) SetUniversityLogoEnqueuer(e UniversityLogoEnqueuer) {
	a.uniEnqueuer = e
}

// SetSystemUserID sets the user ID used for media attribution when
// enqueueing university:fetch_logo. This should be a valid user ID (e.g. the
// super admin). If unset, logo fetch tasks will be skipped by the processor.
func (a *ProcessorAdapter) SetSystemUserID(id pgtype.UUID) {
	a.systemUserID = id
}

// SetMediaUploader wires the media service used to download remote images
// (flags, coats of arms, article photos) into platform storage and attach
// them to the space's logo_media_id/header_image_id. Optional — if not
// called, auto-populate stores wiki_content only.
func (a *ProcessorAdapter) SetMediaUploader(m MediaUploader) {
	a.mediaUploader = m
}

// storeSpaceImage downloads a remote image into the media store and attaches
// it to the space — kind "logo" sets logo_media_id, "header" sets
// header_image_id. Best-effort: failures are logged, never returned (the
// wiki_content enrichment is already stored and a missing image falls back
// to the letter avatar/gradient in the UI).
func (a *ProcessorAdapter) storeSpaceImage(ctx context.Context, space *gen.GetSpaceByIDRow, imageURL, kind string) {
	if imageURL == "" {
		return
	}
	if a.mediaUploader == nil {
		slog.Warn("space:auto_populate — skipping image store, no media uploader wired", "space_id", space.ID.String(), "kind", kind)
		return
	}
	if !a.systemUserID.Valid {
		slog.Warn("space:auto_populate — skipping image store, no system user ID", "space_id", space.ID.String(), "kind", kind)
		return
	}

	mediaID, err := a.mediaUploader.StoreFromURL(ctx, a.systemUserID, imageURL, "space-"+kind+"-"+space.ID.String())
	if err != nil {
		slog.Warn("space:auto_populate — image download/store failed", "space_id", space.ID.String(), "kind", kind, "url", imageURL, "error", err)
		return
	}

	if kind == "header" {
		_, err = a.svc.UpdateSpaceHeaderImageID(ctx, gen.UpdateSpaceHeaderImageIDParams{
			ID:            space.ID,
			HeaderImageID: mediaID,
		})
	} else {
		_, err = a.svc.UpdateSpaceLogoMediaID(ctx, gen.UpdateSpaceLogoMediaIDParams{
			ID:          space.ID,
			LogoMediaID: mediaID,
		})
	}
	if err != nil {
		slog.Warn("space:auto_populate — attaching image to space failed", "space_id", space.ID.String(), "kind", kind, "media_id", mediaID.String(), "error", err)
		return
	}

	slog.Info("space:auto_populate — stored space image", "space_id", space.ID.String(), "kind", kind, "media_id", mediaID.String())
}

// AutoPopulateSpace seeds a newly created space with default content based
// on its space type. For geo spaces, this includes fetching Wikipedia data
// and flag images. For other space types, it creates default rules and tags.
func (a *ProcessorAdapter) AutoPopulateSpace(ctx context.Context, spaceID string) error {
	slog.Info("space:auto_populate", "space_id", spaceID)

	if a.svc == nil {
		slog.Warn("space:auto_populate — service not initialized, skipping")
		return nil
	}

	var sid pgtype.UUID
	if err := sid.Scan(spaceID); err != nil {
		slog.Warn("space:auto_populate — invalid space_id", "error", err)
		return nil
	}

	// Fetch the space to determine its type.
	space, err := a.svc.GetSpace(ctx, sid)
	if err != nil {
		slog.Error("space:auto_populate — failed to fetch space", "error", err, "space_id", spaceID)
		return err
	}

	// Auto-populate based on space type.
	spaceType := ""
	if st, err := a.svc.GetSpaceType(ctx, space.SpaceTypeID); err == nil {
		spaceType = st.Slug
	}

	var populateErr error

	switch spaceType {
	case "country", "state", "city":
		populateErr = a.populateGeoSpace(ctx, &space, spaceType)
	case "university":
		populateErr = a.populateUniversitySpace(ctx, &space)
	case "attraction":
		populateErr = a.populateAttractionSpace(ctx, &space)
	default:
		slog.Info("space:auto_populate — custom space, no auto-population needed", "space_id", spaceID, "type", spaceType)
	}

	if errors.Is(populateErr, ErrWikiRateLimited) {
		// Tag the failure for the server's RetryDelayFunc — upstream
		// rate limits get a hard backoff, not exponential seconds.
		return fmt.Errorf("%w: %w", populateErr, worker.ErrUpstreamRateLimited)
	}

	return populateErr
}

// populateGeoSpace fetches a Wikipedia summary for the space and stores it
// as WikiContent. For countries, the geo_code (ISO 3166-1 alpha-2) is used
// to construct the Wikipedia article title. It also persists the space's
// logo and header images into platform media storage (Section 05
// Auto-Population): flagcdn flags for countries, Wikidata flag/coat-of-arms
// emblems for states and cities, and the Wikipedia article image as the
// header. Images are stored once — only when the space has none — so
// surfaces serve them from our own store rather than hotlinking.
func (a *ProcessorAdapter) populateGeoSpace(ctx context.Context, space *gen.GetSpaceByIDRow, spaceType string) error {
	// Build the Wikipedia article title from the space name. For states,
	// prefer "{name} State" — the bare name often resolves to a same-named
	// city or an unrelated topic ("Ogun" → the deity, "Lagos" → the city).
	var wikiSummary *wikipediaSummary
	var wikiErr error
	title := space.Name

	titles := []string{space.Name}
	if spaceType == "state" {
		titles = []string{space.Name + " State", space.Name}
	}

	var headerURL string

	// §80 multi-P131 linking: the Wikidata entity is resolved before the
	// summary write so the space's wiki_content stamps its QID — that
	// stamp is what lets attraction P131 lookups resolve back to this
	// geo space (SpaceIDsByWikidataIDs).
	ent := a.geoWikidataEntity(ctx, space, spaceType)
	qid := ""
	if ent != nil {
		qid = ent.WikidataID
	}

	for _, t := range titles {
		s, err := fetchWikipediaSummary(ctx, t)
		if err == nil {
			wikiSummary, title = s, t

			break
		}
		// Non-retryable 404: try the next title candidate. Retryable errors
		// (429 rate limit, network errors) are kept so Asynq retries the
		// task for wiki_content — but image enrichment still runs below, so
		// each retry makes progress instead of aborting empty-handed.
		if !errors.Is(err, ErrWikiNotFound) {
			slog.Warn("space:auto_populate — Wikipedia fetch failed, will retry", "space_id", space.ID.String(), "title", t, "error", err)
			wikiErr = fmt.Errorf("fetching Wikipedia summary for %q: %w", t, err)

			break
		}
	}

	if wikiSummary == nil {
		if wikiErr == nil {
			slog.Warn("space:auto_populate — no Wikipedia article found", "space_id", space.ID.String(), "title", title)
		}
		// No summary — still stamp the QID onto wiki_content when it's
		// missing so P131 resolution works for article-less geo spaces.
		a.stampSpaceWikidataID(ctx, space, qid)
	} else {
		// Update the space's WikiContent with the fetched summary, merging
		// the resolved Wikidata QID into the stored JSON.
		wikiJSON, err := marshalWikiContentWithQID(wikiSummary, qid)
		if err != nil {
			slog.Warn("space:auto_populate — failed to marshal wiki summary", "error", err)
			return nil
		}

		if _, err := a.svc.UpdateSpaceWikiContent(ctx, space.ID, wikiJSON); err != nil {
			slog.Warn("space:auto_populate — failed to update wiki content", "space_id", space.ID.String(), "error", err)
			return nil
		}

		headerURL = firstNonEmpty(wikiSummary.OriginalImage, wikiSummary.Thumbnail)
		slog.Info("space:auto_populate — populated wiki content", "space_id", space.ID.String(), "type", spaceType, "name", space.Name)
	}

	// §05 Auto-Population: persist the space logo and header as platform
	// media. Guarded by .Valid so task re-runs don't re-download. One
	// Wikidata entity resolves both the emblem and the P18 image — the
	// P18 photo is preferred over the Wikipedia original for the header
	// because Wikipedia's lead image is often the flag/map itself.
	if !space.LogoMediaID.Valid {
		switch {
		case spaceType == "country" && space.GeoCode != nil && *space.GeoCode != "":
			// flagcdn.com serves the ISO 3166-1 alpha-2 flag as PNG (§05).
			// PNG, not SVG — active-content types are excluded from media
			// storage (media.safeMediaExts) and would land as undisplayable
			// .bin blobs.
			a.storeSpaceImage(ctx, space, "https://flagcdn.com/w640/"+strings.ToLower(*space.GeoCode)+".png", "logo")
		case spaceType == "state" || spaceType == "city":
			logoURL := ""
			if ent != nil {
				logoURL = firstNonEmpty(ent.Flag, ent.CoatOfArms)
			}
			a.storeSpaceImage(ctx, space, logoURL, "logo")
		}
	}
	if !space.HeaderImageID.Valid {
		if ent != nil {
			headerURL = firstNonEmpty(ent.Image, headerURL)
		}
		a.storeSpaceImage(ctx, space, headerURL, "header")
	}

	return wikiErr
}

// geoTypeKeywords disambiguates Wikidata search hits by level — "Georgia"
// alone would otherwise resolve to the U.S. state when enriching the
// country. A hit scores a point per match; higher-scored hits probe first.
var geoTypeKeywords = map[string][]string{
	"country": {"country", "sovereign"},
	"state":   {"state", "region", "province", "oblast", "prefecture", "territory", "republic"},
	"city":    {"city", "town", "village", "municipality", "commune", "capital"},
}

// geoWikidataEntity resolves the best-matching Wikidata entity for a geo
// space — hits are ranked by mentions of the parent space name (e.g.
// "Nigeria") and the level keyword (e.g. "state"), then probed until one
// carries usable imagery. Returns nil when no match or fetch fails (image
// enrichment is best-effort).
func (a *ProcessorAdapter) geoWikidataEntity(ctx context.Context, space *gen.GetSpaceByIDRow, spaceType string) *wikidataUniversity {
	parentName := ""
	if space.ParentID.Valid {
		if parent, err := a.svc.GetSpace(ctx, space.ParentID); err == nil {
			parentName = strings.ToLower(parent.Name)
		}
	}

	// The bare name outranks "Name, Parent" queries — comma phrases match
	// poorly in wbsearchentities ("Lagos, Nigeria" → "Lagosians").
	results, err := searchWikidataEntities(ctx, space.Name, 5)
	if err != nil {
		return nil
	}

	keywords := geoTypeKeywords[spaceType]
	score := func(r wikidataSearchResult) int {
		desc := strings.ToLower(r.Description)
		s := 0
		if parentName != "" && strings.Contains(desc, parentName) {
			s += 2
		}
		for _, kw := range keywords {
			if strings.Contains(desc, kw) {
				s++
				break
			}
		}
		return s
	}

	probed := 0
	for want := 3; want >= 0 && probed < 2; want-- {
		for _, r := range results {
			if score(r) != want || probed >= 2 {
				continue
			}
			probed++
			ent, err := fetchWikidataEntity(ctx, r.ID)
			if err != nil {
				continue
			}
			if ent.Flag != "" || ent.CoatOfArms != "" || ent.Image != "" {
				return ent
			}
		}
	}

	return nil
}

// populateUniversitySpace fetches university data from Wikidata SPARQL and
// stores it as WikiContent. Retrieves: description, country, founding year,
// student count, official website, logo, and coordinates. When the custom
// fields enricher is wired, it also upserts custom_field_values for
// founded_year, student_population, and official_website_url. When the
// university logo enqueuer is wired and a logo URL is found, it enqueues
// university:fetch_logo to download and store the logo.
func (a *ProcessorAdapter) populateUniversitySpace(ctx context.Context, space *gen.GetSpaceByIDRow) error {
	uniData, err := fetchWikidataUniversity(ctx, space.Name)
	if err != nil {
		// Retryable errors (429 rate limit, network): return the error so
		// Asynq retries the task. Don't fall back to Wikipedia — it would
		// just hit the same rate limit.
		if errors.Is(err, ErrWikiRateLimited) {
			slog.Warn("space:auto_populate — Wikidata rate limited, will retry", "space_id", space.ID.String(), "error", err)

			return fmt.Errorf("fetching Wikidata for %q: %w", space.Name, err)
		}

		slog.Warn("space:auto_populate — Wikidata fetch failed, trying Wikipedia fallback", "space_id", space.ID.String(), "error", err)
		// Non-retryable errors: fall back to Wikipedia summary, but still
		// enrich custom fields and enqueue logo fetch from the Wikipedia
		// thumbnail if available.
		return a.populateUniversityFromWikipedia(ctx, space)
	}

	wikiJSON, err := json.Marshal(uniData)
	if err != nil {
		slog.Warn("space:auto_populate — failed to marshal wikidata", "error", err)
		return nil
	}

	if _, err := a.svc.UpdateSpaceWikiContent(ctx, space.ID, wikiJSON); err != nil {
		slog.Warn("space:auto_populate — failed to update wiki content", "space_id", space.ID.String(), "error", err)

		return nil
	}

	// Enrich custom_field_values from Wikidata data (Section 39).
	a.enrichUniversityFields(ctx, space, uniData)

	// Enqueue logo fetch if a logo URL was found and the enqueuer is wired.
	// P154 logo image first, P94 coat of arms as fallback — many
	// universities have only a crest on Wikidata. Guarded by .Valid so task
	// re-runs don't re-download.
	if !space.LogoMediaID.Valid {
		a.enqueueUniversityLogo(ctx, space, firstNonEmpty(uniData.Logo, uniData.CoatOfArms))
	}
	// The P18 image doubles as the space header so the wiki guide hero is
	// served from our own media store.
	if !space.HeaderImageID.Valid {
		a.storeSpaceImage(ctx, space, uniData.Image, "header")
	}

	slog.Info("space:auto_populate — populated university data from Wikidata", "space_id", space.ID.String(), "name", space.Name)

	return nil
}

// populateUniversityFromWikipedia is the fallback when Wikidata fails with
// a non-retryable error (e.g., no match found). It fetches a Wikipedia
// summary, stores it as wiki_content, enriches the official_website_url
// custom field from the Wikipedia content URL, and enqueues a logo fetch
// from the Wikipedia thumbnail if available.
func (a *ProcessorAdapter) populateUniversityFromWikipedia(ctx context.Context, space *gen.GetSpaceByIDRow) error {
	wikiSummary, err := fetchWikipediaSummary(ctx, space.Name)
	if err != nil {
		if errors.Is(err, ErrWikiNotFound) {
			slog.Warn("space:auto_populate — no Wikipedia article found, skipping", "space_id", space.ID.String(), "title", space.Name)

			return nil
		}

		if errors.Is(err, ErrWikiRateLimited) {
			slog.Warn("space:auto_populate — Wikipedia rate limited, will retry", "space_id", space.ID.String(), "title", space.Name)

			return fmt.Errorf("fetching Wikipedia summary for %q: %w", space.Name, err)
		}

		slog.Warn("space:auto_populate — Wikipedia fetch failed, will retry", "space_id", space.ID.String(), "title", space.Name, "error", err)

		return fmt.Errorf("fetching Wikipedia summary for %q: %w", space.Name, err)
	}

	wikiJSON, err := json.Marshal(wikiSummary)
	if err != nil {
		slog.Warn("space:auto_populate — failed to marshal wiki summary", "error", err)
		return nil
	}

	if _, err := a.svc.UpdateSpaceWikiContent(ctx, space.ID, wikiJSON); err != nil {
		slog.Warn("space:auto_populate — failed to update wiki content", "space_id", space.ID.String(), "error", err)

		return nil
	}

	// Enrich official_website_url from the Wikipedia content URL if we
	// don't already have one from the seeder. The Wikipedia page URL is
	// not the university's official website, so only use it as a fallback
	// if no website was seeded.
	if wikiSummary.ContentURL != "" {
		a.enrichWebsiteFromWikipedia(ctx, space, wikiSummary.ContentURL)
	}

	// Enqueue logo fetch from the Wikipedia thumbnail if available.
	if wikiSummary.Thumbnail != "" && !space.LogoMediaID.Valid {
		a.enqueueUniversityLogo(ctx, space, wikiSummary.Thumbnail)
	}
	// The full-resolution article image doubles as the space header.
	if !space.HeaderImageID.Valid {
		a.storeSpaceImage(ctx, space, firstNonEmpty(wikiSummary.OriginalImage, wikiSummary.Thumbnail), "header")
	}

	slog.Info("space:auto_populate — populated wiki content from Wikipedia fallback", "space_id", space.ID.String(), "type", "university", "name", space.Name)

	return nil
}

// enrichWebsiteFromWikipedia upserts the official_website_url custom field
// from the Wikipedia content URL, but only if no value exists yet (avoids
// overwriting a more accurate website from the Hipolabs seeder).
func (a *ProcessorAdapter) enrichWebsiteFromWikipedia(ctx context.Context, space *gen.GetSpaceByIDRow, contentURL string) {
	if a.customFields == nil || contentURL == "" {
		return
	}
	// Check if a website already exists — if so, don't overwrite.
	var fieldID pgtype.UUID
	_ = fieldID.Scan(universityFieldUUIDs.OfficialWebsite)

	if existing, err := a.customFields.GetFieldValue(ctx, fieldID, "space", space.ID); err == nil && existing.ValueText != nil && *existing.ValueText != "" {
		return
	}

	if _, err := a.customFields.UpsertFieldValue(ctx, gen.UpsertCustomFieldValueParams{
		FieldID:    fieldID,
		EntityType: "space",
		EntityID:   space.ID,
		ValueText:  &contentURL,
	}); err != nil {
		slog.Warn("space:auto_populate — failed to upsert official_website_url from Wikipedia", "space_id", space.ID.String(), "error", err)
	}
}

// universityFieldUUIDs are the deterministic UUIDs for university_fields custom
// field definitions, seeded in migration 000288. Using these directly avoids
// a DB lookup per enrichment.
var universityFieldUUIDs = struct {
	Initials        string
	OfficialWebsite string
	FoundedYear     string
	StudentPop      string
}{
	Initials:        "00000000-0000-0000-0000-000000000090",
	OfficialWebsite: "00000000-0000-0000-0000-000000000091",
	FoundedYear:     "00000000-0000-0000-0000-000000000096",
	StudentPop:      "00000000-0000-0000-0000-000000000100",
}

// enrichUniversityFields upserts custom_field_values for initials,
// founded_year, student_population, and official_website_url from Wikidata data.
// Best-effort — errors are logged, never returned (enrichment is non-critical; the
// wiki_content JSON is already stored).
func (a *ProcessorAdapter) enrichUniversityFields(ctx context.Context, space *gen.GetSpaceByIDRow, uni *wikidataUniversity) {
	if a.customFields == nil {
		return
	}

	// initials — P1813 short name (e.g. "UNILAG", "UoPeople", "MIT").
	if uni.ShortName != "" {
		var fieldID pgtype.UUID
		_ = fieldID.Scan(universityFieldUUIDs.Initials)

		sn := uni.ShortName
		if _, err := a.customFields.UpsertFieldValue(ctx, gen.UpsertCustomFieldValueParams{
			FieldID:    fieldID,
			EntityType: "space",
			EntityID:   space.ID,
			ValueText:  &sn,
		}); err != nil {
			slog.Warn("space:auto_populate — failed to upsert initials", "space_id", space.ID.String(), "error", err)
		}
	}

	// official_website_url
	if uni.Website != "" {
		var fieldID pgtype.UUID
		_ = fieldID.Scan(universityFieldUUIDs.OfficialWebsite)

		website := uni.Website
		if _, err := a.customFields.UpsertFieldValue(ctx, gen.UpsertCustomFieldValueParams{
			FieldID:    fieldID,
			EntityType: "space",
			EntityID:   space.ID,
			ValueText:  &website,
		}); err != nil {
			slog.Warn("space:auto_populate — failed to upsert official_website_url", "space_id", space.ID.String(), "error", err)
		}
	}

	// founded_year — Wikidata returns a datetime string; extract the year.
	if year, ok := parseYearFromWikidata(uni.Founded); ok {
		var fieldID pgtype.UUID
		_ = fieldID.Scan(universityFieldUUIDs.FoundedYear)
		yearNum := pgtype.Numeric{}
		_ = yearNum.Scan(strconv.Itoa(year))

		if _, err := a.customFields.UpsertFieldValue(ctx, gen.UpsertCustomFieldValueParams{
			FieldID:     fieldID,
			EntityType:  "space",
			EntityID:    space.ID,
			ValueNumber: yearNum,
		}); err != nil {
			slog.Warn("space:auto_populate — failed to upsert founded_year", "space_id", space.ID.String(), "error", err)
		}
	}

	// student_population
	if pop, ok := parseStudentCount(uni.Students); ok {
		var fieldID pgtype.UUID
		_ = fieldID.Scan(universityFieldUUIDs.StudentPop)
		popNum := pgtype.Numeric{}
		_ = popNum.Scan(strconv.Itoa(pop))

		if _, err := a.customFields.UpsertFieldValue(ctx, gen.UpsertCustomFieldValueParams{
			FieldID:     fieldID,
			EntityType:  "space",
			EntityID:    space.ID,
			ValueNumber: popNum,
		}); err != nil {
			slog.Warn("space:auto_populate — failed to upsert student_population", "space_id", space.ID.String(), "error", err)
		}
	}
}

// enqueueUniversityLogo enqueues university:fetch_logo if a logo URL is
// available and the enqueuer is wired. Fire-and-forget — errors are logged.
func (a *ProcessorAdapter) enqueueUniversityLogo(ctx context.Context, space *gen.GetSpaceByIDRow, logoURL string) {
	if strings.TrimSpace(logoURL) == "" {
		return
	}

	if a.uniEnqueuer == nil {
		slog.Warn("space:auto_populate — skipping logo fetch, no enqueuer wired", "space_id", space.ID.String(), "logo_url", logoURL)
		return
	}

	if !a.systemUserID.Valid {
		slog.Warn("space:auto_populate — skipping logo fetch, no system user ID", "space_id", space.ID.String())
		return
	}

	payload, err := json.Marshal(map[string]string{

		"space_id":    space.ID.String(),
		"logo_url":    logoURL,
		"uploaded_by": a.systemUserID.String(),
	})
	if err != nil {
		slog.Warn("space:auto_populate — marshal fetch_logo payload", "error", err)
		return
	}

	if err := a.uniEnqueuer.Enqueue("university:fetch_logo", payload); err != nil {
		slog.Warn("space:auto_populate — enqueue fetch_logo", "space_id", space.ID.String(), "error", err)
	} else {
		slog.Info("space:auto_populate — enqueued fetch_logo", "space_id", space.ID.String(), "logo_url", logoURL)
	}
}

// parseYearFromWikidata extracts a 4-digit year from a Wikidata datetime
// string (e.g., "1096-01-01T00:00:00Z" → 1096, or "1891" → 1891).
func parseYearFromWikidata(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	// Wikidata dates can be "YYYY-MM-DD..." or just "YYYY".
	// Take the first 4 characters if they're all digits.
	if len(s) >= 4 {
		year, err := strconv.Atoi(s[:4])
		if err == nil && year > 800 && year < 2100 {
			return year, true
		}
	}

	return 0, false
}

// parseStudentCount extracts a student count from a Wikidata string. The
// value may be a plain number or have qualifiers; we take the first integer.
func parseStudentCount(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	// Try parsing as a plain integer first.
	if n, err := strconv.Atoi(s); err == nil && n > 0 {
		return n, true
	}
	// Extract the first sequence of digits.
	var digits strings.Builder

	found := false

	for _, r := range s {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)

			found = true
		} else if found {
			break
		}
	}

	if !found {
		return 0, false
	}

	n, err := strconv.Atoi(digits.String())
	if err != nil || n <= 0 {
		return 0, false
	}

	return n, true
}

// wikidataUniversity holds structured data fetched from Wikidata. Despite
// the name it is the shared wbgetentities parse — attraction enrichment
// (§80) additionally uses InstanceOf/Coords/LocatedIn/Sitelink.
type wikidataUniversity struct {
	Name         string   `json:"name"`
	ShortName    string   `json:"short_name,omitempty"`
	Description  string   `json:"description,omitempty"`
	Country      string   `json:"country,omitempty"`
	Founded      string   `json:"founded,omitempty"`
	Students     string   `json:"students,omitempty"`
	Website      string   `json:"website,omitempty"`
	Logo         string   `json:"logo,omitempty"`
	CoatOfArms   string   `json:"coat_of_arms,omitempty"`
	Flag         string   `json:"flag,omitempty"`
	Image        string   `json:"image,omitempty"`
	WikidataID   string   `json:"wikidata_id,omitempty"`
	WikipediaURL string   `json:"wikipedia_url,omitempty"`
	// §80 attractions: P31 instance-of QIDs (kind classification), P625
	// coordinate ("lat,lon"), P131 located-in QIDs, and the enwiki article
	// title (drives the Wikipedia summary fetch so "Victoria Falls" lands
	// on the falls article, not a name-search guess).
	InstanceOf []string `json:"instance_of,omitempty"`
	Coords     string   `json:"coords,omitempty"`
	LocatedIn  []string `json:"located_in,omitempty"`
	Sitelink   string   `json:"sitelink,omitempty"`
}

// fetchWikidataUniversity uses the Wikidata Search API (wbsearchentities)
// to find a university by name, then fetches its full entity data via
// wbgetentities to get description, country, founding year, student count,
// website, and logo. This is much faster and less rate-limited than the
// SPARQL endpoint, which uses an expensive CONTAINS() full-text scan.
func fetchWikidataUniversity(ctx context.Context, name string) (*wikidataUniversity, error) {
	qid, err := searchWikidataEntity(ctx, name)
	if err != nil {
		return nil, err
	}

	if qid == "" {
		return nil, fmt.Errorf("no university found matching %q", name)
	}

	return fetchWikidataEntity(ctx, qid)
}

// wikidataSearchResult is a single wbsearchentities hit — the QID plus the
// English description used for geo disambiguation.
type wikidataSearchResult struct {
	ID          string
	Description string
}

// searchWikidataEntity uses the wbsearchentities API to find the best
// matching Wikidata QID for a university name. Returns "" if no match.
func searchWikidataEntity(ctx context.Context, name string) (string, error) {
	results, err := searchWikidataEntities(ctx, name, 1)
	if err != nil {
		return "", err
	}

	if len(results) == 0 {
		return "", nil
	}

	return results[0].ID, nil
}

// searchWikidataEntities is searchWikidataEntity's plural form — returns
// the top `limit` hits so callers can disambiguate by description.
func searchWikidataEntities(ctx context.Context, name string, limit int) ([]wikidataSearchResult, error) {
	if err := wikiRateLimiter.wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter: %w", err)
	}

	apiURL := "https://www.wikidata.org/w/api.php"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating search request: %w", err)
	}

	q := req.URL.Query()
	q.Set("action", "wbsearchentities")
	q.Set("search", name)
	q.Set("language", "en")
	q.Set("format", "json")
	q.Set("limit", strconv.Itoa(limit))
	q.Set("type", "item")
	req.URL.RawQuery = q.Encode()
	req.Header.Set("User-Agent", "JapaConnect/1.0 (space:auto_populate)")

	client := &http.Client{Timeout: 15 * time.Second}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing wbsearchentities: %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusTooManyRequests {
			return nil, fmt.Errorf("wbsearchentities returned 429: %w", ErrWikiRateLimited)
		}

		body, _ := io.ReadAll(resp.Body)

		return nil, fmt.Errorf("wbsearchentities returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Search []struct {
			ID          string `json:"id"`
			Description string `json:"description"`
		} `json:"search"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding wbsearchentities response: %w", err)
	}

	hits := make([]wikidataSearchResult, 0, len(result.Search))
	for _, r := range result.Search {
		hits = append(hits, wikidataSearchResult{ID: r.ID, Description: r.Description})
	}

	return hits, nil
}

// fetchWikidataEntity fetches full entity data for a given QID via the
// wbgetentities API, extracting university-specific properties.
func fetchWikidataEntity(ctx context.Context, qid string) (*wikidataUniversity, error) {
	if err := wikiRateLimiter.wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter: %w", err)
	}

	apiURL := "https://www.wikidata.org/w/api.php"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating wbgetentities request: %w", err)
	}

	q := req.URL.Query()
	q.Set("action", "wbgetentities")
	q.Set("ids", qid)
	q.Set("format", "json")
	q.Set("props", "labels|descriptions|claims|sitelinks")
	q.Set("languages", "en")
	q.Set("sitefilter", "enwiki")
	req.URL.RawQuery = q.Encode()
	req.Header.Set("User-Agent", "JapaConnect/1.0 (space:auto_populate)")

	client := &http.Client{Timeout: 15 * time.Second}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing wbgetentities: %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusTooManyRequests {
			return nil, fmt.Errorf("wbgetentities returned 429: %w", ErrWikiRateLimited)
		}

		body, _ := io.ReadAll(resp.Body)

		return nil, fmt.Errorf("wbgetentities returned %d: %s", resp.StatusCode, string(body))
	}

	var wdResp struct {
		Entities map[string]struct {
			Labels map[string]struct {
				Value string `json:"value"`
			} `json:"labels"`
			Descriptions map[string]struct {
				Value string `json:"value"`
			} `json:"descriptions"`
			Claims map[string][]struct {
				Mainsnak struct {
					Datavalue struct {
						Value json.RawMessage `json:"value"`
					} `json:"datavalue"`
				} `json:"mainsnak"`
			} `json:"claims"`
			Sitelinks map[string]struct {
				Title string `json:"title"`
			} `json:"sitelinks"`
		} `json:"entities"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&wdResp); err != nil {
		return nil, fmt.Errorf("decoding wbgetentities response: %w", err)
	}

	entity, ok := wdResp.Entities[qid]
	if !ok {
		return nil, fmt.Errorf("entity %s not found in response", qid)
	}

	uni := &wikidataUniversity{
		WikidataID:   qid,
		WikipediaURL: "https://en.wikipedia.org/wiki/" + qid,
	}
	if l, ok := entity.Labels["en"]; ok {
		uni.Name = l.Value
	}

	if d, ok := entity.Descriptions["en"]; ok {
		uni.Description = d.Value
	}

	// Helper: extract a string value from a RawMessage (handles both
	// plain string values like P856 and object values with a "content"
	// or "text" field).
	extractString := func(raw json.RawMessage) string {
		if len(raw) == 0 {
			return ""
		}
		// Try plain string first (e.g., P856 website, P154 logo, P18 image).
		var s string
		if json.Unmarshal(raw, &s) == nil {
			return s
		}
		// Try object with "content" field (monolingual text).
		var obj struct {
			Content string `json:"content"`
			Text    string `json:"text"`
		}

		if json.Unmarshal(raw, &obj) == nil {
			if obj.Content != "" {
				return obj.Content
			}

			return obj.Text
		}

		return ""
	}

	// P17 = country (entity-item value: { "id": "Q30", ... })
	if claims, ok := entity.Claims["P17"]; ok && len(claims) > 0 {
		var countryObj struct {
			ID string `json:"id"`
		}

		if err := json.Unmarshal(claims[0].Mainsnak.Datavalue.Value, &countryObj); err == nil { // #nosec G104 -- best-effort Wikidata parse
			uni.Country = countryObj.ID
		}
	}
	// P571 = inception (time value: { "time": "+1891-00-00T00:00:00Z", ... })
	if claims, ok := entity.Claims["P571"]; ok && len(claims) > 0 {
		var timeObj struct {
			Time string `json:"time"`
		}

		if err := json.Unmarshal(claims[0].Mainsnak.Datavalue.Value, &timeObj); err == nil { // #nosec G104 -- best-effort Wikidata parse
			uni.Founded = timeObj.Time
		}
	}
	// P2196 = students count (quantity value: { "amount": "+12345", ... })
	if claims, ok := entity.Claims["P2196"]; ok && len(claims) > 0 {
		var qtyObj struct {
			Amount string `json:"amount"`
		}

		if err := json.Unmarshal(claims[0].Mainsnak.Datavalue.Value, &qtyObj); err == nil { // #nosec G104 -- best-effort Wikidata parse
			uni.Students = qtyObj.Amount
		}
	}
	// P856 = official website (string value)
	if claims, ok := entity.Claims["P856"]; ok && len(claims) > 0 {
		uni.Website = extractString(claims[0].Mainsnak.Datavalue.Value)
	}
	// P154 = logo image (string value — Wikimedia Commons filename)
	if claims, ok := entity.Claims["P154"]; ok && len(claims) > 0 {
		uni.Logo = commonsFileURL(extractString(claims[0].Mainsnak.Datavalue.Value), 512)
	}
	// P94 = coat of arms / crest (string value — Wikimedia Commons
	// filename); logo fallback for entities with no P154, and the emblem
	// source for state/city spaces.
	if claims, ok := entity.Claims["P94"]; ok && len(claims) > 0 {
		uni.CoatOfArms = commonsFileURL(extractString(claims[0].Mainsnak.Datavalue.Value), 512)
	}
	// P41 = flag image (string value — Wikimedia Commons filename); the
	// emblem source for state/city spaces.
	if claims, ok := entity.Claims["P41"]; ok && len(claims) > 0 {
		uni.Flag = commonsFileURL(extractString(claims[0].Mainsnak.Datavalue.Value), 512)
	}
	// P18 = image (string value — Wikimedia Commons filename); the space
	// header image source.
	if claims, ok := entity.Claims["P18"]; ok && len(claims) > 0 {
		uni.Image = commonsFileURL(extractString(claims[0].Mainsnak.Datavalue.Value), 1600)
	}
	// P1813 = short name (monolingual text; e.g. "UNILAG", "UoPeople", "MIT").
	if claims, ok := entity.Claims["P1813"]; ok && len(claims) > 0 {
		uni.ShortName = extractString(claims[0].Mainsnak.Datavalue.Value)
	}
	// §80 attractions: P31 = instance of (entity-item values — the class QIDs
	// used for kind classification), P625 = coordinate location
	// (globecoordinate value), P131 = located in the administrative entity.
	extractQID := func(raw json.RawMessage) string {
		var obj struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(raw, &obj); err == nil { // #nosec G104 -- best-effort Wikidata parse
			return obj.ID
		}
		return ""
	}
	for _, c := range entity.Claims["P31"] {
		if id := extractQID(c.Mainsnak.Datavalue.Value); id != "" {
			uni.InstanceOf = append(uni.InstanceOf, id)
		}
	}
	for _, c := range entity.Claims["P131"] {
		if id := extractQID(c.Mainsnak.Datavalue.Value); id != "" {
			uni.LocatedIn = append(uni.LocatedIn, id)
		}
	}
	if claims, ok := entity.Claims["P625"]; ok && len(claims) > 0 {
		var coord struct {
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
		}
		if err := json.Unmarshal(claims[0].Mainsnak.Datavalue.Value, &coord); err == nil { // #nosec G104 -- best-effort Wikidata parse
			uni.Coords = strconv.FormatFloat(coord.Latitude, 'f', -1, 64) + "," +
				strconv.FormatFloat(coord.Longitude, 'f', -1, 64)
		}
	}
	if sl, ok := entity.Sitelinks["enwiki"]; ok {
		uni.Sitelink = sl.Title
	}

	return uni, nil
}

// commonsFileURL converts a Wikimedia Commons filename like "Foo.png" to a
// direct URL via the Special:FilePath redirect endpoint. The width parameter
// asks MediaWiki to render a thumbnail — essential for SVG sources (flags,
// coats of arms, most logos), which would otherwise store as undisplayable
// .bin blobs: SVG is active content and intentionally excluded from
// media.safeMediaExts.
func commonsFileURL(filename string, width int) string {
	if filename == "" {
		return ""
	}

	u := "https://commons.wikimedia.org/wiki/Special:FilePath/" + url.PathEscape(filename)
	if width > 0 {
		u += "?width=" + strconv.Itoa(width)
	}

	return u
}

// firstNonEmpty returns the first non-empty string from its arguments.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}

	return ""
}

// wikipediaSummary is the subset of the Wikipedia REST API response we store.
type wikipediaSummary struct {
	Title       string `json:"title"`
	Extract     string `json:"extract"`
	Description string `json:"description"`
	Thumbnail   string `json:"thumbnail,omitempty"`
	// OriginalImage is the full-resolution article image URL — stored as the
	// space header (the thumbnail is too small for a cover).
	OriginalImage string `json:"original_image,omitempty"`
	ContentURL    string `json:"content_url,omitempty"`
}

// fetchWikipediaSummary calls the Wikipedia REST API to get a page summary.
func fetchWikipediaSummary(ctx context.Context, title string) (*wikipediaSummary, error) {
	if err := wikiRateLimiter.wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter: %w", err)
	}

	encodedTitle := url.PathEscape(title)
	apiURL := "https://en.wikipedia.org/api/rest_v1/page/summary/" + encodedTitle

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("User-Agent", "JapaConnect/1.0 (space:auto_populate)")

	client := &http.Client{Timeout: 10 * time.Second}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching Wikipedia summary: %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusTooManyRequests {
			return nil, fmt.Errorf("Wikipedia API returned 429: %w", ErrWikiRateLimited)
		}

		if resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("Wikipedia API returned 404: %w", ErrWikiNotFound)
		}

		return nil, fmt.Errorf("Wikipedia API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	var result struct {
		Title       string `json:"title"`
		Extract     string `json:"extract"`
		Description string `json:"description"`
		Thumbnail   struct {
			Source string `json:"source"`
		} `json:"thumbnail"`
		OriginalImage struct {
			Source string `json:"source"`
		} `json:"originalimage"`
		ContentURLs struct {
			Desktop struct {
				Page string `json:"page"`
			} `json:"desktop"`
		} `json:"content_urls"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	summary := &wikipediaSummary{
		Title:       result.Title,
		Extract:     result.Extract,
		Description: result.Description,
	}

	if result.Thumbnail.Source != "" {
		summary.Thumbnail = result.Thumbnail.Source
	}

	if result.OriginalImage.Source != "" {
		summary.OriginalImage = result.OriginalImage.Source
	}

	if result.ContentURLs.Desktop.Page != "" {
		summary.ContentURL = result.ContentURLs.Desktop.Page
	}

	return summary, nil
}

// Ensure ProcessorAdapter satisfies the interface at compile time.
var _ interface {
	AutoPopulateSpace(ctx context.Context, spaceID string) error
} = (*ProcessorAdapter)(nil)
