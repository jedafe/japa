package nextactions

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/features"
)

// FlagChecker is the narrow features.Checker surface the engine needs —
// satisfied by *features.Checker; nil disables feature_enabled predicates
// (they fail closed, suppressing the suggestion).
type FlagChecker interface {
	IsEnabled(ctx context.Context, key string) bool
	EvalFlag(ctx context.Context, key string, sc features.CallerScope) features.EvalResult
}

// errNoFlagChecker marks the "no flag checker wired" probe failure —
// evalDepsBool folds it to false like any other deps error.
var errNoFlagChecker = errors.New("nextactions: no flag checker configured")

// queryDeps is the production EvalDeps — all probes backed by sqlc queries
// against the read pool. Session-scoped counts key on session_id, user counts
// on user_id; guests have no user-scoped probes (they return false).
type queryDeps struct {
	read  *gen.Queries
	flags FlagChecker
}

func newQueryDeps(read *gen.Queries, flags FlagChecker) *queryDeps {
	return &queryDeps{read: read, flags: flags}
}

func (d *queryDeps) FeatureEnabled(ctx context.Context, key string) (bool, error) {
	if d.flags == nil {
		return false, errNoFlagChecker
	}
	// §42: request paths carry a caller scope (attached in Suggestions) —
	// a flag enabled globally but scoped away from this caller must not
	// nudge them into a feature that would 404. Unattached contexts
	// (sweeps, worker replays) evaluate globally.
	if sc, ok := features.ScopeFrom(ctx); ok {
		res := d.flags.EvalFlag(ctx, key, sc)
		return res.Enabled && res.Visible, nil
	}
	return d.flags.IsEnabled(ctx, key), nil
}

func (d *queryDeps) CountEvents(ctx context.Context, s *UserState, eventType string, days int, session bool) (int64, error) {
	if s.UserID.Valid && !session {
		return d.read.CountUserEvents(ctx, gen.CountUserEventsParams{
			UserID:    s.UserID,
			EventType: eventType,
			Days:      int32(days), // #nosec G115 — bounded by windowDays parse
		})
	}
	return d.read.CountSessionEvents(ctx, gen.CountSessionEventsParams{
		SessionID: &s.SessionKey,
		EventType: eventType,
		Days:      int32(days), // #nosec G115 — bounded
	})
}

func (d *queryDeps) ToolUsed(ctx context.Context, s *UserState, slug string, days int) (bool, error) {
	var n int64
	var err error
	if s.UserID.Valid {
		n, err = d.read.CountUserToolEvents(ctx, gen.CountUserToolEventsParams{
			UserID: s.UserID, ToolSlug: slug, Days: int32(days), // #nosec G115 — bounded
		})
	} else {
		n, err = d.read.CountSessionToolEvents(ctx, gen.CountSessionToolEventsParams{
			SessionID: &s.SessionKey, ToolSlug: slug, Days: int32(days), // #nosec G115 — bounded
		})
	}
	return err == nil && n > 0, err
}

func (d *queryDeps) FeatureUsed(ctx context.Context, s *UserState, slug string, days int) (bool, error) {
	n, err := d.CountEvents(ctx, s, "feature."+slug, days, false)
	return err == nil && n > 0, err
}

func (d *queryDeps) GateHit(ctx context.Context, s *UserState, feature string, days int) (bool, error) {
	// feature_gate_hit events carry meta.feature; a slug filter uses the same
	// meta->> match convention as tool events. Empty feature matches any hit.
	if feature != "" {
		var n int64
		var err error
		if s.UserID.Valid {
			n, err = d.read.CountUserGateHitEvents(ctx, gen.CountUserGateHitEventsParams{
				UserID: s.UserID, Feature: feature, Days: int32(days), // #nosec G115 — bounded
			})
		} else {
			n, err = d.read.CountSessionGateHitEvents(ctx, gen.CountSessionGateHitEventsParams{
				SessionID: &s.SessionKey, Feature: feature, Days: int32(days), // #nosec G115 — bounded
			})
		}
		return err == nil && n > 0, err
	}
	n, err := d.CountEvents(ctx, s, "feature.gate_hit", days, false)
	return err == nil && n > 0, err
}

func (d *queryDeps) AssessmentDone(ctx context.Context, userID pgtype.UUID) (bool, int, error) {
	done, err := d.read.UserHasAssessmentResult(ctx, userID)
	if err != nil || !done {
		return done, 0, err
	}
	score, err := d.read.GetLatestAssessmentScore(ctx, userID)
	if err != nil {
		return true, 0, nil // scored result missing is non-fatal — done, unknown score
	}
	return true, int(score), nil
}

func (d *queryDeps) DraftExists(ctx context.Context, userID pgtype.UUID, kind string) (bool, error) {
	return d.read.UserHasDraftOfKind(ctx, gen.UserHasDraftOfKindParams{UserID: userID, Kind: kind})
}

func (d *queryDeps) HasPushSub(ctx context.Context, userID pgtype.UUID) (bool, error) {
	return d.read.UserHasPushSubscription(ctx, userID)
}
