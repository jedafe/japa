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

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/academy"
	"github.com/jedafe/japa/internal/ai"
	"github.com/jedafe/japa/internal/arrivalchecklist"
	"github.com/jedafe/japa/internal/attestationguide"
	"github.com/jedafe/japa/internal/calendar"
	"github.com/jedafe/japa/internal/campaign"
	"github.com/jedafe/japa/internal/comments"
	"github.com/jedafe/japa/internal/communitylocator"
	"github.com/jedafe/japa/internal/comparison"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/credentialchecker"
	"github.com/jedafe/japa/internal/crscalculator"
	"github.com/jedafe/japa/internal/customfields"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/docs"
	"github.com/jedafe/japa/internal/forum"
	"github.com/jedafe/japa/internal/fx"
	"github.com/jedafe/japa/internal/housingguide"
	"github.com/jedafe/japa/internal/languagetesthub"
	"github.com/jedafe/japa/internal/menus"
	"github.com/jedafe/japa/internal/personas"
	"github.com/jedafe/japa/internal/pofcalculator"
	"github.com/jedafe/japa/internal/publicapi"
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
// the ai-package mirror type. viewerID scopes retrieval to the caller
// (§69 scoped-retrieval amendment).
func (a *agentRAGAdapter) Retrieve(ctx context.Context, query, language string, geoSpaceID *pgtype.UUID, viewerID pgtype.UUID) ([]ai.RAGChunk, error) {
	chunks, err := a.svc.Retrieve(ctx, query, language, geoSpaceID, viewerID)
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
			Title:      c.Title,
			Slug:       c.Slug,
			URL:        c.URL,
		})
	}

	return out, nil
}

