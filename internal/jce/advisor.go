// Ref: docs/superpowers/specs/2026-08-30-jce-subspec-4-design.md §10
package jce

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/comparison"
	errpkg "github.com/jedafe/japa/internal/errors"
)

const (
	advisorFeatureFlag      = "feature.jce.ai_advisor"
	advisorUsePerm          = "jce.ai_advisor.use"
	advisorDefaultModel     = "openai/gpt-4o-mini"
	advisorDefaultMaxTokens = 1024
)

// defaultAdvisorRecommendations is the maximum number of recommendations to
// include in an advisor prompt. It is a display/prompt-size limit, not a
// business rule, so a compile-time constant is appropriate.
const defaultAdvisorRecommendations = 10

// advisorCacheKey returns the canonical Valkey cache key for an advisor result.
func advisorCacheKey(tenantID, contentID, profileID pgtype.UUID, seed string) string {
	pid := "none"
	if profileID.Valid {
		pid = profileID.String()
	}
	h := sha256.Sum256([]byte(seed))
	return fmt.Sprintf("jc:jce:advisor:%s:%s:%s:%x", tenantID.String(), contentID.String(), pid, h)
}

// advisorCacheSeed builds a deterministic seed for the cache key that depends
// on the advisor operation and the request input. The question hash from the
// design is derived from this seed.
func advisorCacheSeed(kind string, input AdvisorInput) string {
	var sb strings.Builder
	sb.WriteString(kind)
	sb.WriteByte('|')
	sb.WriteString(input.Question)
	if input.ItemID != nil && input.ItemID.Valid {
		sb.WriteByte('|')
		sb.WriteString(input.ItemID.String())
	}
	sb.WriteByte('|')
	sb.WriteString(input.FactorKey)
	return sb.String()
}

// ptrToUUID returns the value pointed to by p, or an invalid UUID when p is nil.
func ptrToUUID(p *pgtype.UUID) pgtype.UUID {
	if p == nil {
		return pgtype.UUID{}
	}
	return *p
}

// buildAdvisorPrompt constructs the user prompt and the structured context
// string for the AI advisor. The context is authoritative: it contains the
// scoring engine's ranking, recommendations, and freshness summary. The LLM
// must only synthesise natural language from this data and must never invent
// its own rankings or scores.
func (s *Service) buildAdvisorPrompt(
	ctx context.Context,
	kind string,
	input AdvisorInput,
	matrix []comparison.ComparisonMatrixItem,
	scoreResult ScoreResult,
	recommendations []ComparisonRecommendation,
	freshness FreshnessSummary,
	recCount int,
) (string, string) {
	title := input.ContentID.String()
	if cmp, err := s.compSvc.GetJCEComparison(ctx, input.ContentID.String(), input.TenantID); err == nil {
		title = cmp.Content.Title
	}

	ranking := rankingFromScoreResult(scoreResult.Scores)
	recs := recommendations
	if recCount <= 0 {
		recCount = defaultAdvisorRecommendations
	}
	if len(recs) > recCount {
		recs = recs[:recCount]
	}

	profileID := "none"
	if input.ProfileID != nil && input.ProfileID.Valid {
		profileID = input.ProfileID.String()
	}

	focusItemID := ""
	if input.ItemID != nil && input.ItemID.Valid {
		focusItemID = input.ItemID.String()
	}

	ctxData := map[string]any{
		"title":             title,
		"content_id":        input.ContentID.String(),
		"profile_id":        profileID,
		"question":          input.Question,
		"ranking":           ranking,
		"recommendations":   recs,
		"freshness_score":   freshness.OverallFreshnessScore,
		"freshness_summary": freshness,
		"matrix":            matrix,
	}

	if focusItemID != "" {
		ctxData["focus_item_id"] = focusItemID
		ctxData["focus_factor_key"] = input.FactorKey
		for _, score := range scoreResult.Scores {
			if score.ItemID == focusItemID {
				ctxData["focus_item"] = score
				break
			}
		}
	}

	ctxJSON, _ := json.MarshalIndent(ctxData, "", "  ")

	var userPrompt string
	switch kind {
	case "analyze":
		userPrompt = fmt.Sprintf("Analyze the comparison and answer this question: %s", input.Question)
	case "rank":
		userPrompt = "Rank the comparison items from best to worst using the authoritative data. Provide concise reasoning for each item."
	case "explain":
		if input.Question != "" {
			userPrompt = fmt.Sprintf("Explain the %s factor for item %s. %s", input.FactorKey, focusItemID, input.Question)
		} else {
			userPrompt = fmt.Sprintf("Explain the %s factor for item %s.", input.FactorKey, focusItemID)
		}
	case "recommend":
		userPrompt = "Recommend the top next actions for this comparison and profile using the authoritative data."
	default:
		userPrompt = input.Question
	}

	instructions := `You are the Japa Comparison Engine AI advisor.
You must only use the authoritative data below.
Do not invent rankings, scores, recommendations, or facts.
The ranking and recommendations in the data are authoritative and must not be changed.
Synthesise a helpful natural-language response.
When you return JSON, use the fields: summary (string), citations (array of strings).`

	context := instructions + "\n\n" + string(ctxJSON)
	return userPrompt, context
}

