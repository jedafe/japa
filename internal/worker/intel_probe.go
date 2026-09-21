package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/intelligence/probe"
	"github.com/jedafe/japa/internal/intelligence/sources"
)

// ---------------------------------------------------------------------------
// intel:probe_verify — the deep post-approval probe (intel-channels spec §2).
// Enqueued by sources.Service.ApproveProposed via the injected probe queue
// (and by the admin re-probe endpoint, Task 12). The handler loads the
// source, runs probe.Verify (sitemap walk + per-category discovery paths +
// feed re-checks under a larger request budget), persists the report +
// flattened detected_methods on the source row, then materializes
// intel_source_channels via the sources service.
//
// Idempotent: re-runs re-probe but the channel materializer dedupes on
// UNIQUE(source_id, entry_url, entity_types), so a second run creates zero
// channels.
// ---------------------------------------------------------------------------

// intelProber is the verify-configured prober. Nil until
// RegisterIntelDependencies wires it from server.go (alongside the
// Task 5 preview prober — a separate instance with the larger verify
// request budget).
var intelProber *probe.Prober

// verifySkipSource reports whether a loaded source is out of scope for the
// verify probe. §106 video_channel rows are channel subscriptions owned by
// video:sync_channels — a probe would only produce a channel_sync
// proposal, which materializes to nothing. Manual rows that never came
// through the propose flow (source_type != "auto" — entity-linked and
// other registry-only rows) carry nothing to probe either; note that
// approved proposals are crawl_method='manual' by design (channels own
// crawling) but keep source_type='auto', so they still verify.
func verifySkipSource(src gen.IntelSource) bool {
	return src.SourceType == string(sources.SourceTypeVideoChannel) ||
		(src.CrawlMethod == string(sources.CrawlMethodManual) && src.SourceType != "auto")
}

// sourceVerifyCategories returns the categories the prober resolves
// against — the multi-category array, falling back to the legacy scalar.
func sourceVerifyCategories(src gen.IntelSource) []string {
	if len(src.Categories) > 0 {
		return src.Categories
	}
	if src.Category != "" {
		return []string{src.Category}
	}

	return nil
}

// proposalInputs adapts probe.ChannelProposal → sources.ChannelProposalInput
// (the sources package must not import probe). ProviderConfig maps are
// marshalled; unmarshallable maps degrade to nil (the materializer
// defaults to "{}").
func proposalInputs(proposals []probe.ChannelProposal) []sources.ChannelProposalInput {
	out := make([]sources.ChannelProposalInput, 0, len(proposals))

	for _, p := range proposals {
		in := sources.ChannelProposalInput{
			Label:           p.Label,
			EntryURL:        p.EntryURL,
			CrawlMethod:     p.CrawlMethod,
			EntityTypes:     p.EntityTypes,
			Pipeline:        p.Pipeline,
			Confidence:      p.Confidence,
			RelevanceTopics: p.RelevanceTopics,
		}

		if len(p.ProviderConfig) > 0 {
			if b, err := json.Marshal(p.ProviderConfig); err == nil {
				in.ProviderConfig = b
			}
		}

		out = append(out, in)
	}

	return out
}

// detectedMethodNames flattens report.Methods to the method-name list
// persisted on intel_sources.detected_methods.
func detectedMethodNames(methods []probe.DetectedMethod) []string {
	out := make([]string, 0, len(methods))
	for _, m := range methods {
		out = append(out, m.Method)
	}

	return out
}

// HandleIntelProbeVerify runs the deep verify probe for one approved
// source and materializes its intel_source_channels rows.
func HandleIntelProbeVerify(ctx context.Context, t *asynq.Task) error {
	defer recoverPanic("TypeIntelProbeVerify")
	slog.Info("task:intel:probe_verify")

	if intelProber == nil || intelSourcesStore == nil {
		slog.Warn("task:intel:probe_verify: dependencies not wired, skipping")
		return nil
	}

	var p intelSourcePayload // {version, source_id} — shared with fetch_source
	if err := parsePayloadOrSkip(t, &p); err != nil {
		return err
	}

	sourceID, err := scanUUID(p.SourceID)
	if err != nil {
		return fmt.Errorf("task:intel:probe_verify: %w: %w", err, asynq.SkipRetry)
	}

	src, err := intelSourcesStore.Get(ctx, sourceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Info("task:intel:probe_verify: source no longer exists, skipping", "source_id", p.SourceID)
			return nil
		}

		return fmt.Errorf("task:intel:probe_verify: load source: %w", err)
	}

	if !src.Enabled || src.Status != "active" {
		slog.Info("task:intel:probe_verify: source disabled or not active, skipping",
			"source_id", p.SourceID, "enabled", src.Enabled, "status", src.Status)
		return nil
	}

	if verifySkipSource(src) {
		slog.Info("task:intel:probe_verify: source type is verify-exempt, skipping",
			"source_id", p.SourceID, "source_type", src.SourceType, "crawl_method", src.CrawlMethod)
		return nil
	}

	maxURLs := probe.DefaultVerifyMaxURLs
	if appSettings != nil {
		maxURLs = appSettings.GetIntOr(ctx, "intel.probe.max_urls", probe.DefaultVerifyMaxURLs)
	}

	report, err := intelProber.Verify(ctx, src.SourceUrl, src.SourceKind, sourceVerifyCategories(src), probe.VerifyOpts{
		MaxURLs: maxURLs,
	})
	if err != nil {
		return fmt.Errorf("task:intel:probe_verify: verify: %w", err)
	}

	reportJSON, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("task:intel:probe_verify: marshal report: %w", err)
	}

	methods := detectedMethodNames(report.Methods)

	if err := intelSourcesStore.UpdateSourceProbe(ctx, gen.UpdateIntelSourceProbeParams{
		ID:              sourceID,
		ProbeReport:     reportJSON,
		DetectedMethods: methods,
	}); err != nil {
		return fmt.Errorf("task:intel:probe_verify: store probe report: %w", err)
	}

	created, skipped, err := sources.NewService(intelSourcesStore).
		MaterializeChannels(ctx, sourceID, proposalInputs(report.Channels))
	if err != nil {
		return fmt.Errorf("task:intel:probe_verify: materialize channels: %w", err)
	}

	slog.Info("task:intel:probe_verify: done",
		"source_id", p.SourceID,
		"detected_methods", len(methods),
		"channels_created", created,
		"channels_skipped", skipped,
		"warnings", len(report.Warnings))

	return nil
}

// EnqueueIntelProbeVerify builds and enqueues an intel:probe_verify task
// for a source — used by the sources service probe queue and the admin
// re-probe endpoint.
func (e *Enqueuer) EnqueueIntelProbeVerify(sourceID string) error {
	payload, err := json.Marshal(intelSourcePayload{
		Version:  intelPayloadVersion,
		SourceID: sourceID,
	})
	if err != nil {
		return err
	}

	return e.Enqueue(TypeIntelProbeVerify, payload)
}
