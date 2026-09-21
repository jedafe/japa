package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/intelligence/changedetector"
	"github.com/jedafe/japa/internal/intelligence/dedup"
	"github.com/jedafe/japa/internal/intelligence/events"
	"github.com/jedafe/japa/internal/intelligence/extractor/schemas"
	"github.com/jedafe/japa/internal/intelligence/fetcher"
	"github.com/jedafe/japa/internal/intelligence/probe"
	"github.com/jedafe/japa/internal/intelligence/relevance"
	"github.com/jedafe/japa/internal/intelligence/reviewerctx"
	"github.com/jedafe/japa/internal/intelligence/scheduler"
	"github.com/jedafe/japa/internal/intelligence/services"
	"github.com/jedafe/japa/internal/intelligence/sources"
	"github.com/jedafe/japa/internal/intelligence/stores"
)

// ---------------------------------------------------------------------------
// Intelligence System (Section INTEL) — Asynq task handlers for the
// ingestion spine. The 11 handlers below implement the task chain defined in
// Section 4 of the intelligence-system design spec:
// //	schedule_crawls → fetch_source → detect_changes → extract → dedup
// → publish_gate → promote → post_publish → notify_discoveries
// // plus the two maintenance tasks refresh_freshness (cron) and
// retry_failed_source (manual control-panel trigger).
// // Dependency injection follows the existing worker pattern: package-level
// variables are set once at startup via RegisterIntelDependencies (called from
// server.go in Task 23) and each handler is nil-safe — when a dependency is
// not wired it logs a warning and returns nil so the worker still boots in
// development and tests.
// // Import-cycle note: the extractor and promoter packages cannot be imported
// here because they transitively pull internal/ai → internal/auth →
// internal/worker (auth/handlers.go enqueues email tasks via worker constants).
// To break that cycle the extractor and promoter dependencies are declared as
// local port interfaces (intelExtractorPort / intelPromoterPort) whose result
// types are mirrored locally. Task 23's server.go registers thin adapters
// that wrap the concrete *extractor.Extractor and *promoter.ScholarshipPromoter
// so the cycle is confined to the wiring layer.
// // The service-call bodies are intentionally stubbed where the concrete wiring
// (Meilisearch indexing, RAG embedding, gate classification, freshness decay)
// is completed in Task 23's server.go alongside the DI registration. The task
// chain enqueue logic is in place so the spine is end-to-end runnable once the
// dependencies are injected.
// // Ref: docs/superpowers/specs/2026-08-12-japa-intelligence-system-design.md
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
// schema by entityType and returns one or more marshalled extractions +
// per-record aggregate confidence + stable external key. entityType is
// derived by the worker via resolveSourceEntityTypes — the source's
// provider_config.entity_types override first, then the category via
// schemas.CategoryToEntityType.
type intelExtractorPort interface {
	Extract(ctx context.Context, entityType, sourceURL, sourceType, sourceText string) ([]IntelExtractResult, error)
}

// IntelExtractResult mirrors the fields the worker persists from one record
// of an extractor.ExtractResult. Exported so the server package can build port
// adapters that satisfy intelExtractorPort from outside the worker package.
type IntelExtractResult struct {
	RawExtraction     []byte
	ConfidenceOverall float32
	ExternalKey       string
}

// ErrIntelExtractionRetriesExhausted is the port-level sentinel returned by
// the extractor adapter when the AI gateway returns malformed JSON on every
// attempt up to maxRetries. The worker's HandleIntelExtract checks for this
// and routes the record to REVIEW_REQUIRED instead of failing the task
// (BUG-16 fix). The adapter wraps extractor.ErrExtractionRetriesExhausted
// with this sentinel so the worker can detect it without importing the
// extractor package.
var ErrIntelExtractionRetriesExhausted = errors.New("intel: extraction retries exhausted")

// intelCuratorPort mirrors the curator.Registry dispatch the worker needs,
// declared locally to avoid the import cycle (worker → curator → extractor
// → ai → auth → worker). server.go registers an adapter wrapping the
// concrete *curator.Registry; the adapter dispatches on the channel's
// crawl_method internally so the worker only passes the raw doc body.
type intelCuratorPort interface {
	Curate(ctx context.Context, body []byte, contentType string, ch sources.Channel) ([]IntelCuratedRecord, error)
}

// IntelCuratedRecord mirrors one curator.Record as persisted by the worker:
// the marshalled extraction, its aggregate confidence, the stable
// external_key, and the record's entity type (the curator's per-item
// override — e.g. a ld+json @type mapping — or "" for the channel default).
// Exported so the server package can build port adapters that satisfy
// intelCuratorPort from outside the worker package.
type IntelCuratedRecord struct {
	RawExtraction     []byte
	ConfidenceOverall float32
	ExternalKey       string
	EntityType        string
}

// ErrIntelNoCurator is the port-level sentinel returned by the curator
// adapter when no deterministic curator exists for the channel's crawl
// method (sitemap, git, api, …). HandleIntelCurateChannel falls back to the
// intel:extract pipeline for those channels.
var ErrIntelNoCurator = errors.New("intel: no curator for crawl method")

// IntelStudioAutoPromoter is the port through which the intelligence spine
// auto-creates Studio jobs for high-confidence extracted records (ADR-076
// Amendment 1). Declared locally to avoid the worker → studio import cycle.
// server.go registers an adapter wrapping *studio.Service.
// // Ref: japaconnect-engineering/sections/75-ai-content-studio.md.
type IntelStudioAutoPromoter interface {
	// AutoPromoteIntelRecord creates a Studio job grounded by the given
	// intel_record_id. extractedJSON is the raw intel_records.extracted
	// payload (used to derive a topic prompt). entityType maps to the
	// Studio content_type.
	AutoPromoteIntelRecord(ctx context.Context, recordID pgtype.UUID, entityType string, extractedJSON []byte) error
}

// Package-level dependencies for the intelligence handlers. All are nil until
// RegisterIntelDependencies is called at server startup (Task 23).
var (
	intelSourcesStore      *sources.Store
	intelStores            *stores.Store
	intelScheduler         *scheduler.Scheduler
	intelExtractor         intelExtractorPort
	intelCurator           intelCuratorPort
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
// // The extractor argument is the local port interface; server.go constructs an
// adapter around the concrete *extractor.Extractor to break the import cycle.
// promoters is a map keyed by entity_type → promoter port adapter; server.go
// registers one adapter per domain (scholarship, job, visa, fx, remittance,
// cost_of_living, housing). HandleIntelPromote dispatches by the record's
// entity_type.
// // prober is the verify-configured probe.Prober (safetransport client,
// larger request budget) used by HandleIntelProbeVerify — nil leaves the
// verify task a logged skip.
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
	prober *probe.Prober,
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
	intelProber = prober

	// Install the production relevance gate (Task 10). It reads the
	// intel.relevance.* settings live and uses the registered classifier
	// port + Valkey cache; both may be nil → deterministic-only gating.
	// Tests override via RegisterIntelRelevanceGate.
	intelRelevanceGate = intelEvaluateRelevance
}

// RegisterIntelStudioAutoPromoter wires the Studio auto-promote port so
// HandleIntelPromote can auto-create Studio jobs for high-confidence
// extracted records when studio.auto_promote_intel_records is true
// (ADR-076 Amendment 1). Called from server.go after RegisterIntelDependencies.
// Pass nil to disable auto-promote (the default).
func RegisterIntelStudioAutoPromoter(p IntelStudioAutoPromoter) {
	intelStudioAutoPromote = p
}

// RegisterIntelCurator wires the curator port so HandleIntelCurateChannel
// can dispatch raw doc bodies to the deterministic curators
// (intel-channels Phase 1). Called from server.go after
// RegisterIntelDependencies. Pass nil to leave the curate pipeline
// unwired — HandleIntelCurateChannel then falls back to intel:extract.
func RegisterIntelCurator(c intelCuratorPort) {
	intelCurator = c
}

// intelRelevanceGate is the per-item relevance-gate seam (Task 10). It
// supersedes Task 9's bool-returning intelCurateRelevanceGate hook: the
// verdict is returned so the caller can persist an 'off_topic'
// intel_records row (external_key preserved for re-crawl dedup) instead
// of silently dropping the item. The signature mirrors
// relevance.Evaluate — item carries the title/summary/url the scorer
// sees, mapTopics the resolved category-map topics. nil → the gate is
// disabled and every item passes; RegisterIntelDependencies installs the
// production implementation (intelEvaluateRelevance).
var intelRelevanceGate func(ctx context.Context, item relevance.ItemInput, ch sources.Channel, mapTopics []string) (relevance.Verdict, error)

// intelRelevanceClassifier is the borderline-band classifier port (§20 AI
// gateway, category="classification"). Nil → borderline items stay
// Borderline and pass through (deterministic gating only). Registered
// from server.go via RegisterIntelRelevanceClassifier — the worker cannot
// import internal/ai (cycle through auth).
var intelRelevanceClassifier relevance.Classifier

// RegisterIntelRelevanceGate installs the per-item relevance gate
// consulted by HandleIntelCurateChannel (pre-persist) and
// HandleIntelExtract (post-extraction). Pass nil to disable gating;
// RegisterIntelDependencies installs intelEvaluateRelevance by default.
func RegisterIntelRelevanceGate(fn func(ctx context.Context, item relevance.ItemInput, ch sources.Channel, mapTopics []string) (relevance.Verdict, error)) {
	intelRelevanceGate = fn
}

