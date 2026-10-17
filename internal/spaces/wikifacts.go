package spaces

// wikifacts.go — wiki:populate_facts enrichment: fetch a space's Wikidata
// entity and write general-info facts (population, languages, currency,
// postal codes, calling code, capital, founded, head of government, time
// zone; universities: founded year, student population, website, initials)
// into the space's wiki page custom fields (geo_fields / university_fields
// groups).
//
// Ref: japaconnect-engineering/sections/11-job-queue-asynq.md (wiki:populate_facts)

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/worker"
)

// geoFactsFieldUUIDs — deterministic UUIDs of geo_fields entries (seeded in
// 000735_baseline_seed + 000872_seed_geo_facts_fields).
var geoFactsFieldUUIDs = struct {
	Population, Area, Capital, Language, Currency, PostalCodes, CallingCode,
	OfficialWebsite, Founded, HeadOfGovernment, TimeZone string
}{
	Population:       "da195d1e-50d1-449c-b23d-4f797c470878",
	Area:             "37bc4b1f-a386-4603-9cf5-afdb6eb5819b",
	Capital:          "32758839-ea78-4aed-8b44-233381bfbd63",
	Language:         "d3be08b8-42a3-4349-9d9c-f6241de66deb",
	Currency:         "4dfd79c2-d087-4e5c-80b1-bb10b5f90001",
	PostalCodes:      "4dfd79c2-d087-4e5c-80b1-bb10b5f90002",
	CallingCode:      "4dfd79c2-d087-4e5c-80b1-bb10b5f90003",
	OfficialWebsite:  "4dfd79c2-d087-4e5c-80b1-bb10b5f90004",
	Founded:          "4dfd79c2-d087-4e5c-80b1-bb10b5f90005",
	HeadOfGovernment: "4dfd79c2-d087-4e5c-80b1-bb10b5f90006",
	TimeZone:         "4dfd79c2-d087-4e5c-80b1-bb10b5f90007",
}

// universityFactsFieldUUIDs — university_fields entries (000735 baseline).
var universityFactsFieldUUIDs = struct {
	Initials, OfficialWebsite, ContactEmail, WorldRanking, CountryRanking,
	FoundedYear, UniType, ProgramsOffered, StudentPopulation, Colleges string
}{
	Initials:          "00000000-0000-0000-0000-000000000090",
	OfficialWebsite:   "00000000-0000-0000-0000-000000000091",
	ContactEmail:      "00000000-0000-0000-0000-000000000092",
	WorldRanking:      "00000000-0000-0000-0000-000000000094",
	CountryRanking:    "00000000-0000-0000-0000-000000000095",
	FoundedYear:       "00000000-0000-0000-0000-000000000096",
	UniType:           "00000000-0000-0000-0000-000000000097",
	ProgramsOffered:   "00000000-0000-0000-0000-000000000098",
	StudentPopulation: "00000000-0000-0000-0000-000000000100",
	Colleges:          "00000000-0000-0000-0000-000000000105",
}

// universitySeedEntry mirrors db/seed/universities.json — the platform's
// curated university dataset (name, initials, world ranking, type, founded
// year, website, email). Used to fill facts Wikidata can't provide
// (world_ranking, type, contact email).
type universitySeedEntry struct {
	Name         string `json:"name"`
	Initials     string `json:"initials"`
	WorldRanking int    `json:"world_ranking"`
	Type         string `json:"type"`
	FoundedYear  int    `json:"founded_year"`
	Website      string `json:"website"`
	Email        string `json:"email"`
}

var (
	uniSeedOnce  sync.Once
	uniSeedIndex map[string]universitySeedEntry // lowercase name/initials → row
)

// universitySeedData lazily loads db/seed/universities.json (canonical
// curated dataset) and indexes it by lowercased name and initials. Missing
// file → empty index, lookups just yield no ranking — graceful.
func universitySeedData() map[string]universitySeedEntry {
	uniSeedOnce.Do(func() {
		uniSeedIndex = map[string]universitySeedEntry{}
		for _, path := range []string{"db/seed/universities.json", "../db/seed/universities.json", "../../db/seed/universities.json"} {
			raw, err := os.ReadFile(path) // #nosec G304 -- path is a fixed internal seed-data candidate
			if err != nil {
				continue
			}
			var rows []universitySeedEntry
			if err := json.Unmarshal(raw, &rows); err != nil {
				break
			}
			for _, r := range rows {
				uniSeedIndex[strings.ToLower(r.Name)] = r
				if r.Initials != "" {
					uniSeedIndex[strings.ToLower(r.Initials)] = r
				}
			}
			break
		}
	})
	return uniSeedIndex
}

