package bots

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/rss/model"
)

// Store provides low-level database access for bot management operations.
// The Service layer wraps Store methods with business logic.
type Store struct {
	write *gen.Queries
	read  *gen.Queries
}

// NewStore creates a Store backed by the given gen.Queries instances.
func NewStore(read, write *gen.Queries) *Store {
	return &Store{read: read, write: write}
}

// NewStoreWithDB creates a Store from a db.DB using its Reader and Writer pools.
func NewStoreWithDB(database *db.DB) *Store {
	return &Store{
		read:  gen.New(database.Reader),
		write: gen.New(database.Writer),
	}
}

// ---------------------------------------------------------------------------
// Known Bots CRUD
// ---------------------------------------------------------------------------

func (s *Store) createKnownBot(ctx context.Context, p gen.CreateKnownBotParams) (gen.KnownBot, error) {
	row, err := s.write.CreateKnownBot(ctx, p)
	if err != nil {
		return gen.KnownBot{}, fmt.Errorf("creating known bot: %w", err)
	}

	return row, nil
}

func (s *Store) getKnownBotByID(ctx context.Context, id pgtype.UUID) (gen.KnownBot, error) {
	row, err := s.read.GetKnownBotByID(ctx, id)
	if err != nil {
		return gen.KnownBot{}, fmt.Errorf("getting known bot by ID: %w", err)
	}

	return row, nil
}

func (s *Store) listKnownBots(ctx context.Context, botType string, pageSize, offset int32) ([]gen.KnownBot, error) {
	rows, err := s.read.ListKnownBots(ctx, gen.ListKnownBotsParams{
		BotType:    botType,
		PageSize:   pageSize,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing known bots: %w", err)
	}

	return rows, nil
}

// countKnownBots returns the total number of bots for the platform, optionally
// filtered by bot_type. Used for pagination metadata in the list handler.
func (s *Store) countKnownBots(ctx context.Context, botType string) (int64, error) {
	count, err := s.read.CountKnownBots(ctx,

		botType,
	)
	if err != nil {
		return 0, fmt.Errorf("counting known bots: %w", err)
	}

	return count, nil
}

func (s *Store) updateKnownBot(ctx context.Context, p gen.UpdateKnownBotParams) (gen.KnownBot, error) {
	row, err := s.write.UpdateKnownBot(ctx, p)
	if err != nil {
		return gen.KnownBot{}, fmt.Errorf("updating known bot: %w", err)
	}

	return row, nil
}

func (s *Store) deleteKnownBot(ctx context.Context, id pgtype.UUID) error {
	if err := s.write.DeleteKnownBot(ctx, id); err != nil {
		return fmt.Errorf("deleting known bot: %w", err)
	}

	return nil
}

// listAllKnownBots pages through the registry until exhausted — the registry
// is small, but a fixed MaxPageSize cap would silently truncate robots.txt
// and UA matching once it grows past one page.
func (s *Store) listAllKnownBots(ctx context.Context) ([]gen.KnownBot, error) {
	var all []gen.KnownBot

	var offset int32
	for {
		rows, err := s.read.ListKnownBots(ctx, gen.ListKnownBotsParams{
			BotType:    "",
			PageSize:   MaxPageSize,
			PageOffset: offset,
		})
		if err != nil {
			return nil, fmt.Errorf("listing all known bots: %w", err)
		}

		all = append(all, rows...)

		if len(rows) < MaxPageSize {
			break
		}

		offset += MaxPageSize
	}

	return all, nil
}

// getBotsByUserAgent matches an incoming User-Agent string against all
// known bot patterns. Returns all matching bots ordered by specificity
// (longer patterns first).
//
//nolint:unused // reserved for future features
func (s *Store) getBotsByUserAgent(ctx context.Context, userAgent string) ([]gen.KnownBot, error) {
	// Query uses ILIKE with pattern concatenation for substring matching.
	// We pass the raw user-agent and the query does pattern matching.
	rows, err := s.read.GetBotsByUserAgent(ctx,

		userAgent,
	)
	if err != nil {
		return nil, fmt.Errorf("getting bots by user agent: %w", err)
	}

	return rows, nil
}

// matchBotsByUserAgent performs in-memory pattern matching against the
// bot registry. This is more flexible than SQL ILIKE for complex patterns.
// Results are ordered by pattern specificity (longest pattern first) so the
// first element is the most specific match — e.g. "Googlebot" wins over "Bot".
func matchBotsByUserAgent(bots []gen.KnownBot, userAgent string) []KnownBot {
	ua := strings.ToLower(userAgent)

	var matched []KnownBot

	for _, b := range bots {
		if strings.Contains(ua, strings.ToLower(b.UserAgentPattern)) {
			matched = append(matched, KnownBot{
				Name:             b.Name,
				UserAgentPattern: b.UserAgentPattern,
				BotType:          BotType(b.BotType),
				Policy:           BotPolicy(b.Policy),
				IsSystem:         b.IsSystem,
				CrawlDelay:       int(b.CrawlDelay),
			})
		}
	}

	// Sort by pattern length descending so the most specific (longest) match
	// is first. This makes matched[0] the best pick for the middleware.
	sort.Slice(matched, func(i, j int) bool {
		return len(matched[i].UserAgentPattern) > len(matched[j].UserAgentPattern)
	})

	return matched
}

// GetBotsByUserAgent returns all known bots matching the given User-Agent
// string. Uses in-memory matching after fetching the full registry.
//

// GetBotsByUserAgent is an exported method.
func (s *Store) GetBotsByUserAgent(ctx context.Context, userAgent string) ([]KnownBot, error) {
	bots, err := s.listAllKnownBots(ctx)
	if err != nil {
		return nil, err
	}

	return matchBotsByUserAgent(bots, userAgent), nil
}

// GetAllBots returns every known bot in the registry (for robots.txt
// generation). Pages internally so the output never truncates.
//

// GetAllBots is an exported method.
func (s *Store) GetAllBots(ctx context.Context) ([]KnownBot, error) {
	rows, err := s.listAllKnownBots(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]KnownBot, len(rows))
	for i, r := range rows {
		result[i] = KnownBot{
			Name:             r.Name,
			UserAgentPattern: r.UserAgentPattern,
			BotType:          BotType(r.BotType),
			Policy:           BotPolicy(r.Policy),
			IsSystem:         r.IsSystem,
			CrawlDelay:       int(r.CrawlDelay),
		}
	}

	return result, nil
}