// RegisterIntelRelevanceClassifier wires the borderline-band classifier
// port (server.go registers the AI-gateway adapter). Pass nil to run the
// deterministic gate only.
func RegisterIntelRelevanceClassifier(c relevance.Classifier) {
	intelRelevanceClassifier = c
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
// (INTEL-020 fix). It logs the panic with the task type for greppability,
// then re-panics so asynq's processor-level recoverer converts it into a
// task failure — the retry policy applies instead of the task silently
// succeeding mid-pipeline (which could also strand the channel crawl lock
// until TTL).
func recoverPanic(taskType string) {
	if r := recover(); r != nil {
		slog.Error("intel: task handler panicked", "task", taskType, "panic", r)
		panic(r)
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
}

// intelChannelPayload is the intel:fetch_channel task payload (intel-channels
// Phase 1). source_id is advisory — the channel row's source_id is
// authoritative and is re-resolved at handling time.
type intelChannelPayload struct {
	Version   string `json:"version"`
	ChannelID string `json:"channel_id"`
	SourceID  string `json:"source_id"`
	// URLOverride + ParentDocID implement the Phase-2 manifest fan-out:
	// a changed sitemap/wayback manifest enqueues one fetch per new or
	// changed URL. The child fetch uses the static adapter on the
	// override URL and the persisted raw doc stamps parent_document_id =
	// the manifest doc — detect_changes treats stamped docs as content
	// (normal pipeline dispatch), not manifests.
	URLOverride string `json:"url_override,omitempty"`
	ParentDocID string `json:"parent_document_id,omitempty"`
}

type intelDetectChangesPayload struct {
	Version   string `json:"version"`
	SourceID  string `json:"source_id"`
	RawDocID  string `json:"raw_doc_id"`
	PriorHash string `json:"prior_hash"`
	// ChannelID scopes the crawl outcome bookkeeping to the channel that
	// produced the raw doc. Empty on legacy fetch_source enqueues — the
	// handler falls back to source-level behaviour.
	ChannelID string `json:"channel_id,omitempty"`
}

type intelExtractPayload struct {
	Version  string `json:"version"`
	RecordID string `json:"record_id"`
	RawDocID string `json:"raw_doc_id"`
	// ChannelID lets the extract handler apply the channel-first entity-type
	// precedence (channel.entity_types → provider_config → category).
	// Empty on legacy enqueues; raw_doc.channel_id is the fallback source.
	ChannelID string `json:"channel_id,omitempty"`
	// SkipItemURLs carries the canonical item URLs the curate pass already
	// claimed for this raw doc. The hybrid pipeline's extract fallback
	// re-derives the whole document, and extractor keys
	// (sourceURL|application_url|hash) never collide with curator keys
	// (feed GUID/link) — without this suppression the already-persisted
	// items double-persist as duplicate intel_records. Empty on
	// non-hybrid enqueues.
	SkipItemURLs []string `json:"skip_item_urls,omitempty"`
}

// intelCurateChannelPayload is the intel:curate_channel task payload
// (intel-channels Phase 1). channel_id selects the crawl-method curator and
// the channel-first entity-type contract; raw_doc_id carries the body.
type intelCurateChannelPayload struct {
	Version   string `json:"version"`
	SourceID  string `json:"source_id"`
	ChannelID string `json:"channel_id"`
	RawDocID  string `json:"raw_doc_id"`
}

type intelRecordPayload struct {
	Version  string `json:"version"`
	RecordID string `json:"record_id"`

	ReviewerUserID string `json:"reviewer_user_id,omitempty"` // INTEL-026: set on approve, empty on auto-publish
	ChannelID      string `json:"channel_id,omitempty"`       // propagated through dedup → gate → promote for Task 9 pipeline routing
}

type intelPostPublishPayload struct {
	Version    string `json:"version"`
	RecordID   string `json:"record_id"`
	DomainID   string `json:"domain_id"`
	EntityType string `json:"entity_type"`
}

type intelNotifyPayload struct {
	Version    string `json:"version"`
	DomainID   string `json:"domain_id"`
	EntityType string `json:"entity_type"`
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

// HandleIntelScheduleCrawls is the cron entry point for the ingestion spine.
// It selects due channels via the scheduler and enqueues a fetch_channel
// task per due channel (Section 4, step 1; intel-channels Phase 1 moved the
// scheduling granularity from source to channel — ListDueIntelChannels
// already excludes manual/inbound channels and disabled/inactive parents).
// // BUG-11/12 fix: the batch size is read from system_settings
// (intel.schedule_batch_size, default 50) instead of being hardcoded to 100.
// The intel.enabled toggle is checked before any work — when false, the
// handler returns nil immediately.
// // BUG-13 fix: a distributed leader lock (jc:intel:scheduler_leader, SET NX
// EX) prevents duplicate cron runs across multiple worker instances. Only
// the instance that acquires the lock proceeds; others skip. The lock TTL
// (5 minutes) is longer than the cron interval (15 minutes is the spec
// default, but the actual interval is set by the Asynq scheduler config).
func HandleIntelScheduleCrawls(ctx context.Context, _ *asynq.Task) error {
	defer recoverPanic("TypeIntelScheduleCrawls")
	slog.Info("task:intel:schedule_crawls")

	if intelScheduler == nil {
		slog.Warn("task:intel:schedule_crawls: scheduler not wired, skipping")
		return nil
	}

	// BUG-12 fix: check intel.enabled toggle. Settings are platform-scoped
	// per ADR-112 Amendment 2; the default platform is used for the cron.
	if appSettings != nil {
		enabled := appSettings.GetBoolOr(ctx, "intel.enabled", true)
		if !enabled {
			slog.Info("task:intel:schedule_crawls: intel.enabled=false, skipping")
			return nil
		}
	}

	// BUG-13 fix: acquire leader lock to prevent duplicate cron runs across
	// worker instances. The lock TTL (5 min) is shorter than the typical
	// cron interval (15 min) so a crashed leader releases the lock quickly.
	if rdb != nil {
		leaderKey := "jc:intel:scheduler_leader"

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
		batchSize = appSettings.GetIntOr(ctx, "intel.schedule_batch_size", 50)
	}

	due, err := intelScheduler.SelectDueChannels(ctx, int32(batchSize)) // #nosec
	if err != nil {
		return fmt.Errorf("task:intel:schedule_crawls: select due channels: %w", err)
	}

	for _, ch := range due {
		if !ch.ID.Valid {
			continue
		}

		payload := marshalPayload(intelChannelPayload{
			Version:   intelPayloadVersion,
			ChannelID: ch.ID.String(),
			SourceID:  ch.SourceID.String(),
		})
		if payload != nil {
			enqueueIntel(TypeIntelFetchChannel, payload)
		}
	}

	return nil
}

// HandleIntelFetchSource is the legacy per-source fetch entry point
// (Section 4, step 2). Intel-channels Phase 1 moved fetching to
// intel:fetch_channel; this handler remains for queued legacy tasks and the
// admin/AI "crawl now" triggers — it resolves the source's default channel
// (GetIntelSourceDefaultChannel: oldest enabled channel, i.e. the backfilled
// 'admin' row) and delegates to the shared channel fetch path.
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

	sourceID, src, proceed, err := resolveIntelFetchSource(ctx, p)
	if err != nil {
		return err
	}

	if !proceed {
		return nil // source gone or disabled — nothing to do
	}

	ch, err := intelSourcesStore.GetDefaultChannel(ctx, sourceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Info("task:intel:fetch_source: source has no enabled channel, skipping", "source_id", sourceID.String())
			return nil
		}

		return fmt.Errorf("task:intel:fetch_source: resolve default channel: %w", err)
	}

	return intelFetchChannel(ctx, "task:intel:fetch_source", src, ch, intelFetchOpts{})
}

// HandleIntelFetchChannel fetches the raw document for a single channel and
// enqueues detect_changes carrying channel_id (intel-channels Phase 1,
// Section 4 step 2). It resolves the channel row and its parent source,
// then delegates to the shared intelFetchChannel core.
func HandleIntelFetchChannel(ctx context.Context, t *asynq.Task) error {
	defer recoverPanic("TypeIntelFetchChannel")
	slog.Info("task:intel:fetch_channel")

	if intelFetcher == nil || intelStores == nil || intelSourcesStore == nil {
		slog.Warn("task:intel:fetch_channel: dependencies not wired, skipping")
		return nil
	}

	var p intelChannelPayload
	if err := parsePayloadOrSkip(t, &p); err != nil {
		return err
	}

	channelID, err := scanUUID(p.ChannelID)
	if err != nil {
		return fmt.Errorf("task:intel:fetch_channel: %w", err)
	}

	ch, err := intelSourcesStore.GetChannel(ctx, channelID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Info("task:intel:fetch_channel: channel no longer exists, skipping", "channel_id", channelID.String())
			return nil
		}

		return fmt.Errorf("task:intel:fetch_channel: load channel: %w", err)
	}

	if !ch.Enabled {
		slog.Info("task:intel:fetch_channel: channel is disabled, skipping", "channel_id", channelID.String())
		return nil
	}

	method := sources.CrawlMethod(ch.CrawlMethod)
	switch {
	case sources.InboundMethod(method):
		// Push channels ingest via the inbound endpoints — a stray enqueue
		// must skip cleanly instead of erroring into the circuit-breaker.
		slog.Info("task:intel:fetch_channel: inbound channel, skipping", "channel_id", channelID.String(), "crawl_method", ch.CrawlMethod)
		return nil
	case method == sources.CrawlMethodManual:
		slog.Info("task:intel:fetch_channel: manual channel, skipping", "channel_id", channelID.String())
		return nil
	case method == sources.CrawlMethodXHRSniff:
		// xhr_sniff channels are task-owned — a crawl request (admin
		// crawl-now) enqueues the headless sniff instead of a fetch; the
		// task discovers and materializes api channels.
		slog.Info("task:intel:fetch_channel: xhr_sniff channel, enqueuing sniff task", "channel_id", channelID.String())
		payload := marshalPayload(intelXHRSniffPayload{
			Version:   intelPayloadVersion,
			SourceID:  ch.SourceID.String(),
			ChannelID: ch.ID.String(),
		})
		if payload != nil {
			enqueueIntel(TypeIntelXHRSniff, payload)
		}
		return nil
	}

	// Load the parent source for trust_level/license context and the
	// enabled/active gate. The payload's source_id is advisory; the channel
	// row's source_id is authoritative.
	if p.SourceID != "" && p.SourceID != ch.SourceID.String() {
		slog.Warn("task:intel:fetch_channel: payload source_id disagrees with channel row, using channel's", "channel_id", channelID.String(), "payload_source_id", p.SourceID, "channel_source_id", ch.SourceID.String())
	}

	src, err := intelSourcesStore.Get(ctx, ch.SourceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Info("task:intel:fetch_channel: parent source no longer exists, skipping", "channel_id", channelID.String())
			return nil
		}

		return fmt.Errorf("task:intel:fetch_channel: load parent source: %w", err)
	}

	if !src.Enabled || src.Status != "active" {
		slog.Info("task:intel:fetch_channel: parent source not active, skipping", "channel_id", channelID.String(), "source_id", ch.SourceID.String(), "enabled", src.Enabled, "status", src.Status)
		return nil
	}

	var opts intelFetchOpts

	// Derived-fetch overrides (Phase-2 manifest fan-out): the child URL
	// came from a fetched manifest — validate it like any entry point
	// (http/https, no private literal) since manifests are
	// attacker-controlled content.
	if p.URLOverride != "" {
		u, uerr := url.Parse(p.URLOverride)
		if uerr != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || sources.IsPrivateIPLiteral(u.Hostname()) {
			slog.Warn("task:intel:fetch_channel: rejecting invalid url_override", "channel_id", channelID.String(), "url_override", p.URLOverride)
			return nil
		}
		opts.URLOverride = p.URLOverride
	}
	if p.ParentDocID != "" {
		parentID, perr := scanUUID(p.ParentDocID)
		if perr != nil {
			return fmt.Errorf("task:intel:fetch_channel: parent_document_id: %w", perr)
		}
		opts.ParentDocID = parentID
	}

	return intelFetchChannel(ctx, "task:intel:fetch_channel", src, ch, opts)
}

// intelFetchOpts carries the Phase-2 derived-fetch overrides: URLOverride
// replaces the channel's entry point for this fetch (manifest child URL —
// fetched via the static adapter) and ParentDocID stamps the raw doc's
// parent_document_id so detect_changes dispatches it to the channel's
// pipeline instead of treating it as a manifest.
type intelFetchOpts struct {
	URLOverride string
	ParentDocID pgtype.UUID
}

