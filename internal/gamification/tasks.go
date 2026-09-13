package gamification

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
)

// ---------------------------------------------------------------------------
// ExpireChallenges — called by gamification:expire_challenges task
// ---------------------------------------------------------------------------

// ExpireChallenges deactivates challenges whose ends_at is in the past.
// Called daily by the gamification:expire_challenges Asynq task.
func (s *Service) ExpireChallenges(ctx context.Context, tenantID pgtype.UUID) (int64, error) {
	count, err := s.store.ExpireChallengesPastEndDate(ctx, tenantID)
	if err != nil {
		return 0, fmt.Errorf("gamification: expire challenges: %w", err)
	}

	if count > 0 {
		slog.Info("gamification: expired challenges", "count", count)
	}

	return count, nil
}