// rankingFromScoreResult sorts the engine scores into an authoritative ranking.
func rankingFromScoreResult(scores []JapaScore) []AdvisorRankItem {
	sorted := make([]JapaScore, len(scores))
	copy(sorted, scores)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].JapaScore != sorted[j].JapaScore {
			return sorted[i].JapaScore > sorted[j].JapaScore
		}
		return sorted[i].ItemTitle < sorted[j].ItemTitle
	})

	items := make([]AdvisorRankItem, len(sorted))
	for i, s := range sorted {
		eligible := s.EligibleStatus != "ineligible"
		reasoning := s.Explanation
		if reasoning == "" {
			reasoning = fmt.Sprintf("JapaScore: %.1f", s.JapaScore)
		}
		items[i] = AdvisorRankItem{
			ItemID:    s.ItemID,
			ItemTitle: s.ItemTitle,
			Rank:      i + 1,
			JapaScore: s.JapaScore,
			Reasoning: reasoning,
			Eligible:  eligible,
		}
	}
	return items
}

// nextActionsFromRecommendations maps stored recommendations into NextAction
// values for the advisor result.
func nextActionsFromRecommendations(recs []ComparisonRecommendation) []NextAction {
	out := make([]NextAction, 0, len(recs))
	for _, r := range recs {
		var itemID, actionType, actionURL string
		if r.ItemID != nil {
			itemID = *r.ItemID
		}
		if r.ActionType != nil {
			actionType = *r.ActionType
		}
		if r.ActionURL != nil {
			actionURL = *r.ActionURL
		}
		out = append(out, NextAction{
			ItemID:     itemID,
			Text:       r.Text,
			ActionType: actionType,
			ActionURL:  actionURL,
			Priority:   r.Priority,
		})
	}
	return out
}

