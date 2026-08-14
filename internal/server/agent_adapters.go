// Package server — agent_adapters.go provides the wiring-layer adapters that
// bridge the AI Agent Executor's (Section 103, ADR-111) narrow dependency
// interfaces to the real service implementations.
//
// The adapters exist because:
//  1. The ai package's CacheClient uses plain (string, error) returns while
//     valkeycompat.Cmdable uses go-redis command types — a thin adapter
//     resolves the signature mismatch without polluting the ai package
//     with valkeycompat imports.
//  2. The ai package's RAGService returns []ai.RAGChunk (a mirror type)
//     to avoid an import cycle with internal/rag — the adapter converts
//     rag.RetrievedChunk to ai.RAGChunk.
//  3. The ContentCreator/ContentUpdater function bridges adapt
//     content.Service.CreateContent/UpdateContent into the ai package's
//     provider-agnostic input/output types, avoiding an import cycle
//     between internal/ai and internal/content.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 6 wiring)
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/ai"
	"github.com/jedafe/japa/internal/arrivalchecklist"
	"github.com/jedafe/japa/internal/attestationguide"
	"github.com/jedafe/japa/internal/comparison"
	"github.com/jedafe/japa/internal/communitylocator"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/credentialchecker"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/docs"
	"github.com/jedafe/japa/internal/languagetesthub"
	"github.com/jedafe/japa/internal/learning"
	"github.com/jedafe/japa/internal/menus"
	"github.com/jedafe/japa/internal/rag"
	"github.com/jedafe/japa/internal/studio"
	"github.com/jedafe/japa/internal/taxcalculator"
	"github.com/jedafe/japa/internal/users"
	"github.com/jedafe/japa/internal/worker"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// agentCacheAdapter adapts valkeycompat.Cmdable to the ai.CacheClient
// interface. The ai package uses plain (string, error) returns so it
// doesn't depend on valkeycompat command types.
type agentCacheAdapter struct {
	rdb valkeycompat.Cmdable
}

// Get retrieves a string value from Valkey. Returns "" + the redis error
// on miss/failure (the caller treats any error as a cache miss).
func (a *agentCacheAdapter) Get(ctx context.Context, key string) (string, error) {
	return a.rdb.Get(ctx, key).Result()
}

// Set writes a string value to Valkey with the given TTL in seconds.
func (a *agentCacheAdapter) Set(ctx context.Context, key, value string, ttlSeconds int) error {
	return a.rdb.Set(ctx, key, value, time.Duration(ttlSeconds)*time.Second).Err()
}

// agentRAGAdapter adapts rag.Service to the ai.RAGService interface. The
// ai package defines its own RAGChunk mirror type to avoid importing
// internal/rag (which imports internal/ai for the embedder adapter).
type agentRAGAdapter struct {
	svc *rag.Service
}

// Retrieve calls rag.Service.Retrieve and converts each RetrievedChunk to
// the ai-package mirror type.
func (a *agentRAGAdapter) Retrieve(ctx context.Context, tenantID pgtype.UUID, query, language string, geoSpaceID *pgtype.UUID) ([]ai.RAGChunk, error) {
	chunks, err := a.svc.Retrieve(ctx, tenantID, query, language, geoSpaceID)
	if err != nil {
		return nil, err
	}
	out := make([]ai.RAGChunk, 0, len(chunks))
	for _, c := range chunks {
		out = append(out, ai.RAGChunk{
			EntityID:   c.EntityID,
			EntityType: c.EntityType,
			ChunkText:  c.ChunkText,
			Similarity: c.Similarity,
			Tier:       c.Tier,
		})
	}
	return out, nil
}

// agentContentCreatorBridge adapts content.Service.CreateContent to the
// ai.ContentCreator function type. The ai package uses a provider-agnostic
// ContentCreateInput/ContentCreateRow so it doesn't import internal/content.
type agentContentCreatorBridge struct {
	svc *content.Service
}

func (b *agentContentCreatorBridge) create(ctx context.Context, tenantID pgtype.UUID, input ai.ContentCreateInput) (ai.ContentCreateRow, error) {
	row, err := b.svc.CreateContent(ctx, tenantID, content.CreateContentInput{
		AuthorID:    input.AuthorID,
		SpaceID:     input.SpaceID,
		ContentType: input.ContentType,
		Title:       input.Title,
		Body:        input.Body,
		FieldValues: input.FieldValues,
		GeoSpaceID:  input.GeoSpaceID,
		Status:      input.Status,
	})
	if err != nil {
		return ai.ContentCreateRow{}, err
	}
	return ai.ContentCreateRow{
		ID:          row.ID,
		Title:       row.Title,
		Slug:        row.Slug,
		Status:      row.Status,
		ContentType: row.ContentType,
		AuthorID:    row.AuthorID,
		GeoSpaceID:  row.GeoSpaceID,
		CreatedAt:   row.CreatedAt,
	}, nil
}

// agentContentUpdaterBridge adapts content.Service.UpdateContent to the
// ai.ContentUpdater function type.
type agentContentUpdaterBridge struct {
	svc *content.Service
}

func (b *agentContentUpdaterBridge) update(ctx context.Context, contentID, tenantID, editedBy pgtype.UUID, input ai.ContentUpdateInput) (ai.ContentUpdateRow, error) {
	// Build the content.UpdateContentInput from the ai-package input. Only
	// the pointer fields that are non-nil are set; the rest default to
	// zero values that content.Service treats as "unchanged" when the
	// corresponding sqlc.narg is invalid.
	var fieldValues []byte
	if input.FieldValues != nil {
		fieldValues = input.FieldValues
	}
	var geoSpaceID pgtype.UUID
	if input.GeoSpaceID != nil {
		geoSpaceID = *input.GeoSpaceID
	}

	row, err := b.svc.UpdateContent(ctx, contentID, tenantID, editedBy, content.UpdateContentInput{
		Title:       derefStr(input.Title),
		Slug:        derefStr(input.Slug),
		Body:        derefStr(input.Body),
		FieldValues: fieldValues,
		GeoSpaceID:  geoSpaceID,
		Status:      derefStr(input.Status),
	}, true) // bypassEditWindow: the agent operates on behalf of the user
	if err != nil {
		return ai.ContentUpdateRow{}, err
	}
	return ai.ContentUpdateRow{
		ID:        row.ID,
		Title:     row.Title,
		Slug:      row.Slug,
		Status:    row.Status,
		UpdatedAt: row.UpdatedAt,
	}, nil
}

// agentRateLimiterAdapter adapts valkeycompat.Cmdable to the ai.RateLimiter
// interface. The ai package uses int TTL seconds and plain error returns;
// valkeycompat uses time.Duration and command types.
type agentRateLimiterAdapter struct {
	rdb valkeycompat.Cmdable
}

// Incr increments the counter at key and returns the new value.
func (a *agentRateLimiterAdapter) Incr(ctx context.Context, key string) (int64, error) {
	return a.rdb.Incr(ctx, key).Result()
}

// IncrByFloat increments the float counter at key by increment and returns the new value.
func (a *agentRateLimiterAdapter) IncrByFloat(ctx context.Context, key string, increment float64) (float64, error) {
	return a.rdb.IncrByFloat(ctx, key, increment).Result()
}

