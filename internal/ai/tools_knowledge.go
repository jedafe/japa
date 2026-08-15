// Package ai — tools_knowledge.go implements the knowledge tool handlers for
// the AI Agent Executor core tool catalog (Section 103, ADR-111).
//
// Tools implemented here (Phase 4, tasks 4.09-4.10):
//
//   - rag.search       — vector retrieval from content_embeddings (geo-filtered)
//   - external.search  — web search via Tavily/Brave API (configurable), cached
//
// Privacy: the RAG retrieval queries (SearchContentEmbeddings,
// SearchContentEmbeddingsGeoFiltered) do not filter ai_exempt at the SQL
// layer. The rag.search handler filters exempt content post-retrieval using
// the PrivacyExemptionChecker — chunks whose source content is ai_exempt
// are stripped from the result set before returning to the LLM. When
// deps.Privacy is nil, no filtering is applied. No PrivacyTargetFunc is
// registered because rag.search returns a result set, not a single entity.
//
// external.search hits an external API and caches results in
// jc:agent:external:{hash} (30-min TTL per the Valkey keys catalog); it
// never touches user data directly.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Tool Catalog Expansion — Knowledge tools)
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 4, tasks 4.09-4.10)
package ai

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/crypto"
	"github.com/jedafe/japa/internal/valkey"
)

// ---------------------------------------------------------------------------
// rag.search (task 4.09)
// ---------------------------------------------------------------------------

// ragSearchArgs is the LLM-produced argument shape for rag.search.
type ragSearchArgs struct {
	Query      string `json:"query"`
	Language   string `json:"language,omitempty"`
	GeoSpaceID string `json:"geo_space_id,omitempty"`
	Limit      int32  `json:"limit,omitempty"`
}

// RAGSearchHandler returns a ToolHandler that searches the knowledge base
// via vector retrieval. When geo_space_id is provided, retrieval is
// geo-filtered to content scoped to that space or its children (§103
// Geo-Filtered RAG amendment to §69).
//
// Privacy: the RAG retrieval queries do not filter ai_exempt at the SQL
// layer. The handler filters exempt content post-retrieval: chunks whose
// entity_type is "content" and whose entity_id resolves to an ai_exempt
// content row are stripped from the result set. When deps.Privacy is nil,
// no filtering is applied.
func RAGSearchHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.RAG == nil {
			return errorToolResult("rag_unavailable", "RAG service is not configured"), nil
		}

		var a ragSearchArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		if a.Query == "" {
			return errorToolResult("missing_query", "query is required"), nil
		}
		if a.Language == "" {
			a.Language = "en"
		}

		var geoSpaceID *pgtype.UUID
		if a.GeoSpaceID != "" {
			id, err := scanUUID(a.GeoSpaceID)
			if err != nil {
				return errorToolResult("invalid_geo_space_id", "geo_space_id is not a valid UUID"), nil
			}
			geoSpaceID = uuidPtr(id)
		}

		chunks, err := deps.RAG.Retrieve(ctx, tenantID, a.Query, a.Language, geoSpaceID)
		if err != nil {
			return errorToolResult("rag_search_failed", err.Error()), nil
		}

		// Filter out chunks whose source content is ai_exempt.
		chunks = filterExemptRAGChunks(ctx, deps.Privacy, tenantID, chunks)

		// Apply the limit if the RAG service returned more than requested.
		if a.Limit > 0 && int32(len(chunks)) > a.Limit { // #nosec G115 -- len() of a RAG result slice
			chunks = chunks[:a.Limit]
		}

		return successToolResult(map[string]any{
			"query":      a.Query,
			"language":   a.Language,
			"geo_scoped": geoSpaceID != nil,
			"results":    chunks,
			"total":      len(chunks),
		}), nil
	}
}

// RAGSearchPrivacyTarget is nil — rag.search returns a result set (not a
// single entity), so no PrivacyTargetFunc is registered. Exempt content is
// filtered post-retrieval by filterExemptRAGChunks.