// universitySeedMatch finds a curated-dataset row for a space by exact
// name or initials match.
func universitySeedMatch(spaceName string) (universitySeedEntry, bool) {
	e, ok := universitySeedData()[strings.ToLower(spaceName)]
	return e, ok
}

// factFieldTypes maps each target field UUID to its custom_fields.field_type
// so the write lands in the right value column (number → value_number,
// select → value_json {"value":k}, multiselect → value_json [..],
// text/url/email → value_text). Writing the wrong column is a silent no-op.
var factFieldTypes = func() map[string]string {
	m := map[string]string{}
	g := geoFactsFieldUUIDs
	for _, id := range []string{g.Population, g.Area, g.Capital, g.Language,
		g.Currency, g.PostalCodes, g.CallingCode, g.OfficialWebsite, g.Founded,
		g.HeadOfGovernment, g.TimeZone} {
		m[id] = "text"
	}
	u := universityFactsFieldUUIDs
	m[u.Initials] = "text"
	m[u.OfficialWebsite] = "url"
	m[u.ContactEmail] = "email"
	m[u.WorldRanking] = "number"
	m[u.CountryRanking] = "number"
	m[u.FoundedYear] = "number"
	m[u.UniType] = "select" // options: public|private — other values skipped
	m[u.ProgramsOffered] = "multiselect"
	m[u.StudentPopulation] = "number"
	m[u.Colleges] = "text"
	return m
}()

// wdClaims is the minimal wbgetentities shape the facts fetcher needs.
type wdClaims map[string][]struct {
	Mainsnak struct {
		Datavalue struct {
			Value json.RawMessage `json:"value"`
		} `json:"datavalue"`
	} `json:"mainsnak"`
}

// factValuePresent reports whether an existing custom_field_values row holds
// a non-empty value in the column that matches the field's type — the
// fill-empty check must read the same column the write targets.
func factValuePresent(v gen.CustomFieldValue, fieldType string) bool {
	switch fieldType {
	case "number":
		return v.ValueNumber.Valid
	case "select", "multiselect":
		return len(v.ValueJson) > 0
	case "boolean":
		return v.ValueBool != nil
	default: // text, url, email
		return v.ValueText != nil && *v.ValueText != ""
	}
}