// intelFetchChannel is the shared fetch core for intel:fetch_channel and the
// legacy intel:fetch_source delegation path: channel config check →
// channel-scoped preflight (crawl lock jc:intel:channel_lock:{id}) →
// Composite.Fetch(src, ch) → persist raw doc with channel_id stamped →
// UpdateChannelCrawlStatus → enqueue detect_changes. logPrefix lets the two
// callers keep their own log/error namespaces.
func intelFetchChannel(ctx context.Context, logPrefix string, src gen.IntelSource, ch gen.IntelSourceChannel, opts intelFetchOpts) error {
	channelIDStr := ch.ID.String()

	// API channels require a provider_config with a non-empty endpoint —
	// resolved channel-first so a backfilled default channel inherits the
	// source's config (intelSourceConfigured's channel-level equivalent).
	// Skip unconfigured channels gracefully: mark the row with the
	// configuration error and return nil so the task is not retried.
	if !intelChannelConfigured(ctx, ch, src) {
		return nil
	}

	// Derived fetch (Phase-2 manifest fan-out): the effective channel
	// points at the child URL via the static adapter — robots/rate-limit
	// preflight and the raw doc's url field must reflect the page actually
	// fetched. The channel row itself is untouched.
	effCh := ch
	if opts.URLOverride != "" {
		effCh.EntryUrl = opts.URLOverride
		effCh.CrawlMethod = string(sources.CrawlMethodStatic)
	}

	// Pre-flight checks: channel crawl lock, rate limit, license, robots.txt.
	// The channel lock is acquired inside CheckChannel; on success the caller
	// must release it after the fetch completes. Derived fetches lock on
	// channel+child URL so fanned-out siblings don't starve each other.
	if err := intelChannelPreflightCheck(ctx, effCh, src, opts); err != nil {
		return err
	}

	// Capture the prior content hash BEFORE inserting the new raw doc so
	// detect_changes compares against this channel's previous crawl, not
	// the doc we just persisted (and not a sibling channel's doc). An error
	// here (no prior doc) degrades to empty string → DecisionFirstCrawl.
	// Derived fetches compare per-URL — a child page's previous capture,
	// not the manifest's hash.
	var priorHash string
	if opts.URLOverride != "" {
		priorHash, _ = intelStores.GetLatestIntelRawDocHashForChannelURL(ctx, ch.ID, opts.URLOverride)
	} else {
		priorHash, _ = intelStores.GetLatestIntelRawDocHashForChannel(ctx, ch.ID)
	}

	// Dispatch to the composite fetcher (selects adapter by channel crawl_method).
	result, err := intelFetcher.Fetch(ctx, src, effCh)
	if err != nil {
		if errors.Is(err, fetcher.ErrAdapterDisabled) {
			// Adapter present but not configured (render service URL
			// unset, social provider not enabled): not a source failure —
			// release the lock and skip without touching
			// consecutive_failures, like ErrCrawlLocked.
			intelReleaseChannelLock(ctx, channelIDStr, opts.URLOverride)
			slog.Info(logPrefix+": adapter disabled, skipping", "channel_id", channelIDStr, "crawl_method", ch.CrawlMethod)

			return nil
		}

		handleChannelFetchFailure(ctx, ch, opts.URLOverride, err)

		if errors.Is(err, fetcher.ErrMethodNotImplemented) {
			// Phase-3 stub method (search — e.g. an admin-created
			// channel): the failure is recorded so the channel still
			// auto-disables, but retrying cannot change the outcome until
			// the adapter lands.
			return nil
		}

		return fmt.Errorf("%s: fetch: %w", logPrefix, err)
	}

	// Release the crawl lock after a successful fetch.
	intelReleaseChannelLock(ctx, channelIDStr, opts.URLOverride)

	// Persist the raw document (headers + body + hash + channel stamp).
	// Derived docs record the fetched child URL and the manifest parent.
	headersJSON, _ := json.Marshal(result.Headers)

	rawDoc, err := intelStores.CreateIntelRawDocument(ctx, gen.CreateIntelRawDocumentParams{
		SourceID:         ch.SourceID,
		ChannelID:        ch.ID,
		Url:              effCh.EntryUrl,
		HttpStatus:       int32(result.HTTPStatus), // #nosec
		HttpHeaders:      headersJSON,
		ContentType:      strPtr(result.ContentType),
		Body:             result.Body,
		ContentHash:      result.ContentHash,
		ParentDocumentID: opts.ParentDocID,
	})
	if err != nil {
		return fmt.Errorf("%s: persist raw doc: %w", logPrefix, err)
	}

	// Stamp the channel's crawl status as successful.
	_ = intelSourcesStore.UpdateChannelCrawlStatus(ctx, gen.UpdateIntelChannelCrawlStatusParams{
		ID:      ch.ID,
		Success: true,
	})

	slog.Info(logPrefix+": fetched", "channel_id", channelIDStr, "source_id", ch.SourceID.String(), "raw_doc_id", rawDoc.ID.String(), "hash", result.ContentHash)

	payload := marshalPayload(intelDetectChangesPayload{
		Version:   intelPayloadVersion,
		SourceID:  ch.SourceID.String(),
		ChannelID: channelIDStr,
		RawDocID:  rawDoc.ID.String(),
		PriorHash: priorHash,
	})
	if payload != nil {
		enqueueIntel(TypeIntelDetectChanges, payload)
	}

	return nil
}

// resolveIntelFetchSource resolves the source row for a fetch task: parses
// the payload's source_id + , loads the source, and reports
// proceed=false for the soft-skip cases (source deleted or disabled).
func resolveIntelFetchSource(ctx context.Context, p intelSourcePayload) (sourceID pgtype.UUID, src gen.IntelSource, proceed bool, err error) {
	sourceID, err = scanUUID(p.SourceID)
	if err != nil {
		return sourceID, src, false, fmt.Errorf("task:intel:fetch_source: %w", err)
	}

	// Load the source row to resolve crawl_method, source_url, and trust_level.
	src, err = intelSourcesStore.Get(ctx, sourceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Info("task:intel:fetch_source: source no longer exists, skipping", "source_id", sourceID.String())
			return sourceID, src, false, nil
		}

		return sourceID, src, false, fmt.Errorf("task:intel:fetch_source: load source: %w", err)
	}

	if !src.Enabled {
		slog.Info("task:intel:fetch_source: source is disabled, skipping", "source_id", sourceID.String())
		return sourceID, src, false, nil
	}

	// Manual sources (§106 video channels, entity-linked rows) carry no
	// crawlable config — a stray enqueue must skip cleanly instead of
	// erroring and tripping the consecutive-failures circuit-breaker.
	if src.CrawlMethod == "manual" {
		slog.Info("task:intel:fetch_source: manual source, skipping", "source_id", sourceID.String())
		return sourceID, src, false, nil
	}

	return sourceID, src, true, nil
}

// intelChannelConfigured is the channel-level equivalent of the retired
// source-level intelSourceConfigured check: API channels need a
// provider_config with a non-empty endpoint. The effective config is
// resolved channel-first via
// fetcher.ResolveChannelProviderConfig — a channel carrying its own API
// config uses it; a backfilled default channel ('{}' config) falls back to
// the parent source's provider_config. On a config error the channel row is
// marked with the failure and the task is skipped rather than retried.
func intelChannelConfigured(ctx context.Context, ch gen.IntelSourceChannel, src gen.IntelSource) bool {
	if sources.CrawlMethod(ch.CrawlMethod) != sources.CrawlMethodAPI {
		return true
	}

	if _, err := fetcher.ParseProviderConfig(fetcher.ResolveChannelProviderConfig(ch, src)); err != nil {
		_ = intelSourcesStore.UpdateChannelCrawlStatus(ctx, gen.UpdateIntelChannelCrawlStatusParams{
			ID:               ch.ID,
			Success:          false,
			LastErrorMessage: strPtr(err.Error()),
		})

		return false
	}

	return true
}

// intelChannelPreflightCheck runs the channel-scoped preflight gate (crawl
// lock jc:intel:channel_lock:{id} — or the derived
// jc:intel:channel_lock:{id}:{url_hash} scope when opts.URLOverride is a
// manifest child — rate limit, license, robots.txt).
// Non-lock failures are recorded on the channel row; ErrCrawlLocked is a
// transient "another worker is crawling this channel" signal that does not
// increment consecutive_failures. The lock is acquired inside CheckChannel;
// on success the caller must release it via intelReleaseChannelLock.
func intelChannelPreflightCheck(ctx context.Context, ch gen.IntelSourceChannel, src gen.IntelSource, opts intelFetchOpts) error {
	if intelPreflight == nil {
		return nil
	}

	var err error
	if opts.URLOverride != "" {
		err = intelPreflight.CheckChannelDerived(ctx, src, ch, opts.URLOverride)
	} else {
		err = intelPreflight.CheckChannel(ctx, src, ch)
	}
	if err != nil {
		if !errors.Is(err, fetcher.ErrCrawlLocked) {
			_ = intelSourcesStore.UpdateChannelCrawlStatus(ctx, gen.UpdateIntelChannelCrawlStatusParams{
				ID:               ch.ID,
				Success:          false,
				LastErrorMessage: strPtr(err.Error()),
			})
		}

		return fmt.Errorf("task:intel:fetch_channel: preflight: %w", err)
	}

	return nil
}

// intelReleaseChannelLock releases the crawl lock acquired by
// intelChannelPreflightCheck — the derived (channel+URL) scope when
// urlOverride is set, else the plain channel scope.
func intelReleaseChannelLock(ctx context.Context, channelID, urlOverride string) {
	if intelPreflight == nil {
		return
	}
	if urlOverride != "" {
		intelPreflight.ReleaseChannelCrawlLockScoped(ctx, channelID, urlOverride)
		return
	}
	intelPreflight.ReleaseChannelCrawlLock(ctx, channelID)
}

// handleChannelFetchFailure is the channel-scoped counterpart of the retired
// source-level handleFetchFailure: releases the channel crawl lock, records
// the failure on the channel row so consecutive_failures increments, and
// auto-disables the channel via UpdateIntelChannelEnabled once it crosses
// the intel.consecutive_failure_disable_threshold (default 5).
func handleChannelFetchFailure(ctx context.Context, ch gen.IntelSourceChannel, urlOverride string, fetchErr error) {
	channelIDStr := ch.ID.String()

	// Release the crawl lock so the channel is not blocked until TTL expiry.
	intelReleaseChannelLock(ctx, channelIDStr, urlOverride)
	// Record the failure on the channel row.
	_ = intelSourcesStore.UpdateChannelCrawlStatus(ctx, gen.UpdateIntelChannelCrawlStatusParams{
		ID:               ch.ID,
		Success:          false,
		LastErrorMessage: strPtr(fetchErr.Error()),
	})

	// Auto-disable channels that fail repeatedly (spec §4 failure modes).
	threshold := 5
	if appSettings != nil {
		threshold = appSettings.GetIntOr(ctx, "intel.consecutive_failure_disable_threshold", 5)
	}

	if threshold <= 0 || int64(ch.ConsecutiveFailures+1) < int64(threshold) {
		return
	}

	slog.Warn("task:intel:fetch_channel: disabling channel after consecutive failures", "channel_id", channelIDStr, "consecutive_failures", ch.ConsecutiveFailures+1, "threshold", threshold)

	if disableErr := intelSourcesStore.UpdateChannelEnabled(ctx, ch.ID, false); disableErr != nil {
		slog.Warn("task:intel:fetch_channel: failed to disable channel", "channel_id", channelIDStr, "error", disableErr)
	}
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
	// channel_id is optional — absent on legacy fetch_source enqueues.
	channelID, err := scanOptionalUUID(p.ChannelID)
	if err != nil {
		return fmt.Errorf("task:intel:detect_changes: channel_id: %w", err)
	}
	// Load the freshly persisted raw document to get its content_hash.
	rawDocID, err := scanUUID(p.RawDocID)
	if err != nil {
		return fmt.Errorf("task:intel:detect_changes: raw_doc_id: %w", err)
	}

	rawDoc, err := intelStores.GetIntelRawDocument(ctx, rawDocID)
	if err != nil {
		return fmt.Errorf("task:intel:detect_changes: load raw doc: %w", err)
	}

	// Compare the new hash to the prior hash (passed from fetch_channel).
	decision := changedetector.Compare(rawDoc.ContentHash, p.PriorHash)
	slog.Info("task:intel:detect_changes: decision", "source_id", p.SourceID, "channel_id", p.ChannelID, "decision", decision.String(), "new_hash", rawDoc.ContentHash, "prior_hash", p.PriorHash)

	switch decision {
	case changedetector.DecisionUnchanged:
		// Content unchanged — the crawl status was already stamped by
		// fetch_channel; stop the chain. No extraction needed.
		intelRecordChannelCrawlOutcome(ctx, sourceID, channelID, false)
		return nil
	case changedetector.DecisionChanged:
		// Mark the crawl unit as having changed content so the promoter can
		// detect re-extraction of an already-promoted record. Channel path
		// stamps the channel row; the legacy path stamps the source.
		if channelID.Valid {
			_ = intelSourcesStore.MarkChannelChanged(ctx, channelID)
		} else {
			_ = intelSourcesStore.MarkLastChanged(ctx, sourceID)
		}
		intelRecordChannelCrawlOutcome(ctx, sourceID, channelID, true)
	// First crawl — no prior content to mark as changed.
	case changedetector.DecisionFirstCrawl:
	}

	// Pipeline routing (intel-channels Phase 1): the channel's pipeline field
	// selects the processing path — 'extract' enqueues intel:extract (LLM),
	// 'curate'/'hybrid' enqueue intel:curate_channel (deterministic
	// curators). Legacy payloads without channel_id resolve the source's
	// default channel; when no channel resolves the path is 'extract',
	// preserving the pre-channels behaviour.
	ch, chResolved := resolveIntelPipelineChannel(ctx, sourceID, channelID)

	// Manifest channels (sitemap, wayback) produce url<TAB>stamp bodies —
	// change-detection signal, not content. Dispatching the manifest
	// onward would spend an LLM pass on a URL list and could only
	// fabricate records from slugs; instead the changed/new URLs fan out
	// as derived fetches (Phase 2). Raw docs stamped with
	// parent_document_id are those derived pages — real content that
	// dispatches to the channel's pipeline normally. The crawl outcome
	// was already recorded above, so the chain ends here either way.
	if chResolved && intelManifestMethod(ch.CrawlMethod) && !rawDoc.ParentDocumentID.Valid {
		if decision == changedetector.DecisionChanged || decision == changedetector.DecisionFirstCrawl {
			intelExpandManifest(ctx, ch, rawDoc)
		}

		return nil
	}

	pipeline := sources.PipelineExtract
	if chResolved {
		if p2 := sources.Pipeline(ch.Pipeline); p2.IsValid() {
			pipeline = p2
		}
	}

	channelIDStr := p.ChannelID
	if channelIDStr == "" && chResolved {
		channelIDStr = ch.ID.String()
	}

	if pipeline == sources.PipelineCurate || pipeline == sources.PipelineHybrid {
		payload := marshalPayload(intelCurateChannelPayload{
			Version:   intelPayloadVersion,
			SourceID:  p.SourceID,
			ChannelID: channelIDStr,
			RawDocID:  p.RawDocID,
		})
		if payload != nil {
			enqueueIntel(TypeIntelCurateChannel, payload)
		}

		return nil
	}

	// Enqueue extract with the raw doc id. The extract handler will load the
	// raw doc text, run AI extraction, persist the intel_record, and pass the
	// record id forward to dedup.
	payload := marshalPayload(intelExtractPayload{
		Version:   intelPayloadVersion,
		RawDocID:  p.RawDocID,
		ChannelID: channelIDStr,
	})
	if payload != nil {
		enqueueIntel(TypeIntelExtract, payload)
	}

	return nil
}

