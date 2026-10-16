package spaces

// Ref: .jeos/sections/80-destination-wiki.md — Wiki-Only Space Types &
// Attractions amendment (wiki:seed_attractions — Wikidata SPARQL bulk seed
// of attraction spaces under a geo parent; dedup on wikidata_id + slug;
// each created space flows through space:auto_populate).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

const (
	// attractionSeedDefaultLimit / attractionSeedMaxLimit bound a single
	// seed run — same package-const pattern as defaultDirectoryLimit (no
	// new system_settings key).
	attractionSeedDefaultLimit = 100
	attractionSeedMaxLimit     = 500
)

// wikidataSPARQLEndpoint is a var so tests can point the seeder at a stub.
var wikidataSPARQLEndpoint = "https://query.wikidata.org/sparql"

// attractionClassQIDs is the union of Wikidata classes (matched via
// wdt:P31/wdt:P279*) the seeder treats as attractions — built kinds,
// natural wonders, and parks/leisure per §80. Keyword fallbacks in
// classifyAttractionKind cover classes not listed here.
var attractionClassQIDs = []string{
	"Q570116",  // tourist attraction
	"Q33506",   // museum
	"Q4989906", // monument
	// Q811979 (architectural structure) is intentionally NOT here: hospitals,
	// schools, and other civic buildings subclass it, so the P31/P279*
	// closure seeded them as "attractions". Concrete built kinds stay listed.
	"Q839954",  // archaeological site
	"Q23413",   // castle
	"Q12518",   // tower
	"Q16970",   // church building
	"Q32815",   // mosque
	"Q44539",   // temple
	"Q330284",  // market
	"Q34038",   // waterfall
	"Q8502",    // mountain
	"Q46831",   // mountain range
	"Q29553",   // hill
	"Q1404150", // rock formation
	"Q23397",   // lake
	"Q35509",   // cave
	"Q40080",   // beach
	"Q150636",  // canyon? (gorge)
	"Q46169",   // national park
	"Q473972",  // protected area
	"Q4421",    // forest
	"Q1107656", // garden
	"Q167346",  // botanical garden
	"Q43501",   // zoo
	"Q194195",  // amusement park
}

// attractionExcludedQIDs are classes an attraction must never be an
// instance of (transitively) — civic infrastructure whose subclasses can
// still reach a listed class (e.g. hospital → built structure before
// Q811979 was dropped; belt-and-suspenders for future additions).
var attractionExcludedQIDs = []string{
	"Q16917",   // hospital
	"Q3914",    // school
	"Q3918",    // university — its own wiki type
	"Q1248784", // airport
	"Q40357",   // prison
	"Q55488",   // railway station
}

