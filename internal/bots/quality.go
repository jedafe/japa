package bots

// SitemapQualityConfig holds configurable thresholds for sitemap inclusion.
type SitemapQualityConfig struct {
	MinMembers int
	MinPosts   int
	MinWords   int
}

// DefaultSitemapQualityConfig returns sensible defaults for sitemap quality gates.
func DefaultSitemapQualityConfig() SitemapQualityConfig {
	return SitemapQualityConfig{
		MinMembers: 5,
		MinPosts:   3,
		MinWords:   100,
	}
}

// IsSpaceIndexable checks whether a space should be included in the sitemap.
// A space must meet minimum member and post counts to be considered indexable.
func IsSpaceIndexable(memberCount, postCount int, minMembers, minPosts int) bool {
	if minMembers <= 0 {
		minMembers = DefaultSitemapQualityConfig().MinMembers
	}
	if minPosts <= 0 {
		minPosts = DefaultSitemapQualityConfig().MinPosts
	}
	return memberCount >= minMembers && postCount >= minPosts
}

// IsContentIndexable checks whether a piece of content should be included
// in the sitemap. Content must be published, have public visibility,
// meet a minimum word count, and have search indexing allowed.
func IsContentIndexable(status, visibility string, wordCount, minWords int, allowSearchIndexing bool) bool {
	if !allowSearchIndexing {
		return false
	}
	if status != "published" {
		return false
	}
	if visibility != "public" && visibility != "" {
		return false
	}
	if minWords <= 0 {
		minWords = DefaultSitemapQualityConfig().MinWords
	}
	return wordCount >= minWords
}