// intelManifestMethod reports whether m produces a url<TAB>stamp manifest
// rather than page content — sitemap and wayback (Phase 2). Manifests end
// at detect_changes and fan out derived fetches; they never reach curate
// or extract.
func intelManifestMethod(method string) bool {
	m := sources.CrawlMethod(method)
	return m == sources.CrawlMethodSitemap || m == sources.CrawlMethodWayback
}

// intelExpandManifest fans out a changed manifest (Phase-2 manifest
// fan-out): diffs the new url<TAB>stamp body against the channel's previous
// manifest doc and enqueues one derived intel:fetch_channel per new/changed
// URL — bounded by provider_config.max_urls then
// intel.sitemap.max_children_per_crawl (default 25), filtered by
// provider_config.url_filter glob on the URL path. Derived fetches stamp
// parent_document_id so their docs dispatch to the channel's pipeline.
// Invalid child URLs (non-http, private literals) are skipped — manifests
// are attacker-controlled content.
func intelExpandManifest(ctx context.Context, ch gen.IntelSourceChannel, manifestDoc gen.IntelRawDocument) {
	var prevBody []byte
	if prev, err := intelStores.GetPrevIntelRawDocByChannel(ctx, ch.ID, manifestDoc.ID); err == nil {
		prevBody = prev.Body
	} else if !errors.Is(err, pgx.ErrNoRows) {
		slog.Warn("task:intel:detect_changes: prev manifest lookup failed, treating all URLs as changed", "channel_id", ch.ID.String(), "error", err)
	}

	changed := fetcher.DiffManifest(manifestDoc.Body, prevBody)
	if len(changed) == 0 {
		return
	}

	var cfg map[string]any
	_ = json.Unmarshal(ch.ProviderConfig, &cfg)

	filter := intelCfgString(cfg, "url_filter")
	maxChildren := intelCfgInt(cfg, "max_urls", 0)
	if maxChildren <= 0 && appSettings != nil {
		maxChildren = appSettings.GetIntOr(ctx, "intel.sitemap.max_children_per_crawl", 25)
	}
	if maxChildren <= 0 {
		maxChildren = 25
	}

	enqueued := 0
	for _, u := range changed {
		if enqueued >= maxChildren {
			slog.Info("task:intel:detect_changes: manifest fan-out capped", "channel_id", ch.ID.String(), "max", maxChildren, "changed_total", len(changed))
			break
		}
		parsed, err := url.Parse(u)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || sources.IsPrivateIPLiteral(parsed.Hostname()) {
			slog.Warn("task:intel:detect_changes: skipping invalid manifest URL", "channel_id", ch.ID.String(), "url", u)
			continue
		}
		if filter != "" && !intelManifestURLMatch(filter, u) {
			continue
		}

		payload := marshalPayload(intelChannelPayload{
			Version:     intelPayloadVersion,
			ChannelID:   ch.ID.String(),
			SourceID:    ch.SourceID.String(),
			URLOverride: u,
			ParentDocID: manifestDoc.ID.String(),
		})
		if payload == nil {
			continue
		}
		enqueueIntel(TypeIntelFetchChannel, payload)
		enqueued++
	}

	slog.Info("task:intel:detect_changes: manifest fan-out", "channel_id", ch.ID.String(), "raw_doc_id", manifestDoc.ID.String(), "changed", len(changed), "enqueued", enqueued)
}

// intelManifestURLMatch applies provider_config.url_filter — a single glob
// pattern (path.Match-style, '*'/'?' wildcards, no '**') matched against
// the URL's path. A malformed pattern matches nothing (fail closed — the
// admin sees no fan-out rather than an unbounded one).
func intelManifestURLMatch(pattern, rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	p := u.EscapedPath()
	if p == "" {
		p = "/"
	}
	matched, err := path.Match(pattern, p)
	return err == nil && matched
}

// intelCfgString / intelCfgInt read loosely-typed channel provider_config
// keys (worker-side counterpart of the fetcher cfg helpers — kept local to
// avoid coupling worker to fetcher internals).
func intelCfgString(cfg map[string]any, key string) string {
	if s, ok := cfg[key].(string); ok {
		return s
	}
	return ""
}

func intelCfgInt(cfg map[string]any, key string, fallback int) int {
	switch v := cfg[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return fallback
}

// resolveIntelPipelineChannel loads the channel that produced a raw doc for
// pipeline routing: the payload's channel_id first, then — for legacy
// enqueues or a deleted channel row — the source's default channel
// (GetIntelSourceDefaultChannel: oldest enabled, i.e. the backfilled
// 'admin' row). ok=false means no channel exists and the caller uses the
// default 'extract' path.
func resolveIntelPipelineChannel(ctx context.Context, sourceID, channelID pgtype.UUID) (gen.IntelSourceChannel, bool) {
	if intelSourcesStore == nil {
		return gen.IntelSourceChannel{}, false
	}

	if channelID.Valid {
		if ch, err := intelSourcesStore.GetChannel(ctx, channelID); err == nil {
			return ch, true
		}
		// Channel row gone — fall through to the default channel.
	}

	if ch, err := intelSourcesStore.GetDefaultChannel(ctx, sourceID); err == nil {
		return ch, true
	}

	return gen.IntelSourceChannel{}, false
}

// intelRecordChannelCrawlOutcome feeds the volatility auto-tuner after a
// crawl outcome is known (detect_changes). The tuning key is the channel_id
// when present (jc:intel:effective_interval::{channel_id}) and falls back to
// source_id for legacy enqueues; the base interval comes from the channel's
// crawl_frequency_mins, else the source's. Best-effort — resolution or
// Valkey errors are logged, never fail the task.
func intelRecordChannelCrawlOutcome(ctx context.Context, sourceID, channelID pgtype.UUID, changed bool) {
	if intelScheduler == nil || intelSourcesStore == nil {
		return
	}

	key := sourceID.String()
	base := int32(0)

	if channelID.Valid {
		key = channelID.String()
		if ch, err := intelSourcesStore.GetChannel(ctx, channelID); err == nil {
			base = ch.CrawlFrequencyMins
		}
	}

	if base <= 0 {
		if src, err := intelSourcesStore.Get(ctx, sourceID); err == nil {
			base = src.CrawlFrequencyMins
		}
	}

	if base <= 0 {
		return
	}

	if err := intelScheduler.RecordCrawlOutcome(ctx, key, base, changed); err != nil {
		slog.Warn("task:intel:detect_changes: record crawl outcome failed", "key", key, "changed", changed, "error", err)
	}
}

// HandleIntelExtract runs AI extraction over the raw document and enqueues
// dedup once a validated intel_record exists (Section 4, step 4).
// // If the extractor returns ErrIntelExtractionRetriesExhausted (malformed
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
	// Load the raw document to get the source text (body) and source id.
	rawDoc, err := intelStores.GetIntelRawDocument(ctx, rawDocID)
	if err != nil {
		return fmt.Errorf("task:intel:extract: load raw doc: %w", err)
	}

	// Load the source to resolve source_url and source_type for the extractor.
	src, err := intelSourcesStore.Get(ctx, rawDoc.SourceID)
	if err != nil {
		return fmt.Errorf("task:intel:extract: load source: %w", err)
	}

	// Run AI extraction over the raw document body. The extractor adapter
	// selects the schema by entityType and returns one or more marshalled
	// extractions + per-record metadata. Entity-type resolution is
	// channel-first when the raw doc was produced by a channel
	// (channel.entity_types → source.provider_config.entity_types →
	// category-derived type); resolveSourceEntityTypes covers the last two
	// legs for legacy documents with no channel stamped.
	entityTypes := resolveIntelEntityTypes(ctx, p.ChannelID, rawDoc, src)
	if len(entityTypes) == 0 {
		// Unknown / unsupported source category — route to review so an
		// admin can reclassify the source. No extraction attempted. The
		// raw category is stored on the record for the moderation surface.
		slog.Warn("task:intel:extract: unsupported source category, routing to review", "raw_doc_id", p.RawDocID, "category", src.Category)

		return intelCreateReviewStub(ctx, rawDoc.SourceID, rawDocID, src.Category)
	}

	sourceText := string(rawDoc.Body)

	// Relevance gate (Task 10, extract path): the raw body is not itemized
	// pre-LLM, so gating runs post-extraction per returned record on its
	// title/summary/article_url fields. Channel context resolves
	// payload.channel_id → raw doc stamp → source default channel; with no
	// channel the record is treated as 'filtered' on category topics only.
	// nil when the gate is not wired.
	rel := intelRelevanceCallFor(ctx, p.ChannelID, rawDoc, src)

	// Hybrid-fallback suppression (intel-channels Phase 1): when the curate
	// pass routed this doc to extract because a record was missing required
	// fields, it also persisted the doc's well-formed items under curator
	// keys (feed GUID/link). Extract re-derives the whole doc under
	// extractor keys (sourceURL|application_url|hash) that never collide —
	// so the claimed item URLs ride the payload and matching results are
	// skipped before insert. Empty on every non-hybrid enqueue.
	skipItems := intelNormalizeItemURLSet(p.SkipItemURLs)

	var persisted int
	var retriesExhausted, curateOnly bool
	for _, entityType := range entityTypes {
		// Curate-only entity types (article, event, dataset) have no LLM
		// prompt template — a dispatch here means a curate/hybrid channel
		// was misrouted to intel:extract. Skip the type instead of burning
		// an LLM call on a guaranteed failure; when every resolved type is
		// curate-only the doc routes to the review queue below.
		if schemas.IsCurateOnly(entityType) {
			slog.Warn("task:intel:extract: curate-only entity type reached extract, skipping", "raw_doc_id", p.RawDocID, "entity_type", entityType)
			curateOnly = true
			continue
		}

		results, extErr := intelExtractor.Extract(ctx, entityType, src.SourceUrl, src.SourceType, sourceText)
		if extErr != nil {
			if errors.Is(extErr, ErrIntelExtractionRetriesExhausted) {
				// Per-type tolerance: a mixed source loses only this type's
				// pass — other types' records are still persisted.
				slog.Warn("task:intel:extract: retries exhausted for entity type", "raw_doc_id", p.RawDocID, "entity_type", entityType)
				retriesExhausted = true
				continue
			}
			if errors.Is(extErr, schemas.ErrCurateOnlyEntityType) {
				// Defense-in-depth: the adapter refused a curate-only type
				// the worker-side guard missed.
				slog.Warn("task:intel:extract: adapter refused curate-only entity type", "raw_doc_id", p.RawDocID, "entity_type", entityType)
				curateOnly = true
				continue
			}

			return fmt.Errorf("task:intel:extract: %w", extErr)
		}

		n, perr := intelPersistExtractedRecords(ctx, rawDoc.SourceID, rawDocID, rawDoc.ChannelID, entityType, "", results, rel, skipItems)
		if perr != nil {
			return perr
		}
		persisted += n
	}

	if persisted == 0 {
		if retriesExhausted || curateOnly {
			slog.Warn("task:intel:extract: no extractable entity types, routing to review", "raw_doc_id", p.RawDocID, "retries_exhausted", retriesExhausted, "curate_only", curateOnly)

			return intelCreateReviewStub(ctx, rawDoc.SourceID, rawDocID, entityTypes[0])
		}

		// Terminal non-failure: the extractor ran clean but yielded zero
		// records (empty listing page, or every result deduped on the
		// 23505 unique key / hybrid skip-set). Retrying the same body
		// cannot produce records — it would only burn an LLM call per
		// asynq retry — so report success. The next changed document gets
		// its own extraction pass.
		slog.Warn("task:intel:extract: extraction produced no records", "raw_doc_id", p.RawDocID)

		return nil
	}

	return nil
}