// AttractionCandidate is one Wikidata hit for the admin dry-run preview.
// LocatedIn carries the item's direct P131 (located-in) QIDs — matched
// against stamped geo spaces at create so border attractions link every
// existing parent, not just the trigger space (§80).
type AttractionCandidate struct {
	QID         string   `json:"qid"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	LocatedIn   []string `json:"located_in,omitempty"`
}

// AttractionSeedResult summarises a seed run for the task log and the
// dry-run admin response.
type AttractionSeedResult struct {
	Candidates       int      `json:"candidates"`
	Created          int      `json:"created"`
	SkippedDuplicate int      `json:"skipped_duplicate"`
	Failed           int      `json:"failed"`
	Slugs            []string `json:"slugs,omitempty"`
}

// SeedAttractions is the worker.WikiSeedAttractionsProcessor entrypoint —
// string payload per the §11 catalog contract.
func (a *ProcessorAdapter) SeedAttractions(ctx context.Context, parentSpaceID string, limit int, dryRun bool) error {
	var parentID pgtype.UUID
	if err := parentID.Scan(parentSpaceID); err != nil || !parentID.Valid {
		return fmt.Errorf("wiki:seed_attractions: invalid parent_space_id %q", parentSpaceID)
	}

	res, err := a.seedAttractions(ctx, parentID, limit, dryRun)
	if err != nil {
		return err
	}

	slog.Info("wiki:seed_attractions: completed",
		"parent_space_id", parentSpaceID, "dry_run", dryRun,
		"candidates", res.Candidates, "created", res.Created,
		"skipped_duplicate", res.SkippedDuplicate, "failed", res.Failed)

	return nil
}

// ListAttractionCandidates returns the Wikidata candidates a seed run
// would create — the dry-run preview for POST /admin/wiki/seed/attractions.
func (a *ProcessorAdapter) ListAttractionCandidates(ctx context.Context, parentSpaceID pgtype.UUID, limit int) ([]AttractionCandidate, error) {
	limit = clampAttractionSeedLimit(limit)

	parent, err := a.svc.GetSpace(ctx, parentSpaceID)
	if err != nil {
		return nil, fmt.Errorf("wiki:seed_attractions: getting parent space: %w", err)
	}

	parentQID, err := a.parentWikidataQID(ctx, &parent)
	if err != nil {
		return nil, err
	}

	return fetchAttractionCandidates(ctx, parentQID, limit)
}

// parentWikidataQID resolves the parent's Wikidata QID — the
// wiki_content stamp first (free), then the geo name-search
// disambiguator, whose result is stamped back so later runs and P131
// lookups skip the search.
func (a *ProcessorAdapter) parentWikidataQID(ctx context.Context, parent *gen.GetSpaceByIDRow) (string, error) {
	if qid := wikiContentWikidataID(parent.WikiContent); qid != "" {
		return qid, nil
	}

	ent := a.geoWikidataEntity(ctx, parent, parent.Level)
	if ent == nil || ent.WikidataID == "" {
		return "", fmt.Errorf("wiki:seed_attractions: could not resolve Wikidata entity for parent %q", parent.Name)
	}

	a.stampSpaceWikidataID(ctx, parent, ent.WikidataID)

	return ent.WikidataID, nil
}

func clampAttractionSeedLimit(limit int) int {
	if limit <= 0 {
		return attractionSeedDefaultLimit
	}

	if limit > attractionSeedMaxLimit {
		return attractionSeedMaxLimit
	}

	return limit
}

// seedAttractions is the QID-deduped create loop: candidates → per-item
// dedup → CreateSpaceWithOwner (space_parent_spaces link lands in the same
// tx) → wikidata_id cfv → space:auto_populate + search index enqueue.
func (a *ProcessorAdapter) seedAttractions(ctx context.Context, parentID pgtype.UUID, limit int, dryRun bool) (*AttractionSeedResult, error) {
	limit = clampAttractionSeedLimit(limit)

	attractionType, err := a.svc.GetSpaceTypeBySlug(ctx, "attraction")
	if err != nil {
		return nil, fmt.Errorf("wiki:seed_attractions: attraction space type not seeded: %w", err)
	}

	candidates, err := a.ListAttractionCandidates(ctx, parentID, limit)
	if err != nil {
		return nil, err
	}

	res := &AttractionSeedResult{Candidates: len(candidates)}

	if dryRun {
		for _, c := range candidates {
			res.Slugs = append(res.Slugs, slugify(c.Name))
		}

		return res, nil
	}

	// §80 border attractions — resolve every candidate's P131 located-in
	// QIDs against stamped geo spaces in one batch; each create links the
	// trigger parent plus every matched extra parent. The seed target's
	// ancestor chain links too — a geo parent without a wikidata_id stamp
	// can't match a P131 QID but is still a legitimate parent.
	extraParents := a.candidateGeoParents(ctx, candidates, parentID)

	// Geo ancestors missing the wikidata_id stamp get auto-populate
	// enqueued (fire-and-forget) so future P131 resolution can match them.
	a.enqueueUnstampedGeoAncestors(ctx, parentID)

	for _, c := range candidates {
		if c.QID == "" || c.Name == "" {
			res.Failed++
			continue
		}

		exists, err := a.svc.read.SpaceExistsByWikidataID(ctx, &c.QID)
		if err != nil {
			slog.Warn("wiki:seed_attractions: dedup check failed", "qid", c.QID, "error", err)

			res.Failed++

			continue
		}

		if exists != nil && *exists {
			res.SkippedDuplicate++
			continue
		}

		slug, err := a.uniqueAttractionSlug(ctx, c.Name)
		if err != nil {
			slog.Warn("wiki:seed_attractions: slug allocation failed", "name", c.Name, "error", err)

			res.Failed++

			continue
		}

		axes := WikiOnlyPermAxes()
		stamp, _ := json.Marshal(map[string]string{"wikidata_id": c.QID})
		desc := c.Description

		space, err := a.svc.CreateSpaceWithOwner(ctx, gen.CreateSpaceParams{
			ParentID:       parentID,
			SpaceTypeID:    attractionType.ID,
			Name:           c.Name,
			Slug:           slug,
			Description:    &desc,
			WikiContent:    stamp,
			Level:          "custom",
			Visibility:     string(axes.Visibility),
			PostingPerm:    string(axes.Posting),
			ReplyingPerm:   string(axes.Replying),
			CommentingPerm: string(axes.Commenting),
			JoiningPerm:    string(axes.Joining),
			Status:         "active",
		}, a.systemUserID, extraParents[c.QID])
		if err != nil {
			slog.Warn("wiki:seed_attractions: create failed", "name", c.Name, "qid", c.QID, "error", err)

			res.Failed++

			continue
		}

		// The wikidata_id cfv is the authoritative dedup token — it
		// survives the wiki_content rewrite in populateAttractionSpace.
		a.upsertAttractionTextField(ctx, space.ID, attractionFieldUUIDs.WikidataID, c.QID)

		a.svc.EnqueueAutoPopulate(ctx, space.ID)
		a.svc.EnqueueSearchIndex(ctx, space.ID, "index")

		res.Created++
		res.Slugs = append(res.Slugs, slug)
	}

	// §80 — the run summary lands in audit_logs so bulk creates are
	// traceable (no actor: the task runs headless, actor_user_id is NULL).
	if meta, merr := json.Marshal(res); merr == nil {
		targetType := "space"
		if _, aerr := a.svc.write.CreateAuditLog(ctx, gen.CreateAuditLogParams{
			ActionType: "wiki.seed_attractions",
			TargetType: &targetType,
			TargetID:   parentID,
			Metadata:   meta,
		}); aerr != nil {
			slog.Warn("wiki:seed_attractions: audit log write failed",
				"parent_space_id", parentID.String(), "error", aerr)
		}
	}

	return res, nil
}

// candidateGeoParents resolves each candidate's P131 located-in chain to
// existing geo spaces — one batch lookup for the whole candidate set —
// returning the full parent list per candidate QID (trigger parent first,
// matched extras after, deduped). Resolution failures degrade to the
// trigger parent alone, never fail the seed.
func (a *ProcessorAdapter) candidateGeoParents(ctx context.Context, candidates []AttractionCandidate, parentID pgtype.UUID) map[string][]pgtype.UUID {
	out := make(map[string][]pgtype.UUID, len(candidates))
	for _, c := range candidates {
		out[c.QID] = []pgtype.UUID{parentID}
	}

	var qids []string

	seenQID := map[string]bool{}

	for _, c := range candidates {
		for _, q := range c.LocatedIn {
			if !seenQID[q] {
				seenQID[q] = true

				qids = append(qids, q)
			}
		}
	}

	if len(qids) == 0 {
		return out
	}

	matches, err := a.svc.read.SpaceIDsByWikidataIDs(ctx, qids)
	if err != nil {
		slog.Warn("wiki:seed_attractions: P131 parent resolution failed", "error", err)
		return out
	}

	// QID → matched geo space IDs (country/state/city only — a QID could
	// collide with a stamped university or attraction).
	byQID := make(map[string][]pgtype.UUID, len(matches))

	for _, m := range matches {
		switch m.TypeSlug {
		case "country", "state", "city":
		default:
			continue
		}

		byQID[m.Qid] = append(byQID[m.Qid], m.ID)
	}

	// The trigger parent's own ancestor chain (city → state → country)
	// always links — P131 matching only reaches stamped geo spaces, and
	// geo ancestors may lack the wikidata_id stamp on first runs.
	ancestors, err := a.svc.read.ListSpaceAncestors(ctx, parentID)
	if err != nil {
		slog.Warn("wiki:seed_attractions: ancestor lookup failed", "error", err)
	}

	for _, c := range candidates {
		seen := map[pgtype.UUID]bool{parentID: true}
		for _, anc := range ancestors {
			if !seen[anc.ID] && isGeoAncestor(anc) {
				seen[anc.ID] = true

				out[c.QID] = append(out[c.QID], anc.ID)
			}
		}

		for _, q := range c.LocatedIn {
			for _, id := range byQID[q] {
				if !seen[id] {
					seen[id] = true

					out[c.QID] = append(out[c.QID], id)
				}
			}
		}
	}

	return out
}

// isGeoAncestor keeps the parent list to geo spaces — an attraction must
// never link a community/custom ancestor.
func isGeoAncestor(anc gen.ListSpaceAncestorsRow) bool {
	switch anc.TypeSlug {
	case "country", "state", "city":
		return true
	}

	return false
}

// enqueueUnstampedGeoAncestors fires space:auto_populate for every geo
// ancestor of the seed parent whose wiki_content lacks a wikidata_id
// stamp — that's the write path SpaceIDsByWikidataIDs reads, so stamped
// ancestors make P131 multi-parent linking work on later runs.
func (a *ProcessorAdapter) enqueueUnstampedGeoAncestors(ctx context.Context, parentID pgtype.UUID) {
	ancestors, err := a.svc.read.ListSpaceAncestors(ctx, parentID)
	if err != nil {
		slog.Warn("wiki:seed_attractions: ancestor populate check failed", "error", err)
		return
	}

	for _, anc := range ancestors {
		if isGeoAncestor(anc) && !anc.HasWikidataID {
			a.svc.EnqueueAutoPopulate(ctx, anc.ID)
		}
	}
}

// upsertAttractionTextField is the value_text upsert shared by the seeder
// (wikidata_id stamp) and enrichAttractionFields.
func (a *ProcessorAdapter) upsertAttractionTextField(ctx context.Context, spaceID pgtype.UUID, fieldUUID, value string) {
	if a.customFields == nil || value == "" {
		return
	}

	var fieldID pgtype.UUID
	_ = fieldID.Scan(fieldUUID)

	if _, err := a.customFields.UpsertFieldValue(ctx, gen.UpsertCustomFieldValueParams{
		FieldID:    fieldID,
		EntityType: "space",
		EntityID:   spaceID,
		ValueText:  &value,
	}); err != nil {
		slog.Warn("spaces: attraction field upsert failed", "space_id", spaceID.String(), "field", fieldUUID, "error", err)
	}
}

// uniqueAttractionSlug allocates a globally-unique slug (spaces.slug is
// unique) with -2/-3/… suffixes on collision.
func (a *ProcessorAdapter) uniqueAttractionSlug(ctx context.Context, name string) (string, error) {
	base := slugify(name)
	if base == "" {
		base = "attraction"
	}

	for i := range 20 {
		candidate := base
		if i > 0 {
			candidate = fmt.Sprintf("%s-%d", base, i+1)
		}

		if _, err := a.svc.GetSpaceBySlug(ctx, candidate); err != nil {
			return candidate, nil
		}
	}

	return "", errors.New("no free slug after 20 suffixes")
}

// fetchAttractionCandidates runs the SPARQL query against the Wikidata
// Query Service — attractions (P31/P279* class union) located-in (P131*,
// transitive so a city's attractions surface for its country too is not
// needed here; the trigger space's subtree is what seeds).
func fetchAttractionCandidates(ctx context.Context, parentQID string, limit int) ([]AttractionCandidate, error) {
	if err := wikiRateLimiter.wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter: %w", err)
	}

	var classValues strings.Builder
	for _, qid := range attractionClassQIDs {
		classValues.WriteString("wd:" + qid + " ")
	}

	var excludedValues strings.Builder
	for _, qid := range attractionExcludedQIDs {
		excludedValues.WriteString("wd:" + qid + " ")
	}

	// ?loc collects the item's direct P131 located-in entities — border
	// attractions get one binding per location, folded into
	// AttractionCandidate.LocatedIn for multi-parent linking. The FILTER
	// NOT EXISTS drops civic infrastructure (hospitals, schools, …) that
	// the broad class union would otherwise admit transitively.
	query := fmt.Sprintf(`SELECT DISTINCT ?item ?itemLabel ?itemDescription ?loc WHERE {
  VALUES ?class { %s}
  ?item wdt:P31/wdt:P279* ?class .
  ?item wdt:P131* wd:%s .
  FILTER NOT EXISTS {
    VALUES ?excluded { %s}
    ?item wdt:P31/wdt:P279* ?excluded .
  }
  OPTIONAL { ?item wdt:P131 ?loc }
  SERVICE wikibase:label { bd:serviceParam wikibase:language "en". }
}
LIMIT %d`, classValues.String(), parentQID, excludedValues.String(), limit)

	u, err := url.Parse(wikidataSPARQLEndpoint)
	if err != nil {
		return nil, err
	}

	q := u.Query()
	q.Set("query", query)
	q.Set("format", "json")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/sparql-results+json")
	req.Header.Set("User-Agent", "JapaConnect/1.0 (wiki:seed_attractions)")

	client := &http.Client{Timeout: 60 * time.Second} // SPARQL unions are slow

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("wikidata SPARQL request: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, ErrWikiRateLimited
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("wikidata SPARQL status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("reading SPARQL response: %w", err)
	}

	var sr struct {
		Results struct {
			Bindings []struct {
				Item struct {
					Value string `json:"value"`
				} `json:"item"`
				ItemLabel struct {
					Value string `json:"value"`
				} `json:"itemLabel"`
				ItemDescription struct {
					Value string `json:"value"`
				} `json:"itemDescription"`
				Loc *struct {
					Value string `json:"value"`
				} `json:"loc"`
			} `json:"bindings"`
		} `json:"results"`
	}

	if err := json.Unmarshal(body, &sr); err != nil {
		return nil, fmt.Errorf("decoding SPARQL response: %w", err)
	}

	// One candidate per item QID — multi-location items repeat across
	// bindings, each carrying one ?loc.
	byItem := make(map[string]*AttractionCandidate, len(sr.Results.Bindings))

	var order []string

	lastSeg := func(uri string) string {
		if i := strings.LastIndexByte(uri, '/'); i >= 0 {
			return uri[i+1:]
		}

		return uri
	}
	for _, b := range sr.Results.Bindings {
		// item.value is the entity URI — the QID is the last path segment.
		qid := lastSeg(b.Item.Value)
		if qid == "" || b.ItemLabel.Value == "" || strings.HasPrefix(b.ItemLabel.Value, "Q") {
			continue // unlabeled entity — the label service fell back to the QID
		}

		c, ok := byItem[qid]
		if !ok {
			c = &AttractionCandidate{
				QID:         qid,
				Name:        b.ItemLabel.Value,
				Description: b.ItemDescription.Value,
			}
			byItem[qid] = c

			order = append(order, qid)
		}

		if b.Loc != nil {
			if loc := lastSeg(b.Loc.Value); strings.HasPrefix(loc, "Q") && !slices.Contains(c.LocatedIn, loc) {
				c.LocatedIn = append(c.LocatedIn, loc)
			}
		}
	}

	out := make([]AttractionCandidate, 0, len(order))
	for _, qid := range order {
		out = append(out, *byItem[qid])
	}

	slog.Info("wiki:seed_attractions: SPARQL returned candidates",
		"parent_qid", parentQID, "bindings", len(sr.Results.Bindings), "candidates", len(out))

	return out, nil
}
