package sources

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// Channel store wrappers — thin adapters over the sqlc-generated
// intel_source_channels / intel_inbound_events queries, mirroring store.go:
// every error carries the "sources: <op>: %w" prefix so callers get a
// consistent, traceable error chain. All queries are platform-scoped (the
// post-de-tenant schema has no tenant_id columns), so wrappers take only
// IDs/values.

// CreateChannel inserts a new intel_source_channels row. A
// UNIQUE(source_id, entry_url, entity_types) collision maps to
// errpkg.ErrAlreadyExists.
//
// entity_types, relevance_topics, and relevance_url_patterns are
// text[] NOT NULL DEFAULT '{}' — a nil slice binds SQL NULL, overrides the
// default, and violates the constraint (SQLSTATE 23502). Normalize here so
// direct store callers (not just Service.CreateChannel) insert cleanly.
func (s *Store) CreateChannel(ctx context.Context, arg gen.CreateIntelSourceChannelParams) (Channel, error) {
	if arg.EntityTypes == nil {
		arg.EntityTypes = []string{}
	}
	if arg.RelevanceTopics == nil {
		arg.RelevanceTopics = []string{}
	}
	if arg.RelevanceUrlPatterns == nil {
		arg.RelevanceUrlPatterns = []string{}
	}

	row, err := s.queries.CreateIntelSourceChannel(ctx, arg)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return Channel{}, errpkg.ErrAlreadyExists
		}

		return Channel{}, fmt.Errorf("sources: create channel: %w", err)
	}

	return row, nil
}

// GetChannel fetches a single channel by its primary key.
func (s *Store) GetChannel(ctx context.Context, id ChannelID) (Channel, error) {
	row, err := s.queries.GetIntelSourceChannel(ctx, id)
	if err != nil {
		return Channel{}, fmt.Errorf("sources: get channel: %w", err)
	}

	return row, nil
}

// GetDefaultChannel resolves a source's oldest enabled channel — the legacy
// intel:fetch_source path uses it to delegate to the channel fetcher.
func (s *Store) GetDefaultChannel(ctx context.Context, sourceID SourceID) (Channel, error) {
	row, err := s.queries.GetIntelSourceDefaultChannel(ctx, sourceID)
	if err != nil {
		return Channel{}, fmt.Errorf("sources: get default channel: %w", err)
	}

	return row, nil
}

// ListChannels returns all channels owned by a source, ordered by
// created_at.
func (s *Store) ListChannels(ctx context.Context, sourceID SourceID) ([]Channel, error) {
	rows, err := s.queries.ListIntelSourceChannels(ctx, sourceID)
	if err != nil {
		return nil, fmt.Errorf("sources: list channels: %w", err)
	}

	return rows, nil
}

// ListDueChannels returns up to limit enabled channels whose crawl interval
// has elapsed (or that have never been crawled), oldest-first. The query
// joins intel_sources so channels on disabled/inactive sources are skipped,
// and excludes manual and inbound (email/webhook) methods.
func (s *Store) ListDueChannels(ctx context.Context, limit int32) ([]Channel, error) {
	rows, err := s.queries.ListDueIntelChannels(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("sources: list due channels: %w", err)
	}

	return rows, nil
}

// UpdateChannel edits the scalar fields of a channel (label, entry_url,
// crawl_method, pipeline, provider_config, crawl_frequency_mins, enabled)
// via COALESCE — nil fields are left unchanged. Array columns go through
// UpdateChannelEntityTypes / UpdateChannelRelevance instead. Returns the
// updated row.
func (s *Store) UpdateChannel(ctx context.Context, arg gen.UpdateIntelSourceChannelParams) (Channel, error) {
	row, err := s.queries.UpdateIntelSourceChannel(ctx, arg)
	if err != nil {
		return Channel{}, fmt.Errorf("sources: update channel: %w", err)
	}

	return row, nil
}

