package content

import (
	"context"
	"fmt"

	"github.com/jedafe/japa/internal/db/gen"
)

// ---------------------------------------------------------------------------
// Share Platforms (admin-configurable social media sharing)
// ---------------------------------------------------------------------------

// ListSharePlatforms returns all share platforms for a tenant, ordered by position.
func (s *Service) ListSharePlatforms(ctx context.Context) ([]gen.SharePlatform, error) {
	platforms, err := s.read.ListSharePlatforms(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing share platforms: %w", err)
	}

	return platforms, nil
}

// ListEnabledSharePlatforms returns only enabled share platforms for a tenant.
func (s *Service) ListEnabledSharePlatforms(ctx context.Context) ([]gen.SharePlatform, error) {
	platforms, err := s.read.ListEnabledSharePlatforms(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing enabled share platforms: %w", err)
	}

	return platforms, nil
}

// CreateSharePlatform creates a new share platform.
func (s *Service) CreateSharePlatform(ctx context.Context, key, label, icon, urlTemplate, color string, enabled bool, position int32) (gen.SharePlatform, error) {
	p, err := s.write.CreateSharePlatform(ctx, gen.CreateSharePlatformParams{

		Key:         key,
		Label:       label,
		Icon:        icon,
		UrlTemplate: urlTemplate,
		Color:       color,
		Enabled:     enabled,
		Position:    position,
	})
	if err != nil {
		return gen.SharePlatform{}, fmt.Errorf("creating share platform: %w", err)
	}

	return p, nil
}

// UpdateSharePlatform updates an existing share platform by key.
func (s *Service) UpdateSharePlatform(ctx context.Context, key, label, icon, urlTemplate, color string, enabled bool, position int32) (gen.SharePlatform, error) {
	p, err := s.write.UpdateSharePlatform(ctx, gen.UpdateSharePlatformParams{

		Key:         key,
		Label:       label,
		Icon:        icon,
		UrlTemplate: urlTemplate,
		Color:       color,
		Enabled:     enabled,
		Position:    position,
	})
	if err != nil {
		return gen.SharePlatform{}, fmt.Errorf("updating share platform: %w", err)
	}

	return p, nil
}

// DeleteSharePlatform removes a share platform by tenant and key.
func (s *Service) DeleteSharePlatform(ctx context.Context, key string) error {
	if err := s.write.DeleteSharePlatform(ctx,

		key,
	); err != nil {
		return fmt.Errorf("deleting share platform: %w", err)
	}

	return nil
}
