// Package sources implements the intel_sources registry for the
// Intelligence System (source idea §17).
//
// Ref: docs/superpowers/specs/2026-08-12-japa-intelligence-system-design.md
package sources

import (
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
)

// Source is the domain type for a registered intelligence source. It is an
// alias for the sqlc-generated IntelSource struct so it can be passed
// interchangeably with the generated type while reading cleanly in
// downstream package signatures (fetcher, extractor, promoter, scheduler).
type Source = gen.IntelSource

// SourceID is the primary-key type for an intel source row.
type SourceID = pgtype.UUID

// TrustLevel classifies source authority (source idea §6).
type TrustLevel int16

// Exported const values.
const (
	// TrustLevelOfficial is an exported constant.
	TrustLevelOfficial   TrustLevel = 1 // government, embassy, university, immigration authority, central bank
	TrustLevelTrusted    TrustLevel = 2 // international org, major financial institution, established educational org
	TrustLevelAggregator TrustLevel = 3 // scholarship databases, news, commercial, blogs
	TrustLevelCommunity  TrustLevel = 4 // reddit, forums, social media, user submissions
)

// SourceType labels what a source represents — descriptive, not the crawl
// mechanism (that is crawl_method).
type SourceType string

// Exported const values.
const (
	// SourceTypeVideo is the user-facing video-platform type — mapped to
	// SourceTypeVideoChannel on create/propose when the URL is a supported
	// provider (§106 channel-sync amendment).
	SourceTypeVideo SourceType = "video"
	// SourceTypeVideoChannel marks a video channel subscription row — synced
	// by video:sync_channels, never crawled by the intel pipeline.
	SourceTypeVideoChannel SourceType = "video_channel"
)

// CrawlMethod selects the fetcher adapter.
type CrawlMethod string

// Exported const values.
const (
	// CrawlMethodStatic is an exported constant.
	CrawlMethodStatic CrawlMethod = "static"
	CrawlMethodRSS    CrawlMethod = "rss"
	CrawlMethodAPI    CrawlMethod = "api"
	CrawlMethodPDF    CrawlMethod = "pdf"
	CrawlMethodOCR    CrawlMethod = "ocr"
	// CrawlMethodManual marks registry-only rows the crawl scheduler must
	// skip (video channels, entity-linked sources).
	CrawlMethodManual CrawlMethod = "manual"
	// Intel-channels phase-1 adapter methods (migration 000681 / channel
	// design §4): probe-discovered fetch strategies owned by channels.
	CrawlMethodRender   CrawlMethod = "render"
	CrawlMethodJSONLD   CrawlMethod = "jsonld"
	CrawlMethodTabular  CrawlMethod = "tabular"
	CrawlMethodSitemap  CrawlMethod = "sitemap"
	CrawlMethodICal     CrawlMethod = "ical"
	CrawlMethodGit      CrawlMethod = "git"
	CrawlMethodSocial   CrawlMethod = "social"
	CrawlMethodSearch   CrawlMethod = "search"
	CrawlMethodWayback  CrawlMethod = "wayback"
	CrawlMethodXHRSniff CrawlMethod = "xhr_sniff"
	CrawlMethodOneshot  CrawlMethod = "oneshot"
	// CrawlMethodSpider is the Phase-4 bounded same-host crawl: the entry
	// page (and optionally its children to a shallow depth) is fetched as
	// HTML and rewritten into a url<TAB>anchor-stamp manifest that fans out
	// like a sitemap — discovery of pages a site omits from feeds/sitemaps.
	CrawlMethodSpider CrawlMethod = "spider"
	// CrawlMethodEmail and CrawlMethodWebhook are inbound push methods —
	// they ingest via the inbound endpoints writing intel_inbound_events
	// and are never returned by ListDueChannels.
	CrawlMethodEmail   CrawlMethod = "email"
	CrawlMethodWebhook CrawlMethod = "webhook"
)

// SourceKind classifies who or what a source represents — the expanded
// CHECK set on intel_sources.source_kind from migration 000681. It drives
// probe/verify defaults (channel_profile → video/social probes) and public
// directory labelling.
type SourceKind string

// Exported const values.
const (
	// SourceKindPublisher is an exported constant.
	SourceKindPublisher       SourceKind = "publisher"
	SourceKindInstitution     SourceKind = "institution"
	SourceKindIndividual      SourceKind = "individual"
	SourceKindAggregator      SourceKind = "aggregator"
	SourceKindChannelProfile  SourceKind = "channel_profile"
	SourceKindInstitutionSite SourceKind = "institution_site"
	SourceKindPersonalBlog    SourceKind = "personal_blog"
	SourceKindJobBoard        SourceKind = "job_board"
	SourceKindGovPortal       SourceKind = "gov_portal"
	SourceKindCommunity       SourceKind = "community"
)

// IsValid reports whether k is a source_kind value the CHECK constraint
// allows.
func (k SourceKind) IsValid() bool {
	switch k {
	case SourceKindPublisher, SourceKindInstitution, SourceKindIndividual,
		SourceKindAggregator, SourceKindChannelProfile, SourceKindInstitutionSite,
		SourceKindPersonalBlog, SourceKindJobBoard, SourceKindGovPortal,
		SourceKindCommunity:
		return true
	}

	return false
}

// IsSubmittable reports whether k may be chosen on a public source proposal.
// Institution and individual are entity-stub kinds assigned only by
// CreateSourceFromEntity (org/space/user-backed rows, crawl_method=manual);
// user-facing equivalents are institution_site, personal_blog and
// channel_profile.
func (k SourceKind) IsSubmittable() bool {
	return k.IsValid() && k != SourceKindInstitution && k != SourceKindIndividual
}

// CrawlFrequency is a volatility bucket.
type CrawlFrequency string

// Exported const values.
const (
	// CrawlFrequencyHourly is an exported constant.
	CrawlFrequencyHourly  CrawlFrequency = "hourly"
	CrawlFrequencyDaily   CrawlFrequency = "daily"
	CrawlFrequencyWeekly  CrawlFrequency = "weekly"
	CrawlFrequencyMonthly CrawlFrequency = "monthly"
	// CrawlFrequencyManual marks rows that are never auto-crawled.
	CrawlFrequencyManual CrawlFrequency = "manual"
)

// Category groups sources by data domain.
type Category string

// Exported const values.
const (
	// CategoryScholarship is an exported constant.
	CategoryScholarship Category = "scholarship"
	CategoryJob         Category = "job"
	CategoryVisa        Category = "visa"
	CategoryFX          Category = "fx"
	CategoryRemittance  Category = "remittance"
	CategoryNews        Category = "news"
	CategoryTravelAlert Category = "travel_alert"
)