// filterExemptRAGChunks strips chunks whose source content is ai_exempt.
// Only entity_type = "content" chunks are checked (other entity types have
// no ai_exempt column). When the privacy checker is nil, no filtering is
// applied.
func filterExemptRAGChunks(ctx context.Context, privacy PrivacyExemptionChecker, tenantID pgtype.UUID, chunks []RAGChunk) []RAGChunk {
	if privacy == nil {
		return chunks
	}
	out := make([]RAGChunk, 0, len(chunks))
	for _, ch := range chunks {
		if ch.EntityType != "content" {
			out = append(out, ch)
			continue
		}
		entityID, err := scanUUID(ch.EntityID)
		if err != nil {
			// Can't parse the entity ID — keep the chunk (don't penalize
			// malformed metadata by hiding legitimate content).
			out = append(out, ch)
			continue
		}
		exempt, err := privacy.IsExempt(ctx, PrivacyEntityContent, tenantID, entityID)
		if err == nil && exempt {
			continue
		}
		out = append(out, ch)
	}
	return out
}

// ---------------------------------------------------------------------------
// external.search (task 4.10)
// ---------------------------------------------------------------------------

// externalSearchArgs is the LLM-produced argument shape for external.search.
type externalSearchArgs struct {
	Query string `json:"query"`
	Limit int32  `json:"limit,omitempty"`
}

// externalSearchResult is a single web search result returned to the LLM.
type externalSearchResult struct {
	Title   string  `json:"title"`
	URL     string  `json:"url"`
	Content string  `json:"content"`
	Score   float64 `json:"score,omitempty"`
}

// externalSearchCacheTTL is the TTL for jc:agent:external:{hash} keys.
// Per the Valkey keys catalog: 30 minutes.
const externalSearchCacheTTL = 30 * time.Minute

// ExternalSearchHandler returns a ToolHandler that searches the web via
// Tavily or Brave (configurable via agent.external_search_provider). Results
// are cached in jc:agent:external:{hash} for 30 minutes per the Valkey keys
// catalog. The API key comes from agent.external_search_api_key (Rule 3 —
// no hardcoded keys).
//
// The handler requires the agent.external_search RBAC permission, which is
// checked by the executor's RBAC gate before dispatch. The handler itself
// only verifies the provider + key are configured.
func ExternalSearchHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil {
			return errorToolResult("external_search_unavailable", "tool deps not configured"), nil
		}
		if deps.Settings == nil {
			return errorToolResult("external_search_unavailable", "settings reader not configured"), nil
		}

		var a externalSearchArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		if a.Query == "" {
			return errorToolResult("missing_query", "query is required"), nil
		}
		if a.Limit <= 0 || a.Limit > 20 {
			a.Limit = 5
		}

		provider := deps.Settings.GetStringOr(ctx, tenantID, "agent.external_search_provider", "tavily")
		apiKey := decryptSettingIfNeeded(deps.Settings.GetStringOr(ctx, tenantID, "agent.external_search_api_key", ""))
		if apiKey == "" {
			return errorToolResult("external_search_unavailable", "external search API key is not configured"), nil
		}

		// Check cache (jc:agent:external:{hash}).
		cacheKey := externalSearchCacheKey(tenantID, a.Query, a.Limit)
		if deps.Cache != nil {
			if cached, err := deps.Cache.Get(ctx, cacheKey); err == nil && cached != "" {
				// Cache hit — return the cached JSON directly.
				return &ToolResult{Status: ToolCallStatusSuccess, Content: cached}, nil
			}
		}

		// Dispatch to the configured provider.
		var results []externalSearchResult
		var err error
		switch strings.ToLower(provider) {
		case "tavily":
			results, err = searchTavily(ctx, deps.httpClient(), apiKey, a.Query, int(a.Limit))
		case "brave":
			results, err = searchBrave(ctx, deps.httpClient(), apiKey, a.Query, int(a.Limit))
		default:
			return errorToolResult("unknown_provider", fmt.Sprintf("unsupported external search provider: %q", provider)), nil
		}
		if err != nil {
			slog.Warn("ai: external search failed", "provider", provider, "error", err)
			return errorToolResult("external_search_failed", err.Error()), nil
		}

		// Shape the response and cache it.
		resp := map[string]any{
			"query":    a.Query,
			"provider": provider,
			"results":  results,
			"total":    len(results),
			"cached":   false,
		}
		respJSON, _ := json.Marshal(resp)

		if deps.Cache != nil {
			if cerr := deps.Cache.Set(ctx, cacheKey, string(respJSON), int(externalSearchCacheTTL.Seconds())); cerr != nil {
				slog.Warn("ai: failed to cache external search results", "error", cerr)
			}
		}

		return &ToolResult{Status: ToolCallStatusSuccess, Content: string(respJSON)}, nil
	}
}