// resolveIntelEntityTypes applies the intel-channels Phase 1 entity-type
// precedence for an extracted document: channel.entity_types →
// source.provider_config.entity_types → category-derived type. The channel
// id comes from the extract payload first, then the raw doc's stamped
// channel_id; when neither resolves to a channel row the source-level
// resolution runs unchanged (legacy documents keep their behaviour).
func resolveIntelEntityTypes(ctx context.Context, payloadChannelID string, rawDoc gen.IntelRawDocument, src gen.IntelSource) []string {
	channelID, err := scanOptionalUUID(payloadChannelID)
	if err != nil || !channelID.Valid {
		channelID = rawDoc.ChannelID
	}

	if channelID.Valid && intelSourcesStore != nil {
		if ch, gerr := intelSourcesStore.GetChannel(ctx, channelID); gerr == nil {
			return resolveChannelEntityTypes(ch, src)
		}
		// Channel gone or unreadable — fall through to source resolution.
	}

	return resolveSourceEntityTypes(src)
}

// resolveChannelEntityTypes returns the extraction entity types for a
// channel: the channel's entity_types array first (each value must map to a
// registered extraction schema — unregistered values are dropped with a
// warning), otherwise the source-level resolution (provider_config override,
// then category). Backfilled channels carry entity_types='{}' and flow
// straight to the source fallback.
func resolveChannelEntityTypes(ch gen.IntelSourceChannel, src gen.IntelSource) []string {
	if len(ch.EntityTypes) > 0 {
		out := make([]string, 0, len(ch.EntityTypes))
		seen := make(map[string]struct{}, len(ch.EntityTypes))

		for _, et := range ch.EntityTypes {
			if et == "" {
				continue
			}
			if _, ok := schemas.ForEntityType(et); !ok {
				slog.Warn("task:intel:extract: channel entity type has no registered schema, skipping", "entity_type", et)
				continue
			}
			if _, dup := seen[et]; dup {
				continue
			}
			seen[et] = struct{}{}
			out = append(out, et)
		}

		if len(out) > 0 {
			return out
		}
	}

	return resolveSourceEntityTypes(src)
}

// resolveSourceEntityTypes returns the extraction entity types for a source:
// the provider_config.entity_types override when declared (each value must
// map to a registered extraction schema — unregistered values are dropped
// with a warning), otherwise the single type derived from the source's
// category. An empty result means the source has no extractable domain and
// the caller routes the document to the review queue.
func resolveSourceEntityTypes(src gen.IntelSource) []string {
	declared := sources.EntityTypesFromProviderConfig(src.ProviderConfig)
	if len(declared) > 0 {
		out := make([]string, 0, len(declared))
		seen := make(map[string]struct{}, len(declared))

		for _, et := range declared {
			if et == "" {
				continue
			}
			if _, ok := schemas.ForEntityType(et); !ok {
				slog.Warn("task:intel:extract: provider_config entity type has no registered schema, skipping", "entity_type", et)
				continue
			}
			if _, dup := seen[et]; dup {
				continue
			}
			seen[et] = struct{}{}
			out = append(out, et)
		}

		if len(out) > 0 {
			return out
		}
	}

	if et := schemas.CategoryToEntityType(src.Category); et != "" {
		return []string{et}
	}

	return nil
}

// intelPersistExtractedRecords persists one intel_record per extraction
// result and enqueues a dedup task for each. Returns the number of records
// created. A (source_id, external_key) unique-violation is skipped rather
// than failing the task — a multi-type extraction pass can legitimately
// re-derive a record that already exists under another entity type.
// // channelID is the raw doc's stamped channel (invalid for legacy
// documents); it rides the dedup payload so Task 9 can route the record
// through the channel's pipeline.
// // rel is the resolved relevance-gate context (Task 10, spec §4 — the
// extract pipeline gates post-extraction per record). An off_topic verdict
// keeps the row (status 'off_topic', external_key preserved for dedup) but
// skips the dedup enqueue; nil disables gating.
// // skipItems is the canonical item-URL set carried by the hybrid extract
// fallback — results matching a URL the doc's curate pass already
// persisted are skipped BEFORE insert, since extractor keys never collide
// with curator keys and would otherwise double-persist the same item.
func intelPersistExtractedRecords(ctx context.Context, sourceID, rawDocID, channelID pgtype.UUID, entityType, platformStr string, results []IntelExtractResult, rel *intelRelevanceCall, skipItems map[string]struct{}) (int, error) {
	persisted := 0

	channelIDStr := ""
	if channelID.Valid {
		channelIDStr = channelID.String()
	}

	for i, result := range results {
		if len(skipItems) > 0 {
			if u := intelNormalizeItemURL(intelItemFromExtraction(result.RawExtraction).URL); u != "" {
				if _, dup := skipItems[u]; dup {
					slog.Info("task:intel:extract: item already claimed by curate pass, skipping", "source_id", sourceID.String(), "external_key", result.ExternalKey, "url", u, "entity_type", entityType)
					continue
				}
			}
		}

		record, err := intelStores.CreateIntelRecord(ctx, gen.CreateIntelRecordParams{
			SourceID:          sourceID,
			RawDocumentID:     rawDocID,
			EntityType:        entityType,
			ExternalKey:       result.ExternalKey,
			Extracted:         result.RawExtraction,
			Confidence:        []byte(`{}`),
			Evidence:          []byte(`{}`),
			ConfidenceOverall: result.ConfidenceOverall,
			GeoSpaceIds:       []pgtype.UUID{},
		})
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				slog.Info("task:intel:extract: record already exists, skipping", "source_id", sourceID.String(), "external_key", result.ExternalKey, "entity_type", entityType)
				continue
			}

			return persisted, fmt.Errorf("task:intel:extract: create record %d: %w", i, err)
		}

		persisted++

		// Relevance gate (Task 10): an off_topic verdict flips the row to
		// 'off_topic' and stops the chain — no dedup/publish enqueue.
		if rel != nil && rel.verdict(ctx, intelItemFromExtraction(result.RawExtraction), "task:intel:extract") == relevance.VerdictOffTopic {
			if uerr := intelStores.UpdateIntelRecordStatus(ctx, gen.UpdateIntelRecordStatusParams{
				ID:     record.ID,
				Status: "off_topic",
			}); uerr != nil {
				return persisted, fmt.Errorf("task:intel:extract: set off_topic status: %w", uerr)
			}

			slog.Info("task:intel:extract: record off-topic", "record_id", record.ID.String(), "index", i, "entity_type", entityType, "external_key", result.ExternalKey)
			continue
		}

		slog.Info("task:intel:extract: extracted", "record_id", record.ID.String(), "index", i, "entity_type", entityType, "confidence", result.ConfidenceOverall)

		payload := marshalPayload(intelRecordPayload{
			Version:   intelPayloadVersion,
			RecordID:  record.ID.String(),
			ChannelID: channelIDStr,
		})
		if payload != nil {
			enqueueIntel(TypeIntelDedup, payload)
		}
	}

	return persisted, nil
}

// intelCreateReviewStub creates a stub intel_record in review_required when
// extraction retries are exhausted so the moderation queue surfaces it. When
// a record already exists for the (source_id, external_key) pair — e.g. from
// a prior failed extraction — the task is a no-op rather than a failure.
func intelCreateReviewStub(ctx context.Context, sourceID, rawDocID pgtype.UUID, entityType string) error {
	record, cErr := intelStores.CreateIntelRecord(ctx, gen.CreateIntelRecordParams{
		SourceID:          sourceID,
		RawDocumentID:     rawDocID,
		EntityType:        entityType,
		ExternalKey:       "raw:" + rawDocID.String(),
		Extracted:         []byte(`{}`),
		Confidence:        []byte(`{}`),
		Evidence:          []byte(`{}`),
		ConfidenceOverall: 0,
		GeoSpaceIds:       []pgtype.UUID{},
	})
	if cErr != nil {
		var pgErr *pgconn.PgError
		if !errors.As(cErr, &pgErr) || pgErr.Code != "23505" {
			return fmt.Errorf("task:intel:extract: create review stub: %w", cErr)
		}
		// Unique-violation — a review record already exists for this source +
		// external_key. Treat as already handled instead of failing the task.
		slog.Warn("task:intel:extract: review record already exists, updating status", "raw_doc_id", rawDocID.String())

		return nil
	}

	if err := intelStores.UpdateIntelRecordStatus(ctx, gen.UpdateIntelRecordStatusParams{
		ID:     record.ID,
		Status: "review_required",
	}); err != nil {
		return fmt.Errorf("task:intel:extract: set review status: %w", err)
	}

	return nil // do NOT retry — record is in the review queue
}

// ---------------------------------------------------------------------------
// intel:curate_channel (intel-channels Phase 1)
// ---------------------------------------------------------------------------