// Expire sets the TTL on key in seconds.
func (a *agentRateLimiterAdapter) Expire(ctx context.Context, key string, ttlSeconds int) error {
	return a.rdb.Expire(ctx, key, time.Duration(ttlSeconds)*time.Second).Err()
}

// Get retrieves the string value at key. Returns the redis nil error
// (valkeycompat.Nil) when the key does not exist — the caller treats any
// error as "no value yet" and skips the budget check.
func (a *agentRateLimiterAdapter) Get(ctx context.Context, key string) (string, error) {
	return a.rdb.Get(ctx, key).Result()
}

// derefStr returns *s if s is non-nil, else "".
func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// agentComparisonAdapter adapts comparison.Service to the ai.ComparisonService
// interface (Phase 7). The ai package defines its own input/output mirror
// types (ComparisonTypeCreateInput, ComparisonCreateInput, etc.) to avoid
// importing internal/comparison — the adapter converts between the two.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Comparison Engine)
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 7 wiring)
type agentComparisonAdapter struct {
	svc *comparison.Service
}

func (a *agentComparisonAdapter) ListComparisonTypes(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]gen.ListComparisonTypesByTenantRow, error) {
	return a.svc.ListComparisonTypes(ctx, tenantID, limit, offset)
}

func (a *agentComparisonAdapter) GetComparisonTypeBySlug(ctx context.Context, slug string, tenantID pgtype.UUID) (gen.GetComparisonTypeBySlugRow, error) {
	return a.svc.GetComparisonTypeBySlug(ctx, slug, tenantID)
}

func (a *agentComparisonAdapter) CreateComparisonType(ctx context.Context, tenantID pgtype.UUID, input ai.ComparisonTypeCreateInput) (gen.CreateComparisonTypeRow, error) {
	return a.svc.CreateComparisonType(ctx, tenantID, comparison.CreateComparisonTypeInput{
		Name:         input.Name,
		Slug:         input.Slug,
		Description:  input.Description,
		Icon:         input.Icon,
		FieldGroupID: input.FieldGroupID,
		ItemLabel:    input.ItemLabel,
		IsSystem:     input.IsSystem,
	})
}

func (a *agentComparisonAdapter) CreateComparison(ctx context.Context, tenantID, userID pgtype.UUID, input ai.ComparisonCreateInput) (ai.ComparisonCreateResult, error) {
	items := make([]comparison.ComparisonItemInput, 0, len(input.Items))
	for _, it := range input.Items {
		items = append(items, comparison.ComparisonItemInput{Name: it.Name, Slug: it.Slug})
	}
	res, err := a.svc.CreateComparison(ctx, tenantID, userID, comparison.CreateComparisonInput{
		Title:      input.Title,
		Body:       input.Body,
		Visibility: input.Visibility,
		Status:     input.Status,
		SpaceID:    input.SpaceID,
		TypeID:     input.TypeID,
		Items:      items,
		Dimensions: input.Dimensions,
	})
	if err != nil {
		return ai.ComparisonCreateResult{}, err
	}
	return ai.ComparisonCreateResult{
		ID:         res.ID,
		TenantID:   res.TenantID,
		AuthorID:   res.AuthorID,
		Title:      res.Title,
		Slug:       res.Slug,
		Visibility: res.Visibility,
		SpaceID:    res.SpaceID,
	}, nil
}

func (a *agentComparisonAdapter) GetComparisonDetail(ctx context.Context, id, tenantID pgtype.UUID) (ai.ComparisonDetail, error) {
	d, err := a.svc.GetComparisonDetail(ctx, id, tenantID)
	if err != nil {
		return ai.ComparisonDetail{}, err
	}
	return ai.ComparisonDetail{
		ID:         d.ID,
		TenantID:   d.TenantID,
		AuthorID:   d.AuthorID,
		Title:      d.Title,
		Slug:       d.Slug,
		Body:       d.Body,
		Visibility: d.Visibility,
		Status:     d.Status,
		SpaceID:    d.SpaceID,
		Items:      d.Items,
		Dimensions: d.Dimensions,
	}, nil
}

func (a *agentComparisonAdapter) GetComparisonMatrix(ctx context.Context, contentID, tenantID pgtype.UUID) ([]ai.ComparisonMatrixItem, error) {
	matrix, err := a.svc.GetComparisonMatrix(ctx, contentID, tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]ai.ComparisonMatrixItem, 0, len(matrix))
	for _, m := range matrix {
		dims := make([]ai.ComparisonMatrixDimension, 0, len(m.Dimensions))
		for _, d := range m.Dimensions {
			dims = append(dims, ai.ComparisonMatrixDimension{
				DimensionID:  d.DimensionID,
				Label:        d.Label,
				Type:         d.Type,
				Value:        d.Value,
				ValueRating:  d.ValueRating,
				ValueBoolean: d.ValueBoolean,
			})
		}
		out = append(out, ai.ComparisonMatrixItem{
			Item:       m.Item,
			Dimensions: dims,
		})
	}
	return out, nil
}

func (a *agentComparisonAdapter) CreateComparisonDimension(ctx context.Context, tenantID pgtype.UUID, input ai.ComparisonDimensionCreateInput) (gen.CreateComparisonDimensionRow, error) {
	return a.svc.CreateComparisonDimension(ctx, tenantID, comparison.CreateComparisonDimensionInput{
		ContentID:     input.ContentID,
		Label:         input.Label,
		DimensionType: input.DimensionType,
		Position:      input.Position,
	})
}

func (a *agentComparisonAdapter) UpsertComparisonValueWithMetadata(ctx context.Context, tenantID pgtype.UUID, input ai.ComparisonValueUpsertInput, metadata []byte) (gen.ComparisonValue, error) {
	return a.svc.UpsertComparisonValueWithMetadata(ctx, tenantID, comparison.UpsertComparisonValueInput{
		DimensionID:  input.DimensionID,
		ItemID:       input.ItemID,
		Value:        input.Value,
		ValueRating:  input.ValueRating,
		ValueBoolean: input.ValueBoolean,
	}, metadata)
}

// ---------------------------------------------------------------------------
// Phase 8 adapters — persona resolver + agent draft processor
// ---------------------------------------------------------------------------

// agentPersonaAdapter adapts personas.Store.GetAIPersonaByID to the
// ai.PersonaResolver interface. The ai package uses a narrow interface so
// it does not import internal/personas (avoids a cycle risk). The adapter
// returns only the backing users.id the content.create_draft tool needs.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Content Request Flow)
// Ref: japaconnect-engineering/sections/68-ai-personas.md (ai_personas.user_id)
type agentPersonaAdapter struct {
	store personaUserIDResolver
}

// personaUserIDResolver is the subset of *personas.Store the adapter calls.
// Defined as an interface so the adapter is testable with a stub.
type personaUserIDResolver interface {
	GetAIPersonaByID(ctx context.Context, tenantID, id pgtype.UUID) (gen.GetAIPersonaByIDRow, error)
}

// GetPersonaUserID returns the backing users.id for the given persona ID.
func (a *agentPersonaAdapter) GetPersonaUserID(ctx context.Context, tenantID, personaID pgtype.UUID) (pgtype.UUID, error) {
	row, err := a.store.GetAIPersonaByID(ctx, tenantID, personaID)
	if err != nil {
		return pgtype.UUID{}, err
	}
	return row.UserID, nil
}

