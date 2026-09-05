package bots

// SitemapQualityConfig holds configurable thresholds for sitemap inclusion.
// Values come from system_settings (seo.sitemap.*) — callers should read
// them via the settings reader and pass them to IsSpaceIndexable /
// IsContentIndexable / IsContentIndexableByType. The fallback defaults
// below match Section 43.
// Ref: japaconnect-engineering/sections/43-bot-crawler-management.md.
type SitemapQualityConfig struct {
	// Space-level gates.
	MinMembers   int
	MinPosts     int
	MinForums    int
	MinQuestions int
	// Content-level fallback word count (used when no per-type threshold is set).
	MinWords int
	// Per content-type word count thresholds.
	BlogMinWords       int
	ForumMinWords      int
	DiscussionMinWords int
	GuideMinWords      int
	ComparisonMinWords int
	// Per content-type engagement thresholds.
	BlogMinEngagement     int
	ForumMinReplies       int
	QuestionMinAnswers    int
	DiscussionMinComments int
}

// DefaultSitemapQualityConfig returns the spec fallback defaults for sitemap
// quality gates. These match the defaults in Section 43:
//   - seo.sitemap.space_min_members   → 1
//   - seo.sitemap.space_min_posts     → 1
//   - seo.sitemap.space_min_forums    → 1
//   - seo.sitemap.space_min_questions → 1
//   - seo.sitemap.content_min_words   → 100  (fallback)
//   - seo.sitemap.blog_min_words      → 100
//   - seo.sitemap.forum_min_words     → 30
//   - seo.sitemap.discussion_min_words → 50
//   - seo.sitemap.guide_min_words     → 200
//   - seo.sitemap.comparison_min_words → 50
//   - seo.sitemap.blog_min_engagement  → 0
//   - seo.sitemap.forum_min_replies    → 1
//   - seo.sitemap.question_min_answers → 1
//   - seo.sitemap.discussion_min_comments → 0
//
// Ref: japaconnect-engineering/sections/43-bot-crawler-management.md.
func DefaultSitemapQualityConfig() SitemapQualityConfig {
	return SitemapQualityConfig{
		MinMembers:            1,
		MinPosts:              1,
		MinForums:             1,
		MinQuestions:          1,
		MinWords:              100,
		BlogMinWords:          100,
		ForumMinWords:         30,
		DiscussionMinWords:    50,
		GuideMinWords:         200,
		ComparisonMinWords:    50,
		BlogMinEngagement:     0,
		ForumMinReplies:       1,
		QuestionMinAnswers:    1,
		DiscussionMinComments: 0,
	}
}

// IsSpaceIndexable checks whether a space should be included in the sitemap.
// A space must meet minimum member, post, forum, and question counts to be
// considered indexable. Callers should pass thresholds read from
// system_settings; if a threshold is <= 0 the spec fallback default is used.
func IsSpaceIndexable(memberCount, postCount, forumCount, questionCount, minMembers, minPosts, minForums, minQuestions int) bool {
	def := DefaultSitemapQualityConfig()
	if minMembers <= 0 {
		minMembers = def.MinMembers
	}

	if minPosts <= 0 {
		minPosts = def.MinPosts
	}

	if minForums <= 0 {
		minForums = def.MinForums
	}

	if minQuestions <= 0 {
		minQuestions = def.MinQuestions
	}

	return memberCount >= minMembers &&
		postCount >= minPosts &&
		forumCount >= minForums &&
		questionCount >= minQuestions
}

// IsContentIndexable checks whether a piece of content should be included
// in the sitemap using the fallback word count threshold. Content must be
// published, have public visibility, meet a minimum word count, and have
// search indexing allowed. Callers should pass minWords read from
// system_settings; if <= 0 the spec fallback default is used.
//
// For per-content-type word count and engagement thresholds, use
// IsContentIndexableByType instead.
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

// ContentIndexableInput holds the fields needed for per-content-type
// sitemap quality gating. This is the expanded version of
// IsContentIndexable that applies per-type word count and engagement
// thresholds per Section 43.
type ContentIndexableInput struct {
	ContentType   string // "blog_post", "forum_thread", "discussion", "guide", "comparison"
	Status        string
	Visibility    string
	WordCount     int
	CommentCount  int
	ShareCount    int
	ReactionCount int
	ReplyCount    int  // forum_threads only
	IsQuestion    bool // true for forum threads that are questions
	Solved        bool // true if the question has been answered
	SeoIndexable  bool // per-content seo_indexable flag
}

// IsContentIndexableByType applies the full Section 43 quality gate with
// per-content-type word count and engagement thresholds. The cfg struct
// should be populated from system_settings by the caller; any zero-value
// field falls back to the spec default.
//
// Engagement thresholds:
//   - blog_post:     commentCount + shareCount + reactionCount >= BlogMinEngagement
//   - forum_thread:  replyCount >= ForumMinReplies; if IsQuestion and not Solved and QuestionMinAnswers > 0, reject
//   - discussion:    commentCount >= DiscussionMinComments
//
// Word count thresholds are per-type; if the per-type threshold is <= 0,
// the fallback MinWords is used.
func IsContentIndexableByType(in ContentIndexableInput, cfg SitemapQualityConfig) bool {
	if !in.SeoIndexable {
		return false
	}

	if in.Status != "published" {
		return false
	}

	if in.Visibility != "public" && in.Visibility != "" {
		return false
	}

	def := DefaultSitemapQualityConfig()

	// Resolve the effective min word count for this content type.
	minWords := cfg.MinWords
	if minWords <= 0 {
		minWords = def.MinWords
	}

	switch in.ContentType {
	case "blog_post":
		if cfg.BlogMinWords > 0 {
			minWords = cfg.BlogMinWords
		} else if def.BlogMinWords > 0 {
			minWords = def.BlogMinWords
		}
	case "forum_thread":
		if cfg.ForumMinWords > 0 {
			minWords = cfg.ForumMinWords
		} else if def.ForumMinWords > 0 {
			minWords = def.ForumMinWords
		}
	case "discussion":
		if cfg.DiscussionMinWords > 0 {
			minWords = cfg.DiscussionMinWords
		} else if def.DiscussionMinWords > 0 {
			minWords = def.DiscussionMinWords
		}
	case "guide":
		if cfg.GuideMinWords > 0 {
			minWords = cfg.GuideMinWords
		} else if def.GuideMinWords > 0 {
			minWords = def.GuideMinWords
		}
	case "comparison":
		if cfg.ComparisonMinWords > 0 {
			minWords = cfg.ComparisonMinWords
		} else if def.ComparisonMinWords > 0 {
			minWords = def.ComparisonMinWords
		}
	}

	if in.WordCount < minWords {
		return false
	}

	// Per-content-type engagement gates.
	// Note: for engagement thresholds, 0 is a valid value meaning "no gate".
	// The caller is responsible for populating cfg from system_settings with
	// proper defaults (via GetIntOr). We do NOT fall back to defaults here
	// because that would make it impossible to disable a gate by setting it to 0.
	switch in.ContentType {
	case "blog_post":
		if in.CommentCount+in.ShareCount+in.ReactionCount < cfg.BlogMinEngagement {
			return false
		}
	case "forum_thread":
		if in.ReplyCount < cfg.ForumMinReplies {
			return false
		}

		if in.IsQuestion && cfg.QuestionMinAnswers > 0 && !in.Solved {
			return false
		}
	case "discussion":
		if in.CommentCount < cfg.DiscussionMinComments {
			return false
		}
	}

	return true
}
