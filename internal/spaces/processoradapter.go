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

// ProcessorAdapter implements worker.SpaceAutoPopulateProcessor by delegating
// to the spaces service's auto-population logic.
type ProcessorAdapter struct {
	svc          *Service
	customFields CustomFieldsEnricher   // nil = skip custom field enrichment
	uniEnqueuer  UniversityLogoEnqueuer // nil = skip logo fetch enqueue
	systemUserID pgtype.UUID            // for media attribution in university:fetch_logo
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

// AutoPopulateSpace seeds a newly created space with default content based
// on its space type. For geo spaces, this includes fetching Wikipedia data
// and flag images. For other space types, it creates default rules and tags.
func (a *ProcessorAdapter) AutoPopulateSpace(ctx context.Context, spaceID, tenantID string) error {
	slog.Info("space:auto_populate", "space_id", spaceID)

	if a.svc == nil {
		slog.Warn("space:auto_populate — service not initialized, skipping")
		return nil
	}

	var sid, tid pgtype.UUID
	if err := sid.Scan(spaceID); err != nil {
		slog.Warn("space:auto_populate — invalid space_id", "error", err)
		return nil
	}

	if err := tid.Scan(tenantID); err != nil {
		slog.Warn("space:auto_populate — invalid tenant_id", "error", err)
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

	switch spaceType {
	case "country", "state", "city":
		return a.populateGeoSpace(ctx, &space, spaceType)
	case "university":
		return a.populateUniversitySpace(ctx, &space, tid)
	default:
		slog.Info("space:auto_populate — custom space, no auto-population needed", "space_id", spaceID, "type", spaceType)
	}

	return nil
}

// populateGeoSpace fetches a Wikipedia summary for the space and stores it
// as WikiContent. For countries, the geo_code (ISO 3166-1 alpha-2) is used
// to construct the Wikipedia article title.
func (a *ProcessorAdapter) populateGeoSpace(ctx context.Context, space *gen.GetSpaceByIDRow, spaceType string) error {
	// Build the Wikipedia article title from the space name.
	// For countries, use the country name directly (e.g. "Nigeria").
	// For states/cities, use "{name}, {parent}" pattern if available.
	title := space.Name

	wikiSummary, err := fetchWikipediaSummary(ctx, title)
	if err != nil {
		// Non-retryable errors (404 — no Wikipedia article exists): don't
		// fail the task, there's nothing to populate.
		if errors.Is(err, ErrWikiNotFound) {
			slog.Warn("space:auto_populate — no Wikipedia article found, skipping", "space_id", space.ID.String(), "title", title)

			return nil
		}
		// Retryable errors (429 rate limit, network errors): return the
		// error so Asynq retries the task with backoff.
		slog.Warn("space:auto_populate — Wikipedia fetch failed, will retry", "space_id", space.ID.String(), "title", title, "error", err)

		return fmt.Errorf("fetching Wikipedia summary for %q: %w", title, err)
	}

	// Update the space's WikiContent with the fetched summary.
	wikiJSON, err := json.Marshal(wikiSummary)
	if err != nil {
		slog.Warn("space:auto_populate — failed to marshal wiki summary", "error", err)
		return nil
	}

	if _, err := a.svc.UpdateSpaceWikiContent(ctx, space.ID, wikiJSON); err != nil {
		slog.Warn("space:auto_populate — failed to update wiki content", "space_id", space.ID.String(), "error", err)

		return nil
	}

	slog.Info("space:auto_populate — populated wiki content", "space_id", space.ID.String(), "type", spaceType, "name", space.Name)

	return nil
}

// populateUniversitySpace fetches university data from Wikidata SPARQL and
// stores it as WikiContent. Retrieves: description, country, founding year,
// student count, official website, logo, and coordinates. When the custom
// fields enricher is wired, it also upserts custom_field_values for
// founded_year, student_population, and official_website_url. When the
// university logo enqueuer is wired and a logo URL is found, it enqueues
// university:fetch_logo to download and store the logo.
func (a *ProcessorAdapter) populateUniversitySpace(ctx context.Context, space *gen.GetSpaceByIDRow, tenantID pgtype.UUID) error {
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
		return a.populateUniversityFromWikipedia(ctx, space, tenantID)
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
	a.enqueueUniversityLogo(ctx, tenantID, space, uniData.Logo)

	slog.Info("space:auto_populate — populated university data from Wikidata", "space_id", space.ID.String(), "name", space.Name)

	return nil
}

// populateUniversityFromWikipedia is the fallback when Wikidata fails with
// a non-retryable error (e.g., no match found). It fetches a Wikipedia
// summary, stores it as wiki_content, enriches the official_website_url
// custom field from the Wikipedia content URL, and enqueues a logo fetch
// from the Wikipedia thumbnail if available.
func (a *ProcessorAdapter) populateUniversityFromWikipedia(ctx context.Context, space *gen.GetSpaceByIDRow, tenantID pgtype.UUID) error {
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
	if wikiSummary.Thumbnail != "" {
		a.enqueueUniversityLogo(ctx, tenantID, space, wikiSummary.Thumbnail)
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
func (a *ProcessorAdapter) enqueueUniversityLogo(ctx context.Context, tenantID pgtype.UUID, space *gen.GetSpaceByIDRow, logoURL string) {
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

// wikidataUniversity holds structured data fetched from Wikidata SPARQL.
type wikidataUniversity struct {
	Name         string `json:"name"`
	ShortName    string `json:"short_name,omitempty"`
	Description  string `json:"description,omitempty"`
	Country      string `json:"country,omitempty"`
	Founded      string `json:"founded,omitempty"`
	Students     string `json:"students,omitempty"`
	Website      string `json:"website,omitempty"`
	Logo         string `json:"logo,omitempty"`
	Image        string `json:"image,omitempty"`
	WikidataID   string `json:"wikidata_id,omitempty"`
	WikipediaURL string `json:"wikipedia_url,omitempty"`
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

// searchWikidataEntity uses the wbsearchentities API to find the best
// matching Wikidata QID for a university name. Returns "" if no match.
func searchWikidataEntity(ctx context.Context, name string) (string, error) {
	if err := wikiRateLimiter.wait(ctx); err != nil {
		return "", fmt.Errorf("rate limiter: %w", err)
	}

	apiURL := "https://www.wikidata.org/w/api.php"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("creating search request: %w", err)
	}

	q := req.URL.Query()
	q.Set("action", "wbsearchentities")
	q.Set("search", name)
	q.Set("language", "en")
	q.Set("format", "json")
	q.Set("limit", "1")
	q.Set("type", "item")
	req.URL.RawQuery = q.Encode()
	req.Header.Set("User-Agent", "JapaConnect/1.0 (space:auto_populate)")

	client := &http.Client{Timeout: 15 * time.Second}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("executing wbsearchentities: %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusTooManyRequests {
			return "", fmt.Errorf("wbsearchentities returned 429: %w", ErrWikiRateLimited)
		}

		body, _ := io.ReadAll(resp.Body)

		return "", fmt.Errorf("wbsearchentities returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Search []struct {
			ID      string `json:"id"`
			Concept string `json:"concepturi"`
		} `json:"search"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding wbsearchentities response: %w", err)
	}

	if len(result.Search) == 0 {
		return "", nil
	}

	return result.Search[0].ID, nil
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
	q.Set("props", "labels|descriptions|claims")
	q.Set("languages", "en")
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
		uni.Logo = commonsFileURL(extractString(claims[0].Mainsnak.Datavalue.Value))
	}
	// P18 = image (string value — Wikimedia Commons filename)
	if claims, ok := entity.Claims["P18"]; ok && len(claims) > 0 {
		uni.Image = commonsFileURL(extractString(claims[0].Mainsnak.Datavalue.Value))
	}
	// P1813 = short name (monolingual text; e.g. "UNILAG", "UoPeople", "MIT").
	if claims, ok := entity.Claims["P1813"]; ok && len(claims) > 0 {
		uni.ShortName = extractString(claims[0].Mainsnak.Datavalue.Value)
	}

	return uni, nil
}

// commonsFileURL converts a Wikimedia Commons filename like "Foo.png" to
// a direct URL via the Special:FilePath redirect endpoint.
func commonsFileURL(filename string) string {
	if filename == "" {
		return ""
	}

	return "http://commons.wikimedia.org/wiki/Special:FilePath/" + url.PathEscape(filename)
}

// wikipediaSummary is the subset of the Wikipedia REST API response we store.
type wikipediaSummary struct {
	Title       string `json:"title"`
	Extract     string `json:"extract"`
	Description string `json:"description"`
	Thumbnail   string `json:"thumbnail,omitempty"`
	ContentURL  string `json:"content_url,omitempty"`
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

	if result.ContentURLs.Desktop.Page != "" {
		summary.ContentURL = result.ContentURLs.Desktop.Page
	}

	return summary, nil
}

// Ensure ProcessorAdapter satisfies the interface at compile time.
var _ interface {
	AutoPopulateSpace(ctx context.Context, spaceID, tenantID string) error
} = (*ProcessorAdapter)(nil)
