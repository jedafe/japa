package sources

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// Channel is the domain type for one entry point under an intel_sources row
// (entry URL + crawl method + entity pipeline). It is an alias for the
// sqlc-generated IntelSourceChannel so it can be passed interchangeably with
// the generated type while reading cleanly in downstream signatures
// (fetcher, extractor, scheduler, inbound handlers).
type Channel = gen.IntelSourceChannel

// ChannelID is the primary-key type for an intel_source_channels row.
type ChannelID = pgtype.UUID

// InboundEvent is the domain type for a push-payload row written by the
// inbound email/webhook endpoints before pipeline processing.
type InboundEvent = gen.IntelInboundEvent

// Pipeline selects which downstream processing path a channel's records
// take (migration 000681 CHECK: extract|curate|hybrid).
type Pipeline string

// Exported const values.
const (
	// PipelineExtract sends the page body to the LLM extractor; records go
	// through intel_records regardless of source trust.
	PipelineExtract Pipeline = "extract"
	// PipelineCurate promotes feed/itemized records directly; trust_level
	// decides auto-publish vs review queue.
	PipelineCurate Pipeline = "curate"
	// PipelineHybrid runs the curate phase per item and the extract phase
	// per page.
	PipelineHybrid Pipeline = "hybrid"
)

// IsValid reports whether p is a pipeline value the CHECK constraint allows.
func (p Pipeline) IsValid() bool {
	switch p {
	case PipelineExtract, PipelineCurate, PipelineHybrid:
		return true
	}

	return false
}

// pipelineRequiresEntityTypes reports whether the pipeline produces typed
// records that must be validated against channel.entity_types schemas —
// the extract and curate pipelines (spec: "channel.entity_types[] is the
// single source of truth for which schema validates each record").
func pipelineRequiresEntityTypes(p Pipeline) bool {
	return p == PipelineExtract || p == PipelineCurate
}

// RelevanceMode controls the per-item relevance gate on multi-niche sources
// (migration 000681 CHECK: open|filtered|custom).
type RelevanceMode string

// Exported const values.
const (
	// RelevanceOpen disables gating — every item is processed.
	RelevanceOpen RelevanceMode = "open"
	// RelevanceFiltered gates items on the source's category/topic defaults.
	RelevanceFiltered RelevanceMode = "filtered"
	// RelevanceCustom gates items on the channel's relevance_topics and
	// relevance_url_patterns.
	RelevanceCustom RelevanceMode = "custom"
)

// IsValid reports whether m is a relevance_mode value the CHECK constraint
// allows.
func (m RelevanceMode) IsValid() bool {
	switch m {
	case RelevanceOpen, RelevanceFiltered, RelevanceCustom:
		return true
	}

	return false
}

// ChannelOrigin records how a channel was created (migration 000681 CHECK:
// probe|admin|user).
type ChannelOrigin string

// Exported const values.
const (
	// ChannelOriginProbe marks channels discovered by the two-stage probe.
	ChannelOriginProbe ChannelOrigin = "probe"
	// ChannelOriginAdmin marks channels created by an admin (incl. backfill).
	ChannelOriginAdmin ChannelOrigin = "admin"
	// ChannelOriginUser marks channels proposed by an authenticated user.
	ChannelOriginUser ChannelOrigin = "user"
)

// IsValid reports whether o is an origin value the CHECK constraint allows.
func (o ChannelOrigin) IsValid() bool {
	switch o {
	case ChannelOriginProbe, ChannelOriginAdmin, ChannelOriginUser:
		return true
	}

	return false
}

// IsValid reports whether m is a crawl method the fetcher adapters or the
// inbound endpoints know how to handle.
func (m CrawlMethod) IsValid() bool {
	switch m {
	case CrawlMethodStatic, CrawlMethodRSS, CrawlMethodAPI, CrawlMethodPDF,
		CrawlMethodOCR, CrawlMethodManual,
		CrawlMethodRender, CrawlMethodJSONLD, CrawlMethodTabular,
		CrawlMethodSitemap, CrawlMethodICal, CrawlMethodGit, CrawlMethodSocial,
		CrawlMethodSearch, CrawlMethodWayback, CrawlMethodXHRSniff,
		CrawlMethodOneshot, CrawlMethodEmail, CrawlMethodWebhook:
		return true
	}

	return false
}