// externalSearchCacheKey builds jc:agent:external:{hash} where hash is the
// SHA-256 of the tenant_id + query + limit. Tenant-scoping prevents
// cross-tenant cache sharing of external search results (Rule 4, §4.2).
// Per the Valkey keys catalog (Rule 4).
func externalSearchCacheKey(tenantID pgtype.UUID, query string, limit int32) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%d", tenantID.String(), query, limit)))
	return valkey.MustKey("agent:external:%s", hex.EncodeToString(h[:16]))
}

// decryptSettingIfNeeded decrypts a system_settings value if it is encrypted
// (v1: prefix format). Plaintext values are returned as-is for backward
// compatibility. Used for secret settings like agent.external_search_api_key
// that should be stored encrypted at rest (§5.4, §103).
func decryptSettingIfNeeded(value string) string {
	if value == "" || !strings.HasPrefix(value, "v1:") {
		return value
	}
	plaintext, err := crypto.Decrypt(value)
	if err != nil {
		slog.Warn("ai: failed to decrypt setting, using raw value", "error", err)
		return value
	}
	return plaintext
}

// ---------------------------------------------------------------------------
// Tavily API client
// ---------------------------------------------------------------------------

// tavilyResponse is the response shape from the Tavily Search API.
type tavilyResponse struct {
	Results []struct {
		Title   string  `json:"title"`
		URL     string  `json:"url"`
		Content string  `json:"content"`
		Score   float64 `json:"score"`
	} `json:"results"`
}

// searchTavily calls the Tavily Search API and returns normalized results.
func searchTavily(ctx context.Context, client *http.Client, apiKey, query string, limit int) ([]externalSearchResult, error) {
	body, err := json.Marshal(map[string]any{
		"api_key":             apiKey,
		"query":               query,
		"max_results":         limit,
		"include_raw_content": false,
	})
	if err != nil {
		return nil, fmt.Errorf("encoding tavily request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.tavily.com/search", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("building tavily request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling tavily: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("tavily returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var tr tavilyResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, fmt.Errorf("decoding tavily response: %w", err)
	}

	out := make([]externalSearchResult, 0, len(tr.Results))
	for _, r := range tr.Results {
		out = append(out, externalSearchResult{
			Title:   r.Title,
			URL:     r.URL,
			Content: r.Content,
			Score:   r.Score,
		})
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Brave Search API client
// ---------------------------------------------------------------------------

// braveResponse is the response shape from the Brave Search API.
type braveResponse struct {
	Web struct {
		Results []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Description string `json:"description"`
		} `json:"results"`
	} `json:"web"`
}

// searchBrave calls the Brave Search API and returns normalized results.
func searchBrave(ctx context.Context, client *http.Client, apiKey, query string, limit int) ([]externalSearchResult, error) {
	u, err := url.Parse("https://api.search.brave.com/res/v1/web/search")
	if err != nil {
		return nil, fmt.Errorf("building brave url: %w", err)
	}
	q := u.Query()
	q.Set("q", query)
	q.Set("count", fmt.Sprintf("%d", limit))
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("building brave request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling brave: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("brave returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var br braveResponse
	if err := json.NewDecoder(resp.Body).Decode(&br); err != nil {
		return nil, fmt.Errorf("decoding brave response: %w", err)
	}

	out := make([]externalSearchResult, 0, len(br.Web.Results))
	for _, r := range br.Web.Results {
		out = append(out, externalSearchResult{
			Title:   r.Title,
			URL:     r.URL,
			Content: r.Description,
		})
	}
	return out, nil
}
