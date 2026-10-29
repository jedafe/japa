package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/content"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/publicapi"
)

// publicReactionAdapter composes the session-side reaction semantics
// (content visibility, enabled-type / §89 achievement validation,
// engagement-count enqueue) for key-scoped reaction writes — parity with
// POST/DELETE /content/{id}/react(ions).
type publicReactionAdapter struct {
	svc      *content.Service
	settings *config.Settings
	onChange func(entityType string, entityID pgtype.UUID, engagementType string, delta int)
}

// allowedAchievementReaction mirrors the session handler's §89 check — the
// curated emoji set comes from system_settings.achievement_reactions with a
// spec default of 🎉 🔥 💪.
func (a *publicReactionAdapter) allowedAchievementReaction(ctx context.Context, emoji string) bool {
	allowed := []string{"🎉", "🔥", "💪"}
	if a.settings != nil {
		raw := a.settings.GetStringOr(ctx, "achievement_reactions", "")
		var parsed []struct {
			Emoji string `json:"emoji"`
		}
		if raw != "" && json.Unmarshal([]byte(raw), &parsed) == nil && len(parsed) > 0 {
			allowed = allowed[:0]
			for _, r := range parsed {
				allowed = append(allowed, r.Emoji)
			}
		}
	}
	for _, e := range allowed {
		if e == emoji {
			return true
		}
	}
	return false
}

func (a *publicReactionAdapter) ListReactions(ctx context.Context, contentID pgtype.UUID) ([]publicapi.PublicReactionCount, error) {
	counts, err := a.svc.CountReactionsByEntity(ctx, content.EntityContent, contentID)
	if err != nil {
		return nil, err
	}
	out := make([]publicapi.PublicReactionCount, len(counts))
	for i, c := range counts {
		out[i] = publicapi.PublicReactionCount{ReactionKey: c.ReactionKey, Count: c.Count}
	}
	return out, nil
}

func (a *publicReactionAdapter) AddReaction(ctx context.Context, userID, contentID pgtype.UUID, reactionKey string) (publicapi.PublicReaction, error) {
	// Same visibility rule as the session path — 404 never leaks.
	visible, err := a.svc.CanReactToContent(ctx, contentID, userID)
	if err != nil {
		return publicapi.PublicReaction{}, err
	}
	if !visible {
		return publicapi.PublicReaction{}, errpkg.ErrNotFound
	}

	existing, err := a.svc.GetContent(ctx, contentID)
	if err != nil {
		return publicapi.PublicReaction{}, errpkg.ErrNotFound
	}

	if existing.ContentType == content.TypeAchievement {
		if !a.allowedAchievementReaction(ctx, reactionKey) {
			return publicapi.PublicReaction{}, fmt.Errorf("%w: invalid_reaction_key", errpkg.ErrInvalid)
		}
	} else {
		rt, rtErr := a.svc.GetReactionType(ctx, reactionKey)
		if rtErr != nil || !rt.Enabled {
			return publicapi.PublicReaction{}, fmt.Errorf("%w: invalid_reaction_key", errpkg.ErrInvalid)
		}
	}

	reaction, err := a.svc.AddReaction(ctx, content.EntityContent, contentID, userID, reactionKey)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return publicapi.PublicReaction{EntityType: content.EntityContent, EntityID: contentID.String(), ReactionKey: reactionKey}, nil
		}
		return publicapi.PublicReaction{}, err
	}

	if a.onChange != nil {
		a.onChange("content", contentID, "reaction", 1)
	}

	return publicapi.PublicReaction{
		EntityType:  reaction.EntityType,
		EntityID:    reaction.EntityID.String(),
		ReactionKey: reaction.ReactionKey,
		CreatedAt:   reaction.CreatedAt.Time.Format("2006-01-02T15:04:05Z07:00"),
	}, nil
}

func (a *publicReactionAdapter) RemoveReaction(ctx context.Context, userID, contentID pgtype.UUID, reactionKey string) error {
	visible, err := a.svc.CanReactToContent(ctx, contentID, userID)
	if err != nil {
		return err
	}
	if !visible {
		return errpkg.ErrNotFound
	}

	removed, err := a.svc.RemoveReaction(ctx, content.EntityContent, contentID, userID, reactionKey)
	if err != nil {
		return err
	}
	if removed && a.onChange != nil {
		a.onChange("content", contentID, "reaction", -1)
	}
	return nil
}