// agentDraftProcessorAdapter implements worker.AgentDraftProcessor. It
// creates a draft content row with the AI persona as author + a HITL
// review with source='agent_draft_request'. The adapter closes over the
// content.Service + ai.Store + personas.Store + AI dispatcher so the
// worker package does not import internal/ai or internal/content (which
// would create a cycle).
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Content Request Flow, Asynq path)
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 8, task 8.03)
type agentDraftProcessorAdapter struct {
	contentSvc   *content.Service
	aiStore      *ai.Store
	personaStore personaUserIDResolver
	dispatcher   agentDraftDispatcher
}

// agentDraftDispatcher is the subset of *ai.Dispatcher the adapter calls to
// generate the draft body from the topic prompt + external results.
type agentDraftDispatcher interface {
	// GenerateDraftBody synthesizes a draft body from the topic prompt +
	// external search results. Returns markdown body text.
	GenerateDraftBody(ctx context.Context, tenantID pgtype.UUID, contentType, topicPrompt, externalResults string) (string, error)
}

// agentDraftDispatcherAdapter implements agentDraftDispatcher by calling
// the AI gateway's Dispatch with category "content_generation". The
// generated text is the draft body.
type agentDraftDispatcherAdapter struct {
	dispatcher aiDispatcherPort
}

// aiDispatcherPort is the subset of *ai.Dispatcher the adapter calls.
type aiDispatcherPort interface {
	Dispatch(ctx context.Context, category string, req ai.Request) (ai.Response, error)
}

// GenerateDraftBody synthesizes a draft body via the AI gateway.
func (a *agentDraftDispatcherAdapter) GenerateDraftBody(ctx context.Context, tenantID pgtype.UUID, contentType, topicPrompt, externalResults string) (string, error) {
	prompt := fmt.Sprintf("Generate a %s draft about: %s", contentType, topicPrompt)
	if externalResults != "" {
		prompt += fmt.Sprintf("\n\nExternal reference material:\n%s", externalResults)
	}
	resp, err := a.dispatcher.Dispatch(ctx, "content_generation", ai.Request{
		Category: "content_generation",
		Payload: map[string]any{
			"prompt":    prompt,
			"tenant_id": tenantID,
		},
	})
	if err != nil {
		return "", err
	}
	if text, ok := resp.Raw["text"].(string); ok {
		return text, nil
	}
	if text, ok := resp.Raw["content"].(string); ok {
		return text, nil
	}
	if output, ok := resp.Raw["output"].(string); ok {
		return output, nil
	}
	// Fallback: no recognized text field; return the topic prompt as the
	// body so the draft is still created with meaningful content.
	return topicPrompt, nil
}

// CreateAgentDraft implements worker.AgentDraftProcessor. It:
//  1. Resolves the persona's backing users.id (the author).
//  2. Generates the draft body via the AI dispatcher (synthesizes the
//     topic prompt + external results).
//  3. Creates the contents row with status='draft', author_id=persona
//     user_id, and _ai_provenance in field_values.
//  4. Creates a HITL review with source='agent_draft_request' + the
//     created content_id.
func (a *agentDraftProcessorAdapter) CreateAgentDraft(ctx context.Context, payload worker.AgentDraftPayload) error {
	var tenantID pgtype.UUID
	if err := tenantID.Scan(payload.TenantID); err != nil {
		return fmt.Errorf("invalid tenant_id: %w", err)
	}
	var personaID pgtype.UUID
	if err := personaID.Scan(payload.PersonaID); err != nil {
		return fmt.Errorf("invalid persona_id: %w", err)
	}
	var userID pgtype.UUID
	if payload.UserID != "" {
		_ = userID.Scan(payload.UserID) // best-effort; only used for provenance
	}
	var geoSpaceID pgtype.UUID
	if payload.GeoSpaceID != "" {
		_ = geoSpaceID.Scan(payload.GeoSpaceID)
	}

	// 1. Resolve the persona's backing user_id (the author).
	persona, err := a.personaStore.GetAIPersonaByID(ctx, tenantID, personaID)
	if err != nil {
		return fmt.Errorf("resolving persona: %w", err)
	}
	if payload.ContentType == "" {
		return fmt.Errorf("content_type is required")
	}

	// 2. Generate the draft body via the AI dispatcher.
	body := payload.TopicPrompt
	if a.dispatcher != nil {
		generated, gerr := a.dispatcher.GenerateDraftBody(ctx, tenantID, payload.ContentType, payload.TopicPrompt, payload.ExternalResults)
		if gerr != nil {
			return fmt.Errorf("generating draft body: %w", gerr)
		}
		if generated != "" {
			body = generated
		}
	}

	// 3. Build field_values JSONB with _ai_provenance.
	provenance := map[string]any{
		"ai_generated":        true,
		"ai_persona_id":       payload.PersonaID,
		"prompted_by_user_id": payload.UserID,
		"original_prompt":     payload.TopicPrompt,
		"agent_session_id":    payload.AgentSessionID,
	}
	fieldValues := map[string]any{
		"_ai_provenance": provenance,
	}
	fvJSON, err := json.Marshal(fieldValues)
	if err != nil {
		return fmt.Errorf("encoding field values: %w", err)
	}

	// 4. Create the contents row with status='draft'.
	row, err := a.contentSvc.CreateContent(ctx, tenantID, content.CreateContentInput{
		AuthorID:    persona.UserID,
		ContentType: payload.ContentType,
		Title:       payload.TopicPrompt,
		Body:        body,
		FieldValues: fvJSON,
		GeoSpaceID:  geoSpaceID,
		Status:      "draft",
	})
	if err != nil {
		return fmt.Errorf("creating draft content: %w", err)
	}

	// 5. Create a HITL review with source='agent_draft_request'.
	if a.aiStore != nil {
		_, hitlErr := a.aiStore.CreateHITLReview(ctx, ai.HITLReview{
			TenantID:    tenantID,
			ContentID:   row.ID,
			ContentType: payload.ContentType,
			AIOutput:    fvJSON,
			Status:      ai.HITLStatusPending,
			Source:      ai.HITLSourceAgentDraftRequest,
		})
		if hitlErr != nil {
			// Non-fatal: the draft was created; the admin can still see
			// it in the content list. The HITL queue simply did not get
			// a row.
		}
	}

	return nil
}

// ---------------------------------------------------------------------------
// Phase 9 adapters (Section 103 — extended tool catalog)
// ---------------------------------------------------------------------------

// agentLearningAdapter bridges learning.Store to the ai.LearningStore
// interface. The ai package uses mirror types (ai.LearningTrack,
// ai.LearningLesson, ai.LearningCreateTrackInput, etc.) to avoid an import
// cycle with internal/learning. This adapter converts between the two.
//
// learn.lessons.reorder has no backing Store method — the adapter uses
// UpdateLesson per entry to set the new sort_order. This is sequential
// (not a single bulk SQL UPDATE) but correct; a future migration can add
// a bulk reorder query if performance requires it.
type agentLearningAdapter struct{ store *learning.Store }

