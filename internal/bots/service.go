package bots

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Service provides business logic for bot management operations.
type Service struct {
	store *Store
}

// NewService creates a Service backed by the given database.
func NewService(database *db.DB) *Service {
	return &Service{store: NewStoreWithDB(database)}
}

// NewServiceFromStore creates a Service from an explicit Store instance.
func NewServiceFromStore(store *Store) *Service {
	return &Service{store: store}
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

// ---------------------------------------------------------------------------
// Known Bots CRUD
// ---------------------------------------------------------------------------

// CreateKnownBot adds a new bot to the registry for the given tenant.
func (s *Service) CreateKnownBot(ctx context.Context, tenantID pgtype.UUID, input CreateKnownBotInput) (BotResponse, error) {
	bot, err := s.store.createKnownBot(ctx, gen.CreateKnownBotParams{
		TenantID:         tenantID,
		Name:             input.Name,
		UserAgentPattern: input.UserAgentPattern,
		BotType:          string(input.BotType),
		Policy:           string(input.Policy),
		IsSystem:         false,
		CrawlDelay:       int32(input.CrawlDelay),
	})
	if err != nil {
		return BotResponse{}, fmt.Errorf("create known bot: %w", err)
	}
	return botToResponse(bot), nil
}

// GetKnownBotByID returns a single bot by ID for the given tenant.
func (s *Service) GetKnownBotByID(ctx context.Context, tenantID, botID pgtype.UUID) (BotResponse, error) {
	bot, err := s.store.getKnownBotByID(ctx, botID, tenantID)
	if err != nil {
		return BotResponse{}, fmt.Errorf("get known bot: %w", err)
	}
	return botToResponse(bot), nil
}

// ListKnownBots returns all bots for the tenant, optionally filtered by bot_type.
func (s *Service) ListKnownBots(ctx context.Context, tenantID pgtype.UUID, botType string) ([]BotResponse, error) {
	bots, err := s.store.listKnownBots(ctx, tenantID, botType)
	if err != nil {
		return nil, fmt.Errorf("list known bots: %w", err)
	}
	result := make([]BotResponse, len(bots))
	for i, b := range bots {
		result[i] = botToResponse(b)
	}
	return result, nil
}

// UpdateKnownBot modifies an existing bot entry.
func (s *Service) UpdateKnownBot(ctx context.Context, tenantID, botID pgtype.UUID, input UpdateKnownBotInput) (BotResponse, error) {
	bot, err := s.store.updateKnownBot(ctx, gen.UpdateKnownBotParams{
		ID:               botID,
		TenantID:         tenantID,
		Name:             input.Name,
		UserAgentPattern: input.UserAgentPattern,
		BotType:          string(input.BotType),
		Policy:           string(input.Policy),
		CrawlDelay:       int32(input.CrawlDelay),
	})
	if err != nil {
		return BotResponse{}, fmt.Errorf("update known bot: %w", err)
	}
	return botToResponse(bot), nil
}

// DeleteKnownBot removes a bot entry.
func (s *Service) DeleteKnownBot(ctx context.Context, tenantID, botID pgtype.UUID) error {
	if err := s.store.deleteKnownBot(ctx, botID, tenantID); err != nil {
		return fmt.Errorf("delete known bot: %w", err)
	}
	return nil
}