// InboundMethod reports whether m is a push method (never scheduled —
// ListDueChannels excludes it and the inbound endpoints own ingestion).
func InboundMethod(m CrawlMethod) bool {
	return m == CrawlMethodEmail || m == CrawlMethodWebhook
}

// validateChannelCreate applies the domain rules for a new channel row after
// the service has filled defaults: known crawl_method/pipeline/relevance_mode/
// origin, a fetchable http(s) entry_url for pull methods (inbound channels
// only require a non-empty entry point — email/webhook entry URLs may be
// mailto: addresses or provider identifiers), and entity_types for the
// record-producing pipelines.
func validateChannelCreate(arg gen.CreateIntelSourceChannelParams) error {
	if !arg.SourceID.Valid {
		return fmt.Errorf("sources: channel: %w: source_id", errpkg.ErrRequired)
	}
	if arg.Label == "" {
		return fmt.Errorf("sources: channel: %w: label", errpkg.ErrRequired)
	}

	method := CrawlMethod(arg.CrawlMethod)
	if !method.IsValid() {
		return fmt.Errorf("sources: channel: %w: unknown crawl_method %q", errpkg.ErrInvalid, arg.CrawlMethod)
	}

	if err := validateChannelEntryURL(arg.EntryUrl, method); err != nil {
		return err
	}

	pipeline := Pipeline(arg.Pipeline)
	if !pipeline.IsValid() {
		return fmt.Errorf("sources: channel: %w: unknown pipeline %q", errpkg.ErrInvalid, arg.Pipeline)
	}
	if pipelineRequiresEntityTypes(pipeline) && len(arg.EntityTypes) == 0 {
		return fmt.Errorf("sources: channel: %w: entity_types required for %s pipeline", errpkg.ErrInvalid, pipeline)
	}

	if !RelevanceMode(arg.RelevanceMode).IsValid() {
		return fmt.Errorf("sources: channel: %w: unknown relevance_mode %q", errpkg.ErrInvalid, arg.RelevanceMode)
	}
	if !ChannelOrigin(arg.Origin).IsValid() {
		return fmt.Errorf("sources: channel: %w: unknown origin %q", errpkg.ErrInvalid, arg.Origin)
	}

	return nil
}

// validateChannelEntryURL enforces the entry point shape: pull methods must
// be fetchable http(s) URLs; inbound methods accept any non-empty entry
// point (mailto:, provider address, webhook slug).
func validateChannelEntryURL(entryURL string, method CrawlMethod) error {
	if entryURL == "" {
		return fmt.Errorf("sources: channel: %w: entry_url", errpkg.ErrRequired)
	}
	if InboundMethod(method) {
		return nil
	}

	u, err := url.Parse(entryURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("sources: channel: %w: entry_url must be an http(s) URL", errpkg.ErrInvalid)
	}

	return nil
}

// validateChannelUpdate applies the per-field rules for a COALESCE-style
// channel patch: only non-nil fields are checked, and trimmed values are
// written back so whitespace-only input cannot reach the database. The
// service performs the cross-field checks that need the existing row
// (e.g. pipeline → entity_types) separately.
func validateChannelUpdate(arg gen.UpdateIntelSourceChannelParams) error {
	if arg.Label != nil {
		label := strings.TrimSpace(*arg.Label)
		if label == "" {
			return fmt.Errorf("sources: channel: %w: label must not be empty", errpkg.ErrInvalid)
		}
		*arg.Label = label
	}

	if arg.EntryUrl != nil {
		entryURL := strings.TrimSpace(*arg.EntryUrl)
		// Strict http(s) for every channel on PATCH — unlike create, the
		// patch payload alone cannot prove the channel is inbound, so the
		// relaxed inbound shape is only allowed through CreateChannel.
		u, err := url.Parse(entryURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("sources: channel: %w: entry_url must be an http(s) URL", errpkg.ErrInvalid)
		}
		*arg.EntryUrl = entryURL
	}

	if arg.CrawlMethod != nil && !CrawlMethod(*arg.CrawlMethod).IsValid() {
		return fmt.Errorf("sources: channel: %w: unknown crawl_method %q", errpkg.ErrInvalid, *arg.CrawlMethod)
	}

	if arg.Pipeline != nil && !Pipeline(*arg.Pipeline).IsValid() {
		return fmt.Errorf("sources: channel: %w: unknown pipeline %q", errpkg.ErrInvalid, *arg.Pipeline)
	}

	return nil
}