func (a *agentLearningAdapter) ListTracks(ctx context.Context, tenantID pgtype.UUID) ([]ai.LearningTrack, error) {
	tracks, err := a.store.ListAllTracks(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]ai.LearningTrack, 0, len(tracks))
	for _, t := range tracks {
		out = append(out, learningTrackToAI(t))
	}
	return out, nil
}

func (a *agentLearningAdapter) CreateTrack(ctx context.Context, tenantID pgtype.UUID, input ai.LearningCreateTrackInput) (*ai.LearningTrack, error) {
	t, err := a.store.CreateTrack(ctx, tenantID, learning.CreateTrackInput{
		Title:                input.Title,
		Slug:                 input.Slug,
		Description:          input.Description,
		Difficulty:           input.Difficulty,
		CoverMediaID:         input.CoverMediaID,
		IsPublished:          input.IsPublished,
		SortOrder:            input.SortOrder,
		Category:             input.Category,
		SubscriptionEligible: input.SubscriptionEligible,
	})
	if err != nil {
		return nil, err
	}
	aiT := learningTrackToAI(*t)
	return &aiT, nil
}

func (a *agentLearningAdapter) UpdateTrack(ctx context.Context, tenantID, id pgtype.UUID, input ai.LearningUpdateTrackInput) (*ai.LearningTrack, error) {
	t, err := a.store.UpdateTrack(ctx, tenantID, id, learning.UpdateTrackInput{
		Title:                input.Title,
		Description:          input.Description,
		Difficulty:           input.Difficulty,
		CoverMediaID:         input.CoverMediaID,
		IsPublished:          input.IsPublished,
		SortOrder:            input.SortOrder,
		Category:             input.Category,
		SubscriptionEligible: input.SubscriptionEligible,
	})
	if err != nil {
		return nil, err
	}
	aiT := learningTrackToAI(*t)
	return &aiT, nil
}

func (a *agentLearningAdapter) ListLessons(ctx context.Context, tenantID pgtype.UUID) ([]ai.LearningLesson, error) {
	lessons, err := a.store.ListAllLessons(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]ai.LearningLesson, 0, len(lessons))
	for _, l := range lessons {
		out = append(out, learningLessonToAI(l))
	}
	return out, nil
}

func (a *agentLearningAdapter) CreateLesson(ctx context.Context, tenantID, moduleID pgtype.UUID, input ai.LearningCreateLessonInput) (*ai.LearningLesson, error) {
	l, err := a.store.CreateLesson(ctx, tenantID, moduleID, learning.CreateLessonInput{
		Title:           input.Title,
		Slug:            input.Slug,
		LessonType:      input.LessonType,
		Content:         input.Content,
		MediaID:         input.MediaID,
		DurationMinutes: input.DurationMinutes,
		SortOrder:       input.SortOrder,
		IsPublished:     input.IsPublished,
		IsPreview:       input.IsPreview,
	})
	if err != nil {
		return nil, err
	}
	aiL := learningLessonToAI(*l)
	return &aiL, nil
}

func (a *agentLearningAdapter) UpdateLesson(ctx context.Context, tenantID, id pgtype.UUID, input ai.LearningUpdateLessonInput) (*ai.LearningLesson, error) {
	l, err := a.store.UpdateLesson(ctx, tenantID, id, learning.UpdateLessonInput{
		Title:           input.Title,
		LessonType:      input.LessonType,
		Content:         input.Content,
		MediaID:         input.MediaID,
		DurationMinutes: input.DurationMinutes,
		SortOrder:       input.SortOrder,
		IsPublished:     input.IsPublished,
		IsPreview:       input.IsPreview,
	})
	if err != nil {
		return nil, err
	}
	aiL := learningLessonToAI(*l)
	return &aiL, nil
}

// ReorderLessons updates each lesson's sort_order sequentially. The
// learning.Store does not expose a bulk reorder query, so we call
// UpdateLesson per entry. A failure on one entry aborts the reorder
// (the caller can retry). This is acceptable for the AI agent's use
// case (small batches).
func (a *agentLearningAdapter) ReorderLessons(ctx context.Context, tenantID pgtype.UUID, entries []ai.LearningReorderEntry) error {
	for _, e := range entries {
		// Fetch the existing lesson to preserve its fields, then update
		// only the sort_order. UpdateLessonInput requires Title and
		// LessonType, so we must read first.
		lesson, err := a.store.GetLessonByID(ctx, tenantID, e.ID)
		if err != nil {
			return fmt.Errorf("reorder: fetching lesson %s: %w", e.ID, err)
		}
		_, err = a.store.UpdateLesson(ctx, tenantID, e.ID, learning.UpdateLessonInput{
			Title:           lesson.Title,
			LessonType:      lesson.LessonType,
			Content:         lesson.Content,
			MediaID:         derefUUID(lesson.MediaID),
			DurationMinutes: lesson.DurationMinutes,
			SortOrder:       e.SortOrder,
			IsPublished:     lesson.IsPublished,
		})
		if err != nil {
			return fmt.Errorf("reorder: updating lesson %s sort_order: %w", e.ID, err)
		}
	}
	return nil
}

func learningTrackToAI(t learning.Track) ai.LearningTrack {
	return ai.LearningTrack{
		ID:          t.ID,
		TenantID:    t.TenantID,
		Title:       t.Title,
		Slug:        t.Slug,
		Description: t.Description,
		Difficulty:  t.Difficulty,
		IsPublished: t.IsPublished,
		SortOrder:   t.SortOrder,
		CreatedAt:   pgtype.Timestamptz{Time: t.CreatedAt, Valid: !t.CreatedAt.IsZero()},
		UpdatedAt:   pgtype.Timestamptz{Time: t.UpdatedAt, Valid: !t.UpdatedAt.IsZero()},
	}
}

func learningLessonToAI(l learning.Lesson) ai.LearningLesson {
	return ai.LearningLesson{
		ID:              l.ID,
		TenantID:        l.TenantID,
		ModuleID:        l.ModuleID,
		Title:           l.Title,
		Slug:            l.Slug,
		LessonType:      l.LessonType,
		Content:         l.Content,
		DurationMinutes: l.DurationMinutes,
		SortOrder:       l.SortOrder,
		IsPublished:     l.IsPublished,
		IsPreview:       l.IsPreview,
		CreatedAt:       pgtype.Timestamptz{Time: l.CreatedAt, Valid: !l.CreatedAt.IsZero()},
		UpdatedAt:       pgtype.Timestamptz{Time: l.UpdatedAt, Valid: !l.UpdatedAt.IsZero()},
	}
}

// derefUUID returns the pgtype.UUID value pointed to by p, or the zero
// value when p is nil.
func derefUUID(p *pgtype.UUID) pgtype.UUID {
	if p == nil {
		return pgtype.UUID{}
	}
	return *p
}

// agentDocsAdapter bridges docs.Service to the ai.DocsService interface.
// The ai package uses mirror types (ai.DocsPage, ai.DocsCreatePageRequest,
// ai.DocsUpdatePageRequest) to avoid an import cycle with internal/docs.
type agentDocsAdapter struct{ svc *docs.Service }