// advisorFromResponse merges the raw LLM text with the authoritative engine
// data. The ranking and next actions always come from the engine, never the
// model, so the model cannot hallucinate its own rankings.
func advisorFromResponse(
	raw string,
	contentID string,
	profileID *string,
	ranking []AdvisorRankItem,
	nextActions []NextAction,
	freshnessScore int,
) AdvisorResult {
	var parsed struct {
		Summary      string   `json:"summary"`
		Explanation  string   `json:"explanation"`
		Citations    []string `json:"citations"`
		ProviderName string   `json:"provider_name"`
		ProviderRef  string   `json:"provider_ref"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
		summary := parsed.Summary
		if summary == "" {
			summary = parsed.Explanation
		}
		return AdvisorResult{
			ContentID:      contentID,
			ProfileID:      profileID,
			Summary:        summary,
			Ranking:        ranking,
			NextActions:    nextActions,
			Citations:      parsed.Citations,
			ProviderName:   parsed.ProviderName,
			ProviderRef:    parsed.ProviderRef,
			FreshnessScore: freshnessScore,
		}
	}

	return AdvisorResult{
		ContentID:      contentID,
		ProfileID:      profileID,
		Summary:        strings.TrimSpace(raw),
		Ranking:        ranking,
		NextActions:    nextActions,
		FreshnessScore: freshnessScore,
	}
}

// runAdvisor is the common implementation for all advisor operations. It
// enforces the feature flag and permission, loads the scoring engine data,
// builds the prompt, calls the dispatcher, and returns a result whose ranking
// and next actions are derived from the engine.
func (s *Service) runAdvisor(ctx context.Context, kind string, input AdvisorInput) (AdvisorResult, error) {
	if !s.featureEnabled(ctx, input.TenantID, advisorFeatureFlag) {
		return AdvisorResult{}, fmt.Errorf("%w: %s disabled", errpkg.ErrFeatureDisabled, advisorFeatureFlag)
	}
	if !input.UserID.Valid {
		return AdvisorResult{}, fmt.Errorf("%w: authentication required", errpkg.ErrForbidden)
	}
	if !s.permissionChecker(ctx, input.UserID, input.TenantID)(advisorUsePerm) {
		return AdvisorResult{}, fmt.Errorf("%w: missing %s", errpkg.ErrForbidden, advisorUsePerm)
	}
	if s.dispatcher == nil {
		return AdvisorResult{}, fmt.Errorf("%w: AI dispatcher not configured", errpkg.ErrNotImplemented)
	}

	cmp, err := s.compSvc.GetJCEComparison(ctx, input.ContentID.String(), input.TenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AdvisorResult{}, fmt.Errorf("%w: comparison not found", errpkg.ErrNotFound)
		}
		return AdvisorResult{}, fmt.Errorf("loading comparison: %w", err)
	}

	check := s.permissionChecker(ctx, input.UserID, input.TenantID)
	if !s.canAccessComparison(cmp, input.UserID, check, advisorUsePerm) {
		return AdvisorResult{}, fmt.Errorf("%w: cannot access comparison", errpkg.ErrForbidden)
	}

	cacheSeed := advisorCacheSeed(kind, input)
	cacheKey := advisorCacheKey(input.TenantID, input.ContentID, ptrToUUID(input.ProfileID), cacheSeed)
	if s.vk != nil {
		data, err := s.vk.Get(ctx, cacheKey).Bytes()
		if err == nil && len(data) > 0 {
			var cached AdvisorResult
			if err := json.Unmarshal(data, &cached); err == nil {
				return cached, nil
			}
		}
	}

	// Load the operational settings for the advisor. The model and max_tokens
	// values are loaded here even though the current dispatcher only accepts a
	// single prompt string, so they are available to future dispatcher variants
	// and documented in the settings catalog.
	var model string
	maxTokens := advisorDefaultMaxTokens
	recCount := defaultAdvisorRecommendations
	if s.settings != nil {
		model = s.settings.GetStringOr(ctx, input.TenantID, "jce.advisor_model", advisorDefaultModel)
		maxTokens = s.settings.GetIntOr(ctx, input.TenantID, "jce.advisor_max_tokens", advisorDefaultMaxTokens)
		recCount = s.settings.GetIntOr(ctx, input.TenantID, "jce.advisor_recommendation_count", defaultAdvisorRecommendations)
	}
	_ = model
	_ = maxTokens

	var scoreResult ScoreResult
	scoreResult, err = s.GetComparisonScore(ctx, input.ContentID, input.TenantID, input.UserID, input.ProfileID)
	if err != nil {
		if errors.Is(err, errpkg.ErrNotFound) {
			scoreResult, err = s.ScoreComparison(ctx, input.ContentID, input.TenantID, input.UserID, input.ProfileID, "", false)
		}
	}
	if err != nil {
		return AdvisorResult{}, fmt.Errorf("loading score: %w", err)
	}

	var recommendations []ComparisonRecommendation
	if kind == "recommend" || kind == "analyze" {
		recommendations, err = s.ListRecommendations(ctx, input.ContentID, input.TenantID, input.UserID, input.ProfileID, recCount)
		if err != nil {
			// Fall back to the recommendations embedded in the score result if
			// the recommendation store cannot be queried.
			recommendations = scoreResult.Recommendations
		}
	} else {
		recommendations = scoreResult.Recommendations
	}

	matrix, err := s.compSvc.GetJCEComparisonMatrix(ctx, input.ContentID, input.TenantID)
	if err != nil {
		return AdvisorResult{}, fmt.Errorf("loading comparison matrix: %w", err)
	}

	freshness, err := s.FreshnessForComparison(ctx, input.ContentID, input.TenantID, input.UserID)
	if err != nil {
		return AdvisorResult{}, fmt.Errorf("loading freshness: %w", err)
	}

	userPrompt, dataContext := s.buildAdvisorPrompt(ctx, kind, input, matrix, scoreResult, recommendations, freshness, recCount)
	fullPrompt := dataContext + "\n\n" + userPrompt

	raw, err := s.dispatcher.Generate(ctx, "chat", fullPrompt)
	if err != nil {
		return AdvisorResult{}, fmt.Errorf("advisor generation failed: %w", err)
	}

	ranking := rankingFromScoreResult(scoreResult.Scores)
	nextActions := nextActionsFromRecommendations(recommendations)

	var profileIDPtr *string
	if input.ProfileID != nil && input.ProfileID.Valid {
		p := input.ProfileID.String()
		profileIDPtr = &p
	}

	result := advisorFromResponse(raw, input.ContentID.String(), profileIDPtr, ranking, nextActions, freshness.OverallFreshnessScore)

	if s.vk != nil {
		ttl := 15 * time.Minute
		if s.settings != nil {
			ttl = time.Duration(s.settings.GetIntOr(ctx, input.TenantID, "jce.advisor_cache_ttl_minutes", 15)) * time.Minute
		}
		b, _ := json.Marshal(result)
		_ = s.vk.Set(ctx, cacheKey, b, ttl).Err()
	}

	return result, nil
}

// Analyze answers a natural-language question about a comparison and profile.
func (s *Service) Analyze(ctx context.Context, contentID, tenantID, userID, profileID pgtype.UUID, question string) (AdvisorResult, error) {
	input := AdvisorInput{
		ContentID: contentID,
		TenantID:  tenantID,
		UserID:    userID,
		Question:  question,
	}
	if profileID.Valid {
		input.ProfileID = &profileID
	}
	return s.runAdvisor(ctx, "analyze", input)
}

// Rank returns a ranked list of comparison items with reasoning.
func (s *Service) Rank(ctx context.Context, contentID, tenantID, userID, profileID pgtype.UUID) (AdvisorResult, error) {
	input := AdvisorInput{
		ContentID: contentID,
		TenantID:  tenantID,
		UserID:    userID,
	}
	if profileID.Valid {
		input.ProfileID = &profileID
	}
	return s.runAdvisor(ctx, "rank", input)
}

// Explain provides an explanation for a single item and factor.
func (s *Service) Explain(ctx context.Context, contentID, itemID, tenantID, userID, profileID pgtype.UUID, factorKey, question string) (AdvisorResult, error) {
	input := AdvisorInput{
		ContentID: contentID,
		TenantID:  tenantID,
		UserID:    userID,
		ItemID:    &itemID,
		FactorKey: factorKey,
		Question:  question,
	}
	if profileID.Valid {
		input.ProfileID = &profileID
	}
	return s.runAdvisor(ctx, "explain", input)
}

// Recommend returns the top next actions and recommendations for a comparison.
func (s *Service) Recommend(ctx context.Context, contentID, tenantID, userID, profileID pgtype.UUID) (AdvisorResult, error) {
	input := AdvisorInput{
		ContentID: contentID,
		TenantID:  tenantID,
		UserID:    userID,
	}
	if profileID.Valid {
		input.ProfileID = &profileID
	}
	return s.runAdvisor(ctx, "recommend", input)
}

// StreamAdvisor calls Analyze and emits the result as a single SSE message
// event. It sets the text/event-stream headers and flushes the response.
func (s *Service) StreamAdvisor(ctx context.Context, contentID, tenantID, userID, profileID pgtype.UUID, question string, w http.ResponseWriter) error {
	res, err := s.Analyze(ctx, contentID, tenantID, userID, profileID, question)
	if err != nil {
		return err
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	payload, _ := json.Marshal(map[string]any{
		"type":    "message",
		"content": res.Summary,
	})

	_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)

	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	return nil
}