// HandleIntelCurateChannel runs the deterministic curator for a channel's
// raw document: the curator registry dispatches on channel.crawl_method
// (rss → feed items, jsonld → ld+json nodes, ical → VEVENTs, tabular →
// rows) and each produced record is persisted as an intel_record with the
// curator's stable external_key, then routed onward — trusted sources
// (trust_level <= intel.curate.auto_publish_trust_max, default 2) go
// straight to intel:promote as 'auto_published', everything else to
// intel:dedup.
// // Crawl methods with no curator (sitemap, git, api, static, …) fall back
// to intel:extract so a curate/hybrid pipeline never dead-ends. Each
// curated record runs the Task 10 relevance gate (intelRelevanceGate) on
// its title/summary/url before persistence — off_topic records are kept
// as 'off_topic' intel_records rows for audit + re-crawl dedup; a nil
// gate passes everything.
func HandleIntelCurateChannel(ctx context.Context, t *asynq.Task) error {
	defer recoverPanic("TypeIntelCurateChannel")
	slog.Info("task:intel:curate_channel")

	if intelStores == nil || intelSourcesStore == nil {
		slog.Warn("task:intel:curate_channel: dependencies not wired, skipping")
		return nil
	}

	var p intelCurateChannelPayload
	if err := parsePayloadOrSkip(t, &p); err != nil {
		return err
	}

	rawDocID, err := scanUUID(p.RawDocID)
	if err != nil {
		return fmt.Errorf("task:intel:curate_channel: raw_doc_id: %w", err)
	}

	rawDoc, err := intelStores.GetIntelRawDocument(ctx, rawDocID)
	if err != nil {
		return fmt.Errorf("task:intel:curate_channel: load raw doc: %w", err)
	}

	// The raw doc's stamped source/channel are authoritative — payload
	// fields are advisory. channel_id falls back to the raw doc's stamp,
	// then to the source's default channel for legacy enqueues.
	channelID, err := scanOptionalUUID(p.ChannelID)
	if err != nil {
		return fmt.Errorf("task:intel:curate_channel: channel_id: %w", err)
	}
	if !channelID.Valid {
		channelID = rawDoc.ChannelID
	}

	ch, chResolved := resolveIntelPipelineChannel(ctx, rawDoc.SourceID, channelID)
	if !chResolved {
		// No channel exists to route by — treat the doc like a legacy
		// source-level fetch and send it through the extract pipeline.
		slog.Info("task:intel:curate_channel: no channel resolved, falling back to extract", "raw_doc_id", p.RawDocID)
		intelEnqueueExtractFallback(p.RawDocID, p.ChannelID, nil)

		return nil
	}

	src, err := intelSourcesStore.Get(ctx, rawDoc.SourceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Info("task:intel:curate_channel: source no longer exists, skipping", "source_id", rawDoc.SourceID.String())
			return nil
		}

		return fmt.Errorf("task:intel:curate_channel: load source: %w", err)
	}

	if intelCurator == nil {
		// Curator port not wired (dev/test) — degrade to the extract path
		// rather than dropping the document.
		slog.Warn("task:intel:curate_channel: curator not wired, falling back to extract", "channel_id", ch.ID.String())
		intelEnqueueExtractFallback(p.RawDocID, ch.ID.String(), nil)

		return nil
	}

	contentType := ""
	if rawDoc.ContentType != nil {
		contentType = *rawDoc.ContentType
	}

	records, err := intelCurator.Curate(ctx, rawDoc.Body, contentType, ch)
	if err != nil {
		if errors.Is(err, ErrIntelNoCurator) {
			// A manifest channel's body is a url<TAB>stamp list —
			// change-detection signal only (derived fetches carry the
			// content), so there is nothing for extract to parse.
			// Derived docs (parent_document_id set) still reach extract —
			// they hold real page content. Other curator-less methods
			// (api, static, git, …) carry real page content too and fall
			// through to the extract pipeline.
			if intelManifestMethod(ch.CrawlMethod) && !rawDoc.ParentDocumentID.Valid {
				slog.Info("task:intel:curate_channel: manifest body, skipping extract fallback", "channel_id", ch.ID.String())
				return nil
			}

			slog.Info("task:intel:curate_channel: no curator for method, falling back to extract", "channel_id", ch.ID.String(), "crawl_method", ch.CrawlMethod)
			intelEnqueueExtractFallback(p.RawDocID, ch.ID.String(), nil)

			return nil
		}

		// A parse failure on a deterministic document will not heal on
		// retry — park the doc in the review queue instead of burning
		// retries.
		slog.Warn("task:intel:curate_channel: curator failed, routing to review", "channel_id", ch.ID.String(), "crawl_method", ch.CrawlMethod, "error", err)

		return intelCreateReviewStub(ctx, rawDoc.SourceID, rawDocID, intelCurateDefaultEntityType(ch, src))
	}

	if len(records) == 0 {
		slog.Info("task:intel:curate_channel: curator produced no records", "channel_id", ch.ID.String())
		return nil
	}

	defaultEntityType := intelCurateDefaultEntityType(ch, src)
	hybrid := ch.Pipeline == string(sources.PipelineHybrid)

	channelIDStr := ch.ID.String()

	// Relevance gate (Task 10): resolve the category-map topic vocabulary
	// once per task when the gate is wired; each curated record is then
	// evaluated on its title/summary/url before persistence. off_topic
	// records are persisted with status 'off_topic' (external_key kept for
	// re-crawl dedup) and never reach dedup/publish_gate.
	var rel *intelRelevanceCall
	if intelRelevanceGate != nil {
		rel = &intelRelevanceCall{ch: ch, mapTopics: intelRelevanceMapTopics(ctx, ch, src)}
	}

	var persisted, gated, skipped int
	extractEnqueued := false

	// claimedURLs tracks the canonical item URL of every record this pass
	// persisted (or found already persisted — a 23505 re-crawl). The
	// hybrid extract fallback re-derives the WHOLE doc under extractor
	// keys that never collide with curator keys, so the extract persist
	// loop suppresses these URLs — otherwise each already-curated item
	// would double-persist under a second external_key.
	var claimedURLs []string

	for _, rec := range records {
		entityType := rec.EntityType
		if entityType == "" {
			entityType = defaultEntityType
		}
		if entityType == "" {
			slog.Warn("task:intel:curate_channel: record has no resolvable entity type, skipping", "channel_id", channelIDStr, "external_key", rec.ExternalKey)
			skipped++
			continue
		}

		item := intelItemFromExtraction(rec.RawExtraction)
		claimItem := func() {
			if u := intelNormalizeItemURL(item.URL); u != "" {
				claimedURLs = append(claimedURLs, u)
			}
		}

		// Relevance gate — an off_topic verdict persists the row as
		// 'off_topic' (auditable + dedupes on re-crawl) and skips the
		// downstream chain entirely.
		if rel != nil && rel.verdict(ctx, item, "task:intel:curate_channel") == relevance.VerdictOffTopic {
			gated++
			if intelPersistOffTopicRecord(ctx, rawDoc, ch, rec, entityType, "task:intel:curate_channel") != intelPersistFailed {
				claimItem()
			}
			continue
		}

		// Hybrid pipeline: a curated record missing required schema fields
		// can't stand on the deterministic fields alone. Extractable
		// entity types route the whole doc to intel:extract (once — the
		// task is per-document); curate-only types can never be
		// LLM-extracted, so they park in review_required.
		if hybrid && intelMissingRequiredFields(rec.RawExtraction, entityType) {
			if schemas.IsCurateOnly(entityType) {
				recordID, outcome := intelPersistCuratedRecord(ctx, rawDoc, ch, rec, entityType)
				if outcome != intelPersistInserted {
					// An already-existing row still claims the item for
					// the extract fallback's skip set.
					if outcome == intelPersistExists {
						claimItem()
					}
					skipped++
					continue
				}
				if uerr := intelStores.UpdateIntelRecordStatus(ctx, gen.UpdateIntelRecordStatusParams{
					ID:     recordID,
					Status: "review_required",
				}); uerr != nil {
					return fmt.Errorf("task:intel:curate_channel: set review status: %w", uerr)
				}
				persisted++
				claimItem()
				continue
			}

			if !extractEnqueued {
				slog.Info("task:intel:curate_channel: hybrid record missing required fields, routing doc to extract", "raw_doc_id", p.RawDocID, "entity_type", entityType, "external_key", rec.ExternalKey)
				extractEnqueued = true
			}
			continue
		}

		recordID, outcome := intelPersistCuratedRecord(ctx, rawDoc, ch, rec, entityType)
		if outcome != intelPersistInserted {
			if outcome == intelPersistExists {
				claimItem()
			}
			skipped++
			continue
		}
		persisted++
		claimItem()

		// Trust gate (Task 11): sources at or below
		// intel.curate.auto_publish_trust_max (default 2) skip
		// dedup+publish_gate and promote directly as 'auto_published' —
		// UNLESS the record's entity type is high-stakes: ADR-088 makes the
		// immigration/financial/grants/health review gate unconditional, so
		// those records always take the dedup → publish_gate path where
		// services.Classify routes them to review regardless of trust.
		// The promote handler + promoters select records by id/entity_type
		// with no status filter, so 'auto_published' rows are processed
		// exactly like gate-approved ones — and they never enter the
		// review queue, which lists only 'review_required'.
		if intelCurateShouldAutoPublish(ctx, src, entityType) {
			if uerr := intelStores.UpdateIntelRecordStatus(ctx, gen.UpdateIntelRecordStatusParams{
				ID:     recordID,
				Status: "auto_published",
			}); uerr != nil {
				return fmt.Errorf("task:intel:curate_channel: set auto_published status: %w", uerr)
			}

			payload := marshalPayload(intelRecordPayload{
				Version:   intelPayloadVersion,
				RecordID:  recordID.String(),
				ChannelID: channelIDStr,
			})
			if payload != nil {
				enqueueIntel(TypeIntelPromote, payload)
			}
			continue
		}

		payload := marshalPayload(intelRecordPayload{
			Version:   intelPayloadVersion,
			RecordID:  recordID.String(),
			ChannelID: channelIDStr,
		})
		if payload != nil {
			enqueueIntel(TypeIntelDedup, payload)
		}
	}

	// Deferred hybrid enqueue — the payload must carry the COMPLETE
	// claimed-item URL set, so it fires only after every record has been
	// evaluated.
	if extractEnqueued {
		intelEnqueueExtractFallback(p.RawDocID, channelIDStr, claimedURLs)
	}

	slog.Info("task:intel:curate_channel: done", "channel_id", channelIDStr, "raw_doc_id", p.RawDocID, "records", len(records), "persisted", persisted, "gated", gated, "skipped", skipped, "extract_fallback", extractEnqueued)

	return nil
}

// intelEnqueueExtractFallback enqueues an intel:extract task for the raw
// doc — the shared fallback when a curate-routed document has no usable
// curator path. skipItemURLs lists the canonical item URLs the curate pass
// already persisted for the doc (hybrid missing-fields routing only —
// nil everywhere else); the extract persist loop suppresses those items
// so they do not double-persist under extractor-derived external keys.
func intelEnqueueExtractFallback(rawDocID, channelID string, skipItemURLs []string) {
	payload := marshalPayload(intelExtractPayload{
		Version:      intelPayloadVersion,
		RawDocID:     rawDocID,
		ChannelID:    channelID,
		SkipItemURLs: skipItemURLs,
	})
	if payload != nil {
		enqueueIntel(TypeIntelExtract, payload)
	}
}

// intelCurateDefaultEntityType resolves the record entity type when a
// curator record carries no per-item override: the channel's first declared
// entity_type, then the full channel-first resolution (provider_config →
// category). "" means nothing is resolvable and the record is skipped.
func intelCurateDefaultEntityType(ch gen.IntelSourceChannel, src gen.IntelSource) string {
	for _, et := range ch.EntityTypes {
		if et != "" {
			return et
		}
	}

	if types := resolveChannelEntityTypes(ch, src); len(types) > 0 {
		return types[0]
	}

	return ""
}

// intelMissingRequiredFields reports whether the marshalled extraction is
// missing any field the entity schema declares Required — the hybrid
// pipeline's "route to extract" trigger. An unregistered entity type or a
// malformed payload cannot be checked and reports false (the record stands
// as curated).
func intelMissingRequiredFields(rawExtraction []byte, entityType string) bool {
	schema, ok := schemas.ForEntityType(entityType)
	if !ok {
		return false
	}

	var fields map[string]struct {
		Value any `json:"value"`
	}
	if err := json.Unmarshal(rawExtraction, &fields); err != nil {
		return false
	}

	for _, f := range schema.Fields {
		if !f.Required {
			continue
		}
		fe, ok := fields[f.Name]
		if !ok || fe.Value == nil {
			return true
		}
		if s, ok := fe.Value.(string); ok && strings.TrimSpace(s) == "" {
			return true
		}
	}

	return false
}

// intelPersistOutcome reports how intelPersistCuratedRecord resolved an
// insert. The distinction matters for the hybrid extract fallback's
// claimed-item suppression: inserted and exists both mean a row covers
// the item (claim it), while failed means nothing was persisted (the
// extract pass may still recover it).
type intelPersistOutcome int

const (
	intelPersistFailed   intelPersistOutcome = iota // insert error — no row exists
	intelPersistInserted                            // new row created
	intelPersistExists                              // 23505 — a prior crawl's row already claims the key
)