// PopulateWikiFacts implements worker.WikiFactsProcessor — fetches the
// space's Wikidata entity and upserts general-info claims into the wiki
// page custom fields for its field group. dry_run reports the planned
// writes without persisting; overwrite replaces existing values (default:
// fill only empty fields so operator/attraction-enriched values win).
func (a *ProcessorAdapter) PopulateWikiFacts(ctx context.Context, spaceID string, dryRun, overwrite bool) error {
	if a.svc == nil || a.customFields == nil {
		slog.Warn("wiki:populate_facts — service or custom-fields enricher not wired, skipping", "space_id", spaceID)
		return nil
	}

	var sid pgtype.UUID
	if err := sid.Scan(spaceID); err != nil {
		return fmt.Errorf("invalid space_id %q: %w", spaceID, err)
	}

	space, err := a.svc.GetSpace(ctx, sid)
	if err != nil {
		return fmt.Errorf("fetch space: %w", err)
	}

	spaceType := ""
	if st, err := a.svc.GetSpaceType(ctx, space.SpaceTypeID); err == nil {
		spaceType = st.Slug
	}

	// Resolve the entity QID: wiki_content stamp first, then wbsearchentities
	// with parent-aware disambiguation ("Georgia, US state" style) — the
	// image-gated geoWikidataEntity is intentionally not used since facts
	// don't require an entity with images, and its silent nil on search
	// errors would hide retryable 429s.
	qid := wikiContentWikidataID(space.WikiContent)
	if qid == "" {
		parentName := ""
		if space.ParentID.Valid {
			if parent, err := a.svc.GetSpace(ctx, space.ParentID); err == nil {
				parentName = strings.ToLower(parent.Name)
			}
		}
		results, err := searchWikidataEntities(ctx, space.Name, 5)
		if err != nil {
			if errors.Is(err, ErrWikiRateLimited) {
				return fmt.Errorf("%w: %w", err, worker.ErrUpstreamRateLimited)
			}
			return fmt.Errorf("wikidata search for %q: %w", space.Name, err)
		}
		for _, r := range results {
			if parentName == "" || strings.Contains(strings.ToLower(r.Description), parentName) {
				qid = r.ID
				break
			}
		}
		if qid == "" && len(results) > 0 {
			qid = results[0].ID // best-effort top hit
		}
	}
	if qid == "" {
		slog.Warn("wiki:populate_facts — no Wikidata entity for space", "space_id", spaceID, "type", spaceType)
		return nil
	}

	claims, err := fetchWikidataFacts(ctx, qid)
	if err != nil {
		if errors.Is(err, ErrWikiRateLimited) {
			return fmt.Errorf("%w: %w", err, worker.ErrUpstreamRateLimited)
		}
		return err
	}

	// Collect entity-valued props → one batched label-resolution call.
	entityProps := map[string][]string{}
	for _, prop := range []string{"P36", "P37", "P38", "P6", "P421", "P527"} {
		entityProps[prop] = entityIDs(claims[prop])
	}
	var labelQIDs []string
	for _, ids := range entityProps {
		labelQIDs = append(labelQIDs, ids...)
	}
	labels, _ := fetchWikidataLabels(ctx, labelQIDs)

	// prop → field UUID map per space type.
	fieldMap := map[string]string{}
	switch spaceType {
	case "country", "state", "city":
		f := geoFactsFieldUUIDs
		fieldMap = map[string]string{
			"P1082": f.Population,       // population (quantity)
			"P2046": f.Area,             // area km² (quantity)
			"P36":   f.Capital,          // capital (entity → label)
			"P37":   f.Language,         // official language (entities → labels)
			"P38":   f.Currency,         // currency (entity → label)
			"P281":  f.PostalCodes,      // postal code (string, may repeat — first)
			"P474":  f.CallingCode,      // country calling code (string)
			"P856":  f.OfficialWebsite,  // official website (string)
			"P571":  f.Founded,          // inception (time → year)
			"P6":    f.HeadOfGovernment, // head of government (entity → label)
			"P421":  f.TimeZone,         // located in time zone (entity → label)
		}
	case "university":
		f := universityFactsFieldUUIDs
		fieldMap = map[string]string{
			"P571":  f.FoundedYear,       // inception (time → year)
			"P2196": f.StudentPopulation, // students count (quantity)
			"P856":  f.OfficialWebsite,   // official website (string)
			"P1813": f.Initials,          // short name (monolingual text)
			"P527":  f.Colleges,          // has part → constituent colleges/faculties (entities → labels)
		}
	default:
		slog.Info("wiki:populate_facts — no fact mapping for space type", "space_id", spaceID, "type", spaceType)
		return nil
	}

	labelOf := func(prop string) []string {
		var out []string
		for _, id := range entityProps[prop] {
			if l := labels[id]; l != "" {
				out = append(out, l)
			}
		}
		return out
	}

	// Candidates: (source tag, field UUID, value, multiselect values) — the
	// seed dataset and Wikidata props funnel through the same
	// fill-empty/overwrite/dry-run path. `values` carries the list form for
	// multiselect fields (stored as a JSON array, not a CSV string).
	type factWrite struct {
		tag, fieldUUID, value string
		values                []string
	}
	var candidates []factWrite

	for prop, fieldUUID := range fieldMap {
		var value string
		switch prop {
		case "P36", "P37", "P38", "P6", "P421", "P527": // entity-valued
			value = strings.Join(labelOf(prop), ", ")
		case "P1082", "P2046", "P2196": // quantity
			value = quantityString(claims[prop])
		case "P571": // time → year
			value = timeYear(claims[prop])
		default: // plain string (P281, P474, P856, P1813)
			value = firstString(claims[prop])
		}
		if value != "" {
			candidates = append(candidates, factWrite{tag: prop, fieldUUID: fieldUUID, value: value})
		}
	}

	// Universities: real course offerings from university_course_offerings
	// JOIN university_course_catalog → programs_offered (published courses,
	// catalog titles joined comma-separated). This is the actual curriculum
	// source — Wikidata has no per-course data.
	if spaceType == "university" && a.svc != nil && a.svc.read != nil {
		f := universityFactsFieldUUIDs
		if rows, err := a.svc.read.ListCourseOfferingsByUniversity(ctx, gen.ListCourseOfferingsByUniversityParams{
			UniversityID: space.ID,
			LimitVal:     100,
		}); err == nil && len(rows) > 0 {
			seen := map[string]bool{}
			var titles []string
			for _, r := range rows {
				if !seen[r.CourseTitle] {
					seen[r.CourseTitle] = true
					titles = append(titles, r.CourseTitle)
				}
			}
			candidates = append(candidates, factWrite{tag: "courses", fieldUUID: f.ProgramsOffered,
				value: strings.Join(titles, ", "), values: titles})
		}
	}

	// Universities: the curated seed dataset fills facts Wikidata cannot
	// provide — world_ranking, institution type, contact email (plus
	// fallback website/founded/initials).
	if spaceType == "university" {
		if row, ok := universitySeedMatch(space.Name); ok {
			f := universityFactsFieldUUIDs
			if row.WorldRanking > 0 {
				candidates = append(candidates, factWrite{tag: "seed:world_ranking", fieldUUID: f.WorldRanking, value: strconv.Itoa(row.WorldRanking)})
			}
			// 'type' is a select field with options public|private — the
			// dataset's axis ("research" etc.) doesn't map, so only write
			// values that are declared options.
			if row.Type == "public" || row.Type == "private" {
				candidates = append(candidates, factWrite{tag: "seed:type", fieldUUID: f.UniType, value: row.Type})
			}
			if row.Email != "" {
				candidates = append(candidates, factWrite{tag: "seed:contact_email", fieldUUID: f.ContactEmail, value: row.Email})
			}
			if row.Website != "" {
				candidates = append(candidates, factWrite{tag: "seed:website", fieldUUID: f.OfficialWebsite, value: row.Website})
			}
			if row.FoundedYear > 0 {
				candidates = append(candidates, factWrite{tag: "seed:founded_year", fieldUUID: f.FoundedYear, value: strconv.Itoa(row.FoundedYear)})
			}
			if row.Initials != "" {
				candidates = append(candidates, factWrite{tag: "seed:initials", fieldUUID: f.Initials, value: row.Initials})
			}
		}
	}

	applied := map[string]string{}
	for _, c := range candidates {
		var fieldID pgtype.UUID
		if err := fieldID.Scan(c.fieldUUID); err != nil {
			continue
		}

		ft := factFieldTypes[c.fieldUUID]

		if !overwrite {
			if existing, err := a.customFields.GetFieldValue(ctx, fieldID, "space", space.ID); err == nil && factValuePresent(existing, ft) {
				continue // don't clobber curated/operator values
			}
		}

		applied[c.tag] = c.value
		if dryRun {
			continue
		}

		params := gen.UpsertCustomFieldValueParams{
			FieldID:    fieldID,
			EntityType: "space",
			EntityID:   space.ID,
		}
		switch ft {
		case "number":
			num := pgtype.Numeric{}
			if err := num.Scan(strings.ReplaceAll(c.value, ",", "")); err != nil {
				continue
			}
			params.ValueNumber = num
		case "select":
			vj, _ := json.Marshal(map[string]string{"value": c.value})
			params.ValueJson = vj
		case "multiselect":
			vj, _ := json.Marshal(c.values)
			params.ValueJson = vj
		default: // text, url, email
			params.ValueText = &c.value
		}

		if _, err := a.customFields.UpsertFieldValue(ctx, params); err != nil {
			slog.Warn("wiki:populate_facts — field upsert failed", "space_id", spaceID, "field", c.tag, "error", err)
		}
	}

	// Stamp the QID into wiki_content so re-runs dedup (skip on dry_run).
	// Guard: if wiki_content holds a valid non-object JSON (array/scalar),
	// merging into an empty map would clobber it — skip the stamp instead.
	if !dryRun {
		m := map[string]any{}
		contentOK := len(space.WikiContent) == 0 || json.Unmarshal(space.WikiContent, &m) == nil
		if contentOK && m["wikidata_id"] != qid {
			m["wikidata_id"] = qid
			if raw, err := json.Marshal(m); err == nil {
				_, _ = a.svc.UpdateSpaceWikiContent(ctx, space.ID, raw)
			}
		}
	}

	slog.Info("wiki:populate_facts", "space_id", spaceID, "type", spaceType, "qid", qid,
		"fields", len(applied), "dry_run", dryRun)
	if dryRun {
		for prop, v := range applied {
			slog.Info("wiki:populate_facts [dry-run]", "space_id", spaceID, "prop", prop, "value", v)
		}
	}

	return nil
}