// ContentURLPath maps a content_type + slug to the canonical frontend URL path.
// Each content type has a distinct route prefix in the main SvelteKit app
// (apps/main/src/routes, ADR-058 route families — see
// apps/main/src/lib/utils/contentLink.ts). Used in LLMS.txt link generation
// and in SEO sitemap loc URLs (internal/seo) so crawlers see the canonical
// detail URLs (/blog/, /forum/, /story/, /guide/, ...) instead of the
// generic /content/ prefix.
// Ref: japaconnect-engineering/sections/43-bot-crawler-management.md.
// ContentURLPath delegates to the shared ADR-058 route-family map in
// internal/rss/model — previously this switch drifted (missing achievement/
// page/list types, and its default emitted /blog/ for unknown types instead
// of the generic /content/ route).
func ContentURLPath(contentType, slug string) string {
	return model.LinkPathFor(contentType) + slug
}

// GetLLMsData fetches the data needed for llms.txt generation: recent
// published + indexable posts, the top tags, and top active public spaces.
// Posts and spaces pass through the same Section 43 quality gates as the
// sitemaps (IsContentIndexableByType / IsSpaceIndexable) using cfg — if it
// isn't good enough for Google, it isn't advertised to AI crawlers either.
// Returned links are root-relative; the handler absolutizes them.
// Ref: japaconnect-engineering/sections/43-bot-crawler-management.md.
func (s *Store) GetLLMsData(ctx context.Context, cfg SitemapQualityConfig) (LLMsData, error) {
	posts, err := s.read.ListPublishedContentForLLMsTxt(ctx)
	if err != nil {
		return LLMsData{}, fmt.Errorf("listing posts for llms: %w", err)
	}

	recentPosts := make([]Link, 0, len(posts))
	for _, p := range posts {
		if p.Slug == "" {
			continue
		}

		isQuestion := p.IsQuestion != nil && *p.IsQuestion
		if !IsContentIndexableByType(ContentIndexableInput{
			ContentType:   p.ContentType,
			Status:        "published",
			Visibility:    p.Visibility,
			WordCount:     int(p.WordCount),
			CommentCount:  int(p.CommentCount),
			ShareCount:    int(p.ShareCount),
			ReactionCount: int(p.ReactionCount),
			ReplyCount:    int(p.ReplyCount),
			IsQuestion:    isQuestion,
			Solved:        p.Solved,
			SeoIndexable:  p.SeoIndexable,
			SkipWordGate:  p.ContentType == "forum_thread" || p.ContentType == "discussion",
		}, cfg) {
			continue
		}

		recentPosts = append(recentPosts, Link{Title: p.Title, URL: ContentURLPath(p.ContentType, p.Slug)})
	}

	tags, err := s.read.ListTopTagsForLLMsTxt(ctx)
	if err != nil {
		return LLMsData{}, fmt.Errorf("listing tags for llms: %w", err)
	}

	tagLinks := make([]Link, 0, len(tags))
	for _, tg := range tags {
		if tg.Slug == "" {
			continue
		}

		tagLinks = append(tagLinks, Link{Title: tg.Name, URL: "/tags/" + tg.Slug})
	}

	spaces, err := s.read.ListTopSpacesForLLMsTxt(ctx)
	if err != nil {
		return LLMsData{}, fmt.Errorf("listing spaces for llms: %w", err)
	}

	spaceLinks := make([]Link, 0, len(spaces))
	for _, sp := range spaces {
		if sp.Slug == "" {
			continue
		}

		if !IsSpaceIndexable(
			int(sp.MemberCount), int(sp.PostCount), int(sp.ForumCount), int(sp.QuestionCount),
			cfg.MinMembers, cfg.MinPosts, cfg.MinForums, cfg.MinQuestions,
		) {
			continue
		}

		spaceLinks = append(spaceLinks, Link{Title: sp.Name, URL: "/spaces/" + sp.Slug})
	}

	return LLMsData{
		RecentPosts: recentPosts,
		Tags:        tagLinks,
		Spaces:      spaceLinks,
	}, nil
}
