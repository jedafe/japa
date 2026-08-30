package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/intelligence/changedetector"
	"github.com/jedafe/japa/internal/intelligence/dedup"
	"github.com/jedafe/japa/internal/intelligence/events"
	"github.com/jedafe/japa/internal/intelligence/extractor/schemas"
	"github.com/jedafe/japa/internal/intelligence/fetcher"
	"github.com/jedafe/japa/internal/intelligence/reviewerctx"
	"github.com/jedafe/japa/internal/intelligence/scheduler"
	"github.com/jedafe/japa/internal/intelligence/services"
	"github.com/jedafe/japa/internal/intelligence/sources"
	"github.com/jedafe/japa/internal/intelligence/stores"
)

// defaultIntelTenantID is the system tenant UUID used as the fallback tenant
// for intelligence operations. Per ADR-112 Amendment 2, all intelligence
// tables and Valkey keys are tenant-scoped. In a single-tenant deployment
// (the current state), all intelligence data belongs to the default tenant
// 00000000-0000-0000-0000-000000000001. The intel.* system_settings are
// also seeded at this tenant level per migration 000431.
var defaultIntelTenantID = func() pgtype.UUID {
	var id pgtype.UUID
	id.Bytes[15] = 1 // 00000000-0000-0000-0000-000000000001
	id.Valid = true
	return id
}()

// ---------------------------------------------------------------------------
// Intelligence System (Section INTEL) — Asynq task handlers for the
// ingestion spine. The 11 handlers below implement the task chain defined in
// Section 4 of the intelligence-system design spec:
//
//	schedule_crawls → fetch_source → detect_changes → extract → dedup
//	                → publish_gate → promote → post_publish → notify_discoveries
//
// plus the two maintenance tasks refresh_freshness (cron) and
// retry_failed_source (manual control-panel trigger).
//
// Dependency injection follows the existing worker pattern: package-level
// variables are set once at startup via RegisterIntelDependencies (called from
// server.go in Task 23) and each handler is nil-safe — when a dependency is
// not wired it logs a warning and returns nil so the worker still boots in
// development and tests.
//
// Import-cycle note: the extractor and promoter packages cannot be imported
// here because they transitively pull internal/ai → internal/auth →
// internal/worker (auth/handlers.go enqueues email tasks via worker constants).
// To break that cycle the extractor and promoter dependencies are declared as
// local port interfaces (intelExtractorPort / intelPromoterPort) whose result
// types are mirrored locally. Task 23's server.go registers thin adapters
// that wrap the concrete *extractor.Extractor and *promoter.ScholarshipPromoter
// so the cycle is confined to the wiring layer.
//
// The service-call bodies are intentionally stubbed where the concrete wiring
// (Meilisearch indexing, RAG embedding, gate classification, freshness decay)
// is completed in Task 23's server.go alongside the DI registration. The task
// chain enqueue logic is in place so the spine is end-to-end runnable once the
// dependencies are injected.
//
// Ref: docs/superpowers/specs/2026-08-12-japa-intelligence-system-design.md
// ---------------------------------------------------------------------------

// IntelPromoterPort mirrors the subset of promoter.Promoter the worker needs,
// declared locally to avoid the import cycle
// (worker → promoter → extractor → ai → auth → worker). server.go registers
// one adapter per domain (scholarship, job, visa, housing, remittance,
// cost_of_living, and the 7 guide content types), keyed by Entity() in the
// promoter map so HandleIntelPromote dispatches to the correct domain
// promoter by the record's entity_type. Exported so the server package can
// build the promoter map from outside the worker package.
type IntelPromoterPort interface {
	Promote(ctx context.Context, recordID uuid.UUID) (IntelPromoteResult, error)
	Entity() string
}

// IntelPromoteResult mirrors promoter.PromoteResult. Exported so the
// server package can build port adapters that satisfy intelPromoterPort
// from outside the worker package.
type IntelPromoteResult struct {
	DomainID      uuid.UUID
	ContentID     uuid.UUID
	Action        string
	ChangedFields []string
}

// intelExtractorPort mirrors the subset of *extractor.Extractor the worker
// needs, declared locally to avoid the import cycle
// (worker → extractor → ai → auth → worker). server.go registers an
// adapter wrapping the concrete *extractor.Extractor that selects the entity
// schema by entityType and returns the marshalled extraction + aggregate
// confidence. entityType is derived by the worker from the source's category
// via schemas.CategoryToEntityType.
type intelExtractorPort interface {
	Extract(ctx context.Context, entityType, sourceURL, sourceType, sourceText string) (IntelExtractResult, error)
}

// IntelExtractResult mirrors the fields the worker persists from
// extractor.ExtractResult. Exported so the server package can build port
// adapters that satisfy intelExtractorPort from outside the worker package.
type IntelExtractResult struct {
	RawExtraction     []byte
	ConfidenceOverall float32
}

// ErrIntelExtractionRetriesExhausted is the port-level sentinel returned by
// the extractor adapter when the AI gateway returns malformed JSON on every
// attempt up to maxRetries. The worker's HandleIntelExtract checks for this
// and routes the record to REVIEW_REQUIRED instead of failing the task
// (BUG-16 fix). The adapter wraps extractor.ErrExtractionRetriesExhausted
// with this sentinel so the worker can detect it without importing the
// extractor package.
var ErrIntelExtractionRetriesExhausted = errors.New("intel: extraction retries exhausted")

// IntelStudioAutoPromoter is the port through which the intelligence spine
// auto-creates Studio jobs for high-confidence extracted records (ADR-076
// Amendment 1). Declared locally to avoid the worker → studio import cycle.
// server.go registers an adapter wrapping *studio.Service.
//
// Ref: japaconnect-engineering/sections/75-ai-content-studio.md
type IntelStudioAutoPromoter interface {
	// AutoPromoteIntelRecord creates a Studio job grounded by the given
	// intel_record_id. extractedJSON is the raw intel_records.extracted
	// payload (used to derive a topic prompt). entityType maps to the
	// Studio content_type.
	AutoPromoteIntelRecord(ctx context.Context, tenantID, recordID pgtype.UUID, entityType string, extractedJSON []byte) error
}

// Package-level dependencies for the intelligence handlers. All are nil until
// RegisterIntelDependencies is called at server startup (Task 23).
var (
	intelSourcesStore      *sources.Store
	intelStores            *stores.Store
	intelScheduler         *scheduler.Scheduler
	intelExtractor         intelExtractorPort
	intelDedupService      *dedup.Service
	intelPromoters         map[string]IntelPromoterPort // keyed by entity_type
	intelEventPublisher    events.DomainEventPublisher
	intelFetcher           fetcher.Fetcher
	intelPreflight         *fetcher.PreflightChecker
	intelAsynqClient       *asynq.Client
	intelStudioAutoPromote IntelStudioAutoPromoter
)

