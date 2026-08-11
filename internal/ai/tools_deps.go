// Package ai — tools_deps.go defines the dependency interfaces and the
// shared ToolDeps container used by the Phase 4 core tool catalog handlers
// (Section 103, ADR-111).
//
// Each tool handler is a closure over a *ToolDeps, registered with the
// ToolHandlerRegistry via RegisterHandler. The interfaces below are the
// minimal subsets of the real service signatures the handlers call, defined
// as interfaces so the handlers can be unit-tested with stubs instead of a
// live database, Meilisearch, or external HTTP API.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Tool Catalog Expansion)
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 4, tasks 4.01-4.10)
package ai

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// ---------------------------------------------------------------------------
// Service interfaces (minimal subsets the tool handlers call)
// ---------------------------------------------------------------------------

// ContentService is the subset of internal/content.Service the content tools
// call. Defined as an interface so handlers can be tested with a stub.
// The signatures match the real content.Service methods 1:1 so the wiring
// layer can assign the concrete service directly (no adapter needed).
type ContentService interface {
	// ListContents returns a tenant-wide text-filtered page of content.
	// cursor is a zero/invalid pgtype.Timestamptz for the first page.
	ListContents(ctx context.Context, tenantID pgtype.UUID, q string, cursor pgtype.Timestamptz, limit int32) ([]gen.ListContentsByTenantRow, error)
	// SearchContentsBySpace returns content within a space whose title
	// matches the query.
	SearchContentsBySpace(ctx context.Context, tenantID, spaceID pgtype.UUID, query string, limit, offset int32) ([]gen.SearchContentsBySpaceRow, error)
	// GetContent returns a single content row by ID.
	GetContent(ctx context.Context, id, tenantID pgtype.UUID) (gen.GetContentByIDRow, error)
}

// CustomFieldsService is the subset of internal/customfields.Service the
// field tools call.
type CustomFieldsService interface {
	ListFieldsByGroup(ctx context.Context, groupID, tenantID pgtype.UUID) ([]gen.ListCustomFieldsByGroupRow, error)
	ListFieldValuesByEntity(ctx context.Context, tenantID pgtype.UUID, entityType string, entityID pgtype.UUID) ([]gen.CustomFieldValue, error)
	UpsertFieldValue(ctx context.Context, params gen.UpsertCustomFieldValueParams) (gen.CustomFieldValue, error)
}

// RAGService is the subset of internal/rag.Service the knowledge tools call.
type RAGService interface {
	Retrieve(ctx context.Context, tenantID pgtype.UUID, query, language string, geoSpaceID *pgtype.UUID) ([]RAGChunk, error)
}

// RAGChunk is the provider-agnostic shape of rag.RetrievedChunk used by the
// rag.search tool. It mirrors rag.RetrievedChunk but is defined here so the
// ai package does not import internal/rag (avoiding a cycle when rag imports
// ai for the embedder adapter).
type RAGChunk struct {
	EntityID   string  `json:"entity_id"`
	EntityType string  `json:"entity_type"`
	ChunkText  string  `json:"chunk_text"`
	Similarity float64 `json:"similarity"`
	Tier       string  `json:"tier,omitempty"`
}

// SettingsReader is the subset of internal/config.Settings the tool handlers
// need for hot-reloadable system_settings lookups (Rule 3).
type SettingsReader interface {
	GetStringOr(ctx context.Context, tenantID pgtype.UUID, key, fallback string) string
	GetIntOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback int) int
	GetFloatOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback float64) float64
	GetBoolOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback bool) bool
}

// CacheClient is the minimal Valkey client interface the external.search tool
// uses for the jc:agent:external:{hash} cache. nil disables caching.
type CacheClient interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string, ttlSeconds int) error
}

// ---------------------------------------------------------------------------
// ToolDeps — shared dependency container
// ---------------------------------------------------------------------------

// ToolDeps holds the service dependencies every Phase 4 tool handler closes
// over. A nil field degrades gracefully: the handler returns an error result
// explaining the dependency is not configured, rather than panicking.
//
// ContentCreator and ContentUpdater are function-type bridges (not
// interfaces on the content.Service) because content.CreateContentInput and
// content.UpdateContentInput are content-package types — importing
// internal/content here would create a cycle risk. The wiring layer
// (internal/server) adapts the real service into these function types.
type ToolDeps struct {
	Content        ContentService
	ContentCreator ContentCreator
	ContentUpdater ContentUpdater
	CustomFields   CustomFieldsService
	RAG            RAGService
	Privacy        PrivacyExemptionChecker
	Settings       SettingsReader
	Cache          CacheClient  // nil disables external-search caching
	HTTPClient     *http.Client // nil → http.DefaultClient is used
}

// NewToolDeps creates an empty ToolDeps. Fields are set directly or via the
// With* setters below.
func NewToolDeps() *ToolDeps {
	return &ToolDeps{}
}

// WithContent injects the content service.
func (d *ToolDeps) WithContent(s ContentService) *ToolDeps { d.Content = s; return d }

// WithCustomFields injects the custom fields service.
func (d *ToolDeps) WithCustomFields(s CustomFieldsService) *ToolDeps { d.CustomFields = s; return d }

// WithRAG injects the RAG service.
func (d *ToolDeps) WithRAG(s RAGService) *ToolDeps { d.RAG = s; return d }

// WithPrivacy injects the privacy exemption checker.
func (d *ToolDeps) WithPrivacy(p PrivacyExemptionChecker) *ToolDeps { d.Privacy = p; return d }

// WithSettings injects the hot-reloadable settings reader.
func (d *ToolDeps) WithSettings(s SettingsReader) *ToolDeps { d.Settings = s; return d }

// WithCache injects the Valkey cache client for external-search caching.
func (d *ToolDeps) WithCache(c CacheClient) *ToolDeps { d.Cache = c; return d }

// WithHTTPClient injects the HTTP client used for external search API calls.
func (d *ToolDeps) WithHTTPClient(c *http.Client) *ToolDeps { d.HTTPClient = c; return d }

// httpClient returns the configured HTTP client or http.DefaultClient.
func (d *ToolDeps) httpClient() *http.Client {
	if d.HTTPClient != nil {
		return d.HTTPClient
	}
	return http.DefaultClient
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

// scanUUID parses a UUID string from tool arguments into pgtype.UUID.
// Returns the zero UUID and the error on failure.
func scanUUID(s string) (pgtype.UUID, error) {
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		return pgtype.UUID{}, err
	}
	return u, nil
}

// optionalUUID parses an optional UUID string. An empty string yields a
// zero (invalid) pgtype.UUID, signalling "not provided".
func optionalUUID(s string) (pgtype.UUID, error) {
	if s == "" {
		return pgtype.UUID{}, nil
	}
	return scanUUID(s)
}

// uuidPtr returns a *pgtype.UUID for a valid UUID, or nil when the pgtype
// value is the zero/invalid value. Used to build the *pgtype.UUID that
// rag.Service.Retrieve expects.
func uuidPtr(u pgtype.UUID) *pgtype.UUID {
	if !u.Valid {
		return nil
	}
	return &u
}
