package spaces

// Ref: .jeos/sections/80-destination-wiki.md — Wiki-Only Space Types &
// Attractions amendment (attraction auto-population from Wikidata/
// Wikipedia, QID-first when the seeder stamped one).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// attractionFieldUUIDs are the deterministic UUIDs for the attraction
// field definitions seeded in migration 000795 — the same convention as
// universityFieldUUIDs so enrichment avoids a key lookup per write.
var attractionFieldUUIDs = struct {
	Kind         string
	Address      string
	Coordinates  string
	OpeningHours string
	EntryFee     string
	Website      string
	WikidataID   string
	Gallery      string
}{
	Kind:         "00000000-0000-0000-0000-000000000111",
	Address:      "00000000-0000-0000-0000-000000000112",
	Coordinates:  "00000000-0000-0000-0000-000000000113",
	OpeningHours: "00000000-0000-0000-0000-000000000114",
	EntryFee:     "00000000-0000-0000-0000-000000000115",
	Website:      "00000000-0000-0000-0000-000000000116",
	WikidataID:   "00000000-0000-0000-0000-000000000117",
	Gallery:      "00000000-0000-0000-0000-000000000118",
}

// attractionKindByQID maps common Wikidata P31 classes to the seeded
// `kind` select options (migration 000795). Anything unmapped falls through
// to the description keyword scan below, then "other".
var attractionKindByQID = map[string]string{
	"Q33506":   "museum",         // museum
	"Q4989906": "monument",       // monument
	"Q839954":  "historic-site",  // archaeological site
	"Q23413":   "historic-site",  // castle
	"Q16970":   "religious-site", // church building
	"Q32815":   "religious-site", // mosque
	"Q330284":  "market",         // market
	"Q34038":   "waterfall",      // waterfall
	"Q8502":    "mountain",       // mountain
	"Q23397":   "lake",           // lake
	"Q35509":   "cave",           // cave
	"Q40080":   "beach",          // beach
	"Q29553":   "mountain",       // hill (closest seeded kind)
	"Q1404150": "rock-formation", // rock formation (Olumo/Zuma Rock)
	"Q46169":   "national-park",  // national park
	"Q1107656": "garden",         // garden
	"Q167346":  "garden",         // botanical garden
	"Q43501":   "zoo",            // zoo
	"Q194195":  "theme-park",     // amusement park
	"Q570116":  "landmark",       // tourist attraction
	"Q811979":  "landmark",       // architectural structure
}

// attractionKindKeywords maps description keywords to kind values — the
// fallback when P31 classes don't cover the entity (e.g. inselbergs like
// Olumo/Zuma Rock whose class is a generic geological formation).
var attractionKindKeywords = []struct {
	kind string
	keys []string
}{
	{"waterfall", []string{"waterfall", "falls"}},
	{"mountain", []string{"mountain", "mount ", "peak", "volcano"}},
	{"rock-formation", []string{"rock formation", "monolith", "inselberg", "rock outcrop", "rock"}},
	{"canyon", []string{"canyon", "gorge"}},
	{"hot-spring", []string{"hot spring"}},
	{"lake", []string{"lake", "lagoon"}},
	{"cave", []string{"cave"}},
	{"beach", []string{"beach"}},
	{"national-park", []string{"national park", "game reserve"}},
	{"forest-reserve", []string{"forest reserve", "rainforest", "forest"}},
	{"theme-park", []string{"theme park", "amusement park"}},
	{"zoo", []string{"zoo", "zoological"}},
	{"garden", []string{"garden"}},
	{"museum", []string{"museum"}},
	{"monument", []string{"monument", "memorial"}},
	{"religious-site", []string{"church", "mosque", "cathedral", "shrine", "temple", "monastery"}},
	{"market", []string{"market"}},
	{"historic-site", []string{"historic site", "archaeological", "fort", "castle", "ruins", "palace"}},
	{"landmark", []string{"landmark", "tower", "tourist attraction", "monument"}},
}

