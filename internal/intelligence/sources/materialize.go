package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// ChannelProposalInput is the plain-struct mirror of probe.ChannelProposal
// consumed by Service.MaterializeChannels. The sources package must not
// import probe (probe already imports sources — the cycle would close), so
// the worker's intel:probe_verify handler adapts report proposals into
// this shape. Fields match the probe contract; ProviderConfig is the
// marshalled provider_config seed.
type ChannelProposalInput struct {
	Label       string
	EntryURL    string
	CrawlMethod string
	EntityTypes []string
	Pipeline    string
	// ProviderConfig is the per-method config seed (feed hub, platform
	// markers); the probe's confidence is stamped in as probe_confidence.
	ProviderConfig []byte
	Confidence     float64
	// RelevanceTopics seeds channel.relevance_topics. Probe fills it from
	// the resolved category map for category-ranked methods and leaves it
	// empty for platform/inbound/discovery proposals; empty falls back to
	// the resolved map here for non-platform methods.
	RelevanceTopics []string
}

// methodChannelSync mirrors probe.MethodChannelSync — the routing marker
// for the §106 video_channel flow. Duplicated here because sources cannot
// import probe (probe imports sources); the value is contractual: a
// channel_sync proposal never produces an intel_source_channels row —
// video:sync_channels owns ingestion on the already video_channel-typed
// source row.
const methodChannelSync = "channel_sync"

// platformProbeMethods are proposal methods that bypass the
// category→method intersection in the probe (platform signatures and
// discovery multipliers). For these the resolved category map does NOT
// seed relevance_topics or pipeline — the method default applies, matching
// probe.fillProposal.
var platformProbeMethods = map[CrawlMethod]bool{
	CrawlMethodEmail:   true,
	CrawlMethodWebhook: true,
	CrawlMethodSitemap: true,
	CrawlMethodSocial:  true,
	CrawlMethodGit:     true,
}

// curateDefaultMethods mirrors probe's method-default pipeline table:
// itemized/structured record producers curate by default; everything else
// extracts. Duplicated because probe's table is unexported and sources
// cannot import probe.
var curateDefaultMethods = map[CrawlMethod]bool{
	CrawlMethodRSS:     true,
	CrawlMethodJSONLD:  true,
	CrawlMethodICal:    true,
	CrawlMethodTabular: true,
	CrawlMethodSitemap: true,
	CrawlMethodGit:     true,
	CrawlMethodSocial:  true,
	CrawlMethodWebhook: true,
	CrawlMethodSearch:  true,
	CrawlMethodWayback: true,
}

// defaultChannelPipeline is the per-method pipeline used when neither the
// proposal nor the resolved category map supplies one.
func defaultChannelPipeline(method CrawlMethod) Pipeline {
	if curateDefaultMethods[method] {
		return PipelineCurate
	}

	return PipelineExtract
}

// sourceCategories returns the source's effective category list — the
// multi-category array first, the legacy scalar as fallback.
func sourceCategories(src Source) []string {
	if len(src.Categories) > 0 {
		return src.Categories
	}
	if src.Category != "" {
		return []string{src.Category}
	}

	return nil
}

// MaterializeChannels turns verify-probe channel proposals into
// intel_source_channels rows for sourceID (spec §2: "writes final
// channels"). It is idempotent: the UNIQUE(source_id, entry_url,
// entity_types) constraint makes a re-run count every existing row as
// skipped instead of duplicating it.
//
// Per-proposal rules:
//   - channel_sync proposals produce no row — they route to the §106
//     video_channel flow and the source row is already video_channel-typed.
//   - unknown crawl methods are skipped with a warning.
//   - entity_types: proposal first, resolved category map as fill; when an
//     extract/curate pipeline still has none the proposal is skipped
//     (Task 2's create validation would reject it anyway).
//   - inbound methods (email/webhook) are created enabled — they are
//     never scheduled (ListDueChannels excludes them); the inbound
//     endpoints own ingestion.
//   - every created row is stamped origin='probe' and
//     relevance_mode='filtered', with relevance_topics from the proposal
//     or — for category-ranked methods only — the resolved category map.
func (s *Service) MaterializeChannels(ctx context.Context, sourceID SourceID, proposals []ChannelProposalInput) (created, skipped int, err error) {
	src, err := s.store.Get(ctx, sourceID)
	if err != nil {
		return 0, 0, fmt.Errorf("sources: materialize channels: load source: %w", err)
	}

	resolved := ResolveCategoryMap(nil, sourceCategories(src))

	for _, prop := range proposals {
		arg, skipReason := materializeProposal(sourceID, prop, resolved)
		if skipReason != "" {
			skipped++
			slog.Debug("sources: materialize channels: proposal skipped",
				"source_id", sourceID.String(), "reason", skipReason,
				"entry_url", prop.EntryURL, "method", prop.CrawlMethod)

			continue
		}

		if _, err := s.CreateChannel(ctx, arg); err != nil {
			switch {
			case errors.Is(err, errpkg.ErrAlreadyExists):
				// UNIQUE(source_id, entry_url, entity_types) hit — the
				// re-run dedupe path.
				skipped++
			case errors.Is(err, errpkg.ErrInvalid), errors.Is(err, errpkg.ErrRequired):
				// A proposal defect — skip it rather than failing the batch.
				skipped++
				slog.Warn("sources: materialize channels: proposal rejected",
					"source_id", sourceID.String(), "entry_url", prop.EntryURL,
					"method", prop.CrawlMethod, "error", err)
			default:
				return created, skipped, fmt.Errorf("sources: materialize channels: %w", err)
			}

			continue
		}

		created++
	}

	created += s.materializeFallbackChannel(ctx, src, resolved)

	return created, skipped, nil
}