// UpdateChannelEntityTypes replaces a channel's entity_types array. Direct
// assignment (not COALESCE) so an empty array is an explicit clear.
func (s *Store) UpdateChannelEntityTypes(ctx context.Context, arg gen.UpdateIntelChannelEntityTypesParams) (Channel, error) {
	row, err := s.queries.UpdateIntelChannelEntityTypes(ctx, arg)
	if err != nil {
		return Channel{}, fmt.Errorf("sources: update channel entity types: %w", err)
	}

	return row, nil
}

// UpdateChannelRelevance replaces a channel's relevance gate fields
// (relevance_mode, relevance_topics, relevance_url_patterns) by direct
// assignment — empty arrays are explicit clears.
func (s *Store) UpdateChannelRelevance(ctx context.Context, arg gen.UpdateIntelChannelRelevanceParams) (Channel, error) {
	row, err := s.queries.UpdateIntelChannelRelevance(ctx, arg)
	if err != nil {
		return Channel{}, fmt.Errorf("sources: update channel relevance: %w", err)
	}

	return row, nil
}

// UpdateChannelEnabled toggles the enabled flag for a channel.
func (s *Store) UpdateChannelEnabled(ctx context.Context, id ChannelID, enabled bool) error {
	if err := s.queries.UpdateIntelChannelEnabled(ctx, gen.UpdateIntelChannelEnabledParams{
		ID:      id,
		Enabled: enabled,
	}); err != nil {
		return fmt.Errorf("sources: update channel enabled: %w", err)
	}

	return nil
}

// UpdateChannelCrawlStatus records the outcome of a channel crawl attempt:
// on success last_success_at is stamped and consecutive_failures reset; on
// failure last_error_at/message are stamped and the counter incremented.
func (s *Store) UpdateChannelCrawlStatus(ctx context.Context, arg gen.UpdateIntelChannelCrawlStatusParams) error {
	if err := s.queries.UpdateIntelChannelCrawlStatus(ctx, arg); err != nil {
		return fmt.Errorf("sources: update channel crawl status: %w", err)
	}

	return nil
}

// DeleteChannel removes a channel. intel_inbound_events and
// intel_raw_documents references are handled by the FK clauses (CASCADE /
// SET NULL respectively).
func (s *Store) DeleteChannel(ctx context.Context, id ChannelID) error {
	if err := s.queries.DeleteIntelSourceChannel(ctx, id); err != nil {
		return fmt.Errorf("sources: delete channel: %w", err)
	}

	return nil
}

// --- Inbound events ---

// CreateInboundEvent stores a raw push payload delivered to an inbound
// (email/webhook) channel. The content_hash is the idempotency key the
// handler computes before insert.
func (s *Store) CreateInboundEvent(ctx context.Context, arg gen.CreateIntelInboundEventParams) (InboundEvent, error) {
	row, err := s.queries.CreateIntelInboundEvent(ctx, arg)
	if err != nil {
		return InboundEvent{}, fmt.Errorf("sources: create inbound event: %w", err)
	}

	return row, nil
}

// MarkInboundEvent stamps an inbound event's terminal status
// (received|processed|failed|ignored) and sets processed_at.
func (s *Store) MarkInboundEvent(ctx context.Context, arg gen.MarkIntelInboundEventParams) error {
	if err := s.queries.MarkIntelInboundEvent(ctx, arg); err != nil {
		return fmt.Errorf("sources: mark inbound event: %w", err)
	}

	return nil
}

// UpdateSourceProbe stores a probe run's results on the parent source row
// (probe_report JSON + flattened detected_methods + last_probed_at).
func (s *Store) UpdateSourceProbe(ctx context.Context, arg gen.UpdateIntelSourceProbeParams) error {
	if err := s.queries.UpdateIntelSourceProbe(ctx, arg); err != nil {
		return fmt.Errorf("sources: update source probe: %w", err)
	}

	return nil
}
