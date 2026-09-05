// Package ai — presearch.go implements the agent pre-search step
// (Section 103, ADR-111).
//
// Before LLM reasoning begins, the agent always performs a server-side
// pre-search: a content full-text search and a RAG vector retrieval against
// the user's prompt. The results are classified into one of three levels
// (full / partial / none) and injected into the LLM system prompt so the
// model follows the "search first, then act" behavior rule — it never
// creates duplicate content without checking what already exists.
//
// Privacy (§103 Privacy Exemption Columns): ai_exempt content is excluded
// from both searches. The content search results are filtered post-query
// via the PrivacyExemptionChecker (the SQL queries do not filter ai_exempt
// at the DB layer). RAG chunks whose source content is ai_exempt are
// stripped post-retrieval. When the privacy checker is nil, no filtering
// is applied — the wiring layer MUST provide a Privacy checker.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Pre-Search Behavior)
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 5, tasks 5.01-5.03)
package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/content/visibility"
)

// ---------------------------------------------------------------------------
// Result types
// ---------------------------------------------------------------------------

// PreSearchLevel is the classification of pre-search result coverage.
type PreSearchLevel string

const (
	// PreSearchFull means existing content covers the request (best RAG
	// similarity >= agent.presearch_full_threshold).
	PreSearchFull PreSearchLevel = "full"
	// PreSearchPartial means some results exist but none meet the full
	// threshold — gaps remain.
	PreSearchPartial PreSearchLevel = "partial"
	// PreSearchNone means no relevant content or RAG chunks were found.
	PreSearchNone PreSearchLevel = "none"
)

// PreSearchContentItem is a single content row in the pre-search result,
// shaped for injection into the LLM system prompt. Only non-exempt content
// appears here.
type PreSearchContentItem struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Slug        string `json:"slug"`
	ContentType string `json:"content_type"`
	GeoSpaceID  string `json:"geo_space_id,omitempty"`
}

// PreSearchResult is the output of PreSearch. It carries the classified
// level plus the raw results so the agent loop can build a system prompt
// and so the ai_agent_sessions.presearch_level column can be populated.
type PreSearchResult struct {
	Level       PreSearchLevel         `json:"level"`
	Content     []PreSearchContentItem `json:"content"`
	RAGChunks   []RAGChunk             `json:"rag_chunks"`
	BestScore   float64                `json:"best_score"`
	TotalResult int                    `json:"total_results"`
}

// HasResults reports whether any content or RAG results were found.
func (r PreSearchResult) HasResults() bool {
	return len(r.Content) > 0 || len(r.RAGChunks) > 0
}

// Summary builds a human-readable summary of the pre-search results for
// injection into the LLM system prompt. The summary is intentionally
// compact — the full result JSON is available to the agent via the
// PreSearchResult struct if the caller wants to include it verbatim.
func (r PreSearchResult) Summary() string {
	switch r.Level {
	case PreSearchFull:
		var parts []string

		parts = append(parts, fmt.Sprintf("FULL coverage (best similarity %.2f). %d content items and %d RAG chunks already cover this topic.",
			r.BestScore, len(r.Content), len(r.RAGChunks)))
		if len(r.Content) > 0 {
			parts = append(parts, "Existing content:")

			for i, c := range r.Content {
				if i >= 5 {
					parts = append(parts, fmt.Sprintf("  ... and %d more", len(r.Content)-5))
					break
				}

				parts = append(parts, fmt.Sprintf("  - [%s] %s (slug: %s)", c.ContentType, c.Title, c.Slug))
			}
		}

		return strings.Join(parts, "\n")

	case PreSearchPartial:
		return fmt.Sprintf("PARTIAL coverage. %d content items and %d RAG chunks found, but none meet the full-coverage threshold (%.2f best). Gaps remain — identify what's missing and fill it.",
			len(r.Content), len(r.RAGChunks), r.BestScore)

	default:
		return "NONE — no existing content or knowledge base chunks cover this topic. You may create new content if authorized."
	}
}

// ---------------------------------------------------------------------------
// PreSearcher
// ---------------------------------------------------------------------------

// preSearchLimit is the max number of results to fetch from each source.
// The pre-search is a coverage check, not a full result set — a small
// sample is enough to classify the level.
const preSearchLimit = 5

// PreSearcher runs the pre-search step. It depends on the same service
// interfaces the Phase 4 tool handlers use (ContentService, RAGService,
// PrivacyExemptionChecker, SettingsReader) so it can be wired from the
// same ToolDeps and tested with the same stubs.
type PreSearcher struct {
	content  ContentService
	rag      RAGService
	privacy  PrivacyExemptionChecker
	settings SettingsReader
}

// NewPreSearcher creates a PreSearcher from a ToolDeps container. Any nil
// dependency degrades gracefully: a nil ContentService or RAGService skips
// that search entirely (the result is based on whichever source is
// available); a nil Privacy checker skips exempt-content filtering; a nil
// Settings reader falls back to the default threshold.
func NewPreSearcher(deps *ToolDeps) *PreSearcher {
	if deps == nil {
		return &PreSearcher{}
	}

	return &PreSearcher{
		content:  deps.Content,
		rag:      deps.RAG,
		privacy:  deps.Privacy,
		settings: deps.Settings,
	}
}