// classifyAttractionKind picks the seeded `kind` value for a Wikidata
// entity: P31 class map first, description keywords as fallback.
func classifyAttractionKind(ent *wikidataUniversity) string {
	if ent == nil {
		return ""
	}

	for _, qid := range ent.InstanceOf {
		if kind, ok := attractionKindByQID[qid]; ok {
			return kind
		}
	}

	desc := strings.ToLower(ent.Description + " " + ent.Name)

	for _, kw := range attractionKindKeywords {
		for _, k := range kw.keys {
			if strings.Contains(desc, k) {
				return kw.kind
			}
		}
	}

	return "other"
}

// attractionSeededQID returns the Wikidata QID the seeder stamped — the
// authoritative wikidata_id custom field value first, then the
// wiki_content->>'wikidata_id' fallback (the populate write below merges
// the QID back in so the fallback survives enrichment).
func (a *ProcessorAdapter) attractionSeededQID(ctx context.Context, space *gen.GetSpaceByIDRow) string {
	if a.customFields != nil {
		var fieldID pgtype.UUID
		_ = fieldID.Scan(attractionFieldUUIDs.WikidataID)

		if v, err := a.customFields.GetFieldValue(ctx, fieldID, "space", space.ID); err == nil &&
			v.ValueText != nil && strings.HasPrefix(*v.ValueText, "Q") {
			return *v.ValueText
		}
	}

	return wikiContentWikidataID(space.WikiContent)
}

// populateAttractionSpace enriches an attraction wiki space from Wikidata +
// Wikipedia (§80): the seeded QID is fetched directly (name search would
// land Victoria Falls on the town, not the falls); the enwiki sitelink
// drives the Wikipedia summary; P18/P154 feed header/logo media; and the
// structured fields (kind, coordinates, website, wikidata_id) upsert
// through the custom-fields enricher.
func (a *ProcessorAdapter) populateAttractionSpace(ctx context.Context, space *gen.GetSpaceByIDRow) error {
	qid := a.attractionSeededQID(ctx, space)

	var ent *wikidataUniversity

	if qid != "" {
		e, err := fetchWikidataEntity(ctx, qid)
		if err != nil {
			if errors.Is(err, ErrWikiRateLimited) {
				return fmt.Errorf("fetching Wikidata entity %s: %w", qid, err)
			}

			slog.Warn("space:auto_populate — seeded QID fetch failed, falling back to name search",
				"space_id", space.ID.String(), "qid", qid, "error", err)
		} else {
			ent = e
		}
	}

	if ent == nil {
		ent = a.attractionWikidataEntity(ctx, space)
	}

	if ent != nil && qid == "" {
		qid = ent.WikidataID
	}

	// Wikipedia summary → wiki_content, with the QID merged back so dedup
	// survives the overwrite.
	wikiTitle := space.Name
	if ent != nil && ent.Sitelink != "" {
		wikiTitle = ent.Sitelink
	}

	var wikiSummary *wikipediaSummary

	var wikiErr error

	if s, err := fetchWikipediaSummary(ctx, wikiTitle); err == nil {
		wikiSummary = s

		wikiJSON, err := marshalWikiContentWithQID(wikiSummary, qid)
		if err == nil {
			if _, uerr := a.svc.UpdateSpaceWikiContent(ctx, space.ID, wikiJSON); uerr != nil {
				slog.Warn("space:auto_populate — failed to update attraction wiki content",
					"space_id", space.ID.String(), "error", uerr)
				return nil
			}
		}

		slog.Info("space:auto_populate — populated attraction wiki content",
			"space_id", space.ID.String(), "title", wikiTitle, "qid", qid)
	} else {
		if errors.Is(err, ErrWikiNotFound) {
			slog.Warn("space:auto_populate — no Wikipedia article for attraction",
				"space_id", space.ID.String(), "title", wikiTitle)
		} else {
			slog.Warn("space:auto_populate — Wikipedia fetch failed for attraction, will retry",
				"space_id", space.ID.String(), "title", wikiTitle, "error", err)

			wikiErr = fmt.Errorf("fetching Wikipedia summary for %q: %w", wikiTitle, err)
		}
	}

	// Media ingest is best-effort and idempotent on .Valid. Header
	// precedence matches populateGeoSpace: Wikidata P18 first, then the
	// freshly fetched summary's image — the space row is stale here (its
	// wiki_content predates the update above), so it is only a fallback
	// for runs where the summary fetch failed.
	var headerURL string
	if ent != nil {
		headerURL = ent.Image
	}

	if headerURL == "" && wikiSummary != nil {
		headerURL = firstNonEmpty(wikiSummary.OriginalImage, wikiSummary.Thumbnail)
	}

	if headerURL == "" {
		if b := wikiContentImageURL(space.WikiContent); b != "" {
			headerURL = b
		}
	}

	if !space.HeaderImageID.Valid {
		a.storeSpaceImage(ctx, space, headerURL, "header")
	}

	if !space.LogoMediaID.Valid && ent != nil {
		a.storeSpaceImage(ctx, space, firstNonEmpty(ent.Logo, ent.CoatOfArms), "logo")
	}

	a.enrichAttractionFields(ctx, space, ent, qid)
	a.linkAttractionParents(ctx, space, ent)

	return wikiErr
}