// intelPersistCuratedRecord inserts one curated record as an intel_record
// in 'extracted' status and returns its id plus the outcome. A
// (source_id, external_key) unique-violation is a re-crawl of unchanged
// content — reported as intelPersistExists, not an error. The evidence
// column notes the curator origin so reviewers can tell deterministic
// records from LLM extractions.
func intelPersistCuratedRecord(ctx context.Context, rawDoc gen.IntelRawDocument, ch gen.IntelSourceChannel, rec IntelCuratedRecord, entityType string) (pgtype.UUID, intelPersistOutcome) {
	confidence := rec.ConfidenceOverall
	if confidence <= 0 {
		confidence = 1.0 // curated fields are deterministic — full confidence
	}

	evidence, _ := json.Marshal(map[string]string{
		"origin":       "curator",
		"crawl_method": ch.CrawlMethod,
		"channel_id":   ch.ID.String(),
	})

	record, err := intelStores.CreateIntelRecord(ctx, gen.CreateIntelRecordParams{
		SourceID:          rawDoc.SourceID,
		RawDocumentID:     rawDoc.ID,
		EntityType:        entityType,
		ExternalKey:       rec.ExternalKey,
		Extracted:         rec.RawExtraction,
		Confidence:        []byte(`{}`),
		Evidence:          evidence,
		ConfidenceOverall: confidence,
		GeoSpaceIds:       []pgtype.UUID{},
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			slog.Info("task:intel:curate_channel: record already exists, skipping", "source_id", rawDoc.SourceID.String(), "external_key", rec.ExternalKey, "entity_type", entityType)
			return pgtype.UUID{}, intelPersistExists
		}

		slog.Error("task:intel:curate_channel: create record failed", "external_key", rec.ExternalKey, "entity_type", entityType, "error", err)

		return pgtype.UUID{}, intelPersistFailed
	}

	slog.Info("task:intel:curate_channel: curated", "record_id", record.ID.String(), "entity_type", entityType, "external_key", rec.ExternalKey, "confidence", confidence)

	return record.ID, intelPersistInserted
}

// intelCurateAutoPublish reports whether the source's trust level clears
// the curated auto-publish gate: trust_level <=
// intel.curate.auto_publish_trust_max. The spec default is 2 — trusted
// sources (trust 1–2) promote directly while aggregators take the
// dedup → publish_gate review path. An admin can raise the ceiling or
// set the setting to 0 to disable auto-publish entirely (Rule 3). A nil
// settings reader still applies the spec default, matching the relevance
// gate's nil→defaults convention.
func intelCurateAutoPublish(ctx context.Context, src gen.IntelSource) bool {
	maxTrust := 2
	if appSettings != nil {
		maxTrust = appSettings.GetIntOr(ctx, "intel.curate.auto_publish_trust_max", 2)
	}

	return maxTrust > 0 && int64(src.TrustLevel) <= int64(maxTrust)
}

// intelCurateShouldAutoPublish is the full auto-publish decision for a
// curated record: trust within the ceiling AND the entity type NOT in the
// ADR-088 high-stakes set (services.IsHighStakes — the same unconditional
// set services.Classify enforces). High-stakes curated records route to
// dedup → publish_gate regardless of source trust so they always land in
// the review queue; the gate cannot be bypassed by raising the trust max.
func intelCurateShouldAutoPublish(ctx context.Context, src gen.IntelSource, entityType string) bool {
	return intelCurateAutoPublish(ctx, src) && !services.IsHighStakes(entityType)
}

// ---------------------------------------------------------------------------
// Relevance gate (Task 10, spec §4)
// ---------------------------------------------------------------------------

// intelRelevanceCall bundles the resolved channel and category-map topics
// so the per-item gate calls inside one task share a single resolution.
// Built once per handler invocation by intelRelevanceCallFor (extract
// path) or inline in HandleIntelCurateChannel.
type intelRelevanceCall struct {
	ch        sources.Channel
	mapTopics []string
}

// verdict evaluates one item through the registered gate. Gate errors
// fail open — a broken gate must never silently drop records, so the item
// passes with a warning.
func (rc *intelRelevanceCall) verdict(ctx context.Context, item relevance.ItemInput, logPrefix string) relevance.Verdict {
	v, err := intelRelevanceGate(ctx, item, rc.ch, rc.mapTopics)
	if err != nil {
		slog.Warn(logPrefix+": relevance gate error, passing item", "channel_id", rc.ch.ID.String(), "error", err)

		return relevance.VerdictOnTopic
	}

	return v
}

// intelRelevanceCallFor resolves the gate context for the extract path:
// the payload's channel_id first, then the raw doc's stamped channel, then
// the source's default channel (resolveIntelPipelineChannel already covers
// all three legs). When no channel row exists at all the spec treats the
// evaluation as 'filtered' with category topics only — the synthetic
// channel carries the source id so the classifier rate-cap key stays
// per-source. Returns nil when the gate is not wired (dev/test).
func intelRelevanceCallFor(ctx context.Context, payloadChannelID string, rawDoc gen.IntelRawDocument, src gen.IntelSource) *intelRelevanceCall {
	if intelRelevanceGate == nil {
		return nil
	}

	channelID, err := scanOptionalUUID(payloadChannelID)
	if err != nil || !channelID.Valid {
		channelID = rawDoc.ChannelID
	}

	ch, resolved := resolveIntelPipelineChannel(ctx, rawDoc.SourceID, channelID)
	if !resolved {
		ch = sources.Channel{RelevanceMode: string(sources.RelevanceFiltered)}
		ch.ID = src.ID // crawl-scope fallback for the rate-cap key
	}

	return &intelRelevanceCall{ch: ch, mapTopics: intelRelevanceMapTopics(ctx, ch, src)}
}

// intelEvaluateRelevance is the production gate installed by
// RegisterIntelDependencies: a relevance.Gate built per call so the
// hot-reloadable intel.relevance.* settings apply to every item, backed
// by the registered classifier port and the shared Valkey client.
func intelEvaluateRelevance(ctx context.Context, item relevance.ItemInput, ch sources.Channel, mapTopics []string) (relevance.Verdict, error) {
	g := relevance.Gate{
		OnTopicThreshold:  relevance.DefaultOnTopicThreshold,
		OffTopicThreshold: relevance.DefaultOffTopicThreshold,
		ClassifierEnabled: true,
		BorderlineRateCap: relevance.DefaultBorderlineRateCap,
		Classifier:        intelRelevanceClassifier,
		Cache:             intelRelevanceCache(),
	}

	if appSettings != nil {
		g.OnTopicThreshold = appSettings.GetFloatOr(ctx, "intel.relevance.on_topic_threshold", g.OnTopicThreshold)
		g.OffTopicThreshold = appSettings.GetFloatOr(ctx, "intel.relevance.off_topic_threshold", g.OffTopicThreshold)
		g.ClassifierEnabled = appSettings.GetBoolOr(ctx, "intel.relevance.classifier_enabled", true)
		g.BorderlineRateCap = appSettings.GetIntOr(ctx, "intel.relevance.borderline_rate_cap", g.BorderlineRateCap)
	}

	return g.Evaluate(ctx, item, ch, mapTopics)
}

// intelRelevanceCache adapts the worker's shared Valkey client to the
// relevance.Cache port; nil when Valkey is not wired (the gate then runs
// deterministic-only — the classifier cap cannot be enforced without a
// counter store).
func intelRelevanceCache() relevance.Cache {
	if rdb == nil {
		return nil
	}

	return relevance.ValkeyCache{Rdb: rdb}
}

// intelRelevanceMapTopics resolves the category-map topic vocabulary for a
// channel's parent source: the source's categories (multi-category array
// first, legacy scalar fallback) are resolved against
// system_settings.intel.category_method_map (empty/malformed → embedded
// default via sources.ResolveCategoryMap). Custom-mode channels ignore
// category topics entirely — nil is returned without a settings read.
func intelRelevanceMapTopics(ctx context.Context, ch sources.Channel, src gen.IntelSource) []string {
	if sources.RelevanceMode(ch.RelevanceMode) == sources.RelevanceCustom {
		return nil
	}

	cats := src.Categories
	if len(cats) == 0 && src.Category != "" {
		cats = []string{src.Category}
	}
	if len(cats) == 0 {
		return nil
	}

	var mapJSON []byte
	if appSettings != nil {
		if s := appSettings.GetStringOr(ctx, "intel.category_method_map", ""); s != "" {
			mapJSON = []byte(s)
		}
	}

	return sources.ResolveCategoryMap(mapJSON, cats).Topics
}

// intelItemFromExtraction derives the gate's ItemInput from a marshalled
// extraction payload (field → {value, confidence, evidence}). Field names
// fall back across the schema vocabulary: title|name|headline,
// summary|description, article_url|url|link|application_url|source_url.
func intelItemFromExtraction(raw []byte) relevance.ItemInput {
	var fields map[string]struct {
		Value any `json:"value"`
	}
	if err := json.Unmarshal(raw, &fields); err != nil {
		return relevance.ItemInput{}
	}

	pick := func(names ...string) string {
		for _, n := range names {
			f, ok := fields[n]
			if !ok {
				continue
			}
			if s, ok := f.Value.(string); ok && strings.TrimSpace(s) != "" {
				return s
			}
		}

		return ""
	}

	return relevance.ItemInput{
		Title:   pick("title", "name", "headline"),
		Summary: pick("summary", "description"),
		URL:     pick("article_url", "url", "link", "application_url", "source_url"),
	}
}

// intelNormalizeItemURL canonicalizes an item URL for cross-path identity
// matching — the hybrid extract fallback suppresses items the curate pass
// already persisted, and curator keys (feed GUID/link) never collide with
// extractor keys (sourceURL|application_url), so the canonical URL is the
// shared identity. Conservative: lowercase scheme/host, fragment dropped,
// one trailing slash trimmed; query params are kept since they can carry
// identity (…?id=123). "" on empty/unparseable input — never matches.
func intelNormalizeItemURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return ""
	}

	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	u.Fragment = ""
	u.Path = strings.TrimSuffix(u.Path, "/")

	return u.String()
}

// intelNormalizeItemURLSet builds the canonical-URL lookup the extract
// persist loop uses to suppress items the curate pass already claimed.
// Empty/nil input yields nil — the skip check is then fully disabled.
func intelNormalizeItemURLSet(raw []string) map[string]struct{} {
	if len(raw) == 0 {
		return nil
	}

	out := make(map[string]struct{}, len(raw))
	for _, u := range raw {
		if n := intelNormalizeItemURL(u); n != "" {
			out[n] = struct{}{}
		}
	}

	return out
}

// intelPersistOffTopicRecord persists a gated-out record as an
// intel_records row with status 'off_topic' so the item is auditable and
// re-crawls dedupe on (source_id, external_key). A unique-violation or
// insert failure is already logged by intelPersistCuratedRecord — the
// outcome is returned so the caller can claim the item's URL for the
// hybrid extract fallback's suppression set whenever a row covers it.
func intelPersistOffTopicRecord(ctx context.Context, rawDoc gen.IntelRawDocument, ch sources.Channel, rec IntelCuratedRecord, entityType, logPrefix string) intelPersistOutcome {
	recordID, outcome := intelPersistCuratedRecord(ctx, rawDoc, ch, rec, entityType)
	if outcome != intelPersistInserted {
		return outcome // 23505 re-crawl dedup or logged insert failure
	}

	if err := intelStores.UpdateIntelRecordStatus(ctx, gen.UpdateIntelRecordStatusParams{
		ID:     recordID,
		Status: "off_topic",
	}); err != nil {
		slog.Error(logPrefix+": set off_topic status failed", "record_id", recordID.String(), "external_key", rec.ExternalKey, "error", err)
	}

	return intelPersistInserted
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
	record, err := intelStores.GetIntelRecord(ctx, recordID)
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
		handled, err := intelHandleCrossSourceConflict(ctx, recordID, record)
		if handled || err != nil {
			return err
		}
	}

	payload := marshalPayload(intelRecordPayload{
		Version:   intelPayloadVersion,
		RecordID:  p.RecordID,
		ChannelID: p.ChannelID,
	})
	if payload != nil {
		enqueueIntel(TypeIntelPublishGate, payload)
	}

	return nil
}

// intelHandleCrossSourceConflict checks whether a different source already
// claims the record's external_key. On conflict it routes the record to
// review_required, writes a conflict review log, and returns handled=true so
// the caller stops the chain.
func intelHandleCrossSourceConflict(ctx context.Context, recordID pgtype.UUID, record gen.IntelRecord) (bool, error) {
	crossResult, err := intelDedupService.CheckAcrossSources(ctx, record.SourceID, record.ExternalKey)
	if err != nil {
		return false, fmt.Errorf("task:intel:dedup: cross-source check: %w", err)
	}

	if crossResult != dedup.CheckResultConflict {
		return false, nil
	}

	slog.Info("task:intel:dedup: cross-source conflict detected", "record_id", recordID.String(), "external_key", record.ExternalKey)

	if err := intelStores.UpdateIntelRecordStatus(ctx, gen.UpdateIntelRecordStatusParams{
		ID:     recordID,
		Status: "review_required",
	}); err != nil {
		return false, fmt.Errorf("task:intel:dedup: set conflict review status: %w", err)
	}

	conflictNote := "cross-source conflict: same external_key from a different source"
	if err := intelStores.CreateIntelReviewLog(ctx, gen.CreateIntelReviewLogParams{
		IntelRecordID: recordID,
		ActorUserID:   pgtype.UUID{}, // system-generated, no human actor
		Action:        "conflict_detected",
		FieldChanges:  []byte(`[]`),
		Note:          &conflictNote,
	}); err != nil {
		return false, fmt.Errorf("task:intel:dedup: write conflict review log: %w", err)
	}

	return true, nil
}