// PreSearch runs the content search and RAG retrieval against the prompt,
// excludes ai_exempt content, and classifies the result level.
//
// geoSpaceID is optional — when nil, both searches run tenant-wide. When
// provided, the content search is scoped to the space and RAG retrieval is
// geo-filtered (§103 Geo-Filtered RAG amendment to §69).
func (p *PreSearcher) PreSearch(ctx context.Context, tenantID pgtype.UUID, prompt string, geoSpaceID *pgtype.UUID) (*PreSearchResult, error) {
	if p == nil {
		return &PreSearchResult{Level: PreSearchNone}, nil
	}

	if prompt == "" {
		return &PreSearchResult{Level: PreSearchNone}, errors.New("ai: pre-search requires a non-empty prompt")
	}

	result := &PreSearchResult{Level: PreSearchNone}

	// --- Content search (§13 / §54 full-text) ---
	if p.content != nil {
		items := p.searchContent(ctx, tenantID, prompt, geoSpaceID)
		result.Content = items
	}

	// --- RAG retrieval (§69, geo-filtered) ---
	if p.rag != nil {
		chunks, err := p.rag.Retrieve(ctx, tenantID, prompt, "", geoSpaceID)
		if err != nil {
			// RAG failure is non-fatal — pre-search can proceed with
			// content results alone. Log and continue.
			slog.Warn("ai: pre-search RAG retrieval failed", "error", err)
		} else {
			result.RAGChunks = filterExemptRAGChunks(ctx, p.privacy, tenantID, chunks)
		}
	}

	// --- Classification ---
	result.TotalResult = len(result.Content) + len(result.RAGChunks)
	result.BestScore = bestRAGSimilarity(result.RAGChunks)

	threshold := p.fullThreshold(ctx, tenantID)
	result.Level = classifyPreSearch(result, threshold)

	return result, nil
}

// searchContent runs the content search and filters exempt rows. When
// geoSpaceID is provided the search is space-scoped; otherwise it is
// tenant-wide.
func (p *PreSearcher) searchContent(ctx context.Context, tenantID pgtype.UUID, prompt string, geoSpaceID *pgtype.UUID) []PreSearchContentItem {
	filter := visibility.InternalVisibilityFilter(pgtype.UUID{})
	if geoSpaceID != nil && geoSpaceID.Valid {
		rows, err := p.content.SearchContentsBySpace(ctx, tenantID, *geoSpaceID, prompt, preSearchLimit, 0, filter)
		if err != nil {
			slog.Warn("ai: pre-search content search by space failed", "error", err)
			return nil
		}

		filtered := filterExemptContentFromSpace(ctx, p.privacy, tenantID, rows)

		return toPreSearchItems(contentSearchResultsFromSpace(filtered))
	}

	rows, err := p.content.ListContents(ctx, tenantID, prompt, pgtype.Timestamptz{}, preSearchLimit, filter)
	if err != nil {
		slog.Warn("ai: pre-search content search failed", "error", err)
		return nil
	}

	filtered := filterExemptContentFromTenant(ctx, p.privacy, tenantID, rows)

	return toPreSearchItems(contentSearchResultsFromTenant(filtered))
}

// toPreSearchItems converts the content.search result shape to the
// PreSearchContentItem shape used in pre-search results.
func toPreSearchItems(items []contentSearchResult) []PreSearchContentItem {
	out := make([]PreSearchContentItem, 0, len(items))
	for _, c := range items {
		out = append(out, PreSearchContentItem{
			ID:          c.ID,
			Title:       c.Title,
			Slug:        c.Slug,
			ContentType: c.ContentType,
			GeoSpaceID:  c.GeoSpaceID,
		})
	}

	return out
}

// fullThreshold reads agent.presearch_full_threshold from system_settings,
// falling back to 0.85 (§103 default) when the settings reader is nil or
// the key is absent.
func (p *PreSearcher) fullThreshold(ctx context.Context, tenantID pgtype.UUID) float64 {
	if p.settings == nil {
		return 0.85
	}

	return p.settings.GetFloatOr(ctx, tenantID, "agent.presearch_full_threshold", 0.85)
}

// bestRAGSimilarity returns the highest similarity score across the RAG
// chunks, or 0 when there are none.
func bestRAGSimilarity(chunks []RAGChunk) float64 {
	var best float64
	for _, c := range chunks {
		if c.Similarity > best {
			best = c.Similarity
		}
	}

	return best
}

// classifyPreSearch applies the §103 classification rules:
//   - FULL  — best RAG similarity >= threshold (existing content covers it)
//   - PARTIAL — some results exist but none meet the threshold
//   - NONE  — no results at all
func classifyPreSearch(r *PreSearchResult, threshold float64) PreSearchLevel {
	if !r.HasResults() {
		return PreSearchNone
	}

	if r.BestScore >= threshold {
		return PreSearchFull
	}

	return PreSearchPartial
}

// ---------------------------------------------------------------------------
// JSON helper for the agent loop
// ---------------------------------------------------------------------------

// preSearchJSON marshals the pre-search result for injection into the LLM
// system prompt as a JSON block. Errors are impossible (all fields are
// plain types) so the error is ignored.
func preSearchJSON(r *PreSearchResult) string {
	b, _ := json.Marshal(r)
	return string(b)
}
