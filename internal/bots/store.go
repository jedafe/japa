package bots

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
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

//nolint:unused // reserved for future features
func (s *Store) createKnownBot(ctx context.Context, p gen.CreateKnownBotParams) (gen.KnownBot, error) {
	row, err := s.write.CreateKnownBot(ctx, p)
	if err != nil {
		return gen.KnownBot{}, fmt.Errorf("creating known bot: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) getKnownBotByID(ctx context.Context, id, tenantID pgtype.UUID) (gen.KnownBot, error) {
	row, err := s.read.GetKnownBotByID(ctx, gen.GetKnownBotByIDParams{ID: id, TenantID: tenantID})
	if err != nil {
		return gen.KnownBot{}, fmt.Errorf("getting known bot by ID: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) listKnownBots(ctx context.Context, tenantID pgtype.UUID, botType string, pageSize, offset int32) ([]gen.KnownBot, error) {
	rows, err := s.read.ListKnownBots(ctx, gen.ListKnownBotsParams{
		TenantID:   tenantID,
		BotType:    botType,
		PageSize:   pageSize,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing known bots: %w", err)
	}

	return rows, nil
}

// countKnownBots returns the total number of bots for the tenant, optionally
// filtered by bot_type. Used for pagination metadata in the list handler.
func (s *Store) countKnownBots(ctx context.Context, tenantID pgtype.UUID, botType string) (int64, error) {
	count, err := s.read.CountKnownBots(ctx, gen.CountKnownBotsParams{
		TenantID: tenantID,
		BotType:  botType,
	})
	if err != nil {
		return 0, fmt.Errorf("counting known bots: %w", err)
	}

	return count, nil
}

//nolint:unused // reserved for future features
func (s *Store) updateKnownBot(ctx context.Context, p gen.UpdateKnownBotParams) (gen.KnownBot, error) {
	row, err := s.write.UpdateKnownBot(ctx, p)
	if err != nil {
		return gen.KnownBot{}, fmt.Errorf("updating known bot: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) deleteKnownBot(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.write.DeleteKnownBot(ctx, gen.DeleteKnownBotParams{ID: id, TenantID: tenantID}); err != nil {
		return fmt.Errorf("deleting known bot: %w", err)
	}

	return nil
}

// getBotsByUserAgent matches an incoming User-Agent string against all
// known bot patterns for the tenant. Returns all matching bots ordered
// by specificity (longer patterns first).
//
//nolint:unused // reserved for future features
func (s *Store) getBotsByUserAgent(ctx context.Context, tenantID pgtype.UUID, userAgent string) ([]gen.KnownBot, error) {
	// Query uses ILIKE with pattern concatenation for substring matching.
	// We pass the raw user-agent and the query does pattern matching.
	rows, err := s.read.GetBotsByUserAgent(ctx, gen.GetBotsByUserAgentParams{
		TenantID:         tenantID,
		UserAgentPattern: userAgent,
	})
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
// string for the specified tenant. Uses in-memory matching after fetching
// all bots for the tenant.
//
//nolint:unused // reserved for future features
func (s *Store) GetBotsByUserAgent(ctx context.Context, tenantID pgtype.UUID, userAgent string) ([]KnownBot, error) {
	bots, err := s.listKnownBots(ctx, tenantID, "", MaxPageSize, 0)
	if err != nil {
		return nil, err
	}

	return matchBotsByUserAgent(bots, userAgent), nil
}

// GetAllBots returns all known bots for the tenant (for robots.txt generation).
//
//nolint:unused // reserved for future features
func (s *Store) GetAllBots(ctx context.Context, tenantID pgtype.UUID) ([]KnownBot, error) {
	rows, err := s.read.ListKnownBots(ctx, gen.ListKnownBotsParams{
		TenantID:   tenantID,
		BotType:    "",
		PageSize:   MaxPageSize,
		PageOffset: 0,
	})
	if err != nil {
		return nil, fmt.Errorf("getting all bots: %w", err)
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

// contentURLPath maps a content_type + slug to the canonical frontend URL path
// used in LLMS.txt link generation. Each content type has a distinct route
// prefix in the main SvelteKit app (apps/main/src/routes).
// Ref: japaconnect-engineering/sections/43-bot-crawler-management.md
func contentURLPath(contentType, slug string) string {
	switch contentType {
	case "blog_post":
		return "/blog/" + slug
	case "forum_thread":
		return "/forum/" + slug
	case "guide":
		return "/blog/" + slug
	case "discussion":
		return "/forum/" + slug
	case "comparison":
		return "/blog/" + slug
	default:
		return "/blog/" + slug
	}
}

// GetLLMsData fetches the data needed for llms-full.txt generation:
// recent published posts, top tags, and active public spaces.
// Ref: japaconnect-engineering/sections/43-bot-crawler-management.md
func (s *Store) GetLLMsData(ctx context.Context, tenantID pgtype.UUID) (LLMsData, error) {
	posts, err := s.read.ListPublishedContentForLLMsTxt(ctx, tenantID)
	if err != nil {
		return LLMsData{}, fmt.Errorf("listing posts for llms: %w", err)
	}

	recentPosts := make([]Link, 0, len(posts))
	for _, p := range posts {
		recentPosts = append(recentPosts, Link{Title: p.Title, URL: contentURLPath(p.ContentType, p.Slug)})
	}

	tags, err := s.read.ListTagsByTenant(ctx, tenantID)
	if err != nil {
		return LLMsData{}, fmt.Errorf("listing tags for llms: %w", err)
	}

	tagLinks := make([]Link, 0, len(tags))
	for _, tg := range tags {
		tagLinks = append(tagLinks, Link{Title: tg.Name, URL: "/tags/" + tg.Slug})
	}

	spaces, err := s.read.ListSpacesByTenant(ctx, gen.ListSpacesByTenantParams{
		TenantID:     tenantID,
		ViewerUserID: pgtype.UUID{},
	})
	if err != nil {
		return LLMsData{}, fmt.Errorf("listing spaces for llms: %w", err)
	}

	spaceLinks := make([]Link, 0, len(spaces))
	for _, sp := range spaces {
		spaceLinks = append(spaceLinks, Link{Title: sp.Name, URL: "/spaces/" + sp.Slug})
	}

	return LLMsData{
		RecentPosts: recentPosts,
		Tags:        tagLinks,
		Spaces:      spaceLinks,
	}, nil
}
