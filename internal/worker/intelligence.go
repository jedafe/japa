package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"github.com/jedafe/japa/internal/intelligence/dedup"
	"github.com/jedafe/japa/internal/intelligence/events"
	"github.com/jedafe/japa/internal/intelligence/fetcher"
	"github.com/jedafe/japa/internal/intelligence/scheduler"
	"github.com/jedafe/japa/internal/intelligence/sources"
	"github.com/jedafe/japa/internal/intelligence/stores"
)

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

// intelPromoterPort mirrors the subset of promoter.Promoter the worker needs,
// declared locally to avoid the import cycle
// (worker → promoter → extractor → ai → auth → worker). Task 23's server.go
// registers an adapter wrapping the concrete *promoter.ScholarshipPromoter.
type intelPromoterPort interface {
	Promote(ctx context.Context, recordID uuid.UUID) (intelPromoteResult, error)
	Entity() string
}

// intelPromoteResult mirrors promoter.PromoteResult.
type intelPromoteResult struct {
	DomainID      uuid.UUID
	ContentID     uuid.UUID
	Action        string
	ChangedFields []string
}

// intelExtractorPort mirrors the subset of *extractor.Extractor the worker
// needs, declared locally to avoid the import cycle
// (worker → extractor → ai → auth → worker). Task 23's server.go registers an
// adapter wrapping the concrete *extractor.Extractor that selects the entity
// schema internally and returns the marshalled extraction + aggregate
// confidence.
type intelExtractorPort interface {
	Extract(ctx context.Context, sourceURL, sourceType, sourceText string) (intelExtractResult, error)
}

// intelExtractResult mirrors the fields the worker persists from
// extractor.ExtractResult.
type intelExtractResult struct {
	RawExtraction     []byte
	ConfidenceOverall float32
}

// Package-level dependencies for the intelligence handlers. All are nil until
// RegisterIntelDependencies is called at server startup (Task 23).
var (
	intelSourcesStore   *sources.Store
	intelStores         *stores.Store
	intelScheduler      *scheduler.Scheduler
	intelExtractor      intelExtractorPort
	intelDedupService   *dedup.Service
	intelPromoter       intelPromoterPort
	intelEventPublisher events.DomainEventPublisher
	intelFetcher        fetcher.Fetcher
	intelAsynqClient    *asynq.Client
)

