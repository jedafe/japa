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
	"Q811979",  // architectural structure
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

// AttractionCandidate is one Wikidata hit for the admin dry-run preview.
type AttractionCandidate struct {
	QID         string `json:"qid"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
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

// parentWikidataQID resolves the parent's Wikidata entity by name — geo
// spaces don't stamp wikidata_id today, so the seeder reuses the geo
// disambiguator (parent-name + level keywords) to find it.
func (a *ProcessorAdapter) parentWikidataQID(ctx context.Context, parent *gen.GetSpaceByIDRow) (string, error) {
	ent := a.geoWikidataEntity(ctx, parent, parent.Level)
	if ent == nil || ent.WikidataID == "" {
		return "", fmt.Errorf("wiki:seed_attractions: could not resolve Wikidata entity for parent %q", parent.Name)
	}

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
		}, a.systemUserID, []pgtype.UUID{parentID})
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

	return res, nil
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

	for i := 0; i < 20; i++ {
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

	query := fmt.Sprintf(`SELECT DISTINCT ?item ?itemLabel ?itemDescription WHERE {
  VALUES ?class { %s}
  ?item wdt:P31/wdt:P279* ?class .
  ?item wdt:P131* wd:%s .
  SERVICE wikibase:label { bd:serviceParam wikibase:language "en". }
}
LIMIT %d`, classValues.String(), parentQID, limit)

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
			} `json:"bindings"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &sr); err != nil {
		return nil, fmt.Errorf("decoding SPARQL response: %w", err)
	}

	var out []AttractionCandidate
	seen := make(map[string]struct{}, len(sr.Results.Bindings))
	for _, b := range sr.Results.Bindings {
		// item.value is the entity URI — the QID is the last path segment.
		qid := b.Item.Value
		if i := strings.LastIndexByte(qid, '/'); i >= 0 {
			qid = qid[i+1:]
		}
		if qid == "" || b.ItemLabel.Value == "" || strings.HasPrefix(b.ItemLabel.Value, "Q") {
			continue // unlabeled entity — the label service fell back to the QID
		}
		if _, dup := seen[qid]; dup {
			continue
		}
		seen[qid] = struct{}{}
		out = append(out, AttractionCandidate{
			QID:         qid,
			Name:        b.ItemLabel.Value,
			Description: b.ItemDescription.Value,
		})
	}

	slog.Info("wiki:seed_attractions: SPARQL returned candidates",
		"parent_qid", parentQID, "bindings", len(sr.Results.Bindings), "candidates", len(out))

	return out, nil
}