// fetchWikidataFacts fetches an entity's claims map via wbgetentities.
func fetchWikidataFacts(ctx context.Context, qid string) (wdClaims, error) {
	if err := wikiRateLimiter.wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.wikidata.org/w/api.php", nil)
	if err != nil {
		return nil, err
	}
	q := req.URL.Query()
	q.Set("action", "wbgetentities")
	q.Set("ids", qid)
	q.Set("format", "json")
	q.Set("props", "claims")
	req.URL.RawQuery = q.Encode()
	req.Header.Set("User-Agent", "JapaConnect/1.0 (wiki:populate_facts)")

	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, ErrWikiRateLimited
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("wbgetentities %d: %s", resp.StatusCode, body)
	}

	var wdResp struct {
		Entities map[string]struct {
			Claims wdClaims `json:"claims"`
		} `json:"entities"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wdResp); err != nil {
		return nil, err
	}
	ent, ok := wdResp.Entities[qid]
	if !ok {
		return nil, fmt.Errorf("entity %s not found", qid)
	}
	return ent.Claims, nil
}

// fetchWikidataLabels resolves entity QIDs → en labels in one batched
// wbgetentities call (API max 50 ids per request).
func fetchWikidataLabels(ctx context.Context, qids []string) (map[string]string, error) {
	out := map[string]string{}
	if len(qids) == 0 {
		return out, nil
	}
	if len(qids) > 50 {
		qids = qids[:50]
	}
	if err := wikiRateLimiter.wait(ctx); err != nil {
		return out, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.wikidata.org/w/api.php", nil)
	if err != nil {
		return out, err
	}
	q := req.URL.Query()
	q.Set("action", "wbgetentities")
	q.Set("ids", strings.Join(qids, "|"))
	q.Set("format", "json")
	q.Set("props", "labels")
	q.Set("languages", "en")
	req.URL.RawQuery = q.Encode()
	req.Header.Set("User-Agent", "JapaConnect/1.0 (wiki:populate_facts)")

	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return out, fmt.Errorf("labels wbgetentities %d", resp.StatusCode)
	}

	var wdResp struct {
		Entities map[string]struct {
			Labels map[string]struct {
				Value string `json:"value"`
			} `json:"labels"`
		} `json:"entities"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wdResp); err != nil {
		return out, err
	}
	for qid, e := range wdResp.Entities {
		if l, ok := e.Labels["en"]; ok {
			out[qid] = l.Value
		}
	}
	return out, nil
}

