// Package server — agent_adapters.go provides the wiring-layer adapters that
// bridge the AI Agent Executor's (Section 103, ADR-111) narrow dependency
// interfaces to the real service implementations.
// // The adapters exist because:
// 1. The ai package's CacheClient uses plain (string, error) returns while
// valkeycompat.Cmdable uses go-redis command types — a thin adapter
// resolves the signature mismatch without polluting the ai package
// with valkeycompat imports.
// 2. The ai package's RAGService returns []ai.RAGChunk (a mirror type)
// to avoid an import cycle with internal/rag — the adapter converts
// rag.RetrievedChunk to ai.RAGChunk.
// 3. The ContentCreator/ContentUpdater function bridges adapt
// content.Service.CreateContent/UpdateContent into the ai package's
// provider-agnostic input/output types, avoiding an import cycle
// between internal/ai and internal/content.
// // Ref: japaconnect-engineering/sections/103-ai-agent-executor.md
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
	"github.com/jedafe/japa/internal/calendar"
	"github.com/jedafe/japa/internal/campaign"
	"github.com/jedafe/japa/internal/communitylocator"
	"github.com/jedafe/japa/internal/comparison"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/credentialchecker"
	"github.com/jedafe/japa/internal/customfields"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/docs"
	"github.com/jedafe/japa/internal/housingguide"
	"github.com/jedafe/japa/internal/languagetesthub"
	"github.com/jedafe/japa/internal/learning"
	"github.com/jedafe/japa/internal/menus"
	"github.com/jedafe/japa/internal/personas"
	"github.com/jedafe/japa/internal/rag"
	"github.com/jedafe/japa/internal/scholarships"
	"github.com/jedafe/japa/internal/story"
	"github.com/jedafe/japa/internal/studio"
	"github.com/jedafe/japa/internal/taxcalculator"
	"github.com/jedafe/japa/internal/users"
	"github.com/jedafe/japa/internal/visaroutes"
	"github.com/jedafe/japa/internal/wiki"
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
	var guideCategory *string
	if input.GuideCategory != "" {
		guideCategory = &input.GuideCategory
	}

	row, err := b.svc.CreateContent(ctx, tenantID, content.CreateContentInput{
		AuthorID:       input.AuthorID,
		OrganizationID: input.OrganizationID,
		SpaceID:        input.SpaceID,
		ContentType:    input.ContentType,
		Title:          input.Title,
		Body:           input.Body,
		FieldValues:    input.FieldValues,
		GeoSpaceID:     input.GeoSpaceID,
		Status:         input.Status,
		Visibility:     "public",
		SeoIndexable:   true,
		PublishAt:      input.PublishAt,
		GuideCategory:  guideCategory,
	})
	if err != nil {
		return ai.ContentCreateRow{}, err
	}

	return ai.ContentCreateRow{
		ID:             row.ID,
		Title:          row.Title,
		Slug:           row.Slug,
		Status:         row.Status,
		ContentType:    row.ContentType,
		AuthorID:       row.AuthorID,
		OrganizationID: row.OrganizationID,
		GeoSpaceID:     row.GeoSpaceID,
		CreatedAt:      row.CreatedAt,
		PublishAt:      row.PublishAt,
		GuideCategory:  derefStr(row.GuideCategory),
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
// // Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Comparison Engine)
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 7 wiring).
type agentComparisonAdapter struct {
	svc *comparison.Service
}

func (a *agentComparisonAdapter) ListComparisonTypes(ctx context.Context, limit, offset int32) ([]gen.ListComparisonTypesByTenantRow, error) {
	return a.svc.ListComparisonTypes(ctx, limit, offset)
}

func (a *agentComparisonAdapter) GetComparisonTypeBySlug(ctx context.Context, slug string) (gen.GetComparisonTypeBySlugRow, error) {
	return a.svc.GetComparisonTypeBySlug(ctx, slug)
}