// linkAttractionParents resolves the attraction's Wikidata P131
// (located-in) chain against existing geo spaces and links every match
// as an additional parent via space_parent_spaces (§80 border
// attractions — Victoria Falls links both Livingstone-side and
// Victoria Falls Town-side chains). The trigger/primary parent set at
// create is untouched; AddParentSpace's ON CONFLICT DO NOTHING keeps
// re-runs idempotent and unmatched QIDs are skipped silently.
func (a *ProcessorAdapter) linkAttractionParents(ctx context.Context, space *gen.GetSpaceByIDRow, ent *wikidataUniversity) {
	if ent == nil || len(ent.LocatedIn) == 0 {
		return
	}

	matches, err := a.svc.read.SpaceIDsByWikidataIDs(ctx, ent.LocatedIn)
	if err != nil {
		slog.Warn("space:auto_populate — P131 parent resolution failed",
			"space_id", space.ID.String(), "error", err)
		return
	}

	if len(matches) == 0 {
		return
	}

	linked := map[pgtype.UUID]bool{space.ID: true}
	if space.ParentID.Valid {
		linked[space.ParentID] = true
	}

	if parents, perr := a.svc.GetParentSpaces(ctx, space.ID); perr == nil {
		for _, p := range parents {
			linked[p.ID] = true
		}
	}

	for _, m := range matches {
		// Only geographic containers are valid attraction parents —
		// a QID could collide with a stamped university or attraction.
		switch m.TypeSlug {
		case "country", "state", "city":
		default:
			continue
		}

		if linked[m.ID] {
			continue
		}

		if err := a.svc.AddParentSpace(ctx, space.ID, m.ID); err != nil {
			slog.Warn("space:auto_populate — P131 parent link failed",
				"space_id", space.ID.String(), "parent_id", m.ID.String(), "error", err)
			continue
		}

		linked[m.ID] = true

		slog.Info("space:auto_populate — linked P131 parent",
			"space_id", space.ID.String(), "parent_id", m.ID.String(), "parent_type", m.TypeSlug)
	}
}

// marshalWikiContentWithQID serializes the Wikipedia summary and merges in
// wikidata_id — the seeder's dedup token must survive the wiki_content
// rewrite (§80 review fix).
func marshalWikiContentWithQID(s *wikipediaSummary, qid string) ([]byte, error) {
	raw, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}

	if qid == "" {
		return raw, nil
	}

	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return raw, nil
	}

	m["wikidata_id"] = qid

	out, err := json.Marshal(m)
	if err != nil {
		return raw, nil
	}

	return out, nil
}

// wikiContentWikidataID reads the wikidata_id stamp out of a space's
// stored wiki_content JSON ("" when absent or unparseable).
func wikiContentWikidataID(wikiContent []byte) string {
	var m map[string]any
	if err := json.Unmarshal(wikiContent, &m); err != nil {
		return ""
	}

	if qid, ok := m["wikidata_id"].(string); ok && strings.HasPrefix(qid, "Q") {
		return qid
	}

	return ""
}

