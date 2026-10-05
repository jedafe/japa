package spaces

// Ref: .jeos/sections/80-destination-wiki.md — Wiki-Only Space Types &
// Attractions amendment. POST /api/v1/admin/wiki/seed/attractions
// (wiki.manage): dry_run returns the Wikidata candidate list synchronously;
// otherwise enqueues wiki:seed_attractions (§11 catalog) and returns 202.

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// attractionSeeder is the subset of *ProcessorAdapter the seed route needs
// for dry-run previews — same package, so no shared types leak into wiki.
type attractionSeeder interface {
	ListAttractionCandidates(ctx context.Context, parentSpaceID pgtype.UUID, limit int) ([]AttractionCandidate, error)
}

// SetAttractionSeeder wires the Wikidata candidate lister used by
// POST /admin/wiki/seed/attractions dry-run previews. Optional.
func (h *Handlers) SetAttractionSeeder(s attractionSeeder) {
	h.attrSeeder = s
}

// HandleSeedAttractions — POST /api/v1/admin/wiki/seed/attractions.
// Body: {within: "<space_slug>", limit?: int, dry_run?: bool}.
func (h *Handlers) HandleSeedAttractions(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Within string `json:"within"`
		Limit  int    `json:"limit"`
		DryRun bool   `json:"dry_run"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	if body.Within == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_within", "within is required")
		return
	}

	space, err := h.Service.GetSpaceBySlug(r.Context(), body.Within)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "space_not_found", "Parent space not found")
		return
	}
	switch space.Level {
	case LevelCountry, LevelState, LevelCity:
	default:
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_parent",
			"Attractions seed under a country, state, or city space")
		return
	}

	limit := clampAttractionSeedLimit(body.Limit)

	if body.DryRun {
		if h.attrSeeder == nil {
			errpkg.WriteError(w, http.StatusServiceUnavailable, "not_configured",
				"Attraction seeder is not configured")
			return
		}
		candidates, err := h.attrSeeder.ListAttractionCandidates(r.Context(), space.ID, limit)
		if err != nil {
			errpkg.WriteError(w, http.StatusBadGateway, "wikidata_error",
				"Failed to query Wikidata candidates")
			return
		}
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{
				"candidates": candidates,
				"count":      len(candidates),
			},
		})
		return
	}

	h.Service.EnqueueWikiSeedAttractions(r.Context(), space.ID, limit, false)
	errpkg.WriteJSON(w, http.StatusAccepted, map[string]any{
		"data": map[string]any{"queued": true},
	})
}