func (a *agentDocsAdapter) ListAllDocPages(ctx context.Context, tenantID pgtype.UUID) ([]ai.DocsPage, error) {
	pages, err := a.svc.ListAllDocPages(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]ai.DocsPage, 0, len(pages))
	for _, p := range pages {
		out = append(out, docsPageToAI(p))
	}
	return out, nil
}

func (a *agentDocsAdapter) GetDocPageByID(ctx context.Context, tenantID, id pgtype.UUID) (*ai.DocsPage, error) {
	p, err := a.svc.GetDocPageByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	aiP := docsPageToAI(*p)
	return &aiP, nil
}

func (a *agentDocsAdapter) CreateDocPage(ctx context.Context, tenantID pgtype.UUID, req ai.DocsCreatePageRequest) (pgtype.UUID, error) {
	return a.svc.CreateDocPage(ctx, tenantID, docs.CreateDocPageRequest{
		Title:        req.Title,
		Slug:         req.Slug,
		Audience:     req.Audience,
		Content:      req.Content,
		AIGenerated:  req.AIGenerated,
		ReviewStatus: req.ReviewStatus,
		ParentID:     req.ParentID,
		Position:     req.Position,
	})
}

func (a *agentDocsAdapter) UpdateDocPage(ctx context.Context, tenantID, id pgtype.UUID, req ai.DocsUpdatePageRequest) error {
	return a.svc.UpdateDocPage(ctx, tenantID, id, docs.UpdateDocPageRequest{
		Title:        req.Title,
		Content:      req.Content,
		ReviewStatus: req.ReviewStatus,
	})
}

func docsPageToAI(p docs.DocPage) ai.DocsPage {
	return ai.DocsPage{
		ID:           p.ID,
		Title:        p.Title,
		Slug:         p.Slug,
		Audience:     p.Audience,
		Content:      p.Content,
		AIGenerated:  p.AIGenerated,
		ReviewStatus: p.ReviewStatus,
		ParentID:     p.ParentID,
		Position:     p.Position,
		CreatedAt:    p.CreatedAt,
		UpdatedAt:    p.UpdatedAt,
	}
}

// agentMenusAdapter bridges menus.Service to the ai.MenusService interface.
// The ai package uses mirror types (ai.MenusCreateMenuItemInput,
// ai.MenusUpdateMenuItemInput, ai.MenusListResponse) to avoid an import
// cycle with internal/menus. ListMenus returns *menus.ListMenusResponse
// (pointer); the adapter dereferences it.
type agentMenusAdapter struct{ svc *menus.Service }

func (a *agentMenusAdapter) ListMenus(ctx context.Context, tenantID pgtype.UUID, q, placement, status, sort, sortDir string, limit, offset int32) (ai.MenusListResponse, error) {
	resp, err := a.svc.ListMenus(ctx, tenantID, q, placement, status, sort, sortDir, limit, offset)
	if err != nil {
		return ai.MenusListResponse{}, err
	}
	return ai.MenusListResponse{
		Items:      resp.Items,
		Total:      resp.Total,
		TotalPages: resp.TotalPages,
		Page:       resp.Page,
	}, nil
}

func (a *agentMenusAdapter) CreateMenuItem(ctx context.Context, tenantID pgtype.UUID, input ai.MenusCreateMenuItemInput) (gen.CreateMenuItemRow, error) {
	return a.svc.CreateMenuItem(ctx, tenantID, menus.CreateMenuItemInput{
		MenuID:           input.MenuID,
		ParentID:         input.ParentID,
		Label:            input.Label,
		URL:              input.URL,
		Icon:             input.Icon,
		SortOrder:        input.SortOrder,
		PermissionNode:   input.PermissionNode,
		RequiredGroupIDs: input.RequiredGroupIDs,
		Visibility:       input.Visibility,
		Target:           input.Target,
		IsAdSlot:         input.IsAdSlot,
		AdSlotConfig:     input.AdSlotConfig,
		Status:           input.Status,
	})
}

func (a *agentMenusAdapter) UpdateMenuItem(ctx context.Context, id, tenantID pgtype.UUID, input ai.MenusUpdateMenuItemInput) (gen.UpdateMenuItemRow, error) {
	return a.svc.UpdateMenuItem(ctx, id, tenantID, menus.UpdateMenuItemInput{
		Label:            input.Label,
		URL:              input.URL,
		Icon:             input.Icon,
		ParentID:         input.ParentID,
		SortOrder:        input.SortOrder,
		PermissionNode:   input.PermissionNode,
		RequiredGroupIDs: input.RequiredGroupIDs,
		Visibility:       input.Visibility,
		Target:           input.Target,
		IsAdSlot:         input.IsAdSlot,
		AdSlotConfig:     input.AdSlotConfig,
		Status:           input.Status,
	})
}

func (a *agentMenusAdapter) DeleteMenuItem(ctx context.Context, id, tenantID pgtype.UUID) error {
	return a.svc.DeleteMenuItem(ctx, id, tenantID)
}

// agentUsersAdapter bridges users.Service to the ai.UsersService interface.
// The ai package uses mirror types (ai.UserProfile,
// ai.UserProfileUpdateRequest) to avoid an import cycle with
// internal/users. The adapter strips PII from UserProfileResponse.
type agentUsersAdapter struct{ svc *users.Service }

func (a *agentUsersAdapter) GetProfile(ctx context.Context, tenantID, userID pgtype.UUID) (*ai.UserProfile, error) {
	p, err := a.svc.GetProfile(ctx, tenantID, userID)
	if err != nil {
		return nil, err
	}
	return &ai.UserProfile{
		ID:            p.ID,
		Username:      p.Username,
		DisplayName:   p.DisplayName,
		AvatarURL:     p.AvatarURL,
		Bio:           p.Bio,
		Timezone:      p.Timezone,
		ProfileType:   p.ProfileType,
		FollowerCount: p.FollowerCount,
		CreatedAt:     p.CreatedAt,
	}, nil
}

func (a *agentUsersAdapter) UpdateProfile(ctx context.Context, tenantID, userID pgtype.UUID, req *ai.UserProfileUpdateRequest) error {
	var uReq *users.UpdateProfileRequest
	if req != nil {
		uReq = &users.UpdateProfileRequest{
			DisplayName:        req.DisplayName,
			AvatarURL:          req.AvatarURL,
			Bio:                req.Bio,
			Timezone:           req.Timezone,
			OriginCountry:      req.OriginCountry,
			DestinationCountry: req.DestinationCountry,
		}
	}
	return a.svc.UpdateProfile(ctx, tenantID, userID, uReq)
}

// ---------------------------------------------------------------------------
// AI Content Studio (Section 75, ADR-076) — studioDraftProcessorAdapter
// ---------------------------------------------------------------------------

// studioDraftProcessorAdapter implements worker.StudioProcessor by closing
// over the studio service (job state + batch/retry orchestration) and the
// content/ai/personas services needed for the actual draft generation. It
// lives in the wiring layer (internal/server) so the worker package does not
// import internal/studio, internal/content, internal/ai, or internal/personas
// (which would create cycles).
// intelRecordLoader is the subset of the intelligence store needed to
// load extraction context for Studio generation (Approach B).
type intelRecordLoader interface {
	GetIntelRecord(ctx context.Context, tenantID, id pgtype.UUID) (gen.IntelRecord, error)
}