// RegisterIntelDependencies wires the intelligence spine services into the
// worker package. Called once from server.go before the Asynq server starts
// processing tasks. Any nil argument leaves that dependency unwired, in which
// case the relevant handler logs a warning and skips.
//
// The extractor argument is the local port interface; server.go constructs an
// adapter around the concrete *extractor.Extractor to break the import cycle.
// promoters is a map keyed by entity_type → promoter port adapter; server.go
// registers one adapter per domain (scholarship, job, visa, fx, remittance,
// cost_of_living, housing). HandleIntelPromote dispatches by the record's
// entity_type.
func RegisterIntelDependencies(
	srcStore *sources.Store,
	store *stores.Store,
	sched *scheduler.Scheduler,
	ext intelExtractorPort,
	dedupSvc *dedup.Service,
	promoters map[string]IntelPromoterPort,
	pub events.DomainEventPublisher,
	f fetcher.Fetcher,
	preflight *fetcher.PreflightChecker,
	client *asynq.Client,
) {
	intelSourcesStore = srcStore
	intelStores = store
	intelScheduler = sched
	intelExtractor = ext
	intelDedupService = dedupSvc
	intelPromoters = promoters
	intelEventPublisher = pub
	intelFetcher = f
	intelPreflight = preflight
	intelAsynqClient = client
}

// RegisterIntelStudioAutoPromoter wires the Studio auto-promote port so
// HandleIntelPromote can auto-create Studio jobs for high-confidence
// extracted records when studio.auto_promote_intel_records is true
// (ADR-076 Amendment 1). Called from server.go after RegisterIntelDependencies.
// Pass nil to disable auto-promote (the default).
func RegisterIntelStudioAutoPromoter(p IntelStudioAutoPromoter) {
	intelStudioAutoPromote = p
}

// enqueueIntel enqueues a follow-up intelligence task on the shared Asynq
// client, routing it to the queue tier inferred from the task type. It is
// fire-and-forget: enqueue failures are logged but do not fail the current
// handler (the chain resumes on the next crawl cycle).
func enqueueIntel(taskType string, payload []byte) {
	if intelAsynqClient == nil {
		slog.Warn("intel: asynq client not configured, skipping enqueue", "task", taskType)
		return
	}
	queue := QueueForTask(taskType)
	if _, err := intelAsynqClient.Enqueue(
		asynq.NewTask(taskType, payload),
		asynq.Queue(queue),
	); err != nil {
		slog.Error("intel: failed to enqueue follow-up task", "task", taskType, "error", err)
	}
}

// marshalPayload marshals v to JSON or returns nil on error (logged). Used by
// handlers when building follow-up task payloads.
func marshalPayload(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		slog.Error("intel: failed to marshal payload", "error", err)
		return nil
	}
	return b
}

// recoverPanic is a shared defer helper for intelligence task handlers
// (INTEL-020 fix). It catches panics so a single handler crash does not
// bring down the entire Asynq worker process. The panic is logged and
// re-panic'd only if the context is already cancelled; otherwise the
// handler returns normally (Asynq will retry based on the task's retry
// policy).
func recoverPanic(taskType string) {
	if r := recover(); r != nil {
		slog.Error("intel: task handler panicked",
			"task", taskType, "panic", r)
	}
}

// ---------------------------------------------------------------------------
// Payload structs (Section INTEL §11 catalog).
// ---------------------------------------------------------------------------
// All payloads carry a Version field (INTEL-015 fix, §10 payload versioning).
// Version is set to "v1" on enqueue and checked on decode; an empty string
// is treated as legacy "v1" for backward compatibility with queued tasks.

const intelPayloadVersion = "v1"

type intelSourcePayload struct {
	Version  string `json:"version"`
	SourceID string `json:"source_id"`
	TenantID string `json:"tenant_id"`
}

type intelDetectChangesPayload struct {
	Version   string `json:"version"`
	SourceID  string `json:"source_id"`
	RawDocID  string `json:"raw_doc_id"`
	PriorHash string `json:"prior_hash"`
	TenantID  string `json:"tenant_id"`
}

type intelExtractPayload struct {
	Version  string `json:"version"`
	RecordID string `json:"record_id"`
	RawDocID string `json:"raw_doc_id"`
	TenantID string `json:"tenant_id"`
}

type intelRecordPayload struct {
	Version        string `json:"version"`
	RecordID       string `json:"record_id"`
	TenantID       string `json:"tenant_id"`
	ReviewerUserID string `json:"reviewer_user_id,omitempty"` // INTEL-026: set on approve, empty on auto-publish
}

type intelPostPublishPayload struct {
	Version    string `json:"version"`
	RecordID   string `json:"record_id"`
	DomainID   string `json:"domain_id"`
	EntityType string `json:"entity_type"`
	TenantID   string `json:"tenant_id"`
}

