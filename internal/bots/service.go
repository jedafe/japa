package bots

import (
	"context"
	"fmt"
	"log/slog"
	"math"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/valkey"
)

// Service provides business logic for bot management operations.
type Service struct {
	store *Store
	rdb   *valkey.Client
}

// NewService creates a Service backed by the given database.
func NewService(database *db.DB) *Service {
	return &Service{store: NewStoreWithDB(database)}
}

// NewServiceFromStore creates a Service from an explicit Store instance.
func NewServiceFromStore(store *Store) *Service {
	return &Service{store: store}
}

// NewServiceWithValkey creates a Service with a Valkey client for cache
// invalidation after bot registry mutations.
func NewServiceWithValkey(store *Store, rdb *valkey.Client) *Service {
	return &Service{store: store, rdb: rdb}
}

// invalidateCaches flushes the robots.txt, llms.txt, and bot_list caches for
// the given platform so the next request regenerates them from the updated
// bot registry.
// Ref: japaconnect-engineering/sections/43-bot-crawler-management.md.
func (s *Service) invalidateCaches(ctx context.Context) {
	if s.rdb == nil {
		return
	}

	InvalidateRobotsCache(ctx, s.rdb)
	InvalidateLLMsCache(ctx, s.rdb)

	// Also invalidate the bot detection middleware's cached bot list.
	botListKey := valkey.MustKey("bot_list:")
	if err := s.rdb.Redis().Del(ctx, botListKey).Err(); err != nil {
		slog.Warn("bots: failed to invalidate bot_list cache", "error", err)
	}

	slog.Info("bots: invalidated robots/llms/bot_list caches after mutation")
}

// CreateKnownBotInput holds the fields needed to create a new known bot.
type CreateKnownBotInput struct {
	Name             string
	UserAgentPattern string
	BotType          BotType
	Policy           BotPolicy
	CrawlDelay       int
}

// UpdateKnownBotInput holds the fields needed to update a known bot.
type UpdateKnownBotInput struct {
	Name             string
	UserAgentPattern string
	BotType          BotType
	Policy           BotPolicy
	CrawlDelay       int
}

// BotResponse is the API response shape for a known bot.
type BotResponse struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	UserAgentPattern string `json:"user_agent_pattern"`
	BotType          string `json:"bot_type"`
	Policy           string `json:"policy"`
	IsSystem         bool   `json:"is_system"`
	CrawlDelay       int    `json:"crawl_delay"`
}

func botToResponse(b gen.KnownBot) BotResponse {
	return BotResponse{
		ID:               b.ID.String(),
		Name:             b.Name,
		UserAgentPattern: b.UserAgentPattern,
		BotType:          b.BotType,
		Policy:           b.Policy,
		IsSystem:         b.IsSystem,
		CrawlDelay:       int(b.CrawlDelay),
	}
}

// safeInt32 converts an int to int32, clamping to [0, math.MaxInt32] —
// negative crawl delays would emit "Crawl-delay: -5" into robots.txt.
func safeInt32(v int) int32 {
	if v > math.MaxInt32 {
		return math.MaxInt32
	}

	if v < 0 {
		return 0
	}

	// #nosec G115 // value bounded by caller
	return int32(v)
}

// ---------------------------------------------------------------------------
// Known Bots CRUD
// ---------------------------------------------------------------------------

// CreateKnownBot adds a new bot to the registry for the given platform.
func (s *Service) CreateKnownBot(ctx context.Context, input CreateKnownBotInput) (BotResponse, error) {
	bot, err := s.store.createKnownBot(ctx, gen.CreateKnownBotParams{
		Name:             input.Name,
		UserAgentPattern: input.UserAgentPattern,
		BotType:          string(input.BotType),
		Policy:           string(input.Policy),
		IsSystem:         false,
		CrawlDelay:       safeInt32(input.CrawlDelay),
	})
	if err != nil {
		return BotResponse{}, fmt.Errorf("create known bot: %w", err)
	}

	s.invalidateCaches(ctx)

	return botToResponse(bot), nil
}