// entityIDs extracts item QIDs from an entity-item claim list.
func entityIDs(claims []struct {
	Mainsnak struct {
		Datavalue struct {
			Value json.RawMessage `json:"value"`
		} `json:"datavalue"`
	} `json:"mainsnak"`
}) []string {
	var out []string
	for _, c := range claims {
		var v struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(c.Mainsnak.Datavalue.Value, &v); err == nil && strings.HasPrefix(v.ID, "Q") {
			out = append(out, v.ID)
		}
	}
	return out
}

// quantityString formats the first quantity claim ("+12345" → "12345").
// Multiple claims (multi-census populations etc.) would join into an
// unparseable "a, b" — number-typed fields must take a single value, and
// Wikidata returns preferred-rank claims first.
func quantityString(claims []struct {
	Mainsnak struct {
		Datavalue struct {
			Value json.RawMessage `json:"value"`
		} `json:"datavalue"`
	} `json:"mainsnak"`
}) string {
	for _, c := range claims {
		var v struct {
			Amount string `json:"amount"`
		}
		if err := json.Unmarshal(c.Mainsnak.Datavalue.Value, &v); err == nil && v.Amount != "" {
			return strings.TrimPrefix(v.Amount, "+")
		}
	}
	return ""
}

// timeYear extracts the year from a time claim ("+1891-00-00T00:00:00Z" → "1891").
func timeYear(claims []struct {
	Mainsnak struct {
		Datavalue struct {
			Value json.RawMessage `json:"value"`
		} `json:"datavalue"`
	} `json:"mainsnak"`
}) string {
	for _, c := range claims {
		var v struct {
			Time string `json:"time"`
		}
		if err := json.Unmarshal(c.Mainsnak.Datavalue.Value, &v); err == nil && len(v.Time) >= 5 {
			return strings.TrimPrefix(v.Time[:5], "+")
		}
	}
	return ""
}

// firstString returns the first plain-string claim value.
func firstString(claims []struct {
	Mainsnak struct {
		Datavalue struct {
			Value json.RawMessage `json:"value"`
		} `json:"datavalue"`
	} `json:"mainsnak"`
}) string {
	for _, c := range claims {
		var s string
		if err := json.Unmarshal(c.Mainsnak.Datavalue.Value, &s); err == nil && s != "" {
			return s
		}
		// monolingual text: {"text":"...","language":"en"}
		var mt struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(c.Mainsnak.Datavalue.Value, &mt); err == nil && mt.Text != "" {
			return mt.Text
		}
	}
	return ""
}
