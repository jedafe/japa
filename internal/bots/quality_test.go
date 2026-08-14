package bots

import (
	"testing"
)

func TestIsSpaceIndexable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		members      int
		posts        int
		forums       int
		questions    int
		minMembers   int
		minPosts     int
		minForums    int
		minQuestions int
		want         bool
	}{
		{"meets defaults", 10, 5, 3, 2, 0, 0, 0, 0, true},
		{"below min members", 0, 5, 3, 2, 0, 0, 0, 0, false},
		{"below min posts", 10, 0, 3, 2, 0, 0, 0, 0, false},
		{"below min forums", 10, 5, 0, 2, 0, 0, 0, 0, false},
		{"below min questions", 10, 5, 3, 0, 0, 0, 0, 0, false},
		{"both below", 0, 0, 0, 0, 0, 0, 0, 0, false},
		{"custom thresholds meet", 20, 10, 5, 3, 15, 8, 4, 2, true},
		{"custom thresholds fail members", 10, 10, 5, 3, 15, 8, 4, 2, false},
		{"custom thresholds fail posts", 20, 5, 5, 3, 15, 8, 4, 2, false},
		{"custom thresholds fail forums", 20, 10, 2, 3, 15, 8, 4, 2, false},
		{"custom thresholds fail questions", 20, 10, 5, 1, 15, 8, 4, 2, false},
		{"zero thresholds use defaults", 10, 5, 3, 2, 0, 0, 0, 0, true},
		{"exact default threshold", 1, 1, 1, 1, 0, 0, 0, 0, true},
		{"just below default threshold members", 0, 1, 1, 1, 0, 0, 0, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := IsSpaceIndexable(tt.members, tt.posts, tt.forums, tt.questions, tt.minMembers, tt.minPosts, tt.minForums, tt.minQuestions)
			if got != tt.want {
				t.Errorf("IsSpaceIndexable(%d, %d, %d, %d, %d, %d, %d, %d) = %v, want %v",
					tt.members, tt.posts, tt.forums, tt.questions, tt.minMembers, tt.minPosts, tt.minForums, tt.minQuestions, got, tt.want)
			}
		})
	}
}

func TestIsContentIndexable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		status              string
		visibility          string
		wordCount           int
		minWords            int
		allowSearchIndexing bool
		want                bool
	}{
		{"published public with indexing", "published", "public", 200, 0, true, true},
		{"published private", "published", "private", 200, 0, true, false},
		{"draft published", "draft", "public", 200, 0, true, false},
		{"no indexing allowed", "published", "public", 200, 0, false, false},
		{"below min words", "published", "public", 50, 100, true, false},
		{"exact min words", "published", "public", 100, 100, true, true},
		{"above min words", "published", "public", 150, 100, true, true},
		{"empty visibility treated as public", "published", "", 200, 0, true, true},
		{"custom min words", "published", "public", 300, 250, true, true},
		{"default min words threshold", "published", "public", 100, 0, true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := IsContentIndexable(tt.status, tt.visibility, tt.wordCount, tt.minWords, tt.allowSearchIndexing)
			if got != tt.want {
				t.Errorf("IsContentIndexable(%q, %q, %d, %d, %v) = %v, want %v",
					tt.status, tt.visibility, tt.wordCount, tt.minWords, tt.allowSearchIndexing, got, tt.want)
			}
		})
	}
}