// stampSpaceWikidataID merges a Wikidata QID into a space's wiki_content
// without disturbing other keys — used when a space has no Wikipedia
// summary to rewrite (or when the seeder resolves a parent's QID). No-op
// when the stamp is already present, so retries stay idempotent.
func (a *ProcessorAdapter) stampSpaceWikidataID(ctx context.Context, space *gen.GetSpaceByIDRow, qid string) {
	if qid == "" || wikiContentWikidataID(space.WikiContent) == qid {
		return
	}

	var m map[string]any
	if err := json.Unmarshal(space.WikiContent, &m); err != nil || m == nil {
		m = map[string]any{}
	}

	m["wikidata_id"] = qid

	raw, err := json.Marshal(m)
	if err != nil {
		return
	}

	if _, err := a.svc.UpdateSpaceWikiContent(ctx, space.ID, raw); err != nil {
		slog.Warn("spaces: wikidata_id stamp failed", "space_id", space.ID.String(), "qid", qid, "error", err)
	}
}

// wikiContentImageURL pulls the best image URL out of a stored
// wikipediaSummary JSON — the header source when Wikidata has no P18.
func wikiContentImageURL(wikiContent []byte) string {
	var m map[string]any
	if err := json.Unmarshal(wikiContent, &m); err != nil {
		return ""
	}

	for _, k := range []string{"original_image", "thumbnail"} {
		if s, ok := m[k].(string); ok && s != "" {
			return s
		}
	}

	return ""
}

// attractionWikidataEntity resolves a Wikidata entity by name search when
// no QID was seeded — hits are ranked by parent-name and attraction-kind
// keywords, then probed until one carries an image or identity data.
func (a *ProcessorAdapter) attractionWikidataEntity(ctx context.Context, space *gen.GetSpaceByIDRow) *wikidataUniversity {
	parentName := ""

	if space.ParentID.Valid {
		if parent, err := a.svc.GetSpace(ctx, space.ParentID); err == nil {
			parentName = strings.ToLower(parent.Name)
		}
	}

	results, err := searchWikidataEntities(ctx, space.Name, 5)
	if err != nil {
		return nil
	}

	keys := []string{
		"attraction", "landmark", "museum", "monument", "waterfall", "mountain",
		"rock", "lake", "cave", "beach", "park", "garden", "zoo", "falls",
		"site", "fort", "castle", "church", "mosque", "market",
	}
	score := func(r wikidataSearchResult) int {
		desc := strings.ToLower(r.Description)
		s := 0

		if parentName != "" && strings.Contains(desc, parentName) {
			s += 2
		}

		for _, kw := range keys {
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

			if ent.Image != "" || ent.Sitelink != "" || len(ent.InstanceOf) > 0 {
				return ent
			}
		}
	}

	return nil
}

// enrichAttractionFields upserts the attraction field values from the
// Wikidata entity — kind (select), coordinates, website, wikidata_id.
// Best-effort: errors are logged, never returned.
func (a *ProcessorAdapter) enrichAttractionFields(ctx context.Context, space *gen.GetSpaceByIDRow, ent *wikidataUniversity, qid string) {
	if a.customFields == nil {
		return
	}

	upsertText := func(fieldUUID, value string) {
		if value == "" {
			return
		}

		var fieldID pgtype.UUID
		_ = fieldID.Scan(fieldUUID)

		if _, err := a.customFields.UpsertFieldValue(ctx, gen.UpsertCustomFieldValueParams{
			FieldID:    fieldID,
			EntityType: "space",
			EntityID:   space.ID,
			ValueText:  &value,
		}); err != nil {
			slog.Warn("space:auto_populate — attraction field upsert failed",
				"space_id", space.ID.String(), "field", fieldUUID, "error", err)
		}
	}

	// wikidata_id always writes — it is the seeder's dedup token.
	upsertText(attractionFieldUUIDs.WikidataID, qid)

	if ent == nil {
		return
	}

	upsertText(attractionFieldUUIDs.Coordinates, ent.Coords)
	upsertText(attractionFieldUUIDs.Website, ent.Website)

	if kind := classifyAttractionKind(ent); kind != "" {
		var fieldID pgtype.UUID
		_ = fieldID.Scan(attractionFieldUUIDs.Kind)

		vj, _ := json.Marshal(map[string]string{"value": kind})
		if _, err := a.customFields.UpsertFieldValue(ctx, gen.UpsertCustomFieldValueParams{
			FieldID:    fieldID,
			EntityType: "space",
			EntityID:   space.ID,
			ValueJson:  vj,
		}); err != nil {
			slog.Warn("space:auto_populate — attraction kind upsert failed",
				"space_id", space.ID.String(), "error", err)
		}
	}
}