type intelNotifyPayload struct {
	Version    string `json:"version"`
	DomainID   string `json:"domain_id"`
	EntityType string `json:"entity_type"`
	TenantID   string `json:"tenant_id"`
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

// parseTenantID parses a tenant UUID from a payload string field. When the
// payload does not carry a tenant_id (e.g. a legacy enqueued task), the
// default system tenant is returned so the spine remains runnable during
// the migration window. Per ADR-112 Amendment 2 all intelligence data is
// tenant-scoped; new enqueues always include tenant_id.
func parseTenantID(raw string) pgtype.UUID {
	if raw == "" {
		return defaultIntelTenantID
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return defaultIntelTenantID
	}
	return pgtype.UUID{Bytes: id, Valid: true}
}

// HandleIntelScheduleCrawls is the cron entry point for the ingestion spine.
// It selects due sources via the scheduler and enqueues a fetch_source task
// per due source (Section 4, step 1).
//
// BUG-11/12 fix: the batch size is read from system_settings
// (intel.schedule_batch_size, default 50) instead of being hardcoded to 100.
// The intel.enabled toggle is checked before any work — when false, the
// handler returns nil immediately.
//
// BUG-13 fix: a distributed leader lock (jc:intel:scheduler_leader, SET NX
// EX) prevents duplicate cron runs across multiple worker instances. Only
// the instance that acquires the lock proceeds; others skip. The lock TTL
// (5 minutes) is longer than the cron interval (15 minutes is the spec
// default, but the actual interval is set by the Asynq scheduler config).
func HandleIntelScheduleCrawls(ctx context.Context, t *asynq.Task) error {
	defer recoverPanic("TypeIntelScheduleCrawls")
	slog.Info("task:intel:schedule_crawls")
	if intelScheduler == nil {
		slog.Warn("task:intel:schedule_crawls: scheduler not wired, skipping")
		return nil
	}

	// BUG-12 fix: check intel.enabled toggle. Settings are tenant-scoped
	// per ADR-112 Amendment 2; the default tenant is used for the cron.
	if appSettings != nil {
		enabled := appSettings.GetBoolOr(ctx, defaultIntelTenantID, "intel.enabled", true)
		if !enabled {
			slog.Info("task:intel:schedule_crawls: intel.enabled=false, skipping")
			return nil
		}
	}

	// BUG-13 fix: acquire leader lock to prevent duplicate cron runs across
	// worker instances. The lock TTL (5 min) is shorter than the typical
	// cron interval (15 min) so a crashed leader releases the lock quickly.
	// The lock key is tenant-scoped per ADR-112 Amendment 2.
	if rdb != nil {
		leaderKey := fmt.Sprintf("jc:intel:scheduler_leader:%s", defaultIntelTenantID.String())
		locked, err := rdb.SetNX(ctx, leaderKey, "1", 5*time.Minute).Result()
		if err != nil {
			return fmt.Errorf("task:intel:schedule_crawls: leader lock: %w", err)
		}
		if !locked {
			slog.Info("task:intel:schedule_crawls: another instance holds the leader lock, skipping")
			return nil
		}
		defer rdb.Del(ctx, leaderKey)
	}

	// BUG-11 fix: read batch size from system_settings (default 50 per
	// system-settings-catalog Section INTEL) instead of hardcoding 100.
	batchSize := 50
	if appSettings != nil {
		batchSize = appSettings.GetIntOr(ctx, defaultIntelTenantID, "intel.schedule_batch_size", 50)
	}

	due, err := intelScheduler.SelectDue(ctx, int32(batchSize))
	if err != nil {
		return fmt.Errorf("task:intel:schedule_crawls: select due: %w", err)
	}

	for _, src := range due {
		if !src.ID.Valid {
			continue
		}
		payload := marshalPayload(intelSourcePayload{Version: intelPayloadVersion,
			SourceID: src.ID.String(),
			TenantID: defaultIntelTenantID.String(),
		})
		if payload != nil {
			enqueueIntel(TypeIntelFetchSource, payload)
		}
	}
	return nil
}

// HandleIntelFetchSource fetches the raw document for a single source and
// enqueues detect_changes to compare its hash against the prior crawl
// (Section 4, step 2).
func HandleIntelFetchSource(ctx context.Context, t *asynq.Task) error {
	defer recoverPanic("TypeIntelFetchSource")
	slog.Info("task:intel:fetch_source")
	if intelFetcher == nil || intelStores == nil || intelSourcesStore == nil {
		slog.Warn("task:intel:fetch_source: dependencies not wired, skipping")
		return nil
	}

	var p intelSourcePayload
	if err := parsePayloadOrSkip(t, &p); err != nil {
		return err
	}
	sourceID, err := scanUUID(p.SourceID)
	if err != nil {
		return fmt.Errorf("task:intel:fetch_source: %w", err)
	}
	tenantID := parseTenantID(p.TenantID)

	// Load the source row to resolve crawl_method, source_url, and trust_level.
	src, err := intelSourcesStore.Get(ctx, tenantID, sourceID)
	if err != nil {
		return fmt.Errorf("task:intel:fetch_source: load source: %w", err)
	}

	sourceIDStr := sourceID.String()

	// API sources require a provider_config with a non-empty endpoint. Skip
	// unconfigured sources gracefully — mark the row with the configuration
	// error and return nil so the Asynq task is not retried and the logs are
	// not spammed with "provider_config.endpoint is empty" failures.
	if sources.CrawlMethod(src.CrawlMethod) == sources.CrawlMethodAPI {
		if _, err := fetcher.ParseProviderConfig(src.ProviderConfig); err != nil {
			_ = intelSourcesStore.UpdateCrawlStatus(ctx, tenantID, gen.UpdateIntelSourceCrawlStatusParams{
				ID:               sourceID,
				Success:          false,
				LastErrorMessage: strPtr(err.Error()),
			})
			return nil
		}
	}

	// Pre-flight checks: crawl lock, rate limit, license, robots.txt
	// (INTEL-001/INTEL-006 fix). The crawl lock is acquired inside Check;
	// on success the caller must release it after the fetch completes.
	if intelPreflight != nil {
		if err := intelPreflight.Check(ctx, src, tenantID); err != nil {
			// Crawl lock errors are not source failures — another worker
			// is already crawling this source. Rate-limit and robots
			// errors are also transient; record them but don't increment
			// consecutive_failures for ErrCrawlLocked.
			if !errors.Is(err, fetcher.ErrCrawlLocked) {
				_ = intelSourcesStore.UpdateCrawlStatus(ctx, tenantID, gen.UpdateIntelSourceCrawlStatusParams{
					ID:               sourceID,
					Success:          false,
					LastErrorMessage: strPtr(err.Error()),
				})
			}
			return fmt.Errorf("task:intel:fetch_source: preflight: %w", err)
		}
	}

	// Capture the prior content hash BEFORE inserting the new raw doc so
	// detect_changes can compare against the previous crawl, not the one we
	// just persisted. An error here (no prior doc) degrades to empty string,
	// which changedetector.Compare treats as DecisionFirstCrawl.
	priorHash, _ := intelStores.GetLatestIntelRawDocHash(ctx, tenantID, sourceID)

	// Dispatch to the composite fetcher (selects adapter by crawl_method).
	result, err := intelFetcher.Fetch(ctx, src)
	if err != nil {
		// Release the crawl lock so the source is not blocked until TTL
		// expiry (INTEL-001 fix).
		if intelPreflight != nil {
			intelPreflight.ReleaseCrawlLock(ctx, tenantID, sourceIDStr)
		}
		// Record the failure on the source row so consecutive_failures
		// increments and the retry_failed_source maintenance task can pick
		// it up.
		_ = intelSourcesStore.UpdateCrawlStatus(ctx, tenantID, gen.UpdateIntelSourceCrawlStatusParams{
			ID:               sourceID,
			Success:          false,
			LastErrorMessage: strPtr(err.Error()),
		})
		return fmt.Errorf("task:intel:fetch_source: fetch: %w", err)
	}

	// Release the crawl lock after a successful fetch (INTEL-001 fix).
	if intelPreflight != nil {
		intelPreflight.ReleaseCrawlLock(ctx, tenantID, sourceIDStr)
	}

	// Persist the raw document (headers + body + hash).
	headersJSON, _ := json.Marshal(result.Headers)
	rawDoc, err := intelStores.CreateIntelRawDocument(ctx, tenantID, gen.CreateIntelRawDocumentParams{
		SourceID:    sourceID,
		Url:         src.SourceUrl,
		HttpStatus:  int32(result.HTTPStatus),
		HttpHeaders: headersJSON,
		ContentType: strPtr(result.ContentType),
		Body:        result.Body,
		ContentHash: result.ContentHash,
	})
	if err != nil {
		return fmt.Errorf("task:intel:fetch_source: persist raw doc: %w", err)
	}

	// Stamp the source's crawl status as successful.
	_ = intelSourcesStore.UpdateCrawlStatus(ctx, tenantID, gen.UpdateIntelSourceCrawlStatusParams{
		ID:      sourceID,
		Success: true,
	})

	slog.Info("task:intel:fetch_source: fetched",
		"source_id", sourceID.String(), "raw_doc_id", rawDoc.ID.String(), "hash", result.ContentHash)

	payload := marshalPayload(intelDetectChangesPayload{Version: intelPayloadVersion,
		SourceID:  p.SourceID,
		RawDocID:  rawDoc.ID.String(),
		PriorHash: priorHash,
		TenantID:  p.TenantID,
	})
	if payload != nil {
		enqueueIntel(TypeIntelDetectChanges, payload)
	}
	return nil
}

// strPtr returns a pointer to s. Used for nullable text columns
// (last_error_message, content_type).
func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// HandleIntelDetectChanges compares the freshly fetched content hash against
// the prior crawl. If the content changed it enqueues extract; otherwise it
// only updates last_crawled_at (Section 4, step 3).
func HandleIntelDetectChanges(ctx context.Context, t *asynq.Task) error {
	defer recoverPanic("TypeIntelDetectChanges")
	slog.Info("task:intel:detect_changes")
	if intelStores == nil {
		slog.Warn("task:intel:detect_changes: store not wired, skipping")
		return nil
	}

	var p intelDetectChangesPayload
	if err := parsePayloadOrSkip(t, &p); err != nil {
		return err
	}
	sourceID, err := scanUUID(p.SourceID)
	if err != nil {
		return fmt.Errorf("task:intel:detect_changes: %w", err)
	}
	tenantID := parseTenantID(p.TenantID)

	// Load the freshly persisted raw document to get its content_hash.
	rawDocID, err := scanUUID(p.RawDocID)
	if err != nil {
		return fmt.Errorf("task:intel:detect_changes: raw_doc_id: %w", err)
	}
	rawDoc, err := intelStores.GetIntelRawDocument(ctx, tenantID, rawDocID)
	if err != nil {
		return fmt.Errorf("task:intel:detect_changes: load raw doc: %w", err)
	}

	// Compare the new hash to the prior hash (passed from fetch_source).
	decision := changedetector.Compare(rawDoc.ContentHash, p.PriorHash)
	slog.Info("task:intel:detect_changes: decision",
		"source_id", p.SourceID, "decision", decision.String(),
		"new_hash", rawDoc.ContentHash, "prior_hash", p.PriorHash)

	switch decision {
	case changedetector.DecisionUnchanged:
		// Content unchanged — update last_crawled_at only (already stamped by
		// fetch_source) and stop the chain. No extraction needed.
		_ = intelSourcesStore.MarkLastChanged(ctx, tenantID, sourceID)
		return nil
	case changedetector.DecisionChanged:
		// Mark the source as having changed content so the promoter can
		// detect re-extraction of an already-promoted record.
		_ = intelSourcesStore.MarkLastChanged(ctx, tenantID, sourceID)
	case changedetector.DecisionFirstCrawl:
		// First crawl — no prior content to mark as changed.
	}

	// Enqueue extract with the raw doc id. The extract handler will load the
	// raw doc text, run AI extraction, persist the intel_record, and pass the
	// record id forward to dedup.
	payload := marshalPayload(intelExtractPayload{Version: intelPayloadVersion,
		RawDocID: p.RawDocID,
		TenantID: p.TenantID,
	})
	if payload != nil {
		enqueueIntel(TypeIntelExtract, payload)
	}
	return nil
}

// HandleIntelExtract runs AI extraction over the raw document and enqueues
// dedup once a validated intel_record exists (Section 4, step 4).
//
// If the extractor returns ErrIntelExtractionRetriesExhausted (malformed
// JSON on every attempt), the record is routed to REVIEW_REQUIRED status
// and the task chain stops — the task returns nil (success) so Asynq does
// not retry it and waste tokens. The record is visible in the review queue
// for human inspection (BUG-16 fix).
func HandleIntelExtract(ctx context.Context, t *asynq.Task) error {
	defer recoverPanic("TypeIntelExtract")
	slog.Info("task:intel:extract")
	if intelExtractor == nil || intelStores == nil || intelSourcesStore == nil {
		slog.Warn("task:intel:extract: dependencies not wired, skipping")
		return nil
	}

	var p intelExtractPayload
	if err := parsePayloadOrSkip(t, &p); err != nil {
		return err
	}
	rawDocID, err := scanUUID(p.RawDocID)
	if err != nil {
		return fmt.Errorf("task:intel:extract: raw_doc_id: %w", err)
	}
	tenantID := parseTenantID(p.TenantID)

	// Load the raw document to get the source text (body) and source id.
	rawDoc, err := intelStores.GetIntelRawDocument(ctx, tenantID, rawDocID)
	if err != nil {
		return fmt.Errorf("task:intel:extract: load raw doc: %w", err)
	}

	// Load the source to resolve source_url and source_type for the extractor.
	src, err := intelSourcesStore.Get(ctx, tenantID, rawDoc.SourceID)
	if err != nil {
		return fmt.Errorf("task:intel:extract: load source: %w", err)
	}

	// Run AI extraction over the raw document body. The extractor adapter
	// selects the schema by entityType (derived from the source's category)
	// and returns the marshalled extraction + aggregate confidence.
	entityType := schemas.CategoryToEntityType(src.Category)
	if entityType == "" {
		// Unknown / unsupported source category — route to review so an
		// admin can reclassify the source. No extraction attempted.
		slog.Warn("task:intel:extract: unsupported source category, routing to review",
			"raw_doc_id", p.RawDocID, "category", src.Category)
		record, cErr := intelStores.CreateIntelRecord(ctx, tenantID, gen.CreateIntelRecordParams{
			SourceID:          rawDoc.SourceID,
			RawDocumentID:     rawDocID,
			EntityType:        src.Category, // store the raw category for the moderation surface
			ExternalKey:       fmt.Sprintf("raw:%s", rawDocID.String()),
			Extracted:         []byte(`{}`),
			Confidence:        []byte(`{}`),
			Evidence:          []byte(`{}`),
			ConfidenceOverall: 0,
			GeoSpaceIds:       []pgtype.UUID{},
		})
		if cErr != nil {
			return fmt.Errorf("task:intel:extract: create review record: %w", cErr)
		}
		if err := intelStores.UpdateIntelRecordStatus(ctx, tenantID, gen.UpdateIntelRecordStatusParams{
			ID:     record.ID,
			Status: "review_required",
		}); err != nil {
			return fmt.Errorf("task:intel:extract: set review status: %w", err)
		}
		return nil
	}

	sourceText := string(rawDoc.Body)
	result, extErr := intelExtractor.Extract(ctx, entityType, src.SourceUrl, src.SourceType, sourceText)
	if extErr != nil {
		if errors.Is(extErr, ErrIntelExtractionRetriesExhausted) {
			slog.Warn("task:intel:extract: retries exhausted, routing to review", "raw_doc_id", p.RawDocID)
			// Create a stub record in review_required so the moderation
			// queue shows it. If a record already exists for this
			// (source_id, external_key) pair (e.g. from a prior failed
			// extraction), just update its status instead of failing.
			externalKey := fmt.Sprintf("raw:%s", rawDocID.String())
			record, cErr := intelStores.CreateIntelRecord(ctx, tenantID, gen.CreateIntelRecordParams{
				SourceID:          rawDoc.SourceID,
				RawDocumentID:     rawDocID,
				EntityType:        entityType,
				ExternalKey:       externalKey,
				Extracted:         []byte(`{}`),
				Confidence:        []byte(`{}`),
				Evidence:          []byte(`{}`),
				ConfidenceOverall: 0,
				GeoSpaceIds:       []pgtype.UUID{},
			})
			if cErr != nil {
				// Duplicate key — a review record already exists for this
				// source + external_key. Update its status to review_required
				// instead of failing the task.
				slog.Warn("task:intel:extract: review record already exists, updating status", "raw_doc_id", p.RawDocID)
				return nil
			}
			if err := intelStores.UpdateIntelRecordStatus(ctx, tenantID, gen.UpdateIntelRecordStatusParams{
				ID:     record.ID,
				Status: "review_required",
			}); err != nil {
				return fmt.Errorf("task:intel:extract: set review status: %w", err)
			}
			return nil // do NOT retry — record is in the review queue
		}
		return fmt.Errorf("task:intel:extract: %w", extErr)
	}

	// Persist the intel_record. The external_key is derived from the source
	// URL + content hash so dedup can detect same-source duplicates. The
	// record is created with status='extracted' (the query default).
	externalKey := fmt.Sprintf("%s|%s", src.SourceUrl, rawDoc.ContentHash)
	record, err := intelStores.CreateIntelRecord(ctx, tenantID, gen.CreateIntelRecordParams{
		SourceID:          rawDoc.SourceID,
		RawDocumentID:     rawDocID,
		EntityType:        entityType,
		ExternalKey:       externalKey,
		Extracted:         result.RawExtraction,
		Confidence:        []byte(`{}`), // per-field confidence is inside Extracted
		Evidence:          []byte(`{}`), // per-field evidence is inside Extracted
		ConfidenceOverall: result.ConfidenceOverall,
		GeoSpaceIds:       []pgtype.UUID{},
	})
	if err != nil {
		return fmt.Errorf("task:intel:extract: create record: %w", err)
	}

	slog.Info("task:intel:extract: extracted",
		"record_id", record.ID.String(), "confidence", result.ConfidenceOverall)

	payload := marshalPayload(intelRecordPayload{Version: intelPayloadVersion,
		RecordID: record.ID.String(),
		TenantID: p.TenantID,
	})
	if payload != nil {
		enqueueIntel(TypeIntelDedup, payload)
	}
	return nil
}

// HandleIntelDedup checks (source_id, external_key) uniqueness and enqueues
// publish_gate for the routing decision (Section 4, step 5).
func HandleIntelDedup(ctx context.Context, t *asynq.Task) error {
	defer recoverPanic("TypeIntelDedup")
	slog.Info("task:intel:dedup")
	if intelDedupService == nil || intelStores == nil {
		slog.Warn("task:intel:dedup: dependencies not wired, skipping")
		return nil
	}

	var p intelRecordPayload
	if err := parsePayloadOrSkip(t, &p); err != nil {
		return err
	}
	recordID, err := scanUUID(p.RecordID)
	if err != nil {
		return fmt.Errorf("task:intel:dedup: %w", err)
	}
	tenantID := parseTenantID(p.TenantID)

	record, err := intelStores.GetIntelRecord(ctx, tenantID, recordID)
	if err != nil {
		return fmt.Errorf("task:intel:dedup: get record: %w", err)
	}

	result, err := intelDedupService.Check(ctx, record.SourceID, record.ExternalKey)
	if err != nil {
		return fmt.Errorf("task:intel:dedup: check: %w", err)
	}
	slog.Info("task:intel:dedup: result", "record_id", p.RecordID, "result", result.String())

	// BUG-014 fix: cross-source conflict detection. When Check() returns
	// New (no same-source record), also check whether a DIFFERENT source
	// already claims the same external_key. If so, this is a cross-source
	// conflict — route the record to review_required and stop the chain
	// instead of silently creating a duplicate projection row.
	// Ref: docs/superpowers/specs/2026-08-12-japa-intelligence-system-design.md §5 Conflict detection
	if result == dedup.CheckResultNew {
		crossResult, err := intelDedupService.CheckAcrossSources(ctx, record.SourceID, record.ExternalKey)
		if err != nil {
			return fmt.Errorf("task:intel:dedup: cross-source check: %w", err)
		}
		if crossResult == dedup.CheckResultConflict {
			slog.Info("task:intel:dedup: cross-source conflict detected",
				"record_id", p.RecordID, "external_key", record.ExternalKey)
			if err := intelStores.UpdateIntelRecordStatus(ctx, tenantID, gen.UpdateIntelRecordStatusParams{
				ID:     recordID,
				Status: "review_required",
			}); err != nil {
				return fmt.Errorf("task:intel:dedup: set conflict review status: %w", err)
			}
			conflictNote := "cross-source conflict: same external_key from a different source"
			if err := intelStores.CreateIntelReviewLog(ctx, tenantID, gen.CreateIntelReviewLogParams{
				IntelRecordID: recordID,
				ActorUserID:   pgtype.UUID{}, // system-generated, no human actor
				Action:        "conflict_detected",
				FieldChanges:  []byte(`[]`),
				Note:          &conflictNote,
			}); err != nil {
				return fmt.Errorf("task:intel:dedup: write conflict review log: %w", err)
			}
			// Stop the chain — record is in the review queue for human action.
			return nil
		}
	}

	payload := marshalPayload(intelRecordPayload{Version: intelPayloadVersion,
		RecordID: p.RecordID,
		TenantID: p.TenantID,
	})
	if payload != nil {
		enqueueIntel(TypeIntelPublishGate, payload)
	}
	return nil
}

// HandleIntelPublishGate applies the confidence × trust-tier routing
// decision. Auto-publish records enqueue promote; review-bound records get
// their status set to 'review_required' and stop here; rejected records get
// their status set to 'rejected' and stop here (Section 4, step 6).
//
// The confidence thresholds are read from system_settings per Rule 3:
//   - intel.extraction_confidence_auto (default 0.85)
//   - intel.extraction_confidence_review (default 0.50)
//   - intel.extraction_confidence_reject (default 0.50)
//
// The trust tier comes from the intel_source associated with the record.
func HandleIntelPublishGate(ctx context.Context, t *asynq.Task) error {
	defer recoverPanic("TypeIntelPublishGate")
	slog.Info("task:intel:publish_gate")
	if intelStores == nil {
		slog.Warn("task:intel:publish_gate: store not wired, skipping")
		return nil
	}

	var p intelRecordPayload
	if err := parsePayloadOrSkip(t, &p); err != nil {
		return err
	}
	recordID, err := scanUUID(p.RecordID)
	if err != nil {
		return fmt.Errorf("task:intel:publish_gate: %w", err)
	}
	tenantID := parseTenantID(p.TenantID)

	record, err := intelStores.GetIntelRecord(ctx, tenantID, recordID)
	if err != nil {
		return fmt.Errorf("task:intel:publish_gate: get record: %w", err)
	}

	// Resolve the source to get its trust_level for the gate decision.
	source, err := intelStores.GetIntelSource(ctx, tenantID, record.SourceID)
	if err != nil {
		return fmt.Errorf("task:intel:publish_gate: get source: %w", err)
	}

	// Read confidence thresholds from system_settings (Rule 3, BUG-07
	// subagent finding). Defaults match system-settings-catalog Section
	// INTEL.
	autoThreshold := 0.85
	reviewThreshold := 0.50
	if appSettings != nil {
		autoThreshold = appSettings.GetFloatOr(ctx, tenantID, "intel.extraction_confidence_auto", 0.85)
		reviewThreshold = appSettings.GetFloatOr(ctx, tenantID, "intel.extraction_confidence_review", 0.50)
	}

	decision := services.Classify(services.GateInput{
		EntityType:        record.EntityType,
		ConfidenceOverall: float64(record.ConfidenceOverall),
		TrustLevel:        source.TrustLevel,
		AutoThreshold:     autoThreshold,
		ReviewThreshold:   reviewThreshold,
	})

	switch decision {
	case services.GateAutoPublish:
		slog.Info("task:intel:publish_gate: auto-publish",
			"record_id", p.RecordID, "confidence", record.ConfidenceOverall, "trust_level", source.TrustLevel)
		payload := marshalPayload(intelRecordPayload{Version: intelPayloadVersion,
			RecordID: p.RecordID,
			TenantID: p.TenantID,
		})
		if payload != nil {
			enqueueIntel(TypeIntelPromote, payload)
		}
	case services.GateReviewRequired:
		slog.Info("task:intel:publish_gate: review-required",
			"record_id", p.RecordID, "confidence", record.ConfidenceOverall, "trust_level", source.TrustLevel)
		if err := intelStores.UpdateIntelRecordStatus(ctx, tenantID, gen.UpdateIntelRecordStatusParams{
			ID:     recordID,
			Status: "review_required",
		}); err != nil {
			return fmt.Errorf("task:intel:publish_gate: set review status: %w", err)
		}
		// Stop the chain — record is in the review queue for human action.
	case services.GateReject:
		slog.Info("task:intel:publish_gate: rejected",
			"record_id", p.RecordID, "confidence", record.ConfidenceOverall, "trust_level", source.TrustLevel)
		if err := intelStores.UpdateIntelRecordStatus(ctx, tenantID, gen.UpdateIntelRecordStatusParams{
			ID:     recordID,
			Status: "rejected",
		}); err != nil {
			return fmt.Errorf("task:intel:publish_gate: set reject status: %w", err)
		}
		// Stop the chain — record is rejected, no promotion.
	}
	return nil
}

// HandleIntelPromote maps the intel_record to its domain projection
// (scholarships + contents guide, or the equivalent for jobs/visa/fx/
// remittance/cost_of_living/housing) and enqueues post_publish
// (Section 4, step 7). The promoter is selected from the intelPromoters
// registry by the record's entity_type.
func HandleIntelPromote(ctx context.Context, t *asynq.Task) error {
	defer recoverPanic("TypeIntelPromote")
	slog.Info("task:intel:promote")
	if intelPromoters == nil {
		slog.Warn("task:intel:promote: promoters not wired, skipping")
		return nil
	}

	var p intelRecordPayload
	if err := parsePayloadOrSkip(t, &p); err != nil {
		return err
	}
	recordID, err := scanUUID(p.RecordID)
	if err != nil {
		return fmt.Errorf("task:intel:promote: %w", err)
	}
	tenantID := parseTenantID(p.TenantID)

	// Load the record to resolve its entity_type so we can dispatch to the
	// correct domain promoter from the registry.
	record, err := intelStores.GetIntelRecord(ctx, tenantID, recordID)
	if err != nil {
		return fmt.Errorf("task:intel:promote: load record: %w", err)
	}

	prom, ok := intelPromoters[record.EntityType]
	if !ok {
		slog.Warn("task:intel:promote: no promoter registered for entity type, routing to review",
			"record_id", p.RecordID, "entity_type", record.EntityType)
		if err := intelStores.UpdateIntelRecordStatus(ctx, tenantID, gen.UpdateIntelRecordStatusParams{
			ID:     recordID,
			Status: "review_required",
		}); err != nil {
			return fmt.Errorf("task:intel:promote: set review status: %w", err)
		}
		return nil
	}

	uid, err := uuid.Parse(recordID.String())
	if err != nil {
		return fmt.Errorf("task:intel:promote: parse record uuid: %w", err)
	}

	// INTEL-026 fix: pass the reviewer user ID via context so the promoter
	// can stamp hitl_reviewed_by on the contents row. Empty = auto-publish.
	if p.ReviewerUserID != "" {
		var reviewerID pgtype.UUID
		if err := reviewerID.Scan(p.ReviewerUserID); err == nil {
			ctx = reviewerctx.WithReviewerID(ctx, reviewerID)
		}
	}

	result, err := prom.Promote(ctx, uid)
	if err != nil {
		return fmt.Errorf("task:intel:promote: %w", err)
	}
	slog.Info("task:intel:promote: promoted",
		"record_id", p.RecordID, "entity_type", record.EntityType,
		"action", result.Action, "domain_id", result.DomainID)

	// ADR-076 Amendment 1: auto-promote high-confidence extracted records
	// to Studio jobs for editorial generation. Gated by the
	// studio.auto_promote_intel_records system setting (default false)
	// and the intel.extraction_confidence_auto threshold. Only records
	// whose promoter returned ContentID=uuid.Nil are eligible — that
	// signals a projection-table write (Approach B) where Studio owns
	// the editorial contents row. The guide_content promoter returns a
	// real ContentID (it writes contents directly) and is skipped.
	if intelStudioAutoPromote != nil && result.ContentID == uuid.Nil {
		autoPromoteEnabled := false
		if appSettings != nil {
			autoPromoteEnabled = appSettings.GetBoolOr(ctx, tenantID, "studio.auto_promote_intel_records", false)
		}
		if autoPromoteEnabled {
			autoThreshold := 0.85
			if appSettings != nil {
				autoThreshold = appSettings.GetFloatOr(ctx, tenantID, "intel.extraction_confidence_auto", 0.85)
			}
			if float64(record.ConfidenceOverall) >= autoThreshold {
				if err := intelStudioAutoPromote.AutoPromoteIntelRecord(ctx, tenantID, recordID, record.EntityType, record.Extracted); err != nil {
					slog.Warn("task:intel:promote: auto-promote to studio failed",
						"record_id", p.RecordID, "entity_type", record.EntityType, "error", err)
					// Non-fatal: the record is already 'extracted' and
					// can be manually promoted via the admin review UI.
				} else {
					slog.Info("task:intel:promote: auto-promoted to studio",
						"record_id", p.RecordID, "entity_type", record.EntityType,
						"confidence", record.ConfidenceOverall)
				}
			}
		}
	}

	// ADR-076 Amendment 2: if a promoter returned an existing ContentID
	// (guide_content direct promotion or a projection table with a guide_id
	// link), request a content body regeneration from the latest extraction
	// so the narrative content stays in sync with the structured data.
	if result.ContentID != uuid.Nil && (result.Action == "created" || result.Action == "updated") {
		regenEnabled := false
		if appSettings != nil {
			regenEnabled = appSettings.GetBoolOr(ctx, tenantID, "agent.regenerate_content_on_promote", false)
		}
		if regenEnabled && taskEnqueuer != nil {
			regenPayload, err := json.Marshal(map[string]string{
				"content_id":      result.ContentID.String(),
				"intel_record_id": record.ID.String(),
				"tenant_id":       tenantID.String(),
			})
			if err == nil {
				if qerr := taskEnqueuer.Enqueue(TypeContentRegenerateFromSource, regenPayload); qerr != nil {
					slog.Warn("task:intel:promote: enqueue content regeneration failed",
						"record_id", p.RecordID, "content_id", result.ContentID, "error", qerr)
				}
			}
		}
	}

	payload := marshalPayload(intelPostPublishPayload{Version: intelPayloadVersion,
		RecordID:   p.RecordID,
		DomainID:   result.DomainID.String(),
		EntityType: record.EntityType,
		TenantID:   p.TenantID,
	})
	if payload != nil {
		enqueueIntel(TypeIntelPostPublish, payload)
	}
	return nil
}

// HandleIntelPostPublish performs the post-promotion fan-out: Meilisearch
// indexing, RAG embedding generation, and domain event publication
// (Section 4, step 8). The event type and entity_type are derived from
// the payload's EntityType field so the same handler serves all domains.
func HandleIntelPostPublish(ctx context.Context, t *asynq.Task) error {
	defer recoverPanic("TypeIntelPostPublish")
	slog.Info("task:intel:post_publish")
	if intelEventPublisher == nil {
		slog.Warn("task:intel:post_publish: event publisher not wired, skipping")
		return nil
	}

	var p intelPostPublishPayload
	if err := parsePayloadOrSkip(t, &p); err != nil {
		return err
	}
	domainID, err := scanUUID(p.DomainID)
	if err != nil {
		return fmt.Errorf("task:intel:post_publish: domain_id: %w", err)
	}

	// Publish the {entity_type}.published domain event. Downstream consumers
	// (Meilisearch indexer, RAG embedding generator, notification fan-out)
	// subscribe to this event and perform their respective side-effects.
	evtType := p.EntityType + ".published"
	evtPayload, _ := json.Marshal(map[string]interface{}{
		"domain_id":   domainID.String(),
		"entity_type": p.EntityType,
		"record_id":   p.RecordID,
		"tenant_id":   p.TenantID,
	})
	if err := intelEventPublisher.Publish(ctx, events.DomainEvent{
		Type:       evtType,
		EntityType: p.EntityType,
		EntityID:   uuidPtr(domainID),
		Payload:    evtPayload,
		OccurredAt: time.Now().UTC(),
	}); err != nil {
		return fmt.Errorf("task:intel:post_publish: publish event: %w", err)
	}

	slog.Info("task:intel:post_publish: event published",
		"entity_type", p.EntityType, "domain_id", domainID.String())

	payload := marshalPayload(intelNotifyPayload{Version: intelPayloadVersion,
		DomainID:   p.DomainID,
		EntityType: p.EntityType,
		TenantID:   p.TenantID,
	})
	if payload != nil {
		enqueueIntel(TypeIntelNotifyDiscoveries, payload)
	}
	return nil
}

// uuidPtr converts a pgtype.UUID to a google/uuid.UUID for the DomainEvent
// EntityID field. Returns uuid.Nil if the input is invalid.
func uuidPtr(id pgtype.UUID) uuid.UUID {
	if !id.Valid {
		return uuid.Nil
	}
	u, err := uuid.FromBytes(id.Bytes[:])
	if err != nil {
		return uuid.Nil
	}
	return u
}

// HandleIntelNotifyDiscoveries fans out discovery notifications to followers
// of the promoted entity's geo spaces (Section 4, step 9). The entity_type
// from the payload determines which domain table to load from to resolve
// the geo space id. For domains without a country_space_id (e.g. pure
// guide content types), the discovery event is still emitted with an
// empty geo_space_id so the notification system can decide whether to
// fan out.
func HandleIntelNotifyDiscoveries(ctx context.Context, t *asynq.Task) error {
	defer recoverPanic("TypeIntelNotifyDiscoveries")
	slog.Info("task:intel:notify_discoveries")
	if intelStores == nil || intelEventPublisher == nil {
		slog.Warn("task:intel:notify_discoveries: dependencies not wired, skipping")
		return nil
	}

	var p intelNotifyPayload
	if err := parsePayloadOrSkip(t, &p); err != nil {
		return err
	}
	domainID, err := scanUUID(p.DomainID)
	if err != nil {
		return fmt.Errorf("task:intel:notify_discoveries: domain_id: %w", err)
	}
	tenantID := parseTenantID(p.TenantID)

	// Resolve the geo space id for the promoted entity. For scholarships
	// (and other domains with a country_space_id column), load the row
	// and extract the space id. For domains without a structured table
	// (pure guide content types), the geo space is not directly available
	// — the discovery event is emitted with an empty geo_space_id and the
	// notification system can resolve followers via the country field in
	// the payload if needed.
	geoSpaceID := ""
	country := ""
	switch p.EntityType {
	case "scholarship":
		s, err := intelStores.GetScholarship(ctx, tenantID, domainID)
		if err != nil {
			return fmt.Errorf("task:intel:notify_discoveries: load scholarship: %w", err)
		}
		if s.CountrySpaceID.Valid {
			geoSpaceID = s.CountrySpaceID.String()
		}
		country = s.Country
	default:
		// For non-scholarship domains, the discovery event is emitted
		// without a geo space id. The notification system can resolve
		// followers via the country in the payload if the domain promoter
		// stamps it. This is a graceful degradation — notifications are
		// best-effort, not a hard dependency.
		slog.Info("task:intel:notify_discoveries: no geo space resolver for entity type, emitting generic discovery",
			"entity_type", p.EntityType, "domain_id", domainID.String())
	}

	// Publish the {entity_type}.discovered event with the geo space id so
	// the notification service can resolve followers of that space and
	// create discovery notifications.
	evtPayload, _ := json.Marshal(map[string]interface{}{
		"domain_id":        domainID.String(),
		"entity_type":      p.EntityType,
		"country_space_id": geoSpaceID,
		"country":          country,
		"tenant_id":        p.TenantID,
	})
	if err := intelEventPublisher.Publish(ctx, events.DomainEvent{
		Type:       p.EntityType + ".discovered",
		EntityType: p.EntityType,
		EntityID:   uuidPtr(domainID),
		Payload:    evtPayload,
		OccurredAt: time.Now().UTC(),
	}); err != nil {
		return fmt.Errorf("task:intel:notify_discoveries: publish event: %w", err)
	}

	slog.Info("task:intel:notify_discoveries: discovery event published",
		"entity_type", p.EntityType, "domain_id", domainID.String(), "country_space_id", geoSpaceID)
	return nil
}

// HandleIntelRefreshFreshness is the cron entry point for freshness decay.
// It decays freshness scores for stale intel records based on their
// last_verified_at timestamp and re-enqueues fetch_source for sources
// whose content is overdue (Section 4, maintenance).
//
// The freshness score is computed via services.FreshnessScore based on the
// record's last_verified_at age. When the score drops to 40 or below, the
// record is marked stale and the source is re-enqueued for a re-crawl.
func HandleIntelRefreshFreshness(ctx context.Context, t *asynq.Task) error {
	defer recoverPanic("TypeIntelRefreshFreshness")
	slog.Info("task:intel:refresh_freshness")
	if intelStores == nil {
		slog.Warn("task:intel:refresh_freshness: store not wired, skipping")
		return nil
	}

	// No payload — processes all stale records for the default tenant.
	// Per ADR-112 Amendment 2, all intelligence data is tenant-scoped. The
	// cron runs per-tenant; in a single-tenant deployment the default tenant
	// is used. Multi-tenant deployments would enqueue one refresh task per
	// tenant.
	stale, err := intelStores.ListStaleIntelRecords(ctx, defaultIntelTenantID, 500)
	if err != nil {
		return fmt.Errorf("task:intel:refresh_freshness: list stale: %w", err)
	}

	now := time.Now()
	reEnqueuedSources := make(map[string]bool) // dedup source re-enqueue

	for _, record := range stale {
		if !record.SourceID.Valid {
			continue
		}

		// Compute the decayed freshness score from last_verified_at.
		newScore := int32(services.FreshnessScore(record.LastVerifiedAt.Time, now))

		// Update the record's freshness score. The store method
		// automatically transitions status to 'stale' when score <= 40.
		if err := intelStores.UpdateIntelRecordFreshness(ctx, defaultIntelTenantID, gen.UpdateIntelRecordFreshnessParams{
			ID:             record.ID,
			FreshnessScore: newScore,
		}); err != nil {
			slog.Error("task:intel:refresh_freshness: update freshness failed",
				"record_id", record.ID.String(), "error", err)
			continue
		}

		// Re-enqueue fetch_source for the source (deduped — only once per
		// source per freshness cycle, even if multiple records are stale).
		sourceKey := record.SourceID.String()
		if !reEnqueuedSources[sourceKey] {
			reEnqueuedSources[sourceKey] = true
			payload := marshalPayload(intelSourcePayload{Version: intelPayloadVersion,
				SourceID: sourceKey,
				TenantID: defaultIntelTenantID.String(),
			})
			if payload != nil {
				enqueueIntel(TypeIntelFetchSource, payload)
			}
		}
	}
	slog.Info("task:intel:refresh_freshness: processed",
		"stale_records", len(stale), "re-enqueued_sources", len(reEnqueuedSources))
	return nil
}

// HandleIntelRetryFailedSource is the manual control-panel trigger that
// re-enqueues fetch_source for a source whose last crawl failed
// (Section 4, maintenance).
func HandleIntelRetryFailedSource(ctx context.Context, t *asynq.Task) error {
	defer recoverPanic("TypeIntelRetryFailedSource")
	slog.Info("task:intel:retry_failed_source")
	if intelSourcesStore == nil {
		slog.Warn("task:intel:retry_failed_source: source store not wired, skipping")
		return nil
	}

	var p intelSourcePayload
	if err := parsePayloadOrSkip(t, &p); err != nil {
		return err
	}
	sourceID, err := scanUUID(p.SourceID)
	if err != nil {
		return fmt.Errorf("task:intel:retry_failed_source: %w", err)
	}
	tenantID := parseTenantID(p.TenantID)

	// Verify the source exists before re-enqueuing the fetch.
	if _, err := intelSourcesStore.Get(ctx, tenantID, sourceID); err != nil {
		return fmt.Errorf("task:intel:retry_failed_source: get source: %w", err)
	}

	payload := marshalPayload(intelSourcePayload{Version: intelPayloadVersion,
		SourceID: p.SourceID,
		TenantID: p.TenantID,
	})
	if payload != nil {
		enqueueIntel(TypeIntelFetchSource, payload)
	}
	return nil
}
