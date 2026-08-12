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

const (
	TrustLevelOfficial   TrustLevel = 1 // government, embassy, university, immigration authority, central bank
	TrustLevelTrusted    TrustLevel = 2 // international org, major financial institution, established educational org
	TrustLevelAggregator TrustLevel = 3 // scholarship databases, news, commercial, blogs
	TrustLevelCommunity  TrustLevel = 4 // reddit, forums, social media, user submissions
)

// CrawlMethod selects the fetcher adapter.
type CrawlMethod string

const (
	CrawlMethodStatic CrawlMethod = "static"
	CrawlMethodRSS    CrawlMethod = "rss"
	CrawlMethodAPI    CrawlMethod = "api"
	CrawlMethodPDF    CrawlMethod = "pdf"
	CrawlMethodOCR    CrawlMethod = "ocr"
)

// CrawlFrequency is a volatility bucket.
type CrawlFrequency string

const (
	CrawlFrequencyHourly  CrawlFrequency = "hourly"
	CrawlFrequencyDaily   CrawlFrequency = "daily"
	CrawlFrequencyWeekly  CrawlFrequency = "weekly"
	CrawlFrequencyMonthly CrawlFrequency = "monthly"
)

// Category groups sources by data domain.
type Category string

const (
	CategoryScholarship Category = "scholarship"
	CategoryJob         Category = "job"
	CategoryVisa        Category = "visa"
	CategoryFX          Category = "fx"
	CategoryRemittance  Category = "remittance"
	CategoryNews        Category = "news"
	CategoryTravelAlert Category = "travel_alert"
)
