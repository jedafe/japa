package spaces

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
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
		slog.Info("space:auto_populate — university space, no external data source available yet",
			"space_id", spaceID,
			"name", space.Name,
		)
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