// materializeFallbackChannel guarantees an approved source is never left
// channel-less: when every proposal was skipped or deduped and the source
// has no channels at all, it creates a minimal static+extract channel on
// source_url — the same shape the 000681 backfill produced for legacy
// sources — so the source is still scheduled and produces records (or
// review stubs for categories with no entity mapping). The row goes
// through the store path like ensureDefaultChannel so empty entity_types
// are allowed (runtime resolution falls back to the source's category).
// Re-probes where channels already exist are untouched — an admin's
// disabled-channel decision is never overridden. Returns 1 when the
// fallback row was created, else 0.
func (s *Service) materializeFallbackChannel(ctx context.Context, src Source, resolved ResolvedMap) int {
	channels, err := s.store.ListChannels(ctx, src.ID)
	if err != nil {
		slog.Warn("sources: materialize channels: fallback check failed",
			"source_id", src.ID.String(), "error", err)
		return 0
	}
	if len(channels) > 0 {
		return 0 // channels exist — re-probe dedupe, not a dead source
	}

	if src.SourceUrl == "" {
		return 0
	}
	// Strict literal check ("" = no same-host carve-out): a private-literal
	// source_url gets no fallback — the dial guard would refuse every crawl.
	if err := validateChannelEntryURL(src.SourceUrl, CrawlMethodStatic, ""); err != nil {
		slog.Warn("sources: materialize channels: fallback entry_url rejected",
			"source_id", src.ID.String(), "entry_url", src.SourceUrl, "error", err)
		return 0
	}

	pipeline := resolved.Pipeline
	if !pipeline.IsValid() {
		pipeline = PipelineExtract
	}

	freq := src.CrawlFrequencyMins
	if freq <= 0 {
		freq = 1440
	}

	_, err = s.store.CreateChannel(ctx, gen.CreateIntelSourceChannelParams{
		SourceID:             src.ID,
		Label:                "Default",
		EntryUrl:             src.SourceUrl,
		CrawlMethod:          string(CrawlMethodStatic),
		EntityTypes:          resolved.EntityTypes,
		Pipeline:             string(pipeline),
		ProviderConfig:       []byte("{}"),
		CrawlFrequencyMins:   freq,
		Enabled:              src.Enabled,
		Origin:               string(ChannelOriginProbe),
		RelevanceMode:        string(RelevanceFiltered),
		RelevanceTopics:      resolved.Topics,
		RelevanceUrlPatterns: []string{},
	})
	if err != nil {
		if errors.Is(err, errpkg.ErrAlreadyExists) {
			return 0 // raced with a concurrent materialization — nothing to do
		}

		slog.Warn("sources: materialize channels: fallback channel create failed",
			"source_id", src.ID.String(), "entry_url", src.SourceUrl, "error", err)

		return 0
	}

	slog.Warn("sources: materialize channels: no proposals survived; created static fallback channel",
		"source_id", src.ID.String(), "entry_url", src.SourceUrl)

	return 1
}

// materializeProposal builds the create params for one probe proposal, or
// reports a skip reason. Pure — no store access — so the decision rules
// are unit-testable without a database.
func materializeProposal(sourceID SourceID, prop ChannelProposalInput, resolved ResolvedMap) (gen.CreateIntelSourceChannelParams, string) {
	method := CrawlMethod(strings.TrimSpace(prop.CrawlMethod))

	switch {
	case string(method) == methodChannelSync:
		return gen.CreateIntelSourceChannelParams{}, "channel_sync routes to the video_channel flow"
	case !method.IsValid():
		return gen.CreateIntelSourceChannelParams{}, fmt.Sprintf("unknown crawl_method %q", prop.CrawlMethod)
	}

	pipeline := Pipeline(strings.TrimSpace(prop.Pipeline))
	if !pipeline.IsValid() {
		if resolved.Pipeline.IsValid() && !platformProbeMethods[method] {
			pipeline = resolved.Pipeline
		} else {
			pipeline = defaultChannelPipeline(method)
		}
	}

	entityTypes := prop.EntityTypes
	if len(entityTypes) == 0 {
		entityTypes = resolved.EntityTypes
	}
	if pipelineRequiresEntityTypes(pipeline) && len(entityTypes) == 0 {
		return gen.CreateIntelSourceChannelParams{}, "entity_types empty after category-map fill"
	}

	topics := prop.RelevanceTopics
	if len(topics) == 0 && !platformProbeMethods[method] {
		topics = resolved.Topics
	}

	label := strings.TrimSpace(prop.Label)
	if label == "" {
		label = string(method) + " channel"
	}

	return gen.CreateIntelSourceChannelParams{
		SourceID:           sourceID,
		Label:              label,
		EntryUrl:           strings.TrimSpace(prop.EntryURL),
		CrawlMethod:        string(method),
		EntityTypes:        entityTypes,
		Pipeline:           string(pipeline),
		ProviderConfig:     stampProbeConfidence(prop.ProviderConfig, prop.Confidence),
		Enabled:            true,
		Origin:             string(ChannelOriginProbe),
		RelevanceMode:      string(RelevanceFiltered),
		RelevanceTopics:    topics,
		CrawlFrequencyMins: 1440,
	}, ""
}

// stampProbeConfidence merges the probe's detection confidence into the
// provider_config seed as probe_confidence — provenance for the admin
// channels editor. An unparseable config is preserved verbatim.
func stampProbeConfidence(pc []byte, confidence float64) []byte {
	var m map[string]any

	if len(pc) > 0 {
		if err := json.Unmarshal(pc, &m); err != nil || m == nil {
			return pc
		}
	} else {
		m = map[string]any{}
	}

	if confidence > 0 {
		m["probe_confidence"] = confidence
	}

	b, err := json.Marshal(m)
	if err != nil {
		return pc
	}

	return b
}