type studioDraftProcessorAdapter struct {
	studio       studioProcessorService
	contentSvc   *content.Service
	aiStore      *ai.Store
	personaStore personaUserIDResolver
	dispatcher   aiDispatcherPort
	intelStore   intelRecordLoader
	// Tool table services for Approach B routing (ADR-076 Amendment 1).
	// When a Studio job has a tool-table content type, GenerateJob routes
	// to the corresponding service instead of creating a contents row.
	arrivalSvc       arrivalDraftGenerator
	communityOrgSvc  communityOrgDraftGenerator
	attestationSvc   attestationDraftGenerator
	languageTestSvc  languageTestDraftGenerator
	taxSvc           taxDraftGenerator
	credentialSvc    credentialDraftGenerator
}

// arrivalDraftGenerator is the subset of *arrivalchecklist.Service needed
// for Studio routing (Approach B).
type arrivalDraftGenerator interface {
	GenerateGuide(ctx context.Context, tenantID pgtype.UUID, req arrivalchecklist.GenerateGuideRequest) (arrivalchecklist.GenerateGuideResponse, error)
}

// communityOrgDraftGenerator is the subset of *communitylocator.Service.
type communityOrgDraftGenerator interface {
	Generate(ctx context.Context, tenantID, generatedBy pgtype.UUID, input communitylocator.GenerateInput, aiModel string) (communitylocator.OrgDTO, error)
}

// attestationDraftGenerator is the subset of *attestationguide.Service.
type attestationDraftGenerator interface {
	GenerateDraft(ctx context.Context, tenantID pgtype.UUID, input attestationguide.GeneratePathwayInput) (attestationguide.PathwayDTO, error)
}

// languageTestDraftGenerator is the subset of *languagetesthub.Service.
type languageTestDraftGenerator interface {
	GenerateDraft(ctx context.Context, tenantID pgtype.UUID, input languagetesthub.GenerateRequirementRequest) (languagetesthub.RequirementDTO, error)
}

// taxDraftGenerator is the subset of *taxcalculator.Service.
type taxDraftGenerator interface {
	GenerateGuide(ctx context.Context, tenantID pgtype.UUID, req taxcalculator.GenerateGuideRequest) (taxcalculator.GenerateGuideResult, error)
}

// credentialDraftGenerator is the subset of *credentialchecker.Service.
type credentialDraftGenerator interface {
	GeneratePathwayDraft(ctx context.Context, tenantID pgtype.UUID, req credentialchecker.GeneratePathwayRequest) (credentialchecker.GeneratePathwayResponse, error)
}

// toolTableTargetTable maps tool-table content types to their backing
// PostgreSQL table name for HITL review routing (migration 000477).
var toolTableTargetTable = map[string]string{
	"housing_guide":            "housing_guides",
	"attestation_guide":        "attestation_pathways",
	"visa_route":               "visa_routes",
	"arrival_checklist":        "arrival_checklist_guides",
	"credential_pathway":       "credential_pathways",
	"language_test_requirement": "language_test_requirements",
	"tax_obligation_guide":     "tax_obligation_guides",
	"community_organisation":   "community_organisations",
	"wiki_page":                "wiki_pages",
	"calendar_entry":           "calendar_entries",
}

// isToolTableType returns true if the content type should route to a
// dedicated tool table instead of the contents table.
func isToolTableType(contentType string) bool {
	_, ok := toolTableTargetTable[contentType]
	return ok
}

// studioProcessorService is the subset of *studio.Service the adapter calls.
type studioProcessorService interface {
	GetJob(ctx context.Context, tenantID, id pgtype.UUID) (studio.Job, error)
	MarkJobGenerating(ctx context.Context, tenantID, jobID pgtype.UUID) error
	MarkJobGenerated(ctx context.Context, tenantID, jobID, draftContentID pgtype.UUID) error
	MarkJobRoutedToReview(ctx context.Context, tenantID, jobID, reviewID pgtype.UUID) error
	MarkJobFailed(ctx context.Context, tenantID, jobID pgtype.UUID, errMsg string) error
	MarkJobPublished(ctx context.Context, tenantID, jobID, draftContentID pgtype.UUID) error
	ShouldAutoPublish(ctx context.Context, tenantID pgtype.UUID, contentType, guideCategory string) bool
	RunBatch(ctx context.Context, tenantID, batchID pgtype.UUID) error
	RetryFailed(ctx context.Context, tenantID, jobID pgtype.UUID) error
	RetryBatchFailed(ctx context.Context, tenantID, batchID pgtype.UUID) (int, error)
}

// RunBatch delegates to the studio service.
func (a *studioDraftProcessorAdapter) RunBatch(ctx context.Context, tenantID, batchID pgtype.UUID) error {
	return a.studio.RunBatch(ctx, tenantID, batchID)
}

// RetryFailed delegates to the studio service.
func (a *studioDraftProcessorAdapter) RetryFailed(ctx context.Context, tenantID, jobID pgtype.UUID) error {
	return a.studio.RetryFailed(ctx, tenantID, jobID)
}

// RetryBatchFailed delegates to the studio service for batch-level retry.
func (a *studioDraftProcessorAdapter) RetryBatchFailed(ctx context.Context, tenantID, batchID pgtype.UUID) (int, error) {
	return a.studio.RetryBatchFailed(ctx, tenantID, batchID)
}