// RetrievePrivileged calls rag.Service.RetrievePrivileged — the §121
// staff-only spec-corpus path used by the spec.search tool. Translates
// rag.ErrSpecCorpusDisabled into the ai-package mirror sentinel so tool
// handlers can distinguish flag-off from real failures.
func (a *agentRAGAdapter) RetrievePrivileged(ctx context.Context, query, language, pathPrefix string, topK int32) ([]ai.RAGChunk, error) {
	chunks, err := a.svc.RetrievePrivileged(ctx, query, language, pathPrefix, topK)
	if err != nil {
		if errors.Is(err, rag.ErrSpecCorpusDisabled) {
			return nil, ai.ErrSpecCorpusDisabled
		}

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
			Title:      c.Title,
			Slug:       c.Slug,
			URL:        c.URL,
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

func (b *agentContentCreatorBridge) create(ctx context.Context, input ai.ContentCreateInput) (ai.ContentCreateRow, error) {
	var guideCategory *string
	if input.GuideCategory != "" {
		guideCategory = &input.GuideCategory
	}

	row, err := b.svc.CreateContent(ctx, content.CreateContentInput{
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

func (b *agentContentUpdaterBridge) update(ctx context.Context, contentID, editedBy pgtype.UUID, input ai.ContentUpdateInput) (ai.ContentUpdateRow, error) {
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

	row, err := b.svc.UpdateContent(ctx, contentID, editedBy, content.UpdateContentInput{
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

func (a *agentComparisonAdapter) ListComparisonTypes(ctx context.Context, limit, offset int32) ([]gen.ListComparisonTypesRow, error) {
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

func (a *agentComparisonAdapter) CreateComparison(ctx context.Context, userID pgtype.UUID, input ai.ComparisonCreateInput) (ai.ComparisonCreateResult, error) {
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

func (a *agentComparisonAdapter) PromoteComparison(ctx context.Context, id, userID pgtype.UUID, bypassOwnership bool) (ai.ComparisonPromoteResult, error) {
	res, err := a.svc.PromoteJCEComparison(ctx, id, userID, bypassOwnership)
	if err != nil {
		return ai.ComparisonPromoteResult{}, err
	}

	return ai.ComparisonPromoteResult{
		ID:         res.ID,
		AuthorID:   res.AuthorID,
		Title:      res.Title,
		Slug:       res.Slug,
		Visibility: res.Visibility,
		Status:     res.Status,
		Depth:      res.Depth,
		SpaceID:    res.SpaceID,
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

func (a *agentComparisonAdapter) DeleteJCEComparisonCaches(ctx context.Context, contentID pgtype.UUID) {
	a.svc.DeleteJCEComparisonCaches(ctx, contentID)
}

// ListComparisons maps the ai filter onto Service.ListJCEComparisons and
// strips each row to listing metadata — bodies and field values stay
// behind the privacy-gated comparison.read tool.
func (a *agentComparisonAdapter) ListComparisons(ctx context.Context, f ai.ComparisonListFilter) (ai.ComparisonListResult, error) {
	res, err := a.svc.ListJCEComparisons(ctx, comparison.JCEComparisonFilter{
		SpaceID:    f.SpaceID,
		TypeID:     f.TypeID,
		Depth:      f.Depth,
		Status:     f.Status,
		Visibility: f.Visibility,
		Mine:       f.Mine,
		UserID:     f.UserID,
		Query:      f.Query,
		Sort:       f.Sort,
		Limit:      f.Limit,
		Offset:     f.Offset,
	})
	if err != nil {
		return ai.ComparisonListResult{}, err
	}

	items := make([]ai.ComparisonListItem, 0, len(res.Items))

	for _, r := range res.Items {
		item := ai.ComparisonListItem{
			ID:           r.ID.String(),
			Title:        r.Title,
			Slug:         r.Slug,
			Status:       r.Status,
			Visibility:   r.Visibility,
			Depth:        r.Depth,
			ViewCount:    r.ViewCount,
			CommentCount: r.CommentCount,
			CreatedAt:    r.CreatedAt.Time.UTC().Format(time.RFC3339),
			UpdatedAt:    r.UpdatedAt.Time.UTC().Format(time.RFC3339),
		}
		if r.SpaceID.Valid {
			item.SpaceID = r.SpaceID.String()
		}

		if r.AuthorID.Valid {
			item.AuthorID = r.AuthorID.String()
		}

		if r.PromotedFromID.Valid {
			item.PromotedFromID = r.PromotedFromID.String()
		}

		if r.PublishedAt.Valid {
			item.PublishedAt = r.PublishedAt.Time.UTC().Format(time.RFC3339)
		}

		items = append(items, item)
	}

	return ai.ComparisonListResult{
		Items: items,
		Total: res.Total,
		Page:  res.Page,
		Size:  res.Size,
	}, nil
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

	if text := ai.ExtractText(resp); text != "" {
		return text, nil
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
	row, err := a.contentSvc.CreateContent(ctx, content.CreateContentInput{
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

// agentAcademyAdapter bridges academy.Store to the ai.AcademyStore
// interface. The ai package uses mirror types (ai.AcademyTrack,
// ai.AcademyLesson, ai.AcademyCreateTrackInput, etc.) to avoid an import
// cycle with internal/academy. This adapter converts between the two.
// // academy.lessons.reorder has no backing Store method — the adapter uses
// UpdateLesson per entry to set the new sort_order. This is sequential
// (not a single bulk SQL UPDATE) but correct; a future migration can add
// a bulk reorder query if performance requires it.
type agentAcademyAdapter struct{ store *academy.Store }

func (a *agentAcademyAdapter) ListTracks(ctx context.Context) ([]ai.AcademyTrack, error) {
	tracks, err := a.store.ListAllTracks(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]ai.AcademyTrack, 0, len(tracks))
	for _, t := range tracks {
		out = append(out, academyTrackToAI(t))
	}

	return out, nil
}

func (a *agentAcademyAdapter) CreateTrack(ctx context.Context, input ai.AcademyCreateTrackInput) (*ai.AcademyTrack, error) {
	t, err := a.store.CreateTrack(ctx, academy.CreateTrackInput{
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

	aiT := academyTrackToAI(*t)

	return &aiT, nil
}

func (a *agentAcademyAdapter) UpdateTrack(ctx context.Context, id pgtype.UUID, input ai.AcademyUpdateTrackInput) (*ai.AcademyTrack, error) {
	t, err := a.store.UpdateTrack(ctx, id, academy.UpdateTrackInput{
		Title:                ptrOrNil(input.Title),
		Description:          input.Description,
		Difficulty:           ptrOrNil(input.Difficulty),
		CoverMediaID:         input.CoverMediaID,
		IsPublished:          ptrOrNil(input.IsPublished),
		SortOrder:            ptrOrNil(input.SortOrder),
		Category:             input.Category,
		SubscriptionEligible: input.SubscriptionEligible,
	})
	if err != nil {
		return nil, err
	}

	aiT := academyTrackToAI(*t)

	return &aiT, nil
}

func (a *agentAcademyAdapter) ListLessons(ctx context.Context) ([]ai.AcademyLesson, error) {
	lessons, err := a.store.ListAllLessons(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]ai.AcademyLesson, 0, len(lessons))
	for _, l := range lessons {
		out = append(out, academyLessonToAI(l))
	}

	return out, nil
}

func (a *agentAcademyAdapter) CreateLesson(ctx context.Context, moduleID pgtype.UUID, input ai.AcademyCreateLessonInput) (*ai.AcademyLesson, error) {
	l, err := a.store.CreateLesson(ctx, moduleID, academy.CreateLessonInput{
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

	aiL := academyLessonToAI(*l)

	return &aiL, nil
}

func (a *agentAcademyAdapter) UpdateLesson(ctx context.Context, id pgtype.UUID, input ai.AcademyUpdateLessonInput) (*ai.AcademyLesson, error) {
	l, err := a.store.UpdateLesson(ctx, id, academy.UpdateLessonInput{
		Title:           ptrOrNil(input.Title),
		LessonType:      ptrOrNil(input.LessonType),
		Content:         input.Content,
		MediaID:         input.MediaID,
		DurationMinutes: input.DurationMinutes,
		SortOrder:       ptrOrNil(input.SortOrder),
		IsPublished:     ptrOrNil(input.IsPublished),
		IsPreview:       input.IsPreview,
	})
	if err != nil {
		return nil, err
	}

	aiL := academyLessonToAI(*l)

	return &aiL, nil
}

// ReorderLessons updates each lesson's sort_order sequentially. The
// academy.Store does not expose a bulk reorder query, so we call
// UpdateLesson per entry. A failure on one entry aborts the reorder
// (the caller can retry). This is acceptable for the AI agent's use
// case (small batches).
func (a *agentAcademyAdapter) ReorderLessons(ctx context.Context, entries []ai.AcademyReorderEntry) error {
	for _, e := range entries {
		// UpdateLessonInput is partial-update safe — nil fields are left
		// untouched, so a reorder only needs to send sort_order.
		_, err := a.store.UpdateLesson(ctx, e.ID, academy.UpdateLessonInput{
			SortOrder: ptrOrNil(e.SortOrder),
		})
		if err != nil {
			return fmt.Errorf("reorder: updating lesson %s sort_order: %w", e.ID, err)
		}
	}

	return nil
}

func academyTrackToAI(t academy.Track) ai.AcademyTrack {
	return ai.AcademyTrack{
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

func academyLessonToAI(l academy.Lesson) ai.AcademyLesson {
	return ai.AcademyLesson{
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

// ptrOrNil returns a pointer to v. The academy update inputs use pointer
// fields so a zero value is still an explicit update.
func ptrOrNil[T any](v T) *T {
	return &v
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

func (a *agentDocsAdapter) CreateDocPage(ctx context.Context, req ai.DocsCreatePageRequest) (pgtype.UUID, error) {
	return a.svc.CreateDocPage(ctx, docs.CreateDocPageRequest{
		Title:        req.Title,
		Slug:         req.Slug,
		Audience:     req.Audience,
		Content:      req.Content,
		AIGenerated:  req.AIGenerated,
		ReviewStatus: req.ReviewStatus,
		ParentID:     req.ParentID,
		Position:     req.Position,
		Category:     req.Category,
		Subcategory:  req.Subcategory,
	})
}

func (a *agentDocsAdapter) UpdateDocPage(ctx context.Context, id pgtype.UUID, req ai.DocsUpdatePageRequest) error {
	return a.svc.UpdateDocPage(ctx, id, docs.UpdateDocPageRequest{
		Title:        req.Title,
		Content:      req.Content,
		ReviewStatus: req.ReviewStatus,
		ParentID:     req.ParentID,
		Position:     req.Position,
		Category:     req.Category,
		Subcategory:  req.Subcategory,
	})
}

func (a *agentDocsAdapter) ApproveDocPage(ctx context.Context, id, reviewerID pgtype.UUID) error {
	return a.svc.ApproveDocPage(ctx, id, reviewerID)
}

func (a *agentDocsAdapter) PublishDocPage(ctx context.Context, id, reviewerID pgtype.UUID) error {
	return a.svc.PublishDocPage(ctx, id, reviewerID)
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
		Category:     p.Category,
		Subcategory:  p.Subcategory,
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
	GenerateDraft(ctx context.Context, input attestationguide.GeneratePathwayInput) (attestationguide.PathwayDTO, error)
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
	GenerateDraft(ctx context.Context, input docs.GenerateDraftRequest) (docs.GenerateDraftResponse, error)
}

// storyDraftGenerator is the subset of *story.Service. Story creates both a
// contents row and a story_meta row, so it needs the persona's backing
// user_id as the author.
type storyDraftGenerator interface {
	GenerateDraft(ctx context.Context, authorID pgtype.UUID, input story.GenerateDraftRequest) (story.GenerateDraftResponse, error)
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
	PublishToolTableDraft(ctx context.Context, actorID pgtype.UUID, targetTable string, targetID pgtype.UUID) error
	ShouldAutoPublish(ctx context.Context, contentType, guideCategory string) bool
	RunBatch(ctx context.Context, batchID pgtype.UUID) error
	RetryFailed(ctx context.Context, jobID pgtype.UUID) error
	RetryBatchFailed(ctx context.Context, batchID pgtype.UUID) (int, error)
}

// RunBatch delegates to the studio service.
func (a *studioDraftProcessorAdapter) RunBatch(ctx context.Context, batchID pgtype.UUID) error {
	return a.studio.RunBatch(ctx, batchID)
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
func (a *studioDraftProcessorAdapter) GenerateJob(ctx context.Context, jobID pgtype.UUID) error {
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

			if errors.Is(gerr, ai.ErrNoProviders) {
				// Permanent configuration gap — the job is already marked
				// failed, so the task archives instead of burning retries.
				return fmt.Errorf("studio: generate: body: %w: %w", gerr, asynq.SkipRetry)
			}

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
		if draftID, targetTable, routed, rerr := a.tryToolTableRouting(ctx, jobID, job, persona, body, extractionContext); rerr == nil && routed {
			// Jobs created by actors holding agent.skip_hitl bypass the
			// review queue (§75): publish the routed draft directly.
			// Story-type routing produces a contents row, so contents
			// targets go through UpdateContentStatus like the default
			// path; dedicated tool tables use the tool-table publisher.
			// A publish failure falls back to the normal HITL route so
			// no draft is lost.
			if a.perms != nil && job.CreatedBy.Valid &&
				a.perms.HasPermission(ctx, job.CreatedBy, "agent.skip_hitl") {
				var perr error
				if targetTable == "contents" {
					_, perr = a.contentSvc.UpdateContentStatus(ctx, draftID, "published")
				} else {
					perr = a.studio.PublishToolTableDraft(ctx, job.CreatedBy, targetTable, draftID)
				}

				if perr != nil {
					slog.Warn("studio: generate: skip_hitl publish failed, routing to review", "job_id", jobID, "draft_id", draftID, "target_table", targetTable, "error", perr)
				} else {
					if err := a.studio.MarkJobPublished(ctx, jobID, draftID, aiModel); err != nil {
						slog.Warn("studio: generate: mark job published failed", "job_id", jobID, "error", err)
					} else {
						a.syncCampaignItem(ctx, jobID, "published", draftID, pgtype.UUID{}, nil)
					}

					a.transitionIntelRecordOnPublish(ctx, job.IntelRecordID)
					slog.Info("studio: generate: tool-table draft auto-published via agent.skip_hitl", "job_id", jobID, "draft_id", draftID, "target_table", targetTable)

					return nil
				}
			}

			// Mark generated (draft_ready) before routing — a failure here
			// is fatal so the task retries rather than leaving a draft
			// orphaned in 'generating'.
			if err := a.studio.MarkJobGenerated(ctx, jobID, draftID, aiModel); err != nil {
				return fmt.Errorf("studio: generate: mark generated: %w", err)
			}

			a.syncCampaignItem(ctx, jobID, "draft_ready", draftID, pgtype.UUID{}, nil)
			a.routeToHITL(ctx, jobID, draftID, targetTable, job.ContentType, aiModel, fvJSON)

			return nil
		}

		slog.Info("studio: generate: tool table routing fell back to contents", "job_id", jobID, "content_type", job.ContentType)
	}

	// 5b. Create the contents row with status='draft' (default path).
	row, err := a.contentSvc.CreateContent(ctx, content.CreateContentInput{
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

			// Mark generated (draft_ready) before routing so the job
			// carries draft_content_id into review — the same contract
			// the normal path follows.
			if err := a.studio.MarkJobGenerated(ctx, jobID, row.ID, aiModel); err != nil {
				return fmt.Errorf("studio: generate: mark generated: %w", err)
			}

			a.syncCampaignItem(ctx, jobID, "draft_ready", row.ID, pgtype.UUID{}, nil)
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
// actorID is the user who triggered the regeneration (from the task
// payload); actors holding agent.skip_hitl take the in-place update path
// directly instead of the draft-and-review detour. A zero/invalid actorID
// (system-triggered runs) keeps full HITL protection.
func (a *studioDraftProcessorAdapter) RegenerateFromSource(ctx context.Context, contentID, intelRecordID, actorID pgtype.UUID) error {
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
	// Actors holding agent.skip_hitl bypass the detour: they deliberately
	// requested the rewrite, so the in-place update runs directly (§75).
	needsHITL := highStakes || manual || (existing.Status == content.StatusPublished && !canAutoPublish)

	if needsHITL && a.perms != nil && actorID.Valid &&
		a.perms.HasPermission(ctx, actorID, "agent.skip_hitl") {
		slog.Info("regenerate: in-place update via agent.skip_hitl", "content_id", contentID, "actor_id", actorID, "high_stakes", highStakes, "manual_edit", manual)

		needsHITL = false
	}

	if needsHITL {
		draft, err := a.contentSvc.CreateContent(ctx, content.CreateContentInput{
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

	updated, err := a.contentSvc.UpdateContent(ctx, contentID, persona.UserID, content.UpdateContentInput{
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
// Preference: field_values._ai_provenance.ai_persona_id, then the platform's
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
	jobID pgtype.UUID,
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

		dto, err := a.attestationSvc.GenerateDraft(ctx, attestationguide.GeneratePathwayInput{
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

		resp, err := a.docsSvc.GenerateDraft(ctx, docs.GenerateDraftRequest{
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
		resp, err := a.storySvc.GenerateDraft(ctx, persona.UserID, story.GenerateDraftRequest{
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

// routeToHITL creates a HITL review with the appropriate target_table and
// transitions the job to 'in_review' (migration 000477, ADR-076 Amendment
// 1). Callers must have already marked the job generated (draft_ready).
func (a *studioDraftProcessorAdapter) routeToHITL(
	ctx context.Context,
	jobID, draftID pgtype.UUID,
	targetTable, contentType, aiModel string,
	fvJSON []byte,
) {
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

	model := ai.ExtractModel(resp)
	if text := ai.ExtractText(resp); text != "" {
		return text, model, nil
	}
	// Fallback: no recognized text field; return the topic prompt so the
	// draft is still created with meaningful content.
	return job.TopicPrompt, model, nil
}

// createStudioReview is a best-effort HITL review creation helper.
func (a *studioDraftProcessorAdapter) createStudioReview(ctx context.Context, contentID pgtype.UUID, contentType string, fvJSON []byte) {
	if a.aiStore == nil {
		return
	}

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

func (a *agentPersonaProvisionerAdapter) ProvisionPersonas(ctx context.Context, adminUserID pgtype.UUID, defs []ai.PersonaDefinition) ([]ai.ProvisionedPersona, error) {
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

	results, err := a.svc.ProvisionPersonas(ctx, adminUserID, inputs)
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

func (a *agentCampaignAdapter) CreateCampaign(ctx context.Context, userID pgtype.UUID, in ai.CampaignCreateInput) (ai.CampaignCreateResult, error) {
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

	c, items, err := a.svc.CreateCampaign(ctx, userID, cIn)
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

	assignments, err := a.cf.ListAllAssignmentsByPlatform(ctx)
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

// ---------------------------------------------------------------------------
// Persona voice + catalog read tools (§68, ADR-114)
// ---------------------------------------------------------------------------

// agentPersonaVoicesAdapter adapts personas.Service to the
// ai.PersonaVoicesService interface, translating personas.PersonaVoice into
// the ai-package mirror type so the ai package does not import
// internal/personas.
type agentPersonaVoicesAdapter struct {
	svc *personas.Service
}

func (a *agentPersonaVoicesAdapter) ListPersonaVoices(ctx context.Context, limit, offset int32) ([]ai.PersonaVoiceItem, int64, error) {
	voices, total, err := a.svc.ListPersonaVoices(ctx, limit, offset)
	if err != nil {
		return nil, 0, err
	}

	out := make([]ai.PersonaVoiceItem, 0, len(voices))
	for _, v := range voices {
		out = append(out, personaVoiceToAI(v))
	}

	return out, total, nil
}

func (a *agentPersonaVoicesAdapter) GetPersonaVoice(ctx context.Context, id pgtype.UUID) (ai.PersonaVoiceItem, error) {
	v, err := a.svc.GetPersonaVoice(ctx, id)
	if err != nil {
		return ai.PersonaVoiceItem{}, err
	}

	return personaVoiceToAI(v), nil
}

func (a *agentPersonaVoicesAdapter) GetPersonaVoiceBySlug(ctx context.Context, slug string) (ai.PersonaVoiceItem, error) {
	v, err := a.svc.GetPersonaVoiceBySlug(ctx, slug)
	if err != nil {
		return ai.PersonaVoiceItem{}, err
	}

	return personaVoiceToAI(v), nil
}

func personaVoiceToAI(v personas.PersonaVoice) ai.PersonaVoiceItem {
	return ai.PersonaVoiceItem{
		ID:          v.ID,
		Name:        v.Name,
		Slug:        v.Slug,
		Description: v.Description,
		VoicePrompt: v.VoicePrompt,
		IsSystem:    v.IsSystem,
		CreatedBy:   v.CreatedBy,
		CreatedAt:   v.CreatedAt,
		UpdatedAt:   v.UpdatedAt,
	}
}

// agentPersonaCatalogAdapter adapts personas.Service to the
// ai.PersonaCatalogService interface for the personas.list / personas.read
// agent tools.
type agentPersonaCatalogAdapter struct {
	svc *personas.Service
}

func (a *agentPersonaCatalogAdapter) ListPersonas(ctx context.Context, status, q, sort, dir string, limit, offset int32) ([]ai.AgentPersonaItem, int64, error) {
	items, total, err := a.svc.ListPersonas(ctx, status, q, sort, dir, limit, offset)
	if err != nil {
		return nil, 0, err
	}

	out := make([]ai.AgentPersonaItem, 0, len(items))
	for _, p := range items {
		out = append(out, ai.AgentPersonaItem{
			ID:              p.ID,
			ProfileName:     p.ProfileName,
			BackingUsername: p.BackingUsername,
			DisclosureMode:  p.DisclosureMode,
			AutonomyMode:    p.AutonomyMode,
			VoiceID:         p.VoiceID,
			VoiceName:       p.VoiceName,
			VoicePrompt:     p.VoicePrompt,
			ActivityPolicy:  p.ActivityPolicy,
			Status:          p.Status,
			CreatedAt:       p.CreatedAt,
			TotalActions:    p.TotalActions,
		})
	}

	return out, total, nil
}

func (a *agentPersonaCatalogAdapter) GetPersona(ctx context.Context, id pgtype.UUID) (ai.AgentPersonaItem, error) {
	d, err := a.svc.GetPersona(ctx, id)
	if err != nil {
		return ai.AgentPersonaItem{}, err
	}

	return ai.AgentPersonaItem{
		ID:              d.ID,
		ProfileName:     d.ProfileName,
		BackingUsername: d.BackingUsername,
		DisclosureMode:  d.DisclosureMode,
		AutonomyMode:    d.AutonomyMode,
		VoiceID:         d.VoiceID,
		VoiceName:       d.VoiceName,
		VoicePrompt:     d.VoicePrompt,
		ActivityPolicy:  d.ActivityPolicy,
		Status:          d.Status,
		CreatedAt:       d.CreatedAt,
	}, nil
}

// ---------------------------------------------------------------------------
// Agent data tools (spec 2026-09-23-agent-data-api)
// ---------------------------------------------------------------------------

// desktopExchangeAuditor implements desktopauth.AuditLogger by writing
// key-issuance records to audit_logs (actor = key owner, action
// desktop.exchange_issuance, metadata carries the key id/prefix + device).
// Minimal wiring so ExchangeKey issuance is traceable.
type desktopExchangeAuditor struct{ db *db.DB }

func (a *desktopExchangeAuditor) LogExchangeIssuance(ctx context.Context, userID, keyID pgtype.UUID, keyPrefix, deviceName, clientIP string) error {
	meta, _ := json.Marshal(map[string]string{
		"key_id":      keyID.String(),
		"key_prefix":  keyPrefix,
		"device_name": deviceName,
	})

	var ipArg any
	if clientIP != "" {
		ipArg = clientIP
	}

	_, err := a.db.Writer.Exec(ctx, `
		INSERT INTO audit_logs (actor_user_id, action_type, target_type, target_id, metadata, ip_address)
		VALUES ($1, 'desktop.exchange_issuance', 'api_key', $2, $3::jsonb, $4::inet)
	`, userID, keyID, meta, ipArg)
	if err != nil {
		return fmt.Errorf("desktop exchange audit log: %w", err)
	}

	return nil
}

// agentDataAdapter adapts *publicapi.Service to ai.AgentDataReader. The ai
// package cannot import publicapi (publicapi imports visaroutes, which
// imports ai — an ai → publicapi edge would be a cycle), so the adapter
// delegates in the wiring layer and returns the domain DTOs as opaque
// payloads that serialize verbatim into tool output.
type agentDataAdapter struct{ svc *publicapi.Service }

func (a *agentDataAdapter) ListVisaRoutes(ctx context.Context, dest pgtype.UUID, profession, education string, experienceYears, page, limit int32) (any, int64, error) {
	return a.svc.ListVisaRoutes(ctx, dest, profession, education, experienceYears, page, limit)
}

func (a *agentDataAdapter) GetVisaRoute(ctx context.Context, id pgtype.UUID) (any, error) {
	return a.svc.GetVisaRoute(ctx, id)
}

func (a *agentDataAdapter) CompareVisaRoutes(ctx context.Context, ids []pgtype.UUID) (any, error) {
	return a.svc.CompareVisaRoutes(ctx, ids)
}

func (a *agentDataAdapter) GetUniversity(ctx context.Context, slug string) (any, error) {
	return a.svc.GetUniversity(ctx, slug)
}

func (a *agentDataAdapter) GetDestinationBrief(ctx context.Context, slug string, include []string) (any, error) {
	return a.svc.GetDestinationBrief(ctx, slug, include)
}

// --- Agent data API Phase 2 (spec 2026-09-24-agent-data-api-phase2) ---

func (a *agentDataAdapter) ExploreFeed(ctx context.Context, name, sort string, limit, offset int32) (any, error) {
	return a.svc.ExploreFeed(ctx, name, sort, limit, offset)
}

// TagFeed bundles the item page and its opaque next_cursor into a single
// payload — the interface returns one any value, so the cursor rides inside.
func (a *agentDataAdapter) TagFeed(ctx context.Context, tagSlug, cursor string, limit int32) (any, error) {
	items, nextCursor, err := a.svc.TagFeed(ctx, tagSlug, cursor, limit)
	if err != nil {
		return nil, err
	}

	return map[string]any{"items": items, "next_cursor": nextCursor}, nil
}

func (a *agentDataAdapter) SpaceFeed(ctx context.Context, viewerID pgtype.UUID, slug, sort string, limit, offset int32) (any, error) {
	return a.svc.SpaceFeed(ctx, viewerID, slug, sort, limit, offset)
}

func (a *agentDataAdapter) MyFeed(ctx context.Context, userID pgtype.UUID, sort string, limit, offset int32) (any, error) {
	return a.svc.MyFeed(ctx, userID, sort, limit, offset)
}

func (a *agentDataAdapter) ListScamAlerts(ctx context.Context, destSpaceID pgtype.UUID, scamType string, limit, offset int32) (any, int64, error) {
	return a.svc.ListScamAlerts(ctx, destSpaceID, scamType, limit, offset)
}

func (a *agentDataAdapter) GetScamAlert(ctx context.Context, id pgtype.UUID) (any, error) {
	return a.svc.GetScamAlert(ctx, id)
}

func (a *agentDataAdapter) SemanticSearch(ctx context.Context, viewerID pgtype.UUID, query, lang, spaceSlug string, limit int32) (any, error) {
	return a.svc.SemanticSearch(ctx, viewerID, query, lang, spaceSlug, limit)
}

// ---------------------------------------------------------------------------
// Forum Q&A adapter (Section 08) — bridges ai.ForumService (mirror input
// types) to internal/forum.Service. Author/moderator RBAC is enforced
// inside the service; the executor's required_permission gate runs first.
// ---------------------------------------------------------------------------

type agentForumAdapter struct{ svc *forum.Service }

func (a *agentForumAdapter) CreateThread(ctx context.Context, authorUserID pgtype.UUID, in ai.ForumThreadInput) (gen.ForumThread, error) {
	out := forum.CreateThreadInput{
		ContentID:   in.ContentID,
		SpaceID:     in.SpaceID,
		Title:       in.Title,
		Slug:        in.Slug,
		Body:        in.Body,
		IsAnonymous: in.IsAnonymous,
	}

	if in.Poll != nil {
		p := &forum.PollInput{
			Question:      in.Poll.Question,
			IsMultiSelect: in.Poll.IsMultiSelect,
			CloseAt:       in.Poll.CloseAt,
		}
		for _, o := range in.Poll.Options {
			p.Options = append(p.Options, forum.PollOptionInput{Label: o.Label, Position: o.Position})
		}

		out.Poll = p
	}

	return a.svc.CreateThread(ctx, authorUserID, out)
}

func (a *agentForumAdapter) GetThread(ctx context.Context, id pgtype.UUID) (gen.ForumThread, error) {
	return a.svc.GetThread(ctx, id)
}

func (a *agentForumAdapter) GetThreadBySlug(ctx context.Context, slug string) (gen.ForumThread, error) {
	return a.svc.GetThreadBySlug(ctx, slug)
}

func (a *agentForumAdapter) ListThreadsBySpace(ctx context.Context, spaceID pgtype.UUID, cursorCreatedAt pgtype.Timestamptz, limit int32) ([]gen.ForumThread, error) {
	return a.svc.ListThreadsBySpace(ctx, spaceID, cursorCreatedAt, limit)
}

func (a *agentForumAdapter) ListThreadsByPlatform(ctx context.Context, cursorCreatedAt pgtype.Timestamptz, limit int32) ([]gen.ForumThread, error) {
	return a.svc.ListThreadsByPlatform(ctx, cursorCreatedAt, limit)
}

func (a *agentForumAdapter) UpdateThread(ctx context.Context, id, userID pgtype.UUID, title string, body []byte, isAnonymous bool) (gen.ForumThread, error) {
	return a.svc.UpdateThread(ctx, id, userID, title, body, isAnonymous)
}

func (a *agentForumAdapter) DeleteThread(ctx context.Context, id, userID pgtype.UUID) error {
	return a.svc.DeleteThread(ctx, id, userID)
}

func (a *agentForumAdapter) SolveThread(ctx context.Context, id, solverUserID, acceptedReplyID pgtype.UUID) error {
	return a.svc.SolveThread(ctx, id, solverUserID, acceptedReplyID)
}

func (a *agentForumAdapter) UnsolveThread(ctx context.Context, id, userID pgtype.UUID) error {
	return a.svc.UnsolveThread(ctx, id, userID)
}

func (a *agentForumAdapter) CreateReply(ctx context.Context, threadID, authorUserID pgtype.UUID, in ai.ForumReplyInput) (gen.ForumReply, error) {
	return a.svc.CreateReply(ctx, threadID, authorUserID, forum.CreateReplyInput{
		ParentReplyID: in.ParentReplyID,
		Body:          in.Body,
		IsAnonymous:   in.IsAnonymous,
	})
}

func (a *agentForumAdapter) ListRepliesByThread(ctx context.Context, threadID pgtype.UUID) ([]gen.ForumReply, error) {
	return a.svc.ListRepliesByThread(ctx, threadID)
}

func (a *agentForumAdapter) UpdateReply(ctx context.Context, id, userID pgtype.UUID, body []byte) (gen.ForumReply, error) {
	return a.svc.UpdateReply(ctx, id, userID, body)
}

func (a *agentForumAdapter) DeleteReply(ctx context.Context, id, userID pgtype.UUID) error {
	return a.svc.DeleteReply(ctx, id, userID)
}

func (a *agentForumAdapter) Vote(ctx context.Context, userID pgtype.UUID, entityType string, entityID pgtype.UUID, value int16) (gen.ForumVote, error) {
	return a.svc.Vote(ctx, userID, forum.VoteInput{EntityType: entityType, EntityID: entityID, Value: value})
}

func (a *agentForumAdapter) RemoveVote(ctx context.Context, userID pgtype.UUID, entityType string, entityID pgtype.UUID) error {
	return a.svc.RemoveVote(ctx, userID, entityType, entityID)
}

// ---------------------------------------------------------------------------
// Comments adapter (Section 49).
// ---------------------------------------------------------------------------

type agentCommentsAdapter struct{ svc *comments.Service }

func (a *agentCommentsAdapter) CreateComment(ctx context.Context, userID pgtype.UUID, in ai.CommentCreateInput) (gen.Comment, error) {
	return a.svc.CreateComment(ctx, userID, comments.CreateCommentInput{
		ContentID:   in.ContentID,
		ParentID:    in.ParentID,
		Body:        in.Body,
		IsAnonymous: in.IsAnonymous,
	})
}

func (a *agentCommentsAdapter) GetComment(ctx context.Context, commentID pgtype.UUID) (gen.GetCommentRow, error) {
	return a.svc.GetComment(ctx, commentID)
}

func (a *agentCommentsAdapter) ListCommentsByContent(ctx context.Context, contentID, viewerID pgtype.UUID, sort string, limit, offset int32) ([]gen.ListCommentsByContentRow, error) {
	return a.svc.ListCommentsByContent(ctx, contentID, viewerID, sort, limit, offset)
}

func (a *agentCommentsAdapter) ListRepliesByParent(ctx context.Context, parentID, viewerID pgtype.UUID, limit, offset int32) ([]gen.ListRepliesByParentRow, error) {
	return a.svc.ListRepliesByParent(ctx, parentID, viewerID, limit, offset)
}

func (a *agentCommentsAdapter) UpdateComment(ctx context.Context, commentID, userID pgtype.UUID, body []byte) (gen.Comment, error) {
	return a.svc.UpdateComment(ctx, commentID, userID, body)
}

func (a *agentCommentsAdapter) SoftDeleteCommentByAuthor(ctx context.Context, commentID, userID pgtype.UUID) error {
	return a.svc.SoftDeleteCommentByAuthor(ctx, commentID, userID)
}

func (a *agentCommentsAdapter) VoteComment(ctx context.Context, commentID, userID pgtype.UUID, value int16) error {
	return a.svc.VoteComment(ctx, commentID, userID, value)
}

func (a *agentCommentsAdapter) RemoveCommentVote(ctx context.Context, commentID, userID pgtype.UUID) error {
	return a.svc.RemoveCommentVote(ctx, commentID, userID)
}

// ---------------------------------------------------------------------------
// Deterministic calculator adapters (§107 CRS, §108 POF, §112 FX).
// ---------------------------------------------------------------------------

type agentFXAdapter struct{ svc *fx.Service }

func (a *agentFXAdapter) Convert(ctx context.Context, amount float64, from, to string) (*ai.FXConvertResult, error) {
	r, err := a.svc.Convert(ctx, amount, from, to)
	if err != nil {
		return nil, err
	}

	return &ai.FXConvertResult{
		Amount:    r.Amount,
		From:      r.From,
		To:        r.To,
		Rate:      r.Rate,
		Converted: r.Converted,
		FetchedAt: r.FetchedAt,
		Stale:     r.Stale,
	}, nil
}

type agentCRSAdapter struct{ svc *crscalculator.Service }

func (a *agentCRSAdapter) Calculate(ctx context.Context, userID *pgtype.UUID, input json.RawMessage) (map[string]any, error) {
	var req crscalculator.CalculateRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, fmt.Errorf("invalid crs.calculate arguments: %w", err)
	}

	result, err := a.svc.Calculate(ctx, userID, req)
	if err != nil {
		return nil, err
	}

	return structToMap(result)
}

type agentPOFAdapter struct{ svc *pofcalculator.Service }

func (a *agentPOFAdapter) Calculate(ctx context.Context, in ai.POFCalculateInput) ([]map[string]any, error) {
	var destID pgtype.UUID
	if err := destID.Scan(in.DestSpaceID); err != nil {
		return nil, fmt.Errorf("invalid dest_space_id: %w", err)
	}

	results, err := a.svc.Calculate(ctx, destID, in.VisaType, in.Members, in.Dependants)
	if err != nil {
		return nil, err
	}

	out := make([]map[string]any, 0, len(results))

	for _, r := range results {
		m, err := structToMap(r)
		if err != nil {
			return nil, err
		}

		out = append(out, m)
	}

	return out, nil
}

// agentWikiAdapter bridges the wiki §80 revision-queue surface onto the
// ai.WikiAgentService mirror interface (internal/wiki imports internal/ai,
// so the ai package cannot reference the concrete types).
type agentWikiAdapter struct {
	svc   *wiki.Service
	store *wiki.Store
}

func (a *agentWikiAdapter) CreatePageWithRevision(ctx context.Context, userID pgtype.UUID, in ai.WikiCreatePageInput) (string, string, string, error) {
	pageID, revID, pageSlug, _, err := a.svc.CreatePageWithRevision(ctx, userID, wiki.CreateWikiPageInput{
		SpaceSlug: in.SpaceSlug,
		Slug:      in.Slug,
		Title:     in.Title,
		Category:  in.Category,
		Content:   in.Content,
		Summary:   in.Summary,
		Position:  in.Position,
	})

	return pageID, revID, pageSlug, err
}

func (a *agentWikiAdapter) CreatePageShell(ctx context.Context, spaceID, createdBy pgtype.UUID, slug, title, category string) (string, error) {
	id, err := a.store.CreatePage(ctx, spaceID, createdBy, slug, title, category)
	if err != nil {
		return "", err
	}

	return id.String(), nil
}

func (a *agentWikiAdapter) SubmitRevision(ctx context.Context, userID pgtype.UUID, spaceSlug, pageSlug string, content json.RawMessage, summary string) (string, error) {
	revID, _, _, err := a.svc.SubmitRevision(ctx, userID, spaceSlug, pageSlug, wiki.SubmitRevisionInput{
		Content: content,
		Summary: summary,
	})

	return revID, err
}

func (a *agentWikiAdapter) SubmitFieldOps(ctx context.Context, userID pgtype.UUID, spaceSlug, pageSlug string, content, opsRaw json.RawMessage, summary string, canManage bool) (string, bool, error) {
	ops, err := wiki.ParseOps(opsRaw)
	if err != nil {
		return "", false, fmt.Errorf("invalid ops: %w", err)
	}

	return a.svc.SubmitPageFieldOps(ctx, userID, spaceSlug, pageSlug, content, ops, summary, canManage)
}

func (a *agentWikiAdapter) StructuredEditsEnabled(ctx context.Context) bool {
	return a.svc.StructuredEditsEnabled(ctx)
}

func (a *agentWikiAdapter) ApproveRevision(ctx context.Context, revID, reviewerID pgtype.UUID, notes string) error {
	return a.svc.ApproveRevisionByID(ctx, revID, reviewerID, notes)
}

func (a *agentWikiAdapter) RejectRevision(ctx context.Context, revID, reviewerID pgtype.UUID, notes string) error {
	return a.svc.RejectRevisionByID(ctx, revID, reviewerID, notes)
}

func (a *agentWikiAdapter) CommunityEditsAllowed(ctx context.Context) bool {
	return a.svc.CommunityEditsAllowed(ctx)
}

func (a *agentWikiAdapter) MinReputationToEdit(ctx context.Context) int {
	return a.svc.MinReputationToEdit(ctx)
}

func (a *agentWikiAdapter) SpaceSlugByID(ctx context.Context, spaceID pgtype.UUID) (string, error) {
	slug, _, err := a.store.GetSpaceByID(ctx, spaceID)
	return slug, err
}

func (a *agentWikiAdapter) SpaceIDBySlug(ctx context.Context, spaceSlug string) (pgtype.UUID, error) {
	id, _, err := a.store.GetSpaceBySlug(ctx, spaceSlug)
	return id, err
}

// ListPages maps the ai filter onto Service.AdminListPages — space_slug is
// passed through as the dest filter (the REST surface accepts a slug there).
func (a *agentWikiAdapter) ListPages(ctx context.Context, filter ai.WikiPageListFilter) ([]ai.WikiPageListItem, int64, error) {
	dtos, total, err := a.svc.AdminListPages(ctx, pgtype.UUID{}, filter.SpaceSlug, filter.Category, filter.Q, "", filter.Status, nil, nil, filter.IsActive, "", filter.Page, filter.Limit)
	if err != nil {
		return nil, 0, err
	}

	items := make([]ai.WikiPageListItem, 0, len(dtos))

	for _, d := range dtos {
		item := ai.WikiPageListItem{
			ID:                   d.ID,
			SpaceID:              d.SpaceID,
			SpaceSlug:            d.SpaceSlug,
			Slug:                 d.Slug,
			Title:                d.Title,
			Category:             d.Category,
			IsActive:             d.IsActive,
			IsLocked:             d.IsLocked,
			IsFeatured:           d.IsFeatured,
			LatestRevisionID:     d.LatestRevisionID,
			LatestRevisionStatus: d.LatestRevisionStatus,
			Excerpt:              d.Excerpt,
			AiGenerated:          d.AiGenerated,
			FieldGroupID:         d.FieldGroupID,
			FieldGroupSlug:       d.FieldGroupSlug,
			FieldGroupName:       d.FieldGroupName,
			UpdatedAt:            d.UpdatedAt,
			DeletedAt:            d.DeletedAt,
		}
		if d.DataEntityType != nil {
			item.DataEntityType = *d.DataEntityType
		}

		items = append(items, item)
	}

	return items, total, nil
}

func (a *agentWikiAdapter) GetPageDetail(ctx context.Context, userID pgtype.UUID, spaceSlug, pageSlug string) (*ai.WikiPageDetail, error) {
	d, err := a.svc.GetWikiPage(ctx, userID, spaceSlug, pageSlug)
	if err != nil {
		return nil, err
	}

	detail := &ai.WikiPageDetail{
		ID:             d.ID,
		SpaceID:        d.SpaceID,
		SpaceSlug:      d.SpaceSlug,
		SpaceName:      d.SpaceName,
		Slug:           d.Slug,
		Title:          d.Title,
		Category:       d.Category,
		IsActive:       d.IsActive,
		IsLocked:       d.IsLocked,
		IsFeatured:     d.IsFeatured,
		CurrentContent: d.CurrentContent,
		FieldGroupID:   d.FieldGroupID,
		FieldGroupSlug: d.FieldGroupSlug,
		CategoryLabel:  d.CategoryLabel,
		Fields:         wikiFieldGroupToAI(d.Fields),
		UpdatedAt:      d.UpdatedAt,
	}
	if d.DataEntityType != nil {
		detail.DataEntityType = *d.DataEntityType
	}

	return detail, nil
}

func (a *agentWikiAdapter) ListRevisionHistory(ctx context.Context, viewerID pgtype.UUID, canReview bool, spaceSlug, pageSlug string, page, limit int32) ([]ai.WikiRevisionInfo, int64, error) {
	dtos, total, err := a.svc.GetRevisionHistory(ctx, viewerID, canReview, spaceSlug, pageSlug, page, limit)
	return wikiRevisionsToAI(dtos), total, err
}

func (a *agentWikiAdapter) ListRevisions(ctx context.Context, kind, status, q string, page, limit int32) ([]ai.WikiRevisionInfo, int64, error) {
	dtos, total, err := a.svc.AdminListRevisions(ctx, kind, status, q, "", "", page, limit)
	return wikiRevisionsToAI(dtos), total, err
}

func (a *agentWikiAdapter) ListEntityRevisions(ctx context.Context, viewerID pgtype.UUID, canReview bool, entityType string, entityID pgtype.UUID, page, limit int32) ([]ai.WikiRevisionInfo, int64, error) {
	dtos, total, err := a.svc.ListEntityRevisions(ctx, viewerID, canReview, entityType, entityID, page, limit)
	return wikiRevisionsToAI(dtos), total, err
}

func (a *agentWikiAdapter) GetEntityFields(ctx context.Context, userID pgtype.UUID, entityType string, entityID pgtype.UUID) (*ai.WikiEntityFields, error) {
	dto, err := a.svc.GetEntityFields(ctx, userID, entityType, entityID)
	if err != nil {
		return nil, err
	}

	groups := make([]ai.WikiEntityFieldGroup, 0, len(dto.Groups))
	for i := range dto.Groups {
		groups = append(groups, *wikiFieldGroupToAI(&dto.Groups[i]))
	}

	return &ai.WikiEntityFields{
		Groups:             groups,
		PendingCount:       dto.PendingCount,
		CanPropose:         dto.CanPropose,
		CanManage:          dto.CanManage,
		CanDelete:          dto.CanDelete,
		EntityClass:        dto.EntityClass,
		EntityCurrencyCode: dto.EntityCurrencyCode,
	}, nil
}

func wikiFieldGroupToAI(g *wiki.EntityFieldGroupDTO) *ai.WikiEntityFieldGroup {
	if g == nil {
		return nil
	}

	fields := make([]ai.WikiEntityField, 0, len(g.Fields))
	for _, f := range g.Fields {
		fields = append(fields, ai.WikiEntityField{
			ID:        f.ID,
			Key:       f.Key,
			Label:     f.Label,
			FieldType: f.FieldType,
			Options:   f.Options,
			Value:     f.Value,
			CanEdit:   f.CanEdit,
			Visible:   f.Visible,
			Section:   f.Section,
			Computed:  f.Computed,
			System:    f.System,
		})
	}

	return &ai.WikiEntityFieldGroup{
		ID:     g.ID,
		Slug:   g.Slug,
		Name:   g.Name,
		Fields: fields,
	}
}

func wikiRevisionsToAI(dtos []wiki.WikiRevisionDTO) []ai.WikiRevisionInfo {
	revs := make([]ai.WikiRevisionInfo, 0, len(dtos))
	for _, d := range dtos {
		revs = append(revs, ai.WikiRevisionInfo{
			ID:               d.ID,
			Status:           d.Status,
			Kind:             d.Kind,
			Content:          d.Content,
			Ops:              d.Ops,
			Summary:          d.Summary,
			AiGenerated:      d.AiGenerated,
			TargetEntityType: d.TargetEntityType,
			TargetEntityID:   d.TargetEntityID,
			SubmittedBy:      d.SubmittedBy,
			ReviewedBy:       d.ReviewedBy,
			ReviewedAt:       d.ReviewedAt,
			WikiPageID:       d.WikiPageID,
			ApplyError:       d.ApplyError,
			CreatedAt:        d.CreatedAt,
		})
	}

	return revs
}