// GetKnownBotByID returns a single bot by ID for the given platform.
func (s *Service) GetKnownBotByID(ctx context.Context, botID pgtype.UUID) (BotResponse, error) {
	bot, err := s.store.getKnownBotByID(ctx, botID)
	if err != nil {
		return BotResponse{}, fmt.Errorf("get known bot: %w", err)
	}

	return botToResponse(bot), nil
}

// ListKnownBots returns bots for the platform, optionally filtered by bot_type,
// with offset pagination (page_size/page_offset).
func (s *Service) ListKnownBots(ctx context.Context, botType string, pageSize, offset int32) ([]BotResponse, error) {
	bots, err := s.store.listKnownBots(ctx, botType, pageSize, offset)
	if err != nil {
		return nil, fmt.Errorf("list known bots: %w", err)
	}

	result := make([]BotResponse, len(bots))
	for i, b := range bots {
		result[i] = botToResponse(b)
	}

	return result, nil
}

// ListKnownBotsResult holds the paginated list response with total count.
type ListKnownBotsResult struct {
	Data       []BotResponse `json:"data"`
	Total      int64         `json:"total"`
	TotalPages int           `json:"total_pages"`
}

// ListKnownBotsWithCount returns bots for the platform with pagination metadata
// (total count and total_pages). The frontend DataTable uses these for
// server-side pagination.
// Ref: japaconnect-engineering/sections/43-bot-crawler-management.md.
func (s *Service) ListKnownBotsWithCount(ctx context.Context, botType string, pageSize, offset int32) (ListKnownBotsResult, error) {
	bots, err := s.store.listKnownBots(ctx, botType, pageSize, offset)
	if err != nil {
		return ListKnownBotsResult{}, fmt.Errorf("list known bots: %w", err)
	}

	total, err := s.store.countKnownBots(ctx, botType)
	if err != nil {
		return ListKnownBotsResult{}, fmt.Errorf("count known bots: %w", err)
	}

	result := make([]BotResponse, len(bots))
	for i, b := range bots {
		result[i] = botToResponse(b)
	}

	ps := int(pageSize)
	if ps <= 0 {
		ps = DefaultPageSize
	}

	totalPages := int(total) / ps
	if int(total)%ps != 0 {
		totalPages++
	}

	if totalPages == 0 && len(result) > 0 {
		totalPages = 1
	}

	return ListKnownBotsResult{
		Data:       result,
		Total:      total,
		TotalPages: totalPages,
	}, nil
}

// UpdateKnownBot modifies an existing bot entry.
func (s *Service) UpdateKnownBot(ctx context.Context, botID pgtype.UUID, input UpdateKnownBotInput) (BotResponse, error) {
	bot, err := s.store.updateKnownBot(ctx, gen.UpdateKnownBotParams{
		ID: botID,

		Name:             input.Name,
		UserAgentPattern: input.UserAgentPattern,
		BotType:          string(input.BotType),
		Policy:           string(input.Policy),
		CrawlDelay:       safeInt32(input.CrawlDelay),
	})
	if err != nil {
		return BotResponse{}, fmt.Errorf("update known bot: %w", err)
	}

	s.invalidateCaches(ctx)

	return botToResponse(bot), nil
}

// DeleteKnownBot removes a bot entry. System bots (is_system=true) are
// protected from deletion to preserve pre-seeded crawler configuration.
// Ref: japaconnect-engineering/sections/43-bot-crawler-management.md.
func (s *Service) DeleteKnownBot(ctx context.Context, botID pgtype.UUID) error {
	existing, err := s.store.getKnownBotByID(ctx, botID)
	if err != nil {
		return fmt.Errorf("delete known bot (lookup): %w", err)
	}

	if existing.IsSystem {
		return errpkg.ErrForbidden
	}

	if err := s.store.deleteKnownBot(ctx, botID); err != nil {
		return fmt.Errorf("delete known bot: %w", err)
	}

	s.invalidateCaches(ctx)

	return nil
}