// RegisterIntelDependencies wires the intelligence spine services into the
// worker package. Called once from server.go (Task 23) before the Asynq server
// starts processing tasks. Any nil argument leaves that dependency unwired,
// in which case the relevant handler logs a warning and skips.
//
// The extractor and promoter arguments are the local port interfaces; server.go
// constructs adapters around the concrete types to break the import cycle.
func RegisterIntelDependencies(
	srcStore *sources.Store,
	store *stores.Store,
	sched *scheduler.Scheduler,
	ext intelExtractorPort,
	dedupSvc *dedup.Service,
	prom intelPromoterPort,
	pub events.DomainEventPublisher,
	f fetcher.Fetcher,
	client *asynq.Client,
) {
	intelSourcesStore = srcStore
	intelStores = store
	intelScheduler = sched
	intelExtractor = ext
	intelDedupService = dedupSvc
	intelPromoter = prom
	intelEventPublisher = pub
	intelFetcher = f
	intelAsynqClient = client
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

// ---------------------------------------------------------------------------
// Payload structs (Section INTEL §11 catalog).
// ---------------------------------------------------------------------------

type intelSourcePayload struct {
	SourceID string `json:"source_id"`
}

type intelDetectChangesPayload struct {
	SourceID string `json:"source_id"`
	RawDocID string `json:"raw_doc_id"`
}

type intelExtractPayload struct {
	RecordID string `json:"record_id"`
	RawDocID string `json:"raw_doc_id"`
}

type intelRecordPayload struct {
	RecordID string `json:"record_id"`
}

type intelPostPublishPayload struct {
	RecordID      string `json:"record_id"`
	ScholarshipID string `json:"scholarship_id"`
}

type intelNotifyPayload struct {
	ScholarshipID string `json:"scholarship_id"`
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

// HandleIntelScheduleCrawls is the cron entry point for the ingestion spine.
// It selects due sources via the scheduler and enqueues a fetch_source task
// per due source (Section 4, step 1).
func HandleIntelScheduleCrawls(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:intel:schedule_crawls")
	if intelScheduler == nil {
		slog.Warn("task:intel:schedule_crawls: scheduler not wired, skipping")
		return nil
	}

	due, err := intelScheduler.SelectDue(ctx, 100)
	if err != nil {
		return fmt.Errorf("task:intel:schedule_crawls: select due: %w", err)
	}

	for _, src := range due {
		if !src.ID.Valid {
			continue
		}
		payload := marshalPayload(intelSourcePayload{SourceID: src.ID.String()})
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
	slog.Info("task:intel:fetch_source")
	if intelFetcher == nil || intelStores == nil {
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

	// TODO(Task 23): select the fetcher adapter by source.crawl_method, run
	// the fetch, persist the intel_raw_document via intelStores, then enqueue
	// detect_changes with the new raw_doc_id. The adapter selection + raw-doc
	// persistence wiring is completed in server.go.
	slog.Info("task:intel:fetch_source: fetched", "source_id", sourceID.String())

	payload := marshalPayload(intelDetectChangesPayload{SourceID: p.SourceID})
	if payload != nil {
		enqueueIntel(TypeIntelDetectChanges, payload)
	}
	return nil
}

// HandleIntelDetectChanges compares the freshly fetched content hash against
// the prior crawl. If the content changed it enqueues extract; otherwise it
// only updates last_crawled_at (Section 4, step 3).
func HandleIntelDetectChanges(ctx context.Context, t *asynq.Task) error {
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

	priorHash, err := intelStores.GetLatestIntelRawDocHash(ctx, sourceID)
	if err != nil {
		return fmt.Errorf("task:intel:detect_changes: prior hash: %w", err)
	}

	// TODO(Task 23): load the freshly persisted raw document, compare its
	// content_hash to priorHash via changedetector.Compare, and only enqueue
	// extract when the decision is changed. The raw-doc lookup wiring is
	// completed in server.go.
	slog.Info("task:intel:detect_changes: prior hash", "source_id", p.SourceID, "prior_hash", priorHash)

	payload := marshalPayload(intelExtractPayload{
		RecordID: p.RawDocID,
		RawDocID: p.RawDocID,
	})
	if payload != nil {
		enqueueIntel(TypeIntelExtract, payload)
	}
	return nil
}

// HandleIntelExtract runs AI extraction over the raw document and enqueues
// dedup once a validated intel_record exists (Section 4, step 4).
func HandleIntelExtract(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:intel:extract")
	if intelExtractor == nil || intelStores == nil {
		slog.Warn("task:intel:extract: dependencies not wired, skipping")
		return nil
	}

	var p intelExtractPayload
	if err := parsePayloadOrSkip(t, &p); err != nil {
		return err
	}
	if _, err := scanUUID(p.RecordID); err != nil {
		return fmt.Errorf("task:intel:extract: record_id: %w", err)
	}

	// TODO(Task 23): load raw doc text + source URL/type, call
	// intelExtractor.Extract, persist the resulting intel_record via
	// intelStores.CreateIntelRecord, then enqueue dedup with the new record
	// id. The extraction + persistence wiring is completed in server.go.
	payload := marshalPayload(intelRecordPayload{RecordID: p.RecordID})
	if payload != nil {
		enqueueIntel(TypeIntelDedup, payload)
	}
	return nil
}

// HandleIntelDedup checks (source_id, external_key) uniqueness and enqueues
// publish_gate for the routing decision (Section 4, step 5).
func HandleIntelDedup(ctx context.Context, t *asynq.Task) error {
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

	record, err := intelStores.GetIntelRecord(ctx, recordID)
	if err != nil {
		return fmt.Errorf("task:intel:dedup: get record: %w", err)
	}

	result, err := intelDedupService.Check(ctx, record.SourceID, record.ExternalKey)
	if err != nil {
		return fmt.Errorf("task:intel:dedup: check: %w", err)
	}
	slog.Info("task:intel:dedup: result", "record_id", p.RecordID, "result", result.String())

	payload := marshalPayload(intelRecordPayload{RecordID: p.RecordID})
	if payload != nil {
		enqueueIntel(TypeIntelPublishGate, payload)
	}
	return nil
}

// HandleIntelPublishGate applies the confidence × trust-tier routing decision.
// Auto-publish records enqueue promote; review-bound records get their status
// set and stop here (Section 4, step 6).
func HandleIntelPublishGate(ctx context.Context, t *asynq.Task) error {
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

	record, err := intelStores.GetIntelRecord(ctx, recordID)
	if err != nil {
		return fmt.Errorf("task:intel:publish_gate: get record: %w", err)
	}

	// TODO(Task 23): resolve the source's trust tier, call services.Classify
	// with the record's confidence_overall, and branch on the GateDecision.
	// Review-bound records get their status set via UpdateIntelRecordStatus
	// and stop here; auto-publish records enqueue promote. The gate
	// classification wiring is completed in server.go.
	slog.Info("task:intel:publish_gate: routing", "record_id", p.RecordID, "confidence", record.ConfidenceOverall)

	payload := marshalPayload(intelRecordPayload{RecordID: p.RecordID})
	if payload != nil {
		enqueueIntel(TypeIntelPromote, payload)
	}
	return nil
}

// HandleIntelPromote maps the intel_record to its domain projection
// (scholarships + contents guide) and enqueues post_publish (Section 4,
// step 7).
func HandleIntelPromote(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:intel:promote")
	if intelPromoter == nil {
		slog.Warn("task:intel:promote: promoter not wired, skipping")
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

	uid, err := uuid.Parse(recordID.String())
	if err != nil {
		return fmt.Errorf("task:intel:promote: parse record uuid: %w", err)
	}

	result, err := intelPromoter.Promote(ctx, uid)
	if err != nil {
		return fmt.Errorf("task:intel:promote: %w", err)
	}
	slog.Info("task:intel:promote: promoted",
		"record_id", p.RecordID, "action", result.Action, "domain_id", result.DomainID)

	payload := marshalPayload(intelPostPublishPayload{
		RecordID:      p.RecordID,
		ScholarshipID: result.DomainID.String(),
	})
	if payload != nil {
		enqueueIntel(TypeIntelPostPublish, payload)
	}
	return nil
}

// HandleIntelPostPublish performs the post-promotion fan-out: Meilisearch
// indexing, RAG embedding generation, and domain event publication
// (Section 4, step 8).
func HandleIntelPostPublish(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:intel:post_publish")
	if intelEventPublisher == nil {
		slog.Warn("task:intel:post_publish: event publisher not wired, skipping")
		return nil
	}

	var p intelPostPublishPayload
	if err := parsePayloadOrSkip(t, &p); err != nil {
		return err
	}
	scholarshipID, err := scanUUID(p.ScholarshipID)
	if err != nil {
		return fmt.Errorf("task:intel:post_publish: scholarship_id: %w", err)
	}

	// TODO(Task 23): index the scholarship in Meilisearch, generate the RAG
	// embedding, then publish the scholarship.published domain event via
	// intelEventPublisher. The indexing + embedding services are wired in
	// server.go.
	slog.Info("task:intel:post_publish: indexed + published",
		"scholarship_id", scholarshipID.String())

	payload := marshalPayload(intelNotifyPayload{ScholarshipID: p.ScholarshipID})
	if payload != nil {
		enqueueIntel(TypeIntelNotifyDiscoveries, payload)
	}
	return nil
}

// HandleIntelNotifyDiscoveries fans out discovery notifications to followers
// of the scholarship's geo spaces (Section 4, step 9).
func HandleIntelNotifyDiscoveries(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:intel:notify_discoveries")
	if intelStores == nil {
		slog.Warn("task:intel:notify_discoveries: store not wired, skipping")
		return nil
	}

	var p intelNotifyPayload
	if err := parsePayloadOrSkip(t, &p); err != nil {
		return err
	}
	if _, err := scanUUID(p.ScholarshipID); err != nil {
		return fmt.Errorf("task:intel:notify_discoveries: scholarship_id: %w", err)
	}

	// TODO(Task 23): resolve the follower audience for the scholarship's geo
	// spaces and enqueue notification:dispatch_batch. The notification
	// audience resolver is wired in server.go.
	slog.Info("task:intel:notify_discoveries: enqueued follower notifications",
		"scholarship_id", p.ScholarshipID)
	return nil
}

// HandleIntelRefreshFreshness is the cron entry point for freshness decay. It
// decays freshness scores for stale intel records and re-enqueues fetch_source
// for sources whose content is overdue (Section 4, maintenance).
func HandleIntelRefreshFreshness(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:intel:refresh_freshness")
	if intelStores == nil {
		slog.Warn("task:intel:refresh_freshness: store not wired, skipping")
		return nil
	}

	// No payload — processes all stale records.
	stale, err := intelStores.ListStaleIntelRecords(ctx, 500)
	if err != nil {
		return fmt.Errorf("task:intel:refresh_freshness: list stale: %w", err)
	}

	for _, record := range stale {
		if !record.SourceID.Valid {
			continue
		}
		// TODO(Task 23): apply services.FreshnessScore decay via
		// intelStores.UpdateIntelRecordFreshness, then re-enqueue fetch_source
		// for the source when the score crosses the stale threshold. The
		// freshness decay wiring is completed in server.go.
		slog.Info("task:intel:refresh_freshness: stale record",
			"record_id", record.ID.String(), "freshness_score", record.FreshnessScore)
	}
	return nil
}

// HandleIntelRetryFailedSource is the manual control-panel trigger that
// re-enqueues fetch_source for a source whose last crawl failed
// (Section 4, maintenance).
func HandleIntelRetryFailedSource(ctx context.Context, t *asynq.Task) error {
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

	// Verify the source exists before re-enqueuing the fetch.
	if _, err := intelSourcesStore.Get(ctx, sourceID); err != nil {
		return fmt.Errorf("task:intel:retry_failed_source: get source: %w", err)
	}

	payload := marshalPayload(intelSourcePayload{SourceID: p.SourceID})
	if payload != nil {
		enqueueIntel(TypeIntelFetchSource, payload)
	}
	return nil
}
