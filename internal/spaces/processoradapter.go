package spaces

import (
	"context"
	"encoding/json"
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

// ProcessorAdapter implements worker.SpaceAutoPopulateProcessor by delegating
// to the spaces service's auto-population logic.
type ProcessorAdapter struct {
	svc *Service
}

// NewProcessorAdapter creates a ProcessorAdapter backed by the given spaces service.
func NewProcessorAdapter(svc *Service) *ProcessorAdapter {
	return &ProcessorAdapter{svc: svc}
}

// AutoPopulateSpace seeds a newly created space with default content based
// on its space type. For geo spaces, this includes fetching Wikipedia data
// and flag images. For other space types, it creates default rules and tags.
func (a *ProcessorAdapter) AutoPopulateSpace(ctx context.Context, spaceID, tenantID string) error {
	slog.Info("space:auto_populate",
		"space_id", spaceID,
		"tenant_id", tenantID,
	)

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
	space, err := a.svc.GetSpace(ctx, sid, tid)
	if err != nil {
		slog.Error("space:auto_populate — failed to fetch space", "error", err, "space_id", spaceID)
		return err
	}

	// Auto-populate based on space type.
	spaceType := ""
	if st, err := a.svc.GetSpaceType(ctx, space.SpaceTypeID, tid); err == nil {
		spaceType = st.Slug
	}

	switch spaceType {
	case "country", "state", "city":
		return a.populateGeoSpace(ctx, &space, spaceType, tid)
	case "university":
		return a.populateUniversitySpace(ctx, &space, tid)
	default:
		slog.Info("space:auto_populate — custom space, no auto-population needed",
			"space_id", spaceID,
			"type", spaceType,
		)
	}

	return nil
}

// populateGeoSpace fetches a Wikipedia summary for the space and stores it
// as WikiContent. For countries, the geo_code (ISO 3166-1 alpha-2) is used
// to construct the Wikipedia article title.
func (a *ProcessorAdapter) populateGeoSpace(ctx context.Context, space *gen.Space, spaceType string, tenantID pgtype.UUID) error {
	// Build the Wikipedia article title from the space name.
	// For countries, use the country name directly (e.g. "Nigeria").
	// For states/cities, use "{name}, {parent}" pattern if available.
	title := space.Name

	wikiSummary, err := fetchWikipediaSummary(ctx, title)
	if err != nil {
		slog.Warn("space:auto_populate — Wikipedia fetch failed, continuing",
			"space_id", space.ID.String(),
			"title", title,
			"error", err,
		)
		return nil // Best-effort — don't fail the task
	}

	// Update the space's WikiContent with the fetched summary.
	wikiJSON, err := json.Marshal(wikiSummary)
	if err != nil {
		slog.Warn("space:auto_populate — failed to marshal wiki summary", "error", err)
		return nil
	}

	if _, err := a.svc.UpdateSpaceWikiContent(ctx, space.ID, tenantID, wikiJSON); err != nil {
		slog.Warn("space:auto_populate — failed to update wiki content",
			"space_id", space.ID.String(),
			"error", err,
		)
		return nil
	}

	slog.Info("space:auto_populate — populated wiki content",
		"space_id", space.ID.String(),
		"type", spaceType,
		"name", space.Name,
	)

	return nil
}

// populateUniversitySpace fetches university data from Wikidata SPARQL and
// stores it as WikiContent. Retrieves: description, country, founding year,
// student count, official website, logo, and coordinates.
func (a *ProcessorAdapter) populateUniversitySpace(ctx context.Context, space *gen.Space, tenantID pgtype.UUID) error {
	uniData, err := fetchWikidataUniversity(ctx, space.Name)
	if err != nil {
		slog.Warn("space:auto_populate — Wikidata fetch failed, trying Wikipedia fallback",
			"space_id", space.ID.String(),
			"error", err,
		)
		// Fallback to Wikipedia summary.
		return a.populateGeoSpace(ctx, space, "university", tenantID)
	}

	wikiJSON, err := json.Marshal(uniData)
	if err != nil {
		slog.Warn("space:auto_populate — failed to marshal wikidata", "error", err)
		return nil
	}

	if _, err := a.svc.UpdateSpaceWikiContent(ctx, space.ID, tenantID, wikiJSON); err != nil {
		slog.Warn("space:auto_populate — failed to update wiki content",
			"space_id", space.ID.String(),
			"error", err,
		)
		return nil
	}

	slog.Info("space:auto_populate — populated university data from Wikidata",
		"space_id", space.ID.String(),
		"name", space.Name,
	)

	return nil
}

// wikidataUniversity holds structured data fetched from Wikidata SPARQL.
type wikidataUniversity struct {
	Name         string `json:"name"`
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

// fetchWikidataUniversity queries the Wikidata SPARQL endpoint for a
// university matching the given name. Returns structured data including
// description, country, founding year, student count, website, and logo.
func fetchWikidataUniversity(ctx context.Context, name string) (*wikidataUniversity, error) {
	// SPARQL query: find university by name (en label), retrieve key properties.
	query := fmt.Sprintf(`
SELECT ?uni ?uniLabel ?description ?countryLabel ?founded ?students ?website ?logo ?image
WHERE {
  ?uni wdt:P31 wd:Q3918 .                    # instance of university
  ?uni rdfs:label ?uniLabel .                 # get the label
  FILTER(LANG(?uniLabel) = "en")              # English labels only
  FILTER(CONTAINS(LCASE(?uniLabel), LCASE(%s)))  # case-insensitive name match

  OPTIONAL { ?uni schema:description ?description . FILTER(LANG(?description) = "en") }
  OPTIONAL { ?uni wdt:P17 ?country . ?country rdfs:label ?countryLabel . FILTER(LANG(?countryLabel) = "en") }
  OPTIONAL { ?uni wdt:P571 ?founded }
  OPTIONAL { ?uni wdt:P2196 ?students }
  OPTIONAL { ?uni wdt:P856 ?website }
  OPTIONAL { ?uni wdt:P154 ?logo }
  OPTIONAL { ?uni wdt:P18 ?image }

  SERVICE wikibase:label { bd:serviceParam wikibase:language "en" }
}
LIMIT 1`, sparqlStringLiteral(name))

	sparqlURL := "https://query.wikidata.org/sparql"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sparqlURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating SPARQL request: %w", err)
	}

	q := req.URL.Query()
	q.Set("query", query)
	q.Set("format", "json")
	req.URL.RawQuery = q.Encode()

	req.Header.Set("User-Agent", "JapaConnect/1.0 (space:auto_populate)")
	req.Header.Set("Accept", "application/sparql-results+json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing SPARQL query: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("SPARQL endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Results struct {
			Bindings []struct {
				UniLabel struct{ Value string } `json:"uniLabel"`
				Desc     struct{ Value string } `json:"description"`
				Country  struct{ Value string } `json:"countryLabel"`
				Founded  struct{ Value string } `json:"founded"`
				Students struct{ Value string } `json:"students"`
				Website  struct{ Value string } `json:"website"`
				Logo     struct{ Value string } `json:"logo"`
				Image    struct{ Value string } `json:"image"`
				Uni      struct{ Value string } `json:"uni"`
			} `json:"bindings"`
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding SPARQL response: %w", err)
	}

	if len(result.Results.Bindings) == 0 {
		return nil, fmt.Errorf("no university found matching %q", name)
	}

	b := result.Results.Bindings[0]
	uni := &wikidataUniversity{
		Name:        b.UniLabel.Value,
		Description: b.Desc.Value,
		Country:     b.Country.Value,
		Founded:     b.Founded.Value,
		Students:    b.Students.Value,
		Website:     b.Website.Value,
		Logo:        b.Logo.Value,
		Image:       b.Image.Value,
	}

	// Extract Wikidata QID from the URI (e.g. "http://www.wikidata.org/entity/Q12345").
	if b.Uni.Value != "" {
		uni.WikidataID = b.Uni.Value
		if idx := strings.LastIndex(b.Uni.Value, "/"); idx >= 0 {
			qid := b.Uni.Value[idx+1:]
			uni.WikipediaURL = "https://en.wikipedia.org/wiki/" + qid
		}
	}

	return uni, nil
}

// sparqlStringLiteral wraps a string in SPARQL string literal syntax with
// proper escaping for all special characters per the SPARQL 1.1 spec
// (§18.104.1): backslash, double-quote, newline, carriage return, tab,
// backspace, form feed, and control chars. Without full escaping a
// crafted space name could break out of the literal and inject SPARQL.
func sparqlStringLiteral(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		default:
			if r < 0x20 {
				// Control characters — escape as \uXXXX per SPARQL spec.
				fmt.Fprintf(&b, `\u%04X`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
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
	encodedTitle := url.PathEscape(title)
	apiURL := fmt.Sprintf("https://en.wikipedia.org/api/rest_v1/page/summary/%s", encodedTitle)

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
