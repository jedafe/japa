// Package bots implements Bot & Crawler Management for JapaConnect (Section 43).
//
// Three-layer architecture:
//   - Known Bots registry (admin-configurable per platform)
//   - Dynamic robots.txt generation from the bot registry
//   - Bot detection middleware that identifies incoming crawlers
//
// robots.txt is generated dynamically from the bot registry, NOT served
// from a static file. The generated output is cached in Valkey with a
// 24-hour TTL (jc:robots_txt). LLMS.txt is cached with a 1-hour TTL
// (jc:llms_txt).
package bots

// BotType classifies a known bot by its primary function.
type BotType string

const (
	// BotTypeSearchEngine is the bot type search engine constant.
	BotTypeSearchEngine BotType = "search_engine"
	// BotTypeAILLM is the bot type a i l m.
	BotTypeAILLM BotType = "ai_llm"
	// BotTypeSocial is the bot type social.
	BotTypeSocial BotType = "social"
	// BotTypeMonitoring is the bot type monitoring.
	BotTypeMonitoring BotType = "monitoring"
	// BotTypeScraper is the bot type scraper.
	BotTypeScraper BotType = "scraper"
	// BotTypeUnknown is the bot type unknown.
	BotTypeUnknown BotType = "unknown"
)

// BotPolicy determines how a bot is treated when crawling the site.
type BotPolicy string

const (
	// PolicyAllow is the policy allow constant.
	PolicyAllow BotPolicy = "allow"
	// PolicyBlock is the policy block.
	PolicyBlock BotPolicy = "block"
	// PolicyLimit is the policy limit.
	PolicyLimit BotPolicy = "limit"
)

// KnownBot represents a registered crawler with its detection pattern and policy.
type KnownBot struct {
	Name             string
	UserAgentPattern string
	BotType          BotType
	Policy           BotPolicy
	IsSystem         bool
	CrawlDelay       int
}

// AllBotTypes returns all valid bot type values.
func AllBotTypes() []BotType {
	return []BotType{
		BotTypeSearchEngine,
		BotTypeAILLM,
		BotTypeSocial,
		BotTypeMonitoring,
		BotTypeScraper,
		BotTypeUnknown,
	}
}

// AllPolicies returns all valid bot policy values.
func AllPolicies() []BotPolicy {
	return []BotPolicy{
		PolicyAllow,
		PolicyBlock,
		PolicyLimit,
	}
}

// IsBotType checks whether a string is a valid BotType.
func IsBotType(s string) bool {
	for _, bt := range AllBotTypes() {
		if string(bt) == s {
			return true
		}
	}

	return false
}

// IsPolicy checks whether a string is a valid BotPolicy.
func IsPolicy(s string) bool {
	for _, p := range AllPolicies() {
		if string(p) == s {
			return true
		}
	}

	return false
}

// Pagination defaults.
const (
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// Link represents a URL entry for LLMS.txt generation.
type Link struct {
	Title string
	URL   string
}

// LLMsData holds data needed for the full LLMS.txt generation.
type LLMsData struct {
	RecentPosts []Link
	Tags        []Link
	Spaces      []Link
}