// GenerateJob runs the §75 single-job generation pipeline:
//  1. Load the studio job + persona (voice prompt + backing author).
//  2. Generate the draft body via the AI gateway (content_generation).
//  3. Create the contents row with status='draft', author=persona user_id.
//  4. Route to the HITL review queue with source='studio', OR auto-publish
//     low-stakes types per studio.auto_publish_types (§75).
//  5. Update the studio job to 'in_review' (or 'published').
func (a *studioDraftProcessorAdapter) GenerateJob(ctx context.Context, tenantID, jobID pgtype.UUID) error {
	job, err := a.studio.GetJob(ctx, tenantID, jobID)
	if err != nil {
		return fmt.Errorf("studio: generate: get job: %w", err)
	}
	// Guard against re-processing a job that already produced a draft.
	if job.Status != "queued" && job.Status != "generating" {
		slog.Warn("studio: generate: job not in a generatable state, skipping",
			"job_id", jobID, "status", job.Status)
		return nil
	}

	// 1. Mark generating (compare-and-swap: only succeeds if status is still
	// 'queued'). If another worker already claimed this job, skip silently —
	// this prevents duplicate draft generation from concurrent tasks.
	if err := a.studio.MarkJobGenerating(ctx, tenantID, jobID); err != nil {
		if errors.Is(err, studio.ErrJobAlreadyGenerating) {
			slog.Info("studio: generate: job already claimed by another worker, skipping",
				"job_id", jobID)
			return nil
		}
		return fmt.Errorf("studio: generate: mark generating: %w", err)
	}

	// 2. Resolve the persona's backing user (the author) + voice prompt.
	persona, err := a.personaStore.GetAIPersonaByID(ctx, tenantID, job.PersonaID)
	if err != nil {
		_ = a.studio.MarkJobFailed(ctx, tenantID, jobID, "persona not found: "+err.Error())
		return fmt.Errorf("studio: generate: resolve persona: %w", err)
	}

	// Approach B: load intel_record extraction as factual grounding context.
	var extractionContext string
	if job.IntelRecordID.Valid && a.intelStore != nil {
		record, rerr := a.intelStore.GetIntelRecord(ctx, tenantID, job.IntelRecordID)
		if rerr == nil && len(record.Extracted) > 0 {
			extractionContext = string(record.Extracted)
		} else if rerr != nil {
			slog.Warn("studio: generate: load intel_record failed, generating without extraction context",
				"job_id", jobID, "intel_record_id", job.IntelRecordID, "error", rerr)
		}
	}

	// 3. Generate the draft body via the AI gateway.
	body := job.TopicPrompt
	if a.dispatcher != nil {
		generated, gerr := a.studioGenerateBody(ctx, tenantID, job, persona, extractionContext)
		if gerr != nil {
			_ = a.studio.MarkJobFailed(ctx, tenantID, jobID, "AI generation failed: "+gerr.Error())
			return fmt.Errorf("studio: generate: body: %w", gerr)
		}
		if generated != "" {
			body = generated
		}
	}

	// 4. Build field_values with _ai_provenance (ADR-088 universal AI
	// content generation pattern).
	provenance := map[string]any{
		"ai_generated":    true,
		"ai_persona_id":   job.PersonaID.String(),
		"source":          "studio",
		"original_prompt": job.TopicPrompt,
	}
	if len(job.TargetTags) > 0 {
		provenance["target_tags"] = job.TargetTags
	}
	fieldValues := map[string]any{"_ai_provenance": provenance}
	fvJSON, err := json.Marshal(fieldValues)
	if err != nil {
		_ = a.studio.MarkJobFailed(ctx, tenantID, jobID, "encode field values: "+err.Error())
		return fmt.Errorf("studio: generate: encode field values: %w", err)
	}

	// 5. Route to the appropriate table based on content type.
	// Tool-table types (Approach B, ADR-076 Amendment 1) route to their
	// dedicated tool service. If the tool service call fails (e.g. missing
	// required fields not available in the Studio job), fall through to
	// contents creation as a safe default.
	if isToolTableType(job.ContentType) {
		if draftID, targetTable, routed, rerr := a.tryToolTableRouting(ctx, tenantID, jobID, job, persona, body, extractionContext); rerr == nil && routed {
			a.routeToHITL(ctx, tenantID, jobID, draftID, targetTable, job.ContentType, fvJSON)
			return nil
		}
		slog.Info("studio: generate: tool table routing fell back to contents",
			"job_id", jobID, "content_type", job.ContentType)
	}

	// 5b. Create the contents row with status='draft' (default path).
	row, err := a.contentSvc.CreateContent(ctx, tenantID, content.CreateContentInput{
		AuthorID:      persona.UserID,
		SpaceID:       job.TargetSpaceID,
		ContentType:   job.ContentType,
		Title:         job.TopicPrompt,
		Body:          body,
		FieldValues:   fvJSON,
		Status:        "draft",
		GuideCategory: job.GuideCategory,
	})
	if err != nil {
		_ = a.studio.MarkJobFailed(ctx, tenantID, jobID, "create content: "+err.Error())
		return fmt.Errorf("studio: generate: create content: %w", err)
	}

	// 6. Route to HITL review (source='studio') OR auto-publish low-stakes.
	if a.studio.ShouldAutoPublish(ctx, tenantID, job.ContentType, derefStr(job.GuideCategory)) {
		if _, perr := a.contentSvc.UpdateContentStatus(ctx, row.ID, tenantID, "published"); perr != nil {
			slog.Warn("studio: generate: auto-publish failed, routing to review",
				"job_id", jobID, "content_id", row.ID, "error", perr)
			_ = a.studio.MarkJobGenerated(ctx, tenantID, jobID, row.ID)
			a.createStudioReview(ctx, tenantID, row.ID, job.ContentType, fvJSON)
			return nil
		}
		if err := a.studio.MarkJobPublished(ctx, tenantID, jobID, row.ID); err != nil {
			slog.Warn("studio: generate: mark job published failed", "job_id", jobID, "error", err)
		}
		slog.Info("studio: generate: auto-published", "job_id", jobID, "content_id", row.ID)
		return nil
	}

	// Step 7 (ADR-076 §2): mark draft_ready with the draft_content_id.
	if err := a.studio.MarkJobGenerated(ctx, tenantID, jobID, row.ID); err != nil {
		return fmt.Errorf("studio: generate: mark generated: %w", err)
	}

	// Step 8 (ADR-076 §2): route to the HITL review queue.
	// NOTE: this path is the contents-creation fallback. Tool-table types
	// that successfully routed to their dedicated table returned early
	// above (via tryToolTableRouting → routeToHITL). When we reach here
	// the draft row lives in `contents`, so target_table must always be
	// 'contents' — even for tool-table content types that fell back.
	a.routeToHITL(ctx, tenantID, jobID, row.ID, "contents", job.ContentType, fvJSON)
	return nil
}