func (a *agentComparisonAdapter) CreateComparisonType(ctx context.Context, input ai.ComparisonTypeCreateInput) (gen.CreateComparisonTypeRow, error) {
	return a.svc.CreateComparisonType(ctx, comparison.CreateComparisonTypeInput{
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
	// If the caller supplied a JCE depth or polymorphic item_refs, route to
	// the JCE service so the comparison content metadata and resolver hooks
	// are created. Otherwise, fall back to the legacy generic create flow.
	usesJCE := input.Depth == comparison.DepthQuick || input.Depth == comparison.DepthDetailed
	if !usesJCE {
		for _, it := range input.Items {
			if len(it.ItemRef) > 0 {
				usesJCE = true
				break
			}
		}
	}

	if usesJCE {
		items := make([]comparison.JCEItemInput, 0, len(input.Items))

		for i, it := range input.Items {
			var refBytes []byte

			if len(it.ItemRef) > 0 {
				b, err := json.Marshal(it.ItemRef)
				if err != nil {
					return ai.ComparisonCreateResult{}, fmt.Errorf("marshaling item_ref for item %d: %w", i, err)
				}

				refBytes = b
			}

			items = append(items, comparison.JCEItemInput{
				Title:    it.Name,
				TypeID:   input.TypeID,
				Position: int32(i), //nolint:gosec // G115: bounded
				ItemRef:  refBytes,
			})
		}

		res, err := a.svc.CreateJCEComparison(ctx, userID, comparison.CreateJCEComparisonInput{
			Title:      input.Title,
			Body:       input.Body,
			Visibility: input.Visibility,
			Status:     input.Status,
			SpaceID:    input.SpaceID,
			TypeID:     input.TypeID,
			Depth:      input.Depth,
			Items:      items,
			Dimensions: input.Dimensions,
		})
		if err != nil {
			return ai.ComparisonCreateResult{}, err
		}

		return ai.ComparisonCreateResult{
			ID: res.ID,

			AuthorID:   res.AuthorID,
			Title:      res.Title,
			Slug:       res.Slug,
			Visibility: res.Visibility,
			SpaceID:    res.SpaceID,
			Depth:      res.Depth,
		}, nil
	}

	items := make([]comparison.ComparisonItemInput, 0, len(input.Items))
	for _, it := range input.Items {
		items = append(items, comparison.ComparisonItemInput{Name: it.Name, Slug: it.Slug})
	}

	res, err := a.svc.CreateComparison(ctx, userID, comparison.CreateComparisonInput{
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
		ID: res.ID,

		AuthorID:   res.AuthorID,
		Title:      res.Title,
		Slug:       res.Slug,
		Visibility: res.Visibility,
		SpaceID:    res.SpaceID,
	}, nil
}

func (a *agentComparisonAdapter) GetComparisonDetail(ctx context.Context, id pgtype.UUID) (ai.ComparisonDetail, error) {
	d, err := a.svc.GetComparisonDetail(ctx, id)
	if err != nil {
		return ai.ComparisonDetail{}, err
	}

	return ai.ComparisonDetail{
		ID: d.ID,

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

func (a *agentComparisonAdapter) GetComparisonMatrix(ctx context.Context, contentID pgtype.UUID) ([]ai.ComparisonMatrixItem, error) {
	matrix, err := a.svc.GetComparisonMatrix(ctx, contentID)
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

func (a *agentComparisonAdapter) CreateComparisonDimension(ctx context.Context, input ai.ComparisonDimensionCreateInput) (gen.CreateComparisonDimensionRow, error) {
	return a.svc.CreateComparisonDimension(ctx, comparison.CreateComparisonDimensionInput{
		ContentID:     input.ContentID,
		Label:         input.Label,
		DimensionType: input.DimensionType,
		Position:      input.Position,
	})
}

func (a *agentComparisonAdapter) UpsertComparisonValueWithMetadata(ctx context.Context, input ai.ComparisonValueUpsertInput, metadata []byte) (gen.ComparisonValue, error) {
	return a.svc.UpsertComparisonValueWithMetadata(ctx, comparison.UpsertComparisonValueInput{
		DimensionID:  input.DimensionID,
		ItemID:       input.ItemID,
		Value:        input.Value,
		ValueRating:  input.ValueRating,
		ValueBoolean: input.ValueBoolean,
	}, metadata)
}

func (a *agentComparisonAdapter) DeleteJCEComparisonCaches(ctx context.Context, contentID, tenantID pgtype.UUID) {
	a.svc.DeleteJCEComparisonCaches(ctx, contentID, tenantID)
}

// ---------------------------------------------------------------------------
// Phase 8 adapters — persona resolver + agent draft processor
// ---------------------------------------------------------------------------

// agentPersonaAdapter adapts personas.Store.GetAIPersonaByID to the
// ai.PersonaResolver interface. The ai package uses a narrow interface so
// it does not import internal/personas (avoids a cycle risk). The adapter
// returns only the backing users.id the content.create_draft tool needs.
// // Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Content Request Flow)
// Ref: japaconnect-engineering/sections/68-ai-personas.md (ai_personas.user_id).
type agentPersonaAdapter struct {
	store personaUserIDResolver
}

// personaUserIDResolver is the subset of *personas.Store the adapter calls.
// Defined as an interface so the adapter is testable with a stub.
type personaUserIDResolver interface {
	GetAIPersonaByID(ctx context.Context, id pgtype.UUID) (gen.GetAIPersonaByIDRow, error)
	GetPersonaVoiceByID(ctx context.Context, id pgtype.UUID) (gen.PersonaVoice, error)
}

// GetPersonaUserID returns the backing users.id for the given persona ID.
func (a *agentPersonaAdapter) GetPersonaUserID(ctx context.Context, personaID pgtype.UUID) (pgtype.UUID, error) {
	row, err := a.store.GetAIPersonaByID(ctx, personaID)
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
// // Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Content Request Flow, Asynq path)
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 8, task 8.03).
// hitlBypassChecker reports whether an actor holds the agent.skip_hitl
// permission node. Holders (super_admin via the '*' wildcard, plus any
// role/group granted the node) bypass the HITL review queue for
// agent-initiated content actions.
type hitlBypassChecker interface {
	HasPermission(ctx context.Context, userID pgtype.UUID, permKey string) bool
}

type agentDraftProcessorAdapter struct {
	contentSvc   *content.Service
	aiStore      *ai.Store
	personaStore personaUserIDResolver
	dispatcher   agentDraftDispatcher
	perms        hitlBypassChecker
}

// agentDraftDispatcher is the subset of *ai.Dispatcher the adapter calls to
// generate the draft body from the topic prompt + external results.
type agentDraftDispatcher interface {
	// GenerateDraftBody synthesizes a draft body from the topic prompt +
	// external search results. Returns markdown body text.
	GenerateDraftBody(ctx context.Context, contentType, topicPrompt, externalResults string) (string, error)
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
func (a *agentDraftDispatcherAdapter) GenerateDraftBody(ctx context.Context, contentType, topicPrompt, externalResults string) (string, error) {
	prompt := fmt.Sprintf("Generate a %s draft about: %s", contentType, topicPrompt)
	if externalResults != "" {
		prompt += "\n\nExternal reference material:\n" + externalResults
	}

	resp, err := a.dispatcher.Dispatch(ctx, "content_generation", ai.Request{
		Category: "content_generation",
		Payload: map[string]any{
			"prompt": prompt,
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
// 1. Resolves the persona's backing users.id (the author).
// 2. Generates the draft body via the AI dispatcher (synthesizes the
// topic prompt + external results).
// 3. Creates the contents row with status='draft', author_id=persona
// user_id, and _ai_provenance in field_values.
// 4. Creates a HITL review with source='agent_draft_request' + the
// created content_id.
func (a *agentDraftProcessorAdapter) CreateAgentDraft(ctx context.Context, payload worker.AgentDraftPayload) error {
	tenantID := pgtype.UUID{}

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
	persona, err := a.personaStore.GetAIPersonaByID(ctx, personaID)
	if err != nil {
		return fmt.Errorf("resolving persona: %w", err)
	}

	if payload.ContentType == "" {
		return errors.New("content_type is required")
	}

	// 2. Generate the draft body via the AI dispatcher.
	body := payload.TopicPrompt

	if a.dispatcher != nil {
		generated, gerr := a.dispatcher.GenerateDraftBody(ctx, payload.ContentType, payload.TopicPrompt, payload.ExternalResults)
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

	// 5. Actors holding agent.skip_hitl bypass the review queue: publish the
	// draft directly. If publish fails (e.g. no primary space was provided),
	// fall back to creating a HITL review so a human can resolve it.
	if a.perms != nil && userID.Valid &&
		a.perms.HasPermission(ctx, userID, "agent.skip_hitl") {
		if _, perr := a.contentSvc.UpdateContentStatus(ctx, row.ID, "published"); perr == nil {
			slog.Info("agent draft: auto-published via agent.skip_hitl", "content_id", row.ID, "user_id", userID)
			return nil
		} else {
			slog.Warn("agent draft: skip_hitl publish failed, routing to HITL", "content_id", row.ID, "error", perr)
		}
	}

	// 6. Create a HITL review with source='agent_draft_request'.
	if a.aiStore != nil {
		_, hitlErr := a.aiStore.CreateHITLReview(ctx, ai.HITLReview{

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
// // learn.lessons.reorder has no backing Store method — the adapter uses
// UpdateLesson per entry to set the new sort_order. This is sequential
// (not a single bulk SQL UPDATE) but correct; a future migration can add
// a bulk reorder query if performance requires it.
type agentLearningAdapter struct{ store *learning.Store }

func (a *agentLearningAdapter) ListTracks(ctx context.Context) ([]ai.LearningTrack, error) {
	tracks, err := a.store.ListAllTracks(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]ai.LearningTrack, 0, len(tracks))
	for _, t := range tracks {
		out = append(out, learningTrackToAI(t))
	}

	return out, nil
}

func (a *agentLearningAdapter) CreateTrack(ctx context.Context, input ai.LearningCreateTrackInput) (*ai.LearningTrack, error) {
	t, err := a.store.CreateTrack(ctx, learning.CreateTrackInput{
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

func (a *agentLearningAdapter) UpdateTrack(ctx context.Context, id pgtype.UUID, input ai.LearningUpdateTrackInput) (*ai.LearningTrack, error) {
	t, err := a.store.UpdateTrack(ctx, id, learning.UpdateTrackInput{
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

func (a *agentLearningAdapter) ListLessons(ctx context.Context) ([]ai.LearningLesson, error) {
	lessons, err := a.store.ListAllLessons(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]ai.LearningLesson, 0, len(lessons))
	for _, l := range lessons {
		out = append(out, learningLessonToAI(l))
	}

	return out, nil
}

func (a *agentLearningAdapter) CreateLesson(ctx context.Context, moduleID pgtype.UUID, input ai.LearningCreateLessonInput) (*ai.LearningLesson, error) {
	l, err := a.store.CreateLesson(ctx, moduleID, learning.CreateLessonInput{
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

func (a *agentLearningAdapter) UpdateLesson(ctx context.Context, id pgtype.UUID, input ai.LearningUpdateLessonInput) (*ai.LearningLesson, error) {
	l, err := a.store.UpdateLesson(ctx, id, learning.UpdateLessonInput{
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
func (a *agentLearningAdapter) ReorderLessons(ctx context.Context, entries []ai.LearningReorderEntry) error {
	for _, e := range entries {
		// Fetch the existing lesson to preserve its fields, then update
		// only the sort_order. UpdateLessonInput requires Title and
		// LessonType, so we must read first.
		lesson, err := a.store.GetLessonByID(ctx, e.ID)
		if err != nil {
			return fmt.Errorf("reorder: fetching lesson %s: %w", e.ID, err)
		}

		_, err = a.store.UpdateLesson(ctx, e.ID, learning.UpdateLessonInput{
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
		ID: t.ID,

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
		ID: l.ID,

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

func (a *agentDocsAdapter) ListAllDocPages(ctx context.Context) ([]ai.DocsPage, error) {
	pages, err := a.svc.ListAllDocPages(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]ai.DocsPage, 0, len(pages))
	for _, p := range pages {
		out = append(out, docsPageToAI(p))
	}

	return out, nil
}

func (a *agentDocsAdapter) GetDocPageByID(ctx context.Context, id pgtype.UUID) (*ai.DocsPage, error) {
	p, err := a.svc.GetDocPageByID(ctx, id)
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

func (a *agentMenusAdapter) ListMenus(ctx context.Context, q, placement, status, sort, sortDir string, limit, offset int32) (ai.MenusListResponse, error) {
	resp, err := a.svc.ListMenus(ctx, q, placement, status, sort, sortDir, limit, offset)
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

func (a *agentMenusAdapter) CreateMenuItem(ctx context.Context, input ai.MenusCreateMenuItemInput) (gen.CreateMenuItemRow, error) {
	return a.svc.CreateMenuItem(ctx, menus.CreateMenuItemInput{
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

func (a *agentMenusAdapter) UpdateMenuItem(ctx context.Context, id pgtype.UUID, input ai.MenusUpdateMenuItemInput) (gen.UpdateMenuItemRow, error) {
	return a.svc.UpdateMenuItem(ctx, id, menus.UpdateMenuItemInput{
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

func (a *agentMenusAdapter) DeleteMenuItem(ctx context.Context, id pgtype.UUID) error {
	return a.svc.DeleteMenuItem(ctx, id)
}

// agentUsersAdapter bridges users.Service to the ai.UsersService interface.
// The ai package uses mirror types (ai.UserProfile,
// ai.UserProfileUpdateRequest) to avoid an import cycle with
// internal/users. The adapter strips PII from UserProfileResponse.
type agentUsersAdapter struct{ svc *users.Service }

func (a *agentUsersAdapter) GetProfile(ctx context.Context, userID pgtype.UUID) (*ai.UserProfile, error) {
	p, err := a.svc.GetProfile(ctx, userID)
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

func (a *agentUsersAdapter) UpdateProfile(ctx context.Context, userID pgtype.UUID, req *ai.UserProfileUpdateRequest) error {
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

	return a.svc.UpdateProfile(ctx, userID, uReq)
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
// load extraction context for Studio generation (Approach B) and to
// transition linked intel_records to 'published' on successful publish (§75).
type intelRecordLoader interface {
	GetIntelRecord(ctx context.Context, id pgtype.UUID) (gen.IntelRecord, error)
	UpdateIntelRecordStatus(ctx context.Context, arg gen.UpdateIntelRecordStatusParams) error
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
	arrivalSvc      arrivalDraftGenerator
	communityOrgSvc communityOrgDraftGenerator
	attestationSvc  attestationDraftGenerator
	languageTestSvc languageTestDraftGenerator
	taxSvc          taxDraftGenerator
	credentialSvc   credentialDraftGenerator
	housingSvc      housingDraftGenerator
	visaRouteSvc    visaRouteDraftGenerator
	wikiSvc         wikiDraftGenerator
	calendarSvc     calendarDraftGenerator
	docsSvc         docsDraftGenerator
	storySvc        storyDraftGenerator
	campaignSvc     campaignItemSyncer
	perms           hitlBypassChecker
}

// campaignItemSyncer is the subset of the campaign service the Studio processor
// uses to keep campaign_items in sync with their Studio jobs.
type campaignItemSyncer interface {
	SyncCampaignItemByStudioJob(ctx context.Context, studioJobID pgtype.UUID, status string, contentID, reviewID pgtype.UUID, errMsg *string) error
}

// arrivalDraftGenerator is the subset of *arrivalchecklist.Service needed
// for Studio routing (Approach B).
type arrivalDraftGenerator interface {
	GenerateGuide(ctx context.Context, req arrivalchecklist.GenerateGuideRequest) (arrivalchecklist.GenerateGuideResponse, error)
}

// communityOrgDraftGenerator is the subset of *communitylocator.Service.
type communityOrgDraftGenerator interface {
	Generate(ctx context.Context, generatedBy pgtype.UUID, input communitylocator.GenerateInput) (communitylocator.OrgDTO, error)
}

// attestationDraftGenerator is the subset of *attestationguide.Service.
type attestationDraftGenerator interface {
	GenerateDraft(ctx context.Context, tenantID pgtype.UUID, input attestationguide.GeneratePathwayInput) (attestationguide.PathwayDTO, error)
}

// languageTestDraftGenerator is the subset of *languagetesthub.Service.
type languageTestDraftGenerator interface {
	GenerateDraft(ctx context.Context, input languagetesthub.GenerateRequirementRequest) (languagetesthub.RequirementDTO, error)
}

// taxDraftGenerator is the subset of *taxcalculator.Service.
type taxDraftGenerator interface {
	GenerateGuide(ctx context.Context, req taxcalculator.GenerateGuideRequest) (taxcalculator.GenerateGuideResult, error)
}

// credentialDraftGenerator is the subset of *credentialchecker.Service.
type credentialDraftGenerator interface {
	GeneratePathwayDraft(ctx context.Context, req credentialchecker.GeneratePathwayRequest) (credentialchecker.GeneratePathwayResponse, error)
	GetProfessionBySlug(ctx context.Context, slug string) (credentialchecker.ProfessionRow, error)
}

// housingDraftGenerator is the subset of *housingguide.Service needed for
// Studio routing (Approach B, ADR-076 Amendment 1).
type housingDraftGenerator interface {
	GenerateDraft(ctx context.Context, input housingguide.GenerateGuideRequest) (housingguide.GenerateDraftResponse, error)
}

// visaRouteDraftGenerator is the subset of *visaroutes.Service.
type visaRouteDraftGenerator interface {
	GenerateDraft(ctx context.Context, input visaroutes.GenerateRouteRequest) (visaroutes.GenerateDraftResponse, error)
}

// wikiDraftGenerator is the subset of *wiki.Service.
type wikiDraftGenerator interface {
	GenerateDraft(ctx context.Context, input wiki.GenerateDraftRequest) (wiki.GenerateDraftResponse, error)
}

// calendarDraftGenerator is the subset of *calendar.Service.
type calendarDraftGenerator interface {
	GenerateDraft(ctx context.Context, input calendar.GenerateDraftRequest) (calendar.GenerateDraftResponse, error)
}

// docsDraftGenerator is the subset of *docs.Service.
type docsDraftGenerator interface {
	GenerateDraft(ctx context.Context, tenantID pgtype.UUID, input docs.GenerateDraftRequest) (docs.GenerateDraftResponse, error)
}

// storyDraftGenerator is the subset of *story.Service. Story creates both a
// contents row and a story_meta row, so it needs the persona's backing
// user_id as the author.
type storyDraftGenerator interface {
	GenerateDraft(ctx context.Context, tenantID, authorID pgtype.UUID, input story.GenerateDraftRequest) (story.GenerateDraftResponse, error)
}

// toolTableTargetTable maps tool-table content types to their backing
// PostgreSQL table name for HITL review routing (migration 000477).
// #nosec G101 — content type → table name mapping, not credentials.
var toolTableTargetTable = map[string]string{
	"housing_guide":             "housing_guides",
	"attestation_guide":         "attestation_pathways",
	"visa_route":                "visa_routes",
	"arrival_checklist":         "arrival_checklist_guides",
	"credential_pathway":        "credential_pathways",
	"language_test_requirement": "language_test_requirements",
	"tax_obligation_guide":      "tax_obligation_guides",
	"community_organisation":    "community_organisations",
	"wiki_page":                 "wiki_pages",
	"calendar_entry":            "calendar_entries",
	"doc_page":                  "doc_pages",
	"story":                     "story_meta",
}

// isToolTableType returns true if the content type should route to a
// dedicated tool table instead of the contents table.
func isToolTableType(contentType string) bool {
	_, ok := toolTableTargetTable[contentType]
	return ok
}

// studioProcessorService is the subset of *studio.Service the adapter calls.
type studioProcessorService interface {
	GetJob(ctx context.Context, id pgtype.UUID) (studio.Job, error)
	DefaultPersonaID(ctx context.Context) (pgtype.UUID, error)
	MarkJobGenerating(ctx context.Context, jobID pgtype.UUID) error
	MarkJobGenerated(ctx context.Context, jobID, draftContentID pgtype.UUID, aiModel string) error
	MarkJobRoutedToReview(ctx context.Context, jobID, reviewID pgtype.UUID) error
	MarkJobFailed(ctx context.Context, jobID pgtype.UUID, errMsg string) error
	MarkJobPublished(ctx context.Context, jobID, draftContentID pgtype.UUID, aiModel string) error
	ShouldAutoPublish(ctx context.Context, contentType, guideCategory string) bool
	RunBatch(ctx context.Context, tenantID, batchID pgtype.UUID) error
	RetryFailed(ctx context.Context, jobID pgtype.UUID) error
	RetryBatchFailed(ctx context.Context, batchID pgtype.UUID) (int, error)
}

// RunBatch delegates to the studio service.
func (a *studioDraftProcessorAdapter) RunBatch(ctx context.Context, tenantID, batchID pgtype.UUID) error {
	return a.studio.RunBatch(ctx, tenantID, batchID)
}

// RetryFailed delegates to the studio service.
func (a *studioDraftProcessorAdapter) RetryFailed(ctx context.Context, jobID pgtype.UUID) error {
	return a.studio.RetryFailed(ctx, jobID)
}

// RetryBatchFailed delegates to the studio service for batch-level retry.
func (a *studioDraftProcessorAdapter) RetryBatchFailed(ctx context.Context, batchID pgtype.UUID) (int, error) {
	return a.studio.RetryBatchFailed(ctx, batchID)
}

// GenerateJob runs the §75 single-job generation pipeline:
// 1. Load the studio job + persona (voice prompt + backing author).
// 2. Generate the draft body via the AI gateway (content_generation).
// 3. Create the contents row with status='draft', author=persona user_id.
// 4. Route to the HITL review queue with source='studio', OR auto-publish
// low-stakes types per studio.auto_publish_types (§75).
// 5. Update the studio job to 'in_review' (or 'published').
func (a *studioDraftProcessorAdapter) GenerateJob(ctx context.Context, tenantID, jobID pgtype.UUID) error {
	job, err := a.studio.GetJob(ctx, jobID)
	if err != nil {
		return fmt.Errorf("studio: generate: get job: %w", err)
	}
	// Guard against re-processing a job that already produced a draft.
	if job.Status != "queued" && job.Status != "generating" {
		slog.Warn("studio: generate: job not in a generatable state, skipping", "job_id", jobID, "status", job.Status)
		return nil
	}

	// 1. Mark generating (compare-and-swap: only succeeds if status is still
	// 'queued'). If another worker already claimed this job, skip silently —
	// this prevents duplicate draft generation from concurrent tasks.
	if err := a.studio.MarkJobGenerating(ctx, jobID); err != nil {
		if errors.Is(err, studio.ErrJobAlreadyGenerating) {
			slog.Info("studio: generate: job already claimed by another worker, skipping", "job_id", jobID)
			return nil
		}

		return fmt.Errorf("studio: generate: mark generating: %w", err)
	}

	// Idempotency re-check after claiming: the job row was read before the
	// CAS, so a redelivered or retried task may find that a prior run already
	// produced a draft/review. Re-running CreateContent would hit the
	// content_hash dedup and (previously) regress the job to 'failed'.
	if fresh, ferr := a.studio.GetJob(ctx, jobID); ferr == nil &&
		(fresh.DraftContentID.Valid || fresh.ReviewID.Valid) {
		slog.Info("studio: generate: job already produced a draft, skipping", "job_id", jobID, "draft_content_id", fresh.DraftContentID, "review_id", fresh.ReviewID)
		return nil
	}

	// 2. Resolve the persona's backing user (the author) + voice prompt.
	persona, err := a.personaStore.GetAIPersonaByID(ctx, job.PersonaID)
	if err != nil {
		msg := "persona not found: " + err.Error()
		_ = a.studio.MarkJobFailed(ctx, jobID, msg)
		a.syncCampaignItem(ctx, jobID, "failed", pgtype.UUID{}, pgtype.UUID{}, &msg)

		return fmt.Errorf("studio: generate: resolve persona: %w", err)
	}

	// Approach B: load intel_record extraction as factual grounding context.
	var extractionContext string

	if job.IntelRecordID.Valid && a.intelStore != nil {
		record, rerr := a.intelStore.GetIntelRecord(ctx, job.IntelRecordID)
		if rerr == nil && len(record.Extracted) > 0 {
			extractionContext = string(record.Extracted)
		} else if rerr != nil {
			slog.Warn("studio: generate: load intel_record failed, generating without extraction context", "job_id", jobID, "intel_record_id", job.IntelRecordID, "error", rerr)
		}
	}

	// 3. Generate the draft body via the AI gateway.
	body := job.TopicPrompt
	aiModel := ""

	if a.dispatcher != nil {
		generated, model, gerr := a.studioGenerateBody(ctx, job, persona, extractionContext)
		if gerr != nil {
			msg := "AI generation failed: " + gerr.Error()
			_ = a.studio.MarkJobFailed(ctx, jobID, msg)
			a.syncCampaignItem(ctx, jobID, "failed", pgtype.UUID{}, pgtype.UUID{}, &msg)

			return fmt.Errorf("studio: generate: body: %w", gerr)
		}

		if generated != "" {
			body = generated
		}

		aiModel = model
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
		msg := "encode field values: " + err.Error()
		_ = a.studio.MarkJobFailed(ctx, jobID, msg)
		a.syncCampaignItem(ctx, jobID, "failed", pgtype.UUID{}, pgtype.UUID{}, &msg)

		return fmt.Errorf("studio: generate: encode field values: %w", err)
	}

	// 5. Route to the appropriate table based on content type.
	// Tool-table types (Approach B, ADR-076 Amendment 1) route to their
	// dedicated tool service. If the tool service call fails (e.g. missing
	// required fields not available in the Studio job), fall through to
	// contents creation as a safe default.
	if isToolTableType(job.ContentType) {
		if draftID, targetTable, routed, rerr := a.tryToolTableRouting(ctx, tenantID, jobID, job, persona, body, extractionContext); rerr == nil && routed {
			a.routeToHITL(ctx, jobID, draftID, targetTable, job.ContentType, aiModel, fvJSON)
			return nil
		}

		slog.Info("studio: generate: tool table routing fell back to contents", "job_id", jobID, "content_type", job.ContentType)
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
		msg := "create content: " + err.Error()
		_ = a.studio.MarkJobFailed(ctx, jobID, msg)
		a.syncCampaignItem(ctx, jobID, "failed", pgtype.UUID{}, pgtype.UUID{}, &msg)

		return fmt.Errorf("studio: generate: create content: %w", err)
	}

	// 6. Route to HITL review (source='studio') OR auto-publish low-stakes.
	// Jobs created by actors holding agent.skip_hitl bypass the review queue
	// and publish directly (same code path as auto-publish — a publish
	// failure still falls back to HITL below).
	skipHITL := a.perms != nil && job.CreatedBy.Valid &&
		a.perms.HasPermission(ctx, job.CreatedBy, "agent.skip_hitl")
	if skipHITL || a.studio.ShouldAutoPublish(ctx, job.ContentType, derefStr(job.GuideCategory)) {
		if _, perr := a.contentSvc.UpdateContentStatus(ctx, row.ID, "published"); perr != nil {
			slog.Warn("studio: generate: auto-publish failed, routing to review", "job_id", jobID, "content_id", row.ID, "error", perr)
			a.routeToHITL(ctx, jobID, row.ID, "contents", job.ContentType, aiModel, fvJSON)

			return nil
		}

		if err := a.studio.MarkJobPublished(ctx, jobID, row.ID, aiModel); err != nil {
			slog.Warn("studio: generate: mark job published failed", "job_id", jobID, "error", err)
		} else {
			a.syncCampaignItem(ctx, jobID, "published", row.ID, pgtype.UUID{}, nil)
		}
		// Transition the linked intel_record to 'published' (§75: "On
		// successful publish the linked intel_records row transitions to
		// status='published'").
		a.transitionIntelRecordOnPublish(ctx, job.IntelRecordID)
		slog.Info("studio: generate: auto-published", "job_id", jobID, "content_id", row.ID)

		return nil
	}

	// Step 7 (ADR-076 §2): mark draft_ready with the draft_content_id.
	if err := a.studio.MarkJobGenerated(ctx, jobID, row.ID, aiModel); err != nil {
		return fmt.Errorf("studio: generate: mark generated: %w", err)
	}

	a.syncCampaignItem(ctx, jobID, "draft_ready", row.ID, pgtype.UUID{}, nil)

	// Step 8 (ADR-076 §2): route to the HITL review queue.
	// NOTE: this path is the contents-creation fallback. Tool-table types
	// that successfully routed to their dedicated table returned early
	// above (via tryToolTableRouting → routeToHITL). When we reach here
	// the draft row lives in `contents`, so target_table must always be
	// 'contents' — even for tool-table content types that fell back.
	a.routeToHITL(ctx, jobID, row.ID, "contents", job.ContentType, aiModel, fvJSON)

	return nil
}

// RegenerateFromSource rewrites an existing contents row from the latest
// intel_record extraction (§75, ADR-076 Amendment 2). It preserves the
// content's identity and slug; if the live body has been manually edited or
// the type requires HITL, it creates a new draft instead of overwriting.
func (a *studioDraftProcessorAdapter) RegenerateFromSource(ctx context.Context, tenantID, contentID, intelRecordID pgtype.UUID) error {
	existing, err := a.contentSvc.GetContent(ctx, contentID)
	if err != nil {
		return fmt.Errorf("regenerate: get content: %w", err)
	}

	record, err := a.intelStore.GetIntelRecord(ctx, intelRecordID)
	if err != nil {
		return fmt.Errorf("regenerate: get intel_record: %w", err)
	}

	persona, personaID, err := a.resolveRegenerationPersona(ctx, existing)
	if err != nil {
		return fmt.Errorf("regenerate: resolve persona: %w", err)
	}

	extractionContext := string(record.Extracted)
	job := studio.Job{
		ContentType:   existing.ContentType,
		GuideCategory: existing.GuideCategory,
		TopicPrompt:   existing.Title,
	}

	body := existing.Title
	aiModel := ""

	if a.dispatcher != nil {
		generated, model, gerr := a.studioGenerateBody(ctx, job, persona, extractionContext)
		if gerr != nil {
			return fmt.Errorf("regenerate: generate body: %w", gerr)
		}

		if generated != "" {
			body = generated
		}

		aiModel = model
	}

	provenance := map[string]any{
		"ai_generated":        true,
		"ai_persona_id":       personaID.String(),
		"ai_model":            aiModel,
		"source":              "regenerate_from_source",
		"original_prompt":     existing.Title,
		"intel_record_id":     intelRecordID.String(),
		"previous_content_id": contentID.String(),
	}
	fieldValues := map[string]any{"_ai_provenance": provenance}

	fvJSON, err := json.Marshal(fieldValues)
	if err != nil {
		return fmt.Errorf("regenerate: encode field values: %w", err)
	}

	manual, err := a.hasManualEditSinceAIGeneration(ctx, contentID, existing.AiGeneratedAt, persona.UserID)
	if err != nil {
		return fmt.Errorf("regenerate: check manual edits: %w", err)
	}

	guideCategory := derefStr(existing.GuideCategory)
	highStakes := studio.IsHighStakes(guideCategory)
	canAutoPublish := a.studio.ShouldAutoPublish(ctx, existing.ContentType, guideCategory)

	// If the content is published and either HITL is required or a human has
	// edited the body since the last AI generation, create a new draft for
	// review rather than overwriting the live article.
	needsHITL := highStakes || manual || (existing.Status == content.StatusPublished && !canAutoPublish)

	if needsHITL {
		draft, err := a.contentSvc.CreateContent(ctx, tenantID, content.CreateContentInput{
			AuthorID:      persona.UserID,
			SpaceID:       existing.GeoSpaceID,
			ContentType:   existing.ContentType,
			Title:         existing.Title,
			Body:          body,
			FieldValues:   fvJSON,
			Status:        content.StatusDraft,
			GuideCategory: existing.GuideCategory,
		})
		if err != nil {
			return fmt.Errorf("regenerate: create HITL draft: %w", err)
		}

		a.createStudioReview(ctx, draft.ID, existing.ContentType, fvJSON)
		slog.Info("regenerate: created HITL draft for manual edit or HITL-required content", "content_id", contentID, "draft_id", draft.ID, "high_stakes", highStakes, "manual_edit", manual)

		return nil
	}

	// In-place update. Compute content hash and stamp ai_generated columns.
	hash := content.ComputeContentHash(body)
	now := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}

	updated, err := a.contentSvc.UpdateContent(ctx, contentID, tenantID, persona.UserID, content.UpdateContentInput{
		Title:         existing.Title,
		Slug:          existing.Slug,
		Body:          body,
		BodyHtml:      "",
		FieldValues:   fvJSON,
		ContentHash:   &hash,
		GeoSpaceID:    existing.GeoSpaceID,
		SeoIndexable:  &existing.SeoIndexable,
		Status:        existing.Status,
		Visibility:    existing.Visibility,
		GuideCategory: existing.GuideCategory,
		AIGeneratedAt: now,
		AIModel:       &aiModel,
	}, true)
	if err != nil {
		return fmt.Errorf("regenerate: update content: %w", err)
	}

	// Transition linked intel_record to 'published' when the regenerated
	// content is live (mirrors Studio publish flow).
	if existing.Status == content.StatusPublished {
		a.transitionIntelRecordOnPublish(ctx, intelRecordID)
	}

	slog.Info("regenerate: updated content from source", "content_id", contentID, "slug", updated.Slug, "ai_model", aiModel)

	return nil
}

// resolveRegenerationPersona picks the persona to use for a regeneration.
// Preference: field_values._ai_provenance.ai_persona_id, then the tenant's
// studio.default_persona_id setting. The returned persona row carries the
// backing user_id that will be stamped as the author.
func (a *studioDraftProcessorAdapter) resolveRegenerationPersona(ctx context.Context, existing gen.GetContentByIDRow) (gen.GetAIPersonaByIDRow, pgtype.UUID, error) {
	var personaID pgtype.UUID

	var empty gen.GetAIPersonaByIDRow

	if len(existing.FieldValues) > 0 {
		var fv map[string]any
		if err := json.Unmarshal(existing.FieldValues, &fv); err == nil {
			if prov, ok := fv["_ai_provenance"].(map[string]any); ok {
				if raw, ok := prov["ai_persona_id"].(string); ok && raw != "" {
					_ = personaID.Scan(raw)
				}
			}
		}
	}

	if !personaID.Valid && a.studio != nil {
		defaultID, err := a.studio.DefaultPersonaID(ctx)
		if err == nil {
			personaID = defaultID
		}
	}

	if !personaID.Valid {
		return empty, pgtype.UUID{}, errors.New("no persona available for regeneration")
	}

	persona, err := a.personaStore.GetAIPersonaByID(ctx, personaID)
	if err != nil {
		return empty, pgtype.UUID{}, err
	}

	return persona, personaID, nil
}

// hasManualEditSinceAIGeneration returns true if a content revision exists
// that was edited by someone other than the current AI persona after the
// last AI generation timestamp. It protects human editorial overrides from
// being silently overwritten by source re-crawls.
func (a *studioDraftProcessorAdapter) hasManualEditSinceAIGeneration(ctx context.Context, contentID pgtype.UUID, aiGeneratedAt pgtype.Timestamptz, personaUserID pgtype.UUID) (bool, error) {
	revisions, err := a.contentSvc.ListContentRevisions(ctx, contentID)
	if err != nil {
		return false, err
	}

	for _, r := range revisions {
		if !r.EditedBy.Valid {
			continue
		}

		if r.EditedBy.Bytes == personaUserID.Bytes {
			continue
		}

		if aiGeneratedAt.Valid && r.CreatedAt.Valid && r.CreatedAt.Time.After(aiGeneratedAt.Time) {
			return true, nil
		}
		// If we have no ai_generated_at, treat any non-persona edit as
		// manual. This is defensive for backfilled/legacy rows.
		if !aiGeneratedAt.Valid {
			return true, nil
		}
	}

	return false, nil
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

		resp, err := a.arrivalSvc.GenerateGuide(ctx, arrivalchecklist.GenerateGuideRequest{
			DestinationSpaceID: destSpaceStr,
			VisaCategory:       visaCat,
			ApplicantType:      arrivalchecklist.ApplicantIndividual,
			Prompt:             promptWithCtx,
		})
		if err != nil {
			slog.Warn("studio: arrival_checklist routing failed, falling back to contents", "job_id", jobID, "error", err)
			return pgtype.UUID{}, "", false, nil
		}

		var id pgtype.UUID
		_ = id.Scan(resp.GuideID)

		return id, "arrival_checklist_guides", true, nil

	case "community_organisation":
		if a.communityOrgSvc == nil {
			return pgtype.UUID{}, "", false, nil
		}

		org, err := a.communityOrgSvc.Generate(ctx, persona.UserID, communitylocator.GenerateInput{
			Prompt:             promptWithCtx,
			DestinationSpaceID: destSpaceStr,
		})
		if err != nil {
			slog.Warn("studio: community_organisation routing failed, falling back to contents", "job_id", jobID, "error", err)
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
			slog.Warn("studio: attestation_guide routing failed, falling back to contents", "job_id", jobID, "error", err)
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

		dto, err := a.languageTestSvc.GenerateDraft(ctx, languagetesthub.GenerateRequirementRequest{
			DestinationSpaceID: destSpaceStr,
			VisaCategory:       visaCat,
			TestSlug:           testSlug,
		})
		if err != nil {
			slog.Warn("studio: language_test_requirement routing failed, falling back to contents", "job_id", jobID, "error", err)
			return pgtype.UUID{}, "", false, nil
		}

		var id pgtype.UUID
		_ = id.Scan(dto.ID)

		return id, "language_test_requirements", true, nil

	case "tax_obligation_guide":
		// Tax guides are keyed on origin×destination×employment_status
		// (§100 schema UNIQUE constraint). The studio.Job model only
		// carries TargetSpaceID (destination) — there is no origin space
		// field. Routing with origin==destination produces a semantically
		// wrong draft (a "Nigeria→Nigeria" tax guide is meaningless).
		// Fall back to contents creation; tax guide AI drafts must be
		// generated via the dedicated
		// POST /admin/tax-calculator/guides/generate endpoint where the
		// admin explicitly provides both origin and destination.
		// Ref: japaconnect-engineering/sections/100-tax-calculator.md
		return pgtype.UUID{}, "", false, nil

	case "credential_pathway":
		if a.credentialSvc == nil || destSpaceStr == "" {
			return pgtype.UUID{}, "", false, nil
		}
		// Resolve profession_id from the job's guide_category (treated as
		// a profession slug) or topic_prompt. Falls back to contents if
		// no profession matches.
		professionSlug := visaCat
		if professionSlug == "" {
			return pgtype.UUID{}, "", false, nil
		}

		prof, perr := a.credentialSvc.GetProfessionBySlug(ctx, professionSlug)
		if perr != nil {
			slog.Info("studio: credential_pathway: profession slug not found, falling back to contents", "job_id", jobID, "profession_slug", professionSlug)
			return pgtype.UUID{}, "", false, nil
		}

		resp, err := a.credentialSvc.GeneratePathwayDraft(ctx, credentialchecker.GeneratePathwayRequest{
			ProfessionID:       prof.ID,
			IssuingSpaceID:     destSpaceStr,
			DestinationSpaceID: destSpaceStr,
		})
		if err != nil {
			slog.Warn("studio: credential_pathway routing failed, falling back to contents", "job_id", jobID, "error", err)
			return pgtype.UUID{}, "", false, nil
		}

		var id pgtype.UUID
		_ = id.Scan(resp.PathwayID)

		return id, "credential_pathways", true, nil

	case "housing_guide":
		if a.housingSvc == nil || destSpaceStr == "" {
			return pgtype.UUID{}, "", false, nil
		}

		resp, err := a.housingSvc.GenerateDraft(ctx, housingguide.GenerateGuideRequest{
			DestinationSpaceID: destSpaceStr,
			AccommodationType:  visaCat,
		})
		if err != nil {
			slog.Warn("studio: housing_guide routing failed, falling back to contents", "job_id", jobID, "error", err)
			return pgtype.UUID{}, "", false, nil
		}

		var id pgtype.UUID
		_ = id.Scan(resp.GuideID)

		return id, "housing_guides", true, nil

	case "visa_route":
		if a.visaRouteSvc == nil || destSpaceStr == "" {
			return pgtype.UUID{}, "", false, nil
		}

		resp, err := a.visaRouteSvc.GenerateDraft(ctx, visaroutes.GenerateRouteRequest{
			DestinationSpaceID: destSpaceStr,
			VisaCategory:       visaCat,
			Prompt:             promptWithCtx,
		})
		if err != nil {
			slog.Warn("studio: visa_route routing failed, falling back to contents", "job_id", jobID, "error", err)
			return pgtype.UUID{}, "", false, nil
		}

		var id pgtype.UUID
		_ = id.Scan(resp.RouteID)

		return id, "visa_routes", true, nil

	case "wiki_page":
		if a.wikiSvc == nil || !job.TargetSpaceID.Valid {
			return pgtype.UUID{}, "", false, nil
		}
		// Wiki needs a space_id + title. Use the job's TargetSpaceID and
		// topic_prompt as the title.
		resp, err := a.wikiSvc.GenerateDraft(ctx, wiki.GenerateDraftRequest{
			SpaceID:  job.TargetSpaceID.String(),
			Title:    job.TopicPrompt,
			Category: visaCat,
			Prompt:   promptWithCtx,
		})
		if err != nil {
			slog.Warn("studio: wiki_page routing failed, falling back to contents", "job_id", jobID, "error", err)
			return pgtype.UUID{}, "", false, nil
		}

		var id pgtype.UUID
		_ = id.Scan(resp.PageID)

		return id, "wiki_pages", true, nil

	case "calendar_entry":
		if a.calendarSvc == nil || destSpaceStr == "" {
			return pgtype.UUID{}, "", false, nil
		}

		resp, err := a.calendarSvc.GenerateDraft(ctx, calendar.GenerateDraftRequest{
			DestinationSpaceID: destSpaceStr,
			Category:           visaCat,
			Prompt:             promptWithCtx,
		})
		if err != nil {
			slog.Warn("studio: calendar_entry routing failed, falling back to contents", "job_id", jobID, "error", err)
			return pgtype.UUID{}, "", false, nil
		}

		var id pgtype.UUID
		_ = id.Scan(resp.EntryID)

		return id, "calendar_entries", true, nil

	case "doc_page":
		if a.docsSvc == nil {
			return pgtype.UUID{}, "", false, nil
		}

		resp, err := a.docsSvc.GenerateDraft(ctx, tenantID, docs.GenerateDraftRequest{
			Title:  job.TopicPrompt,
			Prompt: promptWithCtx,
		})
		if err != nil {
			slog.Warn("studio: doc_page routing failed, falling back to contents", "job_id", jobID, "error", err)
			return pgtype.UUID{}, "", false, nil
		}

		var id pgtype.UUID
		_ = id.Scan(resp.PageID)

		return id, "doc_pages", true, nil

	case "story":
		if a.storySvc == nil || destSpaceStr == "" {
			return pgtype.UUID{}, "", false, nil
		}
		// Story creates both a contents row and a story_meta row. The
		// HITL review targets the contents row; the persona's backing user_id
		// is the author.
		resp, err := a.storySvc.GenerateDraft(ctx, tenantID, persona.UserID, story.GenerateDraftRequest{
			DestinationSpaceID: destSpaceStr,
			VisaCategory:       visaCat,
			Prompt:             promptWithCtx,
		})
		if err != nil {
			slog.Warn("studio: story routing failed, falling back to contents", "job_id", jobID, "error", err)
			return pgtype.UUID{}, "", false, nil
		}

		var id pgtype.UUID
		_ = id.Scan(resp.ContentID)

		return id, "contents", true, nil

	default:
		// Unknown tool-table type: fall back to contents creation.
		return pgtype.UUID{}, "", false, nil
	}
}

// syncCampaignItem keeps the campaign_items table in sync with Studio job
// lifecycle events. It is best-effort and logs failures without failing the
// job, because a campaign item is metadata around a Studio job.
func (a *studioDraftProcessorAdapter) syncCampaignItem(ctx context.Context, jobID pgtype.UUID, status string, contentID, reviewID pgtype.UUID, errMsg *string) {
	if a.campaignSvc == nil {
		return
	}

	if err := a.campaignSvc.SyncCampaignItemByStudioJob(ctx, jobID, status, contentID, reviewID, errMsg); err != nil {
		slog.Warn("studio: sync campaign item failed", "job_id", jobID, "status", status, "error", err)
	}
}

// routeToHITL marks the job as generated and creates a HITL review with
// the appropriate target_table (migration 000477, ADR-076 Amendment 1).
func (a *studioDraftProcessorAdapter) routeToHITL(
	ctx context.Context,
	jobID, draftID pgtype.UUID,
	targetTable, contentType, aiModel string,
	fvJSON []byte,
) {
	if err := a.studio.MarkJobGenerated(ctx, jobID, draftID, aiModel); err != nil {
		slog.Warn("studio: generate: mark generated failed", "job_id", jobID, "draft_id", draftID, "error", err)
		return
	}

	a.syncCampaignItem(ctx, jobID, "draft_ready", draftID, pgtype.UUID{}, nil)

	if a.aiStore == nil {
		return
	}

	review, rerr := a.aiStore.CreateHITLReview(ctx, ai.HITLReview{

		ContentID:   draftID,
		ContentType: contentType,
		AIOutput:    fvJSON,
		Status:      ai.HITLStatusPending,
		Source:      ai.HITLSourceStudio,
		TargetTable: targetTable,
	})
	if rerr != nil {
		slog.Warn("studio: generate: create HITL review failed, draft remains in draft_ready", "job_id", jobID, "draft_id", draftID, "target_table", targetTable, "error", rerr)
		return
	}

	if err := a.studio.MarkJobRoutedToReview(ctx, jobID, review.ID); err != nil {
		slog.Warn("studio: generate: mark routed to review failed", "job_id", jobID, "review_id", review.ID, "error", err)
		return
	}

	a.syncCampaignItem(ctx, jobID, "in_review", draftID, review.ID, nil)
	slog.Info("studio: generate: draft routed to review", "job_id", jobID, "draft_id", draftID, "target_table", targetTable, "review_id", review.ID)
}

// studioGenerateBody synthesizes a draft body via the AI gateway, weaving in
// the persona voice prompt when present and honoring per-job model/voice
// overrides (ADR-114). Returns the body text and the model name (for ADR-088
// ai_model column on studio_jobs).
func (a *studioDraftProcessorAdapter) studioGenerateBody(ctx context.Context, job studio.Job, persona gen.GetAIPersonaByIDRow, extractionContext string) (string, string, error) {
	voicePrompt := persona.VoicePrompt

	if job.VoiceID.Valid {
		voice, err := a.personaStore.GetPersonaVoiceByID(ctx, job.VoiceID)
		if err == nil && voice.VoicePrompt != "" {
			voicePrompt = voice.VoicePrompt
		} else if err != nil {
			slog.Warn("studio: generate: could not resolve job voice override, using persona voice", "job_id", job.ID, "voice_id", job.VoiceID, "error", err)
		}
	}

	prompt := fmt.Sprintf("Generate a %s draft about: %s", job.ContentType, job.TopicPrompt)
	if voicePrompt != "" {
		prompt = fmt.Sprintf("Voice: %s\n\n%s", voicePrompt, prompt)
	}

	if job.GuideCategory != nil && *job.GuideCategory != "" {
		prompt += "\nGuide category: " + *job.GuideCategory
	}

	if extractionContext != "" {
		prompt += "\n\nStructured data from external source (use as factual grounding for the article):\n" + extractionContext
	}

	payload := map[string]any{
		"prompt": prompt,
	}
	if job.AIModel != nil && *job.AIModel != "" {
		payload["model"] = *job.AIModel
	}

	resp, err := a.dispatcher.Dispatch(ctx, "content_generation", ai.Request{
		Category: "content_generation",
		Payload:  payload,
	})
	if err != nil {
		return "", "", err
	}

	model, _ := resp.Raw["model"].(string)
	if text, ok := resp.Raw["text"].(string); ok {
		return text, model, nil
	}

	if text, ok := resp.Raw["content"].(string); ok {
		return text, model, nil
	}

	if output, ok := resp.Raw["output"].(string); ok {
		return output, model, nil
	}
	// Fallback: no recognized text field; return the topic prompt so the
	// draft is still created with meaningful content.
	return job.TopicPrompt, model, nil
}

// createStudioReview is a best-effort HITL review creation helper.
func (a *studioDraftProcessorAdapter) createStudioReview(ctx context.Context, contentID pgtype.UUID, contentType string, fvJSON []byte) {
	_, err := a.aiStore.CreateHITLReview(ctx, ai.HITLReview{

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

// agentScholarshipAdapter adapts scholarships.Service to the
// ai.ScholarshipSearcher interface so the ai package can offer the
// scholarship.search tool without importing internal/scholarships
// (breaks the ai → scholarships → content → spam → ai cycle).
// // Ref: japaconnect-engineering/sections/73-scholarship-finder.md.
type agentScholarshipAdapter struct {
	svc *scholarships.Service
}

func newAgentScholarshipAdapter(svc *scholarships.Service) *agentScholarshipAdapter {
	return &agentScholarshipAdapter{svc: svc}
}

// FindScholarships implements ai.ScholarshipSearcher by mapping to the
// scholarships.Service filters and results.
func (a *agentScholarshipAdapter) FindScholarships(ctx context.Context, filters ai.ScholarshipSearchFilters) ([]ai.ScholarshipSearchResult, int64, error) {
	if a == nil || a.svc == nil {
		return nil, 0, nil
	}

	sf := scholarships.ScholarshipFilters{
		Country:      filters.Country,
		Level:        filters.Level,
		DeadlineFrom: filters.DeadlineFrom,
		DeadlineTo:   filters.DeadlineTo,
		Q:            filters.EligibleCountries + " " + filters.FundingType,
		Limit:        filters.Limit,
		Page:         filters.Page,
	}

	rows, total, err := a.svc.FindScholarships(ctx, sf)
	if err != nil {
		return nil, 0, err
	}

	out := make([]ai.ScholarshipSearchResult, 0, len(rows))
	for _, r := range rows {
		out = append(out, ai.ScholarshipSearchResult{
			ID:                  r.ID,
			GuideID:             r.GuideID,
			IntelRecordID:       r.IntelRecordID,
			Title:               r.Title,
			Slug:                r.Slug,
			Summary:             r.Summary,
			Description:         r.Description,
			Institution:         r.Institution,
			Country:             r.Country,
			LevelOfStudy:        r.LevelOfStudy,
			AwardAmountUSD:      r.AwardAmountUSD,
			IsRenewable:         r.IsRenewable,
			ApplicationDeadline: r.ApplicationDeadline,
			ApplicationURL:      r.ApplicationURL,
		})
	}

	return out, total, nil
}

// transitionIntelRecordOnPublish transitions a linked intel_record to
// 'published' when the studio job is published (§75: "On successful publish
// the linked intel_records row transitions to status='published'"). Safe to
// call when intelRecordID is nil or no intel store is wired.
func (a *studioDraftProcessorAdapter) transitionIntelRecordOnPublish(ctx context.Context, intelRecordID pgtype.UUID) {
	if !intelRecordID.Valid || a.intelStore == nil {
		return
	}

	if err := a.intelStore.UpdateIntelRecordStatus(ctx, gen.UpdateIntelRecordStatusParams{

		ID:     intelRecordID,
		Status: "published",
	}); err != nil {
		slog.Warn("studio: transition intel_record to published failed", "intel_record_id", intelRecordID, "error", err)
	}
}

// ---------------------------------------------------------------------------
// Section 104 — AI Content Campaigns adapters
// ---------------------------------------------------------------------------

// agentPersonaProvisionerAdapter adapts personas.Service.ProvisionPersonas to
// the ai.PersonaProvisioner interface, translating ai.PersonaDefinition into
// personas.PersonaInput and back.
type agentPersonaProvisionerAdapter struct {
	svc *personas.Service
}

func (a *agentPersonaProvisionerAdapter) ProvisionPersonas(ctx context.Context, tenantID, adminUserID pgtype.UUID, defs []ai.PersonaDefinition) ([]ai.ProvisionedPersona, error) {
	inputs := make([]personas.PersonaInput, 0, len(defs))

	for _, d := range defs {
		var ap personas.ActivityPolicy

		if len(d.ActivityPolicy) > 0 {
			b, err := json.Marshal(d.ActivityPolicy)
			if err != nil {
				return nil, fmt.Errorf("marshal activity_policy: %w", err)
			}

			if err := json.Unmarshal(b, &ap); err != nil {
				return nil, fmt.Errorf("unmarshal activity_policy: %w", err)
			}
		}

		tagline := d.Tagline
		if tagline == "" && d.RoleFocus != "" {
			tagline = d.RoleFocus
		}

		inputs = append(inputs, personas.PersonaInput{
			Name:           d.Name,
			Slug:           d.Slug,
			Tagline:        tagline,
			VoicePrompt:    d.VoicePrompt,
			DisclosureMode: d.DisclosureMode,
			AutonomyMode:   d.AutonomyMode,
			ActivityPolicy: ap,
		})
	}

	results, err := a.svc.ProvisionPersonas(ctx, tenantID, adminUserID, inputs)
	if err != nil {
		return nil, err
	}

	out := make([]ai.ProvisionedPersona, 0, len(results))
	for _, r := range results {
		out = append(out, ai.ProvisionedPersona{
			ID:    r.ID,
			Name:  r.Input.Name,
			Slug:  r.Input.Slug,
			Error: r.Error,
		})
	}

	return out, nil
}

// agentCampaignAdapter adapts campaign.Service to the ai.CampaignService
// interface used by the content.campaign.create agent tool.
type agentCampaignAdapter struct {
	svc *campaign.Service
}

func (a *agentCampaignAdapter) CreateCampaign(ctx context.Context, tenantID, userID pgtype.UUID, in ai.CampaignCreateInput) (ai.CampaignCreateResult, error) {
	autoProvision := true
	if in.AutoProvisionPersonas != nil {
		autoProvision = *in.AutoProvisionPersonas
	}

	autoAssign := true
	if in.AutoAssign != nil {
		autoAssign = *in.AutoAssign
	}

	runImmediately := false
	if in.RunImmediately != nil {
		runImmediately = *in.RunImmediately
	}

	cIn := campaign.CreateCampaignInput{
		Prompt:                in.Prompt,
		ContentType:           in.ContentType,
		Count:                 in.Count,
		PersonaPool:           in.PersonaIDs,
		AutoProvisionPersonas: autoProvision,
		AutoAssign:            autoAssign,
		TargetSpaceID:         in.TargetSpaceID,
		TargetTags:            in.TargetTags,
		RunImmediately:        runImmediately,
	}

	c, items, err := a.svc.CreateCampaign(ctx, tenantID, userID, cIn)
	if err != nil {
		return ai.CampaignCreateResult{}, err
	}

	result := ai.CampaignCreateResult{
		CampaignID:          c.ID,
		RequestedCount:      c.RequestedCount,
		ProvisionedPersonas: c.ProvisionedPersonas,
		Status:              c.Status,
	}
	result.Items = make([]ai.CampaignItemSummary, 0, len(items))

	for _, it := range items {
		result.Items = append(result.Items, ai.CampaignItemSummary{
			PersonaID:   it.PersonaID,
			ContentType: it.ContentType,
			Title:       it.Title,
		})
	}

	return result, nil
}

// agentContentTypesAdapter adapts the content package's type catalog plus
// custom field group assignments into the ai.ContentTypesService interface.
// It returns each valid content type with its label, whether it has
// admin-configurable categories, and the entity_class/group to use with
// custom_field_groups.list / custom_fields.list.
// // Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Content Type Discovery).
type agentContentTypesAdapter struct {
	cf *customfields.Service
}

func (a *agentContentTypesAdapter) ListContentTypes(ctx context.Context) ([]ai.ContentType, error) {
	groups, err := a.cf.ListFieldGroups(ctx)
	if err != nil {
		return nil, err
	}

	assignments, err := a.cf.ListAllAssignmentsByTenant(ctx)
	if err != nil {
		return nil, err
	}

	groupByID := make(map[pgtype.UUID]gen.CustomFieldGroup, len(groups))
	for _, g := range groups {
		groupByID[g.ID] = g
	}

	// entity_class -> list of matching groups
	classToGroups := make(map[string][]gen.CustomFieldGroup)

	for _, asn := range assignments {
		g, ok := groupByID[asn.GroupID]
		if !ok {
			continue
		}

		classToGroups[asn.EntityClass] = append(classToGroups[asn.EntityClass], g)
	}

	types := content.ContentTypes()
	out := make([]ai.ContentType, 0, len(types))

	for _, t := range types {
		ct := ai.ContentType{
			ContentType: t,
			Label:       content.ContentTypeLabels[t],
			HasCategory: content.ContentTypeHasCategory(t),
		}

		switch t {
		case content.TypeGuide:
			ct.CategoryListTool = "guide_categories.list"
			// Guide categories are dynamic; the agent calls guide_categories.list
			// to get the category-specific field group. We still provide the
			// generic entity class pattern as a hint.
			ct.FieldGroupEntityClass = "content_type:guide:<category>"
		case content.TypeComparison:
			ct.CategoryListTool = "comparison.types.list"
		default:
			entityClass := "content_type:" + t
			wantSlug := t + "_fields"

			if gs, ok := classToGroups[entityClass]; ok && len(gs) > 0 {
				// Direct match, e.g. content_type:blog_post -> blog_post_fields
				ct.FieldGroupEntityClass = entityClass
				if gs[0].ID.Valid {
					ct.FieldGroupID = gs[0].ID.String()
				}

				ct.FieldGroupSlug = gs[0].Slug
			} else {
				// Fallback: find a group whose slug matches {type}_fields and
				// report the entity_class from its assignment. This covers
				// job_listing, which is assigned to the generic 'content'
				// entity_class.
				var fallbackGroup *gen.CustomFieldGroup

				var fallbackClass string

				for _, g := range groups {
					if g.Slug == wantSlug {
						fallbackGroup = &g

						for _, asn := range assignments {
							if asn.GroupID == g.ID {
								fallbackClass = asn.EntityClass
								break
							}
						}

						break
					}
				}

				if fallbackGroup != nil {
					ct.FieldGroupEntityClass = fallbackClass
					if fallbackGroup.ID.Valid {
						ct.FieldGroupID = fallbackGroup.ID.String()
					}

					ct.FieldGroupSlug = fallbackGroup.Slug
				} else {
					// No group seeded yet; the agent can still try the canonical
					// entity class and discover nothing.
					ct.FieldGroupEntityClass = entityClass
				}
			}
		}

		out = append(out, ct)
	}

	return out, nil
}

// agentGuideCategoriesAdapter adapts customfields.Service.ListGuideCategories
// to the ai.GuideCategoryService interface. The ai package cannot import
// internal/customfields directly because that creates an import cycle
// (customfields imports db, content imports customfields, etc.).
// // Ref: japaconnect-engineering/sections/14-content-type-system.md (Guide Content Type System).
type agentGuideCategoriesAdapter struct {
	svc *customfields.Service
}

func (a *agentGuideCategoriesAdapter) ListGuideCategories(ctx context.Context) ([]ai.GuideCategory, error) {
	cats, err := a.svc.ListGuideCategories(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]ai.GuideCategory, 0, len(cats))
	for _, c := range cats {
		out = append(out, ai.GuideCategory{
			Slug:      c.Slug,
			Label:     c.Label,
			IsSystem:  c.IsSystem,
			GroupID:   c.GroupID,
			GroupSlug: c.GroupSlug,
		})
	}

	return out, nil
}