// HandleIntelPublishGate applies the confidence × trust-tier routing
// decision. Auto-publish records enqueue promote; review-bound records get
// their status set to 'review_required' and stop here; rejected records get
// their status set to 'rejected' and stop here (Section 4, step 6).
// // The confidence thresholds are read from system_settings per Rule 3:
// - intel.extraction_confidence_auto (default 0.85)
// - intel.extraction_confidence_review (default 0.50)
// - intel.extraction_confidence_reject (default 0.50)
// // The trust tier comes from the intel_source associated with the record.
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
	record, err := intelStores.GetIntelRecord(ctx, recordID)
	if err != nil {
		return fmt.Errorf("task:intel:publish_gate: get record: %w", err)
	}

	// Resolve the source to get its trust_level for the gate decision.
	source, err := intelStores.GetIntelSource(ctx, record.SourceID)
	if err != nil {
		return fmt.Errorf("task:intel:publish_gate: get source: %w", err)
	}

	// Read confidence thresholds from system_settings (Rule 3, BUG-07
	// subagent finding). Defaults match system-settings-catalog Section
	// INTEL.
	autoThreshold := 0.85
	reviewThreshold := 0.50

	if appSettings != nil {
		autoThreshold = appSettings.GetFloatOr(ctx, "intel.extraction_confidence_auto", 0.85)
		reviewThreshold = appSettings.GetFloatOr(ctx, "intel.extraction_confidence_review", 0.50)
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
		slog.Info("task:intel:publish_gate: auto-publish", "record_id", p.RecordID, "confidence", record.ConfidenceOverall, "trust_level", source.TrustLevel)

		payload := marshalPayload(intelRecordPayload{
			Version:   intelPayloadVersion,
			RecordID:  p.RecordID,
			ChannelID: p.ChannelID,
		})
		if payload != nil {
			enqueueIntel(TypeIntelPromote, payload)
		}
	case services.GateReviewRequired:
		slog.Info("task:intel:publish_gate: review-required", "record_id", p.RecordID, "confidence", record.ConfidenceOverall, "trust_level", source.TrustLevel)

		if err := intelStores.UpdateIntelRecordStatus(ctx, gen.UpdateIntelRecordStatusParams{
			ID:     recordID,
			Status: "review_required",
		}); err != nil {
			return fmt.Errorf("task:intel:publish_gate: set review status: %w", err)
		}
		// Stop the chain — record is in the review queue for human action.
	case services.GateReject:
		slog.Info("task:intel:publish_gate: rejected", "record_id", p.RecordID, "confidence", record.ConfidenceOverall, "trust_level", source.TrustLevel)

		// Stop the chain — record is rejected, no promotion.
		if err := intelStores.UpdateIntelRecordStatus(ctx, gen.UpdateIntelRecordStatusParams{
			ID:     recordID,
			Status: "rejected",
		}); err != nil {
			return fmt.Errorf("task:intel:publish_gate: set reject status: %w", err)
		}
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
	// Load the record to resolve its entity_type so we can dispatch to the
	// correct domain promoter from the registry.
	record, err := intelStores.GetIntelRecord(ctx, recordID)
	if err != nil {
		return fmt.Errorf("task:intel:promote: load record: %w", err)
	}

	prom, ok := intelPromoters[record.EntityType]
	if !ok {
		slog.Warn("task:intel:promote: no promoter registered for entity type, routing to review", "record_id", p.RecordID, "entity_type", record.EntityType)

		if err := intelStores.UpdateIntelRecordStatus(ctx, gen.UpdateIntelRecordStatusParams{
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

	slog.Info("task:intel:promote: promoted", "record_id", p.RecordID, "entity_type", record.EntityType, "action", result.Action, "domain_id", result.DomainID)

	intelMaybeAutoPromoteStudio(ctx, recordID, p.RecordID, record, result)
	intelMaybeEnqueueRegeneration(ctx, record, result)

	payload := marshalPayload(intelPostPublishPayload{
		Version:    intelPayloadVersion,
		RecordID:   p.RecordID,
		DomainID:   result.DomainID.String(),
		EntityType: record.EntityType,
	})
	if payload != nil {
		enqueueIntel(TypeIntelPostPublish, payload)
	}

	return nil
}

// intelMaybeAutoPromoteStudio implements ADR-076 Amendment 1: auto-promote
// high-confidence extracted records to Studio jobs for editorial generation.
// Gated by the studio.auto_promote_intel_records system setting (default
// false) and the intel.extraction_confidence_auto threshold. Only records
// whose promoter returned ContentID=uuid.Nil are eligible — that signals a
// projection-table write (Approach B) where Studio owns the editorial
// contents row. The guide_content promoter returns a real ContentID (it
// writes contents directly) and is skipped.
func intelMaybeAutoPromoteStudio(ctx context.Context, recordID pgtype.UUID, recordIDStr string, record gen.IntelRecord, result IntelPromoteResult) {
	if intelStudioAutoPromote == nil || result.ContentID != uuid.Nil {
		return
	}

	if appSettings == nil || !appSettings.GetBoolOr(ctx, "studio.auto_promote_intel_records", false) {
		return
	}

	autoThreshold := appSettings.GetFloatOr(ctx, "intel.extraction_confidence_auto", 0.85)
	if float64(record.ConfidenceOverall) < autoThreshold {
		return
	}

	if err := intelStudioAutoPromote.AutoPromoteIntelRecord(ctx, recordID, record.EntityType, record.Extracted); err != nil {
		// Non-fatal: the record is already 'extracted' and can be manually
		// promoted via the admin review UI.
		slog.Warn("task:intel:promote: auto-promote to studio failed", "record_id", recordIDStr, "entity_type", record.EntityType, "error", err)

		return
	}

	slog.Info("task:intel:promote: auto-promoted to studio", "record_id", recordIDStr, "entity_type", record.EntityType, "confidence", record.ConfidenceOverall)
}

// intelMaybeEnqueueRegeneration implements ADR-076 Amendment 2: when a
// promoter returned an existing ContentID (guide_content direct promotion or
// a projection table with a guide_id link), request a content body
// regeneration from the latest extraction so the narrative content stays in
// sync with the structured data.
func intelMaybeEnqueueRegeneration(ctx context.Context, record gen.IntelRecord, result IntelPromoteResult) {
	if result.ContentID == uuid.Nil || (result.Action != "created" && result.Action != "updated") {
		return
	}

	if appSettings == nil || !appSettings.GetBoolOr(ctx, "agent.regenerate_content_on_promote", false) {
		return
	}

	if taskEnqueuer == nil {
		return
	}

	regenPayload, err := json.Marshal(map[string]string{
		"content_id":      result.ContentID.String(),
		"intel_record_id": record.ID.String(),
	})
	if err != nil {
		return
	}

	if qerr := taskEnqueuer.Enqueue(TypeContentRegenerateFromSource, regenPayload); qerr != nil {
		slog.Warn("task:intel:promote: enqueue content regeneration failed", "record_id", record.ID, "content_id", result.ContentID, "error", qerr)
	}
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

	slog.Info("task:intel:post_publish: event published", "entity_type", p.EntityType, "domain_id", domainID.String())

	payload := marshalPayload(intelNotifyPayload{
		Version:    intelPayloadVersion,
		DomainID:   p.DomainID,
		EntityType: p.EntityType,
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
		s, err := intelStores.GetScholarship(ctx, domainID)
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
		slog.Info("task:intel:notify_discoveries: no geo space resolver for entity type, emitting generic discovery", "entity_type", p.EntityType, "domain_id", domainID.String())
	}

	// Publish the {entity_type}.discovered event with the geo space id so
	// the notification service can resolve followers of that space and
	// create discovery notifications.
	evtPayload, _ := json.Marshal(map[string]interface{}{
		"domain_id":        domainID.String(),
		"entity_type":      p.EntityType,
		"country_space_id": geoSpaceID,
		"country":          country,
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

	slog.Info("task:intel:notify_discoveries: discovery event published", "entity_type", p.EntityType, "domain_id", domainID.String(), "country_space_id", geoSpaceID)

	return nil
}

// HandleIntelRefreshFreshness is the cron entry point for freshness decay.
// It decays freshness scores for stale intel records based on their
// last_verified_at timestamp and re-enqueues fetch_source for sources
// whose content is overdue (Section 4, maintenance).
// // The freshness score is computed via services.FreshnessScore based on the
// record's last_verified_at age. When the score drops to 40 or below, the
// record is marked stale and the source is re-enqueued for a re-crawl.
func HandleIntelRefreshFreshness(ctx context.Context, _ *asynq.Task) error {
	defer recoverPanic("TypeIntelRefreshFreshness")
	slog.Info("task:intel:refresh_freshness")

	if intelStores == nil {
		slog.Warn("task:intel:refresh_freshness: store not wired, skipping")
		return nil
	}

	// No payload — processes all stale records for the default platform.
	// Per ADR-112 Amendment 2, all intelligence data is platform-scoped. The
	// cron runs per-platform; in a single-platform deployment the default platform
	// is used. Multi-platform deployments would enqueue one refresh task per
	// platform.
	stale, err := intelStores.ListStaleIntelRecords(ctx, 500)
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
		newScore := int32(services.FreshnessScore(record.LastVerifiedAt.Time, now)) // #nosec

		// Update the record's freshness score. The store method
		// automatically transitions status to 'stale' when score <= 40.
		if err := intelStores.UpdateIntelRecordFreshness(ctx, gen.UpdateIntelRecordFreshnessParams{
			ID:             record.ID,
			FreshnessScore: newScore,
		}); err != nil {
			slog.Error("task:intel:refresh_freshness: update freshness failed", "record_id", record.ID.String(), "error", err)
			continue
		}

		// Re-enqueue fetch_source for the source (deduped — only once per
		// source per freshness cycle, even if multiple records are stale).
		sourceKey := record.SourceID.String()
		if !reEnqueuedSources[sourceKey] {
			reEnqueuedSources[sourceKey] = true

			payload := marshalPayload(intelSourcePayload{
				Version:  intelPayloadVersion,
				SourceID: sourceKey,
			})
			if payload != nil {
				enqueueIntel(TypeIntelFetchSource, payload)
			}
		}
	}

	slog.Info("task:intel:refresh_freshness: processed", "stale_records", len(stale), "re-enqueued_sources", len(reEnqueuedSources))

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
	// Verify the source exists before re-enqueuing the fetch.
	if _, err := intelSourcesStore.Get(ctx, sourceID); err != nil {
		return fmt.Errorf("task:intel:retry_failed_source: get source: %w", err)
	}

	payload := marshalPayload(intelSourcePayload{
		Version:  intelPayloadVersion,
		SourceID: p.SourceID,
	})
	if payload != nil {
		enqueueIntel(TypeIntelFetchSource, payload)
	}

	return nil
}

// EnqueueIntelFetchChannel builds and enqueues an intel:fetch_channel task
// for a single channel — used by the admin channel-crawl endpoint and any
// future control-panel trigger that needs an immediate per-channel fetch.
// Mirrors EnqueueIntelProbeVerify (intel_probe.go).
func (e *Enqueuer) EnqueueIntelFetchChannel(channelID, sourceID string) error {
	payload, err := json.Marshal(intelChannelPayload{
		Version:   intelPayloadVersion,
		ChannelID: channelID,
		SourceID:  sourceID,
	})
	if err != nil {
		return err
	}

	return e.Enqueue(TypeIntelFetchChannel, payload)
}