func TestIsContentIndexableByType(t *testing.T) {
	t.Parallel()

	def := DefaultSitemapQualityConfig()

	tests := []struct {
		name string
		in   ContentIndexableInput
		cfg  SitemapQualityConfig
		want bool
	}{
		// Blog posts — word count + engagement gate.
		{
			"blog post meets defaults",
			ContentIndexableInput{ContentType: "blog_post", Status: "published", Visibility: "public", WordCount: 150, SeoIndexable: true},
			def,
			true,
		},
		{
			"blog post below min words",
			ContentIndexableInput{ContentType: "blog_post", Status: "published", Visibility: "public", WordCount: 50, SeoIndexable: true},
			def,
			false,
		},
		{
			"blog post engagement gate pass (default 0)",
			ContentIndexableInput{ContentType: "blog_post", Status: "published", Visibility: "public", WordCount: 150, CommentCount: 0, SeoIndexable: true},
			def,
			true,
		},
		{
			"blog post engagement gate fail",
			ContentIndexableInput{ContentType: "blog_post", Status: "published", Visibility: "public", WordCount: 150, CommentCount: 0, SeoIndexable: true},
			SitemapQualityConfig{BlogMinWords: 100, BlogMinEngagement: 5},
			false,
		},
		{
			"blog post engagement gate pass with reactions",
			ContentIndexableInput{ContentType: "blog_post", Status: "published", Visibility: "public", WordCount: 150, CommentCount: 2, ShareCount: 1, ReactionCount: 2, SeoIndexable: true},
			SitemapQualityConfig{BlogMinWords: 100, BlogMinEngagement: 5},
			true,
		},
		// Forum threads — word count + reply gate + question/answer gate.
		{
			"forum thread meets defaults with replies",
			ContentIndexableInput{ContentType: "forum_thread", Status: "published", Visibility: "public", WordCount: 50, ReplyCount: 1, SeoIndexable: true},
			def,
			true,
		},
		{
			"forum thread below min replies",
			ContentIndexableInput{ContentType: "forum_thread", Status: "published", Visibility: "public", WordCount: 50, ReplyCount: 0, SeoIndexable: true},
			def,
			false,
		},
		{
			"forum thread below min words",
			ContentIndexableInput{ContentType: "forum_thread", Status: "published", Visibility: "public", WordCount: 10, ReplyCount: 1, SeoIndexable: true},
			def,
			false,
		},
		{
			"question solved passes answer gate",
			ContentIndexableInput{ContentType: "forum_thread", Status: "published", Visibility: "public", WordCount: 50, ReplyCount: 1, IsQuestion: true, Solved: true, SeoIndexable: true},
			def,
			true,
		},
		{
			"question unsolved fails answer gate",
			ContentIndexableInput{ContentType: "forum_thread", Status: "published", Visibility: "public", WordCount: 50, ReplyCount: 1, IsQuestion: true, Solved: false, SeoIndexable: true},
			def,
			false,
		},
		{
			"question unsolved passes when min_answers is 0",
			ContentIndexableInput{ContentType: "forum_thread", Status: "published", Visibility: "public", WordCount: 50, ReplyCount: 1, IsQuestion: true, Solved: false, SeoIndexable: true},
			SitemapQualityConfig{ForumMinWords: 30, ForumMinReplies: 1, QuestionMinAnswers: 0},
			true,
		},
		// Discussions — word count + comment gate.
		{
			"discussion meets defaults",
			ContentIndexableInput{ContentType: "discussion", Status: "published", Visibility: "public", WordCount: 60, CommentCount: 0, SeoIndexable: true},
			def,
			true,
		},
		{
			"discussion below min words",
			ContentIndexableInput{ContentType: "discussion", Status: "published", Visibility: "public", WordCount: 20, CommentCount: 0, SeoIndexable: true},
			def,
			false,
		},
		{
			"discussion comment gate fail",
			ContentIndexableInput{ContentType: "discussion", Status: "published", Visibility: "public", WordCount: 60, CommentCount: 0, SeoIndexable: true},
			SitemapQualityConfig{DiscussionMinWords: 50, DiscussionMinComments: 3},
			false,
		},
		{
			"discussion comment gate pass",
			ContentIndexableInput{ContentType: "discussion", Status: "published", Visibility: "public", WordCount: 60, CommentCount: 5, SeoIndexable: true},
			SitemapQualityConfig{DiscussionMinWords: 50, DiscussionMinComments: 3},
			true,
		},
		// Guides — higher word count threshold.
		{
			"guide meets default 200 words",
			ContentIndexableInput{ContentType: "guide", Status: "published", Visibility: "public", WordCount: 250, SeoIndexable: true},
			def,
			true,
		},
		{
			"guide below default 200 words",
			ContentIndexableInput{ContentType: "guide", Status: "published", Visibility: "public", WordCount: 150, SeoIndexable: true},
			def,
			false,
		},
		// Comparison — lower word count threshold.
		{
			"comparison meets default 50 words",
			ContentIndexableInput{ContentType: "comparison", Status: "published", Visibility: "public", WordCount: 60, SeoIndexable: true},
			def,
			true,
		},
		{
			"comparison below default 50 words",
			ContentIndexableInput{ContentType: "comparison", Status: "published", Visibility: "public", WordCount: 30, SeoIndexable: true},
			def,
			false,
		},
		// Common gates.
		{
			"not seo indexable",
			ContentIndexableInput{ContentType: "blog_post", Status: "published", Visibility: "public", WordCount: 200, SeoIndexable: false},
			def,
			false,
		},
		{
			"draft rejected",
			ContentIndexableInput{ContentType: "blog_post", Status: "draft", Visibility: "public", WordCount: 200, SeoIndexable: true},
			def,
			false,
		},
		{
			"private visibility rejected",
			ContentIndexableInput{ContentType: "blog_post", Status: "published", Visibility: "private", WordCount: 200, SeoIndexable: true},
			def,
			false,
		},
		{
			"empty cfg uses all defaults",
			ContentIndexableInput{ContentType: "blog_post", Status: "published", Visibility: "public", WordCount: 100, SeoIndexable: true},
			SitemapQualityConfig{},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := IsContentIndexableByType(tt.in, tt.cfg)
			if got != tt.want {
				t.Errorf("IsContentIndexableByType(%+v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestDefaultSitemapQualityConfig(t *testing.T) {
	t.Parallel()

	cfg := DefaultSitemapQualityConfig()
	if cfg.MinMembers != 1 {
		t.Errorf("expected MinMembers 1, got %d", cfg.MinMembers)
	}
	if cfg.MinPosts != 1 {
		t.Errorf("expected MinPosts 1, got %d", cfg.MinPosts)
	}
	if cfg.MinForums != 1 {
		t.Errorf("expected MinForums 1, got %d", cfg.MinForums)
	}
	if cfg.MinQuestions != 1 {
		t.Errorf("expected MinQuestions 1, got %d", cfg.MinQuestions)
	}
	if cfg.MinWords != 100 {
		t.Errorf("expected MinWords 100, got %d", cfg.MinWords)
	}
	if cfg.BlogMinWords != 100 {
		t.Errorf("expected BlogMinWords 100, got %d", cfg.BlogMinWords)
	}
	if cfg.ForumMinWords != 30 {
		t.Errorf("expected ForumMinWords 30, got %d", cfg.ForumMinWords)
	}
	if cfg.DiscussionMinWords != 50 {
		t.Errorf("expected DiscussionMinWords 50, got %d", cfg.DiscussionMinWords)
	}
	if cfg.GuideMinWords != 200 {
		t.Errorf("expected GuideMinWords 200, got %d", cfg.GuideMinWords)
	}
	if cfg.ComparisonMinWords != 50 {
		t.Errorf("expected ComparisonMinWords 50, got %d", cfg.ComparisonMinWords)
	}
	if cfg.BlogMinEngagement != 0 {
		t.Errorf("expected BlogMinEngagement 0, got %d", cfg.BlogMinEngagement)
	}
	if cfg.ForumMinReplies != 1 {
		t.Errorf("expected ForumMinReplies 1, got %d", cfg.ForumMinReplies)
	}
	if cfg.QuestionMinAnswers != 1 {
		t.Errorf("expected QuestionMinAnswers 1, got %d", cfg.QuestionMinAnswers)
	}
	if cfg.DiscussionMinComments != 0 {
		t.Errorf("expected DiscussionMinComments 0, got %d", cfg.DiscussionMinComments)
	}
}
