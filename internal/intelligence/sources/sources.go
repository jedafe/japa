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
)

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
