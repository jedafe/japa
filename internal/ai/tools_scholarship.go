// Package ai — tools_scholarship.go implements the scholarship.search agent
// tool for the on-demand intelligence-to-content pipeline.
//
// The tool queries the scholarships projection table directly and returns
// a flat result list with record IDs, source links, and the linked content
// identity so the agent can decide which records to refresh or regenerate.
//
// Ref: docs/superpowers/specs/2026-08-19-on-demand-intel-to-content.md
// Ref: japaconnect-engineering/sections/73-scholarship-finder.md
package ai

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

type scholarshipSearchArgs struct {
	Country           string `json:"country,omitempty"`
	EligibleCountries string `json:"eligible_countries,omitempty"`
	DeadlineAfter     string `json:"deadline_after,omitempty"`
	FundingType       string `json:"funding_type,omitempty"`
	Level             string `json:"level,omitempty"`
	Limit             int32  `json:"limit,omitempty"`
}

// ScholarshipSearchHandler returns a ToolHandler that searches the
// scholarships projection table. The result set is the same flat shape used
// by the Scholarship Finder surface (Section 73) so the agent can reuse
// existing filtering and ranking.
func ScholarshipSearchHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.ScholarshipSearcher == nil {
			return errorToolResult("scholarship_search_unavailable", "scholarship search is not configured"), nil
		}

		var a scholarshipSearchArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		if a.Limit <= 0 || a.Limit > 50 {
			a.Limit = 20
		}

		filters := ScholarshipSearchFilters{
			Country:           a.Country,
			EligibleCountries: a.EligibleCountries,
			DeadlineFrom:      a.DeadlineAfter,
			FundingType:       a.FundingType,
			Level:             a.Level,
			Limit:             int(a.Limit),
			Page:              1,
		}

		results, _, err := deps.ScholarshipSearcher.FindScholarships(ctx, tenantID, filters)
		if err != nil {
			return errorToolResult("scholarship_search_failed", err.Error()), nil
		}

		// Expose the projection record IDs alongside the editorial IDs so the
		// agent can call record.update_now or content.regenerate_from_source.
		out := make([]map[string]any, 0, len(results))
		for _, r := range results {
			out = append(out, map[string]any{
				"id":                   r.ID,
				"guide_id":             r.GuideID,
				"intel_record_id":      r.IntelRecordID,
				"title":                r.Title,
				"slug":                 r.Slug,
				"country":              r.Country,
				"level_of_study":       r.LevelOfStudy,
				"award_amount_usd":     r.AwardAmountUSD,
				"application_deadline": r.ApplicationDeadline,
				"application_url":      r.ApplicationURL,
				"institution":          r.Institution,
				"eligible_countries":   r.EligibleCountries,
			})
		}

		return successToolResult(map[string]any{
			"query": map[string]any{
				"country":            a.Country,
				"eligible_countries": a.EligibleCountries,
				"deadline_after":     a.DeadlineAfter,
				"funding_type":       a.FundingType,
				"level":              a.Level,
				"limit":              a.Limit,
			},
			"results": out,
			"total":   len(out),
		}), nil
	}
}
