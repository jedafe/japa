package publicapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/sync/errgroup"

	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/scholarships"
	"github.com/jedafe/japa/internal/valkey"
	"github.com/jedafe/japa/internal/visaroutes"
)

// briefSections enumerates the composable brief sections (the `include`
// query param values).
var briefSections = []string{"visa", "timelines", "universities", "scholarships", "scam_alerts"}

// DestinationBrief is the aggregate document returned by
// GET /public/destinations/{slug}/brief.
// Ref: docs/superpowers/specs/2026-09-23-agent-data-api-design.md
type DestinationBrief struct {
	Destination         gen.GetPublicSpaceBySlugRow `json:"destination"`
	Summary             string                      `json:"summary"`
	Sections            map[string]any              `json:"sections"`
	SectionsUnavailable []string                    `json:"sections_unavailable"`
	GeneratedAt         time.Time                   `json:"generated_at"`
}

// GetDestinationBrief resolves the destination space then composes the
// requested sections in parallel. A section whose reader is disabled or
// errors is omitted and named in SectionsUnavailable — the brief is
// best-effort, only an unresolvable destination is a hard error.
func (s *Service) GetDestinationBrief(ctx context.Context, slug string, include []string) (*DestinationBrief, error) {
	if s.moat == nil || s.moat.Spaces == nil {
		return nil, ErrReaderUnconfigured
	}

	if len(include) == 0 {
		include = briefSections
	}
	include = dedupeStrings(include)

	cacheKey := valkey.MustKey("api_dest_brief:%s:%s", slug, briefIncludeHash(include))
	if s.cache != nil {
		if raw, err := s.cache.Get(ctx, cacheKey).Bytes(); err == nil {
			var cached DestinationBrief
			if json.Unmarshal(raw, &cached) == nil {
				return &cached, nil
			}
		}
	}

	// GetPublicSpaceBySlug enforces visibility='public' AND status='active'
	// AND deleted_at IS NULL in SQL — no app-level guard needed.
	dest, err := s.moat.Spaces.GetPublicSpaceBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrDestinationNotFound
		}
		return nil, err
	}

	brief := &DestinationBrief{
		Destination: dest,
		Sections:    map[string]any{},
		GeneratedAt: time.Now().UTC(),
	}

	var mu sync.Mutex
	g, gctx := errgroup.WithContext(ctx)

	wants := func(name string) bool {
		for _, i := range include {
			if i == name {
				return true
			}
		}
		return false
	}
	fail := func(name string) {
		mu.Lock()
		brief.SectionsUnavailable = append(brief.SectionsUnavailable, name)
		mu.Unlock()
	}
	put := func(name string, v any) {
		mu.Lock()
		brief.Sections[name] = v
		mu.Unlock()
	}

	if wants("visa") {
		g.Go(func() error {
			if s.moat.VisaRoutes == nil || !s.moat.VisaRoutes.Enabled(gctx) {
				fail("visa")
				return nil
			}
			routes, _, err := s.moat.VisaRoutes.List(gctx, dest.ID, "", "", 0, 1, 10)
			if err != nil {
				fail("visa")
				return nil
			}
			put("visa", map[string]any{"routes": routes})
			return nil
		})
	}
	if wants("timelines") {
		g.Go(func() error {
			if s.moat.VisaTimeline == nil || !s.moat.VisaTimeline.Enabled(gctx) {
				fail("timelines")
				return nil
			}
			stats, err := s.moat.VisaTimeline.GetStats(gctx, pgtype.UUID{}, dest.ID, "", "", 0)
			if err != nil {
				fail("timelines")
				return nil
			}
			put("timelines", map[string]any{"stats": stats})
			return nil
		})
	}
	if wants("universities") {
		g.Go(func() error {
			country := ""
			if dest.GeoCode != nil {
				country = *dest.GeoCode
			}
			items, total, err := s.ListUniversities(gctx, "", country, "", 10, 0)
			if err != nil {
				fail("universities")
				return nil
			}
			put("universities", map[string]any{"items": items, "total": total})
			return nil
		})
	}
	if wants("scholarships") {
		g.Go(func() error {
			if s.moat.Scholarships == nil || !s.moat.Scholarships.Enabled(gctx) {
				fail("scholarships")
				return nil
			}
			country := ""
			if dest.GeoCode != nil {
				country = *dest.GeoCode
			}
			results, total, err := s.moat.Scholarships.FindScholarships(gctx,
				scholarships.ScholarshipFilters{Country: country, Limit: 10, Page: 1})
			if err != nil {
				fail("scholarships")
				return nil
			}
			put("scholarships", map[string]any{"items": results, "total": total})
			return nil
		})
	}
	if wants("scam_alerts") {
		g.Go(func() error {
			if s.moat.ScamAlerts == nil || !s.moat.ScamAlerts.Enabled(gctx) {
				fail("scam_alerts")
				return nil
			}
			alerts, _, err := s.moat.ScamAlerts.List(gctx, dest.ID, "", 1, 5)
			if err != nil {
				fail("scam_alerts")
				return nil
			}
			put("scam_alerts", map[string]any{"items": alerts})
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err // unreachable — goroutines never return non-nil
	}

	sort.Strings(brief.SectionsUnavailable)
	brief.Summary = renderBriefSummary(dest, brief)

	if s.cache != nil {
		ttl := 900
		if s.settingsReader != nil {
			// Rule 3: TTL from system_settings (cataloged key, fallback 900).
			ttl = s.settingsReader.GetIntOr(ctx, "api.destination_brief.cache_ttl_seconds", 900)
		}
		if raw, err := json.Marshal(brief); err == nil {
			_ = s.cache.Set(ctx, cacheKey, raw, time.Duration(ttl)*time.Second).Err()
		}
	}

	return brief, nil
}

// renderBriefSummary produces a deterministic markdown overview — no LLM.
func renderBriefSummary(dest gen.GetPublicSpaceBySlugRow, b *DestinationBrief) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# %s — destination brief\n\n", dest.Name)
	if v, ok := b.Sections["visa"].(map[string]any); ok {
		fmt.Fprintf(&sb, "- **Visa routes:** %d top routes included\n", len(v["routes"].([]visaroutes.RouteDTO)))
	}
	if len(b.SectionsUnavailable) > 0 {
		fmt.Fprintf(&sb, "- _Sections unavailable: %s_\n", strings.Join(b.SectionsUnavailable, ", "))
	}
	return sb.String()
}

// briefIncludeHash hashes the sorted include list so the Valkey key is
// stable regardless of query-param ordering (jc:api_dest_brief:{slug}:{hash}).
func briefIncludeHash(include []string) string {
	sorted := append([]string(nil), include...)
	sort.Strings(sorted)
	sum := sha256.Sum256([]byte(strings.Join(sorted, ",")))
	return hex.EncodeToString(sum[:])[:12]
}

func dedupeStrings(in []string) []string {
	seen := map[string]bool{}
	out := in[:0]
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// GetDestinationBrief handles GET /public/destinations/{slug}/brief.
func (h *Handlers) GetDestinationBrief(w http.ResponseWriter, r *http.Request) {
	if !requireAPIKey(w, r) {
		return
	}
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Slug is required")
		return
	}
	var include []string
	if v := r.URL.Query().Get("include"); v != "" {
		include = strings.Split(v, ",")
	}
	brief, err := h.Service.GetDestinationBrief(r.Context(), slug, include)
	if err != nil {
		if errors.Is(err, ErrDestinationNotFound) {
			errpkg.WriteError(w, http.StatusNotFound, "destination_not_found", "Destination not found")
			return
		}
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": brief})
}