// tryToolTableRouting attempts to route a Studio job to its dedicated tool
// service (Approach B). Returns (draftID, targetTable, routed, error).
// When routed is false and error is nil, the caller should fall back to
// contents creation.
func (a *studioDraftProcessorAdapter) tryToolTableRouting(
	ctx context.Context,
	tenantID, jobID pgtype.UUID,
	job studio.Job,
	persona gen.GetAIPersonaByIDRow,
	body, extractionContext string,
) (pgtype.UUID, string, bool, error) {
	destSpaceStr := ""
	if job.TargetSpaceID.Valid {
		destSpaceStr = job.TargetSpaceID.String()
	}
	visaCat := derefStr(job.GuideCategory)
	promptWithCtx := job.TopicPrompt
	if extractionContext != "" {
		promptWithCtx += "\n\nStructured data from external source:\n" + extractionContext
	}

	switch job.ContentType {
	case "arrival_checklist":
		if a.arrivalSvc == nil {
			return pgtype.UUID{}, "", false, nil
		}
		resp, err := a.arrivalSvc.GenerateGuide(ctx, tenantID, arrivalchecklist.GenerateGuideRequest{
			DestinationSpaceID: destSpaceStr,
			VisaCategory:       visaCat,
			ApplicantType:      arrivalchecklist.ApplicantIndividual,
			Prompt:             promptWithCtx,
		})
		if err != nil {
			slog.Warn("studio: arrival_checklist routing failed, falling back to contents",
				"job_id", jobID, "error", err)
			return pgtype.UUID{}, "", false, nil
		}
		var id pgtype.UUID
		_ = id.Scan(resp.GuideID)
		return id, "arrival_checklist_guides", true, nil

	case "community_organisation":
		if a.communityOrgSvc == nil {
			return pgtype.UUID{}, "", false, nil
		}
		aiModel := "content_agent_v1"
		org, err := a.communityOrgSvc.Generate(ctx, tenantID, persona.UserID, communitylocator.GenerateInput{
			Prompt:             promptWithCtx,
			DestinationSpaceID: destSpaceStr,
		}, aiModel)
		if err != nil {
			slog.Warn("studio: community_organisation routing failed, falling back to contents",
				"job_id", jobID, "error", err)
			return pgtype.UUID{}, "", false, nil
		}
		var id pgtype.UUID
		_ = id.Scan(org.ID)
		return id, "community_organisations", true, nil

	case "attestation_guide":
		if a.attestationSvc == nil || destSpaceStr == "" {
			return pgtype.UUID{}, "", false, nil
		}
		// Attestation needs document_type + issuing_space_id. Parse from
		// guide_category and topic_prompt where possible; fall back if not.
		docType := visaCat
		if docType == "" {
			return pgtype.UUID{}, "", false, nil
		}
		dto, err := a.attestationSvc.GenerateDraft(ctx, tenantID, attestationguide.GeneratePathwayInput{
			DocumentType:       docType,
			IssuingSpaceID:     destSpaceStr,
			DestinationSpaceID: destSpaceStr,
			Prompt:             promptWithCtx,
		})
		if err != nil {
			slog.Warn("studio: attestation_guide routing failed, falling back to contents",
				"job_id", jobID, "error", err)
			return pgtype.UUID{}, "", false, nil
		}
		var id pgtype.UUID
		_ = id.Scan(dto.ID)
		return id, "attestation_pathways", true, nil

	case "language_test_requirement":
		if a.languageTestSvc == nil || destSpaceStr == "" {
			return pgtype.UUID{}, "", false, nil
		}
		// Language test needs test_slug. Parse from guide_category.
		testSlug := visaCat
		if testSlug == "" {
			return pgtype.UUID{}, "", false, nil
		}
		dto, err := a.languageTestSvc.GenerateDraft(ctx, tenantID, languagetesthub.GenerateRequirementRequest{
			DestinationSpaceID: destSpaceStr,
			VisaCategory:       visaCat,
			TestSlug:           testSlug,
		})
		if err != nil {
			slog.Warn("studio: language_test_requirement routing failed, falling back to contents",
				"job_id", jobID, "error", err)
			return pgtype.UUID{}, "", false, nil
		}
		var id pgtype.UUID
		_ = id.Scan(dto.ID)
		return id, "language_test_requirements", true, nil

	case "tax_obligation_guide":
		if a.taxSvc == nil || destSpaceStr == "" {
			return pgtype.UUID{}, "", false, nil
		}
		// Tax needs origin_space_id + employment_status. Use defaults.
		result, err := a.taxSvc.GenerateGuide(ctx, tenantID, taxcalculator.GenerateGuideRequest{
			OriginSpaceID:      destSpaceStr,
			DestinationSpaceID: destSpaceStr,
			EmploymentStatus:   "employed",
			Prompt:             promptWithCtx,
		})
		if err != nil {
			slog.Warn("studio: tax_obligation_guide routing failed, falling back to contents",
				"job_id", jobID, "error", err)
			return pgtype.UUID{}, "", false, nil
		}
		var id pgtype.UUID
		_ = id.Scan(result.GuideID)
		return id, "tax_obligation_guides", true, nil

	case "credential_pathway":
		if a.credentialSvc == nil || destSpaceStr == "" {
			return pgtype.UUID{}, "", false, nil
		}
		// Credential needs profession_id (UUID) — can't be mapped from
		// Studio job fields. Fall back to contents.
		return pgtype.UUID{}, "", false, nil

	default:
		// housing_guide, visa_route, wiki_page, calendar_entry: these
		// either have placeholder GenerateDraft methods or no draft
		// method. Fall back to contents creation.
		return pgtype.UUID{}, "", false, nil
	}
}

// routeToHITL marks the job as generated and creates a HITL review with
// the appropriate target_table (migration 000477, ADR-076 Amendment 1).
func (a *studioDraftProcessorAdapter) routeToHITL(
	ctx context.Context,
	tenantID, jobID, draftID pgtype.UUID,
	targetTable, contentType string,
	fvJSON []byte,
) {
	if err := a.studio.MarkJobGenerated(ctx, tenantID, jobID, draftID); err != nil {
		slog.Warn("studio: generate: mark generated failed",
			"job_id", jobID, "draft_id", draftID, "error", err)
		return
	}

	if a.aiStore == nil {
		return
	}

	review, rerr := a.aiStore.CreateHITLReview(ctx, ai.HITLReview{
		TenantID:    tenantID,
		ContentID:   draftID,
		ContentType: contentType,
		AIOutput:    fvJSON,
		Status:      ai.HITLStatusPending,
		Source:      ai.HITLSourceStudio,
		TargetTable: targetTable,
	})
	if rerr != nil {
		slog.Warn("studio: generate: create HITL review failed, draft remains in draft_ready",
			"job_id", jobID, "draft_id", draftID, "target_table", targetTable, "error", rerr)
		return
	}
	if err := a.studio.MarkJobRoutedToReview(ctx, tenantID, jobID, review.ID); err != nil {
		slog.Warn("studio: generate: mark routed to review failed",
			"job_id", jobID, "review_id", review.ID, "error", err)
	}
	slog.Info("studio: generate: draft routed to review",
		"job_id", jobID, "draft_id", draftID,
		"target_table", targetTable, "review_id", review.ID)
}

// studioGenerateBody synthesizes a draft body via the AI gateway, weaving in
// the persona voice prompt when present.
func (a *studioDraftProcessorAdapter) studioGenerateBody(ctx context.Context, tenantID pgtype.UUID, job studio.Job, persona gen.GetAIPersonaByIDRow, extractionContext string) (string, error) {
	prompt := fmt.Sprintf("Generate a %s draft about: %s", job.ContentType, job.TopicPrompt)
	if persona.VoicePrompt != nil && *persona.VoicePrompt != "" {
		prompt = fmt.Sprintf("Voice: %s\n\n%s", *persona.VoicePrompt, prompt)
	}
	if job.GuideCategory != nil && *job.GuideCategory != "" {
		prompt += fmt.Sprintf("\nGuide category: %s", *job.GuideCategory)
	}
	if extractionContext != "" {
		prompt += fmt.Sprintf("\n\nStructured data from external source (use as factual grounding for the article):\n%s", extractionContext)
	}
	resp, err := a.dispatcher.Dispatch(ctx, "content_generation", ai.Request{
		Category: "content_generation",
		Payload: map[string]any{
			"prompt":    prompt,
			"tenant_id": tenantID,
		},
	})
	if err != nil {
		return "", err
	}
	if text, ok := resp.Raw["text"].(string); ok {
		return text, nil
	}
	if text, ok := resp.Raw["content"].(string); ok {
		return text, nil
	}
	if output, ok := resp.Raw["output"].(string); ok {
		return output, nil
	}
	// Fallback: no recognized text field; return the topic prompt so the
	// draft is still created with meaningful content.
	return job.TopicPrompt, nil
}

// createStudioReview is a best-effort HITL review creation helper.
func (a *studioDraftProcessorAdapter) createStudioReview(ctx context.Context, tenantID, contentID pgtype.UUID, contentType string, fvJSON []byte) {
	_, err := a.aiStore.CreateHITLReview(ctx, ai.HITLReview{
		TenantID:    tenantID,
		ContentID:   contentID,
		ContentType: contentType,
		AIOutput:    fvJSON,
		Status:      ai.HITLStatusPending,
		Source:      ai.HITLSourceStudio,
	})
	if err != nil {
		slog.Warn("studio: create HITL review failed", "content_id", contentID, "error", err)
	}
}
