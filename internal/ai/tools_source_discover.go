// Package ai — tools_source_discover.go implements the source.discover agent
// tool for the on-demand intelligence-to-content pipeline.
//
// The tool runs an external search, evaluates the result URLs, and creates
// new intel_sources rows with status='proposed' and enabled=false. If the
// agent.auto_enable_discovered_sources setting is true and the candidate
// passes a trust heuristic, the source may be created as active instead.
//
// Ref: docs/superpowers/specs/2026-08-19-on-demand-intel-to-content.md
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md
package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

type sourceDiscoverArgs struct {
	Query    string `json:"query"`
	Category string `json:"category,omitempty"`
	Country  string `json:"country,omitempty"`
	Limit    int32  `json:"limit,omitempty"`
}

// sourceDiscoverCandidate is the shape returned for each discovered source.
type sourceDiscoverCandidate struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Domain    string `json:"domain"`
	SourceURL string `json:"source_url"`
	Category  string `json:"category"`
	Status    string `json:"status"`
	Enabled   bool   `json:"enabled"`
	Existing  bool   `json:"existing"`
}

// SourceDiscoverHandler returns a ToolHandler that searches the web for
// candidate sources, evaluates them, and persists proposed sources for
// admin review.
func SourceDiscoverHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil {
			return errorToolResult("source_discover_unavailable", "tool deps not configured"), nil
		}

		if deps.IntelSourceManager == nil {
			return errorToolResult("source_manager_unavailable", "intelligence source manager is not configured"), nil
		}

		if deps.Settings == nil {
			return errorToolResult("settings_unavailable", "settings reader is not configured"), nil
		}

		var a sourceDiscoverArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		if a.Query == "" {
			return errorToolResult("missing_query", "query is required"), nil
		}

		if a.Limit <= 0 || a.Limit > 20 {
			a.Limit = 5
		}

		// Use the same external search provider as external.search (or the
		// injected test/override search function).
		var results []WebSearchResult

		var err error
		if deps.SearchWebFunc != nil {
			_, results, err = deps.SearchWebFunc(ctx, deps.httpClient(), deps.Settings, tenantID, a.Query, int(a.Limit))
		} else {
			_, results, err = SearchWeb(ctx, deps.httpClient(), deps.Settings, tenantID, a.Query, int(a.Limit))
		}

		if err != nil {
			slog.Warn("ai: source discover external search failed", "error", err)
			return errorToolResult("external_search_failed", err.Error()), nil
		}

		autoEnable := false
		if deps.Settings != nil {
			autoEnable = deps.Settings.GetBoolOr(ctx, tenantID, "agent.auto_enable_discovered_sources", false)
		}

		var proposed, existing []sourceDiscoverCandidate

		now := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}

		for _, r := range results {
			if r.URL == "" {
				continue
			}

			u, perr := url.Parse(r.URL)
			if perr != nil || u.Host == "" {
				continue
			}

			domain := u.Host
			if strings.HasPrefix(domain, "www.") {
				domain = strings.TrimPrefix(domain, "www.")
			}

			// Deduplicate against existing sources by source_url.
			if src, gerr := deps.IntelSourceManager.GetByURL(ctx, tenantID, r.URL); gerr == nil && src.ID.Valid {
				existing = append(existing, sourceDiscoverCandidate{
					ID:        src.ID.String(),
					Name:      src.Name,
					Domain:    src.Domain,
					SourceURL: src.SourceUrl,
					Category:  src.Category,
					Status:    src.Status,
					Enabled:   src.Enabled,
					Existing:  true,
				})

				continue
			}

			// Simple trust heuristic: HTTPS and non-empty title.
			trustLevel := int16(50)
			if u.Scheme == "https" {
				trustLevel += 10
			}

			if r.Title != "" {
				trustLevel += 10
			}

			category := a.Category
			if category == "" {
				category = "general"
			}

			name := r.Title
			if name == "" {
				name = r.URL
			}

			status := "proposed"
			enabled := false

			if autoEnable && trustLevel >= 80 {
				status = "active"
				enabled = true
			}

			src, cerr := deps.IntelSourceManager.Create(ctx, tenantID, gen.CreateIntelSourceParams{
				TenantID:            tenantID,
				Name:                name,
				Domain:              domain,
				SourceUrl:           r.URL,
				Category:            category,
				SourceType:          "web_page",
				TrustLevel:          trustLevel,
				CrawlMethod:         "http",
				CrawlFrequency:      "daily",
				CrawlFrequencyMins:  1440,
				RobotsPolicy:        nil,
				SourceLicense:       nil,
				AttributionRequired: true,
				UsageRestriction:    nil,
				ProviderConfig:      nil,
				Enabled:             enabled,
				Status:              status,
				ProposedBy:          userID,
				ProposedAt:          now,
				ApprovedBy:          pgtype.UUID{},
				ApprovedAt:          pgtype.Timestamptz{},
				PersonaID:           pgtype.UUID{},
			})
			if cerr != nil {
				slog.Warn("ai: source discover create failed", "url", r.URL, "error", cerr)
				continue
			}

			proposed = append(proposed, sourceDiscoverCandidate{
				ID:        src.ID.String(),
				Name:      src.Name,
				Domain:    src.Domain,
				SourceURL: src.SourceUrl,
				Category:  src.Category,
				Status:    src.Status,
				Enabled:   src.Enabled,
				Existing:  false,
			})
		}

		return successToolResult(map[string]any{
			"query":          a.Query,
			"category":       a.Category,
			"country":        a.Country,
			"proposed":       proposed,
			"existing":       existing,
			"total_proposed": len(proposed),
			"total_existing": len(existing),
		}), nil
	}
}
